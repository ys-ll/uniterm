<template>
  <div
    ref="sidebarEl"
    class="sidebar"
    :class="{ collapsed: !visible, resizing: isResizing }"
    :style="{ width: sidebarWidth + 'px' }"
  >
    <div class="resize-handle" @mousedown="onResizeStart" />
    <div class="sidebar-header" @contextmenu.prevent="onTabStripContextMenu">
      <button class="sidebar-tab" :class="{ active: activeView === 'connections' }" @click="activeView = 'connections'" :title="t('header.connections')"><el-icon><Network :size="'0.875rem'" /></el-icon></button>
      <button v-if="tabVisible('files')" class="sidebar-tab" :class="{ active: activeView === 'files' }" @click="onFilesTabClick" :title="t('header.files')"><el-icon><FolderTree :size="'0.875rem'" /></el-icon></button>
      <button v-if="tabVisible('monitor')" class="sidebar-tab" :class="{ active: activeView === 'monitor' }" @click="onMonitorTabClick" :title="t('header.monitor')"><el-icon><Activity :size="'0.875rem'" /></el-icon></button>
      <button v-if="tabVisible('tunnels')" class="sidebar-tab" :class="{ active: activeView === 'tunnels' }" @click="activeView = 'tunnels'" :title="t('tunnels.tunnelsTab')"><el-icon><ArrowRightLeft :size="'0.875rem'" /></el-icon></button>
      <button v-if="tabVisible('quickCommands')" class="sidebar-tab" :class="{ active: activeView === 'quickCommands' }" @click="activeView = 'quickCommands'" :title="quickCommandsTitle"><el-icon><Zap :size="'0.875rem'" /></el-icon></button>
      <button v-if="tabVisible('history')" class="sidebar-tab" :class="{ active: activeView === 'history' }" @click="activeView = 'history'" :title="t('quickCommands.historyTab')"><el-icon><Clock :size="'0.875rem'" /></el-icon></button>
      <button v-if="tabVisible('personalization')" class="sidebar-tab" :class="{ active: activeView === 'personalization' }" @click="activeView = 'personalization'" :title="t('sidebar.personalization')"><el-icon><Palette :size="'0.875rem'" /></el-icon></button>
      <button class="icon-btn" @click="emit('toggle')" :title="t('sidebar.collapse')"><el-icon><X :size="'0.875rem'" /></el-icon></button>
    </div>

    <QuickConnectPanel
      v-if="activeView === 'connections'"
      ref="quickConnectPanelRef"
      :tree="tree"
      :open-panel-conn-ids="openPanelConnIds"
      :show-host-row-buttons="showHostRowButtons"
      :is-favorite="(id: string) => favoriteStore.isFavorite(id)"
      :conn-icon="connIcon"
      :get-subtitle="getSubtitle"
      :on-context-menu="onContextMenu"
      :on-conn-more-click="onConnMoreClick"
      @new-conn-select="onNewConnSelect"
      @empty-area-contextmenu="onEmptyAreaContextMenu"
      @virtual-group-contextmenu="onVirtualGroupContextMenu"
      @new-connection-from-search="openNewFormFromSearch"
    />

    <QuickCommandsPanel v-if="activeView === 'quickCommands'" ref="quickCommandsRef" />

    <TunnelsPanel v-if="activeView === 'tunnels'" />

    <HistoryPanel v-if="activeView === 'history'" />

    <FileSidebar v-if="activeView === 'files'" />

    <MonitorOverviewSidebar v-if="activeView === 'monitor'" />

    <PersonalizationPanel v-if="activeView === 'personalization'" />

    <ConnectionForm v-model="showForm" :edit-config="editConfig" :default-group-id="newConnGroupId" @save="onSave" @connect="onConnectFromForm" @connect-only="(c: ConnectionConfig) => { showForm = false; editConfig = undefined; emit('connectOnly', c) }" />
    <ExportDialog v-model:visible="showExportDialog" />
    <ImportDialog v-model:visible="showImportDialog" />


    <!-- Tab-strip context menu: toggle which sidebar tabs are shown.
         Reuses the same AppSettings.sidebarTabs as the Settings page card.
         "connections" is fixed, so it is not listed. -->
    <Menu ref="tabStripMenuRef" v-model:visible="tabStripMenuVisible">
      <MenuItem
        v-for="tab in SIDEBAR_TAB_ORDER.filter(tab => tab.key !== 'connections')"
        :key="tab.key"
        iconic
        :icon="tabVisible(tab.key) ? Check : undefined"
        @click="onTabVisibilityClick(tab.key)"
      >{{ t(tab.labelKey) }}</MenuItem>
    </Menu>

    <!-- Connection context menu -->
    <ConnectionContextMenu
      ref="menuRef"
      :config="selectedConn"
      :targets="ctxTargets"
      v-model:visible="menuVisible"
      @connect="onCtxConnect"
      @connect-to-workspace="onCtxConnectToWorkspace"
      @create-workspace="onCtxCreateWorkspace"
      @edit="doEdit"
      @change-group="(targets: ConnectionConfig[]) => openChangeGroupFor(targets.map(c => c.id))"
      @new-group="() => doNewGroup(selectedGroupParentId())"
      @delete="onCtxDelete"
    />

    <!-- Group context menu -->
    <GroupContextMenu
      ref="groupMenuRef"
      :group="selectedGroup"
      v-model:visible="groupMenuVisible"
      @new-group="() => { if (selectedGroup) doNewGroup(selectedGroup.id) }"
      @new-connection="doNewConnInGroup"
      @rename="doRenameGroup"
      @change-parent="doChangeParentGroup"
      @delete-group="doDeleteGroup"
    />

    <!-- Empty area context menu -->
    <Menu ref="emptyAreaMenuRef" v-model:visible="emptyAreaMenuVisible">
      <MenuItem @click="doNewGroup()">{{ t('conn.newGroupTitle') }}</MenuItem>
    </Menu>

    <DeleteGroupDialog v-model:visible="showDeleteGroupDialog" :group="selectedGroup" @confirm="confirmDeleteGroup" />

    <RenameGroupDialog v-model:visible="showRenameGroupDialog" :name="selectedGroup?.name || ''" @confirm="confirmRenameGroup" />

    <!-- Move to dialog -->
    <el-dialog append-to-body v-model="showChangeGroupDialog" :title="t('conn.group')" width="25rem">
      <el-tree-select
        v-model="changeGroupTargetId"
        :data="groupTreeData"
        :render-after-expand="false"
        check-strictly
        clearable
        :placeholder="t('conn.noGroup')"
        style="width:100%"
      />
      <template #footer>
        <el-button @click="showChangeGroupDialog = false">{{ t('conn.cancel') }}</el-button>
        <el-button type="primary" @click="confirmChangeGroup">{{ t('conn.save') }}</el-button>
      </template>
    </el-dialog>

    <NewGroupDialog v-model:visible="showNewGroupDialog" :parent-id="newGroupParentId" @confirm="confirmNewGroup" />

    <NewGroupDialog v-model:visible="showChangeNewGroupDialog" :parent-id="changeNewGroupParentId" @confirm="confirmChangeNewGroup" />

  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick, provide } from 'vue'
import { X, Check, Network, Zap, Clock, Palette, SquareTerminal, Activity, FolderTree, ArrowRightLeft } from '@lucide/vue'
import { getShellLabel as getShellLabelBase } from '../utils/shellLabel'
import { useConnectionStore } from '../stores/connectionStore'
import { useFavoriteStore } from '../stores/favoriteStore'

import { usePanelStore } from '../stores/panelStore'
import { useTabStore } from '../stores/tabStore'
import { useSettingsStore } from '../stores/settingsStore'
import { useI18n } from '../i18n'
import ConnectionForm from './ConnectionForm.vue'
import ExportDialog from './ExportDialog.vue'
import ImportDialog from './ImportDialog.vue'
import QuickCommandsPanel from './QuickCommandsPanel.vue'
import TunnelsPanel from './TunnelsPanel.vue'
import HistoryPanel from './HistoryPanel.vue'
import FileSidebar from './FileSidebar.vue'
import MonitorOverviewSidebar from './MonitorOverviewSidebar.vue'
import { useCompanionStore } from '../stores/companionStore'

import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'

import type { ConnectionConfig, ConnectionGroup } from '../types/session'
import { parseQuickConnect, formatConnSubtitle } from '../utils/quickConnect'
import ConnectionContextMenu from './ConnectionContextMenu.vue'
import { connectionTypeIcon } from '../utils/connectionTypes'

import GroupContextMenu from './GroupContextMenu.vue'
import RenameGroupDialog from './RenameGroupDialog.vue'
import NewGroupDialog from './NewGroupDialog.vue'
import DeleteGroupDialog from './DeleteGroupDialog.vue'
import { SIDEBAR_TAB_DEFAULTS, SIDEBAR_TAB_ORDER } from '../types/settings'
import { formatKeyBinding } from '../composables/useKeyboardShortcuts'
import { isMobilePlatform } from '../utils/platform'
import { useSidebarDrag } from '../composables/useSidebarDrag'
import { useConnectionTree } from '../composables/useConnectionTree'
import { useGroupActions } from '../composables/useGroupActions'
import QuickConnectPanel from './sidebar/QuickConnectPanel.vue'
import PersonalizationPanel from './sidebar/PersonalizationPanel.vue'

defineProps<{
  visible: boolean
}>()
const emit = defineEmits(['connect', 'connectToWorkspace', 'createWorkspace', 'connectOnly', 'toggle', 'openNetscan'])
const connectionStore = useConnectionStore()
const favoriteStore = useFavoriteStore()
const settingsStore = useSettingsStore()
// "Right-click menu" host-list mode hides the hover star/⋯ buttons entirely —
// the same connection menu stays reachable via right-click (issue #934).
const showHostRowButtons = computed(() => settingsStore.settings.hostListMenuStyle !== 'rightclick')
const panelStore = usePanelStore()
const tabStore = useTabStore()
const companionStore = useCompanionStore()
const { t } = useI18n()
const isMacPlatform = /Mac|iPhone|iPad/.test(navigator.userAgent)
const quickCommandsTitle = computed(() => {
  const label = t('quickCommands.quickCommandsTab')
  // No hardware keyboard on mobile — the shortcut hint is meaningless there.
  if (isMobilePlatform()) return label
  const binding = settingsStore.settings.keyboard.openQuickCommands
  const shortcut = binding ? formatKeyBinding(binding, isMacPlatform) : ''
  return shortcut ? `${label} (${shortcut})` : label
})
// Connection ids that currently have an open panel/session (panel.config.id),
// plus saved-workspace records whose live workspace tab is open. Reactive
// over the panel/tab stores, so it updates as panels and tabs open/close.
const openPanelConnIds = computed<Set<string>>(() => {
  const s = new Set<string>()
  for (const p of panelStore.panels.values()) {
    if (p.config?.id) s.add(p.config.id)
  }
  for (const tb of tabStore.tabs) {
    if (tb.type === 'workspace' && tb.savedWorkspaceId) s.add(tb.savedWorkspaceId)
  }
  return s
})
const showForm = ref(false)
const showExportDialog = ref(false)
const showImportDialog = ref(false)
const editConfig = ref<ConnectionConfig | undefined>(undefined)
const newConnGroupId = ref<string | undefined>(undefined)
type SidebarView = 'connections' | 'quickCommands' | 'history' | 'personalization' | 'files' | 'monitor' | 'tunnels'
const activeView = ref<SidebarView>('connections')
const quickCommandsRef = ref<InstanceType<typeof QuickCommandsPanel> | null>(null)
const quickConnectPanelRef = ref<InstanceType<typeof QuickConnectPanel> | null>(null)

function openQuickCommands() {
  activeView.value = 'quickCommands'
  nextTick(() => quickCommandsRef.value?.focusSearch())
}

// ── SSH companion: files / monitor folded into this sidebar ──
// Generic tab-switch entry mirroring a manual tab click: files/monitor route
// through their click handlers (flag + companion session), the rest just
// switch the view. Used by the App.vue sidebar shortcuts.
function openTab(view: SidebarView) {
  if (view === 'files') return onFilesTabClick()
  if (view === 'monitor') return onMonitorTabClick()
  activeView.value = view
  if (view === 'connections') nextTick(() => focusSearch())
}
function onFilesTabClick() {
  activeView.value = 'files'
  // Always open the files view (shows an idle/"need SSH" empty state when no
  // active SSH panel). If there is an active SSH panel, (re)establish the
  // companion SFTP session. Closing happens via the panel's X button, which
  // clears filesVisible and the watch below falls back to connections.
  companionStore.filesVisible = true
  const pid = companionStore.getActiveFilesPanelId()
  if (pid) companionStore.ensureSftp(pid).catch(() => {})
}
function onMonitorTabClick() {
  activeView.value = 'monitor'
  companionStore.monitorVisible = true
  const pid = companionStore.getActiveSshPanelId()
  if (pid) companionStore.ensureMonitor(pid).catch(() => {})
}
// If the companion panel hides itself (collapse button), fall back to connections view.
watch(
  () => [companionStore.filesVisible, companionStore.monitorVisible],
  ([filesVisible, monitorVisible]) => {
    if (!filesVisible && activeView.value === 'files') activeView.value = 'connections'
    if (!monitorVisible && activeView.value === 'monitor') activeView.value = 'connections'
  },
)

// ── Sidebar tab visibility ──
// Single source of truth is AppSettings.sidebarTabs (editable in Settings →
// basic); right-clicking the tab strip opens the same toggles as a shortcut.
// "connections" is the primary view and can never be hidden.

function tabVisible(key: string): boolean {
  return settingsStore.settings.sidebarTabs?.[key] ?? SIDEBAR_TAB_DEFAULTS[key] ?? true
}

const tabStripMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const tabStripMenuVisible = ref(false)

function onTabStripContextMenu(e: MouseEvent) {
  tabStripMenuRef.value?.openAt(e.clientX, e.clientY)
}

function onTabVisibilityClick(key: string) {
  if (key === 'connections') return // always visible
  const tabs = settingsStore.settings.sidebarTabs
  tabs[key] = !tabVisible(key)
  settingsStore.save()
  // Hiding the view that's currently active falls back to connections.
  if (!tabs[key] && activeView.value === key) activeView.value = 'connections'
}


// ── Sidebar resize (width drag + persistence) ──
const sidebarEl = ref<HTMLDivElement>()
const { sidebarWidth, isResizing, onResizeStart } = useSidebarDrag(sidebarEl)

// ── Connection context menu (needs Sidebar-level dialog refs) ──
const menuVisible = ref(false)
const groupMenuVisible = ref(false)
const selectedConn = ref<ConnectionConfig | null>(null)
const selectedGroup = ref<ConnectionGroup | null>(null)
const menuRef = ref<InstanceType<typeof ConnectionContextMenu> | null>(null)

// ── Connection tree (expand/collapse, filter, DnD, selection, keyboard) ──
const tree = useConnectionTree({
  sidebarEl,
  showForm,
  menuVisible,
  groupMenuVisible,
  selectedConn,
  onConnect: (c: ConnectionConfig) => emit('connect', c),
  openNewFormFromSearch: () => openNewFormFromSearch(),
})

function focusSearch() {
  // The search input only exists in the connections view; switch back first
  // so the shortcut works no matter which view the sidebar was left in.
  activeView.value = 'connections'
  nextTick(() => quickConnectPanelRef.value?.focusSearch())
}

// ── Group actions, context menus & new-connection dropdown ──
const groupMenuRef = ref<InstanceType<typeof GroupContextMenu> | null>(null)
const emptyAreaMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const emptyAreaMenuVisible = ref(false)
const {
  onContextMenu, onConnMoreClick, ctxTargets,
  onCtxConnect, onCtxConnectToWorkspace, onCtxCreateWorkspace, onCtxDelete, doEdit,
  showNewGroupDialog, newGroupParentId, groupTreeData,
  doNewConnInGroup, doNewGroup, selectedGroupParentId, confirmNewGroup,
  doChangeParentGroup, showChangeGroupDialog, changeGroupTargetId,
  showChangeNewGroupDialog, changeNewGroupParentId,
  openChangeGroupFor, confirmChangeGroup, confirmChangeNewGroup,
  onGroupContextMenu,
  onEmptyAreaContextMenu, onVirtualGroupContextMenu,
  showRenameGroupDialog, doRenameGroup, confirmRenameGroup,
  showDeleteGroupDialog, doDeleteGroup, confirmDeleteGroup,
  onNewConnSelect,
  openChangeGroupForGroup,
} = useGroupActions({
  selectedIds: tree.selectedIds,
  focusedId: tree.focusedId,
  getSelectedConnectionIds: tree.getSelectedConnectionIds,
  searchQuery: tree.searchQuery,
  selectedConn,
  selectedGroup,
  showForm,
  editConfig,
  newConnGroupId,
  menuVisible,
  groupMenuVisible,
  emptyAreaMenuVisible,
  menuRef,
  groupMenuRef,
  emptyAreaMenuRef,
  showImportDialog,
  showExportDialog,
  emit,
  openNewForm: () => openNewForm(),
  openNewFormFromSearch: () => openNewFormFromSearch(),
})



function getShellLabel(path: string): string {
  // Shared helper (utils/shellLabel.ts): same labels as the start-page
  // dropdown, connection form and settings, including the Cygwin/MSYS2
  // bash.exe disambiguation the inline copy here was missing.
  return getShellLabelBase(path, 'Local')
}

function getSubtitle(conn: ConnectionConfig): string {
  if (conn.type === 'container') return conn.containerRuntime || 'container'
  return formatConnSubtitle(conn, getShellLabel)
}

function connIcon(conn: ConnectionConfig) {
  return connectionTypeIcon(conn) || SquareTerminal
}


// ── Form handlers ──
function openNewForm() {
  editConfig.value = undefined
  newConnGroupId.value = undefined
  showForm.value = true
}

function openNewFormFromSearch() {
  const parsed = parseQuickConnect(tree.searchQuery.value.trim())
  editConfig.value = (parsed || { host: tree.searchQuery.value.trim() }) as ConnectionConfig
  tree.selectedIds.value = new Set()
  newConnGroupId.value = undefined
  showForm.value = true
}

function onSave(config: ConnectionConfig) {
  if (editConfig.value?.id) {
    connectionStore.update(config.id, config)
  } else {
    connectionStore.add(config)
  }
  showForm.value = false
  editConfig.value = undefined
}

function onConnectFromForm(config: ConnectionConfig) {
  if (editConfig.value?.id) {
    connectionStore.update(config.id, config)
  } else {
    connectionStore.add(config)
  }
  showForm.value = false
  editConfig.value = undefined
  emit('connect', config)
}

// ── Lifecycle ──
onMounted(async () => {
  window.addEventListener('app:locate-connection', tree.onLocateConnection)
  // Restore group collapse state from persisted settings
  await tree.initCollapseState()

})

onUnmounted(() => {
  window.removeEventListener('app:locate-connection', tree.onLocateConnection)
})

// Provide to GroupTreeItem (after all refs/functions are defined)
provide('expandedGroups', tree.expandedGroups)
provide('selectedIds', tree.selectedIds)
provide('openPanelConnIds', openPanelConnIds)
provide('dragOverGroupId', tree.dragOverGroupId)
provide('dropIndicator', tree.dropIndicator)
provide('groupHandlers', {
  onToggleGroup: tree.toggleGroup,
  onGroupContextMenu,
  onGroupDragOver: tree.onGroupDragOver,
  onGroupDragLeave: tree.onGroupDragLeave,
  onGroupDrop: tree.onGroupDrop,
  onItemClick: tree.onItemClick,
  onItemDblClick: tree.onItemDblClick,
  onDragStart: tree.onDragStart,
  onDragEnd: tree.onDragEnd,
  onConnDragOver: tree.onConnDragOver,
  onConnDrop: tree.onConnDrop,
  onContextMenu,
  onConnMoreClick,
})
provide('utils', {
  connIcon,
  getSubtitle,
  t,
})


defineExpose({ focusSearch, openQuickCommands, openTab, openChangeGroupFor, openChangeGroupForGroup, activeView })
</script>

<style scoped>
.sidebar {
  background: var(--bg-elevated);
  border-right: 1px solid var(--border-subtle);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  position: relative;
}

.sidebar.collapsed {
  width: 0 !important;
  border-right: none;
  overflow: hidden;
}

.sidebar.resizing {
  transition: none;
}

.resize-handle {
  position: absolute;
  right: -0.375rem;
  top: 0;
  bottom: 0;
  width: 0.375rem;
  cursor: col-resize;
  z-index: 10;
  background: transparent;
}

/* Hover: 0.1875rem accent bar extending into sidebar */
.resize-handle:hover::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: 0.1875rem;
  background: var(--accent);
  box-shadow: 0 0 0.375rem var(--accent-glow);
}

.sidebar-header {
  display: flex;
  align-items: center;
  gap: 0.125rem;
  /* 下 padding 由各面板顶部工具栏的上 padding 承担，避免间隔叠加 */
  padding: 0.625rem 0.875rem 0;
  flex-shrink: 0;
}

.sidebar-header .icon-btn {
  margin-left: auto;
  font-size: 0.875rem;
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

.sidebar-tab {
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
}

.sidebar-tab:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.sidebar-tab.active {
  color: var(--accent);
  background: var(--accent-subtle);
}

</style>
