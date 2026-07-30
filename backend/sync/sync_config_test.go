package sync

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSyncConfigStore_SaveLoadRoundTrip: write config, read back; the
// load must return the stored values.
func TestSyncConfigStore_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewSyncConfigStore(dir)
	cfg := SyncConfig{
		RepoURL:        "https://example.com/repo.git",
		Branch:         "main",
		Username:       "alice",
		Local:          false,
		AutoSync:       true,
		LastSyncStatus: "success",
		LastSyncError:  "",
	}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoURL != cfg.RepoURL || got.Username != cfg.Username || got.Branch != cfg.Branch ||
		got.AutoSync != cfg.AutoSync || got.LastSyncStatus != cfg.LastSyncStatus {
		t.Fatalf("round-trip mismatch:\ngot  %+v\nwant %+v", got, cfg)
	}
}

// TestSyncConfigStore_LoadMissingFileReturnsDefaults: when no file
// exists, load returns the default SyncConfig with Branch="main" —
// this is the path ConfigureRepo takes on a fresh install.
func TestSyncConfigStore_LoadMissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	store := NewSyncConfigStore(dir)
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Branch != "main" {
		t.Errorf("Branch = %q, want %q", got.Branch, "main")
	}
	if got.RepoURL != "" {
		t.Errorf("expected empty RepoURL on first install, got %q", got.RepoURL)
	}
}

// TestSyncConfigStore_CorruptFileFallsBackToDefaults verifies that a
// corrupt JSON file does NOT block first-load; load returns the
// default config (Branch="main") and preserves the corrupt file for
// the user to inspect.
func TestSyncConfigStore_CorruptFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, syncConfigFileName)
	if err := os.WriteFile(cfgPath, []byte("{ not json"), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewSyncConfigStore(dir)
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Branch != "main" {
		t.Errorf("Branch = %q, want %q", got.Branch, "main")
	}
}

// TestSyncConfigStore_EmptyBranchDefaultsToMain: legacy configs saved
// before the Branch field existed (or were saved with Branch="") need
// to default to "main" so Sync() doesn't blow up on an empty branch.
func TestSyncConfigStore_EmptyBranchDefaultsToMain(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, syncConfigFileName)
	if err := os.WriteFile(cfgPath, []byte(`{"repoUrl":"x","branch":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewSyncConfigStore(dir)
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Branch != "main" {
		t.Errorf("Branch = %q, want %q", got.Branch, "main")
	}
}
