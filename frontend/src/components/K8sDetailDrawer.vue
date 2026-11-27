<template>
  <div class="detail-drawer-backdrop" :class="{ open: !!mode }" @click="$emit('close')"></div>
  <div ref="drawerEl" class="detail-drawer" :class="{ open: !!mode }" :style="mode ? { width: drawerWidth + 'px' } : undefined">
    <div class="drawer-resizer" @mousedown="onResizeStart"></div>
    <div class="detail-drawer-header">
      <span class="detail-drawer-title">{{ headerTitle }}</span>
      <el-button link @click="$emit('close')"><el-icon><Close :size="'1rem'" /></el-icon></el-button>
    </div>

    <template v-if="mode === 'detail'">
      <div class="db-tabs">
        <button class="db-tab" :class="{ active: tab === 'struct' }" @click="tab = 'struct'">{{ t('k8s.detail') }}</button>
        <button class="db-tab" :class="{ active: tab === 'yaml' }" :disabled="keyEditVisible" @click="tab = 'yaml'">YAML</button>
      </div>

      <div v-show="tab === 'struct'" class="detail-body" @contextmenu="onCopyContextMenu">
        <div v-for="sec in sections" :key="sec.label" class="detail-section">
          <div class="detail-section-title">{{ sec.label }}</div>
          <div v-for="f in sec.fields" :key="f.label" class="detail-row">
            <span class="detail-label">{{ f.label }}</span>
            <span class="detail-value">{{ fieldText(f) }}</span>
          </div>
        </div>
        <div v-if="resourceKey === 'configmaps'" :key="target?.metadata?.uid || headerTitle" class="detail-section">
          <div class="detail-section-title">Data ({{ configEntries.length }})</div>
          <template v-for="entry in configEntries" :key="entry.key">
            <div v-if="!entry.multiline" class="detail-row">
              <span class="detail-label config-key">{{ entry.key }}</span>
              <div v-if="isEditingKey(entry.key)" class="detail-value config-inline-edit">
                <el-input v-model="keyEditDraft" type="textarea" :autosize="{ minRows: 1, maxRows: 5 }" :disabled="keySaving" />
                <div class="config-edit-actions">
                  <el-button size="small" :disabled="keySaving" @click="cancelKeyEdit">{{ t('common.cancel') }}</el-button>
                  <el-button size="small" type="primary" :loading="keySaving" @click="saveKeyEdit">{{ t('common.save') }}</el-button>
                </div>
                <div v-if="keySaveError" class="yaml-error">{{ keySaveError }}</div>
              </div>
              <template v-else>
                <span class="detail-value">{{ entry.value === '' ? '—' : entry.value }}</span>
                <el-button v-if="!target?.immutable" link size="small" :disabled="keyEditVisible || editing || saving" @click.stop="startKeyEdit(entry)">{{ t('k8s.edit') }}</el-button>
              </template>
            </div>
            <details v-else class="config-entry" :open="expandedConfig.has(entry.key)" @toggle="onConfigToggle(entry.key, $event)">
              <summary class="config-key config-entry-header" @click="isEditingKey(entry.key) && $event.preventDefault()">
                <span class="config-entry-name">{{ entry.key }}</span>
                <span class="config-header-actions">
                  <span v-if="isEditingKey(entry.key) && keyEditLang === 'conf'" class="config-edit-encoding">{{ keyEditRaw ? 'Raw' : 'Unicode' }}</span>
                  <el-button v-if="expandedConfig.has(entry.key) && !isEditingKey(entry.key) && entry.lang === 'conf' && decodePropertiesUnicode(entry.value) !== entry.value"
                    link size="small" @click.stop.prevent="toggleRawProperties(entry.key)">{{ rawProperties.has(entry.key) ? 'Unicode' : 'Raw' }}</el-button>
                  <el-button v-if="!target?.immutable && !isEditingKey(entry.key)" link size="small"
                    :disabled="keyEditVisible || editing || saving" @click.stop.prevent="startKeyEdit(entry)">{{ t('k8s.edit') }}</el-button>
                </span>
              </summary>
              <div v-if="expandedConfig.has(entry.key)">
                <template v-if="isEditingKey(entry.key)">
                  <div class="config-preview">
                    <SyntaxEditor :ref="setKeyEditor" v-model="keyEditDraft" :file-path="keyEditName" :lang="keyEditLang"
                      compact show-search-button :readonly="keySaving" />
                  </div>
                  <div class="config-edit-actions">
                  <el-button size="small" :disabled="keySaving" @click="cancelKeyEdit">{{ t('common.cancel') }}</el-button>
                  <el-button size="small" type="primary" :loading="keySaving" @click="saveKeyEdit">{{ t('common.save') }}</el-button>
                </div>
                <div v-if="keySaveError" class="yaml-error">{{ keySaveError }}</div>
                </template>
                <template v-else>
                  <div class="config-preview">
                    <SyntaxEditor :model-value="entry.lang === 'conf' && !rawProperties.has(entry.key) ? decodePropertiesUnicode(entry.value) : entry.value"
                      :file-path="entry.key" :lang="entry.lang" readonly compact show-search-button />
                  </div>
                </template>
              </div>
            </details>
          </template>
          <div v-if="!configEntries.length" class="detail-row"><span class="detail-value">—</span></div>
          <template v-if="Object.keys(target?.binaryData || {}).length">
            <div class="detail-section-title">binaryData (Base64)</div>
            <details v-for="(value, key) in target.binaryData" :key="key" class="config-entry">
              <summary class="config-key">{{ key }}</summary>
              <pre class="config-binary">{{ value }}</pre>
            </details>
          </template>
        </div>
        <!-- Pod events (async) -->
        <div v-if="isPod" class="detail-section">
          <div class="detail-section-title">Events</div>
          <div v-if="eventsError" class="detail-row"><span class="detail-value">{{ eventsError }}</span></div>
          <div v-else-if="!events.length" class="detail-row"><span class="detail-value">—</span></div>
          <div v-else v-for="(ev, i) in events" :key="i" class="detail-row">
            <span class="detail-label">{{ ev.lastTimestamp || ev.eventTime || '' }}</span>
            <span class="detail-value">{{ ev.type }} {{ ev.reason }}: {{ ev.message }}</span>
          </div>
        </div>
      </div>

      <div v-show="tab === 'yaml'" class="yaml-pane">
        <div class="yaml-actions">
          <template v-if="!editing">
            <el-button size="small" @click="startEdit">{{ t('k8s.edit') }}</el-button>
            <el-button size="small" @click="copyYaml">{{ t('k8s.copy') }}</el-button>
          </template>
          <template v-else>
            <el-button size="small" type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
            <el-button size="small" @click="cancelEdit">{{ t('common.cancel') }}</el-button>
          </template>
        </div>
        <SyntaxEditor v-if="!editing && tab === 'yaml'" :model-value="yamlText" lang="yaml" readonly compact />
        <SyntaxEditor v-else-if="editing" v-model="draft" lang="yaml" compact />
        <div v-if="saveError" class="yaml-error">{{ saveError }}</div>
      </div>
    </template>

    <div v-else-if="mode === 'logs'" class="logs-pane">
      <div class="logs-toolbar">
        <el-select v-model="logContainer" size="small" style="width: 10.0rem" @change="restartLogs">
          <el-option v-for="c in containerNames" :key="c" :label="c" :value="c" />
        </el-select>
        <el-select v-model="logTail" size="small" style="width: 5.625rem" @change="restartLogs">
          <el-option :value="100" label="100" />
          <el-option :value="500" label="500" />
          <el-option :value="2000" label="2000" />
        </el-select>
        <el-checkbox v-model="logPrevious" border size="small" @change="restartLogs">{{ t('k8s.logPrevious') }}</el-checkbox>
        <el-checkbox v-model="logTimestamps" border size="small">{{ t('k8s.logTimestamps') }}</el-checkbox>
        <el-checkbox v-model="logWrap" border size="small">{{ t('k8s.logWrap') }}</el-checkbox>
        <el-button size="small" @click="logLines = []">{{ t('k8s.logClear') }}</el-button>
        <el-button size="small" @click="logPaused = !logPaused">{{ logPaused ? t('k8s.logResume') : t('k8s.logPause') }}</el-button>
      </div>
      <div
        ref="logBody"
        class="k8s-yaml-drawer-body logs-body"
        :class="{ 'logs-hide-ts': !logTimestamps, 'logs-nowrap': !logWrap }"
        @contextmenu="onCopyContextMenu"
      >
        <div v-for="(l, i) in logLines" :key="i" class="log-line"><span class="log-ts">{{ l.ts }}</span>{{ l.msg }}</div>
      </div>
    </div>

    <Menu ref="copyMenuRef" v-model:visible="copyMenuVisible">
      <MenuItem @click="onCopyMenu">{{ t('k8s.copy') }}</MenuItem>
    </Menu>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick, onBeforeUnmount } from 'vue'
import { ElButton, ElIcon, ElMessage, ElSelect, ElOption, ElCheckbox, ElInput } from 'element-plus'
import { Close } from '@element-plus/icons-vue'
import { dump, load } from 'js-yaml'
import { getResource, genericDetailSections, configMapEntries, type DetailSection } from '../services/k8sResources'
import { requestJSON, startLogStream, type LogHandle } from '../services/k8sClient'
import { useI18n } from '../i18n'
import Menu from './Menu.vue'
import SyntaxEditor from './SyntaxEditor.vue'
import { decodePropertiesUnicode, restorePropertiesUnicode } from '../services/propertiesPreview'
import { Clipboard } from '@wailsio/runtime'
import MenuItem from './MenuItem.vue'

const props = defineProps<{ connId: string; mode: 'detail' | 'logs' | null; target: any | null; resourceKey: string; selfPathOverride?: (obj: any) => string; initialTab?: 'detail' | 'yaml' }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'saved'): void }>()

const { t } = useI18n()

// Right-click "复制" on selectable log text — shows only when something is
// selected; positioned at the pointer (Menu.openAt, viewport-adaptive).
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
  if (sel) {
    try { await Clipboard.SetText(sel) } catch { try { await navigator.clipboard.writeText(sel) } catch { /* ignore */ } }
  }
  copyMenuVisible.value = false
}

// Drawer width (draggable). Defaults widen for logs mode; not persisted.
// Max width tracks the actual parent container (not the window) so the drawer
// never extends past the container's left edge on any screen size.
const drawerEl = ref<HTMLElement | null>(null)
const drawerWidth = ref(420)
function maxDrawerWidth() {
  const parent = drawerEl.value?.parentElement
  return parent ? Math.max(320, parent.clientWidth - 40) : window.innerWidth - 120
}
watch(() => props.mode, (m) => {
  if (m === 'logs' && drawerWidth.value < 640) drawerWidth.value = Math.min(640, maxDrawerWidth())
})
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
  // Block the mousedown default action so the drag cannot promote into a text
  // selection / native drag (see AISidebar.onResizeStart for the full story).
  e.preventDefault()
  resizeStartX = e.clientX
  resizeStartW = drawerWidth.value
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
  e.preventDefault()
}

const tab = ref<'struct' | 'yaml'>('struct')
const editing = ref(false)
const draft = ref('')
const saving = ref(false)
const saveError = ref('')
const expandedConfig = ref(new Set<string>())
const rawProperties = ref(new Set<string>())
function toggleRawProperties(key: string) {
  if (rawProperties.value.has(key)) rawProperties.value.delete(key)
  else rawProperties.value.add(key)
}
const savedConfigMap = ref<any>(null)
const configEntries = computed(() => configMapEntries((savedConfigMap.value || props.target)?.data))
const keyEditVisible = ref(false)
const keySaving = ref(false)
const keySaveError = ref('')
const keyEditName = ref('')
const keyEditLang = ref<string>()
const keyEditOriginal = ref('')
const keyEditRaw = ref(false)
const keyEditDraft = ref('')
const keyEditorRef = ref<InstanceType<typeof SyntaxEditor> | null>(null)
function setKeyEditor(editor: unknown) {
  keyEditorRef.value = editor as InstanceType<typeof SyntaxEditor> | null
}
function isEditingKey(key: string) {
  return keyEditVisible.value && keyEditName.value === key
}
function cancelKeyEdit() {
  if (keySaving.value) return
  keyEditVisible.value = false
  keySaveError.value = ''
  keyEditTarget = null
  keyEditorRef.value = null
}
let keyEditTarget: any = null
let keyEditConnId = ''
let keyEditSession = 0

function startKeyEdit(entry: { key: string; value: string; lang?: string }) {
  const obj = savedConfigMap.value || props.target
  if (!obj || obj.immutable || keySaving.value || keyEditVisible.value || editing.value || saving.value) return
  keyEditTarget = obj
  keyEditConnId = props.connId
  keyEditName.value = entry.key
  keyEditLang.value = entry.lang
  keyEditOriginal.value = entry.value
  keyEditRaw.value = entry.lang === 'conf' && rawProperties.value.has(entry.key)
  keyEditDraft.value = entry.lang === 'conf' && !keyEditRaw.value ? decodePropertiesUnicode(entry.value) : entry.value
  keySaveError.value = ''
  keyEditSession++
  keyEditorRef.value = null
  expandedConfig.value.add(entry.key)
  keyEditVisible.value = true
}

async function saveKeyEdit() {
  if (keySaving.value || !keyEditTarget || !keyEditVisible.value) return
  const session = keyEditSession
  const normalize = (s: string) => s.replace(/\r\n|\r/g, '\n')
  const original = keyEditOriginal.value
  const text = keyEditorRef.value?.getValue() ?? keyEditDraft.value
  const value = keyEditLang.value === 'conf' && !keyEditRaw.value
    ? restorePropertiesUnicode(original, text)
    : normalize(original) === normalize(text) ? original
      : original.includes('\r\n') ? normalize(text).replace(/\n/g, '\r\n') : text
  if (value === original) { keyEditVisible.value = false; return }
  const uid = keyEditTarget.metadata?.uid
  if (!uid) { keySaveError.value = 'ConfigMap UID is missing. Reopen the resource and retry.'; return }
  const keyPath = '/data/' + keyEditName.value.replace(/~/g, '~0').replace(/\//g, '~1')
  const resourcePath = selfPath(keyEditTarget)
  keySaving.value = true
  keySaveError.value = ''
  try {
    // Atomic optimistic concurrency: a concurrent change to this key or a
    // deleted/recreated ConfigMap rejects the patch; other keys are untouched.
    const { status, data, raw } = await requestJSON(keyEditConnId, 'PATCH', resourcePath, [
      { op: 'test', path: '/metadata/uid', value: uid },
      { op: 'test', path: keyPath, value: original },
      { op: 'replace', path: keyPath, value },
    ], 'application/json-patch+json')
    if (status < 200 || status >= 300) throw new Error(`HTTP ${status}: ${raw?.slice(0, 300) || ''}`)
    if (session !== keyEditSession) return
    savedConfigMap.value = data || { ...keyEditTarget, data: { ...keyEditTarget.data, [keyEditName.value]: value } }
    keyEditVisible.value = false
    emit('saved')
    ElMessage.success(t('k8s.saved'))
  } catch (e: any) {
    if (session === keyEditSession) keySaveError.value = String(e?.message || e)
  } finally {
    keySaving.value = false
  }
}


function onConfigToggle(key: string, event: Event) {
  const details = event.target as HTMLDetailsElement
  if (!details.isConnected) return
  if (isEditingKey(key) && !details.open) {
    details.open = true
    return
  }
  if (details.open) expandedConfig.value.add(key)
  else expandedConfig.value.delete(key)

}

watch(() => props.target, () => {
  tab.value = props.initialTab === 'yaml' ? 'yaml' : 'struct'
  editing.value = false; saveError.value = ''
  expandedConfig.value = new Set()
  rawProperties.value = new Set()
  savedConfigMap.value = null
  keyEditSession++
  keyEditVisible.value = false
  keyEditTarget = null
  events.value = []; eventsError.value = ''
  if (isPod.value) loadEvents()
})

const headerTitle = computed(() => {
  const o = props.target
  if (!o) return ''
  return `${o.kind || getResource(props.resourceKey)?.kind || '?'} / ${o.metadata?.namespace || 'cluster'} / ${o.metadata?.name || ''}`
})

const sections = computed<DetailSection[]>(() => {
  const desc = getResource(props.resourceKey)
  if (desc?.detailSections?.length) return props.resourceKey === 'configmaps'
    ? desc.detailSections.filter(s => s.label !== 'Data') : desc.detailSections
  return genericDetailSections()
})

function fieldText(f: any): string {
  if (!props.target) return ''
  const v = f.value(props.target)
  return typeof v === 'object' && v ? v.text : String(v ?? '')
}

// ── Pod events (async fetch; can't ride the synchronous detailSections model) ──
const events = ref<any[]>([])
const eventsError = ref('')
const isPod = computed(() => {
  if (props.mode !== 'detail' || !props.target) return false
  const kind = props.target.kind || getResource(props.resourceKey)?.kind
  return props.resourceKey === 'pods' || kind === 'Pod'
})
let eventsGen = 0
async function loadEvents() {
  const myGen = ++eventsGen
  const o = props.target
  if (!o?.metadata) return
  const ns = o.metadata.namespace || ''
  const name = o.metadata.name || ''
  const path = `/api/v1/namespaces/${encodeURIComponent(ns)}/events?fieldSelector=involvedObject.name%3D${encodeURIComponent(name)}&limit=200`
  try {
    const { status, data, raw } = await requestJSON<any>(props.connId, 'GET', path)
    if (myGen !== eventsGen) return
    if (status === 200 && data) {
      events.value = (data.items || []).slice().sort((a: any, b: any) =>
        String(a.lastTimestamp || a.eventTime || '').localeCompare(String(b.lastTimestamp || b.eventTime || '')))
    } else {
      eventsError.value = `HTTP ${status}: ${raw?.slice(0, 200) || ''}`
    }
  } catch (e: any) {
    if (myGen === eventsGen) eventsError.value = String(e?.message || e)
  }
}

const yamlText = computed(() => {
  if (!props.target) return ''
  try { return dump(savedConfigMap.value || props.target, { sortKeys: false, lineWidth: 120 }) }
  catch (e: any) { return `# dump failed: ${e?.message || e}` }
})

function startEdit() { draft.value = yamlText.value; editing.value = true; saveError.value = '' }
function cancelEdit() { editing.value = false; saveError.value = '' }
async function copyYaml() {
  try { await navigator.clipboard.writeText(yamlText.value); ElMessage.success(t('k8s.copied')) }
  catch (e: any) { ElMessage.error(`${t('k8s.copyFailed')}: ${e?.message || e}`) }
}

function selfPath(o: any): string {
  if (props.selfPathOverride) return props.selfPathOverride(o)
  const desc = getResource(props.resourceKey)!
  const ns = o.metadata?.namespace
  const base = desc.listPath(ns || '').split('?')[0]
  return `${base}/${encodeURIComponent(o.metadata?.name)}`
}

async function save() {
  saving.value = true; saveError.value = ''
  try {
    const parsed = load(draft.value) as any
    if (!parsed || typeof parsed !== 'object') throw new Error('YAML is not an object')
    const { status, raw } = await requestJSON(props.connId, 'PUT', selfPath(props.target), JSON.stringify(parsed), 'application/json')
    if (status < 200 || status >= 300) throw new Error(`HTTP ${status}: ${raw?.slice(0, 200) || ''}`)
    editing.value = false
    emit('saved')
    emit('close')
    ElMessage.success(t('k8s.saved'))
  } catch (e: any) {
    saveError.value = String(e?.message || e)
  } finally {
    saving.value = false
  }
}

const logContainer = ref('')
const logTail = ref(500)
const logPrevious = ref(false)
const logTimestamps = ref(false)
const logWrap = ref(false)
const logPaused = ref(false)
// 日志始终自动滚动到底部（不再提供开关）。
// Timestamps are always streamed from the API; visibility is a CSS-only toggle
// (see .logs-hide-ts), so switching the checkbox never re-fetches the log.
const logLines = ref<{ ts: string; msg: string }[]>([])
const logBody = ref<HTMLElement | null>(null)
let logHandle: LogHandle | null = null
let logGen = 0

const containerNames = computed(() => (props.target?.spec?.containers || []).map((c: any) => c.name))

// Split the API's "<RFC3339 timestamp> <message>" line into its two parts.
function splitLogLine(line: string): { ts: string; msg: string } {
  const sp = line.indexOf(' ')
  if (sp > 0 && /^\d{4}-\d\d-\d\dT/.test(line)) {
    return { ts: line.slice(0, sp), msg: line.slice(sp + 1) }
  }
  return { ts: '', msg: line }
}

function stopLogs() { logGen++; logHandle?.stop(); logHandle = null }
async function restartLogs() {
  stopLogs()
  const myGen = ++logGen
  logLines.value = []
  if (props.mode !== 'logs' || !props.target) return
  const ns = props.target.metadata?.namespace
  const pod = props.target.metadata?.name
  const handle = await startLogStream(
    props.connId, ns, pod, logContainer.value, logTail.value, true, logPrevious.value,
    (line) => {
      if (logPaused.value) return
      logLines.value.push(splitLogLine(line))
      if (logLines.value.length > 5000) logLines.value.splice(0, logLines.value.length - 5000)
      nextTick(() => { if (logBody.value) logBody.value.scrollTop = logBody.value.scrollHeight })
    },
    () => {},
  )
  if (myGen !== logGen || props.mode !== 'logs' || !props.target) { handle.stop(); return }
  logHandle = handle
}

watch(() => [props.mode, props.target], () => {
  if (props.mode === 'logs' && props.target) {
    logContainer.value = containerNames.value[0] || ''
    restartLogs()
  } else {
    stopLogs()
  }
})

onBeforeUnmount(stopLogs)
</script>

<style scoped>
.config-entry-header { display: flex; align-items: center; gap: 0.5rem; list-style: none; }
.config-entry-header::-webkit-details-marker { display: none; }
.config-entry-header::before { content: ''; flex: none; width: 0.375rem; height: 0.375rem; border-right: 1.5px solid currentColor; border-bottom: 1.5px solid currentColor; transform: rotate(-45deg); margin-inline: 0.125rem; }
.config-entry[open] > .config-entry-header::before { transform: rotate(45deg); }
.config-entry-name { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.config-header-actions { display: inline-flex; align-items: center; flex: none; gap: 0.75rem; font-family: var(--font-ui); }
.config-header-actions :deep(.el-button + .el-button) { margin-left: 0; }
.config-inline-edit { min-width: 0; }
.config-edit-actions { display: flex; justify-content: flex-end; gap: 0.25rem; margin-top: 0.5rem; }
.config-edit-actions :deep(.el-button + .el-button) { margin-left: 0; }
.db-tab:disabled { opacity: 0.5; cursor: not-allowed; }
.config-edit-encoding { color: var(--text-muted); font-size: 0.75rem; }


.config-key { overflow-wrap: anywhere; user-select: text; }
.detail-label.config-key { width: 40%; min-width: 0; }
.config-entry { margin: 0.5rem 0; padding: 0.5rem; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); }
.config-entry summary { cursor: pointer; font-family: var(--font-mono); }
.config-preview { height: 16rem; margin-top: 0.5rem; }
.config-binary { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 16rem; overflow: auto; user-select: text; }

/* Copied verbatim from MonitorTabContent.vue */
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
.drawer-resizer:hover,
.drawer-resizer.hint {
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
}

/* Detail rows copied from MonitorTabContent.vue (.process-detail rows); the
   per-section scroll is dropped so the whole detail-body scrolls once. */
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

/* Copied verbatim from DBTabContent.vue */
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

/* Preformatted YAML body */
.k8s-yaml-drawer-body {
  margin: 0;
  padding: 0.75rem;
  overflow: auto;
  font-family: var(--font-mono, monospace);
  font-size: 0.75rem;
  white-space: pre-wrap;
  height: 100%;
  box-sizing: border-box;
  user-select: text;
  cursor: text;
}

.detail-body { flex: 1; overflow: auto; padding: 0.75rem 1rem; }
.detail-section-title { font-weight: 600; color: var(--text-secondary); margin: 0.5rem 0 0.25rem; }
.yaml-pane { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
.yaml-actions { display: flex; gap: 0.5rem; padding: 0.5rem 0.75rem; border-bottom: 1px solid var(--border-subtle); }
.yaml-error { color: var(--el-color-danger, #f56); padding: 0.5rem 0.75rem; font-size: 0.75rem; }

.detail-drawer.wide { width: 40rem; }
.logs-pane { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
/* 统一控件间距，避免 el-checkbox 自带 margin 造成时间戳等控件左右间隔过大 */
.logs-toolbar { display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 0.75rem; border-bottom: 1px solid var(--border-subtle); flex-wrap: wrap; }
.logs-toolbar :deep(.el-checkbox) { margin-right: 0; }
.logs-body { flex: 1; overflow: auto; }
.log-line { white-space: pre-wrap; word-break: break-all; }
/* 不换行模式：整行不折行，横向滚动 */
.logs-nowrap .log-line { white-space: pre; word-break: normal; }
/* 时间戳与正文之间留一个空格 */
.log-ts { color: var(--text-muted); margin-right: 0.5rem; }
.log-ts:empty { margin-right: 0; }
.logs-hide-ts .log-ts { display: none; }
</style>
