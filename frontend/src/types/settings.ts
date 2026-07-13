export const SUPPORTED_LOCALES = [
  'zh-CN', 'zh-TW', 'en', 'ja', 'ko', 'de', 'es', 'fr', 'ru'
] as const

export type Locale = typeof SUPPORTED_LOCALES[number]
export type Language = Locale | 'system'
export type Theme = 'dark' | 'deep-blue' | 'light' | 'system'
export type TerminalTheme = 'uniterm-dark' | 'uniterm-light' | 'solarized-dark' | 'solarized-light' | 'monokai' | 'dracula' | 'molokai' | 'tomorrow-night' | 'tomorrow-night-bright' | 'tomorrow' | 'one-dark' | 'one-light' | 'github-dark' | 'github-light' | 'gotham' | 'hybrid' | 'nord' | 'gruvbox-dark' | 'gruvbox-light' | 'catppuccin-mocha' | 'catppuccin-latte' | 'tokyo-night' | 'tokyo-day' | 'rose-pine' | 'rose-pine-dawn' | 'everforest-dark' | 'everforest-light'

// xterm.js's ITheme shape: the 4 base colors plus the 16 ANSI colors, all as hex strings.
export interface TerminalThemeColors {
  background: string
  foreground: string
  cursor: string
  selection: string
  black: string
  red: string
  green: string
  yellow: string
  blue: string
  magenta: string
  cyan: string
  white: string
  brightBlack: string
  brightRed: string
  brightGreen: string
  brightYellow: string
  brightBlue: string
  brightMagenta: string
  brightCyan: string
  brightWhite: string
}

// A user-defined terminal color scheme. Stored alongside (not inside)
// TerminalSettings since a theme is a reusable resource, not a single
// terminal session's property.
export interface CustomTerminalTheme {
  id: string
  name: string
  type: 'dark' | 'light'
  colors: TerminalThemeColors
}

export interface TerminalSettings {
  theme: TerminalTheme | string
  fontFamily: string
  fontSize: number
  selectionAction: 'none' | 'copy'
  rightClickAction: 'menu' | 'paste'
  middleClickAction: 'none' | 'paste'
  maxHistoryLines: number
  smartCompletion: boolean
  highlightEnabled: boolean
}

export interface AIModelConfig {
  id: string
  name: string
  apiKey: string
  baseURL: string
  model: string
  protocol: 'anthropic' | 'openai'
  userAgent?: string
}

export const USER_AGENT_PRESETS: { label: string; value: string }[] = [
  { label: 'uniTerm', value: 'uniTerm' },
  { label: 'Claude Code', value: 'claude-code/1.0' },
  { label: 'Cursor', value: 'Cursor/1.0' },
  { label: 'Cline', value: 'Cline/1.0' },
  { label: 'OpenCode', value: 'opencode' },
  { label: 'ChatGPT Desktop', value: 'ChatGPT-Desktop/1.0' },
]

export interface AISettings {
  maxTurns: number
  models: AIModelConfig[]
  activeModelId: string
}

export type ShortcutAction =
  | 'nextTab' | 'prevTab'
  | 'newConnection' | 'toggleSidebar'
  | 'focusAI' | 'focusTerminal' | 'lockAI'
  | 'closePanel'
  | 'navigatePrev' | 'navigateNext'
  | 'duplicateSession'

export interface KeyBinding {
  ctrl: boolean
  shift: boolean
  alt: boolean
  key: string
}

export type KeyboardSettings = Partial<Record<ShortcutAction, KeyBinding>>

export const SHORTCUT_LABELS: Record<ShortcutAction, string> = {
  newConnection: 'shortcut.newConnection',
  nextTab: 'shortcut.nextTab',
  prevTab: 'shortcut.prevTab',
  navigatePrev: 'shortcut.navigatePrev',
  navigateNext: 'shortcut.navigateNext',
  closePanel: 'shortcut.closePanel',
  toggleSidebar: 'shortcut.toggleSidebar',
  focusTerminal: 'shortcut.focusTerminal',
  focusAI: 'shortcut.focusAI',
  lockAI: 'shortcut.lockAI',
  duplicateSession: 'shortcut.duplicateSession',
}

export const DEFAULT_KEYBOARD: KeyboardSettings = {
  nextTab: { ctrl: true, shift: false, alt: false, key: 'tab' },
  prevTab: { ctrl: true, shift: true, alt: false, key: 'tab' },
  newConnection: { ctrl: true, shift: true, alt: false, key: 'n' },
  toggleSidebar: { ctrl: true, shift: true, alt: false, key: 'h' },
  focusTerminal: { ctrl: true, shift: true, alt: false, key: 'j' },
  focusAI: { ctrl: true, shift: true, alt: false, key: 'k' },
  closePanel: { ctrl: true, shift: true, alt: false, key: 'q' },
  navigatePrev: { ctrl: false, shift: false, alt: true, key: 'arrowleft' },
  navigateNext: { ctrl: false, shift: false, alt: true, key: 'arrowright' },
  lockAI: { ctrl: true, shift: true, alt: false, key: 'l' },
  duplicateSession: { ctrl: true, shift: true, alt: false, key: 'd' },
}

export interface SFTPBookmarks {
  localPaths: string[]
  remotePaths: string[]
}

export interface AppSettings {
  theme: Theme
  language: Language
  terminal: TerminalSettings
  ai: AISettings
  keyboard: KeyboardSettings
  autoCheckUpdate: boolean
  sftpBookmarks: SFTPBookmarks
  customTerminalThemes: CustomTerminalTheme[]
  defaultLocalShell: string
}

export const DEFAULT_SETTINGS: AppSettings = {
  theme: 'dark',
  language: 'system',
  terminal: {
    theme: 'uniterm-dark',
    fontFamily: 'Consolas, "Courier New", monospace',
    fontSize: 14,
    selectionAction: 'none',
    rightClickAction: 'menu',
    middleClickAction: 'paste',
    maxHistoryLines: 2500,
    smartCompletion: true,
    highlightEnabled: true
  },
  ai: {
    maxTurns: 20,
    models: [
      {
        id: 'model-default',
        name: 'Default',
        apiKey: '',
        baseURL: 'https://api.openai.com/v1',
        model: 'gpt-4o',
        protocol: 'anthropic' as const
      }
    ],
    activeModelId: 'model-default'
  },
  keyboard: { ...DEFAULT_KEYBOARD },
  autoCheckUpdate: true,
  sftpBookmarks: {
    localPaths: [],
    remotePaths: []
  },
  customTerminalThemes: [],
  defaultLocalShell: ''
}

export interface TerminalThemeEntry { label: string; value: string; type: 'dark' | 'light' }
export const TERMINAL_THEMES: TerminalThemeEntry[] = [
  { label: 'uniTerm Dark', value: 'uniterm-dark', type: 'dark' },
  { label: 'uniTerm Light', value: 'uniterm-light', type: 'light' },
  { label: 'Solarized Dark', value: 'solarized-dark', type: 'dark' },
  { label: 'Solarized Light', value: 'solarized-light', type: 'light' },
  { label: 'Monokai', value: 'monokai', type: 'dark' },
  { label: 'Dracula', value: 'dracula', type: 'dark' },
  { label: 'Molokai', value: 'molokai', type: 'dark' },
  { label: 'Tomorrow Night', value: 'tomorrow-night', type: 'dark' },
  { label: 'Tomorrow Night Bright', value: 'tomorrow-night-bright', type: 'dark' },
  { label: 'Tomorrow', value: 'tomorrow', type: 'light' },
  { label: 'One Dark', value: 'one-dark', type: 'dark' },
  { label: 'One Light', value: 'one-light', type: 'light' },
  { label: 'GitHub Dark', value: 'github-dark', type: 'dark' },
  { label: 'GitHub Light', value: 'github-light', type: 'light' },
  { label: 'Gotham', value: 'gotham', type: 'dark' },
  { label: 'Hybrid', value: 'hybrid', type: 'dark' },
  { label: 'Nord', value: 'nord', type: 'dark' },
  { label: 'Gruvbox Dark', value: 'gruvbox-dark', type: 'dark' },
  { label: 'Gruvbox Light', value: 'gruvbox-light', type: 'light' },
  { label: 'Catppuccin Mocha', value: 'catppuccin-mocha', type: 'dark' },
  { label: 'Catppuccin Latte', value: 'catppuccin-latte', type: 'light' },
  { label: 'Tokyo Night', value: 'tokyo-night', type: 'dark' },
  { label: 'Tokyo Day', value: 'tokyo-day', type: 'light' },
  { label: 'Rosé Pine', value: 'rose-pine', type: 'dark' },
  { label: 'Rosé Pine Dawn', value: 'rose-pine-dawn', type: 'light' },
  { label: 'Everforest Dark', value: 'everforest-dark', type: 'dark' },
  { label: 'Everforest Light', value: 'everforest-light', type: 'light' }
]

export const FONT_OPTIONS: { label: string; value: string }[] = [
  { label: 'Consolas', value: 'Consolas, "Courier New", monospace' },
  { label: 'Courier New', value: '"Courier New", Courier, monospace' },
  { label: 'Monaco', value: 'Monaco, "Courier New", monospace' },
  { label: 'Fira Code', value: '"Fira Code", monospace' },
  { label: 'JetBrains Mono', value: '"JetBrains Mono", monospace' },
  { label: 'Source Code Pro', value: '"Source Code Pro", monospace' }
]

export const SELECTION_ACTIONS: { label: string; value: TerminalSettings['selectionAction'] }[] = [
  { label: 'None', value: 'none' },
  { label: 'Copy to clipboard', value: 'copy' }
]

export const RIGHT_CLICK_ACTIONS: { label: string; value: TerminalSettings['rightClickAction'] }[] = [
  { label: 'Show context menu', value: 'menu' },
  { label: 'Paste from clipboard', value: 'paste' }
]

export const LANGUAGE_OPTIONS: { value: Locale; label: string; native: string }[] = [
  { value: 'zh-CN', label: '简体中文', native: '简体中文' },
  { value: 'zh-TW', label: '繁體中文', native: '繁體中文' },
  { value: 'en', label: 'English', native: 'English' },
  { value: 'ja', label: '日本語', native: '日本語' },
  { value: 'ko', label: '한국어', native: '한국어' },
  { value: 'de', label: 'Deutsch', native: 'Deutsch' },
  { value: 'es', label: 'Español', native: 'Español' },
  { value: 'fr', label: 'Français', native: 'Français' },
  { value: 'ru', label: 'Русский', native: 'Русский' },
]

export interface UpdateInfo {
  hasUpdate: boolean
  current: string
  latest: string
  releaseUrl: string
}
