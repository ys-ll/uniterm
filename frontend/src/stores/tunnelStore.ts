import { defineStore } from 'pinia'
import { ref } from 'vue'
import { Events } from '@wailsio/runtime'
import {
  StartTunnel, StopTunnel, ListTunnelStates,
} from '../../bindings/github.com/ys-ll/uniterm/app'
import { LoadTunnels, SaveTunnels, configChanged } from '../services/businessConfig'
import { requestId } from '../utils/requestId'
import { msg } from '../services/message'
import { t } from '../i18n'
// Module-level un-subscribers for tunnel:state / store:tunnels:changed listeners.
// Tracked at module scope so re-imports under HMR can detach the previous
// listener before re-subscribing (FE-03).
let unsubTunnelState: (() => void) | null = null
let unsubTunnelsChanged: (() => void) | null = null

export type TunnelMode = 'local' | 'remote' | 'dynamic'
export type TunnelStatus = 'stopped' | 'connecting' | 'running' | 'reconnecting' | 'error'

export interface SocksProxy {
  kind: string // 'socks5' | 'http'
  host: string
  port: number
  user?: string
  pass?: string
}

export interface Tunnel {
  id: string
  name: string
  mode: TunnelMode
  sshConnId: string
  listenHost?: string
  listenPort: number
  targetHost?: string
  targetPort?: number
  upstream?: SocksProxy
  autoStart?: boolean
  autoReconnect?: boolean
  reconnectInterval?: number
  reconnectJitter?: number
  groupId?: string
  sortOrder?: number
}

export interface TunnelGroup {
  id: string
  name: string
  sortOrder?: number
}

export interface TunnelState {
  id: string
  status: TunnelStatus
  localPort?: number
  error?: string
}

function genId(prefix: string): string {
  return `${prefix}-${requestId()}`
}

export const useTunnelStore = defineStore('tunnels', () => {
  const groups = ref<TunnelGroup[]>([])
  const tunnels = ref<Tunnel[]>([])
  const states = ref<Record<string, TunnelState>>({})
  const loaded = ref(false)
  const requests = new Map<string, symbol>()
  let loading: Promise<void> | undefined
  let loadGeneration = 0

  function load(): Promise<void> {
    if (loaded.value) return Promise.resolve()
    if (loading) return loading
    const generation = loadGeneration
    loading = loadInitial(generation).finally(() => {
      if (generation === loadGeneration) loading = undefined
    })
    return loading
  }

  async function loadInitial(generation: number) {
    try {
      const data = await LoadTunnels()
      if (generation !== loadGeneration) return
      groups.value = (data.groups as TunnelGroup[]) || []
      tunnels.value = (data.tunnels as Tunnel[]) || []
    } catch (e) {
      if (generation !== loadGeneration) return
      console.error('Failed to load tunnels:', e)
      groups.value = []
      tunnels.value = []
    }
    const receivedStates = new Set<string>()
    // Live state pushes. Errors are toasted here — one place covers manual
    // starts, auto-start failures at app boot and mid-run disconnects. The
    // bare backend error (e.g. "ssh handshake 10.x.x.x:22: ...") carries no
    // tunnel identity, so the toast prefixes the localized tunnel name; the
    // mid-run disconnect gets its own wording instead of "failed to start".
    let lastToastError = ''
    unsubTunnelState?.()
    unsubTunnelState =Events.On('tunnel:state', (ev) => { const st: TunnelState = ev.data;
      receivedStates.add(st.id)
      // Backend events can supersede responses still in flight, including
      // changes from another window and an immediate post-connect disconnect.
      requests.delete(st.id)
      if (st.status === 'error' && st.error && st.error !== lastToastError) {
        lastToastError = st.error
        const name = tunnels.value.find(x => x.id === st.id)?.name || st.id
        if (st.error === 'ssh chain disconnected') {
          msg.error(t('tunnels.disconnected', { name }))
        } else {
          msg.error(`${t('tunnels.startFailed', { name })}: ${st.error}`)
        }
      }
      states.value = { ...states.value, [st.id]: st }
     })
    // Cross-window sync.
    unsubTunnelsChanged?.()
    unsubTunnelsChanged = Events.On('store:tunnels:changed', () => { void configChanged('tunnels') })
    // Subscribe before the snapshot so an auto-start/reconnect transition
    // cannot be missed or overwritten by an older snapshot response.
    try {
      const list = await ListTunnelStates()
      if (generation !== loadGeneration) return
      for (const st of list) {
        if (!receivedStates.has(st.id)) states.value[st.id] = st as TunnelState
      }
    } catch (e) {
      if (generation !== loadGeneration) return
      console.error('Failed to list tunnel states:', e)
    }
    loaded.value = true
  }

  function dispose() {
    loadGeneration++
    loading = undefined
    loaded.value = false
    unsubTunnelState?.()
    unsubTunnelState = null
    unsubTunnelsChanged?.()
    unsubTunnelsChanged = null
  }

  async function save() {
    try {
      await SaveTunnels({
        version: 1,
        groups: JSON.parse(JSON.stringify(groups.value)),
        tunnels: JSON.parse(JSON.stringify(tunnels.value)),
      } as any)
    } catch (e) {
      console.error('Failed to save tunnels:', e)
    }
  }

  function statusOf(id: string): TunnelStatus {
    return states.value[id]?.status || 'stopped'
  }

  function isActive(id: string): boolean {
    return ['connecting', 'running', 'reconnecting'].includes(statusOf(id))
  }

  function statusLabel(id: string): string {
    const labels: Record<TunnelStatus, string> = {
      stopped: 'tunnels.statusStopped',
      connecting: 'tunnels.statusConnecting',
      running: 'tunnels.statusRunning',
      reconnecting: 'tunnels.statusReconnecting',
      error: 'tunnels.statusError',
    }
    return t(labels[statusOf(id)])
  }

  function statusTitle(id: string): string {
    const error = states.value[id]?.error
    return error ? `${statusLabel(id)}: ${error}` : statusLabel(id)
  }

  function addTunnel(t: Omit<Tunnel, 'id' | 'sortOrder'>): Tunnel {
    const tunnel: Tunnel = {
      ...t,
      id: genId('tun'),
      sortOrder: tunnels.value.filter(x => (x.groupId || undefined) === (t.groupId || undefined)).length,
    }
    tunnels.value.push(tunnel)
    save()
    return tunnel
  }

  function updateTunnel(id: string, patch: Partial<Tunnel>) {
    const idx = tunnels.value.findIndex(t => t.id === id)
    if (idx >= 0) {
      tunnels.value[idx] = { ...tunnels.value[idx], ...patch, id }
      save()
    }
  }

  async function deleteTunnel(id: string) {
    if (!await stop(id)) return
    tunnels.value = tunnels.value.filter(t => t.id !== id)
    await save()
  }

  function getTunnelsByGroup(groupId?: string): Tunnel[] {
    return tunnels.value
      .filter(t => (t.groupId || undefined) === (groupId || undefined))
      .sort((a, b) => (a.sortOrder || 0) - (b.sortOrder || 0))
  }

  function addGroup(name: string): TunnelGroup {
    const group: TunnelGroup = { id: genId('tung'), name, sortOrder: groups.value.length }
    groups.value.push(group)
    save()
    return group
  }

  function renameGroup(id: string, name: string) {
    const g = groups.value.find(x => x.id === id)
    if (g) { g.name = name; save() }
  }

  async function deleteGroup(id: string, deleteTunnels: boolean) {
    if (deleteTunnels) {
      const stopped = await Promise.all(tunnels.value.filter(t => t.groupId === id).map(t => stop(t.id)))
      if (stopped.some(ok => !ok)) return
      tunnels.value = tunnels.value.filter(t => t.groupId !== id)
    } else {
      tunnels.value.forEach(t => { if (t.groupId === id) t.groupId = undefined })
    }
    groups.value = groups.value.filter(g => g.id !== id)
    await save()
  }

  async function start(id: string, user?: string, password?: string): Promise<TunnelState> {
    if (isActive(id)) return states.value[id]
    const request = Symbol(id)
    requests.set(id, request)
    states.value = { ...states.value, [id]: { id, status: 'connecting' } }
    try {
      const st = await StartTunnel(id, user || '', password || '') as TunnelState
      if (requests.get(id) === request) states.value = { ...states.value, [id]: st }
      return st
    } catch (e) {
      const st: TunnelState = { id, status: 'error', error: String(e) }
      if (requests.get(id) === request) {
        states.value = { ...states.value, [id]: st }
        const name = tunnels.value.find(x => x.id === id)?.name || id
        msg.error(`${t('tunnels.startFailed', { name })}: ${st.error}`)
      }
      return st
    } finally {
      if (requests.get(id) === request) requests.delete(id)
    }
  }

  async function stop(id: string) {
    // A slow StartTunnel response must not restore a manually stopped tunnel.
    const request = Symbol(id)
    requests.set(id, request)
    try {
      await StopTunnel(id)
      if (requests.get(id) === request) states.value = { ...states.value, [id]: { id, status: 'stopped' } }
      return true
    } catch (e) {
      console.error('Failed to stop tunnel:', e)
      msg.error(String(e))
      return false
    } finally {
      if (requests.get(id) === request) requests.delete(id)
    }
  }

  return {
    groups, tunnels, states, loaded,
    load, save, statusOf, isActive, statusLabel, statusTitle,
    addTunnel, updateTunnel, deleteTunnel, getTunnelsByGroup,
    addGroup, renameGroup, deleteGroup,
    start, stop,
    dispose,
  }
})
