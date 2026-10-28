import { queuedSessionWrite } from './sessionWriter'
import { getManagedTerminal } from './terminalManager'
import { observeCommandInput } from './terminalCommandWrap'

// Wraps queuedSessionWrite with command-input observation for hard-wrap
// tracking (command-wrap feature). Only active terminals (those with live
// component refs) are observed, matching the upstream BaseTerminal.vue
// wrapper. terminalManager's own mirror writes bypass this and call
// queuedSessionWrite directly, since mirrored output is not user input.
export function queuedSessionWriteObserved(sessionId: string, data: string): void {
  const managed = getManagedTerminal(sessionId)
  if (managed && managed.activeRefs.size > 0) observeCommandInput(managed.terminal, data)
  return queuedSessionWrite(sessionId, data)
}