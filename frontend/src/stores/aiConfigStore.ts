import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { AIModelConfig } from '../types/settings'
import { DEFAULT_SETTINGS, normalizeContextTokenBudget } from '../types/settings'
import { LoadAIModels, SaveAIModels } from '../../bindings/github.com/ys-ll/uniterm/app'

// aiConfigStore backs ai.json — the syncable slice of the AI settings
// (model catalog, agent turn limit and context preferences). settingsStore composes this into
// settings.ai; the current selection is stored separately in localStateStore.
export const useAIConfigStore = defineStore('aiConfig', () => {
  const maxTurns = ref<number>(DEFAULT_SETTINGS.ai.maxTurns)
  const autoCompact = ref(true)
  const contextTokenBudget = ref(DEFAULT_SETTINGS.ai.contextTokenBudget)
  const summaryModelId = ref('')
  const models = ref<AIModelConfig[]>([...DEFAULT_SETTINGS.ai.models])
  const loaded = ref(false)

  async function load() {
    try {
      const cfg = await LoadAIModels()
      if (cfg) {
        maxTurns.value = cfg.maxTurns ?? DEFAULT_SETTINGS.ai.maxTurns
        autoCompact.value = cfg.autoCompact ?? true
        contextTokenBudget.value = normalizeContextTokenBudget(cfg.contextTokenBudget)
        summaryModelId.value = cfg.summaryModelId || ''
        models.value = cfg.models?.length ? cfg.models : [...DEFAULT_SETTINGS.ai.models]
      }
    } catch {
      // keep defaults
    } finally {
      loaded.value = true
    }
  }

  async function save() {
    try {
      await SaveAIModels({ maxTurns: maxTurns.value, models: models.value,
        autoCompact: autoCompact.value, contextTokenBudget: contextTokenBudget.value,
        summaryModelId: summaryModelId.value })
    } catch {
      // ignore save errors
    }
  }

  return { maxTurns, autoCompact, contextTokenBudget, summaryModelId, models, loaded, load, save }
})
