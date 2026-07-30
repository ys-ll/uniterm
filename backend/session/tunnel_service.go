package session

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// tunnelEntry holds the SSH client and listener for a single tunnel.
type tunnelEntry struct {
	sshClient *ssh.Client
	listener  net.Listener
	quit      chan struct{}
	// wg tracks every in-flight forwarder goroutine spawned by the accept
	// loop so Stop/Shutdown can join them before declaring the tunnel down.
	// Without this, a forwarder could still be holding the SSH client and a
	// half-copied pair of connections when the caller thinks the port is free.
	wg sync.WaitGroup
}

// TunnelService manages SSH tunnel lifecycles.
//
// Two independent kinds of tunnel share it: session tunnels (keyed by the
// parent session ID — the implicit jump-host forwarding used by CreateSession),
// and user tunnels (keyed by Tunnel.ID — the standalone port forwards managed
// from the tunnels panel; see tunnel_forward.go).
type TunnelService struct {
	mu          sync.Mutex
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
func (ts *TunnelService) Start(sessionID string, sshConfig ConnectionConfig, targetHost string, targetPort int) (int, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if _, exists := ts.tunnels[sessionID]; exists {
		return 0, fmt.Errorf("tunnel already exists for session %s", sessionID)
	}

	// 1. Establish SSH connection
	authMethods := makeSSHAuthMethods(sshConfig, nil)
	addr := net.JoinHostPort(sshConfig.Host, strconv.Itoa(sshConfig.Port))
	hostKeyCB, err := sshHostKeyCallback(sshConfig)
	if err != nil {
		return 0, fmt.Errorf("host key config: %w", err)
	}
	clientConfig := &ssh.ClientConfig{
		User:            sshConfig.User,
		Auth:            authMethods,
		Timeout:         30 * time.Second,
		HostKeyCallback: hostKeyCB,
		Config: ssh.Config{
			KeyExchanges: sshKeyExchanges(),
		},
	}

	conn, err := net.DialTimeout("tcp", addr, clientConfig.Timeout)
	if err != nil {
		return 0, fmt.Errorf("tunnel ssh dial: %w", err)
	}
	// TCP keepalive: same interval as direct SSH sessions, so an idle tunnel
	// (e.g. forwarding a single long-lived vim/editor session through a jump
	// host) doesn't get silently dropped by a firewall/NAT idle timeout.
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(sshKeepAliveInterval)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
	if err != nil {
		conn.Close()
		return 0, fmt.Errorf("tunnel ssh handshake: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)

	// 2. Listen on random local port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		client.Close()
		return 0, fmt.Errorf("tunnel listen: %w", err)
	}

	localPort := listener.Addr().(*net.TCPAddr).Port
	target := fmt.Sprintf("%s:%d", targetHost, targetPort)

	// Construct the entry first so the accept loop and its forwarders can
	// register with entry.wg before any handle the Stop signal.
	quit := make(chan struct{})
	entry := &tunnelEntry{
		sshClient: client,
		listener:  listener,
		quit:      quit,
	}
	ts.tunnels[sessionID] = entry

	// 3. Accept loop — forward each connection through SSH
	go func() {
		for {
			localConn, err := listener.Accept()
			if err != nil {
				// Listener closed; tunnel is shutting down
				return
			}
			entry.wg.Add(1)
			go func() {
				defer entry.wg.Done()
				remoteConn, err := client.Dial("tcp", target)
				if err != nil {
					localConn.Close()
					return
				}
				// Bidirectional copy with WaitGroup — ensures both directions
				// finish before closing the underlying connections.
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					io.Copy(remoteConn, localConn)
				}()
				go func() {
					defer wg.Done()
					io.Copy(localConn, remoteConn)
				}()
				wg.Wait()
				localConn.Close()
				remoteConn.Close()
			}()
		}
	}()

	go tunnelKeepAlive(client, quit, "session="+sessionID)

	return localPort, nil
}

// tunnelKeepAlive periodically pings the tunnel's SSH connection with a
// send-only global keepalive request (same cadence as SshSession's) purely to
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
	entry, ok := ts.tunnels[sessionID]
	if ok {
		delete(ts.tunnels, sessionID)
	}
	ts.mu.Unlock()
	if !ok {
		return
	}

	// Signal shutdown, tear down the listener and SSH client, then join the
	// in-flight forwarders. Drop ts.mu first so a slow forwarder doesn't
	// block unrelated tunnels from being started/stopped.
	close(entry.quit)
	entry.listener.Close()
	entry.sshClient.Close()
	entry.wg.Wait()
}

// Shutdown closes all tunnels. Call on app shutdown.
func (ts *TunnelService) Shutdown() {
	ts.mu.Lock()
	entries := make([]*tunnelEntry, 0, len(ts.tunnels))
	for id, entry := range ts.tunnels {
		entries = append(entries, entry)
		delete(ts.tunnels, id)
	}
	ts.mu.Unlock()

	for _, entry := range entries {
		close(entry.quit)
		entry.listener.Close()
		entry.sshClient.Close()
		entry.wg.Wait()
	}
}
