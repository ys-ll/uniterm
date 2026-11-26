<template>
  <div class="k8s-list-wrap">
    <div class="k8s-list-toolbar">
      <el-input
        v-model="filter"
        size="small"
        :placeholder="t('k8s.filter')"
        clearable
        class="k8s-filter"
      />

      <el-button size="small" :icon="Refresh" @click="onRefresh" />

      <el-button v-if="isNamespaceList" size="small" type="primary" @click="onNewNamespace">{{ t('k8s.newNamespace') }}</el-button>
      <el-button v-else-if="frame.kind === 'custom' || desc?.canCreate" size="small" type="primary" :icon="Plus" @click="onCreate" />

      <span class="k8s-list-title">
        {{ titleLabel }} ({{ displayCount }})
      </span>
    </div>

    <div v-if="listError" class="k8s-list-err">{{ listErrorText }}</div>

    <!-- CRD 实例列表：动态列 -->
    <el-table
      v-if="frame.kind === 'custom'"
      :data="crdFiltered"
      size="small"
      height="calc(100% - 2.5rem)"
      class="k8s-list-table"
      border
      v-loading="isLoading"
      @row-click="r => emit('open-detail', r)"
    >
      <el-table-column label="Name" show-overflow-tooltip><template #default="{ row }">{{ row.metadata?.name }}</template></el-table-column>
      <el-table-column v-for="pc in frame.crd.printerColumns" :key="pc.name" :label="pc.name" show-overflow-tooltip>
        <template #default="{ row }">{{ evalJsonPath(row, pc.jsonPath) }}</template>
      </el-table-column>
      <el-table-column label="Age"><template #default="{ row }">{{ age(row.metadata?.creationTimestamp) }}</template></el-table-column>
      <el-table-column :label="t('k8s.actions')" :width="uiPx(66)" fixed="right" class-name="k8s-action-cell">
        <template #default="{ row }">
          <button class="btn btn-ghost btn-icon btn-sm" :title="t('k8s.actionEdit')" @click.stop="emit('open-yaml', row)">
            <Pencil :size="'0.875rem'" />
          </button>
          <button class="btn btn-ghost btn-icon btn-sm danger" :title="t('k8s.actionDelete')" @click.stop="onDeleteCr(row)">
            <Trash2 :size="'0.875rem'" />
          </button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 常规资源列表 -->
    <el-table
      v-else
      :data="filtered"
      size="small"
      height="calc(100% - 2.5rem)"
      class="k8s-list-table"
      border
      v-loading="isLoading"
      :row-class-name="rowClassName"
      @row-click="onRowClick"
    >
      <el-table-column
        v-for="col in desc?.columns || []"
        :key="col.header"
        :label="col.header"
        :width="col.width ? uiPx(col.width) : undefined"
        sortable
        show-overflow-tooltip
        :sort-method="(a, b) => compareCells(col, a, b)"
        :filters="col.filterable ? enumFilters(col) : undefined"
        :filter-method="col.filterable ? (val, row) => cellText(col.value(row, usageOf(row))) === val : undefined"
      >
        <template #default="{ row }">
          <template v-if="resourceKey === 'ingresses' && col.header === 'Rules'">
            <div v-for="rule in ingressRuleLinks(row)" :key="rule.text" class="ingress-rule">
              <a v-if="rule.url" :href="rule.url" :title="rule.text" @click.stop.prevent="openIngress(rule.url)">{{ rule.text }}</a>
              <span v-else>{{ rule.text }}</span>
            </div>
            <span v-if="!ingressRuleLinks(row).length">—</span>
          </template>
          <template v-else>{{ cellText(col.value(row, usageOf(row))) }}</template>
        </template>
      </el-table-column>

      <el-table-column v-if="actionColWidth" :label="t('k8s.actions')" :width="uiPx(actionColWidth)" fixed="right" class-name="k8s-action-cell">
        <template #default="{ row }">
          <button
            v-for="a in rowActions(row)"
            :key="a"
            class="btn btn-ghost btn-icon btn-sm"
            :class="{ danger: actionMeta[a].danger }"
            :title="actionMeta[a].title(row)"
            @click.stop="actionMeta[a].run(row, $event)"
          >
            <component :is="actionMeta[a].icon(row)" :size="'0.875rem'" />
          </button>
        </template>
      </el-table-column>
    </el-table>

    <K8sCreateDialog
      v-model="createVisible"
      :title="createTitle"
      :template="createTemplate"
      :saving="createSaving"
      :error="createError"
      @confirm="onCreateConfirm"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount, h } from 'vue'
import { uiPx } from '../utils/uiScale'
import {
  ElTable, ElTableColumn, ElInput, ElButton,
  ElMessageBox, ElMessage, ElCheckbox,
} from 'element-plus'
import { Refresh, Plus } from '@element-plus/icons-vue'
import {
  Pencil, ScrollText, SquareTerminal, Box, Repeat, ArrowUpDown, Ban, CircleCheck, CirclePower, Trash2, FolderOpen } from '@lucide/vue'
import { useK8sStore } from '../stores/k8sStore'
import { useI18n } from '../i18n'
import K8sCreateDialog from './K8sCreateDialog.vue'
import { getResource, age, ingressRuleLinks, type ColoredCell } from '../services/k8sResources'
import { openExternalUrl } from '../services/webSftpBridge'
import { fetchPodMetrics, fetchNodeMetrics, type Usage } from '../services/k8sMetrics'
import { requestJSON } from '../services/k8sClient'
import { crdListPath, evalJsonPath } from '../services/k8sCrd'
import {
  podsOfOwner, podsOnNode, deleteResource, restartWorkload, scaleWorkload,
  cordonNode, drainNode, createResource, createNamespace,
} from '../services/k8sActions'
import type { NavFrame } from '../types/k8s'

const props = defineProps<{ connId: string; frame: Exclude<NavFrame, { kind: 'overview' }>; namespaceOptions: string[] }>()
const emit = defineEmits<{
  (e: 'open-detail', obj: any): void
  (e: 'open-yaml', obj: any): void
  (e: 'open-logs', pod: any): void
  (e: 'view-pods', owner: { kind: string; name: string; uid: string; namespace: string }): void
  (e: 'open-crd', crdObj: any): void
  (e: 'open-terminal', pod: any, ev?: MouseEvent): void
  (e: 'open-files', pod: any, ev?: MouseEvent): void
  (e: 'changed'): void
}>()

const store = useK8sStore()
const { t } = useI18n()

const isLoading = computed(() => {
  const f = props.frame
  if (f.kind === 'custom') return crdLoading.value
  if (f.kind === 'owned') return store.isLoading(props.connId, 'pods', f.namespace)
  return store.isLoading(props.connId, f.resourceKey, f.namespace)
})

const resourceKey = computed(() => props.frame.kind === 'custom' ? '__crd__' : props.frame.resourceKey)
const desc = computed(() => props.frame.kind === 'custom' ? undefined : getResource(props.frame.resourceKey))
const localNs = computed(() => props.frame.namespace || '')
const filter = ref(props.frame.kind === 'list' ? (props.frame.nameFilter || '') : '')

const isNamespaceList = computed(() => props.frame.kind === 'list' && props.frame.resourceKey === 'namespaces')
const isCrdList = computed(() => props.frame.kind === 'list' && props.frame.resourceKey === 'customresourcedefinitions')

// ── items（三种 frame 各自来源）─────────────────────────────────
const crdItems = ref<any[]>([])
const crdError = ref('')
const crdLoading = ref(false)

const items = computed<any[]>(() => {
  const f = props.frame
  if (f.kind === 'custom') return crdItems.value
  if (f.kind === 'owned') {
    const pods = store.getItems(props.connId, 'pods', f.namespace)
    if (f.ownerKind === 'Node') return podsOnNode(pods, f.ownerName)
    const rs = f.ownerKind === 'Deployment' ? store.getItems(props.connId, 'replicasets', f.namespace) : []
    return podsOfOwner(pods, { uid: f.ownerUid, kind: f.ownerKind }, rs)
  }
  return store.getItems(props.connId, f.resourceKey, f.namespace)
})

const listError = computed(() => {
  const f = props.frame
  if (f.kind === 'custom') return crdError.value
  if (f.kind === 'owned') return store.getError(props.connId, 'pods', f.namespace)
  return store.getError(props.connId, f.resourceKey, f.namespace)
})

// CRD 类资源（如 Gateway API）在未安装的集群上列表 404，翻译成人话
const listErrorText = computed(() =>
  listError.value && /404|not found/i.test(listError.value) ? t('k8s.crdMissing') : listError.value)

const filtered = computed(() => {
  const f = filter.value.trim().toLowerCase()
  const cols = desc.value?.columns || []
  // 参与筛选的列：标记 searchable 的列；若资源没标记任何列，退回仅按 name。
  const searchCols = cols.filter(c => c.searchable)
  const matches = (o: any) => {
    if ((o.metadata?.name || '').toLowerCase().includes(f)) return true
    return searchCols.some(c => cellText(c.value(o, usageOf(o))).toLowerCase().includes(f))
  }
  const list = f ? items.value.filter(matches) : items.value.slice()
  // 默认按名称升序（对齐 k9s）；否则 watch 增删的项会追加到 Map 末尾，
  // 删除后重建的资源就跑到列表最下面找不到了。点表头排序仍可覆盖。
  return list.sort((a, b) => (a.metadata?.name || '').localeCompare(b.metadata?.name || ''))
})
const crdFiltered = computed(() => {
  const f = filter.value.trim().toLowerCase()
  const cols = props.frame.kind === 'custom' ? props.frame.crd.printerColumns : []
  const matches = (o: any) => {
    if ((o.metadata?.name || '').toLowerCase().includes(f)) return true
    return cols.some(pc => String(evalJsonPath(o, pc.jsonPath) ?? '').toLowerCase().includes(f))
  }
  const list = f ? crdItems.value.filter(matches) : crdItems.value.slice()
  return list.sort((a, b) => (a.metadata?.name || '').localeCompare(b.metadata?.name || ''))
})

const titleLabel = computed(() =>
  props.frame.kind === 'custom' ? props.frame.crd.kind : (desc.value?.label || resourceKey.value))
const displayCount = computed(() =>
  props.frame.kind === 'custom' ? crdFiltered.value.length : filtered.value.length)

async function openIngress(url: string) {
  try { await openExternalUrl(url) }
  catch (e: any) { ElMessage.error(String(e?.message || e)) }
}

function cellText(v: string | number | ColoredCell): string {
  if (v == null) return ''
  if (typeof v === 'object' && 'text' in v) return v.text
  return String(v)
}

// 把带单位/百分比/时长的文本解析为可比数字；无法解析返回 null（退回文本比较）。
// 覆盖：内存 Ki~Pi、百分比 %、age/duration（30m/5h/3d/10s）、纯数字（含 "1/2" 取前值）。
function numericSortKey(text: string): number | null {
  const s = text.trim()
  if (s === '' || s === '—') return null
  // 百分比
  let m = s.match(/^(-?\d+(?:\.\d+)?)\s*%$/)
  if (m) return parseFloat(m[1])
  // 内存/存储二进制或十进制单位
  m = s.match(/^(\d+(?:\.\d+)?)\s*(Ki|Mi|Gi|Ti|Pi|Ei|k|M|G|T|P|E)?i?B?$/)
  if (m && m[2]) {
    const units: Record<string, number> = {
      Ki: 1024, Mi: 1024 ** 2, Gi: 1024 ** 3, Ti: 1024 ** 4, Pi: 1024 ** 5, Ei: 1024 ** 6,
      k: 1e3, M: 1e6, G: 1e9, T: 1e12, P: 1e15, E: 1e18,
    }
    return parseFloat(m[1]) * (units[m[2]] || 1)
  }
  // age/duration：3d / 5h / 30m / 10s（取最大单位，够用于排序）
  m = s.match(/^(\d+)(d|h|m|s)$/)
  if (m) {
    const mult: Record<string, number> = { s: 1, m: 60, h: 3600, d: 86400 }
    return parseInt(m[1], 10) * mult[m[2]]
  }
  // "3/5" 之类比值，按分子排序
  m = s.match(/^(\d+)\s*\/\s*\d+$/)
  if (m) return parseInt(m[1], 10)
  // 纯数字
  const n = parseFloat(s)
  if (!isNaN(n) && String(n) === s) return n
  return null
}

// el-table `sortable` needs a comparator when columns render via slot (no prop).
// 优先用列自带的 sortValue（如 cpu 原始毫核）；否则解析单位/百分比/时长做数值比较；
// 都不行再退回 localeCompare。
function compareCells(col: any, a: any, b: any): number {
  if (col.sortValue) return col.sortValue(a, usageOf(a)) - col.sortValue(b, usageOf(b))
  const ta = cellText(col.value(a, usageOf(a)))
  const tb = cellText(col.value(b, usageOf(b)))
  const na = numericSortKey(ta)
  const nb = numericSortKey(tb)
  if (na !== null && nb !== null) return na - nb
  // 一方能解析为数字、另一方不能（如 "—"）：数字排前
  if (na !== null) return -1
  if (nb !== null) return 1
  return ta.localeCompare(tb)
}

function enumFilters(col: any) {
  const set = new Set<string>()
  for (const row of items.value) set.add(cellText(col.value(row, usageOf(row))))
  return Array.from(set).filter(Boolean).sort().map(v => ({ text: v, value: v }))
}

function onRowClick(row: any) {
  // CRD list rows drill into the CRD; everything else opens its detail.
  if (isCrdList.value) emit('open-crd', row)
  else emit('open-detail', row)
}

// 整行着色：非就绪/异常项高亮，对齐 k9s。
function rowClassName({ row }: { row: any }): string {
  const tone = desc.value?.rowTone?.(row)
  return tone ? `k8s-row-${tone}` : ''
}

// ── action column ──────────────────────────────────────────────
function has(a: string) { return (desc.value?.actions || []).includes(a as any) }

// 每行的 action 列表：与资源定义一致（files 的 Running 校验在点击时做）。
function rowActions(_row: any): string[] {
  return (desc.value?.actions || []) as string[]
}

const actionMeta: Record<string, { icon: (row: any) => any; title: (row: any) => string; danger?: boolean; run: (row: any, ev?: MouseEvent) => void }> = {
  detail:   { icon: () => Pencil,          title: () => t('k8s.actionEdit'),     run: (row) => emit('open-yaml', row) },
  logs:     { icon: () => ScrollText,      title: () => t('k8s.actionLogs'),     run: (row) => emit('open-logs', row) },
  terminal: { icon: () => SquareTerminal,  title: () => t('k8s.actionTerminal'), run: (row, ev?) => emit('open-terminal', row, ev) },
  files:    {
    icon: () => FolderOpen,
    title: () => t('container.openFiles'),
    run: (row, ev?) => {
      if (row.status?.phase !== 'Running') {
        ElMessage.warning(t('container.filesRequireRunning'))
        return
      }
      emit('open-files', row, ev)
    },
  },
  viewPods: { icon: () => Box,             title: () => t('k8s.actionViewPods'), run: (row) => onViewPods(row) },
  restart:  { icon: () => Repeat,          title: () => t('k8s.actionRestart'),  run: (row) => onCommand('restart', row) },
  scale:    { icon: () => ArrowUpDown,     title: () => t('k8s.actionScale'),    run: (row) => onCommand('scale', row) },
  cordon:   {
    icon: (row) => (row.spec?.unschedulable ? CircleCheck : Ban),
    title: (row) => (row.spec?.unschedulable ? t('k8s.actionUncordon') : t('k8s.actionCordon')),
    run: (row) => onCommand('cordon', row),
  },
  drain:    { icon: () => CirclePower,     title: () => t('k8s.actionDrain'),    run: (row) => onCommand('drain', row) },
  delete:   { icon: () => Trash2,          title: () => t('k8s.actionDelete'), danger: true, run: (row) => onCommand('delete', row) },
}

const actionColWidth = computed(() => {
  const iconCount =
    (has('detail') ? 1 : 0) + (has('logs') ? 1 : 0) + (has('terminal') ? 1 : 0) +
    (has('viewPods') ? 1 : 0) + (has('restart') ? 1 : 0) + (has('scale') ? 1 : 0) +
    (has('files') ? 1 : 0) +
    (has('cordon') ? 1 : 0) + (has('drain') ? 1 : 0) + (has('delete') ? 1 : 0)
  if (!iconCount) return 0
  return 20 + iconCount * 30
})

function onViewPods(row: any) {
  if (props.frame.kind === 'custom') return
  emit('view-pods', {
    kind: desc.value!.kind, name: row.metadata?.name,
    uid: row.metadata?.uid, namespace: row.metadata?.namespace || '',
  })
}
function selfPathOf(row: any): string {
  const d = desc.value!
  const base = d.listPath(row.metadata?.namespace || '').split('?')[0]
  return `${base}/${encodeURIComponent(row.metadata?.name)}`
}
function scaleApiBase(row: any): string {
  return desc.value!.listPath(row.metadata?.namespace || '').split('?')[0]
}
function isoNow(): string { return new Date().toISOString() }

// 删除确认弹窗，内嵌 force 复选框；返回是否勾选 force。取消会 reject（'cancel'）。
async function confirmDelete(kind: string, name: string): Promise<boolean> {
  const force = ref(false)
  await ElMessageBox({
    title: t('k8s.confirmTitle'),
    type: 'warning',
    showCancelButton: true,
    confirmButtonText: t('common.confirm'),
    cancelButtonText: t('common.cancel'),
    message: () => h('div', [
      h('p', { style: 'margin: 0 0 0.5rem' }, t('k8s.deleteConfirm', { kind, name })),
      h(ElCheckbox, {
        modelValue: force.value,
        'onUpdate:modelValue': (v: any) => { force.value = !!v },
      }, () => t('k8s.deleteForce')),
    ]),
  })
  return force.value
}

async function onCommand(cmd: string, row: any) {
  try {
    if (cmd === 'delete') {
      const force = await confirmDelete(desc.value!.kind, row.metadata?.name)
      await deleteResource(props.connId, selfPathOf(row), force); ElMessage.success(t('k8s.deleted')); emit('changed')
    } else if (cmd === 'restart') {
      await ElMessageBox.confirm(t('k8s.restartConfirm', { name: row.metadata?.name }), t('k8s.confirmTitle'), { confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') })
      await restartWorkload(props.connId, desc.value!.kind, row.metadata?.namespace, row.metadata?.name, isoNow()); ElMessage.success(t('k8s.restarted'))
    } else if (cmd === 'scale') {
      const { value } = await ElMessageBox.prompt(t('k8s.scaleReplicas'), t('k8s.scaleTitle'), { inputPattern: /^\d+$/, inputValue: String(row.spec?.replicas ?? 1), confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') })
      await scaleWorkload(props.connId, scaleApiBase(row), row.metadata?.namespace, row.metadata?.name, Number(value)); ElMessage.success(t('k8s.scaled'))
    } else if (cmd === 'cordon') {
      const unschedulable = !row.spec?.unschedulable
      await ElMessageBox.confirm(t(unschedulable ? 'k8s.cordonConfirm' : 'k8s.uncordonConfirm', { name: row.metadata?.name }), t('k8s.confirmTitle'), { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') })
      await cordonNode(props.connId, row.metadata?.name, unschedulable); ElMessage.success(t('k8s.done')); emit('changed')
    } else if (cmd === 'drain') {
      await ElMessageBox.confirm(t('k8s.drainConfirm', { name: row.metadata?.name }), t('k8s.confirmTitle'), { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') })
      const r = await drainNode(props.connId, row.metadata?.name); ElMessage.success(t('k8s.drained', { evicted: r.evicted, skipped: r.skipped }) + (r.errors.length ? t('k8s.drainErrors', { count: r.errors.length }) : ''))
    }
  } catch (e: any) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(String(e?.message || e))
  }
}

// ── create ─────────────────────────────────────────────────────
async function onNewNamespace() {
  try {
    const { value } = await ElMessageBox.prompt(t('k8s.namespaceName'), t('k8s.newNamespace'), { inputPattern: /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/, confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') })
    await createNamespace(props.connId, value); ElMessage.success(t('k8s.created')); emit('changed')
  } catch (e: any) { if (e !== 'cancel' && e !== 'close') ElMessage.error(String(e?.message || e)) }
}
// ── create (SFTP-style line-numbered YAML dialog) ───────────────
const createVisible = ref(false)
const createTitle = ref('')
const createTemplate = ref('')
const createSaving = ref(false)
const createError = ref('')

function onCreate() {
  const f = props.frame
  createError.value = ''
  if (f.kind === 'custom') {
    const crd = f.crd
    const apiVersion = `${crd.group}/${crd.version}`
    const nsLine = crd.scope === 'Namespaced' ? `\n  namespace: ${f.namespace || 'default'}` : ''
    createTemplate.value = `apiVersion: ${apiVersion}\nkind: ${crd.kind}\nmetadata:\n  name: ${nsLine}\n`
    createTitle.value = t('k8s.createTitle', { kind: crd.kind })
  } else {
    const d = desc.value!
    const ns = localNs.value || 'default'
    createTemplate.value = d.createTemplate ? d.createTemplate(ns) : `apiVersion: ${d.apiVersion}\nkind: ${d.kind}\nmetadata:\n  name: \n  namespace: ${ns}\n`
    createTitle.value = t('k8s.createTitle', { kind: d.kind })
  }
  createVisible.value = true
}

async function onCreateConfirm(yaml: string) {
  const f = props.frame
  createSaving.value = true
  createError.value = ''
  try {
    let path: string
    if (f.kind === 'custom') {
      path = crdListPath(f.crd, f.namespace).split('?')[0]
    } else {
      const d = desc.value!
      path = d.createPath ? d.createPath(localNs.value) : d.listPath(localNs.value).split('?')[0]
    }
    await createResource(props.connId, path, yaml)
    createVisible.value = false
    ElMessage.success(t('k8s.created'))
    if (f.kind === 'custom') await loadCrd()
    else emit('changed')
  } catch (e: any) {
    createError.value = String(e?.message || e)
  } finally {
    createSaving.value = false
  }
}

async function onDeleteCr(row: any) {
  if (props.frame.kind !== 'custom') return
  try {
    const force = await confirmDelete(props.frame.crd.kind, row.metadata?.name)
    const base = crdListPath(props.frame.crd, row.metadata?.namespace || props.frame.namespace).split('?')[0]
    await deleteResource(props.connId, `${base}/${encodeURIComponent(row.metadata?.name)}`, force)
    ElMessage.success(t('k8s.deleted')); await loadCrd()
  } catch (e: any) { if (e !== 'cancel' && e !== 'close') ElMessage.error(String(e?.message || e)) }
}

// ── metrics poller ─────────────────────────────────────────────
const metricsMap = ref<Map<string, Usage> | null>(null)
let metricsTimer: number | null = null

async function pollMetrics() {
  if (!props.connId || !desc.value?.metrics) return
  try {
    metricsMap.value = desc.value.metrics === 'pod'
      ? await fetchPodMetrics(props.connId, localNs.value)
      : await fetchNodeMetrics(props.connId)
  } catch { metricsMap.value = null }
}
function startMetrics() {
  stopMetrics()
  if (!desc.value?.metrics) return
  pollMetrics()
  metricsTimer = window.setInterval(pollMetrics, 15000)
}
function stopMetrics() {
  if (metricsTimer != null) { clearInterval(metricsTimer); metricsTimer = null }
}

// 该行对应的实时用量，注入给列定义的 value(row, usage)。pod 按 ns/name、
// node 按 name 索引；无 metrics 资源返回 null。
function usageOf(row: any): Usage | null {
  if (!metricsMap.value || !desc.value?.metrics) return null
  const key = desc.value.metrics === 'pod'
    ? `${row.metadata?.namespace || ''}/${row.metadata?.name || ''}`
    : row.metadata?.name
  return metricsMap.value.get(key) || null
}

// ── subscription lifecycle（frame 驱动）─────────────────────────
let subs: { res: string; ns: string }[] = []

function subsFor(f: NavFrame): { res: string; ns: string }[] {
  if (f.kind === 'overview' || f.kind === 'custom') return []
  if (f.kind === 'owned') {
    const arr = [{ res: 'pods', ns: f.namespace }]
    if (f.ownerKind === 'Deployment') arr.push({ res: 'replicasets', ns: f.namespace })
    return arr
  }
  return [{ res: f.resourceKey, ns: f.namespace }]
}

async function loadCrd() {
  if (props.frame.kind !== 'custom') return
  crdError.value = ''
  crdLoading.value = true
  try {
    const { status, data, raw } = await requestJSON<any>(props.connId, 'GET', crdListPath(props.frame.crd, props.frame.namespace))
    if (status < 200 || status >= 300) { crdError.value = `HTTP ${status}: ${raw?.slice(0, 300) || ''}`; crdItems.value = []; return }
    crdItems.value = data?.items || []
  } catch (e: any) { crdError.value = String(e?.message || e); crdItems.value = [] }
  finally { crdLoading.value = false }
}

async function applySubs() {
  if (!props.connId) return
  const old = subs
  const next = subsFor(props.frame)
  subs = next
  for (const s of old) store.unsubscribe(props.connId, s.res, s.ns)
  for (const s of next) await store.subscribe(props.connId, s.res, s.ns)
  startMetrics()
  if (props.frame.kind === 'custom') { crdItems.value = []; await loadCrd() }
}

watch(() => props.frame, async () => {
  // frame 变化即视为打开新列表：按 nameFilter 预填过滤词（无则清空）
  filter.value = props.frame.kind === 'list' ? (props.frame.nameFilter || '') : ''
  if (props.connId) await applySubs()
})

watch(() => props.connId, async (v) => {
  if (v) await applySubs()
}, { immediate: true })

async function onRefresh() {
  if (!props.connId) return
  await applySubs()
}

onBeforeUnmount(() => {
  stopMetrics()
  for (const s of subs) store.unsubscribe(props.connId, s.res, s.ns)
})
</script>

<style scoped>
.ingress-rule { overflow-wrap: anywhere; white-space: normal; }
.ingress-rule a { color: var(--accent); }
.ingress-rule a:hover { text-decoration: underline; }

.k8s-list-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  height: 100%;
}
.k8s-list-toolbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.625rem;
  border-bottom: 1px solid var(--el-border-color-lighter, #333);
  flex-shrink: 0;
}
.k8s-filter {
  width: 13.75rem;
}
.k8s-list-title {
  margin-left: auto;
  color: var(--text-secondary, #888);
  font-size: 0.75rem;
}
.k8s-list-table {
  flex: 1;
  /* Flush in the detail pane: no rounded corners, and the outer frame keeps
     only the top divider — left/bottom/right lines removed */
  border-radius: 0;
}
.k8s-list-table::before,
.k8s-list-table::after {
  display: none;
}
.k8s-list-table.el-table--border {
  border-left: none;
  border-right: none;
  border-bottom: none;
}
/* 行可点击（打开详情/CRD），用手型光标提示 */
.k8s-list-table :deep(.el-table__row) {
  cursor: pointer;
}
/* 行着色：非就绪/异常项高亮（对齐 k9s）。用 el-table 的 CSS 变量覆盖 hover/条纹底色。 */
.k8s-list-table :deep(.k8s-row-warn) {
  /* 实色（与底色混合），不用半透明覆盖 */
  --el-table-tr-bg-color: color-mix(in srgb, var(--warning) 16%, var(--bg-surface));
}
.k8s-list-table :deep(.k8s-row-warn td.el-table__cell) {
  background: color-mix(in srgb, var(--warning) 16%, var(--bg-surface));
  color: var(--warning);
}
.k8s-list-table :deep(.k8s-row-err) {
  --el-table-tr-bg-color: color-mix(in srgb, var(--el-color-danger) 14%, var(--bg-surface));
}
.k8s-list-table :deep(.k8s-row-err td.el-table__cell) {
  background: color-mix(in srgb, var(--el-color-danger) 14%, var(--bg-surface));
  color: var(--el-color-danger);
}
/* Action-column cell: tighter cell padding + fixed 0.25rem gap between the
   project-standard .btn-icon buttons (1.5rem square, from style.css .btn). */
.k8s-list-table :deep(.k8s-action-cell .cell) {
  padding: 0 0.25rem;
  white-space: nowrap;
}
.k8s-list-table :deep(.k8s-action-cell .btn-icon + .btn-icon) {
  margin-left: 0.25rem;
}
.k8s-list-err {
  color: var(--el-color-danger, #f56);
  padding: 0.5rem 0.75rem;
  font-size: 0.75rem;
  border-bottom: 1px solid var(--el-border-color-lighter, #333);
}
</style>
