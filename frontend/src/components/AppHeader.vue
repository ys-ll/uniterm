<template>
  <div
    class="app-header"
    :class="`platform-${platform}`"
    @dblclick="onDblClick"
  >
    <!-- macOS: spacer for native traffic lights -->
    <div v-if="platform === 'darwin' && !localStateStore.state.systemTitleBar" class="mac-traffic-light-spacer" />

    <!-- Connections button (icon only, leftmost) -->
    <button class="header-btn" @click="emit('toggle-sidebar')" :title="t('header.connections') + shortcutSuffix('toggleSidebar')">
      <el-icon><PanelLeft :size="14" /></el-icon>
    </button>


    <!-- Tabs list -->
    <div class="header-tabs">
      <TabsList
        @close-tab="(id: string) => emit('close-tab', id)"
        @close-tab-batch="(ids: string[]) => emit('close-tab-batch', ids)"
        @toggle-ai-lock="(panelId: string) => emit('toggle-ai-lock', panelId)"
        @tab-dragstart="(e: DragEvent, tabId: string) => emit('tab-dragstart', e, tabId)"
      />
    </div>

    <!-- AI button -->
    <button class="header-btn" @click="emit('toggle-ai')" :title="t('header.ai') + shortcutSuffix('focusAI')">
      <el-icon><Bot :size="14" /></el-icon>
    </button>

    <!-- Settings button opens a dropdown menu with common settings items -->
    <div class="settings-wrap">
      <button ref="settingsBtnRef" class="header-btn" @click.stop="toggleSettingsMenu" :title="t('header.menu')">
        <el-icon><MenuIcon :size="14" /></el-icon>
      </button>

      <!-- Settings dropdown (theme / language / ai / identities / proxies / settings / check update) -->
      <Menu ref="settingsMenuRef" align="end" v-model:visible="showSettingsMenu">
        <!-- 主题 -->
        <MenuSubmenu :label="t('settings.theme')">
          <MenuItem
            v-for="opt in themeOptions"
            :key="opt.value"
            :class="{ active: settingsStore.settings.theme === opt.value }"
            @click="applyTheme(opt.value)"
          >{{ opt.label }}</MenuItem>
        </MenuSubmenu>

        <!-- 语言 -->
        <MenuSubmenu :label="t('settings.language')">
          <MenuItem
            v-for="lang in LANGUAGE_OPTIONS"
            :key="lang.value"
            :class="{ active: settingsStore.settings.language === lang.value }"
            @click="applyLanguage(lang.value)"
          >{{ lang.native }}</MenuItem>
          <MenuItem
            :class="{ active: settingsStore.settings.language === 'system' }"
            @click="applyLanguage('system')"
          >{{ t('settings.langSystem') }}</MenuItem>
        </MenuSubmenu>

        <MenuDivider />

        <!-- AI模型 / 密钥库 / 代理 -->
        <MenuItem @click="openCategory('ai')">{{ t('settings.ai') }}</MenuItem>
        <MenuItem @click="openCategory('identities')">{{ t('settings.identities') }}</MenuItem>
        <MenuItem @click="openCategory('proxies')">{{ t('settings.proxies') }}</MenuItem>
        <MenuItem @click="openCategory('tunnels')">{{ t('settings.tunnels') }}</MenuItem>

        <MenuDivider />

        <!-- 导入 / 导出连接 -->
        <MenuItem @click="openImport">{{ t('importExport.import') }}</MenuItem>
        <MenuItem @click="openExport">{{ t('importExport.export') }}</MenuItem>

        <MenuDivider />

        <!-- 设置 / 关于 / 检查更新 -->
        <MenuItem :shortcut="menuShortcut('openSettings')" @click="openCategory('basic')">{{ t('settings.title') }}</MenuItem>
        <MenuItem @click="openCategory('about')">{{ t('settings.about') }}</MenuItem>
        <MenuItem @click="checkUpdate">{{ t('settings.checkUpdate') }}</MenuItem>
      </Menu>

      <ImportDialog v-model:visible="showImportDialog" />
      <ExportDialog v-model:visible="showExportDialog" />
    </div>

    <!-- Windows/Linux: window controls right (hidden when using system title bar) -->
    <WindowControls
      v-if="showWindowControls"
      :is-maximised="isMaximised"
      @minimise="onMinimise"
      @maximise="onMaximise"
      @close="onClose"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, h } from 'vue'
import { Menu as MenuIcon, PanelLeft, Bot } from '@lucide/vue'
import { ElMessageBox, ElCheckbox } from 'element-plus'
import { useI18n } from '../i18n'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useSettingsStore } from '../stores/settingsStore'
import { formatKeyBinding } from '../composables/useKeyboardShortcuts'
import { useLocalStateStore } from '../stores/localStateStore'
import { useUpdateCheck } from '../composables/useUpdateCheck'
import { LANGUAGE_OPTIONS } from '../types/settings'
import type { AppSettings, ShortcutAction } from '../types/settings'
import WindowControls from './WindowControls.vue'
import TabsList from './TabsList.vue'
import ImportDialog from './ImportDialog.vue'
import ExportDialog from './ExportDialog.vue'
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
const settingsStore = useSettingsStore()
const localStateStore = useLocalStateStore()

// ── Settings dropdown menu ──
const updateCheck = useUpdateCheck()
const showSettingsMenu = ref(false)
const settingsBtnRef = ref<HTMLElement | null>(null)
const settingsMenuRef = ref<InstanceType<typeof Menu> | null>(null)

const themeOptions = computed(() => [
  { value: 'dark' as const, label: t('settings.themeDark') },
  { value: 'deep-blue' as const, label: t('settings.themeDeepBlue') },
  { value: 'light' as const, label: t('settings.themeLight') },
  { value: 'system' as const, label: t('settings.themeSystem') },
])

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

function applyTheme(value: AppSettings['theme']) {
  settingsStore.updateTheme(value)
  closeSettingsMenu()
}

function applyLanguage(value: AppSettings['language']) {
  settingsStore.updateLanguage(value)
  closeSettingsMenu()
}

function openCategory(category?: string) {
  emit('open-settings', category)
  closeSettingsMenu()
}

function checkUpdate() {
  updateCheck.checkForUpdate(true)
  closeSettingsMenu()
}

const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent)

// " (Ctrl+Shift+K)" suffix for a shortcut action's tooltip, '' when unset.
// Reactive via settingsStore, so tooltips update when the user rebinds keys.
function shortcutSuffix(action: 'focusAI' | 'toggleSidebar'): string {
  const b = settingsStore.settings.keyboard[action]
  if (!b) return ''
  const key = formatKeyBinding(b, isMac)
  return key ? ` (${key})` : ''
}

// Right-aligned keybinding hint for a menu item, '' when unset. Reactive via
// settingsStore, so hints follow the user's rebinds.
function menuShortcut(action: ShortcutAction): string {
  const b = settingsStore.settings.keyboard[action]
  if (!b) return ''
  return formatKeyBinding(b, isMac)
}

const hasActiveConnections = computed(() =>
  tabStore.tabs.some(t => {
    if (t.type === 'start' || t.type === 'settings') return false
    const panelIds = t.type === 'workspace' ? t.panelIds : 'panelId' in t ? [t.panelId] : []
    return panelIds.some(pid => {
      const p = panelStore.getPanel(pid)
      if (!p?.sessionId) return false
      return sessionStore.getStatus(p.sessionId) === 'connected'
    })
  })
)

const emit = defineEmits<{
  'toggle-ai': []
  'toggle-sidebar': []
  'open-settings': [category?: string]
  'close-tab': [id: string]
  'close-tab-batch': [ids: string[]]
  'toggle-ai-lock': [panelId: string]
  'tab-dragstart': [e: DragEvent, tabId: string]
}>()

// Detect synchronously so the layout is correct on first render,
// even if the Wails System.Environment() call is slow or fails.
function detectPlatformSync(): 'windows' | 'darwin' | 'linux' {
  const ua = navigator.userAgent
  if (/Mac|iPhone|iPad/.test(ua)) return 'darwin'
  if (/Linux/.test(ua)) return 'linux'
  return 'windows'
}
const platform = ref<'windows' | 'darwin' | 'linux'>(detectPlatformSync())
const isMaximised = ref(false)

// On Windows/Linux the app draws its own window controls — but not when the
// user opted into the OS native title bar, which already provides them.
const showWindowControls = computed(
  () => platform.value !== 'darwin' && !localStateStore.state.systemTitleBar
)

async function updateMaximisedState() {
  try {
    isMaximised.value = await Window.IsMaximised()
  } catch {
    // ignore
  }
}

function onMinimise() {
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
        Window.SetMaxSize(current.width, current.height)
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
      // Hide the native RDP window so the dialog isn't covered by it (issue #346)
      window.dispatchEvent(new CustomEvent('rdp:overlay-push'))
      try {
        await ElMessageBox.confirm(
          h('div', { style: 'display:flex;flex-direction:column;gap:10px' }, [
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
      } finally {
        window.dispatchEvent(new CustomEvent('rdp:overlay-pop'))
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
  height: 44px;
  padding: 0 8px;
  gap: 2px;
  background: var(--bg-elevated);
  flex-shrink: 0;
  position: relative;
  z-index: 10;
  --wails-draggable: drag;
}

.app-header.platform-darwin {
  height: 52px;
  padding: 0 10px;
  gap: 8px;
}

.app-header::after {
  content: '';
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 1px;
  background: linear-gradient(
    90deg,
    transparent 0%,
    var(--accent-subtle) 20%,
    var(--accent-glow) 50%,
    var(--accent-subtle) 80%,
    transparent 100%
  );
}

.header-tabs {
  display: flex;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  justify-content: flex-start;
  align-items: center;
}

.header-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  padding: 5px 8px;
  font-family: var(--font-ui);
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
  flex-shrink: 0;
  --wails-draggable: no-drag;
}

.header-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.header-btn.active {
  background: var(--bg-active, var(--bg-hover));
  color: var(--accent, var(--text-primary));
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
  font-size: 14px;
}

[data-theme="light"] .app-header::after {
  background: linear-gradient(
    90deg,
    transparent 0%,
    var(--accent-subtle) 20%,
    var(--accent-glow) 50%,
    var(--accent-subtle) 80%,
    transparent 100%
  );
}

.mac-traffic-light-spacer {
  width: 72px;
  height: 1px;
  flex-shrink: 0;
}

.app-header :deep(.window-controls) {
  --wails-draggable: no-drag;
}

.app-header.platform-darwin :deep(.window-controls) {
  align-self: center;
}

/* ── Settings dropdown menu ── */
.settings-wrap {
  position: relative;
  flex-shrink: 0;
  --wails-draggable: no-drag;
}

</style>
