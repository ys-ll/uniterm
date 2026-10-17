import { Events } from '@wailsio/runtime'
import { reactive } from 'vue'
import { t } from '../i18n'

export interface WebBootstrap {
  instanceId: string
  clientId: string
  ready: boolean
  eventsUrl: string
  capabilities: Record<string, unknown>
}

interface WebEvent {
  name: string
  data: unknown
  sender?: string
  sequence: number
}

export const browserConnection = reactive({ connected: false, error: '' })
let stream: EventSource | undefined
let ready: Promise<void> | undefined

export async function loadWebBootstrap(): Promise<WebBootstrap> {
  const response = await fetch('/web/bootstrap', { credentials: 'same-origin', cache: 'no-store' })
  if (response.status === 401 || response.status === 403) {
    window.location.replace('/web/login')
    throw new Error(t('web.loginRequired'))
  }
  if (!response.ok) throw new Error(t('web.bootstrapFailed', { status: response.status }))
  const data: WebBootstrap = await response.json()
  if (!data.instanceId || !data.clientId || !data.ready) throw new Error(t('web.hostNotReady'))
  const url = new URL(data.eventsUrl, window.location.origin)
  if (url.origin !== window.location.origin || url.pathname !== '/web/events' || url.search || url.hash) {
    throw new Error(t('web.invalidEventsUrl'))
  }
  const previousInstance = window.__UNITERM_WEB__?.instanceId
  if (previousInstance && previousInstance !== data.instanceId) window.location.reload()
  window.__UNITERM_WEB__ = { instanceId: data.instanceId, clientId: data.clientId }
  return data
}

export function startBrowserEvents(eventsUrl: string): Promise<void> {
  if (ready) return ready
  // beta.27's dispatcher belongs to the same runtime as generated bindings.
  // Capture it once: importing a second /wails/runtime.js later must not move
  // incoming events to a different module's listener map.
  const dispatch = (window as unknown as {
    _wails?: { dispatchWailsEvent?: (event: { name: string; data: unknown; sender?: string }) => void }
  })._wails?.dispatchWailsEvent
  let verified = false
  const off = Events.Once('uniterm:web-runtime-check', () => { verified = true })
  dispatch?.({ name: 'uniterm:web-runtime-check', data: null })
  off()
  if (!dispatch || !verified) throw new Error(t('web.runtimeUnavailable'))

  let sequence: number | undefined
  let opened = false
  let checkingAuth = false
  let resume: (() => void) | undefined
  const offline = () => {
    if (browserConnection.connected) ready = new Promise<void>(resolve => { resume = resolve })
    sequence = undefined
    browserConnection.connected = false
    browserConnection.error = t('web.reconnecting')
    dispatch({ name: 'web:disconnected', data: null })
  }
  ready = new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(() => {
      if (!opened) {
        stream?.close()
        ready = undefined
        reject(new Error(t('web.eventsUnavailable')))
      }
    }, 15000)
    const connect = () => {
      stream = new EventSource(eventsUrl, { withCredentials: true })
      stream.addEventListener('ready', (event) => {
        try {
          const data = JSON.parse((event as MessageEvent).data)
          if (!Number.isSafeInteger(data.sequence) || data.sequence < 0) throw new Error('Invalid sequence')
          sequence = data.sequence
          browserConnection.connected = true
          browserConnection.error = ''
          // The terminal owner listens locally and snapshots every ready.
          // Never Events.Emit: that would echo back to the host and all peers.
          dispatch({ name: 'web:resync', data: { reason: opened ? 'reconnect' : 'ready', sequence } })
          opened = true
          clearTimeout(timeout)
          resume?.()
          resume = undefined
          resolve()
        } catch {
          reconnect()
        }
      })
      stream.onmessage = (event) => {
        try {
          const frame: WebEvent = JSON.parse(event.data)
          if (typeof frame.name !== 'string' || !frame.name || !Number.isSafeInteger(frame.sequence) || frame.sequence < 0 || sequence === undefined) throw new Error('Invalid event')
          if (frame.sequence <= sequence) return
          if (frame.sequence !== sequence + 1) {
            dispatch({ name: 'web:resync', data: { reason: 'gap', sequence: frame.sequence } })
          }
          sequence = frame.sequence
          // Preserve data exactly, including legacy arrays and webSequence
          // inside the payload. Existing listeners own their normalization.
          dispatch(frame)
        } catch {
          reconnect()
        }
      }
      stream.onerror = () => {
        offline()
        // EventSource cannot expose HTTP status. Recheck auth without putting
        // credentials in URLs; ordinary network outages retain native retry.
        if (!checkingAuth) {
          checkingAuth = true
          void loadWebBootstrap().catch(() => {}).finally(() => { checkingAuth = false })
        }
      }
    }
    const reconnect = () => {
      stream?.close()
      offline()
      connect()
    }
    connect()
  })
  return ready
}

export function waitForBrowserEvents(): Promise<void> {
  if (!ready) return Promise.reject(new Error(t('web.eventsUnavailable')))
  return ready
}

export function stopBrowserEvents() {
  stream?.close()
  stream = undefined
  ready = undefined
  browserConnection.connected = false
}
