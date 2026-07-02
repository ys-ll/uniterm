<template>
  <div class="rdp-tab-content">
    <!-- Connecting state -->
    <div v-if="status === 'connecting'" class="rdp-overlay">
      <el-icon class="is-loading" :size="32"><Loader /></el-icon>
      <p>{{ t('rdp.connecting', { host: config?.host || '...' }) }}</p>
    </div>

    <!-- Error state -->
    <div v-else-if="status === 'error'" class="rdp-overlay">
      <p class="rdp-error-text">{{ t('rdp.error') }}</p>
      <el-button type="primary" @click="reconnect">{{ t('rdp.retry') }}</el-button>
    </div>

    <!-- Disconnected state -->
    <div v-else-if="status === 'disconnected'" class="rdp-overlay">
      <p>{{ t('rdp.disconnected') }}</p>
      <el-button type="primary" @click="reconnect">{{ t('rdp.reconnect') }}</el-button>
    </div>

    <!-- Connected: placeholder div overlaid by native RDP popup window -->
    <div
      v-show="status === 'connected'"
      class="rdp-area"
    />

    <!-- Status bar -->
    <div v-if="status === 'connected'" class="rdp-statusbar">
      <span class="rdp-status-dot" />
      <span>{{ t('rdp.connected') }}</span>
      <span class="rdp-status-sep">|</span>
      <span>{{ config?.host }}:{{ config?.port || 3389 }}</span>
      <span class="rdp-status-sep">|</span>
      <span>{{ t('rdp.resolution') }}: {{ statusResolution }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { Loader } from '@lucide/vue'
import { useI18n } from '../i18n'
import type { ConnectionConfig } from '../types/session'
import { CreateSession, CloseSession, RDPHide } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime'

const { t } = useI18n()

const props = defineProps<{
  panelId: string
  config: ConnectionConfig | null
  sessionId: string | null
}>()

const status = ref<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting')
const currentSessionId = ref<string | null>(props.sessionId)
const statusResolution = computed(() => {
  if (props.config?.rdpFixedWidth && props.config?.rdpFixedHeight) {
    return `${props.config.rdpFixedWidth}×${props.config.rdpFixedHeight}`
  }
  return '800×600'
})

// --- Connection ---

async function connect() {
  if (!props.config) return
  status.value = 'connecting'
  try {
    const info = await CreateSession('rdp', props.config)
    currentSessionId.value = info.id
  } catch (e) {
    console.error('RDP connect error:', e)
    status.value = 'error'
  }
}

async function reconnect() {
  if (currentSessionId.value) {
    try { await CloseSession(currentSessionId.value) } catch (_) {}
    currentSessionId.value = null
  }
  await connect()
}

// --- Events (lifecycle-scoped to avoid listener accumulation) ---

let unsubStatus: (() => void) | null = null
let unsubData: (() => void) | null = null

onMounted(() => {
  if (props.sessionId) {
    currentSessionId.value = props.sessionId
  }
  if (currentSessionId.value) {
    status.value = 'connected'
  } else {
    status.value = 'connecting'
  }

  unsubStatus = EventsOn('session:status', (data: any) => {
    if (data.id !== currentSessionId.value) return
    switch (data.status) {
      case 'connected':
        status.value = 'connected'
        window.dispatchEvent(new CustomEvent('rdp:sync-position'))
        break
      case 'disconnected':
        if (status.value !== 'error') status.value = 'disconnected'
        if (currentSessionId.value) RDPHide(currentSessionId.value)
        break
      case 'error':
        status.value = 'error'
        if (currentSessionId.value) RDPHide(currentSessionId.value)
        break
    }
  })

  unsubData = EventsOn('session:data', (data: any) => {
    if (data.id === currentSessionId.value && data.data?.includes('[Connection failed')) {
      status.value = 'error'
    }
  })
})

onUnmounted(() => {
  unsubStatus?.()
  unsubData?.()
})

watch(() => props.sessionId, (newId) => {
  if (newId && !currentSessionId.value) {
    currentSessionId.value = newId
  }
})
</script>

<style scoped>
.rdp-tab-content {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  background: #000;
  position: relative;
}
.rdp-area {
  flex: 1;
  background: #000;
}
.rdp-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--text-muted);
  z-index: 10;
}
.rdp-error-text { color: var(--error); }
.rdp-statusbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 12px;
  background: var(--bg-elevated);
  color: var(--text-muted);
  font-size: 12px;
  flex-shrink: 0;
}
.rdp-status-dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: var(--success);
}
.rdp-status-sep { color: var(--text-disabled); }
</style>
