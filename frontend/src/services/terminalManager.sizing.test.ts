import { describe, it, expect, vi, afterEach } from 'vitest'

// Minimal xterm.js stand-in: the constructor reports the same 80x24 grid
// real xterm does before any fit, and resize() fires onResize like the
// real core does. vi.hoisted so the vi.mock factories below can use them.
const { FakeTerminal, FakeFitAddon } = vi.hoisted(() => {
  class FakeTerminal {
    cols = 80
    rows = 24
    options: Record<string, unknown> = {}
    element: { parentElement: unknown } | null = null
    unicode = { activeVersion: '6' }
    buffer = { normal: { baseY: 0, cursorY: 0, getLine: () => undefined } }
    resizeHandlers: Array<(dims: { cols: number; rows: number }) => void> = []

    loadAddon(addon: { _terminal?: unknown }) {
      addon._terminal = this
    }
    attachCustomWheelEventHandler() {}
    onResize(cb: (dims: { cols: number; rows: number }) => void) {
      this.resizeHandlers.push(cb)
      return { dispose: () => {} }
    }
    onData(_cb: (data: string) => void) {
      return { dispose: () => {} }
    }
    resize(cols: number, rows: number) {
      this.cols = cols
      this.rows = rows
      this.resizeHandlers.forEach(cb => cb({ cols, rows }))
    }
    open() {
      this.element = { parentElement: null }
    }
    dispose() {}
  }

  // fit() simulates a real measurement on a visible container: the grid
  // changes from the 80x24 constructor defaults to 120x30.
  class FakeFitAddon {
    _terminal: FakeTerminal | null = null
    fit() {
      this._terminal?.resize(120, 30)
    }
  }

  return { FakeTerminal, FakeFitAddon }
})

vi.mock('@xterm/xterm', () => ({
  Terminal: FakeTerminal,
}))
vi.mock('@xterm/addon-fit', () => ({
  FitAddon: FakeFitAddon,
}))
vi.mock('@xterm/addon-unicode11', () => ({
  Unicode11Addon: class {},
}))
vi.mock('@xterm/addon-search', () => ({
  SearchAddon: class {},
}))
vi.mock('@xterm/addon-clipboard', () => ({
  ClipboardAddon: class {},
  Base64: class {},
}))
vi.mock('@xterm/addon-progress', () => ({
  ProgressAddon: class {
    // IEvent: called as a function, returns a disposable.
    onChange = () => ({ dispose() {} })
  },
}))
vi.mock('@wailsio/runtime', () => ({
  Events: { On: () => () => {} },
}))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  SessionResize: vi.fn(() => Promise.resolve()),
  // sessionWriter binds SessionWrite at import time.
  SessionWrite: vi.fn(() => Promise.resolve()),
}))
vi.mock('../composables/useTerminal', () => ({
  getXtermTheme: () => ({}),
}))
vi.mock('../composables/useTerminalTheme', () => ({
  resolveXtermBackground: () => ({}),
  applyTerminalBgVar: () => {},
  resolveTerminalThemeName: (name?: string) => name,
}))
vi.mock('../utils/xtermImeCompatibility', () => ({
  installImeCompatibilityPatch: () => ({ dispose() {} }),
}))
vi.mock('../stores/settingsStore', () => ({
  useSettingsStore: () => ({
    settings: { terminal: { fontFamily: 'monospace', fontSize: 14, scrollback: 1000 } },
  }),
}))
vi.mock('../stores/localStateStore', () => ({
  useLocalStateStore: () => ({
    state: { sessionLogStates: {}, backgroundEnabled: false, backgroundImage: '' },
    setSessionLogState: () => {},
  }),
}))
vi.mock('../stores/panelStore', () => ({
  usePanelStore: () => ({ panels: new Map() }),
}))
vi.mock('../stores/tabStore', () => ({
  useTabStore: () => ({ tabs: [], setTabProgress: () => {} }),
}))

import { getTerminalSize, waitForTerminalSize, acquireTerminal, attachTerminal, disposeTerminal } from './terminalManager'
import { SessionResize } from '../../bindings/github.com/ys-ll/uniterm/app'

// attachTerminal waits for document.fonts.ready, then two rAFs, before
// fitting. Stub both so the chain drains on plain setTimeout ticks.
function stubDomFonts(ready: Promise<void>) {
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
    setTimeout(() => cb(0), 0)
    return 0
  })
  vi.stubGlobal('document', {
    fonts: { ready },
    createElement: () => ({ style: {} }),
  })
}

async function flushAsyncChain() {
  for (let i = 0; i < 6; i++) await new Promise(r => setTimeout(r, 0))
}

function makeContainer(width: number, height: number): HTMLElement {
  return {
    getBoundingClientRect: () => ({ width, height, top: 0, left: 0, right: width, bottom: height, x: 0, y: 0, toJSON: () => ({}) }),
    appendChild: () => {},
  } as unknown as HTMLElement
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('terminalManager sizing helpers', () => {
  afterEach(() => {
    disposeTerminal('s-a')
    disposeTerminal('s-b')
  })

  it('getTerminalSize returns 0,0 for an unknown session id', () => {
    const size = getTerminalSize('does-not-exist')
    expect(size).toEqual({ cols: 0, rows: 0 })
  })

  it('waitForTerminalSize resolves to 0,0 after the timeout when no terminal mounts', async () => {
    const start = Date.now()
    const size = await waitForTerminalSize('still-missing', 120)
    const elapsed = Date.now() - start
    expect(size).toEqual({ cols: 0, rows: 0 })
    expect(elapsed).toBeGreaterThanOrEqual(100)
  })

  it('does not report the 80x24 constructor defaults before the first font-ready fit', () => {
    // acquireTerminal creates the xterm Terminal, which starts at the
    // 80x24 constructor defaults. Those must never surface as a
    // "measured" size, or the PTY starts at 80x24 and the already-drawn
    // banner stays wrapped at 80 columns.
    acquireTerminal('s-a', 'ref-a', {})
    expect(getTerminalSize('s-a')).toEqual({ cols: 0, rows: 0 })
  })

  it('reports the font-measured size and syncs it to the PTY after attach', async () => {
    let resolveFonts: () => void = () => {}
    stubDomFonts(new Promise<void>(r => { resolveFonts = r }))
    const container = makeContainer(800, 600)

    acquireTerminal('s-a', 'ref-a', {})
    attachTerminal('s-a', container)
    expect(getTerminalSize('s-a')).toEqual({ cols: 0, rows: 0 })

    resolveFonts()
    await flushAsyncChain()

    expect(getTerminalSize('s-a')).toEqual({ cols: 120, rows: 30 })
    expect(SessionResize).toHaveBeenCalledWith('s-a', 120, 30)
    // waitForTerminalSize picks up the fitted size immediately now.
    const size = await waitForTerminalSize('s-a', 100)
    expect(size).toEqual({ cols: 120, rows: 30 })
  })

  it('keeps reporting 0,0 while the container is hidden, and fits on a later visible attach', async () => {
    let resolveFonts: () => void = () => {}
    stubDomFonts(new Promise<void>(r => { resolveFonts = r }))
    const hiddenContainer = makeContainer(0, 0)
    const visibleContainer = makeContainer(800, 600)

    acquireTerminal('s-b', 'ref-b', {})
    attachTerminal('s-b', hiddenContainer)
    resolveFonts()
    await flushAsyncChain()
    // fit() no-ops on a zero-size container — the size stays unreported
    // rather than leaking the 80x24 constructor defaults.
    expect(getTerminalSize('s-b')).toEqual({ cols: 0, rows: 0 })

    attachTerminal('s-b', visibleContainer)
    await flushAsyncChain()
    expect(getTerminalSize('s-b')).toEqual({ cols: 120, rows: 30 })
  })
})
