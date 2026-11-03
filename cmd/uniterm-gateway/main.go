package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// UserAccount represents an isolated user account in the uniTerm Web Gateway.
type UserAccount struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"passwordHash"` // hex(salt) + ":" + hex(iterated-hmac-sha256)
	Role         string    `json:"role"`         // "admin" | "user"
	CreatedAt    time.Time `json:"createdAt"`
	Disabled     bool      `json:"disabled,omitempty"`
}

// SessionClaims represents the signed token payload stored in the HttpOnly session cookie.
type SessionClaims struct {
	Username string `json:"u"`
	Role     string `json:"r"`
	Exp      int64  `json:"exp"`
}

// WorkerInstance tracks a per-user headless `uniterm-server` worker process.
type WorkerInstance struct {
	Username   string
	Port       int
	TargetURL  *url.URL
	Proxy      *httputil.ReverseProxy
	Cmd        *exec.Cmd
	LastActive time.Time
	done       chan struct{} // closed once the process has exited and been reaped
	waitErr    error
	mu         sync.Mutex

	// ready is closed once the HTTP port is listening, or spawning failed
	// (readyErr then explains why). Concurrent first-screen requests wait on
	// this instead of being proxied to a port that is not accepting yet.
	ready     chan struct{}
	readyOnce sync.Once
	readyErr  error
}

// MultiUserGateway manages user authentication, isolated per-user directories,
// and on-demand headless `uniterm-server` worker processes.
type MultiUserGateway struct {
	dataRoot          string
	usersFile         string
	workerBinary      string
	jwtSecret         []byte
	workerIdleTimeout time.Duration
	baseWorkerPort    int

	usersMu sync.RWMutex
	users   map[string]UserAccount

	workersMu sync.Mutex
	workers   map[string]*WorkerInstance
	usedPorts map[int]bool
}

var validUsernameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{1,31}$`)

const sessionCookieName = "uniterm_web_session"

func main() {
	dataRoot := envOrDefault("UNITERM_DATA_DIR", "/data")
	listenAddr := envOrDefault("UNITERM_GATEWAY_ADDR", "0.0.0.0:8080")
	workerBin := envOrDefault("UNITERM_SERVER_BIN", "/app/uniterm-server")
	idleMinutes, _ := strconv.Atoi(envOrDefault("UNITERM_WORKER_IDLE_MINUTES", "60"))
	if idleMinutes <= 0 {
		idleMinutes = 60
	}

	gw, err := NewMultiUserGateway(dataRoot, workerBin, time.Duration(idleMinutes)*time.Minute)
	if err != nil {
		log.Fatalf("[uniterm-gateway] initialization failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go gw.reapIdleWorkersLoop(ctx)

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           gw,
		ReadHeaderTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("[uniterm-gateway] Multi-user Web UI listening on http://%s (dataRoot=%s)", listenAddr, dataRoot)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[uniterm-gateway] HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[uniterm-gateway] Shutting down gateway and stopping user workers...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	gw.StopAllWorkers()
}

// NewMultiUserGateway initializes the gateway, loads or bootstraps user accounts, and prepares directories.
func NewMultiUserGateway(dataRoot, workerBinary string, idleTimeout time.Duration) (*MultiUserGateway, error) {
	gatewayDir := filepath.Join(dataRoot, "gateway")
	usersRoot := filepath.Join(dataRoot, "users")
	if err := os.MkdirAll(gatewayDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(usersRoot, 0o755); err != nil {
		return nil, err
	}

	secretFile := filepath.Join(gatewayDir, "jwt_secret.key")
	secret, err := loadOrCreateSecret(secretFile)
	if err != nil {
		return nil, err
	}

	gw := &MultiUserGateway{
		dataRoot:          dataRoot,
		usersFile:         filepath.Join(gatewayDir, "users.json"),
		workerBinary:      workerBinary,
		jwtSecret:         secret,
		workerIdleTimeout: idleTimeout,
		baseWorkerPort:    19000,
		users:             make(map[string]UserAccount),
		workers:           make(map[string]*WorkerInstance),
		usedPorts:         make(map[int]bool),
	}

	if err := gw.loadOrBootstrapUsers(); err != nil {
		return nil, err
	}
	return gw, nil
}

func (gw *MultiUserGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Public health check for Docker / Kubernetes probes
	if r.URL.Path == "/health" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","component":"uniterm-multiuser-gateway"}`))
		return
	}

	// 2. Authentication & User Management routes
	switch {
	case r.URL.Path == "/login" && r.Method == http.MethodGet:
		gw.serveLoginPage(w, r)
		return
	case r.URL.Path == "/api/auth/login" && r.Method == http.MethodPost:
		gw.handleLogin(w, r)
		return
	case r.URL.Path == "/api/auth/logout" && (r.Method == http.MethodPost || r.Method == http.MethodGet):
		gw.handleLogout(w, r)
		return
	}

	// 3. Verify authenticated user session
	claims, err := gw.authenticateRequest(r)
	if err != nil {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/wails/") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// 4. Authenticated user/admin APIs
	switch {
	case r.URL.Path == "/api/auth/me" && r.Method == http.MethodGet:
		writeGatewayJSON(w, http.StatusOK, map[string]any{
			"username": claims.Username,
			"role":     claims.Role,
		})
		return
	case r.URL.Path == "/api/auth/password" && r.Method == http.MethodPost:
		gw.handleChangePassword(w, r, claims)
		return
	case r.URL.Path == "/api/auth/users" && r.Method == http.MethodGet:
		gw.handleListUsers(w, r, claims)
		return
	case r.URL.Path == "/api/auth/users" && r.Method == http.MethodPost:
		gw.handleCreateUser(w, r, claims)
		return
	case strings.HasPrefix(r.URL.Path, "/api/auth/users/") && r.Method == http.MethodDelete:
		gw.handleDeleteUser(w, r, claims)
		return
	}

	// 5. Route to the user's dedicated isolated uniterm-server worker instance
	worker, err := gw.getOrSpawnWorker(claims.Username)
	if err != nil {
		log.Printf("[uniterm-gateway] failed to spawn worker for %s: %v", claims.Username, err)
		http.Error(w, fmt.Sprintf("Failed to start isolated workspace for user %s: %v", claims.Username, err), http.StatusBadGateway)
		return
	}

	worker.Touch()

	// Inject verified identity headers for downstream WebStreamService
	r.Header.Set("X-Uniterm-User", claims.Username)
	r.Header.Set("X-Uniterm-Role", claims.Role)

	compressibleAssets(worker.Proxy).ServeHTTP(w, r)
}

// --- Static asset compression ---

// compressibleAssets gzips the frontend's text assets on the gateway hop. The
// worker serves them uncompressed (4.45 MB of JS/CSS in the current build);
// compressing here avoids touching the frontend build and keeps desktop
// builds byte-identical. Binary assets (woff2/png/wasm) are already
// compressed, so they pass through untouched.
func compressibleAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isCompressibleAsset(r.URL.Path) ||
			r.Method == http.MethodHead ||
			r.Header.Get("Range") != "" ||
			!acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		writer := &gzipResponseWriter{ResponseWriter: w}
		defer writer.Close()
		next.ServeHTTP(writer, r)
	})
}

func isCompressibleAsset(p string) bool {
	if !strings.HasPrefix(p, "/assets/") {
		return false
	}
	switch strings.ToLower(path.Ext(p)) {
	case ".js", ".mjs", ".css", ".json", ".svg", ".map", ".txt", ".html":
		return true
	}
	return false
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), "gzip") {
			continue
		}
		for _, param := range fields[1:] {
			if strings.EqualFold(strings.TrimSpace(param), "q=0") {
				return false
			}
		}
		return true
	}
	return false
}

// gzipResponseWriter compresses only 200 responses that are not already
// encoded (or a partial range); everything else passes through unchanged.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer
	decided bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.decided {
		return
	}
	w.decided = true
	header := w.Header()
	if status == http.StatusOK && header.Get("Content-Encoding") == "" && header.Get("Content-Range") == "" {
		// BestSpeed keeps the 3.7 MB main chunk around 45 ms of CPU while
		// still cutting the transfer to roughly a third.
		if gz, err := gzip.NewWriterLevel(w.ResponseWriter, gzip.BestSpeed); err == nil {
			w.gz = gz
			header.Del("Content-Length")
			header.Set("Content-Encoding", "gzip")
			header.Add("Vary", "Accept-Encoding")
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.decided {
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *gzipResponseWriter) Close() error {
	if w.gz != nil {
		return w.gz.Close()
	}
	return nil
}

func (w *gzipResponseWriter) Flush() {
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// errWorkerExited reports that a worker died before it became ready, so the
// caller should drop that exact instance and spawn a fresh one.
var errWorkerExited = errors.New("worker exited during startup")

// getOrSpawnWorker ensures a dedicated headless uniterm-server process is running for username.
func (gw *MultiUserGateway) getOrSpawnWorker(username string) (*WorkerInstance, error) {
	// Two attempts: the first instance of a user can die right after spawn
	// (e.g. an immediate crash), in which case waiters drop it and retry once.
	for attempt := 0; attempt < 2; attempt++ {
		gw.workersMu.Lock()
		if existing, ok := gw.workers[username]; ok {
			select {
			case <-existing.done:
				// Exited: drop it and fall through to a fresh spawn.
				delete(gw.usedPorts, existing.Port)
				delete(gw.workers, username)
			default:
				gw.workersMu.Unlock()
				instance, err := existing.waitReady()
				if err == nil {
					return instance, nil
				}
				if !errors.Is(err, errWorkerExited) {
					return nil, err
				}
				// The process died while requests were queued on it; remove
				// this exact instance and spawn again.
				gw.dropWorker(existing)
				continue
			}
		}

		port, err := gw.allocatePortLocked()
		if err != nil {
			gw.workersMu.Unlock()
			return nil, err
		}

		userRoot := filepath.Join(gw.dataRoot, "users", username)
		userHome := filepath.Join(userRoot, "home")
		userConfig := filepath.Join(userRoot, "config")
		userData := filepath.Join(userRoot, "data")
		userCache := filepath.Join(userRoot, "cache")
		for _, dir := range []string{userHome, userConfig, userData, userCache} {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				delete(gw.usedPorts, port)
				gw.workersMu.Unlock()
				return nil, err
			}
		}

		targetURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
		cmd := exec.Command(gw.workerBinary)
		cmd.Dir = userHome
		cmd.Env = append(os.Environ(),
			"HOME="+userHome,
			"USER="+username,
			"LOGNAME="+username,
			"XDG_CONFIG_HOME="+userConfig,
			"XDG_DATA_HOME="+userData,
			"XDG_CACHE_HOME="+userCache,
			"WAILS_SERVER_HOST=127.0.0.1",
			"WAILS_SERVER_PORT="+strconv.Itoa(port),
			"UNITERM_TENANT_USER="+username,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			delete(gw.usedPorts, port)
			gw.workersMu.Unlock()
			return nil, fmt.Errorf("start %s failed: %w", gw.workerBinary, err)
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.FlushInterval = -1 // Flush immediately for SSE, WebSocket, and streaming SFTP downloads
		origDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			origDirector(req)
			// Rewrite Host to 127.0.0.1:<port> so Wails v3 Loopback Host Check passes cleanly
			req.Host = targetURL.Host
			if req.Header.Get("Origin") != "" {
				req.Header.Set("Origin", "http://"+targetURL.Host)
			}
		}

		instance := &WorkerInstance{
			Username:   username,
			Port:       port,
			TargetURL:  targetURL,
			Proxy:      proxy,
			Cmd:        cmd,
			LastActive: time.Now(),
			done:       make(chan struct{}),
			ready:      make(chan struct{}),
		}
		// Register before the port is listening: concurrent first-screen
		// requests then wait on ready instead of getting a connection refusal.
		gw.workers[username] = instance
		gw.workersMu.Unlock()

		// Reap the child so a dead worker is detectable instead of leaving a
		// permanent zombie that proxied every request to a closed port.
		go func() {
			instance.waitErr = cmd.Wait()
			close(instance.done)
			log.Printf("[uniterm-gateway] Worker for user=%s exited: %v", username, instance.waitErr)
		}()

		started := time.Now()
		if err := waitForWorkerReady(port, 10*time.Second); err != nil {
			instance.readyErr = err
			instance.readyOnce.Do(func() { close(instance.ready) })
			gw.dropWorker(instance)
			terminateWorkerInstance(instance)
			return nil, err
		}
		instance.readyOnce.Do(func() { close(instance.ready) })

		log.Printf("[uniterm-gateway] Started isolated worker for user=%s on 127.0.0.1:%d in %s (HOME=%s)",
			username, port, time.Since(started), userHome)
		return instance, nil
	}
	return nil, fmt.Errorf("worker for user %s could not be started", username)
}

// waitReady blocks until the worker is accepting connections, failed to
// start, or exited.
func (w *WorkerInstance) waitReady() (*WorkerInstance, error) {
	select {
	case <-w.ready:
		if w.readyErr != nil {
			return nil, w.readyErr
		}
		return w, nil
	case <-w.done:
		return nil, errWorkerExited
	}
}

// dropWorker removes instance from the registry only when it is still the
// user's current worker — a stale waiter must not evict a newer instance.
func (gw *MultiUserGateway) dropWorker(instance *WorkerInstance) {
	gw.workersMu.Lock()
	if current, ok := gw.workers[instance.Username]; ok && current == instance {
		delete(gw.workers, instance.Username)
		delete(gw.usedPorts, instance.Port)
	}
	gw.workersMu.Unlock()
}

func terminateWorkerInstance(instance *WorkerInstance) {
	if instance.Cmd == nil || instance.Cmd.Process == nil {
		return
	}
	_ = instance.Cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-instance.done:
	case <-time.After(3 * time.Second):
		_ = instance.Cmd.Process.Kill()
		<-instance.done
	}
}

func waitForWorkerReady(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 300 * time.Millisecond}

	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("worker on port %d did not become healthy within %s", port, timeout)
}

func (gw *MultiUserGateway) allocatePortLocked() (int, error) {
	for port := gw.baseWorkerPort; port < gw.baseWorkerPort+2000; port++ {
		if gw.usedPorts[port] {
			continue
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = ln.Close()
			gw.usedPorts[port] = true
			return port, nil
		}
	}
	return 0, errors.New("no free loopback port available for worker")
}

func (w *WorkerInstance) Touch() {
	w.mu.Lock()
	w.LastActive = time.Now()
	w.mu.Unlock()
}

func (gw *MultiUserGateway) stopWorker(username string) {
	gw.workersMu.Lock()
	instance, ok := gw.workers[username]
	if ok {
		delete(gw.workers, username)
		delete(gw.usedPorts, instance.Port)
	}
	gw.workersMu.Unlock()

	if ok {
		terminateWorkerInstance(instance)
	}
}

func (gw *MultiUserGateway) StopAllWorkers() {
	gw.workersMu.Lock()
	names := make([]string, 0, len(gw.workers))
	for name := range gw.workers {
		names = append(names, name)
	}
	gw.workersMu.Unlock()

	for _, name := range names {
		gw.stopWorker(name)
	}
}

func (gw *MultiUserGateway) reapIdleWorkersLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			var stale []string
			gw.workersMu.Lock()
			for username, inst := range gw.workers {
				inst.mu.Lock()
				idle := now.Sub(inst.LastActive)
				inst.mu.Unlock()
				if idle > gw.workerIdleTimeout {
					stale = append(stale, username)
				}
			}
			gw.workersMu.Unlock()

			for _, username := range stale {
				log.Printf("[uniterm-gateway] Stopping idle worker for user=%s", username)
				gw.stopWorker(username)
			}
		}
	}
}

// --- User Store & Cryptography ---

func (gw *MultiUserGateway) loadOrBootstrapUsers() error {
	gw.usersMu.Lock()
	defer gw.usersMu.Unlock()

	if data, err := os.ReadFile(gw.usersFile); err == nil {
		var list []UserAccount
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		for _, u := range list {
			gw.users[u.Username] = u
		}
		if len(gw.users) > 0 {
			return nil
		}
	}

	adminUser := envOrDefault("UNITERM_ADMIN_USER", "admin")
	adminPass := os.Getenv("UNITERM_ADMIN_PASSWORD")
	if adminPass == "" {
		var b [9]byte
		_, _ = rand.Read(b[:])
		adminPass = base64.RawURLEncoding.EncodeToString(b[:])
		log.Printf("[uniterm-gateway] ========================================================")
		log.Printf("[uniterm-gateway] Bootstrapped initial admin account: %s", adminUser)
		log.Printf("[uniterm-gateway] Initial generated password:         %s", adminPass)
		log.Printf("[uniterm-gateway] ========================================================")
	}

	gw.users[adminUser] = UserAccount{
		Username:     adminUser,
		PasswordHash: hashPassword(adminPass),
		Role:         "admin",
		CreatedAt:    time.Now().UTC(),
	}
	return gw.saveUsersLocked()
}

func (gw *MultiUserGateway) saveUsersLocked() error {
	list := make([]UserAccount, 0, len(gw.users))
	for _, u := range gw.users {
		list = append(list, u)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(gw.usersFile, data, 0o600)
}

func hashPassword(password string) string {
	var salt [16]byte
	_, _ = rand.Read(salt[:])
	derived := deriveKey(password, salt[:], 100000)
	return hex.EncodeToString(salt[:]) + ":" + hex.EncodeToString(derived)
}

func verifyPassword(password, stored string) bool {
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err1 := hex.DecodeString(parts[0])
	expected, err2 := hex.DecodeString(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	actual := deriveKey(password, salt, 100000)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// deriveKey performs iterated HMAC-SHA256 key stretching using pure Go standard library.
func deriveKey(password string, salt []byte, iterations int) []byte {
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(salt)
	digest := mac.Sum(nil)
	for i := 1; i < iterations; i++ {
		mac.Reset()
		mac.Write(digest)
		digest = mac.Sum(digest[:0])
	}
	return digest
}

func (gw *MultiUserGateway) signToken(claims SessionClaims) string {
	payloadBytes, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, gw.jwtSecret)
	mac.Write([]byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadB64 + "." + sigB64
}

func (gw *MultiUserGateway) verifyToken(token string) (*SessionClaims, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed token")
	}
	mac := hmac.New(sha256.New, gw.jwtSecret)
	mac.Write([]byte(parts[0]))
	expectedSig := mac.Sum(nil)
	actualSig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || subtle.ConstantTimeCompare(expectedSig, actualSig) != 1 {
		return nil, errors.New("invalid signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var claims SessionClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, err
	}
	if time.Now().Unix() > claims.Exp {
		return nil, errors.New("session expired")
	}
	return &claims, nil
}

func (gw *MultiUserGateway) authenticateRequest(r *http.Request) (*SessionClaims, error) {
	var rawToken string
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		rawToken = c.Value
	} else if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		rawToken = strings.TrimPrefix(auth, "Bearer ")
	}
	if rawToken == "" {
		return nil, errors.New("missing session token")
	}
	claims, err := gw.verifyToken(rawToken)
	if err != nil {
		return nil, err
	}
	gw.usersMu.RLock()
	user, exists := gw.users[claims.Username]
	gw.usersMu.RUnlock()
	if !exists || user.Disabled {
		return nil, errors.New("user disabled or removed")
	}
	return claims, nil
}

// --- Auth & Admin HTTP Handlers ---

func (gw *MultiUserGateway) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGatewayJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)

	gw.usersMu.RLock()
	user, ok := gw.users[req.Username]
	gw.usersMu.RUnlock()

	if !ok || user.Disabled || !verifyPassword(req.Password, user.PasswordHash) {
		writeGatewayJSON(w, http.StatusUnauthorized, map[string]any{"error": "用户名或密码错误"})
		return
	}

	token := gw.signToken(SessionClaims{
		Username: user.Username,
		Role:     user.Role,
		Exp:      time.Now().Add(24 * time.Hour).Unix(),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	writeGatewayJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"username": user.Username,
		"role":     user.Role,
	})

	// Pre-warm the isolated worker so the process fork and server
	// initialisation overlap with the browser loading the SPA.
	go func(username string) {
		if _, err := gw.getOrSpawnWorker(username); err != nil {
			log.Printf("[uniterm-gateway] prewarm worker for user=%s failed: %v", username, err)
		}
	}(user.Username)
}

func (gw *MultiUserGateway) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	if r.Method == http.MethodGet {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	writeGatewayJSON(w, http.StatusOK, map[string]any{"status": "logged_out"})
}

func (gw *MultiUserGateway) handleChangePassword(w http.ResponseWriter, r *http.Request, claims *SessionClaims) {
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.NewPassword) < 6 {
		writeGatewayJSON(w, http.StatusBadRequest, map[string]any{"error": "新密码长度至少为 6 位"})
		return
	}

	gw.usersMu.Lock()
	defer gw.usersMu.Unlock()
	user, ok := gw.users[claims.Username]
	if !ok || !verifyPassword(req.CurrentPassword, user.PasswordHash) {
		writeGatewayJSON(w, http.StatusUnauthorized, map[string]any{"error": "原密码校验失败"})
		return
	}
	user.PasswordHash = hashPassword(req.NewPassword)
	gw.users[claims.Username] = user
	_ = gw.saveUsersLocked()
	writeGatewayJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (gw *MultiUserGateway) handleListUsers(w http.ResponseWriter, r *http.Request, claims *SessionClaims) {
	if claims.Role != "admin" {
		writeGatewayJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required"})
		return
	}
	gw.usersMu.RLock()
	defer gw.usersMu.RUnlock()
	var list []map[string]any
	for _, u := range gw.users {
		list = append(list, map[string]any{
			"username":  u.Username,
			"role":      u.Role,
			"createdAt": u.CreatedAt,
			"disabled":  u.Disabled,
		})
	}
	writeGatewayJSON(w, http.StatusOK, map[string]any{"users": list})
}

func (gw *MultiUserGateway) handleCreateUser(w http.ResponseWriter, r *http.Request, claims *SessionClaims) {
	if claims.Role != "admin" {
		writeGatewayJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGatewayJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if !validUsernameRe.MatchString(req.Username) {
		writeGatewayJSON(w, http.StatusBadRequest, map[string]any{"error": "用户名仅允许字母、数字、下划线、点或连字符（2-32位）"})
		return
	}
	if len(req.Password) < 6 {
		writeGatewayJSON(w, http.StatusBadRequest, map[string]any{"error": "密码长度至少为 6 位"})
		return
	}
	if req.Role != "admin" {
		req.Role = "user"
	}

	gw.usersMu.Lock()
	defer gw.usersMu.Unlock()
	if _, exists := gw.users[req.Username]; exists {
		writeGatewayJSON(w, http.StatusConflict, map[string]any{"error": "用户已存在"})
		return
	}
	gw.users[req.Username] = UserAccount{
		Username:     req.Username,
		PasswordHash: hashPassword(req.Password),
		Role:         req.Role,
		CreatedAt:    time.Now().UTC(),
	}
	if err := gw.saveUsersLocked(); err != nil {
		writeGatewayJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeGatewayJSON(w, http.StatusCreated, map[string]any{"status": "created", "username": req.Username})
}

func (gw *MultiUserGateway) handleDeleteUser(w http.ResponseWriter, r *http.Request, claims *SessionClaims) {
	if claims.Role != "admin" {
		writeGatewayJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required"})
		return
	}
	target := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
	if target == "" || target == claims.Username {
		writeGatewayJSON(w, http.StatusBadRequest, map[string]any{"error": "不能删除当前登录的管理员账号"})
		return
	}
	gw.stopWorker(target)

	gw.usersMu.Lock()
	delete(gw.users, target)
	_ = gw.saveUsersLocked()
	gw.usersMu.Unlock()

	writeGatewayJSON(w, http.StatusOK, map[string]any{"status": "deleted", "username": target})
}

func (gw *MultiUserGateway) serveLoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(loginPageHTML))
}

func loadOrCreateSecret(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		return data, nil
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, err
	}
	return secret, nil
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func writeGatewayJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

const loginPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<title>uniTerm Web — 登录</title>
<style>
  :root {
    --bg: #0f141c;
    --card: #171f2c;
    --border: #263246;
    --text: #e5eaf3;
    --muted: #8b9bb4;
    --primary: #3b82f6;
    --primary-hover: #2563eb;
    --danger: #ef4444;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: radial-gradient(circle at top, #1a2536 0%, var(--bg) 70%);
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    color: var(--text);
  }
  .login-card {
    width: 100%;
    max-width: 390px;
    padding: 32px;
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: 14px;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.45);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 8px;
  }
  .brand-icon {
    width: 36px;
    height: 36px;
    border-radius: 8px;
    background: linear-gradient(135deg, #3b82f6, #1d4ed8);
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-family: monospace;
    font-size: 18px;
  }
  h1 { font-size: 20px; margin: 0; }
  p.sub { color: var(--muted); font-size: 13px; margin: 0 0 24px 0; }
  label { display: block; font-size: 13px; color: var(--muted); margin-bottom: 6px; }
  input {
    width: 100%;
    padding: 10px 12px;
    margin-bottom: 16px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: #0c1118;
    color: var(--text);
    font-size: 14px;
    outline: none;
  }
  input:focus { border-color: var(--primary); }
  button {
    width: 100%;
    padding: 11px;
    border: none;
    border-radius: 8px;
    background: var(--primary);
    color: #fff;
    font-weight: 600;
    font-size: 14px;
    cursor: pointer;
    transition: background 0.15s;
  }
  button:hover { background: var(--primary-hover); }
  .err {
    color: var(--danger);
    font-size: 13px;
    margin-bottom: 14px;
    display: none;
  }
</style>
</head>
<body>
  <form class="login-card" id="loginForm">
    <div class="brand">
      <div class="brand-icon">&gt;_</div>
      <h1>uniTerm Web</h1>
    </div>
    <p class="sub">多协议终端与 AI Agent 工作台 · 多用户隔离版</p>
    <div class="err" id="errBox"></div>
    <label for="username">用户名</label>
    <input id="username" name="username" autocomplete="username" required autofocus />
    <label for="password">密码</label>
    <input id="password" name="password" type="password" autocomplete="current-password" required />
    <button type="submit" id="submitBtn">登录工作区</button>
  </form>
  <script>
    const form = document.getElementById('loginForm');
    const errBox = document.getElementById('errBox');
    const btn = document.getElementById('submitBtn');
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      errBox.style.display = 'none';
      btn.disabled = true;
      btn.textContent = '正在启动隔离工作区...';
      try {
        const res = await fetch('/api/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            username: document.getElementById('username').value.trim(),
            password: document.getElementById('password').value,
          }),
        });
        const data = await res.json();
        if (!res.ok) {
          throw new Error(data.error || '登录失败');
        }
        window.location.href = '/';
      } catch (err) {
        errBox.textContent = err.message;
        errBox.style.display = 'block';
        btn.disabled = false;
        btn.textContent = '登录工作区';
      }
    });
  </script>
</body>
</html>`
