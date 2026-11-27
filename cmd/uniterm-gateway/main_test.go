package main

import (
	"compress/gzip"
"encoding/json"
	"errors"
"fmt"
"github.com/gorilla/websocket"
	"io"
"net"
	"net/http"
	"net/http/httptest"
"os"
	"strconv"
	"strings"
	"testing"
)

func TestIsCompressibleAsset(t *testing.T) {
	cases := map[string]bool{
		"/assets/index-abc.js":   true,
		"/assets/index-abc.css":  true,
		"/assets/app.mjs":        true,
		"/assets/font.woff2":     false,
		"/assets/logo.png":       false,
		"/assets/mod.wasm":       false,
		"/index.html":            false,
		"/assets/../secret.json": true,
	}
	for p, want := range cases {
		if got := isCompressibleAsset(p); got != want {
			t.Errorf("isCompressibleAsset(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestAcceptsGzip(t *testing.T) {
	cases := map[string]bool{
		"":                  false,
		"gzip":              true,
		"br, gzip":          true,
		"GZIP; q=1.0":       true,
		"gzip;q=0":          false,
		"br":                false,
		"deflate, gzip;q=0": false,
	}
	for header, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/assets/a.js", nil)
		if header != "" {
			req.Header.Set("Accept-Encoding", header)
		}
		if got := acceptsGzip(req); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

func TestWorkerWaitReady(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		w := &WorkerInstance{Username: "alice", ready: make(chan struct{})}
		close(w.ready)
		got, err := w.waitReady()
		if err != nil || got != w {
			t.Fatalf("waitReady() = %v, %v", got, err)
		}
	})

	t.Run("spawn failed", func(t *testing.T) {
		wantErr := errors.New("boom")
		w := &WorkerInstance{Username: "alice", ready: make(chan struct{}), readyErr: wantErr}
		close(w.ready)
		if _, err := w.waitReady(); !errors.Is(err, wantErr) {
			t.Fatalf("waitReady() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("process exited", func(t *testing.T) {
		w := &WorkerInstance{Username: "alice", ready: make(chan struct{}), done: make(chan struct{})}
		close(w.done)
		if _, err := w.waitReady(); !errors.Is(err, errWorkerExited) {
			t.Fatalf("waitReady() error = %v, want errWorkerExited", err)
		}
	})
}

func TestCompressibleAssets(t *testing.T) {
	body := strings.Repeat("console.log('uniterm');", 1000)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = io.WriteString(w, body)
	})
	handler := compressibleAssets(next)

	cases := []struct {
		name        string
		path        string
		method      string
		accept      string
		rangeHeader string
		wantGzip    bool
	}{
		{name: "gzip", path: "/assets/index.js", method: http.MethodGet, accept: "gzip, deflate", wantGzip: true},
		{name: "client does not accept gzip", path: "/assets/index.js", method: http.MethodGet, wantGzip: false},
		{name: "head passes through", path: "/assets/index.js", method: http.MethodHead, accept: "gzip", wantGzip: false},
		{name: "range passes through", path: "/assets/index.js", method: http.MethodGet, accept: "gzip", rangeHeader: "bytes=0-100", wantGzip: false},
		{name: "non-asset passes through", path: "/index.html", method: http.MethodGet, accept: "gzip", wantGzip: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.accept != "" {
				req.Header.Set("Accept-Encoding", tc.accept)
			}
			if tc.rangeHeader != "" {
				req.Header.Set("Range", tc.rangeHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			gotGzip := rec.Header().Get("Content-Encoding") == "gzip"
			if gotGzip != tc.wantGzip {
				t.Fatalf("Content-Encoding = %q, want gzip=%v", rec.Header().Get("Content-Encoding"), tc.wantGzip)
			}
			if gotGzip && rec.Header().Get("Content-Length") != "" {
				t.Fatalf("Content-Length must be dropped for a compressed response")
			}

			payload := rec.Body.Bytes()
			if gotGzip {
				zr, err := gzip.NewReader(rec.Body)
				if err != nil {
					t.Fatalf("gzip.NewReader: %v", err)
				}
				payload, err = io.ReadAll(zr)
				if err != nil {
					t.Fatalf("read gzip body: %v", err)
				}
			}
			if string(payload) != body {
				t.Fatalf("body mismatch: got %d bytes", len(payload))
			}
		})
	}
}


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


// TestMain doubles as the fake worker binary for the spawn tests: when
// re-executed with GO_WORKER_HELPER=1 it serves /health on
// WAILS_SERVER_PORT until killed, mimicking the real worker's readiness
// handshake without dragging in the wails stack.
func TestMain(m *testing.M) {
	if os.Getenv("GO_WORKER_HELPER") == "1" {
		port := os.Getenv("WAILS_SERVER_PORT")
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok"}`))
		})
		if err := http.ListenAndServe("127.0.0.1:"+port, mux); err != nil {
			fmt.Fprintln(os.Stderr, "helper:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// newTestGateway returns a gateway wired to spawn the test binary as the
// fake worker, with the login-password cache pre-populated per the flag.
func newTestGateway(t *testing.T, cachePassword bool) *MultiUserGateway {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	gw := &MultiUserGateway{
		dataRoot:     t.TempDir(),
		workerBinary: exe,
		workers:      map[string]*WorkerInstance{},
		usedPorts:    map[int]bool{},
		passwords:    map[string]string{},
	}
	// Grab a free loopback port as the spawn base so the fake worker binds
	// somewhere real (allocatePortLocked treats 0 as a literal port).
	if ln, err := net.Listen("tcp", "127.0.0.1:0"); err != nil {
		t.Fatalf("reserve base port: %v", err)
	} else {
		gw.baseWorkerPort = ln.Addr().(*net.TCPAddr).Port
		ln.Close()
	}
	// Terminate spawned fake workers before the TempDir cleanup runs — a
	// live child holds its working directory open, which fails RemoveAll on
	// Windows.
	t.Cleanup(func() {
		gw.workersMu.Lock()
		instances := make([]*WorkerInstance, 0, len(gw.workers))
		for _, inst := range gw.workers {
			instances = append(instances, inst)
		}
		gw.workersMu.Unlock()
		for _, inst := range instances {
			terminateWorkerInstance(inst)
		}
	})
	t.Setenv("GO_WORKER_HELPER", "1")
	if cachePassword {
		gw.passwords["admin"] = "login-password"
	}
	return gw
}

// TestGetOrSpawnWorkerForwardsPasswordOnReplace covers the login-loop fix: a
// worker spawned while the gateway had no cached login password (a browser
// with a still-valid session cookie reconnecting right after a gateway
// restart) is replaced — not reused — once the user actually logs in, so the
// fresh spawn receives UNITERM_MASTER_PASSWORD and can unlock its store.
func TestGetOrSpawnWorkerForwardsPasswordOnReplace(t *testing.T) {
	gw := newTestGateway(t, false)

	// First spawn: pre-login session-carryover request → no password.
	stale, err := gw.getOrSpawnWorker("admin", true)
	if err != nil {
		t.Fatalf("initial spawn: %v", err)
	}
	if stale.hasMasterPassword {
		t.Fatalf("precondition: first spawn should carry no master password")
	}

	// The user logs in: the gateway caches the password...
	gw.passwords["admin"] = "login-password"

	// ...and the next get-or-spawn must replace the stale worker instead of
	// returning it forever.
	fresh, err := gw.getOrSpawnWorker("admin", true)
	if err != nil {
		t.Fatalf("replacement spawn: %v", err)
	}
	if fresh == stale {
		t.Fatalf("stale password-less worker was reused instead of replaced")
	}
	if !fresh.hasMasterPassword {
		t.Fatalf("replacement worker does not carry the master password")
	}

	select {
	case <-stale.done:
	default:
		t.Fatalf("stale worker process was not terminated")
	}

	// Subsequent calls keep returning the (healthy, password-carrying) worker.
	again, err := gw.getOrSpawnWorker("admin", true)
	if err != nil {
		t.Fatalf("third call: %v", err)
	}
	if again != fresh {
		t.Fatalf("healthy worker should be reused, got a different instance")
	}
}

// TestGetOrSpawnWorkerReusesHealthyWorker pins the non-replacement path: a
// worker spawned with a password is reused even when it is re-requested.
func TestGetOrSpawnWorkerReusesHealthyWorker(t *testing.T) {
	gw := newTestGateway(t, true)

	first, err := gw.getOrSpawnWorker("admin", true)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !first.hasMasterPassword {
		t.Fatalf("spawn with cached password should carry the master password")
	}
	second, err := gw.getOrSpawnWorker("admin", true)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if second != first {
		t.Fatalf("healthy worker was not reused")
	}
}
