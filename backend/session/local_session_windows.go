//go:build windows
// +build windows

package session

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
)

const cpUTF8 = 65001

var (
	kernel32Dll        = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole  = kernel32Dll.NewProc("AttachConsole")
	procFreeConsoleWin = kernel32Dll.NewProc("FreeConsole")

	// AttachConsole/FreeConsole operate on process-wide state (a process can
	// only be attached to one console at a time), so concurrent calls from
	// multiple local sessions being opened at once must be serialized.
	consoleAttachMu sync.Mutex
)

// forceUTF8ConsoleCodePage attaches to the hidden conhost that ConPTY
// created for pid and forces its input/output code page to UTF-8 (65001),
// then detaches. Without this, the pseudo console inherits the system's
// default ANSI/OEM code page (e.g. GBK/936 on zh-CN Windows); MSYS2 shells
// like Git Bash write raw UTF-8 to their controlling console, which the
// console then reinterprets under that legacy code page before ConPTY
// re-serializes it as the VT stream uniterm reads, producing mojibake that
// does not occur in standalone Git Bash (which runs under mintty and never
// goes through the Win32 console subsystem). Windows Terminal and VS Code's
// integrated terminal apply the same fix. uniterm's own process is a GUI
// subsystem app with no console of its own, so AttachConsole/FreeConsole
// here only ever touches the child's console, never uniterm's.
func forceUTF8ConsoleCodePage(pid int) {
	consoleAttachMu.Lock()
	defer consoleAttachMu.Unlock()

	// The child's console may not be immediately attachable right after
	// CreateProcess returns; a few short retries comfortably cover that
	// without noticeably delaying session startup.
	for attempt := 0; attempt < 10; attempt++ {
		ret, _, _ := procAttachConsole.Call(uintptr(pid))
		if ret != 0 {
			_ = windows.SetConsoleCP(cpUTF8)
			_ = windows.SetConsoleOutputCP(cpUTF8)
			procFreeConsoleWin.Call()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

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
func (s *LocalSession) updateMouseTrackingState(data []byte) {
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
	cpty                 *conpty.ConPty
	stdin                io.WriteCloser
	stdout               io.Reader
	cmd                  *exec.Cmd
	quit                 chan struct{}
	disconnectOnce       sync.Once
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

	var commandLine string
	var cmd *exec.Cmd
	isMSYSBash := false

	if distro, ok := parseWSLPath(shell); ok {
		if distro == "" {
			s.setStatus(StatusError)
			return fmt.Errorf("empty WSL distribution name")
		}
		commandLine = wslCommandLine(distro)
		cmd = exec.Command("wsl.exe", "-d", distro)
		cmd.Env = os.Environ()
	} else {
		commandLine = buildCommandLine(shell)
		lowerShell := strings.ToLower(shell)
		if strings.Contains(lowerShell, "bash") {
			if strings.Contains(lowerShell, "system32") || strings.Contains(lowerShell, "wsl") {
				cmd = exec.Command(shell)
				cmd.Env = os.Environ()
			} else {
				cmd = exec.Command(shell, "--login", "-i")
				cmd.Env = append(os.Environ(), "TERM=xterm-256color")
				isMSYSBash = true
			}
		} else {
			cmd = exec.Command(shell)
			cmd.Env = os.Environ()
		}
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Dir = workDir

	// Try ConPTY first for a real pseudo-terminal experience.
	if conpty.IsConPtyAvailable() {
		cols, rows := s.GetPendingSize()
		if cols <= 0 || rows <= 0 {
			cols, rows = 80, 24
		}
		c, err := conpty.Start(commandLine, conpty.ConPtyDimensions(cols, rows), conpty.ConPtyWorkDir(workDir), conpty.ConPtyEnv(os.Environ()))
		if err == nil {
			s.cpty = c
			if isMSYSBash {
				go forceUTF8ConsoleCodePage(c.Pid())
			}
			go func() {
				_, _ = s.cpty.Wait(context.Background())
				s.Disconnect()
			}()
			s.setStatus(StatusConnected)
			go s.readLoop()
			go s.runPostLoginScript(config.PostLoginScript)
			return nil
		}
		// Fall through to pipe mode if ConPTY fails.
	}

	// Pipe fallback using cmd built above.
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("start command: %w", err)
	}

	s.stdin = stdinPipe
	s.stdout = stdoutPipe
	s.cmd = cmd

	go func() {
		_ = s.cmd.Wait()
		s.Disconnect()
	}()

	s.setStatus(StatusConnected)
	go s.readLoop()
	go s.runPostLoginScript(config.PostLoginScript)
	return nil
}

func parseWSLPath(path string) (distro string, ok bool) {
	const prefix = "wsl://"
	if !strings.HasPrefix(strings.ToLower(path), prefix) {
		return "", false
	}
	return path[len(prefix):], true
}

func wslCommandLine(distro string) string {
	// Note: do not quote the distro name here. In the ConPTY path, quoted
	// names are interpreted literally by wsl.exe and cause
	// WSL_E_DISTRO_NOT_FOUND. Distribution names with spaces are uncommon;
	// if needed, the pipe fallback (exec.Command with separate args) handles
	// them correctly.
	return fmt.Sprintf(`wsl.exe -d %s`, distro)
}

func buildCommandLine(shell string) string {
	lower := strings.ToLower(shell)
	quoted := fmt.Sprintf(`"%s"`, shell)

	if strings.Contains(lower, "bash") {
		// WSL bash (inside System32) does not support --login -i passed this way.
		if strings.Contains(lower, "system32") || strings.Contains(lower, "wsl") {
			return quoted
		}
		return fmt.Sprintf(`"%s" --login -i`, shell)
	}
	if strings.Contains(lower, "cmd.exe") {
		return fmt.Sprintf(`"%s" /k`, shell)
	}
	return quoted
}

func shellName(path string) string {
	if distro, ok := parseWSLPath(path); ok {
		return "WSL - " + distro
	}
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".exe")
	return base
}

func (s *LocalSession) readLoop() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-s.quit:
			return
		default:
		}

		var n int
		var err error
		if s.cpty != nil {
			n, err = s.cpty.Read(buf)
		} else {
			n, err = s.stdout.Read(buf)
		}

		if n > 0 {
			s.RecordReadActivity()
			data := append([]byte(nil), buf[:n]...)
			s.emitData(data)
			s.updateMouseTrackingState(data)
		}
		if err != nil {
			// If the quit channel is already closed, another goroutine
			// (e.g. the Wait() goroutine) has already initiated disconnect;
			// the read error is a side-effect of the pipe being closed and
			// should be silently ignored.
			select {
			case <-s.quit:
				return
			default:
			}
			if err != io.EOF {
				// On ConPTY (and pipe fallback), any read error after a
				// process exits is a predictable consequence of the pipe
				// breaking — treat it as clean EOF instead of showing a
				// raw OS error message that confuses users (e.g. when
				// opencode /exit kills the parent shell).
				if s.cpty == nil {
					s.emitData([]byte(fmt.Sprintf("\r\n[read error: %v]\r\n", err)))
				}
			}
			s.Disconnect()
			return
		}
	}
}

func (s *LocalSession) Write(data []byte) error {
	var err error
	if s.cpty != nil {
		_, err = s.cpty.Write(data)
	} else if s.stdin != nil {
		_, err = s.stdin.Write(data)
	} else {
		return fmt.Errorf("not connected")
	}
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

// Disconnect tears down the local session. It uses sync.Once so the entire
// teardown sequence (including ConPTY Close / process Kill) executes exactly
// once, regardless of how many goroutines call Disconnect concurrently.
func (s *LocalSession) Disconnect() error {
	s.disconnectOnce.Do(func() {
		close(s.quit)
		if s.cpty != nil {
			s.cpty.Close()
			s.cpty = nil
		}
		if s.stdin != nil {
			s.stdin.Close()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			s.cmd.Process.Kill()
		}
		s.setStatus(StatusDisconnected)
	})
	return nil
}

func (s *LocalSession) Resize(cols, rows int) error {
	s.SetPendingSize(cols, rows)
	if s.cpty != nil {
		return s.cpty.Resize(cols, rows)
	}
	// Pipe mode: no resize support.
	return nil
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
		if s.cpty != nil {
			s.cpty.Write(data)
		} else if s.stdin != nil {
			s.stdin.Write(data)
		}
	}
	s.baseSession.RunPostLoginScript(ctx, script, send, s.IsConnected)
}

func defaultShell() string {
	if _, err := exec.LookPath("pwsh.exe"); err == nil {
		return "pwsh.exe"
	}
	if _, err := exec.LookPath("powershell.exe"); err == nil {
		return "powershell.exe"
	}
	// Prefer Git Bash over WSL bash to avoid WSL relay errors.
	gitBashPaths := []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "bin", "bash.exe"),
	}
	for _, p := range gitBashPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if _, err := exec.LookPath("bash.exe"); err == nil {
		return "bash.exe"
	}
	return "cmd.exe"
}
