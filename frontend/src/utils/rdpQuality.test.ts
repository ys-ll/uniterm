import { describe, expect, it } from 'vitest'
import { DEFAULT_RDP_QUALITY, normalizeRdpQuality, qualityExtensions, rdpQualityProfile } from './rdpQuality'

describe('normalizeRdpQuality', () => {
  it('defaults to the balanced tier', () => {
    expect(DEFAULT_RDP_QUALITY).toBe('balanced')
  })

  it('keeps valid tiers', () => {
    expect(normalizeRdpQuality('high')).toBe('high')
    expect(normalizeRdpQuality('balanced')).toBe('balanced')
    expect(normalizeRdpQuality('low')).toBe('low')
  })

  it('falls back to the default for absent or unknown values', () => {
    expect(normalizeRdpQuality(undefined)).toBe(DEFAULT_RDP_QUALITY)
    expect(normalizeRdpQuality('')).toBe(DEFAULT_RDP_QUALITY)
    expect(normalizeRdpQuality('ultra')).toBe(DEFAULT_RDP_QUALITY)
    expect(normalizeRdpQuality(6)).toBe(DEFAULT_RDP_QUALITY)
  })
})

describe('rdpQualityProfile', () => {
  it('maps the default tier to the engine defaults (Lan + historical perf flags)', () => {
    const profile = rdpQualityProfile('high')
    expect(profile.connectionType).toBe(6) // Lan
    expect(profile.performanceFlags).toBe(0x86) // full-window drag + menu anims off, font smoothing on
  })

  it('maps balanced to broadband with wallpaper/theming off', () => {
    const profile = rdpQualityProfile('balanced')
    expect(profile.connectionType).toBe(4) // BroadbandHigh
    expect(profile.performanceFlags & 0x01).toBe(0x01) // wallpaper off
    expect(profile.performanceFlags & 0x08).toBe(0x08) // theming off
    expect(profile.performanceFlags & 0x80).toBe(0x80) // font smoothing on
    expect(profile.performanceFlags & 0x100).toBe(0) // desktop composition off
  })

  it('maps low to the modem profile with every decoration off', () => {
    const profile = rdpQualityProfile('low')
    expect(profile.connectionType).toBe(1) // Modem
    expect(profile.performanceFlags).toBe(0x6f) // 0x8f minus font smoothing, plus cursor flags
    expect(profile.performanceFlags & 0x80).toBe(0) // font smoothing off
    expect(profile.performanceFlags & 0x100).toBe(0) // no desktop composition
  })
})

describe('qualityExtensions', () => {
  it('always sends the selected tier, including the default balanced', () => {
    expect(DEFAULT_RDP_QUALITY).toBe('balanced')
    expect(qualityExtensions('balanced')).toEqual([
      { name: 'connection_type', value: 4 }, // BroadbandHigh
      { name: 'performance_flags', value: 0x8f }, // wallpaper/theming/animations off, font smoothing on
    ])
  })

  it('sends both extensions for every non-default tier too', () => {
    expect(qualityExtensions('high')).toEqual([
      { name: 'connection_type', value: 6 },
      { name: 'performance_flags', value: 0x86 },
    ])
    expect(qualityExtensions('low')).toEqual([
      { name: 'connection_type', value: 1 },
      { name: 'performance_flags', value: 0x6f },
    ])
  })

  it('falls back to balanced for unknown or malformed tiers without throwing', () => {
    for (const value of [undefined, null, '', 'ultra', 6, {}, []]) {
      expect(() => qualityExtensions(value)).not.toThrow()
      expect(qualityExtensions(value)).toEqual(qualityExtensions('balanced'))
    }
  })
})
