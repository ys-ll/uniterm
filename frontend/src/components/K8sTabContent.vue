<template>
  <div class="db-tab-content">
    <div v-if="error" class="k8s-fatal">{{ error }}</div>
    <div v-else-if="!connId" class="k8s-connecting">Connecting…</div>
    <div v-else class="db-main">
      <div class="db-left" :style="{ width: leftWidth + 'px' }">
        <K8sTree v-model="currentResourceKey" />
      </div>
      <div class="db-resizer" @mousedown="onResizeStart" />
      <div class="db-right">
        <K8sResourceList
          :conn-id="connId"
          :resource-key="currentResourceKey"
          :initial-namespace="initialNamespace"
          :namespace-options="namespaceOptions"
          @row-click="onRowClick"
        />
      </div>
    </div>

    <K8sYamlDrawer :obj="detail" @close="detail = null" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import * as k8sClient from '../services/k8sClient'
import { useTunnelCredentials } from '../composables/useTunnelCredentials'
import K8sTree from './K8sTree.vue'
import K8sResourceList from './K8sResourceList.vue'
import K8sYamlDrawer from './K8sYamlDrawer.vue'
import type { K8sTab } from '../types/k8s'
import type { ConnectionConfig } from '../types/session'

const props = defineProps<{ tab: K8sTab; connection: ConnectionConfig }>()

const { resolveTunnelCredentials } = useTunnelCredentials()

const connId = ref<string>('')
const error = ref('')
const currentResourceKey = ref<string>('pods')
const detail = ref<any | null>(null)
const initialNamespace = ref<string>(props.tab.namespace || '')

// 静态候选 + 打开时选中的 ns，后续 PR 再动态拉 /api/v1/namespaces。
const namespaceOptions = computed(() => {
  const set = new Set<string>(['default', 'kube-system', 'kube-public'])
  if (initialNamespace.value) set.add(initialNamespace.value)
  return Array.from(set)
})

// 左侧宽度 + resizer（抄 DBTabContent）
const leftWidth = ref(220)
let resizeStartX = 0
let resizeStartWidth = 0
let resizing = false
function onResizeStart(e: MouseEvent) {
  resizeStartX = e.clientX
  resizeStartWidth = leftWidth.value
  resizing = true
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
}
function onResizeMove(e: MouseEvent) {
  const dx = e.clientX - resizeStartX
  leftWidth.value = Math.max(150, Math.min(500, resizeStartWidth + dx))
}
function onResizeEnd() {
  resizing = false
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
}

function onRowClick(obj: any) {
  detail.value = obj
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
  } catch (e: any) {
    error.value = String(e?.message || e)
  }
}

onMounted(connect)
onBeforeUnmount(() => {
  if (resizing) {
    document.removeEventListener('mousemove', onResizeMove)
    document.removeEventListener('mouseup', onResizeEnd)
  }
  if (connId.value) {
    // K8sResourceList 内部 onBeforeUnmount 已经 unsubscribe 当前订阅。
    k8sClient.disconnect(connId.value)
  }
})
</script>

<style scoped>
/* 直接抄 DBTabContent 的骨架 CSS，class 同名 */
.db-tab-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}
.db-main {
  flex: 1;
  display: flex;
  overflow: hidden;
}
.db-left {
  flex-shrink: 0;
  border-right: 1px solid var(--border-subtle, #333);
  overflow: hidden;
}
.db-resizer {
  width: 4px;
  cursor: col-resize;
  background: transparent;
  flex-shrink: 0;
  transition: background 0.15s ease;
}
.db-resizer:hover {
  background: var(--border-subtle, #333);
}
.db-right {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.k8s-fatal {
  color: var(--el-color-danger, #f56);
  padding: 12px;
}
.k8s-connecting {
  padding: 12px;
  opacity: 0.7;
}
</style>
