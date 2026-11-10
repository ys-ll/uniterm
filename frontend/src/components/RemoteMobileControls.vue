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
    <div v-show="open" class="remote-mobile-key-bar">
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
    <button
      class="remote-mobile-kbd-btn"
      :class="{ open }"
      :title="t('remoteDesktop.softKeyboard')"
      :aria-label="t('remoteDesktop.softKeyboard')"
      @click.prevent="toggleKeyboard"
    >
      <Keyboard :size="16" />
    </button>
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
}>()

const open = defineModel<boolean>('open', { default: false })

const mobile = isMobilePlatform()
const inputEl = ref<HTMLInputElement | null>(null)
const composing = ref(false)

// ── Soft keyboard ──

// Android IMEs only report Backspace when the field actually has content to
// delete — an always-empty input swallows the key entirely. Keep a zero-width
// sentinel in the buffer so every delete produces a deleteContentBackward
// event, which we translate into a remote Backspace and then restore.
const INPUT_SENTINEL = '​'

function resetInputBuffer() {
  const el = inputEl.value
  if (el) el.value = INPUT_SENTINEL
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
type StickyCode = 'ControlLeft' | 'MetaLeft' | 'AltLeft' | 'ShiftLeft'
const STICKY_CODES: StickyCode[] = ['ControlLeft', 'MetaLeft', 'AltLeft', 'ShiftLeft']
const armed = reactive<Record<StickyCode, boolean>>({
  ControlLeft: false, MetaLeft: false, AltLeft: false, ShiftLeft: false,
})

interface BarKey {
  label: string
  code: string
  sticky?: boolean
}

const KEYS: BarKey[] = [
  { label: 'Ctrl', code: 'ControlLeft', sticky: true },
  { label: 'Win', code: 'MetaLeft', sticky: true },
  { label: 'Alt', code: 'AltLeft', sticky: true },
  { label: 'Shift', code: 'ShiftLeft', sticky: true },
  { label: 'Esc', code: 'Escape' },
  { label: 'Tab', code: 'Tab' },
  { label: 'Del', code: 'Delete' },
  { label: '←', code: 'ArrowLeft' },
  { label: '↑', code: 'ArrowUp' },
  { label: '↓', code: 'ArrowDown' },
  { label: '→', code: 'ArrowRight' },
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

function onKeydown(e: KeyboardEvent) {
  if (composing.value || e.key.length === 1 || e.key === 'Unidentified') return
  e.preventDefault()
  if (e.code in armed) {
    // A physical modifier key: forward it, don't touch the sticky state.
    heldSpecial.add(e.code)
    emit('special', e.code, true)
    return
  }
  heldSpecial.add(e.code)
  sendWithStickyModifiers(() => emit('special', e.code, true))
}

function onKeyup(e: KeyboardEvent) {
  if (heldSpecial.delete(e.code)) emit('special', e.code, false)
}

function onBeforeInput(e: InputEvent) {
  if (composing.value) return
  switch (e.inputType) {
    case 'insertText':
      if (e.data) sendWithStickyModifiers(() => emit('text', e.data!))
      break
    case 'insertCompositionText':
      // Covered by compositionend.
      break
    case 'insertLineBreak':
    case 'insertParagraph':
      e.preventDefault()
      sendWithStickyModifiers(() => emit('text', '\n'))
      break
    case 'deleteContentBackward':
    case 'deleteWordBackward':
      // Reachable when the IME deletes without a keydown (the common case —
      // keydown Backspace is preventDefault'ed above). The sentinel character
      // guarantees the IME always has something to delete here.
      sendWithStickyModifiers(() => {
        emit('special', 'Backspace', true)
        emit('special', 'Backspace', false)
      })
      break
  }
}

function onInput() {
  // Keep the buffer at exactly the sentinel so the next IME delete always
  // has content to remove and fires deleteContentBackward.
  const el = inputEl.value
  if (el && !composing.value) el.value = INPUT_SENTINEL
}

function onCompositionEnd(e: CompositionEvent) {
  composing.value = false
  if (e.data) sendWithStickyModifiers(() => emit('text', e.data!))
  resetInputBuffer()
}

function onImeBlur() {
  open.value = false
  for (const code of STICKY_CODES) armed[code] = false
}

// ── Touch gestures (long-press = right button, tap = left click) ──

const LONG_PRESS_MS = 500
const MOVE_SLOP_PX = 10

let timer: ReturnType<typeof setTimeout> | null = null
let startX = 0
let startY = 0
let curX = 0
let curY = 0
let startedAt = 0
let moved = false
let longpressed = false

function resetGesture() {
  if (timer !== null) {
    clearTimeout(timer)
    timer = null
  }
  longpressed = false
  moved = false
}

function onTouchStart(e: TouchEvent) {
  if (!props.touchEnabled || !props.active) return
  e.preventDefault()
  if (e.touches.length !== 1) {
    resetGesture()
    return
  }
  const touch = e.touches[0]
  startX = curX = touch.clientX
  startY = curY = touch.clientY
  startedAt = Date.now()
  moved = false
  longpressed = false
  if (timer !== null) clearTimeout(timer)
  timer = setTimeout(() => {
    timer = null
    longpressed = true
    emit('right', curX, curY, true)
  }, LONG_PRESS_MS)
}

function onTouchMove(e: TouchEvent) {
  if (!props.touchEnabled || !props.active) return
  e.preventDefault()
  const touch = e.touches[0]
  if (!touch) return
  curX = touch.clientX
  curY = touch.clientY
  if (!longpressed && !moved
      && Math.hypot(curX - startX, curY - startY) > MOVE_SLOP_PX) {
    moved = true
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }
  if (moved || longpressed) emit('move', curX, curY, longpressed)
}

function onTouchEnd(e: TouchEvent) {
  if (!props.touchEnabled || !props.active) return
  e.preventDefault()
  if (timer !== null) {
    clearTimeout(timer)
    timer = null
  }
  if (longpressed) {
    emit('right', curX, curY, false)
  } else if (!moved && Date.now() - startedAt < LONG_PRESS_MS) {
    emit('tap', curX, curY)
  }
  longpressed = false
  moved = false
}

function onTouchCancel() {
  resetGesture()
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
  bottom: 1.5rem;
  z-index: 19;
  background: var(--bg-elevated);
  border-top: 1px solid var(--border-default);
}
.remote-mobile-key-scroll {
  display: flex;
  gap: 0.375rem;
  overflow-x: auto;
  padding: 0.375rem 3rem 0.375rem 0.5rem;
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
.remote-mobile-key-btn.armed,
.remote-mobile-key-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.remote-mobile-kbd-btn {
  position: absolute;
  right: 0.75rem;
  bottom: 2.25rem;
  z-index: 20;
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 50%;
  border: 1px solid var(--border-default);
  background: var(--bg-elevated);
  color: var(--text-muted);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--shadow-md, 0 2px 8px rgba(0, 0, 0, 0.25));
  opacity: 0.85;
}
/* Sit above the key bar while it is shown. */
.remote-mobile-kbd-btn.open {
  bottom: 4.25rem;
}
.remote-mobile-kbd-btn.open,
.remote-mobile-kbd-btn:hover {
  opacity: 1;
  color: var(--text-primary);
  background: var(--bg-hover);
}
</style>
