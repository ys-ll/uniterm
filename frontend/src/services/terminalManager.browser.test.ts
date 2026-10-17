import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mock = vi.hoisted(() => ({
  call: vi.fn(async (..._args: any[]): Promise<any> => {}),
  write: vi.fn(async () => {}),
  panels: new Map(),
  startZmodem: vi.fn(),
  clearTransfers: vi.fn(),
}))
// Exercise the same local beta.27 event dispatcher that browserRuntime uses;
// only the host RPC boundary is mocked, not Events.On or event payloads.
vi.mock('@wailsio/runtime', async importOriginal => {
  // The test server has no Wails custom script. Avoid Vite's HTML fallback,
  // which would otherwise import the full application into this focused suite.
  const fetch = globalThis.fetch
  const optionalScript = vi.spyOn(globalThis, 'fetch').mockImplementation((url, init) =>
    url === '/wails/custom.js' ? Promise.resolve(new Response(null, { status: 404 })) : fetch(url, init))
  try {
    const runtime = await importOriginal<typeof import('@wailsio/runtime')>()
    return { ...runtime, Call: { ...runtime.Call, ByName: mock.call } }
  } finally { optionalScript.mockRestore() }
})
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ SessionWrite: mock.write }))
vi.mock('../stores/settingsStore', () => ({ useSettingsStore: () => ({ settings: { terminal: {}, customTerminalThemes: [] }, resolvedAppTheme: 'dark' }) }))
vi.mock('../stores/localStateStore', () => ({ useLocalStateStore: () => ({ state: {} }) }))
vi.mock('../stores/tabStore', () => ({ useTabStore: () => ({ tabs: [], setTabProgress() {} }) }))
vi.mock('../stores/panelStore', () => ({ usePanelStore: () => ({ panels: mock.panels }) }))
vi.mock('../stores/sessionStore', () => ({ useSessionStore: () => ({ updateStatus() {} }) }))
vi.mock('../composables/useTerminal', () => ({ getXtermTheme: () => ({}) }))
vi.mock('../composables/useTerminalTheme', () => ({ resolveXtermBackground: () => ({}), resolveTerminalThemeName: () => 'dark', applyTerminalBgVar() {} }))
vi.mock('./zmodemService', () => ({ startZmodemService: mock.startZmodem }))
vi.mock('./webZmodem', () => ({ initWebZmodem() {} }))
vi.mock('../stores/zmodemStore', () => ({ useZmodemStore: () => ({ registerAbort() {}, unregisterAbort() {}, clearTransfers: mock.clearTransfers }) }))
vi.mock('./message', () => ({ msg: { error() {}, warning() {}, success() {} } }))
import {
  initWebTerminals, disposeWebTerminals, getManagedTerminal, acquireTerminal,
  attachTerminal, detachTerminal, releaseTerminal, attachWebTerminal,
  resyncWebTerminals, disposeTerminal, resizeTerminalView,
  setTerminalBrokerFactory,
  transferTerminal,
  refreshWebTerminal, terminalSharingErrors, presentTerminal,
} from './terminalManager'
import type { WebSession, WebTerminalSnapshot } from './webTerminalBridge'
import { queuedSessionWrite, enqueueSessionOperation, resizeSessionView } from './sessionWriter'
import { Events } from '@wailsio/runtime'
import '@xterm/xterm/css/xterm.css'
import { initTerminalFiles } from './terminalFileBroker'

const session: WebSession = { id: 'session', type: 'ssh', title: 'Shared', status: 'connected', cols: 25, rows: 6, activeClient: 'desktop', resizeCapability: 'pty' }
const emit = (name: string, data: any) => (window as any)._wails.dispatchWailsEvent({ name, data })
const finish = async (sequence: number) => expect.poll(() => getManagedTerminal(session.id)?.stream.sequence).toBe(sequence)
beforeEach(() => { window.__UNITERM_WEB__ = undefined; vi.resetAllMocks(); initWebTerminals() })
afterEach(() => { disposeWebTerminals(); Events.OffAll(); vi.restoreAllMocks(); setTerminalBrokerFactory(undefined); document.querySelectorAll('[data-test-terminal]').forEach(el => el.remove()) })

async function authoritySnapshot(): Promise<WebTerminalSnapshot> {
  emit('web:terminal-attach', { requestId: 'attach', sessionId: session.id, session })
  await expect.poll(() => mock.call.mock.calls.some(c => c[0] === 'main.App.AcknowledgeWebTerminal')).toBe(true)
  emit('session:data', { id: session.id, data: 'shell\x1b[?1049h\x1b[2Jvim\x1b[31;', webSequence: 10 })
  emit('web:snapshot-request', { requestId: 'snapshot', sessionId: session.id, sequence: 10, session })
  emit('session:data', { id: session.id, data: '1mRED', webSequence: 11 })
  await expect.poll(() => mock.call.mock.calls.some(c => c[0] === 'main.App.ProvideWebTerminalSnapshot')).toBe(true)
  const snap = mock.call.mock.calls.find(c => c[0] === 'main.App.ProvideWebTerminalSnapshot')![2]
  expect(snap.sequence).toBe(10)
  expect(snap.state.pending).toBe('\x1b[31;')
  await finish(11)
  return structuredClone(snap)
}

describe('managed terminal sharing lifecycle', () => {
  it.each(['complete', 'cancel', 'error'])('owns one hidden file parser and restores ordered text after %s', async outcome => {
    const options: any[] = [], services: any[] = []
    const header = '**\x18B00000000000000\r\n\x11'
    mock.startZmodem.mockImplementation(opts => {
      options.push(opts)
      const service = { start: vi.fn(), consume: vi.fn(), consumeBinary: vi.fn((data: string) => {
        opts.onTerminalData(Array.from(atob(data), c => c.charCodeAt(0)))
      }), dispose: vi.fn(async () => {}) }
      services.push(service)
      return service
    })
    initTerminalFiles()
    emit('web:session-created', session)
    expect(mock.startZmodem).toHaveBeenCalledTimes(1)
    expect(options[0].subscribeBinary).toBe(false)
    expect(getManagedTerminal(session.id)?.refs.size).toBe(0)
    emit('web:terminal-attach', { requestId: 'hidden-attach', sessionId: session.id, session })
    acquireTerminal(session.id, 'visible-1', {})
    acquireTerminal(session.id, 'visible-2', {})
    initTerminalFiles()
    expect(mock.startZmodem).toHaveBeenCalledTimes(1)
    emit('session:binary', { id: session.id, data: btoa('ordinary\r\n' + header), webSequence: 1 })
    await finish(1)
    const m = getManagedTerminal(session.id)!
    expect(m.fileTransferActive).toBe(true)
    expect(m.terminal.buffer.active.getLine(0)?.translateToString(true)).toBe('ordinary')
    expect(m.terminal.buffer.active.getLine(1)?.translateToString(true)).toBe('')
    expect(services[0].consumeBinary).toHaveBeenCalledTimes(1)
    emit('web:snapshot-request', { requestId: 'during-transfer', sessionId: session.id, sequence: 1, session })
    await expect.poll(() => mock.call.mock.calls.find(c => c[1] === 'during-transfer')?.[2].state.extra.fileTransferActive).toBe(true)

    options[0].onTerminalRestoreState(true)
    emit('session:data', { id: session.id, data: 'trailing$ ', webSequence: 2 })
    await expect.poll(() => m.eventCount).toBe(1)
    // Let the broker actually enter its deferred-text wait.
    await new Promise(resolve => setTimeout(resolve, 20))
    if (outcome === 'error') options[0].onError('disk full')
    else options[0].onComplete(outcome === 'complete' ? ['file.bin'] : [])
    expect(mock.call.mock.calls.some(c => c[0] === 'main.App.WebZmodemRestored')).toBe(false)
    options[0].onSettled()
    await expect.poll(() => mock.call.mock.calls.some(c => c[0] === 'main.App.WebZmodemRestored')).toBe(true)
    expect(m.stream.sequence).toBe(2)
    expect(m.terminal.buffer.active.getLine(1)?.translateToString(true)).toBe('trailing$ ')
    expect(m.fileTransferActive).toBe(false)
    expect(services[0].dispose).toHaveBeenCalledTimes(1)
    expect(mock.startZmodem).toHaveBeenCalledTimes(2) // one idle replacement
    releaseTerminal(session.id, 'visible-1')
    releaseTerminal(session.id, 'visible-2')
    expect(services[1].dispose).not.toHaveBeenCalled()
    emit('session:closed', { id: session.id, webSequence: 3 })
    await expect.poll(() => getManagedTerminal(session.id)).toBeUndefined()
    expect(services[1].dispose).toHaveBeenCalledTimes(1)
  })

  it('pauses observers before raw headers and automatically resnapshots after native handoff, including mid-transfer attach', async () => {
    const snapshot = await authoritySnapshot()
    const completed = structuredClone(snapshot)
    completed.sequence = 20
    completed.state.extra.fileTransferActive = false
    disposeWebTerminals()
    window.__UNITERM_WEB__ = { instanceId: 'test-host', clientId: 'observer' }
    initWebTerminals()
    mock.call.mockImplementation(async name => name === 'main.App.GetWebTerminalSnapshot' ? snapshot : undefined)
    await attachWebTerminal(session)
    const m = getManagedTerminal(session.id)!
    const output = vi.spyOn(m.stream, 'output')
    emit('session:data', { id: session.id, data: '**\x18B000000', webSequence: 11 })
    await finish(11)
    emit('session:data', { id: session.id, data: '00000000\r\n', webSequence: 12 })
    await expect.poll(() => m.fileTransferActive).toBe(true)
    expect(m.syncing).toBe(true)
    expect(output.mock.calls.every(call => !call[0].includes('**') && !call[0].includes('\x18'))).toBe(true)
    emit('session:binary', { id: session.id, data: btoa('binary protocol'), webSequence: 13 })
    emit('session:data', { id: session.id, data: 'prompt to be restored', webSequence: 20 })
    expect(mock.startZmodem).not.toHaveBeenCalled()

    // A manual refresh or a new browser attached during the transfer must stay
    // gated, even if its snapshot is otherwise valid and has no pending bytes.
    const active = structuredClone(snapshot)
    active.sequence = 13
    active.state.extra.fileTransferActive = true
    mock.call.mockImplementation(async name => name === 'main.App.GetWebTerminalSnapshot' ? active : undefined)
    await refreshWebTerminal(session.id)
    expect(m.syncing).toBe(true)
    expect(terminalSharingErrors.get(session.id)).toContain('ZMODEM')
    mock.call.mockImplementation(async name => name === 'main.App.GetWebTerminalSnapshot' ? completed : undefined)
    emit('web:zmodem-restored', { sessionId: session.id })
    await expect.poll(() => m.syncing).toBe(false)
    expect(m.stream.sequence).toBe(20)
    expect(terminalSharingErrors.has(session.id)).toBe(false)
    emit('session:data', { id: session.id, data: '1mLIVE', webSequence: 21 })
    await finish(21)
    expect(m.terminal.buffer.active.getLine(0)?.translateToString(true)).toBe('     vimLIVE')
    expect(mock.call.mock.calls.some(c => /CloseSession|SessionStart/.test(c[0]))).toBe(false)
  })

  it('transfers a quiet desktop view on reconnect without discarding a new session authority', async () => {
    await authoritySnapshot()
    const previous = acquireTerminal(session.id, 'view', {})
    expect(transferTerminal(session.id, 'reconnected')).toBe(true)
    expect(getManagedTerminal(session.id)).toBeUndefined()
    expect(getManagedTerminal('reconnected')?.terminal).toBe(previous)
    emit('session:data', { id: 'reconnected', data: '\r\nnew connection', webSequence: 20 })
    await expect.poll(() => getManagedTerminal('reconnected')?.stream.sequence).toBe(20)
    emit('web:session-created', { ...session, id: 'already-started' })
    expect(transferTerminal('reconnected', 'already-started')).toBe(false)
    expect(getManagedTerminal('already-started')?.terminal).not.toBe(previous)
  })

  it('ACKs authority before first bytes; hidden/released views keep parsing and replying once', async () => {
    const brokerDisposed = vi.fn()
    setTerminalBrokerFactory(() => ({ consume: data => data, dispose: brokerDisposed }))
    await authoritySnapshot()
    expect(getManagedTerminal(session.id)?.outputBroker).toBeDefined()
    const t = acquireTerminal(session.id, 'view', {})
    const host = document.createElement('div')
    host.dataset.testTerminal = ''
    host.style.cssText = 'width:500px;height:300px'
    document.body.appendChild(host)
    attachTerminal(session.id, host)
    detachTerminal(session.id, host)
    releaseTerminal(session.id, 'view')
    emit('web:disconnected', null)
    expect(getManagedTerminal(session.id)?.syncing).toBe(false)
    queuedSessionWrite(session.id, 'desktop remains live')
    emit('session:data', { id: session.id, data: '\x1b[6n\r\nbackground', webSequence: 12 })
    await finish(12)
    await expect.poll(() => mock.write.mock.calls.length).toBe(1)
    expect(getManagedTerminal(session.id)?.terminal).toBe(t)
    expect(t.buffer.active.getLine(1)?.translateToString(true)).toContain('background')
    expect(mock.call.mock.calls.some(c => c[0] === 'main.App.WebTerminalInput' && c[3] === 'desktop remains live')).toBe(true)
    emit('session:closed', { id: session.id, webSequence: 13 })
    await expect.poll(() => getManagedTerminal(session.id)).toBeUndefined()
    expect(brokerDisposed).toHaveBeenCalledTimes(1)
  })

  it('buffers snapshot/live handoff, shares canonical resize, and rejects stale resnapshot after detach', async () => {
    const snap = await authoritySnapshot()
    disposeWebTerminals()
    window.__UNITERM_WEB__ = { instanceId: 'test-host', clientId: 'test-browser' }
    initWebTerminals()
    let resolve!: (snap: WebTerminalSnapshot) => void
    mock.call.mockImplementation(async name => {
      if (name === 'main.App.GetWebTerminalSnapshot') return new Promise(r => { resolve = r })
    })
    const attaching = attachWebTerminal(session)
    emit('session:data', { id: session.id, data: 'old frame', webSequence: 9 })
    emit('session:data', { id: session.id, data: '1mRED', webSequence: 11 })
    emit('session:resize', { id: session.id, cols: 36, rows: 7, activeClient: 'other-browser', webSequence: 12 })
    resolve(snap)
    const t = await attaching
    await finish(12)
    expect(t.cols).toBe(36)
    expect(t.buffer.active.getLine(0)?.translateToString(true)).toBe('     vimRED')
    expect(getManagedTerminal(session.id)?.activeClient).toBe('other-browser')
    const host = document.createElement('div')
    host.dataset.testTerminal = ''
    host.style.cssText = 'width:800px;height:400px'
    document.body.appendChild(host)
    attachTerminal(session.id, host)
    resizeTerminalView(session.id)
    // Merely fitting/measuring an observer never mutates canonical dimensions.
    expect(t.cols).toBe(36)
    queuedSessionWrite(session.id, 'x')
    await enqueueSessionOperation(session.id, async () => {})
    expect(mock.call.mock.calls.some(c => c[0] === 'main.App.WebTerminalInput')).toBe(true)
    const refreshing = resyncWebTerminals()
    disposeTerminal(session.id)
    resolve(snap)
    await refreshing
    expect(getManagedTerminal(session.id)).toBeUndefined()
    expect(mock.call.mock.calls.some(c => /CloseSession|SessionStart|UnregisterSession|RegisterSessionForPanel/.test(c[0]))).toBe(false)
  })

  it('keeps a new browser session blocked until snapshot and buffered resize/output finish parsing', async () => {
    const snap = await authoritySnapshot()
    disposeWebTerminals()
    window.__UNITERM_WEB__ = { instanceId: 'test-host', clientId: 'test-browser' }
    initWebTerminals()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    let resolve!: (snapshot: WebTerminalSnapshot) => void
    mock.call.mockImplementation(async name => {
      if (name === 'main.App.GetWebTerminalSnapshot') return new Promise(r => { resolve = r })
    })
    const attaching = attachWebTerminal(session)
    const m = getManagedTerminal(session.id)!
    let catchup!: () => void
    const writeRaw = m.stream.writeRaw.bind(m.stream)
    vi.spyOn(m.stream, 'writeRaw').mockImplementation(async data => {
      if (data.includes('RED')) await new Promise<void>(r => { catchup = r })
      await writeRaw(data)
    })
    queuedSessionWrite(session.id, 'before snapshot')
    presentTerminal(session.id, { kind: 'clear' })
    await expect(resizeSessionView(session.id, 90, 30, true)).rejects.toThrow('paused')
    emit('session:data', { id: session.id, data: '1mRED', webSequence: 11 })
    resolve(snap)
    await expect.poll(() => typeof catchup).toBe('function')
    expect(m.syncing).toBe(true)
    expect(terminalSharingErrors.get(session.id)).toContain('Synchronizing')
    queuedSessionWrite(session.id, 'during catch-up', true)
    // This arrives during parsing, so it needs a second catch-up batch.
    emit('session:resize', { id: session.id, cols: 36, rows: 7, activeClient: 'peer', webSequence: 12 })
    await enqueueSessionOperation(session.id, async () => {})
    expect(mock.call.mock.calls.some(c => /WebTerminalInput|ResizeWebTerminal|PublishWebTerminalPresentation/.test(c[0]))).toBe(false)
    catchup()
    const terminal = await attaching
    expect(m.syncing).toBe(false)
    expect(terminalSharingErrors.has(session.id)).toBe(false)
    expect(terminal.buffer.active.getLine(0)?.translateToString(true)).toBe('     vimRED')
    queuedSessionWrite(session.id, 'ready')
    await enqueueSessionOperation(session.id, async () => {})
    expect(mock.call).toHaveBeenCalledWith('main.App.WebTerminalInput', session.id, 'test-browser', 'ready', 36, 7, false)
  })

  it('pauses on real local disconnect events, invalidates queued output/snapshots and resumes only after shell resync', async () => {
    const snap = await authoritySnapshot()
    disposeWebTerminals()
    window.__UNITERM_WEB__ = { instanceId: 'test-host', clientId: 'test-browser' }
    initWebTerminals()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mock.call.mockImplementation(async name => name === 'main.App.GetWebTerminalSnapshot' ? snap : undefined)
    await attachWebTerminal(session)
    const m = getManagedTerminal(session.id)!
    const callsBefore = mock.call.mock.calls.length
    emit('session:data', { id: session.id, data: 'queued before disconnect', webSequence: 11 })
    emit('web:disconnected', null)
    expect(m.syncing).toBe(true)
    expect(terminalSharingErrors.get(session.id)).toContain('interrupted')
    emit('session:data', { id: session.id, data: 'offline frame', webSequence: 12 })
    queuedSessionWrite(session.id, 'offline input')
    await expect(resizeSessionView(session.id, 90, 30, true)).rejects.toThrow('interrupted')
    await expect(refreshWebTerminal(session.id)).rejects.toThrow('interrupted')
    await expect.poll(() => m.eventCount).toBe(0)
    expect(m.stream.sequence).toBe(10)
    expect(m.terminal.buffer.active.getLine(0)?.translateToString(true)).toBe('     vim')
    expect(mock.call.mock.calls.length).toBe(callsBefore)

    let resolve!: (snapshot: WebTerminalSnapshot) => void
    mock.call.mockImplementation(async name => {
      if (name === 'main.App.GetWebTerminalSnapshot') return new Promise(r => { resolve = r })
    })
    let resync!: Promise<void>
    // The application shell owns this hook; the manager does not subscribe a
    // second time or open another event stream.
    Events.On('web:resync', () => { resync = resyncWebTerminals() })
    emit('web:resync', { reason: 'reconnect' })
    emit('web:disconnected', null)
    resolve({ ...snap, sequence: 100 })
    await resync
    expect(m.syncing).toBe(true)
    expect(m.stream.sequence).toBe(10)
    expect(terminalSharingErrors.get(session.id)).toContain('interrupted')

    emit('web:resync', { reason: 'reconnect' })
    emit('session:data', { id: session.id, data: '1mRED', webSequence: 11 })
    resolve(snap)
    await resync
    expect(m.syncing).toBe(false)
    expect(m.stream.sequence).toBe(11)
    expect(terminalSharingErrors.has(session.id)).toBe(false)
    queuedSessionWrite(session.id, 'resumed')
    await enqueueSessionOperation(session.id, async () => {})
    expect(mock.call).toHaveBeenCalledWith('main.App.WebTerminalInput', session.id, 'test-browser', 'resumed', 25, 6, false)
  })

  it('keeps invalid snapshots and input transport failures visibly blocked until refresh succeeds', async () => {
    const snap = await authoritySnapshot()
    disposeWebTerminals()
    window.__UNITERM_WEB__ = { instanceId: 'test-host', clientId: 'test-browser' }
    initWebTerminals()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mock.call.mockImplementation(async name => name === 'main.App.GetWebTerminalSnapshot' ? { ...snap, state: { error: 'Host snapshot unavailable' } } : undefined)
    await expect(attachWebTerminal(session)).rejects.toThrow('Host snapshot unavailable')
    const m = getManagedTerminal(session.id)!
    expect(m.syncing).toBe(true)
    expect(terminalSharingErrors.get(session.id)).toContain('Host snapshot unavailable')
    queuedSessionWrite(session.id, 'invalid snapshot input')
    emit('session:data', { id: session.id, data: 'stale bytes', webSequence: 11 })
    await enqueueSessionOperation(session.id, async () => {})
    expect(m.stream.sequence).toBe(0)
    expect(mock.call.mock.calls.some(c => c[0] === 'main.App.WebTerminalInput')).toBe(false)

    mock.call.mockImplementation(async name => {
      if (name === 'main.App.GetWebTerminalSnapshot') return snap
      if (name === 'main.App.WebTerminalInput') throw new Error('Input transport unavailable')
    })
    await refreshWebTerminal(session.id)
    expect(m.syncing).toBe(false)
    expect(m.stream.sequence).toBe(10)
    queuedSessionWrite(session.id, 'failed RPC')
    queuedSessionWrite(session.id, 'queued behind failed RPC')
    await enqueueSessionOperation(session.id, async () => {})
    await expect.poll(() => terminalSharingErrors.get(session.id)).toContain('Input transport unavailable')
    expect(m.syncing).toBe(true)
    expect(mock.call.mock.calls.filter(c => c[0] === 'main.App.WebTerminalInput')).toHaveLength(1)
    mock.call.mockImplementation(async name => name === 'main.App.GetWebTerminalSnapshot' ? snap : undefined)
    await refreshWebTerminal(session.id)
    expect(m.syncing).toBe(false)
    expect(terminalSharingErrors.has(session.id)).toBe(false)
    queuedSessionWrite(session.id, 'recovered')
    await enqueueSessionOperation(session.id, async () => {})
    expect(mock.call).toHaveBeenCalledWith('main.App.WebTerminalInput', session.id, 'test-browser', 'recovered', 25, 6, false)
  })
})
