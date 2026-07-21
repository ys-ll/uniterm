package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go.bug.st/serial"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	stdsync "sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"github.com/ys-ll/uniterm/backend/database"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/platform"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
	"github.com/ys-ll/uniterm/backend/sync"
	"github.com/ys-ll/uniterm/backend/update"
)

type App struct {
	ctx                  context.Context
	sessionManager       *session.SessionManager
	connectionStore      *store.ConnectionStore
	aiSessionStore       *store.AISessionStore
	settingsStore        *store.SettingsStore
	localStateStore      *store.LocalStateStore
	quickCommandsStore   *store.QuickCommandsStore
	tunnelStore          *store.TunnelStore
	terminalHistoryStore *store.TerminalHistoryStore
	recentStore          *store.RecentStore
	syncService          *sync.SyncService
	tunnelService        *session.TunnelService
	mainHwnd             uintptr
	originalWndProc      uintptr
	wndProcCb            uintptr // keep alive to prevent GC
	inSizeMove           bool
	webviewDataPath      string
	chatCancel           context.CancelFunc // active stream cancellation
	chatCancelMu         stdsync.Mutex      // guards chatCancel
	moveResizeCh         chan string        // defer EventsEmit from WndProc

	// Session output log state (issue #227). Logs are keyed by panelID so
	// they survive reconnects — a single panel may cycle through many
	// session objects and the log file spans all of them. sessionToPanel
	// tracks the current session→panel binding so emitData can look up
	// the right logger. panelAutoTriggered records which panels have
	// already been considered for the LogOnConnect auto-enable so
	// reconnects don't re-enable a log the user manually stopped.
	panelLogs          map[string]*session.OutputLogger
	sessionToPanel     map[string]string
	panelAutoTriggered map[string]bool
	panelLogMu         stdsync.Mutex
	// customLogDir, when non-empty, overrides defaultSessionLogDir()
	// as the target for new session logs. Set from settings via
	// SetDefaultSessionLogDir; ongoing logs are not migrated.
	customLogDir   string
	customLogDirMu stdsync.RWMutex
}

func NewApp(webviewDataPath string) *App {
	return &App{
		webviewDataPath:    webviewDataPath,
		panelLogs:          make(map[string]*session.OutputLogger),
		sessionToPanel:     make(map[string]string),
		panelAutoTriggered: make(map[string]bool),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Init logger first so subsequent log.Writef calls actually write
	if err := log.Init(); err != nil {
		fmt.Printf("WARN: log.Init failed: %v\n", err)
	}

	// On macOS, disable the system press-and-hold accent picker for this app so
	// that holding a key repeats input in the terminal (see app_darwin.go).
	a.configureMacKeyRepeat()

	a.sessionManager = session.NewSessionManager()
	a.tunnelService = session.NewTunnelService()

	// Defer EventsEmit from WndProc to avoid blocking the modal resize/move loop.
	a.moveResizeCh = make(chan string, 10)
	go func() {
		for evt := range a.moveResizeCh {
			runtime.EventsEmit(a.ctx, evt)
			if evt == "rdp:move-resize-end" {
				a.saveWindowStateFromRuntime()
			}
		}
	}()

	// Discover main window HWND for RDP child window embedding
	a.mainHwnd = a.findMainWindow()
	a.subclassMainWindow()

	cs, err := store.NewConnectionStore()
	if err != nil {
		log.Writef("Failed to init connection store: %v", err)
		return
	}
	a.connectionStore = cs

	ass, err := store.NewAISessionStore()
	if err != nil {
		log.Writef("Failed to init AI session store: %v", err)
		return
	}
	a.aiSessionStore = ass

	ss, err := store.NewSettingsStore()
	if err != nil {
		log.Writef("Failed to init settings store: %v", err)
		return
	}
	a.settingsStore = ss

	// Prime the session-log directory override from persisted settings
	// so a log Enable that lands before the settings UI opens still
	// respects the user's choice from a prior run.
	if settings, err := ss.Load(); err == nil {
		a.SetDefaultSessionLogDir(settings.Terminal.SessionLogDir)
	}

	// Init terminal history store (same config dir as other stores)
	configDir, _ := os.UserConfigDir()
	appDir := filepath.Join(configDir, "uniTerm")
	a.terminalHistoryStore = store.NewTerminalHistoryStore(appDir)
	a.quickCommandsStore = store.NewQuickCommandsStore(appDir)
	a.tunnelStore = store.NewTunnelStore(appDir)
	a.localStateStore = store.NewLocalStateStore(appDir)
	a.recentStore = store.NewRecentStore(appDir)
	if _, err := a.recentStore.Load(); err != nil {
		log.Writef("recentStore.Load: %v", err)
	}

	// Push tunnel runtime state to the frontend, and bring up auto-start tunnels.
	a.tunnelService.SetStateCallback(func(st session.TunnelState) {
		runtime.EventsEmit(a.ctx, "tunnel:state", st)
	})
	go a.autoStartTunnels()

	syncSvc, err := sync.NewSyncService()
	if err != nil {
		log.Writef("Failed to create sync service: %v", err)
	} else {
		a.syncService = syncSvc
		// Wire keychain into stores for password/API key migration
		if a.connectionStore != nil {
			a.connectionStore.SetPasswordStore(syncSvc.PasswordStore())
		}
		if a.settingsStore != nil {
			a.settingsStore.SetPasswordStore(syncSvc.PasswordStore())
		}
		// Auto-sync on startup if enabled
		if syncSvc.IsAutoSyncEnabled() {
			go func() {
				result, err := syncSvc.Sync()
				if err != nil {
					log.Writef("Auto-sync on startup failed: %v", err)
				} else if result.Direction == sync.SyncConflict {
					runtime.EventsEmit(a.ctx, "sync:conflict", map[string]interface{}{
						"localTime":  result.Conflict.LocalTime.Format(time.RFC3339),
						"remoteTime": result.Conflict.RemoteTime.Format(time.RFC3339),
					})
				}
			}()
		}
	}

	// Restore window position and size from last session
	a.restoreWindow(ctx)
}

// restoreWindow restores the saved window position and size.
// Windows will constrain off-screen windows to the visible area, so no
// explicit screen-boundary validation is needed.
func (a *App) restoreWindow(ctx context.Context) {
	ls, err := a.localStateStore.Load()
	if err != nil {
		return
	}
	if ls.WindowWidth <= 0 || ls.WindowHeight <= 0 {
		return
	}
	// Move to the correct monitor first, then maximise if needed
	runtime.WindowSetPosition(ctx, ls.WindowX, ls.WindowY)
	if ls.WindowMaximised {
		runtime.WindowMaximise(ctx)
	} else {
		runtime.WindowSetSize(ctx, ls.WindowWidth, ls.WindowHeight)
	}
}

// saveWindowStateFromRuntime saves the current window geometry using runtime
// API calls. Called from the WndProc event loop on Windows (WM_EXITSIZEMOVE).
func (a *App) saveWindowStateFromRuntime() {
	if a.localStateStore == nil {
		return
	}
	// Do not save geometry when minimised — the position is off-screen
	// (-32000, -32000 on Windows) and the size is the tiny taskbar thumbnail,
	// which would restore incorrectly.
	if runtime.WindowIsMinimised(a.ctx) {
		return
	}
	ls, err := a.localStateStore.Load()
	if err != nil {
		return
	}
	ls.WindowX, ls.WindowY = runtime.WindowGetPosition(a.ctx)
	ls.WindowWidth, ls.WindowHeight = runtime.WindowGetSize(a.ctx)
	ls.WindowMaximised = runtime.WindowIsMaximised(a.ctx)
	_ = a.localStateStore.Save(ls)
}

func (a *App) SaveWindowState(x, y, width, height int, maximised bool) {
	if a.localStateStore == nil {
		return
	}
	ls, err := a.localStateStore.Load()
	if err != nil {
		return
	}
	ls.WindowX = x
	ls.WindowY = y
	ls.WindowWidth = width
	ls.WindowHeight = height
	ls.WindowMaximised = maximised
	a.localStateStore.Save(ls)
}

func (a *App) shutdown(ctx context.Context) {
	a.unsubclassMainWindow()
	if a.tunnelService != nil {
		a.tunnelService.Shutdown()
	}
	if a.sessionManager != nil {
		a.sessionManager.CloseAll()
	}
	os.RemoveAll(a.webviewDataPath)
}

// ConnectionStore methods

func (a *App) SaveConnections(data session.ConnectionStoreData) error {
	if a.connectionStore == nil {
		return fmt.Errorf("connection store not initialized")
	}
	err := a.connectionStore.Save(data)
	if err == nil {
		runtime.EventsEmit(a.ctx, "store:connections:changed", data)
		a.triggerAutoSync()
	}
	return err
}

func (a *App) LoadConnections() (session.ConnectionStoreData, error) {
	if a.connectionStore == nil {
		return session.ConnectionStoreData{}, fmt.Errorf("connection store not initialized")
	}
	return a.connectionStore.Load()
}

// TunnelStore methods

func (a *App) SaveTunnels(data session.TunnelStoreData) error {
	if a.tunnelStore == nil {
		return fmt.Errorf("tunnel store not initialized")
	}
	err := a.tunnelStore.Save(data)
	if err == nil {
		runtime.EventsEmit(a.ctx, "store:tunnels:changed", data)
	}
	return err
}

func (a *App) LoadTunnels() (session.TunnelStoreData, error) {
	if a.tunnelStore == nil {
		return session.TunnelStoreData{}, fmt.Errorf("tunnel store not initialized")
	}
	return a.tunnelStore.Load()
}

// connResolver returns a resolver over the current saved connections so the
// tunnel layer can look up the exit connection and recurse its jump hosts.
func (a *App) connResolver() (session.ConnResolver, error) {
	conns, err := a.connectionStore.Load()
	if err != nil {
		return nil, err
	}
	index := make(map[string]session.ConnectionConfig, len(conns.Connections))
	for _, c := range conns.Connections {
		index[c.ID] = c
	}
	return func(id string) (session.ConnectionConfig, bool) {
		c, ok := index[id]
		return c, ok
	}, nil
}

// StartTunnel brings the tunnel with the given ID up and returns its state.
func (a *App) StartTunnel(id string) (session.TunnelState, error) {
	if a.tunnelService == nil || a.tunnelStore == nil || a.connectionStore == nil {
		return session.TunnelState{}, fmt.Errorf("tunnel service not initialized")
	}
	data, err := a.tunnelStore.Load()
	if err != nil {
		return session.TunnelState{}, err
	}
	var t *session.Tunnel
	for i := range data.Tunnels {
		if data.Tunnels[i].ID == id {
			t = &data.Tunnels[i]
			break
		}
	}
	if t == nil {
		return session.TunnelState{}, fmt.Errorf("tunnel %s not found", id)
	}
	resolve, err := a.connResolver()
	if err != nil {
		return session.TunnelState{}, err
	}
	st := a.tunnelService.StartTunnel(*t, resolve)
	if st.Status == session.TunnelError {
		return st, fmt.Errorf("%s", st.Error)
	}
	return st, nil
}

// StopTunnel tears down the tunnel with the given ID.
func (a *App) StopTunnel(id string) error {
	if a.tunnelService != nil {
		a.tunnelService.StopTunnel(id)
	}
	return nil
}

// ListTunnelStates returns the runtime state of every known tunnel.
func (a *App) ListTunnelStates() []session.TunnelState {
	if a.tunnelService == nil {
		return nil
	}
	return a.tunnelService.TunnelStates()
}

// autoStartTunnels starts every tunnel flagged AutoStart. Errors surface via the
// per-tunnel state event, not as a startup failure.
func (a *App) autoStartTunnels() {
	if a.tunnelService == nil || a.tunnelStore == nil || a.connectionStore == nil {
		return
	}
	data, err := a.tunnelStore.Load()
	if err != nil {
		return
	}
	resolve, err := a.connResolver()
	if err != nil {
		return
	}
	for _, t := range data.Tunnels {
		if t.AutoStart {
			a.tunnelService.StartTunnel(t, resolve)
		}
	}
}

// AI Config Store methods

func (a *App) SaveAIConfig(config store.AIConfig) error {
	if a.settingsStore == nil {
		return fmt.Errorf("settings store not initialized")
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	// Update the active model's fields
	for i := range settings.AI.Models {
		if settings.AI.Models[i].ID == settings.AI.ActiveModelID {
			settings.AI.Models[i].APIKey = config.APIKey
			settings.AI.Models[i].BaseURL = config.BaseURL
			settings.AI.Models[i].Model = config.Model
			break
		}
	}
	if err := a.settingsStore.Save(settings); err != nil {
		return err
	}
	a.triggerAutoSync()
	return nil
}

// LocalStateStore methods — sidecar visibility that stays local, never synced.

func (a *App) SaveLocalState(state store.LocalState) error {
	if a.localStateStore == nil {
		return fmt.Errorf("local state store not initialized")
	}
	return a.localStateStore.Save(state)
}

func (a *App) LoadLocalState() (store.LocalState, error) {
	if a.localStateStore == nil {
		return store.LocalState{SidebarVisible: true, AISidebarVisible: true}, nil
	}
	return a.localStateStore.Load()
}

// bgDir returns the directory holding the (local-only, never-synced)
// background image. It is created on demand.
func (a *App) bgDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(configDir, "uniTerm", "backgrounds")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

var allowedBgExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

// SetBackgroundImage copies the chosen image into the app's backgrounds
// directory as a single fixed file (overwriting any previous one) and
// returns the stored file name. It does NOT touch local_state.json.
func (a *App) SetBackgroundImage(srcPath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(srcPath))
	if _, ok := allowedBgExt[ext]; !ok {
		return "", fmt.Errorf("unsupported image type: %s", ext)
	}
	dir, err := a.bgDir()
	if err != nil {
		return "", err
	}
	for e := range allowedBgExt {
		_ = os.Remove(filepath.Join(dir, "bg"+e))
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	name := "bg" + ext
	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return name, nil
}

// GetBackgroundImage reads the stored background file and returns it as a
// data URL. Returns an empty string (no error) when name is empty or the
// file is missing, so the frontend degrades gracefully.
func (a *App) GetBackgroundImage(name string) (string, error) {
	if name == "" {
		return "", nil
	}
	ext := strings.ToLower(filepath.Ext(name))
	mime, ok := allowedBgExt[ext]
	if !ok {
		return "", nil
	}
	dir, err := a.bgDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// ClearBackgroundImage removes any stored background image file.
func (a *App) ClearBackgroundImage() error {
	dir, err := a.bgDir()
	if err != nil {
		return err
	}
	for e := range allowedBgExt {
		_ = os.Remove(filepath.Join(dir, "bg"+e))
	}
	return nil
}

// reloadStoresAfterSync reloads connections and settings from disk and emits
// events so the frontend refreshes after a sync pull.
func (a *App) reloadStoresAfterSync() {
	if a.connectionStore != nil {
		if data, err := a.connectionStore.Load(); err == nil {
			runtime.EventsEmit(a.ctx, "store:connections:changed", data)
		}
	}
	if a.settingsStore != nil {
		if settings, err := a.settingsStore.Load(); err == nil {
			runtime.EventsEmit(a.ctx, "store:settings:changed", settings)
		}
	}
	if a.quickCommandsStore != nil {
		if data, err := a.quickCommandsStore.Load(); err == nil {
			runtime.EventsEmit(a.ctx, "store:quickCommands:changed", data)
		}
	}
}

func (a *App) triggerAutoSync() {
	if a.syncService == nil || !a.syncService.IsAutoSyncEnabled() {
		return
	}
	go func() {
		result, err := a.syncService.Sync()
		if err != nil {
			log.Writef("Auto-sync failed: %v", err)
		} else if result.Direction == sync.SyncConflict {
			runtime.EventsEmit(a.ctx, "sync:conflict", map[string]interface{}{
				"localTime":  result.Conflict.LocalTime.Format(time.RFC3339),
				"remoteTime": result.Conflict.RemoteTime.Format(time.RFC3339),
			})
		}
		if err == nil && result.Direction == sync.SyncPull {
			a.reloadStoresAfterSync()
		}
		runtime.EventsEmit(a.ctx, "sync:completed")
	}()
}
func (a *App) SyncGetConfig() (sync.SyncConfig, error) {
	if a.syncService == nil {
		return sync.SyncConfig{}, fmt.Errorf("sync service not initialized")
	}
	return a.syncService.GetConfig()
}

// SyncSaveConfig saves the sync configuration.
func (a *App) SyncSaveConfig(config sync.SyncConfig, token string) error {
	if a.syncService == nil {
		return fmt.Errorf("sync service not initialized")
	}
	return a.syncService.SaveConfig(config, token)
}

// SyncNow runs an immediate sync.
func (a *App) SyncNow() (*sync.SyncResult, error) {
	if a.syncService == nil {
		return nil, fmt.Errorf("sync service not initialized")
	}
	result, err := a.syncService.Sync()
	if err != nil {
		return nil, err
	}
	if result.Direction == sync.SyncConflict {
		runtime.EventsEmit(a.ctx, "sync:conflict", map[string]interface{}{
			"localTime":  result.Conflict.LocalTime.Format(time.RFC3339),
			"remoteTime": result.Conflict.RemoteTime.Format(time.RFC3339),
		})
	}
	if result.Direction == sync.SyncPull {
		a.reloadStoresAfterSync()
	}
	runtime.EventsEmit(a.ctx, "sync:completed")
	return result, nil
}

// SyncResolveConflict resolves a sync conflict.
func (a *App) SyncResolveConflict(useLocal bool) (*sync.SyncResult, error) {
	if a.syncService == nil {
		return nil, fmt.Errorf("sync service not initialized")
	}
	result, err := a.syncService.ResolveConflict(useLocal)
	if err != nil {
		return nil, err
	}
	if result.Direction == sync.SyncPull {
		if data, err := a.connectionStore.Load(); err == nil {
			runtime.EventsEmit(a.ctx, "store:connections:changed", data)
		}
		if settings, err := a.settingsStore.Load(); err == nil {
			runtime.EventsEmit(a.ctx, "store:settings:changed", settings)
		}
	}
	return result, nil
}

// SyncTestConnection tests the repository connection.
func (a *App) SyncTestConnection() error {
	if a.syncService == nil {
		return fmt.Errorf("sync service not initialized")
	}
	return a.syncService.TestConnection()
}

// SyncConfigureRepo sets up a new or existing sync repository.
func (a *App) SyncConfigureRepo(repoURL, username, token, masterPassword string) (*sync.SyncResult, error) {
	if a.syncService == nil {
		return nil, fmt.Errorf("sync service not initialized")
	}
	result, err := a.syncService.ConfigureRepo(repoURL, username, token, masterPassword)
	if err == nil {
		a.reloadStoresAfterSync()
		runtime.EventsEmit(a.ctx, "sync:completed")
	}
	return result, err
}

// SyncChangePassword re-encrypts synced files with a new master password.
func (a *App) SyncChangePassword(oldPassword, newPassword string) error {
	if a.syncService == nil {
		return fmt.Errorf("sync service not initialized")
	}
	return a.syncService.ChangePassword(oldPassword, newPassword)
}

// SyncVerifyPassword verifies the given password can decrypt the repo config.
func (a *App) SyncVerifyPassword(password, username, token string) error {
	if a.syncService == nil {
		return fmt.Errorf("sync service not initialized")
	}
	return a.syncService.VerifySyncPassword(password, username, token)
}

// SyncDeleteRepo removes the sync repository configuration.
func (a *App) SyncDeleteRepo() error {
	if a.syncService == nil {
		return fmt.Errorf("sync service not initialized")
	}
	return a.syncService.DeleteRepo()
}

func (a *App) LoadAIConfig() (store.AIConfig, error) {
	if a.settingsStore == nil {
		return store.AIConfig{}, fmt.Errorf("settings store not initialized")
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		return store.AIConfig{}, err
	}
	// Return the active model's config
	for _, m := range settings.AI.Models {
		if m.ID == settings.AI.ActiveModelID {
			return store.AIConfig{
				APIKey:  m.APIKey,
				BaseURL: m.BaseURL,
				Model:   m.Model,
			}, nil
		}
	}
	return store.AIConfig{}, nil
}

// AI Session Store methods

func (a *App) SaveAISessions(data store.AISessionData) error {
	if a.aiSessionStore == nil {
		return fmt.Errorf("AI session store not initialized")
	}
	return a.aiSessionStore.Save(data)
}

func (a *App) LoadAISessions() (store.AISessionData, error) {
	if a.aiSessionStore == nil {
		return store.AISessionData{}, fmt.Errorf("AI session store not initialized")
	}
	return a.aiSessionStore.Load()
}

// SettingsStore methods

func (a *App) SaveSettings(settings store.AppSettings) error {
	if a.settingsStore == nil {
		return fmt.Errorf("settings store not initialized")
	}
	err := a.settingsStore.Save(settings)
	if err == nil {
		a.triggerAutoSync()
	}
	return err
}

func (a *App) LoadSettings() (store.AppSettings, error) {
	if a.settingsStore == nil {
		return store.AppSettings{}, fmt.Errorf("settings store not initialized")
	}
	return a.settingsStore.Load()
}

// QuickCommandsStore methods

func (a *App) SaveQuickCommands(data store.QuickCommandData) error {
	if a.quickCommandsStore == nil {
		return fmt.Errorf("quick commands store not initialized")
	}
	err := a.quickCommandsStore.Save(data)
	if err == nil {
		a.triggerAutoSync()
	}
	return err
}

func (a *App) LoadQuickCommands() (store.QuickCommandData, error) {
	if a.quickCommandsStore == nil {
		return store.QuickCommandData{}, fmt.Errorf("quick commands store not initialized")
	}
	return a.quickCommandsStore.Load()
}

func (a *App) OpenFileDialog() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select File",
	})
}

// OpenFileDialogFiltered is like OpenFileDialog but restricts the picker to
// a single extension filter (e.g. for importing a specific file format).
func (a *App) OpenFileDialogFiltered(title, filterDisplayName, filterPattern string) (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: title,
		Filters: []runtime.FileFilter{
			{DisplayName: filterDisplayName, Pattern: filterPattern},
		},
	})
}

func (a *App) OpenMultipleFilesDialog() ([]string, error) {
	files, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Files",
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func (a *App) OpenDirectoryDialog() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Directory",
	})
}

func (a *App) SaveFileDialog(defaultName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Save File",
		DefaultFilename: defaultName,
	})
}

// SaveFileDialogFiltered is like SaveFileDialog but restricts the picker to
// a single extension filter (e.g. for exporting a specific file format).
func (a *App) SaveFileDialogFiltered(title, defaultName, filterDisplayName, filterPattern string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: filterDisplayName, Pattern: filterPattern},
		},
	})
}

func (a *App) GetDesktopPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, "Desktop"), nil
}

func (a *App) GetPlatform() string {
	return goruntime.GOOS
}

func (a *App) GetSystemFonts() ([]string, error) {
	return platform.GetFontFamilies()
}

func (a *App) OnConnectionsChanged(callback func(session.ConnectionStoreData)) {
	runtime.EventsOn(a.ctx, "store:connections:changed", func(optionalData ...interface{}) {
		if len(optionalData) > 0 {
			if data, ok := optionalData[0].(session.ConnectionStoreData); ok {
				callback(data)
			}
		}
	})
}

// SessionManager methods

func (a *App) CreateSession(sessionType string, config session.ConnectionConfig) (*session.SessionInfo, error) {
	if a.sessionManager == nil {
		return nil, fmt.Errorf("session manager not initialized")
	}
	log.Writef("[CreateSession] type=%s, dbType=%s, host=%s, port=%d, user=%s, dbName=%s, name=%s",
		sessionType, config.DBType, config.Host, config.Port, config.User, config.DBName, config.Name)
	s, err := a.sessionManager.Create(sessionType, config)
	if err != nil {
		log.Writef("[CreateSession] manager.Create failed: %v", err)
		return nil, err
	}
	log.Writef("[CreateSession] session created, id=%s", s.ID())
	// Record the LogOnConnect preference synchronously so the frontend's
	// subsequent RegisterSessionForPanel can consult it — the actual
	// Connect() goroutine may not have run yet at Register time.
	if setter, ok := s.(interface{ SetLogOnConnect(bool) }); ok {
		setter.SetLogOnConnect(config.LogOnConnect)
	}
	// Apply terminal character encoding (SSH only). No-op for utf-8/empty.
	if ssh, ok := s.(*session.SSHSession); ok {
		ssh.SetEncoding(config.Encoding)
	}

	// Apply serial config; connection itself is handled by the async goroutine
	// below (same pattern as SSH/Local). Calling serialSess.Connect here as
	// well would open the port a second time in the goroutine and immediately
	// fail with "Serial port busy" once the first handle is still live.
	if serialSess, ok := s.(*session.SerialSession); ok {
		var sb serial.StopBits
		switch config.SerialStopBits {
		case 1.5:
			sb = serial.OnePointFiveStopBits
		case 2:
			sb = serial.TwoStopBits
		default:
			sb = serial.OneStopBit
		}

		parityMap := map[string]serial.Parity{
			"none":  serial.NoParity,
			"odd":   serial.OddParity,
			"even":  serial.EvenParity,
			"mark":  serial.MarkParity,
			"space": serial.SpaceParity,
		}
		par, ok := parityMap[strings.ToLower(config.SerialParity)]
		if !ok {
			par = serial.NoParity
		}

		dataBits := config.SerialDataBits
		if dataBits == 0 {
			dataBits = 8
		}

		serialSess.SetSerialConfig(session.SerialConfig{
			PortName: config.SerialPort,
			BaudRate: config.SerialBaudRate,
			DataBits: dataBits,
			StopBits: sb,
			Parity:   par,
		})
	}

	// ── SSH Tunnel ──────────────────────────────────────────────
	if config.TunnelSSHConnID != "" && a.tunnelService != nil {
		if a.connectionStore == nil {
			_ = a.sessionManager.Close(s.ID())
			return nil, fmt.Errorf("connection store not initialized")
		}
		data, err := a.connectionStore.Load()
		if err != nil {
			_ = a.sessionManager.Close(s.ID())
			return nil, fmt.Errorf("load connections for tunnel: %w", err)
		}
		var tunnelSSHConfig *session.ConnectionConfig
		for _, c := range data.Connections {
			if c.ID == config.TunnelSSHConnID {
				tunnelSSHConfig = &c
				break
			}
		}
		if tunnelSSHConfig == nil {
			_ = a.sessionManager.Close(s.ID())
			return nil, fmt.Errorf("tunnel SSH connection not found: %s", config.TunnelSSHConnID)
		}

		// Apply inline tunnel credentials if the frontend provided them
		// (e.g. credential prompt "connect" without saving to store).
		if config.TunnelSSHUser != "" {
			tunnelSSHConfig.User = config.TunnelSSHUser
		}
		if config.TunnelSSHPassword != "" {
			tunnelSSHConfig.Password = config.TunnelSSHPassword
		}

		// Resolve actual target port. VNC/SPICE use libvirt display
		// numbers (port < 100 means display :N → port 5900+N).
		targetPort := config.Port
		if sessionType == "vnc" || sessionType == "spice" {
			if targetPort <= 0 {
				targetPort = 5900
			} else if targetPort < 100 {
				targetPort += 5900
			}
		}
		localPort, err := a.tunnelService.Start(s.ID(), *tunnelSSHConfig, config.Host, targetPort)
		if err != nil {
			_ = a.sessionManager.Close(s.ID())
			return nil, fmt.Errorf("tunnel start: %w", err)
		}
		log.Writef("[CreateSession] tunnel established for session=%s via ssh=%s, localPort=%d",
			s.ID(), config.TunnelSSHConnID, localPort)
		config.Host = "127.0.0.1"
		config.Port = localPort
	}
	// ── End SSH Tunnel ──────────────────────────────────────────

	// SFTP concurrency limit
	if sessionType == "sftp" {
		if sftp, ok := s.(*session.SFTPSession); ok {
			n := config.SftpMaxConcurrency
			if n <= 0 {
				n = 5
			}
			sftp.SetMaxConcurrency(n)
		}
	}

	// Set parent HWND for RDP sessions
	if rdp, ok := s.(*session.RDPSession); ok {
		rdp.SetParentHwnd(a.mainHwnd)
	}

	s.SetOnDataCallback(func(data []byte) {
		runtime.EventsEmit(a.ctx, "session:data", map[string]interface{}{
			"id":   s.ID(),
			"data": string(data),
		})
	})

	s.SetOnBinaryCallback(func(data []byte) {
		runtime.EventsEmit(a.ctx, "session:binary", map[string]interface{}{
			"id":   s.ID(),
			"data": base64.StdEncoding.EncodeToString(data),
		})
	})

	s.SetOnStatusChangeCallback(func(status session.SessionStatus) {
		payload := map[string]interface{}{
			"id":     s.ID(),
			"status": status,
		}
		// For RDP sessions, include client area screen coordinates so the
		// frontend can position the overlay window without fragile browser APIs.
		if status == session.StatusConnected {
			if rdp, ok := s.(*session.RDPSession); ok {
				cx, cy, cw, ch := rdp.ClientAreaScreenRect()
				payload["clientX"] = cx
				payload["clientY"] = cy
				payload["clientW"] = cw
				payload["clientH"] = ch
			}
			// Attach proxyAddr for VNC and SPICE sessions
			if vnc, ok := s.(*session.VNCSession); ok {
				payload["proxyAddr"] = vnc.ProxyAddr()
			}
			if spice, ok := s.(*session.SPICESession); ok {
				payload["proxyAddr"] = spice.ProxyAddr()
			}
		}

		runtime.EventsEmit(a.ctx, "session:status", payload)
	})

	// Database and Redis sessions connect synchronously so errors are returned to the frontend.
	if sessionType == "database" || sessionType == "redis" {
		log.Writef("[CreateSession] connecting database session synchronously...")
		if err := s.Connect(config); err != nil {
			log.Writef("[CreateSession] database connect failed: %v", err)
			_ = a.sessionManager.Close(s.ID())
			return nil, fmt.Errorf("database connect failed: %w", err)
		}
		log.Writef("[CreateSession] database session connected successfully, id=%s", s.ID())
	} else {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Writef("session %s connect panic: %v\n%s", s.ID(), r, string(debug.Stack()))
				}
			}()

			// RDP TCP pre-check: fail fast before creating the ActiveX window.
			if sessionType == "rdp" {
				port := config.Port
				if port <= 0 { port = 3389 }
				addr := fmt.Sprintf("%s:%d", config.Host, port)
				tcpConn, tcpErr := net.DialTimeout("tcp", addr, 5*time.Second)
				if tcpErr != nil {
					log.Writef("[CreateSession] RDP TCP pre-check to %s failed: %v", addr, tcpErr)
					if a.ctx != nil {
						runtime.EventsEmit(a.ctx, "session:status", map[string]interface{}{
							"id":           s.ID(),
							"status":       "error",
							"errorMessage": fmt.Sprintf("Cannot reach %s: %v", addr, tcpErr),
						})
					}
					if a.sessionManager != nil {
						_ = a.sessionManager.Close(s.ID())
					}
					return
				}
				tcpConn.Close()
				log.Writef("[CreateSession] RDP TCP pre-check to %s succeeded", addr)
			}

			if err := s.Connect(config); err != nil {
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "session:data", map[string]interface{}{
						"id":   s.ID(),
						"data": fmt.Sprintf("\r\n\x1b[31m[Connection failed: %v]\x1b[0m\r\nPress Enter to retry...\r\n", err),
					})
				}
				log.Writef("session %s connect error: %v", s.ID(), err)
				// Remove failed session from manager to avoid leaking stale entries
				if a.sessionManager != nil {
					_ = a.sessionManager.Close(s.ID())
				}
			}
		}()
	}

	info := &session.SessionInfo{
		ID:     s.ID(),
		Type:   s.Type(),
		Title:  s.Title(),
		Status: s.Status(),
	}
	return info, nil
}

func (a *App) CloseSession(sessionID string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	if a.tunnelService != nil {
		a.tunnelService.Stop(sessionID)
	}
	return a.sessionManager.Close(sessionID)
}

func (a *App) ListSessions() []session.SessionInfo {
	if a.sessionManager == nil {
		return []session.SessionInfo{}
	}
	return a.sessionManager.List()
}

func (a *App) SessionWrite(sessionID string, data string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return s.Write([]byte(data))
}

func (a *App) SessionResize(sessionID string, cols, rows int) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return s.Resize(cols, rows)
}

func (a *App) SessionStartZmodem(sessionID string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	s.SetZmodemMode(true)
	return nil
}

func (a *App) SessionEndZmodem(sessionID string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	s.SetZmodemMode(false)
	return nil
}

func (a *App) SessionWriteBinary(sessionID string, base64Data string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	return s.Write(data)
}

func (a *App) ReadFileBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (a *App) FileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("path is a directory: %s", path)
	}
	return info.Size(), nil
}

func (a *App) ReadFileChunkBase64(path string, offset int64, length int64) (string, error) {
	if offset < 0 {
		return "", fmt.Errorf("offset must be non-negative")
	}
	if length <= 0 {
		return "", fmt.Errorf("length must be positive")
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory: %s", path)
	}
	if offset >= info.Size() {
		return "", nil
	}
	if remaining := info.Size() - offset; length > remaining {
		length = remaining
	}

	buf := make([]byte, length)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read file chunk: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf[:n]), nil
}

func (a *App) WriteFileBase64(path string, base64Data string) error {
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func (a *App) AppendFileBase64(path string, base64Data string, offset int64) error {
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}

	flag := os.O_CREATE | os.O_WRONLY
	if offset == 0 {
		flag |= os.O_TRUNC
	} else {
		flag |= os.O_APPEND
	}

	f, err := os.OpenFile(path, flag, 0644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	if info.Size() != offset {
		return fmt.Errorf("append offset mismatch: expected %d, got %d", offset, info.Size())
	}

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (a *App) RDPSetPosition(sessionID string, x, y, w, h int) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	rdp, ok := s.(*session.RDPSession)
	if !ok {
		return fmt.Errorf("session is not RDP")
	}
	rdp.SetPosition(x, y, w, h)
	return nil
}

func (a *App) RDPShow(sessionID string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	rdp, ok := s.(*session.RDPSession)
	if !ok {
		return fmt.Errorf("session is not RDP")
	}
	rdp.Show()
	return nil
}

func (a *App) RDPSetFocus(sessionID string, focused bool) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	rdp, ok := s.(*session.RDPSession)
	if !ok {
		return fmt.Errorf("session is not RDP")
	}
	rdp.SetFocus(focused)
	return nil
}

func (a *App) RDPHide(sessionID string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	rdp, ok := s.(*session.RDPSession)
	if !ok {
		return fmt.Errorf("session is not RDP")
	}
	rdp.Hide()
	return nil
}

// MonitorSession methods

func (a *App) getMonitorSession(sessionID string) (*session.MonitorSession, error) {
	if a.sessionManager == nil {
		return nil, fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	ms, ok := s.(*session.MonitorSession)
	if !ok {
		return nil, fmt.Errorf("session is not a monitor session: %s", sessionID)
	}
	return ms, nil
}

func (a *App) SetMonitorActiveTab(sessionID string, tab string) error {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return err
	}
	ms.SetActiveTab(tab)
	return nil
}

func (a *App) SetMonitorPaused(sessionID string, paused bool) error {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return err
	}
	ms.SetPaused(paused)
	return nil
}

func (a *App) GetProcessDetail(sessionID string, pid int) (map[string]interface{}, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.GetProcessDetail(pid)
}

func (a *App) KillProcess(sessionID string, pid int, signal string) error {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return err
	}
	return ms.KillProcess(pid, signal)
}

func (a *App) GetPorts(sessionID string) ([]session.PortInfo, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.GetPorts()
}

func (a *App) GetDisks(sessionID string) ([]session.DiskInfo, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.GetDisks()
}

func (a *App) GetNetworkCards(sessionID string) ([]session.NetCardInfo, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.GetNetworkCards()
}

type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (a *App) GetAppInfo() AppInfo {
	return AppInfo{
		Name:    "uniTerm",
		Version: Version,
	}
}

func (a *App) CheckForUpdate(source string) (*update.UpdateInfo, error) {
	return update.Check(Version, source)
}

func (a *App) SaveTerminalHistory(entries []store.HistoryEntry) error {
	if a.terminalHistoryStore == nil {
		return fmt.Errorf("terminal history store not initialized")
	}
	return a.terminalHistoryStore.Save(entries)
}

func (a *App) LoadTerminalHistory() ([]store.HistoryEntry, error) {
	if a.terminalHistoryStore == nil {
		return []store.HistoryEntry{}, fmt.Errorf("terminal history store not initialized")
	}
	return a.terminalHistoryStore.Load()
}

func (a *App) DeleteTerminalHistoryEntry(ids []string) error {
	if a.terminalHistoryStore == nil {
		return fmt.Errorf("terminal history store not initialized")
	}
	return a.terminalHistoryStore.DeleteByIDs(ids)
}

// RecentStore methods

func (a *App) RecordRecentConnection(connId string) {
	if a.recentStore == nil {
		return
	}
	a.recentStore.Record(connId)
}

func (a *App) GetRecentConnections() []string {
	if a.recentStore == nil {
		return []string{}
	}
	return a.recentStore.GetAll()
}

// ChatCompletion streams the Anthropic API response via SSE, emitting Wails
// events for each token while collecting the full message. It returns the
// complete message JSON when the stream ends (backward-compatible).
func (a *App) ChatCompletion(apiKey, baseURL, model string, requestJSON string, protocol string, userAgent string) (string, error) {
	// Parse the incoming request body (always Anthropic format from frontend)
	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(requestJSON), &reqBody); err != nil {
		return "", fmt.Errorf("invalid request JSON: %w", err)
	}

	if userAgent == "" {
		userAgent = "uniTerm"
	}

	if protocol == "openai" {
		return a.chatCompletionOpenAI(apiKey, baseURL, model, reqBody, userAgent)
	}
	return a.chatCompletionAnthropic(apiKey, baseURL, model, reqBody, userAgent)
}

// chatCompletionAnthropic handles the native Anthropic Messages API with SSE streaming.
func (a *App) chatCompletionAnthropic(apiKey, baseURL, model string, reqBody map[string]interface{}, userAgent string) (string, error) {
	reqBody["stream"] = true

	modifiedJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal modified request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/messages"

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	a.chatCancelMu.Lock()
	a.chatCancel = cancel
	a.chatCancelMu.Unlock()
	defer func() {
		a.chatCancelMu.Lock()
		a.chatCancel = nil
		a.chatCancelMu.Unlock()
	}()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(modifiedJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 0}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("HTTP %d: %s", res.StatusCode, string(body))
	}

	var contentBlocks []map[string]interface{}
	var currentBlock map[string]interface{}
	var messageRole string
	var usage map[string]interface{}
	currentBlockIndex := -1

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := line[6:]

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)

		switch eventType {
		case "message_start":
			if msg, ok := event["message"].(map[string]interface{}); ok {
				messageRole, _ = msg["role"].(string)
			}

		case "content_block_start":
			currentBlockIndex++
			if block, ok := event["content_block"].(map[string]interface{}); ok {
				currentBlock = block
				runtime.EventsEmit(a.ctx, "ai:block_start", map[string]interface{}{
					"index":         currentBlockIndex,
					"content_block": block,
				})
			}

		case "content_block_delta":
			delta, _ := event["delta"].(map[string]interface{})
			deltaType, _ := delta["type"].(string)

			if deltaType == "text_delta" {
				text, _ := delta["text"].(string)
				if currentBlock != nil {
					if currentBlock["text"] == nil {
						currentBlock["text"] = ""
					}
					currentBlock["text"] = currentBlock["text"].(string) + text
				}
				runtime.EventsEmit(a.ctx, "ai:token", map[string]interface{}{
					"text":  text,
					"index": currentBlockIndex,
				})
			}
			if deltaType == "input_json_delta" && currentBlock != nil {
				partial, _ := delta["partial_json"].(string)
				if currentBlock["input"] == nil || fmt.Sprintf("%T", currentBlock["input"]) != "string" {
					currentBlock["input"] = ""
				}
				if s, ok := currentBlock["input"].(string); ok {
					currentBlock["input"] = s + partial
				}
			}

		case "content_block_stop":
			if currentBlock != nil {
				if blockType, _ := currentBlock["type"].(string); blockType == "tool_use" {
					if inputStr, ok := currentBlock["input"].(string); ok && inputStr != "" {
						var inputObj map[string]interface{}
						if err := json.Unmarshal([]byte(inputStr), &inputObj); err == nil {
							currentBlock["input"] = inputObj
						}
					}
				}
				contentBlocks = append(contentBlocks, currentBlock)
				currentBlock = nil
			}

		case "message_delta":
			if u, ok := event["usage"].(map[string]interface{}); ok {
				usage = u
			}
			if delta, ok := event["delta"].(map[string]interface{}); ok {
				if stopReason, ok := delta["stop_reason"].(string); ok {
					runtime.EventsEmit(a.ctx, "ai:done", map[string]interface{}{
						"message": map[string]interface{}{
							"role":    messageRole,
							"content": contentBlocks,
						},
						"usage":       usage,
						"stop_reason": stopReason,
					})
				}
			}

		case "message_stop":
			fullMessage := map[string]interface{}{
				"role":    messageRole,
				"content": contentBlocks,
			}
			resultJSON, err := json.Marshal(fullMessage)
			if err != nil {
				return "", fmt.Errorf("marshal full message: %w", err)
			}
			return string(resultJSON), nil

		case "error":
			errData, _ := event["error"].(map[string]interface{})
			errMsg, _ := errData["message"].(string)
			return "", fmt.Errorf("stream error: %s", errMsg)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(contentBlocks) > 0 {
		fullMessage := map[string]interface{}{
			"role":    messageRole,
			"content": contentBlocks,
		}
		resultJSON, _ := json.Marshal(fullMessage)
		return string(resultJSON), nil
	}

	return "", fmt.Errorf("stream ended without message_stop")
}

// anthropicToolToOpenAI converts an Anthropic tool definition to OpenAI format.
func anthropicToolToOpenAI(t map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        t["name"],
			"description": t["description"],
			"parameters":  t["input_schema"],
		},
	}
}

// convertAnthropicMessageToOpenAI converts one Anthropic-format message to OpenAI format.
func convertAnthropicMessageToOpenAI(msg map[string]interface{}) []map[string]interface{} {
	role, _ := msg["role"].(string)
	content := msg["content"]

	var results []map[string]interface{}

	switch role {
	case "user":
		out := map[string]interface{}{"role": "user"}
		if contentStr, ok := content.(string); ok {
			out["content"] = contentStr
		} else if contentBlocks, ok := content.([]interface{}); ok {
			for _, block := range contentBlocks {
				if b, ok := block.(map[string]interface{}); ok {
					if bType, _ := b["type"].(string); bType == "text" {
						out["content"] = b["text"]
					}
					if bType, _ := b["type"].(string); bType == "tool_result" {
						toolMsg := map[string]interface{}{
							"role":         "tool",
							"tool_call_id": b["tool_use_id"],
							"content":      toString(b["content"]),
						}
						results = append(results, toolMsg)
					}
				}
			}
		}
		if _, hasContent := out["content"]; hasContent {
			results = append([]map[string]interface{}{out}, results...)
		}

	case "assistant":
		out := map[string]interface{}{"role": "assistant"}
		var toolCalls []map[string]interface{}
		if contentStr, ok := content.(string); ok {
			out["content"] = contentStr
		} else if contentBlocks, ok := content.([]interface{}); ok {
			for _, block := range contentBlocks {
				if b, ok := block.(map[string]interface{}); ok {
					if bType, _ := b["type"].(string); bType == "text" {
						out["content"] = b["text"]
					}
					if bType, _ := b["type"].(string); bType == "tool_use" {
						argsStr := "{}"
						if input, ok := b["input"]; ok {
							argsBytes, _ := json.Marshal(input)
							argsStr = string(argsBytes)
						}
						toolCalls = append(toolCalls, map[string]interface{}{
							"id":   b["id"],
							"type": "function",
							"function": map[string]interface{}{
								"name":      b["name"],
								"arguments": argsStr,
							},
						})
					}
				}
			}
		}
		if len(toolCalls) > 0 {
			out["tool_calls"] = toolCalls
		}
		results = append([]map[string]interface{}{out}, results...)

	default:
		out := map[string]interface{}{"role": role}
		if contentStr, ok := content.(string); ok {
			out["content"] = contentStr
		}
		results = append([]map[string]interface{}{out}, results...)
	}

	return results
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// chatCompletionOpenAI converts the Anthropic-format request to OpenAI,
// calls the OpenAI Chat Completions API with SSE streaming, and converts
// the response back to Anthropic format so the frontend sees no difference.
func (a *App) chatCompletionOpenAI(apiKey, baseURL, model string, reqBody map[string]interface{}, userAgent string) (string, error) {
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"

	// --- Build OpenAI-format request body ---
	openaiBody := map[string]interface{}{
		"model":      model,
		"stream":     true,
		"max_tokens": reqBody["max_tokens"],
	}

	// Convert tools
	if tools, ok := reqBody["tools"].([]interface{}); ok {
		var oaiTools []map[string]interface{}
		for _, t := range tools {
			if tm, ok := t.(map[string]interface{}); ok {
				oaiTools = append(oaiTools, anthropicToolToOpenAI(tm))
			}
		}
		if len(oaiTools) > 0 {
			openaiBody["tools"] = oaiTools
		}
	}

	// Convert messages + system
	var oaiMessages []map[string]interface{}
	if system, ok := reqBody["system"].(string); ok && system != "" {
		oaiMessages = append(oaiMessages, map[string]interface{}{
			"role":    "system",
			"content": system,
		})
	}
	if msgs, ok := reqBody["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if mm, ok := m.(map[string]interface{}); ok {
				converted := convertAnthropicMessageToOpenAI(mm)
				oaiMessages = append(oaiMessages, converted...)
			}
		}
	}
	openaiBody["messages"] = oaiMessages

	requestJSON, err := json.Marshal(openaiBody)
	if err != nil {
		return "", fmt.Errorf("marshal openai request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	a.chatCancelMu.Lock()
	a.chatCancel = cancel
	a.chatCancelMu.Unlock()
	defer func() {
		a.chatCancelMu.Lock()
		a.chatCancel = nil
		a.chatCancelMu.Unlock()
	}()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(requestJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 0}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("HTTP %d: %s", res.StatusCode, string(body))
	}

	// --- Parse OpenAI SSE stream, emit Anthropic-format events ---
	var contentBlocks []map[string]interface{}
	var currentBlock map[string]interface{}
	var messageRole = "assistant"
	currentBlockIndex := -1
	activeToolCalls := make(map[int]map[string]interface{}) // index -> accumulating tool_call

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Emit message_start at the beginning
	runtime.EventsEmit(a.ctx, "ai:message_start", map[string]interface{}{
		"message": map[string]interface{}{"role": "assistant"},
	})

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := line[6:]

		if strings.TrimSpace(dataStr) == "[DONE]" {
			// Emit content_block_stop for any open block
			if currentBlock != nil {
				contentBlocks = append(contentBlocks, currentBlock)
				runtime.EventsEmit(a.ctx, "ai:content_block_stop", map[string]interface{}{
					"index": currentBlockIndex,
				})
				currentBlock = nil
			}
			// Close any open tool_use blocks
			for idx, tc := range activeToolCalls {
				contentBlocks = append(contentBlocks, tc)
				runtime.EventsEmit(a.ctx, "ai:content_block_stop", map[string]interface{}{
					"index": idx,
				})
			}
			activeToolCalls = make(map[int]map[string]interface{})

			// Emit message_delta and message_stop
			runtime.EventsEmit(a.ctx, "ai:done", map[string]interface{}{
				"message": map[string]interface{}{
					"role":    messageRole,
					"content": contentBlocks,
				},
				"stop_reason": "end_turn",
			})

			fullMessage := map[string]interface{}{
				"role":    messageRole,
				"content": contentBlocks,
			}
			resultJSON, _ := json.Marshal(fullMessage)
			return string(resultJSON), nil
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
			continue
		}

		choices, _ := event["choices"].([]interface{})
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]interface{})
		delta, _ := choice["delta"].(map[string]interface{})
		if delta == nil {
			continue
		}

		// Handle text content
		if textDelta, ok := delta["content"].(string); ok && textDelta != "" {
			if currentBlock == nil || currentBlock["type"] != "text" {
				// Close previous block if any
				if currentBlock != nil {
					contentBlocks = append(contentBlocks, currentBlock)
					runtime.EventsEmit(a.ctx, "ai:content_block_stop", map[string]interface{}{
						"index": currentBlockIndex,
					})
				}
				currentBlockIndex++
				currentBlock = map[string]interface{}{
					"type": "text",
					"text": "",
				}
				runtime.EventsEmit(a.ctx, "ai:block_start", map[string]interface{}{
					"index":         currentBlockIndex,
					"content_block": currentBlock,
				})
			}
			currentBlock["text"] = currentBlock["text"].(string) + textDelta
			runtime.EventsEmit(a.ctx, "ai:token", map[string]interface{}{
				"text":  textDelta,
				"index": currentBlockIndex,
			})
		}

		// Handle tool_calls in delta
		if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
			for _, tc := range toolCalls {
				tcMap, _ := tc.(map[string]interface{})
				idxF, _ := tcMap["index"].(float64)
				idx := int(idxF)

				if _, exists := activeToolCalls[idx]; !exists {
					// Close current text block if open
					if currentBlock != nil {
						contentBlocks = append(contentBlocks, currentBlock)
						runtime.EventsEmit(a.ctx, "ai:content_block_stop", map[string]interface{}{
							"index": currentBlockIndex,
						})
						currentBlock = nil
					}
					currentBlockIndex++
					activeToolCalls[idx] = map[string]interface{}{
						"type":  "tool_use",
						"id":    tcMap["id"],
						"name":  "",
						"input": "",
					}
					runtime.EventsEmit(a.ctx, "ai:block_start", map[string]interface{}{
						"index": currentBlockIndex,
						"content_block": map[string]interface{}{
							"type": "tool_use",
							"id":   tcMap["id"],
						},
					})
				}

				atc := activeToolCalls[idx]
				if fn, ok := tcMap["function"].(map[string]interface{}); ok {
					if name, ok := fn["name"].(string); ok && name != "" {
						atc["name"] = name
					}
					if args, ok := fn["arguments"].(string); ok && args != "" {
						if atc["input"] == nil {
							atc["input"] = ""
						}
						atc["input"] = atc["input"].(string) + args
						runtime.EventsEmit(a.ctx, "ai:input_json_delta", map[string]interface{}{
							"partial_json": args,
						})
					}
				}
			}
		}

		// Handle finish_reason on the choice level
		if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" && finishReason != "null" {
			// Close any open text block
			if currentBlock != nil {
				contentBlocks = append(contentBlocks, currentBlock)
				runtime.EventsEmit(a.ctx, "ai:content_block_stop", map[string]interface{}{
					"index": currentBlockIndex,
				})
				currentBlock = nil
			}
			// Close tool_use blocks and parse their input JSON
			for idx, tc := range activeToolCalls {
				if inputStr, ok := tc["input"].(string); ok && inputStr != "" {
					var inputObj map[string]interface{}
					if err := json.Unmarshal([]byte(inputStr), &inputObj); err == nil {
						tc["input"] = inputObj
					}
				}
				contentBlocks = append(contentBlocks, tc)
				runtime.EventsEmit(a.ctx, "ai:content_block_stop", map[string]interface{}{
					"index": idx,
				})
			}
			activeToolCalls = make(map[int]map[string]interface{})

			stopReason := "end_turn"
			if finishReason == "tool_calls" {
				stopReason = "tool_use"
			} else if finishReason == "length" {
				stopReason = "max_tokens"
			} else if finishReason == "stop" {
				stopReason = "end_turn"
			}

			runtime.EventsEmit(a.ctx, "ai:done", map[string]interface{}{
				"message": map[string]interface{}{
					"role":    messageRole,
					"content": contentBlocks,
				},
				"stop_reason": stopReason,
			})

			fullMessage := map[string]interface{}{
				"role":    messageRole,
				"content": contentBlocks,
			}
			resultJSON, _ := json.Marshal(fullMessage)
			return string(resultJSON), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(contentBlocks) > 0 || len(activeToolCalls) > 0 {
		for _, tc := range activeToolCalls {
			contentBlocks = append(contentBlocks, tc)
		}
		fullMessage := map[string]interface{}{
			"role":    messageRole,
			"content": contentBlocks,
		}
		resultJSON, _ := json.Marshal(fullMessage)
		return string(resultJSON), nil
	}

	return "", fmt.Errorf("stream ended without completion")
}

// CancelChatStream cancels the currently active ChatCompletion stream.
func (a *App) CancelChatStream() {
	a.chatCancelMu.Lock()
	defer a.chatCancelMu.Unlock()
	if a.chatCancel != nil {
		a.chatCancel()
	}
}

// ModelInfo represents a model entry from the /v1/models response.
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// FetchModels fetches the available model list from an OpenAI-compatible /v1/models endpoint.
func (a *App) FetchModels(apiKey, baseURL string) ([]ModelInfo, error) {
	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "uniTerm")

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", res.StatusCode, string(body))
	}

	var result struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse models response: %w", err)
	}
	return result.Data, nil
}

// SFTP direct API — called from frontend without terminal layer

// fileTransferSession is the common interface for SFTP and FTP sessions.
type fileTransferSession interface {
	ListRemote(dir string) (session.FileListResult, error)
	ListLocal(dir string) (session.FileListResult, error)
	ChangeRemoteDir(dir string) (session.FileListResult, error)
	ChangeLocalDir(dir string) (session.FileListResult, error)
	ListLocalDrives() ([]session.FileItem, error)
	MakeDir(dir string) error
	Remove(path string, recursive bool) error
	Rename(oldPath, newPath string) error
	Chmod(path string, mode os.FileMode) error
	LocalRemove(path string, recursive bool) error
	LocalRename(oldPath, newPath string) error
	LocalMkdir(dir string) error
	LocalGetContent(path string) ([]byte, error)
	LocalPutContent(path string, content []byte) error
	LocalCopy(oldPath, newPath string) error
	LocalMove(oldPath, newPath string) error
	Get(remotePath, localPath string, recursive bool) (string, error)
	Put(localPath, remotePath string, recursive bool) (string, error)
	PutContent(remotePath string, content []byte) error
	GetContent(remotePath string) ([]byte, error)
	Copy(oldPath, newPath string) error
	Move(oldPath, newPath string) error
	CancelTransfer(taskID string) error
	PauseTransfer(taskID string) error
	ResumeTransfer(taskID string) error
}

func (a *App) getSftp(sid string) (fileTransferSession, error) {
	if a.sessionManager == nil {
		return nil, fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sid)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sid)
	}
	if fs, ok := s.(fileTransferSession); ok {
		return fs, nil
	}
	return nil, fmt.Errorf("not a file transfer session: %s", sid)
}

func (a *App) SftpListRemote(sessionID, dir string) (session.FileListResult, error) {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return session.FileListResult{}, err
	}
	return fs.ListRemote(dir)
}

func (a *App) SftpListLocal(sessionID, dir string) (session.FileListResult, error) {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return session.FileListResult{}, err
	}
	return fs.ListLocal(dir)
}

func (a *App) SftpChangeRemoteDir(sessionID, dir string) (session.FileListResult, error) {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return session.FileListResult{}, err
	}
	return fs.ChangeRemoteDir(dir)
}

func (a *App) SftpChangeLocalDir(sessionID, dir string) (session.FileListResult, error) {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return session.FileListResult{}, err
	}
	return fs.ChangeLocalDir(dir)
}

func (a *App) SftpListLocalDrives(sessionID string) ([]session.FileItem, error) {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return nil, err
	}
	return fs.ListLocalDrives()
}

func (a *App) SftpMakeDir(sessionID, dir string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.MakeDir(dir)
}

func (a *App) SftpRemove(sessionID, path string, recursive bool) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.Remove(path, recursive)
}

func (a *App) SftpRename(sessionID, oldPath, newPath string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.Rename(oldPath, newPath)
}

func (a *App) SftpChmod(sessionID, path, mode string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	modeUint, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return fmt.Errorf("invalid mode: %s", mode)
	}
	return fs.Chmod(path, os.FileMode(modeUint))
}

func (a *App) SftpLocalRemove(sessionID, path string, recursive bool) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.LocalRemove(path, recursive)
}

func (a *App) SftpLocalRename(sessionID, oldPath, newPath string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.LocalRename(oldPath, newPath)
}

func (a *App) SftpLocalMkdir(sessionID, dir string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.LocalMkdir(dir)
}

func (a *App) SftpLocalGetContent(sessionID, localPath string) (string, error) {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return "", err
	}
	content, err := fs.LocalGetContent(localPath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(content), nil
}

func (a *App) SftpLocalPutContent(sessionID, localPath, contentBase64, encoding string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	content, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return err
	}
	// Re-encode if target encoding is not UTF-8 (frontend always sends UTF-8)
	content, err = convertEncoding(content, encoding)
	if err != nil {
		return err
	}
	return fs.LocalPutContent(localPath, content)
}

func (a *App) SftpLocalCopy(sessionID, oldPath, newPath string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.LocalCopy(oldPath, newPath)
}

func (a *App) SftpLocalMove(sessionID, oldPath, newPath string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.LocalMove(oldPath, newPath)
}

func (a *App) SftpGet(sessionID, remotePath, localPath string, recursive bool) (string, error) {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return "", err
	}
	return fs.Get(remotePath, localPath, recursive)
}

func (a *App) SftpCancelTransfer(sessionID, taskID string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.CancelTransfer(taskID)
}

func (a *App) SftpPauseTransfer(sessionID, taskID string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.PauseTransfer(taskID)
}

func (a *App) SftpResumeTransfer(sessionID, taskID string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.ResumeTransfer(taskID)
}

func (a *App) SftpPut(sessionID, localPath, remotePath string, recursive bool) (string, error) {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return "", err
	}
	return fs.Put(localPath, remotePath, recursive)
}

func (a *App) SftpPutContent(sessionID, remotePath, contentBase64, encoding string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	content, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return err
	}
	// Re-encode if target encoding is not UTF-8 (frontend always sends UTF-8)
	content, err = convertEncoding(content, encoding)
	if err != nil {
		return err
	}
	return fs.PutContent(remotePath, content)
}

// convertEncoding converts UTF-8 bytes to the target encoding.
// Returns the original bytes unchanged if encoding is UTF-8 or empty.
func convertEncoding(utf8Bytes []byte, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "", "utf-8", "utf8":
		return utf8Bytes, nil
	case "gbk", "gb2312", "gb18030":
		reader := transform.NewReader(bytes.NewReader(utf8Bytes), simplifiedchinese.GBK.NewEncoder())
		return io.ReadAll(reader)
	default:
		return nil, fmt.Errorf("unsupported encoding: %s", encoding)
	}
}

func (a *App) SftpGetContent(sessionID, remotePath string) (string, error) {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return "", err
	}
	content, err := fs.GetContent(remotePath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(content), nil
}

func (a *App) SftpCopy(sessionID, oldPath, newPath string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.Copy(oldPath, newPath)
}

func (a *App) SftpMove(sessionID, oldPath, newPath string) error {
	fs, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return fs.Move(oldPath, newPath)
}

// WriteTempFile writes base64-encoded content to a temp file and returns its path.
func (a *App) WriteTempFile(fileName, contentBase64 string) (string, error) {
	content, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(os.TempDir(), "uniterm")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, fileName)
	if err := os.WriteFile(dst, content, 0644); err != nil {
		return "", err
	}
	return dst, nil
}

// RemoveTempFile removes a file created by WriteTempFile.
func (a *App) RemoveTempFile(path string) error {
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" && !strings.HasPrefix(path, homeDir) {
		// Safety: only allow removing files in temp dir
		tmpDir := filepath.Join(os.TempDir(), "uniterm")
		if !strings.HasPrefix(path, tmpDir) {
			return fmt.Errorf("path not in temp directory")
		}
	}
	return os.Remove(path)
}

// FrontendLog writes a frontend log message to the application log file.
// This is the canonical interface for the frontend to persist debug/audit
// messages alongside backend logs.
func (a *App) FrontendLog(tag string, message string) {
	_ = log.Init()
	log.Writef("[%s] %s", tag, message)
}

// GetDefaultShell returns the system's default shell path for local terminals.
func (a *App) GetDefaultShell() string {
	switch goruntime.GOOS {
	case "windows":
		if _, err := exec.LookPath("pwsh.exe"); err == nil {
			return "pwsh.exe"
		}
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			return "powershell.exe"
		}
		// Prefer explicit Git for Windows paths over WSL bash to avoid
		// WSL relay errors when no Linux distribution is installed.
		for _, p := range []string{
			`C:\Program Files\Git\bin\bash.exe`,
			`C:\Program Files (x86)\Git\bin\bash.exe`,
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		if _, err := exec.LookPath("bash.exe"); err == nil {
			return "bash.exe"
		}
		return "cmd.exe"
	default:
		if shell := os.Getenv("SHELL"); shell != "" {
			return shell
		}
		if _, err := exec.LookPath("bash"); err == nil {
			return "bash"
		}
		return "sh"
	}
}

// ListSerialPorts returns available serial port names.
func (a *App) ListSerialPorts() ([]string, error) {
	return session.ListSerialPorts()
}

// ConnectSerial creates a new serial session and connects asynchronously.
func (a *App) ConnectSerial(portName string, baudRate int, dataBits int, stopBits float64, parity string) (*session.SessionInfo, error) {
	if a.sessionManager == nil {
		return nil, fmt.Errorf("session manager not initialized")
	}

	// Map JS-friendly strings to serial library constants
	var sb serial.StopBits
	switch stopBits {
	case 1.5:
		sb = serial.OnePointFiveStopBits
	case 2:
		sb = serial.TwoStopBits
	default:
		sb = serial.OneStopBit
	}

	parityMap := map[string]serial.Parity{
		"none":  serial.NoParity,
		"odd":   serial.OddParity,
		"even":  serial.EvenParity,
		"mark":  serial.MarkParity,
		"space": serial.SpaceParity,
	}
	par, ok := parityMap[strings.ToLower(parity)]
	if !ok {
		par = serial.NoParity
	}

	serialCfg := session.SerialConfig{
		PortName: portName,
		BaudRate: baudRate,
		DataBits: dataBits,
		StopBits: sb,
		Parity:   par,
	}

	config := session.ConnectionConfig{
		Name: fmt.Sprintf("%s (%d)", portName, baudRate),
		Type: "serial",
	}

	s, err := a.sessionManager.Create("serial", config)
	if err != nil {
		return nil, err
	}

	serSess, ok := s.(*session.SerialSession)
	if !ok {
		_ = a.sessionManager.Close(s.ID())
		return nil, fmt.Errorf("internal error: session is not SerialSession")
	}
	serSess.SetSerialConfig(serialCfg)

	// Wire callbacks (same pattern as CreateSession)
	s.SetOnDataCallback(func(data []byte) {
		runtime.EventsEmit(a.ctx, "session:data", map[string]interface{}{
			"id":   s.ID(),
			"data": string(data),
		})
	})
	s.SetOnBinaryCallback(func(data []byte) {
		runtime.EventsEmit(a.ctx, "session:binary", map[string]interface{}{
			"id":   s.ID(),
			"data": base64.StdEncoding.EncodeToString(data),
		})
	})
	s.SetOnStatusChangeCallback(func(status session.SessionStatus) {
		runtime.EventsEmit(a.ctx, "session:status", map[string]interface{}{
			"id":     s.ID(),
			"status": status,
		})
	})

	// Connect asynchronously
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Writef("serial session %s connect panic: %v\n%s", s.ID(), r, string(debug.Stack()))
			}
		}()
		if err := s.Connect(config); err != nil {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "session:data", map[string]interface{}{
					"id":   s.ID(),
					"data": fmt.Sprintf("\r\n\x1b[31m[Serial connection failed: %v]\x1b[0m\r\n", err),
				})
			}
			_ = a.sessionManager.Close(s.ID())
		}
	}()

	return &session.SessionInfo{
		ID:     s.ID(),
		Type:   s.Type(),
		Title:  s.Title(),
		Status: s.Status(),
	}, nil
}

// \u2500\u2500 Session output log \u2500\u2500

// SessionLogInfo describes the current session-log state for a panel.
// Path is "" when Enabled is false.
type SessionLogInfo struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

// RegisterSessionForPanel binds a session to a panel and, if the panel
// already has an active log, attaches the log writer to the session so
// output starts landing in the log immediately. The frontend calls this
// right after CreateSession succeeds, and on every reconnect.
//
// On the first Register for a panel (i.e. not a reconnect), if the
// session was created from a connection with LogOnConnect=true, the
// log is enabled automatically. Later Registers for the same panel
// never re-trigger — the user's manual stop is respected across
// reconnects for the life of the panel.
func (a *App) RegisterSessionForPanel(sessionID, panelID string) {
	if sessionID == "" || panelID == "" {
		return
	}
	a.panelLogMu.Lock()
	a.sessionToPanel[sessionID] = panelID
	logger := a.panelLogs[panelID]
	autoTriggered := a.panelAutoTriggered[panelID]
	a.panelLogMu.Unlock()

	// Existing logger (reconnect case): rewire writer, don't re-enable.
	if logger != nil {
		a.installWriter(sessionID, logger)
		return
	}

	// First Register for this panel: check LogOnConnect and auto-enable.
	if !autoTriggered {
		a.panelLogMu.Lock()
		a.panelAutoTriggered[panelID] = true
		a.panelLogMu.Unlock()
		if a.sessionWantsAutoLog(sessionID) {
			// EnableSessionOutputLog handles the writer install internally.
			if _, err := a.EnableSessionOutputLog(panelID, ""); err != nil {
				log.Writef("[RegisterSessionForPanel] auto-enable log failed: %v", err)
			}
		}
	}
}

// sessionWantsAutoLog reports whether the session was created from a
// connection that opted in to LogOnConnect. Returns false for missing
// or non-terminal sessions.
func (a *App) sessionWantsAutoLog(sessionID string) bool {
	if a.sessionManager == nil {
		return false
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return false
	}
	if q, ok := s.(interface{ AutoLogOnConnect() bool }); ok {
		return q.AutoLogOnConnect()
	}
	return false
}

// UnregisterSession clears the session\u2192panel binding and detaches any
// writer from the session. The logger itself is unaffected: it stays on
// the panel, waiting for the next session (reconnect) to register.
func (a *App) UnregisterSession(sessionID string) {
	if sessionID == "" {
		return
	}
	a.panelLogMu.Lock()
	delete(a.sessionToPanel, sessionID)
	a.panelLogMu.Unlock()
	a.installWriter(sessionID, nil)
}

// installWriter finds the given session and installs (or clears) the
// output-log writer callback. Non-terminal session types silently
// ignore the request.
func (a *App) installWriter(sessionID string, logger *session.OutputLogger) {
	if a.sessionManager == nil {
		return
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return
	}
	setter, ok := s.(interface{ SetOutputLogWriter(func([]byte)) })
	if !ok {
		return
	}
	if logger == nil {
		setter.SetOutputLogWriter(nil)
		return
	}
	setter.SetOutputLogWriter(logger.WriteOutput)
}

// panelLogTitle picks the filename base for a panel's log. Uses the
// current session's Title if available, otherwise a short synthetic
// name derived from panelID.
func (a *App) panelLogTitle(panelID string) (name, protocol string) {
	a.panelLogMu.Lock()
	var sessionID string
	for sid, pid := range a.sessionToPanel {
		if pid == panelID {
			sessionID = sid
			break
		}
	}
	a.panelLogMu.Unlock()
	if sessionID != "" && a.sessionManager != nil {
		if s, ok := a.sessionManager.Get(sessionID); ok {
			return s.Title(), s.Type()
		}
	}
	suffix := panelID
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return "panel_" + suffix, "session"
}

// EnableSessionOutputLog starts writing terminal output for the given
// panel to a .log file. If dir is empty, the default session log
// directory is used. Returns the final path after sanitization and
// same-second collision suffixing.
//
// The log is bound to the panel, not the session \u2014 so a reconnect
// (which creates a fresh session under the same panel) keeps writing
// to the same file.
func (a *App) EnableSessionOutputLog(panelID, dir string) (string, error) {
	if panelID == "" {
		return "", fmt.Errorf("panelID required")
	}
	// When the caller didn't pin a directory, fall back to the user's
	// configured override; if that is also empty, OutputLogger.Enable
	// will pick the OS default.
	if dir == "" {
		a.customLogDirMu.RLock()
		dir = a.customLogDir
		a.customLogDirMu.RUnlock()
	}
	name, protocol := a.panelLogTitle(panelID)

	a.panelLogMu.Lock()
	logger := a.panelLogs[panelID]
	if logger == nil {
		logger = &session.OutputLogger{}
		a.panelLogs[panelID] = logger
	}
	// Find any session currently bound to this panel so we can wire the
	// writer while we still hold the lock (avoids a race with concurrent
	// register/unregister calls).
	var sessionID string
	for sid, pid := range a.sessionToPanel {
		if pid == panelID {
			sessionID = sid
			break
		}
	}
	a.panelLogMu.Unlock()

	path, err := logger.Enable(dir, name, protocol)
	if err != nil {
		return "", err
	}
	if sessionID != "" {
		a.installWriter(sessionID, logger)
	}
	return path, nil
}

// DisableSessionOutputLog closes the log file for the given panel,
// writes a footer banner, detaches the writer from any active session,
// and drops the panel's logger. Idempotent.
func (a *App) DisableSessionOutputLog(panelID string) error {
	if panelID == "" {
		return nil
	}
	a.panelLogMu.Lock()
	logger := a.panelLogs[panelID]
	delete(a.panelLogs, panelID)
	var sessionID string
	for sid, pid := range a.sessionToPanel {
		if pid == panelID {
			sessionID = sid
			break
		}
	}
	a.panelLogMu.Unlock()
	if sessionID != "" {
		a.installWriter(sessionID, nil)
	}
	if logger != nil {
		logger.Disable()
	}
	return nil
}

// GetSessionOutputLogInfo returns the current log state for a panel.
// Returns zero value when the panel has no active log.
func (a *App) GetSessionOutputLogInfo(panelID string) SessionLogInfo {
	if panelID == "" {
		return SessionLogInfo{}
	}
	a.panelLogMu.Lock()
	logger := a.panelLogs[panelID]
	a.panelLogMu.Unlock()
	if logger == nil {
		return SessionLogInfo{}
	}
	return SessionLogInfo{Enabled: logger.Enabled(), Path: logger.Path()}
}

// OpenPathInExplorer reveals the given file in the platform file
// manager. On Windows uses `explorer /select,<path>`; macOS uses
// `open -R`; Linux uses `xdg-open <dir>` (no selection semantic in
// xdg-open, so the parent directory is opened).
func (a *App) OpenPathInExplorer(path string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	switch goruntime.GOOS {
	case "windows":
		// explorer.exe returns exit code 1 on success; ignore Run's error.
		_ = exec.Command("explorer", "/select,", abs).Run()
		return nil
	case "darwin":
		return exec.Command("open", "-R", abs).Run()
	default:
		return exec.Command("xdg-open", filepath.Dir(abs)).Run()
	}
}

// SetDefaultSessionLogDir installs a user-configured override for the
// directory used by new session logs. Empty clears the override and
// restores the OS default. Existing log files are not migrated; the
// change only affects logs enabled after this call.
func (a *App) SetDefaultSessionLogDir(dir string) {
	a.customLogDirMu.Lock()
	a.customLogDir = dir
	a.customLogDirMu.Unlock()
}

// GetDefaultSessionLogDir returns the directory a fresh session log
// would land in: the user's override if set, otherwise the OS default
// (~/Documents/uniTerm/logs on all platforms). Used by the settings UI
// to show the current default path as a placeholder.
func (a *App) GetDefaultSessionLogDir() string {
	a.customLogDirMu.RLock()
	custom := a.customLogDir
	a.customLogDirMu.RUnlock()
	if custom != "" {
		return custom
	}
	return session.DefaultSessionLogDir()
}

// ── Database methods ──

func (a *App) dbSession(sessionID string) (*session.DatabaseSession, error) {
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		log.Writef("[dbSession] session not found: %s", sessionID)
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	ds, ok := s.(*session.DatabaseSession)
	if !ok {
		log.Writef("[dbSession] session is not a database session: %s (type=%s)", sessionID, s.Type())
		return nil, fmt.Errorf("session is not a database session: %s", sessionID)
	}
	return ds, nil
}

func (a *App) dbProvider(sessionID string) (*session.DatabaseSession, database.Provider, error) {
	ds, err := a.dbSession(sessionID)
	if err != nil {
		return nil, nil, err
	}
	p, err := database.NewProvider(ds.DBType())
	if err != nil {
		return nil, nil, err
	}
	return ds, p, nil
}

func (a *App) redisSession(sessionID string) (*session.RedisSession, error) {
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	rs, ok := s.(*session.RedisSession)
	if !ok {
		return nil, fmt.Errorf("session is not a redis session: %s (type=%s)", sessionID, s.Type())
	}
	return rs, nil
}

func (a *App) mongoSession(sessionID string) (*session.MongoSession, error) {
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	ms, ok := s.(*session.MongoSession)
	if !ok {
		return nil, fmt.Errorf("session is not a mongodb session: %s (type=%s)", sessionID, s.Type())
	}
	return ms, nil
}

// ── Redis methods ──

func (a *App) RedisPing(sessionID string) error {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return err
	}
	return rs.Ping()
}

func (a *App) RedisSwitchDB(sessionID string, idx int) error {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return err
	}
	return rs.SwitchDB(idx)
}

func (a *App) RedisScanKeys(sessionID string, pattern string, cursor uint64, count int64) (*session.ScanResult, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return nil, err
	}
	return rs.ScanKeys(pattern, cursor, count)
}

func (a *App) RedisGetKeyInfo(sessionID string, key string) (*session.RedisKeyInfo, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return nil, err
	}
	return rs.GetKeyInfo(key)
}

func (a *App) RedisDBSize(sessionID string) (int64, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return 0, err
	}
	return rs.DBSize()
}

func (a *App) RedisKeyspaceInfo(sessionID string) (map[int]int64, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return nil, err
	}
	return rs.KeyspaceInfo()
}

func (a *App) RedisDeleteKey(sessionID string, key string) error {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return err
	}
	return rs.DeleteKey(key)
}

func (a *App) RedisKeyExists(sessionID string, key string) (bool, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return false, err
	}
	return rs.KeyExists(key)
}

func (a *App) RedisGetKeyTTL(sessionID string, key string) (int64, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return -2, err
	}
	return rs.GetKeyTTL(key)
}

func (a *App) RedisSetKeyTTL(sessionID string, key string, seconds int64) error {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return err
	}
	return rs.SetKeyTTL(key, seconds)
}

func (a *App) RedisGetString(sessionID string, key string) (string, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return "", err
	}
	return rs.GetString(key)
}

func (a *App) RedisSetString(sessionID string, key string, value string) error {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return err
	}
	return rs.SetString(key, value)
}

func (a *App) RedisGetHashAll(sessionID string, key string) ([]session.FieldEntry, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return nil, err
	}
	return rs.GetHashAll(key)
}

func (a *App) RedisHashSet(sessionID string, key string, field string, value string) error {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return err
	}
	return rs.HashSet(key, field, value)
}

func (a *App) RedisHashDel(sessionID string, key string, fields []string) (int64, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return 0, err
	}
	return rs.HashDel(key, fields)
}

func (a *App) RedisGetListRange(sessionID string, key string, start int64, stop int64) ([]string, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return nil, err
	}
	return rs.GetListRange(key, start, stop)
}

func (a *App) RedisListPush(sessionID string, key string, direction string, values []string) (int64, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return 0, err
	}
	return rs.ListPush(key, direction, values)
}

func (a *App) RedisListPop(sessionID string, key string, direction string) (string, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return "", err
	}
	return rs.ListPop(key, direction)
}

func (a *App) RedisListSet(sessionID string, key string, index int64, value string) error {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return err
	}
	return rs.ListSet(key, index, value)
}

func (a *App) RedisListRemove(sessionID string, key string, value string, count int64) (int64, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return 0, err
	}
	return rs.ListRemove(key, value, count)
}

func (a *App) RedisGetSetAll(sessionID string, key string) ([]string, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return nil, err
	}
	return rs.GetSetAll(key)
}

func (a *App) RedisSetAdd(sessionID string, key string, members []string) (int64, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return 0, err
	}
	return rs.SetAdd(key, members)
}

func (a *App) RedisSetRemove(sessionID string, key string, members []string) (int64, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return 0, err
	}
	return rs.SetRemove(key, members)
}

func (a *App) RedisGetSortedSetRange(sessionID string, key string, min string, max string) ([]session.ScoredMember, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return nil, err
	}
	return rs.GetSortedSetRange(key, min, max)
}

func (a *App) RedisZSetAdd(sessionID string, key string, members []session.ScoredMember) (int64, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return 0, err
	}
	return rs.ZSetAdd(key, members)
}

func (a *App) RedisZSetRemove(sessionID string, key string, members []string) (int64, error) {
	rs, err := a.redisSession(sessionID)
	if err != nil {
		return 0, err
	}
	return rs.ZSetRemove(key, members)
}

// ── MongoDB methods ──

func (a *App) MongoPing(sessionID string) error {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return err
	}
	return ms.Ping()
}

func (a *App) MongoListDatabases(sessionID string) ([]string, error) {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.ListDatabases()
}

func (a *App) MongoListCollections(sessionID string, dbName string) ([]string, error) {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.ListCollections(dbName)
}

func (a *App) MongoFind(sessionID string, dbName string, collection string, filterJSON string, skip int64, limit int64) (*session.MongoQueryResult, error) {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.Find(dbName, collection, filterJSON, skip, limit)
}

func (a *App) MongoGetDocument(sessionID string, dbName string, collection string, docID string) (string, error) {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return "", err
	}
	return ms.GetDocument(dbName, collection, docID)
}

func (a *App) MongoInsertOne(sessionID string, dbName string, collection string, docJSON string) (string, error) {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return "", err
	}
	return ms.InsertOne(dbName, collection, docJSON)
}

func (a *App) MongoUpdateOne(sessionID string, dbName string, collection string, filterJSON string, updateJSON string) error {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return err
	}
	return ms.UpdateOne(dbName, collection, filterJSON, updateJSON)
}

func (a *App) MongoDeleteOne(sessionID string, dbName string, collection string, filterJSON string) error {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return err
	}
	return ms.DeleteOne(dbName, collection, filterJSON)
}

func (a *App) MongoListIndexes(sessionID string, dbName string, collection string) ([]session.MongoIndexInfo, error) {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.ListIndexes(dbName, collection)
}

func (a *App) MongoCreateCollection(sessionID string, dbName string, collection string) error {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return err
	}
	return ms.CreateCollection(dbName, collection)
}

func (a *App) MongoDropCollection(sessionID string, dbName string, collection string) error {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return err
	}
	return ms.DropCollection(dbName, collection)
}

func (a *App) MongoDropDatabase(sessionID string, dbName string) error {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return err
	}
	return ms.DropDatabase(dbName)
}

func (a *App) MongoCreateIndex(sessionID string, dbName string, collection string, name string, keys []string, unique bool) error {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return err
	}
	return ms.CreateIndex(dbName, collection, name, keys, unique)
}

func (a *App) MongoDropIndex(sessionID string, dbName string, collection string, name string) error {
	ms, err := a.mongoSession(sessionID)
	if err != nil {
		return err
	}
	return ms.DropIndex(dbName, collection, name)
}

func (a *App) GetDatabases(sessionID string) ([]string, error) {
	log.Writef("[GetDatabases] sessionID=%s", sessionID)
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return nil, err
	}
	log.Writef("[GetDatabases] dbType=%s, fetching databases...", ds.DBType())
	dbs, err := p.GetDatabases(ds.DB())
	if err != nil {
		log.Writef("[GetDatabases] failed: %v", err)
	} else {
		log.Writef("[GetDatabases] got %d databases: %v", len(dbs), dbs)
	}
	return dbs, err
}

func (a *App) GetTables(sessionID string, dbName string) ([]database.TableInfo, error) {
	log.Writef("[GetTables] sessionID=%s, dbName=%s", sessionID, dbName)
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return nil, err
	}
	tables, err := p.GetTables(ds.DB(), dbName)
	if err != nil {
		log.Writef("[GetTables] failed: %v", err)
		return nil, err
	}
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].Name < tables[j].Name
	})
	names := make([]string, len(tables))
	for i, t := range tables {
		names[i] = t.Name
	}
	log.Writef("[GetTables] got %d tables: %v", len(tables), names)
	return tables, nil
}

func (a *App) GetTableSchema(sessionID string, dbName string, tableName string) (*database.SchemaResult, error) {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return nil, err
	}
	return p.GetTableSchema(ds.DB(), dbName, tableName)
}

func (a *App) CreateDatabase(sessionID string, dbName string) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.CreateDatabase(ds.DB(), dbName)
}

func (a *App) DropDatabase(sessionID string, dbName string) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.DropDatabase(ds.DB(), dbName)
}

func (a *App) CreateTable(sessionID string, dbName string, tableName string) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.CreateTable(ds.DB(), dbName, tableName)
}

func (a *App) DropTable(sessionID string, dbName string, tableName string) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.DropTable(ds.DB(), dbName, tableName)
}

func (a *App) DropView(sessionID string, dbName string, viewName string) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.DropView(ds.DB(), dbName, viewName)
}

func (a *App) TruncateTable(sessionID string, dbName string, tableName string) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.TruncateTable(ds.DB(), dbName, tableName)
}

func (a *App) ExecuteQuery(sessionID string, dbName string, sql string) (*database.QueryResult, error) {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return nil, err
	}
	return database.ExecuteQuery(p, ds.DB(), dbName, sql)
}

func (a *App) ExecuteStatement(sessionID string, dbName string, sql string) (*database.ExecResult, error) {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return nil, err
	}
	return database.ExecuteStatement(p, ds.DB(), dbName, sql)
}

func (a *App) DBDefaultTableQuery(sessionID string, dbName string, tableName string) (string, error) {
	_, p, err := a.dbProvider(sessionID)
	if err != nil {
		return "", err
	}
	return p.DefaultTableQuery(dbName, tableName, 100), nil
}

func (a *App) DBInsertRow(sessionID string, dbName string, tableName string, values map[string]any) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.InsertRow(ds.DB(), dbName, tableName, values)
}

func (a *App) DBUpdateRow(sessionID string, dbName string, tableName string, set map[string]any, where map[string]any) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.UpdateRow(ds.DB(), dbName, tableName, set, where)
}

func (a *App) DBDeleteRow(sessionID string, dbName string, tableName string, where map[string]any) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.DeleteRow(ds.DB(), dbName, tableName, where)
}

func (a *App) AddColumn(sessionID string, dbName string, tableName string, col database.ColumnDef) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.AddColumn(ds.DB(), dbName, tableName, col)
}

func (a *App) ModifyColumn(sessionID string, dbName string, tableName string, col database.ColumnDef) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.ModifyColumn(ds.DB(), dbName, tableName, col)
}

func (a *App) DropColumn(sessionID string, dbName string, tableName string, colName string) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.DropColumn(ds.DB(), dbName, tableName, colName)
}

func (a *App) AddIndex(sessionID string, dbName string, tableName string, idx database.IndexDef) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.AddIndex(ds.DB(), dbName, tableName, idx)
}

func (a *App) DropIndexOp(sessionID string, dbName string, tableName string, idxName string, isPrimary bool, autoIncCols []string) error {
	ds, p, err := a.dbProvider(sessionID)
	if err != nil {
		return err
	}
	return p.DropIndex(ds.DB(), dbName, tableName, idxName, isPrimary, autoIncCols)
}

func (a *App) GetDBCapabilities(sessionID string) (database.DBCapabilities, error) {
	_, p, err := a.dbProvider(sessionID)
	if err != nil {
		return nil, err
	}
	return database.MergeCapabilities(p.GetCapabilities()), nil
}
