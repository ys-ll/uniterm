<template>
  <div class="tool-pair">
    <!-- IN box -->
    <div class="tool-box in-box">
      <div class="tool-box-header" @click="inExpanded = !inExpanded">
        <span class="tool-box-label">{{ t('ai.in') }}</span>
        <span class="tool-box-name">{{ formatToolName(toolCall) }}</span>
        <span class="tool-box-count"></span>
        <button class="tool-copy-btn" @click.stop="copyToolText(formatToolBody(toolCall), toolCall.id + '-in')" :title="t('ai.copy')">
          <el-icon><Check v-if="copiedTool === toolCall.id + '-in'" :size="'0.875rem'" /><Copy v-else :size="'0.875rem'" /></el-icon>
        </button>
        <span class="toggle-icon">{{ inExpanded ? '▼' : '▶' }}</span>
      </div>
      <div v-show="inExpanded" class="tool-box-body">
        <pre class="tool-call-args" v-if="formatToolBody(toolCall)">{{ formatToolBody(toolCall) }}</pre>
      </div>
    </div>

    <!-- OUT box -->
    <div v-if="getToolResult(toolCall.id)" class="tool-box out-box">
      <div class="tool-box-header" @click="outExpanded = !outExpanded">
        <span class="tool-box-label">{{ t('ai.out') }}</span>
        <span class="tool-box-count"></span>
        <button class="tool-copy-btn" @click.stop="copyToolText(getToolResult(toolCall.id)?.content || '', toolCall.id + '-out')" :title="t('ai.copy')">
          <el-icon><Check v-if="copiedTool === toolCall.id + '-out'" :size="'0.875rem'" /><Copy v-else :size="'0.875rem'" /></el-icon>
        </button>
        <span class="toggle-icon">{{ outExpanded ? '▼' : '▶' }}</span>
      </div>
      <div v-show="outExpanded" class="tool-box-body">
        <pre class="tool-output">{{ getToolResult(toolCall.id)?.content }}</pre>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Copy, Check } from '@lucide/vue'
import { useAIStore } from '../../stores/aiStore'
import { useI18n } from '../../i18n'
import type { AIMessage, ToolCall } from '../../types/ai'

defineProps<{
  toolCall: ToolCall
}>()

const aiStore = useAIStore()
const { t } = useI18n()

const inExpanded = ref(true)
const outExpanded = ref(false)
const copiedTool = ref('')

async function copyToolText(text: string, key: string) {
  try {
    await navigator.clipboard.writeText(text)
    copiedTool.value = key
    setTimeout(() => {
      copiedTool.value = ''
    }, 2000)
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
    // Show the effective wait: the model-provided timeout when present, or
    // the runtime fallback defaults from agent.ts (execute_command 60s,
    // collect_output 30s) — so the badge never disappears just because the
    // model omitted the optional argument.
    if (name === 'execute_command' || name === 'collect_output') {
      const t = args.timeout
      const seconds = typeof t === 'number' && Number.isFinite(t) && t > 0
        ? t
        : (name === 'execute_command' ? 60 : 30)
      return `${translated} [${seconds}s]`
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
      case 'use_skill':
      case 'save_skill':
      case 'read_skill_file':
        return args.name || ''
      default:
        return JSON.stringify(args, null, 2)
    }
  } catch {
    return tc.function.arguments
  }
}

function getToolResult(toolCallId: string): AIMessage | undefined {
  return aiStore.messages.find(
    (m) => m.role === 'tool' && m.tool_call_id === toolCallId,
  )
}
</script>

<style scoped>
.tool-pair {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.tool-box {
  margin-top: 0.375rem;
  border-radius: var(--radius-sm);
  overflow: hidden;
  font-size: 0.75rem;
}
.tool-box-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.5rem;
  user-select: none;
}
.tool-box-label {
  font-weight: bold;
  font-size: 0.625rem;
  padding: 1px 0.3125rem;
  border-radius: var(--radius-sm);
  text-transform: uppercase;
}
.tool-box-count {
  flex: 1;
  color: var(--text-muted);
}
.toggle-icon {
  color: var(--text-muted);
  font-size: 0.625rem;
}
.tool-copy-btn {
  background: none;
  border: none;
  font-size: 0.6875rem;
  padding: 0 0.125rem;
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
  padding: 0.375rem 0.5rem;
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
  font-size: 0.6875rem;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tool-call-args {
  margin: 0;
  padding: 0.25rem 0.375rem;
  background: var(--bg-base);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
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
.tool-output {
  margin: 0;
  padding: 0.25rem 0.375rem;
  background: var(--bg-base);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 18.75rem;
  overflow-y: auto;
}
</style>