<template>
  <div class="settings-tab" ref="settingsTabRef" :class="{ narrow: isNarrow }">
    <div class="settings-sidebar" :class="{ collapsed: sidebarCollapsed, 'no-anim': !sidebarAnimReady }">
      <button
        class="sidebar-collapse-btn"
        :title="t('settings.toggleSidebar')"
        @click="sidebarCollapsed = !sidebarCollapsed"
      >
        <el-icon><component :is="sidebarCollapsed ? ChevronRight : ChevronLeft" /></el-icon>
      </button>
      <div
        v-for="cat in categories"
        :key="cat.key"
        class="settings-category"
        :class="{ active: settingsStore.activeCategory === cat.key }"
        @click="settingsStore.activeCategory = cat.key"
      >
        <el-icon class="category-icon"><component :is="cat.icon" /></el-icon>
        <span v-if="!sidebarCollapsed" class="category-label">{{ cat.label }}</span>
      </div>
    </div>

    <div class="settings-panel">
      <BasicSettings v-if="settingsStore.activeCategory === 'basic'" :is-narrow="isNarrow" />
      <TerminalSettings v-if="settingsStore.activeCategory === 'terminal'" :is-narrow="isNarrow" />
      <LocalDataSettings v-if="settingsStore.activeCategory === 'local-data'" :is-narrow="isNarrow" />
      <SyncSettings v-if="settingsStore.activeCategory === 'sync'" :is-narrow="isNarrow" />
      <ShortcutSettings v-if="settingsStore.activeCategory === 'keyboard' && !isMobile" :is-narrow="isNarrow" />
      <AISettings v-if="settingsStore.activeCategory === 'ai'" :is-narrow="isNarrow" />
      <SkillsSettings v-if="settingsStore.activeCategory === 'skills'" :is-narrow="isNarrow" />
      <MCPSettings v-if="['mcp', 'mcp-client'].includes(settingsStore.activeCategory)" :is-narrow="isNarrow" />
      <ConnectionSettings v-if="['identities', 'proxies', 'tunnels'].includes(settingsStore.activeCategory)" :is-narrow="isNarrow" />
      <AboutSettings v-if="settingsStore.activeCategory === 'about'" :is-narrow="isNarrow" />
    </div>

    <!-- Shared theme-editor dialog (opened from Basic/Terminal panels via the
         useThemeEditor singleton in useSettingsForm). -->
    <CustomThemeEditor v-model="themeEditorVisible" :source-theme-id="themeEditorSourceId" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, onActivated, onDeactivated } from 'vue'
import { Settings, Monitor, MessageCircleMore, Info, RefreshCw, Keyboard, Wrench, SquareSlash, Key, Network, ArrowRightLeft, ChevronLeft, ChevronRight, Plug, Server, HardDrive } from '@lucide/vue'
import { useSettingsStore } from '../stores/settingsStore'
import { useI18n } from '../i18n'
import { isMobilePlatform } from '../utils/platform'
import { useThemeEditor } from '../composables/useSettingsForm'
import CustomThemeEditor from './CustomThemeEditor.vue'
import BasicSettings from './settings/BasicSettings.vue'
import TerminalSettings from './settings/TerminalSettings.vue'
import LocalDataSettings from './settings/LocalDataSettings.vue'
import SyncSettings from './settings/SyncSettings.vue'
import ShortcutSettings from './settings/ShortcutSettings.vue'
import AISettings from './settings/AISettings.vue'
import SkillsSettings from './settings/SkillsSettings.vue'
import MCPSettings from './settings/MCPSettings.vue'
import ConnectionSettings from './settings/ConnectionSettings.vue'
import AboutSettings from './settings/AboutSettings.vue'

const settingsStore = useSettingsStore()
const { t } = useI18n()
const isMobile = isMobilePlatform()

// Theme-editor dialog state is a module-level singleton so the Basic/Terminal
// panels can open it without prop-drilling; the dialog itself renders here.
const { themeEditorVisible, themeEditorSourceId } = useThemeEditor()

// ── Category list ──
const categories = computed(() => {
  // Explicitly read language to ensure reactivity tracking
  void settingsStore.settings.language
  const cats = [
    { key: 'basic', label: t('settings.basic'), icon: Settings },
    { key: 'terminal', label: t('settings.terminal'), icon: Monitor },
    // Keyboard shortcuts still WORK on mobile (physical keyboards), but the
    // rebinding UI is desktop-only and hidden from the category list there.
    ...(!isMobile ? [{ key: 'keyboard', label: t('shortcut.title'), icon: Keyboard, group: 'app' }] : []),
    { key: 'ai', label: t('settings.ai'), icon: MessageCircleMore, group: 'ai' },
    { key: 'skills', label: t('settings.skills'), icon: Wrench, group: 'ai' },
    { key: 'commands', label: t('settings.commands'), icon: SquareSlash, group: 'ai' },
    // MCP features have no mobile backend — hidden from the category list there.
    ...(!isMobile ? [{ key: 'mcp-client', label: t('settings.mcpClient'), icon: Plug, group: 'ai' }] : []),
    ...(!isMobile ? [{ key: 'mcp', label: t('settings.mcp'), icon: Server, group: 'ai' }] : []),
    { key: 'identities', label: t('settings.identities'), icon: Key, group: 'net' },
    { key: 'proxies', label: t('settings.proxies'), icon: Network, group: 'net' },
    { key: 'tunnels', label: t('settings.tunnels'), icon: ArrowRightLeft, group: 'net' },
    // Local data (data dir / encryption / master password) and cloud sync both
    // manage where configuration lives, so they share a group. The local-data
    // tab is desktop-only: on mobile the data dir is OS-managed and keychain
    // encryption is the only supported mode, so nothing is configurable there.
    ...(!isMobile ? [{ key: 'local-data', label: t('settings.localData'), icon: HardDrive, group: 'data' }] : []),
    { key: 'sync', label: t('settings.sync'), icon: RefreshCw, group: 'data' },
    { key: 'about', label: t('settings.about'), icon: Info, group: 'other' },
  ]
  return cats
})

// A persisted desktop-only active category would show an empty settings page
// on mobile — fall back to basic.
watch(categories, (cats) => {
  if (isMobile && ['keyboard', 'local-data', 'mcp-client', 'mcp'].includes(settingsStore.activeCategory) && !cats.some(c => c.key === settingsStore.activeCategory)) {
    settingsStore.activeCategory = 'basic'
  }
}, { immediate: true })

// External requests to jump to a specific category (e.g. from another tab).
watch(() => settingsStore.openCategory, (cat) => {
  if (cat && (cat === 'basic' || cat === 'terminal' || cat === 'ai' || cat === 'sync' || cat === 'about' || (cat === 'keyboard' && !isMobile) || cat === 'identities' || cat === 'proxies' || cat === 'tunnels')) {
    settingsStore.activeCategory = cat
    settingsStore.openCategory = null
  }
})

// ── Sidebar collapse ──
// The category sidebar (11.25rem) pushes the settings panel off-screen on
// narrow windows (phone portrait). Collapse to icons-only below the width
// where the expanded layout stops fitting; the button toggles it manually
// and the auto rule re-applies whenever the window crosses the threshold.
const settingsTabRef = ref<HTMLElement | null>(null)
// Estimate the collapse decision from the window width so the very first
// paint is already correct — the element itself is not measurable before
// mount, and a post-mount correction is what reads as a "slide-in".
const sidebarCollapsed = ref(
  window.innerWidth < 42 * parseFloat(getComputedStyle(document.documentElement).fontSize)
)
// Suppress the width transition until the first post-mount correction has
// landed, covering the case where the estimate above was off (docked
// sidebars make the tab narrower than the window).
const sidebarAnimReady = ref(false)
// Below the same threshold the side-by-side setting cards (info + fixed-width
// control) cannot fit either, so the panel stacks them vertically.
const isNarrow = ref(false)

function updateSidebarCollapse() {
  const el = settingsTabRef.value
  if (!el) return
  const rem = parseFloat(getComputedStyle(document.documentElement).fontSize)
  // Expanded layout needs ~13.1rem sidebar + ~29rem panel => 42rem minimum.
  isNarrow.value = el.clientWidth < 42 * rem
  sidebarCollapsed.value = isNarrow.value
}

let sidebarResizeObserver: ResizeObserver | null = null

onMounted(() => {
  updateSidebarCollapse()
  requestAnimationFrame(() => {
    requestAnimationFrame(() => sidebarAnimReady.value = true)
  })
  if (settingsTabRef.value) {
    sidebarResizeObserver = new ResizeObserver(updateSidebarCollapse)
    sidebarResizeObserver.observe(settingsTabRef.value)
  }
})

onUnmounted(() => {
  sidebarResizeObserver?.disconnect()
  sidebarResizeObserver = null
})

// The tab lives inside <KeepAlive>: re-opening it re-attaches the DOM, which
// fires the ResizeObserver and can flip the collapsed decision while the
// width transition is already enabled. Re-suppress the transition across
// each re-activation so the correction lands instantly, without sliding.
onActivated(() => {
  sidebarAnimReady.value = false
  updateSidebarCollapse()
  requestAnimationFrame(() => {
    requestAnimationFrame(() => sidebarAnimReady.value = true)
  })
})

// KeepAlive deactivates (not unmounts) on tab switch; nothing extra to clean
// up here — sub-components handle their own listeners (e.g. keyboard rebind).
onDeactivated(() => {
})
</script>

<style scoped>
.settings-tab {
  display: flex;
  width: 100%;
  height: 100%;
  background: var(--bg-base);
  color: var(--text-primary);
}

.settings-sidebar {
  width: 11.25rem;
  flex-shrink: 0;
  margin-left: 1.25rem;
  margin-right: 0.625rem;
  padding: 1rem 0;
  border-right: 1px solid var(--border-hover);
  transition: width 0.15s ease;
  /* Scroll independently when the window is too short for every category */
  overflow-y: auto;
  overflow-x: hidden;
}

.sidebar-collapse-btn {
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.75rem;
  height: 1.75rem;
  margin: 0 0.5rem 0.5rem;
  margin-left: auto;
  padding: 0;
  background: var(--bg-base);
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  transition: all 0.12s ease;
}

.sidebar-collapse-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

/* Collapsed: icons only, centered */
.settings-sidebar.no-anim,
.settings-sidebar.no-anim .settings-category,
.settings-sidebar.no-anim .sidebar-collapse-btn {
  transition: none;
}

.settings-sidebar.collapsed {
  width: 3.5rem;
  margin-left: 0.625rem;
  margin-right: 0.375rem;
}

.settings-sidebar.collapsed .sidebar-collapse-btn {
  margin-left: auto;
}

.settings-sidebar.collapsed .settings-category {
  justify-content: center;
  padding: 0.625rem 0.5rem;
  margin: 0 0.375rem;
  gap: 0;
}

.settings-category {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  padding: 0.625rem 1rem;
  margin: 0 0.5rem;
  font-size: 0.8125rem;
  font-family: var(--font-ui);
  user-select: none;
  color: var(--text-secondary);
  border-radius: var(--radius-sm);
  transition: all 0.12s ease;
  border-left: 0.1875rem solid transparent;
}

.settings-category:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.settings-category.active {
  background: var(--accent-subtle);
  color: var(--accent);
  border-left-color: var(--accent);
  backdrop-filter: blur(0.5rem);
}

.category-icon {
  font-size: 1rem;
}

.category-label {
  line-height: 1;
}

.settings-panel {
  flex: 1;
  padding: 1.5rem 2rem;
  overflow-y: auto;
}
</style>
