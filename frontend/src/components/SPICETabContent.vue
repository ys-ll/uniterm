<template>
  <div class="spice-tab-content">
    <!-- Connecting state -->
    <div v-if="status === 'connecting'" class="spice-overlay">
      <el-icon class="is-loading" :size="32"><Loader /></el-icon>
      <p>{{ t('spice.connecting', { host: config?.host || '...' }) }}</p>
    </div>

    <!-- Error state -->
    <div v-else-if="status === 'error'" class="spice-overlay">
      <p class="spice-error-text">{{ t('spice.error') }}</p>
      <el-button type="primary" @click="reconnect">{{ t('spice.retry') }}</el-button>
    </div>

    <!-- Disconnected state -->
    <div v-else-if="status === 'disconnected'" class="spice-overlay">
      <p>{{ t('spice.disconnected') }}</p>
      <el-button type="primary" @click="reconnect">{{ t('spice.reconnect') }}</el-button>
    </div>

    <!-- Connected: spice-html5 Canvas mounts here -->
    <div
      v-show="status === 'connected'"
      :id="'spice-screen-' + panelId"
      ref="spiceContainer"
      class="spice-area"
      tabindex="0"
    />

    <!-- Status bar -->
    <div v-show="status === 'connected'" class="spice-statusbar">
      <span class="spice-status-dot" />
      <span>{{ t('spice.connected') }}</span>
      <span class="spice-status-sep">|</span>
      <span>{{ config?.host }}:{{ config?.port || 5900 }}</span>
      <span class="spice-status-sep">|</span>
      <span class="spice-scale-label">{{ t('spice.scale') }}</span>
      <el-switch
        v-model="scaleViewport"
       
        style="--el-switch-on-color: #67c23a; --el-switch-off-color: #606266"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { Loader } from '@lucide/vue'
import { useI18n } from '../i18n'
import { usePanelStore } from '../stores/panelStore'
import type { ConnectionConfig } from '../types/session'
import { CreateSession, CloseSession } from '../../wailsjs/go/main/App'
import { EventsOn, ClipboardSetText, ClipboardGetText } from '../../wailsjs/runtime'

const { t } = useI18n()
const panelStore = usePanelStore()

const props = defineProps<{
  panelId: string
  config: ConnectionConfig | null
  sessionId: string | null
}>()

const status = ref<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting')
const currentSessionId = ref<string | null>(props.sessionId)
const spiceContainer = ref<HTMLDivElement | null>(null)
const scaleViewport = ref(false)

let sc: any = null
let unsubStatus: (() => void) | null = null
let isIniting = false

async function connect() {
  if (!props.config) return
  if (status.value === 'connecting' || status.value === 'connected') return
  status.value = 'connecting'
  try {
    const info = await CreateSession('spice', props.config)
    currentSessionId.value = info.id
  } catch (e: any) {
    console.error('SPICE connect error:', e)
    status.value = 'error'
  }
}

async function reconnect() {
  if (currentSessionId.value) {
    try { await CloseSession(currentSessionId.value) } catch (_) {}
    currentSessionId.value = null
  }
  if (sc) {
    try { sc.stop() } catch (_) {}
    sc = null
  }
  await connect()
}

async function initSpice(proxyAddr: string, password: string) {
  if (isIniting) return
  isIniting = true

  if (sc) {
    try { sc.stop() } catch (_) {}
    sc = null
  }
  if (spiceContainer.value) {
    spiceContainer.value.innerHTML = ''
  }

  if (!spiceContainer.value || spiceContainer.value.childElementCount > 0) {
    isIniting = false
    return
  }

  try {
    const { SpiceMainConn } = await import('../vendor/spice-html5.js')
    sc = new SpiceMainConn({
      uri: proxyAddr,
      password: password || '',
      screen_id: 'spice-screen-' + props.panelId,
      onerror: (e: any) => {
        console.error('[SPICE] connection error:', e)
      },
      onsuccess: () => {
        // Connected successfully
      },
    })
  } catch (e: any) {
    console.error('Failed to create SpiceMainConn:', e)
    status.value = 'error'
  }

  isIniting = false
}

function handleKeyDown(e: KeyboardEvent) {
  if (!sc || status.value !== 'connected') return
  // Ctrl+Shift+V: paste from local clipboard to SPICE
  if (e.ctrlKey && e.shiftKey && (e.key === 'v' || e.key === 'V')) {
    e.preventDefault()
    ClipboardGetText().then(text => {
      if (text && sc) {
        // SPICE handles clipboard through agent channel
        try { sc.sendClipboard(text) } catch (_) {}
      }
    }).catch(() => {})
  }
}

function applyScale() {
  if (!spiceContainer.value) return
  const container = spiceContainer.value
  const canvases = container.querySelectorAll('canvas')
  canvases.forEach((canvas: HTMLCanvasElement) => {
    if (scaleViewport.value) {
      const cw = container.clientWidth
      const ch = container.clientHeight
      const iw = canvas.width || canvas.clientWidth
      const ih = canvas.height || canvas.clientHeight
      const scale = Math.min(cw / iw, ch / ih, 1)
      const tx = (cw - iw * scale) / 2
      const ty = (ch - ih * scale) / 2
      canvas.style.transformOrigin = '0 0'
      canvas.style.transform = `translate(${tx}px, ${ty}px) scale(${scale})`
    } else {
      canvas.style.transform = ''
    }
  })
  container.style.overflow = scaleViewport.value ? 'hidden' : 'auto'
}

watch(scaleViewport, () => {
  applyScale()
  // ResizeObserver to reapply on container size changes
  if (scaleViewport.value && spiceContainer.value) {
    const ro = new ResizeObserver(() => applyScale())
    ro.observe(spiceContainer.value)
    ;(spiceContainer.value as any).__scaleObserver = ro
  } else if (spiceContainer.value) {
    const ro = (spiceContainer.value as any).__scaleObserver
    if (ro) { ro.disconnect(); (spiceContainer.value as any).__scaleObserver = null }
  }
})

onMounted(() => {
  if (props.sessionId) {
    currentSessionId.value = props.sessionId
  }

  // Restore cached DOM + SPICE if available (zero-delay tab switch)
  const cached = panelStore.getSPICECache(props.panelId)
  if (cached && spiceContainer.value) {
    const children = Array.from(cached.container.children)
    children.forEach(child => spiceContainer.value!.appendChild(child))
    sc = cached.sc
    panelStore.removeSPICECache(props.panelId)
    status.value = 'connected'
    document.addEventListener('keydown', handleKeyDown)
    return
  }

  const storedProxy = panelStore.getProxyAddr(props.panelId)
  if (storedProxy && props.config) {
    status.value = 'connected'
    initSpice(storedProxy, props.config.password || '')
  } else if (currentSessionId.value) {
    status.value = 'connected'
    connect()
  } else {
    connect()
  }

  document.addEventListener('keydown', handleKeyDown)

  unsubStatus = EventsOn('session:status', (data: any) => {
    if (data.id !== currentSessionId.value) return
    switch (data.status) {
      case 'connected':
        status.value = 'connected'
        if (data.proxyAddr) {
          panelStore.setProxyAddr(props.panelId, data.proxyAddr)
        }
        if (data.proxyAddr && props.config) {
          initSpice(data.proxyAddr, props.config.password || '')
        } else {
          const proxy = panelStore.getProxyAddr(props.panelId)
          if (proxy) {
            initSpice(proxy, props.config?.password || '')
          }
        }
        break
      case 'disconnected':
        if (status.value !== 'error') status.value = 'disconnected'
        break
      case 'error':
        status.value = 'error'
        break
    }
  })

  // Observe canvas creation for auto-scale
  if (spiceContainer.value) {
    const mo = new MutationObserver(() => {
      if (scaleViewport.value) applyScale()
    })
    mo.observe(spiceContainer.value, { childList: true, subtree: true, attributes: true, attributeFilter: ['width', 'height'] })
    ;(spiceContainer.value as any).__domObserver = mo
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleKeyDown)
  unsubStatus?.()

  if (spiceContainer.value) {
    const ro = (spiceContainer.value as any).__scaleObserver
    if (ro) { ro.disconnect(); (spiceContainer.value as any).__scaleObserver = null }
    const mo = (spiceContainer.value as any).__domObserver
    if (mo) { mo.disconnect(); (spiceContainer.value as any).__domObserver = null }
  }

  // Cache DOM + SPICE so switching back is instant
  if (sc && spiceContainer.value && spiceContainer.value.childElementCount > 0) {
    const container = document.createElement('div')
    container.style.display = 'none'
    const children = Array.from(spiceContainer.value.children)
    children.forEach(child => container.appendChild(child))
    document.body.appendChild(container)
    panelStore.setSPICECache(props.panelId, { sc, container })
  } else if (sc) {
    try { sc.stop() } catch (_) {}
    sc = null
  }
})

watch(() => props.sessionId, (newId) => {
  if (newId && !currentSessionId.value) {
    currentSessionId.value = newId
  }
})
</script>

<style scoped>
.spice-tab-content {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  background: #000;
}
.spice-area {
  flex: 1;
  overflow: auto;
  background: #000;
  outline: none;
  position: relative;
  min-height: 0;
}
.spice-area :deep(canvas) {
  display: block;
  image-rendering: pixelated;
}
.spice-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #999;
  z-index: 10;
}
.spice-error-text { color: #f56c6c; }
.spice-statusbar {
  flex-shrink: 0;
  height: 24px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  background: #1e1e1e;
  color: #999;
  font-size: 12px;
  box-sizing: border-box;
  z-index: 5;
}
.spice-status-dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: #67c23a;
  flex-shrink: 0;
}
.spice-status-sep { color: #444; }
.spice-scale-label {
  margin-left: auto;
  font-size: 11px;
}
</style>
