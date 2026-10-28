import { onMounted, onBeforeUnmount, onUnmounted, onActivated, onDeactivated, watch, nextTick } from 'vue'
import type { Ref } from 'vue'
import type { Terminal } from '@xterm/xterm'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Browser } from '@wailsio/runtime'
import { SessionResize } from '../../bindings/github.com/ys-ll/uniterm/app'

import { acquireTerminal, releaseTerminal, attachTerminal, detachTerminal, getManagedTerminal, transferTerminal, markTerminalActive } from '../services/terminalManager'
import { attachOverlayHighlighter } from './overlayHighlight'
import { useTerminalInput } from './useTerminalInput'
import type { useSuggestions } from './useSuggestions'
import { useSettingsStore } from '../stores/settingsStore'
import { useSessionStore } from '../stores/sessionStore'
import { useZmodemStore } from '../stores/zmodemStore'

/**
 * Terminal lifecycle orchestration: onMounted/onActivated/onDeactivated/watch/
 * onBeforeUnmount/onUnmounted. Owns scroll-position capture/restore and
 * terminal-options derivation. Extracted from BaseTerminal.vue to reduce
 * component size — all behaviour is preserved exactly.
 */
export function useTerminalLifecycle(ctx: {
  // Terminal instance management (closures over `let` variables in the component)
  getTerminal: () => Terminal | null
  setTerminal: (t: Terminal | null) => void
  getTerminalInput: () => ReturnType<typeof useTerminalInput> | null
  setTerminalInput: (ti: ReturnType<typeof useTerminalInput> | null) => void

  // Refs and shared mutable state
  terminalRef: Ref<HTMLDivElement | undefined>
  terminalInstanceRef: string
  isActive: Ref<boolean>

  replySuppressUntil: { value: number }
  retryOnEnter: { value: boolean }

  // Props getters
  getMode: () => 'ssh' | 'sftp' | 'local'
  getSessionId: () => string | null | undefined

  // Composable instances (methods used by lifecycle)
  nativeDrop: { bind: () => void; unbind: () => void }
  resize: { resize: () => void; setupObservers: () => void; teardownObservers: () => void }
  touchPan: { bindTouchEvents: () => void; unbindTouchEvents: () => void; touchPanStop: () => void }
  zmodem: { initZmodemService: (sid: string) => void; disposeZmodemService: (sid: string) => void; hasService: () => boolean }
  dataFlow: { bindListeners: () => void; disposeListeners: () => void; disposeImeOrder: () => void }
  render: {
    writeStamped: (data: string, onParsed?: () => void) => void
    setWrittenChunks: (n: number) => void
    getWrittenChunks: () => number
    getDcsReassembler: () => { feed: (data: string) => { text: string; dcs?: boolean }[] }
  }
  sessionEvents: { setup: () => void; teardown: () => void }
  keyHandler: { resetIMEComposition: () => void }
  suggestions: ReturnType<typeof useSuggestions>

  // Helpers
  getFitAddon: () => { fit: () => void } | undefined
  sanitizeTerminalHistory: (text: string) => string
}) {
  const {
    getTerminal, setTerminal, setTerminalInput,
    terminalRef, terminalInstanceRef, isActive, replySuppressUntil, retryOnEnter,
    getMode, getSessionId,
    nativeDrop, resize, touchPan, zmodem, dataFlow, render, sessionEvents, keyHandler, suggestions,
    getFitAddon, sanitizeTerminalHistory,
  } = ctx

  const settingsStore = useSettingsStore()
  const sessionStore = useSessionStore()
  const zmodemStore = useZmodemStore()

  // Viewport position (top line in the buffer) captured on KeepAlive
  // deactivation so reactivation can restore the user's scroll position
  // instead of jumping to the bottom. baseY at deactivation is also kept so
  // we can tell whether the user was scrolled to the bottom before
  // deactivation — if they were, new output during the inactive period
  // (replayed on activation) should pull the viewport down to the new
  // bottom; otherwise the viewport stays where the user left it.
  let savedViewportY: number | null = null
  let savedBaseY: number | null = null

  function getTerminalOptions() {
    const ts = settingsStore.settings.terminal
    return {
      fontSize: ts.fontSize || 13,
      fontFamily: ts.fontFamily,
      fallbackFont: ts.fallbackFont || '',
      fontWeight: ts.fontWeight || 400,
      themeName: ts.theme || 'dark',
      scrollback: ts.maxHistoryLines || 2500,
    }
  }

  // Restore the scroll position captured on deactivation. Returns true when the
  // position has landed (or there was nothing to restore) so callers can stop
  // retrying.
  //
  // xterm v6 moved scrolling into the VS Code Scrollable widget: .xterm-viewport
  // is no longer the scroll container and viewport._currentRowHeight is gone, so
  // the old "write scrollTop by hand" fallback silently did nothing. The position
  // now lives in the Scrollable and is derived from render dimensions — right
  // after KeepAlive activation the container can still be 0×0, where cell height
  // is unknown and any position we set clamps back to 0. So we bail while
  // dimensions are invalid and re-apply after each resize() retry instead.
  function restoreSavedScroll(): boolean {
    if (savedViewportY == null || savedBaseY == null) return true
    const terminal = getTerminal()
    const buf = terminal?.buffer?.active
    const core = (terminal as any)?._core
    const cellHeight: number = core?._renderService?.dimensions?.css?.cell?.height ?? 0
    if (!buf || cellHeight <= 0) return false

    const wasAtBottom = savedViewportY >= savedBaseY - 1
    const target = wasAtBottom ? buf.baseY : Math.min(savedViewportY, buf.length - 1)
    // scrollToLine(line, true) on the internal viewport skips the smooth-scroll
    // animation and syncs ydisp in the same tick; the public scrollToLine() would
    // animate the jump on every tab switch.
    const vp = core?._viewport
    if (typeof vp?.scrollToLine === 'function') {
      vp.scrollToLine(target, true)
    } else {
      terminal?.scrollToLine(target)
    }
    if (buf.viewportY !== target) return false
    savedViewportY = null
    savedBaseY = null
    return true
  }

  // ── onMounted ──
  onMounted(() => {
    nativeDrop.bind()
    if (!terminalRef.value) return

    // Acquire shared terminal from manager (or create if first mount)
    const opts = getTerminalOptions()
    const terminal = acquireTerminal(getSessionId() || '', terminalInstanceRef, opts, settingsStore.settings.customTerminalThemes)
    setTerminal(terminal)
    // Publish component activity to the terminal manager: screen-buffer readers
    // (AI output capture, prompt snapshots) must only trust the buffer while at
    // least one visible component drives it — a deactivated panel's buffer is
    // frozen because the live session:data handler below gates on isActive.
    markTerminalActive(getSessionId() || '', terminalInstanceRef, true)
    // Keyword highlighting scans the parsed buffer and overlays decorations;
    // the enable switch is re-read on every refresh, so toggling the setting
    // takes effect live. Applies to every terminal type.
    attachOverlayHighlighter(terminal)

    // Load WebLinksAddon per-component (has custom callbacks)
    let hoverEl: HTMLDivElement | null = null
    const webLinksAddon = new WebLinksAddon(
      (event, uri) => {
        if (event.ctrlKey || event.metaKey) {
          Browser.OpenURL(uri)
        }
      },
      {
        hover(event, _text, _location) {
          if (!hoverEl) {
            hoverEl = document.createElement('div')
            hoverEl.className = 'xterm-link-tooltip'
            terminal.element!.appendChild(hoverEl)
          }
          const rect = terminal.element!.getBoundingClientRect()
          hoverEl.textContent = 'Ctrl + Click to open'
          hoverEl.style.left = (event.clientX - rect.left + 12) + 'px'
          hoverEl.style.top = (event.clientY - rect.top - 28) + 'px'
          hoverEl.style.display = 'block'
        },
        leave() {
          if (hoverEl) {
            hoverEl.style.display = 'none'
          }
        },
      },
    )
    terminal.loadAddon(webLinksAddon)

    // Attach terminal DOM to this component's container
    attachTerminal(getSessionId() || '', terminalRef.value)

    // Touch panning: only wired on touch devices
    touchPan.bindTouchEvents()

    zmodem.initZmodemService(getSessionId() || '')

    // Initialize terminal input handling for SSH
    if (getMode() === 'ssh') {
      setTerminalInput(useTerminalInput(terminal, {
        mode: getMode(),
        sessionId: getSessionId(),
        enableHistory: true,
        onHistoryExtract: (command: string) => {
          suggestions.addHistoryCommand(command)
        },
        onResetSuppress: () => {
          suggestions.resetSuppress()
        },
      }))
      suggestions.loadHistory()
    }

    if (getMode() === 'ssh' || getMode() === 'local') {
      // Restore terminal content from session buffer on first mount only.
      // Subsequent mounts reuse the shared terminal whose buffer already
      // contains the correct content.
      const sid = getSessionId()
      const managed = getManagedTerminal(getSessionId() || '')
      const isNewTerminal = managed?.isNew
      if (sid && isNewTerminal) {
        const raw = sessionStore.getData(sid)
        const history = sanitizeTerminalHistory(raw)
        if (history) {
          // Highlighting is applied by the overlay renderer scanning the
          // buffer, so restored history is decorated the same way as
          // newly arriving lines after a tab switch.
          render.writeStamped(history)
        }
      }
      // Always sync writtenChunks to prevent onActivated from replaying
      // all session data when the terminal was reused (isNewTerminal=false).
      if (sid) {
        render.setWrittenChunks(sessionStore.getChunkCount(sid))
      }
      // Force initial resize with retries — needed because cell dimensions
      // may not be available immediately, and for reused terminals the cols/rows
      // may hold stale dimensions from the previous container.
      ;[50, 150, 300, 600, 1000, 2000].forEach(d => setTimeout(() => {
        const term = getTerminal()
        if (!term) return
        getFitAddon()?.fit()
        const sessionId = getSessionId()
        if (sessionId && term.cols > 0 && term.rows > 0) {
          SessionResize(sessionId, term.cols, term.rows).catch(() => {})
        }
      }, d))
    }

    sessionEvents.setup()

    dataFlow.bindListeners()

    // Resize observers + window/split resize listeners
    resize.setupObservers()
  })

  // ── onActivated ──
  onActivated(() => {
    // Component restored from KeepAlive cache.

    nativeDrop.bind()
    markTerminalActive(getSessionId() || '', terminalInstanceRef, true)

    // Replay session data that arrived while deactivated BEFORE
    // setting isActive = true. The session:data handler gates on
    // isActive, so new data would race with the gap replay and
    // advance writtenChunks, making the gap undetectable.
    // Uses a monotonic sequence number (getChunkCount), not an array
    // index, so front-trimming of the sessionStore buffer during a
    // long background session doesn't invalidate the tracking position.
    if (getSessionId()) {
      const sid = getSessionId()!
      const total = sessionStore.getChunkCount(sid)
      if (total > render.getWrittenChunks()) {
        const tail = sessionStore.getDataFromChunk(sid, render.getWrittenChunks())
        // Route the gap replay through the DCS reassembler like the live
        // path — a sixel sequence may span the deactivation boundary.
        // Suppress auto device replies for the whole replay (see
        // replySuppressUntil): the replayed gap contains the same queries the
        // mirror answered while this panel was frozen.
        replySuppressUntil.value = Date.now() + 10000
        const segs = render.getDcsReassembler().feed(tail)
        for (let i = 0; i < segs.length; i++) {
          render.writeStamped(segs[i].text, i === segs.length - 1 ? () => { replySuppressUntil.value = 0 } : undefined)
        }
        if (segs.length === 0) replySuppressUntil.value = 0
        render.setWrittenChunks(total)
      }
    }

    // Sync retryOnEnter from stored session status. The session:status
    // event is guarded by isActive (which was false during deactivation),
    // so if the session disconnected while we were cached, retryOnEnter
    // was never set and Enter would do nothing despite the reconnect
    // message being replayed above.
    if (getSessionId()) {
      const sid = getSessionId()!
      const st = sessionStore.getStatus(sid)
      if (st === 'disconnected' || st === 'error') {
        retryOnEnter.value = true
      }
    }

    isActive.value = true
    // Re-attach terminal element — another component may have moved it while
    // we were cached (e.g. merge→drag-out→re-merge keeps panelId, KeepAlive
    // reuses the cached BaseTerminal, but the terminal is in holding).
    if (terminalRef.value && getSessionId()) {
      attachTerminal(getSessionId()!, terminalRef.value)
      nextTick(() => getFitAddon()?.fit())
    }
    // Re-register onData/keyHandler listeners that were disposed in onDeactivated.
    dataFlow.bindListeners()
    // Restore the user's scroll position captured on deactivation. Try once now
    // (cheap when layout already settled), then again after each resize() retry —
    // restoreSavedScroll() bails while render dimensions are still invalid, which
    // is the normal state right after KeepAlive activation.
    //
    // Terminal dimensions may also be stale after a tab switch; resize() bails
    // when the rect is 0×0, so it is retried on the same schedule. Both are
    // self-clearing: restoreSavedScroll() nulls the saved position once it lands
    // and returns true immediately afterwards.
    restoreSavedScroll()
    resize.resize()
    ;[0, 50, 150, 300, 600].forEach(d => setTimeout(() => {
      resize.resize()
      restoreSavedScroll()
    }, d))
    // Re-initialize zmodem service only if it was disposed in onDeactivated.
    // If a transfer was active, the service is still running — skip recreate.
    // safe: no focus() call here, avoids WebView2 crash race with native dialogs.
    if (getSessionId() && getMode() === 'ssh' && !zmodem.hasService()) {
      zmodem.initZmodemService(getSessionId()!)
    }
    // Note: focus() is intentionally skipped here. Calling focus() during
    // activation can race with native dialogs (OpenDirectoryDialog etc.)
    // and trigger a WebView2 crash (edge.Chromium.Focus parameter error).
  })

  // ── onDeactivated ──
  onDeactivated(() => {
    // Component deactivated by KeepAlive (e.g. terminal tab moved into workspace).
    // Mark inactive so session event handlers become no-ops.
    nativeDrop.unbind()
    isActive.value = false
    // Buffer freezes from here (live session:data handler gates on isActive) —
    // publish it so buffer readers stop trusting the screen.
    markTerminalActive(getSessionId() || '', terminalInstanceRef, false)
    // Reset IME composition state so the OS IME doesn't continue feeding
    // characters into the textarea while the terminal is hidden. Without
    // this, the stale composition state causes input duplication when the
    // user switches back (issue: IME offscreen duplicate input).
    keyHandler.resetIMEComposition()
    // Capture viewport position before listeners are torn down so reactivation
    // can restore the user's scroll position. Reading from the public IBuffer
    // API avoids depending on internal _core field shape.
    const terminal = getTerminal()
    const buf = terminal?.buffer?.active as { viewportY?: number; baseY?: number } | undefined
    if (buf && typeof buf.viewportY === 'number') {
      savedViewportY = buf.viewportY
    }
    if (buf && typeof buf.baseY === 'number') {
      savedBaseY = buf.baseY
    }
    // Dispose per-component listeners to prevent duplicate input when another
    // BaseTerminal mounts with the same shared terminal instance.
    dataFlow.disposeListeners()

    // If a transfer is still active, keep the service running so the background
    // transfer continues. Otherwise dispose and restore backend state.
    const hasStoreTransfer = zmodemStore.getActiveTransfer(getSessionId() || '')
    if (hasStoreTransfer) {
      return
    }
    zmodem.disposeZmodemService(getSessionId() || '')
  })

  // ── watch sessionId ──
  watch(() => getSessionId(), (newId, oldId) => {
    if (oldId && oldId !== newId) {
      if (terminalRef.value) detachTerminal(oldId, terminalRef.value)
      zmodem.disposeZmodemService(oldId)
      // Publish the active state before/after the terminal object moves between
      // session ids: transferTerminal relocates the whole ManagedTerminal
      // (activeRefs travel with it), so the old id must drop the ref first and
      // the new id re-add it after.
      markTerminalActive(oldId, terminalInstanceRef, false)
      // Transfer the terminal to the new sessionId so scrollback is
      // preserved across reconnects. releaseTerminal is intentionally
      // skipped — we want to keep the same terminal instance alive.
      if (newId) transferTerminal(oldId, newId)
      markTerminalActive(newId || '', terminalInstanceRef, true)
    }
    // Reset write tracking when session changes so onActivated replay
    // starts from the correct offset for the new session.
    render.setWrittenChunks(0)
    // Reset saved scroll position — the new sessionId has a different buffer.
    savedViewportY = null
    savedBaseY = null
    if (newId && (getMode() === 'ssh' || getMode() === 'local')) {
      zmodem.initZmodemService(newId)
      setTerminal(getManagedTerminal(newId)?.terminal ?? null)
      if (terminalRef.value) {
        attachTerminal(newId, terminalRef.value)
      }
      // Re-create terminalInput with the new terminal reference.
      // Otherwise it would still hold the old (disposed) terminal and
      // cursor position tracking returns {0,0}, pinning the suggestion
      // popup to the top-left corner.
      if (getMode() === 'ssh') {
        const terminal = getTerminal()
        if (terminal) {
          setTerminalInput(useTerminalInput(terminal, {
            mode: getMode(),
            sessionId: newId,
            enableHistory: true,
            onHistoryExtract: (command: string) => {
              suggestions.addHistoryCommand(command)
            },
            onResetSuppress: () => {
              suggestions.resetSuppress()
            },
          }))
          suggestions.loadHistory()
        }
      }
      // Re-bind onData/keyHandler on the new terminal
      dataFlow.bindListeners()
      const delays = [200, 400, 600, 800, 1000, 1500, 2000]
      delays.forEach((delay) => {
        setTimeout(() => resize.resize(), delay)
      })
    }
  })

  // ── onBeforeUnmount ──
  // Detach terminal before Vue nulls template refs.
  // In Vue 3, onUnmounted fires AFTER template refs are set to null,
  // so detachTerminal would see terminalRef.value === null and skip.
  // onBeforeUnmount fires while refs are still valid.
  onBeforeUnmount(() => {
    if (getSessionId() && terminalRef.value) {
      detachTerminal(getSessionId()!, terminalRef.value)
    }
    markTerminalActive(getSessionId() || '', terminalInstanceRef, false)
    // Remove touch event listeners (refs still valid in onBeforeUnmount)
    touchPan.unbindTouchEvents()
  })

  // ── onUnmounted ──
  onUnmounted(() => {
    nativeDrop.unbind()
    // Release anything the reorder buffer is still holding rather than dropping
    // a half-finished rewrite on the floor.
    dataFlow.disposeImeOrder()
    resize.teardownObservers()
    touchPan.touchPanStop()

    // Dispose per-component listeners BEFORE releasing terminal.
    // The terminal instance may outlive this component if another
    // component still holds a reference.
    dataFlow.disposeListeners()

    // Release reference (delayed dispose: terminal survives 500ms)
    if (getSessionId()) {
      releaseTerminal(getSessionId()!, terminalInstanceRef)
    }

    sessionEvents.teardown()
    suggestions.close()
    if (!zmodemStore.getActiveTransfer(getSessionId() || '')) {
      zmodem.disposeZmodemService(getSessionId() || '')
    }
  })
}