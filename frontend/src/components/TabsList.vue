<template>
  <div class="tabs-list" ref="tabsListRef" @wheel="onWheel" @dragover.prevent="onTabsContainerDragOver" @dragleave="onTabsDragLeave" @drop="onTabsContainerDrop">
    <template v-for="(tab, index) in tabs" :key="tab.id">
      <div
        v-if="(dragOverTabIndex === index && !dragOverInsertAfter) || (dragOverTabIndex === index - 1 && dragOverInsertAfter)"
        class="tab-drop-indicator"
      ></div>

      <TabItem
        :tab="tab"
        :is-active="tab.id === activeTabId"
        :has-notification="tabStore.hasTabNotification(tab.id)"
        @activate="setActiveTab"
        @close="(id: string) => $emit('close-tab', id)"
        @toggle-ai-lock="(panelId: string) => $emit('toggle-ai-lock', panelId)"
        @dragstart="(e: DragEvent, tabId: string) => $emit('tab-dragstart', e, tabId)"
        @dragover.prevent="(e: DragEvent) => onTabDragOver(e, index)"
        @dragleave="onTabDragLeave"
        @drop="(e: DragEvent) => onTabDrop(e, tab.id, index)"
      />
    </template>
    <div
      v-if="dragOverTabIndex === tabs.length - 1 && dragOverInsertAfter"
      class="tab-drop-indicator"
    ></div>
    <!-- Drop indicator at end when dragging over empty tabs area -->
    <div
      v-if="dragOverContainer"
      class="tab-drop-indicator tab-drop-indicator-end"
    ></div>
    <button
      class="tab-add-btn"
      :title="t('startTab.defaultName')"
      @click="onAddStartTab"
    >
      <el-icon><Plus :size="14" /></el-icon>
    </button>
  </div>
  <div class="tab-more" v-if="showMore">
    <el-dropdown trigger="click" @command="setActiveTab" @visible-change="onMoreDropdownVisibleChange">
      <span class="tab-more-btn" :title="t('tab.more')">
        <el-icon class="tab-more-icon"><MoreHorizontal :size="14" /></el-icon>
      </span>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item
            v-for="tab in tabs"
            :key="tab.id"
            :command="tab.id"
            :class="{ 'is-active': tab.id === activeTabId }"
          >
            {{ tab.name }}
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { MoreHorizontal, Plus } from '@lucide/vue'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useI18n } from '../i18n'
import TabItem from './TabItem.vue'

const tabStore = useTabStore()
const panelStore = usePanelStore()
const { t } = useI18n()
const tabs = computed(() => tabStore.tabs)
const activeTabId = computed(() => tabStore.activeTabId)

const dragOverTabIndex = ref<number | null>(null)
const dragOverInsertAfter = ref(false)
const dragOverContainer = ref(false)

const tabsListRef = ref<HTMLElement | null>(null)
const showMore = ref(false)

function updateOverflow() {
  const el = tabsListRef.value
  if (!el) return
  showMore.value = el.scrollWidth > el.clientWidth + 1
}

// Watch tab changes and window resize to update overflow state

watch(() => tabs.value.length, () => nextTick(updateOverflow))
watch(activeTabId, () => nextTick(updateOverflow))

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  resizeObserver = new ResizeObserver(updateOverflow)
  if (tabsListRef.value) {
    resizeObserver.observe(tabsListRef.value)
  }
  nextTick(updateOverflow)
  window.addEventListener('dragend', clearDragState)
})

onUnmounted(() => {
  resizeObserver?.disconnect()
  window.removeEventListener('dragend', clearDragState)
})

defineEmits<{
  'close-tab': [id: string]
  'toggle-ai-lock': [panelId: string]
  'tab-dragstart': [e: DragEvent, tabId: string]
}>()

function onAddStartTab() {
  tabStore.createStartTab()
}

function onWheel(e: WheelEvent) {
  if (!tabsListRef.value) return
  tabsListRef.value.scrollLeft += e.deltaY
}

function onMoreDropdownVisibleChange(visible: boolean) {
  if (visible) {
    window.dispatchEvent(new CustomEvent('rdp:overlay-push'))
  } else {
    window.dispatchEvent(new CustomEvent('rdp:overlay-pop'))
  }
}

function setActiveTab(id: string) {
  tabStore.setActiveTab(id)
  scrollToTab(id)
}

function scrollToTab(tabId: string) {
  if (!tabsListRef.value) return
  const el = tabsListRef.value.querySelector(`[data-tab-id="${tabId}"]`) as HTMLElement | null
  if (el) {
    el.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'nearest' })
  }
}

function onTabDragOver(e: DragEvent, index: number) {
  const hasPanel = e.dataTransfer?.types.includes('application/panel-id')
  const hasTab = e.dataTransfer?.types.includes('application/tab-id')
  if (!hasPanel && !hasTab) return

  dragOverContainer.value = false
  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  dragOverTabIndex.value = index
  dragOverInsertAfter.value = e.clientX >= rect.left + rect.width / 2
  e.dataTransfer!.dropEffect = 'move'
}

function onTabDragLeave(_e: DragEvent) {
  // Reset handled by onTabDragOver of adjacent tab
}

function onTabsDragLeave(e: DragEvent) {
  const el = e.currentTarget as HTMLElement
  const relatedTarget = e.relatedTarget as HTMLElement | null
  if (!relatedTarget || !el.contains(relatedTarget)) {
    clearDragState()
  }
}

function clearDragState() {
  dragOverTabIndex.value = null
  dragOverInsertAfter.value = false
  dragOverContainer.value = false
}

function onTabsContainerDragOver(e: DragEvent) {
  const hasPanel = e.dataTransfer?.types.includes('application/panel-id')
  const hasTab = e.dataTransfer?.types.includes('application/tab-id')
  if (!hasPanel && !hasTab) return

  // Ignore if hovering over a tab item (handled by onTabDragOver)
  const target = e.target as HTMLElement
  if (target.closest('.tab-item') || target.closest('.workspace-tab-item')) {
    return
  }

  dragOverContainer.value = true
  dragOverTabIndex.value = null
  dragOverInsertAfter.value = false
  e.dataTransfer!.dropEffect = 'move'
}

function onTabsContainerDrop(e: DragEvent) {
  e.stopPropagation()

  // Skip if drop landed on a tab item (already handled by onTabDrop)
  const target = e.target as HTMLElement
  if (target.closest('.tab-item') || target.closest('.workspace-tab-item')) {
    clearDragState()
    return
  }

  const draggedTabId = e.dataTransfer?.getData('application/tab-id')
  const panelId = e.dataTransfer?.getData('application/panel-id')
  const sourceTabId = e.dataTransfer?.getData('application/source-tab-id')

  clearDragState()

  // Case 1: Tab dragged to empty area → move to end
  if (draggedTabId && !panelId) {
    const fromIdx = tabs.value.findIndex(t => t.id === draggedTabId)
    if (fromIdx === -1) return
    const toIdx = tabs.value.length - 1
    if (fromIdx !== toIdx) {
      tabStore.moveTab(fromIdx, toIdx)
    }
    return
  }

  // Case 2: Panel dropped on empty area → create terminal tab at end
  if (panelId) {
    const panel = panelStore.getPanel(panelId)
    if (!panel) return

    if (sourceTabId) {
      tabStore.removePanelFromWorkspaceTab(sourceTabId, panelId)
    }

    const tab = tabStore.createTerminalTab(panel.title, panelId)
    panelStore.movePanelToTab(panelId, tab.id)
  }
}

function onTabDrop(e: DragEvent, targetTabId: string, index: number) {
  e.stopPropagation()

  const insertAfter = dragOverInsertAfter.value
  clearDragState()

  const draggedTabId = e.dataTransfer?.getData('application/tab-id')
  const draggedPanelId = e.dataTransfer?.getData('application/panel-id')
  const sourceTabId = e.dataTransfer?.getData('application/source-tab-id')

  if (draggedTabId && !draggedPanelId) {
    if (draggedTabId === targetTabId) return
    const fromIdx = tabs.value.findIndex(t => t.id === draggedTabId)
    if (fromIdx === -1) return
    let toIdx = index + (insertAfter ? 1 : 0)
    if (toIdx > fromIdx) toIdx--
    if (fromIdx !== toIdx) {
      tabStore.moveTab(fromIdx, toIdx)
    }
    return
  }

  if (draggedPanelId) {
    const panel = panelStore.getPanel(draggedPanelId)
    if (!panel) return

    if (sourceTabId) {
      tabStore.removePanelFromWorkspaceTab(sourceTabId, draggedPanelId)
    }

    const tab = tabStore.createTerminalTab(panel.title, draggedPanelId)
    panelStore.movePanelToTab(draggedPanelId, tab.id)

    const targetIdx = index + (insertAfter ? 1 : 0)
    const currentIdx = tabs.value.findIndex(t => t.id === tab.id)
    if (currentIdx !== targetIdx) {
      tabStore.moveTab(currentIdx, targetIdx)
    }
  }
}
</script>

<style scoped>
.tabs-list {
  display: flex;
  flex: 1;
  overflow-x: auto;
  overflow-y: hidden;
  align-items: center;
  scrollbar-width: none;
  min-width: 0;
}
.tabs-list::-webkit-scrollbar {
  display: none;
}

.tab-more {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  padding: 0 4px;
  height: 28px;
}
.tab-more-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-muted);
  letter-spacing: 1px;
  user-select: none;
  transition: all 0.15s;
  --wails-draggable: no-drag;
}
.tab-more-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.tab-drop-indicator {
  width: 2px;
  min-width: 2px;
  align-self: stretch;
  background: var(--accent);
  opacity: 0.8;
  margin: 4px 0;
  border-radius: 1px;
  flex-shrink: 0;
}
.tab-drop-indicator-end {
  margin-left: auto;
}

.tab-add-btn {
  flex-shrink: 0;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary, #888);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s, color 0.15s;
  margin-left: 2px;
}
.tab-add-btn:hover {
  background: var(--bg-hover, #3a3b44);
  color: var(--text-primary, #e0e0e0);
}
</style>
