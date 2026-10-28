import type { Terminal } from '@xterm/xterm'
import { queuedSessionWriteObserved as queuedSessionWrite } from '../services/terminalCommandWrite'
import { stampCommandLine } from '../services/terminalTimestamps'
import { getManagedTerminal, bumpOnDataGeneration } from '../services/terminalManager'
import { createKeystrokeReorder } from '../utils/imeKeystrokeOrder'
import { stripDeviceReplies } from '../utils/terminalInputFilter'
import { applyMobileCtrl } from '../utils/mobileCtrlKey'
import { useSettingsStore } from '../stores/settingsStore'
import type { useTerminalInput } from './useTerminalInput'
import type { useSuggestions } from './useSuggestions'
import type { useTerminalKeyHandler } from './useTerminalKeyHandler'
import type { useTerminalZmodem } from './useTerminalZmodem'

/** Shared mutable containers so the host and composable stay in sync. */
export interface ReplySuppressFlag { value: number }
export interface RetryFlag { value: boolean }
export interface InputBufferRef { value: string }

export interface UseTerminalDataFlowOptions {
  getTerminal: () => Terminal | null
  getMode: () => 'ssh' | 'sftp' | 'local'
  getSessionId: () => string | null | undefined
  getOnSessionStatus: () => ((status: string) => void) | undefined
  keyHandler: ReturnType<typeof useTerminalKeyHandler>
  suggestions: ReturnType<typeof useSuggestions>
  getTerminalInput: () => ReturnType<typeof useTerminalInput> | null
  zmodem: ReturnType<typeof useTerminalZmodem>
  applySuggestion: (item: ReturnType<typeof useSuggestions>['getSelectedItem'] extends () => infer T ? T : never) => Promise<void>
  isMobileTouch: boolean
  replySuppressUntil: ReplySuppressFlag
  retryOnEnter: RetryFlag
}

/**
 * Keystroke pipeline: onData → IME reorder → processTerminalData.
 *
 * Owns the xterm onData listener, the IME keystroke reorder buffer, the
 * SFTP line buffer, and the listener dispose handles. The host calls
 * bindListeners() from onMounted/onActivated and disposeListeners() from
 * onDeactivated/onUnmounted. replySuppressUntil and retryOnEnter are
 * shared mutables so onActivated (gap replay) and session:status can
 * flip them while processTerminalData reads them here.
 */
export function useTerminalDataFlow(options: UseTerminalDataFlowOptions) {
  const {
    getTerminal, getMode, getSessionId, getOnSessionStatus,
    keyHandler, suggestions, getTerminalInput, zmodem, applySuggestion,
    isMobileTouch, replySuppressUntil, retryOnEnter,
  } = options

  const settingsStore = useSettingsStore()

  let processTerminalData: (data: string) => void = () => {}
  // Input methods can deliver a rewrite's delete and its replacement out of
  // order; this restores the intent before anything downstream sees it.
  const imeOrder = createKeystrokeReorder((data) => processTerminalData(data))
  let onDataDispose: { dispose(): void } | null = null
  let keyHandlerDispose: { dispose(): void } | null = null
  const inputBuffer: InputBufferRef = { value: '' }

  function bindListeners() {
    // Dispose previous listeners before re-registering
    onDataDispose?.dispose()
    onDataDispose = null
    keyHandlerDispose?.dispose()
    keyHandlerDispose = null

    // Bump the TERMINAL-SHARED generation counter so that ALL
    // components sharing this terminal can detect stale callbacks.
    // Per-component counter allowed KeepAlive-cached duplicate
    // components to both pass their independent guards.
    const sidNow = getSessionId()
    const gen = sidNow ? bumpOnDataGeneration(sidNow) : 0

    const terminal = getTerminal()
    if (!terminal) return
    terminal.attachCustomKeyEventHandler(keyHandler.handleTerminalKey)
    keyHandlerDispose = { dispose() {} }

    // Input handling
    onDataDispose = terminal.onData((data) => {
      // ── Stale callback guard (terminal-shared) ──
      // Check against the MANAGED terminal's current generation.
      // If another component (e.g. KeepAlive-cached) has registered
      // a newer handler on the same shared terminal, bail out.
      const curGen = sidNow ? (getManagedTerminal(sidNow)?.onDataGeneration ?? gen) : gen
      if (gen !== curGen) {
        return
      }

      // Gap-replay suppression: while stale queries from the replay are being
      // re-parsed, drop xterm's auto-generated device replies. The mirror
      // already answered them in real time (responder handoff, see
      // terminalManager); a second reply reaches the remote after its read
      // timed out and leaks into the shell. Keystrokes can never match these
      // escape shapes, so real input is untouched.
      if (Date.now() < replySuppressUntil.value) {
        const cleaned = stripDeviceReplies(data)
        if (!cleaned) return
        data = cleaned
      }

      // Mobile sticky Ctrl: transform the next soft-keyboard character into
      // the corresponding Ctrl combo (e.g. 'c' → ^C) when Ctrl is armed.
      if (isMobileTouch && (getMode() === 'ssh' || getMode() === 'local')) {
        const transformed = applyMobileCtrl(data)
        if (transformed !== null) {
          keyHandler.writeTerminalInput(transformed, false)
          return
        }
      }

      // Hand the keystroke to the input-method reorder buffer, which
      // releases it (possibly resequenced) into processTerminalData.
      imeOrder.push(data)
    })

    // The rest of the keystroke pipeline, fed by the reorder buffer rather
    // than straight from onData. Everything downstream — the local line
    // buffer and the bytes sent to the session — therefore sees one
    // consistent order.
    processTerminalData = (data: string) => {
      if (getMode() === 'ssh' || getMode() === 'local') {
      if (retryOnEnter.value && (data === '\r' || data === '\n')) {
        retryOnEnter.value = false
        if (getOnSessionStatus()) {
          getOnSessionStatus()!('retry')
        }
        return
      }

      // Detect rz/sz command to hint zmodem transfer direction.
      // Must happen BEFORE terminalInput.handleInput because handleInput
      // clears the line buffer on Enter.
      const terminalInput = getTerminalInput()
      if ((data === '\r' || data === '\n') && terminalInput && !terminalInput.isInAlternateScreen()) {
        // Stamp the command line with the moment it was executed (timestamp
        // column). Must happen before handleInput clears the line buffer.
        if (getSessionId()) stampCommandLine(getSessionId()!, Date.now())
        const line = terminalInput.lineBuffer.value.trim()
        zmodem.handleCommand(line)
      }

      // Handle suggestions input (skip in alternate screen apps like vim/k9s)
      if (terminalInput && !terminalInput.isInAlternateScreen() && (getMode() === 'ssh' || getMode() === 'local')) {
        terminalInput.handleInput(data)

        // When suggestions are visible, intercept certain keys synchronously
        if (suggestions.isVisible()) {
          if (data === '\t') {
            const selected = suggestions.getSelectedItem()
            if (selected) {
              applySuggestion(selected)
              return
            }
          }
          if (data === '\r' || data === '\n') {
            const selected = suggestions.getSelectedItem()
            if (selected) {
              applySuggestion(selected)
              return
            }
          }
          if (data === '\x1b') {
            suggestions.close()
            return
          }
        }

        // Defer suggestion update/close so SessionWrite is not blocked
        const wasVisible = suggestions.isVisible()
        setTimeout(() => {
          const ti = getTerminalInput()
          if (!ti) return
          const smartOn = settingsStore.settings.terminal.smartCompletion ?? true
          if (!smartOn) {
            suggestions.close()
            return
          }
          // Don't show suggestions if line buffer was already cleared (e.g. Enter pressed)
          if (!ti.lineBuffer.value) {
            suggestions.close()
            return
          }
          // Only show suggestions if they were already visible or if the
          // input is a printable character (not arrow keys / navigation).
          const isPrintable = data.length === 1 && data >= ' '
          if (!wasVisible && !isPrintable) return
          if (ti.isAtLineEnd() && ti.currentToken.value && !ti.isPasswordMode()) {
            suggestions.updateSuggestions(ti.currentToken.value, settingsStore.settings.terminal.aiTranscription)
          } else {
            suggestions.close()
          }
        }, 0)
      } else if (terminalInput?.isInAlternateScreen()) {
        suggestions.close()
      }

      const sid = getSessionId()
      const inAlt = terminalInput?.isInAlternateScreen() ?? false
      if (sid) {

        // The suggestion buffer is only an estimate: native paste, shell
        // completion/history, and interactive programs can bypass it. Always
        // deliver Backspace and let the remote line editor enforce its bounds.
        keyHandler.writeTerminalInput(data, inAlt)
      }
    } else {
      // SFTP line buffering
      const terminal = getTerminal()
      for (let i = 0; i < data.length; i++) {
        const char = data[i]
        const code = data.charCodeAt(i)
        if (char === '\r' || char === '\n') {
          if (inputBuffer.value) {
            const sid = getSessionId()
            if (sid) {
              for (let j = 0; j < inputBuffer.value.length; j++) {
                terminal!.write('\b \b')
              }
              queuedSessionWrite(sid, inputBuffer.value)
            }
            inputBuffer.value = ''
          }
        } else if (code === 127 || char === '\b') {
          if (inputBuffer.value.length > 0) {
            inputBuffer.value = inputBuffer.value.slice(0, -1)
            terminal!.write('\b \b')
          }
        } else if (code >= 32 && code <= 126) {
          inputBuffer.value += char
          terminal!.write(char)
        }
      }
    }
  }
  }

  function disposeListeners() {
    onDataDispose?.dispose()
    onDataDispose = null
    keyHandlerDispose?.dispose()
    keyHandlerDispose = null
  }

  function disposeImeOrder() {
    imeOrder.dispose()
  }

  return { bindListeners, disposeListeners, disposeImeOrder, inputBuffer }
}