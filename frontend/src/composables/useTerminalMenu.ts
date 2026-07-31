import { ref, onMounted, onUnmounted } from 'vue'
import type { Ref } from 'vue'
import { useSettingsStore } from '../stores/settingsStore'
import { ClipboardGetText, ClipboardSetText } from '../../wailsjs/runtime/runtime'

export interface UseTerminalMenuOptions {
  getSelection: () => string
  onPaste: (text: string) => Promise<void> | void
  onAskAI?: (text: string) => void
}

export interface UseTerminalMenuReturn {
  menuVisible: Ref<boolean>
  menuStyle: Ref<{ left: string; top: string }>
  hasSelection: Ref<boolean>
  onContextMenu: (e: MouseEvent) => void
  closeMenu: () => void
  copySelection: () => void
  copyAndPaste: () => Promise<void>
  pasteFromClipboard: () => Promise<void>
  askAI: () => void
}

export function useTerminalMenu(options: UseTerminalMenuOptions): UseTerminalMenuReturn {
  const settingsStore = useSettingsStore()

  const menuVisible = ref(false)
  const menuStyle = ref({ left: '0px', top: '0px' })
  const hasSelection = ref(false)

  function closeMenu() {
    menuVisible.value = false
  }

  function onContextMenu(e: MouseEvent) {
    const rightClickAction = settingsStore.settings.terminal.rightClickAction
    if (rightClickAction === 'paste') {
      e.preventDefault()
      e.stopPropagation()
      pasteFromClipboard()
      return
    }
    e.preventDefault()
    e.stopPropagation()
    window.dispatchEvent(new CustomEvent('global:close-context-menus'))
    hasSelection.value = !!options.getSelection()
    menuStyle.value = fitMenuPosition(e.clientX, e.clientY, 120, 140)
    menuVisible.value = true
  }

  function fitMenuPosition(x: number, y: number, menuW: number, menuH: number) {
    let left = x
    let top = y
    if (x + menuW > window.innerWidth) left = x - menuW
    if (y + menuH > window.innerHeight) top = y - menuH
    return { left: left + 'px', top: top + 'px' }
  }

  // Write to the OS clipboard via Wails first (reliable in WebView2), falling
  // back to the browser clipboard API when the runtime call reports failure.
  //
  // Wails' ClipboardSetText returns Promise<boolean> — `false` means the OS
  // refused (focus loss, AppKit permission glitch on macOS), NOT a thrown
  // error. The previous try/catch only handled rejections, so a `resolve(false)`
  // silently left the clipboard empty. Now we check the resolved value and
  // fall through to navigator.clipboard.writeText in that case.
  async function writeClipboard(text: string) {
    let ok = false
    try {
      ok = await ClipboardSetText(text)
    } catch {
      ok = false
    }
    if (!ok) {
      try { await navigator.clipboard.writeText(text) } catch { /* ignore */ }
    }
  }
  const browserWriter: ClipboardWriter = async (text) => {
    try { await navigator.clipboard.writeText(text); return true } catch { return false }
  }
  const writeClipboard: ClipboardWriter = typeof ClipboardSetText === 'function'
    ? wailsWriter
    : browserWriter

  function copySelection() {
    // Re-read the xterm selection at click time. hasSelection.value was
    // captured during contextmenu, but in WKWebView the right-click
    // mousedown can clear the xterm selection between contextmenu and the
    // menu click, leaving hasSelection stale. Trust getSelection() — the
    // authoritative source — rather than the cached ref.
    const text = options.getSelection()
    if (text) {
      writeClipboard(text)
    }
    closeMenu()
  }

  async function copyAndPaste() {
    const text = options.getSelection()
    if (text) {
      await writeClipboard(text)
      await options.onPaste(text)
    }
    closeMenu()
  }

  function askAI() {
    const text = options.getSelection()
    if (text && options.onAskAI) {
      options.onAskAI(text)
    }
    closeMenu()
  }

  async function pasteFromClipboard() {
    try {
      // Wails clipboard, not navigator.clipboard.readText() — the latter pops
      // a system "Paste" confirmation on macOS WKWebView.
      const text = await ClipboardGetText()
      if (text) {
        await options.onPaste(text)
      }
    } catch {
      // clipboard read failed
    }
    closeMenu()
  }

  onMounted(() => {
    window.addEventListener('global:close-context-menus', closeMenu)
    document.addEventListener('click', closeMenu)
  })

  onUnmounted(() => {
    window.removeEventListener('global:close-context-menus', closeMenu)
    document.removeEventListener('click', closeMenu)
  })

  return {
    menuVisible,
    menuStyle,
    hasSelection,
    onContextMenu,
    closeMenu,
    copySelection,
    copyAndPaste,
    pasteFromClipboard,
    askAI,
  }
}
