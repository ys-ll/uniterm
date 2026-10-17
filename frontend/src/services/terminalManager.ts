import { reactive } from 'vue'
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
import { queuedSessionWrite, queuedSessionReply, resizeSessionView, setSessionInputSizeProvider, enqueueSessionOperation } from './sessionWriter'
import type { CustomTerminalTheme } from '../types/settings'
import { formatFontFamily } from '../utils/formatFontFamily'
import { installImeCompatibilityPatch } from '../utils/xtermImeCompatibility'
import { isWebMode } from '../utils/webMode'
import { bufferRowSource, createLineRegistry, realignRegistry, recordWrite, currentAbsoluteLine, stampCommandLine, type LineRegistryState } from './terminalTimestamps'
import { TerminalStream, installTerminalInputRouter } from './terminalStream'
import { captureTerminalSnapshot, restoreTerminalSnapshot } from './terminalSnapshot'
import { webTerminalBridge, type SnapshotRequest, type WebSession, type TerminalPresentation } from './webTerminalBridge'
import { splitZmodemHeader } from './zmodemHeader'

export interface TerminalOptions {
  fontSize?: number
  fontFamily?: string
  fallbackFont?: string
  fontWeight?: number
  themeName?: string
  scrollback?: number
  sftp?: boolean
}
type Disposable = { dispose(): void }
type TerminalEvent = { id: string; webSequence: number; webTimestamp?: number; data?: string; binary?: boolean; cols?: number; rows?: number; activeClient?: string; operation?: TerminalPresentation; closed?: boolean }
export interface ManagedTerminal {
  terminal: Terminal
  fitAddon: FitAddon
  searchAddon: SearchAddon
  unicodeAddon: Unicode11Addon
  imageAddon: ImageAddon
  progressAddon: ProgressAddon
  stream: TerminalStream
  container: HTMLElement | null
  refs: Set<string>
  activeRefs: Set<string>
  options: TerminalOptions
  disposeTimer: ReturnType<typeof setTimeout> | null
  isNew: boolean
  onDataGeneration: number
  lineRegistry: LineRegistryState
  lineOffset: number
  disposables: Disposable[]
  imeDispose: Disposable | null
  sessionId: string
  activeClient: string
  syncing: boolean
  pending: TerminalEvent[]
  pendingBytes: number
  generation: number
  closed: boolean
  trimDispose?: Disposable
  eventBytes: number
  eventCount: number
  protocolTail: string
  fileTransferActive: boolean
  /** One desktop file broker, installed independently of visible refs. null
   * consumes protocol bytes; returned text alone enters the authority parser. */
  outputBroker?: (data: string, binary?: boolean) => string | null | Promise<string | null>
  brokerDispose?: () => void
  session?: Partial<WebSession> & { id: string }
}

const terminals = new Map<string, ManagedTerminal>()
const holding = new Map<string, HTMLDivElement>()
const textTypes = new Set(['local', 'wsl', 'ssh', 'telnet', 'mosh', 'serial', 'tcp', 'container-exec'])
export const terminalSharingErrors = reactive(new Map<string, string>())
let listeners: (() => void)[] = []
let webDisconnected = false
const syncingMessage = 'Synchronizing terminal; input is paused until the snapshot and live output are restored.'
const offlineMessage = 'Event connection interrupted; terminal input is paused until reconnection and a fresh snapshot.'
const transferMessage = 'ZMODEM transfer in progress; the terminal will restore automatically when it finishes.'
type TerminalBroker = { consume: NonNullable<ManagedTerminal['outputBroker']>; dispose: () => void }
let brokerFactory: ((session: Partial<WebSession> & { id: string }) => TerminalBroker | undefined) | undefined

function failed(id: string, error: unknown): void {
  const m = terminals.get(id)
  if (isWebMode() && m && !m.options.sftp) pauseBrowserTerminal(m, String(error))
  terminalSharingErrors.set(id, String(error))
  console.error('[terminal sharing]', id, error)
}

function pauseBrowserTerminal(m: ManagedTerminal, reason: string): void {
  m.generation++
  m.syncing = true
  m.pending = []
  m.pendingBytes = 0
  m.protocolTail = ''
  terminalSharingErrors.set(m.sessionId, reason)
}

function inputFailed(id: string, error: unknown): void {
  // A refused keystroke must not invalidate the snapshot already in flight.
  // Its pause reason is already visible; a transport failure on a ready view
  // instead blocks that view until an explicit refresh/reconnect succeeds.
  if (isWebMode() && terminals.get(id)?.syncing) return
  failed(id, error)
}

function getHolding(id: string): HTMLDivElement {
  let host = holding.get(id)
  if (!host) {
    host = document.createElement('div')
    // Keep image parsers/render metrics alive before any visible view mounts.
    host.style.cssText = 'position:fixed;left:-10000px;top:0;width:800px;height:600px;visibility:hidden;pointer-events:none'
    document.body.appendChild(host)
    holding.set(id, host)
  }
  return host
}

function setTabProgressForSession(id: string, state: IProgressState | null): void {
  const tabStore = useTabStore()
  for (const [panelId, panel] of usePanelStore().panels) {
    if (panel.sessionId !== id) continue
    const tab = tabStore.tabs.find(t => (t.type === 'terminal' && t.panelId === panelId) || (t.type === 'workspace' && t.panelIds.includes(panelId)))
    if (tab) tabStore.setTabProgress(tab.id, state)
  }
}

export function acquireTerminal(id: string, ref: string, options: TerminalOptions, customThemes?: CustomTerminalTheme[]): Terminal {
  initWebTerminals()
  let m = terminals.get(id)
  if (m) {
    if (m.disposeTimer) clearTimeout(m.disposeTimer)
    m.disposeTimer = null
    m.isNew = false
    if (options.sftp !== undefined) {
      m.options.sftp = options.sftp
      m.stream.options.sftp = options.sftp
      if (options.sftp) { m.syncing = false; terminalSharingErrors.delete(id) }
      setSizeProvider(m)
    }
    if (ref) m.refs.add(ref)
    return m.terminal
  }
  const settings = useSettingsStore()
  const ts = settings.settings.terminal
  const ls = useLocalStateStore()
  const theme = resolveXtermBackground(getXtermTheme(resolveTerminalThemeName(options.themeName, settings.resolvedAppTheme), customThemes), ls.state.backgroundEnabled, ls.state.backgroundImage)
  const terminal = new Terminal({
    fontSize: options.fontSize ?? 13, fontFamily: formatFontFamily(options.fontFamily ?? 'JetBrains Mono Variable', options.fallbackFont),
    fontWeight: options.fontWeight ?? 400, theme, cursorBlink: ts.cursorBlink ?? true, cursorStyle: ts.cursorStyle ?? 'block',
    rightClickSelectsWord: false, scrollback: options.scrollback ?? 2500, allowProposedApi: true, allowTransparency: true,
    minimumContrastRatio: ts.minimumContrast ?? 4.5, wordSeparator: ts.wordSeparator || '\\ :;~`!@#$%^&*()=+|[]{}\'",<>?',
  })
  terminal.attachCustomWheelEventHandler(ev => !ev.ctrlKey)
  const fitAddon = new FitAddon(), searchAddon = new SearchAddon(), unicodeAddon = new Unicode11Addon()
  const imageAddon = new ImageAddon(), progressAddon = new ProgressAddon()
  for (const addon of [fitAddon, searchAddon, unicodeAddon, imageAddon, progressAddon]) terminal.loadAddon(addon)
  // OSC52 executes only at the authority, never on browser restore or live
  // mirrored output. File broker similarly consumes bytes only on desktop.
  terminal.loadAddon(new ClipboardAddon(new Base64(), {
    readText: () => '',
    writeText: (_selection, text) => { if (!isWebMode()) navigator.clipboard?.writeText(text).catch(() => {}) },
  }))
  terminal.unicode.activeVersion = '11'
  const stream = new TerminalStream(terminal, { cursorBlink: ts.cursorBlink ?? true, sftp: options.sftp ?? false })
  m = {
    terminal, fitAddon, searchAddon, unicodeAddon, imageAddon, progressAddon, stream,
    sessionId: id, container: null, refs: new Set(ref ? [ref] : []), activeRefs: new Set(), options,
    disposeTimer: null, isNew: true, onDataGeneration: 0, lineRegistry: createLineRegistry(), lineOffset: 0, fileTransferActive: false,
    disposables: [], imeDispose: null, activeClient: 'desktop', syncing: isWebMode() && !options.sftp, pending: [], pendingBytes: 0, generation: 0, closed: false, eventBytes: 0, eventCount: 0, protocolTail: '',
  }
  const managed = m
  terminals.set(id, managed)
  if (managed.syncing) terminalSharingErrors.set(id, webDisconnected ? offlineMessage : syncingMessage)
  managed.disposables.push(installTerminalInputRouter(terminal, data => { if (!managed.closed && !managed.stream.error) queuedSessionReply(managed.sessionId, data) }))
  managed.disposables.push(terminal.onBinary(data => queuedSessionWrite(managed.sessionId, data, true)))
  trackTrims(managed)
  managed.disposables.push(terminal.onResize(() => {
    const buf = terminal.buffer.normal
    realignRegistry(managed.lineRegistry, { lineOffset: managed.lineOffset, cursorAbs: managed.lineOffset + buf.baseY + buf.cursorY, source: bufferRowSource(buf) })
  }))
  managed.disposables.push(progressAddon.onChange(state => setTabProgressForSession(managed.sessionId, state.state === 0 ? null : state)))
  setSizeProvider(managed)
  terminal.open(getHolding(id))
  installBroker(managed)
  return terminal
}

function trackTrims(m: ManagedTerminal): void {
  m.trimDispose?.dispose()
  const lines = (m.terminal as any)._core._bufferService.buffers.normal.lines
  m.trimDispose = lines.onTrim((n: number) => { m.lineOffset += n })
}

function setSizeProvider(m: ManagedTerminal): void {
  setSessionInputSizeProvider(m.sessionId, () => {
    if (isWebMode() && !m.options.sftp && (webDisconnected || m.syncing || m.stream.error || m.closed)) {
      throw new Error(terminalSharingErrors.get(m.sessionId) ?? 'Terminal view is not ready; input was not sent. Refresh the view and retry.')
    }
    const rect = m.container?.getBoundingClientRect()
    const size = rect?.width && rect.height ? m.fitAddon.proposeDimensions() ?? getTerminalSize(m.sessionId) : getTerminalSize(m.sessionId)
    return { ...size, generation: m.generation }
  }, !m.options.sftp)
}

function ensureAuthority(session: Partial<WebSession> & { id: string }): ManagedTerminal {
  let m = terminals.get(session.id)
  if (!m) {
    const ts = useSettingsStore().settings.terminal
    acquireTerminal(session.id, '', { fontSize: ts.fontSize, fontFamily: ts.fontFamily, fallbackFont: ts.fallbackFont, fontWeight: ts.fontWeight, themeName: ts.theme, scrollback: ts.maxHistoryLines }, useSettingsStore().settings.customTerminalThemes)
    m = terminals.get(session.id)!
    if (session.cols && session.rows) m.terminal.resize(session.cols, session.rows)
  }
  m.session = { ...m.session, ...session }
  installBroker(m)
  return m
}

function installBroker(m: ManagedTerminal): void {
  if (isWebMode() || !brokerFactory || m.outputBroker || m.options.sftp || m.sessionId.startsWith('pending:')) return
  const broker = brokerFactory(m.session ?? { id: m.sessionId })
  if (broker) setTerminalOutputBroker(m.sessionId, broker.consume, broker.dispose)
}

async function consume(m: ManagedTerminal, event: TerminalEvent): Promise<void> {
  if (event.webSequence && event.webSequence <= m.stream.sequence) return
  if (typeof event.data === 'string') {
    // Keep the broker and parse in the same serial queue. output() itself must
    // be queued synchronously, so never await outside the stream ordering lane.
    let data = event.data
    let text = ''
    if (event.binary) {
      m.protocolTail = ''
      m.fileTransferActive = true
    } else if (!m.options.sftp && !m.fileTransferActive) {
      const framed = splitZmodemHeader(m.protocolTail + data)
      m.protocolTail = framed.pending
      data = framed.text
      if (framed.protocol) {
        m.fileTransferActive = true
        text = data
        data = framed.protocol
        if (isWebMode()) {
          await m.stream.output(text, event.webSequence || 0)
          pauseBrowserTerminal(m, transferMessage)
          return
        }
      }
    }
    if (m.outputBroker && !isWebMode()) {
      data = await m.outputBroker(data, event.binary) ?? ''
    } else if (event.binary || m.fileTransferActive) {
      throw new Error('ZMODEM authority broker was not installed before binary output')
    }
    const before = currentAbsoluteLine(m.sessionId)
    await m.stream.output(text + data, event.webSequence || 0)
    recordWrite(m.sessionId, before, event.webTimestamp ?? Date.now())
  } else if (event.cols && event.rows) {
    await m.stream.resize(event.cols, event.rows, event.webSequence)
    m.activeClient = event.activeClient ?? m.activeClient
  } else if (event.operation) {
    await m.stream.run(async () => {
      if (event.webSequence && event.webSequence <= m.stream.sequence) return
      const op = event.operation!
      if (op.kind === 'write') await m.stream.writeRaw(op.data)
      else if (op.kind === 'stamp') stampCommandLine(m.sessionId, op.timestamp)
      else if (op.kind === 'configure') {
        m.stream.options.cursorBlink = op.cursorBlink
        m.terminal.options.scrollback = op.scrollback
        m.options.scrollback = op.scrollback
      }
      else {
        const scrollback = m.terminal.options.scrollback
        m.terminal.options.scrollback = 0
        m.terminal.clear()
        m.terminal.options.scrollback = scrollback
        if (op.kind === 'reset') await m.stream.writeRaw('\r\x1b[H\x1b[2J')
        m.lineRegistry = createLineRegistry()
      }
      m.stream.sequence = Math.max(m.stream.sequence, event.webSequence || 0)
    })
  } else {
    await m.stream.run(() => { m.stream.sequence = Math.max(m.stream.sequence, event.webSequence || 0) })
    if (event.closed) {
      m.closed = true
      if (!m.refs.size) disposeTerminal(m.sessionId)
    }
  }
}

// A single event lane per terminal also orders an asynchronous file broker.
const eventTails = new Map<string, Promise<void>>()
function deliver(m: ManagedTerminal, event: TerminalEvent): void {
  const bytes = event.data?.length ?? 128
  if (m.eventBytes + bytes > 16 * 1024 * 1024 || m.eventCount >= 8192) {
    m.stream.error = new Error('Terminal event queue overflow')
    if (isWebMode()) void resnapshotTerminal(m).catch(() => {})
    else failed(m.sessionId, 'Authority output overflow; exact restoration is unavailable')
    return
  }
  m.eventBytes += bytes
  m.eventCount++
  const generation = m.generation
  const task = (eventTails.get(m.sessionId) ?? Promise.resolve()).then(() => {
    if (!isWebMode() || generation === m.generation) return consume(m, event)
  }).finally(() => { m.eventBytes -= bytes; m.eventCount-- })
  eventTails.set(m.sessionId, task.catch(error => {
    if (terminals.get(m.sessionId) === m && (!isWebMode() || generation === m.generation)) failed(m.sessionId, error)
  }))
}

function receive(event: TerminalEvent): void {
  const m = terminals.get(event.id) ?? (!isWebMode() && typeof event.data === 'string' ? ensureAuthority({ id: event.id }) : undefined)
  if (!m) return
  if (isWebMode() && webDisconnected && !m.options.sftp) return
  if (isWebMode() && event.binary) {
    if (!m.fileTransferActive) {
      m.fileTransferActive = true
      pauseBrowserTerminal(m, transferMessage)
    }
    return
  }
  if (isWebMode() && event.closed && m.fileTransferActive) {
    m.closed = true
    pauseBrowserTerminal(m, 'Terminal closed during file transfer.')
    return
  }
  // During transfer recovery only the authority snapshot can restore text;
  // observers never feed binary payloads to xterm or a second sentry.
  if (isWebMode() && m.fileTransferActive) return
  if (m.syncing) {
    m.pendingBytes += event.data?.length ?? 128
    if (m.pendingBytes > 16 * 1024 * 1024 || m.pending.length >= 8192) {
      failed(event.id, 'Attachment queue overflow; reconnect to request a fresh snapshot')
      return
    }
    m.pending.push(event)
  } else deliver(m, event)
}

/** Install before FrontendReady/SessionStart. The shell forwards Wails SSE
 * events normally and calls resyncWebTerminals after EVERY SSE ready frame. */
export function initWebTerminals(): () => void {
  if (listeners.length) return disposeWebTerminals
  const onInputError = (event: Event) => {
    const { id, error } = (event as CustomEvent<{ id: string; error: string }>).detail
    inputFailed(id, error)
  }
  window.addEventListener('terminal:sharing-error', onInputError)
  listeners = [
    () => window.removeEventListener('terminal:sharing-error', onInputError),
    Events.On('web:disconnected', () => {
      if (!isWebMode()) return
      webDisconnected = true
      for (const m of terminals.values()) if (!m.options.sftp) pauseBrowserTerminal(m, offlineMessage)
    }),
    ...['session:data', 'session:resize', 'session:presentation', 'session:status'].map(name => Events.On(name, ev => receive(ev.data))),
    Events.On('session:binary', ev => receive({ ...ev.data, binary: true })),
    Events.On('web:zmodem-restored', ev => {
      const m = terminals.get(ev.data.sessionId)
      if (isWebMode() && m) {
        m.fileTransferActive = false
        void resnapshotTerminal(m).catch(() => {})
      }
    }),
    Events.On('session:closed', ev => {
      const id = ev.data.id
      useSessionStore().updateStatus(id, 'disconnected')
      // Let already-delivered output finish before closing the authority.
      receive({ ...ev.data, closed: true })
    }),
    Events.On('session:gap', ev => {
      const m = terminals.get(ev.data.id)
      if (m) m.stream.error = new Error(`Session output gap: ${ev.data.reason}`)
      failed(ev.data.id, 'Session output was lost upstream; exact restoration is unavailable')
    }),
    Events.On('web:terminal-attach', ev => {
      if (isWebMode()) return
      const request = ev.data as SnapshotRequest
      ensureAuthority(request.session)
      void webTerminalBridge.acknowledge(request.requestId).catch(error => failed(request.sessionId, error))
    }),
    Events.On('web:session-created', ev => {
      if (!isWebMode() && textTypes.has(ev.data.type)) ensureAuthority(ev.data)
    }),
    Events.On('web:snapshot-request', ev => {
      if (isWebMode()) return
      const request = ev.data as SnapshotRequest
      const m = terminals.get(request.sessionId)
      if (!m) { failed(request.sessionId, 'Authority was not installed before session output'); return }
      // Capture the current event lane, then enqueue the snapshot before any
      // later delivery. Do not chase a constantly moving "latest" sequence.
      const task = (eventTails.get(m.sessionId) ?? Promise.resolve()).then(() => m.stream.run(() => {
        if (m.stream.sequence < request.sequence) throw new Error('Snapshot request is ahead of parsed terminal sequence')
        return captureTerminalSnapshot(m.sessionId, m.stream, { image: m.imageAddon, progress: m.progressAddon }, {
          lineOffset: m.lineOffset, nextNumber: m.lineRegistry.nextNumber, entries: [...m.lineRegistry.entries], activeClient: m.activeClient, closed: m.closed,
          protocolTail: m.protocolTail, fileTransferActive: m.fileTransferActive,
        })
      }))
      eventTails.set(m.sessionId, task.then(() => {}, () => {}))
      void task.then(snapshot => webTerminalBridge.provide(request.requestId, snapshot), error => {
        failed(request.sessionId, error)
        return webTerminalBridge.provide(request.requestId, {
          sessionId: request.sessionId, sequence: request.sequence, cols: m.terminal.cols, rows: m.terminal.rows,
          ansi: '', state: { error: String(error) },
        })
      }).catch(error => failed(request.sessionId, error))
    }),
  ]
  return disposeWebTerminals
}

export function disposeWebTerminals(): void {
  listeners.splice(0).forEach(unsub => unsub())
  for (const id of [...terminals.keys()]) disposeTerminal(id)
  webDisconnected = false
}

export async function attachWebTerminal(session: WebSession): Promise<Terminal> {
  if (!isWebMode()) throw new Error('attachWebTerminal is browser-only')
  const m = ensureAuthority(session)
  m.activeClient = session.activeClient
  await resnapshotTerminal(m)
  return m.terminal
}

async function resnapshotTerminal(m: ManagedTerminal): Promise<void> {
  pauseBrowserTerminal(m, webDisconnected ? offlineMessage : syncingMessage)
  // Buffer live events while refreshing even if the previous view was paused
  // for a transfer. The new snapshot decides whether that transfer is active.
  m.fileTransferActive = false
  const generation = m.generation
  try {
    if (webDisconnected) throw new Error(offlineMessage)
    const snapshot = await webTerminalBridge.snapshot(m.sessionId)
    if (generation !== m.generation || terminals.get(m.sessionId) !== m) return
    if (snapshot.sessionId !== m.sessionId) throw new Error('Snapshot session mismatch')
    await (eventTails.get(m.sessionId) ?? Promise.resolve())
    if (generation !== m.generation) return
    m.stream.error = undefined
    await m.stream.run(() => {
      if (generation === m.generation) return restoreTerminalSnapshot(m.stream, snapshot, { image: m.imageAddon, progress: m.progressAddon })
    })
    if (generation !== m.generation) return
    const extra = snapshot.state.extra
    trackTrims(m)
    m.lineOffset = extra.lineOffset
    m.lineRegistry = { nextNumber: extra.nextNumber, entries: new Map(extra.entries) }
    m.activeClient = extra.activeClient
    m.closed = extra.closed ?? false
    m.protocolTail = extra.protocolTail ?? ''
    m.fileTransferActive = extra.fileTransferActive ?? false
    if (m.fileTransferActive) {
      m.pending = []
      m.pendingBytes = 0
      terminalSharingErrors.set(m.sessionId, transferMessage)
      return
    }
    // Keep input gated through catch-up, including resize/presentation events
    // and bytes that arrive while xterm is still parsing an earlier batch.
    while (m.pending.length) {
      const pending = m.pending
      m.pending = []
      m.pendingBytes = 0
      for (const event of pending) if (event.webSequence > snapshot.sequence) deliver(m, event)
      await eventTails.get(m.sessionId)
      if (generation !== m.generation) return
    }
    m.syncing = false
    terminalSharingErrors.delete(m.sessionId)
  } catch (error) {
    if (generation === m.generation && terminals.get(m.sessionId) === m) failed(m.sessionId, error)
    throw error
  }
}

export async function resyncWebTerminals(): Promise<void> {
  if (!isWebMode()) return
  webDisconnected = false
  await Promise.all([...terminals.values()].filter(m => !m.options.sftp && !m.sessionId.startsWith('pending:')).map(resnapshotTerminal))
}

export async function refreshWebTerminal(id: string): Promise<void> {
  const m = terminals.get(id)
  if (m && isWebMode()) await resnapshotTerminal(m)
}

export function setTerminalOutputBroker(id: string, broker?: ManagedTerminal['outputBroker'], dispose?: () => void): void {
  const m = terminals.get(id)
  if (m && !isWebMode()) {
    m.brokerDispose?.()
    m.outputBroker = broker
    m.brokerDispose = dispose
  }
}

/** File owner installs this at startup; factory registers the one sentry before
 * native attach ACK / first-byte delivery, even when no BaseTerminal mounts. */
export function setTerminalBrokerFactory(factory: typeof brokerFactory): void {
  brokerFactory = factory
  if (isWebMode() || !factory) return
  for (const m of terminals.values()) installBroker(m)
}

/** Called after native binary mode and trailing output handoff have settled. */
export async function finishTerminalFileTransfer(id: string): Promise<void> {
  const m = terminals.get(id)
  if (!m || isWebMode()) return
  m.fileTransferActive = false
  await eventTails.get(id)
  if (terminals.get(id) === m && !m.closed) await webTerminalBridge.fileRestored(id)
}

/** UI annotations and clear actions use the ordered backend presentation lane;
 * observers never clear their own buffer independently of the authority. */
export function presentTerminal(id: string, operation: TerminalPresentation): void {
  if (isWebMode() && (operation.kind === 'write' || operation.kind === 'configure')) return
  void enqueueSessionOperation(id, () => webTerminalBridge.present(id, operation), true).catch(error => inputFailed(id, error))
}

export function resizeTerminalView(id: string, active = false): void {
  const m = terminals.get(id)
  if (!m || m.syncing) return
  if (id.startsWith('pending:')) { m.fitAddon.fit(); return }
  const rect = m.container?.getBoundingClientRect()
  if (!rect?.width || !rect.height) return
  const size = m.fitAddon.proposeDimensions()
  if (size) void resizeSessionView(id, size.cols, size.rows, active).catch(error => inputFailed(id, error))
}

export function releaseTerminal(id: string, ref: string): void {
  const m = terminals.get(id)
  if (!m) return
  m.refs.delete(ref)
  // Desktop authority lives until backend close, even with no visible view.
  if (!m.refs.size && (isWebMode() || m.closed || id.startsWith('pending:'))) m.disposeTimer = setTimeout(() => disposeTerminal(id), 500)
}

export function disposeTerminal(id: string): void {
  const m = terminals.get(id)
  if (!m) return
  m.generation++
  if (m.disposeTimer) clearTimeout(m.disposeTimer)
  m.stream.dispose()
  m.brokerDispose?.()
  m.disposables.forEach(d => d.dispose())
  m.trimDispose?.dispose()
  m.imeDispose?.dispose()
  m.terminal.dispose()
  terminals.delete(id)
  terminalSharingErrors.delete(id)
  eventTails.delete(id)
  holding.get(id)?.remove()
  holding.delete(id)
  setSessionInputSizeProvider(id)
  setTabProgressForSession(id, null)
}

export function transferTerminal(oldId: string, newId: string): boolean {
  const m = terminals.get(oldId)
  if (!m || m.eventCount || isWebMode()) return false
  // New-session first-byte capture may already own a terminal; never overwrite
  // its parsed state with the disconnected session's old screen.
  if (terminals.has(newId)) return false
  if (m.fileTransferActive) return false
  m.brokerDispose?.()
  m.outputBroker = undefined
  m.brokerDispose = undefined
  terminals.delete(oldId)
  terminals.set(newId, m)
  const host = holding.get(oldId)
  if (host) { holding.delete(oldId); holding.set(newId, host) }
  eventTails.delete(oldId)
  terminalSharingErrors.delete(oldId)
  m.sessionId = newId
  if (m.session) m.session = { ...m.session, id: newId }
  m.stream.sequence = 0
  m.stream.frames.pending = ''
  m.protocolTail = ''
  m.stream.error = undefined
  m.closed = false
  setSessionInputSizeProvider(oldId)
  setSizeProvider(m)
  installBroker(m)
  return true
}

export function attachTerminal(id: string, container: HTMLElement): void {
  const m = terminals.get(id)
  if (!m || m.container === container) return
  m.container = container
  if (!m.terminal.element) m.terminal.open(container)
  else container.appendChild(m.terminal.element)
  m.imeDispose ??= installImeCompatibilityPatch(m.terminal)
  applyTerminalBgVar(m.terminal, m.terminal.options.theme ?? {})
  void (document.fonts?.ready ?? Promise.resolve()).then(() => requestAnimationFrame(() => resizeTerminalView(id)))
}

export function detachTerminal(id: string, container: HTMLElement): void {
  const m = terminals.get(id)
  if (!m) return
  if (m.terminal.element?.parentElement === container) getHolding(id).appendChild(m.terminal.element)
  if (m.container === container) m.container = null
}

export function getTerminal(id: string): Terminal | undefined { return terminals.get(id)?.terminal }
export function getManagedTerminal(id: string): ManagedTerminal | undefined { return terminals.get(id) }
export function markTerminalActive(id: string, ref: string, active: boolean): void {
  const m = terminals.get(id)
  if (active) m?.activeRefs.add(ref)
  else m?.activeRefs.delete(ref)
}
export function isTerminalActive(id: string): boolean { return !!terminals.get(id)?.activeRefs.size }
export function bumpOnDataGeneration(id: string): number { const m = terminals.get(id); return m ? ++m.onDataGeneration : 0 }
export function getTerminalSize(id: string): { cols: number; rows: number } {
  const t = getTerminal(id)
  return { cols: t?.cols ?? 0, rows: t?.rows ?? 0 }
}
export async function waitForTerminalSize(id: string, timeoutMs = 1500): Promise<{ cols: number; rows: number }> {
  const start = Date.now()
  while (!getTerminal(id) && Date.now() - start < timeoutMs) await new Promise(r => setTimeout(r, 30))
  const m = terminals.get(id)
  // Initial PTY dimensions can be measured without mutating the observer grid.
  return m?.fitAddon.proposeDimensions() ?? getTerminalSize(id)
}

// Compatibility for AI readers: the existing managed terminal now continuously
// parses, so these helpers no longer need a raw, divergent second emulator.
export function getMirrorPromptSnapshot(id: string): { promptLine: string; startRow: number } | null {
  const m = terminals.get(id)
  if (!m) return null
  const b = m.terminal.buffer.active
  return { promptLine: b.getLine(b.baseY + b.cursorY)?.translateToString(true).trimEnd() ?? '', startRow: m.lineOffset + b.baseY + b.cursorY }
}
export function readMirrorScreenFromRow(id: string, row: number): string | null {
  const m = terminals.get(id)
  if (!m || row < 0) return null
  const b = m.terminal.buffer.active
  const first = b.type === 'alternate' ? b.baseY : Math.max(0, row - m.lineOffset)
  let last = b.length - 1
  if (b.type !== 'alternate') while (last >= first && !b.getLine(last)?.translateToString(true).trim()) last--
  return Array.from({ length: Math.max(0, last - first + 1) }, (_, i) => b.getLine(first + i)?.translateToString(true) ?? '').join('\n')
}
export function readMirrorTail(id: string, count: number): string | null {
  const m = terminals.get(id)
  if (!m) return null
  const b = m.terminal.buffer.active
  let last = b.length - 1
  while (last >= 0 && !b.getLine(last)?.translateToString(true).trim()) last--
  const first = Math.max(0, last - count + 1)
  return Array.from({ length: Math.max(0, last - first + 1) }, (_, i) => b.getLine(first + i)?.translateToString() ?? '').join('\n')
}
