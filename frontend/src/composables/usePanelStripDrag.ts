import { ref } from 'vue'
import { useUiLayoutStore, type PanelLocation } from '../stores/uiLayoutStore'

// Shared tab-strip drag & drop for the left sidebar and the bottom panel.
// A tab button drags with a custom MIME payload (the view id); dropping on
// another tab inserts before/after it, dropping on the strip body appends.
// Both strips reveal themselves while a drag is in flight so a hidden dock
// can still receive tabs.

// Custom MIME type so panel-tab drags never mix with the sidebar's
// connection/group drags (which carry their own payloads).
export const PANEL_DND_MIME = 'application/x-uniterm-panel'

export function usePanelStripDrag(location: PanelLocation) {
  const uiLayout = useUiLayoutStore()

  // Where a preview insertion line should show: on this tab, before/after.
  const dropIndicator = ref<{ key: string; position: 'before' | 'after' } | null>(null)
  // The strip itself is hovered (append mode) — highlights the strip.
  const stripDragOver = ref(false)

  function isPanelDrag(e: DragEvent): boolean {
    return Array.from(e.dataTransfer?.types || []).includes(PANEL_DND_MIME)
  }

  function onDragStart(e: DragEvent, key: string) {
    if (!e.dataTransfer) return
    e.dataTransfer.setData(PANEL_DND_MIME, key)
    e.dataTransfer.effectAllowed = 'move'
    // Drag image: the tab button itself is tiny; default drag image is fine.
    uiLayout.draggingPanel = key
  }

  function clearDragState() {
    uiLayout.draggingPanel = null
    dropIndicator.value = null
    stripDragOver.value = false
  }

  function onDragEnd() {
    clearDragState()
  }

  function dockViews(exclude?: string): string[] {
    return uiLayout.orderedViews.filter(v => uiLayout.effectiveLocation(v) === location && v !== exclude)
  }

  // Compute the insertion index for a drop on `key` (before/after by pointer
  // half), falling back to append when the indicator is stale.
  function insertionIndex(dragged: string, key?: string): number {
    const views = dockViews(dragged)
    if (!key) return views.length
    const ind = dropIndicator.value
    const pos = views.indexOf(key)
    if (pos === -1) return views.length
    const after = ind?.key === key && ind.position === 'after' ? 1 : 0
    return pos + after
  }

  function onTabDragOver(e: DragEvent, key: string) {
    if (!isPanelDrag(e)) return
    e.preventDefault()
    e.stopPropagation()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    stripDragOver.value = false
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    dropIndicator.value = {
      key,
      position: e.clientX < rect.left + rect.width / 2 ? 'before' : 'after',
      }
  }

  function onStripDragOver(e: DragEvent) {
    if (!isPanelDrag(e)) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    stripDragOver.value = true
    dropIndicator.value = null
  }

  function onDrop(e: DragEvent, key?: string) {
    const dragged = e.dataTransfer?.getData(PANEL_DND_MIME)
    if (!dragged) return
    uiLayout.movePanel(dragged, location, insertionIndex(dragged, key))
  }

  function onTabDrop(e: DragEvent, key: string) {
    onDrop(e, key)
    clearDragState()
  }

  function onStripDrop(e: DragEvent) {
    onDrop(e)
    clearDragState()
  }

  return {
    dropIndicator,
    stripDragOver,
    onDragStart,
    onDragEnd,
    onTabDragOver,
    onStripDragOver,
    onTabDrop,
    onStripDrop,
  }
}
