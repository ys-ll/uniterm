package session

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type VNCSession struct{ remoteDesktopSession }

// Shared session state keeps proxy access safe during HTTP upgrades and close.
type remoteDesktopSession struct {
	baseSession
	lifecycleMu sync.Mutex
	closed      bool
	proxyMu     sync.Mutex
	proxy       *remoteDesktopProxy
	proxyAddr   string
}

func NewVNCSession(id string) *VNCSession {
	return &VNCSession{remoteDesktopSession{baseSession: baseSession{id: id, sessionType: "vnc", status: StatusDisconnected}}}
}

func remoteDesktopPort(port int) int {
	if port <= 0 {
		return 5900
	}
	if port < 100 {
		return port + 5900
	}
	return port
}

func (s *VNCSession) Connect(config ConnectionConfig) error {
	return s.connectProxy(config.Host, NewVNCProxy(net.JoinHostPort(strings.Trim(config.Host, "[]"), strconv.Itoa(remoteDesktopPort(config.Port)))).remoteDesktopProxy)
}

func (s *remoteDesktopSession) connectProxy(host string, proxy *remoteDesktopProxy) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.proxyMu.Lock()
	if s.closed || s.proxy != nil {
		s.proxyMu.Unlock()
		proxy.Stop()
		return fmt.Errorf("remote desktop session already started or closed")
	}
	s.proxy = proxy
	s.title = fmt.Sprintf("%s (%s)", host, strings.ToUpper(s.sessionType))
	addr, err := proxy.Start()
	s.proxyAddr = addr
	s.proxyMu.Unlock()
	if err != nil {
		s.setStatus(StatusError)
		return err
	}
	// This means the transport is ready. The client reports protocol success.
	s.setStatus(StatusConnected)
	return nil
}

func (s *remoteDesktopSession) EnableWeb(onClose func()) error {
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	if s.proxy == nil {
		return fmt.Errorf("remote desktop session not started")
	}
	return s.proxy.EnableWeb(onClose)
}

func (s *remoteDesktopSession) IsWeb() bool {
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	return s.proxy != nil && s.proxy.IsWeb()
}

func (s *remoteDesktopSession) ServeWeb(w http.ResponseWriter, r *http.Request, main bool) {
	s.proxyMu.Lock()
	p := s.proxy
	s.proxyMu.Unlock()
	if p == nil {
		http.Error(w, "remote desktop session unavailable", http.StatusGone)
		return
	}
	p.ServeWeb(w, r, main)
}

func (s *remoteDesktopSession) Disconnect() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.closed = true
	s.proxyMu.Lock()
	p := s.proxy
	s.proxyMu.Unlock()
	if p != nil {
		p.Stop()
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *remoteDesktopSession) IsConnected() bool { return s.Status() == StatusConnected }
func (s *remoteDesktopSession) Title() string {
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	return s.title
}
func (s *remoteDesktopSession) Resize(cols, rows int) error { return nil }
func (s *remoteDesktopSession) Write(data []byte) error     { return nil }
func (s *remoteDesktopSession) ProxyAddr() string {
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	return s.proxyAddr
}
