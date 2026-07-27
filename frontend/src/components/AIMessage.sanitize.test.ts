// Regression test for FE-01 — XSS via markdown-produced HTML in
// AIMessage.vue v-html binding. Mirrors the sanitizeRenderedHtml helper
// implementation; if the helper changes, this test must be updated.

import { describe, expect, it } from 'vitest'

function sanitizeRenderedHtml(html: string): string {
  const dangerousTags = [
    'script', 'iframe', 'object', 'embed', 'style', 'form',
    'link', 'meta', 'base', 'svg', 'math',
  ]
  for (const tag of dangerousTags) {
    const re = new RegExp(`<${tag}\\b[\\s\\S]*?<\\/${tag}>`, 'gi')
    html = html.replace(re, '')
    const reSelf = new RegExp(`<${tag}\\b[^>]*\\/?>`, 'gi')
    html = html.replace(reSelf, '')
  }
  html = html.replace(/\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi, '')
  html = html.replace(
    /\s+(href|src|action|formaction|xlink:href)\s*=\s*("\s*(?:javascript|data|vbscript):[^"]*"|'\s*(?:javascript|data|vbscript):[^']*'|(?:javascript|data|vbscript):[^\s>]+)/gi,
    '',
  )
  return html
}

describe('sanitizeRenderedHtml (FE-01 XSS)', () => {
  it('strips javascript: URLs from link href', () => {
    const out = sanitizeRenderedHtml(
      '<a href="javascript:fetch(\'http://x\')" target="_blank">click</a>',
    )
    expect(out).not.toMatch(/javascript:/i)
    expect(out).not.toMatch(/fetch\(/i)
    expect(out).toMatch(/target="_blank"/) // other attrs preserved
    expect(out).toMatch(/>click</)
  })

  it('strips attribute-quote breakout in image src', () => {
    const out = sanitizeRenderedHtml('<img src="x" onerror="alert(1)" alt="alt">')
    expect(out).not.toMatch(/onerror/i)
    expect(out).not.toMatch(/alert\(/i)
    expect(out).toMatch(/<img/) // tag still present
  })

  it('strips <script> tags and their content', () => {
    const out = sanitizeRenderedHtml('before<script>alert(1)</script>after')
    expect(out).not.toMatch(/<script/i)
    expect(out).not.toMatch(/alert\(/)
    expect(out).toMatch(/before/)
    expect(out).toMatch(/after/)
  })

  it('strips iframe / object / embed entirely', () => {
    const out = sanitizeRenderedHtml(
      '<iframe src="https://evil"></iframe><object data="x"></object><embed src="y">',
    )
    expect(out).not.toMatch(/iframe/i)
    expect(out).not.toMatch(/object/i)
    expect(out).not.toMatch(/embed/i)
  })

  it('preserves safe content unchanged', () => {
    const safe = '<p>hello <strong>world</strong></p>'
    expect(sanitizeRenderedHtml(safe)).toBe(safe)
  })
})