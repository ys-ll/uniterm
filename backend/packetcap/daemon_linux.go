//go:build linux

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

// linuxDaemonArgPrefix is the argv marker that selects the resident elevated
// capture daemon mode on Linux. Unlike the Windows per-capture worker, Linux
// keeps ONE elevated daemon alive for the whole app lifetime, so the
// pkexec/sudo password is prompted only once: the daemon itself owns the raw
// socket and runs the AF_PACKET loop; the parent (normal-user GUI) sends it
// start/stop commands over stdin and reads newline-delimited JSON events
// (packets / stopped) from stdout. Reusing the daemon across captures means
// Start/Stop never re-elevate.
const linuxDaemonArgPrefix = "--packetcap-linux-daemon"

// linuxDaemonHello is the first line the daemon writes once its stdin/stdout
// connection is live; it doubles as the connect synchronization point.
const linuxDaemonHello = "packetcap:linux-daemon"

// daemon command the parent sends over the daemon's stdin. One capture runs at
// a time (a raw socket is held open for it).
type linuxDaemonCmd struct {
	Cmd      string `json:"cmd"`      // "start" | "stop"
	ID       string `json:"id"`       // capture id (start only)
	Iface    string `json:"iface"`    // interface IP (start only)
	IfaceName string `json:"ifaceName"` // interface name (start only)
	Filter   string `json:"filter"`   // filter (start only)
}

// daemon event the daemon emits over its stdout.
type linuxDaemonEvent struct {
	Event  string  `json:"event"`            // "packet" | "stopped"
	ID     string  `json:"id"`               // capture id
	Packet *Packet `json:"packet,omitempty"` // present for "packet"
	Reason string  `json:"reason,omitempty"` // "stopped" | "error" | "limit"
	Count  int     `json:"count,omitempty"`  // packets captured (stopped)
}

// ---------------------------------------------------------------------------
// Worker (daemon) side — runs inside the elevated process.
// ---------------------------------------------------------------------------

// serverLinuxDaemon returns true when this process was re-launched as the
// resident elevated capture daemon, and parks in runLinuxDaemon (no GUI).
func serverLinuxDaemon(args []string) bool {
	for _, a := range args {
		if a == linuxDaemonArgPrefix {
			if err := runLinuxDaemon(); err != nil {
				os.Stderr.WriteString("packetcap daemon: " + err.Error() + "\n")
			}
			return true
		}
	}
	return false
}

type linuxDaemonSession struct {
	mu     sync.Mutex
	id     string
	cancel context.CancelFunc
}

// runLinuxDaemon is the elevated daemon's main loop. It reads commands from
// stdin until EOF — the parent closing stdin (process exit) makes EOF, which
// exits the daemon and cleans up the root process. It writes events to stdout.
func runLinuxDaemon() error {
	w := bufio.NewWriter(os.Stdout)
	fmt.Fprintln(w, linuxDaemonHello)
	if err := w.Flush(); err != nil {
		return err
	}

	var daemonMu sync.Mutex
	var cur *linuxDaemonSession

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var cmd linuxDaemonCmd
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
			sess := &linuxDaemonSession{id: cmd.ID, cancel: cancel}
			cur = sess
			daemonMu.Unlock()

			cfg := CaptureConfig{InterfaceName: cmd.IfaceName, InterfaceIP: cmd.Iface, Filter: cmd.Filter, MaxPackets: 3000}
			go func(s *linuxDaemonSession) {
				count := 0
				reason := "stopped"
				// 与 worker_runWorker 同理：逐包 flush 会在 AF_PACKET 大流量下把
				// 管道写爆、拖垮父进程事件通道。改为每 50ms / 每 40 包批量 flush。
				flushedAt := time.Now()
				nSinceFlush := 0
				flushDaemon := func() {
					if err := w.Flush(); err != nil {
						cancel()
					}
				}
				err := platformCapture(ctx, cfg, func(frame []byte) {
					// 兜底：任何解码阶段（ParseFrame 内部）panic 都不允许摧毁 daemon 进程，
					// 否则一旦遇到畸形帧整个抓包会话被杀、且常驻 daemon 不再重建，
					// 表现为「Linux 抓不到包 / 抓几个就断」。
					defer func() {
						if r := recover(); r != nil {
							msg := fmt.Sprintf("packetcap daemon: 解码帧时恢复 panic: %v\n", r)
							_, _ = os.Stderr.WriteString(msg)
						}
					}()
					p := ParseFrame(frame, time.Now())
					if p == nil {
						return
					}
					count++
					if err := enc.Encode(linuxDaemonEvent{Event: "packet", ID: s.id, Packet: p}); err != nil {
						cancel()
						return
					}
					nSinceFlush++
					if nSinceFlush >= 40 || time.Since(flushedAt) >= 50*time.Millisecond {
						nSinceFlush = 0
						flushedAt = time.Now()
						flushDaemon()
					}
				})
				// 抓包结束（用户停止/出错/达上限）：把缓冲里剩余字节推给父进程，
				// 再发送 stopped，保证计数与末尾包不丢。
				flushDaemon()
				if err != nil && err != errStopped {
					reason = "error"
				}
				_ = enc.Encode(linuxDaemonEvent{Event: "stopped", ID: s.id, Reason: reason, Count: count})
				flushDaemon()

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

var daemonSingleton struct {
	mu sync.Mutex
	d  *linuxDaemon
}

// linuxDaemon is the parent-side handle to the resident elevated daemon.
type linuxDaemon struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.Reader

	active atomic.Pointer[activeCapture]
	dead   atomic.Bool // true once the daemon process has exited
}

type activeCapture struct {
	id       string
	onPacket func(*Packet)
	stopCh   chan struct{} // closed when the daemon reports "stopped" for this session
}

// getDaemon lazily starts (one-time pkexec/sudo authentication) and returns the
// resident daemon.
func getDaemon() (*linuxDaemon, error) {
	daemonSingleton.mu.Lock()
	defer daemonSingleton.mu.Unlock()
	// 若常驻 daemon 已死（例如过去解码 panic 杀掉了进程），必须重建，
	// 否则后续所有抓包都会发到死进程、永远等不到 "stopped"，表现为
	// 「Linux 一直抓不到包」。dead 由 startLinuxDaemon 的 Wait goroutine 置位。
	if daemonSingleton.d != nil && !daemonSingleton.d.dead.Load() {
		return daemonSingleton.d, nil
	}
	if daemonSingleton.d != nil {
		daemonSingleton.d = nil
	}
	d, err := startLinuxDaemon()
	if err != nil {
		return nil, err
	}
	daemonSingleton.d = d
	return d, nil
}

// startLinuxDaemon spawns the daemon elevated (pkexec preferred, sudo fallback),
// synchronizes its hello, and installs the stdout reader goroutine.
func startLinuxDaemon() (*linuxDaemon, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}
	cmd, err := startElevated(exe, []string{linuxDaemonArgPrefix})
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

	d := &linuxDaemon{cmd: cmd, stdin: stdin, stdout: stdout}

	// Sync on the hello with a bounded timeout; the pipe stays open afterwards.
	if err := awaitTaskHello(stdout, linuxDaemonHello, 30*time.Second); err != nil {
		killCommand(cmd)
		return nil, fmt.Errorf("daemon handshake: %w", err)
	}
	go d.readerLoop()

	// If the daemon dies, cancel the active session so a pending capture ends
	// instead of blocking forever, and flag it dead so getDaemon rebuilds it.
	go func() {
		_ = cmd.Wait()
		d.dead.Store(true)
		sess := d.active.Load()
		if sess != nil {
			close(sess.stopCh)
		}
	}()
	return d, nil
}

// readerLoop drains the daemon's stdout and routes events to the active capture.
func (d *linuxDaemon) readerLoop() {
	sc := bufio.NewScanner(d.stdout)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line == linuxDaemonHello {
			continue
		}
		var ev linuxDaemonEvent
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

// linuxDaemonCapture is the non-root Linux capture path. It reuses the single
// resident daemon (no re-elevation), requests a capture, streams packets to
// onPacket, and returns when the user stops or the capture ends.
func linuxDaemonCapture(ctx context.Context, cfg CaptureConfig, onPacket func(*Packet)) error {
	d, err := getDaemon()
	if err != nil {
		return err
	}
	if d.active.Load() != nil {
		return errors.New("已有抓包正在进行，请先停止再开始")
	}
	id := fmt.Sprintf("cap-%d", time.Now().UnixNano())
	a := &activeCapture{id: id, onPacket: onPacket, stopCh: make(chan struct{})}
	d.active.Store(a)

	enc := json.NewEncoder(d.stdin)
	if err := enc.Encode(linuxDaemonCmd{Cmd: "start", ID: id, Iface: cfg.InterfaceIP, IfaceName: cfg.InterfaceName, Filter: cfg.Filter}); err != nil {
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
		// capture ended on its own (limit / error / daemon acked an earlier Sto
	case <-stopCh:
		// user stopped: tell the daemon to release the raw socket, then wait
		// for the "stopped" ack so the daemon returns to the idle loop.
		_ = enc.Encode(linuxDaemonCmd{Cmd: "stop"})
		<-a.stopCh
	}
	return errStopped
}

// ---------------------------------------------------------------------------
// Helpers (shared with the old one-shot worker startup path).
// ---------------------------------------------------------------------------

// startElevated builds the elevated command. pkexec is preferred (native
// graphical auth dialog, no controlling-tty requirement); sudo is the fallback.
func startElevated(exe string, args []string) (*exec.Cmd, error) {
	if pkexecPath, err := exec.LookPath("pkexec"); err == nil {
		return exec.Command(pkexecPath, append([]string{exe}, args...)...), nil
	}
	if sudoPath, err := exec.LookPath("sudo"); err == nil {
		return exec.Command(sudoPath, append([]string{exe}, args...)...), nil
	}
	return nil, errors.New("未找到提权程序：请安装 pkexec（polkit）或 sudo")
}

// killCommand forcibly terminates a process that may be parked in a blocking
// raw-socket read.
func killCommand(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

// awaitTaskHello reads the first line from r expecting want, bounded by a
// timeoutcars and the process's liveness.
func awaitTaskHello(r io.Reader, want string, timeout time.Duration) error {
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
