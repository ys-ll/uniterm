// Multi-line paste confirmation service (issue #1370). The dialog itself is
// mounted once (App.vue); BaseTerminal awaits confirmMultilinePaste before
// writing multi-line clipboard content to a session. The dialog shows an
// editable preview — the resolved text is what gets pasted — plus a
// "don't ask again" switch that flips the multilinePaste setting to 'direct'.
import { reactive } from 'vue'
import { useSettingsStore } from '../stores/settingsStore'
import { isMultilinePaste } from '../utils/terminalPaste'

interface MultilinePasteDialogState {
  visible: boolean
  text: string
  dontAskAgain: boolean
}

const state = reactive<MultilinePasteDialogState>({
  visible: false,
  text: '',
  dontAskAgain: false
})

let resolver: ((text: string | null) => void) | null = null

// Gate a paste through the multi-line confirmation dialog when the setting
// asks for it. Resolves with the text to paste (possibly edited by the user)
// or null when the user cancelled. Single-line pastes and the 'direct' mode
// resolve immediately with the input untouched.
export function confirmMultilinePaste(text: string): Promise<string | null> {
  const settings = useSettingsStore()
  if (settings.settings.terminal.multilinePaste !== 'confirm') return Promise.resolve(text)
  if (!isMultilinePaste(text)) return Promise.resolve(text)
  // A second open while one is pending resolves the first as cancelled.
  resolver?.(null)
  state.text = text
  state.dontAskAgain = false
  state.visible = true
  return new Promise(resolve => { resolver = resolve })
}

// Called by the dialog component on confirm / cancel.
export function finishMultilinePaste(text: string | null, dontAskAgain: boolean) {
  state.visible = false
  if (text !== null && dontAskAgain) {
    const settings = useSettingsStore()
    settings.settings.terminal.multilinePaste = 'direct'
    settings.save()
  }
  resolver?.(text)
  resolver = null
}

// For the mounted dialog component.
export function useMultilinePasteDialogState() {
  return state
}
