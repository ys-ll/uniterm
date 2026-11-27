package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.bug.st/serial"

	"github.com/google/uuid"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/mcp"
	"github.com/ys-ll/uniterm/backend/recorder"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
)

// SessionManager methods

func (a *App) CreateSession(sessionType string, config session.ConnectionConfig) (*session.SessionInfo, error) {
	if a.sessionManager == nil {
		return nil, fmt.Errorf("session manager not initialized")
	}
	// Server build: a local shell runs as root inside the multi-tenant
	// container and can read every tenant's data plus the gateway's own
	// secrets (users.json, JWT key). Disabled unconditionally. local-file
	// browses the same filesystem, so it is disabled for the same reason.
	if serverMode && (sessionType == "local" || sessionType == "local-file") {
		return nil, fmt.Errorf("local terminals are disabled in server mode")
	}
	// SQLite uses config.Host as a worker-local database file; confine it
	// before the session manager opens it.
	if sessionType == "database" && config.DBType == "sqlite" {
		host, err := a.confineSQLiteHost(config.Host)
		if err != nil {
			return nil, err
		}
		config.Host = host
	}
	log.Writef("[CreateSession] type=%s, dbType=%s, host=%s, port=%d, user=%s, dbName=%s, name=%s",
		sessionType, config.DBType, config.Host, config.Port, config.User, config.DBName, config.Name)
	// 容器文件会话：通道由 container.Manager 解析（App 层拦截，不走
	// sessionManager.Create 的 config-only 构造）。
	if sessionType == "container-file" {
		return a.createContainerFileSession(config)
	}
	// Resolve an identity reference into a concrete password/key config
	// before the session manager dials.
	if config.AuthType == "identity" {
		mc, err := a.materializeIdentity(config)
		if err != nil {
			return nil, err
		}
		config = mc
	}
	// Resolve a proxy reference into a concrete proxy config so SSH-family
	// first-hop dials route through it.
	mc, err := a.materializeProxy(config)
	if err != nil {
		return nil, err
	}
	config = mc
	s, err := a.sessionManager.Create(sessionType, config)
	if err != nil {
		log.Writef("[CreateSession] manager.Create failed: %v", err)
		return nil, err
	}
	if setter, ok := s.(interface{ SetLogIdentity(string, string) }); ok {
		setter.SetLogIdentity(config.Name, config.Host)
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
	// Apply terminal character encoding. No-op for utf-8/empty.
	if ssh, ok := s.(*session.SSHSession); ok {
		ssh.SetEncoding(config.Encoding)
	}
	if telnet, ok := s.(*session.TelnetSession); ok {
		telnet.SetEncoding(config.Encoding)
	}
	if serial, ok := s.(*session.SerialSession); ok {
		serial.SetEncoding(config.Encoding)
	}
	if mosh, ok := s.(*session.MoshSession); ok {
		mosh.SetEncoding(config.Encoding)
	}
	if local, ok := s.(*session.LocalSession); ok {
		local.SetEncoding(config.Encoding)
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

	// SFTP/SCP concurrency limit
	if sessionType == "sftp" || sessionType == "scp" {
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
	}

	a.sessionCaptureFor(s)
	s.SetOnDataCallback(func(data []byte) {
		a.emit("session:data", map[string]interface{}{
			"id":   s.ID(),
			"data": string(data),
		})
	})

	s.SetOnBinaryCallback(a.instrumentedBinaryEmitter(s.ID()))

	s.SetOnStatusChangeCallback(func(status session.SessionStatus) {
		payload := map[string]interface{}{
			"id":     s.ID(),
			"status": status,
		}
		if status == session.StatusError {
			if rdp, ok := s.(*session.RDPSession); ok {
				payload["errorMessage"] = rdp.LastError()
			}
		}
		if status == session.StatusConnected {
			// Attach proxyAddr for VNC and SPICE sessions
			if vnc, ok := s.(*session.VNCSession); ok {
				payload["proxyAddr"] = vnc.ProxyAddr()
			}
			if ir, ok := s.(*session.RDPSession); ok {
				payload["proxyAddr"] = ir.ProxyAddr()
			}
			if spice, ok := s.(*session.SPICESession); ok {
				payload["proxyAddr"] = spice.ProxyAddr()
			}
			// Attach remoteOS for SSH sessions so the AI agent can distinguish
			// Windows OpenSSH (cmd/PowerShell) from Unix-like shells. Empty for
			// non-Windows or undetermined servers.
			if sshSess, ok := s.(*session.SSHSession); ok {
				if remoteOS := sshSess.RemoteOS(); remoteOS != "" {
					payload["remoteOS"] = remoteOS
				}
			}
		}

		a.emit("session:status", payload)
	})

	// Database, Redis, and MongoDB sessions connect synchronously so
	// errors are returned to the frontend try/catch.
	if sessionType == "database" || sessionType == "redis" || sessionType == "mongodb" {
		// Set up jump-host tunnel before connecting, so database/redis/mongo
		// sessions ride the tunnel just like other session types.
		if err := a.setupJumpHostTunnel(s.ID(), sessionType, &config); err != nil {
			_ = a.sessionManager.Close(s.ID())
			return nil, err
		}
		log.Writef("[CreateSession] connecting %s session synchronously...", sessionType)
		if err := s.Connect(config); err != nil {
			log.Writef("[CreateSession] %s connect failed: %v", sessionType, err)
			// Clean up any tunnel that was set up for this session.
			if a.tunnelService != nil {
				a.tunnelService.Stop(s.ID())
			}
			_ = a.sessionManager.Close(s.ID())
			return nil, fmt.Errorf("%s connect failed: %w", sessionType, err)
		}
		log.Writef("[CreateSession] %s session connected successfully, id=%s", sessionType, s.ID())
	} else if sessionType == "x11-desktop" {
		// x11-desktop uses its own X11DesktopConnect entry point; the
		// generic Connect goroutine must never call s.Connect() here.
	} else if sessionType == "ssh" || sessionType == "local" || sessionType == "wsl" || sessionType == "telnet" || sessionType == "mosh" || sessionType == "serial" {
		// Terminal session types that mount xterm: defer Connect until
		// SessionStart is called after the frontend measures real cols/rows.
		// Without this gap Claude Code draws tables at the 80x24 default
		// before SessionResize propagates the real width, and the borders
		// drift across output batches.
	} else if sessionType == "vnc" || sessionType == "spice" || sessionType == "rdp" {
		// RDP, VNC and SPICE start a local WebSocket↔TCP proxy synchronously
		// so the proxyAddr is available when CreateSession returns.
		// Avoids a race between the goroutine's session:status emit and
		// the frontend's CreateSession IPC response.
		if err := a.setupJumpHostTunnel(s.ID(), sessionType, &config); err != nil {
			_ = a.sessionManager.Close(s.ID())
			return nil, err
		}
		if err := s.Connect(config); err != nil {
			if a.tunnelService != nil {
				a.tunnelService.Stop(s.ID())
			}
			_ = a.sessionManager.Close(s.ID())
			return nil, fmt.Errorf("%s connect failed: %w", sessionType, err)
		}
	} else {
		// Non-terminal sessions (sftp, monitor, ftp, smb, webdav, s3)
		// connect immediately.
		a.launchConnectGoroutine(s, sessionType, config)
	}

	info := &session.SessionInfo{
		ID:     s.ID(),
		Type:   s.Type(),
		Title:  s.Title(),
		Status: s.Status(),
	}
	// Remote desktop sessions expose a local WebSocket↔TCP proxy; return its
	// address in CreateSession so the frontend can mount the protocol client
	// client without racing the session:status event (whose 'connected'
	// emission happens inside s.Connect, before the frontend sets the
	// session id / stores the proxy addr — see SESSION regression).
	if vnc, ok := s.(*session.VNCSession); ok {
		info.ProxyAddr = vnc.ProxyAddr()
	}
	if ir, ok := s.(*session.RDPSession); ok {
		info.ProxyAddr = ir.ProxyAddr()
	}
	if spice, ok := s.(*session.SPICESession); ok {
		info.ProxyAddr = spice.ProxyAddr()
	}
	return info, nil
}

// launchConnectGoroutine starts the async Connect path. CreateSession
// skips it for terminal session types (ssh, local, telnet, mosh, serial,
// x11-desktop) — those instead drive the connection via SessionStart after
// the frontend measures cols/rows.
func (a *App) launchConnectGoroutine(s session.Session, sessionType string, config session.ConnectionConfig) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Writef("session %s connect panic: %v\n%s", s.ID(), r, string(debug.Stack()))
			}
		}()

		// ── SSH Tunnel (jump host) ──────────────────────────────
		// Set up the local-port-forward through the jump host BEFORE
		// any dial / pre-check, then point config at the local listener
		// so the subsequent s.Connect() rides the tunnel. This block lives here
		// rather than CreateSession because terminal session types defer their
		// Connect until
		// SessionStart, by which point any config rewrite from
		// CreateSession would be discarded along with the local config
		// copy.
		if err := a.setupJumpHostTunnel(s.ID(), sessionType, &config); err != nil {
			a.failSessionConnect(s, err)
			return
		}
		// ── End SSH Tunnel ──────────────────────────────────────

		if err := s.Connect(config); err != nil {
			a.failSessionConnect(s, err)
		}
	}()
}

// failSessionConnect is the shared error path inside launchConnectGoroutine
// for tunnel setup and s.Connect failures. It surfaces the
// error to both terminal (session:data) and non-terminal (session:status)
// listeners and tears down any half-started tunnel + the session itself.
func (a *App) failSessionConnect(s session.Session, err error) {
	log.Writef("session %s connect error: %v", s.ID(), err)
	if a.tunnelService != nil {
		a.tunnelService.Stop(s.ID())
	}
	if a.ctx != nil {
		a.emit("session:status", map[string]interface{}{
			"id":           s.ID(),
			"status":       "error",
			"errorMessage": err.Error(),
		})
		a.emit("session:data", map[string]interface{}{
			"id":   s.ID(),
			"data": fmt.Sprintf("\r\n\x1b[31m[Connection failed: %v]\x1b[0m\r\nPress Enter to retry...\r\n", err),
		})
	}
	if a.sessionManager != nil {
		_ = a.sessionManager.Close(s.ID())
	}
}

// setupJumpHostTunnel establishes an SSH jump-host tunnel for the given
// session config. When config.TunnelSSHConnID is set, it opens a local
// port-forward through the referenced SSH connection and rewrites
// config.Host/Port to point at the local listener so the subsequent
// Connect call rides the tunnel.
// Returns nil when no tunnel is configured or when setup succeeds.
func (a *App) setupJumpHostTunnel(sessionID string, sessionType string, config *session.ConnectionConfig) error {
	if config.TunnelSSHConnID == "" {
		return nil
	}
	if a.tunnelService == nil || a.connectionStore == nil {
		return fmt.Errorf("tunnel prerequisites not initialized")
	}
	chain, err := a.sessionJumpHostChain(*config)
	if err != nil {
		return err
	}

	// VNC/SPICE use libvirt display numbers (port < 100 → 5900+N).
	targetPort := config.Port
	if sessionType == "vnc" || sessionType == "spice" {
		if targetPort <= 0 {
			targetPort = 5900
		} else if targetPort < 100 {
			targetPort += 5900
		}
	} else if sessionType == "rdp" && targetPort <= 0 {
		targetPort = session.DefaultRDPPort
	}
	tunnelStart := time.Now()
	localPort, err := a.tunnelService.StartChain(sessionID, chain, config.Host, targetPort, config.Proxy)
	if err != nil {
		log.Writef("[tunnel] start failed for session=%s via ssh=%s after %s: %v",
			sessionID, config.TunnelSSHConnID, time.Since(tunnelStart).Round(time.Millisecond), err)
		return fmt.Errorf("tunnel start: %w", err)
	}
	log.Writef("[tunnel] established for session=%s via ssh=%s, localPort=%d",
		sessionID, config.TunnelSSHConnID, localPort)
	config.TunnelTargetHost = config.Host
	config.TunnelTargetPort = targetPort
	config.Host = "127.0.0.1"
	config.Port = localPort
	config.Proxy = nil // proxy was consumed by the jump-host dial; local dial is direct
	return nil
}

// sessionJumpHostChain resolves saved jump hosts before any network I/O.
// Each hop uses its own identity; temporary credentials on the child only
// fill missing fields on its immediate parent and never override identities.
func (a *App) sessionJumpHostChain(config session.ConnectionConfig) ([]session.ConnectionConfig, error) {
	data, err := a.connectionStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load connections for tunnel: %w", err)
	}
	index := make(map[string]session.ConnectionConfig, len(data.Connections))
	for _, c := range data.Connections {
		index[c.ID] = c
	}
	chain, err := session.BuildHopChain(config.TunnelSSHConnID, func(id string) (session.ConnectionConfig, bool) {
		c, ok := index[id]
		return c, ok
	})
	if err != nil {
		return nil, err
	}
	for i, hop := range chain {
		child := config
		if i+1 < len(chain) {
			child = chain[i+1]
		}
		if hop.AuthType == "identity" {
			hop, err = a.materializeIdentity(hop)
			if err != nil {
				return nil, fmt.Errorf("jump host %s: %w", chain[i].ID, err)
			}
		} else {
			if hop.User == "" {
				hop.User = child.TunnelSSHUser
			}
			if hop.Password == "" {
				hop.Password = child.TunnelSSHPassword
			}
		}
		chain[i] = hop
	}
	return chain, nil
}

// DuplicateSSHChannel opens a new session channel on an existing connected
// SSH session's authenticated client (Xshell-style "duplicate channel",
// issue #983). No re-dial, no re-auth — a jump-host/2FA login is not
// repeated. The new session defers its channel attach until SessionStart,
// like CreateSession's terminal types.
func (a *App) DuplicateSSHChannel(sourceSessionID string, config session.ConnectionConfig) (*session.SessionInfo, error) {
	if a.sessionManager == nil {
		return nil, fmt.Errorf("session manager not initialized")
	}
	src, ok := a.sessionManager.Get(sourceSessionID)
	if !ok {
		return nil, fmt.Errorf("session %s not found", sourceSessionID)
	}
	srcSSH, ok := src.(*session.SSHSession)
	if !ok {
		return nil, fmt.Errorf("session %s is not an SSH session", sourceSessionID)
	}
	if !srcSSH.IsConnected() {
		return nil, fmt.Errorf("source session %s is not connected", sourceSessionID)
	}

	s := session.NewSSHChannelSession(uuid.New().String(), srcSSH)
	s.SetLogIdentity(config.Name, config.Host)
	s.SetLogOnConnect(config.LogOnConnect)
	if config.InitialCols > 0 && config.InitialRows > 0 {
		s.SetPendingSize(config.InitialCols, config.InitialRows)
	}
	s.SetEncoding(config.Encoding)

	a.sessionCaptureFor(s)
	s.SetOnDataCallback(func(data []byte) {
		a.emit("session:data", map[string]interface{}{
			"id":   s.ID(),
			"data": string(data),
		})
	})
	s.SetOnBinaryCallback(a.instrumentedBinaryEmitter(s.ID()))
	s.SetOnStatusChangeCallback(func(status session.SessionStatus) {
		payload := map[string]interface{}{
			"id":     s.ID(),
			"status": status,
		}
		if status == session.StatusConnected {
			if remoteOS := s.RemoteOS(); remoteOS != "" {
				payload["remoteOS"] = remoteOS
			}
		}
		a.emit("session:status", payload)
	})

	a.sessionManager.Add(s)
	log.Writef("[DuplicateSSHChannel] channel clone created, id=%s from=%s", s.ID(), sourceSessionID)
	return &session.SessionInfo{
		ID:     s.ID(),
		Type:   s.Type(),
		Title:  s.Title(),
		Status: s.Status(),
	}, nil
}

// SessionStart triggers the actual Connect() for terminal sessions
// (ssh, local, telnet, mosh, serial) whose Connect was deferred by
// CreateSession. The frontend calls this AFTER mounting the xterm
// terminal and measuring the real cols/rows, so the PTY is created at
// the correct dimensions from the first byte — no 80x24 default phase
// where Claude Code can draw tables at the wrong column count.
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
	// Channel clones (issue #983) attach to an already-authenticated client:
	// skip identity/proxy resolution and jump-host tunnel setup — no dial
	// happens, and a clone of a tunneled session would otherwise spin up a
	// redundant second tunnel from the (unused) config.
	if c, ok := s.(interface{ IsChannelClone() bool }); ok && c.IsChannelClone() {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Writef("session %s connect panic: %v\n%s", s.ID(), r, string(debug.Stack()))
				}
			}()
			if err := s.Connect(config); err != nil {
				a.failSessionConnect(s, err)
			}
		}()
		return nil
	}
	// Terminal session types defer Connect() until SessionStart, and the
	// frontend passes a fresh config here.
	// Resolve an identity reference into a concrete password/key config
	// before launching the connect goroutine.
	if config.AuthType == "identity" {
		mc, err := a.materializeIdentity(config)
		if err != nil {
			return err
		}
		config = mc
	}
	// Resolve a proxy reference into a concrete proxy config so SSH-family
	// first-hop dials route through it.
	mc, err := a.materializeProxy(config)
	if err != nil {
		return err
	}
	config = mc
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
	a.removeCapture(sessionID)
	return a.sessionManager.Close(sessionID)
}

func (a *App) ListSessions() []session.SessionInfo {
	if a.sessionManager == nil {
		return []session.SessionInfo{}
	}
	return a.sessionManager.List()
}

func (a *App) SessionWrite(sessionID string, data string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	// Manual keystrokes break a foreground AI job's coherence: the bytes land
	// in the running command's stdin and its completion marker may never
	// arrive, wedging the session busy until the job TTL. Finish the job (the
	// output stays pollable by id) and free the lock before the user's bytes
	// reach the terminal; no extra interrupt is sent — the user's own Ctrl+C
	// is one.
	a.jobRegistry().UserInput(sessionID)
	return s.Write([]byte(data))
}

// SessionGetCwd refreshes an SSH terminal's cwd on demand. Other terminal
// kinds use their last reported cwd (e.g. WSL's automatic OSC-7 integration).
func (a *App) SessionGetCwd(sessionID string) (string, error) {
	if a.sessionManager == nil {
		return "", fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}
	if terminal, ok := s.(*session.SSHSession); ok {
		return terminal.CurrentCwd()
	}
	return session.GetSessionCwd(sessionID), nil
}

// SessionInjectCwdHook types the WSL cwd-reporting prompt hook into a running
// session on demand. Called when the user enables directory follow for a WSL
// terminal (with an explicit UI confirmation); sessions are never injected at
// start, so a plain first login shows no handshake output (issue #1388).
func (a *App) SessionInjectCwdHook(sessionID string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	local, ok := s.(session.CwdHookInjector)
	if !ok {
		return fmt.Errorf("session type %s does not support cwd hook injection", s.Type())
	}
	return local.InjectCwdHook()
}

// SessionSetCwdTracking controls read-only cwd polling for an SSH terminal.
func (a *App) SessionSetCwdTracking(sessionID string, enabled bool) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	terminal, ok := s.(*session.SSHSession)
	if !ok {
		return fmt.Errorf("session type %s does not support SSH cwd tracking", s.Type())
	}
	return terminal.SetCwdTracking(enabled)
}

func (a *App) SessionResize(sessionID string, cols, rows int) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	// The capture emulator (MCP uniterm_capture / AI collect fallback) is
	// sized from the PTY at attach time; a resized PTY reflows the stream, so
	// the stale grid would mis-render every later frame. Recreate it instead
	// of just dropping it: snapshot its cursor row first (normally the shell
	// prompt the PTY stream never resends — foreground jobs seed from it),
	// attach the fresh emulator BEFORE the resize so ConPTY's post-resize
	// full repaint lands in it, and keep the snapshot as a fallback for
	// remotes that don't repaint on resize (SSH reflows far-side only).
	var carried mcp.ScreenSeed
	a.captureMu.Lock()
	if old, ok := a.captures[sessionID]; ok {
		old.mu.Lock()
		carried = old.screen.PromptSeed()
		if carried.Line == "" {
			carried = old.carriedSeed
		}
		old.mu.Unlock()
	}
	a.captureMu.Unlock()
	a.removeCapture(sessionID)
	cap := a.sessionCaptureFor(s)
	if err := s.Resize(cols, rows); err != nil {
		return err
	}
	cap.mu.Lock()
	cap.carriedSeed = carried
	cap.mu.Unlock()
	return nil
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
	s.EndZmodem(nil)
	return nil
}

// SessionEndZmodemWithTrailing leaves binary mode and emits terminal output
// that zmodem.js found after the final handshake through the normal text path,
// decoded as UTF-8 according to the session's configured character encoding.
func (a *App) SessionEndZmodemWithTrailing(sessionID, base64Data string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("decode zmodem trailing data: %w", err)
	}
	s.EndZmodem(data)
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
	path, err := a.confineLocalPath(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (a *App) FileSize(path string) (int64, error) {
	path, err := a.confineLocalPath(path)
	if err != nil {
		return 0, err
	}
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

	path, err := a.confineLocalPath(path)
	if err != nil {
		return "", err
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
	path, err := a.confineLocalPath(path)
	if err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func (a *App) AppendFileBase64(path string, base64Data string, offset int64) error {
	path, err := a.confineLocalPath(path)
	if err != nil {
		return err
	}
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
	// Resolve the final component through os.Root. Root.OpenFile guarantees
	// that a symlink swapped in after Lstat cannot escape the parent directory.
	dir, name := filepath.Split(path)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open download directory: %w", err)
	}
	defer root.Close()
	if info, statErr := root.Lstat(name); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write symbolic link: %s", path)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect file: %w", statErr)
	}
	f, err := openWithLockRetry(root, name, flag)
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

// SetMonitorSections toggles which monitor sections the poll loop collects
// (cpu/allCpu/memory/disk/network). Disabled sections' remote commands are
// not sent, reducing load on the monitored node.
func (a *App) SetMonitorSections(sessionID string, sections map[string]bool) error {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return err
	}
	ms.SetMonitorSections(sections)
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

func (a *App) GetServices(sessionID string) ([]session.ServiceInfo, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.GetServices()
}

func (a *App) GetServiceDetail(sessionID string, name string) (map[string]string, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.GetServiceDetail(name)
}

func (a *App) GetServiceLogs(sessionID string, name string, lines int) (string, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return "", err
	}
	return ms.GetServiceLogs(name, lines)
}

func (a *App) ServiceAction(sessionID string, name string, action string) error {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return err
	}
	return ms.ServiceAction(name, action)
}

func (a *App) GetDevices(sessionID string) ([]session.DeviceInfo, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.GetDevices()
}

func (a *App) GetHardwareFru(sessionID string) (*session.FruInfo, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.GetHardwareFru()
}

func (a *App) GetHardwareLan(sessionID string) ([]session.LanField, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.GetHardwareLan()
}

func (a *App) GetHardwareSensors(sessionID string) (*session.HardwareSensors, error) {
	ms, err := a.getMonitorSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ms.GetHardwareSensors()
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
// global sessionLogAutoStart setting is on or the session was created
// from a connection with LogOnConnect=true, the log is enabled
// automatically. Later Registers for the same panel never re-trigger —
// the user's manual stop is respected across reconnects for the life
// of the panel.
func (a *App) RegisterSessionForPanel(sessionID, panelID string) {
	if sessionID == "" || panelID == "" {
		return
	}
	a.panelLogMu.Lock()
	a.sessionToPanel[sessionID] = panelID
	logger := a.panelLogs[panelID]
	autoTriggered := a.panelAutoTriggered[panelID]
	a.panelLogMu.Unlock()

	// Existing logger (reconnect case): rewire writer, don't re-enable.
	if logger != nil {
		a.installWriter(sessionID, logger)
		return
	}

	// First Register for this panel: check the global auto-log setting
	// and the per-connection LogOnConnect opt-in, and auto-enable.
	if !autoTriggered {
		a.panelLogMu.Lock()
		a.panelAutoTriggered[panelID] = true
		a.panelLogMu.Unlock()
		if a.autoLogRequested(sessionID) {
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

// UnregisterSession clears the session\u2192panel binding and detaches any
// writer from the session. The logger itself is unaffected: it stays on
// the panel, waiting for the next session (reconnect) to register.
func (a *App) UnregisterSession(sessionID string) {
	if sessionID == "" {
		return
	}
	cancelExternalEdits(sessionID)
	a.panelLogMu.Lock()
	delete(a.sessionToPanel, sessionID)
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
func (a *App) panelLogTitle(panelID string) (name, host, protocol string) {
	a.panelLogMu.Lock()
	var sessionID string
	for sid, pid := range a.sessionToPanel {
		if pid == panelID {
			sessionID = sid
			break
		}
	}
	a.panelLogMu.Unlock()
	if sessionID != "" && a.sessionManager != nil {
		if s, ok := a.sessionManager.Get(sessionID); ok {
			name, host = s.Title(), ""
			if identity, ok := s.(interface{ LogIdentity() (string, string) }); ok {
				configuredName, configuredHost := identity.LogIdentity()
				if configuredName != "" {
					name = configuredName
				}
				host = configuredHost
			}
			return name, host, s.Type()
		}
	}
	suffix := panelID
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return "panel_" + suffix, "", "session"
}

// EnableSessionOutputLog starts writing terminal output for the given
// panel to a .log file. If dir is empty, the default session log
// directory is used. Returns the final path after sanitization and
// same-second collision suffixing.
//
// The log is bound to the panel, not the session \u2014 so a reconnect
// (which creates a fresh session under the same panel) keeps writing
// to the same file.
// DefaultSessionLogDir exposes the OS-default session log directory so the
// UI can open it even when no panel log is currently running.
func (a *App) DefaultSessionLogDir() string {
	return session.DefaultSessionLogDir()
}

func (a *App) EnableSessionOutputLog(panelID, dir string) (string, error) {
	if panelID == "" {
		return "", fmt.Errorf("panelID required")
	}
	// When the caller didn't pin a directory, fall back to the user's
	// configured override; if that is also empty, OutputLogger.Enable
	// will pick the OS default.
	if dir == "" {
		a.sessionLogSettingsMu.RLock()
		dir = a.customLogDir
		a.sessionLogSettingsMu.RUnlock()
	}
	if dir != "" {
		confined, err := a.confineLocalPath(dir)
		if err != nil {
			return "", err
		}
		dir = confined
	}
	name, host, protocol := a.panelLogTitle(panelID)
	a.sessionLogSettingsMu.RLock()
	filenameTemplate := a.sessionLogFilename
	a.sessionLogSettingsMu.RUnlock()

	a.panelLogMu.Lock()
	logger := a.panelLogs[panelID]
	if logger == nil {
		logger = &session.OutputLogger{}
		a.panelLogs[panelID] = logger
	}
	// Find any session currently bound to this panel so we can wire the
	// writer while we still hold the lock (avoids a race with concurrent
	// register/unregister calls).
	var sessionID string
	for sid, pid := range a.sessionToPanel {
		if pid == panelID {
			sessionID = sid
			break
		}
	}
	a.panelLogMu.Unlock()

	path, err := logger.Enable(dir, filenameTemplate, name, host, protocol)
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
	var sessionID string
	for sid, pid := range a.sessionToPanel {
		if pid == panelID {
			sessionID = sid
			break
		}
	}
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

// OpenSessionLogDir opens the effective session log directory (the user's
// configured override, else the OS default) in the platform file manager,
// creating the directory first when it does not exist yet. Used by the
// frontend's "open log dir" menu when no per-panel log is active — without
// the mkdir the explorer call on a not-yet-created directory is a silent
// no-op.
func (a *App) OpenSessionLogDir() error {
	a.sessionLogSettingsMu.RLock()
	dir := a.customLogDir
	a.sessionLogSettingsMu.RUnlock()
	if dir == "" {
		dir = session.DefaultSessionLogDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir log dir %s: %w", dir, err)
	}
	return a.OpenPathInExplorer(dir)
}

// SetDefaultSessionLogDir installs a user-configured override for the
// directory used by new session logs. Empty clears the override and
// restores the OS default. Existing log files are not migrated; the
// change only affects logs enabled after this call.
func (a *App) SetDefaultSessionLogDir(dir string) {
	a.sessionLogSettingsMu.Lock()
	a.customLogDir = dir
	a.sessionLogSettingsMu.Unlock()
}

// setSessionLogFilename installs the template used for newly created logs.
// Empty selects session.DefaultSessionLogFilenameTemplate.
func (a *App) setSessionLogFilename(filenameTemplate string) {
	a.sessionLogSettingsMu.Lock()
	a.sessionLogFilename = filenameTemplate
	a.sessionLogSettingsMu.Unlock()
}

// setSessionLogAutoStart installs the global auto-log preference. When
// true, the first session registered on a panel starts its output log
// automatically. False keeps the manual / per-connection behavior.
func (a *App) setSessionLogAutoStart(v bool) {
	a.sessionLogSettingsMu.Lock()
	a.sessionLogAutoStart = v
	a.sessionLogSettingsMu.Unlock()
}

// autoLogRequested reports whether a freshly created session should have
// its output log started automatically: either the global
// sessionLogAutoStart setting is on, or the connection opted in via
// LogOnConnect.
func (a *App) autoLogRequested(sessionID string) bool {
	a.sessionLogSettingsMu.RLock()
	auto := a.sessionLogAutoStart
	a.sessionLogSettingsMu.RUnlock()
	if auto {
		return true
	}
	return a.sessionWantsAutoLog(sessionID)
}

// GetDefaultSessionLogDir returns the directory a fresh session log
// would land in: the user's override if set, otherwise the OS default
// (~/Documents/uniTerm/logs on all platforms). Used by the settings UI
// to show the current default path as a placeholder.
func (a *App) GetDefaultSessionLogDir() string {
	a.sessionLogSettingsMu.RLock()
	custom := a.customLogDir
	a.sessionLogSettingsMu.RUnlock()
	if custom != "" {
		return custom
	}
	return session.DefaultSessionLogDir()
}

// GetSupportedSSHAlgorithms returns the SSH algorithm candidate pool with
// security levels plus the compatible/secure presets, for the connection
// form's algorithm editor.
func (a *App) GetSupportedSSHAlgorithms() session.SupportedSSHAlgorithms {
	return session.GetSupportedSSHAlgorithms()
}

// AcceptHostKeyChange replaces the known_hosts fingerprint for host:port
// with the new key after the user confirmed a host key change in the
// frontend dialog. See session.AcceptHostKeyChange.
func (a *App) AcceptHostKeyChange(host, keyType, newKeyB64 string, port int) error {
	return session.AcceptHostKeyChange(host, strconv.Itoa(port), keyType, newKeyB64)
}

// AcceptHostKeyOnce records a one-shot bypass for host:port, letting the
// next connection present the given key without recording it in
// known_hosts ("connect once, don't save" in the change dialog). See
// session.AcceptHostKeyOnce.
func (a *App) AcceptHostKeyOnce(host, keyType, newKeyB64 string, port int) error {
	return session.AcceptHostKeyOnce(host, strconv.Itoa(port), newKeyB64)
}

// instrumentedBinaryEmitter returns a session:binary emit callback tagged
// with per-session instrumentation for the issue #1582 investigation: a
// sequence number (gaps = lost or reordered chunks), the raw length and an
// IEEE CRC-32 of the chunk (mismatch = content corruption). The frontend
// checks these on arrival and reports anomalies via FrontendLog; a clean
// stream (seq contiguous, crc matching) rules the transport out as the
// corruption source.
func (a *App) instrumentedBinaryEmitter(id string) func([]byte) {
	seq := 0
	return func(data []byte) {
		seq++
		// Periodic heartbeat so the backend emission timeline can be
		// correlated with the frontend's receipt log after a failure.
		if seq%100 == 0 {
			log.Writef("zmodem-bin %s emitted seq=%d n=%d", id, seq, len(data))
		}
		a.emit("session:binary", map[string]interface{}{
			"id":   id,
			"data": base64.StdEncoding.EncodeToString(data),
			"seq":  seq,
			"n":    len(data),
			"crc":  crc32.ChecksumIEEE(data),
		})
	}
}

// openWithLockRetry opens name under root, retrying briefly when the OS
// reports a transient file lock. Antivirus engines hold freshly written
// executables for a scan the moment they look complete, so a long zmodem
// download can fail at a batch re-open with "being used by another process"
// even though the transfer itself is healthy (issue #1582). The transfer
// tolerates a short stall — sz keeps streaming into the SSH channel window
// while we wait — so the retry window is generous but bounded.
func openWithLockRetry(root *os.Root, name string, flag int) (*os.File, error) {
	f, err := root.OpenFile(name, flag, 0644)
	if err == nil || !isTransientFileLockError(err) {
		return f, err
	}
	for attempt := 0; attempt < fileLockRetries && err != nil; attempt++ {
		time.Sleep(fileLockRetryDelay)
		f, err = root.OpenFile(name, flag, 0644)
	}
	return f, err
}

const (
	// 15 × 200ms = 3s of tolerance for a virus-scanner hold on the target.
	fileLockRetries    = 15
	fileLockRetryDelay = 200 * time.Millisecond
)

// isTransientFileLockError reports whether err is a Windows sharing violation:
// another process (typically a virus scanner holding a freshly written
// executable for inspection) has the file open without share access. These
// holds are brief, so callers retry.
//
// The two errno values are compared numerically so a single definition serves
// every platform: on Windows syscall.Errno 32/33 are exactly
// ERROR_SHARING_VIOLATION / ERROR_LOCK_VIOLATION, and on POSIX an OpenFile
// never fails with EPIPE (32) / EDOM (33), so the check can only ever be
// false there.
func isTransientFileLockError(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == 32 || errno == 33
	}
	return false
}
// app_recorder.go exposes the terminal recorder (asciinema cast v2) as Wails
// bindings. The frontend terminalRecorder.ts service calls these to start
// recording a session, stream session:data into the cast file, and later
// list/delete/play back recordings.
//
// All methods delegate to the process-wide recorder.Recorder singleton
// (recorder.New()), so no App field is required. The singleton pattern keeps
// the hot data path (RecorderWriteData) lock-free at the App layer.

// recorderInst returns the shared Recorder. Constructed lazily on first call
// so apps that never record pay zero cost.
func recorderInst() *recorder.Recorder {
	return recorder.New()
}

// RecorderStart creates a new cast v2 file under <home>/Documents/uniTerm/records/
// and writes the JSON header. Returns the absolute file path the frontend should
// pass to subsequent RecorderWriteData / RecorderStop calls.
func (a *App) RecorderStart(sessionID string, width int, height int, title string) (string, error) {
	return recorderInst().StartRecording(sessionID, width, height, title)
}

// RecorderWriteData appends one `[elapsed, "o", data]` row to the cast file
// at filePath. Called from the terminal session:data hook in BaseTerminal.vue.
// No-op (returns nil) if the path is not an open recording — the frontend
// data path must not error when a recording was stopped concurrently.
func (a *App) RecorderWriteData(filePath string, data string) error {
	filePath, err := a.confineLocalPath(filePath)
	if err != nil {
		return err
	}
	return recorderInst().WriteRecordingData(filePath, data)
}

// RecorderStop flushes and closes the recording at filePath. Idempotent.
func (a *App) RecorderStop(filePath string) error {
	filePath, err := a.confineLocalPath(filePath)
	if err != nil {
		return err
	}
	return recorderInst().StopRecording(filePath)
}

// RecorderIsRecording reports whether filePath is currently being recorded.
func (a *App) RecorderIsRecording(filePath string) bool {
	return recorderInst().IsRecording(filePath)
}

// RecorderList returns metadata for every .cast file in the records
// directory, newest first.
func (a *App) RecorderList() ([]recorder.RecordingInfo, error) {
	return recorderInst().ListRecordings()
}

// RecorderDelete removes a cast file from disk. If the recording is open it
// is stopped first.
func (a *App) RecorderDelete(filePath string) error {
	filePath, err := a.confineLocalPath(filePath)
	if err != nil {
		return err
	}
	return recorderInst().DeleteRecording(filePath)
}

// RecorderGetContent reads the entire cast file as a string. The frontend
// player parses the header + data rows to drive xterm.js replay.
func (a *App) RecorderGetContent(filePath string) (string, error) {
	filePath, err := a.confineLocalPath(filePath)
	if err != nil {
		return "", err
	}
	return recorderInst().GetRecordingContent(filePath)
}

// RecorderOpenDir opens the records directory in the OS file manager.
func (a *App) RecorderOpenDir() error {
	dir := recorderInst().RecordsDir()
	if dir == "" {
		return fmt.Errorf("recorder: records directory unavailable")
	}
	return a.OpenPathInExplorer(dir)
}

// Newest entries only. Shell history files grow without bound and the whole
// list is held in memory and scanned per keystroke.
const shellHistoryLimit = 1000

// shellHistoryTailBytes caps how much of the history file is read. The whole
// file is never loaded: a multi-hundred-megabyte ~/.bash_history would
// otherwise cause a memory spike for data the limit above throws away anyway.
// parseShellHistory only needs the newest commands, which live at the end.
const shellHistoryTailBytes = 4 << 20 // 4 MiB

// zsh with EXTENDED_HISTORY writes ": <started>:<elapsed>;<command>".
var zshTimestampPrefix = regexp.MustCompile(`^: \d+:\d*;`)

// shellHistoryPath locates the history file for the user's login shell, or ""
// when there is nothing sensible to read.
func shellHistoryPath() string {
	if histFile := os.Getenv("HISTFILE"); histFile != "" {
		return histFile
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch shell := os.Getenv("SHELL"); {
	case strings.Contains(shell, "zsh"):
		return filepath.Join(home, ".zsh_history")
	case strings.Contains(shell, "bash"):
		return filepath.Join(home, ".bash_history")
	}
	// Unknown or unset SHELL: take whichever file exists, zsh first since it is
	// the macOS default. fish is deliberately not handled — its history is YAML
	// rather than one command per line and needs its own parser.
	for _, name := range []string{".zsh_history", ".bash_history"} {
		path := filepath.Join(home, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// parseShellHistory returns commands newest first, de-duplicated, capped at
// limit. Invalid UTF-8 is dropped rather than rejected: history files
// accumulate whatever was typed, including broken encodings, and one bad byte
// should not cost the user the whole file.
func parseShellHistory(raw string, limit int) []string {
	lines := strings.Split(strings.ToValidUTF8(raw, ""), "\n")

	commands := make([]string, 0, len(lines))
	var pending []string
	for _, line := range lines {
		if len(pending) == 0 {
			line = zshTimestampPrefix.ReplaceAllString(line, "")
			// bash writes "#<epoch>" comment lines when HISTTIMEFORMAT is set.
			if strings.HasPrefix(line, "#") {
				continue
			}
		}
		// A trailing backslash encodes a newline inside the command.
		if strings.HasSuffix(line, `\`) {
			pending = append(pending, strings.TrimSuffix(line, `\`))
			continue
		}
		pending = append(pending, line)
		command := strings.TrimSpace(strings.Join(pending, "\n"))
		pending = nil

		// Genuinely multi-line commands (loops, heredocs) cannot be offered as
		// a single-line completion. A trailing newline alone trims away above,
		// so this only drops the ones that really span lines.
		if command == "" || strings.Contains(command, "\n") {
			continue
		}
		commands = append(commands, command)
	}

	// Newest first, keeping the most recent occurrence of a repeated command.
	seen := make(map[string]struct{}, len(commands))
	recent := make([]string, 0, limit)
	for i := len(commands) - 1; i >= 0 && len(recent) < limit; i-- {
		if _, dup := seen[commands[i]]; dup {
			continue
		}
		seen[commands[i]] = struct{}{}
		recent = append(recent, commands[i])
	}
	return recent
}

// readShellHistoryTail reads at most limit bytes from the end of path and
// reports whether the read was truncated. When the file is larger than the
// limit the first (partial) line is dropped — the read starts mid-entry, so
// that line is truncated garbage.
func readShellHistoryTail(path string, limit int64) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if info.Size() <= limit {
		raw, err := os.ReadFile(path)
		return raw, false, err
	}
	if _, err := file.Seek(info.Size()-limit, 0); err != nil {
		return nil, false, err
	}
	raw := make([]byte, limit)
	n, err := io.ReadFull(file, raw)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, false, err
	}
	raw = raw[:n]
	if _, rest, found := bytes.Cut(raw, []byte("\n")); found {
		return rest, true, nil
	}
	return raw, true, nil
}

// LoadShellHistory returns the local login shell's history, newest first.
// A missing or unreadable file is not an error — it just means there is
// nothing extra to suggest.
func (a *App) LoadShellHistory() ([]string, error) {
	path := shellHistoryPath()
	if path == "" {
		return []string{}, nil
	}
	raw, _, err := readShellHistoryTail(path, shellHistoryTailBytes)
	if err != nil {
		return []string{}, nil
	}
	return parseShellHistory(string(raw), shellHistoryLimit), nil
}
