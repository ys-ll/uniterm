import { ref, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { attachWebTerminal } from '../services/terminalManager'
import { attachBrowserSession, initWebTerminals, resyncWebTerminals, webTerminalBridge, type WebSession } from '../services/webTerminals'
import { isSharedTextSession } from './nativeCapabilities'
import { isWebMode } from './webMode'
import { waitForBrowserEvents } from './browserRuntime'

export const webTerminalError = ref('')
let dispose: (() => void) | undefined

/** Install after Pinia/settings, before mounting any session-creating UI. */
export function initBrowserTerminalIntegration(): () => void {
  if (dispose) return dispose
  const stopTerminals = initWebTerminals()
  const offs: (() => void)[] = []
  let stopped = false
  const report = (error: unknown) => {
    if (!stopped) webTerminalError.value = error instanceof Error ? error.message : String(error)
  }

  if (isWebMode()) {
    const panels = usePanelStore()
    const sessions = useSessionStore()
    const pending = new Set<string>()

    // All launchers (including container/K8s exec and Panel's retry path)
    // bind through this store. Mark the view immediately, then snapshot only
    // after the host reports connected. Never infer a canonical grid from UI
    // config or start a second session in response to a creation event.
    const attachConnectedPanel = async (panelId: string, id: string) => {
      if (pending.has(id) || sessions.sessions.get(id)?.web) return
      pending.add(id)
      try {
        await waitForBrowserEvents()
        if (stopped || sessions.sessions.get(id)?.web) return
        const session = (await webTerminalBridge.sessions()).find(s => s.id === id)
        // The catalog RPC may outlive a disconnected stream. Re-establish
        // delivery before taking the snapshot, then recheck the local view.
        await waitForBrowserEvents()
        if (stopped || panels.getPanel(panelId)?.sessionId !== id || sessions.sessions.get(id)?.web) return
        if (!session) throw new Error(`Text session is no longer available: ${id}`)
        sessions.attachSession(session)
        panels.updateStatus(panelId, session.status)
        await attachWebTerminal(session)
      } finally {
        pending.delete(id)
      }
    }

    const boundPanels = () => [...panels.panels.values()].map(p => ({ id: p.id, sessionId: p.sessionId, type: p.type, status: p.sessionId ? sessions.getStatus(p.sessionId) : '' }))
    const attachPanels = (entries = boundPanels()) => {
      for (const p of entries) {
        if (!p.sessionId || !isSharedTextSession(p.type)) continue
        panels.bindAttachedSession(p.id, p.sessionId)
        if (p.status === 'connected') void attachConnectedPanel(p.id, p.sessionId).catch(report)
      }
    }
    offs.push(watch(
      boundPanels, attachPanels,
      { immediate: true, flush: 'sync' },
    ))
    offs.push(Events.On('web:resync', () => {
      void waitForBrowserEvents().then(async () => {
        if (stopped) return
        try {
          await resyncWebTerminals()
          if (!stopped) webTerminalError.value = ''
        } finally {
          if (!stopped) attachPanels()
        }
      }).catch(report)
    }))
  }

  dispose = () => {
    if (stopped) return
    stopped = true
    offs.forEach(off => off())
    stopTerminals()
    dispose = undefined
  }
  return dispose
}

export async function attachBrowserTerminal(session: WebSession): Promise<string> {
  await waitForBrowserEvents()
  return attachBrowserSession(session)
}
