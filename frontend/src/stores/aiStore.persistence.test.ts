import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'

const backend = vi.hoisted(() => ({ save: vi.fn(), remove: vi.fn(), load: vi.fn(), error: vi.fn() }))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ SaveAIConversation: backend.save, DeleteAIConversation: backend.remove, LoadAISessions: backend.load }))
vi.mock('../services/message', () => ({ msg: { error: backend.error } }))
vi.mock('./localStateStore', () => ({ useLocalStateStore: () => ({ loaded: true, state: {}, update: vi.fn() }) }))
vi.mock('./tabStore', () => ({ useTabStore: () => ({ clearAILockedPanels: vi.fn() }) }))
vi.mock('../utils/platform', () => ({ isMobilePlatform: () => false }))
vi.mock('../i18n', () => ({ t: (s: string) => s }))
import { useAIStore } from './aiStore'

beforeEach(() => {
  vi.resetAllMocks()
  backend.load.mockResolvedValue({ sessions: [{ id: 'old', revision: 'rev-old', name: 'old', createdAt: 1, updatedAt: 1, messages: [{ id: 'm', role: 'user', content: 'old text' }] }] })
  backend.save.mockImplementation(async (entry) => `rev-${entry.messages.length}`)
  setActivePinia(createPinia())
})

it('independent clients create unique conversations and save only changed conversations', async () => {
  const a = useAIStore(); await a.init()
  setActivePinia(createPinia())
  const b = useAIStore(); await b.init()
  expect(a.currentSessionId).not.toBe(b.currentSessionId)
  a.addMessage({ id: 'a', role: 'user', content: 'alpha' })
  b.addMessage({ id: 'b', role: 'user', content: 'beta' })
  await Promise.all([a.doSave(), b.doSave()])
  expect(backend.save).toHaveBeenCalledTimes(2)
  expect(backend.save.mock.calls.map(([entry]) => entry.id).sort()).toEqual([a.currentSessionId, b.currentSessionId].sort())
  a.addMessage({ id: 'next', role: 'assistant', content: 'response' })
  await a.doSave()
  const [entry, revision] = backend.save.mock.calls[backend.save.mock.calls.length - 1]
  expect(revision).toBe('rev-1')
  expect(entry.messages.map((m: { id: string }) => m.id)).toEqual(['a', 'next'])
  expect(backend.remove).not.toHaveBeenCalled()
})

it('keeps local messages and a visible error on stale save/delete', async () => {
  const a = useAIStore(); await a.init()
  a.switchSession('old')
  backend.save.mockRejectedValue(new Error('CONFIG_CONFLICT'))
  backend.remove.mockRejectedValue(new Error('CONFIG_CONFLICT'))
  a.addMessage({ id: 'local', role: 'user', content: 'unsaved' })
  await a.doSave()
  expect(a.saveError).toContain('CONFIG_CONFLICT')
  expect(a.messages[a.messages.length - 1]?.content).toBe('unsaved')
  await a.deleteSession('old')
  expect(a.sessions.some(s => s.id === 'old')).toBe(true)
  expect(backend.remove).toHaveBeenCalledWith('old', 'rev-old')
  expect(backend.error).toHaveBeenCalled()
})
