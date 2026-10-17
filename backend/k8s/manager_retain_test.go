package k8s

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRetainedConnectionSurvivesPageClose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("alive")) }))
	defer srv.Close()
	var closed atomic.Int32
	m := &Manager{conns: map[string]*connection{
		"runtime": {client: srv.Client(), base: srv.URL, onClose: func() { closed.Add(1) }},
	}}
	execRelease, err := m.Retain("runtime")
	if err != nil {
		t.Fatal(err)
	}
	fileRelease, err := m.Retain("runtime")
	if err != nil {
		t.Fatal(err)
	}
	m.Disconnect("runtime")
	m.Disconnect("runtime")
	execRelease()
	_, body, err := m.Request(context.Background(), "runtime", "GET", "/", nil, "")
	if err != nil || string(body) != "alive" || closed.Load() != 0 {
		t.Fatalf("closed while file/exec session owns connection: %s %v", body, err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); fileRelease() }()
	}
	wg.Wait()
	if closed.Load() != 1 {
		t.Fatalf("tunnel cleanup count = %d", closed.Load())
	}
	if _, _, err := m.Request(context.Background(), "runtime", "GET", "/", nil, ""); err == nil {
		t.Fatal("connection leaked after final release")
	}
}
