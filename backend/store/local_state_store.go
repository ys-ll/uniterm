package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const localStateFileName = "local_state.json"

type LocalState struct {
	SidebarVisible    bool     `json:"sidebarVisible"`
	AISidebarVisible  bool     `json:"aiSidebarVisible"`
	CollapsedGroupIds []string `json:"collapsedGroupIds"`
	WindowX           int      `json:"windowX"`
	WindowY           int      `json:"windowY"`
	WindowWidth       int      `json:"windowWidth"`
	WindowHeight      int      `json:"windowHeight"`
	WindowMaximised   bool     `json:"windowMaximised"`
	// Background image — local-only appearance, never synced.
	BackgroundEnabled bool   `json:"backgroundEnabled"`
	BackgroundImage   string `json:"backgroundImage"`
	BackgroundOpacity int    `json:"backgroundOpacity"`
	BackgroundBlur    int    `json:"backgroundBlur"`
	BackgroundFit     string `json:"backgroundFit"`
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

func defaultLocalState() LocalState {
	return LocalState{
		SidebarVisible:    true,
		AISidebarVisible:  true,
		BackgroundOpacity: 60,
		BackgroundBlur:    3,
		BackgroundFit:     "cover",
	}
}

func (s *LocalStateStore) Load() (LocalState, error) {
	bytes, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return defaultLocalState(), nil
		}
		return LocalState{}, err
	}
	var state LocalState
	if err := json.Unmarshal(bytes, &state); err != nil {
		return defaultLocalState(), nil
	}
	return state, nil
}
