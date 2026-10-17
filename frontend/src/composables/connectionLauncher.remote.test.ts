// Cross-language regression: real launcher/stores + component creation adapter
// against NewApp's actual session manager and /web/remote routes, without a GUI.
// Run: UNITERM_REMOTE_LAUNCHER_TEST=1 go test . -run '^TestWebRemoteDesktopLauncher$' -count=1
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { once } from 'node:events'
import { createServer, type Socket } from 'node:net'
import { createPinia, setActivePinia } from 'pinia'

const { createSessionMock, closeSessionMock } = vi.hoisted(() => ({
  createSessionMock: vi.fn(), closeSessionMock: vi.fn(),
}))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  CreateSession: createSessionMock, CloseSession: closeSessionMock, RecordRecentConnection: vi.fn(),
  SaveConnections: vi.fn(), LoadConnections: vi.fn(),
  DisableSessionOutputLog: vi.fn(), RegisterSessionForPanel: vi.fn(), UnregisterSession: vi.fn(),
}))
vi.mock('../services/message', () => ({ msg: { error: vi.fn(), warning: vi.fn() } }))

import { CreateSession, CloseSession } from '../../bindings/github.com/ys-ll/uniterm/app'
import { configureLauncher, launchConnection } from './connectionLauncher'
import { createRemoteDesktop, remoteDesktopURL } from '../utils/webRemoteDesktop'
import type { ConnectionConfig } from '../types/session'

const base = process.env.UNITERM_REMOTE_LAUNCHER_URL
const transportFetch = globalThis.fetch

describe.skipIf(!base)('launcher with real App remote sessions', () => {
  async function request(path: string, body?: unknown) {
    const response = await fetch(path, body === undefined ? undefined : { method: 'POST', body: JSON.stringify(body) })
    if (!response.ok) throw new Error(await response.text())
    return response.json()
  }
  const sessions = (): Promise<{ id: string; type: string }[]> => request('/launcher/sessions')

  beforeEach(() => {
    setActivePinia(createPinia())
    vi.stubGlobal('window', { __UNITERM_WEB__: { clientId: 'launcher-test' } })
    vi.stubGlobal('location', new URL(base!))
    // Supply the same-origin URL/Origin that a browser supplies automatically.
    vi.stubGlobal('fetch', (path: string, init?: RequestInit) => {
      const headers = new Headers(init?.headers)
      headers.set('Origin', base!)
      return transportFetch(new URL(path, base), { ...init, headers })
    })
    createSessionMock.mockReset().mockImplementation((type: string, config: ConnectionConfig) => request('/launcher/native', { ...config, type }))
    closeSessionMock.mockReset().mockImplementation((id: string) => request('/launcher/close/' + id, {}))
    configureLauncher({ ensureCredentials: async config => config, closeStartAndReposition: () => () => {} })
  })
  afterEach(() => vi.unstubAllGlobals())

  it.each(['vnc', 'spice'] as const)('%s keeps exactly one session through reconnect and zero after close', async type => {
    const peers = new Set<Socket>()
    const sockets: WebSocket[] = []
    // Byte fixture only: the proxy's real socket lifetime is exercised here.
    const upstream = createServer(socket => {
      peers.add(socket)
      socket.on('close', () => peers.delete(socket))
      socket.write('remote desktop bytes')
    })
    upstream.listen(0, '127.0.0.1')
    await once(upstream, 'listening')
    try {
      const address = upstream.address()
      if (!address || typeof address === 'string') throw new Error('missing fixture address')
      const config: ConnectionConfig = {
        id: `launcher-${type}`, type, name: '', user: '', authType: 'password',
        host: type === 'spice' ? 'tcp://127.0.0.1' : '127.0.0.1', port: address.port,
      }
      const connect = async () => {
        // This is the adapter used by both tab components after the launcher.
        const info = await createRemoteDesktop(type, config)
        expect(info.proxyAddr).toBe(`/web/remote/${type}/${info.id}/main`)
        // Node's built-in WebSocket supports headers in its WebSocketInit.
        // @ts-expect-error DOM's overload does not expose Node's header option.
        const ws = new WebSocket(remoteDesktopURL(info.proxyAddr), { headers: { Origin: base } })
        ws.binaryType = 'arraybuffer'
        sockets.push(ws)
        const [message] = await once(ws, 'message', { signal: AbortSignal.timeout(3000) })
        expect(new TextDecoder().decode(message.data)).toBe('remote desktop bytes')
        return { ...info, ws }
      }

      expect(await sessions()).toEqual([])
      await launchConnection(config, { persist: false })
      const first = await connect()
      // Before the fix this contains two IDs: native precreation + browser.
      expect((await sessions()).map(session => session.id)).toEqual([first.id])
      expect(CreateSession).not.toHaveBeenCalled()
      first.ws.close()
      await CloseSession(first.id).catch(() => {})
      await expect.poll(sessions, { timeout: 3000 }).toEqual([])

      const second = await connect()
      expect(second.id).not.toBe(first.id)
      expect((await sessions()).map(session => session.id)).toEqual([second.id])
      // Closing the page has no guaranteed RPC: the socket owns cleanup.
      second.ws.close()
      await expect.poll(sessions, { timeout: 3000 }).toEqual([])
      await expect.poll(() => peers.size, { timeout: 3000 }).toBe(0)
      console.info(`${type}: real manager counts 0 -> 1 -> 0 -> 1 -> 0 (launch, reconnect, browser close)`)
    } finally {
      for (const ws of sockets) ws.close()
      for (const info of await sessions()) await CloseSession(info.id).catch(() => {})
      for (const peer of peers) peer.destroy()
      await new Promise<void>(resolve => upstream.close(() => resolve()))
    }
  })
})
