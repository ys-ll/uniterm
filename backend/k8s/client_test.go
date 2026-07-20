package k8s

import (
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func startTLSServer(t *testing.T, handler http.Handler) (*httptest.Server, []byte) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	cert := srv.Certificate()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		t.Fatal("cannot append test CA")
	}
	return srv, ca
}

func TestClientBearerToken(t *testing.T) {
	var gotAuth string
	srv, ca := startTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		w.Write([]byte(`{"kind":"APIVersions"}`))
	}))
	defer srv.Close()

	kc := &Kubeconfig{
		CurrentContext: "t",
		Contexts:       map[string]contextEntry{"t": {Cluster: "c", User: "u"}},
		Clusters:       map[string]clusterEntry{"c": {Server: srv.URL, CertificateAuthorityData: ca}},
		Users:          map[string]userEntry{"u": {Token: "abc123"}},
	}
	client, base, err := BuildClient(kc, "t")
	if err != nil {
		t.Fatalf("BuildClient: %v", err)
	}
	if base != srv.URL {
		t.Errorf("base = %q want %q", base, srv.URL)
	}
	req, _ := http.NewRequest("GET", base+"/api", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)
	if gotAuth != "Bearer abc123" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

func TestClientInsecureSkip(t *testing.T) {
	srv, _ := startTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	kc := &Kubeconfig{
		CurrentContext: "t",
		Contexts:       map[string]contextEntry{"t": {Cluster: "c", User: "u"}},
		Clusters:       map[string]clusterEntry{"c": {Server: srv.URL, InsecureSkipTLSVerify: true}},
		Users:          map[string]userEntry{"u": {Token: "x"}},
	}
	client, base, err := BuildClient(kc, "t")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", base+"/", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("insecure Do: %v", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d want 200", resp.StatusCode)
	}
}

func TestClientMissingCARejectsUntrustedServer(t *testing.T) {
	srv, _ := startTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	kc := &Kubeconfig{
		CurrentContext: "t",
		Contexts:       map[string]contextEntry{"t": {Cluster: "c", User: "u"}},
		Clusters:       map[string]clusterEntry{"c": {Server: srv.URL}}, // 无 CA，不 skip verify
		Users:          map[string]userEntry{"u": {}},
	}
	client, base, err := BuildClient(kc, "t")
	if err != nil {
		t.Fatalf("BuildClient: %v", err)
	}
	req, _ := http.NewRequest("GET", base+"/", nil)
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected TLS verification error, got nil")
	}
}

func TestClientClientCertRequiresBoth(t *testing.T) {
	kc := &Kubeconfig{
		CurrentContext: "t",
		Contexts:       map[string]contextEntry{"t": {Cluster: "c", User: "u"}},
		Clusters:       map[string]clusterEntry{"c": {Server: "https://example.invalid", InsecureSkipTLSVerify: true}},
		Users:          map[string]userEntry{"u": {ClientCertificateData: []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")}},
	}
	_, _, err := BuildClient(kc, "t")
	if err == nil {
		t.Fatal("expected error for missing client key, got nil")
	}
	if !strings.Contains(err.Error(), "both") {
		t.Errorf("error = %v; want message containing \"both\"", err)
	}
}
