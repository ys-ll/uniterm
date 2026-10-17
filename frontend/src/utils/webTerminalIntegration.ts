import { ref, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useTabStore } from '../stores/tabStore'
import { attachWebTerminal } from '../services/terminalManager'
import { attachBrowserSession, initWebTerminals, resyncWebTerminals, webTerminalBridge, type WebSession } from '../services/webTerminals'
import { isSharedTextSession } from './nativeCapabilities'
import { isWebMode } from './webMode'
import { waitForBrowserEvents } from './browserRuntime'

export const webTerminalError = ref('')
let dispose: (() => void) | undefined
let restoration: Promise<boolean> | undefined

interface BrowserTerminalViews {
  sessionIds: string[]
  activeSessionId: string | null
}

const isSessionId = (id: unknown): id is string => typeof id === 'string' && !!id && !id.startsWith('pending:')

/** App waits for the one startup restoration before deciding to show the picker. */
export function restoreBrowserTerminalViews(): Promise<boolean> {
  return restoration ?? Promise.resolve(false)
}

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
    const connecting = new Set<string>()

    // All launchers (including container/K8s exec and Panel's retry path)
    // bind through this store. Snapshot connecting views too: SSH authentication
    // prompts need a visible, writable terminal. Never infer a canonical grid from UI
    // config or start a second session in response to a creation event.
    const attachLivePanel = async (panelId: string, id: string) => {
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
        if (session.status !== 'connecting' && session.status !== 'connected') return
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
        if (connecting.has(p.sessionId) || p.status === 'connected') void attachLivePanel(p.id, p.sessionId).catch(report)
      }
    }
    offs.push(watch(
      boundPanels, attachPanels,
      { immediate: true, flush: 'sync' },
    ))
    // initSession also uses "connecting" before SessionStart's native attach
    // ACK. Only a host event proves the authority is ready to snapshot.
    offs.push(Events.On('session:status', ev => {
      const { id, status } = ev.data as { id: string; status: string }
      if (status === 'connecting') connecting.add(id)
      else connecting.delete(id)
      attachPanels()
    }))
    offs.push(Events.On('session:closed', ev => connecting.delete((ev.data as { id: string }).id)))
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

    const tabs = useTabStore()
    const key = `uniterm:web:${window.__UNITERM_WEB__!.instanceId}:terminals`
    let saved: BrowserTerminalViews = { sessionIds: [], activeSessionId: null }
    try {
      const value = JSON.parse(sessionStorage.getItem(key) || 'null')
      if (Array.isArray(value?.sessionIds)) saved = {
        sessionIds: [...new Set<string>(value.sessionIds.filter(isSessionId))],
        activeSessionId: isSessionId(value.activeSessionId) ? value.activeSessionId : null,
      }
    } catch { /* Unavailable/corrupt tab storage starts with an empty view list. */ }

    const currentViews = (): BrowserTerminalViews => {
      const ids = tabs.tabs.flatMap(tab => tab.type === 'terminal' ? [tab.panelId] : tab.type === 'workspace' ? tab.panelIds : [])
        .map(id => panels.getPanel(id))
        .filter(p => p && isSharedTextSession(p.type))
        .map(p => p!.sessionId).filter(isSessionId)
      const activeId = panels.getPanel(tabs.getActivePanelId() || '')?.sessionId
      return { sessionIds: [...new Set(ids)], activeSessionId: activeId && ids.includes(activeId) ? activeId : null }
    }
    let restored = false
    let leaving = false
    let preserveSaved = true
    const saveViews = (views: BrowserTerminalViews) => {
      if (!restored || stopped || leaving || preserveSaved && !views.sessionIds.length) return
      try {
        sessionStorage.setItem(key, JSON.stringify(views))
        preserveSaved = false
      } catch { /* Keep the current views when tab storage is unavailable. */ }
    }
    // Never let startup's empty stores or teardown replace the saved IDs.
    offs.push(watch(currentViews, saveViews, { flush: 'sync' }))
    const onPageHide = () => { leaving = true }
    window.addEventListener('pagehide', onPageHide)
    offs.push(() => window.removeEventListener('pagehide', onPageHide))

    restoration = (async () => {
      try {
        if (saved.sessionIds.length) {
          await waitForBrowserEvents()
          if (stopped || leaving) return false
          const catalog = new Map((await webTerminalBridge.sessions()).filter(s => isSharedTextSession(s.type)).map(s => [s.id, s]))
          await waitForBrowserEvents()
          if (stopped || leaving) return false
          await Promise.all(saved.sessionIds.map(async id => {
            const session = catalog.get(id)
            if (session) await attachBrowserTerminal(session).catch(report)
          }))
          const active = [...panels.panels.values()].find(p => p.sessionId === saved.activeSessionId && p.tabId)
          if (!stopped && !leaving && active) tabs.setActiveTab(active.tabId)
        }
        preserveSaved = false // A successful catalog lookup can prune closed IDs.
      } catch (error) { report(error) }
      if (stopped || leaving) return false
      restored = true
      const views = currentViews()
      saveViews(views)
      return views.sessionIds.length > 0
    })()
  }

  dispose = () => {
    if (stopped) return
    stopped = true
    offs.forEach(off => off())
    stopTerminals()
    dispose = undefined
    restoration = undefined
  }
  return dispose
}

export async function attachBrowserTerminal(session: WebSession): Promise<string> {
  await waitForBrowserEvents()
  return attachBrowserSession(session)
}

if (import.meta.hot) import.meta.hot.dispose(() => dispose?.())
