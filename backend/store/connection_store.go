package store

import (
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
	jsonData, err := json.MarshalIndent(saveData, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.filePath(), jsonData, 0600)
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
	for i := range data.Connections {
		conn := &data.Connections[i]
		if conn.AuthType != "password" {
			continue
		}

		if s.passwordStore != nil {
			// Migration: if JSON still has plaintext password, move to keychain
			if conn.Password != "" {
				if err := s.passwordStore.SetPassword(conn.ID, conn.Password); err != nil {
					return err
				}
				conn.Password = ""
				needsSave = true
			}

			// Load password from external store
			if pw, err := s.passwordStore.GetPassword(conn.ID); err == nil && pw != "" {
				conn.Password = pw
			}
		}
	}

	if needsSave {
		// Save cleaned JSON (passwords migrated out)
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return atomicWriteFile(s.filePath(), jsonData, 0600)
	}
	return nil
}
