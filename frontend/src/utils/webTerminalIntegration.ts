import { t } from '../i18n'
import { isWebMode } from './webMode'
import { waitForBrowserEvents } from './browserRuntime'

// Terminal implementation is owned separately. A literal glob allows this
// shell to build independently; it includes the real module at integration.
// Missing integration is a visible error, never a fake attach or new PTY.
const modules = import.meta.glob('../services/webTerminals.ts')
type TerminalHooks = {
  initWebTerminals: () => unknown
  attachWebSession: (sessionId: string) => unknown
}
let hooks: TerminalHooks | undefined

export async function initBrowserTerminalIntegration(): Promise<boolean> {
  const load = modules['../services/webTerminals.ts']
  if (!load) {
    if (isWebMode()) throw new Error(t('web.terminalIntegrationMissing'))
    return false
  }
  if (isWebMode()) await waitForBrowserEvents()
  const loaded = await load() as TerminalHooks
  if (typeof loaded.initWebTerminals !== 'function' || typeof loaded.attachWebSession !== 'function') {
    throw new Error(t('web.terminalIntegrationMissing'))
  }
  await loaded.initWebTerminals()
  hooks = loaded
  return true
}

export async function attachBrowserTerminal(sessionId: string): Promise<void> {
  if (!hooks) throw new Error(t('web.terminalIntegrationMissing'))
  await waitForBrowserEvents()
  await hooks.attachWebSession(sessionId)
}
