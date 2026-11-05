package session

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ys-ll/uniterm/backend/log"
)

// tunnelEntry holds the SSH client and listener for a single tunnel.
type tunnelEntry struct {
	sshClients []*ssh.Client
	listener   net.Listener
	quit       chan struct{}
}

// TunnelService manages SSH tunnel lifecycles.
//
// Two independent kinds of tunnel share it: session tunnels (keyed by the
// parent session ID — the implicit jump-host forwarding used by CreateSession),
// and user tunnels (keyed by Tunnel.ID — the standalone port forwards managed
// from the tunnels panel; see tunnel_forward.go).
type TunnelService struct {
	mu          sync.Mutex
	stateMu     sync.Mutex // serialize state changes and callback delivery
	closed      bool
	tunnels     map[string]*tunnelEntry
	userTunnels map[string]*userTunnelEntry
	states      map[string]TunnelState
	onState     func(TunnelState)
}

func NewTunnelService() *TunnelService {
	return &TunnelService{
		tunnels:     make(map[string]*tunnelEntry),
		userTunnels: make(map[string]*userTunnelEntry),
		states:      make(map[string]TunnelState),
	}
}

// Start establishes an SSH connection using the given config, opens a local
// TCP listener on an auto-assigned port, and forwards every accepted connection
// to targetHost:targetPort through the SSH tunnel.
// Returns the local port number that was assigned.
func (ts *TunnelService) Start(sessionID string, sshConfig ConnectionConfig, targetHost string, targetPort int, upstream *SocksProxy) (int, error) {
	return ts.StartChain(sessionID, []ConnectionConfig{sshConfig}, targetHost, targetPort, upstream)
}

// StartChain forwards a session through SSH hops ordered from entry to exit.
// Every client belongs to this session and is closed by Stop or Shutdown.
func (ts *TunnelService) StartChain(sessionID string, chain []ConnectionConfig, targetHost string, targetPort int, upstream *SocksProxy) (int, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.closed {
		return 0, fmt.Errorf("tunnel service is shut down")
	}
	if _, exists := ts.tunnels[sessionID]; exists {
		return 0, fmt.Errorf("tunnel already exists for session %s", sessionID)
	}
	if len(chain) == 0 {
		return 0, fmt.Errorf("no SSH jump hosts configured")
	}
	// Bound the whole setup, including handshakes over channels that do not
	// support socket deadlines. Allow 30 seconds per hop.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(len(chain))*30*time.Second)
	defer cancel()
	client, clients, err := ts.dialChain(ctx, chain, upstream, sessionID)
	if err != nil {
		return 0, err
	}

	// 2. Listen on random local port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		closeClients(clients)
		return 0, fmt.Errorf("tunnel listen: %w", err)
	}

	localPort := listener.Addr().(*net.TCPAddr).Port
	target := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))

	// 3. Accept loop — forward each connection through SSH
	go func() {
		for {
			localConn, err := listener.Accept()
			if err != nil {
				// Listener closed; tunnel is shutting down
				return
			}
			go func() {
				dialStart := time.Now()
				remoteConn, err := client.Dial("tcp", target)
				dialDur := time.Since(dialStart).Round(time.Millisecond)
				if err != nil {
					log.Writef("[tunnel] forward dial %s failed after %s: %v", target, dialDur, err)
					localConn.Close()
					return
				}
				pipe(localConn, remoteConn)
			}()
		}
	}()

	quit := make(chan struct{})
	ts.tunnels[sessionID] = &tunnelEntry{
		sshClients: clients,
		listener:   listener,
		quit:       quit,
	}
	for _, hop := range clients {
		go tunnelKeepAlive(hop, quit, "session="+sessionID)
	}

	return localPort, nil
}

// tunnelKeepAlive periodically pings the tunnel's SSH connection with a
// send-only global keepalive request (same cadence as SSHSession's) purely to
// keep traffic flowing so an idle jump-host hop isn't dropped by a
// server/NAT/firewall idle timeout. It does not wait for a reply or tear the
// connection down: a dead hop surfaces as EOF on the forwarded sessions riding
// it, and the OS TCP keepalive covers a silently-dropped socket.
func tunnelKeepAlive(client *ssh.Client, quit chan struct{}, label string) {
	ticker := time.NewTicker(sshKeepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// wantReply=false: crypto/ssh takes no lock and returns
			// immediately, so a slow/silent jump host can never stall this loop.
			_, _, _ = client.SendRequest("keepalive@openssh.com", false, nil)

		case <-quit:
			return
		}
	}
}

// Stop closes the tunnel and SSH connection for the given session.
func (ts *TunnelService) Stop(sessionID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	entry, ok := ts.tunnels[sessionID]
	if !ok {
		return
	}
	delete(ts.tunnels, sessionID)

	close(entry.quit)
	entry.listener.Close()
	closeClients(entry.sshClients)
}

// Shutdown closes all tunnels. Call on app shutdown.
func (ts *TunnelService) Shutdown() {
	ts.mu.Lock()
	ts.closed = true
	userIDs := make([]string, 0, len(ts.userTunnels))
	for id, entry := range ts.userTunnels {
		entry.cancel()
		userIDs = append(userIDs, id)
	}
	entries := make([]*tunnelEntry, 0, len(ts.tunnels))
	for id, entry := range ts.tunnels {
		entries = append(entries, entry)
		delete(ts.tunnels, id)
	}
	ts.mu.Unlock()

	for _, id := range userIDs {
		ts.StopTunnel(id)
	}
	for _, entry := range entries {
		close(entry.quit)
		entry.listener.Close()
		closeClients(entry.sshClients)
	}
}
