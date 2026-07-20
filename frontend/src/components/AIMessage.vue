<template>
  <div class="ai-message" :class="[message.role, { interrupted: isInterrupted }]">
    <div class="content">
      <div class="text" v-html="renderedContent" @click="onTextClick" />

      <div v-if="message.role === 'assistant' && message.content?.trim()" class="copy-action">
        <button class="copy-md-btn" @click="copyAsMarkdown" :title="t('ai.copyMarkdown')">
          <el-icon><Copy :size="14" /></el-icon>
          <span class="copy-md-label">{{ copyMdLabel }}</span>
        </button>
      </div>

      <div v-if="message.needsContinue" class="continue-box">
        <el-button type="primary" @click="$emit('continue')">
          {{ t('ai.continue') }}
        </el-button>
      </div>

      <!-- Tool call pairs: IN + OUT grouped together -->
      <div v-if="message.tool_calls?.length" class="tool-pairs">
        <div v-for="tc in message.tool_calls" :key="tc.id" class="tool-pair">
          <!-- IN box -->
          <div class="tool-box in-box">
            <div class="tool-box-header" @click="inExpanded = !inExpanded">
              <span class="tool-box-label">{{ t('ai.in') }}</span>
              <span class="tool-box-name">{{ formatToolName(tc) }}</span>
              <span class="tool-box-count"></span>
              <button class="tool-copy-btn" @click.stop="copyToolText(formatToolBody(tc), tc.id + '-in')" :title="t('ai.copy')"><el-icon><Check v-if="copiedTool === tc.id + '-in'" :size="14" /><Copy v-else :size="14" /></el-icon></button>
              <span class="toggle-icon">{{ inExpanded ? '▼' : '▶' }}</span>
            </div>
            <div v-show="inExpanded" class="tool-box-body">
              <pre class="tool-call-args" v-if="formatToolBody(tc)">{{ formatToolBody(tc) }}</pre>
            </div>
          </div>

          <!-- OUT box -->
          <div v-if="getToolResult(tc.id)" class="tool-box out-box">
            <div class="tool-box-header" @click="outExpanded = !outExpanded">
              <span class="tool-box-label">{{ t('ai.out') }}</span>
              <span class="tool-box-count"></span>
              <button class="tool-copy-btn" @click.stop="copyToolText(getToolResult(tc.id)?.content || '', tc.id + '-out')" :title="t('ai.copy')"><el-icon><Check v-if="copiedTool === tc.id + '-out'" :size="14" /><Copy v-else :size="14" /></el-icon></button>
              <span class="toggle-icon">{{ outExpanded ? '▼' : '▶' }}</span>
            </div>
            <div v-show="outExpanded" class="tool-box-body">
              <pre class="tool-output">{{ getToolResult(tc.id)?.content }}</pre>
            </div>
          </div>
        </div>
      </div>

      <div v-if="pendingCmd" class="pending-tools">
        <div class="pending-tool" :class="{ dangerous: pendingCmd.dangerous }">
          <div class="tool-name">
            execute_command
            <span v-if="pendingCmd.dangerous" class="danger-badge">{{ t('ai.dangerous') }}</span>
          </div>
          <code class="tool-args">{{ pendingCmd.command }}</code>
          <div class="tool-actions">
            <el-button type="primary" @click="handleApprove">{{ t('ai.run') }}</el-button>
            <el-button @click="handleReject">{{ t('ai.skip') }}</el-button>
          </div>
        </div>
      </div>

      <div v-if="pendingQ" class="question-box">
        <div v-if="pendingQ.header" class="question-header">{{ pendingQ.header }}</div>
        <div class="question-text">{{ pendingQ.question }}</div>
        <div class="question-options">
          <label
            v-for="(opt, i) in pendingQ.options"
            :key="i"
            class="question-option"
            :class="{ selected: selectedOptions.includes(i) }"
            @click="toggleOption(i, !pendingQ.multiSelect)"
          >
            <span v-if="pendingQ.multiSelect" class="option-check">
              <el-icon v-if="selectedOptions.includes(i)"><Check :size="14" /></el-icon>
              <span v-else class="option-check-empty"></span>
            </span>
            <span v-else class="option-radio" :class="{ on: selectedOptions.includes(i) }"></span>
            <span class="option-body">
              <span class="option-label">{{ opt.label }}</span>
              <span class="option-desc">{{ opt.description }}</span>
            </span>
          </label>
          <label
            class="question-option"
            :class="{ selected: otherSelected }"
            @click="toggleOther"
          >
            <span v-if="pendingQ.multiSelect" class="option-check">
              <el-icon v-if="otherSelected"><Check :size="14" /></el-icon>
              <span v-else class="option-check-empty"></span>
            </span>
            <span v-else class="option-radio" :class="{ on: otherSelected }"></span>
            <span class="option-body">
              <span class="option-label">{{ t('ai.other') }}</span>
              <input
                ref="otherInputRef"
                class="other-input"
                :placeholder="t('ai.otherPlaceholder')"
                :value="otherText"
                @click.stop
                @input="onOtherInput"
              />
            </span>
          </label>
        </div>
        <div class="question-actions">
          <el-button type="primary" @click="handleAnswer">{{ t('ai.confirm') }}</el-button>
          <el-button @click="handleDismiss">{{ t('ai.skip') }}</el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Copy, Check } from '@lucide/vue'
import { useAIStore } from '../stores/aiStore'
import { useI18n } from '../i18n'
import type { AIMessage } from '../types/ai'

const props = defineProps<{ message: AIMessage; searchText?: string }>()
const emit = defineEmits<{
  (e: 'approve', messageId: string): void
  (e: 'reject', messageId: string): void
  (e: 'continue'): void
  (e: 'answer', selectedLabels: string[], customText?: string): void
  (e: 'dismiss'): void
}>()

const isInterrupted = computed(() => props.message.role === 'tool' && props.message.content === '[INTERRUPTED]')

const isPending = computed(() =>
  aiStore.pendingCommand?.messageId === props.message.id
)
const pendingCmd = computed(() => isPending.value ? aiStore.pendingCommand! : null)

const isPendingQ = computed(() =>
  aiStore.pendingQuestion?.messageId === props.message.id
)
const pendingQ = computed(() => isPendingQ.value ? aiStore.pendingQuestion! : null)

// Question state
const selectedOptions = ref<number[]>([])
const otherSelected = ref(false)
const otherText = ref('')
const otherInputRef = ref<HTMLInputElement | null>(null)

function toggleOption(i: number, single: boolean) {
  if (single) {
    selectedOptions.value = [i]
    otherSelected.value = false
    otherText.value = ''
    return
  }
  const idx = selectedOptions.value.indexOf(i)
  if (idx >= 0) {
    selectedOptions.value.splice(idx, 1)
  } else {
    selectedOptions.value.push(i)
  }
}

function toggleOther() {
  if (pendingQ.value?.multiSelect) {
    otherSelected.value = !otherSelected.value
    if (otherSelected.value) {
      setTimeout(() => otherInputRef.value?.focus(), 50)
    } else {
      otherText.value = ''
    }
  } else {
    otherSelected.value = true
    selectedOptions.value = []
    setTimeout(() => otherInputRef.value?.focus(), 50)
  }
}

function onOtherInput(e: Event) {
  otherText.value = (e.target as HTMLInputElement).value
}

function handleAnswer() {
  const labels: string[] = []
  if (selectedOptions.value.length > 0 && pendingQ.value) {
    for (const i of selectedOptions.value) {
      labels.push(pendingQ.value.options[i].label)
    }
  }
  if (otherSelected.value) {
    if (otherText.value.trim()) {
      labels.push(otherText.value.trim())
    } else {
      labels.push('Other')
    }
  }
  if (labels.length === 0) return
  emit('answer', labels, otherSelected.value ? otherText.value : undefined)
}

function handleDismiss() {
  emit('dismiss')
}

function handleApprove() {
  emit('approve', props.message.id)
}

function handleReject() {
  emit('reject', props.message.id)
}

const aiStore = useAIStore()
const { t } = useI18n()
const inExpanded = ref(true)
const outExpanded = ref(false)
const copyMdLabel = ref(t('ai.copyMarkdown'))

const COPY_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>'
const CHECK_ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>'

async function copyAsMarkdown() {
  try {
    await navigator.clipboard.writeText(props.message.content)
    copyMdLabel.value = t('ai.copied')
    setTimeout(() => { copyMdLabel.value = t('ai.copyMarkdown') }, 2000)
  } catch {
    copyMdLabel.value = t('ai.copyFailed')
    setTimeout(() => { copyMdLabel.value = t('ai.copyMarkdown') }, 2000)
  }
}

const copiedTool = ref('')

async function copyToolText(text: string, key: string) {
  try {
    await navigator.clipboard.writeText(text)
    copiedTool.value = key
    setTimeout(() => { copiedTool.value = '' }, 2000)
  } catch {
    // ignore
  }
}

function onTextClick(event: MouseEvent) {
  const btn = (event.target as HTMLElement).closest('.code-copy-btn') as HTMLElement | null
  if (!btn) return
  const wrapper = btn.closest('.code-block-wrapper')
  const code = wrapper?.querySelector('code')?.textContent
  if (code) {
    navigator.clipboard.writeText(code)
    btn.innerHTML = CHECK_ICON
    setTimeout(() => { btn.innerHTML = COPY_ICON }, 2000)
  }
}

function snakeToCamel(s: string): string {
  return s.replace(/_([a-z])/g, (_, c) => c.toUpperCase())
}

function formatToolName(tc: { function: { name: string; arguments: string } }): string {
  const name = tc.function.name
  const camel = snakeToCamel(name)
  const key = `ai.tool.${camel}`
  const translated = t(key) !== key ? t(key) : name
  try {
    const args = JSON.parse(tc.function.arguments)
    if (name === 'execute_command' && args.timeout) {
      return `${translated} [${args.timeout}s]`
    }
    if (name === 'collect_output' && args.timeout) {
      return `${translated} [${args.timeout}s]`
    }
  } catch {}
  return translated
}

function formatToolBody(tc: { function: { name: string; arguments: string } }): string {
  const name = tc.function.name
  try {
    const args = JSON.parse(tc.function.arguments)
    switch (name) {
      case 'execute_command':
      case 'start_command':
        return args.command || ''
      case 'capture_terminal': {
        return `tail: ${args.tail_lines ?? 50}`
      }
      case 'collect_output': {
        return `head: ${args.head_lines ?? 50}, tail: ${args.tail_lines ?? 150}`
      }
      case 'send_terminal_key': {
        if (args.control) {
          const ctrlMap: Record<string, string> = { ctrl_c: 'Ctrl+C', ctrl_d: 'Ctrl+D', enter: 'Enter' }
          return ctrlMap[args.control] || args.control
        }
        return args.input || ''
      }
      case 'interrupt_command':
        return 'Ctrl+C'
      case 'ask_user':
        return args.question || ''
      default:
        return JSON.stringify(args, null, 2)
    }
  } catch {
    return tc.function.arguments
  }
}

function getToolResult(toolCallId: string): AIMessage | undefined {
  return aiStore.messages.find(
    m => m.role === 'tool' && m.tool_call_id === toolCallId
  )
}

function renderMarkdown(text: string): string {
  let html = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')

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
  const TASK_CHECKED = '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect><path d="m9 12 2 2 4-4"></path></svg>'
  const TASK_UNCHECKED = '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect></svg>'
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

function highlightText(html: string, query: string): string {
  const regex = new RegExp(escaped, 'gi')
  // Split on < to isolate text from HTML tags.
  // Even-indexed segments (after join) are outside tags; odd are inside.
  // Segment 0 is text before first <
  // Segment N after < contains: tag till > then text
  const parts = html.split('<')
  let result = parts[0].replace(regex, '<mark class="ai-search-highlight">$&</mark>')
  for (let i = 1; i < parts.length; i++) {
    const gt = parts[i].indexOf('>')
    if (gt === -1) {
      // Malformed — no closing >, treat entire segment as text
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
  } else if (props.message.role === 'user') {
    html = escapeHtml(props.message.content)
  } else {
    html = renderMarkdown(props.message.content)
  }
  if (props.searchText) {
    html = highlightText(html, props.searchText)
  }
  return html
})

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\n/g, '<br>')
}
</script>

<style scoped>
/* Search highlights — :deep() needed because mark tags are injected via v-html */
:deep(mark.ai-search-highlight) {
  background: rgba(250, 204, 21, 0.4);
  color: inherit;
  border-radius: 2px;
}
:deep(mark.ai-search-highlight.active) {
  background: rgba(250, 204, 21, 0.6);
  color: inherit;
  outline: 2px solid var(--warning);
  border-radius: 2px;
}
.ai-message {
  display: flex;
  padding: 14px 18px;
}
.ai-message.user .content {
  display: flex;
  flex-direction: column;
}
.ai-message.user .text {
  background: var(--bg-surface);
  padding: 10px 16px;
  border-radius: var(--radius-md);
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}
.ai-message.interrupted .text {
  font-style: italic;
  color: var(--text-muted);
  opacity: 0.75;
}
.ai-message.interrupted .text :deep(.interrupted-text) {
  font-style: italic;
  color: var(--text-muted);
  opacity: 0.75;
}
.content {
  flex: 1;
  min-width: 0;
}
.text {
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-primary);
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-ui);
  user-select: text;
  -webkit-user-select: text;
}
.text :deep(.code-block-wrapper) {
  margin: 6px 0;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  overflow: hidden;
}
.text :deep(.code-block-header) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 3px 8px;
  background: var(--bg-overlay);
  border-bottom: 1px solid var(--border-subtle);
}
.text :deep(.code-lang-tag) {
  font-size: 10px;
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
  padding: 10px 12px;
  border-radius: var(--radius-sm);
  overflow-x: auto;
  margin: 6px 0;
  border: 1px solid var(--border-subtle);
}
.text :deep(code) {
  background: var(--bg-base);
  padding: 2px 5px;
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--accent);
}

/* Headings */
.text :deep(h1) { font-size: 16px; margin: 8px 0 4px; }
.text :deep(h2) { font-size: 15px; margin: 8px 0 4px; }
.text :deep(h3) { font-size: 14px; margin: 6px 0 4px; }
.text :deep(h4) { font-size: 13px; margin: 6px 0 4px; }
.text :deep(h5) { font-size: 12px; margin: 4px 0 2px; }
.text :deep(h6) { font-size: 12px; margin: 4px 0 2px; color: var(--text-muted); }

/* Lists */
.text :deep(ul),
.text :deep(ol) {
  padding-left: 0;
  margin: 4px 0;
  list-style-position: inside;
}
.text :deep(li) {
  margin: 2px 0;
}
.text :deep(li ul),
.text :deep(li ol) {
  padding-left: 12px;
}
.text :deep(ol ol) { list-style-type: lower-alpha; }
.text :deep(ol ol ol) { list-style-type: lower-roman; }
.text :deep(ul ul) { list-style-type: circle; }
.text :deep(ul ul ul) { list-style-type: square; }
.text :deep(.task-check) {
  display: inline-flex;
  vertical-align: middle;
  margin-right: 4px;
  color: var(--text-muted);
}
.text :deep(.task-check.checked) {
  color: var(--success);
}

/* Links */
.text :deep(p) {
  margin: 5px 0;
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
  margin: 8px 0;
  padding: 6px 8px 6px 12px;
  border-left: 3px solid var(--accent);
  background: var(--bg-overlay);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  color: var(--text-secondary);
}

/* Horizontal rule */
.text :deep(hr) {
  margin: 14px 0;
  border: none;
  border-top: 1px solid var(--border-hover);
}

/* Tables */
.text :deep(table) {
  border-collapse: collapse;
  margin: 4px 0;
  font-size: 12px;
}
.text :deep(th),
.text :deep(td) {
  border: 1px solid var(--border-hover);
  padding: 4px 8px;
  text-align: left;
}
.text :deep(th) {
  background: var(--bg-overlay);
  font-weight: bold;
}

/* Tool boxes */
.tool-box {
  margin-top: 6px;
  border-radius: var(--radius-sm);
  overflow: hidden;
  font-size: 12px;
}
.tool-box-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  cursor: pointer;
  user-select: none;
}
.tool-box-label {
  font-weight: bold;
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  text-transform: uppercase;
}
.tool-box-count {
  flex: 1;
  color: var(--text-muted);
}
.toggle-icon {
  color: var(--text-muted);
  font-size: 10px;
  cursor: pointer;
}
.tool-copy-btn {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 11px;
  padding: 0 2px;
  opacity: 0;
  transition: opacity 0.15s;
  color: var(--text-muted);
}
.tool-box-header:hover .tool-copy-btn {
  opacity: 0.6;
}
.tool-copy-btn:hover {
  opacity: 1 !important;
}
/* Replicate tool-copy-btn styles for v-html code block buttons (scoped styles don't penetrate v-html) */
.text :deep(.code-block-header .tool-copy-btn) {
  background: none;
  border: none;
  cursor: pointer;
  padding: 0 2px;
  opacity: 0.4;
  transition: opacity 0.15s;
  color: var(--text-muted);
  display: inline-flex;
  align-items: center;
}
.text :deep(.code-block-header .tool-copy-btn:hover) {
  opacity: 1 !important;
}
.tool-box-body {
  padding: 6px 8px;
}

/* IN box - success themed */
.in-box {
  background: var(--success-subtle);
  border: 1px solid var(--success-glow);
}
.in-box .tool-box-header {
  background: var(--success-subtle);
}
.in-box .tool-box-label {
  background: var(--success);
  color: var(--bg-base);
}
.tool-box-name {
  flex: 1;
  font-size: 11px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tool-detail {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
}
.tool-call-item {
  margin-bottom: 6px;
}
.tool-call-item:last-child {
  margin-bottom: 0;
}
.tool-call-name {
  font-weight: bold;
  color: var(--success);
  margin-bottom: 2px;
}
.tool-call-args {
  margin: 0;
  padding: 4px 6px;
  background: var(--bg-base);
  border-radius: 3px;
  color: var(--text-secondary);
  font-family: var(--font-mono);
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-word;
}

/* OUT box - accent themed */
.out-box {
  background: var(--accent-subtle);
  border: 1px solid var(--accent-glow);
}
.out-box .tool-box-header {
  background: var(--accent-subtle);
}
.out-box .tool-box-label {
  background: var(--accent);
  color: var(--on-accent);
}
.tool-pairs {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
}
.tool-pair {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.tool-output {
  margin: 0;
  padding: 4px 6px;
  background: var(--bg-base);
  border-radius: 3px;
  color: var(--text-secondary);
  font-family: var(--font-mono);
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 300px;
  overflow-y: auto;
}

/* Pending tools */
.pending-tools {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 8px;
}
.pending-tool.dangerous {
  border-color: var(--error);
  background: var(--error-subtle);
}
.danger-badge {
  margin-left: 8px;
  font-size: 10px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 3px;
  background: var(--error);
  color: var(--on-accent);
  text-transform: uppercase;
}
.pending-tool {
  margin-top: 8px;
  padding: 8px;
  background: var(--bg-surface);
  border: 1px solid var(--border-hover);
  border-radius: var(--radius-sm);
}
.tool-name {
  font-size: 11px;
  color: var(--text-muted);
  text-transform: uppercase;
}
.pending-count {
  color: var(--text-secondary);
  font-weight: 500;
}
.tool-args {
  display: block;
  margin: 4px 0;
  font-size: 12px;
  color: var(--text-primary);
  white-space: pre-wrap;
}
.tool-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.copy-action {
  margin-top: 4px;
}
.copy-md-btn {
  display: inline-flex;
  align-items: center;
  gap: 0;
  padding: 2px 4px;
  font-size: 11px;
  color: var(--text-muted);
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  cursor: pointer;
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
  gap: 4px;
  padding: 2px 8px;
  color: var(--accent);
  border-color: var(--accent-glow);
  background: var(--accent-subtle);
}
.copy-md-btn:hover .copy-md-label {
  max-width: 150px;
  opacity: 1;
  padding-left: 2px;
}
.continue-box {
  margin-top: 8px;
}

/* Question box (ask_user) */
.question-box {
  margin-top: 8px;
  padding: 10px;
  background: var(--bg-surface);
  border: 1px solid var(--accent-glow);
  border-radius: var(--radius-sm);
}
.question-header {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--accent);
  margin-bottom: 4px;
}
.question-text {
  font-size: 12px;
  color: var(--text-primary);
  margin-bottom: 8px;
  line-height: 1.5;
}
.question-options {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 8px;
}
.question-option {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 8px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
  user-select: none;
}
.question-option:hover {
  border-color: var(--accent-glow);
  background: var(--accent-subtle);
}
.question-option.selected {
  border-color: var(--accent);
  background: var(--accent-subtle);
}
.option-radio {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 2px solid var(--border-hover);
  flex-shrink: 0;
  margin-top: 2px;
  transition: border-color 0.15s, background 0.15s;
}
.option-radio.on {
  border-color: var(--accent);
  background: var(--accent);
  box-shadow: inset 0 0 0 2px var(--bg-surface);
}
.option-check {
  width: 16px;
  height: 16px;
  border: 1px solid var(--border-hover);
  border-radius: 3px;
  flex-shrink: 0;
  margin-top: 2px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--accent);
  transition: border-color 0.15s, background 0.15s;
}
.question-option.selected .option-check {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--on-accent);
}
.option-check-empty {
  display: block;
  width: 100%;
  height: 100%;
}
.option-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.option-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-primary);
}
.option-desc {
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.4;
}
.other-input {
  width: 100%;
  padding: 3px 6px;
  font-size: 11px;
  font-family: var(--font-ui);
  color: var(--text-primary);
  background: var(--bg-base);
  border: 1px solid var(--border-hover);
  border-radius: 3px;
  outline: none;
  margin-top: 4px;
}
.other-input:focus {
  border-color: var(--accent);
}
.question-actions {
  display: flex;
  gap: 8px;
}
</style>
