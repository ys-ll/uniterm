import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import type { AppSettings, AIModelConfig, CustomTerminalTheme } from '../types/settings'
import { DEFAULT_SETTINGS, normalizeKeyBindings, normalizeContextTokenBudget } from '../types/settings'
import { SaveSettings, LoadSettings, GetAvailableShells, SetDefaultSessionLogDir } from '../../bindings/github.com/ys-ll/uniterm/app'
import { Events } from '@wailsio/runtime'
import { setLocale } from '../i18n'
import { useAIConfigStore } from './aiConfigStore'
import { useLocalStateStore } from './localStateStore'

// Module-level un-subscriber for the cross-window store:settings:changed listener.
// Tracked at module scope so re-imports under HMR can detach the previous
// listener before re-subscribing (FE-03).
let unsubSettingsChanged: (() => void) | null = null
let unsubAiChanged: (() => void) | null = null

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<AppSettings>({ ...DEFAULT_SETTINGS })
  const loaded = ref(false)
  const availableShells = ref<string[]>([])

  const theme = computed(() => settings.value.theme)
  const language = computed(() => settings.value.language)
  const terminal = computed(() => settings.value.terminal)
  const ai = computed(() => settings.value.ai)

  // Tracks the OS color-scheme preference so `resolvedAppTheme` stays reactive
  // to live system changes while the app theme is set to 'system'.
  const systemPrefersDark = ref(window.matchMedia('(prefers-color-scheme: dark)').matches)
  // The app theme collapsed to a concrete light/dark choice (never 'system').
  // Used by the terminal to resolve FOLLOW_APP_THEME.
  const resolvedAppTheme = computed<'dark' | 'light'>(() => {
    if (settings.value.theme === 'light') return 'light'
    if (settings.value.theme === 'system') return systemPrefersDark.value ? 'dark' : 'light'
    // 'dark' and 'deep-blue' are both dark.
    return 'dark'
  })

  const activeModel = computed(() =>
    settings.value.ai.models.find(m => m.id === useLocalStateStore().state.ai?.activeModelId) || settings.value.ai.models[0]
  )
  const activeModelId = computed(() => activeModel.value?.id || '')

  // Current active category in the settings page (persisted across tab switches)
  const activeCategory = ref('basic')
  // For navigating to a specific settings category from other components
  const openCategory = ref<string | null>(null)

  // Tracks the last ai.json payload pushed from this store so unrelated
  // settings saves don't rewrite the AI config file needlessly.
  let lastSavedAiJson = ''
  let applyingLoadedSettings = false
  let settingsReadyToSave = false

  // Overlay the syncable AI slice (ai.json); selection stays device-local.
  function recomposeAi() {
    const aiCfg = useAIConfigStore()
    const models = aiCfg.models.length ? aiCfg.models : settings.value.ai.models
    settings.value.ai = {
      maxTurns: aiCfg.maxTurns,
      autoCompact: aiCfg.autoCompact,
      contextTokenBudget: aiCfg.contextTokenBudget,
      summaryModelId: aiCfg.summaryModelId,
      models,
    }
  }

  function syncAiConfigFromSettings() {
    const aiJson = JSON.stringify(settings.value.ai)
    if (aiJson === lastSavedAiJson) return
    lastSavedAiJson = aiJson
    const aiCfg = useAIConfigStore()
    aiCfg.maxTurns = settings.value.ai.maxTurns
    aiCfg.autoCompact = settings.value.ai.autoCompact ?? true
    aiCfg.contextTokenBudget = normalizeContextTokenBudget(settings.value.ai.contextTokenBudget)
    aiCfg.summaryModelId = settings.value.ai.summaryModelId || ''
    aiCfg.models = settings.value.ai.models
    aiCfg.save()
  }

  // Apply the UI font baseline to the rem root and cache it for the
  // pre-paint script in index.html (avoids a wrong-size flash on reload).
  function applyUiFontSize() {
    const px = (settings.value.uiFontSize / 12) * 16
    document.documentElement.style.fontSize = px + 'px'
    try {
      localStorage.setItem('uiFontSize', String(settings.value.uiFontSize))
    } catch {
      // private mode etc. — the loaded value still applies this session
    }
  }

  function applyTheme() {
    let theme = settings.value.theme
    if (theme === 'system') {
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
      theme = prefersDark ? 'dark' : 'light'
    }
    document.documentElement.dataset.theme = theme
    // Cache the resolved theme for the pre-paint script in index.html (issue
    // #1135): it applies this before the first paint so the window doesn't
    // flash the dark no-[data-theme] default while settings load.
    try {
      localStorage.setItem('appTheme', theme)
    } catch {
      // private mode etc. — the pre-paint script falls back to matchMedia
    }
  }

  async function init() {
    applyingLoadedSettings = true
    settingsReadyToSave = false
    try {
      await initSettings()
    } finally {
      applyingLoadedSettings = false
    }
  }

  async function initSettings() {
    await useLocalStateStore().init()
    let legacyModelId = ''
    let settingsRead = false
    try {
      const loadedSettings = await LoadSettings()
      if (loadedSettings) {
        settingsRead = true
        legacyModelId = (loadedSettings.ai as { activeModelId?: string })?.activeModelId || ''
        settings.value = mergeSettings(loadedSettings)
      }
    } catch {
      // use defaults
    } finally {
      loaded.value = true
    }
    // Apply the theme before anything slow: the AI store load and the shell
    // probe (which spawns wsl.exe) below must not delay data-theme past the
    // first paint. applyTheme() only depends on the loaded settings.
    applyTheme()
    await useAIConfigStore().load()
    recomposeAi()
    if (settingsRead) {
      await useLocalStateStore().ensureAISelection(settings.value.ai.models.map(m => m.id), legacyModelId)
      // Save only after the local migration succeeds; never discard the old selection first.
      if (legacyModelId) await SaveSettings(settings.value)
      settingsReadyToSave = true
    }
    lastSavedAiJson = JSON.stringify(settings.value.ai)
    applyUiFontSize()
    try {
      availableShells.value = await GetAvailableShells()
    } catch {
      availableShells.value = []
    }
    // Push the persisted log directory override to the backend so
    // logs enabled before the user opens the settings tab already
    // respect it. Fire-and-forget; the backend has a safe empty fallback.
    SetDefaultSessionLogDir(settings.value.terminal.sessionLogDir || '').catch(() => {})
    setLocale(settings.value.language)
  }

  // Reload settings after the credential store is unlocked (e.g. after a
  // data-dir migration in master-password mode). Model apiKeys can only be
  // decrypted once the store is unlocked; re-running LoadSettings re-fetches
  // them instead of leaving the initial load's empty fallback in place.
  async function reload() {
    applyingLoadedSettings = true
    settingsReadyToSave = false
    try {
      await reloadSettings()
    } finally {
      applyingLoadedSettings = false
    }
  }

  async function reloadSettings() {
    let legacyModelId = ''
    let settingsRead = false
    try {
      const loadedSettings = await LoadSettings()
      if (loadedSettings) {
        settingsRead = true
        legacyModelId = (loadedSettings.ai as { activeModelId?: string })?.activeModelId || ''
        settings.value = mergeSettings(loadedSettings)
        loaded.value = true
      }
    } catch {
      // use defaults
    }
    await useAIConfigStore().load()
    recomposeAi()
    if (settingsRead) {
      await useLocalStateStore().ensureAISelection(settings.value.ai.models.map(m => m.id), legacyModelId)
      if (legacyModelId) await SaveSettings(settings.value)
      settingsReadyToSave = true
    }
    applyUiFontSize()
    applyTheme()
    setLocale(settings.value.language)
  }

  async function refreshAvailableShells() {
    try {
      availableShells.value = await GetAvailableShells()
    } catch {
      availableShells.value = []
    }
  }

  async function save() {
    if (applyingLoadedSettings || !settingsReadyToSave) return
    try {
      await SaveSettings(settings.value)
      // Mirror the syncable AI slice into ai.json (skipped when unchanged).
      syncAiConfigFromSettings()
      // Keep the backend override in sync on every save. Cheap and
      // avoids the need for a dedicated watcher on this single field.
      SetDefaultSessionLogDir(settings.value.terminal.sessionLogDir || '').catch(() => {})
    } catch {
      // ignore save errors
    }
  }

  function updateTheme(value: AppSettings['theme']) {
    settings.value.theme = value
    save()
  }

  function addCustomTheme(theme: CustomTerminalTheme) {
    settings.value.customTerminalThemes.push(theme)
    save()
  }

  function updateCustomTheme(id: string, updates: Partial<CustomTerminalTheme>) {
    const idx = settings.value.customTerminalThemes.findIndex(t => t.id === id)
    if (idx >= 0) {
      settings.value.customTerminalThemes[idx] = { ...settings.value.customTerminalThemes[idx], ...updates }
      save()
    }
  }

  function removeCustomTheme(id: string) {
    const idx = settings.value.customTerminalThemes.findIndex(t => t.id === id)
    if (idx >= 0) {
      settings.value.customTerminalThemes.splice(idx, 1)
      save()
    }
  }

  function updateLanguage(value: AppSettings['language']) {
    settings.value.language = value
    setLocale(value)
    save()
  }

  function updateTerminal(updates: Partial<AppSettings['terminal']>) {
    settings.value.terminal = { ...settings.value.terminal, ...updates }
    save()
  }

  function addModel(model: AIModelConfig) {
    settings.value.ai.models.push(model)
    save()
  }

  function updateModel(id: string, updates: Partial<AIModelConfig>) {
    const idx = settings.value.ai.models.findIndex(m => m.id === id)
    if (idx >= 0) {
      settings.value.ai.models[idx] = { ...settings.value.ai.models[idx], ...updates }
      save()
    }
  }

  function removeModel(id: string) {
    const idx = settings.value.ai.models.findIndex(m => m.id === id)
    if (idx >= 0) {
      settings.value.ai.models.splice(idx, 1)
      if (settings.value.ai.summaryModelId === id) settings.value.ai.summaryModelId = ''
      void useLocalStateStore().ensureAISelection(settings.value.ai.models.map(m => m.id)).catch(() => {})
      save()
    }
  }

  function setActiveModel(id: string) {
    if (!settings.value.ai.models.some(m => m.id === id)) return
    return useLocalStateStore().setActiveAIModel(id)
  }

  const sftpBookmarks = computed(() => settings.value.sftpBookmarks)

  // Writable computed so components can toggle visibility directly; every
  // write is persisted with the settings blob. The transfer panel auto-pops
  // on new tasks regardless of this flag — it only remembers the last
  // visibility across restarts.
  const sftpTransferPanelVisible = computed<boolean>({
    get: () => settings.value.sftpTransferPanelVisible,
    set: (v: boolean) => {
      settings.value.sftpTransferPanelVisible = v
      save()
    }
  })

  function addSftpBookmark(mode: 'local' | 'remote', path: string) {
    const key = mode === 'local' ? 'localPaths' : 'remotePaths'
    const paths = settings.value.sftpBookmarks[key]
    if (!paths.includes(path)) {
      if (paths.length >= 50) {
        paths.shift()
      }
      paths.push(path)
      save()
    }
  }

  function removeSftpBookmark(mode: 'local' | 'remote', path: string) {
    const key = mode === 'local' ? 'localPaths' : 'remotePaths'
    const paths = settings.value.sftpBookmarks[key]
    const idx = paths.indexOf(path)
    if (idx >= 0) {
      paths.splice(idx, 1)
      save()
    }
  }

  // Auto-save when AI models change
  watch(() => settings.value.ai, save, { deep: true, flush: 'sync' })

  // Apply theme when it changes
  watch(() => settings.value.theme, applyTheme)

  // Apply the UI font baseline as soon as it changes (no restart needed)
  watch(() => settings.value.uiFontSize, applyUiFontSize)

  // Listen for system color scheme changes
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
    systemPrefersDark.value = e.matches
    if (settings.value.theme === 'system') {
      applyTheme()
    }
  })

  // Listen for settings changes from sync
  unsubSettingsChanged?.()
  unsubSettingsChanged =Events.On('store:settings:changed', (ev) => { const data: AppSettings = ev.data; 
    if (data) {
      const wasApplyingSettings = applyingLoadedSettings
      applyingLoadedSettings = true
      try {
        settings.value = mergeSettings(data)
      } finally {
        applyingLoadedSettings = wasApplyingSettings
      }
      loaded.value = true
      applyUiFontSize()
      applyTheme()
    }
  })

  // AI config changed via a sync pull — refresh the local composition.
  unsubAiChanged?.()
  unsubAiChanged = Events.On('store:ai:changed', (ev) => {
    const data = ev.data as Partial<AppSettings['ai']> | null
    if (data) {
      const aiCfg = useAIConfigStore()
      aiCfg.maxTurns = data.maxTurns ?? DEFAULT_SETTINGS.ai.maxTurns
      aiCfg.autoCompact = data.autoCompact ?? true
      aiCfg.contextTokenBudget = normalizeContextTokenBudget(data.contextTokenBudget)
      aiCfg.summaryModelId = data.summaryModelId || ''
      aiCfg.models = data.models?.length ? data.models : aiCfg.models
      const wasApplyingSettings = applyingLoadedSettings
      applyingLoadedSettings = true
      try {
        recomposeAi()
      } finally {
        applyingLoadedSettings = wasApplyingSettings
      }
      void useLocalStateStore().ensureAISelection(settings.value.ai.models.map(m => m.id)).catch(() => {})
      lastSavedAiJson = JSON.stringify(settings.value.ai)
    }
  })

  function dispose() {
    unsubSettingsChanged?.()
    unsubSettingsChanged = null
    unsubAiChanged?.()
    unsubAiChanged = null
  }

  return {
    settings,
    loaded,
    availableShells,
    refreshAvailableShells,
    theme,
    resolvedAppTheme,
    language,
    terminal,
    ai,
    activeModel,
    activeModelId,
    activeCategory,
    openCategory,
    init,
    reload,
    save,
    applyTheme,
    updateTheme,
    updateLanguage,
    updateTerminal,
    addModel,
    updateModel,
    removeModel,
    setActiveModel,
    sftpBookmarks,
    sftpTransferPanelVisible,
    addSftpBookmark,
    removeSftpBookmark,
    addCustomTheme,
    updateCustomTheme,
    removeCustomTheme,
    dispose
  }
})

function mergeSettings(loaded: AppSettings): AppSettings {
  return {
    theme: loaded.theme || DEFAULT_SETTINGS.theme,
    language: loaded.language || DEFAULT_SETTINGS.language,
    uiFontSize: loaded.uiFontSize ?? DEFAULT_SETTINGS.uiFontSize,
    terminal: {
      ...DEFAULT_SETTINGS.terminal,
      ...loaded.terminal,
      theme: (loaded.terminal?.theme as string) === 'dark' || (loaded.terminal?.theme as string) === 'light'
        ? DEFAULT_SETTINGS.terminal.theme
        : loaded.terminal?.theme || DEFAULT_SETTINGS.terminal.theme
    },
    ai: {
      maxTurns: loaded.ai?.maxTurns ?? DEFAULT_SETTINGS.ai.maxTurns,
      autoCompact: loaded.ai?.autoCompact ?? true,
      contextTokenBudget: normalizeContextTokenBudget(loaded.ai?.contextTokenBudget),
      summaryModelId: loaded.ai?.summaryModelId || '',
      models: loaded.ai?.models?.length ? loaded.ai.models : DEFAULT_SETTINGS.ai.models,
    },
    keyboard: normalizeKeyBindings(loaded.keyboard || {}),
    autoCheckUpdate: loaded.autoCheckUpdate ?? DEFAULT_SETTINGS.autoCheckUpdate,
    updateSource: loaded.updateSource ?? DEFAULT_SETTINGS.updateSource,
    closeTabPrompt: loaded.closeTabPrompt ?? DEFAULT_SETTINGS.closeTabPrompt,
    closeAppPrompt: loaded.closeAppPrompt ?? DEFAULT_SETTINGS.closeAppPrompt,
    restoreTabsPrompt: loaded.restoreTabsPrompt ?? DEFAULT_SETTINGS.restoreTabsPrompt,
    sftpBookmarks: {
      localPaths: loaded.sftpBookmarks?.localPaths || [],
      remotePaths: loaded.sftpBookmarks?.remotePaths || []
    },
    sftpTransferPanelVisible: loaded.sftpTransferPanelVisible ?? DEFAULT_SETTINGS.sftpTransferPanelVisible,
    customTerminalThemes: loaded.customTerminalThemes || [],
    defaultLocalShell: loaded.defaultLocalShell ?? DEFAULT_SETTINGS.defaultLocalShell,
    gitBashPath: loaded.gitBashPath ?? DEFAULT_SETTINGS.gitBashPath,
    tabCloseButton: loaded.tabCloseButton || DEFAULT_SETTINGS.tabCloseButton,
    showTabShortcutHints: loaded.showTabShortcutHints ?? DEFAULT_SETTINGS.showTabShortcutHints,
    showTrayIcon: loaded.showTrayIcon ?? DEFAULT_SETTINGS.showTrayIcon,
    hostListMenuStyle: loaded.hostListMenuStyle || DEFAULT_SETTINGS.hostListMenuStyle,
    connectionsDisplayMode: loaded.connectionsDisplayMode === 'compact' ? 'compact' : 'full',
    quickCommandsDisplayMode: loaded.quickCommandsDisplayMode === 'buttons' ? 'buttons' : 'list',
    monitorSections: {
      ...DEFAULT_SETTINGS.monitorSections,
      ...(loaded.monitorSections || {})
    },
    newTabAction: loaded.newTabAction || DEFAULT_SETTINGS.newTabAction,
    // Per-key merge so a settings.json written before a view existed (or
    // with a key dropped) still gets the default for that view.
    sidebarTabs: {
      ...DEFAULT_SETTINGS.sidebarTabs,
      ...(loaded.sidebarTabs || {})
    },
    // Dock layout: per-key merge like sidebarTabs; the tab order is
    // validated against known view ids and backfilled with any missing
    // views (orderedViews computes the append at read time).
    panelLocations: {
      ...DEFAULT_SETTINGS.panelLocations,
      ...(loaded.panelLocations || {})
    },
    panelOrder: (loaded.panelOrder || []).filter(v =>
      Object.keys(DEFAULT_SETTINGS.panelLocations).includes(v)
    ),
    // Per-key visible merge; the order is validated against the known block
    // ids and backfilled with any missing blocks (older settings.json files
    // or hand-edited ones).
    startPage: mergeStartPage(loaded.startPage)
  }
}

function mergeStartPage(loaded: AppSettings['startPage']): AppSettings['startPage'] {
  const visible = {
    ...DEFAULT_SETTINGS.startPage.visible,
    ...(loaded?.visible || {})
  }
  const known = DEFAULT_SETTINGS.startPage.order
  const persisted = (loaded?.order || []).filter(b => known.includes(b))
  const order = [...persisted, ...known.filter(b => !persisted.includes(b))]
  return { visible, order }
}
