import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mock = vi.hoisted(() => ({
  events: new Map<string, Set<(event: any) => void>>(),
  call: vi.fn(async (..._args: any[]): Promise<any> => {}),
  write: vi.fn(async () => {}),
  ready: vi.fn(async () => {}),
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
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  SessionWrite: mock.write,
  RegisterSessionForPanel: vi.fn(async () => {}), UnregisterSession: vi.fn(async () => {}),
  DisableSessionOutputLog: vi.fn(async () => {}), CloseSession: vi.fn(async () => {}),
}))
vi.mock('../stores/settingsStore', () => ({ useSettingsStore: () => ({ settings: { terminal: {}, customTerminalThemes: [] }, resolvedAppTheme: 'dark' }) }))
vi.mock('../stores/localStateStore', () => ({ useLocalStateStore: () => ({ state: {} }) }))
vi.mock('../composables/useTerminal', () => ({ getXtermTheme: () => ({}) }))
vi.mock('../composables/useTerminalTheme', () => ({ resolveXtermBackground: () => ({}), resolveTerminalThemeName: () => 'dark', applyTerminalBgVar() {} }))
vi.mock('../utils/browserRuntime', () => ({ waitForBrowserEvents: mock.ready }))

import { initBrowserTerminalIntegration, attachBrowserTerminal } from '../utils/webTerminalIntegration'
import { detachBrowserPanel } from './webTerminals'
import { getManagedTerminal } from './terminalManager'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useTabStore } from '../stores/tabStore'
import { CloseSession, RegisterSessionForPanel, UnregisterSession, DisableSessionOutputLog } from '../../bindings/github.com/ys-ll/uniterm/app'
import type { WebSession, WebTerminalSnapshot } from './webTerminalBridge'
import '@xterm/xterm/css/xterm.css'

const session: WebSession = { id: 'runtime-shell', type: 'local', title: 'Shell', status: 'connected', cols: 41, rows: 9, activeClient: 'desktop', resizeCapability: 'pty', connectionId: 'saved-shell', shellPath: '/bin/zsh' }
const emit = (name: string, data: any) => mock.events.get(name)?.forEach(fn => fn({ data }))
let dispose: (() => void) | undefined

beforeEach(() => {
  setActivePinia(createPinia())
  usePanelStore().panels.clear()
  useSessionStore().sessions.clear()
  useTabStore().tabs.splice(0)
  vi.clearAllMocks()
  mock.call.mockResolvedValue(undefined)
  window.__UNITERM_WEB__ = undefined
})
afterEach(() => { dispose?.(); dispose = undefined; window.__UNITERM_WEB__ = undefined })

async function desktopSnapshot(): Promise<WebTerminalSnapshot> {
  dispose = initBrowserTerminalIntegration()
  emit('web:terminal-attach', { requestId: 'start-request', sessionId: session.id, session })
  const authority = getManagedTerminal(session.id)!.terminal
  emit('web:terminal-attach', { requestId: 'repeat-request', sessionId: session.id, session })
  expect(getManagedTerminal(session.id)!.terminal).toBe(authority)
  expect(mock.call.mock.calls.filter(c => c[0] === 'main.App.AcknowledgeWebTerminal')).toHaveLength(2)
  emit('session:data', { id: session.id, data: 'ready\x1b[?1049h\x1b[2Jeditor', webSequence: 4, webTimestamp: 100 })
  emit('web:snapshot-request', { requestId: 'snapshot-request', sessionId: session.id, sequence: 4, session })
  await expect.poll(() => mock.call.mock.calls.some(c => c[0] === 'main.App.ProvideWebTerminalSnapshot')).toBe(true)
  const snapshot = structuredClone(mock.call.mock.calls.find(c => c[0] === 'main.App.ProvideWebTerminalSnapshot')![2])
  dispose()
  window.__UNITERM_WEB__ = { instanceId: 'host', clientId: 'visitor' }
  mock.call.mockImplementation(async name => name === 'main.App.GetWebSessions' ? [session] : snapshot)
  dispose = initBrowserTerminalIntegration()
  return snapshot
}

it('uses the real authority snapshot for picker attachment and detaches without touching the host', async () => {
  await desktopSnapshot()
  const panelId = await attachBrowserTerminal(session)
  expect(await attachBrowserTerminal(session)).toBe(panelId)
  const managed = getManagedTerminal(session.id)!
  expect(managed.terminal.cols).toBe(41)
  expect(managed.terminal.rows).toBe(9)
  expect(managed.terminal.buffer.active.type).toBe('alternate')
  expect(managed.terminal.buffer.active.getLine(0)?.translateToString(true)).toContain('editor')
  expect(mock.call.mock.calls.filter(c => c[0] === 'main.App.GetWebTerminalSnapshot')).toHaveLength(1)
  expect(detachBrowserPanel(panelId)).toBe(true)
  expect(getManagedTerminal(session.id)).toBeUndefined()
  for (const call of [CloseSession, RegisterSessionForPanel, UnregisterSession, DisableSessionOutputLog]) expect(call).not.toHaveBeenCalled()
})

it('automatically snapshots connected new panels and resnapshots once on a later ready', async () => {
  const snapshot = await desktopSnapshot()
  const panels = usePanelStore()
  const panel = panels.createPanel(null, 'local')
  panels.bindSession(panel.id, session.id)
  useSessionStore().initSession(session.id)
  emit('session:status', { id: session.id, status: 'connected' })
  await expect.poll(() => getManagedTerminal(session.id)?.syncing).toBe(false)
  expect(panels.isAttachedPanel(panel.id)).toBe(true)
  expect(useSessionStore().sessions.get(session.id)?.web).toEqual(session)
  let finish!: (snapshot: WebTerminalSnapshot) => void
  mock.call.mockImplementation(async name => {
    if (name === 'main.App.GetWebTerminalSnapshot') return new Promise(resolve => { finish = resolve })
    return [session]
  })
  emit('web:resync', { reason: 'reconnect', sequence: 25 })
  await expect.poll(() => getManagedTerminal(session.id)?.syncing).toBe(true)
  emit('session:data', { id: session.id, data: 'new', webSequence: 5, webTimestamp: 110 })
  finish(snapshot)
  await expect.poll(() => getManagedTerminal(session.id)?.stream.sequence).toBe(5)
  expect(getManagedTerminal(session.id)!.terminal.buffer.active.getLine(0)?.translateToString(true)).toContain('editornew')
  expect(mock.call.mock.calls.filter(c => c[0] === 'main.App.GetWebTerminalSnapshot')).toHaveLength(2)
  expect(mock.write).not.toHaveBeenCalled()
})
