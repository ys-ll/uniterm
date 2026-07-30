package k8s

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDoGet(t *testing.T) {
	srv, ca := startTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/default/pods" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer x" {
			t.Errorf("auth missing")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"kind": "PodList", "items": []any{}})
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
		t.Fatal(err)
	}
	status, body, err := Do(context.Background(), client, base, "GET", "/api/v1/namespaces/default/pods", nil, "")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d", status)
	}
	if !strings.Contains(string(body), `"kind":"PodList"`) {
		t.Errorf("body = %s", body)
	}
}

func TestDoPatchWithContentType(t *testing.T) {
	var gotCT string
	var gotBody []byte
	srv, ca := startTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	kc := &Kubeconfig{
		CurrentContext: "t",
		Contexts:       map[string]contextEntry{"t": {Cluster: "c", User: "u"}},
		Clusters:       map[string]clusterEntry{"c": {Server: srv.URL, CertificateAuthorityData: ca}},
		Users:          map[string]userEntry{"u": {Token: "x"}},
	}
	client, base, _ := BuildClient(kc, "t")
	body := []byte(`{"kind":"Pod"}`)
	_, _, err := Do(context.Background(), client, base, "PATCH",
		"/api/v1/namespaces/default/pods/p1?fieldManager=uniterm",
		body, "application/apply-patch+yaml")
	if err != nil {
		t.Fatal(err)
	}
	if gotCT != "application/apply-patch+yaml" {
		t.Errorf("content-type = %q", gotCT)
	}
	if string(gotBody) != `{"kind":"Pod"}` {
		t.Errorf("body = %s", gotBody)
	}
}
