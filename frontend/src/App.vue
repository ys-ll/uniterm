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
    />
    <div class="main-content">
      <Sidebar ref="sidebarRef" :visible="sidebarVisible" @toggle="sidebarVisible = !sidebarVisible" @connect="onConnect" @connect-serial="showSerialDialog = true" @connect-sftp="(c: any) => { const p = tabStore.activeTab; onConnectSftp(c, p?.type === 'start' ? p : undefined) }" @connect-ftp="(c: any) => { const p = tabStore.activeTab; onConnectFtp(c, p?.type === 'start' ? p : undefined) }" @connect-smb="(c: any) => { const p = tabStore.activeTab; onConnectSmb(c, p?.type === 'start' ? p : undefined) }" @connect-webdav="(c: any) => { const p = tabStore.activeTab; onConnectWebdav(c, p?.type === 'start' ? p : undefined) }" @connect-s3="(c: any) => { const p = tabStore.activeTab; onConnectS3(c, p?.type === 'start' ? p : undefined) }" @connect-rdp="(c: any) => { const p = tabStore.activeTab; onConnectRDP(c, p?.type === 'start' ? p : undefined) }" @connect-vnc="(c: any) => { const p = tabStore.activeTab; onConnectVNC(c, p?.type === 'start' ? p : undefined) }" @connect-spice="(c: any) => { const p = tabStore.activeTab; onConnectSPICE(c, p?.type === 'start' ? p : undefined) }" @connect-d-b="(c: any) => { const p = tabStore.activeTab; onConnectDB(c, p?.type === 'start' ? p : undefined) }" @connect-monitor="(c: any) => { const p = tabStore.activeTab; onConnectMonitor(c, p?.type === 'start' ? p : undefined) }" @new-local-terminal-with-shell="createLocalTerminalWithShell" />
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
            <SFTPTabContent
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
            <DBTabContent
              v-else-if="activeTab.type === 'database'"
              :key="activeTab.id"
              :session-id="getPanelSessionId(activeTab.panelId)"
              :host-name="getPanelConfig(activeTab.panelId)?.host || ''"
              :default-db-name="getPanelConfig(activeTab.panelId)?.dbName"
              :db-type="getPanelConfig(activeTab.panelId)?.dbType || ''"
            />
            <RedisTabContent
              v-else-if="activeTab.type === 'redis'"
              :key="activeTab.id"
              :session-id="getPanelSessionId(activeTab.panelId) || ''"
            />
            <MongoDBTabContent
              v-else-if="activeTab.type === 'mongodb'"
              :key="activeTab.id"
              :session-id="getPanelSessionId(activeTab.panelId) || ''"
            />
            <MonitorTabContent
              v-else-if="activeTab.type === 'monitor'"
              :key="activeTab.id"
              :session-id="getPanelSessionId(activeTab.panelId) || ''"
            />
            <StartTabContent
              v-else-if="activeTab.type === 'start'"
              :key="activeTab.id"
              :tab="activeTab"
              @connect="onConnect"
              @new-connection="onNewConnectionFromStart"
              @local-terminal="createLocalTerminalWithShell"
              @connect-serial="(keepOpen?: boolean) => { serialKeepOpen = keepOpen; showSerialDialog = true }"
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
    <ConnectionForm v-model="showConnectionForm" :edit-config="editConfig" :default-group-id="pendingGroupId" @save="onSaveOnly" @connect="(c: ConnectionConfig, ko?: boolean) => { const wasEdit = !!editConfig; editConfig = null; onConnect(c, ko, wasEdit) }" @cancel="editConfig = null" />
    <SerialConnectDialog v-model="showSerialDialog" @connect="(sid: string, portName: string, baudRate: number) => onConnectSerial(sid, portName, baudRate, serialKeepOpen)" />
    <CredentialPrompt
      v-model:visible="credentialVisible"
      :title="credentialTitle"
      :subtitle="credentialSubtitle"
      :fields="credentialFields"
      :initial-user="credentialInitialUser"
      :initial-password="credentialInitialPassword"
      @resolve="onCredentialResolve"
    />

    <!-- Input context menu -->
    <div
      v-show="inputMenuVisible"
      class="input-context-menu"
      :style="{ left: inputMenuPos.x + 'px', top: inputMenuPos.y + 'px' }"
      @click.stop
    >
      <div v-if="!inputMenuReadonly" class="input-menu-item" @click="inputMenuCut">{{ t('input.cut') }}</div>
      <div class="input-menu-item" @click="inputMenuCopy">{{ t('input.copy') }}</div>
      <div v-if="!inputMenuReadonly" class="input-menu-item" @click="inputMenuPaste">{{ t('input.paste') }}</div>
      <div class="input-menu-item" @click="inputMenuSelectAll">{{ t('input.selectAll') }}</div>
    </div>

    <SyncConflictDialog />
  </div>
  </el-config-provider>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted, provide, h } from 'vue'
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
import SettingsTabContent from './components/SettingsTabContent.vue'
import WorkspaceContent from './components/WorkspaceContent.vue'
import SFTPTabContent from './components/SFTPTabContent.vue'
import RDPTabContent from './components/RDPTabContent.vue'
import VNCTabContent from './components/VNCTabContent.vue'
import SPICETabContent from './components/SPICETabContent.vue'
import DBTabContent from './components/DBTabContent.vue'
import RedisTabContent from './components/RedisTabContent.vue'
import MongoDBTabContent from './components/MongoDBTabContent.vue'
import MonitorTabContent from './components/MonitorTabContent.vue'
import StartTabContent from './components/StartTabContent.vue'
import ConnectionForm from './components/ConnectionForm.vue'
import AISidebar from './components/AISidebar.vue'
import SyncConflictDialog from './components/SyncConflictDialog.vue'
import SerialConnectDialog from './components/SerialConnectDialog.vue'
import CredentialPrompt from './components/CredentialPrompt.vue'
import type { CredentialResult } from './components/CredentialPrompt.vue'
import { ElMessageBox, ElCheckbox } from 'element-plus'
import { useConnectionStore } from './stores/connectionStore'
import { useTabStore } from './stores/tabStore'
import { usePanelStore } from './stores/panelStore'
import { useSessionStore } from './stores/sessionStore'
import { useAIStore } from './stores/aiStore'
import { useSettingsStore } from './stores/settingsStore'
import { useQuickCommandStore } from './stores/quickCommandStore'
import { useTunnelStore } from './stores/tunnelStore'
import { useLocalStateStore } from './stores/localStateStore'
import { useUpdateCheck } from './composables/useUpdateCheck'
import { loadKeybindings, installGlobalListener, uninstallGlobalListener } from './composables/useKeyboardShortcuts'
import { focusPanelTerminal, installTerminalFocusRestore } from './composables/useFocusTerminal'
import type { ShortcutAction } from './types/settings'
import { useI18n } from './i18n'
import { CreateSession, CloseSession, RDPHide, RDPShow, RDPSetPosition, RDPSetFocus, RecordRecentConnection, GetPlatform, GetBackgroundImage } from '../wailsjs/go/main/App'
import { EventsOn, ClipboardGetText, Quit } from '../wailsjs/runtime'
import { msg } from './services/message'
import type { ConnectionConfig } from './types/session'
import { parseQuickConnect } from './utils/quickConnect'

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
const aiStore = useAIStore()
const settingsStore = useSettingsStore()
const localStateStore = useLocalStateStore()
const updateCheck = useUpdateCheck()
let uninstallFocusRestore: (() => void) | null = null
const { t, locale } = useI18n()
const EL_LOCALE_MAP: Record<string, typeof enUs> = {
  'zh-CN': zhCn, 'zh-TW': zhTw, en: enUs, ja, ko, de, es, fr, ru,
}
const elLocale = computed(() => EL_LOCALE_MAP[locale.value] || enUs)
// ── RDP position sync ──
// Called explicitly on tab switch and overlay restore; no polling needed.

function getActiveRdpSessionId(): string | null {
  const tab = activeTab.value
  if (!tab || tab.type !== 'rdp') return null
  return panelStore.getPanel(tab.panelId)?.sessionId ?? null
}

function rdpSyncPosition() {
  if (rdpOverlayCount.value > 0) return
  const area = document.querySelector('.rdp-area') as HTMLElement | null
  if (!area) return
  const sid = getActiveRdpSessionId()
  if (!sid) return
  const rect = area.getBoundingClientRect()
  if (rect.width <= 0) return
  const dpr = window.devicePixelRatio || 1
  const sx = window.screenLeft ?? (window as any).screenX ?? 0
  const sy = window.screenTop ?? (window as any).screenY ?? 0
  const x = Math.round((sx + rect.left) * dpr)
  const y = Math.round((sy + rect.top) * dpr)

  const w = Math.round(rect.width * dpr)
  const h = Math.round(rect.height * dpr)
  RDPSetPosition(sid, x, y, w, h)
}

function rdpResetTracking() {
  nextTick(() => rdpSyncPosition())
}


// ── RDP overlay tracking: unified show/hide entry points ──
// ALL triggers (context menus, dialogs, drag, resize, external events)
// MUST call RDPHideForOverlay() to hide and RDPShowForOverlay() to restore.
// Reference-counted: nesting works correctly across multiple concurrent triggers.
const rdpOverlayCount = ref(0)
let rdpRestoreTimer: ReturnType<typeof setTimeout> | null = null

function RDPHideForOverlay() {
	rdpOverlayCount.value++
	if (rdpOverlayCount.value === 1) {
		const sid = getActiveRdpSessionId()
		if (sid) RDPHide(sid)
	}
}

function RDPShowForOverlay() {
	if (rdpOverlayCount.value > 0) rdpOverlayCount.value--
	if (rdpRestoreTimer) clearTimeout(rdpRestoreTimer)
	rdpRestoreTimer = setTimeout(() => {
		rdpRestoreTimer = null
		if (rdpOverlayCount.value === 0) {
			const tab = activeTab.value
			if (!tab || tab.type !== 'rdp') return
			const sid = panelStore.getPanel(tab.panelId)?.sessionId
			if (sid) {
				rdpResetTracking()
				nextTick(() => RDPShow(sid))
			}
		}
	}, 150)
}


const showConnectionForm = ref(false)
const showSerialDialog = ref(false)
const serialKeepOpen = ref(false)
const sidebarVisible = ref(false)
const sidebarRef = ref<any>(null)
const aiSidebarRef = ref<any>(null)


// Input context menu state
const inputMenuVisible = ref(false)
const inputMenuPos = ref({ x: 0, y: 0 })
const inputMenuReadonly = ref(false)

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

function needsCredentialCheck(config: ConnectionConfig): boolean {
  const inScope = ['ssh', 'mosh', 'sftp', 'ftp'].includes(config.type)
  if (!inScope) return false
  if ((config.type === 'ssh' || config.type === 'mosh') && config.authType === 'key') return false
  return !config.user || !config.password
}

function getMissingFields(config: ConnectionConfig): ('user' | 'password')[] {
  const fields: ('user' | 'password')[] = []
  if (!config.user) fields.push('user')
  if (!config.password) fields.push('password')
  return fields
}

async function ensureCredentials(config: ConnectionConfig): Promise<ConnectionConfig | null> {
  // 1. Check SSH tunnel connection first
  if (config.tunnelSSHConnId) {
    const tunnelConn = connectionStore.connections.find(c => c.id === config.tunnelSSHConnId)
    if (tunnelConn && needsCredentialCheck(tunnelConn)) {
      const result = await showCredentialDialog(
        t('credential.tunnelTitle'),
        t('credential.tunnelSubtitle', { name: tunnelConn.name }),
        ['user', 'password'],
        tunnelConn.user,
        tunnelConn.password
      )
      if (!result) return null
      // Pass credentials inline so Go can apply them without reading the store
      config.tunnelSSHUser = result.user || tunnelConn.user
      config.tunnelSSHPassword = result.password || tunnelConn.password
      if (result.action === 'save_and_connect') {
        await connectionStore.update(tunnelConn.id, {
          user: config.tunnelSSHUser,
          password: config.tunnelSSHPassword
        })
      }
    }
  }

  // 2. Check main connection
  if (!needsCredentialCheck(config)) return config

  const result = await showCredentialDialog(
    t('credential.title'),
    '',
    ['user', 'password'],
    config.user,
    config.password
  )
  if (!result) return null
  // Create new object instead of mutating the original (which may be
  // referenced by the Pinia store). For "save_and_connect" we explicitly
  // persist via connectionStore.update below.
  config = {
    ...config,
    user: result.user || config.user,
    password: result.password || config.password
  }
  if (result.action === 'save_and_connect') {
    await connectionStore.update(config.id, { user: config.user, password: config.password })
  }
  return config
}

let inputMenuTarget: HTMLInputElement | HTMLTextAreaElement | HTMLElement | null = null

function closeInputMenu() {
  inputMenuVisible.value = false
  inputMenuTarget = null
}

function onInputContextMenu(e: Event) {
  const { x, y, target, readonly } = (e as CustomEvent).detail as {
    x: number; y: number; target: HTMLElement; readonly?: boolean
  }
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  inputMenuTarget = target
  inputMenuReadonly.value = !!readonly
  const pos = fitMenuPosition(x, y, 120, 140)
  inputMenuPos.value = { x: parseInt(pos.left), y: parseInt(pos.top) }
  inputMenuVisible.value = true
}

function fitMenuPosition(x: number, y: number, menuW: number, menuH: number) {
  let left = x
  let top = y
  if (x + menuW > window.innerWidth) left = x - menuW
  if (y + menuH > window.innerHeight) top = y - menuH
  return { left: left + 'px', top: top + 'px' }
}

function inputMenuCut() {
  const el = inputMenuTarget
  closeInputMenu()
  if (!el) return
  const sel = getInputSelection(el)
  navigator.clipboard.writeText(sel)
  if (el.isContentEditable) {
    const s = window.getSelection()
    if (s && s.rangeCount > 0) { s.getRangeAt(0).deleteContents() }
  } else {
    setInputSelection(el as HTMLInputElement | HTMLTextAreaElement, '')
  }
  el.dispatchEvent(new Event('input', { bubbles: true }))
}

function inputMenuCopy() {
  const el = inputMenuTarget
  const readonly = inputMenuReadonly.value
  closeInputMenu()
  if (!el) return
  // Read-only plain element (log-path toast): copy the live selection if any,
  // otherwise the whole text.
  if (readonly) {
    const sel = window.getSelection()?.toString()
    navigator.clipboard.writeText(sel || el.textContent || '')
    return
  }
  navigator.clipboard.writeText(getInputSelection(el))
}

function inputMenuPaste() {
  const el = inputMenuTarget
  closeInputMenu()
  if (!el) return
  ClipboardGetText().then(text => {
    if (el.isContentEditable) {
      insertTextAtContentEditable(el, text)
    } else {
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

function getInputSelection(el: HTMLElement): string {
  if (el.isContentEditable) {
    return window.getSelection()?.toString() || ''
  }
  const input = el as HTMLInputElement | HTMLTextAreaElement
  return input.value.substring(input.selectionStart ?? 0, input.selectionEnd ?? 0)
}

function setInputSelection(el: HTMLInputElement | HTMLTextAreaElement, text: string) {
  const start = el.selectionStart ?? 0
  const end = el.selectionEnd ?? 0
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

function onWheel(e: WheelEvent) {
  if (e.ctrlKey) {
    e.preventDefault()
    const ts = settingsStore.settings.terminal
    const delta = e.deltaY < 0 ? 1 : -1
    const next = Math.max(8, Math.min(32, ts.fontSize + delta))
    if (next !== ts.fontSize) {
      ts.fontSize = next
      settingsStore.save()
    }
  }
}

// WKWebView doesn't forward Cmd/Ctrl+A/C/V on input/textarea/contenteditable.
function onEditShortcut(e: KeyboardEvent) {
  if (e.defaultPrevented) return
  const target = e.target as HTMLElement
  const tag = target.tagName
  const isEditable = tag === 'INPUT' || tag === 'TEXTAREA' || target.isContentEditable
  if (!isEditable) return
  const mod = e.metaKey || e.ctrlKey
  if (!mod || e.shiftKey || e.altKey) return

  if (e.key === 'a' || e.key === 'A') {
    e.preventDefault()
    if (target.isContentEditable) {
      const el = target
      const range = document.createRange()
      range.selectNodeContents(el)
      const sel = window.getSelection()
      sel?.removeAllRanges()
      sel?.addRange(range)
    } else {
      (target as HTMLInputElement | HTMLTextAreaElement).select()
    }
    return
  }

  if (e.key === 'c' || e.key === 'C') {
    e.preventDefault()
    const sel = getInputSelection(target)
    navigator.clipboard.writeText(sel).catch(() => {})
    return
  }

  if (e.key === 'v' || e.key === 'V') {
    e.preventDefault()
    ClipboardGetText().then(text => {
      if (target.isContentEditable) {
        insertTextAtContentEditable(target, text)
      } else {
        setInputSelection(target as HTMLInputElement | HTMLTextAreaElement, text)
      }
      target.dispatchEvent(new Event('input', { bubbles: true }))
    }).catch(() => {})
  }
}

// macOS-only system shortcuts (issue #339): Cmd+Q quits, Cmd+W closes the
// active tab. Guarded by isMac so Windows/Linux never see this behaviour —
// there Ctrl+Q/W stay free for the terminal and the existing keybindings.
let isMac = false
function onMacSystemShortcut(e: KeyboardEvent) {
  if (!isMac || e.defaultPrevented) return
  if (!e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) return
  const key = e.key.toLowerCase()
  if (key === 'q') {
    e.preventDefault()
    Quit()
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
  sidebarVisible.value = localStateStore.state.sidebarVisible ?? false
  // Pre-load quick commands so suggestions can read them immediately
  useQuickCommandStore().load()
  // Pre-load tunnels so auto-start state and the panel are ready
  useTunnelStore().load()
  // Auto-open start tab if no tabs are open
  if (tabStore.tabs.length === 0) {
    tabStore.createStartTab()
  }
  // Pre-load noVNC so VNC tab switches don't pay the dynamic import cost.
  import('@novnc/novnc').then((m: any) => {
    ;(window as any).__novnc_RFB = m.default || m
  }).catch(() => {})
  window.addEventListener('input:contextmenu', onInputContextMenu)
  window.addEventListener('global:close-context-menus', closeInputMenu)
  document.addEventListener('click', closeInputMenu)
  document.addEventListener('wheel', onWheel, { passive: false })
  // WKWebView doesn't forward Cmd+A/C/V on input/textarea/contenteditable — handle globally.
  document.addEventListener('keydown', onEditShortcut)
  // macOS system shortcuts (Cmd+Q / Cmd+W) — only armed on darwin.
  try { isMac = (await GetPlatform()) === 'darwin' } catch { isMac = false }
  if (isMac) document.addEventListener('keydown', onMacSystemShortcut, true)
  // Keyboard shortcuts — load once on mount, watch for settings changes
  applyKeybindings()
  installGlobalListener()

  // Restore terminal focus after window drags and sidebar scrollbar clicks
  // (issue #285) — see composables/useFocusTerminal.ts for the policy.
  uninstallFocusRestore = installTerminalFocusRestore()

  // RDP blur/focus: notify Go side so it can manage focus on the native RDP window
  window.addEventListener('blur', () => {
    const sid = getActiveRdpSessionId()
    if (sid) RDPSetFocus(sid, false)
  })
  window.addEventListener('focus', () => {
    const sid = getActiveRdpSessionId()
    if (sid) RDPSetFocus(sid, true)
  })
  // RDP overlay tracking
  window.addEventListener('rdp:overlay-push', RDPHideForOverlay)
  window.addEventListener('rdp:overlay-pop', RDPShowForOverlay)
  window.addEventListener('split:resize-start', RDPHideForOverlay)
  window.addEventListener('split:resize-end', RDPShowForOverlay)
  window.addEventListener('rdp:sync-position', rdpResetTracking)
  // Go-side WndProc events: window move/resize start/end
  EventsOn('rdp:move-resize-start', () => RDPHideForOverlay())
  EventsOn('rdp:move-resize-end', () => RDPShowForOverlay())

  // Panel/Tab/StartTab menu actions
  window.addEventListener('app:connect-sftp', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; onConnectSftp(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-monitor', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; onConnectMonitor(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-rdp', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; onConnectRDP(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-vnc', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; onConnectVNC(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-spice', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; onConnectSPICE(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-db', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; onConnectDB(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-ftp', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; onConnectFtp(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-smb', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; onConnectSmb(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-webdav', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; onConnectWebdav(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)
  window.addEventListener('app:connect-s3', ((e: CustomEvent) => {
    const d = e.detail; const c = d?.config || d; if (c) { const prev = tabStore.activeTab; onConnectS3(c, prev?.type === 'start' ? prev : undefined) }
  }) as EventListener)

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

// ── Keyboard shortcut handlers (module-level so watch can access them) ──
const actionHandlers: Record<ShortcutAction, () => void> = {
  nextTab: () => tabStore.nextTab(),
  prevTab: () => tabStore.prevTab(),
  newConnection: () => { tabStore.createStartTab() },
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
  focusTerminal: () => {
    const pid = tabStore.getActivePanelId()
    if (pid) focusPanelTerminal(pid)
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
      tabStore.removePanelFromWorkspaceTab(t.id, panelId)
    } else if (t.type === 'workspace' && t.panelIds.length === 1) {
      tabStore.removePanelFromWorkspaceTab(t.id, t.panelIds[0])
    } else {
      closeTab(t.id)
    }
  },
  terminalSearch: () => {
    const pid = tabStore.getActivePanelId()
    if (pid) window.dispatchEvent(new CustomEvent('terminal:open-search', { detail: { panelId: pid } }))
  },
  navigatePrev: () => navigatePanel(-1),
  navigateNext: () => navigatePanel(1),
  openSettings: () => openSettings(),
  duplicateSession: async () => {
    const pid = tabStore.getActivePanelId()
    if (!pid) return
    const panel = panelStore.getPanel(pid)
    if (!panel?.config) return
    const newPanel = panelStore.createPanel(
      { ...panel.config } as ConnectionConfig,
      panel.type
    )
    panelStore.updateTitle(newPanel.id, panel.title)
    try {
      const info = await CreateSession(panel.config.type, panel.config)
      panelStore.bindSession(newPanel.id, info.id)
      const newTab = tabStore.createTerminalTab(newPanel.title, newPanel.id)
      panelStore.movePanelToTab(newPanel.id, newTab.id)
    } catch (e) {
      console.error('Failed to duplicate session:', e)
    }
  },
}

function applyKeybindings() {
  loadKeybindings(settingsStore.settings.keyboard, actionHandlers)
}

onUnmounted(() => {
  uninstallGlobalListener()
  uninstallFocusRestore?.()
  window.removeEventListener('input:contextmenu', onInputContextMenu)
  window.removeEventListener('global:close-context-menus', closeInputMenu)
  document.removeEventListener('click', closeInputMenu)
  document.removeEventListener('wheel', onWheel)
  document.removeEventListener('keydown', onMacSystemShortcut, true)
  // RDP overlay tracking
  window.removeEventListener('rdp:overlay-push', RDPHideForOverlay)
  window.removeEventListener('rdp:overlay-pop', RDPShowForOverlay)
  window.removeEventListener('split:resize-start', RDPHideForOverlay)
  window.removeEventListener('split:resize-end', RDPShowForOverlay)
  window.removeEventListener('rdp:sync-position', rdpResetTracking)

})

function openSettings() {
  // Check if settings tab already exists
  const existingTab = tabStore.tabs.find(t => t.type === 'settings')
  if (existingTab) {
    tabStore.setActiveTab(existingTab.id)
    return
  }

  const panel = panelStore.createPanel(null, 'settings')
  panelStore.updateTitle(panel.id, t('settings.title'))
  const tab = tabStore.createSettingsTab(t('settings.title'), panel.id)
  panelStore.movePanelToTab(panel.id, tab.id)
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
            h('div', { style: 'display:flex;flex-direction:column;gap:10px' }, [
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
  if (tab && (tab.type === 'redis' || tab.type === 'mongodb')) {
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
  // Terminal sessions must be explicitly closed to terminate the connection/shell process
  if (tab && tab.type === 'terminal') {
    const p = panelStore.getPanel(tab.panelId)
    if (p?.sessionId) {
      try { await CloseSession(p.sessionId) } catch (_) {}
    }
  }
  const panelIds = tabStore.closeTab(tabId)
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
        h('div', { style: 'display:flex;flex-direction:column;gap:10px' }, [
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

function onSaveOnly(config: ConnectionConfig) {
  if (editConfig.value) {
    connectionStore.update(config.id, config)
  } else {
    connectionStore.add(config)
  }
  RecordRecentConnection(config.id)
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

async function onConnect(config: ConnectionConfig, keepOpen?: boolean, wasEdit?: boolean) {
  const prev = tabStore.activeTab
  const prevStart = (prev?.type === 'start' && !keepOpen) ? prev : undefined
  // Persist form changes BEFORE dispatching by type. The type-specific
  // handlers below only call connectionStore.add(), which is a silent
  // no-op for existing ids and would otherwise drop edits made in the
  // "Save & Connect" flow.
  if (wasEdit) {
    connectionStore.update(config.id, config)
  } else {
    connectionStore.add(config)
  }
  if (config.type === 'ftp') { await onConnectFtp(config, prevStart); return }
  if (config.type === 'smb') { await onConnectSmb(config, prevStart); return }
  if (config.type === 'webdav') { await onConnectWebdav(config, prevStart); return }
  if (config.type === 's3') { await onConnectS3(config, prevStart); return }
  if (config.type === 'rdp') { await onConnectRDP(config, prevStart); return }
  if (config.type === 'vnc') { await onConnectVNC(config, prevStart); return }
  if (config.type === 'spice') { await onConnectSPICE(config, prevStart); return }
  if (config.type === 'database') { await onConnectDB(config, prevStart); return }

  // Credential check
  const resolved = await ensureCredentials(config)
  if (!resolved) return
  config = resolved

  // Create session BEFORE panel so the terminal has a sessionId when it first
  // fires SessionResize. Otherwise the resize is silently dropped because the
  // terminal calls getSessionId() too early and never retries.
  let sessionId = ''
  try {
    const info = await CreateSession(config.type, config)
    sessionId = info.id
  } catch (e) {
    console.error('Failed to create session:', e)
    return
  }

  const panel = panelStore.createPanel(config, config.type)
  const displayTitle = config.name || (config.type === 'local'
    ? getShellLabel(config.shellPath)
    : config.type === 'serial'
    ? `${config.serialPort || 'Serial'} (${config.serialBaudRate || 115200})`
    : config.type === 'telnet'
    ? `${config.host}:${config.port}`
    : `${config.user}@${config.host}`)
  panelStore.updateTitle(panel.id, displayTitle)
  panelStore.bindSession(panel.id, sessionId)
  sessionStore.initSession(sessionId)
  const tab = prev?.type === 'start'
    ? tabStore.replaceStartTab(prev.id, panel.title, panel.id)
    : tabStore.createTerminalTab(panel.title, panel.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)
}

function getShellLabel(path: string): string {
  if (!path) return 'Local'
  const lower = path.toLowerCase()
  if (lower.startsWith('wsl://')) {
    const distro = path.slice(6)
    return distro ? `WSL - ${distro}` : 'WSL'
  }
  if (lower.includes('pwsh')) return 'PowerShell'
  if (lower.includes('powershell')) return 'Windows PowerShell'
  if (lower.includes('bash')) return 'Git Bash'
  if (lower.includes('cmd')) return 'Command Prompt'
  return path.replace(/\\/g, '/').split('/').pop() || 'Local'
}

async function createLocalTerminalWithShell(shellPath: string, keepOpen?: boolean) {
  await createLocalTerminal(shellPath, keepOpen)
}

const pendingGroupId = ref<string | undefined>(undefined)

function onNewConnectionFromStart(payload?: { host?: string; groupId?: string }) {
  pendingGroupId.value = payload?.groupId
  if (payload?.host) {
    const parsed = parseQuickConnect(payload.host)
    editConfig.value = (parsed || { host: payload.host }) as ConnectionConfig
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
      shellPath: shellPath || undefined
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
  } catch (e) {
    console.error('Failed to create local terminal:', e)
    panelStore.removePanel(panel.id)
  }
}

async function onConnectSftp(config: ConnectionConfig, prevStart?: any) {
  connectionStore.add(config)

  const resolved = await ensureCredentials(config)
  if (!resolved) return
  config = resolved

  const panel = panelStore.createPanel(config, 'sftp')
  const displayTitle = config.name || `${config.user}@${config.host}`
  panelStore.updateTitle(panel.id, displayTitle)
  const reposition = prevStart ? closeStartAndReposition(prevStart) : null
  const tab = tabStore.createSFPTab(displayTitle, panel.id)
  if (reposition) reposition(tab.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)

  try {
    const info = await CreateSession('sftp', config)
    panelStore.bindSession(panel.id, info.id)
  } catch (e) {
    console.error('Failed to create SFTP session:', e)
    tabStore.closeTab(tab.id)
    panelStore.removePanel(panel.id)
  }
}

async function onConnectFtp(config: ConnectionConfig, prevStart?: any) {
  connectionStore.add(config)

  const resolved = await ensureCredentials(config)
  if (!resolved) return
  config = resolved
  const panel = panelStore.createPanel(config, 'sftp')
  const displayTitle = config.name || `${config.user}@${config.host}`
  panelStore.updateTitle(panel.id, displayTitle)
  const reposition = prevStart ? closeStartAndReposition(prevStart) : null
  const tab = tabStore.createFtpTab(displayTitle, panel.id)
  if (reposition) reposition(tab.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)

  try {
    const info = await CreateSession('ftp', config)
    panelStore.bindSession(panel.id, info.id)
  } catch (e) {
    console.error('Failed to create FTP session:', e)
    tabStore.closeTab(tab.id)
    panelStore.removePanel(panel.id)
  }
}

async function onConnectSmb(config: ConnectionConfig, prevStart?: any) {
  connectionStore.add(config)

  const resolved = await ensureCredentials(config)
  if (!resolved) return
  config = resolved
  const panel = panelStore.createPanel(config, 'sftp')
  const displayTitle = config.name || `${config.user}@${config.host}`
  panelStore.updateTitle(panel.id, displayTitle)
  const reposition = prevStart ? closeStartAndReposition(prevStart) : null
  const tab = tabStore.createFtpTab(displayTitle, panel.id)
  if (reposition) reposition(tab.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)

  try {
    const info = await CreateSession('smb', config)
    panelStore.bindSession(panel.id, info.id)
  } catch (e) {
    console.error('Failed to create SMB session:', e)
    tabStore.closeTab(tab.id)
    panelStore.removePanel(panel.id)
  }
}

async function onConnectWebdav(config: ConnectionConfig, prevStart?: any) {
  connectionStore.add(config)

  const resolved = await ensureCredentials(config)
  if (!resolved) return
  config = resolved
  const panel = panelStore.createPanel(config, 'sftp')
  const displayTitle = config.name || `${config.user}@${config.host}`
  panelStore.updateTitle(panel.id, displayTitle)
  const reposition = prevStart ? closeStartAndReposition(prevStart) : null
  const tab = tabStore.createFtpTab(displayTitle, panel.id)
  if (reposition) reposition(tab.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)

  try {
    const info = await CreateSession('webdav', config)
    panelStore.bindSession(panel.id, info.id)
  } catch (e) {
    console.error('Failed to create WebDAV session:', e)
    tabStore.closeTab(tab.id)
    panelStore.removePanel(panel.id)
  }
}

async function onConnectS3(config: ConnectionConfig, prevStart?: any) {
  connectionStore.add(config)

  const panel = panelStore.createPanel(config, 'sftp')
  const displayTitle = config.name || (config.s3Bucket ? `s3://${config.s3Bucket}` : config.host)
  panelStore.updateTitle(panel.id, displayTitle)
  const reposition = prevStart ? closeStartAndReposition(prevStart) : null
  const tab = tabStore.createFtpTab(displayTitle, panel.id)
  if (reposition) reposition(tab.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)

  try {
    const info = await CreateSession('s3', config)
    panelStore.bindSession(panel.id, info.id)
  } catch (e) {
    console.error('Failed to create S3 session:', e)
    tabStore.closeTab(tab.id)
    panelStore.removePanel(panel.id)
  }
}

async function onConnectRDP(config: ConnectionConfig, prevStart?: any) {
  connectionStore.add(config)

  const resolved = await ensureCredentials(config)
  if (!resolved) return
  config = resolved

  const displayTitle = config.name || `${config.user}@${config.host}`

  const panel = panelStore.createPanel(config, 'rdp')
  panelStore.updateTitle(panel.id, displayTitle)
  const reposition = prevStart ? closeStartAndReposition(prevStart) : null
  const tab = tabStore.createRDPTab(displayTitle, panel.id)
  if (reposition) reposition(tab.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)

  try {
    const info = await CreateSession('rdp', config)
    panelStore.bindSession(panel.id, info.id)
    sessionStore.initSession(info.id)
  } catch (e) {
    console.error('Failed to create RDP session:', e)
    tabStore.closeTab(tab.id)
    panelStore.removePanel(panel.id)
  }
}

async function onConnectVNC(config: ConnectionConfig, prevStart?: any) {
  connectionStore.add(config)

  const resolved = await ensureCredentials(config)
  if (!resolved) return
  config = resolved

  const displayTitle = config.name || config.host

  const panel = panelStore.createPanel(config, 'vnc')
  panelStore.updateTitle(panel.id, displayTitle)
  const reposition = prevStart ? closeStartAndReposition(prevStart) : null
  const tab = tabStore.createVNCTab(displayTitle, panel.id)
  if (reposition) reposition(tab.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)

  try {
    const info = await CreateSession('vnc', config)
    panelStore.bindSession(panel.id, info.id)
    sessionStore.initSession(info.id)
  } catch (e) {
    console.error('Failed to create VNC session:', e)
    tabStore.closeTab(tab.id)
    panelStore.removePanel(panel.id)
  }
}

async function onConnectSPICE(config: ConnectionConfig, prevStart?: any) {
  connectionStore.add(config)

  const resolved = await ensureCredentials(config)
  if (!resolved) return
  config = resolved

  const displayTitle = config.name || config.host

  const panel = panelStore.createPanel(config, 'spice')
  panelStore.updateTitle(panel.id, displayTitle)
  const reposition = prevStart ? closeStartAndReposition(prevStart) : null
  const tab = tabStore.createSPICETab(displayTitle, panel.id)
  if (reposition) reposition(tab.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)

  try {
    const info = await CreateSession('spice', config)
    panelStore.bindSession(panel.id, info.id)
    sessionStore.initSession(info.id)
  } catch (e) {
    console.error('Failed to create SPICE session:', e)
    tabStore.closeTab(tab.id)
    panelStore.removePanel(panel.id)
  }
}

async function onConnectMonitor(config: ConnectionConfig, prevStart?: any) {
  connectionStore.add(config)

  const resolved = await ensureCredentials(config)
  if (!resolved) return
  config = resolved

  const panel = panelStore.createPanel(config, 'monitor')
  const displayTitle = config.name || `${config.user}@${config.host}`
  panelStore.updateTitle(panel.id, displayTitle)
  const reposition = prevStart ? closeStartAndReposition(prevStart) : null
  const tab = tabStore.createMonitorTab(displayTitle, panel.id)
  if (reposition) reposition(tab.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)

  try {
    const info = await CreateSession('monitor', config)
    panelStore.bindSession(panel.id, info.id)
    sessionStore.initSession(info.id)
  } catch (e) {
    console.error('Failed to create monitor session:', e)
    tabStore.closeTab(tab.id)
    panelStore.removePanel(panel.id)
  }
}

async function onConnectDB(config: ConnectionConfig, prevStart?: any) {
  connectionStore.add(config)

  const resolved = await ensureCredentials(config)
  if (!resolved) return
  config = resolved

  if (!config.dbType) {
    config.dbType = 'mysql'
  }
  const displayTitle = config.name || `${config.dbType}:${config.user}@${config.host}`

  const panel = panelStore.createPanel(config, 'database')
  panelStore.updateTitle(panel.id, displayTitle)
  const reposition = prevStart ? closeStartAndReposition(prevStart) : null
  const tab = tabStore.createDBTab(displayTitle, panel.id)
  if (reposition) reposition(tab.id)
  if (config.dbType === 'redis') {
    tab.type = 'redis'
  } else if (config.dbType === 'mongodb') {
    tab.type = 'mongodb'
  }
  panelStore.movePanelToTab(panel.id, tab.id)
  RecordRecentConnection(config.id)

  try {
    let sessionType: string
    if (config.dbType === 'redis') {
      sessionType = 'redis'
    } else if (config.dbType === 'mongodb') {
      sessionType = 'mongodb'
    } else {
      sessionType = 'database'
    }
    const info = await CreateSession(sessionType, config)
    panelStore.bindSession(panel.id, info.id)
    sessionStore.initSession(info.id)
  } catch (e: any) {
    const errMsg = e?.message || String(e)
    console.error('Failed to create database session:', errMsg)
    panelStore.updateStatus(panel.id, 'error')
    msg.error(`${t('db.connectFailed')}: ${errMsg}`)
  }
}

async function onConnectSerial(sessionId: string, portName: string, baudRate: number, keepOpen?: boolean) {
  const config: ConnectionConfig = {
    id: '',
    name: `${portName} (${baudRate})`,
    type: 'serial' as any,
    host: portName,
    port: baudRate,
    user: '',
    authType: 'password' as any,
  }
  const panel = panelStore.createPanel(config, 'serial')
  panelStore.updateTitle(panel.id, `${portName} (${baudRate})`)
  panelStore.bindSession(panel.id, sessionId)
  sessionStore.initSession(sessionId)
  const prev = tabStore.activeTab
  const tab = prev?.type === 'start' && !keepOpen
    ? tabStore.replaceStartTab(prev.id, panel.title, panel.id)
    : tabStore.createTerminalTab(panel.title, panel.id)
  panelStore.movePanelToTab(panel.id, tab.id)
  // Serial connections are temporary — don't record in history
}

// Show/hide native RDP window on tab switch.
// Position updates are only sent to the active RDP session (see rdpSyncPosition),
// so background sessions stay at (32000,32000) and don't respond to drag.
watch(() => activeTab.value, (newTab, oldTab) => {
  if (oldTab?.type === 'rdp') {
    const p = panelStore.getPanel(oldTab.panelId)
    if (p?.sessionId) RDPHide(p.sessionId)
  }
  // Clear pending restore timer on tab switch
  if (rdpRestoreTimer) { clearTimeout(rdpRestoreTimer); rdpRestoreTimer = null }
  if (newTab?.type === 'rdp') {
    rdpResetTracking()
    const sid = panelStore.getPanel(newTab.panelId)?.sessionId
    if (sid) nextTick(() => RDPShow(sid))
  }
  // Auto-focus terminal on tab switch (including new connections)
  if (newTab) {
    const pid = tabStore.getActivePanelId()
    if (pid) nextTick(() => focusPanelTerminal(pid))
  }
})

// Hide RDP when new-connection dialog opens (App.vue's ConnectionForm)
watch(showConnectionForm, (val) => {
  if (val) RDPHideForOverlay()
  else RDPShowForOverlay()
})

watch(sidebarVisible, async () => {
  RDPHideForOverlay()
  nextTick(() => RDPShowForOverlay())
  localStateStore.update({ sidebarVisible: sidebarVisible.value })
})

watch(() => aiStore.visible, () => {
  RDPHideForOverlay()
  nextTick(() => RDPShowForOverlay())
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
  padding: 3px;
}

.input-context-menu {
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

.input-menu-item {
  padding: 7px 14px;
  font-size: 12px;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  transition: all 0.1s ease;
}

.input-menu-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.group-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.group-list .group-item {
  padding: 10px 14px;
  border-radius: 6px;
  cursor: pointer;
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
.app-container.has-bg .main-content :deep(*),
.app-container.has-bg .app-header,
.app-container.has-bg :deep(.app-header *) {
  background-color: transparent !important;
}
/* 标签栏毛玻璃 */
.app-container.has-bg :deep(.app-header) {
  backdrop-filter: blur(8px);
}
/* 对话框、下拉/右键菜单保持不透明背景（覆盖全局 * 透明规则）*/
.app-container.has-bg .main-content :deep(.el-dialog),
.app-container.has-bg .main-content :deep(.el-message-box),
.app-container.has-bg .main-content :deep(.el-select-dropdown),
.app-container.has-bg .main-content :deep(.el-dropdown-menu),
.app-container.has-bg .main-content :deep(.context-menu),
.app-container.has-bg .main-content :deep(.ai-context-menu),
.app-container.has-bg .main-content :deep(.conn-context-menu),
.app-container.has-bg .main-content :deep(.qc-context-menu),
.app-container.has-bg .main-content :deep(.sftp-context-menu),
.app-container.has-bg .main-content :deep(.tab-context-menu),
.app-container.has-bg .main-content :deep(.start-context-menu),
.app-container.has-bg .main-content :deep(.tn-context-menu),
.app-container.has-bg .main-content :deep(.ctx-menu),
.app-container.has-bg .main-content :deep(.panel-more-menu),
.app-container.has-bg .main-content :deep(.shell-submenu),
.app-container.has-bg .main-content :deep(.hash-dropdown),
.app-container.has-bg .main-content :deep(.drive-dropdown),
.app-container.has-bg .main-content :deep(.bookmark-dropdown),
.app-container.has-bg .main-content :deep(.type-filter-menu) {
  background-color: var(--bg-surface) !important;
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
/* 终端滚动条：轨道透明，滑块用半透明白（同全局滚动条，背景上可见）*/
.app-container.has-bg .main-content :deep(.xterm-viewport::-webkit-scrollbar-track) {
  background: transparent !important;
}
.app-container.has-bg .main-content :deep(.xterm-viewport::-webkit-scrollbar-thumb) {
  background: var(--scrollbar-thumb) !important;
}
/* 边栏毛玻璃 */
.app-container.has-bg .main-content :deep(.sidebar),
.app-container.has-bg .main-content :deep(.ai-sidebar) {
  backdrop-filter: blur(8px);
}
/* 开始页卡片、按钮毛玻璃 */
.app-container.has-bg .main-content :deep(.start-card),
.app-container.has-bg .main-content :deep(.start-action-btn),
.app-container.has-bg .main-content :deep(.start-action-btn-dropdown-arrow) {
  backdrop-filter: blur(8px);
}
/* 各类输入框毛玻璃 */
.app-container.has-bg .main-content :deep(.el-input__wrapper),
.app-container.has-bg .main-content :deep(.el-textarea__inner),
.app-container.has-bg .main-content :deep(.el-input-number),
.app-container.has-bg .main-content :deep(.el-select__wrapper) {
  backdrop-filter: blur(8px);
}

</style>
