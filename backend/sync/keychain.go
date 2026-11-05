package sync

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ys-ll/uniterm/backend/credentials"
	"github.com/ys-ll/uniterm/backend/kdf"
	"github.com/zalando/go-keyring"
)

var keychainService = "uniTerm"

type Keychain struct{}

func NewKeychain() *Keychain { return &Keychain{} }

// List returns the entry names stored under the app's keychain service.
// Used by credential recovery to find keys written under another data-dir
// hash (copied/moved data directory). Platforms without enumeration return
// an error — recovery is simply skipped there.
func (k *Keychain) List() ([]string, error) {
	return keychainList(keychainService)
}

func (k *Keychain) Get(key string) (string, error) {
	value, err := keychainGet(keychainService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", fmt.Errorf("%w: %w", credentials.ErrKeychainNotFound, err)
	}
	return value, err
}

func (k *Keychain) Set(key, value string) error {
	return keychainSet(keychainService, key, value)
}

func (k *Keychain) Delete(key string) error {
	return keychainDelete(keychainService, key)
}

// GetEncryptionKey retrieves the derived encryption key from OS keychain.
func (k *Keychain) GetEncryptionKey() ([]byte, error) {
	const keyName = "encryption-key"
	hexKey, err := k.Get(keyName)
	if err != nil {
		return nil, fmt.Errorf("encryption key not found: %w", err)
	}
	return hex.DecodeString(hexKey)
}

// StoreEncryptionKey stores the derived encryption key in OS keychain.
func (k *Keychain) StoreEncryptionKey(key []byte) error {
	return k.Set("encryption-key", hex.EncodeToString(key))
}

func (k *Keychain) GetGitToken() (string, error) {
	token, err := k.Get("git-token")
	if err != nil {
		return "", nil
	}
	return token, nil
}

func (k *Keychain) SetGitToken(token string) error {
	if token == "" {
		return k.Delete("git-token")
	}
	return k.Set("git-token", token)
}

func (k *Keychain) GetWebDAVPassword() (string, error) {
	pw, err := k.Get("webdav-password")
	if err != nil {
		return "", nil
	}
	return pw, nil
}

func (k *Keychain) SetWebDAVPassword(pw string) error {
	if pw == "" {
		return k.Delete("webdav-password")
	}
	return k.Set("webdav-password", pw)
}

// remoteSecretKey maps a snapshot backend to its keychain entry name. The
// WebDAV entry keeps its historical "webdav-password" name so existing
// users' stored credentials survive the unified-backend refactor.
func remoteSecretKey(backend string) string {
	switch backend {
	case "webdav":
		return "webdav-password"
	case "sftp":
		return "sync-sftp-password"
	case "s3":
		return "sync-s3-secret"
	case "smb":
		return "sync-smb-password"
	default:
		return "sync-" + backend + "-secret"
	}
}

// GetRemoteSecret reads the password-class credential of a snapshot
// backend. A missing entry reads as empty, like the WebDAV helper.
func (k *Keychain) GetRemoteSecret(backend string) (string, error) {
	secret, err := k.Get(remoteSecretKey(backend))
	if err != nil {
		return "", nil
	}
	return secret, nil
}

// SetRemoteSecret stores (or clears, on empty) the credential of a
// snapshot backend.
func (k *Keychain) SetRemoteSecret(backend, secret string) error {
	if secret == "" {
		return k.Delete(remoteSecretKey(backend))
	}
	return k.Set(remoteSecretKey(backend), secret)
}

// DeleteRemoteSecret removes the credential of a snapshot backend.
func (k *Keychain) DeleteRemoteSecret(backend string) error {
	return k.Delete(remoteSecretKey(backend))
}
func (k *Keychain) GetModelAPIKey(modelID string) (string, error) {
	apiKey, err := k.Get("ai-model/" + modelID)
	if err != nil {
		return "", nil
	}
	return apiKey, nil
}

func (k *Keychain) SetModelAPIKey(modelID, apiKey string) error {
	if apiKey == "" {
		return k.Delete("ai-model/" + modelID)
	}
	return k.Set("ai-model/"+modelID, apiKey)
}

func (k *Keychain) DeleteModelAPIKey(modelID string) error {
	return k.Delete("ai-model/" + modelID)
}

func (k *Keychain) GetPassword(connID string) (string, error) {
	password, err := k.Get("conn/" + connID)
	if err != nil {
		return "", nil
	}
	return password, nil
}

func (k *Keychain) SetPassword(connID, password string) error {
	if password == "" {
		return k.Delete("conn/" + connID)
	}
	return k.Set("conn/"+connID, password)
}

func (k *Keychain) DeletePassword(connID string) error {
	return k.Delete("conn/" + connID)
}

// DeriveKey derives a 32-byte AES-256 key from a password and salt using PBKDF2-SHA256.
// DeriveKey derives the sync encryption key. Re-exported from backend/kdf
// (the shared leaf package credentials also uses), so existing call sites
// inside and outside the package keep compiling.
func DeriveKey(password string, salt []byte) []byte {
	return kdf.DeriveKey(password, salt)
}

// GenerateSalt generates a random 16-byte salt. Re-exported from
// backend/kdf; see DeriveKey.
func GenerateSalt() ([]byte, error) {
	return kdf.GenerateSalt()
}
