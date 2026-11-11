package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileSecretStore is the OS-keychain replacement for builds where none is
// available (headless server containers). Entries live in a single JSON file
// with every value encrypted through the master key (enc:v1:): the key
// re-derives from the tenant's login password on worker respawn, so secrets
// survive container restarts and never touch the disk in plaintext. It is
// the server build's backing store for the sync keychain; desktop builds use
// the platform keychain instead.
type FileSecretStore struct {
	mu   sync.Mutex
	path string
	s    *Store
}

// NewFileSecretStore builds a FileSecretStore persisting to path, encrypting
// values with the given credential store's master key.
func NewFileSecretStore(path string, s *Store) *FileSecretStore {
	return &FileSecretStore{path: path, s: s}
}

type secretFileData struct {
	Version int               `json:"version"`
	Entries map[string]string `json:"entries"`
}

func (f *FileSecretStore) Get(key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := f.load()
	if err != nil {
		return "", err
	}
	enc, ok := data.Entries[key]
	if !ok || enc == "" {
		return "", fmt.Errorf("secret %q: %w", key, ErrKeychainNotFound)
	}
	plain, err := f.s.Decrypt(enc)
	if err != nil {
		return "", fmt.Errorf("decrypt %s: %w", key, err)
	}
	return plain, nil
}

func (f *FileSecretStore) Set(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	enc, err := f.s.Encrypt(value)
	if err != nil {
		return fmt.Errorf("encrypt %s: %w", key, err)
	}
	data, err := f.load()
	if err != nil {
		return err
	}
	if data.Entries == nil {
		data.Entries = map[string]string{}
	}
	data.Entries[key] = enc
	return f.save(data)
}

func (f *FileSecretStore) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := data.Entries[key]; !ok {
		return nil
	}
	delete(data.Entries, key)
	return f.save(data)
}

func (f *FileSecretStore) List() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := f.load()
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(data.Entries))
	for k := range data.Entries {
		keys = append(keys, k)
	}
	return keys, nil
}

// Rekey decrypts every entry under oldKey and re-encrypts it under newKey in
// one atomic write. Wired into the master-password change so a rekey carries
// the stored secrets along; the key-level API is used instead of the store's
// Encrypt/Decrypt because the caller swaps the in-memory key around this
// point. All-or-nothing: a decrypt failure leaves the file untouched under
// the old key.
func (f *FileSecretStore) Rekey(oldKey, newKey []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := f.load()
	if err != nil {
		return err
	}
	reenc := make(map[string]string, len(data.Entries))
	for key, enc := range data.Entries {
		plain, err := DecryptField(enc, oldKey)
		if err != nil {
			return fmt.Errorf("rekey %s: %w", key, err)
		}
		reenc[key], err = EncryptField(plain, newKey)
		if err != nil {
			return fmt.Errorf("rekey %s: %w", key, err)
		}
	}
	data.Entries = reenc
	return f.save(data)
}

func (f *FileSecretStore) load() (*secretFileData, error) {
	raw, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &secretFileData{Version: 1}, nil
		}
		return nil, err
	}
	var data secretFileData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(f.path), err)
	}
	return &data, nil
}

func (f *FileSecretStore) save(data *secretFileData) error {
	data.Version = 1
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, f.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
