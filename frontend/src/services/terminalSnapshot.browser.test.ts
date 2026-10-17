import { afterEach, describe, expect, it } from 'vitest'
import { Terminal } from '@xterm/xterm'
import { ImageAddon } from '@xterm/addon-image'
import { ProgressAddon } from '@xterm/addon-progress'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { ClipboardAddon, Base64 } from '@xterm/addon-clipboard'
import '@xterm/xterm/css/xterm.css'
import { TerminalStream, installTerminalInputRouter } from './terminalStream'
import { captureTerminalSnapshot, restoreTerminalSnapshot } from './terminalSnapshot'

const cleanup: (() => void)[] = []
afterEach(() => cleanup.splice(0).forEach(fn => fn()))
function create(browser = false) {
  const host = document.createElement('div')
  host.style.cssText = 'width:400px;height:160px'
  document.body.appendChild(host)
  const terminal = new Terminal({ cols: 30, rows: 8, scrollback: 25, allowProposedApi: true })
  const image = new ImageAddon(), progress = new ProgressAddon()
  terminal.loadAddon(image)
  terminal.loadAddon(progress)
  terminal.loadAddon(new Unicode11Addon())
  terminal.unicode.activeVersion = '11'
  terminal.open(host)
  const clipboard: string[] = [], replies: string[] = [], input: string[] = []
  terminal.loadAddon(new ClipboardAddon(new Base64(), { readText: () => '', writeText: (_, value) => { if (!browser) clipboard.push(value) } }))
  installTerminalInputRouter(terminal, data => { if (!browser) replies.push(data) })
  terminal.onData(data => input.push(data))
  const stream = new TerminalStream(terminal, { cursorBlink: true, sftp: false })
  cleanup.push(() => { terminal.dispose(); host.remove() })
  return { terminal, stream, image, progress, clipboard, replies, input, host }
}
const state = (t: ReturnType<typeof create>) => JSON.parse(JSON.stringify(captureTerminalSnapshot('s', t.stream, t).state))

describe('actual browser xterm image and DOM fidelity', () => {
  it('restores sixel pixels/tiles/palette, OSC8 and progress without replaying OSC52 or terminal replies', async () => {
    const a = create(), b = create(true)
    await a.stream.output('hello\r\n\x1b]52;c;aGVsbG8=\x07\x1b[6n', 1)
    // A red/green sixel image, then an OSC8 link on another line.
    await a.stream.output('\x1bPq"1;1;12;12#1;2;100;0;0#1!12~-#2;2;0;100;0#2!12~\x1b\\\r\n\x1b]8;id=x;https://example.test\x07linked\x1b]8;;\x07\x1b]9;4;1;61\x07', 2)
    const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream, a))
    expect(snap.state.images.images.length).toBe(1)
    await b.stream.run(() => restoreTerminalSnapshot(b.stream, JSON.parse(JSON.stringify(snap)), b))
    expect(b.clipboard).toEqual([])
    expect(b.replies).toEqual([])
    expect(a.clipboard).toEqual(['hello'])
    expect(state(b)).toEqual(state(a))
    await expect.poll(() => b.host.querySelector('canvas.xterm-image-layer')).not.toBeNull()
    const pixels = (host: HTMLElement) => (host.querySelector('canvas.xterm-image-layer') as HTMLCanvasElement)?.toDataURL()
    await expect.poll(() => pixels(b.host)).toBe(pixels(a.host))
    // Continued image reuses palette entry #1 from before the snapshot.
    await Promise.all([a.stream.output('\r\n\x1bPq#1!6~\x1b\\', 3), b.stream.output('\r\n\x1bPq#1!6~\x1b\\', 3)])
    expect(state(b)).toEqual(state(a))
    b.terminal.paste('\x1b[1;1R')
    expect(b.input).toEqual(['\x1b[1;1R'])
    expect(b.host.querySelector('.xterm')).not.toBeNull()
  })

  it('keeps alternate images and modes across resize and a split iTerm2 PNG sequence', async () => {
    const a = create(), b = create(true)
    const canvas = document.createElement('canvas')
    canvas.width = canvas.height = 12
    const ctx = canvas.getContext('2d')!
    ctx.fillStyle = '#1455ee'
    ctx.fillRect(0, 0, 12, 12)
    const png = canvas.toDataURL().split(',')[1]
    await a.stream.output('\x1b[?1049h\x1b[2J\x1b[?25l\x1b[?1006h\x1b[2;7r\x1b]1337;File=inline=1;size=' + atob(png).length + ';width=12px;height=12px:' + png.slice(0, 20), 1)
    const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream, a))
    await b.stream.run(() => restoreTerminalSnapshot(b.stream, snap, b))
    await Promise.all([a.stream.output(png.slice(20) + '\x07', 2), b.stream.output(png.slice(20) + '\x07', 2)])
    await Promise.all([a.stream.resize(24, 9, 3), b.stream.resize(24, 9, 3)])
    expect(state(b)).toEqual(state(a))
    expect(state(b).images.images.length).toBe(1)
    const second = await a.stream.run(() => captureTerminalSnapshot('s', a.stream, a))
    await b.stream.run(() => restoreTerminalSnapshot(b.stream, second, b))
    expect(state(b)).toEqual(state(a))
  })
})
