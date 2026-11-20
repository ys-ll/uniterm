// AI session store behaviors:
//
// 1. Default naming (issue #1036): a new session created while a terminal is
//    connected is named after that terminal's connection, so the history list
//    stays identifiable as it grows. Without a terminal the generic label
//    stays. The first user message then appends its first sentence to that
//    label; a manual rename is never overwritten.
// 2. Session-boundary binding: creating a new session or switching to another
//    session clears the locked panel, so the next conversation re-locks onto
//    whatever terminal is active at that moment.

import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => () => {}), Off: vi.fn() },
}))

vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  LoadAISessions: vi.fn().mockResolvedValue({ sessions: [], currentSessionId: '' }),
  SaveAISessions: vi.fn().mockResolvedValue(undefined),
  DisableSessionOutputLog: vi.fn().mockResolvedValue(undefined),
  RegisterSessionForPanel: vi.fn().mockResolvedValue(undefined),
  UnregisterSession: vi.fn().mockResolvedValue(undefined),
  LoadLocalState: vi.fn().mockResolvedValue({}),
  SaveLocalState: vi.fn().mockResolvedValue(undefined),
}))

import { useAIStore } from './aiStore'
import { useTabStore } from './tabStore'
import { usePanelStore } from './panelStore'
import { t } from '../i18n'
import type { ConnectionConfig } from '../types/session'

describe('replaying saved tool calls', () => {
  it('repairs an empty string input without mutating saved history', () => {
    setActivePinia(createPinia())
    const ai = useAIStore()
    const raw = { role: 'assistant', content: [{ type: 'tool_use', id: 't1', name: 'interrupt_command', input: '' }] }
    const history = [
      { id: 'a', role: 'assistant' as const, content: '', _rawApiMsg: raw },
      { id: 'b', role: 'tool' as const, content: 'done', tool_call_id: 't1' },
    ]
    const conversation = ai.buildConversation(history)
    expect(conversation[0].content).toEqual([{ type: 'tool_use', id: 't1', name: 'interrupt_command', input: {} }])
    expect(raw.content[0].input).toBe('')
  })

  it.each([null, [], 'invalid', 42])('repairs invalid saved input %s', (input) => {
    setActivePinia(createPinia())
    const ai = useAIStore()
    const history = [
      { id: 'a', role: 'assistant' as const, content: '', _rawApiMsg: { content: [{ type: 'tool_use', id: 't1', input }] } },
      { id: 'b', role: 'tool' as const, content: 'done', tool_call_id: 't1' },
    ]
    expect(ai.buildConversation(history)[0].content).toEqual([{ type: 'tool_use', id: 't1', input: {} }])
  })
})

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

// tabState is a module-level reactive singleton (not pinia state), so tabs
// created by earlier tests in this file leak into later ones — close them
// so "no active terminal" cases start clean.
function closeAllTabs() {
  const tabStore = useTabStore()
  for (const tab of [...tabStore.tabs]) {
    tabStore.closeTab(tab.id)
  }
  tabStore.clearAILockedPanels()
}

function setupTerminalPanel(cfg: ConnectionConfig | null): string {
  closeAllTabs()
  const tabStore = useTabStore()
  const panelStore = usePanelStore()
  const panel = panelStore.createPanel(cfg, 'ssh')
  const tab = tabStore.createTerminalTab('test', panel.id)
  tabStore.setActiveTab(tab.id)
  return panel.id
}

describe('session default naming from the active terminal (#1036)', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    closeAllTabs()
    await useAIStore().init()
  })

  it('names a new session after the active terminal connection', () => {
    const ai = useAIStore()
    setupTerminalPanel(sshConfig())
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    expect(s.name).toBe('prod-web')
  })

  it('falls back to user@host when the connection has no name', () => {
    const ai = useAIStore()
    setupTerminalPanel(sshConfig({ name: '' }))
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    expect(s.name).toBe('root@10.0.0.1')
  })

  it('keeps the generic label without an active terminal', () => {
    const ai = useAIStore()
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    expect(s.name).toBe(t('ai.newSession'))
  })

  it('falls back to the tab name for a config-less panel', () => {
    const ai = useAIStore()
    setupTerminalPanel(null)
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    expect(s.name).toBe('test')
  })

  it('prefers the AI-locked panel over the active tab', () => {
    const ai = useAIStore()
    const tabStore = useTabStore()
    setupTerminalPanel(sshConfig()) // active tab: prod-web
    const lockedPanelId = setupTerminalPanel(sshConfig({ id: 'conn-2', name: 'db', host: '10.0.0.2' }))
    tabStore.addAILockedPanel(lockedPanelId)

    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    expect(s.name).toBe('db')
  })

  it('the first user message combines the host label with the first sentence', () => {
    const ai = useAIStore()
    setupTerminalPanel(sshConfig())
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'a fairly long first message' })
    expect(s.name).toBe('prod-web: a fairly long first ...')
  })

  it('the first user message still auto-names generic-label sessions', () => {
    const ai = useAIStore()
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'hello world this is a long message' })
    expect(s.name).toBe('hello world this is ...')
  })

  it('a manual rename survives the first user message', () => {
    const ai = useAIStore()
    setupTerminalPanel(sshConfig())
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    ai.renameSession(s.id, 'my own name')
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'hello world' })
    expect(s.name).toBe('my own name')
  })

  it('an explicit name argument wins over the host label', () => {
    const ai = useAIStore()
    setupTerminalPanel(sshConfig())
    ai.createSession('custom topic')
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    expect(s.name).toBe('custom topic')
  })
})

describe('AI session changes clear the terminal binding', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    closeAllTabs()
  })

  it('createSession clears the locked panel', () => {
    const ai = useAIStore()
    const tabStore = useTabStore()
    tabStore.addAILockedPanel('panel-1')
    expect(tabStore.getAILockedPanels()).toEqual(['panel-1'])

    ai.createSession()

    expect(tabStore.getAILockedPanels()).toEqual([])
  })

  it('switchSession clears the locked panel', () => {
    const ai = useAIStore()
    const tabStore = useTabStore()
    ai.createSession()
    const firstId = ai.currentSessionId!
    ai.createSession()

    tabStore.addAILockedPanel('panel-1')
    expect(tabStore.getAILockedPanels()).toEqual(['panel-1'])

    ai.switchSession(firstId)

    expect(tabStore.getAILockedPanels()).toEqual([])
  })
})

describe('message deletion keeps the session array independent', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    closeAllTabs()
    await useAIStore().init()
  })

  // Regression: deleteMessage / deleteMessageWithReply / truncateFrom used to
  // alias session.messages to messages.value, which made the double push in
  // addMessage store every later message twice (duplicate ids in the saved
  // session).
  it('adding messages after a delete does not duplicate them in the session', () => {
    const ai = useAIStore()
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'first' })
    ai.addMessage({ id: 'msg-2', role: 'assistant', content: 'reply' })

    ai.deleteMessage('msg-1')
    ai.addMessage({ id: 'msg-3', role: 'user', content: 'second' })
    ai.addMessage({ id: 'msg-4', role: 'assistant', content: 'reply 2' })

    const ids = s.messages.map(m => m.id)
    expect(ids).toEqual(['msg-2', 'msg-3', 'msg-4'])
    expect(new Set(ids).size).toBe(ids.length)
    expect(ai.messages.map(m => m.id)).toEqual(ids)
  })

  it('adding messages after delete-with-reply does not duplicate them in the session', () => {
    const ai = useAIStore()
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'first' })
    ai.addMessage({ id: 'msg-2', role: 'assistant', content: 'reply' })

    ai.deleteMessageWithReply('msg-1')
    ai.addMessage({ id: 'msg-3', role: 'user', content: 'second' })

    const ids = s.messages.map(m => m.id)
    expect(ids).toEqual(['msg-3'])
    expect(ai.messages.map(m => m.id)).toEqual(ids)
  })

  it('adding messages after truncateFrom does not duplicate them in the session', () => {
    const ai = useAIStore()
    ai.createSession()
    const s = ai.sessions.find(s => s.id === ai.currentSessionId)!
    ai.addMessage({ id: 'msg-1', role: 'user', content: 'first' })
    ai.addMessage({ id: 'msg-2', role: 'assistant', content: 'reply' })
    ai.addMessage({ id: 'msg-3', role: 'user', content: 'second' })
    ai.addMessage({ id: 'msg-4', role: 'assistant', content: 'reply 2' })

    ai.truncateFrom('msg-2')
    ai.addMessage({ id: 'msg-5', role: 'user', content: 'third' })

    const ids = s.messages.map(m => m.id)
    expect(ids).toEqual(['msg-1', 'msg-5'])
    expect(ai.messages.map(m => m.id)).toEqual(ids)
  })
})
