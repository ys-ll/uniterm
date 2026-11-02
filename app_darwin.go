//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/log"
)

func (a *App) findMainWindow() uintptr {
	if a.win() == nil {
		return 0
	}
	return uintptr(a.win().NativeWindow())
}

func additionalWindowOptions(options application.WebviewWindowOptions) application.WebviewWindowOptions {
	options.Mac.WebviewPreferences.FullscreenEnabled.Set(true)
	// Keep the macOS native "traffic light" buttons (close/minimise/zoom) even
	// though the window is frameless. The app draws its own title bar (see
	// AppHeader.vue), so we hide the title text but keep FullSizeContent so the
	// native buttons are the real AppKit ones — with true hover/focus/press
	// animations and Dark/Light auto-adaptation — instead of the CSS facsimiles
	// the frontend used to render.
	options.Mac.TitleBar = application.MacTitleBarHidden
	return options
}

// hideProcWindow is a no-op on non-Windows platforms: the console-flash issue
// is Windows-specific (batch-file shims spawning cmd.exe). See app_windows.go.
func hideProcWindow(cmd *exec.Cmd) {}

func (a *App) subclassMainWindow() {}

func (a *App) unsubclassMainWindow() {}

// bringMainWindowToFront is a no-op on non-Windows platforms: the relaunched-
// window-behind-others issue is Windows-specific and does not occur here.
func (a *App) bringMainWindowToFront() {}

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

	add(os.Getenv("SHELL"))
	add("bash")
	add("zsh")
	add("fish")
	add("sh")
	return shells
}

// detectExternalEditors scans for text editors installed on this macOS host
// and returns only those actually found. All editors work here (they inherit a
// usable terminal, and GUI editors open their own windows), so terminal editors
// are offered too. TextEdit is an .app bundle and gets a fixed-path check.
func detectExternalEditors() []ExternalEditorOption {
	type cand struct{ name, prog, command string }
	pathCands := []cand{
		{"VS Code", "code", "code -w"},
		{"VS Code Insiders", "code-insiders", "code-insiders -w"},
		{"VSCodium", "codium", "codium -w"},
		{"Sublime Text", "subl", "subl -w"},
		{"Atom", "atom", "atom"},
		{"Vim", "vim", "vim"},
		{"gVim", "gvim", "gvim"},
		{"Neovim", "nvim", "nvim"},
		{"Emacs", "emacs", "emacs"},
		{"nano", "nano", "nano"},
		{"Micro", "micro", "micro"},
		{"gedit", "gedit", "gedit"},
		{"Kate", "kate", "kate"},
		{"Xcode", "xed", "xed"},
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

	for _, c := range pathCands {
		if _, err := exec.LookPath(c.prog); err == nil {
			add(c.name, c.command)
		}
	}

	// TextEdit is an .app bundle, not a PATH executable. `open -W` blocks until
	// the app closes, matching the -w behaviour of the other presets.
	for _, app := range []string{"/System/Applications/TextEdit.app", "/Applications/TextEdit.app"} {
		if pathExists(app) {
			add("TextEdit", "open -W -a TextEdit")
			break
		}
	}

	return out
}

// macBundleID is the app's bundle identifier. Wails derives the default
// identifier from wails.json's "name" as com.wails.<name>, and this project
// ships no custom build/darwin/Info.plist, so it resolves to com.wails.uniTerm.
const macBundleID = "com.wails.uniTerm"

// configureMacKeyRepeat disables macOS's press-and-hold accent picker for this
// app's bundle only, so that holding a key down produces continuous key-repeat
// input in the terminal (matching Windows behaviour) instead of popping up the
// system accent/variant character picker.
//
// This works around the long-standing upstream xterm.js issue on macOS
// (xtermjs/xterm.js#265, #4385): macOS intercepts a held key at the OS level to
// show the accent popup, which suppresses the key-repeat event stream the
// terminal expects. Scoping the preference to this bundle identifier leaves
// every other application's behaviour untouched.
//
// The setting is written once (only when not already disabled) and persists
// across runs, so it is a no-op on subsequent launches. It runs asynchronously
// to avoid adding latency to startup.
// openWithSystem opens the file with its default associated application via
// `open`. macOS has no scriptable "open with" picker, so the degrading
// behaviour from the issue discussion applies: no chooser, the file simply
// opens in whatever the desktop currently associates with the extension.
func (a *App) openWithSystem(p string) error {
	if err := exec.Command("open", p).Start(); err != nil {
		return fmt.Errorf("failed to open %s: %w", p, err)
	}
	return nil
}

func (a *App) configureMacKeyRepeat() {
	go func() {
		// Skip the write if it's already disabled to avoid churning the
		// preferences daemon on every launch. `defaults read` prints "0" for a
		// false boolean.
		if out, err := exec.Command("defaults", "read", macBundleID, "ApplePressAndHoldEnabled").Output(); err == nil {
			if strings.TrimSpace(string(out)) == "0" {
				return
			}
		}

		if err := exec.Command("defaults", "write", macBundleID, "ApplePressAndHoldEnabled", "-bool", "false").Run(); err != nil {
			log.Writef("configureMacKeyRepeat: failed to disable ApplePressAndHoldEnabled: %v", err)
			return
		}
		log.Writef("configureMacKeyRepeat: disabled ApplePressAndHoldEnabled for %s (key-repeat enabled)", macBundleID)
	}()
}

// applyRoundedCorners is a no-op on macOS: window corners are handled by the
// platform (the Windows build asks DWM for Win11 rounded corners instead).
func applyRoundedCorners(unsafe.Pointer) {}

// systemPrefersDark reports whether macOS is in dark mode via the same global
// preference (`AppleInterfaceStyle`) WKWebView consults for the CSS
// prefers-color-scheme media query. The key only exists when a dark variant
// is active, so a read error means light mode. Needed because v3's
// IsDarkMode() is unavailable before Run() (see main.go windowBackgroundColour).
func systemPrefersDark() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleInterfaceStyle").Output()
	return err == nil && len(out) > 0
}

// systemLanguage returns the user's first preferred language (System
// Settings → General → Language & Region) as a locale tag for tray-menu
// localization (issue #1215). AppleLanguages is the same global preference
// the system locale resolution is based on; it is an array, so the `defaults`
// output is parsed for the first quoted entry. Returns "" when unreadable —
// the caller falls back to English.
func systemLanguage() string {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if i := strings.IndexByte(line, '"'); i >= 0 {
			if j := strings.LastIndexByte(line, '"'); j > i {
				return line[i+1 : j]
			}
		}
	}
	return ""
}

// spawnSuccessorProcess starts a fresh copy of the app after this process has
// exited (see app_windows.go). On macOS the executable lives inside a .app
// bundle (.../Foo.app/Contents/MacOS/Foo); re-exec'ing that path bypasses
// LaunchServices, so walk up to the bundle and use `open -n` instead. Falls
// back to a raw re-exec when not running from a bundle (e.g. `wails dev`).
func spawnSuccessorProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// .../Foo.app/Contents/MacOS/Foo -> .../Foo.app
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if strings.HasSuffix(bundle, ".app") {
		return exec.Command("open", "-n", bundle).Start()
	}
	return exec.Command(exe).Start()
}

// applyTrayIcon installs the tray icon. macOS expects a template image — a
// monochrome black shape with transparency that the system recolours per menu
// bar appearance — so it gets the dedicated template PNG (see
// appIconTrayTemplatePNG in main.go) via SetTemplateIcon, which maps to
// NSImage.setTemplate:YES.
func applyTrayIcon(tray *application.SystemTray) {
	tray.SetTemplateIcon(appIconTrayTemplatePNG)
}

// trayIconPNG returns the tray icon bytes as-is off Windows: macOS and Linux
// tray implementations take the PNG directly and size it themselves (see
// app_windows.go for the Windows exact-size scaling).
func trayIconPNG() []byte {
	return appIconTrayPNG
}

// Non-Windows stubs: reading the local clipboard's file list (CF_HDROP) has
// no portable implementation, so the IronRDP engine's local→remote file paste
// stays Windows-only; text clipboard is unaffected.

func (a *App) ClipboardGetFiles() []ClipboardFileInfo {
	return []ClipboardFileInfo{}
}

func (a *App) ClipboardReadFileChunk(index int, offset int64, size int) (string, error) {
	return "", fmt.Errorf("clipboard file transfer is only supported on Windows")
}

func (a *App) setClipboardFiles(paths []string) error {
	return fmt.Errorf("clipboard file transfer is only supported on Windows")
}

func (a *App) ClipboardReadFileChunkByID(id string, offset int64, size int) (string, error) {
	return "", fmt.Errorf("clipboard file transfer is only supported on Windows")
}
