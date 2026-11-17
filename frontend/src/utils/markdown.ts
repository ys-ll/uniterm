// Dependency-free markdown renderer shared by every surface that renders
// markdown into v-html (AI messages, notes preview, update changelog). Keep
// in sync with AIMessage.sanitize.test.ts, which mirrors sanitizeRenderedHtml.

// Sanitize markdown-produced HTML before it's assigned to v-html.
// This is defense in depth: every caller escapes its input first (see
// escapeHtml), so no raw tag/quote from model or user content can reach
// here. The strip-list below catches what the markdown renderer itself
// could synthesize (javascript: links, quote-less attribute breakouts).
export function sanitizeRenderedHtml(html: string): string {
  // Drop dangerous tags entirely (including their content).
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
  // Strip on*="..." event-handler attributes (any attribute starting with on),
  // whether separated from the previous attribute by whitespace or by the
  // tag-internal slash (<a href="x"/onclick="...">). The slash variant only
  // matches a quoted value so URLs like href="/online=1" stay intact.
  html = html.replace(/\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi, '')
  html = html.replace(/\/on[a-z]+\s*=\s*("[^"]*"|'[^']*')/gi, '')
  // Strip javascript:/data:/vbscript: URL schemes in href/src.
  html = html.replace(
    /\s+(href|src|action|formaction|xlink:href)\s*=\s*("\s*(?:javascript|data|vbscript):[^"]*"|'\s*(?:javascript|data|vbscript):[^']*'|(?:javascript|data|vbscript):[^\s>]+)/gi,
    '',
  )
  return html
}

// Escape HTML-significant characters (including quotes — a raw double quote
// in renderer-synthesized attributes would let `on*=` handlers break out).
// Shared by the markdown pipeline and surfaces that escape model/user text
// before v-html (AIMessage.vue).
export function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

// Copy/check inline SVGs for the copy button the renderer embeds into every
// fenced code block. Exported so components that host rendered output can
// swap in the check icon for copy feedback and restore it afterwards.
export const COPY_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="0.75rem" height="0.75rem" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>'
export const CHECK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="0.75rem" height="0.75rem" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>'

// Code text of the code block whose copy button was clicked, or null when the
// click did not hit a copy button. Shared by every host of rendered markdown
// so copy feedback behaves identically everywhere.
export function codeCopyTarget(event: Event): string | null {
  const btn = (event.target as HTMLElement)?.closest?.('.code-copy-btn') as HTMLElement | null
  if (!btn) return null
  const wrapper = btn.closest('.code-block-wrapper')
  return wrapper?.querySelector('code')?.textContent || null
}

export interface MarkdownRenderOptions {
  // Title (tooltip) for the embedded code-block copy button; i18n'd by the
  // caller since this module has no i18n access.
  copyButtonTitle?: string
}

function buildNestedList(block: string, isOrdered: boolean): string {
  const tag = isOrdered ? 'ol' : 'ul'
  const marker = isOrdered ? /^ *\d+\. / : /^ *- /
  const lines = block.split('\n').filter(l => l.trim() !== '')

  function process(startIdx: number, baseIndent: number): { html: string; endIdx: number } {
    let result = ''
    let i = startIdx

    while (i < lines.length) {
      const line = lines[i]
      const indent = (line.match(/^ */) || [''])[0].length

      if (indent < baseIndent) break

      if (indent === baseIndent) {
        const content = line.replace(marker, '').trim()
        // Check for child items (more indented lines)
        let childrenHtml = ''
        const peek = i + 1
        if (peek < lines.length) {
          const peekIndent = (lines[peek].match(/^ */) || [''])[0].length
          if (peekIndent > baseIndent) {
            const child = process(peek, peekIndent)
            childrenHtml = `<${tag}>${child.html}</${tag}>`
            i = child.endIdx
          } else {
            i++
          }
        } else {
          i++
        }
        result += `<li>${content}${childrenHtml}</li>`
      } else {
        i++
      }
    }

    return { html: result, endIdx: i }
  }

  const { html } = process(0, 0)
  return `<${tag}>${html}</${tag}>`
}

function buildNestedBlockquote(block: string): string {
  const lines = block.split('\n').filter(l => l.trim() !== '')

  function process(startIdx: number, depth: number): { html: string; endIdx: number } {
    let content = ''
    let i = startIdx

    while (i < lines.length) {
      const line = lines[i]
      const match = line.match(/^(&gt; ?)+/)
      const lineDepth = match ? match[0].split('&gt;').length - 1 : 0

      if (lineDepth < depth) break

      if (lineDepth === depth) {
        const text = line.replace(/^(&gt; ?)+/, '')
        // Check for deeper children
        let childHtml = ''
        const peek = i + 1
        if (peek < lines.length) {
          const peekMatch = lines[peek].match(/^(&gt; ?)+/)
          const peekDepth = peekMatch ? peekMatch[0].split('&gt;').length - 1 : 0
          if (peekDepth > depth) {
            const child = process(peek, peekDepth)
            childHtml = child.html
            i = child.endIdx
          } else {
            i++
          }
        } else {
          i++
        }
        content += (content ? '\n' : '') + text
        if (childHtml) content += '\n' + childHtml
      } else {
        i++
      }
    }

    return { html: `<blockquote>${content.trim()}</blockquote>`, endIdx: i }
  }

  const firstDepth = lines.length > 0
    ? (lines[0].match(/^(&gt; ?)+/) || [''])[0].split('&gt;').length - 1
    : 0
  const { html } = process(0, firstDepth || 1)
  return html
}

function autoLinkUrls(html: string): string {
  const regex = /(https?:\/\/[^\s<>\[\]()"']+)/g
  const parts = html.split('<')
  let result = parts[0].replace(regex, '<a href="$1" target="_blank">$1</a>')
  for (let i = 1; i < parts.length; i++) {
    const gt = parts[i].indexOf('>')
    if (gt === -1) {
      result += '<' + parts[i].replace(regex, '<a href="$1" target="_blank">$1</a>')
    } else {
      const tag = parts[i].slice(0, gt + 1)
      const text = parts[i].slice(gt + 1)
      result += '<' + tag + text.replace(regex, '<a href="$1" target="_blank">$1</a>')
    }
  }
  return result
}

// The one markdown renderer for the app: headings, fenced code blocks (with
// language tag + copy button), inline code, footnotes, bold/italic/strikethrough,
// images, links + raw-URL auto-linking, nested blockquotes, nested lists, task
// checkboxes, tables and paragraph wrapping. Input is fully escaped first, so
// raw user/model HTML never survives; the output must still go through
// sanitizeRenderedHtml before being bound to v-html.
export function renderMarkdownHtml(source: string, opts?: MarkdownRenderOptions): string {
  const copyTitle = escapeHtml(opts?.copyButtonTitle ?? 'Copy')
  // escapeHtml escapes & < > and quotes — the quote escape matters here:
  // without it a markdown link like [x](x"/onerror="alert(1)) breaks out of
  // href="..." via the slash attribute separator, and sanitizeRenderedHtml
  // only strips on*= handlers preceded by whitespace (FE-01 follow-up).
  let html = escapeHtml(source)

  // Protect fenced code blocks and inline code from further markdown processing
  const protectedBlocks: string[] = []
  html = html.replace(/```(\w*)[\t ]*[\r\n]*([\s\S]*?)```/g, (_, lang, code) => {
    const idx = protectedBlocks.length
    const langTag = lang ? `<span class="code-lang-tag">${lang}</span>` : ''
    const header = `<div class="code-block-header">${langTag}<button class="tool-copy-btn code-copy-btn" title="${copyTitle}">${COPY_ICON}</button></div>`
    const codeHtml = code.replace(/^[\r\n]+|\s+$/g, '').replace(/\r?\n/g, '&#10;')
    protectedBlocks.push(`<div class="code-block-wrapper">${header}<pre><code>${codeHtml}</code></pre></div>`)
    return `\x00CODEBLOCK${idx}\x00`
  })
  html = html.replace(/`([^`]+)`/g, (_, code) => {
    const idx = protectedBlocks.length
    protectedBlocks.push(`<code>${code}</code>`)
    return `\x00CODEBLOCK${idx}\x00`
  })

  // Headings
  html = html.replace(/^###### (.*$)/gim, '<h6>$1</h6>')
  html = html.replace(/^##### (.*$)/gim, '<h5>$1</h5>')
  html = html.replace(/^#### (.*$)/gim, '<h4>$1</h4>')
  html = html.replace(/^### (.*$)/gim, '<h3>$1</h3>')
  html = html.replace(/^## (.*$)/gim, '<h2>$1</h2>')
  html = html.replace(/^# (.*$)/gim, '<h1>$1</h1>')

  // Horizontal rules
  html = html.replace(/^(-{3,})$/gm, '<hr>')

  // Footnotes: collect definitions, render inline at their position
  const footnotes: { id: string; content: string }[] = []
  const fnIdMap: Record<string, number> = {}
  // First pass: collect footnote definitions and assign numbers
  html = html.replace(/^\[\^([^\]]+)\]:\s*(.+)$/gm, (_, id, content) => {
    footnotes.push({ id, content })
    fnIdMap[id] = footnotes.length
    return `\x00FN${footnotes.length - 1}\x00` // Placeholder to be rendered inline later
  })
  // Second pass: replace footnote references [^id] with superscript numbers
  html = html.replace(/\[\^([^\]]+)\]/g, (_, id) => {
    const num = fnIdMap[id]
    if (num) {
      return `<sup class="footnote-ref">[${num}]</sup>`
    }
    return `[^${id}]`
  })
  // Third pass: render footnote definitions as footnotes section at placeholder positions
  html = html.replace(/\x00FN(\d+)\x00/g, (_, idx) => {
    const fn = footnotes[parseInt(idx)]
    return `<div class="footnote-item"><sup>[${idx + 1}]</sup> ${fn.content}</div>`
  })

  // Remove the append logic later — footnotes are now inline

  html = html.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\*(.*?)\*/g, '<em>$1</em>')
  html = html.replace(/~~(.+?)~~/g, '<del>$1</del>')
  // Images (must be before links so ![ doesn't get partially matched)
  html = html.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1">')
  // Markdown links
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>')
  // Auto-link raw URLs (after markdown links/images, only in text outside HTML tags)
  html = autoLinkUrls(html)

  // Blockquotes (with nesting support)
  html = html.replace(/(?:^(&gt; ?)+.*(?:\n|$))+/gm, (block) => buildNestedBlockquote(block))

  // Nested unordered lists
  html = html.replace(/(?:^ {0,4}- .*(?:\n|$))+/gm, (block) => buildNestedList(block, false))
  // Nested ordered lists
  html = html.replace(/(?:^ {0,4}\d+\. .*(?:\n|$))+/gm, (block) => buildNestedList(block, true))

  // Task list checkboxes (Lucide-style SVGs)
  const TASK_CHECKED = '<svg xmlns="http://www.w3.org/2000/svg" width="0.875rem" height="0.875rem" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect><path d="m9 12 2 2 4-4"></path></svg>'
  const TASK_UNCHECKED = '<svg xmlns="http://www.w3.org/2000/svg" width="0.875rem" height="0.875rem" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect></svg>'
  html = html.replace(/(<li>)\[x\] /gi, '$1<span class="task-check checked">' + TASK_CHECKED + '</span>')
  html = html.replace(/(<li>)\[ \] /gi, '$1<span class="task-check">' + TASK_UNCHECKED + '</span>')

  // Tables
  const tableBlocks = html.match(/(?:^\|.*\|.*\n?)+/gm)
  if (tableBlocks) {
    for (const block of tableBlocks) {
      const lines = block.trim().split('\n').filter(line => line.trim())
      if (lines.length < 2) continue
      const dataLines = lines.filter((line, idx) => idx !== 1 || !/^\s*[|:\-|\s]+\|\s*$/.test(line))
      let tableHtml = '<table>'
      dataLines.forEach((line, rowIdx) => {
        const cells = line.split('|').map(c => c.trim()).filter(c => c)
        const tag = rowIdx === 0 ? 'th' : 'td'
        tableHtml += '<tr>' + cells.map(c => `<${tag}>${c}</${tag}>`).join('') + '</tr>'
      })
      tableHtml += '</table>'
      html = html.replace(block, tableHtml)
    }
  }

  // Restore protected code blocks
  html = html.replace(/\x00CODEBLOCK(\d+)\x00/g, (_, idx) => protectedBlocks[parseInt(idx)])

  // Ensure blank lines after block-level elements for paragraph separation
  html = html.replace(/(<\/(table|ul|ol|blockquote|pre|div)>)\s*/gi, '$1\n\n')
  html = html.replace(/(<hr\s*\/?>)\s*/gi, '$1\n\n')

  // Wrap paragraphs: split by blank lines, wrap each segment in <p>
  const parts = html.split(/\n{2,}/)
  html = parts.map(part => {
    const trimmed = part.trim()
    if (!trimmed) return ''
    // Don't wrap if already a block-level element (opening or closing)
    if (/^<\/?(h[1-6]|pre|table|ul|ol|hr|div|p|blockquote|li|hr\s*\/?>)/i.test(trimmed)) {
      return trimmed
    }
    // Convert single newlines to <br> within paragraph
    return '<p>' + trimmed.replace(/\n/g, '<br>') + '</p>'
  }).join('')
  // Clean up empty <p> tags
  html = html.replace(/<p>\s*<\/p>/g, '')
  html = html.replace(/<p>\s*<br>\s*<\/p>/g, '')
  // Remove <br> after block-level closing tags
  html = html.replace(/(<\/(h[1-6]|pre|table|ul|ol|hr|li|div|p|blockquote)>|<hr\s*\/?>)(\s*<br>)+/gi, '$1')
  html = html.replace(/<br>\s*(<(h[1-6]|pre|table|ul|ol|hr|div|p|blockquote)[\s>])/gi, '$1')
  html = html.replace(/<br>\s*<div class="code-block-wrapper">/gi, '<div class="code-block-wrapper">')
  html = html.replace(/(<\/blockquote>)\s*<br>/gi, '$1')
  html = html.replace(/<br>\s*(<blockquote>)/gi, '$1')
  // Remove leading/trailing <br>
  html = html.replace(/^(\s*<br>)+/i, '')
  html = html.replace(/(<br>\s*)+$/i, '')

  return html
}
