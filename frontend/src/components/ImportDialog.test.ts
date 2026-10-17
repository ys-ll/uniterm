import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { Ref } from 'vue'
import type { ConnectionConfig, ConnectionGroup } from '../types/session'

const backend = vi.hoisted(() => ({
  load: vi.fn(), save: vi.fn(), parse: vi.fn(), apply: vi.fn(),
  openFile: vi.fn(), pickDevice: vi.fn(), withDevicePaths: vi.fn(), fileRPC: vi.fn(),
  success: vi.fn(), error: vi.fn(), warning: vi.fn(), copyable: vi.fn(),
  listeners: new Set<() => void>(),
}))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  LoadBusinessConfig: backend.load, SaveBusinessConfig: backend.save,
  ParseImportFile: backend.parse, ApplyImport: backend.apply,
  OpenFileDialogFiltered: backend.openFile, OpenDirectoryDialog: vi.fn(),
}))
vi.mock('@wailsio/runtime', () => ({ Events: {
  On: (name: string, callback: () => void) => {
    expect(name).toBe('store:connections:changed')
    backend.listeners.add(callback)
    return () => backend.listeners.delete(callback)
  },
} }))
vi.mock('../services/message', () => ({ msg: {
  success: backend.success, error: backend.error, warning: backend.warning, copyable: backend.copyable,
} }))
vi.mock('../services/webFiles', () => ({
  pickDeviceFiles: backend.pickDevice, withDevicePaths: backend.withDevicePaths, fileRPC: backend.fileRPC,
}))

interface StoreData { groups: ConnectionGroup[]; connections: ConnectionConfig[] }
interface DialogState { pickFile(): Promise<void>; onImport(): Promise<void>; importing: Ref<boolean> }
let stored: StoreData
let revision: number
let imported: ConnectionConfig
const cleanups: (() => void)[] = []
const snapshot = () => ({ revision: `r${revision}`, json: JSON.stringify(stored) })
const broadcast = () => { for (const callback of backend.listeners) callback() }

// Separate module instances model two JS clients: their stores and guarded
// revision state are independent, while the mocked backend storage is shared.
async function client() {
  vi.resetModules()
  const { createPinia, setActivePinia } = await import('pinia')
  const { createApp, effectScope, ssrContextKey } = await import('vue')
  const config = await import('../services/businessConfig')
  const { useConnectionStore } = await import('../stores/connectionStore')
  const { default: ImportDialog } = await import('./ImportDialog.vue')
  setActivePinia(createPinia())
  const store = useConnectionStore()
  await store.load()
  const emit = vi.fn()
  const scope = effectScope()
  const app = createApp({})
  app.provide(ssrContextKey, {})
  // Run the real SFC setup/handler and stores with external I/O mocked.
  const dialog = app.runWithContext(() => scope.run(() => ImportDialog.setup!({ visible: true }, {
    attrs: {}, slots: {}, emit, expose: () => {},
  }))) as unknown as DialogState
  cleanups.push(() => { scope.stop(); store.dispose() })
  return { store, config, dialog, emit }
}

beforeEach(() => {
  vi.resetAllMocks()
  backend.listeners.clear()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  revision = 1
  stored = {
    groups: [{ id: 'existing-group', name: 'Servers' }],
    connections: [{ id: 'existing', name: 'Existing', type: 'ssh', host: 'existing.test', port: 22, user: 'test', authType: 'password' }],
  }
  imported = { ...stored.connections[0], id: 'imported', name: 'Imported', host: 'imported.test', groupId: 'parsed-group' }
  backend.parse.mockResolvedValue({ groups: [{ id: 'parsed-group', name: 'Servers' }], connections: [imported], warnings: [] })
  backend.openFile.mockResolvedValue('/host/import.utm')
  backend.pickDevice.mockResolvedValue([{ path: 'import.utm', file: new File(['fixture'], 'import.utm') }])
  backend.withDevicePaths.mockImplementation(async (_entries, fn) => fn({ id: 'staged-import' }))
  backend.fileRPC.mockResolvedValue('/staged/import.utm')
  backend.load.mockImplementation(async (kind) => {
    expect(kind).toBe('connections')
    return snapshot()
  })
  backend.apply.mockImplementation(async (data: StoreData) => {
    // Like ApplyImport's server-side merge, reuse the existing group by path.
    stored.connections.push(...data.connections.map(c => ({ ...c, groupId: 'existing-group' })))
    revision++
    broadcast()
  })
  backend.save.mockImplementation(async (kind, expected, json) => {
    expect(kind).toBe('connections')
    if (expected !== `r${revision}`) throw new Error('CONFIG_CONFLICT')
    stored = JSON.parse(json)
    revision++
    broadcast()
    return snapshot()
  })
})

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it.each([false, true])('refreshes the importing client and revision without rebasing another client (web=%s)', async web => {
  vi.stubGlobal('window', web ? { __UNITERM_WEB__: { instanceId: 'host', clientId: 'visitor' } } : {})
  const other = await client()
  other.store.connections[0].name = 'Unsaved peer edit'
  const initiator = await client()
  await initiator.dialog.pickFile()
  await initiator.dialog.onImport()

  expect(backend.apply).toHaveBeenCalledExactlyOnceWith({
    groups: [{ id: 'parsed-group', name: 'Servers' }], connections: [imported],
  })
  expect(backend.parse).toHaveBeenCalledWith('uniterm', web ? '/staged/import.utm' : '/host/import.utm', '')
  if (web) expect(backend.fileRPC).toHaveBeenCalledWith('WebFilesPath', 'staged-import', 'import.utm')
  else expect(backend.withDevicePaths).not.toHaveBeenCalled()
  expect.soft(initiator.store.connections).toEqual(stored.connections)
  expect.soft(initiator.store.groups).toEqual(stored.groups)

  await initiator.store.update('existing', { name: 'Saved after import' })
  expect(backend.save).toHaveBeenLastCalledWith('connections', 'r2', expect.any(String))
  expect(stored.connections.map(c => c.name)).toEqual(['Saved after import', 'Imported'])
  expect(stored.connections[1].groupId).toBe('existing-group')
  expect(initiator.emit).toHaveBeenCalledWith('done', 1)
  expect(initiator.emit).toHaveBeenCalledWith('update:visible', false)
  expect(backend.success).toHaveBeenCalledTimes(1)

  expect(other.store.connections.map(c => c.name)).toEqual(['Unsaved peer edit'])
  expect(backend.copyable).toHaveBeenCalledWith(expect.stringContaining('configuration changed elsewhere'), 'warning')
  await expect(other.config.SaveConnections({
    groups: other.store.groups, connections: other.store.connections,
  })).rejects.toThrow('CONFIG_CONFLICT')
  expect(backend.save).toHaveBeenLastCalledWith('connections', 'r1', expect.any(String))
  expect(stored.connections.map(c => c.name)).toEqual(['Saved after import', 'Imported'])
  expect(other.store.connections[0].name).toBe('Unsaved peer edit')
  expect(backend.copyable).toHaveBeenCalledWith(expect.stringContaining('CONFIG_CONFLICT'), 'error')
})

it('does not reload local edits or report success when ApplyImport fails', async () => {
  vi.stubGlobal('window', {})
  const initiator = await client()
  initiator.store.connections[0].name = 'Unsaved local edit'
  backend.apply.mockRejectedValueOnce(new Error('import write failed'))
  backend.load.mockClear()
  await initiator.dialog.onImport()
  expect(backend.load).not.toHaveBeenCalled()
  expect(initiator.store.connections[0].name).toBe('Unsaved local edit')
  expect(backend.error).toHaveBeenCalledWith('import write failed')
  expect(backend.success).not.toHaveBeenCalled()
  expect(initiator.emit).not.toHaveBeenCalled()
  expect(initiator.dialog.importing.value).toBe(false)
})
