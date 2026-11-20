import { disposeCommandWraps } from './terminalCommandWrap'
import { Terminal } from '@xterm/xterm'
import { Events } from '@wailsio/runtime'
import { FitAddon } from '@xterm/addon-fit'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { SearchAddon } from '@xterm/addon-search'
import { ImageAddon } from '@xterm/addon-image'
import { ClipboardAddon, Base64 } from '@xterm/addon-clipboard'
import { ProgressAddon, type IProgressState } from '@xterm/addon-progress'
import { getXtermTheme } from '../composables/useTerminal'
import { resolveXtermBackground, applyTerminalBgVar, resolveTerminalThemeName } from '../composables/useTerminalTheme'
import { useSettingsStore } from '../stores/settingsStore'
import { useLocalStateStore } from '../stores/localStateStore'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useZmodemStore } from '../stores/zmodemStore'
import { queuedSessionWrite } from './sessionWriter'
import { SessionResize } from '../../bindings/github.com/ys-ll/uniterm/app'
import { disposeSessionRecorder, getSessionRecorder } from './terminalRecorder'
import type { CustomTerminalTheme } from '../types/settings'
import { formatFontFamily } from '../utils/formatFontFamily'
import { MIN_LINE_NUMBER_DIGITS } from '../utils/terminalGutter'
import { installImeCompatibilityPatch } from '../utils/xtermImeCompatibility'
import {
  bufferRowSource,
  createLineRegistry,
  realignRegistry,
  type LineRegistryState,
} from './terminalTimestamps'

export interface TerminalOptions {
  fontSize?: number
  fontFamily?: string
  // Secondary family (e.g. CJK) appended behind fontFamily at render time.
  fallbackFont?: string
  // Weight for regular terminal text (variable-font weights for JetBrains).
  fontWeight?: number
  themeName?: string
  scrollback?: number
}

export interface ManagedTerminal {
  terminal: Terminal
  fitAddon: FitAddon
  searchAddon: SearchAddon
  unicodeAddon: Unicode11Addon
  progressAddon: ProgressAddon
  container: HTMLElement | null
  refs: Set<string>
  /** Component instances (terminalInstanceRef keys) currently mounted AND
   * visible. A terminal with no active refs is KeepAlive-deactivated: its
   * xterm buffer is frozen (BaseTerminal gates live session:data writes on
   * component activity and replays on reactivation), so screen-buffer reads
   * are only valid while this is non-empty. */
  activeRefs: Set<string>
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
  /** Logical-line registry (sequential number + birth/completion time per
   * shell line), keyed by absolute row index (lineOffset-compensated). Backs
   * the gutter's line-number and time columns. Kept on the shared terminal so
   * it survives KeepAlive and drag-across-panes. */
  lineRegistry: LineRegistryState
  /** Largest reserved line-number digit count, shared across gutter mounts. */
  lineNumberDigits: number
  /** Absolute-row offset accumulated from scrollback trimming, so both the
   * line-number and timestamp columns stay continuous as old lines drop off
   * the top of the buffer. */
  lineOffset: number
  /** Subscription to the normal-buffer line-collection onTrim event. */
  trimDispose: { dispose(): void } | null
  /** Subscription to terminal.onResize — reflows the buffer, so the registry
   * must be re-keyed to the rows lines now start at. */
  resizeDispose: { dispose(): void } | null
  /** Subscription to ProgressAddon.onChange — routes OSC 9;4 progress
   * state into the tab store for the tab-bar indicator. */
  progressDispose: { dispose(): void } | null
  /** IME compatibility patch installed on this terminal (macOS only). */
  imeDispose: { dispose(): void } | null
  /** Whether the terminal has had its first real fit — the font-ready
   * measurement in attachTerminal, not the xterm.js constructor defaults
   * (80x24). getTerminalSize reports 0x0 until this is set so callers
   * waiting for the initial size never mistake the constructor defaults
   * for a measurement — the PTY would start at 80x24, and the
   * already-drawn banner would stay wrapped after the later resize. */
  hasFitted: boolean
  /** Last cols/rows this module pushed to the backend from the font-ready
   * fit, so KeepAlive re-attaches don't spam redundant SessionResize calls. */
  lastSyncedCols: number
  lastSyncedRows: number
}

const terminals = new Map<string, ManagedTerminal>()

// ---------------------------------------------------------------------------
// Headless mirror terminal (AI output path)
// ---------------------------------------------------------------------------
// While a panel is deactivated, its visible terminal's buffer freezes (live
// session:data writes are gated on component activity and replayed on
// reactivation). The AI executor still needs emulator-faithful output:
// raw-stream text reconstruction cannot resolve ConPTY cursor-positioning
// redraws (the issue-624 class), which glues prompt redraws onto output rows
// and silently drops them. A second, never-rendered Terminal instance per
// session parses the same stream continuously, so its buffer always matches
// what the screen would show. Cost: one extra headless parse per session —
// no DOM, no renderer.
const MIRROR_SCROLLBACK_CAP = 2000

interface TerminalMirror {
  terminal: Terminal
  lineOffset: number
  trimDispose: { dispose(): void } | null
  resizeDispose: { dispose(): void } | null
  /** Inactive-window device-query responder (responder handoff). */
  replyDispose: { dispose(): void } | null
  unsub: () => void
}

const mirrors = new Map<string, TerminalMirror>()

function createMirror(sessionId: string, options: TerminalOptions, source: Terminal): TerminalMirror {
  // RESPONDER HANDOFF (refined single-responder rule): exactly one parser may
  // answer device queries (DA, CPR/ESC[6n, DSR…). While the visible panel is
  // ACTIVE, that is the visible terminal — this mirror must stay silent, or a
  // second reply corrupts remote apps. While the panel is INACTIVE its buffer
  // freezes and its onData handlers are unbound, so a query would never be
  // answered: the remote's read times out, and the stale reply the visible
  // terminal emits during the reactivation gap replay leaks into the shell —
  // the "0R" artifact after switching back to a background-opened SSH tab.
  // During that window the mirror is the stream's only parser, so it takes
  // over answering (queuedSessionWrite feeds the reply straight to the PTY);
  // the visible terminal suppresses its own stale replies during the replay.
  const terminal = new Terminal({
    // Unicode11Addon needs the proposed API (unicode.activeVersion), same as
    // the visible terminal's constructor.
    allowProposedApi: true,
    // Bound mirror memory independently of the user's scrollback setting;
    // absolute rows stay self-consistent within the mirror.
    scrollback: Math.min(options.scrollback ?? 2500, MIRROR_SCROLLBACK_CAP),
  })
  // Match the visible terminal's CJK width behavior (see acquireTerminal):
  // the addon must be loaded before switching unicode.activeVersion, or xterm
  // throws "Unicode 11 addon not loaded".
  terminal.loadAddon(new Unicode11Addon())
  terminal.unicode.activeVersion = '11'
  const mirror: TerminalMirror = {
    terminal,
    lineOffset: 0,
    trimDispose: null,
    resizeDispose: null,
    replyDispose: null,
    unsub: () => {},
  }
  // Mirror grid must match the PTY size the visible terminal negotiated:
  // ConPTY streams are size-dependent (cursor positioning, wrapping), so a
  // default 80x24 mirror scrambles the layout — the cursor ends up on a blank
  // row and prompt snapshots come back empty. Follow the source from now on.
  mirror.resizeDispose = source.onResize(({ cols, rows }) => {
    terminal.resize(cols, rows)
  })
  terminal.resize(source.cols, source.rows)
  // Track scrollback trimming so absolute rows stay stable — same internal
  // API and guard as the visible terminal's lineOffset bookkeeping.
  try {
    const core = (terminal as any)._core
    const lines = core?._bufferService?.buffers?.normal?.lines
    if (typeof lines?.onTrim === 'function') {
      const m = mirror
      m.trimDispose = lines.onTrim((amount: number) => {
        m.lineOffset += amount
      })
    }
  } catch { /* noop */ }
  mirror.unsub = Events.On('session:data', (ev: any) => {
    const payload = ev?.data
    if (!payload || payload.id !== sessionId || typeof payload.data !== 'string') return
    // Central recording hook: the mirror receives every session:data event
    // regardless of panel visibility, so a running recording keeps capturing
    // output while the terminal sits in the background.
    getSessionRecorder(sessionId).writeData(payload.data)
    // Same zmodem cancel-window swallow as sessionStore: residual binary
    // garbage from an aborted transfer must not reach the mirror buffer.
    try {
      if (Date.now() < useZmodemStore().getCancelUntil(payload.id)) return
    } catch { /* pinia not installed yet */ }
    mirror.terminal.write(payload.data)
  })
  // Inactive-window responder (see the handoff comment above): the activity
  // check runs at reply-generation time, so a query answered just as the
  // panel reactivates falls back to the visible terminal — worst case the
  // remote's read falls back on its own timeout, same as before the handoff.
  mirror.replyDispose = terminal.onData((data) => {
    if (isTerminalActive(sessionId)) return
    if (data) queuedSessionWrite(sessionId, data)
  })
  return mirror
}

function disposeMirror(sessionId: string): void {
  const mirror = mirrors.get(sessionId)
  if (!mirror) return
  mirrors.delete(sessionId)
  mirror.unsub()
  mirror.trimDispose?.dispose()
  mirror.resizeDispose?.dispose()
  mirror.replyDispose?.dispose()
  mirror.terminal.dispose()
}

// Cursor line + absolute row on the mirror buffer — prompt snapshot source
// for inactive terminals.
export function getMirrorPromptSnapshot(sessionId: string): { promptLine: string; startRow: number } | null {
  const mirror = mirrors.get(sessionId)
  if (!mirror) return null
  const buf = mirror.terminal.buffer.active
  const line = buf.getLine(buf.baseY + buf.cursorY)
  const promptLine = line ? line.translateToString(true).trimEnd() : ''
  return { promptLine, startRow: mirror.lineOffset + buf.baseY + buf.cursorY }
}

// Screen text from the mirror buffer starting at an absolute row — the
// inactive-terminal counterpart of reading the visible terminal's screen.
// Returns null when no mirror exists for the session.
export function readMirrorScreenFromRow(sessionId: string, absStartRow: number): string | null {
  const mirror = mirrors.get(sessionId)
  if (!mirror || absStartRow < 0) return null
  const buf = mirror.terminal.buffer.active
  let first: number
  let last: number
  if (buf.type === 'alternate') {
    first = buf.baseY
    last = buf.length - 1
  } else {
    first = Math.max(0, absStartRow - mirror.lineOffset)
    last = buf.length - 1
    while (last >= first && mirrorRowBlank(buf, last)) last--
    if (last < first) return ''
  }
  const lines: string[] = []
  for (let i = first; i <= last; i++) {
    const line = buf.getLine(i)
    if (line) lines.push(line.translateToString(true))
  }
  return lines.join('\n')
}

// Last tailLines non-blank-terminated lines from the mirror buffer — the
// inactive-terminal counterpart of captureTerminal. Returns null when no
// mirror exists for the session.
export function readMirrorTail(sessionId: string, tailLines: number): string | null {
  const mirror = mirrors.get(sessionId)
  if (!mirror) return null
  const buf = mirror.terminal.buffer.active
  if (buf.length === 0) return ''
  let last = buf.length - 1
  while (last >= 0 && mirrorRowBlank(buf, last)) last--
  if (last < 0) return ''
  const first = Math.max(0, last - tailLines + 1)
  const lines: string[] = []
  for (let i = first; i <= last; i++) {
    const line = buf.getLine(i)
    if (line) lines.push(line.translateToString())
  }
  return lines.join('\n')
}

function mirrorRowBlank(
  buf: { getLine(n: number): { translateToString(): string } | undefined },
  row: number
): boolean {
  const line = buf.getLine(row)
  return !line || line.translateToString().trim() === ''
}

// Route a progress state from a session's terminal to the tab that displays
// it (session → panel → tab, same resolution the notification dots use).
function setTabProgressForSession(sessionId: string, state: IProgressState | null): void {
  const panelStore = usePanelStore()
  const tabStore = useTabStore()
  for (const [panelId, panel] of panelStore.panels) {
    if (panel.sessionId !== sessionId) continue
    const tab = tabStore.tabs.find(t =>
      (t.type === 'terminal' && t.panelId === panelId) ||
      (t.type === 'workspace' && t.panelIds.includes(panelId))
    )
    if (tab) tabStore.setTabProgress(tab.id, state)
    return
  }
}

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
    const wordSeparator = useSettingsStore().settings.terminal.wordSeparator
      || '\\ :;~`!@#$%^&*()=+|[]{}\'",<>?'
    const ls = useLocalStateStore()
    const theme = resolveXtermBackground(
      getXtermTheme(resolveTerminalThemeName(options.themeName, useSettingsStore().resolvedAppTheme), customThemes),
      ls.state.backgroundEnabled,
      ls.state.backgroundImage
    )
    // F-036: italic SGR-3 is handled natively by xterm.js — its DOM
    // renderer emits <span class="xterm-italic">…</span> and applies
    // `font-style: italic` via xterm.css. No @xterm/addon-italic package
    // is published on npm (verified: npm view @xterm/addon-italic → 404),
    // and xterm.js already routes the italic attribute through the
    // browser's CSS font-matching — so the JetBrains Mono Variable /
    // Consolas / Courier New chain in fontFamily already renders italic
    // faces when the system has them.
    //
    // F-037: DEC mode 2026 (synchronized output) bracketing used by
    // Claude Code's thinking-block redraws was not honored by xterm.js
    // v5.5 — the parser core had no handler for SET/RESET 2026. The v6
    // upgrade (PR #437) is what addresses the thinking-block flicker.
    //
    // F-038: bracketed-paste (`\e[?2004h`) and mouse-reporting modes
    // (1000/1006/1015) are handled by the xterm.js parser core itself
    // and surfaced via `terminal.modes`. This is a verification gap,
    // not a production bug — a smoke test asserting
    // terminal.modes.bracketedPasteMode after writing the sequence
    // would catch a regression on future upgrades.
    const terminal = new Terminal({
      fontSize: options.fontSize ?? 13,
      fontFamily: formatFontFamily(options.fontFamily ?? 'JetBrains Mono Variable', options.fallbackFont),
      fontWeight: options.fontWeight ?? 400,
      theme,
      cursorBlink,
      cursorStyle: useSettingsStore().settings.terminal.cursorStyle ?? 'block',
      rightClickSelectsWord: false,
      scrollback: options.scrollback ?? 2500,
      allowProposedApi: true,
      allowTransparency: true,
      // Boost text/background contrast (F-039): xterm auto-brightens or
      // darkens the foreground toward black/white until the configured
      // minimumContrast ratio (1 = off) is met — only for cells below it,
      // normal text is untouched. Fixes ls's colored blocks for 777 dirs
      // (e.g. ow=34;42 blue-on-green is ~1.3:1, illegible) staying readable.
      minimumContrastRatio: useSettingsStore().settings.terminal.minimumContrast ?? 4.5,
      wordSeparator,
    })

    // ctrl+wheel is font zoom (App.vue). xterm v6 reports wheel events to the
    // app regardless of defaultPrevented, so vim would scroll on zoom too.
    terminal.attachCustomWheelEventHandler(ev => !ev.ctrlKey)

    const fitAddon = new FitAddon()
    const searchAddon = new SearchAddon()
    const unicodeAddon = new Unicode11Addon()
    // Terminal image support (sixel + iTerm2 OSC 1337). The addon parses the
    // sequences itself and draws them on its own canvas layer, so it works
    // with the default DOM renderer. DCS sixel sequences are reassembled
    // chunk-wise in BaseTerminal's render path (dcsReassembler) BEFORE they
    // reach xterm, so the parser only ever sees complete sequences.
    const imageAddon = new ImageAddon()
    // OSC 52 clipboard: remote programs (vim "+ register, tmux, ssh through
    // chains) can WRITE to the local clipboard. Reads are deliberately
    // answered with an empty report instead of a rejection — the addon does
    // not catch provider promise rejections (verified in its source), so a
    // rejected read would surface as an unhandled rejection, and refusing to
    // read also keeps remote processes from exfiltrating local clipboard
    // contents. Writes swallow failures (window unfocused etc.) for the
    // same unhandled-rejection reason.
    const clipboardAddon = new ClipboardAddon(new Base64(), {
      readText: () => '',
      writeText: (_selection, text) => {
        navigator.clipboard.writeText(text).catch(() => { /* non-critical */ })
      },
    })
    // ConEmu-style OSC 9;4 progress, surfaced as a tab-bar indicator.
    const progressAddon = new ProgressAddon()

    terminal.loadAddon(fitAddon)
    terminal.loadAddon(searchAddon)
    terminal.loadAddon(unicodeAddon)
    terminal.loadAddon(imageAddon)
    terminal.loadAddon(clipboardAddon)
    terminal.loadAddon(progressAddon)
    // F-035: charSizeCompat / ITheme.codeBlockBackground do not exist in
    // xterm.js v5.5. The Unicode 11 activeVersion below is the available
    // WC-width alignment — the backend PTY uses the same Unicode 11
    // tables so column counts match. The v6 upgrade (PR #437) is what
    // unlocks extended-theme support.
    //
    // Activate Unicode 11 widths (default v6 miscounts CJK cells). Must come
    // AFTER loadAddon — the unicode property is provided by the addon.
    // Issue #902: CJK characters were truncated at the right boundary because
    // the default Unicode 6 width table miscounts full-width cells. Loading
    // Unicode 11 and setting activeVersion='11' ensures both the visible
    // terminal and the headless mirror (createMirror above) use the same
    // WCWidth as the backend PTY, so autowap fires at the correct column.
    terminal.unicode.activeVersion = '11'

    // IME compatibility (macOS) is installed in attachTerminal, after
    // terminal.open() — the patch needs textarea/_compositionHelper, which
    // xterm only creates there.
    managed = {
      terminal,
      fitAddon,
      searchAddon,
      unicodeAddon,
      progressAddon,
      container: null,
      refs: new Set(),
      activeRefs: new Set(),
      options,
      disposeTimer: null,
      isNew: true,
      onDataGeneration: 0,
      lineRegistry: createLineRegistry(),
      lineNumberDigits: MIN_LINE_NUMBER_DIGITS,
      lineOffset: 0,
      trimDispose: null,
      resizeDispose: null,
      progressDispose: null,
      imeDispose: null,
      hasFitted: false,
      lastSyncedCols: 0,
      lastSyncedRows: 0,
    }
    // Headless mirror starts parsing the session stream immediately so its
    // buffer is always current when the panel goes to the background. Never
    // let a mirror failure break terminal creation — the AI path degrades to
    // the raw-stream fallback when no mirror exists.
    try {
      mirrors.set(sessionId, createMirror(sessionId, options, terminal))
    } catch (e) {
      console.warn('[terminalManager] headless mirror creation failed', e)
    }

    // Track scrollback trimming so line-numbers / timestamps stay continuous
    // as old lines drop off the top of the buffer. Internal API — guarded.
    // Doesn't survive terminal re-creation, but a fresh terminal restarts at 0.
    // Snapshot `managed` into a const so the onTrim closure doesn't re-widen it.
    const m = managed
    try {
      const core = (terminal as any)._core
      const lines = core?._bufferService?.buffers?.normal?.lines
      if (typeof lines?.onTrim === 'function') {
        m.trimDispose = lines.onTrim((amount: number) => {
          // NOTE: deliberately no entry pruning here. "key < lineOffset ⇒
          // off-buffer" only holds for append+trim growth; a resize reflow
          // also fires trims while INSERTING physical rows for wrapped lines,
          // so surviving content's absolute coordinates shift and live
          // entries would be deleted (tail lines lose their number/time).
          // Off-buffer entries are bounded by the size cap in the registry
          // and dropped wholesale by realignRegistry on the next resize.
          m.lineOffset += amount
        })
      }
    } catch { /* noop */ }

    // A resize reflows the (normal) buffer: physical rows move but logical
    // lines keep their identity. Re-key the registry so each entry points at
    // the row its line now starts at. Uses the normal buffer explicitly so a
    // resize while an alternate-screen app is up still realigns.
    m.resizeDispose = terminal.onResize(() => {
      const buf = terminal.buffer.normal
      realignRegistry(m.lineRegistry, {
        lineOffset: m.lineOffset,
        cursorAbs: m.lineOffset + buf.baseY + buf.cursorY,
        source: bufferRowSource(buf),
      })
    })

    // OSC 9;4 progress → tab-bar indicator. One subscription per terminal
    // (at creation) so KeepAlive re-mounts never stack listeners; state 0
    // clears the indicator.
    m.progressDispose = progressAddon.onChange(state => {
      setTabProgressForSession(sessionId, state.state === 0 ? null : state)
    })

    terminals.set(sessionId, m)
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
      disposeMirror(sessionId)
      // Terminal is gone: finalize any running recording for this session.
      void disposeSessionRecorder(sessionId)
      managed.trimDispose?.dispose()
      managed.trimDispose = null
      managed.resizeDispose?.dispose()
      managed.resizeDispose = null
      managed.progressDispose?.dispose()
      managed.progressDispose = null
      setTabProgressForSession(sessionId, null)
      managed.imeDispose?.dispose()
      managed.imeDispose = null
      disposeCommandWraps(managed.terminal)
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
  disposeMirror(sessionId)
  // Terminal is gone: finalize any running recording for this session.
  void disposeSessionRecorder(sessionId)
  managed.trimDispose?.dispose()
  managed.trimDispose = null
  managed.resizeDispose?.dispose()
  managed.resizeDispose = null
  managed.progressDispose?.dispose()
  managed.progressDispose = null
  setTabProgressForSession(sessionId, null)
  managed.imeDispose?.dispose()
  managed.imeDispose = null
  disposeCommandWraps(managed.terminal)
  managed.terminal.dispose()
  terminals.delete(sessionId)
}

// Transfer a terminal from oldSessionId to newSessionId so the
// terminal buffer is preserved across session reconnects.
export function transferTerminal(oldSessionId: string, newSessionId: string): boolean {
  const managed = terminals.get(oldSessionId)
  if (!managed) return false
  // Migrate the mirror: its event subscription filters by sessionId and the
  // new session's history lives in the sessionStore — rebuild for the new id,
  // seeded with everything buffered so far.
  disposeMirror(oldSessionId)
  terminals.delete(oldSessionId)
  terminals.set(newSessionId, managed)
  // Re-target the progress subscription: its closure captured oldSessionId,
  // and events for the old id would resolve to no panel and strand a stale
  // indicator on the previous tab.
  managed.progressDispose?.dispose()
  managed.progressDispose = managed.progressAddon.onChange(state => {
    setTabProgressForSession(newSessionId, state.state === 0 ? null : state)
  })
  setTabProgressForSession(oldSessionId, null)
  try {
    const mirror = createMirror(newSessionId, managed.options, managed.terminal)
    mirrors.set(newSessionId, mirror)
    // Seed with the new session's buffered history (reconnect replay reaches
    // the visible terminal directly, not through session:data events).
    const sessionStore = useSessionStore()
    const total = sessionStore.getChunkCount(newSessionId)
    if (total > 0) mirror.terminal.write(sessionStore.getDataFromChunk(newSessionId, 0))
  } catch (e) {
    console.warn('[terminalManager] headless mirror re-creation failed', e)
  }
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

  // IME compatibility (macOS): deliver single-char input directly when an
  // IME marks keystrokes with the phantom keyCode 229, instead of relying on
  // xterm's racy deferred textarea diff that drops chars under fast typing.
  // Must run after open() — that is where xterm creates the textarea and the
  // composition helper the patch hooks into; installing at construction time
  // silently no-ops.
  if (!managed.imeDispose) {
    managed.imeDispose = installImeCompatibilityPatch(managed.terminal)
  }

  // terminal.element only exists after open(), so the padding-ring color is
  // published here rather than at construction time.
  applyTerminalBgVar(managed.terminal, managed.terminal.options.theme ?? {})

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
    requestAnimationFrame(() => requestAnimationFrame(() => {
      managed.fitAddon.fit()
      // fit() no-ops on a zero-size container (hidden tab / not yet laid
      // out) — only mark the size real, and push it to the backend, when
      // the container actually has pixels. Until then getTerminalSize
      // keeps reporting 0x0 so waitForTerminalSize callers keep waiting
      // instead of picking up the 80x24 constructor defaults.
      const rect = managed.container?.getBoundingClientRect()
      if (!rect || rect.width === 0 || rect.height === 0) return
      managed.hasFitted = true
      // Push the font-measured grid to the PTY: BaseTerminal's mount
      // retries fire before the fonts finish loading and may already have
      // sent fallback-font dimensions. Skipped when unchanged so
      // re-attaches (every tab switch) don't spam redundant resizes.
      const t = managed.terminal
      if (t.cols > 0 && t.rows > 0 &&
          (t.cols !== managed.lastSyncedCols || t.rows !== managed.lastSyncedRows)) {
        managed.lastSyncedCols = t.cols
        managed.lastSyncedRows = t.rows
        SessionResize(sessionId, t.cols, t.rows).catch(() => {})
      }
    }))
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

// Mark a component instance as actively displaying the terminal (mounted and
// KeepAlive-active). Called by BaseTerminal on mount/activate with its unique
// terminalInstanceRef, and with active=false on deactivate/unmount.
export function markTerminalActive(sessionId: string, ref: string, active: boolean): void {
  const managed = terminals.get(sessionId)
  if (!managed) return
  if (active) managed.activeRefs.add(ref)
  else managed.activeRefs.delete(ref)
}

// True while any visible component drives the terminal. When false the xterm
// buffer is frozen at the deactivation state — screen-buffer readers (AI
// output capture, prompt snapshots) must fall back to the buffered PTY stream
// in sessionStore instead.
export function isTerminalActive(sessionId: string): boolean {
  const managed = terminals.get(sessionId)
  return !!managed && managed.activeRefs.size > 0
}

export function getManagedTerminal(sessionId: string): ManagedTerminal | undefined {
  return terminals.get(sessionId)
}

/** Return the xterm-measured cols/rows for an existing session, or
 * {0,0} if the session has no terminal yet or hasn't had its first real
 * fit (font-ready measurement in attachTerminal). The gate matters: the
 * xterm.js constructor starts every terminal at 80x24, and reporting
 * those defaults before the first fit made waitForTerminalSize callers
 * start the PTY at 80x24. Callers use this to learn the
 * actual size to send to the backend as InitialCols/Rows BEFORE
 * CreateSession so the remote PTY starts at the right dimensions. */
export function getTerminalSize(sessionId: string): { cols: number; rows: number } {
  const m = terminals.get(sessionId)
  if (!m || !m.hasFitted) return { cols: 0, rows: 0 }
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
