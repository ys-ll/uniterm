import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useTunnelStore, type Tunnel, type TunnelState } from './tunnelStore'

const backend = vi.hoisted(() => ({
  LoadTunnels: vi.fn(),
  SaveTunnels: vi.fn(),
  StartTunnel: vi.fn(),
  StopTunnel: vi.fn(),
  ListTunnelStates: vi.fn(),
}))
const onEvent = vi.hoisted(() => vi.fn())

vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => backend)
vi.mock('../services/businessConfig', () => ({ LoadTunnels: backend.LoadTunnels, SaveTunnels: backend.SaveTunnels, configChanged: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Events: { On: onEvent } }))
vi.mock('../services/message', () => ({ msg: { error: vi.fn() } }))
vi.mock('../i18n', () => ({ t: (key: string) => key }))

const tunnel: Tunnel = {
  id: 'tunnel', name: 'Database', mode: 'local', sshConnId: 'ssh', listenPort: 13306,
  targetHost: 'localhost', targetPort: 3306, autoReconnect: true, reconnectInterval: 120,
  reconnectJitter: 20,
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

describe('tunnel lifecycle', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.resetAllMocks()
    backend.StopTunnel.mockResolvedValue(undefined)
    backend.SaveTunnels.mockResolvedValue(undefined)
    backend.LoadTunnels.mockResolvedValue({ groups: [], tunnels: [tunnel] })
    backend.ListTunnelStates.mockResolvedValue([])
    onEvent.mockReturnValue(() => {})
  })

  afterEach(() => vi.restoreAllMocks())

  it('allows stopping connecting and reconnecting tunnels, and preserves their status labels', () => {
    const store = useTunnelStore()
    for (const status of ['connecting', 'running', 'reconnecting'] as const) {
      store.states[tunnel.id] = { id: tunnel.id, status }
      expect(store.isActive(tunnel.id)).toBe(true)
    }
    store.states[tunnel.id].error = 'connection lost'
    expect(store.statusTitle(tunnel.id)).toBe('tunnels.statusReconnecting: connection lost')
    store.states[tunnel.id].status = 'stopped'
    expect(store.isActive(tunnel.id)).toBe(false)
  })

  it('loads and saves the configured reconnect interval and random variation', async () => {
    const store = useTunnelStore()
    await store.load()
    expect(store.tunnels[0]).toMatchObject({ autoReconnect: true, reconnectInterval: 120, reconnectJitter: 20 })
    await store.save()
    expect(backend.SaveTunnels).toHaveBeenCalledWith({ version: 1, groups: [], tunnels: [tunnel] })
    store.dispose()
  })

  it('subscribes before the initial snapshot and keeps events newer than that snapshot', async () => {
    const snapshot = deferred<TunnelState[]>()
    backend.ListTunnelStates.mockReturnValueOnce(snapshot.promise)
    const store = useTunnelStore()
    const firstLoad = store.load()
    const secondLoad = store.load()
    await Promise.resolve()
    expect(backend.LoadTunnels).toHaveBeenCalledTimes(1)
    expect(backend.ListTunnelStates).toHaveBeenCalledTimes(1)
    expect(onEvent).toHaveBeenCalledTimes(2)
    const stateEvent = onEvent.mock.calls.find(([name]) => name === 'tunnel:state')![1]
    stateEvent({ data: { id: tunnel.id, status: 'running' } })
    snapshot.resolve([
      { id: tunnel.id, status: 'connecting' },
      { id: 'other', status: 'reconnecting' },
    ])
    await Promise.all([firstLoad, secondLoad])
    expect(store.statusOf(tunnel.id)).toBe('running')
    expect(store.statusOf('other')).toBe('reconnecting')
    await store.load()
    expect(onEvent).toHaveBeenCalledTimes(2)
    store.dispose()
  })

  it('discards a snapshot completed after dispose and allows a fresh subscription', async () => {
    const snapshot = deferred<TunnelState[]>()
    const unsubscribe = vi.fn()
    onEvent.mockReturnValue(unsubscribe)
    backend.ListTunnelStates.mockReturnValueOnce(snapshot.promise)
    const store = useTunnelStore()
    const firstLoad = store.load()
    await Promise.resolve()
    store.dispose()
    expect(unsubscribe).toHaveBeenCalledTimes(2)
    await store.load()
    snapshot.resolve([{ id: tunnel.id, status: 'running' }])
    await firstLoad
    expect(store.statusOf(tunnel.id)).toBe('stopped')
    expect(store.loaded).toBe(true)
    expect(onEvent).toHaveBeenCalledTimes(4)
    store.dispose()
  })

  it('ignores an old start response after stop and a new start', async () => {
    const first = deferred<TunnelState>()
    const second = deferred<TunnelState>()
    backend.StartTunnel.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const store = useTunnelStore()
    const firstStart = store.start(tunnel.id)
    expect(store.statusOf(tunnel.id)).toBe('connecting')
    await store.start(tunnel.id)
    expect(backend.StartTunnel).toHaveBeenCalledTimes(1)
    await store.stop(tunnel.id)
    const secondStart = store.start(tunnel.id)
    first.resolve({ id: tunnel.id, status: 'running' })
    await firstStart
    expect(store.statusOf(tunnel.id)).toBe('connecting')
    second.resolve({ id: tunnel.id, status: 'reconnecting', error: 'offline' })
    await secondStart
    expect(store.statusOf(tunnel.id)).toBe('reconnecting')
  })

  it('waits for cancellation before deleting and saving a reconnecting tunnel', async () => {
    const stopped = deferred<void>()
    backend.StopTunnel.mockReturnValueOnce(stopped.promise)
    const store = useTunnelStore()
    store.tunnels = [{ ...tunnel }]
    store.states[tunnel.id] = { id: tunnel.id, status: 'reconnecting' }
    const deletion = store.deleteTunnel(tunnel.id)
    expect(store.tunnels).toHaveLength(1)
    expect(backend.SaveTunnels).not.toHaveBeenCalled()
    stopped.resolve()
    await deletion
    expect(store.tunnels).toHaveLength(0)
    expect(store.statusOf(tunnel.id)).toBe('stopped')
    expect(backend.SaveTunnels).toHaveBeenCalledWith({ version: 1, groups: [], tunnels: [] })
  })

  it('keeps the latest backend event when start and stop responses arrive late', async () => {
    const started = deferred<TunnelState>()
    const stopped = deferred<void>()
    backend.StartTunnel.mockReturnValueOnce(started.promise)
    backend.StopTunnel.mockReturnValueOnce(stopped.promise)
    const store = useTunnelStore()
    await store.load()
    const stateEvent = onEvent.mock.calls.find(([name]) => name === 'tunnel:state')![1]
    const starting = store.start(tunnel.id)
    stateEvent({ data: { id: tunnel.id, status: 'running' } })
    stateEvent({ data: { id: tunnel.id, status: 'reconnecting', error: 'offline' } })
    started.resolve({ id: tunnel.id, status: 'running' })
    await starting
    expect(store.statusOf(tunnel.id)).toBe('reconnecting')
    const stopping = store.stop(tunnel.id)
    stateEvent({ data: { id: tunnel.id, status: 'stopped' } })
    // Another window starts the same tunnel before this StopTunnel returns.
    stateEvent({ data: { id: tunnel.id, status: 'connecting' } })
    stopped.resolve()
    await stopping
    expect(store.statusOf(tunnel.id)).toBe('connecting')
    store.dispose()
  })

  it('keeps a tunnel and its group when cancellation fails', async () => {
    backend.StopTunnel.mockRejectedValue(new Error('stop failed'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useTunnelStore()
    store.tunnels = [{ ...tunnel, groupId: 'group' }]
    store.groups = [{ id: 'group', name: 'Servers' }]
    await store.deleteTunnel(tunnel.id)
    await store.deleteGroup('group', true)
    expect(store.tunnels).toHaveLength(1)
    expect(store.groups).toHaveLength(1)
    expect(backend.SaveTunnels).not.toHaveBeenCalled()
  })
})
