import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { SaveConnections, LoadConnections } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime'
import type { ConnectionConfig, ConnectionGroup } from '../types/session'

export interface GroupTreeNode {
  group: ConnectionGroup
  connections: ConnectionConfig[]
  children: GroupTreeNode[]
}

export interface GroupedConnections {
  roots: GroupTreeNode[]
  ungrouped: ConnectionConfig[]
}

export const useConnectionStore = defineStore('connection', () => {
  const connections = ref<ConnectionConfig[]>([])
  const groups = ref<ConnectionGroup[]>([])
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      const data = await LoadConnections() as { groups?: ConnectionGroup[]; connections?: ConnectionConfig[] }
      groups.value = data.groups || []
      connections.value = (data.connections || []) as ConnectionConfig[]
    } catch (e) {
      console.error('Failed to load connections:', e)
    } finally {
      loading.value = false
    }
  }

  async function save() {
    try {
      await SaveConnections({
        groups: groups.value,
        connections: connections.value
      } as any)
    } catch (e) {
      console.error('Failed to save connections:', e)
    }
  }

  async function add(config: ConnectionConfig) {
    if (!config.id) {
      config.id = `conn-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
    }
    if (connections.value.some(c => c.id === config.id)) {
      return
    }
    connections.value.push(config)
    await save()
  }

  async function update(id: string, config: Partial<ConnectionConfig>) {
    const idx = connections.value.findIndex(c => c.id === id)
    if (idx >= 0) {
      connections.value[idx] = { ...connections.value[idx], ...config }
      await save()
    }
  }

  async function remove(id: string) {
    connections.value = connections.value.filter(c => c.id !== id)
    await save()
  }

  async function removeMany(ids: string[]) {
    const set = new Set(ids)
    connections.value = connections.value.filter(c => !set.has(c.id))
    await save()
  }

  // ── Group CRUD ──

  function generateGroupId(): string {
    return `grp-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
  }

  function uniqueGroupName(baseName: string, parentId: string | undefined, excludeId?: string): string {
    const siblings = groups.value.filter(g => g.parentId === parentId && g.id !== excludeId)
    if (!siblings.some(g => g.name === baseName)) return baseName
    let n = 1
    while (siblings.some(g => g.name === `${baseName} (${n})`)) n++
    return `${baseName} (${n})`
  }

  async function addGroup(name: string, parentId?: string): Promise<ConnectionGroup> {
    const uniqueName = uniqueGroupName(name, parentId)
    const group: ConnectionGroup = { id: generateGroupId(), name: uniqueName, parentId }
    groups.value.push(group)
    await save()
    return group
  }

  async function renameGroup(id: string, name: string) {
    const g = groups.value.find(g => g.id === id)
    if (g) {
      g.name = uniqueGroupName(name, g.parentId, id)
      await save()
    }
  }

  async function deleteGroup(id: string, connAction: 'delete-connections' | 'move-out', childAction: 'move-up' | 'delete-all' = 'move-up') {
    const targetGroup = groups.value.find(g => g.id === id)
    const parentId = targetGroup?.parentId

    // Handle child groups
    if (childAction === 'move-up') {
      for (const g of groups.value) {
        if (g.parentId === id) {
          g.parentId = parentId
        }
      }
    } else {
      // Cascade delete: collect all descendant group IDs
      const toDelete = new Set<string>()
      function collectDescendants(gid: string) {
        for (const g of groups.value) {
          if (g.parentId === gid) {
            toDelete.add(g.id)
            collectDescendants(g.id)
          }
        }
      }
      toDelete.add(id)
      collectDescendants(id)
      connections.value = connections.value.filter(c => {
        const gid = c.groupId
        return !gid || !toDelete.has(gid)
      })
      groups.value = groups.value.filter(g => !toDelete.has(g.id))
      await save()
      return
    }

    // Handle connections
    if (connAction === 'delete-connections') {
      connections.value = connections.value.filter(c => c.groupId !== id)
    } else {
      for (const c of connections.value) {
        if (c.groupId === id) {
          c.groupId = undefined
        }
      }
    }
    groups.value = groups.value.filter(g => g.id !== id)
    await save()
  }

  async function reparentGroup(groupId: string, newParentId: string | undefined) {
    // Prevent self-parenting and cycles
    if (newParentId === groupId) return
    if (newParentId && isDescendantOf(groupId, newParentId)) return

    const g = groups.value.find(g => g.id === groupId)
    if (!g) return

    // Compute new name BEFORE changing parent (check against new siblings)
    const newName = uniqueGroupName(g.name, newParentId, groupId)
    g.parentId = newParentId
    g.name = newName
    await save()
  }

  function isDescendantOf(ancestorId: string, targetId: string): boolean {
    const children = groups.value.filter(g => g.parentId === ancestorId)
    for (const child of children) {
      if (child.id === targetId) return true
      if (isDescendantOf(child.id, targetId)) return true
    }
    return false
  }

  function getChildGroups(parentId: string): ConnectionGroup[] {
    return groups.value.filter(g => g.parentId === parentId)
  }

  async function setConnectionGroup(connectionId: string, groupId: string | undefined) {
    const c = connections.value.find(c => c.id === connectionId)
    if (c) {
      c.groupId = groupId
      await save()
    }
  }

  async function setConnectionsGroup(connectionIds: string[], groupId: string | undefined) {
    for (const id of connectionIds) {
      const c = connections.value.find(c => c.id === id)
      if (c) {
        c.groupId = groupId
      }
    }
    await save()
  }

  // ── Derived: tree structure ──

  const groupedConnections = computed<GroupedConnections>(() => {
    const nodeMap = new Map<string, GroupTreeNode>()
    for (const g of groups.value) {
      nodeMap.set(g.id, { group: g, connections: [], children: [] })
    }

    // Build tree: identify roots vs children
    const roots: GroupTreeNode[] = []
    for (const g of groups.value) {
      const node = nodeMap.get(g.id)!
      if (g.parentId && nodeMap.has(g.parentId)) {
        nodeMap.get(g.parentId)!.children.push(node)
      } else {
        roots.push(node)
      }
    }

    // Assign connections to their group nodes
    const ungrouped: ConnectionConfig[] = []
    for (const c of connections.value) {
      if (c.groupId && nodeMap.has(c.groupId)) {
        nodeMap.get(c.groupId)!.connections.push(c)
      } else {
        ungrouped.push(c)
      }
    }

    // Recursive sort
    function sortNode(node: GroupTreeNode) {
      node.connections.sort((a, b) => a.name.localeCompare(b.name))
      node.children.sort((a, b) => a.group.name.localeCompare(b.group.name))
      node.children.forEach(sortNode)
    }
    roots.forEach(sortNode)
    roots.sort((a, b) => a.group.name.localeCompare(b.group.name))
    ungrouped.sort((a, b) => a.name.localeCompare(b.name))

    return { roots, ungrouped }
  })

  // Flat list of all group IDs (for Sidebar expand/collapse management)
  const allGroupIds = computed<string[]>(() => {
    const ids: string[] = []
    function collect(nodes: GroupTreeNode[]) {
      for (const node of nodes) {
        ids.push(node.group.id)
        collect(node.children)
      }
    }
    collect(groupedConnections.value.roots)
    return ids
  })

  // Listen for cross-window connection sync
  EventsOn('store:connections:changed', (data: { groups?: ConnectionGroup[]; connections?: ConnectionConfig[] }) => {
    if (data) {
      if (data.groups) groups.value = data.groups
      if (data.connections) connections.value = data.connections
    }
  })

  return {
    connections,
    groups,
    loading,
    load,
    save,
    add,
    update,
    remove,
    removeMany,
    addGroup,
    renameGroup,
    deleteGroup,
    reparentGroup,
    isDescendantOf,
    getChildGroups,
    setConnectionGroup,
    setConnectionsGroup,
    groupedConnections,
    allGroupIds,
  }
})
