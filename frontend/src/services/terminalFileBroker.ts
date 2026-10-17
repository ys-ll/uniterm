import { startZmodemService } from './zmodemService'
import { initWebZmodem } from './webZmodem'
import { finishTerminalFileTransfer, getManagedTerminal, setTerminalBrokerFactory } from './terminalManager'
import { useZmodemStore } from '../stores/zmodemStore'
import { useSettingsStore } from '../stores/settingsStore'
import { isWebMode } from '../utils/webMode'
import { msg } from './message'
import { splitZmodemHeader } from './zmodemHeader'

/** Install after Pinia/settings, before FrontendReady or the first SessionStart.
 * No BaseTerminal lifecycle owns this parser, including hidden authorities. */
export function initTerminalFiles(): void {
  if (isWebMode()) { initWebZmodem(); return }
  setTerminalBrokerFactory(({ id }) => {
    let service: ReturnType<typeof startZmodemService> | undefined
    let disposed = false
    let restoring = false
    let visible = ''
    let visibleTail = ''
    let textStarted = false
    const decoder = new TextDecoder()
    const deferred: Array<() => void> = []
    const store = useZmodemStore()
    const releaseOutput = () => { restoring = false; deferred.splice(0).forEach(resolve => resolve()) }
    const createService = () => {
      service = startZmodemService({
        sessionId: id,
        subscribeBinary: false,
        // Sentry forwards the initial header to to_terminal too. Only its
        // ordinary prefix enters xterm, in the original event's sequence.
        onTerminalData: bytes => {
          const framed = splitZmodemHeader(visibleTail + decoder.decode(Uint8Array.from(bytes), { stream: true }))
          visible += framed.text
          visibleTail = framed.pending
        },
        getDefaultDownloadDir: () => useSettingsStore().settings.terminal.zmodemDownloadDir,
        onRegister: abort => store.registerAbort(id, abort),
        onUnregister: () => store.unregisterAbort(id),
        onTerminalRestoreState: value => { restoring = value; if (!value) releaseOutput() },
        onComplete: (files, hint) => {
          if (hint) msg.warning(hint)
          else if (files.length) msg.success(`ZMODEM: ${files.length} file(s) transferred`)
        },
        onWarning: warning => msg.warning(warning),
        onError: error => msg.error(`ZMODEM: ${error}`),
        onSettled: () => {
          const finished = service
          service = undefined
          textStarted = false
          visibleTail = ''
          releaseOutput()
          store.clearTransfers(id)
          void finished?.dispose().catch(error => msg.error(String(error)))
          if (!disposed) {
            void finishTerminalFileTransfer(id).catch(error => msg.error(String(error)))
            createService()
          }
        },
      })
    }
    createService()
    return {
      async consume(data, binary) {
        // Binary events, including late trailing output, must continue through
        // the sentry while normal post-EndZmodem text waits for its handoff.
        if (!binary && restoring) {
          await new Promise<void>(resolve => deferred.push(resolve))
          return data
        }
        if (!binary && !getManagedTerminal(id)?.fileTransferActive) return data
        visible = ''
        if (binary) service?.consumeBinary(data)
        else if (!textStarted) { textStarted = true; service?.start(data) }
        else service?.consume(data)
        return visible
      },
      dispose() {
        disposed = true
        releaseOutput()
        void service?.dispose().catch(error => console.error('[ZMODEM dispose]', error))
        service = undefined
        store.clearTransfers(id)
      },
    }
  })
}
