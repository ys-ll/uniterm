import { CaptureScreen, SendKey, StartForegroundCapture, InterruptForeground, PollForeground, StartForeground } from '../../bindings/github.com/ys-ll/uniterm/app'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useSessionStore } from '../stores/sessionStore'
import { watch } from 'vue'

// F-317: maintain a title→panelId index for O(1) lookups in
// resolveActiveSession. The original implementation spread panelStore.panels
// into an array and ran two .find passes per command call (O(n) per call).
// A vue watch on the reactive panels Map keeps the index in sync with
// add / remove / rename; we rebuild on every change because titles can
// duplicate and the index is tiny.
const panelTitleIndex: Map<string, string[]> = new Map()
let panelTitleIndexInit = false

function rebuildPanelTitleIndex(panels: Map<string, { id: string; title: string }>): void {
  panelTitleIndex.clear()
  for (const p of panels.values()) {
    const arr = panelTitleIndex.get(p.title)
    if (arr) arr.push(p.id)
    else panelTitleIndex.set(p.title, [p.id])
  }
}

function ensurePanelTitleIndex(): void {
  if (panelTitleIndexInit) return
  panelTitleIndexInit = true
  const panelStore = usePanelStore()
  rebuildPanelTitleIndex(panelStore.panels as Map<string, { id: string; title: string }>)
  // Watch the reactive Map: rebuild on size change (set/delete) and on any
  // title mutation (deep watch on the values). The index is tiny, so a full
  // rebuild is cheaper than a more granular diff.
  watch(
    () => panelStore.panels,
    () => rebuildPanelTitleIndex(panelStore.panels as Map<string, { id: string; title: string }>),
    { deep: true, flush: 'sync' }
  )
}

export interface ExecuteResult {
  output: string
  exitCode: number
  timedOut?: boolean
  cancelled?: boolean
  completed?: boolean
}

// One cursor per terminal. Repeated collection reads the original job buffer,
// including output that arrived between tool calls.
const foregroundCursors = new Map<string, { jobId: string; offset: number }>()

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
  return `${head}\n\n─────── [TRUNCATED: ${total} lines total, ${omitted} lines omitted] ────────\nAdjust head_lines / tail_lines to see more content.\n\n${tail}`
}

function resolveActiveSession(panelTitle?: string): { sessionId: string; shellPath?: string; remoteOS?: string } {
  const tabStore = useTabStore()
  const panelStore = usePanelStore()

  let panel

  if (panelTitle) {
    ensurePanelTitleIndex()
    // F-317: O(1) exact title match via the title→panelId index. The index
    // covers add/remove/rename via a vue watch; titles can duplicate, so we
    // verify the cached id still points at a panel with the requested title.
    let panelId: string | undefined
    const ids = panelTitleIndex.get(panelTitle)
    if (ids && ids.length > 0) {
      const candidate = panelStore.getPanel(ids[0])
      if (candidate && candidate.title === panelTitle) {
        panelId = ids[0]
      } else {
        // Stale entry (title just changed). Rebuild on demand.
        rebuildPanelTitleIndex(panelStore.panels as Map<string, { id: string; title: string }>)
        const refreshed = panelTitleIndex.get(panelTitle)
        if (refreshed && refreshed.length > 0) panelId = refreshed[0]
      }
    } else {
      // Title isn't in the index yet — rebuild and try once more.
      rebuildPanelTitleIndex(panelStore.panels as Map<string, { id: string; title: string }>)
      const refreshed = panelTitleIndex.get(panelTitle)
      if (refreshed && refreshed.length > 0) panelId = refreshed[0]
    }
    if (panelId) {
      panel = panelStore.getPanel(panelId)
    } else {
      // Suffix match for duplicate names: "title (id: xxx)". The id is
      // already known, so use getPanel for an O(1) by-id lookup.
      const suffixMatch = panelTitle.match(/^(.+)\s+\(id:\s*(.+)\)$/)
      if (suffixMatch) {
        panel = panelStore.getPanel(suffixMatch[2])
      }
    }
    if (!panel || !panel.sessionId) {
      throw new Error(`Panel "${panelTitle}" not found or has no active session`)
    }
  } else {
    // Default logic: first locked panel > active panel
    const lockedPanels = tabStore.getAILockedPanels()
    if (lockedPanels.length > 0) {
      panel = panelStore.getPanel(lockedPanels[0])
    }
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
  }

  const sessionId = panel.sessionId
  return {
    sessionId,
    shellPath: panel.config?.shellPath,
    remoteOS: useSessionStore().getRemoteOS(sessionId),
  }
}

export async function executeCommand(
  command: string,
  timeoutMs: number = 60000,
  headLines: number = 50,
  tailLines: number = 300,
  shouldCancel?: () => boolean,
  panelTitle?: string
): Promise<ExecuteResult> {
  const { sessionId, shellPath, remoteOS } = resolveActiveSession(panelTitle)
  // Single execution engine: the backend MCP foreground job registry types the command
  // into the live terminal, watches via an additive data tap, and resolves
  // output through a screen emulator — so ConPTY redraw duplication (Windows
  // OpenSSH) never reaches the AI. The per-session busy lock lives there too,
  // shared with the MCP foreground toolset. timeoutMs is the max wait: the
  // engine moves the job to the pollable "timeout" state on expiry, matching
  // the built-in AI's timedOut + collect_output flow.
  let jobId: string
  try {
    jobId = await StartForeground(sessionId, buildCommand(command, shellPath, remoteOS), timeoutMs)
  } catch (e: any) {
    throw new Error(e?.message ?? String(e))
  }

  const cursor = { jobId, offset: 0 }
  foregroundCursors.set(sessionId, cursor)
  return waitForForeground(jobId, timeoutMs, headLines, tailLines, shouldCancel, cursor)
}

async function waitForForeground(
  jobId: string, timeoutMs: number, headLines: number, tailLines: number,
  shouldCancel?: () => boolean,
  cursor = { jobId, offset: 0 }
): Promise<ExecuteResult> {
  const dec = new TextDecoder()
  const POLL_MS = 150
  const PAGE = 64 * 1024
  const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

  let offset = cursor.offset
  let acc = ''
  let state = 'running'
  let exitCode = -1
  const start = Date.now()
  while (true) {
    if (shouldCancel?.()) {
      await InterruptForeground(jobId, 'SIGINT').catch(() => {})
      return { output: '', exitCode: -1, timedOut: false, cancelled: true }
    }
    try {
      const [out, , code, st] = await PollForeground(jobId, offset, PAGE)
      // Wails serializes Go []byte as base64. Offsets count bytes, and
      // streaming decode preserves UTF-8 characters split across pages.
      const bytes = foregroundBytes(out)
      acc += dec.decode(bytes, { stream: true })
      offset += bytes.length
      cursor.offset = offset
      exitCode = code
      if (st !== 'running' && bytes.length < PAGE) {
        state = st
        break
      }
      if (bytes.length === PAGE) continue
    } catch (e: any) {
      // Job reaped or the bridge errored: surface as an execution error.
      throw new Error(e?.message ?? String(e))
    }
    // Safety net; the engine itself moves the job to "timeout" at hardLimit.
    if (Date.now() - start >= timeoutMs + 5000) {
      state = 'timeout'
      break
    }
    await sleep(POLL_MS)
  }

  return {
    output: truncateOutput(acc + dec.decode(), headLines, tailLines),
    exitCode: state === 'exited' ? exitCode : -1,
    timedOut: state === 'timeout',
    completed: state === 'exited',
  }
}

export interface StartResult {
  output: string
  started: boolean
}

function foregroundBytes(base64: string): Uint8Array {
  return Uint8Array.from(atob(base64), c => c.charCodeAt(0))
}

export async function startCommand(command: string, panelTitle?: string): Promise<StartResult> {
  const { sessionId, shellPath, remoteOS } = resolveActiveSession(panelTitle)
  // Same engine as executeCommand; start_command just samples the job's
  // screen-resolved output after a short window and returns while the
  // command and output capture keep running until shell completion or retention expiry.
  let jobId: string
  try {
    jobId = await StartForeground(sessionId, buildCommand(command, shellPath, remoteOS), 5 * 60_000)
  } catch (e: any) {
    throw new Error(e?.message ?? String(e))
  }

  const cursor = { jobId, offset: 0 }
  foregroundCursors.set(sessionId, cursor)
  await new Promise((r) => setTimeout(r, 3000))
  try {
    const [out] = await PollForeground(jobId, 0, 64 * 1024)
    const bytes = foregroundBytes(out)
    cursor.offset = bytes.length
    const output = new TextDecoder().decode(bytes).trim()
    return { output: output || '(no output yet)', started: true }
  } catch (e: any) {
    return { output: '(output unavailable)', started: true }
  }
}

export interface CaptureResult {
  output: string
}

export async function captureTerminal(tailLines: number = 200, panelTitle?: string): Promise<CaptureResult> {
  const { sessionId } = resolveActiveSession(panelTitle)
  return { output: await CaptureScreen(sessionId, tailLines) }
}

export interface CollectResult {
  output: string
  timedOut: boolean
  completed: boolean
}

// Passive collection uses the same backend observer as MCP.
export async function collectOutput(
  timeoutMs: number = 30000,
  headLines: number = 100,
  tailLines: number = 300,
  shouldCancel?: () => boolean,
  panelTitle?: string
): Promise<CollectResult> {
  const { sessionId } = resolveActiveSession(panelTitle)
  const jobId = await StartForegroundCapture(sessionId, timeoutMs)
  let cursor = foregroundCursors.get(sessionId)
  if (!cursor || cursor.jobId !== jobId) {
    cursor = { jobId, offset: 0 }
    foregroundCursors.set(sessionId, cursor)
  }
  const result = await waitForForeground(jobId, timeoutMs, headLines, tailLines, shouldCancel, cursor)
  return {
    output: result.output,
    timedOut: !!result.timedOut,
    completed: !!result.completed,
  }
}

interface SendKeyResult {
  output: string
}

// Close the backend watcher (and with it the per-session busy lock) for the
// session's tracked job. Killing the foreground process with Ctrl+C prevents
// the shell acknowledgement from ever printing, so interrupt_command must
// close the watcher explicitly or the session stays locked until the job TTL.
export async function interruptJob(panelTitle?: string): Promise<void> {
  const { sessionId } = resolveActiveSession(panelTitle)
  const cursor = foregroundCursors.get(sessionId)
  if (cursor) {
    await InterruptForeground(cursor.jobId, 'SIGINT').catch(() => {})
  }
}

export async function sendTerminalKey(
  input?: string,
  control?: 'ctrl_c' | 'ctrl_d' | 'enter',
  sendEnter: boolean = true,
  panelTitle?: string
): Promise<SendKeyResult> {
  const { sessionId } = resolveActiveSession(panelTitle)
  await SendKey(sessionId, input ?? '', control ?? '', sendEnter)
  if (control === 'ctrl_c' || control === 'ctrl_d') {
    await new Promise(r => setTimeout(r, 1000))
    return { output: await CaptureScreen(sessionId, 200) || '(input sent)' }
  }
  return { output: '(input sent)' }
}

// Shell-history adaptation only; completion and terminal parsing live in Go.
function buildCommand(command: string, shellPath?: string, remoteOS?: string): string {
  const lower = (shellPath || '').toLowerCase()
  if (remoteOS === 'windows-openssh' || lower.includes('powershell') || lower.includes('pwsh') || lower.includes('cmd')) {
    return command
  }
  // bash / sh / zsh / fish
  return ` ${command}`
}
