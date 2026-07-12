<template>
  <div
    :data-panel-id="panel.id"
    class="panel"
    :class="{ 'panel-active': isActive }"
    draggable="true"
    @dragstart="emit('dragstart', $event)"
  >
    <div v-if="showHeader" class="panel-header" :class="{ 'ai-locked': isAILocked }" @dblclick.stop>
      <span v-if="!editing" class="panel-title" @dblclick.stop="startEdit">{{ panel.title }}</span>
      <input
        v-else
        ref="editInputRef"
        v-model="editName"
        class="panel-title-input"
        @keydown.enter="confirmEdit"
        @keydown.escape="cancelEdit"
        @blur="confirmEdit"
        @click.stop
      />
      <div class="panel-header-actions">
        <button
          v-if="(panel.type === 'ssh' || panel.type === 'local') && workspaceId"
          class="panel-broadcast"
          :class="{ active: broadcastActive }"
          @click.stop="tabStore.toggleBroadcast(workspaceId)"
          :title="t('terminal.broadcastInput')"
        >
          <Radio :size="14" />
        </button>
        <button
          v-if="panel.type === 'ssh' || panel.type === 'local'"
          class="panel-ai-lock"
          :class="{ locked: isAILocked }"
          @click.stop="emit('toggleAiLock', panel.id)"
          :title="isAILocked ? t('terminal.aiLockedToPanel') : t('terminal.lockAIToPanel')"
        >
          <Sparkles :size="14" />
        </button>
        <div v-if="panel.type === 'ssh' || panel.type === 'local'" class="panel-more-wrapper">
          <button
            class="panel-more"
            @click.stop="toggleMoreMenu"
            :title="t('terminal.more')"
          >
            <MoreHorizontal :size="14" />
          </button>
          <div v-show="moreMenuVisible" class="panel-more-menu" @click.stop>
            <div class="menu-item" @click="emit('duplicate', panel.id); moreMenuVisible = false">{{ t('terminal.duplicate') }}</div>
            <div v-if="panel.type === 'ssh'" class="menu-item" @click="connectSftp(); moreMenuVisible = false">{{ t('sidebar.connectSftp') }}</div>
            <div v-if="panel.type === 'ssh'" class="menu-item" @click="uploadFileRz(); moreMenuVisible = false">{{ t('terminal.uploadFileRz') }}</div>
            <div v-if="panel.type === 'ssh'" class="menu-item" @click="connectMonitor(); moreMenuVisible = false">{{ t('sidebar.connectMonitor') }}</div>
            <div class="menu-item" @click="triggerSearch(); moreMenuVisible = false">{{ t('terminal.searchText') }}</div>
            <div class="menu-item" @click="triggerExport(); moreMenuVisible = false">{{ t('terminal.export') }}</div>
          </div>
        </div>
        <button class="panel-close" @click.stop="emit('close', panel.id)"><X :size="14" /></button>
      </div>
    </div>
    <BaseTerminal
      ref="baseTerminalRef"
      :mode="panel.type === 'local' ? 'local' : 'ssh'"
      :session-id="panel.sessionId"
      :on-session-status="onSessionStatus"
      :broadcast-active="broadcastActive"
      :workspace-id="workspaceId"
      :panel-id="panel.id"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed, nextTick, onMounted, onUnmounted, inject } from 'vue'
import { Radio, Sparkles, MoreHorizontal, X } from '@lucide/vue'
import BaseTerminal from './BaseTerminal.vue'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useSettingsStore } from '../stores/settingsStore'
import { CreateSession } from '../../wailsjs/go/main/App'
import { useI18n } from '../i18n'
import type { Panel } from '../types/workspace'
import type { ConnectionConfig } from '../types/session'
import type { CredentialResult } from './CredentialPrompt.vue'

// Escape sequences to disable all xterm mouse tracking modes.
// When a terminal app (e.g. opencode, vim, tmux) enables mouse tracking
// and then exits/crashes without disabling it, the xterm.js terminal
// is left in tracking mode — mouse events are captured as escape
// sequences and text selection stops working. Writing these reset
// sequences to the terminal before reconnecting restores normal
// selection behaviour without clearing the screen.
const RESET_MOUSE_MODES = '\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1004l\x1b[?1005l\x1b[?1006l\x1b[?1015l'

const { t } = useI18n()

const props = defineProps<{
  panel: Panel
  showHeader: boolean
  isActive: boolean
  broadcastActive?: boolean
  workspaceId?: string
}>()

const emit = defineEmits<{
  close: [panelId: string]
  dragstart: [e: DragEvent]
  toggleAiLock: [panelId: string]
  duplicate: [panelId: string]
  rename: [panelId: string, newName: string]
  connectSftp: [panelId: string]
  connectMonitor: [panelId: string]
}>()

const tabStore = useTabStore()
const panelStore = usePanelStore()
const sessionStore = useSessionStore()
const settingsStore = useSettingsStore()

const showCredentialDialog = inject<(title: string, subtitle: string, fields: ('user' | 'password')[], initialUser?: string, initialPassword?: string) => Promise<CredentialResult | null>>('showCredentialDialog', () => Promise.resolve(null))

const isAILocked = computed(() =>
  tabStore.aiLockedPanelId === props.panel.id
)

const baseTerminalRef = ref<InstanceType<typeof BaseTerminal> | null>(null)

const editing = ref(false)
const editName = ref('')
const editInputRef = ref<HTMLInputElement>()
const moreMenuVisible = ref(false)

function toggleMoreMenu() {
  moreMenuVisible.value = !moreMenuVisible.value
}

function connectSftp() {
  window.dispatchEvent(new CustomEvent('app:connect-sftp', { detail: props.panel }))
}

function uploadFileRz() {
  window.dispatchEvent(new CustomEvent('terminal:send-rz', { detail: { panelId: props.panel.id } }))
}

function connectMonitor() {
  window.dispatchEvent(new CustomEvent('app:connect-monitor', { detail: props.panel }))
}

function triggerSearch() {
  window.dispatchEvent(new CustomEvent('terminal:open-search', { detail: { panelId: props.panel.id } }))
}

function triggerExport() {
  window.dispatchEvent(new CustomEvent('terminal:export', { detail: { panelId: props.panel.id } }))
}

function onDocumentClick() {
  moreMenuVisible.value = false
}

function startEdit() {
  editName.value = props.panel.title
  editing.value = true
  nextTick(() => {
    editInputRef.value?.focus()
    editInputRef.value?.select()
  })
}

function confirmEdit() {
  if (!editing.value) return
  editing.value = false
  const newName = editName.value.trim()
  if (newName && newName !== props.panel.title) {
    emit('rename', props.panel.id, newName)
  }
}

function cancelEdit() {
  editing.value = false
}

let retryAttempt = 0
let autoRetried = false

function onSessionStatus(status: string) {
  if (status === 'retry') {
    // Manual retry — user pressed Enter
    autoRetried = false
    retryConnection(false)
  } else if (status === 'connected') {
    retryAttempt = 0
    autoRetried = false
  } else if (status === 'disconnected' && props.panel.type === 'local') {
    // Auto-reconnect local sessions silently on disconnect to handle
    // ConPTY edge cases where a child process (e.g. opencode /exit)
    // tears down the pseudo-console. Only auto-retry once per disconnect
    // cycle (manual Enter resets the guard).
    if (!autoRetried) {
      autoRetried = true
      setTimeout(() => retryConnection(true), 200)
    }
  }
}

async function retryConnection(silent = false) {
  retryAttempt++
  if (props.panel.type === 'local') {
    // Auto-retry (silent): just reset mouse modes and add a newline so the
    // new prompt is separated from the previous session's output.
    // Manual retry: show the yellow "Restarting..." message.
    if (silent) {
      baseTerminalRef.value?.write(RESET_MOUSE_MODES + '\r\n')
    } else {
      baseTerminalRef.value?.write(RESET_MOUSE_MODES + '\r\n\x1b[33mRestarting local shell...\x1b[0m\r\n')
    }
    try {
      const shellPath = props.panel.config?.shellPath || ''
      const config = { ...props.panel.config, type: 'local', shellPath } as ConnectionConfig
      const info = await CreateSession('local', config)
      panelStore.bindSession(props.panel.id, info.id)
      sessionStore.initSession(info.id)
      retryAttempt = 0
    } catch (e: any) {
      baseTerminalRef.value?.write(`\r\n\x1b[31mFailed to start local shell: ${e}\x1b[0m\r\n`)
      baseTerminalRef.value?.setRetryOnEnter(true)
    }
    return
  }
  if (!props.panel.config) return

  // On first retry, try with existing credentials; on subsequent retries, re-prompt
  const credTypes = ['ssh', 'mosh', 'sftp', 'ftp', 'telnet']
  if (credTypes.includes(props.panel.type) && props.panel.config.authType !== 'key' && retryAttempt > 1) {
    const result = await showCredentialDialog(
      t('credential.title'),
      props.panel.config.user || props.panel.config.host ? `${props.panel.config.user}@${props.panel.config.host}` : '',
      ['user', 'password'],
      props.panel.config.user,
      props.panel.config.password || ''
    )
    if (!result) {
      baseTerminalRef.value?.write('\r\n\x1b[33mRetry cancelled.\x1b[0m\r\n')
      baseTerminalRef.value?.setRetryOnEnter(true)
      return
    }
    props.panel.config.user = result.user || props.panel.config.user
    props.panel.config.password = result.password
  }

  baseTerminalRef.value?.write(RESET_MOUSE_MODES + '\r\n\x1b[33mReconnecting...\x1b[0m\r\n')
  try {
    const info = await CreateSession(props.panel.config.type, props.panel.config)
    panelStore.bindSession(props.panel.id, info.id)
    sessionStore.initSession(info.id)
  } catch (e: any) {
    baseTerminalRef.value?.write(`\r\n\x1b[31mReconnect failed: ${e}\x1b[0m\r\n`)
    baseTerminalRef.value?.setRetryOnEnter(true)
  }
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick)
})

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
})

// Watch panel sessionId changes and retry resize
watch(() => props.panel.sessionId, (newId) => {
  if (newId) {
    const delays = [200, 400, 600, 800, 1000, 1500, 2000]
    delays.forEach((delay) => {
      setTimeout(() => baseTerminalRef.value?.resize(), delay)
    })
  }
})

watch(() => props.isActive, (active) => {
  if (active) {
    nextTick(() => baseTerminalRef.value?.focus())
  }
})
</script>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
  background: var(--bg-base);
}
.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 8px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
  cursor: grab;
}
.panel-header:active {
  cursor: grabbing;
}
.panel-active .panel-header {
  background: var(--bg-elevated);
  border-bottom-color: var(--accent-dim);
}
.panel-header.ai-locked {
  border-left: 3px solid var(--warning);
  box-shadow: inset 0 0 12px var(--warning-subtle);
}
.panel-title {
  font-size: 12px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: text;
}
.panel-active .panel-title {
  color: var(--text-primary);
}
.panel-title-input {
  font-size: 12px;
  font-family: inherit;
  color: var(--text-primary);
  background: var(--bg-base);
  border: 1px solid var(--accent-dim);
  border-radius: var(--radius-sm);
  padding: 2px 6px;
  width: 120px;
  outline: none;
}
.panel-header-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}
.panel-broadcast {
  background: none;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  font-size: 12px;
  padding: 2px 4px;
  border-radius: 3px;
  line-height: 1;
}
.panel-broadcast:hover {
  background: var(--bg-hover);
}
.panel-broadcast.active {
  color: var(--accent);
  background: var(--accent-subtle);
}
.broadcast-icon {
  display: inline-block;
  line-height: 1;
}
.panel-ai-lock {
  background: none;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 3px;
  display: inline-flex;
  align-items: center;
}
.ai-lock-icon {
  display: block;
}
.panel-ai-lock:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}
.panel-ai-lock.locked {
  color: var(--warning);
}
.panel-duplicate {
  background: none;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 3px;
  display: inline-flex;
  align-items: center;
}
.panel-duplicate:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}
.panel-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  font-size: 14px;
  transition: all 0.12s ease;
}
.panel-close:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.panel-more-wrapper {
  position: relative;
}
.panel-more {
  background: none;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 3px;
  display: inline-flex;
  align-items: center;
}
.panel-more:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}
.panel-more-menu {
  position: absolute;
  top: 100%;
  right: 0;
  z-index: 100;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
  min-width: 120px;
  padding: 4px;
}
.panel-more-menu .menu-item {
  padding: 7px 14px;
  font-size: 12px;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  white-space: nowrap;
}
.panel-more-menu .menu-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
</style>
