//go:build !android

package credentials_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ys-ll/uniterm/backend/credentials"
	keychainsync "github.com/ys-ll/uniterm/backend/sync"
	"github.com/zalando/go-keyring"
)

// The OS read uses the production adapter. Enumeration is unavailable on macOS.
type nonEnumeratingKeychain struct {
	*keychainsync.Keychain
}

func (nonEnumeratingKeychain) List() ([]string, error) {
	return nil, errors.New("keychain enumeration unavailable")
}

func TestOSKeychainMissingKeyPreservesCredentialState(t *testing.T) {
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	kc := nonEnumeratingKeychain{keychainsync.NewKeychain()}
	dir := t.TempDir()
	original := credentials.New(dir, kc)
	if err := original.Setup(credentials.ModeKeychain, ""); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := original.Encrypt("saved password")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"password":"` + ciphertext + `"}`)
	path := filepath.Join(dir, "connections.json")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(dir, "credentials.meta")
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := kc.Delete("keychain-key/" + original.DirHash()); err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Get("keychain-key/" + original.DirHash()); !errors.Is(err, credentials.ErrKeychainNotFound) || !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("missing key error = %v", err)
	}

	restarted := credentials.New(dir, kc)
	restarted.SetSecretFiles([]string{"connections.json"})
	if err := restarted.AutoUnlock(); err != nil {
		t.Fatalf("AutoUnlock: %v", err)
	}
	st := restarted.Status()
	if st.Mode != credentials.ModeKeychain || !st.KeychainLost || st.NeedsSetup || st.Unlocked || st.DecryptFailed || st.ExistingSecrets != 1 {
		t.Fatalf("status = %+v", st)
	}
	for name, want := range map[string][]byte{path: body, metaPath: meta} {
		got, err := os.ReadFile(name)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("stored data changed at %s: %v", name, err)
		}
	}
}

func TestOSKeychainReadFailureRemainsAnError(t *testing.T) {
	readErr := errors.New("keychain access denied")
	keyring.MockInitWithError(readErr)
	t.Cleanup(keyring.MockInit)
	kc := keychainsync.NewKeychain()
	if _, err := kc.Get("missing"); !errors.Is(err, readErr) || errors.Is(err, credentials.ErrKeychainNotFound) {
		t.Fatalf("Get error = %v", err)
	}
	dir := t.TempDir()
	if err := credentials.WriteMeta(dir, &credentials.Meta{Mode: credentials.ModeKeychain}); err != nil {
		t.Fatal(err)
	}
	s := credentials.New(dir, nonEnumeratingKeychain{kc})
	if err := s.AutoUnlock(); !errors.Is(err, readErr) {
		t.Fatalf("AutoUnlock error = %v", err)
	}
	if s.Status().KeychainLost {
		t.Fatal("a read failure must not report a lost key")
	}
}

func TestOSKeychainStoredKeyUnlocksCredentials(t *testing.T) {
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	kc := keychainsync.NewKeychain()
	dir := t.TempDir()
	original := credentials.New(dir, kc)
	if err := original.Setup(credentials.ModeKeychain, ""); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := original.Encrypt("saved password")
	if err != nil {
		t.Fatal(err)
	}
	restarted := credentials.New(dir, kc)
	if err := restarted.AutoUnlock(); err != nil {
		t.Fatal(err)
	}
	got, err := restarted.Decrypt(ciphertext)
	if err != nil || got != "saved password" {
		t.Fatalf("Decrypt = %q, %v", got, err)
	}
}
