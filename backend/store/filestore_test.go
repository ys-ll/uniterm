package store

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
