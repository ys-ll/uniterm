<template>
  <div class="detail-drawer-backdrop" :class="{ open: !!mode }" @click.self="$emit('close')"></div>
  <div ref="drawerEl" class="detail-drawer" :class="{ open: !!mode }" :style="mode ? { width: drawerWidth + 'px' } : undefined" @click.stop>
    <div class="drawer-resizer" @mousedown="onResizeStart"></div>
    <div class="detail-drawer-header">
      <span class="detail-drawer-title">{{ target ? fullRef(target) : '' }}</span>
      <el-button link @click="$emit('close')"><el-icon><Close :size="'1rem'" /></el-icon></el-button>
    </div>

    <div class="db-tabs">
      <button class="db-tab" :class="{ active: tab === 'layers' }" @click="tab = 'layers'">{{ t('container.layers') }}</button>
      <button class="db-tab" :class="{ active: tab === 'struct' }" @click="tab = 'struct'">{{ t('container.imageDetail') }}</button>
      <button class="db-tab" :class="{ active: tab === 'json' }" @click="tab = 'json'">JSON</button>
    </div>

    <div v-show="tab === 'layers'" class="detail-body">
      <div v-if="historyError" class="detail-section">
        <div class="detail-row"><span class="detail-value">{{ historyError }}</span></div>
      </div>
      <div v-else-if="historyLoading" class="detail-section">
        <div class="detail-row"><span class="detail-value">{{ t('container.detailLoading') }}</span></div>
      </div>
      <div v-else class="layer-list">
        <div v-for="(l, i) in layers" :key="i" class="layer-row">
          <span class="layer-index">#{{ layers.length - i }}</span>
          <span
            class="layer-cmd"
            :class="{ expanded: expandedLayers.has(i) }"
            @click.stop="toggleLayer(i)"
          >{{ l.createdBy || shortDigest(l.id) || '—' }}</span>
          <span class="layer-meta">
            <span v-if="l.size" class="layer-size">{{ l.size }}</span>
            <span v-if="l.createdAt" class="layer-time">{{ l.createdAt }}</span>
          </span>
        </div>
        <div v-if="!layers.length" class="detail-row">
          <span class="detail-value">—</span>
        </div>
      </div>
    </div>

    <div v-show="tab === 'struct'" class="detail-body">
      <div v-if="detailError" class="detail-section">
        <div class="detail-row"><span class="detail-value">{{ detailError }}</span></div>
      </div>
      <div v-else-if="detailLoading" class="detail-section">
        <div class="detail-row"><span class="detail-value">{{ t('container.detailLoading') }}</span></div>
      </div>
      <div v-for="sec in detailSections" :key="sec.label" class="detail-section">
        <div class="detail-section-title">{{ sec.label }}</div>
        <div v-for="f in sec.fields" :key="f[0]" class="detail-row">
          <span class="detail-label">{{ f[0] }}</span>
          <span class="detail-value">{{ f[1] || '—' }}</span>
        </div>
      </div>
    </div>

    <div v-show="tab === 'json'" class="json-pane">
      <div class="json-actions">
        <el-button size="small" @click="copyRaw">{{ t('container.copy') }}</el-button>
      </div>
      <pre class="json-body">{{ prettyRaw }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElButton, ElIcon, ElMessage } from 'element-plus'
import { Close } from '@element-plus/icons-vue'
import { useContainerStore } from '../stores/containerStore'
import { useI18n } from '../i18n'
import { writeClipboard } from '../composables/useClipboardWrite'
import type { ContainerImage, ImageLayer } from '../types/container'

const props = defineProps<{ mode: 'detail' | null; tabId: string; target: ContainerImage | null }>()
defineEmits<{ (e: 'close'): void }>()

const { t } = useI18n()
const store = useContainerStore()

function fullRef(img: ContainerImage) {
  return `${img.repository}:${img.tag}`
}

// 与镜像列表一致 shortId：nerdctl 的 images ID 是 manifest 摘要，inspect 的
// Id 是 config 摘要，两者值不同——详情页 ID 字段显示列表同款，避免对不上。
function shortId(id: string) {
  const raw = id.replace(/^sha256:/, '')
  return raw.length > 12 ? raw.slice(0, 12) : raw
}

// 查询用引用：优先完整 repo:tag；悬空镜像退回 ID。
// nerdctl 用短 ID 查 history/inspect 会报 multiple IDs found，故不用短 ID。
function queryRef(img: ContainerImage) {
  if (img.repository !== '<none>' && img.tag && img.tag !== '<none>') return fullRef(img)
  return img.id
}

// Drawer width (draggable). Styles mirror ContainerDetailDrawer.vue.
// Max width tracks the actual parent container (not the window) so the drawer
// never extends past the container's left edge on any screen size.
const drawerEl = ref<HTMLElement | null>(null)
const drawerWidth = ref(420)
function maxDrawerWidth() {
  const parent = drawerEl.value?.parentElement
  return parent ? Math.max(320, parent.clientWidth - 40) : window.innerWidth - 120
}
let resizeStartX = 0
let resizeStartW = 0
function onResizeMove(e: MouseEvent) {
  const dx = resizeStartX - e.clientX
  drawerWidth.value = Math.max(320, Math.min(maxDrawerWidth(), resizeStartW + dx))
}
function onResizeEnd() {
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
}
function onResizeStart(e: MouseEvent) {
  resizeStartX = e.clientX
  resizeStartW = drawerWidth.value
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
  e.preventDefault()
}

// ── detail ─────────────────────────────────────────────────────
interface DetailSection { label: string; fields: [string, string][] }

const tab = ref<'layers' | 'struct' | 'json'>('layers')
const rawInspect = ref('')
const detailError = ref('')
const detailLoading = ref(false)
let detailGen = 0

// ── layers（history）───────────────────────────────────────────
const layers = ref<ImageLayer[]>([])
const historyError = ref('')
const historyLoading = ref(false)
const expandedLayers = ref<Set<number>>(new Set())
let historyGen = 0

// 层命令默认单行省略，点击展开全部、再点击收起。
function toggleLayer(i: number) {
  const next = new Set(expandedLayers.value)
  if (next.has(i)) next.delete(i)
  else next.add(i)
  expandedLayers.value = next
}

// WSLC 的 image inspect 不返回逐层命令，层列表只有层摘要（diff ID）；
// createdBy 为空时用短摘要占位，避免整列显示「—」。
function shortDigest(id: string) {
  const raw = (id || '').replace(/^sha256:/, '')
  return raw.length > 12 ? raw.slice(0, 12) : raw
}

async function loadHistory() {
  const myGen = ++historyGen
  layers.value = []
  historyError.value = ''
  historyLoading.value = true
  try {
    const r = await store.loadImageHistory(props.tabId, queryRef(props.target!))
    if (myGen !== historyGen) return
    layers.value = r || []
  } catch (e: any) {
    if (myGen === historyGen) historyError.value = String(e?.message || e)
  } finally {
    if (myGen === historyGen) historyLoading.value = false
  }
}

async function loadDetail() {
  const myGen = ++detailGen
  rawInspect.value = ''
  detailError.value = ''
  detailLoading.value = true
  try {
    const r = await store.loadImageInspect(props.tabId, queryRef(props.target!))
    if (myGen !== detailGen) return
    rawInspect.value = r || ''
  } catch (e: any) {
    if (myGen === detailGen) detailError.value = String(e?.message || e)
  } finally {
    if (myGen === detailGen) detailLoading.value = false
  }
}

// inspect 输出是 JSON 数组，字段从首个元素提取；镜像结构与容器不同，
// 不走后端归一化，直接读原始 JSON 的常用键。
function firstObject(): any | null {
  try {
    const parsed = JSON.parse(rawInspect.value)
    return Array.isArray(parsed) ? parsed[0] : parsed
  } catch {
    return null
  }
}

function humanSize(bytes: unknown): string {
  const n = Number(bytes)
  if (!Number.isFinite(n) || n <= 0) return ''
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

function joinList(v: unknown): string {
  return Array.isArray(v) ? v.filter(Boolean).join(', ') : ''
}

function joinKeys(v: unknown): string {
  return v && typeof v === 'object' ? Object.keys(v).join(', ') : ''
}

function joinEntries(v: unknown): string {
  if (!v || typeof v !== 'object') return ''
  return Object.entries(v as Record<string, unknown>).map(([k, val]) => `${k}=${val}`).join('\n')
}

function sections(d: any): DetailSection[] {
  const entrypoint = joinList(d?.Config?.Entrypoint)
  const cmd = joinList(d?.Config?.Cmd)
  return [
    { label: t('container.secOverview'), fields: [
      [t('container.fRepoTags'), joinList(d?.RepoTags)],
      [t('container.fId'), props.target ? shortId(props.target.id) : ''],
      [t('container.fSize'), humanSize(d?.Size)],
      [t('container.fCreated'), d?.Created ? String(d.Created).replace('T', ' ').replace('Z', ' UTC') : ''],
    ]},
    { label: t('container.secImageConfig'), fields: [
      [t('container.fArch'), [d?.Architecture, d?.Os].filter(Boolean).join('/')],
      [t('container.fEntrypoint'), entrypoint],
      [t('container.fCommand'), cmd],
      [t('container.fWorkDir'), d?.Config?.WorkingDir],
      [t('container.fUser'), d?.Config?.User],
      [t('container.fExposedPorts'), joinKeys(d?.Config?.ExposedPorts)],
      [t('container.fVolumes'), joinKeys(d?.Config?.Volumes)],
    ]},
    { label: t('container.secEnv'), fields: joinList(d?.Config?.Env) ? [[t('container.secEnv'), joinList(d?.Config?.Env)]] : [] },
    { label: t('container.fLabels'), fields: joinEntries(d?.Config?.Labels) ? [[t('container.fLabels'), joinEntries(d?.Config?.Labels)]] : [] },
  ]
}

const detailSections = computed(() => {
  const d = firstObject()
  if (!d) return []
  return sections(d).filter(s => s.fields.length > 0)
})

const prettyRaw = computed(() => {
  const raw = rawInspect.value
  if (!raw) return ''
  try {
    const parsed = JSON.parse(raw)
    return JSON.stringify(Array.isArray(parsed) ? parsed[0] : parsed, null, 2)
  } catch { return raw }
})

async function copyRaw() {
  try {
    if (await writeClipboard(prettyRaw.value)) ElMessage.success(t('k8s.copied'))
    else ElMessage.error(t('k8s.copyFailed'))
  }
  catch (e: any) { ElMessage.error(`${t('k8s.copyFailed')}: ${e?.message || e}`) }
}

watch(() => [props.mode, props.target], () => {
  if (props.mode === 'detail' && props.target) {
    tab.value = 'layers'
    loadHistory()
    loadDetail()
  }
})
</script>

<style scoped>
/* Shell classes mirror ContainerDetailDrawer.vue (scoped styles don't cross components) */
.detail-drawer-backdrop {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.3s ease;
  z-index: 99;
}

.detail-drawer-backdrop.open {
  opacity: 1;
  pointer-events: auto;
}

.detail-drawer {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 26.25rem;
  max-width: 100%;
  background: var(--bg-elevated);
  border-left: 1px solid var(--border-subtle);
  transform: translateX(100%);
  transition: transform 0.3s ease;
  z-index: 100;
  display: flex;
  flex-direction: column;
}

.detail-drawer.open {
  transform: translateX(0);
}

.drawer-resizer {
  position: absolute;
  top: 0;
  left: 0;
  bottom: 0;
  width: 0.3125rem;
  cursor: col-resize;
  z-index: 101;
  background: transparent;
  transition: background 0.15s ease;
}
.drawer-resizer:hover {
  background: var(--accent, #4096ff);
}

.detail-drawer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.detail-drawer-title {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-ui);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-section {
  padding: 0 0.25rem;
  margin-bottom: 0.75rem;
}

.detail-row {
  display: flex;
  padding: 0.625rem 0;
  border-bottom: 1px solid var(--border-subtle);
  gap: 0.75rem;
}

.detail-row:last-child {
  border-bottom: none;
}

.detail-label {
  font-size: 0.75rem;
  color: var(--text-muted);
  font-family: var(--font-ui);
  flex-shrink: 0;
  width: 6.25rem;
  min-width: 6.25rem;
}

.detail-value {
  font-size: 0.8125rem;
  color: var(--text-primary);
  font-family: var(--font-mono);
  word-break: break-all;
  white-space: pre-wrap;
  flex: 1;
  user-select: text;
}

.db-tabs {
  display: flex;
  border-bottom: 1px solid var(--border-subtle);
  padding: 0 0.5rem;
  flex-shrink: 0;
}
.db-tab {
  padding: 0.375rem 1rem;
  border: none;
  background: none;
  color: var(--text-secondary);
  font-family: var(--font-ui);
  font-size: 0.8125rem;
  border-bottom: 0.125rem solid transparent;
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

.detail-body { flex: 1; overflow: auto; padding: 0.75rem 1rem; }
.detail-section-title { font-weight: 600; color: var(--text-secondary); margin: 0.5rem 0 0.25rem; }

.layer-list {
  display: flex;
  flex-direction: column;
}
.layer-row {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  padding: 0.375rem 0;
  border-bottom: 1px solid var(--border-subtle);
}
.layer-row:last-child {
  border-bottom: none;
}
.layer-index {
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--text-muted);
  flex-shrink: 0;
  width: 2rem;
  text-align: right;
}
.layer-cmd {
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--text-primary);
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: text;
}
/* 点击展开：完整显示命令内容 */
.layer-cmd.expanded {
  white-space: normal;
  word-break: break-all;
}
.layer-meta {
  flex-shrink: 0;
  display: flex;
  gap: 0.5rem;
  align-items: baseline;
}
.layer-size {
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--text-secondary);
}
.layer-time {
  font-family: var(--font-ui);
  font-size: 0.75rem;
  color: var(--text-muted);
}

.json-pane { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
.json-actions { display: flex; gap: 0.5rem; padding: 0.5rem 0.75rem; border-bottom: 1px solid var(--border-subtle); }
.json-body {
  margin: 0;
  padding: 0.75rem;
  overflow: auto;
  font-family: var(--font-mono, monospace);
  font-size: 0.75rem;
  white-space: pre-wrap;
  flex: 1;
  user-select: text;
  cursor: text;
}
</style>
