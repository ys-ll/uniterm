package store

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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
	// MultilinePaste controls multi-line paste protection (issue #1370):
	// "confirm" opens an editable preview dialog before multi-line clipboard
	// content is written to the terminal, "direct" pastes without asking.
	// Empty means the frontend falls back to "confirm".
	MultilinePaste string `json:"multilinePaste,omitempty"`
	MaxHistoryLines   int    `json:"maxHistoryLines"`
	SmartCompletion   *bool  `json:"smartCompletion"`
	SuggestionPopup   *bool  `json:"suggestionPopup"`
	InlineHint        *bool  `json:"inlineHint"`
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
	// ZmodemDownloadDir is the device-local default directory for files
	// received with sz. Empty preserves the directory picker behavior.
	ZmodemDownloadDir string `json:"zmodemDownloadDir,omitempty"`
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
	// CommandSendMode controls how the command-send panel delivers
	// multi-line content (issue #1341): "lines" converts each newline to
	// Enter so every line executes, "bracketed" sends the block wrapped in
	// bracketed-paste markers without executing. Empty means the frontend
	// falls back to "lines".
	CommandSendMode string `json:"commandSendMode,omitempty"`
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
	// SessionHeader names an HTTP header that each request to this model's
	// BaseURL carries with the current chat session's ID as the value —
	// for gateways that do prompt caching / sticky routing per session
	// (issue #1170). Empty = no such header.
	SessionHeader string `json:"sessionHeader,omitempty"`
}

type AISettings struct {
	MaxTurns           *int            `json:"maxTurns"`
	AutoCompact        *bool           `json:"autoCompact,omitempty"`
	ContextTokenBudget int             `json:"contextTokenBudget,omitempty"`
	SummaryModelID     string          `json:"summaryModelId,omitempty"`
	Models             []AIModelConfig `json:"models"`
	// Read only for migration to local_state.json. New clients omit this field.
	ActiveModelID string `json:"activeModelId,omitempty"`
}

// MCPSettings configures the external-agent MCP server endpoint. Pointer on
// AppSettings + omitempty so settings.json written by older builds still
// loads; nil means "feature off with defaults". The former per-group tool
// toggles ("tools" in old settings.json) are gone — all tools are available
// and side effects are governed by the approval policy alone; the stale
// field is ignored on load.
type MCPSettings struct {
	// Enabled starts the Streamable HTTP endpoint on launch.
	Enabled bool `json:"enabled"`
	// AllowRemote opts into listening on all IPv4 interfaces and accepting
	// non-local Host headers, including requests forwarded over SSH.
	// Defaults to false so existing installations stay loopback-only.
	AllowRemote bool `json:"allowRemote,omitempty"`
	// Port for the HTTP listener; 0 = DefaultPort.
	Port int `json:"port,omitempty"`
	// Policy is the approval policy: confirm_write (default) / confirm_all /
	// confirm_dangerous / bypass. Unknown or empty values fall back to
	// confirm_write when the policy is evaluated (mcpPolicy in app_mcp.go).
	Policy string `json:"policy,omitempty"`
	// ExecMode selects which terminal-execution toolset the MCP server
	// exposes: "background" (default, non-PTY SSH exec with real exit codes)
	// or "foreground" (types into any live terminal, screen-resolved output,
	// plus capture/send-key tools). The user picks the mode — the agent never
	// chooses at call time. Unknown or empty values fall back to background
	// (mcpExecMode in app_mcp.go). Takes effect on the next MCP server start
	// (StartMCP runs after every SaveLocalState).
	ExecMode string `json:"execMode,omitempty"`
}

// DefaultMCPSettings is the off-by-default MCP block for settings.json.
func DefaultMCPSettings() MCPSettings {
	return MCPSettings{
		Enabled:  false,
		Policy:   "confirm_write",
		ExecMode: "background",
	}
}

type KeyBinding struct {
	Ctrl  bool   `json:"ctrl"`
	Meta  bool   `json:"meta"`
	Shift bool   `json:"shift"`
	Alt   bool   `json:"alt"`
	Key   string `json:"key"`
}

type AppSettings struct {
	Theme    string `json:"theme"`
	Language string `json:"language"`
	// UiFontSize is the UI design baseline in px (how large "normal" text
	// renders): the rem root derives from it as uiFontSize/12*16. Pointer +
	// omitempty so settings.json written by older builds still loads; nil
	// means "use the platform default" (14 on macOS, 12 elsewhere). Stored
	// per device on purpose — settings.json is not synced.
	UiFontSize      *int                  `json:"uiFontSize,omitempty"`
	Terminal        TerminalSettings      `json:"terminal"`
	AI              AISettings            `json:"ai"`
	Keyboard        map[string]KeyBinding `json:"keyboard"`
	AutoCheckUpdate *bool                 `json:"autoCheckUpdate"`
	// UpdateSource selects where update checks and downloads come from:
	// "auto" (default, picks by UI language with fallback), "github" or
	// "gitee" (domestic mirror). Pointer + omitempty so settings.json written
	// by older builds still load; nil means "auto".
	UpdateSource   *string `json:"updateSource,omitempty"`
	CloseTabPrompt *bool   `json:"closeTabPrompt"`
	CloseAppPrompt *bool   `json:"closeAppPrompt"`
	// RestoreTabsPrompt gates the "reopen last session's tabs" prompt at
	// startup (issue #937). Pointer + omitempty so settings.json written by
	// older builds still loads; nil means "on" (frontend default).
	RestoreTabsPrompt *bool         `json:"restoreTabsPrompt,omitempty"`
	SFTPBookmarks     SFTPBookmarks `json:"sftpBookmarks"`
	// SftpTransferPanelVisible remembers whether the SFTP transfer panel was
	// last left visible. Pointer + omitempty so settings.json written by older
	// builds (which lack this field) still load; nil means "use the frontend
	// default" (hidden).
	SftpTransferPanelVisible *bool                 `json:"sftpTransferPanelVisible,omitempty"`
	CustomTerminalThemes     []CustomTerminalTheme `json:"customTerminalThemes"`
	DefaultLocalShell        string                `json:"defaultLocalShell"`
	// GitBashPath is an optional user-supplied Git for Windows location (the
	// install directory or bash.exe itself, e.g. a scoop install). Resolved on
	// Windows when listing the available local shells.
	GitBashPath    string `json:"gitBashPath,omitempty"`
	TabCloseButton string `json:"tabCloseButton"`
	// ShowTabShortcutHints toggles the per-tab numeric shortcut hint
	// (e.g. Ctrl+1) on the tab itself. Pointer + omitempty so settings.json
	// written by older builds (which lack this field) still loads; the
	// frontend defaults to `true` when nil.
	ShowTabShortcutHints *bool `json:"showTabShortcutHints,omitempty"`
	// ShowTrayIcon toggles the system tray icon (macOS menu bar status item).
	// Pointer + omitempty so settings.json written by older builds still
	// loads; nil means "show" (the frontend default).
	ShowTrayIcon *bool `json:"showTrayIcon,omitempty"`
	// HostListMenuStyle selects how the host list opens the connection
	// menu: "button" (hover buttons) or "rightclick". Empty means the
	// frontend falls back to `button`.
	HostListMenuStyle string `json:"hostListMenuStyle,omitempty"`
	// NewTabAction selects what the tab bar's "+" button (and the
	// newConnection shortcut) opens: "startPage" or "localTerminal".
	// Empty means the frontend falls back to "startPage".
	NewTabAction string `json:"newTabAction,omitempty"`
	// ConnectionsDisplayMode selects the connections panel list density:
	// "full" (each connection shows a subtitle line) or "compact"
	// (single-line rows). Empty means the frontend falls back to "full".
	ConnectionsDisplayMode string `json:"connectionsDisplayMode,omitempty"`
	// QuickCommandsDisplayMode selects the quick-commands panel display
	// style: "list" (rows with name + command) or "buttons" (name-only
	// buttons tiled horizontally). Empty means the frontend falls back to
	// "list".
	QuickCommandsDisplayMode string `json:"quickCommandsDisplayMode,omitempty"`
	// MonitorSections toggles which monitor sections are enabled (basic/
	// cpu/allCpu/memory/disk/allDisk/network). Disabled sections are hidden
	// in the UI and skipped by the monitor poll loop. Nil means "use the
	// frontend defaults" (everything enabled).
	MonitorSections map[string]bool `json:"monitorSections,omitempty"`
	// SidebarTabs toggles which connection-sidebar tab icons are visible,
	// keyed by view id (connections/files/monitor/tunnels/quickCommands/
	// history/personalization). "connections" is always shown in the UI and
	// never hidden. Pointer + omitempty so settings.json written by older
	// builds (which lack this field) still load; a nil map means "use the
	// frontend defaults" (everything visible).
	SidebarTabs map[string]bool `json:"sidebarTabs,omitempty"`
	// PanelLocations maps sidebar view id (connections/files/monitor/
	// tunnels/quickCommands/history/personalization) to its dock position:
	// "sidebar" or "bottom". Nil means "use the frontend defaults"
	// (PANEL_LOCATION_DEFAULTS).
	PanelLocations map[string]string `json:"panelLocations,omitempty"`
	// PanelOrder is the global tab order across both docks (view ids);
	// views absent from the list append in default order.
	PanelOrder []string `json:"panelOrder,omitempty"`
	// StartPage controls which blocks (search/favorites/recent/groups) the
	// start page shows and in what order. Pointer + omitempty so
	// settings.json written by older builds (which lack this field) still
	// loads; a nil value means "use the frontend defaults" (everything
	// visible, search → favorites → recent → groups).
	StartPage *StartPageSettings `json:"startPage,omitempty"`
}

type SFTPBookmarks struct {
	LocalPaths  []string `json:"localPaths"`
	RemotePaths []string `json:"remotePaths"`
}

// StartPageSettings controls the layout of the new-tab start page: which
// blocks (search/favorites/recent/groups) are visible and in what order.
// Block ids are strings rather than a typed enum so hand-edited or
// cross-version settings.json files degrade gracefully (unknown ids are
// ignored by the frontend).
type StartPageSettings struct {
	Visible map[string]bool `json:"visible,omitempty"`
	Order   []string        `json:"order,omitempty"`
}

type SettingsStore struct {
	configDir     string
	passwordStore PasswordStore
	mu            sync.Mutex // serializes Save + Load migration writes (STORE-05/06).
	lastSavedHash string     // F-105 skip state (pre-encryption content hash)
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
	return s.save(settings, false)
}

// ForceSave rewrites the file even when the content hash matches the
// seeded state. Used by re-keying: the plaintext is unchanged, so the
// F-105 hash-skip would otherwise swallow the re-encryption and leave
// the file under the old key.
func (s *SettingsStore) ForceSave(settings AppSettings) error {
	return s.save(settings, true)
}

func (s *SettingsStore) save(settings AppSettings, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deep-copy models so we don't mutate the caller's backing array
	models := make([]AIModelConfig, len(settings.AI.Models))
	copy(models, settings.AI.Models)

	settings.AI.Models = models
	// Shared Save envelope (filestore.go): hash-skip, encrypt, atomic write.
	skip := s.lastSavedHash
	if force {
		// Re-keying saves unchanged plaintext under a new key — the
		// F-105 hash-skip would otherwise swallow the rewrite.
		skip = forceWrite
	}
	h, err := saveEncryptedFile(settingsFileName, s.filePath(), &settings, s.passwordStore, skip)
	s.lastSavedHash = h
	return err
}

func (s *SettingsStore) Load() (AppSettings, error) {
	// filePath is immutable after construction, so read it without the lock.
	// Disk I/O on the settings file can be slow (cold cache, encrypted FS);
	// holding the mutex across os.ReadFile would block every concurrent Save.
	path := s.filePath()
	settings, res, hash, existed, err := loadEncryptedFile[AppSettings](settingsFileName, path, s.passwordStore)
	if !existed {
		return defaultSettings(), nil
	}
	if err != nil {
		if errors.Is(err, errUnparseableConfig) {
			// STORE-09: the corrupt file was quarantined before falling back
			// to defaults so the next Save doesn't silently overwrite the
			// user's prior data.
			return defaultSettings(), nil
		}
		return AppSettings{}, err
	}
	needsSave := res.Plaintext > 0 // legacy plaintext — re-save will encrypt
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
	// Default maxTurns when missing (older settings.json files predating the
	// multi-model AI block, or hand-edited files). Pointer + backfill so the
	// stored copy always carries an explicit value.
	if settings.AI.MaxTurns == nil {
		settings.AI.MaxTurns = intPtr(defaultMaxTurns)
		needsSave = true
	}
	if settings.UiFontSize == nil {
		n := defaultUiFontSize()
		settings.UiFontSize = &n
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
	if needsSave {
		// Force: the legacy-plaintext case changes nothing in the plaintext
		// content, so the hash-skip would swallow the lazy encryption.
		_ = s.ForceSave(settings)
	} else {
		s.lastSavedHash = hash
	}

	return settings, nil
}

func defaultSettings() AppSettings {
	n := defaultUiFontSize()
	return AppSettings{
		Theme:      "dark",
		Language:   "system",
		UiFontSize: &n,
		Terminal: TerminalSettings{
			Theme:            "uniterm-dark",
			FontFamily:       "Consolas, \"Courier New\", monospace",
			FontSize:         14,
			SelectionAction:  "none",
			RightClickAction: "menu",
			MaxHistoryLines:  5000,
		},
		AI:              defaultAISettings(),
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

// defaultUiFontSize returns the platform UI text baseline in px: macOS
// native text runs larger (HIG 13pt+) than the Windows 12px design size,
// and users sit further from laptop Retina screens.
func defaultUiFontSize() int {
	if runtime.GOOS == "darwin" {
		return 14
	}
	return 12
}

const defaultMaxTurns = 20

// defaultAISettings is the seed AI block for a fresh settings.json. Shared
// with AIConfigStore so the settings and ai.json defaults can never drift.
func defaultAISettings() AISettings {
	return AISettings{
		MaxTurns: intPtr(defaultMaxTurns),
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
	}
}

// defaultAIConfig mirrors defaultAISettings for the standalone ai.json file.
func defaultAIConfig() AIStoreData {
	ai := defaultAISettings()
	return AIStoreData{MaxTurns: ai.MaxTurns, Models: ai.Models}
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
		"openSettings":     {Ctrl: true, Shift: false, Alt: false, Key: ","},
	}
}

// BlobName, ExportPlain and ImportPlain implement the sync exchange contract
// (SyncableStore, declared in backend/sync/backend.go).

func (s *SettingsStore) BlobName() string { return settingsFileName }

func (s *SettingsStore) ExportPlain() ([]byte, error) {
	return exportPlainStore(settingsFileName, s.filePath(), s.Load)
}

func (s *SettingsStore) ImportPlain(raw []byte) error {
	return importPlainStore(raw, s.Save)
}
