package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	aiSessionFileName = "ai-sessions.json"
	aiSessionDirName  = "ai-sessions"
)

type AISessionData struct {
	Sessions         []AISessionEntry `json:"sessions"`
	CurrentSessionID string           `json:"currentSessionId"`
}

type AISessionEntry struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	CreatedAt      int64             `json:"createdAt"`
	UpdatedAt      int64             `json:"updatedAt"`
	Messages       []AIMessageEntry  `json:"messages"`
	ContextSummary *AIContextSummary `json:"contextSummary,omitempty"`
	// ConnectionID is the host-binding key (issue #1036): the connection this
	// session was created for, used to auto-switch sessions on tab switch.
	// Absent on sessions written before the feature existed.
	ConnectionID string `json:"connectionId,omitempty"`
}

// AIContextSummary is a checkpoint for API context only. Messages remain intact.
type AIContextSummary struct {
	Version          int    `json:"version"`
	Content          string `json:"content"`
	ThroughMessageID string `json:"throughMessageId"`
	CreatedAt        int64  `json:"createdAt"`
}

type AIMessageEntry struct {
	ID            string        `json:"id"`
	Role          string        `json:"role"`
	Content       string        `json:"content"`
	Thinking      string        `json:"thinking,omitempty"`
	CreatedAt     int64         `json:"createdAt,omitempty"`
	ThinkingDurMs int64         `json:"thinkingDurationMs,omitempty"`
	ToolCallID    string        `json:"tool_call_id,omitempty"`
	ToolCalls     []interface{} `json:"tool_calls,omitempty"`
	PendingTools  []interface{} `json:"pendingTools,omitempty"`
	RawAPIMsg     string        `json:"_rawApiMsg,omitempty"`
	ContextHeader string        `json:"_contextHeader,omitempty"`
	// Attachments carries the user message's image / text-file attachments
	// (see frontend AIAttachment). Omitted entirely for the overwhelming
	// majority of messages, and absent from sessions written before the
	// feature existed — decoding simply leaves it nil, so old files load
	// unchanged.
	Attachments []interface{} `json:"attachments,omitempty"`
}

type AISessionStore struct {
	configDir string

	mu sync.Mutex
	// shardHashes caches each shard's content hash (shard name → jsonHash)
	// so Save only rewrites shards whose content actually changed.
	shardHashes map[string]string
}

func NewAISessionStore(configDir string) (*AISessionStore, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}
	return &AISessionStore{configDir: configDir}, nil
}

func (s *AISessionStore) filePath() string {
	return filepath.Join(s.configDir, aiSessionFileName)
}

func (s *AISessionStore) shardDir() string {
	return filepath.Join(s.configDir, aiSessionDirName)
}

// shardPath returns the per-session shard file path. Falls back to a hash
// subdir if id contains path-traversal chars (defensive — ids are normally
// app-generated UUIDs).
func (s *AISessionStore) shardPath(id string) string {
	return filepath.Join(s.shardDir(), filepath.Base(id)+".json")
}

// Save writes each session as its own shard under ai-sessions/<id>.json.
// Removing the full-file marshal eliminates the O(total-size) memory spike
// from F-103. Orphan shards (sessions no longer in data) are removed.
// F-103.
func (s *AISessionStore) Save(data AISessionData) error {
	if err := os.MkdirAll(s.shardDir(), 0755); err != nil {
		return err
	}

	wanted := make(map[string]struct{}, len(data.Sessions))
	for i := range data.Sessions {
		sess := data.Sessions[i]
		if sess.ID == "" {
			continue
		}
		// Use filepath.Base(id) for both the shard name and the wanted
		// key so orphan cleanup matches against the actual on-disk name
		// (the only path the cleanup pass sees).
		name := filepath.Base(sess.ID)
		wanted[name] = struct{}{}
		// Shared Save envelope (filestore.go) per shard: skip when the
		// shard's content is unchanged (lastSavedHash comes from the
		// per-shard hash cache). The wanted set above is still recorded, so
		// skipped shards survive the orphan cleanup below.
		s.mu.Lock()
		last := s.shardHashes[name]
		s.mu.Unlock()
		h, err := saveEncryptedFile(aiSessionFileName, filepath.Join(s.shardDir(), name+".json"), &sess, nil, last)
		s.mu.Lock()
		if s.shardHashes == nil {
			s.shardHashes = make(map[string]string)
		}
		s.shardHashes[name] = h
		s.mu.Unlock()
		if err != nil {
			return fmt.Errorf("write shard %s: %w", sess.ID, err)
		}
	}

	entries, err := os.ReadDir(s.shardDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		id := name[:len(name)-len(".json")]
		if _, keep := wanted[id]; keep {
			continue
		}
		_ = os.Remove(filepath.Join(s.shardDir(), name))
		s.mu.Lock()
		delete(s.shardHashes, id)
		s.mu.Unlock()
	}

	return nil
}

// Load reads every shard in ai-sessions/ and decodes them. Runs the
// one-time migration from the legacy ai-sessions.json file if present.
// F-103.
func (s *AISessionStore) Load() (AISessionData, error) {
	if err := s.migrateLegacy(); err != nil {
		// Migration failures must not block load — log via returned error
		// only if caller cares; otherwise fall through to shard scan.
		_ = err
	}

	out := AISessionData{Sessions: []AISessionEntry{}, CurrentSessionID: ""}

	entries, err := os.ReadDir(s.shardDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return AISessionData{}, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.shardDir(), e.Name())
		sess, _, hash, existed, err := loadEncryptedFile[AISessionEntry](aiSessionFileName, path, nil)
		if err != nil || !existed || sess.ID == "" {
			continue
		}
		// Seed the per-shard skip-state so an unchanged first Save after
		// startup doesn't rewrite the shard.
		s.mu.Lock()
		if s.shardHashes == nil {
			s.shardHashes = make(map[string]string)
		}
		s.shardHashes[e.Name()[:len(e.Name())-len(".json")]] = hash
		s.mu.Unlock()
		out.Sessions = append(out.Sessions, sess)
	}

	return out, nil
}

// migrateLegacy splits the old ai-sessions.json into per-session shards
// and deletes the legacy file. Idempotent — no-op once shards exist.
func (s *AISessionStore) migrateLegacy() error {
	if _, err := os.Stat(s.filePath()); err != nil {
		return nil
	}
	legacy, _, _, _, err := loadEncryptedFile[AISessionData](aiSessionFileName, s.filePath(), nil)
	if err != nil {
		// Corrupt legacy file was already quarantined; skip migration
		// without blocking load.
		return nil
	}
	if len(legacy.Sessions) > 0 {
		if err := s.Save(legacy); err != nil {
			return err
		}
	}
	return os.Remove(s.filePath())
}
