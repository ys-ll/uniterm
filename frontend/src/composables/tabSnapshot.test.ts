import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

vi.hoisted(() => {
  vi.stubGlobal('window', { matchMedia: () => ({ matches: false, addEventListener: () => {} }) })
  vi.stubGlobal('document', { createElement: () => ({}), documentElement: { dataset: {}, style: {} } })
  vi.stubGlobal('localStorage', { setItem: () => {} })
})
vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) } }))
vi.mock('../i18n', () => ({ setLocale: vi.fn() }))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  LoadSettings: vi.fn(), SaveSettings: vi.fn(), LoadLocalState: vi.fn(), SaveLocalState: vi.fn(),
  LoadAIModels: vi.fn(), SaveAIModels: vi.fn(), GetAvailableShells: vi.fn(), SetDefaultSessionLogDir: vi.fn(),
}))
vi.mock('./connectionLauncher', () => ({ launchConnection: vi.fn() }))
vi.mock('./savedWorkspace', () => ({ openSavedWorkspace: vi.fn(), serializeWorkspaceState: vi.fn() }))

import { LoadSettings, LoadLocalState, SaveLocalState, LoadAIModels, GetAvailableShells, SetDefaultSessionLogDir } from '../../bindings/github.com/ys-ll/uniterm/app'
import { useSettingsStore } from '../stores/settingsStore'
import { useLocalStateStore } from '../stores/localStateStore'
import { useConnectionStore } from '../stores/connectionStore'
import { maybeRestoreLastTabs, restoreTabsSnapshot } from './tabSnapshot'
import type { RestoreDeps } from './tabSnapshot'
import { launchConnection } from './connectionLauncher'

const snapshot = {
  tabs: [{ kind: 'terminal' as const, name: 'Local', connectionId: '', type: 'local', shellPath: '/bin/zsh' }],
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(LoadLocalState).mockResolvedValue({ lastTabsSnapshot: snapshot } as any)
  vi.mocked(SaveLocalState).mockResolvedValue(undefined)
  vi.mocked(LoadAIModels).mockResolvedValue({ models: [], maxTurns: 20 } as any)
  vi.mocked(GetAvailableShells).mockResolvedValue([])
  vi.mocked(SetDefaultSessionLogDir).mockResolvedValue(undefined)
})

describe('startup tab restore', () => {
  it.each([false, true, undefined])('waits for delayed settings with restoreTabsPrompt=%s', async enabled => {
    let finishLoad!: (settings: any) => void
    vi.mocked(LoadSettings).mockImplementation(() => new Promise(resolve => { finishLoad = resolve }))
    const settings = useSettingsStore()
    const init = settings.init()
    // Reproduce App.vue mounting after local state loads but before settings.
    await useLocalStateStore().init()
    const confirm = vi.fn().mockResolvedValue(undefined)
    const deps: RestoreDeps = { connectTerminal: vi.fn(), connectMember: vi.fn() }
    const restore = maybeRestoreLastTabs(confirm, deps)
    expect(settings.settings.restoreTabsPrompt).toBe(true)
    expect(confirm).not.toHaveBeenCalled()
    expect(deps.connectTerminal).not.toHaveBeenCalled()

    finishLoad({ restoreTabsPrompt: enabled })
    await init
    await restore
    if (enabled === false) {
      expect(confirm).not.toHaveBeenCalled()
      expect(deps.connectTerminal).not.toHaveBeenCalled()
      expect(useLocalStateStore().state.lastTabsSnapshot).toEqual(snapshot)
    } else {
      expect(confirm).toHaveBeenCalledExactlyOnceWith(1)
      expect(deps.connectTerminal).toHaveBeenCalledWith(expect.objectContaining({ type: 'local', shellPath: '/bin/zsh' }), false)
    }
  })

  it('does not wait again when settings are already loaded', async () => {
    vi.mocked(LoadSettings).mockResolvedValue({ restoreTabsPrompt: false } as any)
    await useSettingsStore().init()
    const confirm = vi.fn()
    await maybeRestoreLastTabs(confirm, { connectTerminal: vi.fn(), connectMember: vi.fn() })
    expect(confirm).not.toHaveBeenCalled()
  })

  it('clears the saved snapshot when the user declines', async () => {
    vi.mocked(LoadSettings).mockResolvedValue({ restoreTabsPrompt: true } as any)
    await useSettingsStore().init()
    const deps: RestoreDeps = { connectTerminal: vi.fn(), connectMember: vi.fn() }
    await maybeRestoreLastTabs(vi.fn().mockRejectedValue('cancel'), deps)
    expect(useLocalStateStore().state.lastTabsSnapshot).toBeNull()
    expect(deps.connectTerminal).not.toHaveBeenCalled()
  })

  it('does not prompt when there are no saved tabs', async () => {
    vi.mocked(LoadSettings).mockResolvedValue({ restoreTabsPrompt: true } as any)
    vi.mocked(LoadLocalState).mockResolvedValue({ lastTabsSnapshot: { tabs: [] } } as any)
    await useSettingsStore().init()
    const confirm = vi.fn()
    await maybeRestoreLastTabs(confirm, { connectTerminal: vi.fn(), connectMember: vi.fn() })
    expect(confirm).not.toHaveBeenCalled()
  })
})

// Issue #1336: a tab renamed from the context menu must come back under its
// custom name, not under the connection's name (the launcher titles tabs from
// config.name).
describe('restore keeps renamed tab names', () => {
  const hostTab = { kind: 'terminal' as const, name: 'prod-db', connectionId: 'c1', type: 'ssh', shellPath: '' }

  beforeEach(() => {
    useConnectionStore().connections.push({
      id: 'c1', name: 'user@host', type: 'ssh', host: 'host', port: 22, user: 'user', authType: 'password',
    } as any)
  })

  it('restores the saved custom name of a host-backed tab', async () => {
    const deps: RestoreDeps = { connectTerminal: vi.fn(), connectMember: vi.fn() }
    await restoreTabsSnapshot({ tabs: [hostTab] }, deps)
    expect(launchConnection).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'c1', name: 'prod-db' }),
      { persist: false, connectTerminal: deps.connectTerminal },
    )
  })

  it('falls back to the connection name when the snapshot has no name', async () => {
    const deps: RestoreDeps = { connectTerminal: vi.fn(), connectMember: vi.fn() }
    await restoreTabsSnapshot({ tabs: [{ ...hostTab, name: '' }] }, deps)
    expect(launchConnection).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'c1', name: 'user@host' }),
      expect.anything(),
    )
  })
})
