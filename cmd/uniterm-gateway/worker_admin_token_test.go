package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestPostToWorkerSendsAdminToken(t *testing.T) {
	var gotAdmin, gotWorker string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAdmin = r.Header.Get("X-Uniterm-Worker-Admin-Token")
		gotWorker = r.Header.Get("X-Uniterm-Worker-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	inst := &WorkerInstance{Port: port, token: "worker-token", adminToken: "admin-token"}
	code, body, err := (&MultiUserGateway{}).postToWorker(inst, "/api/web/credentials/reset", nil)
	if err != nil || code != http.StatusOK {
		t.Fatalf("postToWorker = %d, %v, body=%s", code, err, body)
	}
	if gotAdmin != "admin-token" {
		t.Fatalf("admin header = %q, want admin-token", gotAdmin)
	}
	if gotWorker != "worker-token" {
		t.Fatalf("worker header = %q, want worker-token", gotWorker)
	}
}

func TestWorkerProxyStripsTenantAdminHeader(t *testing.T) {
	var gotAdmin, gotWorker string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAdmin = r.Header.Get("X-Uniterm-Worker-Admin-Token")
		gotWorker = r.Header.Get("X-Uniterm-Worker-Token")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := newWorkerProxy(target, "gateway-worker-token")

	front := httptest.NewServer(proxy)
	defer front.Close()

	req, err := http.NewRequest(http.MethodPost, front.URL+"/api/web/credentials/reset", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Uniterm-Worker-Admin-Token", "smuggled-admin-token")
	req.Header.Set("X-Uniterm-Worker-Token", "tenant-supplied-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if gotAdmin != "" {
		t.Fatalf("admin header reached the worker: %q", gotAdmin)
	}
	if gotWorker != "gateway-worker-token" {
		t.Fatalf("worker token = %q, want the gateway token", gotWorker)
	}
}
