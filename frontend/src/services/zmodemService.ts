import Zmodem from 'zmodem.js/src/zmodem_browser'
import {
  SessionEndZmodem,
  SessionWriteBinary,
  SessionWrite,
  AppendFileBase64,
  ReadFileBase64,
  OpenDirectoryDialog,
  OpenMultipleFilesDialog,
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime'
import { useZmodemStore } from '../stores/zmodemStore'

// Global dialog lock: prevents multiple BaseTerminal instances (e.g. from
// KeepAlive-cached hidden tabs) from opening overlapping native dialogs for
// the same session, which triggers a WebView2 crash on Windows.
const dialogLocks = new Set<string>()

export interface ZmodemServiceOptions {
  sessionId: string
  direction?: 'upload' | 'download'
  onComplete?: (files: string[], hint?: string) => void
  onError?: (err: string) => void
  onRegister?: (abort: () => void) => void
}

export function startZmodemService(options: ZmodemServiceOptions) {
  let binaryUnsub: (() => void) | null = null
  let currentZsession: import('zmodem.js/src/zmodem_browser').Session | null = null
  let aborted = false
  const isAborted = () => aborted
  const { sessionId, onComplete, onError } = options
  const abortCtl = { reject: null as (() => void) | null, done: null as (() => void) | null }

  // Per-instance write tracking. Each chunk's SessionWriteBinary
  // promise is pushed here. Between chunks we drain all pending
  // writes so the Wails bridge doesn't overflow.
  const pendingWrites: Promise<void>[] = []

  async function drainWrites() {
    const batch = pendingWrites.splice(0)
    await Promise.all(batch)
  }

  function sender(octets: number[]) {
    const base64 = arrayBufferToBase64(new Uint8Array(octets))
    const p = SessionWriteBinary(sessionId, base64).catch(() => {
      // noop
    })
    pendingWrites.push(p)
  }

  const sentry = new Zmodem.Sentry({
    to_terminal: (_octets: number[]) => {},
    sender,
    on_detect: (detection: import('zmodem.js/src/zmodem_browser').Detection) => {
      // Global lock: another instance already opened a dialog for this session.
      if (dialogLocks.has(sessionId)) {
        return
      }
      const zsession = detection.confirm()
      currentZsession = zsession
      // zmodem session detected

      if (zsession.type === 'send') {
        // ── Upload (rz) ──────────────────────────────────────────
        dialogLocks.add(sessionId)

        // Check for drag-and-drop pending files before opening dialog
        const store = useZmodemStore()
        const pendingPaths = store.getPendingUploadFiles(sessionId)
        const uploadPaths = pendingPaths && pendingPaths.length > 0
          ? Promise.resolve(pendingPaths)
          : OpenMultipleFilesDialog()

        uploadPaths.then(async (paths) => {
          if (!paths || paths.length === 0) {
            zsession.abort()
            await SessionEndZmodem(sessionId).catch(() => {})
            onComplete?.([])
            return
          }
          await handleSend(zsession, sessionId, paths, drainWrites, isAborted, abortCtl, onComplete, onError)
        }).catch((err: any) => {
          if (!aborted) onError?.(String(err))
        }).finally(() => {
          dialogLocks.delete(sessionId)
        })
      } else {
        // ── Download (sz) ────────────────────────────────────────
        dialogLocks.add(sessionId)
        OpenDirectoryDialog().then(async (saveDir: string) => {
          if (!saveDir) {
            zsession.abort()
            await SessionEndZmodem(sessionId).catch(() => {})
            onComplete?.([])
            return
          }
          await handleReceive(zsession, sessionId, saveDir, isAborted, abortCtl, onComplete, onError)
        }).catch((err: any) => {
          if (!aborted) onError?.(String(err))
        }).finally(() => {
          dialogLocks.delete(sessionId)
        })
      }
    },
    on_retract: () => {
      currentZsession = null
    },
  })

  binaryUnsub = EventsOn('session:binary', (payload: { id: string; data: string }) => {
    if (payload.id !== sessionId) return
    try { sentry.consume(base64ToUint8Array(payload.data)) } catch (_) {}
  })

  const svc = {
    consume: (data: string) => {
      try { sentry.consume(new TextEncoder().encode(data)) } catch (_) {}
    },
    dispose: () => { binaryUnsub?.(); binaryUnsub = null },
    isAborted,
    abort: async () => {
      if (!currentZsession || aborted) return
      aborted = true

      // 1. 协议层面：让 zmodem.js 发送 ZABORT 帧结束会话
      try { currentZsession.abort() } catch (_) {}

      pendingWrites.length = 0

      // 2. 发送经典 ABORT 序列给远程（兼容老式 sz）
      const ABORT = [0x18, 0x18, 0x18, 0x18, 0x18, 0x08, 0x08, 0x08, 0x08, 0x08]
      await SessionWriteBinary(sessionId, arrayBufferToBase64(new Uint8Array(ABORT)))

      // 3. 发送 Ctrl+C 强制终止远程前台 sz 进程
      await SessionWrite(sessionId, '\x03').catch(() => {})

      // 4. 触发 handleReceive 中的 race / donePromise 强制退出
      // 不立即 SessionEndZmodem，等 handleReceive 自然退出后再清理
      const reject = abortCtl.reject
      if (reject) reject()
      const doneFn = abortCtl.done
      if (doneFn) doneFn()
    },
  }

  // Register the abort function so any BaseTerminal can cancel this transfer
  options.onRegister?.(() => svc.abort())

  return svc
}

// ── Upload (rz) ──────────────────────────────────────────────────────

async function handleSend(
  zsession: import('zmodem.js/src/zmodem_browser').Session,
  sessionId: string,
  paths: string[],
  drainWrites: () => Promise<void>,
  isAborted: () => boolean,
  abortCtl: { reject: (() => void) | null; done: (() => void) | null },
  onComplete?: (files: string[], hint?: string) => void,
  onError?: (err: string) => void,
) {
  const store = useZmodemStore()
  const files: string[] = []

  try {
    for (let i = 0; i < paths.length; i++) {
      const path = paths[i]
      const filename = path.split(/[\\/]/).pop() || 'unknown'
      const transferId = `${sessionId}-up-${i}`
      const fileData = base64ToUint8Array(await ReadFileBase64(path))

      store.addTransfer(sessionId, {
        id: transferId, sessionId, filename,
        size: fileData.length, transferred: 0,
        direction: 'upload', status: 'transferring', speed: 0,
      })

      const xfer: any = await (zsession as any).send_offer({
        name: filename, size: fileData.length,
        mode: 0o644, mtime: new Date(),
        files_remaining: 1, bytes_remaining: fileData.length,
      })
      if (!xfer) {
        store.updateTransfer(sessionId, transferId, {
          status: 'cancelled', error: 'File exists, please delete and retry'
        })
        try { await zsession.close() } catch (_) {}
        onComplete?.(files, `"${filename}" already exists. Please delete it first (rm "${filename}"), then retry.`)
        return
      }

      // Race sendChunks against abort so cancel during rz can interrupt
      let done = false
      const abortPromise = new Promise<Error>((_, reject) => {
        abortCtl.reject = () => { if (!done) reject(new Error('aborted')) }
      })
      try {
        await Promise.race([
          sendChunks(xfer, fileData, CHUNK, sessionId, transferId, drainWrites, isAborted),
          abortPromise,
        ])
      } catch (e: any) {
        if (e?.message !== 'aborted') throw e
      } finally {
        done = true
        abortCtl.reject = null
      }
      if (isAborted()) break

      store.updateTransfer(sessionId, transferId, { status: 'completed', transferred: fileData.length })
      files.push(filename)
    }

    if (!isAborted()) {
      await zsession.close()
    }
    onComplete?.(isAborted() ? [] : files)
  } catch (err: any) {
    if (err?.message !== 'aborted') {
      onError?.(err.message || String(err))
    }
  } finally {
    abortCtl.reject = null
    abortCtl.done = null
    await SessionEndZmodem(sessionId).catch(() => {})
  }
}

const CHUNK = 8192

async function sendChunks(
  xfer: any, data: Uint8Array, chunkSize: number,
  sessionId: string, transferId: string,
  drainWrites: () => Promise<void>,
  isAborted: () => boolean,
) {
  const store = useZmodemStore()
  let offset = 0
  while (offset < data.length) {
    if (isAborted()) throw new Error('aborted')
    const end = Math.min(offset + chunkSize, data.length)
    const chunk = Array.from(data.slice(offset, end)) as number[]
    if (end >= data.length) {
      xfer.send(chunk); await drainWrites()
      await xfer.end([]); await drainWrites()
    } else {
      xfer.send(chunk); await drainWrites()
    }
    offset = end
    store.updateTransfer(sessionId, transferId, { transferred: offset })
  }
}

// ── Download (sz) ────────────────────────────────────────────────────

async function handleReceive(
  zsession: import('zmodem.js/src/zmodem_browser').Session,
  sessionId: string,
  saveDir: string,
  isAborted: () => boolean,
  abortCtl: { reject: (() => void) | null; done: (() => void) | null },
  onComplete?: (files: string[]) => void,
  onError?: (err: string) => void,
) {
  const store = useZmodemStore()
  const files: string[] = []
  let offerCount = 0

  let done!: () => void
  const donePromise = new Promise<void>(r => { done = r; abortCtl.done = r })
  let idleTimer: ReturnType<typeof setTimeout> | null = null
  function resetIdle() { if (idleTimer) clearTimeout(idleTimer); idleTimer = setTimeout(done, 2000) }

  try {
    const sep = saveDir.includes('\\') ? '\\' : '/'

    zsession.on('offer', async (offer: any) => {
      if (isAborted()) {
        try { offer.skip() } catch (_) {}
        return
      }
      resetIdle()
      const details = offer.get_details()
      const filename = details.name
      const size = details.size || 0
      const finalSavePath = `${saveDir}${sep}${filename}`

      const transferId = `${sessionId}-dl-${offerCount++}`
      store.addTransfer(sessionId, {
        id: transferId, sessionId, filename,
        size, transferred: 0,
        direction: 'download', status: 'transferring', speed: 0,
        savePath: finalSavePath,
      })
      let received = 0
      let writeChain = Promise.resolve()
      let writeError: unknown = null
      const onInput = (payload: number[]) => {
        const offset = received
        const chunk = Uint8Array.from(payload)
        received += chunk.length
        resetIdle()
        store.updateTransfer(sessionId, transferId, { transferred: received })
        writeChain = writeChain.then(async () => {
          if (writeError) return
          await AppendFileBase64(finalSavePath, arrayBufferToBase64(chunk), offset)
        }).catch((err) => {
          writeError = err
        })
      }
      try {
        await offer.accept({ on_input: onInput })
        await writeChain
        if (writeError) throw writeError
        store.updateTransfer(sessionId, transferId, {
          status: 'completed', transferred: received,
        })
        files.push(finalSavePath)
      } catch (e: any) {
        store.updateTransfer(sessionId, transferId, {
          status: 'error', error: e?.message || String(e),
        })
      }
    })

    resetIdle()  // start initial timeout

    // race: zsession.start() vs 用户取消
    const abortPromise = new Promise<void>((_, reject) => {
      abortCtl.reject = () => reject(new Error('aborted'))
    })

    await Promise.race([zsession.start(), abortPromise])
    abortCtl.reject = null

    await donePromise
    if (idleTimer) clearTimeout(idleTimer)
    onComplete?.(files)
  } catch (err: any) {
    if (err?.message !== 'aborted') {
      onError?.(err.message || String(err))
    } else {
      // 用户取消：触发 onComplete 进行清理（files 为空，不会显示成功消息）
      onComplete?.(files)
    }
  } finally {
    abortCtl.reject = null
    abortCtl.done = null
    if (idleTimer) clearTimeout(idleTimer)
    await SessionEndZmodem(sessionId).catch(() => {})
  }
}

// ── Helpers ───────────────────────────────────────────────────────────

function base64ToUint8Array(base64: string): Uint8Array {
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}

function arrayBufferToBase64(buffer: Uint8Array): string {
  let binary = ''
  for (let i = 0; i < buffer.length; i++) binary += String.fromCharCode(buffer[i])
  return btoa(binary)
}
