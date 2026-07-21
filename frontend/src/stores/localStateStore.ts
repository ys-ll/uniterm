import { defineStore } from 'pinia'
import { ref } from 'vue'
import { LoadLocalState, SaveLocalState } from '../../wailsjs/go/main/App'
import { store } from '../../wailsjs/go/models'

type LocalState = store.LocalState

const DEFAULT: LocalState = {
  sidebarVisible: true,
  aiSidebarVisible: true,
  collapsedGroupIds: [],
  windowX: 0,
  windowY: 0,
  windowWidth: 0,
  windowHeight: 0,
  windowMaximised: false,
  backgroundEnabled: false,
  backgroundImage: '',
  backgroundOpacity: 60,
  backgroundBlur: 3,
  backgroundFit: 'cover',
} as LocalState

export const useLocalStateStore = defineStore('localState', () => {
  const state = ref<LocalState>({ ...DEFAULT })
  const loaded = ref(false)
  let initPromise: Promise<void> | null = null

  async function init() {
    if (loaded.value) return
    if (!initPromise) {
      initPromise = (async () => {
        try {
          const s = await LoadLocalState()
          state.value = { ...DEFAULT, ...s } as LocalState
        } catch {
          // keep defaults
        } finally {
          loaded.value = true
        }
      })()
    }
    return initPromise
  }

  async function update(patch: Partial<LocalState>) {
    state.value = { ...state.value, ...patch } as LocalState
    try {
      await SaveLocalState(state.value as LocalState)
    } catch {
      // ignore save errors
    }
  }

  return { state, loaded, init, update }
})
