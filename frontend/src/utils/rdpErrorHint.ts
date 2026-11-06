// Maps raw RDP failure text (IronRDP engine / backend proxy) to a localized
// hint key shown under the error message in the RDP error overlay. Matching
// keys off the engine's stable NegotiationFailure Display strings and the
// X.224 initiation error, so the mapping survives message rewording upstream.
const HINT_MATCHERS: Array<[RegExp, string]> = [
  // Server replied NEG_FAILURE(5): CredSSP (NLA) is required (issue #1404).
  [/Enhanced RDP Security with CredSSP/i, 'rdp.hintNlaRequired'],
  // Server only speaks the legacy standard RDP security (RC4) — e.g. xrdp
  // with security_layer=rdp (issue #1374).
  [/STANDARD_RDP_SECURITY|Standard RDP Security/i, 'rdp.hintLegacySecurity'],
]

export function rdpErrorHintKey(reason: string): string | null {
  if (!reason) return null
  for (const [pattern, key] of HINT_MATCHERS) {
    if (pattern.test(reason)) return key
  }
  return null
}
