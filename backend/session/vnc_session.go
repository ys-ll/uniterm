package session

import (
	"fmt"
	"net"
	"strconv"
)

type VNCSession struct {
	baseSession
	proxy     *VNCProxy
	proxyAddr string
}

func NewVNCSession(id string) *VNCSession {
	return &VNCSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "vnc",
			status:      StatusDisconnected,
		},
	}
}

func (s *VNCSession) Connect(config ConnectionConfig) error {
	s.setStatus(StatusConnecting)

	port := config.Port
	if port <= 0 {
		port = 5900
	} else if port < 100 {
		// libvirt display port format: :1 -> 5901, :23 -> 5923
		port += 5900
	}
	target := net.JoinHostPort(config.Host, strconv.Itoa(port))

	s.title = fmt.Sprintf("%s (VNC)", config.Host)

	proxy := NewVNCProxy(target, config.Proxy)
	addr, err := proxy.Start()
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("vnc proxy start: %w", err)
	}

	s.proxy = proxy
	s.proxyAddr = addr

	// Set connected immediately so frontend gets proxyAddr.
	// The actual VNC handshake happens between noVNC and the VNC server
	// through the proxy; we don't wait for it here.
	s.setStatus(StatusConnected)

	return nil
}

func (s *VNCSession) Disconnect() error {
	if s.proxy != nil {
		s.proxy.Stop()
		s.proxy = nil
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *VNCSession) IsConnected() bool {
	return s.Status() == StatusConnected
}

func (s *VNCSession) Resize(cols, rows int) error {
	// VNC desktop size is managed by noVNC's resizeSession or the server.
	return nil
}

func (s *VNCSession) Write(data []byte) error {
	// VNC data flows through WebSocket, not this method.
	return nil
}

func (s *VNCSession) ProxyAddr() string {
	return s.proxyAddr
}
