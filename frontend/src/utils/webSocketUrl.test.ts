import { afterEach, describe, expect, it, vi } from 'vitest'
import { proxyAuthToken, toWebSocketURL } from './webSocketUrl'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('toWebSocketURL', () => {
  it('keeps absolute loopback addresses untouched', () => {
    expect(toWebSocketURL('ws://127.0.0.1:37647/some-token')).toBe('ws://127.0.0.1:37647/some-token')
  })

  it('absolutizes path-form addresses against the page origin', () => {
    vi.stubGlobal('location', { protocol: 'https:', host: 'term.example.com' })
    expect(toWebSocketURL('/api/web/session-proxy/abc123')).toBe(
      'wss://term.example.com/api/web/session-proxy/abc123',
    )
  })

  it('uses ws for plain-http pages', () => {
    vi.stubGlobal('location', { protocol: 'http:', host: 'localhost:8080' })
    expect(toWebSocketURL('/api/web/session-proxy/abc123')).toBe(
      'ws://localhost:8080/api/web/session-proxy/abc123',
    )
  })
})

describe('proxyAuthToken', () => {
  it('extracts the token from a desktop loopback URL', () => {
    expect(proxyAuthToken('ws://127.0.0.1:37647/some-token')).toBe('some-token')
  })

  it('extracts the token from a server path-form address', () => {
    vi.stubGlobal('location', { protocol: 'https:', host: 'term.example.com' })
    expect(proxyAuthToken('/api/web/session-proxy/abc123')).toBe('abc123')
  })
})
