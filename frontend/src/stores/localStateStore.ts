import { defineStore } from 'pinia'
import { ref } from 'vue'
import { LoadLocalState, SaveLocalState } from '../services/businessConfig'
import type { MCPSettings } from '../types/mcp'
import { DEFAULT_MCP_SETTINGS } from '../types/mcp'
import type { SavedTabsSnapshot } from '../types/tabSnapshot'

// Shape of the backend's LocalState store (bindings are untyped JS, so the
// type is declared here and must stay in sync with backend/store).
interface LocalState {
  sidebarVisible: boolean
  aiSidebarVisible: boolean
  // Persisted sidebar widths in px (0 = old config, keep component default).
  sidebarWidth: number
  aiSidebarWidth: number
  collapsedGroupIds: string[]
  collapsedQuickCommandGroupIds: string[]
  windowX: number
  windowY: number
  windowWidth: number
  windowHeight: number
  windowMaximised: boolean
  backgroundEnabled: boolean
  backgroundImage: string
  backgroundOpacity: number
  backgroundBlur: number
  backgroundFit: string
  systemTitleBar: boolean
  externalEditor: string
  // SFTP file-list columns the user hid via the header context menu. Columns
  // absent from the list stay visible.
  sftpHiddenColumns: string[]
  // MCP server config (external AI agents). Per device — never synced.
  mcp: MCPSettings
  // Open-tabs snapshot for "reopen last session's tabs". Opaque shape
  // (types/tabSnapshot.ts); local-only, never synced.
  lastTabsSnapshot: SavedTabsSnapshot | null
}

const DEFAULT: LocalState = {
  sidebarVisible: true,
  aiSidebarVisible: true,
  sidebarWidth: 240,
  aiSidebarWidth: 360,
  collapsedGroupIds: [],
  collapsedQuickCommandGroupIds: [],
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
  systemTitleBar: false,
  externalEditor: '',
  sftpHiddenColumns: [],
  mcp: { ...DEFAULT_MCP_SETTINGS },
  lastTabsSnapshot: null,
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
