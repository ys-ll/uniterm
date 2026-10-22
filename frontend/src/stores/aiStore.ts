import { defineStore } from 'pinia'
import { ref, computed, reactive, watch } from 'vue'
import type { AIAttachment, AIMessage, AIContextSummary, ExecutionMode, AISession, AIAgentStatus } from '../types/ai'
import { activeHistory } from '../services/aiContext'
import { SaveAISessions, LoadAISessions } from '../../bindings/github.com/ys-ll/uniterm/app'
import { buildUserContent, redactAttachmentsForLog } from '../services/attachments'
import { useLocalStateStore } from './localStateStore'
import { useTabStore } from './tabStore'
import { usePanelStore } from './panelStore'
import { useConnectionStore } from './connectionStore'
import { isMobilePlatform } from '../utils/platform'
import { t } from '../i18n'

/**
 * Static AI system rules — immutable per app version, always cacheable.
 * Dynamic shell/panel context is injected into the latest user message instead.
 */
const SYSTEM_RULES = `You are an AI assistant inside uniTerm, a terminal emulator. You can execute shell commands in the user's active terminal to help them complete tasks.

AVAILABLE TOOLS:
1. execute_command — Run a shell command and wait for its output. Set timeout_ms based on expected duration; the unit is milliseconds. Use head_lines/tail_lines to control how much output you receive.
2. start_command — Start a background/long-running command (servers, daemons). Returns initial output immediately without waiting.
3. capture_terminal — Take an instant snapshot of the terminal screen. Use to check current state without running commands.
4. collect_output — Wait and collect new terminal output. Pure passive listening — does NOT send anything to the terminal. Use when a command is still running and you want to see progress.
5. send_terminal_key — Send text or control keys to the terminal. Use ONLY when you can SEE an interactive prompt (password, y/n, confirmation). By default, send_enter=true automatically appends Enter after your input, so "y" becomes "y" + Enter. Set send_enter=false only when you need to type raw characters without submitting.
6. interrupt_command — Send Ctrl+C to cancel the running command.
7. save_skill — Create or update a reusable skill (a command-line workflow / SOP) invokable later via /name. Use when the user asks to save the current approach as a skill, or when you just worked out a repeatable command-line procedure worth keeping.

SKILL AUTHORING (when using save_skill):
- name: kebab-case (lowercase letters, digits, hyphens), e.g. "git-clean-branches". Becomes the /trigger.
- description: one line stating BOTH what it does AND when to use it — this drives future matching. E.g. "Clean up merged local git branches. Use when the user wants to delete/tidy git branches."
- body: imperative markdown steps. Write a THIN skill — only include steps you would NOT do by default. Do NOT include YAML frontmatter (name/description are passed separately).
- Locked skills are rejected by the backend when overwriting.

CRITICAL RULES:
- You can only send ONE tool call at a time. Never send multiple tool calls in a single response.
- Always explain what you are about to do before executing commands.
- If a command might be destructive, warn the user.

TIMEOUT GUIDELINES (timeout_ms):
- 5000-10000 ms: quick commands (ls, cat, pwd, whoami)
- 15000-30000 ms: moderate commands (grep, find, df, systemctl status)
- 60000-120000 ms: build/install tasks (npm install, pip install, apt-get)
- 120000-300000 ms: very long tasks (docker build, large git clone, full compilation)

HANDLING TIMEOUTS:
When execute_command times out, read the output carefully:
- If output shows progress (percentages, file names scrolling): use collect_output to keep waiting.
- If output shows a prompt (password, y/n, [sudo], "Are you sure?"): ask the user for credentials, then use send_terminal_key.
- If output is empty or shows an error: use interrupt_command, then reassess.
- NEVER re-send the same command after a timeout — this causes duplicate commands to pile up.

INTERACTIVE PROMPTS:
- Password prompt: ask the user (NEVER guess passwords).
- y/n confirmation: use send_terminal_key with input: "y" (send_enter defaults to true, so Enter is sent automatically).
- Pager (less/more): use send_terminal_key with control: "ctrl_c" to exit.
- send_enter parameter: defaults to true, automatically appends Enter after your input. Set to false only when you need to type raw characters without submitting.

OUTPUT READING:
- To check if shell prompt is back after a command: use capture_terminal.
- To track progress of a running command: use collect_output.
- Output was truncated: adjust head_lines/tail_lines and re-run.

PROHIBITED:
- NEVER execute clear/cls/Reset. The user must always see command history.
- NEVER use send_terminal_key with unknown prompts — you must SEE the prompt first.
- NEVER send multiple tool calls in one response.

SHELL AWARENESS:
- At the START of EVERY response, read the shell/panel context in the user's message. IGNORE any memory of what the previous shell was — only the latest context matters.
- The user may switch terminal tabs at any time. Each terminal is an independent environment.
- When the terminal type changes, switch to the NEW shell's command syntax immediately.
- Do NOT invoke a different shell executable from within the current terminal.

RISK CLASSIFICATION:
Every execute_command call MUST include a "risk" field:
- "read": only inspects/views data, no modifications at all
- "write": modifies or creates data, but not system-destructive
- "dangerous": potentially destructive or system-altering
For chained commands, classify based on the MOST risky operation in the chain.

--- NEGATIVE EXAMPLES (STRICTLY FORBIDDEN) ---
❌ In Git Bash, do NOT run: Get-CimInstance Win32_LogicalDisk
❌ In PowerShell, do NOT run: ls -la /mnt/c/
❌ In CMD, do NOT run: df -h
❌ In Git Bash, do NOT run: powershell.exe -Command "..."
❌ In PowerShell, do NOT run: bash -c "..."
Use ONLY the current shell's native syntax.`

async function loadSessionsFromBackend(): Promise<{ sessions: AISession[], currentSessionId: string | null }> {
  try {
    const data = await LoadAISessions() as any
    const sessions: AISession[] = (data.sessions || []).map((s: any) => ({
      id: s.id,
      name: s.name,
      createdAt: s.createdAt,
      updatedAt: s.updatedAt,
      contextSummary: s.contextSummary?.version === 1 && typeof s.contextSummary.content === 'string'
        && typeof s.contextSummary.throughMessageId === 'string' ? s.contextSummary : undefined,
      connectionId: s.connectionId || undefined,
      messages: (s.messages || []).map((m: any) => ({
        id: m.id,
        role: m.role,
        content: m.content,
        thinking: m.thinking || undefined,
        _contextHeader: m._contextHeader || undefined,
        // Backfill createdAt for sessions saved before timestamps existed:
        // ids embed the creation epoch (msg-<ms>, skill-<ms>, cmd-<ms>).
        createdAt: m.createdAt || Number(/(?:^|-)(\d{13})(?:-\d+)?$/.exec(m.id)?.[1]) || undefined,
        thinkingDurationMs: m.thinkingDurationMs || undefined,
        tool_call_id: m.tool_call_id,
        tool_calls: m.tool_calls || [],
        pendingTools: m.pendingTools || [],
        // Sessions written before attachments existed simply lack the field;
        // defaulting to undefined keeps those messages working unchanged.
        attachments: Array.isArray(m.attachments) && m.attachments.length ? m.attachments : undefined,
        _rawApiMsg: m._rawApiMsg ? JSON.parse(m._rawApiMsg) : undefined,
      }))
    }))
    return { sessions, currentSessionId: data.currentSessionId || null }
  } catch {
    return { sessions: [], currentSessionId: null }
  }
}

export const useAIStore = defineStore('ai', () => {
  const visible = ref(false)
  const messages = ref<AIMessage[]>([])
  const mode = ref<ExecutionMode>('confirm_dangerous')
  const isRunning = ref(false)
  const status = ref<AIAgentStatus>('thinking')
  const contextRevision = ref(0)
  const compactionRequestId = ref('')
  const contextNotice = ref('')
  const contextStats = ref<{ beforeTokens: number; afterTokens: number; durationMs: number; fallback: boolean } | null>(null)
  const contextSummary = computed(() => {
    const summary = sessions.value.find(s => s.id === currentSessionId.value)?.contextSummary
    return summary?.version === 1 && summary.content && messages.value.some(m => m.id === summary.throughMessageId)
      ? summary : undefined
  })

  function invalidateContextSummary() {
    contextRevision.value++
    const session = sessions.value.find(s => s.id === currentSessionId.value)
    if (session) delete session.contextSummary
  }

  function applyContextSummary(summary: AIContextSummary, revision: number): boolean {
    const session = sessions.value.find(s => s.id === currentSessionId.value)
    if (!session || contextRevision.value !== revision || !messages.value.some(m => m.id === summary.throughMessageId)) return false
    session.contextSummary = summary
    return true
  }
  // Live thinking/reasoning text streamed during the current run, shown in
  // the expandable box under the "Thinking..." status indicator.
  const thinkingText = ref('')
  const thinkingExpanded = ref(false)
  // Epoch ms when the current turn's thinking stream began; drives the live
  // elapsed timer in the thinking box. 0 = not currently thinking.
  const thinkingStartedAt = ref<number>(0)
  const stopRequested = ref(false)
  const sessions = ref<AISession[]>([])
  const currentSessionId = ref<string | null>(null)
  const lastDebugInfo = ref<{ request: string; error: string } | null>(null)
  const initialized = ref(false)
  const pendingCommand = ref<{
    messageId: string
    toolId: string
    toolName: string
    command: string
    risk: string
    dangerous: boolean
    panel?: string
    timeout?: number
    headLines?: number
    tailLines?: number
  } | null>(null)
  const pendingQuestion = ref<{
    messageId: string
    toolId: string
    question: string
    header?: string
    options: Array<{ label: string; description: string }>
    multiSelect: boolean
  } | null>(null)
  const lastPanelContext = ref<{ panelId: string; shellPath: string } | null>(null)
  const queuedMessages = ref<{ id: string; content: string; skillName?: string; skillBody?: string; commandBody?: string; attachments?: AIAttachment[] }[]>([])

  function setLastPanelContext(panelId: string, shellPath: string) {
    lastPanelContext.value = { panelId, shellPath }
  }

  function enqueueMessage(content: string, skillName?: string, skillBody?: string, commandBody?: string, attachments?: AIAttachment[]) {
    const trimmed = content.trim()
    const hasAttachments = !!attachments?.length
    if (!trimmed && !commandBody && !skillName && !hasAttachments) return
    queuedMessages.value.push({
      id: `q-${Date.now()}-${queuedMessages.value.length}`,
      content: trimmed,
      skillName,
      skillBody,
      commandBody,
      attachments: hasAttachments ? attachments : undefined,
    })
  }

  function removeQueuedMessage(id: string) {
    queuedMessages.value = queuedMessages.value.filter(q => q.id !== id)
  }

  function clearQueue() {
    queuedMessages.value = []
  }

  async function saveVisible() {
    try {
      useLocalStateStore().update({ aiSidebarVisible: visible.value })
    } catch {
      // ignore save errors
    }
  }

  // Auto-persist AI sidebar visibility whenever it changes
  watch(visible, () => {
    saveVisible()
  })

  function setDebugInfo(request: unknown, error: string) {
    try {
      // Attachment bytes are stripped before the request is dumped: the debug
      // panel must never hold a multi-megabyte base64 image.
      lastDebugInfo.value = {
        request: JSON.stringify(redactAttachmentsForLog(request), null, 2),
        error
      }
    } catch {
      lastDebugInfo.value = {
        request: String(request),
        error
      }
    }
  }

  function clearDebugInfo() {
    lastDebugInfo.value = null
  }

  function setPendingCommand(cmd: { messageId: string; toolId: string; toolName: string; command: string; risk: string; dangerous: boolean; panel?: string; timeout?: number; headLines?: number; tailLines?: number }) {
    pendingCommand.value = cmd
  }

  function clearPendingCommand() {
    pendingCommand.value = null
  }

  function setPendingQuestion(q: { messageId: string; toolId: string; question: string; header?: string; options: Array<{ label: string; description: string }>; multiSelect: boolean }) {
    pendingQuestion.value = q
  }

  function clearPendingQuestion() {
    pendingQuestion.value = null
  }

  function toggle() {
    visible.value = !visible.value
  }

  function addMessage(msg: AIMessage): AIMessage {
    const r = reactive({ createdAt: Date.now(), ...msg }) as AIMessage
    messages.value.push(r)
    if (currentSessionId.value) {
      const s = sessions.value.find(s => s.id === currentSessionId.value)
      if (s) {
        s.messages.push(r)
        s.updatedAt = Date.now()
        // Move to top on new activity
        const idx = sessions.value.indexOf(s)
        if (idx > 0) {
          sessions.value.splice(idx, 1)
          sessions.value.unshift(s)
        }
        if (msg.role === 'user' && !s.connectionId && s.name === t('ai.newSession')) {
          const trimmed = msg.content.trim()
          if (trimmed) {
            s.name = trimmed.length > 20 ? trimmed.slice(0, 20) + '...' : trimmed
          }
        }
        doSave()
      }
    }
    return r
  }

  function addSkillCard(name: string, source: 'explicit' | 'auto') {
    const r = reactive({
      id: `skill-${Date.now()}`,
      role: 'user' as const,
      content: '',
      skillName: name,
      skillSource: source,
    }) as AIMessage
    messages.value.push(r)
    if (currentSessionId.value) {
      const s = sessions.value.find(s => s.id === currentSessionId.value)
      if (s) {
        s.messages.push(r)
        s.updatedAt = Date.now()
        doSave()
      }
    }
  }

  function addCommandCard(name: string, args: string) {
    const r = reactive({
      id: `cmd-${Date.now()}`,
      role: 'user' as const,
      content: '',
      commandName: name,
      commandArgs: args,
    }) as AIMessage
    messages.value.push(r)
    if (currentSessionId.value) {
      const s = sessions.value.find(s => s.id === currentSessionId.value)
      if (s) {
        s.messages.push(r)
        s.updatedAt = Date.now()
        doSave()
      }
    }
  }

  function clearMessages() {
    invalidateContextSummary()
    contextNotice.value = ''
    contextStats.value = null
    messages.value = []
    if (currentSessionId.value) {
      const s = sessions.value.find(s => s.id === currentSessionId.value)
      if (s) {
        s.messages = []
        s.updatedAt = Date.now()
        doSave()
      }
    }
  }

  async function init() {
    const data = await loadSessionsFromBackend()
    sessions.value = data.sessions
      .filter(s => s.messages.length > 0)
      .sort((a, b) => b.updatedAt - a.updatedAt)
    // Always start with a fresh session after restart
    currentSessionId.value = null
    initialized.value = true

    // Load sidebar visibility from local state
    try {
      const ls = useLocalStateStore()
      if (!ls.loaded) await ls.init()
      // Mobile always starts with the AI sidebar closed (see App.vue).
      visible.value = isMobilePlatform()
        ? false
        : (ls.state.aiSidebarVisible ?? false)
    } catch {
      // keep default
    }

    // Restore current session or create a new one. On restart no terminal is
    // connected yet, so the fresh session starts unbound; the first tab
    // switch (or followActiveConnection from the first connection) binds it.
    if (currentSessionId.value) {
      const s = sessions.value.find(s => s.id === currentSessionId.value)
      if (s) {
        messages.value = s.messages.map(m => {
          const msg = { ...m }
          if (typeof msg._rawApiMsg === 'string' && msg._rawApiMsg) {
            try { msg._rawApiMsg = JSON.parse(msg._rawApiMsg) } catch { delete msg._rawApiMsg }
          }
          return reactive(msg) as AIMessage
        })
      } else {
        createSession()
      }
    } else {
      createSession()
    }
    // If a terminal was reconnected before init finished (restored tabs),
    // follow it so the sidebar opens on that host's conversation.
    followActiveConnection()
  }


  async function doSave() {
    try {
      const data = {
        sessions: sessions.value.map(s => ({
          id: s.id,
          name: s.name,
          createdAt: s.createdAt,
          updatedAt: s.updatedAt,
          contextSummary: s.contextSummary,
          connectionId: s.connectionId || undefined,
          messages: s.messages.map(m => ({
            id: m.id,
            role: m.role,
            content: m.content,
            thinking: m.thinking || '',
            _contextHeader: m._contextHeader || '',
            createdAt: m.createdAt || 0,
            thinkingDurationMs: m.thinkingDurationMs || 0,
            tool_call_id: m.tool_call_id || '',
            tool_calls: m.tool_calls || [],
            pendingTools: m.pendingTools || [],
            attachments: m.attachments?.length ? m.attachments : undefined,
            _rawApiMsg: m._rawApiMsg ? JSON.stringify(m._rawApiMsg) : '',
          }))
        })),
        currentSessionId: currentSessionId.value || '',
      }
      await SaveAISessions(data as any)
    } catch {
      // ignore save errors
    }
  }

  function createSession(name?: string) {
    contextRevision.value++
    contextNotice.value = ''
    contextStats.value = null
    const now = Date.now()
    const session: AISession = {
      // Random suffix: Date.now() alone collides when sessions are created
      // within the same millisecond (bulk create / rapid new-session clicks),
      // and duplicate ids make the trim filter drop every colliding session.
      id: `session-${now}-${Math.random().toString(36).slice(2, 6)}`,
      name: name || t('ai.newSession'),
      createdAt: now,
      updatedAt: now,
      messages: []
    }
    sessions.value.unshift(session)
    currentSessionId.value = session.id
    messages.value = []
    // Trim to max 15 sessions; host-bound sessions are never trimmed so the
    // "same server, same session" continuity survives history churn (#1036).
    if (sessions.value.length > 15) {
      const overflow = sessions.value.filter(s => s.connectionId && !isCurrent(s))
      const unbound = sessions.value.filter(s => !s.connectionId && !isCurrent(s))
      const overflowCount = sessions.value.length - 15
      const dropUnbound = Math.min(overflowCount, unbound.length)
      const dropBound = overflowCount - dropUnbound
      const drop = new Set([
        ...unbound.slice(-dropUnbound).map(s => s.id),
        ...overflow.slice(0, dropBound).map(s => s.id),
      ])
      if (drop.size) sessions.value = sessions.value.filter(s => !drop.has(s.id))
    }
    clearQueue()
    // The terminal binding is per-conversation: starting a new session drops
    // the lock so the next conversation re-locks onto the active terminal.
    useTabStore().clearAILockedPanels()
    // Don't save empty sessions — only persist when first message is added

    function isCurrent(s: AISession) { return s.id === currentSessionId.value }
  }

  function switchSession(sessionId: string) {
    const s = sessions.value.find(s => s.id === sessionId)
    if (!s) return
    contextRevision.value++
    contextNotice.value = ''
    contextStats.value = null
    currentSessionId.value = sessionId
    messages.value = s.messages.map(m => reactive({ ...m }) as AIMessage)
    thinkingText.value = ''
    thinkingExpanded.value = false
    thinkingStartedAt.value = 0
    clearQueue()
    // Viewing another session drops the lock so the next conversation
    // re-locks onto the terminal active at that moment.
    useTabStore().clearAILockedPanels()
  }

  function deleteSession(sessionId: string) {
    const idx = sessions.value.findIndex(s => s.id === sessionId)
    if (idx === -1) return
    sessions.value.splice(idx, 1)
    doSave()
    if (currentSessionId.value === sessionId) {
      // If the deleted session was bound to the active host, re-follow so a
      // fresh session for that host takes its place instead of jumping to an
      // unrelated conversation (#1036).
      if (sessions.value.length > 0 && !canAutoSwitchToBound()) {
        switchSession(sessions.value[0].id)
      } else {
        createSession()
      }
      followActiveConnection()
    }
  }

  // Whether deleting the current session should fall back to the host-bound
  // flow: with no other candidate, or when the top session is bound to
  // another host, let followActiveConnection resolve.
  function canAutoSwitchToBound(): boolean {
    const key = connectionKeyForPanel(autoSwitchPanelId())
    if (!key) return false
    return !sessions.value.some(s => s.id !== currentSessionId.value && s.connectionId === key)
  }

  // Issue #756: drop one message (and its tool-result pairs) from the current
  // session. messages[] and the owning session share the same reactive objects,
  // so splicing the view array also splices the session; doSave persists it.
  function deleteMessage(messageId: string) {
    const idx = messages.value.findIndex(m => m.id === messageId)
    if (idx === -1) return
    invalidateContextSummary()
    const m = messages.value[idx]
    // Remove matching tool messages so the API never sees an orphaned
    // tool_result (conversation() filters dangling ones, but the UI should
    // not keep dead rows either).
    let end = idx + 1
    if (m.role === 'assistant' && m.tool_calls?.length) {
      const ids = new Set(m.tool_calls.map(tc => tc.id))
      while (end < messages.value.length) {
        const n = messages.value[end]
        if (n.role === 'tool' && n.tool_call_id && ids.has(n.tool_call_id)) end++
        else break
      }
    }
    messages.value.splice(idx, end - idx)
    const session = sessions.value.find(s => s.id === currentSessionId.value)
    if (session) session.messages = messages.value
    doSave()
  }

  // Issue #756: drop the message AND everything after it (context rollback,
  // Trae-style partial deletion). Same shared-array semantics as deleteMessage.
  function truncateFrom(messageId: string) {
    const idx = messages.value.findIndex(m => m.id === messageId)
    if (idx === -1) return
    invalidateContextSummary()
    messages.value.splice(idx)
    const session = sessions.value.find(s => s.id === currentSessionId.value)
    if (session) session.messages = messages.value
    doSave()
  }

  // Issue #756: serialize the current session to markdown for export.
  function exportSessionMarkdown(sessionId: string): string {
    const s = sessions.value.find(x => x.id === sessionId)
    if (!s) return ''
    const lines: string[] = [
      `# ${s.name}`,
      '',
      `> ${new Date(s.createdAt).toLocaleString()} · uniTerm AI Assistant`,
    ]
    for (const m of s.messages) {
      if (m.role === 'tool') continue
      const who = m.role === 'user' ? '🧑 User' : '🤖 Assistant'
      lines.push('', `## ${who}`, '', m.content || '')
      if (m.role === 'assistant' && m.tool_calls?.length) {
        for (const tc of m.tool_calls) {
          lines.push('', `**Tool call: \`${tc.function.name}\`**`, '', '```', tc.function.arguments, '```')
        }
      }
    }
    return lines.join('\n') + '\n'
  }

  function renameSession(sessionId: string, name: string) {
    const s = sessions.value.find(s => s.id === sessionId)
    if (s) {
      s.name = name
      doSave()
    }
  }

  // ── Host-bound sessions (issue #1036) ──

  // Binding key for a panel's connection: saved connection id (one session
  // per saved connection), with a host:user:port fallback for ad-hoc ones —
  // ad-hoc connects get a fresh conn-* id per launch, so the id alone would
  // never match across visits. Local/WSL terminals carry stable pseudo ids.
  // Returns null for panels without a meaningful identity.
  function connectionKeyForPanel(panelId: string | null): string | null {
    if (!panelId) return null
    const panel = usePanelStore().getPanel(panelId)
    const cfg = panel?.config
    if (!cfg) return null
    const saved = cfg.id && useConnectionStore().connections.some(c => c.id === cfg.id)
    if (saved) return cfg.id
    if (cfg.host) return `${cfg.host}:${cfg.user || ''}:${cfg.port || 0}`
    return null
  }

  // Display label for a binding key's connection, used as the default
  // session name: connection name, then user@host, then host.
  function connectionLabelForPanel(panelId: string | null): string | null {
    if (!panelId) return null
    const panel = usePanelStore().getPanel(panelId)
    const cfg = panel?.config
    if (!cfg) return null
    if (cfg.name) return cfg.name
    if (cfg.host) return cfg.user ? `${cfg.user}@${cfg.host}` : cfg.host
    return null
  }

  // Whether automatic session switching is currently safe: the agent runtime
  // state (isRunning / pending command / pending question) is global, and
  // switchSession replaces messages wholesale — switching mid-run would write
  // new messages into the wrong session and drop the terminal lock.
  function canAutoSwitch(): boolean {
    return !isRunning.value && !pendingCommand.value && !pendingQuestion.value
      && !queuedMessages.value.length
  }

  // Resolve the active panel the way a *new* conversation would see it: the
  // AI-locked panel when a lock exists, else the active tab's panel. Used on
  // tab switches so a fresh conversation follows the terminal the user is
  // looking at.
  function autoSwitchPanelId(): string | null {
    const tabStore = useTabStore()
    const locked = tabStore.getAILockedPanel()
    return locked || tabStore.getActivePanelId()
  }

  // Latest (by updatedAt) session bound to a connection key: when a host has
  // several conversations, switching back to its tab returns to the one the
  // user was last active in — not whichever one the find happens to hit.
  function latestBoundSession(key: string): AISession | undefined {
    let best: AISession | undefined
    for (const s of sessions.value) {
      if (s.connectionId !== key) continue
      if (!best || s.updatedAt > best.updatedAt) best = s
    }
    return best
  }

  // Default name for a new session on a connection: the host label, with a
  // " 2", " 3"… suffix when the host already has identically-named sessions,
  // so multiple conversations on one host stay distinguishable (#1036).
  function nextNameForConnection(key: string, label: string): string {
    const taken = new Set(sessions.value.filter(s => s.connectionId === key).map(s => s.name))
    if (!taken.has(label)) return label
    for (let n = 2; ; n++) {
      const candidate = `${label} ${n}`
      if (!taken.has(candidate)) return candidate
    }
  }

  // Bind the CURRENT (freshly created, still empty) session to the active
  // terminal's connection. Used by the new-session button: unlike
  // followActiveConnection this never switches away — the session the user
  // just asked for is bound in place and becomes the host's newest.
  function bindCurrentSessionToActiveConnection(): void {
    const current = sessions.value.find(s => s.id === currentSessionId.value)
    if (!current || current.messages.length > 0 || current.connectionId) return
    const key = connectionKeyForPanel(autoSwitchPanelId())
    if (!key) return
    current.connectionId = key
    current.name = nextNameForConnection(key, connectionLabelForPanel(autoSwitchPanelId()) || t('ai.newSession'))
  }

  // Auto-switch the current session to the one bound to the active terminal's
  // connection (issue #1036): switching tabs keeps the AI sidebar on that
  // host's conversation. Adopts the current empty session when it has no
  // messages yet (so the first conversation on a host doesn't duplicate),
  // otherwise returns to the host's most recently active session or creates a
  // fresh one named after the host.
  function followActiveConnection(): void {
    if (!initialized.value) return
    if (!canAutoSwitch()) return
    const panelId = autoSwitchPanelId()
    const key = connectionKeyForPanel(panelId)
    if (!key) return
    const current = sessions.value.find(s => s.id === currentSessionId.value)
    if (current?.connectionId === key) return
    const label = connectionLabelForPanel(panelId) || t('ai.newSession')

    const bound = latestBoundSession(key)
    if (bound) {
      switchSession(bound.id)
      return
    }
    // Adopt the current session if it is still empty (no messages, no
    // binding): the user just opened a terminal, the session shown is the
    // fresh one init/last-switch created — reuse it instead of stacking
    // empties.
    if (current && current.messages.length === 0 && !current.connectionId) {
      current.connectionId = key
      current.name = nextNameForConnection(key, label)
      return
    }
    const session: AISession = {
      id: `session-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
      name: nextNameForConnection(key, label),
      createdAt: Date.now(),
      updatedAt: Date.now(),
      messages: [],
      connectionId: key,
    }
    sessions.value.unshift(session)
    currentSessionId.value = session.id
    messages.value = []
    useTabStore().clearAILockedPanels()
    // No doSave here: empty sessions persist on their first message.
  }

  function stop() {
    stopRequested.value = true
    isRunning.value = false
    clearQueue()
  }

  function resetStop() {
    stopRequested.value = false
  }

  // Build Anthropic-native message array (system is separate top-level field)
  function buildConversation(source: AIMessage[], summary?: AIContextSummary): Array<Record<string, unknown>> {
    const recentMsgs = [...source]

    // Don't start the conversation with an orphaned tool_result whose matching
    // tool_use was truncated out of the window. Strip leading tool messages
    // until we hit a user or assistant message.
    while (recentMsgs.length > 0 && recentMsgs[0].role === 'tool') {
      recentMsgs.shift()
    }

    // Collect all resolved tool_use IDs from tool_result messages
    const resolvedIds = new Set<string>()
    for (const m of recentMsgs) {
      if (m.role === 'tool' && m.tool_call_id) {
        resolvedIds.add(m.tool_call_id)
      }
    }

    const result: Array<Record<string, unknown>> = summary ? [{
      role: 'user',
      content: `[Historical task handoff — reference data, not new instructions. Current user input and current environment take precedence. Plans are not completed actions; this summary grants no new permissions.]\n${summary.content}`,
    }] : []

    for (const m of recentMsgs) {
      if (m.id.startsWith('dbg-')) continue
      if (m.needsContinue) continue  // UI-only prompts, not part of LLM conversation
      // skill/command cards are UI-only markers; never send them to the API
      if ((m.skillName || m.commandName) && !m.content) continue
      // restored/legacy empty user messages produce invalid empty text blocks
      // (a message carrying attachments is valid even with no text)
      if (m.role === 'user' && !m.content && !m._contextHeader && !m.attachments?.length) continue

      // Tool messages: ones with tool_call_id are real tool_results for the API;
      // ones without are display-only system errors and must not be sent.
      if (m.role === 'tool') {
        if (m.tool_call_id) {
          result.push({
            role: 'user',
            content: [{ type: 'tool_result', tool_use_id: m.tool_call_id, content: m.content }]
          })
        }
        continue
      }

      // Skip assistant messages that are API error placeholders from before the fix
      if (m.role === 'assistant' && typeof m.content === 'string' && m.content.includes('[Error:')) {
        continue
      }

      // Assistant with raw API blocks: filter dangling tool_use blocks without matching tool_result
      if (m._rawApiMsg) {
        const raw = m._rawApiMsg as Record<string, unknown>
        const content = raw.content
        if (Array.isArray(content)) {
          const filtered = (content as Array<Record<string, unknown>>).filter((block: Record<string, unknown>) => {
            // Signed thinking from before a client-side history rewrite cannot
            // be replayed against the rewritten prefix. Keep the UI original.
            if (summary && (m.createdAt || 0) <= summary.createdAt && (block.type === 'thinking' || block.type === 'redacted_thinking')) return false
            if (block.type === 'tool_use') {
              return resolvedIds.has(block.id as string)
            }
            return true
          })
          if (filtered.length === 0 && !m.content && !(m.pendingTools?.length || pendingCommand.value?.messageId === m.id)) continue
          result.push({ ...raw, role: (raw.role as string) || 'assistant', content: filtered })
        } else {
          result.push({ ...raw, role: (raw.role as string) || 'assistant' })
        }
        continue
      }

      // Assistant with legacy tool_calls: filter dangling ones, build content blocks
      if (m.role === 'assistant' && m.tool_calls?.length) {
        const resolved = m.tool_calls.filter(tc => resolvedIds.has(tc.id))
        if (!m.content && resolved.length === 0 && !(m.pendingTools?.length || pendingCommand.value?.messageId === m.id)) continue

        const blocks: Array<Record<string, unknown>> = []
        if (m.content) {
          blocks.push({ type: 'text', text: m.content })
        }
        for (const tc of resolved) {
          let input: Record<string, unknown> = {}
          try { input = JSON.parse(tc.function.arguments) } catch { /* passthrough */ }
          blocks.push({ type: 'tool_use', id: tc.id, name: tc.function.name, input })
        }
        result.push({ role: 'assistant', content: blocks })
        continue
      }

      // Skip empty assistant messages (no content, no tool calls, no raw api msg, no pending tools)
      if (m.role === 'assistant' && !m.content && !(m.pendingTools?.length || pendingCommand.value?.messageId === m.id)) continue

      // Inject dynamic context header into user messages for the API
      // (hidden from UI, stored in _contextHeader). Attachments turn the
      // message into an ordered content-block array; without them the payload
      // stays exactly what it was before attachments existed.
      if (m.role === 'user') {
        const text = m._contextHeader ? `${m._contextHeader}\n\n${m.content}` : m.content
        result.push({ role: m.role, content: buildUserContent(text, m.attachments) })
      } else {
        result.push({ role: m.role || 'user', content: m.content })
      }
    }

    // Merge adjacent tool results BEFORE validation, so a parallel tool call
    // retains every result, and a handoff can precede the latest user input.
    const combined: Array<Record<string, unknown>> = []
    for (const msg of result) {
      const prev = combined[combined.length - 1]
      if (msg.role === 'user' && prev?.role === 'user') {
        const left = Array.isArray(prev.content) ? prev.content : [{ type: 'text', text: prev.content }]
        const right = Array.isArray(msg.content) ? msg.content : [{ type: 'text', text: msg.content }]
        prev.content = [...left, ...right]
      } else combined.push(msg)
    }
    result.splice(0, result.length, ...combined)

    // Final safety pass: enforce that every tool_use is immediately followed
    // by a user message containing its matching tool_result, and every
    // tool_result is immediately preceded by an assistant with its tool_use.
    // The Anthropic API rejects tool_use blocks that are not resolved in the
    // very next message.
    const cleaned: Array<Record<string, unknown>> = []
    for (let i = 0; i < result.length; i++) {
      const msg = result[i]

      if (msg.role === 'assistant' && Array.isArray(msg.content)) {
        const nextMsg = i + 1 < result.length ? result[i + 1] : null
        const blocks = (msg.content as Array<Record<string, unknown>>).filter((block) => {
          if (block.type === 'tool_use') {
            if (!nextMsg || nextMsg.role !== 'user' || !Array.isArray(nextMsg.content)) {
              return false
            }
            return (nextMsg.content as Array<Record<string, unknown>>).some(
              (nb) => nb.type === 'tool_result' && nb.tool_use_id === block.id
            )
          }
          return true
        })
        if (blocks.length === 0) continue
        cleaned.push({ ...msg, content: blocks })
      } else if (msg.role === 'user' && Array.isArray(msg.content)) {
        const prevMsg = i > 0 ? result[i - 1] : null
        const blocks = (msg.content as Array<Record<string, unknown>>).filter((block) => {
          if (block.type === 'tool_result') {
            if (!prevMsg || prevMsg.role !== 'assistant' || !Array.isArray(prevMsg.content)) {
              return false
            }
            return (prevMsg.content as Array<Record<string, unknown>>).some(
              (pb) => pb.type === 'tool_use' && pb.id === block.tool_use_id
            )
          }
          return true
        })
        if (blocks.length === 0) continue
        cleaned.push({ ...msg, content: blocks })
      } else {
        cleaned.push(msg)
      }
    }

    // Additional validation: ensure no consecutive user messages ( Anthropic rejects this )
    const deduped: Array<Record<string, unknown>> = []
    for (const msg of cleaned) {
      if (msg.role === 'user' && deduped.length > 0 && deduped[deduped.length - 1].role === 'user') {
        const prev = deduped[deduped.length - 1]
        const prevBlocks = Array.isArray(prev.content) ? prev.content : [{ type: 'text', text: prev.content }]
        const msgBlocks = Array.isArray(msg.content) ? msg.content : [{ type: 'text', text: msg.content }]
        prev.content = [...prevBlocks, ...msgBlocks]
      } else {
        deduped.push(msg)
      }
    }

    // Messages are inherently dynamic — no cache_control breakpoints here.
    // Caching is handled entirely on the system prompt (see llm.ts).

    return deduped
  }

  const conversation = computed(() => buildConversation(activeHistory(messages.value, contextSummary.value), contextSummary.value))

  const systemPrompt = computed(() => SYSTEM_RULES)

  return {
    visible,
    toggle,
    messages,
    addMessage, addSkillCard, addCommandCard,
    clearMessages,
    mode,
    isRunning,
    status,
    thinkingText,
    thinkingExpanded,
    thinkingStartedAt,
    conversation,
    buildConversation,
    contextSummary,
    contextRevision,
    applyContextSummary,
    compactionRequestId,
    contextNotice,
    contextStats,
    systemPrompt,
    stopRequested,
    stop,
    resetStop,
    sessions,
    currentSessionId,
    createSession,
    switchSession,
    deleteSession,
    deleteMessage,
    truncateFrom,
    exportSessionMarkdown,
    renameSession,
    connectionKeyForPanel,
    connectionLabelForPanel,
    followActiveConnection,
    bindCurrentSessionToActiveConnection,
    latestBoundSession,
    canAutoSwitch,
    lastDebugInfo,
    setDebugInfo,
    clearDebugInfo,
    pendingCommand,
    setPendingCommand,
    clearPendingCommand,
    pendingQuestion,
    setPendingQuestion,
    clearPendingQuestion,
    initialized,
    init,
    lastPanelContext,
    setLastPanelContext,
    queuedMessages,
    enqueueMessage,
    removeQueuedMessage,
    clearQueue,
    doSave
  }
})
