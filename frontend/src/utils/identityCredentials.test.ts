import { describe, expect, it } from 'vitest'
import { resolveIdentityCredentials } from './identityCredentials'
import type { Identity } from '../types/identity'
import type { ConnectionConfig } from '../types/session'

const identities: Identity[] = [
  { id: 'id-pw', name: 'ops', username: 'alice', authType: 'password', password: 'secret' },
  { id: 'id-key', name: 'keyed', username: 'bob', authType: 'key', keyPath: '/tmp/k', password: 'passphrase' },
  { id: 'id-nopw', name: 'empty', username: 'carol', authType: 'password', password: '' },
]

function cfg(over: Partial<ConnectionConfig>): ConnectionConfig {
  return { type: 'rdp', host: 'h', port: 3389, user: '', authType: 'password', ...over } as ConnectionConfig
}

describe('resolveIdentityCredentials', () => {
  it('resolves a password identity and honors the user override', () => {
    expect(resolveIdentityCredentials(cfg({ authType: 'identity', identityId: 'id-pw' }), identities))
      .toEqual({ user: 'alice', password: 'secret' })
    expect(resolveIdentityCredentials(cfg({ authType: 'identity', identityId: 'id-pw', user: 'override' }), identities))
      .toEqual({ user: 'override', password: 'secret' })
  })

  it('returns null for non-identity configs and missing references', () => {
    expect(resolveIdentityCredentials(cfg({}), identities)).toBeNull()
    expect(resolveIdentityCredentials(cfg({ authType: 'identity' }), identities)).toBeNull()
    expect(resolveIdentityCredentials(cfg({ authType: 'identity', identityId: 'gone' }), identities)).toBeNull()
  })

  it('rejects key-type identities (no private-key concept in remote desktop)', () => {
    expect(resolveIdentityCredentials(cfg({ authType: 'identity', identityId: 'id-key' }), identities)).toBeNull()
  })

  it('resolves an identity with an empty password (still valid for autologon-less connect)', () => {
    expect(resolveIdentityCredentials(cfg({ authType: 'identity', identityId: 'id-nopw' }), identities))
      .toEqual({ user: 'carol', password: '' })
  })
})
