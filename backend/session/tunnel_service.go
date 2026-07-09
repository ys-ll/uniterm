package session

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// tunnelEntry holds the SSH client and listener for a single tunnel.
type tunnelEntry struct {
	sshClient *ssh.Client
	listener  net.Listener
	quit      chan struct{}
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
	addr := fmt.Sprintf("%s:%d", sshConfig.Host, sshConfig.Port)
	clientConfig := &ssh.ClientConfig{
		User:            sshConfig.User,
		Auth:            authMethods,
		Timeout:         30 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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

	// 3. Accept loop — forward each connection through SSH
	go func() {
		for {
			localConn, err := listener.Accept()
			if err != nil {
				// Listener closed; tunnel is shutting down
				return
			}
			go func() {
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

	quit := make(chan struct{})
	ts.tunnels[sessionID] = &tunnelEntry{
		sshClient: client,
		listener:  listener,
		quit:      quit,
	}
	go tunnelKeepAlive(client, quit)

	return localPort, nil
}

// tunnelKeepAlive periodically pings the tunnel's SSH connection with a
// global keepalive request (same cadence/threshold as SSHSession's) and
// closes the connection if the remote stops responding, so a dead jump-host
// hop doesn't linger silently while the forwarded session on top of it hangs.
func tunnelKeepAlive(client *ssh.Client, quit chan struct{}) {
	ticker := time.NewTicker(sshKeepAliveInterval)
	defer ticker.Stop()

	failures := 0
	for {
		select {
		case <-ticker.C:
			done := make(chan error, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						done <- fmt.Errorf("panic: %v", r)
					}
				}()
				_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
				done <- err
			}()

			select {
			case err := <-done:
				if err != nil {
					failures++
				} else {
					failures = 0
				}
			case <-time.After(sshKeepAliveTimeout):
				failures++
			}

			if failures >= sshKeepAliveMaxFail {
				client.Close()
				return
			}

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
	entry.sshClient.Close()
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
	}
}
