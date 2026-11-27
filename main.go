package main

import (
	"embed"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/packetcap"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
)

var Version = "dev"

// devBuild is true for `wails dev` (Version == "dev"); false for production
// builds where `-ldflags '-X main.Version=...'` sets a real version string.
// Used to gate update downloads (app.go) and to pick the dev WebView2
// user-data folder (webview2DataPath below).
var devBuild = Version == "dev"

// webview2DataPath returns the fixed WebView2 user-data folder:
// %LOCALAPPDATA%\uniTerm\webview2, with a "-dev" suffix on dev builds. The
// suffix keeps dev and release instances from contending for the same
// folder — a WebView2 user-data folder only tolerates a single browser
// process tree, and dev/release deliberately use distinct single-instance
// lock IDs (see singleinstance_dev.go) precisely so they can run side by
// side. The fixed folder replaces the old per-PID Temp dir, which was needed
// when multiple instances could run concurrently and accumulated one ~11MB
// dir per launch with no cleanup.
func webview2DataPath() string {
	name := "webview2"
	if devBuild {
		name += "-dev"
	}
	// %LOCALAPPDATA% unset (unexpected, but avoids a bogus relative path).
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return filepath.Join(os.TempDir(), "uniTerm-"+name)
	}
	return filepath.Join(local, "uniTerm", name)
}

// sweepLegacyWebview2Dirs removes per-PID WebView2 Temp dirs
// (uniTerm-webview2-<pid>) left behind by versions that created one per
// launch without ever cleaning them up. Every dir matching the legacy
// pattern is stale by definition — current versions keep their WebView2
// user-data folder fixed and outside Temp — so there is no stale-age check
// (unlike sweepStaleExtEditDirs): a dir newer than the sweep cannot belong
// to a current build. If a legacy build is somehow still running, its locked
// files fail to delete harmlessly. Runs async at startup.
func sweepLegacyWebview2Dirs() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		matched, _ := filepath.Match("uniTerm-webview2-*", name)
		if !e.IsDir() || !matched {
			continue
		}
		// Defensive: never touch this process's own folder (only reachable
		// when %LOCALAPPDATA% is unset and the fallback lands in Temp).
		if filepath.Join(os.TempDir(), name) == webview2DataPath() {
			continue
		}
		_ = os.RemoveAll(filepath.Join(os.TempDir(), name))
	}
}

// indexNoCache serves the root document with no-store so the WebView2 HTTP
// cache can never pin a stale index.html — and with it the old hashed JS
// bundle and WASM engine — after an app update. Every other resource is
// content-hashed, so it self-invalidates on update; only index.html can go
// stale, and the wails asset server sets no cache headers for it.
//
// The root document also gets the persisted theme injected as
// window.__initialTheme: the pre-paint script in index.html otherwise picks
// the theme from the localStorage cache (stale right after a theme switch)
// or the OS preference (independent of the app theme), so the webview's
// first paint could flash a different theme than the native window frame —
// which is painted from the same persisted settings. Serving the resolved
// value from Go makes frame, first webview paint and the loaded UI one
// single source (settings.json). next still serves when the embedded read
// fails; the pre-paint script keeps its cache/OS fallback for the vite dev
// server, which bypasses this middleware entirely.
func indexNoCache(initialTheme string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dev builds: "/" must fall through to the asset server's dev proxy
		// so the webview loads the vite dev server's live index.html. Serving
		// the embedded build-time index here would pin the hashed bundles of
		// the dev build — and every asset request then resolves against vite,
		// which lacks those files and SPA-falls-back to HTML (strict MIME
		// errors, black screen). The theme injection below only matters for
		// the embedded document anyway; the dev index reads its fallbacks.
		if devBuild {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")
			if html, err := assets.ReadFile("frontend/dist/index.html"); err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				// Inject right after <head> opens: the pre-paint theme script
				// lives later in <head> and must see window.__initialTheme
				// when it runs (injecting before </head> was too late — the
				// theme script had already fallen back to matchMedia).
				_, _ = w.Write([]byte(strings.Replace(
					string(html),
					"<head>",
					"<head><script>window.__initialTheme="+strconv.Quote(initialTheme)+"</script>",
					1,
				)))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

//go:embed all:frontend/dist
var assets embed.FS

// appIconTrayPNG is a dedicated tray icon: just the cyan "U" glyph cropped
// from the app icon, pre-scaled to 64px. At the tray's 16-24px sizes the full
// app icon (dark rounded tile + small glyph) turns into an unreadable dark
// blob, and GDI's own downscale of the 1024px icon is mushy — a high-contrast
// glyph filling the canvas stays legible. Scaled at runtime to the exact
// OS small-icon size on Windows (see trayIconPNG in app_windows.go).
//
//go:embed build/appicon_tray.png
var appIconTrayPNG []byte

// appIconTrayTemplatePNG is the macOS variant: the same rounded tile rendered
// as a template image — black tile, "U" knocked out to transparency. macOS
// recolours template images itself (black on a light menu bar, white on a
// dark one), matching the monochrome icons of other apps. Generated from
// appicon_tray.png by knocking the cyan glyph out and flattening the rest to
// black.
//
//go:embed build/appicon_tray_template.png
var appIconTrayTemplatePNG []byte

func main() {
	// Redirect fd 2 to panic-stderr.log when there is no stderr handle
	// (windowsgui release builds). Must run first so it also covers the
	// elevated broker process and any goroutine panic the recover below
	// cannot see. No-op under a console (dev) or explicit redirection.
	log.CaptureStderr()

	// Administrator-shell broker mode: an elevated copy of uniTerm launched
	// via the "runas" verb relays ConPTY I/O for admin local terminals (see
	// backend/session/local_admin_windows.go). Must run before anything else
	// so the broker never creates a window, webview or app store. It also
	// runs before the single-instance lock, which keeps the broker process
	// exempt — only GUI instances compete for the lock.
	if session.RunLocalPtyBroker(os.Args) {
		return
	}

	// Elevated packet-capture worker mode: when a capture starts from a normal
	// (unelevated) uniTerm, the app re-launches itself via "runas" with the
	// --packetcap-worker flag and this worker streams raw packets back over a
	// user-only named pipe. Like the broker it must run before anything else so
	// it never builds a window/webview, and before the single-instance lock so
	// it does not compete with GUI instances.
	if packetcap.ServeWorker(os.Args) {
		return
	}

	// Single-instance relaunch: RelaunchApp/autotest set relaunchPending and
	// quit; the successor is spawned once shutdown releases the lock.

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

	// Fixed folder (not per-PID): the single-instance lock guarantees only one
	// GUI process owns the WebView2 user-data folder; dev and release builds
	// each get their own folder so they can still run side by side. See
	// webview2DataPath above.
	webviewDataPath := webview2DataPath()
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
	// directly here via the data-dir bootstrap (loadSavedSettings). Both loads
	// run in parallel against the same deadline.
	systemTitleBar := false
	winW, winH := 1200, 800 // fallback before any saved geometry is applied
	savedX, savedY := 0, 0
	savedMaxed := false
	savedTheme := ""
	var savedSettings store.AppSettings

	// 400ms rather than 100ms: the window cannot show before WebView2's
	// environment is up (~0.7-1s even warm), so waiting longer for the theme
	// and geometry never delays first paint — while a too-tight deadline made
	// a cold/slow disk silently drop the persisted theme, painting the native
	// frame in the OS-default colour until the webview corrected it.
	deadline := time.After(400 * time.Millisecond)

	settingsCh := make(chan store.AppSettings, 1)
	go func() {
		settingsCh <- loadSavedSettings()
	}()

	// The early geometry read must use the same data dir the app saves to
	// (store.ResolveDataDir), not a hardcoded <UserConfigDir>/uniTerm: in
	// portable mode (exe/data) and custom-dir setups the runtime saves window
	// geometry next to the data dir, while a fixed config-dir read would
	// silently restore the geometry of a differently-located install (or the
	// defaults) on every launch.
	if dd, err := store.ResolveDataDir(); err == nil && dd.Path != "" {
		ls := store.NewLocalStateStore(dd.Path)
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
	case savedSettings = <-settingsCh:
		savedTheme = savedSettings.Theme
	case <-deadline:
		// Too slow — windowBackgroundColour falls back to the dark default.
	}

	// Theme for the injected window.__initialTheme: same resolution as
	// windowBackgroundColour ('' / system → OS preference). Empty only on a
	// cold-disk deadline miss, where the pre-paint script falls back to the
	// localStorage cache / OS preference as before.
	resolvedTheme := savedTheme
	if resolvedTheme == "" || resolvedTheme == "system" {
		if systemPrefersDark() {
			resolvedTheme = "dark"
		} else {
			resolvedTheme = "light"
		}
	}

	// Clean external-edit scratch dirs left behind by previous runs that
	// exited without cleanup (crash / force-kill). Old dirs only: a
	// concurrently running instance keeps its own. Async: it only ever
	// touches dirs older than the stale age, which no new session can
	// collide with (fresh PID + fresh session ID), so startup never waits
	// on it.
	go sweepStaleExtEditDirs()

	// Remove per-PID WebView2 Temp dirs left behind by pre-fixed-folder
	// versions (every launch created one and nothing ever deleted it). All
	// dirs matching the legacy pattern are stale by definition — the fixed
	// folder lives outside Temp. If an old-version instance is somehow still
	// running, its locked files fail to delete harmlessly; retry next launch.
	// Async and self-healing: after the first successful pass the Temp glob
	// matches nothing, so it costs nothing.
	go sweepLegacyWebview2Dirs()

	// Declared before application.New so the single-instance callback (which
	// runs after startup) can capture it.
	var window *application.WebviewWindow

	w3app := application.New(application.Options{
		Name:       "uniTerm",
		Server:     BuildServerOptions(), // empty unless built with -tags server
		Assets:     application.AssetOptions{Handler: indexNoCache(resolvedTheme, application.AssetFileServerFS(assets))},
		OnShutdown: app.shutdown,
		// AppKit's terminate: exits the process without returning from Run().
		// PostShutdown runs after store cleanup and single-instance lock release.
		PostShutdown: func() {
			if runtime.GOOS == "darwin" {
				relaunchIfPending()
			}
		},
		// Single-instance: a second GUI launch acquires the OS lock, notifies
		// this instance (which shows/focuses the existing window) and exits.
		// This prevents two GUI processes from racing on the shared stores in
		// the user's data directory (settings.json, connections, credentials).
		// The admin-shell broker process never reaches application.New, so it
		// is exempt. RelaunchApp sets relaunchPending and quits first; the
		// successor is spawned after shutdown releases the lock.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: singleInstanceID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				showMainWindow(window)
			},
		},
		// WebviewUserDataPath is a Windows-only path for WebView2 user data; it is
		// harmless (ignored) on other platforms.
		Windows: application.WindowsOptions{
			WebviewUserDataPath: webviewDataPath,
			// WebView2 runtimes 152.x/153.x call LogonUser(current local
			// account, invalid password) once per environment creation at
			// startup — a Chromium regression in the autofill-AI-wallet
			// feature that emits a 4625 failed-logon event per launch and,
			// on machines with an account lockout policy, can lock the
			// user out of Windows entirely. Disabling that feature stops
			// the logon attempt. Must go through DisabledFeatures so it
			// merges into Wails' single --disable-features switch:
			// Chromium honours only the LAST occurrence, so a second
			// switch in AdditionalBrowserArgs would silently re-enable
			// Wails' disabled SmartScreen protection.
			DisabledFeatures: []string{"AutofillAiWalletPrivatePasses"},
		},
		// Fixed program name so the window's X11 WM_CLASS stays "uniterm" —
		// the StartupWMClass in the .desktop file matches it under X11/XWayland.
		// NOTE: on native Wayland this does NOT control the taskbar association:
		// the XDG app_id comes from the GtkApplication id, which Wails derives
		// as "org.wails." + lowercase Options.Name → "org.wails.uniterm". GNOME
		// matches Wayland windows to launchers by desktop-file filename, so the
		// installed .desktop must be named org.wails.uniterm.desktop (see
		// build/linux/Taskfile.yml generate:dotdesktop).
		Linux: application.LinuxOptions{
			ProgramName: "uniterm",
		},
	})

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

	window = w3app.Window.NewWithOptions(additionalWindowOptions(application.WebviewWindowOptions{
		Title:           "uniTerm",
		Width:           winW,
		Height:          winH,
		X:               savedX,
		Y:               savedY,
		InitialPosition: startPos,
		StartState:      startState,
		MinWidth:        500,
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
	}))

	// The screen list is empty until Run() populates it (see #1335), so the
	// saved rect can only be validated here, on first show: a window restored
	// on a since-disconnected monitor is moved back onto the primary instead of
	// staying out of reach.
	window.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
		x, y := window.Position()
		w, h := window.Size()
		if windowGeometryVisible(x, y, w, h, w3app.Screen.GetAll()) {
			return
		}
		if screen := w3app.Screen.GetPrimary(); screen != nil {
			wa := screen.WorkArea
			window.SetPosition(wa.X+(wa.Width-w)/2, wa.Y+(wa.Height-h)/2)
		} else {
			window.Center()
		}
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

	// Native close paths (Alt+F4, taskkill, any external WM_CLOSE) close the
	// window WITHOUT running the application quit flow: wails v3 on Windows
	// only runs cleanup()/OnShutdown when App.Quit is called explicitly (the
	// in-app close button and the tray Quit item do that). The window then
	// merely unregisters, Run() returns, and main() calls os.Exit(0) with the
	// WebView2 controller still alive — the DLL-detach teardown inside
	// ExitProcess blocks on the browser subprocess, leaving a frozen residual
	// process that still holds the single-instance lock, so relaunching fails.
	// Cancel the native close and reroute it through the same
	// full quit path as the in-app close button. The hook must run on Windows
	// only: on macOS Cmd+Q terminates via AppKit's terminate (cleanup runs),
	// and on Linux the GTK delete-event handler calls cleanup directly.
	if runtime.GOOS == "windows" {
		window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			// Already quitting (App.shutdown started, or this close comes from
			// cleanup()'s own window.Close()): let the default close listener
			// proceed so the window actually closes during teardown.
			if app.quitting.Load() {
				return
			}
			event.Cancel()
			go func() {
				app.saveWindowStateFromRuntime()
				app.app.Quit()
			}()
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

	// Browser streaming SFTP/SCP + file staging routes (/api/web/*) exist only
	// in the headless server build; a no-op on desktop.
	registerWebStreamService(w3app, app)

	// System notification service: app_<os>.go notifyMCPApproval posts MCP
	// approval prompts through it when the window is unfocused. Windows needs
	// the service's Startup to run so the toast AUMID/registry side is set up;
	// macOS needs it for UNUserNotificationCenter authorization handling.
	//
	// Not registered on Android: the wails notifications service has no
	// Android backend, so on GOOS=android it compiles the linux D-Bus
	// implementation whose Startup fails (there is no session bus) and aborts
	// w3app.Run before platformRun signals the app ready — leaving the
	// WebView stuck on a blank wails.localhost error page. notifyMCPApproval
	// already handles a nil notifier.
	var mcpNotifier *notifications.NotificationService
	if runtime.GOOS != "android" && !serverMode {
		mcpNotifier = notifications.New()
		w3app.RegisterService(application.NewService(mcpNotifier))
	}
	app.notifier = mcpNotifier

	// System tray (issue #982): persistent icon with left-click show/hide
	// toggle and a menu (show / hide / reset position / settings / about /
	// quit). Menu labels follow the persisted UI language; English is the
	// fallback for unknown codes. Tray creation must happen after the window
	// exists so the click handlers can drive it. Skipped entirely when the
	// user turned the icon off; created lazily if re-enabled at runtime
	// (see applyTrayVisibility).
	if trayIconEnabled(&savedSettings) || minimizeToTrayEnabled(&savedSettings) {
		setupTray(w3app, app, window, savedSettings.Language)
	}

	// Global show/hide hotkey (issue #982, Windows + platform backends via
	// the wails GlobalShortcutManager). Default Ctrl+M when unset; can be
	// changed or disabled in Settings → Shortcuts.
	applyGlobalShowHideHotkey(w3app, window, trayHotkeyBinding(&savedSettings))

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

	// Windows: the window is deliberately NOT shown early here (unlike the
	// darwin commit-based show above). A WebView2 controller presents nothing
	// until an IsVisible false→true cycle runs (wails does it at
	// NavigationCompleted), and a window visible during that cycle shows the
	// torn surface as a black frame — measured alternatives (skip, position-
	// notify, resize kicks, earlier kicks) either never present or present
	// black for the whole load. The stock timing shows the window with fully
	// rendered content in one step, no flash.

	err := w3app.Run()
	if err != nil {
		log.Writef("Wails run error: %v", err)
	}

	// Quit-then-spawn relaunch (RelaunchApp / update autotest): the process
	// is exiting and the single-instance lock is released, so the successor
	// starts clean — it would otherwise be rejected as a "second instance".
	relaunchIfPending()
	// Graceful exit. shutdown() (see app.go) has already released the WebView2
	// COM controller synchronously on the UI thread. ICoreWebView2Controller::Close()
	// blocks until the controller and, being the last one on the environment, the
	// WebView2 browser subprocess are torn down, so by the time we reach ExitProcess
	// the WebView2 DLL-detach teardown has nothing live to block on — the process
	// exits immediately, no frozen window. All user-facing teardown (local
	// servers, sessions, tunnels, webview data dir) already ran synchronously in
	// shutdown().
	os.Exit(0)
}

// windowGeometryVisible reports whether the rect (x, y, w, h) overlaps at least
// one screen's WorkArea by the 100x50 px a user needs to grab the window. An
// empty screen list (headless/server backend) reports true, preserving the
// pre-#1335 behavior instead of centering on every launch.
func windowGeometryVisible(x, y, w, h int, screens []*application.Screen) bool {
	if len(screens) == 0 {
		return true
	}
	if w <= 0 || h <= 0 {
		return false
	}
	for _, s := range screens {
		wa := s.WorkArea
		overlapW := min(x+w, wa.X+wa.Width) - max(x, wa.X)
		overlapH := min(y+h, wa.Y+wa.Height) - max(y, wa.Y)
		if overlapW >= min(100, w) && overlapH >= min(50, h) {
			return true
		}
	}
	return false
}

// loadSavedSettings reads the persisted app settings so the window can be
// created with a background colour matching the theme (see
// windowBackgroundColour), the tray menu can follow the UI language, and the
// global hotkey can be registered before Run(). The settings store normally
// initializes later in ServiceStartup, so this goes through the data-dir
// bootstrap directly. Returns the zero value when unavailable (first run /
// read error) — the caller maps that to the dark theme default and Ctrl+M.
func loadSavedSettings() store.AppSettings {
	dd, err := store.ResolveDataDir()
	if err != nil || dd.FirstRun || dd.Path == "" {
		return store.AppSettings{}
	}
	// ResolveDataDir only returns paths that exist (bootstrap paths are
	// validated, the upgrade path is checked for config files), so
	// NewSettingsStore's MkdirAll is a no-op here.
	ss, err := store.NewSettingsStore(dd.Path)
	if err != nil {
		return store.AppSettings{}
	}
	settings, err := ss.Load()
	if err != nil {
		return store.AppSettings{}
	}
	return settings
}

// windowBackgroundColour maps the persisted app theme to the native window
// background colour. That colour is only visible before the webview's first
// paint, so it is matched to each theme's --bg-base in
// frontend/src/style.css — the base surface the loaded UI settles on, so
// first paint and the loaded UI form one seamless surface. 'system' is
// resolved from the OS the same way the webview engine does
// (systemtheme_*.go); v3's IsDarkMode() can't be used because it reports
// false before Run().
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
		return application.RGBA{Red: 11, Green: 19, Blue: 34, Alpha: 255} // #0b1322
	case "light":
		return application.RGBA{Red: 255, Green: 255, Blue: 255, Alpha: 255} // #ffffff
	default:
		return application.RGBA{Red: 16, Green: 16, Blue: 16, Alpha: 255} // #101010
	}
}

// System tray support (issue #982): a persistent tray icon with a left-click
// show/hide toggle and a small right-click menu. The tray is what makes the
// window "hide to the notification area" recoverable — without it a hidden
// window could only be brought back via the global hotkey.

// trayLabels carries the tray menu strings for every UI language the
// frontend offers. The Go side can't use the frontend's i18n bundles, so the
// labels live here, keyed by the settings language code; unknown codes fall
// back to English.
var trayLabels = map[string][6]string{
	// show, hide, reset, settings, about, quit
	"en":    {"Show Main Window", "Hide to Tray", "Reset Window Position", "Settings", "About", "Quit"},
	"zh-CN": {"显示主窗口", "隐藏到托盘", "重置窗口位置", "设置", "关于", "退出"},
	"zh-TW": {"顯示主視窗", "隱藏到系統匣", "重置視窗位置", "設定", "關於", "結束"},
	"ja":    {"メインウィンドウを表示", "トレイに隠す", "ウィンドウ位置をリセット", "設定", "バージョン情報", "終了"},
	"ko":    {"메인 창 표시", "트레이로 숨기기", "창 위치 재설정", "설정", "정보", "종료"},
	"de":    {"Hauptfenster anzeigen", "In den Tray minimieren", "Fensterposition zurücksetzen", "Einstellungen", "Über", "Beenden"},
	"es":    {"Mostrar ventana principal", "Ocultar en la bandeja", "Restablecer posición", "Ajustes", "Acerca de", "Salir"},
	"fr":    {"Afficher la fenêtre", "Réduire dans la zone de notification", "Réinitialiser la position", "Paramètres", "À propos", "Quitter"},
	"ru":    {"Показать главное окно", "Свернуть в трей", "Сбросить положение окна", "Настройки", "О программе", "Выход"},
	"pt":    {"Mostrar janela principal", "Ocultar na bandeja", "Redefinir posição da janela", "Configurações", "Sobre", "Sair"},
	"vi":    {"Hiện cửa sổ chính", "Ẩn vào khay hệ thống", "Đặt lại vị trí cửa sổ", "Cài đặt", "Giới thiệu", "Thoát"},
}

func trayLabel(lang string, idx int) string {
	if labels, ok := trayLabels[lang]; ok {
		return labels[idx]
	}
	return trayLabels["en"][idx]
}

// resolveTrayLanguage maps the persisted settings language to one of the
// trayLabels keys. "system" (and empty — the unset default) resolves to the
// OS UI language so the tray follows the system locale (issue #1215),
// mirroring the frontend's navigator.language resolution; anything unmapped
// falls back to English.
func resolveTrayLanguage(lang string) string {
	if lang != "" && lang != "system" {
		return normalizeLanguageCode(lang)
	}
	if sys := systemLanguage(); sys != "" {
		return normalizeLanguageCode(sys)
	}
	return "en"
}

// normalizeLanguageCode matches a raw locale tag ("zh-TW", "zh-cn", "pt-BR",
// "zh_CN.UTF-8", "zh-Hans-CN", ...) against the languages the tray offers,
// using the same rules as the frontend's resolveLocale: zh variants split
// between Simplified and Traditional, everything else matches by the primary
// language subtag. Unmatched tags fall back to English.
func normalizeLanguageCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(code, "_", "-")))
	// Strip encoding and modifier suffixes ("zh-cn.utf-8", "zh-cn@rg=...").
	if i := strings.IndexAny(code, ".@"); i >= 0 {
		code = code[:i]
	}
	if strings.HasPrefix(code, "zh") {
		if strings.Contains(code, "tw") || strings.Contains(code, "hk") ||
			strings.Contains(code, "mo") || strings.Contains(code, "hant") {
			return "zh-TW"
		}
		return "zh-CN"
	}
	if i := strings.IndexByte(code, '-'); i >= 0 {
		code = code[:i]
	}
	if _, ok := trayLabels[code]; ok {
		return code
	}
	return "en"
}

const (
	trayShow  = 0
	trayHide  = 1
	trayReset = 2
	traySet   = 3
	trayAbout = 4
	trayQuit  = 5
)

// setupTray creates the system tray icon and menu. lang is the persisted UI
// language (from settings.json) used to localize the menu labels. "system"
// resolves to the OS UI language (issue #1215), and the menu is rebuilt when
// the user changes the UI language at runtime (see refreshTrayLanguage).
func setupTray(w3app *application.App, app *App, window *application.WebviewWindow, lang string) {
	tray := w3app.SystemTray.New()
	applyTrayIcon(tray)
	tray.SetTooltip("uniTerm")

	app.tray = tray
	app.trayLang = resolveTrayLanguage(lang)
	tray.SetMenu(buildTrayMenu(app, app.trayLang))

	// Left click toggles visibility, mirroring MobaXterm's tray behaviour.
	tray.OnClick(func() {
		if window.IsVisible() {
			window.Hide()
		} else {
			showMainWindow(window)
		}
	})

	tray.Show()
}

// buildTrayMenu assembles the tray context menu in the given resolved UI
// language, with the same handlers as the initial setup. Rebuilt and
// re-applied via SystemTray.SetMenu when the user changes the UI language at
// runtime (issue #1215); reads a.app/a.window so the handlers keep working
// after the rebuild.
func buildTrayMenu(app *App, lang string) *application.Menu {
	menu := application.NewMenu()
	menu.Add(trayLabel(lang, trayShow)).OnClick(func(*application.Context) {
		showMainWindow(app.window)
	})
	menu.Add(trayLabel(lang, trayHide)).OnClick(func(*application.Context) {
		app.window.Hide()
	})
	menu.AddSeparator()
	menu.Add(trayLabel(lang, trayReset)).OnClick(func(*application.Context) {
		app.resetWindowGeometry()
	})
	menu.AddSeparator()
	menu.Add(trayLabel(lang, traySet)).OnClick(func(*application.Context) {
		showMainWindow(app.window)
		app.emit("app:open-settings")
	})
	menu.AddSeparator()
	menu.Add(trayLabel(lang, trayAbout)).OnClick(func(*application.Context) {
		showMainWindow(app.window)
		app.emit("app:open-about")
	})
	menu.AddSeparator()
	menu.Add(trayLabel(lang, trayQuit)).OnClick(func(*application.Context) {
		app.app.Quit()
	})
	return menu
}

// trayIconEnabled reports whether the tray icon should be shown. Nil (older
// settings.json without the field) means on.
func trayIconEnabled(s *store.AppSettings) bool {
	return s.ShowTrayIcon == nil || *s.ShowTrayIcon
}

// minimizeToTrayEnabled reports whether the minimize button should hide the
// window into the tray instead of the taskbar (issue #1556). Nil (older
// settings.json without the field) means off — minimise as usual.
func minimizeToTrayEnabled(s *store.AppSettings) bool {
	return s.MinimizeToTray != nil && *s.MinimizeToTray
}

// applyTrayVisibility shows or hides the tray icon when the setting changes
// at runtime. When the app started with the icon disabled no tray exists yet,
// so the first enable creates it.
func (a *App) applyTrayVisibility(show bool, lang string) {
	if a.app == nil {
		return
	}
	switch {
	case !show:
		if a.tray != nil {
			a.tray.Hide()
		}
	case a.tray == nil:
		setupTray(a.app, a, a.window, lang)
	default:
		a.tray.Show()
	}
}

// refreshTrayLanguage rebuilds the tray menu when the persisted UI language
// changed since the menu was last built (issue #1215). Called from
// SaveSettings so runtime language switches reach the tray without a
// restart; a no-op when the resolved language is unchanged (e.g. switching
// between "system" and the locale the system already resolves to).
func (a *App) refreshTrayLanguage(lang string) {
	if a.tray == nil {
		return
	}
	resolved := resolveTrayLanguage(lang)
	if resolved == a.trayLang {
		return
	}
	a.trayLang = resolved
	a.tray.SetMenu(buildTrayMenu(a, resolved))
}

// showMainWindow brings the window back from hidden/minimised state and
// gives it focus. Safe to call from any goroutine — the wails v3 window
// methods dispatch to the main thread internally.
func showMainWindow(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	if !window.IsVisible() {
		window.Show()
	}
	window.UnMinimise()
	window.Focus()
}

// toggleMainWindow shows the window when hidden (or minimised), hides it
// otherwise.
func toggleMainWindow(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	if window.IsVisible() {
		window.Hide()
	} else {
		showMainWindow(window)
	}
}

// Global show/hide hotkey state. applyGlobalShowHideHotkey swaps the
// registered accelerator; the currently registered string is tracked here so
// unrelated settings saves don't re-register (which would briefly release
// the OS-wide binding). Access is mutex-guarded: SaveSettings runs on a
// binding goroutine while startup applies the initial binding from main.
var (
	trayHotkeyMu      sync.Mutex
	trayHotkeyCurrent string
)

// applyGlobalShowHideHotkey registers (or re-registers) the OS-global
// show/hide shortcut via the wails GlobalShortcutManager, which handles the
// platform backends (RegisterHotKey on Windows, Carbon on macOS, X11 on
// Linux). A nil binding means the default Ctrl+M; a binding with an empty
// key (the UI's "cleared" state) unregisters. Failures (the combo already
// owned by another app) are logged and leave no binding registered.
func applyGlobalShowHideHotkey(w3app *application.App, window *application.WebviewWindow, binding *store.KeyBinding) {
	if w3app == nil {
		return
	}
	trayHotkeyMu.Lock()
	defer trayHotkeyMu.Unlock()

	if trayHotkeyCurrent != "" {
		if err := w3app.GlobalShortcut.Unregister(trayHotkeyCurrent); err != nil {
			log.Writef("global hotkey: unregister %q failed: %v", trayHotkeyCurrent, err)
		}
		trayHotkeyCurrent = ""
	}
	if binding == nil {
		binding = &store.KeyBinding{Ctrl: true, Key: "m"}
	}
	accel := keyBindingAccelerator(binding)
	if accel == "" {
		// Empty key = the UI's "cleared" state — stays unregistered.
		return
	}
	if err := w3app.GlobalShortcut.Register(accel, func() {
		toggleMainWindow(window)
	}); err != nil {
		log.Writef("global hotkey: register %q failed: %v", accel, err)
		return
	}
	trayHotkeyCurrent = accel
}

// keyBindingAccelerator renders a frontend KeyBinding as a wails accelerator
// string ("Ctrl+Alt+M"). Empty string when the binding has no key.
func keyBindingAccelerator(b *store.KeyBinding) string {
	key := strings.ToLower(strings.TrimSpace(b.Key))
	if key == "" {
		return ""
	}
	var parts []string
	if b.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if b.Alt {
		parts = append(parts, "Alt")
	}
	if b.Shift {
		parts = append(parts, "Shift")
	}
	if b.Meta {
		parts = append(parts, "Super")
	}
	return strings.Join(append(parts, key), "+")
}

// trayHotkeyBinding extracts the global show/hide hotkey from the keyboard
// settings map ("trayShowHide" key, same shape as the in-app bindings).
// Absent = default Ctrl+M; present-with-empty-key = disabled.
func trayHotkeyBinding(s *store.AppSettings) *store.KeyBinding {
	if b, ok := s.Keyboard["trayShowHide"]; ok {
		return &b
	}
	return nil
}
