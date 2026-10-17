// Set by web.html before any application modules are imported.
declare global {
  interface Window {
    __UNITERM_WEB__?: { instanceId: string; clientId: string }
  }
}

export function isWebMode(): boolean {
  return typeof window !== 'undefined' && !!window.__UNITERM_WEB__
}

export function webClientId(): string {
  return typeof window !== 'undefined' ? window.__UNITERM_WEB__?.clientId || 'desktop' : 'desktop'
}

export function setWebClientId(id: string): void {
  if (window.__UNITERM_WEB__) window.__UNITERM_WEB__.clientId = id
}
