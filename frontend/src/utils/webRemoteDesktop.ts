import { isWebMode } from './webMode'
import type { ConnectionConfig } from '../types/session'
import { CreateSession } from '../../bindings/github.com/ys-ll/uniterm/app'

export function remoteDesktopURL(path: string): string {
  if (!isWebMode()) return path
  // Never consume a desktop's loopback proxy address in the visitor browser.
  if (!/^\/web\/remote\/(vnc|spice)\/[^/]+\/main$/.test(path)) {
    throw new Error('A new browser-owned remote desktop connection is required')
  }
  return `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}${path}`
}

export async function createRemoteDesktop(type: 'vnc' | 'spice', config: ConnectionConfig) {
  if (!isWebMode()) return CreateSession(type, config)
  const response = await fetch(`/web/remote/${type}`, {
    method: 'POST', credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(config),
  })
  if (!response.ok) throw new Error(await response.text())
  return response.json() as Promise<{ id: string; proxyAddr: string }>
}

// Call synchronously from a click/key gesture; each layer keeps its own decoder.
export async function activateRemoteMedia(container: HTMLElement): Promise<boolean> {
  const results = await Promise.allSettled(Array.from(container.querySelectorAll('audio,video'), media => (media as HTMLMediaElement).play()))
  return results.every(result => result.status === 'fulfilled')
}
