import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import type { AppSettings, AIModelConfig, CustomTerminalTheme } from '../types/settings'
import { DEFAULT_SETTINGS, DEFAULT_KEYBOARD } from '../types/settings'
import { SaveSettings, LoadSettings, GetAvailableShells } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime'
import { setLocale } from '../i18n'

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<AppSettings>({ ...DEFAULT_SETTINGS })
  const loaded = ref(false)
  const availableShells = ref<string[]>([])

  const theme = computed(() => settings.value.theme)
  const language = computed(() => settings.value.language)
  const terminal = computed(() => settings.value.terminal)
  const ai = computed(() => settings.value.ai)

  const activeModel = computed(() =>
    settings.value.ai.models.find(m => m.id === settings.value.ai.activeModelId) || settings.value.ai.models[0]
  )

  // Current active category in the settings page (persisted across tab switches)
  const activeCategory = ref('basic')
  // For navigating to a specific settings category from other components
  const openCategory = ref<string | null>(null)

  function applyTheme() {
    let theme = settings.value.theme
    if (theme === 'system') {
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
      document.documentElement.dataset.theme = prefersDark ? 'dark' : 'light'
    } else {
      document.documentElement.dataset.theme = theme
    }
  }

  async function init() {
    try {
      const loadedSettings = await LoadSettings()
      if (loadedSettings) {
        settings.value = mergeSettings(loadedSettings)
        loaded.value = true
      }
    } catch {
      // use defaults
    }
    try {
      availableShells.value = await GetAvailableShells()
    } catch {
      availableShells.value = []
    }
    applyTheme()
    setLocale(settings.value.language)
  }

  async function save() {
    try {
      await SaveSettings(settings.value)
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
      if (settings.value.ai.activeModelId === id && settings.value.ai.models.length > 0) {
        settings.value.ai.activeModelId = settings.value.ai.models[0].id
      }
      save()
    }
  }

  function setActiveModel(id: string) {
    settings.value.ai.activeModelId = id
    save()
  }

  const sftpBookmarks = computed(() => settings.value.sftpBookmarks)

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
  watch(() => settings.value.ai, save, { deep: true })

  // Apply theme when it changes
  watch(() => settings.value.theme, applyTheme)

  // Listen for system color scheme changes
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (settings.value.theme === 'system') {
      applyTheme()
    }
  })

  // Listen for settings changes from sync
  EventsOn('store:settings:changed', (data: AppSettings) => {
    if (data) {
      settings.value = mergeSettings(data)
      loaded.value = true
      applyTheme()
    }
  })

  return {
    settings,
    loaded,
    availableShells,
    theme,
    language,
    terminal,
    ai,
    activeModel,
    activeCategory,
    openCategory,
    init,
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
    addSftpBookmark,
    removeSftpBookmark,
    addCustomTheme,
    updateCustomTheme,
    removeCustomTheme
  }
})

function mergeSettings(loaded: AppSettings): AppSettings {
  return {
    theme: loaded.theme || DEFAULT_SETTINGS.theme,
    language: loaded.language || DEFAULT_SETTINGS.language,
    terminal: {
      ...DEFAULT_SETTINGS.terminal,
      ...loaded.terminal,
      theme: (loaded.terminal?.theme as string) === 'dark' || (loaded.terminal?.theme as string) === 'light'
        ? DEFAULT_SETTINGS.terminal.theme
        : loaded.terminal?.theme || DEFAULT_SETTINGS.terminal.theme
    },
    ai: {
      maxTurns: loaded.ai?.maxTurns ?? DEFAULT_SETTINGS.ai.maxTurns,
      models: loaded.ai?.models?.length ? loaded.ai.models : DEFAULT_SETTINGS.ai.models,
      activeModelId: loaded.ai?.activeModelId || DEFAULT_SETTINGS.ai.activeModelId
    },
    keyboard: {
      ...DEFAULT_KEYBOARD,
      ...(loaded.keyboard || {})
    },
    autoCheckUpdate: loaded.autoCheckUpdate ?? DEFAULT_SETTINGS.autoCheckUpdate,
    sftpBookmarks: {
      localPaths: loaded.sftpBookmarks?.localPaths || [],
      remotePaths: loaded.sftpBookmarks?.remotePaths || []
    },
    customTerminalThemes: loaded.customTerminalThemes || [],
    defaultLocalShell: loaded.defaultLocalShell ?? DEFAULT_SETTINGS.defaultLocalShell
  }
}
