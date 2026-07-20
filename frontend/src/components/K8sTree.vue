<template>
  <div class="db-tree-panel">
    <div class="tree-content">
      <template v-for="group in groups" :key="group.key">
        <div
          class="db-header"
          :class="{ selected: false }"
          @click="toggle(group.key)"
        >
          <span class="db-arrow">
            <component :is="expanded.has(group.key) ? ChevronDown : ChevronRight" :size="12" />
          </span>
          <component :is="groupIcon(group.key)" class="db-icon" :size="14" />
          <span class="db-name">{{ group.label }}</span>
        </div>
        <template v-if="expanded.has(group.key)">
          <div
            v-for="r in group.resources"
            :key="r.key"
            class="table-item"
            :class="{ selected: modelValue === r.key }"
            @click="$emit('update:modelValue', r.key)"
          >
            <span class="table-icon-spacer" />
            <component :is="iconOf(r.icon)" class="table-icon" :size="14" />
            <span class="table-name">{{ r.label }}</span>
          </div>
          <div v-if="group.resources.length === 0" class="empty-hint">
            (empty)
          </div>
        </template>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  ChevronDown, ChevronRight,
  Box, Boxes, Layers, GitFork, CirclePlay, Clock, Copy,
  Network, Globe,
  FileText, Lock,
  HardDrive, Database,
  Server, Folder, Bell,
  Package,   // 兜底
} from '@lucide/vue'
import { RESOURCES, type ResourceGroup, type ResourceDescriptor } from '../services/k8sResources'

defineProps<{ modelValue: string }>()
defineEmits<{ (e: 'update:modelValue', key: string): void }>()

interface Group {
  key: ResourceGroup
  label: string
  resources: ResourceDescriptor[]
}

const GROUP_ORDER: ResourceGroup[] = ['workloads', 'network', 'config', 'storage', 'cluster']
const GROUP_LABELS: Record<ResourceGroup, string> = {
  workloads: 'Workloads',
  network: 'Network',
  config: 'Config',
  storage: 'Storage',
  cluster: 'Cluster',
}

const groups = computed<Group[]>(() =>
  GROUP_ORDER.map(g => ({
    key: g,
    label: GROUP_LABELS[g],
    resources: RESOURCES.filter(r => r.group === g),
  }))
)

// 默认全部展开
const expanded = ref<Set<string>>(new Set(GROUP_ORDER))
function toggle(k: string) {
  if (expanded.value.has(k)) expanded.value.delete(k)
  else expanded.value.add(k)
  expanded.value = new Set(expanded.value)
}

// lucide 名 → 组件映射（预加载所需，避免运行时 dynamic import）
// PlayCircle 在 @lucide/vue 已更名为 CirclePlay，这里保留描述器里的 'PlayCircle' 键。
const ICON_MAP: Record<string, any> = {
  Box, Boxes, Layers, GitFork, PlayCircle: CirclePlay, Clock, Copy,
  Network, Globe,
  FileText, Lock,
  HardDrive, Database,
  Server, Folder, Bell,
}
function iconOf(name: string) {
  return ICON_MAP[name] || Package
}
function groupIcon(_g: ResourceGroup) {
  return Folder
}
</script>

<style scoped>
/* 完全复用 DBTreePanel 的样式规则；class 同名，粘贴过来避免跨组件 scoped 冲突。 */
.db-tree-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--bg-elevated, transparent);
}
.tree-content {
  flex: 1;
  overflow: auto;
  padding: 4px 0;
}
.db-header {
  display: flex;
  align-items: center;
  padding: 4px 8px;
  cursor: pointer;
  font-family: var(--font-ui, sans-serif);
  font-size: 13px;
  color: var(--text-primary, inherit);
  user-select: none;
}
.db-header:hover {
  background: var(--bg-hover, rgba(255,255,255,0.04));
}
.db-header.selected {
  background: var(--bg-selected, rgba(255,255,255,0.06));
}
.db-arrow {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  cursor: pointer;
  color: var(--text-secondary, #888);
}
.db-arrow:hover {
  color: var(--text-primary, inherit);
}
.db-icon {
  margin-right: 6px;
  color: var(--text-secondary, #888);
}
.db-name {
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.table-item {
  display: flex;
  align-items: center;
  padding: 3px 8px 3px 24px;
  cursor: pointer;
  font-family: var(--font-ui, sans-serif);
  font-size: 12px;
  color: var(--text-primary, inherit);
  user-select: none;
}
.table-item:hover {
  background: var(--bg-hover, rgba(255,255,255,0.04));
}
.table-item.selected {
  background: var(--accent-bg, rgba(64,150,255,0.15));
  color: var(--accent, #4096ff);
}
.table-icon-spacer {
  width: 16px;
}
.table-icon {
  margin-right: 6px;
}
.table-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.empty-hint {
  padding: 3px 8px 3px 40px;
  color: var(--text-secondary, #888);
  font-size: 11px;
  font-style: italic;
}
</style>
