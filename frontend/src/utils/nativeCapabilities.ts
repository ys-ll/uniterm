import { Browser } from '@wailsio/runtime'
import { isWebMode } from './webMode'

// These describe UI capabilities, not permissions: authenticated Web clients
// are trusted host operators, but have no native window or host file picker.
export const hasNativeWindow = () => !isWebMode()
export const hasNativeFileDialogs = () => !isWebMode()

export function supportsConnectionType(type: string): boolean {
  return !isWebMode() || (type !== 'rdp' && type !== 'x11-desktop')
}

export function isSharedTextSession(type: string): boolean {
  return ['local', 'ssh', 'telnet', 'mosh', 'wsl', 'tcp', 'serial', 'k8s-exec', 'container-exec'].includes(type)
}

export function shouldCloseSession(type: string): boolean {
  return !isWebMode() || !isSharedTextSession(type)
}

export async function openExternalLink(url: string): Promise<void> {
  if (isWebMode()) {
    window.open(url, '_blank', 'noopener,noreferrer')
  } else {
    await Browser.OpenURL(url)
  }
}
