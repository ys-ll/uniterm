package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ys-ll/uniterm/backend/log"
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
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	CreatedAt int64            `json:"createdAt"`
	UpdatedAt int64            `json:"updatedAt"`
	Messages  []AIMessageEntry `json:"messages"`
}

type AIMessageEntry struct {
	ID           string        `json:"id"`
	Role         string        `json:"role"`
	Content      string        `json:"content"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	ToolCalls    []interface{} `json:"tool_calls,omitempty"`
	PendingTools []interface{} `json:"pendingTools,omitempty"`
	RawAPIMsg    string        `json:"_rawApiMsg,omitempty"`
}

type AISessionStore struct {
	configDir string
}

func NewAISessionStore() (*AISessionStore, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(configDir, "uniTerm")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, err
	}
	return &AISessionStore{configDir: appDir}, nil
}

func (s *AISessionStore) filePath() string {
	return filepath.Join(s.configDir, aiSessionFileName)
}

func (s *AISessionStore) shardDir() string {
	return filepath.Join(s.configDir, aiSessionDirName)
}

// shardPath returns the per-session shard file path. The id is reduced
// to its base name and additionally rejected if it contains any path
// separator or traversal segment after cleaning, so a malformed id
// cannot escape the per-session shard directory.
func (s *AISessionStore) shardPath(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("shard id is empty")
	}
	base := filepath.Base(id)
	if base == "." || base == "/" || base == ".." {
		return "", fmt.Errorf("invalid shard id %q", id)
	}
	if strings.ContainsAny(base, `/\`) {
		return "", fmt.Errorf("shard id contains path separator: %q", id)
	}
	return filepath.Join(s.shardDir(), base+".json"), nil
}

// Save writes each session as its own shard under ai-sessions/<id>.json.
// Per-shard writes eliminate the O(total-size) memory spike from a single
// full-file marshal. Orphan shards (sessions no longer in data) are
// removed.
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
		path, err := s.shardPath(sess.ID)
		if err != nil {
			return fmt.Errorf("shard path %s: %w", sess.ID, err)
		}
		payload, err := json.Marshal(sess)
		if err != nil {
			return fmt.Errorf("marshal session %s: %w", sess.ID, err)
		}
		if err := atomicWriteFile(path, payload, 0600); err != nil {
			return fmt.Errorf("write shard %s: %w", sess.ID, err)
		}
		wanted[sess.ID] = struct{}{}
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
		if rmErr := os.Remove(filepath.Join(s.shardDir(), name)); rmErr != nil && !os.IsNotExist(rmErr) {
			log.Writef("ai_session: stale shard remove failed: %v", rmErr)
		}
	}

	return nil
}

// Load reads every shard in ai-sessions/ and decodes them. Runs the
// one-time migration from the legacy ai-sessions.json file if present.
func (s *AISessionStore) Load() (AISessionData, error) {
	if err := s.migrateLegacy(); err != nil {
		// Migration failures must not block load — log so the user can
		// diagnose a corrupted legacy file, but always fall through to
		// the shard scan so they keep their session history.
		log.Writef("ai_session_store: legacy migration failed, continuing with shard scan: %v", err)
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
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var sess AISessionEntry
		if err := json.Unmarshal(raw, &sess); err != nil {
			quarantineCorrupt(path)
			continue
		}
		if sess.ID == "" {
			continue
		}
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
	raw, err := os.ReadFile(s.filePath())
	if err != nil {
		return err
	}
	var legacy AISessionData
	if err := json.Unmarshal(raw, &legacy); err != nil {
		quarantineCorrupt(s.filePath())
		return nil
	}
	if len(legacy.Sessions) > 0 {
		if err := s.Save(legacy); err != nil {
			return err
		}
	}
	return os.Remove(s.filePath())
}
