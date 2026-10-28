import { ref } from 'vue'
import type { Terminal } from '@xterm/xterm'
import type { Panel } from '../types/workspace'
import {
  WriteFileBase64,
  SaveFileDialog,
  EnableSessionOutputLog,
  DisableSessionOutputLog,
  GetSessionOutputLogInfo,
  OpenPathInExplorer,
} from '../../bindings/github.com/ys-ll/uniterm/app'
import { tryAndroidSave } from '../utils/androidSave'
import { msg } from '../services/message'
import { usePanelStore } from '../stores/panelStore'
import { queuedSessionWriteObserved as queuedSessionWrite } from '../services/terminalCommandWrite'
import { clearRegistry } from '../services/terminalTimestamps'
import { useI18n } from '../i18n'

export interface UseTerminalRecorderOptions {
  getTerminal: () => Terminal | null
  getPanelId: () => string | undefined
  getPanel: () => Panel | undefined
  getSessionId: () => string | undefined
  closeMenu: () => void
  /** Whether a zmodem transfer is active for the given session. */
  isZmodemActive: (sessionId: string) => boolean
}

/**
 * Terminal output recording / session log + buffer export operations.
 *
 * Bundles the "session output log" (start/stop/open-dir) feature with the
 * MobaXterm-style export/reset/clear-scrollback buffer operations. All of
 * these only touch the xterm buffer and backend log APIs — they don't depend
 * on the input pipeline or resize, so they extract cleanly.
 */
export function useTerminalRecorder(options: UseTerminalRecorderOptions) {
  const { getTerminal, getPanelId, getPanel, getSessionId, closeMenu, isZmodemActive } = options

  const { t } = useI18n()
  const panelStore = usePanelStore()

  const isOutputLogOn = ref(false)
  const outputLogPath = ref('')
  let exporting = false

  function toBase64(str: string): string {
    const encoder = new TextEncoder()
    const bytes = encoder.encode(str)
    let binary = ''
    for (let i = 0; i < bytes.length; i++) {
      binary += String.fromCharCode(bytes[i])
    }
    return btoa(binary)
  }

  async function exportContent() {
    const terminal = getTerminal()
    if (!terminal || exporting) return
    exporting = true
    try {
      let content = ''
      try {
        const buffer = terminal.buffer.active
        const totalLines = buffer.length
        const lines = new Array(totalLines)
        for (let i = 0; i < totalLines; i++) {
          const line = buffer.getLine(i)
          lines[i] = line ? line.translateToString() : ''
        }
        content = lines.join('\n')
      } catch (e) {
        console.error('Buffer read failed:', e)
        return
      }
      try {
        const b64 = toBase64(content)
        // Android: save through the system "save to" picker instead of the
        // (unsupported) native save dialog.
        const androidOutcome = await tryAndroidSave('terminal.txt', 'text/plain', b64)
        if (androidOutcome !== 'fallback') return
        const filePath = await SaveFileDialog('terminal.txt')
        if (!filePath) return
        await WriteFileBase64(filePath, b64)
      } catch (e) {
        console.error('Export failed:', e)
      }
    } finally {
      exporting = false
    }
  }

  // Shell-idle heuristic for the post-reset Enter: only fire when the cursor
  // sits on the last non-blank row of the normal buffer — i.e. it looks like a
  // fresh prompt with nothing running in the foreground. If a program owns the
  // terminal (vim/less → alternate buffer, or a running command mid-output),
  // injecting Enter would land in that program, so we keep MobaXterm's plain
  // behavior instead: wipe and wait for the next real keypress.
  function shellLooksIdle(): boolean {
    const terminal = getTerminal()
    if (!terminal) return false
    const buf = terminal.buffer.active
    if (buf.type === 'alternate') return false
    const cursorLine = buf.getLine(buf.baseY + buf.cursorY)
    const text = cursorLine ? (cursorLine.translateToString(true) || '').trim() : ''
    if (!text) return false
    // The cursor row must be the last row holding content.
    let lastContent = buf.length - 1
    while (lastContent > 0) {
      const line = buf.getLine(lastContent)
      if (line && (line.translateToString(true) || '').trim()) break
      lastContent--
    }
    return buf.baseY + buf.cursorY >= lastContent
  }

  // Reset terminal output (MobaXterm-style, issue #989): wipe the screen AND
  // the scrollback, then home the cursor. The wipe is client-side only — the
  // shell never learns the screen was cleared, so when it looks idle send an
  // Enter for the prompt to redraw immediately (MobaXterm stops here and
  // waits for the next keypress; we only inject the Enter when it is safe).
  function resetTerminalOutput() {
    const terminal = getTerminal()
    if (!terminal) return
    const idle = shellLooksIdle()
    const scrollback = terminal.options.scrollback
    terminal.clear()
    terminal.options.scrollback = 0
    terminal.clear()
    terminal.options.scrollback = scrollback
    terminal.write('\r\x1b[H\x1b[2J')
    const sid = getSessionId()
    if (sid) clearRegistry(sid)
    if (sid && idle && !isZmodemActive(sid)) {
      queuedSessionWrite(sid, '\n')
    }
  }

  // Clear scrollback only (issue #989): the visible screen stays; rows above
  // the viewport are dropped. ED3 (erase saved lines) does exactly this — a
  // client-side write, so the \x1b[3J strip in renderTerminalData (which only
  // filters session data) does not apply. terminal.clear() must NOT be used
  // here: it wipes the viewport as well, leaving only the cursor line.
  function clearScrollback() {
    const terminal = getTerminal()
    if (!terminal) return
    terminal.write('\x1b[3J')
    const sid = getSessionId()
    if (sid) clearRegistry(sid)
  }

  async function refreshOutputLogState() {
    const pid = getPanelId()
    const p = getPanel()
    if (!pid || !p) {
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
    closeMenu()
    const pid = getPanelId()
    if (!pid) return
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
    closeMenu()
    if (!outputLogPath.value) return
    try {
      await OpenPathInExplorer(outputLogPath.value)
    } catch (e: any) {
      msg.error(String(e?.message ?? e))
    }
  }

  return {
    isOutputLogOn,
    refreshOutputLogState,
    toggleOutputLog,
    openLogDir,
    exportContent,
    resetTerminalOutput,
    clearScrollback,
  }
}