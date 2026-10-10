//go:build windows
// +build windows

package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"

	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/platform"
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
	admin                *adminPty // elevated shell relayed from the broker process
	stdin                io.WriteCloser
	stdout               io.Reader
	cmd                  *exec.Cmd
	quit                 chan struct{}
	disconnectOnce       sync.Once
	mouseTrackingEnabled atomic.Bool

	// osc7 extracts injected OSC-7 cwd reports from the raw ConPTY/pipe
	// output stream (see shell_integration.go). Only used from readLoop.
	osc7 osc7Scanner

	// WSL typed cwd-hook injection (two-phase). wslDistro is non-empty only
	// for wsl:// shells. bash/zsh run in two phases: the arm line turns
	// stty -echo on and the hook body is written only once the echo-armed
	// marker is seen in the output stream, so the body never renders. fish
	// skips phase 1 (no stty in its snippet) and accepts the single-line
	// echo leak, same as SSH fish. The marker scanners and the arm/body
	// handoff live in the shared typedCwdHookState.
	wslDistro        string
	wslShellBase     string
	wslCwdHookBody   string
	cwdHookFollow    bool
	cwdHookTriggered atomic.Bool
	hook             typedCwdHookState

	mu             sync.RWMutex
	enc            encoding.Encoding
	decoder        *encoding.Decoder
	encoder        *encoding.Encoder
	decodeLeftover []byte
	decodeScratch  []byte
	encScratch     []byte
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
	s.hook = newTypedCwdHookState()
	return s
}

func (s *LocalSession) Connect(config ConnectionConfig) error {
	s.SetLogOnConnect(config.LogOnConnect)
	s.setStatus(StatusConnecting)

	shell := config.ShellPath
	if shell == "" {
		shell = defaultShell()
	}
	displayPath := shell

	// admin:// shells spawn elevated. When uniTerm itself already runs as
	// administrator the prefix is dropped and the shell starts like any
	// other; otherwise an elevated broker process relays the ConPTY.
	elevate := false
	if inner, ok := ParseAdminShellPath(shell); ok {
		shell = inner
		elevate = !IsProcessElevated()
	}

	// clink:// shells run cmd.exe with clink injected (Tabby-style). The
	// WT_SESSION hint tells clink the host is a ConPTY terminal like Windows
	// Terminal, so it renders through the terminal's native VT stream instead
	// of its own ANSI emulation layer.
	clinkExe := ""
	var clinkEnv []string
	if p, ok := ParseClinkShellPath(shell); ok {
		clinkExe = p
		clinkEnv = []string{"WT_SESSION=0"}
	}
	clinkProfile := ""
	if clinkExe != "" {
		clinkProfile = ensureClinkProfile()
	}

	// Determine working directory: use config.Cwd if set, otherwise user home.
	workDir := config.Cwd
	if workDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			workDir = home
		}
	}

	s.title = shellName(displayPath)

	var commandLine string
	var cmd *exec.Cmd
	isMSYSBash := false

	if distro, ok := parseWSLPath(shell); ok {
		if distro == "" {
			s.setStatus(StatusError)
			return fmt.Errorf("empty WSL distribution name")
		}
		// SSH-style typed cwd hook: the distro starts through its native
		// login flow (`wsl.exe` without -e), so the MOTD banner and profile
		// scripts run exactly as in any other terminal; the OSC-7 hook is
		// then typed into the running shell, never injected via launch args.
		// Any probe failure or timeout degrades to a plain shell — the hook
		// is optional and must never fail the session.
		s.wslDistro = distro
		s.cwdHookFollow = config.CwdHookMode == CwdHookModeFollow
		if s.cwdHookFollow {
			log.Writef("wsl: cwd hook deferred to directory-follow toggle")
		} else if sh, ok := wslDetectShell(distro); ok {
			if body, ok := buildShellCwdHookBody(sh); ok {
				s.wslShellBase = shellBasename(sh)
				s.wslCwdHookBody = body
			} else {
				log.Writef("wsl: cwd hook skipped (unsupported shell %q)", sh)
			}
		}
		wslArgs := buildWSLStartArgs(distro, config.Cwd, nil)
		commandLine = "wsl.exe " + strings.Join(wslArgs, " ")
		cmd = exec.Command("wsl.exe", wslArgs...)
		cmd.Env = os.Environ()
	} else if clinkExe != "" {
		// Tabby-style clink injection: cmd.exe /k <clink> inject --profile
		// <dir>, started through ConPTY (clink's DLL injection needs the
		// console context ConPTY provides, so the pipe fallback below runs
		// plain cmd instead).
		commandLine = buildClinkCommandLine(clinkExe, clinkProfile)
		cmd = exec.Command("cmd.exe")
		cmd.Env = append(os.Environ(), clinkEnv...)
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

	platform.HideConsoleWindow(cmd)
	cmd.Dir = workDir

	// Elevated shell: the ConPTY lives in the elevated broker process and is
	// relayed to us over the control pipe, so everything below (direct
	// conpty.Start / pipe fallback) only applies to unelevated shells.
	if elevate {
		cols, rows := s.GetPendingSize()
		if cols <= 0 || rows <= 0 {
			cols, rows = 80, 24
		}
		tp, err := startElevatedPty(localPtySpawn{
			CommandLine: commandLine,
			WorkDir:     workDir,
			Env:         clinkEnv,
			Cols:        cols,
			Rows:        rows,
		})
		if errors.Is(err, errElevationCancelled) {
			s.setStatus(StatusError)
			return fmt.Errorf("administrator terminal: %w", err)
		}
		if err != nil {
			s.setStatus(StatusError)
			return fmt.Errorf("administrator terminal: %w", err)
		}
		s.admin = tp
		s.setStatus(StatusConnected)
		s.startEmitLoop()
		go s.readLoop()
		go s.runPostLoginScript(config.PostLoginScript)
		s.injectStartupCwdHook()
		return nil
	}

	// Try ConPTY first for a real pseudo-terminal experience.
	if conpty.IsConPtyAvailable() {
		cols, rows := s.GetPendingSize()
		if cols <= 0 || rows <= 0 {
			cols, rows = 80, 24
		}
		env := os.Environ()
		if len(clinkEnv) > 0 {
			env = append(env, clinkEnv...)
		}
		c, err := conpty.Start(commandLine, conpty.ConPtyDimensions(cols, rows), conpty.ConPtyWorkDir(workDir), conpty.ConPtyEnv(env))
		if err == nil {
			s.cpty = c
			if isMSYSBash {
				go forceUTF8ConsoleCodePage(c.Pid())
			}
			// cpty.Wait returning is the only reliable signal that the shell
			// exited — readLoop's EOF path can return early via the quit
			// check and never report the status. disconnectOnce makes the
			// double call harmless.
			go func() {
				_, _ = c.Wait(context.Background())
				s.Disconnect()
			}()
			s.setStatus(StatusConnected)
			s.startEmitLoop()
			go s.readLoop()
			go s.runPostLoginScript(config.PostLoginScript)
			s.injectStartupCwdHook()
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

	// Goroutine must NOT call s.Disconnect(); see ConPTY branch above.
	go func() {
		_ = s.cmd.Wait()
	}()

	s.setStatus(StatusConnected)
	s.startEmitLoop()
	go s.readLoop()
	go s.runPostLoginScript(config.PostLoginScript)
	s.injectStartupCwdHook()
	return nil
}

func parseWSLPath(path string) (distro string, ok bool) {
	const prefix = "wsl://"
	if !strings.HasPrefix(strings.ToLower(path), prefix) {
		return "", false
	}
	return path[len(prefix):], true
}

// wslIntegrationTimeout bounds every wsl.exe one-shot call of the WSL shell
// integration (shell detection, temp file/dir writes). WSL cold starts can
// take a few seconds per call; any timeout or error silently degrades to a
// plain `wsl.exe -d <distro>`.
const wslIntegrationTimeout = 10 * time.Second

// wslDetectShell probes the distro's default shell via a one-shot
// wsl.exe -e process. This probe is throwaway and does not affect the
// session's own shell, mirroring the SSH probe design: the session shell
// starts untouched (MOTD and profile scripts intact) and the OSC-7 hook is
// typed in afterwards.
func wslDetectShell(distro string) (shell string, ok bool) {
	shell, err := wslRunCommand(distro, "echo $SHELL", "", wslIntegrationTimeout)
	if err != nil {
		log.Writef("wsl: shell detection failed for %s: %v", distro, err)
		return "", false
	}
	return strings.TrimSpace(shell), true
}

// wslCwdHookRowCleanup is prepended to the hook body (phase 2, echo off) to
// wipe the conhost echo of phase 1: save the cursor, clear the home row,
// restore. Absolute positioning, so it cannot drift with the prompt layout.
const wslCwdHookRowCleanup = " printf '\\0337\\033[H\\033[2K\\0338';"

// injectStartupCwdHook starts the typed cwd-hook injection right after the
// PTY starts. ConPTY buffers the bytes, so the shell executes the phases in
// order: phase 1 arms echo-off (bash/zsh) and the hook body is written only
// after the echo-armed marker is seen in the output stream (read loop);
// phase 2's body must never render. fish skips phase 1 (no stty in its
// snippet) and accepts the single-line echo leak, same as SSH fish.
func (s *LocalSession) injectStartupCwdHook() {
	if s.cwdHookFollow || s.wslDistro == "" || s.wslCwdHookBody == "" {
		return
	}
	if s.wslShellBase == "bash" || s.wslShellBase == "zsh" {
		// The body carries the row cleanup with it: the arm line's conhost
		// echo must be erased by the time the body runs (echo off).
		s.hook.arm(wslCwdHookRowCleanup + s.wslCwdHookBody)
		_ = s.Write([]byte(cwdHookArmLine))
		return
	}
	// fish and other supported shells without an echo-off prefix: single
	// phase, watch for the ready marker.
	if err := s.Write([]byte(s.wslCwdHookBody)); err != nil {
		log.Writef("wsl: cwd hook write failed: %v", err)
		return
	}
	log.Writef("wsl: cwd hook injected (shell=%s)", s.wslShellBase)
	go s.watchCwdHookConfirm()
}

// writeWslCwdHookBody is phase 2: runs once the echo-armed marker confirmed
// that stty -echo took effect, so the hook body cannot echo. The body already
// carries the home-row cleanup that erases the conhost echo of phase 1 (it is
// prepended when the hook is armed).
func (s *LocalSession) writeWslCwdHookBody(body string) {
	if body == "" {
		return
	}
	if err := s.Write([]byte(body)); err != nil {
		log.Writef("wsl: cwd hook body write failed: %v", err)
		return
	}
	log.Writef("wsl: cwd hook injected (shell=%s)", s.wslShellBase)
	go s.watchCwdHookConfirm()
}

// watchCwdHookConfirm bounds how long the session waits for the injected
// hook's ready marker. If the hook never confirms (unsupported setup, the
// user exited the shell instantly), echo is restored blindly so the session
// is never left with terminal echo off.
func (s *LocalSession) watchCwdHookConfirm() {
	select {
	case <-time.After(cwdHookConfirmTimeout):
	case <-s.quit:
		return
	}
	if s.hook.isInstalled() || s.Status() != StatusConnected {
		return
	}
	log.Writef("wsl: cwd hook not confirmed after %s, restoring echo blindly", cwdHookConfirmTimeout)
	_ = s.Write([]byte(cwdHookBlindRestoreLine))
}

// InjectCwdHook types the OSC-7 cwd hook into the running shell on demand,
// for WSL sessions with CwdHookMode "follow" (the startup injection was
// skipped). WSLSession satisfies session.CwdHookInjector through embedding,
// so the frontend's directory-follow toggle reaches it via the same
// App.SessionInjectCwdHook route SSH uses. Repeated calls are no-ops; a
// transiently failed attempt re-arms so a later call can retry.
func (s *LocalSession) InjectCwdHook() error {
	if s.wslDistro == "" {
		return fmt.Errorf("on-demand cwd hook injection is only available for WSL sessions")
	}
	if !s.cwdHookTriggered.CompareAndSwap(false, true) {
		return nil
	}
	if s.Status() != StatusConnected {
		s.cwdHookTriggered.Store(false)
		return fmt.Errorf("wsl session not connected")
	}
	if s.hook.isInstalled() {
		return nil
	}
	shell, ok := wslDetectShell(s.wslDistro)
	if !ok {
		s.cwdHookTriggered.Store(false)
		return fmt.Errorf("wsl shell detection failed")
	}
	base := shellBasename(shell)
	body, ok := buildShellCwdHookBody(shell)
	if !ok {
		s.cwdHookTriggered.Store(false)
		return fmt.Errorf("cwd hook unavailable (shell %q)", shell)
	}
	if base == "bash" || base == "zsh" {
		// Two-phase: arm echo-off, then let the read loop write the body
		// once the echo-armed marker confirms stty -echo took effect. The
		// body carries the home-row cleanup for the arm line's conhost echo.
		s.mu.Lock()
		s.wslShellBase = base
		s.mu.Unlock()
		s.hook.arm(wslCwdHookRowCleanup + body)
		if err := s.Write([]byte(cwdHookArmLine)); err != nil {
			s.hook.disarm()
			s.cwdHookTriggered.Store(false)
			return fmt.Errorf("cwd hook write: %w", err)
		}
		return nil
	}
	if err := s.Write([]byte(body)); err != nil {
		s.cwdHookTriggered.Store(false)
		return fmt.Errorf("cwd hook write: %w", err)
	}
	log.Writef("wsl: cwd hook injected on demand (shell=%s)", base)
	go s.watchCwdHookConfirm()
	return nil
}
// wslRunCommand runs a one-shot command inside the distro via wsl.exe -e
// with a timeout, optionally feeding stdin, and returns stdout.
func wslRunCommand(distro, command, stdin string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", "-d", distro, "-e", "sh", "-c", command)
	platform.HideConsoleWindow(cmd)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// buildWSLStartArgs returns the full wsl.exe argument list for launching a
// distro. Without an explicit cwd, `--cd ~` makes wsl.exe start the shell in
// the distro user's Linux home; otherwise the shell inherits uniTerm's own
// working directory, which wsl.exe maps to /mnt/c/Users/<name>.
//
// Note: the distro name is not quoted here. In the ConPTY path, quoted names
// are interpreted literally by wsl.exe and cause WSL_E_DISTRO_NOT_FOUND.
// Distribution names with spaces are uncommon; if needed, the pipe fallback
// (exec.Command with separate args) handles them correctly.
func buildWSLStartArgs(distro string, cwd string, startArgs []string) []string {
	args := []string{"-d", distro}
	if cwd == "" {
		args = append(args, "--cd", "~")
	}
	return append(args, startArgs...)
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
	if inner, ok := ParseAdminShellPath(path); ok {
		return shellName(inner) + " (Admin)"
	}
	if _, ok := ParseClinkShellPath(path); ok {
		return "CMD (Clink)"
	}
	if distro, ok := parseWSLPath(path); ok {
		return "WSL - " + distro
	}
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".exe")
	// Disambiguate bash flavors by install path: Git for Windows, Cygwin and
	// MSYS2 all ship a bash.exe, and a bare "bash" title tells the user
	// nothing. Mirrors the frontend getShellLabel rules.
	if strings.EqualFold(base, "bash") {
		lower := strings.ToLower(path)
		if strings.Contains(lower, `\git\`) || strings.Contains(lower, `/git/`) ||
			strings.Contains(lower, "chocolatey") {
			if strings.Contains(lower, `\scoop\`) || strings.Contains(lower, `/scoop/`) {
				return "Git Bash (Scoop)"
			}
			return "Git Bash"
		}
		if strings.Contains(lower, "cygwin") {
			return "Cygwin bash"
		}
		if strings.Contains(lower, "msys") {
			return "MSYS2 bash"
		}
	}
	return base
}

// SetEncoding configures the character encoding for this session.
// name: "" / "utf-8" (passthrough) | "gbk" | "gb2312" | "gb18030" |
// "big5" | "shift-jis" | "euc-jp" | "euc-kr".
func (s *LocalSession) SetEncoding(name string) {
	enc := encodingByName(name)
	s.mu.Lock()
	s.enc = enc
	if enc == nil {
		s.decoder = nil
		s.encoder = nil
	} else {
		s.decoder = enc.NewDecoder()
		s.encoder = enc.NewEncoder()
	}
	s.decodeLeftover = nil
	s.mu.Unlock()
}

// decodeOutput converts a chunk of PTY bytes to UTF-8 using the configured
// decoder. Partial trailing multibyte sequences are buffered until the next
// call. Must only be called from the single readLoop goroutine.
func (s *LocalSession) decodeOutput(data []byte) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.decoder == nil {
		return data
	}
	s.decodeScratch = s.decodeScratch[:0]
	s.decodeScratch = append(s.decodeScratch, s.decodeLeftover...)
	s.decodeScratch = append(s.decodeScratch, data...)
	src := s.decodeScratch

	var out []byte
	dst := make([]byte, 8192)
	for {
		nDst, nSrc, err := s.decoder.Transform(dst, src, false)
		out = append(out, dst[:nDst]...)
		src = src[nSrc:]
		if err == transform.ErrShortDst {
			continue
		}
		break
	}
	if len(src) > 0 {
		s.decodeLeftover = append(s.decodeLeftover[:0], src...)
	} else {
		s.decodeLeftover = src[:0]
	}
	return out
}

// encodeInput converts user keystrokes (UTF-8) to the configured encoding
// before writing to the PTY. Each call handles a complete UTF-8 input.
func (s *LocalSession) encodeInput(data []byte) []byte {
	s.mu.RLock()
	encoder := s.encoder
	s.mu.RUnlock()
	if encoder == nil {
		return data
	}
	encoder.Reset()
	s.encScratch = s.encScratch[:0]
	nDst, _, err := encoder.Transform(s.encScratch, data, true)
	if err != nil && err != transform.ErrShortSrc {
		return data
	}
	return s.encScratch[:nDst]
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
		usingConPty := s.cpty != nil
		if s.admin != nil {
			n, err = s.admin.Read(buf)
		} else if usingConPty {
			n, err = s.cpty.Read(buf)
		} else {
			n, err = s.stdout.Read(buf)
		}

		if n > 0 {
			s.RecordReadActivity()
			data := append([]byte(nil), buf[:n]...)
			// OSC-7 extraction runs on the RAW byte stream, BEFORE decoding:
			// the sequence is pure ASCII while legacy codecs (GBK/Big5/...)
			// could mangle its bytes or withhold a fragment in their
			// cross-chunk multibyte leftover. The cleaned remainder replaces
			// the data for every downstream consumer so stripped sequences
			// never render.
			cwd, cleaned, found := s.osc7.Feed(data)
			if found {
				recordSessionCwd(s.id, cwd)
				if TerminalCwdSink != nil {
					TerminalCwdSink(s.id, cwd)
				}
			}
			cleaned, armed, confirmed := s.hook.onOutput(cleaned)
			if armed {
				go s.writeWslCwdHookBody(s.hook.takeArmedBody())
			}
			if confirmed {
				s.hook.confirm()
				log.Writef("wsl: cwd hook confirmed via ready marker")
			}
			s.emitData(s.decodeOutput(cleaned))
			s.updateMouseTrackingState(cleaned)
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
			// If a terminal application enabled mouse tracking and then exited
			// without disabling it, reset tracking now that the process is gone
			// — this restores native text selection.
			if s.mouseTrackingEnabled.Load() {
				s.emitData(mouseTrackingReset)
				s.mouseTrackingEnabled.Store(false)
			}
			if err != io.EOF {
				// On ConPTY (and pipe fallback), any read error after a
				// process exits is a predictable consequence of the pipe
				// breaking — treat it as clean EOF instead of showing a
				// raw OS error message that confuses users (e.g. when
				// opencode /exit kills the parent shell). Only the pipe
				// fallback surfaces read errors to the user.
				if !usingConPty {
					s.emitData([]byte(fmt.Sprintf("\r\n[read error: %v]\r\n", err)))
				}
			}
			s.Disconnect()
			return
		}
	}
}

func (s *LocalSession) Write(data []byte) error {
	encoded := s.encodeInput(data)
	var err error
	if s.admin != nil {
		_, err = s.admin.Write(encoded)
	} else if s.cpty != nil {
		_, err = s.cpty.Write(encoded)
	} else if s.stdin != nil {
		_, err = s.stdin.Write(encoded)
	} else {
		return fmt.Errorf("not connected")
	}
	return err
}

// Disconnect tears down the local session. It uses sync.Once so the entire
// teardown sequence (including ConPTY Close / process Kill) executes exactly
// once, regardless of how many goroutines call Disconnect concurrently.
func (s *LocalSession) Disconnect() error {
	s.disconnectOnce.Do(func() {
		close(s.quit)
		if s.admin != nil {
			// Closing the control pipe makes the elevated broker kill the
			// shell and exit.
			s.admin.Close()
		}
		if s.cpty != nil {
			// Close but leave the field set: readLoop, Write and Resize read
			// s.cpty without holding a lock, and nilling it here raced them.
			// A closed ConPty returns errors, which those paths handle.
			s.cpty.Close()
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
	if s.admin != nil {
		return s.admin.Resize(cols, rows)
	}
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
