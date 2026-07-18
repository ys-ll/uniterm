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
        @answer="onAnswer"
        @dismiss="onDismiss"
      />
      <div v-if="aiStore.isRunning || aiStore.pendingCommand || aiStore.pendingQuestion" class="ai-thinking">
        <div class="thinking-text">{{ statusText }}</div>
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
      <!-- Panel tags area -->
      <div class="ai-panel-tags">
        <div class="panel-tags-list">
          <template v-if="lockedPanels.length === 0 && currentIsTerminal">
            <span class="panel-tag panel-tag-default">{{ currentTerminalLabel }}</span>
          </template>
          <template v-else-if="lockedPanels.length > 0">
            <span
              v-for="pid in lockedPanels"
              :key="pid"
              class="panel-tag"
            >
              {{ getPanelDisplayName(pid) }}
              <button class="panel-tag-close" @click="onRemovePanelTag(pid)">&times;</button>
            </span>
          </template>
          <template v-else>
            <span class="no-terminal-hint">{{ t('ai.noTerminalHint') }}</span>
          </template>
          <el-dropdown trigger="click" @command="onAddPanelTag">
            <button class="panel-tag-add-btn" :title="t('ai.addTerminal')">+</button>
            <template #dropdown>
              <el-dropdown-menu class="dark-dropdown">
                <el-dropdown-item
                  v-for="p in availableTerminalPanels"
                  :key="p.id"
                  :command="p.id"
                  :class="{ selected: lockedPanels.includes(p.id) }"
                >
                  <span>{{ getPanelDisplayName(p.id) }}</span>
                  <span class="panel-shell-hint">{{ getPanelShellHint(p.id) }}</span>
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>

      <div class="input-container">
        <!-- # reference dropdown -->
        <div
          v-if="hashDropdownVisible && hashMatchingPanels.length > 0"
          class="hash-dropdown"
        >
          <div
            v-for="(p, i) in hashMatchingPanels"
            :key="p.id"
            class="hash-dropdown-item"
            :class="{ highlighted: i === hashHighlightIndex }"
            @mousedown.prevent="onSelectHashPanel(p.title)"
          >
            <span class="hash-panel-name">#{{ p.title }}</span>
            <span v-if="lockedPanels.includes(p.id)" class="hash-associated-badge">已关联</span>
            <span class="hash-panel-hint">{{ getPanelShellHint(p.id) }}</span>
          </div>
        </div>

        <div v-if="aiStore.queuedMessages.length" class="queued-area">
          <div v-for="q in aiStore.queuedMessages" :key="q.id" class="queued-chip">
            <span class="queued-text">{{ q.content }}</span>
            <button class="queued-remove" :title="t('ai.queueRemove')" @click="aiStore.removeQueuedMessage(q.id)">
              <X :size="12" />
            </button>
          </div>
        </div>
        <div class="textarea-wrap">
          <div
            ref="editableRef"
            class="ai-editable"
            contenteditable="true"
            :data-placeholder="t('ai.placeholder')"
            @input="onEditableInput"
            @keydown="onKeydown"
            @paste="onPaste"
          />
        </div>
        <div class="input-actions">
          <div class="input-actions-left">
            <button class="ghost-btn hash-btn" title="引用终端" @click="onHashButtonClick">
              <span class="hash-btn-icon">#</span>
            </button>
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
                  <el-dropdown-item class="add-model-item" command="__add_model__" :divided="true">
                    <Plus :size="14" class="add-model-icon" />
                    <span>{{ t('settings.addModel') }}</span>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <button v-else class="ghost-btn model-btn add-model-btn" @click="onModelChange('__add_model__')">
            <Plus :size="14" />
            <span>{{ t('settings.addModel') }}</span>
          </button>
          </div>
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
              v-if="!(busy && !inputText.trim())"
              class="send-btn"
              :disabled="!inputText.trim()"
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
import { X, Trash2, Expand, Shrink, History, MessageSquarePlus, Search, ChevronDown, ChevronUp, ArrowUp, Square, Plus } from '@lucide/vue'
import { useAIStore } from '../stores/aiStore'
import { useSettingsStore } from '../stores/settingsStore'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useI18n } from '../i18n'
import { runAgent, approveTool, rejectTool, continueAgent, answerQuestion, dismissQuestion } from '../services/agent'
import { CancelChatStream } from '../../wailsjs/go/main/App'
import { ClipboardGetText } from '../../wailsjs/runtime'
import type { ExecutionMode } from '../types/ai'
import AIMessage from './AIMessage.vue'

const aiStore = useAIStore()
const settingsStore = useSettingsStore()
const tabStore = useTabStore()
const panelStore = usePanelStore()
const { t } = useI18n()
const editableRef = ref<HTMLDivElement | null>(null)

// Derive plain text from contenteditable div (hash-tag spans contribute #PanelName)
function getEditableText(): string {
  const el = editableRef.value
  if (!el) return ''
  let text = ''
  const walk = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      text += node.textContent || ''
    } else if (node instanceof HTMLElement) {
      if (node.classList.contains('hash-tag')) {
        text += node.getAttribute('data-ref') || node.textContent || ''
      } else {
        node.childNodes.forEach(walk)
      }
    }
  }
  el.childNodes.forEach(walk)
  return text
}

// Create a hash-tag span with scoped CSS attribute so styles apply
function createHashTagSpan(panelTitle: string): HTMLSpanElement {
  const span = document.createElement('span')
  span.className = 'hash-tag'
  span.setAttribute('data-ref', '#' + panelTitle)
  span.contentEditable = 'false'
  span.textContent = '#' + panelTitle
  // Copy scoped style attribute from editable div so Vue scoped CSS matches
  const el = editableRef.value
  if (el) {
    for (const attr of el.attributes) {
      if (attr.name.startsWith('data-v-') || attr.name.startsWith('data-v')) {
        span.setAttribute(attr.name, '')
        break
      }
    }
  }
  return span
}

// Computed: input text (for watch)
const inputText = ref('')
function syncInputText() {
  inputText.value = getEditableText()
}

function onEditableInput() {
  syncInputText(); refreshHashDropdown()
  refreshHashDropdown()
}

// MutationObserver as backup — catches changes that don't fire 'input' event
onMounted(() => {
  if (editableRef.value) {
    mutationObserver = new MutationObserver(() => { syncInputText(); refreshHashDropdown() })
    mutationObserver.observe(editableRef.value, { childList: true, subtree: true, characterData: true })
  }
})
onUnmounted(() => {
  mutationObserver?.disconnect()
})

function focusInput() {
  nextTick(() => {
    editableRef.value?.focus()
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
const statusText = computed(() => {
  if (aiStore.pendingCommand) return t('ai.confirming')
  if (aiStore.pendingQuestion) return t('ai.awaitingAnswer')
  const key = `ai.${aiStore.status}` as any
  return t(key) || t('ai.thinking')
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

const busy = computed(() => aiStore.isRunning || !!aiStore.pendingCommand || !!aiStore.pendingQuestion)

// Panel tags
const lockedPanels = computed(() => [...tabStore.aiLockedPanelIds])

const currentIsTerminal = computed(() => {
  const tab = tabStore.activeTab
  if (tab?.type === 'terminal') return true
  if (tab?.type === 'workspace') {
    const pid = (tab as any).activePanelId
    return !!(pid && panelStore.getPanel(pid))
  }
  return false
})

const currentTerminalLabel = computed(() => {
  const tab = tabStore.activeTab
  if (!tab) return t('ai.currentTerminal')
  let panelId: string | undefined
  if (tab.type === 'terminal') {
    panelId = (tab as any).panelId
  } else if (tab.type === 'workspace') {
    panelId = (tab as any).activePanelId
  }
  if (!panelId) return t('ai.currentTerminal')
  const panel = panelStore.getPanel(panelId)
  return panel ? `${t('ai.currentTerminal')}: ${panel.title}` : t('ai.currentTerminal')
})

const availableTerminalPanels = computed(() => {
  const result: Array<{ id: string; title: string; type: string; shellPath?: string; config?: any }> = []
  const seen = new Set<string>()
  for (const tab of tabStore.tabs) {
    if (tab.type === 'terminal' && (tab as any).panelId) {
      const p = panelStore.getPanel((tab as any).panelId)
      if (p && (p.type === 'ssh' || p.type === 'local') && !seen.has(p.id)) {
        seen.add(p.id)
        result.push({ id: p.id, title: p.title, type: p.type, shellPath: p.config?.shellPath, config: p.config })
      }
    }
    if (tab.type === 'workspace' && (tab as any).panelIds) {
      for (const pid of (tab as any).panelIds) {
        const p = panelStore.getPanel(pid)
        if (p && (p.type === 'ssh' || p.type === 'local') && !seen.has(p.id)) {
          seen.add(p.id)
          result.push({ id: p.id, title: p.title, type: p.type, shellPath: p.config?.shellPath, config: p.config })
        }
      }
    }
  }
  return result
})

function getPanelDisplayName(panelId: string): string {
  const p = panelStore.getPanel(panelId)
  if (!p) return panelId
  const dup = availableTerminalPanels.value.filter(ap => ap.title === p.title)
  return dup.length > 1 ? `${p.title} (id: ${p.id})` : p.title
}

function getPanelShellHint(panelId: string): string {
  const p = panelStore.getPanel(panelId)
  if (!p) return ''
  const shellPath = p.config?.shellPath
  if (shellPath) {
    const lower = shellPath.toLowerCase()
    if (lower.includes('bash') || lower.includes('sh')) return 'Bash'
    if (lower.includes('powershell') || lower.includes('pwsh')) return 'PowerShell'
    if (lower.includes('cmd')) return 'CMD'
    if (lower.includes('zsh')) return 'Zsh'
    return shellPath.split(/[\\/]/).pop() || 'Shell'
  }
  if (p.type === 'ssh') return 'SSH'
  return ''
}

function onRemovePanelTag(panelId: string) {
  tabStore.removeAILockedPanel(panelId)
}

function onAddPanelTag(panelId: string) {
  if (tabStore.isPanelAILocked(panelId)) {
    tabStore.removeAILockedPanel(panelId)
  } else {
    tabStore.addAILockedPanel(panelId)
  }
}

// # reference state
const hashQuery = ref('')
const hashDropdownVisible = ref(false)
const hashHighlightIndex = ref(0)

const hashMatchingPanels = computed(() => {
  const src = hashDropdownVisible.value && !hashQuery.value
    ? availableTerminalPanels.value
    : availableTerminalPanels.value
  let list = hashQuery.value
    ? src.filter(p => p.title.toLowerCase().includes(hashQuery.value.toLowerCase()))
    : [...src]
  // Sort: associated panels first
  list = [...list].sort((a, b) => {
    const aLocked = lockedPanels.value.includes(a.id) ? 0 : 1
    const bLocked = lockedPanels.value.includes(b.id) ? 0 : 1
    return aLocked - bLocked
  })
  return list
})

function findLastHashIndex(text: string): number {
  for (let i = text.length - 1; i >= 0; i--) {
    if (text[i] === '#') {
      if (i === 0 || /[\s,;:.(\{\[]/.test(text[i - 1])) {
        return i
      }
    }
  }
  return -1
}

// Detect an active #query at the caret. Works on the DOM text node the caret
// sits in, so an adjacent hash-tag span never interferes with the check.
function detectHashQuery(): string | null {
  const sel = window.getSelection()
  const el = editableRef.value
  if (!sel || !sel.rangeCount || !el) return null
  const node = sel.anchorNode
  if (!node || node.nodeType !== Node.TEXT_NODE || !el.contains(node)) return null
  const caret = sel.anchorOffset
  const before = (node.textContent || '').slice(0, caret)
  // Find last # in this text node before the caret
  const hashIdx = before.lastIndexOf('#')
  if (hashIdx < 0) return null
  const query = before.slice(hashIdx + 1)
  // query must not contain whitespace
  if (/\s/.test(query)) return null
  // char before # (within this node) must be empty or a separator
  if (hashIdx > 0 && !/[\s,;:.(\{\[]/.test(before[hashIdx - 1])) return null
  return query
}

function refreshHashDropdown() {
  const query = detectHashQuery()
  if (query !== null) {
    hashDropdownVisible.value = true
    hashQuery.value = query
    hashHighlightIndex.value = 0
  } else {
    hashDropdownVisible.value = false
    hashQuery.value = ''
  }
}

function onSelectHashPanel(panelTitle: string) {
  const el = editableRef.value
  if (!el) return

  const text = getEditableText()
  const lastHashIdx = -1

  if (lastHashIdx >= 0) {
    // Text-triggered: replace #query with tag span
    const before = text.slice(0, lastHashIdx)
    const after = text.slice(lastHashIdx + 1)
    const spaceIdx = after.indexOf(' ')
    const rest = spaceIdx >= 0 ? after.slice(spaceIdx) : ' '

    const tagSpan = createHashTagSpan(panelTitle)

    // Rebuild content
    el.innerHTML = ''
    if (before) el.appendChild(document.createTextNode(before))
    el.appendChild(tagSpan)
    if (rest) el.appendChild(document.createTextNode(rest))
    // Place cursor after tag, delay to let DOM settle
    nextTick(() => {
      const sel2 = window.getSelection()
      if (sel2) {
        const range = document.createRange()
        range.setStartAfter(tagSpan)
        range.collapse(true)
        sel2.removeAllRanges()
        sel2.addRange(range)
      }
    })
  } else {
    // Button-triggered: append tag span at cursor position
    const sel = window.getSelection()
    // Remove the typed #query first (caret text node), then insert the tag
    if (sel && sel.rangeCount > 0 && sel.anchorNode && sel.anchorNode.nodeType === Node.TEXT_NODE && el.contains(sel.anchorNode)) {
      const tn = sel.anchorNode as Text
      const caretPos = sel.anchorOffset
      const c = tn.textContent || ''
      const hi = c.slice(0, caretPos).lastIndexOf('#')
      if (hi >= 0) {
        const delRange = document.createRange()
        delRange.setStart(tn, hi)
        delRange.setEnd(tn, caretPos)
        delRange.deleteContents()
        sel.removeAllRanges()
        sel.addRange(delRange)
      }
    }
    if (sel && sel.rangeCount > 0 && el.contains(sel.anchorNode)) {
      const range = sel.getRangeAt(0)
      range.collapse(false) // collapse to end

      const tagSpan = createHashTagSpan(panelTitle)

      range.insertNode(tagSpan)
      const trailingBtn = document.createTextNode(' ')
      tagSpan.after(trailingBtn)
      range.setStart(trailingBtn, 0)
      range.collapse(true)
      sel.removeAllRanges()
      sel.addRange(range)
    } else {
      // Fallback: append at end
      const tagSpan = createHashTagSpan(panelTitle)
      el.appendChild(tagSpan)
      el.appendChild(document.createTextNode(' '))
    }
  }

  syncInputText(); refreshHashDropdown()
  hashDropdownVisible.value = false
  hashQuery.value = ''
  hashHighlightIndex.value = 0

  // Auto-add to locked panels
  const panel = availableTerminalPanels.value.find(p => p.title === panelTitle)
  if (panel && !tabStore.isPanelAILocked(panel.id)) {
    tabStore.addAILockedPanel(panel.id)
  }
}

function onHashButtonClick() {
  const el = editableRef.value
  if (!el) return
  el.focus()

  const sel = window.getSelection()
  if (sel && sel.rangeCount > 0 && el.contains(sel.anchorNode)) {
    const range = sel.getRangeAt(0)
    range.deleteContents()
    const textNode = document.createTextNode('#')
    range.insertNode(textNode)
    range.setStart(textNode, 1)
    range.collapse(true)
    sel.removeAllRanges()
    sel.addRange(range)
  } else {
    const textNode = document.createTextNode('#')
    el.appendChild(textNode)
    const range = document.createRange()
    range.setStart(textNode, 1)
    range.collapse(true)
    sel.removeAllRanges()
    sel.addRange(range)
  }
  syncInputText(); refreshHashDropdown()
}

function onEscHashDropdown() {
  hashDropdownVisible.value = false
  hashQuery.value = ''
}

function onModeChange(mode: string) {
  aiStore.mode = mode as ExecutionMode
}

const emit = defineEmits<{
  'open-settings': []
}>()

function onModelChange(modelId: string) {
  if (modelId === '__add_model__') {
    emit('open-settings')
    nextTick(() => {
      settingsStore.openCategory = 'ai'
    })
    return
  }
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
    const el = editableRef.value
    if (el) el.textContent = selection.toString()
    syncInputText(); refreshHashDropdown()
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
    nextTick(() => editableRef.value?.focus())
  }
  if (!visible && isMaximized.value) {
    isMaximized.value = false
    sidebarWidth.value = preMaxWidth.value
    window.dispatchEvent(new CustomEvent('rdp:overlay-pop'))
  }
})

function onKeydown(e: KeyboardEvent) {
  // Hash dropdown navigation
  if (hashDropdownVisible.value) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onEscHashDropdown()
      return
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      hashHighlightIndex.value = Math.min(hashHighlightIndex.value + 1, hashMatchingPanels.value.length - 1)
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      hashHighlightIndex.value = Math.max(hashHighlightIndex.value - 1, 0)
      return
    }
    if (e.key === 'Enter') {
      e.preventDefault()
      if (hashMatchingPanels.value.length > 0) {
        onSelectHashPanel(hashMatchingPanels.value[hashHighlightIndex.value].title)
      }
      return
    }
  }

  // Cmd/Ctrl+V: paste via Wails clipboard (DOM paste unreliable in WKWebView)
  if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && (e.key === 'v' || e.key === 'V')) {
    e.preventDefault()
    ClipboardGetText().then(text => { if (text) insertTextAtCursor(text) }).catch(() => {})
    return
  }

  // Normal Enter to send
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    onSend()
  }
}

function onPaste(e: ClipboardEvent) {
  e.preventDefault()
  ClipboardGetText().then(text => { if (text) insertTextAtCursor(text) }).catch(() => {})
}

function insertTextAtCursor(text: string) {
  const el = editableRef.value
  if (!el) return
  el.focus()
  const sel = window.getSelection()
  if (sel && sel.rangeCount > 0) {
    const range = sel.getRangeAt(0)
    range.deleteContents()
    range.insertNode(document.createTextNode(text))
    range.collapse(false)
    sel.removeAllRanges()
    sel.addRange(range)
  } else {
    el.textContent += text
  }
  syncInputText()
  refreshHashDropdown()
}

function clearInput() {
  const el = editableRef.value
  if (el) el.innerHTML = ''
  syncInputText(); refreshHashDropdown()
}

async function onSend() {
  const text = getEditableText().trim()
  if (!text) return
  if (busy.value) {
    aiStore.enqueueMessage(text)
    clearInput()
    return
  }
  clearInput()
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
  if (aiStore.pendingQuestion) {
    dismissQuestion()
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

function onAnswer(selectedLabels: string[], customText?: string) {
  answerQuestion(selectedLabels, customText)
  scrollToBottom()
}

function onDismiss() {
  dismissQuestion()
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
    const el = editableRef.value
    if (el) el.textContent = text
    syncInputText(); refreshHashDropdown()
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
  background: var(--bg-elevated);
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
  width: 6px;
  cursor: col-resize;
  z-index: 10;
  background: transparent;
  transition: background 0.15s ease;
}

.resize-handle::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: 1px;
  background: linear-gradient(
    180deg,
    transparent 0%,
    var(--accent-subtle) 20%,
    var(--accent-glow) 50%,
    var(--accent-subtle) 80%,
    transparent 100%
  );
  transition: opacity 0.15s;
}

.resize-handle:hover::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: 3px;
  background: var(--accent);
  box-shadow: 0 0 6px var(--accent-glow);
}

.resize-handle:hover::before {
  opacity: 0;
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
  animation: status-pulse 1.2s ease-in-out infinite;
}

@keyframes status-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
.ai-input {
  padding: 10px 16px;
  flex-shrink: 0;
  position: relative;
}
.input-container {
  border: 1px solid var(--border-subtle);
  border-top-color: transparent;
  border-radius: 0 0 var(--radius-md) var(--radius-md);
  background: var(--bg-elevated);
  transition: border-color 0.15s ease;
  position: relative;
}
.input-container:focus-within {
  border-color: var(--accent);
  border-top-color: var(--accent);
}
.textarea-wrap {
  position: relative;
}
.ai-editable {
  padding: 12px 16px;
  font-size: 13px;
  font-family: var(--font-ui);
  color: var(--text-primary);
  background: transparent;
  border: none;
  outline: none;
  min-height: 60px;
  max-height: 220px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
}
.ai-editable:empty::before {
  content: attr(data-placeholder);
  color: var(--text-muted);
  pointer-events: none;
}
.hash-tag {
  display: inline;
  background: var(--accent);
  color: var(--on-accent);
  border-radius: 3px;
  padding: 1px 5px;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  user-select: none;
  margin: 0 2px;
}
.input-actions {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  align-items: center;
  padding: 0 8px 8px 8px;
}
.input-actions-left {
  display: flex;
  gap: 2px;
  align-items: center;
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
.add-model-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: none;
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
  background: var(--accent);
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
.ai-panel-tags {
  padding: 4px 12px;
  background: var(--bg-overlay);
  border-radius: var(--radius-md) var(--radius-md) 0 0;
}
.panel-tags-list {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  min-height: 22px;
}
.panel-tag {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 1px 6px;
  font-size: 11px;
  background: var(--accent-subtle);
  color: var(--accent);
  border: 1px solid var(--accent-glow);
  border-radius: var(--radius-sm);
  line-height: 1.5;
}
.panel-tag-default {
  background: var(--bg-overlay);
  color: var(--text-muted);
  border-color: var(--border-subtle);
}
.panel-tag-close {
  background: none;
  border: none;
  cursor: pointer;
  padding: 0;
  font-size: 13px;
  line-height: 1;
  color: var(--text-muted);
  transition: color 0.15s;
}
.panel-tag-close:hover {
  color: var(--text-primary);
}
.panel-tag-add-btn {
  background: none;
  border: 1px dashed var(--border-hover);
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  width: 20px;
  height: 20px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  line-height: 1;
  padding: 0;
  transition: border-color 0.15s, color 0.15s;
}
.panel-tag-add-btn:hover {
  border-color: var(--accent);
  color: var(--accent);
}
.no-terminal-hint {
  font-size: 11px;
  color: var(--text-muted);
}
.panel-shell-hint {
  margin-left: 8px;
  font-size: 10px;
  color: var(--text-muted);
}
.hash-dropdown {
  position: absolute;
  bottom: 100%;
  left: -1px;
  right: -1px;
  max-height: 180px;
  overflow-y: auto;
  background: var(--bg-surface);
  border: 1px solid var(--accent-glow);
  border-radius: var(--radius-sm);
  box-shadow: 0 -4px 12px rgba(0, 0, 0, 0.4);
  z-index: 100;
  margin-bottom: 4px;
}
.hash-dropdown-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  cursor: pointer;
  font-size: 12px;
  transition: background 0.1s;
}
.hash-dropdown-item:hover,
.hash-dropdown-item.highlighted {
  background: var(--accent-subtle);
}
.hash-panel-name {
  color: var(--accent);
  font-weight: 500;
}
.hash-panel-hint {
  font-size: 10px;
  color: var(--text-muted);
  margin-left: auto;
}
.hash-associated-badge {
  font-size: 9px;
  color: var(--accent);
  background: var(--accent-subtle);
  padding: 0 4px;
  border-radius: 2px;
  margin-left: 4px;
  flex-shrink: 0;
}
.hash-btn-icon {
  font-family: var(--font-mono);
  font-size: 14px;
  font-weight: 600;
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
