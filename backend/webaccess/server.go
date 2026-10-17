// Package webaccess serves the desktop instance's existing Wails handler.
package webaccess

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const DefaultPort = 34116

type Config struct {
	Enabled   bool   `json:"enabled"`
	Host      string `json:"host"`
	StartPort int    `json:"startPort"`
}

type Status struct {
	Config     Config   `json:"config"`
	Running    bool     `json:"running"`
	ActualPort int      `json:"actualPort"`
	URLs       []string `json:"urls"`
	TokenSet   bool     `json:"tokenSet"`
	InstanceID string   `json:"instanceId"`
	Error      string   `json:"error"`
}

type settings struct {
	Config   Config `json:"config"`
	Salt     []byte `json:"salt,omitempty"`
	Verifier []byte `json:"verifier,omitempty"`
}

type contextKey int

const (
	browserKey contextKey = iota
	connectionKey
)

// IsBrowser identifies calls arriving through the authenticated external listener.
func IsBrowser(ctx context.Context) bool { return ctx != nil && ctx.Value(browserKey) == true }

type Server struct {
	mu           sync.Mutex
	settings     settings
	path         string
	instanceID   string
	cookieName   string
	lastError    string
	closed       bool
	assets       http.Handler
	mux          *http.ServeMux
	server       *http.Server
	listener     net.Listener
	allowedHosts map[string]bool
	cookies      map[[32]byte]time.Time
	generation   context.Context
	revoke       context.CancelFunc
	hijacked     map[net.Conn]bool
	loginSlots   chan struct{}
	Events       *Broker
}

func New() *Server {
	id := GenerateToken()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{settings: settings{Config: Config{Host: "127.0.0.1", StartPort: DefaultPort}},
		instanceID: id, cookieName: "uniterm_web_" + id[:16], mux: http.NewServeMux(),
		cookies: make(map[[32]byte]time.Time), generation: ctx, revoke: cancel,
		hijacked: make(map[net.Conn]bool), loginSlots: make(chan struct{}, 2), Events: NewBroker()}
	s.mux.HandleFunc("/web/bootstrap", s.bootstrap)
	s.mux.HandleFunc("/web/events", s.Events.ServeHTTP)
	s.mux.HandleFunc("/web/logout", s.logout)
	s.mux.HandleFunc("/", s.serveAssets)
	return s
}

func GenerateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Server) SetAssets(h http.Handler) { s.mu.Lock(); s.assets = h; s.mu.Unlock() }

// Handle registers a feature route behind the same authentication/origin gate.
// net/http's mux supports concurrent registration, including after startup.
func (s *Server) Handle(pattern string, h http.Handler) { s.mux.Handle(pattern, h) }

// Load runs after the App's stores are ready, rooted in the active data directory.
func (s *Server) Load(dataDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("web access is shut down")
	}
	s.path = filepath.Join(dataDir, "web_access.json")
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return s.failLocked(err)
	}
	cfg := s.settings
	if err = json.Unmarshal(b, &cfg); err != nil {
		return s.failLocked(fmt.Errorf("read web access settings: %w", err))
	}
	if err = validate(cfg); err != nil {
		return s.failLocked(err)
	}
	s.settings = cfg
	return s.startLocked()
}

func validate(cfg settings) error {
	if cfg.Config.Host != "127.0.0.1" && cfg.Config.Host != "0.0.0.0" {
		return errors.New("web host must be 127.0.0.1 or 0.0.0.0")
	}
	if cfg.Config.StartPort < 1 || cfg.Config.StartPort > 65535 {
		return errors.New("web start port must be between 1 and 65535")
	}
	if len(cfg.Verifier) != 0 && (len(cfg.Verifier) != 32 || len(cfg.Salt) != 32) {
		return errors.New("invalid web token verifier")
	}
	if cfg.Config.Enabled && cfg.Config.Host == "0.0.0.0" && len(cfg.Verifier) == 0 {
		return errors.New("LAN web access requires a token")
	}
	return nil
}

func tokenHash(token string, salt []byte) []byte {
	return pbkdf2.Key([]byte(token), salt, 120000, 32, sha256.New)
}

func (s *Server) Configure(config Config, token string) error {
	if len(token) > 4096 {
		return errors.New("web token is too long")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("web access is shut down")
	}
	next := s.settings
	next.Config = config
	if token != "" {
		next.Salt = make([]byte, 32)
		if _, err := rand.Read(next.Salt); err != nil {
			return s.failLocked(err)
		}
		next.Verifier = tokenHash(token, next.Salt)
	}
	if err := validate(next); err != nil {
		return s.failLocked(err)
	}
	if s.path == "" {
		return s.failLocked(errors.New("web access settings are not ready"))
	}
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return s.failLocked(err)
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".web_access-*")
	if err != nil {
		return s.failLocked(err)
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err == nil {
		err = os.Rename(f.Name(), s.path)
	}
	if err != nil {
		return s.failLocked(err)
	}
	changed := s.settings.Config != next.Config
	if changed {
		s.stopLocked()
	} else if token != "" {
		s.revokeLocked()
	}
	s.settings = next
	s.lastError = ""
	return s.startLocked()
}

func (s *Server) failLocked(err error) error { s.lastError = err.Error(); return err }

// listenFrom retains the successful listener; only EADDRINUSE advances the port.
func listenFrom(host string, port int, listen func(string, string) (net.Listener, error)) (net.Listener, error) {
	for p := port; p <= 65535; p++ {
		l, err := listen("tcp4", net.JoinHostPort(host, strconv.Itoa(p)))
		if err == nil {
			return l, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, fmt.Errorf("listen on %s:%d: %w", host, p, err)
		}
	}
	return nil, errors.New("no available web port before 65535")
}

func (s *Server) startLocked() error {
	if !s.settings.Config.Enabled || s.server != nil {
		return nil
	}
	if s.assets == nil {
		return s.failLocked(errors.New("Wails assets are not ready"))
	}
	l, err := listenFrom(s.settings.Config.Host, s.settings.Config.StartPort, net.Listen)
	if err != nil {
		return s.failLocked(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	s.allowedHosts = map[string]bool{net.JoinHostPort("127.0.0.1", strconv.Itoa(port)): true, net.JoinHostPort("localhost", strconv.Itoa(port)): true}
	if s.settings.Config.Host == "0.0.0.0" {
		addrs, _ := net.InterfaceAddrs()
		for _, addr := range addrs {
			if ip, _, err := net.ParseCIDR(addr.String()); err == nil && ip.To4() != nil {
				s.allowedHosts[net.JoinHostPort(ip.String(), strconv.Itoa(port))] = true
			}
		}
	}
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context { return context.WithValue(ctx, connectionKey, c) },
	}
	s.listener, s.server = l, srv
	go func() {
		err := srv.Serve(l)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.server == srv {
			s.server, s.listener = nil, nil
			s.revokeLocked()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.lastError = err.Error()
			}
		}
	}()
	return nil
}

func (s *Server) revokeLocked() {
	s.revoke()
	s.generation, s.revoke = context.WithCancel(context.Background())
	clear(s.cookies)
	s.Events.DisconnectAll()
	for conn := range s.hijacked {
		_ = conn.Close()
		delete(s.hijacked, conn)
	}
}

func (s *Server) stopLocked() {
	s.revokeLocked()
	if s.server != nil {
		srv := s.server
		_ = s.listener.Close() // release the port before reconfiguration binds it
		s.server, s.listener = nil, nil
		go func() {
			// Let the RPC that disabled/reconfigured access return its status.
			// Streams were revoked above and cannot hold shutdown open.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if srv.Shutdown(ctx) != nil {
				_ = srv.Close()
			}
		}()
	}
}

func (s *Server) Close() { s.mu.Lock(); defer s.mu.Unlock(); s.closed = true; s.stopLocked() }

func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Config: s.settings.Config, Running: s.listener != nil, TokenSet: len(s.settings.Verifier) > 0, InstanceID: s.instanceID, Error: s.lastError, URLs: []string{}}
	if s.listener != nil {
		st.ActualPort = s.listener.Addr().(*net.TCPAddr).Port
		st.URLs = append(st.URLs, "http://127.0.0.1:"+strconv.Itoa(st.ActualPort))
		for host := range s.allowedHosts {
			if !strings.HasPrefix(host, "127.") && !strings.HasPrefix(host, "localhost:") {
				st.URLs = append(st.URLs, "http://"+host)
			}
		}
	}
	return st
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	s.mu.Lock()
	validHost := s.allowedHosts[r.Host]
	running := s.listener != nil
	gen := s.generation
	// Loopback exemption is deliberate and only applies when no token is set.
	remote, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(remote)
	authed := s.settings.Config.Host == "127.0.0.1" && len(s.settings.Verifier) == 0 && ip != nil && ip.IsLoopback()
	if c, err := r.Cookie(s.cookieName); err == nil {
		expiry, ok := s.cookies[sha256.Sum256([]byte(c.Value))]
		authed = authed || (ok && time.Now().Before(expiry))
	}
	s.mu.Unlock()
	if !running {
		http.Error(w, "web access disabled", http.StatusServiceUnavailable)
		return
	}
	if !validHost || !sameOrigin(r) {
		http.Error(w, "invalid request origin", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), browserKey, true))
	conn, _ := r.Context().Value(connectionKey).(net.Conn)
	closeOnRevoke := r.URL.Path != "/wails/runtime" && r.URL.Path != "/web/logout"
	stop := context.AfterFunc(gen, func() {
		cancel()
		if conn != nil && closeOnRevoke {
			_ = conn.Close()
		}
	})
	defer stop()
	defer cancel()
	r = r.WithContext(ctx)
	if gen.Err() != nil {
		http.Error(w, "authentication revoked", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/web/login" {
		s.login(w, r, gen)
		return
	}
	if !authed {
		if r.Method == "GET" && (r.URL.Path == "/" || r.URL.Path == "/web.html") {
			http.Redirect(w, r, "/web/login", http.StatusSeeOther)
		} else {
			http.Error(w, "authentication required", http.StatusUnauthorized)
		}
		return
	}
	// Wails supports legacy query-string RPC. External callers use the normal
	// POST transport; never allow a navigation/image GET to perform an RPC.
	if r.URL.Path == "/wails/runtime" && r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	s.mux.ServeHTTP(&authenticatedWriter{ResponseWriter: w, server: s, generation: gen}, r)
}

// Upgraded feature connections outlive ServeHTTP and http.Server.Close. Track
// them until Close so rotation also revokes VNC/SPICE without leaking entries.
type authenticatedWriter struct {
	http.ResponseWriter
	server     *Server
	generation context.Context
}

func (w *authenticatedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *authenticatedWriter) Flush()                      { _ = http.NewResponseController(w.ResponseWriter).Flush() }
func (w *authenticatedWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err != nil {
		return nil, nil, err
	}
	w.server.mu.Lock()
	defer w.server.mu.Unlock()
	if w.generation.Err() != nil {
		c.Close()
		return nil, nil, errors.New("authentication revoked")
	}
	w.server.hijacked[c] = true
	return &authenticatedConn{Conn: c, server: w.server}, rw, nil
}

type authenticatedConn struct {
	net.Conn
	server *Server
}

func (c *authenticatedConn) Close() error {
	c.server.mu.Lock()
	delete(c.server.hijacked, c.Conn)
	c.server.mu.Unlock()
	return c.Conn.Close()
}

func sameOrigin(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	if site == "cross-site" || site == "same-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "http" || u.Host != r.Host || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return false
		}
	}
	// Even loopback mutations require an explicit same-origin browser signal.
	return r.Method == "GET" || r.Method == "HEAD" || origin != "" || site == "same-origin"
}

func (s *Server) login(w http.ResponseWriter, r *http.Request, gen context.Context) {
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; frame-ancestors 'none'")
		io.WriteString(w, `<!doctype html><html><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>uniTerm login</title><h1>uniTerm</h1><form method="post" action="/web/login"><label>Access token <input type="password" name="token" required autocomplete="current-password" maxlength="4096"></label><button>Sign in</button></form></html>`)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer rc.SetReadDeadline(time.Time{})
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var token string
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Token string `json:"token"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid login", 400)
			return
		}
		token = body.Token
	} else {
		if r.ParseForm() != nil {
			http.Error(w, "invalid login", 400)
			return
		}
		token = r.PostForm.Get("token")
	}
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
	default:
		http.Error(w, "try again shortly", 429)
		return
	}
	s.mu.Lock()
	salt, verifier := s.settings.Salt, s.settings.Verifier
	s.mu.Unlock()
	valid := token != "" && len(token) <= 4096 && len(verifier) == 32 && subtle.ConstantTimeCompare(tokenHash(token, salt), verifier) == 1
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid || gen.Err() != nil {
		http.Error(w, "invalid token", 401)
		return
	}
	for key, expiry := range s.cookies {
		if time.Now().After(expiry) {
			delete(s.cookies, key)
		}
	}
	if len(s.cookies) >= 64 {
		http.Error(w, "too many authenticated clients", 429)
		return
	}
	value := GenerateToken()
	s.cookies[sha256.Sum256([]byte(value))] = time.Now().Add(12 * time.Hour)
	http.SetCookie(w, &http.Cookie{Name: s.cookieName, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	s.mu.Lock()
	// One trusted operator: logging out revokes all of this instance's clients.
	s.revokeLocked()
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: s.cookieName, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET required", 405)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"instanceId": s.instanceID, "clientId": GenerateToken(), "ready": true, "eventsUrl": "/web/events", "capabilities": map[string]any{"terminal": true, "snapshot": "desktop-xterm", "responder": "desktop", "input": "shared", "sizeOwner": "last-active"}})
}

func (s *Server) serveAssets(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	h := s.assets
	s.mu.Unlock()
	if h == nil {
		http.Error(w, "assets not ready", 503)
		return
	}
	if r.URL.Path == "/" {
		r = r.Clone(r.Context())
		u := *r.URL
		u.Path = "/web.html"
		r.URL = &u
	}
	h.ServeHTTP(w, r)
}
