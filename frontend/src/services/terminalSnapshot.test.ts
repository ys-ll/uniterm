import { describe, expect, it } from 'vitest'
import { Terminal } from '@xterm/xterm'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { ProgressAddon } from '@xterm/addon-progress'
import { TerminalStream, installTerminalInputRouter } from './terminalStream'
import { captureTerminalSnapshot, restoreTerminalSnapshot } from './terminalSnapshot'

function create() {
  const terminal = new Terminal({ cols: 18, rows: 5, scrollback: 12, allowProposedApi: true })
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

describe('pinned xterm exact snapshots', () => {
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
