<template>
  <Teleport to="body">
    <div
      v-show="visible"
      ref="menuEl"
      class="conn-context-menu"
      :class="[{ overflow: overflow }, rootClass]"
      :style="menuStyle"
      @mouseover="onInnerMouseOver"
      @mouseleave="onRootLeave"
      @scroll="submenu.active = ''"
      @contextmenu.stop
    >
      <slot :current="current" />
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { releaseRemoteKeyboardFocus } from '../utils/remoteKeyboard'
import { ref, reactive, provide, watch, nextTick } from 'vue'

// Single unified menu. Covers every showed-panel style of menu the app uses:
//   - flat button trigger    → toggle(anchorEl, data) / open(anchorEl, data)
//   - right-click / pointer  → openAt(x, y, data)
//   - nested submenus        → render .submenu-wrap rows in the slot; the active
//                               flyout key is the `submenu` slot prop ({ active }).
// The .conn-context-menu skin lives once in style.css, reused by everything.

// ── Global single-open coordinator (module scope, shared across ALL <Menu>s) ──
// Only the most recently opened menu stays open: opening one closes whatever was
// open before, and ONE shared document listener dismisses the active menu on
// outside-click / Escape. Defined here once so hosts never reimplement it.
// Note: per/future migration this also broadcasts so non-Menu popovers close.
interface ActiveMenu { el: () => HTMLElement | null; close: () => void }
let active: ActiveMenu | null = null
let docBound = false
// Pause for a host modal (e.g. an ElMessageBox prompt) opened on top of the
// active menu: while paused, outside-click / Escape must not dismiss the menu,
// so row actions (rename / delete) can be repeated without reopening. The
// dialog behaves like a child of the menu for dismissal purposes.
let docClosePaused = false
// Generation counter: every pause gets a token; resuming with a stale token
// (a newer pause took over) is a no-op, so a lost finally() can never leave
// an orphaned resume able to clobber the current pause.
let pauseGen = 0

// Close the active menu whenever any surface broadcasts the legacy
// global:close-context-menus event (opening a dialog,
// keyboard nav, etc.). Centralized here so no host re-subscribes to force-close
// menus — single-open + this event is the whole close story.
window.addEventListener('global:close-context-menus', () => { active?.close() })

function activate(m: ActiveMenu) {
  releaseRemoteKeyboardFocus()
  // Right-click menus close each other because main.ts dispatches the global
  // close signal on every `contextmenu` (capture) before the new menu opens.
  // Broadcast the same signal here when taking over a *different* menu so a
  // button-triggered open closes everything too — this is the close-other path
  // that's actually proven to fire. Dispatch BEFORE switching `active` so the
  // signal closes the previous menu, not this one.
  //
  if (active === m) {
    // The visible-prop watcher can activate an already opened menu.
    if (!docBound) {
      document.addEventListener('click', onDocClick)
      document.addEventListener('keydown', onDocKeydown)
      docBound = true
    }
    return
  }
  window.dispatchEvent(new Event('global:close-context-menus'))
  active?.close()
  // A different menu is taking over: any modal-pause belonging to the previous
  // interaction is dead (its dialog goes away with the menu it guarded), so
  // drop the flag here — before the host of the new menu can call pauseDocClose
  // (which happens synchronously in its click handler, i.e. after this line).
  docClosePaused = false
  active = m
  if (!docBound) {
    document.addEventListener('click', onDocClick)
    document.addEventListener('keydown', onDocKeydown)
    docBound = true
  }
}
function deactivate(m: ActiveMenu) {
  if (active === m) active = null
}

// Bubble-phase outside-click close. Trigger buttons use @click.stop so their own
// click only runs toggle(); anything that bubbles past the open menu closes it.
// Clicks inside a teleported flyout (see MenuSubmenu) live outside the menu root
// in the DOM but must count as inside.
function onDocClick(e: MouseEvent) {
  if (!active || docClosePaused) return
  const t = e.target as HTMLElement
  const el = active.el()
  if (el && !el.contains(t) && !t.closest('.menu-submenu')) active.close()
}
function onDocKeydown(e: KeyboardEvent) {
  if (docClosePaused) return
  if (e.key === 'Escape') active?.close()
}

// Hosts call this (via the exposed wrapper) right before opening a modal that
// floats above the open menu. The pause must be synchronous because the click
// that triggered the host handler is still propagating to onDocClick; the
// returned resume() (call it when the dialog closes) lets outside-click/Escape
// dismissal take effect again. A newer pause supersedes an older one whose
// resume was never called.
function pauseDocClose(): () => void {
  docClosePaused = true
  const token = ++pauseGen
  return () => {
    if (token === pauseGen) docClosePaused = false
  }
}

const props = defineProps<{
  visible: boolean
  align?: 'start' | 'end'
  /** Extra class(es) merged onto the teleported root; lets hosts keep
   *  per-menu skin modifiers. */
  rootClass?: string
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
}>()

const menuEl = ref<HTMLElement | null>(null)
const menuStyle = ref({ top: '-9999px', left: '-9999px' })
// True when the menu is taller than the viewport: scroll inside the container
// instead of letting the window clip the bottom. Only set in that state —
// overflow != visible would clip content even when nothing actually overflows.
// Submenu flyouts are teleported to <body> (see MenuSubmenu), so this does NOT
// clip them.
const overflow = ref(false)
const current = ref<unknown>()
// Writable submenu state shared with descendant <MenuSubmenu> rows via provide;
// MenuSubmenu sets `active` on hover and reads it to show/hide its flyout.
const submenu = reactive({ active: '' as string })
provide('menuSubmenu', submenu)

const closeFn = () => emit('update:visible', false)
const menuController: ActiveMenu = { el: () => menuEl.value, close: closeFn }

async function position(x: number, y: number) {
  const m = menuEl.value
  if (!m) return
  const mr = m.getBoundingClientRect()
  overflow.value = mr.height > window.innerHeight - 8
  menuStyle.value = {
    left: Math.max(4, Math.min(x, window.innerWidth - mr.width - 4)) + 'px',
    top: Math.max(4, Math.min(y, window.innerHeight - mr.height - 4)) + 'px',
  }
}

// Mouse leaving the menu root collapses the open flyout — unless the pointer
// moved into a teleported flyout (a sibling of the menu root in the DOM, so
// leaving the root to reach it must not count as leaving the menu).
function onRootLeave(e: MouseEvent) {
  const rt = e.relatedTarget as HTMLElement | null
  if (rt && rt.closest('.menu-submenu')) return
  submenu.active = ''
}

// Flat menu anchored below a trigger button.
function open(anchorEl: HTMLElement, data?: unknown) {
  current.value = data
  emit('update:visible', true)
  // Close any other open menu synchronously at open-time (not deferred to the
  // async visible-prop watcher) so a button-triggered open behaves exactly like
  // a right-click one and always drops the previously-open menu.
  activate(menuController)
  nextTick(() => {
    const m = menuEl.value
    if (!anchorEl || !m) return
    const br = anchorEl.getBoundingClientRect()
    const mr = m.getBoundingClientRect()
    const gap = 4
    // Open below the trigger: `start` left-aligns the menu to the button's left
    // edge, `end` right-aligns it to the button's right edge (x = right - width).
    // Using the button's right as the menu's LEFT edge (the old code) shifted the
    // whole menu off to the button's corner instead of directly under it.
    const x = props.align === 'end' ? br.right - mr.width : br.left
    // Open downward below the trigger; flip upward when it would run past the
    // bottom edge of the window so the menu never gets clipped at the edge.
    let y = br.bottom + gap
    if (y + mr.height > window.innerHeight - gap) {
      y = Math.max(gap, br.top - mr.height - gap)
    }
    position(x, y)
  })
}

// Right-click menu positioned at a pointer/X-Y coordinate.
function openAt(x: number, y: number, data?: unknown) {
  current.value = data
  emit('update:visible', true)
  // See open(): close others synchronously instead of waiting on the watcher.
  activate(menuController)
  nextTick(() => position(x, y))
}

function toggle(anchorEl: HTMLElement, data?: unknown) {
  if (props.visible) {
    emit('update:visible', false)
  } else {
    open(anchorEl, data)
  }
}

defineExpose({ open, openAt, toggle, close: closeFn, pauseDocClose })

// Hovering anywhere outside a submenu-wrap or its open flyout collapses it.
function onInnerMouseOver(e: MouseEvent) {
  const t = e.target as HTMLElement
  if (!t.closest('.submenu-wrap') && !t.closest('.menu-submenu')) {
    submenu.active = ''
  }
}

watch(() => props.visible, (v) => {
  if (v) {
    activate(menuController)
  } else {
    deactivate(menuController)
    current.value = undefined
    submenu.active = ''
  }
})
</script>

<!-- Non-scoped so the rules still reach the building-block rows
     (.menu-item / .menu-divider / .menu-submenu) rendered by child components
     (MenuItem / MenuDivider / MenuSubmenu) nested inside the menu's slot. -->
<style>
/* ── Unified context-menu styling (single source of truth) ─────────────
   Every right-click / popover / dropdown menu in the app should reuse these
   classes instead of defining its own look. All containers share the same
   surface skin (background, border, radius, shadow, padding, frosted-glass
   blur) and the same row styling (.menu-item / .menu-divider), so font size,
   text colors, hover, danger, active and radius stay consistent everywhere.

   Which container class to use:
   - .conn-context-menu          → a top-level menu (this component's root),
     teleported to <body> and positioned with screen coordinates (fixed).
   - .menu-submenu               → a nested flyout / submenu. It reuses the
     same surface and row styling as the parent; MenuSubmenu teleports it to
     <body> and fixed-positions it beside its row against the window edges.

   Modifiers / building blocks (all defined once here):
   - .conn-context-menu.anchored → absolute, anchored to a relatively
     positioned parent (e.g. an inline "⋯" button) instead of fixed.
   - .menu-item.active           → selected / highlighted row (accent).
   - .menu-item.iconic           → row with a leading icon; keeps icon+label
                                   and a trailing slot aligned on one line.
   - .menu-item.submenu-wrap     → row that owns a nested .menu-submenu
                                   flyout (see MenuSubmenu.vue).
   - .menu-item.mono             → monospaced path / label row (SFTP drives,
                                   bookmarks).
   - .menu-divider               → separator line between groups.
     (.menu-shortcut / trailing overlay live scoped inside MenuItem.vue.) */
.conn-context-menu {
  position: fixed;
  z-index: 99999;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-md);
  /* Auto-width: the menu hugs its widest row, so no per-menu min-width tuning
     is needed. min-width only guarantees a floor for single-word menus. */
  width: max-content;
  min-width: 8.75rem;
  padding: 0.25rem;
  backdrop-filter: blur(0.5rem);
}
.conn-context-menu.anchored {
  position: absolute;
  top: 100%;
  right: 0;
}
/* Too tall for the window: scroll inside the container instead of being cut
   off by the window edge. Toggled only when the measured height exceeds the
   viewport (see position()), because overflow != visible would clip the
   absolutely positioned submenu flyouts even when nothing overflows. */
.conn-context-menu.overflow {
  max-height: calc(100vh - 0.5rem);
  overflow-y: auto;
  overflow-x: hidden;
}
.conn-context-menu .menu-item {
  padding: 0.4375rem 0.875rem;
  font-size: 0.75rem;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  user-select: none;
  white-space: nowrap;
  border-radius: var(--radius-sm);
  transition: all 0.1s ease;
}
.conn-context-menu .menu-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.conn-context-menu .menu-item.disabled {
  color: var(--text-disabled);
  pointer-events: none;
}
.conn-context-menu .menu-item.danger {
  color: var(--error);
}
.conn-context-menu .menu-item.danger:hover {
  background: var(--error-subtle);
  color: var(--error);
}
/* Selected / highlighted row — accent + semibold (same look as the settings
   submenus). */
.conn-context-menu .menu-item.active {
  color: var(--accent);
  font-weight: 500;
}
.conn-context-menu .menu-item.iconic {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}
.conn-context-menu .menu-item.mono {
  font-family: var(--font-mono);
}
.conn-context-menu .menu-item.submenu-wrap {
  position: relative;
}
/* Standalone (not nested under .conn-context-menu): MenuDivider is also
   rendered inside teleported .menu-submenu flyouts, which live at <body>. */
.conn-context-menu .menu-divider,
.menu-submenu .menu-divider {
  height: 1px;
  background: var(--border-subtle);
  margin: 0.25rem 0.375rem;
}
/* Trailing chevron on a submenu-wrap row (settings + type-filter menus).
   margin-left:auto pushes it to the right edge of the flex .iconic row. */
.conn-context-menu .menu-icon-trailing {
  margin-left: auto;
  flex-shrink: 0;
  font-size: 0.8125rem;
  color: var(--text-tertiary);
}
/* Nested flyout / submenu surface — inherits the same surface skin. Teleported
   to <body> and fixed-positioned by MenuSubmenu (so a scrollable overflow root
   menu can never clip it), each flyout is placed beside its .submenu-wrap row
   against the window edges: right by default, flipped left only when the right
   side can't fit it, vertically clamped so its scrollbar stays on screen.
   The 0.1875rem placement overlap onto the parent row keeps the parent→flyout
   mouse path free of a dead gap (which would close the flyout). */
.menu-submenu {
  position: fixed;
  /* above the root menu (z-index 99999): the placement overlap paints over it */
  z-index: 100000;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-lg);
  min-width: 8.75rem;
  padding: 0.25rem;
  backdrop-filter: blur(0.5rem);
  overflow-y: auto;
}
.menu-submenu .menu-item {
  padding: 0.4375rem 0.75rem;
  font-size: 0.75rem;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  user-select: none;
  white-space: nowrap;
  border-radius: var(--radius-sm);
  transition: all 0.1s ease;
}
.menu-submenu .menu-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.menu-submenu .menu-item.active {
  color: var(--accent);
  font-weight: 500;
}
/* Iconic rows inside a submenu flyout need the same flex+gap as the main
   menu's (.conn-context-menu .menu-item.iconic) — without it the leading
   icon hugs the label with baseline misalignment. */
.menu-submenu .menu-item.iconic {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}
</style>