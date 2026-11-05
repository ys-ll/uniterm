<template>
  <div class="db-object-list">
    <div class="object-toolbar">
      <input
        v-model="search"
        class="object-search"
        :placeholder="t('db.searchTables')"
      />
      <button class="btn btn-default btn-sm" @click="openNewTable">
        <Plus :size="'0.875rem'" /> {{ t('db.newTable') }}
      </button>
    </div>
    <!-- 大数据量列表：用轻量分页列表替代 el-table，避免数千行一次性渲染导致渲染进程卡死 -->
    <div v-loading="loading" class="object-table">
      <div class="object-table-head">
        <span class="oc-name">{{ t('db.colName') }}</span>
        <span class="oc-type">{{ t('db.colType') }}</span>
        <span class="oc-comment">{{ t('db.colComment') }}</span>
        <span class="oc-actions">{{ t('db.actions') }}</span>
      </div>
      <div class="object-table-body">
        <div
          v-for="row in paged"
          :key="row.name + row.type"
          class="object-row"
        >
          <span class="oc-name" :title="row.name">
            <span class="object-name" @click="onRowClick(row)">
              <component :is="row.type === 'view' ? Eye : Table2" :size="'0.875rem'" class="object-icon" />
              <span class="object-name-text">{{ row.name }}</span>
            </span>
          </span>
          <span class="oc-type">{{ row.type === 'view' ? t('db.typeView') : t('db.typeTable') }}</span>
          <span class="oc-comment" :title="row.comment">{{ row.comment || '' }}</span>
          <span class="oc-actions">
            <button
              v-if="row.type === 'view'"
              class="btn btn-ghost btn-icon btn-sm danger"
              :title="t('db.dropView')"
              @click.stop="askDropView(row)"
            >
              <Trash2 :size="'0.875rem'" />
            </button>
            <template v-else>
              <button class="btn btn-ghost btn-icon btn-sm" :title="t('db.truncateTable')" @click.stop="askTruncate(row)">
                <Eraser :size="'0.875rem'" />
              </button>
              <button class="btn btn-ghost btn-icon btn-sm danger" :title="t('db.dropTable')" @click.stop="askDrop(row)">
                <Trash2 :size="'0.875rem'" />
              </button>
            </template>
          </span>
        </div>
        <div v-if="!loading && filtered.length === 0" class="object-empty">{{ t('db.noTables') }}</div>
      </div>
      <div v-if="!loading && allLoaded === false" class="object-loading-all">
        <span class="spinner" /> {{ t('db.loadingAllTables') }}
      </div>
      <div v-if="totalPages > 1" class="object-pager">
        <button class="btn btn-ghost btn-icon btn-sm" :disabled="currentPage <= 1" @click="currentPage--">
          <ChevronLeft :size="'0.875rem'" />
        </button>
        <span class="pager-info">{{ currentPage }} / {{ totalPages }}</span>
        <button class="btn btn-ghost btn-icon btn-sm" :disabled="currentPage >= totalPages" @click="currentPage++">
          <ChevronRight :size="'0.875rem'" />
        </button>
      </div>
    </div>

    <!-- Confirm dialog (type-to-confirm), mirrors the tree context menu -->
    <el-dialog append-to-body v-model="confirmVisible" :title="confirmTitle" width="26.25rem">
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

    <!-- New Table dialog -->
    <el-dialog append-to-body v-model="newTableVisible" :title="t('db.newTable')" width="23.75rem">
      <el-form label-width="5rem">
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
import { ref, computed, watch } from 'vue'
import { Table2, Eye, Eraser, Trash2, Plus, ChevronLeft, ChevronRight } from '@lucide/vue'
import { useI18n } from '../i18n'
import { GetTables, GetTablePage, CreateTable, DropTable, DropView, TruncateTable } from '../../bindings/github.com/ys-ll/uniterm/app'
import { msg } from '../services/message'
import type { TableInfo } from '../types/database'

defineOptions({ name: 'DBObjectList' })

const { t } = useI18n()

const props = defineProps<{
  sessionId: string
  dbName: string
}>()

const emit = defineEmits<{
  open: [dbName: string, tableName: string, isView?: boolean]
  changed: [dbName: string]
  objectRemoved: [payload: { dbName: string; tableName?: string; kind: 'table' | 'view' | 'database' }]
}>()

// ── 轻量分页：全量就绪后为内存分页（秒开）；未就绪时首屏只有第一页 ──
const PAGE_SIZE = 200
const currentPage = ref(1)

const objects = ref<TableInfo[]>([])
const search = ref('')
const loading = ref(false)
// allLoaded: 全量是否已就绪。就绪后翻页/搜索都走内存（秒开），避免每页重跑
// Oracle 深分页查询（offset 越大越慢）造成的卡顿。
const allLoaded = ref(false)
let loadSeq = 0

const totalPages = computed(() => Math.max(1, Math.ceil(filtered.value.length / PAGE_SIZE)))
const paged = computed(() => {
  const start = (currentPage.value - 1) * PAGE_SIZE
  return filtered.value.slice(start, start + PAGE_SIZE)
})

// 折中方案：首屏先用 GetTablePage 只取第一页（近瞬时显示），同时后台用
// GetTables 拉全量；全量就绪后翻页秒开。loadSeq 用于防止切换库时旧请求
// 的异步结果覆盖新库的数据。
async function load() {
  if (!props.sessionId || !props.dbName) return
  const seq = ++loadSeq
  loading.value = true
  allLoaded.value = false
  objects.value = []
  try {
    const [first] = await GetTablePage(props.sessionId, props.dbName, '', 0, PAGE_SIZE)
    if (seq !== loadSeq) return
    objects.value = first
    loading.value = false // 首屏就绪，立即显示第一页
    const all = await GetTables(props.sessionId, props.dbName)
    if (seq !== loadSeq) return
    objects.value = all
    allLoaded.value = true
  } catch {
    if (seq !== loadSeq) return
    objects.value = []
    loading.value = false
  }
}

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return objects.value
  return objects.value.filter(o => o.name.toLowerCase().includes(q))
})

watch(() => [props.sessionId, props.dbName], () => {
  currentPage.value = 1
  load()
}, { immediate: true })

watch([search, filtered], () => {
  currentPage.value = 1
})

function onRowClick(row: TableInfo) {
  emit('open', props.dbName, row.name, row.type === 'view')
}

// ── Confirm dialog (type-to-confirm) ──

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
}

async function onConfirm() {
  if (confirmAction) {
    try {
      await confirmAction()
    } catch (e: any) {
      msg.error(e?.message || String(e))
    }
  }
  confirmVisible.value = false
}

function askTruncate(row: TableInfo) {
  showConfirm(
    t('db.truncateTable'),
    t('db.truncateTableConfirm', { name: row.name }),
    row.name,
    async () => {
      await TruncateTable(props.sessionId, props.dbName, row.name)
      emit('changed', props.dbName)
    }
  )
}

function askDrop(row: TableInfo) {
  showConfirm(
    t('db.dropTable'),
    t('db.dropTableConfirm', { name: row.name }),
    row.name,
    async () => {
      await DropTable(props.sessionId, props.dbName, row.name)
      await load()
      emit('changed', props.dbName)
      emit('objectRemoved', { dbName: props.dbName, tableName: row.name, kind: 'table' })
    }
  )
}

function askDropView(row: TableInfo) {
  showConfirm(
    t('db.dropView'),
    t('db.dropViewConfirm', { name: row.name }),
    row.name,
    async () => {
      await DropView(props.sessionId, props.dbName, row.name)
      await load()
      emit('changed', props.dbName)
      emit('objectRemoved', { dbName: props.dbName, tableName: row.name, kind: 'view' })
    }
  )
}

// ── New Table dialog ──

const newTableVisible = ref(false)
const newTableName = ref('')

function openNewTable() {
  newTableName.value = ''
  newTableVisible.value = true
}

async function onCreateTable() {
  if (!newTableName.value.trim()) return
  try {
    await CreateTable(props.sessionId, props.dbName, newTableName.value.trim())
    newTableVisible.value = false
    await load()
    emit('changed', props.dbName)
  } catch (e: any) {
    msg.error(e?.message || String(e))
  }
}
</script>

<style scoped>
.db-object-list {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}
.object-toolbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  flex-shrink: 0;
}
.object-search {
  width: 15rem;
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
.object-search:focus {
  border-color: var(--accent);
}
.object-search::placeholder {
  color: var(--text-muted);
}

.object-table {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.object-table-head,
.object-row {
  display: grid;
  grid-template-columns: minmax(8rem, 1.4fr) 6rem minmax(6rem, 1fr) 6.25rem;
  gap: 0.5rem;
  align-items: center;
  padding: 0.25rem 0.75rem;
  font-size: 0.75rem;
}
.object-table-head {
  flex-shrink: 0;
  color: var(--text-muted);
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-raised, var(--bg-base));
  font-weight: 600;
}
.object-table-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.object-row {
  border-bottom: 1px solid var(--border-subtle);
  transition: background 0.15s ease;
}
.object-row:hover {
  background: var(--bg-raised, var(--bg-base));
}
.oc-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.oc-type {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
}
.oc-comment {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
}
.oc-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.25rem;
}
.object-name-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.object-empty {
  padding: 1.5rem 0.75rem;
  text-align: center;
  color: var(--text-muted);
  font-size: 0.75rem;
}
.object-loading-all {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.75rem;
  font-size: 0.72rem;
  color: var(--text-muted);
  border-top: 1px solid var(--border-subtle);
}
.spinner {
  width: 0.7rem;
  height: 0.7rem;
  border: 2px solid var(--border-strong, #888);
  border-top-color: transparent;
  border-radius: 50%;
  animation: db-loading-all-spin 0.8s linear infinite;
}
@keyframes db-loading-all-spin {
  to { transform: rotate(360deg); }
}
.object-pager {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 0.25rem 0.75rem;
  border-top: 1px solid var(--border-subtle);
}
.pager-info {
  font-size: 0.75rem;
  color: var(--text-muted);
  min-width: 4rem;
  text-align: center;
}
.object-toolbar .btn:last-child { margin-left: auto; }
.object-name {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  cursor: pointer;
  transition: color 0.15s ease;
}
.object-name:hover {
  color: var(--accent);
}
.object-icon {
  color: var(--text-muted);
  flex-shrink: 0;
}
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
