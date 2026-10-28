import type { Terminal } from '@xterm/xterm'
import { recordWrite, currentAbsoluteLine } from '../services/terminalTimestamps'
import { updateCommandWraps } from '../services/terminalCommandWrap'
import { stripCursorBlink } from '../utils/cursor'
import { sanitizeLiveTerminalOutput } from '../utils/terminalSanitize'
import { DcsReassembler } from '../utils/dcsReassembler'
import { useSettingsStore } from '../stores/settingsStore'
import type { useTerminalInput } from './useTerminalInput'
import type { useSuggestions } from './useSuggestions'

export interface UseTerminalRenderOptions {
  getTerminal: () => Terminal | null
  getMode: () => 'ssh' | 'sftp' | 'local'
  getSessionId: () => string | null | undefined
  getTerminalInput: () => ReturnType<typeof useTerminalInput> | null
  suggestions: ReturnType<typeof useSuggestions>
}

/**
 * Terminal data rendering pipeline: strip cursor blink, expand clear-screen,
 * sanitize live output, reassemble DCS sequences, and stamp writes with
 * timestamps. Owns the DCS reassembler and the written-chunk counter.
 */
export function useTerminalRender(options: UseTerminalRenderOptions) {
  const { getTerminal, getMode, getSessionId, getTerminalInput, suggestions } = options
  const settingsStore = useSettingsStore()

  // Track how many sessionStore chunks have been written to the terminal
  // so we can replay only missed data on KeepAlive reactivation.
  let writtenChunks = 0

  // Reassembles DCS (sixel image) sequences split across session:data chunks
  // so highlight's per-chunk segmenter never injects SGR codes into an open
  // DCS body. Every chunk this component renders must pass through it exactly
  // once, in order — including the KeepAlive gap replay below.
  const dcsReassembler = new DcsReassembler()

  // get their timestamp fixed to the completion time. `before` is an absolute
  // row index, so a trim inside the write doesn't skew the band.
  function writeStamped(data: string, onParsed?: () => void) {
    const term = getTerminal()
    const sid = getSessionId()
    if (!term || !sid) { term?.write(data, onParsed); return }
    const ts = Date.now()
    const before = currentAbsoluteLine(sid)
    term.write(data, () => {
      updateCommandWraps(term)
      recordWrite(sid, before, ts)
      onParsed?.()
    })
  }

  function renderTerminalData(rawData: string, countChunk = true) {
    const terminal = getTerminal()
    if (!terminal) return
    // MobaXterm-style clear-screen (owner decision, supersedes the native
    // handling from issue #1028): `clear` should scroll the current viewport
    // into the scrollback instead of wiping it, so history is never lost.
    // \x1b[3J (erase scrollback) is stripped — otherwise shells that emit
    // ED2+ED3 would wipe the scrollback right after we pushed content into it.
    // \x1b[2J is replaced by newline padding + home, pushing the viewport into
    // scrollback before clearing. Full-screen apps (vim, tmux…) run on the
    // alternate buffer and are left untouched.
    let data = stripCursorBlink(rawData, settingsStore.settings.terminal.cursorBlink ?? true).replace(/\x1b\[3J/g, '')
    if (data.includes('\x1b[2J') && terminal.buffer.active.type !== 'alternate') {
      // Scroll from the current cursor position: rows newlines push every row
      // above and including the cursor into the scrollback — and nothing
      // below it, so the blank rows under a mid-screen prompt never pollute
      // the history. Then wipe whatever is left on screen: the trailing blank
      // rows for a shell prompt, or residue from a program that exited with
      // the cursor mid-screen (that residue is dropped from history by
      // design — trading it for a clean screen and unpolluted scrollback).
      const scrollClear = '\n'.repeat(terminal.rows) + '\x1b[H\x1b[2J'
      // Single pass: scrollClear itself ends with \x1b[H\x1b[2J, so a second
      // replace pass would re-expand the sequence we just inserted.
      data = data.replace(/\x1b\[H?\x1b\[2J/g, scrollClear)
    }
    data = sanitizeLiveTerminalOutput(data)
    if (getMode() === 'sftp') {
      const cleaned = data.replace(/\x1b\]633;S[^\x07]*\x07/g, '')
      if (cleaned) writeStamped(cleaned)
    } else {
      // Keyword highlighting is applied by the overlay renderer, which scans
      // the parsed buffer and lays decorations over matches — the data stream
      // is written verbatim.
      for (const seg of dcsReassembler.feed(data)) {
        if (seg.dcs) {
          // Complete DCS (sixel image, DECRQSS reply, …): write verbatim —
          // SGR codes injected inside a DCS body would corrupt it.
          writeStamped(seg.text)
          continue
        }
        const terminalInput = getTerminalInput()
        if (getMode() === 'ssh' && terminalInput) {
          terminalInput.handleSessionData(seg.text)
          if (terminalInput.isInAlternateScreen()) suggestions.close()
        }
        writeStamped(seg.text)
      }
    }
    if (countChunk) writtenChunks++
  }

  return {
    renderTerminalData,
    writeStamped,
    getWrittenChunks: () => writtenChunks,
    setWrittenChunks: (n: number) => { writtenChunks = n },
    getDcsReassembler: () => dcsReassembler,
  }
}