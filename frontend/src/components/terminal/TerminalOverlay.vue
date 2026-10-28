<template>
  <div v-if="dragOver" class="drop-overlay">
    <span>{{ t('sftp.dropHere') }}</span>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from '../../i18n'

const { t } = useI18n()

const dragOver = ref(false)
let dragEnterCount = 0

function onDragOver(e: DragEvent) {
  if (!e.dataTransfer?.types.includes('Files')) return
  e.stopPropagation()
  e.dataTransfer.dropEffect = 'copy'
  dragOver.value = true
}

function onDragEnter(e: DragEvent) {
  if (!e.dataTransfer?.types.includes('Files')) return
  e.stopPropagation()
  dragEnterCount++
  dragOver.value = true
}

function onDragLeave() {
  dragEnterCount--
  if (dragEnterCount <= 0) {
    dragEnterCount = 0
    dragOver.value = false
  }
}

function onDragDrop(e: DragEvent) {
  dragOver.value = false
  dragEnterCount = 0
  // Internal panel/tab drags — let them bubble to workspace handlers
  if (e.dataTransfer?.types.includes('application/panel-id') ||
      e.dataTransfer?.types.includes('application/tab-id')) return
  // OS file drops are handled by the native file-drop event; prevent the
  // WebView from navigating and stop here.
  if (e.dataTransfer?.types.includes('Files')) e.preventDefault()
}

defineExpose({
  onDragOver,
  onDragEnter,
  onDragLeave,
  onDragDrop,
})
</script>

<style scoped>
.drop-overlay {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--scrim);
  pointer-events: none;
}
.drop-overlay span {
  font-size: 0.875rem;
  color: var(--text-primary);
  padding: 0.75rem 1.5rem;
  border: 0.125rem dashed var(--border-hover);
  border-radius: var(--radius-sm);
}
</style>