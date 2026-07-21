<template>
  <div class="db-tree-panel">

    <div class="search-wrap">
      <input
        v-model="searchQuery"
        class="search-input"
        :placeholder="t('db.searchTables')"
      />
    </div>
    <div ref="treeContentRef" class="tree-content" @click="closeContextMenu" @contextmenu.prevent="onTreeContextMenu">
      <div v-if="loading" class="tree-loading">{{ t('db.loading') }}</div>
      <template v-for="db in filteredDbs" :key="db.name">
        <div
          class="db-header"
          :data-db-row="db.name"
          :class="{ selected: selectedDb === db.name && !selectedTable }"
          @click="onDbClick(db.name)"
          @dblclick="emit('openDatabase', db.name)"
          @contextmenu.prevent="onDbContextMenu($event, db.name)"
        >
          <span class="db-arrow" @click.stop="onToggleDb(db.name)">
            <component :is="expandedDbs.has(db.name) ? ChevronDown : ChevronRight" :size="12" />
          </span>
          <Database class="db-icon" :size="14" />
          <span class="db-name">{{ db.name }}</span>
        </div>
        <template v-if="expandedDbs.has(db.name)">
          <div
            v-for="t in db.tables"
            :key="t.name"
            class="table-item"
            :data-table-row="db.name + '/' + t.name"
            :class="{ selected: selectedTable === t.name && selectedDb === db.name }"
            @click="onTableClick(db.name, t.name)"
            @dblclick="onTableDblClick(db.name, t)"
            @contextmenu.prevent="onTableContextMenu($event, db.name, t)"
          >
            <span class="table-icon-spacer" />
            <component :is="t.type === 'view' ? Eye : Table2" class="table-icon" :size="14" />
            <span class="table-name">{{ t.name }}</span>
          </div>
          <div v-if="db.tables.length === 0" class="empty-hint">
            {{ t('db.noTables') }}
          </div>
        </template>
      </template>
    </div>

    <!-- Context menu -->
    <div
      v-if="ctxVisible"
      class="ctx-menu"
      :style="{ left: ctxX + 'px', top: ctxY + 'px' }"
      @click.stop
    >
      <template v-if="ctxTargetType === 'db'">
        <div class="ctx-item" @click="onCtxQueryDatabase">{{ t('db.dataQuery') }}</div>
        <div class="ctx-item" @click="onCtxListDatabase">{{ t('db.tableList') }}</div>
        <div class="ctx-sep" />
        <div v-if="canCreateDatabase" class="ctx-item" @click="onCtxNewDatabase">{{ t('db.newDatabase') }}</div>
        <div class="ctx-item" @click="onCtxNewTable">{{ t('db.newTable') }}</div>
        <div class="ctx-item danger" @click="onCtxDropDatabase">{{ t('db.dropDatabase') }}</div>
        <div class="ctx-sep" />
        <div class="ctx-item" @click="onCtxRefresh">{{ t('db.refreshTables') }}</div>
      </template>
      <template v-else-if="ctxTargetType === 'table'">
        <div class="ctx-item" @click="onCtxViewData">{{ t('db.dataQuery') }}</div>
        <div v-if="ctxTableType !== 'view'" class="ctx-item" @click="onCtxViewStructure">{{ t('db.tableStructure') }}</div>
        <div class="ctx-sep" />
        <div class="ctx-item" @click="onCtxCopyName">{{ t('db.copyName') }}</div>
        <div class="ctx-sep" />
        <template v-if="ctxTableType === 'view'">
          <div class="ctx-item danger" @click="onCtxDropView">{{ t('db.dropView') }}</div>
        </template>
        <template v-else>
          <div class="ctx-item danger" @click="onCtxTruncateTable">{{ t('db.truncateTable') }}</div>
          <div class="ctx-item danger" @click="onCtxDropTable">{{ t('db.dropTable') }}</div>
        </template>
      </template>
      <template v-else-if="ctxTargetType === 'blank'">
        <div v-if="canCreateDatabase" class="ctx-item" @click="onCtxNewDatabase">{{ t('db.newDatabase') }}</div>
        <div class="ctx-item" @click="onCtxRefreshDatabases">{{ t('db.refreshDatabases') }}</div>
      </template>
    </div>

    <!-- Confirm dialog -->
    <el-dialog append-to-body
      v-model="confirmVisible"
      :title="confirmTitle"
      width="420px"
    >
      <div class="confirm-body">
        <p class="confirm-text">{{ confirmText }}</p>
        <p class="confirm-hint">{{ t('db.typeToConfirm', { name: confirmName }) }}</p>
        <el-input v-model="confirmInput" :placeholder="confirmName" />
      </div>
      <template #footer>
        <el-button @click="confirmVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :disabled="confirmInput !== confirmName" @click="onConfirm">
          {{ t('common.confirm') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- New Database dialog -->
    <el-dialog append-to-body v-model="newDbVisible" :title="t('db.newDatabase')" width="380px">
      <el-form label-width="80px">
        <el-form-item :label="t('db.dbName')">
          <el-input v-model="newDbName" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="newDbVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="!newDbName.trim()" @click="onCreateDatabase">
          {{ t('common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- New Table dialog -->
    <el-dialog append-to-body v-model="newTableVisible" :title="t('db.newTable')" width="380px">
      <el-form label-width="80px">
        <el-form-item :label="t('db.tableName')">
          <el-input v-model="newTableName" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="newTableVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="!newTableName.trim()" @click="onCreateTable">
          {{ t('common.save') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed, nextTick, onMounted, onUnmounted } from 'vue'
import { Database, Table2, Eye, ChevronRight, ChevronDown } from '@lucide/vue'
import { useI18n } from '../i18n'
import { GetDatabases, GetTables, CreateDatabase, DropDatabase, CreateTable, DropTable, DropView, TruncateTable, GetDBCapabilities } from '../../wailsjs/go/main/App'
import { msg } from '../services/message'
import type { TableInfo } from '../types/database'

const { t } = useI18n()

interface DbEntry {
  name: string
  tables: TableInfo[]
  loaded: boolean
}

const props = defineProps<{
  sessionId: string
  defaultDbName?: string
  activeDb?: string
  activeTable?: string
}>()

const caps = ref<Record<string, any> | null>(null)
const canCreateDatabase = computed(() => caps.value?.['supportsCreateDatabase'] ?? true)

async function loadCapabilities() {
  try {
    caps.value = await GetDBCapabilities(props.sessionId)
  } catch (e) {
    console.error('Failed to load capabilities:', e)
  }
}

watch(() => props.sessionId, () => {
  if (props.sessionId) loadCapabilities()
}, { immediate: true })

const emit = defineEmits<{
  selectTable: [dbName: string, tableName: string, isView?: boolean]
  openDatabase: [dbName: string, tab?: 'query' | 'objects']
  viewStructure: [dbName: string, tableName: string]
}>()

const databases = ref<DbEntry[]>([])
const expandedDbs = ref(new Set<string>())
const selectedDb = ref('')
const selectedTable = ref('')
const searchQuery = ref('')
const loading = ref(false)
const treeContentRef = ref<HTMLElement | null>(null)

async function loadTree() {
  if (!props.sessionId) return
  loading.value = true
  try {
    const dbs = await GetDatabases(props.sessionId)
    // Auto-expand the connection's default db only when it's actually a real
    // entry in the list. For databases where the configured "dbName" is not a
    // browsable namespace (e.g. Oracle's service name), it won't match and we
    // just list the available schemas collapsed.
    if (props.defaultDbName && dbs.includes(props.defaultDbName)) {
      const tables = await GetTables(props.sessionId, props.defaultDbName)
      databases.value = [{ name: props.defaultDbName, tables, loaded: true }]
      expandedDbs.value = new Set([props.defaultDbName])
    } else if (dbs.length === 1) {
      // Single schema/db (e.g. Oracle showing only the current schema) — expand it.
      const only = dbs[0]
      const tables = await GetTables(props.sessionId, only)
      databases.value = [{ name: only, tables, loaded: true }]
      expandedDbs.value = new Set([only])
    } else {
      databases.value = dbs.map((db: string) => ({ name: db, tables: [], loaded: false }))
      expandedDbs.value = new Set()
    }
  } catch (e) {
    console.error('Failed to load tree:', e)
  } finally {
    loading.value = false
  }
}

watch(() => props.sessionId, (newId) => {
  if (newId) loadTree()
}, { immediate: true })

// Sync tree highlight/position when the active db/table changes from outside
// (breadcrumb navigation, object list, etc.)
watch(() => [props.activeDb, props.activeTable], async ([db, table]) => {
  if (!db) return
  selectedDb.value = db
  selectedTable.value = table || ''
  if (table && !expandedDbs.value.has(db)) {
    expandedDbs.value.add(db)
    const entry = databases.value.find(d => d.name === db)
    if (entry && !entry.loaded) {
      try {
        entry.tables = await GetTables(props.sessionId, db)
        entry.loaded = true
      } catch { /* ignore */ }
    }
    expandedDbs.value = new Set(expandedDbs.value)
  }
  await nextTick()
  scrollActiveIntoView()
})

function scrollActiveIntoView() {
  const root = treeContentRef.value
  if (!root || !selectedDb.value) return
  const key = selectedTable.value ? `${selectedDb.value}/${selectedTable.value}` : selectedDb.value
  const attr = selectedTable.value ? 'data-table-row' : 'data-db-row'
  const esc = key.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
  const el = root.querySelector(`[${attr}="${esc}"]`) as HTMLElement | null
  el?.scrollIntoView({ block: 'nearest' })
}

function onDbClick(dbName: string) {
  selectedDb.value = dbName
  selectedTable.value = ''
}

async function onToggleDb(dbName: string) {
  if (expandedDbs.value.has(dbName)) {
    expandedDbs.value.delete(dbName)
  } else {
    expandedDbs.value.add(dbName)
    const db = databases.value.find(d => d.name === dbName)
    if (db && !db.loaded) {
      try {
        db.tables = await GetTables(props.sessionId, dbName)
        db.loaded = true
      } catch (e) {
        console.error('Failed to load tables:', e)
      }
    }
  }
  expandedDbs.value = new Set(expandedDbs.value)
}

function onTableClick(dbName: string, tableName: string) {
  selectedDb.value = dbName
  selectedTable.value = tableName
}

function onTableDblClick(dbName: string, table: TableInfo) {
  emit('selectTable', dbName, table.name, table.type === 'view')
}

const filteredDbs = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return databases.value
  return databases.value.map(db => ({
    ...db,
    tables: db.tables.filter(t => t.name.toLowerCase().includes(q))
  }))
})

watch(searchQuery, (q) => {
  if (q.trim()) {
    const all = new Set(databases.value.map(d => d.name))
    for (const db of databases.value) {
      if (!db.loaded) {
        GetTables(props.sessionId, db.name).then(tables => {
          db.tables = tables
          db.loaded = true
        }).catch(() => {})
      }
    }
    expandedDbs.value = all
  }
})

// ── Context menu ──

const ctxVisible = ref(false)
const ctxX = ref(0)
const ctxY = ref(0)
const ctxTargetType = ref('')
const ctxDbName = ref('')
const ctxTableName = ref('')
const ctxTableType = ref('')

function closeContextMenu() {
  ctxVisible.value = false
}

function fitContextMenu(x: number, y: number, type: string) {
  const heights: Record<string, number> = { db: 210, table: 180, blank: 60 }
  const menuW = 160
  const menuH = heights[type] || 150

  let left = x
  let top = y

  if (left + menuW > window.innerWidth) left = x - menuW
  if (left < 0) left = 4

  if (top + menuH > window.innerHeight) top = y - menuH
  if (top < 0) top = window.innerHeight - menuH - 4
  if (top < 0) top = 4

  return { left, top }
}

function onDbContextMenu(e: MouseEvent, dbName: string) {
  ctxTargetType.value = 'db'
  ctxDbName.value = dbName
  ctxTableName.value = ''
  const pos = fitContextMenu(e.clientX, e.clientY, 'db')
  ctxX.value = pos.left
  ctxY.value = pos.top
  ctxVisible.value = true
}

function onTableContextMenu(e: MouseEvent, dbName: string, table: TableInfo) {
  ctxTargetType.value = 'table'
  ctxDbName.value = dbName
  ctxTableName.value = table.name
  ctxTableType.value = table.type || ''
  const pos = fitContextMenu(e.clientX, e.clientY, 'table')
  ctxX.value = pos.left
  ctxY.value = pos.top
  ctxVisible.value = true
}

function onTreeContextMenu(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (target.closest('.db-header') || target.closest('.table-item')) return
  ctxTargetType.value = 'blank'
  ctxDbName.value = ''
  ctxTableName.value = ''
  const pos = fitContextMenu(e.clientX, e.clientY, 'blank')
  ctxX.value = pos.left
  ctxY.value = pos.top
  ctxVisible.value = true
}

function onCtxQueryDatabase() {
  emit('openDatabase', ctxDbName.value, 'query')
  ctxVisible.value = false
}

function onCtxListDatabase() {
  emit('openDatabase', ctxDbName.value, 'objects')
  ctxVisible.value = false
}

function onCtxViewData() {
  emit('selectTable', ctxDbName.value, ctxTableName.value, ctxTableType.value === 'view')
  ctxVisible.value = false
}

function onCtxViewStructure() {
  emit('viewStructure', ctxDbName.value, ctxTableName.value)
  ctxVisible.value = false
}

function onCtxCopyName() {
  navigator.clipboard.writeText(ctxTableName.value).catch(() => {})
  ctxVisible.value = false
}

// ── Confirm dialog ──

const confirmVisible = ref(false)
const confirmTitle = ref('')
const confirmText = ref('')
const confirmName = ref('')
const confirmInput = ref('')
let confirmAction: (() => Promise<void>) | null = null

function showConfirm(title: string, text: string, name: string, action: () => Promise<void>) {
  confirmTitle.value = title
  confirmText.value = text
  confirmName.value = name
  confirmInput.value = ''
  confirmAction = action
  confirmVisible.value = true
  ctxVisible.value = false
}

async function onConfirm() {
  if (confirmAction) {
    try {
      await confirmAction()
      await loadTree()
    } catch (e: any) {
      console.error(e)
      msg.error(e?.message || String(e))
    }
  }
  confirmVisible.value = false
}

function onCtxDropDatabase() {
  showConfirm(
    t('db.dropDatabase'),
    t('db.dropDatabaseConfirm', { name: ctxDbName.value }),
    ctxDbName.value,
    async () => { await DropDatabase(props.sessionId, ctxDbName.value) }
  )
}

function onCtxDropTable() {
  showConfirm(
    t('db.dropTable'),
    t('db.dropTableConfirm', { name: ctxTableName.value }),
    ctxTableName.value,
    async () => { await DropTable(props.sessionId, ctxDbName.value, ctxTableName.value) }
  )
}

function onCtxDropView() {
  showConfirm(
    t('db.dropView'),
    t('db.dropViewConfirm', { name: ctxTableName.value }),
    ctxTableName.value,
    async () => { await DropView(props.sessionId, ctxDbName.value, ctxTableName.value) }
  )
}

function onCtxTruncateTable() {
  showConfirm(
    t('db.truncateTable'),
    t('db.truncateTableConfirm', { name: ctxTableName.value }),
    ctxTableName.value,
    async () => { await TruncateTable(props.sessionId, ctxDbName.value, ctxTableName.value) }
  )
}

async function onCtxRefresh() {
  ctxVisible.value = false
  const db = databases.value.find(d => d.name === ctxDbName.value)
  if (db) {
    try {
      db.tables = await GetTables(props.sessionId, ctxDbName.value)
      db.loaded = true
    } catch (e) {
      console.error('Failed to refresh:', e)
    }
  }
}

async function onCtxRefreshDatabases() {
  ctxVisible.value = false
  await loadTree()
}

// Refresh a single database's tables (called from parent after external mutations)
async function refreshDb(dbName: string) {
  const db = databases.value.find(d => d.name === dbName)
  if (db) {
    try {
      db.tables = await GetTables(props.sessionId, dbName)
      db.loaded = true
    } catch (e) {
      console.error('Failed to refresh db:', e)
    }
  }
}

defineExpose({ refreshDb })

// ── New Database / Table dialogs ──

const newDbVisible = ref(false)
const newDbName = ref('')

function onCtxNewDatabase() {
  ctxVisible.value = false
  newDbName.value = ''
  newDbVisible.value = true
}

async function onCreateDatabase() {
  if (!newDbName.value.trim()) return
  try {
    await CreateDatabase(props.sessionId, newDbName.value.trim())
    newDbVisible.value = false
    await loadTree()
  } catch (e: any) {
    console.error('Failed to create database:', e)
    msg.error(e?.message || String(e))
  }
}

const newTableVisible = ref(false)
const newTableName = ref('')

function onCtxNewTable() {
  ctxVisible.value = false
  newTableName.value = ''
  newTableVisible.value = true
}

async function onCreateTable() {
  if (!newTableName.value.trim()) return
  try {
    await CreateTable(props.sessionId, ctxDbName.value, newTableName.value.trim())
    newTableVisible.value = false
    const db = databases.value.find(d => d.name === ctxDbName.value)
    if (db) {
      db.tables = await GetTables(props.sessionId, ctxDbName.value)
      db.loaded = true
      expandedDbs.value = new Set([...expandedDbs.value, ctxDbName.value])
    }
  } catch (e: any) {
    console.error('Failed to create table:', e)
    msg.error(e?.message || String(e))
  }
}

// Close context menu on click outside
onMounted(() => {
  document.addEventListener('click', closeContextMenu)
})
onUnmounted(() => {
  document.removeEventListener('click', closeContextMenu)
})
</script>

<style scoped>
.db-tree-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.panel-header {
  padding: 8px 12px 4px;
  font-family: var(--font-ui);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  flex-shrink: 0;
}
.search-wrap {
  padding: 4px 8px;
  flex-shrink: 0;
}
.search-input {
  width: 100%;
  padding: 4px 8px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-base);
  color: var(--text-primary);
  font-family: var(--font-ui);
  font-size: 12px;
  outline: none;
  transition: border-color 0.15s ease;
}
.search-input:focus {
  border-color: var(--accent);
}
.search-input::placeholder {
  color: var(--text-muted);
}
.tree-content {
  flex: 1;
  overflow: auto;
}
.tree-loading {
  padding: 12px;
  color: var(--text-secondary);
  font-family: var(--font-ui);
  font-size: 12px;
  text-align: center;
}
.db-header {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 8px;
  cursor: pointer;
  user-select: none;
  transition: background 0.12s ease;
}
.db-header:hover {
  background: var(--bg-hover);
}
.db-header.selected {
  background: var(--bg-hover);
}
.db-arrow {
  width: 12px;
  flex-shrink: 0;
  color: var(--text-muted);
  display: flex;
  align-items: center;
  cursor: pointer;
}
.db-arrow:hover {
  color: var(--text-primary);
}
.db-icon {
  flex-shrink: 0;
  color: var(--text-muted);
}
.db-name {
  font-family: var(--font-ui);
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.table-item {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 8px;
  cursor: pointer;
  user-select: none;
  transition: background 0.12s ease;
}
.table-item:hover {
  background: var(--bg-hover);
}
.table-item.selected {
  background: var(--bg-hover);
}
.table-icon-spacer {
  width: 30px;
  flex-shrink: 0;
}
.table-icon {
  flex-shrink: 0;
  color: var(--text-muted);
}
.table-name {
  font-family: var(--font-ui);
  font-size: 13px;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.empty-hint {
  padding: 4px 8px 4px 28px;
  font-family: var(--font-ui);
  font-size: 12px;
  color: var(--text-muted);
}

/* Context menu */
.ctx-menu {
  position: fixed;
  z-index: 1000;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 4px 0;
  min-width: 150px;
  box-shadow: var(--shadow-md);
}
.ctx-item {
  padding: 6px 12px;
  font-family: var(--font-ui);
  font-size: 13px;
  color: var(--text-primary);
  cursor: pointer;
  transition: background 0.1s ease;
}
.ctx-item:hover {
  background: var(--bg-hover);
}
.ctx-item.danger {
  color: var(--error);
}
.ctx-sep {
  height: 1px;
  background: var(--border-subtle);
  margin: 4px 0;
}

/* Confirm dialog */
.confirm-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.confirm-text {
  font-family: var(--font-ui);
  font-size: 14px;
  color: var(--text-primary);
  margin: 0;
}
.confirm-hint {
  font-family: var(--font-ui);
  font-size: 12px;
  color: var(--text-muted);
  margin: 0;
}
</style>
