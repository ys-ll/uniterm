package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/webaccess"
)

type WebTerminalError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *WebTerminalError) Error() string { return e.Code + ": " + e.Message }
func webError(code, message string) error { return &WebTerminalError{code, message} }

type WebSession struct {
	ID               string                `json:"id"`
	Type             string                `json:"type"`
	Title            string                `json:"title"`
	Status           session.SessionStatus `json:"status"`
	Cols             int                   `json:"cols"`
	Rows             int                   `json:"rows"`
	ActiveClient     string                `json:"activeClient"`
	ConnectionID     string                `json:"connectionId,omitempty"`
	Host             string                `json:"host,omitempty"`
	ShellPath        string                `json:"shellPath,omitempty"`
	RemoteOS         string                `json:"remoteOS,omitempty"`
	ResizeCapability string                `json:"resizeCapability"`
}

type webTerminal struct {
	mu             sync.Mutex // serializes input/grid/close and setup state, never network Connect
	events         sync.Mutex // orders output enqueue against accepted grid changes
	delivery       sync.Mutex // drains session callbacks before session:closed
	info           WebSession
	started        bool
	connecting     bool // Connect owns resource setup; input/close must not wait on it.
	closed         bool
	session        session.Session
	connectedEvent map[string]any
}

type WebTerminalSnapshot struct {
	SessionID string         `json:"sessionId"`
	Sequence  uint64         `json:"sequence"`
	Cols      int            `json:"cols"`
	Rows      int            `json:"rows"`
	ANSI      string         `json:"ansi"`
	State     map[string]any `json:"state"`
}

type WebTerminalPresentationOperation struct {
	Kind        string  `json:"kind"`
	Data        *string `json:"data,omitempty"`
	Timestamp   *int64  `json:"timestamp,omitempty"`
	CursorBlink *bool   `json:"cursorBlink,omitempty"`
	Scrollback  *int    `json:"scrollback,omitempty"`
}

// PublishWebTerminalPresentation puts local UI changes on the same timeline as
// PTY output. Only the desktop writes prompts or configures the terminal; either
// trusted client may clear, reset, or stamp it. Nothing writes back to the PTY.
func (a *App) PublishWebTerminalPresentation(ctx context.Context, sessionID string, operation WebTerminalPresentationOperation) error {
	if webaccess.IsBrowser(ctx) && (operation.Kind == "write" || operation.Kind == "configure") {
		return webError("native_only", "only the desktop can publish terminal writes or configuration")
	}
	if (operation.Kind != "write" && operation.Data != nil) ||
		(operation.Kind != "stamp" && operation.Timestamp != nil) ||
		(operation.Kind != "configure" && (operation.CursorBlink != nil || operation.Scrollback != nil)) {
		return webError("invalid_presentation", "presentation fields do not match kind")
	}
	payload := map[string]any{"kind": operation.Kind}
	switch operation.Kind {
	case "write":
		if operation.Data == nil || !utf8.ValidString(*operation.Data) {
			return webError("invalid_presentation", "write requires UTF-8 string data")
		}
		if len(*operation.Data) > 1<<20 {
			return webError("presentation_too_large", "presentation write exceeds 1 MiB")
		}
		payload["data"] = *operation.Data
	case "clear", "reset":
	case "stamp":
		// JavaScript Date's non-negative Unix-millisecond range, all exact integers.
		if operation.Timestamp == nil || *operation.Timestamp < 0 || *operation.Timestamp > 8640000000000000 {
			return webError("invalid_presentation", "stamp requires integer Unix milliseconds in 0..8640000000000000")
		}
		payload["timestamp"] = *operation.Timestamp
	case "configure":
		// Match the settings UI's history ceiling; zero disables scrollback.
		if operation.CursorBlink == nil || operation.Scrollback == nil || *operation.Scrollback < 0 || *operation.Scrollback > 50000 {
			return webError("invalid_presentation", "configure requires boolean cursorBlink and integer scrollback in 0..50000")
		}
		payload["cursorBlink"] = *operation.CursorBlink
		payload["scrollback"] = *operation.Scrollback
	default:
		return webError("invalid_presentation", "presentation kind must be write, clear, reset, stamp, or configure")
	}
	terminal, err := a.webTerminalFor(sessionID)
	if err != nil {
		return err
	}
	terminal.mu.Lock()
	defer terminal.mu.Unlock()
	if terminal.closed {
		return webError("session_closed", "terminal is closed")
	}
	terminal.events.Lock()
	defer terminal.events.Unlock()
	a.emit("session:presentation", map[string]any{"id": sessionID, "operation": payload})
	return nil
}

type webTerminalRequest struct {
	sessionID string
	minimum   uint64
	attach    bool
	result    chan WebTerminalSnapshot
}

func textTerminal(kind string) bool {
	switch kind {
	case "local", "wsl", "ssh", "telnet", "serial", "tcp", "mosh", "container-exec":
		return true
	}
	return false
}
func validWebSize(cols, rows int) error {
	if cols < 2 || rows < 1 || cols > 1000 || rows > 1000 || cols*rows > 250000 {
		return webError("invalid_size", "terminal grid must be 2..1000 columns, 1..1000 rows, at most 250000 cells")
	}
	return nil
}

func (a *App) registerWebTerminal(s session.Session, config session.ConnectionConfig) error {
	if !textTerminal(s.Type()) {
		return nil
	}
	cols, rows := config.InitialCols, config.InitialRows
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	capability := "pty"
	if s.Type() == "serial" || s.Type() == "tcp" {
		capability = "display"
	}
	title := config.Name
	if title == "" {
		title = s.Type()
	}
	t := &webTerminal{session: s, started: s.IsConnected(), info: WebSession{ID: s.ID(), Type: s.Type(), Title: title, Status: s.Status(), Cols: cols, Rows: rows, ActiveClient: "desktop", ConnectionID: config.ID, Host: config.Host, ShellPath: config.ShellPath, ResizeCapability: capability}}
	info := t.info
	s.SetPendingSize(cols, rows)
	a.webTermMu.Lock()
	if a.webTermClosing {
		a.webTermMu.Unlock()
		return webError("app_closed", "application is shutting down")
	}
	if a.webTerminals == nil {
		a.webTerminals = make(map[string]*webTerminal)
	}
	a.webTerminals[s.ID()] = t
	a.webTermMu.Unlock()
	if gap, ok := s.(interface{ SetOnOutputGap(func()) }); ok {
		gap.SetOnOutputGap(func() { a.emit("session:gap", map[string]any{"id": s.ID(), "reason": "upstream_output_overflow"}) })
	}
	a.emit("web:session-created", info)
	return nil
}

func (a *App) webTerminalFor(id string) (*webTerminal, error) {
	a.webTermMu.Lock()
	t := a.webTerminals[id]
	a.webTermMu.Unlock()
	if t == nil {
		return nil, webError("session_not_found", "text terminal not found: "+id)
	}
	return t, nil
}

func (a *App) GetWebSessions() []WebSession {
	a.webTermMu.Lock()
	terminals := make([]*webTerminal, 0, len(a.webTerminals))
	for _, t := range a.webTerminals {
		terminals = append(terminals, t)
	}
	a.webTermMu.Unlock()
	result := make([]WebSession, 0, len(terminals))
	for _, t := range terminals {
		t.mu.Lock()
		if !t.closed {
			info := t.info
			info.Status = t.session.Status()
			if t.connecting && info.Status == session.StatusConnected {
				info.Status = session.StatusConnecting
			}
			if ssh, ok := t.session.(*session.SSHSession); ok && info.Status == session.StatusConnected {
				info.RemoteOS = ssh.RemoteOS()
			}
			result = append(result, info)
		}
		t.mu.Unlock()
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (a *App) WebTerminalInput(sessionID, clientID, data string, cols, rows int, binary bool) error {
	if len(clientID) == 0 || len(clientID) > 128 {
		return webError("invalid_client", "client ID is required")
	}
	if err := validWebSize(cols, rows); err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return webError("input_too_large", "input exceeds 1 MiB")
	}
	b := []byte(data)
	if binary {
		var err error
		b, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			return webError("invalid_input", "binary input must be base64")
		}
	}
	t, err := a.webTerminalFor(sessionID)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return webError("session_closed", "terminal is closed")
	}
	if !terminalInputReady(t) {
		return webError("session_not_ready", "terminal is not connected")
	}
	if err = a.resizeWebTerminalLocked(t, clientID, cols, rows, true); err != nil {
		return err
	}
	if !binary {
		if err := a.webZmodemInputTarget(sessionID, clientID); err != nil {
			return err
		}
	}
	if err := writeTerminalLocked(t, b); err != nil {
		return webError("write_failed", err.Error())
	}
	return nil
}

func (a *App) ResizeWebTerminal(sessionID, clientID string, cols, rows int, active bool) error {
	if len(clientID) == 0 || len(clientID) > 128 {
		return webError("invalid_client", "client ID is required")
	}
	if err := validWebSize(cols, rows); err != nil {
		return err
	}
	t, err := a.webTerminalFor(sessionID)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return a.resizeWebTerminalLocked(t, clientID, cols, rows, active)
}

func (a *App) resizeWebTerminalLocked(t *webTerminal, clientID string, cols, rows int, active bool) error {
	if t.closed {
		return webError("session_closed", "terminal is closed")
	}
	if !active && t.info.ActiveClient != clientID {
		// Layout/font observers run on every attached view. Their stale size is
		// advisory, not a failed input operation; only the active client applies
		// it. A no-op also covers ownership changing while this RPC is queued.
		return nil
	}
	t.events.Lock()
	defer t.events.Unlock()
	if t.info.Cols != cols || t.info.Rows != rows {
		if t.started && !t.connecting {
			if !t.session.IsConnected() {
				return webError("session_not_ready", "terminal is not connected")
			}
			if err := t.session.Resize(cols, rows); err != nil {
				return webError("resize_failed", err.Error())
			}
		} else {
			t.session.SetPendingSize(cols, rows)
		}
	}
	changed := t.info.Cols != cols || t.info.Rows != rows || t.info.ActiveClient != clientID
	t.info.Cols, t.info.Rows, t.info.ActiveClient = cols, rows, clientID
	if changed {
		a.emit("session:resize", map[string]any{"id": t.info.ID, "cols": cols, "rows": rows, "activeClient": clientID})
	}
	return nil
}

// A connecting SSH session accepts only its authentication channel, never a
// partially initialized PTY. Other unconnected sessions still reject input.
func terminalInputReady(t *webTerminal) bool {
	if !t.started {
		return false
	}
	if !t.connecting && t.session.IsConnected() {
		return true
	}
	ssh, ok := t.session.(*session.SSHSession)
	return ok && ssh.Authenticating()
}

func writeTerminalLocked(t *webTerminal, data []byte) error {
	if t.connecting || !t.session.IsConnected() {
		if ssh, ok := t.session.(*session.SSHSession); ok {
			return ssh.WriteAuthInput(data)
		}
		return webError("session_not_ready", "terminal is not connected")
	}
	return t.session.Write(data)
}

// Automatic replies and MCP/AI writes share the input lock but never take
// active ownership. Only WebTerminalInput/active ResizeWebTerminal do that.
func (a *App) writeSession(id string, data []byte) error {
	if t, err := a.webTerminalFor(id); err == nil {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.closed {
			return webError("session_closed", "terminal is closed")
		}
		if !terminalInputReady(t) {
			return webError("session_not_ready", "terminal is not connected")
		}
		return writeTerminalLocked(t, data)
	}
	if a.sessionManager == nil {
		return webError("not_ready", "session manager not initialized")
	}
	s, ok := a.sessionManager.Get(id)
	if !ok {
		return webError("session_not_found", "session not found: "+id)
	}
	return s.Write(data)
}

func (a *App) GetWebTerminalSnapshot(ctx context.Context, sessionID string) (WebTerminalSnapshot, error) {
	return a.requestWebTerminal(ctx, sessionID, false)
}

func (a *App) requestWebTerminal(ctx context.Context, id string, attach bool) (WebTerminalSnapshot, error) {
	t, err := a.webTerminalFor(id)
	if err != nil {
		return WebTerminalSnapshot{}, err
	}
	t.mu.Lock()
	closed := t.closed
	info := t.info
	t.mu.Unlock()
	if closed {
		return WebTerminalSnapshot{}, webError("session_closed", "terminal is closed")
	}
	requestID := webaccess.GenerateToken()
	req := &webTerminalRequest{sessionID: id, minimum: a.webTransport.terminalSequence(id), attach: attach, result: make(chan WebTerminalSnapshot, 1)}
	a.webTermMu.Lock()
	if a.webTerminals[id] != t || a.webTermClosing {
		a.webTermMu.Unlock()
		return WebTerminalSnapshot{}, webError("session_closed", "terminal closed during desktop request")
	}
	if len(a.webRequests) >= 64 {
		a.webTermMu.Unlock()
		return WebTerminalSnapshot{}, webError("too_many_requests", "too many pending desktop requests")
	}
	if a.webRequests == nil {
		a.webRequests = make(map[string]*webTerminalRequest)
	}
	a.webRequests[requestID] = req
	a.webTermMu.Unlock()
	defer func() { a.webTermMu.Lock(); delete(a.webRequests, requestID); a.webTermMu.Unlock() }()
	name := "web:snapshot-request"
	if attach {
		name = "web:terminal-attach"
	}
	a.emit(name, map[string]any{"requestId": requestID, "sessionId": id, "sequence": req.minimum, "session": info})
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case result := <-req.result:
		if result.SessionID == "" {
			return result, webError("session_closed", "terminal closed during desktop request")
		}
		terminal, err := a.webTerminalFor(id)
		if err != nil {
			return WebTerminalSnapshot{}, err
		}
		terminal.mu.Lock()
		closed := terminal.closed
		terminal.mu.Unlock()
		if closed {
			return WebTerminalSnapshot{}, webError("session_closed", "terminal closed during desktop request")
		}
		if !attach {
			if err := a.webTransport.validateSnapshot(id, result.Sequence, req.minimum); err != nil {
				return WebTerminalSnapshot{}, err
			}
		}
		return result, nil
	case <-ctx.Done():
		return WebTerminalSnapshot{}, ctx.Err()
	case <-timer.C:
		return WebTerminalSnapshot{}, webError("authority_unavailable", "desktop terminal did not respond")
	}
}

func (a *App) ProvideWebTerminalSnapshot(ctx context.Context, requestID string, snapshot WebTerminalSnapshot) error {
	if webaccess.IsBrowser(ctx) {
		return webError("native_only", "only the desktop can author terminal snapshots")
	}
	if err := validWebSize(snapshot.Cols, snapshot.Rows); err != nil {
		return err
	}
	b, err := json.Marshal(snapshot.State)
	if err != nil || len(b)+len(snapshot.ANSI) > 16<<20 {
		return webError("snapshot_too_large", "snapshot exceeds 16 MiB or contains invalid state")
	}
	a.webTermMu.Lock()
	defer a.webTermMu.Unlock()
	req := a.webRequests[requestID]
	if req == nil || req.attach || req.sessionID != snapshot.SessionID {
		return webError("invalid_request", "unknown snapshot request capability")
	}
	if err = a.webTransport.validateSnapshot(snapshot.SessionID, snapshot.Sequence, req.minimum); err != nil {
		return err
	}
	delete(a.webRequests, requestID)
	req.result <- snapshot
	return nil
}

// The capability is delivered only to native windows. The desktop acknowledges
// after installing its authoritative xterm; only then may a browser start PTY IO.
func (a *App) AcknowledgeWebTerminal(ctx context.Context, requestID string) error {
	if webaccess.IsBrowser(ctx) {
		return webError("native_only", "only the desktop can acknowledge terminal attachment")
	}
	a.webTermMu.Lock()
	defer a.webTermMu.Unlock()
	req := a.webRequests[requestID]
	if req == nil || !req.attach {
		return webError("invalid_request", "unknown terminal attachment capability")
	}
	delete(a.webRequests, requestID)
	req.result <- WebTerminalSnapshot{SessionID: req.sessionID}
	return nil
}

func (a *App) forgetWebTerminal(id string) {
	if terminal, err := a.webTerminalFor(id); err == nil {
		terminal.delivery.Lock()
		defer terminal.delivery.Unlock()
	}
	a.webTermMu.Lock()
	delete(a.webTerminals, id)
	for key, req := range a.webRequests {
		if req.sessionID == id {
			delete(a.webRequests, key)
			req.result <- WebTerminalSnapshot{}
		}
	}
	a.webTermMu.Unlock()
	a.emit("session:closed", map[string]any{"id": id})
}
