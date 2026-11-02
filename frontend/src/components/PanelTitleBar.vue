<template>
  <!-- Per-panel title bar. In the left sidebar (default): panel title + a
       right-aligned "more" button. In the bottom panel (button-mode): just
       the more button, which the host places next to its close button — the
       tab already shows the panel's name there. The more menu's default
       items: 移动到 (same as the tab-strip right-click) and 隐藏面板. -->
  <div v-if="!buttonMode" class="panel-title-bar">
    <span class="panel-title" :title="title">{{ title }}</span>
    <button class="panel-more-btn" :title="t('sidebar.more')" @click.stop="openMenu">
      <el-icon><MoreHorizontal :size="'0.875rem'" /></el-icon>
    </button>
  </div>
  <button v-else class="panel-more-btn" :title="t('sidebar.more')" @click.stop="openMenu">
    <el-icon><MoreHorizontal :size="'0.875rem'" /></el-icon>
  </button>

  <Menu ref="menuRef" v-model:visible="visible">
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
    <MenuItem iconic @click="hidePanel">{{ t('sidebar.hidePanel') }}</MenuItem>
  </Menu>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Check, MoreHorizontal } from '@lucide/vue'
import { useI18n } from '../i18n'
import { useUiLayoutStore, type PanelLocation } from '../stores/uiLayoutStore'
import { SIDEBAR_TAB_ORDER } from '../types/settings'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import MenuSubmenu from './MenuSubmenu.vue'
import MenuDivider from './MenuDivider.vue'

const props = defineProps<{
  view: string
  /** Bottom-panel mode: render only the more button (the tab strip already
   *  labels the view). Default renders the full sidebar title bar. */
  buttonMode?: boolean
}>()

const { t } = useI18n()
const uiLayout = useUiLayoutStore()

const DOCKS: PanelLocation[] = ['sidebar', 'bottom']

const visible = ref(false)
const menuRef = ref<InstanceType<typeof Menu> | null>(null)

const title = computed(() => {
  const entry = SIDEBAR_TAB_ORDER.find(tab => tab.key === props.view)
  return entry ? t(entry.labelKey) : props.view
})

const currentLocation = computed(() => uiLayout.effectiveLocation(props.view))

function openMenu(e: MouseEvent) {
  const el = (e.currentTarget as HTMLElement).getBoundingClientRect()
  menuRef.value?.openAt(el.left, el.bottom + 4)
}

function moveTo(loc: PanelLocation) {
  if (currentLocation.value === loc) return
  uiLayout.movePanel(props.view, loc, Number.MAX_SAFE_INTEGER)
  // One-shot action — close the menu once done.
  visible.value = false
}

function hidePanel() {
  uiLayout.setViewVisible(props.view, false)
  visible.value = false
}
</script>

<style scoped>
.panel-title-bar {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  /* No bottom padding — the panels' own toolbars (search box etc.) carry
     their top padding, same as the sidebar tab strip. */
  padding: 0.625rem 0.875rem 0;
  flex-shrink: 0;
}

.panel-title {
  flex: 1;
  min-width: 0;
  font-size: 0.75rem;
  font-weight: 600;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.panel-more-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.625rem;
  height: 1.625rem;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  transition: all 0.12s ease;
  flex-shrink: 0;
}

.panel-more-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
</style>
