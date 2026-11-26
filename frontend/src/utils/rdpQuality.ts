export type RdpQuality = 'high' | 'balanced' | 'low'

export const DEFAULT_RDP_QUALITY: RdpQuality = 'balanced'

// MS-RDPBCGR TS_CONNECTION_TYPE (2.2.1.11.1.1.1.1): 1 = modem,
// 4 = broadband-high, 6 = LAN. The server picks the desktop experience tier
// from it (connection_type only takes effect at session setup).
const CONNECTION_TYPES: Record<RdpQuality, number> = {
  high: 6, // Lan
  balanced: 4, // BroadbandHigh
  low: 1, // Modem
}

// TS_PERF_* bitmask (2.2.1.11.1.1.1.2):
//   0x01 wallpaper off, 0x02 full-window drag off, 0x04 menu animations off,
//   0x08 theming off, 0x20 cursor shadow off, 0x40 cursor settings off,
//   0x80 font smoothing on, 0x100 desktop composition on.
// 'high' mirrors the IronRDP historical default; 'balanced' (the default
// tier, like mstsc's auto-detected WAN profile) keeps font smoothing but
// drops wallpaper/theming; 'low' strips every decoration.
const PERF_FLAGS: Record<RdpQuality, number> = {
  high: 0x02 | 0x04 | 0x80,
  balanced: 0x01 | 0x02 | 0x04 | 0x08 | 0x80,
  low: 0x01 | 0x02 | 0x04 | 0x08 | 0x20 | 0x40,
}

export function normalizeRdpQuality(value: unknown): RdpQuality {
  return value === 'high' || value === 'balanced' || value === 'low' ? value : DEFAULT_RDP_QUALITY
}

// Protocol values negotiated by the WASM client for the tier. `rdpQualityProfile`
// is the single source of truth for the masks.
export function rdpQualityProfile(quality: RdpQuality): { connectionType: number; performanceFlags: number } {
  return { connectionType: CONNECTION_TYPES[quality], performanceFlags: PERF_FLAGS[quality] }
}

export interface RdpQualityExtension {
  name: 'connection_type' | 'performance_flags'
  value: number
}

// Extensions sent to the engine for the selected tier. Every tier is sent,
// including the default `balanced`: the engine falls back to its own LAN/high
// defaults when these are omitted, so the UI tier would silently not apply.
// Unknown values normalize to the default tier instead of throwing.
export function qualityExtensions(quality: unknown): RdpQualityExtension[] {
  const profile = rdpQualityProfile(normalizeRdpQuality(quality))
  return [
    { name: 'connection_type', value: profile.connectionType },
    { name: 'performance_flags', value: profile.performanceFlags },
  ]
}
