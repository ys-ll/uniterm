<template>
  <MenuItem iconic class="submenu-wrap" :class="{ disabled }" :aria-disabled="disabled || undefined" @mouseenter="onEnter" @click="onTap">
    {{ label }}
    <el-icon class="menu-icon-trailing"><ChevronRight :size="'0.8125rem'" /></el-icon>
    <!-- Teleported to <body>: a fixed-positioned flyout can never be clipped by
         the parent menu (e.g. its overflow scrollbar container), on any platform. -->
    <Teleport to="body">
      <!-- Same as Menu.vue's root: swallow right-clicks instead of letting the
           native context menu pop over the flyout rows. -->
      <div
        v-show="submenu.active === name"
        ref="flyEl"
        class="menu-submenu"
        :style="flyStyle"
        @mouseleave="submenu.active = ''"
        @contextmenu.stop.prevent
      >
        <slot />
      </div>
    </Teleport>
  </MenuItem>
</template>

<script lang="ts">
// Module-scoped counter: `<script setup>` re-runs its body per component
// instance, so a counter declared there would always reset to 0. Keeping it in
// a plain `<script>` block gives every instance a distinct, stable key.
let submenuSeq = 0
</script>

<script setup lang="ts">
import { inject, ref, watch, nextTick } from 'vue'
import { ChevronRight } from '@lucide/vue'
import MenuItem from './MenuItem.vue'

// Hover-driven nested flyout row. The `active` state lives on the host <Menu>
// (single flyout open at a time); this row sets `active` on hover, and the
// flyout shows only when this instance's auto-generated key matches. Hides the
// .submenu-wrap / arrow / .menu-submenu / mouseenter-leave boilerplate that
// hosts used to hand-write.
const props = defineProps<{
  label: string
  disabled?: boolean
}>()

// Stable per instance; globally unique is safe because the single-open
// coordinator shows only one <Menu> at a time.
const name = `submenu-${submenuSeq++}`

// `submenu.active` reactive object provided by the enclosing <Menu>.
const submenu = inject<{ active: string }>('menuSubmenu')!

const rowEl = ref<HTMLElement | null>(null)
const flyEl = ref<HTMLElement | null>(null)
// Parked offscreen until the first position pass; never rendered at 0,0.
const flyStyle = ref<Record<string, string>>({ left: '-9999px', top: '-9999px' })

// Touch (coarse pointer) devices have no hover: tap toggles the flyout,
// otherwise the submenu — and everything inside it — is unreachable.
const isTouch =
  typeof window !== 'undefined' &&
  window.matchMedia('(pointer: coarse)').matches

function onTap(e: MouseEvent) {
  if (props.disabled) return
  rowEl.value = (e.currentTarget as HTMLElement) || rowEl.value
  // Hover already covers mouse users; tap toggles for touch.
  if (isTouch) submenu.active = submenu.active === name ? '' : name
}

// A touch tap synthesizes the compatibility mouse sequence, so mouseenter
// fires before click: opening here would make onTap's toggle immediately
// close the flyout. Guard it to real (non-touch) pointers.
function onEnter(e: MouseEvent) {
  if (props.disabled) return
  rowEl.value = (e.currentTarget as HTMLElement) || rowEl.value
  if (!isTouch) submenu.active = name
}

watch(() => props.disabled, disabled => { if (disabled && submenu.active === name) submenu.active = '' })

// Place the flyout beside its row against the window edges whenever it opens:
// fly RIGHT by default, mirror LEFT only when the right can't fit it but the
// left can. Vertically anchor to the row's top edge, flipping upward and
// clamping the height when the row sits low, so the flyout (and its scrollbar)
// always lands inside the viewport.
watch(() => submenu.active, async (key) => {
  if (key !== name) return
  // Reset first so the flyout measures at its natural size, unclamped by any
  // maxHeight left over from a previous open.
  flyStyle.value = { left: '-9999px', top: '-9999px' }
  await nextTick()
  const row = rowEl.value
  const fly = flyEl.value
  if (!row || !fly) return
  const rr = row.getBoundingClientRect()
  const fr = fly.getBoundingClientRect()
  const edge = 4
  // Row/flyout placement overlap keeps the mouse path across the gap gap-free.
  const overlap = 3
  const fitsRight = rr.right + fr.width <= window.innerWidth - edge
  const fitsLeft = rr.left - fr.width >= edge
  let left = !fitsRight && fitsLeft ? rr.left - fr.width + overlap : rr.right - overlap
  left = Math.max(edge, Math.min(left, window.innerWidth - fr.width - edge))
  const below = window.innerHeight - rr.top - 8
  const above = rr.bottom - 8
  const cap = (room: number) => Math.max(192, Math.min(window.innerHeight - 8, room))
  let top = rr.top - 4
  let maxHeight = cap(below)
  if (fr.height > below && above > below) {
    maxHeight = cap(above)
    top = rr.bottom + 4 - Math.min(fr.height, maxHeight)
  }
  flyStyle.value = {
    left: left + 'px',
    top: Math.max(edge, top) + 'px',
    maxHeight: maxHeight + 'px',
  }
})
</script>
