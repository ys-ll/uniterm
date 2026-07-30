package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/ys-ll/uniterm/backend/session"
)

const storeFileName = "connections.json"

// PasswordStore is the interface for reading/writing connection passwords and AI model API keys.
// Implementations store secrets externally (e.g. OS keychain).
type PasswordStore interface {
	GetPassword(connID string) (string, error)
	SetPassword(connID, password string) error
	DeletePassword(connID string) error

	GetModelAPIKey(modelID string) (string, error)
	SetModelAPIKey(modelID, apiKey string) error
	DeleteModelAPIKey(modelID string) error
}

type ConnectionStore struct {
	configDir     string
	passwordStore PasswordStore // nil = passwords kept in JSON (backward compat)
	mu            sync.Mutex    // serializes Save + populatePasswords writes (STORE-05/06).
	pwdMu         sync.RWMutex  // guards pwdCache for async keychain fill.
	pwdCache      map[string]string
	lastSavedHash string // skip no-op rewrites keyed by canonical content hash.
}

func NewConnectionStore() (*ConnectionStore, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(configDir, "uniTerm")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, err
	}
	return &ConnectionStore{configDir: appDir}, nil
}

// SetPasswordStore sets the external password store. Once set, passwords
// are written to the store and cleared from the JSON file on save.
func (s *ConnectionStore) SetPasswordStore(ps PasswordStore) {
	s.passwordStore = ps
}

func (s *ConnectionStore) filePath() string {
	return filepath.Join(s.configDir, storeFileName)
}

func (s *ConnectionStore) Save(data session.ConnectionStoreData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deep-copy connections so we don't mutate the caller's backing array
	connections := make([]session.ConnectionConfig, len(data.Connections))
	copy(connections, data.Connections)

	// Extract passwords to external store before writing JSON
	for i := range connections {
		conn := &connections[i]
		if conn.AuthType != "password" {
			continue
		}
		if conn.Password == "" {
			// Password was cleared - remove old entry from keychain.
			if s.passwordStore != nil {
				_ = s.passwordStore.DeletePassword(conn.ID)
			}
			continue
		}
		if s.passwordStore == nil {
			// Fail closed: never write a plaintext password to disk when the
			// keychain isn't available. STORE-04.
			return errors.New("passwordStore not initialized; refusing to save plaintext password")
		}
		if err := s.passwordStore.SetPassword(conn.ID, conn.Password); err != nil {
			return err
		}
		conn.Password = ""
	}

	saveData := session.ConnectionStoreData{
		Groups:      data.Groups,
		Connections: connections,
	}
	return s.writeJSONLocked(saveData)
}

// writeJSONLocked serializes data to the connections file atomically.
// Uses json.NewEncoder to stream directly to the temp file (no intermediate
// buffer the size of the output), and skips the temp+sync+rename cycle when
// the canonical content hash matches the last successful save. Caller must
// hold s.mu.
func (s *ConnectionStore) writeJSONLocked(data session.ConnectionStoreData) error {
	preview, err := json.Marshal(data)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(preview)
	hashHex := hex.EncodeToString(sum[:])
	if hashHex == s.lastSavedHash {
		return nil
	}

	path := s.filePath()
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	s.lastSavedHash = hashHex
	return nil
}

func (s *ConnectionStore) Load() (session.ConnectionStoreData, error) {
	fileData, err := os.ReadFile(s.filePath())
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
	var data session.ConnectionStoreData
	if err := json.Unmarshal(fileData, &data); err == nil && (data.Groups != nil || data.Connections != nil) {
		if data.Groups == nil {
			data.Groups = []session.ConnectionGroup{}
		}
		if data.Connections == nil {
			data.Connections = []session.ConnectionConfig{}
		}
		if err := s.populatePasswords(&data); err != nil {
			return session.ConnectionStoreData{}, err
		}
		return data, nil
	}

	// Fallback: old format — plain array of connections
	var connections []session.ConnectionConfig
	if err := json.Unmarshal(fileData, &connections); err != nil {
		// STORE-09: rename corrupt JSON aside before re-attempting.
		quarantineCorrupt(s.filePath())
		return session.ConnectionStoreData{}, err
	}
	data = session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: connections,
	}
	if err := s.populatePasswords(&data); err != nil {
		return session.ConnectionStoreData{}, err
	}
	return data, nil
}

func (s *ConnectionStore) populatePasswords(data *session.ConnectionStoreData) error {
	needsSave := false
	var toFetch []string

	for i := range data.Connections {
		conn := &data.Connections[i]
		if conn.AuthType != "password" {
			continue
		}

		if s.passwordStore != nil {
			// One-time migration: plaintext JSON password → keychain.
			if conn.Password != "" {
				if err := s.passwordStore.SetPassword(conn.ID, conn.Password); err != nil {
					return err
				}
				conn.Password = ""
				needsSave = true
			}
		}

		// Serve from cache when available, otherwise schedule an async
		// keychain fill so Load() does not block on per-connection IPC.
		s.pwdMu.RLock()
		pw, cached := s.pwdCache[conn.ID]
		s.pwdMu.RUnlock()
		if cached && pw != "" {
			conn.Password = pw
			continue
		}
		if s.passwordStore != nil {
			toFetch = append(toFetch, conn.ID)
		}
	}

	if len(toFetch) > 0 {
		go s.asyncFillPasswords(toFetch)
	}

	if needsSave {
		// Save cleaned JSON (passwords migrated out)
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.writeJSONLocked(*data)
	}
	return nil
}

// asyncFillPasswords runs after Load() returns; it fetches each requested
// password from the keychain and writes the result into pwdCache. A cache
// hit on the next Load avoids the per-connection IPC entirely.
func (s *ConnectionStore) asyncFillPasswords(ids []string) {
	for _, id := range ids {
		pw, err := s.passwordStore.GetPassword(id)
		if err != nil || pw == "" {
			continue
		}
		s.pwdMu.Lock()
		if s.pwdCache == nil {
			s.pwdCache = map[string]string{}
		}
		s.pwdCache[id] = pw
		s.pwdMu.Unlock()
	}
}

// EnsurePassword returns the password for a connection ID, populating from
// the keychain on cache miss. Callers needing synchronous access (e.g.
// SSH Connect right after Load) should call this instead of relying on
// conn.Password, which may be empty for the first Load after process start.
// Returns "" if no password store is set or the keychain has no entry.
func (s *ConnectionStore) EnsurePassword(connID string) (string, error) {
	s.pwdMu.RLock()
	pw, ok := s.pwdCache[connID]
	s.pwdMu.RUnlock()
	if ok {
		return pw, nil
	}
	if s.passwordStore == nil {
		return "", nil
	}
	pw, err := s.passwordStore.GetPassword(connID)
	if err != nil {
		return "", err
	}
	if pw != "" {
		s.pwdMu.Lock()
		if s.pwdCache == nil {
			s.pwdCache = map[string]string{}
		}
		s.pwdCache[connID] = pw
		s.pwdMu.Unlock()
	}
	return pw, nil
}
