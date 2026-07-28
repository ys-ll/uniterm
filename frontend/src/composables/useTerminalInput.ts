import { ref, computed } from 'vue'
import type { Terminal } from '@xterm/xterm'

export interface CursorPosition {
  x: number
  y: number
}

export interface UseTerminalInputOptions {
  mode: 'ssh' | 'sftp' | 'local'
  sessionId: string | null | undefined
  onHistoryExtract?: (command: string) => void
  onResetSuppress?: () => void
  enableHistory?: boolean
}

export function useTerminalInput(terminal: Terminal | null, options: UseTerminalInputOptions) {
  // Source of truth for the line being typed. Stored as a char-array so
  // mid-string inserts cost O(n) splice instead of 2 string copies + concat
  // per keystroke. lineBuffer is a writable computed that mirrors it as a
  // string for downstream consumers (BaseTerminal reads / writes the
  // string view).
  const lineChars = ref<string[]>([])
  const lineBuffer = computed<string>({
    get: () => lineChars.value.join(''),
    set: (val) => { lineChars.value = Array.from(val) }
  })
  const cursorIndex = ref(0)
  const currentToken = ref('')
  const cursorPixelPos = ref<CursorPosition>({ x: 0, y: 0 })

  let inAlternateScreen = false
  let isPasswordPrompt = false
  let cursorPosRAF: number | null = null

  function stripAnsi(str: string): string {
    return str
      // OSC sequences: ESC ] ... BEL or ESC ] ... ESC \
      .replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, '')
      // CSI sequences: ESC [ params final-byte
      .replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, '')
      // Single-char FE escapes: ESC @ to ESC _, ESC ` to ESC ~
      .replace(/\x1b[@-Z\-_]/g, '')
      // Character set designation: ESC ( B, ESC ) B, etc.
      .replace(/\x1b[()[\]{}][0-9A-Za-z]/g, '')
  }

  const MAX_COMMAND_LENGTH = 200

  function getCurrentCommandFromTerminal(): string | null {
    if (!terminal) return null
    try {
      const buffer = (terminal as any).buffer?.active
      if (!buffer) return null
      // Prompt endings: $ # > ] plus common zsh/oh-my-zsh/powerline glyphs
      const PROMPT_RE = /(.+?[$#>\]❯➜→»λ])(?:\s+|$)(.*)/
      const rows = (terminal as any).rows || 24
      // Cursor row is the cheapest probe: the running command (if any) lives
      // on the line where the cursor currently sits. Check it first, then only
      // walk up until we find a prompt — don't scan the full visible row span
      // when only the most recent prompt line is interesting.
      const cursorY = (buffer.y ?? 0) + (buffer.baseY ?? 0)
      const tryLine = (y: number): string | null => {
        if (y < 0) return null
        const line = buffer.getLine(y)
        if (!line) return null
        const rawText = line.translateToString().trim()
        if (!rawText) return null
        const cleanText = stripAnsi(rawText)
        const match = cleanText.match(PROMPT_RE)
        if (!match) return null
        const promptPart = match[1]
        const lastChar = promptPart.charAt(promptPart.length - 1)
        const isUnicodePrompt = /[❯➜→»λ]/.test(lastChar)
        if (!promptPart.includes('@') && !promptPart.includes('~') &&
            promptPart !== '$' && promptPart !== '#' && !isUnicodePrompt) return null
        const command = match[2].trim()
        if (command && !command.includes('__AI_DONE_') && command.length <= MAX_COMMAND_LENGTH) {
          return command
        }
        return null
      }
      const fromCursor = tryLine(cursorY)
      if (fromCursor !== null) return fromCursor
      // Walk up from the cursor row toward the top of the visible buffer.
      for (let dy = 1; dy < rows; dy++) {
        const y = cursorY - dy
        if (y < 0) break
        const hit = tryLine(y)
        if (hit !== null) return hit
      }
    } catch {
      // Ignore errors
    }
    return null
  }

  function updateToken() {
    const buf = lineChars.value
    const idx = cursorIndex.value
    let beforeCursor = ''
    for (let i = 0; i < idx && i < buf.length; i++) beforeCursor += buf[i]
    // Use the entire command before cursor for suggestion matching,
    // so "git status" matches history entries like "git status --short".
    currentToken.value = beforeCursor.trim()
  }

  let lastTerminalCursorX = -1

  function updateCursorPosition() {
    if (!terminal) {
      cursorPixelPos.value = { x: 0, y: 0 }
      return
    }
    try {
      const core = (terminal as any)._core
      if (!core) return
      const buffer = core.buffer
      const renderer = core._renderService
      if (!buffer || !renderer) return
      const cursorX = buffer.x
      const cursorY = buffer.y
      const dims = renderer.dimensions
      if (dims && dims.css && dims.css.cell) {
        const cellWidth = dims.css.cell.width || 9
        const cellHeight = dims.css.cell.height || 17
        const x = cursorX * cellWidth
        const belowY = (cursorY + 1) * cellHeight
        cursorPixelPos.value = { x, y: belowY }
      }
      // Detect hidden input: local buffer grew but terminal cursor
      // didn't advance → echo is off (password mode)
      if (lineChars.value.length > 0 && cursorX === lastTerminalCursorX && cursorX >= 0) {
        isPasswordPrompt = true
      } else if (cursorX !== lastTerminalCursorX) {
        isPasswordPrompt = false
      }
      lastTerminalCursorX = cursorX
    } catch {
      const el = terminal.element
      if (el) {
        const rect = el.getBoundingClientRect()
        cursorPixelPos.value = { x: 0, y: rect.height }
      }
    }
  }

  function isAtLineEnd(): boolean {
    return cursorIndex.value >= lineChars.value.length
  }

  function handleInput(data: string) {
    if (options.mode !== 'ssh') return
    if (inAlternateScreen) return
    for (let i = 0; i < data.length; i++) {
      const char = data[i]
      const code = data.charCodeAt(i)
      if (char === '\r' || char === '\n') {
        // Save command to history before clearing.
        // Always prefer terminal buffer (has server echo + tab completion),
        // only fall back to local lineBuffer if terminal buffer is unreadable.
        if (options.enableHistory !== false && !isPasswordPrompt) {
          const command = getCurrentCommandFromTerminal()
          if (command && options.onHistoryExtract) {
            options.onHistoryExtract(command)
          }
        }
        if (isPasswordPrompt) {
          isPasswordPrompt = false
        }
        lineChars.value = []
        cursorIndex.value = 0
        // Reset suggestion suppress on new command
        if (options.onResetSuppress) {
          options.onResetSuppress()
        }
      } else if (code === 127 || char === '\b') {
        if (cursorIndex.value > 0) {
          lineChars.value.splice(cursorIndex.value - 1, 1)
          cursorIndex.value--
        }
      } else if (code === 1) {
        // Ctrl+A — beginning of line
        cursorIndex.value = 0
      } else if (code === 5) {
        // Ctrl+E — end of line
        cursorIndex.value = lineChars.value.length
      } else if (code === 11) {
        // Ctrl+K — delete from cursor to end of line
        lineChars.value.length = cursorIndex.value
      } else if (code === 21) {
        // Ctrl+U — delete from beginning to cursor
        lineChars.value.splice(0, cursorIndex.value)
        cursorIndex.value = 0
      } else if (code === 27) {
        i++
        if (data[i] === '[') {
          i++
          let param = ''
          while (i < data.length && ((data[i] >= '0' && data[i] <= '9') || data[i] === ';')) {
            param += data[i]
            i++
          }
          const finalChar = data[i]
          if (finalChar === 'D') {
            // Left arrow
            if (cursorIndex.value > 0) cursorIndex.value--
          } else if (finalChar === 'C') {
            // Right arrow
            if (cursorIndex.value < lineChars.value.length) cursorIndex.value++
          } else if (finalChar === 'H' && param === '') {
            // Home
            cursorIndex.value = 0
          } else if (finalChar === 'F' && param === '') {
            // End
            cursorIndex.value = lineChars.value.length
          } else if (finalChar === '~') {
            if (param === '1' || param === '7') {
              // Home (alternate)
              cursorIndex.value = 0
            } else if (param === '4' || param === '8') {
              // End (alternate)
              cursorIndex.value = lineChars.value.length
            } else if (param === '3') {
              // Delete
              if (cursorIndex.value < lineChars.value.length) {
                lineChars.value.splice(cursorIndex.value, 1)
              }
            }
          }
        }
      } else if (code >= 32) {
        // Support all printable characters including CJK
        lineChars.value.splice(cursorIndex.value, 0, char)
        cursorIndex.value++
      }
    }
    updateToken()
    // Coalesce cursor position update to next paint frame via rAF. The
    // suggestion popup only needs roughly correct placement, so per-keystroke
    // updates are wasteful under typing bursts — rAF naturally coalesces
    // multiple keystrokes within one frame into a single update.
    if (cursorPosRAF !== null) {
      cancelAnimationFrame(cursorPosRAF)
    }
    cursorPosRAF = requestAnimationFrame(() => {
      cursorPosRAF = null
      updateCursorPosition()
    })
  }

  function handleSessionData(data: string) {
    if (options.mode !== 'ssh') return

    // Detect alternate screen buffer enter/exit (vim, k9s, less, etc.)
    if (data.includes('\x1b[?1049h') || data.includes('\x1b[?47h')) {
      inAlternateScreen = true
      return
    }
    if (data.includes('\x1b[?1049l') || data.includes('\x1b[?47l')) {
      inAlternateScreen = false
    }
  }

  function clearBuffer() {
    lineChars.value = []
    cursorIndex.value = 0
    currentToken.value = ''
  }

  function isInAlternateScreen(): boolean {
    return inAlternateScreen
  }

  function isPasswordMode(): boolean {
    return isPasswordPrompt
  }

  return {
    lineBuffer,
    cursorIndex,
    currentToken,
    cursorPixelPos,
    isAtLineEnd,
    handleInput,
    handleSessionData,
    clearBuffer,
    isInAlternateScreen,
    isPasswordMode,
  }
}
