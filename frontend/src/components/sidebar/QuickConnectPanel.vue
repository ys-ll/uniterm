<template>
  <div class="search-box">
    <el-input
      ref="searchInputRef"
      v-model="searchQuery"
      :placeholder="t('sidebar.searchPlaceholder')"
      clearable
      @keydown="onListKeydown"
    >
      <template #suffix>
        <span class="filter-trigger" :class="{ active: selectedTypeFilter !== 'all' }" @click.stop="filterMenuRef?.toggle($event.currentTarget)">
          <el-icon><Filter :size="'0.875rem'" /></el-icon>
        </span>
        <TypeFilterMenu ref="filterMenuRef" align="end" v-model="selectedTypeFilter" />
      </template>
    </el-input>
    <button class="sb-icon-btn" :title="t('header.newConnection')" @click.stop="newConnMenuRef?.toggle($event.currentTarget as HTMLElement)">
      <Plus :size="'0.9375rem'" />
    </button>
    <!-- New-connection menu — Menu.vue, teleported + anchored right-edge (align=end). -->
    <Menu ref="newConnMenuRef" align="end" v-model:visible="showNewConnMenu">
      <MenuItem @click="emit('new-conn-select', 'new-connection')">{{ t('header.newConnection') }}</MenuItem>
      <MenuItem @click="emit('new-conn-select', 'new-workspace')">{{ t('workspace.newWorkspace') }}</MenuItem>
      <MenuItem @click="emit('new-conn-select', 'new-group')">{{ t('conn.newGroupTitle') }}</MenuItem>
      <MenuDivider />
      <MenuItem @click="emit('new-conn-select', 'netscan')">{{ t('netscan.title') }}</MenuItem>
      <MenuDivider />
      <MenuItem @click="emit('new-conn-select', 'import')">{{ t('importExport.import') }}</MenuItem>
      <MenuItem @click="emit('new-conn-select', 'export')">{{ t('importExport.export') }}</MenuItem>
    </Menu>
  </div>

  <div class="connection-list" tabindex="0" @keydown="onListKeydown" @contextmenu.prevent="emit('empty-area-contextmenu', $event)">
    <!-- Virtual favorites group — persisted in favorites.json, rendered above all groups -->
    <ConnectionGroup
      v-if="favoriteConns.length > 0"
      variant="favorites"
      :label="t('sidebar.favorites')"
      :count="favoriteConns.length"
      :expanded="favoritesExpanded"
      :conns="favoriteConns"
      :selected-ids="selectedIds"
      :open-panel-conn-ids="openPanelConnIds"
      :drop-indicator="dropIndicator"
      :drag-over-group-id="dragOverGroupId"
      :show-host-row-buttons="showHostRowButtons"
      :is-favorite="isFavorite"
      :conn-icon="connIcon"
      :get-subtitle="getSubtitle"
      @toggle="favoritesExpanded = !favoritesExpanded"
      @item-dragstart="onDragStart"
      @item-dragend="onDragEnd"
      @item-dragover="onFavDragOver"
      @item-drop="onFavDrop"
      @item-click="onItemClick"
      @item-dblclick="onItemDblClick"
      @item-contextmenu="onContextMenu"
      @item-more="onConnMoreClick"
      @item-toggle-favorite="onToggleFavorite"
    />

    <!-- Nested group tree -->
    <GroupTreeItem
      v-for="root in filteredGrouped.roots"
      :key="root.group.id"
      :node="root"
      :depth="0"
    />

    <!-- Virtual (No Group) group - only when real groups exist -->
    <ConnectionGroup
      v-if="connectionStore.groups.length > 0 && filteredGrouped.ungrouped.length > 0"
      variant="ungrouped"
      :label="t('conn.noGroup')"
      :count="filteredGrouped.ungrouped.length"
      :expanded="expandedGroups.has('__ungrouped__')"
      :conns="filteredGrouped.ungrouped"
      :selected-ids="selectedIds"
      :open-panel-conn-ids="openPanelConnIds"
      :drop-indicator="dropIndicator"
      :drag-over-group-id="dragOverGroupId"
      :show-host-row-buttons="showHostRowButtons"
      :is-favorite="isFavorite"
      :conn-icon="connIcon"
      :get-subtitle="getSubtitle"
      @toggle="toggleGroup('__ungrouped__')"
      @group-contextmenu="emit('virtual-group-contextmenu', $event)"
      @group-dragover="onGroupDragOver"
      @group-dragleave="onGroupDragLeave"
      @group-drop="onGroupDrop"
      @item-dragstart="onDragStart"
      @item-dragend="onDragEnd"
      @item-dragover="onConnDragOver"
      @item-drop="onConnDrop"
      @item-click="onItemClick"
      @item-dblclick="onItemDblClick"
      @item-contextmenu="onContextMenu"
      @item-more="onConnMoreClick"
      @item-toggle-favorite="onToggleFavorite"
    />

    <!-- Flat ungrouped connections (only when no real groups exist) -->
    <template v-if="connectionStore.groups.length === 0">
      <ConnectionItem
        v-for="conn in filteredGrouped.ungrouped"
        :key="conn.id"
        :conn="conn"
        :icon="connIcon(conn)"
        :subtitle="getSubtitle(conn)"
        :is-selected="selectedIds.has(conn.id)"
        :has-session="openPanelConnIds.has(conn.id)"
        :drop-before="dropIndicator?.id === conn.id && dropIndicator?.position === 'before'"
        :drop-after="dropIndicator?.id === conn.id && dropIndicator?.position === 'after'"
        :show-host-row-buttons="showHostRowButtons"
        favorite-mode="toggle"
        :is-favorite="isFavorite(conn.id)"
        @dragstart="onDragStart"
        @dragend="onDragEnd"
        @dragover="onConnDragOver"
        @drop="onConnDrop"
        @click="onItemClick"
        @dblclick="onItemDblClick"
        @contextmenu="onContextMenu"
        @more="onConnMoreClick"
        @toggle-favorite="onToggleFavorite"
      />
    </template>

    <!-- Virtual "New Connection..." when searching -->
    <div
      v-if="searchQuery.trim()"
      class="connection-item virtual-new-conn"
      :class="{ active: focusedId === '__new_connection__' }"
      @click="focusedId = '__new_connection__'; selectedIds = new Set(); lastClickId = '__new_connection__'"
      @dblclick="emit('new-connection-from-search')"
    >
      <div class="conn-details">
        <span class="name virtual-name">{{ t('sidebar.newConnectionFromSearch') }}</span>
        <span class="host">{{ t('conn.host') }}: {{ searchQuery.trim() }}</span>
      </div>
    </div>

    <div v-if="totalFiltered === 0 && connectionStore.connections.length > 0 && !searchQuery.trim()" class="empty-state">
      {{ t('sidebar.noSearchResults') }}
    </div>
    <div v-if="connectionStore.connections.length === 0" class="empty-state">
      {{ t('sidebar.noConnections') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick } from 'vue'
import type { Component } from 'vue'
import { Filter, Plus } from '@lucide/vue'
import type { ConnectionConfig } from '../../types/session'
import { useConnectionStore } from '../../stores/connectionStore'
import { useFavoriteStore } from '../../stores/favoriteStore'
import { useI18n } from '../../i18n'
import { useConnectionTree } from '../../composables/useConnectionTree'
import Menu from '../Menu.vue'
import MenuItem from '../MenuItem.vue'
import MenuDivider from '../MenuDivider.vue'
import TypeFilterMenu from '../TypeFilterMenu.vue'
import GroupTreeItem from '../GroupTreeItem.vue'
import ConnectionItem from './ConnectionItem.vue'
import ConnectionGroup from './ConnectionGroup.vue'

type ConnectionTreeAPI = ReturnType<typeof useConnectionTree>


const props = defineProps<{
  tree: ConnectionTreeAPI
  openPanelConnIds: Set<string>
  showHostRowButtons: boolean
  isFavorite: (id: string) => boolean
  connIcon: (conn: ConnectionConfig) => Component
  getSubtitle: (conn: ConnectionConfig) => string
  /** Right-click handler for a connection row (context menu / more button). */
  onContextMenu: (e: MouseEvent, conn: ConnectionConfig) => void
  onConnMoreClick: (e: MouseEvent, conn: ConnectionConfig) => void
}>()

const emit = defineEmits<{
  (e: 'new-conn-select', cmd: string): void
  (e: 'empty-area-contextmenu', event: MouseEvent): void
  (e: 'virtual-group-contextmenu', event: MouseEvent): void
  (e: 'new-connection-from-search'): void
}>()

const { t } = useI18n()
const connectionStore = useConnectionStore()
const favoriteStore = useFavoriteStore()

// Destructure the tree API. The refs returned by useConnectionTree have stable
// references (created once in the parent's setup), so destructuring is safe.
const {
  searchQuery,
  selectedTypeFilter,
  filteredGrouped,
  totalFiltered,
  favoritesExpanded,
  favoriteConns,
  expandedGroups,
  selectedIds,
  focusedId,
  lastClickId,
  dragOverGroupId,
  dropIndicator,
  toggleGroup,
  onItemClick,
  onItemDblClick,
  onListKeydown,
  onDragStart,
  onDragEnd,
  onConnDragOver,
  onConnDrop,
  onGroupDragOver,
  onGroupDragLeave,
  onGroupDrop,
  onFavDragOver,
  onFavDrop,
} = props.tree

// Local UI state for the search box dropdowns.
const searchInputRef = ref<any>(null)
const filterMenuRef = ref<InstanceType<typeof TypeFilterMenu> | null>(null)
const showNewConnMenu = ref(false)
const newConnMenuRef = ref<InstanceType<typeof Menu> | null>(null)

function onToggleFavorite(conn: ConnectionConfig) {
  favoriteStore.toggle(conn.id)
}

/** Focus + select the search input text. Called by the parent sidebar shortcut. */
function focusSearch() {
  nextTick(() => {
    const el = searchInputRef.value?.$el?.querySelector('input')
    if (el instanceof HTMLInputElement) {
      el.focus()
      el.select()
    }
  })
}

defineExpose({ focusSearch })
</script>

<style scoped>
.search-box {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.625rem 0.625rem 0.375rem;
  flex-shrink: 0;
}

.sb-icon-btn {
  width: 1.625rem;
  height: 1.625rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  flex-shrink: 0;
}

.sb-icon-btn:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.connection-list {
  flex: 1;
  overflow-y: auto;
  padding: 0 0.5rem 0.5rem;
  outline: none;
}

.connection-item {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.5rem 0.625rem;
  border-radius: var(--radius-sm);
  transition: all 0.12s ease;
  margin-bottom: 0.125rem;
  user-select: none;
  position: relative;
}

.connection-item:hover {
  background: var(--bg-hover);
}

.connection-item.active {
  background: var(--accent-subtle);
  box-shadow: inset 0 0 0 1px var(--accent);
}

.connection-item.active .name {
  color: var(--accent);
}

.conn-details {
  display: flex;
  flex-direction: column;
  flex: 1;
  gap: 0.125rem;
  min-width: 0;
}

.name {
  font-family: var(--font-ui);
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.4;
}

.host {
  font-family: var(--font-ui);
  font-size: 0.6875rem;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.4;
}

.empty-state {
  padding: 2rem 1rem;
  text-align: center;
  font-size: 0.75rem;
  color: var(--text-disabled);
  font-family: var(--font-ui);
}

.virtual-new-conn .virtual-name {
  color: var(--accent);
}

.filter-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  transition: color 0.12s ease;
  padding: 0.125rem;
  border-radius: var(--radius-sm);
}

.filter-trigger:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.filter-trigger.active {
  color: var(--accent);
}

.filter-trigger.active:hover {
  color: var(--accent);
  background: var(--accent-subtle);
}
</style>