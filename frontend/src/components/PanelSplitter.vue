<template>
  <div
    ref="el"
    class="panel-splitter"
    :class="direction"
    @mousedown="startDrag"
  ></div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  direction: 'horizontal' | 'vertical'
}>()

const emit = defineEmits<{
  resize: [delta: number]
}>()

const el = ref<HTMLElement | null>(null)

/* Crossing grips (SplitterCrossings.vue) start drags with a synthetic
   mousedown carrying the grip's coordinates, so the plain @mousedown entry
   point serves both real and grip-driven gestures. */
function startDrag(e: MouseEvent) {
  e.preventDefault()
  window.dispatchEvent(new CustomEvent('split:resize-start'))
  const direction = props.direction
  const splitter = el.value ?? (e.currentTarget as HTMLElement)
  const grid = splitter.parentElement
  const gridRect = grid?.getBoundingClientRect()
  const gridSize = direction === 'horizontal' ? (gridRect?.width || 400) : (gridRect?.height || 400)
  let lastPos = direction === 'horizontal' ? e.clientX : e.clientY
  splitter.classList.add('dragging')

  function onMove(ev: MouseEvent) {
    ev.preventDefault()
    const currentPos = direction === 'horizontal' ? ev.clientX : ev.clientY
    const delta = currentPos - lastPos
    lastPos = currentPos
    emit('resize', delta / gridSize)
  }

  function onUp() {
    splitter.classList.remove('dragging')
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    window.dispatchEvent(new CustomEvent('split:resize-end'))
  }

  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}
</script>

<style scoped>
.panel-splitter {
  flex-shrink: 0;
  /* The gutter between island panels shows the floor; only a hover lights it */
  background: transparent;
  transition: background 0.15s;
}
.panel-splitter:hover,
.panel-splitter.hint,
.panel-splitter.dragging {
  background: var(--accent);
}
.panel-splitter.horizontal {
  width: 0.25rem;
  cursor: col-resize;
}
.panel-splitter.vertical {
  height: 0.25rem;
  cursor: row-resize;
}
</style>
