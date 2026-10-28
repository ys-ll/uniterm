import { computed, type ComputedRef } from 'vue'
import { useSettingsStore } from '../../stores/settingsStore'
import { getShellLabel as getShellLabelBase } from '../../utils/shellLabel'
import type { ConnectionConfig } from '../../types/session'

export interface UseShellOptionsReturn {
  localShellOptions: ComputedRef<{ label: string; value: string }[]>
  wslShellOptions: ComputedRef<{ label: string; value: string }[]>
  shellOptionsForType: ComputedRef<{ label: string; value: string }[]>
}

/**
 * Shell option lists for the local/wsl terminal types, extracted from
 * ConnectionForm. `shellOptionsForType` switches between local and WSL
 * shells based on the current form type.
 */
export function useShellOptions(form: ConnectionConfig): UseShellOptionsReturn {
  const settingsStore = useSettingsStore()

  // Local-shell options exclude WSL entries: the ordinary local terminal type
  // never surfaces `wsl://<distro>` shells (those belong to the wsl type).
  const localShellOptions = computed(() =>
    settingsStore.availableShells
      .filter(sh => !sh.toLowerCase().startsWith('wsl://'))
      .map(sh => ({ label: getShellLabelBase(sh), value: sh }))
  )

  // WSL options reuse the terminal "shell" field: their value is the `wsl://<distro>`
  // shell path (which carries the distro on the backend), not a separate field.
  const wslShellOptions = computed(() =>
    settingsStore.availableShells
      .filter(sh => sh.toLowerCase().startsWith('wsl://'))
      .map(sh => ({ label: getShellLabelBase(sh), value: sh }))
  )

  const shellOptionsForType = computed(() =>
    form.type === 'wsl' ? wslShellOptions.value : localShellOptions.value
  )

  return { localShellOptions, wslShellOptions, shellOptionsForType }
}