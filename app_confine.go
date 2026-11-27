package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ys-ll/uniterm/backend/recorder"
	"github.com/ys-ll/uniterm/backend/serverpolicy"
)

// tenantLocalRoots returns the local filesystem roots a server-mode tenant may
// address: its HOME (which includes the /api/web staging root under
// $HOME/.cache/uniterm-web-staging), the resolved data directory, the records
// directory, and the recorder's own records directory.
func (a *App) tenantLocalRoots() []string {
	var roots []string
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		home = strings.TrimSpace(home)
		roots = append(roots,
			home,
			filepath.Join(home, ".cache", "uniterm-web-staging"),
			filepath.Join(home, "Documents", "uniTerm", "records"),
		)
	}
	if a != nil && strings.TrimSpace(a.dataDir) != "" {
		roots = append(roots,
			a.dataDir,
			filepath.Join(a.dataDir, "records"),
		)
	}
	if records := recorder.New().RecordsDir(); records != "" {
		roots = append(roots, records)
	}
	return roots
}

// serverStagingDir is the /api/web staging root used by the headless server
// build (see NewWebStreamService). It must stay in sync with that path.
func serverStagingDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("tenant home directory is unavailable: %w", err)
	}
	return filepath.Join(strings.TrimSpace(home), ".cache", "uniterm-web-staging"), nil
}

// confineLocalPath enforces tenant path confinement in the server build and is
// a no-op on desktop. It returns the canonical path to use for the actual
// filesystem operation.
func (a *App) confineLocalPath(path string) (string, error) {
	if !serverMode {
		return path, nil
	}
	return serverpolicy.ConfineLocalPath(a.tenantLocalRoots(), path)
}

// checkContainerTransport applies the worker-local container rule. The
// transport discriminator is cfg.ContainerTransport; sshUser is never used.
func (a *App) checkContainerTransport(transport string) error {
	return serverpolicy.CheckContainerTransport(currentServerPolicy(), transport)
}

// confineSQLiteHost confines the Host field of a SQLite database connection.
// SQLite carries a worker-local file in Host; file: URIs are rejected in
// server mode because the driver would follow them outside confinement, and
// :memory: is preserved because it touches no file. Desktop is unchanged.
func (a *App) confineSQLiteHost(host string) (string, error) {
	if !serverMode {
		return host, nil
	}
	trimmed := strings.TrimSpace(host)
	if trimmed == ":memory:" {
		return host, nil
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "file:") {
		return "", fmt.Errorf("%w: file: URI SQLite paths are not allowed in server mode", serverpolicy.ErrPathDenied)
	}
	return a.confineLocalPath(host)
}

// serverMCPClientProcessEnabled gates every path that can dial a persisted
// external MCP server. The bound methods are already filtered out by default,
// but startup, save and sync-pull reloads call the manager directly; without
// this guard a default-deny policy would still spawn enabled stdio MCP
// clients.
func serverMCPClientProcessEnabled() bool {
	return currentServerPolicy().Enabled(serverpolicy.GroupMCPClientProcess)
}
