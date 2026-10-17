import { Call } from '@wailsio/runtime'
import { OpenFileDialogFiltered, ReadFileBase64, SaveFileDialogFiltered, WriteFileBase64, SetBackgroundImage } from '../../bindings/github.com/ys-ll/uniterm/app'
import { isWebMode } from '../utils/webMode'
import { ref } from 'vue'

export interface FileStage { id: string; files: string[]; state: string; sessionId?: string; taskId?: string; error?: string }
export interface DeviceEntry { path: string; file?: File }
type FileMethod = 'WebFilesPut' | 'WebFilesGet' | 'WebFilesPath' | 'WebFilesPersist' | 'WebFilesCopyToHost' | 'WebFilesHostDownload' | 'WebFilesExportConnections'
  | 'WebZmodemTarget' | 'WebZmodemPick' | 'WebZmodemAnswer' | 'WebZmodemFinish' | 'WebZmodemCancel' | 'WebZmodemProgress'

// Wails beta.27 resolves fully-qualified Go method names (main.App), not the
// generated bindings' import directory. Keep new RPCs here until regeneration.
export function fileRPC<T>(method: FileMethod, ...args: unknown[]): Promise<T> {
  return Call.ByName(`main.App.${method}`, ...args) as Promise<T>
}

export function validateDevicePath(path: string): string {
  if (!path || path.length > 4096 || /[\\:\x00-\x1f\x7f<>"|?*]/.test(path)
    || path.split('/').some(p => !p || p === '.' || p === '..' || /[. ]$/.test(p)
      || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(\.|$)/i.test(p))) {
    throw new Error(`Invalid relative file path: ${path}`)
  }
  return path
}

export function pickDeviceFiles(options: { multiple?: boolean; directory?: boolean; accept?: string } = {}): Promise<DeviceEntry[]> {
  return new Promise(resolve => {
    const input = document.createElement('input')
    input.type = 'file'
    input.multiple = !!options.multiple || !!options.directory
    input.accept = options.accept || ''
    if (options.directory) input.setAttribute('webkitdirectory', '')
    input.style.display = 'none'
    const finish = (entries: DeviceEntry[]) => { input.remove(); resolve(entries) }
    input.addEventListener('change', () => finish(Array.from(input.files || [], file => ({ path: file.webkitRelativePath || file.name, file }))), { once: true })
    input.addEventListener('cancel', () => finish([]), { once: true })
    document.body.append(input)
    input.click()
  })
}

// Entries API preserves dropped directory layout and empty directories. Capture
// entries before the first await; DataTransfer is only readable during drop.
export async function deviceDropEntries(dt: DataTransfer): Promise<DeviceEntry[]> {
  const roots = Array.from(dt.items || [], item => item.webkitGetAsEntry?.()).filter(Boolean)
  if (!roots.length) return Array.from(dt.files, file => ({ path: file.webkitRelativePath || file.name, file }))
  const result: DeviceEntry[] = []
  async function visit(entry: any, parent = ''): Promise<void> {
    const path = validateDevicePath(parent + entry.name)
    if (entry.isFile) {
      const file = await new Promise<File>((resolve, reject) => entry.file(resolve, reject))
      result.push({ path, file })
    } else {
      result.push({ path })
      const reader = entry.createReader()
      for (;;) {
        const children = await new Promise<any[]>((resolve, reject) => reader.readEntries(resolve, reject))
        if (!children.length) break
        for (const child of children) await visit(child, path + '/')
      }
    }
    if (result.length > 10000) throw new Error('Too many file entries')
  }
  for (const root of roots) await visit(root)
  return result
}

export async function stageDeviceFiles(entries: DeviceEntry[], signal?: AbortSignal): Promise<FileStage> {
  const body = new FormData()
  let size = 0
  if (!entries.length || entries.length > 10000) throw new Error('Select between 1 and 10000 file entries')
  for (const entry of entries) {
    const path = validateDevicePath(entry.path)
    size += entry.file?.size || 0
    if (size >= 2 * 1024 ** 3) throw new Error('Upload exceeds the 2 GiB host staging limit')
    if (entry.file) body.append(path, entry.file, entry.file.name)
    else body.append(path + '/', '')
  }
  const response = await fetch('/web/files/upload', { method: 'POST', body, signal, credentials: 'same-origin' })
  if (!response.ok) throw new Error(await response.text())
  return response.json()
}

export async function dismissFileStage(id: string): Promise<void> {
  const response = await fetch(`/web/files/${encodeURIComponent(id)}`, { method: 'DELETE', credentials: 'same-origin' })
  if (!response.ok && response.status !== 404) throw new Error(await response.text())
}

export async function fileStageStatus(id: string, signal?: AbortSignal): Promise<FileStage> {
  const response = await fetch(`/web/files/${encodeURIComponent(id)}`, { signal, credentials: 'same-origin' })
  if (!response.ok) throw new Error(await response.text())
  return response.json()
}

export async function waitFileStage(id: string, signal?: AbortSignal): Promise<FileStage> {
  for (;;) {
    const stage = await fileStageStatus(id, signal)
    if (stage.state === 'ready') return stage
    if (stage.state === 'error' || stage.state === 'cancelled') throw new Error(stage.error || `Transfer ${stage.state}`)
    await new Promise(resolve => setTimeout(resolve, 300))
    signal?.throwIfAborted()
  }
}

export const fileDownloadURL = (id: string) => `/web/files/${encodeURIComponent(id)}/download`

function clickDownload(url: string, name = '') {
  const link = document.createElement('a')
  link.href = url
  link.download = name
  document.body.append(link)
  link.click()
  link.remove()
}

// The browser download manager streams to disk. Never fetch().blob() a staged
// download, and never report that the visitor has finished saving it.
export function downloadFileStage(stage: FileStage) {
  if (stage.state !== 'ready') throw new Error('Download is not ready')
  clickDownload(fileDownloadURL(stage.id))
}

/** Text already exists in the editor/result. Send ordinary HTTP bytes; even a
 * large SQL export doesn't allocate a browser Blob or a base64/JSON copy. */
export async function saveTextFile(text: string, name: string, title = 'Export', filter = '*.*'): Promise<boolean> {
  if (isWebMode()) {
    validateDevicePath(name)
    const response = await fetch(`/web/files/export?name=${encodeURIComponent(name)}`, { method: 'POST', body: text, credentials: 'same-origin', headers: { 'Content-Type': 'text/plain;charset=utf-8' } })
    if (!response.ok) throw new Error(await response.text())
    downloadFileStage(await response.json())
    return true
  }
  const path = await SaveFileDialogFiltered(title, name, 'File', filter)
  if (!path) return false
  const bytes = new TextEncoder().encode(text)
  let binary = ''
  for (const b of bytes) binary += String.fromCharCode(b)
  await WriteFileBase64(path, btoa(binary))
  return true
}

export async function openTextFile(title: string, filter = '*.*', maxBytes = 16 * 1024 ** 2): Promise<{ name: string; text: string } | null> {
  if (isWebMode()) {
    const [entry] = await pickDeviceFiles({ accept: filter === '*.*' ? '' : filter.replace(/\*/g, '').replace(/;/g, ',') })
    if (!entry?.file) return null
    if (entry.file.size > maxBytes) throw new Error(`Text file exceeds ${maxBytes} bytes`)
    return { name: entry.file.name, text: await entry.file.text() }
  }
  const path = await OpenFileDialogFiltered(title, 'File', filter)
  if (!path) return null
  const bytes = Uint8Array.from(atob(await ReadFileBase64(path)), c => c.charCodeAt(0))
  return { name: path.split(/[\\/]/).pop() || path, text: new TextDecoder().decode(bytes) }
}

export async function withDevicePaths<T>(entries: DeviceEntry[], consume: (stage: FileStage) => Promise<T>): Promise<T> {
  const stage = await stageDeviceFiles(entries)
  try { return await consume(stage) }
  finally { await dismissFileStage(stage.id) }
}

export async function pickPersistentHostCopy(accept = ''): Promise<string> {
  const entries = await pickDeviceFiles({ accept })
  if (!entries.length) return ''
  return withDevicePaths(entries, s => fileRPC<string>('WebFilesPersist', s.id, entries[0].path))
}

// SettingsTab hook: SetBackgroundImage already copies into durable app storage.
export async function pickWebBackgroundImage(): Promise<string> {
  const entries = await pickDeviceFiles({ accept: 'image/*' })
  if (!entries.length) return ''
  return withDevicePaths(entries, async s => SetBackgroundImage(await fileRPC<string>('WebFilesPath', s.id, entries[0].path)))
}

// BaseTerminal local-shell drop hook: returns durable host copies for explicit
// shell quoting by its existing drop handler, never visitor "absolute paths".
export async function copyDeviceFilesForShell(entries: DeviceEntry[]): Promise<string[]> {
  return withDevicePaths(entries, async s => {
    const paths: string[] = []
    for (const root of new Set(entries.map(e => e.path.split('/')[0]))) paths.push(await fileRPC<string>('WebFilesPersist', s.id, root))
    return paths
  })
}

export const webDownloads = ref<Array<{ id: string; name: string; state: string; error?: string; sessionId?: string; taskId?: string }>>([])
const watchingDownloads = new Set<string>()
let downloadsRestored = false
function saveDownloads() {
  sessionStorage.setItem('uniterm:web-downloads', JSON.stringify(webDownloads.value))
}
export function restoreWebDownloads() {
  if (!isWebMode() || downloadsRestored) return
  downloadsRestored = true
  try { webDownloads.value = JSON.parse(sessionStorage.getItem('uniterm:web-downloads') || '[]') } catch { /* empty */ }
  for (const item of webDownloads.value) if (item.state !== 'ready') void watchDownload(item.id)
}
export function trackWebDownload(stage: FileStage, name: string) {
  restoreWebDownloads()
  webDownloads.value.push({ id: stage.id, name, state: stage.state, sessionId: stage.sessionId, taskId: stage.taskId })
  saveDownloads()
  void watchDownload(stage.id)
}
export async function watchDownload(id: string) {
  if (watchingDownloads.has(id)) return
  watchingDownloads.add(id)
  const item = webDownloads.value.find(d => d.id === id)
  try {
    const ready = await waitFileStage(id)
    if (item) { item.state = 'ready'; item.error = undefined }
    downloadFileStage(ready)
  } catch (e) {
    if (item) { item.state = 'error'; item.error = String(e) }
  } finally { watchingDownloads.delete(id); saveDownloads() }
}
export async function removeWebDownload(id: string) {
  await dismissFileStage(id)
  webDownloads.value = webDownloads.value.filter(d => d.id !== id)
  saveDownloads()
}
