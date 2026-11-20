package sync

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeSnapshotBackend is an in-memory SnapshotBackend for flow tests.
type fakeSnapshotBackend struct {
	store    map[string][]byte // name → content
	manifest *SnapshotInfo
	// raceAfterStore simulates another device overwriting the snapshot
	// right after our StoreAll (lost-update window): StoreAll succeeds
	// but the remote manifest ends up belonging to someone else.
	raceAfterStore bool
}

func newFakeSnapshotBackend() *fakeSnapshotBackend {
	return &fakeSnapshotBackend{store: map[string][]byte{}}
}

func (f *fakeSnapshotBackend) Probe() error { return nil }

func (f *fakeSnapshotBackend) Head() (*SnapshotInfo, error) { return f.manifest, nil }

func (f *fakeSnapshotBackend) FetchAll(dir string) error {
	if f.manifest == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for name, data := range f.store {
		if name == saltName {
			// Mirror the real backend layout: the salt is written via
			// WriteSaltFile (hex-encoded) so ReadSaltFile can read it back.
			if err := WriteSaltFile(dir, data); err != nil {
				return err
			}
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeSnapshotBackend) StoreAll(stagingDir string, salt []byte, manifest SnapshotInfo) error {
	// Delta contract: staged blobs replace their remote copies, blobs not
	// staged stay as they are — the real remote is not wiped either.
	if f.store == nil {
		f.store = map[string][]byte{}
	}
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == saltName {
			continue
		}
		data, err := os.ReadFile(filepath.Join(stagingDir, e.Name()))
		if err != nil {
			return err
		}
		f.store[e.Name()] = data
	}
	f.store[saltName] = salt
	if f.raceAfterStore {
		// Another device's upload landed in the lost-update window: the
		// manifest on the remote is no longer ours.
		f.manifest = &SnapshotInfo{
			Version:    "clobbered-" + manifest.Version,
			UpdatedAt:  time.Now().UTC(),
			FileHashes: map[string]string{"connections.json": "someone-elses-content"},
		}
		return nil
	}
	m := manifest
	f.manifest = &m
	return nil
}

func (f *fakeSnapshotBackend) staging(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// newSnapshotTestService builds a SyncService against temp dirs with the
// encryption key stored, mirroring a configured device.
func newSnapshotTestService(t *testing.T, scope []string) (*SyncService, string, []byte) {
	t.Helper()
	// Isolate from the real "uniTerm" keyring service so the test never
	// touches the user's stored credentials (same pattern as
	// keychain_webdav_test.go).
	origService := keychainService
	keychainService = "uniTerm-test"
	t.Cleanup(func() { keychainService = origService })

	dataDir := t.TempDir()
	// Build the service with explicit temp dirs — NOT NewSyncService(dataDir),
	// which hardcodes configDir/repoPath to the real UserConfigDir and would
	// clobber the user's sync-config.json and sync-repo during test runs
	// (syncSnapshot uses s.repoPath as its fetch staging dir).
	s := newScopeTestService(t, t.TempDir(), dataDir)
	stores := make([]SyncableStore, 0, len(syncableFiles))
	for _, name := range syncableFiles {
		stores = append(stores, &snapshotBlobStore{name: name, dir: dataDir})
	}
	s.SetStores(stores)
	key := []byte("0123456789abcdef0123456789abcdef")
	if err := s.keychain.StoreEncryptionKey(key); err != nil {
		t.Fatal(err)
	}
	if err := s.configStore.Save(SyncConfig{Branch: "main", SyncScope: scope, Backend: "webdav"}); err != nil {
		t.Fatal(err)
	}
	return s, dataDir, key
}

func writeConfig(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotBootstrapPush(t *testing.T) {
	s, dataDir, _ := newSnapshotTestService(t, []string{"connections.json"})
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	b := newFakeSnapshotBackend()

	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncPush {
		t.Fatalf("direction = %v", res.Direction)
	}
	if b.manifest == nil {
		t.Fatal("remote must be populated")
	}
	cfg, _ := s.configStore.Load()
	if cfg.LastSyncedHash == "" {
		t.Fatal("LastSyncedHash must be recorded")
	}
}

func TestSnapshotSilentPull(t *testing.T) {
	s, dataDir, _ := newSnapshotTestService(t, []string{"connections.json"})
	// Device B: local empty, remote has data → silent pull.
	b := newFakeSnapshotBackend()
	salt := []byte("0123456789abcdef")
	key, _ := s.keychain.GetEncryptionKey()
	// Device A populates the remote. One shared staging dir: each
	// b.staging(t) call makes a fresh temp dir, so encrypt and store
	// must use the same one.
	staging := b.staging(t)
	srcDir := t.TempDir()
	writeConfig(t, srcDir, "connections.json", `{"v":2}`)
	if err := EncryptConfigFilesScoped([]string{"connections.json"}, srcDir, staging, key); err != nil {
		t.Fatal(err)
	}
	hashes, _, err := canonicalDirHashes([]string{"connections.json"}, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.StoreAll(staging, salt, SnapshotInfo{
		Version: "v1", UpdatedAt: time.Now().UTC(), FileHashes: hashes,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncPull {
		t.Fatalf("direction = %v, want pull", res.Direction)
	}
	data, _ := os.ReadFile(filepath.Join(dataDir, "connections.json"))
	if string(data) != `{"v":2}` {
		t.Fatalf("pulled content = %q", data)
	}
}

func TestSnapshotConflictWhenBothChanged(t *testing.T) {
	s, dataDir, key := newSnapshotTestService(t, []string{"connections.json"})
	writeConfig(t, dataDir, "connections.json", `{"local":true}`)
	b := newFakeSnapshotBackend()

	// First sync: local data → push, records LastSyncedHash.
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}
	// Remote advanced by another device (local LastSyncedHash is stale).
	other := t.TempDir()
	staging := b.staging(t)
	writeConfig(t, other, "connections.json", `{"remote":true}`)
	if err := EncryptConfigFilesScoped([]string{"connections.json"}, other, staging, key); err != nil {
		t.Fatal(err)
	}
	hashes, _, err := canonicalDirHashes([]string{"connections.json"}, other)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.StoreAll(staging, []byte("0123456789abcdef"), SnapshotInfo{
		Version: "v2", UpdatedAt: time.Now().UTC(), FileHashes: hashes,
	}); err != nil {
		t.Fatal(err)
	}
	// Local changed too.
	writeConfig(t, dataDir, "connections.json", `{"local":true,"more":1}`)

	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncConflict || res.Conflict == nil {
		t.Fatalf("want conflict, got %+v", res)
	}
}

func TestSnapshotUpToDate(t *testing.T) {
	s, dataDir, _ := newSnapshotTestService(t, []string{"connections.json"})
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	b := newFakeSnapshotBackend()
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}
	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncNone {
		t.Fatalf("direction = %v, want none", res.Direction)
	}
}

// TestSnapshotPullKeepsOutOfManifestLocalFile: another device synced with a
// narrower scope, so the remote manifest carries only a subset of this
// device's scope. The pull must decrypt only the files the remote actually
// carries — writing "{}" over the out-of-manifest local file would silently
// wipe it.
func TestSnapshotPullKeepsOutOfManifestLocalFile(t *testing.T) {
	s, dataDir, key := newSnapshotTestService(t, []string{"connections.json", "quickCommands.json"})
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	writeConfig(t, dataDir, "quickCommands.json", `{"cmds":[1]}`)
	b := newFakeSnapshotBackend()

	// First sync pushes both files and records LastSyncedHash.
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}

	// Another device (narrower scope) overwrites the remote with only
	// connections.json — same content as before.
	staging := b.staging(t)
	srcDir := t.TempDir()
	writeConfig(t, srcDir, "connections.json", `{"v":1}`)
	if err := EncryptConfigFilesScoped([]string{"connections.json"}, srcDir, staging, key); err != nil {
		t.Fatal(err)
	}
	hashes, _, err := canonicalDirHashes([]string{"connections.json"}, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.StoreAll(staging, []byte("0123456789abcdef"), SnapshotInfo{
		Version: "v2", UpdatedAt: time.Now().UTC(), FileHashes: hashes,
	}); err != nil {
		t.Fatal(err)
	}

	// Local is unchanged; only the remote moved → silent pull, and the
	// out-of-manifest local file must survive.
	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncPull {
		t.Fatalf("direction = %v, want pull", res.Direction)
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "quickCommands.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"cmds":[1]}` {
		t.Fatalf("out-of-manifest local file wiped: %q", data)
	}
}

// TestSnapshotPushRaceDetected: another device overwrites the remote within
// the Head→StoreAll window. The push must surface the conflict dialog and
// must NOT record LastSyncedHash — otherwise the local changes would be
// silently lost.
func TestSnapshotPushRaceDetected(t *testing.T) {
	s, dataDir, _ := newSnapshotTestService(t, []string{"connections.json"})
	writeConfig(t, dataDir, "connections.json", `{"local":true}`)
	b := newFakeSnapshotBackend()
	b.raceAfterStore = true

	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncConflict || res.Conflict == nil {
		t.Fatalf("want conflict, got %+v", res)
	}
	cfg, _ := s.configStore.Load()
	if cfg.LastSyncedHash != "" {
		t.Fatalf("LastSyncedHash = %q, want empty after lost update", cfg.LastSyncedHash)
	}
}

func TestSnapshotOnlyLocalChangedPushes(t *testing.T) {
	s, dataDir, _ := newSnapshotTestService(t, []string{"connections.json"})
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	b := newFakeSnapshotBackend()
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}
	remoteHashBefore := aggregateHash(b.manifest.FileHashes)

	writeConfig(t, dataDir, "connections.json", `{"v":2}`)
	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncPush {
		t.Fatalf("direction = %v, want push", res.Direction)
	}
	if aggregateHash(b.manifest.FileHashes) == remoteHashBefore {
		t.Fatal("remote snapshot not updated after push")
	}
}

func TestSnapshotPasswordMismatch(t *testing.T) {
	s, _, _ := newSnapshotTestService(t, []string{"connections.json"})
	b := newFakeSnapshotBackend()

	// Remote ciphertext was encrypted under a different key (another
	// device with a different master password, or a corrupted blob).
	otherKey := []byte("ffffffffffffffffffffffffffffffff")
	staging := b.staging(t)
	srcDir := t.TempDir()
	writeConfig(t, srcDir, "connections.json", `{"v":9}`)
	if err := EncryptConfigFilesScoped([]string{"connections.json"}, srcDir, staging, otherKey); err != nil {
		t.Fatal(err)
	}
	hashes, _, err := canonicalDirHashes([]string{"connections.json"}, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.StoreAll(staging, []byte("0123456789abcdef"), SnapshotInfo{
		Version: "v1", UpdatedAt: time.Now().UTC(), FileHashes: hashes,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.syncSnapshot(b); err == nil {
		t.Fatal("want error for undecryptable remote ciphertext")
	}
	cfg, _ := s.configStore.Load()
	if cfg.LastSyncStatus != "password_mismatch" {
		t.Fatalf("LastSyncStatus = %q, want password_mismatch", cfg.LastSyncStatus)
	}
}

func TestSnapshotFirstJoinConflict(t *testing.T) {
	s, dataDir, key := newSnapshotTestService(t, []string{"connections.json"})
	writeConfig(t, dataDir, "connections.json", `{"local":true}`)
	b := newFakeSnapshotBackend()

	// First join with DIFFERENT remote content: no merge base exists and
	// the sides differ → conflict dialog, not a silent overwrite.
	staging := b.staging(t)
	srcDir := t.TempDir()
	writeConfig(t, srcDir, "connections.json", `{"remote":true}`)
	if err := EncryptConfigFilesScoped([]string{"connections.json"}, srcDir, staging, key); err != nil {
		t.Fatal(err)
	}
	hashes, _, err := canonicalDirHashes([]string{"connections.json"}, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.StoreAll(staging, []byte("0123456789abcdef"), SnapshotInfo{
		Version: "v1", UpdatedAt: time.Now().UTC(), FileHashes: hashes,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncConflict || res.Conflict == nil {
		t.Fatalf("want conflict, got %+v", res)
	}
}

// TestSnapshotResolveConflictUseLocal: both sides changed → the conflict
// dialog. useLocal must re-publish the local content while reusing the
// remote salt (repo identity), and useRemote must adopt the remote
// snapshot wholesale.
func TestSnapshotResolveConflictUseLocal(t *testing.T) {
	s, dataDir, key := newSnapshotTestService(t, []string{"connections.json"})
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	b := newFakeSnapshotBackend()
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}
	// Remote advanced by another device; local changed too → conflict state.
	other := t.TempDir()
	writeConfig(t, other, "connections.json", `{"remote":1}`)
	staging := b.staging(t) // shared staging dir for encrypt + StoreAll
	if err := EncryptConfigFilesScoped([]string{"connections.json"}, other, staging, key); err != nil {
		t.Fatal(err)
	}
	hashes, _, _ := canonicalDirHashes([]string{"connections.json"}, other)
	_ = b.StoreAll(staging, []byte("0123456789abcdef"), SnapshotInfo{Version: "v2", UpdatedAt: time.Now().UTC(), FileHashes: hashes})
	writeConfig(t, dataDir, "connections.json", `{"local":1}`)

	res, err := s.resolveConflictSnapshot(b, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncPush {
		t.Fatalf("want push, got %v", res.Direction)
	}
	cfg, _ := s.configStore.Load()
	if cfg.LastSyncedHash == "" {
		t.Fatal("hash must update after resolve")
	}
	// useLocal push must reuse the remote salt — the repo identity must
	// survive a conflict resolve.
	if got := string(b.store[saltName]); got != "0123456789abcdef" {
		t.Fatalf("salt after useLocal = %q, want remote salt preserved", got)
	}
	// The remote now carries the local content: the recorded hash must
	// equal the remote manifest's aggregate.
	if cfg.LastSyncedHash != aggregateHash(b.manifest.FileHashes) {
		t.Fatalf("LastSyncedHash %q != remote aggregate %q", cfg.LastSyncedHash, aggregateHash(b.manifest.FileHashes))
	}

	// Another device publishes again; this device resolves the new
	// conflict toward the remote.
	writeConfig(t, dataDir, "connections.json", `{"local":2}`)
	staging2 := b.staging(t)
	if err := EncryptConfigFilesScoped([]string{"connections.json"}, other, staging2, key); err != nil {
		t.Fatal(err)
	}
	if err := b.StoreAll(staging2, []byte("0123456789abcdef"), SnapshotInfo{Version: "v3", UpdatedAt: time.Now().UTC(), FileHashes: hashes}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolveConflictSnapshot(b, false); err != nil {
		t.Fatalf("resolve useRemote after useLocal should work, got %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dataDir, "connections.json"))
	if string(data) != `{"remote":1}` {
		t.Fatalf("useRemote content = %q", data)
	}
}

// TestSnapshotSaltReusedAcrossPushes: the remote salt is the repo identity;
// consecutive pushes must reuse it, not regenerate on every upload.
func TestSnapshotSaltReusedAcrossPushes(t *testing.T) {
	s, dataDir, _ := newSnapshotTestService(t, []string{"connections.json"})
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	b := newFakeSnapshotBackend()
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}
	salt1 := append([]byte(nil), b.store[saltName]...)

	writeConfig(t, dataDir, "connections.json", `{"v":2}`)
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}
	salt2 := append([]byte(nil), b.store[saltName]...)

	if !bytes.Equal(salt1, salt2) {
		t.Fatalf("salt changed across pushes: %x → %x", salt1, salt2)
	}
}

// newWebDAVConfigTestService builds a SyncService for ConfigureRepoWebDAV
// tests: temp config/data dirs (newScopeTestService — ConfigureRepoWebDAV
// writes sync-config.json, so the real UserConfigDir must not be touched)
// plus the snapshot tests' isolated keyring service.
func newWebDAVConfigTestService(t *testing.T) *SyncService {
	t.Helper()
	origService := keychainService
	keychainService = "uniTerm-test"
	t.Cleanup(func() { keychainService = origService })
	return newScopeTestService(t, t.TempDir(), t.TempDir())
}

// publishWebDAVSnapshot pre-populates the fake WebDAV remote with a
// one-file snapshot encrypted under DeriveKey(masterPassword, salt).
func publishWebDAVSnapshot(t *testing.T, f *fakeDAV, masterPassword string) SnapshotBackend {
	t.Helper()
	salt := []byte("0123456789abcdef")
	key := DeriveKey(masterPassword, salt)
	staging := t.TempDir()
	srcDir := t.TempDir()
	writeConfig(t, srcDir, "connections.json", `{"remote":1}`)
	if err := EncryptConfigFilesScoped([]string{"connections.json"}, srcDir, staging, key); err != nil {
		t.Fatal(err)
	}
	hashes, _, err := canonicalDirHashes([]string{"connections.json"}, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	b := NewWebDAVBackend(f.server.URL, "sync", "u", "p", []string{"connections.json"})
	if err := b.Probe(); err != nil {
		t.Fatal(err)
	}
	if err := b.StoreAll(staging, salt, SnapshotInfo{Version: "v1", UpdatedAt: time.Now().UTC(), FileHashes: hashes}); err != nil {
		t.Fatal(err)
	}
	return b
}

// TestConfigureRepoWebDAVFreshRemote: a brand-new WebDAV share is
// provisioned from the local config, and the saved config keeps the
// device-local AutoSync/SyncScope (Part A regression) while gaining the
// webdav connection fields and the agreed content hash.
func TestConfigureRepoWebDAVFreshRemote(t *testing.T) {
	s := newWebDAVConfigTestService(t)
	scope := []string{"connections.json"}
	if err := s.configStore.Save(SyncConfig{Branch: "main", AutoSync: true, SyncScope: scope}); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, s.dataDir, "connections.json", `{"v":1}`)

	f := newFakeDAV(t)
	res, err := s.ConfigureRepoRemote("webdav", RemoteConfig{Server: f.server.URL, Path: "sync", User: "u"}, "p", "master-pw")
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncPush {
		t.Fatalf("direction = %v, want push", res.Direction)
	}
	if f.get("sync/manifest.json") == nil || f.get("sync/.sync-salt") == nil {
		t.Fatal("remote must carry manifest and salt after configure")
	}
	cfg, err := s.configStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend != "webdav" || cfg.WebDAVServer != f.server.URL || cfg.WebDAVPath != "sync" || cfg.WebDAVUser != "u" {
		t.Fatalf("config = %+v", cfg)
	}
	if !cfg.AutoSync {
		t.Error("AutoSync = false, want preserved true")
	}
	if !reflect.DeepEqual(cfg.SyncScope, scope) {
		t.Errorf("SyncScope = %v, want preserved %v", cfg.SyncScope, scope)
	}
	if cfg.LastSyncedHash == "" {
		t.Fatal("LastSyncedHash must be recorded")
	}
	// The stored key must be derived from the master password + remote salt.
	salt, err := ReadSaltFile(s.repoPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.keychain.GetEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	if want := DeriveKey("master-pw", salt); !bytes.Equal(got, want) {
		t.Fatal("stored encryption key does not match master password + salt")
	}
}

// TestConfigureRepoWebDAVAdoptsRemote: joining an existing WebDAV share
// with an empty local data dir adopts the remote snapshot wholesale.
func TestConfigureRepoWebDAVAdoptsRemote(t *testing.T) {
	s := newWebDAVConfigTestService(t) // dataDir stays empty
	f := newFakeDAV(t)
	publishWebDAVSnapshot(t, f, "master-pw")

	res, err := s.ConfigureRepoRemote("webdav", RemoteConfig{Server: f.server.URL, Path: "sync", User: "u"}, "p", "master-pw")
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncPull {
		t.Fatalf("direction = %v, want pull", res.Direction)
	}
	data, err := os.ReadFile(filepath.Join(s.dataDir, "connections.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"remote":1}` {
		t.Fatalf("adopted content = %q", data)
	}
	cfg, _ := s.configStore.Load()
	if cfg.Backend != "webdav" || cfg.LastSyncedHash == "" {
		t.Fatalf("config after adopt = %+v", cfg)
	}
}

// TestConfigureRepoWebDAVWrongMasterPassword: a master password that
// cannot open the existing remote ciphertext must fail with the
// master_password_mismatch code and must NOT switch the config to webdav.
func TestConfigureRepoWebDAVWrongMasterPassword(t *testing.T) {
	s := newWebDAVConfigTestService(t)
	if err := s.configStore.Save(SyncConfig{Branch: "main", AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, s.dataDir, "connections.json", `{"local":1}`)

	f := newFakeDAV(t)
	publishWebDAVSnapshot(t, f, "master-pw")

	if _, err := s.ConfigureRepoRemote("webdav", RemoteConfig{Server: f.server.URL, Path: "sync", User: "u"}, "p", "wrong-pw"); err == nil {
		t.Fatal("want error for wrong master password")
	} else if !strings.Contains(err.Error(), "master_password_mismatch") {
		t.Fatalf("err = %v, want master_password_mismatch", err)
	}
	cfg, _ := s.configStore.Load()
	if cfg.Backend == "webdav" {
		t.Fatalf("backend switched despite failure: %+v", cfg)
	}
}

// TestIsAutoSyncEnabledCoversWebDAV: the auto-sync gate must recognize a
// configured WebDAV backend, not only a git repo URL — otherwise the
// AutoSync carried over by ConfigureRepoWebDAV silently does nothing.
func TestIsAutoSyncEnabledCoversWebDAV(t *testing.T) {
	cases := []struct {
		name string
		cfg  SyncConfig
		want bool
	}{
		{"git configured", SyncConfig{AutoSync: true, RepoURL: "https://example.com/r.git"}, true},
		{"git without url", SyncConfig{AutoSync: true}, false},
		{"webdav configured", SyncConfig{AutoSync: true, Backend: "webdav", WebDAVServer: "https://dav.example.com"}, true},
		{"webdav without server", SyncConfig{AutoSync: true, Backend: "webdav"}, false},
		{"autosync off", SyncConfig{RepoURL: "https://example.com/r.git"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newScopeTestService(t, t.TempDir(), t.TempDir())
			if err := s.configStore.Save(tc.cfg); err != nil {
				t.Fatal(err)
			}
			if got := s.IsAutoSyncEnabled(); got != tc.want {
				t.Fatalf("IsAutoSyncEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// snapshotBlobStore is a minimal SyncableStore fake backed by a plain file
// in dir. ExportPlain reads the backing blob — the raw os error already
// wraps fs.ErrNotExist when the file was never written, so sync skips the
// store instead of uploading an empty document that would clobber the
// remote copy — and ImportPlain persists through a plain write, mirroring
// the contract app.go's assembly satisfies structurally: sync holds only
// interface values and never imports store (sync → store → credentials →
// sync would cycle), so the test fake lives on the consumer side too.
type snapshotBlobStore struct {
	name string
	dir  string
}

func (f *snapshotBlobStore) BlobName() string { return f.name }

func (f *snapshotBlobStore) ExportPlain() ([]byte, error) {
	return os.ReadFile(filepath.Join(f.dir, f.name))
}

func (f *snapshotBlobStore) ImportPlain(plain []byte) error {
	return os.WriteFile(filepath.Join(f.dir, f.name), plain, 0600)
}

// TestSyncSkipsNeverPersistedStore: a device that has only ever written two
// of the scoped files must still publish a snapshot — the manifest records
// the never-persisted stores with the null hash and carries no blob for them,
// so neither StoreAll nor FetchAll may demand one. A second device with the
// same content must then recognise the snapshot as identical.
func TestSyncSkipsNeverPersistedStore(t *testing.T) {
	s, dataDir, _ := newSnapshotTestService(t, nil)
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	writeConfig(t, dataDir, "favorites.json", `["c1"]`)

	b := newFakeSnapshotBackend()
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatalf("push with never-persisted stores: %v", err)
	}
	for _, name := range []string{"proxies.json", "tunnels.json", "identities.json"} {
		if _, ok := b.store[name]; ok {
			t.Errorf("never-persisted %s must not be uploaded", name)
		}
	}
	if b.manifest.FileHashes["proxies.json"] == "" {
		t.Error("the manifest must still record the null hash for hash comparison")
	}

	// Same content on a second device: the aggregate hashes agree, so the
	// snapshot must be recognised rather than reported as a conflict.
	s2, dataDir2, _ := newSnapshotTestService(t, nil)
	writeConfig(t, dataDir2, "connections.json", `{"v":1}`)
	writeConfig(t, dataDir2, "favorites.json", `["c1"]`)
	res, err := s2.syncSnapshot(b)
	if err != nil {
		t.Fatalf("second device sync: %v", err)
	}
	if res.Direction == SyncConflict {
		t.Fatal("identical content must not surface a conflict")
	}
}

// TestPushSnapshotUploadsOnlyChangedBlobs pins the per-file granularity: the
// per-blob encryption nonce is random, so re-encrypting unchanged content
// yields different bytes — without the change-detection skip, a one-file edit
// re-uploaded the whole config.
func TestPushSnapshotUploadsOnlyChangedBlobs(t *testing.T) {
	s, dataDir, _ := newSnapshotTestService(t, nil)
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	writeConfig(t, dataDir, "favorites.json", `["c1"]`)

	b := newFakeSnapshotBackend()
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatalf("initial push: %v", err)
	}
	oldFavorites := b.store["favorites.json"]

	writeConfig(t, dataDir, "connections.json", `{"v":2}`)
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatalf("incremental push: %v", err)
	}
	if string(b.store["favorites.json"]) != string(oldFavorites) {
		t.Fatal("unchanged blob was re-encrypted and re-uploaded")
	}
	if string(b.store["connections.json"]) == string(b.store["favorites.json"]) {
		t.Fatal("changed blob must have been re-encrypted")
	}

	// The manifest still describes the whole scope, so a third device
	// pulling gets every file.
	if len(b.manifest.FileHashes) != len(defaultSyncedFiles) {
		t.Fatalf("manifest hashes = %d entries, want %d", len(b.manifest.FileHashes), len(defaultSyncedFiles))
	}
}

// TestSnapshotPushKeepsOutOfScopeRemoteBlob: device A (wide scope incl.
// skills.json) pushes; device B (narrow scope, no skills.json) then pushes
// its own change. B's manifest must keep A's skills.json hash so a third
// pull still delivers the blob — the scope is per-device, not per-repo
// (issue #1517).
func TestSnapshotPushKeepsOutOfScopeRemoteBlob(t *testing.T) {
	s, dataDir, key := newSnapshotTestService(t, []string{"connections.json"})
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	b := newFakeSnapshotBackend()

	// Device B bootstrap: narrow-scope push, records LastSyncedHash.
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}

	// Device A (wide scope incl. skills.json) pushes on top: same
	// connections.json content, plus the skills.json blob.
	staging := b.staging(t)
	srcDir := t.TempDir()
	writeConfig(t, srcDir, "connections.json", `{"v":1}`)
	writeConfig(t, srcDir, "skills.json", `{"skills":[{"name":"demo"}]}`)
	wide := []string{"connections.json", "skills.json"}
	if err := EncryptConfigFilesScoped(wide, srcDir, staging, key); err != nil {
		t.Fatal(err)
	}
	hashes, _, err := canonicalDirHashes(wide, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	skillHash := hashes["skills.json"]
	if err := b.StoreAll(staging, []byte("0123456789abcdef"), SnapshotInfo{
		Version: "v2", UpdatedAt: time.Now().UTC(), FileHashes: hashes,
	}); err != nil {
		t.Fatal(err)
	}

	// B syncs: the wide-scope remote entry is invisible to B's narrower
	// scope — over the shared scope nothing moved, so B stays "up to
	// date" without a pull. The skills.json blob stays on the remote for
	// the devices whose scope covers it.
	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncNone {
		t.Fatalf("direction = %v, want none", res.Direction)
	}

	// B edits its scoped file and pushes.
	writeConfig(t, dataDir, "connections.json", `{"v":2}`)
	res, err = s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncPush {
		t.Fatalf("direction = %v, want push", res.Direction)
	}
	if b.manifest.FileHashes["skills.json"] != skillHash {
		t.Fatalf("push dropped out-of-scope remote skills.json hash: got %q want %q",
			b.manifest.FileHashes["skills.json"], skillHash)
	}
	if _, ok := b.store["skills.json"]; !ok {
		t.Fatal("push deleted the out-of-scope remote skills.json blob")
	}
}

// TestSnapshotWidenedScopePullsOutOfScopeBlob: after B widens its scope to
// cover skills.json, the blob A left on the remote must arrive — the scoped
// comparison must not have recorded an anchor that hides it (issue #1517).
func TestSnapshotWidenedScopePullsOutOfScopeBlob(t *testing.T) {
	s, dataDir, key := newSnapshotTestService(t, []string{"connections.json"})
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	b := newFakeSnapshotBackend()

	// B bootstrap push (narrow scope).
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}

	// Device A (wide scope) pushes connections.json + skills.json.
	staging := b.staging(t)
	srcDir := t.TempDir()
	writeConfig(t, srcDir, "connections.json", `{"v":1}`)
	writeConfig(t, srcDir, "skills.json", `{"skills":[{"name":"demo"}]}`)
	wide := []string{"connections.json", "skills.json"}
	if err := EncryptConfigFilesScoped(wide, srcDir, staging, key); err != nil {
		t.Fatal(err)
	}
	hashes, _, err := canonicalDirHashes(wide, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.StoreAll(staging, []byte("0123456789abcdef"), SnapshotInfo{
		Version: "v2", UpdatedAt: time.Now().UTC(), FileHashes: hashes,
	}); err != nil {
		t.Fatal(err)
	}

	// B syncs (no-op: out-of-scope entry invisible), then widens its scope.
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}
	cfg, _ := s.configStore.Load()
	cfg.SyncScope = wide
	if err := s.configStore.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// The skills.json blob is not in B's data dir yet, and B's local store
	// for it reads as never-persisted. Over the widened scope the remote
	// moved while local == last (scoped to the old scope) → pull must
	// deliver the blob, not a conflict.
	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "skills.json"))
	if err != nil {
		t.Fatalf("skills.json must be pulled on widened scope: %v (direction=%v)", err, res.Direction)
	}
	if string(data) != `{"skills":[{"name":"demo"}]}` {
		t.Fatalf("pulled skills.json = %q", data)
	}
}

// TestSnapshotNeverPersistedScopedFileStillConflicts: a scoped file that
// was never persisted locally (null hash) while the scope itself never
// changed must NOT be mistaken for a scope widening — a genuine
// both-sides change still surfaces the conflict dialog.
func TestSnapshotNeverPersistedScopedFileStillConflicts(t *testing.T) {
	s, dataDir, key := newSnapshotTestService(t, []string{"connections.json", "skills.json"})
	// Local has connections only; skills.json never written.
	writeConfig(t, dataDir, "connections.json", `{"v":1}`)
	b := newFakeSnapshotBackend()

	// Bootstrap: the manifest covers both files (skills = null hash).
	if _, err := s.syncSnapshot(b); err != nil {
		t.Fatal(err)
	}

	// Local edits connections; remote (another device) rewrites
	// connections differently and now carries skills.json.
	writeConfig(t, dataDir, "connections.json", `{"v":2}`)
	staging := b.staging(t)
	srcDir := t.TempDir()
	writeConfig(t, srcDir, "connections.json", `{"v":3}`)
	writeConfig(t, srcDir, "skills.json", `{"skills":[{"name":"demo"}]}`)
	wide := []string{"connections.json", "skills.json"}
	if err := EncryptConfigFilesScoped(wide, srcDir, staging, key); err != nil {
		t.Fatal(err)
	}
	hashes, _, err := canonicalDirHashes(wide, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.StoreAll(staging, []byte("0123456789abcdef"), SnapshotInfo{
		Version: "v2", UpdatedAt: time.Now().UTC(), FileHashes: hashes,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := s.syncSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Direction != SyncConflict {
		t.Fatalf("direction = %v, want conflict", res.Direction)
	}
	// The local edit must survive the conflict round.
	data, err := os.ReadFile(filepath.Join(dataDir, "connections.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"v":2}` {
		t.Fatalf("local edit overwritten by conflict round: %q", data)
	}
}
