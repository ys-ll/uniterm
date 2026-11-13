// Frontend-side identity resolution for the remote-desktop family (RDP/VNC/
// SPICE). Those protocols run in WASM clients inside the webview, which read
// the credentials straight off the panel config — the backend's
// MaterializeIdentity output never reaches them. The resolution therefore
// happens here, from the identity store the frontend already holds decrypted.
//
// Only password-type identities can drive these protocols (no private-key
// concept), so key/keyText references resolve to null. The user field is the
// explicit per-connection override (issue #959 semantics): non-empty wins,
// empty falls back to the identity's username. Userless protocols (VNC/SPICE)
// simply ignore the user part.
import type { ConnectionConfig } from '../types/session'
import type { Identity } from '../types/identity'

export interface ResolvedIdentityCredentials {
  user: string
  password: string
}

export function resolveIdentityCredentials(
  config: ConnectionConfig,
  identities: Identity[]
): ResolvedIdentityCredentials | null {
  if (config.authType !== 'identity' || !config.identityId) return null
  const identity = identities.find(i => i.id === config.identityId)
  if (!identity || identity.authType !== 'password') return null
  return {
    user: config.user || identity.username || '',
    password: identity.password || '',
  }
}
