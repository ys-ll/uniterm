package session

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

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

	// mcpMaxConcurrentCommands caps simultaneously running exec channels
	// (defense against runaway agent loops).
	mcpMaxConcurrentCommands = 8

	// mcpOutputCap bounds in-memory output accumulation per stream before the
	// tail starts overwriting the head.
	mcpOutputCap = DefaultMCPMaxOutput * 4
)

// mcpCommand is one exec-channel command lifecycle.
type mcpCommand struct {
	id     string
	sessMu sync.Mutex
	sess   *ssh.Session
	done   chan struct{}

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
	return c.stdout, c.stderr, c.exitCode, c.state, c.err
}

func (c *mcpCommand) setState(state string, exitCode int, err error) {
	c.mu.Lock()
	c.state = state
	c.exitCode = exitCode
	c.err = err
	c.mu.Unlock()
}

// closeChannel closes the exec channel (idempotent via ssh internals).
func (c *mcpCommand) closeChannel() {
	c.sessMu.Lock()
	if c.sess != nil {
		_ = c.sess.Close()
	}
	c.sessMu.Unlock()
}

// signalChannel delivers an SSH exec-channel signal request.
func (c *mcpCommand) signalChannel(sig ssh.Signal) error {
	c.sessMu.Lock()
	defer c.sessMu.Unlock()
	if c.sess == nil {
		return fmt.Errorf("channel closed")
	}
	return c.sess.Signal(sig)
}

// mcpRegistry tracks commands across all SSH sessions (keyed by commandId):
// MCP tool calls arrive with only a commandId, so it must be resolvable
// without knowing the session.
var mcpRegistry = struct {
	mu       sync.Mutex
	commands map[string]*mcpCommand
	running  int
}{commands: make(map[string]*mcpCommand)}

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
	if mcpRegistry.running >= mcpMaxConcurrentCommands {
		mcpRegistry.mu.Unlock()
		return nil, nil, -1, "", fmt.Errorf("too many concurrent MCP commands (max %d)", mcpMaxConcurrentCommands)
	}
	mcpRegistry.running++
	mcpRegistry.mu.Unlock()
	defer func() {
		mcpRegistry.mu.Lock()
		mcpRegistry.running--
		mcpRegistry.mu.Unlock()
	}()

	// Hold the client for the command's whole lifetime so closing the source
	// tab mid-command does not tear down the transport under us.
	ref.acquire()
	defer ref.release()

	c := &mcpCommand{
		id:    uuid.New().String(),
		done:  make(chan struct{}),
		state: "running",
	}
	mcpRegisterCommand(c)
	defer mcpRetireCommand(c)

	// Session teardown (tab closed / disconnect) kills running commands.
	quitCh := s.quit
	go func() {
		select {
		case <-quitCh:
			c.closeChannel()
			c.setState("session_closed", -1, fmt.Errorf("session closed"))
		case <-c.done:
		}
	}()

	go s.mcpRunCommand(c, ref.client, cmd, hardTimeoutMs)

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

// mcpRegisterCommand installs the command in the global registry with a TTL
// reaper so finished commands don't accumulate.
func mcpRegisterCommand(c *mcpCommand) {
	mcpRegistry.mu.Lock()
	mcpRegistry.commands[c.id] = c
	mcpRegistry.mu.Unlock()
	time.AfterFunc(mcpCommandTTL, func() {
		mcpRegistry.mu.Lock()
		delete(mcpRegistry.commands, c.id)
		mcpRegistry.mu.Unlock()
	})
}

func mcpRetireCommand(c *mcpCommand) {
	<-c.done // wait for the runner to finish before allowing removal
}

// mcpRunCommand drives one exec channel to completion.
func (s *SSHSession) mcpRunCommand(c *mcpCommand, client *ssh.Client, cmd string, hardTimeoutMs int) {
	defer close(c.done)

	sess, err := client.NewSession()
	if err != nil {
		c.setState("error", -1, err)
		return
	}
	c.sessMu.Lock()
	c.sess = sess
	c.sessMu.Unlock()

	stdoutPipe, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		c.setState("error", -1, err)
		return
	}
	stderrPipe, err := sess.StderrPipe()
	if err != nil {
		sess.Close()
		c.setState("error", -1, err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, rerr := stdoutPipe.Read(buf)
			if n > 0 {
				c.mu.Lock()
				c.appendLocked(&c.stdout, buf[:n])
				c.mu.Unlock()
			}
			if rerr != nil {
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, rerr := stderrPipe.Read(buf)
			if n > 0 {
				c.mu.Lock()
				c.appendLocked(&c.stderr, buf[:n])
				c.mu.Unlock()
			}
			if rerr != nil {
				return
			}
		}
	}()

	if err := sess.Start(cmd); err != nil {
		sess.Close()
		wg.Wait()
		c.setState("error", -1, err)
		return
	}

	hardTimer := time.NewTimer(time.Duration(hardTimeoutMs) * time.Millisecond)
	defer hardTimer.Stop()

	waitDone := make(chan error, 1)
	go func() { waitDone <- sess.Wait() }()

	select {
	case werr := <-waitDone:
		wg.Wait()
		sess.Close()
		exitCode := 0
		state := "exited"
		if werr != nil {
			var exitErr *ssh.ExitError
			if errors.As(werr, &exitErr) {
				exitCode = exitErr.ExitStatus()
			} else {
				exitCode = -1
				state = "error"
			}
		}
		c.mu.Lock()
		c.exitCode = exitCode
		c.state = state
		c.err = werr
		c.mu.Unlock()
	case <-hardTimer.C:
		sess.Close()
		wg.Wait()
		c.setState("killed", -1, fmt.Errorf("command exceeded hard timeout (%dms)", hardTimeoutMs))
	}

	log.Writef("mcp: command %s finished state=%s exit=%d", c.id, c.state, c.exitCode)
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
		c.closeChannel()
		return nil
	case "SIGTERM":
		if err := c.signalChannel(ssh.SIGTERM); err != nil {
			c.closeChannel()
		}
		return nil
	default: // SIGINT
		if err := c.signalChannel(ssh.SIGINT); err != nil {
			c.closeChannel()
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
