package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ys-ll/uniterm/backend/log"
)

// SPICEProxy bridges WebSocket (frontend spice-html5) to TCP (SPICE server).
// One instance per SPICE session, bound to a random local port.
type SPICEProxy struct {
	listener net.Listener
	server   *http.Server
	target   string
	// authToken scopes the proxy's path in server mode; also the registry
	// key for the /api/web dispatch.
	authToken string
	wg        sync.WaitGroup
	mu        sync.Mutex
	conns     map[*websocket.Conn]net.Conn
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewSPICEProxy(target string) *SPICEProxy {
	ctx, cancel := context.WithCancel(context.Background())
	return &SPICEProxy{
		target: target,
		conns:  make(map[*websocket.Conn]net.Conn),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins listening on a random local port and returns the WebSocket URL.
func (p *SPICEProxy) Start() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx.Err() != nil {
		return "", fmt.Errorf("spice proxy stopped")
	}
	if p.listener != nil {
		return "", fmt.Errorf("spice proxy already started")
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("spice proxy token: %w", err)
	}
	p.authToken = hex.EncodeToString(token[:])
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("spice proxy listen: %w", err)
	}
	p.listener = ln

	upgrader := websocket.Upgrader{
		CheckOrigin:  func(r *http.Request) bool { return true },
		Subprotocols: []string{"binary"},
	}

	mux := http.NewServeMux()
	path := "/" + hex.EncodeToString(token[:])
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Writef("[SPICEProxy] WebSocket upgrade failed: %v", err)
			return
		}
		p.handleWebSocket(ws)
	})
	p.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	// Also reachable through the server build's /api/web mount.
	registerSessionProxyHandler(p.authToken, mux)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		_ = p.server.Serve(ln)
	}()

	addr := ln.Addr().(*net.TCPAddr)
	log.Writef("[SPICEProxy] Listening on ws://127.0.0.1:%d, target: %s", addr.Port, p.target)
	return sessionProxyAddress(p.authToken, fmt.Sprintf("ws://127.0.0.1:%d%s", addr.Port, path)), nil
}

func (p *SPICEProxy) handleWebSocket(ws *websocket.Conn) {
	defer ws.Close()
	p.mu.Lock()
	if p.ctx.Err() != nil {
		p.mu.Unlock()
		return
	}
	p.conns[ws] = nil
	p.wg.Add(1)
	p.mu.Unlock()
	defer p.wg.Done()

	// Allow large SPICE frames (up to 16 MB).
	ws.SetReadLimit(16 << 20)

	defer func() {
		p.mu.Lock()
		delete(p.conns, ws)
		p.mu.Unlock()
	}()

	dialer := net.Dialer{Timeout: 5 * time.Second}
	tcp, err := dialer.DialContext(p.ctx, "tcp", p.target)
	if err != nil {
		log.Writef("[SPICEProxy] TCP dial to %s failed: %v", p.target, err)
		return
	}

	p.mu.Lock()
	if p.ctx.Err() != nil {
		p.mu.Unlock()
		tcp.Close()
		return
	}
	p.conns[ws] = tcp
	p.mu.Unlock()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		defer tcp.Close()
		defer ws.Close()
		for {
			msgType, reader, err := ws.NextReader()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				if _, err := io.Copy(tcp, reader); err != nil {
					return
				}
			}
		}
	}()
	defer func() {
		tcp.Close()
		ws.Close()
		<-readDone
	}()
	buf := make([]byte, 32768)
	for {
		n, err := tcp.Read(buf)
		if n > 0 {
			if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// Stop closes all connections and waits for goroutines to exit.
// Listener is closed before wg.Wait() so handleWebSocket has no
// window to accept and spawn goroutines that never reach Done
// (SESSION-04).
func (p *SPICEProxy) Stop() {
	p.cancel()
	// Nothing is registered when Start never ran (Stop after a failed start).
	if p.authToken != "" {
		unregisterSessionProxyHandler(p.authToken)
	}
	p.mu.Lock()
	for ws, tcp := range p.conns {
		ws.Close()
		if tcp != nil {
			tcp.Close()
		}
	}
	if p.listener != nil {
		p.listener.Close()
	}
	server := p.server
	p.mu.Unlock()
	if server != nil {
		server.Close()
	}
	p.wg.Wait()
}
