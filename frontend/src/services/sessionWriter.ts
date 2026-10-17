import { SessionWrite } from '../../bindings/github.com/ys-ll/uniterm/app'
import { isWebMode, webClientId } from '../utils/webMode'
import { webTerminalBridge } from './webTerminalBridge'

// Wails calls run in independent goroutines. Serialize input, replies and
// resize requests per session; an automatic reply must never claim its grid.
export const writeFn: { current: (id: string, data: string) => Promise<unknown> } = { current: SessionWrite }
const tails = new Map<string, Promise<unknown>>()
const sizes = new Map<string, { measure: () => { cols: number; rows: number }; shared: boolean }>()

export function setSessionInputSizeProvider(id: string, provider?: () => { cols: number; rows: number }, shared = true): void {
  if (provider) sizes.set(id, { measure: provider, shared })
  else sizes.delete(id)
}

export function enqueueSessionOperation(id: string, operation: () => Promise<unknown>): Promise<void> {
  const task = (tails.get(id) ?? Promise.resolve()).catch(() => {}).then(operation).then(() => {})
  tails.set(id, task)
  const finish = () => { if (tails.get(id) === task) tails.delete(id) }
  task.then(finish, finish)
  return task
}

function report(id: string, error: unknown): void {
  console.error('[terminal input]', id, error)
  if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent('terminal:sharing-error', { detail: { id, error: String(error) } }))
}

export function queuedSessionWrite(id: string, data: string, binary = false): void {
  if (!id || !data) return
  const size = sizes.get(id)
  const { cols = 0, rows = 0 } = size?.measure() ?? {}
  void enqueueSessionOperation(id, () => size?.shared === false ? writeFn.current(id, data) : webTerminalBridge.input(id, isWebMode() ? webClientId() : 'desktop', binary ? btoa(data) : data, cols, rows, binary))
    .catch(error => report(id, error))
}

export function queuedSessionReply(id: string, data: string): void {
  if (isWebMode() || !id || !data) return
  void enqueueSessionOperation(id, () => writeFn.current(id, data)).catch(error => report(id, error))
}

export function resizeSessionView(id: string, cols: number, rows: number, active = false): Promise<void> {
  return enqueueSessionOperation(id, () => webTerminalBridge.resize(id, isWebMode() ? webClientId() : 'desktop', cols, rows, active))
}
