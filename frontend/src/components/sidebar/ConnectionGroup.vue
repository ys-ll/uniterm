<template>
  <!-- Group header -->
  <div
    class="group-header"
    :class="{
      'drag-over': variant === 'ungrouped' && dragOverGroupId === '__ungrouped__',
    }"
    @click="emit('toggle')"
    @contextmenu.prevent="onHeaderContextMenu"
    @dragover.prevent="variant === 'ungrouped' ? emit('group-dragover', '__ungrouped__', $event) : undefined"
    @dragleave="variant === 'ungrouped' ? emit('group-dragleave', '__ungrouped__') : undefined"
    @drop.prevent="variant === 'ungrouped' ? emit('group-drop', '__ungrouped__', $event) : undefined"
  >
    <span class="group-arrow">
      <el-icon v-if="expanded"><ChevronDown :size="'0.875rem'" /></el-icon>
      <el-icon v-else><ChevronRight :size="'0.875rem'" /></el-icon>
    </span>
    <span class="group-name">{{ label }}</span>
    <span v-if="count > 0" class="group-count">{{ count }}</span>
  </div>

  <!-- Connection items (when expanded) -->
  <template v-if="expanded">
    <ConnectionItem
      v-for="conn in conns"
      :key="conn.id"
      :conn="conn"
      :icon="connIcon(conn)"
      :subtitle="getSubtitle(conn)"
      :is-selected="selectedIds.has(conn.id)"
      :has-session="openPanelConnIds.has(conn.id)"
      :drop-before="dropIndicator?.id === conn.id && dropIndicator?.position === 'before'"
      :drop-after="dropIndicator?.id === conn.id && dropIndicator?.position === 'after'"
      indented
      :show-host-row-buttons="showHostRowButtons"
      :favorite-mode="variant === 'favorites' ? 'lit' : 'toggle'"
      :is-favorite="variant === 'ungrouped' ? isFavorite(conn.id) : false"
      @dragstart="emit('item-dragstart', $event, conn)"
      @dragend="emit('item-dragend')"
      @dragover="emit('item-dragover', $event, conn)"
      @drop="emit('item-drop', $event, conn)"
      @click="emit('item-click', $event, conn)"
      @dblclick="emit('item-dblclick', conn)"
      @contextmenu="emit('item-contextmenu', $event, conn)"
      @more="emit('item-more', $event, conn)"
      @toggle-favorite="emit('item-toggle-favorite', conn)"
    />
  </template>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { ChevronDown, ChevronRight } from '@lucide/vue'
import type { ConnectionConfig } from '../../types/session'
import ConnectionItem from './ConnectionItem.vue'

type DropIndicator = { id: string; position: 'before' | 'after' | 'inside' } | null

const props = defineProps<{
  /** 'favorites' = virtual favorites group; 'ungrouped' = virtual No-Group group */
  variant: 'favorites' | 'ungrouped'
  label: string
  count: number
  expanded: boolean
  conns: ConnectionConfig[]
  selectedIds: Set<string>
  openPanelConnIds: Set<string>
  dropIndicator: DropIndicator
  dragOverGroupId: string | null
  showHostRowButtons: boolean
  isFavorite: (id: string) => boolean
  connIcon: (conn: ConnectionConfig) => Component
  getSubtitle: (conn: ConnectionConfig) => string
}>()

const emit = defineEmits<{
  (e: 'toggle'): void
  (e: 'group-contextmenu', event: MouseEvent): void
  (e: 'group-dragover', groupId: string, event: DragEvent): void
  (e: 'group-dragleave', groupId: string): void
  (e: 'group-drop', groupId: string, event: DragEvent): void
  (e: 'item-dragstart', event: DragEvent, conn: ConnectionConfig): void
  (e: 'item-dragend'): void
  (e: 'item-dragover', event: DragEvent, conn: ConnectionConfig): void
  (e: 'item-drop', event: DragEvent, conn: ConnectionConfig): void
  (e: 'item-click', event: MouseEvent, conn: ConnectionConfig): void
  (e: 'item-dblclick', conn: ConnectionConfig): void
  (e: 'item-contextmenu', event: MouseEvent, conn: ConnectionConfig): void
  (e: 'item-more', event: MouseEvent, conn: ConnectionConfig): void
  (e: 'item-toggle-favorite', conn: ConnectionConfig): void
}>()

function onHeaderContextMenu(e: MouseEvent) {
  // Favorites group header has no context menu (original code only did
  // `.prevent`); the virtual No-Group header forwards to the parent which
  // opens the group context menu.
  if (props.variant === 'ungrouped') emit('group-contextmenu', e)
}
</script>

<style scoped>
.group-header {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.375rem 0.625rem 0.375rem 0.375rem;
  user-select: none;
  border-radius: var(--radius-sm);
  transition: background 0.12s ease;
  font-family: var(--font-ui);
  font-size: 0.75rem;
  color: var(--text-secondary);
}

.group-header:hover {
  background: var(--bg-hover);
}

.group-arrow {
  display: inline-flex;
  align-items: center;
  width: 1rem;
  color: var(--text-disabled);
}

.group-name {
  font-weight: 600;
}

.group-count {
  margin-left: auto;
  font-size: 0.625rem;
  color: var(--text-disabled);
  background: var(--bg-subtle);
  padding: 0 0.3125rem;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}

.group-header.drag-over {
  background: var(--accent-subtle);
  box-shadow: inset 0 0 0 1px var(--accent);
}
</style>