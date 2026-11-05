<template>
  <div class="db-tree-panel">


    <div class="search-wrap">
      <input
        v-model="searchQuery"
        class="search-input"
        :placeholder="t('db.searchTables')"
      />
      <button class="btn btn-ghost btn-icon btn-sm" :title="t('db.refreshDatabases')" @click="refreshAll">
        <RefreshCw :size="'0.875rem'" />
      </button>
      <button class="btn btn-ghost btn-icon btn-sm" :title="t('common.more')" @click.stop="moreMenuRef?.toggle($event.currentTarget)">
        <MoreHorizontal :size="'0.875rem'" />
      </button>
      <Menu ref="moreMenuRef" v-model:visible="moreMenuVisible" align="end">
        <MenuItem :class="{ disabled: !canCreateDatabase }" @click="onMoreNewDatabase">{{ t('db.newDatabase') }}</MenuItem>
        <MenuItem @click="onMoreNewTable">{{ t('db.newTable') }}</MenuItem>
        <MenuDivider />
        <MenuItem @click="onMoreRefresh">{{ t('db.refreshDatabases') }}</MenuItem>
      </Menu>
    </div>
    <div ref="treeContentRef" class="tree-content" @scroll.passive="onTreeScroll" @contextmenu.prevent="onTreeContextMenu">
      <div v-if="loading || runningSqlFile" class="tree-loading">{{ t('db.loading') }}</div>
      <div class="virt-pad" :style="{ height: topPad + 'px' }"></div>
      <template v-for="row in visibleRows" :key="row.key">
        <div
          v-if="row.kind === 'db'"
          class="db-header"
          :data-db-row="row.db.name"
          :class="{ selected: selectedDb === row.db.name && !selectedTable }"
          @click="onDbClick(row.db.name)"
          @dblclick="onDbActivate(row.db.name)"
          @contextmenu.prevent="onDbContextMenu($event, row.db.name)"
        >
          <span class="db-arrow" @click.stop="onToggleDb(row.db.name)">
            <component :is="expandedDbs.has(row.db.name) ? ChevronDown : ChevronRight" :size="'0.75rem'" />
          </span>
          <Database class="db-icon" :size="'0.875rem'" />
          <span class="db-name">{{ row.db.name }}</span>
        </div>
        <div
          v-else-if="row.kind === 'table' && row.table"
          class="table-item"
          :data-table-row="row.db.name + '/' + row.table.name"
          :class="{ selected: selectedTable === row.table.name && selectedDb === row.db.name }"
          @click="onTableClick(row.db.name, row.table.name)"
          @dblclick="onTableDblClick(row.db.name, row.table)"
          @contextmenu.prevent="onTableContextMenu($event, row.db.name, row.table)"
        >
          <span class="table-icon-spacer" />
          <component :is="row.table.type === 'view' ? Eye : Table2" class="table-icon" :size="'0.875rem'" />
          <span class="table-name" @mouseenter="onTableNameHover($event, row.table.name)">{{ row.table.name }}</span>
        </div>
        <div v-else-if="row.kind === 'empty'" class="empty-hint">
          {{ t('db.noTables') }}
        </div>
        <div v-else-if="row.kind === 'more'" class="tree-more">
          <span class="tree-more-info">
            共 {{ row.total }} 张表，已显示 {{ row.loaded }}，加载中…
          </span>
        </div>
        <!-- unloaded slots render as blank rows so the scroll position stays stable -->
        <div v-else-if="row.kind === 'blank'" class="table-item table-item-blank"></div>
      </template>
      <div class="virt-pad" :style="{ height: bottomPad + 'px' }"></div>
    </div>

    <Menu
      ref="ctxMenuRef"
      v-model:visible="ctxMenuVisible"
      v-slot="{ current }"
    >
      <template v-if="current && current.type === 'db'">
        <MenuItem @click="onCtxListDatabase">{{ t('db.tableList') }}</MenuItem>
        <MenuItem @click="onCtxNewQuery">{{ t('db.newQuery') }}</MenuItem>
        <MenuItem @click="onCtxRunSqlFile">{{ t('db.runSqlFile') }}</MenuItem>
        <MenuDivider />
        <MenuItem v-if="canCreateDatabase" @click="onCtxNewDatabase">{{ t('db.newDatabase') }}</MenuItem>
        <MenuItem @click="onCtxNewTable">{{ t('db.newTable') }}</MenuItem>
        <MenuItem class="danger" @click="onCtxDropDatabase">{{ t('db.dropDatabase') }}</MenuItem>
        <MenuDivider />
        <MenuItem @click="onCtxRefresh">{{ t('db.refreshTables') }}</MenuItem>
      </template>
      <template v-else-if="current && current.type === 'table'">
        <MenuItem @click="onCtxViewData">{{ t('db.openData') }}</MenuItem>
        <MenuItem v-if="current.tableType !== 'view'" @click="onCtxViewStructure">{{ t('db.tableStructure') }}</MenuItem>
        <MenuDivider />
        <MenuItem @click="onCtxCopyName">{{ t('db.copyName') }}</MenuItem>
        <MenuItem v-if="(current as DbMenuCtx).tableType !== 'view'" @click="onCtxCopyTable">{{ t('db.copyTable') }}</MenuItem>
        <MenuDivider />
        <MenuItem @click="onCtxExportStructure">{{ t('db.exportStructure') }}</MenuItem>
        <MenuItem v-if="current.tableType !== 'view'" @click="onCtxExportData">{{ t('db.exportData') }}</MenuItem>
        <MenuItem v-if="current.tableType !== 'view'" @click="onCtxExportStructureData">{{ t('db.exportStructureData') }}</MenuItem>
        <MenuDivider />
        <template v-if="current.tableType === 'view'">
          <MenuItem class="danger" @click="onCtxDropView">{{ t('db.dropView') }}</MenuItem>
        </template>
        <template v-else>
          <MenuItem class="danger" @click="onCtxTruncateTable">{{ t('db.truncateTable') }}</MenuItem>
          <MenuItem class="danger" @click="onCtxDropTable">{{ t('db.dropTable') }}</MenuItem>
        </template>
      </template>
      <template v-else-if="current && current.type === 'blank'">
        <MenuItem @click="onCtxNewQueryBlank">{{ t('db.newQuery') }}</MenuItem>
        <MenuItem v-if="canCreateDatabase" @click="onCtxNewDatabase">{{ t('db.newDatabase') }}</MenuItem>
        <MenuItem @click="onCtxRefreshDatabases">{{ t('db.refreshDatabases') }}</MenuItem>
      </template>
    </Menu>

    <!-- Confirm dialog -->
    <el-dialog append-to-body
      v-model="confirmVisible"
      :title="confirmTitle"
      width="26.25rem"
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
    <el-dialog append-to-body v-model="newDbVisible" :title="t('db.newDatabase')" width="23.75rem">
      <el-form label-width="5rem">
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
    <el-dialog append-to-body v-model="newTableVisible" :title="t('db.newTable')" width="23.75rem">
      <el-form label-width="5rem">
        <el-form-item :label="t('db.tableName')">
          <el-input v-model="newTableName" />
        </el-form-item>
        <el-form-item :label="t('db.databases')">
          <el-select v-model="newTableDb" style="width:100%">
            <el-option v-for="d in databases" :key="d.name" :label="d.name" :value="d.name" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="newTableVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="!newTableName.trim() || !newTableDb" @click="onCreateTable">
          {{ t('common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Copy Table dialog -->
    <el-dialog append-to-body v-model="copyTableVisible" :title="t('db.copyTable')" width="23.75rem">
      <el-form label-width="5rem">
        <el-form-item :label="t('db.sourceTable')">
          <el-input :model-value="copySourceTable" disabled />
        </el-form-item>
        <el-form-item :label="t('db.tableName')">
          <el-input v-model="copyTableName" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="copyTableVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="!copyTableName.trim()" @click="onCopyTable">
          {{ t('common.confirm') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed, nextTick, h } from 'vue'
import { Database, Table2, Eye, ChevronRight, ChevronDown, RefreshCw, MoreHorizontal } from '@lucide/vue'
import { useI18n } from '../i18n'
import { ElMessageBox } from 'element-plus'
import { GetDatabases, GetTablePage, CreateDatabase, DropDatabase, CreateTable, DropTable, DropView, TruncateTable, CopyTable, GetDBCapabilities, DumpTable, SaveFileDialogFiltered, WriteFileBase64, OpenFileDialogFiltered, ReadFileBase64, ExecuteSQLScript } from '../../bindings/github.com/ys-ll/uniterm/app'
import { msg } from '../services/message'
import { isWebServerMode, pickBrowserFiles, readBrowserFileAsText } from '../services/webSftpBridge'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import MenuDivider from './MenuDivider.vue'
import type { TableInfo } from '../types/database'
import { tryAndroidSave } from '../utils/androidSave'

const { t } = useI18n()

interface DbEntry {
  name: string
  // Sparse page storage: indexed by absolute row, null = not yet loaded.
  // Length equals `total` once the first page has been fetched, so virtual
  // slot positions stay stable as pages stream in (including after a jump).
  tables: (TableInfo | null)[]
  loaded: boolean
  total: number
  loading: boolean
  // Number of non-null (actually loaded) table rows — used for the footer.
  loadedCount: number
  // Table-name filter (search) that produced the current array contents.
  query: string
  // Set of page offsets already requested, to coalesce rapid scrolls.
  requested: Set<number>
}

const props = defineProps<{
  sessionId: string
  defaultDbName?: string
  dbType?: string
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
  newQuery: [dbName?: string]
  objectRemoved: [payload: { dbName: string; tableName?: string; kind: 'table' | 'view' | 'database' }]
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
    // 默认全部折叠：只加载库/schema 名称，表在点击节点展开时按需懒加载
    // （见 ensureDbExpanded）。需要改为默认展开时，把 expandedDbs 设为第一个节点
    // 并对该节点调用一次 fetchPage(0, INITIAL_PAGE) 即可。
    const fresh = (name: string): DbEntry =>
      ({ name, tables: [], loaded: false, total: 0, loading: false, loadedCount: 0, query: '', requested: new Set() })
    databases.value = dbs.map((db: string) => fresh(db))
    expandedDbs.value = new Set()
  } catch (e) {
    console.error('Failed to load tree:', e)
  } finally {
    loading.value = false
    await nextTick()
    recomputeVisible()
  }
}

watch(() => props.sessionId, (newId) => {

  if (newId) loadTree()
}, { immediate: true })

// ── Backend pagination + jump loading + virtual scrolling ──
//
// The tree pages its table lists from the backend (GetTablePage), which also
// accepts a table-name filter, and renders only the rows in the viewport
// (virtual scroll). Tables are stored sparsely by absolute index (null = not
// yet loaded), so dragging the scrollbar straight to the bottom requests that
// bottom page directly — you no longer wait for every intermediate page. A
// non-empty search term filters the WHOLE schema server-side (LIKE), not just
// what has been paged in. This keeps a ~100k-table schema smooth: bounded DOM
// nodes, fast jumps, and full-schema search.

const INITIAL_PAGE = 200
const JUMP_PAGE = 400
const ROW_H = 28
const OVERSCAN = 30

// fetchPage loads one page of a database's tables at an absolute offset and
// stores it in the sparse db.tables array. Results are discarded (via the
// db.query check) if the search term changed while the request was in flight.
async function fetchPage(db: DbEntry, offset: number, limit: number) {
  const q = searchQuery.value.trim()
  db.loading = true
  try {
    const [tables, total] = await GetTablePage(props.sessionId, db.name, q, offset, limit)
    if (q !== db.query) return // stale result from a previous search term
    db.total = total
    ensureSized(db, total)
    for (let i = 0; i < tables.length; i++) {
      db.tables[offset + i] = tables[i]
    }
    db.loadedCount = db.tables.reduce((c, t) => c + (t ? 1 : 0), 0)
    db.loaded = true
  } catch (e) {
    console.error('Failed to load table page:', e)
  } finally {
    db.loading = false
    if (q === db.query) recomputeVisible()
  }
}

// Keep the table array's length equal to the total so virtual slot positions
// stay stable while pages stream in (missing entries become null).
function ensureSized(db: DbEntry, total: number) {
  if (db.tables.length === total) return
  db.tables.length = total
  for (let i = 0; i < total; i++) {
    if (db.tables[i] === undefined) db.tables[i] = null
  }
}

// resetDb clears a database's list and loads its first page (used on refresh
// and whenever the search term changes).
async function resetDb(db: DbEntry) {
  db.tables = []
  db.total = 0
  db.loaded = false
  db.loadedCount = 0
  db.requested.clear()
  db.query = searchQuery.value.trim()
  await fetchPage(db, 0, INITIAL_PAGE)
}

// rebuildDb is an alias used by the refresh handlers.
async function rebuildDb(db: DbEntry) {
  await resetDb(db)
}

function pageStart(idx: number) {
  return Math.floor(idx / JUMP_PAGE) * JUMP_PAGE
}

// Jump-load: for a section, request the page covering the first visible,
// not-yet-loaded table row. Coalesces on a per-db set of requested offsets so
// a fast scrollbar drag triggers at most one page fetch per pass.
function scheduleJumpLoad(s: Section, startGlobal: number, endGlobal: number) {
  const db = s.db
  if (s.kind !== 'more' && s.kind !== 'full') return
  const slotStart = Math.max(0, startGlobal - s.start - 1)
  const slotEnd = Math.min(db.total - 1, endGlobal - s.start - 1)
  for (let i = slotStart; i <= slotEnd; i++) {
    const tbl = db.tables[i]
    if (tbl) continue
    const off = pageStart(i)
    if (db.requested.has(off)) continue
    db.requested.add(off)
    fetchPage(db, off, JUMP_PAGE)
    return
  }
}

// Virtualized flat layout: each expanded database contributes its header row
// plus one slot per table plus (while more remain unloaded) a "loading more"
// footer row. The slot height reserves room for the *total* table count so the
// scrollbar spans the whole list, even the not-yet-fetched rows, enabling
// immersive infinite scroll.
interface Section {
  db: DbEntry
  start: number
  n: number
  kind: 'collapsed' | 'empty' | 'more' | 'full'
}

const sections = computed<Section[]>(() => {
  const arr: Section[] = []
  for (const db of databases.value) {
    const expanded = expandedDbs.value.has(db.name)
    if (!expanded || !db.loaded) {
      arr.push({ db, start: 0, n: 1, kind: 'collapsed' })
      continue
    }
    const total = db.total
    if (total === 0) {
      arr.push({ db, start: 0, n: 2, kind: 'empty' })
      continue
    }
    const more = db.loadedCount < total
    arr.push({ db, start: 0, n: 1 + total + (more ? 1 : 0), kind: more ? 'more' : 'full' })
  }
  let acc = 0
  for (const s of arr) {
    s.start = acc
    acc += s.n
  }
  return arr
})

const totalRows = computed(() => sections.value.reduce((a, s) => a + s.n, 0))

const scrollTop = ref(0)
const viewH = ref(0)
const topPad = ref(0)
const bottomPad = ref(0)
const visibleRows = ref<any[]>([])

function describeRow(s: Section, idx: number, key: string): any {
  if (idx === 0) return { key, kind: 'db', db: s.db }
  if (s.kind === 'empty') return { key, kind: 'empty', db: s.db }
  const slot = idx - 1
  if (s.kind === 'more' && idx === s.n - 1) {
    return { key, kind: 'more', db: s.db, total: s.db.total, loaded: s.db.loadedCount }
  }
  const tbl = s.db.tables[slot]
  if (!tbl) return { key, kind: 'blank', db: s.db }
  return { key, kind: 'table', db: s.db, table: tbl }
}

function recomputeVisible() {
  const secs = sections.value
  const total = totalRows.value
  if (total === 0) {
    visibleRows.value = []
    topPad.value = 0
    bottomPad.value = 0
    return
  }
  const view = viewH.value > 0 ? viewH.value : 600
  const start = Math.max(0, Math.floor(scrollTop.value / ROW_H) - OVERSCAN)
  const end = Math.min(total, Math.ceil((scrollTop.value + view) / ROW_H) + OVERSCAN)
  topPad.value = start * ROW_H
  bottomPad.value = Math.max(0, (total - end) * ROW_H)
  const rows: any[] = []
  for (const s of secs) {
    const a = Math.max(start, s.start)
    const b = Math.min(end, s.start + s.n - 1)
    if (a > b) continue
    for (let i = a; i <= b; i++) {
      const idx = i - s.start
      rows.push(describeRow(s, idx, s.db.name + ':' + idx))
    }
  }
  visibleRows.value = rows
  for (const s of secs) {
    scheduleJumpLoad(s, start, end)
  }
}

function onTreeScroll(e: Event) {
  const el = e.target as HTMLElement
  scrollTop.value = el.scrollTop
  viewH.value = el.clientHeight
  recomputeVisible()
}

watch(() => [sections.value, expandedDbs.value], () => recomputeVisible())

// Keep viewport height in sync with the container's size.
let ro: ResizeObserver | undefined
watch(treeContentRef, (el) => {
  ro?.disconnect()
  if (el) {
    ro = new ResizeObserver(() => {
      viewH.value = el.clientHeight
      recomputeVisible()
    })
    ro.observe(el)
  }
})

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
      await resetDb(entry)
    }
    expandedDbs.value = new Set(expandedDbs.value)
  }
  await nextTick()
  scrollActiveIntoView()
})

function scrollActiveIntoView() {
  const root = treeContentRef.value
  if (!root || !selectedDb.value) return
  const sec = sections.value.find(s => s.db.name === selectedDb.value)
  if (!sec || sec.kind === 'collapsed') return
  let target: number
  if (selectedTable.value) {
    const slot = sec.db.tables.findIndex(t => t && t.name === selectedTable.value)
    if (slot < 0) return
    target = sec.start + 1 + slot
  } else {
    target = sec.start
  }
  root.scrollTop = Math.max(0, target * ROW_H - viewH.value / 2)
  scrollTop.value = root.scrollTop
  recomputeVisible()
}

async function ensureDbExpanded(dbName: string) {
  if (expandedDbs.value.has(dbName)) return
  expandedDbs.value.add(dbName)
  expandedDbs.value = new Set(expandedDbs.value)
  recomputeVisible()
  const db = databases.value.find(d => d.name === dbName)
  if (db && !db.loaded) {
    await resetDb(db)
  }
}

async function onDbActivate(dbName: string) {
  selectedDb.value = dbName
  selectedTable.value = ''
  await ensureDbExpanded(dbName)
  emit('openDatabase', dbName, 'objects')
}

async function onDbClick(dbName: string) {
  await onDbActivate(dbName)
}

async function onToggleDb(dbName: string) {
  if (expandedDbs.value.has(dbName)) {
    expandedDbs.value.delete(dbName)
    expandedDbs.value = new Set(expandedDbs.value)
    recomputeVisible()
    return
  }
  await ensureDbExpanded(dbName)
}

function onTableClick(dbName: string, tableName: string) {
  const table = databases.value.find(d => d.name === dbName)?.tables.find(t => t && t.name === tableName)
  selectedDb.value = dbName
  selectedTable.value = tableName
  emit('selectTable', dbName, tableName, table?.type === 'view')
}

function onTableDblClick(dbName: string, table: TableInfo) {
  selectedDb.value = dbName
  selectedTable.value = table.name
  emit('selectTable', dbName, table.name, table.type === 'view')
}

// Show the full table name as a native tooltip only when the name is actually
// truncated with an ellipsis (scrollWidth > clientWidth). Non-truncated names
// keep no tooltip so nothing pops up when hovering them.
function onTableNameHover(e: MouseEvent, name: string) {
  const el = e.currentTarget as HTMLElement
  if (el.scrollWidth > el.clientWidth) {
    el.title = name
  } else {
    el.removeAttribute('title')
  }
}

// Searching runs a full-schema, server-side name filter (LIKE) across every
// database, paged just like the normal list, so it finds tables no matter how
// far down the 100k-row list they live. Debounced while the user types.
let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(searchQuery, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    if (searchQuery.value.trim()) {
      expandedDbs.value = new Set(databases.value.map(d => d.name))
    }
    for (const db of databases.value) {
      resetDb(db)
    }
  }, 250)
})


// ── Context menu ──

interface DbMenuCtx {
  type: 'db' | 'table' | 'blank'
  dbName: string
  tableName: string
  tableType: string
}

const ctxMenuVisible = ref(false)
const ctxMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const ctxDbName = ref('')
const ctxTableName = ref('')
const ctxTableType = ref('')

function openDbMenu(e: MouseEvent, item: DbMenuCtx) {
  ctxMenuRef.value?.openAt(e.clientX, e.clientY, item)
}

function onDbContextMenu(e: MouseEvent, dbName: string) {
  ctxDbName.value = dbName
  ctxTableName.value = ''
  openDbMenu(e, { type: 'db', dbName, tableName: '', tableType: '' })
}

function onTableContextMenu(e: MouseEvent, dbName: string, table: TableInfo) {
  selectedDb.value = dbName
  selectedTable.value = table.name
  // 右键表节点时直接切换到该表数据页，同时弹出菜单
  emit('selectTable', dbName, table.name, table.type === 'view')
  ctxDbName.value = dbName
  ctxTableName.value = table.name
  ctxTableType.value = table.type || ''
  openDbMenu(e, { type: 'table', dbName, tableName: table.name, tableType: table.type || '' })
}

function onTreeContextMenu(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (target.closest('.db-header') || target.closest('.table-item')) return
  ctxDbName.value = ''
  ctxTableName.value = ''
  openDbMenu(e, { type: 'blank', dbName: '', tableName: '', tableType: '' })
}

function onCtxListDatabase() {
  emit('openDatabase', ctxDbName.value, 'objects')
  ctxMenuVisible.value = false
}

function onCtxNewQuery() {
  emit('newQuery', ctxDbName.value)
  ctxMenuVisible.value = false
}

function onCtxNewQueryBlank() {
  emit('newQuery')
  ctxMenuVisible.value = false
}

function onCtxViewData() {
  emit('selectTable', ctxDbName.value, ctxTableName.value, ctxTableType.value === 'view')
  ctxMenuVisible.value = false
}

function onCtxViewStructure() {
  emit('viewStructure', ctxDbName.value, ctxTableName.value)
  ctxMenuVisible.value = false
}

function onCtxCopyName() {
  navigator.clipboard.writeText(ctxTableName.value).catch(() => {})
  ctxMenuVisible.value = false
}

// ── Export table as .sql ──

async function exportTable(withStructure: boolean, withData: boolean) {
  ctxMenuVisible.value = false
  const dbName = ctxDbName.value
  const tableName = ctxTableName.value
  try {
    const sql = await DumpTable(props.sessionId, dbName, tableName, withStructure, withData)
    const b64 = encodeBase64(sql)
    // Android: save through the system "save to" picker instead of the
    // (unsupported) native save dialog.
    const androidOutcome = await tryAndroidSave(tableName + '.sql', 'text/plain', b64)
    if (androidOutcome !== 'fallback') {
      if (androidOutcome === 'saved') msg.success(t('db.exportDone', { name: tableName }))
      return
    }
    const path = await SaveFileDialogFiltered(t('db.exportSQL'), tableName + '.sql', 'SQL File', '*.sql')
    if (!path) return
    await WriteFileBase64(path, b64)
    msg.success(t('db.exportDone', { name: tableName }))
  } catch (e: any) {
    msg.error(e?.message || String(e))
  }
}

function onCtxExportStructure() {
  exportTable(true, false)
}

function onCtxExportData() {
  exportTable(false, true)
}

function onCtxExportStructureData() {
  exportTable(true, true)
}

// ── Run SQL file (database context menu) ──

// Wails v3 rejects the picker promise with "cancelled by user" when the
// dialog is dismissed, instead of resolving with an empty path.
function isDialogCancel(e: unknown): boolean {
  return String(e).toLowerCase().includes('cancel')
}

function decodeBase64(b64: string): string {
  try {
    const bin = atob(b64)
    const bytes = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
    return new TextDecoder('utf-8').decode(bytes)
  } catch {
    return ''
  }
}

const runningSqlFile = ref(false)

async function onCtxRunSqlFile() {
  ctxMenuVisible.value = false
  const dbName = ctxDbName.value
  let path = ''
  let text = ''
  try {
    // Web mode: read the script content in the browser, no Go-side file IO.
    if (await isWebServerMode()) {
      const [file] = await pickBrowserFiles({ accept: '.sql' })
      if (!file) return
      path = file.name
      text = await readBrowserFileAsText(file)
    } else {
      path = await OpenFileDialogFiltered(t('db.runSqlFile'), 'SQL File', '*.sql')
      if (!path) return
    }
  } catch (e) {
    if (!isDialogCancel(e)) msg.error((e as any)?.message || String(e))
    return
  }
  if (!path) return
  try {
    runningSqlFile.value = true
    if (!text) {
      const b64 = await ReadFileBase64(path)
      text = decodeBase64(b64)
    }
    const result = await ExecuteSQLScript(props.sessionId, dbName, text)
    if (result?.failedLine) {
      showScriptFailure(result)
    } else {
      const name = path.split(/[\\/]/).pop() || path
      msg.success(t('db.scriptExecuted', { n: result?.executed ?? 0 }) + ` · ${t('db.affectedRows')}: ${result?.affectedTotal ?? 0} · ${name}`)
    }
  } catch (e: any) {
    msg.error(e?.message || String(e))
  } finally {
    runningSqlFile.value = false
  }
}

// Failure detail dialog: line number alone doesn't tell the user what went
// wrong (e.g. a duplicate-key INSERT on a rerun). Show the driver error and
// the failing statement, matching the query editor's script error panel.
function showScriptFailure(result: { failedLine: number; failedSql?: string; error?: string }) {
  const children = [h('p', { style: 'margin:0;font-weight:600' }, t('db.scriptFailedLine', { line: result.failedLine }))]
  if (result.error) {
    children.push(h('p', { style: 'margin:6px 0 0;color:var(--el-color-error);word-break:break-word;font-family:var(--font-mono,monospace);font-size:12px' }, result.error))
  }
  if (result.failedSql) {
    children.push(h('pre', { style: 'margin:8px 0 0;padding:8px;background:var(--bg-base,#f5f5f5);border-radius: var(--radius-sm);font-family:var(--font-mono,monospace);font-size:12px;white-space:pre-wrap;word-break:break-word;max-height:180px;overflow:auto' }, result.failedSql))
  }
  ElMessageBox.alert(h('div', { style: 'display:flex;flex-direction:column' }, children), t('db.runSqlFile'), {
    confirmButtonText: t('common.confirm'),
    type: 'error',
    customStyle: { maxWidth: '35rem', width: '35rem' },
  }).catch(() => {})
}

// ── Copy table ──

const copyTableVisible = ref(false)
const copySourceTable = ref('')
const copyTableName = ref('')

function onCtxCopyTable() {
  ctxMenuVisible.value = false
  copySourceTable.value = ctxTableName.value
  copyTableName.value = ctxTableName.value + '_copy'
  copyTableVisible.value = true
}

async function onCopyTable() {
  const dbName = ctxDbName.value
  const newName = copyTableName.value.trim()
  if (!copySourceTable.value || !newName) return
  try {
    await CopyTable(props.sessionId, dbName, copySourceTable.value, newName)
    copyTableVisible.value = false
    msg.success(t('db.copyTableDone', { name: newName }))
    await refreshDb(dbName)
  } catch (e: any) {
    console.error('Failed to copy table:', e)
    msg.error(e?.message || String(e))
  }
}

function encodeBase64(text: string): string {
  const bytes = new TextEncoder().encode(text)
  let bin = ''
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i])
  return btoa(bin)
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
  ctxMenuVisible.value = false
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
  const dbName = ctxDbName.value
  showConfirm(
    t('db.dropDatabase'),
    t('db.dropDatabaseConfirm', { name: dbName }),
    dbName,
    async () => {
      await DropDatabase(props.sessionId, dbName)
      emit('objectRemoved', { dbName, kind: 'database' })
    }
  )
}

function onCtxDropTable() {
  const dbName = ctxDbName.value
  const tableName = ctxTableName.value
  showConfirm(
    t('db.dropTable'),
    t('db.dropTableConfirm', { name: tableName }),
    tableName,
    async () => {
      await DropTable(props.sessionId, dbName, tableName)
      emit('objectRemoved', { dbName, tableName, kind: 'table' })
    }
  )
}

function onCtxDropView() {
  const dbName = ctxDbName.value
  const tableName = ctxTableName.value
  showConfirm(
    t('db.dropView'),
    t('db.dropViewConfirm', { name: tableName }),
    tableName,
    async () => {
      await DropView(props.sessionId, dbName, tableName)
      emit('objectRemoved', { dbName, tableName, kind: 'view' })
    }
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
  ctxMenuVisible.value = false
  const db = databases.value.find(d => d.name === ctxDbName.value)
  if (db) {
    try {
      await rebuildDb(db)
    } catch (e) {
      console.error('Failed to refresh:', e)
    }
  }
}

async function onCtxRefreshDatabases() {
  ctxMenuVisible.value = false
  await loadTree()
}

// Refresh a single database's tables (called from parent after external mutations)
async function refreshDb(dbName: string) {
  const db = databases.value.find(d => d.name === dbName)
  if (db) {
    try {
      await rebuildDb(db)
    } catch (e) {
      console.error('Failed to refresh db:', e)
    }
  }
}

defineExpose({ refreshDb })

// ── Toolbar (search box) refresh + more menu ──

const moreMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const moreMenuVisible = ref(false)

function refreshAll() {
  loadTree()
}

function onMoreNewDatabase() {
  moreMenuVisible.value = false
  newDbName.value = ''
  newDbVisible.value = true
}

function onMoreNewTable() {
  moreMenuVisible.value = false
  newTableDb.value = selectedDb.value || databases.value[0]?.name || ''
  newTableName.value = ''
  newTableVisible.value = true
}

function onMoreRefresh() {
  moreMenuVisible.value = false
  loadTree()
}

// ── New Database / Table dialogs ──

const newDbVisible = ref(false)
const newDbName = ref('')

function onCtxNewDatabase() {
  ctxMenuVisible.value = false
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
const newTableDb = ref('')

function onCtxNewTable() {
  ctxMenuVisible.value = false
  newTableDb.value = ctxDbName.value
  newTableName.value = ''
  newTableVisible.value = true
}

async function onCreateTable() {
  if (!newTableName.value.trim() || !newTableDb.value) return
  try {
    await CreateTable(props.sessionId, newTableDb.value, newTableName.value.trim())
    newTableVisible.value = false
    const db = databases.value.find(d => d.name === newTableDb.value)
    if (db) {
      await rebuildDb(db)
      expandedDbs.value = new Set([...expandedDbs.value, newTableDb.value])
    }
  } catch (e: any) {
    console.error('Failed to create table:', e)
    msg.error(e?.message || String(e))
  }
}
</script>

<style scoped>
.db-tree-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.panel-header {
  padding: 0.5rem 0.75rem 0.25rem;
  font-family: var(--font-ui);
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  flex-shrink: 0;
}
.search-wrap {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem 0.5rem;
  flex-shrink: 0;
}
.search-input {
  flex: 1;
  min-width: 0;
  padding: 0.25rem 0.5rem;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-base);
  color: var(--text-primary);
  font-family: var(--font-ui);
  font-size: 0.75rem;
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
  padding: 0.75rem;
  color: var(--text-secondary);
  font-family: var(--font-ui);
  font-size: 0.75rem;
  text-align: center;
}
.db-header {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.375rem 0.5rem;
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
  width: 0.75rem;
  flex-shrink: 0;
  color: var(--text-muted);
  display: flex;
  align-items: center;
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
  font-size: 0.8125rem;
  font-weight: 600;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.table-item {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.375rem 0.5rem;
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
  width: 1.875rem;
  flex-shrink: 0;
}
.table-icon {
  flex-shrink: 0;
  color: var(--text-muted);
}
.table-name {
  font-family: var(--font-ui);
  font-size: 0.8125rem;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.empty-hint {
  padding: 0.25rem 0.5rem 0.25rem 1.75rem;
  font-family: var(--font-ui);
  font-size: 0.75rem;
  color: var(--text-muted);
}
.tree-more {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem 0.5rem 0.25rem 1.75rem;
  font-family: var(--font-ui);
  font-size: 0.75rem;
  color: var(--text-muted);
  flex-wrap: wrap;
}
.tree-more-info {
  margin-right: 0.25rem;
}
.tree-more-btn {
  font-family: var(--font-ui);
  font-size: 0.75rem;
  color: var(--accent, #409eff);
  background: transparent;
  border: 1px solid var(--border, #e0e0e0);
  border-radius: 4px;
  padding: 0.125rem 0.375rem;
  cursor: pointer;
}
.tree-more-btn:hover {
  background: var(--bg-hover);
}

/* Virtual-scroll spacers. They reserve the scrollbar length for rows that are
   not currently mounted, so the tree can appear to be one long list. */
.virt-pad {
  width: 100%;
  flex-shrink: 0;
}
.table-item-blank {
  pointer-events: none;
}

/* Confirm dialog */
.confirm-body {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.confirm-text {
  font-family: var(--font-ui);
  font-size: 0.875rem;
  color: var(--text-primary);
  margin: 0;
}
.confirm-hint {
  font-family: var(--font-ui);
  font-size: 0.75rem;
  color: var(--text-muted);
  margin: 0;
}
</style>
