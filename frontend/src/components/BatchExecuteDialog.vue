<template>
  <el-dialog
    v-model="visible"
    :title="t('batch.title')"
    width="900px"
    :close-on-click-modal="false"
    :before-close="onBeforeClose"
    append-to-body
    class="batch-dialog"
  >
    <!-- 顶部：选中主机列表 -->
    <div class="section">
      <div class="section-title">{{ t('batch.selectedHosts', { count: editableHosts.length }) }}</div>
      <div class="host-chips">
        <el-tag
          v-for="h in editableHosts"
          :key="h.id"
          closable
          :disable-transitions="false"
          @close="removeHost(h.id)"
          class="host-chip"
        >
          {{ h.name }}
          <span class="host-chip-sub">{{ h.host }}:{{ h.port }}</span>
        </el-tag>
        <span v-if="editableHosts.length === 0" class="empty-hint">{{ t('batch.noHosts') }}</span>
      </div>
      <div v-if="skippedCount > 0" class="skip-hint">
        {{ t('batch.onlySshSupported', { count: skippedCount }) }}
      </div>
    </div>

    <!-- 中部：命令输入 -->
    <div class="section">
      <div class="section-title">{{ t('batch.command') }}</div>
      <el-input
        v-model="command"
        type="textarea"
        :rows="5"
        :placeholder="t('batch.commandPlaceholder')"
        :disabled="batchStore.isExecuting"
        class="command-input"
      />
      <div class="command-actions">
        <el-button :disabled="batchStore.isExecuting" @click="onSelectScript">
          {{ t('batch.selectScript') }}
        </el-button>
      </div>
    </div>

    <!-- 配置区 -->
    <div class="section">
      <div class="section-title">{{ t('batch.config') }}</div>
      <div class="config-row">
        <div class="config-item">
          <span class="config-label">{{ t('batch.concurrency') }}</span>
          <el-input-number v-model="concurrency" :min="1" :max="50" :disabled="batchStore.isExecuting" />
        </div>
        <div class="config-item">
          <span class="config-label">{{ t('batch.timeout') }}</span>
          <el-input-number v-model="timeout" :min="0" :max="3600" :disabled="batchStore.isExecuting" />
        </div>
        <div class="config-item">
          <span class="config-label">{{ t('batch.onFailure') }}</span>
          <el-select v-model="onFailure" :disabled="batchStore.isExecuting" style="width: 120px">
            <el-option :label="t('batch.continue')" value="continue" />
            <el-option :label="t('batch.stop')" value="stop" />
          </el-select>
        </div>
      </div>
    </div>

    <!-- 进度 -->
    <div v-if="batchStore.isExecuting || batchStore.sessions.length > 0" class="section">
      <div class="section-title">
        {{ t('batch.results') }}
        <span v-if="batchStore.isExecuting" class="progress-text">
          {{ t('batch.progress', { done: batchStore.completedCount(), total: batchStore.sessions.length }) }}
        </span>
        <span v-else-if="batchCompleted" class="progress-text">
          {{ t('batch.completed', { success: batchStore.countByStatus('success'), failed: batchStore.countByStatus('failed') }) }}
        </span>
      </div>
      <el-table :data="batchStore.sessions" border stripe size="small" class="result-table">
        <el-table-column :label="t('batch.host')" prop="hostName" width="160">
          <template #default="{ row }">
            <div class="host-cell">
              <span class="host-name">{{ row.hostName }}</span>
              <span class="host-sub">{{ row.hostSubtitle }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="t('batch.status')" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">{{ statusLabel(row) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('batch.duration')" width="100" align="right">
          <template #default="{ row }">
            <span v-if="row.startTime && row.endTime">{{ formatDuration(row.endTime - row.startTime) }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('batch.output')" min-width="200">
          <template #default="{ row }">
            <div class="output-cell">
              <span class="output-preview">{{ row.preview || t('batch.noOutput') }}</span>
              <el-button
                v-if="row.output || row.error"
                link
                size="small"
                @click="showFullOutput(row)"
                class="view-full-btn"
              >{{ t('batch.viewFullOutput') }}</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="batchStore.sessions.length > 0 && !batchStore.isExecuting" class="export-actions">
        <el-button size="small" @click="onExportCsv">{{ t('batch.exportCsv') }}</el-button>
        <el-button size="small" @click="onExportJson">{{ t('batch.exportJson') }}</el-button>
      </div>
    </div>

    <!-- 底部按钮 -->
    <template #footer>
      <div class="dialog-footer">
        <el-button v-if="batchStore.isExecuting" type="warning" @click="onCancel">{{ t('batch.cancel') }}</el-button>
        <el-button v-else @click="visible = false">{{ t('batch.close') }}</el-button>
        <el-button
          type="primary"
          :disabled="!canExecute"
          :loading="batchStore.isExecuting"
          @click="onExecute"
        >{{ t('batch.execute') }}</el-button>
      </div>
    </template>
  </el-dialog>

  <!-- 完整输出对话框 -->
  <el-dialog
    v-model="fullOutputVisible"
    :title="t('batch.fullOutput', { host: fullOutputHost })"
    width="700px"
    append-to-body
    class="full-output-dialog"
  >
    <div v-if="fullOutputTruncated" class="truncated-hint">{{ t('batch.outputTruncated') }}</div>
    <pre ref="fullOutputPreRef" class="full-output-pre" @contextmenu="onCopyContextMenu">{{ fullOutputText }}</pre>
    <template #footer>
      <el-button @click="fullOutputVisible = false">{{ t('batch.close') }}</el-button>
    </template>
  </el-dialog>

  <!-- Right-click copy menu for the log output (same pattern as the
       container/K8s log panels) -->
  <Menu ref="copyMenuRef" v-model:visible="copyMenuVisible">
    <MenuItem @click="onCopyMenu">{{ t('container.copy') }}</MenuItem>
    <MenuItem @click="onSelectAllMenu">{{ t('batch.selectAll') }}</MenuItem>
  </Menu>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { Clipboard } from '@wailsio/runtime'
import { useI18n } from '../i18n'
import { useBatchStore, stripAnsi, type BatchSession, type BatchSessionStatus } from '../stores/batchStore'
import { assessLocalRisk } from '../services/localRisk'
import { executeBatch, exportCsv, exportJson } from '../services/batchExecute'
import { OpenFileDialogFiltered, ReadFileBase64, SaveFileDialogFiltered, WriteFileBase64 } from '../../bindings/github.com/ys-ll/uniterm/app'
import { isWebServerMode, pickBrowserFiles, readBrowserFileAsText, saveFileInBrowser } from '../services/webSftpBridge'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import type { ConnectionConfig } from '../types/session'

const { t } = useI18n()
const batchStore = useBatchStore()

// 任务日志右键复制选中文本（与 ContainerDetailDrawer 的日志面板一致）
const copyMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const copyMenuVisible = ref(false)
const fullOutputPreRef = ref<HTMLPreElement | null>(null)

// Always open the menu on the log box — 全选 needs it even with no selection
function onCopyContextMenu(e: MouseEvent) {
  e.preventDefault()
  e.stopPropagation()
  copyMenuRef.value?.openAt(e.clientX, e.clientY)
}

async function onCopyMenu() {
  const sel = window.getSelection()?.toString() || ''
  if (sel) {
    try { await Clipboard.SetText(sel) } catch { try { await navigator.clipboard.writeText(sel) } catch { /* ignore */ } }
  }
  copyMenuVisible.value = false
}

function onSelectAllMenu() {
  const el = fullOutputPreRef.value
  const sel = window.getSelection()
  if (el && sel) {
    const range = document.createRange()
    range.selectNodeContents(el)
    sel.removeAllRanges()
    sel.addRange(range)
  }
  copyMenuVisible.value = false
}

const props = defineProps<{
  modelValue: boolean
  hosts: ConnectionConfig[]
}>()

const emit = defineEmits<{
  'update:modelValue': [v: boolean]
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

// 可编辑的主机列表（用户可在 dialog 中移除）
const editableHosts = ref<ConnectionConfig[]>([])
// 被跳过的非 SSH/local 主机数
const skippedCount = ref(0)

const command = ref('')
const concurrency = ref(5)
const timeout = ref(30)
const onFailure = ref<'continue' | 'stop'>('continue')

// 完整输出对话框
const fullOutputVisible = ref(false)
const fullOutputHost = ref('')
const fullOutputText = ref('')
const fullOutputTruncated = ref(false)

// 批量是否已完成（用于显示汇总）
const batchCompleted = ref(false)

// 当 dialog 打开或 hosts 变化时，初始化 editableHosts
watch(
  () => [props.modelValue, props.hosts],
  ([open, hs]) => {
    if (open) {
      const hosts = hs as ConnectionConfig[]
      // 只保留 ssh 和 local 类型
      const supported = hosts.filter(h => h.type === 'ssh' || h.type === 'local')
      skippedCount.value = hosts.length - supported.length
      editableHosts.value = supported.map(h => ({ ...h }))
      // 重置完成标志
      batchCompleted.value = false
      // 如果 store 中有旧结果且未在执行，清空
      if (!batchStore.isExecuting) {
        batchStore.clearResults()
      }
    }
  },
  { immediate: true },
)

function removeHost(id: string) {
  editableHosts.value = editableHosts.value.filter(h => h.id !== id)
}

const canExecute = computed(() => {
  return editableHosts.value.length > 0 && command.value.trim().length > 0 && !batchStore.isExecuting
})

// ── 危险命令确认 ──
async function confirmIfRisky(cmd: string): Promise<boolean> {
  const level = assessLocalRisk(cmd)
  if (level === 'read') return true
  try {
    await ElMessageBox.confirm(
      t('batch.riskConfirm', { level }),
      t('batch.riskTitle'),
      {
        confirmButtonText: t('batch.execute'),
        cancelButtonText: t('batch.cancel'),
        type: 'warning',
      },
    )
    return true
  } catch {
    return false
  }
}

// ── 执行 ──
async function onExecute() {
  const cmd = command.value.trim()
  if (!cmd) {
    ElMessage.warning(t('batch.noCommand'))
    return
  }
  if (editableHosts.value.length === 0) {
    ElMessage.warning(t('batch.noHosts'))
    return
  }

  // 危险命令确认
  const confirmed = await confirmIfRisky(cmd)
  if (!confirmed) return

  batchCompleted.value = false

  await executeBatch({
    hosts: editableHosts.value,
    command: cmd,
    config: {
      concurrency: concurrency.value,
      timeout: timeout.value,
      onFailure: onFailure.value,
    },
  })

  batchCompleted.value = true
  const success = batchStore.countByStatus('success')
  const failed = batchStore.countByStatus('failed')
  ElMessage.success(t('batch.completed', { success, failed }))
}

// ── 取消 ──
async function onCancel() {
  try {
    await ElMessageBox.confirm(t('batch.confirmCancel'), t('batch.cancel'), {
      confirmButtonText: t('batch.cancel'),
      cancelButtonText: t('batch.close'),
      type: 'warning',
    })
  } catch {
    return
  }
  batchStore.requestCancel()
}

// ── 关闭前确认 ──
async function onBeforeClose(done: () => void) {
  if (!batchStore.isExecuting) {
    done()
    return
  }
  try {
    await ElMessageBox.confirm(t('batch.confirmClose'), t('batch.close'), {
      confirmButtonText: t('batch.close'),
      cancelButtonText: t('batch.cancel'),
      type: 'warning',
    })
    batchStore.requestCancel()
    done()
  } catch {
    // 用户选择不关闭
  }
}

// ── 选择脚本文件 ──
async function onSelectScript() {
  try {
    // Web mode: read the script content in the browser, no Go-side file IO.
    if (await isWebServerMode()) {
      const [file] = await pickBrowserFiles({ accept: '.sh,.bash,.py,.rb,.pl,.txt' })
      if (!file) return
      command.value = await readBrowserFileAsText(file)
      return
    }
    const path = await OpenFileDialogFiltered(t('batch.selectScript'), 'Shell Script', '*.sh;*.bash;*.py;*.rb;*.pl;*.txt')
    if (!path) return
    const b64 = await ReadFileBase64(path)
    const text = decodeBase64(b64)
    command.value = text
  } catch (e: any) {
    if (!isDialogCancel(e)) {
      ElMessage.error(e?.message || String(e))
    }
  }
}

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

function encodeBase64(text: string): string {
  const bytes = new TextEncoder().encode(text)
  let bin = ''
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i])
  return btoa(bin)
}

// ── 状态标签 ──
function statusTagType(status: BatchSessionStatus): 'info' | 'warning' | 'success' | 'danger' {
  switch (status) {
    case 'pending': return 'info'
    case 'running': return 'warning'
    case 'success': return 'success'
    case 'failed': return 'danger'
  }
}

// Status text: failed rows are split into timeout / command exit /
// connection / cancelled / skipped via failKind.
function statusLabel(row: BatchSession): string {
  if (row.status === 'failed') {
    switch (row.failKind) {
      case 'timeout': return t('batch.statusTimeout')
      case 'exit': return t('batch.statusCmdFailed')
      case 'connection': return t('batch.statusConnFailed')
      case 'cancelled': return t('batch.statusCancelled')
      case 'skipped': return t('batch.statusSkipped')
      default: return t('batch.statusFailed')
    }
  }
  switch (row.status) {
    case 'pending': return t('batch.statusPending')
    case 'running': return t('batch.statusRunning')
    case 'success': return t('batch.statusSuccess')
  }
}

// ── 耗时格式化 ──
function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  const m = Math.floor(ms / 60000)
  const s = Math.round((ms % 60000) / 1000)
  return `${m}m${s}s`
}

// ── 显示完整输出 ──
function showFullOutput(row: BatchSession) {
  fullOutputHost.value = row.hostName
  fullOutputTruncated.value = !!row.truncated
  // Strip once per open; output is tail-capped in the store so this stays
  // cheap even for runaway commands. The error goes AFTER the log so the
  // failure reason reads as the tail of the transcript.
  const stripped = stripAnsi(row.output)
  fullOutputText.value = row.error ? `${stripped}\n\n[${row.error}]` : stripped
  fullOutputVisible.value = true
}

// ── 导出 ──
async function onExportCsv() {
  const csv = exportCsv(
    batchStore.sessions.map(s => ({
      hostName: s.hostName,
      hostSubtitle: s.hostSubtitle,
      status: s.status,
      duration: s.startTime && s.endTime ? s.endTime - s.startTime : undefined,
      output: s.output,
      error: s.error,
    })),
  )
  await saveFile(csv, 'batch-results.csv')
}

async function onExportJson() {
  const json = exportJson(
    batchStore.sessions.map(s => ({
      hostName: s.hostName,
      hostSubtitle: s.hostSubtitle,
      status: s.status,
      startTime: s.startTime,
      endTime: s.endTime,
      output: s.output,
      error: s.error,
      truncated: s.truncated,
    })),
  )
  await saveFile(json, 'batch-results.json')
}

async function saveFile(content: string, defaultName: string) {
  try {
    // Web-server mode: no native save dialog — hand the text to the
    // browser's download manager instead.
    if (await isWebServerMode()) {
      saveFileInBrowser(defaultName, defaultName.endsWith('.json') ? 'application/json' : 'text/csv', content)
      return
    }
    const path = await SaveFileDialogFiltered(t('batch.export'), defaultName, 'File', '*')
    if (!path) return
    await WriteFileBase64(path, encodeBase64(content))
    ElMessage.success(t('batch.export'))
  } catch (e: any) {
    if (!isDialogCancel(e)) {
      ElMessage.error(e?.message || String(e))
    }
  }
}
</script>

<style scoped>
.batch-dialog :deep(.el-dialog__body) {
  max-height: 70vh;
  overflow-y: auto;
}

.section {
  margin-bottom: 16px;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary, #909399);
  margin-bottom: 8px;
  display: flex;
  align-items: center;
  gap: 12px;
}

.progress-text {
  font-size: 12px;
  font-weight: 400;
  color: var(--text-muted, #c0c4cc);
}

.host-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 8px;
  background: var(--bg-overlay);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  min-height: 40px;
  align-items: center;
}

.host-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.host-chip-sub {
  font-size: 10px;
  opacity: 0.6;
  margin-left: 4px;
}

.empty-hint {
  color: var(--text-disabled, #c0c4cc);
  font-size: 12px;
}

.skip-hint {
  margin-top: 6px;
  font-size: 11px;
  color: var(--warning, #e6a23c);
}

.command-input :deep(textarea) {
  font-family: var(--font-mono, 'Consolas', 'Courier New', monospace);
  font-size: 12px;
}

.command-actions {
  margin-top: 6px;
}

.config-row {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
}

.config-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.config-label {
  font-size: 11px;
  color: var(--text-muted, #909399);
}

.result-table {
  margin-top: 4px;
}

.host-cell {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.host-name {
  font-size: 12px;
  font-weight: 500;
}

.host-sub {
  font-size: 10px;
  color: var(--text-muted, #c0c4cc);
}

.output-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.output-preview {
  flex: 1;
  font-family: var(--font-mono, 'Consolas', monospace);
  font-size: 11px;
  color: var(--text-secondary, #606266);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}

.view-full-btn {
  flex-shrink: 0;
}

.export-actions {
  margin-top: 8px;
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.truncated-hint {
  margin-bottom: 8px;
  font-size: 12px;
  color: var(--warning);
}

.full-output-pre {
  max-height: 60vh;
  overflow: auto;
  font-family: var(--font-mono, 'Consolas', monospace);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  background: var(--bg-overlay);
  border: 1px solid var(--border-subtle);
  padding: 12px;
  border-radius: 4px;
  margin: 0;
  /* Opt back into text selection: the app root disables it globally. */
  user-select: text;
  -webkit-user-select: text;
  cursor: text;
}
</style>