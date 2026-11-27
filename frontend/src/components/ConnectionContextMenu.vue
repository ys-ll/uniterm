<template>
  <Menu ref="menuRef" v-model:visible="visible">
    <MenuItem v-if="connectMenuLabelKey" @click="onConnectClick($event)">{{ t(connectMenuLabelKey) }}</MenuItem>
    <MenuSubmenu v-if="isWorkspaceHostType(config?.type)" :label="t('sidebar.connectToWorkspace')">
      <MenuItem iconic :icon="Plus" @click="onCreateWorkspace">{{ t('workspace.newWorkspace') }}</MenuItem>
      <MenuItem v-for="w in workspaceTabs" :key="w.id" @click="onConnectToWorkspace(w.id)">{{ w.name }}</MenuItem>
    </MenuSubmenu>
    <!-- Companion views: session type is not derivable from config.type.
         Hidden when the connection disables file transfer ('none'). -->
    <MenuItem v-if="config?.type === 'ssh' && connectFileMenuKey(config)" @click="onConnectClick($event, 'file')">{{ t(connectFileMenuKey(config)) }}</MenuItem>
    <MenuItem v-if="config?.type === 'wsl'" @click="onConnectClick($event, 'wsl-file')">{{ t('sidebar.connectWslFile') }}</MenuItem>
    <MenuItem v-if="config?.type === 'ssh'" @click="onConnectClick($event, 'monitor')">{{ t('sidebar.connectMonitor') }}</MenuItem>
    <!-- Launch the OS-native mstsc client instead of the built-in RDP one (#1520). -->
    <MenuItem v-if="config?.type === 'rdp' && isWindows" @click="onLaunchMstsc">{{ t('sidebar.openInMstsc') }}</MenuItem>
    <MenuItem v-if="config && canLocate" @click="locateSession">{{ t('sidebar.locateSession') }}</MenuItem>
    <MenuItem v-if="canBatchExecute" @click="onBatchExecute">{{ t('sidebar.batchExecute') }}</MenuItem>
    <MenuItem v-if="canViewTotpCode" @click="onViewTotpCode">{{ t('conn.totpViewCode') }}</MenuItem>
    <MenuDivider />
    <MenuItem :class="{ disabled: targets.length > 1 }" @click="targets.length === 1 && onEditClick()">{{ t('sidebar.edit') }}</MenuItem>
    <MenuItem @click="duplicate()">{{ t('sidebar.duplicate') }}</MenuItem>
    <MenuItem v-if="config" @click="favoriteStore.toggle(config.id)">{{ isFavorite ? t('sidebar.removeFromFavorites') : t('sidebar.addToFavorites') }}</MenuItem>
    <MenuDivider />
    <MenuItem @click="close(); emit('changeGroup', targets)">{{ t('conn.moveTo') }}</MenuItem>
    <MenuItem @click="close(); emit('newGroup')">{{ t('conn.newGroupTitle') }}</MenuItem>
    <MenuDivider />
    <MenuItem class="danger" @click="onDeleteClick">{{ t('sidebar.delete') }}</MenuItem>
  </Menu>

  <!-- Batch execute dialog (opened from the context menu) -->
  <BatchExecuteDialog v-model="batchDialogVisible" :hosts="batchHosts" />
  <!-- Live TOTP code for bastion web-console binding (issue #1583) -->
  <TotpCodeDialog
    v-model:visible="totpDialogVisible"
    :secret="totpConfig?.totpSecret || ''"
    :host-name="totpConfig?.name"
  />
</template>

<script setup lang="ts">
// Shared connection context menu for the sidebar list and the start page.
// Owns all type metadata (registry labels, companion items, workspace
// submenu), favorite state and duplicate/delete flows; parents only wire the
// connect routing and the edit/group dialogs.
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@lucide/vue'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import MenuDivider from './MenuDivider.vue'
import MenuSubmenu from './MenuSubmenu.vue'
import BatchExecuteDialog from './BatchExecuteDialog.vue'
import TotpCodeDialog from './TotpCodeDialog.vue'
import type { ConnectionConfig } from '../types/session'
import { CONNECTION_TYPES } from '../utils/connectionTypes'
import { connectFileMenuKey } from '../utils/fileTransferUtils'
import { useI18n } from '../i18n'
import { useFavoriteStore } from '../stores/favoriteStore'
import { usePanelStore } from '../stores/panelStore'
import { useTabStore } from '../stores/tabStore'
import { useConnectionStore } from '../stores/connectionStore'
import { isWorkspaceHostType } from '../composables/createWorkspace'
import { openWorkspaceNameDialog } from '../composables/workspaceNameDialog'
import { GetPlatform, LaunchMstsc } from '../../bindings/github.com/ys-ll/uniterm/app'

// kind selects a companion view (file browser / monitor / WSL files) whose
// session type is not derivable from config.type.
export type ConnectKind = 'file' | 'wsl-file' | 'monitor'

const props = defineProps<{
  // The connection that was right-clicked (drives the type-specific items).
  config: ConnectionConfig | null
  // Action targets: the resolved selection, guaranteed to include `config`.
  targets: ConnectionConfig[]
  visible: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'connect', targets: ConnectionConfig[], kind: ConnectKind | undefined, event?: MouseEvent): void
  (e: 'connectToWorkspace', targets: ConnectionConfig[], workspaceId: string): void
  (e: 'createWorkspace', targets: ConnectionConfig[]): void
  (e: 'edit', config: ConnectionConfig): void
  (e: 'changeGroup', targets: ConnectionConfig[]): void
  (e: 'newGroup'): void
  (e: 'delete', targets: ConnectionConfig[]): void
}>()

const { t } = useI18n()
const favoriteStore = useFavoriteStore()
const panelStore = usePanelStore()
const tabStore = useTabStore()
const connectionStore = useConnectionStore()

const menuRef = ref<InstanceType<typeof Menu> | null>(null)
defineExpose({ openAt: (x: number, y: number, data?: unknown) => menuRef.value?.openAt(x, y, data) })

// mstsc only exists on Windows; the item is hidden on every other platform.
const isWindows = ref(false)
onMounted(() => {
  GetPlatform().then(p => { isWindows.value = p === 'windows' })
})

const visible = computed({
  get: () => props.visible,
  set: v => emit('update:visible', v),
})

const workspaceTabs = computed(() => tabStore.tabs.filter(tab => tab.type === 'workspace'))

const connectMenuLabelKey = computed(() => {
  const type = props.config?.type
  return type ? CONNECTION_TYPES.find(i => i.type === type && i.connectMenuKey)?.connectMenuKey : undefined
})

// Whether「定位会话」has a target: a panel holding the connection, or — for a
// saved workspace — a live workspace tab linked to the record.
const canLocate = computed(() => {
  const id = props.config?.id
  if (!id) return false
  if (props.config.type === 'workspace') {
    return tabStore.tabs.some(tb => tb.type === 'workspace' && tb.savedWorkspaceId === id)
  }
  for (const p of panelStore.panels.values()) {
    if (p.config?.id === id) return true
  }
  return false
})

const isFavorite = computed(() => (props.config ? favoriteStore.isFavorite(props.config.id) : false))

// Batch execute: shown when ≥ 2 selected hosts are SSH or local (the only
// types BatchExecuteDialog supports). Non-supported types are filtered by the
// dialog itself, but we gate the menu item here so it only appears when it
// would be useful.
const batchHosts = computed(() =>
  props.targets.filter(h => h.type === 'ssh' || h.type === 'local'),
)
const canBatchExecute = computed(() => batchHosts.value.length >= 2)
const batchDialogVisible = ref(false)

// Only offer the live code for a single saved host that already has a secret;
// the edit form covers the not-yet-saved case. Keep a copy of the config since
// closing the menu clears the parent's selection.
const canViewTotpCode = computed(() =>
  props.targets.length === 1 && !!props.config?.totpEnabled && !!props.config?.totpSecret,
)
const totpDialogVisible = ref(false)
const totpConfig = ref<ConnectionConfig | null>(null)

function onViewTotpCode() {
  const c = props.config
  if (!c) return
  totpConfig.value = c
  close()
  totpDialogVisible.value = true
}

function onBatchExecute() {
  close()
  batchDialogVisible.value = true
}

function close() {
  emit('update:visible', false)
}

function onConnectClick(event: MouseEvent, kind?: ConnectKind) {
  close()
  emit('connect', props.targets, kind, event)
}

// Open the right-clicked RDP connection in the Windows Remote Desktop client.
// The backend pre-fills address/username/domain; mstsc prompts for the
// password itself, so no credential ever leaves through this path.
async function onLaunchMstsc() {
  close()
  const c = props.config
  if (!c) return
  try {
    await LaunchMstsc(c)
  } catch (err) {
    ElMessage.error(String(err))
  }
}

function onConnectToWorkspace(workspaceId: string) {
  close()
  emit('connectToWorkspace', props.targets, workspaceId)
}

// 新建工作区 from the current targets (multi-selection or right-clicked row).
function onCreateWorkspace() {
  close()
  emit('createWorkspace', props.targets)
}

// Edit dispatches on kind: saved workspaces get the name+group dialog;
// everything else opens the connection form.
function onEditClick() {
  if (props.config?.type === 'workspace') {
    void onEditSavedWorkspace()
    return
  }
  close()
  emit('edit', props.targets[0])
}

// Edit a saved workspace record (name + group); keeps any live tab linked to
// it in sync on rename.
async function onEditSavedWorkspace() {
  close()
  const c = props.config
  if (!c) return
  const result = await openWorkspaceNameDialog({
    title: t('sidebar.edit'),
    initialName: c.name,
    initialGroupId: c.groupId
  })
  if (!result) return
  const { name, groupId } = result
  if (name === c.name && groupId === c.groupId) return
  await connectionStore.update(c.id, { name, groupId })
  for (const tb of tabStore.tabs) {
    if (tb.type === 'workspace' && tb.savedWorkspaceId === c.id) tabStore.renameTab(tb.id, name)
  }
}

// Switch to the existing tab/session hosting this connection. For a saved
// workspace the target is its linked live workspace tab.
function locateSession() {
  close()
  const id = props.config?.id
  if (!id) return
  if (props.config.type === 'workspace') {
    const tab = tabStore.tabs.find(tb => tb.type === 'workspace' && tb.savedWorkspaceId === id)
    if (tab) tabStore.setActiveTab(tab.id)
    return
  }
  for (const p of panelStore.panels.values()) {
    if (p.config?.id !== id) continue
    const tab = tabStore.tabs.find(tb => tb.id === p.tabId)
    if (!tab) continue
    tabStore.setActiveTab(tab.id)
    if (tab.type === 'workspace') {
      tabStore.setActivePanel(tab.id, p.id)
    }
    break
  }
}

function generateDuplicateName(name: string): string {
  const match = name.match(/^(.*)\s*\((\d+)\)$/)
  const base = match ? match[1].trim() : name
  const re = new RegExp('^' + escapeRegex(base) + '\s*\(\d+\)$')
  let maxNum = 0
  for (const c of connectionStore.connections) {
    if (c.name === base || re.test(c.name)) {
      const m = c.name.match(/\((\d+)\)$/)
      if (m) {
        maxNum = Math.max(maxNum, parseInt(m[1], 10))
      } else {
        maxNum = Math.max(maxNum, 0)
      }
    }
  }
  return `${base} (${maxNum + 1})`
}

function escapeRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

async function duplicate() {
  close()
  for (const c of props.targets) {
    const dup: ConnectionConfig = {
      ...c,
      id: `conn-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      name: generateDuplicateName(c.name)
    }
    connectionStore.add(dup)
  }
}

// Confirms internally, then hands the targets to the parent for removal.
async function onDeleteClick() {
  close()
  const ids = props.targets.map(c => c.id)
  try {
    await ElMessageBox.confirm(
      t('sidebar.deleteConfirm', { count: ids.length }),
      t('sidebar.delete'),
      { confirmButtonText: t('sftp.dialog.confirm'), cancelButtonText: t('sftp.dialog.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  emit('delete', props.targets)
}
</script>
