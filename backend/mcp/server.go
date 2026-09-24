// Package mcp exposes uniTerm as an MCP (Model Context Protocol) server so
// external AI agents (Claude Code, Codex CLI, Kimi CLI, ...) can run commands
// on the user's saved / live SSH connections without ever seeing a password
// or private key: the app holds the credentials, executes through its own
// authenticated clients, and gates every command behind an approval dialog
// surfaced in the UI.
//
// Transport: Streamable HTTP on 127.0.0.1 only, authenticated with a
// per-client bearer token (stored as SHA-256, shown once at creation).
package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ys-ll/uniterm/backend/log"
)

// DefaultPort is the MCP endpoint's listen port. Uncommon on purpose; the
// user can override it in settings.
const DefaultPort = 61207

// DefaultApprovalTimeout bounds how long a tool call waits for the user to
// answer the approval dialog. Kept under Codex's 120s tools/call limit so a
// stale approval can never fire after the client already gave up.
const DefaultApprovalTimeout = 110 * time.Second

// DefaultSyncTimeout is how long exec_command waits for completion before
// returning an async handle (commandId) the agent can poll.
const DefaultSyncTimeout = 20 * time.Second

// DefaultCommandTimeout is the hard runtime cap for one exec command.
const DefaultCommandTimeout = 5 * time.Minute

// DefaultMaxOutputBytes caps the output carried back in one tool result;
// larger output is truncated with a marker.
const DefaultMaxOutputBytes = 64 * 1024

// SessionFunc resolves a live session id to its SSH executor. Injected by the
// App layer (backend/session imports are kept out of this package's API so
// the mcp package stays testable in isolation).
type SessionFunc func(sessionID string) (SSHExecutor, bool)

// FileEntry is one remote listing row for list_remote_dir (kept free of
// session types so the mcp package stays isolated).
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime int64  `json:"modTime,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

// CommandFunc resolves a commandId to its owning session's executor and the
// session id. Injected by the App layer.
type CommandFunc func(commandID string) (sessionID string, exec SSHExecutor, ok bool)

// Env is everything the MCP server needs from the app. All fields are
// read-only after Start.
type Env struct {
	Sessions SessionFunc
	// Commands resolves a commandId to its owning session + executor.
	Commands CommandFunc
	// ListSessions returns live session metadata (id, type, title, status).
	ListSessions func() []SessionSummary
	// ListConnections returns saved SSH connection profiles with safe
	// metadata only — never credentials.
	ListConnections func() []ConnectionSummary
	// Connect opens a new SSH session for a saved connection id and returns
	// the session id. The approval gate runs before dialing.
	Connect func(connectionID string) (sessionID string, err error)
	// Approve surfaces a confirmation request to the user and waits for the
	// verdict. err is non-nil on timeout/no-window.
	Approve func(req ApprovalRequest) error
	// Audit appends one JSONL entry.
	Audit func(entry AuditEntry)
	// ToolsEnabled mirrors the per-group toggles from settings.
	ToolsEnabled func() ToolGroups
	// Policy returns the active approval policy.
	Policy func() Policy
	// ResolveToken resolves a token hash to its client name, re-reading
	// persisted tokens when the file changed (so tokens written elsewhere
	// take effect without a restart). "" = unknown token. When nil, the
	// in-memory map from SetTokens is authoritative.
	ResolveToken func(hash string) (name string)
	// FileSession resolves a live session id to its SFTP-backed file
	// executor; (nil,false) when the session is gone or not SSH.
	FileSession func(sessionID string) (FileExecutor, bool)
	// ResolveLocalPath validates a local path against the user-configured
	// allowed directories (SFTP bookmarks) and returns the resolved path.
	ResolveLocalPath func(path string) (resolved string, err error)
}

// SessionSummary is one live session row for list_sessions.
type SessionSummary struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status string `json:"status"`
	// Cwd is the last OSC-7 reported working directory, if known.
	Cwd string `json:"cwd,omitempty"`
}

// ConnectionSummary is one saved connection for list_connections. Deliberately
// free of credential fields.
type ConnectionSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
}

// SSHExecutor runs one command on an authenticated SSH connection and
// returns stdout, stderr, the exit code and an async handle when the sync
// wait budget is exceeded. Implemented by session.SSHSession.
type SSHExecutor interface {
	// MCPExec runs cmd. If the command is still running after syncTimeout it
	// keeps running and commandID is set; poll with MCPLatestOutput.
	MCPExec(cmd string, syncTimeout, hardTimeout, maxOutput int) (stdout, stderr []byte, exitCode int, commandID string, err error)
	// MCPLatestOutput returns the captured output so far for a commandID.
	MCPLatestOutput(commandID string, offset int, max int) (stdout, stderr []byte, exitCode int, state string, err error)
	// MCPInterrupt kills a command started via MCPExec.
	MCPInterrupt(commandID string, signal string) error
}

// FileExecutor carries the SFTP-backed file tools. Split from SSHExecutor so
// the exec group can be implemented/tested without SFTP.
type FileExecutor interface {
	MCPListDir(remotePath string) (entries []FileEntry, err error)
	MCPReadFile(remotePath string, offset int64, max int) (data []byte, truncated bool, err error)
	MCPWriteFile(localPath, remotePath string) (bytes int64, err error)
	MCPReadRemoteToFile(remotePath, localPath string) (bytes int64, err error)
}

// ToolGroups are the per-group tool toggles.
type ToolGroups struct {
	Discovery bool
	Exec      bool
	Terminal  bool
	Files     bool
}

// Policy is the approval policy for exec calls.
//   - "confirm_all": every command needs approval (default)
//   - "confirm_write": only risk≥write commands
//   - "confirm_dangerous": only dangerous commands
//   - "bypass": no dialogs (dangerous commands still require approval)
type Policy string

const (
	PolicyConfirmAll       Policy = "confirm_all"
	PolicyConfirmWrite     Policy = "confirm_write"
	PolicyConfirmDangerous Policy = "confirm_dangerous"
	PolicyBypass           Policy = "bypass"
)

// RiskClass is the coarse command classification used by the policy engine.
type RiskClass int

const (
	RiskRead RiskClass = iota
	RiskWrite
	RiskDangerous
)

// ApprovalRequest is one confirmation dialog.
type ApprovalRequest struct {
	ID         string `json:"id"`
	Client     string `json:"client"` // token name of the calling agent
	Connection string `json:"connection"`
	Command    string `json:"command"`
	Cwd        string `json:"cwd,omitempty"`
	Timeout    int    `json:"timeout"`
	CreatedAt  int64  `json:"createdAt"`
}

// AuditEntry is one JSONL line in mcp-audit.log.
type AuditEntry struct {
	Time     string `json:"time"`
	Token    string `json:"token"`
	Tool     string `json:"tool"`
	Session  string `json:"session,omitempty"`
	Command  string `json:"command,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
	Approved *bool  `json:"approved,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Server is the uniTerm MCP server: HTTP listener + SDK server + token auth.
type Server struct {
	env Env

	mu      sync.Mutex
	tokens  map[string]TokenInfo // keyed by sha256(token)
	handler *mcp.StreamableHTTPHandler
	ln      net.Listener
	httpSrv *http.Server
	port    int
	started bool
}

// TokenInfo is the stored form of one client token.
type TokenInfo struct {
	Name  string `json:"name"`
	Scope string `json:"scope,omitempty"`
}

// NewServer builds a server (not yet listening).
func NewServer(env Env) *Server {
	return &Server{env: env, tokens: make(map[string]TokenInfo)}
}

// Port reports the listening port (0 when not running).
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// Running reports whether the HTTP listener is up.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

// SetTokens replaces the token set (called when settings change).
func (s *Server) SetTokens(tokens map[string]TokenInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = tokens
}

// Start brings up the listener on 127.0.0.1:port. Idempotent: a running
// server is stopped first when the port changed, or the call is a no-op.
func (s *Server) Start(port int) error {
	s.Stop()

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "uniterm",
		Version: "1.0.0",
	}, nil)
	s.registerTools(srv)

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return srv
	}, nil)

	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return fmt.Errorf("mcp: listen: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.authMiddleware(handler))
	httpSrv := &http.Server{Handler: mux}

	s.mu.Lock()
	s.handler = handler
	s.ln = ln
	s.httpSrv = httpSrv
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.started = true
	s.mu.Unlock()

	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Writef("mcp: serve: %v", err)
		}
	}()
	log.Writef("mcp: server listening on 127.0.0.1:%d", s.port)
	return nil
}

// Stop tears the listener down.
func (s *Server) Stop() {
	s.mu.Lock()
	httpSrv := s.httpSrv
	s.httpSrv = nil
	s.ln = nil
	s.handler = nil
	s.port = 0
	s.started = false
	s.mu.Unlock()
	if httpSrv != nil {
		_ = httpSrv.Close()
	}
}

// tokenNameContextKey carries the resolved token name from the auth
// middleware into tool handlers (the SDK passes req.Context() through).
type tokenNameContextKey struct{}

// authMiddleware rejects requests without a valid bearer token. Loopback
// binding alone is not trust: browser pages and local processes can reach
// 127.0.0.1 unauthenticated. Token resolution prefers Env.ResolveToken
// (file-backed, hot-reloaded); the in-memory map is the fallback. The
// resolved token name is stashed in the request context so tool handlers can
// label approvals/audit entries.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		key := HashToken(token)
		name := ""
		if s.env.ResolveToken != nil {
			name = s.env.ResolveToken(key)
		}
		if name == "" {
			s.mu.Lock()
			info, ok := s.tokens[key]
			s.mu.Unlock()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="uniterm"`)
				http.Error(w, "invalid or missing token", http.StatusUnauthorized)
				return
			}
			name = info.Name
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tokenNameContextKey{}, name)))
	})
}

// bearerToken extracts the Authorization: Bearer value.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if len(auth) > len(prefix) && (auth[:len(prefix)] == prefix || auth[:len(prefix)] == "bearer ") {
		return auth[len(prefix):]
	}
	return ""
}

// GenerateToken mints a new random 32-byte hex token and returns it together
// with its stored hash key.
func GenerateToken() (token, hashKey string) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	token = hex.EncodeToString(buf)
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:])
}

// HashToken derives the storage key for a plaintext token.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// TokenEntry is one persisted token record (hash only — the plaintext is
// shown once and then forgotten).
type TokenEntry struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
}
