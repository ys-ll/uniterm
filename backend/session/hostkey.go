package session

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ys-ll/uniterm/backend/log"
	"golang.org/x/crypto/ssh"
)

// HostKeyChangedError describes one host key mismatch. The exported fields
// are serialized into the frontend event payload.
type HostKeyChangedError struct {
	Host      string `json:"host"`
	Port      string `json:"port"`
	SessionID string `json:"sessionId"`
	KeyType   string `json:"keyType"`
	OldFp     string `json:"oldFp"` // SHA256 fingerprint (ssh.FingerprintSHA256 format)
	NewFp     string `json:"newFp"` // SHA256 fingerprint
	OldKeyB64 string `json:"oldKeyB64"`
	NewKeyB64 string `json:"newKeyB64"`
}

func (e *HostKeyChangedError) Error() string {
	return fmt.Sprintf("host key for %s:%s changed (%s): old=%s new=%s — possible MITM; verify the new fingerprint before accepting",
		e.Host, e.Port, e.KeyType, e.OldFp, e.NewFp)
}

// Is lets errors.Is/As recognize host key change errors.
func (e *HostKeyChangedError) Is(target error) bool {
	_, ok := target.(*HostKeyChangedError)
	return ok
}

// hostKeyEmitterFunc is the event emitter signature injected by the App
// layer; it fires when a key change should surface a frontend dialog.
type hostKeyEmitterFunc = func(name string, payload any)

var (
	// hostKeyMu serializes known_hosts file access so concurrent
	// connections cannot clobber each other's entries.
	hostKeyMu sync.Mutex

	hostKeyEmitterMu sync.RWMutex
	hostKeyEmitter   hostKeyEmitterFunc
)

// onceGrant is a one-shot bypass recorded by the change dialog ("connect
// once, don't save"): the exact key shown to the user is allowed through a
// single time without touching known_hosts.
type onceGrant struct {
	keyB64  string
	granted time.Time
}

const (
	onceGrantTTL = 10 * time.Minute
)

var (
	// onceGrants is guarded by hostKeyMu, same as the known_hosts file.
	onceGrants = map[string]onceGrant{}
)

// SetHostKeyEmitter injects the Wails event emitter. Called once by the App
// layer at startup. Passing nil clears it (mainly for tests).
func SetHostKeyEmitter(emitter hostKeyEmitterFunc) {
	hostKeyEmitterMu.Lock()
	defer hostKeyEmitterMu.Unlock()
	hostKeyEmitter = emitter
}

func getHostKeyEmitter() hostKeyEmitterFunc {
	hostKeyEmitterMu.RLock()
	defer hostKeyEmitterMu.RUnlock()
	return hostKeyEmitter
}

// knownHostsKey builds the host column of a known_hosts line, OpenSSH
// style: non-22 ports use the [host]:port form.
func knownHostsKey(host, port string) string {
	if port == "" || port == "22" {
		return host
	}
	return fmt.Sprintf("[%s]:%s", host, port)
}

// HostKeyCallback returns the TOFU host key verification callback. host/port
// locate the known_hosts entry; sessionID (optional) is carried in the change
// event payload so the frontend can reconnect the failed session after the
// user accepts the new key.
func HostKeyCallback(host, port, sessionID string) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		return verifyHostKey(host, port, sessionID, key)
	}
}

// verifyHostKey implements the TOFU verification core.
func verifyHostKey(host, port, sessionID string, newKey ssh.PublicKey) error {
	hostKeyMu.Lock()
	defer hostKeyMu.Unlock()

	path, err := KnownHostsPath()
	if err != nil {
		// Failing to resolve the home directory is an environment problem;
		// refusing the connection is safer than letting any key through.
		return fmt.Errorf("hostkey: resolve known_hosts path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("hostkey: mkdir %s: %w", filepath.Dir(path), err)
	}

	keyType := newKey.Type()
	newKeyB64 := base64.StdEncoding.EncodeToString(newKey.Marshal())
	newFp := ssh.FingerprintSHA256(newKey)
	lookupKey := knownHostsKey(host, port)

	// One-shot bypass: the user accepted this exact key in the change dialog
	// for a single connection without recording it ("connect once"). The
	// grant is consumed on use and expires after a while.
	if g, ok := onceGrants[lookupKey]; ok {
		if time.Since(g.granted) > onceGrantTTL {
			delete(onceGrants, lookupKey)
		} else if g.keyB64 == newKeyB64 {
			delete(onceGrants, lookupKey)
			return nil
		}
	}

	oldKeyB64, oldKey, err := lookupKnownHostKey(path, lookupKey, keyType)
	if err != nil {
		// A corrupt file must not wedge the user out of their hosts: log it
		// and treat as "not recorded" so the new fingerprint can be written.
		log.Writef("hostkey: read %s failed (%v); treating as first connect", path, err)
		oldKeyB64, oldKey = "", nil
	}

	if oldKey == nil {
		// First connection: record and allow (TOFU).
		if err := appendKnownHostKey(path, lookupKey, keyType, newKeyB64); err != nil {
			return fmt.Errorf("hostkey: write %s: %w", path, err)
		}
		return nil
	}

	if oldKeyB64 == newKeyB64 {
		// Fingerprint matches: allow.
		return nil
	}

	// Key changed: reject and notify the frontend so the user can confirm
	// the new fingerprint and reconnect.
	changed := &HostKeyChangedError{
		Host:      host,
		Port:      port,
		SessionID: sessionID,
		KeyType:   keyType,
		OldFp:     ssh.FingerprintSHA256(oldKey),
		NewFp:     newFp,
		OldKeyB64: oldKeyB64,
		NewKeyB64: newKeyB64,
	}
	if emitter := getHostKeyEmitter(); emitter != nil {
		emitter("session:hostkey-changed", map[string]any{
			"host":      changed.Host,
			"port":      changed.Port,
			"sessionId": changed.SessionID,
			"keyType":   changed.KeyType,
			"oldFp":     changed.OldFp,
			"newFp":     changed.NewFp,
			"oldKeyB64": changed.OldKeyB64,
			"newKeyB64": changed.NewKeyB64,
		})
	}
	return changed
}

// lookupKnownHostKey finds the base64 public key recorded for host+keyType.
// Returns (keyB64, parsedKey, error); ("", nil, nil) when not found.
func lookupKnownHostKey(path, host, keyType string) (string, ssh.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[0] != host || fields[1] != keyType {
			continue
		}
		keyB64 := fields[2]
		raw, err := base64.StdEncoding.DecodeString(keyB64)
		if err != nil {
			return "", nil, fmt.Errorf("decode key for %s: %w", host, err)
		}
		parsed, err := ssh.ParsePublicKey(raw)
		if err != nil {
			return "", nil, fmt.Errorf("parse key for %s: %w", host, err)
		}
		return keyB64, parsed, nil
	}
	return "", nil, nil
}

// appendKnownHostKey appends one entry to the known_hosts file.
func appendKnownHostKey(path, host, keyType, keyB64 string) error {
	line := fmt.Sprintf("%s %s %s\n", host, keyType, keyB64)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

// AcceptHostKeyChange replaces the known_hosts entry for host with the new
// public key. Called from the App layer after the user confirms a host key
// change in the frontend dialog. newKeyB64 is the base64-encoded SSH wire
// format public key delivered with the change event.
func AcceptHostKeyChange(host, port, keyType, newKeyB64 string) error {
	hostKeyMu.Lock()
	defer hostKeyMu.Unlock()

	if _, _, err := parseHostKeyB64(newKeyB64); err != nil {
		return fmt.Errorf("hostkey: invalid key: %w", err)
	}

	path, err := KnownHostsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lookupKey := knownHostsKey(host, port)
	return replaceKnownHostKey(path, lookupKey, keyType, newKeyB64)
}

// AcceptHostKeyOnce records a one-shot bypass for host:port, allowing the
// given key through a single connection without writing it to known_hosts.
// Called from the App layer when the user picks "connect once, don't save"
// in the change dialog. The grant is bound to the exact key displayed in the
// dialog and expires after onceGrantTTL.
func AcceptHostKeyOnce(host, port, keyB64 string) error {
	if _, _, err := parseHostKeyB64(keyB64); err != nil {
		return fmt.Errorf("hostkey: invalid key: %w", err)
	}
	hostKeyMu.Lock()
	defer hostKeyMu.Unlock()
	onceGrants[knownHostsKey(host, port)] = onceGrant{keyB64: keyB64, granted: time.Now()}
	return nil
}

// parseHostKeyB64 validates a base64-encoded SSH wire format public key.
func parseHostKeyB64(keyB64 string) (ssh.PublicKey, []byte, error) {
	raw, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := ssh.ParsePublicKey(raw)
	if err != nil {
		return nil, nil, err
	}
	return parsed, raw, nil
}

// replaceKnownHostKey swaps the host+keyType entry in the known_hosts file;
// appends one when absent.
func replaceKnownHostKey(path, host, keyType, newKeyB64 string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 3 && fields[0] == host && fields[1] == keyType {
			continue // drop the stale entry
		}
		kept = append(kept, trimmed)
	}
	kept = append(kept, fmt.Sprintf("%s %s %s", host, keyType, newKeyB64))
	content := strings.Join(kept, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0600)
}

// AsHostKeyChangedError returns the *HostKeyChangedError inside err and
// true when err carries one, for the App layer to render friendly terminal
// messages.
func AsHostKeyChangedError(err error) (*HostKeyChangedError, bool) {
	var changed *HostKeyChangedError
	if errors.As(err, &changed) {
		return changed, true
	}
	return nil, false
}
