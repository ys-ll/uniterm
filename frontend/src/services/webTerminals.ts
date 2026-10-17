import { usePanelStore } from '../stores/panelStore'
import { useTabStore } from '../stores/tabStore'
import { useSessionStore } from '../stores/sessionStore'
import { useConnectionStore } from '../stores/connectionStore'
import { attachWebTerminal, disposeTerminal } from './terminalManager'
import { isWebMode } from '../utils/webMode'
import type { WebSession } from './webTerminalBridge'
import { CloseSession } from '../../bindings/github.com/ys-ll/uniterm/app'
export { initWebTerminals, resyncWebTerminals, refreshWebTerminal, disposeWebTerminals, terminalSharingErrors } from './terminalManager'
export { webTerminalBridge } from './webTerminalBridge'
export type { WebSession } from './webTerminalBridge'

/** Call after SSE's ready frame, instead of saved-tab restoration/SessionStart.
 * Existing views are reused; closing/reloading this page never closes a PTY. */
export async function attachBrowserSession(session: WebSession): Promise<string> {
  if (!isWebMode()) throw new Error('Browser attachment is browser-only')
  const panels = usePanelStore(), tabs = useTabStore()
  const existing = [...panels.panels.values()].find(p => p.sessionId === session.id)
  if (existing) {
    if (existing.tabId) tabs.setActiveTab(existing.tabId)
    return existing.id
  }
  const panel = panels.createPanel(null, session.type)
  panels.updateTitle(panel.id, session.title)
  panels.bindAttachedSession(panel.id, session.id)
  panels.updateStatus(panel.id, session.status)
  useSessionStore().attachSession(session)
  if (session.connectionId) {
    const connections = useConnectionStore()
    // Refresh restoration precedes App.onMounted's store load. Keep this view
    // out of the tab/sidebar until its saved reference has been resolved.
    if (!connections.connections.some(c => c.id === session.connectionId)) await connections.load()
    const current = panels.getPanel(panel.id)
    if (current?.sessionId !== session.id) return panel.id
    const config = connections.connections.find(c => c.id === session.connectionId)
    current.config = config ? JSON.parse(JSON.stringify(config)) : null
  }
  const tab = tabs.createTerminalTab(panel.title, panel.id)
  panels.movePanelToTab(panel.id, tab.id)
  await attachWebTerminal(session)
  return panel.id
}

/** App.vue closePanel calls this BEFORE its CloseSession/dispose/log path.
 * true means local view removal handled here; shell still removes tab/layout. */
export function detachBrowserPanel(panelId: string): boolean {
  if (!isWebMode()) return false
  const panels = usePanelStore()
  const panel = panels.getPanel(panelId)
  if (!panel || !panels.isAttachedPanel(panelId)) return false
  const id = panel.sessionId
  panels.removePanel(panelId)
  if (id && ![...panels.panels.values()].some(p => p.sessionId === id)) {
    disposeTerminal(id)
    useSessionStore().removeSession(id)
  }
  return true
}

/** Explicit Disconnect action; never called by view/tab cleanup. */
export async function terminateBrowserSession(panelId: string): Promise<void> {
  const store = usePanelStore()
  const panel = store.getPanel(panelId)
  if (!isWebMode() || !store.isAttachedPanel(panelId) || !panel?.sessionId) return
  await CloseSession(panel.sessionId)
}
