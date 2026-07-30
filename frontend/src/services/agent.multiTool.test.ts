// Regression test for the multi-tool-use branch in agent.ts. Previously
// `toolUses.splice(1)` silently dropped every tool call after the first
// one — the model would never see them executed, but had no way to know
// it should re-issue them next turn. The fix surfaces dropped tool_uses
// as synthetic tool_result messages so the model sees "[Skipped: only
// one tool call per turn. Re-issue in your next turn.]" with the matching
// tool_call_id.

import { describe, expect, it, vi, beforeEach } from 'vitest'

vi.mock('../../wailsjs/runtime', () => ({
  EventsOn: vi.fn(() => () => {}),
}))

const { mockChat } = vi.hoisted(() => ({
  mockChat: vi.fn(),
}))

vi.mock('./llm', async () => {
  const actual = await vi.importActual<typeof import('./llm')>('./llm')
  return { ...actual, chat: mockChat, AVAILABLE_TOOLS: actual.AVAILABLE_TOOLS, ChatCancelledError: actual.ChatCancelledError, ChatTimeoutError: actual.ChatTimeoutError }
})

vi.mock('../../wailsjs/go/main/App', () => ({
  ClassifyCommandRisk: vi.fn(() => 'read'),
  SessionWrite: vi.fn().mockResolvedValue(undefined),
  GetSkillFile: vi.fn(),
  ListSkillFiles: vi.fn(),
}))

vi.mock('./terminalManager', () => ({
  getManagedTerminal: vi.fn(),
}))

vi.mock('./terminalAgent', () => ({
  executeCommand: vi.fn().mockResolvedValue({ output: '', exitCode: 0, timedOut: false }),
  startCommand: vi.fn().mockResolvedValue({ output: '', started: true }),
  captureTerminal: vi.fn().mockReturnValue({ output: '' }),
  collectOutput: vi.fn().mockResolvedValue({ output: '', timedOut: false, completed: true }),
  sendTerminalKey: vi.fn().mockResolvedValue({ output: '' }),
}))

vi.mock('../utils/runtimeTypeCheck', () => ({
  InputValidationError: class extends Error {},
  validateRequiredString: () => '',
  validateObject: () => ({}),
}))

// Module-level array the test seeds and asserts against. Mirrors the shape
// of the aiStore.messages array — just the fields the dispatcher reads and
// writes. Imports of the real aiStore are avoided because the dispatcher
// reads many other fields and we'd need to mock them all.
const recorded: Array<{ id: string; role: string; content: string; tool_call_id?: string }> = []

vi.mock('../stores/aiStore', () => ({
  useAIStore: () => ({
    addMessage: (m: any) => {
      recorded.push({
        id: m.id,
        role: m.role,
        content: m.content,
        tool_call_id: m.tool_call_id,
      })
      return m
    },
    pendingCommand: null,
    pendingQuestion: null,
    messages: [],
    conversation: [],
    queuedMessages: [],
    status: 'thinking',
    isRunning: false,
    stopRequested: false,
    mode: 'bypass',
    setLastPanelContext: () => {},
    resetStop: () => {},
    clearPendingCommand: () => {},
    clearPendingQuestion: () => {},
    setPendingCommand: () => {},
    setPendingQuestion: () => {},
    doSave: () => {},
    setDebugInfo: () => {},
  }),
}))

// Settings store needs its own mock — the real one calls Pinia init that
// needs a Vue app. We only need settings.ai.maxTurns.
vi.mock('../stores/settingsStore', () => ({
  useSettingsStore: () => ({ settings: { ai: { maxTurns: 0 } } }),
}))

vi.mock('../stores/tabStore', () => ({
  useTabStore: () => ({
    getAILockedPanel: () => null,
    getAILockedPanels: () => [],
    activeTab: { type: 'terminal', panelId: 'panel-1' },
  }),
}))

vi.mock('../stores/panelStore', () => ({
  usePanelStore: () => ({
    getPanel: () => ({ sessionId: 's', type: 'ssh', config: { shellPath: '/bin/bash' } }),
  }),
}))

vi.mock('../stores/skillStore', () => ({
  useSkillStore: () => ({ enabledSkills: [] }),
}))

vi.mock('../stores/commandStore', () => ({
  useCommandStore: () => ({ commands: [] }),
}))

import { runAgent } from './agent'

beforeEach(() => {
  recorded.length = 0
})

describe('multi-tool-use: dropped calls are surfaced, not silently dropped', () => {
  it('two tool_uses in one turn: first executes, second becomes a tool_result "skipped" message', async () => {
    // Mock the chat() binding to return two tool_use blocks.
    mockChat.mockImplementationOnce(async (opts: any) => {
      opts._rawApiMsg = {
        role: 'assistant',
        content: [
          { type: 'tool_use', id: 'toolu_1', name: 'execute_command', input: { command: 'ls', risk: 'read' } },
          { type: 'tool_use', id: 'toolu_2', name: 'capture_terminal', input: { tail_lines: 50 } },
        ],
      }
      opts.onToolUse?.({ id: 'toolu_1', name: 'execute_command', input: { command: 'ls', risk: 'read' } })
      opts.onToolUse?.({ id: 'toolu_2', name: 'capture_terminal', input: { tail_lines: 50 } })
    })

    await runAgent('do two things at once')

    expect(mockChat).toHaveBeenCalled()
    const skipped = recorded.find(
      (m) => m.role === 'tool' && m.tool_call_id === 'toolu_2' && /skipped/i.test(m.content)
    )
    expect(skipped).toBeDefined()
    expect(skipped?.content).toMatch(/only one tool call per turn/i)

    // The first tool_use (toolu_1) must NOT receive a "skipped" tool_result —
    // it should run normally.
    const skipOfFirst = recorded.find(
      (m) => m.role === 'tool' && m.tool_call_id === 'toolu_1' && /skipped/i.test(m.content)
    )
    expect(skipOfFirst).toBeUndefined()
  })

  it('three tool_uses in one turn: only the first runs, the other two are skipped', async () => {
    mockChat.mockImplementationOnce(async (opts: any) => {
      opts.onToolUse?.({ id: 'toolu_A', name: 'execute_command', input: { command: 'ls', risk: 'read' } })
      opts.onToolUse?.({ id: 'toolu_B', name: 'capture_terminal', input: {} })
      opts.onToolUse?.({ id: 'toolu_C', name: 'collect_output', input: {} })
    })

    await runAgent('three at once')

    console.log('DEBUG three tools recorded:', JSON.stringify(recorded, null, 2))
    expect(recorded.find((m) => m.role === 'tool' && m.tool_call_id === 'toolu_B' && /skipped/i.test(m.content))).toBeDefined()
    expect(recorded.find((m) => m.role === 'tool' && m.tool_call_id === 'toolu_C' && /skipped/i.test(m.content))).toBeDefined()
    // toolu_A is the first tool_use — it executes, no "skipped" message.
    expect(recorded.find((m) => m.role === 'tool' && m.tool_call_id === 'toolu_A' && /skipped/i.test(m.content))).toBeUndefined()
  })

  it('single tool_use: no skipped message is emitted', async () => {
    mockChat.mockImplementationOnce(async (opts: any) => {
      opts.onToolUse?.({ id: 'toolu_only', name: 'execute_command', input: { command: 'pwd', risk: 'read' } })
    })

    await runAgent('just one')

    const skipped = recorded.filter((m) =>
      m.role === 'tool' && /skipped/i.test(m.content)
    )
    expect(skipped).toHaveLength(0)
  })
})

