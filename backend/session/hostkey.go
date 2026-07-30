package session

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/ys-ll/uniterm/backend/log"
)

// sshHostKeyCallback returns an ssh.HostKeyCallback for the given config.
//
// Behavior:
//   - StrictHostKeyChecking=true  → reject unknown keys, reject changed keys (no prompt).
//   - StrictHostKeyChecking=false, AcceptNewHostKey=true → accept first-seen keys and
//     append them to the known_hosts file; reject changed keys.
//   - Otherwise (both false) → verify against known_hosts when the file is readable;
//     fall back to insecure-accept (with a one-shot warning) when no file is configured
//     or it cannot be read. This preserves backward compatibility for users who have
//     no known_hosts yet.
//
// KnownHostsPath falls back to ~/.ssh/known_hosts when empty.
func sshHostKeyCallback(config ConnectionConfig) (ssh.HostKeyCallback, error) {
	path, err := resolveKnownHostsPath(config.KnownHostsPath)
	if err != nil {
		return nil, err
	}

	// Try to load the file. Missing file is not an error: it just means the
	// user has never accepted a host key before, so we will either fall back
	// or seed it below depending on the config flags.
	cb, loadErr := knownhosts.New(path)
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		// Corrupt file or permission issue — surface the error so we never
		// silently downgrade to InsecureIgnoreHostKey on a config mistake.
		return nil, fmt.Errorf("load known_hosts %s: %w", path, loadErr)
	}

	if config.StrictHostKeyChecking {
		// Strict mode requires a real DB — missing file is a hard error so
		// the user cannot accidentally accept any host on first connect.
		if loadErr != nil {
			return nil, fmt.Errorf("strict host key checking requested but %s is not readable: %w", path, loadErr)
		}
		return cb, nil
	}

	// Permissive path: cache the file path + mutex so the new-key branch can
	// append under a lock without data races.
	w := &knownHostsWriter{path: path}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if cb == nil {
			// No known_hosts loaded; if user opted into AcceptNewHostKey,
			// seed the file with this key. Otherwise warn once and accept
			// (legacy behavior — unchanged from InsecureIgnoreHostKey).
			if config.AcceptNewHostKey {
				return w.append(hostname, key)
			}
			log.Writef("ssh: no known_hosts at %s, falling back to insecure host key acceptance", path)
			return nil
		}
		err := cb(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		// Populated Want slice means the host IS in the DB but presented a
		// different key → possible MITM. Never silently accept.
		if len(keyErr.Want) > 0 {
			return fmt.Errorf("host key for %s has changed (possible MITM); refusing to connect. "+
				"Update %s only after verifying the new fingerprint out-of-band", hostname, path)
		}
		// Empty Want → host is unknown. Accept only if the user opted in.
		if !config.AcceptNewHostKey {
			return fmt.Errorf("host %s is not in known_hosts; set AcceptNewHostKey=true to trust it on first connect", hostname)
		}
		return w.append(hostname, key)
	}, nil
}

func resolveKnownHostsPath(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate user home for known_hosts: %w", err)
	}
	return filepath.Join(home, ".ssh", "known_hosts"), nil
}

type knownHostsWriter struct {
	path string
	mu   sync.Mutex
}

// append writes a single known_hosts line for hostname+key, atomic via
// temp+rename so concurrent first connects don't truncate each other.
func (w *knownHostsWriter) append(hostname string, key ssh.PublicKey) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(w.path), 0700); err != nil {
		return fmt.Errorf("mkdir known_hosts dir: %w", err)
	}

	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key) + "\n"

	// Read existing contents (tolerate missing file — first-accept case).
	existing, _ := os.ReadFile(w.path)
	if err := os.WriteFile(w.path+".tmp", append(existing, []byte(line)...), 0600); err != nil {
		return fmt.Errorf("write known_hosts.tmp: %w", err)
	}
	if err := os.Rename(w.path+".tmp", w.path); err != nil {
		_ = os.Remove(w.path + ".tmp")
		return fmt.Errorf("rename known_hosts.tmp: %w", err)
	}
	log.Writef("ssh: appended new host key for %s to %s", hostname, w.path)
	return nil
}
