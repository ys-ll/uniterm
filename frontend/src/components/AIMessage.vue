<template>
  <div class="ai-message" :class="message.role">
    <div class="avatar">{{ avatar }}</div>
    <div class="content">
      <div class="text" v-html="renderedContent" />

      <div v-if="message.role === 'assistant' && message.content" class="copy-action">
        <button class="copy-md-btn" @click="copyAsMarkdown" :title="t('ai.copyMarkdown')">
          <el-icon><Copy :size="14" /></el-icon>
          <span class="copy-md-label">{{ copyMdLabel }}</span>
        </button>
      </div>

      <div v-if="isError && aiStore.lastDebugInfo" class="debug-actions">
        <button class="debug-copy-btn" @click="copyDebugInfo">
          <el-icon><Copy :size="14" /></el-icon>
          {{ copyLabel }}
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
}>()

const isPending = computed(() =>
  aiStore.pendingCommand?.messageId === props.message.id
)
const pendingCmd = computed(() => isPending.value ? aiStore.pendingCommand! : null)

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
const copyLabel = ref(t('ai.copyDebug'))
const copyMdLabel = ref(t('ai.copyMarkdown'))

const avatar = computed(() => {
  if (props.message.role === 'user') return t('ai.avatarUser')
  if (props.message.role === 'tool') return t('ai.avatarTool')
  return t('ai.avatarAI')
})

const isError = computed(() => {
  const content = props.message.content
  if (!content) return false
  const hasErrorText = content.includes('[Error:')
  // Assistant errors (legacy) or display-only tool errors
  return (props.message.role === 'assistant' || (props.message.role === 'tool' && !props.message.tool_call_id)) && hasErrorText
})

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

async function copyDebugInfo() {
  const info = aiStore.lastDebugInfo
  if (!info) return
  const text = `## Error\n\n${info.error}\n\n## Request\n\n\`\`\`json\n${info.request}\n\`\`\``
  try {
    await navigator.clipboard.writeText(text)
    copyLabel.value = t('ai.copied')
    setTimeout(() => { copyLabel.value = t('ai.copyDebug') }, 2000)
  } catch {
    copyLabel.value = t('ai.copyFailed')
    setTimeout(() => { copyLabel.value = t('ai.copyDebug') }, 2000)
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
  html = html.replace(/```([\s\S]*?)```/g, (_, code) => {
    const idx = protectedBlocks.length
    protectedBlocks.push(`<pre><code>${code.trim()}</code></pre>`)
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

  html = html.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\*(.*?)\*/g, '<em>$1</em>')
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>')

  const ulBlocks = html.match(/(?:^- .*\n?)+/gm)
  if (ulBlocks) {
    for (const block of ulBlocks) {
      const items = block.replace(/^- (.*)$/gm, '<li>$1</li>')
      html = html.replace(block, '<ul>' + items + '</ul>')
    }
  }

  const olBlocks = html.match(/(?:^\d+\. .*\n?)+/gm)
  if (olBlocks) {
    for (const block of olBlocks) {
      const items = block.replace(/^\d+\. (.*)$/gm, '<li>$1</li>')
      html = html.replace(block, '<ol>' + items + '</ol>')
    }
  }

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

  html = html.replace(/^---+$/gm, '<hr>')

  // Restore protected code blocks
  html = html.replace(/\x00CODEBLOCK(\d+)\x00/g, (_, idx) => protectedBlocks[parseInt(idx)])

  // Convert remaining newlines to <br>, but remove <br> after block-level elements
  html = html.replace(/\n/g, '<br>')
  html = html.replace(/(<\/(h[1-6]|pre|table|ul|ol|hr|li|div|p)>|<hr\/?>)\s*<br>/gi, '$1')
  html = html.replace(/<br>\s*(<(h[1-6]|pre|table|ul|ol|hr|div|p)>)/gi, '$1')
  // Collapse multiple consecutive <br> into one
  html = html.replace(/(<br>\s*)+/g, '<br>')

  return html
}

function highlightText(html: string, query: string): string {
  if (!query) return html
  const escaped = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
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

const renderedContent = computed(() => {
  let html: string
  if (props.message.role === 'user') {
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
  gap: 10px;
  padding: 10px 14px;
}
.ai-message.user .content {
  display: flex;
  flex-direction: column;
}
.ai-message.user .text {
  background: var(--bg-surface);
  padding: 8px 12px;
  border-radius: var(--radius-md);
  box-shadow: inset 0 0 0 1px var(--border-subtle);
}
.avatar {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--accent-dim), var(--accent));
  color: #fff;
  font-size: 9px;
  font-family: var(--font-ui);
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  letter-spacing: 0.5px;
}
.ai-message.user .avatar {
  background: var(--bg-active);
  color: var(--text-secondary);
}
.ai-message.tool .avatar {
  background: linear-gradient(135deg, var(--success-dim), var(--success));
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
.tool-box-body {
  padding: 6px 8px;
}

/* IN box - success themed */
.in-box {
  background: rgba(52, 211, 153, 0.04);
  border: 1px solid rgba(52, 211, 153, 0.15);
}
.in-box .tool-box-header {
  background: rgba(52, 211, 153, 0.08);
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
  background: var(--accent-dim);
  color: #fff;
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
  background: rgba(248, 113, 113, 0.04);
}
.danger-badge {
  margin-left: 8px;
  font-size: 10px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 3px;
  background: var(--error);
  color: #fff;
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

.debug-actions {
  margin-top: 6px;
  display: flex;
  gap: 8px;
}
.debug-copy-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  font-size: 11px;
  color: var(--text-muted);
  background: var(--bg-surface);
  border: 1px solid var(--border-hover);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.15s;
}
.debug-copy-btn:hover {
  color: var(--accent);
  border-color: var(--accent-glow);
  background: var(--accent-subtle);
}
</style>
