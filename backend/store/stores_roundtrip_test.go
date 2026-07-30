package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ys-ll/uniterm/backend/session"
)

// QA-024: 6 stores had no direct Save/Load round-trip tests.
// Each TestStore_RoundTrip writes a representative payload, saves,
// reloads through a fresh store, and asserts the payload survived.

// --- ConnectionStore ---
//
// ConnectionStore uses os.UserConfigDir; we test it through a
// smaller shim by exercising SetPasswordStore + Save/Load directly
// against a known tempdir via a freshly-constructed store. To avoid
// touching the user's real configDir, we only validate behavior at
// the file level by injecting one via a custom *ConnectionStore
// pattern. Because NewConnectionStore hardcodes UserConfigDir, this
// test instead validates the file format and PasswordStore behavior
// at the JSON level.

func TestStore_ConnectionStore_RoundTrip(t *testing.T) {
	// Save then load round-trip via the on-disk JSON format the
	// store writes.  We construct two ConnectionStores pointing to
	// the same path (using a custom helper that swaps configDir).
	dir := t.TempDir()
	fkc := newFakeKeychainHelper()
	s1 := makeConnStoreForTest(dir)
	s1.SetPasswordStore(fkc)
	data := session.ConnectionStoreData{
		Groups: []session.ConnectionGroup{{ID: "g1", Name: "prod"}},
		Connections: []session.ConnectionConfig{
			{ID: "c1", Host: "host.example", Port: 22, User: "u", AuthType: "key"},
			{ID: "c2", Host: "host2.example", Port: 22, User: "u", AuthType: "password", Password: "secret"},
		},
	}
	if err := s1.Save(data); err != nil {
		t.Fatal(err)
	}
	s2 := makeConnStoreForTest(dir)
	s2.SetPasswordStore(fkc)
	got, err := s2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Connections) != 2 || got.Connections[0].Host != "host.example" {
		t.Fatalf("connections: %+v", got.Connections)
	}
	if got.Groups[0].Name != "prod" {
		t.Fatalf("groups: %+v", got.Groups)
	}
	// Password should have been exported to the fake keychain.
	if pw := fkc.passwords["c2"]; pw != "secret" {
		t.Errorf("expected keychain password 'secret', got %q", pw)
	}
}

// Password auto-exported to keychain on save — without a
// PasswordStore the Save should fail closed (STORE-04).  We
// confirm that path here as a separate sub-test.
func TestStore_ConnectionStore_RefusesPlaintextWithoutPasswordStore(t *testing.T) {
	dir := t.TempDir()
	s := makeConnStoreForTest(dir)
	data := session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", Host: "h", Port: 22, AuthType: "password", Password: "secret"},
		},
	}
	if err := s.Save(data); err == nil {
		t.Fatal("Save without PasswordStore must fail closed")
	}
}

// newFakeKeychainHelper is a minimal in-memory PasswordStore.
type fakeKeychainHelper struct {
	passwords map[string]string
	apiKeys   map[string]string
}

func newFakeKeychainHelper() *fakeKeychainHelper {
	return &fakeKeychainHelper{
		passwords: map[string]string{},
		apiKeys:   map[string]string{},
	}
}

func (f *fakeKeychainHelper) GetPassword(id string) (string, error) {
	return f.passwords[id], nil
}
func (f *fakeKeychainHelper) SetPassword(id, pw string) error {
	f.passwords[id] = pw
	return nil
}
func (f *fakeKeychainHelper) DeletePassword(id string) error {
	delete(f.passwords, id)
	return nil
}
func (f *fakeKeychainHelper) GetModelAPIKey(id string) (string, error) {
	return f.apiKeys[id], nil
}
func (f *fakeKeychainHelper) SetModelAPIKey(id, k string) error {
	f.apiKeys[id] = k
	return nil
}
func (f *fakeKeychainHelper) DeleteModelAPIKey(id string) error {
	delete(f.apiKeys, id)
	return nil
}

// --- TunnelStore ---

func TestStore_TunnelStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewTunnelStore(dir)
	data := session.TunnelStoreData{
		Version: 1,
		Groups: []session.TunnelGroup{
			{ID: "g1", Name: "all"},
		},
		Tunnels: []session.Tunnel{
			{ID: "t1", Mode: session.TunnelLocal, ListenHost: "127.0.0.1", ListenPort: 8080, TargetHost: "127.0.0.1", TargetPort: 80},
			{ID: "t2", Mode: session.TunnelRemote, ListenHost: "127.0.0.1", ListenPort: 9090, TargetHost: "127.0.0.1", TargetPort: 90},
		},
	}
	if err := s.Save(data); err != nil {
		t.Fatal(err)
	}

	s2 := NewTunnelStore(dir)
	got, err := s2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || len(got.Tunnels) != 2 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.Tunnels[0].Mode != session.TunnelLocal || got.Tunnels[1].Mode != session.TunnelRemote {
		t.Errorf("modes: %+v", got.Tunnels)
	}
}

// --- QuickCommandsStore ---

func TestStore_QuickCommandsStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewQuickCommandsStore(dir)
	data := QuickCommandData{
		Version: 1,
		Groups: []QuickCommandGroup{
			{ID: "g1", Name: "shell"},
		},
		Commands: []QuickCommand{
			{ID: "q1", Name: "ll", Command: "ls -la", GroupID: "g1", SortOrder: 0},
			{ID: "q2", Name: "tree", Command: "tree -L 2", GroupID: "g1", SortOrder: 1},
		},
	}
	if err := s.Save(data); err != nil {
		t.Fatal(err)
	}
	s2 := NewQuickCommandsStore(dir)
	got, err := s2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || len(got.Commands) != 2 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.Commands[0].Command != "ls -la" {
		t.Errorf("first command mismatch: %+v", got.Commands[0])
	}

	// Missing file → defaults.
	s3 := NewQuickCommandsStore(t.TempDir())
	got, err = s3.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || len(got.Commands) != 0 {
		t.Errorf("fresh load should return defaults, got %+v", got)
	}
}

// --- LocalStateStore ---

func TestStore_LocalStateStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewLocalStateStore(dir)
	state := LocalState{
		SidebarVisible:    false,
		AISidebarVisible:  true,
		WindowX:           100,
		WindowY:           200,
		WindowWidth:       800,
		WindowHeight:      600,
		BackgroundEnabled: true,
		BackgroundImage:   "/path/to/img.png",
		BackgroundOpacity: 75,
		BackgroundBlur:    8,
		BackgroundFit:     "contain",
	}
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	s2 := NewLocalStateStore(dir)
	got, err := s2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.WindowX != 100 || got.WindowHeight != 600 || got.BackgroundOpacity != 75 {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	// Missing file → defaults.
	s3 := NewLocalStateStore(t.TempDir())
	def, err := s3.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !def.SidebarVisible {
		t.Errorf("default SidebarVisible=true, got %+v", def)
	}
}

// --- CommandsStore ---

func TestStore_CommandsStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewCommandsStore(dir)

	if err := s.CreateCommand("foo", "do the foo thing", "<args>", "echo $1\n# body"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateCommand("bar", "do the bar thing", "", "# bar body"); err != nil {
		t.Fatal(err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(list))
	}

	// Find "foo" via List (sorted by sortOrder then name).
	var foo *CommandMeta
	for i := range list {
		if list[i].Name == "foo" {
			foo = &list[i]
			break
		}
	}
	if foo == nil {
		t.Fatal("foo missing from List()")
	}
	if foo.Description != "do the foo thing" || foo.ArgumentHint != "<args>" {
		t.Errorf("foo = %+v", foo)
	}

	// GetBody round-trip — should return body WITHOUT frontmatter.
	body, err := s.GetBody("foo")
	if err != nil {
		t.Fatal(err)
	}
	if body != "echo $1\n# body" {
		t.Errorf("body = %q", body)
	}
}

// --- AISessionStore ---

func TestStore_AISessionStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := newAISessionStoreForTest(dir)
	data := AISessionData{
		Sessions: []AISessionEntry{
			{
				ID:        "sess-1",
				Name:      "first session",
				CreatedAt: 1700000000,
				UpdatedAt: 1700000100,
				Messages: []AIMessageEntry{
					{ID: "m1", Role: "user", Content: "hello"},
					{ID: "m2", Role: "assistant", Content: "world"},
				},
			},
			{
				ID:        "sess-2",
				Name:      "second session",
				CreatedAt: 1700000200,
				UpdatedAt: 1700000300,
				Messages: []AIMessageEntry{
					{ID: "m3", Role: "user", Content: "ping"},
				},
			},
		},
		CurrentSessionID: "sess-2",
	}
	if err := s.Save(data); err != nil {
		t.Fatal(err)
	}
	s2 := newAISessionStoreForTest(dir)
	got, err := s2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got.Sessions))
	}
	if got.Sessions[0].Name != "first session" || len(got.Sessions[0].Messages) != 2 {
		t.Errorf("session 1 mismatch: %+v", got.Sessions[0])
	}

	// Orphan shard cleanup: re-save with a removed session — the
	// shard for the removed one should be deleted.
	data.Sessions = data.Sessions[:1]
	if err := s2.Save(data); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, aiSessionDirName))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 shard after orphan cleanup, got %d", count)
	}
}

// --- helpers ---

// makeConnStoreForTest creates a ConnectionStore pinned to dir
// instead of the user-level config dir.  Avoids touching the
// real ~/.config/uniTerm/ on the test machine.
func makeConnStoreForTest(dir string) *ConnectionStore {
	return &ConnectionStore{configDir: dir}
}

// newAISessionStoreForTest is the same trick for AISessionStore.
func newAISessionStoreForTest(dir string) *AISessionStore {
	return &AISessionStore{configDir: dir}
}
