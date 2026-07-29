import { runtimeLog } from '../services/runtimeLog'

export async function call<T>(name: string, fn: () => Promise<T>): Promise<T> {
  const start = performance.now()
  try {
    const r = await fn()
    runtimeLog.record(name, performance.now() - start)
    return r
  } catch (e) {
    runtimeLog.record(name, performance.now() - start, e)
    throw e
  }
}

export { runtimeLog }