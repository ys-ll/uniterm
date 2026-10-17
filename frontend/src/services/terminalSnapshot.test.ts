import { describe, expect, it } from 'vitest'
import { Terminal, type IBuffer } from '@xterm/xterm'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { ProgressAddon } from '@xterm/addon-progress'
import { TerminalStream, installTerminalInputRouter } from './terminalStream'
import { captureTerminalSnapshot, restoreTerminalSnapshot } from './terminalSnapshot'

function create(options: { cols?: number; rows?: number; scrollback?: number } = {}) {
  const terminal = new Terminal({ cols: 18, rows: 5, scrollback: 12, allowProposedApi: true, ...options })
  terminal.loadAddon(new Unicode11Addon())
  terminal.unicode.activeVersion = '11'
  const progress = new ProgressAddon()
  terminal.loadAddon(progress)
  const stream = new TerminalStream(terminal, { cursorBlink: true, sftp: false })
  return { terminal, stream, progress }
}

function state(t: ReturnType<typeof create>) {
  return JSON.parse(JSON.stringify(captureTerminalSnapshot('s', t.stream, { progress: t.progress }).state))
}

// Compare restored xterm cells directly with the running authority, without
// making another snapshot the oracle for the snapshot under test.
function rows(buffer: IBuffer, start = 0) {
  return Array.from({ length: buffer.length - start }, (_, i) => {
    const line = buffer.getLine(start + i)!
    return { wrapped: line.isWrapped, cells: Array.from({ length: line.length }, (_, x) => {
      const cell = line.getCell(x)!
      return [cell.getChars(), cell.getWidth(), cell.getFgColorMode(), cell.getFgColor(), cell.getBgColorMode(), cell.getBgColor(), cell.isBold(), cell.isItalic(), cell.isUnderline(), cell.isInverse()]
    }) }
  })
}
function expectCurrent(a: Terminal, b: Terminal) {
  for (const kind of ['normal', 'alternate'] as const) {
    const source = a.buffer[kind], restored = b.buffer[kind]
    expect(rows(restored, restored.baseY)).toEqual(rows(source, source.baseY))
    expect([restored.cursorX, restored.cursorY]).toEqual([source.cursorX, source.cursorY])
  }
}

describe('pinned xterm exact snapshots', () => {
  it.each([[250, 5000, 1000], [250, 50000, 1000], [1024, 5000, 256]])('fits a filled %i-column, %i-line desktop history into the snapshot budget (%i retained)', async (cols, history, retained) => {
    const a = create({ cols, rows: 24, scrollback: history }), b = create()
    try {
      await a.stream.output(('x'.repeat(cols) + '\r\n').repeat(history + 24), 1)
      expect(a.terminal.buffer.normal.baseY).toBe(history)
      const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream))
      expect(new TextEncoder().encode(JSON.stringify(snap)).length).toBeLessThanOrEqual(16 * 1024 * 1024)
      expect(snap.state.normal.lines).toHaveLength(retained + 24)
      expect(snap.state.normal.ybase).toBe(retained)
      expect(a.terminal.options.scrollback).toBe(history)
      expect(a.terminal.buffer.normal.length).toBe(history + 24)
      await b.stream.run(() => restoreTerminalSnapshot(b.stream, snap))
      expectCurrent(a.terminal, b.terminal)
      expect(b.terminal.buffer.normal.length).toBe(retained + 24)
      expect(b.terminal.buffer.normal.getLine(0)?.translateToString()).toBe('x'.repeat(cols))
    } finally { a.terminal.dispose(); b.terminal.dispose() }
  }, 15000)

  it('includes link payloads and extended attributes in the final JSON byte budget', async () => {
    const a = create({ cols: 250, rows: 8, scrollback: 5000 }), b = create()
    try {
      const link = '\x1b]8;;https://example.test/' + 'u'.repeat(5000) + '\x07' + 'x'.repeat(250) + '\x1b]8;;\x07\r\n'
      await a.stream.output(link.repeat(1100), 1)
      const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream))
      expect(snap.state.normal.ybase).toBeLessThan(1000)
      expect(snap.state.normal.ybase).toBeGreaterThan(0)
      expect(new TextEncoder().encode(JSON.stringify(snap)).length).toBeLessThanOrEqual(16 * 1024 * 1024)
      await b.stream.run(() => restoreTerminalSnapshot(b.stream, snap))
      expectCurrent(a.terminal, b.terminal)
    } finally { a.terminal.dispose(); b.terminal.dispose() }
  })

  it('rebases wide normal history, saved cursor, OSC8 and timestamps while preserving alternate state and continuation', async () => {
    const a = create({ cols: 250, rows: 24, scrollback: 5000 }), b = create()
    try {
      const history = Array.from({ length: 5024 }, (_, i) => {
        const text = `${i.toString().padStart(5, '0')} ${'x'.repeat(244)}`
        return (i === 10 || i === 4500 ? `\x1b]8;id=${i};https://example.test/${i}\x07${text}\x1b]8;;\x07` : text) + '\r\n'
      }).join('')
      await a.stream.output(history, 1)
      await a.stream.output('\x1b[7;248H\x1b[38;2;12;24;36m界x\x1b7\x1b[?1049h\x1b[2J\x1b[2;20r\x1b[3;248H界e\u0301WRAP\x1b]8;id=alt;https://example.test/alt\x07link\x1b]8;;\x07\x1b7\x1b[?25l\x1b]9;4;1;43\x07', 2)
      const extra = { lineOffset: 73, nextNumber: 9024, entries: Array.from({ length: 5024 }, (_, i) => [73 + i, { number: 4000 + i, ts: 1700000000000 + i }]), activeClient: 'desktop' }
      const before = JSON.stringify(extra)
      const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream, a, extra))
      expect(snap.state.extra).toEqual({ ...extra, lineOffset: 4073, entries: extra.entries.slice(4000) })
      expect(JSON.stringify(extra)).toBe(before)
      expect(snap.state.normal.savedY).toBe(1006)
      expect(snap.state.links.map((link: any) => link.data.id)).toEqual(['4500', 'alt'])
      expect(snap.state.links[0].lines[0]).toEqual({ buffer: 'normal', line: 499 })
      expect(a.terminal.buffer.normal.baseY).toBe(5000)
      expect(a.terminal.options.scrollback).toBe(5000)
      await b.stream.run(() => restoreTerminalSnapshot(b.stream, JSON.parse(JSON.stringify(snap)), b))
      expect(rows(b.terminal.buffer.normal)).toEqual(rows(a.terminal.buffer.normal, 4000))
      expectCurrent(a.terminal, b.terminal)
      expect(b.progress.progress).toEqual(a.progress.progress)
      expect(b.terminal.modes).toEqual(a.terminal.modes)
      for (const [i, output] of [
        '\x1b8\tZ',
        '\x1b[?1049l\x1b8NEXT',
        '\x1b[24;1H' + 'scroll\r\n'.repeat(6),
        '\x1b8SAVED\x1b]8;id=new;https://example.test/new\x07new link\x1b]8;;\x07',
      ].entries()) {
        await Promise.all([a.stream.output(output, i + 3), b.stream.output(output, i + 3)])
        expectCurrent(a.terminal, b.terminal)
      }
      expect(rows(b.terminal.buffer.normal)).toEqual(rows(a.terminal.buffer.normal, 4000))
      expect((b.terminal as any)._core._oscLinkService._nextId).toBe((a.terminal as any)._core._oscLinkService._nextId)
    } finally { a.terminal.dispose(); b.terminal.dispose() }
  })

  it('retains wrapped-line metadata at the cut and clamps a saved cursor in omitted history', async () => {
    const a = create({ cols: 20, rows: 5, scrollback: 5000 }), b = create()
    try {
      await a.stream.output('\x1b7' + ('w'.repeat(60) + '\r\n').repeat(1000), 1)
      const extra = { lineOffset: 50, nextNumber: 1001, entries: Array.from({ length: 1000 }, (_, i) => [50 + i * 3, { number: i + 1, ts: 1000 + i }]) }
      a.terminal.scrollToTop()
      const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream, {}, extra))
      expect(snap.state.normal.lines[0].wrapped).toBe(true)
      expect(snap.state.normal.savedY).toBe(0)
      expect(snap.state.normal.ydisp).toBe(0)
      expect(snap.state.extra.lineOffset).toBe(2046)
      expect(snap.state.extra.entries[0]).toEqual([2046, { number: 666, ts: 1665 }])
      // Both buffers still have exactly 2004 rows of unused history capacity.
      expect(snap.state.scrollback - snap.state.normal.ybase).toBe(5000 - a.terminal.buffer.normal.baseY)
      await b.stream.run(() => restoreTerminalSnapshot(b.stream, snap))
      expect(rows(b.terminal.buffer.normal)).toEqual(rows(a.terminal.buffer.normal, 1996))
      await Promise.all([a.stream.output('\x1b8restored', 2), b.stream.output('\x1b8restored', 2)])
      expectCurrent(a.terminal, b.terminal)
    } finally { a.terminal.dispose(); b.terminal.dispose() }
  })

  it('continues an open OSC8 whose old markers were cropped without reusing discarded link IDs', async () => {
    const a = create({ scrollback: 5000 }), b = create()
    try {
      const open = '\x1b]8;id=open;https://example.test/open\x07'
      await a.stream.output(open + '\x1b]8;id=discarded;https://example.test/old\x07' + open + '\r\n'.repeat(2500), 1)
      const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream))
      expect(snap.state.links).toEqual([{ id: 1, data: { id: 'open', uri: 'https://example.test/open' }, lines: [] }])
      await b.stream.run(() => restoreTerminalSnapshot(b.stream, snap))
      const output = 'continued\x1b]8;;\x07\x1b]8;id=new;https://example.test/new\x07new'
      await Promise.all([a.stream.output(output, 2), b.stream.output(output, 2)])
      expectCurrent(a.terminal, b.terminal)
      const source = (a.terminal as any)._core, restored = (b.terminal as any)._core
      expect(restored._inputHandler._curAttrData.extended.urlId).toBe(3)
      for (const id of [1, 3]) expect(restored._oscLinkService.getLinkData(id)).toEqual(source._oscLinkService.getLinkData(id))
    } finally { a.terminal.dispose(); b.terminal.dispose() }
  })

  it('uses the desktop clear-screen transform without erasing retained history, including inactive parsing', async () => {
    const a = create()
    try {
      await a.stream.output('old line\r\nprompt>', 1)
      await a.stream.output('\x1b[H\x1b[2Jfresh', 2)
      const buffer = a.terminal.buffer.normal
      const text = Array.from({ length: buffer.length }, (_, i) => buffer.getLine(i)?.translateToString(true)).join('\n')
      expect(text).toContain('old line')
      expect(text).toContain('fresh')
      expect(buffer.cursorY).toBe(0)
      expect(buffer.cursorX).toBe(5)
    } finally { a.terminal.dispose() }
  })

  it('restores bounded history, alternate screen, Unicode/cells, OSC8, progress and live continuation', async () => {
    const a = create(), b = create()
    try {
      await a.stream.output(Array.from({ length: 30 }, (_, i) => `${i} 界e\u0301\r\n`).join(''), 1)
      await a.stream.output('\x1b]8;id=docs;https://example.test\x07link\x1b]8;;\x07\x1b[38;2;12;24;36mcolor\x1b7', 2)
      await a.stream.output('\x1b[?1049h\x1b[2J\x1b[2;4r\x1b[?25l\x1b[?1003h\x1b[?1006h\x1b[?2004h\x1b[3;2H界e\u0301\x1b]9;4;1;43\x07', 3)
      const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream, { progress: a.progress }))
      await b.stream.run(() => restoreTerminalSnapshot(b.stream, JSON.parse(JSON.stringify(snap)), { progress: b.progress }))
      expect(state(b)).toEqual(state(a))
      for (const output of ['\r\nmore\r\nscroll\r\n', '\x1b[?1049l\x1b8\tZ', '\x1b[?1049h\x1b[2;1Hnext']) {
        await a.stream.output(output, ++snap.sequence)
        await b.stream.output(output, snap.sequence)
        expect(state(b)).toEqual(state(a))
      }
      expect(a.terminal.buffer.normal.length).toBeLessThanOrEqual(17)
    } finally { a.terminal.dispose(); b.terminal.dispose() }
  })

  it.each(['\x1b[31;', '\x1b]8;;https://example.', '\x1bP$q', '\ud83d'])('retains an unfinished sequence %j across snapshot and next output', async prefix => {
    const a = create(), b = create()
    const suffix = prefix.startsWith('\x1b[') ? '1mRED' : prefix.startsWith('\x1b]') ? 'test\x1b\\link' : prefix.startsWith('\x1bP') ? 'm\x1b\\' : '\ude00'
    try {
      await a.stream.output('prompt>' + prefix, 7)
      const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream))
      expect(snap.state.pending).toBe(prefix)
      await b.stream.run(() => restoreTerminalSnapshot(b.stream, snap))
      await Promise.all([a.stream.output(suffix, 8), b.stream.output(suffix, 8)])
      expect(state(b)).toEqual(state(a))
    } finally { a.terminal.dispose(); b.terminal.dispose() }
  })

  it('serializes snapshot/write/resize barriers and ignores replayed sequence numbers', async () => {
    const a = create(), b = create()
    try {
      const first = a.stream.output('hello\x1b[?1049hTUI', 10)
      const snapshot = a.stream.run(() => captureTerminalSnapshot('s', a.stream))
      const next = a.stream.output('\r\nnext', 11)
      const resize = a.stream.resize(13, 6, 12)
      await first
      const snap = await snapshot
      expect(snap.sequence).toBe(10)
      expect(snap.cols).toBe(18)
      await b.stream.run(() => restoreTerminalSnapshot(b.stream, snap))
      await b.stream.output('hello\x1b[?1049hTUI', 10)
      await b.stream.output('\r\nnext', 11)
      await b.stream.resize(13, 6, 12)
      await Promise.all([next, resize])
      expect(state(b)).toEqual(state(a))
    } finally { a.terminal.dispose(); b.terminal.dispose() }
  })

  it('continues saved charsets/attributes, custom tab stops, origin/insert modes and wrap-pending cursors', async () => {
    const a = create(), b = create()
    try {
      await a.stream.output('\x1b[3g\x1b[6G\x1bH\x1b[2;5r\x1b[?6h\x1b[4h\x1b(0\x1b[38;2;1;2;3m\x1b7\x1b[1;18HX', 1)
      const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream))
      await b.stream.run(() => restoreTerminalSnapshot(b.stream, snap))
      for (const [i, text] of ['Y\r\nline', '\x1b8\tq\x1b[3b', '\x1b[?6l\x1b[4l\x1b(B\r\nend'].entries()) {
        await Promise.all([a.stream.output(text, i + 2), b.stream.output(text, i + 2)])
        expect(state(b)).toEqual(state(a))
      }
    } finally { a.terminal.dispose(); b.terminal.dispose() }
  })

  it('routes automatic replies by source, keeping manual CPR-shaped paste and binary mouse input', async () => {
    const a = create(), b = create()
    const replies: string[] = [], manual: string[] = [], browser: string[] = [], binary: string[] = []
    installTerminalInputRouter(a.terminal, data => replies.push(data))
    installTerminalInputRouter(b.terminal, () => {})
    a.terminal.onData(data => manual.push(data))
    b.terminal.onData(data => browser.push(data))
    b.terminal.onBinary(data => binary.push(data))
    try {
      await Promise.all([a.stream.output('\x1b[6n\x1b[c', 1), b.stream.output('\x1b[6n\x1b[c', 1)])
      expect(replies.length).toBe(2)
      expect(manual).toEqual([])
      expect(browser).toEqual([])
      b.terminal.input('\x1b[1;1R', true)
      ;(b.terminal as any)._core.coreService.triggerBinaryEvent('\x1b[M\xff !')
      expect(browser).toEqual(['\x1b[1;1R'])
      expect(binary).toEqual(['\x1b[M\xff !'])
    } finally { a.terminal.dispose(); b.terminal.dispose() }
  })
})
