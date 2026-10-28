import { ref, computed, watch, nextTick, type Ref } from 'vue'
import { useConnectionStore } from '../stores/connectionStore'
import type { GroupTreeNode } from '../stores/connectionStore'
import { useFavoriteStore } from '../stores/favoriteStore'
import { useLocalStateStore } from '../stores/localStateStore'
import type { ConnectionConfig } from '../types/session'
import { matchTypeFilter } from '../utils/quickConnect'
import { isWorkspaceHostType } from './createWorkspace'

/**
 * Connection-tree state & behaviour extracted from Sidebar.vue.
 *
 * Owns: expand/collapse persistence, search + type-filter tree building,
 * favorites virtual group, drag-and-drop reordering (connections, groups,
 * favorites), multi-select, keyboard navigation and "locate connection".
 *
 * The sidebar element ref and a few UI-level refs (form/menu visibility,
 * the right-clicked connection) are injected so keyboard/locate handlers
 * can guard and mutate them exactly as the original inline code did.
 */
export function useConnectionTree(options: {
  sidebarEl: Ref<HTMLDivElement | undefined>
  showForm: Ref<boolean>
  menuVisible: Ref<boolean>
  groupMenuVisible: Ref<boolean>
  selectedConn: Ref<ConnectionConfig | null>
  onConnect: (conn: ConnectionConfig) => void
  openNewFormFromSearch: () => void
}) {
  const { sidebarEl, showForm, menuVisible, groupMenuVisible, selectedConn, onConnect, openNewFormFromSearch } = options
  const connectionStore = useConnectionStore()
  const favoriteStore = useFavoriteStore()

  // ── Search + type filter ──
  const searchQuery = ref('')
  const selectedTypeFilter = ref('all')

  // ── Expand/collapse state ──
  const expandedGroups = ref<Set<string>>(new Set())
  const collapsedGroupIds = ref<Set<string>>(new Set())

  // Build expandedGroups from all known group IDs minus those explicitly collapsed
  function syncExpandedFromCollapsed(groupIds: string[]) {
    expandedGroups.value = new Set(groupIds.filter(id => !collapsedGroupIds.value.has(id)))
  }

  // Persist collapsedGroupIds to LocalState
  async function persistCollapsedState() {
    try {
      useLocalStateStore().update({ collapsedGroupIds: [...collapsedGroupIds.value] })
    } catch {
      // ignore errors — collapse state is not critical
    }
  }

  async function toggleGroup(groupId: string) {
    if (expandedGroups.value.has(groupId)) {
      expandedGroups.value.delete(groupId)
      collapsedGroupIds.value.add(groupId)
    } else {
      expandedGroups.value.add(groupId)
      collapsedGroupIds.value.delete(groupId)
    }
    await persistCollapsedState()
  }

  // ── Multi-select ──
  const selectedIds = ref<Set<string>>(new Set())
  const focusedId = ref<string | null>(null)
  const lastClickId = ref<string | null>(null)

  // ── Search filter ──
  function filterTreeNode(node: GroupTreeNode, q: string, matchConn: (c: ConnectionConfig) => boolean): GroupTreeNode | null {
    const groupNameMatch = node.group.name.toLowerCase().includes(q)
    const filteredConns = groupNameMatch
      ? node.connections.filter(matchConn)
      : node.connections.filter(matchConn)
    const filteredChildren: GroupTreeNode[] = []
    for (const child of node.children) {
      const filtered = filterTreeNode(child, q, matchConn)
      if (filtered) filteredChildren.push(filtered)
    }
    const hasMatchingName = groupNameMatch
    const hasConnections = filteredConns.length > 0
    const hasChildren = filteredChildren.length > 0
    if (hasMatchingName || hasConnections || hasChildren) {
      return { group: node.group, connections: hasMatchingName ? node.connections.filter(matchConn) : filteredConns, children: filteredChildren }
    }
    return null
  }

  // Shared search-text + type-filter predicate used by the group tree and the
  // favorites list so both sections filter identically.
  function matchesConnFilters(c: ConnectionConfig): boolean {
    const q = searchQuery.value.trim().toLowerCase()
    const textMatch = !q || c.name.toLowerCase().includes(q) || c.host.toLowerCase().includes(q)
    return textMatch && matchTypeFilter(c, selectedTypeFilter.value)
  }

  const filteredGrouped = computed(() => {
    const q = searchQuery.value.trim().toLowerCase()
    const data = connectionStore.groupedConnections

    const matchConn = matchesConnFilters

    const filteredRoots: GroupTreeNode[] = []
    for (const root of data.roots) {
      const filtered = filterTreeNode(root, q, matchConn)
      if (filtered) filteredRoots.push(filtered)
    }

    const filteredUngrouped = data.ungrouped.filter(matchConn)

    return { roots: filteredRoots, ungrouped: filteredUngrouped }
  })

  // ── Favorites (virtual group above all groups) ──
  // Favorites keep their own expand state — they are not a real group, so they
  // are neither persisted to collapsedGroupIds nor touched by the search-mode
  // expand/restore logic.
  const favoritesExpanded = ref(true)

  // Resolved favorite connections in saved order; ids of deleted connections
  // are silently dropped (same tolerate-missing policy as recent connections).
  const favoriteConns = computed<ConnectionConfig[]>(() => {
    return favoriteStore.favoriteIds
      .map(id => connectionStore.connections.find(c => c.id === id))
      .filter((c): c is ConnectionConfig => !!c)
      .filter(matchesConnFilters)
  })

  function countTreeNodes(nodes: GroupTreeNode[]): number {
    let count = 0
    for (const node of nodes) {
      count += node.connections.length
      count += countTreeNodes(node.children)
    }
    return count
  }

  const totalFiltered = computed(() => {
    return countTreeNodes(filteredGrouped.value.roots) + filteredGrouped.value.ungrouped.length
  })

  function collectTreeConnIds(nodes: GroupTreeNode[]): string[] {
    const ids: string[] = []
    for (const node of nodes) {
      for (const c of node.connections) ids.push(c.id)
      ids.push(...collectTreeConnIds(node.children))
    }
    return ids
  }

  // Update selection when filter changes
  watch(filteredGrouped, () => {
    const allIds = collectTreeConnIds(filteredGrouped.value.roots)
    for (const c of filteredGrouped.value.ungrouped) allIds.push(c.id)
    // Favorites rows are also selectable; they render above the tree
    if (favoritesExpanded.value) {
      allIds.unshift(...favoriteConns.value.map(c => c.id))
    }

    if (allIds.length === 0) {
      focusedId.value = searchQuery.value.trim() ? '__new_connection__' : null
      selectedIds.value = new Set()
    } else if (!focusedId.value || !allIds.includes(focusedId.value)) {
      focusedId.value = allIds[0]
      selectedIds.value = new Set([allIds[0]])
    }
  }, { immediate: true })

  // Track the previous group ID set so we can detect new vs deleted groups
  const prevGroupIds = ref<Set<string>>(new Set())
  const collapsedStateLoaded = ref(false)

  // Sync expanded state when groups change (add/delete/rename) — preserve user collapse choices
  watch(() => connectionStore.allGroupIds, (ids) => {
    if (!collapsedStateLoaded.value) return
    const currentIds = new Set<string>(ids)
    if (connectionStore.groups.length > 0) {
      currentIds.add('__ungrouped__')
    }

    // Remove stale group IDs from both expanded and collapsed
    const allKnown = new Set([...expandedGroups.value, ...collapsedGroupIds.value])
    for (const id of allKnown) {
      if (id === '__ungrouped__' && connectionStore.groups.length === 0) continue
      if (id !== '__ungrouped__' && !currentIds.has(id)) {
        expandedGroups.value.delete(id)
        collapsedGroupIds.value.delete(id)
      }
    }

    // New groups (and __ungrouped__ if first time) — expand by default
    for (const id of currentIds) {
      if (!prevGroupIds.value.has(id) && !collapsedGroupIds.value.has(id)) {
        expandedGroups.value.add(id)
      }
    }

    prevGroupIds.value = currentIds
  })

  // Initialize collapse state from persisted LocalState
  async function initCollapseState() {
    try {
      const ls = useLocalStateStore()
      if (!ls.loaded) await ls.init()
      if (ls.state.collapsedGroupIds && ls.state.collapsedGroupIds.length > 0) {
        collapsedGroupIds.value = new Set(ls.state.collapsedGroupIds)
      }
    } catch {
      // Use default (all expanded)
    }

    // Build initial expandedGroups and prevGroupIds
    const allIds = connectionStore.allGroupIds.slice()
    if (connectionStore.groups.length > 0) {
      allIds.push('__ungrouped__')
    }
    expandedGroups.value = new Set(allIds.filter(id => !collapsedGroupIds.value.has(id)))
    prevGroupIds.value = new Set(allIds)
    collapsedStateLoaded.value = true
  }

  // Track previous search query state
  const wasSearching = ref(false)

  // Auto-expand all groups while searching; restore persisted state when search is cleared
  watch(searchQuery, (q) => {
    const isSearching = !!q.trim()
    if (isSearching && !wasSearching.value) {
      // Entering search mode — expand all visually (don't touch collapsedGroupIds)
      expandedGroups.value = new Set()
      function expandTree(nodes: GroupTreeNode[]) {
        for (const node of nodes) {
          expandedGroups.value.add(node.group.id)
          expandTree(node.children)
        }
      }
      expandTree(filteredGrouped.value.roots)
      if (connectionStore.groups.length > 0) {
        expandedGroups.value.add('__ungrouped__')
      }
    } else if (!isSearching && wasSearching.value) {
      // Exiting search mode — restore from persisted collapsed state
      const allIds = connectionStore.allGroupIds
      if (connectionStore.groups.length > 0) {
        allIds.push('__ungrouped__')
      }
      syncExpandedFromCollapsed(allIds)
      // Clean stale entries from collapsedGroupIds
      collapsedGroupIds.value = new Set(
        [...collapsedGroupIds.value].filter(id => id === '__ungrouped__' || connectionStore.groups.some(g => g.id === id))
      )
    }
    wasSearching.value = isSearching
  })

  // ── Keyboard navigation ──
  function getAllVisibleIds(): string[] {
    const ids: string[] = []
    if (favoritesExpanded.value) {
      for (const c of favoriteConns.value) ids.push(c.id)
    }
    function collectVisible(nodes: GroupTreeNode[]) {
      for (const node of nodes) {
        if (expandedGroups.value.has(node.group.id)) {
          for (const c of node.connections) ids.push(c.id)
          collectVisible(node.children)
        }
      }
    }
    collectVisible(filteredGrouped.value.roots)
    if (connectionStore.groups.length > 0) {
      if (expandedGroups.value.has('__ungrouped__')) {
        for (const c of filteredGrouped.value.ungrouped) ids.push(c.id)
      }
    } else {
      for (const c of filteredGrouped.value.ungrouped) ids.push(c.id)
    }
    if (searchQuery.value.trim()) {
      ids.push('__new_connection__')
    }
    return ids
  }

  function scrollActiveIntoView() {
    nextTick(() => {
      const activeEl = sidebarEl.value?.querySelector('.connection-item.active') as HTMLElement
      activeEl?.scrollIntoView({ block: 'nearest' })
    })
  }

  // ── Connection click / multi-select ──
  function onItemClick(e: MouseEvent, conn: ConnectionConfig) {
    if (e.shiftKey && lastClickId.value) {
      // Range select from last click to current
      const ids = getAllVisibleIds()
      const anchorIdx = ids.indexOf(lastClickId.value)
      const currentIdx = ids.indexOf(conn.id)
      if (anchorIdx >= 0 && currentIdx >= 0) {
        const [start, end] = anchorIdx < currentIdx ? [anchorIdx, currentIdx] : [currentIdx, anchorIdx]
        const selected = new Set<string>()
        for (let i = start; i <= end; i++) {
          selected.add(ids[i])
        }
        selectedIds.value = selected
      }
      focusedId.value = conn.id
    } else if (e.ctrlKey || e.metaKey) {
      // Toggle multi-select
      if (selectedIds.value.has(conn.id)) {
        selectedIds.value.delete(conn.id)
      } else {
        selectedIds.value.add(conn.id)
      }
      selectedIds.value = new Set(selectedIds.value)
      lastClickId.value = conn.id
    } else {
      focusedId.value = conn.id
      selectedIds.value = new Set([conn.id])
      lastClickId.value = conn.id
    }
  }

  function onItemDblClick(conn: ConnectionConfig) {
    selectedIds.value = new Set()
    onConnect(conn)
  }

  // ── Context menu helper ──
  function getSelectedConnectionIds(): string[] {
    if (selectedIds.value.size > 0) {
      return [...selectedIds.value]
    }
    if (selectedConn.value) {
      return [selectedConn.value.id]
    }
    if (focusedId.value) {
      return [focusedId.value]
    }
    return []
  }

  function onListKeydown(e: KeyboardEvent) {
    if (showForm.value || menuVisible.value || groupMenuVisible.value) return
    const ids = getAllVisibleIds()
    if (ids.length === 0) return

    const idx = ids.indexOf(focusedId.value || '')

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      const nextIdx = idx >= 0 && idx < ids.length - 1 ? idx + 1 : 0
      focusedId.value = ids[nextIdx]
      selectedIds.value = new Set([ids[nextIdx]])
      scrollActiveIntoView()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      const prevIdx = idx > 0 ? idx - 1 : ids.length - 1
      focusedId.value = ids[prevIdx]
      selectedIds.value = new Set([ids[prevIdx]])
      scrollActiveIntoView()
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (focusedId.value === '__new_connection__') {
        openNewFormFromSearch()
        return
      }
      const ids = getSelectedConnectionIds()
      if (ids.length > 0) {
        for (const id of ids) {
          const c = connectionStore.connections.find(c => c.id === id)
          if (c) {
            onConnect(c)
          }
        }
        selectedIds.value = new Set()
      }
    }
  }

  // ── Drag & drop ──
  const dragOverGroupId = ref<string | null>(null)
  // Insertion indicator for manual reordering. `id` is a connection id or a
  // group id (or '__ungrouped__'); position drives before/after/inside visuals.
  const dropIndicator = ref<{ id: string; position: 'before' | 'after' | 'inside' } | null>(null)

  // Reordering is only meaningful against the full, unfiltered list.
  const reorderEnabled = computed(() => !searchQuery.value.trim() && selectedTypeFilter.value === 'all')

  function onDragStart(e: DragEvent, conn: ConnectionConfig) {
    const ids = getSelectedConnectionIds()
    // If dragging an unselected item, drag just this one
    if (!ids.includes(conn.id)) {
      selectedIds.value = new Set()
      e.dataTransfer!.setData('text/plain', JSON.stringify([conn.id]))
    } else {
      e.dataTransfer!.setData('text/plain', JSON.stringify(ids))
    }
    // Terminal-type connections can also be dropped onto a workspace panel to
    // connect + split at the drop position. The row-under-cursor wins over the
    // selection on purpose — a split drop is a single-connection gesture.
    if (isWorkspaceHostType(conn.type)) {
      e.dataTransfer!.setData('application/conn-id', conn.id)
    }
    // Any connection can be dropped onto the tab bar to open a tab there.
    e.dataTransfer!.setData('application/conn-open', conn.id)
    e.dataTransfer!.effectAllowed = 'move'
  }

  function onDragEnd() {
    dragOverGroupId.value = null
    dropIndicator.value = null
  }

  // Ordered connection ids within a group, matching connections array order.
  function connSiblingIds(groupId: string | undefined): string[] {
    return connectionStore.connections
      .filter(c => (c.groupId || undefined) === (groupId || undefined))
      .map(c => c.id)
  }

  // ── Connection item drag (reorder within / across groups) ──
  function onConnDragOver(e: DragEvent, conn: ConnectionConfig) {
    if (!reorderEnabled.value) return
    const el = e.currentTarget as HTMLElement
    const rect = el.getBoundingClientRect()
    const position = (e.clientY - rect.top) < rect.height / 2 ? 'before' : 'after'
    dropIndicator.value = { id: conn.id, position }
    dragOverGroupId.value = null
  }

  async function onConnDrop(e: DragEvent, conn: ConnectionConfig) {
    const indicator = dropIndicator.value
    dropIndicator.value = null
    const raw = e.dataTransfer?.getData('text/plain')
    if (!raw || !indicator) return
    try {
      const data = JSON.parse(raw)
      if (!Array.isArray(data) || data.length === 0) return
      const targetGroupId = conn.groupId || undefined
      let beforeId: string | undefined
      if (indicator.position === 'before') {
        beforeId = conn.id
      } else {
        const siblings = connSiblingIds(targetGroupId)
        const idx = siblings.indexOf(conn.id)
        beforeId = idx >= 0 ? siblings[idx + 1] : undefined
      }
      await connectionStore.moveConnections(data, targetGroupId, beforeId)
      selectedIds.value = new Set()
    } catch {
      // ignore parse errors
    }
  }

  // ── Group header drag (reorder siblings vs reparent/move-in) ──
  function onGroupDragOver(groupId: string, e?: DragEvent) {
    if (reorderEnabled.value && e && groupId !== '__ungrouped__') {
      const el = e.currentTarget as HTMLElement
      const rect = el.getBoundingClientRect()
      const offset = e.clientY - rect.top
      const edge = rect.height * 0.28
      if (offset < edge) {
        dropIndicator.value = { id: groupId, position: 'before' }
        dragOverGroupId.value = null
        return
      }
      if (offset > rect.height - edge) {
        dropIndicator.value = { id: groupId, position: 'after' }
        dragOverGroupId.value = null
        return
      }
    }
    // Middle region (or reorder disabled / ungrouped): move-in / reparent
    dropIndicator.value = null
    dragOverGroupId.value = groupId
  }

  function onGroupDragLeave(groupId: string) {
    if (dragOverGroupId.value === groupId) {
      dragOverGroupId.value = null
    }
    if (dropIndicator.value?.id === groupId) {
      dropIndicator.value = null
    }
  }

  async function onGroupDrop(groupId: string, e: DragEvent) {
    const indicator = dropIndicator.value
    dragOverGroupId.value = null
    dropIndicator.value = null
    const raw = e.dataTransfer?.getData('text/plain')
    if (!raw) return
    try {
      const data = JSON.parse(raw)
      // Dropping a group
      if (data && data.type === 'group') {
        if (indicator && indicator.id === groupId && indicator.position !== 'inside') {
          // Reorder as sibling of the target group
          const target = connectionStore.groups.find(g => g.id === groupId)
          const newParentId = target?.parentId
          let beforeId: string | undefined
          if (indicator.position === 'before') {
            beforeId = groupId
          } else {
            const siblings = connectionStore.groups
              .filter(g => (g.parentId || undefined) === (newParentId || undefined))
              .map(g => g.id)
            const idx = siblings.indexOf(groupId)
            beforeId = idx >= 0 ? siblings[idx + 1] : undefined
          }
          await connectionStore.moveGroup(data.id, newParentId, beforeId)
          return
        }
        // Middle: move into this group as a child
        const targetParentId = groupId === '__ungrouped__' ? undefined : groupId
        await connectionStore.reparentGroup(data.id, targetParentId)
        return
      }
      // Dropping connections
      if (Array.isArray(data) && data.length > 0) {
        const targetGroupId = groupId === '__ungrouped__' ? undefined : groupId
        if (indicator && indicator.id === groupId && indicator.position !== 'inside') {
          // Reorder relative to the group header: before = group's first slot
          let beforeId: string | undefined
          if (indicator.position === 'before') {
            beforeId = connSiblingIds(targetGroupId)[0]
          } else {
            beforeId = undefined // after header: append to group end
          }
          await connectionStore.moveConnections(data, targetGroupId, beforeId)
        } else {
          await connectionStore.moveConnections(data, targetGroupId)
        }
        selectedIds.value = new Set()
      }
    } catch {
      // ignore parse errors
    }
  }

  // ── Favorites drag & drop: reorder within the favorites list only ──
  function onFavDragOver(e: DragEvent, conn: ConnectionConfig) {
    if (!reorderEnabled.value) return
    const el = e.currentTarget as HTMLElement
    const rect = el.getBoundingClientRect()
    const position = (e.clientY - rect.top) < rect.height / 2 ? 'before' : 'after'
    dropIndicator.value = { id: conn.id, position }
    dragOverGroupId.value = null
  }

  async function onFavDrop(e: DragEvent, conn: ConnectionConfig) {
    if (!reorderEnabled.value) return
    const indicator = dropIndicator.value
    dropIndicator.value = null
    const raw = e.dataTransfer?.getData('text/plain')
    if (!raw || !indicator) return
    try {
      const data = JSON.parse(raw)
      if (!Array.isArray(data) || data.length === 0) return
      let beforeId: string | undefined
      if (indicator.position === 'before') {
        beforeId = conn.id
      } else {
        // After: insert before the favorite that follows the target
        const ids = favoriteStore.favoriteIds
        const idx = ids.indexOf(conn.id)
        beforeId = idx >= 0 ? ids[idx + 1] : undefined
      }
      await favoriteStore.reorder(data, beforeId)
    } catch {
      // ignore parse errors
    }
  }

  // ── Locate a connection from the tab context menu ──
  async function locateConnectionById(id: string) {
    const conn = connectionStore.connections.find(c => c.id === id)
    if (!conn) return

    // Surface the target: drop any search / type filter that could hide it.
    searchQuery.value = ''
    selectedTypeFilter.value = 'all'

    // Expand the containing group (and its ancestors) so the row is rendered.
    if (conn.groupId) {
      let gid: string | undefined = conn.groupId
      while (gid) {
        expandedGroups.value.add(gid)
        collapsedGroupIds.value.delete(gid)
        const g = connectionStore.groups.find(g => g.id === gid)
        gid = g?.parentId
      }
      expandedGroups.value = new Set(expandedGroups.value)
    } else if (connectionStore.groups.length > 0) {
      // Ungrouped target with real groups present → expand the virtual No-Group group.
      expandedGroups.value.add('__ungrouped__')
      collapsedGroupIds.value.delete('__ungrouped__')
      expandedGroups.value = new Set(expandedGroups.value)
    }

    // Select the target. focusedId is set before the filter watcher flushes,
    // so that watcher keeps our selection instead of resetting to the first row.
    focusedId.value = conn.id
    lastClickId.value = conn.id
    selectedConn.value = conn
    selectedIds.value = new Set([conn.id])

    await nextTick()
    await nextTick()
    const row = sidebarEl.value?.querySelector<HTMLElement>(`.connection-item[data-conn-id="${conn.id}"]`)
    row?.scrollIntoView({ block: 'nearest' })
  }

  function onLocateConnection(e: Event) {
    const id = (e as CustomEvent)?.detail?.id
    if (typeof id === 'string') locateConnectionById(id)
  }

  return {
    // search + filter
    searchQuery,
    selectedTypeFilter,
    filteredGrouped,
    totalFiltered,
    matchesConnFilters,
    // expand/collapse
    expandedGroups,
    collapsedGroupIds,
    toggleGroup,
    initCollapseState,
    // favorites
    favoritesExpanded,
    favoriteConns,
    // selection
    selectedIds,
    focusedId,
    lastClickId,
    onItemClick,
    onItemDblClick,
    getSelectedConnectionIds,
    // keyboard
    onListKeydown,
    getAllVisibleIds,
    scrollActiveIntoView,
    // drag & drop
    dragOverGroupId,
    dropIndicator,
    reorderEnabled,
    onDragStart,
    onDragEnd,
    onConnDragOver,
    onConnDrop,
    onGroupDragOver,
    onGroupDragLeave,
    onGroupDrop,
    onFavDragOver,
    onFavDrop,
    // locate
    locateConnectionById,
    onLocateConnection,
  }
}