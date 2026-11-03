//go:build server

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/ys-ll/uniterm/backend/credentials"
)

// fileKeychain stores keychain entries in a 0600 JSON file under HOME.
//
// The web build runs in a container with no D-Bus session bus, so the OS
// keychain (secret-service) is unavailable and every credential setup or
// unlock fails with `exec: "dbus-launch": executable file not found`. Each
// tenant already has its own HOME, so that file is the trust boundary here —
// weaker than an OS keychain (anything that can read the user's home can read
// the key) but the only option without a session bus.
type fileKeychain struct {
	path string
	mu   sync.Mutex
}

func credentialKeychain() credentials.Keychain {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/tmp"
	}
	return &fileKeychain{path: filepath.Join(home, ".uniterm", "keychain.json")}
}

func (k *fileKeychain) load() (map[string]string, error) {
	entries := map[string]string{}
	data, err := os.ReadFile(k.path)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (k *fileKeychain) save(entries map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(k.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	tmp := k.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.path)
}

func (k *fileKeychain) Get(key string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	entries, err := k.load()
	if err != nil {
		return "", err
	}
	return entries[key], nil
}

func (k *fileKeychain) Set(key, value string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	entries, err := k.load()
	if err != nil {
		return err
	}
	entries[key] = value
	return k.save(entries)
}

func (k *fileKeychain) Delete(key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	entries, err := k.load()
	if err != nil {
		return err
	}
	delete(entries, key)
	return k.save(entries)
}

func (k *fileKeychain) List() ([]string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	entries, err := k.load()
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}
