import { describe, it, expect, vi } from 'vitest'

// useTerminal.ts pulls in xterm.js + Wails bindings at module load time.
// Stub these browser/host modules so getXtermTheme() can be unit-tested
// in a pure-Node environment without loading xterm's UMD wrapper.
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

describe('getXtermTheme — uniterm-soft-gray', () => {
  it('returns the soft gray palette', () => {
    const theme = getXtermTheme('uniterm-soft-gray')

    expect(theme.background).toBe('#e8e8e8')
    expect(theme.foreground).toBe('#1a1a1a')
    expect(theme.cursor).toBe('#1a1a1a')

    // Distinguishable ANSI: red/magenta and green/cyan must have different hues
    expect(theme.red).not.toBe(theme.magenta)
    expect(theme.green).not.toBe(theme.cyan)
    expect(theme.blue).not.toBe(theme.magenta)

    // Light bg → brightWhite is actually a dark color (for legibility)
    expect(theme.brightWhite).toBe('#2d2d2d')

    // 20 expected fields (xterm.js ITheme shape: 4 base + 16 ANSI)
    expect(Object.keys(theme).sort()).toEqual(
      [
        'background', 'foreground', 'cursor', 'selectionBackground',
        'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
        'brightBlack', 'brightRed', 'brightGreen', 'brightYellow', 'brightBlue', 'brightMagenta', 'brightCyan', 'brightWhite'
      ].sort()
    )
  })
})
