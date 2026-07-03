<template>
  <div
    ref="sidebarEl"
    class="sidebar"
    :class="{ collapsed: !visible, resizing: isResizing }"
    :style="{ width: sidebarWidth + 'px' }"
  >
    <div class="resize-handle" @mousedown="onResizeStart" />
    <div class="sidebar-header">
      <button class="sidebar-tab" :class="{ active: activeView === 'connections' }" @click="activeView = 'connections'" :title="t('header.connections')"><el-icon><Network :size="14" /></el-icon></button>
      <button class="sidebar-tab" :class="{ active: activeView === 'quickCommands' }" @click="activeView = 'quickCommands'" :title="t('quickCommands.quickCommandsTab')"><el-icon><Zap :size="14" /></el-icon></button>
      <button class="sidebar-tab" :class="{ active: activeView === 'history' }" @click="activeView = 'history'" :title="t('quickCommands.historyTab')"><el-icon><Clock :size="14" /></el-icon></button>
      <button class="sidebar-tab" :class="{ active: activeView === 'personalization' }" @click="activeView = 'personalization'" :title="t('sidebar.personalization')"><el-icon><Palette :size="14" /></el-icon></button>
      <button class="icon-btn" @click="emit('toggle')" :title="t('sidebar.collapse')"><el-icon><X :size="14" /></el-icon></button>
    </div>

    <template v-if="activeView === 'connections'">
      <div class="search-box">
        <el-input
          ref="searchInputRef"
          v-model="searchQuery"
          :placeholder="t('sidebar.searchPlaceholder')"
          clearable
          @keydown="onListKeydown"
        >
          <template #suffix>
            <el-dropdown trigger="click" placement="bottom-end" :teleported="false">
              <span class="filter-trigger" :class="{ active: selectedTypeFilter !== 'all' }" @click.stop>
                <el-icon><Filter :size="14" /></el-icon>
              </span>
              <template #dropdown>
                <el-dropdown-menu class="type-filter-menu">
                  <el-dropdown-item
                    :class="{ 'is-active': selectedTypeFilter === 'all' }"
                    @click="selectedTypeFilter = 'all'"
                  >
                    <span class="dropdown-item-content">
                      <el-icon v-if="selectedTypeFilter === 'all'"><Check :size="14" /></el-icon>
                      <span v-else class="check-placeholder"></span>
                      <span>{{ t('sidebar.filterAll') }}</span>
                    </span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    v-for="typeOpt in availableTypes"
                    :key="typeOpt.value"
                    :class="{ 'is-active': selectedTypeFilter === typeOpt.value }"
                    @click="selectedTypeFilter = typeOpt.value"
                  >
                    <span class="dropdown-item-content">
                      <el-icon v-if="selectedTypeFilter === typeOpt.value"><Check :size="14" /></el-icon>
                      <span v-else class="check-placeholder"></span>
                      <span>{{ typeOpt.label }}</span>
                    </span>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-input>
        <el-dropdown trigger="click" placement="bottom-end" :teleported="false" popper-class="new-conn-popper" @command="onNewConnCommand" @visible-change="onNewConnVisibleChange">
          <button class="sb-icon-btn" :title="t('header.newConnection')" @click.stop>
            <Plus :size="15" />
          </button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="new-connection">{{ t('header.newConnection') }}</el-dropdown-item>
              <el-dropdown-item command="new-group">{{ t('conn.newGroupTitle') }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
      <Teleport to="body">
        <div
          v-show="showShellSubmenu"
          class="shell-submenu"
          :style="shellSubmenuStyle"
          @mouseenter="showShellSubmenu = true"
          @mouseleave="showShellSubmenu = false"
        >
          <div
            v-for="sh in settingsStore.availableShells"
            :key="sh"
            class="shell-item"
            @click="onShellSelect(sh)"
          >
            {{ getShellLabel(sh) }}
          </div>
        </div>
      </Teleport>

    <div class="connection-list" tabindex="0" @keydown="onListKeydown" @contextmenu.prevent="onEmptyAreaContextMenu">
      <!-- Grouped connections -->
      <template v-for="entry in filteredGrouped.groups" :key="entry.group.id">
        <div
          class="group-header"
          :class="{ 'drag-over': dragOverGroupId === entry.group.id }"
          @click="toggleGroup(entry.group.id)"
          @contextmenu.prevent="onGroupContextMenu($event, entry.group)"
          @dragover.prevent="onGroupDragOver(entry.group.id)"
          @dragleave="onGroupDragLeave(entry.group.id)"
          @drop.prevent="onGroupDrop(entry.group.id, $event)"
        >
          <span class="group-arrow">
            <el-icon v-if="expandedGroups.has(entry.group.id)"><ChevronDown :size="14" /></el-icon>
            <el-icon v-else><ChevronRight :size="14" /></el-icon>
          </span>
          <span class="group-name">{{ entry.group.name }}</span>
        </div>
        <template v-if="expandedGroups.has(entry.group.id)">
          <div
            v-for="conn in entry.connections"
            :key="conn.id"
            class="connection-item indented"
            :class="{
              active: selectedIds.has(conn.id)
            }"
            draggable="true"
            @dragstart="onDragStart($event, conn)"
            @dragend="onDragEnd"
            @click="onItemClick($event, conn)"
            @dblclick="onItemDblClick(conn)"
            @contextmenu.prevent="onContextMenu($event, conn)"
          >
            <span class="conn-icon"><component :is="connIcon(conn)" :size="14" /></span>
            <div class="conn-details">
              <span class="name">{{ conn.name }}</span>
              <span class="conn-meta">
                <span class="host">{{ getSubtitle(conn) }}</span>
              </span>
            </div>
          </div>
        </template>
      </template>

      <!-- Virtual (No Group) group - only when real groups exist -->
      <template v-if="connectionStore.groups.length > 0 && filteredGrouped.ungrouped.length > 0">
        <div
          class="group-header"
          :class="{ 'drag-over': dragOverGroupId === '__ungrouped__' }"
          @click="toggleGroup('__ungrouped__')"
          @contextmenu.prevent="onVirtualGroupContextMenu($event)"
          @dragover.prevent="onGroupDragOver('__ungrouped__')"
          @dragleave="onGroupDragLeave('__ungrouped__')"
          @drop.prevent="onGroupDrop('__ungrouped__', $event)"
        >
          <span class="group-arrow">
            <el-icon v-if="expandedGroups.has('__ungrouped__')"><ChevronDown :size="14" /></el-icon>
            <el-icon v-else><ChevronRight :size="14" /></el-icon>
          </span>
          <span class="group-name">{{ t('conn.noGroup') }}</span>
        </div>
        <template v-if="expandedGroups.has('__ungrouped__')">
          <div
            v-for="conn in filteredGrouped.ungrouped"
            :key="conn.id"
            class="connection-item indented"
            :class="{
              active: selectedIds.has(conn.id)
            }"
            draggable="true"
            @dragstart="onDragStart($event, conn)"
            @dragend="onDragEnd"
            @click="onItemClick($event, conn)"
            @dblclick="onItemDblClick(conn)"
            @contextmenu.prevent="onContextMenu($event, conn)"
          >
            <span class="conn-icon"><component :is="connIcon(conn)" :size="14" /></span>
            <div class="conn-details">
              <span class="name">{{ conn.name }}</span>
              <span class="conn-meta">
                <span class="host">{{ getSubtitle(conn) }}</span>
              </span>
            </div>
          </div>
        </template>
      </template>

      <!-- Flat ungrouped connections (only when no real groups exist) -->
      <template v-if="connectionStore.groups.length === 0">
        <div
          v-for="conn in filteredGrouped.ungrouped"
          :key="conn.id"
          class="connection-item"
          :class="{
            active: selectedIds.has(conn.id)
          }"
          draggable="true"
          @dragstart="onDragStart($event, conn)"
          @dragend="onDragEnd"
          @click="onItemClick($event, conn)"
          @dblclick="onItemDblClick(conn)"
          @contextmenu.prevent="onContextMenu($event, conn)"
        >
          <span class="conn-icon"><component :is="connIcon(conn)" :size="14" /></span>
          <div class="conn-details">
            <span class="name">{{ conn.name }}</span>
            <span class="conn-meta">
              <span class="host">{{ getSubtitle(conn) }}</span>
            </span>
          </div>
        </div>
      </template>

      <!-- Virtual "New Connection..." when searching -->
      <div
        v-if="searchQuery.trim()"
        class="connection-item virtual-new-conn"
        :class="{ active: focusedId === '__new_connection__' }"
        @click="focusedId = '__new_connection__'; selectedIds = new Set(); lastClickId = '__new_connection__'"
        @dblclick="openNewFormFromSearch"
      >
        <div class="conn-details">
          <span class="name virtual-name">{{ t('sidebar.newConnectionFromSearch') }}</span>
          <span class="host">{{ t('conn.host') }}: {{ searchQuery.trim() }}</span>
        </div>
      </div>

      <div v-if="totalFiltered === 0 && connectionStore.connections.length > 0 && !searchQuery.trim()" class="empty-state">
        {{ t('sidebar.noSearchResults') }}
      </div>
      <div v-if="connectionStore.connections.length === 0" class="empty-state">
        {{ t('sidebar.noConnections') }}
      </div>
    </div>
    </template>

    <QuickCommandsPanel v-if="activeView === 'quickCommands'" />

    <HistoryPanel v-if="activeView === 'history'" />

    <template v-if="activeView === 'personalization'">
      <div class="personalization-panel">
        <div class="persist-section-title">{{ t('settings.app') }}</div>
        <div class="persist-section">
          <div class="persist-label">{{ t('settings.theme') }}</div>
          <el-select v-model="settingsStore.settings.theme" @change="settingsStore.save()">
            <el-option :label="t('settings.themeDark')" value="dark" />
            <el-option :label="t('settings.themeDeepBlue')" value="deep-blue" />
            <el-option :label="t('settings.themeLight')" value="light" />
            <el-option :label="t('settings.themeSystem')" value="system" />
          </el-select>
        </div>
        <div class="persist-section">
          <div class="persist-label">{{ t('settings.language') }}</div>
          <el-select :model-value="settingsStore.settings.language" @change="settingsStore.updateLanguage">
            <el-option
              v-for="lang in LANGUAGE_OPTIONS"
              :key="lang.value"
              :label="lang.native"
              :value="lang.value"
            />
            <el-option :label="t('settings.langSystem')" value="system" />
          </el-select>
        </div>
        <div class="persist-section-title">{{ t('settings.terminal') }}</div>
        <div class="persist-section">
          <div class="persist-label">{{ t('settings.colorScheme') }}</div>
          <div class="theme-select-row">
            <el-select v-model="settingsStore.settings.terminal.theme" @change="settingsStore.save()" popper-class="theme-select-popper">
              <el-option-group v-for="group in terminalThemeGroups" :key="group.label" :label="group.label">
                <el-option v-for="th in group.options" :key="th.value" :label="th.label" :value="th.value" />
              </el-option-group>
            </el-select>
            <button class="btn btn-ghost btn-icon btn-sm" :title="t('theme.newTitle')" @click="openThemeEditor()">
              <Plus :size="14" />
            </button>
            <button
              v-if="isCustomTheme(settingsStore.settings.terminal.theme)"
              class="btn btn-ghost btn-icon btn-sm"
              :title="t('theme.editTitle')"
              @click="openThemeEditor(settingsStore.settings.terminal.theme)"
            >
              <Pencil :size="14" />
            </button>
          </div>
        </div>
        <div class="persist-section">
          <div class="persist-label">{{ t('settings.font') }}</div>
          <el-select v-model="settingsStore.settings.terminal.fontFamily" @change="settingsStore.save()">
            <el-option
              v-for="f in personalizationFontOptions"
              :key="f.value"
              :label="f.label"
              :value="f.value"
              :style="{ fontFamily: f.value }"
            />
          </el-select>
        </div>
        <div class="persist-section">
          <div class="persist-label">{{ t('settings.fontSize') }}</div>
          <el-input-number
            v-model="settingsStore.settings.terminal.fontSize"
            :min="8"
            :max="32"
            @change="settingsStore.save()"
          />
        </div>
      </div>
    </template>

    <ConnectionForm v-model="showForm" :edit-config="editConfig" :default-group-id="newConnGroupId" @save="onSave" @connect="onConnectFromForm" />
    <CustomThemeEditor v-model="themeEditorVisible" :source-theme-id="themeEditorSourceId" />

    <!-- Connection context menu (kept inside sidebar to avoid native RDP occlusion) -->
    <div
      v-show="menuVisible"
      ref="menuRef"
      class="conn-context-menu"
      :style="menuStyle"
      @click.stop
    >
      <!-- Terminal -->
      <div v-if="selectedConn && selectedConn.type === 'ssh'" class="menu-item" @click="doConnect">{{ t('sidebar.connectSSH') }}</div>
      <div v-if="selectedConn && selectedConn.type === 'telnet'" class="menu-item" @click="doConnect">{{ t('sidebar.connectTelnet') }}</div>
      <div v-if="selectedConn && selectedConn.type === 'mosh'" class="menu-item" @click="doConnect">{{ t('sidebar.connectMosh') }}</div>
      <div v-if="selectedConn && selectedConn.type === 'local'" class="menu-item" @click="doConnect">{{ t('sidebar.connectLocal') }}</div>
      <div v-if="selectedConn && selectedConn.type === 'serial'" class="menu-item" @click="emit('connectSerial')">{{ t('sidebar.connectSerial') }}</div>
      <!-- File Transfer -->
      <div v-if="selectedConn && selectedConn.type === 'ssh'" class="menu-item" @click="doConnectSFTP">{{ t('sidebar.connectSftp') }}</div>
      <div v-if="selectedConn && selectedConn.type === 'ftp'" class="menu-item" @click="doConnectFTP">{{ t('sidebar.connectFtp') }}</div>
      <div v-if="selectedConn && selectedConn.type === 'smb'" class="menu-item" @click="doConnectSMB">{{ t('sidebar.connectSmb') }}</div>
      <div v-if="selectedConn && selectedConn.type === 's3'" class="menu-item" @click="doConnectS3">{{ t('sidebar.connectS3') }}</div>
      <div v-if="selectedConn && selectedConn.type === 'webdav'" class="menu-item" @click="doConnectWebDAV">{{ t('sidebar.connectWebdav') }}</div>
      <!-- Remote Desktop -->
      <div v-if="selectedConn && selectedConn.type === 'rdp'" class="menu-item" @click="doConnectRDP">{{ t('sidebar.connectRDP') }}</div>
      <div v-if="selectedConn && selectedConn.type === 'vnc'" class="menu-item" @click="doConnectVNC">{{ t('sidebar.connectVNC') }}</div>
      <div v-if="selectedConn && selectedConn.type === 'spice'" class="menu-item" @click="doConnectSPICE">{{ t('sidebar.connectSPICE') }}</div>
      <!-- Database & Monitor -->
      <div v-if="selectedConn && selectedConn.type === 'database'" class="menu-item" @click="doConnectDB">{{ t('db.connectDB') }}</div>
      <div v-if="selectedConn && selectedConn.type === 'ssh'" class="menu-item" @click="doConnectMonitor">{{ t('sidebar.connectMonitor') }}</div>
      <div class="menu-divider" />
      <div class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doEdit()">{{ t('sidebar.edit') }}</div>
      <div class="menu-item" @click="doDuplicate">{{ t('sidebar.duplicate') }}</div>
      <div class="menu-divider" />
      <div class="menu-item" @click="doChangeGroup">{{ t('conn.changeGroup') }}</div>
      <div class="menu-item" @click="doNewGroup">{{ t('conn.newGroupTitle') }}</div>
      <div class="menu-divider" />
      <div class="menu-item danger" @click="doDelete">{{ t('sidebar.delete') }}</div>
    </div>

    <!-- Group context menu (kept inside sidebar to avoid native RDP occlusion) -->
    <div
      v-show="groupMenuVisible"
      ref="groupMenuRef"
      class="conn-context-menu"
      :style="groupMenuStyle"
      @click.stop
    >
      <div class="menu-item" @click="doNewGroup">{{ t('conn.newGroupTitle') }}</div>
      <div class="menu-item" @click="doNewConnInGroup">{{ t('sidebar.newConnection') }}</div>
      <template v-if="selectedGroup && selectedGroup.id !== '__ungrouped__'">
        <div class="menu-divider" />
        <div class="menu-item" @click="doRenameGroup">{{ t('conn.renameGroup') }}</div>
        <div class="menu-divider" />
        <div class="menu-item danger" @click="doDeleteGroup">{{ t('conn.deleteGroup') }}</div>
      </template>
    </div>

    <!-- Empty area context menu (kept inside sidebar to avoid native RDP occlusion) -->
    <div
      v-show="emptyAreaMenuVisible"
      ref="emptyAreaMenuRef"
      class="conn-context-menu"
      :style="emptyAreaMenuStyle"
      @click.stop
    >
      <div class="menu-item" @click="doNewGroup">{{ t('conn.newGroupTitle') }}</div>
    </div>

    <!-- Delete group dialog -->
    <el-dialog v-model="showDeleteGroupDialog" :title="t('conn.deleteGroupTitle')" width="450px">
      <p>{{ deleteGroupPromptText }}</p>
      <template #footer>
        <el-button @click="showDeleteGroupDialog = false">{{ t('conn.deleteGroupCancel') }}</el-button>
        <el-button type="warning" @click="confirmDeleteGroup('move-out')">{{ t('conn.deleteGroupMoveOut') }}</el-button>
        <el-button type="danger" @click="confirmDeleteGroup('delete-connections')">{{ t('conn.deleteGroupDeleteAll') }}</el-button>
      </template>
    </el-dialog>

    <!-- Rename group dialog -->
    <el-dialog v-model="showRenameGroupDialog" :title="t('conn.renameGroup')" width="360px">
      <el-form @submit.prevent="confirmRenameGroup">
        <el-form-item :label="t('conn.groupName')">
          <el-input
            v-model="renameGroupName"
            :placeholder="t('conn.groupNamePlaceholder')"
            @keyup.enter="confirmRenameGroup"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRenameGroupDialog = false">{{ t('conn.cancel') }}</el-button>
        <el-button type="primary" @click="confirmRenameGroup">{{ t('conn.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- Change group dialog -->
    <el-dialog v-model="showChangeGroupDialog" :title="t('conn.changeGroup')" width="360px">
      <el-select v-model="changeGroupTargetId" :placeholder="t('conn.noGroup')" clearable style="width:100%">
        <el-option
          v-for="g in connectionStore.groups"
          :key="g.id"
          :label="g.name"
          :value="g.id"
        />
        <el-option
          :label="t('conn.noGroup')"
          value="__none__"
        />
        <el-option
          :label="t('conn.newGroup')"
          value="__new__"
        />
      </el-select>
      <template #footer>
        <el-button @click="showChangeGroupDialog = false">{{ t('conn.cancel') }}</el-button>
        <el-button type="primary" @click="confirmChangeGroup">{{ t('conn.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- Standalone new group dialog -->
    <el-dialog v-model="showNewGroupDialog" :title="t('conn.newGroupTitle')" width="360px">
      <el-form @submit.prevent="confirmNewGroup">
        <el-form-item :label="t('conn.groupName')">
          <el-input
            v-model="newGroupName"
            :placeholder="t('conn.groupNamePlaceholder')"
            @keyup.enter="confirmNewGroup"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showNewGroupDialog = false">{{ t('conn.cancel') }}</el-button>
        <el-button type="primary" @click="confirmNewGroup">{{ t('conn.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- New group dialog (for change group flow) -->
    <el-dialog v-model="showChangeNewGroupDialog" :title="t('conn.newGroupTitle')" width="360px">
      <el-form @submit.prevent="confirmChangeNewGroup">
        <el-form-item :label="t('conn.groupName')">
          <el-input
            v-model="changeNewGroupName"
            :placeholder="t('conn.groupNamePlaceholder')"
            @keyup.enter="confirmChangeNewGroup"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showChangeNewGroupDialog = false">{{ t('conn.cancel') }}</el-button>
        <el-button type="primary" @click="confirmChangeNewGroup">{{ t('conn.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, nextTick } from 'vue'
import { X, ChevronRight, ChevronDown, Filter, Check, Network, Zap, Clock, Plus, Palette, SquareTerminal, Terminal, FolderUp, HardDrive, Cloud, Globe, Monitor, MonitorCloud, MonitorSmartphone, Database, DatabaseZap, Activity, Laptop, Cable, Pencil } from '@lucide/vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { useConnectionStore } from '../stores/connectionStore'
import { useSettingsStore } from '../stores/settingsStore'
import { useI18n } from '../i18n'
import ConnectionForm from './ConnectionForm.vue'
import QuickCommandsPanel from './QuickCommandsPanel.vue'
import HistoryPanel from './HistoryPanel.vue'
import CustomThemeEditor from './CustomThemeEditor.vue'
import type { ConnectionConfig, ConnectionGroup } from '../types/session'
import { parseQuickConnect, formatConnSubtitle } from '../utils/quickConnect'
import { FONT_OPTIONS, LANGUAGE_OPTIONS } from '../types/settings'
import { useTerminalThemeOptions } from '../composables/useTerminalThemeOptions'
import { GetSystemFonts } from '../../wailsjs/go/main/App'

defineProps<{
  visible: boolean
}>()
const emit = defineEmits(['connect', 'connectSftp', 'connectFtp', 'connectSmb', 'connectWebdav', 'connectS3', 'connectRdp', 'connectVnc', 'connectSpice', 'connectDB', 'connectMonitor', 'connectSerial', 'toggle', 'new-local-terminal-with-shell'])
const connectionStore = useConnectionStore()
const settingsStore = useSettingsStore()
const { t } = useI18n()
const showForm = ref(false)
const editConfig = ref<ConnectionConfig | undefined>(undefined)
const activeView = ref<'connections' | 'quickCommands' | 'history' | 'personalization'>('connections')

// ── Personalization panel ──
const systemFonts = ref<{ label: string; value: string }[]>([])
const personalizationFontOptions = computed(() => {
  if (systemFonts.value.length > 0) {
    return systemFonts.value
  }
  return FONT_OPTIONS
})

const { terminalThemeGroups, isCustomTheme } = useTerminalThemeOptions()

const themeEditorVisible = ref(false)
const themeEditorSourceId = ref<string | undefined>(undefined)
function openThemeEditor(sourceThemeId?: string) {
  themeEditorSourceId.value = sourceThemeId
  themeEditorVisible.value = true
}

// Notify App.vue to hide native RDP window when edit dialog opens
watch(showForm, (val) => {
  window.dispatchEvent(new CustomEvent(val ? 'rdp:overlay-push' : 'rdp:overlay-pop'))
})

const searchQuery = ref('')
const searchInputRef = ref<any>(null)
const selectedTypeFilter = ref('all')
const focusedId = ref<string | null>(null)

function focusSearch() {
  nextTick(() => {
    const el = searchInputRef.value?.$el?.querySelector('input')
    if (el instanceof HTMLInputElement) {
      el.focus()
      el.select()
    }
  })
}

// ── Type filter ──
interface TypeOption {
  label: string
  value: string
}

const TYPE_LABELS: Record<string, string> = {
  ssh: 'SSH',
  telnet: 'Telnet',
  mosh: 'Mosh',
  rdp: 'RDP',
  vnc: 'VNC',
  spice: 'SPICE',
  local: 'Local',
  sftp: 'SFTP',
  ftp: 'FTP',
  smb: 'SMB',
  s3: 'S3',
  webdav: 'WebDAV',
  monitor: 'Monitor',
  'database:mysql': 'MySQL',
  'database:postgres': 'PostgreSQL',
  'database:rqlite': 'rqlite',
  'database:oracle': 'Oracle',
  'database:sqlserver': 'SQL Server',
  'database:redis': 'Redis',
}

const availableTypes = computed<TypeOption[]>(() => {
  const types = new Set<string>()
  for (const c of connectionStore.connections) {
    if (c.type === 'database' && c.dbType) {
      types.add(`database:${c.dbType}`)
    } else {
      types.add(c.type)
    }
  }
  return [...types].map(value => ({
    value,
    label: TYPE_LABELS[value] || value
  })).sort((a, b) => a.label.localeCompare(b.label))
})

function matchTypeFilter(conn: ConnectionConfig, filter: string): boolean {
  if (filter === 'all') return true
  if (filter.startsWith('database:')) {
    return conn.type === 'database' && conn.dbType === filter.slice(9)
  }
  return conn.type === filter
}

// ── Expand/collapse state ──
const expandedGroups = ref<Set<string>>(new Set())

function toggleGroup(groupId: string) {
  if (expandedGroups.value.has(groupId)) {
    expandedGroups.value.delete(groupId)
  } else {
    expandedGroups.value.add(groupId)
  }
}

// ── Multi-select ──
const selectedIds = ref<Set<string>>(new Set())

// ── Search filter ──
const filteredGrouped = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  const typeFilter = selectedTypeFilter.value
  const data = connectionStore.groupedConnections

  const matchConn = (c: ConnectionConfig) => {
    const textMatch = !q || c.name.toLowerCase().includes(q) || c.host.toLowerCase().includes(q)
    const typeMatch = matchTypeFilter(c, typeFilter)
    return textMatch && typeMatch
  }

  const filteredGroups = data.groups
    .map(entry => ({
      group: entry.group,
      connections: entry.connections.filter(matchConn)
    }))
    .filter(entry => {
      const groupNameMatch = entry.group.name.toLowerCase().includes(q)
      if (groupNameMatch) {
        // Show all connections in group when group name matches, but still apply type filter
        entry.connections = data.groups.find(g => g.group.id === entry.group.id)!.connections.filter(matchConn)
        return true
      }
      return entry.connections.length > 0
    })

  const filteredUngrouped = data.ungrouped.filter(matchConn)

  return { groups: filteredGroups, ungrouped: filteredUngrouped }
})

const totalFiltered = computed(() => {
  let count = 0
  for (const g of filteredGrouped.value.groups) {
    count += g.connections.length
  }
  count += filteredGrouped.value.ungrouped.length
  return count
})

// Update selection when filter changes
watch(filteredGrouped, () => {
  const allIds: string[] = []
  for (const g of filteredGrouped.value.groups) {
    for (const c of g.connections) allIds.push(c.id)
  }
  for (const c of filteredGrouped.value.ungrouped) allIds.push(c.id)

  if (allIds.length === 0) {
    focusedId.value = searchQuery.value.trim() ? '__new_connection__' : null
    selectedIds.value = new Set()
  } else if (!focusedId.value || !allIds.includes(focusedId.value)) {
    focusedId.value = allIds[0]
    selectedIds.value = new Set([allIds[0]])
  }
}, { immediate: true })

// Auto-expand all groups by default
watch(() => connectionStore.groups, (groups) => {
  for (const g of groups) {
    expandedGroups.value.add(g.id)
  }
  if (groups.length > 0) {
    expandedGroups.value.add('__ungrouped__')
  }
}, { immediate: true })

// Auto-expand groups when searching
watch(searchQuery, (q) => {
  if (q.trim()) {
    for (const g of filteredGrouped.value.groups) {
      expandedGroups.value.add(g.group.id)
    }
    if (connectionStore.groups.length > 0) {
      expandedGroups.value.add('__ungrouped__')
    }
  }
})

// ── Resize ──
const sidebarWidth = ref(220)
const isResizing = ref(false)
const sidebarEl = ref<HTMLDivElement>()

function onResizeStart(e: MouseEvent) {
  isResizing.value = true
  const el = sidebarEl.value
  if (!el) return
  const startX = e.clientX
  const startWidth = el.offsetWidth

  window.dispatchEvent(new CustomEvent('split:resize-start'))

  function onMouseMove(ev: MouseEvent) {
    if (!isResizing.value) return
    const delta = ev.clientX - startX
    const newWidth = Math.min(Math.max(startWidth + delta, 180), 400)
    el!.style.width = newWidth + 'px'
  }

  function onMouseUp() {
    isResizing.value = false
    sidebarWidth.value = el!.offsetWidth
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    window.dispatchEvent(new CustomEvent('split:resize-end'))
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

// ── Keyboard navigation ──
function getAllVisibleIds(): string[] {
  const ids: string[] = []
  for (const g of filteredGrouped.value.groups) {
    if (expandedGroups.value.has(g.group.id)) {
      for (const c of g.connections) ids.push(c.id)
    }
  }
  if (connectionStore.groups.length > 0) {
    if (expandedGroups.value.has('__ungrouped__')) {
      for (const c of filteredGrouped.value.ungrouped) ids.push(c.id)
    }
  } else {
    for (const c of filteredGrouped.value.ungrouped) ids.push(c.id)
  }
  if (searchQuery.value.trim()) {
    ids.push('__new_connection__')
  }
  return ids
}

function scrollActiveIntoView() {
  nextTick(() => {
    const activeEl = sidebarEl.value?.querySelector('.connection-item.active') as HTMLElement
    activeEl?.scrollIntoView({ block: 'nearest' })
  })
}

function onListKeydown(e: KeyboardEvent) {
  if (showForm.value || menuVisible.value || groupMenuVisible.value) return
  const ids = getAllVisibleIds()
  if (ids.length === 0) return

  const idx = ids.indexOf(focusedId.value || '')

  if (e.key === 'ArrowDown') {
    e.preventDefault()
    const nextIdx = idx >= 0 && idx < ids.length - 1 ? idx + 1 : 0
    focusedId.value = ids[nextIdx]
    selectedIds.value = new Set([ids[nextIdx]])
    scrollActiveIntoView()
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    const prevIdx = idx > 0 ? idx - 1 : ids.length - 1
    focusedId.value = ids[prevIdx]
    selectedIds.value = new Set([ids[prevIdx]])
    scrollActiveIntoView()
  } else if (e.key === 'Enter') {
    e.preventDefault()
    if (focusedId.value === '__new_connection__') {
      openNewFormFromSearch()
      return
    }
    const ids = getSelectedConnectionIds()
    if (ids.length > 0) {
      for (const id of ids) {
        const c = connectionStore.connections.find(c => c.id === id)
        if (c) {
          if (c.type === 'database') {
            emit('connectDB', c)
          } else if (c.type === 'rdp') {
            emit('connectRdp', c)
          } else if (c.type === 'vnc') {
            emit('connectVnc', c)
          } else {
            emit('connect', c)
          }
        }
      }
      selectedIds.value = new Set()
    }
  }
}

function findConnById(id: string | null): ConnectionConfig | undefined {
  if (!id) return undefined
  return connectionStore.connections.find(c => c.id === id)
}

// ── Drag & drop ──
const dragOverGroupId = ref<string | null>(null)

function onDragStart(e: DragEvent, conn: ConnectionConfig) {
  const ids = getSelectedConnectionIds()
  // If dragging an unselected item, drag just this one
  if (!ids.includes(conn.id)) {
    selectedIds.value = new Set()
    e.dataTransfer!.setData('text/plain', JSON.stringify([conn.id]))
  } else {
    e.dataTransfer!.setData('text/plain', JSON.stringify(ids))
  }
  e.dataTransfer!.effectAllowed = 'move'
}

function onDragEnd() {
  dragOverGroupId.value = null
}

function onGroupDragOver(groupId: string) {
  dragOverGroupId.value = groupId
}

function onGroupDragLeave(groupId: string) {
  if (dragOverGroupId.value === groupId) {
    dragOverGroupId.value = null
  }
}

async function onGroupDrop(groupId: string, e: DragEvent) {
  const targetGroupId = groupId === '__ungrouped__' ? undefined : groupId
  const raw = e.dataTransfer?.getData('text/plain')
  if (raw) {
    const ids: string[] = JSON.parse(raw)
    await connectionStore.setConnectionsGroup(ids, targetGroupId)
    selectedIds.value = new Set()
  }
  dragOverGroupId.value = null
}

// ── Connection click / multi-select ──
const lastClickId = ref<string | null>(null)

function onItemClick(e: MouseEvent, conn: ConnectionConfig) {
  if (e.shiftKey && lastClickId.value) {
    // Range select from last click to current
    const ids = getAllVisibleIds()
    const anchorIdx = ids.indexOf(lastClickId.value)
    const currentIdx = ids.indexOf(conn.id)
    if (anchorIdx >= 0 && currentIdx >= 0) {
      const [start, end] = anchorIdx < currentIdx ? [anchorIdx, currentIdx] : [currentIdx, anchorIdx]
      const selected = new Set<string>()
      for (let i = start; i <= end; i++) {
        selected.add(ids[i])
      }
      selectedIds.value = selected
    }
    focusedId.value = conn.id
  } else if (e.ctrlKey || e.metaKey) {
    // Toggle multi-select
    if (selectedIds.value.has(conn.id)) {
      selectedIds.value.delete(conn.id)
    } else {
      selectedIds.value.add(conn.id)
    }
    selectedIds.value = new Set(selectedIds.value)
    lastClickId.value = conn.id
  } else {
    focusedId.value = conn.id
    selectedIds.value = new Set([conn.id])
    lastClickId.value = conn.id
  }
}

function onItemDblClick(conn: ConnectionConfig) {
  selectedIds.value = new Set()
  if (conn.type === 'database') {
    emit('connectDB', conn)
  } else if (conn.type === 'rdp') {
    emit('connectRdp', conn)
  } else if (conn.type === 'vnc') {
    emit('connectVnc', conn)
  } else if (conn.type === 'spice') {
    emit('connectSpice', conn)
  } else {
    emit('connect', conn)
  }
}

// ── Context menu helper ──
function getSelectedConnectionIds(): string[] {
  if (selectedIds.value.size > 0) {
    return [...selectedIds.value]
  }
  if (selectedConn.value) {
    return [selectedConn.value.id]
  }
  if (focusedId.value) {
    return [focusedId.value]
  }
  return []
}

// ── Connection context menu ──
const menuVisible = ref(false)
const menuStyle = ref({ left: '0px', top: '0px' })
const selectedConn = ref<ConnectionConfig | null>(null)
const menuRef = ref<HTMLDivElement>()

function clampMenuPosition(x: number, y: number): { left: string, top: string } {
  const el = sidebarEl.value
  if (!el) return { left: x + 'px', top: y + 'px' }
  const sr = el.getBoundingClientRect()
  const menuW = 150
  const menuH = 220
  let left = x
  let top = y
  if (left + menuW > sr.right) left = sr.right - menuW - 4
  if (left < sr.left) left = sr.left + 4
  if (top + menuH > window.innerHeight) top = y - menuH
  if (top < 0) top = 4
  return { left: left + 'px', top: top + 'px' }
}

function onContextMenu(e: MouseEvent, conn: ConnectionConfig) {
  e.stopPropagation()
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  selectedConn.value = conn
  // If right-clicking on a non-multi-selected item, clear others and select this one
  if (!selectedIds.value.has(conn.id)) {
    selectedIds.value = new Set([conn.id])
    focusedId.value = conn.id
  }
  menuStyle.value = clampMenuPosition(e.clientX, e.clientY)
  menuVisible.value = true
}

function closeMenu() {
  menuVisible.value = false
}

function doConnect() {
  const ids = getSelectedConnectionIds()
  // Collect connections before any state changes
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  // Emit sequentially — each onConnect runs async but tabs/panels are created synchronously
  for (const c of conns) {
    emit('connect', c)
  }
}

function doConnectSFTP() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    emit('connectSftp', c)
  }
}

function doConnectFTP() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    emit('connectFtp', c)
  }
}

function doConnectSMB() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    emit('connectSmb', c)
  }
}

function doConnectWebDAV() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    emit('connectWebdav', c)
  }
}

function doConnectS3() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    emit('connectS3', c)
  }
}

function doConnectMonitor() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    emit('connectMonitor', c)
  }
}

function doConnectRDP() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    emit('connectRdp', c)
  }
}

function doConnectVNC() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    emit('connectVnc', c)
  }
}

function doConnectSPICE() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    emit('connectSpice', c)
  }
}

function doConnectDB() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    emit('connectDB', c)
  }
}

function doEdit() {
  if (selectedConn.value) {
    editConfig.value = { ...selectedConn.value }
    showForm.value = true
  }
  closeMenu()
}

function doDuplicate() {
  const ids = getSelectedConnectionIds()
  const conns = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
  selectedIds.value = new Set()
  closeMenu()
  for (const c of conns) {
    const dupName = generateDuplicateName(c.name)
    const dup: ConnectionConfig = {
      ...c,
      id: `conn-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      name: dupName
    }
    connectionStore.add(dup)
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

async function doDelete() {
  closeMenu()
  const ids = getSelectedConnectionIds()
  try {
    await ElMessageBox.confirm(
      t('sidebar.deleteConfirm', { count: ids.length }),
      t('sidebar.delete'),
      { confirmButtonText: t('sftp.dialog.confirm'), cancelButtonText: t('sftp.dialog.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  connectionStore.removeMany(ids)
  selectedIds.value = new Set()
}

// ── Standalone new group ──
const showNewGroupDialog = ref(false)
const newGroupName = ref('')

function doNewGroup() {
  closeMenu()
  closeGroupMenu()
  closeEmptyAreaMenu()
  newGroupName.value = ''
  showNewGroupDialog.value = true
}

const newConnGroupId = ref<string | undefined>(undefined)

function doNewConnInGroup() {
  closeMenu()
  closeGroupMenu()
  closeEmptyAreaMenu()
  editConfig.value = undefined
  newConnGroupId.value = selectedGroup.value?.id !== '__ungrouped__' ? selectedGroup.value?.id : undefined
  showForm.value = true
}

async function confirmNewGroup() {
  const name = newGroupName.value.trim()
  if (!name) return
  if (connectionStore.groups.some(g => g.name === name)) {
    ElMessage.warning(t('conn.groupNameDuplicate'))
    return
  }
  await connectionStore.addGroup(name)
  showNewGroupDialog.value = false
}

// ── Change group ──
const showChangeGroupDialog = ref(false)
const changeGroupTargetId = ref<string | undefined>(undefined)
const showChangeNewGroupDialog = ref(false)
const changeNewGroupName = ref('')

function doChangeGroup() {
  closeMenu()
  const ids = getSelectedConnectionIds()
  // Find common group if all selected connections are in the same group
  const groups = new Set(ids.map(id => {
    const c = connectionStore.connections.find(c => c.id === id)
    return c?.groupId || '__none__'
  }))
  if (groups.size === 1) {
    const g = [...groups][0]
    changeGroupTargetId.value = g === '__none__' ? undefined : g
  } else {
    changeGroupTargetId.value = undefined
  }
  showChangeGroupDialog.value = true
}

function confirmChangeGroup() {
  const val = changeGroupTargetId.value
  if (val === '__new__') {
    showChangeNewGroupDialog.value = true
    changeNewGroupName.value = ''
    return
  }
  const groupId = val === '__none__' ? undefined : val
  const ids = getSelectedConnectionIds()
  connectionStore.setConnectionsGroup(ids, groupId)
  selectedIds.value = new Set()
  showChangeGroupDialog.value = false
  changeGroupTargetId.value = undefined
}

async function confirmChangeNewGroup() {
  const name = changeNewGroupName.value.trim()
  if (!name) return
  if (connectionStore.groups.some(g => g.name === name)) {
    ElMessage.warning(t('conn.groupNameDuplicate'))
    return
  }
  const group = await connectionStore.addGroup(name)
  const ids = getSelectedConnectionIds()
  connectionStore.setConnectionsGroup(ids, group.id)
  selectedIds.value = new Set()
  showChangeNewGroupDialog.value = false
  showChangeGroupDialog.value = false
  changeGroupTargetId.value = undefined
}

// ── Group context menu ──
const groupMenuVisible = ref(false)
const groupMenuStyle = ref({ left: '0px', top: '0px' })
const selectedGroup = ref<ConnectionGroup | null>(null)
const groupMenuRef = ref<HTMLDivElement>()

function onGroupContextMenu(e: MouseEvent, group: ConnectionGroup) {
  e.stopPropagation()
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  selectedGroup.value = group
  groupMenuStyle.value = clampMenuPosition(e.clientX, e.clientY)
  groupMenuVisible.value = true
}

function closeGroupMenu() {
  groupMenuVisible.value = false
}

// ── Empty area context menu ──
const emptyAreaMenuVisible = ref(false)
const emptyAreaMenuStyle = ref({ left: '0px', top: '0px' })
const emptyAreaMenuRef = ref<HTMLDivElement>()

function onEmptyAreaContextMenu(e: MouseEvent) {
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  emptyAreaMenuStyle.value = clampMenuPosition(e.clientX, e.clientY)
  emptyAreaMenuVisible.value = true
}

function closeEmptyAreaMenu() {
  emptyAreaMenuVisible.value = false
}

// ── Virtual group context menu ──
function onVirtualGroupContextMenu(e: MouseEvent) {
  e.stopPropagation()
  window.dispatchEvent(new CustomEvent('global:close-context-menus'))
  selectedGroup.value = { id: '__ungrouped__', name: t('conn.noGroup') }
  groupMenuStyle.value = clampMenuPosition(e.clientX, e.clientY)
  groupMenuVisible.value = true
}

// ── Rename group ──
const showRenameGroupDialog = ref(false)
const renameGroupName = ref('')

function doRenameGroup() {
  closeGroupMenu()
  renameGroupName.value = selectedGroup.value?.name || ''
  showRenameGroupDialog.value = true
}

function confirmRenameGroup() {
  const name = renameGroupName.value.trim()
  if (!name || !selectedGroup.value) return
  connectionStore.renameGroup(selectedGroup.value.id, name)
  showRenameGroupDialog.value = false
}

// ── Delete group ──
const showDeleteGroupDialog = ref(false)

const deleteGroupPromptText = computed(() => {
  const g = selectedGroup.value
  if (!g) return ''
  const count = connectionStore.connections.filter(c => c.groupId === g.id).length
  return t('conn.deleteGroupPrompt', { name: g.name, count })
})

function doDeleteGroup() {
  closeGroupMenu()
  if (!selectedGroup.value) return
  const count = connectionStore.connections.filter(c => c.groupId === selectedGroup.value!.id).length
  if (count === 0) {
    connectionStore.deleteGroup(selectedGroup.value.id, 'move-out')
    return
  }
  showDeleteGroupDialog.value = true
}

async function confirmDeleteGroup(action: 'delete-connections' | 'move-out') {
  if (selectedGroup.value) {
    await connectionStore.deleteGroup(selectedGroup.value.id, action)
  }
  showDeleteGroupDialog.value = false
  selectedGroup.value = null
}

// ── New-connection dropdown + shell submenu ──
const showShellSubmenu = ref(false)
const shellSubmenuStyle = ref({ left: '0px', top: '0px' })

function onNewConnCommand(cmd: string) {
  if (cmd === 'new-connection') {
    showForm.value = true
    editConfig.value = undefined
    newConnGroupId.value = undefined
  } else if (cmd === 'new-serial') {
    emit('connectSerial')
  } else if (cmd === 'new-group') {
    showNewGroupDialog.value = true
  }
}

function onShellSelect(sh: string) {
  showShellSubmenu.value = false
  emit('new-local-terminal-with-shell', sh)
}

function getShellLabel(path: string): string {
  if (!path) return 'Local'
  const lower = path.toLowerCase()
  if (lower.startsWith('wsl://')) {
    const distro = path.slice(6)
    return distro ? `WSL - ${distro}` : 'WSL'
  }
  if (lower.includes('pwsh')) return 'PowerShell'
  if (lower.includes('powershell')) return 'Windows PowerShell'
  if (lower.includes('bash')) return 'Git Bash'
  if (lower.includes('cmd')) return 'Command Prompt'
  return path.split(/[\\/]/).pop() || path
}

function getSubtitle(conn: ConnectionConfig): string {
  return formatConnSubtitle(conn, getShellLabel)
}

function connIcon(conn: ConnectionConfig) {
  switch (conn.type) {
    case 'ssh': return SquareTerminal
    case 'telnet': return Terminal
    case 'mosh': return Zap
    case 'local': return Laptop
    case 'serial': return Cable
    case 'sftp': return FolderUp
    case 'ftp': return FolderUp
    case 'smb': return HardDrive
    case 's3': return Cloud
    case 'webdav': return Globe
    case 'rdp': return Monitor
    case 'vnc': return MonitorSmartphone
    case 'spice': return MonitorCloud
    case 'database': return conn.dbType === 'redis' ? DatabaseZap : Database
    case 'monitor': return Activity
    default: return SquareTerminal
  }
}

function onNewConnVisibleChange(visible: boolean) {
  if (!visible) return
  nextTick(() => {
    // Position submenu flush against the right edge of the button, aligned with dropdown
    const btn = document.querySelector('.sb-icon-btn')
    if (btn) {
      const rect = btn.getBoundingClientRect()
      shellSubmenuStyle.value = {
        left: rect.right + 'px',
        top: (rect.bottom + 4) + 'px',
      }
    }
  })
}

// ── Form handlers ──
function openNewForm() {
  editConfig.value = undefined
  newConnGroupId.value = undefined
  showForm.value = true
}

function openNewFormFromSearch() {
  const parsed = parseQuickConnect(searchQuery.value.trim())
  editConfig.value = (parsed || { host: searchQuery.value.trim() }) as ConnectionConfig
  selectedIds.value = new Set()
  newConnGroupId.value = undefined
  showForm.value = true
}

function onSave(config: ConnectionConfig) {
  if (editConfig.value) {
    connectionStore.update(config.id, config)
  } else {
    connectionStore.add(config)
  }
  showForm.value = false
  editConfig.value = undefined
}

function onConnectFromForm(config: ConnectionConfig) {
  if (editConfig.value) {
    connectionStore.update(config.id, config)
  } else {
    connectionStore.add(config)
  }
  showForm.value = false
  editConfig.value = undefined
  if (config.type === 'database') {
    emit('connectDB', config)
  } else if (config.type === 'rdp') {
    emit('connectRdp', config)
  } else if (config.type === 'vnc') {
    emit('connectVnc', config)
  } else {
    emit('connect', config)
  }
}

// ── Lifecycle ──
onMounted(async () => {
  window.addEventListener('global:close-context-menus', () => {
    closeMenu()
    closeGroupMenu()
    closeEmptyAreaMenu()
  })
  document.addEventListener('click', () => {
    closeMenu()
    closeGroupMenu()
    closeEmptyAreaMenu()
  })
  // Load system fonts for personalization panel
  try {
    const fonts = await GetSystemFonts()
    if (fonts && fonts.length > 0) {
      systemFonts.value = fonts.map(f => ({ label: f, value: f }))
    }
  } catch {
    // Fall back to FONT_OPTIONS
  }
})

onUnmounted(() => {
  window.removeEventListener('global:close-context-menus', () => {
    closeMenu()
    closeGroupMenu()
    closeEmptyAreaMenu()
  })
  document.removeEventListener('click', () => {
    closeMenu()
    closeGroupMenu()
    closeEmptyAreaMenu()
  })
})

defineExpose({ focusSearch })
</script>

<style scoped>
.sidebar {
  background: var(--bg-elevated);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  position: relative;
}

.sidebar.collapsed {
  width: 0 !important;
  overflow: hidden;
}

.sidebar.resizing {
  transition: none;
}

.resize-handle {
  position: absolute;
  right: -6px;
  top: 0;
  bottom: 0;
  width: 6px;
  cursor: col-resize;
  z-index: 10;
  background: transparent;
}

/* Decorative line stays at the sidebar right edge */
.resize-handle::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: 1px;
  background: linear-gradient(
    180deg,
    transparent 0%,
    var(--accent-subtle) 20%,
    var(--accent-glow) 50%,
    var(--accent-subtle) 80%,
    transparent 100%
  );
}

/* Hover: 3px accent bar extending into sidebar */
.resize-handle:hover::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: 3px;
  background: var(--accent);
  box-shadow: 0 0 6px var(--accent-glow);
}

.sidebar-header {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 10px 14px;
  flex-shrink: 0;
}

.sidebar-header .icon-btn {
  margin-left: auto;
}


.sb-icon-btn {
  width: 26px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  flex-shrink: 0;
}

.sb-icon-btn:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.icon-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  transition: all 0.12s ease;
}

.icon-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.sidebar-tab {
  width: 26px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  color: var(--text-muted);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.15s;
}

.sidebar-tab:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.sidebar-tab.active {
  color: var(--accent);
  background: var(--accent-subtle);
}

.search-box {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 10px 6px;
  flex-shrink: 0;
}

.connection-list {
  flex: 1;
  overflow-y: auto;
  padding: 0 8px 8px;
  outline: none;
}

/* ── Group header ── */
.group-header {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 10px 6px 6px;
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  transition: background 0.12s ease;
  font-family: var(--font-ui);
  font-size: 12px;
  color: var(--text-secondary);
}

.group-header:hover {
  background: var(--bg-hover);
}

.group-arrow {
  display: inline-flex;
  align-items: center;
  width: 16px;
  color: var(--text-disabled);
}
.group-arrow-icon {
  display: block;
}

.group-name {
  font-weight: 600;
}

.group-header.drag-over {
  background: var(--accent-subtle);
  box-shadow: inset 0 0 0 1px var(--accent);
}

/* ── Connection item ── */
.connection-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.12s ease;
  margin-bottom: 2px;
  user-select: none;
}

.connection-item.indented {
  padding-left: 24px;
}

.connection-item:hover {
  background: var(--bg-hover);
}

.connection-item.active {
  background: var(--accent-subtle);
  box-shadow: inset 0 0 0 1px var(--accent-dim);
}

.connection-item.active .name {
  color: var(--accent);
}

.conn-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  flex-shrink: 0;
  color: var(--text-muted);
}

.conn-details {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.name {
  font-family: var(--font-ui);
  font-size: 12px;
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.host {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.conn-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.empty-state {
  padding: 32px 16px;
  text-align: center;
  font-size: 12px;
  color: var(--text-disabled);
  font-family: var(--font-ui);
}

.virtual-new-conn .virtual-name {
  color: var(--accent);
}

.filter-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  color: var(--text-muted);
  transition: color 0.12s ease;
  padding: 2px;
  border-radius: var(--radius-sm);
}

.filter-trigger:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.filter-trigger.active {
  color: var(--accent);
}

.filter-trigger.active:hover {
  color: var(--accent);
  background: var(--accent-subtle);
}

/* ── Personalization panel ── */
.personalization-panel {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.persist-section-title {
  font-size: 12px;
  font-weight: 600;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  padding: 0 0 4px 0;
  margin-bottom: -4px;
}

.persist-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.persist-label {
  font-size: 11px;
  font-family: var(--font-ui);
  color: var(--text-muted);
  padding-left: 2px;
}

.persist-section .el-select,
.persist-section .el-input-number {
  width: 100%;
}

.theme-select-row {
  display: flex;
  align-items: center;
  gap: 4px;
}

.theme-select-row .el-select {
  flex: 1;
  min-width: 0;
}
</style>

<style>
.conn-context-menu {
  position: fixed;
  z-index: 99999;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
  min-width: 140px;
  padding: 4px;
  backdrop-filter: blur(8px);
}

.conn-context-menu .menu-item {
  padding: 7px 14px;
  font-size: 12px;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  transition: all 0.1s ease;
}

.conn-context-menu .menu-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.conn-context-menu .menu-item.disabled {
  color: var(--text-disabled);
  cursor: default;
  pointer-events: none;
}

.conn-context-menu .menu-item.danger:hover {
  background: var(--error-subtle);
  color: var(--error);
}

.conn-context-menu .menu-divider {
  height: 1px;
  background: var(--border-subtle);
  margin: 4px 6px;
}

.type-filter-menu .el-dropdown-menu__item {
  padding: 6px 12px;
  font-size: 12px;
  font-family: var(--font-ui);
}

.type-filter-menu .el-dropdown-menu__item.is-active {
  color: var(--accent);
}

.dropdown-item-content {
  display: flex;
  align-items: center;
  gap: 6px;
}

.check-placeholder {
  display: inline-block;
  width: 14px;
  height: 14px;
  flex-shrink: 0;
}
</style>

<style>
.submenu-wrapper {
  position: relative;
}
.submenu-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.shell-submenu {
  position: fixed;
  z-index: 10001;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  box-shadow: var(--shadow-lg);
  padding: 4px;
  min-width: 140px;
}
.shell-item {
  padding: 6px 10px;
  font-size: 12px;
  border-radius: 4px;
  cursor: pointer;
  color: var(--text-primary);
}
.shell-item:hover {
  background: var(--bg-hover);
}

.new-conn-popper {
  margin-top: -8px !important;
  margin-left: 4px !important;
}

.theme-select-popper .el-select-group__title {
  font-size: 10px;
  color: var(--text-disabled);
  text-align: center;
  padding: 6px 12px 2px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.theme-select-popper .el-select-group__title::before,
.theme-select-popper .el-select-group__title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--border-subtle);
}
</style>
