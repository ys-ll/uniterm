<template>
  <div
    ref="sidebarEl"
    class="ai-sidebar"
    :class="{ collapsed: !aiStore.visible, resizing: isResizing, maximized: isMaximized }"
    :style="{ width: sidebarWidth + 'px' }"
  >
    <div class="resize-handle" @mousedown="onResizeStart" />
    <div class="ai-header">
      <span>{{ t('ai.title') }}</span>
      <div class="ai-actions">
        <button class="ai-action-btn" @click="onNewSession" :title="t('ai.newSession')">
          <el-icon><MessageSquarePlus :size="'0.875rem'" /></el-icon>
        </button>
        <button
          v-if="aiStore.sessions.length > 0"
          class="ai-action-btn"
          :title="t('ai.recentSessions')"
          @click.stop="sessionMenuRef?.toggle($event.currentTarget as HTMLElement)"
        >
          <el-icon><History :size="'0.875rem'" /></el-icon>
        </button>
        <Menu ref="sessionMenuRef" v-model:visible="sessionMenuVisible">
          <MenuItem
            v-for="s in aiStore.sessions"
            :key="s.id"
            :class="{ active: s.id === aiStore.currentSessionId }"
            @click="onSessionSelect(s.id)"
          >
            <span class="session-item-name">{{ s.name }}</span>
            <span class="session-time">{{ formatRelativeTime(s.updatedAt) }}</span>
            <template #trailing>
              <el-icon class="session-delete" :title="t('ai.renameSession')" @click.stop="onSessionRename(s.id)"><Pencil :size="'0.875rem'" /></el-icon>
              <el-icon class="session-delete" :title="t('common.delete')" @click.stop="aiStore.deleteSession(s.id)"><Trash2 :size="'0.875rem'" /></el-icon>
            </template>
          </MenuItem>
        </Menu>
        <button class="ai-action-btn" @click="searchVisible = !searchVisible" :title="t('ai.search')">
          <el-icon><Search :size="'0.875rem'" /></el-icon>
        </button>
        <button class="ai-action-btn" @click="toggleMaximize" :title="isMaximized ? t('ai.restore') : t('ai.maximize')">
          <el-icon><Shrink v-if="isMaximized" :size="'0.875rem'" /><Expand v-else :size="'0.875rem'" /></el-icon>
        </button>
        <button class="ai-action-btn" @click="onClose" :title="t('sidebar.collapse')">
          <el-icon><X :size="'0.875rem'" /></el-icon>
        </button>
      </div>
    </div>

    <AIChatList
      ref="aiChatListRef"
      v-model:searchVisible="searchVisible"
      @ask-ai="onAskAIFromList"
    />
    <AIChatInput
      ref="aiChatInputRef"
      @open-settings="onOpenSettings"
      @sent="onSent"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { X, Trash2, Expand, Shrink, History, MessageSquarePlus, Search, Pencil } from '@lucide/vue'
import { ElMessageBox } from 'element-plus'
import { useAIStore } from '../stores/aiStore'
import { useI18n } from '../i18n'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import { useLocalStateStore } from '../stores/localStateStore'
import { useAIChat } from '../composables/useAIChat'
import AIChatList from './ai-sidebar/AIChatList.vue'
import AIChatInput from './ai-sidebar/AIChatInput.vue'

const emit = defineEmits<{
  'open-settings': []
}>()

const aiStore = useAIStore()
const { t } = useI18n()
const { formatRelativeTime } = useAIChat()

// ── Child component refs ──
const aiChatListRef = ref<InstanceType<typeof AIChatList> | null>(null)
const aiChatInputRef = ref<InstanceType<typeof AIChatInput> | null>(null)

// ── Search bar visibility (controlled by header button, consumed by AIChatList) ──
const searchVisible = ref(false)

// ── Sidebar width / resize / maximize ──
const sidebarEl = ref<HTMLDivElement>()
const AI_SIDEBAR_MIN_WIDTH = 300
const AI_SIDEBAR_MAX_WIDTH = 800
const sidebarWidth = ref(360)
const isResizing = ref(false)
const isMaximized = ref(false)
const preMaxWidth = ref(360)

// Restore the persisted width once LocalState is loaded (App.vue awaits
// init() on mount; children mount first, so re-init here — it is idempotent).
onMounted(async () => {
  try {
    const ls = useLocalStateStore()
    if (!ls.loaded) await ls.init()
    const w = ls.state.aiSidebarWidth
    if (w > 0) sidebarWidth.value = Math.min(Math.max(w, AI_SIDEBAR_MIN_WIDTH), AI_SIDEBAR_MAX_WIDTH)
  } catch {
    // keep the default width
  }
})

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
    const newWidth = Math.min(Math.max(startWidth + delta, AI_SIDEBAR_MIN_WIDTH), AI_SIDEBAR_MAX_WIDTH)
    if (el) el.style.width = newWidth + 'px'
  }

  function onMouseUp() {
    isResizing.value = false
    sidebarWidth.value = el?.offsetWidth ?? sidebarWidth.value
    // Skip while maximised — the CSS forces 100% width, so offsetWidth is
    // the full window width and must not overwrite the persisted value.
    if (!isMaximized.value) {
      useLocalStateStore().update({ aiSidebarWidth: sidebarWidth.value })
    }
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    window.dispatchEvent(new CustomEvent('split:resize-end'))
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

// ── Session menu ──
const sessionMenuVisible = ref(false)
const sessionMenuRef = ref<InstanceType<typeof Menu> | null>(null)

function closeSessionMenu() {
  sessionMenuVisible.value = false
}

function onNewSession() {
  aiStore.createSession()
}

function onSessionSelect(id: string) {
  closeSessionMenu()
  aiStore.switchSession(id)
}

// ── Session rename (pencil button in the history dropdown) ──
// The menu stays open across rename/delete so the actions can be repeated
// without reopening. The dialog floats above the menu, so pause the global
// outside-click/Escape dismissal while it is up.
async function onSessionRename(id: string) {
  const s = aiStore.sessions.find((x) => x.id === id)
  if (!s) return
  const resume = sessionMenuRef.value?.pauseDocClose()
  try {
    const name = await ElMessageBox.prompt('', t('ai.renameSession'), {
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
      inputValue: s.name,
    })
    const v = (name.value || '').trim()
    if (v) aiStore.renameSession(id, v)
  } catch {
    /* cancelled */
  } finally {
    resume?.()
  }
}

// ── Child event handlers ──
function onOpenSettings() {
  emit('open-settings')
}

/** AIChatInput emitted 'sent' → scroll the message list to the bottom. */
function onSent() {
  aiChatListRef.value?.scrollToBottom()
}

/** AIChatList emitted 'ask-ai' → put the selected text into the input box. */
function onAskAIFromList(text: string) {
  aiChatInputRef.value?.setText(text)
}

// ── External 'ai:ask' event (e.g. from terminal "Ask AI" action) ──
function onAskAI(e: Event) {
  const text = (e as CustomEvent).detail as string
  if (text) {
    aiChatInputRef.value?.setText(text)
    if (!aiStore.visible) {
      aiStore.visible = true
    }
  }
}

onMounted(() => {
  window.addEventListener('ai:ask', onAskAI)
})

onUnmounted(() => {
  window.removeEventListener('ai:ask', onAskAI)
})

// Restore sidebar width when the sidebar is hidden while maximised.
watch(
  () => aiStore.visible,
  (visible) => {
    if (!visible && isMaximized.value) {
      isMaximized.value = false
      sidebarWidth.value = preMaxWidth.value
      window.dispatchEvent(new CustomEvent('rdp:overlay-pop'))
    }
  },
)

function focusInput() {
  aiChatInputRef.value?.focus()
}

defineExpose({ focusInput })
</script>

<style scoped>
.ai-sidebar {
  background: var(--bg-elevated);
  border-left: 1px solid var(--border-subtle);
  display: flex;
  flex-direction: column;
  position: relative;
  flex-shrink: 0;
}
.ai-sidebar.collapsed {
  width: 0 !important;
  border-left: none;
  overflow: hidden;
}
.ai-sidebar.maximized {
  position: absolute !important;
  left: 0;
  top: 0;
  right: 0;
  bottom: 0;
  width: 100% !important;
  border-left: none;
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
  width: 0.375rem;
  cursor: col-resize;
  z-index: 10;
  background: transparent;
  transition: background 0.15s ease;
}

.resize-handle:hover::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: 0.1875rem;
  background: var(--accent);
  box-shadow: 0 0 0.375rem var(--accent-glow);
}

.resize-handle:hover::before {
  opacity: 0;
}
.ai-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  font-size: 0.75rem;
  font-family: var(--font-ui);
  font-weight: 600;
  color: var(--text-primary);
}
.ai-actions {
  display: flex;
  gap: 0.125rem;
}
.ai-action-btn {
  font-size: 0.875rem;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.625rem;
  height: 1.625rem;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  transition: all 0.12s ease;
}
.ai-action-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.session-item-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.session-time {
  margin-left: 0.5rem;
  font-size: 0.625rem;
  font-family: var(--font-mono);
  color: var(--text-muted);
  white-space: nowrap;
}
.session-delete {
  margin-left: 0.5rem;
  color: var(--text-muted);
}
.session-delete:hover {
  color: var(--text-primary);
}
</style>
