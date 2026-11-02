// Restart tab snapshot (issue #937-4). Two halves:
//
// 1. Record — a debounced watcher over the tab/panel stores persists the
//    currently-open tabs into local_state.json. Watching (instead of hooking
//    quit) is deliberate: Alt+F4 / the native title bar give the frontend no
//    reliable close notification, so "record on close" is approximated by
//    "record continuously". Host references are connectionIds — credentials
//    re-resolve from the connection store at restore time, so no secret is
//    ever written to the snapshot.
// 2. Restore — re-opens each saved tab through the existing connect flows
//    (launchConnection specs / connectTerminalSession / openSavedWorkspace).
import { watch } from 'vue'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useConnectionStore } from '../stores/connectionStore'
import { useLocalStateStore } from '../stores/localStateStore'
import { useSettingsStore } from '../stores/settingsStore'
import { launchConnection } from './connectionLauncher'
import { openSavedWorkspace, serializeWorkspaceState } from './savedWorkspace'
import type { ConnectionConfig, MemberConnectResult, MemberConnectStatus } from '../types/session'
import type { SavedTabEntry, SavedTabsSnapshot } from '../types/tabSnapshot'
import type { WorkspaceTab } from '../types/workspace'

// Connect primitives the restore flow needs from App.vue (the generic
// terminal path must wait for the real xterm size, so it lives there).
export interface RestoreDeps {
  connectTerminal(config: ConnectionConfig, persist: boolean): Promise<MemberConnectResult | void> | MemberConnectResult | void
  connectMember(config: ConnectionConfig, workspaceTabId: string, persist: boolean): Promise<{ status: MemberConnectStatus; panelId?: string }>
}

// Serialize the currently-open tabs. Tabs without a session-bearing panel
// (start/settings) are skipped; workspaces whose members all vanished too.
export function buildTabSnapshot(): SavedTabsSnapshot {
  const tabStore = useTabStore()
  const panelStore = usePanelStore()
  const tabs: SavedTabEntry[] = []
  for (const tab of tabStore.tabs) {
    if (tab.type === 'start' || tab.type === 'settings' || tab.type === 'netscan') continue
    if (tab.type === 'workspace') {
      const ws = tab as WorkspaceTab
      const serialized = serializeWorkspaceState(ws)
      if (!serialized) continue
      tabs.push({
        kind: 'workspace',
        name: tab.name,
        savedWorkspaceId: tab.savedWorkspaceId,
        members: serialized.members,
        layout: serialized.layout,
      })
      continue
    }
    if (!('panelId' in tab)) continue
    const panel = panelStore.getPanel((tab as any).panelId)
    if (!panel?.config) continue
    tabs.push({
      kind: tab.type,
      name: tab.name,
      connectionId: panel.config.id || '',
      type: panel.type,
      shellPath: panel.config.shellPath || '',
    })
  }
  return { tabs }
}

// Persist the snapshot (debounced) whenever tabs/panels change. Installed
// once from App.vue's onMounted, after the stores are loaded.
export function installTabSnapshotSaver() {
  const tabStore = useTabStore()
  const panelStore = usePanelStore()
  const localStateStore = useLocalStateStore()
  let timer: ReturnType<typeof setTimeout> | null = null
  const schedule = () => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = null
      localStateStore.update({ lastTabsSnapshot: buildTabSnapshot() })
    }, 1500)
  }
  // Deep on tabs: renames, workspace panel membership, lock flags…
  watch(() => tabStore.tabs, schedule, { deep: true })
  // Panels: titles and connectionId binding land on the panel, not the tab.
  watch(
    () => [...panelStore.panels.values()].map(p => `${p.id}:${p.title}:${p.config?.id ?? ''}`).join('|'),
    schedule
  )
}

// Settings load in parallel with mounting. Wait before reading the startup
// preference so a saved false cannot be mistaken for the default true (#1292).
export async function maybeRestoreLastTabs(
  confirmRestore: (count: number) => Promise<unknown>,
  deps: RestoreDeps,
): Promise<void> {
  const settingsStore = useSettingsStore()
  const localStateStore = useLocalStateStore()
  if (!settingsStore.loaded) {
    await new Promise<void>(resolve => {
      watch(() => settingsStore.loaded, () => resolve(), { once: true })
    })
  }
  if (!settingsStore.settings.restoreTabsPrompt) return
  const snapshot = localStateStore.state.lastTabsSnapshot
  if (!snapshot?.tabs?.length) return
  try {
    await confirmRestore(snapshot.tabs.length)
  } catch {
    // A decline means "don't offer these tabs again". The saver records a
    // new snapshot when the user opens or changes tabs.
    await localStateStore.update({ lastTabsSnapshot: null })
    return
  }
  await restoreTabsSnapshot(snapshot, deps)
}

// Re-open every saved tab in order. Entries that cannot be resolved anymore
// (connection deleted, self-contained panel without enough info) are skipped
// silently — restore is best-effort, mirroring openSavedWorkspace's policy.
export async function restoreTabsSnapshot(snapshot: SavedTabsSnapshot, deps: RestoreDeps): Promise<void> {
  const connectionStore = useConnectionStore()
  for (const entry of snapshot.tabs) {
    try {
      if (entry.kind === 'workspace') {
        const cfg: ConnectionConfig = {
          id: entry.savedWorkspaceId || '',
          name: entry.name,
          type: 'workspace',
          host: '',
          port: 0,
          user: '',
          authType: 'password',
          workspaceMembers: entry.members,
          workspaceLayout: entry.layout ?? undefined,
        }
        await openSavedWorkspace(cfg, { connectMember: deps.connectMember })
        continue
      }
      const host = entry.connectionId
        ? connectionStore.connections.find(c => c.id === entry.connectionId)
        : undefined
      if (host) {
        // Title the tab with the name the user renamed it to; the launcher
        // titles tabs from config.name (persist:false keeps the store untouched).
        await launchConnection({ ...host, name: entry.name || host.name }, { persist: false, connectTerminal: deps.connectTerminal })
        continue
      }
      // Self-contained local/wsl panel: rebuild from type + shellPath.
      if (entry.type === 'local' || entry.type === 'wsl') {
        const cfg: ConnectionConfig = {
          id: '',
          name: entry.name,
          type: entry.type as ConnectionConfig['type'],
          host: '',
          port: 0,
          user: '',
          authType: 'password',
          shellPath: entry.shellPath,
        }
        await deps.connectTerminal(cfg, false)
      }
    } catch (e) {
      console.warn(`Failed to restore tab "${entry.name}":`, e)
    }
  }
}
