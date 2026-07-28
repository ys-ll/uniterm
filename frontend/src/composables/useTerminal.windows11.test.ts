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

describe('getXtermTheme — uniterm-windows11', () => {
  it('returns the Windows Terminal 11 default palette', () => {
    const theme = getXtermTheme('uniterm-windows11')

    expect(theme.background).toBe('#0c0c0c')
    expect(theme.foreground).toBe('#cccccc')
    expect(theme.cursor).toBe('#ffffff')

    // Signature Windows Terminal accent blue
    expect(theme.blue).toBe('#3b78ff')
    expect(theme.brightBlue).toBe('#3b78ff')

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