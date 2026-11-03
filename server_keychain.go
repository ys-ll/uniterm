//go:build server

package main

import (
	"sort"
	"sync"

	"github.com/ys-ll/uniterm/backend/credentials"
)

// memKeychain is the server-build keychain: an in-memory map with no disk
// presence. The container has no D-Bus secret service, and the previous
// file-backed implementation wrote the credential master key to a plaintext
// JSON file — which defeated the login-password-as-master-password model
// (anyone reading the tenant home could decrypt the stores). With the
// in-memory keychain, master keys live only in worker process memory: a
// respawned worker re-derives the key from the login password the gateway
// forwards (UNITERM_MASTER_PASSWORD), or the user unlocks in the browser.
type memKeychain struct {
	mu      sync.Mutex
	entries map[string]string
}

func credentialKeychain() credentials.Keychain {
	return &memKeychain{entries: map[string]string{}}
}

func (k *memKeychain) Get(key string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.entries[key], nil
}

func (k *memKeychain) Set(key, value string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.entries == nil {
		k.entries = map[string]string{}
	}
	k.entries[key] = value
	return nil
}

func (k *memKeychain) Delete(key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.entries, key)
	return nil
}

func (k *memKeychain) List() ([]string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	keys := make([]string, 0, len(k.entries))
	for key := range k.entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}
