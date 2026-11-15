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
          v-else-if="row.kind === 'schema'"
          class="db-header schema-header"
          :data-schema-row="row.unitName"
          :class="{ selected: selectedDb === row.unitName && !selectedTable }"
          @click="onSchemaClick(row.unitName)"
          @contextmenu.prevent="onSchemaContextMenu($event, row.unitName)"
        >
          <span class="db-arrow" @click.stop="onToggleSchema(row.unitName)">
            <component :is="expandedSchemas.has(row.unitName) ? ChevronDown : ChevronRight" :size="'0.75rem'" />
          </span>
          <FolderOpen class="table-icon" :size="'0.875rem'" />
          <span class="db-name">{{ row.schema }}</span>
        </div>
        <div
          v-else-if="row.kind === 'table' && row.table"
          class="table-item"
          :data-table-row="row.dbName + '/' + row.table.name"
          :class="{ selected: selectedTable === row.table.name && selectedDb === row.dbName, nested: !!row.schema }"
          @click="onTableClick(row.dbName, row.table.name)"
          @dblclick="onTableDblClick(row.dbName, row.table)"
          @contextmenu.prevent="onTableContextMenu($event, row.dbName, row.table)"
        >
          <span class="table-icon-spacer" />
          <component :is="row.table.type === 'view' ? Eye : Table" class="table-icon" :size="'0.875rem'" />
          <span class="table-name" @mouseenter="onTableNameHover($event, row.table.name)">{{ shortTable(row.table.name) }}</span>
        </div>
        <div v-else-if="row.kind === 'empty'" class="empty-hint" :class="{ nested: !!row.schema }">
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
        <MenuItem v-if="!isSchemaNode && schemaCapable" @click="onCtxNewSchema">{{ t('db.newSchema') }}</MenuItem>
        <MenuItem v-if="!isSchemaNode && canCreateDatabase" @click="onCtxNewDatabase">{{ t('db.newDatabase') }}</MenuItem>
        <MenuItem @click="onCtxNewTable">{{ t('db.newTable') }}</MenuItem>
        <MenuDivider />
        <MenuSubmenu v-if="!isSchemaNode || !isDboNode" class="danger" :label="t('db.dangerZone')">
          <MenuItem v-if="isSchemaNode" class="danger" @click="onCtxDropSchema">{{ t('db.dropSchema') }}</MenuItem>
          <MenuItem v-if="!isSchemaNode" class="danger" @click="onCtxDropDatabase">{{ t('db.dropDatabase') }}</MenuItem>
        </MenuSubmenu>
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
        <MenuSubmenu :label="t('db.export')">
          <MenuItem @click="onCtxExportStructure">{{ t('db.exportStructure') }}</MenuItem>
          <MenuItem v-if="current.tableType !== 'view'" @click="onCtxExportData">{{ t('db.exportData') }}</MenuItem>
          <MenuItem v-if="current.tableType !== 'view'" @click="onCtxExportStructureData">{{ t('db.exportStructureData') }}</MenuItem>
        </MenuSubmenu>
        <MenuDivider />
        <MenuSubmenu class="danger" :label="t('db.dangerZone')">
          <template v-if="current.tableType === 'view'">
            <MenuItem class="danger" @click="onCtxDropView">{{ t('db.dropView') }}</MenuItem>
          </template>
          <template v-else>
            <MenuItem class="danger" @click="onCtxTruncateTable">{{ t('db.truncateTable') }}</MenuItem>
            <MenuItem class="danger" @click="onCtxDropTable">{{ t('db.dropTable') }}</MenuItem>
          </template>
        </MenuSubmenu>
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
        <p class="confirm-hint copyable-text">{{ t('db.typeToConfirm', { name: confirmName }) }}</p>
        <el-input v-model="confirmInput" :placeholder="confirmName" />
      </div>
      <template #footer>
        <el-button @click="confirmVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" :disabled="confirmInput !== confirmName || confirmBusy" @click="onConfirm">
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
        <el-form-item v-if="schemaCapable" :label="t('db.schemaName')">
          <el-input v-model="newDbSchema" :placeholder="defaultSchema" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="newDbVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="!newDbName.trim()" @click="onCreateDatabase">
          {{ t('common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- New Schema dialog (two-level tree) -->
    <el-dialog append-to-body v-model="newSchemaVisible" :title="t('db.newSchema')" width="23.75rem">
      <el-form label-width="5rem">
        <el-form-item :label="t('db.schemaName')">
          <el-input v-model="newSchemaName" :placeholder="defaultSchema" />
        </el-form-item>
        <el-form-item :label="t('db.databases')">
          <el-select v-model="newSchemaTargetDb" style="width:100%">
            <el-option v-for="t in newSchemaTargets" :key="t" :label="t" :value="t" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="newSchemaVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :disabled="!newSchemaName.trim()" @click="onCreateSchema">
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
            <el-option v-for="t in newTableTargets" :key="t" :label="t" :value="t" />
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
import { Database, Table, Eye, FolderOpen, ChevronRight, ChevronDown, RefreshCw, MoreHorizontal } from '@lucide/vue'
import { useI18n } from '../i18n'
import { ElMessageBox } from 'element-plus'
import { GetDatabases, GetSchemas, GetTablePage, CreateDatabase, DropDatabase, DropSchema, CreateTable, DropTable, DropView, TruncateTable, CopyTable, GetDBCapabilities, DumpTable, SaveFileDialogFiltered, WriteFileBase64, OpenFileDialogFiltered, ReadFileBase64, ExecuteSQLScript } from '../../bindings/github.com/ys-ll/uniterm/app'
import { msg } from '../services/message'
import { isWebServerMode, pickBrowserFiles, readBrowserFileAsText, saveBase64InWebServerMode } from '../services/webSftpBridge'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import MenuDivider from './MenuDivider.vue'
import MenuSubmenu from './MenuSubmenu.vue'
import type { TableInfo } from '../types/database'
import { tryAndroidSave } from '../utils/androidSave'

const { t } = useI18n()

// Two-level tree: a dialect with inner schemas (SQL Server,
// PostgreSQL) renders db → schema → tables. Both the db itself (schema-less
// dialects, degraded databases) and every schema node own a paged table
// list, so the pagination machinery works on this shared shape.
interface PagedList {
  // The name handed to GetTablePage / CRUD: bare db name or "db.schema".
  name: string
  tables: (TableInfo | null)[]
  loaded: boolean
  total: number
  loading: boolean
  loadedCount: number
  query: string
  requested: Set<number>
}

interface SchemaUnit extends PagedList {
  schema: string // display label inside its db
}

interface DbEntry extends PagedList {
  name: string
  // null = not fetched yet; [] = schema-less/degraded (tables render
  // directly under the db node).
  schemas: string[] | null
  schemasLoading: boolean
  schemaUnits: SchemaUnit[] | null
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
// Expansion state of the second tree level ("db.schema" keys).
const expandedSchemas = ref(new Set<string>())
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
      ({ name, tables: [], loaded: false, total: 0, loading: false, loadedCount: 0, query: '', requested: new Set(), schemas: null, schemasLoading: false, schemaUnits: null })
    databases.value = dbs.map((db: string) => fresh(db))
    expandedDbs.value = new Set()
    expandedSchemas.value = new Set()
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

// fetchPage loads one page of a list unit's tables (the db itself when
// schema-less, otherwise a "db.schema" schema unit) at an absolute offset and
// stores it in the sparse tables array. Results are discarded (via the unit's
// query check) if the search term changed while the request was in flight.
async function fetchPage(unit: PagedList, offset: number, limit: number) {
  const q = searchQuery.value.trim()
  unit.loading = true
  try {
    const [tables, total] = await GetTablePage(props.sessionId, unit.name, q, offset, limit)
    if (q !== unit.query) return // stale result from a previous search term
    unit.total = total
    ensureSized(unit, total)
    for (let i = 0; i < tables.length; i++) {
      unit.tables[offset + i] = tables[i]
    }
    unit.loadedCount = unit.tables.reduce((c, t) => c + (t ? 1 : 0), 0)
    unit.loaded = true
  } catch (e) {
    console.error('Failed to load table page:', e)
  } finally {
    unit.loading = false
    if (q === unit.query) recomputeVisible()
  }
}

// Keep the table array's length equal to the total so virtual slot positions
// stay stable while pages stream in (missing entries become null).
function ensureSized(unit: PagedList, total: number) {
  if (unit.tables.length === total) return
  unit.tables.length = total
  for (let i = 0; i < total; i++) {
    if (unit.tables[i] === undefined) unit.tables[i] = null
  }
}

// resetList clears a list unit and loads its first page (used on refresh and
// whenever the search term changes).
async function resetList(unit: PagedList) {
  unit.tables = []
  unit.total = 0
  unit.loaded = false
  unit.loadedCount = 0
  unit.requested.clear()
  unit.query = searchQuery.value.trim()
  await fetchPage(unit, 0, INITIAL_PAGE)
}

// resetDb resets the db's own list (schema-less mode) and every loaded schema
// unit; only currently expanded units fetch a first page. An empty
// schemaUnits array means schema-less too — the db's own list is the target.
async function resetDb(db: DbEntry) {
  db.query = searchQuery.value.trim()
  const jobs: Promise<void>[] = []
  if (!db.schemaUnits || db.schemaUnits.length === 0) {
    jobs.push(resetList(db))
  } else {
    for (const su of db.schemaUnits) {
      if (expandedSchemas.value.has(su.name)) {
        jobs.push(resetList(su))
      } else {
        su.tables = []
        su.total = 0
        su.loaded = false
        su.loadedCount = 0
        su.requested.clear()
        su.query = searchQuery.value.trim()
      }
    }
  }
  await Promise.all(jobs)
}

// rebuildDb is an alias used by the refresh handlers.
async function rebuildDb(db: DbEntry) {
  await resetDb(db)
}

// loadSchemas fetches a database's schemas once and builds the second-level
// list units. Empty/error → schema-less (or degraded) single-level tree.
async function loadSchemas(db: DbEntry) {
  if (db.schemas !== null || db.schemasLoading) return
  db.schemasLoading = true
  try {
    const schemas = await GetSchemas(props.sessionId, db.name)
    db.schemas = schemas || []
    db.schemaUnits = (db.schemas || []).map(s => {
      const key = db.name + '.' + s
      return { name: key, schema: s, tables: [], loaded: false, total: 0, loading: false, loadedCount: 0, query: searchQuery.value.trim(), requested: new Set() }
    })
  } catch (e) {
    console.error('Failed to load schemas:', e)
    db.schemas = []
    db.schemaUnits = []
  } finally {
    db.schemasLoading = false
    recomputeVisible()
  }
}

function pageStart(idx: number) {
  return Math.floor(idx / JUMP_PAGE) * JUMP_PAGE
}

// Jump-load: for a list block, request the page covering the first visible,
// not-yet-loaded table row. Coalesces on a per-unit set of requested offsets so
// a fast scrollbar drag triggers at most one page fetch per pass.
function scheduleJumpLoad(b: Block, startGlobal: number, endGlobal: number) {
  if (b.type !== 'list') return
  if (b.kind !== 'more' && b.kind !== 'full') return
  const unit = b.unit
  const slotStart = Math.max(0, startGlobal - b.rowIdx)
  const slotEnd = Math.min(unit.total - 1, endGlobal - b.rowIdx)
  for (let i = slotStart; i <= slotEnd; i++) {
    const tbl = unit.tables[i]
    if (tbl) continue
    const off = pageStart(i)
    if (unit.requested.has(off)) continue
    unit.requested.add(off)
    fetchPage(unit, off, JUMP_PAGE)
    return
  }
}

// Virtualized flat layout . Blocks model the
// visible layout as contiguous row runs; rows reserve room for the *total*
// table counts so the scrollbar spans the whole list, even the not-yet-fetched
// rows, enabling immersive infinite scroll.
type Block =
  | { type: 'dbrow'; db: DbEntry; rowIdx: number }
  | { type: 'schemarow'; db: DbEntry; unit: SchemaUnit; rowIdx: number }
  | { type: 'list'; unit: PagedList; db: DbEntry; schema: string; rowIdx: number; n: number; kind: 'empty' | 'more' | 'full' }

const blocks = computed<Block[]>(() => {
  const arr: Block[] = []
  for (const db of databases.value) {
    // The db header row always renders, collapsed or not.
    arr.push({ type: 'dbrow', db, rowIdx: 0 })
    const expanded = expandedDbs.value.has(db.name)
    if (!expanded) continue
    if (db.schemaUnits !== null && db.schemaUnits.length > 0) {
      // Two-level: schema header rows, each owning its own paged table list.
      for (const su of db.schemaUnits) {
        arr.push({ type: 'schemarow', db, unit: su, rowIdx: 0 })
        if (!expandedSchemas.value.has(su.name) || !su.loaded) continue
        const total = su.total
        if (total === 0) {
          arr.push({ type: 'list', unit: su, db, schema: su.schema, rowIdx: 0, n: 1, kind: 'empty' })
          continue
        }
        const more = su.loadedCount < total
        arr.push({ type: 'list', unit: su, db, schema: su.schema, rowIdx: 0, n: total + (more ? 1 : 0), kind: more ? 'more' : 'full' })
      }
    } else {
      // Schema-less (or degraded permission fallback): tables under the db.
      if (!db.loaded) continue
      const total = db.total
      if (total === 0) {
        arr.push({ type: 'list', unit: db, db, schema: '', rowIdx: 0, n: 1, kind: 'empty' })
        continue
      }
      const more = db.loadedCount < total
      arr.push({ type: 'list', unit: db, db, schema: '', rowIdx: 0, n: total + (more ? 1 : 0), kind: more ? 'more' : 'full' })
    }
  }
  let acc = 0
  for (const b of arr) {
    const n = b.type === 'dbrow' || b.type === 'schemarow' ? 1 : b.n
    b.rowIdx = acc
    acc += n
  }
  return arr
})

const totalRows = computed(() => blocks.value.reduce((a, b) => a + (b.type === 'dbrow' || b.type === 'schemarow' ? 1 : b.n), 0))

const scrollTop = ref(0)
const viewH = ref(0)
const topPad = ref(0)
const bottomPad = ref(0)
const visibleRows = ref<any[]>([])

function recomputeVisible() {
  const blks = blocks.value
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
  for (const b of blks) {
    const bn = b.type === 'dbrow' || b.type === 'schemarow' ? 1 : b.n
    const a = Math.max(start, b.rowIdx)
    const be = Math.min(end, b.rowIdx + bn - 1)
    if (a > be) continue
    if (b.type === 'dbrow') {
      rows.push({ key: 'db:' + b.db.name, kind: 'db', db: b.db })
      continue
    }
    if (b.type === 'schemarow') {
      rows.push({ key: 'sch:' + b.unit.name, kind: 'schema', db: b.db, unitName: b.unit.name, schema: b.unit.schema })
      continue
    }
    for (let i = a; i <= be; i++) {
      const slot = i - b.rowIdx
      const key = b.unit.name + ':' + slot
      if (b.kind === 'empty') {
        rows.push({ key, kind: 'empty', db: b.db, schema: b.schema })
        continue
      }
      if (b.kind === 'more' && slot === b.n - 1) {
        rows.push({ key, kind: 'more', db: b.db, schema: b.schema, total: b.unit.total, loaded: b.unit.loadedCount })
        continue
      }
      const tbl = b.unit.tables[slot]
      if (!tbl) {
        rows.push({ key, kind: 'blank', db: b.db, schema: b.schema })
        continue
      }
      rows.push({ key, kind: 'table', db: b.db, dbName: b.unit.name, schema: b.schema, table: tbl })
    }
  }
  visibleRows.value = rows
  for (const b of blks) {
    scheduleJumpLoad(b, start, end)
  }
}

function onTreeScroll(e: Event) {
  const el = e.target as HTMLElement
  scrollTop.value = el.scrollTop
  viewH.value = el.clientHeight
  recomputeVisible()
}

watch(() => [blocks.value, expandedDbs.value, expandedSchemas.value], () => recomputeVisible())

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
// (breadcrumb navigation, object list, etc.). Composite activeDb names
// ("db.schema") expand the parent db and the schema second level.
watch(() => [props.activeDb, props.activeTable], async ([db, table]) => {
  if (!db) return
  selectedDb.value = db
  selectedTable.value = table || ''
  const i = db.indexOf('.')
  const composite = i >= 0 && schemaCapable.value
  if (composite) {
    const parentDb = db.slice(0, i)
    const entry = findDbEntry(db)
    if (!expandedDbs.value.has(parentDb)) {
      await ensureDbExpanded(parentDb)
    } else if (entry && entry.schemas === null) {
      await loadSchemas(entry)
    }
    if (table) await ensureSchemaExpanded(db)
    await nextTick()
    scrollActiveIntoView()
    return
  }
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
  const b = blocks.value.find(x => x.type !== 'list' && (x.type === 'dbrow' ? x.db.name === selectedDb.value : x.unit.name === selectedDb.value))
    || blocks.value.find(x => x.type === 'list' && (x.db.name === selectedDb.value || x.unit.name === selectedDb.value))
  if (!b) return
  const rowIdx = b.type === 'list' ? b.rowIdx : b.rowIdx
  root.scrollTop = Math.max(0, rowIdx * ROW_H - viewH.value / 2)
  scrollTop.value = root.scrollTop
  recomputeVisible()
}

// findDbEntry / findSchemaUnit resolve tree entries for a node name; bare
// names are databases, "db.schema" composites are schema units.
function findDbEntry(name: string): DbEntry | undefined {
  return databases.value.find(d => d.name === name)
    ?? databases.value.find(d => name.startsWith(d.name + '.'))
}

function findSchemaUnit(unitName: string): SchemaUnit | undefined {
  for (const db of databases.value) {
    const su = db.schemaUnits?.find(s => s.name === unitName)
    if (su) return su
  }
  return undefined
}

async function ensureDbExpanded(dbName: string) {
  if (expandedDbs.value.has(dbName)) return
  expandedDbs.value.add(dbName)
  expandedDbs.value = new Set(expandedDbs.value)
  recomputeVisible()
  const db = findDbEntry(dbName)
  if (!db) return
  if (db.schemas === null) {
    await loadSchemas(db)
  }
  if (db.schemaUnits !== null && db.schemaUnits.length > 0) {
    return // second level owns the table lists now
  }
  if (!db.loaded) {
    await resetDb(db)
  }
}

// ── Second-level (schema) expansion ──

async function ensureSchemaExpanded(unitName: string) {
  if (expandedSchemas.value.has(unitName)) return
  expandedSchemas.value.add(unitName)
  expandedSchemas.value = new Set(expandedSchemas.value)
  recomputeVisible()
  const su = findSchemaUnit(unitName)
  if (su && !su.loaded) {
    await resetList(su)
  }
}

async function onSchemaClick(unitName: string) {
  selectedDb.value = unitName
  selectedTable.value = ''
  await ensureSchemaExpanded(unitName)
  // Consistent with db nodes: clicking opens the object-list tab, which a
  // composite name scopes to just this schema.
  emit('openDatabase', unitName, 'objects')
}

async function onToggleSchema(unitName: string) {
  if (expandedSchemas.value.has(unitName)) {
    expandedSchemas.value.delete(unitName)
    expandedSchemas.value = new Set(expandedSchemas.value)
    recomputeVisible()
    return
  }
  await ensureSchemaExpanded(unitName)
}

function onSchemaContextMenu(e: MouseEvent, unitName: string) {
  ctxDbName.value = unitName
  ctxTableName.value = ''
  openDbMenu(e, { type: 'db', dbName: unitName, tableName: '', tableType: '' })
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
  const table = findSchemaUnit(dbName)?.tables.find(t => t && t.name === tableName)
    ?? findDbEntry(dbName)?.tables.find(t => t && t.name === tableName)
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

// Table names arrive schema-qualified ("sales.orders") so the
// backend can resolve non-default schemas; display strips the prefix inside
// a schema node, while the qualified name stays in data-* attrs, tooltips
// and copy-name.
function shortTable(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1) : name
}

// Searching runs a full-schema, server-side name filter (LIKE) across every
// database, paged just like the normal list, so it finds tables no matter how
// far down the 100k-row list they live. Debounced while the user types.
// Two-level dbs expand all their schemas so the search reaches them too.
let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(searchQuery, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(async () => {
    if (searchQuery.value.trim()) {
      expandedDbs.value = new Set(databases.value.map(d => d.name))
      for (const db of databases.value) {
        if (db.schemas === null) await loadSchemas(db)
        if (db.schemaUnits && db.schemaUnits.length > 0) {
          expandedSchemas.value = new Set([...expandedSchemas.value, ...db.schemaUnits.map(s => s.name)])
        }
      }
      expandedSchemas.value = new Set(expandedSchemas.value)
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
// Schema capability comes from backend capabilities, never from
// dbType checks in the frontend. supportsSchemas gates the composite-node UI
// (dialog fields, schema menus); undroppableSchemas hides the schema-delete
// item for schemas the server refuses to drop (e.g. SQL Server's dbo).
const schemaCapable = computed(() => !!caps.value?.['supportsSchemas'])
const undroppableSchemas = computed(() => {
  const v = caps.value?.['undroppableSchemas']
  return Array.isArray(v) ? (v as string[]).map(s => String(s).toLowerCase()) : []
})
const defaultSchema = computed(() => (caps.value?.['defaultSchema'] as string) || '')
// A schema node offers 删除 Schema (unless the schema is undroppable), and
// its 删除数据库 confirmation must be answered with the real database name,
// not the node name.
const isSchemaNode = computed(() => schemaCapable.value && ctxDbName.value.includes('.'))
const isDboNode = computed(() => {
  const i = ctxDbName.value.lastIndexOf('.')
  if (i < 0) return false
  return undroppableSchemas.value.includes(ctxDbName.value.slice(i + 1).toLowerCase())
})
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
    // Web-server mode: no native save dialog — hand the payload to the
    // browser's download manager instead.
    if (await saveBase64InWebServerMode(tableName + '.sql', 'text/plain', b64)) return
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

const confirmBusy = ref(false)

async function onConfirm() {
  // Guard against double-firing (double-click on 确认 ran destructive
  // actions twice — the second attempt then failed with "does not exist").
  if (!confirmAction || confirmBusy.value) return
  confirmBusy.value = true
  try {
    await confirmAction()
    await loadTree()
  } catch (e: any) {
    console.error(e)
    msg.error(e?.message || String(e))
  } finally {
    confirmBusy.value = false
    confirmVisible.value = false
  }
}

function onCtxDropDatabase() {
  const nodeName = ctxDbName.value
  const i = nodeName.lastIndexOf('.')
  // On a composite schema node the node name is NOT what gets dropped — the
  // whole database is. The confirmation must be answered with the real
  // database name so intent and effect stay aligned.
  if (schemaCapable.value && i >= 0) {
    const db = nodeName.slice(0, i)
    showConfirm(
      t('db.dropDatabase'),
      t('db.dropDatabaseWhole', { name: nodeName, db }),
      db,
      async () => {
        await DropDatabase(props.sessionId, nodeName)
        emit('objectRemoved', { dbName: nodeName, kind: 'database' })
      }
    )
    return
  }
  showConfirm(
    t('db.dropDatabase'),
    t('db.dropDatabaseConfirm', { name: nodeName }),
    nodeName,
    async () => {
      await DropDatabase(props.sessionId, nodeName)
      emit('objectRemoved', { dbName: nodeName, kind: 'database' })
    }
  )
}

function onCtxDropSchema() {
  const nodeName = ctxDbName.value
  const i = nodeName.lastIndexOf('.')
  const schema = i >= 0 ? nodeName.slice(i + 1) : nodeName
  showConfirm(
    t('db.dropSchema'),
    t('db.dropSchemaConfirm', { name: schema }),
    schema,
    async () => {
      await DropSchema(props.sessionId, nodeName)
      emit('objectRemoved', { dbName: nodeName, kind: 'database' })
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

// refreshUnit re-reads a node's listing: a schema unit refreshes its table
// page, a two-level db re-fetches its schema list (preserving expansion), and
// a schema-less db rebuilds its flat list.
async function refreshUnit(unitName: string) {
  const su = findSchemaUnit(unitName)
  if (su) {
    await resetList(su)
    return
  }
  const db = findDbEntry(unitName)
  if (!db) return
  if (db.schemaUnits !== null && db.schemaUnits.length > 0) {
    const prevExpanded = new Set(db.schemaUnits.filter(u => expandedSchemas.value.has(u.name)).map(u => u.schema))
    db.schemas = null
    db.schemaUnits = null
    expandedSchemas.value = new Set([...expandedSchemas.value].filter(k => !k.startsWith(db.name + '.')))
    await loadSchemas(db)
    for (const u of (db.schemaUnits || []) as SchemaUnit[]) {
      if (prevExpanded.has(u.schema)) {
        expandedSchemas.value = new Set([...expandedSchemas.value, u.name])
        await resetList(u)
      }
    }
    recomputeVisible()
    return
  }
  await rebuildDb(db)
}

async function onCtxRefresh() {
  ctxMenuVisible.value = false
  try {
    await refreshUnit(ctxDbName.value)
  } catch (e) {
    console.error('Failed to refresh:', e)
  }
}

async function onCtxRefreshDatabases() {
  ctxMenuVisible.value = false
  await loadTree()
}

// Refresh a single database's tables (called from parent after external mutations)
async function refreshDb(dbName: string) {
  try {
    await refreshUnit(dbName)
  } catch (e) {
    console.error('Failed to refresh db:', e)
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
  newDbSchema.value = ''
  newDbVisible.value = true
}

function onMoreNewTable() {
  moreMenuVisible.value = false
  newTableDb.value = selectedDb.value || databases.value[0]?.name || ''
  newTableName.value = ''
  newTableVisible.value = true
  preselectNewTableTarget()
}

function onMoreRefresh() {
  moreMenuVisible.value = false
  loadTree()
}

// ── New Database / Table dialogs ──

const newDbVisible = ref(false)
const newDbName = ref('')
// Optional schema created together with the database. Only
// dialects with an inner schema concept show the field; empty means the
// login's default schema (usually dbo/public). The backend receives the
// composite "db.schema" encoding and splits it.
const newDbSchema = ref('')

function onCtxNewDatabase() {
  ctxMenuVisible.value = false
  newDbName.value = ''
  newDbSchema.value = ''
  newDbVisible.value = true
}

async function onCreateDatabase() {
  const name = newDbName.value.trim()
  if (!name) return
  const schema = newDbSchema.value.trim()
  if (schemaCapable.value) {
    // Only the schema field may not contain dots: the wire format is always
    // "name.schema" with a trailing dot when empty, so the last segment is
    // unambiguously the schema and dot-named databases ("my.db") stay legal.
    if (schema.includes('.')) {
      msg.error(t('db.nameNoDot'))
      return
    }
  }
  try {
    await CreateDatabase(props.sessionId, schemaCapable.value ? `${name}.${schema}` : name)
    newDbVisible.value = false
    await loadTree()
  } catch (e: any) {
    console.error('Failed to create database:', e)
    msg.error(e?.message || String(e))
  }
}

// ── New Schema dialog (schema-node menu) ──

const newSchemaVisible = ref(false)
const newSchemaName = ref('')
const newSchemaTargetDb = ref('')

// Target databases for the new-schema dialog: every database can host
// another schema.
const newSchemaTargets = computed(() => databases.value.map(d => d.name))

function onCtxNewSchema() {
  ctxMenuVisible.value = false
  // The target database is the composite node's db part.
  const nodeName = ctxDbName.value
  const i = nodeName.lastIndexOf('.')
  newSchemaTargetDb.value = i >= 0 ? nodeName.slice(0, i) : nodeName
  newSchemaName.value = ''
  newSchemaVisible.value = true
}

async function onCreateSchema() {
  const schema = newSchemaName.value.trim()
  if (!schema || !newSchemaTargetDb.value) return
  if (schema.includes('.')) {
    msg.error(t('db.nameNoDot'))
    return
  }
  try {
    // The tolerant CreateDatabase path: database exists → only the missing
    // schema is created; both exist → duplicate error surfaces as-is.
    await CreateDatabase(props.sessionId, `${newSchemaTargetDb.value}.${schema}`)
    newSchemaVisible.value = false
    await refreshUnit(newSchemaTargetDb.value)
    // Expand the freshly created schema so it is immediately visible.
    const key = `${newSchemaTargetDb.value}.${schema}`
    const su = findSchemaUnit(key)
    if (su) {
      expandedSchemas.value = new Set([...expandedSchemas.value, key])
      await resetList(su)
    }
  } catch (e: any) {
    console.error('Failed to create schema:', e)
    msg.error(e?.message || String(e))
  }
}

const newTableVisible = ref(false)
const newTableName = ref('')
const newTableDb = ref('')

// Targets for the new-table dialog. Schema-capable dialects (SQL Server,
// PostgreSQL) ONLY offer schemas — every table must live in one; schema-less
// dialects offer the bare database names.
const newTableTargets = computed(() => {
  const opts: string[] = []
  for (const db of databases.value) {
    if (schemaCapable.value) {
      if (db.schemaUnits && db.schemaUnits.length > 0) {
        for (const su of db.schemaUnits) {
          opts.push(su.name)
        }
      }
    } else {
      opts.push(db.name)
    }
  }
  return opts
})

// Schemas load lazily on tree expand; make sure they exist before the
// new-table dialog renders its target select.
async function ensureAllSchemasLoaded() {
  if (!schemaCapable.value) return
  for (const db of databases.value) {
    if (db.schemas === null) {
      await loadSchemas(db)
    }
  }
}

// After schemas load, a bare-db preselection must snap to one of its schemas
// (schema-capable dialects only offer schemas as targets).
function preselectNewTableTarget() {
  ensureAllSchemasLoaded().then(() => {
    if (!schemaCapable.value) return
    if (newTableTargets.value.includes(newTableDb.value)) return
    const db = findDbEntry(newTableDb.value)
    if (db?.schemaUnits && db.schemaUnits.length > 0) {
      newTableDb.value = db.schemaUnits[0].name
      return
    }
    if (newTableTargets.value.length > 0) {
      newTableDb.value = newTableTargets.value[0]
    }
  })
}

function onCtxNewTable() {
  ctxMenuVisible.value = false
  newTableDb.value = ctxDbName.value
  newTableName.value = ''
  newTableVisible.value = true
  preselectNewTableTarget()
}

async function onCreateTable() {
  if (!newTableName.value.trim() || !newTableDb.value) return
  try {
    await CreateTable(props.sessionId, newTableDb.value, newTableName.value.trim())
    newTableVisible.value = false
    const su = findSchemaUnit(newTableDb.value)
    if (su) {
      expandedSchemas.value = new Set([...expandedSchemas.value, newTableDb.value])
      await resetList(su)
      return
    }
    const db = findDbEntry(newTableDb.value)
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
/* Second-level rows indent under their parent: schemas sit
   one step in from the database, and tables inside a schema one step deeper
   than schema-less table rows. */
.schema-header {
  padding-left: 1.25rem;
}
.table-item.nested {
  padding-left: 1.25rem;
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
/* An empty hint inside a schema aligns with that schema's nested tables. */
.empty-hint.nested {
  padding-left: 3rem;
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
.confirm-text,
.confirm-hint {
  /* global tree styles set user-select: none; the confirm hint must stay
     selectable so the name can be selected and copied manually */
  user-select: text;
}
.confirm-hint {
  font-family: var(--font-ui);
  font-size: 0.75rem;
  color: var(--text-muted);
  margin: 0;
}
</style>
