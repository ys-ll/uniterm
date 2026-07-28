package k8s

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWatchDeliversEvents(t *testing.T) {
	srv, ca := startTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "watch=true") {
			t.Errorf("query missing watch=true: %q", r.URL.RawQuery)
		}
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(200)
		lines := []string{
			`{"type":"ADDED","object":{"kind":"Pod","metadata":{"name":"p1","uid":"u1","resourceVersion":"1"}}}`,
			`{"type":"MODIFIED","object":{"kind":"Pod","metadata":{"name":"p1","uid":"u1","resourceVersion":"2"}}}`,
			`{"type":"DELETED","object":{"kind":"Pod","metadata":{"name":"p1","uid":"u1","resourceVersion":"3"}}}`,
		}
		for _, l := range lines {
			fmt.Fprintln(w, l)
			flusher.Flush()
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer srv.Close()

	kc := &Kubeconfig{
		CurrentContext: "t",
		Contexts:       map[string]contextEntry{"t": {Cluster: "c", User: "u"}},
		Clusters:       map[string]clusterEntry{"c": {Server: srv.URL, CertificateAuthorityData: ca}},
		Users:          map[string]userEntry{"u": {Token: "x"}},
	}
	client, base, _ := BuildClient(kc, "t")

	var mu sync.Mutex
	var got []string
	done := make(chan struct{})
	cb := func(ev WatchEvent) {
		mu.Lock()
		got = append(got, ev.Type)
		mu.Unlock()
	}
	onEnd := func(err error) {
		close(done)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := startWatchStream(ctx, client, base, "/api/v1/namespaces/default/pods?watch=true", cb, onEnd, nil, make(chan struct{}))
	if err != nil {
		t.Fatalf("startWatchStream: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watch timeout")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 || got[0] != "ADDED" || got[2] != "DELETED" {
		t.Errorf("got = %v", got)
	}
}

func TestWatchContextCancel(t *testing.T) {
	srv, ca := startTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.WriteHeader(200)
		for i := 0; ; i++ {
			_, err := fmt.Fprintf(w, `{"type":"ADDED","object":{"metadata":{"resourceVersion":"%d"}}}`+"\n", i)
			if err != nil {
				return
			}
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer srv.Close()

	kc := &Kubeconfig{
		CurrentContext: "t",
		Contexts:       map[string]contextEntry{"t": {Cluster: "c", User: "u"}},
		Clusters:       map[string]clusterEntry{"c": {Server: srv.URL, CertificateAuthorityData: ca}},
		Users:          map[string]userEntry{"u": {Token: "x"}},
	}
	client, base, _ := BuildClient(kc, "t")
	ctx, cancel := context.WithCancel(context.Background())
	ended := make(chan struct{})
	err := startWatchStream(ctx, client, base, "/api/v1/pods?watch=true",
		func(WatchEvent) {},
		func(err error) { close(ended) },
		nil, make(chan struct{}))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-ended:
	case <-time.After(1 * time.Second):
		t.Fatal("cancel did not stop watch")
	}
}
