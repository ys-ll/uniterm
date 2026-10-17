import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useLocalStateStore } from './localStateStore'

const { load, save } = vi.hoisted(() => ({ load: vi.fn(), save: vi.fn() }))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ LoadLocalState: load, SaveLocalState: save }))

function storage() {
  const values = new Map<string, string>()
  return { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value) }
}

function view(instanceId: string, localStorage: ReturnType<typeof storage>) {
  vi.stubGlobal('window', { __UNITERM_WEB__: { instanceId, clientId: 'browser' } })
  vi.stubGlobal('localStorage', localStorage)
  setActivePinia(createPinia())
  return useLocalStateStore()
}

beforeEach(() => { load.mockReset(); save.mockReset() })
afterEach(() => vi.unstubAllGlobals())

describe('browser-local state isolation', () => {
  it('never restores desktop IDs or writes native localState when browser preferences change', async () => {
    const disk = storage()
    disk.setItem('uniterm:web:host:view', JSON.stringify({ sidebarWidth: 317, windowWidth: 999, mcp: { enabled: true }, lastTabsSnapshot: { tabs: [{ id: 'desktop-panel' }] } }))
    const store = view('host', disk)
    await store.init()
    expect(store.state.sidebarWidth).toBe(317)
    expect(store.state.windowWidth).toBe(0)
    expect(store.state.lastTabsSnapshot).toBeNull()
    await store.update({ sidebarVisible: false, windowWidth: 1234, systemTitleBar: true, lastTabsSnapshot: { tabs: [] } })
    const saved = JSON.parse(disk.getItem('uniterm:web:host:view')!)
    expect(saved.sidebarVisible).toBe(false)
    expect(saved).not.toHaveProperty('windowWidth')
    expect(saved).not.toHaveProperty('lastTabsSnapshot')
    expect(saved).not.toHaveProperty('mcp')
    expect(store.state.systemTitleBar).toBe(false)
    expect(load).not.toHaveBeenCalled()
    expect(save).not.toHaveBeenCalled()
  })

  it('survives refresh without sharing preferences across browsers or host instances', async () => {
    const browserA = storage()
    const browserB = storage()
    await view('host-1', browserA).update({ sidebarWidth: 333, collapsedGroupIds: ['group-a'] })
    let store = view('host-1', browserB)
    await store.init()
    expect(store.state.sidebarWidth).toBe(240)
    await store.update({ sidebarWidth: 444 })
    store = view('host-2', browserA)
    await store.init()
    expect(store.state.sidebarWidth).toBe(240)
    store.state.collapsedGroupIds.push('host-2-only')
    const fresh = view('host-3', browserA)
    await fresh.init()
    expect(fresh.state.collapsedGroupIds).toEqual([])
    store = view('host-1', browserA)
    await store.init()
    expect(store.state.sidebarWidth).toBe(333)
    expect(store.state.collapsedGroupIds).toEqual(['group-a'])
    expect(save).not.toHaveBeenCalled()
  })

  it('keeps native persistence and rejects corrupt browser-local values', async () => {
    const disk = storage()
    disk.setItem('uniterm:web:host:view', JSON.stringify({ sidebarWidth: -20, collapsedGroupIds: [123], aiSidebarVisible: 'yes' }))
    let store = view('host', disk)
    await store.init()
    expect(store.state.sidebarWidth).toBe(240)
    expect(store.state.collapsedGroupIds).toEqual([])
    expect(store.state.aiSidebarVisible).toBe(true)
    vi.stubGlobal('window', {})
    setActivePinia(createPinia())
    load.mockResolvedValue({ windowWidth: 1200, sidebarVisible: false })
    store = useLocalStateStore()
    await store.init()
    await store.update({ windowWidth: 1400 })
    expect(load).toHaveBeenCalledOnce()
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ windowWidth: 1400, sidebarVisible: false }))
  })
})
