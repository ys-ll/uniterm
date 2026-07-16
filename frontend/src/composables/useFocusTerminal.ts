/**
 * Terminal-focus helpers shared across the app.
 *
 * xterm.js listens for keystrokes on an internal <textarea class="xterm-helper-textarea">.
 * To type into a terminal it must be the document's active element. Any UI action
 * that writes to a session — quick commands, history replay, paste — should
 * restore that focus so the user can immediately press Enter without an extra
 * click. And chrome-only mouse gestures (dragging the frameless window, clicking
 * the sidebar scrollbar) must not steal focus away from the terminal either.
 *
 * All lookups are DOM-based (via `[data-panel-id="..."]`) so we avoid the ref
 * plumbing that would be needed to reach BaseTerminal from unrelated components.
 */

import { useTabStore } from '../stores/tabStore'

/**
 * Focus the terminal in the given panel. Retries a few times because xterm's
 * internal textarea can be temporarily absent right after a KeepAlive
 * re-mount or during a session reconnect.
 */
export function focusPanelTerminal(panelId: string, attempt = 0) {
  if (attempt > 10) return
  const el = document.querySelector(`[data-panel-id="${panelId}"] .xterm-helper-textarea`)
  if (el instanceof HTMLTextAreaElement) {
    el.focus()
  } else {
    setTimeout(() => focusPanelTerminal(panelId, attempt + 1), 100)
  }
}

/** Focus the terminal in the currently active panel of the active tab. */
export function focusActivePanelTerminal() {
  const tabStore = useTabStore()
  const panelId = tabStore.getActivePanelId()
  if (panelId) focusPanelTerminal(panelId)
}

/**
 * Install a document-level guard that restores terminal focus after mouse
 * gestures that would otherwise steal it from xterm without the user meaning to
 * — dragging the frameless window by the header, or dragging the sidebar
 * scrollbar. Genuine clicks on interactive controls (buttons, inputs, links)
 * are left alone; they change focus intentionally.
 *
 * Runs on `window` in the capture phase so the check fires before any
 * component handlers, and only arms the mouseup restore when focus was on a
 * terminal at mousedown time.
 *
 * Returns a teardown function.
 */
export function installTerminalFocusRestore(): () => void {
  const INTERACTIVE_SEL = 'input, textarea, select, button, a, [role="button"], [contenteditable="true"], .xterm-helper-textarea'

  // Walk ancestors looking for the --wails-draggable custom property so we
  // can tell a frameless-window drag region apart from ordinary chrome. Any
  // ancestor tagged `no-drag` short-circuits to false — same precedence Wails
  // itself uses when deciding whether to hand the mouse to the OS.
  const isWailsDraggable = (el: HTMLElement | null): boolean => {
    let cur = el
    while (cur) {
      const val = getComputedStyle(cur).getPropertyValue('--wails-draggable').trim()
      if (val === 'drag') return true
      if (val === 'no-drag') return false
      cur = cur.parentElement
    }
    return false
  }

  const onMouseDown = (e: MouseEvent) => {
    const active = document.activeElement
    const wasTerminal = active instanceof HTMLElement &&
      active.classList.contains('xterm-helper-textarea')
    if (!wasTerminal) return

    const target = e.target as HTMLElement | null
    // If the press lands on an interactive control the user is deliberately
    // moving focus — do not fight it.
    if (target && target.closest(INTERACTIVE_SEL)) return

    const capturedPanelId = active.closest('[data-panel-id]')?.getAttribute('data-panel-id') || null

    const restore = () => {
      const now = document.activeElement
      if (now instanceof HTMLElement && now.classList.contains('xterm-helper-textarea')) {
        return // browser kept us on the terminal, nothing to do
      }
      if (capturedPanelId) {
        focusPanelTerminal(capturedPanelId)
      } else {
        focusActivePanelTerminal()
      }
    }

    // Frameless-window drag: as soon as mousedown fires inside a
    // --wails-draggable: drag region, WebView2 hands the mouse to the OS and
    // no mouseup ever reaches the DOM. Fall back to a short-delay reclaim
    // instead of waiting on an event that will never arrive.
    if (isWailsDraggable(target)) {
      setTimeout(restore, 100)
      return
    }

    // Otherwise (sidebar scrollbar, empty regions) we can wait for the real
    // mouseup so we don't fight the browser's own focus bookkeeping mid-drag.
    const onUp = () => {
      window.removeEventListener('mouseup', onUp, true)
      // Defer so the browser has finished its own focus bookkeeping for the
      // gesture before we try to reclaim.
      setTimeout(restore, 0)
    }
    window.addEventListener('mouseup', onUp, true)
  }

  window.addEventListener('mousedown', onMouseDown, true)
  return () => window.removeEventListener('mousedown', onMouseDown, true)
}
