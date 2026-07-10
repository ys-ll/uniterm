<template>
  <div class="mongodb-tab-content" @click="closeContextMenu">
    <div class="mongo-main">
      <!-- Left tree panel -->
      <div class="mongo-left" :style="{ width: leftWidth + 'px' }">
        <div class="search-wrap">
          <input
            v-model="treeSearchQuery"
            class="search-input"
            :placeholder="t('db.searchTables')"
          />
        </div>
        <div class="tree-content" @contextmenu.prevent="onTreeContextMenu">
          <div v-if="treeLoading" class="tree-loading">{{ t('db.loading') }}</div>
          <template v-else>
            <div v-for="db in filteredDatabases" :key="db">
              <div
                class="db-header"
                :class="{ selected: activeDb === db && !activeCollection }"
                @click="toggleDb(db)"
                @contextmenu.prevent="onDbContextMenu($event, db)"
              >
                <span class="db-arrow" @click.stop="toggleDb(db)">
                  <component :is="expandedDbs.has(db) ? ChevronDown : ChevronRight" :size="12" />
                </span>
                <Database :size="14" class="db-icon" />
                <span class="db-name">{{ db }}</span>
              </div>
              <div v-if="expandedDbs.has(db)" class="child-list">
                <div
                  v-for="col in (collections[db] || [])"
                  :key="col"
                  class="table-item"
                  :class="{ selected: activeCollection === col && activeDb === db }"
                  @dblclick="selectCollection(db, col)"
                  @click="selectTreeNode(db, col)"
                  @contextmenu.prevent="onColContextMenu($event, db, col)"
                >
                  <span class="table-icon-spacer" />
                  <Layers :size="14" class="table-icon" />
                  <span class="table-name">{{ col }}</span>
                </div>
                <div v-if="!collections[db] || collections[db].length === 0" class="empty-hint">
                  {{ t('mongodb.noData') }}
                </div>
              </div>
            </div>
            <div v-if="filteredDatabases.length === 0 && !treeLoading" class="empty-hint">
              {{ t('mongodb.noData') }}
            </div>
          </template>
        </div>
      </div>

      <!-- Resizer -->
      <div class="mongo-resizer" @mousedown="onResizeStart" />

      <!-- Right content area -->
      <div class="mongo-right">
        <!-- Breadcrumb -->
        <div class="mongo-breadcrumb" v-if="activeDb">
          <span class="crumb crumb-static">{{ activeDb }}</span>
          <template v-if="activeCollection">
            <span class="crumb-sep">/</span>
            <span class="crumb current">{{ activeCollection }}</span>
          </template>
        </div>

        <!-- Sub-tabs -->
        <div class="mongo-tabs" v-if="activeDb && activeCollection">
          <button class="mongo-tab" :class="{ active: activeSubTab === 'query' }" @click="activeSubTab = 'query'">
            {{ t('mongodb.queryTab') }}
          </button>
          <button class="mongo-tab" :class="{ active: activeSubTab === 'indexes' }" @click="activeSubTab = 'indexes'; loadIndexes()">
            {{ t('mongodb.indexesTab') }}
          </button>
        </div>

        <!-- Query sub-tab -->
        <div v-if="activeSubTab === 'query' && activeCollection" class="query-section">
          <div class="editor-top" :style="{ height: topHeight + 'px' }">
            <div class="query-editor-wrap">
              <textarea
                ref="queryTextareaRef"
                v-model="filterText"
                class="query-textarea"
                :placeholder="t('mongodb.filter') + '  e.g. {}'"
                @keydown="onQueryKeydown"
              />
              <div class="exec-btn-wrapper">
                <button class="btn btn-primary exec-btn-overlay" @click="executeQuery">
                  {{ t('mongodb.executeQuery') }}
                </button>
                <span class="shortcut-hint">Ctrl+Enter</span>
              </div>
            </div>
            <div style="display:flex;gap:8px;margin-top:8px">
              <div class="query-field">
                <label>{{ t('mongodb.projection') }}</label>
                <textarea
                  v-model="projectionText"
                  class="query-field-input"
                  placeholder="{}"
                  rows="1"
                />
              </div>
              <div class="query-field" style="max-width:120px">
                <label>{{ t('mongodb.limit') }}</label>
                <el-input-number v-model="queryLimit" :min="1" :max="1000" size="small" controls-position="right" style="width:100%" />
              </div>
            </div>
          </div>

          <div class="editor-resizer" @mousedown="onTopResizeStart" />

          <div class="editor-bottom">
            <!-- Error display -->
            <div v-if="queryError" class="error-msg">{{ queryError }}</div>

            <!-- Loading overlay -->
            <div v-if="queryLoading" class="loading-overlay">
              <div class="loading-box">
                <div class="spinner" />
                <span class="loading-text">{{ t('db.loading') }}</span>
                <button class="btn btn-default" @click="cancelQuery">{{ t('common.cancel') }}</button>
              </div>
            </div>

            <!-- Results -->
            <div v-if="columns.length > 0" class="result-grid">
              <div class="result-info">
                {{ t('mongodb.totalDocs', { count: totalDocs }) }}
                <span v-if="queryResult">
                  | {{ t('mongodb.pageInfo', { start: (queryResult.skip || 0) + 1, end: Math.min((queryResult.skip || 0) + queryLimit, totalDocs), total: totalDocs }) }}
                </span>
              </div>
              <div class="result-table-wrap">
                <el-table
                  :data="tableData"
                  border
                  size="small"
                  style="width:100%"
                  :empty-text="t('db.noData')"
                  @row-dblclick="onRowDblClick"
                >
                  <el-table-column
                    v-for="col in columns"
                    :key="col"
                    :prop="col"
                    :label="col"
                    min-width="120"
                    show-overflow-tooltip
                  >
                    <template #default="{ row }">
                      <span class="cell-value">{{ formatCellValue(row[col]) }}</span>
                    </template>
                  </el-table-column>
                  <el-table-column width="80" fixed="right">
                    <template #default="{ row }">
                      <button class="btn btn-ghost btn-icon btn-sm" style="color:var(--text-secondary)" @click="onRowDblClick(row)">
                        <Pencil :size="14" />
                      </button>
                      <button class="btn btn-ghost btn-icon btn-sm" style="color:var(--error)" @click="deleteDocument(row)">
                        <Trash2 :size="14" />
                      </button>
                    </template>
                  </el-table-column>
                </el-table>
              </div>
              <div class="pagination">
                <button class="btn btn-default btn-sm" :disabled="currentSkip <= 0" @click="prevPage">
                  {{ t('mongodb.prev') }}
                </button>
                <button class="btn btn-default btn-sm" :disabled="currentSkip + queryLimit >= totalDocs" @click="nextPage">
                  {{ t('mongodb.next') }}
                </button>
              </div>
            </div>
            <div v-else-if="!queryLoading && activeCollection" class="db-placeholder">
              <span>{{ t('mongodb.noData') }}</span>
            </div>
          </div>
        </div>

        <!-- Indexes sub-tab -->
        <div v-if="activeSubTab === 'indexes' && activeCollection" class="indexes-section">
          <div v-if="indexLoading" class="loading-overlay">
            <div class="loading-box">
              <div class="spinner" />
              <span class="loading-text">{{ t('db.loading') }}</span>
            </div>
          </div>
          <el-table :data="indexes" border size="small" style="width:100%" :empty-text="t('db.noData')">
            <el-table-column prop="name" :label="t('db.colName')" show-overflow-tooltip />
            <el-table-column label="Fields">
              <template #default="{ row }">
                {{ (row as MongoIndexInfo).keys.join(', ') }}
              </template>
            </el-table-column>
            <el-table-column prop="type" :label="t('db.colType')" />
            <el-table-column prop="unique" label="Unique" width="80">
              <template #default="{ row }">
                {{ row.unique ? '✓' : '' }}
              </template>
            </el-table-column>
          </el-table>
        </div>

        <!-- Empty state -->
        <div v-if="!activeCollection && activeDb" class="db-placeholder">
          <span>{{ t('db.selectTableHint') }}</span>
        </div>
        <div v-if="!activeDb" class="db-placeholder">
          <span>{{ t('db.loading') }}</span>
        </div>
      </div>
    </div>

    <!-- Context menu -->
    <div
      v-if="ctxVisible"
      class="ctx-menu"
      :style="{ left: ctxX + 'px', top: ctxY + 'px' }"
      @click.stop
    >
      <template v-if="ctxTargetType === 'db'">
        <div class="ctx-item" @click="onCtxOpenQuery">{{ t('mongodb.openQuery') }}</div>
        <div class="ctx-sep" />
        <div class="ctx-item" @click="onCtxRefresh">{{ t('mongodb.refresh') }}</div>
        <div class="ctx-sep" />
        <div class="ctx-item danger" @click="onCtxDropDatabase">{{ t('mongodb.dropDatabase') }}</div>
      </template>
      <template v-else-if="ctxTargetType === 'col'">
        <div class="ctx-item" @click="onCtxOpenColQuery">{{ t('mongodb.openQuery') }}</div>
        <div class="ctx-item" @click="onCtxViewIndexes">{{ t('mongodb.indexesTab') }}</div>
        <div class="ctx-sep" />
        <div class="ctx-item" @click="onCtxCopyName">{{ t('mongodb.copyName') }}</div>
        <div class="ctx-sep" />
        <div class="ctx-item danger" @click="onCtxDropCollection">{{ t('mongodb.dropCollection') }}</div>
      </template>
    </div>

    <!-- Confirm dialog -->
    <el-dialog
      v-model="confirmVisible"
      :title="confirmTitle"
      width="420px"
    >
      <div class="confirm-body">
        <p class="confirm-text">{{ confirmText }}</p>
        <p class="confirm-hint">{{ t('mongodb.typeToConfirm', { name: confirmName }) }}</p>
        <el-input v-model="confirmInput" :placeholder="confirmName" />
      </div>
      <template #footer>
        <button class="btn btn-default" @click="confirmVisible = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-danger" :disabled="confirmInput !== confirmName" @click="onConfirm">
          {{ t('common.confirm') }}
        </button>
      </template>
    </el-dialog>

    <!-- Document editor dialog -->
    <el-dialog
      v-model="docDialogVisible"
      :title="docDialogMode === 'insert' ? t('mongodb.newDocument') : t('mongodb.editDocument')"
      width="600px"
    >
      <textarea
        v-model="docEditorText"
        class="doc-editor-textarea"
        placeholder="{}"
        rows="14"
      />
      <div v-if="docEditorError" class="error-msg" style="margin-top:8px">{{ docEditorError }}</div>
      <template #footer>
        <button class="btn btn-default" @click="docDialogVisible = false">{{ t('settings.cancel') }}</button>
        <button class="btn btn-primary" @click="saveDocument" :disabled="docSaving">
          {{ t('redis.save') }}
        </button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, onMounted, onUnmounted, computed } from 'vue'
import { Database, Layers, ChevronRight, ChevronDown, RefreshCw, Plus, Pencil, Trash2 } from '@lucide/vue'
import { ElMessageBox } from 'element-plus'
import { useI18n } from '../i18n'
import { msg } from '../services/message'
import {
  MongoListDatabases,
  MongoListCollections,
  MongoFind,
  MongoInsertOne,
  MongoUpdateOne,
  MongoDeleteOne,
  MongoListIndexes,
  MongoDropCollection,
  MongoDropDatabase,
} from '../../wailsjs/go/main/App'
import type { MongoIndexInfo, MongoQueryResult } from '../types/mongodb'

const { t } = useI18n()

const props = defineProps<{
  sessionId: string
}>()

// ── Resize state ──
const leftWidth = ref(220)
let resizeStartX = 0
let resizeStartWidth = 0
let resizing = false

function onResizeStart(e: MouseEvent) {
  resizeStartX = e.clientX
  resizeStartWidth = leftWidth.value
  resizing = true
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
}
function onResizeMove(e: MouseEvent) {
  const dx = e.clientX - resizeStartX
  leftWidth.value = Math.max(150, Math.min(500, resizeStartWidth + dx))
}
function onResizeEnd() {
  resizing = false
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
}

// ── Editor resize state ──
const topHeight = ref(180)
let topResizeStartY = 0
let topResizeStartHeight = 0
let topResizing = false

function onTopResizeStart(e: MouseEvent) {
  topResizeStartY = e.clientY
  topResizeStartHeight = topHeight.value
  topResizing = true
  document.addEventListener('mousemove', onTopResizeMove)
  document.addEventListener('mouseup', onTopResizeEnd)
}
function onTopResizeMove(e: MouseEvent) {
  const el = (e.target as HTMLElement).closest('.mongo-right')
  const maxTop = el ? el.clientHeight - 100 : 400
  const dy = e.clientY - topResizeStartY
  topHeight.value = Math.max(80, Math.min(maxTop, topResizeStartHeight + dy))
}
function onTopResizeEnd() {
  topResizing = false
  document.removeEventListener('mousemove', onTopResizeMove)
  document.removeEventListener('mouseup', onTopResizeEnd)
}

// ── Tree state ──
const databases = ref<string[]>([])
const collections = ref<Record<string, string[]>>({})
const expandedDbs = reactive(new Set<string>())
const treeLoading = ref(false)
const treeSearchQuery = ref('')
const activeDb = ref('')
const activeCollection = ref('')

const filteredDatabases = computed(() => {
  const q = treeSearchQuery.value.trim().toLowerCase()
  if (!q) return databases.value
  return databases.value.filter(db => db.toLowerCase().includes(q))
})

// ── Context menu state ──
const ctxVisible = ref(false)
const ctxX = ref(0)
const ctxY = ref(0)
const ctxTargetType = ref<'db' | 'col'>('db')
const ctxDbName = ref('')
const ctxColName = ref('')

function closeContextMenu() {
  ctxVisible.value = false
}

function fitContextMenu(x: number, y: number, type: string) {
  const heights: Record<string, number> = { db: 130, col: 170 }
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

// ── Confirm dialog state ──
const confirmVisible = ref(false)
const confirmTitle = ref('')
const confirmText = ref('')
const confirmName = ref('')
const confirmInput = ref('')
let confirmAction: (() => void) | null = null

function askConfirm(title: string, text: string, name: string, action: () => void) {
  confirmTitle.value = title
  confirmText.value = text
  confirmName.value = name
  confirmInput.value = ''
  confirmAction = action
  confirmVisible.value = true
}

function onConfirm() {
  confirmVisible.value = false
  if (confirmAction) confirmAction()
}

// ── Query state ──
const activeSubTab = ref<'query' | 'indexes'>('query')
const filterText = ref('{}')
const projectionText = ref('{}')
const queryLimit = ref(100)
const queryLoading = ref(false)
const queryError = ref('')
const queryResult = ref<MongoQueryResult | null>(null)
const currentSkip = ref(0)
const queryTextareaRef = ref<HTMLTextAreaElement | null>(null)

const totalDocs = computed(() => queryResult.value?.total || 0)

const tableData = computed(() => {
  if (!queryResult.value) return []
  return queryResult.value.documents.map(d => {
    try { return JSON.parse(d) } catch { return {} }
  })
})

const columns = computed(() => {
  const keySet = new Set<string>()
  for (const row of tableData.value) {
    for (const key of Object.keys(row)) {
      keySet.add(key)
    }
  }
  const cols = Array.from(keySet)
  const idIdx = cols.indexOf('_id')
  if (idIdx > 0) {
    cols.splice(idIdx, 1)
    cols.unshift('_id')
  }
  return cols
})

// ── Index state ──
const indexes = ref<MongoIndexInfo[]>([])
const indexLoading = ref(false)

// ── Document editor state ──
const docDialogVisible = ref(false)
const docDialogMode = ref<'insert' | 'edit'>('insert')
const docEditorText = ref('{}')
const docEditorError = ref('')
const docSaving = ref(false)
const editingRow = ref<any>(null)

// ── Tree methods ──
async function refreshDatabases() {
  if (!props.sessionId) return
  treeLoading.value = true
  try {
    databases.value = await MongoListDatabases(props.sessionId)
  } catch (e: any) {
    const err = e?.message || String(e)
    // Retry once after a brief delay in case session isn't fully ready yet
    if (err.includes('not connected') || err.includes('session not found')) {
      await new Promise(r => setTimeout(r, 300))
      try {
        databases.value = await MongoListDatabases(props.sessionId)
        treeLoading.value = false
        return
      } catch (_e2: any) {
        msg.error(_e2?.message || String(_e2))
      }
    } else {
      msg.error(err)
    }
  }
  treeLoading.value = false
}

async function toggleDb(db: string) {
  if (expandedDbs.has(db)) {
    expandedDbs.delete(db)
  } else {
    expandedDbs.add(db)
    if (!collections.value[db]) {
      try {
        collections.value[db] = await MongoListCollections(props.sessionId, db)
        collections.value = { ...collections.value }
      } catch (e: any) {
        msg.error(e?.message || String(e))
      }
    }
  }
}

function selectTreeNode(db: string, col: string) {
  activeDb.value = db
  activeCollection.value = col
}

function selectCollection(db: string, col: string) {
  activeDb.value = db
  activeCollection.value = col
  activeSubTab.value = 'query'
  filterText.value = '{}'
  projectionText.value = '{}'
  currentSkip.value = 0
  queryResult.value = null
  queryError.value = ''
  executeQuery()
}

// ── Context menu handlers ──
function onTreeContextMenu(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (target.closest('.db-header') || target.closest('.table-item')) return
}

function onDbContextMenu(e: MouseEvent, db: string) {
  ctxTargetType.value = 'db'
  ctxDbName.value = db
  ctxColName.value = ''
  const pos = fitContextMenu(e.clientX, e.clientY, 'db')
  ctxX.value = pos.left
  ctxY.value = pos.top
  ctxVisible.value = true
}

function onColContextMenu(e: MouseEvent, db: string, col: string) {
  ctxTargetType.value = 'col'
  ctxDbName.value = db
  ctxColName.value = col
  const pos = fitContextMenu(e.clientX, e.clientY, 'col')
  ctxX.value = pos.left
  ctxY.value = pos.top
  ctxVisible.value = true
}

function onCtxOpenQuery() {
  activeDb.value = ctxDbName.value
  activeCollection.value = ''
  ctxVisible.value = false
}

function onCtxOpenColQuery() {
  selectCollection(ctxDbName.value, ctxColName.value)
  ctxVisible.value = false
}

function onCtxViewIndexes() {
  selectCollection(ctxDbName.value, ctxColName.value)
  activeSubTab.value = 'indexes'
  loadIndexes()
  ctxVisible.value = false
}

function onCtxCopyName() {
  navigator.clipboard.writeText(ctxColName.value)
  ctxVisible.value = false
}

function onCtxRefresh() {
  refreshDatabases()
  ctxVisible.value = false
}

function onCtxDropDatabase() {
  const db = ctxDbName.value
  askConfirm(
    t('mongodb.dropDatabase'),
    t('mongodb.dropDatabase') + ': ' + db,
    db,
    async () => {
      try {
        await MongoDropDatabase(props.sessionId, db)
        msg.success(t('mongodb.databaseDropped'))
        databases.value = databases.value.filter(d => d !== db)
        if (activeDb.value === db) {
          activeDb.value = ''
          activeCollection.value = ''
        }
      } catch (e: any) {
        msg.error(e?.message || String(e))
      }
    }
  )
  ctxVisible.value = false
}

function onCtxDropCollection() {
  const db = ctxDbName.value
  const col = ctxColName.value
  askConfirm(
    t('mongodb.dropCollection'),
    t('mongodb.dropCollection') + ': ' + col,
    col,
    async () => {
      try {
        await MongoDropCollection(props.sessionId, db, col)
        msg.success(t('mongodb.collectionDropped'))
        if (collections.value[db]) {
          collections.value[db] = collections.value[db].filter(c => c !== col)
          collections.value = { ...collections.value }
        }
        if (activeCollection.value === col) {
          activeCollection.value = ''
        }
      } catch (e: any) {
        msg.error(e?.message || String(e))
      }
    }
  )
  ctxVisible.value = false
}

// ── Query methods ──
function onQueryKeydown(e: KeyboardEvent) {
  if (e.ctrlKey && e.key === 'Enter') {
    e.preventDefault()
    executeQuery()
  }
}

async function executeQuery() {
  if (!activeDb.value || !activeCollection.value) return
  queryLoading.value = true
  queryError.value = ''
  try {
    queryResult.value = await MongoFind(
      props.sessionId,
      activeDb.value,
      activeCollection.value,
      filterText.value,
      currentSkip.value,
      queryLimit.value
    )
  } catch (e: any) {
    queryError.value = e?.message || String(e)
    queryResult.value = null
  }
  queryLoading.value = false
}

function refreshQuery() {
  executeQuery()
}

function cancelQuery() {
  queryLoading.value = false
}

function prevPage() {
  currentSkip.value = Math.max(0, currentSkip.value - queryLimit.value)
  executeQuery()
}

function nextPage() {
  currentSkip.value = currentSkip.value + queryLimit.value
  executeQuery()
}

// ── Document CRUD ──
function openNewDocument() {
  docDialogMode.value = 'insert'
  docEditorText.value = '{}'
  docEditorError.value = ''
  editingRow.value = null
  docDialogVisible.value = true
}

function onRowDblClick(row: any) {
  docDialogMode.value = 'edit'
  docEditorText.value = JSON.stringify(row, null, 2)
  docEditorError.value = ''
  editingRow.value = row
  docDialogVisible.value = true
}

async function saveDocument() {
  try {
    JSON.parse(docEditorText.value)
  } catch {
    docEditorError.value = t('mongodb.invalidJSON')
    return
  }
  docEditorError.value = ''
  docSaving.value = true
  try {
    if (docDialogMode.value === 'insert') {
      await MongoInsertOne(props.sessionId, activeDb.value, activeCollection.value, docEditorText.value)
      msg.success(t('mongodb.insertSuccess'))
    } else {
      const filter = JSON.stringify({ _id: editingRow.value._id })
      const updateObj = JSON.parse(docEditorText.value)
      delete updateObj._id
      await MongoUpdateOne(props.sessionId, activeDb.value, activeCollection.value, filter, JSON.stringify(updateObj))
      msg.success(t('mongodb.updateSuccess'))
    }
    docDialogVisible.value = false
    executeQuery()
  } catch (e: any) {
    msg.error(e?.message || String(e))
  }
  docSaving.value = false
}

async function deleteDocument(row: any) {
  try {
    await ElMessageBox.confirm(t('mongodb.deleteConfirm'))
  } catch {
    return
  }
  try {
    const filter = JSON.stringify({ _id: row._id })
    await MongoDeleteOne(props.sessionId, activeDb.value, activeCollection.value, filter)
    msg.success(t('mongodb.deleteSuccess'))
    executeQuery()
  } catch (e: any) {
    msg.error(e?.message || String(e))
  }
}

// ── Indexes ──
async function loadIndexes() {
  if (!activeDb.value || !activeCollection.value) return
  indexLoading.value = true
  try {
    indexes.value = await MongoListIndexes(props.sessionId, activeDb.value, activeCollection.value)
  } catch (e: any) {
    msg.error(e?.message || String(e))
  }
  indexLoading.value = false
}

// ── Helpers ──
function formatCellValue(val: any): string {
  if (val === null || val === undefined) return 'null'
  if (typeof val === 'object') return JSON.stringify(val)
  return String(val)
}

// ── Lifecycle ──
onMounted(() => {
  document.addEventListener('click', closeContextMenu)
  if (props.sessionId) {
    refreshDatabases()
  }
})

onUnmounted(() => {
  if (resizing) {
    document.removeEventListener('mousemove', onResizeMove)
    document.removeEventListener('mouseup', onResizeEnd)
  }
  if (topResizing) {
    document.removeEventListener('mousemove', onTopResizeMove)
    document.removeEventListener('mouseup', onTopResizeEnd)
  }
  document.removeEventListener('click', closeContextMenu)
})

watch(() => props.sessionId, () => {
  if (props.sessionId) {
    activeDb.value = ''
    activeCollection.value = ''
    databases.value = []
    collections.value = {}
    expandedDbs.clear()
    refreshDatabases()
  }
})
</script>

<style scoped>
/* ── Root ── */
.mongodb-tab-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}

/* ── Main layout ── */
.mongo-main {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.mongo-left {
  flex-shrink: 0;
  border-right: 1px solid var(--border-subtle);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.mongo-resizer {
  width: 4px;
  cursor: col-resize;
  background: transparent;
  flex-shrink: 0;
  transition: background 0.15s ease;
}
.mongo-resizer:hover {
  background: var(--border-subtle);
}

.mongo-right {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* ── Search ── */
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

/* ── Tree ── */
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
.child-list {
  /* indent is via table-icon-spacer on each item */
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

/* ── Breadcrumb ── */
.mongo-breadcrumb {
  display: flex;
  align-items: center;
  padding: 4px 12px;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
  white-space: nowrap;
  overflow: hidden;
}
.crumb {
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}
.crumb.current {
  color: var(--text-primary);
  font-weight: 600;
}
.crumb-sep {
  color: var(--text-disabled);
  margin: 0 2px;
  flex-shrink: 0;
}

/* ── Sub-tabs ── */
.mongo-tabs {
  display: flex;
  border-bottom: 1px solid var(--border-subtle);
  padding: 0 8px;
  flex-shrink: 0;
}
.mongo-tab {
  padding: 6px 16px;
  border: none;
  background: none;
  color: var(--text-secondary);
  cursor: pointer;
  font-family: var(--font-ui);
  font-size: 13px;
  border-bottom: 2px solid transparent;
  transition: all 0.15s ease;
}
.mongo-tab:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}
.mongo-tab.active {
  color: var(--text-primary);
  border-bottom-color: var(--accent);
}

/* ── Query editor ── */
.query-section {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-height: 0;
}
.editor-top {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 8px 8px 0;
}
.query-editor-wrap {
  position: relative;
  flex: 1;
  display: flex;
}
.query-textarea {
  flex: 1;
  width: 100%;
  font-family: var(--font-mono);
  font-size: 13px;
  line-height: 1.5;
  background: var(--bg-base);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 8px 80px 36px 8px;
  resize: none;
  transition: border-color 0.15s ease;
}
.query-textarea:focus {
  border-color: var(--accent);
  outline: none;
}
.exec-btn-wrapper {
  position: absolute;
  left: 6px;
  bottom: 6px;
  display: flex;
  align-items: center;
  gap: 6px;
  z-index: 1;
}
.exec-btn-overlay {
  padding: 4px 14px;
  font-size: 12px;
}
.shortcut-hint {
  font-family: var(--font-ui);
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
}
.query-field {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
}
.query-field label {
  font-family: var(--font-ui);
  font-size: 11px;
  font-weight: 500;
  color: var(--text-muted);
}
.query-field-input {
  font-family: var(--font-mono);
  font-size: 13px;
  line-height: 1.5;
  background: var(--bg-base);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 6px 8px;
  resize: none;
}
.query-field-input:focus {
  border-color: var(--accent);
  outline: none;
}

/* ── Editor resizer ── */
.editor-resizer {
  height: 4px;
  cursor: row-resize;
  background: transparent;
  flex-shrink: 0;
  transition: background 0.15s ease;
}
.editor-resizer:hover {
  background: var(--border-subtle);
}

/* ── Editor bottom (results) ── */
.editor-bottom {
  flex: 1;
  padding: 0 8px 8px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 0;
  position: relative;
}

/* ── Error ── */
.error-msg {
  color: var(--error);
  padding: 8px;
  background: var(--error-subtle);
  border-radius: var(--radius-sm);
  margin-bottom: 8px;
  user-select: text;
  -webkit-user-select: text;
  cursor: text;
  font-family: var(--font-mono);
  font-size: 13px;
}

/* ── Loading overlay ── */
.loading-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.3);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 10;
}
.loading-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 24px 36px;
  background: var(--bg-elevated);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-subtle);
}
.spinner {
  width: 28px;
  height: 28px;
  border: 3px solid var(--border-subtle);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
.loading-text {
  font-family: var(--font-ui);
  font-size: 13px;
  color: var(--text-secondary);
}

/* ── Result grid ── */
.result-grid {
  flex: 1;
  overflow: auto;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.result-info {
  padding: 4px 0;
  font-family: var(--font-ui);
  font-size: 13px;
  color: var(--text-secondary);
  flex-shrink: 0;
}
.result-table-wrap {
  flex: 1;
  overflow: auto;
  min-height: 0;
}
.cell-value {
  font-size: 12px;
  word-break: break-all;
  cursor: default;
}
.pagination {
  display: flex;
  justify-content: center;
  gap: 8px;
  margin-top: 8px;
  flex-shrink: 0;
}

/* ── Indexes ── */
.indexes-section {
  flex: 1;
  overflow: auto;
  padding: 8px;
  position: relative;
}

/* ── Placeholder ── */
.db-placeholder {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary);
  font-family: var(--font-ui);
  font-size: 14px;
}

/* ── Context menu ── */
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

/* ── Confirm dialog ── */
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

/* ── Document editor ── */
.doc-editor-textarea {
  width: 100%;
  font-family: var(--font-mono);
  font-size: 13px;
  line-height: 1.5;
  background: var(--bg-base);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 8px;
  resize: vertical;
}
.doc-editor-textarea:focus {
  border-color: var(--accent);
  outline: none;
}
</style>
