// getRandomValues also works on the explicitly supported LAN HTTP origin,
// where randomUUID/subtle may be unavailable.
export function requestId(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), n => n.toString(16).padStart(2, '0')).join('')
}
