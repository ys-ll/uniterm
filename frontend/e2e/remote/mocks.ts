// Only the surrounding shell is mocked; protocol clients and HTTP creation are real.
const proxies = new Map<string, string>()
const caches = new Map<string, any>()
function removeCache(id: string) { caches.get(id)?.container.remove(); caches.delete(id) }
export const usePanelStore = () => ({
  bindSession() {}, setProxyAddr: (id: string, addr: string) => proxies.set(id, addr),
  getProxyAddr: (id: string) => proxies.get(id), removeProxyAddr: (id: string) => proxies.delete(id),
  getVNCCache: (id: string) => caches.get(id), setVNCCache: (id: string, cache: any) => caches.set(id, cache), removeVNCCache: removeCache,
  getSPICECache: (id: string) => caches.get(id), setSPICECache: (id: string, cache: any) => caches.set(id, cache), removeSPICECache: removeCache,
})
export const useSessionStore = () => ({ initSession() {} })
export const Events = { On: () => () => {} }
export const Clipboard = { SetText: () => { throw new Error('Host clipboard must not be used by browser graphical clients') } }
export const CreateSession = () => { throw new Error('Browser creation must use the authenticated route') }
export const CloseSession = (id: string) => {
  if (id === 'desktop-session-must-not-be-adopted') {
    ;(window as any).__remoteClosedDesktop = true
    throw new Error('Browser cleanup attempted to close a desktop session')
  }
  return fetch(`/e2e/close/${id}`, { method: 'POST' })
}
