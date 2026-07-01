package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const recentFileName = "recent.json"
const maxRecent = 10

type RecentStore struct {
	filePath string
	mu       sync.RWMutex
	ids      []string
}

func NewRecentStore(configDir string) *RecentStore {
	return &RecentStore{
		filePath: filepath.Join(configDir, recentFileName),
		ids:      make([]string, 0),
	}
}

func (s *RecentStore) Load() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.ids = make([]string, 0)
			result := make([]string, len(s.ids))
			copy(result, s.ids)
			return result, nil
		}
		return nil, err
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		// Corrupted file — reset
		s.ids = make([]string, 0)
		result := make([]string, len(s.ids))
		copy(result, s.ids)
		return result, nil
	}
	s.ids = ids
	result := make([]string, len(s.ids))
	copy(result, s.ids)
	return result, nil
}

func (s *RecentStore) Record(id string) error {
	if id == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.ids) > 0 && s.ids[0] == id {
		return nil
	}

	// Deduplicate
	filtered := make([]string, 0, len(s.ids))
	for _, existing := range s.ids {
		if existing != id {
			filtered = append(filtered, existing)
		}
	}
	// Prepend
	s.ids = append([]string{id}, filtered...)
	// Trim
	if len(s.ids) > maxRecent {
		s.ids = s.ids[:maxRecent]
	}

	return s.saveLocked()
}

func (s *RecentStore) GetAll() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, len(s.ids))
	copy(result, s.ids)
	return result
}

func (s *RecentStore) saveLocked() error {
	data, err := json.Marshal(s.ids)
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}
