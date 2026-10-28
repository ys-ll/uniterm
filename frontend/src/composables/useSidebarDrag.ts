import { ref, onMounted, type Ref } from 'vue'
import { useLocalStateStore } from '../stores/localStateStore'

/**
 * Sidebar width drag-resize + persistence.
 *
 * Extracted from Sidebar.vue to isolate the resize-handle interaction and
 * LocalState width persistence from the connection-tree logic. The sidebar
 * element ref is supplied by the caller so the composable can read/flush the
 * live width during a drag (avoiding reactive re-renders mid-drag).
 */
export const SIDEBAR_MIN_WIDTH = 240
export const SIDEBAR_MAX_WIDTH = 400

export function useSidebarDrag(sidebarEl: Ref<HTMLDivElement | undefined>) {
  const sidebarWidth = ref(SIDEBAR_MIN_WIDTH)
  const isResizing = ref(false)

  // Restore the persisted width once LocalState is loaded (App.vue awaits
  // init() on mount; children mount first, so re-init here — it is idempotent).
  onMounted(async () => {
    try {
      const ls = useLocalStateStore()
      if (!ls.loaded) await ls.init()
      const w = ls.state.sidebarWidth
      if (w > 0) sidebarWidth.value = Math.min(Math.max(w, SIDEBAR_MIN_WIDTH), SIDEBAR_MAX_WIDTH)
    } catch {
      // keep the default width
    }
  })

  function onResizeStart(e: MouseEvent) {
    isResizing.value = true
    const el = sidebarEl.value
    if (!el) return
    const startX = e.clientX
    const startWidth = el.offsetWidth

    window.dispatchEvent(new CustomEvent('split:resize-start'))

    function onMouseMove(ev: MouseEvent) {
      if (!isResizing.value) return
      const delta = ev.clientX - startX
      const newWidth = Math.min(Math.max(startWidth + delta, SIDEBAR_MIN_WIDTH), SIDEBAR_MAX_WIDTH)
      el!.style.width = newWidth + 'px'
    }

    function onMouseUp() {
      isResizing.value = false
      sidebarWidth.value = el!.offsetWidth
      useLocalStateStore().update({ sidebarWidth: sidebarWidth.value })
      document.removeEventListener('mousemove', onMouseMove)
      document.removeEventListener('mouseup', onMouseUp)
      window.dispatchEvent(new CustomEvent('split:resize-end'))
    }

    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
  }

  return { sidebarWidth, isResizing, onResizeStart }
}