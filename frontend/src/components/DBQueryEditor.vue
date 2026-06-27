<template>
  <div class="db-query-editor">
    <div v-if="loading" class="loading-overlay">
      <div class="loading-box">
        <div class="spinner" />
        <span class="loading-text">{{ t('db.loading') }}</span>
        <button class="cancel-btn" @click="onCancelQuery">{{ t('common.cancel') }}</button>
      </div>
    </div>
    <div class="editor-top" :style="{ height: topHeight + 'px' }">
      <div class="sql-editor-wrap">
        <textarea
          v-model="sql"
          class="sql-editor"
          :placeholder="t('db.sqlPlaceholder')"
          @keydown="onKeydown"
        />
        <div class="exec-btn-wrapper">
          <button class="exec-btn exec-btn-overlay" @click="onExecute">{{ t('db.execute') }}</button>
          <span class="shortcut-hint">Ctrl+Enter</span>
        </div>
      </div>
    </div>
    <div class="editor-resizer" @mousedown="onResizeStart" />
    <div class="editor-bottom">
      <div v-if="error" class="error-msg">{{ error }}</div>
      <div v-if="execResult" class="result-info">
        {{ t('db.affectedRows') }}: {{ execResult.affected }}
      </div>
      <div v-if="queryResult" class="result-grid">
        <el-table
          :data="queryResult.rows"
          border
          size="small"
          style="width:100%"
          :empty-text="t('db.noData')"
          @cell-dblclick="onCellDblClick"
        >
          <el-table-column
            v-if="tableName && primaryKeys?.length"
            :label="t('db.actions')"
            width="120"
            fixed="right"
          >
            <template #default="{ $index }">
              <button class="action-icon-btn" title="Edit" @click="startEditRow($index)"><Pencil :size="14" /></button>
              <button class="action-icon-btn danger" title="Delete" @click="onDeleteRow($index)"><Trash2 :size="14" /></button>
            </template>
          </el-table-column>
          <el-table-column
            v-for="col in queryResult.columns"
            :key="col.name"
            :prop="col.name"
            :label="col.name"
            min-width="100"
            show-overflow-tooltip
          >
            <template #default="{ row, column, $index }">
              <div
                v-if="editingCell && editingCell.rowIndex === $index && editingCell.colName === column.property"
                class="cell-edit-wrap"
              >
                <input
                  ref="cellInputEl"
                  v-model="editingCell.value"
                  class="cell-edit-input"
                  @keydown.enter="onCellEditConfirm"
                  @keydown.escape="onCellEditCancel"
                  @blur="onCellEditCancel"
                />
              </div>
              <span v-else-if="row[column.property] === null" class="cell-null">NULL</span>
              <span v-else class="cell-value">{{ row[column.property] }}</span>
            </template>
          </el-table-column>
        </el-table>
        <div class="result-count">{{ queryResult.rows.length }} {{ t('db.rows') }}</div>
      </div>

      <div v-if="queryResult && tableName && primaryKeys?.length" class="insert-row-bar">
        <button class="exec-btn" @click="startInsertRow">{{ t('db.insertRow') }}</button>
      </div>

      <div v-if="insertingRow" class="insert-row-form">
        <div class="insert-row-fields">
          <div v-for="col in insertColumns" :key="col" class="insert-field">
            <div class="field-label-row">
              <label>{{ col }} <span class="col-type-hint">{{ getColumnType(col) }}</span></label>
              <label v-if="isColumnAuto(col)" class="null-toggle"><input type="checkbox" v-model="insertAutoIncrement[col]" /> 自增</label>
              <label v-else-if="!isColumnPrimary(col) && getColumnNullable(col)" class="null-toggle"><input type="checkbox" v-model="insertNulls[col]" /> NULL</label>
            </div>
            <input v-model="insertValues[col]" class="insert-input" :disabled="insertNulls[col] || insertAutoIncrement[col]" :placeholder="getColumnPlaceholder(col)" />
          </div>
        </div>
        <div class="insert-actions">
          <button class="exec-btn" @click="onInsertConfirm">{{ t('common.confirm') }}</button>
          <button class="cancel-btn" @click="onInsertCancel">{{ t('common.cancel') }}</button>
        </div>
      </div>

      <div v-if="editingRow" class="insert-row-form">
        <div class="insert-row-fields">
          <div v-for="col in editRowColumns" :key="col" class="insert-field">
            <div class="field-label-row">
              <label>{{ col }} <span class="col-type-hint">{{ getColumnType(col) }}</span></label>
              <label v-if="!isColumnPrimary(col) && getColumnNullable(col)" class="null-toggle"><input type="checkbox" v-model="editNulls[col]" /> NULL</label>
            </div>
            <input v-model="editRowValues[col]" class="insert-input" :disabled="editNulls[col]" />
          </div>
        </div>
        <div class="insert-actions">
          <button class="exec-btn" @click="onEditRowConfirm">{{ t('common.save') }}</button>
          <button class="cancel-btn" @click="onEditRowCancel">{{ t('common.cancel') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onMounted } from 'vue'
import { Pencil, Trash2 } from '@lucide/vue'
import { ElMessageBox } from 'element-plus'
import { useI18n } from '../i18n'
import { ExecuteQuery, ExecuteStatement, GetTableSchema } from '../../wailsjs/go/main/App'
import type { QueryResult, ExecResult, ColumnInfo } from '../types/database'

const { t } = useI18n()

const props = defineProps<{
  sessionId: string
  tableName?: string
  dbName?: string
  primaryKeys?: string[]
  tableColumns?: ColumnInfo[]
}>()

const emit = defineEmits<{
  cellUpdated: []
}>()

const sql = ref('')
const queryResult = ref<QueryResult | null>(null)
const execResult = ref<ExecResult | null>(null)
const error = ref('')
const loading = ref(false)
let cancelled = false

watch(() => props.tableName, async (name) => {
  insertingRow.value = false
  editingRow.value = false
  if (!name) return
  sql.value = `SELECT * FROM \`${name}\` LIMIT 100`
  await onExecute()
})

onMounted(async () => {
  if (!props.tableName) return
  sql.value = `SELECT * FROM \`${props.tableName}\` LIMIT 100`
  await onExecute()
})

function onKeydown(e: KeyboardEvent) {
  if (e.ctrlKey && e.key === 'Enter') {
    e.preventDefault()
    onExecute()
  }
}

async function onExecute() {
  if (!sql.value.trim()) return
  error.value = ''
  queryResult.value = null
  execResult.value = null
  loading.value = true
  cancelled = false

  const trimmed = sql.value.trim()
  const isSelect = /^\s*SELECT\b/i.test(trimmed) ||
    /^\s*SHOW\b/i.test(trimmed) ||
    /^\s*DESCRIBE\b/i.test(trimmed) ||
    /^\s*EXPLAIN\b/i.test(trimmed) ||
    /^\s*PRAGMA\b/i.test(trimmed)

  try {
    if (isSelect) {
      const result = await ExecuteQuery(props.sessionId, props.dbName || '', trimmed)
      if (!cancelled) queryResult.value = result
    } else {
      const result = await ExecuteStatement(props.sessionId, props.dbName || '', trimmed)
      if (!cancelled) execResult.value = result
    }
  } catch (e: any) {
    if (!cancelled) error.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}

function onCancelQuery() {
  cancelled = true
  loading.value = false
}

// ── Resize splitter ──

const topHeight = ref(180)
let resizeStartY = 0
let resizeStartHeight = 0

function onResizeStart(e: MouseEvent) {
  resizeStartY = e.clientY
  resizeStartHeight = topHeight.value
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
}

function onResizeMove(e: MouseEvent) {
  const dy = e.clientY - resizeStartY
  const el = document.querySelector('.db-query-editor') as HTMLElement
  const maxTop = el ? el.clientHeight - 100 : 600
  topHeight.value = Math.max(80, Math.min(maxTop, resizeStartHeight + dy))
}

function onResizeEnd() {
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
}

// ── Inline cell editing ──

interface EditingCell {
  rowIndex: number
  colName: string
  originalValue: any
  value: string
}

const editingCell = ref<EditingCell | null>(null)
const cellInputEl = ref<HTMLInputElement | null>(null)

function onCellDblClick(row: any, column: any, _cell: HTMLElement, _event: MouseEvent) {
  if (!props.tableName || !props.primaryKeys || props.primaryKeys.length === 0) return

  const colName = column.property
  const originalValue = row[colName]
  editingCell.value = {
    rowIndex: queryResult.value!.rows.indexOf(row),
    colName,
    originalValue,
    value: originalValue ?? ''
  }
  nextTick(() => {
    cellInputEl.value?.focus()
    cellInputEl.value?.select()
  })
}

async function onCellEditConfirm() {
  if (!editingCell.value || !props.tableName || !props.primaryKeys) return

  const { rowIndex, colName, originalValue, value } = editingCell.value
  if (value === String(originalValue ?? '')) {
    editingCell.value = null
    return
  }

  const row = queryResult.value!.rows[rowIndex]
  const whereParts = props.primaryKeys.map(
    pk => `\`${pk}\` = '${String(row[pk] ?? '').replace(/'/g, "''")}'`
  )
  const whereClause = whereParts.join(' AND ')
  const updateSQL = `UPDATE \`${props.tableName}\` SET \`${colName}\` = '${value.replace(/'/g, "''")}' WHERE ${whereClause}`

  try {
    await ExecuteStatement(props.sessionId, props.dbName || '', updateSQL)
    queryResult.value!.rows[rowIndex][colName] = value
    error.value = ''
    emit('cellUpdated')
  } catch (e: any) {
    error.value = e?.message || String(e)
  }
  editingCell.value = null
}

function onCellEditCancel() {
  editingCell.value = null
}

// ── Delete row ──

async function onDeleteRow(rowIndex: number) {
  if (!props.tableName || !props.primaryKeys || props.primaryKeys.length === 0) return

  try {
    await ElMessageBox.confirm(t('db.deleteRowConfirm'), t('common.confirm'), {
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }

  const row = queryResult.value!.rows[rowIndex]
  const whereParts = props.primaryKeys.map(
    pk => `\`${pk}\` = '${String(row[pk] ?? '').replace(/'/g, "''")}'`
  )
  const whereClause = whereParts.join(' AND ')
  const deleteSQL = `DELETE FROM \`${props.tableName}\` WHERE ${whereClause}`

  try {
    await ExecuteStatement(props.sessionId, props.dbName || '', deleteSQL)
    queryResult.value!.rows.splice(rowIndex, 1)
    error.value = ''
    emit('cellUpdated')
  } catch (e: any) {
    error.value = e?.message || String(e)
  }
}

// ── Insert row ──

const insertingRow = ref(false)
const insertValues = ref<Record<string, string>>({})
const insertNulls = ref<Record<string, boolean>>({})
const insertAutoIncrement = ref<Record<string, boolean>>({})
const insertColumns = ref<string[]>([])

async function startInsertRow() {
  let cols: ColumnInfo[]
  try {
    const schema = await GetTableSchema(props.sessionId, props.dbName || '', props.tableName || '')
    cols = schema.columns
  } catch {
    cols = queryResult.value!.columns.map(c => ({ name: c.name, type: c.type, nullable: true, defaultVal: '', defaultType: 'none', isPrimary: false, collation: '', comment: '', onUpdate: false }))
  }
  insertColumns.value = cols.map(c => c.name)
  insertNulls.value = {}
  insertAutoIncrement.value = {}
  insertValues.value = {}
  for (const col of cols) {
    if (col.defaultType === 'auto') {
      insertAutoIncrement.value[col.name] = true
      insertNulls.value[col.name] = false
      insertValues.value[col.name] = ''
    } else {
      const isNullDefault = col.defaultType === 'null' || (col.nullable && col.defaultType === 'none')
      insertNulls.value[col.name] = isNullDefault
      const rawDefault = col.defaultType === 'value' ? (col.defaultVal ?? '') : ''
      insertValues.value[col.name] = rawDefault === "''" ? '' : rawDefault
    }
  }
  editingRow.value = false
  insertingRow.value = true
}

async function onInsertConfirm() {
  if (!props.tableName) return

  const includedCols = insertColumns.value.filter(c => !insertAutoIncrement.value[c])
  const cols = includedCols.map(c => `\`${c}\``).join(', ')
  const vals = includedCols.map(c => {
    if (insertNulls.value[c]) return 'NULL'
    const v = insertValues.value[c] ?? ''
    return `'${v.replace(/'/g, "''")}'`
  }).join(', ')
  const insertSQL = `INSERT INTO \`${props.tableName}\` (${cols}) VALUES (${vals})`

  try {
    await ExecuteStatement(props.sessionId, props.dbName || '', insertSQL)
    error.value = ''
    insertingRow.value = false
    emit('cellUpdated')
    await onExecute()
  } catch (e: any) {
    error.value = e?.message || String(e)
  }
}

function onInsertCancel() {
  insertingRow.value = false
}

function getColumnType(colName: string): string {
  const col = props.tableColumns?.find(c => c.name === colName)
  return col?.type ?? ''
}

function isColumnPrimary(colName: string): boolean {
  const col = props.tableColumns?.find(c => c.name === colName)
  return col?.isPrimary ?? false
}

function isColumnAuto(colName: string): boolean {
  if (props.tableColumns) {
    const col = props.tableColumns.find(c => c.name === colName)
    if (col) return col.defaultType === 'auto'
  }
  return insertAutoIncrement.value[colName] === true
}

function getColumnNullable(colName: string): boolean {
  const col = props.tableColumns?.find(c => c.name === colName)
  return col?.nullable ?? true
}

function getColumnPlaceholder(colName: string): string {
  const col = props.tableColumns?.find(c => c.name === colName)
  const val = col?.defaultVal ?? ''
  return val === "''" ? '' : val
}

// ── Edit row ──

const editingRow = ref(false)
const editingRowIndex = ref(-1)
const editRowValues = ref<Record<string, string>>({})
const editNulls = ref<Record<string, boolean>>({})
const editRowColumns = ref<string[]>([])

function startEditRow(rowIndex: number) {
  editingRowIndex.value = rowIndex
  const row = queryResult.value!.rows[rowIndex]
  editRowColumns.value = queryResult.value!.columns.map(c => c.name)
  editRowValues.value = {}
  editNulls.value = {}
  for (const col of editRowColumns.value) {
    if (row[col] === null) {
      editRowValues.value[col] = ''
      editNulls.value[col] = true
    } else {
      editRowValues.value[col] = String(row[col] ?? '')
      editNulls.value[col] = false
    }
  }
  insertingRow.value = false
  editingRow.value = true
}

async function onEditRowConfirm() {
  if (!props.tableName) return
  if (!props.primaryKeys || props.primaryKeys.length === 0) {
    error.value = t('db.noPrimaryKey')
    return
  }
  if (editingRowIndex.value < 0) return

  const row = queryResult.value!.rows[editingRowIndex.value]
  const sets: string[] = []
  for (const col of editRowColumns.value) {
    if (editNulls.value[col]) {
      if (row[col] !== null) {
        sets.push(`\`${col}\` = NULL`)
      }
    } else {
      const newVal = editRowValues.value[col] ?? ''
      const oldVal = String(row[col] ?? '')
      if (newVal !== oldVal) {
        sets.push(`\`${col}\` = '${newVal.replace(/'/g, "''")}'`)
      }
    }
  }
  if (sets.length === 0) {
    editingRow.value = false
    return
  }

  const whereParts = props.primaryKeys.map(
    pk => `\`${pk}\` = '${String(row[pk] ?? '').replace(/'/g, "''")}'`
  )
  const sql = `UPDATE \`${props.tableName}\` SET ${sets.join(', ')} WHERE ${whereParts.join(' AND ')}`

  try {
    await ExecuteStatement(props.sessionId, props.dbName || '', sql)
    for (const col of editRowColumns.value) {
      queryResult.value!.rows[editingRowIndex.value][col] = editNulls.value[col] ? null : editRowValues.value[col]
    }
    error.value = ''
    editingRow.value = false
    emit('cellUpdated')
  } catch (e: any) {
    error.value = e?.message || String(e)
  }
}

function onEditRowCancel() {
  editingRow.value = false
}
</script>

<style scoped>
.db-query-editor {
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}
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
.cancel-btn {
  padding: 4px 16px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-family: var(--font-ui);
  font-size: 13px;
  transition: all 0.15s ease;
}
.cancel-btn:hover {
  background: var(--bg-hover);
  border-color: var(--border-hover);
}
.editor-top {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 8px 8px 0;
}
.sql-editor-wrap {
  position: relative;
  flex: 1;
  display: flex;
}
.sql-editor {
  flex: 1;
  width: 100%;
  font-family: var(--font-mono);
  font-size: 13px;
  line-height: 1.5;
  background: var(--bg-base);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 8px 80px 8px 8px;
  resize: none;
  transition: border-color 0.15s ease;
}
.sql-editor:focus {
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
  font-weight: 400;
  white-space: nowrap;
}
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
.editor-bottom {
  flex: 1;
  padding: 0 8px 8px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.exec-btn {
  padding: 4px 16px;
  background: var(--accent);
  color: #fff;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-family: var(--font-ui);
  font-size: 13px;
  transition: all 0.15s ease;
}
.exec-btn:hover {
  background: var(--accent-dim);
  box-shadow: 0 0 12px var(--accent-glow);
}
.error-msg {
  color: var(--error);
  padding: 8px;
  background: rgba(248, 113, 113, 0.1);
  border-radius: var(--radius-sm);
  margin-bottom: 8px;
  user-select: text;
  -webkit-user-select: text;
  cursor: text;
  font-family: var(--font-mono);
  font-size: 13px;
}
.result-info {
  padding: 4px 0;
  font-family: var(--font-ui);
  font-size: 13px;
  color: var(--text-secondary);
  flex-shrink: 0;
}
.result-grid { flex: 1; overflow: auto; display: flex; flex-direction: column; min-height: 0; }
.result-count {
  padding: 4px 0;
  font-family: var(--font-ui);
  font-size: 12px;
  color: var(--text-secondary);
}
.cell-value { cursor: default; }
.cell-null {
  color: var(--text-muted);
  font-style: italic;
  cursor: default;
}
.cell-edit-wrap { margin: -8px -12px; }
.cell-edit-input {
  width: 100%;
  padding: 4px 8px;
  border: 2px solid var(--accent);
  border-radius: var(--radius-sm);
  font-family: var(--font-ui);
  font-size: 13px;
  outline: none;
}
.action-icon-btn {
  border: none;
  background: none;
  color: var(--text-secondary);
  padding: 2px 4px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: all 0.12s ease;
}
.action-icon-btn:hover { color: var(--text-primary); background: var(--bg-hover); }
.action-icon-btn.danger:hover { color: var(--error); }
.insert-row-bar { padding: 4px 0; }
.insert-row-form {
  border: 1px solid var(--accent);
  border-radius: var(--radius-sm);
  padding: 8px;
  margin-top: 4px;
}
.insert-row-fields { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 8px; }
.insert-field { display: flex; flex-direction: column; gap: 2px; }
.field-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.field-label-row label {
  font-family: var(--font-ui);
  font-size: 11px;
  color: var(--text-secondary);
}
.null-toggle {
  display: flex;
  align-items: center;
  gap: 3px;
  font-size: 10px;
  cursor: pointer;
  user-select: none;
  color: var(--text-muted);
}
.null-toggle input {
  cursor: pointer;
  margin: 0;
}
.insert-input {
  padding: 4px 8px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  font-family: var(--font-ui);
  font-size: 13px;
  width: 140px;
  background: var(--bg-base);
  color: var(--text-primary);
}
.insert-input:disabled {
  background: var(--bg-elevated);
  color: var(--text-muted);
  border-color: var(--border-subtle);
  cursor: not-allowed;
}
.col-type-hint {
  font-size: 10px;
  color: var(--text-muted);
  font-weight: 400;
}
.insert-actions { display: flex; gap: 8px; }
</style>
