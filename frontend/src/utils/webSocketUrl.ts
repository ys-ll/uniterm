// Session proxy addresses come from the backend in two forms:
// - desktop: an absolute loopback URL (ws://127.0.0.1:<port>/<token>) that
//   works because the webview shares the backend's machine.
// - server build: a path (/api/web/session-proxy/<token>) meant to ride the
//   same origin as the web page — a loopback URL would resolve against the
//   remote browser's own machine and fail (issue #1397).
// Absolutize path-form addresses against the current page location.
export function toWebSocketURL(proxyAddr: string): string {
  if (proxyAddr.startsWith('/')) {
    const loc = globalThis.location
    const proto = loc?.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${loc?.host}${proxyAddr}`
  }
  return proxyAddr
}

// The session proxies' auth token is the last path segment of the proxy
// address, for both forms (ws://127.0.0.1:<port>/<token> and
// /api/web/session-proxy/<token>).
export function proxyAuthToken(proxyAddr: string): string {
  const path = new URL(toWebSocketURL(proxyAddr)).pathname
  const segments = path.split('/').filter(Boolean)
  return segments[segments.length - 1] ?? ''
}
