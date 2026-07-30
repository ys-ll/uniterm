package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"strings"
	stdsync "sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/ys-ll/uniterm/backend/container"
	"github.com/ys-ll/uniterm/backend/database"
	"github.com/ys-ll/uniterm/backend/k8s"
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
	k8sManager           *k8s.Manager
	containerManager     *container.Manager
	connectionStore      *store.ConnectionStore
	aiSessionStore       *store.AISessionStore
	settingsStore        *store.SettingsStore
	localStateStore      *store.LocalStateStore
	quickCommandsStore   *store.QuickCommandsStore
	skillsStore          *store.SkillsStore
	commandsStore        *store.CommandsStore
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
	chatCancel           atomic.Pointer[context.CancelFunc] // per-call swap so overlapping ChatCompletion calls cancel cleanly
	moveResizeCh         chan string                        // defer EventsEmit from WndProc
	// foreground flag — true while the window is visible and the
	// user is interacting. Background goroutines (keepalive, output_log
	// flush, k8s watches, auto-sync) consult IsForeground before burning
	// CPU. Updated via SetAppVisibility and a low-frequency minimised poll
	// as a fallback for paths that don't fire visibilitychange (e.g. macOS
	// Cmd+H before the WebView loads).
	foreground   atomic.Bool
	foregroundMu stdsync.RWMutex

	// lastConnSnapshot is the last-saved connection blob. Used to compute
	// delta emits so a SaveConnections burst doesn't ship the entire store
	// across the bridge on every call. Guarded by lastConnSnapshotMu.
	lastConnSnapshot   session.ConnectionStoreData
	lastConnSnapshotMu stdsync.RWMutex

	// triggerAutoSync coalescing: syncInFlight tracks whether a sync
	// goroutine is running; syncPending records that a new trigger arrived
	// during the run so we fire exactly one follow-up.
	syncInFlight atomic.Bool
	syncPending  atomic.Bool

	// Single shared http.Client for chatCompletion* / FetchModels. Built
	// lazily once on first use so tests that don't hit the LLM path don't
	// pay for the transport; subsequent calls reuse the keep-alive pool.
	httpClient     *http.Client
	httpClientOnce stdsync.Once

	// ListSessions memoization. Frontend polls this and allocates a fresh
	// []SessionInfo on every call; when the (id, status) set hasn't changed
	// we return the cached slice and skip the per-session SessionInfo copy
	// and the JSON-marshal pass on the Wails side.
	listSessionsMu    stdsync.Mutex
	listSessionsCache []session.SessionInfo
	listSessionsHash  uint64

	// Session output log state (issue #227). Logs are keyed by panelID so
	// they survive reconnects — a single panel may cycle through many
	// session objects and the log file spans all of them. sessionToPanel
	// tracks the current session→panel binding so emitData can look up
	// the right logger. panelToSession is the inverse for O(1) lookup.
	// panelAutoTriggered records which panels have already been considered
	// for LogOnConnect auto-enable so reconnects don't re-enable a log the
	// user manually stopped.
	panelLogs          map[string]*session.OutputLogger
	sessionToPanel     map[string]string
	panelToSession     map[string]string
	panelAutoTriggered map[string]bool
	panelLogMu         stdsync.Mutex
	// customLogDir overrides defaultSessionLogDir() as the target for new
	// session logs. Set from settings via SetDefaultSessionLogDir; ongoing
	// logs are not migrated.
	customLogDir   string
	customLogDirMu stdsync.RWMutex

	// errCh accumulates non-fatal init failures during startup() so the
	// frontend can surface them via StartupError / "app:startup-error".
	// Stores may stay nil if their init fails — the nil-guard pattern is
	// preserved; the additive channel just makes the failure visible.
	errCh      chan error
	startupErr error
}

func NewApp(webviewDataPath string) *App {
	a := &App{
		webviewDataPath:    webviewDataPath,
		panelLogs:          make(map[string]*session.OutputLogger),
		sessionToPanel:     make(map[string]string),
		panelToSession:     make(map[string]string),
		panelAutoTriggered: make(map[string]bool),
		k8sManager:         k8s.NewManager(),
		containerManager:   container.NewManager(),
		errCh:              make(chan error, 16),
	}
	a.foreground.Store(true) // optimistic until SetAppVisibility lands
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.k8sManager.SetEventEmitter(func(name string, payload any) {
		runtime.EventsEmit(a.ctx, name, payload)
	})
	a.containerManager.SetEventEmitter(func(name string, payload any) {
		runtime.EventsEmit(a.ctx, name, payload)
	})

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
		// shutdown() closes the channel; the range loop exits and the
		// goroutine returns instead of leaking for the process lifetime.
		for evt := range a.moveResizeCh {
			if a.ctx == nil {
				continue
			}
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
		a.sendStartupErr(fmt.Errorf("connection store: %w", err))
	} else {
		a.connectionStore = cs
	}

	ass, err := store.NewAISessionStore()
	if err != nil {
		log.Writef("Failed to init AI session store: %v", err)
		a.sendStartupErr(fmt.Errorf("ai session store: %w", err))
	} else {
		a.aiSessionStore = ass
	}

	ss, err := store.NewSettingsStore()
	if err != nil {
		log.Writef("Failed to init settings store: %v", err)
		a.sendStartupErr(fmt.Errorf("settings store: %w", err))
	} else {
		a.settingsStore = ss
		// Prime the session-log directory override from persisted settings
		// so a log Enable that lands before the settings UI opens still
		// respects the user's choice from a prior run.
		if settings, err := ss.Load(); err == nil {
			a.SetDefaultSessionLogDir(settings.Terminal.SessionLogDir)
		}
	}

	// Init terminal history store (same config dir as other stores)
	configDir, _ := os.UserConfigDir()
	appDir := filepath.Join(configDir, "uniTerm")
	a.terminalHistoryStore = store.NewTerminalHistoryStore(appDir)
	a.quickCommandsStore = store.NewQuickCommandsStore(appDir)
	a.skillsStore = store.NewSkillsStore(appDir)
	a.commandsStore = store.NewCommandsStore(appDir)
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

	// watchForeground polls WindowIsMinimised as a fallback for paths
	// where the JS visibilitychange event doesn't fire (Cmd+H before any
	// document loads, OS-level Alt+Tab). SetAppVisibility is the primary
	// entry point; this is belt-and-suspenders.
	go a.watchForeground(ctx)

	// NewSyncService's disk-touching init (UserConfigDir + MkdirAll +
	// NewKeychain probing the OS keychain) can take 50–500ms+ on macOS.
	// Run it on a goroutine so OnStartup returns promptly. Wails callers
	// that hit a.syncService before init completes briefly wait on Ready().
	syncSvc, _ := sync.NewSyncServiceAsync()
	a.syncService = syncSvc
	// Schedule post-init wiring (password store + auto-sync) once init
	// completes so we don't race the goroutine.
	go func() {
		select {
		case <-syncSvc.Ready():
		case <-ctx.Done():
			return
		}
		if syncSvc.PasswordStore() == nil {
			return
		}
		// Wire keychain into stores for password/API key migration
		if a.connectionStore != nil {
			a.connectionStore.SetPasswordStore(syncSvc.PasswordStore())
		}
		if a.settingsStore != nil {
			a.settingsStore.SetPasswordStore(syncSvc.PasswordStore())
		}
		// Gate on foreground so a hidden app (launched then immediately
		// minimised, or started by a file association in the background)
		// doesn't burn CPU + PBKDF2 + AES + git push cycles the user can't
		// see.
		if syncSvc.IsAutoSyncEnabled() {
			go func() {
				if !a.waitForegroundFor(3 * time.Second) {
					log.Writef("Auto-sync on startup skipped: app not foreground within 3s")
					return
				}
				result, err := syncSvc.Sync()
				if err != nil {
					log.Writef("Auto-sync on startup failed: %v", err)
				} else if result != nil && result.Direction == sync.SyncConflict {
					runtime.EventsEmit(a.ctx, "sync:conflict", map[string]interface{}{
						"localTime":  result.Conflict.LocalTime.Format(time.RFC3339),
						"remoteTime": result.Conflict.RemoteTime.Format(time.RFC3339),
					})
				}
			}()
		}
	}()

	// Restore window position and size from last session
	a.restoreWindow(ctx)

	// Drain any non-fatal init failures and surface them to the frontend so
	// the user sees a banner instead of getting an NPE on the first store
	// call. Additive only — stores that failed to init are still nil and
	// guarded as before; the app still launches.
	a.drainStartupErr()
	if a.startupErr != nil {
		runtime.EventsEmit(ctx, "app:startup-error", a.startupErr.Error())
	}
}

// sendStartupErr records a non-fatal init failure so the frontend can see
// it after startup completes. Channel is buffered (16) and only written
// from the startup goroutine, so the send is non-blocking.
func (a *App) sendStartupErr(err error) {
	if err == nil {
		return
	}
	select {
	case a.errCh <- err:
	default:
		// Channel full — best-effort drop. The log line is the
		// last-resort record in this case.
		log.Writef("startup error channel full, dropping: %v", err)
	}
}

// drainStartupErr joins every error sent during startup into a single
// startupErr the frontend can query via StartupError().
func (a *App) drainStartupErr() {
	var errs []error
	for {
		select {
		case err := <-a.errCh:
			if err != nil {
				errs = append(errs, err)
			}
		default:
			a.startupErr = errors.Join(errs...)
			return
		}
	}
}

// StartupError returns a human-readable, newline-joined list of any
// non-fatal errors that occurred during startup, or "" if startup
// completed cleanly. The frontend can call this on demand (e.g. after
// the "app:startup-error" event) to display a banner.
func (a *App) StartupError() string {
	if a.startupErr == nil {
		return ""
	}
	return a.startupErr.Error()
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

// IsForeground reports whether the app window is currently in the
// foreground. Background goroutines consult this before running work
// that should pause when the user can't see the terminal.
func (a *App) IsForeground() bool {
	return a.foreground.Load()
}

// waitForegroundFor blocks until the app is foreground or d elapses,
// whichever comes first. Returns whether the foreground state was reached.
// Used to keep background goroutines (auto-sync on startup, triggerAutoSync)
// from burning CPU/wakeups while the user can't see the result. The 3s
// ceiling matches the manual recent bound fixes and keeps a slow startup
// from blocking the caller.
func (a *App) waitForegroundFor(d time.Duration) bool {
	if a.IsForeground() {
		return true
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		<-ticker.C
		if a.IsForeground() {
			return true
		}
	}
	return a.IsForeground()
}

// SetAppVisibility is the lifecycle hook the frontend fires from
// document.visibilitychange. Safe to call from any goroutine. Pass
// visible=false when the page goes hidden (tab switch, OS minimise, Cmd+H).
// The polling goroutine started in startup() is a fallback for cases where
// the JS event doesn't fire (e.g. macOS Cmd+H before any document loaded).
func (a *App) SetAppVisibility(visible bool) {
	prev := a.foreground.Load()
	if prev == visible {
		return
	}
	a.foreground.Store(visible)
	a.foregroundMu.Lock()
	a.foregroundMu.Unlock()
}

// connDelta is the wire shape for store:connections:delta — only the
// changed connection (or all connections on first emit) crosses the
// bridge instead of the full store blob.
type connDelta struct {
	Kind string                       `json:"kind"`         // "upsert" | "remove" | "replace"
	ID   string                       `json:"id,omitempty"` // for upsert/remove
	Conn *session.ConnectionConfig    `json:"connection,omitempty"`
	All  *session.ConnectionStoreData `json:"all,omitempty"` // for replace (first emit)
}

type sessionDataEvent struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

type sessionBinaryEvent struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

// computeConnDelta returns the set of upsert/remove deltas between
// the last snapshot and newData. If no snapshot exists yet (first save
// after startup), returns a single "replace" delta carrying the full
// new data so the frontend can hydrate without waiting for a sync.
func (a *App) computeConnDelta(newData session.ConnectionStoreData) []connDelta {
	a.lastConnSnapshotMu.RLock()
	prev := a.lastConnSnapshot
	a.lastConnSnapshotMu.RUnlock()

	if prev.Connections == nil && prev.Groups == nil {
		// No prior snapshot — ship a single replace so the frontend can
		// hydrate without waiting for sync.
		all := newData
		return []connDelta{{Kind: "replace", All: &all}}
	}

	prevIDs := make(map[string]struct{}, len(prev.Connections))
	for _, c := range prev.Connections {
		prevIDs[c.ID] = struct{}{}
	}
	newIDs := make(map[string]struct{}, len(newData.Connections))
	for _, c := range newData.Connections {
		newIDs[c.ID] = struct{}{}
	}

	var deltas []connDelta
	for _, c := range newData.Connections {
		if _, ok := prevIDs[c.ID]; !ok {
			cc := c
			deltas = append(deltas, connDelta{Kind: "upsert", ID: c.ID, Conn: &cc})
		}
	}
	for id := range prevIDs {
		if _, ok := newIDs[id]; !ok {
			deltas = append(deltas, connDelta{Kind: "remove", ID: id})
		}
	}
	return deltas
}

// saveConnSnapshot updates the snapshot used for future delta
// computation. Called after every successful Save.
func (a *App) saveConnSnapshot(data session.ConnectionStoreData) {
	a.lastConnSnapshotMu.Lock()
	a.lastConnSnapshot = data
	a.lastConnSnapshotMu.Unlock()
}

// llmHTTPClient returns the App-wide *http.Client used by every LLM-bound
// call. Hoisted here so three back-to-back ChatCompletion calls reuse the
// same TCP+TLS connection instead of paying a fresh handshake each time.
// FetchModels uses a shorter timeout via a derived client (see FetchModels).
func (a *App) llmHTTPClient() *http.Client {
	a.httpClientOnce.Do(func() {
		tr := &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 2 * time.Second,
		}
		a.httpClient = &http.Client{Transport: tr}
	})
	return a.httpClient
}

// injectCacheControl adds ephemeral cache_control breakpoints on the
// static system prompt and tools array so Anthropic's prompt caching
// beta actually caches them across turns. Without this the
// prompt-caching-2024-07-31 header is sent but the request body has no
// breakpoints, so every turn re-ships and re-bills the static prefix
// (~3 KB in typical Claude Code sessions).
func injectCacheControl(reqBody map[string]interface{}) {
	if sys, ok := reqBody["system"].(string); ok && sys != "" {
		reqBody["system"] = []map[string]interface{}{{
			"type":          "text",
			"text":          sys,
			"cache_control": map[string]string{"type": "ephemeral"},
		}}
	}
	if tools, ok := reqBody["tools"].([]interface{}); ok && len(tools) > 0 {
		if last, ok := tools[len(tools)-1].(map[string]interface{}); ok {
			last["cache_control"] = map[string]string{"type": "ephemeral"}
		}
	}
}

// watchForeground polls window state so paths which don't fire the JS
// visibilitychange event (Cmd+H on macOS before the WebView is loaded,
// OS-level Alt+Tab) still update the foreground flag. Runs every 2s —
// coarse on purpose, this is a lifecycle hint not a hot path. Exits when
// ctx is done.
func (a *App) watchForeground(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.ctx == nil {
				continue
			}
			visible := !runtime.WindowIsMinimised(a.ctx)
			if visible != a.foreground.Load() {
				a.SetAppVisibility(visible)
			}
		}
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.unsubclassMainWindow()
	if a.tunnelService != nil {
		a.tunnelService.Shutdown()
	}
	if a.sessionManager != nil {
		a.sessionManager.CloseAll()
	}
	if a.terminalHistoryStore != nil {
		_ = a.terminalHistoryStore.Close()
	}
	// close moveResizeCh so the deferred EventsEmit goroutine started in
	// startup() returns instead of living for the process lifetime.
	// close(nil) is a no-op so the nil guard below is safe.
	if a.moveResizeCh != nil {
		close(a.moveResizeCh)
	}
	os.RemoveAll(a.webviewDataPath)
}

// ConnectionStore methods

func (a *App) SaveConnections(data session.ConnectionStoreData) error {
	if a.connectionStore == nil {
		return fmt.Errorf("connection store not initialized")
	}
	// Write off the handler thread and only ship the diff across the bridge.
	// The frontend listens on store:connections:delta for save-time edits
	// and on store:connections:changed (full blob) for sync-driven reloads —
	// so the hot path transfers a few-byte delta instead of every connection
	// on every keystroke.
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.connectionStore.Save(data)
	}()
	go func() {
		err := <-errCh
		if err != nil {
			log.Writef("SaveConnections async: %v", err)
			return
		}
		deltas := a.computeConnDelta(data)
		a.saveConnSnapshot(data)
		if a.ctx != nil {
			for _, d := range deltas {
				runtime.EventsEmit(a.ctx, "store:connections:delta", d)
			}
		}
		a.triggerAutoSync()
	}()
	return nil
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
	// tunnel save touches fs (atomic-rename); keep handler fast.
	go func() {
		if err := a.tunnelStore.Save(data); err != nil {
			log.Writef("SaveTunnels async: %v", err)
			return
		}
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "store:tunnels:changed", data)
		}
	}()
	return nil
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
	// Same async pattern as SaveConnections / SaveTunnels.
	go func() {
		if err := a.quickCommandsStore.Save(data); err != nil {
			log.Writef("SaveQuickCommands async: %v", err)
			return
		}
		a.triggerAutoSync()
	}()
	return nil
}

func (a *App) LoadQuickCommands() (store.QuickCommandData, error) {
	if a.quickCommandsStore == nil {
		return store.QuickCommandData{}, fmt.Errorf("quick commands store not initialized")
	}
	return a.quickCommandsStore.Load()
}

// SkillsStore methods

func (a *App) ListSkills() ([]store.SkillMeta, error) {
	if a.skillsStore == nil {
		return nil, fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.List()
}

func (a *App) GetSkillBody(name string) (string, error) {
	if a.skillsStore == nil {
		return "", fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.GetBody(name)
}

func (a *App) GetSkillFile(name, relPath string) (string, error) {
	if a.skillsStore == nil {
		return "", fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.GetSkillFile(name, relPath)
}

func (a *App) ListSkillFiles(name string) (store.SkillFileList, error) {
	if a.skillsStore == nil {
		return store.SkillFileList{}, fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.ListSkillFiles(name)
}

// ClassifyCommandRisk exposes the backend risk classifier to the frontend so
// effectiveRisk() in services/agent.ts can merge the model-claimed risk with
// the server-classified risk.
func (a *App) ClassifyCommandRisk(command string) store.RiskLevel {
	return store.ClassifyCommandRisk(command)
}

func (a *App) SetSkillEnabled(name string, enabled bool) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.SetEnabled(name, enabled)
}

func (a *App) SetSkillLocked(name string, locked bool) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.SetLocked(name, locked)
}

func (a *App) DeleteSkill(name string) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.Delete(name)
}

func (a *App) ImportSkillFromDir(srcDir string) (string, error) {
	if a.skillsStore == nil {
		return "", fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.ImportFromDir(srcDir)
}

func (a *App) ImportSkillFromZip(zipPath string) (string, error) {
	if a.skillsStore == nil {
		return "", fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.ImportFromZip(zipPath)
}

func (a *App) CreateSkill(name, description, body string) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.CreateSkill(name, description, body)
}

func (a *App) SaveSkill(name, description, body string) error {
	if a.skillsStore == nil {
		return fmt.Errorf("skills store not initialized")
	}
	return a.skillsStore.SaveSkill(name, description, body)
}

// CommandsStore methods

func (a *App) ListCommands() ([]store.CommandMeta, error) {
	if a.commandsStore == nil {
		return nil, fmt.Errorf("commands store not initialized")
	}
	return a.commandsStore.List()
}

func (a *App) GetCommandBody(name string) (string, error) {
	if a.commandsStore == nil {
		return "", fmt.Errorf("commands store not initialized")
	}
	return a.commandsStore.GetBody(name)
}

func (a *App) SetCommandEnabled(name string, enabled bool) error {
	if a.commandsStore == nil {
		return fmt.Errorf("commands store not initialized")
	}
	return a.commandsStore.SetEnabled(name, enabled)
}

func (a *App) SetCommandLocked(name string, locked bool) error {
	if a.commandsStore == nil {
		return fmt.Errorf("commands store not initialized")
	}
	return a.commandsStore.SetLocked(name, locked)
}

func (a *App) DeleteCommand(name string) error {
	if a.commandsStore == nil {
		return fmt.Errorf("commands store not initialized")
	}
	return a.commandsStore.Delete(name)
}

func (a *App) CreateCommand(name, description, argumentHint, body string) error {
	if a.commandsStore == nil {
		return fmt.Errorf("commands store not initialized")
	}
	return a.commandsStore.CreateCommand(name, description, argumentHint, body)
}

func (a *App) SaveCommand(name, description, argumentHint, body string) error {
	if a.commandsStore == nil {
		return fmt.Errorf("commands store not initialized")
	}
	return a.commandsStore.SaveCommand(name, description, argumentHint, body)
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
	// Stash the initial terminal size the frontend measured BEFORE
	// calling CreateSession. Connect() (called async below) reads it via
	// getInitialSize() and uses it for PTY sizing — so the remote shell
	// and Claude Code see the actual xterm cols from the first byte, not
	// the default 80x24 that would otherwise be in use until the late
	// SessionResize arrives.
	if config.InitialCols > 0 && config.InitialRows > 0 {
		if sz, ok := s.(interface{ SetPendingSize(int, int) }); ok {
			sz.SetPendingSize(config.InitialCols, config.InitialRows)
		}
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
	if rdp, ok := s.(*session.RdpSession); ok {
		rdp.SetParentHwnd(a.mainHwnd)
		// Notify the frontend when the user exits native full screen so it can
		// resume position sync.
		rdp.SetOnFullScreenExit(func() {
			runtime.EventsEmit(a.ctx, "rdp:fullscreen-exit", s.ID())
		})
	}

	s.SetOnDataCallback(func(data []byte) {
		if a.ctx != nil {
			// Pass struct, not pre-encoded JSON — EventsEmit marshals
			// each arg itself, so a string would be quoted twice.
			runtime.EventsEmit(a.ctx, "session:data", sessionDataEvent{
				ID:   s.ID(),
				Data: string(data),
			})
		}
	})

	s.SetOnBinaryCallback(func(data []byte) {
		runtime.EventsEmit(a.ctx, "session:binary", sessionBinaryEvent{
			ID:   s.ID(),
			Data: base64.StdEncoding.EncodeToString(data),
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
			if rdp, ok := s.(*session.RdpSession); ok {
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
	} else if !config.DeferConnect {
		// Non-database sessions (SSH, Local, Mosh, Telnet, SFTP, FTP, SMB,
		// WebDAV, S3, Serial, K8s, Mongo, Spice) auto-connect UNLESS the
		// frontend set DeferConnect — which it does so it can mount the
		// xterm terminal, fitAddon-measure the real cols/rows, write them
		// into config.InitialCols/InitialRows and only THEN call
		// SessionStart. Without that gap Claude Code draws tables at the
		// 80x24 default before SessionResize propagates the real width,
		// and the borders drift across output batches.
		a.launchConnectGoroutine(s, sessionType, config)
	}

	info := &session.SessionInfo{
		ID:     s.ID(),
		Type:   s.Type(),
		Title:  s.Title(),
		Status: s.Status(),
	}
	return info, nil
}

// launchConnectGoroutine starts the async Connect path that used to live
// inline in CreateSession. Extracted so CreateSession can skip it when
// the frontend opts into a deferred-start flow (DeferConnect=true) and
// instead drives the connection via SessionStart after measuring cols/rows.
func (a *App) launchConnectGoroutine(s session.Session, sessionType string, config session.ConnectionConfig) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Writef("session %s connect panic: %v\n%s", s.ID(), r, string(debug.Stack()))
			}
		}()

		// RDP TCP pre-check: fail fast before creating the ActiveX window.
		if sessionType == "rdp" {
			port := config.Port
			if port <= 0 {
				port = 3389
			}
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
			if a.sessionManager != nil {
				_ = a.sessionManager.Close(s.ID())
			}
		}
	}()
}

// SessionStart triggers the actual Connect() for a session that was
// created with config.DeferConnect=true. The frontend calls this AFTER
// mounting the xterm terminal and writing the measured InitialCols/InitialRows
// into the deferred config, so the PTY is created at the correct
// dimensions from the first byte — no 80x24 default phase where Claude
// Code can draw tables at the wrong column count.
func (a *App) SessionStart(sessionID string, config session.ConnectionConfig) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	// Re-stash the latest measured size in case the deferred config
	// carries the real cols/rows the frontend discovered after mount.
	if config.InitialCols > 0 && config.InitialRows > 0 {
		s.SetPendingSize(config.InitialCols, config.InitialRows)
	}
	a.launchConnectGoroutine(s, config.Type, config)
	return nil
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

// ListSessions memoizes the last session list returned to the frontend
// along with a hash of (id, status) pairs. When the frontend polls
// ListSessions without an underlying change, we return the same slice
// and skip the per-call []SessionInfo allocation.
//
// Hashing (id, status) only — Title / Type are static per session and
// status is what the polling frontend cares about. Status transitions
// always go through SetOnStatusChangeCallback → emit session:status, so
// the frontend can rely on that event for live updates; ListSessions is
// the initial / refresh path.
func (a *App) ListSessions() []session.SessionInfo {
	if a.sessionManager == nil {
		return []session.SessionInfo{}
	}
	a.listSessionsMu.Lock()
	defer a.listSessionsMu.Unlock()

	infos := a.sessionManager.List()
	hash := hashSessions(infos)
	if hash == a.listSessionsHash && len(a.listSessionsCache) == len(infos) {
		// No underlying change — return the cached slice so the caller
		// gets the same backing array and Go skips allocating a new
		// one. The slice header is still copied by value, which is what
		// the Wails JSON encoder needs.
		return a.listSessionsCache
	}
	a.listSessionsCache = infos
	a.listSessionsHash = hash
	return infos
}

// hashSessions produces a quick stable hash of (id, status) pairs.
// Uses FNV-1a; collisions are vanishingly rare for any realistic
// session count and just cause a single false refresh.
func hashSessions(infos []session.SessionInfo) uint64 {
	h := uint64(14695981039346656037) // FNV offset basis
	for _, s := range infos {
		for i := 0; i < len(s.ID); i++ {
			h ^= uint64(s.ID[i])
			h *= 1099511628211 // FNV prime
		}
		// SessionStatus is a string alias; fold the first 4 bytes in
		// (status strings are short — typically "connected",
		// "disconnected", "error").
		status := string(s.Status)
		for i := 0; i < len(status) && i < 4; i++ {
			h ^= uint64(status[i]) << (uint(i) * 8)
		}
		h *= 1099511628211
	}
	return h
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
	rdp, ok := s.(*session.RdpSession)
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
	rdp, ok := s.(*session.RdpSession)
	if !ok {
		return fmt.Errorf("session is not RDP")
	}
	rdp.Show()
	return nil
}

// RDPSetFullScreen toggles the ActiveX control's built-in full-screen mode.
func (a *App) RDPSetFullScreen(sessionID string, full bool) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	rdp, ok := s.(*session.RdpSession)
	if !ok {
		return fmt.Errorf("session is not RDP")
	}
	rdp.SetFullScreen(full)
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
	rdp, ok := s.(*session.RdpSession)
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

	switch protocol {
	case "openai":
		return a.chatCompletionOpenAI(apiKey, baseURL, model, reqBody, userAgent)
	case "responses":
		return a.chatCompletionResponses(apiKey, baseURL, model, reqBody, userAgent)
	}
	return a.chatCompletionAnthropic(apiKey, baseURL, model, reqBody, userAgent)
}

// anthropicStreamEvent is the typed SSE envelope for Anthropic Messages
// events. Variant fields stay as json.RawMessage so we only decode the
// few fields the handler actually reads per event type.
type anthropicStreamEvent struct {
	Type         string          `json:"type"`
	Index        int             `json:"index"`
	ContentBlock json.RawMessage `json:"content_block"`
	Delta        json.RawMessage `json:"delta"`
	Message      json.RawMessage `json:"message"`
	Usage        json.RawMessage `json:"usage"`
	Error        json.RawMessage `json:"error"`
}

type anthropicDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	PartialJSON string `json:"partial_json"`
}

type anthropicMessageRole struct {
	Role string `json:"role"`
}

type anthropicStopDelta struct {
	StopReason string `json:"stop_reason"`
}

// Typed payloads for the ai:* Wails events. Replacing the per-token
// map[string]interface{} literal with a fixed struct saves the alloc per
// event; json.Marshal on the Wails side writes the same JSON shape
// (lowercase keys) so the frontend contract is unchanged.
type aiTokenEvent struct {
	Text  string `json:"text"`
	Index int    `json:"index"`
}

type aiBlockStartEvent struct {
	Index        int                    `json:"index"`
	ContentBlock map[string]interface{} `json:"content_block"`
}

type aiContentBlockStopEvent struct {
	Index int `json:"index"`
}

type aiInputJsonDeltaEvent struct {
	PartialJSON string `json:"partial_json"`
}

type aiMessageStartEvent struct {
	Role string `json:"role"`
}

type aiDoneEvent struct {
	Message    map[string]interface{} `json:"message"`
	Usage      map[string]interface{} `json:"usage,omitempty"`
	StopReason string                 `json:"stop_reason"`
}

// chatCompletionAnthropic handles the native Anthropic Messages API with SSE streaming.
func (a *App) chatCompletionAnthropic(apiKey, baseURL, model string, reqBody map[string]interface{}, userAgent string) (string, error) {
	reqBody["stream"] = true

	// Insert ephemeral cache_control breakpoints on the static system +
	// tools prefixes so Anthropic reuses the cached tokens across turns.
	// Without these the prompt-caching beta header is a no-op — every turn
	// re-ships and re-bills the static prefix.
	injectCacheControl(reqBody)

	modifiedJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal modified request: %w", err)
	}

	// Anthropic base URL conventionally omits /v1 (client appends /v1/messages).
	// Tolerate legacy configs that already include the /v1 suffix.
	base := strings.TrimRight(baseURL, "/")
	var url string
	if strings.HasSuffix(base, "/v1") {
		url = base + "/messages"
	} else {
		url = base + "/v1/messages"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	// Register our cancel in the App-level pointer and only clear it on
	// the way out if no one replaced us. Per-call swap so overlapping
	// ChatCompletion calls each keep their own cancel; the old mutex
	// pattern would let call A's defer wipe call B's cancel.
	myCancel := cancel
	a.chatCancel.Store(&myCancel)
	defer func() {
		// CAS the slot back to nil, but only if it still points at
		// our own cancel — a newer call may have already taken
		// over the slot.
		a.chatCancel.CompareAndSwap(&myCancel, nil)
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

	client := a.llmHTTPClient()
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("AI_REQUEST_TIMEOUT")
		}
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// Cap the error-body read at 64 KiB so a hostile or buggy upstream
		// returning a multi-GB error body can't OOM the Go process.
		body, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		return "", fmt.Errorf("HTTP %d: %s", res.StatusCode, string(body))
	}

	var contentBlocks []map[string]interface{}
	var currentBlock map[string]interface{}
	var messageRole string
	var usage map[string]interface{}
	currentBlockIndex := -1
	// bytes.Buffer per block. Accumulating text/input via string + string
	// was O(n²) and paid a fresh alloc per token; WriteString into a buffer
	// and flush to a string exactly once at content_block_stop. Only one
	// block is open at a time so a single pair of buffers is sufficient.
	var currentTextBuf, currentInputBuf bytes.Buffer

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := line[6:]

		var ev anthropicStreamEvent
		if err := json.Unmarshal([]byte(dataStr), &ev); err != nil {
			continue
		}

		switch ev.Type {
		case "message_start":
			var mr anthropicMessageRole
			if err := json.Unmarshal(ev.Message, &mr); err == nil {
				messageRole = mr.Role
			}

		case "content_block_start":
			currentBlockIndex++
			currentTextBuf.Reset()
			currentInputBuf.Reset()
			var block map[string]interface{}
			if err := json.Unmarshal(ev.ContentBlock, &block); err == nil {
				currentBlock = block
				runtime.EventsEmit(a.ctx, "ai:block_start", aiBlockStartEvent{
					Index:        currentBlockIndex,
					ContentBlock: block,
				})
			}

		case "content_block_delta":
			var delta anthropicDelta
			if err := json.Unmarshal(ev.Delta, &delta); err != nil {
				continue
			}
			switch delta.Type {
			case "text_delta":
				if currentBlock != nil {
					currentTextBuf.WriteString(delta.Text)
				}
				runtime.EventsEmit(a.ctx, "ai:token", aiTokenEvent{
					Text:  delta.Text,
					Index: currentBlockIndex,
				})
			case "input_json_delta":
				if currentBlock != nil {
					currentInputBuf.WriteString(delta.PartialJSON)
				}
			}

		case "content_block_stop":
			if currentBlock != nil {
				// Flush buffers once so downstream code sees a single
				// string per field.
				if currentTextBuf.Len() > 0 {
					currentBlock["text"] = currentTextBuf.String()
				}
				currentTextBuf.Reset()
				if currentInputBuf.Len() > 0 {
					inputStr := currentInputBuf.String()
					if blockType, _ := currentBlock["type"].(string); blockType == "tool_use" && inputStr != "" {
						var inputObj map[string]interface{}
						if err := json.Unmarshal([]byte(inputStr), &inputObj); err == nil {
							currentBlock["input"] = inputObj
						} else {
							currentBlock["input"] = inputStr
						}
					} else {
						currentBlock["input"] = inputStr
					}
				}
				currentInputBuf.Reset()
				contentBlocks = append(contentBlocks, currentBlock)
				currentBlock = nil
			}

		case "message_delta":
			if len(ev.Usage) > 0 {
				var u map[string]interface{}
				if err := json.Unmarshal(ev.Usage, &u); err == nil {
					usage = u
				}
			}
			var sd anthropicStopDelta
			if err := json.Unmarshal(ev.Delta, &sd); err == nil && sd.StopReason != "" {
				// Marshal the full message once into a pooled buffer and
				// reuse the bytes for both the ai:done emit and the eventual
				// return at message_stop. Previously this struct was rebuilt
				// + remarshaled twice per turn.
				fullMessage := map[string]interface{}{
					"role":    messageRole,
					"content": contentBlocks,
				}
				resultJSON, err := marshalAnthropicFinalMessage(fullMessage)
				if err == nil {
					runtime.EventsEmit(a.ctx, "ai:done", struct {
						Message    json.RawMessage        `json:"message"`
						Usage      map[string]interface{} `json:"usage,omitempty"`
						StopReason string                 `json:"stop_reason"`
					}{
						Message:    json.RawMessage(resultJSON),
						Usage:      usage,
						StopReason: sd.StopReason,
					})
				}
			}

		case "message_stop":
			fullMessage := map[string]interface{}{
				"role":    messageRole,
				"content": contentBlocks,
			}
			resultJSON, err := marshalAnthropicFinalMessage(fullMessage)
			if err != nil {
				return "", fmt.Errorf("marshal full message: %w", err)
			}
			return string(resultJSON), nil

		case "error":
			var e struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(ev.Error, &e)
			return "", fmt.Errorf("stream error: %s", e.Message)
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
		resultJSON, _ := marshalAnthropicFinalMessage(fullMessage)
		return string(resultJSON), nil
	}

	return "", fmt.Errorf("stream ended without message_stop")
}

// anthropicToolToOpenAI converts an Anthropic tool definition to OpenAI format.
// pooled *bytes.Buffer and returns the resulting JSON string. The
// pool avoids per-turn allocator churn; in a heavy Claude Code session
// the buffer grows once to ~3 KiB and stays warm. Returns the buffer
// to the pool via defer in the caller (no — the string escapes the
// goroutine, so we keep ownership here; the buffer can be reused when
// the underlying JSON is no longer referenced by Wails).
func marshalAnthropicFinalMessage(msg map[string]interface{}) ([]byte, error) {
	buf := finalMsgPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer finalMsgPool.Put(buf)
	enc := json.NewEncoder(buf)
	if err := enc.Encode(msg); err != nil {
		return nil, err
	}
	// json.Encoder always appends a trailing newline; trim it.
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return out, nil
}

var finalMsgPool = stdsync.Pool{
	New: func() any {
		b := &bytes.Buffer{}
		b.Grow(4 * 1024)
		return b
	},
}

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
		// Emit tool messages first, then any text user message. An OpenAI-format
		// assistant message with tool_calls must be immediately followed by the
		// matching tool messages; a user text block placed before them triggers a
		// 400 "insufficient tool messages following tool_calls" error.
		if _, hasContent := out["content"]; hasContent {
			results = append(results, out)
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

// Typed SSE shapes for OpenAI Chat Completions. Only the few fields the
// loop reads (delta.content, delta.tool_calls[], choice.finish_reason)
// get decoded; the rest is discarded by the json decoder.
type openaiDeltaToolCall struct {
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type openaiStreamDelta struct {
	Content   string                `json:"content"`
	ToolCalls []openaiDeltaToolCall `json:"tool_calls"`
}

type openaiStreamChoice struct {
	Delta        openaiStreamDelta `json:"delta"`
	FinishReason string            `json:"finish_reason"`
}

type openaiStreamEvent struct {
	Choices []openaiStreamChoice `json:"choices"`
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

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	// Per-call swap so overlapping ChatCompletion calls each keep their own
	// cancel; see chatCompletionAnthropic for full rationale.
	myCancel := cancel
	a.chatCancel.Store(&myCancel)
	defer func() {
		a.chatCancel.CompareAndSwap(&myCancel, nil)
	}()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(requestJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", userAgent)

	client := a.llmHTTPClient()
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("AI_REQUEST_TIMEOUT")
		}
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// Cap the error-body read at 64 KiB so a hostile or buggy upstream
		// returning a multi-GB error body can't OOM the Go process.
		body, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		return "", fmt.Errorf("HTTP %d: %s", res.StatusCode, string(body))
	}

	// --- Parse OpenAI SSE stream, emit Anthropic-format events ---
	var contentBlocks []map[string]interface{}
	var currentBlock map[string]interface{}
	var messageRole = "assistant"
	currentBlockIndex := -1
	activeToolCalls := make(map[int]map[string]interface{}) // index -> accumulating tool_call
	// Per-block text and input buffers so accumulation is O(n) instead of
	// O(n²) string concat per token. Flushed to the block map on
	// content_block_stop / finish_reason.
	var currentTextBuf, currentInputBuf bytes.Buffer
	// Per-tool input buffer so each tool_call's argument concat stays
	// O(n). Keyed by the tool's index — multiple tool_calls can run in
	// parallel (one per idx).
	toolInputBufs := make(map[int]*bytes.Buffer)

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Emit message_start at the beginning.
	runtime.EventsEmit(a.ctx, "ai:message_start", aiMessageStartEvent{
		Role: "assistant",
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
				if currentTextBuf.Len() > 0 {
					currentBlock["text"] = currentTextBuf.String()
				}
				currentTextBuf.Reset()
				contentBlocks = append(contentBlocks, currentBlock)
				runtime.EventsEmit(a.ctx, "ai:content_block_stop", aiContentBlockStopEvent{
					Index: currentBlockIndex,
				})
				currentBlock = nil
			}
			// Close any open tool_use blocks
			for idx, tc := range activeToolCalls {
				contentBlocks = append(contentBlocks, tc)
				runtime.EventsEmit(a.ctx, "ai:content_block_stop", aiContentBlockStopEvent{
					Index: idx,
				})
			}
			activeToolCalls = make(map[int]map[string]interface{})
			currentInputBuf.Reset()

			// Emit message_delta and message_stop
			runtime.EventsEmit(a.ctx, "ai:done", aiDoneEvent{
				Message: map[string]interface{}{
					"role":    messageRole,
					"content": contentBlocks,
				},
				StopReason: "end_turn",
			})

			fullMessage := map[string]interface{}{
				"role":    messageRole,
				"content": contentBlocks,
			}
			resultJSON, _ := marshalAnthropicFinalMessage(fullMessage)
			return string(resultJSON), nil
		}

		var ev openaiStreamEvent
		if err := json.Unmarshal([]byte(dataStr), &ev); err != nil {
			continue
		}
		if len(ev.Choices) == 0 {
			continue
		}
		choice := ev.Choices[0]
		delta := choice.Delta

		// Handle text content
		if delta.Content != "" {
			if currentBlock == nil || currentBlock["type"] != "text" {
				// Close previous block if any
				if currentBlock != nil {
					if currentTextBuf.Len() > 0 {
						currentBlock["text"] = currentTextBuf.String()
					}
					currentTextBuf.Reset()
					contentBlocks = append(contentBlocks, currentBlock)
					runtime.EventsEmit(a.ctx, "ai:content_block_stop", aiContentBlockStopEvent{
						Index: currentBlockIndex,
					})
				}
				currentBlockIndex++
				currentBlock = map[string]interface{}{
					"type": "text",
					"text": "",
				}
				currentTextBuf.Reset()
				runtime.EventsEmit(a.ctx, "ai:block_start", aiBlockStartEvent{
					Index:        currentBlockIndex,
					ContentBlock: currentBlock,
				})
			}
			currentTextBuf.WriteString(delta.Content)
			runtime.EventsEmit(a.ctx, "ai:token", aiTokenEvent{
				Text:  delta.Content,
				Index: currentBlockIndex,
			})
		}

		// Handle tool_calls in delta
		for _, tc := range delta.ToolCalls {
			if tc.Index == nil {
				continue
			}
			idx := *tc.Index

			if _, exists := activeToolCalls[idx]; !exists {
				// Close current text block if open
				if currentBlock != nil {
					if currentTextBuf.Len() > 0 {
						currentBlock["text"] = currentTextBuf.String()
					}
					currentTextBuf.Reset()
					contentBlocks = append(contentBlocks, currentBlock)
					runtime.EventsEmit(a.ctx, "ai:content_block_stop", aiContentBlockStopEvent{
						Index: currentBlockIndex,
					})
					currentBlock = nil
				}
				currentBlockIndex++
				activeToolCalls[idx] = map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  "",
					"input": "",
				}
				runtime.EventsEmit(a.ctx, "ai:block_start", aiBlockStartEvent{
					Index: currentBlockIndex,
					ContentBlock: map[string]interface{}{
						"type": "tool_use",
						"id":   tc.ID,
					},
				})
			}

			atc := activeToolCalls[idx]
			if tc.Function.Name != "" {
				atc["name"] = tc.Function.Name
			}
			if args := tc.Function.Arguments; args != "" {
				// Append to a per-tool *bytes.Buffer instead of string
				// concat (O(n²) over a long tool-args stream).
				buf, ok := toolInputBufs[idx]
				if !ok {
					buf = &bytes.Buffer{}
					toolInputBufs[idx] = buf
				}
				buf.WriteString(args)
				runtime.EventsEmit(a.ctx, "ai:input_json_delta", aiInputJsonDeltaEvent{
					PartialJSON: args,
				})
			}
		}

		// Handle finish_reason on the choice level
		finishReason := choice.FinishReason
		if finishReason != "" && finishReason != "null" {
			// Close any open text block
			if currentBlock != nil {
				if currentTextBuf.Len() > 0 {
					currentBlock["text"] = currentTextBuf.String()
				}
				currentTextBuf.Reset()
				contentBlocks = append(contentBlocks, currentBlock)
				runtime.EventsEmit(a.ctx, "ai:content_block_stop", aiContentBlockStopEvent{
					Index: currentBlockIndex,
				})
				currentBlock = nil
			}
			// Close tool_use blocks and parse their input JSON
			for idx, tc := range activeToolCalls {
				// Prefer the per-tool buffer over the possibly-empty
				// tc["input"] string.
				if buf, ok := toolInputBufs[idx]; ok && buf.Len() > 0 {
					inputStr := buf.String()
					var inputObj map[string]interface{}
					if err := json.Unmarshal([]byte(inputStr), &inputObj); err == nil {
						tc["input"] = inputObj
					} else {
						tc["input"] = inputStr
					}
				} else if inputStr, ok := tc["input"].(string); ok && inputStr != "" {
					var inputObj map[string]interface{}
					if err := json.Unmarshal([]byte(inputStr), &inputObj); err == nil {
						tc["input"] = inputObj
					}
				}
				contentBlocks = append(contentBlocks, tc)
				runtime.EventsEmit(a.ctx, "ai:content_block_stop", aiContentBlockStopEvent{
					Index: idx,
				})
			}
			activeToolCalls = make(map[int]map[string]interface{})
			toolInputBufs = nil
			currentInputBuf.Reset()

			stopReason := "end_turn"
			if finishReason == "tool_calls" {
				stopReason = "tool_use"
			} else if finishReason == "length" {
				stopReason = "max_tokens"
			} else if finishReason == "stop" {
				stopReason = "end_turn"
			}

			runtime.EventsEmit(a.ctx, "ai:done", aiDoneEvent{
				Message: map[string]interface{}{
					"role":    messageRole,
					"content": contentBlocks,
				},
				StopReason: stopReason,
			})

			fullMessage := map[string]interface{}{
				"role":    messageRole,
				"content": contentBlocks,
			}
			resultJSON, _ := marshalAnthropicFinalMessage(fullMessage)
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
		resultJSON, _ := marshalAnthropicFinalMessage(fullMessage)
		return string(resultJSON), nil
	}

	return "", fmt.Errorf("stream ended without completion")
}

// anthropicToolToResponses converts an Anthropic tool definition to the
// Responses API function format (flat, unlike Chat Completions' nested form).
func anthropicToolToResponses(t map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":        "function",
		"name":        t["name"],
		"description": t["description"],
		"parameters":  t["input_schema"],
	}
}

// convertAnthropicMessageToResponses converts one Anthropic-format message to
// Responses API input items. Text turns become message items with
// input_text/output_text; tool_use becomes function_call; tool_result becomes
// function_call_output.
func convertAnthropicMessageToResponses(msg map[string]interface{}) []map[string]interface{} {
	role, _ := msg["role"].(string)
	content := msg["content"]

	var results []map[string]interface{}

	textType := "input_text"
	if role == "assistant" {
		textType = "output_text"
	}

	if contentStr, ok := content.(string); ok {
		if contentStr != "" {
			results = append(results, map[string]interface{}{
				"role": role,
				"content": []map[string]interface{}{
					{"type": textType, "text": contentStr},
				},
			})
		}
		return results
	}

	contentBlocks, ok := content.([]interface{})
	if !ok {
		return results
	}

	var textParts []map[string]interface{}
	for _, block := range contentBlocks {
		b, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		switch b["type"] {
		case "text":
			if txt, _ := b["text"].(string); txt != "" {
				textParts = append(textParts, map[string]interface{}{"type": textType, "text": txt})
			}
		case "tool_use":
			argsStr := "{}"
			if input, ok := b["input"]; ok {
				argsBytes, _ := json.Marshal(input)
				argsStr = string(argsBytes)
			}
			results = append(results, map[string]interface{}{
				"type":      "function_call",
				"call_id":   b["id"],
				"name":      b["name"],
				"arguments": argsStr,
			})
		case "tool_result":
			results = append(results, map[string]interface{}{
				"type":    "function_call_output",
				"call_id": b["tool_use_id"],
				"output":  toString(b["content"]),
			})
		}
	}

	if len(textParts) > 0 {
		msgItem := map[string]interface{}{"role": role, "content": textParts}
		if role == "assistant" {
			results = append([]map[string]interface{}{msgItem}, results...)
		} else {
			results = append(results, msgItem)
		}
	}

	return results
}

// Typed SSE shapes for OpenAI Responses events. The wrapper captures
// the discriminator + output_index; nested item fields are decoded
// lazily per branch so we skip the ~99% of fields the loop discards.
type responsesStreamItem struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Name   string `json:"name"`
}

type responsesStreamEvent struct {
	Type        string          `json:"type"`
	OutputIndex int             `json:"output_index"`
	Item        json.RawMessage `json:"item"`
	Delta       string          `json:"delta"`
}

// chatCompletionResponses converts the Anthropic-format request to the OpenAI
// Responses API, calls /responses with SSE streaming, and converts the response
// events back to Anthropic-format events so the frontend sees no difference.
// Stateless: full history is sent as `input` each turn; reasoning items are ignored.
func (a *App) chatCompletionResponses(apiKey, baseURL, model string, reqBody map[string]interface{}, userAgent string) (string, error) {
	url := strings.TrimRight(baseURL, "/") + "/responses"

	// --- Build Responses-format request body ---
	respBody := map[string]interface{}{
		"model":  model,
		"stream": true,
	}
	if mt, ok := reqBody["max_tokens"]; ok {
		respBody["max_output_tokens"] = mt
	}
	if system, ok := reqBody["system"].(string); ok && system != "" {
		respBody["instructions"] = system
	}

	if tools, ok := reqBody["tools"].([]interface{}); ok {
		var respTools []map[string]interface{}
		for _, t := range tools {
			if tm, ok := t.(map[string]interface{}); ok {
				respTools = append(respTools, anthropicToolToResponses(tm))
			}
		}
		if len(respTools) > 0 {
			respBody["tools"] = respTools
		}
	}

	var input []map[string]interface{}
	if msgs, ok := reqBody["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if mm, ok := m.(map[string]interface{}); ok {
				input = append(input, convertAnthropicMessageToResponses(mm)...)
			}
		}
	}
	respBody["input"] = input

	requestJSON, err := json.Marshal(respBody)
	if err != nil {
		return "", fmt.Errorf("marshal responses request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	// Per-call swap so overlapping ChatCompletion calls each keep their own
	// cancel; see chatCompletionAnthropic for full rationale.
	myCancel := cancel
	a.chatCancel.Store(&myCancel)
	defer func() {
		a.chatCancel.CompareAndSwap(&myCancel, nil)
	}()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(requestJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", userAgent)

	client := a.llmHTTPClient()
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("AI_REQUEST_TIMEOUT")
		}
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// Cap the error-body read at 64 KiB so a hostile or buggy upstream
		// returning a multi-GB error body can't OOM the Go process.
		body, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		return "", fmt.Errorf("HTTP %d: %s", res.StatusCode, string(body))
	}

	// --- Parse Responses SSE stream, emit Anthropic-format events ---
	var contentBlocks []map[string]interface{}
	// Map Responses output_index -> our content block index / accumulating block.
	blockByOutputIdx := make(map[int]map[string]interface{})
	idxByOutputIdx := make(map[int]int)
	nextBlockIndex := 0
	// Parallel maps of *bytes.Buffer so text/input accumulation is O(n)
	// instead of O(n²) string concat per token. Outputs may run in
	// parallel (different output_index) so a single shared buffer doesn't
	// work — keep one per output_index.
	textBufs := make(map[int]*bytes.Buffer)
	inputBufs := make(map[int]*bytes.Buffer)

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	runtime.EventsEmit(a.ctx, "ai:message_start", aiMessageStartEvent{
		Role: "assistant",
	})

	finish := func(stopReason string) (string, error) {
		fullMessage := map[string]interface{}{
			"role":    "assistant",
			"content": contentBlocks,
		}
		resultJSON, err := marshalAnthropicFinalMessage(fullMessage)
		if err != nil {
			return "", fmt.Errorf("marshal final message: %w", err)
		}
		runtime.EventsEmit(a.ctx, "ai:done", struct {
			Message    json.RawMessage `json:"message"`
			StopReason string          `json:"stop_reason"`
		}{
			Message:    json.RawMessage(resultJSON),
			StopReason: stopReason,
		})
		return string(resultJSON), nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := line[6:]
		if strings.TrimSpace(dataStr) == "[DONE]" {
			continue
		}

		var ev responsesStreamEvent
		if err := json.Unmarshal([]byte(dataStr), &ev); err != nil {
			continue
		}

		switch ev.Type {
		case "response.output_item.added":
			var item responsesStreamItem
			if err := json.Unmarshal(ev.Item, &item); err != nil {
				continue
			}
			switch item.Type {
			case "message":
				block := map[string]interface{}{"type": "text", "text": ""}
				blockByOutputIdx[ev.OutputIndex] = block
				idxByOutputIdx[ev.OutputIndex] = nextBlockIndex
				runtime.EventsEmit(a.ctx, "ai:block_start", aiBlockStartEvent{
					Index:        nextBlockIndex,
					ContentBlock: block,
				})
				nextBlockIndex++
			case "function_call":
				block := map[string]interface{}{
					"type":  "tool_use",
					"id":    item.CallID,
					"name":  item.Name,
					"input": "",
				}
				blockByOutputIdx[ev.OutputIndex] = block
				idxByOutputIdx[ev.OutputIndex] = nextBlockIndex
				runtime.EventsEmit(a.ctx, "ai:block_start", aiBlockStartEvent{
					Index: nextBlockIndex,
					ContentBlock: map[string]interface{}{
						"type": "tool_use",
						"id":   item.CallID,
						"name": item.Name,
					},
				})
				nextBlockIndex++
			}

		case "response.output_text.delta":
			block := blockByOutputIdx[ev.OutputIndex]
			if block == nil {
				continue
			}
			if ev.Delta == "" {
				continue
			}
			// Append to per-block *bytes.Buffer instead of O(n²) string
			// concatenation. Flushed on output_item.done.
			buf, ok := textBufs[ev.OutputIndex]
			if !ok {
				buf = &bytes.Buffer{}
				textBufs[ev.OutputIndex] = buf
			}
			buf.WriteString(ev.Delta)
			runtime.EventsEmit(a.ctx, "ai:token", aiTokenEvent{
				Text:  ev.Delta,
				Index: idxByOutputIdx[ev.OutputIndex],
			})

		case "response.function_call_arguments.delta":
			block := blockByOutputIdx[ev.OutputIndex]
			if block == nil {
				continue
			}
			if ev.Delta == "" {
				continue
			}
			buf, ok := inputBufs[ev.OutputIndex]
			if !ok {
				buf = &bytes.Buffer{}
				inputBufs[ev.OutputIndex] = buf
			}
			buf.WriteString(ev.Delta)
			runtime.EventsEmit(a.ctx, "ai:input_json_delta", aiInputJsonDeltaEvent{
				PartialJSON: ev.Delta,
			})

		case "response.output_item.done":
			block := blockByOutputIdx[ev.OutputIndex]
			if block == nil {
				continue
			}
			// Flush per-block buffers once into the block map.
			if buf, ok := textBufs[ev.OutputIndex]; ok {
				if buf.Len() > 0 {
					block["text"] = buf.String()
				}
				delete(textBufs, ev.OutputIndex)
			}
			if buf, ok := inputBufs[ev.OutputIndex]; ok {
				if buf.Len() > 0 {
					inputStr := buf.String()
					if block["type"] == "tool_use" {
						var inputObj map[string]interface{}
						if json.Unmarshal([]byte(inputStr), &inputObj) == nil {
							block["input"] = inputObj
						} else {
							block["input"] = map[string]interface{}{}
						}
					} else {
						block["input"] = inputStr
					}
				}
				delete(inputBufs, ev.OutputIndex)
			}
			contentBlocks = append(contentBlocks, block)
			runtime.EventsEmit(a.ctx, "ai:content_block_stop", aiContentBlockStopEvent{
				Index: idxByOutputIdx[ev.OutputIndex],
			})
			delete(blockByOutputIdx, ev.OutputIndex)

		case "response.completed":
			stopReason := "end_turn"
			for _, b := range contentBlocks {
				if b["type"] == "tool_use" {
					stopReason = "tool_use"
					break
				}
			}
			return finish(stopReason)

		case "response.failed", "error":
			// Marshal the typed event back out for the error message; the
			// caller doesn't need the original map shape.
			body, _ := json.Marshal(ev)
			return "", fmt.Errorf("responses stream error: %s", string(body))
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(contentBlocks) > 0 {
		return finish("end_turn")
	}

	return "", fmt.Errorf("stream ended without completion")
}

// CancelChatStream cancels the currently active ChatCompletion stream.
func (a *App) CancelChatStream() {
	// atomic.Pointer.Load is lock-free; a nil slot means no stream is
	// active.
	if p := a.chatCancel.Load(); p != nil {
		(*p)()
	}
}

// ModelInfo represents a model entry from the /v1/models response.
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// FetchModels fetches the available model list. openai/responses hit an
// OpenAI-compatible /models endpoint with a Bearer token; anthropic hits
// /v1/models with the x-api-key + anthropic-version headers, mirroring
// chatCompletionAnthropic's URL and auth handling.
func (a *App) FetchModels(apiKey, baseURL, protocol string) ([]ModelInfo, error) {
	base := strings.TrimRight(baseURL, "/")

	var url string
	if protocol == "anthropic" {
		// Base URL conventionally omits /v1; tolerate legacy configs with it.
		if strings.HasSuffix(base, "/v1") {
			url = base + "/models"
		} else {
			url = base + "/v1/models"
		}
	} else {
		url = base + "/models"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if protocol == "anthropic" {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("User-Agent", "uniTerm")

	// Share the same transport as the LLM clients so the model list call
	// also benefits from the keep-alive pool; the request itself carries
	// its own 10s deadline via the per-request context.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	res, err := a.llmHTTPClient().Do(req)
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

// ─── Container ─────────────────────────────────────────────────

// ContainerConnect 打开容器连接：解析配置、按 transport 建 Local 或 SSH runner。
// SSH 传输时若被引用连接配了跳板机（TunnelSSHConnID），先起本地转发隧道，
// 与 CreateSession 的单层隧道行为一致。
func (a *App) ContainerConnect(connectionID string) error {
	if a.containerManager == nil || a.connectionStore == nil {
		return fmt.Errorf("container manager not initialized")
	}
	data, err := a.connectionStore.Load()
	if err != nil {
		return err
	}
	var cfg *session.ConnectionConfig
	for _, c := range data.Connections {
		if c.ID == connectionID {
			cfg = &c
			break
		}
	}
	if cfg == nil {
		return fmt.Errorf("connection not found: %s", connectionID)
	}
	if cfg.Type != "container" {
		return fmt.Errorf("connection %s is not a container connection", connectionID)
	}
	rt := container.Runtime(cfg.ContainerRuntime)

	if cfg.ContainerTransport == "local" {
		return a.containerManager.ConnectLocal(connectionID, rt, "")
	}

	var sshCfg *session.ConnectionConfig
	for _, c := range data.Connections {
		if c.ID == cfg.ContainerSSHConnID {
			sshCfg = &c
			break
		}
	}
	if sshCfg == nil {
		return fmt.Errorf("referenced SSH connection missing: %s", cfg.ContainerSSHConnID)
	}

	// 跳板机：被引用连接自身的 tunnel 配置
	if sshCfg.TunnelSSHConnID != "" && a.tunnelService != nil {
		var tunnelCfg *session.ConnectionConfig
		for _, c := range data.Connections {
			if c.ID == sshCfg.TunnelSSHConnID {
				tunnelCfg = &c
				break
			}
		}
		if tunnelCfg == nil {
			return fmt.Errorf("tunnel SSH connection not found: %s", sshCfg.TunnelSSHConnID)
		}
		localPort, err := a.tunnelService.Start(connectionID, *tunnelCfg, sshCfg.Host, sshCfg.Port)
		if err != nil {
			return fmt.Errorf("tunnel start: %w", err)
		}
		sshCfg.Host = "127.0.0.1"
		sshCfg.Port = localPort
		if err := a.containerManager.ConnectSSH(connectionID, rt, "", *sshCfg); err != nil {
			a.tunnelService.Stop(connectionID) // 与 K8sConnect 一致：连接失败时回收隧道
			return err
		}
		return nil
	}
	return a.containerManager.ConnectSSH(connectionID, rt, "", *sshCfg)
}

func (a *App) ContainerDisconnect(connectionID string) {
	a.containerManager.Disconnect(connectionID)
	if a.tunnelService != nil {
		a.tunnelService.Stop(connectionID) // 无同名隧道时为 no-op
	}
}

func (a *App) ContainerList(connectionID string) ([]container.Container, error) {
	p, err := a.containerManager.Provider(connectionID)
	if err != nil {
		return nil, err
	}
	return p.List(a.ctx)
}

func (a *App) ContainerInspect(connectionID, containerID string) (container.InspectResult, error) {
	p, err := a.containerManager.Provider(connectionID)
	if err != nil {
		return container.InspectResult{}, err
	}
	return p.Inspect(a.ctx, containerID)
}

func (a *App) ContainerAction(connectionID, containerID, action string) error {
	p, err := a.containerManager.Provider(connectionID)
	if err != nil {
		return err
	}
	return p.Action(a.ctx, containerID, action)
}

func (a *App) ContainerRename(connectionID, containerID, newName string) error {
	p, err := a.containerManager.Provider(connectionID)
	if err != nil {
		return err
	}
	return p.Rename(a.ctx, containerID, newName)
}

func (a *App) ContainerStats(connectionID string) ([]container.Stats, error) {
	p, err := a.containerManager.Provider(connectionID)
	if err != nil {
		return nil, err
	}
	return p.Stats(a.ctx)
}

func (a *App) ContainerImages(connectionID string) ([]container.Image, error) {
	p, err := a.containerManager.Provider(connectionID)
	if err != nil {
		return nil, err
	}
	return p.Images(a.ctx)
}

func (a *App) ContainerRemoveImage(connectionID, imageID string) error {
	p, err := a.containerManager.Provider(connectionID)
	if err != nil {
		return err
	}
	return p.RemoveImage(a.ctx, imageID)
}

func (a *App) ContainerCreate(connectionID string, opts container.CreateOptions) error {
	p, err := a.containerManager.Provider(connectionID)
	if err != nil {
		return err
	}
	return p.Create(a.ctx, opts)
}

func (a *App) ContainerNamespaces(connectionID string) ([]string, error) {
	p, err := a.containerManager.Provider(connectionID)
	if err != nil {
		return nil, err
	}
	return p.Namespaces(a.ctx)
}

func (a *App) ContainerSetNamespace(connectionID, ns string) error {
	return a.containerManager.SetNamespace(connectionID, ns)
}

func (a *App) ContainerStartLogs(connectionID, containerID string, tail int, timestamps bool) (string, error) {
	return a.containerManager.StartLogStream(connectionID, containerID, tail, timestamps)
}

func (a *App) ContainerStartPull(connectionID, image string) (string, error) {
	return a.containerManager.StartPullStream(connectionID, image)
}

func (a *App) ContainerStopStream(streamID string) {
	a.containerManager.StopStream(streamID)
}

func (a *App) ContainerExecSession(connectionID, containerID, shell string) (*session.SessionInfo, error) {
	if a.containerManager == nil {
		return nil, fmt.Errorf("container manager not initialized")
	}
	// initial size fallback; real size arrives via Resize after the frontend mounts xterm
	pty, err := a.containerManager.Exec(connectionID, containerID, shell, 80, 24)
	if err != nil {
		return nil, err
	}
	id := uuid.New().String()
	sess := session.NewContainerExecSession(id, pty)
	sess.SetOnDataCallback(func(data []byte) {
		runtime.EventsEmit(a.ctx, "session:data", map[string]interface{}{
			"id":   sess.ID(),
			"data": string(data),
		})
	})
	sess.SetOnStatusChangeCallback(func(status session.SessionStatus) {
		runtime.EventsEmit(a.ctx, "session:status", map[string]interface{}{
			"id":     sess.ID(),
			"status": status,
		})
	})
	a.sessionManager.Add(sess)
	return &session.SessionInfo{ID: id, Type: "container-exec", Title: containerID, Status: session.StatusConnected}, nil
}
