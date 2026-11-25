<template>
  <div class="panel-grid" :class="{ single: layout.root.type === 'leaf' }">
    <RenderNode
      :node="layout.root"
      :panel-ids="panelIds"
      :active-panel-id="activePanelId"
      :maximized-panel-id="maximizedPanelId"
      :tab-id="tabId"
      @close-panel="$emit('closePanel', $event)"
      @toggle-ai-lock="$emit('toggleAiLock', $event)"
      @duplicate="$emit('duplicate', $event)"
      @rename="(id, name) => $emit('rename', id, name)"
      @panel-drag-start="(e, id) => $emit('panelDragStart', e, id)"
      @panel-drop="(e, id, rect) => $emit('panelDrop', e, id, rect)"
      @resize="$emit('resize', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import type { PanelLayout } from '../types/workspace'
import RenderNode from './RenderNode.vue'

defineProps<{
  layout: PanelLayout
  panelIds: string[]
  activePanelId: string | null
  maximizedPanelId: string | null
  tabId: string
}>()

defineEmits<{
  closePanel: [panelId: string]
  toggleAiLock: [panelId: string]
  duplicate: [panelId: string]
  rename: [panelId: string, newName: string]
  panelDragStart: [e: DragEvent, panelId: string]
  panelDrop: [e: DragEvent, targetPanelId: string, rect?: DOMRect]
  resize: [payload: { node: any, index: number, delta: number }]
}>()
</script>

<style scoped>
.panel-grid {
  width: 100%;
  height: 100%;
  overflow: hidden;
  /* Anchor for the splitter-crossing grips overlay */
  position: relative;
}
/* Single-panel workspace: the panel fills the tab-area frame, so the leaf's
   own island border/radius would read as a second frame inside it */
.panel-grid.single :deep(.leaf-node) {
  border: none;
  border-radius: 0;
}
</style>
