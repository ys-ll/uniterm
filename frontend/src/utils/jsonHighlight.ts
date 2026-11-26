// JSON syntax highlighting shared by the tools tab (JSON output, JWT
// sections, YAML→JSON result). Tokenizes the raw text and wraps tokens in
// <span>s, HTML-escaped when emitted — safe to render via v-html.
export function escapeHtml(src: string): string {
  return src.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

const JSON_TOKEN_RE = /("(?:\\.|[^"\\])*"\s*:?)|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g

export function highlightJSON(src: string): string {
  if (!src) return ''
  let out = ''
  let last = 0
  for (let m: RegExpExecArray | null; (m = JSON_TOKEN_RE.exec(src)) !== null;) {
    out += escapeHtml(src.slice(last, m.index))
    let cls: string
    if (m[1] !== undefined) {
      // String token; a trailing colon marks a property key.
      cls = m[1].trimEnd().endsWith(':') ? 'tok-key' : 'tok-string'
    } else if (m[2] !== undefined) {
      cls = m[2] === 'null' ? 'tok-null' : 'tok-bool'
    } else {
      cls = 'tok-number'
    }
    out += `<span class="${cls}">${escapeHtml(m[0])}</span>`
    last = m.index + m[0].length
  }
  out += escapeHtml(src.slice(last))
  return out
}
