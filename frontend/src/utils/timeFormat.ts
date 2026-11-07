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

const DAY_MS = 86_400_000

function startOfDay(v: Date | number): number {
  const d = new Date(v)
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
}

function clockTime(ts: number, locale?: string): string {
  return new Intl.DateTimeFormat(locale ?? undefined, {
    hour: '2-digit',
    minute: '2-digit',
  })
    .format(new Date(ts))
    .replace(/\s/g, '')
}

/**
 * Message-list timestamp that degrades by age (local calendar days):
 * - today     -> clock only, e.g. "14:32"
 * - yesterday -> relative word + clock, e.g. "昨天 14:32" / "yesterday 14:32"
 * - older     -> date + clock via {@link formatDateTime}, e.g. "11月5日 14:32"
 * The relative "yesterday" word is produced by Intl.RelativeTimeFormat so it
 * follows the UI language without extra translation strings.
 */
export function formatMessageTime(ts?: number, locale?: string, now: Date = new Date()): string {
  if (!ts) return ''
  try {
    const t = startOfDay(ts)
    const today = startOfDay(now)
    if (t === today) return clockTime(ts, locale)
    if (t === today - DAY_MS) {
      const yesterday = new Intl.RelativeTimeFormat(locale ?? undefined, {
        numeric: 'auto',
      }).format(-1, 'day')
      return `${yesterday} ${clockTime(ts, locale)}`
    }
    return formatDateTime(ts, locale)
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
