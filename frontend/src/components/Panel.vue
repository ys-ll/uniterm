<template>
  <div
    :data-panel-id="panel.id"
    class="panel"
    :class="{ 'panel-active': isActive }"
    draggable="true"
    @dragstart="emit('dragstart', $event)"
  >
    <div
      v-if="showHeader"
      class="panel-header"
      :class="{ 'ai-locked': isAILocked }"
      @dblclick.stop
      @contextmenu="onHeaderContextMenu"
    >
      <div class="panel-header-left">
        <span class="panel-icon-wrapper">
          <component :is="panelIcon" class="panel-type-icon" :class="{ 'ai-locked-icon': isAILocked }" />
          <span
            v-if="isOutputLogOn"
            class="panel-log-dot"
            :title="t('session.recording', { path: outputLogPath })"
          />
        </span>
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
          @contextmenu.stop
        />
        <span v-if="panelShortcut" class="panel-shortcut">{{ panelShortcut }}</span>
      </div>
      <div class="panel-header-actions">
        <button
          v-if="workspaceId"
          class="panel-maximize"
          @click.stop="toggleMaximize"
          :title="maximizeTitle"
        >
          <Minimize2 v-if="isMaximized" :size="'0.875rem'" />
          <Maximize2 v-else :size="'0.875rem'" />
        </button>
        <button
          v-if="(panel.type === 'ssh' || panel.type === 'local' || panel.type === 'wsl') && workspaceId"
          class="panel-broadcast"
          :class="{ active: panelBroadcastActive }"
          @click.stop="onBroadcastClick"
          :title="broadcastTitle"
        >
          <Radio :size="'0.875rem'" />
        </button>
        <div class="panel-more-wrapper">
          <button
            class="panel-more"
            @click.stop="toggleMoreMenu($event)"
            :title="t('terminal.more')"
          >
            <MoreHorizontal :size="'0.875rem'" />
          </button>
          <Menu ref="moreMenuRef" align="end" v-model:visible="moreMenuVisible">
            <!-- ① 面板操作 -->
            <MenuItem :shortcut="menuShortcut('duplicateSession')" @click="emit('duplicate', panel.id); moreMenuVisible = false">
              {{ t('tab.duplicate') }}
            </MenuItem>
            <MenuItem v-if="canDuplicateChannel" @click="onDuplicateChannel(); moreMenuVisible = false">
              {{ t('tab.duplicateChannel') }}
            </MenuItem>
            <MenuItem @click="forceReconnect(); moreMenuVisible = false">{{ t('tab.reconnect') }}</MenuItem>
            <!-- Tab-management items collapsed into a "标签" submenu, same
                 folding as the tab context menu; hidden when every child is. -->
            <MenuSubmenu v-if="hasPanelTabMenu" :label="t('tab.tabMenu')">
              <MenuItem v-if="serverHost" @click="copyHostAddress">{{ t('tab.copyHostAddress') }}</MenuItem>
              <MenuItem :shortcut="menuShortcut('lockAI')" @click="toggleAiLockFromMenu">
                {{ isAILocked ? t('terminal.aiLocked') : t('terminal.lockAI') }}
              </MenuItem>
              <MenuItem @click="renamePanel">{{ t('tab.rename') }}</MenuItem>
              <MenuItem v-if="panel.config?.id" @click="locateConnection">{{ t('tab.locate') }}</MenuItem>
              <MenuItem
                v-if="(panel.type === 'ssh' || panel.type === 'local' || panel.type === 'wsl') && workspaceId"
                @click="toggleBroadcastTarget"
              >
                {{ isPanelBroadcastTarget ? t('tab.unbroadcast') : t('tab.broadcast') }}
              </MenuItem>
            </MenuSubmenu>

            <!-- ② 会话文本操作 -->
            <MenuDivider />
            <MenuItem :shortcut="menuShortcut('terminalSearch')" @click="triggerSearch(); moreMenuVisible = false">
              {{ t('terminal.searchText') }}
            </MenuItem>
            <MenuSubmenu :label="t('terminal.display')">
              <MenuItem @click="togglePanelLineNumbers(); moreMenuVisible = false">
                {{ settingsStore.settings.terminal.showLineNumbers ? t('settings.hideLineNumbers') : t('settings.showLineNumbers') }}
              </MenuItem>
              <MenuItem @click="togglePanelTimestamps(); moreMenuVisible = false">
                {{ settingsStore.settings.terminal.showTimestamps ? t('settings.hideTimestamps') : t('settings.showTimestamps') }}
              </MenuItem>
            </MenuSubmenu>
            <MenuSubmenu :label="t('terminal.clearScreen')">
              <MenuItem @click="triggerResetOutput(); moreMenuVisible = false">{{ t('terminal.resetOutput') }}</MenuItem>
              <MenuItem @click="triggerClearScrollback(); moreMenuVisible = false">{{ t('terminal.clearScrollback') }}</MenuItem>
            </MenuSubmenu>
            <MenuSubmenu :label="t('session.logRecordMenu')">
              <MenuItem @click="toggleOutputLog(); moreMenuVisible = false">
                {{ isOutputLogOn ? t('session.stopLog') : t('session.startLog') }}
              </MenuItem>
              <MenuItem @click="openLogDir(); moreMenuVisible = false">
                {{ t('session.openLogDir') }}
              </MenuItem>
              <MenuDivider />
              <MenuItem @click="triggerExport(); moreMenuVisible = false">{{ t('terminal.export') }}</MenuItem>
              <MenuDivider />
              <MenuItem @click="togglePanelRecording(); moreMenuVisible = false">
                {{ isPanelRecording() ? t('record.stop') : t('record.start') }}
              </MenuItem>
              <MenuItem @click="openRecordListEvent(); moreMenuVisible = false">{{ t('record.openList') }}</MenuItem>
            </MenuSubmenu>

            <!-- ③ 连接功能（ssh） -->
            <MenuDivider />
            <MenuSubmenu v-if="panel.type === 'ssh' && connectFileMenuKey(panel.config)" :label="t('terminal.transfer')">
              <MenuItem @click="connectSftp(); moreMenuVisible = false">{{ t(connectFileMenuKey(panel.config)) }}</MenuItem>
              <MenuItem @click="uploadFileRz(); moreMenuVisible = false">{{ t('terminal.uploadFileRz') }}</MenuItem>
            </MenuSubmenu>
            <MenuItem v-if="panel.type === 'ssh'" @click="connectMonitor(); moreMenuVisible = false">{{ t('sidebar.connectMonitor') }}</MenuItem>

            <!-- ④ 关闭 -->
            <MenuDivider />
            <MenuItem :shortcut="menuShortcut('closePanel')" @click="emit('close', panel.id); moreMenuVisible = false">
              {{ t('tab.close') }}
            </MenuItem>
          </Menu>
        </div>
        <button class="panel-close" @click.stop="emit('close', panel.id)"><X :size="'0.875rem'" /></button>
      </div>
    </div>
    <BaseTerminal
      ref="baseTerminalRef"
      :mode="panel.type === 'local' || panel.type === 'wsl' ? 'local' : 'ssh'"
      :session-id="panel.sessionId"
      :on-session-status="onSessionStatus"
      :broadcast-active="panelBroadcastActive"
      :workspace-id="workspaceId"
      :panel-id="panel.id"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed, nextTick, onMounted, onUnmounted, inject } from 'vue'
import { Radio, MoreHorizontal, X, SquareTerminal, Laptop, LaptopMinimal, Cable, Terminal, Zap, Maximize2, Minimize2 } from '@lucide/vue'
import BaseTerminal from './BaseTerminal.vue'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import MenuDivider from './MenuDivider.vue'
import MenuSubmenu from './MenuSubmenu.vue'
import { getSessionRecorder, maybeWarnSensitive } from '../services/terminalRecorder'
import { getManagedTerminal } from '../services/terminalManager'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useSettingsStore } from '../stores/settingsStore'
import { formatKeyBinding, panelDigitShortcutsSuppressed, panelDigitShortcutPrefix, formatDigitShortcut } from '../composables/useKeyboardShortcuts'
import { useDuplicateSession } from '../composables/useDuplicateSession'
import { useOutputLog } from '../composables/useOutputLog'
import type { ShortcutAction } from '../types/settings'
import {
  CreateSession,
  CloseSession,
  K8sExecSession,
  ContainerExecSession,
  SessionStart,
} from '../../bindings/github.com/ys-ll/uniterm/app'
import { msg } from '../services/message'
import { useI18n } from '../i18n'
import type { Panel } from '../types/workspace'
import { waitForTerminalSize } from '../services/terminalManager'
import { connectFileMenuKey } from '../utils/fileTransferUtils'
import { isMobilePlatform } from '../utils/platform'
import type { ConnectionConfig } from '../types/session'
import type { CredentialResult } from './CredentialPrompt.vue'
import { applyRetryCredentials } from '../composables/applyRetryCredentials'
import { Clipboard } from '@wailsio/runtime'

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
  workspaceId?: string
  shortcutIndex?: number
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

const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent)
const panelShortcut = computed(() => {
  // Hidden on mobile (no hardware keyboard), by the "show shortcut hints"
  // toggle, same as the tab badges.
  if (isMobilePlatform()) return ''
  if (!settingsStore.settings.showTabShortcutHints) return ''
  if (!props.shortcutIndex || props.shortcutIndex > 9) return ''
  // Option+N on macOS / Alt+N elsewhere by default, or the user-configured
  // keyboard.panelSwitchModifier combo — hidden while the tab-switch modifier
  // claims the same combo or the user cleared the panel modifier, so a badge
  // never advertises a binding that does nothing.
  const kb = settingsStore.settings.keyboard
  if (panelDigitShortcutsSuppressed(kb.tabSwitchModifier, kb.panelSwitchModifier)) return ''
  const prefix = panelDigitShortcutPrefix(isMac, kb.panelSwitchModifier)
  return formatDigitShortcut(prefix, props.shortcutIndex, isMac)
})
const workspaceTab = computed(() =>
  props.workspaceId ? tabStore.tabs.find(tab => tab.id === props.workspaceId && tab.type === 'workspace') : undefined
)
const isMaximized = computed(() => workspaceTab.value?.maximizedPanelId === props.panel.id)
const maximizeTitle = computed(() => {
  const label = t(isMaximized.value ? 'workspace.restorePanel' : 'workspace.maximizePanel')
  if (isMobilePlatform()) return label
  // Reactive via settingsStore, so the hint follows the user's rebind.
  const b = settingsStore.settings.keyboard.maximizePanel
  const shortcut = b ? formatKeyBinding(b, isMac) : ''
  return shortcut ? `${label} (${shortcut})` : label
})

function toggleMaximize() {
  if (!props.workspaceId) return
  tabStore.setActivePanel(props.workspaceId, props.panel.id)
  tabStore.toggleWorkspacePanelMaximize(props.workspaceId)
  nextTick(() => baseTerminalRef.value?.focus())
}

// Human-readable keybinding for a shortcut action ('' when unset), shown as a
// hint in the panel "..." menu. Reactive via settingsStore, so the hint updates
// automatically when the user rebinds keys.
function menuShortcut(action: ShortcutAction): string {
  const b = settingsStore.settings.keyboard[action]
  if (!b) return ''
  return formatKeyBinding(b, isMac)
}

const showCredentialDialog = inject<(title: string, subtitle: string, fields: ('user' | 'password')[], initialUser?: string, initialPassword?: string) => Promise<CredentialResult | null>>('showCredentialDialog', () => Promise.resolve(null))

const isAILocked = computed(() =>
  tabStore.isPanelAILocked(props.panel.id)
)

const panelBroadcastActive = computed(() =>
  tabStore.isPanelBroadcasting(props.panel.id)
)

const broadcastTitle = computed(() =>
  `${t('terminal.broadcastInput')}\n${t('terminal.broadcastCtrlHint')}`
)

function onBroadcastClick(e: MouseEvent) {
  if (!props.workspaceId) return
  if (e.ctrlKey || e.metaKey) {
    tabStore.toggleBroadcastPanel(props.panel.id)
  } else {
    tabStore.toggleBroadcast(props.workspaceId)
  }
}

const baseTerminalRef = ref<InstanceType<typeof BaseTerminal> | null>(null)

const editing = ref(false)
const editName = ref('')
const editInputRef = ref<HTMLInputElement>()
const moreMenuVisible = ref(false)
const moreMenuRef = ref<InstanceType<typeof Menu> | null>(null)

// Session output log state + menu operations, shared with BaseTerminal /
// TabItem via the useOutputLog composable. Refreshed when the more menu
// opens and on mount; stays in sync with TabItem through
// panelStore.setOutputLog and the watch below.
const { isOutputLogOn, outputLogPath, refreshOutputLogState, toggleOutputLog, openLogDir } =
  useOutputLog({ getPanelId: () => props.panel.id })

const panelIcon = computed(() => {
  const t = props.panel.type
  if (t === 'local') return Laptop
  if (t === 'wsl') return LaptopMinimal
  if (t === 'serial') return Cable
  if (t === 'telnet') return Terminal
  if (t === 'mosh') return Zap
  return SquareTerminal
})

function toggleMoreMenu(e: MouseEvent) {
  const opening = !moreMenuVisible.value
  moreMenuRef.value?.toggle(e.currentTarget)
  if (opening) refreshOutputLogState()
}

// Right-click on the panel header opens the SAME menu as the "..." button
// (one shared Menu instance), so the two entry points can never drift apart.
function onHeaderContextMenu(e: MouseEvent) {
  e.preventDefault()
  e.stopPropagation()
  moreMenuRef.value?.openAt(e.clientX, e.clientY)
  refreshOutputLogState()
}

// Connection host (IP or hostname) — empty for local/wsl/serial panels.
const serverHost = computed(() => props.panel.config?.host || '')

/* The "标签" submenu is shown only when at least one of its children is
   visible (mirrors TabItem's hasTabMenu) */
const hasPanelTabMenu = computed(
  () =>
    !!serverHost.value ||
    !!props.panel.config?.id ||
    ((props.panel.type === 'ssh' || props.panel.type === 'local' || props.panel.type === 'wsl') && !!props.workspaceId)
)

const isPanelBroadcastTarget = computed(() =>
  tabStore.isPanelBroadcasting(props.panel.id)
)

function toggleAiLockFromMenu() {
  emit('toggleAiLock', props.panel.id)
  moreMenuVisible.value = false
}

// Equivalent to Ctrl+click on the header broadcast button: toggle THIS panel
// (not the whole workspace) as a broadcast target.
function toggleBroadcastTarget() {
  tabStore.toggleBroadcastPanel(props.panel.id)
  moreMenuVisible.value = false
}

async function copyHostAddress() {
  moreMenuVisible.value = false
  const host = serverHost.value
  if (!host) return
  // Wails clipboard, falling back to the browser API when the runtime is
  // absent (plain dev in a browser) or the call fails.
  let ok = false
  try { ok = await Clipboard.SetText(host) } catch { ok = false }
  if (!ok) {
    try { await navigator.clipboard.writeText(host) } catch { /* no clipboard */ }
  }
  msg.success(t('tab.hostCopied', { host }))
}

// Cast v2 recording controls (same session-keyed recorder the terminal uses).
// State is read at menu render time, so labels refresh on every menu open.
function isPanelRecording(): boolean {
  const sid = props.panel.sessionId
  return !!sid && getSessionRecorder(sid).isRecording()
}

async function togglePanelRecording() {
  const sid = props.panel.sessionId
  if (!sid) return
  const recorder = getSessionRecorder(sid)
  try {
    if (recorder.isRecording()) {
      const path = await recorder.stop()
      msg.copyable(t('record.stopped', { path }), 'info')
    } else {
      const managed = getManagedTerminal(sid)
      const path = await recorder.start(sid, managed?.terminal.cols || 80, managed?.terminal.rows || 24, props.panel.config?.name || sid)
      maybeWarnSensitive(t('record.sensitiveWarning'))
      msg.copyable(t('record.started', { path }), 'success')
    }
  } catch (e: any) {
    msg.error(t('record.startFailed', { error: String(e?.message ?? e) }))
  }
}

// One RecordList instance lives in BaseTerminal; reuse the event pattern the
// export/reset actions already use.
function openRecordListEvent() {
  window.dispatchEvent(new CustomEvent('terminal:record-list', { detail: { panelId: props.panel.id } }))
}

function togglePanelLineNumbers() {
  settingsStore.updateTerminal({ showLineNumbers: !settingsStore.settings.terminal.showLineNumbers })
}

function togglePanelTimestamps() {
  settingsStore.updateTerminal({ showTimestamps: !settingsStore.settings.terminal.showTimestamps })
}

function connectSftp() {
  // sourceTabId anchors the new SFTP tab beside the owning tab
  window.dispatchEvent(new CustomEvent('app:connect-sftp', { detail: { ...props.panel, sourceTabId: props.panel.tabId } }))
}

function uploadFileRz() {
  window.dispatchEvent(new CustomEvent('terminal:send-rz', { detail: { panelId: props.panel.id } }))
}

function connectMonitor() {
  // sourceTabId anchors the new monitor tab beside the owning tab
  window.dispatchEvent(new CustomEvent('app:connect-monitor', { detail: { ...props.panel, sourceTabId: props.panel.tabId } }))
}

function triggerSearch() {
  window.dispatchEvent(new CustomEvent('terminal:open-search', { detail: { panelId: props.panel.id } }))
}

function triggerExport() {
  window.dispatchEvent(new CustomEvent('terminal:export', { detail: { panelId: props.panel.id } }))
}

function triggerResetOutput() {
  window.dispatchEvent(new CustomEvent('terminal:reset-output', { detail: { panelId: props.panel.id } }))
}

function triggerClearScrollback() {
  window.dispatchEvent(new CustomEvent('terminal:clear-scrollback', { detail: { panelId: props.panel.id } }))
}

function startEdit() {
  editName.value = props.panel.title
  editing.value = true
  nextTick(() => {
    editInputRef.value?.focus()
    editInputRef.value?.select()
  })
}

function renamePanel() {
  moreMenuVisible.value = false
  startEdit()
}

function locateConnection() {
  moreMenuVisible.value = false
  if (props.panel.config?.id) {
    window.dispatchEvent(new CustomEvent('app:locate-connection', { detail: { id: props.panel.config.id } }))
  }
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

function onSessionStatus(status: string) {
  if (status === 'retry') {
    // Manual retry — user pressed Enter
    retryConnection()
  } else if (status === 'auth_failed') {
    // Issue #949: credentials were rejected — re-open the credential dialog
    // right away instead of making the user press Enter through two full
    // reconnect attempts before it appears.
    repromptCredentials()
  } else if (status === 'connected') {
    retryAttempt = 0
  }
  // Local sessions previously auto-reconnected here after a 200ms delay to
  // paper over ConPTY tearing down the pseudo-console (e.g. opencode /exit).
  // That raced the "Press Enter to restart" prompt and left users unable to
  // tell a dead shell from a live one, so a dead local shell now just waits
  // for Enter like every other session type.
}

// Password-auth SSH-family sessions: prompt for fresh credentials on an auth
// rejection. Key/identity authTypes are excluded — a new user/password pair
// cannot fix a rejected key, passphrase or identity-store reference.
async function repromptCredentials() {
  const config = props.panel.config
  if (!config) return
  const credTypes = ['ssh', 'mosh', 'sftp', 'scp', 'ftp', 'telnet']
  if (!credTypes.includes(props.panel.type)) return
  if (['key', 'keyText', 'kerberos', 'agent', 'identity'].includes(config.authType)) return
  const result = await showCredentialDialog(
    t('credential.title'),
    t('credential.authFailedSubtitle'),
    ['user', 'password'],
    config.user || '',
    ''
  )
  if (!result) return
  props.panel.config = await applyRetryCredentials(config, result)
  retryAttempt = 0
  await retryConnection()
}

async function retryConnection() {
  retryAttempt++
  if (props.panel.type === 'local') {
    baseTerminalRef.value?.write(RESET_MOUSE_MODES + '\r\n\x1b[33mRestarting local shell...\x1b[0m\r\n')
    try {
      const shellPath = props.panel.config?.shellPath || ''
      const config: ConnectionConfig = {
        ...props.panel.config,
        type: 'local',
        shellPath,
        initialCols: 0,
        initialRows: 0,
      }
      const info = await CreateSession('local', config)
      panelStore.bindSession(props.panel.id, info.id)
      sessionStore.initSession(info.id)
      const size = await waitForTerminalSize(info.id)
      if (size.cols > 0 && size.rows > 0) {
        config.initialCols = size.cols
        config.initialRows = size.rows
      }
      await SessionStart(info.id, config).catch((e) => {
        baseTerminalRef.value?.write(`\r\n\x1b[31mFailed to start local shell: ${e}\x1b[0m\r\n`)
        CloseSession(info.id).catch(() => {})
      })
      retryAttempt = 0
    } catch (e: any) {
      baseTerminalRef.value?.write(`\r\n\x1b[31mFailed to start local shell: ${e}\x1b[0m\r\n`)
      baseTerminalRef.value?.setRetryOnEnter(true)
    }
    return
  }
  if (!props.panel.config) return

  // Exec panels (k8s-exec / container-exec): rebuild the exec stream via the
  // dedicated wails method — CreateSession has no such type. The original
  // params live on the config from open time.
  if (props.panel.type === 'container-exec') {
    const c = props.panel.config
    const now = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    const at = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`
    baseTerminalRef.value?.write(RESET_MOUSE_MODES + `\r\n\x1b[33mReconnecting... (${at})\x1b[0m\r\n`)
    try {
      let info
      if (c.containerExecNamespace) {
        // k8s 连接：containerExecContainerId 为 Pod 名
        if (!c.containerExecConnId || !c.containerExecContainerId || !c.containerExecContainerName) throw new Error('exec session parameters missing')
        info = await K8sExecSession(c.containerExecConnId, c.containerExecNamespace, c.containerExecContainerId, c.containerExecContainerName)
      } else {
        if (!c.containerExecConnId || !c.containerExecContainerId) throw new Error('exec session parameters missing')
        info = await ContainerExecSession(c.containerExecConnId, c.containerExecContainerId, c.containerExecShell || 'sh')
      }
      panelStore.bindSession(props.panel.id, info.id)
      sessionStore.initSession(info.id)
      sessionStore.updateStatus(info.id, 'connected')
    } catch (e: any) {
      baseTerminalRef.value?.write(`\r\n\x1b[31mReconnect failed: ${e?.message || e}\x1b[0m\r\n`)
      baseTerminalRef.value?.setRetryOnEnter(true)
    }
    return
  }

  // On first retry, try with existing credentials; on subsequent retries, re-prompt
  const credTypes = ['ssh', 'mosh', 'sftp', 'scp', 'ftp', 'telnet']
  if (credTypes.includes(props.panel.type) && !['key', 'keyText', 'kerberos'].includes(props.panel.config.authType) && retryAttempt > 1) {
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
    props.panel.config = await applyRetryCredentials(props.panel.config, result)
  }

  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  const reconnectAt = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`
  baseTerminalRef.value?.write(RESET_MOUSE_MODES + `\r\n\x1b[33mReconnecting... (${reconnectAt})\x1b[0m\r\n`)
  try {
    const config: ConnectionConfig = {
      ...props.panel.config,
      initialCols: 0,
      initialRows: 0,
    }
    const info = await CreateSession(props.panel.config.type, config)
    panelStore.bindSession(props.panel.id, info.id)
    sessionStore.initSession(info.id)
    const size = await waitForTerminalSize(info.id)
    if (size.cols > 0 && size.rows > 0) {
      config.initialCols = size.cols
      config.initialRows = size.rows
    }
    await SessionStart(info.id, config).catch((e) => {
      baseTerminalRef.value?.write(`\r\n\x1b[31mReconnect failed: ${e}\x1b[0m\r\n`)
      CloseSession(info.id).catch(() => {})
    })
  } catch (e: any) {
    baseTerminalRef.value?.write(`\r\n\x1b[31mReconnect failed: ${e}\x1b[0m\r\n`)
    baseTerminalRef.value?.setRetryOnEnter(true)
  }
}

// Force-disconnect the current session and re-initiate the connection, invoked
// from the tab's right-click 「重连」(Reconnect) menu. Unlike the Enter-to-retry
// path, it first kills the in-flight attempt with CloseSession so a stale
// "connecting" session can't linger in the backend.
async function forceReconnect() {
  if (!props.panel.config) return
  baseTerminalRef.value?.setRetryOnEnter(false)
  const oldId = props.panel.sessionId
  if (oldId) {
    try { await CloseSession(oldId) } catch (_) {}
  }
  retryAttempt = 0
  await retryConnection()
}

// Channel clone (issue #983): only meaningful for a connected SSH panel.
const canDuplicateChannel = computed(() =>
  props.panel.type === 'ssh' && !!props.panel.sessionId &&
  sessionStore.getStatus(props.panel.sessionId) === 'connected'
)

function onDuplicateChannel() {
  const { duplicateChannel } = useDuplicateSession()
  duplicateChannel(
    { type: 'terminal', panelId: props.panel.id, title: props.panel.title },
    props.workspaceId ? { workspaceId: props.workspaceId, targetPanelId: props.panel.id } : undefined,
  )
}

// Reconnect menu in the tab right-click menu dispatches a 'panel:reconnect'
// CustomEvent carrying the target panel id; only the matching panel acts.
function onReconnectEvent(e: Event) {
  const panelId = (e as CustomEvent)?.detail?.panelId
  if (panelId && panelId === props.panel.id) {
    forceReconnect()
  }
}

onMounted(() => {
  window.addEventListener('panel:reconnect', onReconnectEvent)
  refreshOutputLogState()
})

onUnmounted(() => {
  window.removeEventListener('panel:reconnect', onReconnectEvent)
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

// Keep local log state in sync when TabItem (or anyone else) toggles the log
// via panelStore.setOutputLog — the header dot updates without reopening the menu.
watch(() => props.panel.outputLog, (val) => {
  if (val) {
    isOutputLogOn.value = !!val.enabled
    outputLogPath.value = val.path || ''
  } else {
    isOutputLogOn.value = false
    outputLogPath.value = ''
  }
}, { deep: true })
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
  padding: 0.25rem 0.5rem;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
  cursor: grab;
}
.panel-header:active {
  cursor: grabbing;
}
.panel-header-left {
  display: flex;
  align-items: center;
  min-width: 0;
}
.panel-active .panel-header {
  background: var(--bg-elevated);
}
/* The AI-locked panel header keeps its normal surface (issue #928: an
   alpha-tinted warning background turns muddy on dark themes); the state is
   carried by the title-bar type icon, which lights up solid amber. */
.panel-title {
  font-size: 0.75rem;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: text;
}
.panel-shortcut {
  flex-shrink: 0;
  margin-left: 0.375rem;
  color: var(--text-muted);
  font-size: 0.625rem;
  font-weight: 500;
  /* UI font so macOS modifier symbols (⌘⌥⇧) render with their native
     system-font shapes instead of a mono fallback. */
  font-family: var(--font-ui);
}
.panel-icon-wrapper {
  position: relative;
  display: inline-flex;
  flex-shrink: 0;
  margin-right: 0.375rem;
}
.panel-type-icon {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 0.875rem;
  height: 0.875rem;
  color: var(--text-muted);
}
.panel-active .panel-type-icon {
  color: var(--accent);
}
/* Locked wins over the active accent: the AI state must stay visible when the
   panel is focused (mirrors the tab icon treatment). */
.panel-type-icon.ai-locked-icon {
  color: var(--warning);
}
.panel-active .panel-type-icon.ai-locked-icon {
  color: var(--warning);
}
.panel-log-dot {
  position: absolute;
  right: -0.125rem;
  bottom: -0.125rem;
  width: 0.375rem;
  height: 0.375rem;
  background: #e5484d;
  border-radius: 50%;
  pointer-events: auto;
}
.panel-active .panel-title {
  color: var(--text-primary);
}
.panel-title-input {
  font-size: 0.75rem;
  font-family: inherit;
  color: var(--text-primary);
  background: var(--bg-base);
  border: 1px solid var(--accent);
  border-radius: var(--radius-sm);
  padding: 0.125rem 0.375rem;
  width: 7.5rem;
  outline: none;
}
.panel-header-actions {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  flex-shrink: 0;
}
.panel-broadcast,
.panel-maximize {
  background: none;
  border: none;
  color: var(--text-muted);
  font-size: 0.75rem;
  padding: 0.125rem 0.25rem;
  border-radius: var(--radius-sm);
  line-height: 1;
}
.panel-broadcast:hover,
.panel-maximize:hover {
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
.panel-duplicate {
  background: none;
  border: none;
  color: var(--text-muted);
  padding: 0.125rem 0.25rem;
  border-radius: var(--radius-sm);
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
  width: 1.375rem;
  height: 1.375rem;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  font-size: 0.875rem;
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
  padding: 0.125rem 0.25rem;
  border-radius: var(--radius-sm);
  display: inline-flex;
  align-items: center;
}
.panel-more:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}
</style>
