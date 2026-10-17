package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/mcp"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
)

// MCP server wiring: settings persistence (mcp.json), token management, the
// approval bridge (Wails events ↔ dialog component), audit logging, and the
// Env glue between backend/mcp and the session/connection stores.

// MCPTokenRecord is one persisted token (hash only).
type MCPTokenRecord struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
}

// MCPTokensFile is the on-disk shape of mcp.json.
type MCPTokensFile struct {
	Tokens []MCPTokenRecord `json:"tokens"`
}

// MCPStatus reports runtime state to the frontend.
type MCPStatus struct {
	Running bool `json:"running"`
	Port    int  `json:"port"`
}

const mcpTokensFileName = "mcp.json"
const mcpAuditFileName = "mcp-audit.log"

func (a *App) mcpTokensPath() string {
	return filepath.Join(a.dataDir, mcpTokensFileName)
}

// mcpServer is lazily created once stores are ready.
func (a *App) ensureMCPServer() *mcp.Server {
	a.mcpOnce.Do(func() {
		env := mcp.Env{
			Sessions:         a.mcpSessionExec,
			Commands:         a.mcpCommandOwner,
			ListSessions:     a.mcpListSessions,
			ListConnections:  a.mcpListConnections,
			Connect:          a.mcpConnect,
			OpenTerminal:     a.mcpOpenTerminal,
			Approve:          a.mcpApprove,
			Audit:            a.mcpAudit,
			Policy:           a.mcpPolicy,
			ResolveToken:     a.mcpResolveToken,
			FileSession:      a.mcpFileSession,
			ResolveLocalPath: a.mcpResolveLocalPath,
			Foreground:       a,
			ExecMode:         a.mcpExecMode,
			Capture:          a.CaptureScreen,
			SendKey:          a.SendKey,
			Collect:          a.StartForegroundCapture,
		}
		a.mcpServer = mcp.NewServer(env)
	})
	return a.mcpServer
}

// StartMCP brings the MCP endpoint up per local state and installs the
// active token set. Called from initStores (auto-start) and from
// SaveLocalState when the user toggles the switch.
func (a *App) StartMCP() error {
	if a.localStateStore == nil {
		return fmt.Errorf("local state store not initialized")
	}
	srv := a.ensureMCPServer()
	state, err := a.localStateStore.Load()
	if err != nil {
		return err
	}
	cfg := state.MCP
	if cfg == nil || !cfg.Enabled {
		srv.Stop()
		return nil
	}
	srv.SetTokens(a.mcpLoadTokens())
	port := mcp.DefaultPort
	if cfg.Port > 0 {
		port = cfg.Port
	}
	return srv.Start(port)
}

// MCPStatus_ reports endpoint state to the settings UI.
func (a *App) GetMCPStatus() MCPStatus {
	if a.mcpServer == nil {
		return MCPStatus{}
	}
	return MCPStatus{Running: a.mcpServer.Running(), Port: a.mcpServer.Port()}
}

// GenerateMCPToken creates a new named token, persists its hash, returns the
// plaintext exactly once.
func (a *App) GenerateMCPToken(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("token name is required")
	}
	token, hash := mcp.GenerateToken()
	if err := a.mcpSaveToken(MCPTokenRecord{Name: name, Hash: hash}); err != nil {
		return "", err
	}
	return token, nil
}

// RevokeMCPToken removes one token by name.
func (a *App) RevokeMCPToken(name string) error {
	data, err := a.mcpReadTokens()
	if err != nil {
		return err
	}
	kept := data.Tokens[:0]
	for _, t := range data.Tokens {
		if t.Name != name {
			kept = append(kept, t)
		}
	}
	data.Tokens = kept
	return a.mcpWriteTokens(data)
}

// ListMCPTokens returns token names (no hashes).
func (a *App) ListMCPTokens() ([]string, error) {
	data, err := a.mcpReadTokens()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(data.Tokens))
	for _, t := range data.Tokens {
		names = append(names, t.Name)
	}
	return names, nil
}

func (a *App) mcpSaveToken(rec MCPTokenRecord) error {
	data, err := a.mcpReadTokens()
	if err != nil {
		return err
	}
	// Replace a token with the same name (regenerate = rotate).
	for i, t := range data.Tokens {
		if t.Name == rec.Name {
			data.Tokens[i] = rec
			return a.mcpWriteTokens(data)
		}
	}
	data.Tokens = append(data.Tokens, rec)
	return a.mcpWriteTokens(data)
}

func (a *App) mcpReadTokens() (MCPTokensFile, error) {
	data, err := os.ReadFile(a.mcpTokensPath())
	if err != nil {
		if os.IsNotExist(err) {
			return MCPTokensFile{Tokens: []MCPTokenRecord{}}, nil
		}
		return MCPTokensFile{}, err
	}
	var f MCPTokensFile
	if err := json.Unmarshal(data, &f); err != nil {
		return MCPTokensFile{}, err
	}
	if f.Tokens == nil {
		f.Tokens = []MCPTokenRecord{}
	}
	return f, nil
}

func (a *App) mcpWriteTokens(f MCPTokensFile) error {
	buf, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(a.dataDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(a.mcpTokensPath(), buf, 0600); err != nil {
		return err
	}
	// Hot-reload the live server's token set.
	if a.mcpServer != nil {
		a.mcpServer.SetTokens(a.mcpLoadTokens())
	}
	return nil
}

// mcpLoadTokens maps the persisted records into the server's live map.
func (a *App) mcpLoadTokens() map[string]mcp.TokenInfo {
	data, err := a.mcpReadTokens()
	if err != nil {
		log.Writef("mcp: read tokens: %v", err)
		return map[string]mcp.TokenInfo{}
	}
	out := make(map[string]mcp.TokenInfo, len(data.Tokens))
	for _, t := range data.Tokens {
		out[t.Hash] = mcp.TokenInfo{Name: t.Name}
	}
	return out
}

// mcpResolveToken backs Env.ResolveToken: resolves a token hash to its name,
// re-reading mcp.json when its mtime changed (tokens generated/revoked from
// the UI hot-reload; the file is also authoritative for external writers).
func (a *App) mcpResolveToken(hash string) string {
	a.mcpTokenCacheMu.Lock()
	defer a.mcpTokenCacheMu.Unlock()
	fi, err := os.Stat(a.mcpTokensPath())
	if err != nil || fi.ModTime() != a.mcpTokenCacheMtime {
		// Reload (also resets the mtime on failure so we don't hammer the
		// disk on every request when the file is unreadable).
		a.mcpTokenCache = a.mcpLoadTokens()
		a.mcpTokenCacheMtime = fi.ModTime()
	}
	if info, ok := a.mcpTokenCache[hash]; ok {
		return info.Name
	}
	return ""
}

// ── Env glue ─────────────────────────────────────────────────────

func (a *App) mcpSessionExec(sessionID string) (mcp.SSHExecutor, bool) {
	if a.sessionManager == nil {
		return nil, false
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return nil, false
	}
	ssh, ok := s.(*session.SSHSession)
	if !ok {
		return nil, false
	}
	return ssh, true
}

func (a *App) mcpCommandOwner(commandID string) (string, mcp.SSHExecutor, bool) {
	// Commands are registered globally in the session package; resolve by
	// scanning live SSH sessions (tens at most).
	if a.sessionManager == nil {
		return "", nil, false
	}
	for _, s := range a.sessionManager.List() {
		if exec, ok := a.mcpSessionExec(s.ID); ok {
			if _, _, _, state, _ := exec.MCPLatestOutput(commandID, 0, 1); state != "unknown" {
				return s.ID, exec, true
			}
		}
	}
	return "", nil, false
}

func (a *App) mcpListSessions() []mcp.SessionSummary {
	if a.sessionManager == nil {
		return []mcp.SessionSummary{}
	}
	out := make([]mcp.SessionSummary, 0)
	mode := a.mcpExecMode()
	for _, s := range a.sessionManager.List() {
		if !mcpTerminalAllowed(mode, s.Type) {
			continue
		}
		sum := mcp.SessionSummary{
			ID:     s.ID,
			Type:   s.Type,
			Title:  s.Title,
			Status: string(s.Status),
		}
		if cwd := session.GetSessionCwd(s.ID); cwd != "" {
			sum.Cwd = cwd
		}
		out = append(out, sum)
	}
	return out
}

// ── MCP file tools: companion file-transfer session ─────────────────────
//
// Instead of a bespoke SFTP client pool riding the terminal's SSH client,
// the MCP layer opens the same companion file-transfer session the frontend
// file sidebar opens — an SFTPSession or SCPSession per the connection's
// FileTransferProto, lazily created on the first MCP file call and cached
// per SSH session. All transfer/list/read behavior (progress events into
// the task center, pause/cancel/retry, uid/gid resolution) is the sidebar's
// own, and hosts whose SSH server provides no SFTP subsystem (common on
// embedded devices) work by setting the connection's file protocol to SCP.

// mcpTransferOutcome is what a completed transfer tells its MCP waiter.
type mcpTransferOutcome struct {
	status string // "done" | "error" | "cancelled"
	errMsg string
}

// mcpDispatchTransferEvent feeds "complete" transfer events to registered
// MCP waiters. Installed inside the global TransferEventSink; no-op overhead
// for transfers nobody waits on.
func (a *App) mcpDispatchTransferEvent(payload map[string]any) {
	if event, _ := payload["event"].(string); event != "complete" {
		return
	}
	taskID, _ := payload["taskId"].(string)
	if taskID == "" {
		return
	}
	a.mcpTransferMu.Lock()
	ch, ok := a.mcpTransferWaiters[taskID]
	if ok {
		delete(a.mcpTransferWaiters, taskID)
	}
	a.mcpTransferMu.Unlock()
	if !ok {
		return
	}
	out := mcpTransferOutcome{status: "done"}
	if status, _ := payload["status"].(string); status == "error" || status == "cancelled" {
		out.status = status
		out.errMsg, _ = payload["error"].(string)
	}
	select {
	case ch <- out:
	default:
	}
}

// mcpRunTransfer starts a transfer and synchronously waits for its
// completion event. The waiter is registered while holding mcpTransferMu so
// a transfer that completes between start() returning and registration is
// not missed (dispatch blocks on the same mutex). Error text comes from the
// completion event, so the AI sees why a transfer failed.
func (a *App) mcpRunTransfer(start func() (string, error), timeout time.Duration) error {
	a.mcpTransferMu.Lock()
	taskID, err := start()
	if err != nil {
		a.mcpTransferMu.Unlock()
		return err
	}
	ch := make(chan mcpTransferOutcome, 1)
	a.mcpTransferWaiters[taskID] = ch
	a.mcpTransferMu.Unlock()
	defer func() {
		a.mcpTransferMu.Lock()
		delete(a.mcpTransferWaiters, taskID)
		a.mcpTransferMu.Unlock()
	}()
	select {
	case out := <-ch:
		switch out.status {
		case "error":
			if out.errMsg != "" {
				return fmt.Errorf("transfer failed: %s", out.errMsg)
			}
			return fmt.Errorf("transfer failed")
		case "cancelled":
			return fmt.Errorf("transfer cancelled (task %s)", taskID)
		}
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("transfer did not finish within %s (task %s may still be running — see the transfer task center)", timeout, taskID)
	}
}

// mcpFileSession resolves an MCP file tool call to the SSH session's
// companion file-transfer session, creating it on first use.
func (a *App) mcpFileSession(sessionID string) (mcp.FileExecutor, error) {
	if a.sessionManager == nil {
		return nil, fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	ssh, ok := s.(*session.SSHSession)
	if !ok {
		return nil, fmt.Errorf("session %s is not an SSH session", sessionID)
	}
	fs, err := a.mcpEnsureFileSession(ssh)
	if err != nil {
		return nil, err
	}
	fts, ok := fs.(fileTransferSession)
	if !ok {
		return nil, fmt.Errorf("transfer session %s is not a file transfer backend", fs.ID())
	}
	return fileExecutorAdapter{app: a, fs: fts}, nil
}

// mcpEnsureFileSession returns the cached companion file session for the SSH
// session, or creates one. The companion is closed when the SSH session
// disconnects; a cached-but-dead companion is replaced on the next call.
func (a *App) mcpEnsureFileSession(ssh *session.SSHSession) (session.Session, error) {
	sshSID := ssh.ID()
	a.mcpFileMu.Lock()
	fileSID, ok := a.mcpFileSessions[sshSID]
	a.mcpFileMu.Unlock()
	if ok {
		if fs, ok := a.sessionManager.Get(fileSID); ok && fs.Status() == session.StatusConnected {
			return fs, nil
		}
		// Stale entry: tear the leftover down and recreate below.
		_ = a.sessionManager.Close(fileSID)
		a.mcpFileMu.Lock()
		delete(a.mcpFileSessions, sshSID)
		a.mcpFileMu.Unlock()
	}
	if ssh.Status() != session.StatusConnected {
		return nil, fmt.Errorf("ssh session %s is not connected", sshSID)
	}
	config := ssh.ConnectionConfig()
	fs, err := a.mcpCreateFileSession(sshSID, config)
	if err != nil {
		return nil, err
	}
	createdSID := fs.ID()
	a.mcpFileMu.Lock()
	a.mcpFileSessions[sshSID] = createdSID
	a.mcpFileMu.Unlock()
	// Follow the SSH session down so the companion never outlives its
	// terminal. Only the registered generation cleans up.
	ssh.AddStatusListener(func(st session.SessionStatus) {
		if st != session.StatusDisconnected && st != session.StatusError {
			return
		}
		a.mcpFileMu.Lock()
		cur, ok := a.mcpFileSessions[sshSID]
		if ok && cur == createdSID {
			delete(a.mcpFileSessions, sshSID)
		} else {
			ok = false
		}
		a.mcpFileMu.Unlock()
		if ok {
			log.Writef("[mcp-file] ssh session %s gone; closing transfer session %s", sshSID, createdSID)
			_ = a.sessionManager.Close(createdSID)
		}
	})
	return fs, nil
}

// mcpCreateFileSession creates and connects the companion file-transfer
// session (SFTP or SCP per config.FileTransferProto) with the same
// credential resolution chain the terminal connect path uses. Connect runs
// synchronously so MCP callers get the real failure reason.
func (a *App) mcpCreateFileSession(sshSID string, config session.ConnectionConfig) (session.Session, error) {
	proto := "sftp"
	if config.FileTransferProto == "scp" {
		proto = "scp"
	}
	// Same credential resolution as CreateSession/SessionStart
	// (app_terminal.go): identity and proxy materialization. The SSH
	// session's stored config is already materialized, so these are no-ops
	// except for channel clones, whose frontend-passed config skipped
	// resolution.
	if config.AuthType == "identity" {
		mc, err := a.materializeIdentity(config)
		if err != nil {
			return nil, fmt.Errorf("transfer session (%s): %w", proto, err)
		}
		config = mc
	}
	if mc, err := a.materializeProxy(config); err != nil {
		return nil, fmt.Errorf("transfer session (%s): %w", proto, err)
	} else {
		config = mc
	}
	s, err := a.sessionManager.Create(proto, config)
	if err != nil {
		return nil, fmt.Errorf("transfer session (%s): %w", proto, err)
	}
	fail := func(err error) (session.Session, error) {
		_ = a.sessionManager.Close(s.ID())
		return nil, fmt.Errorf("%s transfer session connect failed: %w (hosts with interactive logon need the password saved or key auth configured — same as the file sidebar)", proto, err)
	}
	if setter, ok := s.(interface{ SetLogIdentity(string, string) }); ok {
		setter.SetLogIdentity(config.Name, config.Host)
	}
	// Mirror CreateSession's concurrency limit for file-transfer sessions.
	n := config.SftpMaxConcurrency
	if n <= 0 {
		n = 5
	}
	if sftp, ok := s.(*session.SFTPSession); ok {
		sftp.SetMaxConcurrency(n)
	}
	if scp, ok := s.(*session.SCPSession); ok {
		scp.SetMaxConcurrency(n)
	}
	// Status events so the session's lifecycle stays observable (task
	// center transfers already flow through the global TransferEventSink).
	s.SetOnStatusChangeCallback(func(st session.SessionStatus) {
		a.emit("session:status", map[string]interface{}{
			"id":     s.ID(),
			"status": st,
		})
	})
	// Jump-host tunnel first (launchConnectGoroutine's ordering), then the
	// dial — for tunneled terminals the stored config still carries
	// TunnelSSHConnID, and each companion gets its own fresh forward.
	if err := a.setupJumpHostTunnel(s.ID(), proto, &config); err != nil {
		return fail(err)
	}
	if err := s.Connect(config); err != nil {
		return fail(err)
	}
	log.Writef("[mcp-file] opened %s transfer session %s for ssh session %s", proto, s.ID(), sshSID)
	return s, nil
}

// mcpTransferTimeout bounds a single MCP upload/download wait. The transfer
// keeps running in the task center when the wait expires.
const mcpTransferTimeout = 10 * time.Minute

// fileExecutorAdapter exposes a companion file session as the mcp package's
// FileExecutor, converting types and adding the synchronous wait. fs is the
// same fileTransferSession contract the frontend Sftp* bindings dispatch
// through.
type fileExecutorAdapter struct {
	app *App
	fs  fileTransferSession
}

func (a fileExecutorAdapter) MCPListDir(remotePath string) ([]mcp.FileEntry, error) {
	res, err := a.fs.ListRemote(remotePath)
	if err != nil {
		return nil, err
	}
	files := res.Files
	// Dirs first, then name — mirrors the old MCP listing order.
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})
	if len(files) > mcp.DefaultMCPDirEntries {
		files = files[:mcp.DefaultMCPDirEntries]
	}
	out := make([]mcp.FileEntry, 0, len(files))
	for _, f := range files {
		var mtime int64
		if t, err := time.Parse(time.RFC3339, f.ModTime); err == nil {
			mtime = t.UnixMilli()
		}
		out = append(out, mcp.FileEntry{
			Name:    f.Name,
			Size:    f.Size,
			IsDir:   f.IsDir,
			ModTime: mtime,
			Mode:    f.Mode,
		})
	}
	return out, nil
}

func (a fileExecutorAdapter) MCPWriteFile(localPath, remotePath string) (int64, error) {
	if err := a.app.mcpRunTransfer(func() (string, error) {
		return a.fs.Put(localPath, remotePath, false)
	}, mcpTransferTimeout); err != nil {
		return 0, err
	}
	return localFileSize(localPath)
}

func (a fileExecutorAdapter) MCPReadRemoteToFile(remotePath, localPath string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return 0, err
	}
	if err := a.app.mcpRunTransfer(func() (string, error) {
		return a.fs.Get(remotePath, localPath, false)
	}, mcpTransferTimeout); err != nil {
		return 0, err
	}
	return localFileSize(localPath)
}

func localFileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// mcpResolveLocalPath validates an agent-supplied local path against the
// user's SFTP local bookmarks (settings.sftpBookmarks.localPaths): resolved
// paths must sit inside one of them; an empty list rejects everything.
func (a *App) mcpResolveLocalPath(path string) (string, error) {
	settings, err := a.settingsStore.Load()
	if err != nil {
		return "", err
	}
	return mcp.ResolveMcpLocalPath(path, settings.SFTPBookmarks.LocalPaths)
}

func (a *App) mcpListConnections() []mcp.ConnectionSummary {
	if a.connectionStore == nil {
		return []mcp.ConnectionSummary{}
	}
	data, err := a.connectionStore.Load()
	if err != nil {
		return []mcp.ConnectionSummary{}
	}
	out := make([]mcp.ConnectionSummary, 0, len(data.Connections))
	mode := a.mcpExecMode()
	for _, c := range data.Connections {
		if !mcpTerminalAllowed(mode, c.Type) {
			continue
		}
		out = append(out, mcp.ConnectionSummary{
			ID:   c.ID,
			Name: c.Name,
			Type: c.Type,
			Host: c.Host,
			Port: c.Port,
			User: c.User,
		})
	}
	return out
}

func mcpTerminalAllowed(mode, kind string) bool {
	if mode == mcp.ModeForeground {
		return mcp.IsTerminalType(kind)
	}
	return kind == "ssh"
}

// Both MCP modes reuse the normal session creation and frontend sizing flow.
func (a *App) mcpConnect(connectionID string) (string, error) {
	return a.mcpOpenTerminal(connectionID, "", "")
}

func (a *App) mcpOpenTerminal(connectionID, terminalType, shellPath string) (string, error) {
	var config session.ConnectionConfig
	if connectionID != "" {
		if a.connectionStore == nil {
			return "", fmt.Errorf("connection store not initialized")
		}
		data, err := a.connectionStore.Load()
		if err != nil {
			return "", err
		}
		found := false
		for _, c := range data.Connections {
			if c.ID == connectionID {
				config = c
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("connection %s not found", connectionID)
		}
		terminalType = config.Type
	} else {
		if terminalType != "local" && terminalType != "wsl" {
			return "", fmt.Errorf("an unsaved terminal must have type local or wsl")
		}
		config.Type = terminalType
		config.Name = terminalType
		config.ShellPath = shellPath
	}
	if !mcpTerminalAllowed(a.mcpExecMode(), terminalType) {
		return "", fmt.Errorf("terminal type %s is unavailable in this execution mode", terminalType)
	}
	info, err := a.CreateSession(terminalType, config)
	if err != nil {
		return "", err
	}
	a.emit("mcp:session-created", map[string]interface{}{
		"sessionId": info.ID, "type": terminalType, "name": config.Name, "host": config.Host, "config": config,
	})
	return info.ID, nil
}

// ── Approval bridge ──────────────────────────────────────────────

// mcpConnectionLabel resolves the approval request's raw target — a session
// ID for the exec/file tools, a connection-profile ID for the connect tool —
// into a human-readable "tab title · host" label for the approval dialog.
// Falls back to the raw ID when neither registry knows it.
func (a *App) mcpConnectionLabel(target string) string {
	if target == "" {
		return ""
	}
	// Session target: tab title plus the host recorded at creation.
	if s, ok := a.sessionManager.Get(target); ok {
		title := s.Title()
		label := title
		if li, ok := s.(interface{ LogIdentity() (name, host string) }); ok {
			if _, host := li.LogIdentity(); host != "" && host != title {
				label = title + " · " + host
			}
		}
		return label
	}
	// Connection-profile target (connect tool): profile name + host:port.
	if a.connectionStore != nil {
		if data, err := a.connectionStore.Load(); err == nil {
			for _, c := range data.Connections {
				if c.ID == target {
					label := c.Name
					if c.Host != "" && c.Host != c.Name {
						label = c.Name + " · " + c.Host
					}
					return label
				}
			}
		}
	}
	return target
}

// mcpApprove emits mcp:approval-request and blocks for the verdict via
// ResolveMCPApproval (frontend dialog) or the timeout.
func (a *App) mcpApprove(req mcp.ApprovalRequest) error {
	ch := make(chan mcpApprovalVerdict, 1)
	a.mcpApprovalsMu.Lock()
	a.mcpApprovals[req.ID] = ch
	a.mcpApprovalsMu.Unlock()
	defer func() {
		a.mcpApprovalsMu.Lock()
		delete(a.mcpApprovals, req.ID)
		a.mcpApprovalsMu.Unlock()
	}()

	a.emit("mcp:approval-request", map[string]interface{}{
		"id":         req.ID,
		"client":     req.Client,
		"connection": a.mcpConnectionLabel(req.Connection),
		"command":    req.Command,
		"createdAt":  req.CreatedAt,
	})

	// High-priority surface: the dialog lives in the app window, but a
	// pending request is easy to miss (another app focused, another tab
	// active). Escalate to the OS on every request regardless of foreground
	// state: system notification + window raise.
	a.notifyMCPApproval(req)

	// No-window guard: if the frontend never answers (window closed), the
	// timeout below denies. Frontend dialogs auto-dismiss on timeout too.
	select {
	case verdict := <-ch:
		if !verdict.Approved {
			return fmt.Errorf("denied by user: %s", verdict.Reason)
		}
		return nil
	case <-time.After(mcp.DefaultApprovalTimeout):
		return fmt.Errorf("approval timed out after %s (user unavailable)", mcp.DefaultApprovalTimeout)
	}
}

// mcpApprovalVerdict is the frontend's answer.
type mcpApprovalVerdict struct {
	Approved bool
	Reason   string
}

// ResolveMCPApproval is the Wails binding the frontend calls when the user
// answers the approval dialog.
func (a *App) ResolveMCPApproval(requestID string, approved bool, reason string) error {
	a.mcpApprovalsMu.Lock()
	ch, ok := a.mcpApprovals[requestID]
	a.mcpApprovalsMu.Unlock()
	if !ok {
		return fmt.Errorf("no pending approval %s", requestID)
	}
	select {
	case ch <- mcpApprovalVerdict{Approved: approved, Reason: reason}:
	default:
	}
	return nil
}

// ── Audit ────────────────────────────────────────────────────────

var mcpAuditMu sync.Mutex

func (a *App) mcpAudit(entry mcp.AuditEntry) {
	if a.dataDir == "" {
		return
	}
	mcpAuditMu.Lock()
	defer mcpAuditMu.Unlock()
	f, err := os.OpenFile(filepath.Join(a.dataDir, mcpAuditFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	buf, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = f.Write(append(buf, '\n'))
}

// ── Local-state-backed callbacks ─────────────────────────────────

func (a *App) mcpSettings() store.MCPSettings {
	if a.localStateStore == nil {
		return store.DefaultMCPSettings()
	}
	state, err := a.localStateStore.Load()
	if err != nil {
		return store.DefaultMCPSettings()
	}
	if state.MCP == nil {
		return store.DefaultMCPSettings()
	}
	return *state.MCP
}

func (a *App) mcpPolicy() mcp.Policy {
	// Unknown or empty stored values (older configs, hand-edited settings)
	// must not silently disable approvals: fall back to the default.
	switch p := mcp.Policy(a.mcpSettings().Policy); p {
	case mcp.PolicyConfirmAll, mcp.PolicyConfirmWrite, mcp.PolicyConfirmDangerous, mcp.PolicyBypass:
		return p
	default:
		return mcp.PolicyConfirmWrite
	}
}

// mcpExecMode returns the user-configured execution mode, defaulting to
// background. Read once per MCP server Start (which re-runs after every
// SaveLocalState) to pick the registered toolset.
func (a *App) mcpExecMode() string {
	if a.mcpSettings().ExecMode == mcp.ModeForeground {
		return mcp.ModeForeground
	}
	return mcp.ModeBackground
}

// ── Approval notification strings ───────────────────────────────
// MCP approval notification strings per UI language, mirroring the
// trayLabels table in main.go: the Go side can't use the frontend's i18n
// bundles, so the strings live here keyed by the settings language code;
// unknown or "system" falls back to English. The per-platform notification
// implementations (app_darwin.go / app_linux.go / app_windows.go) share
// this table.

type mcpNotifyStrings struct {
	Title       string // notification title
	BodyExec    string // body: command execution request (%s = client label)
	BodyConnect string // body: new connection request (%s = client label)
}

var mcpNotifyLabels = map[string]mcpNotifyStrings{
	"en":    {"uniTerm · MCP approval request", "%s requests to run a command — confirm in uniTerm", "%s requests a new connection — confirm in uniTerm"},
	"zh-CN": {"uniTerm · MCP 审批请求", "%s 请求执行命令,请在 uniTerm 中确认", "%s 请求建立新连接,请在 uniTerm 中确认"},
	"zh-TW": {"uniTerm · MCP 審批請求", "%s 請求執行命令,請在 uniTerm 中確認", "%s 請求建立新連線,請在 uniTerm 中確認"},
	"ja":    {"uniTerm · MCP承認リクエスト", "%s がコマンド実行を要求しています。uniTerm で確認してください", "%s が新しい接続を要求しています。uniTerm で確認してください"},
	"ko":    {"uniTerm · MCP 승인 요청", "%s이(가) 명령 실행을 요청했습니다. uniTerm에서 확인해 주세요", "%s이(가) 새 연결을 요청했습니다. uniTerm에서 확인해 주세요"},
	"de":    {"uniTerm · MCP-Freigabe-Anfrage", "%s möchte einen Befehl ausführen — in uniTerm bestätigen", "%s möchte eine neue Verbindung öffnen — in uniTerm bestätigen"},
	"es":    {"uniTerm · Solicitud de aprobación MCP", "%s solicita ejecutar un comando; confírmalo en uniTerm", "%s solicita una nueva conexión; confírmalo en uniTerm"},
	"fr":    {"uniTerm · Demande d'approbation MCP", "%s demande à exécuter une commande — à confirmer dans uniTerm", "%s demande une nouvelle connexion — à confirmer dans uniTerm"},
	"ru":    {"uniTerm · запрос на одобрение MCP", "%s запрашивает выполнение команды — подтвердите в uniTerm", "%s запрашивает новое подключение — подтвердите в uniTerm"},
	"pt":    {"uniTerm · Solicitação de aprovação MCP", "%s solicita executar um comando — confirme no uniTerm", "%s solicita uma nova conexão — confirme no uniTerm"},
	"vi":    {"uniTerm · Yêu cầu phê duyệt MCP", "%s yêu cầu chạy lệnh — xác nhận trong uniTerm", "%s yêu cầu kết nối mới — xác nhận trong uniTerm"},
}

// mcpNotifyStringsFor returns the string set for lang, English fallback.
func mcpNotifyStringsFor(lang string) mcpNotifyStrings {
	if s, ok := mcpNotifyLabels[lang]; ok {
		return s
	}
	return mcpNotifyLabels["en"]
}

// mcpNotifyBody renders the notification body: inline command preview for
// short commands, truncated preview for long ones, the connect template when
// no command is involved.
func mcpNotifyBody(s mcpNotifyStrings, client, command string) string {
	switch {
	case command == "":
		return fmt.Sprintf(s.BodyConnect, client)
	case len(command) <= 60:
		return fmt.Sprintf("%s: %s", client, command)
	default:
		return fmt.Sprintf("%s: %s…", client, command[:57])
	}
}

// mcpNotifyLanguage resolves the UI language for notifications. Load errors
// and the "system" sentinel fall back to English (same as the tray menu).
func (a *App) mcpNotifyLanguage() string {
	if a.settingsStore == nil {
		return "en"
	}
	settings, err := a.settingsStore.Load()
	if err != nil {
		return "en"
	}
	return settings.Language
}

// ── Approval notification (all platforms) ───────────────────────

// notifyMCPApproval surfaces a pending MCP approval through the Wails
// notifications service when the window is unfocused, and brings the window
// forward (Focus/Show → Dock bounce on macOS, taskbar attention elsewhere).
// Platform backends live inside the service: UNUserNotificationCenter on
// macOS, wintoast on Windows, D-Bus org.freedesktop.Notifications on Linux
// (requires a running notification daemon). Strings come from
// mcpNotifyLabels above.
func (a *App) notifyMCPApproval(req mcp.ApprovalRequest) {
	if a.notifier == nil {
		log.Writef("mcp: notification skipped: notifications service unavailable")
	} else {
		strs := mcpNotifyStringsFor(a.mcpNotifyLanguage())
		body := mcpNotifyBody(strs, req.Client, req.Command)
		// First line: which tab/host the request targets (session title ·
		// host for exec/file tools, connection name · host for connect).
		if target := a.mcpConnectionLabel(req.Connection); target != "" {
			body = target + "\n" + body
		}
		// First-run authorization (macOS only; always granted on the other
		// platforms). Safe to call on every request.
		if ok, err := a.notifier.CheckNotificationAuthorization(); err == nil && !ok {
			if _, err := a.notifier.RequestNotificationAuthorization(); err != nil {
				log.Writef("mcp: notification authorization failed: %v", err)
			}
		}
		if err := a.notifier.SendNotification(notifications.NotificationOptions{
			ID:    "mcp-approval",
			Title: strs.Title,
			Body:  body,
		}); err != nil {
			log.Writef("mcp: notification failed: %v", err)
		}
	}

	// Bring the window forward so the approval dialog is directly in front
	// of the user.
	if a.window != nil {
		a.window.Focus()
		a.window.Show()
	}
}

// Unified async-job bridge: implements mcp.ForegroundJobs over the app's live
// session manager and the MCP foreground job registry. Background (non-PTY SSH
// exec) stays on the existing session.mcpExec path; this bridge owns the
// foreground live-PTY tap.

// tapSource is the additive-observer surface baseSession provides; the generic
// Session interface only exposes the single-slot setters.
type tapSource interface {
	AddDataTap(cb func([]byte)) func()
	AddStatusTap(cb func(session.SessionStatus)) func()
}

// ptyAdapter turns a generic session.Session into the narrow mcp.Pty
// surface. Data taps are additive (AddDataTap) so the frontend's own stream
// subscription is never replaced.
type ptyAdapter struct {
	a    *App
	s    session.Session
	taps tapSource
}

func (p ptyAdapter) Write(data []byte) error {
	if p.a != nil {
		return p.a.writeSession(p.s.ID(), data)
	}
	return p.s.Write(data)
}

func (p ptyAdapter) OnData(cb func([]byte)) func() {
	if p.taps != nil {
		return p.taps.AddDataTap(cb)
	}
	return func() {}
}

func (p ptyAdapter) OnStatusChange(cb func(string)) func() {
	if p.taps == nil {
		return func() {}
	}
	return p.taps.AddStatusTap(func(st session.SessionStatus) {
		cb(string(st))
	})
}

func (p ptyAdapter) Alive() bool { return p.s.Status() == session.StatusConnected }

// newlineFor returns the Enter bytes the shell expects. Windows OpenSSH and
// PowerShell need CR; POSIX shells accept LF (the frontend mirrors this).
func newlineFor(s session.Session) string {
	if runtime.GOOS == "windows" && (s.Type() == "local" || s.Type() == "wsl") {
		return "\r"
	}
	if ssh, ok := s.(*session.SSHSession); ok && ssh.RemoteOS() == "windows-openssh" {
		return "\r"
	}
	return "\n"
}

func (a *App) jobRegistry() *mcp.Registry {
	a.jobOnce.Do(func() {
		a.jobs = mcp.NewRegistry()
	})
	return a.jobs
}

// screenCols/screenRows is the emulator grid size used for foreground jobs
// and captures. A smaller real terminal only means shorter lines in the dump
// (content wraps at the remote's width); the grid is just the accumulator,
// so it is sized generously rather than tracked to the session's PTY size.
const (
	screenCols = 200
	screenRows = 60
)

// StartForeground implements mcp.ForegroundJobs and is the single execution
// entry for the live-terminal path: the MCP server's foreground toolset and
// the built-in AI panel's execute path both come through here, so MCP foreground job's
// per-session busy lock applies to both consumers equally.
func (a *App) StartForeground(sessionID, command string, idleMs, hardLimitMs int) (string, error) {
	if a.sessionManager == nil {
		return "", errors.New("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return "", errors.New("session not found: " + sessionID)
	}
	pty := ptyAdapter{a: a, s: s}
	if taps, ok := s.(tapSource); ok {
		pty.taps = taps
	}
	job, err := a.jobRegistry().StartWithScreen(
		pty, sessionID, command, newlineFor(s),
		time.Duration(idleMs)*time.Millisecond,
		time.Duration(hardLimitMs)*time.Millisecond,
		mcp.DefaultMaxOutput,
		screenCols, screenRows,
	)
	if err != nil {
		return "", err
	}
	return job.ID, nil
}

// PollForeground implements mcp.ForegroundJobs.
func (a *App) PollForeground(jobID string, offset, max int) (stdout, stderr []byte, exitCode int, state string, err error) {
	job, ok := a.jobRegistry().Lookup(jobID)
	if !ok {
		return nil, nil, -1, "unknown", mcp.ErrUnknownJob
	}
	out, errOut, code, st, jerr := job.Snapshot()
	if offset < 0 {
		offset = 0
	}
	if offset > len(out) {
		offset = len(out)
	}
	out = out[offset:]
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out, errOut, code, st, jerr
}

// StartForegroundCapture passively collects terminal output using the same
// job engine and completion rules as foreground command execution.
func (a *App) StartForegroundCapture(sessionID string, idleMs, hardLimitMs int) (string, error) {
	if a.sessionManager == nil {
		return "", errors.New("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}
	p := ptyAdapter{a: a, s: s}
	if taps, ok := s.(tapSource); ok {
		p.taps = taps
	}
	job, err := a.jobRegistry().ObserveWithScreen(p, sessionID,
		time.Duration(idleMs)*time.Millisecond, time.Duration(hardLimitMs)*time.Millisecond,
		screenCols, screenRows)
	if err != nil {
		return "", err
	}
	return job.ID, nil
}

// InterruptForeground implements mcp.ForegroundJobs.
func (a *App) InterruptForeground(jobID, _ string) error {
	return a.jobRegistry().Interrupt(jobID)
}

// screenCols/screenRows live above StartForeground; the capture emulator
// shares the same grid constants.

// sessionCapture is one session's screen emulator, attached before connecting
// so the first capture includes output already displayed by the terminal.
// Its additive taps are unsubscribed when the session disconnects.
type sessionCapture struct {
	mu        sync.Mutex
	screen    *mcp.Screen
	dataOff   func()
	statusOff func()
}

// CaptureScreen snapshots a live session's terminal screen — what the user
// currently sees, redraw-resolved. Serves the MCP uniterm_capture tool and
// the built-in panel's collect-output fallback.
func (a *App) CaptureScreen(sessionID string, tailLines int) (string, error) {
	if a.sessionManager == nil {
		return "", errors.New("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}
	cap := a.sessionCaptureFor(s)
	cap.mu.Lock()
	defer cap.mu.Unlock()
	if tailLines <= 0 {
		tailLines = 200
	}
	return cap.screen.Tail(tailLines), nil
}

// sessionCaptureFor lazily creates the emulator+tap pair for a session.
func (a *App) sessionCaptureFor(s session.Session) *sessionCapture {
	a.captureMu.Lock()
	defer a.captureMu.Unlock()
	if a.captures == nil {
		a.captures = make(map[string]*sessionCapture)
	}
	if c, ok := a.captures[s.ID()]; ok {
		return c
	}
	screen := mcp.NewScreen(screenCols, screenRows)
	c := &sessionCapture{screen: screen}
	if taps, ok := s.(tapSource); ok {
		c.dataOff = taps.AddDataTap(func(b []byte) {
			c.mu.Lock()
			screen.Feed(b)
			c.mu.Unlock()
		})
		// Detach asynchronously: the untap helpers take the session mutex,
		// which the tap callback itself may be running under.
		c.statusOff = taps.AddStatusTap(func(st session.SessionStatus) {
			if st == session.StatusDisconnected || st == session.StatusError {
				go a.removeCapture(s.ID())
			}
		})
	}
	a.captures[s.ID()] = c
	return c
}

// removeCapture drops a session's capture emulator and unsubscribes its taps.
func (a *App) removeCapture(id string) {
	a.captureMu.Lock()
	c, ok := a.captures[id]
	delete(a.captures, id)
	a.captureMu.Unlock()
	if ok {
		if c.dataOff != nil {
			c.dataOff()
		}
		if c.statusOff != nil {
			c.statusOff()
		}
	}
}

// SendKey types text or a control key into a live session's terminal — the
// backend twin of the built-in panel's send_terminal_key, shared by the MCP
// uniterm_send_key tool. control is "ctrl_c" | "ctrl_d" | "enter"; with text,
// sendEnter appends the shell newline.
func (a *App) SendKey(sessionID, text, control string, sendEnter bool) error {
	if a.sessionManager == nil {
		return errors.New("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	var data string
	switch control {
	case "ctrl_c":
		data = "\x03"
	case "ctrl_d":
		data = "\x04"
	case "enter":
		data = newlineFor(s)
	case "":
		if text == "" {
			return errors.New("either text or control is required")
		}
		data = text
		if sendEnter {
			data += newlineFor(s)
		}
	default:
		return fmt.Errorf("unknown control %q (want ctrl_c|ctrl_d|enter)", control)
	}
	return a.writeSession(s.ID(), []byte(data))
}
