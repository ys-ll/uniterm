package session

// Regression tests for issue #983 (Xshell-style "duplicate channel"):
// NewSSHChannelSession must open a new session channel on the source
// session's authenticated client — no second TCP connection, no re-auth —
// and must keep the shared client alive after the source disconnects.

import (
	"bytes"
"fmt"
"io"
	"net"
"path/filepath"
"strconv"
	"strings"
"sync/atomic"
	"testing"
	"time"

"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// testHostKeyPEM is re-exported from ssh_dial_test.go in this package.
// startChannelTestServer accepts session channels and runs a trivial echo
// shell: whatever the client types is written back. Returns the listener's
// address plus counters for accepted TCP connections and opened channels.
func startChannelTestServer(t *testing.T) (addr string, tcpConns *int32, channels *int32) {
	t.Helper()
	signer, err := ssh.ParsePrivateKey([]byte(testHostKeyPEM))
	if err != nil {
		t.Fatalf("parse test host key: %v", err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, got []byte) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil // accept any password
		},
	}
	config.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	var connCount, chCount int32
	tcpConns, channels = &connCount, &chCount

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			connCount++
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for newCh := range chans {
					if newCh.ChannelType() != "session" {
						newCh.Reject(ssh.UnknownChannelType, "only session channels")
						continue
					}
					ch, chReqs, err := newCh.Accept()
					if err != nil {
						continue
					}
					chCount++
					go func(ch ssh.Channel, chReqs <-chan *ssh.Request) {
						defer ch.Close()
						for req := range chReqs {
							switch req.Type {
							case "shell", "pty-req":
								if req.WantReply {
									req.Reply(true, nil)
								}
							case "exec":
								if req.WantReply {
									req.Reply(true, nil)
								}
								// One-shot exec (cwd-hook shell detection): report
								// success and close the channel so the client's
								// Output returns immediately (Output waits for
								// the channel to close, not just exit-status)
								// instead of stalling until the client's
								// command timeout.
								_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
								ch.CloseWrite()
								ch.Close()
							default:
								if req.WantReply {
									req.Reply(false, nil)
								}
							}
						}
					}(ch, chReqs)
					// Echo loop: send back whatever arrives.
					go func(ch ssh.Channel) {
						buf := make([]byte, 4096)
						for {
							n, err := ch.Read(buf)
							if n > 0 {
								if _, werr := ch.Write(buf[:n]); werr != nil {
									return
								}
							}
							if err != nil {
								return
							}
						}
					}(ch)
				}
			}()
		}
	}()
	return ln.Addr().String(), tcpConns, channels
}

func dialTestSSHClient(t *testing.T, addr string) *ssh.Client {
	t.Helper()
	config := &ssh.ClientConfig{
		User:            "tester",
		Auth:            []ssh.AuthMethod{ssh.Password("pw")},
		Timeout:         5 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		t.Fatalf("dial test server: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// plainConfig returns a minimal connection config for attach. Each attach
// opens ONE session channel on the test server — the shell itself. The
// cwd-hook shell detection no longer runs on the session connection: the
// startup probe rides a separate short-lived TCP connection in Connect
// (issues #1031/#1057) and channel clones inherit the probe result.
func plainConfig() ConnectionConfig {
	return ConnectionConfig{User: "tester", Host: "testhost", AuthType: "password", Password: "pw"}
}

// connectTestSSHSession builds an SSHSession wired to a fresh client on the
// test server, completing Connect's channel attach (the dial step is
// bypassed by assigning the clientRef directly).
func connectTestSSHSession(t *testing.T, addr string) *SSHSession {
	t.Helper()
	client := dialTestSSHClient(t, addr)
	s := NewSSHSession("test-" + t.Name())
	s.mu.Lock()
	s.clientRef = newSSHClientRef(client)
	s.mu.Unlock()
	if err := s.attach(client, plainConfig()); err != nil {
		t.Fatalf("attach: %v", err)
	}
	return s
}

func TestSSHChannelCloneSharesConnection(t *testing.T) {
	addr, tcpConns, channels := startChannelTestServer(t)

	parent := connectTestSSHSession(t, addr)
	if parent.Status() != StatusConnected {
		t.Fatalf("parent status = %s, want connected", parent.Status())
	}

	clone := NewSSHChannelSession("clone-1", parent)
	if !clone.IsChannelClone() {
		t.Fatal("clone reports IsChannelClone() = false")
	}
	if clone.RemoteOS() != parent.RemoteOS() {
		t.Fatalf("clone remoteOS %q != parent %q", clone.RemoteOS(), parent.RemoteOS())
	}
	if err := clone.Connect(plainConfig()); err != nil {
		t.Fatalf("clone connect: %v", err)
	}
	defer clone.Disconnect()

	if clone.Status() != StatusConnected {
		t.Fatalf("clone status = %s, want connected", clone.Status())
	}

	// Core assertion (issue #983): one TCP connection, no re-auth. Each
	// attach opens one session channel (the shell); the shell probe runs on
	// its own connection, so parent and clone never add exec channels here.
	if got := *tcpConns; got != 1 {
		t.Fatalf("server accepted %d TCP connections, want 1 (clone must not re-dial)", got)
	}
	if got := *channels; got != 2 {
		t.Fatalf("server opened %d channels, want 2 (one shell per attach)", got)
	}
}

func TestSSHChannelCloneEcho(t *testing.T) {
	addr, _, _ := startChannelTestServer(t)
	parent := connectTestSSHSession(t, addr)
	defer parent.Disconnect()

	clone := NewSSHChannelSession("clone-2", parent)
	if err := clone.Connect(plainConfig()); err != nil {
		t.Fatalf("clone connect: %v", err)
	}

	var got bytes.Buffer
	clone.SetOnDataCallback(func(data []byte) { got.Write(data) })

	if err := clone.Write([]byte("ping")); err != nil {
		t.Fatalf("clone write: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(got.String(), "ping") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(got.String(), "ping") {
		t.Fatalf("clone did not echo input; got %q", got.String())
	}
	clone.Disconnect()
}

func TestSSHChannelCloneOutlivesParent(t *testing.T) {
	addr, _, _ := startChannelTestServer(t)
	parent := connectTestSSHSession(t, addr)

	clone := NewSSHChannelSession("clone-3", parent)
	if err := clone.Connect(plainConfig()); err != nil {
		t.Fatalf("clone connect: %v", err)
	}

	// Closing the source tab must not tear down the shared client.
	parent.Disconnect()
	if parent.Status() != StatusDisconnected {
		t.Fatalf("parent status = %s, want disconnected", parent.Status())
	}
	if clone.Status() != StatusConnected {
		t.Fatalf("clone status = %s after parent disconnect, want connected", clone.Status())
	}

	var got bytes.Buffer
	clone.SetOnDataCallback(func(data []byte) { got.Write(data) })
	if err := clone.Write([]byte("still-alive")); err != nil {
		t.Fatalf("clone write after parent disconnect: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(got.String(), "still-alive") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(got.String(), "still-alive") {
		t.Fatalf("clone not usable after parent disconnect; got %q", got.String())
	}

	// The last holder's disconnect closes the shared client.
	clone.Disconnect()
	if clone.Status() != StatusDisconnected {
		t.Fatalf("clone status = %s, want disconnected", clone.Status())
	}
}

func TestSSHChannelCloneWithoutSourceClient(t *testing.T) {
	parent := NewSSHSession("orphan-parent")
	clone := NewSSHChannelSession("clone-4", parent)
	err := clone.Connect(plainConfig())
	if err == nil {
		clone.Disconnect()
		t.Fatal("expected error when source session never connected")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("error %q does not mention the missing source connection", err.Error())
	}
}


// A rejected keyboard-interactive answer must print "Permission denied, please
// try again." before the next prompt (matching the OpenSSH client), so the
// user knows the previous answer failed instead of staring at a silent
// re-prompt while the server's fail delay runs (issue #949).
func TestKeyboardInteractiveChallengeDenialFeedback(t *testing.T) {
	s := NewSSHSession("auth-test")
	var out []byte
	s.SetOnDataCallback(func(d []byte) { out = append(out, d...) })
	s.mu.Lock()
	s.authAnswerCh = make(chan []byte, 4)
	s.mu.Unlock()

	var autoAnswered int32
	cb := s.keyboardInteractiveChallenge(ConnectionConfig{AuthType: "password", Password: "saved"}, &autoAnswered)

	// First challenge: the saved password is auto-submitted silently.
	answers, err := cb("", "", []string{"Password1: "}, []bool{false})
	if err != nil {
		t.Fatalf("auto-answer challenge failed: %v", err)
	}
	if len(answers) != 1 || answers[0] != "saved" {
		t.Fatalf("auto-answer = %v, want [saved]", answers)
	}
	if len(out) != 0 {
		t.Fatalf("auto-answer must not print anything, got %q", out)
	}

	// Second challenge: the saved password was rejected — the denial line
	// precedes the new prompt, and the typed answer is collected.
	go func() { s.authAnswerCh <- []byte("typed\r\n") }()
	answers, err = cb("", "", []string{"Password2: "}, []bool{false})
	if err != nil {
		t.Fatalf("interactive challenge failed: %v", err)
	}
	if len(answers) != 1 || answers[0] != "typed" {
		t.Fatalf("interactive answer = %v, want [typed]", answers)
	}
	if !strings.Contains(string(out), "Permission denied, please try again.") {
		t.Fatalf("re-challenge must print the denial line, got %q", out)
	}
	if !strings.Contains(string(out), "Password2: ") {
		t.Fatalf("re-challenge must print the prompt, got %q", out)
	}

	// A challenge in the same callback invocation is one rejection round: no
	// denial line between the questions of a single multi-prompt challenge.
	out = nil
	s.authAnswerCh <- []byte("a\r\n")
	s.authAnswerCh <- []byte("b\r\n")
	answers, err = cb("", "", []string{"OTP: ", "Password: "}, []bool{false, false})
	if err != nil {
		t.Fatalf("multi-prompt challenge failed: %v", err)
	}
	if len(answers) != 2 || answers[0] != "a" || answers[1] != "b" {
		t.Fatalf("multi-prompt answers = %v, want [a b]", answers)
	}
	if strings.Count(string(out), "Permission denied") != 1 {
		t.Fatalf("multi-prompt challenge must print the denial once, got %q", out)
	}
}


// probeConnActivity records the channel requests seen on one TCP connection.
type probeConnActivity struct {
	exec   int32
	shell  int32
	killed int32
}

// startProbeTrackingServer accepts SSH connections with password auth. When
// killOnExec is set it simulates a CLI-only appliance: an exec request tears
// down the whole transport (not just the channel). Otherwise exec is served
// one-shot like the channel test server. Every connection gets an activity
// record appended before the handshake.
func startProbeTrackingServer(t *testing.T, killOnExec bool) (addr string, conns chan *probeConnActivity) {
	t.Helper()
	signer, err := ssh.ParsePrivateKey([]byte(testHostKeyPEM))
	if err != nil {
		t.Fatalf("parse test host key: %v", err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, got []byte) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	config.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	conns = make(chan *probeConnActivity, 16)

	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			act := &probeConnActivity{}
			conns <- act
			go func(raw net.Conn, act *probeConnActivity) {
				_, chans, reqs, err := ssh.NewServerConn(raw, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for newCh := range chans {
					if newCh.ChannelType() != "session" {
						newCh.Reject(ssh.UnknownChannelType, "only session channels")
						continue
					}
					ch, chReqs, err := newCh.Accept()
					if err != nil {
						continue
					}
					go func(ch ssh.Channel, chReqs <-chan *ssh.Request) {
						defer ch.Close()
						for req := range chReqs {
							switch req.Type {
							case "shell", "pty-req":
								atomic.AddInt32(&act.shell, 1)
								if req.WantReply {
									req.Reply(true, nil)
								}
							case "exec":
								atomic.AddInt32(&act.exec, 1)
								if killOnExec {
									// Appliance behavior: the exec request kills
									// the whole transport, not just the channel.
									atomic.AddInt32(&act.killed, 1)
									raw.Close()
									return
								}
								if req.WantReply {
									req.Reply(true, nil)
								}
								_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
								ch.CloseWrite()
								ch.Close()
							default:
								if req.WantReply {
									req.Reply(false, nil)
								}
							}
						}
					}(ch, chReqs)
				}
			}(raw, act)
		}
	}()
	return ln.Addr().String(), conns
}

func probeTestConfig(addr string) ConnectionConfig {
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	return ConnectionConfig{
		User:     "admin",
		Host:     host,
		Port:     port,
		AuthType: "password",
		Password: "pw",
		Name:     "probe-test",
	}
}

// collectProbeConns drains the connection channel non-blockingly into a slice.
func collectProbeConns(conns chan *probeConnActivity) []*probeConnActivity {
	var out []*probeConnActivity
	for {
		select {
		case act := <-conns:
			out = append(out, act)
		default:
			return out
		}
	}
}

// waitForProbeConns polls until the server has recorded at least n
// connections or the deadline passes, accumulating whatever has arrived.
func waitForProbeConns(t *testing.T, conns chan *probeConnActivity, n int) []*probeConnActivity {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var got []*probeConnActivity
	for time.Now().Before(deadline) {
		got = append(got, collectProbeConns(conns)...)
		if len(got) >= n {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server accepted %d connections, want >= %d", len(got), n)
	return nil
}

func TestSSHConnectNeverExecutesCwdProbe(t *testing.T) {
	for _, killOnExec := range []bool{false, true} {
		t.Run(strconv.FormatBool(killOnExec), func(t *testing.T) {
			addr, conns := startProbeTrackingServer(t, killOnExec)
			s := NewSSHSession("no-cwd-exec-" + t.Name())
			if err := s.Connect(probeTestConfig(addr)); err != nil {
				t.Fatal(err)
			}
			defer s.Disconnect()
			if s.Status() != StatusConnected {
				t.Fatalf("status = %s", s.Status())
			}
			acts := waitForProbeConns(t, conns, 1)
			if len(acts) != 1 {
				t.Fatalf("got %d transports, want one", len(acts))
			}
			if atomic.LoadInt32(&acts[0].shell) != 2 || atomic.LoadInt32(&acts[0].exec) != 0 {
				t.Fatal("expected pty-req + shell without exec")
			}
		})
	}
}


// Require an interactive password and OTP. A companion cannot authenticate
// again from the source config, which deliberately contains no credentials.
func fileTestSSHSource(t *testing.T, rejectSFTP bool) (*SSHSession, *atomic.Int32) {
	t.Helper()
	var authentications atomic.Int32
	config := &ssh.ServerConfig{KeyboardInteractiveCallback: func(_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		authentications.Add(1)
		answers, err := challenge("", "", []string{"Password:", "OTP:"}, []bool{false, false})
		if err != nil {
			return nil, err
		}
		if len(answers) != 2 || answers[0] != "password" || answers[1] != "123456" {
			return nil, fmt.Errorf("invalid answers")
		}
		return nil, nil
	}}
	signer, err := ssh.ParsePrivateKey([]byte(testHostKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					channel, requests, err := incoming.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer channel.Close()
						for request := range requests {
							switch request.Type {
							case "subsystem":
								if rejectSFTP {
									request.Reply(false, nil)
									continue
								}
								request.Reply(true, nil)
								server, err := sftp.NewServer(channel)
								if err != nil {
									return
								}
								defer server.Close()
								_ = server.Serve()
								return
							case "exec":
								request.Reply(true, nil)
								_, _ = io.WriteString(channel, "/\n")
								_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
								return
							default:
								request.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		User: "tester", Timeout: 5 * time.Second, HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth: []ssh.AuthMethod{ssh.KeyboardInteractive(func(string, string, []string, []bool) ([]string, error) {
			return []string{"password", "123456"}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := NewSSHSession("interactive-source")
	source.clientRef = newSSHClientRef(client)
	source.setStatus(StatusConnected)
	t.Cleanup(func() { source.Disconnect() })
	return source, &authentications
}

func TestSFTPConnectSSHInteractiveTransfer(t *testing.T) {
	source, authentications := fileTestSSHSource(t, false)
	files := NewSFTPSession("files")
	if err := files.ConnectSSH(source); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { files.Disconnect() })
	remote := filepath.Join(t.TempDir(), "transfer.txt")
	if err := files.PutContent(remote, []byte("uploaded over authenticated SSH")); err != nil {
		t.Fatal(err)
	}
	content, err := files.GetContent(remote)
	if err != nil || string(content) != "uploaded over authenticated SSH" {
		t.Fatalf("download = %q, %v", content, err)
	}
	if authentications.Load() != 1 {
		t.Fatalf("authenticated %d times", authentications.Load())
	}
	files.Disconnect()
	files.Disconnect()
	if refs := source.clientRef.refs.Load(); refs != 1 {
		t.Fatalf("remaining refs = %d, want 1", refs)
	}
	assertFileTestSSHAlive(t, source)
	source.Disconnect()
	if refs := source.clientRef.refs.Load(); refs != 0 {
		t.Fatalf("remaining refs after final disconnect = %d, want 0", refs)
	}
	if channel, err := source.clientRef.client.NewSession(); err == nil {
		channel.Close()
		t.Fatal("transport survived final disconnect")
	}
}

func TestSFTPConnectSSHFailurePreservesTerminal(t *testing.T) {
	source, _ := fileTestSSHSource(t, true)
	files := NewSFTPSession("unsupported-sftp")
	if err := files.ConnectSSH(source); err == nil {
		t.Fatal("expected subsystem failure")
	}
	files.Disconnect()
	if refs := source.clientRef.refs.Load(); refs != 1 {
		t.Fatalf("remaining refs = %d, want 1", refs)
	}
	assertFileTestSSHAlive(t, source)
}

func TestSCPConnectSSHInteractive(t *testing.T) {
	source, authentications := fileTestSSHSource(t, false)
	files := NewSCPSession("scp-files")
	if err := files.ConnectSSH(source); err != nil {
		t.Fatal(err)
	}
	if _, err := files.runCommand("pwd", time.Second); err != nil {
		t.Fatal(err)
	}
	if authentications.Load() != 1 {
		t.Fatalf("authenticated %d times", authentications.Load())
	}
	files.Disconnect()
	files.Disconnect()
	if refs := source.clientRef.refs.Load(); refs != 1 {
		t.Fatalf("remaining refs = %d, want 1", refs)
	}
	assertFileTestSSHAlive(t, source)
}

func TestFileConnectSSHRequiresConnectedSource(t *testing.T) {
	source := NewSSHSession("disconnected")
	if err := NewSFTPSession("sftp").ConnectSSH(source); err == nil {
		t.Fatal("SFTP accepted disconnected source")
	}
	if err := NewSCPSession("scp").ConnectSSH(source); err == nil {
		t.Fatal("SCP accepted disconnected source")
	}
}

func assertFileTestSSHAlive(t *testing.T, source *SSHSession) {
	t.Helper()
	channel, err := source.clientRef.client.NewSession()
	if err != nil {
		t.Fatalf("terminal transport closed: %v", err)
	}
	defer channel.Close()
	if _, err := channel.Output("pwd"); err != nil {
		t.Fatalf("terminal exec: %v", err)
	}
}


func TestZmodemModeTimesOut(t *testing.T) {
	oldTimeout := zmodemModeTimeout
	zmodemModeTimeout = 10 * time.Millisecond
	t.Cleanup(func() { zmodemModeTimeout = oldTimeout })

	s := &baseSession{}
	s.SetZmodemMode(true)
	if !s.IsZmodemMode() {
		t.Fatal("zmodem mode was not enabled")
	}
	time.Sleep(30 * time.Millisecond)
	if s.IsZmodemMode() {
		t.Fatal("zmodem mode did not time out")
	}
}

func TestZmodemBinaryActivityExtendsTimeout(t *testing.T) {
	oldTimeout := zmodemModeTimeout
	zmodemModeTimeout = 30 * time.Millisecond
	t.Cleanup(func() { zmodemModeTimeout = oldTimeout })

	s := &baseSession{}
	s.SetZmodemMode(true)
	time.Sleep(20 * time.Millisecond)
	s.emitBinary([]byte{1})
	time.Sleep(20 * time.Millisecond)
	if !s.IsZmodemMode() {
		t.Fatal("binary activity did not extend zmodem timeout")
	}
	time.Sleep(20 * time.Millisecond)
	if s.IsZmodemMode() {
		t.Fatal("zmodem mode remained enabled after activity stopped")
	}
}

func TestEndingZmodemModeStopsTimeout(t *testing.T) {
	oldTimeout := zmodemModeTimeout
	zmodemModeTimeout = 10 * time.Millisecond
	t.Cleanup(func() { zmodemModeTimeout = oldTimeout })

	s := &baseSession{}
	s.SetZmodemMode(true)
	s.SetZmodemMode(false)
	time.Sleep(20 * time.Millisecond)
	if s.IsZmodemMode() {
		t.Fatal("zmodem mode was re-enabled by a stale timer")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.zmodemTimer != nil {
		t.Fatal("zmodem timer was not cleared")
	}
}

func TestOldZmodemTimerCannotDisableNewGeneration(t *testing.T) {
	oldTimeout := zmodemModeTimeout
	zmodemModeTimeout = 20 * time.Millisecond
	t.Cleanup(func() { zmodemModeTimeout = oldTimeout })

	s := &baseSession{}
	s.SetZmodemMode(true)
	time.Sleep(15 * time.Millisecond)
	s.SetZmodemMode(false)
	s.SetZmodemMode(true)
	time.Sleep(10 * time.Millisecond)
	if !s.IsZmodemMode() {
		t.Fatal("stale timer disabled a newer zmodem generation")
	}
	s.SetZmodemMode(false)
}

func TestEndZmodemRestoresAndLogsTrailingOutput(t *testing.T) {
	var logged []byte
	var emitted []byte
	s := &baseSession{outputLogWriter: func(data []byte) {
		logged = append(logged, data...)
	}, onDataCallback: func(data []byte) { emitted = append(emitted, data...) }}
	s.SetZmodemMode(true)

	trailing := []byte("root@host:~# ")
	s.EndZmodem(trailing)

	if s.IsZmodemMode() {
		t.Fatal("EndZmodem did not leave binary mode")
	}
	if !bytes.Equal(logged, trailing) {
		t.Fatalf("logged output = %q, want %q", logged, trailing)
	}
	if !bytes.Equal(emitted, trailing) {
		t.Fatalf("emitted output = %q, want %q", emitted, trailing)
	}
}

func TestSSHEndZmodemDecodesTrailingOutput(t *testing.T) {
	s := NewSSHSession("test")
	s.SetEncoding("gbk")
	s.SetZmodemMode(true)

	// GBK for "你好# ". This verifies trailing prompt bytes return through
	// the same configured decoder rather than being written raw to xterm.
	var got []byte
	s.SetOnDataCallback(func(data []byte) { got = append(got, data...) })
	s.EndZmodem([]byte{0xc4, 0xe3, 0xba, 0xc3, '#', ' '})
	if string(got) != "你好# " {
		t.Fatalf("emitted output = %q, want %q", got, "你好# ")
	}
	if s.IsZmodemMode() {
		t.Fatal("EndZmodem did not leave binary mode")
	}
}
