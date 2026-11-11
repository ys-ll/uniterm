package credentials

import (
	"errors"
	"testing"
)

type fileStoreTestKeychain struct {
	entries map[string]string
}

func (k *fileStoreTestKeychain) Get(key string) (string, error) { return k.entries[key], nil }
func (k *fileStoreTestKeychain) Set(key, value string) error    { k.entries[key] = value; return nil }
func (k *fileStoreTestKeychain) Delete(key string) error        { delete(k.entries, key); return nil }
func (k *fileStoreTestKeychain) List() ([]string, error) {
	keys := make([]string, 0, len(k.entries))
	for key := range k.entries {
		keys = append(keys, key)
	}
	return keys, nil
}

// TestFileSecretStoreRoundtripAndRekey covers the OS-keychain replacement
// used by server builds: values encrypt at rest, survive a store reload
// (container restart), and Rekey carries every entry to the new master key
// atomically (a failed rekey leaves the file untouched).
func TestFileSecretStoreRoundtripAndRekey(t *testing.T) {
	dir := t.TempDir()
	cs := New(dir, &fileStoreTestKeychain{entries: map[string]string{}})
	if err := cs.Setup(ModeMasterPassword, "login-pass"); err != nil {
		t.Fatal(err)
	}
	fs := NewFileSecretStore(dir+"/secret-file.json", cs)

	if err := fs.Set("git-token", "tok-1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := fs.Set("webdav-password", "pw-1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Missing key reads as keychain-not-found (the sync keychain maps that
	// to "empty", mirroring the OS keychain).
	if _, err := fs.Get("missing"); !errors.Is(err, ErrKeychainNotFound) {
		t.Fatalf("get missing: want ErrKeychainNotFound, got %v", err)
	}

	if err := fs.Set("git-token", "tok-2"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Simulate a worker respawn: a fresh store re-derives the key from the
	// login password and must still read the entries.
	reloaded := NewFileSecretStore(dir+"/secret-file.json", cs)
	if v, err := reloaded.Get("git-token"); err != nil || v != "tok-2" {
		t.Fatalf("reload get git-token: %q %v", v, err)
	}
	if v, err := reloaded.Get("webdav-password"); err != nil || v != "pw-1" {
		t.Fatalf("reload get webdav-password: %q %v", v, err)
	}

	// Rekey to a new master key, reload, read under the new key.
	oldKey := cs.Key()
	newKey := []byte("0123456789abcdef0123456789abcdef")
	if err := reloaded.Rekey(oldKey, newKey); err != nil {
		t.Fatalf("rekey: %v", err)
	}
	cs.SetKey(newKey)
	after := NewFileSecretStore(dir+"/secret-file.json", cs)
	if v, err := after.Get("git-token"); err != nil || v != "tok-2" {
		t.Fatalf("post-rekey get git-token: %q %v", v, err)
	}

	// Rekey with a wrong old key must fail and leave the file untouched.
	if err := after.Rekey([]byte("wrong-wrong-wrong-wrong-wrong-wrong"), newKey); err == nil {
		t.Fatal("rekey with wrong old key: want error")
	}
	if v, err := after.Get("git-token"); err != nil || v != "tok-2" {
		t.Fatalf("get after failed rekey: %q %v", v, err)
	}

	if err := after.Delete("git-token"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := after.Get("git-token"); err == nil {
		t.Fatal("get deleted: want error")
	}
}
