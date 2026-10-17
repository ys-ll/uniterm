package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// VNCProxy owns one native RFB connection. Browser connections use ServeWeb
// inside the authenticated listener, never a redirect to the loopback listener.
type VNCProxy struct{ *remoteDesktopProxy }

func NewVNCProxy(target string) *VNCProxy {
	return &VNCProxy{newRemoteDesktopProxy(target, 1)}
}

// remoteDesktopProxy also serves SPICE's sibling channels. Only the main Web
// socket owns the session lifetime; closing display/audio must not close main.
type remoteDesktopProxy struct {
	mu        sync.Mutex
	wg        sync.WaitGroup
	target    string
	limit     int
	server    *http.Server
	addr      string
	closed    bool
	web       bool
	main      bool
	peers     map[*websocket.Conn]struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	timer     *time.Timer
	onClose   func()
	closeOnce sync.Once
}

func newRemoteDesktopProxy(target string, limit int) *remoteDesktopProxy {
	ctx, cancel := context.WithCancel(context.Background())
	return &remoteDesktopProxy{target: target, limit: limit, peers: make(map[*websocket.Conn]struct{}), ctx: ctx, cancel: cancel}
}

// Native origins are explicit: arbitrary websites must not use a loopback
// proxy as an alternate route around Web access authentication.
func nativeRemoteOrigin(r *http.Request) bool {
	switch r.Header.Get("Origin") {
	case "wails://localhost", "wails://wails.localhost", "http://wails.localhost", "https://wails.localhost",
		"http://127.0.0.1:34115", "http://localhost:34115":
		return true
	}
	return false
}

func sameRemoteOrigin(r *http.Request) bool {
	u, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host == r.Host && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}

func (p *remoteDesktopProxy) Start() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.web || p.server != nil {
		return "", fmt.Errorf("remote desktop proxy already started or closed")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	p.addr = "ws://" + ln.Addr().String() + "/"
	p.server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || !nativeRemoteOrigin(r) {
			http.Error(w, "native remote desktop origin required", http.StatusForbidden)
			return
		}
		p.serve(w, r, false, false)
	}), ReadHeaderTimeout: 5 * time.Second}
	go p.server.Serve(ln)
	return p.addr, nil
}

// EnableWeb is called only for a newly created browser-owned session. The
// timeout also reclaims sessions when a page disappears before opening its WS.
func (p *remoteDesktopProxy) EnableWeb(onClose func()) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.web || len(p.peers) != 0 {
		return fmt.Errorf("remote desktop session is already in use or closed")
	}
	p.web, p.onClose = true, onClose
	if p.server != nil {
		_ = p.server.Close()
	}
	p.timer = time.AfterFunc(30*time.Second, p.finishWeb)
	return nil
}

func (p *remoteDesktopProxy) IsWeb() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.web
}

func (p *remoteDesktopProxy) ServeWeb(w http.ResponseWriter, r *http.Request, main bool) {
	if !sameRemoteOrigin(r) {
		http.Error(w, "same-origin WebSocket required", http.StatusForbidden)
		return
	}
	p.serve(w, r, true, main)
}

func (p *remoteDesktopProxy) serve(w http.ResponseWriter, r *http.Request, web, main bool) {
	p.mu.Lock()
	if p.closed || p.web != web || len(p.peers) >= p.limit || (web && (main == p.main)) {
		p.mu.Unlock()
		http.Error(w, "remote desktop session unavailable", http.StatusConflict)
		return
	}
	upgrader := websocket.Upgrader{Subprotocols: []string{"binary"}, CheckOrigin: func(*http.Request) bool { return true }, HandshakeTimeout: 5 * time.Second}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		p.mu.Unlock()
		return
	}
	p.peers[ws] = struct{}{}
	if web && main {
		p.main = true
		p.timer.Stop()
	}
	p.wg.Add(1) // Stop and Add share the lock; no Add-after-Wait race.
	p.mu.Unlock()
	defer func() {
		ws.Close()
		p.mu.Lock()
		delete(p.peers, ws)
		p.mu.Unlock()
		p.wg.Done()
		if web && main {
			p.finishWeb()
		}
	}()

	ctx, cancel := context.WithCancel(p.ctx)
	defer cancel()
	go func() {
		select {
		case <-r.Context().Done():
			cancel()
		case <-ctx.Done():
		}
		ws.Close()
	}()
	ws.SetReadLimit(16 << 20)
	ws.SetReadDeadline(time.Now().Add(90 * time.Second))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)) != nil {
					ws.Close()
					return
				}
			}
		}
	}()
	if u, err := url.Parse(p.target); err == nil && (u.Scheme == "ws" || u.Scheme == "wss") {
		dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second, Subprotocols: []string{"binary"}}
		upstream, _, err := dialer.DialContext(ctx, p.target, http.Header{"Origin": {r.Header.Get("Origin")}})
		if err != nil {
			return
		}
		defer upstream.Close()
		stopClose := context.AfterFunc(ctx, func() { upstream.Close() })
		defer stopClose()
		upstream.SetReadLimit(16 << 20)
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer ws.Close()
			copyRemoteMessages(ws, upstream)
		}()
		copyRemoteMessages(upstream, ws)
		upstream.Close()
		<-done
		return
	}
	tcp, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", p.target)
	if err != nil {
		return
	}
	defer tcp.Close()
	stopClose := context.AfterFunc(ctx, func() { tcp.Close() })
	defer stopClose()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer ws.Close()
		buf := make([]byte, 32768)
		for {
			n, err := tcp.Read(buf)
			if n > 0 && ws.WriteMessage(websocket.BinaryMessage, buf[:n]) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		kind, reader, err := ws.NextReader()
		if err != nil || kind != websocket.BinaryMessage {
			break
		}
		if _, err := io.Copy(tcp, reader); err != nil {
			break
		}
	}
	tcp.Close()
	<-done
}

func copyRemoteMessages(dst, src *websocket.Conn) {
	for {
		kind, data, err := src.ReadMessage()
		if err != nil || kind != websocket.BinaryMessage || dst.WriteMessage(kind, data) != nil {
			return
		}
	}
}

func (p *remoteDesktopProxy) finishWeb() {
	p.closeOnce.Do(func() {
		p.Stop()
		if p.onClose != nil {
			p.onClose()
		}
	})
}

func (p *remoteDesktopProxy) Stop() {
	p.mu.Lock()
	p.closed = true
	p.cancel()
	if p.timer != nil {
		p.timer.Stop()
	}
	if p.server != nil {
		_ = p.server.Close()
	}
	for ws := range p.peers {
		ws.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
}
