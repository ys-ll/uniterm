import type { Terminal } from '@xterm/xterm'
import { stripCursorBlink } from '../utils/cursor'
import { sanitizeLiveTerminalOutput } from '../utils/terminalSanitize'

/** Parse at Ground boundaries, retaining unfinished CSI/OSC/DCS and surrogate
 * pairs verbatim. A write callback alone does NOT imply parser Ground. Use the
 * pinned xterm transition table rather than a second approximate VT parser. */
export class TerminalFrames {
  pending = ''
  private readonly table: Uint8Array
  constructor(terminal: Terminal) {
    this.table = (terminal as any)._core._inputHandler._parser._transitions.table
    if (!(this.table instanceof Uint8Array) || this.table.length !== 4095) {
      throw new Error('Unsupported xterm parser table; terminal sharing requires 6.0.0-uniterm.1')
    }
  }

  feed(chunk: string): string[] {
    const data = this.pending + chunk
    let start = 0
    let state = 0
    let text = ''
    const frames: string[] = []
    for (let i = 0; i < data.length; i++) {
      const code = data.codePointAt(i)!
      // StringToUtf32 retains a trailing high surrogate too.
      if (i === data.length - 1 && code >= 0xd800 && code <= 0xdbff) break
      const next = this.table[(state << 8) | (code < 0xa0 ? code : 0xa0)] & 15
      if (state === 0 && next !== 0 && text) { frames.push(text); text = '' }
      state = next
      if (code > 0xffff) i++
      if (state === 0) {
        const frame = data.slice(start, i + 1)
        if (frame.length > 1 && (frame[0] === '\x1b' || frame.charCodeAt(0) >= 0x80 && frame.charCodeAt(0) < 0xa0)) {
          if (text) { frames.push(text); text = '' }
          frames.push(frame)
        } else text += frame
        start = i + 1
      }
    }
    if (text) frames.push(text)
    this.pending = data.slice(start)
    if (this.pending.length > 8 * 1024 * 1024) throw new Error('Unfinished terminal control sequence exceeds 8 MiB')
    return frames
  }
}

export interface TerminalOutputOptions { cursorBlink: boolean; sftp: boolean }

/** Same writer for visible, hidden and attached terminals. One complete control
 * frame at a time makes clear-screen transforms independent of transport chunks. */
export class TerminalStream {
  readonly frames: TerminalFrames
  sequence = 0
  error: Error | undefined
  private tail: Promise<unknown> = Promise.resolve()
  private disposed = false
  private queuedBytes = 0

  constructor(
    readonly terminal: Terminal,
    public options: TerminalOutputOptions,
    private readonly afterWrite: () => void = () => {},
  ) { this.frames = new TerminalFrames(terminal) }

  run<T>(fn: () => T | Promise<T>, fatal = false): Promise<T> {
    const result = this.tail.then(() => {
      if (this.disposed) throw new Error('Terminal view detached')
      if (this.error) throw this.error
      return fn()
    })
    this.tail = result.catch(e => { if (fatal) this.error = e instanceof Error ? e : new Error(String(e)) })
    return result
  }

  writeRaw(data: string): Promise<void> {
    if (!data) return Promise.resolve()
    return new Promise(resolve => this.terminal.write(data, () => { this.afterWrite(); resolve() }))
  }

  output(data: string, sequence: number): Promise<void> {
    this.queuedBytes += data.length
    if (this.queuedBytes > 16 * 1024 * 1024) this.error = new Error('Terminal parser queue overflow; exact state unavailable')
    return this.run(async () => {
      try {
        if (sequence && sequence <= this.sequence) return
        let batch = ''
        const frames = this.frames.feed(data)
        for (let i = 0; i < frames.length; i++) {
          const frame = frames[i]
          // Flush before screen-dependent transforms, so ?1049h in this same
          // transport chunk takes effect before an ED request.
          if (frame.startsWith('\x1b') || frame.charCodeAt(0) >= 0x80 && frame.charCodeAt(0) < 0xa0) {
            let text = stripCursorBlink(frame, this.options.cursorBlink)
            if (text === '\x1b[3J') continue
            if (text === '\x1b[2J' || text === '\x1b[H' && frames[i + 1] === '\x1b[2J') {
              await this.writeRaw(batch)
              batch = ''
              if (this.terminal.buffer.active.type !== 'alternate') {
                if (text === '\x1b[H') continue
                text = '\n'.repeat(this.terminal.rows) + '\x1b[H'
              }
            }
            if (this.options.sftp && /^\x1b\]633;S/.test(text)) continue
            batch += text
          } else batch += sanitizeLiveTerminalOutput(frame)
        }
        await this.writeRaw(batch)
        this.sequence = Math.max(this.sequence, sequence)
      } finally { this.queuedBytes -= data.length }
    }, true)
  }

  resize(cols: number, rows: number, sequence: number): Promise<void> {
    return this.run(() => {
      if (sequence && sequence <= this.sequence) return
      this.terminal.resize(cols, rows)
      this.sequence = Math.max(this.sequence, sequence)
    }, true)
  }

  dispose(): void { this.disposed = true }
}

/** Auto replies never reach component onData, suggestions, broadcast or the
 * active-size path. Binary mouse reports are user input (xterm's separate API).
 * No reply regex: pasted text can legitimately look exactly like a CPR. */
export function installTerminalInputRouter(terminal: Terminal, reply: (data: string) => void) {
  const service = (terminal as any)._core.coreService
  const original = service.triggerDataEvent
  if (typeof original !== 'function') throw new Error('Unsupported xterm input source API')
  service.triggerDataEvent = function (data: string, wasUserInput = false) {
    if (wasUserInput) original.call(this, data, true)
    else reply(data)
  }
  return { dispose() { service.triggerDataEvent = original } }
}
