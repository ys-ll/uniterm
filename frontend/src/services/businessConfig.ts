import { LoadBusinessConfig, SaveBusinessConfig } from '../../bindings/github.com/ys-ll/uniterm/app'
import { msg } from './message'

// Per JS client, not localStorage: in particular, never cache decrypted secrets
// on the browser's disk. One queue preserves the ordering of local autosaves.
const states = new Map<string, { revision: string; tail: Promise<unknown>; loading: Promise<any> | null; pending: number; failed: boolean; notified: boolean }>()
function state(kind: string) {
  let value = states.get(kind)
  if (!value) {
    value = { revision: '', tail: Promise.resolve(), loading: null, pending: 0, failed: false, notified: false }
    states.set(kind, value)
  }
  return value
}

export function loadConfig(kind: string): Promise<any> {
  const s = state(kind)
  if (s.loading) return s.loading
  s.loading = (async () => {
    await s.tail.catch(() => {})
    s.revision = ''
    const result = await LoadBusinessConfig(kind)
    s.revision = result.revision
    s.failed = false
    s.notified = false
    return JSON.parse(result.json)
  })().finally(() => { s.loading = null })
  return s.loading
}

export function saveConfig(kind: string, value: unknown): Promise<void> {
  const json = JSON.stringify(value) // capture before waiting for an earlier save
  const s = state(kind)
  const loading = !!s.loading
  s.pending++
  const pending = s.tail.catch(() => {}).then(async () => {
    try {
      if (loading) throw new Error('Configuration is reloading; wait for the loaded values before saving.')
      if (!s.revision) throw new Error('Load the configuration before saving it.')
      const result = await SaveBusinessConfig(kind, s.revision, json)
      s.revision = result.revision
      s.failed = false
    } catch (error) {
      if (!s.failed) msg.copyable(`${kind}: ${String(error)}. Local edits are retained; copy them before reloading.`, 'error')
      s.failed = true
      throw error
    } finally {
      s.pending--
    }
  })
  s.tail = pending
  // Several existing autosave callers are fire-and-forget. The error is visible
  // above, while await callers still receive the rejection.
  void pending.catch(() => {})
  return pending
}

// Dialogs can retain a copied form. A broadcast must not advance the revision
// belonging to that editor's snapshot, even when the store could refresh.
export async function configChanged(kind: string) {
  const s = state(kind)
  if (s.pending || s.failed || s.notified || !s.revision) return
  try {
    const result = await LoadBusinessConfig(kind)
    if (s.pending || result.revision === s.revision || s.notified) return
    s.notified = true
    msg.copyable(`${kind}: configuration changed elsewhere. Reload before editing; copy any unsaved edits first.`, 'warning')
  } catch (error) {
    msg.error(`${kind}: ${String(error)}`)
  }
}

export const LoadConnections = () => loadConfig('connections')
export const SaveConnections = (data: unknown) => saveConfig('connections', data)
export const LoadIdentities = () => loadConfig('identities')
export const SaveIdentities = (data: unknown) => saveConfig('identities', data)
export const LoadProxies = () => loadConfig('proxies')
export const SaveProxies = (data: unknown) => saveConfig('proxies', data)
export const LoadTunnels = () => loadConfig('tunnels')
export const SaveTunnels = (data: unknown) => saveConfig('tunnels', data)
export const LoadLocalState = () => loadConfig('localState')
export const SaveLocalState = (data: unknown) => saveConfig('localState', data)
export const LoadSettings = () => loadConfig('settings')
export const SaveSettings = (data: unknown) => saveConfig('settings', data)
export const LoadAIModels = () => loadConfig('ai')
export const SaveAIModels = (data: unknown) => saveConfig('ai', data)
export const LoadQuickCommands = () => loadConfig('quickCommands')
export const SaveQuickCommands = (data: unknown) => saveConfig('quickCommands', data)
export const GetFavoriteConnections = () => loadConfig('favorites')
export const SaveFavoriteConnections = (data: unknown) => saveConfig('favorites', data)
export const SyncGetConfig = () => loadConfig('sync')
export const SyncSaveConfig = (config: unknown, token: string) => saveConfig('sync', { config, token })
