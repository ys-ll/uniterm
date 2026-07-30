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

	target := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	if config.Port <= 0 {
		target = net.JoinHostPort(config.Host, "5900")
	} else if config.Port < 100 {
		// libvirt display port format: :1 -> 5901, :23 -> 5923
		target = net.JoinHostPort(config.Host, strconv.Itoa(config.Port+5900))
	}

	s.title = fmt.Sprintf("%s (VNC)", config.Host)
	s.LogConnect(config.Host, config.Port)

	proxy := NewVNCProxy(target)
	addr, err := proxy.Start()
	if err != nil {
		s.setStatus(StatusError)
		s.LogError("vnc-proxy", err)
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
	s.LogDisconnect("user")
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
