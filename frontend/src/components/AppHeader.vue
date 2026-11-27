<template>
  <div
    class="app-header"
    :class="`platform-${platform}`"
    @dblclick="onDblClick"
  >
    <!-- macOS: custom traffic lights (Wails frameless hides the native ones) -->
    <WindowControls
      v-if="showWindowControls && platform === 'darwin'"
      variant="mac"
      :is-maximised="isMaximised"
      @minimise="onMinimise"
      @maximise="onMaximise"
      @close="onClose"
    />

    <!-- Brand mark anchors the tab strip; the panel toggles share the right side. -->
    <div v-if="showBrandMark" class="brand-mark" role="img" aria-label="uniTerm" title="uniTerm">
      <img src="/favicon.png" alt="" draggable="false" />
    </div>

    <!-- Tabs list -->
    <div class="header-tabs">
      <TabsList
        @close-tab="(id: string) => emit('close-tab', id)"
        @close-tab-batch="(ids: string[]) => emit('close-tab-batch', ids)"
        @toggle-ai-lock="(panelId: string) => emit('toggle-ai-lock', panelId)"
        @tab-dragstart="(e: DragEvent, tabId: string) => emit('tab-dragstart', e, tabId)"
        @new-local-terminal="emit('new-local-terminal')"
      />
    </div>

    <button
      v-if="mcpSessions.length"
      class="header-btn mcp-sessions-btn"
      :class="{ active: !!tabStore.activeMcpTab }"
      :aria-expanded="mcpSessionsVisible"
      :title="tabStore.activeMcpTab?.name || t('mcp.sessions.title')"
      @click.stop="mcpSessionsMenu?.toggle($event.currentTarget as HTMLElement)"
    >
      {{ t('mcp.sessions.title') }} ({{ mcpSessions.length }})
    </button>
    <Menu ref="mcpSessionsMenu" v-model:visible="mcpSessionsVisible" align="end">
      <MenuSubmenu
        v-for="{ tab, status } in mcpSessions"
        :key="tab.id"
        :label="`${tab.name} · ${t('mcp.sessions.' + status)}`"
      >
        <MenuItem v-if="tab.id === tabStore.activeTabId" @click="tabStore.hideMcpTab(tab.id); mcpSessionsVisible = false">
          {{ t('mcp.sessions.hide') }}
        </MenuItem>
        <MenuItem v-else @click="tabStore.showMcpTab(tab.id); mcpSessionsVisible = false">
          {{ t('mcp.sessions.show') }}
        </MenuItem>
        <MenuItem
          :class="{ disabled: status === 'connecting' }"
          :aria-disabled="status === 'connecting'"
          @click="disconnectMcpSession(tab.id, status)"
        >
          {{ status === 'connected' || status === 'connecting' ? t('mcp.sessions.disconnect') : t('tab.close') }}
        </MenuItem>
      </MenuSubmenu>
    </Menu>

    <div class="panel-toggles">
      <button class="header-btn" @click="emit('toggle-sidebar')" :title="t('sidebar.locSidebar')" :aria-label="t('sidebar.locSidebar')" :aria-pressed="!!sidebarVisible">
        <PanelToggleIcon side="left" :active="!!sidebarVisible" />
      </button>

      <!-- In narrow layout these views merge into the left sidebar. -->
      <button
        v-if="!uiLayout.narrowLayout"
        class="header-btn"
        @click="emit('toggle-bottom-panel')"
        :title="t('sidebar.locBottom')"
        :aria-label="t('sidebar.locBottom')"
        :aria-pressed="bottomPanelShown"
      >
        <PanelToggleIcon side="bottom" :active="bottomPanelShown" />
      </button>

      <!-- Right rail toggle: shows/hides the whole dock (like the left
           sidebar and bottom panel buttons), not a single panel's. -->
      <button
        v-if="!uiLayout.narrowLayout"
        class="header-btn"
        @click="emit('toggle-right-panel')"
        :title="t('sidebar.locRight')"
        :aria-label="t('sidebar.locRight')"
        :aria-pressed="rightPanelShown"
      >
        <PanelToggleIcon side="right" :active="rightPanelShown" />
      </button>
    </div>

    <!-- Settings button opens a dropdown menu with common settings items -->
    <div class="settings-wrap">
      <button ref="settingsBtnRef" class="header-btn" @click.stop="toggleSettingsMenu" :title="t('header.menu')">
        <el-icon><MenuIcon class="header-icon" /></el-icon>
      </button>

      <!-- Settings dropdown (theme / language / ai / identities / proxies / settings / check update) -->
      <Menu ref="settingsMenuRef" align="end" v-model:visible="showSettingsMenu">
        <!-- 主题 -->
        <MenuSubmenu :label="t('settings.theme')">
          <MenuItem
            :class="{ active: settingsStore.settings.theme === 'system' }"
            @click="applyTheme('system')"
          >{{ t('settings.themeSystem') }}</MenuItem>
          <MenuDivider />
          <MenuItem
            v-for="opt in themeOptions.slice(1)"
            :key="opt.value"
            :class="{ active: settingsStore.settings.theme === opt.value }"
            @click="applyTheme(opt.value)"
          >{{ opt.label }}</MenuItem>
        </MenuSubmenu>

        <!-- 终端主题 -->
        <MenuSubmenu :label="t('settings.terminalTheme')" class="terminal-theme-submenu">
          <MenuItem
            :class="{ active: settingsStore.settings.terminal.theme === FOLLOW_APP_THEME }"
            @click="applyTerminalTheme(FOLLOW_APP_THEME)"
          >{{ t('settings.followAppTheme') }}</MenuItem>
          <MenuDivider />
          <MenuItem
            v-for="entry in darkTerminalThemes"
            :key="entry.value"
            :class="{ active: settingsStore.settings.terminal.theme === entry.value }"
            @click="applyTerminalTheme(entry.value)"
          >{{ entry.label }}</MenuItem>
          <MenuDivider />
          <MenuItem
            v-for="entry in lightTerminalThemes"
            :key="entry.value"
            :class="{ active: settingsStore.settings.terminal.theme === entry.value }"
            @click="applyTerminalTheme(entry.value)"
          >{{ entry.label }}</MenuItem>
          <template v-if="settingsStore.settings.customTerminalThemes.length > 0">
            <MenuDivider />
            <MenuItem
              v-for="custom in settingsStore.settings.customTerminalThemes"
              :key="custom.id"
              :class="{ active: settingsStore.settings.terminal.theme === custom.id }"
              @click="applyTerminalTheme(custom.id)"
            >{{ custom.name }}</MenuItem>
          </template>
        </MenuSubmenu>

        <!-- 语言 -->
        <MenuSubmenu :label="t('settings.language')">
          <MenuItem
            :class="{ active: settingsStore.settings.language === 'system' }"
            @click="applyLanguage('system')"
          >{{ t('settings.langSystem') }}</MenuItem>
          <MenuDivider />
          <MenuItem
            v-for="lang in LANGUAGE_OPTIONS"
            :key="lang.value"
            :class="{ active: settingsStore.settings.language === lang.value }"
            @click="applyLanguage(lang.value)"
          >{{ lang.native }}</MenuItem>
        </MenuSubmenu>

        <MenuDivider />

        <!-- 连接：新建连接 + 连接的安全与通道配置（密钥库 / 代理） -->
        <MenuSubmenu :label="t('menu.connectionToolbox')">
          <MenuItem @click="openNewConnection">{{ t('header.newConnection') }}</MenuItem>
          <MenuItem @click="openCategory('identities')">{{ t('settings.identities') }}</MenuItem>
          <MenuItem @click="openCategory('proxies')">{{ t('settings.proxies') }}</MenuItem>
        </MenuSubmenu>

        <!-- 网络工具箱子菜单：网络扫描 / 本地服务 / SSH隧道。Server deployments
             hide local services: the servers bind inside the container and
             share its filesystem, so "local" files/fports are not the browser
             user's machine. -->
        <MenuSubmenu :label="t('menu.networkToolbox')">
          <MenuItem @click="openNetscan">{{ t('netscan.title') }}</MenuItem>
          <MenuItem v-if="!webServerMode" @click="openLocalServices">{{ t('localsrv.title') }}</MenuItem>
          <MenuItem v-if="!webServerMode" @click="openPacketcap">{{ t('packetcap.title') }}</MenuItem>
          <MenuItem v-if="!webServerMode" @click="openFirewall">{{ t('firewall.title') }}</MenuItem>
          <MenuItem @click="openCategory('tunnels')">{{ t('settings.tunnels') }}</MenuItem>
        </MenuSubmenu>

        <!-- 开发者工具箱：JSON 格式化 / 哈希与编码 -->
        <MenuItem @click="openTools">{{ t('tools.title') }}</MenuItem>

        <!-- AI 子菜单（与设置页 AI 分组一致） -->
        <MenuSubmenu :label="t('menu.ai')">
          <MenuItem @click="openCategory('ai')">{{ t('settings.ai') }}</MenuItem>
          <MenuItem @click="openCategory('skills')">{{ t('settings.skills') }}</MenuItem>
          <MenuItem @click="openCategory('commands')">{{ t('settings.commands') }}</MenuItem>
          <!-- MCP features have no mobile backend; the built-in MCP server is
               also hidden in server deployments (binds a worker loopback port). -->
          <MenuItem v-if="!isMobilePlatform(platform.value)" @click="openCategory('mcp-client')">{{ t('settings.mcpClient') }}</MenuItem>
          <MenuItem v-if="!isMobilePlatform(platform.value) && !webServerMode" @click="openCategory('mcp')">{{ t('settings.mcp') }}</MenuItem>
        </MenuSubmenu>

        <MenuDivider />

        <!-- 导入 / 导出连接 -->
        <MenuItem @click="openImport">{{ t('importExport.import') }}</MenuItem>
        <MenuItem @click="openExport">{{ t('importExport.export') }}</MenuItem>

        <MenuDivider />

        <!-- 设置 / 云同步 / 关于 / 检查更新 -->
        <MenuItem :shortcut="menuShortcut('openSettings')" @click="openCategory('basic')">{{ t('settings.title') }}</MenuItem>
        <MenuItem @click="openCategory('sync')">{{ t('settings.sync') }}</MenuItem>
        <MenuItem @click="openCategory('about')">{{ t('settings.about') }}</MenuItem>
        <!-- In-app update download is desktop-only; server deployments update
             by pulling a new image and the backend refuses the call. -->
        <MenuItem v-if="!isMobilePlatform(platform.value) && !webServerMode" @click="checkUpdate">{{ t('settings.checkUpdate') }}</MenuItem>
      </Menu>

      <ImportDialog v-model:visible="showImportDialog" />
      <ExportDialog v-model:visible="showExportDialog" />
    </div>

    <!-- Web edition account menu: renders nothing unless /api/web/mode reports
         server mode, so the desktop header is unchanged. -->
    <WebAccountMenu />

    <!-- Windows/Linux: window controls right -->
    <WindowControls
      v-if="showWindowControls && platform !== 'darwin'"
      :is-maximised="isMaximised"
      @minimise="onMinimise"
      @maximise="onMaximise"
      @close="onClose"
    />
    <!-- Server mode: the web account widget takes the window-controls spot
         (those buttons are hidden — there is no OS window to control). -->
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, h } from 'vue'
import { Menu as MenuIcon } from '@lucide/vue'
import { ElMessageBox, ElCheckbox } from 'element-plus'
import { useI18n } from '../i18n'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useSettingsStore } from '../stores/settingsStore'
import { formatKeyBinding } from '../composables/useKeyboardShortcuts'
import { isWebServerMode } from '../services/webSftpBridge'
import { useLocalStateStore } from '../stores/localStateStore'
import { useUiLayoutStore } from '../stores/uiLayoutStore'
import { useUpdateCheck } from '../composables/useUpdateCheck'
import { LANGUAGE_OPTIONS, TERMINAL_THEMES, FOLLOW_APP_THEME } from '../types/settings'
import type { AppSettings, ShortcutAction } from '../types/settings'
import { detectPlatformSync, isMobilePlatform, isPhoneLayout } from '../utils/platform'
import WindowControls from './WindowControls.vue'
import PanelToggleIcon from './icons/PanelToggleIcon.vue'
import TabsList from './TabsList.vue'
import ImportDialog from './ImportDialog.vue'
import ExportDialog from './ExportDialog.vue'
import WebAccountMenu from './WebAccountMenu.vue'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import MenuSubmenu from './MenuSubmenu.vue'
import MenuDivider from './MenuDivider.vue'
import { Application, Screens, System, Window } from '@wailsio/runtime'
import { SaveWindowState } from '../../bindings/github.com/ys-ll/uniterm/app'

const { t } = useI18n()
const tabStore = useTabStore()
const panelStore = usePanelStore()
const sessionStore = useSessionStore()
const mcpSessionsMenu = ref<InstanceType<typeof Menu> | null>(null)
const mcpSessionsVisible = ref(false)
const mcpSessions = computed(() => tabStore.mcpTabs.map(tab => {
  const sessionId = panelStore.getPanel(tab.panelId)?.sessionId
  const status = sessionId ? sessionStore.getStatus(sessionId) : 'disconnected'
  return { tab, status }
}))
const settingsStore = useSettingsStore()
const localStateStore = useLocalStateStore()
// Window frame is fixed at startup (Wails limitation): the OS title bar only
// appears after a relaunch. Snapshot the choice once the persisted local
// state has loaded (reads before that would only see the default, since
// LocalState loads via async IPC), then stop following the store — so
// toggling it in settings doesn't strip our own window controls
// (close/min/max) before the restart takes effect (issue #935).
const systemTitleBarAtStartup = ref<boolean | null>(null)
watch(
  () => localStateStore.loaded,
  (loaded) => {
    if (loaded && systemTitleBarAtStartup.value === null) {
      systemTitleBarAtStartup.value = localStateStore.state.systemTitleBar
    }
  },
  { immediate: true }
)

// ── Settings dropdown menu ──
const updateCheck = useUpdateCheck()
const showSettingsMenu = ref(false)
const settingsBtnRef = ref<HTMLElement | null>(null)
const settingsMenuRef = ref<InstanceType<typeof Menu> | null>(null)

const themeOptions = computed(() => [
  { value: 'system' as const, label: t('settings.themeSystem') },
  { value: 'dark' as const, label: t('settings.themeDark') },
  { value: 'deep-blue' as const, label: t('settings.themeDeepBlue') },
  { value: 'light' as const, label: t('settings.themeLight') },
])

// Terminal theme submenu: built-ins split into dark / light sections.
const darkTerminalThemes = TERMINAL_THEMES.filter(t => t.type === 'dark')
const lightTerminalThemes = TERMINAL_THEMES.filter(t => t.type === 'light')

function toggleSettingsMenu() {
  settingsMenuRef.value?.toggle(settingsBtnRef.value!)
}

function closeSettingsMenu() {
  showSettingsMenu.value = false
}

// ── 导入 / 导出（对话框自包含，菜单关闭后由对话框接管） ──
const showImportDialog = ref(false)
const showExportDialog = ref(false)

function openImport() {
  closeSettingsMenu()
  showImportDialog.value = true
}

function openExport() {
  closeSettingsMenu()
  showExportDialog.value = true
}

// Theme / language / terminal-theme picks stay open on click so the user can
// flip through options and preview live; the menu closes on outside click or
// Escape (standard Menu behavior).

function applyTheme(value: AppSettings['theme']) {
  settingsStore.updateTheme(value)
}

function applyLanguage(value: AppSettings['language']) {
  settingsStore.updateLanguage(value)
}

function applyTerminalTheme(value: string) {
  settingsStore.settings.terminal.theme = value
  settingsStore.save()
}

function openCategory(category?: string) {
  emit('open-settings', category)
  closeSettingsMenu()
}

function openNetscan() {
  emit('open-netscan')
  closeSettingsMenu()
}

function openTools() {
  emit('open-tools')
  closeSettingsMenu()
}

function openPacketcap() {
  emit('open-packetcap')
  closeSettingsMenu()
}

function openFirewall() {
  emit('open-firewall')
  closeSettingsMenu()
}

function openLocalServices() {
  emit('open-local-services')
  closeSettingsMenu()
}

function openNewConnection() {
  emit('open-new-connection')
  closeSettingsMenu()
}

function checkUpdate() {
  updateCheck.checkForUpdate(true)
  closeSettingsMenu()
}

const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent)

// Right-aligned keybinding hint for a menu item, '' when unset. Reactive via
// settingsStore, so hints follow the user's rebinds.
function menuShortcut(action: ShortcutAction): string {
  if (isMobilePlatform(platform.value)) return ''
  const b = settingsStore.settings.keyboard[action]
  if (!b) return ''
  return formatKeyBinding(b, isMac)
}

const hasActiveConnections = computed(() =>
  tabStore.tabs.some(t => {
    if (t.type === 'start' || t.type === 'settings' || t.type === 'tools' || t.type === 'netscan') return false
    const panelIds = t.type === 'workspace' ? t.panelIds : 'panelId' in t ? [t.panelId] : []
    return panelIds.some(pid => {
      const p = panelStore.getPanel(pid)
      if (!p?.sessionId) return false
      return sessionStore.getStatus(p.sessionId) === 'connected'
    })
  })
)

// Bottom panel button active state — the toggle opens the panel even when
// it has no tabs (empty panel offers dockable options). Read straight from
// the layout store (no prop drilling).
const uiLayout = useUiLayoutStore()
const bottomPanelShown = computed(() => uiLayout.bottomPanelVisible)
const rightPanelShown = computed(() => uiLayout.rightPanelVisible)

// Sidebar/right-rail/bottom-panel toggle icons fill the corresponding region when open.
defineProps<{
  sidebarVisible?: boolean
}>()

const emit = defineEmits<{
  'toggle-right-panel': []
  'toggle-bottom-panel': []
  'toggle-sidebar': []
  'open-settings': [category?: string]
  'open-netscan': []
  'open-tools': []
  'open-packetcap': []
  'open-firewall': []
  'open-local-services': []
  'open-new-connection': []
  'close-tab': [id: string]
  'close-tab-batch': [ids: string[]]
  'disconnect-mcp-session': [id: string]
  'toggle-ai-lock': [panelId: string]
  'tab-dragstart': [e: DragEvent, tabId: string]
  'new-local-terminal': []
}>()

function disconnectMcpSession(id: string, status: string) {
  if (status === 'connecting') return
  mcpSessionsVisible.value = false
  emit('disconnect-mcp-session', id)
}

const platform = ref(detectPlatformSync())
const isMaximised = ref(false)

// The app draws its own window controls on every platform — but not when the
// user opted into the OS native title bar at startup, which already provides
// them, and never on mobile (no OS window to minimise/maximise/close). On
// macOS they render as traffic lights on the left (see template). Before the
// snapshot lands, fall back to the live store value (the frameless default).
// In headless server mode the UI runs in a browser tab — there is no OS
// window to minimise/maximise/close, so the buttons are hidden entirely.
const webServerMode = ref(false)
isWebServerMode().then((v) => { webServerMode.value = v })

const showWindowControls = computed(
  () =>
    !(systemTitleBarAtStartup.value ?? localStateStore.state.systemTitleBar) &&
    !isMobilePlatform(platform.value) &&
    !webServerMode.value
)

// The OS title bar already carries the app icon. Wait for the persisted frame
// choice so the custom logo cannot flash briefly over a native title bar.
// Phone layouts hide it too: the strip is narrow there and the tabs need the room.
const showBrandMark = computed(() =>
  platform.value !== 'darwin' &&
  localStateStore.loaded &&
  !isPhoneLayout.value &&
  !(systemTitleBarAtStartup.value ?? localStateStore.state.systemTitleBar)
)

async function updateMaximisedState() {
  try {
    isMaximised.value = await Window.IsMaximised()
  } catch {
    // ignore
  }
}

function onMinimise() {
  // Minimize-to-tray (issue #1556): hide the window instead of minimising so
  // it leaves the taskbar. Tabs and sessions keep running either way — only
  // the window is hidden; restore via the tray icon (forced on while this
  // setting is enabled). Close paths are untouched and keep quitting.
  if (settingsStore.settings.minimizeToTray) {
    Window.Hide()
    return
  }
  Window.Minimise()
}

async function onMaximise() {
  if (platform.value === 'linux') {
    await linuxMaximise()
  } else {
    Window.ToggleMaximise()
  }
  setTimeout(() => {
    updateMaximisedState()
    saveWindowState()
  }, 100)
}

async function linuxMaximise() {
  const maximised = await Window.IsMaximised()
  if (maximised) {
    // Restore: use native unmaximise, then clear max size constraint
    Window.UnMaximise()
    Window.SetMaxSize(0, 0)
  } else {
    // Before native maximise, set max size to current screen dimensions
    // to prevent GTK from clamping to the wrong monitor's size.
    try {
      const screens = await Screens.GetAll()
      const current = screens.find((s: { isCurrent: boolean }) => s.isCurrent) || screens[0]
      if (current) {
        Window.SetMaxSize(current.Size.Width, current.Size.Height)
      }
    } catch {
      // Fallback: set large max size to disable any constraint
      Window.SetMaxSize(9999, 9999)
    }
    Window.Maximise()
  }
}

let saveTimer: ReturnType<typeof setTimeout> | null = null

async function saveWindowState() {
  try {
    // Do not save geometry when minimised — the position is off-screen
    // and the size is the tiny taskbar thumbnail.
    if (await Window.IsMinimised()) return
    const maxed = await Window.IsMaximised()
    const { x, y } = await Window.Position()
    const { width, height } = await Window.Size()
    SaveWindowState(x, y, width, height, maxed)
  } catch {
    // ignore
  }
}

async function onClose() {
  if (hasActiveConnections.value) {
    if (!settingsStore.settings.closeAppPrompt) {
      // skip dialog, proceed to quit
    } else {
      const dontShowAgain = ref(false)

      try {
        await ElMessageBox.confirm(
          h('div', { style: 'display:flex;flex-direction:column;gap:0.625rem' }, [
            h('span', t('app.closeConfirm')),
            h(ElCheckbox, {
              'onUpdate:modelValue': (v: boolean) => { dontShowAgain.value = v }
            }, () => t('app.dontShowAgain'))
          ]),
          t('app.closeTitle'),
          { confirmButtonText: t('tab.close'), cancelButtonText: t('conn.cancel'), type: 'warning' }
        )
      } catch {
        return // user cancelled
      }
      if (dontShowAgain.value) {
        settingsStore.settings.closeAppPrompt = false
        settingsStore.save()
      }
    }
  }
  await saveWindowState()
  Application.Quit()
}

function onDblClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (target.closest('button, input, textarea, select, a, [role="button"], .tab-item, .tab-more, .window-controls')) return
  onMaximise()
}

function onWindowResize() {
  updateMaximisedState()
  // Debounce save to avoid frequent writes during drag-resize
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(saveWindowState, 500)
}

onMounted(async () => {
  try {
    const env = await System.Environment()
    const p = env.OS.toLowerCase()
    if (p === 'darwin') platform.value = 'darwin'
    else if (p === 'linux') platform.value = 'linux'
    else if (p === 'android') platform.value = 'android'
    else if (p === 'ios') platform.value = 'ios'
    else platform.value = 'windows'
  } catch {
    platform.value = 'windows'
  }
  updateMaximisedState()
  window.addEventListener('resize', onWindowResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', onWindowResize)
})
</script>

<style scoped>
.app-header {
  display: flex;
  align-items: center;
  height: 2.75rem;
  padding: 0 0.5rem;
  gap: 0.125rem;
  background: var(--bg-titlebar);
  flex-shrink: 0;
  position: relative;
  z-index: 10;
  --wails-draggable: drag;
}

.header-tabs {
  display: flex;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  justify-content: flex-start;
  align-items: center;
}

.brand-mark {
  width: 1.5rem;
  height: 1.5rem;
  flex: none;
  overflow: hidden;
  border-radius: 0.375rem;
  position: relative;
  margin-right: 0.25rem;
}

.brand-mark img {
  width: 1.75rem;
  height: 1.75rem;
  max-width: none;
  max-height: none;
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  pointer-events: none;
}

.app-header.platform-darwin :deep(.window-controls.mac) {
  margin-right: 0.75rem;
}

.panel-toggles {
  display: flex;
  align-items: center;
  gap: 0.125rem;
  flex-shrink: 0;
  margin-left: 0.375rem;
}

.header-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 1.75rem;
  padding: 0.3125rem 0.375rem;
  font-family: var(--font-ui);
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--text-secondary);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  transition: all 0.15s ease;
  white-space: nowrap;
  flex-shrink: 0;
  --wails-draggable: no-drag;
}

.header-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

/* Other active header actions use an accent icon; panel toggles indicate
   their state by filling the corresponding region of their icon. */
.header-btn.active {
  background: transparent;
  color: var(--accent);
}

.header-btn.active:hover {
  background: var(--bg-hover);
  color: var(--accent);
}

.header-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.header-btn:disabled:hover {
  background: transparent;
  color: var(--text-secondary);
}

.header-btn .el-icon {
  font-size: 0.875rem;
}

.header-btn :deep(.header-icon) {
  width: 0.875rem;
  height: 0.875rem;
}

.mcp-sessions-btn {
  width: auto;
  padding-inline: 0.5rem;
  white-space: nowrap;
}

.app-header :deep(.window-controls) {
  --wails-draggable: no-drag;
}

/* ── Settings dropdown menu ── */
.settings-wrap {
  position: relative;
  flex-shrink: 0;
  --wails-draggable: no-drag;
}

</style>
