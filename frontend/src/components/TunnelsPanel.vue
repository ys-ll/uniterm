<template>
  <div class="tunnels-panel">
    <!-- Toolbar -->
    <div class="tn-toolbar">
      <el-input
        v-model="searchQuery"
        :placeholder="t('tunnels.searchPlaceholder')"
        clearable
        class="tn-search-input"
      />
      <el-dropdown trigger="click" placement="bottom-end" :teleported="false">
        <button class="tn-icon-btn" :title="t('tunnels.addTunnel')" @click.stop>
          <Plus :size="15" />
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item @click="addTunnel()">{{ t('tunnels.addTunnel') }}</el-dropdown-item>
            <el-dropdown-item @click="addGroup">{{ t('tunnels.addGroup') }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>

    <!-- List -->
    <div class="tn-list">
      <template v-for="group in store.groups" :key="group.id">
        <div
          class="tn-group-header"
          :class="{ 'drag-over': dragOverGroupId === group.id }"
          @click="toggleGroup(group.id)"
          @contextmenu.prevent="onGroupContextMenu($event, group)"
          @dragover.prevent="onGroupDragOver($event, group.id)"
          @dragleave="onGroupDragLeave(group.id)"
          @drop.prevent="onGroupDrop(group.id, $event)"
        >
          <span class="tn-group-arrow">
            <el-icon v-if="expandedGroups.has(group.id)"><ChevronDown :size="14" /></el-icon>
            <el-icon v-else><ChevronRight :size="14" /></el-icon>
          </span>
          <span class="tn-group-name">{{ group.name }}</span>
        </div>
        <template v-if="expandedGroups.has(group.id)">
          <TunnelRow
            v-for="tn in store.getTunnelsByGroup(group.id).filter(matchesSearch)"
            :key="tn.id"
            :tunnel="tn"
            @edit="editTunnel"
            @context="onTunnelContextMenu"
            @dragstart="onTunnelDragStart"
          />
        </template>
      </template>

      <!-- Flat (no groups) -->
      <template v-if="store.groups.length === 0">
        <TunnelRow
          v-for="tn in store.getTunnelsByGroup(undefined).filter(matchesSearch)"
          :key="tn.id"
          :tunnel="tn"
          @edit="editTunnel"
          @context="onTunnelContextMenu"
          @dragstart="onTunnelDragStart"
        />
      </template>

      <!-- Ungrouped virtual group -->
      <template v-if="store.groups.length > 0 && store.getTunnelsByGroup(undefined).filter(matchesSearch).length > 0">
        <div
          class="tn-group-header"
          :class="{ 'drag-over': dragOverGroupId === '__ungrouped__' }"
          @click="toggleGroup('__ungrouped__')"
          @dragover.prevent="onGroupDragOver($event, '__ungrouped__')"
          @dragleave="onGroupDragLeave('__ungrouped__')"
          @drop.prevent="onGroupDrop('__ungrouped__', $event)"
        >
          <span class="tn-group-arrow">
            <el-icon v-if="expandedGroups.has('__ungrouped__')"><ChevronDown :size="14" /></el-icon>
            <el-icon v-else><ChevronRight :size="14" /></el-icon>
          </span>
          <span class="tn-group-name">{{ t('tunnels.noGroup') }}</span>
        </div>
        <template v-if="expandedGroups.has('__ungrouped__')">
          <TunnelRow
            v-for="tn in store.getTunnelsByGroup(undefined).filter(matchesSearch)"
            :key="tn.id"
            :tunnel="tn"
            @edit="editTunnel"
            @context="onTunnelContextMenu"
            @dragstart="onTunnelDragStart"
          />
        </template>
      </template>

      <div v-if="store.tunnels.length === 0" class="tn-empty">{{ t('tunnels.empty') }}</div>
    </div>

    <!-- Context menu -->
    <div v-show="menuVisible" class="tn-context-menu" :style="menuStyle" @click.stop>
      <template v-if="selectedTunnel">
        <div class="menu-item" @click="toggleRun(selectedTunnel!); closeMenu()">
          {{ store.statusOf(selectedTunnel!.id) === 'running' ? t('tunnels.stop') : t('tunnels.start') }}
        </div>
        <div class="menu-divider" />
        <div class="menu-item" @click="editTunnel(selectedTunnel!.id)">{{ t('tunnels.editTunnel') }}</div>
        <div class="menu-item danger" @click="doDeleteTunnel(selectedTunnel!)">{{ t('tunnels.deleteTunnel') }}</div>
      </template>
      <template v-if="selectedGroup">
        <div class="menu-item" @click="addTunnel(selectedGroup!.id)">{{ t('tunnels.addTunnel') }}</div>
        <div class="menu-item" @click="renameGroup(selectedGroup!)">{{ t('tunnels.renameGroup') }}</div>
        <div class="menu-item danger" @click="deleteGroupDialog(selectedGroup!)">{{ t('tunnels.deleteGroup') }}</div>
      </template>
    </div>

    <!-- Delete group dialog -->
    <el-dialog v-model="deleteGroupDialogVisible" :title="t('tunnels.deleteGroupTitle')" width="400px" :close-on-click-modal="false">
      <p>{{ t('tunnels.deleteGroupDesc') }}</p>
      <div class="delete-group-actions">
        <el-button @click="doDeleteGroup(false)">{{ t('tunnels.moveToUngrouped') }}</el-button>
        <el-button type="danger" @click="doDeleteGroup(true)">{{ t('tunnels.deleteTunnels') }}</el-button>
      </div>
    </el-dialog>

    <!-- Group name dialog -->
    <el-dialog v-model="groupNameDialogVisible" :title="renamingGroup ? t('tunnels.renameGroup') : t('tunnels.addGroup')" width="360px" :close-on-click-modal="false">
      <el-input v-model="groupNameInput" :placeholder="t('tunnels.groupName')" maxlength="30" @keyup.enter="doSaveGroupName" />
      <template #footer>
        <el-button @click="groupNameDialogVisible = false">{{ t('tunnels.cancel') }}</el-button>
        <el-button type="primary" :disabled="!groupNameInput.trim()" @click="doSaveGroupName">{{ t('tunnels.save') }}</el-button>
      </template>
    </el-dialog>

    <TunnelEditDialog v-model="editDialogVisible" :editing-id="editingTunnelId" :initial-group-id="editingGroupId" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Plus, ChevronDown, ChevronRight } from '@lucide/vue'
import { useTunnelStore, type Tunnel, type TunnelGroup } from '../stores/tunnelStore'
import { useConnectionStore } from '../stores/connectionStore'
import { useI18n } from '../i18n'
import { msg } from '../services/message'
import TunnelEditDialog from './TunnelEditDialog.vue'
import TunnelRow from './TunnelRow.vue'

const { t } = useI18n()
const store = useTunnelStore()
const connectionStore = useConnectionStore()

const searchQuery = ref('')
const expandedGroups = ref<Set<string>>(new Set())
const dragOverGroupId = ref<string | null>(null)

const menuVisible = ref(false)
const menuStyle = ref({ left: '0px', top: '0px' })
const selectedTunnel = ref<Tunnel | null>(null)
const selectedGroup = ref<TunnelGroup | null>(null)

const deleteGroupDialogVisible = ref(false)
const deletingGroup = ref<TunnelGroup | null>(null)
const groupNameDialogVisible = ref(false)
const groupNameInput = ref('')
const renamingGroup = ref<TunnelGroup | null>(null)

const editDialogVisible = ref(false)
const editingTunnelId = ref<string | undefined>(undefined)
const editingGroupId = ref<string | undefined>(undefined)

onMounted(async () => {
  await store.load()
  await connectionStore.load()
  store.groups.forEach(g => expandedGroups.value.add(g.id))
  expandedGroups.value.add('__ungrouped__')
  document.addEventListener('click', closeMenu)
  window.addEventListener('global:close-context-menus', closeMenu)
})
onUnmounted(() => {
  document.removeEventListener('click', closeMenu)
  window.removeEventListener('global:close-context-menus', closeMenu)
})

function closeMenu() { menuVisible.value = false }
function clampMenuPosition(x: number, y: number) {
  const mx = Math.min(x, window.innerWidth - 160)
  const my = Math.min(y, window.innerHeight - 100)
  return { left: mx + 'px', top: my + 'px' }
}
function toggleGroup(id: string) {
  if (expandedGroups.value.has(id)) expandedGroups.value.delete(id)
  else expandedGroups.value.add(id)
}
function matchesSearch(tn: Tunnel): boolean {
  if (!searchQuery.value.trim()) return true
  return tn.name.toLowerCase().includes(searchQuery.value.toLowerCase())
}

async function toggleRun(tn: Tunnel) {
  if (store.statusOf(tn.id) === 'running') {
    await store.stop(tn.id)
  } else {
    const st = await store.start(tn.id)
    if (st.status === 'error') msg.error(st.error || t('tunnels.startFailed'))
  }
}

function onTunnelContextMenu(e: MouseEvent, tn: Tunnel) {
  e.stopPropagation()
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  selectedTunnel.value = tn
  selectedGroup.value = null
  menuStyle.value = clampMenuPosition(e.clientX, e.clientY)
  menuVisible.value = true
}
function onGroupContextMenu(e: MouseEvent, group: TunnelGroup) {
  e.stopPropagation()
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  selectedGroup.value = group
  selectedTunnel.value = null
  menuStyle.value = clampMenuPosition(e.clientX, e.clientY)
  menuVisible.value = true
}

function editTunnel(id: string) {
  editingTunnelId.value = id
  editingGroupId.value = undefined
  editDialogVisible.value = true
  closeMenu()
}
function addTunnel(groupId?: string) {
  editingTunnelId.value = undefined
  editingGroupId.value = groupId
  editDialogVisible.value = true
  closeMenu()
}
function doDeleteTunnel(tn: Tunnel) {
  store.deleteTunnel(tn.id)
  closeMenu()
}

function addGroup() {
  renamingGroup.value = null
  groupNameInput.value = ''
  groupNameDialogVisible.value = true
}
function renameGroup(group: TunnelGroup) {
  renamingGroup.value = group
  groupNameInput.value = group.name
  groupNameDialogVisible.value = true
  closeMenu()
}
function doSaveGroupName() {
  const name = groupNameInput.value.trim()
  if (!name) return
  if (renamingGroup.value) store.renameGroup(renamingGroup.value.id, name)
  else {
    const g = store.addGroup(name)
    expandedGroups.value.add(g.id)
  }
  groupNameDialogVisible.value = false
}
function deleteGroupDialog(group: TunnelGroup) {
  deletingGroup.value = group
  deleteGroupDialogVisible.value = true
  closeMenu()
}
function doDeleteGroup(deleteTunnels: boolean) {
  if (deletingGroup.value) store.deleteGroup(deletingGroup.value.id, deleteTunnels)
  deleteGroupDialogVisible.value = false
  deletingGroup.value = null
}

// Drag & drop between groups
function onTunnelDragStart(e: DragEvent, tn: Tunnel) {
  if (!e.dataTransfer) return
  e.dataTransfer.setData('application/tn-id', tn.id)
  e.dataTransfer.effectAllowed = 'move'
}
function onGroupDragOver(e: DragEvent, groupId: string) {
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  dragOverGroupId.value = groupId
}
function onGroupDragLeave(groupId: string) {
  if (dragOverGroupId.value === groupId) dragOverGroupId.value = null
}
function onGroupDrop(groupId: string, e: DragEvent) {
  e.preventDefault()
  dragOverGroupId.value = null
  const id = e.dataTransfer?.getData('application/tn-id')
  if (!id) return
  const targetGroupId = groupId === '__ungrouped__' ? undefined : groupId
  store.updateTunnel(id, { groupId: targetGroupId })
}
</script>

<style scoped>
.tunnels-panel { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
.tn-toolbar { display: flex; align-items: center; gap: 4px; padding: 0 10px 6px; flex-shrink: 0; border-bottom: 1px solid var(--border-subtle); }
.tn-search-input { flex: 1; min-width: 0; }
.tn-icon-btn { width: 26px; height: 26px; display: flex; align-items: center; justify-content: center; border: none; border-radius: 4px; background: transparent; color: var(--text-muted); cursor: pointer; flex-shrink: 0; }
.tn-icon-btn:hover { color: var(--text-primary); background: var(--bg-hover); }
.tn-list { flex: 1; overflow-y: auto; padding: 0 8px 8px; }
.tn-group-header { display: flex; align-items: center; gap: 4px; padding: 6px 10px 6px 6px; cursor: pointer; user-select: none; border-radius: var(--radius-sm); font-family: var(--font-ui); font-size: 12px; color: var(--text-secondary); }
.tn-group-header:hover { background: var(--bg-hover); }
.tn-group-header.drag-over { background: var(--accent-subtle); box-shadow: inset 0 0 0 1px var(--accent); }
.tn-group-arrow { display: inline-flex; align-items: center; width: 16px; color: var(--text-disabled); }
.tn-group-name { font-weight: 600; flex: 1; }
.tn-empty { padding: 24px 12px; text-align: center; color: var(--text-muted); font-size: 12px; }
.tn-context-menu { position: fixed; z-index: 9999; background: var(--bg-surface); border: 1px solid var(--border-subtle); border-radius: 6px; box-shadow: var(--shadow-lg); padding: 4px; min-width: 140px; }
.tn-context-menu .menu-item { padding: 6px 10px; font-size: 12px; border-radius: 4px; cursor: pointer; color: var(--text-primary); }
.tn-context-menu .menu-item:hover { background: var(--bg-hover); }
.tn-context-menu .menu-item.danger { color: var(--error); }
.tn-context-menu .menu-divider { height: 1px; background: var(--border-subtle); margin: 4px 6px; }
.delete-group-actions { display: flex; gap: 8px; margin-top: 12px; }
</style>
