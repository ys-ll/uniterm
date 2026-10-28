import type { Terminal } from '@xterm/xterm'
import type { Ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { SessionResize } from '../../bindings/github.com/ys-ll/uniterm/app'
import { queuedSessionWriteObserved as queuedSessionWrite } from '../services/terminalCommandWrite'
import { getTerminalSelection } from '../services/terminalCommandWrap'
import { useSettingsStore } from '../stores/settingsStore'
import { usePanelStore } from '../stores/panelStore'
import { useTabStore } from '../stores/tabStore'
import { writeClipboard, readClipboardText } from './useClipboardWrite'
import { isAuthFailureError } from '../utils/authError'
import type { useSuggestions } from './useSuggestions'
import type { useTerminalKeyHandler } from './useTerminalKeyHandler'
import type { useTerminalRecorder } from './useTerminalRecorder'
import type { useTerminalZmodem } from './useTerminalZmodem'
import type { useTerminalMenu } from './useTerminalMenu'

/** Mutable container shared with the host so session:status can flip retry mode. */
export interface RetryFlag { value: boolean }

export interface UseTerminalSessionEventsOptions {
  getTerminal: () => Terminal | null
  isActive: Ref<boolean>
  getSessionId: () => string | null | undefined
  getPanelId: () => string | undefined
  getMode: () => 'ssh' | 'sftp' | 'local'
  getOnSessionStatus: () => ((status: string) => void) | undefined
  terminalRef: Ref<HTMLDivElement | undefined>
  searchRef: Ref<{ openSearch(): void } | null>
  retryOnEnter: RetryFlag
  zmodem: ReturnType<typeof useTerminalZmodem>
  renderTerminalData: (data: string) => void
  suggestions: ReturnType<typeof useSuggestions>
  keyHandler: ReturnType<typeof useTerminalKeyHandler>
  recorder: ReturnType<typeof useTerminalRecorder>
  menu: ReturnType<typeof useTerminalMenu>
  onResize: () => void
  focus: () => void
}

/**
 * Sets up and tears down all per-component event listeners:
 * xterm onSelectionChange, document mousedown/auxclick, session:data,
 * session:status, window terminal:* events, and visibilitychange.
 *
 * The host calls setup() inside onMounted (after bindListeners is defined)
 * and teardown() inside onUnmounted. retryOnEnter is a shared mutable so
 * the session:status handler can flip it and processTerminalData can read it.
 */
export function useTerminalSessionEvents(options: UseTerminalSessionEventsOptions) {
  const {
    getTerminal, isActive, getSessionId, getPanelId, getMode, getOnSessionStatus,
    terminalRef, searchRef, retryOnEnter, zmodem, renderTerminalData,
    suggestions, keyHandler, recorder, menu, onResize, focus,
  } = options

  const settingsStore = useSettingsStore()
  const panelStore = usePanelStore()
  const tabStore = useTabStore()

  let unsubscribe: (() => void) | null = null
  let statusUnsubscribe: (() => void) | null = null
  let onDocumentMouseDown: ((e: MouseEvent) => void) | null = null
  let onTerminalAuxClick: ((e: MouseEvent) => void) | null = null
  let onOpenSearch: ((e: Event) => void) | null = null
  let onExport: ((e: Event) => void) | null = null
  let onResetOutput: ((e: Event) => void) | null = null
  let onClearScrollback: ((e: Event) => void) | null = null
  let onSendRz: ((e: Event) => void) | null = null
  let onTerminalCopy: ((e: Event) => void) | null = null
  let onTerminalPaste: ((e: Event) => void) | null = null
  let onNativeTerminalCopy: ((e: ClipboardEvent) => void) | null = null
  let onVisibilityChange: (() => void) | null = null

  function setup() {
    const terminal = getTerminal()
    if (!terminal) return

    // Selection action: copy to clipboard via xterm's native selection event.
    // Use setTimeout to let xterm finish processing the selection (especially
    // for double-click word selection) before reading getSelection().
    let lastSelectionText = ''
    terminal.onSelectionChange(() => {
      if (settingsStore.settings.terminal.selectionAction !== 'copy') return
      setTimeout(() => {
        const text = getTerminal() ? getTerminalSelection(getTerminal()!) : undefined
        if (!text) {
          // Selection cleared: reset the dedup guard, or re-selecting the same
          // text would never re-copy and a lost write could never be retried.
          lastSelectionText = ''
          return
        }
        if (text !== lastSelectionText) {
          lastSelectionText = text
          writeClipboard(text)
        }
      }, 0)
    })

    // Close suggestion popup when clicking outside
    onDocumentMouseDown = (event: MouseEvent) => {
      if (!suggestions.isVisible()) return
      const baseTerminalEl = terminalRef.value?.closest('.base-terminal')
      const popupEl = baseTerminalEl?.querySelector('.terminal-suggestion-popup')
      if (popupEl && !popupEl.contains(event.target as Node)) {
        suggestions.close()
      }
    }
    document.addEventListener('mousedown', onDocumentMouseDown)

    // Middle-click action: read setting. 'paste' pastes the clipboard, 'menu'
    // opens the context menu at the cursor, 'none' does nothing.
    onTerminalAuxClick = (event: MouseEvent) => {
      if (event.button !== 1) return
      if (!terminalRef.value?.contains(event.target as Node)) return
      const action = settingsStore.settings.terminal.middleClickAction
      if (action === 'menu') {
        // auxclick exposes the same clientX/clientY as a contextmenu event, so
        // reuse the menu opener to position the <Menu> at the cursor. openMenu
        // (not onContextMenu) is used so it opens regardless of the right-click
        // action setting and never falls through to paste.
        menu.openMenu(event)
        return
      }
      if (action !== 'paste') return
      event.preventDefault()
      event.stopPropagation()
      readClipboardText().then(text => {
        if (text && getSessionId()) {
          keyHandler.pasteToSession(text)
        }
      })
    }
    document.addEventListener('auxclick', onTerminalAuxClick)

    // Session data
    unsubscribe = Events.On('session:data', (ev) => { const payload: { id: string; data: string } = ev.data;
      if (!isActive.value) {
        // Mark notification dot on the tab when inactive terminal receives output
        // Only process events for this instance's session (events are global)
        if (payload.id === getSessionId() && getSessionId()) {
          // Find the panel by sessionId (panels Map is keyed by panelId)
          let panelId: string | null = null
          for (const [id, p] of panelStore.panels) {
            if (p.sessionId === getSessionId()) {
              panelId = id
              break
            }
          }
          if (panelId) {
            const tab = tabStore.tabs.find(t =>
              (t.type === 'terminal' && t.panelId === panelId) ||
              (t.type === 'workspace' && t.panelIds.includes(panelId))
            )
            if (tab && tab.id !== tabStore.activeTabId) {
              tabStore.markTabNotification(tab.id)
            }
          }
        }
        return
      }
      if (payload.id !== getSessionId() || !getTerminal()) return

      // Zmodem interception: consume transfer data / detect HEX headers.
      if (zmodem.handleSessionData(payload.data)) return

      renderTerminalData(payload.data)
    })

    // SSH/Local: session status events
    if (getMode() === 'ssh' || getMode() === 'local') {
      retryOnEnter.value = false
      statusUnsubscribe = Events.On('session:status', (ev) => {
        const payload = ev.data as { id: string; status: string; errorMessage?: string }
        if (!isActive.value) return
        if (payload.id !== getSessionId()) return
        if (payload.status === 'connected') {
          retryOnEnter.value = false
          if (getOnSessionStatus()) {
            getOnSessionStatus()!(payload.status)
          }
          // Force send current terminal size to sync the backend PTY after reconnect.
          // The new session defaults to 80x24; without this, apps like vim/k9s use the wrong size.
          const terminal = getTerminal()
          if (terminal && terminal.cols > 0 && terminal.rows > 0) {
            SessionResize(getSessionId()!, terminal.cols, terminal.rows).catch(() => {})
          }
          onResize()
        } else if (payload.status === 'error') {
          retryOnEnter.value = true
          if (getOnSessionStatus()) {
            // 认证类失败（密码被拒）单独上报，让面板立即弹重新认证对话框，
            // 而不是让用户按回车再全量重连两次（issue #949）。
            getOnSessionStatus()!(isAuthFailureError(payload.errorMessage) ? 'auth_failed' : payload.status)
          }
          getTerminal()?.write('\r\n\x1b[31mConnection failed. Press Enter to retry.\x1b[0m\r\n')
        } else if (payload.status === 'disconnected') {
          retryOnEnter.value = true
          if (getOnSessionStatus()) {
            getOnSessionStatus()!(payload.status)
          }
          // Local shells exit for ordinary reasons (`exit`, or a child like
          // opencode's /exit tearing down the ConPT }), so say the shell ended
          // rather than implying a failure. Without this the pane just freezes
          // mid-output and looks hung — see the stray `+q4d73` case.
          //
          // Reset mouse tracking here too: an app that exited without
          // disabling it leaves selection broken, and the backend's
          // reset-on-Enter path needs a live session it no longer has.
          if (getMode() === 'local') {
            getTerminal()?.write(
              '\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1004l\x1b[?1005l\x1b[?1006l\x1b[?1015l' +
              '\r\n\x1b[33mShell exited. Press Enter to restart.\x1b[0m\r\n'
            )
          }
        } else {
          // Focus terminal on connecting so user can type password immediately.
          focus()
          if (getOnSessionStatus()) {
            getOnSessionStatus()!(payload.status)
          }
        }
      })
    }

    onOpenSearch = (e: Event) => {
      if (!isActive.value) return
      const detail = (e as CustomEvent).detail
      if (!getPanelId() || detail?.panelId !== getPanelId()) return
      searchRef.value?.openSearch()
    }
    window.addEventListener('terminal:open-search', onOpenSearch)

    onExport = (e: Event) => {
      if (!isActive.value) return
      const detail = (e as CustomEvent).detail
      if (!getPanelId() || detail?.panelId !== getPanelId()) return
      recorder.exportContent()
    }
    window.addEventListener('terminal:export', onExport)

    onResetOutput = (e: Event) => {
      if (!isActive.value) return
      const detail = (e as CustomEvent).detail
      if (!getPanelId() || detail?.panelId !== getPanelId()) return
      recorder.resetTerminalOutput()
    }
    window.addEventListener('terminal:reset-output', onResetOutput)

    onClearScrollback = (e: Event) => {
      if (!isActive.value) return
      const detail = (e as CustomEvent).detail
      if (!getPanelId() || detail?.panelId !== getPanelId()) return
      recorder.clearScrollback()
    }
    window.addEventListener('terminal:clear-scrollback', onClearScrollback)

    onSendRz = (e: Event) => {
      const detail = (e as CustomEvent).detail
      if (detail?.panelId && detail.panelId !== getPanelId()) return
      const sid = getSessionId()
      if (sid) {
        queuedSessionWrite(sid, 'rz -be\n')
      }
    }
    window.addEventListener('terminal:send-rz', onSendRz)

    // Configurable copy/paste shortcuts (Ctrl+Shift+C/V on Windows/Linux).
    // The App-level shortcut handler dispatches these events to the active panel.
    onTerminalCopy = (e: Event) => {
      if (!isActive.value) return
      const detail = (e as CustomEvent).detail
      if (detail?.panelId && detail.panelId !== getPanelId()) return
      const sel = getTerminal() ? getTerminalSelection(getTerminal()!) : undefined
      if (sel) {
        writeClipboard(sel)
      }
    }
    window.addEventListener('terminal:copy', onTerminalCopy)

    onTerminalPaste = (e: Event) => {
      if (!isActive.value) return
      const detail = (e as CustomEvent).detail
      if (detail?.panelId && detail.panelId !== getPanelId()) return
      if (getMode() === 'ssh' || getMode() === 'local') {
        readClipboardText().then(text => {
          if (text) keyHandler.pasteToSession(text)
        })
      }
    }
    window.addEventListener('terminal:paste', onTerminalPaste)

    // Native terminal copy event: when the command-wrap selection differs from
    // xterm's raw selection, override the clipboard with the wrapped version so
    // copying a hard-wrapped command yields the joined logical line.
    onNativeTerminalCopy = (event: ClipboardEvent) => {
      const terminal = getTerminal()
      if (!isActive.value || !terminal || !event.clipboardData) return
      const text = getTerminalSelection(terminal)
      if (text === terminal.getSelection()) return
      event.clipboardData.setData('text/plain', text)
      event.preventDefault()
      event.stopImmediatePropagation()
    }
    terminalRef.value?.addEventListener('copy', onNativeTerminalCopy, true)

    // When the browser tab/page becomes hidden (user switches to another app
    // or another browser tab), reset IME composition state. This prevents the
    // OS IME from continuing to feed characters into the hidden textarea,
    // which causes input duplication when the user returns.
    onVisibilityChange = () => {
      if (document.hidden && isActive.value) {
        keyHandler.resetIMEComposition()
      }
    }
    document.addEventListener('visibilitychange', onVisibilityChange)
  }

  function teardown() {
    unsubscribe?.()
    statusUnsubscribe?.()
    if (onDocumentMouseDown) {
      document.removeEventListener('mousedown', onDocumentMouseDown)
      onDocumentMouseDown = null
    }
    if (onTerminalAuxClick) {
      document.removeEventListener('auxclick', onTerminalAuxClick)
      onTerminalAuxClick = null
    }
    if (onOpenSearch) window.removeEventListener('terminal:open-search', onOpenSearch)
    if (onExport) window.removeEventListener('terminal:export', onExport)
    if (onResetOutput) window.removeEventListener('terminal:reset-output', onResetOutput)
    if (onClearScrollback) window.removeEventListener('terminal:clear-scrollback', onClearScrollback)
    if (onSendRz) window.removeEventListener('terminal:send-rz', onSendRz)
    if (onTerminalCopy) window.removeEventListener('terminal:copy', onTerminalCopy)
    if (onTerminalPaste) window.removeEventListener('terminal:paste', onTerminalPaste)
    if (onNativeTerminalCopy) {
      terminalRef.value?.removeEventListener('copy', onNativeTerminalCopy, true)
      onNativeTerminalCopy = null
    }
    if (onVisibilityChange) document.removeEventListener('visibilitychange', onVisibilityChange)
    onVisibilityChange = null
  }

  return { setup, teardown }
}