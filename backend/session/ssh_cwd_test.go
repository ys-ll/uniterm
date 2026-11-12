package session

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"github.com/ys-ll/uniterm/backend/mcp"
	"golang.org/x/crypto/ssh"
)

// The test server exposes Linux-shaped proc files over real SSH/SFTP even
// when these tests run on Windows. No shell executes commands on the host.
type testCwdFS struct {
	mu        sync.Mutex
	files     map[string]string
	links     map[string]string
	children  []int
	fallback  bool
	afterLink func()
}

func procStat(pid, parent, sid int, tty int64, start uint64) string {
	f := make([]string, 20)
	for i := range f {
		f[i] = "0"
	}
	f[0], f[1], f[3], f[4], f[19] = "S", strconv.Itoa(parent), strconv.Itoa(sid), strconv.FormatInt(tty, 10), strconv.FormatUint(start, 10)
	return fmt.Sprintf("%d (a shell (name)) %s\n", pid, strings.Join(f, " "))
}

func newTestCwdFS() *testCwdFS {
	return &testCwdFS{
		files: map[string]string{"/proc/self/stat": procStat(900, 42, 900, 0, 100)},
		links: make(map[string]string),
	}
}

func (f *testCwdFS) addTerminal(pid int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.children = append(f.children, pid)
	f.files[fmt.Sprintf("/proc/%d/stat", pid)] = procStat(pid, 42, pid, int64(pid), uint64(pid+1000))
	f.links[fmt.Sprintf("/proc/%d/cwd", pid)] = fmt.Sprintf("/home/terminal-%d", pid)
	f.links[fmt.Sprintf("/proc/%d/exe", pid)] = "/usr/bin/bash"
}

func (f *testCwdFS) setLink(name, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.links[name] = value
}

func (f *testCwdFS) ReadFile(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if name == "/proc/42/task/42/children" && !f.fallback {
		pids := []string{"900"}
		for _, pid := range f.children {
			pids = append(pids, strconv.Itoa(pid))
		}
		return []byte(strings.Join(pids, " ")), nil
	}
	if data, ok := f.files[name]; ok {
		return []byte(data), nil
	}
	return nil, os.ErrNotExist
}

type cwdTestInfo string

func (f cwdTestInfo) Name() string       { return string(f) }
func (f cwdTestInfo) Size() int64        { return 0 } // proc files have zero size
func (f cwdTestInfo) Mode() os.FileMode  { return 0444 }
func (f cwdTestInfo) ModTime() time.Time { return time.Time{} }
func (f cwdTestInfo) IsDir() bool        { return false }
func (f cwdTestInfo) Sys() any           { return nil }

func (f *testCwdFS) ReadDir(name string) ([]os.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if name != "/proc" {
		return nil, os.ErrNotExist
	}
	entries := []os.FileInfo{cwdTestInfo("self"), cwdTestInfo("900")}
	for _, pid := range f.children {
		entries = append(entries, cwdTestInfo(strconv.Itoa(pid)))
	}
	return entries, nil
}

func (f *testCwdFS) ReadLink(name string) (string, error) {
	f.mu.Lock()
	v, ok := f.links[name]
	after := f.afterLink
	f.mu.Unlock()
	if after != nil {
		after()
	}
	if !ok {
		return "", os.ErrNotExist
	}
	return v, nil
}

func TestCwdProcessIdentityAndDiscovery(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(strconv.FormatBool(fallback), func(t *testing.T) {
			fs := newTestCwdFS()
			fs.fallback = fallback
			fs.addTerminal(101)
			assigned := make(map[cwdProcessID]bool)
			p, err := findCwdTerminal(fs, assigned)
			if err != nil || p.pid != 101 || p.start != 1101 {
				t.Fatalf("identity = %+v, %v", p, err)
			}
			assigned[p.cwdProcessID] = true
			fs.addTerminal(102)
			clone, err := findCwdTerminal(fs, assigned)
			if err != nil || clone.pid != 102 {
				t.Fatalf("clone = %+v, %v", clone, err)
			}
			// Two new PTYs must never be guessed by PID order.
			fs.addTerminal(103)
			if _, err := findCwdTerminal(fs, assigned); err == nil {
				t.Fatal("accepted ambiguous PTYs")
			}
		})
	}
	for _, data := range []string{"", "12 (broken)", "-1 (x) S 1 2", "12 x S"} {
		if _, err := parseCwdProcess([]byte(data)); err == nil {
			t.Fatalf("accepted malformed stat %q", data)
		}
	}
}

func TestCwdRejectsPIDReuseDuringReadlink(t *testing.T) {
	fs := newTestCwdFS()
	fs.addTerminal(101)
	p, err := findCwdTerminal(fs, nil)
	if err != nil {
		t.Fatal(err)
	}
	fs.setLink("/proc/101/cwd", "/tmp/space dir/中文")
	if cwd, err := readTerminalCwd(fs, p); err != nil || cwd != "/tmp/space dir/中文" {
		t.Fatalf("cwd=%q err=%v", cwd, err)
	}
	fs.afterLink = func() {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		fs.files["/proc/101/stat"] = procStat(101, 42, 101, 101, 99999)
	}
	if _, err := readTerminalCwd(fs, p); err == nil {
		t.Fatal("accepted reused PID")
	}
}

func (f *testCwdFS) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	data, err := f.ReadFile(r.Filepath)
	return strings.NewReader(string(data)), err
}

type cwdTestList []os.FileInfo

func (l cwdTestList) ListAt(dst []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}
	n := copy(dst, l[offset:])
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}
func (f *testCwdFS) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	if r.Method == "List" {
		entries, err := f.ReadDir(r.Filepath)
		return cwdTestList(entries), err
	}
	return cwdTestList{cwdTestInfo(r.Filepath)}, nil
}
func (f *testCwdFS) Readlink(name string) (string, error) { return f.ReadLink(name) }

type cwdServerCounters struct {
	connections atomic.Int32
	shells      atomic.Int32
	subsystems  atomic.Int32
	execs       atomic.Int32
	input       atomic.Int32
}

func startCwdSSHServer(t *testing.T, fs *testCwdFS, subsystemMode string, execHandlers ...func(string) ([]byte, error)) (string, *cwdServerCounters) {
	t.Helper()
	signer, err := ssh.ParsePrivateKey([]byte(testHostKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{ServerVersion: "SSH-2.0-OpenSSH_9.9", PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	counters := &cwdServerCounters{}
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			counters.connections.Add(1)
			go func() {
				defer raw.Close()
				_, channels, requests, err := ssh.NewServerConn(raw, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					if incoming.ChannelType() != "session" {
						incoming.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					ch, reqs, err := incoming.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer ch.Close()
						for req := range reqs {
							switch req.Type {
							case "pty-req", "window-change":
								req.Reply(true, nil)
							case "shell":
								pid := 100 + int(counters.shells.Add(1))
								fs.addTerminal(pid)
								req.Reply(true, nil)
								_, _ = ch.Write([]byte("Native MOTD\r\n$ "))
								go func() {
									buf := make([]byte, 1024)
									// The mock shell only echoes input back, but it
									// simulates the one universal shell builtin the
									// echo fallback relies on: a line "echo __UT_x__"
									// prints the marker, which acknowledges completion.
									// Like a real shell, the marker is emitted after
									// the input echo, never before it.
									var line []byte
									var marker string
									for {
										n, err := ch.Read(buf)
										if n > 0 {
											counters.input.Add(int32(n))
											_, _ = ch.Write(buf[:n])
											for _, b := range buf[:n] {
												if b == '\n' || b == '\r' {
													if marker == "" {
														if tok, ok := echoMarkerToken(line); ok {
															marker = "\r\n__UT_" + tok + "__\r\n"
														}
													}
													line = line[:0]
												} else {
													line = append(line, b)
												}
											}
											if marker != "" {
												_, _ = ch.Write([]byte(marker))
												marker = ""
											}
										}
										if err != nil {
											return
										}
									}
								}()
							case "subsystem":
								counters.subsystems.Add(1)
								if subsystemMode == "hang" {
									continue
								}
								if subsystemMode == "deny" {
									req.Reply(false, nil)
									return
								}
								req.Reply(true, nil)
								server := sftp.NewRequestServer(ch, sftp.Handlers{FileGet: fs, FileList: fs})
								go func() { defer server.Close(); _ = server.Serve() }()
							case "exec":
								counters.execs.Add(1)
								if len(execHandlers) == 0 {
									req.Reply(false, nil)
									continue
								}
								var request struct{ Command string }
								if err := ssh.Unmarshal(req.Payload, &request); err != nil {
									req.Reply(false, nil)
									continue
								}
								req.Reply(true, nil)
								data, err := execHandlers[0](request.Command)
								_, _ = ch.Write(data)
								var status uint32
								if err != nil {
									status = 1
								}
								_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
								return
							default:
								req.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()
	return ln.Addr().String(), counters
}

// echoMarkerToken extracts the nonce from a typed "echo __UT_x__" line — the
// one builtin the mock shell "executes" to simulate any shell that can echo.
func echoMarkerToken(line []byte) (string, bool) {
	s := strings.TrimSpace(string(line))
	rest, ok := strings.CutPrefix(s, "echo __UT_")
	if !ok || !strings.HasSuffix(rest, "__") {
		return "", false
	}
	return rest[:len(rest)-2], true
}

func awaitCwd(t *testing.T, s *SSHSession, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if GetSessionCwd(s.id) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("cwd=%q, want %q", GetSessionCwd(s.id), want)
}

func TestSSHCwdOverSFTPAndClones(t *testing.T) {
	fs := newTestCwdFS()
	addr, counts := startCwdSSHServer(t, fs, "")
	source := NewSSHSession("cwd-source-" + t.Name())
	if err := source.Connect(probeTestConfig(addr)); err != nil {
		t.Fatal(err)
	}
	defer source.Disconnect()
	awaitCwd(t, source, "/home/terminal-101")
	if source.ForegroundShell() != "bash" {
		t.Fatalf("shell=%q", source.ForegroundShell())
	}
	// Discovery must not type even a single byte, nor open another transport.
	if counts.input.Load() != 0 || counts.execs.Load() != 0 || counts.connections.Load() != 1 {
		t.Fatal("cwd discovery used shell input, exec, or another transport")
	}
	source.SetCwdTracking(true)
	fs.setLink("/proc/101/cwd", "/tmp/中文 space")
	awaitCwd(t, source, "/tmp/中文 space")
	source.SetCwdTracking(false)
	fs.setLink("/proc/101/cwd", "/etc")
	time.Sleep(1100 * time.Millisecond)
	if GetSessionCwd(source.id) != "/tmp/中文 space" {
		t.Fatal("disabled tracker published cwd")
	}
	// Opening an SFTP tab with follow disabled still needs the current cwd.
	if cwd, err := source.CurrentCwd(); err != nil || cwd != "/etc" {
		t.Fatalf("on-demand cwd=%q err=%v", cwd, err)
	}
	if source.cwdTracking.Load() {
		t.Fatal("on-demand query enabled background polling")
	}

	clone := NewSSHChannelSession("cwd-clone-"+t.Name(), source)
	if err := clone.Connect(probeTestConfig(addr)); err != nil {
		t.Fatal(err)
	}
	defer clone.Disconnect()
	awaitCwd(t, clone, "/home/terminal-102")
	if counts.connections.Load() != 1 || counts.subsystems.Load() != 1 {
		t.Fatal("clone did not reuse the transport and probe channel")
	}
	source.Disconnect()
	clone.SetCwdTracking(true)
	fs.setLink("/proc/102/cwd", "/var")
	awaitCwd(t, clone, "/var")
	if err := clone.Write([]byte("still alive\r")); err != nil {
		t.Fatal(err)
	}
	if counts.execs.Load() != 0 {
		t.Fatal("exec request used for cwd tracking")
	}
}

func TestSSHCwdUnavailablePreservesTerminal(t *testing.T) {
	for _, mode := range []string{"deny", "hang", "no-proc"} {
		t.Run(mode, func(t *testing.T) {
			fs := newTestCwdFS()
			if mode == "no-proc" {
				delete(fs.files, "/proc/self/stat")
			}
			addr, counts := startCwdSSHServer(t, fs, mode)
			s := NewSSHSession("cwd-unavailable-" + mode)
			started := time.Now()
			if err := s.Connect(probeTestConfig(addr)); err != nil {
				t.Fatal(err)
			}
			defer s.Disconnect()
			if time.Since(started) > sshCwdTimeout+2*time.Second {
				t.Fatal("unbounded cwd probe")
			}
			if s.Status() != StatusConnected || counts.input.Load() != 0 || counts.execs.Load() != 0 {
				t.Fatal("unavailable probe affected terminal")
			}
			if err := s.Write([]byte("echo remains usable\r")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// ptyTestAdapter mirrors app_mcp.go's ptyAdapter: it narrows a session to the
// mcp.Pty surface, including the ForegroundShell() delegation that decides
// whether the completion acknowledgement is injected.
type ptyTestAdapter struct{ s *SSHSession }

func (p ptyTestAdapter) Write(data []byte) error { return p.s.Write(data) }
func (p ptyTestAdapter) OnData(cb func([]byte)) func() {
	return p.s.AddDataTap(cb)
}
func (p ptyTestAdapter) OnStatusChange(cb func(string)) func() {
	return p.s.AddStatusTap(func(st SessionStatus) { cb(string(st)) })
}
func (p ptyTestAdapter) Alive() bool { return p.s.Status() == StatusConnected }
func (p ptyTestAdapter) ForegroundShell() string {
	if shell, ok := any(p.s).(interface{ ForegroundShell() string }); ok {
		return shell.ForegroundShell()
	}
	return ""
}

// waitJobState polls a job until it leaves the running state.
func waitJobState(t *testing.T, j *mcp.Job, timeout time.Duration) (string, string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, _, _, state, _ := j.Snapshot()
		if state != mcp.StateRunning {
			return state, string(out)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("job never left running state")
	return "", ""
}

// Reproduction of the stuck-AI-execution report on hardened SSH hosts (CentOS
// 7 behind jump servers): the cwd/SFTP probe fails, shell detection returns
// empty. Previously that meant no completion acknowledgement at all — every
// execute_command ran to its full deadline reporting timeout even though the
// command had visibly finished, and kept the per-session busy lock until the
// TTL, so every later command failed with "session is busy".
//
// With the echo fallback, an undetected shell still gets an in-band
// acknowledgement (bare echo lines), so the result returns and the lock
// releases on completion.
func TestReproShellDetectFailStillReturnsResult(t *testing.T) {
	fs := newTestCwdFS()
	// "deny" = server rejects the sftp subsystem, like the hardened target.
	addr, counts := startCwdSSHServer(t, fs, "deny")
	s := NewSSHSession("repro1307-deny")
	if err := s.Connect(probeTestConfig(addr)); err != nil {
		t.Fatal(err)
	}
	defer s.Disconnect()

	if s.ForegroundShell() != "" {
		t.Fatalf("expected undetected shell, got %q", s.ForegroundShell())
	}

	reg := mcp.NewRegistry()
	pty := ptyTestAdapter{s}
	const hardLimit = 3 * time.Second

	// The first AI command completes through the echo acknowledgement.
	j, err := reg.StartWithScreen(pty, s.ID(), `echo hello && uname -a`, "\n", hardLimit, 64*1024, 120, 30)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	state, out := waitJobState(t, j, 10*time.Second)
	if state != mcp.StateExited {
		t.Fatalf("state=%q, want exited via echo acknowledgement", state)
	}
	// The typed command arrived complete — no truncation.
	if !strings.Contains(out, "echo hello && uname -a") {
		t.Fatalf("echo of typed command missing from output: %q", out)
	}
	if counts.input.Load() == 0 {
		t.Fatal("command never reached the shell")
	}

	// The lock is free on completion: the next AI command starts immediately.
	j2, err := reg.StartWithScreen(pty, s.ID(), `echo second`, "\n", hardLimit, 64*1024, 120, 30)
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	if state, _ := waitJobState(t, j2, 10*time.Second); state != mcp.StateExited {
		t.Fatalf("second job state=%q", state)
	}
}

// Takeover semantics over a real SSH session: when a command genuinely keeps
// running past the deadline, the watcher keeps the session lock until real
// completion — but a NEW command typed after the timeout supersedes the wedged
// job instead of failing busy, so a missed completion marker can never wedge
// the AI's terminal until the job TTL.
//
// The mock never executes the eval wrapper's printf, so the known-shell
// session plays the genuinely-still-running command here.
func TestTimedOutJobYieldsSessionToNewCommand(t *testing.T) {
	fs := newTestCwdFS()
	addr, _ := startCwdSSHServer(t, fs, "")
	s := NewSSHSession("repro1307-interrupt")
	if err := s.Connect(probeTestConfig(addr)); err != nil {
		t.Fatal(err)
	}
	defer s.Disconnect()

	reg := mcp.NewRegistry()
	pty := ptyTestAdapter{s}
	j, err := reg.Start(pty, s.ID(), `tail -f /var/log/messages`, "\n", 300*time.Millisecond, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	waitState := func(want string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, _, _, st, _ := j.Snapshot(); st == want {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("state never reached %q", want)
	}
	waitState(mcp.StateTimeout)

	// A command typed after the timeout supersedes the wedged job instead of
	// failing busy: the old job is finished as killed, an interrupt reaches
	// the terminal (killing tail -f for real), and the new command runs.
	next, err := reg.Start(pty, s.ID(), `echo after-timeout`, "\n", 2*time.Second, 64*1024)
	if err != nil {
		t.Fatalf("timed-out job did not yield the session: %v", err)
	}
	if _, _, _, st, jerr := j.Snapshot(); st != mcp.StateKilled || jerr != mcp.ErrSuperseded {
		t.Fatalf("old job state=%q err=%v, want killed/%v", st, jerr, mcp.ErrSuperseded)
	}
	// The mock shell never acknowledges commands, so the replacement job ends
	// in timeout like the original; the takeover itself is what's asserted.
	_ = reg.Interrupt(next.ID)
}

// Contrast: with the sftp probe available, shell detection succeeds and the
// known-shell path (eval + concealed printf with exit code) is unchanged.
func TestReproContrastShellDetectedInjectsSentinel(t *testing.T) {
	fs := newTestCwdFS()
	addr, _ := startCwdSSHServer(t, fs, "")
	s := NewSSHSession("repro1307-ok")
	if err := s.Connect(probeTestConfig(addr)); err != nil {
		t.Fatal(err)
	}
	defer s.Disconnect()

	if s.ForegroundShell() != "bash" {
		t.Fatalf("shell=%q, want bash", s.ForegroundShell())
	}

	reg := mcp.NewRegistry()
	pty := ptyTestAdapter{s}
	j, err := reg.StartWithScreen(pty, s.ID(), `echo hello && uname -a`, "\n", 1500*time.Millisecond, 64*1024, 120, 30)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	_, out := waitJobState(t, j, 10*time.Second)
	if !strings.Contains(out, "eval") || !strings.Contains(out, "__UT_") {
		t.Fatalf("sentinel wrapper missing from typed command: %q", out)
	}
	if strings.Contains(out, "echo __UT_") {
		t.Fatalf("known shell must keep the eval path, not the echo fallback: %q", out)
	}
}
