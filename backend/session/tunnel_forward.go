package session

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

// ConnResolver resolves a saved SSH connection by its ID. app.go supplies one
// backed by the connection store so the tunnel layer can look up the exit
// connection and recurse its jump hosts without importing the store package.
type ConnResolver func(id string) (ConnectionConfig, bool)

// userTunnelEntry owns one requested lifecycle, including connection attempts and
// retry waits. Its resolver (and any temporary credentials) lives only in memory.
type userTunnelEntry struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	state  TunnelState // guarded by TunnelService.mu
}

// StartTunnel connects once before returning. When automatic reconnection is
// enabled, a failed attempt returns reconnecting and retries in the background.
func (ts *TunnelService) StartTunnel(t Tunnel, resolve ConnResolver) TunnelState {
	return ts.StartTunnelContext(context.Background(), t, resolve)
}

// StartTunnelContext also honors cancellation while the caller prepares a
// tunnel. Cancelling it tears down the lifecycle and publishes stopped.
func (ts *TunnelService) StartTunnelContext(ctx context.Context, t Tunnel, resolve ConnResolver) TunnelState {
	ts.mu.Lock()
	if ts.closed || ctx.Err() != nil {
		ts.mu.Unlock()
		return TunnelState{ID: t.ID, Status: TunnelStopped}
	}
	if entry, exists := ts.userTunnels[t.ID]; exists {
		st := entry.state
		ts.mu.Unlock()
		return st
	}
	ctx, cancel := context.WithCancel(ctx)
	entry := &userTunnelEntry{
		ctx: ctx, cancel: cancel, done: make(chan struct{}),
		state: TunnelState{ID: t.ID, Status: TunnelConnecting},
	}
	ts.userTunnels[t.ID] = entry
	first := make(chan TunnelState, 1)
	go ts.runUserTunnel(t, resolve, entry, first)
	ts.mu.Unlock()
	select {
	case st := <-first:
		return st
	case <-ctx.Done():
		// A completed first attempt also cancels its lifecycle during cleanup.
		// Preserve that result when both cases became ready together.
		select {
		case st := <-first:
			return st
		default:
			return TunnelState{ID: t.ID, Status: TunnelStopped}
		}
	}
}

func tunnelReconnectDelay(seconds, jitter int) time.Duration {
	if seconds < 1 || seconds > 86400 {
		seconds = 120
	}
	base := time.Duration(seconds) * time.Second
	if jitter < 1 || jitter > 100 {
		return base
	}
	spread := base * time.Duration(jitter) / 100
	lower := max(time.Second, base-spread)
	upper := base + spread
	return lower + time.Duration(rand.Int64N(int64(upper-lower)+1))
}

func (ts *TunnelService) runUserTunnel(t Tunnel, resolve ConnResolver, entry *userTunnelEntry, first chan<- TunnelState) {
	defer func() {
		entry.cancel()
		ts.finishStoppedTunnel(t.ID, entry)
		close(entry.done)
	}()
	report := func(st TunnelState) {
		if first != nil {
			first <- st
			first = nil
		}
	}
	for {
		if !ts.publishTunnelState(entry, TunnelState{ID: t.ID, Status: TunnelConnecting}, false) {
			return
		}
		// Bound proxy negotiation, SSH handshakes, and remote listener setup.
		attempt, cancel := context.WithTimeout(entry.ctx, 30*time.Second)
		listener, clients, err := ts.openUserTunnel(attempt, t, resolve)
		cancel()
		if err == nil {
			exit := clients[len(clients)-1]
			stopClose := context.AfterFunc(entry.ctx, func() { closeClients(clients) })
			st := TunnelState{ID: t.ID, Status: TunnelRunning, LocalPort: listener.Addr().(*net.TCPAddr).Port}
			if ts.publishTunnelState(entry, st, false) {
				report(st)
				alive, stopAlive := context.WithCancel(entry.ctx)
				go userTunnelKeepAlive(alive, clients)
				exit.Wait()
				stopAlive()
			}
			stopClose()
			// Close SSH first: remote listener.Close sends an SSH request and
			// must not wait forever for an unresponsive server to acknowledge it.
			closeClients(clients)
			listener.Close()
			err = fmt.Errorf("ssh chain disconnected")
		}
		if entry.ctx.Err() != nil {
			return
		}
		st := TunnelState{ID: t.ID, Status: TunnelError, Error: err.Error()}
		if !t.AutoReconnect {
			ts.publishTunnelState(entry, st, true)
			report(st)
			return
		}
		st.Status = TunnelReconnecting
		if !ts.publishTunnelState(entry, st, false) {
			return
		}
		report(st)
		timer := time.NewTimer(tunnelReconnectDelay(t.ReconnectInterval, t.ReconnectJitter))
		select {
		case <-timer.C:
		case <-entry.ctx.Done():
			timer.Stop()
			return
		}
	}
}

func (ts *TunnelService) openUserTunnel(ctx context.Context, t Tunnel, resolve ConnResolver) (net.Listener, []*ssh.Client, error) {
	chain, err := BuildHopChain(t.SSHConnID, resolve)
	if err != nil {
		return nil, nil, err
	}
	exit, clients, err := ts.dialChain(ctx, chain, t.Upstream, t.SSHConnID)
	if err != nil {
		return nil, nil, err
	}
	stopClose := context.AfterFunc(ctx, func() { closeClients(clients) })
	listener, err := ts.startListener(t, exit)
	stopped := stopClose()
	if err == nil && (!stopped || ctx.Err() != nil) {
		err = ctx.Err()
	}
	if err != nil {
		closeClients(clients)
		if listener != nil {
			listener.Close()
		}
		return nil, nil, err
	}
	return listener, clients, nil
}

// Unlike session keepalives, a standalone tunnel must detect silent connection
// loss. A reply (including an unsupported-request reply) proves the chain is
// alive; a missing reply closes it so Wait starts the configured retry delay.
func userTunnelKeepAlive(ctx context.Context, clients []*ssh.Client) {
	client := clients[len(clients)-1]
	ticker := time.NewTicker(sshKeepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		reply := make(chan error, 1)
		go func() {
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			reply <- err
		}()
		timer := time.NewTimer(30 * time.Second)
		select {
		case err := <-reply:
			timer.Stop()
			if err != nil {
				closeClients(clients)
				return
			}
		case <-timer.C:
			closeClients(clients)
			return
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}

// startListener opens the listener appropriate to the tunnel's mode and starts
// its accept loop. For local/dynamic the listener is local (net.Listen); for
// remote it lives on the exit host (exit.Listen).
func (ts *TunnelService) startListener(t Tunnel, exit *ssh.Client) (net.Listener, error) {
	bindHost := t.ListenHost
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	bindAddr := net.JoinHostPort(bindHost, strconv.Itoa(t.ListenPort))

	switch t.Mode {
	case TunnelLocal:
		ln, err := net.Listen("tcp", bindAddr)
		if err != nil {
			return nil, fmt.Errorf("tunnel listen: %w", err)
		}
		target := net.JoinHostPort(t.TargetHost, strconv.Itoa(t.TargetPort))
		go acceptLoop(ln, func(c net.Conn) {
			remote, err := exit.Dial("tcp", target)
			if err != nil {
				c.Close()
				return
			}
			pipe(c, remote)
		})
		return ln, nil

	case TunnelDynamic:
		ln, err := net.Listen("tcp", bindAddr)
		if err != nil {
			return nil, fmt.Errorf("tunnel listen: %w", err)
		}
		go acceptLoop(ln, func(c net.Conn) {
			target, err := socks5Handshake(c)
			if err != nil {
				c.Close()
				return
			}
			remote, err := exit.Dial("tcp", target)
			if err != nil {
				socks5Reply(c, socks5HostUnreachable)
				c.Close()
				return
			}
			if err := socks5Reply(c, socks5Success); err != nil {
				remote.Close()
				c.Close()
				return
			}
			pipe(c, remote)
		})
		return ln, nil

	case TunnelRemote:
		ln, err := exit.Listen("tcp", bindAddr)
		if err != nil {
			// The common failure is the remote port already being bound — a
			// stale sshd session that never tore its forward down, or another
			// process on the server — so point the user there.
			return nil, fmt.Errorf("tunnel remote listen: %w (check whether the port is already in use on the server by a stale session or another process)", err)
		}
		target := net.JoinHostPort(t.TargetHost, strconv.Itoa(t.TargetPort))
		go acceptLoop(ln, func(c net.Conn) {
			local, err := net.Dial("tcp", target)
			if err != nil {
				c.Close()
				return
			}
			pipe(c, local)
		})
		return ln, nil

	default:
		return nil, fmt.Errorf("unknown tunnel mode %q", t.Mode)
	}
}

// TestTunnel validates a tunnel configuration without persisting or
// registering it: the tunnel is brought up through the same path as StartTunnel
// (SSH chain dial + auth, then listener bind — for remote mode that also proves
// the port is free on the server) under a throwaway ID, and torn down
// immediately once the listener is bound. Running means everything bound;
// Error carries the reason. Throwaway-ID states never reach the UI (see
// recordTunnelStateLocked), so failures are reported only through the return value.
func (ts *TunnelService) TestTunnel(t Tunnel, resolve ConnResolver) TunnelState {
	t.ID = fmt.Sprintf("__test__%d", time.Now().UnixNano())
	t.AutoReconnect = false
	st := ts.StartTunnel(t, resolve)
	ts.StopTunnel(t.ID)
	return st
}

// StopTunnel cancels retries and in-flight connections, then waits until the
// listener is released. A subsequent StartTunnel can safely reuse its port.
func (ts *TunnelService) StopTunnel(id string) {
	ts.mu.Lock()
	entry, ok := ts.userTunnels[id]
	if ok {
		entry.cancel()
	}
	ts.mu.Unlock()
	if !ok {
		return
	}
	<-entry.done
}

// The lifecycle owns final cleanup, including cancellation from a caller's
// context before StopTunnel reaches the service.
func (ts *TunnelService) finishStoppedTunnel(id string, entry *userTunnelEntry) {
	ts.stateMu.Lock()
	defer ts.stateMu.Unlock()
	ts.mu.Lock()
	if ts.userTunnels[id] != entry {
		ts.mu.Unlock()
		return
	}
	delete(ts.userTunnels, id)
	st := TunnelState{ID: id, Status: TunnelStopped}
	cb := ts.recordTunnelStateLocked(st)
	ts.mu.Unlock()
	if cb != nil {
		cb(st)
	}
}

// BuildHopChain resolves the exit connection and recurses its jump host
// (ConnectionConfig.TunnelSSHConnID) into an ordered chain, outermost first:
// [firstHop, …, exit]. A connection with no jump host yields a single-element
// chain (direct to the exit). Loops are rejected.
func BuildHopChain(exitID string, resolve ConnResolver) ([]ConnectionConfig, error) {
	if exitID == "" {
		return nil, fmt.Errorf("no SSH connection selected")
	}
	var chain []ConnectionConfig
	seen := make(map[string]bool)
	for id := exitID; id != ""; {
		if seen[id] {
			return nil, fmt.Errorf("proxy chain has a loop at connection %s", id)
		}
		seen[id] = true
		cfg, ok := resolve(id)
		if !ok {
			return nil, fmt.Errorf("SSH connection %s not found", id)
		}
		if cfg.Type != "" && cfg.Type != "ssh" {
			return nil, fmt.Errorf("jump host %s is not an SSH connection", id)
		}
		chain = append(chain, cfg)
		id = cfg.TunnelSSHConnID
	}
	// chain is exit-first; reverse to outermost-first.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// dialChain establishes the SSH clients along the chain and returns the exit
// client (chain tail) plus every client for teardown. The first hop is dialed
// directly (or through the upstream proxy); each subsequent hop is dialed
// through the previous client — the standard ProxyJump chaining.
// dialChain dials the hop chain. chainSessionID identifies the tunnel's SSH
// connection; it rides along in host key change events so the frontend can
// relate the dialog to the right connection.
func (ts *TunnelService) dialChain(ctx context.Context, chain []ConnectionConfig, upstream *SocksProxy, chainSessionID string) (*ssh.Client, []*ssh.Client, error) {
	var clients []*ssh.Client
	var prev *ssh.Client
	var stopClose []func() bool
	defer func() {
		for _, stop := range stopClose {
			stop()
		}
	}()

	for i, cfg := range chain {
		addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
		authMethods, cleanup, err := makeSSHAuthMethodsForAttempt(cfg, nil)
		if err != nil {
			closeClients(clients)
			return nil, nil, fmt.Errorf("ssh auth %s: %w", addr, err)
		}

		cleanup = sync.OnceFunc(cleanup)
		stopClose = append(stopClose, context.AfterFunc(ctx, cleanup))

		var raw net.Conn
		if i == 0 {
			raw, err = dialFirstHopContext(ctx, addr, upstream)
		} else {
			raw, err = prev.DialContext(ctx, "tcp", addr)
		}
		if err != nil {
			cleanup()
			closeClients(clients)
			return nil, nil, fmt.Errorf("dial %s: %w", addr, err)
		}

		stopClose = append(stopClose, context.AfterFunc(ctx, func() { raw.Close() }))
		algoSet, _, err := resolveSSHAlgorithms(cfg.SSHAlgorithms)
		if err != nil {
			cleanup()
			raw.Close()
			closeClients(clients)
			return nil, nil, err
		}
		clientConfig := &ssh.ClientConfig{
			User:              cfg.User,
			Auth:              authMethods,
			Timeout:           30 * time.Second,
			HostKeyCallback:   HostKeyCallback(cfg.Host, strconv.Itoa(cfg.Port), chainSessionID),
			Config:            algoSet.config(),
			HostKeyAlgorithms: algoSet.HostKeys,
		}
		sshConn, chans, reqs, err := ssh.NewClientConn(raw, addr, clientConfig)
		cleanup()
		if err != nil {
			raw.Close()
			closeClients(clients)
			return nil, nil, fmt.Errorf("ssh handshake %s: %w", addr, err)
		}
		client := ssh.NewClient(sshConn, chans, reqs)
		clients = append(clients, client)
		prev = client
	}

	return prev, clients, nil
}

// dialFirstHop dials the first hop directly, or through an upstream SOCKS5/HTTP
// proxy when configured (the chain's entry point).
func dialFirstHop(addr string, upstream *SocksProxy) (net.Conn, error) {
	return dialFirstHopContext(context.Background(), addr, upstream)
}

func dialFirstHopContext(ctx context.Context, addr string, upstream *SocksProxy) (net.Conn, error) {
	if upstream == nil {
		conn, err := (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, "tcp", addr)
		if err == nil {
			if tcp, ok := conn.(*net.TCPConn); ok {
				tcp.SetKeepAlive(true)
				tcp.SetKeepAlivePeriod(sshKeepAliveInterval)
			}
		}
		return conn, err
	}

	proxyAddr := net.JoinHostPort(upstream.Host, strconv.Itoa(upstream.Port))
	switch upstream.Kind {
	case "", "socks5":
		var auth *proxy.Auth
		if upstream.User != "" {
			auth = &proxy.Auth{User: upstream.User, Password: upstream.Pass}
		}
		d, err := proxy.SOCKS5("tcp", proxyAddr, auth, &net.Dialer{Timeout: 30 * time.Second})
		if err != nil {
			return nil, err
		}
		return d.(proxy.ContextDialer).DialContext(ctx, "tcp", addr)
	case "http":
		return dialHTTPConnectContext(ctx, proxyAddr, upstream, addr)
	default:
		return nil, fmt.Errorf("unsupported upstream proxy kind %q", upstream.Kind)
	}
}

// dialHTTPConnect opens a CONNECT tunnel through an HTTP proxy to addr.
func dialHTTPConnect(proxyAddr string, up *SocksProxy, addr string) (net.Conn, error) {
	return dialHTTPConnectContext(context.Background(), proxyAddr, up, addr)
}

func dialHTTPConnectContext(ctx context.Context, proxyAddr string, up *SocksProxy, addr string) (net.Conn, error) {
	conn, err := (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	stopClose := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopClose()
	req := "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n"
	if up.User != "" {
		cred := basicAuth(up.User, up.Pass)
		req += "Proxy-Authorization: Basic " + cred + "\r\n"
	}
	req += "\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, err
	}
	// Read status line up to the header terminator, bounded by the connect timeout.
	if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		conn.Close()
		return nil, err
	}
	buf := make([]byte, 0, 256)
	one := make([]byte, 1)
	for {
		if _, err := io.ReadFull(conn, one); err != nil {
			conn.Close()
			return nil, err
		}
		buf = append(buf, one[0])
		if len(buf) >= 4 && string(buf[len(buf)-4:]) == "\r\n\r\n" {
			break
		}
		if len(buf) > 4096 {
			conn.Close()
			return nil, fmt.Errorf("http proxy response too large")
		}
	}
	if len(buf) < 12 || string(buf[9:12]) != "200" {
		conn.Close()
		return nil, fmt.Errorf("http proxy CONNECT failed")
	}
	// Clear the read deadline so the connection is usable for the SSH session.
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// --- accept / copy helpers ---

func acceptLoop(ln net.Listener, handle func(net.Conn)) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handle(conn)
	}
}

// pipe copies bidirectionally between a and b and closes both when either side
// ends.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { io.Copy(a, b); done <- struct{}{} }()
	go func() { io.Copy(b, a); done <- struct{}{} }()
	<-done
	a.Close()
	b.Close()
}

func closeClients(clients []*ssh.Client) {
	for _, c := range clients {
		c.Close()
	}
}

func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

// --- runtime state ---

// publishTunnelState rejects results from a cancelled/replaced lifecycle and
// serializes callback delivery so an older event cannot overwrite a newer one.
func (ts *TunnelService) publishTunnelState(entry *userTunnelEntry, st TunnelState, finished bool) bool {
	ts.stateMu.Lock()
	defer ts.stateMu.Unlock()
	ts.mu.Lock()
	if ts.userTunnels[st.ID] != entry || entry.ctx.Err() != nil {
		ts.mu.Unlock()
		return false
	}
	entry.state = st
	if finished {
		delete(ts.userTunnels, st.ID)
	}
	cb := ts.recordTunnelStateLocked(st)
	ts.mu.Unlock()
	if cb != nil {
		cb(st)
	}
	return true
}

// recordTunnelStateLocked omits throwaway test IDs from snapshots and events.
func (ts *TunnelService) recordTunnelStateLocked(st TunnelState) func(TunnelState) {
	if strings.HasPrefix(st.ID, "__test__") {
		return nil
	}
	ts.states[st.ID] = st
	return ts.onState
}

// SetStateCallback registers a callback invoked whenever a tunnel's state
// changes.
func (ts *TunnelService) SetStateCallback(cb func(TunnelState)) {
	ts.mu.Lock()
	ts.onState = cb
	ts.mu.Unlock()
}

// TunnelStates returns a snapshot of all known tunnel runtime states.
func (ts *TunnelService) TunnelStates() []TunnelState {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	out := make([]TunnelState, 0, len(ts.states))
	for _, st := range ts.states {
		out = append(out, st)
	}
	return out
}

// --- minimal SOCKS5 server (CONNECT only, no auth) for dynamic tunnels ---

const (
	socks5Success         = 0x00
	socks5HostUnreachable = 0x04
)

// socks5Handshake performs the server side of a SOCKS5 CONNECT negotiation on
// conn and returns the requested "host:port". Only the no-auth method and the
// CONNECT command are supported — enough for a local dynamic-forwarding proxy.
func socks5Handshake(conn net.Conn) (string, error) {
	head := make([]byte, 2)
	if _, err := io.ReadFull(conn, head); err != nil {
		return "", err
	}
	if head[0] != 0x05 {
		return "", fmt.Errorf("socks: bad version %d", head[0])
	}
	methods := make([]byte, int(head[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", err
	}
	// Reply: version 5, no authentication required.
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return "", err
	}

	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return "", err
	}
	if req[0] != 0x05 {
		return "", fmt.Errorf("socks: bad request version %d", req[0])
	}
	if req[1] != 0x01 { // CONNECT only
		socks5Reply(conn, 0x07)
		return "", fmt.Errorf("socks: unsupported command %d", req[1])
	}

	var host string
	switch req[3] {
	case 0x01: // IPv4
		b := make([]byte, 4)
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	case 0x03: // domain
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return "", err
		}
		b := make([]byte, int(l[0]))
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", err
		}
		host = string(b)
	case 0x04: // IPv6
		b := make([]byte, 16)
		if _, err := io.ReadFull(conn, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	default:
		socks5Reply(conn, 0x08)
		return "", fmt.Errorf("socks: unknown address type %d", req[3])
	}

	pb := make([]byte, 2)
	if _, err := io.ReadFull(conn, pb); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(pb)
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

// socks5Reply writes a SOCKS5 reply with the given status and a zero
// bound-address (which clients ignore for CONNECT).
func socks5Reply(conn net.Conn, status byte) error {
	_, err := conn.Write([]byte{0x05, status, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	return err
}
