<template>
  <el-dialog
    append-to-body
    :model-value="visible"
    :title="t('netscan.title')"
    width="85%"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    destroy-on-close
    class="netscan-dialog"
    style="display:flex; flex-direction:column; height:90vh; max-height:90vh; border-radius:var(--radius-sm);"
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <!-- ── Content root: fixed-height flex column. A fixed height here (rather
         than relying on el-dialog's internal header/body/footer flex, whose
         :deep selectors may not resolve to the dialog root) guarantees the two
         scan sections always get a definite height to lay out inside. ── -->
    <div class="netscan-content">
      <div class="netscan-body">
        <!-- ═══════════ IP scan section: controls live directly above its table ═══════════ -->
        <div class="scan-section">
          <div class="scan-header">
            <span class="scan-title">{{ t('netscan.ipScan.title') }}</span>
            <div v-if="ipScanning" class="scan-progress">
              <el-progress
                :percentage="ipProgressPercent"
                :status="ipTotal > 0 && ipDone >= ipTotal ? 'success' : undefined"
                :stroke-width="8"
              />
              <span class="progress-label">{{ ipDone }} / {{ ipTotal }}</span>
            </div>
          </div>
          <div class="scan-controls">
            <el-input
              v-model="octet1"
              :placeholder="'10'"
              maxlength="3"
              class="octet-input"
              :disabled="ipScanning"
              @keyup.enter="startIPScan"
            />
            <span class="toolbar-sep">.</span>
            <el-input
              v-model="octet2"
              :placeholder="'0'"
              maxlength="3"
              class="octet-input"
              :disabled="ipScanning"
              @keyup.enter="startIPScan"
            />
            <span class="toolbar-sep">.</span>
            <el-input
              v-model="octet3"
              :placeholder="'0'"
              maxlength="3"
              class="octet-input"
              :disabled="ipScanning"
              @keyup.enter="startIPScan"
            />
            <span class="toolbar-sep">:</span>
            <el-input-number
              v-model="lastStart"
              :min="1"
              :max="254"
              :disabled="ipScanning"
              controls-position="right"
              class="octet-range"
            />
            <span class="toolbar-sep">→</span>
            <el-input-number
              v-model="lastEnd"
              :min="1"
              :max="254"
              :disabled="ipScanning"
              controls-position="right"
              class="octet-range"
            />
            <el-button v-if="!ipScanning" type="primary" class="go-btn" @click="startIPScan">
              {{ t('netscan.ipScan.start') }}
            </el-button>
            <el-button v-else type="danger" class="go-btn" @click="stopIPScan">
              {{ t('netscan.common.stop') }}
            </el-button>
          </div>
          <div class="table-wrap">
            <el-table
              :data="ipHosts"
              size="small"
              border
              height="100%"
              class="scan-table"
              @row-contextmenu="onIpCtxMenu"
            >
              <el-table-column prop="ip" :label="t('netscan.ipScan.columns.ip')" :width="uiPx(120)" align="center" sortable :sort-method="compareIp">
                <template #default="{ row }">
                  <!-- Click the IP to open the connection form prefilled with the host -->
                  <span class="host-ip clickable" :title="t('netscan.ipScan.connectHint')" @click.stop="openConnectionForm(row.ip)">{{ row.ip }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="mac" :label="t('netscan.ipScan.columns.mac')" :width="uiPx(120)" align="center">
                <template #default="{ row }">{{ row.mac || '-' }}</template>
              </el-table-column>
              <el-table-column prop="hostname" :label="t('netscan.ipScan.columns.hostname')" :width="uiPx(120)" align="center" show-overflow-tooltip>
                <template #default="{ row }">{{ row.hostname || '-' }}</template>
              </el-table-column>
              <el-table-column
                v-for="sp in serviceColumns"
                :key="sp.port"
                :label="`${sp.port} (${sp.name})`"
                align="center"
                :width="sp.name === 'telnet' ? uiPx(108) : uiPx(88)"
              >
                <template #default="{ row }">
                  <!-- Open services show a check icon and are clickable: opens
                       the connection form with host + port. Closed ones show --. -->
                  <button
                    v-if="row.ports && row.ports[sp.port]"
                    class="svc-open-btn"
                    :title="t('netscan.ipScan.connectHint')"
                    @click.stop="openConnectionForm(row.ip, sp.port)"
                  >
                    <CircleCheck :size="16" />
                  </button>
                  <span v-else class="svc-closed">--</span>
                </template>
              </el-table-column>
              <el-table-column :label="t('netscan.ipScan.deepScan')" :width="uiPx(96)" align="center" :sortable="false">
                <template #default="{ row }">
                  <el-button
                    class="deep-scan-btn"
                    size="small"
                    :disabled="portScanning"
                    @click.stop="deepScan(row)"
                  >
                    <Search :size="12" class="deep-scan-icon" />
                    {{ t('netscan.ipScan.deepScan') }}
                  </el-button>
                </template>
              </el-table-column>
            </el-table>
            <div v-if="!ipScanning && ipHosts.length === 0 && !ipStarted" class="scan-hint">
              {{ t('netscan.ipScan.ipHint') }}
            </div>
            <div v-if="!ipScanning && ipHosts.length === 0 && ipStarted" class="scan-hint">
              {{ t('netscan.ipScan.noHosts') }}
            </div>
          </div>
        </div>

        <!-- ═══════════ Port scan section: controls live directly above its table ═══════════ -->
        <div class="scan-section">
          <div class="scan-header">
            <span class="scan-title">{{ t('netscan.portScan.title') }}</span>
            <div v-if="portScanning" class="scan-progress">
              <el-progress
                :percentage="portProgressPercent"
                :status="portTotal > 0 && portDone >= portTotal ? 'success' : undefined"
                :stroke-width="8"
              />
              <span class="progress-label">{{ portDone }} / {{ portTotal }}</span>
            </div>
          </div>
          <div class="scan-controls">
            <el-input
              v-model="portHost"
              :placeholder="t('netscan.portScan.host')"
              class="port-host-input"
              :disabled="portScanning"
              @keyup.enter="startPortScan"
            />
            <el-radio-group v-model="portMode" :disabled="portScanning" class="port-mode">
              <el-radio-button :value="'common'">{{ t('netscan.portScan.common') }}</el-radio-button>
              <el-radio-button :value="'range'">{{ t('netscan.portScan.range') }}</el-radio-button>
            </el-radio-group>
            <template v-if="portMode === 'range'">
              <el-input-number
                v-model="portStart"
                :min="1"
                :max="65535"
                :disabled="portScanning"
                controls-position="right"
                class="port-num"
              />
              <span class="toolbar-sep">→</span>
              <el-input-number
                v-model="portEnd"
                :min="1"
                :max="65535"
                :disabled="portScanning"
                controls-position="right"
                class="port-num"
              />
            </template>
            <span class="ctl-label" :title="t('netscan.portScan.workersHint')">{{ t('netscan.portScan.workers') }}</span>
            <el-input-number
              v-model="portWorkers"
              :min="1"
              :max="512"
              :disabled="portScanning"
              controls-position="right"
              class="workers-num"
            />
            <el-button v-if="!portScanning" type="primary" class="go-btn" @click="startPortScan">
              {{ t('netscan.portScan.start') }}
            </el-button>
            <el-button v-else type="danger" class="go-btn" @click="stopPortScan">
              {{ t('netscan.common.stop') }}
            </el-button>
          </div>
          <div class="table-wrap">
            <el-table
              :data="portResults"
              size="small"
              border
              height="100%"
              class="scan-table port-table"
              @row-click="onPortRowClick"
              @row-contextmenu="onPortCtxMenu"
            >
              <el-table-column prop="host" :label="t('netscan.portScan.columns.host')" :width="uiPx(150)" align="center">
                <template #default="{ row }">
                  <span class="host-ip">{{ row.host }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="port" :label="t('netscan.portScan.columns.port')" sortable :width="uiPx(90)" align="center">
                <template #default="{ row }">
                  <span class="port-num-text">#{{ row.port }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="service" :label="t('netscan.portScan.columns.service')" :width="uiPx(160)" align="center">
                <template #default="{ row }">{{ row.service || '-' }}</template>
              </el-table-column>
              <el-table-column :label="t('netscan.portScan.columns.state')" :width="uiPx(160)" align="center">
                <template #default="{ row }">
                  <el-tag type="success" size="small">{{ row.state }}</el-tag>
                </template>
              </el-table-column>
            </el-table>
            <div v-if="!portScanning && portResults.length === 0" class="scan-hint">
              {{ t('netscan.portScan.portHint') }}
            </div>
          </div>
        </div>
      </div>
    </div>
    <!-- end .netscan-content -->

    <!-- ── Right-click export context menu (for both result tables) ── -->
    <Teleport to="body">
      <ul
        v-if="ctxMenu.visible"
        class="ctx-menu"
        :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
        @contextmenu.prevent
        @click="hideCtxMenu"
      >
        <li class="ctx-item" @click.stop="copyResults(ctxMenu.kind)">
          <span class="ctx-label">{{ t('netscan.export.copy') }}</span>
        </li>
        <li class="ctx-item" @click.stop="exportResults(ctxMenu.kind)">
          <span class="ctx-label">{{ t('netscan.export.results') }}</span>
        </li>
      </ul>
    </Teleport>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'
import { Search, CircleCheck } from '@lucide/vue'
import { StartIPScan, StopIPScan, StartPortScan, StopPortScan, SaveFileDialogFiltered, WriteFileBase64, GetLocalSubnet } from '../../../bindings/github.com/ys-ll/uniterm/app'
import { msg } from '../../services/message'
import { useI18n } from '../../i18n'
import { uiPx } from '../../utils/uiScale'
import { utf8ToBase64 } from '../../utils/base64'

defineOptions({ name: 'NetworkScanTabContent' })

const props = defineProps<{ visible?: boolean }>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'new-connection', d: { host: string; port?: number; type?: string }): void
}>()

const { t } = useI18n()

// Service columns probed per host by the Go backend (ipscan.go ServicePorts).
const serviceColumns = [
  { port: 22, name: 'ssh' },
  { port: 23, name: 'telnet' },
  { port: 3389, name: 'rdp' },
  { port: 5900, name: 'vnc' },
  { port: 21, name: 'ftp' },
  { port: 445, name: 'smb' },
  { port: 80, name: 'http' }
]

// Port → connection type for the click-to-create actions, mirroring the
// backend's serviceNames. Ports not listed open the form with the default
// type so the user can pick one themselves.
const portConnTypes: Record<number, string> = {
  22: 'ssh',
  23: 'telnet',
  3389: 'rdp',
  5900: 'vnc',
  21: 'ftp',
  445: 'smb',
  80: 'tcp'
}

// ── IP scan state: Moba-style base (first three octets) + last-octet range ──
const octet1 = ref('')
const octet2 = ref('')
const octet3 = ref('')
const lastStart = ref(1)
const lastEnd = ref(254)
const ipScanning = ref(false)
const ipStarted = ref(false)
const ipScanId = ref('')
const ipDone = ref(0)
const ipTotal = ref(0)
// keyed by ip so re-emitted hosts (with MAC filled) upsert instead of duplicating
const ipHosts = ref<any[]>([])

// ── Port scan state ──
const portHost = ref('')
const portMode = ref<'common' | 'range'>('common')
const portStart = ref(1)
const portEnd = ref(1000)
// Concurrent probes. Low by default: a SYN burst against one host gets
// rate-limited or dropped by many firewalls, which silently hides open ports.
const portWorkers = ref(32)
const portScanning = ref(false)
const portScanId = ref('')
const portDone = ref(0)
const portTotal = ref(0)
const portResults = ref<any[]>([])

// The set of scan ids this component has started. All netscan events are
// global (any open tab receives them), so we filter by ownership before
// mutating any state.
const ownedScans = new Set<string>()
// scanId -> host, to label port results when scanning multiple targets.
const portScanTargets = new Map<string, string>()

const ipProgressPercent = computed(() => ipTotal.value > 0 ? Math.round((ipDone.value / ipTotal.value) * 100) : 0)
const portProgressPercent = computed(() => portTotal.value > 0 ? Math.round((portDone.value / portTotal.value) * 100) : 0)

// ── Scan watchdog ──
// Safety net so the UI can never stay stuck in a "scanning" state even if a
// netscan done/progress event is lost (a backend scan always completes, so an
// event that never arrives would otherwise leave the progress bar frozen and
// the stop button unresponsive). Every progress/done event for an owned scan
// re-arms the timer; it only fires when an active scan has gone silent well
// past any plausible completion time for that range.
let watchdogTimer: ReturnType<typeof setTimeout> | null = null
const WATCHDOG_MS = 60000
function armScanWatchdog() {
  if (watchdogTimer) clearTimeout(watchdogTimer)
  watchdogTimer = setTimeout(() => {
    watchdogTimer = null
    // Only fire when a scan is still marked in progress. If one is, assume the
    // done event was lost and force the UI back to idle.
    if (ipScanning.value || portScanning.value) {
      ipScanning.value = false
      portScanning.value = false
      msg.error(t('netscan.common.scanStalled'))
    }
  }, WATCHDOG_MS)
}
function clearScanWatchdog() {
  if (watchdogTimer) {
    clearTimeout(watchdogTimer)
    watchdogTimer = null
  }
}

// Prefill the base octets from the machine's own /24 subnet the first time
// the dialog opens. Errors are silent — the inputs just stay empty.
let subnetFetched = false
watch(() => props.visible, (v) => {
  if (!v || subnetFetched) return
  subnetFetched = true
  GetLocalSubnet().then((s: { base: string; lastOctet: number }) => {
    const parts = (s.base || '').split('.')
    if (parts.length === 3) {
      octet1.value = parts[0]
      octet2.value = parts[1]
      octet3.value = parts[2]
    }
  }).catch(() => {})
}, { immediate: true })

// Compare two IPv4 strings segment-by-segment by numeric value, so that
// 192.168.1.10 sorts after 192.168.1.2 instead of lexicographically first.
function compareIp(a: { ip: string }, b: { ip: string }): number {
  const pa = (a.ip || '').split('.').map(Number)
  const pb = (b.ip || '').split('.').map(Number)
  for (let i = 0; i < 4; i++) {
    const diff = (pa[i] || 0) - (pb[i] || 0)
    if (diff !== 0) return diff
  }
  return 0
}

// Parse one octet input; null when it is not a number in [0, 255].
function parseOctet(s: string): number | null {
  const v = s.trim()
  if (!/^\d{1,3}$/.test(v)) return null
  const n = Number(v)
  return n >= 0 && n <= 255 ? n : null
}

async function startIPScan() {
  const o1 = parseOctet(octet1.value)
  const o2 = parseOctet(octet2.value)
  const o3 = parseOctet(octet3.value)
  if (o1 === null || o2 === null || o3 === null) {
    msg.warning(t('netscan.ipScan.invalidBase'))
    return
  }
  const base = `${o1}.${o2}.${o3}`
  let from = lastStart.value || 1
  let to = lastEnd.value || 254
  if (from > to) [from, to] = [to, from]
  try {
    // Start capturing *before* calling the backend: for a fast target the done
    // event can arrive before `await StartIPScan` resolves, and without the
    // capture window it would be dropped by the ownership check, leaving the
    // UI stuck on "scanning". Events buffered here are replayed after the id
    // is registered below.
    captureActive = true
    const id = await StartIPScan(`${base}.${from}`, `${base}.${to}`)
    ownedScans.add(id)
    ipScanId.value = id
    ipStarted.value = true
    ipScanning.value = true
    ipHosts.value = []
    ipDone.value = 0
    ipTotal.value = 0
    replayCaptured()
    armScanWatchdog()
  } catch (e: any) {
    captureActive = false
    pendingEvents.length = 0
    msg.error(e?.message || t('netscan.ipScan.startFailed'))
  }
}

function stopIPScan() {
  clearScanWatchdog()
  if (ipScanId.value) StopIPScan(ipScanId.value)
}

async function startPortScan() {
  const host = portHost.value.trim()
  if (!host) {
    msg.warning(t('netscan.portScan.missingHost'))
    return
  }
  const commonOnly = portMode.value === 'common'
  const startPort = portStart.value || 1
  const endPort = portEnd.value || 65535
  const workers = portWorkers.value || 32
  try {
    // Capture early events (see startIPScan): a fast-refusing port can finish
    // the scan before `await StartPortScan` resolves, so its done event would
    // otherwise be dropped and the UI would stay stuck on "scanning".
    captureActive = true
    const id = await StartPortScan(host, commonOnly, startPort, endPort, workers)
    ownedScans.add(id)
    portScanId.value = id
    portScanTargets.set(id, host)
    portScanning.value = true
    // Fresh result set each run — replace, never append to stale data.
    portResults.value = []
    portDone.value = 0
    portTotal.value = 0
    replayCaptured()
    armScanWatchdog()
  } catch (e: any) {
    captureActive = false
    pendingEvents.length = 0
    msg.error(e?.message || t('netscan.portScan.startFailed'))
  }
}

function stopPortScan() {
  clearScanWatchdog()
  if (portScanId.value) StopPortScan(portScanId.value)
}

// Deep scan: fill the port-scan target with this host and immediately start
// a common-ports scan, like MobaXterm's per-row deep scan action.
function deepScan(row: any) {
  if (portScanning.value) return
  portHost.value = row.ip
  portMode.value = 'common'
  startPortScan()
}

// Open the global connection form prefilled with the scan result. The
// connection type is mapped from the service port when it is a known one,
// so clicking a service's check creates that type of connection.
function openConnectionForm(host: string, port?: number) {
  emit('new-connection', { host, port, type: port ? portConnTypes[port] : undefined })
}

// ── Right-click export context menu ──
const ctxMenu = ref<{ visible: boolean; x: number; y: number; kind: 'ip' | 'port' }>({
  visible: false,
  x: 0,
  y: 0,
  kind: 'ip'
})

function onIpCtxMenu(_row: any, _column: any, event: MouseEvent) {
  showCtxMenu(event, 'ip')
}
function onPortCtxMenu(_row: any, _column: any, event: MouseEvent) {
  showCtxMenu(event, 'port')
}
// Clicking a port-scan result row opens the connection form with host + port.
function onPortRowClick(row: any) {
  openConnectionForm(row.host, row.port)
}
function showCtxMenu(event: MouseEvent, kind: 'ip' | 'port') {
  event.preventDefault()
  event.stopPropagation()
  const menuW = 140
  const menuH = 34
  const margin = 8
  let x = event.clientX
  let y = event.clientY
  if (x + menuW > window.innerWidth - margin) x = window.innerWidth - menuW - margin
  if (y + menuH > window.innerHeight - margin) y = window.innerHeight - menuH - margin
  ctxMenu.value = { visible: true, x, y, kind }
}
function hideCtxMenu() {
  ctxMenu.value.visible = false
}

// ── Export the current result table as CSV via the native save dialog ──
async function exportResults(kind: 'ip' | 'port') {
  hideCtxMenu()
  const sep = ','
  const now = new Date()
  const stamp = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}_${String(now.getHours()).padStart(2, '0')}-${String(now.getMinutes()).padStart(2, '0')}-${String(now.getSeconds()).padStart(2, '0')}`

  let fileName: string
  let content: string

  if (kind === 'ip') {
    fileName = `ip_scan_${stamp}.csv`
    const header = [
      t('netscan.ipScan.columns.ip'),
      t('netscan.ipScan.columns.mac'),
      t('netscan.ipScan.columns.hostname'),
      ...serviceColumns.map(sp => `${sp.port} (${sp.name})`)
    ].join(sep)
    const openTag = t('netscan.ipScan.open')
    const closedTag = t('netscan.ipScan.closed')
    const body = ipHosts.value.map(h =>
      [
        h.ip || '',
        h.mac || '',
        h.hostname || '',
        ...serviceColumns.map(sp => (h.ports && h.ports[sp.port] ? openTag : closedTag))
      ].join(sep)
    )
    content = [header, ...body].join('\n')
  } else {
    fileName = `port_scan_${stamp}.csv`
    const header = [
      t('netscan.portScan.columns.host'),
      t('netscan.portScan.columns.port'),
      t('netscan.portScan.columns.service'),
      t('netscan.portScan.columns.state')
    ].join(sep)
    const body = portResults.value.map(r =>
      [r.host || '', r.port ?? '', r.service || '', r.state || ''].join(sep)
    )
    content = [header, ...body].join('\n')
  }

  // Escape fields so the CSV stays valid even if a value contains a delimiter.
  const escapeCell = (cell: string) => {
    const s = String(cell ?? '')
    return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
  }
  const lines = content.split('\n').map(line => line.split(sep).map(escapeCell).join(','))
  content = '﻿' + lines.join('\n') // BOM helps Excel open UTF-8 CSV

  // Ask the user where to save via the native save dialog, then write the file.
  try {
    const path = await SaveFileDialogFiltered(t('netscan.export.title'), fileName, t('netscan.export.filterCsv'), '*.csv')
    if (!path) return // user dismissed the dialog
    await WriteFileBase64(path, utf8ToBase64(content))
    msg.success(t('netscan.export.saved'))
  } catch (e) {
    msg.error(`${t('netscan.export.failed')} ${(e as Error)?.message ?? ''}`)
  }
}

// Copy the visible result table (IP or port) to the clipboard as
// tab-separated text so it pastes cleanly into spreadsheets/editors.
async function copyResults(kind: 'ip' | 'port') {
  hideCtxMenu()
  const sep = '\t'
  let lines: string[][]
  if (kind === 'ip') {
    const header = [
      t('netscan.ipScan.columns.ip'),
      t('netscan.ipScan.columns.mac'),
      t('netscan.ipScan.columns.hostname'),
      ...serviceColumns.map(sp => `${sp.port} (${sp.name})`)
    ]
    const openTag = t('netscan.ipScan.open')
    const closedTag = t('netscan.ipScan.closed')
    const body = ipHosts.value.map(h => [
      h.ip || '',
      h.mac || '',
      h.hostname || '',
      ...serviceColumns.map(sp => (h.ports && h.ports[sp.port] ? openTag : closedTag))
    ])
    lines = [header, ...body]
  } else {
    const header = [
      t('netscan.portScan.columns.host'),
      t('netscan.portScan.columns.port'),
      t('netscan.portScan.columns.service'),
      t('netscan.portScan.columns.state')
    ]
    const body = portResults.value.map(r => [r.host || '', r.port ?? '', r.service || '', r.state || ''])
    lines = [header, ...body]
  }
  try {
    await navigator.clipboard.writeText(lines.map(row => row.join(sep)).join('\n'))
    msg.success(t('netscan.export.copied'))
  } catch (e) {
    msg.error(`${t('netscan.export.copyFailed')} ${(e as Error)?.message ?? ''}`)
  }
}

// 事件归属判断：若该扫描 id 已注册，或（作为竞态兜底）与最近一次启动的
// 端口/IP 扫描 id 一致，则视为属于本组件。避免首次运行偶尔丢失全部事件。
function isOwnedScan(id: string): boolean {
  if (ownedScans.has(id)) return true
  return !!portScanId.value && id === portScanId.value || !!ipScanId.value && id === ipScanId.value
}

// ── 早期事件捕获 ──
// 后端在 StartPortScan 内部即 `go m.runPortScan(...)` 异步启动扫描并立刻发
// 事件。对极快返回的目标（例如 GitLab 主机上立即回 RST 的关闭端口），扫描的
// done 事件可能在等待中的 start*Scan resolve 并注册返回的 id 之前就到达 JS。
// 此时 isOwnedScan 会误判并丢弃该事件，导致 await 置 portScanning = true 后
// 再也没有 done 来复位它：UI 永久卡在“扫描中”，进度条冻结、停止无反应
// （后端早已 finish，Stop 是空操作）。原本埋日志桩之所以“有效”，正是因为
// 它引入了恰好够 await 抢先完成注册的延迟。为真正关闭这个竞态：在启动进行
// 期间缓存所有到达的 netscan 事件，待 id 注册后立即回放归属本组件的事件。
let captureActive = false
const pendingEvents: any[] = []

// 当一次扫描启动仍在进行（await 尚未返回 id）、且事件尚不属于本组件时，
// 将事件缓存待回放。返回 true 表示已缓存（调用方不应立即处理），
// 返回 false 表示应当丢弃（捕获窗口已关闭且事件不属于本组件）。
function tryCapture(type: string, ev: any): boolean {
  if (!captureActive) return false
  pendingEvents.push({ type, ev })
  return true
}

// 按到达顺序回放所有已归属到本组件扫描的缓存事件，确保快速扫描的 done
// 即使在 id 注册前到达也不会丢失。
function replayCaptured() {
  captureActive = false
  if (pendingEvents.length === 0) return
  const evs = pendingEvents.splice(0)
  for (const { type, ev } of evs) {
    if (!isOwnedScan(ev?.data?.scanId)) continue
    dispatchScanEvent(type, ev)
  }
}

// 所有 netscan 回调的统一入口：已归属的事件立即处理；尚未归属的事件在
// 启动窗口内缓存（注册 id 后回放），否则丢弃。
function routeScanEvent(type: string, ev: any) {
  const d = ev?.data
  if (!d) return
  if (isOwnedScan(d.scanId)) {
    dispatchScanEvent(type, ev)
  } else {
    tryCapture(type, ev)
  }
}

// 每种事件的实际状态更新（实时回调与缓存回放共用，保证回放的早期事件
// 与正常到达的事件对状态的影响完全一致）。
function dispatchScanEvent(type: string, ev: any) {
  const d = ev?.data
  if (!d) return
  switch (type) {
    case 'host-found':
      const host = d.host
      if (!host?.ip) return
      const idx = ipHosts.value.findIndex(h => h.ip === host.ip)
      if (idx >= 0) ipHosts.value[idx] = host
      else ipHosts.value.push(host)
      break
    case 'progress': {
      const done = Number(d.done) || 0
      const total = Number(d.total) || 0
      if (d.kind === 'ip') {
        ipDone.value = done
        ipTotal.value = total
      } else if (d.kind === 'port') {
        portDone.value = done
        portTotal.value = total
      }
      armScanWatchdog()
      break
    }
    case 'port-open': {
      const host_ = portScanTargets.get(d.scanId) || portHost.value || ''
      // Avoid duplicate rows for the same host+port.
      if (!portResults.value.some(r => r.host === host_ && r.port === d.port)) {
        portResults.value.push({ host: host_, port: d.port, service: d.service, state: d.state })
      }
      break
    }
    case 'done':
      ownedScans.delete(d.scanId)
      if (d.kind === 'ip') {
        ipScanning.value = false
      } else if (d.kind === 'port') {
        portScanning.value = false
      }
      if (!ipScanning.value && !portScanning.value) clearScanWatchdog()
      break
  }
}

let unsubs: (() => void)[] = []

onMounted(() => {
  unsubs = [
    Events.On('netscan:host-found', (ev: any) => routeScanEvent('host-found', ev)),
    Events.On('netscan:progress', (ev: any) => routeScanEvent('progress', ev)),
    Events.On('netscan:port-open', (ev: any) => routeScanEvent('port-open', ev)),
    Events.On('netscan:done', (ev: any) => routeScanEvent('done', ev))
  ]
  // Close the export context menu on any outside click.
  document.addEventListener('click', hideCtxMenu)
})

onUnmounted(() => {
  // Stop any in-flight scans this component started, then detach listeners.
  clearScanWatchdog()
  if (ipScanId.value) StopIPScan(ipScanId.value)
  if (portScanId.value) StopPortScan(portScanId.value)
  unsubs.forEach(u => u())
  document.removeEventListener('click', hideCtxMenu)
  ownedScans.clear()
  portScanTargets.clear()
  // Drop any buffered early events and the open capture window so a detached
  // component can't swallow events meant for a sibling instance.
  captureActive = false
  pendingEvents.length = 0
})
</script>

<style scoped>
/* Aligned with the SFTP built-in editor / file-list look: a dialog whose
   header is a single full-width flat toolbar, with CSS-variable theming so the
   whole scan UI follows dark / deep-blue / light automatically. */
/* The el-dialog root carries class="netscan-dialog" directly, so style it with
   that selector (NOT ".netscan-dialog :deep(.el-dialog)" — that descendant form
   never matches because they are the same element, which silently disabled the
   fixed-height flex column and collapsed the tables). */
.netscan-dialog {
  display: flex;
  flex-direction: column;
  height: 90vh;
  max-height: 90vh;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
}
.netscan-dialog :deep(.el-dialog__header) {
  padding: 0.625rem 1rem;
  border-bottom: 1px solid var(--border-subtle);
  margin: 0;
  background: var(--bg-elevated);
  flex-shrink: 0;
}
.netscan-dialog :deep(.el-dialog__title) {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-ui);
}
.netscan-dialog :deep(.el-dialog__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.75rem;
  overflow: hidden;
}
.netscan-dialog :deep(.el-dialog__footer) {
  padding: 0.5rem 1rem;
  border-top: 1px solid var(--border-subtle);
  background: var(--bg-elevated);
  flex-shrink: 0;
}

/* Fixed-height content root ensures the two sections always have room to lay
   out, independent of el-dialog's internal header/body/footer sizing. Keep the
   explicit height here: relying on flex:1 against el-dialog__body collapses to
   zero height when the :deep body selector fails to match (append-to-body). */
.netscan-content {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: calc(90vh - 7.5rem);
  max-height: calc(90vh - 7.5rem);
  overflow: hidden;
}
/* ── Results region: two stacked scan sections, each locked to exactly half
   the body height so each table scrolls independently and neither gets pushed
   out of the dialog. ── */
.netscan-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.scan-section {
  flex: 1 1 0;         /* both sections share the body height equally */
  min-height: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  overflow: hidden;
  background: var(--bg-elevated);
}

/* ── Per-section control row: sits directly above its own result table ── */
.scan-controls {
  display: flex;
  align-items: center;
  gap: 0.4375rem;
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
  flex-wrap: wrap;
}
.toolbar-sep {
  color: var(--text-muted);
  font-size: 0.75rem;
}

/* ── Flat inputs: drop Element's default chrome for a slim underlying field
       that reads like the SFTP filter input. ── */
.scan-controls .el-input,
.scan-controls .el-input-number {
  --el-input-bg-color: var(--bg-base);
  --el-input-border-color: var(--border-subtle);
  --el-input-hover-border-color: var(--accent);
  --el-input-focus-border-color: var(--accent);
  --el-input-text-color: var(--text-primary);
  --el-input-placeholder-color: var(--text-muted);
  --el-input-height: 1.875rem;
  --el-input-border-radius: var(--radius-sm);
}
.scan-controls .el-input__wrapper,
.scan-controls .el-input-number .el-input__wrapper {
  background: var(--bg-base);
  box-shadow: 0 0 0 1px var(--border-subtle) inset;
  transition: box-shadow 0.12s ease;
}
.scan-controls .el-input__wrapper:hover {
  box-shadow: 0 0 0 1px var(--bg-hover) inset;
}
.scan-controls .el-input__wrapper.is-focus {
  box-shadow: 0 0 0 1px var(--accent) inset;
}

.octet-input { width: 2.75rem; }
.octet-input :deep(.el-input__inner) { text-align: center; font-family: var(--font-mono); }
/* Wide enough for 3 digits next to the right-side up/down controls. */
.octet-range { width: 5.5rem; }
.octet-range :deep(.el-input__inner) { text-align: center; font-family: var(--font-mono); }
.port-host-input { width: 9.5rem; }
.port-num { width: 6.25rem; }
/* Inline label for the concurrency option. */
.ctl-label {
  font-size: 0.6875rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  color: var(--text-secondary);
  font-family: var(--font-ui);
}
.workers-num { width: 5rem; }

/* ── Mode toggle as a flat segmented control ── */
.scan-controls .el-radio-group.port-mode {
  display: inline-flex;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  overflow: hidden;
  flex-shrink: 0;
}
.scan-controls .port-mode .el-radio-button__inner {
  border: none;
  background: transparent;
  color: var(--text-muted);
  font-family: var(--font-ui);
  font-size: 0.75rem;
  padding: 0.3rem 0.625rem;
  box-shadow: none;
  transition: all 0.12s ease;
}
.scan-controls .port-mode .el-radio-button__original-radio:checked + .el-radio-button__inner {
  background: var(--accent-subtle);
  color: var(--accent);
}

/* ── Buttons: flat, accent-primary / danger-secondary ── */
.scan-controls .el-button {
  --el-button-border-radius: var(--radius-sm);
  font-family: var(--font-ui);
}
.go-btn.el-button--primary {
  --el-button-bg-color: var(--accent);
  --el-button-border-color: var(--accent);
  --el-button-text-color: #fff;
  --el-button-hover-bg-color: color-mix(in srgb, var(--accent) 85%, #000);
  --el-button-hover-border-color: color-mix(in srgb, var(--accent) 85%, #000);
  --el-button-active-bg-color: color-mix(in srgb, var(--accent) 70%, #000);
  --el-button-active-border-color: color-mix(in srgb, var(--accent) 70%, #000);
}
.go-btn.el-button--danger {
  --el-button-bg-color: transparent;
  --el-button-border-color: var(--el-color-danger);
  --el-button-text-color: var(--el-color-danger);
  --el-button-hover-bg-color: color-mix(in srgb, var(--el-color-danger) 12%, transparent);
}

/* ── Deep scan per-row button ── */
.deep-scan-btn {
  font-size: 0.6875rem;
  padding: 0.25rem 0.5rem;
}
.deep-scan-icon { margin-right: 0.2rem; }

/* Each table lives in its own well. The el-table inside uses height="100%"
   and manages its own scrolling, so the table always fills the whole section
   and its scrollbars sit at the region's edges (not floating under short
   result sets). */
.table-wrap {
  flex: 1;
  min-height: 0;
  position: relative;
  overflow: hidden;
}
.scan-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.4375rem 0.75rem;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}
.scan-title {
  font-size: 0.8125rem;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-ui);
  flex-shrink: 0;
  letter-spacing: 0.01em;
}

/* ── Progress bar fits the flat header ── */
.scan-progress {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex: 1;
  min-width: 12rem;
}
.scan-progress .el-progress { flex: 1; margin: 0; }
.progress-label {
  font-family: var(--font-mono);
  font-size: 0.75rem;
  color: var(--text-secondary);
  white-space: nowrap;
}

/* ── Result tables: compact, hairline rows, mono numerals — mirrors the
   SFTP file list instead of Element's boxy default grid.
   user-select: text lets cell contents be mouse-selected and copied. ── */
.scan-table {
  width: 100%;
  --el-table-border-color: var(--border-subtle);
}
.scan-table :deep(.el-table__inner-wrapper::before),
.scan-table :deep(.el-table__border-left-patch),
.scan-table :deep(.el-table::after),
.scan-table :deep(.el-table__inner-wrapper::after) { background: var(--border-subtle); }

.scan-table :deep(.el-table__body td.el-table__cell),
.scan-table :deep(.el-table__header-wrapper th.el-table__cell) {
  user-select: text;
  -webkit-user-select: text;
}

.scan-table :deep(.el-table__header-wrapper th.el-table__cell) {
  background: var(--bg-elevated);
  color: var(--text-secondary);
  font-size: 0.6875rem;
  font-weight: 600;
  letter-spacing: 0.02em;
  text-transform: uppercase;
  border-bottom: 1px solid var(--border-subtle);
  padding: 0.375rem 0.25rem;
}
.scan-table :deep(.el-table__header-wrapper th.el-table__cell .cell) {
  padding: 0;
}
.scan-table :deep(.el-table__body td.el-table__cell) {
  padding: 0.3125rem 0.25rem;
  font-size: 0.75rem;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-subtle);
}
.scan-table :deep(.el-table__body td.el-table__cell .cell) {
  padding: 0;
}
/* Only genuinely clickable things get the hand cursor: the IP text, the
   open-service tags (.clickable below) and — since clicking anywhere on a
   port-scan result row opens the connection form — every port-table row. */
.scan-table :deep(.el-table__row) {
  transition: background 0.1s ease;
}
.port-table :deep(.el-table__row) {
  cursor: pointer;
}
.scan-table :deep(.el-table__row:hover > td.el-table__cell) {
  background: var(--bg-hover);
}
.scan-table :deep(.el-table__row.current-row > td.el-table__cell) {
  background: var(--accent-subtle);
}

.host-ip,
.port-num-text {
  font-family: var(--font-mono);
  font-weight: 600;
}
.host-ip { color: var(--accent); user-select: text; }
.port-num-text { color: var(--success); }

/* Clickable cells (IP text, open-service checks) invite a click. */
.clickable { cursor: pointer; }
.clickable:hover { filter: brightness(1.2); }

/* Service cells: a green circle-check icon = port open (click to open the
   connection form); dim dash pair = closed. */
.svc-open-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--success);
  cursor: pointer;
  vertical-align: middle;
  transition: filter 0.12s ease;
}
.svc-open-btn:hover {
  filter: brightness(1.25);
}
/* style.css normalises all buttons to the arrow cursor (native-style cursors,
   !important); this one acts as a link-like affordance, so beat it explicitly. */
.scan-table :deep(.svc-open-btn:not(:disabled)) {
  cursor: pointer !important;
}
.svc-closed {
  color: var(--text-muted);
  font-size: 0.75rem;
}

.scan-table :deep(.el-table__empty-block) { background: var(--bg-elevated); }
.scan-table :deep(.el-table__empty-text) { color: var(--text-muted); font-size: 0.75rem; }

/* Open/closed status tags: subtle, tinted like the editor's status colors. */
.scan-table :deep(.el-tag) {
  border: none;
  font-size: 0.6875rem;
  font-weight: 500;
}
.scan-table :deep(.el-tag--success) {
  background: var(--success-subtle);
  color: var(--success);
}
.scan-table :deep(.el-tag--info) {
  background: var(--accent-subtle);
  color: var(--text-muted);
}

.scan-hint {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 1rem;
  color: var(--text-muted);
  font-size: 0.75rem;
  pointer-events: none;
}

/* ── Right-click export context menu (teleported to <body>) ── */
.ctx-menu {
  position: fixed;
  z-index: 99999;
  list-style: none;
  margin: 0;
  padding: 0.25rem;
  min-width: 9rem;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  box-shadow: 0 0.375rem 1.25rem rgba(0, 0, 0, 0.25);
  font-family: var(--font-ui);
}
.ctx-item {
  display: flex;
  align-items: center;
  padding: 0.375rem 0.625rem;
  border-radius: calc(var(--radius-sm) * 0.6);
  font-size: 0.75rem;
  color: var(--text-primary);
  cursor: pointer;
  white-space: nowrap;
  user-select: none;
}
.ctx-item:hover {
  background: var(--accent-subtle);
  color: var(--accent);
}

</style>
