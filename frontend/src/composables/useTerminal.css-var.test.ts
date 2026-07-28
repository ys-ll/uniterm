import { describe, it, expect, vi } from 'vitest'

// Same module-load stub pattern as useTerminal.soft-gray.test.ts — keeps
// getXtermTheme() pure and avoids pulling in xterm's UMD wrapper in Node.
vi.mock('@xterm/xterm', () => ({ Terminal: class {} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class {} }))
vi.mock('@xterm/addon-search', () => ({ SearchAddon: class {} }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))
vi.mock('../../wailsjs/go/main/App', () => ({ SessionWrite: () => {}, SessionResize: () => {} }))
vi.mock('../../wailsjs/runtime', () => ({ EventsOn: () => {}, BrowserOpenURL: () => {} }))
vi.mock('../stores/settingsStore', () => ({ useSettingsStore: () => ({ settings: {} }) }))
vi.mock('../stores/localStateStore', () => ({ useLocalStateStore: () => ({ state: {} }) }))
vi.mock('../stores/sessionStore', () => ({ useSessionStore: () => ({ getData: () => '' }) }))
vi.mock('./useHighlight', () => ({ highlight: (x: string) => x }))
vi.mock('../utils/cursor', () => ({ stripCursorBlink: (x: string) => x }))

import { getXtermTheme } from './useTerminal'
import { TERMINAL_THEMES } from '../types/settings'

// xterm.js parses background/foreground/cursor to drive OSC 11/10/12 responses.
// CSS variables like `var(--bg-base)` are opaque to it and silently fall back
// to defaults (typically black), which lies to TUI clients about the real
// background. We forbid them in built-in themes.
const FORBIDDEN_VAR_FIELDS = ['background', 'foreground', 'cursor'] as const

describe('getXtermTheme — no CSS variables in built-in themes', () => {
  for (const { value } of TERMINAL_THEMES) {
    it(`${value} does not use var(--…) for background/foreground/cursor`, () => {
      const theme = getXtermTheme(value)
      for (const field of FORBIDDEN_VAR_FIELDS) {
        const v = theme[field]
        expect(v, `${value}.${field}`).toBeDefined()
        expect(
          typeof v === 'string' && v.startsWith('var('),
          `${value}.${field} should be a hex/rgb string, got ${JSON.stringify(v)}`
        ).toBe(false)
      }
    })
  }
})