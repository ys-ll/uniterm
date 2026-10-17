package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ys-ll/uniterm/backend/session"
)

// registerWebRemoteDesktopRoutes must be passed the shared authenticated
// listener's Handle. Upgrades stay inside that handler (including Origin/auth).
func (a *App) registerWebRemoteDesktopRoutes(handle func(string, http.Handler)) {
	handle("/web/remote/", http.HandlerFunc(a.serveWebRemoteDesktop))
}

type webRemoteDesktopSession interface {
	session.Session
	EnableWeb(func()) error
	IsWeb() bool
	ServeWeb(http.ResponseWriter, *http.Request, bool)
}

func (a *App) serveWebRemoteDesktop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/web/remote/"), "/")
	if a.sessionManager == nil || len(parts) == 0 || (parts[0] != "vnc" && parts[0] != "spice") {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodPost {
		var config session.ConnectionConfig
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&config); err != nil {
			http.Error(w, "invalid remote desktop configuration", http.StatusBadRequest)
			return
		}
		info, err := a.CreateSession(parts[0], config)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s, ok := a.sessionManager.Get(info.ID)
		if !ok {
			http.Error(w, "session closed", http.StatusGone)
			return
		}
		remote := s.(webRemoteDesktopSession)
		if err := remote.EnableWeb(func() { _ = a.CloseSession(info.ID) }); err != nil {
			_ = a.CloseSession(info.ID)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if r.Context().Err() != nil {
			_ = a.CloseSession(info.ID)
			return
		}
		info.ProxyAddr = "/web/remote/" + parts[0] + "/" + info.ID + "/main"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if json.NewEncoder(w).Encode(info) != nil {
			_ = a.CloseSession(info.ID)
		}
		return
	}
	if len(parts) != 3 || r.Method != http.MethodGet || (parts[2] != "main" && !(parts[0] == "spice" && parts[2] == "channel")) {
		http.NotFound(w, r)
		return
	}
	s, ok := a.sessionManager.Get(parts[1])
	if !ok {
		http.NotFound(w, r)
		return
	}
	remote, ok := s.(webRemoteDesktopSession)
	if !ok || s.Type() != parts[0] || !remote.IsWeb() || !s.IsConnected() {
		http.NotFound(w, r)
		return
	}
	remote.ServeWeb(w, r, parts[2] == "main")
}

// Call when Web access stops/revokes authentication. net/http Shutdown does not
// close hijacked WebSockets. Application shutdown already closes all sessions.
func (a *App) closeWebRemoteDesktop() {
	if a.sessionManager == nil {
		return
	}
	for _, info := range a.sessionManager.List() {
		s, ok := a.sessionManager.Get(info.ID)
		if remote, yes := s.(webRemoteDesktopSession); ok && yes && remote.IsWeb() {
			_ = a.CloseSession(info.ID)
		}
	}
}
