// Locale-aware clock/duration formatters shared by the AI message list and
// the live thinking box.

/** Clock time of an epoch-ms timestamp, e.g. "14:32" (24h, locale-aware). */
export function formatClock(ts?: number): string {
  if (!ts) return ''
  try {
    return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  } catch {
    return ''
  }
}

/**
 * Date + clock of an epoch-ms timestamp, e.g. "11月7日 14:32" / "Nov 7, 14:32".
 * When the year differs from the current year the year is included, e.g.
 * "2025年11月7日 14:32" / "Nov 7, 2025, 14:32". `locale` falls back to the
 * environment default when omitted, so dates follow the UI language.
 */
export function formatDateTime(ts?: number, locale?: string): string {
  if (!ts) return ''
  try {
    const d = new Date(ts)
    const sameYear = d.getFullYear() === new Date().getFullYear()
    const parts: Intl.DateTimeFormatOptions = sameYear
      ? { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }
      : { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }
    return new Intl.DateTimeFormat(locale ?? undefined, parts).format(d)
  } catch {
    return ''
  }
}

/** Short duration like "42s" or "3m 12s". Returns '' for empty/zero input. */
export function formatDuration(ms?: number): string {
  if (!ms || ms < 100) return ''
  const totalSeconds = Math.max(1, Math.round(ms / 1000))
  if (totalSeconds < 60) return `${totalSeconds}s`
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`
}
