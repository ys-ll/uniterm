import { EventsOn } from '../../wailsjs/runtime'
import { SessionWrite } from '../../wailsjs/go/main/App'
import { getManagedTerminal } from '../services/terminalManager'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'

export interface ExecuteResult {
  output: string
  exitCode: number
  timedOut?: boolean
  cancelled?: boolean
}

export interface WatchResult {
  output: string
  timedOut: boolean
  cancelled?: boolean
}

// Split terminal output into display lines. Splits on newlines and, within a
// line, keeps only the text after the last carriage return so progress-bar
// style redraws (which overwrite the line with bare \r) collapse to their
// final state — the same way the text appears on screen.
function toDisplayLines(clean: string): string[] {
  return clean.split(/\r?\n/).map((line) => {
    const cr = line.lastIndexOf('\r')
    return cr >= 0 ? line.slice(cr + 1) : line
  })
}

// Watch session output and resolve when the command finishes.
//
// Completion is detected by the shell prompt reappearing: `promptLine` is the
// prompt captured immediately before the command was sent, and once that exact
// line shows up again at the bottom of the output the shell is back at the
// prompt and the command is done. No marker is injected into the shell, so the
// terminal shows nothing extra.
//
// As a fallback for dynamic prompts (timestamps, git branches, etc.) or ANSI
// mismatches, an idle heuristic is also used: if output stops for a short
// period and the last non-blank line looks like a prompt (ends with $, #, >,
// %, or :), the command is considered finished.
//
// When `promptLine` is empty and the heuristic cannot match, detection is
// skipped entirely and the call resolves on timeout.
//
// `shouldCancel` is polled periodically. When it returns true the watcher
// stops listening and resolves with cancelled=true. The terminal command is
// NOT interrupted; callers should discard the output and not pass it to the
// LLM.
export function watchOutput(
  sessionId: string,
  promptLine: string,
  timeoutMs: number,
  shouldCancel?: () => boolean
): { promise: Promise<WatchResult>; cleanup: () => void } {
  const IDLE_MS = 800
  const CANCEL_POLL_MS = 150
  let timeoutId: ReturnType<typeof setTimeout>
  let idleTimeoutId: ReturnType<typeof setTimeout> | null = null
  let cancelPollId: ReturnType<typeof setTimeout> | null = null
  let unsubscribe: (() => void) | null = null
  let resolved = false
  let output = ''

  const cleanup = () => {
    clearTimeout(timeoutId)
    if (idleTimeoutId) {
      clearTimeout(idleTimeoutId)
      idleTimeoutId = null
    }
    if (cancelPollId) {
      clearTimeout(cancelPollId)
      cancelPollId = null
    }
    unsubscribe?.()
    resolved = true
  }

  function getLastDisplayLine(text: string): { line: string; index: number } | null {
    const lines = toDisplayLines(stripAnsi(text))
    let last = lines.length - 1
    while (last >= 0 && lines[last].trimEnd() === '') last--
    if (last < 0) return null
    return { line: lines[last].trimEnd(), index: last }
  }

  function looksLikePrompt(line: string): boolean {
    // Common prompt terminators: $, #, >, %, :
    // Avoid matching plain text lines that happen to end with these chars by
    // requiring a reasonably short line (prompts are rarely > 200 chars).
    return line.length <= 200 && /[\$#>%:]\s*$/.test(line)
  }

  const promise = new Promise<WatchResult>((resolve) => {
    const finish = (timedOut: boolean, cancelled = false) => {
      cleanup()
      if (cancelled) {
        resolve({ output: '', timedOut: false, cancelled: true })
        return
      }
      if (timedOut) {
        resolve({ output: stripAnsi(output).trim(), timedOut: true })
        return
      }
      const lastInfo = getLastDisplayLine(output)
      const result = lastInfo
        ? toDisplayLines(stripAnsi(output)).slice(0, lastInfo.index).join('\n').trim()
        : stripAnsi(output).trim()
      resolve({ output: result, timedOut: false })
    }

    const checkIdle = () => {
      if (resolved || !promptLine) return
      const lastInfo = getLastDisplayLine(output)
      if (!lastInfo || lastInfo.index < 1) return
      if (looksLikePrompt(lastInfo.line)) {
        finish(false)
      }
    }

    const checkCancel = () => {
      if (resolved) return
      if (shouldCancel?.()) {
        finish(false, true)
        return
      }
      cancelPollId = setTimeout(checkCancel, CANCEL_POLL_MS)
    }

    unsubscribe = EventsOn('session:data', (payload: { id: string; data: string }) => {
      if (payload.id !== sessionId || resolved) return

      output += payload.data

      // Reset idle detection whenever new data arrives.
      if (idleTimeoutId) {
        clearTimeout(idleTimeoutId)
        idleTimeoutId = null
      }
      idleTimeoutId = setTimeout(checkIdle, IDLE_MS)

      const lastInfo = getLastDisplayLine(output)
      if (!lastInfo || lastInfo.index < 1) return

      // Exact prompt match (works when the prompt is static and ANSI-stripped).
      if (promptLine && lastInfo.line === promptLine) {
        finish(false)
        return
      }
    })

    if (shouldCancel) {
      cancelPollId = setTimeout(checkCancel, CANCEL_POLL_MS)
    }

    timeoutId = setTimeout(() => {
      finish(true)
    }, timeoutMs)
  })

  return { promise, cleanup }
}

export function truncateOutput(
  text: string,
  headLines: number,
  tailLines: number
): string {
  const lines = text.split('\n')
  const total = lines.length
  const threshold = headLines + tailLines
  if (total <= threshold) return text

  const head = lines.slice(0, headLines).join('\n')
  const tail = lines.slice(total - tailLines).join('\n')
  const omitted = total - headLines - tailLines
  return `${head}\n\n─────── [截断: 共 ${total} 行, 已省略 ${omitted} 行] ────────\n调整 head_lines / tail_lines 参数可查看更多内容。\n\n${tail}`
}

function resolveActiveSession(): { sessionId: string; shellPath?: string } {
  const tabStore = useTabStore()
  const panelStore = usePanelStore()

  const lockedPanelId = tabStore.getAILockedPanel()
  let panel = lockedPanelId ? panelStore.getPanel(lockedPanelId) : null

  if (!panel) {
    const activeTab = tabStore.activeTab
    if (activeTab?.type === 'terminal' || activeTab?.type === 'settings') {
      panel = panelStore.getPanel(activeTab.panelId)
    } else if (activeTab?.type === 'workspace' && activeTab.activePanelId) {
      panel = panelStore.getPanel(activeTab.activePanelId)
    }
  }

  if (!panel || !panel.sessionId) {
    throw new Error('No active terminal session')
  }

  return { sessionId: panel.sessionId, shellPath: panel.config?.shellPath }
}

function getShellNewline(shellPath?: string): string {
  const lowerShell = (shellPath || '').toLowerCase()
  if (lowerShell.includes('powershell') || lowerShell.includes('pwsh')) {
    return '\r'
  } else if (lowerShell.includes('cmd')) {
    return '\r\n'
  } else if (lowerShell.includes('bash') || lowerShell.includes('sh')) {
    return '\r\n'
  } else {
    return '\n'
  }
}

// Read the current prompt line from the terminal buffer. Called right before a
// command is sent, when the cursor sits on the (freshly drawn) prompt with no
// input yet, so the cursor line's text is exactly the prompt string. ANSI
// sequences are stripped so the captured prompt matches the stripped output
// used in watchOutput. Returns '' when unavailable, which disables exact prompt
// detection for that command (idle heuristic still applies).
function capturePromptLine(sessionId: string): string {
  const managed = getManagedTerminal(sessionId)
  const terminal = managed?.terminal
  if (!terminal) return ''
  const buffer = terminal.buffer.active
  const line = buffer.getLine(buffer.baseY + buffer.cursorY)
  if (!line) return ''
  return stripAnsi(line.translateToString(true)).trimEnd()
}

export async function executeCommand(
  command: string,
  timeoutMs: number = 60000,
  headLines: number = 50,
  tailLines: number = 300,
  shouldCancel?: () => boolean
): Promise<ExecuteResult> {
  const { sessionId, shellPath } = resolveActiveSession()
  const promptLine = capturePromptLine(sessionId)
  const fullCommand = buildCommand(command, shellPath)
  const newline = getShellNewline(shellPath)

  await SessionWrite(sessionId, fullCommand + newline)

  const { promise } = watchOutput(sessionId, promptLine, timeoutMs, shouldCancel)
  const result = await promise

  if (result.cancelled) {
    return {
      output: '',
      exitCode: -1,
      timedOut: false,
    }
  }

  if (result.timedOut) {
    const truncated = truncateOutput(result.output, headLines, tailLines)
    const timeoutSec = Math.round(timeoutMs / 1000)
    return {
      output: truncated
        + `\n\n⚠️ 命令在 ${timeoutSec}s 内未完成，可能仍在运行中。\n`
        + `请勿重复发送相同命令。\n`
        + `• 如果输出显示进度（百分比、文件名滚动等）→ 使用 collect_output 继续等待\n`
        + `• 如果输出显示密码/确认提示 → 使用 send_terminal_key 响应\n`
        + `• 如果命令卡住无响应 → 使用 interrupt_command 取消`,
      exitCode: -1,
      timedOut: true,
    }
  }

  return {
    output: truncateOutput(result.output, headLines, tailLines),
    exitCode: 0,
    timedOut: false,
  }
}

export interface StartResult {
  output: string
  started: boolean
}

export async function startCommand(command: string): Promise<StartResult> {
  const { sessionId, shellPath } = resolveActiveSession()
  const newline = getShellNewline(shellPath)

  await SessionWrite(sessionId, command + newline)

  // Collect output for 3 seconds, then return
  return new Promise((resolve) => {
    let output = ''
    const unsubscribe = EventsOn('session:data', (payload: { id: string; data: string }) => {
      if (payload.id !== sessionId) return
      output += payload.data
    })

    setTimeout(() => {
      unsubscribe()
      resolve({
        output: stripAnsi(output).trim(),
        started: true,
      })
    }, 3000)
  })
}

export interface CaptureResult {
  output: string
}

export function captureTerminal(tailLines: number = 200): CaptureResult {
  const { sessionId } = resolveActiveSession()

  const managed = getManagedTerminal(sessionId)
  if (!managed || !managed.terminal) {
    return { output: '' }
  }

  const terminal = managed.terminal
  const buffer = terminal.buffer.active
  const totalLines = buffer.length

  if (totalLines === 0) {
    return { output: '' }
  }

  // Find the last non-blank line — skip trailing empty space at the bottom of the terminal
  let lastContentLine = totalLines - 1
  while (lastContentLine >= 0) {
    const line = buffer.getLine(lastContentLine)
    if (line && line.translateToString().trim() !== '') break
    lastContentLine--
  }

  if (lastContentLine < 0) {
    return { output: '' }
  }

  // Capture up to tailLines lines, ending at the last non-blank line
  const startLine = Math.max(0, lastContentLine - tailLines + 1)
  const lines: string[] = []

  for (let i = startLine; i <= lastContentLine; i++) {
    const line = buffer.getLine(i)
    if (line) lines.push(line.translateToString())
  }

  return { output: lines.join('\n') }
}

export interface CollectResult {
  output: string
  timedOut: boolean
  completed: boolean
}

// Collect output passively — no command is sent. Detects completion by watching
// for the shell prompt to reappear (same idle heuristic as watchOutput), so the
// call returns as soon as the running command finishes instead of always waiting
// for the full timeout.
export async function collectOutput(
  timeoutMs: number = 30000,
  headLines: number = 100,
  tailLines: number = 300,
  shouldCancel?: () => boolean
): Promise<CollectResult> {
  const { sessionId } = resolveActiveSession()
  const promptLine = capturePromptLine(sessionId)

  const { promise } = watchOutput(sessionId, promptLine, timeoutMs, shouldCancel)
  const result = await promise

  if (result.cancelled) {
    return { output: '', timedOut: false, completed: false }
  }

  return {
    output: truncateOutput(result.output, headLines, tailLines),
    timedOut: result.timedOut,
    completed: !result.timedOut,
  }
}

interface SendKeyResult {
  output: string
}

export async function sendTerminalKey(
  input?: string,
  control?: 'ctrl_c' | 'ctrl_d' | 'enter',
  sendEnter: boolean = true
): Promise<SendKeyResult> {
  const { sessionId, shellPath } = resolveActiveSession()

  let data: string
  if (control) {
    if (control === 'ctrl_c') {
      data = '\x03'
    } else if (control === 'ctrl_d') {
      data = '\x04'
    } else if (control === 'enter') {
      data = '\n'
    } else {
      data = ''
    }
  } else if (input !== undefined && input !== '') {
    data = input
  } else {
    throw new Error('Either input or control must be provided')
  }

  // Append shell-appropriate newline when send_enter is true and input was provided
  if (sendEnter && !control && input !== undefined && input !== '') {
    data += getShellNewline(shellPath)
  }

  await SessionWrite(sessionId, data)

  // For ctrl_c / ctrl_d: passively capture shell response for a short time.
  // No marker injection — avoids corrupting interactive program input.
  if (control === 'ctrl_c' || control === 'ctrl_d') {
    return new Promise((resolve) => {
      let output = ''
      const unsubscribe = EventsOn('session:data', (payload: { id: string; data: string }) => {
        if (payload.id !== sessionId) return
        output += payload.data
      })
      setTimeout(() => {
        unsubscribe()
        resolve({ output: stripAnsi(output).trim() || '(input sent)' })
      }, 1000)
    })
  }

  return { output: '(input sent)' }
}

// Build the string sent to the shell. No completion marker is appended — the
// AI executor detects completion by watching for the shell prompt to reappear
// (see watchOutput). This keeps the terminal clean and, for POSIX shells,
// avoids corrupting multi-line input such as here-documents. A single leading
// space keeps the command out of shell history (HISTCONTROL=ignorespace).
function buildCommand(command: string, shellPath?: string): string {
  const lower = (shellPath || '').toLowerCase()
  if (lower.includes('powershell') || lower.includes('pwsh') || lower.includes('cmd')) {
    return command
  }
  // bash / sh / zsh / fish
  return ` ${command}`
}

// Simple ANSI stripper for extracting readable text from terminal output
function stripAnsi(str: string): string {
  return str
    .replace(/\x1B\[[0-9;?]*[A-Za-z]/g, '')
    .replace(/\x1B][0-9;]*(?:\x07|\x1B\\)/g, '')
    .replace(/\x1B[()[\]#\^%@>=]/g, '')
    .replace(/\x1B[/!_]./g, '')
    .replace(/\x1B./g, '')
}
