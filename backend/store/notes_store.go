package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

const notesFileName = "notes.json"

// Note is one user note (issue #1204): a markdown scratchpad for scripts,
// snippets and work notes. Category is a free-form tag the notes panel
// groups and filters by (no folder hierarchy).
type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title,omitempty"`
	Category  string    `json:"category,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type NotesData struct {
	Version int    `json:"version"`
	Notes   []Note `json:"notes"`
}

type NotesStore struct {
	configDir     string
	lastSavedHash string // F-105 skip state (pre-encryption content hash)
}

func NewNotesStore(configDir string) *NotesStore {
	return &NotesStore{configDir: configDir}
}

func (s *NotesStore) filePath() string {
	return filepath.Join(s.configDir, notesFileName)
}

// Shared Save envelope (filestore.go): hash-skip, encrypt, atomic write.
func (s *NotesStore) Save(data NotesData) error {
	h, err := saveEncryptedFile(notesFileName, s.filePath(), &data, nil, s.lastSavedHash)
	s.lastSavedHash = h
	return err
}

func (s *NotesStore) Load() (NotesData, error) {
	out, _, hash, existed, err := loadEncryptedFile[NotesData](notesFileName, s.filePath(), nil)
	if !existed {
		return NotesData{Version: 1, Notes: []Note{}}, nil
	}
	if err != nil {
		return NotesData{}, err
	}
	if out.Version == 0 {
		out.Version = 1
	}
	if out.Notes == nil {
		out.Notes = []Note{}
	}
	s.lastSavedHash = hash
	return out, nil
}

// BlobName, ExportPlain and ImportPlain implement the sync exchange contract
// (SyncableStore, declared in backend/sync/backend.go).

func (s *NotesStore) BlobName() string { return notesFileName }

func (s *NotesStore) ExportPlain() ([]byte, error) {
	return exportPlainStore(notesFileName, s.filePath(), s.Load)
}

// ImportPlain backfills Version before saving so the on-disk copy always
// carries the explicit value, mirroring Load's normalization.
func (s *NotesStore) ImportPlain(raw []byte) error {
	var data NotesData
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse sync blob: %w", err)
	}
	if data.Version == 0 {
		data.Version = 1
	}
	return s.Save(data)
}
