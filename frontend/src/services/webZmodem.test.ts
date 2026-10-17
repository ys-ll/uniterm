import { afterEach, beforeEach, expect, it, vi } from 'vitest'
const mock = vi.hoisted(() => ({ call: vi.fn(), reply: vi.fn(), fetch: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Call: { ByName: mock.call }, Events: { On: vi.fn() } }))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ SessionWrite: mock.reply }))
vi.mock('element-plus', () => ({ ElMessageBox: {} }))
vi.mock('./message', () => ({ msg: {} }))
vi.mock('../stores/zmodemStore', () => ({ useZmodemStore: () => ({}) }))
import { stageWebZmodemUpload } from './webZmodem'
import { writeSessionInput, queuedSessionWrite, queuedSessionReply, setSessionInputSizeProvider, enqueueSessionOperation } from './sessionWriter'

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('window', { __UNITERM_WEB__: { clientId: 'browser-a' } })
  vi.stubGlobal('sessionStorage', { getItem: () => null })
  vi.stubGlobal('fetch', mock.fetch)
  setSessionInputSizeProvider('drop', () => ({ cols: 80, rows: 24, generation: 1 }))
})
afterEach(() => { setSessionInputSizeProvider('drop'); vi.unstubAllGlobals() })

it('arms the uploaded stage before rz, then sends later keys without clearing it or targeting replies', async () => {
  let upload!: (response: Response) => void, target!: () => void
  mock.fetch.mockImplementationOnce(() => new Promise(resolve => { upload = resolve }))
  mock.call.mockImplementation(async method => {
    if (method === 'main.App.WebZmodemTarget') await new Promise<void>(resolve => { target = resolve })
  })
  const dropping = writeSessionInput('drop', 'rz -be\n', false, () => stageWebZmodemUpload('drop', [
    { path: '访客上传.bin', file: new File([new Uint8Array([0, 255, 42])], '访客上传.bin') },
  ]))
  queuedSessionWrite('drop', 'next')
  queuedSessionReply('drop', 'observer must not reply')
  await vi.waitFor(() => expect(upload).toBeTypeOf('function'))
  expect(mock.call).not.toHaveBeenCalled()
  upload(new Response(JSON.stringify({ id: 'stage', state: 'ready' })))
  await vi.waitFor(() => expect(target).toBeTypeOf('function'))
  expect(mock.call.mock.calls).toEqual([['main.App.WebZmodemTarget', 'drop', 'browser-a', 'stage']])
  target()
  await dropping
  await enqueueSessionOperation('drop', async () => {})
  expect(mock.call.mock.calls).toEqual([
    ['main.App.WebZmodemTarget', 'drop', 'browser-a', 'stage'],
    ['main.App.WebTerminalInput', 'drop', 'browser-a', 'rz -be\n', 80, 24, false],
    ['main.App.WebTerminalInput', 'drop', 'browser-a', 'next', 80, 24, false],
  ])
  expect(mock.reply).not.toHaveBeenCalled()
})

it('rejects directory ZMODEM drops before staging and cleans a stage cancelled before targeting', async () => {
  await expect(stageWebZmodemUpload('drop', [{ path: 'directory' }])).rejects.toThrow('individual files')
  expect(mock.fetch).not.toHaveBeenCalled()
  const abort = new AbortController()
  mock.fetch.mockImplementationOnce(async () => {
    abort.abort()
    return new Response(JSON.stringify({ id: 'cancelled', state: 'ready' }))
  }).mockResolvedValueOnce(new Response(null, { status: 204 }))
  await expect(stageWebZmodemUpload('drop', [{ path: 'file', file: new File(['x'], 'file') }], abort.signal)).rejects.toThrow()
  expect(mock.call).not.toHaveBeenCalled()
  expect(mock.fetch.mock.calls[1]).toEqual(['/web/files/cancelled', { method: 'DELETE', credentials: 'same-origin' }])
})
