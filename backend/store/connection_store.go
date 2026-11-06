package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/ys-ll/uniterm/backend/session"
)

const storeFileName = "connections.json"

// PasswordStore encrypts/decrypts individual secret field values in-place.
// Implementations own the master key (OS keychain or master-password derived).
type PasswordStore interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(encoded string) (string, error)
}

type ConnectionStore struct {
	configDir     string
	passwordStore PasswordStore        // nil = encrypted fields pass through untouched (backward compat)
	legacy        LegacyPasswordSource // optional fallback to pre-enc:v1 keychain entries
	mu            sync.Mutex           // serializes Save + Load lazy re-encrypts (STORE-05/06).
	lastSavedHash string               // F-105: skip no-op rewrites keyed by canonical content hash.
}

func NewConnectionStore(configDir string) (*ConnectionStore, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}
	return &ConnectionStore{configDir: configDir}, nil
}

// SetPasswordStore sets the external password store. Once set, secret fields
// are written encrypted and decrypted on load.
func (s *ConnectionStore) SetPasswordStore(ps PasswordStore) {
	s.passwordStore = ps
}

// SetLegacyKeychain wires the pre-enc:v1 keychain so a connection that lacks
// an enc:v1 field can still recover its password from the legacy conn/<id>
// entry. Lazy-migrates the value into enc:v1 on the next save.
func (s *ConnectionStore) SetLegacyKeychain(kc LegacyPasswordSource) {
	s.legacy = kc
}

func (s *ConnectionStore) filePath() string {
	return filepath.Join(s.configDir, storeFileName)
}

func (s *ConnectionStore) Save(data session.ConnectionStoreData) error {
	return s.save(data, false)
}

// ForceSave rewrites the file even when the content hash matches the
// seeded state. Used by re-keying: the plaintext is unchanged, so the
// F-105 hash-skip would otherwise swallow the re-encryption and leave
// the file under the old key.
func (s *ConnectionStore) ForceSave(data session.ConnectionStoreData) error {
	return s.save(data, true)
}

func (s *ConnectionStore) save(data session.ConnectionStoreData, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deep-copy connections so we don't mutate the caller's backing array
	connections := make([]session.ConnectionConfig, len(data.Connections))
	copy(connections, data.Connections)

	for i := range connections {
		conn := &connections[i]
		if err := session.ValidateTOTPConfig(*conn); err != nil {
			return err
		}
		if conn.AuthType == "identity" {
			// Identity connections obtain credentials solely from the
			// referenced identity (MaterializeIdentity resolves them at
			// connect time). A stale password left when switching away from
			// another auth mode is unused — and, if it still holds an
			// enc:v1: field, would be carried verbatim into cloud sync,
			// surfacing as a literal enc:v1: on other devices (issue #711).
			// User, however, is the explicit username override typed into
			// the connection form (issue #959) — plain text by definition,
			// so it is kept as-is.
			conn.Password = ""
		}
	}

	saveData := session.ConnectionStoreData{
		Groups:      data.Groups,
		Connections: connections,
	}
	// Shared Save envelope (filestore.go): hash-skip, encrypt, atomic write.
	skip := s.lastSavedHash
	if force {
		// Re-keying saves unchanged plaintext under a new key — the
		// F-105 hash-skip would otherwise swallow the rewrite.
		skip = forceWrite
	}
	h, err := saveEncryptedFile(storeFileName, s.filePath(), &saveData, s.passwordStore, skip)
	s.lastSavedHash = h
	if errors.Is(err, ErrNoCipher) {
		return errors.New("passwordStore not initialized; refusing to save plaintext password")
	}
	return err
}

func (s *ConnectionStore) Load() (session.ConnectionStoreData, error) {
	raw, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return session.ConnectionStoreData{
				Groups:      []session.ConnectionGroup{},
				Connections: []session.ConnectionConfig{},
			}, nil
		}
		return session.ConnectionStoreData{}, err
	}

	// Try new format first: {"groups": [...], "connections": [...]}
	data, res, err := loadEncryptedData[session.ConnectionStoreData](storeFileName, raw, s.passwordStore)
	if err == nil && (data.Groups != nil || data.Connections != nil) {
		return s.finishLoaded(&data, res), nil
	}
	if err != nil && !errors.Is(err, errUnparseableConfig) {
		// Parsed as the new shape but the fields wouldn't decrypt — the file
		// itself is intact, surface the error instead of quarantining.
		return session.ConnectionStoreData{}, err
	}

	// Fallback: old format — plain array of connections
	connections, res, err := loadEncryptedData[[]session.ConnectionConfig](storeFileName, raw, s.passwordStore)
	if err != nil {
		if errors.Is(err, errUnparseableConfig) {
			// STORE-09: rename corrupt JSON aside before re-attempting.
			quarantineCorrupt(s.filePath())
		}
		return session.ConnectionStoreData{}, err
	}
	return s.finishLoaded(&session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: connections,
	}, res), nil
}

// finishLoaded normalizes nil slices and applies the load-time policies the
// manifest walk can't express: the legacy keychain backfill with move
// semantics (entries are deleted once the re-encrypted copy is on disk, so a
// user-cleared password can't resurrect on the next load), and the lazy
// re-encrypt of legacy plaintext (and orphaned enc:v1 residue left by older
// auth-type-conditional builds).
func (s *ConnectionStore) finishLoaded(data *session.ConnectionStoreData, res DecryptResult) session.ConnectionStoreData {
	if data.Groups == nil {
		data.Groups = []session.ConnectionGroup{}
	}
	if data.Connections == nil {
		data.Connections = []session.ConnectionConfig{}
	}
	needsSave := res.Plaintext > 0

	// Connections backfilled from the legacy keychain are tracked so their
	// entries are deleted once the re-encrypted copy is on disk — move
	// semantics: a copy kept forever is what let a user-cleared password
	// resurrect on the next load, since the backfill is keyed on an empty
	// JSON field.
	var backfilled []string

	for i := range data.Connections {
		conn := &data.Connections[i]
		if conn.AuthType == "password" && conn.Password == "" && s.legacy != nil {
			// Pre-enc:v1: the password lived in the keychain under conn/<id>
			// and the JSON field was omitted; recover it so the value stays
			// visible without a manual re-entry. Password auth only —
			// key/keyText passphrases never went through the legacy keychain,
			// and backfilling them would break unencrypted keys. Falls back
			// to "" when neither source has it.
			if pw, err := s.legacy.GetPassword(conn.ID); err == nil && pw != "" {
				conn.Password = pw
				backfilled = append(backfilled, conn.ID)
				needsSave = true // lazy-migrate into enc:v1 on next save
			}
			continue
		}
	}

	if needsSave {
		clean := *data
		clean.Connections = make([]session.ConnectionConfig, len(data.Connections))
		copy(clean.Connections, data.Connections)
		// Best-effort lazy re-encrypt: a missing cipher or an encrypt error
		// leaves the file untouched; the plaintext re-encrypts on next Save.
		// forceWrite bypasses the hash-skip: the plaintext content is
		// unchanged on disk, so the seeded hash would match and the lazy
		// encryption would never run (docs 4.3). On error the envelope
		// returns "" so the next Save retries the re-encrypt.
		h, _ := saveEncryptedFile(storeFileName, s.filePath(), &clean, s.passwordStore, forceWrite)
		s.mu.Lock()
		s.lastSavedHash = h
		s.mu.Unlock()
		// The re-encrypted copy is on disk — delete the legacy entries (move
		// semantics: a kept-forever copy is what let a user-cleared password
		// resurrect on the next load, since the backfill keys on an empty
		// JSON field). Best-effort: a failed delete leaves a stale entry that
		// only resurfaces if that password is cleared later.
		for _, id := range backfilled {
			_ = s.legacy.DeletePassword(id)
		}
	} else {
		// Seed the skip-state from the just-loaded content so an unchanged
		// first Save after startup skips the rewrite.
		if hashHex, err := jsonHash(*data); err == nil {
			s.mu.Lock()
			s.lastSavedHash = hashHex
			s.mu.Unlock()
		}
	}
	return *data
}

// BlobName, ExportPlain and ImportPlain implement the sync exchange contract
// (SyncableStore, declared in backend/sync/backend.go).

func (s *ConnectionStore) BlobName() string { return storeFileName }

func (s *ConnectionStore) ExportPlain() ([]byte, error) {
	return exportPlainStore(storeFileName, s.filePath(), s.Load)
}

func (s *ConnectionStore) ImportPlain(raw []byte) error {
	return importPlainStore(raw, s.Save)
}
