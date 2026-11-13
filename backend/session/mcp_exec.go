package session

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	"github.com/ys-ll/uniterm/backend/commandlimit"
	"github.com/ys-ll/uniterm/backend/log"
)

// MCP exec support: run commands on an authenticated SSH client over a
// dedicated non-PTY exec channel — separate stdout/stderr and a real exit
// code, without touching the user's interactive terminal. Commands that
// outlive the sync wait budget keep running and become pollable handles
// (commandId), bounded by a hard timeout.

const (
	// DefaultMCPMaxOutput mirrors mcp.DefaultMaxOutputBytes without importing
	// the mcp package (session must not depend on it).
	DefaultMCPMaxOutput = 64 * 1024

	// mcpCommandTTL bounds how long a finished async command's output stays
	// pollable after completion.
	mcpCommandTTL = 10 * time.Minute

	// mcpMaxConcurrentCommands is the default global exec-channel budget;
	// settings may override it within commandlimit bounds.
	mcpMaxConcurrentCommands = commandlimit.Default

	// mcpOutputCap bounds in-memory output accumulation per stream before the
	// tail starts overwriting the head.
	mcpOutputCap = DefaultMCPMaxOutput * 4
)

// mcpCommand is one exec-channel command lifecycle.
type mcpCommand struct {
	id         string
	sessMu     sync.Mutex
	sess       *ssh.Session
	done       chan struct{}
	closed     bool // guarded by sessMu; rejects channels opened after cancellation
	cancel     chan struct{}
	cancelOnce sync.Once

	mu       sync.Mutex
	stdout   []byte
	stderr   []byte
	exitCode int
	state    string // "running" | "exited" | "error" | "killed" | "session_closed"
	err      error
}

func (c *mcpCommand) appendLocked(dst *[]byte, b []byte) {
	if len(*dst)+len(b) > mcpOutputCap {
		drop := len(*dst) + len(b) - mcpOutputCap
		if drop >= len(b) {
			*dst = append((*dst)[:0], b...)
		} else {
			*dst = append((*dst)[drop:], b...)
		}
	} else {
		*dst = append(*dst, b...)
	}
}

func (c *mcpCommand) snapshot() (stdout, stderr []byte, exitCode int, state string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.stdout...), append([]byte(nil), c.stderr...), c.exitCode, c.state, c.err
}

func (c *mcpCommand) setState(state string, exitCode int, err error) {
	c.mu.Lock()
	c.state = state
	c.exitCode = exitCode
	c.err = err
	c.mu.Unlock()
}

// closeChannel also rejects a channel that is still being opened. SSH channel
// operations can block on the transport, so never hold sessMu across I/O.
func (c *mcpCommand) closeChannel() {
	c.sessMu.Lock()
	c.closed = true
	sess := c.sess
	c.sessMu.Unlock()
	if sess != nil {
		go sess.Close()
	}
}

func (c *mcpCommand) signalChannel(sig ssh.Signal) error {
	c.sessMu.Lock()
	sess, closed := c.sess, c.closed
	c.sessMu.Unlock()
	if sess == nil || closed {
		return fmt.Errorf("channel closed")
	}
	return sess.Signal(sig)
}

func (c *mcpCommand) interrupt() {
	c.cancelOnce.Do(func() { close(c.cancel) })
}

// mcpRegistry tracks commands across all SSH sessions (keyed by commandId):
// MCP tool calls arrive with only a commandId, so it must be resolvable
// without knowing the session.
var mcpRegistry = struct {
	mu            sync.Mutex
	commands      map[string]*mcpCommand
	running       int
	maxConcurrent int
}{commands: make(map[string]*mcpCommand), maxConcurrent: mcpMaxConcurrentCommands}

// SetMCPMaxConcurrentCommands updates the global background-command budget.
// Lowering it only prevents new commands; existing commands keep running.
func SetMCPMaxConcurrentCommands(limit int) {
	mcpRegistry.mu.Lock()
	mcpRegistry.maxConcurrent = commandlimit.Normalize(limit)
	mcpRegistry.mu.Unlock()
}

// MCPExec implements the mcp.SSHExecutor contract on SSHSession. It opens a
// new exec channel on the shared authenticated client, holds a clientRef for
// the command's lifetime, and returns either the completed result or an
// async handle (commandID) once the sync wait budget passes.
func (s *SSHSession) MCPExec(cmd string, syncTimeoutMs, hardTimeoutMs, maxOutput int) (stdout, stderr []byte, exitCode int, commandID string, err error) {
	s.mu.RLock()
	ref := s.clientRef
	s.mu.RUnlock()
	if ref == nil || ref.client == nil || s.Status() != StatusConnected {
		return nil, nil, -1, "", fmt.Errorf("session %s is not connected", s.id)
	}

	mcpRegistry.mu.Lock()
	if mcpRegistry.running >= mcpRegistry.maxConcurrent {
		limit := mcpRegistry.maxConcurrent
		mcpRegistry.mu.Unlock()
		return nil, nil, -1, "", fmt.Errorf("too many concurrent MCP commands (max %d across all SSH sessions); poll or interrupt existing commands, or adjust Maximum concurrent commands in MCP settings (higher limits increase memory use and remote load and may exceed SSH server channel limits)", limit)
	}
	mcpRegistry.running++
	mcpRegistry.mu.Unlock()
	// The supervisor owns the slot and client reference until completion,
	// independently of the synchronous caller's wait budget.
	ref.acquire()
	c := &mcpCommand{
		id:       uuid.New().String(),
		done:     make(chan struct{}),
		cancel:   make(chan struct{}),
		state:    "running",
		exitCode: -1,
	}
	mcpRegisterCommand(c)
	go s.mcpRunCommand(c, ref, cmd, hardTimeoutMs)

	syncTimeout := time.Duration(syncTimeoutMs) * time.Millisecond
	select {
	case <-c.done:
		out, errOut, code, _, rerr := c.snapshot()
		return truncateOutput(out, maxOutput), truncateOutput(errOut, maxOutput), code, "", rerr
	case <-time.After(syncTimeout):
		// Still running: return the async handle; the registry entry (already
		// installed) stays pollable.
		return nil, nil, -1, c.id, nil
	}
}

// Running commands remain registered; the result TTL starts on completion.
func mcpRegisterCommand(c *mcpCommand) {
	mcpRegistry.mu.Lock()
	mcpRegistry.commands[c.id] = c
	mcpRegistry.mu.Unlock()
}

func mcpRetireCommand(c *mcpCommand) {
	time.AfterFunc(mcpCommandTTL, func() {
		mcpRegistry.mu.Lock()
		delete(mcpRegistry.commands, c.id)
		mcpRegistry.mu.Unlock()
	})
}

type mcpRunResult struct {
	state string
	code  int
	err   error
}

// The supervisor bounds the entire lifecycle, including NewSession and Start.
// OpenChannel has no cancellation API on a shared SSH transport. A late result
// is therefore closed by the worker without disturbing the interactive session.
func (s *SSHSession) mcpRunCommand(c *mcpCommand, ref *sshClientRef, cmd string, hardTimeoutMs int) {
	timer := time.NewTimer(time.Duration(hardTimeoutMs) * time.Millisecond)
	defer timer.Stop()
	result := make(chan mcpRunResult, 1)
	go func() { result <- mcpExecuteCommand(c, ref.client, cmd) }()

	var r mcpRunResult
	select {
	case r = <-result:
	case <-timer.C:
		r = mcpRunResult{"killed", -1, fmt.Errorf("command exceeded hard timeout (%dms)", hardTimeoutMs)}
	case <-s.quit:
		r = mcpRunResult{"session_closed", -1, fmt.Errorf("session closed")}
	case <-c.cancel:
		r = mcpRunResult{"killed", -1, fmt.Errorf("command interrupted")}
	}
	c.setState(r.state, r.code, r.err)
	log.Writef("mcp: command %s finished state=%s exit=%d", c.id, r.state, r.code)
	c.closeChannel()
	mcpRegistry.mu.Lock()
	mcpRegistry.running--
	mcpRegistry.mu.Unlock()
	mcpRetireCommand(c)
	// release can close the transport when this was its last owner.
	// Keep completion independent of a stalled network write during Close.
	go ref.release()
	close(c.done)
}

type mcpOutputWriter struct {
	command *mcpCommand
	stderr  bool
}

func (w mcpOutputWriter) Write(p []byte) (int, error) {
	c := w.command
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == "running" {
		dst := &c.stdout
		if w.stderr {
			dst = &c.stderr
		}
		c.appendLocked(dst, p)
	}
	return len(p), nil
}

func mcpExecuteCommand(c *mcpCommand, client *ssh.Client, cmd string) mcpRunResult {
	sess, err := client.NewSession()
	if err != nil {
		return mcpRunResult{"error", -1, err}
	}
	c.sessMu.Lock()
	if c.closed {
		c.sessMu.Unlock()
		_ = sess.Close()
		return mcpRunResult{"killed", -1, fmt.Errorf("command already finished")}
	}
	c.sess = sess
	c.sessMu.Unlock()
	sess.Stdout = mcpOutputWriter{command: c}
	sess.Stderr = mcpOutputWriter{command: c, stderr: true}
	if err := sess.Start(cmd); err != nil {
		return mcpRunResult{"error", -1, err}
	}
	err = sess.Wait()
	if err == nil {
		return mcpRunResult{"exited", 0, nil}
	}
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		return mcpRunResult{"exited", exitErr.ExitStatus(), err}
	}
	return mcpRunResult{"error", -1, err}
}

// MCPLatestOutput implements the poll contract for one commandId.
func (s *SSHSession) MCPLatestOutput(commandID string, offset, max int) (stdout, stderr []byte, exitCode int, state string, err error) {
	mcpRegistry.mu.Lock()
	c, ok := mcpRegistry.commands[commandID]
	mcpRegistry.mu.Unlock()
	if !ok {
		return nil, nil, -1, "unknown", fmt.Errorf("command %s not found or expired", commandID)
	}
	out, errOut, code, st, rerr := c.snapshot()
	if max <= 0 {
		max = DefaultMCPMaxOutput
	}
	// offset is a byte offset into stdout.
	if offset > 0 {
		if offset >= len(out) {
			out = nil
		} else {
			out = out[offset:]
		}
	}
	return truncateOutput(out, max), truncateOutput(errOut, max), code, st, rerr
}

// mcpLookupCommand resolves a commandId to its runner (package-level, used by
// MCPInterrupt's caller path too).
func mcpLookupCommand(commandID string) (*mcpCommand, bool) {
	mcpRegistry.mu.Lock()
	defer mcpRegistry.mu.Unlock()
	c, ok := mcpRegistry.commands[commandID]
	return c, ok
}

// MCPInterrupt signals a running command. Supports SIGINT/SIGTERM/SIGKILL;
// SIGKILL falls back to closing the channel (the SSH channel-close teardown).
func (s *SSHSession) MCPInterrupt(commandID string, signal string) error {
	c, ok := mcpLookupCommand(commandID)
	if !ok {
		return fmt.Errorf("command %s not found or expired", commandID)
	}
	if state := func() string { c.mu.Lock(); defer c.mu.Unlock(); return c.state }(); state != "running" {
		return fmt.Errorf("command %s is not running (state=%s)", commandID, state)
	}
	switch signal {
	case "SIGKILL":
		c.interrupt()
		return nil
	case "SIGTERM":
		if err := c.signalChannel(ssh.SIGTERM); err != nil {
			c.interrupt()
		}
		return nil
	default: // SIGINT
		if err := c.signalChannel(ssh.SIGINT); err != nil {
			c.interrupt()
		}
		return nil
	}
}

// truncateOutput caps a byte slice, flagging truncation by marker.
func truncateOutput(b []byte, max int) []byte {
	if max <= 0 || len(b) <= max {
		return b
	}
	const marker = "\n…[uniterm: output truncated]"
	trunc := b[:max]
	// Cut at the last newline before the limit so we don't split a line in
	// the middle when we can help it.
	if i := strings.LastIndexByte(string(trunc), '\n'); i > max/2 {
		trunc = trunc[:i]
	}
	return append(trunc, []byte(marker)...)
}
