//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/platform"
	"github.com/ys-ll/uniterm/backend/session"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func additionalWindowOptions(options application.WebviewWindowOptions) application.WebviewWindowOptions {
	return options
}

func (a *App) GetAvailableShells() []string {
	var shells []string
	var seen = make(map[string]bool)

	add := func(path string) {
		if path == "" {
			return
		}
		abs, err := exec.LookPath(path)
		if err != nil {
			return
		}
		key := strings.ToLower(strings.ReplaceAll(abs, `\`, `/`))
		if seen[key] {
			return
		}
		seen[key] = true
		shells = append(shells, abs)
	}

	hasShell := func(name string) bool {
		for _, sh := range shells {
			if strings.EqualFold(filepath.Base(sh), name) {
				return true
			}
		}
		return false
	}

	add("pwsh.exe")
	add("powershell.exe")
	add("cmd.exe")
	// CMD (Clink) rides the user's installed clink (1.1+); the Tabby-style
	// profile tuning happens at session start (backend/session/local_clink.go).
	if clink := findClink(); clink != "" {
		shells = append(shells, session.ClinkShellPathPrefix+clink)
	}
	// A user-configured Git Bash location (settings.gitBashPath) wins over the
	// auto-probed ones, so it is listed first among the bash shells.
	if a.settingsStore != nil {
		if st, err := a.settingsStore.Load(); err == nil {
			if p := resolveGitBashPath(st.GitBashPath, fileExists); p != "" {
				add(p)
			}
		}
	}
	for _, p := range gitBashCandidates() {
		add(p)
	}
	for _, p := range []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
		`C:\ProgramData\chocolatey\bin\bash.exe`,
	} {
		add(p)
	}
	// Third-party shells (Cygwin, MSYS2, Nushell): probe well-known install
	// locations and the registry so they are offered automatically, without
	// any user configuration. Added after the built-ins and before the
	// generic PATH bash.exe fallback, whose System32 hit they preempt when a
	// real bash exists.
	for _, sh := range detectThirdPartyShells(probeCygwinRootdir(),
		func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		exec.LookPath) {
		add(sh)
	}
	if !hasShell("bash.exe") {
		add("bash.exe")
	}
	if distros, _ := listWSLDistros(); len(distros) > 0 {
		for _, d := range distros {
			shells = append(shells, "wsl://"+d)
		}
	}
	// Administrator variants ride on the detected cmd/powershell paths (UAC
	// elevation via the broker in backend/session/local_admin_windows.go).
	// Only offered when uniTerm itself is unelevated — as admin they would be
	// indistinguishable duplicates of the plain entries.
	if !session.IsProcessElevated() {
		for _, sh := range shells {
			base := strings.ToLower(filepath.Base(sh))
			if base == "cmd.exe" || base == "powershell.exe" {
				shells = append(shells, session.AdminShellPathPrefix+sh)
			}
		}
		for _, sh := range shells {
			if _, ok := session.ParseClinkShellPath(sh); ok {
				shells = append(shells, session.AdminShellPathPrefix+sh)
			}
		}
	}
	return shells
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// resolveGitBashPath turns a user-supplied Git location into a bash.exe path.
// It accepts bash.exe itself, a Git for Windows root (such as a scoop
// `apps\git\current` directory) or its bin / usr\bin subdirectory. Returns ""
// when nothing usable exists.
func resolveGitBashPath(input string, exists func(string) bool) string {
	input = strings.Trim(strings.TrimSpace(input), `"`)
	if input == "" {
		return ""
	}
	input = os.ExpandEnv(strings.ReplaceAll(input, "%USERPROFILE%", "$USERPROFILE"))
	if strings.HasSuffix(strings.ToLower(input), ".exe") {
		if exists(input) {
			return input
		}
		return ""
	}
	for _, rel := range [][]string{{"bin", "bash.exe"}, {"usr", "bin", "bash.exe"}, {"bash.exe"}} {
		p := filepath.Join(append([]string{input}, rel...)...)
		if exists(p) {
			return p
		}
	}
	return ""
}

// gitBashCandidates lists per-user Git for Windows installs (scoop) that the
// fixed Program Files probes miss.
func gitBashCandidates() []string {
	var out []string
	for _, root := range []string{
		filepath.Join(os.Getenv("USERPROFILE"), "scoop"),
		os.Getenv("SCOOP"),
	} {
		if root == "" || root == "scoop" {
			continue
		}
		if p := resolveGitBashPath(filepath.Join(root, "apps", "git", "current"), fileExists); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// detectThirdPartyShells probes well-known third-party shell installs that
// should be offered alongside the built-in shells: Cygwin bash, MSYS2 bash
// and Nushell. cygwinRegRoot is the Cygwin setup registry probe result (""
// when absent). Only paths that actually exist are returned; duplicates are
// removed case-insensitively.
func detectThirdPartyShells(cygwinRegRoot string, exists func(string) bool, lookPath func(string) (string, error)) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(path string) {
		if path == "" {
			return
		}
		key := strings.ToLower(path)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, path)
	}

	// Cygwin: setup.exe records the install root in the registry; also probe
	// the conventional install dirs.
	cygwinRoots := []string{`C:\cygwin64`, `C:\cygwin`, `C:\tools\cygwin64`, `C:\tools\cygwin`}
	if cygwinRegRoot != "" {
		cygwinRoots = append([]string{cygwinRegRoot}, cygwinRoots...)
	}
	for _, root := range cygwinRoots {
		bash := filepath.Join(root, "bin", "bash.exe")
		if exists(bash) {
			add(bash)
		}
	}

	// MSYS2: bash lives under usr\bin and there is no registry entry.
	msysRoots := []string{`C:\msys64`, `C:\msys32`, `C:\tools\msys64`, `C:\tools\msys32`}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		msysRoots = append(msysRoots, filepath.Join(home, "msys64"), filepath.Join(home, "msys32"))
	}
	for _, root := range msysRoots {
		bash := filepath.Join(root, "usr", "bin", "bash.exe")
		if exists(bash) {
			add(bash)
		}
	}

	// Nushell: usually on PATH (scoop/choco/winget shims).
	if nu, err := lookPath("nu.exe"); err == nil && nu != "" {
		add(nu)
	}

	return out
}

// probeCygwinRootdir reads the Cygwin setup install root from the registry.
// Returns "" when Cygwin is absent or the key cannot be read.
func probeCygwinRootdir() string {
	keys := []string{
		`SOFTWARE\Cygwin\setup`,
		`SOFTWARE\WOW6432Node\Cygwin\setup`,
	}
	for _, k := range keys {
		for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
			key, err := registry.OpenKey(root, k, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			rootdir, _, err := key.GetStringValue("rootdir")
			key.Close()
			if err == nil && rootdir != "" {
				return rootdir
			}
		}
	}
	return ""
}

// findClink locates the user's clink (1.1+): PATH first (covers installs
// that put themselves on PATH, scoop shims and chocolatey shims), then the
// well known install directories clink's setup and package managers use.
// The release layout ships arch-named executables (clink_x64.exe,
// clink_arm64.exe, ... from the official zip), older/manual installs have a
// plain clink.exe — both are accepted. Returns "" when clink is not found.
func findClink() string {
	archExe := map[string]string{
		"amd64": "clink_x64.exe",
		"arm64": "clink_arm64.exe",
		"386":   "clink_x86.exe",
	}[runtime.GOARCH]
	for _, name := range []string{archExe, "clink.exe"} {
		if name == "" {
			continue
		}
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, dir := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), "clink"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "clink"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "clink"),
		filepath.Join(os.Getenv("USERPROFILE"), "scoop", "apps", "clink", "current"),
		filepath.Join(os.Getenv("SCOOP"), "apps", "clink", "current"),
	} {
		if dir == "" {
			continue
		}
		for _, name := range []string{archExe, "clink.exe"} {
			if name == "" {
				continue
			}
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

func listWSLDistros() ([]string, error) {
	cmd := exec.Command("wsl.exe", "-l", "-q")
	platform.HideConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	return parseWSLDistros(out), nil
}

func parseWSLDistros(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}

	content := string(raw)
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		u16 := make([]uint16, 0, len(raw)/2)
		for i := 2; i+1 < len(raw); i += 2 {
			u16 = append(u16, uint16(raw[i])|uint16(raw[i+1])<<8)
		}
		content = string(utf16.Decode(u16))
	}

	var distros []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		line = strings.ReplaceAll(line, "\x00", "")
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "docker-desktop") {
			continue
		}
		if !seen[line] {
			seen[line] = true
			distros = append(distros, line)
		}
	}
	return distros
}

const (
	GWLP_WNDPROC     = ^uintptr(3)
	WM_ENTERSIZEMOVE = 0x0231
	WM_EXITSIZEMOVE  = 0x0232
	WM_SIZE          = 0x0005
)

func (a *App) findMainWindow() uintptr {
	pid := windows.GetCurrentProcessId()
	var result uintptr

	user32 := windows.NewLazySystemDLL("user32.dll")
	procEnumWindows := user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId := user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextW := user32.NewProc("GetWindowTextW")

	cb := windows.NewCallback(func(hwnd windows.HWND, lParam uintptr) uintptr {
		var wndPid uint32
		procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wndPid)))
		if wndPid != pid {
			return 1 // continue
		}
		// Verify it has our window title so we don't pick up invisible helper windows.
		buf := make([]uint16, 256)
		procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), 255)
		if windows.UTF16ToString(buf) == "uniTerm" {
			result = uintptr(hwnd)
			return 0 // stop
		}
		return 1 // continue
	})
	procEnumWindows.Call(cb, 0)
	return result
}

// bringMainWindowToFront raises the main window to the foreground once. After a
// relaunch the fresh instance can otherwise open behind other windows on Windows:
// SetForegroundWindow is normally restricted, but a process spawned by the
// currently-foreground process is one of the documented exceptions, so a single
// raise works while the old instance (the foreground process) is still alive.
// No-op when no main window has been created yet.
func (a *App) bringMainWindowToFront() {
	hwnd := a.mainHwnd
	if hwnd == 0 {
		hwnd = a.findMainWindow()
	}
	if hwnd == 0 {
		return
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	procSetForegroundWindow := user32.NewProc("SetForegroundWindow")
	procBringWindowToTop := user32.NewProc("BringWindowToTop")
	procSetForegroundWindow.Call(hwnd)
	procBringWindowToTop.Call(hwnd)
}

// emitMoveResize sends a move/resize event to the frontend without blocking.
// It must never be called from within the WndProc modal resize/move loop.
func (a *App) emitMoveResize(event string) {
	if a.moveResizeCh == nil {
		return
	}
	select {
	case a.moveResizeCh <- event:
	default:
	}
}

func (a *App) subclassMainWindow() {
	if a.mainHwnd == 0 {
		return
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	procSetWindowLongPtrW := user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW := user32.NewProc("CallWindowProcW")

	cb := windows.NewCallback(func(hwnd windows.HWND, msg uint32, wparam, lparam uintptr) uintptr {
		switch msg {
		case WM_ENTERSIZEMOVE:
			a.inSizeMove = true
		case WM_EXITSIZEMOVE:
			a.inSizeMove = false
			a.emitMoveResize("window:resize-end")
		case WM_SIZE:
			if !a.inSizeMove {
				a.emitMoveResize("window:resize-end")
			}
		}
		ret, _, _ := procCallWindowProcW.Call(a.originalWndProc, uintptr(hwnd), uintptr(msg), wparam, lparam)
		return ret
	})
	a.wndProcCb = cb

	orig, _, _ := procSetWindowLongPtrW.Call(a.mainHwnd, GWLP_WNDPROC, cb)
	a.originalWndProc = orig
}

func (a *App) unsubclassMainWindow() {
	if a.originalWndProc == 0 || a.mainHwnd == 0 {
		return
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	procSetWindowLongPtrW := user32.NewProc("SetWindowLongPtrW")
	procSetWindowLongPtrW.Call(a.mainHwnd, GWLP_WNDPROC, a.originalWndProc)
	a.originalWndProc = 0
}

// configureMacKeyRepeat is a no-op on Windows; the press-and-hold accent
// picker only exists on macOS. See app_darwin.go for details.
func (a *App) configureMacKeyRepeat() {}

// hideProcWindow prevents batch-file shims (e.g. VS Code's code.cmd) from
// flashing a console window: shims can only run through cmd.exe, which
// allocates a console of its own when launched from a GUI process that has
// none. HideWindow (STARTF_USESHOWWINDOW, SW_HIDE) suppresses that console.
// It must only be applied to shims: GUI exes honor STARTF_USESHOWWINDOW for
// their first window too, so hiding them would leave the editor invisible.
// Editors needing a CLI shim for wait semantics (VS Code's -w lives in the
// node cli.js, not in Code.exe) are launched via their shim + HideWindow.
func hideProcWindow(cmd *exec.Cmd) {
	ext := strings.ToLower(filepath.Ext(cmd.Path))
	if ext == ".cmd" || ext == ".bat" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}
}

// openAsInfo mirrors the shell32 OPENASINFO structure for SHOpenWithDialog.
type openAsInfo struct {
	file  uintptr // PCSZW: file to open
	class uintptr // PCSZW: optional ProgID hint (unused)
	flags uint32
}

const (
	oaifAllowRegistration   = 0x00000001 // offer the "always use this app" checkbox
	oaifExec                = 0x00000004 // execute the picked application
	coinitApartmentThreaded = 0x2        // COINIT_APARTMENTTHREADED
)

// openWithSystem pops the system "open with" dialog for the file and executes
// the application the user picks (owned by the main window, so it stays on
// top). It calls the documented SHOpenWithDialog API directly: the legacy
// rundll32 shell32.dll,OpenAs_RunDLLW entry was tried first, but its
// dialog-to-app hand-off corrupts the path on Win10 — the picked app received
// mojibake instead of the file (reproduced with Notepad). The call blocks
// until the dialog closes; cancelling it counts as success.
func (a *App) openWithSystem(p string) error {
	p16, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return err
	}
	// SHOpenWithDialog requires COM (STA) on the calling thread. S_OK (0)
	// means this call initialized it and must pair an uninitialize; S_FALSE
	// (1, already initialized) must not.
	ole32 := windows.NewLazySystemDLL("ole32.dll")
	if r, _, _ := ole32.NewProc("CoInitializeEx").Call(0, coinitApartmentThreaded); r == 0 {
		defer ole32.NewProc("CoUninitialize").Call()
	}
	info := openAsInfo{file: uintptr(unsafe.Pointer(p16)), flags: oaifAllowRegistration | oaifExec}
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	r1, _, callErr := shell32.NewProc("SHOpenWithDialog").Call(a.findMainWindow(), uintptr(unsafe.Pointer(&info)))
	if r1 != 0 {
		return nil
	}
	// User closed the dialog without choosing an app — not an error.
	if errno, ok := callErr.(windows.Errno); ok && errno == windows.ERROR_CANCELLED {
		return nil
	}
	return fmt.Errorf("open-with dialog failed: %w", callErr)
}

// detectExternalEditors scans for text editors installed on this Windows host
// and returns only those actually found. Console-based editors (Vim, Neovim,
// nano, Micro) are excluded: spawned from a GUI app without a console they
// exit immediately, so they aren't offered. Every GUI editor is probed on PATH
// first, then falls back to common install dirs (globbing versioned folders),
// because default Windows installs often leave these off PATH.
func detectExternalEditors() []ExternalEditorOption {
	type fixed struct{ glob, suffix string }
	type editor struct {
		name, prog, pathCmd string
		fixed               []fixed
	}
	// VS Code family: the -w (wait) flag is implemented by the node cli.js
	// invoked from the bin\*.cmd shim, NOT by the GUI exe — launching Code.exe
	// directly exits immediately and breaks auto-upload. So even when the exe
	// is found at a fixed path, the command must point at the bin shim.
	editors := []editor{
		{"VS Code", "code", "code -w", []fixed{
			{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code", "bin", "code.cmd"), " -w"},
			{filepath.Join(os.Getenv("ProgramFiles"), "Microsoft VS Code", "bin", "code.cmd"), " -w"},
		}},
		{"VS Code Insiders", "code-insiders", "code-insiders -w", []fixed{
			{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code Insiders", "bin", "code-insiders.cmd"), " -w"},
			{filepath.Join(os.Getenv("ProgramFiles"), "Microsoft VS Code Insiders", "bin", "code-insiders.cmd"), " -w"},
		}},
		{"VSCodium", "codium", "codium -w", []fixed{
			{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "VSCodium", "bin", "codium.cmd"), " -w"},
			{filepath.Join(os.Getenv("ProgramFiles"), "VSCodium", "bin", "codium.cmd"), " -w"},
		}},
		{"Sublime Text", "subl", "subl -w", []fixed{
			{filepath.Join(os.Getenv("ProgramFiles"), "Sublime Text", "subl.exe"), " -w"},
			{filepath.Join(os.Getenv("ProgramFiles(x86)"), "Sublime Text", "subl.exe"), " -w"},
			{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Sublime Text", "subl.exe"), " -w"},
		}},
		{"Atom", "atom", "atom", []fixed{
			{filepath.Join(os.Getenv("LOCALAPPDATA"), "atom", "bin", "atom.exe"), ""},
			{filepath.Join(os.Getenv("ProgramFiles"), "Atom", "bin", "atom.exe"), ""},
		}},
		{"gVim", "gvim", "gvim", []fixed{
			{filepath.Join(os.Getenv("ProgramFiles"), "Vim", "vim*", "gvim.exe"), ""},
			{filepath.Join(os.Getenv("ProgramFiles(x86)"), "Vim", "vim*", "gvim.exe"), ""},
		}},
		{"Emacs", "emacs", "emacs", []fixed{
			{filepath.Join(os.Getenv("ProgramFiles"), "Emacs", "emacs-*", "bin", "emacs.exe"), ""},
			{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Emacs", "emacs-*", "bin", "emacs.exe"), ""},
		}},
		{"Notepad++", "notepad++", "notepad++", []fixed{
			{filepath.Join(os.Getenv("ProgramFiles"), "Notepad++", "notepad++.exe"), ""},
			{filepath.Join(os.Getenv("ProgramFiles(x86)"), "Notepad++", "notepad++.exe"), ""},
			{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Notepad++", "notepad++.exe"), ""},
		}},
		{"EmEditor", "emeditor", "emeditor", []fixed{
			{filepath.Join(os.Getenv("ProgramFiles"), "EmEditor", "emeditor.exe"), ""},
			{filepath.Join(os.Getenv("ProgramFiles(x86)"), "EmEditor", "emeditor.exe"), ""},
			{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "EmEditor", "emeditor.exe"), ""},
		}},
		{"Notepad--", "notepad--", "notepad--", []fixed{
			{filepath.Join(os.Getenv("ProgramFiles"), "Notepad--", "Notepad--.exe"), ""},
			{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Notepad--", "Notepad--.exe"), ""},
		}},
		// Notepad lives in System32, which is always on PATH.
		{"Notepad", "notepad", "notepad", nil},
	}

	var out []ExternalEditorOption
	seen := map[string]bool{} // by canonical display name
	add := func(name, command string) {
		if seen[name] {
			return
		}
		seen[name] = true
		out = append(out, ExternalEditorOption{Label: name + " (" + command + ")", Value: command})
	}

	for _, e := range editors {
		// Prefer the real GUI exe in known install dirs: launching it directly
		// avoids the batch shim (code.cmd) and the console window it flashes.
		for _, f := range e.fixed {
			matches, _ := filepath.Glob(f.glob)
			if len(matches) > 0 {
				add(e.name, "\""+matches[0]+"\""+f.suffix)
				break
			}
		}
		if seen[e.name] {
			continue
		}
		// Fall back to PATH (may resolve to a shim — covered by hideProcWindow).
		if _, err := exec.LookPath(e.prog); err == nil {
			add(e.name, e.pathCmd)
		}
	}

	return out
}

// Win11 rounds the corners of top-level windows whose DWM frame is intact.
// Wails' frameless windows keep WS_THICKFRAME, but Wails only extends the DWM
// frame (DwmExtendFrameIntoClientArea) from its WM_ACTIVATE handler, and the
// window's first activation happens inside CreateWindowEx (WS_VISIBLE) before
// Wails' WndProc is hooked — so that first extension is missed. A freshly
// launched binary then shows square corners until the next activation
// (minimise/restore, alt-tab) reapplies it. Asking DWM directly for rounded
// corners makes the first paint correct regardless of activation timing; on
// pre-Win11 systems the unsupported attribute call fails silently, which is
// fine.
const (
	dwmwaWindowCornerPreference = 33
	dwmwcpRound                 = 2
)

var procDwmSetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

// applyRoundedCorners sets DWMWCP_ROUND on the given HWND.
// applyMacTrafficLightLayout is a no-op off macOS: only AppKit has standard
// window buttons to re-frame (see traffic_lights_darwin.go).
func applyMacTrafficLightLayout(unsafe.Pointer) {}

func applyRoundedCorners(hwnd unsafe.Pointer) {
	if hwnd == nil {
		return
	}
	preference := uint32(dwmwcpRound)
	_, _, _ = procDwmSetWindowAttribute.Call(
		uintptr(hwnd),
		dwmwaWindowCornerPreference,
		uintptr(unsafe.Pointer(&preference)),
		unsafe.Sizeof(preference),
	)
}

// systemPrefersDark reports whether Windows is in app dark mode, reading the
// same Personalization registry value the WebView2 engine maps to the CSS
// prefers-color-scheme media query. Needed because v3's IsDarkMode() is
// unavailable before Run() and the startup background colour must be resolved
// at window creation. Any failure defaults to dark.
func systemPrefersDark() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer k.Close()
	val, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return true
	}
	return val == 0
}

// systemLanguage returns the Windows display language (Settings → Time &
// Language → Language) as a locale tag for tray-menu localization
// (issue #1215). GetUserDefaultUILanguage is the same value the WebView2
// engine derives navigator.language from, so the tray matches the UI when
// the app language is set to "system". x/sys/windows does not wrap the
// kernel32 export, so it is loaded via LazyDLL. Returns "" when the LANGID
// is not one of the app's supported languages — the caller falls back.
func systemLanguage() string {
	r, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	return langIDToLocale(uint16(r))
}

// langIDToLocale maps a Windows LANGID to one of the app's supported locale
// tags. Only the Chinese sublanguages matter for picking the written form;
// every other language matches on the primary identifier alone. Unknown
// LANGIDs return "".
func langIDToLocale(langid uint16) string {
	primary := langid & 0x03FF
	if primary == 0x0004 { // Chinese
		switch langid >> 10 {
		case 0x01, 0x03, 0x05: // Traditional, Hong Kong, Macao
			return "zh-TW"
		default: // Simplified (PRC, Singapore, zh-Hans)
			return "zh-CN"
		}
	}
	switch primary {
	case 0x0009:
		return "en"
	case 0x0011:
		return "ja"
	case 0x0012:
		return "ko"
	case 0x0007:
		return "de"
	case 0x000A:
		return "es"
	case 0x000C:
		return "fr"
	case 0x0019:
		return "ru"
	case 0x0016:
		return "pt"
	case 0x002A:
		return "vi"
	}
	return ""
}

// waitForRelaunchParent and the UNITERM_RELAUNCH_PID handshake were removed:
// the relaunch is now quit-then-spawn (relaunchPending + spawnSuccessorProcess
// in main.go), so the successor never sees the lock held.

// spawnSuccessorProcess starts a fresh, detached copy of the current
// executable. Called from main() AFTER w3app.Run() has returned and the
// process is about to exit, so the successor starts with the single-instance
// lock free and the stores quiesced (see RelaunchApp for why spawn happens
// after quit).
func spawnSuccessorProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	return cmd.Start()
}

// applyTrayIcon installs the tray icon: the colored glyph PNG pre-scaled to
// the exact OS small-icon size. Windows tray icons keep their own colors —
// no template-image treatment like macOS (see app_darwin.go).
func applyTrayIcon(tray *application.SystemTray) {
	tray.SetIcon(trayIconPNG())
}

// trayIconPNG returns the tray icon bytes, pre-scaled to the exact OS
// small-icon size (SM_CXSMICON — 16px at 100% DPI, 20/24px at higher scaling)
// with a box filter. wails otherwise hands the raw PNG to
// CreateIconFromResourceEx and GDI's rescale is what made the icon look
// mushy. Called once during tray setup; per-monitor DPI changes are not
// re-rendered (rare for a tray icon).
func trayIconPNG() []byte {
	user32 := windows.NewLazySystemDLL("user32.dll")
	const smCxSmIcon = 49
	size, _, _ := user32.NewProc("GetSystemMetrics").Call(smCxSmIcon)
	if size == 0 {
		size = 16
	}
	return scalePNG(appIconTrayPNG, int(size), int(size))
}

// scalePNG decodes a PNG and box-filter downsamples it to w×h, returning the
// result re-encoded as PNG. On any decode error the input is returned
// unchanged (wails then falls back to GDI scaling). Box filtering averages
// each destination pixel over its source rectangle — the right filter for
// large ratios and cheap enough for a one-shot 64→16px icon.
func scalePNG(data []byte, w, h int) []byte {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw == 0 || sh == 0 {
		return data
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy0, sy1 := y*sh/h, (y+1)*sh/h
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < w; x++ {
			sx0, sx1 := x*sw/w, (x+1)*sw/w
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			// Average in premultiplied space so transparent edges don't
			// bleed dark fringes into the glyph.
			var rs, gs, bs, as float64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					r, g, bl, a := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					rs += float64(r)
					gs += float64(g)
					bs += float64(bl)
					as += float64(a)
				}
			}
			n := float64((sx1 - sx0) * (sy1 - sy0))
			rs, gs, bs, as = rs/n, gs/n, bs/n, as/n
			a8 := uint8(as/257 + 0.5)
			off := (y*w + x) * 4
			if a8 == 0 {
				continue // leave transparent
			}
			dst.Pix[off+0] = uint8(rs/as*257 + 0.5)
			dst.Pix[off+1] = uint8(gs/as*257 + 0.5)
			dst.Pix[off+2] = uint8(bs/as*257 + 0.5)
			dst.Pix[off+3] = a8
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return data
	}
	return buf.Bytes()
}

// Clipboard file bindings for the IronRDP engine's local→remote file paste:
// the frontend polls ClipboardGetFiles to detect an CF_HDROP clipboard, and
// the WASM client's cliprdr layer translates the advertised list into a
// FileGroupDescriptorW format list; the remote then fetches chunks through
// ClipboardReadFileChunk (via the client's FileContentsRequest flow).

// setClipboardFiles publishes completed downloads as CF_HDROP so Explorer
// and other native applications can paste them with the ordinary Ctrl+V.
func (a *App) setClipboardFiles(paths []string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32 := windows.NewLazySystemDLL("user32.dll")
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	var names []uint16
	for _, path := range paths {
		names = append(names, utf16.Encode([]rune(path))...)
		names = append(names, 0)
	}
	names = append(names, 0)
	data := make([]byte, 20+len(names)*2)
	binary.LittleEndian.PutUint32(data, 20)
	binary.LittleEndian.PutUint32(data[16:], 1) // DROPFILES.fWide
	for i, c := range names {
		binary.LittleEndian.PutUint16(data[20+i*2:], c)
	}
	handle, _, err := kernel32.NewProc("GlobalAlloc").Call(0x42, uintptr(len(data))) // MOVEABLE | ZEROINIT
	if handle == 0 {
		return fmt.Errorf("GlobalAlloc: %w", err)
	}
	defer func() {
		if handle != 0 {
			kernel32.NewProc("GlobalFree").Call(handle)
		}
	}()
	ptr, _, err := kernel32.NewProc("GlobalLock").Call(handle)
	if ptr == 0 {
		return fmt.Errorf("GlobalLock: %w", err)
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data)), data)
	kernel32.NewProc("GlobalUnlock").Call(handle)
	deadline := time.Now().Add(time.Second)
	for {
		opened, _, err := user32.NewProc("OpenClipboard").Call(a.mainHwnd)
		if opened != 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("OpenClipboard: %w", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer user32.NewProc("CloseClipboard").Call()
	if ok, _, err := user32.NewProc("EmptyClipboard").Call(); ok == 0 {
		return fmt.Errorf("EmptyClipboard: %w", err)
	}
	if ok, _, err := user32.NewProc("SetClipboardData").Call(15, handle); ok == 0 {
		return fmt.Errorf("SetClipboardData: %w", err)
	}
	handle = 0 // Ownership transferred to Windows.
	return nil
}

var (
	clipboardFilesMu    sync.Mutex
	clipboardFilesPaths []string // full paths of the last advertised list, indexed by lindex
	clipboardFilesByID  = make(map[string]string)
)

// ClipboardGetFiles returns the files currently on the clipboard (CF_HDROP)
// and remembers their full paths for ClipboardReadFileChunk. Directories are
// skipped in v1.
func (a *App) ClipboardGetFiles() []ClipboardFileInfo {
	clipboardFilesMu.Lock()
	defer clipboardFilesMu.Unlock()

	paths, err := readClipboardHDROP()
	if err != nil {
		// Most common case: the clipboard holds no file list (or another
		// process holds it open). Not worth logging at volume.
		return []ClipboardFileInfo{}
	}

	out := make([]ClipboardFileInfo, 0, len(paths))
	var advertisedPaths []string
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		out = append(out, ClipboardFileInfo{
			ID:           fmt.Sprintf("%x", sha256.Sum256([]byte(p))),
			Name:         filepath.Base(p),
			Size:         st.Size(),
			LastModified: st.ModTime().UnixMilli(),
		})
		advertisedPaths = append(advertisedPaths, p)
		clipboardFilesByID[out[len(out)-1].ID] = p
	}
	clipboardFilesPaths = advertisedPaths
	return out
}

// ClipboardReadFileChunk reads up to `size` bytes of the advertised file at
// `index` starting at `offset` (the remote's FileContentsRequest flow) and
// returns it base64-encoded.
func (a *App) ClipboardReadFileChunk(index int, offset int64, size int) (string, error) {
	clipboardFilesMu.Lock()
	if index < 0 || index >= len(clipboardFilesPaths) {
		count := len(clipboardFilesPaths)
		clipboardFilesMu.Unlock()
		return "", fmt.Errorf("clipboard file index %d out of range (%d advertised)", index, count)
	}
	path := clipboardFilesPaths[index]
	clipboardFilesMu.Unlock()
	return readClipboardFileChunk(path, offset, size)
}

// ClipboardReadFileChunkByID keeps an advertised file bound to its original
// path even when polling discovers a new CF_HDROP list during a transfer.
func (a *App) ClipboardReadFileChunkByID(id string, offset int64, size int) (string, error) {
	clipboardFilesMu.Lock()
	path, ok := clipboardFilesByID[id]
	clipboardFilesMu.Unlock()
	if !ok {
		return "", fmt.Errorf("unknown clipboard file")
	}
	return readClipboardFileChunk(path, offset, size)
}

func readClipboardFileChunk(path string, offset int64, size int) (string, error) {
	if offset < 0 || size < 0 || size > 8*1024*1024 {
		return "", fmt.Errorf("invalid clipboard file range")
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open clipboard file: %w", err)
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek clipboard file: %w", err)
		}
	}
	buf := make([]byte, size)
	n, err := io.ReadFull(f, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		buf = buf[:n]
	} else if err != nil {
		return "", fmt.Errorf("read clipboard file: %w", err)
	} else {
		buf = buf[:n]
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// readClipboardHDROP enumerates the CF_HDROP file list on the clipboard.
func readClipboardHDROP() ([]string, error) {
	// OpenClipboard/CloseClipboard belong to an OS thread, not a goroutine.
	// Migration between these calls can leave the clipboard locked for every app.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const CF_HDROP = 15

	user32 := windows.NewLazySystemDLL("user32.dll")
	openClipboard := user32.NewProc("OpenClipboard")
	closeClipboard := user32.NewProc("CloseClipboard")
	getClipboardData := user32.NewProc("GetClipboardData")
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	globalLock := kernel32.NewProc("GlobalLock")
	globalUnlock := kernel32.NewProc("GlobalUnlock")
	globalSize := kernel32.NewProc("GlobalSize")

	if err := openClipboard.Find(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		r, _, _ := openClipboard.Call(0)
		if r != 0 {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("OpenClipboard failed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer closeClipboard.Call()

	r2, _, _ := getClipboardData.Call(CF_HDROP)
	if r2 == 0 {
		return nil, fmt.Errorf("clipboard holds no CF_HDROP")
	}
	hmem := windows.Handle(r2)

	locked, _, _ := globalLock.Call(r2)
	if locked == 0 {
		return nil, fmt.Errorf("GlobalLock failed")
	}
	defer globalUnlock.Call(uintptr(hmem))

	total, _, _ := globalSize.Call(r2)
	if total == 0 || total > 64*1024*1024 {
		return nil, fmt.Errorf("bad clipboard block size %d", total)
	}
	base := unsafe.Pointer(locked)

	// DROPFILES: pFiles (offset to the file list, DWORD), POINT pt (8),
	// fNC (4), fWide (4).
	if total < 20 {
		return nil, fmt.Errorf("DROPFILES header too small")
	}
	pFiles := *(*uint32)(unsafe.Pointer(base))
	fWide := *(*uint32)(unsafe.Pointer(uintptr(base) + 16))
	if pFiles == 0 || uintptr(pFiles) >= total {
		return nil, fmt.Errorf("bad DROPFILES pFiles offset %d", pFiles)
	}
	if fWide == 0 {
		return nil, fmt.Errorf("ANSI CF_HDROP is not supported")
	}

	wide := unsafe.Slice((*uint16)(unsafe.Pointer(uintptr(base)+uintptr(pFiles))), (total-uintptr(pFiles))/2)

	var names []string
	start := -1
	for i, c := range wide {
		if c == 0 {
			if start >= 0 {
				if i > start {
					names = append(names, windows.UTF16ToString(wide[start:i]))
				}
				start = -1
				// The list is double-null terminated: an empty entry ends it.
				if i+1 < len(wide) && wide[i+1] == 0 {
					break
				}
			} else {
				break // leading empty string = end of list
			}
		} else if start < 0 {
			start = i
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("CF_HDROP holds no files")
	}
	return names, nil
}
