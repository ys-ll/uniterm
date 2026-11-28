//go:build server

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ys-ll/uniterm/backend/k8s"
	"github.com/ys-ll/uniterm/backend/serverpolicy"
	"github.com/ys-ll/uniterm/backend/session"
)

func setTenantHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestServerKubeconfigPathConfined(t *testing.T) {
	home := setTenantHome(t)
	a := &App{k8sManager: k8s.NewManager()}

	victim := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(victim, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{victim, "/proc/self/environ", "/data/users/victim/home/kubeconfig"} {
		if _, err := a.readKubeconfigSource(path, true); !errors.Is(err, serverpolicy.ErrPathDenied) {
			t.Fatalf("readKubeconfigSource(%q) error = %v, want ErrPathDenied", path, err)
		}
	}
	if _, err := a.K8sListContexts(victim, true); !errors.Is(err, serverpolicy.ErrPathDenied) {
		t.Fatalf("K8sListContexts victim error = %v, want ErrPathDenied", err)
	}
	if _, err := a.K8sConnect(victim, true, "", "", "", "", ""); !errors.Is(err, serverpolicy.ErrPathDenied) {
		t.Fatalf("K8sConnect victim error = %v, want ErrPathDenied", err)
	}
	if _, err := a.TestConnection(session.ConnectionConfig{
		Type: "k8s", K8sConfigPath: victim,
	}); !errors.Is(err, serverpolicy.ErrPathDenied) {
		t.Fatalf("TestConnection k8s victim error = %v, want ErrPathDenied", err)
	}

	allowed := filepath.Join(home, "kubeconfig")
	if err := os.WriteFile(allowed, []byte("apiVersion: v1\nkind: Config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.readKubeconfigSource(allowed, true); err != nil {
		t.Fatalf("allowed kubeconfig path rejected: %v", err)
	}

	inline := "apiVersion: v1\nkind: Config\nclusters: []\n"
	raw, err := a.readKubeconfigSource(inline, false)
	if err != nil {
		t.Fatalf("inline kubeconfig rejected: %v", err)
	}
	if string(raw) != inline {
		t.Fatalf("inline kubeconfig changed: %q", raw)
	}
}

func TestServerDBCSVPathConfined(t *testing.T) {
	home := setTenantHome(t)
	a := &App{sessionManager: session.NewSessionManager()}

	victim := filepath.Join(t.TempDir(), "leak.csv")
	if err := os.WriteFile(victim, []byte("stolen-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{victim, "/proc/self/environ", "/data/users/victim/home/leak.csv"} {
		if _, err := a.DBPreviewCSV(path); !errors.Is(err, serverpolicy.ErrPathDenied) {
			t.Fatalf("DBPreviewCSV(%q) error = %v, want ErrPathDenied", path, err)
		}
	}
	if _, err := a.DBImportCSV("missing-session", "", "", victim, nil); !errors.Is(err, serverpolicy.ErrPathDenied) {
		t.Fatalf("DBImportCSV victim error = %v, want ErrPathDenied", err)
	}

	allowed := filepath.Join(home, "ok.csv")
	if err := os.WriteFile(allowed, []byte("name,age\nalice,30\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := a.DBPreviewCSV(allowed)
	if err != nil {
		t.Fatalf("allowed CSV preview rejected: %v", err)
	}
	if len(preview.Headers) != 2 || preview.Headers[0] != "name" {
		t.Fatalf("allowed CSV preview headers = %#v", preview.Headers)
	}
	// The allowed path must pass confinement and fail later on the missing
	// session, not with a path-denial error.
	if _, err := a.DBImportCSV("missing-session", "", "", allowed, nil); err == nil || errors.Is(err, serverpolicy.ErrPathDenied) {
		t.Fatalf("DBImportCSV allowed path error = %v, want a provider error", err)
	}
}

func TestServerSQLiteHostConfined(t *testing.T) {
	home := setTenantHome(t)
	a := &App{sessionManager: session.NewSessionManager()}

	victim := filepath.Join(t.TempDir(), "victim.db")
	if err := os.WriteFile(victim, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := session.ConnectionConfig{Type: "database", DBType: "sqlite", Host: victim}
	if _, err := a.CreateSession("database", cfg); !errors.Is(err, serverpolicy.ErrPathDenied) {
		t.Fatalf("CreateSession sqlite victim error = %v, want ErrPathDenied", err)
	}
	if _, err := a.TestConnection(cfg); !errors.Is(err, serverpolicy.ErrPathDenied) {
		t.Fatalf("TestConnection sqlite victim error = %v, want ErrPathDenied", err)
	}
	if _, err := a.TestConnection(session.ConnectionConfig{
		Type: "database", DBType: "sqlite", Host: "file:/data/users/victim/db",
	}); !errors.Is(err, serverpolicy.ErrPathDenied) {
		t.Fatalf("file: URI sqlite error = %v, want ErrPathDenied", err)
	}
	if _, err := a.TestConnection(session.ConnectionConfig{
		Type: "database", DBType: "sqlite", Host: "/data/users/victim/home/db",
	}); !errors.Is(err, serverpolicy.ErrPathDenied) {
		t.Fatalf("victim sqlite path error = %v, want ErrPathDenied", err)
	}
	if got, err := a.confineSQLiteHost(":memory:"); err != nil || got != ":memory:" {
		t.Fatalf("sqlite :memory: = %q, %v; want unchanged", got, err)
	}

	allowed := filepath.Join(home, "ok.db")
	if err := os.WriteFile(allowed, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.TestConnection(session.ConnectionConfig{
		Type: "database", DBType: "sqlite", Host: allowed,
	}); err != nil {
		t.Fatalf("allowed sqlite path rejected: %v", err)
	}
}

func TestServerSftpListLocalEmptyDirUsesTenantHome(t *testing.T) {
	home := setTenantHome(t)
	if err := os.WriteFile(filepath.Join(home, "visible.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}

	manager := session.NewSessionManager()
	local := session.NewLocalFileSession("local-1")
	if err := local.Connect(session.ConnectionConfig{}); err != nil {
		t.Fatalf("local connect: %v", err)
	}
	manager.Add(local)
	a := &App{sessionManager: manager}

	result, err := a.SftpListLocal("local-1", "")
	if err != nil {
		t.Fatalf("SftpListLocal(\"\") = %v, want the tenant home listing", err)
	}
	found := false
	for _, file := range result.Files {
		if file.Name == "visible.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("home listing did not include visible.txt: %#v", result.Files)
	}

	// Other confined methods still reject an empty path.
	if _, err := a.ReadFileBase64(""); !errors.Is(err, serverpolicy.ErrPathDenied) {
		t.Fatalf("ReadFileBase64(\"\") error = %v, want ErrPathDenied", err)
	}
}
