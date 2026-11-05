import type { Terminal } from '@xterm/xterm'

// IME compatibility patch for xterm.js on macOS (WKWebView).
//
// With an IME active, WKWebView can report ordinary keystrokes as keydown
// keyCode=229 even when no composition is active. xterm then diverts the
// character to its deferred textarea-diff path, which can drop characters.
// Force eligible single-character insertText events through xterm's direct
// path and rewind the textarea so the deferred diff cannot send them twice.
//
// After switching a Chinese IME to English mode with Caps Lock, WKWebView can
// also emit the input event before its corresponding keyCode-229 keydown. If
// that input event was already delivered, the late keydown must not inject a
// fallback. If no usable input event was delivered, the keydown fallback
// keeps the character from being swallowed.
//
// Some IMEs (e.g. Doubao) swallow keypress for Shift+letter without reporting
// keyCode 229, so uppercase had no delivery path left at all. xterm skips its
// direct path whenever `composed && _keyDownSeen`, so that is the condition we
// key off, rather than inferring it from an IME-specific keyCode.
//
// Event-tap IMEs (XKey/EVKey/OpenKey for Vietnamese) never compose at all; they
// backspace and retype, delivering the replacement as a multi-character
// `keydown.key`. That needs intercepting before keypress — see
// deliverKeyTextRewrite below.
//
// Windows (WebView2 / Chromium): xterm's CompositionHelper has two independent
// paths that can deliver one IME commit. `compositionend` queues a deferred
// send that slices the hidden textarea (_finalizeComposition), while the
// `input` event (inputType `insertText`) is delivered through _inputEvent.
// Chromium only runs the second path when _keyDownSeen is already false, which
// happens for candidate words picked with the mouse and for IMEs whose commit
// keyup lands before the input event. Both paths then send the word and the
// terminal shows it twice (reported with WeChat IME + opencode). The commit
// guard below keeps a short window after compositionend and drops a payload
// that was already sent in it, so whichever path wins the race the pty sees
// the word once. It also re-anchors the IME textarea on compositionstart.

export interface Disposable {
  dispose(): void
}

interface XtermCompositionHelperInternals {
  _isComposing?: unknown
  _isSendingComposition?: unknown
  isComposing?: unknown
  updateCompositionElements?: (dontRecurse?: boolean) => void
}

interface XtermCoreServiceInternals {
  triggerDataEvent?: (
    this: XtermCoreServiceInternals,
    data: string,
    wasUserInput?: boolean,
  ) => void
}

interface XtermCoreInternals {
  _inputEvent?: (this: XtermCoreInternals, ev: InputEvent) => boolean
  _keyDownSeen?: boolean
  _compositionHelper?: XtermCompositionHelperInternals
  coreService?: XtermCoreServiceInternals
  textarea?: {
    value: string
    addEventListener: (type: string, listener: EventListener, capture?: boolean) => void
    removeEventListener: (type: string, listener: EventListener, capture?: boolean) => void
  } | null
}

type TerminalWithCore = Terminal & { _core?: XtermCoreInternals }

interface PendingFallback {
  character: string
  textareaValueBeforeKeydown: string
  createdAt: number
  injected: boolean
  timeoutId: ReturnType<typeof setTimeout>
}

interface DeliveredInput {
  character: string
  deliveredAt: number
}

const PRINTABLE_ASCII = /^[\x20-\x7E]$/
// Every multi-character DOM key value is a CamelCase ASCII identifier
// ("Backspace", "ArrowUp", "F11", "Dead", "Process"). Anything else longer
// than one character is literal text the IME wants inserted.
const NAMED_KEY = /^[A-Z][A-Za-z0-9]*$/
const PRINTABLE_TEXT = /^[^\p{C}]+$/u
const NON_ASCII = /[^\x00-\x7F]/
const LATE_EVENT_WINDOW_MS = 250
// How long after compositionend a payload counts as part of that commit. The
// two xterm delivery paths land in the same task or in the 0ms timer queued by
// _finalizeComposition, so anything this late is a different keystroke.
const COMMIT_DEDUPE_WINDOW_MS = 100
const noopDisposable: Disposable = { dispose() {} }

function isMacPlatform(): boolean {
  return /Mac|iPhone|iPad/.test(navigator.userAgent)
}

function isCompositionActive(helper: XtermCompositionHelperInternals): boolean {
  return (
    helper._isComposing === true ||
    helper._isSendingComposition === true ||
    helper.isComposing === true
  )
}

function isSameCharacter(left: string, right: string): boolean {
  return left.toLowerCase() === right.toLowerCase()
}

// Escape sequences (arrow keys, mouse reports, device replies) and newlines
// start with a control character, so this separates real text payloads from
// protocol traffic without parsing the sequence.
function isPlainTextPayload(data: string): boolean {
  if (data.length === 0) return false
  for (let index = 0; index < data.length; index += 1) {
    const code = data.charCodeAt(index)
    if (code < 0x20 || code === 0x7f) return false
  }
  return true
}

// Exactly-once delivery for an IME commit. Returns null when the terminal does
// not expose the services it needs (older xterm builds, test fakes).
function installCompositionCommitGuard(
  core: XtermCoreInternals,
  helper: XtermCompositionHelperInternals,
): Disposable | null {
  const coreService = core.coreService
  const textarea = core.textarea
  if (
    !coreService ||
    typeof coreService.triggerDataEvent !== 'function' ||
    !textarea ||
    typeof textarea.addEventListener !== 'function'
  ) {
    return null
  }

  let commitWindowUntil = 0
  let windowTimer: ReturnType<typeof setTimeout> | null = null
  const committedPayloads = new Set<string>()

  const cancelCommitWindow = () => {
    if (windowTimer !== null) {
      clearTimeout(windowTimer)
      windowTimer = null
    }
    commitWindowUntil = 0
    committedPayloads.clear()
  }

  const closeCommitWindow = () => {
    windowTimer = null
    commitWindowUntil = 0
    committedPayloads.clear()
  }

  const onCompositionStart = () => {
    cancelCommitWindow()
    // xterm repositions the IME anchor on render and on compositionupdate
    // only. Doing it here keeps a textarea that a right-click parked under the
    // mouse from anchoring the candidate window at the mouse pointer.
    helper.updateCompositionElements?.()
  }

  const onCompositionEnd = () => {
    commitWindowUntil = Date.now() + COMMIT_DEDUPE_WINDOW_MS
    if (windowTimer !== null) clearTimeout(windowTimer)
    windowTimer = setTimeout(closeCommitWindow, COMMIT_DEDUPE_WINDOW_MS)
  }

  const originalTriggerDataEvent = coreService.triggerDataEvent
  const patchedTriggerDataEvent = function patchedTriggerDataEvent(
    this: XtermCoreServiceInternals,
    data: string,
    wasUserInput?: boolean,
  ): void {
    if (
      commitWindowUntil > 0 &&
      Date.now() < commitWindowUntil &&
      isPlainTextPayload(data) &&
      // A lone ASCII key is what fast typing and key auto-repeat produce.
      // IME commits are either multi-character or non-ASCII, so leave plain
      // single characters alone and only collapse those.
      (data.length > 1 || NON_ASCII.test(data))
    ) {
      // Both xterm commit paths send the same text; keep the first one.
      if (committedPayloads.has(data)) return
      committedPayloads.add(data)
    }
    originalTriggerDataEvent.call(this, data, wasUserInput)
  }

  coreService.triggerDataEvent = patchedTriggerDataEvent
  textarea.addEventListener('compositionstart', onCompositionStart, true)
  textarea.addEventListener('compositionend', onCompositionEnd, true)

  return {
    dispose() {
      cancelCommitWindow()
      textarea.removeEventListener('compositionstart', onCompositionStart, true)
      textarea.removeEventListener('compositionend', onCompositionEnd, true)
      if (coreService.triggerDataEvent === patchedTriggerDataEvent) {
        coreService.triggerDataEvent = originalTriggerDataEvent
      }
    },
  }
}

export function installImeCompatibilityPatch(terminal: Terminal): Disposable {
  const core = (terminal as TerminalWithCore)._core
  const helper = core?._compositionHelper
  if (!core || !helper) {
    return noopDisposable
  }

  // macOS has its own delivery patch below; the commit guard targets the
  // Chromium two-path commit that WebView2 and Linux webviews hit.
  if (!isMacPlatform()) {
    return installCompositionCommitGuard(core, helper) ?? noopDisposable
  }

  if (
    typeof core._inputEvent !== 'function' ||
    typeof core._keyDownSeen !== 'boolean' ||
    !core.textarea ||
    typeof core.textarea.addEventListener !== 'function'
  ) {
    return noopDisposable
  }

  let latestKeydownWas229 = false
  let forcedSinceKeydown = false
  let textareaValueBefore229Keydown = ''
  const pendingFallbacks: PendingFallback[] = []
  const deliveredInputs: DeliveredInput[] = []

  const removePendingFallback = (pendingFallback: PendingFallback) => {
    const index = pendingFallbacks.indexOf(pendingFallback)
    if (index >= 0) pendingFallbacks.splice(index, 1)
  }

  const pruneLateEventRecords = () => {
    const now = Date.now()
    for (let index = pendingFallbacks.length - 1; index >= 0; index -= 1) {
      const pendingFallback = pendingFallbacks[index]
      if (now - pendingFallback.createdAt <= LATE_EVENT_WINDOW_MS) continue
      if (!pendingFallback.injected) clearTimeout(pendingFallback.timeoutId)
      pendingFallbacks.splice(index, 1)
    }
    for (let index = deliveredInputs.length - 1; index >= 0; index -= 1) {
      if (now - deliveredInputs[index].deliveredAt <= LATE_EVENT_WINDOW_MS) continue
      deliveredInputs.splice(index, 1)
    }
  }

  // Event-tap Vietnamese IMEs (XKey, EVKey, OpenKey) edit the text themselves
  // instead of composing: they post real Backspace key events, then ONE keydown
  // whose `key` holds the entire replacement run. Typing "tieengs" ends with
  // Backspace x3 followed by `key="iếng"` (code=KeyA, keyCode=73, charCode=105).
  //
  // xterm has no path for that. evaluateKeyboardEvent only accepts
  // `key.length === 1`, so _keyDown bails at `if (!result.key) return true`
  // without sending or cancelling. _keyPress then runs and sends
  // String.fromCharCode(ev.charCode) — just the FIRST character, "i" — and sets
  // _keyPressHandled, which makes _inputEvent discard the full "iếng" at its
  // first line. The backspaces land, the replacement does not: "tiêng" becomes
  // "ti". Only letters carrying a second diacritic hit this, because those are
  // the ones whose tone mark lands on a vowel that is no longer last; "ê", "ô",
  // "đ" arrive as a single-character `key` that xterm handles on its own.
  //
  // Deliver the run here. preventDefault suppresses both the keypress that
  // would leak the first character and the input event that would double-send.
  const deliverKeyTextRewrite = (keyboardEvent: KeyboardEvent): boolean => {
    if (
      keyboardEvent.key.length < 2 ||
      NAMED_KEY.test(keyboardEvent.key) ||
      !PRINTABLE_TEXT.test(keyboardEvent.key) ||
      keyboardEvent.keyCode === 229 ||
      keyboardEvent.isComposing ||
      keyboardEvent.ctrlKey ||
      keyboardEvent.metaKey ||
      keyboardEvent.altKey ||
      isCompositionActive(helper)
    ) {
      return false
    }
    keyboardEvent.preventDefault()
    terminal.input(keyboardEvent.key)
    return true
  }

  const onKeyDown = (event: Event) => {
    const keyboardEvent = event as KeyboardEvent
    pruneLateEventRecords()

    latestKeydownWas229 = keyboardEvent.keyCode === 229
    forcedSinceKeydown = false
    const textareaValueBeforeKeydown = core.textarea?.value ?? ''
    if (latestKeydownWas229) {
      textareaValueBefore229Keydown = textareaValueBeforeKeydown
    }

    if (deliverKeyTextRewrite(keyboardEvent)) {
      return
    }

    if (
      keyboardEvent.keyCode !== 229 ||
      keyboardEvent.isComposing ||
      !keyboardEvent.getModifierState('CapsLock') ||
      !/^[A-Za-z]$/.test(keyboardEvent.key) ||
      keyboardEvent.ctrlKey ||
      keyboardEvent.metaKey ||
      keyboardEvent.altKey ||
      isCompositionActive(helper)
    ) {
      return
    }

    const character = keyboardEvent.shiftKey
      ? keyboardEvent.key
      : keyboardEvent.key.toLowerCase()
    const deliveredIndex = deliveredInputs.findIndex(deliveredInput =>
      isSameCharacter(deliveredInput.character, character),
    )

    // WKWebView can emit insertText before the corresponding 229 keydown. If
    // xterm already delivered that input, only cancel the late keydown.
    if (deliveredIndex >= 0) {
      deliveredInputs.splice(deliveredIndex, 1)
      keyboardEvent.preventDefault()
      return
    }

    keyboardEvent.preventDefault()
    const pendingFallback: PendingFallback = {
      character,
      textareaValueBeforeKeydown,
      createdAt: Date.now(),
      injected: false,
      timeoutId: setTimeout(() => {
        // xterm queues a textarea-diff timeout on the same keydown. Wait one
        // more turn so that diff can run before the fallback injects.
        pendingFallback.timeoutId = setTimeout(() => {
          removePendingFallback(pendingFallback)
          if (
            isCompositionActive(helper) ||
            (core.textarea && core.textarea.value !== textareaValueBeforeKeydown)
          ) {
            return
          }
          pendingFallback.injected = true
          pendingFallbacks.push(pendingFallback)
          terminal.input(character)
        }, 0)
      }, 0),
    }
    pendingFallbacks.push(pendingFallback)
  }

  core.textarea.addEventListener('keydown', onKeyDown, true)

  const originalInputEvent = core._inputEvent
  const patchedInputEvent = function patchedInputEvent(
    this: XtermCoreInternals,
    ev: InputEvent,
  ): boolean {
    pruneLateEventRecords()

    let matchingIndex = -1
    if (ev.data) {
      for (let index = pendingFallbacks.length - 1; index >= 0; index -= 1) {
        if (isSameCharacter(pendingFallbacks[index].character, ev.data)) {
          matchingIndex = index
          break
        }
      }
    }
    const matchingFallback = matchingIndex >= 0
      ? pendingFallbacks[matchingIndex]
      : null

    // A fallback already delivered this character; suppress the late input.
    if (matchingFallback?.injected) {
      if (this.textarea) {
        this.textarea.value = matchingFallback.textareaValueBeforeKeydown
      }
      removePendingFallback(matchingFallback)
      return true
    }

    if (ev.isComposing || isCompositionActive(helper)) {
      for (let index = pendingFallbacks.length - 1; index >= 0; index -= 1) {
        const pendingFallback = pendingFallbacks[index]
        if (pendingFallback.injected) continue
        clearTimeout(pendingFallback.timeoutId)
        pendingFallbacks.splice(index, 1)
      }
    }

    const was229 = latestKeydownWas229
    const shouldForceDirectPath =
      ev.inputType === 'insertText' &&
      Boolean(ev.data) &&
      ev.data!.length === 1 &&
      PRINTABLE_ASCII.test(ev.data!) &&
      !ev.isComposing &&
      !isCompositionActive(helper) &&
      this._keyDownSeen === true &&
      (was229 ? !forcedSinceKeydown : ev.composed)

    let result: boolean
    if (shouldForceDirectPath) {
      forcedSinceKeydown = true
      const savedKeyDownSeen = this._keyDownSeen
      this._keyDownSeen = false
      try {
        result = originalInputEvent!.call(this, ev)
      } finally {
        // Only the 229 path has a deferred diff to neutralise; rewinding on the
        // composed path would discard text xterm never queued.
        if (was229 && this.textarea) {
          this.textarea.value = textareaValueBefore229Keydown
        }
        this._keyDownSeen = savedKeyDownSeen
      }
    } else {
      result = originalInputEvent!.call(this, ev)
    }

    // If xterm delivered this input, remember it briefly. WKWebView may send
    // the matching 229 keydown afterward, and that keydown must not inject a
    // second copy of the character.
    if (
      result &&
      ev.inputType === 'insertText' &&
      ev.data &&
      PRINTABLE_ASCII.test(ev.data) &&
      !ev.isComposing &&
      !isCompositionActive(helper)
    ) {
      deliveredInputs.push({ character: ev.data, deliveredAt: Date.now() })
      if (matchingFallback && !matchingFallback.injected) {
        clearTimeout(matchingFallback.timeoutId)
        removePendingFallback(matchingFallback)
      }
    }

    return result
  }

  core._inputEvent = patchedInputEvent

  return {
    dispose() {
      for (const pendingFallback of pendingFallbacks) {
        if (!pendingFallback.injected) clearTimeout(pendingFallback.timeoutId)
      }
      pendingFallbacks.length = 0
      deliveredInputs.length = 0
      core.textarea?.removeEventListener('keydown', onKeyDown, true)
      if (core._inputEvent === patchedInputEvent) {
        core._inputEvent = originalInputEvent
      }
    },
  }
}
