<template>
  <!-- Group header -->
  <div
    class="group-header"
    :class="{ 'drag-over': dragOverId === node.group.id }"
    :style="{ paddingLeft: (6 + depth * 16) + 'px' }"
    draggable="true"
    @click="onToggle"
    @contextmenu.prevent="onCtxMenu"
    @dragstart.stop="onGrpDragStart"
    @dragover.prevent="onGrpDragOver"
    @dragleave="onGrpDragLeave"
    @drop.prevent="onGrpDrop"
  >
    <span class="group-arrow">
      <el-icon v-if="expanded.has(node.group.id)"><ChevronDown :size="14" /></el-icon>
      <el-icon v-else><ChevronRight :size="14" /></el-icon>
    </span>
    <span class="group-name">{{ node.group.name }}</span>
    <span v-if="totalCount > 0" class="group-count">{{ totalCount }}</span>
  </div>

  <!-- Children (when expanded): child groups first, then connections -->
  <template v-if="expanded.has(node.group.id)">
    <!-- Child groups (recursive) -->
    <GroupTreeItem
      v-for="child in node.children"
      :key="child.group.id"
      :node="child"
      :depth="depth + 1"
    />

    <!-- Connections -->
    <div
      v-for="conn in node.connections"
      :key="conn.id"
      class="connection-item indented"
      :class="{ active: selected.has(conn.id) }"
      :style="{ paddingLeft: (24 + depth * 16) + 'px' }"
      draggable="true"
      @dragstart="onConnDragStart($event, conn)"
      @dragend="onConnDragEnd"
      @click="onItemClick($event, conn)"
      @dblclick="onItemDblClick(conn)"
      @contextmenu.prevent="onConnCtxMenu($event, conn)"
    >
      <span class="conn-icon"><component :is="connIcon(conn)" :size="14" /></span>
      <div class="conn-details">
        <span class="name">{{ conn.name }}</span>
        <span class="conn-meta">
          <span class="host">{{ getSubtitle(conn) }}</span>
        </span>
      </div>
      <button class="conn-more-btn" @click.stop="onMoreClick($event, conn)" :title="t('terminal.more')">
        <MoreHorizontal :size="14" />
      </button>
    </div>
  </template>
</template>

<script setup lang="ts">
import { inject, computed } from 'vue'
import { ChevronDown, ChevronRight, MoreHorizontal } from '@lucide/vue'
import type { GroupTreeNode, ConnectionConfig, ConnectionGroup } from '../types/session'

const props = defineProps<{
  node: GroupTreeNode
  depth: number
}>()

// Recursive count of all connections in this subtree
const totalCount = computed(() => {
  function count(node: GroupTreeNode): number {
    let n = node.connections.length
    for (const child of node.children) n += count(child)
    return n
  }
  return count(props.node)
})

// Injected from Sidebar
const expanded = inject<Set<string>>('expandedGroups')!
const selected = inject<Set<string>>('selectedIds')!
const dragOverId = inject<any>('dragOverGroupId')!
const handlers = inject<any>('groupHandlers')!
const utils = inject<any>('utils')!

const { connIcon, getSubtitle, t } = utils

function onToggle() {
  handlers.onToggleGroup(props.node.group.id)
}

function onCtxMenu(e: MouseEvent) {
  handlers.onGroupContextMenu(e, props.node.group)
}

function onGrpDragStart(e: DragEvent) {
  e.dataTransfer!.setData('text/plain', JSON.stringify({ type: 'group', id: props.node.group.id }))
  e.dataTransfer!.effectAllowed = 'move'
}

function onGrpDragOver() {
  handlers.onGroupDragOver(props.node.group.id)
}

function onGrpDragLeave() {
  handlers.onGroupDragLeave(props.node.group.id)
}

function onGrpDrop(e: DragEvent) {
  handlers.onGroupDrop(props.node.group.id, e)
}

function onConnDragStart(e: DragEvent, conn: ConnectionConfig) {
  handlers.onDragStart(e, conn)
}

function onConnDragEnd() {
  handlers.onDragEnd()
}

function onItemClick(e: MouseEvent, conn: ConnectionConfig) {
  handlers.onItemClick(e, conn)
}

function onItemDblClick(conn: ConnectionConfig) {
  handlers.onItemDblClick(conn)
}

function onConnCtxMenu(e: MouseEvent, conn: ConnectionConfig) {
  handlers.onContextMenu(e, conn)
}

function onMoreClick(e: MouseEvent, conn: ConnectionConfig) {
  handlers.onConnMoreClick(e, conn)
}
</script>

<style scoped>
.group-header {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 10px 6px 0;
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  transition: background 0.12s ease;
  font-family: var(--font-ui);
  font-size: 12px;
  color: var(--text-secondary);
}
.group-header:hover {
  background: var(--bg-hover);
}
.group-header.drag-over {
  background: var(--accent-subtle);
  box-shadow: inset 0 0 0 1px var(--accent);
}
.group-arrow {
  display: inline-flex;
  align-items: center;
  width: 16px;
  color: var(--text-disabled);
  flex-shrink: 0;
}
.group-name {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-count {
  margin-left: auto;
  font-size: 10px;
  color: var(--text-disabled);
  background: var(--bg-subtle);
  padding: 0 5px;
  border-radius: 8px;
  flex-shrink: 0;
}

/* Connection item styles (mirror Sidebar.vue) */
.connection-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.12s ease;
  margin-bottom: 2px;
  user-select: none;
}
.connection-item.indented {
  padding-left: 24px;
}
.connection-item:hover {
  background: var(--bg-hover);
}
.connection-item.active {
  background: var(--accent-subtle);
  box-shadow: inset 0 0 0 1px var(--accent-dim);
}
.connection-item.active .name {
  color: var(--accent);
}

.conn-more-btn {
  display: none;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
  margin-left: auto;
  padding: 0;
}
.connection-item:hover .conn-more-btn {
  display: flex;
}
.conn-more-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.conn-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  flex-shrink: 0;
  color: var(--text-muted);
}

.conn-details {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.name {
  font-family: var(--font-ui);
  font-size: 12px;
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.4;
}

.host {
  font-family: var(--font-ui);
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.4;
}

.conn-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
</style>
