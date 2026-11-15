<template>
  <div class="field-tree">
    <template v-for="(node, i) in nodes" :key="keyOf(path, i)">
      <div class="ft-node" :style="{ paddingLeft: (depth * 14) + 'px' }">
        <span
          v-if="hasChildren(node)"
          class="ft-caret"
          :class="{ open: isOpen(keyOf(path, i)) }"
          @click="toggle(keyOf(path, i))"
        ></span>
        <span v-else class="ft-caret ft-caret-empty"></span>
        <span class="ft-label">{{ node.name }}</span>
        <span v-if="node.value !== undefined && node.value !== ''" class="ft-value">{{ node.value }}</span>
        <span class="ft-offset" v-if="showInfo">[{{ node.offset }}, {{ node.len }}]</span>
      </div>
      <div v-if="hasChildren(node) && isOpen(keyOf(path, i))">
        <FieldTree :nodes="node.children || []" :depth="depth + 1" :path="keyOf(path, i)" :show-info="showInfo" />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

export interface FieldTreeNode {
  name: string
  value?: string
  offset?: number
  len?: number
  children?: FieldTreeNode[]
}

const props = withDefaults(
  defineProps<{
    nodes: FieldTreeNode[]
    depth?: number
    path?: string
    showInfo?: boolean
  }>(),
  { depth: 0, path: '', showInfo: false }
)

function keyOf(path: string, i: number): string {
  return path ? `${path}/${i}` : `${i}`
}

function hasChildren(node: FieldTreeNode): boolean {
  return !!(node.children && node.children.length > 0)
}

const openMap = ref<Record<string, boolean>>({})
function defaultOpen(node: FieldTreeNode): boolean {
  // Layer nodes expand by default; leaves are not togglable.
  return hasChildren(node)
}
function isOpen(key: string): boolean {
  if (openMap.value[key] === undefined) {
    openMap.value[key] = defaultOpen(props.nodes[Number(key.split('/').pop())])
  }
  return !!openMap.value[key]
}
function toggle(key: string) {
  openMap.value[key] = !(openMap.value[key] ?? defaultOpen(props.nodes[Number(key.split('/').pop())]))
  openMap.value = { ...openMap.value }
}
</script>

<style scoped>
.field-tree {
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  line-height: 1.55;
  user-select: text;
  -webkit-user-select: text;
}
.ft-node {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  padding: 0.0625rem 0.25rem;
  border-radius: 3px;
  white-space: nowrap;
}
.ft-node:hover {
  background: var(--bg-hover);
}
.ft-caret {
  flex-shrink: 0;
  width: 0;
  height: 0;
  border-top: 4px solid transparent;
  border-bottom: 4px solid transparent;
  border-left: 5px solid var(--text-muted);
  transition: transform 0.12s ease;
  cursor: pointer;
  align-self: center;
}
.ft-caret.open {
  transform: rotate(90deg);
}
.ft-caret-empty {
  border: none;
  cursor: default;
  width: 6px;
  height: 2px;
  background: transparent;
}
.ft-label {
  color: var(--text-secondary);
}
.ft-value {
  color: var(--text-primary);
  word-break: break-all;
}
.ft-offset {
  color: var(--text-muted);
  flex-shrink: 0;
}
</style>
