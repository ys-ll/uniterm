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

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/ys-ll/uniterm/backend/log"
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

	// Linux multi-monitor maximize workaround:
	// Wails sets default max size to primary display, which can clamp
	// maximize on secondary monitors. Set to large values to disable.
	// See: https://github.com/wailsapp/wails/issues/2431
	maxW, maxH := 0, 0
	if runtime.GOOS == "linux" {
		maxW, maxH = 9999, 9999
	}

	// On macOS, install the standard App + Edit menus. The Edit menu is what
	// routes the native Cmd+C/V/X/A/Z shortcuts to the first responder — every
	// WKWebView text field (input/textarea/contenteditable) relies on it. An
	// empty menu here used to suppress Wails' defaults but also killed those
	// shortcuts app-wide, forcing per-component JS reimplementations. The menu
	// lives in the top system menu bar, so it doesn't affect the frameless
	// window. On Linux (GTK) a non-nil Menu creates an empty GtkMenuBar that
	// shows as a thin white line in the frameless window, so leave it nil
	// there. See issue #291.
	var appMenu *menu.Menu
	if runtime.GOOS == "darwin" {
		appMenu = menu.NewMenuFromItems(
			menu.AppMenu(),
			menu.EditMenu(),
		)
	}

	// Read the persisted window-frame preference before wails.Run — the frame
	// style is fixed at startup and can't be toggled at runtime.
	//
	// F-410: the previous version did the Load() synchronously, so a slow
	// home directory (network share, antivirus scan, locked file) could stall
	// main() and delay the entire first paint. Default to the frameless
	// preference (systemTitleBar=false) and race the disk load against a
	// short timeout; on a slow disk we paint the default, on a fast disk
	// we use the persisted value. The background goroutine's result is
	// discarded — it's purely there to absorb the disk latency.
	systemTitleBar := false
	if configDir, err := os.UserConfigDir(); err == nil {
		ls := store.NewLocalStateStore(filepath.Join(configDir, "uniTerm"))
		done := make(chan bool, 1)
		go func() {
			if state, err := ls.Load(); err == nil {
				done <- state.SystemTitleBar
				return
			}
			done <- false
		}()
		select {
		case v := <-done:
			systemTitleBar = v
		case <-time.After(100 * time.Millisecond):
			// Slow disk — paint the default. The goroutine continues to load
			// in the background; its result is discarded because the
			// window-frame option is fixed at startup.
		}
	}

	macTitleBar := mac.TitleBarHiddenInset()
	if systemTitleBar {
		macTitleBar = mac.TitleBarDefault()
	}

	err := wails.Run(&options.App{
		Title:     "uniTerm",
		Width:     1200,
		Height:    800,
		MinWidth:  400,
		MinHeight: 300,
		MaxWidth:  maxW,
		MaxHeight: maxH,
		Frameless: runtime.GOOS != "darwin" && !systemTitleBar,
		Menu:      appMenu,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Mac: &mac.Options{
			TitleBar: macTitleBar,
		},
		Windows: &windows.Options{
			WebviewUserDataPath: webviewDataPath,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		fmt.Println("Error:", err.Error())
		log.Writef("Wails run error: %v", err)
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
