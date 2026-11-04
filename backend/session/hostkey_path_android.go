//go:build android

package session

import (
	"errors"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// KnownHostsPath on Android: the app sandbox has no usable home directory.
// os.UserHomeDir can resolve to /sdcard, where creating .uniterm is denied
// by scoped storage ("operation not permitted"), which aborts the SSH
// handshake. Store known_hosts in the app's private files dir instead —
// same base as the store bootstrap (<filesDir>/uniTerm).
func KnownHostsPath() (string, error) {
	base := application.Android.StoragePath()
	if base == "" {
		return "", errors.New("hostkey: android storage path unavailable (bridge not attached)")
	}
	return filepath.Join(base, "uniTerm", "known_hosts"), nil
}
