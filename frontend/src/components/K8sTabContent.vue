<template>
  <div class="k8s-tab">
    <div v-if="error" class="err">{{ error }}</div>
    <div v-else-if="!connId" class="loading">Connecting…</div>
    <template v-else>
      <div v-if="listError" class="err">{{ listError }}</div>
      <div class="toolbar">
        <span class="title">Pods ({{ pods.length }})</span>
        <input v-model="filter" placeholder="filter" class="filter" />
        <button @click="refresh">↻</button>
      </div>
      <table class="list">
        <thead><tr><th>Name</th><th>Namespace</th><th>Ready</th><th>Status</th><th>Restarts</th><th>Age</th></tr></thead>
        <tbody>
          <tr v-for="p in filtered" :key="p.metadata.uid" @click="openDetail(p)" class="row">
            <td>{{ p.metadata.name }}</td>
            <td>{{ p.metadata.namespace }}</td>
            <td>{{ readyString(p) }}</td>
            <td>{{ p.status?.phase || '' }}</td>
            <td>{{ totalRestarts(p) }}</td>
            <td>{{ age(p.metadata.creationTimestamp) }}</td>
          </tr>
        </tbody>
      </table>
      <div v-if="!pods.length && !listError" class="empty">
        No pods in namespace <code>{{ ns || '(all)' }}</code>.
      </div>

      <div v-if="detail" class="drawer">
        <div class="drawer-head">
          <span>{{ detail.metadata.namespace }} / {{ detail.metadata.name }}</span>
          <button @click="detail = null">✕</button>
        </div>
        <pre class="yaml">{{ detailYaml }}</pre>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useK8sStore } from '../stores/k8sStore'
import * as k8sClient from '../services/k8sClient'
import { useTunnelCredentials } from '../composables/useTunnelCredentials'
import type { K8sTab } from '../types/k8s'
import type { ConnectionConfig } from '../types/session'

const props = defineProps<{ tab: K8sTab; connection: ConnectionConfig }>()

const store = useK8sStore()
const { resolveTunnelCredentials } = useTunnelCredentials()
const connId = ref<string>('')
const error = ref('')
const filter = ref('')
const detail = ref<any | null>(null)
const detailYaml = ref('')

const ns = computed(() => props.tab.namespace)

const pods = computed(() => store.getPods(connId.value, ns.value))
const listError = computed(() => connId.value ? store.getError(connId.value, ns.value) : '')
const filtered = computed(() => {
  const f = filter.value.trim().toLowerCase()
  if (!f) return pods.value
  return pods.value.filter(p => (p.metadata?.name || '').toLowerCase().includes(f))
})

function readyString(pod: any): string {
  const cs = pod.status?.containerStatuses || []
  const ready = cs.filter((c: any) => c.ready).length
  return `${ready}/${cs.length}`
}

function totalRestarts(pod: any): number {
  const cs = pod.status?.containerStatuses || []
  return cs.reduce((sum: number, c: any) => sum + (c.restartCount || 0), 0)
}

function age(ts: string | undefined): string {
  if (!ts) return '—'
  const diff = Date.now() - new Date(ts).getTime()
  const s = Math.floor(diff / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h`
  return `${Math.floor(h / 24)}d`
}

async function openDetail(pod: any) {
  detail.value = pod
  const path = `/api/v1/namespaces/${pod.metadata.namespace}/pods/${pod.metadata.name}`
  const { status, raw } = await k8sClient.requestJSON(connId.value, 'GET', path)
  if (status === 200) {
    // 简单展示为格式化 JSON（P2 换 YAML editor）
    try {
      detailYaml.value = JSON.stringify(JSON.parse(raw), null, 2)
    } catch {
      detailYaml.value = raw
    }
  } else {
    detailYaml.value = `HTTP ${status}\n${raw}`
  }
}

async function refresh() {
  if (!connId.value) return
  store.unsubscribePods(connId.value, ns.value)
  await store.subscribePods(connId.value, ns.value)
}

async function connect() {
  try {
    const cfg = props.connection
    const source = cfg.k8sConfigInline ? cfg.k8sConfigInline : (cfg.k8sConfigPath || '~/.kube/config')
    const isPath = !cfg.k8sConfigInline
    let tunnelUser = ''
    let tunnelPassword = ''
    if (cfg.tunnelSSHConnId) {
      const creds = await resolveTunnelCredentials(cfg.tunnelSSHConnId)
      if (!creds) {
        error.value = 'Tunnel credentials cancelled'
        return
      }
      tunnelUser = creds.user
      tunnelPassword = creds.password
    }
    connId.value = await k8sClient.connect(
      source,
      isPath,
      cfg.k8sContext || '',
      cfg.tunnelSSHConnId || '',
      tunnelUser,
      tunnelPassword
    )
    await store.subscribePods(connId.value, ns.value)
  } catch (e: any) {
    error.value = String(e?.message || e)
  }
}

onMounted(connect)
onBeforeUnmount(() => {
  if (connId.value) {
    store.unsubscribePods(connId.value, ns.value)
    k8sClient.disconnect(connId.value)
  }
})

// 命名空间切换
watch(ns, async (newNs, oldNs) => {
  if (!connId.value) return
  store.unsubscribePods(connId.value, oldNs)
  await store.subscribePods(connId.value, newNs)
})
</script>

<style scoped>
.k8s-tab { display: flex; flex-direction: column; height: 100%; }
.toolbar { display: flex; align-items: center; gap: 8px; padding: 6px 10px; border-bottom: 1px solid var(--el-border-color-lighter, #333); }
.title { font-weight: 500; }
.filter { flex: 1; max-width: 220px; padding: 4px 8px; }
.list { width: 100%; border-collapse: collapse; }
.list th, .list td { padding: 6px 10px; text-align: left; border-bottom: 1px solid var(--el-border-color-lighter, #333); font-size: 13px; }
.row { cursor: pointer; }
.row:hover { background: rgba(255,255,255,0.03); }
.drawer { position: absolute; right: 0; top: 0; bottom: 0; width: 60%; background: var(--el-bg-color, #1e1e1e); border-left: 1px solid var(--el-border-color-lighter, #333); display: flex; flex-direction: column; }
.drawer-head { display: flex; justify-content: space-between; padding: 8px 12px; border-bottom: 1px solid var(--el-border-color-lighter, #333); }
.yaml { flex: 1; overflow: auto; padding: 12px; font-family: monospace; font-size: 12px; white-space: pre-wrap; }
.err { color: var(--el-color-danger, #f56); padding: 12px; }
.loading { padding: 12px; opacity: 0.7; }
.empty { padding: 24px; text-align: center; opacity: 0.55; font-size: 13px; }
.empty code { padding: 1px 6px; background: rgba(255,255,255,0.06); border-radius: 3px; font-family: monospace; }
</style>
