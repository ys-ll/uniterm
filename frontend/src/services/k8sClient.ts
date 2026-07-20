import {
  K8sListContexts,
  K8sConnect,
  K8sDisconnect,
  K8sRequest,
  K8sStartWatch,
  K8sStopWatch,
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import type { K8sContextInfo, K8sWatchEvent } from '../types/k8s'

export async function listContexts(source: string, sourceIsPath: boolean): Promise<K8sContextInfo[]> {
  return await K8sListContexts(source, sourceIsPath) as K8sContextInfo[]
}

export async function connect(
  source: string,
  sourceIsPath: boolean,
  contextName: string,
  tunnelSSHConnId = '',
  tunnelSSHUser = '',
  tunnelSSHPassword = ''
): Promise<string> {
  return await K8sConnect(source, sourceIsPath, contextName, tunnelSSHConnId, tunnelSSHUser, tunnelSSHPassword)
}

export function disconnect(connID: string): void {
  K8sDisconnect(connID)
}

export async function requestJSON<T = any>(
  connID: string,
  method: string,
  path: string,
  body: any = null,
  contentType = ''
): Promise<{ status: number; data: T | null; raw: string }> {
  const rawBody = body == null ? '' : (typeof body === 'string' ? body : JSON.stringify(body))
  const resp = await K8sRequest(connID, method, path, rawBody, contentType)
  let data: T | null = null
  try {
    data = resp.body ? JSON.parse(resp.body) as T : null
  } catch {
    data = null
  }
  return { status: resp.status, data, raw: resp.body }
}

export interface WatchHandle {
  id: string
  stop: () => void
}

export async function startWatch(
  connID: string,
  path: string,
  onEvent: (ev: K8sWatchEvent) => void,
  onEnd?: (err: string) => void
): Promise<WatchHandle> {
  const id = await K8sStartWatch(connID, path)
  const eventName = `k8s:watch:${id}`
  const endName = `k8s:watch-end:${id}`
  EventsOn(eventName, (payload: K8sWatchEvent) => onEvent(payload))
  EventsOn(endName, (payload: { error: string }) => {
    onEnd?.(payload?.error || '')
  })
  return {
    id,
    stop: () => {
      EventsOff(eventName)
      EventsOff(endName)
      K8sStopWatch(id)
    },
  }
}
