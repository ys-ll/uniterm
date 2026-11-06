package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// TestServeSessionProxyRelay verifies the gateway's session-proxy relay end
// to end: a browser WebSocket upgrade for /api/web/session-proxy/<token> is
// resolved via the worker's plain-HTTP lookup endpoint and then relayed to
// the session proxy's internal loopback listener, whose mux guard matches on
// the rewritten /<token> path.
func TestServeSessionProxyRelay(t *testing.T) {
	const (
		token       = "0123456789abcdef0123456789abcdef"
		workerToken = "worker-secret"
	)

	// The "internal listener": a WebSocket echo behind the token-path guard,
	// shaped like the RDP/VNC/SPICE proxy muxes.
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+token {
			http.NotFound(w, r)
			return
		}
		ws, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		_ = ws
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if err := ws.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}))
	defer internal.Close()
	internalAddr := strings.TrimPrefix(internal.URL, "http://")

	// The "worker": serves the plain-HTTP lookup endpoint with the same
	// worker-token guard the real one enforces.
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Uniterm-Worker-Token") != workerToken {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/web/session-proxy-lookup/"+token {
			json.NewEncoder(w).Encode(map[string]string{"addr": internalAddr})
			return
		}
		http.NotFound(w, r)
	}))
	defer worker.Close()
	port, _ := strconv.Atoi(strings.TrimPrefix(worker.URL, "http://127.0.0.1:"))

	inst := &WorkerInstance{Port: port, token: workerToken}

	// The relay under test, behind a real (hijackable) HTTP server.
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gw := &MultiUserGateway{}
		gw.serveSessionProxyRelay(w, r, inst)
	}))
	defer relay.Close()

	// Dial through the relay like the browser would.
	url := "ws" + strings.TrimPrefix(relay.URL, "http") + "/api/web/session-proxy/" + token
	ws, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial failed: %v (HTTP %d)", err, resp.StatusCode)
		}
		t.Fatalf("dial failed: %v", err)
	}
	defer ws.Close()

	if err := ws.WriteMessage(websocket.BinaryMessage, []byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(msg) != "ping" {
		t.Fatalf("echo mismatch: %q", msg)
	}
}
