//go:build server

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ys-ll/uniterm/backend/serverpolicy"
)

// resetServerPolicy reloads the package-level policy snapshot for the test.
func resetServerPolicy() {
	serverPolicyOnce = sync.Once{}
	serverPolicyValue = nil
}

func TestServerPolicyDefaultsDenyLocalGroups(t *testing.T) {
	t.Setenv("UNITERM_SERVER_POLICY", "")
	t.Setenv("UNITERM_SERVER_POLICY_FILE", "")
	resetServerPolicy()
	t.Cleanup(resetServerPolicy)

	policy := currentServerPolicy()
	if err := policy.LoadError(); err != nil {
		t.Fatalf("policy load: %v", err)
	}
	if policy.Allowed("StartTelnetServer") {
		t.Fatal("server defaults must deny localServices")
	}
	if !policy.Allowed("CreateSession") || !policy.Allowed("K8sConnect") {
		t.Fatal("server defaults must keep the tenant remote groups")
	}
	if appServiceOptions().MethodFilter == nil {
		t.Fatal("server build must install a MethodFilter")
	}
}

func TestServerContainerTransportGuard(t *testing.T) {
	t.Setenv("UNITERM_SERVER_POLICY", "")
	t.Setenv("UNITERM_SERVER_POLICY_FILE", "")
	resetServerPolicy()
	t.Cleanup(resetServerPolicy)

	a := &App{}
	if err := a.checkContainerTransport("local"); !errors.Is(err, serverpolicy.ErrDenied) {
		t.Fatalf("local transport error = %v, want ErrDenied", err)
	}
	if err := a.checkContainerTransport("ssh"); err != nil {
		t.Fatalf("SSH transport must be accepted: %v", err)
	}

	t.Setenv("UNITERM_SERVER_POLICY", `{"workerDocker":true,"iAcceptRisk":true}`)
	resetServerPolicy()
	if err := a.checkContainerTransport("local"); err != nil {
		t.Fatalf("operator-enabled local transport was rejected: %v", err)
	}
}

func TestServerGetSaveTempPathIsStaged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := (&App{}).GetSaveTempPath("uniterm-connections.utm")
	if err != nil {
		t.Fatalf("GetSaveTempPath: %v", err)
	}
	staging := filepath.Join(home, ".cache", "uniterm-web-staging")
	rel, err := filepath.Rel(staging, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("GetSaveTempPath returned %q outside %q", path, staging)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("staging/save directory was not created: %v", err)
	}
}

func TestServerConfineLocalPath(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "tenant", "home")
	dataDir := filepath.Join(base, "tenant", "config")
	victim := filepath.Join(base, "tenant2", "home")
	for _, dir := range []string{home, dataDir, victim} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	a := &App{dataDir: dataDir}

	for _, allowed := range []string{
		filepath.Join(home, "notes.txt"),
		filepath.Join(dataDir, "connections.json"),
		filepath.Join(home, ".cache", "uniterm-web-staging", "keep", "key.pem"),
	} {
		if _, err := a.confineLocalPath(allowed); err != nil {
			t.Errorf("tenant path %q rejected: %v", allowed, err)
		}
	}
	if _, err := a.confineLocalPath(filepath.Join(victim, "secret.txt")); err == nil {
		t.Fatal("second tenant path was allowed")
	}
	if _, err := a.confineLocalPath("/data/users/victim/home/secret.txt"); err == nil {
		t.Fatal("/data/users/victim path was allowed")
	}
	if err := os.Symlink(victim, filepath.Join(home, "escape")); err == nil {
		if _, err := a.confineLocalPath(filepath.Join(home, "escape", "secret.txt")); err == nil {
			t.Fatal("symlink escape was allowed")
		}
	}
}
