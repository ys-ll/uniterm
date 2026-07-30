package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"

	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/session"
)

// ── Session output log ──

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
	// Maintain the inverse index so panel→session lookups in
	// panelLogTitle / EnableSessionOutputLog / DisableSessionOutputLog
	// are O(1) instead of scanning every binding.
	a.sessionToPanel[sessionID] = panelID
	a.panelToSession[panelID] = sessionID
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

// UnregisterSession clears the session→panel binding and detaches any
// writer from the session. The logger itself is unaffected: it stays on
// the panel, waiting for the next session (reconnect) to register.
func (a *App) UnregisterSession(sessionID string) {
	if sessionID == "" {
		return
	}
	a.panelLogMu.Lock()
	if panelID, ok := a.sessionToPanel[sessionID]; ok {
		delete(a.sessionToPanel, sessionID)
		// Only clear the inverse index when it still points at the session
		// we're unregistering — the panel may already be bound to a
		// different session (the next reconnect).
		if a.panelToSession[panelID] == sessionID {
			delete(a.panelToSession, panelID)
		}
	}
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
	// O(1) lookup via the inverse index maintained in
	// RegisterSessionForPanel / UnregisterSession.
	a.panelLogMu.Lock()
	sessionID := a.panelToSession[panelID]
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
// The log is bound to the panel, not the session — so a reconnect
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
	// O(1) inverse-index lookup instead of scanning sessionToPanel.
	sessionID := a.panelToSession[panelID]
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
	// O(1) inverse-index lookup instead of scanning sessionToPanel.
	sessionID := a.panelToSession[panelID]
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
	isDir := false
	if info, err := os.Stat(abs); err == nil {
		isDir = info.IsDir()
	}
	switch goruntime.GOOS {
	case "windows":
		// explorer.exe returns exit code 1 on success; ignore Run's error.
		if isDir {
			_ = exec.Command("explorer", abs).Run()
		} else {
			_ = exec.Command("explorer", "/select,", abs).Run()
		}
		return nil
	case "darwin":
		if isDir {
			return exec.Command("open", abs).Run()
		}
		return exec.Command("open", "-R", abs).Run()
	default:
		if isDir {
			return exec.Command("xdg-open", abs).Run()
		}
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
