import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mock = vi.hoisted(() => ({
  events: new Map<string, Set<(event: any) => void>>(),
  call: vi.fn(async (..._args: any[]): Promise<any> => {}),
  write: vi.fn(async () => {}),
  panels: new Map(),
}))
vi.mock('@wailsio/runtime', () => ({
  Call: { ByName: mock.call },
  Events: { On(name: string, handler: (ev: any) => void) {
    if (!mock.events.has(name)) mock.events.set(name, new Set())
    mock.events.get(name)!.add(handler)
    return () => mock.events.get(name)?.delete(handler)
  } },
}))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ SessionWrite: mock.write }))
vi.mock('../stores/settingsStore', () => ({ useSettingsStore: () => ({ settings: { terminal: {}, customTerminalThemes: [] }, resolvedAppTheme: 'dark' }) }))
vi.mock('../stores/localStateStore', () => ({ useLocalStateStore: () => ({ state: {} }) }))
vi.mock('../stores/tabStore', () => ({ useTabStore: () => ({ tabs: [], setTabProgress() {} }) }))
vi.mock('../stores/panelStore', () => ({ usePanelStore: () => ({ panels: mock.panels }) }))
vi.mock('../stores/sessionStore', () => ({ useSessionStore: () => ({ updateStatus() {} }) }))
vi.mock('../composables/useTerminal', () => ({ getXtermTheme: () => ({}) }))
vi.mock('../composables/useTerminalTheme', () => ({ resolveXtermBackground: () => ({}), resolveTerminalThemeName: () => 'dark', applyTerminalBgVar() {} }))
import {
  initWebTerminals, disposeWebTerminals, getManagedTerminal, acquireTerminal,
  attachTerminal, detachTerminal, releaseTerminal, attachWebTerminal,
  resyncWebTerminals, disposeTerminal, resizeTerminalView,
  setTerminalBrokerFactory,
  transferTerminal,
} from './terminalManager'
import type { WebSession, WebTerminalSnapshot } from './webTerminalBridge'
import { queuedSessionWrite, enqueueSessionOperation } from './sessionWriter'
import '@xterm/xterm/css/xterm.css'

const session: WebSession = { id: 'session', type: 'ssh', title: 'Shared', status: 'connected', cols: 25, rows: 6, activeClient: 'desktop', resizeCapability: 'pty' }
const emit = (name: string, data: any) => mock.events.get(name)?.forEach(fn => fn({ data }))
const finish = async (sequence: number) => expect.poll(() => getManagedTerminal(session.id)?.stream.sequence).toBe(sequence)
beforeEach(() => { window.__UNITERM_WEB__ = undefined; vi.clearAllMocks(); initWebTerminals() })
afterEach(() => { disposeWebTerminals(); setTerminalBrokerFactory(undefined); document.querySelectorAll('[data-test-terminal]').forEach(el => el.remove()) })

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
    emit('session:data', { id: session.id, data: '\x1b[6n\r\nbackground', webSequence: 12 })
    await finish(12)
    await expect.poll(() => mock.write.mock.calls.length).toBe(1)
    expect(getManagedTerminal(session.id)?.terminal).toBe(t)
    expect(t.buffer.active.getLine(1)?.translateToString(true)).toContain('background')
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
})
