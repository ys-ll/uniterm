package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/ys-ll/uniterm/backend/session"
)

const tunnelsFileName = "tunnels.json"

// TunnelStore persists the user's SSH tunnel definitions to tunnels.json in the
// app config dir, mirroring QuickCommandsStore.
type TunnelStore struct {
	mu            sync.Mutex
	configDir     string
	passwordStore PasswordStore
	lastSavedHash string // F-105 skip state (pre-encryption content hash)
}

func NewTunnelStore(configDir string) *TunnelStore {
	return &TunnelStore{configDir: configDir}
}

// SetPasswordStore sets the external password store used to encrypt the
// upstream proxy password (manifest path tunnels.*.upstream.pass). Once set,
// existing plaintext fields are lazily re-encrypted on the next save.
func (s *TunnelStore) SetPasswordStore(ps PasswordStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.passwordStore = ps
}

func (s *TunnelStore) filePath() string {
	return filepath.Join(s.configDir, tunnelsFileName)
}

// Shared Save envelope (filestore.go): hash-skip, encrypt, atomic write.
func (s *TunnelStore) Save(data session.TunnelStoreData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := saveEncryptedFile(tunnelsFileName, s.filePath(), &data, s.passwordStore, s.lastSavedHash)
	s.lastSavedHash = h
	return err
}

// ForceSave bypasses the F-105 hash-skip. Used by re-keying: the plaintext is
// unchanged, so the skip would otherwise swallow the re-encryption and leave
// the file under the old key.
func (s *TunnelStore) ForceSave(data session.TunnelStoreData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := saveEncryptedFile(tunnelsFileName, s.filePath(), &data, s.passwordStore, "")
	if err == nil {
		s.lastSavedHash = h
	}
	return err
}

func (s *TunnelStore) Load() (session.TunnelStoreData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out, _, hash, existed, err := loadEncryptedFile[session.TunnelStoreData](tunnelsFileName, s.filePath(), s.passwordStore)
	if !existed {
		return session.TunnelStoreData{Version: 1, Groups: []session.TunnelGroup{}, Tunnels: []session.Tunnel{}}, nil
	}
	if err != nil {
		return session.TunnelStoreData{}, err
	}
	if out.Version == 0 {
		out.Version = 1
	}
	s.lastSavedHash = hash
	return out, nil
}

// BlobName, ExportPlain and ImportPlain implement the sync exchange contract
// (SyncableStore, declared in backend/sync/backend.go).

func (s *TunnelStore) BlobName() string { return tunnelsFileName }

func (s *TunnelStore) ExportPlain() ([]byte, error) {
	return exportPlainStore(tunnelsFileName, s.filePath(), s.Load)
}

// ImportPlain backfills Version before saving, mirroring Load.
func (s *TunnelStore) ImportPlain(raw []byte) error {
	var data session.TunnelStoreData
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse sync blob: %w", err)
	}
	if data.Version == 0 {
		data.Version = 1
	}
	return s.Save(data)
}
