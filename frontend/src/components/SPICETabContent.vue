<template>
  <div
    ref="container"
    class="spice-tab-content"
    :class="{ 'is-fullscreen': isFullscreen, 'mobile-bar': mobileBar }"
    :data-remote-keyboard="keyboardFocused || mobileKbOpen ? 'true' : undefined"
    @focusin="onFocusIn"
    @focusout="onFocusOut"
    @keydown.capture="onKey"
    @keyup.capture="onKey"
  >
    <!-- Connecting state -->
    <div v-if="status === 'connecting'" class="spice-overlay">
      <el-icon class="is-loading" :size="'2rem'"><Loader /></el-icon>
      <p>{{ t('spice.connecting', { host: config?.host || '...' }) }}</p>
    </div>

    <!-- Error state -->
    <div v-else-if="status === 'error'" class="spice-overlay">
      <p class="spice-error-text">{{ t('spice.error') }}</p>
      <p v-if="lastError" class="spice-error-detail">{{ lastError }}</p>
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
      ref="viewport"
      class="spice-area"
      :class="{ 'is-unscaled': !scaleViewport }"
      tabindex="0"
      @paste="onPaste"
    >
      <div ref="desktop" class="spice-desktop">
        <div :id="'spice-screen-' + panelId" ref="spiceContainer" class="spice-screen" />
      </div>
    </div>

    <!-- Status bar -->
    <div v-show="status === 'connected' && !isFullscreen" class="spice-statusbar">
      <span class="spice-status-dot" />
      <span>{{ t('spice.connected') }}</span>
      <span class="spice-status-sep">|</span>
      <RemoteDesktopZoomSelect v-model="zoom" />
      <span class="spice-status-spacer" />
      <button class="spice-fullscreen-btn" @click="toggleFullscreen">{{ t('remoteDesktop.fullscreen') }}</button>
    </div>
    <RemoteDesktopToolbar
      v-if="isFullscreen"
      v-model:zoom="zoom"
      :host="config?.host || ''"
      :port="config?.port || 5900"
      :resolution="resolution || undefined"
      :disabled="status !== 'connected'"
      @restore="toggleFullscreen"
      @focus-desktop="focusDesktop"
      @minimize="blurDesktop"
      @error="onToolbarError"
    />
    <!-- Android/iOS: soft keyboard trigger + touch gestures (tap = left
         click, long-press = right click; spice-html5 has no touch support). -->
    <RemoteMobileControls
      v-model:open="mobileKbOpen"
      :active="status === 'connected'"
      :target="viewport"
      @tap="onMobileTap"
      @right="onMobileRight"
      @move="onMobileMove"
      @drag-start="onMobileDragStart"
      @drag-move="onMobileDragMove"
      @drag-end="onMobileDragEnd"
      @wheel="onMobileWheel"
      @text="onMobileText"
      @special="onMobileSpecial"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, onActivated, onDeactivated, nextTick } from 'vue'
import { Loader } from '@lucide/vue'
import RemoteDesktopToolbar from './RemoteDesktopToolbar.vue'
import RemoteDesktopZoomSelect from './RemoteDesktopZoomSelect.vue'
import RemoteMobileControls from './RemoteMobileControls.vue'
import { useRemoteDesktopPreferences } from '../composables/useRemoteDesktopPreferences'
import { spiceDesktopGeometry } from '../utils/spiceGeometry'
import { SpiceKeyboard } from '../utils/spiceKeyboard'
import { createSpiceClipboard } from '../utils/spiceClipboard'
import { getSpiceConnectionMonitor } from '../utils/spiceConnection'
import { RdpClipboardSync, RdpInputQueue } from '../utils/rdpClipboard'
import { readClipboardText, writeClipboard } from '../composables/useClipboardWrite'
import { queueRemoteKeyboardChange, REMOTE_KEY_SCANCODES } from '../utils/remoteKeyboard'
import type { NativeRdpKey } from '../utils/remoteKeyboard'
import { charToScancode } from '../utils/remoteCharInput'
import { isMobilePlatform } from '../utils/platform'
import { toWebSocketURL } from '../utils/webSocketUrl'
import { useI18n } from '../i18n'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import type { ConnectionConfig } from '../types/session'
import { Events } from '@wailsio/runtime'
import { CreateSession, CloseSession, SetSPICEKeyboardCapture } from '../../bindings/github.com/ys-ll/uniterm/app'
const { t } = useI18n()
const panelStore = usePanelStore()
const sessionStore = useSessionStore()

const props = defineProps<{
  panelId: string
  config: ConnectionConfig | null
  sessionId: string | null
}>()

const status = ref<'connecting' | 'connected' | 'disconnected' | 'error'>('disconnected')
const lastError = ref('')
const currentSessionId = ref<string | null>(props.sessionId)
const container = ref<HTMLDivElement | null>(null)
const viewport = ref<HTMLDivElement | null>(null)
const desktop = ref<HTMLDivElement | null>(null)
const spiceContainer = ref<HTMLDivElement | null>(null)
const { zoom, scaleViewport } = useRemoteDesktopPreferences(() => props.config)
const isFullscreen = ref(false)
const resolution = ref('')
const keyboardFocused = ref(false)
const mobileKbOpen = ref(false)
const mobileBar = isMobilePlatform()
let keyboardCaptureId = ''
let keyboardGeneration = 0
let keyboardHeartbeat: ReturnType<typeof setInterval> | null = null
let unsubKeyboard: (() => void) | null = null
let clipboard: ReturnType<typeof createSpiceClipboard> | null = null
let clipboardSync: RdpClipboardSync | null = null
let clipboardPoll: ReturnType<typeof setInterval> | null = null
let restoreClipboard: (() => void) | null = null
const inputKeys = new Set<number>()
const inputQueue = new RdpInputQueue(error => console.error('SPICE input/clipboard error:', error))
const keyboard = new SpiceKeyboard(packet => {
  if (sc?.inputs?.state !== 'ready' || sc.inputs.ws?.readyState !== 1) return false
  sc.inputs.ws.send(packet)
  return true
})

let sc: any = null
let unsubStatus: (() => void) | null = null
let unsubConnection: (() => void) | null = null
let clientGeneration = 0
let disposed = false
let reconnecting = false
let resizeObserver: ResizeObserver | null = null
let canvasObserver: MutationObserver | null = null
let pixelRatioQuery: MediaQueryList | null = null

async function connect() {
  if (!props.config) return
  if (status.value === 'connecting' || status.value === 'connected') return
  const generation = ++clientGeneration
  lastError.value = ''
  status.value = 'connecting'
  try {
    const info = await CreateSession('spice', props.config)
    if (disposed || generation !== clientGeneration) {
      await CloseSession(info.id)
      return
    }
    currentSessionId.value = info.id
    // Bind the session to the panel so tab-close cleanup resolves to this
    // id, and drive the SPICE client from the proxy address returned by
    // CreateSession (the synchronous SPICE connect makes it available
    // here) rather than the racy session:status 'connected' event.
    panelStore.bindSession(props.panelId, info.id)
    sessionStore.initSession(info.id)
    if (info.proxyAddr) {
      panelStore.setProxyAddr(props.panelId, info.proxyAddr)
      await initSpice(info.proxyAddr, props.config.password || '')
    } else throw new Error('SPICE proxy address is unavailable.')
  } catch (e: any) {
    if (disposed || generation !== clientGeneration) return
    console.error('SPICE connect error:', e)
    lastError.value = e?.message || String(e)
    status.value = 'error'
  }
}

async function reconnect() {
  if (reconnecting) return
  reconnecting = true
  ++clientGeneration
  blurDesktop()
  stopSpiceClient()
  const sessionID = currentSessionId.value
  currentSessionId.value = null
  status.value = 'connecting'
  try {
    if (sessionID) {
      try { await CloseSession(sessionID) } catch (_) {}
    }
    if (disposed) return
    status.value = 'disconnected'
    await connect()
  } finally {
    reconnecting = false
  }
}

function stopSpiceClient() {
  onFocusOut()
  unsubConnection?.()
  unsubConnection = null
  restoreClipboard?.()
  restoreClipboard = null
  if (sc) {
    getSpiceConnectionMonitor(sc).stop()
    sc = null
  }
  resolution.value = ''
}

function syncSessionStatus() {
  panelStore.updateStatus(props.panelId, status.value)
  if (currentSessionId.value) sessionStore.updateStatus(currentSessionId.value, status.value)
}

function observeConnection() {
  unsubConnection?.()
  const target = sc
  unsubConnection = getSpiceConnectionMonitor(target).subscribe(state => {
    if (disposed || sc !== target) return
    lastError.value = state.error
    status.value = state.status
    syncSessionStatus()
    if (state.status === 'connected') void nextTick(() => {
      if (disposed || sc !== target) return
      applyScale()
      focusDesktop()
    })
  })
}

async function initSpice(proxyAddr: string, password: string) {
  const generation = ++clientGeneration
  stopSpiceClient()
  status.value = 'connecting'
  lastError.value = ''
  if (spiceContainer.value) {
    spiceContainer.value.innerHTML = ''
  }

  if (!spiceContainer.value || spiceContainer.value.childElementCount > 0) {
    return
  }

  try {
    const { SpiceMainConn } = await import('../vendor/spice-html5.js')
    if (disposed || generation !== clientGeneration) return
    sc = new SpiceMainConn({
      uri: toWebSocketURL(proxyAddr),
      password: password || '',
      screen_id: 'spice-screen-' + props.panelId,
      onerror: (e: any) => {
        console.error('[SPICE] connection error:', e)
      },
      onsuccess: () => {
        // Connected successfully
      },
    })
    observeConnection()
    if (sc.state !== 'closing' && sc.state !== 'error') setupClipboard()
  } catch (e: any) {
    if (disposed || generation !== clientGeneration) return
    console.error('Failed to create SpiceMainConn:', e)
    stopSpiceClient()
    lastError.value = e?.message || String(e)
    status.value = 'error'
  }
}

// Paste into the SPICE remote via the native paste event (Ctrl+V / context menu).
// SPICE handles clipboard through the agent channel.
function onPaste(e: ClipboardEvent) {
  e.preventDefault()
  if (!ownsRemoteKeyboard()) return
  const text = e.clipboardData?.getData('text')
  if (text !== undefined) {
    const generation = keyboardGeneration
    inputQueue.enqueue(async () => {
      await syncLocalClipboard(text)
      if (generation === keyboardGeneration && ownsRemoteKeyboard()) keyboard.paste()
    })
  }
}

function syncLocalClipboard(text?: string): Promise<void> {
  return ownsRemoteKeyboard() && clipboard?.ready ? clipboardSync?.sync(false, text) ?? Promise.resolve() : Promise.resolve()
}

function setupClipboard() {
  restoreClipboard?.()
  const target = sc
  const sync = new RdpClipboardSync({
    read: async () => {
      const generation = keyboardGeneration
      const text = await readClipboardText()
      return generation === keyboardGeneration && ownsRemoteKeyboard() && clipboard?.ready && sc === target ? { kind: 'text', text } : null
    },
    publish: async content => {
      if (content.kind === 'text' && ownsRemoteKeyboard() && sc === target) clipboard?.publish(content.text)
    },
    writeText: writeClipboard,
  }, true)
  clipboardSync = sync
  const agent = createSpiceClipboard(target, {
    canReceive: ownsRemoteKeyboard,
    onText: text => sync.receiveText(text),
    onReady: () => {
      sync.invalidateLocal()
      syncLocalClipboard().catch(error => console.error('SPICE clipboard sync error:', error))
    },
    onError: error => console.error('SPICE clipboard error:', error),
  })
  clipboard = agent
  restoreClipboard = () => {
    sync.dispose()
    agent.dispose()
    if (clipboard === agent) {
      clipboard = null
      clipboardSync = null
    }
  }
}

function applyScale() {
  const screen = spiceContainer.value
  const area = viewport.value
  const bounds = desktop.value
  if (!screen || !area || !bounds) return
  area.style.overflow = scaleViewport.value ? 'hidden' : 'auto'
  const primary = sc?.display?.surfaces?.[sc.display.primary_surface]?.canvas as HTMLCanvasElement | undefined
  const canvas = primary && screen.contains(primary) ? primary : screen.querySelector('canvas')
  const geometry = canvas && spiceDesktopGeometry(canvas.width, canvas.height,
    area.clientWidth, area.clientHeight, zoom.value, window.devicePixelRatio)
  if (!geometry || !canvas) {
    resolution.value = ''
    bounds.style.width = bounds.style.height = ''
    return
  }
  resolution.value = `${canvas.width} × ${canvas.height}`
  bounds.style.width = `${geometry.width}px`
  bounds.style.height = `${geometry.height}px`
  screen.style.width = `${canvas.width}px`
  screen.style.height = `${canvas.height}px`
  screen.style.transform = `scale(${geometry.scaleX}, ${geometry.scaleY})`
}

watch(zoom, applyScale, { flush: 'post' })

function onPixelRatioChange() {
  pixelRatioQuery?.removeEventListener('change', onPixelRatioChange)
  pixelRatioQuery = window.matchMedia(`(resolution: ${window.devicePixelRatio || 1}dppx)`)
  pixelRatioQuery.addEventListener('change', onPixelRatioChange)
  applyScale()
}

function focusDesktop() {
  if (status.value === 'connected') spiceContainer.value?.querySelector('canvas')?.focus({ preventScroll: true })
}

function blurDesktop() {
  onFocusOut()
  const active = document.activeElement
  if (active instanceof HTMLElement && spiceContainer.value?.contains(active)) active.blur()
}

function ownsRemoteKeyboard(): boolean {
  return status.value === 'connected' && document.hasFocus() && document.activeElement instanceof HTMLCanvasElement &&
    !!spiceContainer.value?.contains(document.activeElement)
}

function onKey(event: KeyboardEvent) {
  if (!ownsRemoteKeyboard()) return
  if (event.code === 'F12' || event.isComposing) {
    event.stopPropagation()
    return
  }
  const scancode = REMOTE_KEY_SCANCODES[event.code]
  if (scancode === undefined) return
  queueKey(scancode, event.type === 'keydown')
  event.preventDefault()
  event.stopPropagation()
}

// ── Mobile touch / soft keyboard input (issue #1425) ──

// spice-html5 listens on the canvas for mouse events; the touch layer emits
// logical gestures, so re-dispatch them as regular MouseEvents and let the
// existing handlers do the work.
function dispatchSpiceMouse(type: string, button: number, clientX: number, clientY: number) {
  const primary = sc?.display?.surfaces?.[sc.display.primary_surface]?.canvas as HTMLCanvasElement | undefined
  const canvas = primary && spiceContainer.value?.contains(primary)
    ? primary
    : spiceContainer.value?.querySelector('canvas')
  if (!canvas) return
  canvas.dispatchEvent(new MouseEvent(type, { clientX, clientY, button, bubbles: false, cancelable: true }))
}

function onMobileTap(x: number, y: number) {
  dispatchSpiceMouse('mousedown', 0, x, y)
  dispatchSpiceMouse('mouseup', 0, x, y)
}

function onMobileRight(x: number, y: number, down: boolean) {
  dispatchSpiceMouse(down ? 'mousedown' : 'mouseup', 2, x, y)
}

function onMobileMove(x: number, y: number) {
  dispatchSpiceMouse('mousemove', 0, x, y)
}

function onMobileDragStart(x: number, y: number, button: number) {
  dispatchSpiceMouse('mousedown', button, x, y)
}

function onMobileDragMove(x: number, y: number) {
  // The button state was sent with dragStart; only the pointer moves.
  dispatchSpiceMouse('mousemove', 0, x, y)
}

function onMobileDragEnd(x: number, y: number, button: number) {
  dispatchSpiceMouse('mouseup', button, x, y)
}

function onMobileWheel(x: number, y: number, deltaY: number) {
  const primary = sc?.display?.surfaces?.[sc.display.primary_surface]?.canvas as HTMLCanvasElement | undefined
  const canvas = primary && spiceContainer.value?.contains(primary)
    ? primary
    : spiceContainer.value?.querySelector('canvas')
  if (!canvas) return
  canvas.dispatchEvent(new WheelEvent('wheel', {
    clientX: x, clientY: y, deltaY, bubbles: false, cancelable: true,
  }))
}

function onMobileText(chars: string) {
  for (const ch of chars) {
    const key = charToScancode(ch)
    if (!key) continue
    if (key.shift) keyboard.key(REMOTE_KEY_SCANCODES.ShiftLeft, true)
    keyboard.key(key.scancode, true)
    keyboard.key(key.scancode, false)
    if (key.shift) keyboard.key(REMOTE_KEY_SCANCODES.ShiftLeft, false)
  }
}

function onMobileSpecial(code: string, down: boolean) {
  const scancode = REMOTE_KEY_SCANCODES[code]
  if (scancode !== undefined) keyboard.key(scancode, down)
}

function queueKey(scancode: number, down: boolean) {
  const paste = down && !inputKeys.has(scancode) && ((scancode === 0x2f &&
    [0x1d, 0xe01d, 0xe05b, 0xe05c].some(scan => inputKeys.has(scan))) ||
    (scancode === 0xe052 && [0x2a, 0x36].some(scan => inputKeys.has(scan))))
  if (down) inputKeys.add(scancode)
  else inputKeys.delete(scancode)
  if (paste) inputQueue.enqueue(() => syncLocalClipboard())
  inputQueue.enqueue(() => { keyboard.key(scancode, down) })
}

function onFocusIn() {
  if (!ownsRemoteKeyboard()) return
  if (!keyboardFocused.value) clipboardSync?.invalidateLocal()
  keyboardFocused.value = true
  syncLocalClipboard().catch(error => console.error('SPICE clipboard sync error:', error))
  if (keyboardCaptureId || !currentSessionId.value) return
  const captureID = crypto.randomUUID()
  const sessionID = currentSessionId.value
  keyboardCaptureId = captureID
  queueRemoteKeyboardChange(() => SetSPICEKeyboardCapture(sessionID, captureID, true, isFullscreen.value))
    .then(enabled => {
      if (!enabled || keyboardCaptureId !== captureID) return
      let pending = false
      keyboardHeartbeat = setInterval(() => {
        if (!ownsRemoteKeyboard()) { onFocusOut(); return }
        if (pending) return
        pending = true
        queueRemoteKeyboardChange(() => SetSPICEKeyboardCapture(sessionID, captureID, true, isFullscreen.value))
          .catch(error => {
            console.error('SPICE keyboard capture error:', error)
            if (keyboardCaptureId === captureID) onFocusOut()
          })
          .finally(() => { pending = false })
      }, 1000)
    })
    .catch(error => console.error('SPICE keyboard capture error:', error))
}

function onFocusOut() {
  ++keyboardGeneration
  if (keyboardHeartbeat !== null) clearInterval(keyboardHeartbeat)
  keyboardHeartbeat = null
  keyboardFocused.value = false
  const captureID = keyboardCaptureId
  keyboardCaptureId = ''
  if (captureID) {
    const sessionID = currentSessionId.value || ''
    queueRemoteKeyboardChange(() => SetSPICEKeyboardCapture(sessionID, captureID, false))
      .catch(error => console.error('SPICE keyboard release error:', error))
  }
  inputQueue.clear()
  inputKeys.clear()
  keyboard.release()
}

function onNativeKey(data: NativeRdpKey) {
  if (data.captureID !== keyboardCaptureId || !ownsRemoteKeyboard()) return
  if (data.reset) { onFocusOut(); onFocusIn(); return }
  queueKey(data.scancode, data.down)
}

function onVisibilityChange() {
  if (document.hidden) onFocusOut()
  else onFocusIn()
}

function onToolbarError(error: unknown) {
  console.error('SPICE minimize error:', error)
}

function onFullscreenChange() {
  isFullscreen.value = document.fullscreenElement === container.value
  if (keyboardCaptureId && currentSessionId.value) {
    // Apply the new fullscreen flag immediately instead of waiting for the
    // next capture heartbeat, so Escape is forwarded right away.
    const captureID = keyboardCaptureId
    const sessionID = currentSessionId.value
    queueRemoteKeyboardChange(() => SetSPICEKeyboardCapture(sessionID, captureID, true, isFullscreen.value))
      .catch(error => console.error('SPICE keyboard capture error:', error))
  }
  void nextTick(applyScale)
}

async function toggleFullscreen() {
  const el = container.value
  if (!el) return
  try {
    if (document.fullscreenElement === el) await document.exitFullscreen()
    else await el.requestFullscreen()
    focusDesktop()
  } catch (error) {
    console.error('SPICE fullscreen error:', error)
  }
}

function onFullscreenEvent(event: Event) {
  if ((event as CustomEvent<{ panelId: string }>).detail?.panelId === props.panelId) void toggleFullscreen()
}

function observeDisplay() {
  // ResizeObserver to reapply on container size changes
  if (viewport.value) {
    resizeObserver = new ResizeObserver(applyScale)
    resizeObserver.observe(viewport.value)
  }
  // Observe canvas creation for auto-scale
  if (spiceContainer.value) {
    canvasObserver = new MutationObserver(applyScale)
    canvasObserver.observe(spiceContainer.value, { childList: true, subtree: true, attributes: true, attributeFilter: ['width', 'height'] })
  }
  onPixelRatioChange()
}

function onReconnectEvent(e: Event) {
  const panelId = (e as CustomEvent)?.detail?.panelId
  if (panelId && panelId === props.panelId) reconnect()
}

onMounted(() => {
  unsubKeyboard = Events.On('remote-desktop:keyboard', event => onNativeKey(event.data as NativeRdpKey))
  window.addEventListener('blur', onFocusOut)
  window.addEventListener('focus', onFocusIn)
  document.addEventListener('visibilitychange', onVisibilityChange)
  clipboardPoll = setInterval(() => {
    syncLocalClipboard().catch(error => console.error('SPICE clipboard sync error:', error))
  }, 1000)
  document.addEventListener('fullscreenchange', onFullscreenChange)
  window.addEventListener('spice:fullscreen', onFullscreenEvent)
  // Tab right-click 「重连」(Reconnect) menu → forced reconnect of this panel.
  window.addEventListener('panel:reconnect', onReconnectEvent)
  observeDisplay()
  if (props.sessionId) {
    currentSessionId.value = props.sessionId
  }

  // Restore cached DOM + SPICE if available (zero-delay tab switch)
  const cached = panelStore.getSPICECache(props.panelId)
  if (cached && spiceContainer.value) {
    const children = Array.from(cached.container.children)
    children.forEach(child => spiceContainer.value!.appendChild(child))
    sc = cached.sc
    observeConnection()
    if (sc.state !== 'closing' && sc.state !== 'error') setupClipboard()
    panelStore.removeSPICECache(props.panelId)
    void nextTick(applyScale)
  } else {
    const storedProxy = panelStore.getProxyAddr(props.panelId)
    if (storedProxy && props.config) {
      initSpice(storedProxy, props.config.password || '')
    } else {
      connect()
    }
  }

  unsubStatus =Events.On('session:status', (ev) => { const data: any = ev.data; 
    if (data.id !== currentSessionId.value) return
    switch (data.status) {
      case 'connected':
        if (data.proxyAddr) {
          panelStore.setProxyAddr(props.panelId, data.proxyAddr)
        }
        if (!sc && status.value === 'connecting') {
          const proxy = data.proxyAddr || panelStore.getProxyAddr(props.panelId)
          if (proxy) initSpice(proxy, props.config?.password || '')
        }
        syncSessionStatus()
        break
      case 'disconnected':
        ++clientGeneration
        stopSpiceClient()
        if (status.value !== 'error') status.value = 'disconnected'
        syncSessionStatus()
        break
      case 'error':
        ++clientGeneration
        stopSpiceClient()
        lastError.value = data.errorMessage || lastError.value || t('spice.error')
        status.value = 'error'
        syncSessionStatus()
        break
    }
   })

})

onBeforeUnmount(() => {
  disposed = true
  ++clientGeneration
  onFocusOut()
  unsubConnection?.()
  unsubConnection = null
  if (clipboardPoll !== null) clearInterval(clipboardPoll)
  clipboardPoll = null
  restoreClipboard?.()
  restoreClipboard = null
  unsubKeyboard?.()
  window.removeEventListener('blur', onFocusOut)
  window.removeEventListener('focus', onFocusIn)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  unsubStatus?.()
  window.removeEventListener('panel:reconnect', onReconnectEvent)
  window.removeEventListener('spice:fullscreen', onFullscreenEvent)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  resizeObserver?.disconnect()
  canvasObserver?.disconnect()
  pixelRatioQuery?.removeEventListener('change', onPixelRatioChange)

  // Cache DOM + SPICE so switching back is instant
  if (sc && spiceContainer.value && spiceContainer.value.childElementCount > 0) {
    const container = document.createElement('div')
    container.style.display = 'none'
    const children = Array.from(spiceContainer.value.children)
    children.forEach(child => container.appendChild(child))
    document.body.appendChild(container)
    panelStore.setSPICECache(props.panelId, { sc, container })
  } else if (sc) {
    getSpiceConnectionMonitor(sc).stop()
    sc = null
  }
})

watch(() => props.sessionId, (newId) => {
  if (newId && !currentSessionId.value) {
    currentSessionId.value = newId
  }
})
onActivated(() => { void nextTick(applyScale) })
onActivated(onFocusIn)
onDeactivated(blurDesktop)
watch(status, value => {
  syncSessionStatus()
  if (value === 'connected') void nextTick(onFocusIn)
  else onFocusOut()
}, { flush: 'sync' })
</script>

<style scoped>
.spice-tab-content {
  position: relative;
  width: 100%;
  height: 100%;
  background: #000;
}
.spice-area {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 1.5rem;
  display: flex;
  overflow: auto;
  background: #000;
  outline: none;
  /* Drop the double-tap-zoom / tap-delay behavior on touch devices. */
  touch-action: manipulation;
}
.spice-desktop {
  position: relative;
  flex: none;
  margin: auto;
}
.spice-screen {
  position: absolute;
  top: 0;
  left: 0;
  transform-origin: 0 0;
}
.spice-area :deep(canvas) {
  display: block;
}
.spice-area.is-unscaled :deep(canvas) {
  image-rendering: pixelated;
}
.spice-tab-content.is-fullscreen .spice-area,
.spice-tab-content.is-fullscreen .spice-overlay {
  bottom: 0;
}
/* Mobile: reserve room for the always-visible key bar at the bottom. */
.spice-tab-content.mobile-bar .spice-area {
  bottom: 4.25rem;
}
.spice-tab-content.mobile-bar .spice-statusbar {
  bottom: 2.75rem;
}
.spice-tab-content.mobile-bar .spice-overlay {
  bottom: 4.25rem;
}
.spice-tab-content.mobile-bar.is-fullscreen .spice-area,
.spice-tab-content.mobile-bar.is-fullscreen .spice-overlay {
  bottom: 2.75rem;
}
.spice-tab-content.mobile-bar.is-fullscreen .spice-statusbar {
  display: none;
}
.spice-overlay {
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
.spice-error-text { color: var(--error); }
.spice-error-detail {
  color: var(--text-muted);
  font-size: 0.75rem;
  max-width: 80%;
  text-align: center;
  word-break: break-word;
}
.spice-statusbar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
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
.spice-status-dot {
  width: 0.5rem; height: 0.5rem;
  border-radius: 50%;
  background: var(--success);
  flex-shrink: 0;
}
.spice-status-sep { color: var(--text-disabled); }
.spice-status-label { font-size: 0.6875rem; }
.spice-status-spacer { flex: 1; }
.spice-fullscreen-btn {
  border: 1px solid var(--border-default);
  background: transparent;
  color: var(--text-muted);
  font-size: 0.75rem;
  padding: 1px 0.625rem;
  border-radius: var(--radius-sm);
}
.spice-fullscreen-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
</style>
