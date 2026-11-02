<template>
  <!-- Panel-specific "more menu" items, keyed by view. Written ONCE here and
       rendered by PanelTitleBar, so the left sidebar's title bar and the
       bottom panel's header button always expose the same menu. Each section
       ends with its own divider so it separates cleanly from the common
       移动到 / 隐藏面板 pair. -->
  <template v-if="view === 'connections'">
    <MenuSubmenu :label="t('sidebar.displayMode')">
      <MenuItem
        iconic
        :icon="settingsStore.settings.connectionsDisplayMode === 'full' ? Check : undefined"
        @click="settingsStore.settings.connectionsDisplayMode = 'full'; settingsStore.save()"
      >{{ t('sidebar.displayModeFull') }}</MenuItem>
      <MenuItem
        iconic
        :icon="settingsStore.settings.connectionsDisplayMode === 'compact' ? Check : undefined"
        @click="settingsStore.settings.connectionsDisplayMode = 'compact'; settingsStore.save()"
      >{{ t('sidebar.displayModeCompact') }}</MenuItem>
    </MenuSubmenu>
    <MenuDivider />
  </template>

  <template v-else-if="view === 'quickCommands'">
    <MenuSubmenu :label="t('sidebar.displayMode')">
      <MenuItem
        iconic
        :icon="settingsStore.settings.quickCommandsDisplayMode === 'list' ? Check : undefined"
        @click="settingsStore.settings.quickCommandsDisplayMode = 'list'; settingsStore.save()"
      >{{ t('sidebar.displayStyleList') }}</MenuItem>
      <MenuItem
        iconic
        :icon="settingsStore.settings.quickCommandsDisplayMode === 'buttons' ? Check : undefined"
        @click="settingsStore.settings.quickCommandsDisplayMode = 'buttons'; settingsStore.save()"
      >{{ t('sidebar.displayStyleButtons') }}</MenuItem>
    </MenuSubmenu>
    <MenuDivider />
  </template>

  <template v-else-if="view === 'monitor'">
    <!-- Open the full monitor tab for this connection (the sidebar's pinned
         button was removed in favour of this menu item). -->
    <MenuItem iconic @click="openFullMonitor">{{ t('companion.openFullMonitor') }}</MenuItem>
    <MenuDivider />
    <MenuSubmenu :label="t('monitor.sectionConfig')">
      <template v-for="sec in MONITOR_SECTIONS" :key="sec.key">
        <!-- 全部网卡 is a sub-item of 网络: hidden while 网络 is hidden. -->
        <MenuItem
          v-if="sec.key !== 'allNet' || sectionEnabled('network')"
          iconic
          :icon="sectionEnabled(sec.key) ? Check : undefined"
          @click="toggleSection(sec.key)"
        >{{ t(sec.labelKey) }}</MenuItem>
      </template>
    </MenuSubmenu>
    <MenuDivider />
  </template>
</template>

<script setup lang="ts">
import { Check } from '@lucide/vue'
import { useI18n } from '../i18n'
import { useSettingsStore } from '../stores/settingsStore'
import { useCompanionStore } from '../stores/companionStore'
import { usePanelStore } from '../stores/panelStore'
import { monitorSectionEnabled } from '../types/settings'
import MenuItem from './MenuItem.vue'
import MenuSubmenu from './MenuSubmenu.vue'
import MenuDivider from './MenuDivider.vue'

defineProps<{
  view: string
}>()

const { t } = useI18n()
const settingsStore = useSettingsStore()
const companionStore = useCompanionStore()
const panelStore = usePanelStore()

// Order mirrors the monitor panel's card layout (top to bottom).
const MONITOR_SECTIONS: { key: string; labelKey: string }[] = [
  { key: 'basic', labelKey: 'monitor.secBasic' },
  { key: 'cpu', labelKey: 'monitor.secCpu' },
  { key: 'allCpu', labelKey: 'monitor.secAllCpu' },
  { key: 'memory', labelKey: 'monitor.secMemory' },
  { key: 'network', labelKey: 'monitor.secNetwork' },
  { key: 'allNet', labelKey: 'monitor.secAllNet' },
  { key: 'disk', labelKey: 'monitor.secDisk' },
  { key: 'allDisk', labelKey: 'monitor.secAllDisk' },
  { key: 'clock', labelKey: 'monitor.secClock' },
]

function sectionEnabled(key: string): boolean {
  return monitorSectionEnabled(settingsStore.settings.monitorSections, key)
}

function toggleSection(key: string) {
  settingsStore.settings.monitorSections = {
    ...(settingsStore.settings.monitorSections || {}),
    [key]: !sectionEnabled(key),
  }
  settingsStore.save()
}

// Same action as the monitor sidebar's removed pinned button: open the full
// monitor tab beside the owning terminal tab.
function openFullMonitor() {
  const pid = companionStore.activeSshPanelId
  const panel = pid ? panelStore.getPanel(pid) : null
  if (!panel?.config) return
  window.dispatchEvent(new CustomEvent('app:connect-monitor', { detail: { ...panel, sourceTabId: panel.tabId } }))
}
</script>
