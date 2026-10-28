<template>
  <!-- Thought process (reasoning models): full-width block above the
       answer, collapsed by default. Rendered inside .content so it flows
       as a block instead of competing with it in the flex row.
       Icon: pulsing outline circle while streaming, filled circle once done. -->
  <div v-if="message.role === 'assistant' && message.thinking" class="thinking-block">
    <div class="thinking-header" @click="thinkingShown = !thinkingShown">
      <span class="thinking-dot" :class="thinkingLive ? 'live' : 'done'">{{ thinkingLive ? '○' : '●' }}</span>
      <span class="thinking-label">{{ t('ai.thoughtProcess') }}</span>
      <span v-if="thinkingDurationLabel" class="thinking-meta">· {{ thinkingDurationLabel }}</span>
      <span v-if="createdAtLabel" class="thinking-meta">· {{ createdAtLabel }}</span>
      <span class="toggle-icon">{{ thinkingShown ? '▼' : '▶' }}</span>
    </div>
    <pre v-show="thinkingShown" class="thinking-content">{{ message.thinking }}</pre>
  </div>

  <!-- An image-only message has no prose to render: skip the empty bubble. -->
  <div
    v-if="!(message.role === 'user' && !message.content && message.attachments?.length)"
    class="text"
    v-html="renderedContent"
    @click="onTextClick"
  />

  <!-- Attachments the user sent with this message. The payload lives on the
       message (it is replayed to the model on later turns); here we only
       show what was attached. -->
  <div v-if="message.attachments?.length" class="msg-attachments">
    <div
      v-for="att in message.attachments"
      :key="att.id"
      class="msg-attachment"
      :title="`${att.name} · ${formatBytes(att.size)}`"
    >
      <el-image
        v-if="att.kind === 'image'"
        class="msg-attachment-thumb"
        :src="attachmentPreviewUrl(att)"
        :preview-src-list="[attachmentPreviewUrl(att)]"
        :preview-teleported="true"
        fit="cover"
        :alt="att.name"
      />
      <FileText v-else :size="14" class="msg-attachment-icon" />
      <span class="msg-attachment-name">{{ att.name }}</span>
    </div>
  </div>

  <div v-if="message.role === 'assistant' && message.content?.trim()" class="copy-action">
    <button class="copy-md-btn" @click="copyAsMarkdown" :title="t('ai.copyMarkdown')">
      <el-icon><Copy :size="'0.875rem'" /></el-icon>
      <span class="copy-md-label">{{ copyMdLabel }}</span>
    </button>
  </div>

  <div v-if="message.needsContinue" class="continue-box">
    <el-button type="primary" @click="$emit('continue')">
      {{ t('ai.continue') }}
    </el-button>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { Copy, FileText } from '@lucide/vue'
import { useAIStore } from '../../stores/aiStore'
import { useI18n } from '../../i18n'
import { sanitizeRenderedHtml, escapeHtml as escapeHtmlBase } from '../../utils/markdown'
import { formatClock, formatDuration } from '../../utils/timeFormat'
import { attachmentPreviewUrl, formatBytes } from '../../services/attachments'
import type { AIMessage } from '../../types/ai'

const props = defineProps<{ message: AIMessage; searchText?: string }>()
defineEmits<{
  (e: 'continue'): void
}>()

const aiStore = useAIStore()
const { t } = useI18n()

const isInterrupted = computed(() => props.message.role === 'tool' && props.message.content === '[INTERRUPTED]')
const isTimeout = computed(() => props.message.role === 'tool' && props.message.content === '[TIMEOUT]')

const thinkingShown = ref(false)

// ── Timestamps & thinking status ──
const createdAtLabel = computed(() => formatClock(props.message.createdAt))

// Duration label: prefer the stored measurement; while the current turn is
// still thinking, tick live off the store start time.
const liveNow = ref(0)
let liveTimer: ReturnType<typeof setInterval> | null = null
function ensureLiveTimer(on: boolean) {
  if (on && liveTimer === null) {
    liveNow.value = Date.now()
    liveTimer = setInterval(() => {
      liveNow.value = Date.now()
    }, 1000)
  } else if (!on && liveTimer !== null) {
    clearInterval(liveTimer)
    liveTimer = null
  }
}

// True only for the message currently receiving a thinking stream (drives the
// pulsing open circle); every other block shows the finished filled circle.
const thinkingLive = computed(() =>
  aiStore.isRunning
  && aiStore.status === 'thinking'
  && !!props.message.thinking
  && !props.message.thinkingDurationMs,
)

const thinkingDurationLabel = computed(() => {
  if (props.message.thinkingDurationMs) return formatDuration(props.message.thinkingDurationMs)
  if (thinkingLive.value && aiStore.thinkingStartedAt) return formatDuration(liveNow.value - aiStore.thinkingStartedAt)
  return ''
})

watch(thinkingLive, (on) => ensureLiveTimer(on), { immediate: true })
onUnmounted(() => ensureLiveTimer(false))

// ── Copy as markdown ──
const copyMdLabel = ref(t('ai.copyMarkdown'))

async function copyAsMarkdown() {
  try {
    await navigator.clipboard.writeText(props.message.content)
    copyMdLabel.value = t('ai.copied')
    setTimeout(() => {
      copyMdLabel.value = t('ai.copyMarkdown')
    }, 2000)
  } catch {
    copyMdLabel.value = t('ai.copyFailed')
    setTimeout(() => {
      copyMdLabel.value = t('ai.copyMarkdown')
    }, 2000)
  }
}

const COPY_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="0.75rem" height="0.75rem" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>'
const CHECK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="0.75rem" height="0.75rem" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>'

function onTextClick(event: MouseEvent) {
  const btn = (event.target as HTMLElement).closest('.code-copy-btn') as HTMLElement | null
  if (!btn) return
  const wrapper = btn.closest('.code-block-wrapper')
  const code = wrapper?.querySelector('code')?.textContent
  if (code) {
    navigator.clipboard.writeText(code)
    btn.innerHTML = CHECK_ICON
    setTimeout(() => {
      btn.innerHTML = COPY_ICON
    }, 2000)
  }
}

function renderMarkdown(text: string): string {
  // escapeHtmlBase escapes & < > and quotes — the quote escape matters here:
  // without it a markdown link like [x](x"/onerror="alert(1)) breaks out of
  // href="..." via the slash attribute separator, and sanitizeRenderedHtml
  // only strips on*= handlers preceded by whitespace (FE-01 follow-up).
  let html = escapeHtmlBase(text)

  // Protect fenced code blocks and inline code from further markdown processing
  const protectedBlocks: string[] = []
  html = html.replace(/```(\w*)[\t ]*[\r\n]*([\s\S]*?)```/g, (_, lang, code) => {
    const idx = protectedBlocks.length
    const langTag = lang ? `<span class="code-lang-tag">${lang}</span>` : ''
    const header = `<div class="code-block-header">${langTag}<button class="tool-copy-btn code-copy-btn" title="${t('ai.copy')}">${COPY_ICON}</button></div>`
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
  html = html.replace(/^\[\^([^\]]+)\]:\s*(.+)$/gm, (_, id, content) => {
    footnotes.push({ id, content })
    fnIdMap[id] = footnotes.length
    return `\x00FN${footnotes.length - 1}\x00`
  })
  html = html.replace(/\[\^([^\]]+)\]/g, (_, id) => {
    const num = fnIdMap[id]
    if (num) {
      return `<sup class="footnote-ref">[${num}]</sup>`
    }
    return `[^${id}]`
  })
  html = html.replace(/\x00FN(\d+)\x00/g, (_, idx) => {
    const fn = footnotes[parseInt(idx)]
    return `<div class="footnote-item"><sup>[${idx + 1}]</sup> ${fn.content}</div>`
  })

  html = html.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\*(.*?)\*/g, '<em>$1</em>')
  html = html.replace(/~~(.+?)~~/g, '<del>$1</del>')
  html = html.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1">')
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>')
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
      const lines = block.trim().split('\n').filter((line) => line.trim())
      if (lines.length < 2) continue
      const dataLines = lines.filter((line, idx) => idx !== 1 || !/^\s*[|:\-|\s]+\|\s*$/.test(line))
      let tableHtml = '<table>'
      dataLines.forEach((line, rowIdx) => {
        const cells = line.split('|').map((c) => c.trim()).filter((c) => c)
        const tag = rowIdx === 0 ? 'th' : 'td'
        tableHtml += '<tr>' + cells.map((c) => `<${tag}>${c}</${tag}>`).join('') + '</tr>'
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
  html = parts
    .map((part) => {
      const trimmed = part.trim()
      if (!trimmed) return ''
      if (/^<\/?(h[1-6]|pre|table|ul|ol|hr|div|p|blockquote|li|hr\s*\/?>)/i.test(trimmed)) {
        return trimmed
      }
      return '<p>' + trimmed.replace(/\n/g, '<br>') + '</p>'
    })
    .join('')
  html = html.replace(/<p>\s*<\/p>/g, '')
  html = html.replace(/<p>\s*<br>\s*<\/p>/g, '')
  html = html.replace(/(<\/(h[1-6]|pre|table|ul|ol|hr|li|div|p|blockquote)>|<hr\s*\/?>)(\s*<br>)+/gi, '$1')
  html = html.replace(/<br>\s*(<(h[1-6]|pre|table|ul|ol|hr|div|p|blockquote)[\s>])/gi, '$1')
  html = html.replace(/<br>\s*<div class="code-block-wrapper">/gi, '<div class="code-block-wrapper">')
  html = html.replace(/(<\/blockquote>)\s*<br>/gi, '$1')
  html = html.replace(/<br>\s*(<blockquote>)/gi, '$1')
  html = html.replace(/^(\s*<br>)+/i, '')
  html = html.replace(/(<br>\s*)+$/i, '')

  return html
}

function buildNestedList(block: string, isOrdered: boolean): string {
  const tag = isOrdered ? 'ol' : 'ul'
  const marker = isOrdered ? /^ *\d+\. / : /^ *- /
  const lines = block.split('\n').filter((l) => l.trim() !== '')

  function process(startIdx: number, baseIndent: number): { html: string; endIdx: number } {
    let result = ''
    let i = startIdx

    while (i < lines.length) {
      const line = lines[i]
      const indent = (line.match(/^ */) || [''])[0].length

      if (indent < baseIndent) break

      if (indent === baseIndent) {
        const content = line.replace(marker, '').trim()
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
  const lines = block.split('\n').filter((l) => l.trim() !== '')

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

function highlightText(html: string, query: string): string {
  const escaped = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const regex = new RegExp(escaped, 'gi')
  const parts = html.split('<')
  let result = parts[0].replace(regex, '<mark class="ai-search-highlight">$&</mark>')
  for (let i = 1; i < parts.length; i++) {
    const gt = parts[i].indexOf('>')
    if (gt === -1) {
      result += '<' + parts[i].replace(regex, '<mark class="ai-search-highlight">$&</mark>')
    } else {
      const tag = parts[i].slice(0, gt + 1)
      const text = parts[i].slice(gt + 1)
      result += '<' + tag + text.replace(regex, '<mark class="ai-search-highlight">$&</mark>')
    }
  }
  return result
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

const renderedContent = computed(() => {
  let html: string
  if (isInterrupted.value) {
    html = `<span class="interrupted-text">${escapeHtml(t('ai.interrupted'))}</span>`
  } else if (isTimeout.value) {
    html = `<span class="interrupted-text">${escapeHtml(t('ai.timeout'))}</span>`
  } else if (props.message.role === 'user') {
    html = escapeHtml(props.message.content)
  } else {
    html = renderMarkdown(props.message.content)
    html = sanitizeRenderedHtml(html)
  }
  if (props.searchText) {
    html = highlightText(html, props.searchText)
  }
  return html
})

function escapeHtml(text: string): string {
  return escapeHtmlBase(text).replace(/\n/g, '<br>')
}
</script>

<style scoped>
/* Search highlights — :deep() needed because mark tags are injected via v-html */
:deep(mark.ai-search-highlight) {
  background: rgba(250, 204, 21, 0.4);
  color: inherit;
  border-radius: var(--radius-sm);
}
:deep(mark.ai-search-highlight.active) {
  background: rgba(250, 204, 21, 0.6);
  color: inherit;
  outline: 0.125rem solid var(--warning);
  border-radius: var(--radius-sm);
}
.msg-attachments {
  display: flex;
  flex-wrap: wrap;
  gap: 0.375rem;
  margin-top: 0.375rem;
}
.msg-attachment {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  max-width: 100%;
  padding: 0.25rem 0.5rem;
  background: var(--bg-surface);
  border-radius: var(--radius-sm);
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}
.msg-attachment-thumb {
  width: 4.5rem;
  height: 4.5rem;
  border-radius: var(--radius-sm);
  cursor: zoom-in;
  flex-shrink: 0;
}
.msg-attachment-icon {
  color: var(--text-muted);
  flex-shrink: 0;
}
.msg-attachment-name {
  font-size: 0.6875rem;
  color: var(--text-secondary, var(--text-muted));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 14rem;
}
.text {
  font-size: 0.75rem;
  line-height: 1.6;
  color: var(--text-primary);
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-ui);
  user-select: text;
  -webkit-user-select: text;
}
.text :deep(.code-block-wrapper) {
  margin: 0.375rem 0;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  overflow: hidden;
}
.text :deep(.code-block-header) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.1875rem 0.5rem;
  background: var(--bg-overlay);
  border-bottom: 1px solid var(--border-subtle);
}
.text :deep(.code-lang-tag) {
  font-size: 0.625rem;
  font-family: var(--font-ui);
  color: var(--text-muted);
}
.text :deep(.code-block-wrapper pre) {
  margin: 0;
  border: none;
  border-radius: 0;
  background: var(--bg-base);
}
.text :deep(.code-block-wrapper code) {
  padding: 0;
  background: transparent;
  color: var(--text-primary);
}
.text :deep(pre) {
  background: var(--bg-base);
  padding: 0.625rem 0.75rem;
  border-radius: var(--radius-sm);
  overflow-x: auto;
  margin: 0.375rem 0;
  border: 1px solid var(--border-subtle);
}
.text :deep(code) {
  background: var(--bg-base);
  padding: 0.125rem 0.3125rem;
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  color: var(--accent);
}

/* Headings */
.text :deep(h1) { font-size: 1rem; margin: 0.5rem 0 0.25rem; }
.text :deep(h2) { font-size: 0.875rem; margin: 0.5rem 0 0.25rem; }
.text :deep(h3) { font-size: 0.875rem; margin: 0.375rem 0 0.25rem; }
.text :deep(h4) { font-size: 0.8125rem; margin: 0.375rem 0 0.25rem; }
.text :deep(h5) { font-size: 0.75rem; margin: 0.25rem 0 0.125rem; }
.text :deep(h6) { font-size: 0.75rem; margin: 0.25rem 0 0.125rem; color: var(--text-muted); }

/* Lists */
.text :deep(ul),
.text :deep(ol) {
  padding-left: 0;
  margin: 0.25rem 0;
  list-style-position: inside;
}
.text :deep(li) {
  margin: 0.125rem 0;
}
.text :deep(li ul),
.text :deep(li ol) {
  padding-left: 0.75rem;
}
.text :deep(ol ol) { list-style-type: lower-alpha; }
.text :deep(ol ol ol) { list-style-type: lower-roman; }
.text :deep(ul ul) { list-style-type: circle; }
.text :deep(ul ul ul) { list-style-type: square; }
.text :deep(.task-check) {
  display: inline-flex;
  vertical-align: middle;
  margin-right: 0.25rem;
  color: var(--text-muted);
}
.text :deep(.task-check.checked) {
  color: var(--success);
}

/* Links */
.text :deep(p) {
  margin: 0.3125rem 0;
}
.text :deep(a) {
  color: var(--info);
}
.text :deep(a:hover) {
  color: var(--info);
}

/* Strikethrough */
.text :deep(del) {
  text-decoration: line-through;
  color: var(--text-muted);
}

/* Blockquote */
.text :deep(blockquote) {
  margin: 0.5rem 0;
  padding: 0.375rem 0.5rem 0.375rem 0.75rem;
  border-left: 0.1875rem solid var(--accent);
  background: var(--bg-overlay);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  color: var(--text-secondary);
}

/* Horizontal rule */
.text :deep(hr) {
  margin: 0.875rem 0;
  border: none;
  border-top: 1px solid var(--border-hover);
}

/* Tables */
.text :deep(table) {
  border-collapse: collapse;
  margin: 0.25rem 0;
  font-size: 0.75rem;
}
.text :deep(th),
.text :deep(td) {
  border: 1px solid var(--border-hover);
  padding: 0.25rem 0.5rem;
  text-align: left;
}
.text :deep(th) {
  background: var(--bg-overlay);
  font-weight: bold;
}

/* Thought process (reasoning models) */
.thinking-block {
  margin: 0 0 0.375rem 0;
  border-radius: var(--radius-sm);
  overflow: hidden;
}
.thinking-header {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.125rem 0.25rem;
  cursor: pointer;
  user-select: none;
}
.thinking-header:hover .thinking-label {
  color: var(--text-secondary);
}
.thinking-dot {
  font-size: 0.75rem;
  line-height: 1;
}
.thinking-dot.live {
  color: var(--accent);
  animation: thinking-pulse 1.2s ease-in-out infinite;
}

@keyframes thinking-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.35; }
}
.thinking-dot.done {
  color: var(--success);
}
.thinking-label {
  font-size: 0.6875rem;
  color: var(--text-muted);
  font-style: italic;
}
.thinking-meta {
  font-size: 0.625rem;
  color: var(--text-muted);
  opacity: 0.8;
  font-variant-numeric: tabular-nums;
}
.thinking-content {
  margin: 0.25rem 0 0 0;
  padding: 0.375rem 0.625rem;
  max-height: 16rem;
  overflow-y: auto;
  border-left: 2px solid var(--border-subtle);
  background: var(--bg-elevated);
  border-radius: var(--radius-sm);
  font-size: 0.6875rem;
  font-family: var(--font-ui);
  color: var(--text-muted);
  white-space: pre-wrap;
  word-break: break-word;
  user-select: text;
  -webkit-user-select: text;
}
.toggle-icon {
  color: var(--text-muted);
  font-size: 0.625rem;
}
/* Replicate tool-copy-btn styles for v-html code block buttons (scoped styles don't penetrate v-html) */
.text :deep(.code-block-header .tool-copy-btn) {
  background: none;
  border: none;
  padding: 0 0.125rem;
  opacity: 0.4;
  transition: opacity 0.15s;
  color: var(--text-muted);
  display: inline-flex;
  align-items: center;
}
.text :deep(.code-block-header .tool-copy-btn:hover) {
  opacity: 1 !important;
}
.copy-action {
  margin-top: 0.25rem;
}
.copy-md-btn {
  display: inline-flex;
  align-items: center;
  gap: 0;
  padding: 0.125rem 0.25rem;
  font-size: 0.6875rem;
  color: var(--text-muted);
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  transition: all 0.2s;
}
.copy-md-label {
  max-width: 0;
  overflow: hidden;
  opacity: 0;
  white-space: nowrap;
  padding-left: 0;
  transition: max-width 0.2s, opacity 0.15s, padding-left 0.2s;
}
.copy-md-btn:hover {
  gap: 0.25rem;
  padding: 0.125rem 0.5rem;
  color: var(--accent);
  border-color: var(--accent-glow);
  background: var(--accent-subtle);
}
.copy-md-btn:hover .copy-md-label {
  max-width: 9.375rem;
  opacity: 1;
  padding-left: 0.125rem;
}
.continue-box {
  margin-top: 0.5rem;
}
</style>