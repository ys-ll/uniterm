package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const settingsFileName = "settings.json"

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

type TerminalSettings struct {
	Theme            string `json:"theme"`
	FontFamily       string `json:"fontFamily"`
	FontSize         int    `json:"fontSize"`
	SelectionAction  string `json:"selectionAction"`
	RightClickAction string `json:"rightClickAction"`
	MaxHistoryLines  int    `json:"maxHistoryLines"`
	SmartCompletion  *bool  `json:"smartCompletion"`
	HighlightEnabled *bool  `json:"highlightEnabled"`
}

// TerminalThemeColors mirrors xterm.js's ITheme shape: the 4 base colors
// plus the 16 ANSI colors, all as hex strings.
type TerminalThemeColors struct {
	Background    string `json:"background"`
	Foreground    string `json:"foreground"`
	Cursor        string `json:"cursor"`
	Selection     string `json:"selection"`
	Black         string `json:"black"`
	Red           string `json:"red"`
	Green         string `json:"green"`
	Yellow        string `json:"yellow"`
	Blue          string `json:"blue"`
	Magenta       string `json:"magenta"`
	Cyan          string `json:"cyan"`
	White         string `json:"white"`
	BrightBlack   string `json:"brightBlack"`
	BrightRed     string `json:"brightRed"`
	BrightGreen   string `json:"brightGreen"`
	BrightYellow  string `json:"brightYellow"`
	BrightBlue    string `json:"brightBlue"`
	BrightMagenta string `json:"brightMagenta"`
	BrightCyan    string `json:"brightCyan"`
	BrightWhite   string `json:"brightWhite"`
}

// CustomTerminalTheme is a user-defined terminal color scheme, stored
// alongside (not inside) TerminalSettings since a theme is a reusable
// resource, not a single terminal session's property.
type CustomTerminalTheme struct {
	ID     string              `json:"id"`
	Name   string              `json:"name"`
	Type   string              `json:"type"` // "dark" | "light"
	Colors TerminalThemeColors `json:"colors"`
}

// AIConfig is the legacy flat AI config type, kept for Wails binding compatibility.
// New code should use AppSettings.AI (active model from AISettings).
type AIConfig struct {
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseURL"`
	Model   string `json:"model"`
}

type AIModelConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	APIKey   string `json:"apiKey"`
	BaseURL  string `json:"baseURL"`
	Model    string `json:"model"`
	Protocol string `json:"protocol"`
}

type AISettings struct {
	MaxTurns      *int            `json:"maxTurns"`
	Models        []AIModelConfig `json:"models"`
	ActiveModelID string          `json:"activeModelId"`
}

type KeyBinding struct {
	Ctrl  bool   `json:"ctrl"`
	Shift bool   `json:"shift"`
	Alt   bool   `json:"alt"`
	Key   string `json:"key"`
}

type AppSettings struct {
	Theme                string                `json:"theme"`
	Language             string                `json:"language"`
	Terminal             TerminalSettings      `json:"terminal"`
	AI                   AISettings            `json:"ai"`
	Keyboard             map[string]KeyBinding `json:"keyboard"`
	AutoCheckUpdate      *bool                 `json:"autoCheckUpdate"`
	SFTPBookmarks        SFTPBookmarks         `json:"sftpBookmarks"`
	CustomTerminalThemes []CustomTerminalTheme `json:"customTerminalThemes"`
	DefaultLocalShell    string                `json:"defaultLocalShell"`
}

type SFTPBookmarks struct {
	LocalPaths  []string `json:"localPaths"`
	RemotePaths []string `json:"remotePaths"`
}

type SettingsStore struct {
	configDir     string
	passwordStore PasswordStore
}

func NewSettingsStore() (*SettingsStore, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(configDir, "uniTerm")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, err
	}
	return &SettingsStore{configDir: appDir}, nil
}

func (s *SettingsStore) SetPasswordStore(ps PasswordStore) {
	s.passwordStore = ps
}

func (s *SettingsStore) filePath() string {
	return filepath.Join(s.configDir, settingsFileName)
}

func (s *SettingsStore) Save(settings AppSettings) error {
	// Deep-copy models so we don't mutate the caller's backing array
	models := make([]AIModelConfig, len(settings.AI.Models))
	copy(models, settings.AI.Models)

	// Extract model apiKeys to keychain before writing JSON
	for i := range models {
		m := &models[i]
		if m.APIKey != "" && s.passwordStore != nil {
			_ = s.passwordStore.SetModelAPIKey(m.ID, m.APIKey)
		}
		m.APIKey = ""
	}

	settings.AI.Models = models
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath(), data, 0600)
}

func (s *SettingsStore) Load() (AppSettings, error) {
	data, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return defaultSettings(), nil
		}
		return AppSettings{}, err
	}
	var settings AppSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return defaultSettings(), nil
	}

	// Backfill model apiKeys from keychain; migrate if still in JSON
	needsSave := false
	for i := range settings.AI.Models {
		m := &settings.AI.Models[i]
		if s.passwordStore != nil {
			// Migration: if JSON still has plaintext apiKey, move to keychain
			if m.APIKey != "" {
				_ = s.passwordStore.SetModelAPIKey(m.ID, m.APIKey)
				m.APIKey = ""
				needsSave = true
			}
			// Backfill from keychain
			if ak, err := s.passwordStore.GetModelAPIKey(m.ID); err == nil && ak != "" {
				m.APIKey = ak
			}
		}
	}
	// Default autoCheckUpdate to true if not present
	if settings.AutoCheckUpdate == nil {
		settings.AutoCheckUpdate = boolPtr(true)
		needsSave = true
	}
	if needsSave {
		jsonData, _ := json.MarshalIndent(settings, "", "  ")
		_ = os.WriteFile(s.filePath(), jsonData, 0600)
	}

	return settings, nil
}

func defaultSettings() AppSettings {
	return AppSettings{
		Theme:    "dark",
		Language: "system",
		Terminal: TerminalSettings{
			Theme:            "uniterm-dark",
			FontFamily:       "Consolas, \"Courier New\", monospace",
			FontSize:         14,
			SelectionAction:  "none",
			RightClickAction: "menu",
			MaxHistoryLines:  5000,
		},
		AI: AISettings{
			MaxTurns: intPtr(20),
			Models: []AIModelConfig{
				{
					ID:       "model-default",
					Name:     "Default",
					APIKey:   "",
					BaseURL:  "https://api.openai.com/v1",
					Model:    "gpt-4o",
					Protocol: "anthropic",
				},
			},
			ActiveModelID: "model-default",
		},
		Keyboard:        defaultKeyboard(),
		AutoCheckUpdate: boolPtr(true),
		SFTPBookmarks: SFTPBookmarks{
			LocalPaths:  []string{},
			RemotePaths: []string{},
		},
		CustomTerminalThemes: []CustomTerminalTheme{},
	}
}

func defaultKeyboard() map[string]KeyBinding {
	return map[string]KeyBinding{
		"nextTab":          {Ctrl: true, Shift: false, Alt: false, Key: "tab"},
		"prevTab":          {Ctrl: true, Shift: true, Alt: false, Key: "tab"},
		"newConnection":    {Ctrl: true, Shift: true, Alt: false, Key: "n"},
		"toggleSidebar":    {Ctrl: true, Shift: true, Alt: false, Key: "h"},
		"focusTerminal":    {Ctrl: true, Shift: true, Alt: false, Key: "j"},
		"focusAI":          {Ctrl: true, Shift: true, Alt: false, Key: "k"},
		"lockAI":           {Ctrl: true, Shift: true, Alt: false, Key: "l"},
		"duplicateSession": {Ctrl: true, Shift: true, Alt: false, Key: "d"},
		"closePanel":       {Ctrl: true, Shift: true, Alt: false, Key: "q"},
		"navigatePrev":     {Ctrl: false, Shift: false, Alt: true, Key: "arrowleft"},
		"navigateNext":     {Ctrl: false, Shift: false, Alt: true, Key: "arrowright"},
		"openSettings":     {Ctrl: true, Shift: true, Alt: false, Key: "c"},
	}
}
