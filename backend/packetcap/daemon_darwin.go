//go:build darwin

package packetcap

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// darwinDaemonArgPrefix is the argv marker that selects the resident elevated
// capture daemon mode on macOS. It mirrors the Linux daemon design: keep ONE
// elevated helper alive for the whole app lifetime so the administrator
// password is only prompted oncechl. The helper owns /dev/bpf (the raw capture
// device) and runs the capture loop; the parent (normal-user GUI) sends it
// start/stop commands over stdin and reads newline-delimited JSON events
// (packets / stopped) from stdout. Reusing the daemon across captures means
// Start/Stop never re-elevate.
//
// macOS's `osascript ... with administrator privileges` does NOT cache
// credentials across separate invocations, so without a persistent daemon every
// capture would re-prompt. The daemon is what gives "提权后，软件关闭之前一直
// 拥有权限".
const darwinDaemonArgPrefix = "--packetcap-darwin-daemon"

// darwinDaemonHello is the first line the daemon writes once its stdin/stdout
// connection is live; it doubles as the connect synchronization point.
const darwinDaemonHello = "packetcap:darwin-daemon"

// darwinDaemonCmd is the command the parent sends over the daemon's stdin.
type darwinDaemonCmd struct {
	Cmd    string `json:"cmd"`    // "start" | "stop"
	ID     string `json:"id"`     // capture id (start only)
	Iface  string `json:"iface"`  // interface IP (start only)
	Filter string `json:"filter"` // filter (start only)
}

// darwinDaemonEvent is the event the daemon emits over its stdout.
type darwinDaemonEvent struct {
	Event  string  `json:"event"`            // "packet" | "stopped"
	ID     string  `json:"id"`               // capture id
	Packet *Packet `json:"packet,omitempty"` // present for "packet"
	Reason string  `json:"reason,omitempty"` // "stopped" | "error" | "limit"
	Count  int     `json:"count,omitempty"`  // packets captured (stopped)
}

// ---------------------------------------------------------------------------
// Worker (daemon) side — runs inside the elevated process.
// ---------------------------------------------------------------------------

// serverDarwinDaemon returns true when this process was re-launched as the
// resident elevated capture daemon, and parks in runDarwinDaemon (no GUI).
func serverDarwinDaemon(args []string) bool {
	for _, a := range args {
		if a == darwinDaemonArgPrefix {
			if err := runDarwinDaemon(); err != nil {
				os.Stderr.WriteString("packetcap daemon: " + err.Error() + "\n")
			}
			return true
		}
	}
	return false
}

type darwinDaemonSession struct {
	mu     sync.Mutex
	id     string
	cancel context.CancelFunc
}

// runDarwinDaemon is the elevated daemon's main loop. It reads commands from
// stdin until EOF — the parent closing stdin (process exit) makes EOF, which
// exits the daemon and cleans up the root process. It writes events to stdout.
func runDarwinDaemon() error {
	w := bufio.NewWriter(os.Stdout)
	fmt.Fprintln(w, darwinDaemonHello)
	if err := w.Flush(); err != nil {
		return err
	}

	var daemonMu sync.Mutex
	var cur *darwinDaemonSession

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var cmd darwinDaemonCmd
		if err := json.Unmarshal(sc.Bytes(), &cmd); err != nil {
			continue
		}
		enc := json.NewEncoder(w)
		switch cmd.Cmd {
		case "start":
			daemonMu.Lock()
			if cur != nil { // one capture at a time
				daemonMu.Unlock()
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			sess := &darwinDaemonSession{id: cmd.ID, cancel: cancel}
			cur = sess
			daemonMu.Unlock()

			cfg := CaptureConfig{InterfaceIP: cmd.Iface, Filter: cmd.Filter, MaxPackets: 3000}
			go func(s *darwinDaemonSession) {
				count := 0
				reason := "stopped"
				err := platformCapture(ctx, cfg, func(frame []byte) {
					p := ParseFrame(frame, time.Now())
					if p == nil {
						return
					}
					count++
					_ = enc.Encode(darwinDaemonEvent{Event: "packet", ID: s.id, Packet: p})
					_ = w.Flush()
				})
				if err != nil && err != errStopped {
					reason = "error"
				}
				_ = enc.Encode(darwinDaemonEvent{Event: "stopped", ID: s.id, Reason: reason, Count: count})
				_ = w.Flush()

				daemonMu.Lock()
				if cur == s {
					cur = nil
				}
				daemonMu.Unlock()
			}(sess)
		case "stop":
			daemonMu.Lock()
			if cur != nil {
				cur.cancel()
			}
			daemonMu.Unlock()
		}
	}
	return sc.Err()
}

// ---------------------------------------------------------------------------
// Parent side — the resident daemon singleton and the capture bridge.
// ---------------------------------------------------------------------------

var darwinDaemonSingleton struct {
	mu sync.Mutex
	d  *darwinDaemon
}

// darwinDaemon is the parent-side handle to the resident elevated daemon.
type darwinDaemon struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.Reader

	active atomic.Pointer[darwinActiveCapture]
}

type darwinActiveCapture struct {
	id       string
	onPacket func(*Packet)
	stopCh   chan struct{} // closed when the daemon reports "stopped" for this session
}

// getDarwinDaemon lazily starts (one-time administrator authentication) and
// returns the resident daemon.
func getDarwinDaemon() (*darwinDaemon, error) {
	darwinDaemonSingleton.mu.Lock()
	defer darwinDaemonSingleton.mu.Unlock()
	if darwinDaemonSingleton.d != nil {
		return darwinDaemonSingleton.d, nil
	}
	d, err := startDarwinDaemon()
	if err != nil {
		return nil, err
	}
	darwinDaemonSingleton.d = d
	return d, nil
}

// startDarwinDaemon spawns the daemon elevated (via osascript with
// administrator privileges, one prompt), synchronizes its hello, and installs
// the stdout reader goroutine.
func startDarwinDaemon() (*darwinDaemon, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}
	cmd, err := startDarwinElevated(exe, []string{darwinDaemonArgPrefix})
	if err != nil {
		return nil, err
	}
	// Both pipes must be retrieved BEFORE Start (os/exec requirement).
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start elevated daemon: %w", err)
	}

	d := &darwinDaemon{cmd: cmd, stdin: stdin, stdout: stdout}

	// Sync on the hello with a bounded timeout; the pipe stays open afterwards.
	if err := darwinAwaitHello(stdout, darwinDaemonHello, 30*time.Second); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("daemon handshake: %w", err)
	}
	go d.readerLoop()

	// If the daemon dies, cancel the active session so a pending capture ends
	// instead of blocking forever.
	go func() {
		_ = cmd.Wait()
		sess := d.active.Load()
		if sess != nil {
			close(sess.stopCh)
		}
	}()
	return d, nil
}

// readerLoop drains the daemon's stdout and routes events to the active capture.
func (d *darwinDaemon) readerLoop() {
	sc := bufio.NewScanner(d.stdout)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line == darwinDaemonHello {
			continue
		}
		var ev darwinDaemonEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		a := d.active.Load()
		switch ev.Event {
		case "packet":
			if a != nil && a.id == ev.ID && ev.Packet != nil {
				a.onPacket(ev.Packet)
			}
		case "stopped":
			if a != nil && a.id == ev.ID {
				if d.active.CompareAndSwap(a, nil) {
					close(a.stopCh)
				}
			}
		}
	}
}

// darwinDaemonCapture is the non-root macOS capture path. It reuses the single
// resident daemon (no re-elevation), requests a capture, streams packets to
// onPacket, and returns when the user stops or the capture ends.
func darwinDaemonCapture(ctx context.Context, cfg CaptureConfig, onPacket func(*Packet)) error {
	d, err := getDarwinDaemon()
	if err != nil {
		return err
	}
	if d.active.Load() != nil {
		return errors.New("已有抓包正在进行，请先停止再开始")
	}
	id := fmt.Sprintf("cap-%d", time.Now().UnixNano())
	a := &darwinActiveCapture{id: id, onPacket: onPacket, stopCh: make(chan struct{})}
	d.active.Store(a)

	enc := json.NewEncoder(d.stdin)
	if err := enc.Encode(darwinDaemonCmd{Cmd: "start", ID: id, Iface: cfg.InterfaceIP, Filter: cfg.Filter}); err != nil {
		d.active.CompareAndSwap(a, nil)
		return fmt.Errorf("启动抓包失败: %w", err)
	}

	// User Stop -> ask the daemon to stop the current capture.
	stopCh := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(stopCh)
	}()

	select {
	case <-a.stopCh:
		// capture ended on its own (limit / error / daemon acked an earlier Stop)
	case <-stopCh:
		// user stopped: tell the daemon to release the raw device, then wait
		// for the "stopped" ack so the daemon returns to the idle loop.
		_ = enc.Encode(darwinDaemonCmd{Cmd: "stop"})
		<-a.stopCh
	}
	return errStopped
}

// startDarwinElevated launches the daemon as an administrator using osascript's
// "do shell script ... with administrator privileges". This pops the macOS
// authentication dialog once; because the spawned helper stays resident, the
// privilege is held until the app closes (no per-capture prompts).
func startDarwinElevated(exe string, args []string) (*exec.Cmd, error) {
	var b strings.Builder
	b.WriteString("do shell script \"")
	b.WriteString(escapeAppleScript(exe))
	b.WriteString(" ")
	for _, a := range args {
		b.WriteString(escapeAppleScript(a))
		b.WriteString(" ")
	}
	b.WriteString("\" with administrator privileges")
	osascript, err := exec.LookPath("osascript")
	if err != nil {
		return nil, errors.New("未找到 osascript：无法以管理员权限启动抓包助手")
	}
	return exec.Command(osascript, "-e", b.String()), nil
}

// escapeAppleScript wraps a string in double quotes, escaping embedded quotes
// and backslashes so arbitrary executable paths / args survive AppleScript.
func escapeAppleScript(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"")
	return "\"" + r.Replace(s) + "\""
}

// darwinAwaitHello reads the first line from r expecting want, bounded by a
// timeout and the process's liveness.
func darwinAwaitHello(r io.Reader, want string, timeout time.Duration) error {
	type res struct {
		line string
		err  error
	}
	result := make(chan res, 1)
	go func() {
		sc := bufio.NewReaderSize(r, 64*1024)
		line, err := sc.ReadString('\n')
		result <- res{line: line, err: err}
	}()
	select {
	case r := <-result:
		if strings.TrimSpace(r.line) == want {
			return nil
		}
		if r.err != nil {
			return r.err
		}
		return fmt.Errorf("unexpected hello %q", strings.TrimSpace(r.line))
	case <-time.After(timeout):
		return errors.New("timed out waiting for elevated daemon")
	}
}
