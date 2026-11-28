//go:build !server

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/serverpolicy"
)

// currentServerPolicy on desktop is allow-all: the desktop process is not the
// multi-tenant boundary, and the shared container transport guard must keep
// accepting the local Docker transport exactly as before.
func currentServerPolicy() *serverpolicy.Policy {
	return serverpolicy.AllowAll()
}

// appServiceOptions returns empty options on desktop: MethodFilter is nil, so
// every exported App method stays bound and every desktop feature is kept.
func appServiceOptions() application.ServiceOptions {
	return application.ServiceOptions{}
}
