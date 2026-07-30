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

describe('getXtermTheme — uniterm-windows7-light', () => {
  it('returns the Win7 Aurora wallpaper light palette', () => {
    const theme = getXtermTheme('uniterm-windows7-light')

    // Pale sky-blue aurora ground, foreground is the wallpaper's deep teal
    expect(theme.background).toBe('#bcd5e3')
    expect(theme.foreground).toBe('#0d2436')
    expect(theme.cursor).toBe('#1a4a7c')

    // Same aurora blue/teal signatures as the dark variant
    expect(theme.blue).toBe('#2a6aaa')
    expect(theme.cyan).toBe('#3a8a8a')
    expect(theme.brightBlue).toBe('#4a8acc')
    expect(theme.brightCyan).toBe('#5ab0b0')

    // Light bg → brightWhite is intentionally dark
    expect(theme.brightWhite).toBe('#0d2436')

    // 20 expected fields (4 base + 16 ANSI)
    expect(Object.keys(theme).sort()).toEqual(
      [
        'background', 'foreground', 'cursor', 'selectionBackground',
        'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
        'brightBlack', 'brightRed', 'brightGreen', 'brightYellow', 'brightBlue', 'brightMagenta', 'brightCyan', 'brightWhite'
      ].sort()
    )
  })
})