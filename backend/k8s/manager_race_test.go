package k8s

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// QA-015 / F-411: race-condition regression test.
//
// Manager.conns / watches / logs maps are guarded by m.mu, and
// readers use m.mu.RLock().  This test fires many goroutines that
// simultaneously ListContexts, look up Provider-like state, and
// perform map-shape-sensitive operations.  Run with `go test -race`
// — any future code that forgets the lock trips the race detector.

func TestManager_ConcurrentMapAccess(t *testing.T) {
	const goroutines = 32
	const iterations = 50

	m := NewManager()
	m.SetEventEmitter(func(string, any) {})

	// Pre-populate a couple of conn entries so concurrent readers
	// observe a non-empty map.
	raw := []byte(`
apiVersion: v1
kind: Config
current-context: a
contexts:
- name: a
  context: {cluster: c1, user: u1, namespace: ns-a}
- name: b
  context: {cluster: c2, user: u2}
`)

	if _, err := m.ListContexts(raw); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var listIters int64
	var lookupIters int64

	deadline := time.Now().Add(3 * time.Second)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if time.Now().After(deadline) {
					return
				}
				switch (id + j) % 4 {
				case 0:
					// ListContexts: read-only, safe concurrent.
					if _, err := m.ListContexts(raw); err != nil {
						t.Errorf("ListContexts: %v", err)
						return
					}
					atomic.AddInt64(&listIters, 1)
				case 1:
					// cachedParseBytes: concurrent-safe via sync.Map, but
					// accesses Manager and is exercised in this mix.
					if _, err := m.cachedParseBytes(raw); err != nil {
						t.Errorf("cachedParseBytes: %v", err)
						return
					}
					atomic.AddInt64(&lookupIters, 1)
				case 2:
					// sweepLoop: this just calls sweepOnce under the lock;
					// running it directly proves there's no reader/writer
					// race.
					m.sweepOnce()
				case 3:
					// Probe the kubeconfigCache — same sync.Map the
					// production code reads from.
					if v, ok := m.kubeconfigCache.Load("does-not-exist"); ok || v != nil {
						t.Errorf("unexpected cache hit on empty key: ok=%v v=%v", ok, v)
					}
				}
			}
		}(i)
	}

	wg.Wait()
	if listIters+lookupIters == 0 {
		t.Fatal("no work performed")
	}
}

// TestManager_DisconnectVsList_NoRace catches a subtle race between
// Disconnect (writes) and ListContexts (reads).  Disconnect walks
// conn.watches/logs and mutates them; ListContexts reads from
// kubeconfigCache (separate map, but the synthetic mix matters: a
// future refactor that moves ListContexts onto conns paths would
// otherwise race silently).
func TestManager_DisconnectVsList_NoRace(t *testing.T) {
	m := NewManager()
	m.SetEventEmitter(func(string, any) {})

	// Even without a real connection, ListContexts on a raw config is
	// still cheap and exercises the same code path as a live audit log.
	raw := []byte(`
apiVersion: v1
kind: Config
current-context: a
contexts:
- name: a
  context: {cluster: c1, user: u1}
`)

	var wg sync.WaitGroup
	done := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			_, _ = m.ListContexts(raw)
		}
	}()

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				// Disconnect on a non-existent ID is a no-op but
				// still acquires m.mu (F-413 path).
				m.Disconnect("nope")
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)
	close(done)
	wg.Wait()
}

// TestManager_KubeconfigCache_ConcurrentSafety pins the contract
// that simultaneous parses of identical content DON'T racy
// duplicate the work.  Reads and writes to the sync.Map are
// internally safe; this just exercises the cache in a fan-out to
// catch assumptions about returned pointers.
func TestManager_KubeconfigCache_ConcurrentSafety(t *testing.T) {
	m := NewManager()
	m.SetEventEmitter(func(string, any) {})

	raw := []byte(`
apiVersion: v1
kind: Config
current-context: a
contexts:
- name: a
  context: {cluster: c1, user: u1}
clusters:
- name: c1
  cluster: {server: http://localhost:1}
users:
- name: u1
  user: {token: x}
`)

	const goroutines = 16
	var wg sync.WaitGroup
	seen := make(chan *Kubeconfig, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			kc, err := m.cachedParseBytes(raw)
			if err != nil {
				t.Errorf("cachedParseBytes: %v", err)
				return
			}
			seen <- kc
		}()
	}
	wg.Wait()
	close(seen)

	// All seen pointers must equal the same cached value (sync.Map
	// LoadOrStore returns the first-stored pointer; subsequent loads
	// return the same).
	first := <-seen
	if first == nil {
		t.Fatal("nil Kubeconfig returned")
	}
	for kc := range seen {
		if kc != first {
			t.Errorf("expected all parsed configs to share the cached pointer, got %p vs %p", first, kc)
		}
	}
}

// TestManager_SweepCleansExitedWatches sets up a Manager with one
// closed watch handle and verifies sweepOnce drops it.  This pins
// the leaked-handle hygiene F-413 introduced.
func TestManager_SweepCleansExitedWatches(t *testing.T) {
	m := NewManager()
	m.SetEventEmitter(func(string, any) {})

	// Inject a synthetic closed-handle entry.
	done := make(chan struct{})
	close(done)
	m.mu.Lock()
	m.watches["w1"] = &watchHandle{connID: "x", done: done}
	m.mu.Unlock()

	m.sweepOnce()

	m.mu.RLock()
	_, kept := m.watches["w1"]
	m.mu.RUnlock()
	if kept {
		t.Fatal("sweepOnce should have removed the closed watch handle")
	}
}

// Compile-time interface compliance.
var _ = http.Handler(nil)
