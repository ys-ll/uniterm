import { ref } from 'vue'
import {
  EnableSessionOutputLog,
  DisableSessionOutputLog,
  GetSessionOutputLogInfo,
  OpenSessionLogDir,
  OpenPathInExplorer,
} from '../../bindings/github.com/ys-ll/uniterm/app'
import { msg } from '../services/message'
import { usePanelStore } from '../stores/panelStore'
import { useI18n } from '../i18n'

export interface UseOutputLogOptions {
  getPanelId: () => string | undefined
}

/**
 * Session output log state + operations for one terminal panel.
 *
 * Single source of truth for the "日志与录制" menu entries shared by
 * BaseTerminal (terminal right-click), Panel (panel menu) and TabItem
 * (tab context menu). Reads fresh state from the backend before acting
 * so a stale local ref can never send the toggle the wrong way or make
 * "open log dir" a silent no-op.
 */
export function useOutputLog(options: UseOutputLogOptions) {
  const { getPanelId } = options

  const { t } = useI18n()
  const panelStore = usePanelStore()

  const isOutputLogOn = ref(false)
  const outputLogPath = ref('')

  async function refreshOutputLogState() {
    const pid = getPanelId()
    if (!pid) {
      isOutputLogOn.value = false
      outputLogPath.value = ''
      return
    }
    try {
      const info = await GetSessionOutputLogInfo(pid)
      isOutputLogOn.value = !!info.enabled
      outputLogPath.value = info.path || ''
      panelStore.setOutputLog(pid, { enabled: isOutputLogOn.value, path: outputLogPath.value })
    } catch {
      isOutputLogOn.value = false
      outputLogPath.value = ''
    }
  }

  async function toggleOutputLog() {
    const pid = getPanelId()
    if (!pid) return
    // Re-read the live state first: the menu may have been opened a while
    // ago and refreshOutputLogState only ran asynchronously at open time.
    await refreshOutputLogState()
    try {
      if (isOutputLogOn.value) {
        await DisableSessionOutputLog(pid)
        isOutputLogOn.value = false
        const prev = outputLogPath.value
        outputLogPath.value = ''
        panelStore.setOutputLog(pid, { enabled: false, path: '' })
        msg.copyable(t('session.logStopped', { path: prev }), 'info')
        return
      }
      const path = await EnableSessionOutputLog(pid, '')
      if (!path) {
        msg.error(t('session.logFailed', { error: 'unknown' }))
        return
      }
      isOutputLogOn.value = true
      outputLogPath.value = path
      panelStore.setOutputLog(pid, { enabled: true, path })
      msg.copyable(t('session.logStarted', { path }), 'success')
    } catch (e: any) {
      msg.error(t('session.logFailed', { error: String(e?.message ?? e) }))
    }
  }

  async function openLogDir() {
    // Re-read the live state instead of trusting the (possibly stale)
    // ref. With no active log, open the effective log directory (user
    // override or OS default), creating it on demand — previously this
    // either silently returned or pointed explorer at a nonexistent
    // folder, which looks like the menu doing nothing (issue #1525).
    await refreshOutputLogState()
    const path = outputLogPath.value
    try {
      if (path) {
        await OpenPathInExplorer(path)
      } else {
        await OpenSessionLogDir()
      }
    } catch (e: any) {
      msg.error(String(e?.message ?? e))
    }
  }

  return {
    isOutputLogOn,
    outputLogPath,
    refreshOutputLogState,
    toggleOutputLog,
    openLogDir,
  }
}
