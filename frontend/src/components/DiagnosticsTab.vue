<template>
  <div class="diag-tab">
    <h2 class="section-title">{{ t('diag.title') }}</h2>
    <p class="section-desc">{{ t('diag.subtitle') }}</p>

    <!-- KPI strip. Pulled from diag.Snapshot() on each refresh; reads stay
         cheap because Summary is an in-process registry, not a file scan. -->
    <div class="diag-kpis">
      <div class="diag-kpi">
        <div class="diag-kpi-value">{{ entries.length }}</div>
        <div class="diag-kpi-label">{{ t('diag.kpi.linesShown') }}</div>
      </div>
      <div class="diag-kpi">
        <div class="diag-kpi-value">{{ summary?.droppedTotal ?? 0 }}</div>
        <div class="diag-kpi-label">{{ t('diag.kpi.dropped') }}</div>
      </div>
      <div class="diag-kpi">
        <div class="diag-kpi-value">{{ summary?.dedupTotal ?? 0 }}</div>
        <div class="diag-kpi-label">{{ t('diag.kpi.dedup') }}</div>
      </div>
      <div class="diag-kpi">
        <div class="diag-kpi-value">{{ summary?.ops.length ?? 0 }}</div>
        <div class="diag-kpi-label">{{ t('diag.kpi.ops') }}</div>
      </div>
      <div v-for="lvl in (['DEBUG', 'INFO', 'WARN', 'ERROR'] as DiagLevel[])" :key="lvl" class="diag-kpi">
        <div class="diag-kpi-value">{{ summary?.levels?.[lvl] ?? 0 }}</div>
        <div class="diag-kpi-label">{{ lvl }}</div>
      </div>
    </div>

    <!-- Filter row. Level multi-select filters by entry.level, tag is an
         exact match (diag.Query treats it as equality), text is a substring
         search over msg. Tail toggle controls auto-refresh. -->
    <div class="diag-toolbar">
      <el-select
        v-model="levels"
        multiple
        collapse-tags
        :placeholder="t('diag.filter.level')"
        size="small"
        style="width: 240px"
      >
        <el-option v-for="lvl in (['DEBUG', 'INFO', 'WARN', 'ERROR'] as DiagLevel[])" :key="lvl" :label="lvl" :value="lvl" />
      </el-select>
      <el-input
        v-model="tag"
        :placeholder="t('diag.filter.tag')"
        size="small"
        clearable
        style="width: 180px"
      />
      <el-input
        v-model="text"
        :placeholder="t('diag.filter.text')"
        size="small"
        clearable
        style="flex: 1; min-width: 160px"
      />
      <el-tooltip :content="t('diag.tail')" placement="top">
        <el-switch v-model="tail" />
      </el-tooltip>
      <el-button size="small" :loading="exporting" @click="onExport">
        <Download :size="14" /> {{ t('diag.export') }}
      </el-button>
    </div>

    <div v-if="loading" class="diag-loading">…</div>

    <div v-else-if="entries.length === 0" class="diag-empty">
      {{ t('diag.empty') }}
    </div>

    <el-table
      v-else
      :data="entries"
      :default-sort="{ prop: 'ts', order: 'descending' }"
      stripe
      size="small"
      class="diag-table"
      max-height="480"
    >
      <el-table-column prop="ts" :label="t('diag.colTs')" width="200" sortable />
      <el-table-column prop="level" :label="t('diag.colLevel')" width="90">
        <template #default="{ row }: { row: DiagEntry }">
          <span :class="`diag-level diag-level-${row.level.toLowerCase()}`">{{ row.level }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="tag" :label="t('diag.colTag')" width="220" />
      <el-table-column prop="msg" :label="t('diag.colMsg')" min-width="320" show-overflow-tooltip />
      <el-table-column prop="dedup_count" :label="t('diag.colDedup')" width="80" align="right" />
      <el-table-column :label="t('diag.colFields')" min-width="200">
        <template #default="{ row }: { row: DiagEntry }">
          <code v-if="row.fields && Object.keys(row.fields).length" class="diag-fields">{{ formatFields(row.fields) }}</code>
          <span v-else class="diag-fields-empty">—</span>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { Download } from '@lucide/vue'
import { ElMessage } from 'element-plus'
import {
  GetDiagnosticLogs,
  DiagnosticLogSummary,
  ExportDiagnosticBundle,
  SaveFileDialogFiltered,
} from '../../wailsjs/go/main/App'
import type { DiagEntry, DiagSummary, DiagLevel } from '../types/diag'
import { useI18n } from '../i18n'

const { t } = useI18n()

const levels = ref<DiagLevel[]>([])
const tag = ref('')
const text = ref('')
// Tail = poll continuously. Default off so opening the tab doesn't burn
// CPU when the user just wants to look at recent logs.
const tail = ref(false)

const entries = ref<DiagEntry[]>([])
const summary = ref<DiagSummary | null>(null)
const loading = ref(false)
const exporting = ref(false)

let pollTimer: number | undefined

async function refresh() {
  loading.value = true
  try {
    // GetDiagnosticLogs uses "" to mean "no filter" — pass "" for tag/text
    // when they're empty rather than serialising the placeholder empty-string.
    const [logs, sum] = await Promise.all([
      GetDiagnosticLogs(
        '',
        levels.value as unknown as Array<string>,
        tag.value,
        text.value,
        500,
      ) as unknown as Promise<DiagEntry[]>,
      DiagnosticLogSummary() as unknown as Promise<DiagSummary>,
    ])
    entries.value = logs || []
    summary.value = sum || null
  } catch (e: any) {
    // Swallow; the table will show its empty state and the next refresh
    // may succeed (e.g. diag not initialised yet during boot).
    entries.value = []
    summary.value = null
  } finally {
    loading.value = false
  }
}

function startTailing() {
  if (pollTimer !== undefined) return
  pollTimer = window.setInterval(refresh, 2000)
}

function stopTailing() {
  if (pollTimer !== undefined) {
    window.clearInterval(pollTimer)
    pollTimer = undefined
  }
}

watch(tail, (v) => {
  if (v) startTailing()
  else stopTailing()
})

watch([levels, tag, text], () => {
  refresh()
})

function formatFields(fields: Record<string, unknown>): string {
  try {
    return JSON.stringify(fields)
  } catch {
    return String(fields)
  }
}

async function onExport() {
  exporting.value = true
  try {
    const target = await SaveFileDialogFiltered(
      t('diag.export'),
      'Tar archive',
      '*.tar.gz',
    )
    if (!target) {
      exporting.value = false
      return
    }
    await ExportDiagnosticBundle(target)
    ElMessage.success(t('diag.exportOk', { path: target }))
  } catch (e: any) {
    ElMessage.error(t('diag.exportFail', { err: String(e?.message ?? e) }))
  } finally {
    exporting.value = false
  }
}

onMounted(() => {
  refresh()
  if (tail.value) startTailing()
})

onUnmounted(() => {
  stopTailing()
})
</script>

<style scoped>
.diag-tab {
  display: flex;
  flex-direction: column;
  gap: 14px;
  width: 100%;
}

.diag-kpis {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 8px;
}

.diag-kpi {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.diag-kpi-value {
  font-family: var(--font-mono);
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.diag-kpi-label {
  font-size: 11px;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.diag-toolbar {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

.diag-loading,
.diag-empty {
  text-align: center;
  padding: 32px 0;
  color: var(--text-muted);
  font-size: 13px;
}

.diag-table {
  font-size: 12px;
}

.diag-level {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
  font-family: var(--font-mono);
}

.diag-level-debug { background: var(--bg-overlay); color: var(--text-muted); }
.diag-level-info  { background: var(--el-color-info-light-9);   color: var(--el-color-info-dark-2); }
.diag-level-warn  { background: var(--el-color-warning-light-9); color: var(--el-color-warning-dark-2); }
.diag-level-error { background: var(--el-color-danger-light-9);  color: var(--el-color-danger-dark-2); }

.diag-fields {
  font-family: var(--font-mono);
  font-size: 11px;
  background: var(--bg-overlay);
  padding: 1px 6px;
  border-radius: 3px;
  color: var(--text-secondary);
}

.diag-fields-empty {
  color: var(--text-muted);
  font-size: 12px;
}

.section-title {
  font-size: 18px;
  font-weight: 600;
  font-family: var(--font-ui);
  margin: 0 0 4px 0;
  color: var(--text-primary);
}

.section-desc {
  font-size: 13px;
  color: var(--text-secondary);
  margin: 0 0 14px 0;
  line-height: 1.5;
}
</style>
