package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const aiConfigFileName = "ai.json"

// AIStoreData is the syncable slice of the AI settings: the model catalog,
// agent turn limit and context preferences. It lives in its own ai.json so it can ride the
// cloud sync while the rest of settings.json stays device-local. The active
// model choice lives in local_state.json — it is per-device
// "current selection" state, not a shareable asset.
type AIStoreData struct {
	MaxTurns           *int            `json:"maxTurns"`
	// Nil keeps automatic compaction enabled for older configurations.
	AutoCompact        *bool           `json:"autoCompact,omitempty"`
	ContextTokenBudget int             `json:"contextTokenBudget,omitempty"`
	SummaryModelID     string          `json:"summaryModelId,omitempty"`
	// UserMessageStyle controls how user messages render in the AI panel:
	// "" (plain, default) keeps the shared left-aligned full-width layout,
	// "bubble" right-aligns a content-sized chat bubble (issue #1478).
	UserMessageStyle string          `json:"userMessageStyle,omitempty"`
	Models           []AIModelConfig `json:"models"`
}

type AIConfigStore struct {
	configDir     string
	passwordStore PasswordStore
	mu            sync.Mutex
	lastSavedHash string // F-105 skip state (pre-encryption content hash)
}

func NewAIConfigStore(configDir string) (*AIConfigStore, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}
	return &AIConfigStore{configDir: configDir}, nil
}

func (s *AIConfigStore) SetPasswordStore(ps PasswordStore) {
	s.passwordStore = ps
}

func (s *AIConfigStore) filePath() string {
	return filepath.Join(s.configDir, aiConfigFileName)
}

// Exists reports whether ai.json is on disk yet. The one-shot settings→ai
// migration uses this to stay idempotent.
func (s *AIConfigStore) Exists() bool {
	_, err := os.Stat(s.filePath())
	return err == nil
}

// Save writes ai.json, encrypting model apiKeys in place (same policy as
// settings.json: best-effort — when no passwordStore is wired the values
// are kept as-is rather than failing the save).
func (s *AIConfigStore) Save(cfg AIStoreData) error {
	return s.save(cfg, false)
}

// ForceSave rewrites the file even when the content hash matches the
// seeded state. Used by re-keying: the plaintext is unchanged, so the
// F-105 hash-skip would otherwise swallow the re-encryption and leave
// the file under the old key.
func (s *AIConfigStore) ForceSave(cfg AIStoreData) error {
	return s.save(cfg, true)
}

func (s *AIConfigStore) save(cfg AIStoreData, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	models := make([]AIModelConfig, len(cfg.Models))
	copy(models, cfg.Models)
	cfg.Models = models
	// Shared Save envelope (filestore.go): hash-skip, encrypt, atomic write.
	skip := s.lastSavedHash
	if force {
		// Re-keying saves unchanged plaintext under a new key — the
		// F-105 hash-skip would otherwise swallow the rewrite.
		skip = forceWrite
	}
	h, err := saveEncryptedFile(aiConfigFileName, s.filePath(), &cfg, s.passwordStore, skip)
	s.lastSavedHash = h
	return err
}

// Load reads ai.json. A missing file yields the default config; a corrupt
// file is quarantined before falling back, matching SettingsStore's
// STORE-09 behaviour.
func (s *AIConfigStore) Load() (AIStoreData, error) {
	cfg, res, hash, existed, err := loadEncryptedFile[AIStoreData](aiConfigFileName, s.filePath(), s.passwordStore)
	if !existed {
		return defaultAIConfig(), nil
	}
	if err != nil {
		if errors.Is(err, errUnparseableConfig) {
			return defaultAIConfig(), nil
		}
		return AIStoreData{}, err
	}
	needsSave := res.Plaintext > 0
	if cfg.MaxTurns == nil {
		cfg.MaxTurns = intPtr(defaultMaxTurns)
		needsSave = true
	}
	if needsSave {
		// Force: the legacy-plaintext case changes nothing in the plaintext
		// content, so the hash-skip would swallow the lazy encryption.
		_ = s.ForceSave(cfg)
	} else {
		s.lastSavedHash = hash
	}
	return cfg, nil
}

// MigrateFromSettings seeds ai.json from the AI block of settings.json on
// first run after the split. The settings copy is deliberately left intact
// so a user rolling back to a build without ai.json still finds their
// models there. Idempotent: skipped once ai.json exists.
func (s *AIConfigStore) MigrateFromSettingsIfNeeded(settings AppSettings) error {
	if s.Exists() {
		return nil
	}
	if len(settings.AI.Models) == 0 && settings.AI.MaxTurns == nil {
		return nil
	}
	maxTurns := settings.AI.MaxTurns
	if maxTurns == nil {
		t := defaultMaxTurns
		maxTurns = &t
	}
	return s.Save(AIStoreData{MaxTurns: maxTurns, Models: settings.AI.Models,
		AutoCompact: settings.AI.AutoCompact, ContextTokenBudget: settings.AI.ContextTokenBudget,
		SummaryModelID: settings.AI.SummaryModelID, UserMessageStyle: settings.AI.UserMessageStyle})
}

// BlobName, ExportPlain and ImportPlain implement the sync exchange contract
// (SyncableStore, declared in backend/sync/backend.go).

func (s *AIConfigStore) BlobName() string { return aiConfigFileName }

func (s *AIConfigStore) ExportPlain() ([]byte, error) {
	return exportPlainStore(aiConfigFileName, s.filePath(), s.Load)
}

func (s *AIConfigStore) ImportPlain(raw []byte) error {
	return importPlainStore(raw, s.Save)
}
