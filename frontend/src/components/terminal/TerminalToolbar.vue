<template>
  <!-- Terminal context menu -->
  <Menu ref="terminalMenuRef" v-model:visible="terminalMenuVisible">
    <!-- ① 剪贴板 -->
    <MenuItem :class="{ disabled: !menu.hasSelection.value }" :shortcut="menuShortcut('copy')" @click="menu.copySelection">
      {{ t('terminal.copy') }}
    </MenuItem>
    <MenuItem :class="{ disabled: !menu.hasSelection.value }" @click="menu.copySingleLine">{{ t('terminal.copySingleLine') }}</MenuItem>
    <MenuItem :class="{ disabled: !menu.hasSelection.value }" @click="menu.copyAndPaste">
      {{ t('terminal.copyAndPaste') }}
    </MenuItem>
    <MenuItem :shortcut="menuShortcut('paste')" @click="menu.pasteFromClipboard">
      {{ t('terminal.paste') }}
    </MenuItem>
    <MenuItem @click="menu.selectAll">
      {{ t('terminal.selectAll') }}
    </MenuItem>
    <MenuItem :class="{ disabled: !menu.hasSelection.value }" @click="menu.askAI">
      {{ t('terminal.askAI') }}
    </MenuItem>

    <!-- ② 会话文本操作 -->
    <MenuDivider />
    <MenuItem :shortcut="menuShortcut('terminalSearch')" @click="onTriggerSearch">
      {{ t('terminal.searchText') }}
    </MenuItem>
    <MenuItem @click="onExport">{{ t('terminal.export') }}</MenuItem>
    <MenuItem @click="onResetOutput">{{ t('terminal.resetOutput') }}</MenuItem>
    <MenuItem @click="onClearScrollback">{{ t('terminal.clearScrollback') }}</MenuItem>
    <MenuItem :shortcut="menuShortcut('toggleLineNumbers')" @click="emit('toggleLineNumbers')">
      {{ showLineNumbers ? t('settings.hideLineNumbers') : t('settings.showLineNumbers') }}
    </MenuItem>
    <MenuItem :shortcut="menuShortcut('toggleTimestamps')" @click="emit('toggleTimestamps')">
      {{ showTimestamps ? t('settings.hideTimestamps') : t('settings.showTimestamps') }}
    </MenuItem>
    <MenuItem v-if="supportsOutputLog" @click="emit('toggleOutputLog')">
      {{ isOutputLogOn ? t('session.stopLog') : t('session.startLog') }}
    </MenuItem>
    <MenuItem v-if="supportsOutputLog && isOutputLogOn" @click="emit('openLogDir')">
      {{ t('session.openLogDir') }}
    </MenuItem>

    <!-- ③ SSH 连接功能 -->
    <MenuDivider />
    <MenuItem v-if="isSsh" @click="emit('openSftp')">{{ t(connectFileMenuKey(panel?.config)) }}</MenuItem>
    <MenuItem v-if="isSsh" @click="emit('uploadFileRz')">{{ t('terminal.uploadFileRz') }}</MenuItem>
    <MenuItem v-if="isSsh" @click="emit('openMonitor')">{{ t('sidebar.connectMonitor') }}</MenuItem>
  </Menu>

  <!-- Gutter context menu — right-click on the line-number/time columns -->
  <Menu ref="gutterMenuRef" v-model:visible="gutterMenuVisible">
    <MenuItem :shortcut="menuShortcut('toggleLineNumbers')" @click="emit('toggleLineNumbers')">
      {{ showLineNumbers ? t('settings.hideLineNumbers') : t('settings.showLineNumbers') }}
    </MenuItem>
    <MenuItem :shortcut="menuShortcut('toggleTimestamps')" @click="emit('toggleTimestamps')">
      {{ showTimestamps ? t('settings.hideTimestamps') : t('settings.showTimestamps') }}
    </MenuItem>
  </Menu>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import type { UseTerminalMenuReturn } from '../../composables/useTerminalMenu'
import type { ShortcutAction } from '../../types/settings'
import type { Panel } from '../../types/workspace'
import { connectFileMenuKey } from '../../utils/fileTransferUtils'
import { useI18n } from '../../i18n'
import Menu from '../Menu.vue'
import MenuItem from '../MenuItem.vue'
import MenuDivider from '../MenuDivider.vue'

const props = defineProps<{
  menu: UseTerminalMenuReturn
  menuShortcut: (action: ShortcutAction) => string
  showLineNumbers: boolean
  showTimestamps: boolean
  isSsh: boolean
  supportsOutputLog: boolean
  isOutputLogOn: boolean
  panel: Panel | undefined
}>()

const emit = defineEmits<{
  toggleLineNumbers: []
  toggleTimestamps: []
  triggerSearch: []
  export: []
  resetOutput: []
  clearScrollback: []
  toggleOutputLog: []
  openLogDir: []
  openSftp: []
  uploadFileRz: []
  openMonitor: []
}>()

const { t } = useI18n()

const terminalMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const gutterMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const gutterMenuVisible = ref(false)

// `menu.menuVisible` is a ref nested in a plain object, so the template would
// not auto-unwrap it — route v-model through a top-level computed so get/set
// both hit the real boolean.
const terminalMenuVisible = computed({
  get: () => props.menu.menuVisible.value,
  set: (v: boolean) => { props.menu.menuVisible.value = v },
})

// Menu items that need to close the menu before emitting an action.
function onTriggerSearch() { props.menu.closeMenu(); emit('triggerSearch') }
function onExport() { props.menu.closeMenu(); emit('export') }
function onResetOutput() { props.menu.closeMenu(); emit('resetOutput') }
function onClearScrollback() { props.menu.closeMenu(); emit('clearScrollback') }

defineExpose({
  terminalMenuRef,
  gutterMenuRef,
  gutterMenuVisible,
})
</script>