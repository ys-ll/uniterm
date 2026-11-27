// Runs fn when the browser is idle — after first paint and the startup-critical
// work has settled. Used to take non-essential startup work (shell probes,
// update checks, chunk preloads) off the critical path so it neither delays
// first paint nor competes with it for IPC/renderer time. timeoutMs bounds how
// long the task may stay queued when the page never goes idle (busy renders);
// Chromium fires it anyway at the deadline.
export function deferToIdle(fn: () => void, timeoutMs = 3000) {
  if (typeof requestIdleCallback === 'function') {
    requestIdleCallback(() => fn(), { timeout: timeoutMs })
  } else {
    setTimeout(fn, Math.min(timeoutMs, 2000))
  }
}
