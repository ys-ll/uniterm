<template>
  <div class="db-tab-content">
    <div class="db-main">
      <div class="db-left" :style="{ width: leftWidth + 'px' }">
        <DBTreePanel
          ref="treeRef"
          :session-id="sessionId"
          :default-db-name="defaultDbName"
          :active-db="selectedDb"
          :active-table="selectedTable"
          @select-table="onSelectTable"
          @open-database="onOpenDatabase"
          @view-structure="onViewStructure"
        />
      </div>
      <div class="db-resizer" @mousedown="onResizeStart" />
      <div class="db-right">
        <div v-if="!selectedTable && !dbQueryMode" class="db-placeholder">
          <span>{{ t('db.selectTableHint') }}</span>
        </div>
        <!-- Database-level: query + object list -->
        <template v-else-if="dbQueryMode">
          <div class="db-breadcrumb">
            <span class="crumb crumb-static">{{ hostName }}</span>
            <span class="crumb-sep">/</span>
            <span class="crumb current">{{ selectedDb }}</span>
          </div>
          <div class="db-right-top">
            <div class="db-tabs">
              <button
                class="db-tab"
                :class="{ active: dbActiveTab === 'query' }"
                @click="dbActiveTab = 'query'"
              >
                {{ t('db.dataQuery') }}
              </button>
              <button
                class="db-tab"
                :class="{ active: dbActiveTab === 'objects' }"
                @click="dbActiveTab = 'objects'"
              >
                {{ t('db.tableList') }}
              </button>
            </div>
            <div class="db-right-top-content">
              <DBQueryEditor
                v-show="dbActiveTab === 'query'"
                :key="'dbquery-' + selectedDb"
                :session-id="sessionId"
                :db-name="selectedDb"
              />
              <DBObjectList
                v-if="dbActiveTab === 'objects'"
                :session-id="sessionId"
                :db-name="selectedDb"
                @open="onSelectTable"
                @changed="onObjectsChanged"
              />
            </div>
          </div>
        </template>
        <!-- Table-level: data query + structure -->
        <template v-else>
          <div class="db-breadcrumb">
            <span class="crumb crumb-static">{{ hostName }}</span>
            <span class="crumb-sep">/</span>
            <span class="crumb clickable" @click="onOpenDatabase(selectedDb)">{{ selectedDb }}</span>
            <span class="crumb-sep">/</span>
            <span class="crumb current">{{ selectedTable }}</span>
          </div>
          <div class="db-right-top">
            <div class="db-tabs">
              <button
                class="db-tab"
                :class="{ active: activeTab === 'query' }"
                @click="activeTab = 'query'"
              >
                {{ t('db.dataQuery') }}
              </button>
              <button
                v-if="!selectedIsView"
                class="db-tab"
                :class="{ active: activeTab === 'structure' }"
                @click="onStructureTabClick"
              >
                {{ t('db.tableStructure') }}
              </button>
            </div>
            <div class="db-right-top-content">
              <DBQueryEditor
                v-if="activeTab === 'query'"
                :session-id="sessionId"
                :table-name="selectedTable"
                :db-name="selectedDb"
                :primary-keys="primaryKeys"
                :table-columns="tableColumns"
                :is-view="selectedIsView"
              />
              <DBTableStructure
                v-else
                :session-id="sessionId"
                :db-name="selectedDb"
                :table-name="selectedTable"
                :db-type="props.dbType || 'mysql'"
                :load-trigger="structureLoadTrigger"
                @schema-loaded="onSchemaLoaded"
              />
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onUnmounted } from 'vue'
import { useI18n } from '../i18n'
import DBTreePanel from './DBTreePanel.vue'
import DBTableStructure from './DBTableStructure.vue'
import DBQueryEditor from './DBQueryEditor.vue'
import DBObjectList from './DBObjectList.vue'
import { GetTableSchema } from '../../wailsjs/go/main/App'
import type { ColumnInfo } from '../types/database'

defineOptions({ name: 'DBTabContent' })

const { t } = useI18n()

const props = defineProps<{
  sessionId: string
  hostName?: string
  defaultDbName?: string
  dbType?: string
}>()

const activeTab = ref<'structure' | 'query'>('query')
const dbActiveTab = ref<'query' | 'objects'>('query')
const treeRef = ref<InstanceType<typeof DBTreePanel> | null>(null)
const selectedDb = ref('')
const selectedTable = ref('')
const selectedIsView = ref(false)
const dbQueryMode = ref(false)
const primaryKeys = ref<string[]>([])
const tableColumns = ref<ColumnInfo[]>([])
const structureLoadTrigger = ref(0)

const leftWidth = ref(220)
let resizeStartX = 0
let resizeStartWidth = 0
let resizing = false

function onOpenDatabase(dbName: string, tab: 'query' | 'objects' = 'query') {
  selectedDb.value = dbName
  selectedTable.value = ''
  dbQueryMode.value = true
  dbActiveTab.value = tab
  primaryKeys.value = []
  tableColumns.value = []
}

async function onSelectTable(dbName: string, tableName: string, isView = false) {
  selectedDb.value = dbName
  selectedTable.value = tableName
  selectedIsView.value = isView
  dbQueryMode.value = false
  primaryKeys.value = []
  tableColumns.value = []
  activeTab.value = 'query'
  try {
    const schema = await GetTableSchema(props.sessionId, dbName, tableName)
    tableColumns.value = schema.columns
    primaryKeys.value = schema.columns.filter(c => c.isPrimary).map(c => c.name)
  } catch { /* ignore */ }
}

async function onViewStructure(dbName: string, tableName: string) {
  selectedDb.value = dbName
  selectedTable.value = tableName
  selectedIsView.value = false
  dbQueryMode.value = false
  primaryKeys.value = []
  tableColumns.value = []
  activeTab.value = 'structure'
  structureLoadTrigger.value++
  try {
    const schema = await GetTableSchema(props.sessionId, dbName, tableName)
    tableColumns.value = schema.columns
    primaryKeys.value = schema.columns.filter(c => c.isPrimary).map(c => c.name)
  } catch { /* ignore */ }
}

function onStructureTabClick() {
  activeTab.value = 'structure'
  structureLoadTrigger.value++
}

function onSchemaLoaded(pks: string[]) {
  primaryKeys.value = pks
}

function onObjectsChanged(dbName: string) {
  treeRef.value?.refreshDb(dbName)
}

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

onUnmounted(() => {
  if (resizing) {
    document.removeEventListener('mousemove', onResizeMove)
    document.removeEventListener('mouseup', onResizeEnd)
  }
})
</script>

<style scoped>
.db-tab-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.db-main {
  flex: 1;
  display: flex;
  overflow: hidden;
}
.db-left {
  flex-shrink: 0;
  border-right: 1px solid var(--border-subtle);
  overflow: hidden;
}
.db-resizer {
  width: 4px;
  cursor: col-resize;
  background: transparent;
  flex-shrink: 0;
  transition: background 0.15s ease;
}
.db-resizer:hover {
  background: var(--border-subtle);
}
.db-right {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.db-right-top {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.db-tabs {
  display: flex;
  border-bottom: 1px solid var(--border-subtle);
  padding: 0 8px;
  flex-shrink: 0;
}
.db-tab {
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
.db-tab:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}
.db-tab.active {
  color: var(--text-primary);
  border-bottom-color: var(--accent);
}
.db-tab:disabled {
  opacity: 0.4;
  cursor: default;
}
.db-right-top-content {
  flex: 1;
  overflow: hidden;
}
.db-placeholder {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary);
  font-family: var(--font-ui);
  font-size: 14px;
}
.db-breadcrumb {
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
.crumb.clickable {
  cursor: pointer;
  transition: all 0.1s ease;
}
.crumb.clickable:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
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
.db-right-bottom {
  height: 180px;
  border-top: 1px solid var(--border-subtle);
  overflow: auto;
  flex-shrink: 0;
}
</style>
