//go:build !android

package session

import (
	"os"
	"path/filepath"
)

// KnownHostsPath returns the absolute path of ~/.uniterm/known_hosts.
// Exported so tests and management tools can locate the file.
func KnownHostsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".uniterm", "known_hosts"), nil
}
