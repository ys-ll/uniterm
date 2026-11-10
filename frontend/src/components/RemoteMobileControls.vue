<template>
  <!-- Mobile-only input bridge for remote desktop sessions (issue #1425):
       a floating button that summons the Android soft keyboard through a
       hidden input, and (for canvases without their own touch handling)
       long-press → right-click / tap → left-click gesture translation. -->
  <template v-if="active && mobile">
    <input
      ref="inputEl"
      class="remote-mobile-ime-input"
      type="text"
      autocomplete="off"
      autocapitalize="none"
      autocorrect="off"
      spellcheck="false"
      :aria-hidden="true"
      tabindex="-1"
      @keydown="onKeydown"
      @keyup="onKeyup"
      @beforeinput="onBeforeInput"
      @input="onInput"
      @compositionstart="composing = true"
      @compositionend="onCompositionEnd"
      @blur="open = false"
    />
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
import { ref, watch, onBeforeUnmount } from 'vue'
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

function toggleKeyboard() {
  const el = inputEl.value
  if (!el) return
  if (open.value) {
    el.blur()
    open.value = false
  } else {
    // Focus from a user gesture is what makes the Android IME appear.
    el.focus({ preventScroll: true })
    open.value = true
  }
}

// Special keys travel through keydown/keyup (Enter, Backspace, arrows, …).
// Printable characters are delivered by the IME as input events; keydown for
// them is left alone so we never double-send.
const heldSpecial = new Set<string>()

function onKeydown(e: KeyboardEvent) {
  if (composing.value || e.key.length === 1 || e.key === 'Unidentified') return
  e.preventDefault()
  heldSpecial.add(e.code)
  emit('special', e.code, true)
}

function onKeyup(e: KeyboardEvent) {
  if (heldSpecial.delete(e.code)) emit('special', e.code, false)
}

function onBeforeInput(e: InputEvent) {
  if (composing.value) return
  switch (e.inputType) {
    case 'insertText':
      if (e.data) emit('text', e.data)
      break
    case 'insertLineBreak':
    case 'insertParagraph':
      e.preventDefault()
      emit('text', '\n')
      break
    case 'deleteContentBackward':
      // Reachable when the IME deletes without a keydown (e.g. inside a
      // composition); the normal path already handled it after keydown.
      if (!heldSpecial.has('Backspace')) {
        emit('special', 'Backspace', true)
        emit('special', 'Backspace', false)
      }
      break
  }
}

function onInput() {
  // Keep the buffer empty so every IME commit arrives as fresh data.
  const el = inputEl.value
  if (el && !composing.value) el.value = ''
}

function onCompositionEnd(e: CompositionEvent) {
  composing.value = false
  if (e.data) emit('text', e.data)
  const el = inputEl.value
  if (el) el.value = ''
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
// synthetic mouse events, long-press context menu and text-selection
// callout, so the gesture layer fully owns the single-touch stream.
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
.remote-mobile-kbd-btn.open,
.remote-mobile-kbd-btn:hover {
  opacity: 1;
  color: var(--text-primary);
  background: var(--bg-hover);
}
</style>
