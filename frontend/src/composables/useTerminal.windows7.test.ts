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

describe('getXtermTheme — uniterm-windows7', () => {
  it('returns the Win7 Aurora wallpaper palette', () => {
    const theme = getXtermTheme('uniterm-windows7')

    // Aurora teal-blue ground, lifted slightly off the wallpaper
    expect(theme.background).toBe('#1a3a5c')
    expect(theme.foreground).toBe('#dceaf2')
    expect(theme.cursor).toBe('#a8d4e8')

    // Signature aurora blue + teal ANSI colors
    expect(theme.blue).toBe('#7ab8e8')
    expect(theme.cyan).toBe('#7ad8d8')
    expect(theme.brightBlue).toBe('#90d0ff')
    expect(theme.brightCyan).toBe('#90e8e8')

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