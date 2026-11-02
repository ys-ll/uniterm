<template>
  <!-- Tab-strip context menu shared by the sidebar and the bottom panel, so
       both strips offer exactly the same actions. Owns its <Menu> instance
       (like ConnectionContextMenu) instead of filling a host slot — the host
       only keeps a ref and calls openAt(). Top section: move the
       right-clicked tab to the other dock (its current dock is checked;
       hidden when the menu was opened from the strip's empty area). Bottom
       section: toggle which views are visible at all (same
       AppSettings.sidebarTabs as the Settings card), including "connections"
       itself. -->
  <Menu ref="menuRef" v-model:visible="visible">
    <template v-if="targetView">
      <MenuSubmenu v-if="!uiLayout.narrowLayout" :label="t('sidebar.moveTo')">
        <MenuItem
          v-for="loc in DOCKS"
          :key="loc"
          iconic
          :icon="currentLocation === loc ? Check : undefined"
          @click="moveTo(loc)"
        >{{ t(loc === 'sidebar' ? 'sidebar.locSidebar' : 'sidebar.locBottom') }}</MenuItem>
      </MenuSubmenu>
      <MenuDivider v-if="!uiLayout.narrowLayout" />
    </template>
    <MenuItem
      v-for="tab in SIDEBAR_TAB_ORDER"
      :key="tab.key"
      iconic
      :icon="tabVisible(tab.key) ? Check : undefined"
      @click="onTabVisibilityClick(tab.key)"
    >{{ t(tab.labelKey) }}</MenuItem>
  </Menu>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Check } from '@lucide/vue'
import { useI18n } from '../i18n'
import { useUiLayoutStore, type PanelLocation } from '../stores/uiLayoutStore'
import { useSettingsStore } from '../stores/settingsStore'
import { SIDEBAR_TAB_DEFAULTS, SIDEBAR_TAB_ORDER } from '../types/settings'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import MenuSubmenu from './MenuSubmenu.vue'
import MenuDivider from './MenuDivider.vue'

const { t } = useI18n()
const uiLayout = useUiLayoutStore()
const settingsStore = useSettingsStore()

const DOCKS: PanelLocation[] = ['sidebar', 'bottom']

const visible = ref(false)
const menuRef = ref<InstanceType<typeof Menu> | null>(null)

// The tab the menu's "move to" acts on: set by the host when the user
// right-clicks a specific tab button (undefined → submenu hidden, e.g.
// right-click on the strip's empty area).
const targetView = ref<string | undefined>(undefined)

const currentLocation = computed(() => (targetView.value ? uiLayout.panelLocation(targetView.value) : null))

function openAt(x: number, y: number, view?: string) {
  targetView.value = view
  menuRef.value?.openAt(x, y)
}

function moveTo(loc: PanelLocation) {
  if (!targetView.value || currentLocation.value === loc) return
  uiLayout.movePanel(targetView.value, loc, Number.MAX_SAFE_INTEGER)
  // The move is a one-shot action — close the menu once done.
  visible.value = false
}

function tabVisible(key: string): boolean {
  return settingsStore.settings.sidebarTabs?.[key] ?? SIDEBAR_TAB_DEFAULTS[key] ?? true
}

// Show/hide the view's tab everywhere; the store handles active-view
// fallbacks when hiding.
function onTabVisibilityClick(key: string) {
  uiLayout.setViewVisible(key, !tabVisible(key))
}

defineExpose({ openAt })
</script>
