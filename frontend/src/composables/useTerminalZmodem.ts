import type { Terminal } from '@xterm/xterm'
import { SessionEndZmodem } from '../../bindings/github.com/ys-ll/uniterm/app'
import { queuedSessionWriteObserved as queuedSessionWrite } from '../services/terminalCommandWrite'
import { startZmodemService } from '../services/zmodemService'
import { useZmodemStore } from '../stores/zmodemStore'
import { useSettingsStore } from '../stores/settingsStore'

export interface UseTerminalZmodemOptions {
  getTerminal: () => Terminal | null
  getMode: () => 'ssh' | 'sftp' | 'local'
  getSessionId: () => string | null | undefined
  /** Render terminal data — used to flush deferred output after zmodem restore. */
  renderTerminalData: (data: string, countChunk?: boolean) => void
}

/**
 * Zmodem file-transfer service lifecycle and session:data interception.
 *
 * Owns all zmodem state (service handle, direction, cancel/restore gates,
 * deferred output buffer) and the HEX-header detection that flips a session
 * into binary transfer mode. The host routes session:data through
 * handleSessionData() first — a true return means the chunk was consumed by
 * zmodem and must not be rendered. handleCommand() detects rz/sz in the
 * command line to preset the transfer direction.
 */
export function useTerminalZmodem(options: UseTerminalZmodemOptions) {
  const { getTerminal, getMode, getSessionId, renderTerminalData } = options

  const zmodemStore = useZmodemStore()
  const settingsStore = useSettingsStore()

  let zmodemService: ReturnType<typeof startZmodemService> | null = null
  let isZmodemStarting = false
  let zmodemStartTimer: ReturnType<typeof setTimeout> | null = null
  let zmodemDirection: 'upload' | 'download' | undefined = undefined
  let zmodemCancellingUntil = 0
  let zmodemRestoringOutput = false
  let zmodemCompletionPending = false
  const zmodemDeferredOutput: string[] = []

  function initZmodemService(sessionId: string) {
    if (!sessionId || getMode() !== 'ssh') return
    // Don't create a duplicate zmodem service if a transfer is already
    // active for this session. The existing service (in a deactivated
    // BaseTerminal) continues to handle the transfer.
    if (zmodemStore.getActiveTransfer(sessionId)) return
    zmodemService = startZmodemService({
      // Register abort so any BaseTerminal component can cancel the transfer
      onRegister: (abort) => zmodemStore.registerAbort(sessionId, abort),
      onUnregister: () => zmodemStore.unregisterAbort(sessionId),
      sessionId,
      direction: zmodemDirection,
      getDefaultDownloadDir: () => settingsStore.settings.terminal.zmodemDownloadDir,
      onTerminalRestoreState: restoring => {
        zmodemRestoringOutput = restoring
        if (!restoring) {
          for (const data of zmodemDeferredOutput.splice(0)) renderTerminalData(data)
          if (zmodemCompletionPending) {
            zmodemCompletionPending = false
            void disposeZmodemService(sessionId, true, false)
            initZmodemService(sessionId)
          }
        }
      },
      onWarning: warning => {
        getTerminal()?.write(`\r\n\x1b[33mZmodem: ${warning}\x1b[0m\r\n`)
      },
      onComplete: (files, hint) => {
        if (files.length > 0) {
          getTerminal()?.write(`\r\n\x1b[32mZmodem: ${files.length} file(s) transferred\x1b[0m\r\n`)
        }
        if (hint) {
          getTerminal()?.write(`\r\n\x1b[33m${hint}\x1b[0m\r\n`)
        }
        if (files.length === 0 && !hint) {
          // 取消或未选择文件：打印提示
          getTerminal()?.write(`\r\n\x1b[33mZmodem transfer cancelled\x1b[0m\r\n`)
          // 等吞数据保护过期后再发送一次回车，确保 sz 已退出、bash 恢复前台后触发提示符
          const cancelUntil = Math.max(zmodemCancellingUntil, zmodemStore.getCancelUntil(sessionId))
          const remaining = Math.max(0, cancelUntil - Date.now())
          setTimeout(() => {
            queuedSessionWrite(sessionId, '\n')
          }, remaining + 100)
        }
        zmodemStore.clearTransfers(sessionId)
        zmodemDirection = undefined
        if (zmodemRestoringOutput) {
          // Keep the binary listener alive until finishTransfer has drained any
          // prompt event queued concurrently with the backend mode handoff.
          zmodemCompletionPending = true
        } else {
          void disposeZmodemService(sessionId, true, false)
          initZmodemService(sessionId)
        }
      },
      onError: (err) => {
        zmodemCompletionPending = false
        getTerminal()?.write(`\r\n\x1b[31mZmodem error: ${err}\x1b[0m\r\n`)
        zmodemStore.clearTransfers(sessionId)
        zmodemDirection = undefined
        void disposeZmodemService(sessionId, true, false)
        initZmodemService(sessionId)
      },
    })
  }

  async function disposeZmodemService(sessionId: string, resetDirection = true, endSession = true) {
    const service = zmodemService
    const serviceDisposed = service?.dispose()
    zmodemService = null
    isZmodemStarting = false
    zmodemCompletionPending = false
    if (resetDirection) {
      zmodemDirection = undefined
    }
    if (zmodemStartTimer) {
      clearTimeout(zmodemStartTimer)
      zmodemStartTimer = null
    }
    // A service that started binary mode also ends it, ordered after its pending
    // start. Only use the direct fallback when no service owns that lifecycle.
    if (!service && sessionId && endSession) {
      await SessionEndZmodem(sessionId).catch(() => {})
    }
    await serviceDisposed
  }

  function onZmodemCancel() {
    const ts = Date.now() + 2000
    zmodemCancellingUntil = ts
    const sid = getSessionId()
    if (sid) {
      zmodemStore.setCancelUntil(sid, ts)
      zmodemStore.abortTransfer(sid)
    }
  }

  /**
   * Intercept session:data for zmodem handling. Returns true when the chunk
   * was consumed (cancelled, deferred, or detected as zmodem traffic) and
   * must not be rendered to the terminal; false to let the host render it.
   */
  function handleSessionData(data: string): boolean {
    const sid = getSessionId() || ''

    // 取消后 2 秒内吞掉所有数据，防止残余二进制乱码
    if (Date.now() < zmodemCancellingUntil) return true

    // Keep output arriving after EndZmodem behind the restored prompt.
    if (zmodemRestoringOutput) {
      zmodemDeferredOutput.push(data)
      return true
    }

    // tab 切换后服务还没重建，但 store 里还有活跃传输（旧的 handleReceive 还在跑），先吞数据
    const hasStoreTransfer = zmodemStore.getActiveTransfer(sid)
    if (!zmodemService && hasStoreTransfer) return true

    // Zmodem detection: scan for HEX header in normal terminal output.
    // Skip if the service was aborted (waiting for the next rz/sz command).
    const activeZmodem = zmodemService && zmodemStore.getActiveTransfer(sid)
    if (zmodemService && !activeZmodem && !zmodemService.isAborted?.()) {
      if (isZmodemStarting) {
        // Already detected header and waiting for SessionStartZmodem / on_detect.
        // Feed subsequent data to sentry so it can complete detection without
        // re-processing the header heuristic on every retry frame.
        zmodemService.consume(data)
        return true
      }
      // Real ZMODEM headers always contain the ZDLE control byte (0x18)
      // before the frame type: `**` (ZPAD ZPAD) `\x18` (ZDLE) `[ABC]` then
      // hex digits. Requiring the 0x18 avoids false positives on ordinary
      // terminal output — e.g. vim rendering a file whose content happens to
      // contain `**B<hex>` — which previously flipped the session into binary
      // ZMODEM mode and made the sentry write protocol bytes back to the
      // server, crashing the remote shell (issue #242).
      const ZMODEM_HEX_RE = /\*{2,}\x18[ABC][0-9a-fA-F]{10,}/
      if (ZMODEM_HEX_RE.test(data)) {
        console.debug('[zmodem] header detected, entering transfer mode')
        isZmodemStarting = true
        if (zmodemStartTimer) clearTimeout(zmodemStartTimer)
        zmodemStartTimer = setTimeout(() => {
          isZmodemStarting = false
        }, 3000)
        const sessionId = getSessionId()
        if (sessionId) {
          // Consume immediately to avoid losing data during async handoff
          zmodemService.start(data)
        }
        // Hide zmodem data from terminal
        return true
      }
    }

    // If zmodem is active, skip writing data to terminal (data comes via session:binary)
    if (activeZmodem) {
      isZmodemStarting = false
      return true
    }

    return false
  }

  /**
   * Detect rz/sz in the command line to preset zmodem transfer direction.
   * Called from the keystroke pipeline right before the line buffer is
   * cleared by Enter.
   */
  function handleCommand(line: string) {
    if (/^(?:sudo\s+)?rz\b/.test(line)) {
      zmodemDirection = 'upload'
      // Recreate the zmodem service so on_detect sees the new direction.
      const sid = getSessionId()
      if (sid) {
        disposeZmodemService(sid, false).then(() => {
          initZmodemService(sid)
        })
      }
    } else if (/^(?:sudo\s+)?sz\b/.test(line)) {
      zmodemDirection = 'download'
      const sid = getSessionId()
      if (sid) {
        disposeZmodemService(sid, false).then(() => {
          initZmodemService(sid)
        })
      }
    }
  }

  return {
    initZmodemService,
    disposeZmodemService,
    onZmodemCancel,
    handleSessionData,
    handleCommand,
    hasService: () => zmodemService !== null,
  }
}