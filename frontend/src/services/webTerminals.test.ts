import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  DisableSessionOutputLog: vi.fn(async () => {}), RegisterSessionForPanel: vi.fn(async () => {}), UnregisterSession: vi.fn(async () => {}),
  CloseSession: vi.fn(async () => {}),
  CreateSession: vi.fn(async () => ({ id: 'file-session' })), SessionStart: vi.fn(), ListSessions: vi.fn(async () => []),
  LoadBusinessConfig: vi.fn(), SaveBusinessConfig: vi.fn(), RecordRecentConnection: vi.fn(),
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) }, Call: { ByName: vi.fn() }, Browser: { OpenURL: vi.fn() } }))
vi.mock('../utils/webMode', () => ({ isWebMode: () => true, webClientId: () => 'visitor' }))
vi.mock('./terminalManager', () => ({
  attachWebTerminal: vi.fn(async () => {}), disposeTerminal: vi.fn(), initWebTerminals: vi.fn(),
  resyncWebTerminals: vi.fn(), refreshWebTerminal: vi.fn(), disposeWebTerminals: vi.fn(), terminalSharingErrors: new Map(),
}))
vi.mock('./webTerminalBridge', () => ({ webTerminalBridge: {} }))
vi.mock('./message', () => ({ msg: { error: vi.fn(), warning: vi.fn(), copyable: vi.fn() } }))
import { DisableSessionOutputLog, RegisterSessionForPanel, UnregisterSession, CloseSession, CreateSession, SessionStart, LoadBusinessConfig, SaveBusinessConfig, RecordRecentConnection } from '../../bindings/github.com/ys-ll/uniterm/app'
import { attachBrowserSession, detachBrowserPanel, terminateBrowserSession } from './webTerminals'
import { disposeTerminal, attachWebTerminal } from './terminalManager'
import { usePanelStore } from '../stores/panelStore'
import { useTabStore } from '../stores/tabStore'
import { useSessionStore } from '../stores/sessionStore'
import { useConnectionStore } from '../stores/connectionStore'
import { useCompanionStore } from '../stores/companionStore'
import { launchFileBrowser, launchMonitor } from '../composables/connectionLauncher'
import type { WebSession } from './webTerminalBridge'
import type { ConnectionConfig } from '../types/session'

const ssh: WebSession = { id: 'shared-ssh', connectionId: 'saved-ssh', type: 'ssh', title: 'Shared SSH', status: 'connected', cols: 92, rows: 31, activeClient: 'desktop', resizeCapability: 'pty' }
const savedSSH: ConnectionConfig = { id: 'saved-ssh', type: 'ssh', name: 'Server', host: 'server.example', port: 2202, user: 'operator', authType: 'password', password: 'test-password', fileTransferProto: 'scp', sshAlgorithms: { mode: 'custom', ciphers: ['aes256-ctr'] } }
const configResponse = (connections: ConnectionConfig[]) => ({ revision: 'revision-1', json: JSON.stringify({ groups: [], connections }) })

describe('browser-local views of existing sessions', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    usePanelStore().panels.clear()
    useTabStore().tabs.splice(0)
    useSessionStore().sessions.clear()
    vi.mocked(LoadBusinessConfig).mockResolvedValue(configResponse([]))
    vi.mocked(SaveBusinessConfig).mockResolvedValue(configResponse([]))
  })
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

  it('loads the saved reference before exposing a restored SSH tab to file-sidebar actions', async () => {
    let loaded!: (value: ReturnType<typeof configResponse>) => void
    vi.mocked(LoadBusinessConfig).mockReturnValueOnce(new Promise(resolve => { loaded = resolve }))
    const attaching = attachBrowserSession(ssh)
    // Restoration runs before App.onMounted loads the connections store.
    expect(useTabStore().tabs).toHaveLength(0)
    await vi.waitFor(() => expect(LoadBusinessConfig).toHaveBeenCalledWith('connections'))
    const appLoad = useConnectionStore().load()
    loaded(configResponse([savedSSH]))
    const id = await attaching
    await appLoad
    const panel = usePanelStore().getPanel(id)!
    expect(panel.config).toEqual(savedSSH)
    expect(useTabStore().getActivePanelId()).toBe(id)
    expect(usePanelStore().isAttachedPanel(id)).toBe(true)
    expect(attachWebTerminal).toHaveBeenCalledExactlyOnceWith(ssh)
    expect(CreateSession).not.toHaveBeenCalled()
    expect(SessionStart).not.toHaveBeenCalled()
    // This is the real store action used by FileSidebar.ensureConnected.
    const companion = useCompanionStore()
    expect(companion.activeFilesPanelId).toBe(id)
    expect(await companion.ensureSftp(id)).toBe('file-session')
    expect(CreateSession).toHaveBeenCalledExactlyOnceWith('scp', expect.objectContaining({ host: 'server.example', port: 2202, user: 'operator', password: 'test-password', deferConnect: false }))
    expect(useConnectionStore().connections).toEqual([savedSSH])
    expect(SaveBusinessConfig).not.toHaveBeenCalled()
  })

  it.each([undefined, 'deleted-reference'])('keeps a terminal with reference %s usable without saving a panel as a connection', async connectionId => {
    vi.mocked(LoadBusinessConfig).mockResolvedValue(configResponse([savedSSH]))
    const connections = useConnectionStore()
    await connections.load()
    const id = await attachBrowserSession({ ...ssh, connectionId, host: 'descriptor-only.example' })
    const panel = usePanelStore().getPanel(id)!
    expect(panel.config).toBeNull()
    expect(useTabStore().getActivePanelId()).toBe(id)
    expect(attachWebTerminal).toHaveBeenCalledTimes(1)
    expect(await useCompanionStore().ensureSftp(id)).toBeNull()
    // App's existing event handlers accept either a config or a panel. Exercise
    // their config-less fallback, including the terminal context-menu route.
    const detail: any = panel
    await launchFileBrowser(detail.config || detail)
    await launchMonitor(detail.config || detail)
    expect(connections.connections).toEqual([savedSSH])
    expect(SaveBusinessConfig).not.toHaveBeenCalled()
    expect(CreateSession).not.toHaveBeenCalled()
    expect(SessionStart).not.toHaveBeenCalled()
  })

  it('retains a current in-memory config and opens its companions without saving it', async () => {
    const connections = useConnectionStore()
    await connections.load()
    const config = { ...savedSSH, id: 'connect-only', password: 'live-only-password' }
    const panels = usePanelStore(), tabs = useTabStore()
    const panel = panels.createPanel(config, 'ssh')
    panels.bindAttachedSession(panel.id, ssh.id)
    const tab = tabs.createTerminalTab('Transient SSH', panel.id)
    panels.movePanelToTab(panel.id, tab.id)
    vi.mocked(LoadBusinessConfig).mockClear()
    expect(await attachBrowserSession(ssh)).toBe(panel.id)
    expect(panels.getPanel(panel.id)?.config).toEqual(config)
    expect(LoadBusinessConfig).not.toHaveBeenCalled()
    expect(attachWebTerminal).not.toHaveBeenCalled()
    await launchFileBrowser(panel.config!)
    await launchMonitor(panel.config!)
    expect(CreateSession).toHaveBeenNthCalledWith(1, 'scp', expect.objectContaining({ host: 'server.example', user: 'operator', password: 'live-only-password' }))
    expect(CreateSession).toHaveBeenNthCalledWith(2, 'monitor', expect.objectContaining({ host: 'server.example', user: 'operator', password: 'live-only-password' }))
    expect(connections.connections).toEqual([])
    expect(SaveBusinessConfig).not.toHaveBeenCalled()
    expect(RecordRecentConnection).not.toHaveBeenCalled()
    expect(SessionStart).not.toHaveBeenCalled()
    expect(panels.getPanel(panel.id)?.config).toEqual(config)
  })

  it('uses the loaded saved reference without aliasing it or overwriting a live panel config', async () => {
    const connections = useConnectionStore()
    connections.connections = [JSON.parse(JSON.stringify(savedSSH))]
    const id = await attachBrowserSession(ssh)
    const panel = usePanelStore().getPanel(id)!
    panel.config!.password = 'live-password'
    panel.config!.sshAlgorithms!.ciphers!.push('aes128-ctr')
    expect(await attachBrowserSession(ssh)).toBe(id)
    expect(panel.config!.password).toBe('live-password')
    expect(connections.connections).toEqual([savedSSH])
    expect(LoadBusinessConfig).not.toHaveBeenCalled()
    expect(SaveBusinessConfig).not.toHaveBeenCalled()
    expect(CreateSession).not.toHaveBeenCalled()
    expect(SessionStart).not.toHaveBeenCalled()
    expect(attachWebTerminal).toHaveBeenCalledTimes(1)
  })

  it('does not recreate a closed view when its saved reference finishes loading', async () => {
    let loaded!: (value: ReturnType<typeof configResponse>) => void
    vi.mocked(LoadBusinessConfig).mockReturnValueOnce(new Promise(resolve => { loaded = resolve }))
    const attaching = attachBrowserSession(ssh)
    await vi.waitFor(() => expect(LoadBusinessConfig).toHaveBeenCalledTimes(1))
    const panel = [...usePanelStore().panels.values()][0]
    expect(detachBrowserPanel(panel.id)).toBe(true)
    loaded(configResponse([savedSSH]))
    await attaching
    expect(usePanelStore().panels.size).toBe(0)
    expect(useTabStore().tabs).toHaveLength(0)
    expect(useSessionStore().sessions.size).toBe(0)
    expect(attachWebTerminal).not.toHaveBeenCalled()
    expect(CreateSession).not.toHaveBeenCalled()
    expect(SessionStart).not.toHaveBeenCalled()
    expect(CloseSession).not.toHaveBeenCalled()
    expect(SaveBusinessConfig).not.toHaveBeenCalled()
  })
})
