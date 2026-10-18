package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"go.bug.st/serial"

	"github.com/google/uuid"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
	"github.com/ys-ll/uniterm/backend/webaccess"
)

// SessionManager methods

func (a *App) CreateSession(sessionType string, config session.ConnectionConfig) (*session.SessionInfo, error) {
	if a.sessionManager == nil {
		return nil, fmt.Errorf("session manager not initialized")
	}
	if textTerminal(sessionType) && (config.InitialCols != 0 || config.InitialRows != 0) {
		if err := validWebSize(config.InitialCols, config.InitialRows); err != nil {
			return nil, err
		}
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

	// Set parent HWND for RDP sessions
	if rdp, ok := s.(*session.RDPSession); ok {
		rdp.SetParentHwnd(a.mainHwnd)
		// Notify the frontend when the user exits native full screen so it can
		// resume position sync.
		rdp.SetOnFullScreenExit(func() {
			a.emit("rdp:fullscreen-exit", s.ID())
		})
		// Adaptive resize lifecycle: the frontend freezes the last frame over
		// .rdp-area ("start", snapshot included) and restores the live window
		// afterwards ("end"), so the disconnect/reconnect cycle doesn't flash.
		rdp.SetOnAdaptiveEvent(func(event, snapshot string) {
			a.emit("rdp:adaptive", map[string]interface{}{
				"id":       s.ID(),
				"event":    event,
				"snapshot": snapshot,
			})
		})
	}

	a.sessionCaptureFor(s)
	if err := a.registerWebTerminal(s, config); err != nil {
		_ = a.sessionManager.Close(s.ID())
		return nil, err
	}
	s.SetOnDataCallback(func(data []byte) {
		a.emitSessionEvent(s, "session:data", map[string]interface{}{
			"id":   s.ID(),
			"data": string(data),
		})
	})

	s.SetOnBinaryCallback(func(data []byte) {
		a.emitSessionEvent(s, "session:binary", map[string]interface{}{
			"id":   s.ID(),
			"data": base64.StdEncoding.EncodeToString(data),
		})
	})

	s.SetOnStatusChangeCallback(func(status session.SessionStatus) {
		if status == session.StatusDisconnected || status == session.StatusError {
			a.closeWebFileSession(s.ID())
		}
		payload := map[string]interface{}{
			"id":     s.ID(),
			"status": status,
		}
		// For RDP sessions, include client area screen coordinates so the
		// frontend can position the overlay window without fragile browser APIs.
		if status == session.StatusConnected {
			if rdp, ok := s.(*session.RDPSession); ok {
				cx, cy, cw, ch := rdp.ClientAreaScreenRect()
				payload["clientX"] = cx
				payload["clientY"] = cy
				payload["clientW"] = cw
				payload["clientH"] = ch
				// The RDP ActiveX (and any credential/security dialog it owns) can
				// push uniTerm behind other windows during connect. Raise the main
				// window once the session is up so it stays visible above unrelated
				// windows. No-op on non-Windows; harmless if already in front.
				a.bringMainWindowToFront()
			}
			// Attach proxyAddr for VNC and SPICE sessions
			if vnc, ok := s.(*session.VNCSession); ok {
				payload["proxyAddr"] = vnc.ProxyAddr()
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

		a.emitSessionEvent(s, "session:status", payload)
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
	} else if textTerminal(sessionType) {
		// Terminal session types that mount xterm: defer Connect until
		// SessionStart is called after the frontend measures real cols/rows.
		// Without this gap Claude Code draws tables at the 80x24 default
		// before SessionResize propagates the real width, and the borders
		// drift across output batches.
	} else if sessionType == "vnc" || sessionType == "spice" {
		// VNC and SPICE start a local WebSocket↔TCP proxy synchronously
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
		// Non-terminal sessions (sftp, monitor, ftp, smb, webdav, s3, rdp)
		// connect immediately.
		a.launchConnectGoroutine(s, sessionType, config)
	}

	info := &session.SessionInfo{
		ID:     s.ID(),
		Type:   s.Type(),
		Title:  s.Title(),
		Status: s.Status(),
	}
	// VNC and SPICE expose a local WebSocket↔TCP proxy; return its address
	// in the CreateSession result so the frontend can mount the RFB/SPICE
	// client without racing the session:status event (whose 'connected'
	// emission happens inside s.Connect, before the frontend sets the
	// session id / stores the proxy addr — see SESSION regression).
	if vnc, ok := s.(*session.VNCSession); ok {
		info.ProxyAddr = vnc.ProxyAddr()
	}
	if spice, ok := s.(*session.SPICESession); ok {
		info.ProxyAddr = spice.ProxyAddr()
	}
	return info, nil
}

// Text-terminal Connect may finish after Close detached its session. Late
// callbacks must not recreate frontend session state after session:closed.
func (a *App) emitSessionEvent(s session.Session, name string, payload map[string]any) {
	if !textTerminal(s.Type()) {
		// Non-terminal Disconnect emits its normal status after manager removal.
		a.emit(name, payload)
		return
	}
	if terminal, err := a.webTerminalFor(s.ID()); err == nil {
		if name == "session:status" && payload["status"] == session.StatusConnected {
			terminal.mu.Lock()
			defer terminal.mu.Unlock()
			if terminal.connecting {
				// Session implementations may announce connected before Connect
				// returns. Publish readiness only after final grid reconciliation.
				terminal.connectedEvent = payload
				return
			}
		}
		terminal.delivery.Lock()
		defer terminal.delivery.Unlock()
	}
	if current, ok := a.sessionManager.Get(s.ID()); ok && current == s {
		a.emit(name, payload)
	}
}

// launchConnectGoroutine starts the async Connect path. CreateSession
// skips it for terminal session types (ssh, local, telnet, mosh, serial,
// x11-desktop) — those instead drive the connection via SessionStart after
// the frontend measures cols/rows.
func (a *App) launchConnectGoroutine(s session.Session, sessionType string, config session.ConnectionConfig) {
	terminal, _ := a.webTerminalFor(s.ID())
	if textTerminal(s.Type()) && terminal == nil {
		return
	}
	go func() {
		if terminal != nil {
			terminal.mu.Lock()
			if terminal.closed {
				terminal.mu.Unlock()
				return
			}
			terminal.connecting = true
			terminal.mu.Unlock()
		}
		var connectErr error
		defer func() {
			if r := recover(); r != nil {
				log.Writef("session %s connect panic: %v\n%s", s.ID(), r, string(debug.Stack()))
				connectErr = fmt.Errorf("connection failed unexpectedly: %v", r)
			}
			if terminal != nil {
				terminal.mu.Lock()
				defer terminal.mu.Unlock()
				terminal.connecting = false
				if terminal.closed {
					// Close detached the session while Connect owned its handles.
					// Tear down only after setup returns, including late success.
					_ = s.Disconnect()
					if a.tunnelService != nil {
						a.tunnelService.Stop(s.ID())
					}
					return
				}
				if connectErr == nil {
					// An auth-time resize may have followed Connect's initial-size
					// read. Apply the latest grid before admitting normal input.
					connectErr = s.Resize(terminal.info.Cols, terminal.info.Rows)
				}
				if connectErr != nil {
					terminal.closed = true
				} else if terminal.connectedEvent != nil {
					a.emit("session:status", terminal.connectedEvent)
					terminal.connectedEvent = nil
				}
			}
			if connectErr != nil {
				a.failSessionConnect(s, connectErr)
			}
		}()
		if clone, ok := s.(interface{ IsChannelClone() bool }); ok && clone.IsChannelClone() {
			connectErr = s.Connect(config)
			return
		}

		// ── SSH Tunnel (jump host) ──────────────────────────────
		// Set up the local-port-forward through the jump host BEFORE
		// any dial / pre-check, then point config at the local listener
		// so the subsequent s.Connect() (and the RDP pre-check below)
		// ride the tunnel. This block lives here — not in CreateSession —
		// because terminal session types defer their Connect until
		// SessionStart, by which point any config rewrite from
		// CreateSession would be discarded along with the local config
		// copy.
		if err := a.setupJumpHostTunnel(s.ID(), sessionType, &config); err != nil {
			connectErr = err
			return
		}
		// ── End SSH Tunnel ──────────────────────────────────────
		if terminal != nil {
			terminal.mu.Lock()
			closed := terminal.closed
			terminal.mu.Unlock()
			if closed {
				return
			}
		}

		// RDP TCP pre-check: fail fast before creating the ActiveX window.
		if sessionType == "rdp" {
			port := config.Port
			if port <= 0 {
				port = 3389
			}
			addr := net.JoinHostPort(config.Host, strconv.Itoa(port))
			tcpConn, tcpErr := net.DialTimeout("tcp", addr, 5*time.Second)
			if tcpErr != nil {
				log.Writef("[launchConnect] RDP TCP pre-check to %s failed: %v", addr, tcpErr)
				connectErr = fmt.Errorf("Cannot reach %s: %v", addr, tcpErr)
				return
			}
			tcpConn.Close()
			log.Writef("[launchConnect] RDP TCP pre-check to %s succeeded", addr)
		}

		connectErr = s.Connect(config)
	}()
}

// failSessionConnect is the shared error path inside launchConnectGoroutine
// for tunnel setup, RDP pre-check, and s.Connect failures. It surfaces the
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
	a.forgetWebTerminal(s.ID())
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
	data, err := a.connectionStore.Load()
	if err != nil {
		return fmt.Errorf("load connections for tunnel: %w", err)
	}
	var tunnelSSHConfig *session.ConnectionConfig
	for _, c := range data.Connections {
		if c.ID == config.TunnelSSHConnID {
			tunnelSSHConfig = &c
			break
		}
	}
	if tunnelSSHConfig == nil {
		return fmt.Errorf("tunnel SSH connection not found: %s", config.TunnelSSHConnID)
	}

	// Resolve an identity reference into a concrete password/key config
	// before handing the jump host to the tunnel service. Identity is
	// authoritative from the 密钥库(identity store); inline credentials
	// never override it.
	wasIdentity := tunnelSSHConfig.AuthType == "identity"
	if tunnelSSHConfig.AuthType == "identity" {
		m, err := a.materializeIdentity(*tunnelSSHConfig)
		if err != nil {
			return err
		}
		tunnelSSHConfig = &m
	}

	// Inline tunnel credentials (ephemeral prompt "connect" without saving)
	// only fill gaps — they never override already-resolved values, and
	// never touch an identity-resolved config.
	if !wasIdentity {
		if config.TunnelSSHUser != "" && tunnelSSHConfig.User == "" {
			tunnelSSHConfig.User = config.TunnelSSHUser
		}
		if config.TunnelSSHPassword != "" && tunnelSSHConfig.Password == "" {
			tunnelSSHConfig.Password = config.TunnelSSHPassword
		}
	}

	// VNC/SPICE use libvirt display numbers (port < 100 → 5900+N).
	targetPort := config.Port
	if sessionType == "vnc" || sessionType == "spice" {
		if targetPort <= 0 {
			targetPort = 5900
		} else if targetPort < 100 {
			targetPort += 5900
		}
	}
	localPort, err := a.tunnelService.Start(sessionID, *tunnelSSHConfig, config.Host, targetPort, config.Proxy)
	if err != nil {
		return fmt.Errorf("tunnel start: %w", err)
	}
	log.Writef("[tunnel] established for session=%s via ssh=%s, localPort=%d",
		sessionID, config.TunnelSSHConnID, localPort)
	config.Host = "127.0.0.1"
	config.Port = localPort
	config.Proxy = nil // proxy was consumed by the jump-host dial; local dial is direct
	return nil
}

// DuplicateSSHChannel opens a new session channel on an existing connected
// SSH session's authenticated client (Xshell-style "duplicate channel",
// issue #983). No re-dial, no re-auth — a jump-host/2FA login is not
// repeated. The new session defers its channel attach until SessionStart,
// like CreateSession's terminal types.
func (a *App) DuplicateSSHChannel(sourceSessionID string, config session.ConnectionConfig) (*session.SessionInfo, error) {
	if config.InitialCols != 0 || config.InitialRows != 0 {
		if err := validWebSize(config.InitialCols, config.InitialRows); err != nil {
			return nil, err
		}
	}
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
		a.emitSessionEvent(s, "session:data", map[string]interface{}{
			"id":   s.ID(),
			"data": string(data),
		})
	})
	s.SetOnBinaryCallback(func(data []byte) {
		a.emitSessionEvent(s, "session:binary", map[string]interface{}{
			"id":   s.ID(),
			"data": base64.StdEncoding.EncodeToString(data),
		})
	})
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
		a.emitSessionEvent(s, "session:status", payload)
	})

	a.sessionManager.Add(s)
	if err := a.registerWebTerminal(s, config); err != nil {
		_ = a.sessionManager.Close(s.ID())
		return nil, err
	}
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
func (a *App) SessionStart(ctx context.Context, sessionID string, config session.ConnectionConfig) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	terminal, err := a.webTerminalFor(sessionID)
	if err != nil {
		return err
	}
	if config.InitialCols != 0 || config.InitialRows != 0 {
		if err := validWebSize(config.InitialCols, config.InitialRows); err != nil {
			return err
		}
	}
	// Native callers already mounted their authority. Browser callers must ask
	// the desktop to attach before any read loop can emit its first byte.
	if webaccess.IsBrowser(ctx) {
		if _, err := a.requestWebTerminal(ctx, sessionID, true); err != nil {
			return err
		}
	}
	terminal.mu.Lock()
	defer terminal.mu.Unlock()
	if terminal.closed {
		return webError("session_closed", "terminal is closed")
	}
	if terminal.started {
		return nil
	}
	config.Type = s.Type()
	// Re-stash the latest measured size in case the deferred config
	// carries the real cols/rows the frontend discovered after mount.
	if config.InitialCols > 0 && config.InitialRows > 0 {
		s.SetPendingSize(config.InitialCols, config.InitialRows)
		terminal.info.Cols, terminal.info.Rows = config.InitialCols, config.InitialRows
		a.emit("session:resize", map[string]any{"id": sessionID, "cols": config.InitialCols, "rows": config.InitialRows, "activeClient": terminal.info.ActiveClient})
	}
	// Channel clones (issue #983) attach to an already-authenticated client:
	// skip identity/proxy resolution and jump-host tunnel setup — no dial
	// happens, and a clone of a tunneled session would otherwise spin up a
	// redundant second tunnel from the (unused) config.
	if c, ok := s.(interface{ IsChannelClone() bool }); ok && c.IsChannelClone() {
		terminal.started = true
		a.launchConnectGoroutine(s, config.Type, config)
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
	terminal.started = true
	a.launchConnectGoroutine(s, config.Type, config)
	return nil
}

func (a *App) CloseSession(sessionID string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	defer a.closeWebFileSession(sessionID)
	terminal, _ := a.webTerminalFor(sessionID)
	if terminal != nil {
		terminal.mu.Lock()
		defer terminal.mu.Unlock()
		terminal.closed = true
	}
	defer a.forgetWebTerminal(sessionID)
	if terminal != nil && terminal.connecting {
		// Disconnect must not race partially initialized session handles.
		// Remove it now; the Connect owner performs the eventual teardown.
		_, err := a.sessionManager.Detach(sessionID)
		if ssh, ok := terminal.session.(*session.SSHSession); ok {
			ssh.CancelConnect()
		}
		a.removeCapture(sessionID)
		return err
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
	return a.writeSession(sessionID, []byte(data))
}

// SessionInjectCwdHook types the OSC-7 cwd hook into a running SSH login
// shell on demand. Used by connections with cwdHookMode "follow": the
// frontend calls this when the user first enables directory follow for the
// session's file panel.
func (a *App) SessionInjectCwdHook(sessionID string) error {
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	inj, ok := s.(session.CwdHookInjector)
	if !ok {
		return fmt.Errorf("session type %s does not support on-demand cwd hook injection", s.Type())
	}
	if terminal, err := a.webTerminalFor(sessionID); err == nil {
		terminal.mu.Lock()
		defer terminal.mu.Unlock()
		if terminal.closed {
			return webError("session_closed", "terminal is closed")
		}
	}
	return inj.InjectCwdHook()
}

func (a *App) SessionResize(sessionID string, cols, rows int) error {
	if _, err := a.webTerminalFor(sessionID); err == nil {
		return a.ResizeWebTerminal(sessionID, "desktop", cols, rows, false)
	}
	if err := validWebSize(cols, rows); err != nil {
		return err
	}
	if a.sessionManager == nil {
		return fmt.Errorf("session manager not initialized")
	}
	s, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return s.Resize(cols, rows)
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
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	return a.writeSession(sessionID, data)
}

func (a *App) ReadFileBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (a *App) FileSize(path string) (int64, error) {
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
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func (a *App) AppendFileBase64(path string, base64Data string, offset int64) error {
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
	f, err := root.OpenFile(name, flag, 0644)
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
// session was created from a connection with LogOnConnect=true, the
// log is enabled automatically. Later Registers for the same panel
// never re-trigger — the user's manual stop is respected across
// reconnects for the life of the panel.
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
