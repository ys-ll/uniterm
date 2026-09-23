<template>
  <div
    v-if="hasVisibleTabs"
    class="bottom-bar"
    :class="{ collapsed, resizing: isResizing }"
    :style="collapsed ? undefined : { height: height + 'px' }"
  >
    <div v-if="!collapsed" class="bottom-resize-handle" @mousedown="onResizeStart" />
    <!-- The panel area is the same view stack as the left sidebar, mounted in
         its 'bottom' variant: full width, labelled tabs, no width drag handle.
         It keeps its own active view, so both areas can show different views. -->
    <Sidebar
      :visible="true"
      variant="bottom"
      tabs-setting="bottomBarTabs"
      @toggle="collapsed = !collapsed"
      @view-change="collapsed = false"
      @connect="emit('connect', $event)"
      @connect-to-workspace="emit('connectToWorkspace', $event)"
      @create-workspace="emit('createWorkspace', $event)"
      @connect-only="emit('connectOnly', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import Sidebar from './Sidebar.vue'
import { useSettingsStore } from '../stores/settingsStore'
import { SIDEBAR_TAB_ORDER, BOTTOM_BAR_TAB_DEFAULTS } from '../types/settings'

const emit = defineEmits(['connect', 'connectToWorkspace', 'createWorkspace', 'connectOnly'])

const settingsStore = useSettingsStore()

// The bar only exists while at least one view is offered. "connections" is
// fixed (never hideable), so in practice the bar is always available and the
// user hides it by collapsing it instead.
const hasVisibleTabs = computed(() => SIDEBAR_TAB_ORDER.some(tab => bottomTabVisible(tab.key)))

function bottomTabVisible(key: string): boolean {
  return settingsStore.settings.bottomBarTabs?.[key] ?? BOTTOM_BAR_TAB_DEFAULTS[key] ?? true
}

// ── Panel height ──
// Session-local (like the left sidebar's width): dragging the top edge resizes
// the panel, the collapse button hides the body and leaves the tab strip.
const DEFAULT_HEIGHT = 260
const MIN_HEIGHT = 120
const MAX_HEIGHT_RATIO = 0.7

const height = ref(DEFAULT_HEIGHT)
const collapsed = ref(false)
const isResizing = ref(false)

function maxHeight(): number {
  return Math.max(MIN_HEIGHT, Math.round(window.innerHeight * MAX_HEIGHT_RATIO))
}

function onResizeStart(e: MouseEvent) {
  isResizing.value = true
  const startY = e.clientY
  const startHeight = height.value

  // Same convention as the sidebar/split resizes: terminals stand down while a
  // drag is in flight so they don't fight the live layout changes.
  window.dispatchEvent(new CustomEvent('split:resize-start'))

  function onMouseMove(ev: MouseEvent) {
    // Dragging the top edge upwards grows the panel.
    const next = startHeight + (startY - ev.clientY)
    height.value = Math.min(Math.max(next, MIN_HEIGHT), maxHeight())
  }

  function onMouseUp() {
    isResizing.value = false
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    window.dispatchEvent(new CustomEvent('split:resize-end'))
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}
</script>

<style scoped>
.bottom-bar {
  position: relative;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  min-height: 0;
  background: var(--bg-elevated);
  border-top: 1px solid var(--border-subtle);
  z-index: 1;
}

.bottom-bar.resizing {
  transition: none;
}

/* Collapsed: the tab strip stays (so the panel can be reopened), the view body
   is dropped from layout entirely. */
.bottom-bar.collapsed :deep(.sidebar) {
  overflow: hidden;
}

.bottom-bar.collapsed :deep(.sidebar) > :not(.sidebar-header) {
  display: none;
}

.bottom-resize-handle {
  position: absolute;
  top: -0.1875rem;
  left: 0;
  right: 0;
  height: 0.375rem;
  cursor: row-resize;
  z-index: 10;
  background: transparent;
}

.bottom-resize-handle:hover::after {
  content: '';
  position: absolute;
  top: 0.09375rem;
  left: 0;
  right: 0;
  height: 0.1875rem;
  background: var(--accent);
  box-shadow: 0 0 0.375rem var(--accent-glow);
}
</style>
