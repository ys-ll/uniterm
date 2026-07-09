<template>
  <div ref="sidebarEl" class="ai-sidebar" :class="{ collapsed: !aiStore.visible, resizing: isResizing, maximized: isMaximized }" :style="{ width: sidebarWidth + 'px' }">
    <div class="resize-handle" @mousedown="onResizeStart" />
    <div class="ai-header">
      <span>{{ t('ai.title') }}</span>
      <div class="ai-actions">
        <button class="ai-action-btn" @click="onNewSession" :title="t('ai.newSession')">
          <el-icon><MessageSquarePlus :size="14" /></el-icon>
        </button>
        <el-dropdown v-if="aiStore.sessions.length > 0" trigger="click" @command="onSessionCommand">
          <button class="ai-action-btn" :title="t('ai.recentSessions')">
            <el-icon><History :size="14" /></el-icon>
          </button>
          <template #dropdown>
            <el-dropdown-menu class="dark-dropdown">
              <el-dropdown-item v-for="s in aiStore.sessions" :key="s.id" :command="s.id" :class="{ active: s.id === aiStore.currentSessionId }">
                <span class="session-item-name">{{ s.name }}</span>
                <span class="session-time">{{ formatRelativeTime(s.updatedAt) }}</span>
                <el-icon class="session-delete" @click.stop="aiStore.deleteSession(s.id)"><Trash2 :size="14" /></el-icon>
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <button class="ai-action-btn" @click="searchVisible = !searchVisible" :title="t('ai.search')">
          <el-icon><Search :size="14" /></el-icon>
        </button>
        <button class="ai-action-btn" @click="toggleMaximize" :title="isMaximized ? t('ai.restore') : t('ai.maximize')">
          <el-icon><Shrink v-if="isMaximized" :size="14" /><Expand v-else :size="14" /></el-icon>
        </button>
        <button class="ai-action-btn" @click="onClose" :title="t('sidebar.collapse')">
          <el-icon><X :size="14" /></el-icon>
        </button>
      </div>
    </div>

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
        <ChevronUp :size="14" />
      </button>
      <button class="search-btn" @click="onSearchNext" :title="t('terminal.searchNext')">
        <ChevronDown :size="14" />
      </button>
      <button class="search-btn" @click="closeSearch" :title="t('ai.close')">
        <el-icon><X :size="12" /></el-icon>
      </button>
    </div>

    <div ref="messagesRef" class="ai-messages" @contextmenu="onAIContextMenu">
      <AIMessage
        v-for="msg in visibleMessages"
        :key="msg.id"
        :message="msg"
        :search-text="searchText"
        @approve="onApprove"
        @reject="onReject"
        @continue="onContinue"
      />
      <div v-if="aiStore.isRunning && !streamingMsgHasContent" class="ai-thinking">
        <div class="thinking-text">{{ t('ai.thinking') }}</div>
      </div>
    </div>

    <!-- AI messages context menu -->
    <div
      v-show="aiMenuVisible"
      ref="aiMenuRef"
      class="ai-context-menu"
      :style="aiMenuStyle"
      @click.stop
    >
      <div class="ai-menu-item" @click="aiCopySelection">{{ t('terminal.copy') }}</div>
      <div class="ai-menu-item" @click="aiAskSelection">{{ t('terminal.askAI') }}</div>
    </div>

    <div class="ai-input">
      <div class="input-container">
        <div v-if="aiStore.queuedMessages.length" class="queued-area">
          <div v-for="q in aiStore.queuedMessages" :key="q.id" class="queued-chip">
            <span class="queued-text">{{ q.content }}</span>
            <button class="queued-remove" :title="t('ai.queueRemove')" @click="aiStore.removeQueuedMessage(q.id)">
              <X :size="12" />
            </button>
          </div>
        </div>
        <div class="textarea-wrap">
          <el-input
            ref="chatInputRef"
            v-model="input"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 8 }"
            :placeholder="t('ai.placeholder')"
            @keydown.enter="onKeydownEnter"
          />
        </div>
        <div class="input-actions">
          <el-dropdown trigger="click" @command="onModelChange" v-if="settingsStore.settings.ai.models.length > 0">
            <button class="ghost-btn model-btn" :title="currentModelName">{{ currentModelName }}</button>
            <template #dropdown>
              <el-dropdown-menu class="dark-dropdown">
                <el-dropdown-item
                  v-for="m in settingsStore.settings.ai.models"
                  :key="m.id"
                  :command="m.id"
                  :class="{ active: m.id === settingsStore.settings.ai.activeModelId }"
                >
                  {{ m.name }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <div class="input-actions-right">
            <el-dropdown trigger="click" @command="onModeChange">
              <button class="ghost-btn mode-btn" :title="modeLabel">{{ modeLabel }}</button>
              <template #dropdown>
                <el-dropdown-menu class="dark-dropdown">
                  <el-dropdown-item command="confirm_all">
                    <span class="mode-option mode-confirm">{{ t('ai.confirmAll') }}</span>
                  </el-dropdown-item>
                  <el-dropdown-item command="confirm_write">
                    <span class="mode-option mode-write">{{ t('ai.confirmWrite') }}</span>
                  </el-dropdown-item>
                  <el-dropdown-item command="confirm_dangerous">
                    <span class="mode-option mode-warning">{{ t('ai.confirmDangerous') }}</span>
                  </el-dropdown-item>
                  <el-dropdown-item command="bypass">
                    <span class="mode-option mode-auto">{{ t('ai.bypass') }}</span>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <button
              v-if="!(busy && !input.trim())"
              class="send-btn"
              :disabled="!input.trim()"
              :title="busy ? t('ai.queue') : t('ai.send')"
              @click="onSend"
            >
              <ArrowUp :size="18" />
            </button>
            <button v-else class="send-btn stop" :title="t('ai.stop')" @click="onStop">
              <Square :size="15" :fill="'currentColor'" />
            </button>
          </div>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, nextTick, computed, watch, onMounted, onUnmounted } from 'vue'
import { X, Trash2, Expand, Shrink, History, MessageSquarePlus, Search, ChevronDown, ChevronUp, ArrowUp, Square } from '@lucide/vue'
import { useAIStore } from '../stores/aiStore'
import { useSettingsStore } from '../stores/settingsStore'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useI18n } from '../i18n'
import { runAgent, approveTool, rejectTool, continueAgent } from '../services/agent'
import { CancelChatStream } from '../../wailsjs/go/main/App'
import type { ExecutionMode } from '../types/ai'
import AIMessage from './AIMessage.vue'

const aiStore = useAIStore()
const settingsStore = useSettingsStore()
const tabStore = useTabStore()
const panelStore = usePanelStore()
const { t } = useI18n()
const input = ref('')
const chatInputRef = ref<any>(null)

function focusInput() {
  nextTick(() => {
    const el = chatInputRef.value?.$el?.querySelector('textarea')
    if (el instanceof HTMLTextAreaElement) {
      el.focus()
    }
  })
}

function resizeTextarea() {
  nextTick(() => {
    const el = chatInputRef.value?.$el?.querySelector('textarea')
    if (el instanceof HTMLTextAreaElement) {
      el.style.height = 'auto'
      requestAnimationFrame(() => {
        el.style.height = 'auto'
        el.style.height = el.scrollHeight + 'px'
      })
    }
  })
}

const visibleMessages = computed(() => {
  return aiStore.messages.filter(m => {
    if (m.role === 'tool' && m.tool_call_id) return false
    if (m.role === 'tool' && !m.tool_call_id) return true
    if (m.role !== 'assistant') return true
    const hasPending = aiStore.pendingCommand?.messageId === m.id
    return m.content || m.tool_calls?.length || hasPending || m.needsContinue
  })
})

// ── Search ──
const searchVisible = ref(false)
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
  searchVisible.value = false
  searchText.value = ''
  currentMatchIndex.value = 0
  totalMatchCount.value = 0
}

// Watch for DOM changes (messages loaded/streamed) to re-count highlights
watch(() => [searchText.value, visibleMessages.value.length], () => {
  if (searchText.value) highlightMatches()
})
// Hide thinking indicator once streaming content arrives
const streamingMsgHasContent = computed(() => {
  const msgs = aiStore.messages
  if (!aiStore.isRunning || msgs.length === 0) return false
  const last = msgs[msgs.length - 1]
  return last.role === 'assistant' && !!last.content
})

const messagesRef = ref<HTMLDivElement>()
const aiMenuRef = ref<HTMLDivElement>()
const sidebarWidth = ref(360)
const isResizing = ref(false)
const isMaximized = ref(false)
const preMaxWidth = ref(360)

function toggleMaximize() {
  if (isMaximized.value) {
    sidebarWidth.value = preMaxWidth.value
    isMaximized.value = false
    window.dispatchEvent(new CustomEvent('rdp:overlay-pop'))
  } else {
    preMaxWidth.value = sidebarWidth.value
    isMaximized.value = true
    window.dispatchEvent(new CustomEvent('rdp:overlay-push'))
  }
}

function onClose() {
  if (isMaximized.value) {
    isMaximized.value = false
    sidebarWidth.value = preMaxWidth.value
    window.dispatchEvent(new CustomEvent('rdp:overlay-pop'))
  }
  aiStore.toggle()
}
const sidebarEl = ref<HTMLDivElement>()
const aiMenuVisible = ref(false)
const aiMenuStyle = ref({ left: '0px', top: '0px' })
const isAtBottom = ref(true)
let mutationObserver: MutationObserver | null = null

const modeLabel = computed(() => {
  switch (aiStore.mode) {
    case 'bypass': return t('ai.bypass')
    case 'confirm_dangerous': return t('ai.confirmDangerous')
    case 'confirm_write': return t('ai.confirmWrite')
    case 'confirm_all': return t('ai.confirmAll')
    default: return t('ai.confirmDangerous')
  }
})

const currentModelName = computed(() => {
  const m = settingsStore.settings.ai.models.find(m => m.id === settingsStore.settings.ai.activeModelId)
  return m?.name || 'Model'
})

const busy = computed(() => aiStore.isRunning || !!aiStore.pendingCommand)

function onModeChange(mode: string) {
  aiStore.mode = mode as ExecutionMode
}

function onModelChange(modelId: string) {
  settingsStore.setActiveModel(modelId)
  const model = settingsStore.settings.ai.models.find(m => m.id === modelId)
  if (model) {
    aiStore.setConfig({
      apiKey: model.apiKey,
      baseURL: model.baseURL,
      model: model.model,
    })
  }
}

function formatRelativeTime(timestamp: number): string {
  const diff = Date.now() - timestamp
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return t('ai.justNow')
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return t('ai.minutesAgo', { n: minutes })
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return t('ai.hoursAgo', { n: hours })
  const days = Math.floor(hours / 24)
  if (days < 30) return t('ai.daysAgo', { n: days })
  const months = Math.floor(days / 30)
  if (months < 12) return t('ai.monthsAgo', { n: months })
  const years = Math.floor(months / 12)
  return t('ai.yearsAgo', { n: years })
}

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

function closeAIMenu() {
  aiMenuVisible.value = false
}

function onAIContextMenu(e: MouseEvent) {
  e.preventDefault()
  e.stopPropagation()
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  aiMenuStyle.value = fitMenuPosition(e.clientX, e.clientY, 120, 76)
  aiMenuVisible.value = true
}

function fitMenuPosition(x: number, y: number, menuW: number, menuH: number) {
  let left = x
  let top = y
  if (x + menuW > window.innerWidth) left = x - menuW
  if (y + menuH > window.innerHeight) top = y - menuH
  return { left: left + 'px', top: top + 'px' }
}

function aiCopySelection() {
  const selection = window.getSelection()
  if (selection && selection.toString()) {
    navigator.clipboard.writeText(selection.toString())
  }
  closeAIMenu()
}

function aiAskSelection() {
  const selection = window.getSelection()
  if (selection && selection.toString()) {
    input.value = selection.toString()
    if (!aiStore.visible) {
      aiStore.visible = true
    }
  }
  closeAIMenu()
}

function onNewSession() {
  aiStore.createSession()
}

function onSessionCommand(sessionId: string) {
  aiStore.switchSession(sessionId)
}

watch(() => aiStore.currentSessionId, () => {
  isAtBottom.value = true
  scrollToBottom()
})

watch(() => aiStore.visible, (visible) => {
  if (visible) {
    resizeTextarea()
  }
  if (!visible && isMaximized.value) {
    isMaximized.value = false
    sidebarWidth.value = preMaxWidth.value
    window.dispatchEvent(new CustomEvent('rdp:overlay-pop'))
  }
})

function onKeydownEnter(e: KeyboardEvent) {
  if (e.shiftKey) {
    return
  }
  e.preventDefault()
  onSend()
}

async function onSend() {
  const text = input.value.trim()
  if (!text) return
  if (busy.value) {
    aiStore.enqueueMessage(text)
    input.value = ''
    resizeTextarea()
    return
  }
  input.value = ''
  scrollToBottom()
  await runAgent(text)
  scrollToBottom()
}

function onStop() {
  if (aiStore.pendingCommand) {
    const cmd = aiStore.pendingCommand
    aiStore.clearPendingCommand()
    aiStore.addMessage({
      id: `msg-${Date.now()}`,
      role: 'tool',
      content: 'User cancelled this command.',
      tool_call_id: cmd.toolId
    })
    aiStore.clearQueue()
    return
  }
  CancelChatStream().catch(() => { /* ignore */ })
  aiStore.stop()
}

async function onApprove(messageId: string) {
  await approveTool(messageId)
  scrollToBottom()
}

function onReject(messageId: string) {
  rejectTool(messageId)
  scrollToBottom()
}

async function onContinue() {
  await continueAgent()
  scrollToBottom()
}

function onResizeStart(e: MouseEvent) {
  isResizing.value = true
  const el = sidebarEl.value
  if (!el) return
  const startX = e.clientX
  const startWidth = el.offsetWidth

  window.dispatchEvent(new CustomEvent('split:resize-start'))

  function onMouseMove(ev: MouseEvent) {
    if (!isResizing.value) return
    const delta = startX - ev.clientX
    const newWidth = Math.min(Math.max(startWidth + delta, 300), 800)
    if (el) el.style.width = newWidth + 'px'
  }

  function onMouseUp() {
    isResizing.value = false
    sidebarWidth.value = el.offsetWidth
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    window.dispatchEvent(new CustomEvent('split:resize-end'))
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

function onAskAI(e: Event) {
  const text = (e as CustomEvent).detail as string
  if (text) {
    input.value = text
    if (!aiStore.visible) {
      aiStore.visible = true
    }
  }
}

onMounted(() => {
  window.addEventListener('ai:ask', onAskAI)
  window.addEventListener('global:close-context-menus', closeAIMenu)
  document.addEventListener('click', closeAIMenu)

  if (messagesRef.value) {
    messagesRef.value.addEventListener('scroll', onMessagesScroll)
    mutationObserver = new MutationObserver(() => {
      if (isAtBottom.value) {
        autoScrollToBottom()
      }
    })
    mutationObserver.observe(messagesRef.value, { childList: true, subtree: true })
  }
  scrollToBottom()
})

onUnmounted(() => {
  window.removeEventListener('ai:ask', onAskAI)
  window.removeEventListener('global:close-context-menus', closeAIMenu)
  document.removeEventListener('click', closeAIMenu)

  if (messagesRef.value) {
    messagesRef.value.removeEventListener('scroll', onMessagesScroll)
  }
  mutationObserver?.disconnect()
})

defineExpose({ focusInput })
</script>

<style scoped>
.ai-sidebar {
  background: var(--bg-base);
  display: flex;
  flex-direction: column;
  position: relative;
  flex-shrink: 0;
}
.ai-sidebar.collapsed {
  width: 0 !important;
  overflow: hidden;
}
.ai-sidebar.maximized {
  position: absolute !important;
  left: 0;
  top: 0;
  right: 0;
  bottom: 0;
  width: 100% !important;
  z-index: 100;
}
.ai-sidebar.resizing {
  transition: none;
}
.resize-handle {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  cursor: col-resize;
  z-index: 10;
  background: var(--border-subtle);
  transition: background 0.15s ease;
}

.resize-handle:hover {
  background: var(--accent);
}
.ai-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  font-size: 12px;
  font-family: var(--font-ui);
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: 0.5px;
}
.ai-actions {
  display: flex;
  gap: 2px;
}
.ai-action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  transition: all 0.12s ease;
}
.ai-action-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.ai-session-bar {
  padding: 6px 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.session-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 28px;
  padding: 0 10px;
  box-sizing: border-box;
  background: var(--bg-surface);
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-size: 12px;
  font-family: var(--font-ui);
  color: var(--text-primary);
  box-shadow: inset 0 0 0 1px var(--border-subtle);
  transition: all 0.12s ease;
}
.session-trigger:hover {
  background: var(--bg-hover);
  box-shadow: inset 0 0 0 1px var(--border-hover);
}
.session-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.session-item-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.session-time {
  margin-left: 8px;
  font-size: 10px;
  font-family: var(--font-mono);
  color: var(--text-muted);
  white-space: nowrap;
}
.session-delete {
  margin-left: 8px;
  opacity: 0;
  transition: opacity 0.15s;
  color: var(--text-muted);
}
.session-delete:hover {
  color: var(--text-primary);
}
:deep(.el-dropdown-menu__item) {
  display: flex;
  align-items: center;
  font-family: var(--font-ui);
  font-size: 12px;
}
:deep(.el-dropdown-menu__item:hover .session-delete) {
  opacity: 1;
}
:deep(.el-dropdown-menu__item.active) {
  background: var(--success-subtle);
  color: var(--success);
}

:deep(.dark-dropdown) {
  background: var(--bg-surface) !important;
  border: 1px solid var(--border-subtle) !important;
  border-radius: var(--radius-md) !important;
  box-shadow: var(--shadow-md) !important;
}
:deep(.dark-dropdown .el-dropdown-menu__item) {
  color: var(--text-secondary);
}
:deep(.dark-dropdown .el-dropdown-menu__item.is-disabled) {
  color: var(--text-disabled);
}
:deep(.dark-dropdown .el-dropdown-menu__item:not(.is-disabled):hover) {
  background: var(--bg-hover);
  color: var(--text-primary);
}
:deep(.dark-dropdown .el-dropdown-menu__item.divided) {
  border-top: 1px solid var(--border-subtle);
}
:deep(.dark-dropdown .el-dropdown-menu__item.divided::before) {
  background-color: var(--border-subtle);
}
.ai-search-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
}
.ai-search-bar .search-input {
  flex: 1;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 4px 8px;
  color: var(--text-primary);
  font-family: var(--font-ui);
  font-size: 12px;
  outline: none;
}
.ai-search-bar .search-input:focus {
  border-color: var(--accent);
}
.ai-search-bar .search-count {
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
}
.ai-search-bar .search-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  padding: 2px;
}
.ai-search-bar .search-btn:hover {
  color: var(--text-primary);
}
.ai-messages {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
  user-select: text;
  -webkit-user-select: text;
}
.ai-thinking {
  display: flex;
  align-items: center;
  padding: 10px 14px;
}
.thinking-text {
  font-size: 11px;
  font-family: var(--font-ui);
  color: var(--text-muted);
  font-style: italic;
}
.ai-input {
  padding: 10px 16px;
  flex-shrink: 0;
}
.input-container {
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  transition: border-color 0.15s ease;
}
.input-container:focus-within {
  border-color: var(--accent);
}
.textarea-wrap :deep(.el-textarea) {
  box-shadow: none;
  border: none;
}
.textarea-wrap :deep(.el-textarea__inner) {
  padding: 12px 16px;
  font-size: 13px;
  border: none!important;
  box-shadow: none !important;
  background: transparent;
  border-radius: 0;
  resize: none;
  outline: none;
}
.textarea-wrap :deep(.el-textarea__inner:focus) {
  border: none !important;
  box-shadow: none !important;
  outline: none;
}
.textarea-wrap :deep(.el-textarea__inner::placeholder) {
  font-size: 13px;
  color: var(--text-muted);
}
.input-actions {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: center;
  padding: 0 8px 8px 8px;
}
.input-actions-right {
  display: flex;
  gap: 6px;
  align-items: center;
}
/* Ghost buttons: no border/background by default, reveal on hover */
.ghost-btn {
  display: inline-block;
  box-sizing: border-box;
  height: 24px;
  line-height: 24px;
  padding: 0 6px;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  font-family: var(--font-ui);
  font-size: 11px;
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
  transition: background 0.12s ease, color 0.12s ease;
}
.ghost-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.model-btn {
  max-width: 96px;
}
.mode-btn {
  max-width: 108px;
}
/* Send / Stop icon button */
.send-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm);
  background: var(--accent);
  color: var(--on-accent);
  cursor: pointer;
  transition: background 0.12s ease, opacity 0.12s ease;
}
.send-btn:hover:not(:disabled) {
  background: var(--accent-dim);
}
.send-btn:disabled {
  background: var(--bg-active);
  color: var(--text-disabled);
  cursor: not-allowed;
}
.send-btn.stop {
  background: var(--error);
  color: var(--on-accent);
}
.send-btn.stop:hover {
  background: var(--error);
  opacity: 0.85;
}
.mode-option {
  font-size: 12px;
  font-weight: 500;
  font-family: var(--font-ui);
}
.mode-auto {
  color: var(--error);
}
.mode-confirm {
  color: var(--success);
}
.mode-write {
  color: var(--accent);
}
.mode-warning {
  color: var(--warning);
}

.ai-context-menu {
  position: fixed;
  z-index: 9999;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
  min-width: 120px;
  padding: 4px;
  backdrop-filter: blur(8px);
}

.ai-menu-item {
  padding: 7px 14px;
  font-size: 12px;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  transition: all 0.1s ease;
}

.ai-menu-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.queued-area {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 8px 0 8px;
}
.queued-chip {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  font-size: 12px;
  font-family: var(--font-ui);
  color: var(--text-secondary);
}
.queued-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.queued-remove {
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  padding: 0;
}
.queued-remove:hover {
  color: var(--text-primary);
}
</style>
