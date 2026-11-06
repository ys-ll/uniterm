import { describe, expect, it } from 'vitest'
import { rdpErrorHintKey } from './rdpErrorHint'

describe('rdpErrorHintKey', () => {
  it('maps a CredSSP (NLA) negotiation failure to the NLA hint', () => {
    expect(rdpErrorHintKey('kind=5 | server requires Enhanced RDP Security with CredSSP')).toBe(
      'rdp.hintNlaRequired',
    )
  })

  it('maps a standard-RDP-security rejection to the legacy security hint', () => {
    expect(
      rdpErrorHintKey(
        'kind=0 | [Initiation] reason: client advertised SSL | HYBRID | HYBRID_EX, but server selected STANDARD_RDP_SECURITY',
      ),
    ).toBe('rdp.hintLegacySecurity')
    expect(rdpErrorHintKey('kind=5 | server only supports Standard RDP Security')).toBe(
      'rdp.hintLegacySecurity',
    )
  })

  it('returns null for unmatched or empty errors', () => {
    expect(rdpErrorHintKey('kind=5 | failed to connect (WebSocket is Closed)')).toBeNull()
    expect(rdpErrorHintKey('')).toBeNull()
  })
})
