<template>
  <div class="history-panel">
    <div class="qc-toolbar">
      <el-input
        v-model="searchQuery"
        :placeholder="t('settings.historySearchPlaceholder')"
        clearable
       
        class="qc-search-input"
        @keydown="onListKeydown"
      />
    </div>

    <div class="qc-list" ref="listRef" tabindex="0" @keydown="onListKeydown">
      <div
        v-for="entry in filteredEntries"
        :key="entry.id"
        class="qc-item"
        :class="{ active: selectedIds.has(entry.id) }"
        @click="onItemClick($event, entry)"
        @dblclick="runCommand(entry)"
        @contextmenu.prevent="onContextMenu($event, entry)"
        @mouseenter="hoveredId = entry.id; showTooltip($event, entry.command)"
        @mouseleave="hoveredId = null; hideTooltip()"
      >
        <span class="history-command">{{ entry.command }}</span>
        <div v-if="selectedIds.size <= 1 && (selectedIds.has(entry.id) || hoveredId === entry.id)" class="qc-item-actions">
          <button class="qc-action-btn run" @click.stop="runCommand(entry)" :title="t('quickCommands.run')">
            <Play :size="14" />
          </button>
          <button class="qc-action-btn paste" @click.stop="pasteCommand(entry)" :title="t('quickCommands.paste')">
            <Clipboard :size="14" />
          </button>
        </div>
      </div>

      <div v-if="filteredEntries.length === 0" class="qc-empty">
        {{ searchQuery ? t('sidebar.noSearchResults') : t('settings.historyEmpty') }}
      </div>
    </div>

    <!-- Context menu -->
    <div
      v-show="menuVisible"
      class="qc-context-menu"
      :style="menuStyle"
      @click.stop
    >
      <div class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && runCommand(menuTarget!)">{{ t('quickCommands.run') }}</div>
      <div class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && pasteCommand(menuTarget!)">{{ t('quickCommands.paste') }}</div>
      <div class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && saveAsQuickCommand(menuTarget!)">{{ t('quickCommands.saveAs') }}</div>
      <div class="menu-divider" />
      <div class="menu-item danger" @click="deleteSelected(); closeMenu()">{{ t('sidebar.delete') }}</div>
    </div>

    <!-- Custom tooltip -->
    <div v-show="tooltipVisible" class="qc-tooltip" :style="tooltipStyle">{{ tooltipText }}</div>

    <!-- Quick command edit dialog -->
    <QuickCommandEditDialog
      v-model="editDialogVisible"
      :initial-command="editingCmdCommand"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { Play, Clipboard } from '@lucide/vue'
import { useSuggestions, type HistoryEntry } from '../composables/useSuggestions'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useQuickCommandStore } from '../stores/quickCommandStore'
import { SessionWrite } from '../../wailsjs/go/main/App'
import { useI18n } from '../i18n'
import QuickCommandEditDialog from './QuickCommandEditDialog.vue'

const { t } = useI18n()
const suggestions = useSuggestions()
const tabStore = useTabStore()
const panelStore = usePanelStore()
const qcStore = useQuickCommandStore()

const searchQuery = ref('')
const selectedIds = ref<Set<string>>(new Set())
const focusedId = ref<string | null>(null)
const hoveredId = ref<string | null>(null)
const listRef = ref<HTMLDivElement | null>(null)
const lastClickId = ref<string | null>(null)

const menuVisible = ref(false)
const menuStyle = ref({ left: '0px', top: '0px' })
const menuTarget = ref<HistoryEntry | null>(null)

const editDialogVisible = ref(false)
const editingCmdCommand = ref('')

const tooltipVisible = ref(false)
const tooltipText = ref('')
const tooltipStyle = ref({ left: '0px', top: '0px' })

function showTooltip(e: MouseEvent, text: string) {
  tooltipText.value = text
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  tooltipStyle.value = { left: rect.left + 'px', top: (rect.top - 8) + 'px' }
  tooltipVisible.value = true
}

function hideTooltip() {
  tooltipVisible.value = false
}
const entries = ref<HistoryEntry[]>([])

onMounted(async () => {
  entries.value = await suggestions.loadHistory()
  document.addEventListener('click', closeMenu)
})

onUnmounted(() => {
  document.removeEventListener('click', closeMenu)
})

const filteredEntries = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return entries.value
  return entries.value.filter(e => e.command.toLowerCase().includes(q))
})

function getAllVisibleIds(): string[] {
  return filteredEntries.value.map(e => e.id)
}

function onListKeydown(e: KeyboardEvent) {
  const ids = getAllVisibleIds()
  if (ids.length === 0) return
  const idx = ids.indexOf(focusedId.value || '')

  if (e.key === 'ArrowDown') {
    e.preventDefault()
    const nextIdx = idx >= 0 && idx < ids.length - 1 ? idx + 1 : 0
    focusedId.value = ids[nextIdx]
    selectedIds.value = new Set([ids[nextIdx]])
    lastClickId.value = ids[nextIdx]
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    const prevIdx = idx > 0 ? idx - 1 : ids.length - 1
    focusedId.value = ids[prevIdx]
    selectedIds.value = new Set([ids[prevIdx]])
    lastClickId.value = ids[prevIdx]
  } else if (e.key === 'Enter') {
    e.preventDefault()
    if (selectedIds.value.size === 1 && focusedId.value) {
      const entry = entries.value.find(e => e.id === focusedId.value)
      if (entry) runCommand(entry)
    }
  } else if (e.key === 'Delete') {
    e.preventDefault()
    deleteSelected()
  }
}

function onItemClick(e: MouseEvent, entry: HistoryEntry) {
  if (e.shiftKey && lastClickId.value) {
    const ids = getAllVisibleIds()
    const anchorIdx = ids.indexOf(lastClickId.value)
    const currentIdx = ids.indexOf(entry.id)
    if (anchorIdx >= 0 && currentIdx >= 0) {
      const [start, end] = anchorIdx < currentIdx ? [anchorIdx, currentIdx] : [currentIdx, anchorIdx]
      selectedIds.value = new Set(ids.slice(start, end + 1))
    }
  } else if (e.ctrlKey || e.metaKey) {
    const next = new Set(selectedIds.value)
    if (next.has(entry.id)) next.delete(entry.id)
    else next.add(entry.id)
    selectedIds.value = next
    lastClickId.value = entry.id
    focusedId.value = entry.id
  } else {
    selectedIds.value = new Set([entry.id])
    lastClickId.value = entry.id
    focusedId.value = entry.id
  }
}

function getTargetSessionIds(): string[] {
  const activeTabId = tabStore.activeTabId
  if (!activeTabId) return []
  const tab = tabStore.tabs.find(t => t.id === activeTabId)
  if (!tab) return []
  if (tab.type === 'workspace' && tabStore.isBroadcasting(tab.id)) {
    const ids: string[] = []
    for (const pid of tab.panelIds) {
      const p = panelStore.getPanel(pid)
      if (p?.sessionId && (p.type === 'ssh' || p.type === 'local')) {
        ids.push(p.sessionId)
      }
    }
    return ids
  }
  const activePanelId = tab.type === 'workspace' ? tab.activePanelId : (tab.type === 'terminal' ? tab.panelId : null)
  if (!activePanelId) return []
  const panel = panelStore.getPanel(activePanelId)
  if (!panel?.sessionId) return []
  return [panel.sessionId]
}

function runCommand(entry: HistoryEntry) {
  const sids = getTargetSessionIds()
  if (sids.length === 0) return
  const text = entry.command.endsWith('\n') ? entry.command : entry.command + '\n'
  for (const sid of sids) {
    SessionWrite(sid, text)
  }
}

function pasteCommand(entry: HistoryEntry) {
  const sids = getTargetSessionIds()
  if (sids.length === 0) return
  for (const sid of sids) {
    SessionWrite(sid, entry.command)
  }
}

function saveAsQuickCommand(entry: HistoryEntry) {
  editingCmdCommand.value = entry.command
  editDialogVisible.value = true
  closeMenu()
}

function deleteEntries(ids: string[]) {
  suggestions.removeHistoryCommandsById(ids)
  entries.value = entries.value.filter(e => !ids.includes(e.id))
  selectedIds.value = new Set()
}

function deleteSelected() {
  const ids = [...selectedIds.value]
  if (ids.length === 0) return
  deleteEntries(ids)
}

function onContextMenu(e: MouseEvent, entry: HistoryEntry) {
  e.stopPropagation()
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  menuTarget.value = entry
  if (!selectedIds.value.has(entry.id)) {
    selectedIds.value = new Set([entry.id])
    focusedId.value = entry.id
  }
  menuStyle.value = clampMenuPosition(e.clientX, e.clientY)
  menuVisible.value = true
}

function closeMenu() { menuVisible.value = false }

function clampMenuPosition(x: number, y: number) {
  const mx = Math.min(x, window.innerWidth - 160)
  const my = Math.min(y, window.innerHeight - 100)
  return { left: mx + 'px', top: my + 'px' }
}

watch(searchQuery, () => {
  const ids = getAllVisibleIds()
  if (ids.length > 0) {
    focusedId.value = ids[0]
    selectedIds.value = new Set([ids[0]])
    lastClickId.value = ids[0]
  } else {
    focusedId.value = null
    selectedIds.value = new Set()
  }
})
</script>

<style scoped>
.history-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.qc-tooltip {
  position: fixed;
  z-index: 10000;
  max-width: 480px;
  padding: 6px 10px;
  font-family: var(--font-mono, 'Consolas', 'Courier New', monospace);
  font-size: 12px;
  color: var(--text-primary);
  background: var(--bg-overlay);
  border: 1px solid var(--border-color);
  border-radius: 4px;
  box-shadow: var(--shadow-md);
  pointer-events: none;
  white-space: pre-wrap;
  word-break: break-all;
  transform: translateY(-100%);
}

.history-command {
  flex: 1;
  font-family: var(--font-mono, 'Consolas', 'Courier New', monospace);
  font-size: 12px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.qc-toolbar {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 10px 6px;
  flex-shrink: 0;
}

.qc-search-input { flex: 1; min-width: 0; }

.qc-list {
  flex: 1;
  overflow-y: auto;
  padding: 0 8px 8px;
}

.qc-item {
  display: flex;
  align-items: center;
  padding: 6px 10px;
  gap: 10px;
  min-height: 36px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.12s ease;
  margin-bottom: 2px;
  user-select: none;
}

.qc-item:hover { background: var(--bg-hover); }

.qc-item.active {
  background: var(--accent-subtle);
  box-shadow: inset 0 0 0 1px var(--accent-dim);
}

.qc-item.active .history-command { color: var(--accent); }

.qc-item-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}

.qc-action-btn {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  color: var(--text-muted);
  background: transparent;
}

.qc-action-btn:hover { color: var(--text-primary); background: var(--bg-hover); }
.qc-action-btn.run:hover { color: var(--success-color, #22c55e); }
.qc-action-btn.paste:hover { color: var(--accent-color, #22d3ee); }

.qc-empty {
  padding: 24px 12px;
  text-align: center;
  color: var(--text-muted);
  font-size: 12px;
}

.qc-context-menu {
  position: fixed;
  z-index: 9999;
  background: var(--bg-surface);
  border: 1px solid var(--border-color);
  border-radius: 6px;
  box-shadow: var(--shadow-lg);
  padding: 4px;
  min-width: 140px;
}

.qc-context-menu .menu-item {
  padding: 6px 10px;
  font-size: 12px;
  border-radius: 4px;
  cursor: pointer;
  color: var(--text-primary);
}

.qc-context-menu .menu-item:hover { background: var(--bg-hover); }
.qc-context-menu .menu-item.danger { color: var(--danger-color, #f56c6c); }
.qc-context-menu .menu-item.disabled { color: var(--text-disabled); pointer-events: none; }

.qc-context-menu .menu-divider {
  height: 1px;
  background: var(--border-color);
  margin: 4px 6px;
}
</style>
