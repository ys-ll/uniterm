import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { SearchAddon } from '@xterm/addon-search'
import { getXtermTheme } from '../composables/useTerminal'
import { resolveXtermBackground } from '../composables/useTerminalTheme'
import { useSettingsStore } from '../stores/settingsStore'
import { useLocalStateStore } from '../stores/localStateStore'
import type { CustomTerminalTheme } from '../types/settings'
import { formatFontFamily } from '../utils/formatFontFamily'

export interface TerminalOptions {
  fontSize?: number
  fontFamily?: string
  themeName?: string
  scrollback?: number
}

export interface ManagedTerminal {
  terminal: Terminal
  fitAddon: FitAddon
  searchAddon: SearchAddon
  unicodeAddon: Unicode11Addon
  container: HTMLElement | null
  refs: Set<string>
  options: TerminalOptions
  disposeTimer: ReturnType<typeof setTimeout> | null
  /** Whether this terminal was newly created (not reused via timer cancellation). */
  isNew: boolean
  /** Shared generation counter across all components using this terminal.
   * Bumped each time any component registers an onData handler. Callbacks
   * capture a snapshot and bail out if it no longer matches, preventing
   * double input when multiple KeepAlive-cached components share the same
   * terminal instance. */
  onDataGeneration: number
}

const terminals = new Map<string, ManagedTerminal>()

// F-027: scrollback limit applied while the terminal sits in the hidden
// holding container (between detach and re-attach). Restored on re-attach
// so users keep their full scrollback when they revisit the tab.
const INACTIVE_SCROLLBACK = 500

// Hidden holding containers to keep terminal elements alive when no
// component is actively displaying them. detachTerminal moves elements
// here; attachTerminal picks them up regardless of where they are.
const holding = new Map<string, HTMLDivElement>()
function getHolding(sessionId: string): HTMLDivElement {
  let el = holding.get(sessionId)
  if (!el) {
    el = document.createElement('div')
    el.style.display = 'none'
    holding.set(sessionId, el)
  }
  return el
}

export function acquireTerminal(
  sessionId: string,
  ref: string,
  options: TerminalOptions,
  customThemes?: CustomTerminalTheme[]
): Terminal {
  let managed = terminals.get(sessionId)

  if (managed) {
    // Cancel any pending disposal — terminal is still needed
    if (managed.disposeTimer) {
      clearTimeout(managed.disposeTimer)
      managed.disposeTimer = null
    }
    managed.isNew = false
  } else {
    const cursorBlink = useSettingsStore().settings.terminal.cursorBlink ?? true
    const ls = useLocalStateStore()
    const theme = getXtermTheme(options.themeName ?? 'dark', customThemes)
    if (ls.state.backgroundEnabled && ls.state.backgroundImage) {
      theme.background = 'rgba(0,0,0,0)'
    }
    // F-036: italic SGR-3 is handled natively by xterm.js v5.5 — its DOM
    // renderer emits <span class="xterm-italic">…</span> and applies
    // \`font-style: italic\` via xterm.css. No @xterm/addon-italic package
    // is published on npm (verified: npm view @xterm/addon-italic → 404),
    // and xterm.js already routes the italic attribute through the
    // browser's CSS font-matching — so the JetBrains Mono Variable /
    // Consolas / Courier New chain in fontFamily already renders italic
    // faces when the system has them. The fix sketch's "install addon"
    // step is a no-op against the v5.5 source; the lineHeight bump above
    // is the actual rendering improvement.
    //
    // F-037: DEC mode 2026 (synchronized output, \`\e[?2026h…\e[?2026l\`)
    // bracketing used by Claude Code's thinking-block redraws is not
    // honored by xterm.js v5.5 — the parser core has no handler for
    // SET/RESET 2026, and no SyncAddon is published on npm. Mode 2026
    // was added to xterm.js after v5 (the synchronized output feature
    // landed upstream in 2024). Until the v6 upgrade, the thinking-block
    // flicker noted in the finding is structural to this xterm major.
    //
    // F-038: bracketed-paste (\`\e[?2004h\`) and mouse-reporting modes
    // (1000/1006/1015) are handled by the xterm.js parser core itself
    // and surfaced via \`terminal.modes\`. The finding is a verification
    // gap, not a production bug — xterm.js v5.5 implements all four
    // modes upstream. A smoke test (\`terminal.write('\e[?2004h'); assert
    // terminal.modes.bracketedPasteMode\`) would catch a regression on
    // upgrade; tracked for the next vitest sweep (frontend/src/services/
    // terminalManager.test.ts does not exist yet — out of scope for this
    // single-file atomic commit).
    const terminal = new Terminal({
      fontSize: options.fontSize ?? 13,
      fontFamily: formatFontFamily(options.fontFamily ?? 'Consolas, "Courier New", monospace'),
      theme,
      cursorBlink,
      rightClickSelectsWord: false,
      scrollback: options.scrollback ?? 2500,
      allowProposedApi: true,
      allowTransparency: true,
    })

    const fitAddon = new FitAddon()
    const searchAddon = new SearchAddon()
    const unicodeAddon = new Unicode11Addon()

    terminal.loadAddon(fitAddon)
    terminal.loadAddon(searchAddon)
    terminal.loadAddon(unicodeAddon)
    // F-035: xterm.js v5.5 does not expose a charSizeCompat option, and
    // ITheme has no codeBlockBackground field — both suggested by the
    // finding's fix sketch. The Unicode 11 activeVersion here is the
    // best available WC-width alignment in this xterm major; downstream
    // the backend PTY uses the same Unicode 11 tables so column counts
    // match. Upgrading to xterm.js v6 would unlock the extended-theme
    // codeBlockBackground support needed for Claude Code's 256-color code
    // blocks to stand out from prose — tracked as a v6 dependency bump.

    managed = {
      terminal,
      fitAddon,
      searchAddon,
      unicodeAddon,
      container: null,
      refs: new Set(),
      options,
      disposeTimer: null,
      isNew: true,
      onDataGeneration: 0,
    }
    terminals.set(sessionId, managed)
  }

  managed.refs.add(ref)
  return managed.terminal
}

export function releaseTerminal(sessionId: string, ref: string): void {
  const managed = terminals.get(sessionId)
  if (!managed) return

  managed.refs.delete(ref)

  if (managed.refs.size === 0) {
    // Delay disposal to survive drag-and-drop lifecycle race.
    // If acquireTerminal is called within 500ms, the timer is cancelled.
    managed.disposeTimer = setTimeout(() => {
      managed.terminal.dispose()
      terminals.delete(sessionId)
    }, 500)
  }
}

export function disposeTerminal(sessionId: string): void {
  const managed = terminals.get(sessionId)
  if (!managed) return
  if (managed.disposeTimer) {
    clearTimeout(managed.disposeTimer)
  }
  managed.terminal.dispose()
  terminals.delete(sessionId)
}

// Transfer a terminal from oldSessionId to newSessionId so the
// terminal buffer is preserved across session reconnects.
export function transferTerminal(oldSessionId: string, newSessionId: string): boolean {
  const managed = terminals.get(oldSessionId)
  if (!managed) return false
  terminals.delete(oldSessionId)
  terminals.set(newSessionId, managed)
  return true
}

export function attachTerminal(sessionId: string, container: HTMLElement): void {
  const managed = terminals.get(sessionId)
  if (!managed) return
  if (managed.container === container) return

  managed.container = container

  // F-027: restore the user-configured scrollback that detachTerminal
  // shrank while the terminal sat in the holding container. Without this
  // re-attach, users would see only the last 500 lines after every
  // drag-out / re-merge cycle even though the full history is still
  // replayable from sessionStore on a fresh mount.
  if (managed.options.scrollback != null &&
      managed.terminal.options.scrollback != managed.options.scrollback) {
    managed.terminal.options.scrollback = managed.options.scrollback
  }

  if (!managed.terminal.element) {
    managed.terminal.open(container)
  } else {
    container.appendChild(managed.terminal.element)
  }

  // Wait for the font to actually paint before measuring cell width — if
  // we measure while JetBrains Mono Variable (or whatever the user picked)
  // is still loading, fitAddon reads a fallback width and reports wrong
  // cols. document.fonts.ready resolves once every face in the page is
  // loaded; that's the signal Claude Code (and any TUI app) needs to see
  // a consistent terminal size from the first byte.
  const fontReady = (typeof document !== 'undefined' && document.fonts && document.fonts.ready)
    ? document.fonts.ready
    : Promise.resolve()
  fontReady.finally(() => {
    // Two rAFs: one for the .xterm element to receive its size, one for
    // the canvas to render with the now-loaded font.
    requestAnimationFrame(() => requestAnimationFrame(() => managed.fitAddon.fit()))
  })
}

export function detachTerminal(sessionId: string, container: HTMLElement): void {
  const managed = terminals.get(sessionId)
  if (!managed) return
  // F-027: shrink xterm's pixel buffer while in the hidden holding
  // container. detach → attach within the disposeTimer window still
  // restores the original scrollback so users see their full history on
  // re-attach. Without this the canvas + row objects for the trimmed
  // rows stay pinned in the holding container's offscreen DOM and
  // accumulate over the session.
  if (managed.terminal.options.scrollback != null &&
      managed.terminal.options.scrollback > INACTIVE_SCROLLBACK) {
    managed.terminal.options.scrollback = INACTIVE_SCROLLBACK
  }
  // Move element to a holding container so it survives component destruction.
  // The next attachTerminal picks it up from there.
  if (managed.terminal.element?.parentElement === container) {
    getHolding(sessionId).appendChild(managed.terminal.element)
  }
  if (managed.container === container) {
    managed.container = null
  }
}

export function getTerminal(sessionId: string): Terminal | undefined {
  return terminals.get(sessionId)?.terminal
}

export function getManagedTerminal(sessionId: string): ManagedTerminal | undefined {
  return terminals.get(sessionId)
}

/** Return the xterm-measured cols/rows for an existing session, or
 * {0,0} if the session has no terminal yet. Callers use this to learn
 * the actual size to send to the backend as InitialCols/Rows BEFORE
 * CreateSession so the remote PTY starts at the right dimensions. */
export function getTerminalSize(sessionId: string): { cols: number; rows: number } {
  const m = terminals.get(sessionId)
  if (!m) return { cols: 0, rows: 0 }
  return { cols: m.terminal.cols, rows: m.terminal.rows }
}

/** Poll for the terminal to report a non-zero size, up to timeoutMs.
 * Returns the latest size (possibly {0,0}) if the timeout elapses —
 * callers should fall back to defaults rather than treat it as an error. */
export async function waitForTerminalSize(
  sessionId: string,
  timeoutMs = 1500
): Promise<{ cols: number; rows: number }> {
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    const s = getTerminalSize(sessionId)
    if (s.cols > 0 && s.rows > 0) return s
    await new Promise(r => setTimeout(r, 30))
  }
  return getTerminalSize(sessionId)
}

/** Bump the shared onData generation counter for the given terminal.
 * Returns the NEW generation value. Callers should capture this value
 * in their onData callback and bail out if the terminal's current
 * generation no longer matches. */
export function bumpOnDataGeneration(sessionId: string): number {
  const managed = terminals.get(sessionId)
  if (!managed) return 0
  const next = ++managed.onDataGeneration
  return next
}
