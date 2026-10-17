package session

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func remoteEcho(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); io.Copy(conn, conn) }()
		}
	}()
	return ln.Addr().String()
}

func remoteDial(t *testing.T, addr, origin string) *websocket.Conn {
	t.Helper()
	dialer := websocket.Dialer{Subprotocols: []string{"binary"}}
	ws, _, err := dialer.Dial(addr, http.Header{"Origin": {origin}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	return ws
}

func remoteRoundTrip(t *testing.T, ws *websocket.Conn) {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := ws.WriteMessage(websocket.BinaryMessage, []byte("independent channel")); err != nil {
		t.Fatal(err)
	}
	_, data, err := ws.ReadMessage()
	if err != nil || string(data) != "independent channel" {
		t.Fatalf("echo %q: %v", data, err)
	}
}

func remoteRejected(t *testing.T, addr, origin string, status int) {
	t.Helper()
	ws, response, err := websocket.DefaultDialer.Dial(addr, http.Header{"Origin": {origin}})
	if ws != nil {
		ws.Close()
	}
	if err == nil || response == nil || response.StatusCode != status {
		t.Fatalf("expected %d, got %v, %v", status, response, err)
	}
	response.Body.Close()
}

func TestRemoteDesktopNativeOriginAndIndependentSessions(t *testing.T) {
	for _, multi := range []bool{false, true} {
		limit := 1
		if multi {
			limit = 32
		}
		p := newRemoteDesktopProxy(remoteEcho(t), limit)
		t.Cleanup(p.Stop)
		addr, err := p.Start()
		if err != nil {
			t.Fatal(err)
		}
		for _, origin := range []string{"", "null", "https://evil.test", "http://127.0.0.1:34116", "http://localhost:34115.evil.test"} {
			remoteRejected(t, addr, origin, http.StatusForbidden)
		}
		ws := remoteDial(t, addr, "wails://localhost")
		remoteRoundTrip(t, ws)
		if multi {
			sibling := remoteDial(t, addr, "https://wails.localhost")
			remoteRoundTrip(t, sibling)
			sibling.Close()
			remoteRoundTrip(t, ws)
		} else {
			remoteRejected(t, addr, "http://127.0.0.1:34115", http.StatusConflict)
		}
		p.Stop()
		p.Stop()
	}
}

func TestRemoteDesktopWebChannelLifetime(t *testing.T) {
	p := NewSPICEProxy(remoteEcho(t))
	closed := make(chan struct{})
	if err := p.EnableWeb(func() { close(closed) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Stop)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { p.ServeWeb(w, r, r.URL.Path == "/main") }))
	defer server.Close()
	addr := "ws" + strings.TrimPrefix(server.URL, "http")
	remoteRejected(t, addr+"/channel", server.URL, http.StatusConflict)
	remoteRejected(t, addr+"/main", "http://evil.test", http.StatusForbidden)
	main := remoteDial(t, addr+"/main", server.URL)
	if main.Subprotocol() != "binary" {
		t.Fatal("SPICE binary subprotocol not negotiated")
	}
	remoteRejected(t, addr+"/main", server.URL, http.StatusConflict)
	channels := make([]*websocket.Conn, 4)
	for i := range channels {
		channels[i] = remoteDial(t, addr+"/channel", server.URL)
		remoteRoundTrip(t, channels[i])
	}
	channels[0].Close()
	remoteRoundTrip(t, main)
	remoteRoundTrip(t, channels[1])
	select {
	case <-closed:
		t.Fatal("sibling destroyed main")
	default:
	}
	main.Close()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("main did not reclaim all channels")
	}
	for _, ch := range channels {
		ch.SetReadDeadline(time.Now().Add(time.Second))
		if _, _, err := ch.ReadMessage(); err == nil {
			t.Fatal("sibling survived main")
		}
	}
	remoteRejected(t, addr+"/main", server.URL, http.StatusConflict)
}

func TestRemoteDesktopStopUpgradeRace(t *testing.T) {
	for i := 0; i < 12; i++ {
		p := NewSPICEProxy(remoteEcho(t))
		if err := p.EnableWeb(func() {}); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { p.ServeWeb(w, r, true) }))
		var wg sync.WaitGroup
		for j := 0; j < 5; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ws, response, _ := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), http.Header{"Origin": {server.URL}})
				if ws != nil {
					ws.Close()
				} else if response != nil {
					response.Body.Close()
				}
			}()
		}
		p.Stop()
		wg.Wait()
		server.Close()
	}
}

func TestRemoteDesktopAbandonedCreation(t *testing.T) {
	p := NewVNCProxy(remoteEcho(t))
	addr, err := p.Start()
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	if err := p.EnableWeb(func() { close(closed) }); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	p.timer.Reset(time.Millisecond)
	p.mu.Unlock()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("abandoned session leaked")
	}
	if ws, _, err := websocket.DefaultDialer.Dial(addr, http.Header{"Origin": {"wails://localhost"}}); err == nil {
		ws.Close()
		t.Fatal("native bypass listener survived")
	}
}

func TestRemoteDesktopSPICETargetCompatibility(t *testing.T) {
	for _, tc := range []struct {
		host string
		port int
		want string
	}{
		{"guest", 5901, "ws://guest:5901/"},
		{"::1", 1, "ws://[::1]:5901/"},
		{"[::1]", 1, "ws://[::1]:5901/"},
		{"tcp://guest", 5900, "guest:5900"},
		{"tcp://[::1]:33925", 5900, "[::1]:33925"},
		{"wss://guest:443/spice?token=example", 5900, "wss://guest:443/spice?token=example"},
	} {
		got, err := spiceTarget(ConnectionConfig{Host: tc.host, Port: tc.port})
		if err != nil || got != tc.want {
			t.Fatalf("%s: %q %v", tc.host, got, err)
		}
	}
	for _, host := range []string{"https://guest", "tcp://guest/path", "tcp://guest?target=other", "ws://user:password@guest"} {
		if _, err := spiceTarget(ConnectionConfig{Host: host}); err == nil {
			t.Fatalf("accepted %s", host)
		}
	}
}

func TestRemoteDesktopLegacyWebSocketRelay(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{Subprotocols: []string{"binary"}, CheckOrigin: nativeRemoteOrigin}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		copyRemoteMessages(ws, ws)
	}))
	defer upstream.Close()
	p := NewSPICEProxy("ws" + strings.TrimPrefix(upstream.URL, "http"))
	addr, err := p.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	ws := remoteDial(t, addr, "wails://localhost")
	remoteRoundTrip(t, ws)
}

func TestRemoteDesktopClosedSessionCannotRestart(t *testing.T) {
	// CloseAll can race CreateSession before it reaches Connect. Closing an
	// unstarted session must prevent a later Connect from leaking a listener.
	for _, s := range []Session{NewVNCSession("closed-vnc"), NewSPICESession("closed-spice")} {
		if err := s.Disconnect(); err != nil {
			t.Fatal(err)
		}
		if err := s.Connect(ConnectionConfig{Host: "127.0.0.1", Port: 5900}); err == nil {
			t.Fatal("closed session restarted")
		}
		if s.IsConnected() {
			t.Fatal("closed session became connected")
		}
	}
}
