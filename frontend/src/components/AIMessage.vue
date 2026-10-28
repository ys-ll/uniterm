<template>
  <div class="ai-message" :class="[message.role, { interrupted: isInterrupted || isTimeout }]">
    <!-- Skill card: 仅显示用了哪个 skill，正文已隐藏进 _contextHeader -->
    <div v-if="message.skillName" class="skill-card">
      <BookOpen :size="'0.8125rem'" class="skill-card-icon" />
      <span class="skill-card-name">{{ message.skillName }}</span>
      <span class="skill-card-src">{{ message.skillSource === 'auto' ? t('ai.skillAuto') : t('ai.skillExplicit') }}</span>
    </div>

    <!-- Command card: 显示命令名 + 参数，正文已展开进 user 消息 -->
    <div v-if="message.commandName" class="skill-card">
      <Terminal :size="'0.8125rem'" class="skill-card-icon" />
      <span class="skill-card-name">{{ message.commandName }}</span>
      <span v-if="message.commandArgs" class="skill-card-src">{{ message.commandArgs }}</span>
    </div>

    <div class="content" v-if="!message.skillName && !message.commandName">
      <AIMessageItem
        :message="message"
        :search-text="searchText"
        @continue="$emit('continue')"
      />

      <!-- Tool call pairs: IN + OUT grouped together -->
      <div v-if="message.tool_calls?.length" class="tool-pairs">
        <AIToolCallItem
          v-for="tc in message.tool_calls"
          :key="tc.id"
          :tool-call="tc"
        />
      </div>

      <div v-if="pendingCmd" class="pending-tools">
        <div class="pending-tool" :class="{ dangerous: pendingCmd.dangerous }">
          <div class="tool-name">
            {{ pendingCmd.toolName }}
            <span v-if="pendingCmd.dangerous" class="danger-badge">{{ t('ai.dangerous') }}</span>
            <span v-if="pendingCmd.panel" class="panel-badge">#{{ pendingCmd.panel }}</span>
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
              <el-icon v-if="selectedOptions.includes(i)"><Check :size="'0.875rem'" /></el-icon>
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
              <el-icon v-if="otherSelected"><Check :size="'0.875rem'" /></el-icon>
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

      <!-- Message timestamp (user questions and assistant answers only) -->
      <div v-if="createdAtLabel && message.role !== 'tool'" class="msg-meta">{{ createdAtLabel }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Check, BookOpen, Terminal } from '@lucide/vue'
import { useAIStore } from '../stores/aiStore'
import { useI18n } from '../i18n'
import { formatClock } from '../utils/timeFormat'
import type { AIMessage } from '../types/ai'
import AIMessageItem from './ai-sidebar/AIMessageItem.vue'
import AIToolCallItem from './ai-sidebar/AIToolCallItem.vue'

const props = defineProps<{ message: AIMessage; searchText?: string }>()
const emit = defineEmits<{
  (e: 'approve', messageId: string): void
  (e: 'reject', messageId: string): void
  (e: 'continue'): void
  (e: 'answer', selectedLabels: string[], customText?: string): void
  (e: 'dismiss'): void
}>()

const aiStore = useAIStore()
const { t } = useI18n()

const isInterrupted = computed(() => props.message.role === 'tool' && props.message.content === '[INTERRUPTED]')
const isTimeout = computed(() => props.message.role === 'tool' && props.message.content === '[TIMEOUT]')

const isPending = computed(() =>
  aiStore.pendingCommand?.messageId === props.message.id,
)
const pendingCmd = computed(() => isPending.value ? aiStore.pendingCommand! : null)

const isPendingQ = computed(() =>
  aiStore.pendingQuestion?.messageId === props.message.id,
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

const createdAtLabel = computed(() => formatClock(props.message.createdAt))
</script>

<style scoped>
.ai-message {
  display: flex;
  padding: 0.875rem 1.125rem;
}
.ai-message.user .content {
  display: flex;
  flex-direction: column;
}
.ai-message.user .text {
  background: var(--bg-surface);
  padding: 0.625rem 1rem;
  border-radius: var(--radius-sm);
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
.msg-meta {
  margin-top: 0.25rem;
  font-size: 0.625rem;
  color: var(--text-muted);
  opacity: 0.75;
  user-select: none;
}
.ai-message.user .msg-meta {
  text-align: right;
}
.tool-pairs {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  margin-top: 0.375rem;
}
/* Pending tools */
.pending-tools {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-top: 0.5rem;
}
.pending-tool.dangerous {
  border-color: var(--error);
  background: var(--error-subtle);
}
.danger-badge {
  margin-left: 0.5rem;
  font-size: 0.625rem;
  font-weight: 600;
  padding: 1px 0.375rem;
  border-radius: var(--radius-sm);
  background: var(--error);
  color: var(--on-accent);
  text-transform: uppercase;
}
.panel-badge {
  margin-left: 0.5rem;
  font-size: 0.625rem;
  font-weight: 500;
  padding: 1px 0.375rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-hover);
  color: var(--text-secondary);
}
.pending-tool {
  margin-top: 0.5rem;
  padding: 0.5rem;
  background: var(--bg-surface);
  border: 1px solid var(--border-hover);
  border-radius: var(--radius-sm);
}
.tool-name {
  font-size: 0.6875rem;
  color: var(--text-muted);
  text-transform: uppercase;
}
.tool-args {
  display: block;
  margin: 0.25rem 0;
  font-size: 0.75rem;
  color: var(--text-primary);
  white-space: pre-wrap;
}
.tool-actions {
  display: flex;
  gap: 0.5rem;
  margin-top: 0.5rem;
}
/* Question box (ask_user) */
.question-box {
  margin-top: 0.5rem;
  padding: 0.625rem;
  background: var(--bg-surface);
  border: 1px solid var(--accent-glow);
  border-radius: var(--radius-sm);
}
.question-header {
  font-size: 0.625rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--accent);
  margin-bottom: 0.25rem;
}
.question-text {
  font-size: 0.75rem;
  color: var(--text-primary);
  margin-bottom: 0.5rem;
  line-height: 1.5;
}
.question-options {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  margin-bottom: 0.5rem;
}
.question-option {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  padding: 0.375rem 0.5rem;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
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
  width: 0.875rem;
  height: 0.875rem;
  border-radius: 50%;
  border: 0.125rem solid var(--border-hover);
  flex-shrink: 0;
  margin-top: 0.125rem;
  transition: border-color 0.15s, background 0.15s;
}
.option-radio.on {
  border-color: var(--accent);
  background: var(--accent);
  box-shadow: inset 0 0 0 0.125rem var(--bg-surface);
}
.option-check {
  width: 1rem;
  height: 1rem;
  border: 1px solid var(--border-hover);
  border-radius: var(--radius-sm);
  flex-shrink: 0;
  margin-top: 0.125rem;
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
  gap: 0.125rem;
  min-width: 0;
}
.option-label {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-primary);
}
.option-desc {
  font-size: 0.6875rem;
  color: var(--text-muted);
  line-height: 1.4;
}
.other-input {
  width: 100%;
  padding: 0.1875rem 0.375rem;
  font-size: 0.6875rem;
  font-family: var(--font-ui);
  color: var(--text-primary);
  background: var(--bg-base);
  border: 1px solid var(--border-hover);
  border-radius: var(--radius-sm);
  outline: none;
  margin-top: 0.25rem;
}
.other-input:focus {
  border-color: var(--accent);
}
.question-actions {
  display: flex;
  gap: 0.5rem;
}
.skill-card {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.25rem 0.625rem;
  background: var(--accent-subtle, rgba(64,158,255,0.1));
  border: 1px solid var(--accent-glow, rgba(64,158,255,0.3));
  border-radius: var(--radius-sm);
  font-size: 0.75rem;
  margin: 0.125rem 0;
}
.skill-card-icon {
  font-size: 0.75rem;
}
.skill-card-name {
  color: var(--accent);
  font-weight: 600;
  font-family: var(--font-mono);
}
.skill-card-src {
  color: var(--text-muted);
  font-size: 0.6875rem;
}
</style>
