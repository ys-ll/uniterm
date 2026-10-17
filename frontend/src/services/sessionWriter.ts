import { SessionWrite } from '../../bindings/github.com/ys-ll/uniterm/app'
import { isWebMode, webClientId } from '../utils/webMode'
import { webTerminalBridge } from './webTerminalBridge'

// Wails calls run in independent goroutines. Serialize input, replies and
// resize requests per session; an automatic reply must never claim its grid.
export const writeFn: { current: (id: string, data: string) => Promise<unknown> } = { current: SessionWrite }
const tails = new Map<string, Promise<unknown>>()
type InputSize = { cols: number; rows: number; generation?: number }
const sizes = new Map<string, { measure: () => InputSize; shared: boolean }>()

export function setSessionInputSizeProvider(id: string, provider?: () => InputSize, shared = true): void {
  if (provider) sizes.set(id, { measure: provider, shared })
  else sizes.delete(id)
}

export function enqueueSessionOperation(id: string, operation: () => Promise<unknown>, requiresReadyView = false): Promise<void> {
  const checkView = requiresReadyView && isWebMode()
  const view = sizes.get(id)
  // Check both when input is offered and when it reaches the transport. Never
  // replay keystrokes queued before a disconnect into a newly restored screen.
  let generation: number | undefined
  try {
    if (checkView && !view) throw new Error('Terminal view is not attached; input was not sent.')
    generation = checkView ? view!.measure().generation : undefined
  } catch (error) { return Promise.reject(error) }
  const task = (tails.get(id) ?? Promise.resolve()).catch(() => {}).then(() => {
    if (checkView && (sizes.get(id) !== view || view!.measure().generation !== generation)) {
      throw new Error('Terminal view changed; input was not sent. Refresh the view and retry.')
    }
    return operation()
  }).then(() => {})
  tails.set(id, task)
  const finish = () => { if (tails.get(id) === task) tails.delete(id) }
  task.then(finish, finish)
  // Return this exact promise: error reporting must pause a failed browser
  // before the next queued operation, without an async-wrapper microtask hop.
  return task
}

function report(id: string, error: unknown): void {
  console.error('[terminal input]', id, error)
  if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent('terminal:sharing-error', { detail: { id, error: String(error) } }))
}

export function queuedSessionWrite(id: string, data: string, binary = false): void {
  if (!id || !data) return
  void writeSessionInput(id, data, binary).catch(error => report(id, error))
}

/** Prepared drops use the same manual-input lane. Preparation must finish before
 * rz, and subsequent human input cannot overtake it. Automatic replies bypass it.
 * WebTerminalInput selects the file target under its backend per-session lock. */
export function writeSessionInput(id: string, data: string, binary = false, prepare?: () => Promise<unknown>): Promise<void> {
  if (!id || !data) return Promise.resolve()
  const view = prepare ? sizes.get(id) : undefined
  let generation: number | undefined
  try { generation = view?.measure().generation } catch (error) { return Promise.reject(error) }
  return enqueueSessionOperation(id, async () => {
    if (prepare) {
      await prepare()
      if (isWebMode() && (sizes.get(id) !== view || view?.measure().generation !== generation)) {
        throw new Error('Terminal view changed during file preparation; input was not sent.')
      }
    }
    const size = sizes.get(id)
    const { cols = 0, rows = 0 } = size?.measure() ?? {}
    return size?.shared === false ? writeFn.current(id, data) : webTerminalBridge.input(id, isWebMode() ? webClientId() : 'desktop', binary ? btoa(data) : data, cols, rows, binary)
  }, true)
}

export function queuedSessionReply(id: string, data: string): void {
  if (isWebMode() || !id || !data) return
  void enqueueSessionOperation(id, () => writeFn.current(id, data)).catch(error => report(id, error))
}

export function resizeSessionView(id: string, cols: number, rows: number, active = false): Promise<void> {
  return enqueueSessionOperation(id, () => webTerminalBridge.resize(id, isWebMode() ? webClientId() : 'desktop', cols, rows, active), true)
}
