package store

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
"github.com/ys-ll/uniterm/backend/session"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
"sync/atomic"
	"testing"
"time"
)

// ensure all manifest files resolve their paths against a representative
// document shape for that file.
func TestSecretFilesManifestCoversRealShapes(t *testing.T) {
	shapes := map[string]map[string]interface{}{
		"connections.json": {
			"groups": []interface{}{},
			"connections": []interface{}{
				map[string]interface{}{
					"id": "c1", "authType": "key",
					"password":   "key-passphrase",
					"keyContent": "",
				},
			},
		},
		"identities.json": {
			"identities": []interface{}{
				map[string]interface{}{"id": "i1", "password": "pw", "keyContent": "k"},
			},
		},
		"proxies.json": {
			"proxies": []interface{}{
				map[string]interface{}{"id": "p1", "pass": "pp"},
			},
		},
		"ai.json": {
			"models": []interface{}{
				map[string]interface{}{"id": "m1", "apiKey": "sk"},
			},
		},
		"settings.json": {
			"ai": map[string]interface{}{
				"models": []interface{}{
					map[string]interface{}{"id": "m1", "apiKey": "sk"},
				},
			},
		},
	}
	for file, obj := range shapes {
		if err := EncryptSecretFields(file, &obj, fakeCipherStore{}); err != nil {
			t.Fatalf("%s: EncryptSecretFields: %v", file, err)
		}
		if !ObjHasEncryptedSecrets(file, obj) {
			t.Fatalf("%s: expected enc:v1 residue after encrypt", file)
		}
		res, err := DecryptSecretFields(file, &obj, fakeCipherStore{})
		if err != nil {
			t.Fatalf("%s: DecryptSecretFields: %v", file, err)
		}
		if res.Decrypted == 0 {
			t.Fatalf("%s: expected at least one decrypted field", file)
		}
		if ObjHasEncryptedSecrets(file, obj) {
			t.Fatalf("%s: unexpected enc:v1 residue after decrypt", file)
		}
	}
}

func TestEncryptSecretFieldsFailsClosedWithoutCipher(t *testing.T) {
	obj := map[string]interface{}{
		"connections": []interface{}{
			map[string]interface{}{"id": "c1", "password": "secret"},
		},
	}
	if err := EncryptSecretFields("connections.json", &obj, nil); !errors.Is(err, ErrNoCipher) {
		t.Fatalf("expected ErrNoCipher, got %v", err)
	}
}

func TestEncryptSecretFieldsSkipsEmptyAndAlreadyEncrypted(t *testing.T) {
	obj := map[string]interface{}{
		"connections": []interface{}{
			map[string]interface{}{"id": "c1", "password": ""},
			map[string]interface{}{"id": "c2", "password": "enc:v1:already"},
		},
	}
	if err := EncryptSecretFields("connections.json", &obj, fakeCipherStore{}); err != nil {
		t.Fatal(err)
	}
	if obj["connections"].([]interface{})[1].(map[string]interface{})["password"] != "enc:v1:already" {
		t.Fatal("already-encrypted value must not be double-encrypted")
	}
}

func TestDecryptSecretFieldsCountsPlaintextForLazyMigration(t *testing.T) {
	obj := map[string]interface{}{
		"connections": []interface{}{
			map[string]interface{}{"id": "c1", "password": "enc:v1:" + base64.StdEncoding.EncodeToString([]byte("aaa"))},
			map[string]interface{}{"id": "c2", "password": "legacy-plaintext"},
		},
	}
	res, err := DecryptSecretFields("connections.json", &obj, fakeCipherStore{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Decrypted != 1 || res.Plaintext != 1 {
		t.Fatalf("got %+v, want Decrypted=1 Plaintext=1", res)
	}
}

func TestClearSecretFieldsBlanksAllDeclaredPaths(t *testing.T) {
	obj := map[string]interface{}{
		"groups": []interface{}{},
		"connections": []interface{}{
			map[string]interface{}{"id": "c1", "password": "enc:v1:aaa", "keyContent": "enc:v1:bbb"},
		},
		"identities": []interface{}{
			map[string]interface{}{"id": "i1", "password": "enc:v1:ccc", "keyContent": "enc:v1:ddd"},
		},
	}
	// Applying every manifest file's paths to one object: the connections
	// paths clear the connection fields, the identities paths the identity
	// fields, and everything else no-ops.
	for _, sf := range SecretFiles {
		if err := ClearSecretFields(sf.File, obj); err != nil {
			t.Fatal(err)
		}
	}
	cm := obj["connections"].([]interface{})[0].(map[string]interface{})
	if cm["password"] != "" || cm["keyContent"] != "" {
		t.Fatalf("connection secrets not cleared: %v", cm)
	}
	im := obj["identities"].([]interface{})[0].(map[string]interface{})
	if im["password"] != "" || im["keyContent"] != "" {
		t.Fatalf("identity secrets not cleared: %v", im)
	}
}

func TestClearSecretFieldsClearsKeyContentToo(t *testing.T) {
	// Regression for the #1115 reset dead-end: clearSecretsInFile used to
	// miss connections[].keyContent, leaving residue that made Setup refuse
	// with "existing encrypted data" after a reset.
	obj := map[string]interface{}{
		"connections": []interface{}{
			map[string]interface{}{"id": "c1", "keyContent": "enc:v1:residue"},
		},
	}
	if err := ClearSecretFields("connections.json", obj); err != nil {
		t.Fatal(err)
	}
	cm := obj["connections"].([]interface{})[0].(map[string]interface{})
	if cm["keyContent"] != "" {
		t.Fatalf("keyContent not cleared: %v", cm)
	}
}

func TestDecryptStopsAtDecryptError(t *testing.T) {
	obj := map[string]interface{}{
		"connections": []interface{}{
			map[string]interface{}{"id": "c1", "password": "enc:v1:not-base64!!"},
		},
	}
	fc := fakeCipherStore{}
	if _, err := DecryptSecretFields("connections.json", &obj, fc); err == nil {
		t.Fatal("expected decrypt error for malformed ciphertext")
	}
}

func TestFieldRefSkipsNonStringLeaves(t *testing.T) {
	obj := map[string]interface{}{
		"connections": []interface{}{
			map[string]interface{}{"id": "c1", "port": 22, "password": "pw"},
		},
	}
	var refs []fieldRef
	collectFieldRefs(obj, strings.Split("connections.*.password", "."), &refs)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	// A path whose final segment lands on a non-string is not addressed.
	var none []fieldRef
	collectFieldRefs(obj, strings.Split("connections.*.port", "."), &none)
	if len(none) != 0 {
		t.Fatalf("expected 0 refs for non-string leaf, got %d", len(none))
	}
}

func TestUnknownFileIsNoOp(t *testing.T) {
	obj := map[string]interface{}{"password": "pw"}
	if err := EncryptSecretFields("unknown.json", &obj, fakeCipherStore{}); err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
	if res, err := DecryptSecretFields("unknown.json", &obj, fakeCipherStore{}); err != nil || res.Decrypted != 0 {
		t.Fatalf("expected no-op, got %+v err=%v", res, err)
	}
	if err := ClearSecretFields("unknown.json", obj); err != nil {
		t.Fatal(err)
	}
	if ObjHasEncryptedSecrets("unknown.json", obj) {
		t.Fatal("expected false for unknown file")
	}
}

// syncable mirrors sync.SyncableStore structurally. The interface lives in
// backend/sync (consumer side) because store → credentials → sync makes
// sync → store an import cycle; this local mirror plus the slice literals
// below are the compile-time assertion that every syncable store keeps
// satisfying it.
type syncable interface {
	BlobName() string
	ExportPlain() ([]byte, error)
	ImportPlain([]byte) error
}

// allSyncableStores is every store that participates in config exchange.
func allSyncableStores(t *testing.T) []syncable {
	t.Helper()
	return []syncable{
		mustConnStore(t), mustIdentityStore(t), mustProxyStore(t), mustAIStore(t),
		NewFavoriteStore(t.TempDir()), NewQuickCommandsStore(t.TempDir()),
		NewNotesStore(t.TempDir()),
		NewTunnelStore(t.TempDir()), mustSettingsStore(t),
		NewSkillsStore(t.TempDir()), NewCommandsStore(t.TempDir()),
		NewMCPClientStore(t.TempDir()),
	}
}

// TestSyncableExportPlainSkipsNeverPersistedStore locks in the sync-blob
// contract: a store whose backing file was never written exports nothing —
// the sync layer must skip it rather than upload an empty blob that would
// clobber the remote copy (favorites.json wipe class of bugs).
func TestSyncableExportPlainSkipsNeverPersistedStore(t *testing.T) {
	for _, s := range allSyncableStores(t) {
		_, err := s.ExportPlain()
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: ExportPlain on never-persisted store = %v, want fs.ErrNotExist", s.BlobName(), err)
		}
	}
}

// TestSyncableBlobNames pins the remote blob names — the registry keys and
// the git/WebDAV remote file names must not drift.
func TestSyncableBlobNames(t *testing.T) {
	want := map[string]string{
		"connections.json":   "connections.json",
		"identities.json":    "identities.json",
		"proxies.json":       "proxies.json",
		"ai.json":            "ai.json",
		"favorites.json":     "favorites.json",
		"quickCommands.json": "quickCommands.json",
		"notes.json":         "notes.json",
		"tunnels.json":       "tunnels.json",
		"settings.json":      "settings.json",
		"skills.json":        "skills.json",
		"commands.json":      "commands.json",
		"mcp_clients.json":   "mcp_clients.json",
	}
	for _, s := range allSyncableStores(t) {
		if s.BlobName() != want[s.BlobName()] {
			t.Errorf("BlobName %q not in the synced set", s.BlobName())
		}
		if !strings.HasSuffix(s.BlobName(), ".json") {
			t.Errorf("BlobName %q must be a .json blob", s.BlobName())
		}
	}
}

// TestClearSecretFieldsBlanksEveryManifestField is the reset-path guard: the
// credential reset blanks secret fields WITHOUT decrypting (the vault is gone
// by then), and it must cover every manifest-declared field of every
// manifest-declared file — a missed field leaves ciphertext behind and wedges
// the next Setup, which refuses a data dir that still carries it.
func TestClearSecretFieldsBlanksEveryManifestField(t *testing.T) {
	dir := t.TempDir()
	seed := map[string]string{
		"connections.json": `{"name":"keepme","connections":[{"id":"c1","password":"enc:v1:aaa","keyContent":"enc:v1:bbb"},{"id":"c2","password":"enc:v1:ccc","keyContent":"enc:v1:ddd"}]}`,
		"identities.json":  `{"identities":[{"id":"i1","password":"enc:v1:eee","keyContent":"enc:v1:fff"}]}`,
		"proxies.json":     `{"proxies":[{"id":"p1","pass":"enc:v1:ggg"}]}`,
		"ai.json":          `{"models":[{"id":"m1","apiKey":"enc:v1:hhh"}]}`,
		"settings.json":    `{"theme":"dark","ai":{"models":[{"id":"m2","apiKey":"enc:v1:iii"}]}}`,
	}
	for name, body := range seed {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}

	ClearAllSecretFields(dir)

	for name := range seed {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("enc:v1:")) {
			t.Errorf("%s still carries ciphertext after clear: %s", name, data)
		}
	}
	// Non-secret fields must survive untouched.
	raw, err := os.ReadFile(filepath.Join(dir, "connections.json"))
	if err != nil {
		t.Fatal(err)
	}
	var conn struct {
		Name        string `json:"name"`
		Connections []struct {
			ID       string `json:"id"`
			Password string `json:"password"`
		} `json:"connections"`
	}
	if err := json.Unmarshal(raw, &conn); err != nil {
		t.Fatal(err)
	}
	if conn.Name != "keepme" || len(conn.Connections) != 2 || conn.Connections[0].Password != "" {
		t.Fatalf("unexpected cleared connections.json: %s", raw)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "settings.json"))
	var settings struct {
		Theme string `json:"theme"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Theme != "dark" {
		t.Fatalf("non-secret settings field lost: %s", raw)
	}
}


// TestAtomicWrite_NotPartialOnCrash simulates a kill -9 mid-save by calling
// atomicWriteFile with a destination that already holds valid JSON. The
// contract is that the destination must end up holding either the old
// bytes verbatim or the new bytes verbatim — never a half-written mix.
// If a non-atomic os.WriteFile were used here, a crash between write and
// close would leave the file truncated to whatever made it onto disk.
func TestAtomicWrite_NotPartialOnCrash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	oldJSON := []byte(`{"theme":"dark","fontFamily":"Consolas","fontSize":14}`)

	// Pre-populate with valid prior content (the "previous good state").
	if err := os.WriteFile(path, oldJSON, 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Race a concurrent reader against the atomic write. Even with the
	// reader hitting the file at any moment, it must observe only the
	// old contents or the new contents — never a mix.
	oldMatches := atomic.Bool{}
	newMatches := atomic.Bool{}
	sawPartial := atomic.Bool{}

	newJSON := []byte(`{"theme":"light","fontFamily":"Menlo","fontSize":16}`)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			switch string(data) {
			case string(oldJSON):
				oldMatches.Store(true)
			case string(newJSON):
				newMatches.Store(true)
			default:
				sawPartial.Store(true)
			}
		}
	}()

	// On Windows an open handle blocks os.Rename, so the polling reader
	// above can make the rename fail spuriously. Retry briefly — the
	// property under test is atomicity of the visible content, not the
	// rename winning on the first attempt.
	var writeErr error
	for i := 0; i < 50; i++ {
		writeErr = atomicWriteFile(path, newJSON, 0600)
		if writeErr == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if writeErr != nil {
		close(stop)
		t.Fatalf("atomicWriteFile: %v", writeErr)
	}
	close(stop)

	if sawPartial.Load() {
		t.Fatalf("observed partial file content during atomic write — kill -9 mid-save would corrupt user data")
	}
	if !oldMatches.Load() && !newMatches.Load() {
		t.Fatalf("reader never observed either old or new content — reader raced ahead of file create")
	}

	// Final state must be the new bytes.
	final, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if string(final) != string(newJSON) {
		t.Fatalf("final content mismatch:\n got %q\nwant %q", final, newJSON)
	}
}

// TestAtomicWrite_NoTempLeakOnSuccess verifies the temp file is removed on
// the happy path so repeated saves don't accumulate junk in the config dir.
func TestAtomicWrite_NoTempLeakOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := atomicWriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected exactly 1 file in %s, got %d: %v", dir, len(entries), names)
	}
}

// TestAtomicWrite_ReplacesTargetOverwritingExisting ensures atomicWriteFile
// can replace an existing regular file (not just create new).
func TestAtomicWrite_ReplacesTargetOverwritingExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := atomicWriteFile(path, []byte("second"), 0600); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("got %q, want %q", got, "second")
	}
}

// TestConnectionStore_SaveRefusesPlaintextWithoutKeychain verifies STORE-04:
// when no PasswordStore is configured, Save() with a password connection
// must return an error and must NOT write a plaintext password to disk.
func TestConnectionStore_SaveRefusesPlaintextWithoutKeychain(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir}

	data := session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", Name: "prod", Type: "ssh", AuthType: "password", Password: "s3cret"},
		},
	}
	err := s.Save(data)
	if err == nil {
		t.Fatalf("Save without passwordStore should return an error, got nil")
	}

	// The connections.json must not exist on disk — fail closed.
	connPath := filepath.Join(dir, storeFileName)
	if _, statErr := os.Stat(connPath); statErr == nil {
		t.Fatalf("connections.json was written despite keychain refusal — plaintext password leak")
	}
}

// TestConnectionStore_SaveStripsPasswordWhenKeychainWired verifies that once
// a PasswordStore is configured, the on-disk JSON has no password field even
// though the caller submitted one.
func TestConnectionStore_SaveStripsPasswordWhenKeychainWired(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir}
	s.SetPasswordStore(fakeCipherStore{})

	data := session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", Name: "prod", Type: "ssh", AuthType: "password", Password: "s3cret"},
		},
	}
	if err := s.Save(data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, storeFileName))
	if err != nil {
		t.Fatalf("read on-disk: %v", err)
	}
	if got := string(raw); contains(got, "s3cret") {
		t.Fatalf("plaintext password leaked to connections.json: %s", got)
	}
}

// TestSkillsStore_DeleteRefusesSymlinkedDir verifies STORE-02: an imported
// skill whose directory is actually a symlink pointing outside the skills
// root must NOT be followed by RemoveAll (which would let an attacker
// delete arbitrary directories on the host).
func TestSkillsStore_DeleteRefusesSymlinkedDir(t *testing.T) {
	root := t.TempDir()
	skillsRoot := filepath.Join(root, "skills")
	if err := os.MkdirAll(skillsRoot, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}

	// Build a target outside the skills root that the attacker wants nuked.
	victim := filepath.Join(root, "victim")
	if err := os.MkdirAll(victim, 0755); err != nil {
		t.Fatalf("mkdir victim: %v", err)
	}
	marker := filepath.Join(victim, "do-not-delete.txt")
	if err := os.WriteFile(marker, []byte("important"), 0644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	// Plant a symlink inside skills/ that points at the victim dir.
	evilLink := filepath.Join(skillsRoot, "evil")
	if err := os.Symlink(victim, evilLink); err != nil {
		t.Skipf("symlink not supported on this filesystem: %v", err)
	}

	// Seed a minimal prefs.json so List() returns a meta whose Dir == "evil".
	prefs := `{"version":1,"prefs":[{"name":"evil","origin":"imported","locked":true,"enabled":true,"sortOrder":0,"createdAt":"2026-01-01T00:00:00Z","version":1}]}`
	if err := os.WriteFile(filepath.Join(skillsRoot, "skills.json"), []byte(prefs), 0600); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}

	s := NewSkillsStore(root)
	if err := s.Delete("evil"); err == nil {
		t.Fatalf("Delete should refuse to traverse a symlinked skill directory")
	}

	// The victim directory and its marker must still be intact.
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("victim marker disappeared after Delete — symlink was followed: %v", err)
	}
}

// TestSkillsStore_ImportRefusesSymlinkedSource verifies STORE-02 on the
// import side: importing a single symlinked file (not a directory of
// symlinks) must also be refused, otherwise an attacker can plant a
// symlink that gets copied into the skills tree.
func TestSkillsStore_ImportRefusesSymlinkedSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	realMD := filepath.Join(root, "real.md")
	if err := os.WriteFile(realMD, []byte("---\nname: x\ndescription: y\n---\nbody"), 0644); err != nil {
		t.Fatalf("seed real: %v", err)
	}
	linkMD := filepath.Join(src, "SKILL.md")
	if err := os.Symlink(realMD, linkMD); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	s := NewSkillsStore(root)
	if _, err := s.ImportFromDir(src); err == nil {
		t.Fatalf("ImportFromDir should refuse to follow a symlinked SKILL.md")
	}
}

// TestSettingsStore_LoadQuarantinesCorrupt verifies STORE-09: a corrupt
// settings.json must be renamed aside before defaulting, so the next Save
// doesn't silently overwrite the user's prior data.
func TestSettingsStore_LoadQuarantinesCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, settingsFileName)
	if err := os.WriteFile(path, []byte("not json {{{"), 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := &SettingsStore{configDir: dir}
	_, err := s.Load()
	if err != nil {
		t.Fatalf("Load should not propagate corrupt-file errors: %v", err)
	}

	// Quarantine file must exist; original must not.
	matches, err := filepath.Glob(filepath.Join(dir, settingsFileName+".corrupt-*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 quarantine file, got %d", len(matches))
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("original settings.json still present after Load — would be silently overwritten by next Save")
	}
}

// ---- helpers ----

// fakeCipherStore is a deterministic in-memory PasswordStore for tests.
// Encrypt wraps plaintext as "enc:v1:<base64>" — the "enc:v1:" prefix makes
// credentials.IsEncrypted recognize it, and base64 hides the plaintext so
// tests can assert no plaintext leaks to disk without real crypto.
type fakeCipherStore struct{}

func (fakeCipherStore) Encrypt(plaintext string) (string, error) {
	return "enc:v1:" + base64.StdEncoding.EncodeToString([]byte(plaintext)), nil
}
func (fakeCipherStore) Decrypt(encoded string) (string, error) {
	if !strings.HasPrefix(encoded, "enc:v1:") {
		return encoded, nil
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, "enc:v1:"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}