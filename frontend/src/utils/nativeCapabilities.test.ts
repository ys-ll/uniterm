import { afterEach, describe, expect, it, vi } from 'vitest'
import { hasNativeFileDialogs, hasNativeWindow, openExternalLink, shouldCloseSession, supportsConnectionType } from './nativeCapabilities'

const { openURL } = vi.hoisted(() => ({ openURL: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Browser: { OpenURL: openURL } }))
afterEach(() => { vi.unstubAllGlobals(); openURL.mockReset() })

describe('native capability boundary', () => {
  it('keeps browser text views detached from host lifecycle while allowing independent connections', async () => {
    const open = vi.fn()
    vi.stubGlobal('window', { __UNITERM_WEB__: { instanceId: 'host', clientId: 'visitor' }, open })
    expect(hasNativeWindow()).toBe(false)
    expect(hasNativeFileDialogs()).toBe(false)
    for (const type of ['local', 'ssh', 'tcp', 'serial', 'container-exec', 'k8s-exec']) expect(shouldCloseSession(type)).toBe(false)
    for (const type of ['database', 'redis', 'mongodb', 'elasticsearch', 'sftp', 'vnc', 'spice']) {
      expect(shouldCloseSession(type)).toBe(true)
      expect(supportsConnectionType(type)).toBe(true)
    }
    expect(supportsConnectionType('rdp')).toBe(false)
    expect(supportsConnectionType('x11-desktop')).toBe(false)
    await openExternalLink('https://uniterm.net')
    expect(open).toHaveBeenCalledWith('https://uniterm.net', '_blank', 'noopener,noreferrer')
    expect(openURL).not.toHaveBeenCalled()
  })

  it('preserves native close, protocol and external-link behavior', async () => {
    vi.stubGlobal('window', {})
    expect(hasNativeWindow()).toBe(true)
    expect(hasNativeFileDialogs()).toBe(true)
    expect(shouldCloseSession('ssh')).toBe(true)
    expect(supportsConnectionType('rdp')).toBe(true)
    await openExternalLink('https://uniterm.net')
    expect(openURL).toHaveBeenCalledWith('https://uniterm.net')
  })
})
