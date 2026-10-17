import type { Terminal } from '@xterm/xterm'
import { SerializeAddon } from '@xterm/addon-serialize'
import type { ImageAddon } from '@xterm/addon-image'
import type { ProgressAddon } from '@xterm/addon-progress'
import type { TerminalStream } from './terminalStream'
import type { WebTerminalSnapshot } from './webTerminalBridge'

// Deliberately versioned, private adapters. SerializeAddon omits scroll margins,
// saved attributes, OSC8, image tiles, hidden cursor, tabs and several modes.
// Cell storage is included as a bounded sidecar, not inferred from ANSI text.
const FORMAT = 'xterm-6.0.0-uniterm.1/serialize-0.14.0/image-0.9.0/v1'
// Snapshot history is a suffix of at most 1000 physical rows, with 4 MiB
// reserved for its base64 cells (3 uint32s/cell = 16 encoded bytes). Never
// capture the desktop's whole 5000–50000-row buffer just to check its size.
const MAX_HISTORY_LINES = 1000
const HISTORY_CELL_BYTES = 4 * 1024 * 1024
const MAX_SNAPSHOT_BYTES = 16 * 1024 * 1024
const bufferKeys = ['x', 'y', 'ybase', 'ydisp', 'scrollTop', 'scrollBottom', 'savedX', 'savedY', 'savedCharset', 'tabs']
const inputKeys = ['_windowTitle', '_iconName', '_windowTitleStack', '_iconNameStack']
const clone = <T>(value: T): T => value === undefined ? value : JSON.parse(JSON.stringify(value))
const pick = (value: any, keys: string[]) => Object.fromEntries(keys.map(key => [key, clone(value[key])]))

function coreOf(terminal: Terminal): any {
  const core = (terminal as any)._core
  if (!core?._bufferService?.buffers?.normal?.getBlankLine ||
      !core?._inputHandler?._curAttrData?.extended?.clone ||
      !core?._oscLinkService?._dataByLinkId) {
    throw new Error('Unsupported xterm snapshot internals; expected 6.0.0-uniterm.1')
  }
  return core
}

function encode(data: Uint32Array): string {
  // Explicit LE: sidecar is independent of the host CPU's byte order.
  const bytes = new Uint8Array(data.length * 4)
  const view = new DataView(bytes.buffer)
  data.forEach((n, i) => view.setUint32(i * 4, n, true))
  let text = ''
  for (let i = 0; i < bytes.length; i += 8192) text += String.fromCharCode(...bytes.subarray(i, i + 8192))
  return btoa(text)
}

function decode(text: string): Uint32Array {
  const raw = atob(text)
  if (raw.length % 4) throw new Error('Invalid terminal cell data')
  const view = new DataView(Uint8Array.from(raw, c => c.charCodeAt(0)).buffer)
  return Uint32Array.from({ length: raw.length / 4 }, (_, i) => view.getUint32(i * 4, true))
}

function restoreExtended(core: any, data: any): any {
  const attr = core._inputHandler._curAttrData.extended.clone()
  Object.assign(attr, data)
  if (typeof data.imageId === 'number') {
    // ImageStorage's ExtendedAttrsImage is private. Retain base getters and
    // image-aware cloning/isEmpty, the two virtual methods used by BufferLine.
    attr.clone = function () { return restoreExtended(core, clone(this)) }
    attr.isEmpty = function () { return this.underlineStyle === 0 && this._urlId === 0 && this.imageId === -1 }
  }
  return attr
}

function restoreAttr(core: any, target: any, data: any): void {
  target.fg = data.fg
  target.bg = data.bg
  target.extended = restoreExtended(core, data.extended)
}

function captureBuffer(buffer: any, start = 0): any {
  return {
    ...pick(buffer, bufferKeys),
    ybase: buffer.ybase - start,
    ydisp: Math.max(0, buffer.ydisp - start),
    // InputHandler.saveCursor stores ybase+y, not a screen-relative row.
    savedY: Math.max(0, buffer.savedY - start),
    savedCurAttrData: clone(buffer.savedCurAttrData),
    lines: Array.from({ length: buffer.lines.length - start }, (_, i) => {
      const line = buffer.lines.get(start + i)
      return { data: encode(line._data.subarray(0, line.length * 3)), combined: clone(line._combined), extended: clone(line._extendedAttrs), wrapped: line.isWrapped }
    }),
  }
}

function restoreBuffer(core: any, buffer: any, data: any): void {
  buffer.clearAllMarkers()
  buffer.lines.length = 0
  Object.assign(buffer, pick(data, bufferKeys))
  restoreAttr(core, buffer.savedCurAttrData, data.savedCurAttrData)
  for (const saved of data.lines) {
    const line = buffer.getBlankLine(core._inputHandler._curAttrData, saved.wrapped)
    line._data = decode(saved.data)
    line.length = line._data.length / 3
    line._combined = clone(saved.combined)
    line._extendedAttrs = Object.fromEntries(Object.entries(saved.extended).map(([col, attr]) => [col, restoreExtended(core, attr)]))
    buffer.lines.push(line)
  }
}

function captureLinks(core: any, normalStart: number): any[] {
  const { normal, alt } = core._bufferService.buffers
  const normalMarkers = new Set(normal.markers)
  const activeIds = new Set([core._inputHandler._curAttrData.extended.urlId, normal.savedCurAttrData.extended.urlId, alt.savedCurAttrData.extended.urlId])
  const links = []
  for (const [id, entry] of core._oscLinkService._dataByLinkId) {
    const lines = []
    for (const marker of entry.lines) {
      const buffer = normalMarkers.has(marker) ? 'normal' : 'alt'
      const line = marker.line - (buffer === 'normal' ? normalStart : 0)
      if (line >= 0) lines.push({ line, buffer })
    }
    // An open OSC8 may continue after all its existing cells were cropped.
    if (lines.length || activeIds.has(id)) links.push({ id, data: clone(entry.data), lines })
  }
  return links
}

function restoreLinks(core: any, links: any[]): void {
  const service = core._oscLinkService
  service._entriesWithId.clear()
  service._dataByLinkId.clear()
  service._nextId = 1
  for (const saved of links) {
    const key = saved.data.id === undefined ? undefined : service._getEntryIdKey(saved.data)
    const entry: any = { id: saved.id, data: clone(saved.data), lines: [], ...(key === undefined ? {} : { key }) }
    service._dataByLinkId.set(entry.id, entry)
    if (key !== undefined) service._entriesWithId.set(key, entry)
    service._nextId = Math.max(service._nextId, entry.id + 1)
    for (const { line, buffer } of saved.lines) {
      const marker = core._bufferService.buffers[buffer].addMarker(line)
      entry.lines.push(marker)
      marker.onDispose(() => service._removeMarkerFromLink(entry, marker))
    }
  }
}

function captureImages(addon: ImageAddon | undefined, core: any, normalStart: number): any {
  if (!addon) return undefined
  const a = addon as any
  const storage = a._storage
  if (!storage?._images) throw new Error('Unsupported ImageAddon snapshot internals')
  const tiles = new Map<number, number>()
  for (const [buffer, start] of [[core._bufferService.buffers.normal, normalStart], [core._bufferService.buffers.alt, 0]]) {
    for (let y = start; y < buffer.lines.length; y++) {
      const line = buffer.lines.get(y)
      for (const [col, attr] of Object.entries<any>(line._extendedAttrs)) {
        // Extended-attribute objects can outlive erased cells in this fork.
        if ((line.getBg(Number(col)) & 0x10000000) && attr.imageId >= 0) tiles.set(attr.imageId, (tiles.get(attr.imageId) ?? 0) + 1)
      }
    }
  }
  const images = []
  for (const [id, image] of storage._images) {
    const tileCount = tiles.get(id)
    if (!tileCount) continue
    if (!image.orig) throw new Error('Image data was evicted before snapshot')
    const canvas = document.createElement('canvas')
    canvas.width = image.orig.width
    canvas.height = image.orig.height
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('Cannot capture terminal image canvas')
    ctx.drawImage(image.orig, 0, 0)
    const marker = image.marker ? Math.max(0, image.marker.line - (image.bufferType === 'normal' ? normalStart : 0)) : undefined
    images.push({ id, png: canvas.toDataURL('image/png'), origCellSize: image.origCellSize, tileCount, bufferType: image.bufferType, marker })
  }
  // Sixel palette persists between images, even when their cells were erased.
  const sixel = [...a._handlers.values()].find((h: any) => '_dec' in h) as any
  const decoder = sixel?._dec
  return { images, lastId: storage._lastId, opts: clone(a._opts), palette: decoder?.palette ? Array.from(decoder.palette) : undefined }
}

async function restoreImages(terminal: Terminal, addon: ImageAddon | undefined, saved: any): Promise<void> {
  if (!saved) return
  if (!addon) throw new Error('Snapshot requires ImageAddon')
  const a = addon as any
  const storage = a._storage
  storage.reset()
  Object.assign(a._opts, saved.opts)
  // Initialize the asynchronous sixel decoder through its own public parser.
  // Buffer cells are restored AFTER this harmless empty image below.
  if (saved.palette) {
    await new Promise<void>(resolve => terminal.write('\x1bPq\x1b\\', resolve))
    const sixel = [...a._handlers.values()].find((h: any) => '_dec' in h) as any
    if (!sixel?._dec?.palette) throw new Error('Unsupported sixel palette adapter')
    sixel._dec.palette.set(saved.palette)
  }
  const core = coreOf(terminal)
  for (const image of saved.images) {
    const img = new Image()
    img.src = image.png
    await img.decode()
    const canvas = document.createElement('canvas')
    canvas.width = img.width
    canvas.height = img.height
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('Cannot restore terminal image canvas')
    ctx.drawImage(img, 0, 0)
    const buffer = image.bufferType === 'normal' ? core._bufferService.buffers.normal : core._bufferService.buffers.alt
    const marker = image.marker === undefined ? undefined : buffer.addMarker(image.marker)
    marker?.onDispose(() => storage._delImg(image.id))
    storage._images.set(image.id, {
      orig: canvas, actual: canvas, origCellSize: clone(image.origCellSize), actualCellSize: clone(image.origCellSize),
      tileCount: image.tileCount, bufferType: image.bufferType, marker,
    })
  }
  storage._lastId = saved.lastId
  storage._needsFullClear = true
}

export interface SnapshotAddons { image?: ImageAddon; progress?: ProgressAddon }

function captureExtra(extra: Record<string, any>, normal: any, start: number): Record<string, any> {
  if (!start) return clone(extra)
  const lineOffset = (extra.lineOffset ?? 0) + start
  const entries = extra.entries?.filter(([row]: [number, unknown]) => row >= lineOffset && row < (extra.lineOffset ?? 0) + normal.lines.length)
  // A cut can bisect a wrapped logical line. Anchor its original number/time
  // at the first retained row; all other registry keys stay absolute.
  if (entries && normal.lines.get(start)?.isWrapped) {
    let first = start
    while (first > 0 && normal.lines.get(first).isWrapped) first--
    const meta = extra.entries.find(([row]: [number, unknown]) => row === (extra.lineOffset ?? 0) + first)?.[1]
    if (meta && !entries.some(([row]: [number, unknown]) => row === lineOffset)) entries.unshift([lineOffset, meta])
  }
  return clone({ ...extra, lineOffset, ...(entries ? { entries } : {}) })
}

/** Called inside stream.run: no output or canonical resize can cross this
 * boundary while PNGs/sidecars are captured. The returned sequence is parsed. */
export function captureTerminalSnapshot(id: string, stream: TerminalStream, addons: SnapshotAddons = {}, extra: Record<string, any> = {}): WebTerminalSnapshot {
  const terminal = stream.terminal
  const core = coreOf(terminal)
  if (core._inputHandler._parser.currentState !== 0) throw new Error('Snapshot requires parser Ground')
  const { normal, alt } = core._bufferService.buffers
  let history = Math.min(normal.ybase, MAX_HISTORY_LINES, Math.floor(HISTORY_CELL_BYTES / (terminal.cols * 16)))
  const serializer = new SerializeAddon()
  terminal.loadAddon(serializer)
  try {
    for (;;) {
      const start = normal.ybase - history
      const ansi = serializer.serialize({ scrollback: Math.max(0, normal.lines.length - start - terminal.rows) })
      const state = {
        format: FORMAT, pending: stream.frames.pending, outputOptions: clone(stream.options),
        // Preserve remaining headroom so both emulators begin trimming on the
        // same future scroll. This also preserves the fork's saved-cursor behavior
        // at a full buffer; desktop options and buffers are never modified here.
        scrollback: terminal.options.scrollback! - start,
        normal: captureBuffer(normal, start), alt: captureBuffer(alt),
        active: terminal.buffer.active.type,
        input: pick(core._inputHandler, inputKeys), attr: clone(core._inputHandler._curAttrData),
        precedingJoinState: core._inputHandler._parser.precedingJoinState,
        charset: pick(core._charsetService, ['charset', 'glevel', '_charsets']),
        modes: clone(core.coreService.modes), decModes: clone(core.coreService.decPrivateModes),
        cursorHidden: core.coreService.isCursorHidden,
        mouse: { protocol: core.coreMouseService.activeProtocol, encoding: core.coreMouseService.activeEncoding },
        options: pick(terminal.options, ['cursorStyle', 'cursorBlink', 'applicationCursorKeys', 'convertEol'].filter(k => (terminal.options as any)[k] !== undefined)),
        links: captureLinks(core, start), nextLinkId: core._oscLinkService._nextId,
        images: captureImages(addons.image, core, start),
        progress: addons.progress ? clone(addons.progress.progress) : undefined,
        // OSC palette changes, including restore defaults, are presentation state.
        colors: core._themeService ? pick(core._themeService.colors, ['foreground', 'background', 'cursor', 'ansi']) : undefined,
        restoreColors: core._themeService ? clone(core._themeService._restoreColors) : undefined,
        extra: captureExtra(extra, normal, start),
      }
      const snapshot = { sessionId: id, sequence: stream.sequence, cols: terminal.cols, rows: terminal.rows, ansi, state }
      if (new TextEncoder().encode(JSON.stringify(snapshot)).length <= MAX_SNAPSHOT_BYTES) return snapshot
      // Attrs/links/images count too. Retry only the already-bounded suffix;
      // neither current screen may be cut to make an oversized snapshot fit.
      if (!history) throw new Error('Exact terminal snapshot exceeds 16 MiB')
      history = Math.floor(history / 2)
    }
  } finally { serializer.dispose() }
}

export async function restoreTerminalSnapshot(stream: TerminalStream, snapshot: WebTerminalSnapshot, addons: SnapshotAddons = {}): Promise<void> {
  const terminal = stream.terminal
  const s = snapshot.state
  if (s.error) throw new Error(s.error)
  if (s.format !== FORMAT) throw new Error('Unsupported terminal snapshot version')
  terminal.options.scrollback = s.scrollback
  terminal.reset()
  terminal.resize(snapshot.cols, snapshot.rows)
  // No historical PTY bytes, OSC52, ZMODEM or queries are replayed. Serializer
  // contains only state; raw sidecars correct its known omissions afterwards.
  await stream.writeRaw(snapshot.ansi)
  const core = coreOf(terminal)
  if (s.active === 'alternate') core._bufferService.buffers.activateAltBuffer()
  else core._bufferService.buffers.activateNormalBuffer()
  restoreBuffer(core, core._bufferService.buffers.normal, s.normal)
  restoreBuffer(core, core._bufferService.buffers.alt, s.alt)
  await restoreImages(terminal, addons.image, s.images)
  restoreLinks(core, s.links)
  core._oscLinkService._nextId = s.nextLinkId ?? core._oscLinkService._nextId
  Object.assign(core._inputHandler, s.input)
  restoreAttr(core, core._inputHandler._curAttrData, s.attr)
  Object.assign(core._charsetService, s.charset)
  Object.assign(core.coreService.modes, s.modes)
  Object.assign(core.coreService.decPrivateModes, s.decModes)
  core.coreService.isCursorHidden = s.cursorHidden
  core.coreMouseService.activeProtocol = s.mouse.protocol
  core.coreMouseService.activeEncoding = s.mouse.encoding
  Object.assign(terminal.options, s.options)
  core._inputHandler._parser.precedingJoinState = s.precedingJoinState
  if (s.colors && core._themeService) {
    core._themeService.modifyColors((colors: any) => Object.assign(colors, clone(s.colors)))
    core._themeService._restoreColors = clone(s.restoreColors)
  }
  if (addons.progress && s.progress) addons.progress.progress = s.progress
  stream.frames.pending = s.pending
  stream.options = clone(s.outputOptions)
  stream.sequence = snapshot.sequence
  core._inputHandler._dirtyRowTracker.markAllDirty()
  if (terminal.element) terminal.refresh(0, terminal.rows - 1)
}
