package main

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
