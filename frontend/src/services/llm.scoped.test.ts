import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  call: vi.fn(),
  listeners: new Map<string, (event: { data: unknown }) => void>(),
}))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ ChatCompletionScoped: mocks.call }))
vi.mock('@wailsio/runtime', () => ({ Events: { On: (name: string, cb: (event: { data: unknown }) => void) => {
  mocks.listeners.set(name, cb)
  return () => mocks.listeners.delete(name)
} } }))
vi.mock('../stores/settingsStore', () => ({ useSettingsStore: () => ({ activeModel: { apiKey: 'test', model: 'test' } }) }))
vi.mock('../stores/aiStore', () => ({ useAIStore: () => ({ currentSessionId: 'same-conversation' }) }))
vi.mock('../i18n', () => ({ t: (key: string) => key }))

import { chat, ChatCancelledError } from './llm'

function callResult() {
  let resolve!: (value: string) => void
  let reject!: (error: Error) => void
  const promise = new Promise<string>((res, rej) => { resolve = res; reject = rej })
  const cancel = vi.fn(() => { reject(new Error('context canceled')); return Promise.resolve() })
  return { call: Object.assign(promise, { cancel }), resolve, cancel }
}

describe('request-scoped LLM calls', () => {
  beforeEach(() => { mocks.call.mockReset(); mocks.listeners.clear() })

  it('isolates live text/thinking, cancellation, final text fallback and tools for overlapping callers', async () => {
    const a = callResult(), b = callResult()
    mocks.call.mockReturnValueOnce(a.call).mockReturnValueOnce(b.call)
    const controller = new AbortController()
    const tokenA = vi.fn(), tokenB = vi.fn(), thinkingB = vi.fn(), textB = vi.fn(), toolB = vi.fn()
    const first = chat({ system: '', messages: [], onToken: tokenA, signal: controller.signal }).catch(e => e)
    const second = chat({ system: '', messages: [], onToken: tokenB, onThinking: thinkingB, onChunk: textB, onToolUse: toolB })
    const idA = mocks.call.mock.calls[0][0], idB = mocks.call.mock.calls[1][0]
    expect(idA).not.toBe(idB)
    mocks.listeners.get(`ai:token:${idA}`)?.({ data: { text: 'alpha' } })
    mocks.listeners.get(`ai:token:${idB}`)?.({ data: { text: 'beta' } })
    mocks.listeners.get(`ai:thinking:${idB}`)?.({ data: { text: 'reason' } })
    controller.abort()
    expect(await first).toBeInstanceOf(ChatCancelledError)
    expect(a.cancel).toHaveBeenCalledOnce()
    expect(b.cancel).not.toHaveBeenCalled()
    b.resolve(JSON.stringify({ role: 'assistant', content: [{ type: 'tool_use', id: 'tool', name: 'inspect', input: { x: 1 } }] }))
    await second
    expect(tokenA.mock.calls).toEqual([['alpha']])
    expect(tokenB.mock.calls).toEqual([['beta']])
    expect(thinkingB).toHaveBeenCalledWith('reason')
    expect(textB).toHaveBeenCalledWith('beta')
    expect(toolB).toHaveBeenCalledWith({ id: 'tool', name: 'inspect', input: { x: 1 } })
    expect(mocks.listeners.size).toBe(0)
  })

  it('does not start an already cancelled call and removes its listeners', async () => {
    const controller = new AbortController(); controller.abort()
    await expect(chat({ system: '', messages: [], signal: controller.signal })).rejects.toBeInstanceOf(ChatCancelledError)
    expect(mocks.call).not.toHaveBeenCalled()
    expect(mocks.listeners.size).toBe(0)
  })
})
