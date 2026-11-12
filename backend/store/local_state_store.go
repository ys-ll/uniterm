package store

import (
	"encoding/json"
	"path/filepath"
)

const localStateFileName = "local_state.json"

type LocalState struct {
	AI               *AILocalState `json:"ai,omitempty"`
	SidebarVisible   bool          `json:"sidebarVisible"`
	AISidebarVisible bool          `json:"aiSidebarVisible"`
	// Bottom panel (docked panel under the tab area) visibility. Local-only
	// UI state, never synced.
	BottomPanelVisible bool `json:"bottomPanelVisible"`
	// Sidebar widths in px. Local-only UI state, never synced; 0 means an
	// older config without the key and keeps the component default.
	SidebarWidth      int      `json:"sidebarWidth"`
	AISidebarWidth    int      `json:"aiSidebarWidth"`
	CollapsedGroupIds []string `json:"collapsedGroupIds"`
	// Collapsed quick-command group ids (plus "__ungrouped__"). Local-only
	// UI state, never synced; groups absent from the list stay expanded.
	CollapsedQuickCommandGroupIds []string `json:"collapsedQuickCommandGroupIds"`
	WindowX                       int      `json:"windowX"`
	WindowY                       int      `json:"windowY"`
	WindowWidth                   int      `json:"windowWidth"`
	WindowHeight                  int      `json:"windowHeight"`
	WindowMaximised               bool     `json:"windowMaximised"`
	// Background image — local-only appearance, never synced.
	BackgroundEnabled bool   `json:"backgroundEnabled"`
	BackgroundImage   string `json:"backgroundImage"`
	BackgroundOpacity int    `json:"backgroundOpacity"`
	BackgroundBlur    int    `json:"backgroundBlur"`
	BackgroundFit     string `json:"backgroundFit"`
	// SystemTitleBar switches the window to the OS native frame instead of
	// the built-in one. Read at startup only — changing it needs a restart.
	SystemTitleBar bool `json:"systemTitleBar"`
	// ExternalEditor command used to open remote files in an external editor
	// (SFTP "edit externally"). Local-only preference, never synced.
	ExternalEditor string `json:"externalEditor"`
	// SftpHiddenColumns lists SFTP file-list columns hidden via the header
	// context menu (type/modTime/size/permission/owner/group). Local-only UI
	// state, never synced; columns absent from the list stay visible.
	SftpHiddenColumns []string `json:"sftpHiddenColumns,omitempty"`
	// SftpDialogDir is the last directory used by the SFTP upload/download
	// pickers (one shared path for both). Local-only, never synced. The
	// backend skips it when it no longer exists (resolveDialogDir).
	SftpDialogDir string `json:"sftpDialogDir,omitempty"`
	// MCP configures the external-agent MCP server (see MCPSettings).
	// Pointer + omitempty so local_state.json written by older builds still
	// loads; nil means the feature is off with defaults. Lives in local
	// state on purpose — the server is a per-device capability (it exposes
	// THIS machine's terminal sessions) and must never sync across devices.
	MCP *MCPSettings `json:"mcp,omitempty"`
	// LastTabsSnapshot holds the open-tabs snapshot used by "reopen last
	// session's tabs" (issue #937). Opaque JSON written by the frontend
	// (types/tabSnapshot.ts); local-only, never synced, and holds no
	// secrets — host references are connectionIds re-resolved at restore.
	LastTabsSnapshot json.RawMessage `json:"lastTabsSnapshot,omitempty"`
	// CommandSendDraft keeps the command-send panel's edited content across
	// restarts (issue #1341). Opaque text written by the frontend; local-only,
	// never synced. Pointer + omitempty so older local_state.json still loads.
	CommandSendDraft *string `json:"commandSendDraft,omitempty"`
	// CommandSendWrap toggles word wrap in the command-send editor.
	// Local-only UI preference, never synced; true for older configs keeps
	// the wrapped default.
	CommandSendWrap bool `json:"commandSendWrap"`
}

// AILocalState holds device-local selections, separate from the synced model catalog.
type AILocalState struct {
	ActiveModelID    string                           `json:"activeModelId"`
	ModelPreferences map[string]AIReasoningPreference `json:"modelPreferences"`
}

type AIReasoningPreference struct {
	ReasoningEffort string `json:"reasoningEffort"`
}

type LocalStateStore struct {
	configDir     string
	lastSavedHash string // F-105 skip state (pre-encryption content hash)
}

func NewLocalStateStore(configDir string) *LocalStateStore {
	return &LocalStateStore{configDir: configDir}
}

func (s *LocalStateStore) filePath() string {
	return filepath.Join(s.configDir, localStateFileName)
}

// Shared Save envelope (filestore.go): hash-skip, encrypt, atomic write.
func (s *LocalStateStore) Save(state LocalState) error {
	h, err := saveEncryptedFile(localStateFileName, s.filePath(), &state, nil, s.lastSavedHash)
	s.lastSavedHash = h
	return err
}

// Window minimums, mirrored from the window options in main.go. A smaller
// report comes from a minimised or tearing-down window and would restore an
// invisible or minuscule window.
const (
	MinWindowWidth  = 500
	MinWindowHeight = 450
)

// SaveGeometry merges window geometry into the current local state and saves
// it. Too-small sizes are dropped and reported as success so a quit at an
// awkward moment never loses the previous geometry.
func (s *LocalStateStore) SaveGeometry(x, y, width, height int, maximised bool) error {
	if width < MinWindowWidth || height < MinWindowHeight {
		return nil
	}
	ls, err := s.Load()
	if err != nil {
		return err
	}
	ls.WindowX = x
	ls.WindowY = y
	ls.WindowWidth = width
	ls.WindowHeight = height
	ls.WindowMaximised = maximised
	return s.Save(ls)
}

func defaultLocalState() LocalState {
	return LocalState{
		SidebarVisible:                true,
		AISidebarVisible:              true,
		SidebarWidth:                  240,
		AISidebarWidth:                360,
		CollapsedQuickCommandGroupIds: []string{},
		BackgroundOpacity:             60,
		BackgroundBlur:                3,
		BackgroundFit:                 "cover",
		CommandSendWrap:               true,
	}
}

func (s *LocalStateStore) Load() (LocalState, error) {
	out, _, hash, existed, err := loadEncryptedFileBase[LocalState](localStateFileName, s.filePath(), defaultLocalState(), nil)
	if !existed || err != nil {
		return defaultLocalState(), nil
	}
	s.lastSavedHash = hash
	return out, nil
}
