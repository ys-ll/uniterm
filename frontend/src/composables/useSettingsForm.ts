import { ref, computed } from 'vue'
import { GetPlatform } from '../../bindings/github.com/ys-ll/uniterm/app'

/**
 * Shared settings-form state that spans multiple settings sub-components.
 *
 * `useThemeEditor` — the CustomThemeEditor dialog is rendered once by the
 * SettingsTab shell but opened from both the Basic (app theme) and Terminal
 * (terminal theme) panels. A module-level singleton keeps the dialog state
 * coherent across the panels without prop-drilling.
 *
 * `usePlatform` — several panels need the OS platform (mac detection for
 * keyboard symbols, shell labels). The backend call is made once and cached.
 */

// ── Theme editor (shared by Basic + Terminal panels) ──
const themeEditorVisible = ref(false)
const themeEditorSourceId = ref<string | undefined>(undefined)

export function useThemeEditor() {
  function openThemeEditor(sourceThemeId?: string) {
    themeEditorSourceId.value = sourceThemeId
    themeEditorVisible.value = true
  }
  return { themeEditorVisible, themeEditorSourceId, openThemeEditor }
}

// ── Platform (shared by Terminal + Shortcut panels) ──
const platform = ref('')
const platformLoaded = ref(false)
const isMac = computed(() => platform.value === 'darwin')

export function usePlatform() {
  if (!platformLoaded.value) {
    platformLoaded.value = true
    GetPlatform().then((p: string) => { platform.value = p })
      .catch(() => { platform.value = '' })
  }
  return { platform, isMac }
}