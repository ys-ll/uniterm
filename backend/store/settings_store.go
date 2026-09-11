package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ys-ll/uniterm/backend/credentials"
)

const settingsFileName = "settings.json"

func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }

type TerminalSettings struct {
	Theme             string `json:"theme"`
	FontFamily        string `json:"fontFamily"`
	FontSize          int    `json:"fontSize"`
	SelectionAction   string `json:"selectionAction"`
	RightClickAction  string `json:"rightClickAction"`
	MiddleClickAction string `json:"middleClickAction"`
	MaxHistoryLines   int    `json:"maxHistoryLines"`
	SmartCompletion   *bool  `json:"smartCompletion"`
	AiTranscription   *bool  `json:"aiTranscription"`
	HighlightEnabled  *bool  `json:"highlightEnabled"`
	// CursorBlink controls xterm.js's cursor blink. Pointer + omitempty so
	// settings.json written by older builds (which lack this field) still
	// load; the frontend falls back to `true` when nil, matching the
	// pre-existing default behaviour.
	CursorBlink *bool `json:"cursorBlink,omitempty"`
	// CtrlWheelZoom enables Ctrl/Cmd + mouse wheel font zoom. Pointer +
	// omitempty so older settings.json files (which lack this field) still
	// load; the frontend defaults to `true` when nil, preserving the existing
	// behaviour (issue #671 lets users disable it for scroll-sensitive mice).
	CtrlWheelZoom *bool `json:"ctrlWheelZoom,omitempty"`
	// SessionLogDir overrides the default directory used for session
	// output logs (issue #227). Empty means: use the OS-appropriate
	// default under ~/Documents/uniTerm/logs.
	SessionLogDir string `json:"sessionLogDir,omitempty"`
	// SessionLogFilename controls names for new output logs. Supported tokens:
	// %S session name, %H host, %M month, %D day, %h hour, %m minute.
	SessionLogFilename string `json:"sessionLogFilename,omitempty"`
	// WordSeparator overrides xterm.js's double-click word-selection
	// separators. Empty means the frontend falls back to its built-in
	// default. Mirrors the `wordSeparator` Terminal option.
	WordSeparator string `json:"wordSeparator,omitempty"`
	// FallbackFont is the secondary font family for glyphs the primary font
	// lacks (most useful for CJK). Empty means no explicit fallback.
	FallbackFont string `json:"fallbackFont,omitempty"`
	// FontWeight is the CSS weight of regular terminal text. Pointer +
	// omitempty so older settings.json files still load; the frontend
	// defaults to 400 when nil.
	FontWeight *int `json:"fontWeight,omitempty"`
	// CursorStyle is the focused-terminal cursor shape (block/underline/bar).
	// Empty means the frontend falls back to `block`.
	CursorStyle string `json:"cursorStyle,omitempty"`
	// MinimumContrast is xterm's text/background contrast boost (1 = off).
	// Pointer + omitempty so older settings.json files still load; the
	// frontend defaults to 4.5 when nil.
	MinimumContrast *float64 `json:"minimumContrast,omitempty"`
	// ShowLineNumbers / ShowTimestamps toggle the gutter columns in front of
	// the terminal. Pointer + omitempty so settings.json written by older
	// builds (which lack these fields) still loads; the frontend defaults
	// to false when nil.
	ShowLineNumbers *bool `json:"showLineNumbers,omitempty"`
	ShowTimestamps  *bool `json:"showTimestamps,omitempty"`
	// TimestampFormat renders the gutter time column (e.g. "HH:mm:ss").
	// Empty means the frontend falls back to its built-in default.
	TimestampFormat string `json:"timestampFormat,omitempty"`
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
	// UserAgent overrides the HTTP User-Agent for this model's API calls.
	// Empty means the frontend/protocol default.
	UserAgent string `json:"userAgent,omitempty"`
	// ProxyId references a saved outbound proxy (proxies.json) used for all
	// HTTP traffic to this model's BaseURL. Empty = direct connection.
	ProxyID string `json:"proxyId,omitempty"`
}

type AISettings struct {
	MaxTurns      *int            `json:"maxTurns"`
	FontSize      *int            `json:"fontSize"`
	Models        []AIModelConfig `json:"models"`
	ActiveModelID string          `json:"activeModelId"`
}

type KeyBinding struct {
	Ctrl    bool   `json:"ctrl"`
	Primary bool   `json:"primary"`
	Meta    bool   `json:"meta"`
	Shift   bool   `json:"shift"`
	Alt     bool   `json:"alt"`
	Key     string `json:"key"`
}

type AppSettings struct {
	Theme           string                `json:"theme"`
	Language        string                `json:"language"`
	Terminal        TerminalSettings      `json:"terminal"`
	AI              AISettings            `json:"ai"`
	Keyboard        map[string]KeyBinding `json:"keyboard"`
	AutoCheckUpdate *bool                 `json:"autoCheckUpdate"`
	// UpdateSource selects where update checks and downloads come from:
	// "auto" (default, picks by UI language with fallback), "github" or
	// "gitee" (domestic mirror). Pointer + omitempty so settings.json written
	// by older builds still load; nil means "auto".
	UpdateSource   *string       `json:"updateSource,omitempty"`
	CloseTabPrompt *bool         `json:"closeTabPrompt"`
	CloseAppPrompt *bool         `json:"closeAppPrompt"`
	SFTPBookmarks  SFTPBookmarks `json:"sftpBookmarks"`
	// SftpTransferPanelVisible remembers whether the SFTP transfer panel was
	// last left visible. Pointer + omitempty so settings.json written by older
	// builds (which lack this field) still load; nil means "use the frontend
	// default" (hidden).
	SftpTransferPanelVisible *bool                 `json:"sftpTransferPanelVisible,omitempty"`
	CustomTerminalThemes     []CustomTerminalTheme `json:"customTerminalThemes"`
	DefaultLocalShell        string                `json:"defaultLocalShell"`
	TabCloseButton           string                `json:"tabCloseButton"`
	// SidebarTabs toggles which connection-sidebar tab icons are visible,
	// keyed by view id (connections/files/monitor/tunnels/quickCommands/
	// history/personalization). "connections" is always shown in the UI and
	// never hidden. Pointer + omitempty so settings.json written by older
	// builds (which lack this field) still load; a nil map means "use the
	// frontend defaults" (everything visible).
	SidebarTabs map[string]bool `json:"sidebarTabs,omitempty"`
}

type SFTPBookmarks struct {
	LocalPaths  []string `json:"localPaths"`
	RemotePaths []string `json:"remotePaths"`
}

type SettingsStore struct {
	configDir     string
	passwordStore PasswordStore
	mu            sync.Mutex // serializes Save + Load migration writes (STORE-05/06).
}

func NewSettingsStore(configDir string) (*SettingsStore, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}
	return &SettingsStore{configDir: configDir}, nil
}

func (s *SettingsStore) SetPasswordStore(ps PasswordStore) {
	s.passwordStore = ps
}

func (s *SettingsStore) filePath() string {
	return filepath.Join(s.configDir, settingsFileName)
}

func (s *SettingsStore) Save(settings AppSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deep-copy models so we don't mutate the caller's backing array
	models := make([]AIModelConfig, len(settings.AI.Models))
	copy(models, settings.AI.Models)

	// Encrypt model apiKeys in place before writing JSON.
	for i := range models {
		m := &models[i]
		if m.APIKey == "" || credentials.IsEncrypted(m.APIKey) {
			continue
		}
		if s.passwordStore == nil {
			continue // no cipher — keep as-is (best effort; never leak is N/A for settings)
		}
		enc, err := s.passwordStore.Encrypt(m.APIKey)
		if err != nil {
			return err
		}
		m.APIKey = enc
	}

	settings.AI.Models = models
	// Settings file is internal — no indent. Encoder streams into the buf
	// so we skip the intermediate allocation of json.Marshal.
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(settings); err != nil {
		return err
	}
	return atomicWriteFile(s.filePath(), buf.Bytes(), 0600)
}

func (s *SettingsStore) Load() (AppSettings, error) {
	// filePath is immutable after construction, so read it without the lock.
	// Disk I/O on the settings file can be slow (cold cache, encrypted FS);
	// holding the mutex across os.ReadFile would block every concurrent Save.
	path := s.filePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultSettings(), nil
		}
		return AppSettings{}, err
	}
	var settings AppSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		// STORE-09: preserve corrupt file before falling back to defaults so
		// the next Save doesn't silently overwrite the user's prior data.
		s.mu.Lock()
		quarantineCorrupt(path)
		s.mu.Unlock()
		return defaultSettings(), nil
	}

	// Snapshot passwordStore under the lock; everything below mutates only
	// the local `settings` value, so the rest of Load runs lock-free.
	s.mu.Lock()
	ps := s.passwordStore
	s.mu.Unlock()

	// Decrypt model apiKeys; migrate legacy plaintext to encrypted on save.
	needsSave := false
	if migrateLegacyPrimaryBindings(data, settings.Keyboard) {
		needsSave = true
	}
	for i := range settings.AI.Models {
		m := &settings.AI.Models[i]
		if m.APIKey == "" || ps == nil {
			continue
		}
		if credentials.IsEncrypted(m.APIKey) {
			ak, err := ps.Decrypt(m.APIKey)
			if err != nil {
				return AppSettings{}, err
			}
			m.APIKey = ak
		} else {
			needsSave = true // legacy plaintext — re-save will encrypt
		}
	}
	// Default autoCheckUpdate to true if not present
	if settings.AutoCheckUpdate == nil {
		settings.AutoCheckUpdate = boolPtr(true)
		needsSave = true
	}
	// Default updateSource to "auto" if not present
	if settings.UpdateSource == nil {
		settings.UpdateSource = strPtr("auto")
		needsSave = true
	}
	if settings.CloseTabPrompt == nil {
		settings.CloseTabPrompt = boolPtr(true)
		needsSave = true
	}
	if settings.CloseAppPrompt == nil {
		settings.CloseAppPrompt = boolPtr(true)
		needsSave = true
	}
	if settings.AI.FontSize == nil {
		settings.AI.FontSize = intPtr(15)
		needsSave = true
	}
	if needsSave {
		// Re-save through Save() which takes the lock itself.
		_ = s.Save(settings)
	}

	return settings, nil
}

// migrateLegacyPrimaryBindings must inspect the original JSON: after normal
// unmarshalling, a missing primary field is indistinguishable from an
// explicitly saved false value. Only bindings from the old defaults are
// migrated, so custom Ctrl/Meta shortcuts retain their exact modifiers.
func migrateLegacyPrimaryBindings(data []byte, keyboard map[string]KeyBinding) bool {
	var raw struct {
		Keyboard map[string]json.RawMessage `json:"keyboard"`
	}
	if len(keyboard) == 0 || json.Unmarshal(data, &raw) != nil {
		return false
	}

	changed := false
	for action, primaryDefault := range defaultKeyboard() {
		binding, ok := keyboard[action]
		if !ok || !primaryDefault.Primary || jsonFieldPresent(raw.Keyboard[action], "primary") {
			continue
		}
		if binding.Ctrl && !binding.Meta && binding.Shift == primaryDefault.Shift &&
			binding.Alt == primaryDefault.Alt && strings.EqualFold(binding.Key, primaryDefault.Key) {
			binding.Ctrl = false
			binding.Primary = true
			keyboard[action] = binding
			changed = true
		}
	}

	// One released default used Meta+K for quick commands on every platform.
	// It is equivalent to the primary modifier on macOS and was broken on
	// Windows, so it is safe to migrate without knowing the current platform.
	if binding, ok := keyboard["openQuickCommands"]; ok &&
		!jsonFieldPresent(raw.Keyboard["openQuickCommands"], "primary") &&
		!binding.Ctrl && binding.Meta && !binding.Shift && !binding.Alt && strings.EqualFold(binding.Key, "k") {
		binding.Meta = false
		binding.Primary = true
		keyboard["openQuickCommands"] = binding
		changed = true
	}
	return changed
}

func jsonFieldPresent(data json.RawMessage, field string) bool {
	if len(data) == 0 {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil {
		return false
	}
	_, ok := object[field]
	return ok
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
			FontSize: intPtr(15),
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
		UpdateSource:    strPtr("auto"),
		CloseTabPrompt:  boolPtr(true),
		CloseAppPrompt:  boolPtr(true),
		SFTPBookmarks: SFTPBookmarks{
			LocalPaths:  []string{},
			RemotePaths: []string{},
		},
		CustomTerminalThemes: []CustomTerminalTheme{},
	}
}

func defaultKeyboard() map[string]KeyBinding {
	return map[string]KeyBinding{
		"nextTab":                 {Primary: true, Shift: false, Alt: false, Key: "tab"},
		"prevTab":                 {Primary: true, Shift: true, Alt: false, Key: "tab"},
		"newConnection":           {Primary: true, Shift: true, Alt: false, Key: "n"},
		"toggleSidebar":           {Primary: true, Shift: true, Alt: false, Key: "h"},
		"openQuickCommands":       {Primary: true, Shift: false, Alt: false, Key: "k"},
		"focusTerminal":           {Primary: true, Shift: true, Alt: false, Key: "j"},
		"focusAI":                 {Primary: true, Shift: true, Alt: false, Key: "k"},
		"lockAI":                  {Primary: true, Shift: true, Alt: false, Key: "l"},
		"duplicateSession":        {Primary: true, Shift: true, Alt: false, Key: "d"},
		"closePanel":              {Primary: true, Shift: true, Alt: false, Key: "q"},
		"navigatePrev":            {Ctrl: false, Shift: false, Alt: true, Key: "arrowleft"},
		"navigateNext":            {Ctrl: false, Shift: false, Alt: true, Key: "arrowright"},
		"toggleWorkspaceMaximize": {Primary: true, Shift: true, Alt: false, Key: "enter"},
		"terminalSearch":          {Primary: true, Shift: true, Alt: false, Key: "f"},
		"openSettings":            {Primary: true, Shift: false, Alt: false, Key: ","},
		"copy":                    {Primary: true, Shift: true, Alt: false, Key: "c"},
		"paste":                   {Primary: true, Shift: true, Alt: false, Key: "v"},
		"toggleLineNumbers":       {Primary: true, Shift: true, Alt: false, Key: "g"},
		"toggleTimestamps":        {Primary: true, Shift: true, Alt: false, Key: "t"},
		"zoomFontIn":              {Primary: true, Shift: false, Alt: false, Key: "="},
		"zoomFontOut":             {Primary: true, Shift: false, Alt: false, Key: "-"},
	}
}
