package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
)

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
