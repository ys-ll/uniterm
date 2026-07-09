import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// ---- mock wails + i18n so the store module can be imported ----
vi.mock('../../wailsjs/runtime', () => ({
  EventsOn: vi.fn(() => () => {}),
}))
vi.mock('../../wailsjs/go/main/App', () => ({
  SaveAIConfig: vi.fn().mockResolvedValue(undefined),
  LoadAIConfig: vi.fn().mockResolvedValue({}),
  SaveAISessions: vi.fn().mockResolvedValue(undefined),
  LoadAISessions: vi.fn().mockResolvedValue({ sessions: [], currentSessionId: null }),
  SaveLocalState: vi.fn().mockResolvedValue(undefined),
  LoadLocalState: vi.fn().mockResolvedValue({}),
}))
vi.mock('../i18n', () => ({ t: (k: string) => k }))

import { useAIStore } from './aiStore'

describe('aiStore message queue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('enqueues trimmed messages', () => {
    const store = useAIStore()
    store.enqueueMessage('  hello  ')
    expect(store.queuedMessages).toHaveLength(1)
    expect(store.queuedMessages[0].content).toBe('hello')
    expect(store.queuedMessages[0].id).toBeTruthy()
  })

  it('ignores empty / whitespace-only input', () => {
    const store = useAIStore()
    store.enqueueMessage('')
    store.enqueueMessage('   ')
    expect(store.queuedMessages).toHaveLength(0)
  })

  it('keeps insertion order for multiple messages', () => {
    const store = useAIStore()
    store.enqueueMessage('first')
    store.enqueueMessage('second')
    expect(store.queuedMessages.map(q => q.content)).toEqual(['first', 'second'])
  })

  it('removes a queued message by id', () => {
    const store = useAIStore()
    store.enqueueMessage('a')
    store.enqueueMessage('b')
    const id = store.queuedMessages[0].id
    store.removeQueuedMessage(id)
    expect(store.queuedMessages.map(q => q.content)).toEqual(['b'])
  })

  it('clearQueue empties the queue', () => {
    const store = useAIStore()
    store.enqueueMessage('a')
    store.clearQueue()
    expect(store.queuedMessages).toHaveLength(0)
  })

  it('stop() clears the queue', () => {
    const store = useAIStore()
    store.enqueueMessage('a')
    store.stop()
    expect(store.queuedMessages).toHaveLength(0)
    expect(store.stopRequested).toBe(true)
  })

  it('createSession() clears the queue', () => {
    const store = useAIStore()
    store.enqueueMessage('a')
    store.createSession()
    expect(store.queuedMessages).toHaveLength(0)
  })

  it('switchSession() clears the queue', () => {
    const store = useAIStore()
    store.createSession()               // creates session A (currentSessionId = A)
    const sessionA = store.currentSessionId!
    store.createSession()               // creates session B, now current
    store.enqueueMessage('pending')
    // confirm we switch to an existing session (successful-switch path, not early return)
    expect(store.sessions.some(s => s.id === sessionA)).toBe(true)
    store.switchSession(sessionA)       // switching sessions must clear the queue
    expect(store.currentSessionId).toBe(sessionA)
    expect(store.queuedMessages).toHaveLength(0)
  })
})
