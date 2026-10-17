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
function create(browser = false, options: { cols?: number; rows?: number; scrollback?: number } = {}) {
  const host = document.createElement('div')
  host.style.cssText = 'width:400px;height:160px'
  document.body.appendChild(host)
  const terminal = new Terminal({ cols: 30, rows: 8, scrollback: 25, allowProposedApi: true, ...options })
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
  it('crops wide history before encoding images and keeps crossing tiles, both screens and palette continuation', async () => {
    const a = create(false, { cols: 250, rows: 8, scrollback: 5000 }), b = create(true)
    const line = 'x'.repeat(250) + '\r\n'
    await a.stream.output(line.repeat(100) + '\x1bPq#17;2;20;40;60!12~\x1b\\\r\n' + line.repeat(3898), 1)
    expect(a.terminal.buffer.normal.baseY + a.terminal.buffer.normal.cursorY).toBe(3999)
    // A tall image crosses the future 4000-row crop boundary.
    await a.stream.output('\x1bPq"1;1;12;60#1;2;100;0;0' + '!12~-'.repeat(9) + '!12~\x1b\\', 2)
    const current = a.terminal.buffer.normal.baseY + a.terminal.buffer.normal.cursorY
    await a.stream.output(line.repeat(5007 - current), 3)
    expect(a.terminal.buffer.normal.baseY).toBe(5000)
    await a.stream.output('\x1b[2;4H\x1bPq#2;2;0;100;0!12~\x1b\\\x1b[6;4H\x1b]8;id=normal;https://example.test\x07normal link\x1b]8;;\x07\x1b[?1049h\x1b[2J\x1b[3;4H\x1bPq#1!12~\x1b\\\x1b]9;4;1;61\x07', 4)
    const originalImages = (a.image as any)._storage._images
    const crossingMarker = originalImages.get(2).marker.line
    const crossingTiles = originalImages.get(2).tileCount
    const snap = await a.stream.run(() => captureTerminalSnapshot('s', a.stream, a))
    expect(new TextEncoder().encode(JSON.stringify(snap)).length).toBeLessThanOrEqual(16 * 1024 * 1024)
    expect(snap.state.normal.lines).toHaveLength(1008)
    expect(snap.state.images.images.map((image: any) => image.id)).toEqual([2, 3, 4])
    expect(snap.state.images.images[0].marker).toBe(crossingMarker - 4000)
    expect(snap.state.images.images[0].tileCount).toBeLessThan(crossingTiles)
    expect(originalImages.size).toBe(4)
    expect(originalImages.get(2).marker.line).toBe(crossingMarker)
    expect(originalImages.get(2).tileCount).toBe(crossingTiles)
    await b.stream.run(() => restoreTerminalSnapshot(b.stream, JSON.parse(JSON.stringify(snap)), b))
    expect(state(b).images).toEqual(snap.state.images)
    const pixels = (host: HTMLElement) => (host.querySelector('canvas.xterm-image-layer') as HTMLCanvasElement)?.toDataURL()
    await expect.poll(() => pixels(b.host)).toBe(pixels(a.host))
    // Palette #17 survives even though its only old image was omitted.
    for (const [i, output] of ['\x1b[5;2H\x1bPq#17!6~\x1b\\', '\x1b[?1049l', '\x1b[7;2H\x1bPq#17!6~\x1b\\'].entries()) {
      await Promise.all([a.stream.output(output, i + 5), b.stream.output(output, i + 5)])
      await expect.poll(() => pixels(b.host)).toBe(pixels(a.host))
    }
    expect(b.image.extractTileAtBufferCell(0, 0)?.toDataURL()).toBe(a.image.extractTileAtBufferCell(0, 4000)?.toDataURL())
    expect(b.image.extractTileAtBufferCell(0, 0)).toBeDefined()
    expect(b.clipboard).toEqual([])
    expect(b.replies).toEqual([])
  })

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
