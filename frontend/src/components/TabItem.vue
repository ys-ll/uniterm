<template>
  <div
    class="tab-item"
    :class="{ active: isActive, 'ai-locked': isAILocked }"
    :data-tab-id="tab.id"
    @click="$emit('activate', tab.id)"
    @mouseenter="hovered = true"
    @mouseleave="hovered = false"
    draggable="true"
    @dragstart="onDragStart"
    @contextmenu="onContextMenu"
  >
    <button
      v-if="hovered && !tab.locked"
      class="tab-close"
      @click.stop="$emit('close', tab.id)"
    ><X /></button>
    <span
      v-else
      class="tab-icon-wrapper"
    >
      <component
        :is="tab.locked ? Lock : tabIcon"
        class="tab-type-icon"
      />
      <span
        v-if="isOutputLogOn"
        class="tab-log-dot"
        :title="t('session.recording', { path: outputLogPath })"
      />
      <span v-else-if="!isActive && hasNotification && !tab.locked" class="tab-notification-dot" />
    </span>
    <button
      v-if="tab.type === 'terminal'"
      class="tab-ai-lock"
      :class="{ locked: isAILocked }"
      @click.stop="$emit('toggleAiLock', tab.panelId)"
      :title="isAILocked ? t('terminal.aiLocked') : t('terminal.lockAI')"
    >
      <Sparkles :size="14" />
    </button>
    <span
      v-else-if="tab.type === 'workspace' && isAILocked"
      class="tab-ai-lock locked"
      title="AI locked to a panel in this workspace"
    >
      <Sparkles :size="14" />
    </span>
    <span v-if="!editing" class="tab-name" :class="{ 'tab-disconnected': isDisconnected }" @dblclick.stop="startEdit">
      <ArrowDownUp v-if="hasActiveTransfers" class="transfer-indicator" :size="14" title="Transferring..." />
      {{ tab.name }}
    </span>
    <input
      v-else
      ref="editInputRef"
      v-model="editName"
      class="tab-name-input"
      @keydown.enter="confirmEdit"
      @keydown.escape="cancelEdit"
      @blur="confirmEdit"
      @click.stop
    />
    <button
      class="tab-more"
      @click.stop="onMoreClick"
      :title="t('terminal.more')"
    >
      <MoreHorizontal :size="14" />
    </button>

    <Teleport to="body">
      <div
        v-show="contextMenuVisible"
        ref="menuRef"
        class="tab-context-menu"
        :style="contextMenuStyle"
        @click.stop
      >
        <div v-if="tab.type === 'terminal'" class="menu-item" @click="duplicateTab">{{ t('tab.duplicate') }}</div>
        <div v-if="tab.type === 'terminal' && panelStore.getPanel(tab.panelId)?.type === 'ssh'" class="menu-item" @click="openSftp">{{ t('sidebar.connectSftp') }}</div>
        <div v-if="tab.type === 'terminal' && panelStore.getPanel(tab.panelId)?.type === 'ssh'" class="menu-item" @click="uploadFileRz">{{ t('terminal.uploadFileRz') }}</div>
        <div v-if="tab.type === 'terminal' && panelStore.getPanel(tab.panelId)?.type === 'ssh'" class="menu-item" @click="openMonitor">{{ t('sidebar.connectMonitor') }}</div>
        <div v-if="supportsOutputLog" class="menu-item" @click="toggleOutputLog">
          {{ isOutputLogOn ? t('session.stopLog') : t('session.startLog') }}
        </div>
        <div v-if="supportsOutputLog && isOutputLogOn" class="menu-item" @click="openLogDir">
          {{ t('session.openLogDir') }}
        </div>
        <div v-if="tab.type === 'terminal'" class="menu-item" @click="triggerSearch">{{ t('terminal.searchText') }}</div>
        <div v-if="tab.type === 'terminal'" class="menu-item" @click="triggerExport">{{ t('terminal.export') }}</div>
        <div v-if="tab.type === 'terminal'" class="menu-item" @click="startEdit">{{ t('tab.rename') }}</div>
        <div v-if="tab.type === 'terminal'" class="menu-divider" />
        <div v-if="tab.type !== 'start' && tab.type !== 'settings'" class="menu-item" @click="toggleLock">
          {{ tab.locked ? t('tab.unlock') : t('tab.lock') }}
        </div>
        <div class="menu-item" :class="{ 'menu-item-disabled': tab.locked }" @click="tab.locked ? null : closeTab()">{{ t('tab.close') }}</div>
        <div class="menu-item" @click="closeOther">{{ t('tab.closeOther') }}</div>
        <div class="menu-item" @click="closeRight">{{ t('tab.closeRight') }}</div>
        <div class="menu-item" @click="closeLeft">{{ t('tab.closeLeft') }}</div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useI18n } from '../i18n'
import {
  CreateSession,
  EnableSessionOutputLog,
  DisableSessionOutputLog,
  GetSessionOutputLogInfo,
  OpenPathInExplorer,
} from '../../wailsjs/go/main/App'
import { msg } from '../services/message'
import type { TerminalTab, SettingsTab, SFTPTab, RDPTab, VNCTab, SPICETab, DBTab, MonitorTab, WorkspaceTab } from '../types/workspace'
import { SquareTerminal, Laptop, FolderUp, HardDrive, Cloud, Globe, Monitor, MonitorCloud, MonitorSmartphone, Settings, Sparkles, Database, DatabaseZap, Layers, Activity, Terminal, Zap, X, ArrowDownUp, LayoutDashboard, Cable, SquarePlus, Lock, MoreHorizontal } from '@lucide/vue'

const props = defineProps<{
  tab: TerminalTab | SettingsTab | SFTPTab | RDPTab | VNCTab | SPICETab | DBTab | MonitorTab | WorkspaceTab
  isActive: boolean
  hasNotification?: boolean
  showClose?: boolean
}>()

const emit = defineEmits<{
  activate: [id: string]
  close: [id: string]
  toggleAiLock: [panelId: string]
}>()

const tabStore = useTabStore()
const panelStore = usePanelStore()
const sessionStore = useSessionStore()
const { t } = useI18n()

const hovered = ref(false)
const contextMenuVisible = ref(false)
const contextMenuStyle = ref({ left: '0px', top: '0px' })

const editing = ref(false)
const editName = ref('')
const editInputRef = ref<HTMLInputElement>()

const tabIcon = computed(() => {
  const t = props.tab
  if (t.type === 'settings') return Settings
  if (t.type === 'sftp') {
    const panel = panelStore.getPanel(t.panelId)
    if (panel?.config?.type === 'smb') return HardDrive
    if (panel?.config?.type === 's3') return Cloud
    if (panel?.config?.type === 'webdav') return Globe
    return FolderUp
  }
  if (t.type === 'rdp') return Monitor
  if (t.type === 'vnc') return MonitorSmartphone
  if (t.type === 'spice') return MonitorCloud
  if (t.type === 'database' || t.type === 'mongodb') {
    const panel = panelStore.getPanel(t.panelId)
    if (panel?.config?.dbType === 'redis') return DatabaseZap
    if (panel?.config?.dbType === 'mongodb') return Layers
    return Database
  }
  if (t.type === 'monitor') return Activity
  if (t.type === 'workspace') return LayoutDashboard
  if (t.type === 'terminal') {
    const panel = panelStore.getPanel(t.panelId)
    if (panel?.type === 'local') return Laptop
    if (panel?.type === 'serial') return Cable
    if (panel?.type === 'telnet') return Terminal
    if (panel?.type === 'mosh') return Zap
    return SquareTerminal
  }
  if (t.type === 'start') return SquarePlus
  return null
})

const isAILocked = computed(() => {
  if (props.tab.type === 'workspace') {
    if (tabStore.aiLockedPanelIds.size === 0) return false
    return props.tab.panelIds.some(id => tabStore.isPanelAILocked(id))
  }
  if (props.tab.type !== 'terminal') return false
  return tabStore.isPanelAILocked(props.tab.panelId)
})


const hasActiveTransfers = computed(() => {
  if (props.tab.type === 'workspace') return false
  const tasks = panelStore.getTransferTasks(props.tab.panelId)
  return tasks.some(t => t.status === 'running' || t.status === 'paused')
})

const isDisconnected = computed(() => {
  if (props.tab.type === 'start' || props.tab.type === 'settings') return false
  const panelIds: string[] = props.tab.type === 'workspace' ? props.tab.panelIds : 'panelId' in props.tab ? [props.tab.panelId] : []
  if (panelIds.length === 0) return false
  return panelIds.every(pid => {
    const p = panelStore.getPanel(pid)
    if (!p?.sessionId) return true
    const s = sessionStore.getStatus(p.sessionId)
    return s === 'disconnected' || s === 'error'
  })
})

// Session output log state. Refreshed lazily when the right-click menu
// opens; also written after enable/disable so the REC badge stays in
// sync without an extra round-trip.
const isOutputLogOn = ref(false)
const outputLogPath = ref('')
const supportsOutputLog = computed(() => {
  if (props.tab.type !== 'terminal') return false
  const p = panelStore.getPanel((props.tab as TerminalTab).panelId)
  return !!p && ['ssh', 'telnet', 'serial', 'mosh', 'local'].includes(p.type)
})

function onDragStart(e: DragEvent) {
  e.dataTransfer?.setData('application/tab-id', props.tab.id)
  e.dataTransfer?.setData('application/tab-type', props.tab.type)
  if (props.isActive) {
    e.dataTransfer?.setData('application/is-active-tab', '1')
  }
  e.dataTransfer!.effectAllowed = 'move'

  // If dragging the active terminal tab, switch to adjacent tab first
  // so the dragged tab becomes "background" and can be merged into it
  if (props.isActive && props.tab.type === 'terminal') {
    const tabs = tabStore.tabs
    const fromIdx = tabs.findIndex(t => t.id === props.tab.id)
    const adjacentTab = tabs[fromIdx - 1] || tabs[fromIdx + 1]
    if (adjacentTab) {
      tabStore.setActiveTab(adjacentTab.id)
    }
  }
}

function onContextMenu(e: MouseEvent) {
  e.preventDefault()
  e.stopPropagation()
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  contextMenuStyle.value = { left: e.clientX + 'px', top: e.clientY + 'px' }
  contextMenuVisible.value = true
  if (supportsOutputLog.value) {
    refreshOutputLogState()
  }
}

function onMoreClick(e: MouseEvent) {
  e.stopPropagation()
  const btn = e.currentTarget as HTMLElement
  const rect = btn.getBoundingClientRect()
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  contextMenuStyle.value = { left: rect.left + 'px', top: rect.bottom + 4 + 'px' }
  contextMenuVisible.value = true
  if (supportsOutputLog.value) {
    refreshOutputLogState()
  }
}

async function refreshOutputLogState() {
  const panel = panelStore.getPanel((props.tab as TerminalTab).panelId)
  if (!panel) {
    isOutputLogOn.value = false
    outputLogPath.value = ''
    return
  }
  try {
    const info = await GetSessionOutputLogInfo(panel.id)
    isOutputLogOn.value = !!info.enabled
    outputLogPath.value = info.path || ''
    panelStore.setOutputLog(panel.id, { enabled: isOutputLogOn.value, path: outputLogPath.value })
  } catch {
    isOutputLogOn.value = false
    outputLogPath.value = ''
  }
}

function closeContextMenu() {
  contextMenuVisible.value = false
}

watch(contextMenuVisible, (val) => {
  window.dispatchEvent(new CustomEvent(val ? 'rdp:overlay-push' : 'rdp:overlay-pop'))
})

function startEdit() {
  closeContextMenu()
  editName.value = props.tab.name
  editing.value = true
  nextTick(() => {
    editInputRef.value?.focus()
    editInputRef.value?.select()
  })
}

function confirmEdit() {
  if (!editing.value) return
  editing.value = false
  const newName = editName.value.trim()
  if (newName && newName !== props.tab.name) {
    tabStore.renameTab(props.tab.id, newName)
  }
}

function cancelEdit() {
  editing.value = false
}

function toggleLock() {
  tabStore.toggleTabLock(props.tab.id)
  closeContextMenu()
}

function closeTab() {
  emit('close', props.tab.id)
  closeContextMenu()
}

function closeOther() {
  const allTabs = tabStore.tabs
  const currentIdx = allTabs.findIndex(t => t.id === props.tab.id)
  const others = allTabs.filter((t, i) => i !== currentIdx && !t.locked)
  others.forEach(t => emit('close', t.id))
  closeContextMenu()
}

function closeRight() {
  const allTabs = tabStore.tabs
  const currentIdx = allTabs.findIndex(t => t.id === props.tab.id)
  allTabs.slice(currentIdx + 1).filter(t => !t.locked).forEach(t => emit('close', t.id))
  closeContextMenu()
}

function closeLeft() {
  const allTabs = tabStore.tabs
  const currentIdx = allTabs.findIndex(t => t.id === props.tab.id)
  allTabs.slice(0, currentIdx).filter(t => !t.locked).forEach(t => emit('close', t.id))
  closeContextMenu()
}

async function duplicateTab() {
  const panel = panelStore.getPanel((props.tab as TerminalTab).panelId)
  if (!panel) return
  const newPanel = panelStore.createPanel(panel.config, panel.type)
  panelStore.updateTitle(newPanel.id, panel.title)
  if (panel.config) {
    try {
      const info = await CreateSession(panel.config.type, panel.config)
      panelStore.bindSession(newPanel.id, info.id)
    } catch (e) {
      console.error('Failed to duplicate session:', e)
    }
  }
  const newTab = tabStore.createTerminalTab(newPanel.title, newPanel.id)
  panelStore.movePanelToTab(newPanel.id, newTab.id)
  closeContextMenu()
}

function openSftp() {
  const panel = panelStore.getPanel((props.tab as TerminalTab).panelId)
  if (panel) {
    window.dispatchEvent(new CustomEvent('app:connect-sftp', { detail: panel }))
  }
  closeContextMenu()
}

function uploadFileRz() {
  window.dispatchEvent(new CustomEvent('terminal:send-rz', { detail: { panelId: (props.tab as TerminalTab).panelId } }))
  closeContextMenu()
}

function openMonitor() {
  const panel = panelStore.getPanel((props.tab as TerminalTab).panelId)
  if (panel) {
    window.dispatchEvent(new CustomEvent('app:connect-monitor', { detail: panel }))
  }
  closeContextMenu()
}

async function toggleOutputLog() {
  closeContextMenu()
  const panel = panelStore.getPanel((props.tab as TerminalTab).panelId)
  if (!panel) return
  try {
    if (isOutputLogOn.value) {
      await DisableSessionOutputLog(panel.id)
      isOutputLogOn.value = false
      const prev = outputLogPath.value
      outputLogPath.value = ''
      panelStore.setOutputLog(panel.id, { enabled: false, path: '' })
      msg.info(t('session.logStopped', { path: prev }))
      return
    }
    const path = await EnableSessionOutputLog(panel.id, '')
    if (!path) {
      msg.error(t('session.logFailed', { error: 'unknown' }))
      return
    }
    isOutputLogOn.value = true
    outputLogPath.value = path
    panelStore.setOutputLog(panel.id, { enabled: true, path })
    msg.success(t('session.logStarted', { path }))
  } catch (e: any) {
    msg.error(t('session.logFailed', { error: String(e?.message ?? e) }))
  }
}

async function openLogDir() {
  closeContextMenu()
  if (!outputLogPath.value) return
  try {
    await OpenPathInExplorer(outputLogPath.value)
  } catch (e: any) {
    msg.error(String(e?.message ?? e))
  }
}

function triggerSearch() {
  window.dispatchEvent(new CustomEvent('terminal:open-search', { detail: { panelId: (props.tab as TerminalTab).panelId } }))
  closeContextMenu()
}

function triggerExport() {
  window.dispatchEvent(new CustomEvent('terminal:export', { detail: { panelId: (props.tab as TerminalTab).panelId } }))
  closeContextMenu()
}

onMounted(async () => {
  window.addEventListener('global:close-context-menus', closeContextMenu)
  document.addEventListener('click', closeContextMenu)
  if (supportsOutputLog.value) {
    await refreshOutputLogState()
  }
})

onUnmounted(() => {
  window.removeEventListener('global:close-context-menus', closeContextMenu)
  document.removeEventListener('click', closeContextMenu)
})
</script>

<style scoped>
.tab-item {
  display: flex;
  align-items: center;
  gap: 2px;
  height: 28px;
  min-width: 120px;
  padding: 0 12px;
  margin: 0 1px;
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  position: relative;
  color: var(--text-secondary);
  font-size: 12px;
  transition: background 0.15s ease, color 0.15s ease;
  flex-shrink: 0;
  --wails-draggable: no-drag;
}
.tab-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.tab-item.active {
  background: var(--bg-hover);
  color: var(--text-primary);
  box-shadow: inset 0 0 0 1px var(--accent);
}
.tab-item.ai-locked {
  box-shadow: inset 2px 0 0 var(--warning);
}
.tab-item.active.ai-locked {
  background: var(--bg-hover);
  color: var(--text-primary);
  box-shadow: inset 0 0 0 1px var(--accent), inset 2px 0 0 var(--warning);
}
.tab-name {
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: flex;
  align-items: center;
  gap: 6px;
  margin-right: 4px;
  font-weight: 500;
}
.tab-disconnected {
  opacity: 0.5;
}
.tab-icon-wrapper {
  position: relative;
  display: inline-flex;
  flex-shrink: 0;
  margin-right: 4px;
}
.tab-type-icon {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  color: var(--text-muted);
}
.tab-notification-dot {
  position: absolute;
  top: -2px;
  right: -4px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 1px var(--bg-base);
}
.tab-log-dot {
  position: absolute;
  right: -2px;
  bottom: -2px;
  width: 6px;
  height: 6px;
  background: #e5484d;
  border-radius: 50%;
  border: 1px solid var(--bg-primary, #fff);
  pointer-events: auto;
}
.tab-item.active .tab-type-icon {
  color: var(--accent);
}
.transfer-indicator {
  color: var(--accent);
  flex-shrink: 0;
  line-height: 1;
}
.tab-name-input {
  font-size: 12px;
  font-family: inherit;
  color: var(--text-primary);
  background: var(--bg-base);
  border: 1px solid var(--accent);
  border-radius: var(--radius-sm);
  padding: 2px 6px;
  width: 120px;
  outline: none;
}
.tab-ai-lock {
  position: absolute;
  left: 30px;
  top: 50%;
  transform: translateY(-50%);
  background: var(--bg-hover);
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  width: 18px;
  height: 18px;
  padding: 0;
  border-radius: 3px;
  display: none;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.tab-item:hover .tab-ai-lock {
  display: inline-flex;
}
.tab-ai-lock.locked {
  position: static;
  transform: none;
  display: inline-flex;
  background: transparent;
  color: var(--warning);
  margin-right: 2px;
}
.tab-ai-lock:hover {
  color: var(--text-primary);
}
.tab-ai-lock.locked:hover {
  color: var(--warning);
}
.tab-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  margin-right: 4px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  font-size: 14px;
  transition: all 0.12s ease;
}
.tab-close:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.tab-more {
  position: absolute;
  right: 6px;
  top: 50%;
  transform: translateY(-50%);
  display: none;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  background: var(--bg-hover);
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  flex-shrink: 0;
}
.tab-item:hover .tab-more {
  display: inline-flex;
}
.tab-more:hover {
  color: var(--text-primary);
}
</style>

<style>
.tab-context-menu {
  position: fixed;
  z-index: 99999;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
  min-width: 180px;
  padding: 4px;
  backdrop-filter: blur(8px);
}
.tab-context-menu .menu-item {
  padding: 7px 14px;
  font-size: 12px;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  transition: all 0.1s ease;
}
.tab-context-menu .menu-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.tab-context-menu .menu-item-disabled {
  opacity: 0.4;
  pointer-events: none;
}
.tab-context-menu .menu-item-icon {
  margin-right: 6px;
  vertical-align: middle;
}
.tab-context-menu .menu-divider {
  height: 1px;
  background: var(--border-subtle);
  margin: 4px 6px;
}
</style>
