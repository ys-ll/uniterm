import {
  CreateSession,
  CloseSession,
  DuplicateSSHChannel,
  K8sExecSession,
  ContainerExecSession,
  SessionStart,
} from '../../bindings/github.com/ys-ll/uniterm/app'
import { usePanelStore } from '../stores/panelStore'
import { useTabStore } from '../stores/tabStore'
import { useSessionStore } from '../stores/sessionStore'
import { waitForTerminalSize } from '../services/terminalManager'
import { fileTransferProto } from '../utils/fileTransferUtils'
import { shouldCloseSession } from '../utils/nativeCapabilities'
import type { ConnectionConfig } from '../types/session'

// The session-type argument to CreateSession isn't always tab.config.type:
// a file-transfer (sftp) tab may share an SSH connection whose config.type is
// 'ssh' but whose session must be created as 'sftp' (per the connection's
// fileTransferProto preference). Everything else — including the whole
// database family — already carries its session type in config.type.
function resolveSessionType(tabType: string, config: any): string {
  if (tabType === 'sftp') {
    return config?.type === 'ssh' ? fileTransferProto(config) : config?.type
  }
  return config?.type
}

// Tab kinds whose duplicates are plain tab-shaped (session already bound by
// the shared flow). Terminal tabs are special (workspace embedding + PTY size
// wait) and keep their own branch.
const TAB_BACKED_DUPLICATE_TYPES = ['sftp', 'database', 'mongodb', 'redis', 'elasticsearch']

/**
 * Duplicate a session/tab. Shared by the tab context menu ("复制会话") and the
 * keyboard shortcut (duplicateSession) so both paths behave identically.
 *
 * The new tab or workspace panel is created right after the session is bound
 * but BEFORE it is started, so the duplicate appears immediately while the
 * SSH/PTY handshake runs in the background.
 */
export function useDuplicateSession() {
  const panelStore = usePanelStore()
  const tabStore = useTabStore()
  const sessionStore = useSessionStore()

  async function duplicateSession(
    tab: any,
    targetWorkspace?: { workspaceId: string; targetPanelId?: string },
  ) {
    if (!tab || !('panelId' in tab)) return
    const panel = panelStore.getPanel(tab.panelId)
    if (!panel) return

    // k8s tab has no backend session; it connects itself on mount from
    // connectionId + namespace. Duplicate = a fresh panel + K8s tab reusing the
    // same connection (a new independent session), matching other tab types.
    if (tab.type === 'k8s') {
      const newPanel = panelStore.createPanel(panel.config, 'k8s')
      panelStore.updateTitle(newPanel.id, panel.title)
      const newTab = tabStore.createTab('k8s', newPanel.title, newPanel.id, { connectionId: tab.connectionId, connId: null, namespace: tab.namespace || '' })
      panelStore.movePanelToTab(newPanel.id, newTab.id)
      return
    }

    const newPanel = panelStore.createPanel(panel.config, panel.type)
    panelStore.updateTitle(newPanel.id, panel.title)

    // Create + bind the session BEFORE mounting the tab, so the terminal has a
    // sessionId on first mount. Mounting first (empty sessionId) leaves the
    // shared terminal keyed by '' and bindSession's later id change can't
    // transfer it (the watch skips when oldId is falsy), so server output is
    // dropped until an incidental resize rebuilds the reference.
    let info
    let config: ConnectionConfig | undefined
    if (panel.config) {
      try {
        if (panel.type === 'container-exec') {
          // Exec panels can't be rebuilt via CreateSession (no such type); re-dial the exec stream.
          const c = panel.config
          info = c.containerExecNamespace
            ? await K8sExecSession(c.containerExecConnId, c.containerExecNamespace, c.containerExecContainerId, c.containerExecContainerName)
            : await ContainerExecSession(c.containerExecConnId, c.containerExecContainerId, c.containerExecShell || 'sh')
          panelStore.bindSession(newPanel.id, info.id)
          sessionStore.initSession(info.id)
          sessionStore.updateStatus(info.id, 'connected')
        } else {
          const sessionType = resolveSessionType(tab.type, panel.config)
          config = {
            ...panel.config,
            initialCols: 0,
            initialRows: 0,
          }
          info = await CreateSession(sessionType, config)
          panelStore.bindSession(newPanel.id, info.id)
          sessionStore.initSession(info.id)
        }
      } catch (e) {
        console.error('Failed to duplicate session:', e)
        return
      }
    }

    // Mount the duplicate now that the session is bound but BEFORE it is
    // started, either beside the source panel or in a standalone tab.
    let newTab
    if (tab.type === 'terminal') {
      const addedToWorkspace = targetWorkspace
        ? tabStore.addNewPanelToWorkspace(
            targetWorkspace.workspaceId,
            newPanel.id,
            targetWorkspace.targetPanelId,
          )
        : false
      if (addedToWorkspace) {
        panelStore.movePanelToTab(newPanel.id, targetWorkspace!.workspaceId)
      } else {
        newTab = tabStore.createTerminalTab(newPanel.title, newPanel.id)
      }
    } else if (TAB_BACKED_DUPLICATE_TYPES.includes(tab.type)) {
      newTab = tabStore.createTab(tab.type, newPanel.title, newPanel.id)
    } else {
      return
    }
    if (newTab) panelStore.movePanelToTab(newPanel.id, newTab.id)

    // Start the connection after the tab is visible. The terminal has now
    // mounted (with its sessionId bound), so waitForTerminalSize resolves with
    // the real size instead of blocking forever on a terminal that didn't exist
    // yet.
    if (tab.type === 'terminal' && info && config) {
      try {
        const size = await waitForTerminalSize(info.id)
        if (size.cols > 0 && size.rows > 0) {
          config.initialCols = size.cols
          config.initialRows = size.rows
        }
        await SessionStart(info.id, config).catch((e) => {
          console.error('Failed to start duplicated session:', e)
          if (shouldCloseSession(panel.type)) CloseSession(info.id).catch(() => {})
        })
      } catch (e) {
        console.error('Failed to duplicate session:', e)
      }
    }
  }

  /**
   * Duplicate an SSH terminal tab as a CHANNEL clone (issue #983,
   * Xshell-style): the new tab opens a fresh session channel on the source
   * session's already-authenticated SSH client — no re-dial, no re-auth, no
   * repeated 2FA/verification code on jump hosts. Only available while the
   * source session is connected; the backend re-validates (race guard).
   */
  async function duplicateChannel(
    tab: any,
    targetWorkspace?: { workspaceId: string; targetPanelId?: string },
  ) {
    if (!tab || !('panelId' in tab)) return
    const panel = panelStore.getPanel(tab.panelId)
    if (!panel || panel.type !== 'ssh' || !panel.sessionId) return
    if (sessionStore.getStatus(panel.sessionId) !== 'connected') return

    let info
    try {
      const config = { ...panel.config, initialCols: 0, initialRows: 0 }
      info = await DuplicateSSHChannel(panel.sessionId, config)
    } catch (e) {
      console.error('Failed to duplicate channel:', e)
      return
    }

    const newPanel = panelStore.createPanel(panel.config, 'ssh')
    panelStore.updateTitle(newPanel.id, panel.title)
    panelStore.bindSession(newPanel.id, info.id)
    sessionStore.initSession(info.id)
    sessionStore.updateStatus(info.id, 'connecting')

    // Same mount flow as duplicateSession: standalone tab, or embedded
    // beside the source panel when a workspace target was given.
    if (targetWorkspace) {
      const addedToWorkspace = tabStore.addNewPanelToWorkspace(
        targetWorkspace.workspaceId,
        newPanel.id,
        targetWorkspace.targetPanelId,
      )
      if (addedToWorkspace) {
        panelStore.movePanelToTab(newPanel.id, targetWorkspace.workspaceId)
        // Channel attach runs after the xterm mounted and measured its size.
        try {
          const size = await waitForTerminalSize(info.id)
          const config = {
            ...panel.config,
            initialCols: size.cols > 0 ? size.cols : 0,
            initialRows: size.rows > 0 ? size.rows : 0,
          }
          await SessionStart(info.id, config).catch((e: any) => {
            console.error('Failed to start channel clone:', e)
            if (shouldCloseSession('ssh')) CloseSession(info.id).catch(() => {})
          })
        } catch (e) {
          console.error('Failed to start channel clone:', e)
        }
        return
      }
    }
    const newTab = tabStore.createTerminalTab(newPanel.title, newPanel.id)
    panelStore.movePanelToTab(newPanel.id, newTab.id)

    try {
      const size = await waitForTerminalSize(info.id)
      const config = {
        ...panel.config,
        initialCols: size.cols > 0 ? size.cols : 0,
        initialRows: size.rows > 0 ? size.rows : 0,
      }
      await SessionStart(info.id, config).catch((e: any) => {
        console.error('Failed to start channel clone:', e)
        if (shouldCloseSession('ssh')) CloseSession(info.id).catch(() => {})
      })
    } catch (e) {
      console.error('Failed to start channel clone:', e)
    }
  }

  return { duplicateSession, duplicateChannel }
}
