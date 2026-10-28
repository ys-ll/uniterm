package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ys-ll/uniterm/backend/store"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ys-ll/uniterm/backend/log"
)

// Session is the subset of *mcp.ClientSession the Manager needs. Extracted as
// an interface so tests can inject an in-memory session without a subprocess.
type Session interface {
	ListTools(ctx context.Context, params *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	Close() error
}

// dialer connects a config to a live MCP session. The default uses the SDK;
// tests replace it with an in-memory dialer.
type dialer func(ctx context.Context, cfg store.MCPClientConfig) (Session, error)

// DiscoveredTool is one external tool flattened for the built-in AI.
type DiscoveredTool struct {
	// Server is the owning external client config name.
	Server string `json:"server"`
	// Name is the tool's original name on the remote server.
	Name string `json:"name"`
	// QualifiedName namespaces it for the model: mcp__<server>__<tool>.
	QualifiedName string `json:"qualifiedName"`
	Description   string `json:"description"`
	InputSchema   any    `json:"inputSchema,omitempty"`
}

// Conn is one live external server connection.
type Conn struct {
	cfg     store.MCPClientConfig
	session Session
	tools   []DiscoveredTool
	// err records the last connect/list failure for status display; a conn
	// with a non-nil err is skipped by ListTools/CallTool.
	err error

	// reconnectMu serializes reconnect attempts for this conn so concurrent
	// tool calls after a dropped connection trigger exactly one redial.
	reconnectMu sync.Mutex

	closeOnce sync.Once
	closeCh   chan struct{}
}

// Manager owns the set of live external MCP connections.
type Manager struct {
	store *store.MCPClientStore
	dial  dialer
	mu    sync.Mutex
	conns map[string]*Conn
}

// NewManager builds a manager over the store with the default SDK dialer.
func NewManager(configStore *store.MCPClientStore) *Manager {
	m := &Manager{store: configStore, conns: map[string]*Conn{}}
	m.dial = m.defaultDial
	return m
}

// defaultDial connects via the go-sdk transport matching cfg.Transport.
func (m *Manager) defaultDial(ctx context.Context, cfg store.MCPClientConfig) (Session, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "uniterm-mcp-client", Version: "1.0.0"}, nil)

	var transport mcp.Transport
	switch cfg.Transport {
	case store.MCPTransportStdio:
		cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
		if len(cfg.Env) > 0 {
			cmd.Env = append(cmd.Environ(), envPairs(cfg.Env)...)
		}
		transport = &mcp.CommandTransport{Command: cmd}
	case store.MCPTransportStreamableHTTP:
		transport = &mcp.StreamableClientTransport{
			Endpoint:             cfg.URL,
			HTTPClient:           &http.Client{Transport: &headerRoundTripper{headers: cfg.Headers}, Timeout: 30 * time.Second},
			DisableStandaloneSSE: true, // request-response only; no server push needed by the AI panel
		}
	case store.MCPTransportSSE:
		transport = &mcp.SSEClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: &http.Client{Transport: &headerRoundTripper{headers: cfg.Headers}, Timeout: 30 * time.Second},
		}
	default:
		return nil, fmt.Errorf("unknown transport %q", cfg.Transport)
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// headerRoundTripper injects static headers (Authorization, ...) into every
// request. The SDK's HTTP transports expose no static Headers field, so the
// custom client carries them.
type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		if strings.EqualFold(k, "host") {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	base := h.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func envPairs(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// QualifiedName namespaces a remote tool for the model.
func QualifiedName(server, tool string) string {
	return "mcp__" + sanitize(server) + "__" + sanitize(tool)
}

// sanitize lowers and maps anything outside [a-z0-9_] to underscore so the
// qualified name is a safe tool identifier across providers.
func sanitize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// splitQualified reverses QualifiedName: "mcp__server__tool" → (server, tool).
// The tool portion keeps its original case, so it is matched case-sensitively
// against the remote list by the caller.
func splitQualified(q string) (server, tool string, ok bool) {
	rest, found := strings.CutPrefix(q, "mcp__")
	if !found {
		return "", "", false
	}
	server, tool, found = strings.Cut(rest, "__")
	if !found || server == "" || tool == "" {
		return "", "", false
	}
	return server, tool, true
}

// Reload reconciles live connections with the persisted config: connects newly
// enabled servers, closes disabled/removed ones, leaves unchanged ones alone.
// Connect failures are recorded on the conn (surfaced in status) rather than
// aborting the whole reload — one bad server must not block the others.
func (m *Manager) Reload(ctx context.Context) {
	configs, err := m.store.Load()
	if err != nil {
		log.Writef("mcpclient: load config: %v", err)
		return
	}
	wanted := map[string]bool{}
	for _, cfg := range configs {
		if err := cfg.Validate(); err != nil {
			log.Writef("mcpclient: skip %q: %v", cfg.Name, err)
			continue
		}
		wanted[cfg.Name] = true
		if !cfg.Enabled {
			m.closeConn(cfg.Name)
			continue
		}
		m.ensureConn(ctx, cfg)
	}

	m.mu.Lock()
	for name, c := range m.conns {
		if !wanted[name] {
			m.closeLocked(c)
			delete(m.conns, name)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) ensureConn(ctx context.Context, cfg store.MCPClientConfig) {
	m.mu.Lock()
	if existing, ok := m.conns[cfg.Name]; ok && existing.session != nil && existing.err == nil {
		m.mu.Unlock()
		return
	}
	// Drop a stale failed/closed slot before retrying.
	if old, ok := m.conns[cfg.Name]; ok {
		m.closeLocked(old)
		delete(m.conns, cfg.Name)
	}
	c := &Conn{cfg: cfg, closeCh: make(chan struct{})}
	m.conns[cfg.Name] = c
	m.mu.Unlock()

	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	session, err := m.dial(connectCtx, cfg)
	if err != nil {
		m.mu.Lock()
		c.err = err
		m.mu.Unlock()
		log.Writef("mcpclient: connect %q: %v", cfg.Name, err)
		return
	}
	tools, err := listTools(connectCtx, session, cfg.Name)
	if err != nil {
		_ = session.Close()
		m.mu.Lock()
		c.err = err
		m.mu.Unlock()
		log.Writef("mcpclient: list tools %q: %v", cfg.Name, err)
		return
	}
	m.mu.Lock()
	c.session = session
	c.tools = tools
	c.err = nil
	m.mu.Unlock()
	log.Writef("mcpclient: %q connected, %d tools", cfg.Name, len(tools))
}

func (m *Manager) closeConn(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conns[name]; ok {
		m.closeLocked(c)
		delete(m.conns, name)
	}
}

func (m *Manager) closeLocked(c *Conn) {
	c.closeOnce.Do(func() { close(c.closeCh) })
	if c.session != nil {
		_ = c.session.Close()
	}
}

func listTools(ctx context.Context, s Session, server string) ([]DiscoveredTool, error) {
	res, err := s.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	out := make([]DiscoveredTool, 0, len(res.Tools))
	for _, t := range res.Tools {
		if t == nil || t.Name == "" {
			continue
		}
		out = append(out, DiscoveredTool{
			Server:        server,
			Name:          t.Name,
			QualifiedName: QualifiedName(server, t.Name),
			Description:   t.Description,
			InputSchema:   t.InputSchema,
		})
	}
	return out, nil
}

// ListTools returns the flattened tool set across healthy connections.
func (m *Manager) ListTools() []DiscoveredTool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []DiscoveredTool{}
	for _, c := range m.conns {
		if c.session == nil || c.err != nil {
			continue
		}
		out = append(out, c.tools...)
	}
	return out
}

// ConnStatus reports per-server health for the settings UI.
type ConnStatus struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	Tools     int    `json:"tools"`
	Error     string `json:"error,omitempty"`
}

// Statuses returns a health row for every persisted config (including
// disconnected/failed ones) so the UI can show why a server is down.
func (m *Manager) Statuses(configs []store.MCPClientConfig) []ConnStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ConnStatus, 0, len(configs))
	for _, cfg := range configs {
		row := ConnStatus{Name: cfg.Name, Enabled: cfg.Enabled}
		if c, ok := m.conns[cfg.Name]; ok {
			row.Connected = c.session != nil && c.err == nil
			row.Tools = len(c.tools)
			if c.err != nil {
				row.Error = c.err.Error()
			}
		}
		out = append(out, row)
	}
	return out
}

// CallTool invokes a remote tool by its qualified name and returns the
// concatenated text content. When the call fails with a connection-level
// error (dropped SSE stream, wedged stdio pipe, half-closed HTTP session),
// the conn is redialed once and the call retried transparently (issue
// #1174: previously every call after a dropped SSE connection returned a
// bare EOF until an app restart).
func (m *Manager) CallTool(ctx context.Context, qualified string, args map[string]any) (string, error) {
	server, tool, ok := splitQualified(qualified)
	if !ok {
		return "", fmt.Errorf("not an mcp tool: %q", qualified)
	}
	m.mu.Lock()
	c, ok := m.conns[server]
	m.mu.Unlock()
	if !ok || c == nil {
		return "", fmt.Errorf("mcp server %q is not connected (add or enable it in settings)", server)
	}
	if c.session == nil && c.err != nil {
		return "", fmt.Errorf("mcp server %q (%s, %s): %w", server, c.cfg.URL, c.cfg.Transport, c.err)
	}
	if c.session == nil {
		return "", fmt.Errorf("mcp server %q is not connected", server)
	}

	res, err := m.callWithRetry(ctx, c, tool, args)
	if err != nil {
		return "", err
	}
	text := extractText(res)
	if res.IsError {
		return "", fmt.Errorf("mcp tool %s returned error: %s", qualified, text)
	}
	if text == "" {
		text = "[mcp tool returned no text content]"
	}
	return text, nil
}

// callWithRetry performs the tool call and, on a connection-level failure,
// redials the conn once and replays the call on the fresh session.
func (m *Manager) callWithRetry(ctx context.Context, c *Conn, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	m.mu.Lock()
	sess := c.session
	m.mu.Unlock()
	if sess == nil {
		return nil, fmt.Errorf("mcp server %q is not connected", c.cfg.Name)
	}
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	res, err := sess.CallTool(callCtx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err == nil {
		return res, nil
	}
	if !isConnError(err) {
		return nil, fmt.Errorf("call mcp__%s__%s: %w", sanitize(c.cfg.Name), tool, err)
	}

	// The transport died mid-session (SSE stream dropped, stdio pipe
	// closed, ...). Redial and retry once; the reconnect lock collapses a
	// stampede of concurrent failed calls into a single redial.
	log.Writef("mcpclient: connection lost on %q (%s), reconnecting: %v", c.cfg.Name, c.cfg.Transport, err)
	session, rerr := m.reconnect(c, sess)
	if rerr != nil {
		return nil, fmt.Errorf("mcp server %q (%s, %s) connection lost and reconnect failed: %v (original call error: %w; check the server or re-enable it in settings)",
			c.cfg.Name, c.cfg.URL, c.cfg.Transport, rerr, err)
	}
	retryCtx, retryCancel := context.WithTimeout(ctx, 60*time.Second)
	defer retryCancel()
	res, rerr2 := session.CallTool(retryCtx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if rerr2 != nil {
		// The fresh session failed too — mark the conn so ListTools stops
		// reporting the stale tool cache and Statuses shows the server down.
		m.mu.Lock()
		c.err = rerr2
		m.mu.Unlock()
		return nil, fmt.Errorf("call mcp__%s__%s after reconnect: %w (server %q, %s, %s; check the server or re-enable it in settings)",
			sanitize(c.cfg.Name), tool, rerr2, c.cfg.Name, c.cfg.URL, c.cfg.Transport)
	}
	return res, nil
}

// reconnect redials a dead conn in place: new session, fresh tool list. The
// per-conn lock guarantees one dialer at a time; concurrent callers ride
// whichever redial wins the lock (generation counter tells riders whether
// the session on the conn is newer than the one they saw fail).
func (m *Manager) reconnect(c *Conn, failed Session) (Session, error) {
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	// Another caller already reconnected while this goroutine waited on the
	// lock: the conn now carries a session that is not the one this caller
	// saw fail, so ride it instead of dialing again. Session identity (not a
	// generation counter) is the test: a caller that failed on ANY session
	// still served by the conn is served by the same redial it waits behind.
	m.mu.Lock()
	session := c.session
	m.mu.Unlock()
	if session != nil && session != failed {
		return session, nil
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	newSession, err := m.dial(connectCtx, c.cfg)
	if err != nil {
		m.mu.Lock()
		c.err = err
		if c.session != nil {
			_ = c.session.Close()
		}
		c.session = nil
		m.mu.Unlock()
		return nil, err
	}
	newTools, err := listTools(connectCtx, newSession, c.cfg.Name)
	if err != nil {
		_ = newSession.Close()
		m.mu.Lock()
		c.err = err
		if c.session != nil {
			_ = c.session.Close()
		}
		c.session = nil
		m.mu.Unlock()
		return nil, err
	}

	m.mu.Lock()
	old := c.session
	c.session = newSession
	c.tools = newTools
	c.err = nil
	m.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	log.Writef("mcpclient: %q reconnected, %d tools", c.cfg.Name, len(newTools))
	return newSession, nil
}

// isConnError reports whether err is a transport-level failure worth a
// reconnect (as opposed to a tool-level error the retry cannot fix).
// The SDK wraps dropped transports as ErrConnectionClosed ("connection
// closed: calling ..."); the "client is closing" phrasing appears on
// sessions whose transport goroutine has exited. io.EOF surfaces from raw
// pipes (stdio).
func isConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, mcp.ErrConnectionClosed) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "client is closing") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "use of closed")
}

// TestConnection dials a one-shot connection, lists its tools, and closes. Used
// by the settings "test" button without persisting state first.
func (m *Manager) TestConnection(ctx context.Context, cfg store.MCPClientConfig) ([]DiscoveredTool, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	session, err := m.dial(dialCtx, cfg)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return listTools(dialCtx, session, cfg.Name)
}

// Reconnect closes and re-dials a single server's live connection by name.
// When the server is disabled the existing connection is simply dropped.
// A missing/unknown name is a no-op so the caller can fire-and-forget from the
// settings UI without first checking the config list.
func (m *Manager) Reconnect(ctx context.Context, name string) {
	configs, err := m.store.Load()
	if err != nil {
		log.Writef("mcpclient: reconnect %q: load config: %v", name, err)
		return
	}
	for _, cfg := range configs {
		if cfg.Name != name {
			continue
		}
		m.closeConn(name)
		if cfg.Enabled {
			m.ensureConn(ctx, cfg)
		}
		return
	}
}

// CloseAll tears down every connection on app shutdown.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.conns {
		m.closeLocked(c)
	}
	m.conns = map[string]*Conn{}
}

// extractText concatenates text content blocks; non-text blocks are rendered as
// their JSON so image/resource results are still legible to the model.
func extractText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, content := range res.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
			b.WriteByte('\n')
			continue
		}
		raw, err := json.Marshal(content)
		if err != nil {
			continue
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
