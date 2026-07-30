package main

import (
	"embed"
	"encoding/json"
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
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/ys-ll/uniterm/backend/diag"
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

	// Initialize the structured diagnostic logger. Settings are read from
	// the user's persisted diag config; falls back to defaults on error.
	if dir, err := os.UserHomeDir(); err == nil {
		logDir := filepath.Join(dir, ".uniterm", "logs")
		diagCfg := diagConfigFromSettings()
		if err := diag.Init(logDir, diagCfg); err != nil {
			println("Failed to init diag:", err.Error())
		}
	}

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
		// Match :root --bg-base in frontend/src/style.css so the window
		// background blends with the webview first paint (no flash).
		BackgroundColour: &options.RGBA{R: 0x14, G: 0x17, B: 0x1d, A: 1},
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
//
// /debug/diag/metrics serves the in-process diag metrics registry as
// JSON, so dev can `curl http://localhost:6060/debug/diag/metrics` to
// eyeball per-op percentile sketches without round-tripping through
// the Vue viewer.
func startPprofIfDev() {
	if !devBuild {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/diag/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(diag.Snapshot())
	})
	// pprof handlers self-register on http.DefaultServeMux (from the
	// blank import of net/http/pprof above); expose that handler tree
	// under /debug/pprof/ on our own mux.
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
	go func() {
		if err := http.ListenAndServe("localhost:6060", mux); err != nil && err != http.ErrServerClosed {
			log.Writef("pprof listener failed: %v", err)
		}
	}()
}

// diagConfigFromSettings reads the persisted diag config and converts it
// to a *diag.DiagConfig. Falls back to defaults on any error.
func diagConfigFromSettings() *diag.DiagConfig {
	cfg := &diag.DiagConfig{
		Enabled:     true,
		FileSizeCap: 10 << 20,
		DirSizeCap:  50 << 20,
		KeepFiles:   5,
		Level:       diag.LevelInfo,
	}
	if configDir, err := os.UserConfigDir(); err == nil {
		if s, err := store.NewSettingsStoreWithDir(filepath.Join(configDir, "uniTerm")); err == nil {
			if settings, err := s.Load(); err == nil {
				d := settings.Diag
				cfg.Enabled = d.Enabled
				if d.FileSizeCapMiB > 0 {
					cfg.FileSizeCap = int64(d.FileSizeCapMiB) << 20
				}
				if d.DirSizeCapMiB > 0 {
					cfg.DirSizeCap = int64(d.DirSizeCapMiB) << 20
				}
				if d.KeepFiles > 0 {
					cfg.KeepFiles = d.KeepFiles
				}
				if d.Level != "" {
					cfg.Level = diag.Level(d.Level)
				}
			}
		}
	}
	return cfg
}
