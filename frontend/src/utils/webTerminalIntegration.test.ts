import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mock = vi.hoisted(() => ({
  events: new Map<string, Set<(event: any) => void>>(),
  call: vi.fn(),
  ready: vi.fn(async () => {}),
  dispose: vi.fn(),
}))
vi.mock('@wailsio/runtime', () => ({
  Call: { ByName: mock.call },
  Browser: { OpenURL: vi.fn() },
  Events: { On(name: string, handler: (event: any) => void) {
    if (!mock.events.has(name)) mock.events.set(name, new Set())
    mock.events.get(name)!.add(handler)
    return () => mock.events.get(name)?.delete(handler)
  } },
}))
vi.mock('./browserRuntime', () => ({ waitForBrowserEvents: mock.ready }))
vi.mock('../services/terminalManager', () => ({
  initWebTerminals: vi.fn(() => mock.dispose),
  attachWebTerminal: vi.fn(async () => {}),
  resyncWebTerminals: vi.fn(async () => {}),
  disposeTerminal: vi.fn(),
  disposeWebTerminals: mock.dispose,
  refreshWebTerminal: vi.fn(),
  terminalSharingErrors: new Map(),
  waitForTerminalSize: vi.fn(async () => ({ cols: 120, rows: 36 })),
}))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  CreateSession: vi.fn(), SessionStart: vi.fn(), CloseSession: vi.fn(async () => {}),
  DuplicateSSHChannel: vi.fn(), K8sExecSession: vi.fn(), ContainerExecSession: vi.fn(),
  DisableSessionOutputLog: vi.fn(async () => {}), RegisterSessionForPanel: vi.fn(async () => {}), UnregisterSession: vi.fn(async () => {}),
}))

import { attachBrowserTerminal, initBrowserTerminalIntegration, restoreBrowserTerminalViews, webTerminalError } from './webTerminalIntegration'
import { detachBrowserPanel, terminateBrowserSession, type WebSession } from '../services/webTerminals'
import { attachWebTerminal, disposeTerminal, initWebTerminals, resyncWebTerminals } from '../services/terminalManager'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useTabStore } from '../stores/tabStore'
import { useDuplicateSession } from '../composables/useDuplicateSession'
import { CreateSession, SessionStart, CloseSession, RegisterSessionForPanel, UnregisterSession, DisableSessionOutputLog, ContainerExecSession } from '../../bindings/github.com/ys-ll/uniterm/app'

const session: WebSession = { id: 'host-session', type: 'ssh', title: 'Host shell', status: 'connected', cols: 93, rows: 27, activeClient: 'other-browser', resizeCapability: 'pty', connectionId: 'saved-config', host: 'host.example', remoteOS: 'linux' }
const emit = (name: string, data: unknown) => mock.events.get(name)?.forEach(fn => fn({ data }))
let dispose: (() => void) | undefined
const storageKey = 'uniterm:web:host:terminals'
function storage() {
  const values = new Map<string, string>()
  return {
    getItem: vi.fn((key: string) => values.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => { values.set(key, value) }),
  }
}
let tabStorage: ReturnType<typeof storage>
const savedViews = () => JSON.parse(tabStorage.getItem(storageKey) || 'null')
function clearViews() {
  usePanelStore().panels.clear()
  useSessionStore().sessions.clear()
  useTabStore().tabs.splice(0)
}

beforeEach(() => {
  vi.stubGlobal('window', Object.assign(new EventTarget(), { __UNITERM_WEB__: { instanceId: 'host', clientId: 'visitor' } }))
  tabStorage = storage()
  vi.stubGlobal('sessionStorage', tabStorage)
  setActivePinia(createPinia())
  clearViews()
  vi.clearAllMocks()
  mock.ready.mockResolvedValue()
  mock.call.mockImplementation(async name => name === 'main.App.GetWebSessions' ? [session] : undefined)
  webTerminalError.value = ''
})
afterEach(() => { dispose?.(); dispose = undefined; vi.unstubAllGlobals() })

describe('shell integration with the real terminal attachment service', () => {
  it('attaches an existing descriptor once without creating/starting a session and only detaches its view', async () => {
    dispose = initBrowserTerminalIntegration()
    const panelId = await attachBrowserTerminal(session)
    expect(await attachBrowserTerminal(session)).toBe(panelId)
    await Promise.resolve()
    expect(attachWebTerminal).toHaveBeenCalledExactlyOnceWith(session)
    expect(usePanelStore().getPanel(panelId)?.sessionId).toBe(session.id)
    expect(useSessionStore().sessions.get(session.id)?.web).toEqual(session)
    const tabId = usePanelStore().getPanel(panelId)!.tabId
    expect(detachBrowserPanel(panelId)).toBe(true)
    useTabStore().closeTab(tabId)
    expect(usePanelStore().getPanel(panelId)).toBeUndefined()
    expect(useTabStore().tabs).toHaveLength(0)
    expect(disposeTerminal).toHaveBeenCalledExactlyOnceWith(session.id)
    for (const call of [CreateSession, SessionStart, CloseSession, RegisterSessionForPanel, UnregisterSession, DisableSessionOutputLog]) expect(call).not.toHaveBeenCalled()
  })

  it('starts a user-created duplicate once, then attaches using canonical metadata and the runtime ID', async () => {
    dispose = initBrowserTerminalIntegration()
    const panels = usePanelStore()
    const original = panels.createPanel({ id: 'saved-config', type: 'ssh', host: 'host.example' } as any, 'ssh')
    panels.bindAttachedSession(original.id, 'source-session')
    vi.mocked(CreateSession).mockResolvedValue({ id: session.id } as any)
    vi.mocked(SessionStart).mockImplementation(async (id: string) => { emit('session:status', { id, status: 'connected' }) })
    await useDuplicateSession().duplicateSession({ type: 'terminal', panelId: original.id })
    await vi.waitFor(() => expect(attachWebTerminal).toHaveBeenCalledExactlyOnceWith(session))
    const duplicate = [...panels.panels.values()].find(p => p.sessionId === session.id)!
    expect(panels.isAttachedPanel(duplicate.id)).toBe(true)
    expect(CreateSession).toHaveBeenCalledTimes(1)
    expect(SessionStart).toHaveBeenCalledExactlyOnceWith(session.id, expect.objectContaining({ initialCols: 120, initialRows: 36 }))
    expect(mock.call).toHaveBeenCalledExactlyOnceWith('main.App.GetWebSessions')
    expect(RegisterSessionForPanel).not.toHaveBeenCalled()
    expect(CloseSession).not.toHaveBeenCalled()
  })

  it('attaches exec sessions whose connected event precedes panel binding, without SessionStart', async () => {
    const exec = { ...session, type: 'container-exec' as const }
    mock.call.mockResolvedValue([exec])
    dispose = initBrowserTerminalIntegration()
    const panels = usePanelStore()
    const original = panels.createPanel({ type: 'container-exec', containerExecConnId: 'runtime-handle', containerExecContainerId: 'container' } as any, 'container-exec')
    vi.mocked(ContainerExecSession).mockImplementation(async () => {
      emit('session:status', { id: session.id, status: 'connected' })
      return { id: session.id } as any
    })
    await useDuplicateSession().duplicateSession({ type: 'terminal', panelId: original.id })
    await vi.waitFor(() => expect(attachWebTerminal).toHaveBeenCalledExactlyOnceWith(exec))
    expect(ContainerExecSession).toHaveBeenCalledExactlyOnceWith('runtime-handle', 'container', 'sh')
    expect(CreateSession).not.toHaveBeenCalled()
    expect(SessionStart).not.toHaveBeenCalled()
  })

  it('tracks a retry/reconnect rebind and does not attach the saved connection ID', async () => {
    dispose = initBrowserTerminalIntegration()
    const panelId = await attachBrowserTerminal(session)
    const replacement = { ...session, id: 'new-runtime-id', cols: 71, rows: 19 }
    mock.call.mockResolvedValue([replacement])
    usePanelStore().bindSession(panelId, replacement.id)
    useSessionStore().initSession(replacement.id)
    expect(attachWebTerminal).toHaveBeenCalledTimes(1)
    emit('session:status', { id: replacement.id, status: 'connected' })
    await vi.waitFor(() => expect(attachWebTerminal).toHaveBeenLastCalledWith(replacement))
    expect(attachWebTerminal).toHaveBeenCalledTimes(2)
    expect(usePanelStore().isAttachedPanel(panelId)).toBe(true)
    expect(SessionStart).not.toHaveBeenCalled()
  })

  it('attaches a connecting SSH prompt before authentication and never starts a second connection', async () => {
    const connecting = { ...session, status: 'connecting' as const }
    mock.call.mockResolvedValue([connecting])
    dispose = initBrowserTerminalIntegration()
    const panel = usePanelStore().createPanel(null, 'ssh')
    usePanelStore().bindSession(panel.id, session.id)
    useSessionStore().initSession(session.id)
    await Promise.resolve()
    expect(attachWebTerminal).not.toHaveBeenCalled()
    expect(mock.call).not.toHaveBeenCalled()
    emit('session:status', { id: session.id, status: 'connecting' })
    await vi.waitFor(() => expect(attachWebTerminal).toHaveBeenCalledExactlyOnceWith(connecting))
    emit('session:status', { id: session.id, status: 'connected' })
    await Promise.resolve()
    expect(attachWebTerminal).toHaveBeenCalledTimes(1)
    expect(SessionStart).not.toHaveBeenCalled()
    expect(CreateSession).not.toHaveBeenCalled()
  })

  it('attaches when the host connecting event precedes binding the browser panel', async () => {
    const connecting = { ...session, status: 'connecting' as const }
    mock.call.mockResolvedValue([connecting])
    dispose = initBrowserTerminalIntegration()
    emit('session:status', { id: session.id, status: 'connecting' })
    const panel = usePanelStore().createPanel(null, 'ssh')
    usePanelStore().bindSession(panel.id, session.id)
    useSessionStore().initSession(session.id)
    await vi.waitFor(() => expect(attachWebTerminal).toHaveBeenCalledExactlyOnceWith(connecting))
    expect(SessionStart).not.toHaveBeenCalled()
  })

  it('never resurrects a panel closed while canonical metadata is loading', async () => {
    let finish!: (sessions: WebSession[]) => void
    mock.call.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    dispose = initBrowserTerminalIntegration()
    const panels = usePanelStore()
    const panel = panels.createPanel(null, 'local')
    panels.bindSession(panel.id, session.id)
    useSessionStore().initSession(session.id)
    emit('session:status', { id: session.id, status: 'connected' })
    await vi.waitFor(() => expect(mock.call).toHaveBeenCalledTimes(1))
    expect(detachBrowserPanel(panel.id)).toBe(true)
    finish([session])
    await Promise.resolve()
    expect(attachWebTerminal).not.toHaveBeenCalled()
    expect(CloseSession).not.toHaveBeenCalled()
    expect(panels.getPanel(panel.id)).toBeUndefined()
  })

  it('rechecks SSE readiness if a disconnect overlaps the catalog RPC', async () => {
    let reconnect!: () => void
    mock.ready.mockResolvedValueOnce().mockImplementation(() => new Promise(resolve => { reconnect = resolve }))
    dispose = initBrowserTerminalIntegration()
    const panel = usePanelStore().createPanel(null, 'local')
    usePanelStore().bindSession(panel.id, session.id)
    useSessionStore().initSession(session.id)
    emit('session:status', { id: session.id, status: 'connected' })
    await vi.waitFor(() => expect(mock.ready).toHaveBeenCalledTimes(2))
    expect(attachWebTerminal).not.toHaveBeenCalled()
    reconnect()
    await vi.waitFor(() => expect(attachWebTerminal).toHaveBeenCalledExactlyOnceWith(session))
  })

  it('waits for SSE readiness before attach/resync and installs one disposable resync subscription', async () => {
    let ready!: () => void
    mock.ready.mockImplementation(() => new Promise(resolve => { ready = resolve }))
    dispose = initBrowserTerminalIntegration()
    expect(initBrowserTerminalIntegration()).toBe(dispose)
    const attaching = attachBrowserTerminal(session)
    expect(attachWebTerminal).not.toHaveBeenCalled()
    ready()
    await attaching
    emit('web:resync', { reason: 'reconnect', sequence: 23 })
    expect(resyncWebTerminals).not.toHaveBeenCalled()
    ready()
    await vi.waitFor(() => expect(resyncWebTerminals).toHaveBeenCalledTimes(1))
    expect(initWebTerminals).toHaveBeenCalledTimes(1)
    dispose()
    dispose = undefined
    emit('web:resync', { reason: 'gap', sequence: 27 })
    expect(mock.dispose).toHaveBeenCalledTimes(1)
    expect(resyncWebTerminals).toHaveBeenCalledTimes(1)
  })

  it('retains native registration and never resnapshots the desktop authority', async () => {
    window.__UNITERM_WEB__ = undefined
    dispose = initBrowserTerminalIntegration()
    const panel = usePanelStore().createPanel(null, 'local')
    usePanelStore().bindSession(panel.id, session.id)
    emit('session:status', { id: session.id, status: 'connected' })
    emit('web:resync', { reason: 'ready' })
    expect(RegisterSessionForPanel).toHaveBeenCalledExactlyOnceWith(session.id, panel.id)
    expect(usePanelStore().isAttachedPanel(panel.id)).toBe(false)
    expect(attachWebTerminal).not.toHaveBeenCalled()
    expect(resyncWebTerminals).not.toHaveBeenCalled()
    expect(detachBrowserPanel(panel.id)).toBe(false)
    expect(await restoreBrowserTerminalViews()).toBe(false)
    expect(tabStorage.getItem).not.toHaveBeenCalled()
    expect(tabStorage.setItem).not.toHaveBeenCalled()
  })

  it('keeps explicit Disconnect as a real session close', async () => {
    dispose = initBrowserTerminalIntegration()
    const id = await attachBrowserTerminal(session)
    await terminateBrowserSession(id)
    expect(CloseSession).toHaveBeenCalledExactlyOnceWith(session.id)
    expect(usePanelStore().getPanel(id)?.sessionId).toBe(session.id)
  })

  it('does not close a newly created browser PTY when its start response is lost', async () => {
    dispose = initBrowserTerminalIntegration()
    const panel = usePanelStore().createPanel({ type: 'ssh', id: 'saved-config' } as any, 'ssh')
    vi.mocked(CreateSession).mockResolvedValue({ id: session.id } as any)
    vi.mocked(SessionStart).mockRejectedValue(new Error('Connection lost after start'))
    const log = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
      await useDuplicateSession().duplicateSession({ type: 'terminal', panelId: panel.id })
      expect(SessionStart).toHaveBeenCalledTimes(1)
      expect(CloseSession).not.toHaveBeenCalled()
    } finally { log.mockRestore() }
  })
})

describe('per-browser-tab terminal view restoration', () => {
  it('waits for SSE, restores each surviving ID once, and selects the saved active ID', async () => {
    const other = { ...session, id: 'other-session', cols: 72, rows: 20 }
    const saved = { sessionIds: [session.id, 'closed-session', other.id, session.id, 'pending:placeholder', ''], activeSessionId: session.id }
    tabStorage.setItem(storageKey, JSON.stringify(saved))
    tabStorage.setItem.mockClear()
    mock.call.mockResolvedValue([session, other])
    let ready!: () => void
    mock.ready.mockReturnValue(new Promise(resolve => { ready = resolve }))
    // Status may already have arrived before the browser shell is mounted.
    emit('session:status', { id: session.id, status: 'connected' })
    dispose = initBrowserTerminalIntegration()
    const restored = restoreBrowserTerminalViews()
    expect(restoreBrowserTerminalViews()).toBe(restored)
    await Promise.resolve()
    expect(mock.call).not.toHaveBeenCalled()
    expect(attachWebTerminal).not.toHaveBeenCalled()
    expect(tabStorage.setItem).not.toHaveBeenCalled()
    expect(savedViews()).toEqual(saved)
    ready()
    expect(await restored).toBe(true)
    await Promise.resolve()
    expect(mock.call).toHaveBeenCalledExactlyOnceWith('main.App.GetWebSessions')
    expect(attachWebTerminal).toHaveBeenCalledTimes(2)
    expect(attachWebTerminal).toHaveBeenNthCalledWith(1, session)
    expect(attachWebTerminal).toHaveBeenNthCalledWith(2, other)
    const tabs = useTabStore(), panels = usePanelStore()
    expect(tabs.tabs).toHaveLength(2)
    expect(panels.getPanel(tabs.getActivePanelId()!)?.sessionId).toBe(session.id)
    expect(savedViews()).toEqual({ sessionIds: [session.id, other.id], activeSessionId: session.id })
    for (const call of [CreateSession, SessionStart, CloseSession, RegisterSessionForPanel, UnregisterSession, DisableSessionOutputLog]) expect(call).not.toHaveBeenCalled()
  })

  it('drops IDs absent from the host and requests the picker when none survive', async () => {
    tabStorage.setItem(storageKey, JSON.stringify({ sessionIds: ['closed-session'], activeSessionId: 'closed-session' }))
    dispose = initBrowserTerminalIntegration()
    expect(await restoreBrowserTerminalViews()).toBe(false)
    expect(savedViews()).toEqual({ sessionIds: [], activeSessionId: null })
    expect(attachWebTerminal).not.toHaveBeenCalled()
    expect(useTabStore().tabs).toHaveLength(0)
  })

  it.each(['instance', 'browser-tab'])('isolates restored views by %s', async boundary => {
    const saved = { sessionIds: [session.id], activeSessionId: session.id }
    tabStorage.setItem(storageKey, JSON.stringify(saved))
    if (boundary === 'instance') window.__UNITERM_WEB__!.instanceId = 'another-host'
    else vi.stubGlobal('sessionStorage', storage())
    dispose = initBrowserTerminalIntegration()
    expect(await restoreBrowserTerminalViews()).toBe(false)
    expect(attachWebTerminal).not.toHaveBeenCalled()
    expect(mock.call).not.toHaveBeenCalled()
    expect(savedViews()).toEqual(saved)
  })

  it('saves only bound text views and their selection, and removes closed views from storage', async () => {
    dispose = initBrowserTerminalIntegration()
    expect(await restoreBrowserTerminalViews()).toBe(false)
    const panels = usePanelStore(), tabs = useTabStore()
    const text = panels.createPanel(null, 'local')
    const tab = tabs.createTerminalTab('New terminal', text.id)
    panels.movePanelToTab(text.id, tab.id)
    expect(savedViews().sessionIds).toEqual([])
    panels.bindSession(text.id, 'pending:placeholder')
    expect(savedViews().sessionIds).toEqual([])
    panels.bindSession(text.id, session.id)
    expect(savedViews()).toEqual({ sessionIds: [session.id], activeSessionId: session.id })
    const database = panels.createPanel(null, 'database')
    panels.bindSession(database.id, 'independent-database')
    tabs.createTab('database', 'Database', database.id)
    expect(savedViews()).toEqual({ sessionIds: [session.id], activeSessionId: null })
    tabs.setActiveTab(tab.id)
    expect(savedViews().activeSessionId).toBe(session.id)
    expect(detachBrowserPanel(text.id)).toBe(true)
    tabs.closeTab(tab.id)
    expect(savedViews()).toEqual({ sessionIds: [], activeSessionId: null })
    expect(CloseSession).not.toHaveBeenCalled()
  })

  it.each(['pagehide', 'dispose'])('preserves saved IDs when %s is followed by empty-store teardown', async cleanup => {
    tabStorage.setItem(storageKey, JSON.stringify({ sessionIds: [session.id], activeSessionId: session.id }))
    dispose = initBrowserTerminalIntegration()
    expect(await restoreBrowserTerminalViews()).toBe(true)
    const before = savedViews()
    tabStorage.setItem.mockClear()
    if (cleanup === 'pagehide') window.dispatchEvent(new Event('pagehide'))
    else dispose() // App unmount and HMR use this same disposer.
    clearViews()
    await Promise.resolve()
    expect(tabStorage.setItem).not.toHaveBeenCalled()
    expect(savedViews()).toEqual(before)
    expect(CloseSession).not.toHaveBeenCalled()
  })

  it('does not attach or erase IDs if teardown occurs during the catalog request', async () => {
    const saved = { sessionIds: [session.id], activeSessionId: session.id }
    tabStorage.setItem(storageKey, JSON.stringify(saved))
    let finish!: (sessions: WebSession[]) => void
    mock.call.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    dispose = initBrowserTerminalIntegration()
    const restored = restoreBrowserTerminalViews()
    await vi.waitFor(() => expect(mock.call).toHaveBeenCalledTimes(1))
    dispose()
    clearViews()
    finish([session])
    expect(await restored).toBe(false)
    expect(attachWebTerminal).not.toHaveBeenCalled()
    expect(savedViews()).toEqual(saved)
  })

  it('preserves a failed snapshot view and its error for the existing refresh path', async () => {
    tabStorage.setItem(storageKey, JSON.stringify({ sessionIds: [session.id], activeSessionId: session.id }))
    vi.mocked(attachWebTerminal).mockRejectedValueOnce(new Error('Snapshot unavailable'))
    dispose = initBrowserTerminalIntegration()
    expect(await restoreBrowserTerminalViews()).toBe(true)
    const panels = usePanelStore(), tabs = useTabStore()
    const panelId = tabs.getActivePanelId()!
    expect(panels.getPanel(panelId)?.sessionId).toBe(session.id)
    expect(panels.isAttachedPanel(panelId)).toBe(true)
    expect(webTerminalError.value).toBe('Snapshot unavailable')
    expect(savedViews()).toEqual({ sessionIds: [session.id], activeSessionId: session.id })
    emit('web:resync', { reason: 'reconnect', sequence: 7 })
    await vi.waitFor(() => expect(resyncWebTerminals).toHaveBeenCalledTimes(1))
    expect(attachWebTerminal).toHaveBeenCalledTimes(1)
    expect(CreateSession).not.toHaveBeenCalled()
    expect(SessionStart).not.toHaveBeenCalled()
  })

  it('retains IDs on catalog failure until an explicit new view selection can replace them', async () => {
    const saved = { sessionIds: [session.id], activeSessionId: session.id }
    tabStorage.setItem(storageKey, JSON.stringify(saved))
    mock.call.mockRejectedValueOnce(new Error('Catalog unavailable'))
    dispose = initBrowserTerminalIntegration()
    expect(await restoreBrowserTerminalViews()).toBe(false)
    expect(savedViews()).toEqual(saved)
    expect(webTerminalError.value).toBe('Catalog unavailable')
    const other = { ...session, id: 'selected-session' }
    const panelId = await attachBrowserTerminal(other)
    expect(savedViews()).toEqual({ sessionIds: [other.id], activeSessionId: other.id })
    detachBrowserPanel(panelId)
    expect(savedViews()).toEqual({ sessionIds: [], activeSessionId: null })
  })
})
