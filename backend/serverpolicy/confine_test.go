package serverpolicy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConfineLocalPathRejectsOtherTenantsAndAllowsTenantRoots(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "tenant", "home")
	dataDir := filepath.Join(base, "tenant", "config")
	staging := filepath.Join(home, ".cache", "uniterm-web-staging")
	victim := filepath.Join(base, "tenant2", "home")
	for _, dir := range []string{home, dataDir, staging, victim} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(victim, "secret.txt"), []byte("victim"), 0o600); err != nil {
		t.Fatal(err)
	}

	roots := []string{home, dataDir, staging}
	allowed := []string{
		home,
		filepath.Join(home, "notes.txt"),
		filepath.Join(home, "sub", "new-file.txt"),
		filepath.Join(dataDir, "connections.json"),
		filepath.Join(dataDir, "records", "session.cast"),
		filepath.Join(staging, "keep", "upload.bin"),
	}
	for _, path := range allowed {
		got, err := ConfineLocalPath(roots, path)
		if err != nil {
			t.Errorf("allowed path %q was rejected: %v", path, err)
			continue
		}
		if !filepath.IsAbs(got) {
			t.Errorf("confined path %q is not absolute", got)
		}
	}

	denied := []string{
		filepath.Join(victim, "secret.txt"),
		"/data/users/victim/home/secret.txt",
		filepath.Join(home, "..", "..", "tenant2", "home", "secret.txt"),
		filepath.Join(base, "outside.txt"),
	}
	for _, path := range denied {
		if _, err := ConfineLocalPath(roots, path); err == nil {
			t.Errorf("path %q escaped confinement", path)
		} else if !errors.Is(err, ErrPathDenied) {
			t.Errorf("path %q error = %v, want ErrPathDenied", path, err)
		}
	}
}

func TestConfineLocalPathSymlinksCannotEscape(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	staging := filepath.Join(home, ".cache", "uniterm-web-staging")
	victim := filepath.Join(base, "victim")
	for _, dir := range []string{home, staging, victim, filepath.Join(home, "inside")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(victim, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "inside", "ok.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(victim, filepath.Join(home, "to-victim")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := os.Symlink(filepath.Join(home, "inside"), filepath.Join(home, "to-inside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(staging, "staged-victim")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "inside"), filepath.Join(staging, "staged-inside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "missing-target"), filepath.Join(home, "dangling")); err != nil {
		t.Fatal(err)
	}

	roots := []string{home, staging}

	escapes := []string{
		filepath.Join(home, "to-victim", "secret.txt"),
		filepath.Join(home, "to-victim"),
		filepath.Join(staging, "staged-victim", "secret.txt"),
		filepath.Join(home, "dangling"),
	}
	for _, path := range escapes {
		if _, err := ConfineLocalPath(roots, path); err == nil {
			t.Errorf("symlink path %q escaped confinement", path)
		}
	}

	inside := []string{
		filepath.Join(home, "to-inside", "ok.txt"),
		filepath.Join(staging, "staged-inside", "ok.txt"),
	}
	for _, path := range inside {
		if _, err := ConfineLocalPath(roots, path); err != nil {
			t.Errorf("inside-root symlink %q was rejected: %v", path, err)
		}
	}
}

func TestConfineLocalPathRejectsEmptyAndRootless(t *testing.T) {
	if _, err := ConfineLocalPath([]string{t.TempDir()}, "  "); err == nil {
		t.Fatal("empty path must be rejected")
	}
	if _, err := ConfineLocalPath(nil, "x"); err == nil {
		t.Fatal("no roots must be rejected")
	}
}
