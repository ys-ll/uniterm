import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  DisableSessionOutputLog: vi.fn(async () => {}), RegisterSessionForPanel: vi.fn(async () => {}), UnregisterSession: vi.fn(async () => {}),
  CloseSession: vi.fn(async () => {}),
}))
vi.mock('../utils/webMode', () => ({ isWebMode: () => true }))
vi.mock('./terminalManager', () => ({
  attachWebTerminal: vi.fn(async () => {}), disposeTerminal: vi.fn(), initWebTerminals: vi.fn(),
  resyncWebTerminals: vi.fn(), refreshWebTerminal: vi.fn(), disposeWebTerminals: vi.fn(), terminalSharingErrors: new Map(),
}))
vi.mock('./webTerminalBridge', () => ({ webTerminalBridge: {} }))
vi.mock('../stores/sessionStore', () => ({ useSessionStore: () => ({ attachSession() {}, removeSession() {} }) }))
vi.mock('../stores/tabStore', () => ({ useTabStore: () => ({ setActiveTab() {}, createTerminalTab: (name: string, panelId: string) => ({ id: panelId + '-tab', name }) }) }))
import { DisableSessionOutputLog, RegisterSessionForPanel, UnregisterSession, CloseSession } from '../../bindings/github.com/ys-ll/uniterm/app'
import { attachBrowserSession, detachBrowserPanel, terminateBrowserSession } from './webTerminals'
import { disposeTerminal, attachWebTerminal } from './terminalManager'
import { usePanelStore } from '../stores/panelStore'
import type { WebSession } from './webTerminalBridge'

describe('browser-local views of existing sessions', () => {
  beforeEach(() => { setActivePinia(createPinia()); vi.clearAllMocks(); usePanelStore().panels.clear() })
  it('attaches existing text session once and removes only its view without touching desktop logging', async () => {
    const session: WebSession = { id: 'existing', type: 'tcp', title: 'raw socket', status: 'connected', cols: 80, rows: 24, activeClient: 'desktop', resizeCapability: 'display' }
    const id = await attachBrowserSession(session)
    const again = await attachBrowserSession(session)
    expect(again).toBe(id)
    expect(attachWebTerminal).toHaveBeenCalledTimes(1)
    expect(usePanelStore().isAttachedPanel(id)).toBe(true)
    expect(usePanelStore().getPanel(id)?.sessionId).toBe('existing')
    expect(detachBrowserPanel(id)).toBe(true)
    expect(usePanelStore().getPanel(id)).toBeUndefined()
    expect(disposeTerminal).toHaveBeenCalledWith('existing')
    expect(RegisterSessionForPanel).not.toHaveBeenCalled()
    expect(UnregisterSession).not.toHaveBeenCalled()
    expect(DisableSessionOutputLog).not.toHaveBeenCalled()
    expect(CloseSession).not.toHaveBeenCalled()
  })

  it('closes the real session only for explicit Disconnect', async () => {
    const store = usePanelStore()
    const panel = store.createPanel(null, 'local')
    store.bindAttachedSession(panel.id, 'running-shell')
    await terminateBrowserSession(panel.id)
    expect(CloseSession).toHaveBeenCalledExactlyOnceWith('running-shell')
  })
})
