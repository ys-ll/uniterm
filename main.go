package main

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	// F-201: register pprof handlers on the default mux.
	_ "net/http/pprof"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
)

var Version = "dev"

// devBuild is true for `wails dev` (Version == "dev"); false for production
// builds where `-ldflags '-X main.Version=...'` sets a real version string.
// Used to gate the pprof HTTP listener so production binaries don't open it.
var devBuild = Version == "dev"

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Administrator-shell broker mode: an elevated copy of uniTerm launched
	// via the "runas" verb relays ConPTY I/O for admin local terminals (see
	// backend/session/local_admin_windows.go). Must run before anything else
	// so the broker never creates a window, webview or app store.
	if session.RunLocalPtyBroker(os.Args) {
		return
	}

	// Capture top-level panics
	defer func() {
		if r := recover(); r != nil {
			_ = log.Init()
			log.Writef("FATAL PANIC: %v\n%s", r, string(debug.Stack()))
			log.Close()
			os.Exit(1)
		}
	}()

	if err := log.Init(); err != nil {
		println("Failed to init log:", err.Error())
	}
	defer log.Close()

	// F-201: expose net/http/pprof on localhost:6060 for dev builds only.
	// Production builds (wails build) leave Version unchanged from "dev"
	// unless ldflags set it; gate behind a build flag so production does
	// not open a listener.
	startPprofIfDev()

	webviewDataPath := filepath.Join(os.TempDir(), fmt.Sprintf("uniTerm-webview2-%d", os.Getpid()))
	os.MkdirAll(webviewDataPath, 0700)

	app := NewApp(webviewDataPath)

	// Read persisted window geometry and theme before creating the window —
	// both are fixed at creation in v3 (services start before the window
	// exists, so ServiceStartup can't position it). Race the loads against a
	// short deadline so a slow disk doesn't delay first paint; on timeout the
	// defaults (1200x800 centered, dark background) are used instead.
	//
	// Geometry comes from localState.json. The theme comes from settings.json,
	// whose store normally starts later in ServiceStartup, so it is loaded
	// directly here via the data-dir bootstrap (loadSavedTheme). Both loads
	// run in parallel against the same deadline.
	systemTitleBar := false
	winW, winH := 1200, 800 // fallback before any saved geometry is applied
	savedX, savedY := 0, 0
	savedMaxed := false
	savedTheme := ""

	deadline := time.After(100 * time.Millisecond)

	themeCh := make(chan string, 1)
	go func() {
		themeCh <- loadSavedTheme()
	}()

	if configDir, err := os.UserConfigDir(); err == nil {
		ls := store.NewLocalStateStore(filepath.Join(configDir, "uniTerm"))
		done := make(chan store.LocalState, 1)
		go func() {
			if state, err := ls.Load(); err == nil {
				done <- state
				return
			}
			done <- store.LocalState{}
		}()
		select {
		case state := <-done:
			systemTitleBar = state.SystemTitleBar
			if state.WindowWidth > 0 && state.WindowHeight > 0 {
				winW, winH = state.WindowWidth, state.WindowHeight
			}
			savedX, savedY = state.WindowX, state.WindowY
			savedMaxed = state.WindowMaximised
		case <-deadline:
			// Slow disk — paint the defaults. The goroutine continues to load
			// in the background; its result is discarded because the window
			// geometry options are fixed at startup.
		}
	}

	select {
	case savedTheme = <-themeCh:
	case <-deadline:
		// Too slow — windowBackgroundColour falls back to the dark default.
	}

	// Restore a saved position only when one actually exists; otherwise keep v3's
	// default (centered) so a fresh install doesn't land at (0,0).
	startPos := application.WindowCentered
	if savedX != 0 || savedY != 0 {
		startPos = application.WindowXY
	}
	startState := application.WindowStateNormal
	if savedMaxed {
		startState = application.WindowStateMaximised
	}

	// Clean external-edit scratch dirs left behind by previous runs that
	// exited without cleanup (crash / force-kill). Old dirs only: a
	// concurrently running instance keeps its own. Async: it only ever
	// touches dirs older than the stale age, which no new session can
	// collide with (fresh PID + fresh session ID), so startup never waits
	// on it.
	go sweepStaleExtEditDirs()

	w3app := application.New(application.Options{
		Name:       "uniTerm",
		Assets:     application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		OnShutdown: app.shutdown,
		// WebviewUserDataPath is a Windows-only path for WebView2 user data; it is
		// harmless (ignored) on other platforms.
		Windows: application.WindowsOptions{
			WebviewUserDataPath: webviewDataPath,
		},
		// Fixed program name so the window's WM_CLASS stays "uniterm" — the
		// installed package's .desktop file sets StartupWMClass to the same
		// value, which is what lets the dock/taskbar associate the running
		// window with the app icon.
		Linux: application.LinuxOptions{
			ProgramName: "uniterm",
		},
	})

	// On macOS, install the standard App + Edit menus. This must run AFTER
	// application.New(): menu roles dereference the global app instance and
	// panic with a nil pointer otherwise (macOS-only — NewAppMenu returns nil
	// on other platforms, which masked the crash). The Edit menu is what
	// routes the native Cmd+C/V/X/A/Z shortcuts to the first responder — every
	// WKWebView text field (input/textarea/contenteditable) relies on it. An
	// empty menu here used to suppress Wails' defaults but also killed those
	// shortcuts app-wide, forcing per-component JS reimplementations. The menu
	// lives in the top system menu bar, so it doesn't affect the frameless
	// window. On Linux (GTK) a non-nil Menu creates an empty GtkMenuBar that
	// shows as a thin white line in the frameless window, so leave it nil
	// there. See issue #291.
	var appMenu *application.Menu
	if runtime.GOOS == "darwin" {
		appMenu = application.NewMenu()
		appMenu.AddRole(application.AppMenu)
		appMenu.AddRole(application.EditMenu)
	}

	if appMenu != nil {
		w3app.Menu.SetApplicationMenu(appMenu)
	}

	window := w3app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           "uniTerm",
		Width:           winW,
		Height:          winH,
		X:               savedX,
		Y:               savedY,
		InitialPosition: startPos,
		StartState:      startState,
		MinWidth:        700,
		MinHeight:       450,
		// Headless local update e2e runs must not flash a window.
		Hidden:           os.Getenv("UNITERM_UPDATE_AUTOTEST") == "1",
		Frameless:        !systemTitleBar,
		BackgroundColour: windowBackgroundColour(savedTheme),
		EnableFileDrop:   true,
		// Open the WebView2 inspector when the window is first shown. Wails
		// disables browser accelerator keys on Windows (F12 / Ctrl+Shift+I
		// never reach us), and the app's custom context menus hide the
		// default "Inspect" entry — without this there is NO way to open
		// DevTools, even in dev builds. Inert in production: the wails
		// runtime only honors it when built without the `production` tag.
		// (Deliberately NOT a window KeyBinding for F12: that would be
		// window-global and could swallow F12 from TUI apps.)
		OpenInspectorOnStartup: true,
	})

	// Win11 rounded corners: Wails only extends the DWM frame from its
	// WM_ACTIVATE handler, whose first firing (inside CreateWindowEx, before
	// its WndProc is hooked) is missed — so a production binary starts with
	// square corners until the next activation (minimise/restore). Ask DWM
	// for rounded corners on first show instead; see win11_corners_windows.go.
	// WindowShow reliably fires after WebView2 navigation completes, when the
	// native window exists. Idempotent; a no-op on pre-Win11 and other OSes.
	if runtime.GOOS == "windows" {
		window.OnWindowEvent(events.Windows.WindowShow, func(*application.WindowEvent) {
			applyRoundedCorners(window.NativeWindow())
		})
	}

	// Wails v3 delivers OS file drops to Go-side window-event listeners rather
	// than (as v2 did) auto-forwarding them to the frontend. Re-emit the dropped
	// absolute paths under the original v2 event name so `Events.On(...)` pickers
	// (FileSidebar, SFTP tab) keep receiving them for path-based upload.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		filenames := event.Context().DroppedFiles()
		if len(filenames) == 0 {
			return
		}
		x, y, elementID := 0, 0, ""
		if details := event.Context().DropTargetDetails(); details != nil {
			x, y = details.X, details.Y
			elementID = details.ElementID
		}
		w3app.Event.Emit("common:WindowFilesDropped", map[string]any{
			"x":         x,
			"y":         y,
			"elementId": elementID,
			"filenames": filenames,
		})
	})

	// Wire the bound App back to the runtime before registering it as a service:
	// emit() / window operations route through these references.
	app.app = w3app
	app.window = window
	w3app.RegisterService(application.NewService(app))

	// Local end-to-end update test hook — inert unless the env var is set
	// (see autotest_update.go).
	if os.Getenv("UNITERM_UPDATE_AUTOTEST") == "1" {
		go app.autotestUpdate()
	}

	// Show the window as soon as the page's DOM is committed (content starts
	// rendering) instead of waiting for Wails' default, which defers Show until
	// WebViewDidFinishNavigation — i.e. after the multi-MB bundle is parsed and
	// executed. On macOS the window is created hidden and only shown via that
	// late callback, so users see the dock icon for seconds before anything
	// appears. Commit fires well before that; the body background (dark in
	// style.css) is already the right colour, so the early show paints a clean
	// dark window instead of white. Finish-navigation still runs its own
	// Show(), which is idempotent.
	window.OnWindowEvent(events.Mac.WebViewDidCommitNavigation, func(*application.WindowEvent) {
		if os.Getenv("UNITERM_UPDATE_AUTOTEST") != "1" {
			window.Show()
		}
	})

	err := w3app.Run()
	if err != nil {
		log.Writef("Wails run error: %v", err)
	}
}

// loadSavedTheme reads the persisted app theme so the window can be created
// with a background colour matching it (see windowBackgroundColour). The
// settings store normally initializes later in ServiceStartup, so this goes
// through the data-dir bootstrap directly. Returns "" when unavailable
// (first run / read error) — the caller maps that to the dark default.
func loadSavedTheme() string {
	dd, err := store.ResolveDataDir()
	if err != nil || dd.FirstRun || dd.Path == "" {
		return ""
	}
	// ResolveDataDir only returns paths that exist (bootstrap paths are
	// validated, the upgrade path is checked for config files), so
	// NewSettingsStore's MkdirAll is a no-op here.
	ss, err := store.NewSettingsStore(dd.Path)
	if err != nil {
		return ""
	}
	settings, err := ss.Load()
	if err != nil {
		return ""
	}
	return settings.Theme
}

// windowBackgroundColour maps the persisted app theme to the native window
// background colour. That colour is only visible before the webview's first
// paint, so it is matched to the TOP colour of each theme's body gradient in
// frontend/src/style.css — the seam that would otherwise flash in the wrong
// colour on startup. 'system' is resolved from the OS the same way the
// webview engine does (systemtheme_*.go); v3's IsDarkMode() can't be used
// because it reports false before Run().
func windowBackgroundColour(theme string) application.RGBA {
	resolved := theme
	if theme == "" || theme == "system" {
		if systemPrefersDark() {
			resolved = "dark"
		} else {
			resolved = "light"
		}
	}
	switch resolved {
	case "deep-blue":
		return application.RGBA{Red: 16, Green: 23, Blue: 40, Alpha: 255} // #101728
	case "light":
		return application.RGBA{Red: 255, Green: 255, Blue: 255, Alpha: 255} // #ffffff
	default:
		return application.RGBA{Red: 27, Green: 31, Blue: 39, Alpha: 255} // #1b1f27
	}
}

// startPprofIfDev spawns a goroutine that serves net/http/pprof on
// localhost:6060 — only when running a dev build. Production builds
// (Version != "dev") deliberately skip this so end-users never have
// the debug listener open.
//
// The listener stays up for the lifetime of the process; its only job
// is to let `go tool pprof http://localhost:6060/debug/pprof/profile`
// connect and capture CPU/heap/block/goroutine profiles during
// reproduction of perf issues (see F-201 / audit §8.2).
func startPprofIfDev() {
	if !devBuild {
		return
	}
	go func() {
		if err := http.ListenAndServe("localhost:6060", nil); err != nil && err != http.ErrServerClosed {
			log.Writef("pprof listener failed: %v", err)
		}
	}()
}
