import type { Terminal } from '@xterm/xterm'
import { onTerminalKey } from './useKeyboardShortcuts'

import { useSuggestions } from './useSuggestions'
import type { SuggestionItem } from './useSuggestions'

import { queuedSessionWriteObserved as queuedSessionWrite } from '../services/terminalCommandWrite'
import { getTerminalSelection } from '../services/terminalCommandWrap'
import { filterTerminalInput } from '../utils/terminalInputFilter'
import { applyBackspaceKey } from '../utils/terminalOptions'
import { handleTerminalWordDelete } from '../utils/terminalWordDelete'
import { normalizePastedText, pasteWithScroll } from '../utils/terminalPaste'
import { readClipboardText, writeClipboard } from './useClipboardWrite'
import { getManagedTerminal } from '../services/terminalManager'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import type { Panel } from '../types/workspace'
import type { useTerminalInput } from './useTerminalInput'

type SuggestionsApi = ReturnType<typeof useSuggestions>


export interface UseTerminalKeyHandlerOptions {
  getTerminal: () => Terminal | null
  isMac: boolean
  getMode: () => 'ssh' | 'sftp' | 'local'
  getSessionId: () => string | null | undefined
  getPanelId: () => string | undefined
  getBroadcastActive: () => boolean | undefined
  getPanel: () => Panel | undefined
  getTerminalInput: () => ReturnType<typeof useTerminalInput> | null
  suggestions: SuggestionsApi

  applySuggestion: (item: SuggestionItem | null) => void | Promise<void>
}

/**
 * Terminal keyboard handling, IME composition reset, and paste logic.
 *
 * Owns the xterm attachCustomKeyEventHandler callback (handleTerminalKey),
 * the filtered session write path (writeTerminalInput), the smart copy/paste
 * guard, IME state reset, and paste-to-session (including broadcast). The host
 * binds handleTerminalKey via terminal.attachCustomKeyEventHandler and calls
 * resetIMEComposition on deactivation/visibility change.
 */
export function useTerminalKeyHandler(options: UseTerminalKeyHandlerOptions) {
  const {
    getTerminal,
    isMac,
    getMode,
    getSessionId,
    getPanelId,
    getBroadcastActive,
    getPanel,
    getTerminalInput,
    suggestions,

    applySuggestion,
  } = options

  const tabStore = useTabStore()
  const panelStore = usePanelStore()
  const sessionStore = useSessionStore()


  // Reset xterm's internal IME composition state. Two variants:
  // - resetIMEState:        only clears internal flags (safe during active typing)
  // - resetIMEComposition:  also blurs textarea to end OS-level composition (for
  //                         deactivation / visibility change when terminal is hidden)
  //
  // Accessing _core._compositionHelper is fragile but necessary — xterm exposes
  // no public API for this. The guard ensures we only act when composition is
  // actually active.
  function resetIMEState(): boolean {
    const terminal = getTerminal()
    if (!terminal) return false
    const core = (terminal as any)._core
    const ch = core?._compositionHelper
    if (!ch) return false
    if (!ch._isComposing && !ch._isSendingComposition) return false
    ch._isSendingComposition = false
    ch._isComposing = false
    ch._dataAlreadySent = ''
    const cv = core?._helperContainer?.querySelector?.('.composition-view')
    if (cv) cv.classList.remove('active')
    return true
  }

  function resetIMEComposition() {
    const terminal = getTerminal()
    if (!resetIMEState()) return
    // Clear the textarea and end the OS-level composition via blur.
    if (terminal?.textarea) {
      terminal.textarea.value = ''
      terminal.textarea.blur()
    }
  }

  function writeTerminalInput(data: string, inAlternateScreen: boolean) {
    const sid = getSessionId()
    const filtered = filterTerminalInput(data, inAlternateScreen)
    // Don't send empty input - avoids extra blank line when pressing Enter with no command
    if (!sid || !filtered) return

    if (getBroadcastActive()) {
      const targets = tabStore.getAllBroadcastPanelIds()
      if (targets.length > 0) {
        for (const pid of targets) {
          const p = panelStore.getPanel(pid)
          if (p?.sessionId && (p.type === 'ssh' || p.type === 'local' || p.type === 'wsl')) {
            const translated = applyBackspaceKey(
              filtered,
              p.config?.backspaceKey,
              p.config?.type,
            )
            queuedSessionWrite(p.sessionId, translated)
          }
        }
        return
      }
    }

    const panel = getPanelId() ? panelStore.getPanel(getPanelId()!) : undefined
    const translated = applyBackspaceKey(
      filtered,
      panel?.config?.backspaceKey,
      panel?.config?.type,
    )
    queuedSessionWrite(sid, translated)
  }

  function handleTerminalKey(e: KeyboardEvent): boolean {
    // Check global shortcuts first (Ctrl+Shift+/Alt+ combos)
    if (e.type === 'keydown' && !onTerminalKey(e)) return false

    // A bare modifier key (Shift/Ctrl/Alt/Meta held alone) produces no input, yet
    // xterm's _keyDown still runs and, with scrollOnUserInput=true, fires
    // scrollToBottom() — yanking the viewport back to the bottom when the user
    // has scrolled up to read history (issue #538). Short-circuit bare modifiers
    // before they reach xterm's scroll logic; they shouldn't be PTY input anyway.
    if (e.type === 'keydown' && (e.key === 'Control' || e.key === 'Shift' || e.key === 'Alt' || e.key === 'Meta')) {
      return false
    }

    // macOS 26 WKWebView: xterm's modifier handling for Shift+Delete is
    // unreliable and may emit no sequence, so the key silently fails to delete
    // (issue #538). Inject the standard CSI delete sequence ESC[3~ directly for
    // all plain Delete presses. Ctrl/Meta/Cmd combos are left to xterm so future
    // Delete-chord shortcuts can still hook in.
    if (e.type === 'keydown' && e.key === 'Delete' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault()
      getTerminal()?.input('\x1b[3~')
      return false
    }

    // Ctrl+Backspace word-delete: Unix shells treat ^H as single-char erase,
    // so emit ^W (delete previous word) for active Unix-style shells.
    const term = getTerminal()
    if (term && handleTerminalWordDelete(e, {
      connectionType: getPanel()?.config?.type ?? getPanel()?.type,
      connected: !!getSessionId() && sessionStore.getStatus(getSessionId()!) === 'connected',
      alternateScreen: term.buffer.active.type === 'alternate',
      passwordPrompt: getTerminalInput()?.isPasswordMode() ?? false,
      remoteOS: getSessionId() ? sessionStore.getRemoteOS(getSessionId()!) : undefined,
      hostPlatform: document.documentElement.dataset.platform,
    }, data => getTerminal()?.input(data))) return false

    // Paste via Wails clipboard (xterm's DOM paste is unreliable in WKWebView).
    // macOS Cmd+V stays hardcoded — it is not one of the configurable shortcuts.
    // On Windows/Linux, Ctrl+Shift+V is handled by the configurable shortcut
    // system (see the terminal:paste event below). Plain Ctrl+V is never
    // intercepted, so it passes through to the terminal app (vim visual block,
    // bash literal-next…) on every platform.


    if (isMac && e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey && (e.key === 'v' || e.key === 'V') && e.type === 'keydown') {
      e.preventDefault()
      if (getMode() === 'ssh' || getMode() === 'local') {
        readClipboardText().then(text => {
          if (text) pasteToSession(text)
        })
      }
      return false
    }

    // macOS Cmd+C: copy selection when there is one. With no selection, don't
    // intercept — let it pass through (Ctrl+C interrupt uses ctrlKey, unaffected).
    if (isMac && e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey && (e.key === 'c' || e.key === 'C') && e.type === 'keydown') {
      const sel = getTerminal() ? getTerminalSelection(getTerminal()!) : undefined
      if (sel) {
        e.preventDefault()
        writeClipboard(sel)
        return false
      }
      return true
    }

    // macOS-style cursor word/line jumping via Option/Cmd + arrow keys
    if (e.type === 'keydown' && (e.altKey || e.metaKey)) {
      if (e.key === 'ArrowLeft') {
        e.preventDefault()
        if (e.metaKey) {
          // Cmd+Left → beginning of line
          queuedSessionWrite(getSessionId() || '', '\x1b[H')
        } else if (e.altKey) {
          // Option+Left → backward word
          queuedSessionWrite(getSessionId() || '', '\x1bb')
        }
        return false
      }
      if (e.key === 'ArrowRight') {
        e.preventDefault()
        if (e.metaKey) {
          // Cmd+Right → end of line
          queuedSessionWrite(getSessionId() || '', '\x1b[F')
        } else if (e.altKey) {
          // Option+Right → forward word
          queuedSessionWrite(getSessionId() || '', '\x1bf')
        }
        return false
      }
    }

    // Suggestion navigation (only on keydown, ignore keyup)
    if (suggestions.isVisible() && e.type === 'keydown') {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        suggestions.selectNext()
        return false
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        suggestions.selectPrev()
        return false
      }
      if (e.key === 'Tab') {
        const selected = suggestions.getSelectedItem()
        if (selected) {
          e.preventDefault()
          applySuggestion(selected)
          return false
        }
      }
      if (e.key === 'Enter') {
        // Only apply suggestion if user explicitly selected one with arrow keys
        const selected = suggestions.getSelectedItem()
        if (selected) {
          e.preventDefault()
          applySuggestion(selected)
          return false
        }
        // No selection: let xterm handle Enter normally (terminal command execution)
      }
      if (e.key === 'Escape') {
        suggestions.close()
        return false
      }
    }

    return true
  }

  async function pasteToSession(text: string) {
    if (getMode() === 'ssh' || getMode() === 'local') {
      // Normalize line endings (CRLF -> LF, drop stray CR) so pasting into
      // editors like vim doesn't double newlines (Windows clipboard has \r\n).
      const normalized = normalizePastedText(text)

      // Broadcast mode: write to every target panel instead of only this
      // session (issue #597 — pasted command must sync like keyboard input).
      if (getBroadcastActive()) {
        const targets = tabStore.getAllBroadcastPanelIds()
        if (targets.length > 0) {
          for (const pid of targets) {
            const p = panelStore.getPanel(pid)
            const managed = p?.sessionId ? getManagedTerminal(p.sessionId) : undefined
            if (p?.sessionId && (p.type === 'ssh' || p.type === 'local' || p.type === 'wsl') && managed) {
              // Bracket per target session — each has its own paste mode.
              // Scroll each target's viewport to the bottom too, so broadcast
              // paste behaves like keyboard input everywhere (issue 629).
              pasteWithScroll(
                {
                  bracketedPasteMode: managed.terminal.modes.bracketedPasteMode,
                  write: (payload) => queuedSessionWrite(p.sessionId!, payload),
                  scrollToBottom: () => managed.terminal.scrollToBottom(),
                },
                normalized,
              )
            }
          }
          return
        }
      }

      const sid = getSessionId()
      if (sid) {
        const managed = getManagedTerminal(sid)
        pasteWithScroll(
          {
            bracketedPasteMode: managed?.terminal.modes.bracketedPasteMode ?? false,
            write: (payload) => queuedSessionWrite(sid, payload),
            scrollToBottom: () => getTerminal()?.scrollToBottom(),
          },
          normalized,
        )
      }
    }
  }

  return {
    handleTerminalKey,
    writeTerminalInput,
    resetIMEState,
    resetIMEComposition,
    pasteToSession,

  }
}