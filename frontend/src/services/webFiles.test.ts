import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { call } = vi.hoisted(() => ({ call: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Call: { ByName: call } }))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({}))
import { deviceDropEntries, dismissFileStage, downloadFileStage, fileRPC, pickPersistentHostCopy, saveTextFile, stageDeviceFiles, validateDevicePath, waitFileStage } from './webFiles'

describe('visitor device file adapter', () => {
  const fetchMock = vi.fn()
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('window', { __UNITERM_WEB__: { clientId: 'browser-a' } })
  })
  afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })

  it('streams File parts with relative sidecar names and a native abort signal', async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ id: 'opaque', state: 'ready' })))
    const ctl = new AbortController()
    const file = new File([new Uint8Array([0,255,3])], 'user.sessions')
    await stageDeviceFiles([{ path: 'profile/terminal/user.sessions', file }, { path: 'profile/empty' }], ctl.signal)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/web/files/upload')
    expect(init.signal).toBe(ctl.signal)
    expect(init.body).toBeInstanceOf(FormData)
    expect(Array.from(init.body.keys())).toEqual(['profile/terminal/user.sessions', 'profile/empty/'])
    expect(Array.from(new Uint8Array(await init.body.get('profile/terminal/user.sessions').arrayBuffer()))).toEqual([0,255,3])
  })

  it('rejects traversal and reserved filenames before a request', async () => {
    for (const path of ['../bad', '/absolute', 'a//b', 'C:/bad', 'a\\b', 'CON.txt', 'a/../b', 'file:stream']) {
      expect(() => validateDevicePath(path)).toThrow()
    }
    await expect(stageDeviceFiles([{ path: '../bad', file: new File(['x'], 'bad') }])).rejects.toThrow()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('waits for remote completion and surfaces remote failures', async () => {
    vi.useFakeTimers()
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ id: 'd', state: 'transferring' })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ id: 'd', state: 'ready' })))
    const waiting = waitFileStage('d')
    let finished = false
    void waiting.then(() => { finished = true })
    await vi.advanceTimersByTimeAsync(299)
    expect(finished).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    expect((await waiting).state).toBe('ready')
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ state: 'error', error: 'disk full' })))
    await expect(waitFileStage('d')).rejects.toThrow('disk full')
  })

  it('hands a ready HTTP URL to the download manager without fetching a Blob', () => {
    const link = { href: '', download: '', click: vi.fn(), remove: vi.fn() }
    vi.stubGlobal('document', { createElement: vi.fn(() => link), body: { append: vi.fn() } })
    expect(() => downloadFileStage({ id: 'd', state: 'transferring', files: [] })).toThrow('not ready')
    downloadFileStage({ id: 'd', state: 'ready', files: [] })
    expect(link.href).toBe('/web/files/d/download')
    expect(link.click).toHaveBeenCalledOnce()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('enumerates every directory reader batch and retains empty directories', async () => {
    const file = new File(['bytes'], 'child')
    const child = { name: 'child', isFile: true, file: (resolve: (f: File) => void) => resolve(file) }
    const empty = { name: 'empty', isFile: false, createReader: () => ({ readEntries: (resolve: (e: any[]) => void) => resolve([]) }) }
    const batches = [[child], [empty], []]
    const root = { name: 'root', isFile: false, createReader: () => ({ readEntries: (resolve: (e: any[]) => void) => resolve(batches.shift()!) }) }
    const entries = await deviceDropEntries({ items: [{ webkitGetAsEntry: () => root }], files: [] } as unknown as DataTransfer)
    expect(entries.map(e => e.path)).toEqual(['root', 'root/child', 'root/empty'])
  })

  it('persists a saved picker path before dismissing its temporary stage', async () => {
    const listeners: Record<string, () => void> = {}
    const input = { style: {}, files: [new File(['sqlite'], 'db.sqlite')], setAttribute() {}, remove() {},
      addEventListener: (name: string, handler: () => void) => { listeners[name] = handler }, click: () => listeners.change() }
    vi.stubGlobal('document', { createElement: () => input, body: { append() {} } })
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ id: 'stage', state: 'ready' }))).mockResolvedValueOnce(new Response(null, { status: 204 }))
    call.mockResolvedValue('/host/data/web-files/saved/db.sqlite')
    expect(await pickPersistentHostCopy()).toBe('/host/data/web-files/saved/db.sqlite')
    expect(call).toHaveBeenCalledWith('main.App.WebFilesPersist', 'stage', 'db.sqlite')
    expect(fetchMock.mock.calls[1]).toEqual(['/web/files/stage', { method: 'DELETE', credentials: 'same-origin' }])
  })

  it('uses exact Wails ByName resolution and tolerates an expired dismiss', async () => {
    call.mockResolvedValue({ id: 'd' })
    await fileRPC('WebFilesGet', 'session', '/remote', 'file', false)
    expect(call).toHaveBeenCalledWith('main.App.WebFilesGet', 'session', '/remote', 'file', false)
    fetchMock.mockResolvedValue(new Response('', { status: 404 }))
    await expect(dismissFileStage('gone')).resolves.toBeUndefined()
  })

  it('exports text as ordinary HTTP bytes without base64, JSON, or Blob copies', async () => {
    const link = { href: '', download: '', click: vi.fn(), remove: vi.fn() }
    vi.stubGlobal('document', { createElement: () => link, body: { append() {} } })
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ id: 'export', state: 'ready' })))
    await saveTextFile('select "中文";', 'table.sql')
    expect(fetchMock.mock.calls[0][1].body).toBe('select "中文";')
    expect(link.href).toBe('/web/files/export/download')
    expect(link.click).toHaveBeenCalledOnce()
  })
})
