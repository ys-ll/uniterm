<template>
  <div
    class="app-header"
    :class="`platform-${platform}`"
    @dblclick="onDblClick"
  >
    <!-- macOS: spacer for native traffic lights -->
    <div v-if="platform === 'darwin'" class="mac-traffic-light-spacer" />

    <!-- Connections button (icon only, leftmost) -->
    <button class="header-btn" @click="emit('toggle-sidebar')" :title="t('header.connections')">
      <el-icon><PanelLeft :size="14" /></el-icon>
    </button>


    <!-- Tabs list -->
    <div class="header-tabs" :class="{ 'tabs-centered': platform === 'darwin' }">
      <TabsList
        @close-tab="(id: string) => emit('close-tab', id)"
        @toggle-ai-lock="(panelId: string) => emit('toggle-ai-lock', panelId)"
        @tab-dragstart="(e: DragEvent, tabId: string) => emit('tab-dragstart', e, tabId)"
      />
    </div>

    <!-- AI button -->
    <button class="header-btn" @click="emit('toggle-ai')" :title="t('header.ai')">
      <el-icon><Bot :size="14" /></el-icon>
    </button>

    <!-- Settings button (icon only, rightmost) -->
    <button class="header-btn" @click="emit('open-settings')" :title="t('header.settings')">
      <el-icon><Settings :size="14" /></el-icon>
    </button>

    <!-- Windows/Linux: window controls right -->
    <WindowControls
      v-if="platform !== 'darwin'"
      :is-maximised="isMaximised"
      @minimise="onMinimise"
      @maximise="onMaximise"
      @close="onClose"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, h } from 'vue'
import { Settings, PanelLeft, Bot } from '@lucide/vue'
import { ElMessageBox, ElCheckbox } from 'element-plus'
import { useI18n } from '../i18n'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useSettingsStore } from '../stores/settingsStore'
import WindowControls from './WindowControls.vue'
import TabsList from './TabsList.vue'
import {
  Environment,
  WindowMinimise,
  WindowToggleMaximise,
  WindowMaximise,
  WindowUnmaximise,
  WindowIsMaximised,
  WindowIsMinimised,
  WindowSetMaxSize,
  WindowGetPosition,
  WindowGetSize,
  Quit,
  ScreenGetAll,
} from '../../wailsjs/runtime'
import { SaveWindowState } from '../../wailsjs/go/main/App'

const { t } = useI18n()
const tabStore = useTabStore()
const panelStore = usePanelStore()
const sessionStore = useSessionStore()
const settingsStore = useSettingsStore()

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
  'open-settings': []
  'close-tab': [id: string]
  'toggle-ai-lock': [panelId: string]
  'tab-dragstart': [e: DragEvent, tabId: string]
}>()

const platform = ref<'windows' | 'darwin' | 'linux'>('windows')
const isMaximised = ref(false)

async function updateMaximisedState() {
  try {
    isMaximised.value = await WindowIsMaximised()
  } catch {
    // ignore
  }
}

function onMinimise() {
  WindowMinimise()
}

async function onMaximise() {
  if (platform.value === 'linux') {
    await linuxMaximise()
  } else {
    WindowToggleMaximise()
  }
  setTimeout(() => {
    updateMaximisedState()
    saveWindowState()
  }, 100)
}

async function linuxMaximise() {
  const maximised = await WindowIsMaximised()
  if (maximised) {
    // Restore: use native unmaximise, then clear max size constraint
    WindowUnmaximise()
    WindowSetMaxSize(0, 0)
  } else {
    // Before native maximise, set max size to current screen dimensions
    // to prevent GTK from clamping to the wrong monitor's size.
    try {
      const screens = await ScreenGetAll()
      const current = screens.find((s: { isCurrent: boolean }) => s.isCurrent) || screens[0]
      if (current) {
        WindowSetMaxSize(current.width, current.height)
      }
    } catch {
      // Fallback: set large max size to disable any constraint
      WindowSetMaxSize(9999, 9999)
    }
    WindowMaximise()
  }
}

let saveTimer: ReturnType<typeof setTimeout> | null = null

async function saveWindowState() {
  try {
    // Do not save geometry when minimised — the position is off-screen
    // and the size is the tiny taskbar thumbnail.
    if (await WindowIsMinimised()) return
    const maxed = await WindowIsMaximised()
    const { x, y } = await WindowGetPosition()
    const { w, h } = await WindowGetSize()
    SaveWindowState(x, y, w, h, maxed)
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
      }
      if (dontShowAgain.value) {
        settingsStore.settings.closeAppPrompt = false
        settingsStore.save()
      }
    }
  }
  await saveWindowState()
  Quit()
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
    const env = await Environment()
    const p = env.platform.toLowerCase()
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

.header-tabs.tabs-centered {
  justify-content: center;
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

</style>
