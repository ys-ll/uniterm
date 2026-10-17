import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

// Real beta.27 Events and Call modules; only DOM/network boundaries are faked.
// This catches dispatching into a second runtime or accidentally using Emit.
class TestEventSource extends EventTarget {
  static all: TestEventSource[] = []
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  constructor(public url: string) { super(); TestEventSource.all.push(this) }
  close() { this.closed = true }
  ready(sequence: number) {
    this.dispatchEvent(new MessageEvent('ready', { data: JSON.stringify({ sequence, resnapshot: true }) }))
  }
  message(value: unknown) { this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(value) })) }
}

let runtime: typeof import('@wailsio/runtime')
let browser: typeof import('./browserRuntime')
const fetchMock = vi.fn()
const location = { origin: 'http://192.0.2.1:34116', replace: vi.fn(), reload: vi.fn() }
const bootstrap = { instanceId: 'instance-a', clientId: 'browser-a', ready: true, eventsUrl: '/web/events', capabilities: { terminal: true } }

beforeAll(async () => {
  vi.stubGlobal('window', Object.assign(new EventTarget(), { location, setInterval: vi.fn(), clearInterval: vi.fn() }))
  vi.stubGlobal('document', Object.assign(new EventTarget(), { documentElement: new EventTarget(), readyState: 'loading', createElement: () => ({}) }))
  vi.stubGlobal('MouseEvent', class extends Event { buttons = 0 })
  vi.stubGlobal('fetch', fetchMock.mockResolvedValue(new Response('', { status: 404 })))
  vi.stubGlobal('EventSource', TestEventSource)
  runtime = await import('@wailsio/runtime')
  browser = await import('./browserRuntime')
})

beforeEach(() => {
  TestEventSource.all = []
  location.replace.mockClear()
  location.reload.mockClear()
  window.__UNITERM_WEB__ = { instanceId: '', clientId: '' }
  fetchMock.mockReset().mockImplementation(async (url: string | URL) => {
    if (String(url) === '/web/bootstrap') return Response.json(bootstrap)
    return Response.json({ ansi: 'snapshot' })
  })
})

afterEach(() => { browser.stopBrowserEvents(); runtime.Events.OffAll() })
afterAll(() => vi.unstubAllGlobals())

describe('browser bootstrap and event bridge', () => {
  it('authenticates and establishes SSE before the first real Wails snapshot RPC', async () => {
    const data = await browser.loadWebBootstrap()
    const pending = browser.startBrowserEvents(data.eventsUrl)
    const snapshot = browser.waitForBrowserEvents().then(() => runtime.Call.ByName('main.App.GetWebTerminalSnapshot', 'existing-id'))
    await Promise.resolve()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(window.__UNITERM_WEB__).toEqual({ instanceId: 'instance-a', clientId: 'browser-a' })
    expect(TestEventSource.all[0].url).toBe('/web/events')
    TestEventSource.all[0].ready(12)
    await pending
    await snapshot
    const [url, request] = fetchMock.mock.calls[1]
    expect(String(url)).toBe(`${location.origin}/wails/runtime`)
    expect(JSON.parse(request.body)).toMatchObject({ object: 0, method: 0, args: { methodName: 'main.App.GetWebTerminalSnapshot', args: ['existing-id'] } })
    expect(request.headers['x-wails-client-id']).toBeTruthy()
  })

  it('delivers object and legacy array data to real Events.On without echoing HTTP', async () => {
    const pending = browser.startBrowserEvents('/web/events')
    const stream = TestEventSource.all[0]
    stream.ready(0)
    await pending
    const seen: unknown[] = []
    runtime.Events.On('session:data', event => seen.push(event.data))
    const data = { id: 'shared', data: 'output', webSequence: 21 }
    stream.message({ name: 'session:data', data, sequence: 1, sender: 'desktop' })
    stream.message({ name: 'session:data', data: [data], sequence: 2 })
    expect(seen).toEqual([data, [data]])
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('signals gaps before live data and waits for a new ready on disconnect', async () => {
    const order: string[] = []
    runtime.Events.On('web:resync', event => order.push(`resync:${event.data.reason}`))
    runtime.Events.On('web:disconnected', () => order.push('offline'))
    runtime.Events.On('session:data', () => order.push('data'))
    const pending = browser.startBrowserEvents('/web/events')
    const stream = TestEventSource.all[0]
    stream.ready(9)
    await pending
    stream.message({ name: 'session:data', data: { id: 'shared' }, sequence: 11 })
    stream.message({ name: 'session:data', data: {}, sequence: 11 })
    stream.onerror?.()
    let snapshotAllowed = false
    const resumed = browser.waitForBrowserEvents().then(() => { snapshotAllowed = true })
    await Promise.resolve()
    expect(snapshotAllowed).toBe(false)
    stream.ready(30)
    await resumed
    expect(order).toEqual(['resync:ready', 'resync:gap', 'data', 'offline', 'resync:reconnect'])
    expect(browser.browserConnection.connected).toBe(true)
    browser.stopBrowserEvents()
    expect(stream.closed).toBe(true)
    expect(fetchMock.mock.calls.every(([url]) => String(url) === '/web/bootstrap')).toBe(true)
  })

  it('reopens a malformed stream and requests a fresh snapshot instead of dropping the gap', async () => {
    const resync = vi.fn()
    runtime.Events.On('web:resync', resync)
    const pending = browser.startBrowserEvents('/web/events')
    TestEventSource.all[0].ready(0)
    await pending
    TestEventSource.all[0].message({ unexpected: true })
    expect(TestEventSource.all[0].closed).toBe(true)
    expect(browser.browserConnection.connected).toBe(false)
    TestEventSource.all[1].ready(45)
    expect(resync).toHaveBeenCalledTimes(2)
    expect(resync.mock.calls[1][0].data.reason).toBe('reconnect')
  })

  it('redirects expired authentication without opening any stream', async () => {
    fetchMock.mockResolvedValue(new Response('', { status: 401 }))
    await expect(browser.loadWebBootstrap()).rejects.toThrow('Sign in')
    expect(location.replace).toHaveBeenCalledWith('/web/login')
    expect(TestEventSource.all).toHaveLength(0)
  })

  it('rejects non-ready hosts and cross-origin or token-bearing event URLs', async () => {
    for (const change of [{ ready: false }, { eventsUrl: 'https://elsewhere.example/web/events' }, { eventsUrl: '/web/events?token=secret' }]) {
      fetchMock.mockResolvedValueOnce(Response.json({ ...bootstrap, ...change }))
      await expect(browser.loadWebBootstrap()).rejects.toThrow()
    }
    expect(TestEventSource.all).toHaveLength(0)
  })

  it('uses verified named bindings for Web settings and keeps tokens in the RPC body', async () => {
    const { configureWebAccess, generateWebAccessToken, getWebAccessStatus } = await import('../services/webAccess')
    await getWebAccessStatus()
    await generateWebAccessToken()
    await configureWebAccess({ enabled: true, host: '0.0.0.0', startPort: 34116 }, 'entered-secret')
    const requests = fetchMock.mock.calls.map(([url, request]) => {
      expect(String(url)).toBe(`${location.origin}/wails/runtime`)
      return JSON.parse(request.body).args
    })
    expect(requests.map(r => r.methodName)).toEqual(['main.App.GetWebAccessStatus', 'main.App.GenerateWebAccessToken', 'main.App.ConfigureWebAccess'])
    expect(requests[2].args[1]).toBe('entered-secret')
    await configureWebAccess({ enabled: false, host: '127.0.0.1', startPort: 34116 }, '')
    expect(JSON.parse(fetchMock.mock.calls[3][1].body).args.args[1]).toBe('')
  })
})
