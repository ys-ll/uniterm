import { spawn } from 'node:child_process'
import { mkdtemp, mkdir, readFile, writeFile, rm, open } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, it, vi } from 'vitest'

const state = vi.hoisted(() => {
  vi.stubGlobal('window', {})
  return {
    binary: false, text: '', child: undefined,
    binaryHandler: undefined,
    choice: {},
  }
})
vi.mock('@wailsio/runtime', () => ({ Events: { On: (name, handler) => {
  if (name === 'session:binary') state.binaryHandler = handler
  return () => { if (name === 'session:binary') state.binaryHandler = undefined }
} } }))
vi.mock('../stores/zmodemStore', () => ({ useZmodemStore: () => ({ addTransfer() {}, updateTransfer() {}, getPendingUploadFiles() {} }) }))
vi.mock('./webZmodem', () => ({ nativeZmodemChoice: async () => state.choice, finishNativeZmodem: async () => {} }))
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  SessionStartZmodem: async () => { state.binary = true },
  SessionEndZmodem: async () => { state.binary = false },
  SessionEndZmodemWithTrailing: async (_id, data) => { state.text += Buffer.from(data, 'base64').toString(); state.binary = false },
  SessionWriteBinary: vi.fn(async (_id, data) => {
    await new Promise((resolve, reject) => state.child.stdin.write(Buffer.from(data, 'base64'), error => error ? reject(error) : resolve()))
  }),
  SessionWrite: async (_id, data) => { state.child.stdin.write(data) },
  FileSize: async path => (await readFile(path)).length,
  ReadFileChunkBase64: async (path, offset, length) => (await readFile(path)).subarray(offset, offset + length).toString('base64'),
  AppendFileBase64: async (path, data, offset) => {
    const handle = await open(path, offset === 0 ? 'w' : 'r+')
    try { await handle.write(Buffer.from(data, 'base64'), 0, Buffer.from(data, 'base64').length, offset) }
    finally { await handle.close() }
  },
  OpenDirectoryDialog: async () => { throw new Error('Unexpected native picker') },
  OpenMultipleFilesDialog: async () => { throw new Error('Unexpected native picker') },
}))
import { startZmodemService } from './zmodemService'
import { splitZmodemHeader } from './zmodemHeader'
import { quoteShellPath } from '../utils/shellPath'
import { SessionWriteBinary } from '../../bindings/github.com/ys-ll/uniterm/app'

// Optional real lrzsz peer; no GUI, extra parser, SSH fixture mutation or network
// service required. Set UNITERM_ZMODEM_BIN to the directory containing lrz/lsz.
it.skipIf(!process.env.UNITERM_ZMODEM_BIN)('round-trips real lrzsz bytes through the native service and restores both trailing prompts', async () => {
  const root = await mkdtemp(join(tmpdir(), 'uniterm-zmodem-'))
  try {
    const source = join(root, '访客上传.bin')
    const bytes = Buffer.from(Array.from({ length: 262144 }, (_, i) => (i * 37 + (i >> 8)) & 255))
    await writeFile(source, bytes)
    for (const direction of ['upload', 'download']) {
      state.binary = false
      state.text = ''
      vi.mocked(SessionWriteBinary).mockClear()
      const target = join(root, direction)
      await mkdir(target)
      state.choice = direction === 'upload' ? { requestId: 'web', paths: [source] } : { requestId: 'web', directory: target }
      const errors = []
      let settled
      const done = new Promise(resolve => { settled = resolve })
      const service = startZmodemService({ sessionId: 'protocol', subscribeBinary: false, onError: e => errors.push(e), onWarning: e => errors.push(e), onSettled: settled })
      expect(state.binaryHandler).toBeUndefined()
      const program = join(process.env.UNITERM_ZMODEM_BIN, direction === 'upload' ? 'lrz' : 'lsz')
      const command = `${quoteShellPath(program)} -be ${direction === 'download' ? quoteShellPath(source) : ''}; printf '\\r\\nrestored$ '`
      const peer = spawn('/bin/sh', ['-c', command], { cwd: target, stdio: ['pipe', 'pipe', 'pipe'] })
      state.child = peer
      let pending = '', started = false, stderr = ''
      const decoder = new TextDecoder()
      peer.stderr.on('data', data => { stderr += String(data) })
      peer.stdout.on('data', data => {
        if (state.binary) service.consumeBinary(data.toString('base64'))
        else if (started) state.text += decoder.decode(data, { stream: true })
        else {
          const framed = splitZmodemHeader(pending + data.toString('latin1'))
          pending = framed.pending
          state.text += framed.text
          if (framed.protocol) {
            // SSH/serial detect the initial header and emit raw binary before
            // the frontend asks for binary mode; retain lrzsz's high-bit LF.
            started = true
            state.binary = true
            service.consumeBinary(Buffer.from(framed.protocol, 'latin1').toString('base64'))
          }
        }
      })
      let timeout
      try {
        await Promise.race([new Promise((_, reject) => {
          timeout = setTimeout(() => reject(new Error(`${direction} timed out: ${stderr}; visible=${JSON.stringify(state.text)}; errors=${errors}`)), 8000)
        }), Promise.all([done, new Promise((resolve, reject) => {
          peer.on('error', reject)
          peer.on('exit', code => code === 0 ? resolve() : reject(new Error(`lrzsz ${code}: ${stderr}`)))
        })])])
        expect(errors, stderr).toEqual([])
        expect(await readFile(join(target, '访客上传.bin'))).toEqual(bytes)
        expect(state.text).toContain('restored$ ')
        expect(SessionWriteBinary).toHaveBeenCalled()
      } finally {
        clearTimeout(timeout)
        peer.kill()
        await service.dispose()
      }
    }
  } finally { await rm(root, { recursive: true, force: true }) }
}, 20000)
