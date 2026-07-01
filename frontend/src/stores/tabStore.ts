import { defineStore } from 'pinia'
import { reactive, computed } from 'vue'
import type { Tab, TerminalTab, SettingsTab, WorkspaceTab, SFTPTab, RDPTab, VNCTab, SPICETab, DBTab, MonitorTab, StartTab, PanelLayout, LayoutNode } from '../types/workspace'
import { usePanelStore } from './panelStore'
import { t } from '../i18n'

const tabState = reactive<{
  tabs: Tab[]
  activeTabId: string | null
  aiLockedPanelId: string | null
  broadcastWorkspaceId: string | null
  tabNotifications: Record<string, boolean>
}>({
  tabs: [],
  activeTabId: null,
  aiLockedPanelId: null,
  broadcastWorkspaceId: null,
  tabNotifications: {}
})

let idCounter = 0
function genId(prefix: string): string {
  return `${prefix}-${Date.now()}-${++idCounter}`
}

function generateWorkspaceName(existingTabs: Tab[]): string {
  const base = t('workspace.defaultName')
  const existingNames = existingTabs.filter(t => t.type === 'workspace').map(t => t.name)
  if (!existingNames.includes(base)) return base
  let i = 2
  while (existingNames.includes(`${base} (${i})`)) i++
  return `${base} (${i})`
}

export const useTabStore = defineStore('tab', () => {
  const tabs = computed(() => tabState.tabs)
  const activeTabId = computed(() => tabState.activeTabId)
  const activeTab = computed(() =>
    tabState.tabs.find(t => t.id === tabState.activeTabId) || null
  )
  const aiLockedPanelId = computed(() => tabState.aiLockedPanelId)
  const broadcastWorkspaceId = computed(() => tabState.broadcastWorkspaceId)

  function toggleBroadcast(workspaceId: string) {
    if (tabState.broadcastWorkspaceId === workspaceId) {
      tabState.broadcastWorkspaceId = null
    } else {
      tabState.broadcastWorkspaceId = workspaceId
    }
  }

  function isBroadcasting(workspaceId: string): boolean {
    return tabState.broadcastWorkspaceId === workspaceId
  }

  // ── Create tabs ──

  function createTerminalTab(name: string, panelId: string): TerminalTab {
    const tab: TerminalTab = {
      type: 'terminal',
      id: genId('term-tab'),
      panelId,
      name
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  function createSettingsTab(name: string, panelId: string): SettingsTab {
    const tab: SettingsTab = {
      type: 'settings',
      id: genId('settings-tab'),
      panelId,
      name
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  function createSFPTab(name: string, panelId: string): SFTPTab {
    const tab: SFTPTab = {
      type: 'sftp',
      id: genId('sftp-tab'),
      panelId,
      name
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  function createFtpTab(name: string, panelId: string): SFTPTab {
    const tab: SFTPTab = {
      type: 'sftp',
      id: genId('ftp-tab'),
      panelId,
      name
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  function createRDPTab(name: string, panelId: string): RDPTab {
    const tab: RDPTab = {
      type: 'rdp',
      id: genId('rdp-tab'),
      panelId,
      name
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  function createVNCTab(name: string, panelId: string): VNCTab {
    const tab: VNCTab = {
      type: 'vnc',
      id: genId('vnc-tab'),
      panelId,
      name
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  function createSPICETab(name: string, panelId: string): SPICETab {
    const tab: SPICETab = {
      type: 'spice',
      id: genId('spice-tab'),
      panelId,
      name
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  function createDBTab(name: string, panelId: string): DBTab {
    const tab: DBTab = {
      type: 'database',
      id: genId('db-tab'),
      panelId,
      name
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  function createMonitorTab(name: string, panelId: string): MonitorTab {
    const tab: MonitorTab = {
      type: 'monitor',
      id: genId('monitor-tab'),
      panelId,
      name
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  function createStartTab(): StartTab {
    const tab: StartTab = {
      type: 'start',
      id: genId('start-tab'),
      name: t('startTab.defaultName'),
      viewMode: 'home'
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  function createWorkspaceTab(name: string, panelIds: string[], layout: PanelLayout): WorkspaceTab {
    const tab: WorkspaceTab = {
      type: 'workspace',
      id: genId('ws-tab'),
      name,
      panelIds: [...panelIds],
      layout,
      activePanelId: panelIds[0] || null
    }
    tabState.tabs.push(tab)
    tabState.activeTabId = tab.id
    return tab
  }

  // ── Close tab ──

  function closeTab(id: string): string[] {
    const idx = tabState.tabs.findIndex(t => t.id === id)
    if (idx === -1) return []
    const removed = tabState.tabs.splice(idx, 1)[0]

    if (tabState.activeTabId === id) {
      // Activate nearest tab (prefer right, then left)
      if (tabState.tabs.length > 0) {
        const newIdx = Math.min(idx, tabState.tabs.length - 1)
        tabState.activeTabId = tabState.tabs[newIdx].id
      } else {
        tabState.activeTabId = null
      }
    }

    // Clear AI lock if locked panel was in this tab
    const removedPanelIds: string[] = (() => {
      if (removed.type === 'start') return []
      if (removed.type === 'workspace') return removed.panelIds
      return [removed.panelId]
    })()

    if (tabState.aiLockedPanelId && removedPanelIds.includes(tabState.aiLockedPanelId)) {
      tabState.aiLockedPanelId = null
    }

    return removedPanelIds
  }

  // ── Activate / reorder / rename ──

  function setActiveTab(id: string) {
    tabState.activeTabId = id
    // Clear notification dot when user switches to this tab
    tabState.tabNotifications[id] = false
  }

  // ── Notification dots ──

  function markTabNotification(tabId: string) {
    tabState.tabNotifications[tabId] = true
  }

  function clearTabNotification(tabId: string) {
    tabState.tabNotifications[tabId] = false
  }

  function hasTabNotification(tabId: string): boolean {
    return !!tabState.tabNotifications[tabId]
  }

  function nextTab() {
    const idx = tabState.tabs.findIndex(t => t.id === tabState.activeTabId)
    if (idx < 0) return
    const next = tabState.tabs[(idx + 1) % tabState.tabs.length]
    tabState.activeTabId = next.id
  }

  function prevTab() {
    const idx = tabState.tabs.findIndex(t => t.id === tabState.activeTabId)
    if (idx < 0) return
    const next = tabState.tabs[(idx - 1 + tabState.tabs.length) % tabState.tabs.length]
    tabState.activeTabId = next.id
  }

  function getActivePanelId(): string | null {
    const t = tabState.tabs.find(t => t.id === tabState.activeTabId)
    if (!t) return null
    if (t.type === 'workspace') return t.activePanelId || t.panelIds[0] || null
    if ('panelId' in t) return (t as any).panelId as string
    return null
  }

  function moveTab(fromIdx: number, toIdx: number) {
    const [t] = tabState.tabs.splice(fromIdx, 1)
    tabState.tabs.splice(toIdx, 0, t)
  }

  function renameTab(id: string, name: string) {
    const t = tabState.tabs.find(x => x.id === id)
    if (t) t.name = name
    // Sync panel title for terminal tabs
    if (t && t.type === 'terminal') {
      const panelStore = usePanelStore()
      const panel = panelStore.getPanel(t.panelId)
      if (panel) panel.title = name
    }
  }

  // ── Workspace panel management ──

  function setActivePanel(tabId: string, panelId: string) {
    const t = tabState.tabs.find(x => x.id === tabId)
    if (t && t.type === 'workspace') {
      t.activePanelId = panelId
    }
  }

  function updateWorkspaceLayout(tabId: string, layout: PanelLayout) {
    const t = tabState.tabs.find(x => x.id === tabId)
    if (t && t.type === 'workspace') {
      t.layout = layout
      // Sync panelIds from layout
      t.panelIds = collectPanelIds(layout.root)
    }
  }

  // ── Merge: two terminal tabs → workspace tab ──

  function mergeToWorkspace(
    terminalTabAId: string,
    terminalTabBId: string,
    direction: 'horizontal' | 'vertical',
    insertBefore: boolean
  ): WorkspaceTab | null {
    const idxA = tabState.tabs.findIndex(t => t.id === terminalTabAId)
    const idxB = tabState.tabs.findIndex(t => t.id === terminalTabBId)
    if (idxA === -1 || idxB === -1) return null

    const tabA = tabState.tabs[idxA] as TerminalTab
    const tabB = tabState.tabs[idxB] as TerminalTab
    if (tabA.type !== 'terminal' || tabB.type !== 'terminal') return null

    const children = insertBefore
      ? [{ type: 'leaf' as const, panelId: tabA.panelId }, { type: 'leaf' as const, panelId: tabB.panelId }]
      : [{ type: 'leaf' as const, panelId: tabB.panelId }, { type: 'leaf' as const, panelId: tabA.panelId }]

    const layout: PanelLayout = {
      root: {
        type: 'split',
        direction,
        sizes: [0.5, 0.5],
        children
      }
    }

    const workspaceTab: WorkspaceTab = {
      type: 'workspace',
      id: genId('ws-tab'),
      name: generateWorkspaceName(tabState.tabs),
      panelIds: [tabA.panelId, tabB.panelId],
      layout,
      activePanelId: tabB.panelId
    }

    // Remove in reverse order to preserve indices
    const removeIdxA = tabState.tabs.findIndex(t => t.id === terminalTabAId)
    const removeIdxB = tabState.tabs.findIndex(t => t.id === terminalTabBId)
    if (removeIdxA > removeIdxB) {
      tabState.tabs.splice(removeIdxA, 1)
      tabState.tabs.splice(removeIdxB, 1)
    } else {
      tabState.tabs.splice(removeIdxB, 1)
      tabState.tabs.splice(removeIdxA, 1)
    }

    // Re-associate panels with the new workspace tab
    const panelStore = usePanelStore()
    panelStore.movePanelToTab(tabA.panelId, workspaceTab.id)
    panelStore.movePanelToTab(tabB.panelId, workspaceTab.id)

    // Insert workspace tab at the position of the first removed tab
    const insertIdx = Math.min(removeIdxA, removeIdxB)
    tabState.tabs.splice(insertIdx, 0, workspaceTab)
    tabState.activeTabId = workspaceTab.id

    return workspaceTab
  }

  // ── Merge: terminal tab → existing workspace tab ──

  function addPanelToWorkspaceTab(
    terminalTabId: string,
    workspaceTabId: string,
    targetPanelId: string,
    direction: 'horizontal' | 'vertical',
    insertBefore: boolean
  ) {
    const termIdx = tabState.tabs.findIndex(t => t.id === terminalTabId)
    const wsTab = tabState.tabs.find(t => t.id === workspaceTabId)
    if (termIdx === -1 || !wsTab || wsTab.type !== 'workspace') return

    const termTab = tabState.tabs[termIdx] as TerminalTab
    if (termTab.type !== 'terminal') return

    const newPanelId = termTab.panelId

    // Remove terminal tab
    tabState.tabs.splice(termIdx, 1)

    // Add panel to workspace
    wsTab.panelIds.push(newPanelId)
    wsTab.layout = {
      root: insertPanelIntoLayout(wsTab.layout.root, targetPanelId, newPanelId, direction, insertBefore)
    }
    wsTab.activePanelId = newPanelId
    tabState.activeTabId = workspaceTabId
  }

  // ── Detach: panel from workspace ──
  // Returns the detached panelId; caller is responsible for creating a terminal
  // tab with the correct name. Handles workspace cleanup (auto-convert to
  // terminal tab when 1 panel remains, close when empty).

  function removePanelFromWorkspaceTab(workspaceTabId: string, panelId: string): string | null {
    const wsTab = tabState.tabs.find(t => t.id === workspaceTabId)
    if (!wsTab || wsTab.type !== 'workspace') return null

    const wsIdx = tabState.tabs.findIndex(t => t.id === workspaceTabId)

    // Remove panel from workspace
    wsTab.panelIds = wsTab.panelIds.filter(id => id !== panelId)
    if (wsTab.activePanelId === panelId) {
      wsTab.activePanelId = wsTab.panelIds[0] || null
    }

    // Keep AI lock when panel is detached from workspace — the
    // locked session should remain locked regardless of whether
    // the panel is in a workspace or standalone tab.

    if (wsTab.panelIds.length === 1) {
      // Auto-convert remaining workspace to terminal tab
      const panelStore = usePanelStore()
      const remainingPanelId = wsTab.panelIds[0]
      const remainingPanel = panelStore.getPanel(remainingPanelId)
      const convertedTab: TerminalTab = {
        type: 'terminal',
        id: genId('term-tab'),
        panelId: remainingPanelId,
        name: remainingPanel?.title || 'Terminal'
      }
      tabState.tabs.splice(wsIdx, 1, convertedTab)
      panelStore.movePanelToTab(remainingPanelId, convertedTab.id)
      tabState.activeTabId = convertedTab.id
    } else if (wsTab.panelIds.length === 0) {
      tabState.tabs.splice(wsIdx, 1)
    } else {
      wsTab.layout = { root: removeFromLayout(wsTab.layout.root, panelId) }
    }

    return panelId
  }

  // ── Workspace internal: move panel to new position ──

  function movePanelInWorkspace(
    workspaceTabId: string,
    panelId: string,
    targetPanelId: string,
    direction: 'horizontal' | 'vertical',
    insertBefore: boolean
  ) {
    const wsTab = tabState.tabs.find(t => t.id === workspaceTabId)
    if (!wsTab || wsTab.type !== 'workspace' || panelId === targetPanelId) return

    // Remove panel from old position
    let tempLayout = { root: removeFromLayout(wsTab.layout.root, panelId) }
    // Insert at new position
    tempLayout = {
      root: insertPanelIntoLayout(tempLayout.root, targetPanelId, panelId, direction, insertBefore)
    }
    wsTab.layout = tempLayout
    wsTab.panelIds = collectPanelIds(tempLayout.root)
  }

  // ── AI lock ──

  function setAILockedPanel(panelId: string | null) {
    tabState.aiLockedPanelId = panelId
  }

  function getAILockedPanel(): string | null {
    return tabState.aiLockedPanelId
  }

  // ── Layout helpers ──

  function collectPanelIds(node: LayoutNode): string[] {
    if (node.type === 'leaf') return node.panelId ? [node.panelId] : []
    return node.children.flatMap(collectPanelIds)
  }

  function hasPanelInNode(node: LayoutNode, panelId: string): boolean {
    if (node.type === 'leaf') return node.panelId === panelId
    return node.children.some(child => hasPanelInNode(child, panelId))
  }

  function insertPanelIntoLayout(
    node: LayoutNode,
    targetId: string,
    newId: string,
    direction: 'horizontal' | 'vertical',
    before: boolean
  ): LayoutNode {
    if (node.type === 'leaf') {
      if (node.panelId === targetId) {
        const children = before
          ? [{ type: 'leaf' as const, panelId: newId }, node]
          : [node, { type: 'leaf' as const, panelId: newId }]
        return { type: 'split', direction, sizes: [0.5, 0.5], children }
      }
      return node
    }
    const hasTarget = node.children.some(child => hasPanelInNode(child, targetId))
    if (hasTarget) {
      return {
        ...node,
        children: node.children.map(child =>
          insertPanelIntoLayout(child, targetId, newId, direction, before)
        )
      }
    }
    return node
  }

  function removeFromLayout(node: LayoutNode, panelId: string): LayoutNode {
    if (node.type === 'leaf') {
      return node.panelId === panelId
        ? { type: 'leaf' as const, panelId: '' }
        : node
    }
    const newChildren = node.children
      .map(child => removeFromLayout(child, panelId))
      .filter(child => !(child.type === 'leaf' && child.panelId === ''))

    if (newChildren.length === 0) {
      return { type: 'leaf' as const, panelId: '' }
    }
    if (newChildren.length === 1) {
      return newChildren[0]
    }
    return { ...node, children: newChildren }
  }

  function updateNodeInTree(
    node: LayoutNode,
    oldNode: LayoutNode,
    newNode: LayoutNode
  ): LayoutNode {
    if (node === oldNode) return newNode
    if (node.type === 'leaf') return node
    return {
      ...node,
      children: node.children.map(child => updateNodeInTree(child, oldNode, newNode))
    }
  }

  return {
    tabs,
    activeTabId,
    activeTab,
    aiLockedPanelId,
    createTerminalTab,
    createSettingsTab,
    createSFPTab,
    createFtpTab,
    createRDPTab,
    createVNCTab,
    createSPICETab,
    createDBTab,
    createMonitorTab,
    createStartTab,
    createWorkspaceTab,
    closeTab,
    setActiveTab,
    nextTab,
    prevTab,
    getActivePanelId,
    moveTab,
    renameTab,
    setActivePanel,
    updateWorkspaceLayout,
    mergeToWorkspace,
    addPanelToWorkspaceTab,
    removePanelFromWorkspaceTab,
    movePanelInWorkspace,
    setAILockedPanel,
    getAILockedPanel,
    broadcastWorkspaceId,
    toggleBroadcast,
    isBroadcasting,
    markTabNotification,
    clearTabNotification,
    hasTabNotification,
    // Expose helpers for components
    collectPanelIds,
    insertPanelIntoLayout,
    removeFromLayout,
    updateNodeInTree
  }
})
