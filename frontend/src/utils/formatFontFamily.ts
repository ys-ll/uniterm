/**
 * Format a font family name for use in CSS `font-family` declarations.
 *
 * System fonts that contain spaces (e.g. "DejaVu Sans Mono", "Fira Code",
 * "GB18030 Bitmap") must be wrapped in CSS quotes so the browser treats them
 * as a single family name. Without quotes, "DejaVu Sans Mono" is parsed as
 * three separate families (DejaVu, Sans, Mono), the browser can't find the
 * intended font, and xterm.js canvas width measurement diverges from actual
 * rendering — causing double-width characters and layout corruption.
 *
 * An already-formatted CSS font-family stack (containing commas) is passed
 * through unchanged.
 */
export function formatFontFamily(name: string): string {
  if (!name) return name
  // Already a CSS font-family stack (has commas) — pass through
  if (name.includes(',')) return name
  const needsQuotes = /\s/.test(name)
  const quoted = needsQuotes ? `"${name}"` : name
  return `${quoted}, monospace`
}