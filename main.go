package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/ys-ll/uniterm/backend/log"
)

var Version = "dev"

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

	// Only set an (empty) app menu on macOS — this is the trick that hides
	// Wails' default Edit/Window menus there. On Linux (GTK) a non-nil Menu
	// makes the backend create an empty GtkMenuBar at the top of the window,
	// which shows up as a thin white line in the Frameless window. Leaving
	// Menu as nil elsewhere avoids that. See issue #291.
	var appMenu *menu.Menu
	if runtime.GOOS == "darwin" {
		appMenu = &menu.Menu{}
	}

	err := wails.Run(&options.App{
		Title:     "uniTerm",
		Width:     1200,
		Height:    800,
		MinWidth:  400,
		MinHeight: 300,
		MaxWidth:  maxW,
		MaxHeight: maxH,
		Frameless: runtime.GOOS != "darwin",
		Menu:      appMenu,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
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
