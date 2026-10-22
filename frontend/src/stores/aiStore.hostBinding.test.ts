// Host-bound sessions (issue #1036): sessions bind to the active terminal's
// connection so switching tabs keeps the AI sidebar on that host's
// conversation, and new sessions are named after the host by default.

import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => () => {}), Off: vi.fn() },
}))

vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  LoadAISessions: vi.fn().mockResolvedValue({ sessions: [], currentSessionId: '' }),
  SaveAISessions: vi.fn().mockResolvedValue(undefined),
  LoadLocalState: vi.fn().mockResolvedValue({}),
  SaveLocalState: vi.fn().mockResolvedValue(undefined),
}))

import { useAIStore } from './aiStore'
import { useTabStore } from './tabStore'
import { usePanelStore } from './panelStore'
import { useConnectionStore } from './connectionStore'
import type { ConnectionConfig } from '../types/session'

function sshConfig(overrides: Partial<ConnectionConfig> = {}): ConnectionConfig {
  return {
    id: 'conn-1',
    name: 'prod-web',
    type: 'ssh',
    host: '10.0.0.1',
    port: 22,
    user: 'root',
    authType: 'password',
    ...overrides,
  } as ConnectionConfig
}

function setupTerminalPanel(cfg: ConnectionConfig | null): string {
  const tabStore = useTabStore()
  const panelStore = usePanelStore()
  const panel = panelStore.createPanel(cfg, 'ssh')
  const tab = tabStore.createTerminalTab('test', panel.id)
  tabStore.setActiveTab(tab.id)
  return panel.id
}

describe('host-bound AI sessions (#1036)', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    // followActiveConnection only acts once the store is initialized; init
    // also runs the load path the real app goes through.
    await useAIStore().init()
  })

  it('connectionKeyForPanel uses the saved connection id', () => {
    const ai = useAIStore()
    const conn = useConnectionStore()
    conn.connections.push(sshConfig())
    const panelId = setupTerminalPanel(sshConfig())
    expect(ai.connectionKeyForPanel(panelId)).toBe('conn-1')
  })

  it('connectionKeyForPanel falls back to host:user:port for ad-hoc connects', () => {
    const ai = useAIStore()
    // Not in the connection store → ad-hoc (fresh conn-* id per launch).
    const cfg = sshConfig({ id: 'conn-1730000000000-abc12' })
    const panelId = setupTerminalPanel(cfg)
    expect(ai.connectionKeyForPanel(panelId)).toBe('10.0.0.1:root:22')
  })

  it('connectionKeyForPanel returns null for config-less panels', () => {
    const ai = useAIStore()
    const panelId = setupTerminalPanel(null)
    expect(ai.connectionKeyForPanel(panelId)).toBeNull()
  })

  it('connectionLabelForPanel prefers the connection name', () => {
    const ai = useAIStore()
    const panelId = setupTerminalPanel(sshConfig())
    expect(ai.connectionLabelForPanel(panelId)).toBe('prod-web')
  })

  it('connectionLabelForPanel falls back to user@host', () => {
    const ai = useAIStore()
    const panelId = setupTerminalPanel(sshConfig({ name: '' }))
    expect(ai.connectionLabelForPanel(panelId)).toBe('root@10.0.0.1')
  })

  it('followActiveConnection adopts the current empty session and names it after the host', () => {
    const ai = useAIStore()
    const conn = useConnectionStore()
    conn.connections.push(sshConfig())
    setupTerminalPanel(sshConfig())
    ai.createSession() // fresh empty session, unbound

    ai.followActiveConnection()

    const current = ai.sessions.find(s => s.id === ai.currentSessionId)!
    expect(current.connectionId).toBe('conn-1')
    expect(current.name).toBe('prod-web')
  })

  it('followActiveConnection switches back to the host session on tab switch', () => {
    const ai = useAIStore()
    const conn = useConnectionStore()
    conn.connections.push(sshConfig())
    setupTerminalPanel(sshConfig())

    // First visit: adopt current empty session.
    ai.followActiveConnection()
    const bound = ai.currentSessionId!
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'check nginx' })

    // Somewhere else: another connection, new empty session.
    const other = sshConfig({ id: 'conn-2', name: 'db', host: '10.0.0.2' })
    conn.connections.push(other)
    setupTerminalPanel(other)
    ai.followActiveConnection()
    expect(ai.currentSessionId).not.toBe(bound)

    // Switch back to the first tab: the prod-web conversation returns.
    setupTerminalPanel(sshConfig())
    ai.followActiveConnection()
    expect(ai.currentSessionId).toBe(bound)
  })

  it('followActiveConnection skips switching while the agent is running', () => {
    const ai = useAIStore()
    const conn = useConnectionStore()
    conn.connections.push(sshConfig())
    setupTerminalPanel(sshConfig())
    ai.followActiveConnection()
    const bound = ai.currentSessionId!
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'work' })

    const other = sshConfig({ id: 'conn-2', name: 'db', host: '10.0.0.2' })
    conn.connections.push(other)
    setupTerminalPanel(other)

    ai.isRunning = true
    ai.followActiveConnection()
    expect(ai.currentSessionId).toBe(bound) // untouched mid-run
  })

  it('followActiveConnection does nothing without an active panel identity', () => {
    const ai = useAIStore()
    setupTerminalPanel(null)
    ai.createSession()
    const before = ai.currentSessionId
    ai.followActiveConnection()
    expect(ai.currentSessionId).toBe(before)
  })

  it('host-bound sessions survive the 15-session trim; unbound ones go first', () => {
    const ai = useAIStore()
    for (let i = 0; i < 14; i++) {
      ai.createSession()
      ai.addMessage({ id: `msg-u${i}`, role: 'user', content: `u${i}` })
    }
    ai.createSession()
    ai.addMessage({ id: 'msg-b', role: 'user', content: 'bound' })
    const bound = ai.sessions.find(s => s.name === 'bound')!
    bound.connectionId = 'conn-keep'

    ai.createSession() // triggers the trim

    expect(ai.sessions.length).toBeLessThanOrEqual(15)
    expect(ai.sessions.some(s => s.connectionId === 'conn-keep')).toBe(true)
  })

  it('first user message does not rename a host-bound session', () => {
    const ai = useAIStore()
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    s.connectionId = 'conn-1'
    s.name = 'prod-web'

    ai.addMessage({ id: 'msg-1', role: 'user', content: 'hello world this is a long message' })

    expect(s.name).toBe('prod-web')
  })

  it('first user message still auto-names unbound sessions', () => {
    const ai = useAIStore()
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'hello world this is a long message' })
    expect(s.name).toBe('hello world this is ...')
  })

  it('switching back to a host with several sessions returns to the most recently active one', () => {
    const ai = useAIStore()
    const conn = useConnectionStore()
    conn.connections.push(sshConfig())
    setupTerminalPanel(sshConfig())
    ai.followActiveConnection()
    const first = ai.currentSessionId!
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'first conversation' })

    // Second conversation on the same host.
    ai.createSession()
    ai.bindCurrentSessionToActiveConnection()
    const second = ai.currentSessionId!
    expect(second).not.toBe(first)
    ai.addMessage({ id: 'msg-2', role: 'user', content: 'second conversation' })

    // Detour to another host, then back: the SECOND (last active) returns.
    const other = sshConfig({ id: 'conn-2', name: 'db', host: '10.0.0.2' })
    conn.connections.push(other)
    setupTerminalPanel(other)
    ai.followActiveConnection()
    expect(ai.currentSessionId).not.toBe(second)

    setupTerminalPanel(sshConfig())
    ai.followActiveConnection()
    expect(ai.currentSessionId).toBe(second)
  })

  it('the new-session button keeps the fresh session instead of pulling back to the host conversation', () => {
    const ai = useAIStore()
    const conn = useConnectionStore()
    conn.connections.push(sshConfig())
    setupTerminalPanel(sshConfig())
    ai.followActiveConnection()
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'first conversation' })
    const first = ai.currentSessionId!

    // What onNewSession does: create + bind in place.
    ai.createSession()
    ai.bindCurrentSessionToActiveConnection()

    expect(ai.currentSessionId).not.toBe(first)
    const fresh = ai.sessions.find(s => s.id === ai.currentSessionId)!
    expect(fresh.connectionId).toBe('conn-1')
    expect(fresh.name).toBe('prod-web 2') // numbered, not a duplicate name
  })

  it('the first session on a host keeps the plain host label', () => {
    const ai = useAIStore()
    const conn = useConnectionStore()
    conn.connections.push(sshConfig())
    setupTerminalPanel(sshConfig())
    ai.followActiveConnection()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    expect(s.name).toBe('prod-web')
  })
})
