//go:build !server

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/credentials"
	"github.com/ys-ll/uniterm/backend/sync"
)

// serverMode lets shared (untagged) code branch on the -tags server build.
const serverMode = false

// credentialKeychain uses the OS keychain on desktop; the server build swaps in
// a file-backed one (see server_keychain.go).
func credentialKeychain() credentials.Keychain {
	return sync.NewKeychain()
}

func BuildServerOptions() application.ServerOptions {
	return application.ServerOptions{}
}

// registerWebStreamService is a no-op on desktop: the /api/web routes only
// exist in the headless server build (see server_web_extension.go).
func registerWebStreamService(_ *application.App, _ *App) {}

// touchWebActivity tracks frontend-visible activity for the gateway's idle
// reaper; desktop builds have no gateway, so this is a no-op.
func touchWebActivity() {}

// newFileSecretStore has no desktop counterpart: the desktop sync keychain
// uses the OS keychain, so there is no encrypted-file store to wire.
func newFileSecretStore(_ string, _ *credentials.Store) secretRekeyer {
	return nil
}
