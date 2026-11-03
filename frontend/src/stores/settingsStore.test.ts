import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { Events } from '@wailsio/runtime'

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
import { LoadSettings, SaveSettings, LoadLocalState, SaveLocalState, LoadAIModels, SaveAIModels, GetAvailableShells, SetDefaultSessionLogDir } from '../../bindings/github.com/ys-ll/uniterm/app'
import { useSettingsStore } from './settingsStore'
import { useLocalStateStore } from './localStateStore'

const models = ['a', 'b'].map(id => ({ id, name: id, model: id, baseURL: 'https://example.com', apiKey: '', protocol: 'responses' }))
beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(LoadSettings).mockResolvedValue({ ai: { maxTurns: 20, models, activeModelId: 'b' } } as any)
  vi.mocked(LoadLocalState).mockResolvedValue({ sidebarVisible: false } as any)
  vi.mocked(LoadAIModels).mockResolvedValue({ models, maxTurns: 20 } as any)
  vi.mocked(SaveLocalState).mockResolvedValue(undefined)
  vi.mocked(SaveSettings).mockResolvedValue(undefined)
  vi.mocked(GetAvailableShells).mockResolvedValue([])
  vi.mocked(SetDefaultSessionLogDir).mockResolvedValue(undefined)
})

describe('active AI model migration', () => {
  it('saves the old choice locally before removing it from settings', async () => {
    const store = useSettingsStore()
    await store.init()
    expect(store.activeModelId).toBe('b')
    expect(useLocalStateStore().state.sidebarVisible).toBe(false)
    expect(vi.mocked(SaveLocalState).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(SaveSettings).mock.invocationCallOrder[0])
    expect(vi.mocked(SaveSettings).mock.calls[0][0].ai).not.toHaveProperty('activeModelId')
    vi.mocked(SaveSettings).mockClear()
    await store.setActiveModel('a')
    expect(store.activeModelId).toBe('a')
    expect(SaveSettings).not.toHaveBeenCalled()
    expect(SaveAIModels).not.toHaveBeenCalled()
  })

  it('prefers the existing local choice', async () => {
    vi.mocked(LoadLocalState).mockResolvedValue({ ai: { activeModelId: 'a', modelPreferences: {} } } as any)
    await useSettingsStore().init()
    expect(useSettingsStore().activeModelId).toBe('a')
  })

  it('preserves legacy settings when the local migration cannot be saved', async () => {
    vi.mocked(SaveLocalState).mockRejectedValueOnce(new Error('disk full'))
    await expect(useSettingsStore().init()).rejects.toThrow('Could not save AI selection')
    expect(SaveSettings).not.toHaveBeenCalled()
  })

  it('defers migration when settings cannot be read until unlock', async () => {
    vi.mocked(LoadSettings).mockRejectedValueOnce(new Error('locked'))
    const store = useSettingsStore()
    await store.init()
    expect(SaveLocalState).not.toHaveBeenCalled()
    await store.reload()
    expect(store.activeModelId).toBe('b')
  })
})

describe('AI context preferences', () => {
  it('defaults legacy configurations to automatic 96K compaction with the current model', async () => {
    const store = useSettingsStore()
    await store.init()
    expect(store.ai).toMatchObject({ autoCompact: true, contextTokenBudget: 96000, summaryModelId: '' })
  })

  it('persists context settings to ai.json and restores them in a fresh store', async () => {
    const store = useSettingsStore()
    await store.init()
    Object.assign(store.settings.ai, { autoCompact: false, contextTokenBudget: 512000, summaryModelId: 'a' })
    await store.save()
    const saved = JSON.parse(JSON.stringify(vi.mocked(SaveAIModels).mock.calls.slice(-1)[0][0]))
    expect(saved).toMatchObject({ autoCompact: false, contextTokenBudget: 512000, summaryModelId: 'a' })
    vi.mocked(LoadAIModels).mockResolvedValue(saved)
    setActivePinia(createPinia())
    const restarted = useSettingsStore()
    await restarted.init()
    expect(restarted.ai).toMatchObject({ autoCompact: false, contextTokenBudget: 512000, summaryModelId: 'a' })
  })

  it('resets the summary model selection when the selected model is removed', async () => {
    const store = useSettingsStore()
    await store.init()
    store.settings.ai.summaryModelId = 'a'
    store.removeModel('a')
    expect(store.settings.ai.summaryModelId).toBe('')
  })

  it('applies context preferences received through cloud sync', async () => {
    const store = useSettingsStore()
    await store.init()
    const listener = vi.mocked(Events.On).mock.calls.find(call => call[0] === 'store:ai:changed')![1]
    listener({ data: { models, maxTurns: 20, autoCompact: false, contextTokenBudget: 1000000, summaryModelId: 'b' } } as any)
    expect(store.ai).toMatchObject({ autoCompact: false, contextTokenBudget: 1000000, summaryModelId: 'b' })
  })
})
