<template>
  <div class="vnc-tab-content">
    <RemoteDesktopClipboard v-if="isWebMode() && status === 'connected'" :id="'vnc-clipboard-' + panelId" :received="remoteClipboard" :disabled="viewOnly" @send="sendClipboard" />
    <!-- Connecting state -->
    <div v-if="status === 'connecting'" class="vnc-overlay">
      <el-icon class="is-loading" :size="'2rem'"><Loader /></el-icon>
      <p>{{ t('vnc.connecting', { host: config?.host || '...' }) }}</p>
    </div>

    <!-- Error state -->
    <div v-else-if="status === 'error'" class="vnc-overlay">
      <p class="vnc-error-text">{{ t('vnc.error') }}</p>
      <p v-if="lastError" class="vnc-error-detail">{{ lastError }}</p>
      <el-button type="primary" @click="reconnect">{{ t('vnc.retry') }}</el-button>
    </div>

    <!-- Disconnected state -->
    <div v-else-if="status === 'disconnected'" class="vnc-overlay">
      <p>{{ t('vnc.disconnected') }}</p>
      <el-button type="primary" @click="reconnect">{{ t('vnc.reconnect') }}</el-button>
    </div>

    <!-- Connected: noVNC Canvas mounts here -->
    <div
      v-show="status === 'connected'"
      ref="vncContainer"
      class="vnc-area"
      tabindex="0"
      @paste="onPaste"
    />

    <!-- Status bar -->
    <div v-show="status === 'connected'" class="vnc-statusbar">
      <span class="vnc-status-dot" />
      <span>{{ t('vnc.connected') }}</span>
      <span class="vnc-status-sep">|</span>
      <span>{{ config?.host }}:{{ config?.port || 5900 }}</span>
      <span class="vnc-status-sep">|</span>
      <span class="vnc-status-label">{{ t('vnc.scale') }}</span>
      <el-switch
        v-model="scaleViewport"
        inline-prompt
        :active-text="t('vnc.scaleOn')"
        :inactive-text="t('vnc.scaleOff')"
        style="--el-switch-on-color: var(--success); --el-switch-off-color: var(--text-disabled)"
      />
      <span class="vnc-status-sep">|</span>
      <span class="vnc-status-label">{{ t('vnc.viewOnly') }}</span>
      <el-switch
        v-model="viewOnly"
        inline-prompt
        :active-text="t('vnc.scaleOn')"
        :inactive-text="t('vnc.scaleOff')"
        style="--el-switch-on-color: var(--success); --el-switch-off-color: var(--text-disabled)"
      />
      <span class="vnc-status-sep">|</span>
      <span class="vnc-status-label">{{ t('vnc.showDotCursor') }}</span>
      <el-switch
        v-model="showDotCursor"
        inline-prompt
        :active-text="t('vnc.scaleOn')"
        :inactive-text="t('vnc.scaleOff')"
        style="--el-switch-on-color: var(--success); --el-switch-off-color: var(--text-disabled)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { markRaw, nextTick, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { Loader } from '@lucide/vue'
import { useI18n } from '../i18n'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import type { ConnectionConfig } from '../types/session'
import { Clipboard, Events } from '@wailsio/runtime'
import { CloseSession } from '../../bindings/github.com/ys-ll/uniterm/app'
import { isWebMode } from '../utils/webMode'
import { createRemoteDesktop, remoteDesktopURL } from '../utils/webRemoteDesktop'
import RemoteDesktopClipboard from './RemoteDesktopClipboard.vue'
const { t } = useI18n()
const panelStore = usePanelStore()
const sessionStore = useSessionStore()

const props = defineProps<{
  panelId: string
  config: ConnectionConfig | null
  sessionId: string | null
}>()

const status = ref<'connecting' | 'connected' | 'disconnected' | 'error'>('disconnected')
const lastError = ref<string>('')
const currentSessionId = ref<string | null>(isWebMode() ? null : props.sessionId)
const vncContainer = ref<HTMLDivElement | null>(null)
const savedPassword = ref<string>('')
const scaleViewport = ref(true)
const viewOnly = ref(false)
const showDotCursor = ref(false)
const remoteClipboard = ref('')
let mounted = false
let generation = 0
let reconnecting = false
let removeRFBEvents = () => {}

function readThemeBackground(): string {
  // Pull the resolved --bg-base CSS variable, which the global theme
  // system (settingsStore.applyTheme) updates on `data-theme` change.
  // Returning the computed value rather than a hardcoded color keeps
  // the VNC canvas background in sync with the app's light / dark /
  // deep-blue / custom themes.
  return getComputedStyle(document.documentElement)
    .getPropertyValue('--bg-base')
    .trim() || '#000000'
}

let themeObserver: MutationObserver | null = null
function watchThemeBackground() {
  if (themeObserver) return
  themeObserver = new MutationObserver(() => {
    if (rfb) rfb.background = readThemeBackground()
  })
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-theme'],
  })
}

let rfb: any = null
let unsubStatus: (() => void) | null = null
let isIniting = false

async function connect() {
  if (!props.config) return
  if (status.value === 'connecting' || status.value === 'connected') return
  status.value = 'connecting'
  lastError.value = ''
  const attempt = ++generation
  try {
    const info = await createRemoteDesktop('vnc', props.config)
    if (!mounted || attempt !== generation) { await CloseSession(info.id); return }
    currentSessionId.value = info.id
    // Bind the session to the panel so tab-close cleanup and later
    // session:status events resolve to this id. The proxy address comes
    // straight from the CreateSession result: the synchronous VNC connect
    // makes it available here, independent of the racy session:status
    // 'connected' event that used to be dropped before the session id was
    // set, leaving VNC stuck on "connecting".
    panelStore.bindSession(props.panelId, info.id)
    sessionStore.initSession(info.id)
    if (info.proxyAddr) {
      panelStore.setProxyAddr(props.panelId, info.proxyAddr)
      savedPassword.value = props.config.password || ''
      initRFB(info.proxyAddr, savedPassword.value)
    }
  } catch (e: any) {
    if (!mounted || attempt !== generation) return
    console.error('VNC connect error:', e)
    lastError.value = e?.message || String(e)
    status.value = 'error'
  }
}

async function reconnect() {
  if (reconnecting) return
  reconnecting = true
  try {
    generation++
    removeRFBEvents()
    if (rfb) { rfb.disconnect(); rfb = null }
    if (currentSessionId.value) {
      try { await CloseSession(currentSessionId.value) } catch (_) {}
      currentSessionId.value = null
    }
    isIniting = false
    panelStore.removeProxyAddr(props.panelId)
    status.value = 'disconnected'
    await connect()
  } finally { reconnecting = false }
}

function initRFB(proxyAddr: string, password: string) {
  if (isIniting) return
  isIniting = true
  const attempt = ++generation
  status.value = 'connecting'

  if (rfb) {
    removeRFBEvents()
    try { rfb.disconnect() } catch (_) {}
    rfb = null
  }
  if (vncContainer.value) {
    vncContainer.value.innerHTML = ''
  }

  const RFB = (window as any).__novnc_RFB
  if (RFB) {
    createRFB(RFB, proxyAddr, password)
    return
  }

  import('@novnc/novnc').then((module: any) => {
    if (!mounted || attempt !== generation) return
    const LoadedRFB = module.default || module
    ;(window as any).__novnc_RFB = LoadedRFB
    createRFB(LoadedRFB, proxyAddr, password)
  }).catch((e: any) => {
    console.error('Failed to load noVNC module:', e)
    lastError.value = e?.message || String(e)
    status.value = 'error'
    isIniting = false
  })
}

function applyRFBOptions() {
  if (!rfb) return
  rfb.viewOnly = viewOnly.value
  rfb.scaleViewport = scaleViewport.value
  rfb.showDotCursor = showDotCursor.value
  rfb.background = readThemeBackground()
}

function createRFB(RFB: any, proxyAddr: string, password: string) {
  if (!vncContainer.value || vncContainer.value.childElementCount > 0) {
    isIniting = false
    return
  }

  try {
    rfb = markRaw(new RFB(vncContainer.value, remoteDesktopURL(proxyAddr), {
      credentials: { password: password || '' },
      shared: props.config?.vncShared ?? true,
      repeaterID: props.config?.vncRepeaterID || '',
    }))
    rfb.__unitermSessionId = currentSessionId.value
  } catch (e: any) {
    console.error('Failed to create RFB instance:', e)
    lastError.value = e?.message || String(e)
    status.value = 'error'
    isIniting = false
    return
  }

  // Force RFB 3.3 negotiation. RealVNC Server advertises a private
  // "RFB 005.000" banner which noVNC maps to 3.8; in 3.8 the server then
  // offers a security-type list ([6,130,2,...]) and noVNC picks RA2ne (6)
  // ahead of VNC Auth (2), failing with a 1006 close. Cap the max version to
  // 3.3 so noVNC replies "RFB 003.003" and the server falls back to legacy
  // single-security-type negotiation, sending VNC Auth (2) directly.
  rfb._rfbMaxVersion = 3.3

  applyRFBOptions()

  bindRFBEvents()
  isIniting = false
}

function bindRFBEvents() {
  removeRFBEvents()
  const client = rfb
  client.__unitermRemoveEvents?.()
  const listeners: [string, (e: any) => void][] = []
  const on = (name: string, listener: (e: any) => void) => {
    client.addEventListener(name, listener)
    listeners.push([name, listener])
  }
  removeRFBEvents = () => listeners.forEach(([name, listener]) => client.removeEventListener(name, listener))
  client.__unitermRemoveEvents = removeRFBEvents
  remoteClipboard.value = client.__unitermClipboard || ''

  // issue #95: only flip to 'connected' on the noVNC connect event.
  // Previously status='connected' was set as soon as the local proxy was
  // up, well before the RFB handshake completed, producing a black
  // screen with no error on security failure or wrong password.
  on('connect', () => {
    lastError.value = ''
    status.value = 'connected'
  })

  on('securityfailure', (e: any) => {
    const reason = e.detail?.reason ?? e.detail?.status
    lastError.value = t('vnc.errorSecurity', { reason: String(reason ?? '') })
    status.value = 'error'
  })

  on('credentialsrequired', (e: any) => {
    const types = (e.detail?.types || []).join(', ')
    lastError.value = t('vnc.errorCredentials', { types })
    status.value = 'error'
    client.disconnect()
  })

  on('disconnect', (e: any) => {
    if (!e.detail.clean) {
      if (!lastError.value) lastError.value = t('vnc.errorClosed')
      status.value = 'error'
    } else if (status.value !== 'error') {
      status.value = 'disconnected'
    }
  })

  on('clipboard', (e: any) => {
    const text = e.detail.text
    client.__unitermClipboard = text
    if (isWebMode()) remoteClipboard.value = text
    else Clipboard.SetText(text).catch(() => {})
  })
}

function sendClipboard(text: string) {
  if (rfb && !viewOnly.value) rfb.clipboardPasteFrom(text)
}

function onPaste(e: ClipboardEvent) {
  const text = e.clipboardData?.getData('text')
  if (text && rfb) {
    e.preventDefault()
    sendClipboard(text)
  }
}

function onReconnectEvent(e: Event) {
  const panelId = (e as CustomEvent)?.detail?.panelId
  if (panelId && panelId === props.panelId) reconnect()
}

onMounted(() => {
  mounted = true
  window.addEventListener('panel:reconnect', onReconnectEvent)
  if (props.sessionId && !isWebMode()) {
    currentSessionId.value = props.sessionId
  }

  const cached = panelStore.getVNCCache(props.panelId)
  if (cached && vncContainer.value) {
    const children = Array.from(cached.container.children)
    children.forEach(child => vncContainer.value!.appendChild(child))
    rfb = cached.rfb
    currentSessionId.value = rfb.__unitermSessionId || null
    rfb._target = vncContainer.value
    panelStore.removeVNCCache(props.panelId)
    status.value = rfb._rfbConnectionState === 'connected' ? 'connected' : 'disconnected'
    bindRFBEvents()
    // v-show must reveal the new container before noVNC measures it. Otherwise
    // a same-size tab restore scales the canvas to zero with no resize to fix it.
    nextTick(() => { if (mounted) applyRFBOptions() })
    watchThemeBackground()
    return
  }

  // Register event listener BEFORE connect() — with synchronous VNC/SPICE
  // connect the session:status event is emitted during CreateSession and
  // would arrive before the listener if we registered it after.
  unsubStatus = Events.On('session:status', (ev) => { const data: any = ev.data; 
    if (data.id !== currentSessionId.value) return
    switch (data.status) {
      case 'connected':
        if (isWebMode() || rfb || isIniting) break
        if (data.proxyAddr) {
          panelStore.setProxyAddr(props.panelId, data.proxyAddr)
        }
        if (props.config) {
          savedPassword.value = props.config.password || ''
        }
        if (data.proxyAddr && props.config) {
          initRFB(data.proxyAddr, props.config.password || '')
        } else {
          const proxy = panelStore.getProxyAddr(props.panelId)
          if (proxy) {
            initRFB(proxy, savedPassword.value)
          }
        }
        break
      case 'disconnected':
        if (status.value !== 'error') status.value = 'disconnected'
        break
      case 'error':
        if (!lastError.value) lastError.value = t('vnc.errorClosed')
        status.value = 'error'
        break
    }
  })

  const storedProxy = panelStore.getProxyAddr(props.panelId)
  if (storedProxy && props.config && !isWebMode()) {
    savedPassword.value = props.config.password || ''
    initRFB(storedProxy, savedPassword.value)
  } else {
    connect()
  }

  watchThemeBackground()

})

onBeforeUnmount(() => {
  mounted = false
  generation++
  themeObserver?.disconnect()
  themeObserver = null
  unsubStatus?.()
  window.removeEventListener('panel:reconnect', onReconnectEvent)

  if (rfb && status.value === 'connected' && vncContainer.value && vncContainer.value.childElementCount > 0) {
    const container = document.createElement('div')
    container.style.display = 'none'
    const children = Array.from(vncContainer.value.children)
    children.forEach(child => container.appendChild(child))
    document.body.appendChild(container)
    rfb._target = container
    panelStore.setVNCCache(props.panelId, { rfb, container })
  } else if (rfb) {
    removeRFBEvents()
    rfb.disconnect()
    rfb = null
  } else if (isWebMode() && currentSessionId.value) {
    CloseSession(currentSessionId.value).catch(() => {})
  }
})

watch(() => props.sessionId, (newId) => {
  if (!isWebMode() && newId && !currentSessionId.value) {
    currentSessionId.value = newId
  }
})

watch(scaleViewport, (val) => {
  if (rfb) rfb.scaleViewport = val
})
watch(viewOnly, (val) => {
  if (rfb) rfb.viewOnly = val
})
watch(showDotCursor, (val) => {
  if (rfb) rfb.showDotCursor = val
})
</script>

<style scoped>
.vnc-tab-content {
  position: relative;
  width: 100%;
  height: 100%;
  background: #000;
}
.vnc-area {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 1.5rem;
  background: #000;
  outline: none;
  overflow: auto;
}
.vnc-area :deep(canvas) {
  display: block;
  flex-shrink: 0;
}
.vnc-overlay {
  position: absolute;
  inset: 0;
  bottom: 1.5rem;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  color: var(--text-muted);
  z-index: 10;
}
.vnc-error-text { color: var(--error); }
.vnc-error-detail {
  color: var(--text-muted);
  font-size: 0.75rem;
  max-width: 80%;
  text-align: center;
  word-break: break-word;
}
.vnc-statusbar {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 1.5rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0 0.75rem;
  background: var(--bg-elevated);
  color: var(--text-muted);
  font-size: 0.75rem;
  box-sizing: border-box;
  z-index: 5;
}
.vnc-status-dot {
  width: 0.5rem; height: 0.5rem;
  border-radius: 50%;
  background: var(--success);
  flex-shrink: 0;
}
.vnc-status-sep { color: var(--text-disabled); }
.vnc-status-label { font-size: 0.6875rem; }
</style>
