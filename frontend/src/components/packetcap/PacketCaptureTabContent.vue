<template>
  <el-dialog
    append-to-body
    :model-value="visible"
    :title="t('packetcap.title')"
    width="88%"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    destroy-on-close
    class="packetcap-dialog"
    style="display:flex; flex-direction:column; height:90vh; max-height:90vh; border-radius:var(--radius-sm);"
    @update:model-value="(v: boolean) => {
      if (!v) handleDialogClose()
      emit('update:visible', v)
    }"
  >
    <div class="packetcap-content">
      <!-- ── Control toolbar: local/remote toggle, interface selector, filter, start/stop, export ── -->
      <div class="packet-toolbar">
        <el-segmented
          v-model="captureMode"
          class="mode-seg"
          :options="modeOptions"
          :disabled="capturing"
        />

        <template v-if="captureMode === 'local'">
          <span class="ctl-label">{{ t('packetcap.interface') }}</span>
          <el-select
            v-model="selectedInterface"
            class="iface-select"
            :disabled="capturing || loadingInterfaces"
            :placeholder="t('packetcap.ifacePlaceholder')"
            filterable
          >
            <el-option
              v-for="iface in interfaces"
              :key="iface.name"
              :value="iface.name"
              :label="`${iface.name}  (${iface.ip})`"
            >
              <div class="iface-opt">
                <span class="iface-name">{{ iface.name }} ({{ iface.ip }})</span>
                <span class="iface-meta">
                  <span v-if="iface.hardwareAddr" class="iface-mac">{{ iface.hardwareAddr }}</span>
                  <span v-if="iface.flags" class="iface-flags">{{ iface.flags }}</span>
                </span>
              </div>
            </el-option>
          </el-select>
          <el-button class="flat-btn" :disabled="capturing || loadingInterfaces" @click="refreshInterfaces">
            {{ t('packetcap.refresh') }}
          </el-button>
        </template>

        <template v-else>
          <span class="ctl-label">{{ t('packetcap.remoteHost') }}</span>
          <el-select
            v-model="selectedRemoteConn"
            class="remote-host-select"
            :disabled="capturing"
            :placeholder="t('packetcap.remoteHostPlaceholder')"
            filterable
            @change="onRemoteConnChange"
          >
            <el-option
              v-for="conn in remoteConnections"
              :key="conn.id"
              :value="conn.id"
              :label="`${conn.name}  (${conn.user}@${conn.host})`"
            />
          </el-select>

          <span class="ctl-label">{{ t('packetcap.remoteIface') }}</span>
          <el-select
            v-model="selectedRemoteIface"
            class="remote-iface-select"
            :disabled="capturing || loadingRemoteInterfaces"
            :placeholder="t('packetcap.remoteIfacePlaceholder')"
            filterable
            @click="ensureRemoteInterfaces"
          >
            <el-option
              v-for="ri in remoteInterfaces"
              :key="ri.name"
              :value="ri.name"
              :label="ri.name === 'any' ? `${ri.name}  (${t('packetcap.remoteAllIfaces')})` : `${ri.name}  (${ri.ip || t('packetcap.remoteNoIp')})`"
            />
          </el-select>
          <el-button class="flat-btn" :disabled="capturing || loadingRemoteInterfaces" @click="refreshRemoteInterfaces">
            {{ t('packetcap.refresh') }}
          </el-button>
        </template>

        <span class="ctl-label filter-ctl-label">{{ t('packetcap.filter') }}</span>
        <el-tooltip :content="t('packetcap.filterHint')" placement="bottom" :show-after="200">
          <span class="filter-help">?</span>
        </el-tooltip>
        <el-input
          v-model="filter"
          class="filter-input"
          :disabled="capturing"
          :placeholder="t('packetcap.filterPlaceholder')"
          clearable
        ></el-input>

        <el-button
          v-if="!capturing"
          type="primary"
          class="go-btn"
          :disabled="busy || !canStart"
          :loading="busy"
          @click="startCapture"
        >
          {{ t('packetcap.start') }}
        </el-button>
        <el-button v-else type="danger" class="go-btn" @click="stopCapture">
          {{ t('packetcap.stop') }}
        </el-button>

        <div class="spacer"></div>

        <el-button class="flat-btn" :disabled="packets.length === 0" @click="clearPackets">
          {{ t('packetcap.clear') }}
        </el-button>
        <el-dropdown trigger="click" :disabled="packets.length === 0" @command="handleExport">
          <el-button class="flat-btn" :disabled="packets.length === 0">
            {{ t('packetcap.export') }}<span class="export-caret">▾</span>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="csv">.csv</el-dropdown-item>
              <el-dropdown-item command="pcap">.pcap</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>

      <!-- ── Live status bar ── -->
      <div class="packet-statusbar">
        <span class="status-count">{{ t('packetcap.captured') }}: {{ packets.length }}</span>
        <span v-if="capturing" class="status-capturing">
          <span class="pulse-dot"></span>{{ t('packetcap.running') }}
        </span>
        <span v-else-if="stopReason" class="status-stopped">{{ t('packetcap.stopped') }} ({{ stopReason }})</span>
      </div>

      <!-- ── Packet table + raw hex side panel ── -->
      <div class="packet-body">
        <div class="table-wrap">
          <el-table
            ref="packetTableRef"
            :data="packets"
            size="small"
            border
            height="100%"
            class="packet-table"
            highlight-current-row
            @row-click="onRowClick"
          >
            <el-table-column prop="time" :label="t('packetcap.columns.time')" :width="uiPx(150)" align="center">
              <template #default="{ row }"><span class="mono-nowrap">{{ row.time }}</span></template>
            </el-table-column>
            <el-table-column prop="protocol" :label="t('packetcap.columns.protocol')" :width="uiPx(90)" align="center">
              <template #default="{ row }">
                <el-tag :type="protoTagType(row.protocol)" size="small">{{ row.protocol }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column :label="t('packetcap.columns.source')" :width="uiPx(180)" align="center">
              <template #default="{ row }">
                <span class="mono-nowrap">{{ row.sourceIP }}<span v-if="row.sourcePort">:{{ row.sourcePort }}</span></span>
              </template>
            </el-table-column>
            <el-table-column :label="t('packetcap.columns.dest')" :width="uiPx(180)" align="center">
              <template #default="{ row }">
                <span class="mono-nowrap">{{ row.destIP }}<span v-if="row.destPort">:{{ row.destPort }}</span></span>
              </template>
            </el-table-column>
            <el-table-column prop="service" :label="t('packetcap.columns.service')" :width="uiPx(120)" align="center" show-overflow-tooltip>
              <template #default="{ row }">{{ row.service || '-' }}</template>
            </el-table-column>
            <el-table-column prop="flags" :label="t('packetcap.columns.flags')" :width="uiPx(90)" align="center">
              <template #default="{ row }">
                <span v-if="row.flags" class="mono-nowrap flags-text">{{ row.flags }}</span>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column prop="len" :label="t('packetcap.columns.len')" :width="uiPx(80)" align="center" sortable>
              <template #default="{ row }"><span class="mono-nowrap">{{ row.len }}</span></template>
            </el-table-column>
          </el-table>
          <div v-if="packets.length === 0 && !capturing" class="empty-hint">
            {{ captureMode === 'remote' ? t('packetcap.emptyHintRemote') : t('packetcap.emptyHint') }}
          </div>
        </div>

        <!-- Draggable divider between the table and the raw hex panel -->
        <div
          v-if="selectedPacket"
          class="split-drag"
          @mousedown="startDrag"
        ></div>

        <!-- ── Raw hex view of the selected packet ── -->
        <div v-if="selectedPacket" class="hex-panel" :style="{ width: hexPanelW }">
          <div class="hex-header">
            <span class="hex-title">{{ t('packetcap.rawHex') }}</span>
            <el-button class="flat-btn hex-close" size="small" @click="selectedPacket = null">✕</el-button>
          </div>
          <!-- Hover shows the full packet info only when the UDP line is truncated -->
          <el-tooltip :content="metaFull" placement="top" :show-after="200" :disabled="!metaOverflowed">
            <div ref="metaRef" class="hex-meta mono-nowrap">
              {{ metaShort }}
            </div>
          </el-tooltip>
          <!-- Single Wireshark-style dump: offset + hex + ascii on every row -->
          <div class="hex-section" :style="{ flex: hexFlex }">
            <div class="hex-dump" :ref="(el: any) => (hexDumpEl = el)">{{ hexDump }}</div>
          </div>
          <!-- Draggable divider between the hex dump and the protocol tree -->
          <div v-if="treeVisible" class="tree-split-drag" @mousedown="startTreeDrag"></div>
          <!-- Protocol tree (Wireshark-style decoded fields) — bottom, collapsed by default -->
          <div class="tree-section" :style="{ flex: treeFlex }">
            <div class="tree-header" @click="toggleTree">
              <span class="tree-caret" :class="{ open: treeVisible }"></span>
              <span class="tree-title">{{ t('packetcap.detail') }}</span>
              <span class="tree-count" v-if="selectedPacket?.fields?.length">({{ selectedPacket.fields.length }})</span>
            </div>
            <div v-if="treeVisible" class="tree-body">
              <FieldTree v-if="selectedPacket?.fields?.length" :nodes="selectedPacket.fields" :show-info="true" />
              <span v-else class="tree-empty">{{ t('packetcap.detailEmpty') }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { Events } from '@wailsio/runtime'
import { ListCaptureInterfaces, StartCapture, StartRemoteCapture, StopCapture, ListRemoteInterfaces, SaveFileDialogFiltered, WriteFileBase64 } from '../../../bindings/github.com/ys-ll/uniterm/app'
import { msg } from '../../services/message'
import { useI18n } from '../../i18n'
import { uiPx } from '../../utils/uiScale'
import { utf8ToBase64 } from '../../utils/base64'
import { useConnectionStore } from '../../stores/connectionStore'
import { useRemoteCaptureCredentials } from '../../composables/useRemoteCaptureCredentials'
import FieldTree from './FieldTree.vue'
import type { FieldTreeNode } from './FieldTree.vue'

defineOptions({ name: 'PacketCaptureTabContent' })

const props = defineProps<{ visible?: boolean }>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
}>()

const { t } = useI18n()

// ── Mode toggle: local vs remote (SSH tcpdump) ──
const captureMode = ref<'local' | 'remote'>('local')
const modeOptions = [
  { label: t('packetcap.modeLocal'), value: 'local' },
  { label: t('packetcap.modeRemote'), value: 'remote' }
]

// ── Remote host / interface selection ──
const connectionStore = useConnectionStore()
const { resolveRemoteCaptureCredentials, invalidRemoteCaptureCredentials } = useRemoteCaptureCredentials()

// 判断是否为 SSH 认证失败：此时应清空该主机的会话账密缓存并重新弹框，
// 而不是反复复用错误的凭据。
function isSshAuthError(e: any): boolean {
  const m = String(e?.message || e || '')
  return /unable to authenticate|handshake failed|authentication failed|no supported methods|permission denied/i.test(m)
}

// 带“认证失败重试”的凭据解析 + 后端调用：密码输错时清缓存、重新弹框后再试，
// 最多重试 3 次；用户取消或非认证错误则中止并抛出。
async function withSshAuthRetry<T>(
  connId: string,
  op: (cred: { user: string; password: string }) => Promise<T>
): Promise<T> {
  for (let attempt = 0; attempt < 3; attempt++) {
    const cred = await resolveRemoteCaptureCredentials(connId)
    if (cred === null) throw new Error('cancel')
    try {
      return await op(cred)
    } catch (e: any) {
      if (isSshAuthError(e)) {
        // 认证失败：让缓存失效，下一轮循环会重新弹框输入正确账密。
        invalidRemoteCaptureCredentials(connId)
        continue
      }
      throw e
    }
  }
  // 重试 3 次仍失败（用户反复输错或主机持续拒绝），抛出最后一次错误。
  throw new Error('auth-retry-exhausted')
}
const remoteConnections = computed(() => connectionStore.connections.filter(c => c.type === 'ssh'))
const selectedRemoteConn = ref('')
const remoteInterfaces = ref<CaptureInterface[]>([])
const selectedRemoteIface = ref('')
const loadingRemoteInterfaces = ref(false)

const canStart = computed(() => {
  if (busy.value) return false
  if (captureMode.value === 'remote') return !!selectedRemoteConn.value && !!selectedRemoteIface.value
  return !!selectedInterface.value
})

// 远程模式：切换主机时重置远程网卡列表，并预解析账密（未保存密码则弹框）。
async function onRemoteConnChange() {
  selectedRemoteIface.value = ''
  remoteInterfaces.value = []
  await listRemoteInterfaces()
}

// 点开远程网卡下拉时若尚未加载，触发一次枚举。
async function ensureRemoteInterfaces() {
  if (remoteInterfaces.value.length === 0 && selectedRemoteConn.value) {
    await listRemoteInterfaces()
  }
}

function refreshRemoteInterfaces() {
  listRemoteInterfaces()
}

// 经 SSH 枚举远程网卡：先取账密（未保存则弹一次性框、输错自动重试重弹），
// 再调后端；失败提示。
async function listRemoteInterfaces() {
  const connId = selectedRemoteConn.value
  if (!connId) return
  loadingRemoteInterfaces.value = true
  try {
    const list = await withSshAuthRetry(connId, (cred) => ListRemoteInterfaces(connId, cred.user, cred.password))
    // 合成一个 “any（所有网卡）” 选项放最前，供 tcpdump -i any 使用。
    remoteInterfaces.value = [{ name: 'any', ip: '', hardwareAddr: '', flags: '' }, ...(list || [])]
    if (selectedRemoteIface.value && !remoteInterfaces.value.some(ri => ri.name === selectedRemoteIface.value)) {
      selectedRemoteIface.value = ''
    }
  } catch (e: any) {
    if (e?.message === 'cancel') return // 用户取消账密弹框，静默返回
    msg.error(e?.message || t('packetcap.remoteIfaceFailed'))
  } finally {
    loadingRemoteInterfaces.value = false
  }
}


interface CaptureInterface {
  name: string
  ip: string
  hardwareAddr: string
  flags: string
}

interface Packet {
  time: string
  protocol: string
  network: string
  sourceIP: string
  sourcePort: number
  destIP: string
  destPort: number
  service: string
  flags: string
  len: number
  data: string
  fields?: FieldTreeNode[]
}

// ── Interface selection ──
const interfaces = ref<CaptureInterface[]>([])
const selectedInterface = ref('')
const loadingInterfaces = ref(false)

async function loadInterfaces() {
  loadingInterfaces.value = true
  try {
    const list = await ListCaptureInterfaces()
    interfaces.value = list || []
    // Keep a valid selection if it still exists, otherwise default to the first.
    if (selectedInterface.value && interfaces.value.some(i => i.name === selectedInterface.value)) {
      // keep
    } else if (interfaces.value.length > 0) {
      selectedInterface.value = interfaces.value[0].name
    } else {
      selectedInterface.value = ''
    }
  } catch (e: any) {
    msg.error(e?.message || t('packetcap.loadFailed'))
  } finally {
    loadingInterfaces.value = false
  }
}

function refreshInterfaces() {
  loadInterfaces()
}

const selectedIface = computed(() => interfaces.value.find(i => i.name === selectedInterface.value))

// ── Capture state ──
const filter = ref('')
const capturing = ref(false)
const busy = ref(false)
const captureId = ref('')
const packets = ref<Packet[]>([])
const selectedPacket = ref<Packet | null>(null)
const stopReason = ref('')
const MAX_BUFFER = 5000

// Only packets belonging to the capture this component started are kept, since
// packetcap events are global and other instances may be active.
const owned = new Set<string>()

async function startCapture() {
  busy.value = true
  // Show the stopped/error banner only while actually capturing; reset on start.
  stopReason.value = ''
  try {
    if (captureMode.value === 'remote') {
      await startRemoteCapture()
    } else {
      await startLocalCapture()
    }
  } catch (e: any) {
    msg.error(e?.message || t('packetcap.startFailed'))
  } finally {
    busy.value = false
  }
}

async function startLocalCapture() {
  const iface = selectedIface.value
  if (!iface) {
    msg.warning(t('packetcap.noInterface'))
    return
  }
  const id = await StartCapture({
    interfaceIP: iface.ip,
    interfaceName: iface.name,
    filter: filter.value.trim(),
    maxPackets: 0
  })
  owned.add(id)
  captureId.value = id
  capturing.value = true
  packets.value = []
  selectedPacket.value = null
}

async function startRemoteCapture() {
  const connId = selectedRemoteConn.value
  if (!connId) {
    msg.warning(t('packetcap.noRemoteHost'))
    return
  }
  if (!selectedRemoteIface.value) {
    msg.warning(t('packetcap.noRemoteIface'))
    return
  }
  // 取账密（本会话已缓存则跳过弹框、输错自动清缓存重弹）；用户取消则中止。
  let id: string
  try {
    id = await withSshAuthRetry(connId, (cred) =>
      StartRemoteCapture(
        {
          remote: true,
          remoteConnId: connId,
          remoteIface: selectedRemoteIface.value,
          interfaceIP: '',
          interfaceName: '',
          filter: filter.value.trim(),
          maxPackets: 0
        },
        cred.user,
        cred.password
      )
    )
  } catch (e: any) {
    if (e?.message === 'cancel') return // 用户取消账密弹框，静默返回
    if (e?.message === 'auth-retry-exhausted') {
      msg.error(t('packetcap.remoteAuthFailed'))
      return
    }
    throw e
  }
  owned.add(id)
  captureId.value = id
  capturing.value = true
  packets.value = []
  selectedPacket.value = null
}

function stopCapture() {
  const id = captureId.value
  if (!id) return
  // Reset the UI synchronously FIRST — do not wait on the StopCapture binding
  // to resolve, because under heavy packet load the binding / event channel can
  // be delayed and the button would stay stuck. Tell the backend to stop in the
  // background (fire-and-forget); backend Stop is idempotent and synchronous,
  // so even a late call is safe.
  forcedStop.value = false
  capturing.value = false
  captureId.value = ''
  pendingPackets.length = 0 // 丢弃停止前仍积压在缓冲里的包，避免停止后表格继续增长
  StopCapture(id).catch(() => {})
}

// Closing the dialog must reliably stop any running capture. This is wired to
// el-dialog's @update:model-value(false) so it runs while the component is
// STILL MOUNTED — unlike onUnmounted, where the fire-and-forget backend call can
// be dropped when the WebView context is torn down, leaving the elevated worker
// running in the background (which then stalls the next open until it is killed).
function handleDialogClose() {
  const id = captureId.value
  if (!id) return
  forcedStop.value = false
  capturing.value = false
  captureId.value = ''
  pendingPackets.length = 0
  StopCapture(id).catch(() => {})
}

// True when the user pressed stop (vs. an event-driven end) — used only to avoid
// re-triggering a backend stop in response to a stopped event we just caused.
const forcedStop = ref(false)

// ── Live packet handling ──
// 高频逐包渲染是界面卡死 / 停止按钮失效的根源：每来一包都对 el-table 做一次
// 全量重渲染 + 滚底。这里把收包与渲染解耦——事件回调只把包塞进 pending 缓冲，
// 再由一个 ~60ms 的定时器批量 flush，把每包的 DOM 工作摊平成低频批量更新。
// 这样即使后端瞬时涌入上千包，主线程也不会被逐包重渲染卡住，停止/关闭按钮
// 始终能响应。
const pendingPackets: Packet[] = []
let flushTimer: ReturnType<typeof setInterval> | null = null

// flushPendingPackets 把 pending 缓冲一口气写入响应式 packets，并执行缓冲截断，
// 然后（节流地）滚底。stop 后清空 pending 缓冲，避免残留包继续填充表格。
function flushPendingPackets() {
  if (pendingPackets.length === 0) return
  const batch = pendingPackets.splice(0, pendingPackets.length)
  // 维护“需要显示的总顺序”，先追加再统一截断，保证 MAX_BUFFER 语义不变。
  packets.value.push(...batch)
  if (packets.value.length > MAX_BUFFER) {
    packets.value.splice(0, packets.value.length - MAX_BUFFER)
  }
  autoScrollToBottom()
}

function onPacketEvent(ev: any) {
  const d = ev?.data
  if (!d || !owned.has(d.captureId)) return
  const p: Packet = d.packet
  if (!p) return
  // Drop packets that arrive after we stopped / for a capture that is no longer
  // active. On stop, captureId is cleared, so the many packet events that were
  // already queued in the event channel while the capture ran get discarded
  // here instead of continuing to fill the table after the UI shows "stopped".
  if (d.captureId !== captureId.value) return
  pendingPackets.push(p)
  // 仅当蜂拥时批量 flush；单个事件不立刻触发 DOM 更新，交由定时器汇总。
  if (pendingPackets.length >= 200) {
    flushPendingPackets()
  }
}

function onStoppedEvent(ev: any) {
  const d = ev?.data
  console.log('[pktcap] stopped event', JSON.stringify(d), 'captureId=', captureId.value, 'forcedStop=', forcedStop.value, 'owned=', [...owned])
  if (!d || !owned.has(d.captureId)) {
    // Fallback: if this stopped event matches the capture we are actively
    // showing (but somehow isn't in `owned`), still finalise the UI so the
    // stop button is never left stuck.
    if (!(d && (d.captureId === captureId.value || captureId.value === ''))) return
  }
  owned.delete(d.captureId)
  if (d.reason === 'manual') {
    stopReason.value = t('packetcap.reasonManual')
  } else if (d.reason === 'limit') {
    stopReason.value = `${t('packetcap.reasonLimit')} (${d.count})`
  } else if (d.reason === 'error') {
    stopReason.value = t('packetcap.reasonError')
  }
  if (!forcedStop.value && captureId.value) {
    // Backend stopped on its own (limit/error) — tell it to finalize.
    StopCapture(captureId.value).catch(() => {})
  }
  forcedStop.value = false
  capturing.value = false
  captureId.value = ''
  // 抓包结束后，把缓冲里最后一批包刷进表格再清空，保证计数完整且无残留。
  flushPendingPackets()
  pendingPackets.length = 0
}

function onErrorEvent(ev: any) {
  const d = ev?.data
  if (!d || !owned.has(d.captureId)) return
  msg.error(d.message || t('packetcap.captureError'))
}

function onInterfaceErrorEvent(ev: any) {
  const d = ev?.data
  if (!d || !owned.has(d.captureId)) return
  msg.error(`${d.name || ''}: ${d.message || t('packetcap.ifaceError')}`.trim())
}

function clearPackets() {
  packets.value = []
  selectedPacket.value = null
}

// ── Table / selection ──
const packetTableRef = ref<any>(null)
function onRowClick(row: Packet) {
  selectedPacket.value = row
}

// ── Hex panel: resizable width + hover full info ──
// 面板宽度钳制常量。CSS 中 .hex-panel 为 width:22rem; min-width:16rem
// （根字号 16px → 352px / 256px），JS 与其保持一致，避免单位换算死区。
const DEFAULT_HEX_W_PX = 352
const MIN_HEX_W_PX = 256
// 默认值用 px；若用户机器上残留旧的 rem 值，startDrag 会改用 DOM 实测宽度兜底。
const hexPanelW = ref(localStorage.getItem('pktcap.hexPanelWidth') || DEFAULT_HEX_W_PX + 'px')
const metaFull = computed(() => {
  const p = selectedPacket.value
  if (!p) return ''
  return `${p.protocol} · ${p.sourceIP}:${p.sourcePort || '-'} → ${p.destIP}:${p.destPort || '-'} · ${p.len} B`
})
const metaShort = computed(() => metaFull.value)

// The hover tooltip for the info line should only appear when the line is
// actually truncated (showing an ellipsis). Detect overflow via scroll size.
const metaRef = ref<HTMLElement | null>(null)
const metaOverflowed = ref(false)
function checkMetaOverflow() {
  nextTick(() => {
    const el = metaRef.value
    metaOverflowed.value = !!el && el.scrollWidth > el.clientWidth + 1
  })
}

// Drag the divider between the table and the hex panel to resize the width.
// 流畅 + 跟手：mousemove 只记录最新 x，由下一帧的 requestAnimationFrame 用最新值
// 渲染一次（不丢弃事件、每帧只渲染一次）；localStorage 仅在 mouseup 时写一次。
// 起始宽度取 DOM 实测像素（而非解析 localStorage 字符串），避免坏值/单位混淆
// 与钳制下限叠加造成「拖一段/完全不动」的非线性死区。
function startDrag(e: MouseEvent) {
  e.preventDefault()
  const body = document.body
  const startX = e.clientX
  // 同步阶段缓存 DOM 引用（rAF 异步回调里 e.currentTarget 已失效，故不可用）
  const container = (e.currentTarget as HTMLElement).parentElement
  const panel = (e.currentTarget as HTMLElement).nextElementSibling as HTMLElement | null
  // 起始宽度取 DOM 实测像素（而非解析 localStorage 字符串），避免坏值/单位混淆
  // 与钳制下限叠加造成「拖一段/完全不动」的非线性死区。
  const startW =
    (panel && panel.getBoundingClientRect().width) || parseFloat(hexPanelW.value) || MIN_HEX_W_PX
  let latestX = e.clientX
  let raf = 0
  const render = () => {
    raf = 0
    const delta = startX - latestX
    let w = startW + delta // px
    // 钳制范围与 CSS `.hex-panel` 的 min-width 对齐；上限相对容器取 80%。
    const maxW = container ? Math.max(MIN_HEX_W_PX, Math.floor(container.clientWidth * 0.8)) : 1200
    w = Math.max(MIN_HEX_W_PX, Math.min(Math.round(w), maxW))
    hexPanelW.value = `${w}px`
  }
  const schedule = () => {
    if (!raf) raf = requestAnimationFrame(render)
  }
  const onMove = (ev: MouseEvent) => {
    latestX = ev.clientX
    schedule()
  }
  const onUp = () => {
    if (raf) { cancelAnimationFrame(raf); raf = 0 }
    body.style.cursor = ''
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    // 拖动结束才落盘，避免每帧写 localStorage
    try { localStorage.setItem('pktcap.hexPanelWidth', hexPanelW.value) } catch {}
  }
  body.style.cursor = 'col-resize'
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}
function autoScrollToBottom() {
  nextTick(() => {
    const table = packetTableRef.value
    const wrap = table?.$el?.querySelector?.('.el-scrollbar__wrap')
    if (wrap) wrap.scrollTop = wrap.scrollHeight
  })
}

// ── Protocol-tree / hex split within the side panel ──
// 协议详情默认折叠（在底部），点击头部标题展开。hex 区占满剩余空间。
const treeVisible = ref(false)
const treeRatio = ref(parseFloat(localStorage.getItem('pktcap.treeRatio') || '0.5'))
function toggleTree() {
  treeVisible.value = !treeVisible.value
}
// flex values: tree grows according to ratio, hex gets the remainder.
const treeFlex = computed(() => (treeVisible.value ? `${treeRatio.value} 1 0` : '0 0 auto'))
const hexFlex = computed(() => (treeVisible.value ? `${1 - treeRatio.value} 1 0` : '1 1 0'))
const hexDumpEl = ref<HTMLElement | null>(null)

// Drag the divider inside the side panel to resize tree vs hex heights.
// 与 startDrag 同理：mousemove 只记录最新 y，rAF 每帧用最新值渲染一次，
// 既流畅（每帧一次）又跟手（不滞后）；localStorage 改为 mouseup 时写一次。
function startTreeDrag(e: MouseEvent) {
  e.preventDefault()
  const startY = e.clientY
  const startRatio = treeRatio.value
  const panel = (e.currentTarget as HTMLElement).parentElement!
  let latestY = e.clientY
  let raf = 0
  const render = () => {
    raf = 0
    const h = panel.clientHeight || 1
    const delta = (latestY - startY) / h
    let r = startRatio + delta
    r = Math.max(0.15, Math.min(Math.round(r * 100) / 100, 0.85))
    treeRatio.value = r
  }
  const schedule = () => {
    if (!raf) raf = requestAnimationFrame(render)
  }
  const onMove = (ev: MouseEvent) => {
    latestY = ev.clientY
    schedule()
  }
  const onUp = () => {
    if (raf) { cancelAnimationFrame(raf); raf = 0 }
    document.body.style.cursor = ''
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    try { localStorage.setItem('pktcap.treeRatio', String(treeRatio.value)) } catch {}
  }
  document.body.style.cursor = 'row-resize'
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

// Re-sync overflow detection of the meta line when selection/panel changes.
watch([selectedPacket, hexPanelW, treeVisible], () => {
  checkMetaOverflow()
})

// ── Raw hex decoding ──
// Wireshark 风格单块显示：每一行显示 偏移量 + hex 字节 + 同行 ASCII，合并为一块。
function buildHexDump(): string {
  const bytes = hexToBytes(selectedPacket.value?.data)
  const lines: string[] = []
  for (let i = 0; i < bytes.length; i += 16) {
    const chunk = bytes.slice(i, i + 16)
    const offset = i.toString(16).padStart(8, '0')
    const hex = chunk.map(b => b.toString(16).padStart(2, '0')).join(' ')
    const ascii = chunk.map(b => (b >= 32 && b <= 126 ? String.fromCharCode(b) : '.')).join('')
    lines.push(`${offset}  ${hex.padEnd(47)}  |${ascii}|`)
  }
  return lines.join('\n')
}

const hexDump = computed(() => buildHexDump())

function hexToBytes(hex?: string): number[] {
  if (!hex) return []
  const clean = hex.replace(/\s+/g, '')
  if (clean.length % 2 !== 0) return []
  const bytes: number[] = []
  for (let i = 0; i < clean.length; i += 2) {
    const b = parseInt(clean.substr(i, 2), 16)
    if (!Number.isNaN(b)) bytes.push(b)
  }
  return bytes
}

function protoTagType(proto: string): 'success' | 'warning' | 'info' | 'danger' {
  const p = (proto || '').toLowerCase()
  if (p === 'tcp') return 'success'
  if (p === 'udp') return 'warning'
  if (p === 'icmp') return 'danger'
  return 'info'
}

// ── Export ──
function csvEscape(cell: unknown): string {
  const s = String(cell ?? '')
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

function buildCsv(): string {
  const header = [
    t('packetcap.columns.time'),
    t('packetcap.columns.protocol'),
    t('packetcap.columns.source'),
    t('packetcap.columns.dest'),
    t('packetcap.columns.service'),
    t('packetcap.columns.flags'),
    t('packetcap.columns.len')
  ]
  const rows = packets.value.map(p => [
    p.time,
    p.protocol,
    p.sourcePort ? `${p.sourceIP}:${p.sourcePort}` : p.sourceIP,
    p.destPort ? `${p.destIP}:${p.destPort}` : p.destIP,
    p.service,
    p.flags,
    p.len
  ])
  const all = [header, ...rows].map(r => r.map(csvEscape).join(','))
  return '﻿' + all.join('\n')
}

async function handleExport(command: string) {
  if (command === 'csv') await exportCsv()
  else if (command === 'pcap') await exportPcap()
}

async function exportCsv() {
  const now = new Date()
  const stamp = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}_${String(now.getHours()).padStart(2, '0')}-${String(now.getMinutes()).padStart(2, '0')}-${String(now.getSeconds()).padStart(2, '0')}`
  const fileName = `packetcap_${stamp}.csv`
  try {
    const path = await SaveFileDialogFiltered(t('packetcap.exportTitle'), fileName, t('packetcap.filterCsv'), '*.csv')
    if (!path) return
    await WriteFileBase64(path, utf8ToBase64(buildCsv()))
    msg.success(t('packetcap.exportSaved'))
  } catch (e: any) {
    msg.error(`${t('packetcap.exportFailed')} ${e?.message ?? ''}`)
  }
}

async function exportPcap() {
  const now = new Date()
  const stamp = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}_${String(now.getHours()).padStart(2, '0')}-${String(now.getMinutes()).padStart(2, '0')}-${String(now.getSeconds()).padStart(2, '0')}`
  const fileName = `packetcap_${stamp}.pcap`

  // Global header: little-endian pcap, version 2.4, network type 1 (Ethernet).
  const header = new Uint8Array(24)
  const dv = new DataView(header.buffer)
  // magic number 0xa1b2c3d4 little-endian = d4 c3 b2 a1
  dv.setUint8(0, 0xd4); dv.setUint8(1, 0xc3); dv.setUint8(2, 0xb2); dv.setUint8(3, 0xa1)
  dv.setUint16(4, 2, true)  // version_major
  dv.setUint16(6, 4, true)  // version_minor
  dv.setInt32(8, 0, true)   // thiszone
  dv.setUint32(12, 0, true) // sigfigs
  dv.setUint32(16, 65535, true) // snaplen
  dv.setUint32(20, 1, true) // network = Ethernet

  const records: Uint8Array[] = []
  for (const p of packets.value) {
    const ipBytes = hexToBytes(p.data)
    // The backend hands us the parsed IP datagram without the Ethernet header
    // (Windows raw socket). Prepend a synthetic 14-byte Ethernet header so
    // Wireshark parses the file: dst/src MAC zeros + ethertype (0x0800 IPv4,
    // 0x86dd IPv6) based on the packet's protocol.
    const ethertype = (p.protocol || '').toLowerCase() === 'ipv6' ? 0x86dd : 0x0800
    const eth = new Uint8Array(14)
    const edv = new DataView(eth.buffer)
    edv.setUint16(12, ethertype, false) // ethertype is network-order

    const packet = new Uint8Array(eth.length + ipBytes.length)
    packet.set(eth, 0)
    packet.set(ipBytes, eth.length)

    const rec = new Uint8Array(16 + packet.length)
    const rdv = new DataView(rec.buffer)
    let tsSec = Math.floor(Date.now() / 1000)
    let tsUsec = (Date.now() % 1000) * 1000
    if (p.time) {
      const parsed = new Date(p.time)
      if (!Number.isNaN(parsed.getTime())) {
        tsSec = Math.floor(parsed.getTime() / 1000)
        tsUsec = (parsed.getTime() % 1000) * 1000
      }
    }
    rdv.setUint32(0, tsSec, true)
    rdv.setUint32(4, tsUsec, true)
    rdv.setUint32(8, packet.length, true) // incl_len
    rdv.setUint32(12, packet.length, true) // orig_len
    rec.set(packet, 16)
    records.push(rec)
  }

  // Concatenate everything into a single byte buffer, then base64-encode.
  const total = header.length + records.reduce((n, r) => n + r.length, 0)
  const buf = new Uint8Array(total)
  buf.set(header, 0)
  let off = header.length
  for (const r of records) {
    buf.set(r, off)
    off += r.length
  }
  const b64 = bytesToBase64(buf)

  try {
    const path = await SaveFileDialogFiltered(t('packetcap.exportTitle'), fileName, t('packetcap.filterPcap'), '*.pcap')
    if (!path) return
    await WriteFileBase64(path, b64)
    msg.success(t('packetcap.exportSaved'))
  } catch (e: any) {
    msg.error(`${t('packetcap.exportFailed')} ${e?.message ?? ''}`)
  }
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  const CHUNK = 0x2000
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode.apply(null, bytes.subarray(i, Math.min(i + CHUNK, bytes.length)) as unknown as number[])
  }
  return btoa(binary)
}

// ── Lifecycle ──
let unsubs: (() => void)[] = []

onMounted(() => {
  loadInterfaces()
  window.addEventListener('resize', checkMetaOverflow)
  unsubs = [
    Events.On('packetcap:packet', (ev: any) => onPacketEvent(ev)),
    Events.On('packetcap:stopped', (ev: any) => onStoppedEvent(ev)),
    Events.On('packetcap:error', (ev: any) => onErrorEvent(ev)),
    Events.On('packetcap:interface-error', (ev: any) => onInterfaceErrorEvent(ev))
  ]
  // 批量渲染定时器：每 60ms 把积压的包一次性写入表格。始终运行，但无包时
  // flushPendingPackets 立即返回，开销可忽略；不被“是否在抓包”直接耦合，
  // 避免停止瞬间的残留包卡在缓冲里。
  flushTimer = setInterval(flushPendingPackets, 60)
})

onUnmounted(() => {
  if (flushTimer) {
    clearInterval(flushTimer)
    flushTimer = null
  }
  pendingPackets.length = 0
  // Closing the dialog must always stop any running capture. StopCapture is a
  // safe no-op for unknown ids, so call it unconditionally.
  if (captureId.value) StopCapture(captureId.value).catch(() => {})
  window.removeEventListener('resize', checkMetaOverflow)
  unsubs.forEach(u => u())
  owned.clear()
})

// Reload interfaces each time the dialog (re)opens — destroy-on-close remounts
// the component, so this runs on every open as a fresh mount.
watch(() => props.visible, (v) => {
  if (v) {
    // Fresh interface list in case the user changed adapters since last open.
    loadInterfaces()
  }
}, { immediate: true })
</script>

<style scoped>
.packetcap-dialog {
  display: flex;
  flex-direction: column;
  height: 90vh;
  max-height: 90vh;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
}
.packetcap-dialog :deep(.el-dialog__header) {
  padding: 0.625rem 1rem;
  border-bottom: 1px solid var(--border-subtle);
  margin: 0;
  background: var(--bg-elevated);
  flex-shrink: 0;
}
.packetcap-dialog :deep(.el-dialog__title) {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-ui);
}
.packetcap-dialog :deep(.el-dialog__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.75rem;
  overflow: hidden;
}

.packetcap-content {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: calc(90vh - 7.5rem);
  max-height: calc(90vh - 7.5rem);
  overflow: hidden;
}

/* ── Toolbar ── */
.packet-toolbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  flex-shrink: 0;
  flex-wrap: wrap;
}
.packet-toolbar .el-input,
.packet-toolbar .el-select {
  --el-input-bg-color: var(--bg-base);
  --el-input-border-color: var(--border-subtle);
  --el-input-hover-border-color: var(--accent);
  --el-input-focus-border-color: var(--accent);
  --el-input-text-color: var(--text-primary);
  --el-input-placeholder-color: var(--text-muted);
  --el-input-height: 1.875rem;
  --el-input-border-radius: var(--radius-sm);
}
.packet-toolbar .el-input__wrapper {
  background: var(--bg-base);
  box-shadow: 0 0 0 1px var(--border-subtle) inset;
}
.packet-toolbar .el-input__wrapper:hover {
  box-shadow: 0 0 0 1px var(--bg-hover) inset;
}
.packet-toolbar .el-input__wrapper.is-focus {
  box-shadow: 0 0 0 1px var(--accent) inset;
}
.iface-select { width: 12.5rem; flex-shrink: 0; }
.filter-input { width: 11rem; flex-shrink: 0; }
.mode-seg { margin-right: 0.5rem; flex-shrink: 0; }
/* 去掉 el-segmented 本地/远程切换的 hover 底色变化，避免“灰底白字/白底白字”看不清 */
.mode-seg :deep(.el-segmented__item:hover) {
  background: transparent;
  color: var(--text-primary);
}
.mode-seg :deep(.el-segmented__item.is-selected:hover) {
  background: var(--accent);
  color: #fff;
}
.remote-host-select { width: 12.5rem; flex-shrink: 0; }
.remote-iface-select { width: 10rem; flex-shrink: 0; }

.ctl-label {
  font-size: 0.6875rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  color: var(--text-secondary);
  font-family: var(--font-ui);
  flex-shrink: 0;
}
.filter-ctl-label {
  margin-left: 0.75rem;
}
.filter-help {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1rem;
  height: 1rem;
  border-radius: 50%;
  border: 1px solid var(--text-muted);
  color: var(--text-muted);
  font-size: 0.6875rem;
  margin: 0 0.125rem 0 0;
  cursor: help;
  flex-shrink: 0;
}
.export-caret {
  margin-left: 0.25rem;
  font-size: 0.625rem;
  color: var(--text-secondary);
}
.spacer { flex: 1; }
.packet-toolbar .el-button {
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
.flat-btn.el-button {
  --el-button-bg-color: transparent;
  --el-button-border-color: var(--border-subtle);
  --el-button-text-color: var(--text-secondary);
  --el-button-hover-bg-color: var(--bg-hover);
  --el-button-hover-border-color: var(--border-subtle);
  --el-button-hover-text-color: var(--text-primary);
}

/* ── Option metadata ── */
.iface-opt {
  display: flex;
  flex-direction: column;
  line-height: 1.3;
  padding: 0.1rem 0;
}
.iface-meta {
  display: flex;
  gap: 0.5rem;
  font-size: 0.6875rem;
  color: var(--text-muted);
  font-family: var(--font-mono);
}

/* ── Status bar ── */
.packet-statusbar {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0 0.25rem;
  flex-shrink: 0;
  font-size: 0.75rem;
}
.status-count { color: var(--text-secondary); font-family: var(--font-mono); }
.status-capturing { color: var(--success); display: inline-flex; align-items: center; gap: 0.4rem; }
.status-stopped { color: var(--text-muted); }
.pulse-dot {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 50%;
  background: var(--success);
  animation: pulse 1.2s ease-in-out infinite;
}
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.25; }
}

/* ── Body: table + hex panel ── */
.packet-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: row;
  gap: 0.125rem;
  align-items: stretch;
}
.table-wrap {
  flex: 1;
  min-height: 0;
  min-width: 0;
  position: relative;
  overflow: hidden;
}

.packet-table {
  width: 100%;
  --el-table-border-color: var(--border-subtle);
}
.packet-table :deep(.el-table__inner-wrapper::before),
.packet-table :deep(.el-table__border-left-patch),
.packet-table :deep(.el-table::after),
.packet-table :deep(.el-table__inner-wrapper::after) { background: var(--border-subtle); }
.packet-table :deep(.el-table__body td.el-table__cell),
.packet-table :deep(.el-table__header-wrapper th.el-table__cell) {
  user-select: text;
  -webkit-user-select: text;
}
.packet-table :deep(.el-table__header-wrapper th.el-table__cell) {
  background: var(--bg-elevated);
  color: var(--text-secondary);
  font-size: 0.6875rem;
  font-weight: 600;
  letter-spacing: 0.02em;
  text-transform: uppercase;
  border-bottom: 1px solid var(--border-subtle);
  padding: 0.375rem 0.25rem;
}
.packet-table :deep(.el-table__header-wrapper th.el-table__cell .cell) { padding: 0; }
.packet-table :deep(.el-table__body td.el-table__cell) {
  padding: 0.3125rem 0.25rem;
  font-size: 0.75rem;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-subtle);
}
.packet-table :deep(.el-table__body td.el-table__cell .cell) { padding: 0; }
.packet-table :deep(.el-table__row) { transition: background 0.1s ease; cursor: pointer; }
.packet-table :deep(.el-table__row:hover > td.el-table__cell) { background: var(--bg-hover); }
.packet-table :deep(.el-table__row.current-row > td.el-table__cell) { background: var(--accent-subtle); }
.packet-table :deep(.el-table__empty-block) { background: var(--bg-elevated); }
.packet-table :deep(.el-table__empty-text) { color: var(--text-muted); font-size: 0.75rem; }
.packet-table :deep(.el-tag) {
  border: none;
  font-size: 0.6875rem;
  font-weight: 500;
}
.packet-table :deep(.el-tag--success) { background: var(--success-subtle); color: var(--success); }
.packet-table :deep(.el-tag--warning) { background: var(--warning-subtle); color: var(--warning); }
.packet-table :deep(.el-tag--danger) { background: var(--danger-subtle); color: var(--danger); }
.packet-table :deep(.el-tag--info) { background: var(--accent-subtle); color: var(--text-muted); }

.mono-nowrap {
  font-family: var(--font-mono);
  white-space: nowrap;
}
.flags-text { color: var(--accent); font-weight: 600; }

.empty-hint {
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

/* ── Raw hex side panel ── */
.hex-panel {
  width: 22rem;
  min-width: 16rem;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  overflow: hidden;
  flex-shrink: 0;
}
/* Draggable divider between table and hex panel.
   加宽抓取区域(8px)以提升 mousedown 命中率，视觉分隔线仍为细线居中显示，
   避免拖动条过窄导致“要移一段距离才触发/跟手”。 */
.split-drag {
  flex-shrink: 0;
  width: 0.5rem;
  cursor: col-resize;
  position: relative;
  touch-action: none;
  z-index: 2;
}
.split-drag::before {
  content: "";
  position: absolute;
  top: 0;
  bottom: 0;
  left: 50%;
  transform: translateX(-50%);
  width: 1px;
  background: transparent;
  transition: background 0.15s ease, width 0.15s ease;
}
.split-drag:hover::before {
  background: var(--accent);
  width: 3px;
}
.hex-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.375rem 0.625rem;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}
.hex-title {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-ui);
}
.hex-close { padding: 0.2rem 0.4rem; }
.hex-meta {
  padding: 0.375rem 0.625rem;
  border-bottom: 1px solid var(--border-subtle);
  font-size: 0.6875rem;
  color: var(--text-secondary);
  flex-shrink: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.hex-dump {
  flex: 1;
  margin: 0;
  padding: 0.5rem;
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  line-height: 1.5;
  color: var(--text-primary);
  background: var(--bg-base);
  user-select: text;
  -webkit-user-select: text;
  overflow: auto;
  min-height: 0;
  cursor: default;
  white-space: pre;
}

/* ── Protocol-tree section (Wireshark-style decoded fields) ── */
.tree-section {
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-bottom: 1px solid var(--border-subtle);
}
.tree-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.625rem;
  flex-shrink: 0;
  cursor: pointer;
  user-select: none;
}
.tree-header:hover { background: var(--bg-hover); }
.tree-caret {
  flex-shrink: 0;
  width: 0;
  height: 0;
  border-top: 4px solid transparent;
  border-bottom: 4px solid transparent;
  border-left: 5px solid var(--text-muted);
  transition: transform 0.12s ease;
}
.tree-caret.open { transform: rotate(90deg); }
.tree-title {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-ui);
}
.tree-count { font-size: 0.6875rem; color: var(--text-muted); }
.tree-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0.25rem 0.375rem 0.375rem;
  background: var(--bg-base);
}
.tree-empty {
  display: block;
  padding: 0.5rem;
  color: var(--text-muted);
  font-size: 0.75rem;
}
/* Divider between the protocol tree and the hex dump */
.tree-split-drag {
  flex-shrink: 0;
  height: 0.25rem;
  cursor: row-resize;
  position: relative;
  touch-action: none;
  border-top: 1px dashed var(--border-subtle);
}
.tree-split-drag::before {
  content: "";
  position: absolute;
  left: 0;
  right: 0;
  top: 50%;
  transform: translateY(-50%);
  height: 1px;
  background: transparent;
}
.tree-split-drag:hover::before {
  background: var(--accent);
  height: 3px;
}
/* Hex dump now lives inside a flexible column that shares the panel with tree */
.hex-section {
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--bg-base);
}

</style>