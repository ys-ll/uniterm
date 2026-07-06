<template>
  <div ref="startTabRef" class="start-tab">
    <div class="start-content" :style="contentStyle">
      <!-- Branding -->
      <div class="start-brand" v-show="!searchQuery.trim()">uniTerm</div>

      <!-- Search row -->
    <div class="start-search-row">
      <el-dropdown trigger="click" placement="bottom-start" :teleported="false">
        <span class="start-filter-btn" :class="{ active: selectedTypeFilter !== 'all' }">
          <el-icon><Filter :size="14" /></el-icon>
          <span>{{ selectedTypeFilter === 'all' ? t('sidebar.filterAll') : (TYPE_LABELS[selectedTypeFilter] || selectedTypeFilter) }}</span>
        </span>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item
              :class="{ 'is-active': selectedTypeFilter === 'all' }"
              @click="selectedTypeFilter = 'all'"
            >
              {{ t('sidebar.filterAll') }}
            </el-dropdown-item>
            <el-dropdown-item
              v-for="typeOpt in availableTypes"
              :key="typeOpt.value"
              :class="{ 'is-active': selectedTypeFilter === typeOpt.value }"
              @click="selectedTypeFilter = typeOpt.value"
            >
              {{ typeOpt.label }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-input
        ref="searchInputRef"
        v-model="searchQuery"
        class="start-search-input"
        :placeholder="t('sidebar.searchPlaceholder')"
        clearable
        @keydown="onSearchKeydown"
      />
    </div>

    <!-- Action buttons -->
    <div class="start-action-btns">
      <button class="start-action-btn primary" @click="emit('new-connection', tab.viewMode === 'group' ? { groupId: tab.groupId } : undefined)">
        <el-icon><Plus :size="14" /></el-icon>
        {{ t('header.newConnection') }}
      </button>
      <el-dropdown trigger="click" placement="bottom-start" :teleported="false">
        <button class="start-action-btn">
          <el-icon><Laptop :size="14" /></el-icon>
          {{ t('conn.localTerminal') }}
        </button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item
              v-for="sh in settingsStore.availableShells"
              :key="sh"
              @click="emit('local-terminal', sh, $event.ctrlKey || $event.metaKey)"
            >
              {{ getShellLabel(sh) }}
            </el-dropdown-item>
            <el-dropdown-item v-if="settingsStore.availableShells.length === 0" disabled>
              No shells available
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <button class="start-action-btn" @click="emit('connect-serial', $event.ctrlKey || $event.metaKey)">
        <el-icon><Cable :size="14" /></el-icon>
        {{ t('sidebar.connectSerial') }}
      </button>
    </div>

    <!-- Breadcrumb for group view -->
    <div v-if="tab.viewMode === 'group'" class="start-breadcrumb">
      <span class="link" @click="goHome">{{ t('startTab.backToStart') }}</span>
      <span class="sep">/</span>
      <span class="current">{{ getGroupName(tab.groupId || '') }}</span>
    </div>

    <!-- Home view sections -->
    <template v-if="tab.viewMode === 'home'">
      <!-- Recent connections -->
      <template v-if="recentConfigs.length > 0">
        <div class="start-section-label">{{ t('startTab.recentConnections') }}</div>
        <div class="start-cards-grid">
          <div
            v-for="config in recentConfigs"
            :key="config.id"
            class="start-card"
            :class="{ focused: isCardFocused('recent:' + config.id), selected: selectedIds.has('recent:' + config.id) }"
            @click="onCardClick(config, $event, 'recent:')"
            @dblclick="onCardDblClick(config, $event)"
            @contextmenu.prevent="onContextMenu($event, config, 'recent:')"
          >
            <div class="start-card-top">
              <div class="start-card-icon" :class="config.type">
                <el-icon v-if="config.type === 'ssh'"><SquareTerminal :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'telnet'"><Terminal :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'mosh'"><Zap :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'local'"><Laptop :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'serial'"><Cable :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'ftp' || config.type === 'sftp'"><FolderUp :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'smb'"><HardDrive :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 's3'"><Cloud :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'webdav'"><Globe :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'rdp'"><Monitor :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'vnc'"><MonitorSmartphone :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'spice'"><MonitorCloud :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'database'">
                  <DatabaseZap v-if="config.dbType === 'redis'" :size="28" />
                  <Database v-else :size="28" />
                </el-icon>
                <el-icon v-else><Server :size="28" /></el-icon>
              </div>
              <div>
                <div class="start-card-name">{{ config.name }}</div>
                <div class="start-card-meta">{{ getCardSubtitle(config) }}</div>
              </div>
            </div>
            <button class="card-more-btn" @click.stop="onCardMoreClick($event, config, 'recent:')" :title="t('terminal.more')"><MoreHorizontal :size="16" /></button>
          </div>
        </div>
      </template>

      <!-- Groups -->
      <div class="start-section-label">
        {{ t('startTab.groups') }}
        <span class="start-add-group-btn" @click="showNewGroupDialog = true" :title="t('conn.newGroupTitle')"><el-icon><Plus :size="12" /></el-icon></span>
      </div>
      <div class="start-cards-grid">
        <div
          v-for="group in groupCards.groups"
          :key="group.id"
          class="start-card"
          :class="{ focused: isCardFocused('group:' + group.id) }"
          @click="onGroupClick(group.id)"
          @dblclick="enterGroup(group.id)"
          @contextmenu.prevent="onGroupContextMenu($event, group.id, group.name)"
        >
          <div class="start-card-top">
            <div class="start-card-icon group"><el-icon><Folder :size="22" /></el-icon></div>
            <div>
              <div class="start-card-name">{{ group.name }}</div>
              <div class="start-card-meta">{{ t('startTab.connectionsCount', { count: group.count }) }}</div>
            </div>
          </div>
        </div>
        <div
          v-if="groupCards.ungroupedCount > 0"
          class="start-card"
          :class="{ focused: isCardFocused('group:__ungrouped__') }"
          @click="onGroupClick('__ungrouped__')"
          @dblclick="enterGroup('__ungrouped__')"
        >
          <div class="start-card-top">
            <div class="start-card-icon ungrouped"><el-icon><FolderOpen :size="22" /></el-icon></div>
            <div>
              <div class="start-card-name">{{ t('conn.noGroup') }}</div>
              <div class="start-card-meta">{{ t('startTab.connectionsCount', { count: groupCards.ungroupedCount }) }}</div>
            </div>
          </div>
        </div>
      </div>

      <!-- All connections - only shown when searching -->
      <template v-if="searchQuery.trim()">
        <div class="start-section-label">{{ t('startTab.allConnections') }}</div>
        <div class="start-cards-grid">
          <div
            v-for="{ config } in filteredConnections"
            :key="config.id"
            class="start-card"
            :class="{ focused: isCardFocused('conn:' + config.id), selected: selectedIds.has('conn:' + config.id) }"
            @click="onCardClick(config, $event)"
            @dblclick="onCardDblClick(config, $event)"
            @contextmenu.prevent="onContextMenu($event, config)"
          >
            <div class="start-card-top">
              <div class="start-card-icon" :class="config.type">
                <el-icon v-if="config.type === 'ssh'"><SquareTerminal :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'telnet'"><Terminal :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'mosh'"><Zap :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'local'"><Laptop :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'serial'"><Cable :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'ftp' || config.type === 'sftp'"><FolderUp :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'smb'"><HardDrive :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 's3'"><Cloud :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'webdav'"><Globe :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'rdp'"><Monitor :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'vnc'"><MonitorSmartphone :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'spice'"><MonitorCloud :size="28" /></el-icon>
                <el-icon v-else-if="config.type === 'database'">
                  <DatabaseZap v-if="config.dbType === 'redis'" :size="28" />
                  <Database v-else :size="28" />
                </el-icon>
                <el-icon v-else><Server :size="28" /></el-icon>
              </div>
              <div>
                <div class="start-card-name">{{ config.name }}</div>
                <div class="start-card-meta">{{ getCardSubtitle(config) }}</div>
              </div>
            </div>
            <button class="card-more-btn" @click.stop="onCardMoreClick($event, config)" :title="t('terminal.more')"><MoreHorizontal :size="16" /></button>
          </div>
        </div>
        <div v-if="filteredConnections.length === 0 && connectionStore.connections.length > 0" class="start-empty-hint">
          {{ t('startTab.noConnections') }}
        </div>
      </template>
    </template>

    <!-- Group detail view -->
    <template v-if="tab.viewMode === 'group'">
      <div class="start-cards-grid">
        <div
          v-for="{ config } in filteredConnections"
          :key="config.id"
          class="start-card"
          :class="{ focused: isCardFocused('conn:' + config.id), selected: selectedIds.has('conn:' + config.id) }"
          @click="onCardClick(config, $event)"
          @dblclick="onCardDblClick(config, $event)"
	          @contextmenu.prevent="onContextMenu($event, config)"
        >
          <div class="start-card-top">
            <div class="start-card-icon" :class="config.type">
              <el-icon v-if="config.type === 'ssh'"><SquareTerminal :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'telnet'"><Terminal :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'mosh'"><Zap :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'local'"><Laptop :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'serial'"><Cable :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'ftp' || config.type === 'sftp'"><FolderUp :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'smb'"><HardDrive :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 's3'"><Cloud :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'webdav'"><Globe :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'rdp'"><Monitor :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'vnc'"><MonitorSmartphone :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'spice'"><MonitorCloud :size="28" /></el-icon>
              <el-icon v-else-if="config.type === 'database'">
                <DatabaseZap v-if="config.dbType === 'redis'" :size="28" />
                <Database v-else :size="28" />
              </el-icon>
              <el-icon v-else><Server :size="28" /></el-icon>
            </div>
            <div>
              <div class="start-card-name">{{ config.name }}</div>
              <div class="start-card-meta">{{ getCardSubtitle(config) }}</div>
            </div>
          </div>
          <button class="card-more-btn" @click.stop="onCardMoreClick($event, config)" :title="t('terminal.more')"><MoreHorizontal :size="16" /></button>
        </div>
      </div>
      <div v-if="filteredConnections.length === 0" class="start-empty-hint">
        {{ t('startTab.emptyGroup') }}
      </div>
    </template>

    <!-- Quick connect virtual card -->
    <div
      v-if="searchQuery.trim()"
      class="start-quick-card"
      :class="{ focused: isCardFocused('quick') }"
      @click="onQuickClick"
      @dblclick="emit('new-connection', { host: searchQuery.trim() })"
    >
      <div class="start-card-top">
        <div class="start-card-icon quick"><el-icon><Zap :size="22" /></el-icon></div>
        <div>
          <div class="start-card-name quick-name">{{ t('startTab.quickConnect', { host: searchQuery.trim() }) }}</div>
          <div class="start-card-meta">{{ t('startTab.quickConnectDesc') }}</div>
        </div>
      </div>
    </div>

    <!-- Empty state -->
    <div v-if="connectionStore.connections.length === 0" class="start-empty-state">
      <span class="empty-icon">📋</span>
      <p>{{ t('startTab.noConnections') }}</p>
    </div>

    <!-- Context menu -->
    <div
      v-show="contextMenuVisible"
      class="start-context-menu"
      :style="contextMenuStyle"
      @click.stop
    >
      <!-- Terminal -->
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'ssh'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnect(contextMenuConfig, $event)">{{ t('sidebar.connectSSH') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'telnet'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnect(contextMenuConfig, $event)">{{ t('sidebar.connectTelnet') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'mosh'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnect(contextMenuConfig, $event)">{{ t('sidebar.connectMosh') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'local'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnect(contextMenuConfig, $event)">{{ t('sidebar.connectLocal') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'serial'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectSerial(contextMenuConfig, $event)">{{ t('sidebar.connectSerial') }}</div>
      <!-- File Transfer -->
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'ssh'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectSftp(contextMenuConfig)">{{ t('sidebar.connectSftp') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'ftp'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectFtp(contextMenuConfig)">{{ t('sidebar.connectFtp') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'smb'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectSmb(contextMenuConfig)">{{ t('sidebar.connectSmb') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 's3'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectS3(contextMenuConfig)">{{ t('sidebar.connectS3') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'webdav'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectWebdav(contextMenuConfig)">{{ t('sidebar.connectWebdav') }}</div>
      <!-- Remote Desktop -->
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'rdp'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectRdp(contextMenuConfig)">{{ t('sidebar.connectRDP') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'vnc'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectVnc(contextMenuConfig)">{{ t('sidebar.connectVNC') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'spice'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectSpice(contextMenuConfig)">{{ t('sidebar.connectSPICE') }}</div>
      <!-- Database & Monitor -->
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'database'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectDb(contextMenuConfig)">{{ t('db.connectDB') }}</div>
      <div v-if="contextMenuConfig && contextMenuConfig.type === 'ssh'" class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doConnectMonitor(contextMenuConfig)">{{ t('sidebar.connectMonitor') }}</div>
      <div class="menu-divider" />
      <div class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doEditConnection(contextMenuConfig)">{{ t('sidebar.edit') }}</div>
      <div class="menu-item" @click="doChangeGroupBulk">{{ t('conn.changeGroup') }}</div>
      <div class="menu-item" :class="{ disabled: selectedIds.size > 1 }" @click="selectedIds.size <= 1 && doDuplicate(contextMenuConfig)">{{ t('sidebar.duplicate') }}</div>
      <div class="menu-divider" />
      <div class="menu-item danger" @click="doDeleteBulk">{{ t('sidebar.delete') }}</div>
    </div>

    <!-- Group context menu -->
    <div
      v-show="groupContextVisible"
      class="start-context-menu"
      :style="contextMenuStyle"
      @click.stop
    >
      <div class="menu-item" @click="doRenameGroup">{{ t('conn.renameGroup') }}</div>
      <div class="menu-divider" />
      <div class="menu-item danger" @click="doDeleteGroup">{{ t('conn.deleteGroup') }}</div>
    </div>
    </div>

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

    <!-- New group dialog -->
    <el-dialog v-model="showNewGroupDialog" :title="t('conn.newGroupTitle')" width="360px">
      <el-input v-model="newGroupDialogName" :placeholder="t('conn.newGroupTitle')" @keyup.enter="doAddGroup" />
      <template #footer>
        <el-button @click="showNewGroupDialog = false">{{ t('conn.deleteGroupCancel') }}</el-button>
        <el-button type="primary" @click="doAddGroup">{{ t('conn.newGroupTitle') }}</el-button>
      </template>
    </el-dialog>

    <!-- Delete group dialog -->
    <el-dialog v-model="showDeleteGroupDialog" :title="t('conn.deleteGroupTitle')" width="450px">
      <p>{{ deleteGroupPromptText }}</p>
      <template #footer>
        <el-button @click="showDeleteGroupDialog = false">{{ t('conn.deleteGroupCancel') }}</el-button>
        <el-button type="warning" @click="confirmDeleteGroup('move-out')">{{ t('conn.deleteGroupMoveOut') }}</el-button>
        <el-button type="danger" @click="confirmDeleteGroup('delete-connections')">{{ t('conn.deleteGroupDeleteAll') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { ElMessageBox } from 'element-plus'
import { msg } from '../services/message'
import type { StartTab } from '../types/workspace'
import type { ConnectionConfig, ConnectionGroup } from '../types/session'
import { useConnectionStore } from '../stores/connectionStore'
import { useTabStore } from '../stores/tabStore'
import { useSettingsStore } from '../stores/settingsStore'
import { useI18n } from '../i18n'
import { GetRecentConnections } from '../../wailsjs/go/main/App'
import { formatConnSubtitle } from '../utils/quickConnect'
import { Filter, Plus, Laptop, Cable, SquareTerminal, Terminal, Database, DatabaseZap, Monitor, MonitorSmartphone, MonitorCloud, FolderUp, HardDrive, Cloud, Globe, Server, Folder, FolderOpen, Zap, MoreHorizontal } from '@lucide/vue'

const props = defineProps<{
  tab: StartTab
}>()

const emit = defineEmits<{
  connect: [config: ConnectionConfig, keepOpen?: boolean]
  'new-connection': [payload?: { host?: string; groupId?: string }]
  'local-terminal': [shellPath: string, keepOpen?: boolean]
  'connect-serial': [keepOpen?: boolean]
  'close-self': [tabId: string]
  'edit-connection': [config: ConnectionConfig]
  'change-group': [config: ConnectionConfig]
  'change-group-ids': [ids: string[]]
}>()

const { t } = useI18n()
const connectionStore = useConnectionStore()
const tabStore = useTabStore()
const settingsStore = useSettingsStore()

// ── Card subtitle (matches sidebar display format) ──
function getCardSubtitle(config: ConnectionConfig): string {
  return formatConnSubtitle(config, getShellLabel)
}

// ── Multi-select ──
// Card keys: each card gets a unique key = prefix + config.id.
// "recent:" prefix for recent-connection cards, "conn:" for all others.
// This way the same connection appearing in both sections has two
// independently selectable cards.
const selectedIds = ref<Set<string>>(new Set())
const lastClickId = ref<string | null>(null)

function getAllVisibleIds(): string[] {
  const ids: string[] = []
  if (props.tab.viewMode === 'home') {
    for (const c of recentConfigs.value) ids.push('recent:' + c.id)
  }
  for (const { config } of filteredConnections.value) ids.push('conn:' + config.id)
  return ids
}

function cardKeyToId(key: string): string {
  const idx = key.indexOf(':')
  return idx >= 0 ? key.slice(idx + 1) : key
}

function getSelectedConnectionIds(): string[] {
  if (selectedIds.value.size > 0) {
    // Deduplicate by config ID (one connection may be selected via multiple cards)
    const seen = new Set<string>()
    const ids: string[] = []
    for (const key of selectedIds.value) {
      const id = cardKeyToId(key)
      if (!seen.has(id)) {
        seen.add(id)
        ids.push(id)
      }
    }
    return ids
  }
  if (contextMenuConfig.value) return [contextMenuConfig.value.id]
  return []
}

// ── Search & filter ──
const searchQuery = ref('')
const selectedTypeFilter = ref('all')
const searchInputRef = ref<HTMLInputElement>()

const TYPE_LABELS: Record<string, string> = {
  ssh: 'SSH', telnet: 'Telnet', mosh: 'Mosh', rdp: 'RDP', vnc: 'VNC', spice: 'SPICE',
  local: 'Local', sftp: 'SFTP', ftp: 'FTP', smb: 'SMB', s3: 'S3', webdav: 'WebDAV', monitor: 'Monitor',
  'database:mysql': 'MySQL', 'database:postgres': 'PostgreSQL', 'database:rqlite': 'rqlite',
  'database:oracle': 'Oracle', 'database:sqlserver': 'SQL Server', 'database:redis': 'Redis',
}

const availableTypes = computed(() => {
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

// ── Shell label helper ──
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

// ── Recent connections ──
const recentConnectionIds = ref<string[]>([])

async function loadRecent() {
  try {
    recentConnectionIds.value = await GetRecentConnections()
  } catch {
    recentConnectionIds.value = []
  }
}
loadRecent()

const recentConfigs = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return recentConnectionIds.value
    .map(id => connectionStore.connections.find(c => c.id === id))
    .filter((c): c is ConnectionConfig => !!c)
    .filter(c => matchTypeFilter(c, selectedTypeFilter.value))
    .filter(c => !query ||
      c.name.toLowerCase().includes(query) ||
      (c.host || '').toLowerCase().includes(query) ||
      c.type.toLowerCase().includes(query))
    .slice(0, 12)
})

// ── Filtered connections ──
const filteredConnections = computed(() => {
  let conns = connectionStore.connections

  if (props.tab.viewMode === 'group' && props.tab.groupId) {
    if (props.tab.groupId === '__ungrouped__') {
      conns = conns.filter(c => !c.groupId)
    } else {
      conns = conns.filter(c => c.groupId === props.tab.groupId)
    }
  }

  const query = searchQuery.value.trim().toLowerCase()

  return conns
    .filter(c => matchTypeFilter(c, selectedTypeFilter.value))
    .filter(c => !query ||
      c.name.toLowerCase().includes(query) ||
      (c.host || '').toLowerCase().includes(query) ||
      c.type.toLowerCase().includes(query))
    .sort((a, b) => a.name.localeCompare(b.name))
    .map(c => ({ config: c }))
})

// ── Groups ──
const groupCards = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  const hasFilter = !!query || selectedTypeFilter.value !== 'all'
  const matchFilter = (c: ConnectionConfig) =>
    matchTypeFilter(c, selectedTypeFilter.value) &&
    (!query || c.name.toLowerCase().includes(query) || (c.host || '').toLowerCase().includes(query) || c.type.toLowerCase().includes(query))
  let groups = [...connectionStore.groups]
    .sort((a, b) => a.name.localeCompare(b.name))
    .map(g => ({
      ...g,
      count: connectionStore.connections.filter(c => c.groupId === g.id && matchFilter(c)).length
    }))
  // Hide groups with 0 matches when filtering
  if (hasFilter) {
    groups = groups.filter(g => g.count > 0)
  }
  const ungroupedCount = connectionStore.connections.filter(c => !c.groupId && matchFilter(c)).length
  return { groups, ungroupedCount }
})

function getGroupName(groupId: string): string {
  if (groupId === '__ungrouped__') return t('conn.noGroup')
  return connectionStore.groups.find(g => g.id === groupId)?.name || ''
}

// ── Navigation ──
function onCardClick(config: ConnectionConfig, e: MouseEvent, prefix = 'conn:') {
  const key = prefix + config.id
  if (e.shiftKey && lastClickId.value) {
    const ids = getAllVisibleIds()
    const anchorIdx = ids.indexOf(lastClickId.value)
    const currentIdx = ids.indexOf(key)
    if (anchorIdx >= 0 && currentIdx >= 0) {
      const [start, end] = anchorIdx < currentIdx ? [anchorIdx, currentIdx] : [currentIdx, anchorIdx]
      const set = new Set<string>()
      for (let i = start; i <= end; i++) set.add(ids[i])
      selectedIds.value = set
    }
  } else if (e.ctrlKey || e.metaKey) {
    if (selectedIds.value.has(key)) {
      selectedIds.value.delete(key)
    } else {
      selectedIds.value.add(key)
    }
    selectedIds.value = new Set(selectedIds.value)
    lastClickId.value = key
  } else {
    selectedIds.value = new Set([key])
    lastClickId.value = key
  }
  const idx = focusableIndexMap.value.get(prefix + config.id)
  if (idx !== undefined) {
    focusedCardIndex.value = idx
    focusInGrid.value = true
  }
}

function onGroupClick(groupId: string) {
  const idx = focusableIndexMap.value.get('group:' + groupId)
  if (idx !== undefined) {
    focusedCardIndex.value = idx
    focusInGrid.value = true
    startTabRef.value?.focus()
  }
}

function onQuickClick() {
  const idx = focusableIndexMap.value.get('quick')
  if (idx !== undefined) {
    focusedCardIndex.value = idx
    focusInGrid.value = true
    startTabRef.value?.focus()
  }
}

function onCardDblClick(config: ConnectionConfig, e?: { ctrlKey?: boolean; metaKey?: boolean }) {
  emit('connect', config, e ? !!(e.ctrlKey || e.metaKey) : false)
}

function enterGroup(groupId: string) {
  props.tab.viewMode = 'group'
  props.tab.groupId = groupId
  focusedCardIndex.value = 0
  focusInGrid.value = true
}

function goHome() {
  const returnGroupId = props.tab.groupId
  props.tab.viewMode = 'home'
  props.tab.groupId = undefined
  focusInGrid.value = true
  nextTick(() => {
    if (returnGroupId) {
      const idx = focusableIndexMap.value.get('group:' + returnGroupId)
      focusedCardIndex.value = idx ?? 0
    } else {
      focusedCardIndex.value = 0
    }
  })
}


// ── Content area centering ──
const startTabRef = ref<HTMLElement | null>(null)
const contentWidth = ref(0)

const CARD_WIDTH = 240
const CARD_GAP = 12
const PADDING = 64 // .start-tab padding on each side

function updateContentWidth() {
  const el = startTabRef.value
  if (!el) return
  const available = el.clientWidth - PADDING * 2
  const cols = Math.max(2, Math.min(6, Math.floor((available + CARD_GAP) / (CARD_WIDTH + CARD_GAP))))
  contentWidth.value = cols * CARD_WIDTH + (cols - 1) * CARD_GAP
}

const contentStyle = computed(() => ({
  width: contentWidth.value + 'px',
  margin: '0 auto'
}))

let resizeObserver: ResizeObserver | null = null

// ── Keyboard navigation ──
const focusedCardIndex = ref(-1)
const focusInGrid = ref(false)

type FocusableItem =
  | { kind: 'recent'; config: ConnectionConfig }
  | { kind: 'group'; groupId: string; name: string }
  | { kind: 'connection'; config: ConnectionConfig }
  | { kind: 'quick' }

const focusableItems = computed<FocusableItem[]>(() => {
  if (props.tab.viewMode === 'group') {
    const items: FocusableItem[] = filteredConnections.value.map(({ config }) => ({ kind: 'connection' as const, config }))
    if (searchQuery.value.trim()) items.push({ kind: 'quick' })
    return items
  }
  const items: FocusableItem[] = []
  for (const config of recentConfigs.value) items.push({ kind: 'recent', config })
  for (const group of groupCards.value.groups) items.push({ kind: 'group', groupId: group.id, name: group.name })
  if (groupCards.value.ungroupedCount > 0) items.push({ kind: 'group', groupId: '__ungrouped__', name: t('conn.noGroup') })
  for (const { config } of filteredConnections.value) items.push({ kind: 'connection', config })
  if (searchQuery.value.trim()) items.push({ kind: 'quick' })
  return items
})

const focusableIndexMap = computed(() => {
  const map = new Map<string, number>()
  focusableItems.value.forEach((item, idx) => {
    if (item.kind === 'recent') map.set('recent:' + item.config.id, idx)
    else if (item.kind === 'connection') map.set('conn:' + item.config.id, idx)
    else if (item.kind === 'group') map.set('group:' + item.groupId, idx)
    else if (item.kind === 'quick') map.set('quick', idx)
  })
  return map
})

function isCardFocused(key: string): boolean {
  if (!focusInGrid.value) return false
  const idx = focusableIndexMap.value.get(key)
  return idx === focusedCardIndex.value
}

function getGridColumns(): number {
  if (contentWidth.value === 0) return 3
  return Math.max(1, Math.floor((contentWidth.value + CARD_GAP) / (CARD_WIDTH + CARD_GAP)))
}

function onSearchKeydown(e: KeyboardEvent) {
  if (e.key === 'Tab' || e.key === 'ArrowDown') {
    e.preventDefault()
    e.stopPropagation()
    searchInputRef.value?.blur()
    focusInGrid.value = true
    focusedCardIndex.value = 0
    return
  }
}

function onKeydown(e: KeyboardEvent) {
  // Don't handle keyboard navigation when a dialog is open
  if (showNewGroupDialog.value || showDeleteGroupDialog.value) return
  // Don't intercept keyboard events from modal dialogs (e.g. ConnectionForm)
  if ((e.target as HTMLElement)?.closest?.('.el-dialog')) return
  // Only handle when this component is mounted and this tab is active
  if (!startTabRef.value) return
  if (tabStore.activeTabId !== props.tab.id) return
  if (e.key === 'Tab') {
    e.preventDefault()
    if (focusInGrid.value) {
      focusInGrid.value = false
      focusedCardIndex.value = -1
      searchInputRef.value?.focus()
    } else {
      focusInGrid.value = true
      focusedCardIndex.value = 0
    }
    return
  }

  if ((e.key === 'Escape' || e.key === 'Backspace') && props.tab.viewMode === 'group') {
    goHome()
    focusedCardIndex.value = 0
    focusInGrid.value = true
    return
  }

  if (!focusInGrid.value) return

  const cols = getGridColumns()
  const items = focusableItems.value
  const total = items.length
  if (total === 0) return

  // Section boundaries: indices where kind changes
  const sectionStarts: number[] = [0]
  for (let i = 1; i < total; i++) {
    if (items[i].kind !== items[i - 1].kind) sectionStarts.push(i)
  }

  function sectionOf(idx: number) {
    for (let s = sectionStarts.length - 1; s >= 0; s--) {
      if (idx >= sectionStarts[s]) return s
    }
    return 0
  }

  if (e.key === 'ArrowRight') {
    e.preventDefault()
    focusedCardIndex.value = Math.min(focusedCardIndex.value + 1, total - 1)
  } else if (e.key === 'ArrowLeft') {
    e.preventDefault()
    focusedCardIndex.value = Math.max(focusedCardIndex.value - 1, 0)
  } else if (e.key === 'ArrowUp' && focusedCardIndex.value === 0) {
    e.preventDefault()
    focusInGrid.value = false
    focusedCardIndex.value = -1
    searchInputRef.value?.focus()
    return
  } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    const cur = focusedCardIndex.value
    const curSection = sectionOf(cur)
    const curStart = sectionStarts[curSection]
    const curEnd = curSection + 1 < sectionStarts.length ? sectionStarts[curSection + 1] : total
    const visualCol = (cur - curStart) % cols
    const dir = e.key === 'ArrowDown' ? 1 : -1

    // Stay in current section if possible
    const sameSectionTarget = cur + dir * cols
    if (sameSectionTarget >= curStart && sameSectionTarget < curEnd) {
      focusedCardIndex.value = sameSectionTarget
    } else {
      // Cross to next/prev section, align to same visual column
      const nextSection = curSection + dir
      if (nextSection < 0) {
        focusedCardIndex.value = Math.max(cur - cols, 0)
      } else if (nextSection >= sectionStarts.length) {
        focusedCardIndex.value = Math.min(cur + cols, total - 1)
      } else {
        const secStart = sectionStarts[nextSection]
        const secEnd = nextSection + 1 < sectionStarts.length ? sectionStarts[nextSection + 1] : total
        const secLen = secEnd - secStart
        const lastRowStart = secStart + Math.floor((secLen - 1) / cols) * cols
        const rowTarget = dir === 1 ? secStart : lastRowStart
        const target = Math.min(rowTarget + visualCol, secEnd - 1)
        focusedCardIndex.value = target
      }
    }
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const ids = getSelectedConnectionIds()
    if (ids.length > 1) {
      // Bulk connect: open all selected connections
      const configs = ids.map(id => connectionStore.connections.find(c => c.id === id)).filter(Boolean) as ConnectionConfig[]
      for (const c of configs) {
        emit('connect', c, e.ctrlKey || e.metaKey)
      }
      selectedIds.value = new Set()
    } else {
      const item = focusableItems.value[focusedCardIndex.value]
      if (!item) return
      if (item.kind === 'recent' || item.kind === 'connection') {
        onCardDblClick(item.config, e)
      } else if (item.kind === 'group') {
        enterGroup(item.groupId)
      } else if (item.kind === 'quick') {
        emit('new-connection', { host: searchQuery.value.trim() })
      }
    }
  }
}

// ── Context menu ──
const contextMenuVisible = ref(false)
const contextMenuStyle = ref<Record<string, string>>({})
const contextMenuConfig = ref<ConnectionConfig | null>(null)

function clampMenuPos(x: number, y: number, menuW = 160, menuH = 280): { left: string; top: string } {
  let left = x
  let top = y
  if (left + menuW > window.innerWidth) left = window.innerWidth - menuW - 4
  if (left < 0) left = 4
  if (top + menuH > window.innerHeight) top = y - menuH
  if (top < 0) top = 4
  return { left: left + 'px', top: top + 'px' }
}

function onContextMenu(e: MouseEvent, config: ConnectionConfig, prefix = 'conn:') {
  closeContextMenu()
  // If right-clicking an unselected item, replace selection with this card
  const key = prefix + config.id
  if (!selectedIds.value.has(key)) {
    selectedIds.value = new Set([key])
  }
  contextMenuConfig.value = config
  const pos = clampMenuPos(e.clientX, e.clientY)
  contextMenuStyle.value = { position: 'fixed', left: pos.left, top: pos.top, zIndex: '10000' }
  contextMenuVisible.value = true
}

function closeContextMenu() {
  contextMenuVisible.value = false
  groupContextVisible.value = false
}

function onCardMoreClick(e: MouseEvent, config: ConnectionConfig, prefix = 'conn:') {
  const btn = e.currentTarget as HTMLElement
  const rect = btn.getBoundingClientRect()
  const x = rect.right + 4
  const y = rect.top
  const key = prefix + config.id
  if (!selectedIds.value.has(key)) {
    selectedIds.value = new Set([key])
  }
  contextMenuConfig.value = config
  const pos = clampMenuPos(x, y)
  contextMenuStyle.value = { position: 'fixed', left: pos.left, top: pos.top, zIndex: '10000' }
  contextMenuVisible.value = true
}

// ── Group context menu ──
const groupContextVisible = ref(false)
const groupContextTarget = ref<{ id: string; name: string } | null>(null)

function onGroupContextMenu(e: MouseEvent, groupId: string, groupName: string) {
  closeContextMenu()
  groupContextTarget.value = { id: groupId, name: groupName }
  const pos = clampMenuPos(e.clientX, e.clientY, 140, 80)
  contextMenuStyle.value = { position: 'fixed', left: pos.left, top: pos.top, zIndex: '10000' }
  groupContextVisible.value = true
}

const showRenameGroupDialog = ref(false)
const renameGroupName = ref('')

function doRenameGroup() {
  closeContextMenu()
  if (!groupContextTarget.value) return
  renameGroupName.value = groupContextTarget.value.name
  showRenameGroupDialog.value = true
}

function confirmRenameGroup() {
  const name = renameGroupName.value.trim()
  if (!name || !groupContextTarget.value) return
  connectionStore.renameGroup(groupContextTarget.value.id, name)
  showRenameGroupDialog.value = false
}

const showNewGroupDialog = ref(false)
const newGroupDialogName = ref('')

async function doAddGroup() {
  const name = newGroupDialogName.value.trim()
  if (!name) return
  if (connectionStore.groups.some(g => g.name === name)) {
    msg.warning(t('conn.groupNameDuplicate'))
    return
  }
  await connectionStore.addGroup(name)
  newGroupDialogName.value = ''
  showNewGroupDialog.value = false
}

async function doDeleteGroup() {
  if (!groupContextTarget.value) return
  closeContextMenu()
  const g = groupContextTarget.value
  const count = connectionStore.connections.filter(c => c.groupId === g.id).length
  if (count === 0) {
    await connectionStore.deleteGroup(g.id, 'move-out')
    return
  }
  deleteGroupTarget.value = g
  showDeleteGroupDialog.value = true
}

const showDeleteGroupDialog = ref(false)
const deleteGroupTarget = ref<ConnectionGroup | null>(null)
const deleteGroupPromptText = computed(() => {
  const g = deleteGroupTarget.value
  if (!g) return ''
  const count = connectionStore.connections.filter(c => c.groupId === g.id).length
  return t('conn.deleteGroupPrompt', { name: g.name, count })
})

async function confirmDeleteGroup(action: 'delete-connections' | 'move-out') {
  if (deleteGroupTarget.value) {
    await connectionStore.deleteGroup(deleteGroupTarget.value.id, action)
  }
  showDeleteGroupDialog.value = false
  deleteGroupTarget.value = null
}

function onDocumentClick() {
  contextMenuVisible.value = false
  groupContextVisible.value = false
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick)
  window.addEventListener('keydown', onKeydown)
  updateContentWidth()
  if (startTabRef.value) {
    resizeObserver = new ResizeObserver(() => updateContentWidth())
    resizeObserver.observe(startTabRef.value)
  }
  nextTick(() => searchInputRef.value?.focus())
})

watch(focusedCardIndex, () => {
  nextTick(() => {
    const el = document.querySelector('.start-card.focused') as HTMLElement | null
    el?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  })
})

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
  window.removeEventListener('keydown', onKeydown)
  resizeObserver?.disconnect()
})

// Context menu actions
function doConnect(config: ConnectionConfig, e: MouseEvent) { closeContextMenu(); emit('connect', config, e.ctrlKey || e.metaKey) }
function doConnectSerial(_config: ConnectionConfig, e: MouseEvent) { closeContextMenu(); emit('connect-serial', e.ctrlKey || e.metaKey) }
function doConnectSftp(config: ConnectionConfig) { closeContextMenu(); window.dispatchEvent(new CustomEvent('app:connect-sftp', { detail: config })) }
function doConnectMonitor(config: ConnectionConfig) { closeContextMenu(); window.dispatchEvent(new CustomEvent('app:connect-monitor', { detail: config })) }
function doConnectRdp(config: ConnectionConfig) { closeContextMenu(); window.dispatchEvent(new CustomEvent('app:connect-rdp', { detail: config })) }
function doConnectVnc(config: ConnectionConfig) { closeContextMenu(); window.dispatchEvent(new CustomEvent('app:connect-vnc', { detail: config })) }
function doConnectSpice(config: ConnectionConfig) { closeContextMenu(); window.dispatchEvent(new CustomEvent('app:connect-spice', { detail: config })) }
function doConnectDb(config: ConnectionConfig) { closeContextMenu(); window.dispatchEvent(new CustomEvent('app:connect-db', { detail: config })) }
function doConnectFtp(config: ConnectionConfig) { closeContextMenu(); window.dispatchEvent(new CustomEvent('app:connect-ftp', { detail: config })) }
function doConnectSmb(config: ConnectionConfig) { closeContextMenu(); window.dispatchEvent(new CustomEvent('app:connect-smb', { detail: config })) }
function doConnectS3(config: ConnectionConfig) { closeContextMenu(); window.dispatchEvent(new CustomEvent('app:connect-s3', { detail: config })) }
function doConnectWebdav(config: ConnectionConfig) { closeContextMenu(); window.dispatchEvent(new CustomEvent('app:connect-webdav', { detail: config })) }
function doEditConnection(config: ConnectionConfig | null) {
  if (!config) return
  closeContextMenu()
  emit('edit-connection', config)
}

function doChangeGroupBulk() {
  const ids = getSelectedConnectionIds()
  if (ids.length === 0) return
  closeContextMenu()
  emit('change-group-ids', ids)
}

async function doDeleteBulk() {
  const ids = getSelectedConnectionIds()
  if (ids.length === 0) return
  closeContextMenu()
  try {
    await ElMessageBox.confirm(
      t('sidebar.deleteConfirm', { count: ids.length }),
      '',
      { confirmButtonText: t('sidebar.delete'), cancelButtonText: 'Cancel', type: 'warning' }
    )
  } catch {
    return
  }
  await connectionStore.removeMany(ids)
  selectedIds.value = new Set()
}

function doChangeGroup(config: ConnectionConfig | null) {
  if (!config) return
  closeContextMenu()
  emit('change-group', config)
}

function doDuplicate(config: ConnectionConfig | null) {
  if (!config) return
  closeContextMenu()
  const newConfig = { ...config, id: '', name: config.name + ' (Copy)' }
  connectionStore.add(newConfig)
}
async function doDelete(config: ConnectionConfig | null) {
  if (!config) return
  closeContextMenu()
  try {
    await ElMessageBox.confirm(
      `${t('sidebar.deleteConfirm', { count: 1 })}`,
      '',
      { confirmButtonText: t('sidebar.delete'), cancelButtonText: 'Cancel', type: 'warning' }
    )
    connectionStore.remove(config.id)
  } catch { /* cancelled */ }
}
</script>

<style scoped>
.start-tab {
  padding: 32px 64px 32px 64px;
  height: 100%;
  overflow-y: auto;
  outline: none;
}

.start-content {
  margin: 0 auto;
}

.start-brand {
  text-align: center;
  font-size: 32px;
  font-weight: 700;
  color: var(--accent);
  margin-top: 64px;
  margin-bottom: 36px;
  user-select: none;
}

.start-search-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 20px;
}

.start-filter-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 8px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: var(--bg-surface);
  color: var(--text-secondary);
  cursor: pointer;
  font-size: 13px;
  white-space: nowrap;
  user-select: none;
}
.start-filter-btn:hover,
.start-filter-btn.active {
  background: var(--bg-hover);
  color: var(--accent);
}

.start-search-input {
  flex: 1;
}
</style>

<style>
.start-search-input .el-input__wrapper {
  background-color: var(--bg-surface) !important;
  box-shadow: 0 0 0 1px var(--border-subtle) inset !important;
  padding: 4px 14px !important;
  border-radius: var(--radius-md) !important;
}
.start-search-input .el-input__wrapper.is-focus {
  box-shadow: 0 0 0 1px var(--accent) inset !important;
}
.start-search-input .el-input__inner {
  font-family: inherit !important;
  font-size: 13px !important;
  color: var(--text-primary) !important;
}
.start-search-input .el-input__inner::placeholder {
  color: var(--text-disabled) !important;
}

.start-action-btns {
  display: flex;
  gap: 10px;
  margin-bottom: 28px;
  align-items: flex-start;
}

.start-action-btn {
  padding: 8px 20px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: var(--bg-surface);
  color: var(--text-secondary);
  cursor: pointer;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 6px;
  transition: background 0.15s;
}
.start-action-btn:hover {
  background: var(--bg-hover);
}
.start-action-btn.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bg-base);
}
.start-action-btn.primary:hover {
  filter: brightness(0.9);
}

.start-breadcrumb {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 10px;
  font-size: 12px;
  color: var(--text-disabled);
}
.start-breadcrumb .link {
  color: var(--accent);
  cursor: pointer;
}
.start-breadcrumb .link:hover {
  text-decoration: underline;
}
.start-breadcrumb .sep {
  color: var(--text-disabled);
}
.start-breadcrumb .current {
  color: var(--text-primary);
  font-weight: 600;
}

.start-section-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-disabled);
  text-transform: uppercase;
  letter-spacing: 1px;
  margin-top: 24px;
  margin-bottom: 10px;
}
.start-add-group-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 4px;
  cursor: pointer;
  color: var(--text-disabled);
  transition: background 0.15s, color 0.15s;
}
.start-add-group-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.start-divider {
  border: none;
  border-top: 1px solid var(--border-subtle);
  margin: 20px 0;
}

.start-cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, 240px);
  gap: 12px;
}

.start-card {
  position: relative;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  padding: 8px 12px;
  cursor: pointer;
  transition: border-color 0.15s;
  width: 240px;
}
.start-card:hover {
  border-color: var(--accent);
}
.card-more-btn {
  display: none;
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  right: 6px;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  background: var(--bg-elevated);
  color: var(--text-muted);
  cursor: pointer;
  border-radius: var(--radius-sm);
  padding: 0;
  z-index: 2;
}
.start-card:hover .card-more-btn {
  display: flex;
}
.card-more-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.start-card.dimmed {
  opacity: 0.35;
}
.start-card.focused {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}
.start-card.selected {
  border-color: var(--accent);
  background: var(--accent-subtle);
}

.start-card-top {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.start-card-top > div:last-child {
  overflow: hidden;
  min-width: 0;
  flex: 1;
}

.start-card-icon {
  width: 36px;
  height: 36px;
  border-radius: 7px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  background: var(--bg-overlay);
  color: var(--text-secondary);
}
.start-card-icon.ssh,
.start-card-icon.telnet,
.start-card-icon.mosh { color: var(--accent); }
.start-card-icon.local { color: var(--success); }
.start-card-icon.database { color: var(--warning); }
.start-card-icon.rdp,
.start-card-icon.vnc,
.start-card-icon.spice { color: var(--accent-dim); }
.start-card-icon.serial { color: var(--success-dim); }
.start-card-icon.group { color: var(--text-secondary); }
.start-card-icon.ungrouped { color: var(--text-muted); }
.start-card-icon.quick { background: var(--accent-subtle); color: var(--accent); }

.start-card-name {
  font-weight: 600;
  font-size: 12px;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 172px;
}
.start-card-meta {
  margin-top: 3px;
  font-size: 10px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.start-quick-card {
  background: transparent;
  border: 1px dashed var(--accent);
  border-radius: var(--radius-lg);
  padding: 8px 12px;
  cursor: pointer;
  transition: background 0.15s;
  margin-top: 12px;
  width: 240px;
}
.start-quick-card.focused {
  border-style: solid;
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}
.start-quick-card:hover {
  background: var(--accent-subtle);
}
.quick-name {
  color: var(--accent);
}

.start-empty-hint,
.start-empty-state {
  text-align: center;
  color: var(--text-disabled);
  font-size: 14px;
  margin-top: 48px;
}
.empty-icon {
  font-size: 48px;
  display: block;
  margin-bottom: 16px;
  opacity: 0.3;
}

.start-context-menu {
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
.start-context-menu .menu-item {
  padding: 7px 14px;
  font-size: 12px;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  transition: all 0.1s ease;
}
.start-context-menu .menu-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.start-context-menu .menu-item.danger {
  color: var(--error);
}
.start-context-menu .menu-item.danger:hover {
  background: var(--bg-hover);
  color: var(--error);
}
.start-context-menu .menu-item.disabled {
  color: var(--text-disabled);
  cursor: default;
  pointer-events: none;
}
.start-context-menu .menu-divider {
  border: none;
  border-top: 1px solid var(--border-subtle);
  margin: 4px;
}
</style>
