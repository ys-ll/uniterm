<template>
  <div class="monitor-tab">
    <div class="monitor-tabs-header">
      <div class="tab-item" :class="{ active: activeTab === 'performance' }" @click="activeTab = 'performance'">{{ t('monitor.performance') }}</div>
      <div class="tab-item" :class="{ active: activeTab === 'processes' }" @click="activeTab = 'processes'">{{ t('monitor.processes') }}</div>
      <div class="tab-item" :class="{ active: activeTab === 'ports' }" @click="activeTab = 'ports'">{{ t('monitor.ports') }}</div>
      <div class="tab-item" :class="{ active: activeTab === 'disks' }" @click="activeTab = 'disks'">{{ t('monitor.disks') }}</div>
      <div class="tab-item" :class="{ active: activeTab === 'network' }" @click="activeTab = 'network'">{{ t('monitor.networkCards') }}</div>
      <div class="tab-item" :class="{ active: activeTab === 'system' }" @click="activeTab = 'system'">{{ t('monitor.system') }}</div>
    </div>

    <!-- Performance -->
    <div v-show="activeTab === 'performance'" class="tab-pane performance-pane">
      <div class="perf-sidebar">
        <div
          v-for="item in perfItems"
          :key="item.key"
          class="perf-nav-item"
          :class="{ active: selectedPerf === item.key }"
          @click="selectedPerf = item.key"
        >
          <div class="perf-nav-name">{{ item.label }}</div>
          <div class="perf-nav-value" :style="{ color: item.color }">{{ item.value }}</div>
          <div class="perf-nav-bar"><div class="perf-nav-bar-inner" :style="{ width: item.percent + '%', background: item.color }" /></div>
        </div>
      </div>
      <div class="perf-main">
        <div class="perf-big-value" :style="{ color: currentPerf.color }">{{ currentPerf.bigValue }}</div>
        <canvas ref="chartCanvas" class="perf-chart" />
        <div class="perf-details">
          <div v-for="d in currentPerf.details" :key="d.label" class="perf-detail-item">
            <span class="detail-label">{{ d.label }}</span>
            <span class="detail-value">{{ d.value }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Processes -->
    <div v-show="activeTab === 'processes'" class="tab-pane processes-pane">
      <div class="process-toolbar">
        <div class="process-summary">
          <div class="summary-item">
            <span class="summary-label">CPU</span>
            <span class="summary-value">{{ processSummaryCpu.usage }}%</span>
          </div>
          <div class="summary-item">
            <span class="summary-label">{{ t('monitor.memory') }}</span>
            <span class="summary-value">{{ processSummaryMem.usage }}%</span>
          </div>
          <div class="summary-item">
            <span class="summary-label">{{ t('monitor.total') }}</span>
            <span class="summary-value">{{ processSummaryMem.total.toFixed(2) }} GB</span>
          </div>
          <div class="summary-item">
            <span class="summary-label">{{ t('monitor.used') }}</span>
            <span class="summary-value">{{ processSummaryMem.used.toFixed(2) }} GB</span>
          </div>
          <div class="summary-item">
            <span class="summary-label">{{ t('monitor.free') }}</span>
            <span class="summary-value">{{ processSummaryMem.free.toFixed(2) }} GB</span>
          </div>
          <div class="summary-item">
            <span class="summary-label">{{ t('monitor.processCount') }}</span>
            <span class="summary-value">{{ processSummaryCpu.processes }}</span>
          </div>
        </div>
        <div class="process-actions">
          <el-button :type="paused ? 'primary' : 'default'" @click="togglePause">
            {{ paused ? t('monitor.resume') : t('monitor.pause') }}
          </el-button>
        </div>
      </div>
      <el-input v-model="processSearch" :placeholder="t('monitor.searchProcess')" clearable class="process-search" />
      <el-table :data="filteredProcesses" size="small" height="calc(100% - 40px)" class="process-table" @row-click="onProcessRowClick">
        <el-table-column prop="pid" label="PID" sortable width="80" />
        <el-table-column prop="name" :label="t('monitor.processName')" sortable />
        <el-table-column prop="user" :label="t('monitor.user')" sortable width="100" />
        <el-table-column prop="state" :label="t('monitor.state')" sortable width="80">
          <template #default="{ row }">{{ row.state ? String(row.state)[0] : '-' }}</template>
        </el-table-column>
        <el-table-column prop="cpu" :label="t('monitor.cpu')" sortable width="90">
          <template #default="{ row }">{{ row.cpu }}%</template>
        </el-table-column>
        <el-table-column prop="mem" :label="t('monitor.mem')" sortable width="90">
          <template #default="{ row }">{{ row.mem }}%</template>
        </el-table-column>
      </el-table>
    </div>

    <!-- Ports -->
    <div v-show="activeTab === 'ports'" class="tab-pane ports-pane" @contextmenu.prevent="showContextMenu($event)">
      <div class="od-toolbar">
        <el-input v-model="portSearch" :placeholder="t('monitor.searchPort')" clearable class="od-search" />
        <el-button :icon="RefreshRight" :loading="loadingPorts" @click="fetchPorts">
          {{ t('monitor.refresh') }}
        </el-button>
      </div>
      <el-table :data="filteredPorts" size="small" v-loading="loadingPorts" height="calc(100% - 36px)" class="od-table">
        <el-table-column prop="protocol" :label="t('monitor.port.protocol')" sortable width="90" />
        <el-table-column prop="localAddr" :label="t('monitor.port.localAddr')" sortable width="160" />
        <el-table-column prop="process" :label="t('monitor.port.process')" sortable />
      </el-table>
    </div>

    <!-- Disks -->
    <div v-show="activeTab === 'disks'" class="tab-pane disks-pane" @contextmenu.prevent="showContextMenu($event)">
      <div class="od-toolbar">
        <el-input v-model="diskSearch" :placeholder="t('monitor.searchDisk')" clearable class="od-search" />
        <el-button :icon="RefreshRight" :loading="loadingDisks" @click="fetchDisks">
          {{ t('monitor.refresh') }}
        </el-button>
      </div>
      <el-table :data="filteredDisks" size="small" v-loading="loadingDisks" height="calc(100% - 36px)" class="od-table">
        <el-table-column prop="name" :label="t('monitor.disk.name')" sortable>
          <template #default="{ row }">
            <span :style="{ paddingLeft: (row.name.match(/^ +/)?.[0].length || 0) * 6 + 'px' }">{{ row.name.trim() }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="type" :label="t('monitor.disk.type')" sortable width="90" />
        <el-table-column prop="mountPoint" :label="t('monitor.disk.mountPoint')" sortable />
        <el-table-column prop="size" :label="t('monitor.disk.size')" sortable width="100" />
        <el-table-column prop="used" :label="t('monitor.disk.used')" sortable width="90" />
        <el-table-column prop="usage" :label="t('monitor.disk.usage')" sortable width="100">
          <template #default="{ row }">{{ row.usage ? row.usage + '%' : '-' }}</template>
        </el-table-column>
        <el-table-column prop="media" :label="t('monitor.disk.media')" sortable width="80" />
        <el-table-column prop="fsType" :label="t('monitor.disk.fstype')" sortable width="100" />
        <el-table-column prop="uuid" :label="t('monitor.disk.uuid')" sortable width="180" />
        <el-table-column prop="vendor" :label="t('monitor.disk.vendor')" sortable width="120" />
        <el-table-column prop="model" :label="t('monitor.disk.model')" sortable />
      </el-table>
    </div>

    <!-- Network -->
    <div v-show="activeTab === 'network'" class="tab-pane network-pane" @contextmenu.prevent="showContextMenu($event)">
      <div class="od-toolbar">
        <el-input v-model="netSearch" :placeholder="t('monitor.searchNetwork')" clearable class="od-search" />
        <el-button :icon="RefreshRight" :loading="loadingNetCards" @click="fetchNetCards">
          {{ t('monitor.refresh') }}
        </el-button>
      </div>
      <el-table :data="filteredNetCards" size="small" v-loading="loadingNetCards" height="calc(100% - 36px)" class="od-table">
        <el-table-column prop="name" :label="t('monitor.net.name')" sortable width="120" />
        <el-table-column prop="state" :label="t('monitor.net.state')" sortable width="90" />
        <el-table-column prop="mac" :label="t('monitor.net.mac')" sortable width="160" />
        <el-table-column prop="speed" :label="t('monitor.net.speed')" sortable width="120" />
        <el-table-column prop="type" :label="t('monitor.net.type')" sortable width="100" />
        <el-table-column prop="bondMaster" :label="t('monitor.net.bond')" sortable width="120" />
        <el-table-column prop="ipAddrs" :label="t('monitor.net.ipAddrs')" sortable>
          <template #default="{ row }">{{ row.ipAddrs?.join(', ') || '-' }}</template>
        </el-table-column>
      </el-table>
    </div>

    <!-- System Info -->
    <div v-show="activeTab === 'system'" class="tab-pane system-pane">
      <div v-if="systemInfo" class="system-content">
        <div v-for="group in systemGroups" :key="group.title" class="system-group">
          <div class="system-group-title">{{ group.title }}</div>
          <div class="system-group-items">
            <div v-for="item in group.items" :key="item.label" class="system-row">
              <span class="system-row-label">{{ item.label }}</span>
              <span class="system-row-value" @contextmenu.prevent="showContextMenu($event)">{{ item.value }}</span>
            </div>
          </div>
        </div>
      </div>
      <div v-else class="system-loading">{{ t('monitor.loading') }}</div>
    </div>

    <!-- Process Detail Panel (inside monitor-tab) -->
    <div class="detail-drawer-backdrop" :class="{ open: detailDrawerVisible }" @click="detailDrawerVisible = false"></div>
    <div class="detail-drawer" :class="{ open: detailDrawerVisible }">
      <div class="detail-drawer-header">
        <span class="detail-drawer-title">{{ t('monitor.processDetail') }}</span>
        <el-button link @click="detailDrawerVisible = false">
          <el-icon><Close /></el-icon>
        </el-button>
      </div>
      <div v-if="processDetail" class="process-detail">
        <div class="detail-section" @contextmenu="onDetailSectionContextMenu">
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.pid') }}</span>
            <span class="detail-value">{{ processDetail.pid }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.ppid') }}</span>
            <span class="detail-value">{{ processDetail.ppid ?? '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.processName') }}</span>
            <span class="detail-value">{{ processDetail.name ?? '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.state') }}</span>
            <span class="detail-value">{{ processDetail.state ?? '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.threads') }}</span>
            <span class="detail-value">{{ processDetail.threads ?? '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.exe') }}</span>
            <span class="detail-value">{{ processDetail.exe ?? '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.cwd') }}</span>
            <span class="detail-value">{{ processDetail.cwd ?? '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.cmdline') }}</span>
            <span class="detail-value cmdline">{{ processDetail.cmdline ?? '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.startTime') }}</span>
            <span class="detail-value">{{ processDetail.startTime ?? '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.fd') }}</span>
            <div class="detail-value io-stats" v-if="processDetail.fd">
              <div>{{ t('monitor.detail.fdTotal') }}: {{ processDetail.fd.total ?? 0 }}</div>
              <div>{{ t('monitor.detail.fdFiles') }}: {{ processDetail.fd.files ?? 0 }}</div>
              <div>{{ t('monitor.detail.fdSockets') }}: {{ processDetail.fd.sockets ?? 0 }}</div>
              <div>{{ t('monitor.detail.fdPipes') }}: {{ processDetail.fd.pipes ?? 0 }}</div>
              <div>{{ t('monitor.detail.fdAnons') }}: {{ processDetail.fd.anons ?? 0 }}</div>
              <div>{{ t('monitor.detail.fdDevs') }}: {{ processDetail.fd.devs ?? 0 }}</div>
              <div>{{ t('monitor.detail.fdOthers') }}: {{ processDetail.fd.others ?? 0 }}</div>
            </div>
            <span class="detail-value" v-else>-</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.vmRss') }}</span>
            <span class="detail-value">{{ processDetail.vmRss ?? '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.vmSize') }}</span>
            <span class="detail-value">{{ processDetail.vmSize ?? '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.cpuTicks') }}</span>
            <span class="detail-value">{{ processDetail.cpuTicks ?? '-' }}</span>
          </div>
          <div v-if="processDetail.voluntaryCtxSwitches != null || processDetail.nonvoluntaryCtxSwitches != null" class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.ctxSwitches') }}</span>
            <span class="detail-value">
              vol: {{ processDetail.voluntaryCtxSwitches ?? 0 }} / nonvol: {{ processDetail.nonvoluntaryCtxSwitches ?? 0 }}
            </span>
          </div>
          <div v-if="processDetail.io" class="detail-row">
            <span class="detail-label">{{ t('monitor.detail.io') }}</span>
            <div class="detail-value io-stats">
              <div>rchar: {{ formatIo(processDetail.io.rchar) }}</div>
              <div>wchar: {{ formatIo(processDetail.io.wchar) }}</div>
              <div>read_bytes: {{ formatIo(processDetail.io.read_bytes) }}</div>
              <div>write_bytes: {{ formatIo(processDetail.io.write_bytes) }}</div>
            </div>
          </div>
        </div>
        <div class="detail-actions">
          <el-button @click="detailDrawerVisible = false">{{ t('common.cancel') }}</el-button>
          <el-dropdown trigger="click" @command="(cmd: string) => onDetailAction(cmd)">
            <el-button type="primary">
              {{ t('monitor.sendSignal') }}<el-icon class="dropdown-icon"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="term">{{ t('monitor.signalTerm') }}</el-dropdown-item>
                <el-dropdown-item command="kill">{{ t('monitor.signalKill') }}</el-dropdown-item>
                <el-dropdown-item command="hup">{{ t('monitor.signalHup') }}</el-dropdown-item>
                <el-dropdown-item command="int">{{ t('monitor.signalInt') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>
      <div v-else class="process-detail-empty">{{ t('monitor.noProcessSelected') }}</div>
    </div>

    <!-- Kill Confirmation Dialog -->
    <el-dialog append-to-body v-model="killDialogVisible" :title="killType === 'kill' ? t('monitor.forceKill') : t('monitor.kill')" width="360px" align-center>
      <p>{{ killMessage }}</p>
      <template #footer>
        <el-button @click="killDialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" @click="confirmKill">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>

    <!-- Context Menu -->
    <div
      v-show="contextMenuVisible"
      class="context-menu"
      :style="{ left: contextMenuX + 'px', top: contextMenuY + 'px' }"
      @click.stop
    >
      <div class="context-menu-item" @click="copyContextText">{{ t('terminal.copy') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, onActivated, onDeactivated, watch, nextTick } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime'
import { SetMonitorActiveTab, SetMonitorPaused, GetProcessDetail, KillProcess, GetPorts, GetDisks, GetNetworkCards } from '../../wailsjs/go/main/App'
import { msg } from '../services/message'
import { ArrowDown, Close, RefreshRight } from '@element-plus/icons-vue'
import { useI18n } from '../i18n'

const props = defineProps<{
  sessionId: string
}>()

defineOptions({ name: 'MonitorTabContent' })

const { t } = useI18n()
const activeTab = ref('performance')
const selectedPerf = ref('cpu')
const processSearch = ref('')
const portSearch = ref('')
const diskSearch = ref('')
const netSearch = ref('')
const paused = ref(false)

// Process detail
const detailDrawerVisible = ref(false)
const selectedProcess = ref<any>(null)
const processDetail = ref<any>(null)

// Kill dialog
const killDialogVisible = ref(false)
const killTarget = ref<any>(null)
const killType = ref<string>('term') // 'term' | 'kill' | 'hup' | 'int'

// Context menu
const contextMenuVisible = ref(false)
const contextMenuX = ref(0)
const contextMenuY = ref(0)
const contextMenuText = ref('')

// Histories (max 60 points)
const cpuHistory = ref<number[]>([])
const memHistory = ref<number[]>([])
const diskHistory = ref<number[]>([])
const netRxHistory = ref<number[]>([])
const netTxHistory = ref<number[]>([])

// Current values (performance tab)
const currentCpu = ref({ usage: 0, cores: 0, processes: 0, handles: 0, load1: '-', load5: '-', load15: '-' })
const currentMem = ref({ total: 0, used: 0, free: 0, usage: 0, cached: 0, buffers: 0 })
const currentDisk = ref({ total: '', used: '', usage: 0 })
const currentNet = ref({ rx: 0, tx: 0 })
const processList = ref<any[]>([])
const systemInfo = ref<Record<string, any> | null>(null)

// Summary values for processes tab (independent from performance tab)
const processSummaryCpu = ref({ usage: 0, cores: 0, processes: 0, load1: '-', load5: '-', load15: '-' })
const processSummaryMem = ref({ total: 0, used: 0, free: 0, usage: 0, cached: 0, buffers: 0 })

// On-demand tab data
const portList = ref<any[]>([])
const diskList = ref<any[]>([])
const netCardList = ref<any[]>([])
const loadingPorts = ref(false)
const loadingDisks = ref(false)
const loadingNetCards = ref(false)

const chartCanvas = ref<HTMLCanvasElement>()

function pushHistory(arr: number[], val: number) {
  arr.push(val)
  if (arr.length > 60) arr.shift()
}

const perfItems = computed(() => [
  {
    key: 'cpu',
    label: t('monitor.cpu'),
    value: currentCpu.value.usage + '%',
    percent: Math.min(currentCpu.value.usage, 100),
    color: 'var(--chart-1)'
  },
  {
    key: 'memory',
    label: t('monitor.memory'),
    value: currentMem.value.usage + '%',
    percent: Math.min(currentMem.value.usage, 100),
    color: 'var(--chart-2)'
  },
  {
    key: 'disk',
    label: t('monitor.disk'),
    value: currentDisk.value.usage + '%',
    percent: Math.min(currentDisk.value.usage, 100),
    color: 'var(--chart-3)'
  },
  {
    key: 'network',
    label: t('monitor.network'),
    value: formatBytes(currentNet.value.rx + currentNet.value.tx) + '/s',
    percent: Math.min((currentNet.value.rx + currentNet.value.tx) / 1048576 * 100, 100),
    color: 'var(--chart-4)'
  }
])

const currentPerf = computed(() => {
  switch (selectedPerf.value) {
    case 'cpu':
      return {
        bigValue: currentCpu.value.usage + '%',
        color: 'var(--chart-1)',
        history: cpuHistory.value,
        yMin: 0,
        yMax: 100,
        details: [
          { label: t('monitor.cores'), value: String(currentCpu.value.cores) },
          { label: t('monitor.processCount'), value: String(currentCpu.value.processes) },
          { label: t('monitor.handleCount'), value: String(currentCpu.value.handles) },
          { label: t('monitor.load1'), value: String(currentCpu.value.load1 ?? '-') },
          { label: t('monitor.load5'), value: String(currentCpu.value.load5 ?? '-') },
          { label: t('monitor.load15'), value: String(currentCpu.value.load15 ?? '-') }
        ]
      }
    case 'memory':
      return {
        bigValue: currentMem.value.usage + '%',
        color: 'var(--chart-2)',
        history: memHistory.value,
        yMin: 0,
        yMax: 100,
        details: [
          { label: t('monitor.total'), value: currentMem.value.total.toFixed(2) + ' GB' },
          { label: t('monitor.used'), value: currentMem.value.used.toFixed(2) + ' GB' },
          { label: t('monitor.free'), value: currentMem.value.free.toFixed(2) + ' GB' },
          { label: t('monitor.cached'), value: (currentMem.value.cached?.toFixed(2) ?? '-') + ' GB' },
          { label: t('monitor.buffers'), value: (currentMem.value.buffers?.toFixed(2) ?? '-') + ' GB' }
        ]
      }
    case 'disk':
      return {
        bigValue: currentDisk.value.usage + '%',
        color: 'var(--chart-3)',
        history: diskHistory.value,
        yMin: 0,
        yMax: 100,
        details: [
          { label: t('monitor.total'), value: currentDisk.value.total },
          { label: t('monitor.used'), value: currentDisk.value.used }
        ]
      }
    case 'network':
      return {
        bigValue: formatBytes(currentNet.value.rx + currentNet.value.tx) + '/s',
        color: 'var(--chart-4)',
        color2: 'var(--chart-4-alt)',
        history: netRxHistory.value,
        history2: netTxHistory.value,
        yMin: 0,
        details: [
          { label: t('monitor.rx'), value: formatBytes(currentNet.value.rx) + '/s' },
          { label: t('monitor.tx'), value: formatBytes(currentNet.value.tx) + '/s' }
        ]
      }
    default:
      return { bigValue: '', color: 'var(--text-primary)', history: [] as number[], details: [] }
  }
})

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function formatIo(val: number | undefined): string {
  if (val == null) return '-'
  return formatBytes(val)
}

const filteredProcesses = computed(() => {
  const q = processSearch.value.trim().toLowerCase()
  if (!q) return processList.value
  return processList.value.filter((p: any) =>
    String(p.name).toLowerCase().includes(q) ||
    String(p.user).toLowerCase().includes(q) ||
    String(p.pid).includes(q)
  )
})

const filteredPorts = computed(() => {
  const q = portSearch.value.trim().toLowerCase()
  if (!q) return portList.value
  return portList.value.filter((p: any) =>
    String(p.localAddr).toLowerCase().includes(q) ||
    String(p.process).toLowerCase().includes(q)
  )
})

const filteredNetCards = computed(() => {
  const q = netSearch.value.trim().toLowerCase()
  if (!q) return netCardList.value
  return netCardList.value.filter((n: any) =>
    String(n.name).toLowerCase().includes(q) ||
    String(n.mac).toLowerCase().includes(q) ||
    (n.ipAddrs || []).some((ip: string) => ip.toLowerCase().includes(q)) ||
    String(n.bondMaster).toLowerCase().includes(q) ||
    (n.bondSlaves || []).some((s: string) => s.toLowerCase().includes(q))
  )
})

const filteredDisks = computed(() => {
  const q = diskSearch.value.trim().toLowerCase()
  if (!q) return diskList.value
  return diskList.value.filter((d: any) =>
    String(d.name).toLowerCase().includes(q)
  )
})

const systemGroups = computed(() => {
  if (!systemInfo.value) return []
  return [
    {
      title: t('monitor.system'),
      items: [
        { label: t('monitor.os'), value: systemInfo.value.os || '' },
        { label: t('monitor.version'), value: systemInfo.value.version || '' },
        { label: t('monitor.kernel'), value: systemInfo.value.kernel || '' },
        { label: t('monitor.hostname'), value: systemInfo.value.hostname || '' },
        { label: t('monitor.timezone'), value: systemInfo.value.timezone || '' }
      ].filter(i => i.value)
    },
    {
      title: t('monitor.cpu'),
      items: [
        { label: t('monitor.cpuModel'), value: systemInfo.value.cpuModel || '' },
        { label: t('monitor.cores'), value: String(systemInfo.value.cores || '') },
        { label: t('monitor.arch'), value: systemInfo.value.arch || '' },
        { label: t('monitor.cpuFreq'), value: systemInfo.value.cpuFreq ? systemInfo.value.cpuFreq + ' MHz' : '' },
        { label: t('monitor.memTotal'), value: systemInfo.value.memTotal ? systemInfo.value.memTotal + ' GB' : '' },
        { label: t('monitor.diskTotal'), value: systemInfo.value.diskTotal || '' }
      ].filter(i => i.value)
    },
    {
      title: t('monitor.network'),
      items: [
        { label: t('monitor.localIP'), value: systemInfo.value.localIP || '' }
      ].filter(i => i.value)
    }
  ].filter(g => g.items.length > 0)
})

const killMessage = computed(() => {
  if (!killTarget.value) return ''
  const name = killTarget.value.name || ''
  const pid = killTarget.value.pid || ''
  if (killType.value === 'kill') {
    return t('monitor.forceKillConfirm', { name, pid })
  }
  return t('monitor.killConfirm', { name, pid })
})

function togglePause() {
  paused.value = !paused.value
  SetMonitorPaused(props.sessionId, paused.value).catch(() => {})
}

async function onProcessRowClick(row: any) {
  selectedProcess.value = row
  await fetchProcessDetail(row.pid)
  detailDrawerVisible.value = true
}

async function fetchProcessDetail(pid: number) {
  try {
    const detail = await GetProcessDetail(props.sessionId, pid)
    processDetail.value = detail
  } catch (e: any) {
    msg.error(e?.message || 'Failed to fetch process detail')
    processDetail.value = null
  }
}

function onDetailAction(cmd: string) {
  if (!selectedProcess.value) return
  killTarget.value = selectedProcess.value
  killType.value = cmd
  killDialogVisible.value = true
}

async function confirmKill() {
  if (!killTarget.value) return
  const pid = killTarget.value.pid
  const signalMap: Record<string, string> = {
    term: 'TERM',
    kill: 'KILL',
    hup: 'HUP',
    int: 'INT'
  }
  const signal = signalMap[killType.value] || 'TERM'
  try {
    await KillProcess(props.sessionId, pid, signal)
    msg.success('Signal sent')
    killDialogVisible.value = false
  } catch (e: any) {
    msg.error(e?.message || 'Failed to send signal')
  }
}

async function fetchPorts() {
  loadingPorts.value = true
  try {
    portList.value = await GetPorts(props.sessionId)
  } catch (e: any) {
    msg.error(e?.message || 'Failed to fetch ports')
    portList.value = []
  } finally {
    loadingPorts.value = false
  }
}

async function fetchDisks() {
  loadingDisks.value = true
  try {
    diskList.value = await GetDisks(props.sessionId)
  } catch (e: any) {
    msg.error(e?.message || 'Failed to fetch disks')
    diskList.value = []
  } finally {
    loadingDisks.value = false
  }
}

async function fetchNetCards() {
  loadingNetCards.value = true
  try {
    netCardList.value = await GetNetworkCards(props.sessionId)
  } catch (e: any) {
    msg.error(e?.message || 'Failed to fetch network cards')
    netCardList.value = []
  } finally {
    loadingNetCards.value = false
  }
}

function showContextMenu(e: MouseEvent) {
  const selection = window.getSelection()?.toString().trim()
  if (!selection) return
  e.preventDefault()
  contextMenuText.value = selection
  contextMenuX.value = e.clientX
  contextMenuY.value = e.clientY
  contextMenuVisible.value = true
}

function hideContextMenu() {
  contextMenuVisible.value = false
}

function copyContextText() {
  if (!contextMenuText.value) return
  navigator.clipboard.writeText(contextMenuText.value).then(() => {
    msg.success(t('ai.copied'))
  }).catch(() => {
    // fallback
    const ta = document.createElement('textarea')
    ta.value = contextMenuText.value
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
    msg.success(t('ai.copied'))
  })
  hideContextMenu()
}

function onDetailSectionContextMenu(e: MouseEvent) {
  const target = e.target as HTMLElement
  const detailValue = target.closest('.detail-value') as HTMLElement | null
  if (detailValue) {
    showContextMenu(e)
  }
}

function drawChart() {
  const canvas = chartCanvas.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const dpr = window.devicePixelRatio || 1
  const rect = canvas.getBoundingClientRect()
  canvas.width = rect.width * dpr
  canvas.height = rect.height * dpr
  ctx.scale(dpr, dpr)

  const w = rect.width
  const h = rect.height
  const history = currentPerf.value.history
  const history2 = currentPerf.value.history2 as number[] | undefined

  ctx.clearRect(0, 0, w, h)

  if (history.length < 2) return

  const yMin = currentPerf.value.yMin ?? 0
  let yMax = currentPerf.value.yMax
  if (yMax == null) {
    const allVals = [...history]
    if (history2) allVals.push(...history2)
    yMax = Math.max(...allVals, yMin + 1)
  }
  const range = yMax - yMin
  const padding = 4

  // Grid lines
  ctx.strokeStyle = 'rgba(255,255,255,0.05)'
  ctx.lineWidth = 1
  for (let i = 1; i < 5; i++) {
    const y = h - (h * i / 5)
    ctx.beginPath()
    ctx.moveTo(0, y)
    ctx.lineTo(w, y)
    ctx.stroke()
  }

  // Helper to draw a line
  const ctx2 = ctx
  function drawLine(data: number[], color: string, fill?: boolean) {
    ctx2.strokeStyle = color
    ctx2.lineWidth = 2
    ctx2.beginPath()
    data.forEach((val: number, i: number) => {
      const x = (i / (data.length - 1)) * w
      const normalizedVal = Math.max(Math.min(val, yMax!) - yMin, 0)
      const y = h - padding - ((normalizedVal / range) * (h - padding * 2))
      if (i === 0) ctx2.moveTo(x, y)
      else ctx2.lineTo(x, y)
    })
    ctx2.stroke()

    if (fill) {
      ctx2.fillStyle = color + '20'
      ctx2.beginPath()
      data.forEach((val: number, i: number) => {
        const x = (i / (data.length - 1)) * w
        const normalizedVal = Math.max(Math.min(val, yMax!) - yMin, 0)
        const y = h - padding - ((normalizedVal / range) * (h - padding * 2))
        if (i === 0) ctx2.moveTo(x, y)
        else ctx2.lineTo(x, y)
      })
      ctx2.lineTo(w, h)
      ctx2.lineTo(0, h)
      ctx2.closePath()
      ctx2.fill()
    }
  }

  // Draw second line first (so it appears behind the main line)
  if (history2 && history2.length >= 2) {
    drawLine(history2, currentPerf.value.color2 || currentPerf.value.color)
  }

  // Draw main line with fill
  drawLine(history, currentPerf.value.color, true)
}

let unlisten: (() => void) | null = null

onMounted(() => {
  unlisten = EventsOn('session:data', (data: any) => {
    if (data?.id !== props.sessionId) return
    try {
      const payload = JSON.parse(data.data)
      if (payload.type === 'system') {
        systemInfo.value = payload.system
        return
      }
      if (payload.type === 'performance') {
        if (payload.cpu) {
          currentCpu.value = payload.cpu
          pushHistory(cpuHistory.value, payload.cpu.usage || 0)
        }
        if (payload.memory) {
          currentMem.value = payload.memory
          pushHistory(memHistory.value, payload.memory.usage || 0)
        }
        if (payload.disk) {
          currentDisk.value = payload.disk
          pushHistory(diskHistory.value, payload.disk.usage || 0)
        }
        if (payload.network) {
          currentNet.value = payload.network
          pushHistory(netRxHistory.value, payload.network.rx || 0)
          pushHistory(netTxHistory.value, payload.network.tx || 0)
        }
      }
      if (payload.type === 'processes' && payload.processes) {
        processList.value = payload.processes
        if (payload.summary) {
          if (payload.summary.cpu) {
            processSummaryCpu.value = payload.summary.cpu
          }
          if (payload.summary.memory) {
            processSummaryMem.value = payload.summary.memory
          }
        }
      }
      nextTick(drawChart)
    } catch {
      // ignore parse errors
    }
  })

  // Notify backend of initial active tab
  SetMonitorActiveTab(props.sessionId, activeTab.value).catch(() => {})

  document.addEventListener('click', hideContextMenu)
})

onActivated(() => {
  // Sync active tab when component is reactivated from KeepAlive cache
  SetMonitorActiveTab(props.sessionId, activeTab.value).catch(() => {})
  SetMonitorPaused(props.sessionId, false).catch(() => {})
})

onDeactivated(() => {
  // Pause data collection when component is hidden by KeepAlive
  SetMonitorPaused(props.sessionId, true).catch(() => {})
})

onUnmounted(() => {
  if (unlisten) unlisten()
  document.removeEventListener('click', hideContextMenu)
  SetMonitorPaused(props.sessionId, true).catch(() => {})
})

watch(() => currentPerf.value.history, drawChart, { deep: true })
watch(() => currentPerf.value.history2, drawChart, { deep: true })
watch(selectedPerf, () => nextTick(drawChart))
watch(activeTab, (tab) => {
  SetMonitorActiveTab(props.sessionId, tab).catch(() => {})
  if (tab === 'ports' && portList.value.length === 0 && !loadingPorts.value) {
    fetchPorts()
  }
  if (tab === 'disks' && diskList.value.length === 0 && !loadingDisks.value) {
    fetchDisks()
  }
  if (tab === 'network' && netCardList.value.length === 0 && !loadingNetCards.value) {
    fetchNetCards()
  }
})
</script>

<style scoped>
.monitor-tab {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--bg-base);
  position: relative;
  overflow: hidden;
}

.monitor-tabs-header {
  display: flex;
  gap: 0;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.tab-item {
  padding: 8px 20px;
  font-size: 13px;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  cursor: pointer;
  user-select: none;
  border-bottom: 2px solid transparent;
  transition: all 0.15s ease;
}

.tab-item:hover {
  color: var(--text-primary);
}

.tab-item.active {
  color: var(--accent);
  border-bottom-color: var(--accent);
}

.tab-pane {
  flex: 1;
  overflow: hidden;
  display: flex;
}

/* Performance pane */
.performance-pane {
  display: flex;
}

.perf-sidebar {
  width: 180px;
  flex-shrink: 0;
  border-right: 1px solid var(--border-subtle);
  padding: 8px;
  overflow-y: auto;
}

.perf-nav-item {
  padding: 10px 12px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  margin-bottom: 4px;
  transition: background 0.12s ease;
}

.perf-nav-item:hover {
  background: var(--bg-hover);
}

.perf-nav-item.active {
  background: var(--accent-subtle);
}

.perf-nav-name {
  font-size: 12px;
  color: var(--text-secondary);
  font-family: var(--font-ui);
}

.perf-nav-value {
  font-size: 18px;
  font-weight: 600;
  font-family: var(--font-mono);
  margin: 4px 0;
}

.perf-nav-bar {
  height: 4px;
  background: var(--bg-hover);
  border-radius: 2px;
  overflow: hidden;
}

.perf-nav-bar-inner {
  height: 100%;
  border-radius: 2px;
  transition: width 0.3s ease;
}

.perf-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 16px 20px;
  overflow-y: auto;
  min-height: 0;
}

.perf-big-value {
  font-size: 48px;
  font-weight: 700;
  font-family: var(--font-mono);
  margin-bottom: 12px;
}

.perf-chart {
  height: 180px;
  width: 100%;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}

.perf-details {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border-subtle);
}

.perf-detail-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.detail-label {
  font-size: 11px;
  color: var(--text-muted);
  font-family: var(--font-ui);
}

.detail-value {
  font-size: 14px;
  color: var(--text-primary);
  font-family: var(--font-mono);
  user-select: text;
}

/* Processes pane */
.processes-pane {
  flex-direction: column;
  padding: 12px;
}

.process-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
  flex-shrink: 0;
}

.process-summary {
  display: flex;
  gap: 20px;
  padding: 8px 12px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  overflow-x: auto;
  flex: 1;
}

.summary-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 60px;
  justify-content: center;
}

.summary-label {
  font-size: 10px;
  color: var(--text-muted);
  font-family: var(--font-ui);
  text-transform: uppercase;
  height: 14px;
  line-height: 14px;
}

.summary-value {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-mono);
}

.process-actions {
  flex-shrink: 0;
}

.process-search {
  width: 280px;
  margin-bottom: 8px;
  flex-shrink: 0;
}

.process-table {
  flex: 1;
  min-height: 0;
}

.process-table :deep(.el-table__row) {
  cursor: pointer;
}

/* System pane */
.system-pane {
  padding: 20px;
  overflow-y: auto;
}

.system-content {
  max-width: 600px;
}

.system-group {
  margin-bottom: 24px;
}

.system-group-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-ui);
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--border-subtle);
}

.system-group-items {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.system-row {
  display: flex;
  align-items: baseline;
  padding: 8px 0;
  gap: 40px;
  border-bottom: 1px solid var(--border-subtle);
}

.system-row:last-child {
  border-bottom: none;
}

.system-row-label {
  font-size: 12px;
  color: var(--text-muted);
  font-family: var(--font-ui);
  flex-shrink: 0;
  width: 120px;
  min-width: 120px;
}

.system-row-value {
  font-size: 13px;
  color: var(--text-primary);
  font-family: var(--font-mono);
  word-break: break-all;
  text-align: left;
  flex: 1;
  user-select: text;
}

.system-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-muted);
  font-size: 14px;
}

/* Process detail drawer */
.process-detail {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.process-detail .detail-section {
  flex: 1;
  overflow-y: auto;
  padding: 0 16px;
}

.process-detail .detail-row {
  display: flex;
  padding: 10px 0;
  border-bottom: 1px solid var(--border-subtle);
  gap: 12px;
}

.process-detail .detail-row:last-child {
  border-bottom: none;
}

.process-detail .detail-label {
  font-size: 12px;
  color: var(--text-muted);
  font-family: var(--font-ui);
  flex-shrink: 0;
  width: 100px;
  min-width: 100px;
}

.process-detail .detail-value {
  font-size: 13px;
  color: var(--text-primary);
  font-family: var(--font-mono);
  word-break: break-all;
  flex: 1;
  user-select: text;
}

.process-detail .detail-value.cmdline {
  white-space: pre-wrap;
  line-height: 1.5;
}

.process-detail .io-stats {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.process-detail .detail-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--border-subtle);
  margin-top: 12px;
}

.dropdown-icon {
  margin-left: 4px;
}

.process-detail-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-muted);
  font-size: 14px;
}

/* Context Menu */
.context-menu {
  position: fixed;
  z-index: 9999;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-md);
  padding: 4px 0;
  min-width: 100px;
}

.context-menu-item {
  padding: 8px 16px;
  font-size: 13px;
  color: var(--text-primary);
  font-family: var(--font-ui);
  cursor: pointer;
  transition: background 0.12s ease;
}

.context-menu-item:hover {
  background: var(--accent-subtle);
  color: var(--accent);
}

/* Detail drawer (inside monitor-tab) */
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
  width: 420px;
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

.detail-drawer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.detail-drawer-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-ui);
}

/* On-demand tabs (ports, disks, network) */
.od-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  flex-shrink: 0;
  gap: 12px;
}

.od-search {
  width: 240px;
}

.ports-pane,
.disks-pane,
.network-pane {
  flex-direction: column;
  padding: 0;
}

.od-table {
  flex: 1;
  min-height: 0;
}

.od-table :deep(.cell) {
  user-select: text;
}
</style>
