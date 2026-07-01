<template>
  <div
    class="tab-item"
    :class="{ active: isActive, 'ai-locked': isAILocked }"
    :data-tab-id="tab.id"
    @click="$emit('activate', tab.id)"
    draggable="true"
    @dragstart="onDragStart"
    @contextmenu="onContextMenu"
  >
    <button
      v-if="isActive"
      class="tab-close"
      @click.stop="$emit('close', tab.id)"
    ><X /></button>
    <span
      v-else
      class="tab-icon-wrapper"
    >
      <component
        :is="tabIcon"
        class="tab-type-icon"
      />
      <span v-if="!isActive && hasNotification" class="tab-notification-dot" />
    </span>
    <span v-if="!editing" class="tab-name" @dblclick.stop="startEdit">
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
        <div v-if="tab.type === 'terminal'" class="menu-item" @click="triggerSearch">{{ t('terminal.searchText') }}</div>
        <div v-if="tab.type === 'terminal'" class="menu-item" @click="startEdit">{{ t('tab.rename') }}</div>
        <div v-if="tab.type === 'terminal'" class="menu-divider" />
        <div class="menu-item" @click="closeTab">{{ t('tab.close') }}</div>
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
import { useI18n } from '../i18n'
import { CreateSession } from '../../wailsjs/go/main/App'
import type { TerminalTab, SettingsTab, SFTPTab, RDPTab, VNCTab, SPICETab, DBTab, MonitorTab, WorkspaceTab } from '../types/workspace'
import { SquareTerminal, Laptop, FolderUp, Monitor, MonitorCloud, Settings, Sparkles, Database, Activity, X, ArrowDownUp, LayoutDashboard, Cable, SquarePlus } from '@lucide/vue'

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
const { t } = useI18n()

const contextMenuVisible = ref(false)
const contextMenuStyle = ref({ left: '0px', top: '0px' })

const editing = ref(false)
const editName = ref('')
const editInputRef = ref<HTMLInputElement>()

const tabIcon = computed(() => {
  const t = props.tab
  if (t.type === 'settings') return Settings
  if (t.type === 'sftp') return FolderUp
  if (t.type === 'rdp') return Monitor
  if (t.type === 'vnc') return MonitorCloud
  if (t.type === 'spice') return MonitorCloud
  if (t.type === 'database') return Database
  if (t.type === 'monitor') return Activity
  if (t.type === 'workspace') return LayoutDashboard
  if (t.type === 'terminal') {
    const panel = panelStore.getPanel(t.panelId)
    if (panel?.type === 'local') return Laptop
    if (panel?.type === 'serial') return Cable
    return SquareTerminal
  }
  if (t.type === 'start') return SquarePlus
  return null
})

const isAILocked = computed(() => {
  if (props.tab.type === 'workspace') {
    if (!tabStore.aiLockedPanelId) return false
    return props.tab.panelIds.includes(tabStore.aiLockedPanelId)
  }
  if (props.tab.type !== 'terminal') return false
  return tabStore.aiLockedPanelId === props.tab.panelId
})


const hasActiveTransfers = computed(() => {
  if (props.tab.type === 'workspace') return false
  const tasks = panelStore.getTransferTasks(props.tab.panelId)
  return tasks.some(t => t.status === 'running' || t.status === 'paused')
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

function closeTab() {
  emit('close', props.tab.id)
  closeContextMenu()
}

function closeOther() {
  const allTabs = tabStore.tabs
  const currentIdx = allTabs.findIndex(t => t.id === props.tab.id)
  const others = allTabs.filter((_, i) => i !== currentIdx)
  others.forEach(t => emit('close', t.id))
  closeContextMenu()
}

function closeRight() {
  const allTabs = tabStore.tabs
  const currentIdx = allTabs.findIndex(t => t.id === props.tab.id)
  allTabs.slice(currentIdx + 1).forEach(t => emit('close', t.id))
  closeContextMenu()
}

function closeLeft() {
  const allTabs = tabStore.tabs
  const currentIdx = allTabs.findIndex(t => t.id === props.tab.id)
  allTabs.slice(0, currentIdx).forEach(t => emit('close', t.id))
  closeContextMenu()
}

async function duplicateTab() {
  const panel = panelStore.getPanel((props.tab as TerminalTab).panelId)
  if (!panel) return
  const newPanel = panelStore.createPanel(panel.config, panel.type)
  newPanel.title = panel.title
  if (panel.config) {
    try {
      const info = await CreateSession(panel.config.type, panel.config)
      panelStore.bindSession(newPanel.id, info.id)
    } catch (e) {
      console.error('Failed to duplicate session:', e)
    }
  }
  const newTab = tabStore.createTerminalTab(panel.title, newPanel.id)
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

function triggerSearch() {
  window.dispatchEvent(new CustomEvent('terminal:open-search', { detail: { panelId: (props.tab as TerminalTab).panelId } }))
  closeContextMenu()
}

onMounted(() => {
  window.addEventListener('global:close-context-menus', closeContextMenu)
  document.addEventListener('click', closeContextMenu)
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
  padding: 0 12px;
  margin: 0 1px;
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  position: relative;
  color: var(--text-secondary);
  font-size: 12px;
  transition: all 0.15s ease;
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
  box-shadow: inset 0 0 0 1px var(--accent-dim);
}
.tab-item.ai-locked {
  box-shadow: inset 2px 0 0 var(--warning, #f59e0b);
}
.tab-item.active.ai-locked {
  background: var(--bg-hover);
  color: var(--text-primary);
  box-shadow: inset 0 0 0 1px var(--accent-dim), inset 2px 0 0 var(--warning, #f59e0b);
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
  border: 1px solid var(--accent-dim);
  border-radius: var(--radius-sm);
  padding: 2px 6px;
  width: 120px;
  outline: none;
}
.tab-ai-lock {
  background: none;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  width: 18px;
  height: 18px;
  padding: 0;
  border-radius: 3px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.tab-ai-lock .ai-lock-icon {
  display: block;
}
.tab-ai-lock:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}
.tab-ai-lock.locked {
  color: var(--warning, #f59e0b);
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
.tab-context-menu .menu-divider {
  height: 1px;
  background: var(--border-subtle);
  margin: 4px 6px;
}
</style>
