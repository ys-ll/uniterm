package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools installs the four tool groups on the SDK server. All groups
// are always available; side effects are governed by the approval policy
// (gateExec) and every call is audited.
func (s *Server) registerTools(srv *mcp.Server) {
	connectionsDesc := "List saved SSH connection profiles with safe metadata only. Use connectionId with connect."
	sessionsDesc := "List SSH sessions. Pass sessionId to exec_command."
	if s.execMode() == ModeForeground {
		connectionsDesc = "List saved text-terminal profiles (SSH, local, WSL, telnet, mosh, serial, TCP). No credentials are returned. Use connection_id with connect. Unsaved local/WSL terminals can also be opened with connect."
		sessionsDesc = "List all text-terminal sessions, including local, WSL, SSH, telnet, mosh, serial and TCP. Use id as session_id with the foreground tools."
	}
	// ── Discovery group ─────────────────────────────────────────────
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_connections",
		Description: connectionsDesc,
	}, s.toolListConnections)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_sessions",
		Description: sessionsDesc,
	}, s.toolListSessions)

	// ── Exec group ──────────────────────────────────────────────────
	if s.execMode() == ModeForeground {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "connect",
			Description: "Open a visible terminal from connection_id in list_connections (SSH, local, WSL, telnet, mosh, serial or TCP). Without a saved profile, set type=local or wsl and optionally shell_path. Returns session_id; wait for connected status in list_sessions before executing.",
		}, s.toolConnectTerminal)
	} else {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "connect",
			Description: "Open a background SSH session from a saved connectionId in list_connections. The user can view the terminal in the MCP sessions menu. Wait for connected status in list_sessions before executing.",
		}, s.toolConnect)
	}

	if s.execMode() != ModeForeground {
		mcp.AddTool(srv, &mcp.Tool{
			Name: "exec_command",
			Description: "Run a shell command on a remote host over an existing connected uniTerm SSH session (non-interactive; separate stdout/stderr; real exit code). " +
				"Prefer this over writing into the user's terminal. Long-running commands (longer than ~20s) return a commandId you can poll with get_command_output. " +
				"Each command may require user approval in the uniTerm window.",
		}, s.toolExecCommand)

		mcp.AddTool(srv, &mcp.Tool{
			Name:        "get_command_output",
			Description: "Fetch output (and status) of a command started by exec_command, by commandId. Supports paging via offset. Use for long-running commands.",
		}, s.toolGetCommandOutput)

		mcp.AddTool(srv, &mcp.Tool{
			Name:        "interrupt_command",
			Description: "Send a signal (SIGINT/SIGTERM/SIGKILL) to a still-running command started via exec_command.",
		}, s.toolInterruptCommand)
	}

	// ── Files group (SFTP over the session's connection) ─────────────
	// ── Unified async-job group (uniterm_exec / result / interrupt) ──
	s.registerJobTools(srv)

	s.registerFileTools(srv)
}

// ── Inputs / outputs ────────────────────────────────────────────

type listConnectionsIn struct{}
type listConnectionsOut struct {
	Connections []ConnectionSummary `json:"connections"`
}

type listSessionsIn struct{}
type listSessionsOut struct {
	Sessions []SessionSummary `json:"sessions"`
}

type connectIn struct {
	ConnectionID string `json:"connectionId" jsonschema:"the connection id from list_connections"`
}
type connectOut struct {
	SessionID string `json:"sessionId"`
}

// IsTerminalType excludes graphical, database and file-transfer sessions.
func IsTerminalType(kind string) bool {
	switch kind {
	case "ssh", "local", "wsl", "telnet", "mosh", "serial", "tcp":
		return true
	}
	return false
}

type connectTerminalIn struct {
	ConnectionID string `json:"connection_id,omitempty" jsonschema:"saved terminal connection id from list_connections"`
	Type         string `json:"type,omitempty" jsonschema:"local or wsl when connection_id is omitted; saved profiles select their own type"`
	ShellPath    string `json:"shell_path,omitempty" jsonschema:"local shell executable or wsl://distribution; only for terminals without a saved profile"`
}

type connectTerminalOut struct {
	SessionID string `json:"session_id"`
}

func (s *Server) toolConnectTerminal(ctx context.Context, req *mcp.CallToolRequest, in connectTerminalIn) (*mcp.CallToolResult, connectTerminalOut, error) {
	if in.ConnectionID != "" && (in.Type != "" || in.ShellPath != "") {
		return nil, connectTerminalOut{}, fmt.Errorf("connection_id cannot be combined with type or shell_path")
	}
	if in.ConnectionID == "" && in.Type != "local" && in.Type != "wsl" {
		return nil, connectTerminalOut{}, fmt.Errorf("provide connection_id, or type=local|wsl")
	}
	if in.Type == "wsl" && (!strings.HasPrefix(strings.ToLower(in.ShellPath), "wsl://") || len(in.ShellPath) <= len("wsl://")) {
		return nil, connectTerminalOut{}, fmt.Errorf("WSL requires shell_path=wsl://distribution")
	}
	if s.env.OpenTerminal == nil {
		return nil, connectTerminalOut{}, fmt.Errorf("terminal creation unavailable")
	}
	target := in.ConnectionID
	if target == "" {
		target = in.Type + ":" + in.ShellPath
	}
	if err := s.gateConnect(ctx, req, target); err != nil {
		return nil, connectTerminalOut{}, err
	}
	id, err := s.env.OpenTerminal(in.ConnectionID, in.Type, in.ShellPath)
	s.audit(ctx, req, "connect", target, id, nil, err)
	if err != nil {
		return nil, connectTerminalOut{}, err
	}
	return nil, connectTerminalOut{SessionID: id}, nil
}

type execIn struct {
	SessionID   string `json:"sessionId" jsonschema:"session id from list_sessions / connect"`
	Command     string `json:"command" jsonschema:"shell command to execute"`
	TimeoutMs   int    `json:"timeoutMs,omitempty" jsonschema:"sync wait budget in ms before going async (default 20000)"`
	HardLimitMs int    `json:"hardLimitMs,omitempty" jsonschema:"hard runtime cap in ms (default 300000)"`
}
type execOut struct {
	CommandID string `json:"commandId,omitempty"`
	Async     bool   `json:"async,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	ExitCode  int    `json:"exitCode"`
	Truncated bool   `json:"truncated,omitempty"`
}

type getOutputIn struct {
	CommandID string `json:"commandId" jsonschema:"command id returned by exec_command"`
	Offset    int    `json:"offset,omitempty" jsonschema:"byte offset to resume from (default 0)"`
}
type getOutputOut struct {
	CommandID  string `json:"commandId"`
	State      string `json:"state"` // running | exited | error | session_closed
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	ExitCode   int    `json:"exitCode,omitempty"`
	NextOffset int    `json:"nextOffset"`
	Truncated  bool   `json:"truncated,omitempty"`
}

type interruptIn struct {
	CommandID string `json:"commandId" jsonschema:"command id from exec_command"`
	Signal    string `json:"signal,omitempty" jsonschema:"SIGINT (default), SIGTERM or SIGKILL"`
}
type interruptOut struct{}

// ── Handlers ─────────────────────────────────────────────────────

func (s *Server) toolListConnections(ctx context.Context, req *mcp.CallToolRequest, _ listConnectionsIn) (*mcp.CallToolResult, listConnectionsOut, error) {
	conns := s.env.ListConnections()
	if conns == nil {
		conns = []ConnectionSummary{}
	}
	s.audit(ctx, req, "list_connections", "", "", nil, nil)
	return nil, listConnectionsOut{Connections: conns}, nil
}

func (s *Server) toolListSessions(ctx context.Context, req *mcp.CallToolRequest, _ listSessionsIn) (*mcp.CallToolResult, listSessionsOut, error) {
	sessions := s.env.ListSessions()
	if sessions == nil {
		sessions = []SessionSummary{}
	}
	s.audit(ctx, req, "list_sessions", "", "", nil, nil)
	return nil, listSessionsOut{Sessions: sessions}, nil
}

func (s *Server) toolConnect(ctx context.Context, req *mcp.CallToolRequest, in connectIn) (*mcp.CallToolResult, connectOut, error) {
	if strings.TrimSpace(in.ConnectionID) == "" {
		return nil, connectOut{}, fmt.Errorf("connectionId is required")
	}
	if err := s.gateConnect(ctx, req, in.ConnectionID); err != nil {
		return nil, connectOut{}, err
	}
	sid, err := s.env.Connect(in.ConnectionID)
	if err != nil {
		s.audit(ctx, req, "connect", in.ConnectionID, "", nil, err)
		return nil, connectOut{}, err
	}
	s.audit(ctx, req, "connect", in.ConnectionID, sid, nil, nil)
	return nil, connectOut{SessionID: sid}, nil
}

func (s *Server) toolExecCommand(ctx context.Context, req *mcp.CallToolRequest, in execIn) (*mcp.CallToolResult, execOut, error) {
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, execOut{}, fmt.Errorf("sessionId is required")
	}
	if strings.TrimSpace(in.Command) == "" {
		return nil, execOut{}, fmt.Errorf("command is required")
	}

	exec, ok := s.env.Sessions(in.SessionID)
	if !ok {
		return nil, execOut{}, fmt.Errorf("session %s not found (use list_sessions)", in.SessionID)
	}

	syncTimeout := in.TimeoutMs
	if syncTimeout <= 0 {
		syncTimeout = int(DefaultSyncTimeout / time.Millisecond)
	}
	hardTimeout := in.HardLimitMs
	if hardTimeout <= 0 {
		hardTimeout = int(DefaultCommandTimeout / time.Millisecond)
	}
	if hardTimeout < syncTimeout {
		hardTimeout = syncTimeout
	}
	maxOutput := DefaultMaxOutputBytes

	// Approval gate: every exec path (sync or async) goes through the policy
	// engine before any byte leaves the machine.
	risk := classifyCommand(in.Command)
	if classifyDownloadPipe(in.Command) {
		risk = RiskDangerous
	}
	if err := s.gateExec(ctx, req, in.SessionID, in.Command, risk); err != nil {
		return nil, execOut{}, err
	}

	stdout, stderr, exitCode, cmdID, err := exec.MCPExec(in.Command, syncTimeout, hardTimeout, maxOutput)
	out := execOut{ExitCode: exitCode}
	if cmdID != "" {
		out.CommandID = cmdID
		out.Async = true
	} else {
		out.Stdout = string(stdout)
		out.Stderr = string(stderr)
		out.ExitCode = exitCode
	}
	s.audit(ctx, req, "exec_command", in.SessionID, in.Command, &exitCode, err)
	return nil, out, err
}

func (s *Server) toolGetCommandOutput(ctx context.Context, req *mcp.CallToolRequest, in getOutputIn) (*mcp.CallToolResult, getOutputOut, error) {
	if strings.TrimSpace(in.CommandID) == "" {
		return nil, getOutputOut{}, fmt.Errorf("commandId is required")
	}
	// The command registry is resolved via Env.Commands (App layer owns the
	// session→command mapping).
	sid, exec, ok := s.env.Commands(in.CommandID)
	if !ok || exec == nil {
		return nil, getOutputOut{}, fmt.Errorf("command %s not found or expired", in.CommandID)
	}
	stdout, stderr, exitCode, state, err := exec.MCPLatestOutput(in.CommandID, in.Offset, DefaultMaxOutputBytes)
	out := getOutputOut{
		CommandID:  in.CommandID,
		State:      state,
		Stdout:     string(stdout),
		Stderr:     string(stderr),
		ExitCode:   exitCode,
		NextOffset: in.Offset + len(stdout),
	}
	s.audit(ctx, req, "get_command_output", sid, "", &exitCode, err)
	return nil, out, nil
}

func (s *Server) toolInterruptCommand(ctx context.Context, req *mcp.CallToolRequest, in interruptIn) (*mcp.CallToolResult, interruptOut, error) {
	if strings.TrimSpace(in.CommandID) == "" {
		return nil, interruptOut{}, fmt.Errorf("commandId is required")
	}

	_, exec, ok := s.env.Commands(in.CommandID)
	if !ok || exec == nil {
		return nil, interruptOut{}, fmt.Errorf("command %s not found or expired", in.CommandID)
	}
	sig := in.Signal
	if sig == "" {
		sig = "SIGINT"
	}
	err := exec.MCPInterrupt(in.CommandID, sig)
	s.audit(ctx, req, "interrupt_command", "", in.CommandID, nil, err)
	return nil, interruptOut{}, err
}

// ── Approval gating ──────────────────────────────────────────────

// gateExec applies the policy matrix and, when needed, asks the user.
func (s *Server) gateExec(ctx context.Context, req *mcp.CallToolRequest, sessionID, command string, risk RiskClass) error {
	policy := s.env.Policy()
	needsDialog := false
	switch policy {
	case PolicyConfirmAll:
		needsDialog = true
	case PolicyConfirmWrite:
		needsDialog = risk >= RiskWrite
	case PolicyConfirmDangerous:
		needsDialog = risk >= RiskDangerous
	case PolicyBypass:
		// Bypass all: no dialogs, not even for dangerous commands — same
		// semantics as the built-in AI panel's 全部免确认 (agent.ts returns
		// false for every risk class).
		needsDialog = false
	}
	if !needsDialog {
		return nil
	}
	return s.requestApproval(ctx, req, sessionID, command)
}

func (s *Server) gateConnect(ctx context.Context, req *mcp.CallToolRequest, connectionID string) error {
	// Opening a new connection normally always confirms: it dials out with
	// stored credentials. Under bypass (全部免确认) nothing prompts at all.
	if s.env.Policy() == PolicyBypass {
		return nil
	}
	return s.requestApproval(ctx, req, connectionID, "")
}

func (s *Server) requestApproval(ctx context.Context, req *mcp.CallToolRequest, target, command string) error {
	id := uuid.New().String()
	areq := ApprovalRequest{
		ID:         id,
		Client:     s.clientLabel(ctx, req),
		Connection: target,
		Command:    command,
		CreatedAt:  time.Now().UnixMilli(),
	}
	err := s.env.Approve(areq)
	approved := err == nil
	s.auditApproval(req, areq, approved)
	if err != nil {
		return fmt.Errorf("user did not approve: %v", err)
	}
	return nil
}

// clientLabel names the calling agent for dialogs: the token name injected by
// the auth middleware, or the MCP clientInfo as fallback.
func (s *Server) clientLabel(ctx context.Context, req *mcp.CallToolRequest) string {
	if name, _ := ctx.Value(tokenNameContextKey{}).(string); name != "" {
		return name
	}
	if sess := req.Session; sess != nil {
		if init := sess.InitializeParams(); init != nil && init.ClientInfo != nil {
			return init.ClientInfo.Name
		}
	}
	if req == nil {
		return "unknown"
	}
	return "unknown"
}

// audit writes one entry (best effort).
func (s *Server) audit(ctx context.Context, req *mcp.CallToolRequest, tool, session, command string, exit *int, err error) {
	if s.env.Audit == nil {
		return
	}
	e := AuditEntry{
		Time:    time.Now().Format(time.RFC3339),
		Token:   s.clientLabel(ctx, req),
		Tool:    tool,
		Session: session,
		Command: command,
		Error:   errString(err),
	}
	if exit != nil {
		e.ExitCode = *exit
	}
	s.env.Audit(e)
}

func (s *Server) auditApproval(req *mcp.CallToolRequest, areq ApprovalRequest, approved bool) {
	if s.env.Audit == nil {
		return
	}
	s.env.Audit(AuditEntry{
		Time:     time.Now().Format(time.RFC3339),
		Token:    areq.Client,
		Tool:     "approval",
		Command:  areq.Command,
		Approved: &approved,
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
