//go:build server

package main

import (
	"path/filepath"

	"github.com/ys-ll/uniterm/backend/credentials"
)

// syncKeychainFile is the sync keychain's encrypted backing file inside the
// tenant data dir. Registered in store.SecretFiles so credential reset blanks
// it and the setup guard counts its ciphertext like any other secret file.
const syncKeychainFile = "sync-keychain.json"

// newFileSecretStore builds the server-build replacement for the OS keychain
// used by the sync service. Desktop builds have no counterpart (see
// server_desktop_stub.go).
func newFileSecretStore(dataDir string, cred *credentials.Store) secretRekeyer {
	return credentials.NewFileSecretStore(filepath.Join(dataDir, syncKeychainFile), cred)
}
