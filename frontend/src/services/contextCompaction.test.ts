import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}), Off: vi.fn() } }))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  SummarizeAIContext: vi.fn(), ChatCompletion: vi.fn(),
  LoadAISessions: vi.fn().mockResolvedValue({ sessions: [] }),
  SaveAISessions: vi.fn().mockResolvedValue(undefined),
}))
const preferences = vi.hoisted(() => ({
  autoCompact: true, contextTokenBudget: 32000, summaryModelId: '',
  models: [] as Array<{ id: string; apiKey: string; baseURL: string; model: string; protocol: string; userAgent?: string; proxyId?: string }>,
}))
vi.mock('../stores/settingsStore', () => ({ useSettingsStore: () => ({ ai: preferences, activeModel: {
  apiKey: 'test', baseURL: 'https://example.invalid/v1', model: 'test', protocol: 'openai',
} }) }))

import { LoadAISessions, SaveAISessions, SummarizeAIContext } from '../../bindings/github.com/ys-ll/uniterm/app'
import { useAIStore } from '../stores/aiStore'
import { prepareAgentContext } from './contextCompaction'
import { estimateRequestTokens } from './aiContext'
import { ChatCancelledError } from './llm'

function longSession() {
  const store = useAIStore()
  store.createSession()
  for (let i = 0; i < 20; i++) store.addMessage({
    id: `message-${i}`, role: i % 2 === 0 ? 'user' : 'assistant',
    content: `${i}:` + 'history '.repeat(1400), createdAt: 1,
  })
  store.addMessage({ id: 'latest-user', role: 'user', content: 'Continue with the pending fix', _contextHeader: 'PowerShell; current directory C:/work', createdAt: 1 })
  return store
}

describe('incremental context compaction', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    Object.assign(preferences, { autoCompact: true, contextTokenBudget: 32000, summaryModelId: '', models: [] })
    vi.mocked(SummarizeAIContext).mockResolvedValue('Goal: pending fix. Completed: inspected logs. Next: apply fix.')
  })

  it('does not request summaries for short conversations', async () => {
    const store = useAIStore()
    store.createSession()
    store.addMessage({ id: 'user', role: 'user', content: 'hello' })
    const result = await prepareAgentContext('rules', [])
    expect(SummarizeAIContext).not.toHaveBeenCalled()
    expect(result.messages).toEqual(store.conversation)
  })

  it('sends the full history when disabled, including history behind an existing checkpoint', async () => {
    const store = longSession()
    await prepareAgentContext('rules', [])
    expect(store.contextSummary).toBeDefined()
    vi.mocked(SummarizeAIContext).mockClear()
    preferences.autoCompact = false
    const result = await prepareAgentContext('rules', [], true)
    expect(result.messages).toEqual(store.buildConversation(store.messages))
    expect(JSON.stringify(result.messages)).toContain('0:history')
    expect(SummarizeAIContext).not.toHaveBeenCalled()
    expect(result.failed).toBe(false)
    expect(store.contextRecord).toBeNull()
  })

  it('uses the configured budget instead of compacting every model at 32K', async () => {
    const store = longSession()
    preferences.contextTokenBudget = 512000
    const result = await prepareAgentContext('rules', [])
    expect(estimateRequestTokens('rules', result.messages, [])).toBeGreaterThan(32000)
    expect(result.messages).toEqual(store.buildConversation(store.messages))
    expect(SummarizeAIContext).not.toHaveBeenCalled()
  })

  it('uses the selected summary model with its own credentials, protocol and proxy', async () => {
    longSession()
    preferences.summaryModelId = 'summary'
    preferences.models = [{ id: 'summary', apiKey: 'summary-key', baseURL: 'https://summary.invalid',
      model: 'small-model', protocol: 'responses', userAgent: 'summary-agent', proxyId: 'summary-proxy' }]
    await prepareAgentContext('rules', [], false, () => true, {
      id: 'main', name: 'main', apiKey: 'main-key', baseURL: 'https://main.invalid', model: 'large', protocol: 'openai',
    })
    expect(vi.mocked(SummarizeAIContext).mock.calls[0].slice(0, 6)).toEqual([
      'summary-key', 'https://summary.invalid', 'small-model', 'responses', 'summary-agent', 'summary-proxy',
    ])
  })

  it('scales the retained history with the configured compaction budget', async () => {
    const store = longSession()
    preferences.contextTokenBudget = 64000
    const result = await prepareAgentContext('rules', [])
    const size = estimateRequestTokens('rules', result.messages, [])
    expect(size).toBeGreaterThan(20000)
    expect(size).toBeLessThan(64000)
    expect(store.contextSummary).toBeDefined()
  })

  it('falls back to the request model when the summary model no longer exists', async () => {
    longSession()
    preferences.summaryModelId = 'removed'
    await prepareAgentContext('rules', [], false, () => true, {
      id: 'snapshot', name: 'snapshot', apiKey: 'snapshot-key', baseURL: 'https://snapshot.invalid', model: 'snapshot', protocol: 'openai',
    })
    expect(vi.mocked(SummarizeAIContext).mock.calls[0][0]).toBe('snapshot-key')
  })

  it('compresses incrementally, saves a checkpoint and preserves full history', async () => {
    const store = longSession()
    const full = JSON.stringify(store.messages)
    const result = await prepareAgentContext('rules', [{ description: 'read files' }])
    expect(result.failed).toBe(false)
    expect(store.contextRecord?.failed).toBe(false)
    expect(store.contextRecord?.afterId).toBe(store.contextSummary?.throughMessageId)
    expect(JSON.stringify(store.messages)).toBe(full)
    expect(store.contextSummary?.content).toContain('pending fix')
    expect(estimateRequestTokens('rules', result.messages, [])).toBeLessThan(24000)
    expect(JSON.stringify(result.messages)).toContain('PowerShell')
    expect(JSON.stringify(result.messages)).toContain('Goal: pending fix')
    expect(store.compactionRequestId).toBe('')
    const calls = vi.mocked(SummarizeAIContext).mock.calls
    // Whole history fits in one summary request now (the conversation
    // request that just went out was larger).
    expect(calls.length).toBe(1)
    // The summary pass reuses the chat request shape: the system prompt and
    // serialized history match the conversation, the instruction is the
    // final user message.
    const summaryRequest = JSON.parse(calls[0][7] as unknown as string)
    expect(summaryRequest.system).toBe('rules')
    expect(summaryRequest.messages[summaryRequest.messages.length - 1].content).toContain('handoff')
    expect(summaryRequest.messages[summaryRequest.messages.length - 1].role).toBe('user')
    expect(JSON.stringify(vi.mocked(SaveAISessions).mock.calls)).toContain('contextSummary')
    await prepareAgentContext('rules', [])
    expect(SummarizeAIContext).toHaveBeenCalledTimes(1)
  })

  it('keeps complete recent exchanges and shows a record after summary failure', async () => {
    const store = longSession()
    vi.mocked(SummarizeAIContext).mockRejectedValue(new Error('AI_COMPACTION_HTTP_404'))
    const result = await prepareAgentContext('rules', [])
    expect(result.failed).toBe(true)
    expect(store.contextSummary).toBeUndefined()
    expect(store.contextRecord?.failed).toBe(true)
    expect(store.contextRecord?.text).toContain('failed')
    expect(store.contextRecord?.text).toContain('AI_COMPACTION_HTTP_404')
    // The record is anchored at the cut point, not pinned to the bottom.
    expect(store.contextRecord?.afterId).toMatch(/^message-/)
    expect(JSON.stringify(result.messages)).toContain('Continue with the pending fix')
    expect(store.messages.length).toBe(21)
    // A later step re-enters with skipCompaction and must keep the reason.
    await prepareAgentContext('rules', [], true)
    expect(SummarizeAIContext).toHaveBeenCalledTimes(1)
    expect(store.contextRecord?.text).toContain('AI_COMPACTION_HTTP_404')
  })

  it('carries the checkpoint into later compression passes', async () => {
    const store = longSession()
    await prepareAgentContext('rules', [])
    const firstCheckpoint = store.contextSummary!
    const count = vi.mocked(SummarizeAIContext).mock.calls.length
    for (let i = 0; i < 16; i++) store.addMessage({
      id: `later-${i}`, role: i % 2 === 0 ? 'user' : 'assistant', content: 'new evidence '.repeat(1000),
    })
    vi.mocked(SummarizeAIContext).mockResolvedValue('Updated goal and new evidence')
    await prepareAgentContext('rules', [])
    const laterPass = JSON.parse(vi.mocked(SummarizeAIContext).mock.calls[count][7] as unknown as string)
    expect(JSON.stringify(laterPass.messages[0])).toContain(firstCheckpoint.content)
    expect(store.contextSummary?.throughMessageId).not.toBe(firstCheckpoint.throughMessageId)
    expect(store.contextSummary?.content).toBe('Updated goal and new evidence')
    expect(store.messages.some(m => m.id === 'message-0')).toBe(true)
  })

  it('does not commit partial summaries when a later pass fails', async () => {
    const store = useAIStore()
    store.createSession()
    // Large enough that the summary input splits into two request-sized runs.
    for (let i = 0; i < 70; i++) store.addMessage({
      id: `message-${i}`, role: i % 2 === 0 ? 'user' : 'assistant',
      content: `${i}:` + 'history '.repeat(1400), createdAt: 1,
    })
    vi.mocked(SummarizeAIContext).mockResolvedValueOnce('partial handoff').mockRejectedValueOnce(new Error('offline'))
    const result = await prepareAgentContext('rules', [])
    expect(vi.mocked(SummarizeAIContext).mock.calls.length).toBe(2)
    expect(result.failed).toBe(true)
    expect(store.contextSummary).toBeUndefined()
  })

  it('discards oversized summaries without committing a checkpoint', async () => {
    longSession()
    vi.mocked(SummarizeAIContext).mockResolvedValueOnce('PRIVATE_SUMMARY '.repeat(5000))
    const result = await prepareAgentContext('rules', [])
    expect(result.failed).toBe(true)
    expect(useAIStore().contextSummary).toBeUndefined()
    expect(useAIStore().contextRecord?.failed).toBe(true)
  })

  it.each(['stop', 'rollback', 'session', 'superseded'])('discards results after %s', async (action) => {
    const store = longSession()
    let current = true
    vi.mocked(SummarizeAIContext).mockImplementationOnce(async () => {
      if (action === 'stop') store.stop()
      if (action === 'rollback') store.truncateFrom('message-2')
      if (action === 'session') store.createSession()
      if (action === 'superseded') current = false
      return 'stale summary'
    })
    await expect(prepareAgentContext('rules', [], false, () => current)).rejects.toBeInstanceOf(ChatCancelledError)
    expect(store.contextSummary).toBeUndefined()
    expect(store.compactionRequestId).toBe('')
  })

  it('invalidates checkpoints on deletion and persists rollback after switching sessions', async () => {
    const store = longSession()
    await prepareAgentContext('rules', [])
    const id = store.currentSessionId!
    expect(store.contextSummary).toBeDefined()
    store.switchSession(id)
    store.deleteMessage('message-0')
    expect(store.contextSummary).toBeUndefined()
    expect(store.sessions.find(s => s.id === id)?.messages.some(m => m.id === 'message-0')).toBe(false)
    store.truncateFrom('message-3')
    expect(store.sessions.find(s => s.id === id)?.messages.map(m => m.id)).toEqual(['message-1', 'message-2'])
  })

  it('restores checkpoints and environment headers from saved sessions', async () => {
    const store = longSession()
    await prepareAgentContext('rules', [])
    await store.doSave()
    const saved = vi.mocked(SaveAISessions).mock.calls.slice(-1)[0][0]
    vi.mocked(LoadAISessions).mockResolvedValueOnce(saved as any)
    setActivePinia(createPinia())
    const restored = useAIStore()
    await restored.init()
    restored.switchSession(saved.sessions[0].id)
    expect(restored.contextSummary?.content).toContain('pending fix')
    expect(JSON.stringify(restored.conversation)).toContain('PowerShell')
  })

  it('preserves multiple tool results through conversation normalization', () => {
    const store = useAIStore()
    const calls = ['a', 'b'].map(id => ({ id, type: 'function' as const, function: { name: 'read', arguments: '{}' } }))
    const messages = store.buildConversation([
      { id: 'call', role: 'assistant', content: '', tool_calls: calls },
      { id: 'a', role: 'tool', content: 'first result', tool_call_id: 'a' },
      { id: 'b', role: 'tool', content: 'second result', tool_call_id: 'b' },
    ])
    expect((messages[0].content as any[]).filter(b => b.type === 'tool_use')).toHaveLength(2)
    expect((messages[1].content as any[]).filter(b => b.type === 'tool_result')).toHaveLength(2)
  })

  it('sends oversized current input unchanged instead of dropping it', async () => {
    const store = useAIStore()
    store.createSession()
    store.addMessage({ id: 'only', role: 'user', content: 'X'.repeat(200000) })
    const result = await prepareAgentContext('rules', [])
    expect(result.messages).toEqual(store.buildConversation(store.messages))
    expect(store.messages[0].content.length).toBe(200000)
    expect(SummarizeAIContext).not.toHaveBeenCalled()
  })

  it('keeps an oversized latest exchange and still compacts the older history', async () => {
    const store = longSession()
    store.addMessage({ id: 'huge-latest', role: 'user', content: 'X'.repeat(200000) })
    const result = await prepareAgentContext('rules', [])
    // The oversized latest input is never dropped or shortened; the request
    // still goes out (the provider is the authority on its own limit).
    expect(JSON.stringify(result.messages)).toContain('X'.repeat(1000))
    expect(store.messages[store.messages.length - 1].content.length).toBe(200000)
    // Older history is still compacted and the checkpoint committed.
    expect(store.contextSummary).toBeDefined()
    expect(JSON.stringify(result.messages)).toContain('Goal: pending fix')
  })

  it('stubs oversized tool results in the request copy only', async () => {
    const store = useAIStore()
    store.createSession()
    store.addMessage({
      id: 'asst', role: 'assistant', content: '',
      tool_calls: [{ id: 'call-1', type: 'function', function: { name: 'dump', arguments: '{}' } }],
    })
    store.addMessage({ id: 'tool', role: 'tool', content: 'R'.repeat(200000), tool_call_id: 'call-1' })
    const result = await prepareAgentContext('rules', [])
    const payload = JSON.stringify(result.messages)
    expect(payload).toContain('Tool result omitted')
    expect(payload).toContain('"dump"')
    expect(payload).not.toContain('R'.repeat(1000))
    // The stored history keeps the full output for the UI.
    expect(store.messages.find(m => m.id === 'tool')!.content.length).toBe(200000)
    expect(SummarizeAIContext).not.toHaveBeenCalled()
  })

  it('stays usable when system prompt and tool definitions alone approach the trigger', async () => {
    const store = useAIStore()
    store.createSession()
    store.addMessage({ id: 'u', role: 'user', content: 'hello' })
    const hugeTools = [{ description: 'x'.repeat(200000) }]
    const result = await prepareAgentContext('rules', hugeTools)
    expect(result.messages).toEqual(store.buildConversation(store.messages))
    expect(SummarizeAIContext).not.toHaveBeenCalled()
  })
})
