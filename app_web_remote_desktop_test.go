package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
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
