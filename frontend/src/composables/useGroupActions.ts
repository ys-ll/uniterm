import { ref, computed, watch, type Ref } from 'vue'
import { useConnectionStore } from '../stores/connectionStore'
import type { GroupTreeNode } from '../stores/connectionStore'
import { useI18n } from '../i18n'
import type { ConnectionConfig, ConnectionGroup } from '../types/session'

import { createEmptyWorkspaceRecord } from './savedWorkspace'
import { openWorkspaceNameDialog } from './workspaceNameDialog'
import type Menu from '../components/Menu.vue'
import type ConnectionContextMenu from '../components/ConnectionContextMenu.vue'
import type GroupContextMenu from '../components/GroupContextMenu.vue'

/** Subset of Sidebar.vue emits that group actions can trigger. */
export interface SidebarEmit {
  (e: 'connect', c: ConnectionConfig, kind?: 'file' | 'wsl-file' | 'monitor'): void
  (e: 'connectToWorkspace', payload: { config: ConnectionConfig, workspaceId: string }): void
  (e: 'createWorkspace', targets: ConnectionConfig[]): void
  (e: 'openNetscan'): void
}

/**
 * Group CRUD, context-menu coordination and the new-connection dropdown.
 *
 * Extracted from Sidebar.vue so the sidebar shell stays a thin layout
 * component. All dialog visibility refs and menu component refs are owned
 * here; the sidebar only supplies the tree selection API, the form refs it
 * also needs for inline edits, and the emit bridge for app-level events.
 */
export function useGroupActions(options: {
  selectedIds: Ref<Set<string>>
  focusedId: Ref<string | null>
  getSelectedConnectionIds: () => string[]
  searchQuery: Ref<string>
  selectedConn: Ref<ConnectionConfig | null>
  selectedGroup: Ref<ConnectionGroup | null>
  showForm: Ref<boolean>
  editConfig: Ref<ConnectionConfig | undefined>
  newConnGroupId: Ref<string | undefined>
  menuVisible: Ref<boolean>
  groupMenuVisible: Ref<boolean>
  emptyAreaMenuVisible: Ref<boolean>
  menuRef: Ref<InstanceType<typeof ConnectionContextMenu> | null>
  groupMenuRef: Ref<InstanceType<typeof GroupContextMenu> | null>
  emptyAreaMenuRef: Ref<InstanceType<typeof Menu> | null>
  showImportDialog: Ref<boolean>
  showExportDialog: Ref<boolean>
  emit: SidebarEmit
  openNewForm: () => void
  openNewFormFromSearch: () => void
}) {
  const {
    selectedIds, focusedId, getSelectedConnectionIds, searchQuery,
    selectedConn, selectedGroup, showForm, editConfig, newConnGroupId,
    menuVisible, groupMenuVisible, emptyAreaMenuVisible,
    menuRef, groupMenuRef, emptyAreaMenuRef, showImportDialog, showExportDialog,
    emit, openNewForm, openNewFormFromSearch,
  } = options
  const connectionStore = useConnectionStore()
  const { t } = useI18n()

  // ── Connection context menu helpers ──
  function onContextMenu(e: MouseEvent, conn: ConnectionConfig) {
    e.stopPropagation()
    selectedConn.value = conn
    if (!selectedIds.value.has(conn.id)) {
      selectedIds.value = new Set([conn.id])
      focusedId.value = conn.id
    }
    menuRef.value?.openAt(e.clientX, e.clientY)
  }

  function onConnMoreClick(e: MouseEvent, conn: ConnectionConfig) {
    const btn = e.currentTarget as HTMLElement
    const rect = btn.getBoundingClientRect()
    selectedConn.value = conn
    if (!selectedIds.value.has(conn.id)) {
      selectedIds.value = new Set([conn.id])
      focusedId.value = conn.id
    }
    menuRef.value?.openAt(rect.right + 4, rect.top)
  }

  function closeMenu() {
    menuVisible.value = false
  }

  // Resolved multi-select targets for the shared context menu (always includes
  // the right-clicked connection).
  const ctxTargets = computed<ConnectionConfig[]>(() => {
    const conns = getSelectedConnectionIds()
      .map(id => connectionStore.connections.find(c => c.id === id))
      .filter(Boolean) as ConnectionConfig[]
    if (conns.length > 0) return conns
    return selectedConn.value ? [selectedConn.value] : []
  })

  function onCtxConnect(targets: ConnectionConfig[], kind?: 'file' | 'wsl-file' | 'monitor') {
    for (const c of targets) emit('connect', c, kind)
  }

  function onCtxConnectToWorkspace(targets: ConnectionConfig[], workspaceId: string) {
    for (const c of targets) {
      if (c.type !== 'ssh') continue
      emit('connectToWorkspace', { config: c, workspaceId })
    }
  }

  function onCtxCreateWorkspace(targets: ConnectionConfig[]) {
    emit('createWorkspace', targets)
  }

  function onCtxDelete(targets: ConnectionConfig[]) {
    connectionStore.removeMany(targets.map(c => c.id))
    selectedIds.value = new Set()
  }

  function doEdit(config: ConnectionConfig) {
    editConfig.value = { ...config }
    showForm.value = true
  }

  // ── Standalone new group ──
  const showNewGroupDialog = ref(false)
  const newGroupParentId = ref<string | undefined>(undefined)

  // Tree data for el-tree-select
  interface TreeOption {
    value: string
    label: string
    children?: TreeOption[]
  }

  const groupTreeData = computed<TreeOption[]>(() => {
    function buildTree(nodes: GroupTreeNode[]): TreeOption[] {
      return nodes.map(node => ({
        value: node.group.id,
        label: node.group.name,
        children: node.children.length > 0 ? buildTree(node.children) : undefined,
      }))
    }
    return [
      { value: '__none__', label: t('conn.noGroup') },
      ...buildTree(connectionStore.groupedConnections.roots),
    ]
  })

  function doNewConnInGroup() {
    closeMenu()
    closeGroupMenu()
    closeEmptyAreaMenu()
    editConfig.value = undefined
    newConnGroupId.value = selectedGroup.value?.id !== '__ungrouped__' ? selectedGroup.value?.id : undefined
    showForm.value = true
  }

  function doNewGroup(parentGroupId?: string) {
    closeMenu()
    closeGroupMenu()
    closeEmptyAreaMenu()
    newGroupParentId.value = parentGroupId
    showNewGroupDialog.value = true
  }

  function selectedGroupParentId(): string | undefined {
    return (selectedGroup.value && selectedGroup.value.id !== '__ungrouped__') ? selectedGroup.value.id : undefined
  }

  async function confirmNewGroup(name: string, parentId: string | undefined) {
    await connectionStore.addGroup(name, parentId)
  }

  // ── Move to (unified dialog for connections and groups) ──
  const changeDialogMode = ref<'connections' | 'group'>('connections')
  const showChangeGroupDialog = ref(false)
  const changeGroupTargetId = ref<string | undefined>(undefined)
  const showChangeNewGroupDialog = ref(false)
  const changeNewGroupParentId = ref<string | undefined>(undefined)
  const externalChangeGroupIds = ref<string[]>([])

  function doChangeParentGroup() {
    closeMenu()
    closeGroupMenu()
    closeEmptyAreaMenu()
    changeDialogMode.value = 'group'
    changeGroupTargetId.value = selectedGroup.value?.parentId
    showChangeGroupDialog.value = true
  }

  function openChangeGroupFor(ids: string[]) {
    externalChangeGroupIds.value = ids
    changeDialogMode.value = 'connections'
    const groups = new Set(ids.map(id => {
      const c = connectionStore.connections.find(c => c.id === id)
      return c?.groupId || '__none__'
    }))
    if (groups.size === 1) {
      const g = [...groups][0]
      changeGroupTargetId.value = g === '__none__' ? undefined : g
    } else {
      changeGroupTargetId.value = undefined
    }
    showChangeGroupDialog.value = true
  }

  function getChangeGroupIds(): string[] {
    if (externalChangeGroupIds.value.length > 0) return externalChangeGroupIds.value
    return getSelectedConnectionIds()
  }

  async function confirmChangeGroup() {
    if (changeDialogMode.value === 'group') {
      if (selectedGroup.value && selectedGroup.value.id !== '__ungrouped__') {
        await connectionStore.reparentGroup(selectedGroup.value.id, changeGroupTargetId.value)
      }
      showChangeGroupDialog.value = false
      return
    }
    const val = changeGroupTargetId.value
    if (val === '__new__') {
      showChangeNewGroupDialog.value = true
      return
    }
    const groupId = val === '__none__' ? undefined : val
    const ids = getChangeGroupIds()
    connectionStore.setConnectionsGroup(ids, groupId)
    selectedIds.value = new Set()
    showChangeGroupDialog.value = false
    changeGroupTargetId.value = undefined
    externalChangeGroupIds.value = []
  }

  async function confirmChangeNewGroup(name: string, parentId: string | undefined) {
    const group = await connectionStore.addGroup(name, parentId)
    const ids = getChangeGroupIds()
    if (ids.length > 0) {
      connectionStore.setConnectionsGroup(ids, group.id)
      selectedIds.value = new Set()
    }
    showChangeGroupDialog.value = false
    changeGroupTargetId.value = undefined
    externalChangeGroupIds.value = []
  }

  watch(showChangeGroupDialog, (open) => {
    if (!open) externalChangeGroupIds.value = []
  })

  // ── Group context menu ──
  function onGroupContextMenu(e: MouseEvent, group: ConnectionGroup) {
    e.stopPropagation()
    selectedGroup.value = group
    groupMenuRef.value?.openAt(e.clientX, e.clientY)
  }

  function closeGroupMenu() {
    groupMenuVisible.value = false
  }

  // ── Empty area context menu ──
  function onEmptyAreaContextMenu(e: MouseEvent) {
    emptyAreaMenuRef.value?.openAt(e.clientX, e.clientY)
  }

  function closeEmptyAreaMenu() {
    emptyAreaMenuVisible.value = false
  }

  // ── Virtual group context menu ──
  function onVirtualGroupContextMenu(e: MouseEvent) {
    e.stopPropagation()
    selectedGroup.value = { id: '__ungrouped__', name: t('conn.noGroup') }
    groupMenuRef.value?.openAt(e.clientX, e.clientY)
  }

  // ── Rename group ──
  const showRenameGroupDialog = ref(false)

  function doRenameGroup() {
    if (!selectedGroup.value) return
    showRenameGroupDialog.value = true
  }

  function confirmRenameGroup(name: string) {
    if (!selectedGroup.value) return
    connectionStore.renameGroup(selectedGroup.value.id, name)
  }

  // ── Delete group ──
  const showDeleteGroupDialog = ref(false)

  function doDeleteGroup() {
    closeGroupMenu()
    if (!selectedGroup.value) return
    const connCount = connectionStore.connections.filter(c => c.groupId === selectedGroup.value!.id).length
    const childCount = connectionStore.groups.filter(cg => cg.parentId === selectedGroup.value!.id).length
    if (connCount === 0 && childCount === 0) {
      connectionStore.deleteGroup(selectedGroup.value.id, 'move-out')
      return
    }
    showDeleteGroupDialog.value = true
  }

  async function confirmDeleteGroup(connAction: 'delete-connections' | 'move-out') {
    if (selectedGroup.value) {
      const childAction = connAction === 'delete-connections' ? 'delete-all' : 'move-up'
      await connectionStore.deleteGroup(selectedGroup.value.id, connAction, childAction)
    }
    showDeleteGroupDialog.value = false
    selectedGroup.value = null
  }

  // ── New-connection dropdown ──
  const showNewConnMenu = ref(false)
  const newConnMenuRef = ref<InstanceType<typeof Menu> | null>(null)

  function closeNewConnMenu() {
    showNewConnMenu.value = false
  }

  function onNewConnSelect(cmd: string) {
    closeNewConnMenu()
    onNewConnCommand(cmd)
  }

  function onNewConnCommand(cmd: string) {
    if (cmd === 'new-connection') {
      if (searchQuery.value.trim()) {
        openNewFormFromSearch()
      } else {
        openNewForm()
      }
    } else if (cmd === 'new-group') {
      newGroupParentId.value = undefined
      showNewGroupDialog.value = true
    } else if (cmd === 'new-workspace') {
      openWorkspaceNameDialog({ title: t('workspace.newWorkspace') }).then(async result => {
        if (!result) return
        await createEmptyWorkspaceRecord(result.name, result.groupId)
      })
    } else if (cmd === 'netscan') {
      emit('openNetscan')
    } else if (cmd === 'import') {
      showImportDialog.value = true
    } else if (cmd === 'export') {
      showExportDialog.value = true
    }
  }

  // Open move-to dialog for a specific group (from StartTabContent)
  function openChangeGroupForGroup(groupId: string) {
    selectedGroup.value = connectionStore.groups.find(g => g.id === groupId) || null
    changeDialogMode.value = 'group'
    changeGroupTargetId.value = selectedGroup.value?.parentId
    showChangeGroupDialog.value = true
  }

  return {
    onContextMenu,
    onConnMoreClick,
    closeMenu,
    ctxTargets,
    onCtxConnect,
    onCtxConnectToWorkspace,
    onCtxCreateWorkspace,
    onCtxDelete,
    doEdit,
    showNewGroupDialog,
    newGroupParentId,
    groupTreeData,
    doNewConnInGroup,
    doNewGroup,
    selectedGroupParentId,
    confirmNewGroup,
    doChangeParentGroup,
    showChangeGroupDialog,
    changeGroupTargetId,
    showChangeNewGroupDialog,
    changeNewGroupParentId,
    openChangeGroupFor,
    confirmChangeGroup,
    confirmChangeNewGroup,
    onGroupContextMenu,
    closeGroupMenu,
    onEmptyAreaContextMenu,
    closeEmptyAreaMenu,
    onVirtualGroupContextMenu,
    showRenameGroupDialog,
    doRenameGroup,
    confirmRenameGroup,
    showDeleteGroupDialog,
    doDeleteGroup,
    confirmDeleteGroup,
    showNewConnMenu,
    newConnMenuRef,
    closeNewConnMenu,
    onNewConnSelect,
    openChangeGroupForGroup,
  }
}