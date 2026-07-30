package k8s

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogStreamDeliversLines(t *testing.T) {
	srv, ca := startTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/log") {
			t.Errorf("path missing /log: %q", r.URL.Path)
		}
		if !strings.Contains(r.URL.RawQuery, "follow=true") {
			t.Errorf("query missing follow=true: %q", r.URL.RawQuery)
		}
		flusher, _ := w.(http.Flusher)
		w.WriteHeader(200)
		for _, l := range []string{"line-1", "line-2", "line-3"} {
			fmt.Fprintln(w, l)
			flusher.Flush()
			time.Sleep(3 * time.Millisecond)
		}
	}))
	defer srv.Close()

	kc := &Kubeconfig{
		CurrentContext: "t",
		Contexts:       map[string]contextEntry{"t": {Cluster: "c", User: "u"}},
		Clusters:       map[string]clusterEntry{"c": {Server: srv.URL, CertificateAuthorityData: ca}},
		Users:          map[string]userEntry{"u": {Token: "x"}},
	}
	client, base, err := BuildClient(kc, "t")
	if err != nil {
		t.Fatalf("BuildClient: %v", err)
	}

	m := NewManager()
	// 直接注入一个连接，避开 kubeconfig->connection 的 YAML 编解码环节。
	m.mu.Lock()
	m.conns["c1conn"] = &connection{
		id:      "c1conn",
		client:  client,
		base:    base,
		watches: make(map[string]struct{}),
	}
	m.mu.Unlock()

	var mu sync.Mutex
	var got []string
	m.SetEventEmitter(func(name string, payload any) {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(name, "k8s:log:") {
			// DEV-036: log lines are now batched in 50ms windows, so
			// the emit payload is an array of strings. The test only
			// fires 3 lines with 3ms gaps, but they all flush in one
			// batch on the next 50ms tick.
			switch v := payload.(type) {
			case string:
				got = append(got, v)
			case []string:
				got = append(got, v...)
			}
		}
	})

	sid, err := m.StartLogStream("c1conn", "default", "p1", "c1", 100, false, false)
	if err != nil {
		t.Fatalf("StartLogStream: %v", err)
	}
	if sid == "" {
		t.Fatal("empty streamID")
	}
	time.Sleep(80 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d: %v", len(got), got)
	}
}

func TestBuildLogPath(t *testing.T) {
	// follow 模式（previous=false）：follow=true & previous=false
	p := buildLogPath("default", "p1", "c1", 100, true, false)
	if !strings.HasPrefix(p, "/api/v1/namespaces/default/pods/p1/log?") {
		t.Errorf("bad prefix: %q", p)
	}
	if !strings.Contains(p, "container=c1") ||
		!strings.Contains(p, "follow=true") ||
		!strings.Contains(p, "previous=false") ||
		!strings.Contains(p, "tailLines=100") ||
		!strings.Contains(p, "timestamps=true") {
		t.Errorf("follow path wrong: %q", p)
	}

	// previous 模式：一次性，follow=false & previous=true
	p = buildLogPath("ns2", "p2", "", 50, false, true)
	if !strings.Contains(p, "follow=false") || !strings.Contains(p, "previous=true") {
		t.Errorf("previous path wrong: %q", p)
	}
}
