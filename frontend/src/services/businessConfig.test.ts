import { beforeEach, expect, it, vi } from 'vitest'

const backend = vi.hoisted(() => ({ load: vi.fn(), save: vi.fn(), message: vi.fn() }))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ LoadBusinessConfig: backend.load, SaveBusinessConfig: backend.save }))
vi.mock('./message', () => ({ msg: { copyable: backend.message, error: backend.message } }))

beforeEach(() => { vi.resetModules(); vi.resetAllMocks() })

it('retains the editing revision after remote notification and exposes a rejected stale save', async () => {
  const client = await import('./businessConfig')
  backend.load.mockResolvedValueOnce({ revision: 'old', json: '{"theme":"dark"}' })
  const draft = await client.LoadSettings()
  backend.load.mockResolvedValue({ revision: 'other-client', json: '{"theme":"light"}' })
  await client.configChanged('settings')
  draft.theme = 'deep-blue'
  backend.save.mockRejectedValue(new Error('CONFIG_CONFLICT'))
  await expect(client.SaveSettings(draft)).rejects.toThrow('CONFIG_CONFLICT')
  expect(backend.save).toHaveBeenCalledWith('settings', 'old', JSON.stringify(draft))
  expect(draft.theme).toBe('deep-blue')
  expect(backend.message).toHaveBeenLastCalledWith(expect.stringContaining('CONFIG_CONFLICT'), 'error')
})

it('serializes local autosaves using acknowledged revisions and captures each payload before queuing', async () => {
  const client = await import('./businessConfig')
  backend.load.mockResolvedValue({ revision: 'one', json: '{}' })
  await client.LoadConnections()
  let finish!: (value: unknown) => void
  backend.save.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    .mockResolvedValueOnce({ revision: 'three', json: '{}' })
  const draft = { connections: ['a'] }
  const first = client.SaveConnections(draft)
  draft.connections.push('b')
  const second = client.SaveConnections(draft)
  await vi.waitFor(() => expect(backend.save).toHaveBeenCalledTimes(1))
  finish({ revision: 'two', json: '{}' })
  await Promise.all([first, second])
  expect(backend.save.mock.calls).toEqual([
    ['connections', 'one', '{"connections":["a"]}'],
    ['connections', 'two', '{"connections":["a","b"]}'],
  ])
})

it('never rebases an old autosave onto a revision from an in-flight reload', async () => {
  const client = await import('./businessConfig')
  backend.load.mockResolvedValueOnce({ revision: 'old', json: '{"theme":"dark"}' })
  const draft = await client.LoadSettings()
  let finish!: (value: unknown) => void
  backend.load.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  const loading = client.LoadSettings()
  await vi.waitFor(() => expect(backend.load).toHaveBeenCalledTimes(2))
  const saving = client.SaveSettings(draft)
  finish({ revision: 'new', json: '{"theme":"light"}' })
  await loading
  await expect(saving).rejects.toThrow('reloading')
  expect(backend.save).not.toHaveBeenCalled()
})
