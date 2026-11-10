<template>
  <div
    ref="container"
    class="vnc-tab-content"
    :class="{ 'is-fullscreen': isFullscreen }"
    :data-remote-keyboard="(keyboardFocused && !viewOnly) || mobileKbOpen ? 'true' : undefined"
    @focusin="onFocusIn"
    @focusout="onFocusOut"
    @keydown.capture="onLocalKey"
    @keyup.capture="onLocalKey"
  >
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
      :class="{ 'is-unscaled': !scaleViewport }"
      tabindex="0"
      @paste="onPaste"
    />

    <!-- Status bar -->
    <div v-show="status === 'connected' && !isFullscreen" class="vnc-statusbar">
      <RemoteDisplaySelect
        :displays="displayState.displays" :width="displayState.width" :height="displayState.height"
        :view="displayState.view" allow-custom @select="selectDisplay" @focus-desktop="focusDesktop"
        @create-region="openRegionEditor()" @edit-region="openRegionEditor"
      />
      <RemoteDesktopZoomSelect v-model="zoom" />
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
      <span class="vnc-status-spacer" />
      <button class="vnc-fullscreen-btn" @click="toggleFullscreen">{{ t('remoteDesktop.fullscreen') }}</button>
    </div>
    <RemoteDesktopToolbar
      v-if="isFullscreen"
      v-model:zoom="zoom"
      v-model:show-dot-cursor="showDotCursor"
      :host="config?.host || ''"
      :port="config?.port || 5900"
      :disabled="status !== 'connected'"
      @restore="toggleFullscreen"
      @focus-desktop="focusDesktop"
      @minimize="blurDesktop"
      @error="onToolbarError"
    >
      <template #displays>
        <RemoteDisplaySelect
          :displays="displayState.displays" :width="displayState.width" :height="displayState.height"
          :view="displayState.view" allow-custom @select="selectDisplay" @focus-desktop="focusDesktop"
          @create-region="openRegionEditor()" @edit-region="openRegionEditor"
        />
        <span class="vnc-status-sep">|</span>
      </template>
    </RemoteDesktopToolbar>
    <DesktopRegionEditor
      v-if="regionEditor" :key="regionEditor.region.id" :preview="regionEditor.preview" :region="regionEditor.region"
      :regions="displayState.view.regions || []" :editing-existing="regionEditor.editingExisting"
      :desktop-width="displayState.width" :desktop-height="displayState.height"
      @save="saveRegion" @cancel="closeRegionEditor" @delete="deleteRegion"
    />
    <!-- Android/iOS soft keyboard trigger (touch gestures are handled by
         noVNC's built-in gesture handler). -->
    <RemoteMobileControls
      v-model:open="mobileKbOpen"
      :active="status === 'connected' && !viewOnly"
      :target="null"
      :touch-enabled="false"
      @text="onMobileText"
      @special="onMobileSpecial"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, onActivated, onDeactivated, nextTick } from 'vue'
import { Loader } from '@lucide/vue'
import RemoteDesktopToolbar from './RemoteDesktopToolbar.vue'
import RemoteDisplaySelect from './RemoteDisplaySelect.vue'
import RemoteDesktopZoomSelect from './RemoteDesktopZoomSelect.vue'
import DesktopRegionEditor from './DesktopRegionEditor.vue'
import RemoteMobileControls from './RemoteMobileControls.vue'
import { useI18n } from '../i18n'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { useConnectionStore } from '../stores/connectionStore'
import { useRemoteDesktopPreferences } from '../composables/useRemoteDesktopPreferences'
import type { ConnectionConfig } from '../types/session'
import { Events } from '@wailsio/runtime'
import { CreateSession, CloseSession, SetVNCKeyboardCapture, EncodeVNCClipboardText } from '../../bindings/github.com/ys-ll/uniterm/app'
import { createVncClipboard } from '../utils/vncClipboard'
import { installVncPixelScaling } from '../utils/vncScaling'
import { installVncCursor } from '../utils/vncCursor'
import { captureVncPreview, installVncDisplays, type DesktopDisplayState, type DesktopPreview, type DesktopRegion, type DesktopView } from '../utils/vncDisplays'
import { deleteDesktopRegion, saveDesktopRegion, suggestedDesktopRegion } from '../utils/desktopRegions'
import { msg } from '../services/message'
import { VncNativeKeyboard } from '../utils/vncKeyboard'
import { queueRemoteKeyboardChange } from '../utils/remoteKeyboard'
import { charToKeysym, CODE_KEYSYMS } from '../utils/remoteCharInput'
import type { NativeRdpKey } from '../utils/remoteKeyboard'
import { RdpClipboardSync, RdpInputQueue } from '../utils/rdpClipboard'
import { readClipboardText, writeClipboard } from '../composables/useClipboardWrite'
import { toWebSocketURL } from '../utils/webSocketUrl'
const { t } = useI18n()
const panelStore = usePanelStore()
const sessionStore = useSessionStore()
const connectionStore = useConnectionStore()

const props = defineProps<{
  panelId: string
  config: ConnectionConfig | null
  sessionId: string | null
}>()

const status = ref<'connecting' | 'connected' | 'disconnected' | 'error'>('disconnected')
const lastError = ref<string>('')
const currentSessionId = ref<string | null>(props.sessionId)
const container = ref<HTMLDivElement | null>(null)
const isFullscreen = ref(false)
const vncContainer = ref<HTMLDivElement | null>(null)
const savedPassword = ref<string>('')
const { zoom, scaleViewport, viewOnly, showDotCursor } = useRemoteDesktopPreferences(() => props.config)
const displayState = ref<DesktopDisplayState>({ displays: [], width: 0, height: 0, view: { selected: 'all' } })
let displayController: ReturnType<typeof installVncDisplays> | null = null
const regionEditor = ref<{ preview: DesktopPreview; region: DesktopRegion; editingExisting: boolean } | null>(null)

function openRegionEditor(id?: string) {
  if (!rfb || status.value !== 'connected') return
  blurDesktop()
  try {
    const preview = captureVncPreview(rfb)
    const regions = displayState.value.view.regions || []
    const existing = regions.find(region => region.id === id)
    let number = 1
    while (regions.some((region, index) => (region.name || t('remoteDesktop.regionNumber', { n: index + 1 })) === t('remoteDesktop.regionNumber', { n: number }))) number++
    const region = existing ? { ...existing, name: existing.name || t('remoteDesktop.regionNumber', { n: regions.indexOf(existing) + 1 }) } : {
      ...suggestedDesktopRegion(displayState.value.view, preview.width, preview.height, displayState.value.displays),
      id: crypto.randomUUID(), name: t('remoteDesktop.regionNumber', { n: number }),
    }
    regionEditor.value = { preview, region, editingExisting: !!existing }
  } catch (error) {
    console.error('Desktop preview failed:', error)
    msg.error(t('remoteDesktop.previewUnavailable'))
    focusDesktop()
  }
}

function closeRegionEditor() {
  regionEditor.value = null
  void nextTick(focusDesktop)
}

function saveRegion(region: DesktopRegion) {
  selectDisplay(saveDesktopRegion(displayState.value.view, region))
  closeRegionEditor()
}

function deleteRegion(id: string) {
  selectDisplay(deleteDesktopRegion(displayState.value.view, id))
  closeRegionEditor()
}

function selectDisplay(view: DesktopView) {
  displayController?.select(view)
  if (props.config) {
    props.config.desktopView = view
    void connectionStore.update(props.config.id, { desktopView: view })
  }
}
const keyboardFocused = ref(false)
const mobileKbOpen = ref(false)
let keyboardCaptureId = ''
let keyboardHeartbeat: ReturnType<typeof setInterval> | null = null
let clipboardPoll: ReturnType<typeof setInterval> | null = null
let clipboardSync: RdpClipboardSync | null = null
let nativeKeyboard: VncNativeKeyboard | null = null
let restoreDesktopInput: (() => void) | null = null
let unsubKeyboard: (() => void) | null = null
const inputKeys = new Set<string>()
const sentKeys = new Map<string, number>()
const inputQueue = new RdpInputQueue(error => console.error('VNC input/clipboard error:', error))

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
let restorePixelScaling: (() => void) | null = null
let restoreCursor: (() => void) | null = null
let pixelRatioQuery: MediaQueryList | null = null

function onPixelRatioChange() {
  pixelRatioQuery?.removeEventListener('change', onPixelRatioChange)
  pixelRatioQuery = window.matchMedia(`(resolution: ${window.devicePixelRatio || 1}dppx)`)
  pixelRatioQuery.addEventListener('change', onPixelRatioChange)
  rfb?._updateScale()
}

// RFB version-negotiation retry state: currentProxyAddr powers the 3.3
// downgrade rebuild; everConnected/rfb33Retried guard the one-shot retry.
let currentProxyAddr = ''
let everConnected = false
let rfb33Retried = false

async function connect() {
  if (!props.config) return
  if (status.value === 'connecting' || status.value === 'connected') return
  status.value = 'connecting'
  lastError.value = ''
  try {
    const info = await CreateSession('vnc', props.config)
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
    console.error('VNC connect error:', e)
    lastError.value = e?.message || String(e)
    status.value = 'error'
  }
}

async function reconnect() {
  regionEditor.value = null
  onFocusOut()
  restoreDesktopInput?.()
  restoreDesktopInput = null
  displayController?.dispose()
  displayController = null
  restorePixelScaling?.()
  restorePixelScaling = null
  restoreCursor?.()
  restoreCursor = null
  if (currentSessionId.value) {
    try { await CloseSession(currentSessionId.value) } catch (_) {}
    currentSessionId.value = null
  }
  if (rfb) {
    rfb.disconnect()
    rfb = null
  }
  await connect()
}

function initRFB(proxyAddr: string, password: string, forceRfb33 = false) {
  if (isIniting) return
  isIniting = true
  onFocusOut()
  restoreDesktopInput?.()
  restoreDesktopInput = null
  displayController?.dispose()
  displayController = null
  restorePixelScaling?.()
  restorePixelScaling = null
  restoreCursor?.()
  restoreCursor = null

  // Fresh attempt (not the 3.3 downgrade retry): reset negotiation state
  // and remember the proxy address the downgrade retry would reuse.
  if (!forceRfb33) {
    currentProxyAddr = proxyAddr
    everConnected = false
    rfb33Retried = false
  }

  if (rfb) {
    try { rfb.disconnect() } catch (_) {}
    rfb = null
  }
  if (vncContainer.value) {
    vncContainer.value.innerHTML = ''
  }

  const RFB = (window as any).__novnc_RFB
  if (RFB) {
    createRFB(RFB, proxyAddr, password, forceRfb33)
    return
  }

  import('@novnc/novnc').then((module: any) => {
    const LoadedRFB = module.default || module
    ;(window as any).__novnc_RFB = LoadedRFB
    createRFB(LoadedRFB, proxyAddr, password, forceRfb33)
  }).catch((e: any) => {
    console.error('Failed to load noVNC module:', e)
    lastError.value = e?.message || String(e)
    status.value = 'error'
    isIniting = false
  })
}

function applyRFBOptions() {
  if (!rfb) return
  displayController?.dispose()
  restorePixelScaling?.()
  restorePixelScaling = installVncPixelScaling(rfb, () => zoom.value)
  restoreCursor?.()
  if (container.value) restoreCursor = installVncCursor(rfb, container.value)
  rfb.viewOnly = viewOnly.value
  rfb.scaleViewport = scaleViewport.value
  rfb.showDotCursor = showDotCursor.value
  rfb.background = readThemeBackground()
  displayController = installVncDisplays(rfb, props.config?.desktopView ?? { selected: 'all' }, state => {
    displayState.value = state
  })
  setupDesktopInput()
}

function ownsRemoteKeyboard(): boolean {
  return !regionEditor.value && status.value === 'connected' && !viewOnly.value && document.hasFocus() &&
    document.activeElement === vncContainer.value?.querySelector('canvas')
}

function onLocalKey(event: KeyboardEvent) {
  if (event.code === 'F12') event.stopPropagation()
}

function syncLocalClipboard(text?: string): Promise<void> {
  return ownsRemoteKeyboard() ? clipboardSync?.sync(false, text) ?? Promise.resolve() : Promise.resolve()
}

function queueVncKey(keysym: number, code: string, down: boolean, send: () => void) {
  const repeat = inputKeys.has(code)
  const paste = down && !repeat && ((code === 'KeyV' &&
    ['ControlLeft', 'ControlRight', 'MetaLeft', 'MetaRight'].some(key => inputKeys.has(key))) ||
    (code === 'Insert' && ['ShiftLeft', 'ShiftRight'].some(key => inputKeys.has(key))))
  if (down) inputKeys.add(code)
  else inputKeys.delete(code)
  if (paste) inputQueue.enqueue(() => syncLocalClipboard())
  inputQueue.enqueue(() => {
    send()
    if (down) sentKeys.set(code, keysym)
    else sentKeys.delete(code)
  })
}

function setupDesktopInput() {
  restoreDesktopInput?.()
  const target = rfb
  const clipboard = createVncClipboard(target, props.config?.vncClipboardEncoding, EncodeVNCClipboardText,
    () => ownsRemoteKeyboard() && rfb === target)
  const sync = new RdpClipboardSync({
    read: async () => {
      const text = await readClipboardText()
      return ownsRemoteKeyboard() ? { kind: 'text', text } : null
    },
    publish: async content => {
      if (content.kind === 'text') await clipboard.publish(content.text)
    },
    writeText: writeClipboard,
  }, true)
  clipboardSync = sync
  nativeKeyboard = new VncNativeKeyboard((keysym, code, down) => {
    queueVncKey(keysym, code, down, () => target.sendKey(keysym, code, down))
  })
  const keyboard = target._keyboard
  const original = keyboard.onkeyevent
  const handler = (keysym: number, code: string, down: boolean, numlock: boolean, capslock: boolean) => {
    queueVncKey(keysym, code, down, () => original(keysym, code, down, numlock, capslock))
  }
  keyboard.onkeyevent = handler
  const onClipboard = (event: any) => {
    try {
      sync.receiveText(clipboard.decode(event.detail.text)).catch(error => console.error('VNC clipboard write error:', error))
    } catch (error) {
      console.error('VNC clipboard decode error:', error)
    }
  }
  target.addEventListener('clipboard', onClipboard)
  restoreDesktopInput = () => {
    sync.dispose()
    clipboard.dispose()
    target.removeEventListener('clipboard', onClipboard)
    if (keyboard.onkeyevent === handler) keyboard.onkeyevent = original
    inputQueue.clear()
    nativeKeyboard = null
    clipboardSync = null
    inputKeys.clear()
    sentKeys.clear()
  }
}

function onFocusIn() {
  if (!ownsRemoteKeyboard()) return
  keyboardFocused.value = true
  syncLocalClipboard().catch(error => console.error('VNC clipboard sync error:', error))
  if (keyboardCaptureId || !currentSessionId.value) return
  const captureID = crypto.randomUUID()
  const sessionID = currentSessionId.value
  keyboardCaptureId = captureID
  queueRemoteKeyboardChange(() => SetVNCKeyboardCapture(sessionID, captureID, true, isFullscreen.value))
    .then(enabled => {
      if (!enabled || keyboardCaptureId !== captureID) return
      let pending = false
      keyboardHeartbeat = setInterval(() => {
        if (!ownsRemoteKeyboard()) { onFocusOut(); return }
        if (pending) return
        pending = true
        queueRemoteKeyboardChange(() => SetVNCKeyboardCapture(sessionID, captureID, true, isFullscreen.value))
          .catch(error => {
            console.error('VNC keyboard capture error:', error)
            if (keyboardCaptureId === captureID) onFocusOut()
          })
          .finally(() => { pending = false })
      }, 1000)
    })
    .catch(error => console.error('VNC keyboard capture error:', error))
}

function onFocusOut() {
  if (keyboardHeartbeat !== null) clearInterval(keyboardHeartbeat)
  keyboardHeartbeat = null
  keyboardFocused.value = false
  const captureID = keyboardCaptureId
  keyboardCaptureId = ''
  if (captureID) {
    const sessionID = currentSessionId.value || ''
    queueRemoteKeyboardChange(() => SetVNCKeyboardCapture(sessionID, captureID, false))
      .catch(error => console.error('VNC keyboard release error:', error))
  }
  inputQueue.clear()
  nativeKeyboard?.release()
  for (const [code, keysym] of Array.from(sentKeys).reverse()) rfb?.sendKey(keysym, code, false)
  sentKeys.clear()
  inputKeys.clear()
}

function onNativeKey(data: NativeRdpKey) {
  if (data.captureID !== keyboardCaptureId || !ownsRemoteKeyboard()) return
  if (data.reset) { onFocusOut(); onFocusIn(); return }
  nativeKeyboard?.key(data)
}

// ── Mobile soft keyboard input (issue #1425) ──

function onMobileText(chars: string) {
  if (!rfb) return
  for (const ch of chars) {
    const keysym = charToKeysym(ch)
    if (keysym === null) continue
    const code = `mobile-${keysym}`
    queueVncKey(keysym, code, true, () => rfb?.sendKey(keysym, code, true))
    queueVncKey(keysym, code, false, () => rfb?.sendKey(keysym, code, false))
  }
}

function onMobileSpecial(code: string, down: boolean) {
  const keysym = CODE_KEYSYMS[code]
  if (keysym === undefined) return
  queueVncKey(keysym, code, down, () => rfb?.sendKey(keysym, code, down))
}

function onVisibilityChange() {
  if (document.hidden) onFocusOut()
  else onFocusIn()
}

function createRFB(RFB: any, proxyAddr: string, password: string, forceRfb33 = false) {
  if (!vncContainer.value || vncContainer.value.childElementCount > 0) {
    isIniting = false
    return
  }

  try {
    rfb = new RFB(vncContainer.value, toWebSocketURL(proxyAddr), {
      credentials: { password: password || '' },
      shared: props.config?.vncShared ?? true,
      repeaterID: props.config?.vncRepeaterID || '',
    })
  } catch (e: any) {
    console.error('Failed to create RFB instance:', e)
    lastError.value = e?.message || String(e)
    status.value = 'error'
    isIniting = false
    return
  }

  // Smart RFB version negotiation (replaces the old blanket
  // `_rfbMaxVersion = 3.3` hammer):
  //
  // - Default (modern servers): normal 3.8 list negotiation. noVNC picks
  //   from the server's offered security types (VncAuth / None / MS-Logon
  //   II all reachable). Two types are skipped in the preference order:
  //   RA2ne (6) — RealVNC advertises [6,130,2,...] and noVNC's RA2ne
  //   implementation dies with an unexplained 1006 against RealVNC
  //   servers; and VeNCrypt (19) — noVNC only implements the Plain
  //   sub-type while TigerVNC-style servers offer TLS sub-types, so
  //   picking 19 fails the handshake instead of falling through to the
  //   VNC Auth (2) they also offer.
  // - Legacy fallback (forceRfb33, auto-retried once): cap to RFB 3.3 so
  //   legacy servers (UltraVNC 003.003/003.006 banners) that reject 3.8
  //   viewers or single-handedly dictate a security type still connect.
  //   This preserves the original RealVNC workaround as a fallback instead
  //   of penalising every server.
  if (forceRfb33) {
    rfb._rfbMaxVersion = 3.3
  } else {
    const origIsSupported = rfb._isSupportedSecurityType?.bind(rfb)
    if (typeof origIsSupported === 'function') {
      rfb._isSupportedSecurityType = (type: number) =>
        type !== 6 && type !== 19 && origIsSupported(type)
    }
  }

  applyRFBOptions()

  // issue #95: only flip to 'connected' on the noVNC connect event.
  // Previously status='connected' was set as soon as the local proxy was
  // up, well before the RFB handshake completed, producing a black
  // screen with no error on security failure or wrong password.
  rfb.addEventListener('connect', () => {
    lastError.value = ''
    status.value = 'connected'
    everConnected = true
  })

  rfb.addEventListener('securityfailure', (e: any) => {
    const reason = e.detail?.reason ?? e.detail?.status
    lastError.value = t('vnc.errorSecurity', { reason: String(reason ?? '') })
    status.value = 'error'
  })

  rfb.addEventListener('credentialsrequired', (e: any) => {
    const types = (e.detail?.types || []).join(', ')
    lastError.value = t('vnc.errorCredentials', { types })
    status.value = 'error'
  })

  // Auto-downgrade retry: if the 3.8 negotiation dies without ever
  // reaching 'connected' (typical symptom: unexplained 1006 close or a
  // legacy server that refuses modern viewers), rebuild once under
  // RFB 3.3 before surfacing the error to the user.
  rfb.addEventListener('disconnect', (e: any) => {
    if (!e.detail.clean && !everConnected && !rfb33Retried && currentProxyAddr) {
      rfb33Retried = true
      status.value = 'connecting'
      // Give the backend VNCProxy a beat to release the previous
      // WebSocket/TCP pair before reconnecting through it.
      setTimeout(() => {
        initRFB(currentProxyAddr!, savedPassword.value, true)
      }, 500)
      return
    }
    if (!e.detail.clean) {
      if (!lastError.value) lastError.value = t('vnc.errorClosed')
      status.value = 'error'
    } else {
      status.value = 'disconnected'
    }
  })

  isIniting = false
}

function onPaste(e: ClipboardEvent) {
  const text = e.clipboardData?.getData('text')
  if (!text || !rfb || !ownsRemoteKeyboard()) return
  e.preventDefault()
  const target = rfb
  inputQueue.enqueue(() => syncLocalClipboard(text))
  inputQueue.enqueue(() => {
    target.sendKey(0xffe3, 'ControlLeft', true)
    target.sendKey(0x76, 'KeyV', true)
    target.sendKey(0x76, 'KeyV', false)
    target.sendKey(0xffe3, 'ControlLeft', false)
  })
}

function onReconnectEvent(e: Event) {
  const panelId = (e as CustomEvent)?.detail?.panelId
  if (panelId && panelId === props.panelId) reconnect()
}

function focusDesktop() {
  if (status.value === 'connected') rfb?.focus({ preventScroll: true })
}

function blurDesktop() {
  onFocusOut()
  rfb?.blur()
}

function onToolbarError(error: unknown) {
  console.error('VNC minimize error:', error)
}

function onFullscreenChange() {
  isFullscreen.value = document.fullscreenElement === container.value
  if (keyboardCaptureId && currentSessionId.value) {
    // Apply the new fullscreen flag immediately instead of waiting for the
    // next capture heartbeat, so Escape is forwarded right away.
    const captureID = keyboardCaptureId
    const sessionID = currentSessionId.value
    queueRemoteKeyboardChange(() => SetVNCKeyboardCapture(sessionID, captureID, true, isFullscreen.value))
      .catch(error => console.error('VNC keyboard capture error:', error))
  }
}

async function toggleFullscreen() {
  const el = container.value
  if (!el) return
  try {
    if (document.fullscreenElement === el) await document.exitFullscreen()
    else await el.requestFullscreen()
    focusDesktop()
  } catch (error) {
    console.error('VNC fullscreen error:', error)
  }
}

function onFullscreenEvent(event: Event) {
  if ((event as CustomEvent<{ panelId: string }>).detail?.panelId === props.panelId) void toggleFullscreen()
}

onMounted(() => {
  unsubKeyboard = Events.On('remote-desktop:keyboard', event => onNativeKey(event.data as NativeRdpKey))
  window.addEventListener('blur', onFocusOut)
  window.addEventListener('focus', onFocusIn)
  document.addEventListener('visibilitychange', onVisibilityChange)
  clipboardPoll = setInterval(() => {
    syncLocalClipboard().catch(error => console.error('VNC clipboard sync error:', error))
  }, 1000)
  onPixelRatioChange()
  document.addEventListener('fullscreenchange', onFullscreenChange)
  window.addEventListener('vnc:fullscreen', onFullscreenEvent)
  if (props.sessionId) {
    currentSessionId.value = props.sessionId
  }

  const cached = panelStore.getVNCCache(props.panelId)
  if (cached && vncContainer.value) {
    const children = Array.from(cached.container.children)
    children.forEach(child => vncContainer.value!.appendChild(child))
    rfb = cached.rfb
    panelStore.removeVNCCache(props.panelId)
    status.value = 'connected'
    // Cached RFB instances already completed their handshake; mark them so
    // a later disconnect doesn't fire the 3.3 downgrade retry against a
    // stale proxy address.
    everConnected = true
    applyRFBOptions()
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
  if (storedProxy && props.config) {
    savedPassword.value = props.config.password || ''
    initRFB(storedProxy, savedPassword.value)
  } else {
    connect()
  }

  watchThemeBackground()

  // Tab right-click 「重连」(Reconnect) menu → forced reconnect of this panel.
  window.addEventListener('panel:reconnect', onReconnectEvent)
})

onBeforeUnmount(() => {
  onFocusOut()
  restoreDesktopInput?.()
  restoreDesktopInput = null
  unsubKeyboard?.()
  if (clipboardPoll !== null) clearInterval(clipboardPoll)
  clipboardPoll = null
  window.removeEventListener('blur', onFocusOut)
  window.removeEventListener('focus', onFocusIn)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  pixelRatioQuery?.removeEventListener('change', onPixelRatioChange)
  pixelRatioQuery = null
  displayController?.dispose()
  displayController = null
  restorePixelScaling?.()
  restorePixelScaling = null
  restoreCursor?.()
  restoreCursor = null
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  window.removeEventListener('vnc:fullscreen', onFullscreenEvent)
  themeObserver?.disconnect()
  themeObserver = null
  unsubStatus?.()
  window.removeEventListener('panel:reconnect', onReconnectEvent)

  if (rfb && vncContainer.value && vncContainer.value.childElementCount > 0) {
    const container = document.createElement('div')
    container.style.display = 'none'
    const children = Array.from(vncContainer.value.children)
    children.forEach(child => container.appendChild(child))
    document.body.appendChild(container)
    panelStore.setVNCCache(props.panelId, { rfb, container })
  } else if (rfb) {
    rfb.disconnect()
    rfb = null
  }
})

watch(() => props.sessionId, (newId) => {
  if (newId && !currentSessionId.value) {
    currentSessionId.value = newId
  }
})

watch(zoom, () => {
  if (!rfb) return
  rfb.scaleViewport = scaleViewport.value
  rfb._updateScale?.()
})
watch(viewOnly, (val) => {
  if (val) onFocusOut()
  if (rfb) rfb.viewOnly = val
  if (!val) onFocusIn()
})
watch(showDotCursor, (val) => {
  if (rfb) rfb.showDotCursor = val
})
watch(status, value => {
  if (value === 'connected') void nextTick(onFocusIn)
  else { regionEditor.value = null; onFocusOut() }
})
onDeactivated(onFocusOut)
onActivated(onFocusIn)
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
  /* Drop the double-tap-zoom / tap-delay behavior on touch devices. */
  touch-action: manipulation;
}
.vnc-area :deep(canvas) {
  display: block;
  flex-shrink: 0;
}
.vnc-area.is-unscaled :deep(canvas) {
  image-rendering: pixelated;
}
.vnc-tab-content.is-fullscreen .vnc-area,
.vnc-tab-content.is-fullscreen .vnc-overlay {
  bottom: 0;
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
  white-space: nowrap;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: none;
  z-index: 5;
}
.vnc-statusbar::-webkit-scrollbar { display: none; }
.vnc-statusbar :deep(.el-switch) { height: 1.25rem; }
.vnc-status-sep { color: var(--text-disabled); }
.vnc-status-label { font-size: 0.6875rem; }
.vnc-status-spacer { flex: 1; }
.vnc-fullscreen-btn {
  border: 1px solid var(--border-default);
  background: transparent;
  color: var(--text-muted);
  font-size: 0.75rem;
  padding: 1px 0.625rem;
  border-radius: var(--radius-sm);
}
.vnc-fullscreen-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
</style>
