package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const localStateFileName = "local_state.json"

type LocalState struct {
	SidebarVisible   bool `json:"sidebarVisible"`
	AISidebarVisible bool `json:"aiSidebarVisible"`
}

type LocalStateStore struct {
	configDir string
}

func NewLocalStateStore(configDir string) *LocalStateStore {
	return &LocalStateStore{configDir: configDir}
}

func (s *LocalStateStore) filePath() string {
	return filepath.Join(s.configDir, localStateFileName)
}

func (s *LocalStateStore) Save(state LocalState) error {
	bytes, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath(), bytes, 0600)
}

func (s *LocalStateStore) Load() (LocalState, error) {
	bytes, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return LocalState{SidebarVisible: true, AISidebarVisible: true}, nil
		}
		return LocalState{}, err
	}
	var state LocalState
	if err := json.Unmarshal(bytes, &state); err != nil {
		return LocalState{SidebarVisible: true, AISidebarVisible: true}, nil
	}
	return state, nil
}
