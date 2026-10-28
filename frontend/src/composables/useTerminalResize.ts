import type { Ref } from 'vue'
import type { Terminal } from '@xterm/xterm'
import { nextTick } from 'vue'
import { SessionResize } from '../../bindings/github.com/ys-ll/uniterm/app'
import { Events } from '@wailsio/runtime'

export interface UseTerminalResizeOptions {
  getTerminal: () => Terminal | null
  terminalRef: Ref<HTMLDivElement | undefined>
  getMode: () => 'ssh' | 'sftp' | 'local'
  getSessionId: () => string | null | undefined
  getFitAddon: () => { fit(): void } | undefined
}

/**
 * Terminal resize logic: fit/resize the xterm instance, debounce window and
 * split-pane resizes, and observe the host element for size changes.
 *
 * Extracted from BaseTerminal.vue so the core component can stay focused on
 * session data flow. The composable owns the resize timers/gates and the
 * ResizeObserver/IntersectionObserver lifecycle; the host calls
 * `setupObservers()` in onMounted and `teardownObservers()` in onUnmounted.
 */
export function useTerminalResize(options: UseTerminalResizeOptions) {
  const { getTerminal, terminalRef, getMode, getSessionId, getFitAddon } = options

  let resizeTimer: ReturnType<typeof setTimeout> | null = null
  // Trailing resize for size changes observed while a resize gate (window
  // resize debounce / split drag / suppress window) was active. Separate from
  // resizeTimer so it never cancels another handler's pending resize.
  let deferredResizeTimer: ReturnType<typeof setTimeout> | null = null
  let unsubNativeResizeEnd: (() => void) | null = null
  let isResizing = false
  let splitResizing = false
  let suppressResizeUntil = 0
  let resizeObserver: ResizeObserver | null = null
  let intersectionObserver: IntersectionObserver | null = null

  function resize() {
    if (getMode() === 'ssh' || getMode() === 'local') {
      const sid = getSessionId()
      const terminal = getTerminal()
      if (!terminal || !sid) return
      const fitAddon = getFitAddon()
      if (!fitAddon) return
      const el = terminalRef.value
      if (!el) return

      const rect = el.getBoundingClientRect()
      // Skip resize when the component is hidden (e.g. during tab switching
      // with KeepAlive). A zero-size resize would corrupt xterm.js buffers.
      if (rect.width === 0 || rect.height === 0) return

      // Heal any stale scroll offset on the host: a focus()-driven auto-scroll
      // during a resize transition can leave it scrolled (content displaced
      // upward, blank space below). The host is never scrolled deliberately.
      if (el.scrollTop !== 0) {
        el.scrollTop = 0
      }

      // Always fit first to update CSS dimensions on the .xterm element.
      // Without this, stale inline pixel dimensions from a previous fit()
      // prevent the terminal from filling the container after window resize
      // (e.g. after duplicating a session tab).
      fitAddon.fit()
      if (terminal.cols <= 0 || terminal.rows <= 0) return

      // Trust fitAddon.fit() — its measure already accounts for the scrollbar
      // gutter; manual recomputation double-subtracts it.
      terminal.resize(terminal.cols, terminal.rows)
      // Full-viewport redraw — prevents canvas ghosting during rapid write bursts.
      terminal.refresh(0, terminal.rows - 1)
      SessionResize(sid, terminal.cols, terminal.rows).catch(() => {})
    } else {
      getFitAddon()?.fit()
    }
  }

  function onWindowResize() {
    const el = terminalRef.value
    if (!el) return
    if (!isResizing) {
      isResizing = true
      el.classList.add('resizing')
    }
    if (resizeTimer) clearTimeout(resizeTimer)
    resizeTimer = setTimeout(() => {
      isResizing = false
      el.classList.remove('resizing')
      resize()
    }, 400)
  }

  // Native window finished resizing (drag-to-restore / maximize / programmatic
  // resize). Go emits this after WM_EXITSIZEMOVE / WM_SIZE settles, because the
  // browser does not always fire a final window.resize at the settled size after
  // a native drag — which would leave the canvas at a stale small row count with
  // blank space below (issue #656). Bypass the isResizing gate so this cleanup
  // fit always runs, then resolve for a clean re-fit at the true container size.
  function onNativeResizeEnd() {
    if (resizeTimer) clearTimeout(resizeTimer)
    isResizing = false
    terminalRef.value?.classList.remove('resizing')
    resizeTimer = setTimeout(() => resize(), 100)
  }

  function onSplitResizeStart() {
    splitResizing = true
  }

  function onSplitResizeEnd() {
    splitResizing = false
    if (resizeTimer) {
      clearTimeout(resizeTimer)
      resizeTimer = null
    }
    suppressResizeUntil = Date.now() + 200
    nextTick(() => {
      setTimeout(() => {
        void terminalRef.value?.offsetWidth
        resize()
      }, 0)
    })
  }

  // Wire window/document event listeners and create observers. Call from the
  // host's onMounted, after the terminal element is attached.
  function setupObservers() {
    window.addEventListener('resize', onWindowResize)
    window.addEventListener('split:resize-start', onSplitResizeStart)
    window.addEventListener('split:resize-end', onSplitResizeEnd)
    unsubNativeResizeEnd = Events.On('window:resize-end', () => onNativeResizeEnd())

    const el = terminalRef.value
    if (!el) return

    resizeObserver = new ResizeObserver(() => {
      // A size change landing inside a resize gate must not be dropped: if it
      // were the last event (e.g. maximize/restore right after a split-drag),
      // the terminal would keep its old pixel size — blank space below — with
      // no further trigger. Park a trailing resize that runs once the gate
      // clears; harmless if another handler (window debounce, native resize
      // end) already fits at the settled size.
      if (isResizing || splitResizing || Date.now() < suppressResizeUntil) {
        if (deferredResizeTimer) clearTimeout(deferredResizeTimer)
        deferredResizeTimer = setTimeout(() => {
          deferredResizeTimer = null
          if (isResizing || splitResizing || Date.now() < suppressResizeUntil) return
          resize()
        }, Math.max(0, suppressResizeUntil - Date.now()) + 150)
        return
      }
      if (!terminalRef.value) return
      if (resizeTimer) clearTimeout(resizeTimer)
      resizeTimer = setTimeout(() => resize(), 150)
    })
    resizeObserver.observe(el)

    if (getMode() === 'ssh') {
      intersectionObserver = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
          if (entry.isIntersecting) {
            resize()
          }
        })
      })
      intersectionObserver.observe(el)
    }
  }

  // Tear down all observers and listeners. Call from the host's onUnmounted.
  function teardownObservers() {
    resizeObserver?.disconnect()
    intersectionObserver?.disconnect()
    if (deferredResizeTimer) {
      clearTimeout(deferredResizeTimer)
      deferredResizeTimer = null
    }
    window.removeEventListener('resize', onWindowResize)
    window.removeEventListener('split:resize-start', onSplitResizeStart)
    window.removeEventListener('split:resize-end', onSplitResizeEnd)
    unsubNativeResizeEnd?.()
    unsubNativeResizeEnd = null
  }

  return {
    resize,
    onWindowResize,
    onNativeResizeEnd,
    onSplitResizeStart,
    onSplitResizeEnd,
    setupObservers,
    teardownObservers,
  }
}