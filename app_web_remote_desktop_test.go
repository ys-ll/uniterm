package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/session"
)

func TestWebRemoteDesktopRoutes(t *testing.T) {
	a := &App{sessionManager: session.NewSessionManager()}
	defer a.sessionManager.CloseAll()
	mux := http.NewServeMux()
	// Contract03 supplies authentication *outside* every registered handler.
	a.registerWebRemoteDesktopRoutes(func(pattern string, handler http.Handler) {
		mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("test-auth")
			if err != nil || cookie.Value != "yes" {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, r)
		}))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	request := func(method, path, body string, auth bool) *http.Response {
		t.Helper()
		r, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if auth {
			r.AddCookie(&http.Cookie{Name: "test-auth", Value: "yes"})
		}
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	if response := request("POST", "/web/remote/vnc", `{}`, false); response.StatusCode != 401 {
		t.Fatal(response.Status)
	}
	if len(a.sessionManager.List()) != 0 {
		t.Fatal("unauthenticated creation")
	}
	response := request("POST", "/web/remote/spice", `{"host":"tcp://127.0.0.1:33925"}`, true)
	if response.StatusCode != 201 {
		t.Fatal(response.Status)
	}
	var info session.SessionInfo
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.ProxyAddr != "/web/remote/spice/"+info.ID+"/main" {
		t.Fatal(info.ProxyAddr)
	}
	_, unauth, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+info.ProxyAddr, http.Header{"Origin": {server.URL}})
	if err == nil || unauth.StatusCode != 401 {
		t.Fatal("unauthenticated upgrade")
	}
	unauth.Body.Close()
	native := session.NewVNCSession("native-session")
	if err := native.Connect(session.ConnectionConfig{Host: "127.0.0.1", Port: 5909}); err != nil {
		t.Fatal(err)
	}
	a.sessionManager.Add(native)
	for _, path := range []string{
		"/web/remote/vnc/" + info.ID + "/main", "/web/remote/vnc/missing/main", "/web/remote/spice/" + info.ID + "/other",
		"/web/remote/vnc/native-session/main", "/web/remote/ssh", "/web/remote/spice/" + info.ID + "/main/extra",
	} {
		if response := request("GET", path, "", true); response.StatusCode != 404 {
			t.Fatalf("%s: %s", path, response.Status)
		}
	}
	a.closeWebRemoteDesktop()
	if _, ok := a.sessionManager.Get(info.ID); ok {
		t.Fatal("browser session survived Web shutdown")
	}
	if !native.IsConnected() {
		t.Fatal("Web shutdown closed a native session")
	}
	if response := request("GET", info.ProxyAddr, "", true); response.StatusCode != 404 {
		t.Fatal("closed URL survived")
	}
}

func TestWebRemoteDesktopNewAppBrowserClose(t *testing.T) {
	transfer, cwd := session.TransferEventSink, session.TerminalCwdSink
	a := NewApp(t.TempDir())
	a.sessionManager = session.NewSessionManager()
	// Keep CreateSession's status/capture observers and the real Web transport.
	// Only native window delivery is replaced; no Wails GUI is started.
	events := make(chan *application.CustomEvent, 16)
	a.webTransport.native = func(event *application.CustomEvent) { events <- event }
	t.Cleanup(func() {
		a.webAccess.Close()
		a.sessionManager.CloseAll()
		session.TransferEventSink, session.TerminalCwdSink = transfer, cwd
	})
	base := enableWebTestListener(t, a)
	for _, kind := range []string{"vnc", "spice"} {
		t.Run(kind, func(t *testing.T) {
			upstream, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
			if err != nil {
				t.Fatal(err)
			}
			defer upstream.Close()
			config := session.ConnectionConfig{Host: "127.0.0.1", Port: upstream.Addr().(*net.TCPAddr).Port}
			if kind == "spice" {
				config.Host = "tcp://127.0.0.1"
			}
			body, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			request, err := http.NewRequest("POST", base+"/web/remote/"+kind, strings.NewReader(string(body)))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Origin", base)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusCreated {
				t.Fatal(response.Status)
			}
			var info session.SessionInfo
			if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
				t.Fatal(err)
			}
			paths := []string{info.ProxyAddr}
			if kind == "spice" {
				paths = append(paths, "/web/remote/spice/"+info.ID+"/channel")
			}
			var main *websocket.Conn
			var peers []net.Conn
			for i, path := range paths {
				ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(base, "http")+path, http.Header{"Origin": {base}})
				if err != nil {
					t.Fatal(err)
				}
				defer ws.Close()
				if i == 0 {
					main = ws
				}
				upstream.SetDeadline(time.Now().Add(2 * time.Second))
				conn, err := upstream.Accept()
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				peers = append(peers, conn)
				conn.SetDeadline(time.Now().Add(2 * time.Second))
				// Byte-level fixture: exercise socket lifetime, not protocol decoding.
				if _, err := io.WriteString(conn, "remote desktop bytes"); err != nil {
					t.Fatal(err)
				}
				ws.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, data, err := ws.ReadMessage()
				if err != nil || string(data) != "remote desktop bytes" {
					t.Fatalf("upstream data %q: %v", data, err)
				}
			}
			// Closing only main must also reclaim the open SPICE sibling.
			main.Close()
			for _, conn := range peers {
				conn.SetReadDeadline(time.Now().Add(2 * time.Second))
				if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
					t.Fatalf("browser close did not close upstream: %v", err)
				}
			}
			for _, want := range []struct{ name, status string }{
				{"session:status", "connected"},
				{"session:status", "disconnected"},
				{"session:closed", ""},
			} {
				select {
				case event := <-events:
					data, ok := event.Data.(map[string]any)
					if !ok || event.Name != want.name || data["id"] != info.ID || (want.status != "" && data["status"] != want.status) {
						t.Fatalf("want %s/%s for %s, got %+v", want.name, want.status, info.ID, event)
					}
				case <-time.After(2 * time.Second):
					t.Fatalf("browser-close observer did not deliver %s/%s", want.name, want.status)
				}
			}
			if sessions := a.ListSessions(); len(sessions) != 0 {
				t.Fatalf("browser close left registered sessions: %+v", sessions)
			}
			a.captureMu.Lock()
			_, captured := a.captures[info.ID]
			a.captureMu.Unlock()
			if captured {
				t.Fatal("browser close left the App session capture registered")
			}
			// Manager removal precedes Disconnect. Check the proxy callbacks,
			// copy loops, heartbeat and cancellation goroutines have also exited.
			stack := make([]byte, 1<<20)
			deadline := time.Now().Add(2 * time.Second)
			for {
				stacks := string(stack[:runtime.Stack(stack, true)])
				if !strings.Contains(stacks, "(*remoteDesktopProxy).") {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("remote desktop goroutines survived browser close:\n%s", stacks)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}
