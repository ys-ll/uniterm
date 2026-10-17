package main

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ys-ll/uniterm/backend/session"
)

// Opt-in protocol harness. The Vue components and Go routes are real; only the
// application shell/RPC facade is replaced. See frontend/e2e/remote/README.md.
func TestWebRemoteDesktopProtocolE2E(t *testing.T) {
	vite := os.Getenv("UNITERM_REMOTE_E2E_VITE")
	if vite == "" {
		t.Skip("requires real VNC/SPICE servers and the Vite E2E harness")
	}
	target, err := url.Parse(vite)
	if err != nil {
		t.Fatal(err)
	}
	// Keep a native-origin RFB connection alive throughout browser refreshes.
	// It deliberately stays outside the browser session registry.
	native := session.NewVNCSession("native-protocol-e2e")
	if err := native.Connect(session.ConnectionConfig{Host: "127.0.0.1", Port: 5909}); err != nil {
		t.Fatal(err)
	}
	defer native.Disconnect()
	nativeWS, _, err := websocket.DefaultDialer.Dial(native.ProxyAddr(), http.Header{"Origin": {"wails://localhost"}})
	if err != nil {
		t.Fatal(err)
	}
	defer nativeWS.Close()
	var buffered []byte
	readVNC := func(n int) []byte {
		t.Helper()
		for len(buffered) < n {
			nativeWS.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, data, err := nativeWS.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			buffered = append(buffered, data...)
		}
		data := buffered[:n]
		buffered = buffered[n:]
		return data
	}
	if string(readVNC(12)[:4]) != "RFB " {
		t.Fatal("no native RFB banner")
	}
	if err := nativeWS.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.003\n")); err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint32(readVNC(4)) != 1 {
		t.Fatal("fixture must use VNC security None")
	}
	nativeWS.WriteMessage(websocket.BinaryMessage, []byte{1})
	init := readVNC(24)
	readVNC(int(binary.BigEndian.Uint32(init[20:24])))
	t.Log("native-origin VNC proxy completed a real RFB handshake")
	a := &App{sessionManager: session.NewSessionManager()}
	defer a.sessionManager.CloseAll()
	mux := http.NewServeMux()
	a.registerWebRemoteDesktopRoutes(mux.Handle)
	done := make(chan struct{}, 1)
	mux.HandleFunc("/e2e/finish", func(w http.ResponseWriter, r *http.Request) {
		select {
		case done <- struct{}{}:
		default:
		}
	})
	mux.HandleFunc("/e2e/state", func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(a.sessionManager.List()) })
	mux.HandleFunc("/e2e/close/", func(w http.ResponseWriter, r *http.Request) {
		_ = a.CloseSession(strings.TrimPrefix(r.URL.Path, "/e2e/close/"))
	})
	mux.Handle("/", httputil.NewSingleHostReverseProxy(target))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("remote-e2e")
		if err != nil || cookie.Value != "test-only" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	defer server.Close()
	if err := os.WriteFile(os.Getenv("UNITERM_REMOTE_E2E_URL_FILE"), []byte(server.URL), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("protocol harness:", server.URL)
	select {
	case <-done:
	case <-time.After(12 * time.Minute):
		t.Fatal("protocol harness timed out")
	}
	if err := nativeWS.WriteMessage(websocket.BinaryMessage, []byte{3, 0, 0, 0, 0, 0, 0, 1, 0, 1}); err != nil {
		t.Fatal(err)
	}
	if readVNC(1)[0] != 0 {
		t.Fatal("native RFB did not survive browser cleanup")
	}
	t.Log("native-origin VNC proxy still receives framebuffer updates after browser cleanup")
}
