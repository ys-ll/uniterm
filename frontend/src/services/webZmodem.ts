import { Events } from '@wailsio/runtime'
import { ElMessageBox } from 'element-plus'
import { isWebMode, webClientId } from '../utils/webMode'
import { fileRPC, stageDeviceFiles, pickDeviceFiles, dismissFileStage, trackWebDownload, restoreWebDownloads, type DeviceEntry } from './webFiles'
import { msg } from './message'
import { useZmodemStore, type TransferInfo } from '../stores/zmodemStore'

export interface WebZmodemChoice {
  requestId: string; clientId: string; sessionId: string; direction: 'upload' | 'download'
  paths?: string[]; directory?: string; stageId?: string
}
let initialized = false

export function initWebZmodem() {
  if (!isWebMode() || initialized) return
  initialized = true
  restoreWebDownloads()
  Events.On('web:zmodem-progress', event => {
    const payload = event.data as { clientId: string; sessionId: string; transfer: TransferInfo }
    if (payload.clientId !== webClientId()) return
    const store = useZmodemStore()
    if (store.getTransfers(payload.sessionId).some(t => t.id === payload.transfer.id)) store.updateTransfer(payload.sessionId, payload.transfer.id, payload.transfer)
    else store.addTransfer(payload.sessionId, payload.transfer)
  })
  Events.On('web:zmodem-pick', async event => {
    const request = event.data as WebZmodemChoice
    if (request.clientId !== webClientId()) return
    let stageId = ''
    try {
      // The explicit confirmation button supplies user activation for a file
      // input opened in response to an asynchronous native-parser request.
      await ElMessageBox.confirm(request.direction === 'upload' ? 'Choose device files for ZMODEM upload?' : 'Receive ZMODEM files as a browser download?', 'ZMODEM')
      if (request.direction === 'upload') {
        const entries = await pickDeviceFiles({ multiple: true })
        if (entries.length) stageId = (await stageDeviceFiles(entries)).id
      } else stageId = 'receive'
      await fileRPC('WebZmodemAnswer', request.requestId, webClientId(), stageId)
    } catch (e) {
      if (stageId && stageId !== 'receive') await dismissFileStage(stageId).catch(() => {})
      await fileRPC('WebZmodemAnswer', request.requestId, webClientId(), '').catch(() => {})
      if (e !== 'cancel' && e !== 'close') msg.error(String(e))
    }
  })
  Events.On('web:zmodem-done', event => {
    const result = event.data as WebZmodemChoice & { error?: string }
    if (result.clientId !== webClientId()) return
    useZmodemStore().clearTransfers(result.sessionId)
    if (result.error) msg.error(result.error)
    else if (result.direction === 'download' && result.stageId) {
      trackWebDownload({ id: result.stageId, sessionId: result.sessionId, files: [], state: 'ready' }, 'ZMODEM', true)
    } else msg.success('ZMODEM upload complete')
  })
}

/** Explicit picker override. Normal manual input selects its target atomically
 * in WebTerminalInput; do not add an extra target RPC for every keystroke. */
export async function targetWebZmodem(sessionId: string) {
  initWebZmodem()
  await fileRPC('WebZmodemTarget', sessionId, isWebMode() ? webClientId() : '', '')
}

/** Browser remote-drop hook, awaited inside writeSessionInput's preparation.
 * The backend input hook claims this stage for the following rz command. */
export async function stageWebZmodemUpload(sessionId: string, entries: DeviceEntry[], signal?: AbortSignal) {
  initWebZmodem()
  if (entries.some(e => !e.file || e.path.includes('/'))) throw new Error('ZMODEM accepts individual files')
  const stage = await stageDeviceFiles(entries, signal)
  try { signal?.throwIfAborted(); await fileRPC('WebZmodemTarget', sessionId, webClientId(), stage.id) }
  catch (e) { await dismissFileStage(stage.id); throw e }
  return stage.id
}

export function cancelWebZmodem(sessionId: string) {
  return fileRPC('WebZmodemCancel', sessionId, webClientId())
}

export async function nativeZmodemChoice(sessionId: string, direction: 'upload' | 'download') {
  if (isWebMode()) throw new Error('ZMODEM parser belongs to the desktop')
  return fileRPC<WebZmodemChoice>('WebZmodemPick', sessionId, direction)
}
export function finishNativeZmodem(requestId: string, error = '') {
  return fileRPC('WebZmodemFinish', requestId, error)
}
