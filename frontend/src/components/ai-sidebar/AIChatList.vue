<template>
  <!-- Search bar -->
  <div v-show="searchVisible" class="ai-search-bar">
    <input
      ref="searchInputRef"
      v-model="searchText"
      class="search-input"
      :placeholder="t('ai.searchPlaceholder')"
      @input="onSearchInput"
      @keydown.enter.prevent="onSearchNext"
      @keydown.shift.enter.prevent="onSearchPrev"
      @keydown.escape="closeSearch"
    />
    <span class="search-count" v-if="searchText">{{ currentMatchIndex + 1 }}/{{ totalMatchCount || 0 }}</span>
    <button class="search-btn" @click="onSearchPrev" :title="t('terminal.searchPrev')">
      <ChevronUp :size="'0.875rem'" />
    </button>
    <button class="search-btn" @click="onSearchNext" :title="t('terminal.searchNext')">
      <ChevronDown :size="'0.875rem'" />
    </button>
    <button class="search-btn" @click="closeSearch" :title="t('ai.close')">
      <el-icon><X :size="'0.75rem'" /></el-icon>
    </button>
  </div>

  <div ref="messagesRef" class="ai-messages" @contextmenu="onAIContextMenu">
    <div
      v-for="msg in visibleMessages"
      :key="msg.id"
      @contextmenu.stop.prevent="onMessageContextMenu($event, msg.id)"
    >
      <AIMessage
        :message="msg"
        :search-text="searchText"
        @approve="onApprove"
        @reject="onReject"
        @continue="onContinue"
        @answer="onAnswer"
        @dismiss="onDismiss"
      />
    </div>
    <div v-if="aiStore.isRunning || aiStore.pendingCommand || aiStore.pendingQuestion" class="ai-thinking">
      <div class="thinking-row" :title="t('ai.thinkingToggleHint')" @click="toggleThinking">
        <div class="thinking-text">{{ statusText }}</div>
        <ChevronDown v-if="!aiStore.thinkingExpanded" :size="'0.75rem'" class="thinking-chevron" />
        <ChevronUp v-else :size="'0.75rem'" class="thinking-chevron" />
      </div>
      <div v-show="aiStore.thinkingExpanded" ref="thinkingPanelRef" class="thinking-panel">
        <div class="thinking-panel-header">
          <span class="thinking-dot" :class="aiStore.status === 'thinking' ? 'live' : 'done'">{{ aiStore.status === 'thinking' ? '○' : '●' }}</span>
          <span class="thinking-panel-title">{{ t('ai.thoughtProcess') }}</span>
          <span v-if="liveThinkingElapsed" class="thinking-panel-time">{{ liveThinkingElapsed }}</span>
          <span v-if="thinkingStartLabel" class="thinking-panel-time">· {{ thinkingStartLabel }}</span>
        </div>
        <pre v-if="aiStore.thinkingText" class="thinking-body">{{ aiStore.thinkingText }}</pre>
        <div v-else class="thinking-empty">{{ t('ai.noThinkingContent') }}</div>
      </div>
    </div>
  </div>

  <!-- AI messages context menu -->
  <Menu ref="aiMenuRef" v-model:visible="aiMenuVisible">
    <MenuItem @click="aiCopySelection">{{ t('terminal.copy') }}</MenuItem>
    <MenuItem @click="aiAskSelection">{{ t('terminal.askAI') }}</MenuItem>
  </Menu>

  <!-- Message context menu: right-click a message bubble for export /
       delete / rollback; falls back to copy / ask-AI when text is selected. -->
  <Menu ref="msgCtxMenuRef" v-model:visible="msgCtxMenuVisible">
    <template v-if="msgCtxId">
      <template v-if="msgCtxHasSelection">
        <MenuItem @click="aiCopySelection">{{ t('terminal.copy') }}</MenuItem>
        <MenuItem @click="aiAskSelection">{{ t('terminal.askAI') }}</MenuItem>
        <MenuDivider />
      </template>
      <MenuItem @click="onMsgCtxExport">{{ t('ai.exportMd') }}</MenuItem>
      <MenuDivider />
      <MenuItem @click="onMsgCtxDelete(msgCtxId)">{{ t('ai.deleteMessage') }}</MenuItem>
      <MenuItem @click="onMsgCtxTruncate(msgCtxId)">{{ t('ai.truncateFrom') }}</MenuItem>
    </template>
  </Menu>
</template>

<script setup lang="ts">
import { ref, nextTick, computed, watch, onMounted, onUnmounted } from 'vue'
import { X, ChevronDown, ChevronUp } from '@lucide/vue'
import { ElMessage } from 'element-plus'
import { useAIStore } from '../../stores/aiStore'
import { useI18n } from '../../i18n'
import { SaveFileDialogFiltered, WriteFileBase64 } from '../../../bindings/github.com/ys-ll/uniterm/app'
import AIMessage from '../AIMessage.vue'
import Menu from '../Menu.vue'
import MenuItem from '../MenuItem.vue'
import MenuDivider from '../MenuDivider.vue'
import { writeClipboard } from '../../composables/useClipboardWrite'
import { formatClock, formatDuration } from '../../utils/timeFormat'
import { useAIChat } from '../../composables/useAIChat'

const props = defineProps<{
  searchVisible: boolean
}>()

const emit = defineEmits<{
  'update:searchVisible': [value: boolean]
  'ask-ai': [text: string]
}>()

const aiStore = useAIStore()
const { t } = useI18n()
const { visibleMessages, statusText, approve, reject, answer, dismiss, continueChat } = useAIChat()

// ── Search ──
const searchText = ref('')
const searchInputRef = ref<HTMLInputElement>()
const currentMatchIndex = ref(0)
const totalMatchCount = ref(0)

function onSearchInput() {
  currentMatchIndex.value = 0
  highlightMatches()
}

function highlightMatches() {
  nextTick(() => {
    const marks = messagesRef.value?.querySelectorAll('mark.ai-search-highlight')
    totalMatchCount.value = marks?.length || 0
    updateActiveMark()
  })
}

function updateActiveMark() {
  const marks = messagesRef.value?.querySelectorAll('mark.ai-search-highlight')
  marks?.forEach((m, i) => {
    m.classList.toggle('active', i === currentMatchIndex.value)
  })
  if (marks && marks[currentMatchIndex.value]) {
    marks[currentMatchIndex.value].scrollIntoView({ block: 'center', behavior: 'smooth' })
  }
}

function onSearchNext() {
  if (totalMatchCount.value === 0) return
  currentMatchIndex.value = (currentMatchIndex.value + 1) % totalMatchCount.value
  updateActiveMark()
}

function onSearchPrev() {
  if (totalMatchCount.value === 0) return
  currentMatchIndex.value = (currentMatchIndex.value - 1 + totalMatchCount.value) % totalMatchCount.value
  updateActiveMark()
}

function closeSearch() {
  emit('update:searchVisible', false)
  searchText.value = ''
  currentMatchIndex.value = 0
  totalMatchCount.value = 0
}

// Focus the search input when the bar opens
watch(
  () => props.searchVisible,
  (visible) => {
    if (visible) nextTick(() => searchInputRef.value?.focus())
  },
)

// Watch for DOM changes (messages loaded/streamed) to re-count highlights
watch(
  () => [searchText.value, visibleMessages.value.length],
  () => {
    if (searchText.value) highlightMatches()
  },
)

// ── Live thinking box (click the status text to expand/collapse) ──
const thinkingPanelRef = ref<HTMLDivElement>()

function scrollThinkingToBottom() {
  const el = thinkingPanelRef.value
  if (el) el.scrollTop = el.scrollHeight
}

function toggleThinking() {
  aiStore.thinkingExpanded = !aiStore.thinkingExpanded
  if (aiStore.thinkingExpanded) nextTick(scrollThinkingToBottom)
}

watch(
  () => aiStore.thinkingText,
  () => {
    if (aiStore.thinkingExpanded) nextTick(scrollThinkingToBottom)
  },
)

// 1s tick feeding the live elapsed counter while the model is thinking.
const thinkingTick = ref(0)
let thinkingTickTimer: ReturnType<typeof setInterval> | null = null
watch(
  () => aiStore.thinkingExpanded && aiStore.isRunning && aiStore.status === 'thinking',
  (on) => {
    if (on && thinkingTickTimer === null) {
      thinkingTick.value = Date.now()
      thinkingTickTimer = setInterval(() => {
        thinkingTick.value = Date.now()
      }, 1000)
    } else if (!on && thinkingTickTimer !== null) {
      clearInterval(thinkingTickTimer)
      thinkingTickTimer = null
    }
  },
  { immediate: true },
)
onUnmounted(() => {
  if (thinkingTickTimer !== null) clearInterval(thinkingTickTimer)
})

const liveThinkingElapsed = computed(() => {
  if (aiStore.status !== 'thinking' || !aiStore.thinkingStartedAt) return ''
  return formatDuration(Math.max(0, thinkingTick.value - aiStore.thinkingStartedAt))
})

const thinkingStartLabel = computed(() => formatClock(aiStore.thinkingStartedAt))

// ── Messages scroll ──
const messagesRef = ref<HTMLDivElement>()
const isAtBottom = ref(true)
let messagesObserver: MutationObserver | null = null

function scrollToBottom() {
  nextTick(() => {
    if (messagesRef.value) {
      messagesRef.value.scrollTop = messagesRef.value.scrollHeight
      isAtBottom.value = true
    }
  })
}

function onMessagesScroll() {
  if (!messagesRef.value) return
  const el = messagesRef.value
  isAtBottom.value = el.scrollTop + el.clientHeight >= el.scrollHeight - 30
}

function autoScrollToBottom() {
  if (isAtBottom.value && messagesRef.value) {
    messagesRef.value.scrollTop = messagesRef.value.scrollHeight
  }
}

// ── AI context menu (right-click on messages area) ──
const aiMenuVisible = ref(false)
const aiMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const aiSelectionText = ref('')

function captureAISelection() {
  aiSelectionText.value = window.getSelection()?.toString() || ''
}

function onAIContextMenu(e: MouseEvent) {
  e.preventDefault()
  captureAISelection()
  // Position at the pointer; Menu.openAt is viewport-clamped + single-open.
  aiMenuRef.value?.openAt(e.clientX, e.clientY)
}

function aiCopySelection() {
  if (aiSelectionText.value) void writeClipboard(aiSelectionText.value)
  aiMenuVisible.value = false
  msgCtxMenuVisible.value = false
}

function aiAskSelection() {
  if (aiSelectionText.value) {
    emit('ask-ai', aiSelectionText.value)
    if (!aiStore.visible) {
      aiStore.visible = true
    }
  }
  aiMenuVisible.value = false
  msgCtxMenuVisible.value = false
}

// ── Message context menu ──
const msgCtxMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const msgCtxMenuVisible = ref(false)
const msgCtxId = ref('')
const msgCtxHasSelection = ref(false)

function onMessageContextMenu(e: MouseEvent, id: string) {
  e.preventDefault()
  captureAISelection()
  msgCtxId.value = id
  msgCtxHasSelection.value = !!(window.getSelection()?.toString())
  msgCtxMenuRef.value?.openAt(e.clientX, e.clientY, id)
}

function onMsgCtxDelete(id: string) {
  msgCtxMenuVisible.value = false
  aiStore.deleteMessage(id)
}

function onMsgCtxTruncate(id: string) {
  msgCtxMenuVisible.value = false
  aiStore.truncateFrom(id)
}

// ── Markdown export of the current session (message context menu) ──
async function onMsgCtxExport() {
  msgCtxMenuVisible.value = false
  const id = aiStore.currentSessionId
  const s = aiStore.sessions.find((x) => x.id === id)
  if (!id || !s) return
  try {
    const md = aiStore.exportSessionMarkdown(id)
    const safe = s.name.replace(/[\\/:*?"<>|]/g, '_').slice(0, 60)
    const path = await SaveFileDialogFiltered(t('ai.exportMd'), `${safe}.md`, 'Markdown File', '*.md')
    if (!path) return
    const bytes = new TextEncoder().encode(md)
    let bin = ''
    for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i])
    await WriteFileBase64(path, btoa(bin))
    ElMessage.success(t('ai.exportDone', { name: s.name }))
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

// ── Message action handlers (approve/reject/answer/dismiss/continue) ──
async function onApprove(messageId: string) {
  await approve(messageId)
  scrollToBottom()
}

function onReject(messageId: string) {
  reject(messageId)
  scrollToBottom()
}

function onAnswer(selectedLabels: string[], customText?: string) {
  answer(selectedLabels, customText)
  scrollToBottom()
}

function onDismiss() {
  dismiss()
  scrollToBottom()
}

async function onContinue() {
  await continueChat()
  scrollToBottom()
}

// ── Session switch → scroll to bottom ──
watch(
  () => aiStore.currentSessionId,
  () => {
    isAtBottom.value = true
    scrollToBottom()
  },
)

onMounted(() => {
  if (messagesRef.value) {
    messagesRef.value.addEventListener('scroll', onMessagesScroll)
    messagesObserver = new MutationObserver(() => {
      if (isAtBottom.value) {
        autoScrollToBottom()
      }
    })
    messagesObserver.observe(messagesRef.value, { childList: true, subtree: true })
  }
  scrollToBottom()
})

onUnmounted(() => {
  if (messagesRef.value) {
    messagesRef.value.removeEventListener('scroll', onMessagesScroll)
  }
  messagesObserver?.disconnect()
})

defineExpose({ scrollToBottom })
</script>

<style scoped>
.ai-search-bar {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.375rem 0.625rem;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
}
.ai-search-bar .search-input {
  flex: 1;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 0.25rem 0.5rem;
  color: var(--text-primary);
  font-family: var(--font-ui);
  font-size: 0.75rem;
  outline: none;
}
.ai-search-bar .search-input:focus {
  border-color: var(--accent);
}
.ai-search-bar .search-count {
  font-size: 0.6875rem;
  color: var(--text-muted);
  white-space: nowrap;
}
.ai-search-bar .search-btn {
  font-size: 0.875rem;
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  color: var(--text-muted);
  padding: 0.125rem;
}
.ai-search-bar .search-btn:hover {
  color: var(--text-primary);
}
.ai-messages {
  flex: 1;
  overflow-y: auto;
  padding: 0.5rem 0;
  user-select: text;
  -webkit-user-select: text;
}
.ai-thinking {
  display: flex;
  flex-direction: column;
  padding: 0.625rem 0.875rem;
}
.thinking-row {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  align-self: flex-start;
  cursor: pointer;
}
.thinking-row:hover .thinking-text {
  color: var(--text-secondary);
}
.thinking-text {
  font-size: 0.6875rem;
  font-family: var(--font-ui);
  color: var(--text-muted);
  font-style: italic;
  animation: status-pulse 1.2s ease-in-out infinite;
}
.thinking-chevron {
  color: var(--text-muted);
  flex-shrink: 0;
}
.thinking-panel {
  margin-top: 0.375rem;
  max-height: 12rem;
  overflow-y: auto;
  border-left: 2px solid var(--border-subtle);
  background: var(--bg-elevated);
  border-radius: var(--radius-sm);
  padding: 0.375rem 0.625rem;
}
.thinking-panel-header {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding-bottom: 0.25rem;
  margin-bottom: 0.25rem;
  border-bottom: 1px solid var(--border-subtle);
  user-select: none;
}
.thinking-dot {
  font-size: 0.6875rem;
  line-height: 1;
}
.thinking-dot.live {
  color: var(--accent);
  animation: status-pulse 1.2s ease-in-out infinite;
}
.thinking-dot.done {
  color: var(--success);
}
.thinking-panel-title {
  font-size: 0.625rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  color: var(--text-muted);
}
.thinking-panel-time {
  font-size: 0.625rem;
  color: var(--text-muted);
  opacity: 0.85;
  font-variant-numeric: tabular-nums;
}
.thinking-body {
  margin: 0;
  font-size: 0.6875rem;
  font-family: var(--font-ui);
  color: var(--text-muted);
  white-space: pre-wrap;
  word-break: break-word;
  user-select: text;
  -webkit-user-select: text;
}
.thinking-empty {
  font-size: 0.6875rem;
  color: var(--text-muted);
  font-style: italic;
}

@keyframes status-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.5;
  }
}
</style>