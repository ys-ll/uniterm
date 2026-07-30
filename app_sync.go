package main

import (
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/sync"
)

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
	// Skip the sync when the app is hidden. Saves triggered while the
	// user is away (background tab, minimised window, macOS App Nap)
	// would otherwise hit git fetch + push + PBKDF2 + AES with no
	// benefit. The next foreground save trigger will re-arm coalesced
	// sync.
	if !a.IsForeground() {
		return
	}
	// Coalesce triggerAutoSync. A burst of saves (e.g. pasting 50
	// commands into QuickCommands) used to launch 50 concurrent git
	// pull+push coroutines. We run at most one at a time and remember if
	// a new trigger arrived during the run so we fire exactly one
	// follow-up.
	if !a.syncInFlight.CompareAndSwap(false, true) {
		a.syncPending.Store(true)
		return
	}
	a.syncPending.Store(false)
	go func() {
		defer a.syncInFlight.Store(false)
		for {
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
			if !a.syncPending.CompareAndSwap(true, false) {
				return
			}
		}
	}()
}

// waitSyncReady briefly blocks on the async NewSyncService's Ready()
// channel so callers that arrive during the ~ms-scale startup window
// don't fail with "sync service not initialized". Returns true once
// ready, false on timeout.
func (a *App) waitSyncReady(timeout time.Duration) bool {
	if a.syncService == nil {
		return false
	}
	select {
	case <-a.syncService.Ready():
		return true
	case <-time.After(timeout):
		return false
	}
}

func (a *App) SyncGetConfig() (sync.SyncConfig, error) {
	if a.syncService == nil {
		return sync.SyncConfig{}, fmt.Errorf("sync service not initialized")
	}
	if !a.waitSyncReady(time.Second) {
		return sync.SyncConfig{}, fmt.Errorf("sync service still initializing")
	}
	return a.syncService.GetConfig()
}

// SyncSaveConfig saves the sync configuration.
func (a *App) SyncSaveConfig(config sync.SyncConfig, token string) error {
	if a.syncService == nil {
		return fmt.Errorf("sync service not initialized")
	}
	if !a.waitSyncReady(time.Second) {
		return fmt.Errorf("sync service still initializing")
	}
	return a.syncService.SaveConfig(config, token)
}

// SyncNow runs an immediate sync.
func (a *App) SyncNow() (*sync.SyncResult, error) {
	if a.syncService == nil {
		return nil, fmt.Errorf("sync service not initialized")
	}
	if !a.waitSyncReady(time.Second) {
		return nil, fmt.Errorf("sync service still initializing")
	}
	// Coalesce concurrent SyncNow calls. Multiple Settings→Sync clicks
	// while a slow git push is in flight used to stack up N concurrent
	// sync goroutines, each holding the Wails handler thread. If a sync
	// is already running we return the "skipped" sentinel direction so
	// the frontend's syncing flag still flips but no extra work is done.
	if a.syncInFlight.Load() {
		return &sync.SyncResult{Direction: sync.SyncSkipped, Message: "sync already in flight"}, nil
	}
	a.syncInFlight.Store(true)
	defer a.syncInFlight.Store(false)

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
	if !a.waitSyncReady(time.Second) {
		return nil, fmt.Errorf("sync service still initializing")
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
	if !a.waitSyncReady(time.Second) {
		return fmt.Errorf("sync service still initializing")
	}
	return a.syncService.TestConnection()
}

// SyncConfigureRepo sets up a new or existing sync repository.
func (a *App) SyncConfigureRepo(repoURL, username, token, masterPassword string) (*sync.SyncResult, error) {
	if a.syncService == nil {
		return nil, fmt.Errorf("sync service not initialized")
	}
	if !a.waitSyncReady(time.Second) {
		return nil, fmt.Errorf("sync service still initializing")
	}
	result, err := a.syncService.ConfigureRepo(repoURL, username, token, masterPassword)
	if err == nil {
		a.reloadStoresAfterSync()
		runtime.EventsEmit(a.ctx, "sync:completed")
	}
	return result, err
}

// SyncConfigureLocalRepo sets up a local-only sync backup directory.
func (a *App) SyncConfigureLocalRepo(localPath, masterPassword string) (*sync.SyncResult, error) {
	if a.syncService == nil {
		return nil, fmt.Errorf("sync service not initialized")
	}
	if !a.waitSyncReady(time.Second) {
		return nil, fmt.Errorf("sync service still initializing")
	}
	result, err := a.syncService.ConfigureLocalRepo(localPath, masterPassword)
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
	if !a.waitSyncReady(time.Second) {
		return fmt.Errorf("sync service still initializing")
	}
	return a.syncService.ChangePassword(oldPassword, newPassword)
}

// SyncVerifyPassword verifies the given password can decrypt the repo config.
func (a *App) SyncVerifyPassword(password, username, token string) error {
	if a.syncService == nil {
		return fmt.Errorf("sync service not initialized")
	}
	if !a.waitSyncReady(time.Second) {
		return fmt.Errorf("sync service still initializing")
	}
	return a.syncService.VerifySyncPassword(password, username, token)
}

// SyncDeleteRepo removes the sync repository configuration.
func (a *App) SyncDeleteRepo() error {
	if a.syncService == nil {
		return fmt.Errorf("sync service not initialized")
	}
	if !a.waitSyncReady(time.Second) {
		return fmt.Errorf("sync service still initializing")
	}
	return a.syncService.DeleteRepo()
}
