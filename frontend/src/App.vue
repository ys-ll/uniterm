<template>
  <el-config-provider :locale="elLocale">
  <div class="app-container" :class="{ 'has-bg': bgVisible }">
    <div v-if="bgVisible" class="app-bg" :style="bgStyle"></div>
    <AppHeader
      @toggle-ai="aiStore.toggle"
      @toggle-sidebar="sidebarVisible = !sidebarVisible"
      @open-settings="openSettings"
      @close-tab="closeTab"
      @close-tab-batch="closeTabBatch"
      @toggle-ai-lock="onToggleAiLock"
      @tab-dragstart="onTabDragStart"
      @new-local-terminal="createNewTab"
    />
    <div class="main-content">
      <Sidebar ref="sidebarRef" :visible="sidebarVisible" @toggle="sidebarVisible = !sidebarVisible" @connect="onSidebarConnect" @connect-to-workspace="({ config, workspaceId }: any) => onConnect(config, undefined, undefined, true, workspaceId)" @create-workspace="(configs: any) => onCreateWorkspaceFromConfigs(configs)" @connect-only="onConnectOnly" />
      <div class="tab-area">
        <template v-if="activeTab">
          <KeepAlive>
            <TerminalTabContent
              v-if="activeTab.type === 'terminal'"
              :key="activeTab.id"
              :tab="activeTab"
              @close="closeTab"
            />
            <SettingsTabContent
              v-else-if="activeTab.type === 'settings'"
            />
            <WorkspaceContent
              v-else-if="activeTab.type === 'workspace'"
              :tab="activeTab"
            />
            <FileTabContent
              v-else-if="activeTab.type === 'sftp'"
              :key="activeTab.id"
              :panel-id="activeTab.panelId"
            />
            <RDPTabContent
              v-else-if="activeTab.type === 'rdp'"
              :key="activeTab.id"
              :panel-id="activeTab.panelId"
              :config="getPanelConfig(activeTab.panelId)"
              :session-id="getPanelSessionId(activeTab.panelId)"
            />
            <VNCTabContent
              v-else-if="activeTab.type === 'vnc'"
              :key="activeTab.id"
              :panel-id="activeTab.panelId"
              :config="getPanelConfig(activeTab.panelId)"
              :session-id="getPanelSessionId(activeTab.panelId)"
            />
            <SPICETabContent
              v-else-if="activeTab.type === 'spice'"
              :key="activeTab.id"
              :panel-id="activeTab.panelId"
              :config="getPanelConfig(activeTab.panelId)"
              :session-id="getPanelSessionId(activeTab.panelId)"
            />
            <X11DesktopTabContent
              v-else-if="activeTab.type === 'x11-desktop'"
              :key="activeTab.id"
              :panel-id="activeTab.panelId"
              :config="getPanelConfig(activeTab.panelId)"
              :session-id="getPanelSessionId(activeTab.panelId)"
            />
            <DBTabContent
              v-else-if="activeTab.type === 'database'"
              :key="activeTab.id"
              :session-id="getPanelSessionId(activeTab.panelId) || ''"
              :host-name="getPanelConfig(activeTab.panelId)?.host || ''"
              :default-db-name="getPanelConfig(activeTab.panelId)?.dbName"
              :db-type="getPanelConfig(activeTab.panelId)?.dbType || ''"
            />
            <RedisTabContent
              v-else-if="activeTab.type === 'redis'"
              :key="activeTab.id"
              :session-id="getPanelSessionId(activeTab.panelId) || ''"
              :key-separator="getPanelConfig(activeTab.panelId)?.redisKeySeparator ?? ''"
            />
            <MongoDBTabContent
              v-else-if="activeTab.type === 'mongodb'"
              :key="activeTab.id"
              :session-id="getPanelSessionId(activeTab.panelId) || ''"
            />
            <ElasticsearchTabContent
              v-else-if="activeTab.type === 'elasticsearch'"
              :key="activeTab.id"
              :session-id="getPanelSessionId(activeTab.panelId) || ''"
            />
            <MonitorTabContent
              v-else-if="activeTab.type === 'monitor'"
              :key="activeTab.id"
              :session-id="getPanelSessionId(activeTab.panelId) || ''"
            />
            <K8sTabContent
              v-else-if="activeTab.type === 'k8s'"
              :key="activeTab.id"
              :tab="activeTab"
              :connection="k8sConnectionForTab(activeTab)"
            />
            <ContainerTabContent
              v-else-if="activeTab.type === 'container'"
              :key="activeTab.id"
              :tab="activeTab"
            />
            <StartTabContent
              v-else-if="activeTab.type === 'start'"
              :key="activeTab.id"
              :tab="activeTab"
              @connect="onConnect"
              @connect-to-workspace="({ configs, workspaceId }: any) => { for (const c of configs) onConnect(c, undefined, undefined, true, workspaceId) }"
              @create-workspace="(configs: any) => onCreateWorkspaceFromConfigs(configs)"
              @new-workspace="onNewWorkspace"
              @new-connection="onNewConnectionFromStart"
              @local-terminal="createLocalTerminalWithShell"
              @close-self="(tabId: string) => closeTab(tabId)"
              @edit-connection="onEditConnection"
              @change-group="onChangeGroupFromStart"
              @change-group-ids="onChangeGroupFromStartIds"
              @change-group-parent="onChangeGroupParentFromStart"
            />
          </KeepAlive>
        </template>
      </div>
      <AISidebar ref="aiSidebarRef" @open-settings="openSettings" />
    </div>
    <ConnectionForm v-model="showConnectionForm" :edit-config="editConfig" :default-group-id="pendingGroupId" @save="onSaveOnly" @connect="(c: ConnectionConfig, ko?: boolean) => { const wasEdit = !!editConfig?.id; editConfig = null; onConnect(c, ko, wasEdit) }" @connect-only="onConnectOnly" @cancel="editConfig = null" />

    <CredentialPrompt
      v-model:visible="credentialVisible"
      :title="credentialTitle"
      :subtitle="credentialSubtitle"
      :fields="credentialFields"
      :initial-user="credentialInitialUser"
      :initial-password="credentialInitialPassword"
      @resolve="onCredentialResolve"
    />

    <Menu ref="inputMenuRef" v-model:visible="inputMenuVisible">
      <MenuItem v-if="!inputMenuReadonly" @click="inputMenuCut">{{ t('input.cut') }}</MenuItem>
      <MenuItem @click="inputMenuCopy">{{ t('input.copy') }}</MenuItem>
      <MenuItem v-if="!inputMenuReadonly" @click="inputMenuPaste">{{ t('input.paste') }}</MenuItem>
      <MenuItem @click="inputMenuSelectAll">{{ t('input.selectAll') }}</MenuItem>
    </Menu>

    <MCPApprovalDialog
      v-model:visible="mcpApprovalVisible"
      :request="mcpApprovalRequest"
      @resolve="onMcpApprovalResolve"
    />

    <SyncConflictDialog />
    <UpdateDialog />
    <WorkspaceSaveDialog />
    <DataDirDialog v-model:visible="dataDirVisible" :first-run="credStore.firstRun || credStore.dataDirInfo.firstRun" @done="onDataDirDone" />
    <EncryptionModeDialog v-model:visible="encryptVisible" :existing-secrets="credStore.status.existingSecrets" @done="onEncryptDone" />
    <CredentialUnlockDialog v-model:visible="unlockVisible" @done="onUnlockDone" @reset="onReset" />
    <MobileKeyBar v-if="keyBarSessionId" :key="keyBarSessionId" :session-id="keyBarSessionId" />
  </div>
  </el-config-provider>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted, provide, h } from 'vue'
import { remoteDesktopOwnsKey } from './utils/remoteKeyboard'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import zhTw from 'element-plus/es/locale/lang/zh-tw'
import enUs from 'element-plus/es/locale/lang/en'
import ja from 'element-plus/es/locale/lang/ja'
import ko from 'element-plus/es/locale/lang/ko'
import de from 'element-plus/es/locale/lang/de'
import es from 'element-plus/es/locale/lang/es'
import fr from 'element-plus/es/locale/lang/fr'
import ru from 'element-plus/es/locale/lang/ru'
import AppHeader from './components/AppHeader.vue'
import Sidebar from './components/Sidebar.vue'
import TerminalTabContent from './components/TerminalTabContent.vue'
import MobileKeyBar from './components/MobileKeyBar.vue'
import { isMobilePlatform } from './utils/platform'
import { startAndroidKeepAlive, stopAndroidKeepAlive } from './utils/androidKeepAlive'
import { writeClipboard } from './composables/useClipboardWrite'
import SettingsTabContent from './components/SettingsTabContent.vue'
import WorkspaceContent from './components/WorkspaceContent.vue'
import FileTabContent from './components/FileTabContent.vue'
import RDPTabContent from './components/RDPTabContent.vue'
import VNCTabContent from './components/VNCTabContent.vue'
import SPICETabContent from './components/SPICETabContent.vue'
import X11DesktopTabContent from './components/X11DesktopTabContent.vue'
import DBTabContent from './components/DBTabContent.vue'
import RedisTabContent from './components/RedisTabContent.vue'
import MongoDBTabContent from './components/MongoDBTabContent.vue'
import ElasticsearchTabContent from './components/ElasticsearchTabContent.vue'
import MonitorTabContent from './components/MonitorTabContent.vue'
import K8sTabContent from './components/K8sTabContent.vue'
import ContainerTabContent from './components/ContainerTabContent.vue'
import StartTabContent from './components/StartTabContent.vue'
import ConnectionForm from './components/ConnectionForm.vue'
import AISidebar from './components/AISidebar.vue'
import SyncConflictDialog from './components/SyncConflictDialog.vue'
import WorkspaceSaveDialog from './components/WorkspaceSaveDialog.vue'
import UpdateDialog from './components/UpdateDialog.vue'
import DataDirDialog from './components/DataDirDialog.vue'
import EncryptionModeDialog from './components/EncryptionModeDialog.vue'
import CredentialUnlockDialog from './components/CredentialUnlockDialog.vue'
import CredentialPrompt from './components/CredentialPrompt.vue'
import type { CredentialResult } from './components/CredentialPrompt.vue'
import MCPApprovalDialog from './components/MCPApprovalDialog.vue'
import type { MCPApprovalRequest } from './types/mcp'
import Menu from './components/Menu.vue'
import MenuItem from './components/MenuItem.vue'
import { ElMessageBox, ElCheckbox } from 'element-plus'
import { useConnectionStore } from './stores/connectionStore'
import { useTabStore } from './stores/tabStore'
import { usePanelStore } from './stores/panelStore'
import { useSessionStore } from './stores/sessionStore'
import { useAIStore } from './stores/aiStore'
import { useCompanionStore } from './stores/companionStore'
import { useSettingsStore } from './stores/settingsStore'
import { useQuickCommandStore } from './stores/quickCommandStore'
import { useSkillStore } from './stores/skillStore'
import { useCommandStore } from './stores/commandStore'
import { useTunnelStore } from './stores/tunnelStore'
import { useLocalStateStore } from './stores/localStateStore'
import { useContainerStore } from './stores/containerStore'
import { useSyncStore } from './stores/syncStore'
import { useCredentialStore } from './stores/credentialStore'
import { disposeSessionStore } from './stores/sessionStore'
import { useUpdateCheck } from './composables/useUpdateCheck'
import { loadKeybindings, installGlobalListener, uninstallGlobalListener, matchDigitShortcut, isRebinding } from './composables/useKeyboardShortcuts'
import { focusPanelTerminal, installTerminalFocusRestore } from './composables/useFocusTerminal'
import { useDuplicateSession } from './composables/useDuplicateSession'
import { useTunnelCredentials } from './composables/useTunnelCredentials'
import type { ShortcutAction } from './types/settings'
import { useI18n } from './i18n'
import { CreateSession, CloseSession, RecordRecentConnection, GetPlatform, GetBackgroundImage, SessionStart, RelaunchApp, ResolveMCPApproval } from '../bindings/github.com/ys-ll/uniterm/app'
import { waitForTerminalSize } from './services/terminalManager'
import { msg } from './services/message'
import { unregisterTransferRoute } from './services/transferTaskCenter'
import type { ConnectionConfig } from './types/session'
import type { PanelType } from './types/workspace'
import { Application, Clipboard, Events } from '@wailsio/runtime'
import { parseQuickConnect } from './utils/quickConnect'
import { getShellLabel as getShellLabelBase, parseWslFromShell } from './utils/shellLabel'
import { reconnectFileTransferPanel } from './composables/usePanelReconnect'
import { launchConnection, launchFileBrowser, launchMonitor, launchWslFileBrowser, launchContainerFileBrowser, persistConnection, configureLauncher } from './composables/connectionLauncher'
import { openSavedWorkspace } from './composables/savedWorkspace'
import { installTabSnapshotSaver, restoreTabsSnapshot } from './composables/tabSnapshot'
import { createWorkspaceFromSelection } from './composables/createWorkspace'

const bgDataUrl = ref('')

async function loadBackgroundImage() {
  const ls = localStateStore.state
  if (ls.backgroundEnabled && ls.backgroundImage) {
    try {
      bgDataUrl.value = await GetBackgroundImage(ls.backgroundImage)
    } catch {
      bgDataUrl.value = ''
    }
  } else {
    bgDataUrl.value = ''
  }
}

const bgVisible = computed(
  () => localStateStore.state.backgroundEnabled && !!bgDataUrl.value
)

const bgStyle = computed(() => {
  const ls = localStateStore.state
  const fit = ls.backgroundFit || 'cover'
  const style: Record<string, string> = {
    backgroundImage: `url("${bgDataUrl.value}")`,
    filter: ls.backgroundBlur ? `blur(${ls.backgroundBlur}px)` : 'none',
  }
  if (fit === 'cover') {
    style.backgroundSize = 'cover'; style.backgroundPosition = 'center'; style.backgroundRepeat = 'no-repeat'
  } else if (fit === 'contain') {
    style.backgroundSize = 'contain'; style.backgroundPosition = 'center'; style.backgroundRepeat = 'no-repeat'
  } else if (fit === 'center') {
    style.backgroundSize = 'auto'; style.backgroundPosition = 'center'; style.backgroundRepeat = 'no-repeat'
  } else {
    style.backgroundSize = 'auto'; style.backgroundRepeat = 'repeat'
  }
  style['--bg-mask-opacity'] = String((ls.backgroundOpacity ?? 60) / 100)
  return style
})

const connectionStore = useConnectionStore()
const tabStore = useTabStore()
const activeTab = computed(() => tabStore.activeTab)
const panelStore = usePanelStore()
const sessionStore = useSessionStore()

// ── Mobile key bar (app-level) ──
// Soft keyboards lack Esc/Tab/arrows/Ctrl combos. The bar floats at the bottom
// of .app-container — directly above the soft keyboard, because MainActivity
// pads the app root by the IME inset. It shows only while the keyboard is up
// from a terminal textarea focus and its session is connected.
const isMobile = isMobilePlatform()
const focusedTerminalSessionId = ref<string | null>(null)

function updateFocusedTerminal(el: HTMLElement | null) {
  focusedTerminalSessionId.value = el?.classList?.contains('xterm-helper-textarea')
    ? el.closest<HTMLElement>('[data-terminal-session-id]')?.dataset.terminalSessionId || null
    : null
}

function onDocFocusIn(e: FocusEvent) {
  updateFocusedTerminal(e.target as HTMLElement | null)
}
function onDocFocusOut() {
  // Focus may move between xterm instances or to nothing; re-check after the
  // focusin/focusout pair settles.
  requestAnimationFrame(() => {
    updateFocusedTerminal(document.activeElement as HTMLElement | null)
  })
}
if (isMobile) {
  document.addEventListener('focusin', onDocFocusIn)
  document.addEventListener('focusout', onDocFocusOut)
}

// ── Background keep-alive (Android) ──
// Without a foreground service, Android freezes the cached process shortly
// after the app is switched away — timers stop and network is suspended,
// which drops live SSH/SFTP sessions. While the app is hidden with live
// sessions, run the keep-alive foreground service (notification appears);
// stop it when the app is visible again.
function hasLiveSessions(): boolean {
  for (const s of sessionStore.sessions.values()) {
    if (s.status === 'connected' || s.status === 'connecting') return true
  }
  return false
}

function onVisibilityChange() {
  if (document.visibilityState === 'hidden') {
    if (hasLiveSessions()) {
      startAndroidKeepAlive('uniTerm', t('app.backgroundKeepAlive'))
    }
  } else {
    stopAndroidKeepAlive()
  }
}
onUnmounted(() => {
  if (isMobile) {
    document.removeEventListener('focusin', onDocFocusIn)
    document.removeEventListener('focusout', onDocFocusOut)
    document.removeEventListener('visibilitychange', onVisibilityChange)
  }
  stopAndroidKeepAlive()
})

const keyBarSessionId = computed(() => {
  const sessionId = focusedTerminalSessionId.value
  if (!isMobile || !sessionId) return null
  if (sessionStore.getStatus(sessionId) !== 'connected') return null
  return sessionId
})
const { duplicateSession } = useDuplicateSession()
const aiStore = useAIStore()
const companionStore = useCompanionStore()
const settingsStore = useSettingsStore()
const localStateStore = useLocalStateStore()
const containerStore = useContainerStore()
const syncStore = useSyncStore()
const tunnelStore = useTunnelStore()
const updateCheck = useUpdateCheck()
let uninstallFocusRestore: (() => void) | null = null
// Unsubscribers for module-level Wails EventsOn listeners (FE-03).
// Tray menu → open the settings tab (Go shows the window first).
let unsubTrayOpenSettings: (() => void) | null = null
let unsubTrayOpenAbout: (() => void) | null = null
const { t, locale } = useI18n()
const EL_LOCALE_MAP: Record<string, typeof enUs> = {
  'zh-CN': zhCn, 'zh-TW': zhTw, en: enUs, ja, ko, de, es, fr, ru,
}
const elLocale = computed(() => EL_LOCALE_MAP[locale.value] || enUs)

// ── Credential / first-run flow ─────────────────────────────────
const credStore = useCredentialStore()
const dataDirVisible = ref(false)
const encryptVisible = ref(false)
const unlockVisible = ref(false)

// Determine which credential dialog (recovery-unlock / setup) to show based
// on the current store status. Shared by startup and by the first-run
// data-dir selection path so that pointing at a directory that already holds
// master-password config correctly prompts for the password.
//
// Mode selection is a first-run (and post-reset) decision only: any state
// with existing local data — a lost keychain key on a copied dir, a failed
// decrypt, missing meta over surviving ciphertext — lands in the unlock
// dialog, whose reset button is the only path that may clear credentials
// before a new mode is chosen. resolveCredentialDialog must never route
// straight to EncryptionModeDialog over existing secrets.
function resolveCredentialDialog() {
  const st = credStore.status
  if (st.needsSetup && !st.existingSecrets) { encryptVisible.value = true; return }
  if (!st.unlocked) { unlockVisible.value = true }
}

function onDataDirDone(restart: boolean) {
  if (restart) {
    ElMessageBox.confirm(t('dataDir.restartMsg'), t('dataDir.restartTitle'),
      { confirmButtonText: t('settings.restartNow'), cancelButtonText: t('conn.cancel'), type: 'warning' })
      .then(() => RelaunchApp()).catch(() => {})
    return
  }
  // Backend just initialized the stores for the newly selected data dir. Reload
  // so the frontend reflects existing connections/settings/quick-commands/tunnels.
  // When credentials are still locked the loads come back empty and onUnlockDone
  // re-loads after unlock.
  connectionStore.load()
  settingsStore.reload()
  useQuickCommandStore().load()
  useSkillStore().reload()
  useCommandStore().reload()
  tunnelStore.load()
  resolveCredentialDialog()
}
function onEncryptDone() { connectionStore.load() }
function onUnlockDone() {
  connectionStore.load()
  settingsStore.reload()
}
async function onReset() {
  try {
    await ElMessageBox.confirm(t('unlock.resetConfirm'), t('unlock.reset'), { type: 'warning' })
    await credStore.reset()
    encryptVisible.value = true
    unlockVisible.value = false
  } catch { /* cancelled */ }
}

async function checkCredentials() {
  credStore.watchEvents()
  await credStore.loadDataDir()
  await credStore.loadStatus()
  // Backend unreachable (wails3 dev browser preview): every binding call
  // rejects and the placeholder status would wrongly pop the encryption
  // dialog. Skip the whole credential flow so the UI renders for debugging.
  if (!credStore.backendAvailable) {
    console.warn('[uniTerm] Backend unreachable — browser debug mode: credential flow skipped')
    return
  }
  if (credStore.firstRun || credStore.dataDirInfo.firstRun) {
    dataDirVisible.value = true
    return
  }
  resolveCredentialDialog()
}
const showConnectionForm = ref(false)
const sidebarVisible = ref(false)
const sidebarRef = ref<any>(null)
const aiSidebarRef = ref<any>(null)


// Input context menu state
const inputMenuVisible = ref(false)
const inputMenuReadonly = ref(false)
const inputMenuRef = ref<InstanceType<typeof Menu> | null>(null)

// ── Credential prompt ──────────────────────────────────────────
const credentialVisible = ref(false)
const credentialTitle = ref('')
const credentialSubtitle = ref('')
const credentialFields = ref<('user' | 'password')[]>([])
const credentialResolve = ref<((result: CredentialResult | null) => void) | null>(null)

const credentialInitialUser = ref('')
const credentialInitialPassword = ref('')

function showCredentialDialog(
  title: string,
  subtitle: string,
  fields: ('user' | 'password')[],
  initialUser = '',
  initialPassword = ''
): Promise<CredentialResult | null> {
  return new Promise((resolve) => {
    credentialTitle.value = title
    credentialSubtitle.value = subtitle
    credentialFields.value = fields
    credentialInitialUser.value = initialUser
    credentialInitialPassword.value = initialPassword
    credentialResolve.value = resolve
    credentialVisible.value = true
  })
}

provide('showCredentialDialog', showCredentialDialog)

function onCredentialResolve(result: CredentialResult | null) {
  credentialVisible.value = false
  if (credentialResolve.value) {
    credentialResolve.value(result)
    credentialResolve.value = null
  }
}

// ── MCP approval bridge (external AI agents) ──────────────────────
// backend mcp:approval-request event → dialog → ResolveMCPApproval binding.
const mcpApprovalVisible = ref(false)
const mcpApprovalRequest = ref<MCPApprovalRequest | null>(null)
let unsubMcpApproval: (() => void) | null = null
let unsubMcpSessionCreated: (() => void) | null = null

function onMcpApprovalResolve(approved: boolean, reason: string) {
  const req = mcpApprovalRequest.value
  mcpApprovalVisible.value = false
  mcpApprovalRequest.value = null
  if (req) ResolveMCPApproval(req.id, approved, reason).catch(() => {})
}

// MCP-created sessions follow the same terminal mounting and sizing flow as UI sessions.
async function onMcpSessionCreated(payload: { sessionId: string; type: string; name?: string; host?: string; config: ConnectionConfig }) {
  const cfg = payload.config
  const panel = panelStore.createPanel(cfg, payload.type as PanelType)
  const title = payload.name || payload.host || payload.type
  panelStore.updateTitle(panel.id, title)
  panelStore.bindSession(panel.id, payload.sessionId)
  sessionStore.initSession(payload.sessionId)
  sessionStore.updateStatus(payload.sessionId, 'connecting')
  const tab = tabStore.activeTab?.type === 'start'
    ? tabStore.replaceStartTab(tabStore.activeTab.id, title, panel.id)
    : tabStore.createTerminalTab(title, panel.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  // TCP sessions already connect during CreateSession; other text terminals
  // wait for measured dimensions before starting their PTY.
  if (payload.type === 'tcp') return
  try {
    const size = await waitForTerminalSize(payload.sessionId)
    cfg.initialCols = size.cols
    cfg.initialRows = size.rows
    await SessionStart(payload.sessionId, cfg)
  } catch (e) {
    console.error('Failed to start MCP terminal:', e)
    CloseSession(payload.sessionId).catch(() => {})
  }
}


// 凭据补全统一在 composables/useTunnelCredentials.ts：终端连接、容器、K8s 等
// 入口共用同一实现。App.vue 自身的 showCredentialDialog 无法被本组件
// inject 到，需显式传入。
const { ensureConnectionCredentials: ensureCredentials } = useTunnelCredentials(showCredentialDialog)

let inputMenuTarget: HTMLInputElement | HTMLTextAreaElement | HTMLElement | null = null
interface InputMenuSelection {
  text: string
  range?: Range
  start?: number | null
  end?: number | null
}
let inputMenuSelection: InputMenuSelection | null = null

function closeInputMenu() {
  inputMenuVisible.value = false
  inputMenuTarget = null
  inputMenuSelection = null
}

function onInputContextMenu(e: Event) {
  const { x, y, target, readonly } = (e as CustomEvent).detail as {
    x: number; y: number; target: HTMLElement; readonly?: boolean
  }
  inputMenuTarget = target
  inputMenuReadonly.value = !!readonly
  inputMenuSelection = captureInputMenuSelection(target, !!readonly)
  inputMenuRef.value?.openAt(x, y)
}

function captureInputMenuSelection(target: HTMLElement, readonly: boolean): InputMenuSelection {
  if (readonly) {
    return { text: window.getSelection()?.toString() || target.textContent || '' }
  }
  if (target.isContentEditable) {
    const selection = window.getSelection()
    if (selection && selection.rangeCount > 0) {
      const range = selection.getRangeAt(0)
      if (target.contains(range.commonAncestorContainer)) {
        return { text: selection.toString(), range: range.cloneRange() }
      }
    }
    const range = document.createRange()
    range.selectNodeContents(target)
    range.collapse(false)
    return { text: '', range }
  }
  const input = target as HTMLInputElement | HTMLTextAreaElement
  return {
    text: input.value.substring(input.selectionStart ?? 0, input.selectionEnd ?? 0),
    start: input.selectionStart,
    end: input.selectionEnd,
  }
}

function restoreInputMenuSelection(el: HTMLElement, selection: InputMenuSelection) {
  if (el.isContentEditable) {
    el.focus()
    const current = window.getSelection()
    current?.removeAllRanges()
    current?.addRange(selection.range!.cloneRange())
    return
  }
  if (typeof selection.start === 'number' && typeof selection.end === 'number') {
    const input = el as HTMLInputElement | HTMLTextAreaElement
    input.focus()
    input.setSelectionRange(selection.start, selection.end)
  }
}

async function inputMenuCut() {
  const el = inputMenuTarget
  const selection = inputMenuSelection
  closeInputMenu()
  if (!el || !selection) return
  if (!selection.text) return
  const copied = await writeClipboard(selection.text)
  if (!copied) return
  if (el.isContentEditable) {
    restoreInputMenuSelection(el, selection)
    const s = window.getSelection()
    if (s && s.rangeCount > 0) { s.getRangeAt(0).deleteContents() }
  } else {
    setInputSelection(el as HTMLInputElement | HTMLTextAreaElement, '')
  }
  el.dispatchEvent(new Event('input', { bubbles: true }))
}

function inputMenuCopy() {
  const el = inputMenuTarget
  const selection = inputMenuSelection
  closeInputMenu()
  if (!el || !selection) return
  if (!selection.text) return
  void writeClipboard(selection.text)
}

function inputMenuPaste() {
  const el = inputMenuTarget
  const selection = inputMenuSelection
  closeInputMenu()
  if (!el || !selection) return
  Clipboard.Text().then(text => {
    if (el.isContentEditable) {
      restoreInputMenuSelection(el, selection)
      insertTextAtContentEditable(el, text)
    } else {
      restoreInputMenuSelection(el, selection)
      setInputSelection(el as HTMLInputElement | HTMLTextAreaElement, text)
    }
    el.dispatchEvent(new Event('input', { bubbles: true }))
  }).catch(() => {})
}

function inputMenuSelectAll() {
  const el = inputMenuTarget
  const readonly = inputMenuReadonly.value
  if (el && readonly) {
    const range = document.createRange()
    range.selectNodeContents(el)
    const sel = window.getSelection()
    sel?.removeAllRanges()
    sel?.addRange(range)
  } else if (el && 'select' in el) {
    (el as HTMLInputElement | HTMLTextAreaElement).select()
  }
  closeInputMenu()
}

function setInputSelection(el: HTMLInputElement | HTMLTextAreaElement, text: string) {
  // <input type="number"> (e.g. the port field) doesn't expose a text selection, so
  // selectionStart/End are null. Falling the offsets back to 0 would prepend the pasted
  // text instead of replacing it. Treat the unreadable selection as select-all so paste
  // overwrites the whole value (issue #555).
  const start = el.selectionStart
  const end = el.selectionEnd
  if (start === null || end === null) {
    el.value = text
    el.setSelectionRange(text.length, text.length)
    el.focus()
    return
  }
  el.value = el.value.substring(0, start) + text + el.value.substring(end)
  const pos = start + text.length
  el.setSelectionRange(pos, pos)
  el.focus()
}

function insertTextAtContentEditable(el: HTMLElement, text: string) {
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
}

// Adjust the terminal font size, clamped to the valid range. Shared by the
// Ctrl/Cmd+wheel zoom and the zoom-font-in/out shortcuts.
function adjustFontSize(delta: number) {
  const ts = settingsStore.settings.terminal
  const next = Math.max(8, Math.min(32, ts.fontSize + delta))
  if (next !== ts.fontSize) {
    ts.fontSize = next
    settingsStore.save()
  }
}

function onWheel(e: WheelEvent) {
  // Ctrl/Cmd + wheel zooms the terminal font — macOS (Cmd) and Windows/Linux
  // (Ctrl) alike. Skipped when the user disables it (issue #671, e.g. for
  // scroll-sensitive Mac mice); without preventDefault the wheel then scrolls
  // normally.
  if ((e.ctrlKey || e.metaKey) && settingsStore.settings.terminal.ctrlWheelZoom !== false) {
    e.preventDefault()
    adjustFontSize(e.deltaY < 0 ? 1 : -1)
  }
}

// Platform digit shortcuts: macOS uses Cmd/Option, Windows and Linux use
// Ctrl/Alt. Cmd/Ctrl+1…9 switches tabs, Alt/Option+1…9 switches workspace
// panels. Both digit families are user-configurable
// (keyboard.tabSwitchModifier / keyboard.panelSwitchModifier); when they
// resolve to the same combo the panel shortcuts step aside and their badges
// hide. Panel maximizing moved to the configurable keyboard settings
// (maximizePanel action).
let isMac = false
function onPlatformSystemShortcut(e: KeyboardEvent) {
  // Stand down while the settings page captures a rebind — otherwise
  // recording e.g. Ctrl+1 would switch tabs (and Cmd+Q would quit on mac).
  if (e.defaultPrevented || isRebinding() || remoteDesktopOwnsKey(e)) return
  const digitMatch = e.code.match(/^Digit([1-9])$/)
  if (digitMatch) {
    const kb = settingsStore.settings.keyboard
    const target = matchDigitShortcut(e, isMac, kb.tabSwitchModifier, kb.panelSwitchModifier)
    if (target === 'panel') {
      const tab = tabStore.activeTab
      if (!tab || tab.type !== 'workspace') return
      const panelId = tab.panelIds[Number(digitMatch[1]) - 1]
      if (!panelId) return
      e.preventDefault()
      e.stopImmediatePropagation()
      tabStore.setActivePanel(tab.id, panelId)
      nextTick(() => focusPanelTerminal(panelId))
      return
    }
    if (target === 'tab') {
      const tab = tabStore.tabs[Number(digitMatch[1]) - 1]
      if (!tab) return
      e.preventDefault()
      e.stopImmediatePropagation()
      tabStore.setActiveTab(tab.id)
      return
    }
  }
  if (!isMac || !e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return
  const key = e.key.toLowerCase()
  if (key === 'q') {
    e.preventDefault()
    Application.Quit()
  } else if (key === 'w') {
    e.preventDefault()
    const t = tabStore.activeTab
    if (t) closeTab(t.id)
  }
}

onMounted(async () => {
  connectionStore.load()
  aiStore.init()
  updateCheck.initAutoCheck()

  // Load local-only state (sidebar visibility, background image, etc.)
  await localStateStore.init()
  await loadBackgroundImage()
  // Mobile always opens with both sidebars closed, regardless of the
  // persisted desktop state — screen space is too tight to restore panels.
  sidebarVisible.value = isMobilePlatform()
    ? false
    : (localStateStore.state.sidebarVisible ?? false)
  // Pre-load quick commands so suggestions can read them immediately
  useQuickCommandStore().load()
  // Pre-load tunnels so auto-start state and the panel are ready
  useTunnelStore().load()
  // Auto-open start tab if no tabs are open
  if (tabStore.tabs.length === 0) {
    tabStore.createStartTab()
  }
  // Reopen-last-session prompt (issue #937): ask before touching tabs, then
  // start recording so the snapshot always mirrors what's open. Not awaited —
  // the dialog must not block the rest of the mount sequence.
  maybeRestoreLastTabs()
  installTabSnapshotSaver()
  // Pre-load noVNC so VNC tab switches don't pay the dynamic import cost.
  import('@novnc/novnc').then((m: any) => {
    ;(window as any).__novnc_RFB = m.default || m
  }).catch(() => {})
  window.addEventListener('input:contextmenu', onInputContextMenu)
  // Capture phase: xterm v6's viewport stopPropagation()s wheel events it
  // scrolls, but bails on defaultPrevented — so we must preempt it.
  document.addEventListener('wheel', onWheel, { passive: false, capture: true })
  // macOS system shortcuts (Cmd+Q / Cmd+W) — the digit shortcuts in
  // onPlatformSystemShortcut are armed on every platform.
  try {
    const platform = await GetPlatform()
    isMac = platform === 'darwin'
    // Expose for platform-specific CSS (e.g. dialog close-button styles)
    document.documentElement.dataset.platform = platform
  } catch {
    isMac = false
  }
  document.addEventListener('keydown', onPlatformSystemShortcut, true)
  // Keyboard shortcuts — load once on mount, watch for settings changes
  applyKeybindings()
  installGlobalListener()

  // Restore terminal focus after window drags and sidebar scrollbar clicks
  // (issue #285) — see composables/useFocusTerminal.ts for the policy.
  uninstallFocusRestore = installTerminalFocusRestore()

  // Tray menu: "Settings" / "About" show the window (Go side) then land here.
  unsubTrayOpenSettings = Events.On('app:open-settings', () => openSettings())
  unsubTrayOpenAbout = Events.On('app:open-about', () => openSettings('about'))
  // MCP (external AI agents): approval dialogs + session takeover.
  unsubMcpApproval = Events.On('mcp:approval-request', (ev) => {
    mcpApprovalRequest.value = ev.data as MCPApprovalRequest
    mcpApprovalVisible.value = true
  })
  unsubMcpSessionCreated = Events.On('mcp:session-created', (ev) => {
    void onMcpSessionCreated(ev.data)
  })
  // Panel/Tab/StartTab menu actions
  // Sidebar connection dropped onto a workspace panel: connect it and split
  // in at the drop position (see WorkspaceContent.onPanelDrop Case 0).
  window.addEventListener('app:connect-workspace-panel', ((e: CustomEvent) => {
    const { config, workspaceTabId, targetPanelId, direction, insertBefore } = e.detail || {}
    if (config && workspaceTabId) {
      connectTerminalSession(config, true, undefined, workspaceTabId, { targetPanelId, direction, insertBefore })
    }
  }) as EventListener)
  window.addEventListener('app:open-connection-at', ((e: CustomEvent) => {
    const { connId, index } = e.detail || {}
    const c = connId ? connectionStore.connections.find(x => x.id === connId) : undefined
    if (c) void openConnectionAtIndex(c, typeof index === 'number' ? index : tabStore.tabs.length)
  }) as EventListener)
  window.addEventListener('app:connect-sftp', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; openFileBrowser(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-terminal', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) onConnect(c)
  }) as EventListener)
  window.addEventListener('app:connect-wsl-file', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchWslFileBrowser(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-container-file', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchContainerFileBrowser(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-monitor', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; openMonitorPanel(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-rdp', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchConnection(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-vnc', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchConnection(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-spice', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchConnection(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-x11-desktop', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchConnection(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-db', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchConnection(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-k8s', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchConnection(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-ftp', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchConnection(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-smb', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchConnection(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-webdav', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchConnection(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)
  window.addEventListener('app:connect-s3', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; launchConnection(c, { prevStart: prev?.type === 'start' ? prev : undefined }) }
  }) as EventListener)

  // DB/Redis/Mongo tabs create their connection via connectionLauncher (runSpec),
  // not in a content component, so their right-click 「重连」(Reconnect) is handled here.
  // Terminal and desktop-protocol panels handle the same event themselves.
  window.addEventListener('panel:reconnect', onPanelReconnectEvent)

  await checkCredentials()
})

function navigatePanel(dir: number) {
  const t = tabStore.activeTab
  if (!t || t.type !== 'workspace') return
  if (t.panelIds.length <= 1) return
  const current = t.activePanelId || t.panelIds[0]
  const idx = t.panelIds.indexOf(current)
  if (idx < 0) return
  const next = t.panelIds[(idx + dir + t.panelIds.length) % t.panelIds.length]
  tabStore.setActivePanel(t.id, next)
}

// What the tab bar "+" button (relayed from TabsList) and the newConnection
// shortcut open (issue #1043): the start page by default, or a local terminal
// with the default shell when newTabAction says so. No shell available ->
// fall back to the start page.
function createNewTab() {
  if (settingsStore.settings.newTabAction === 'localTerminal') {
    const shells = settingsStore.availableShells
    const preferred = settingsStore.settings.defaultLocalShell
    const shell = preferred && shells.includes(preferred) ? preferred : shells[0]
    if (shell) {
      createLocalTerminalWithShell(shell)
      return
    }
  }
  tabStore.createStartTab()
}

// ── Keyboard shortcut handlers (module-level so watch can access them) ──
const actionHandlers: Record<ShortcutAction, () => void> = {
  nextTab: () => tabStore.nextTab(),
  prevTab: () => tabStore.prevTab(),
  newConnection: () => { createNewTab() },
  toggleSidebar: () => {
    if (sidebarVisible.value) {
      sidebarVisible.value = false
      const pid = tabStore.getActivePanelId()
      if (pid) nextTick(() => focusPanelTerminal(pid))
    } else {
      sidebarVisible.value = true
      nextTick(() => sidebarRef.value?.focusSearch())
    }
  },
  toggleFilePanel: () => {
    // 切换文件边栏（需当前面板支持，如 SSH/WSL/容器 exec 终端）
    companionStore.toggleFiles()
  },
  toggleMonitor: () => {
    // 切换资源监控边栏（需当前面板支持，如 SSH 终端）
    companionStore.toggleMonitor()
  },
  openQuickCommands: () => {
    sidebarVisible.value = true
    nextTick(() => sidebarRef.value?.openQuickCommands())
  },
  focusTerminal: () => {
    const pid = tabStore.getActivePanelId()
    if (pid) focusPanelTerminal(pid)
  },
  maximizePanel: () => {
    const tab = tabStore.activeTab
    if (!tab || tab.type !== 'workspace' || !tab.activePanelId) return
    const panelId = tab.activePanelId
    tabStore.toggleWorkspacePanelMaximize(tab.id)
    nextTick(() => focusPanelTerminal(panelId))
  },
  lockAI: () => {
    const t = tabStore.activeTab
    if (!t) return
    let panelId: string | null = null
    if (t.type === 'workspace') {
      panelId = t.activePanelId || t.panelIds[0] || null
    } else if (t.type === 'terminal') {
      panelId = t.panelId
    }
    if (panelId) onToggleAiLock(panelId)
  },
  focusAI: () => {
    if (aiStore.visible) {
      aiStore.visible = false
      const pid = tabStore.getActivePanelId()
      if (pid) nextTick(() => focusPanelTerminal(pid))
    } else {
      aiStore.visible = true
      nextTick(() => aiSidebarRef.value?.focusInput())
    }
  },
  closePanel: () => {
    const t = tabStore.activeTab
    if (!t) return
    if (t.locked) return
    if (t.type === 'workspace' && t.panelIds.length > 1) {
      const panelId = t.activePanelId || t.panelIds[t.panelIds.length - 1]
      const p = panelStore.getPanel(panelId)
      if (p?.sessionId) CloseSession(p.sessionId).catch(() => {})
      companionStore.disposeForPanel(panelId).catch(() => {})
      tabStore.removePanelFromWorkspaceTab(t.id, panelId)
      panelStore.removePanel(panelId)
    } else if (t.type === 'workspace' && t.panelIds.length === 1) {
      const panelId = t.panelIds[0]
      const p = panelStore.getPanel(panelId)
      if (p?.sessionId) CloseSession(p.sessionId).catch(() => {})
      companionStore.disposeForPanel(panelId).catch(() => {})
      tabStore.removePanelFromWorkspaceTab(t.id, panelId)
      panelStore.removePanel(panelId)
    } else {
      closeTab(t.id)
    }
  },
  terminalSearch: () => {
    const pid = tabStore.getActivePanelId()
    if (pid) window.dispatchEvent(new CustomEvent('terminal:open-search', { detail: { panelId: pid } }))
  },
  copy: () => {
    const pid = tabStore.getActivePanelId()
    if (pid) window.dispatchEvent(new CustomEvent('terminal:copy', { detail: { panelId: pid } }))
  },
  toggleLineNumbers: () => {
    // Line numbers / timestamps are global terminal settings, so these toggle
    // them directly rather than dispatching a per-panel event.
    const cur = settingsStore.settings.terminal.showLineNumbers ?? false
    settingsStore.updateTerminal({ showLineNumbers: !cur })
  },
  toggleTimestamps: () => {
    const cur = settingsStore.settings.terminal.showTimestamps ?? false
    settingsStore.updateTerminal({ showTimestamps: !cur })
  },
  zoomFontIn: () => adjustFontSize(1),
  zoomFontOut: () => adjustFontSize(-1),
  paste: () => {
    const pid = tabStore.getActivePanelId()
    if (pid) window.dispatchEvent(new CustomEvent('terminal:paste', { detail: { panelId: pid } }))
  },
  navigatePrev: () => navigatePanel(-1),
  navigateNext: () => navigatePanel(1),
  openSettings: () => openSettings(),
  duplicateSession: () => {
    // Same logic as the tab context menu's "复制会话": delegate to the shared
    // duplicate routine so both entry points behave identically.
    const tab = tabStore.activeTab
    if (!tab) return
    if (tab.type === 'workspace') {
      // A workspace holds several panels; the shortcut duplicates the focused
      // one and keeps the duplicate beside it in the same workspace.
      const pid = tabStore.getActivePanelId()
      const panel = pid ? panelStore.getPanel(pid) : undefined
      if (panel) {
        duplicateSession(
          { type: 'terminal', panelId: pid, title: panel.title },
          { workspaceId: tab.id, targetPanelId: pid },
        )
      }
      return
    }
    duplicateSession(tab)
  },
}

function applyKeybindings() {
  // Digit shortcuts (Ctrl/Cmd+digit → tab, Alt/Option+digit → workspace
  // panel) are fixed platform bindings handled by onPlatformSystemShortcut.
  loadKeybindings(settingsStore.settings.keyboard, actionHandlers)
}

onUnmounted(() => {
  uninstallGlobalListener()
  uninstallFocusRestore?.()
  updateCheck.dispose()
  window.removeEventListener('input:contextmenu', onInputContextMenu)
  document.removeEventListener('wheel', onWheel, { capture: true })
  document.removeEventListener('keydown', onPlatformSystemShortcut, true)
  window.removeEventListener('panel:reconnect', onPanelReconnectEvent)
  // Wails EventsOn teardown (FE-03)
  unsubTrayOpenSettings?.()
  unsubTrayOpenAbout?.()
  unsubMcpApproval?.()
  unsubMcpSessionCreated?.()
  settingsStore.dispose?.()
  connectionStore.dispose?.()
  syncStore.dispose?.()
  tunnelStore.dispose?.()
  disposeSessionStore?.()
  credStore.dispose()
})

function openSettings(category?: string) {
  // Check if settings tab already exists
  const existingTab = tabStore.tabs.find(t => t.type === 'settings')
  if (existingTab) {
    tabStore.setActiveTab(existingTab.id)
  } else {
    const panel = panelStore.createPanel(null, 'settings')
    panelStore.updateTitle(panel.id, t('settings.title'))
    const tab = tabStore.createTab('settings', t('settings.title'), panel.id)
    panelStore.movePanelToTab(panel.id, tab.id)
  }
  // Jump to the requested category (e.g. ai / identities / proxies)
  if (category) {
    settingsStore.activeCategory = category
  }
}

async function closeTab(tabId: string, opts: { skipConfirm?: boolean } = {}) {
  // Close session before removing panel to clean up Go-side resources
  const tab = tabStore.tabs.find(t => t.id === tabId)
  if (tab?.locked) return
  if (tab && tab.type === 'start') {
    tabStore.closeTab(tabId)
    nextTick(() => {
      if (tabStore.tabs.length === 0) {
        tabStore.createStartTab()
      }
    })
    return
  }
  // Confirm before closing connected sessions to prevent accidental disconnect
  if (tab && tab.type !== 'settings' && !opts.skipConfirm) {
    const panelIds = tab.type === 'workspace' ? tab.panelIds : 'panelId' in tab ? [tab.panelId] : []
    const hasConnected = panelIds.some(pid => {
      const p = panelStore.getPanel(pid)
      if (!p?.sessionId) return false
      return sessionStore.getStatus(p.sessionId) === 'connected'
    })
    if (hasConnected) {
      if (!settingsStore.settings.closeTabPrompt) {
        // skip dialog, proceed to close
      } else {
        const dontShowAgain = ref(false)
        try {
          await ElMessageBox.confirm(
            h('div', { style: 'display:flex;flex-direction:column;gap:0.625rem' }, [
              h('span', t('tab.closeConnectedConfirm')),
              h(ElCheckbox, {
                'onUpdate:modelValue': (v: boolean) => { dontShowAgain.value = v }
              }, () => t('tab.dontShowAgain'))
            ]),
            t('tab.closeConfirmTitle'),
            { confirmButtonText: t('tab.close'), cancelButtonText: t('conn.cancel'), type: 'warning' }
          )
        } catch {
          return
        }
        if (dontShowAgain.value) {
          settingsStore.settings.closeTabPrompt = false
          settingsStore.save()
        }
      }
    }
  }
  if (tab && tab.type === 'rdp') {
    const p = panelStore.getPanel(tab.panelId)
    if (p?.sessionId) {
      try { await CloseSession(p.sessionId) } catch (_) {}
    }
  }
  // Close VNC session
  if (tab && tab.type === 'vnc') {
    const p = panelStore.getPanel(tab.panelId)
    if (p?.sessionId) {
      try { await CloseSession(p.sessionId) } catch (_) {}
    }
    panelStore.disconnectVNCCache(tab.panelId)
    panelStore.removeVNCCache(tab.panelId)
  }
  // Close SPICE session
  if (tab && tab.type === 'spice') {
    const p = panelStore.getPanel(tab.panelId)
    if (p?.sessionId) {
      try { await CloseSession(p.sessionId) } catch (_) {}
    }
    panelStore.disconnectSPICECache(tab.panelId)
    panelStore.removeSPICECache(tab.panelId)
  }
  // Close database session
  if (tab && tab.type === 'database') {
    const p = panelStore.getPanel(tab.panelId)
    if (p?.sessionId) {
      try { await CloseSession(p.sessionId) } catch (_) {}
    }
  }
  // Close redis session
  if (tab && (tab.type === 'redis' || tab.type === 'mongodb' || tab.type === 'elasticsearch')) {
    const p = panelStore.getPanel(tab.panelId)
    if (p?.sessionId) {
      try { await CloseSession(p.sessionId) } catch (_) {}
    }
  }
  // Close monitor session
  if (tab && tab.type === 'monitor') {
    const p = panelStore.getPanel(tab.panelId)
    if (p?.sessionId) {
      try { await CloseSession(p.sessionId) } catch (_) {}
    }
  }
  // Close container session (KeepAlive may keep the component mounted, so the
  // poll timer + backend connection in containerStore must be closed explicitly)
  if (tab && tab.type === 'container') {
    containerStore.close(tab.id)
  }
  // Terminal sessions must be explicitly closed to terminate the connection/shell process
  if (tab && tab.type === 'terminal') {
    const p = panelStore.getPanel(tab.panelId)
    if (p?.sessionId) {
      try { await CloseSession(p.sessionId) } catch (_) {}
    }
  }
  // Workspace: close each panel session
  if (tab && tab.type === 'workspace') {
    for (const pid of tab.panelIds) {
      const p = panelStore.getPanel(pid)
      if (p?.sessionId) {
        try { await CloseSession(p.sessionId) } catch (_) {}
      }
    }
  }
  // X11 desktop session cleanup
  if (tab && tab.type === 'x11-desktop') {
    const p = panelStore.getPanel(tab.panelId)
    if (p?.sessionId) {
      try { await CloseSession(p.sessionId) } catch (_) {}
    }
  }
  const panelIds = tabStore.closeTab(tabId)
  // Dispose SSH companion sidebars (sftp/monitor) bound to these panels
  companionStore.disposeForPanels(panelIds).catch(() => {})
  panelIds.forEach(pid => {
    // Drop transfer-event routing and the panel's task list at close time —
    // KeepAlive may keep the tab component cached, so its onUnmounted (if any)
    // can run much later or never.
    const p = panelStore.getPanel(pid)
    if (p?.sessionId) unregisterTransferRoute(p.sessionId)
    panelStore.removeTransferTasks(pid)
  })
  panelIds.forEach(pid => panelStore.removePanel(pid))
  nextTick(() => {
    if (tabStore.tabs.length === 0) {
      tabStore.createStartTab()
    }
  })
}

// Batch close (close left/right/others). Consolidate the "has connected
// sessions" confirmation into a single dialog so users don't get one prompt
// per tab, and honor "don't show again" for the current batch too.
async function closeTabBatch(tabIds: string[]) {
  if (!tabIds.length) return
  const targets = tabIds
    .map(id => tabStore.tabs.find(t => t.id === id))
    .filter((t): t is NonNullable<typeof t> => !!t && !t.locked)
  if (!targets.length) return

  const connectedCount = targets.reduce((n, tab) => {
    if (tab.type === 'settings' || tab.type === 'start') return n
    const panelIds = tab.type === 'workspace' ? tab.panelIds : 'panelId' in tab ? [tab.panelId] : []
    const hit = panelIds.some(pid => {
      const p = panelStore.getPanel(pid)
      if (!p?.sessionId) return false
      return sessionStore.getStatus(p.sessionId) === 'connected'
    })
    return hit ? n + 1 : n
  }, 0)

  if (connectedCount > 0 && settingsStore.settings.closeTabPrompt) {
    const dontShowAgain = ref(false)
    try {
      await ElMessageBox.confirm(
        h('div', { style: 'display:flex;flex-direction:column;gap:0.625rem' }, [
          h('span', t('tab.closeConnectedBatchConfirm', { count: connectedCount })),
          h(ElCheckbox, {
            'onUpdate:modelValue': (v: boolean) => { dontShowAgain.value = v }
          }, () => t('tab.dontShowAgain'))
        ]),
        t('tab.closeConfirmTitle'),
        { confirmButtonText: t('tab.close'), cancelButtonText: t('conn.cancel'), type: 'warning' }
      )
    } catch {
      return
    }
    if (dontShowAgain.value) {
      settingsStore.settings.closeTabPrompt = false
      settingsStore.save()
    }
  }

  for (const tab of targets) {
    await closeTab(tab.id, { skipConfirm: true })
  }
}

function getPanelConfig(panelId: string): ConnectionConfig | null {
  return panelStore.getPanel(panelId)?.config || null
}

function getPanelSessionId(panelId: string): string | null {
  return panelStore.getPanel(panelId)?.sessionId || null
}

function k8sConnectionForTab(tab: any): ConnectionConfig {
  const conn = connectionStore.connections.find((c: ConnectionConfig) => c.id === tab.connectionId)
  return conn || ({
    id: tab.connectionId,
    name: tab.name,
    type: 'k8s',
    host: '',
    port: 0,
    user: '',
    authType: 'password',
    k8sConfigPath: '',
    k8sConfigInline: '',
    k8sContext: '',
    k8sNamespace: '',
  } as ConnectionConfig)
}

function onSaveOnly(saveOnlyConfig: ConnectionConfig) {
  persistConnection(saveOnlyConfig, !!editConfig.value?.id)
  RecordRecentConnection(saveOnlyConfig.id)
}

// Transient connection ("connect only"): connect a new form config without
// persisting it to the connection list. Reuses onConnect with persist=false.
function onConnectOnly(config: ConnectionConfig) {
  editConfig.value = null
  onConnect(config, undefined, false, false)
}

// Companion entries that force a spec regardless of config.type (used by the
// sidebar's `kind` routing and the app:connect-sftp / app:connect-monitor
// window events).
function onSidebarConnect(config: ConnectionConfig, kind?: 'file' | 'wsl-file' | 'monitor') {
  const prev = tabStore.activeTab
  const prevStart = prev?.type === 'start' ? prev : undefined
  if (kind === 'file') return void openFileBrowser(config, prevStart)
  if (kind === 'monitor') return void openMonitorPanel(config, prevStart)
  if (kind === 'wsl-file') return void launchWslFileBrowser(config, { prevStart })
  return onConnect(config)
}

function openFileBrowser(config: ConnectionConfig, prevStart?: any) {
  return launchFileBrowser(config, { prevStart })
}

function openMonitorPanel(config: ConnectionConfig, prevStart?: any) {
  return launchMonitor(config, { prevStart })
}

// Atomically remove a start tab and place a newly-created tab in its position.
// Returns a cleanup function: call it AFTER creating the tab to reposition it.
function closeStartAndReposition(prevTab: any): (newTabId: string) => void {
  const idx = tabStore.tabs.indexOf(prevTab)
  if (idx >= 0) tabStore.tabs.splice(idx, 1)
  return (newTabId: string) => {
    if (idx < 0) return
    const newIdx = tabStore.tabs.findIndex((t: any) => t.id === newTabId)
    if (newIdx < 0 || newIdx === idx) return
    const [moved] = tabStore.tabs.splice(newIdx, 1)
    tabStore.tabs.splice(Math.min(idx, tabStore.tabs.length), 0, moved)
  }
}

configureLauncher({ ensureCredentials, closeStartAndReposition })

// ── Reopen last session's tabs (issue #937-4) ──
// The snapshot itself is written continuously by installTabSnapshotSaver;
// this is the startup half: prompt (when the setting is on) and re-open the
// recorded tabs through the regular connect flows.
async function maybeRestoreLastTabs() {
  if (!settingsStore.settings.restoreTabsPrompt) return
  const snapshot = localStateStore.state.lastTabsSnapshot
  if (!snapshot?.tabs?.length) return
  try {
    await ElMessageBox.confirm(
      t('start.restoreTabsMessage', { count: snapshot.tabs.length }),
      t('start.restoreTabsTitle'),
      {
        confirmButtonText: t('start.restoreTabsConfirm'),
        cancelButtonText: t('common.cancel'),
        type: 'info'
      }
    )
  } catch {
    // One-shot prompt: a decline means "don't offer these tabs again", so
    // drop the snapshot. It only re-accumulates once the user actually opens
    // tabs (the watcher in installTabSnapshotSaver re-records on change).
    localStateStore.update({ lastTabsSnapshot: null })
    return // user declined
  }
  await restoreTabsSnapshot(snapshot, {
    connectTerminal: (c, p) => connectTerminalSession(c, p, undefined),
    connectMember: workspaceConnectMember
  })
}

async function onConnect(config: ConnectionConfig, keepOpen?: boolean, wasEdit?: boolean, persist = true, targetWorkspaceId?: string) {
  // Saved workspace connection: open it as a live workspace instead of the
  // per-type launch flow (no spec exists — and must not exist — for this type).
  if (config.type === 'workspace') {
    await openSavedWorkspace(config, { connectMember: workspaceConnectMember })
    return
  }
  const prev = tabStore.activeTab
  const prevStart = (prev?.type === 'start' && !keepOpen) ? prev : undefined
  await launchConnection(config, {
    persist,
    wasEdit,
    prevStart,
    connectTerminal: (c, p) => connectTerminalSession(c, p, prev, targetWorkspaceId),
  })
}

// Generic terminal connect path (ssh/telnet/mosh/local/wsl/tcp/serial).
// Persistence bookkeeping happened in launchConnection. The type-specific
// non-terminal specs live in connectionLauncher.ts.
// Returns the created panel id so workspace orchestration (create/restore)
// can remap layouts; status distinguishes credential-cancel from failure.
// placement (workspace targets only): where the new panel splits in relative
// to targetPanelId — used by the sidebar-connection-onto-workspace-panel drop.
interface WorkspacePlacement {
  targetPanelId: string
  direction: 'horizontal' | 'vertical'
  insertBefore: boolean
}
async function connectTerminalSession(config: ConnectionConfig, persist: boolean, prev: any, targetWorkspaceId?: string, placement?: WorkspacePlacement): Promise<{ status: 'ok' | 'cancelled' | 'failed'; panelId?: string }> {
  // Credential check
  const resolved = await ensureCredentials(config)
  if (!resolved) return { status: 'cancelled' }
  config = resolved

  // Create session BEFORE panel so the terminal has a sessionId when it first
  // fires SessionResize. Otherwise the resize is silently dropped because the
  // terminal calls getSessionId() too early and never retries.
  //
  // Defer the actual Connect until BaseTerminal has mounted, fitAddon has
  // measured the real xterm cols/rows, and we can write that into
  // config.initialCols/Rows — otherwise the SSH/local PTY starts at the
  // default 80x24 and Claude Code draws its first batch of tables at that
  // width, drifting relative to later output that wraps at the real cols.
  let sessionId = ''
  try {
    const info = await CreateSession(config.type, {
      ...config,
      initialCols: 0,
      initialRows: 0,
    })
    sessionId = info.id
  } catch (e) {
    console.error('Failed to create session:', e)
    return { status: 'failed' }
  }

  const panel = panelStore.createPanel(config, config.type)
  const displayTitle = config.name || (config.type === 'local' || config.type === 'wsl'
    ? getShellLabel(config.shellPath)
    : config.type === 'serial'
    ? `${config.serialPort || 'Serial'} (${config.serialBaudRate || 115200})`
    : config.type === 'telnet'
    ? `${config.host}:${config.port}`
    : `${config.user}@${config.host}`)
  panelStore.updateTitle(panel.id, displayTitle)
  panelStore.bindSession(panel.id, sessionId)
  sessionStore.initSession(sessionId)
  const addedToWorkspace = targetWorkspaceId
    ? tabStore.addNewPanelToWorkspace(
        targetWorkspaceId,
        panel.id,
        placement?.targetPanelId,
        placement?.direction ?? 'horizontal',
        placement?.insertBefore ?? false
      )
    : false
  const tab = addedToWorkspace
    ? tabStore.tabs.find(t => t.id === targetWorkspaceId)!
    : prev?.type === 'start'
      ? tabStore.replaceStartTab(prev.id, panel.title, panel.id)
      : tabStore.createTerminalTab(panel.title, panel.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  if (persist) RecordRecentConnection(config.id)

  // Wait for BaseTerminal to mount + fitAddon.fit() to compute the real
  // xterm cols/rows (which itself waits for document.fonts.ready + 2 rAFs
  // to ensure the actually-loaded font is being measured). Then start the
  // session with those dimensions so the remote PTY matches from byte 0.
  const size = await waitForTerminalSize(sessionId)
  if (size.cols > 0 && size.rows > 0) {
    config.initialCols = size.cols
    config.initialRows = size.rows
  }
  try {
    await SessionStart(sessionId, config)
  } catch (e) {
    console.error('Failed to start session:', e)
    // Session is registered in the backend but never connected — close it
    // so it doesn't leak until app shutdown. UI rollback is the caller's
    // responsibility (this helper is reused across connect / duplicate).
    CloseSession(sessionId).catch(() => {})
  }
  return { status: 'ok', panelId: panel.id }
}

// ── Workspace orchestration ──
// One connect Member primitive shared by create-from-selection and open-saved;
// wraps the generic terminal path with workspace targeting.

function workspaceConnectMember(config: ConnectionConfig, workspaceTabId: string, persist: boolean) {
  return connectTerminalSession(config, persist, undefined, workspaceTabId)
}

// Sidebar connection dropped onto the tab bar: open it as a new tab at the
// requested position. Works for every connection type — the connect flow is
// the same as clicking the connection (workspace type opens as a workspace).
// The new tab is created wherever the flow puts it, then moved into place.
async function openConnectionAtIndex(config: ConnectionConfig, index: number) {
  const before = new Set(tabStore.tabs.map(t => t.id))
  await onConnect(config)
  const newTab = tabStore.tabs.find(t => !before.has(t.id))
  if (!newTab) return // nothing opened (credential cancelled / connect failed)
  const fromIdx = tabStore.tabs.findIndex(t => t.id === newTab.id)
  const toIdx = Math.max(0, Math.min(index, tabStore.tabs.length - 1))
  if (fromIdx !== toIdx) tabStore.moveTab(fromIdx, toIdx)
}

// Entry: 「在工作区打开 → 新建工作区」 from the sidebar/start-page context
// menu (targets = the multi-selection or the right-clicked connection).
async function onCreateWorkspaceFromConfigs(configs: ConnectionConfig[]) {
  await createWorkspaceFromSelection('', [], configs, { connectMember: workspaceConnectMember })
}

// Entry: 新建工作区 button (start page) — opens an empty workspace tab the
// user fills by dragging connections / terminal tabs in. Point-and-click
// creation from a selection lives in the「在工作区打开」context menu.
function onNewWorkspace() {
  tabStore.createWorkspaceTab(tabStore.generateWorkspaceName(tabStore.tabs), [], { root: { type: 'leaf', panelId: '' } })
}

// Force-reconnect a database-family panel (database/redis/mongodb/
// elasticsearch) from the tab's right-click 「重连」(Reconnect) menu. These
// sessions are created in connectionLauncher (runSpec), so — unlike terminal/
// desktop panels — no content component owns the lifecycle: close the old
// session, then create a fresh one and rebind the same panel.
async function reconnectDatabasePanel(panel: { id: string; sessionId: string | null; config: ConnectionConfig | null }) {
  const oldId = panel.sessionId
  if (oldId) {
    try { await CloseSession(oldId) } catch (_) {}
  }
  const cfg = panel.config
  if (!cfg) return
  // Standalone NoSQL types carry their session type in config.type; the SQL
  // family stays on the generic 'database' session.
  const sessionType = cfg.type === 'redis' || cfg.type === 'mongodb' || cfg.type === 'elasticsearch'
    ? cfg.type
    : 'database'
  try {
    const info = await CreateSession(sessionType, cfg)
    panelStore.bindSession(panel.id, info.id)
    sessionStore.initSession(info.id)
  } catch (e: any) {
    panelStore.updateStatus(panel.id, 'error')
    msg.error(`${t('db.connectFailed')}: ${e?.message || String(e)}`)
  }
}

// Force-reconnect a monitor panel. The session is created in
// connectionLauncher (runSpec), so it's re-initiated here like the database panels.
async function reconnectMonitorPanel(panel: { id: string; sessionId: string | null; config: ConnectionConfig | null }) {
  const oldId = panel.sessionId
  if (oldId) {
    try { await CloseSession(oldId) } catch (_) {}
  }
  const cfg = panel.config
  if (!cfg) return
  try {
    const info = await CreateSession('monitor', cfg)
    panelStore.bindSession(panel.id, info.id)
    sessionStore.initSession(info.id)
  } catch (e: any) {
    panelStore.updateStatus(panel.id, 'error')
    msg.error(`${t('tab.reconnectFailed')}: ${e?.message || String(e)}`)
  }
}

// Force-reconnect a file-transfer panel (sftp/ftp/smb/webdav/s3). The actual
// close→create→rebind orchestration lives in usePanelReconnect, shared with
// the refresh-triggered auto-reconnect in the file tab content.
async function reconnectSftpPanel(panel: { id: string; sessionId: string | null; config: ConnectionConfig | null }) {
  try {
    const newId = await reconnectFileTransferPanel(panel.id)
    if (!newId) panelStore.updateStatus(panel.id, 'error')
  } catch (e: any) {
    panelStore.updateStatus(panel.id, 'error')
    msg.error(`${t('tab.reconnectFailed')}: ${e?.message || String(e)}`)
  }
}

// Panel kinds whose sessions are created in connectionLauncher's database
// family spec (shared 'database' panel type + own session kind).
const DB_FAMILY_PANEL_TYPES = ['database', 'redis', 'mongodb', 'elasticsearch']

function onPanelReconnectEvent(e: Event) {
  const panelId = (e as CustomEvent)?.detail?.panelId
  if (!panelId) return
  const panel = panelStore.getPanel(panelId)
  if (!panel) return
  // These session types are created in connectionLauncher (not in a content
  // component), so their right-click 「重连」(Reconnect) is handled here.
  // Terminal and desktop-protocol panels handle the same event themselves.
  if (DB_FAMILY_PANEL_TYPES.includes(panel.type)) reconnectDatabasePanel(panel)
  else if (panel.type === 'monitor') reconnectMonitorPanel(panel)
  else if (panel.type === 'sftp') reconnectSftpPanel(panel)
}

function getShellLabel(path: string): string {
  return getShellLabelBase(path, 'Local')
}

async function createLocalTerminalWithShell(shellPath: string, keepOpen?: boolean) {
  // A `wsl://<distro>` shell opens a dedicated WSL terminal (own session type),
  // everything else stays a normal local terminal.
  const distro = parseWslFromShell(shellPath)
  if (distro) return createWslTerminal(distro, keepOpen)
  await createLocalTerminal(shellPath, keepOpen)
}

const pendingGroupId = ref<string | undefined>(undefined)

function onNewConnectionFromStart(payload?: { host?: string; groupId?: string; type?: string }) {
  pendingGroupId.value = payload?.groupId
  if (payload?.host) {
    const parsed = parseQuickConnect(payload.host)
    editConfig.value = (parsed || { host: payload.host }) as ConnectionConfig
  } else if (payload?.type) {
    editConfig.value = { type: payload.type } as ConnectionConfig
  } else {
    editConfig.value = null
  }
  showConnectionForm.value = true
}

const editConfig = ref<ConnectionConfig | null>(null)

function onEditConnection(config: ConnectionConfig) {
  editConfig.value = config
  showConnectionForm.value = true
}

function onChangeGroupFromStart(config: ConnectionConfig) {
  sidebarRef.value?.openChangeGroupFor([config.id])
}

function onChangeGroupFromStartIds(ids: string[]) {
  sidebarRef.value?.openChangeGroupFor(ids)
}

function onChangeGroupParentFromStart(groupId: string) {
  sidebarRef.value?.openChangeGroupForGroup(groupId)
}

function onToggleAiLock(panelId: string) {
  if (tabStore.isPanelAILocked(panelId)) {
    tabStore.removeAILockedPanel(panelId)
  } else {
    tabStore.addAILockedPanel(panelId)
  }
}

function onTabDragStart(_e: DragEvent, _tabId: string) {
  // Data is set in TabItem
}

async function createLocalTerminal(shellPath?: string, keepOpen?: boolean) {
  const panel = panelStore.createPanel(null, 'local')
  const shellName = getShellLabel(shellPath)
  panelStore.updateTitle(panel.id, shellName)

  try {
    // Use a stable ID based on shell type so repeated local terminals
    // merge into one recent-history entry instead of creating a new one
    // every time.
    const stableId = `local-terminal:${shellName.toLowerCase().replace(/\s+/g, '-')}`
    const config: ConnectionConfig = {
      id: stableId,
      name: shellName,
      type: 'local' as any,
      host: '',
      port: 0,
      user: '',
      authType: 'password' as any,
      shellPath: shellPath || undefined,
      initialCols: 0,
      initialRows: 0,
    }
    panel.config = config
    connectionStore.add(config)
    // Local terminal sessions are temporary — don't record in history
    const info = await CreateSession('local', config)
    panelStore.bindSession(panel.id, info.id)
    sessionStore.initSession(info.id)
    // Create tab AFTER session is bound so BaseTerminal mounts with valid sessionId
    const prev = tabStore.activeTab
    const tab = prev?.type === 'start' && !keepOpen
      ? tabStore.replaceStartTab(prev.id, panel.title, panel.id)
      : tabStore.createTerminalTab(panel.title, panel.id)
    panelStore.movePanelToTab(panel.id, tab.id)

    // Same as onConnect: measure the real xterm size, then start the PTY
    // at those dimensions.
    const size = await waitForTerminalSize(info.id)
    if (size.cols > 0 && size.rows > 0) {
      config.initialCols = size.cols
      config.initialRows = size.rows
    }
    try {
      await SessionStart(info.id, config)
    } catch (e) {
      console.error('Failed to start local session:', e)
      CloseSession(info.id).catch(() => {})
    }
  } catch (e) {
    console.error('Failed to create local terminal:', e)
    panelStore.removePanel(panel.id)
  }
}

// WSL terminal: a dedicated session type (`wsl`) that launches wsl.exe inside
// the distro and supports the file sidebar via its companion WSL file session.
async function createWslTerminal(distro: string, keepOpen?: boolean) {
  const shellPath = 'wsl://' + distro
  const panel = panelStore.createPanel(null, 'wsl')
  const shellName = getShellLabel(shellPath)
  panelStore.updateTitle(panel.id, shellName)

  try {
    const stableId = `wsl-terminal:${distro.toLowerCase().replace(/\s+/g, '-')}`
    const config: ConnectionConfig = {
      id: stableId,
      name: shellName,
      type: 'wsl' as any,
      host: '',
      port: 0,
      user: '',
      authType: 'password' as any,
      shellPath,
      initialCols: 0,
      initialRows: 0,
    }
    panel.config = config
    connectionStore.add(config)
    // WSL terminal sessions are temporary — don't record in history
    const info = await CreateSession('wsl', config)
    panelStore.bindSession(panel.id, info.id)
    sessionStore.initSession(info.id)
    const prev = tabStore.activeTab
    const tab = prev?.type === 'start' && !keepOpen
      ? tabStore.replaceStartTab(prev.id, panel.title, panel.id)
      : tabStore.createTerminalTab(panel.title, panel.id)
    panelStore.movePanelToTab(panel.id, tab.id)

    const size = await waitForTerminalSize(info.id)
    if (size.cols > 0 && size.rows > 0) {
      config.initialCols = size.cols
      config.initialRows = size.rows
    }
    try {
      await SessionStart(info.id, config)
    } catch (e) {
      console.error('Failed to start wsl session:', e)
      CloseSession(info.id).catch(() => {})
    }
  } catch (e) {
    console.error('Failed to create wsl terminal:', e)
    panelStore.removePanel(panel.id)
  }
}

watch(() => activeTab.value, (newTab) => {
  // Auto-focus terminal on tab switch (including new connections)
  if (newTab) {
    const pid = tabStore.getActivePanelId()
    if (pid) nextTick(() => focusPanelTerminal(pid))
  }
  // Keep the AI sidebar on the active host's conversation (#1036)
  aiStore.followActiveConnection()
})

// Workspace tab: switching the focused panel is also a host change
watch(() => tabStore.getActivePanelId(), () => {
  aiStore.followActiveConnection()
})

watch(sidebarVisible, async () => {
  localStateStore.update({ sidebarVisible: sidebarVisible.value })
})

watch(() => settingsStore.settings.keyboard, () => {
  applyKeybindings()
}, { deep: true })

watch(
  () => [localStateStore.state.backgroundEnabled, localStateStore.state.backgroundImage],
  () => loadBackgroundImage()
)
</script>

<style scoped>
.app-container {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  position: relative;
}
.main-content {
  display: flex;
  flex: 1;
  overflow: hidden;
  gap: 0;
  position: relative;
  z-index: 1;
}

.tab-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--bg-base);
}

.group-list {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}
.group-list .group-item {
  padding: 0.625rem 0.875rem;
  border-radius: var(--radius-sm);
  transition: background .15s;
}
.group-list .group-item:hover {
  background: var(--bg-hover);
}

.app-bg {
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background-color: transparent;
}
.app-bg::after {
  content: '';
  position: absolute;
  inset: 0;
  background: var(--bg-base);
  opacity: var(--bg-mask-opacity, 0.6);
}
.app-container.has-bg .main-content,
.app-container.has-bg .main-content :deep(*:not(:where(.xterm-cursor, [class*="xterm-bg-"], .xterm-selection, .xterm-selection *, .cm-selectionBackground, .cm-selectionLayer .cm-selectionBackground, .el-progress-bar__inner, .perf-nav-bar-inner))),
.app-container.has-bg .app-header,
.app-container.has-bg :deep(.app-header *) {
  background-color: transparent !important;
}
/* 排除 xterm 自身着色的单元格与选区层：否则通配透明规则会抹掉 vim 可视模式
   (Ctrl-V/Shift-V)、ls 配色、鼠标选区等由终端渲染的背景色。
   用 :where() 包住，避免 :not 提升特异性把下面对话框/下拉/开关/滑杆
   的不透明例外规则压过去。 */
/* 开背景时 AI 输入(IN)标签底色被抹透明，深色文字会糊在图上；
   改用与输出(OUT)标签一致的白字，保证在背景图上清晰可读 */
.app-container.has-bg .main-content :deep(.in-box .tool-box-label) {
  color: var(--on-accent) !important;
}
/* 标签栏毛玻璃 */
.app-container.has-bg :deep(.app-header) {
  backdrop-filter: blur(0.5rem);
}
/* 对话框、下拉/右键菜单保持不透明背景（覆盖全局 * 透明规则）*/
.app-container.has-bg .main-content :deep(.el-dialog),
.app-container.has-bg .main-content :deep(.el-message-box),
.app-container.has-bg .main-content :deep(.el-select-dropdown),
.app-container.has-bg .main-content :deep(.conn-context-menu),
.app-container.has-bg .main-content :deep(.hash-dropdown),
.app-container.has-bg .main-content :deep(.skill-dropdown) {
  background-color: var(--bg-surface) !important;
}
/* CodeMirror 选区：覆盖 has-bg 通配透明，保证 SQL/语法编辑器选中可见 */
.app-container.has-bg .main-content :deep(.cm-selectionBackground),
.app-container.has-bg .main-content :deep(.cm-focused .cm-selectionBackground) {
  background-color: rgba(64, 158, 255, 0.45) !important;
}
/* Teleport 到 body 的菜单不在 .main-content 内，单独强制不透明 */
body > .conn-context-menu {
  background-color: var(--bg-surface) !important;
  opacity: 1 !important;
  backdrop-filter: none !important;
}
/* 设置左侧选中分类：保留强调色高亮（* 规则会清掉）*/
.app-container.has-bg .main-content :deep(.settings-category.active) {
  background-color: var(--accent-subtle) !important;
}
.app-container.has-bg .main-content :deep(.el-switch__core) {
  background-color: var(--bg-hover) !important;
}
.app-container.has-bg .main-content :deep(.el-switch.is-checked .el-switch__core) {
  background-color: var(--accent) !important;
}
.app-container.has-bg .main-content :deep(.el-switch__core .el-switch__action) {
  background-color: #ffffff !important;
}
.app-container.has-bg .main-content :deep(.el-slider__runway) {
  background-color: var(--bg-hover) !important;
}
.app-container.has-bg .main-content :deep(.el-slider__bar) {
  background-color: var(--accent) !important;
}
.app-container.has-bg .main-content :deep(.el-slider__button) {
  background-color: #ffffff !important;
}
/* 进度条/传输面板：背景图下保持可见（通配透明会把 el-progress 轨道和
   传输面板容器一并抹掉，只剩淡灰小字浮在背景上）。填充色(.el-progress-bar__inner)
   按状态着色，不能一刀切，已在上方 :where 排除列表里整体豁免 */
.app-container.has-bg .main-content :deep(.transfer-panel),
.app-container.has-bg .main-content :deep(.transfer-progress-bar),
.app-container.has-bg .main-content :deep(.el-progress-bar__outer) {
  background-color: var(--bg-elevated) !important;
}
.app-container.has-bg .main-content :deep(.zmodem-transfer-panel) {
  background-color: var(--bg-surface) !important;
}
.app-container.has-bg .main-content :deep(.zmodem-transfer-panel .progress-bar) {
  background-color: var(--bg-hover) !important;
}
.app-container.has-bg .main-content :deep(.zmodem-transfer-panel .progress-fill) {
  background-color: var(--accent) !important;
}
/* K8s 总览用量条：轨道底色是 color-mix、填充按 ok/warn/danger 着色，
   不能一刀切，逐级恢复 */
.app-container.has-bg .main-content :deep(.ov-meter.m-ok) {
  background-color: color-mix(in srgb, var(--el-color-primary) 16%, var(--bg-elevated)) !important;
}
.app-container.has-bg .main-content :deep(.ov-meter.m-warn) {
  background-color: color-mix(in srgb, var(--warning) 16%, var(--bg-elevated)) !important;
}
.app-container.has-bg .main-content :deep(.ov-meter.m-danger) {
  background-color: color-mix(in srgb, var(--el-color-danger) 16%, var(--bg-elevated)) !important;
}
.app-container.has-bg .main-content :deep(.ov-meter-fill.ok) {
  background-color: var(--el-color-primary) !important;
}
.app-container.has-bg .main-content :deep(.ov-meter-fill.warn) {
  background-color: var(--warning) !important;
}
.app-container.has-bg .main-content :deep(.ov-meter-fill.danger) {
  background-color: var(--el-color-danger) !important;
}
/* 容器镜像页拉取/推送进度条 */
.app-container.has-bg .main-content :deep(.task-bar) {
  background-color: var(--bg-hover) !important;
}
.app-container.has-bg .main-content :deep(.task-bar-fill) {
  background-color: var(--accent) !important;
}
/* K8s 总览：可点行 hover（纯毛玻璃，不改底色）与 pod 状态点着色 */
.app-container.has-bg .main-content :deep(.ov-clickable:hover) {
  backdrop-filter: blur(0.875rem);
}
.app-container.has-bg .main-content :deep(.state-dot) {
  background-color: var(--text-muted) !important;
}
.app-container.has-bg .main-content :deep(.state-dot[data-state='running']) {
  background-color: var(--el-color-success) !important;
}
.app-container.has-bg .main-content :deep(.state-dot[data-state='pending']) {
  background-color: var(--warning) !important;
}
.app-container.has-bg .main-content :deep(.state-dot[data-state='failed']) {
  background-color: var(--el-color-danger) !important;
}
/* 树形结构选中项（K8s / 容器 / 数据库 / Mongo）：纯毛玻璃高亮，
   否则背景图下看不出选中了哪一项 */
.app-container.has-bg .main-content :deep(.table-item.selected),
.app-container.has-bg .main-content :deep(.db-header.selected) {
  backdrop-filter: blur(0.875rem);
}
/* 监控-性能左侧导航选中项 */
.app-container.has-bg .main-content :deep(.perf-nav-item.active) {
  backdrop-filter: blur(0.875rem);
}
/* 监控-性能：CPU/磁盘等展开行的百分比条与左侧导航迷你条
   （.perf-nav-bar-inner 的颜色是行内样式，已在上方 :where 排除）*/
.app-container.has-bg .main-content :deep(.sub-bar),
.app-container.has-bg .main-content :deep(.perf-nav-bar) {
  background-color: var(--bg-hover) !important;
}
.app-container.has-bg .main-content :deep(.sub-fill) {
  background: linear-gradient(90deg, var(--accent), var(--accent-glow)) !important;
}
/* 监控边栏：CPU/内存/磁盘百分比条（填充是各资源专属渐变，逐个恢复）*/
.app-container.has-bg .main-content :deep(.mem-bar),
.app-container.has-bg .main-content :deep(.detail-bar) {
  background-color: var(--bg-hover) !important;
}
.app-container.has-bg .main-content :deep(.mem-fill.cpu) {
  background: linear-gradient(90deg, #6366f1, #818cf8) !important;
}
.app-container.has-bg .main-content :deep(.mem-fill.phys) {
  background: linear-gradient(90deg, #3b82f6, #60a5fa) !important;
}
.app-container.has-bg .main-content :deep(.mem-fill.swap) {
  background: linear-gradient(90deg, #14b8a6, #2dd4bf) !important;
}
.app-container.has-bg .main-content :deep(.mem-fill.disk) {
  background: linear-gradient(90deg, #8b5cf6, #a78bfa) !important;
}
.app-container.has-bg .main-content :deep(.detail-fill) {
  background: linear-gradient(90deg, var(--accent), var(--accent-glow)) !important;
}
/* 开背景时，开始页「新建连接」主按钮改用普通按钮样式（透明+blur 与其它按钮一致），
   仅修正文字/边框颜色，避免深色文字糊在图上 */
.app-container.has-bg .main-content :deep(.start-action-btn.primary) {
  border-color: var(--border-subtle) !important;
  color: var(--text-secondary) !important;
}
/* 恢复终端选区高亮（全局透明规则会清掉 xterm 内联选区色）*/
.app-container.has-bg .main-content :deep(.xterm-selection div) {
  background-color: rgba(120, 150, 200, 0.4) !important;
}
/* 终端滚动条（xterm v6 自绘 div）在 style.css 里统一处理：轨道本身已是透明，
   开背景图时无需再开例外。 */
/* el-table（el-scrollbar）横/纵滚动条：滑块是 div，被全局 * 透明规则抹掉了，
   这里恢复半透明滑块，背景图下仍可见。 */
.app-container.has-bg .main-content :deep(.el-scrollbar__thumb) {
  background-color: var(--scrollbar-thumb) !important;
}
/* 表格 body 若走原生滚动条也一并恢复 */
.app-container.has-bg .main-content :deep(.el-table__body-wrapper::-webkit-scrollbar-thumb),
.app-container.has-bg .main-content :deep(.el-scrollbar__wrap::-webkit-scrollbar-thumb) {
  background: var(--scrollbar-thumb) !important;
}
/* 边栏毛玻璃 */
.app-container.has-bg .main-content :deep(.sidebar),
.app-container.has-bg .main-content :deep(.ai-sidebar),
.app-container.has-bg .main-content :deep(.companion-sidebar) {
  backdrop-filter: blur(0.5rem);
}
/* 开始页卡片、按钮毛玻璃 */
.app-container.has-bg .main-content :deep(.start-card),
.app-container.has-bg .main-content :deep(.start-action-btn),
.app-container.has-bg .main-content :deep(.start-action-btn-dropdown-arrow) {
  backdrop-filter: blur(0.5rem);
}
/* 各类输入框毛玻璃 */
.app-container.has-bg .main-content :deep(.el-input__wrapper),
.app-container.has-bg .main-content :deep(.el-textarea__inner),
.app-container.has-bg .main-content :deep(.el-input-number),
.app-container.has-bg .main-content :deep(.el-select__wrapper) {
  backdrop-filter: blur(0.5rem);
}
/* 表格固定列（el-table fixed right/left）：背景图模式下被透明规则抹掉背景，
   固定列会和下方内容重叠 → 用毛玻璃遮住滚动内容（透明底 + blur）。
   旧版用 .el-table__fixed 容器，新版(2.5+)用 sticky 单元格 .el-table-fixed-column--*，两种都覆盖。 */
.app-container.has-bg .main-content :deep(.el-table__fixed-right),
.app-container.has-bg .main-content :deep(.el-table__fixed),
.app-container.has-bg .main-content :deep(.el-table-fixed-column--right),
.app-container.has-bg .main-content :deep(.el-table-fixed-column--left),
.app-container.has-bg .main-content :deep(.k8s-action-cell),
.app-container.has-bg .main-content :deep(.db-action-cell) {
  backdrop-filter: blur(0.875rem);
  pointer-events: auto !important;
  z-index: 3 !important;
}
/* 划出面板（K8sDetailDrawer / MonitorTabContent 等）：背景图模式下同样被透明规则抹掉背景，
   加毛玻璃保证内容在背景图上清晰。 */
.app-container.has-bg .main-content :deep(.detail-drawer) {
  backdrop-filter: blur(0.5rem);
}
/* 拖拽/等待遮罩：全局透明规则会抹掉底色，这里给几处遮罩恢复颜色，
   保证背景图下 SFTP 拖入、终端拖文件、分屏落位提示、loading 蒙层仍清晰可见 */
.app-container.has-bg .main-content :deep(.drop-overlay),
.app-container.has-bg .main-content :deep(.loading-overlay) {
  background-color: var(--scrim) !important;
}
.app-container.has-bg .main-content :deep(.drop-zone-overlay .dz) {
  background-color: var(--accent-subtle) !important;
}
.app-container.has-bg .main-content :deep(.drop-zone-overlay .dz.active) {
  background-color: var(--accent-glow) !important;
}

</style>
