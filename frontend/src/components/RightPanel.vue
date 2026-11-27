<template>
  <div
    ref="panelEl"
    class="right-panel"
    :class="{ resizing: isResizing }"
    v-show="!uiLayout.narrowLayout && (uiLayout.rightPanelVisible || !!uiLayout.draggingPanel)"
    :style="{ width: uiLayout.rightPanelWidth + 'px' }"
  >
    <div class="resize-handle" @mousedown="onResizeStart" />
    <div
      class="right-panel-header"
      :class="{ 'drag-over': stripDragOver }"
      @contextmenu.prevent="onTabStripContextMenu"
      @dragover="onStripDragOver"
      @drop.prevent="onStripDrop"
    >
      <!-- Tabs (same style as the left sidebar's strip): icon-only, the
           active one spells its panel name out (all do with the tab-names
           preference on). Tooltip adds the toggle shortcut. -->
      <button
        v-for="key in rightTabKeys"
        :key="key"
        class="right-panel-tab"
        :class="{ active: uiLayout.rightActiveView === key, labeled: showTabLabel(key), 'drop-before': dropIndicator?.key === key && dropIndicator?.position === 'before', 'drop-after': dropIndicator?.key === key && dropIndicator?.position === 'after' }"
        :title="tabTooltip(key)"
        draggable="true"
        @dragstart="onDragStart($event, key)"
        @dragend="onDragEnd"
        @dragover="onTabDragOver($event, key)"
        @drop.prevent.stop="onTabDrop($event, key)"
        @contextmenu.prevent.stop="onTabContextMenu($event, key)"
        @click="emit('select', key)"
      >
        <el-icon><component :is="TAB_ICONS[key]" :size="'0.875rem'" /></el-icon>
        <span v-if="showTabLabel(key)" class="right-panel-tab-label">{{ t(tabTitleKey(key)) }}</span>
      </button>
      <span class="right-panel-spacer" />
      <!-- More menu of the active panel (move to dock / hide), left of the
           close button — the per-panel title row is gone. -->
      <PanelTitleBar v-if="uiLayout.rightActiveView" :view="uiLayout.rightActiveView" button-mode />
      <button class="icon-btn" @click="uiLayout.setRightPanelVisible(false)" :title="t('sidebar.collapse')">
        <el-icon><X :size="'0.875rem'" /></el-icon>
      </button>
    </div>

    <!-- Teleport target for views docked to the right rail. Always in the
         DOM (the panel root only v-shows) so conditional <Teleport>s can
         resolve it at any time. -->
    <div id="right-panel-body" class="right-panel-body">
      <!-- Empty rail: offer one-click docking for the views living
           elsewhere. -->
      <div v-if="rightTabKeys.length === 0" class="right-panel-empty">
        <div class="empty-hint">{{ t('sidebar.sidebarEmpty') }}</div>
        <div class="empty-actions">
          <button
            v-for="key in dockableViews"
            :key="key"
            class="empty-chip"
            @click="uiLayout.movePanel(key, 'right', Number.MAX_SAFE_INTEGER)"
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
import { X, Network, FolderTree, Activity, ArrowRightLeft, Zap, Clock, Palette, Send, NotebookPen, Sparkles } from '@lucide/vue'
import { useI18n } from '../i18n'
import { useUiLayoutStore, RIGHT_PANEL_MIN_WIDTH, type SidebarView } from '../stores/uiLayoutStore'
/* Dragging the rail width below this hides it (VS Code-style); the same drag
   pulling back out restores it at the min width */
const RIGHT_PANEL_COLLAPSE_AT = 60
import { useSettingsStore } from '../stores/settingsStore'
import { SIDEBAR_TAB_DEFAULTS, SIDEBAR_TAB_ORDER } from '../types/settings'
import { usePanelStripDrag } from '../composables/usePanelStripDrag'
import { shortcutSuffix, PANEL_VIEW_ACTIONS } from '../composables/shortcutHint'
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
  commandSend: Send,
  notes: NotebookPen,
  ai: Sparkles,
}

function tabVisible(key: string): boolean {
  return settingsStore.settings.sidebarTabs?.[key] ?? SIDEBAR_TAB_DEFAULTS[key] ?? true
}

// Right-strip tabs: views effectively docked to the right rail (none in
// narrow layout — everything merges into the sidebar) and not hidden via the
// sidebarTabs visibility setting, in persisted order.
const rightTabKeys = computed(() =>
  uiLayout.orderedViews.filter(key => tabVisible(key) && uiLayout.effectiveLocation(key) === 'right')
)

// Empty-rail placeholder: views docked somewhere else (one click docks them
// here). Fixed SIDEBAR_TAB_ORDER — the hint list must not jump around as the
// user reorders tabs.
const dockableViews = computed(() =>
  SIDEBAR_TAB_ORDER.map(tab => tab.key)
    .filter(key => tabVisible(key) && uiLayout.panelLocation(key) !== 'right')
)

function tabTitleKey(key: string): string {
  const entry = SIDEBAR_TAB_ORDER.find(tab => tab.key === key)
  return entry ? entry.labelKey : key
}

// Whether this tab carries a text label: 'all' labels every tab, 'active'
// only the strip's current tab, 'none' is icon-only.
function showTabLabel(key: string): boolean {
  const mode = uiLayout.panelTabLabels
  return mode === 'all' || (mode === 'active' && uiLayout.rightActiveView === key)
}

// Tab tooltip: panel name plus the bound toggle shortcut, e.g. "AI (Ctrl+I)".
// '' suffix when unbound / on mobile.
function tabTooltip(key: string): string {
  const action = PANEL_VIEW_ACTIONS[key]
  return t(tabTitleKey(key)) + (action ? shortcutSuffix(action) : '')
}

// ── Tab-strip drag & drop (reorder / move between docks) ──
const { dropIndicator, stripDragOver, onDragStart, onDragEnd, onTabDragOver, onStripDragOver, onTabDrop, onStripDrop } = usePanelStripDrag('right')

// Keep the right dock's active view valid: when the active view leaves the
// rail (moved to another dock / hidden / none docked yet), fall back to the
// first right tab so the rail never shows an empty body.
watch(
  () => [rightTabKeys.value, uiLayout.rightActiveView] as const,
  ([keys, active]) => {
    if (keys.length > 0 && !(active && keys.includes(active))) {
      uiLayout.rightActiveView = keys[0] as SidebarView
    }
  },
  { immediate: true },
)

// ── Left-edge resize (mirrors the bottom panel's top handle) ──
const panelEl = ref<HTMLElement | null>(null)
const isResizing = ref(false)

function onResizeStart(e: MouseEvent) {
  e.preventDefault()
  isResizing.value = true
  const el = panelEl.value
  if (!el) return
  const startX = e.clientX
  const startWidth = el.offsetWidth

  window.dispatchEvent(new CustomEvent('split:resize-start'))

  let collapsedDuringDrag = false

  function onMouseMove(ev: MouseEvent) {
    if (!isResizing.value) return
    const raw = startWidth + (startX - ev.clientX)
    /* Dragged below the collapse point: hide the rail, but keep tracking the
       drag — pulling back out reopens it at the min width (VS Code-style) */
    if (raw <= RIGHT_PANEL_COLLAPSE_AT) {
      collapsedDuringDrag = true
      el!.style.width = '0px'
      return
    }
    collapsedDuringDrag = false
    const maxWidth = Math.min(720, window.innerWidth - 320)
    const newWidth = Math.min(Math.max(raw, RIGHT_PANEL_MIN_WIDTH), maxWidth)
    el!.style.width = newWidth + 'px'
  }

  function onMouseUp() {
    isResizing.value = false
    if (collapsedDuringDrag) {
      /* Closed via drag: hide the rail and persist a sane width so the next
         open doesn't restore 0 */
      uiLayout.setRightPanelWidth(RIGHT_PANEL_MIN_WIDTH)
      uiLayout.setRightPanelVisible(false)
    } else {
      uiLayout.setRightPanelWidth(el!.offsetWidth)
    }
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    window.dispatchEvent(new CustomEvent('split:resize-end'))
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

// ── Tab-strip context menu (move views between docks) ──
const tabStripMenuRef = ref<InstanceType<typeof PanelStripMenu> | null>(null)

function onTabStripContextMenu(e: MouseEvent) {
  // Right-click on the strip's empty area: visibility toggles only.
  tabStripMenuRef.value?.openAt(e.clientX, e.clientY)
}

// Right-click on a specific tab: the "move to" submenu acts on that tab.
function onTabContextMenu(e: MouseEvent, key: string) {
  tabStripMenuRef.value?.openAt(e.clientX, e.clientY, key)
}

// #right-panel-body exists from now on — DockHost teleports may engage.
onMounted(() => {
  uiLayout.rightPanelBodyReady = true
})
</script>

<style scoped>
.right-panel {
  position: relative;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  /* No overflow:hidden: the resize handle sits outside the box in the gutter */
}

.right-panel.resizing {
  transition: none;
}

.resize-handle {
  position: absolute;
  /* Sit centered in the 4px gutter left of the rail. The offset is from the
     padding-box edge, and the panel's 1px border shifts that 1px sideways,
     so the extra -1px puts the handle exactly over the gutter. */
  top: 0;
  bottom: 0;
  left: calc(-0.25rem - 1px);
  width: 0.25rem;
  cursor: col-resize;
  z-index: 10;
  background: transparent;
}

/* Hover or crossing-grip hint: accent bar filling the 4px gutter */
.resize-handle:hover::after,
.resize-handle.hint::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  bottom: 0;
  background: var(--accent);
  box-shadow: 0 0 0.375rem var(--accent-glow);
}

.right-panel-header {
  display: flex;
  align-items: center;
  gap: 0.125rem;
  padding: 0.375rem 0.375rem 0;
  flex-shrink: 0;
  /* No border-bottom: the active panel's title bar below is the separator,
     same as the left sidebar (its strip has no line either). */
}

/* View title bars docked in THIS rail sit further in from the left edge
   than the shell-wide 0.375rem — mirrors the left sidebar's override. */
.right-panel :deep(.panel-title-bar) {
  padding-left: 0.625rem;
}

.right-panel-tab {
  /* Mirrors .sidebar-tab: icon-only square tab, name in the tooltip */
  font-size: 0.875rem;
  width: 1.625rem;
  height: 1.625rem;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  color: var(--text-muted);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  transition: all 0.15s;
  position: relative;
}

/* Active tab spells its panel name out: icon+label pill (left sidebar parity) */
.right-panel-tab.labeled {
  width: auto;
  padding: 0 0.5rem;
  gap: 0.25rem;
}

.right-panel-tab-label {
  font-size: 0.75rem;
  font-family: var(--font-ui);
  white-space: nowrap;
  /* Long locales (fr "Envoi de commandes" = 18 chars) must not push the
     strip's action buttons out — clip with an ellipsis instead. */
  max-width: 10rem;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Drag & drop insertion line (tab reorder / cross-dock move) */
.right-panel-tab.drop-before::before,
.right-panel-tab.drop-after::after {
  content: '';
  position: absolute;
  top: 0.125rem;
  bottom: 0.125rem;
  width: 2px;
  border-radius: 1px;
  background: var(--accent);
}

.right-panel-tab.drop-before::before { left: -1px; }
.right-panel-tab.drop-after::after { right: -1px; }

.right-panel-header.drag-over {
  background: var(--bg-hover);
}

.right-panel-tab:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.right-panel-tab.active {
  /* Activity-bar tab: neutral gray rounded highlight, bright label */
  color: var(--text-primary);
  background: var(--bg-active);
}

.right-panel-spacer {
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

.right-panel-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* Empty rail placeholder: dockable-panel chips */
.right-panel-empty {
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
  padding: 0.25rem 0.375rem;
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
