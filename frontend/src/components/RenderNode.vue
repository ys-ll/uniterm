<template>
  <div v-if="node.type === 'leaf'" class="leaf-node">
    <SettingsTab v-if="panel?.type === 'settings'" />
    <div
      v-else-if="panel"
      class="panel-wrapper"
      @click="onPanelClick(panel.id)"
      @dragover.prevent="onDragOver($event, node.panelId)"
      @dragleave="onDragLeave($event)"
      @drop="onDrop($event, node.panelId)"
    >
      <Panel
        :panel="panel"
        :show-header="isMultiPanel"
        :is-active="activePanelId === panel.id"
        :workspace-id="tabId"
        :key="`${panel.id}-${tabId}`"
        @close="handleClosePanel(panel.id)"
        @dragstart="onPanelDragStart($event, panel.id)"
        @toggle-ai-lock="$emit('toggleAiLock', $event)"
        @duplicate="$emit('duplicate', $event)"
        @rename="(id, name) => $emit('rename', id, name)"
      />
      <!-- Drop zone overlay -->
      <div v-if="dragOverId === node.panelId" class="drop-zone-overlay">
        <div class="dz dz-left" :class="{ active: dropPos === 'left' }"></div>
        <div class="dz dz-right" :class="{ active: dropPos === 'right' }"></div>
        <div class="dz dz-top" :class="{ active: dropPos === 'top' }"></div>
        <div class="dz dz-bottom" :class="{ active: dropPos === 'bottom' }"></div>
      </div>
    </div>
  </div>
  <div v-else class="split-node" :class="node.direction" :style="splitStyle">
    <template v-for="(child, index) in node.children" :key="getNodeKey(child)">
      <RenderNode
        :node="child"
        :panel-ids="panelIds"
        :active-panel-id="activePanelId"
        :tab-id="tabId"
        @close-panel="(id) => $emit('closePanel', id)"
        @toggle-ai-lock="(id) => $emit('toggleAiLock', id)"
        @duplicate="(id) => $emit('duplicate', id)"
        @rename="(id, name) => $emit('rename', id, name)"
        @panel-drag-start="(e, id) => $emit('panelDragStart', e, id)"
        @panel-drop="(e, id) => $emit('panelDrop', e, id)"
        @resize="(p) => $emit('resize', p)"
      />
      <PanelSplitter
        v-if="index < node.children.length - 1"
        :direction="node.direction"
        @resize="(delta) => $emit('resize', { node, index, delta })"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { usePanelStore } from '../stores/panelStore'
import { useTabStore } from '../stores/tabStore'
import type { LayoutNode } from '../types/workspace'
import Panel from './Panel.vue'
import PanelSplitter from './PanelSplitter.vue'
import SettingsTab from './SettingsTab.vue'

const props = defineProps<{
  node: LayoutNode
  panelIds: string[]
  activePanelId: string | null
  tabId: string
}>()

const emit = defineEmits<{
  closePanel: [panelId: string]
  toggleAiLock: [panelId: string]
  duplicate: [panelId: string]
  rename: [panelId: string, newName: string]
  panelDragStart: [e: DragEvent, panelId: string]
  panelDrop: [e: DragEvent, targetPanelId: string, rect?: DOMRect]
  resize: [payload: { node: any, index: number, delta: number }]
}>()

const panelStore = usePanelStore()
const tabStore = useTabStore()

const dragOverId = ref<string | null>(null)
const dropPos = ref<string | null>(null)

const panel = computed(() => {
  if (props.node.type === 'leaf') {
    return panelStore.getPanel(props.node.panelId)
  }
  return null
})

const isMultiPanel = computed(() => props.panelIds.length > 1)

const splitStyle = computed(() => {
  if (props.node.type !== 'split') return {}
  const parts: string[] = []
  for (let i = 0; i < props.node.children.length; i++) {
    if (i > 0) parts.push('4px')
    parts.push(`${props.node.sizes[i]}fr`)
  }
  const template = parts.join(' ')
  return {
    display: 'grid',
    gridTemplateColumns: props.node.direction === 'horizontal' ? template : '1fr',
    gridTemplateRows: props.node.direction === 'vertical' ? template : '1fr',
  }
})

function getNodeKey(node: LayoutNode): string {
  if (node.type === 'leaf') return node.panelId
  return `split-${node.direction}-${node.children.length}-${node.children.map(c => getNodeKey(c)).join('-')}`
}

function handleClosePanel(panelId: string) {
  emit('closePanel', panelId)
}

function onPanelClick(panelId: string) {
  tabStore.setActivePanel(props.tabId, panelId)
}

function onPanelDragStart(e: DragEvent, panelId: string) {
  if (!e.dataTransfer) return

  e.dataTransfer.setData('application/panel-id', panelId)
  e.dataTransfer.effectAllowed = 'move'

  // Use a lightweight custom drag image to avoid the browser snapshotting
  // the entire panel (which contains an xterm.js canvas and can be slow).
  const panel = panelStore.getPanel(panelId)
  const title = panel?.title || ''
  const img = document.createElement('div')
  img.textContent = title
  img.style.cssText = `
    position: fixed; left: -9999px; top: -9999px;
    padding: 6px 14px; background: var(--bg-surface);
    border: 1px solid var(--accent); border-radius: var(--radius-sm);
    color: var(--text-primary); font-size: 12px; font-family: var(--font-ui);
    box-shadow: var(--shadow-md); white-space: nowrap; pointer-events: none;
  `
  document.body.appendChild(img)
  e.dataTransfer.setDragImage(img, 10, 10)

  const cleanup = () => {
    img.remove()
    window.removeEventListener('dragend', cleanup)
  }
  window.addEventListener('dragend', cleanup)

  emit('panelDragStart', e, panelId)
}

function onDragOver(e: DragEvent, panelId: string) {
  const types = e.dataTransfer?.types ? Array.from(e.dataTransfer.types) : []
  const hasPanel = types.includes('application/panel-id')
  const hasWorkspace = types.includes('application/workspace-id')
  const hasTabType = types.includes('application/tab-type')
  // Reject workspace tabs and settings tabs
  if (hasWorkspace) return
  if (!hasPanel && !hasTabType) return

  dragOverId.value = panelId
  e.dataTransfer!.dropEffect = 'move'

  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  const x = e.clientX - rect.left
  const y = e.clientY - rect.top

  const xRatio = x / rect.width
  const yRatio = y / rect.height

  if (Math.abs(xRatio - 0.5) >= Math.abs(yRatio - 0.5)) {
    dropPos.value = xRatio < 0.5 ? 'left' : 'right'
  } else {
    dropPos.value = yRatio < 0.5 ? 'top' : 'bottom'
  }
}

function onDragLeave(e: DragEvent) {
  const el = e.currentTarget as HTMLElement
  const relatedTarget = e.relatedTarget as HTMLElement | null
  if (!relatedTarget || !el.contains(relatedTarget)) {
    dragOverId.value = null
    dropPos.value = null
  }
}

function onDrop(e: DragEvent, panelId: string) {
  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  dragOverId.value = null
  dropPos.value = null
  emit('panelDrop', e, panelId, rect)
}
</script>

<style scoped>
.leaf-node {
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}
.panel-wrapper {
  width: 100%;
  height: 100%;
  position: relative;
}
.split-node {
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.drop-zone-overlay {
  position: absolute;
  inset: 0;
  z-index: 10;
  pointer-events: none;
}
.dz {
  position: absolute;
  background: var(--accent-subtle);
  transition: background 0.12s;
}
.dz.active {
  background: var(--accent-glow);
}
.dz-left { left: 0; top: 0; width: 50%; height: 100%; }
.dz-right { right: 0; top: 0; width: 50%; height: 100%; }
.dz-top { left: 0; top: 0; width: 100%; height: 50%; }
.dz-bottom { left: 0; bottom: 0; width: 100%; height: 50%; }
</style>
