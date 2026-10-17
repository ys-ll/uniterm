import { afterEach, describe, expect, it, vi } from 'vitest'
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ CreateSession: vi.fn() }))
import { activateRemoteMedia, createRemoteDesktop, remoteDesktopURL } from './webRemoteDesktop'

afterEach(() => vi.unstubAllGlobals())
describe('browser remote desktop adapter', () => {
  it('uses the visitor origin and rejects desktop proxy URLs', async () => {
    vi.stubGlobal('window', { __UNITERM_WEB__: { clientId: 'browser' } })
    vi.stubGlobal('location', { protocol: 'https:', host: 'uniterm.example:34116' })
    expect(remoteDesktopURL('/web/remote/spice/session/main')).toBe('wss://uniterm.example:34116/web/remote/spice/session/main')
    expect(() => remoteDesktopURL('ws://127.0.0.1:3333/')).toThrow('browser-owned')
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ id: 'browser-session' }) })
    vi.stubGlobal('fetch', fetch)
    await createRemoteDesktop('vnc', { host: 'guest' } as any)
    expect(fetch).toHaveBeenCalledWith('/web/remote/vnc', expect.objectContaining({ method: 'POST', credentials: 'same-origin' }))
  })
  it('activates every audio/video layer and reports blocked playback', async () => {
    const play = vi.fn().mockResolvedValue(undefined)
    const blocked = vi.fn().mockRejectedValue(new Error('user gesture required'))
    expect(await activateRemoteMedia({ querySelectorAll: () => [{ play }, { play: blocked }] } as any)).toBe(false)
    expect(play).toHaveBeenCalledOnce()
    expect(blocked).toHaveBeenCalledOnce()
  })
})
