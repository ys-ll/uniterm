//go:build !windows
// +build !windows

package session

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
	"github.com/ys-ll/uniterm/backend/platform"
)

// Mouse tracking escape sequences that terminal applications (e.g. opencode,
// vim, tmux) send to enable xterm mouse tracking. When an application exits
// without sending the corresponding disable sequences, the terminal is left
// in tracking mode and native text selection stops working. We detect these
// sequences in the output stream and automatically inject the reset when the
// user next presses Enter.
var (
	mouseTrackingEnableSeqs = [][]byte{
		[]byte("\x1b[?1000h"), // normal tracking
		[]byte("\x1b[?1002h"), // button-event tracking
		[]byte("\x1b[?1003h"), // any-event tracking
		[]byte("\x1b[?1004h"), // focus-event tracking
		[]byte("\x1b[?1005h"), // UTF-8 extended mode
		[]byte("\x1b[?1006h"), // SGR extended mode
		[]byte("\x1b[?1015h"), // urxvt extended mode
	}
	mouseTrackingDisableSeqs = [][]byte{
		[]byte("\x1b[?1000l"),
		[]byte("\x1b[?1002l"),
		[]byte("\x1b[?1003l"),
		[]byte("\x1b[?1004l"),
		[]byte("\x1b[?1005l"),
		[]byte("\x1b[?1006l"),
		[]byte("\x1b[?1015l"),
	}
	mouseTrackingReset = []byte("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1004l\x1b[?1005l\x1b[?1006l\x1b[?1015l")
)

// updateMouseTrackingState scans data for mouse tracking enable/disable
// sequences and updates the session's tracking flag accordingly.
//
// Fast-path: most chunks (60+ chunks/sec on a busy PTY) are pure text
// without any ESC (0x1b). Skip the 14 bytes.Contains scans when no
// ESC byte is present — saves ~120 ms CPU/sec on a 12% single-core.
func (s *LocalSession) updateMouseTrackingState(data []byte) {
	if bytes.IndexByte(data, 0x1b) < 0 {
		return
	}
	for _, seq := range mouseTrackingEnableSeqs {
		if bytes.Contains(data, seq) {
			s.mouseTrackingEnabled.Store(true)
			return
		}
	}
	for _, seq := range mouseTrackingDisableSeqs {
		if bytes.Contains(data, seq) {
			s.mouseTrackingEnabled.Store(false)
			return
		}
	}
}

type LocalSession struct {
	baseSession
	cmd      *exec.Cmd
	pty      *os.File
	quit     chan struct{}
	quitOnce sync.Once
	// waitDone is closed when the cmd.Wait goroutine has returned, so
	// Disconnect can join it (and avoid racing with the shell's own exit
	// handler while still applying a kill only when the child didn't exit
	// on its own).
	waitDone             chan struct{}
	mouseTrackingEnabled atomic.Bool
}

func NewLocalSession(id string) *LocalSession {
	s := &LocalSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "local",
			status:      StatusDisconnected,
		},
		quit: make(chan struct{}),
	}
	// Set a generous default size so the PTY is unlikely to scroll before
	// the frontend sends its first Resize() with the real dimensions.
	s.SetPendingSize(200, 60)
	return s
}

func (s *LocalSession) Connect(config ConnectionConfig) error {
	s.SetLogOnConnect(config.LogOnConnect)
	s.setStatus(StatusConnecting)

	shell := config.ShellPath
	if shell == "" {
		shell = defaultShell()
	}

	// Determine working directory: use config.Cwd if set, otherwise user home.
	workDir := config.Cwd
	if workDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			workDir = home
		}
	}

	s.title = shellName(shell)

	s.cmd = exec.Command(shell, loginShellArgs(shell)...)
	s.cmd.Env = ensureTerminalEnv(os.Environ())
	s.cmd.Dir = workDir

	ptyFile, err := pty.Start(s.cmd)
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("start pty: %w", err)
	}
	s.pty = ptyFile
	s.waitDone = make(chan struct{})

	// Do NOT call s.Disconnect() from this goroutine: Disconnect waits
	// on waitDone, but the defer above only fires after Disconnect
	// returns — a self-deadlock (see TestLocalSessionWaitGoroutineDoesNotCallDisconnect).
	go func() {
		defer close(s.waitDone)
		_ = s.cmd.Wait()
	}()

	s.setStatus(StatusConnected)

	if cols, rows := s.GetPendingSize(); cols > 0 && rows > 0 {
		_ = pty.Setsize(s.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	}

	go s.readLoop()
	go s.runPostLoginScript(config.PostLoginScript)

	return nil
}

func (s *LocalSession) readLoop() {
	// 16 KiB reused read buffer; emitData's callbacks copy the bytes, so
	// buf[:n] can be handed off without an extra append.
	buf := make([]byte, 16384)
	for {
		select {
		case <-s.quit:
			return
		default:
		}
		// Disconnect nils s.pty; a concurrent Disconnect can land here
		// between the select and the read load. Bail before dereferencing.
		if s.pty == nil {
			return
		}

		n, err := s.pty.Read(buf)
		if n > 0 {
			s.RecordReadActivity()
			data := buf[:n]
			s.emitData(data)
			s.updateMouseTrackingState(data)
		}
		if err != nil {
			// If the quit channel is already closed, another goroutine
			// (e.g. the Wait() goroutine) has already initiated disconnect;
			// the read error is a side-effect of the PTY being closed and
			// should be silently ignored.
			select {
			case <-s.quit:
				return
			default:
			}
			if err != io.EOF {
				s.emitData([]byte(fmt.Sprintf("\r\n[read error: %v]\r\n", err)))
			}
			s.Disconnect()
			return
		}
	}
}

func (s *LocalSession) Write(data []byte) error {
	if s.pty == nil {
		return fmt.Errorf("not connected")
	}
	_, err := s.pty.Write(data)
	// If a terminal application enabled mouse tracking and then exited
	// without disabling it, automatically reset tracking when the user
	// presses Enter — this restores native text selection.
	if err == nil && s.mouseTrackingEnabled.Load() {
		for _, b := range data {
			if b == '\r' || b == '\n' {
				s.emitData(mouseTrackingReset)
				s.mouseTrackingEnabled.Store(false)
				break
			}
		}
	}
	return err
}

func (s *LocalSession) Disconnect() error {
	s.quitOnce.Do(func() {
		close(s.quit)
	})
	// Closing the PTY first sends EOF on the child's stdin; well-behaved
	// shells exit on their own. Only if the child doesn't exit within a short
	// grace window do we escalate to a SIGKILL. Either way we join the Wait
	// goroutine via waitDone before returning, so callers don't see a
	// session marked down while the child is still being reaped.
	if s.pty != nil {
		s.pty.Close()
		// Nil the handle so post-Disconnect Write/Resize return
		// `not connected` (matching the Windows branch's s.cpty = nil)
		// instead of silently writing to a closed fd.
		s.pty = nil
	}
	if s.cmd != nil && s.cmd.Process != nil && s.waitDone != nil {
		select {
		case <-s.waitDone:
			// Child reaped itself; nothing more to do.
		case <-time.After(500 * time.Millisecond):
			_ = s.cmd.Process.Kill()
			<-s.waitDone
		}
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *LocalSession) Resize(cols, rows int) error {
	s.SetPendingSize(cols, rows)
	if s.pty == nil {
		return fmt.Errorf("session not connected")
	}
	return pty.Setsize(s.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *LocalSession) IsConnected() bool {
	return s.Status() == StatusConnected
}

func (s *LocalSession) runPostLoginScript(script string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-s.quit:
			cancel()
		case <-ctx.Done():
		}
	}()

	send := func(data []byte) {
		if s.pty != nil {
			s.pty.Write(data)
		}
	}
	s.baseSession.RunPostLoginScript(ctx, script, send, s.IsConnected)
}

func defaultShell() string { return platform.DefaultShell() }

func shellName(path string) string { return platform.ShellName(path) }

// loginShellArgs returns the arguments needed to start the shell as a login
// shell on macOS. See platform.LoginShellArgs for the rationale.
func loginShellArgs(shellPath string) []string {
	return platform.LoginShellArgs(shellPath)
}

// ensureTerminalEnv guarantees the local shell starts with a usable terminal
// environment: a UTF-8 locale and a valid TERM.
//
// When the app is launched from the macOS Finder/Dock (rather than a terminal),
// it inherits no LANG/LC_* and no TERM. The PTY shell then runs in the C/POSIX
// locale with an unknown terminal, which breaks zsh in two ways:
//   - C locale: zsh's line editor binds every 0x80-0xFF byte to self-insert and
//     cannot compose multibyte UTF-8 — garbled CJK/IME input.
//   - missing TERM: zsh's ZLE has no terminfo to redraw the line, so backspace
//     and other editing keys leave the display out of sync and appear broken.
//
// bash/sh tolerate both; zsh's ZLE does not. We inject each variable only when
// it is not already set so we never override the user's explicit configuration.
func ensureTerminalEnv(env []string) []string {
	cleaned := make([]string, 0, len(env)+3)
	hasLocale := false
	hasTerm := false
	for _, kv := range env {
		isLocaleKey := strings.HasPrefix(kv, "LC_ALL=") ||
			strings.HasPrefix(kv, "LC_CTYPE=") ||
			strings.HasPrefix(kv, "LANG=")
		if isLocaleKey {
			idx := strings.IndexByte(kv, '=')
			if idx >= 0 && kv[idx+1:] != "" {
				hasLocale = true
			} else {
				// Drop empty locale entries so they can't shadow the value
				// we inject below (libc may honor the first duplicate key).
				continue
			}
		}
		if strings.HasPrefix(kv, "TERM=") {
			if kv != "TERM=" {
				hasTerm = true
			} else {
				continue
			}
		}
		cleaned = append(cleaned, kv)
	}
	if !hasLocale {
		locale := preferredUTF8Locale()
		cleaned = append(cleaned, "LANG="+locale, "LC_CTYPE="+locale)
	}
	if !hasTerm {
		cleaned = append(cleaned, "TERM=xterm-256color")
	}
	return cleaned
}

// preferredUTF8Locale returns a UTF-8 locale to use for the local shell,
// preferring the user's system language when a matching UTF-8 locale exists and
// falling back to the universally available en_US.UTF-8.
func preferredUTF8Locale() string {
	const fallback = "en_US.UTF-8"
	if runtime.GOOS != "darwin" {
		return fallback
	}
	out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		return fallback
	}
	region := strings.TrimSpace(string(out))
	// AppleLocale may carry a script subtag (e.g. zh_Hans_CN); reduce to
	// language_region which is what UTF-8 locales are named after.
	parts := strings.Split(region, "_")
	if len(parts) >= 3 {
		region = parts[0] + "_" + parts[len(parts)-1]
	}
	if region == "" {
		return fallback
	}
	candidate := region + ".UTF-8"
	if localeAvailable(candidate) {
		return candidate
	}
	return fallback
}

// localeAvailable reports whether `locale -a` lists the given locale.
func localeAvailable(name string) bool {
	out, err := exec.Command("locale", "-a").Output()
	if err != nil {
		return false
	}
	target := strings.ToLower(strings.ReplaceAll(name, "-", ""))
	for _, line := range strings.Split(string(out), "\n") {
		if strings.ToLower(strings.ReplaceAll(strings.TrimSpace(line), "-", "")) == target {
			return true
		}
	}
	return false
}
