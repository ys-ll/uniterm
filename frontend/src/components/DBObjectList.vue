<template>
  <div class="db-object-list">
    <div class="object-toolbar">
      <input
        v-model="search"
        class="object-search"
        :placeholder="t('db.searchTables')"
      />
      <button class="btn btn-primary" @click="openNewTable">{{ t('db.newTable') }}</button>
    </div>
    <el-table
      :data="filtered"
      size="small"
      v-loading="loading"
      height="calc(100% - 44px)"
      class="object-table"
    >
      <el-table-column :label="t('db.colName')" prop="name" sortable>
        <template #default="{ row }">
          <span class="object-name" @click="onRowClick(row)">
            <component :is="row.type === 'view' ? Eye : Table2" :size="14" class="object-icon" />
            {{ row.name }}
          </span>
        </template>
      </el-table-column>
      <el-table-column :label="t('db.colType')" prop="type" sortable width="100">
        <template #default="{ row }">
          {{ row.type === 'view' ? t('db.typeView') : t('db.typeTable') }}
        </template>
      </el-table-column>
      <el-table-column :label="t('db.actions')" width="110" align="right">
        <template #default="{ row }">
          <button
            v-if="row.type === 'view'"
            class="btn btn-ghost btn-icon btn-sm danger"
            :title="t('db.dropView')"
            @click.stop="askDropView(row)"
          >
            <Trash2 :size="14" />
          </button>
          <template v-else>
            <button class="btn btn-ghost btn-icon btn-sm" :title="t('db.truncateTable')" @click.stop="askTruncate(row)">
              <Eraser :size="14" />
            </button>
            <button class="btn btn-ghost btn-icon btn-sm danger" :title="t('db.dropTable')" @click.stop="askDrop(row)">
              <Trash2 :size="14" />
            </button>
          </template>
        </template>
      </el-table-column>
    </el-table>

    <!-- Confirm dialog (type-to-confirm), mirrors the tree context menu -->
    <el-dialog append-to-body v-model="confirmVisible" :title="confirmTitle" width="420px">
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
import { ref, computed, watch } from 'vue'
import { Table2, Eye, Eraser, Trash2 } from '@lucide/vue'
import { useI18n } from '../i18n'
import { GetTables, CreateTable, DropTable, DropView, TruncateTable } from '../../wailsjs/go/main/App'
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
}>()

const objects = ref<TableInfo[]>([])
const search = ref('')
const loading = ref(false)

async function load() {
  if (!props.sessionId || !props.dbName) return
  loading.value = true
  try {
    objects.value = await GetTables(props.sessionId, props.dbName)
  } catch {
    objects.value = []
  } finally {
    loading.value = false
  }
}

watch(() => [props.sessionId, props.dbName], load, { immediate: true })

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return objects.value
  return objects.value.filter(o => o.name.toLowerCase().includes(q))
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
    async () => { await TruncateTable(props.sessionId, props.dbName, row.name) }
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
  gap: 8px;
  padding: 8px 12px;
  flex-shrink: 0;
}
.object-search {
  width: 240px;
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
.object-search:focus {
  border-color: var(--accent);
}
.object-search::placeholder {
  color: var(--text-muted);
}

.object-table {
  flex: 1;
  min-height: 0;
}
.object-name {
  display: inline-flex;
  align-items: center;
  gap: 6px;
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
