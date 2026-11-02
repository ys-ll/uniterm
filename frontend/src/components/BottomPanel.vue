<template>
  <div
    ref="panelEl"
    class="bottom-panel"
    :class="{ resizing: isResizing }"
    v-show="!uiLayout.narrowLayout && (uiLayout.bottomPanelVisible || !!uiLayout.draggingPanel)"
    :style="{ height: uiLayout.bottomPanelHeight + 'px' }"
  >
    <div class="resize-handle" @mousedown="onResizeStart" />
    <div
      class="bottom-panel-header"
      :class="{ 'drag-over': stripDragOver }"
      @contextmenu.prevent="onTabStripContextMenu"
      @dragover="onStripDragOver"
      @drop.prevent="onStripDrop"
    >
      <button
        v-for="key in bottomTabKeys"
        :key="key"
        class="bottom-panel-tab"
        :class="{ active: uiLayout.bottomActiveView === key, 'drop-before': dropIndicator?.key === key && dropIndicator?.position === 'before', 'drop-after': dropIndicator?.key === key && dropIndicator?.position === 'after' }"
        :title="t(tabTitleKey(key))"
        draggable="true"
        @dragstart="onDragStart($event, key)"
        @dragend="onDragEnd"
        @dragover="onTabDragOver($event, key)"
        @drop.prevent.stop="onTabDrop($event, key)"
        @contextmenu.prevent.stop="onTabContextMenu($event, key)"
        @click="emit('select', key)"
      >
        <el-icon><component :is="TAB_ICONS[key]" :size="'0.75rem'" /></el-icon>
        <span class="bottom-panel-tab-label">{{ t(tabTitleKey(key)) }}</span>
      </button>
      <span class="bottom-panel-spacer" />
      <!-- More menu for the bottom dock's active view (no title bar here —
           the tab already labels it); sits left of the close button. -->
      <PanelTitleBar v-if="uiLayout.bottomActiveView" :view="uiLayout.bottomActiveView" button-mode />
      <button class="icon-btn" @click="uiLayout.setBottomPanelVisible(false)" :title="t('sidebar.collapse')">
        <el-icon><X :size="'0.875rem'" /></el-icon>
      </button>
    </div>

    <!-- Teleport target for the sidebar's docked views. Always in the DOM
         (the panel root only v-shows) so Sidebar's conditional <Teleport>
         can resolve it at any time. -->
    <div id="bottom-panel-body" class="bottom-panel-body">
      <!-- Empty panel: the toggle button opens it even with nothing docked;
           offer one-click docking for the views living in the sidebar. -->
      <div v-if="bottomTabKeys.length === 0" class="bottom-panel-empty">
        <div class="empty-hint">{{ t('sidebar.bottomPanelEmpty') }}</div>
        <div class="empty-actions">
          <button
            v-for="key in dockableViews"
            :key="key"
            class="empty-chip"
            @click="uiLayout.movePanel(key, 'bottom', Number.MAX_SAFE_INTEGER)"
          >
            <el-icon><component :is="TAB_ICONS[key]" :size="'0.75rem'" /></el-icon>
            {{ t(tabTitleKey(key)) }}
          </button>
        </div>
      </div>
    </div>

    <PanelStripMenu ref="tabStripMenuRef" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { X, Network, FolderTree, Activity, ArrowRightLeft, Zap, Clock, Palette } from '@lucide/vue'
import { useI18n } from '../i18n'
import { useUiLayoutStore, BOTTOM_PANEL_MIN_HEIGHT, type SidebarView } from '../stores/uiLayoutStore'
import { useSettingsStore } from '../stores/settingsStore'
import { SIDEBAR_TAB_DEFAULTS, SIDEBAR_TAB_ORDER } from '../types/settings'
import { usePanelStripDrag } from '../composables/usePanelStripDrag'
import PanelStripMenu from './PanelStripMenu.vue'
import PanelTitleBar from './PanelTitleBar.vue'

const { t } = useI18n()
const uiLayout = useUiLayoutStore()
const settingsStore = useSettingsStore()

const emit = defineEmits<{
  (e: 'select', view: string): void
}>()

const TAB_ICONS: Record<string, any> = {
  connections: Network,
  files: FolderTree,
  monitor: Activity,
  tunnels: ArrowRightLeft,
  quickCommands: Zap,
  history: Clock,
  personalization: Palette,
}

function tabVisible(key: string): boolean {
  return settingsStore.settings.sidebarTabs?.[key] ?? SIDEBAR_TAB_DEFAULTS[key] ?? true
}

// Bottom-strip tabs: views effectively docked to the bottom panel (none in
// narrow layout — everything merges into the sidebar) and not hidden via the
// sidebarTabs visibility setting, in persisted order.
const bottomTabKeys = computed(() =>
  uiLayout.orderedViews.filter(key => tabVisible(key) && uiLayout.effectiveLocation(key) === 'bottom')
)

// Empty-panel placeholder: views still docked to the sidebar (one click
// docks them here). Fixed SIDEBAR_TAB_ORDER — the hint list must not jump
// around as the user reorders tabs.
const dockableViews = computed(() =>
  SIDEBAR_TAB_ORDER.map(tab => tab.key)
    .filter(key => tabVisible(key) && uiLayout.panelLocation(key) !== 'bottom')
)

// Panel is shown whenever it's toggled on (even empty — see the placeholder
// in the body); during a drag it also reveals as a drop target. The root
// v-show reads uiLayout.bottomPanelVisible directly.

function tabTitleKey(key: string): string {
  const entry = SIDEBAR_TAB_ORDER.find(tab => tab.key === key)
  return entry ? entry.labelKey : key
}

// ── Tab-strip drag & drop (reorder / move between sidebar and bottom) ──
const { dropIndicator, stripDragOver, onDragStart, onDragEnd, onTabDragOver, onStripDragOver, onTabDrop, onStripDrop } = usePanelStripDrag('bottom')

// Keep the bottom dock's active view valid: when the active view leaves the
// bottom panel (moved to the sidebar / hidden / none docked yet), fall back
// to the first bottom tab so the panel never shows an empty body.
watch(
  () => [bottomTabKeys.value, uiLayout.bottomActiveView] as const,
  ([keys, active]) => {
    if (keys.length > 0 && !(active && keys.includes(active))) {
      uiLayout.bottomActiveView = keys[0] as SidebarView
    }
  },
  { immediate: true },
)

// ── Top-edge resize (mirrors the sidebar's side handle) ──
const panelEl = ref<HTMLElement | null>(null)
const isResizing = ref(false)

function onResizeStart(e: MouseEvent) {
  e.preventDefault()
  isResizing.value = true
  const el = panelEl.value
  if (!el) return
  const startY = e.clientY
  const startHeight = el.offsetHeight

  window.dispatchEvent(new CustomEvent('split:resize-start'))

  function onMouseMove(ev: MouseEvent) {
    if (!isResizing.value) return
    const maxHeight = window.innerHeight - 160
    const newHeight = Math.min(Math.max(startHeight + (startY - ev.clientY), BOTTOM_PANEL_MIN_HEIGHT), maxHeight)
    el!.style.height = newHeight + 'px'
  }

  function onMouseUp() {
    isResizing.value = false
    uiLayout.setBottomPanelHeight(el!.offsetHeight)
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    window.dispatchEvent(new CustomEvent('split:resize-end'))
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

// ── Tab-strip context menu (move views between sidebar and bottom) ──
const tabStripMenuRef = ref<InstanceType<typeof PanelStripMenu> | null>(null)

function onTabStripContextMenu(e: MouseEvent) {
  // Right-click on the strip's empty area: visibility toggles only.
  tabStripMenuRef.value?.openAt(e.clientX, e.clientY)
}

// Right-click on a specific tab: the "move to" submenu acts on that tab.
function onTabContextMenu(e: MouseEvent, key: string) {
  tabStripMenuRef.value?.openAt(e.clientX, e.clientY, key)
}

// #bottom-panel-body exists from now on — DockHost teleports may engage.
onMounted(() => {
  uiLayout.bottomPanelBodyReady = true
})
</script>

<style scoped>
.bottom-panel {
  position: relative;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-elevated);
  border-top: 1px solid var(--border-subtle);
}

.bottom-panel.resizing {
  transition: none;
}

.resize-handle {
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  height: 0.375rem;
  cursor: row-resize;
  z-index: 10;
  background: transparent;
}

/* Hover: 0.1875rem accent bar along the top edge */
.resize-handle:hover::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  height: 0.1875rem;
  background: var(--accent);
  box-shadow: 0 0 0.375rem var(--accent-glow);
}

.bottom-panel-header {
  display: flex;
  align-items: center;
  gap: 0.125rem;
  /* Horizontal padding matches the sidebar's toolbar rows. */
  padding: 0.375rem 0.625rem;
  flex-shrink: 0;
  border-bottom: 1px solid var(--border-subtle);
}

.bottom-panel-tab {
  font-size: 0.75rem;
  height: 1.625rem;
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0 0.5rem;
  color: var(--text-muted);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  transition: all 0.15s;
  position: relative;
  white-space: nowrap;
}

.bottom-panel-tab-label {
  font-family: var(--font-ui);
}

/* Drag & drop insertion line (tab reorder / cross-dock move) */
.bottom-panel-tab.drop-before::before,
.bottom-panel-tab.drop-after::after {
  content: '';
  position: absolute;
  top: 0.125rem;
  bottom: 0.125rem;
  width: 2px;
  border-radius: 1px;
  background: var(--accent);
}

.bottom-panel-tab.drop-before::before { left: -1px; }
.bottom-panel-tab.drop-after::after { right: -1px; }

.bottom-panel-header.drag-over {
  background: var(--bg-hover);
}

.bottom-panel-tab:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.bottom-panel-tab.active {
  color: var(--accent);
  background: var(--accent-subtle);
}

.bottom-panel-spacer {
  flex: 1;
}

.icon-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.625rem;
  height: 1.625rem;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  transition: all 0.12s ease;
}

.icon-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.bottom-panel-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* Empty panel placeholder: dockable-panel chips */
.bottom-panel-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 0.75rem;
}

.empty-hint {
  font-size: 0.75rem;
  font-family: var(--font-ui);
  color: var(--text-muted);
}

.empty-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.375rem;
}

.empty-chip {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem 0.625rem;
  font-size: 0.75rem;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  background: var(--bg-subtle);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.12s ease;
}

.empty-chip:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
  border-color: var(--accent);
}
</style>
