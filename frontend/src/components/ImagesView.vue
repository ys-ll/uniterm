<template>
  <div class="view-wrap">
    <div class="container-toolbar">
      <slot name="toggle" />
      <el-select
        v-if="session?.runtime === 'nerdctl'"
        :model-value="session.namespace"
        size="small"
        style="width: 8.75rem"
        filterable
        allow-create
        @change="onNamespaceChange"
      >
        <el-option v-for="ns in namespaceOptions" :key="ns" :label="ns" :value="ns" />
      </el-select>
      <el-input
        v-model="filter"
        size="small"
        :placeholder="t('k8s.filter')"
        clearable
        style="width: 12.5rem"
      />
      <div class="toolbar-spacer" />
      <el-button size="small" :icon="ArrowDownToLine" @click="openTransfer('pull')">{{ t('container.pull') }}</el-button>
      <el-button size="small" @click="loginOpen = true">{{ t('container.login') }}</el-button>
      <el-button size="small" @click="onPrune">{{ t('container.prune') }}</el-button>
      <el-button size="small" :icon="RefreshCw" @click="store.refresh(tab.id); store.loadImages(tab.id)" />
    </div>

    <div class="container-table-wrap">
      <div class="table-flex">
      <el-table
        v-loading="!session?.images.length && !!session?.imagesLoading"
        :data="filteredImages"
        size="small"
        height="100%"
        class="k8s-list-table"
        border
        @row-click="openDetail"
      >
        <el-table-column :label="t('container.colRepository')" :min-width="uiPx(200)" sortable :sort-method="(a, b) => a.repository.localeCompare(b.repository)" show-overflow-tooltip>
          <template #default="{ row }">{{ row.repository }}</template>
        </el-table-column>
        <el-table-column :label="t('container.colTag')" :min-width="uiPx(120)" sortable :sort-method="(a, b) => a.tag.localeCompare(b.tag)" show-overflow-tooltip>
          <template #default="{ row }">{{ row.tag }}</template>
        </el-table-column>
        <el-table-column :label="t('container.colId')" :width="uiPx(120)">
          <template #default="{ row }">{{ shortId(row.id) }}</template>
        </el-table-column>
        <el-table-column :label="t('container.colSize')" :width="uiPx(100)" sortable :sort-method="(a, b) => a.size.localeCompare(b.size)">
          <template #default="{ row }">{{ row.size }}</template>
        </el-table-column>
        <el-table-column :label="t('container.colCreated')" :min-width="uiPx(120)" sortable :sort-method="(a, b) => a.createdAt.localeCompare(b.createdAt)" show-overflow-tooltip>
          <template #default="{ row }">{{ row.createdAt }}</template>
        </el-table-column>
        <el-table-column :label="t('container.colRelated')" :width="uiPx(90)" align="center">
          <template #default="{ row }">
            <el-link
              v-if="relatedCount(row) > 0"
              type="primary"
              :underline="false"
              class="related-link"
              @click="$emit('view-related', imageRef(row))"
            >{{ relatedCount(row) }}</el-link>
          </template>
        </el-table-column>
        <el-table-column :label="t('container.colActions')" :width="uiPx(120)" fixed="right" class-name="k8s-action-cell">
          <template #default="{ row }">
            <button class="btn btn-ghost btn-icon btn-sm" :title="t('container.create')" @click.stop="onRunImage(row)">
              <Play :size="'0.875rem'" />
            </button>
            <button v-if="session?.runtime !== 'wslc'" class="btn btn-ghost btn-icon btn-sm" :title="t('container.tag')" @click.stop="onTag(row)">
              <Tag :size="'0.875rem'" />
            </button>
            <button class="btn btn-ghost btn-icon btn-sm" :title="t('container.push')" @click.stop="openTransfer('push', imageRef(row))">
              <ArrowUpFromLine :size="'0.875rem'" />
            </button>
            <button class="btn btn-ghost btn-icon btn-sm danger" :title="t('container.removeImage')" @click.stop="onRemoveImage(row)">
              <Trash2 :size="'0.875rem'" />
            </button>
          </template>
        </el-table-column>
      </el-table>
      </div>
      <!-- 拉取/推送任务面板：多任务并行，层级进度 + 可展开日志 + 停止/清除 -->
      <div v-if="tasks.length" class="task-panel">
        <div class="task-panel-head">
          <span class="task-panel-title">{{ t('container.pushPullTasks') }}</span>
          <button
            class="filter-icon-btn"
            :title="t('companion.clearTransfers')"
            @click="clearFinishedTasks"
          ><el-icon><BrushCleaning :size="'0.875rem'" /></el-icon></button>
        </div>
        <div class="task-list">
          <div v-for="tk in tasks" :key="tk.id" class="task-row">
            <div class="task-main" @click="expandedTaskId = expandedTaskId === tk.id ? null : tk.id">
              <span class="task-kind" :data-kind="tk.kind">
                <ArrowDownToLine v-if="tk.kind === 'pull'" :size="'0.75rem'" />
                <ArrowUpFromLine v-else :size="'0.75rem'" />
              </span>
              <span class="task-image" :title="tk.image">{{ tk.image }}</span>
              <span class="task-bar" v-if="tk.status === 'running'">
                <div class="task-bar-fill" :class="{ indeterminate: percent(tk) === null }" :style="percent(tk) !== null ? { width: percent(tk) + '%' } : undefined" />
              </span>
              <span v-else-if="tk.status === 'done'" class="task-pct">100%</span>
              <span class="task-right">
                <span class="task-status" :data-status="tk.status">{{ statusText(tk) }}</span>
                <span class="task-elapsed">{{ elapsedText(tk) }}</span>
                <span class="task-actions" @click.stop>
                  <button class="btn btn-ghost btn-icon btn-sm" :title="t('container.logs')" @click.stop="expandedTaskId = expandedTaskId === tk.id ? null : tk.id">
                    <ScrollText :size="'0.75rem'" />
                  </button>
                  <button v-if="tk.status === 'running'" class="btn btn-ghost btn-icon btn-sm" :title="t('container.stop')" @click.stop="stopTask(tk)">
                    <Square :size="'0.75rem'" />
                  </button>
                  <button v-if="tk.status !== 'running'" class="btn btn-ghost btn-icon btn-sm" :title="t('k8s.logClear')" @click.stop="clearTask(tk)">
                    <Trash2 :size="'0.75rem'" />
                  </button>
                </span>
              </span>
            </div>
            <pre v-if="expandedTaskId === tk.id" class="task-log" @contextmenu="onCopyContextMenu"><div v-for="(l, i) in tk.lines" :key="i" class="log-line">{{ l }}</div></pre>
          </div>
        </div>
      </div>
    </div>

    <ImageDetailDrawer
      :mode="drawerMode"
      :tab-id="tab.id"
      :target="drawerTarget"
      @close="drawerMode = null"
    />
    <ContainerCreateDialog v-model="createOpen" :tab-id="tab.id" :initial-image="createImage" @created="onCreated" />
    <RegistryLoginDialog v-model="loginOpen" :conn-id="session?.connId || ''" :runtime="session?.runtime || ''" :preset-registry="loginRegistry" />
    <ImageTransferDialog
      v-model="transferOpen"
      :mode="transferMode"
      :runtime="session?.runtime || ''"
      :initial-image="transferImage"
      @start="onTransferStart"
    />
    <Menu ref="copyMenuRef" v-model:visible="copyMenuVisible">
      <MenuItem @click="onCopyMenu">{{ t('container.copy') }}</MenuItem>
    </Menu>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { RefreshCw, Play, Tag, ArrowUpFromLine, ArrowDownToLine, Trash2, Square, ScrollText, BrushCleaning } from '@lucide/vue'
import { useContainerStore } from '../stores/containerStore'
import { writeClipboard } from '../composables/useClipboardWrite'
import { useI18n } from '../i18n'
import * as client from '../services/containerClient'
import type { StreamHandle } from '../services/containerClient'
import ImageDetailDrawer from './ImageDetailDrawer.vue'
import ContainerCreateDialog from './ContainerCreateDialog.vue'
import RegistryLoginDialog from './RegistryLoginDialog.vue'
import ImageTransferDialog from './ImageTransferDialog.vue'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import type { ContainerImage, ContainerInfo, ContainerTab, ContainerTransferOptions } from '../types/container'
import { uiPx } from '../utils/uiScale'

// 任务日志右键复制选中文本（与 ContainerDetailDrawer 的日志面板一致）
const copyMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const copyMenuVisible = ref(false)

function onCopyContextMenu(e: MouseEvent) {
  const sel = window.getSelection()?.toString() || ''
  if (!sel) return
  e.preventDefault()
  e.stopPropagation()
  copyMenuRef.value?.openAt(e.clientX, e.clientY)
}

async function onCopyMenu() {
  const sel = window.getSelection()?.toString() || ''
  copyMenuVisible.value = false
  if (sel) await writeClipboard(sel)
}

const props = defineProps<{ tab: ContainerTab; session: ReturnType<typeof useContainerStore>['sessions'][string] | undefined }>()
defineEmits<{ (e: 'view-related', ref: string): void }>()

const { t } = useI18n()
const store = useContainerStore()

const namespaceOptions = computed(() => {
  const s = props.session
  if (!s) return []
  const set = new Set(s.namespaces.length ? s.namespaces : [s.namespace])
  set.add('default')
  return [...set]
})

async function onNamespaceChange(ns: string) {
  try {
    await store.setNamespace(props.tab.id, ns)
    await store.loadImages(props.tab.id)
  } catch (e: any) {
    ElMessage.error(String(e?.message || e))
  }
}

// ── search / filter ────────────────────────────────────────────
const filter = ref('')
const filteredImages = computed(() => {
  const f = filter.value.trim().toLowerCase()
  const list = props.session?.images || []
  return f ? list.filter(i =>
    i.repository.toLowerCase().includes(f) ||
    i.tag.toLowerCase().includes(f) ||
    i.id.toLowerCase().includes(f)) : list
})

function shortId(id: string) {
  const raw = id.replace(/^sha256:/, '')
  return raw.length > 12 ? raw.slice(0, 12) : raw
}

function imageRef(img: ContainerImage) {
  if (img.tag && img.tag !== '<none>') return `${img.repository}:${img.tag}`
  return img.id
}

// 关联容器数：ps 输出的 image 列是 "repo:tag"，悬空镜像显示 ID。
function matchesImage(c: ContainerInfo, img: ContainerImage) {
  const ref = imageRef(img)
  if (c.image === ref) return true
  const id = img.id.replace(/^sha256:/, '')
  if (id && c.image.replace(/^sha256:/, '').startsWith(id)) return true
  return false
}

function relatedCount(img: ContainerImage) {
  return (props.session?.containers || []).filter(c => matchesImage(c, img)).length
}

// ── pull/push 传输对话框（platform/allTags 仅 pull 有意义）─────
const transferOpen = ref(false)
const transferMode = ref<'pull' | 'push'>('pull')
const transferImage = ref('')

function openTransfer(mode: 'pull' | 'push', image = '') {
  transferMode.value = mode
  transferImage.value = image
  transferOpen.value = true
}

function onTransferStart(opts: { image: string; platform: string; insecure: boolean; allTags: boolean }) {
  startTransfer(transferMode.value, opts.image, {
    platform: opts.platform,
    insecure: opts.insecure,
    allTags: opts.allTags,
  })
}

// ── image actions ──────────────────────────────────────────────
const drawerMode = ref<'detail' | null>(null)
const drawerTarget = ref<ContainerImage | null>(null)
const createOpen = ref(false)
const createImage = ref('')
const loginOpen = ref(false)
const loginRegistry = ref('')

function openDetail(img: ContainerImage) {
  drawerMode.value = 'detail'
  drawerTarget.value = img
}

function onRunImage(img: ContainerImage) {
  createImage.value = imageRef(img)
  createOpen.value = true
}

function onCreated() {
  store.refresh(props.tab.id)
}

async function onTag(img: ContainerImage) {
  try {
    const { value } = await ElMessageBox.prompt(
      t('container.tagMessage', { name: imageRef(img) }),
      t('container.tag'),
      {
        inputValue: imageRef(img),
        confirmButtonText: t('common.confirm'),
        cancelButtonText: t('common.cancel'),
        inputPattern: /\S/,
        inputErrorMessage: t('container.tagRequired'),
      },
    )
    if (!value || value.trim() === imageRef(img)) return
    await client.tagImage(props.session!.connId, imageRef(img), value.trim())
    ElMessage.success(t('container.tagDone'))
    await store.loadImages(props.tab.id)
  } catch (e: any) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(String(e?.message || e))
  }
}

async function onRemoveImage(img: ContainerImage) {
  try {
    await ElMessageBox.confirm(
      t('container.removeImageConfirm', { name: `${img.repository}:${img.tag}` }),
      t('common.confirm'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
    if (!props.session) return
    await client.removeImage(props.session.connId, img.id)
    await store.loadImages(props.tab.id)
  } catch (e: any) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(String(e?.message || e))
  }
}

async function onPrune() {
  try {
    await ElMessageBox.confirm(
      t('container.pruneConfirm'),
      t('common.confirm'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
    if (!props.session) return
    await client.imagePrune(props.session.connId)
    ElMessage.success(t('container.pruneDone'))
    await store.loadImages(props.tab.id)
  } catch (e: any) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(String(e?.message || e))
  }
}

// ── pull / push 任务面板（多任务并行，层级进度，可展开日志）─────
interface TransferTask {
  id: number
  kind: 'pull' | 'push'
  image: string
  status: 'running' | 'done' | 'failed' | 'stopped'
  lines: string[]
  layerIds: Set<string>
  doneIds: Set<string>
  layersTotal: number
  layersDone: number
  startedAt: number
  endedAt: number // 任务结束时刻（冻结计时用），0 = 进行中
  handle: StreamHandle | null
  error: string
}
const tasks = ref<TransferTask[]>([])
let taskSeq = 0
const expandedTaskId = ref<number | null>(null)
const nowTick = ref(Date.now())
let tickTimer: number | undefined

// docker/podman/nerdctl 非交互输出的层级行解析：按层 ID 聚合出层级进度
function parseLayerLine(tk: TransferTask, line: string) {
  let m = line.match(/^\s*(\S+):\s*(?:Pulling fs layer|Preparing|Pushing|Waiting)/)
  if (m) {
    tk.layerIds.add(m[1])
    tk.layersTotal = tk.layerIds.size
    return
  }
  m = line.match(/^\s*(\S+):\s*(?:Pull complete|Pushed|Layer already exists|Already exists)/)
  if (m) {
    tk.doneIds.add(m[1])
    tk.layersDone = tk.doneIds.size
  }
}

function percent(tk: TransferTask): number | null {
  if (!tk.layersTotal) return null
  return Math.min(100, Math.round((tk.layersDone / tk.layersTotal) * 100))
}

function statusText(tk: TransferTask) {
  if (tk.status === 'failed') return t('container.taskFailed')
  if (tk.status === 'stopped') return t('container.taskStopped')
  if (tk.status === 'done') return t('container.taskDone')
  return t('container.taskRunning')
}

function elapsedText(tk: TransferTask) {
  const end = tk.endedAt || nowTick.value
  const sec = Math.max(0, Math.round((end - tk.startedAt) / 1000))
  return sec >= 60 ? `${Math.floor(sec / 60)}m${sec % 60}s` : `${sec}s`
}

// push/pull 报认证类错误时引导先 Registry Login。
function isAuthError(msg: string) {
  return /unauthorized|authentication required|login|401|credentials/i.test(msg)
}

function registryHost(ref: string) {
  const i = ref.indexOf('/')
  if (i <= 0) return ''
  const first = ref.slice(0, i)
  if (first === 'localhost' || /[.:]/.test(first)) return first
  return ''
}

function promptLogin(ref: string) {
  loginRegistry.value = registryHost(ref)
  loginOpen.value = true
}

async function startTransfer(kind: 'pull' | 'push', image: string, opts: ContainerTransferOptions = {}) {
  const s = props.session
  if (!s || !image) return
  const tk: TransferTask = {
    id: ++taskSeq,
    kind,
    image,
    status: 'running',
    lines: [],
    layerIds: new Set(),
    doneIds: new Set(),
    layersTotal: 0,
    layersDone: 0,
    startedAt: Date.now(),
    endedAt: 0,
    handle: null,
    error: '',
  }
  tasks.value.push(tk)
  const onLine = (line: string) => {
    tk.lines.push(line)
    if (tk.lines.length > 500) tk.lines.splice(0, tk.lines.length - 500)
    parseLayerLine(tk, line)
  }
  const onEnd = (err: string) => {
    tk.handle = null
    if (tk.status === 'stopped') return // 用户已停止，别覆盖状态
    tk.endedAt = Date.now()
    if (err) {
      tk.status = 'failed'
      tk.error = err
      ElMessage.error(err)
      if (isAuthError(err)) promptLogin(image)
    } else {
      tk.status = 'done'
      if (kind === 'pull') store.loadImages(props.tab.id)
    }
  }
  try {
    tk.handle = kind === 'pull'
      ? await client.startPull(s.connId, image, opts, onLine, onEnd)
      : await client.startPush(s.connId, image, { insecure: opts.insecure }, onLine, onEnd)
  } catch (e: any) {
    tk.status = 'failed'
    tk.error = String(e?.message || e)
    ElMessage.error(tk.error)
  }
}

function stopTask(tk: TransferTask) {
  tk.endedAt = Date.now()
  tk.status = 'stopped'
  tk.handle?.stop()
  tk.handle = null
}

function clearTask(tk: TransferTask) {
  if (tk.status === 'running') return
  tasks.value = tasks.value.filter(x => x.id !== tk.id)
  if (expandedTaskId.value === tk.id) expandedTaskId.value = null
}

function clearFinishedTasks() {
  tasks.value = tasks.value.filter(x => x.status === 'running')
  expandedTaskId.value = null
}

// 首次进入镜像视图时拉取列表；store.open 只加载容器列表。
onMounted(() => {
  store.loadImages(props.tab.id)
  // 任务面板的已用时间刷新
  tickTimer = window.setInterval(() => { nowTick.value = Date.now() }, 1000)
})

onBeforeUnmount(() => {
  if (tickTimer) window.clearInterval(tickTimer)
  tasks.value.forEach(t => t.handle?.stop())
})
</script>

<style scoped>
.view-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.container-toolbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.625rem;
  border-bottom: 1px solid var(--el-border-color-lighter, #333);
  flex-shrink: 0;
}

.toolbar-spacer {
  flex: 1;
}

.container-table-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

/* 表格自适应剩余空间；任务面板在 flex 流内固定在表格之下 */
.table-flex {
  flex: 1;
  min-height: 0;
}

.k8s-list-table {
  flex: 1;
}

/* Action-column cell: tighter padding + 0.25rem gap between .btn-icon buttons
   (mirrors K8sResourceList's action-cell styling). */
.k8s-list-table :deep(.k8s-action-cell .cell) {
  padding: 0 0.25rem;
  white-space: nowrap;
  display: flex;
  align-items: center;
}
.k8s-list-table :deep(.k8s-action-cell .btn-icon + .btn-icon) {
  margin-left: 0.25rem;
}

.k8s-list-table :deep(.el-table__row) {
  cursor: pointer;
}

.related-link {
  font-size: 0.8125rem;
}

/* 拉取/推送任务面板 */
.task-panel {
  margin: 0 0 0.5rem;
  border: 1px solid var(--el-border-color-lighter, #333);
  border-radius: var(--radius-sm);
  background: var(--bg-surface);
  max-height: 11rem;
  overflow: auto;
  flex-shrink: 0;
}

.task-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.25rem 0.625rem;
  border-bottom: 1px solid var(--border-subtle);
}

/* 与 TransferPanel 的清除按钮同款（scoped 样式不跨组件，复制过来） */
.filter-icon-btn {
  font-size: 0.875rem;
  width: 1.625rem;
  height: 1.625rem;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-secondary);
  flex-shrink: 0;
}
.filter-icon-btn:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}
.filter-icon-btn:disabled {
  opacity: 0.4;
}

.task-panel-title {
  font-family: var(--font-ui);
  font-size: 0.6875rem;
  font-weight: 600;
  color: var(--text-secondary);
}

.task-row + .task-row {
  border-top: 1px solid var(--border-subtle);
}

.task-main {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.625rem;
  min-width: 0;
  white-space: nowrap;
}

.task-kind {
  flex-shrink: 0;
  display: flex;
  color: var(--text-muted);
}
.task-kind[data-kind='pull'] {
  color: var(--el-color-success);
}
.task-kind[data-kind='push'] {
  color: var(--el-color-primary);
}

.task-image {
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex-shrink: 1;
}

.task-bar {
  flex: 1;
  min-width: 4rem;
  height: 0.375rem;
  border-radius: var(--radius-sm);
  background: var(--bg-hover);
  overflow: hidden;
}
.task-bar-fill {
  height: 100%;
  background: var(--accent);
  border-radius: var(--radius-sm);
  transition: width 0.3s ease;
}
.task-bar-fill.indeterminate {
  width: 40%;
  animation: task-indeterminate 1.2s ease-in-out infinite alternate;
}
@keyframes task-indeterminate {
  from { margin-left: 0; }
  to { margin-left: 60%; }
}

.task-pct {
  flex: 1;
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  color: var(--text-muted);
}

.task-right {
  flex-shrink: 0;
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.task-status {
  flex-shrink: 0;
  font-size: 0.6875rem;
  color: var(--text-secondary);
}
.task-status[data-status='done'] {
  color: var(--el-color-success);
}
.task-status[data-status='failed'] {
  color: var(--el-color-danger);
}
.task-status[data-status='stopped'] {
  color: var(--text-muted);
}

.task-elapsed {
  flex-shrink: 0;
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  color: var(--text-muted);
}

.task-actions {
  flex-shrink: 0;
  display: flex;
  gap: 0.125rem;
}

.task-log {
  margin: 0;
  padding: 0.5rem 0.75rem;
  border-top: 1px solid var(--border-subtle);
  font-family: var(--font-mono, monospace);
  font-size: 0.6875rem;
  user-select: text;
  background: var(--bg-surface);
}

.task-log .log-line {
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
