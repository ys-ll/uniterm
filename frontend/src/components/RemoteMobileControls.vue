<template>
  <!-- Mobile-only input bridge for remote desktop sessions (issue #1425):
       a floating button that summons the Android soft keyboard through a
       hidden input, a sticky modifier / special-key bar shown while the
       keyboard is open, and (for canvases without their own touch handling)
       long-press → right-click / tap → left-click gesture translation. -->
  <template v-if="active && mobile">
    <input
      ref="inputEl"
      class="remote-mobile-ime-input"
      type="text"
      autocomplete="off"
      autocapitalize="none"
      autocorrect="off"
      inputmode="email"
      enterkeyhint="send"
      spellcheck="false"
      :aria-hidden="true"
      tabindex="-1"
      @keydown="onKeydown"
      @keyup="onKeyup"
      @beforeinput="onBeforeInput"
      @input="onInput"
      @focus="resetInputBuffer"
      @compositionstart="composing = true"
      @compositionend="onCompositionEnd"
      @blur="onImeBlur"
    />
    <div class="remote-mobile-key-bar">
      <button
        class="remote-mobile-key-btn remote-mobile-kbd-toggle"
        :class="{ armed: open }"
        :title="t('remoteDesktop.softKeyboard')"
        :aria-label="t('remoteDesktop.softKeyboard')"
        @pointerdown.prevent="toggleKeyboard"
      >
        <Keyboard :size="14" />
      </button>
      <div class="remote-mobile-key-scroll">
        <button
          v-for="key in KEYS"
          :key="key.code"
          class="remote-mobile-key-btn"
          :class="{ armed: key.sticky && isArmed(key.code) }"
          @pointerdown.prevent="onBarKey(key)"
        >{{ key.label }}</button>
      </div>
    </div>
  </template>
</template>

<script setup lang="ts">
import { ref, reactive, watch, onBeforeUnmount } from 'vue'
import { Keyboard } from '@lucide/vue'
import { useI18n } from '../i18n'
import { isMobilePlatform } from '../utils/platform'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  /** Session live — controls whether the controls render at all */
  active: boolean
  /** Element receiving touch gestures (the remote-desktop viewport) */
  target: HTMLElement | null
  /** false when the renderer already handles touch gestures (noVNC) */
  touchEnabled?: boolean
}>(), {
  touchEnabled: true,
})

const emit = defineEmits<{
  /** Soft keyboard committed this text (typed chars, Enter as '\n') */
  (e: 'text', chars: string): void
  /** Special key from keydown/keyup, identified by KeyboardEvent.code */
  (e: 'special', code: string, down: boolean): void
  (e: 'tap', x: number, y: number): void
  (e: 'right', x: number, y: number, down: boolean): void
  (e: 'move', x: number, y: number, buttonDown: boolean): void
  /** Button-press drag lifecycle (left drag after a long-press) */
  (e: 'dragStart', x: number, y: number, button: number): void
  (e: 'dragMove', x: number, y: number, button: number): void
  (e: 'dragEnd', x: number, y: number, button: number): void
  /** Two-finger wheel scroll, deltaY in display pixels per tick */
  (e: 'wheel', x: number, y: number, deltaY: number): void
}>()

const open = defineModel<boolean>('open', { default: false })

const mobile = isMobilePlatform()
const inputEl = ref<HTMLInputElement | null>(null)
const composing = ref(false)

// ── Soft keyboard ──

// Android IMEs only report Backspace for content they can see in the field:
// an always-empty buffer swallows the key, and worse, IMEs that "undo" their
// last commit (common with Chinese keyboards) skip the field entirely unless
// the committed text is still there. So keep the committed text in the
// buffer — only the leading zero-width sentinel is guaranteed — and trim
// once it grows past a bound.
const INPUT_SENTINEL = '​'
const INPUT_LIMIT = 64

function resetInputBuffer() {
  const el = inputEl.value
  if (el) el.value = INPUT_SENTINEL
}

function trimInputBuffer() {
  const el = inputEl.value
  if (el && (el.value.length === 0 || el.value.length > INPUT_LIMIT)) {
    el.value = INPUT_SENTINEL
  }
}

function toggleKeyboard() {
  const el = inputEl.value
  if (!el) return
  if (open.value) {
    el.blur()
    open.value = false
  } else {
    // Focus from a user gesture is what makes the Android IME appear.
    resetInputBuffer()
    el.focus({ preventScroll: true })
    open.value = true
  }
}

// Sticky modifiers: armed by the key bar, wrapped around the next emitted
// key/text, consumed after one use (same model as the terminal MobileKeyBar).
type StickyCode = 'ControlLeft' | 'AltLeft' | 'ShiftLeft'
const STICKY_CODES: StickyCode[] = ['ControlLeft', 'AltLeft', 'ShiftLeft']
const armed = reactive<Record<StickyCode, boolean>>({
  ControlLeft: false, AltLeft: false, ShiftLeft: false,
})

interface BarKey {
  label: string
  code: string
  sticky?: boolean
}

const KEYS: BarKey[] = [
  { label: 'Ctrl', code: 'ControlLeft', sticky: true },
  { label: 'Alt', code: 'AltLeft', sticky: true },
  { label: 'Shift', code: 'ShiftLeft', sticky: true },
  { label: 'Esc', code: 'Escape' },
  { label: 'Tab', code: 'Tab' },
  { label: '⌫', code: 'Backspace' },
  { label: '⏎', code: 'Enter' },
  { label: '←', code: 'ArrowLeft' },
  { label: '↑', code: 'ArrowUp' },
  { label: '↓', code: 'ArrowDown' },
  { label: '→', code: 'ArrowRight' },
  { label: 'Del', code: 'Delete' },
]

function isArmed(code: string): boolean {
  return STICKY_CODES.includes(code as StickyCode) && armed[code as StickyCode]
}

// Wrap a payload emission with the armed sticky modifiers (down before,
// up after) and consume them.
function sendWithStickyModifiers(payload: () => void) {
  for (const code of STICKY_CODES) if (armed[code]) emit('special', code, true)
  payload()
  for (const code of STICKY_CODES) if (armed[code]) emit('special', code, false)
  for (const code of STICKY_CODES) armed[code] = false
}

function onBarKey(key: BarKey) {
  if (key.sticky) {
    const code = key.code as StickyCode
    armed[code] = !armed[code]
    return
  }
  sendWithStickyModifiers(() => {
    emit('special', key.code, true)
    emit('special', key.code, false)
  })
}

// Special keys travel through keydown/keyup (Enter, Backspace, arrows, …).
// Printable characters are delivered by the IME as input events; keydown for
// them is left alone so we never double-send.
const heldSpecial = new Set<string>()
// Some IMEs deliver Backspace/Enter as BOTH a keydown (which we
// preventDefault, yet the IME may still perform the edit) and a
// beforeinput — dedupe the two sources within a small window.
let lastBackspaceAt = 0
let lastEnterAt = 0

// IME-generated keys often never produce a matching keyup (and Backspace /
// Enter sent held-down would wedge the remote), so they go out as
// transient press+release pairs.
function isTransientKey(e: KeyboardEvent): boolean {
  return e.keyCode === 8 || e.keyCode === 13
}

function transientCode(e: KeyboardEvent): string {
  if (e.key !== 'Unidentified') return e.code
  return e.keyCode === 8 ? 'Backspace' : 'Enter'
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Unidentified' && e.keyCode !== 8 && e.keyCode !== 13) return
  if (composing.value && !isTransientKey(e)) return
  if (e.key.length === 1) return
  e.preventDefault()
  if (e.keyCode === 8) lastBackspaceAt = Date.now()
  if (e.keyCode === 13) lastEnterAt = Date.now()
  const code = isTransientKey(e) ? transientCode(e) : e.code
  if (isTransientKey(e)) {
    sendWithStickyModifiers(() => {
      emit('special', code, true)
      emit('special', code, false)
    })
    return
  }
  if (code in armed) {
    // A physical modifier key: forward it, don't touch the sticky state.
    heldSpecial.add(code)
    emit('special', code, true)
    return
  }
  heldSpecial.add(code)
  sendWithStickyModifiers(() => emit('special', code, true))
}

function onKeyup(e: KeyboardEvent) {
  if (heldSpecial.delete(e.code)) emit('special', e.code, false)
}

function onBeforeInput(e: InputEvent) {
  switch (e.inputType) {
    case 'insertText':
      if (composing.value) return
      if (e.data) sendWithStickyModifiers(() => emit('text', e.data!))
      break
    case 'insertCompositionText':
      // Covered by compositionend.
      break
    case 'insertLineBreak':
    case 'insertParagraph':
      e.preventDefault()
      // Skipped when a keydown Enter just handled the same press.
      if (Date.now() - lastEnterAt > 120) {
        sendWithStickyModifiers(() => emit('text', '\n'))
      }
      break
    case 'deleteContentBackward':
    case 'deleteWordBackward':
      // Forward every deletion — even one happening mid-composition: the
      // remote has no notion of the local IME's composition, so any delete
      // must reach it as Backspace. Skipped when a Backspace keydown just
      // handled the same press.
      if (Date.now() - lastBackspaceAt > 120) {
        sendWithStickyModifiers(() => {
          emit('special', 'Backspace', true)
          emit('special', 'Backspace', false)
        })
      }
      break
  }
}

function onInput() {
  // Keep committed text in the buffer (IMEs need it for Backspace); only
  // trim when it grows past the bound or got emptied.
  if (!composing.value) trimInputBuffer()
}

function onCompositionEnd(e: CompositionEvent) {
  composing.value = false
  if (e.data) sendWithStickyModifiers(() => emit('text', e.data!))
  trimInputBuffer()
}

function onImeBlur() {
  open.value = false
  for (const code of STICKY_CODES) armed[code] = false
}

// ── Touch gestures ──
//
// Long-press is only a *preparation*: the decision happens when the finger
// moves or lifts. Sending the right-button press at the timer would force
// every later drag to be a right-button drag — and undoing it (press right,
// then release) pops the remote context menu mid-gesture.
//
//   press → move before long-press:
//     viewport overflows → pan the view (restore native touch scrolling)
//     viewport fits      → hover-move the remote pointer
//   press → 500 ms held    → armed, nothing sent yet
//     ├─ lift             → right click (press+release at lift)
//     └─ move             → left-button drag (press at move start)
//   two fingers            → wheel scroll of the remote (accumulated)
//
const LONG_PRESS_MS = 500
const MOVE_SLOP_PX = 10
// The remote's own double-click rect is tiny; consecutive touchscreen taps
// differ by a few display pixels, which a scaled-down remote desktop
// magnifies far past it. Snap the second tap of a pair onto the first one's
// exact position so the remote sees two clicks at the same coordinates.
const DOUBLE_TAP_MS = 500
const DOUBLE_TAP_SLOP_PX = 15
// Two-finger movement is accumulated and emitted as discrete wheel ticks.
const WHEEL_TICK_PX = 48

type GestureMode = 'idle' | 'single' | 'wheel' | 'dead'

let mode: GestureMode = 'idle'
let timer: ReturnType<typeof setTimeout> | null = null
let startX = 0
let startY = 0
let curX = 0
let curY = 0
let startedAt = 0
let moved = false
// Long-press confirmed, decision pending (right click on lift / left drag on move)
let longpressed = false
// Left-button drag in progress (started from a confirmed long-press)
let dragButton: number | null = null
// Panning the overflowing viewport instead of moving the remote pointer
let panning = false
let panLastX = 0
let panLastY = 0
let wheelLastY = 0
let wheelAccum = 0
let lastTapX = 0
let lastTapY = 0
let lastTapAt = 0

function isScrollable(): boolean {
  const el = props.target
  if (!el) return false
  return el.scrollWidth > el.clientWidth + 1 || el.scrollHeight > el.clientHeight + 1
}

function resetGesture() {
  if (timer !== null) {
    clearTimeout(timer)
    timer = null
  }
  longpressed = false
  moved = false
  panning = false
  dragButton = null
}

function dismissKeyboard() {
  if (!open.value) return
  const el = inputEl.value
  if (el) el.blur()
  open.value = false
  for (const code of STICKY_CODES) armed[code] = false
}

function onTouchStart(e: TouchEvent) {
  if (!props.touchEnabled || !props.active) return
  e.preventDefault()
  // Touching the desktop is a mouse interaction: dismiss the soft keyboard.
  // With the hidden input still focused, Android re-summons the IME on any
  // tap, which both covers the screen and cancels in-flight tap streams.
  dismissKeyboard()
  if (e.touches.length >= 2) {
    // Two-finger wheel mode (or re-anchor it when a third finger lands).
    resetGesture()
    mode = 'wheel'
    wheelLastY = (e.touches[0].clientY + e.touches[1].clientY) / 2
    wheelAccum = 0
    return
  }
  if (mode !== 'idle') {
    // A finger landed while winding down a previous gesture — ignore it
    // until every finger is up.
    mode = 'dead'
    return
  }
  mode = 'single'
  const touch = e.touches[0]
  startX = curX = touch.clientX
  startY = curY = touch.clientY
  startedAt = Date.now()
  moved = false
  longpressed = false
  dragButton = null
  panning = false
  if (timer !== null) clearTimeout(timer)
  timer = setTimeout(() => {
    timer = null
    longpressed = true
  }, LONG_PRESS_MS)
}

function onTouchMove(e: TouchEvent) {
  if (!props.touchEnabled || !props.active) return
  e.preventDefault()
  if (mode === 'wheel') {
    if (e.touches.length >= 2) {
      const midY = (e.touches[0].clientY + e.touches[1].clientY) / 2
      const midX = (e.touches[0].clientX + e.touches[1].clientX) / 2
      wheelAccum += midY - wheelLastY
      wheelLastY = midY
      // Emit discrete ticks so per-tick protocols (SPICE consumes one
      // wheel event = one notch) and continuous ones (RDP pixel deltas)
      // both behave.
      while (Math.abs(wheelAccum) >= WHEEL_TICK_PX) {
        const step = wheelAccum > 0 ? WHEEL_TICK_PX : -WHEEL_TICK_PX
        wheelAccum -= step
        emit('wheel', midX, midY, step)
      }
    }
    return
  }
  if (mode !== 'single') return
  const touch = e.touches[0]
  if (!touch) return
  curX = touch.clientX
  curY = touch.clientY
  if (dragButton !== null) {
    emit('dragMove', curX, curY, dragButton)
    return
  }
  if (!longpressed && !moved
      && Math.hypot(curX - startX, curY - startY) > MOVE_SLOP_PX) {
    moved = true
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
    if (isScrollable()) {
      // The desktop is larger than the pane: a plain drag pans the view.
      panning = true
      panLastX = curX
      panLastY = curY
      return
    }
  }
  if (longpressed) {
    // Confirmed long-press now turns into a left-button drag.
    dragButton = 0
    sendWithStickyModifiers(() => emit('dragStart', curX, curY, 0))
    return
  }
  if (moved) {
    if (panning) {
      const el = props.target
      if (el) {
        el.scrollLeft -= curX - panLastX
        el.scrollTop -= curY - panLastY
      }
      panLastX = curX
      panLastY = curY
    } else {
      emit('move', curX, curY, false)
    }
  }
}

function onTouchEnd(e: TouchEvent) {
  if (!props.touchEnabled || !props.active) return
  e.preventDefault()
  if (mode === 'wheel') {
    // Keep wheeling while at least two fingers stay down; one finger left
    // over does nothing until all are lifted.
    if (e.touches.length < 2) mode = 'dead'
    else wheelLastY = (e.touches[0].clientY + e.touches[1].clientY) / 2
    return
  }
  if (timer !== null) {
    clearTimeout(timer)
    timer = null
  }
  if (mode === 'single') {
    if (dragButton !== null) {
      emit('dragEnd', curX, curY, dragButton)
    } else if (longpressed && !moved) {
      // Lifted after a long-press without dragging: right click.
      sendWithStickyModifiers(() => {
        emit('right', curX, curY, true)
        emit('right', curX, curY, false)
      })
    } else if (!moved && Date.now() - startedAt < LONG_PRESS_MS) {
      let tx = curX
      let ty = curY
      const now = Date.now()
      if (now - lastTapAt <= DOUBLE_TAP_MS
          && Math.hypot(tx - lastTapX, ty - lastTapY) <= DOUBLE_TAP_SLOP_PX) {
        tx = lastTapX
        ty = lastTapY
      }
      lastTapX = tx
      lastTapY = ty
      lastTapAt = now
      // Taps honor armed sticky modifiers too: Ctrl+click multi-select,
      // Shift+click range select.
      sendWithStickyModifiers(() => emit('tap', tx, ty))
    }
  }
  resetGesture()
  // Remaining fingers (e.g. one lifted out of a two-finger wheel) are
  // ignored until all are up.
  if (e.touches.length > 0) mode = 'dead'
  else mode = 'idle'
}

function onTouchCancel() {
  // A cancelled drag must still release the remote button, or it sticks.
  if (dragButton !== null) {
    emit('dragEnd', curX, curY, dragButton)
  }
  resetGesture()
  mode = 'idle'
}

// Listeners are non-passive: preventDefault suppresses the browser's
// synthetic mouse events, double-tap zoom, long-press context menu and
// text-selection callout, so the gesture layer fully owns the touch stream.
function bindTouch(el: HTMLElement, on: boolean) {
  const opts: AddEventListenerOptions = { passive: false }
  const methods: [string, EventListener][] = [
    ['touchstart', onTouchStart as EventListener],
    ['touchmove', onTouchMove as EventListener],
    ['touchend', onTouchEnd as EventListener],
    ['touchcancel', onTouchCancel as EventListener],
  ]
  for (const [name, fn] of methods) {
    el.removeEventListener(name, fn, opts)
    if (on) el.addEventListener(name, fn, opts)
  }
}

watch(() => props.target, (el, prev) => {
  if (prev) bindTouch(prev, false)
  if (el) bindTouch(el, true)
}, { immediate: true })

onBeforeUnmount(() => {
  if (props.target) bindTouch(props.target, false)
  if (timer !== null) clearTimeout(timer)
})
</script>

<style scoped>
.remote-mobile-ime-input {
  position: absolute;
  top: 0;
  left: 0;
  width: 1px;
  height: 1px;
  opacity: 0;
  border: none;
  padding: 0;
  margin: 0;
  background: transparent;
  pointer-events: none;
}
.remote-mobile-key-bar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 19;
  height: 2.75rem;
  background: var(--bg-elevated);
  border-top: 1px solid var(--border-default);
}
.remote-mobile-key-scroll {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  overflow-x: auto;
  height: 100%;
  box-sizing: border-box;
  padding: 0.375rem 0.5rem 0.375rem 3rem;
}
/* The keyboard toggle stays pinned on the left, outside the scroll list. */
.remote-mobile-kbd-toggle {
  position: absolute;
  left: 0.375rem;
  top: 0.375rem;
  bottom: 0.375rem;
  width: 2.25rem;
  flex: none;
}
.remote-mobile-key-btn {
  flex: none;
  min-width: 2.25rem;
  height: 2rem;
  padding: 0 0.5rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-default);
  background: var(--bg-primary);
  color: var(--text-secondary);
  font-size: 0.8125rem;
}
.remote-mobile-key-btn.armed {
  background: var(--bg-hover);
  color: var(--text-primary);
}
/* Hover is sticky on touch screens and would make keys look stuck down. */
@media (hover: hover) {
  .remote-mobile-key-btn:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }
}
.remote-mobile-kbd-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
