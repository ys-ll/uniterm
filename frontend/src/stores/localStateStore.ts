import { defineStore } from 'pinia'
import { ref } from 'vue'
import { LoadLocalState, SaveLocalState } from '../services/businessConfig'
import type { MCPSettings } from '../types/mcp'
import { DEFAULT_MCP_SETTINGS } from '../types/mcp'
import type { SavedTabsSnapshot } from '../types/tabSnapshot'
import { isWebMode } from '../utils/webMode'

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

// Only view preferences cross a browser refresh. Never import native window
// state, MCP configuration or reconnectable desktop tab snapshots.
const WEB_FIELDS = [
  'sidebarVisible', 'aiSidebarVisible', 'sidebarWidth', 'aiSidebarWidth',
  'collapsedGroupIds', 'collapsedQuickCommandGroupIds', 'sftpHiddenColumns',
  'backgroundEnabled', 'backgroundImage', 'backgroundOpacity', 'backgroundBlur', 'backgroundFit',
] as const

function browserViewState(value: Partial<LocalState>): Partial<LocalState> {
  return Object.fromEntries(WEB_FIELDS.flatMap(key => {
    const v = value[key]
    const valid = Array.isArray(DEFAULT[key])
      ? Array.isArray(v) && v.every(item => typeof item === 'string')
      : typeof v === typeof DEFAULT[key] && (typeof v !== 'number' || Number.isFinite(v) && v >= 0)
    return valid ? [[key, v]] : []
  }))
}

function browserStorageKey(): string {
  const id = window.__UNITERM_WEB__?.instanceId
  if (!id) throw new Error('Browser bootstrap must complete before loading view state')
  return `uniterm:web:${id}:view`
}

export const useLocalStateStore = defineStore('localState', () => {
  const state = ref<LocalState>(structuredClone(DEFAULT))
  const loaded = ref(false)
  let initPromise: Promise<void> | null = null

  async function init() {
    if (loaded.value) return
    if (!initPromise) {
      initPromise = (async () => {
        try {
          const s = isWebMode()
            ? browserViewState(JSON.parse(localStorage.getItem(browserStorageKey()) || '{}') || {})
            : await LoadLocalState()
          state.value = { ...structuredClone(DEFAULT), ...s } as LocalState
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
    if (isWebMode()) {
      // Wait for the browser's own preferences so early sidebar watchers
      // cannot overwrite stored widths before startup has loaded them.
      await init()
      state.value = { ...state.value, ...browserViewState(patch) }
      try {
        localStorage.setItem(browserStorageKey(), JSON.stringify(browserViewState(state.value)))
      } catch { /* Private browsing / full storage: keep this view in memory. */ }
      return
    }
    state.value = { ...state.value, ...patch } as LocalState
    try {
      await SaveLocalState(state.value as LocalState)
    } catch {
      // ignore save errors
    }
  }

  return { state, loaded, init, update }
})
