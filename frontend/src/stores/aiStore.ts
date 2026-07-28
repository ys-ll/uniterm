import { defineStore } from 'pinia'
import { ref, computed, reactive, shallowReactive, shallowRef, watch, markRaw } from 'vue'
import type { AIMessage, AIConfig, ExecutionMode, AISession, AIAgentStatus } from '../types/ai'
import { SaveAIConfig, LoadAIConfig, SaveAISessions, LoadAISessions } from '../../wailsjs/go/main/App'
import { useLocalStateStore } from './localStateStore'
import { EventsOn } from '../../wailsjs/runtime'
import { t } from '../i18n'

// Module-level un-subscriber for the cross-window store:settings:changed listener.
// Tracked at module scope so re-imports under HMR can detach the previous
// listener before re-subscribing (FE-03).
let unsubSettingsChanged: (() => void) | null = null

/**
 * Estimate token count for a string using character-based heuristics.
 * ASCII/English: ~3.5 chars per token. CJK/non-ASCII: ~1.8 chars per token.
 * Accurate to within ~15% for typical mixed content.
 */
function estimateTokens(text: string): number {
  let asciiChars = 0
  let nonAsciiChars = 0
  for (let i = 0; i < text.length; i++) {
    if (text.charCodeAt(i) <= 0x7f) {
      asciiChars++
    } else {
      nonAsciiChars++
    }
  }
  return Math.ceil(asciiChars / 3.5 + nonAsciiChars / 1.8)
}

// F-310: cache token estimates per message in a WeakMap so the per-token
// `conversation` re-evaluation (shouldn't happen post-F-301, but guards
// against any remaining hot reads) doesn't re-stringify _rawApiMsg.
const tokenEstimateCache = new WeakMap<AIMessage, number>()

// F-314: cache the serialized _rawApiMsg JSON per message so doSave doesn't
// re-stringify on every save. The cache is keyed by the AIMessage object and
// stores the object reference alongside the JSON so agent.ts can safely
// replace _rawApiMsg (it assigns a new object) without invalidating reads.
const rawApiMsgJsonCache = new WeakMap<AIMessage, { obj: object; json: string }>()

function getRawApiMsgJson(msg: AIMessage): string {
  if (!msg._rawApiMsg) return ''
  // F-316: inactive sessions retain _rawApiMsg as a JSON string. The disk
  // form is already the JSON we want to persist; double-stringifying would
  // produce a quoted string. Use it verbatim.
  if (typeof msg._rawApiMsg === 'string') return msg._rawApiMsg
  const cached = rawApiMsgJsonCache.get(msg)
  if (cached && cached.obj === msg._rawApiMsg) return cached.json
  const json = JSON.stringify(msg._rawApiMsg)
  rawApiMsgJsonCache.set(msg, { obj: msg._rawApiMsg, json })
  return json
}

/**
 * Estimate tokens for an AIMessage, including content, tool_calls, and
 * serialized _rawApiMsg. Cached on the message itself after first compute.
 */
function estimateMessageTokens(msg: AIMessage): number {
  const cached = tokenEstimateCache.get(msg)
  if (cached !== undefined) return cached
  let total = estimateTokens(msg.content)
  if (msg.tool_calls) {
    for (const tc of msg.tool_calls) {
      total += estimateTokens(tc.function.name)
      total += estimateTokens(tc.function.arguments)
    }
  }
  if (msg._rawApiMsg) {
    total += estimateTokens(getRawApiMsgJson(msg))
  }
  tokenEstimateCache.set(msg, total)
  return total
}

/**
 * Static AI system rules — immutable per app version, always cacheable.
 * Dynamic shell/panel context is injected into the latest user message instead.
 */
const SYSTEM_RULES = `You are an AI assistant inside uniTerm, a terminal emulator. You can execute shell commands in the user's active terminal to help them complete tasks.

AVAILABLE TOOLS:
1. execute_command — Run a shell command and wait for its output. Set timeout based on expected duration. Use head_lines/tail_lines to control how much output you receive.
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

TIMEOUT GUIDELINES:
- 5-10s: quick commands (ls, cat, pwd, whoami)
- 15-30s: moderate commands (grep, find, df, systemctl status)
- 60-120s: build/install tasks (npm install, pip install, apt-get)
- 120-300s: very long tasks (docker build, large git clone, full compilation)

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

const DEFAULT_CONFIG: AIConfig = {
  apiKey: '',
  baseURL: 'https://api.openai.com/v1',
  model: 'gpt-4o'
}

async function loadSessionsFromBackend(): Promise<{ sessions: AISession[], currentSessionId: string | null }> {
  try {
    const data = await LoadAISessions() as any
    // F-316: keep _rawApiMsg as a JSON string in inactive sessions. Parse
    // only when the session is opened (in init / switchSession). Inactive
    // sessions retain the raw string form; the conversation computed refs
    // parsed objects only for the active session.
    const sessions: AISession[] = (data.sessions || []).map((s: any) => ({
      id: s.id,
      name: s.name,
      createdAt: s.createdAt,
      updatedAt: s.updatedAt,
      messages: (s.messages || []).map((m: any) => ({
        id: m.id,
        role: m.role,
        content: m.content,
        tool_call_id: m.tool_call_id,
        tool_calls: m.tool_calls || [],
        pendingTools: m.pendingTools || [],
        _rawApiMsg: m._rawApiMsg || undefined,
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
  const config = ref<AIConfig>({ ...DEFAULT_CONFIG })
  const isRunning = ref(false)
  const status = ref<AIAgentStatus>('thinking')
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
  const queuedMessages = ref<{ id: string; content: string; skillName?: string; skillBody?: string; commandBody?: string }[]>([])

  function setLastPanelContext(panelId: string, shellPath: string) {
    lastPanelContext.value = { panelId, shellPath }
  }

  function enqueueMessage(content: string, skillName?: string, skillBody?: string, commandBody?: string) {
    const trimmed = content.trim()
    if (!trimmed && !commandBody && !skillName) return
    queuedMessages.value.push({
      id: `q-${Date.now()}-${queuedMessages.value.length}`,
      content: trimmed,
      skillName,
      skillBody,
      commandBody,
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
      lastDebugInfo.value = {
        request: JSON.stringify(request, null, 2),
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

  function setPendingCommand(cmd: { messageId: string; toolId: string; command: string; risk: string; dangerous: boolean }) {
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
    // F-302: shallowReactive + markRaw _rawApiMsg. The raw API block is only
    // mutated by the LLM, never read by UI components, so it doesn't need
    // deep reactive tracking. The shallow wrapper covers the message's
    // own fields (content, pendingTools) without descending into nested arrays.
    const wrapped: AIMessage = msg._rawApiMsg
      ? (shallowReactive({ ...msg, _rawApiMsg: markRaw(msg._rawApiMsg) }) as AIMessage)
      : (shallowReactive({ ...msg }) as AIMessage)
    messages.value.push(wrapped)
    if (currentSessionId.value) {
      const s = sessions.value.find(s => s.id === currentSessionId.value)
      if (s) {
        s.messages.push(wrapped)
        s.updatedAt = Date.now()
        if (msg.role === 'user' && s.name === t('ai.newSession')) {
          const trimmed = msg.content.trim()
          if (trimmed) {
            s.name = trimmed.length > 20 ? trimmed.slice(0, 20) + '...' : trimmed
          }
        }
        debouncedSave()
      }
    }
    messagesVersion.value++
    return wrapped
  }

  function addSkillCard(name: string, source: 'explicit' | 'auto') {
    const r = shallowReactive({
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
        debouncedSave()
      }
    }
    messagesVersion.value++
  }

  function addCommandCard(name: string, args: string) {
    const r = shallowReactive({
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
        debouncedSave()
      }
    }
    messagesVersion.value++
  }

  function clearMessages() {
    messages.value = []
    if (currentSessionId.value) {
      const s = sessions.value.find(s => s.id === currentSessionId.value)
      if (s) {
        s.messages = []
        s.updatedAt = Date.now()
        debouncedSave()
      }
    }
    messagesVersion.value++
  }

  async function init() {
    await initConfig()
    const data = await loadSessionsFromBackend()
    sessions.value = data.sessions.filter(s => s.messages.length > 0)
    // Always start with a fresh session after restart
    currentSessionId.value = null
    initialized.value = true

    // Load sidebar visibility from local state
    try {
      const ls = useLocalStateStore()
      if (!ls.loaded) await ls.init()
      visible.value = ls.state.aiSidebarVisible ?? false
    } catch {
      // keep default
    }

    // Restore current session or create a new one
    if (currentSessionId.value) {
      const s = sessions.value.find(s => s.id === currentSessionId.value)
      if (s) {
        // F-316: parse _rawApiMsg in place so messages.value and s.messages
        // share the same backing object — agent.ts mutations to the active
        // message propagate to the stored session and survive a save.
        for (const m of s.messages) {
          if (typeof m._rawApiMsg === 'string' && m._rawApiMsg) {
            try {
              m._rawApiMsg = JSON.parse(m._rawApiMsg)
            } catch {
              delete m._rawApiMsg
            }
          }
          if (m._rawApiMsg) {
            m._rawApiMsg = markRaw(m._rawApiMsg)
          }
        }
        messages.value = s.messages.map(m => shallowReactive(m) as AIMessage)
      } else {
        createSession()
      }
    } else {
      createSession()
    }
    messagesVersion.value++
  }

  async function initConfig() {
    try {
      const loaded = await LoadAIConfig()
      if (loaded.apiKey || loaded.baseURL || loaded.model) {
        config.value = {
          apiKey: loaded.apiKey || DEFAULT_CONFIG.apiKey,
          baseURL: loaded.baseURL || DEFAULT_CONFIG.baseURL,
          model: loaded.model || DEFAULT_CONFIG.model,
        }
      }
    } catch {
      // ignore, use defaults
    }
  }

  async function saveConfig() {
    try {
      await SaveAIConfig({
        apiKey: config.value.apiKey,
        baseURL: config.value.baseURL,
        model: config.value.model,
      })
    } catch {
      // ignore save errors
    }
  }

  function setConfig(updates: Partial<AIConfig>) {
    config.value = { ...config.value, ...updates }
  }

  // F-314: serialize a session to the JSON shape persisted to disk.
  // The session structure is rebuilt each save (cheap), but the heavy
  // JSON.stringify of _rawApiMsg is cached per-message via getRawApiMsgJson.
  function serializeSession(s: AISession): Record<string, unknown> {
    return {
      id: s.id,
      name: s.name,
      createdAt: s.createdAt,
      updatedAt: s.updatedAt,
      messages: s.messages.map(m => ({
        id: m.id,
        role: m.role,
        content: m.content,
        tool_call_id: m.tool_call_id || '',
        tool_calls: m.tool_calls || [],
        pendingTools: m.pendingTools || [],
        _rawApiMsg: getRawApiMsgJson(m),
      }))
    }
  }

  async function doSave() {
    try {
      const data = {
        sessions: sessions.value.map(s => serializeSession(s)),
        currentSessionId: currentSessionId.value || '',
      }
      await SaveAISessions(data as any)
    } catch {
      // ignore save errors
    }
  }

  // F-304: debounce doSave by 500ms to coalesce bursts from addMessage and
  // multi-token streaming. saveNow() flushes immediately for explicit user
  // actions (deleteSession, renameSession, after-chat completion).
  let saveTimer: ReturnType<typeof setTimeout> | null = null
  function debouncedSave() {
    if (saveTimer) return
    saveTimer = setTimeout(() => {
      saveTimer = null
      doSave()
    }, 500)
  }
  async function saveNow() {
    if (saveTimer) {
      clearTimeout(saveTimer)
      saveTimer = null
    }
    await doSave()
  }

  function createSession(name?: string) {
    const now = Date.now()
    const session: AISession = {
      id: `session-${now}`,
      name: name || t('ai.newSession'),
      createdAt: now,
      updatedAt: now,
      messages: []
    }
    sessions.value.unshift(session)
    currentSessionId.value = session.id
    messages.value = []
    // Trim to max 15 sessions
    if (sessions.value.length > 15) {
      sessions.value = sessions.value.slice(0, 15)
    }
    clearQueue()
    messagesVersion.value++
    // Don't save empty sessions — only persist when first message is added
  }

  function switchSession(sessionId: string) {
    const s = sessions.value.find(s => s.id === sessionId)
    if (!s) return
    currentSessionId.value = sessionId
    // F-316: parse _rawApiMsg in place so messages.value and s.messages
    // share the same backing object — agent.ts mutations to the active
    // message propagate to the stored session and survive a save.
    for (const m of s.messages) {
      if (typeof m._rawApiMsg === 'string' && m._rawApiMsg) {
        try {
          m._rawApiMsg = JSON.parse(m._rawApiMsg)
        } catch {
          delete m._rawApiMsg
        }
      }
      if (m._rawApiMsg) {
        m._rawApiMsg = markRaw(m._rawApiMsg)
      }
    }
    messages.value = s.messages.map(m => shallowReactive(m) as AIMessage)
    clearQueue()
    messagesVersion.value++
  }

  function deleteSession(sessionId: string) {
    const idx = sessions.value.findIndex(s => s.id === sessionId)
    if (idx === -1) return
    sessions.value.splice(idx, 1)
    saveNow()
    if (currentSessionId.value === sessionId) {
      if (sessions.value.length > 0) {
        switchSession(sessions.value[0].id)
      } else {
        createSession()
      }
    }
  }

  function renameSession(sessionId: string, name: string) {
    const s = sessions.value.find(s => s.id === sessionId)
    if (s) {
      s.name = name
      saveNow()
    }
  }

  function stop() {
    stopRequested.value = true
    isRunning.value = false
    clearQueue()
  }

  function resetStop() {
    stopRequested.value = false
  }

  // Build Anthropic-native message array (system is separate top-level field).
  // F-301: conversation is rebuilt only when messagesVersion bumps (add/remove),
  // not when per-token content mutates. Stored as a shallowRef; the computed
  // wrapper preserves the existing public API.
  const conversationValue = shallowRef<Array<Record<string, unknown>>>([])

  function buildConversation() {
    // Token budget: 80% of Claude's 200K context window, minus headroom
    const MAX_CONTEXT_TOKENS = 160000

    // Estimate static overhead (cached, counted once)
    // Tools definition is small and static (~1KB); hardcode estimate to avoid
    // a circular dependency on llm.ts
    const systemTokens = estimateTokens(SYSTEM_RULES)
    const toolsTokens = 250  // ~1KB execute_command tool definition
    let tokenCount = systemTokens + toolsTokens

    // Walk backwards through messages, accumulate token estimates.
    // Stop when we exceed the budget.
    const msgs = messages.value
    const n = msgs.length
    const kept: AIMessage[] = []
    const keptStart = (() => {
      let start = n
      for (let i = n - 1; i >= 0; i--) {
        const msgTokens = estimateMessageTokens(msgs[i])
        if (tokenCount + msgTokens > MAX_CONTEXT_TOKENS) break
        tokenCount += msgTokens
        start = i
      }
      // Strip leading tool messages whose matching tool_use was truncated out.
      while (start < n && msgs[start].role === 'tool') start++
      return start
    })()
    for (let i = keptStart; i < n; i++) kept.push(msgs[i])

    // F-313: single forward pass that:
    //   - collects resolved tool_use IDs from tool_result messages,
    //   - filters dangling tool_use blocks (assistant raw / legacy),
    //   - merges consecutive user messages,
    //   - validates pairings (assistant tool_use ↔ user tool_result).
    const resolvedIds = new Set<string>()
    const result: Array<Record<string, unknown>> = []
    const pendingMsgId = pendingCommand.value?.messageId

    const isMessagePending = (m: AIMessage) => !!(m.pendingTools?.length || pendingMsgId === m.id)

    for (let i = 0; i < kept.length; i++) {
      const m = kept[i]
      const pending = isMessagePending(m)

      if (m.id.startsWith('dbg-')) continue
      if (m.needsContinue) continue
      if ((m.skillName || m.commandName) && !m.content) continue
      if (m.role === 'user' && !m.content && !m._contextHeader) continue

      // Tool message: register id, emit as user tool_result wrapper.
      if (m.role === 'tool') {
        if (m.tool_call_id) {
          resolvedIds.add(m.tool_call_id)
          const toolResultBlocks = [{
            type: 'tool_result',
            tool_use_id: m.tool_call_id,
            content: m.content
          }]

          // Pair-validate backward: if previous result is assistant with
          // tool_use blocks, prune any block that doesn't have a matching
          // tool_result in this new message.
          const prev = result[result.length - 1]
          if (prev && prev.role === 'assistant' && Array.isArray(prev.content)) {
            const prevBlocks = prev.content as Array<Record<string, unknown>>
            const filtered = prevBlocks.filter((b) => {
              if (b.type === 'tool_use') {
                return toolResultBlocks.some((tr) => tr.tool_use_id === (b.id as string))
              }
              return true
            })
            if (filtered.length === 0) {
              result.pop()
            } else {
              prev.content = filtered
            }
          }

          // Two consecutive tool messages must each appear as their own
          // user wrapper so they pair with their respective tool_use blocks.
          // Don't merge into the previous user.
          result.push({ role: 'user', content: toolResultBlocks })
        }
        continue
      }

      if (m.role === 'assistant' && typeof m.content === 'string' && m.content.includes('[Error:')) continue

      // Assistant with raw API blocks: filter dangling tool_use blocks.
      if (m._rawApiMsg) {
        const raw = m._rawApiMsg as Record<string, unknown>
        const content = raw.content
        if (Array.isArray(content)) {
          const filtered = (content as Array<Record<string, unknown>>).filter((b) => {
            if (b.type === 'tool_use') return resolvedIds.has(b.id as string)
            return true
          })
          if (filtered.length === 0 && !m.content && !pending) continue
          result.push({ ...raw, role: (raw.role as string) || 'assistant', content: filtered })
        } else {
          result.push({ ...raw, role: (raw.role as string) || 'assistant' })
        }
        continue
      }

      // Assistant with legacy tool_calls.
      if (m.role === 'assistant' && m.tool_calls?.length) {
        const resolved = m.tool_calls.filter(tc => resolvedIds.has(tc.id))
        if (!m.content && resolved.length === 0 && !pending) continue

        const blocks: Array<Record<string, unknown>> = []
        if (m.content) blocks.push({ type: 'text', text: m.content })
        for (const tc of resolved) {
          let input: Record<string, unknown> = {}
          try { input = JSON.parse(tc.function.arguments) } catch { /* passthrough */ }
          blocks.push({ type: 'tool_use', id: tc.id, name: tc.function.name, input })
        }
        result.push({ role: 'assistant', content: blocks })
        continue
      }

      if (m.role === 'assistant' && !m.content && !pending) continue

      // User / plain assistant: dedup consecutive user messages.
      let content: string
      if (m.role === 'user' && m._contextHeader) {
        content = m._contextHeader + '\n\n' + m.content
      } else {
        content = m.content
      }
      const apiMsg = { role: m.role || 'user', content }
      const last = result[result.length - 1]
      if (m.role === 'user' && last && last.role === 'user') {
        const prevBlocks = Array.isArray(last.content)
          ? last.content as Array<Record<string, unknown>>
          : [{ type: 'text', text: last.content as string }]
        const msgBlocks = Array.isArray(content)
          ? content as Array<Record<string, unknown>>
          : [{ type: 'text', text: content as string }]
        last.content = [...prevBlocks, ...msgBlocks]
      } else {
        result.push(apiMsg)
      }
    }

    // Pair validation: prune any tool_use blocks whose next-of-pair msg
    // doesn't carry the matching tool_result. The Anthropic API rejects
    // tool_use blocks not resolved in the very next message, regardless of
    // where in the conversation they appear. Walk backward so we can
    // prune-then-collapse in place.
    for (let i = result.length - 1; i >= 0; i--) {
      const msg = result[i]
      if (msg.role !== 'assistant' || !Array.isArray(msg.content)) continue
      const next = i + 1 < result.length ? result[i + 1] : null
      const nextBlocks = next && next.role === 'user' && Array.isArray(next.content)
        ? next.content as Array<Record<string, unknown>>
        : null
      const blocks = (msg.content as Array<Record<string, unknown>>).filter((b) => {
        if (b.type === 'tool_use') {
          return nextBlocks !== null && nextBlocks.some((nb) => nb.type === 'tool_result' && nb.tool_use_id === b.id)
        }
        return true
      })
      if (blocks.length === 0) {
        result.splice(i, 1)
      } else {
        msg.content = blocks
      }
    }
    // Mirror pass: prune tool_result blocks in user messages whose prev
    // assistant no longer has the matching tool_use (after the previous
    // pass may have removed it). Drop user messages whose content is empty.
    for (let i = 0; i < result.length; i++) {
      const msg = result[i]
      if (msg.role !== 'user' || !Array.isArray(msg.content)) continue
      const prev = i > 0 ? result[i - 1] : null
      const prevBlocks = prev && prev.role === 'assistant' && Array.isArray(prev.content)
        ? prev.content as Array<Record<string, unknown>>
        : null
      const blocks = (msg.content as Array<Record<string, unknown>>).filter((b) => {
        if (b.type === 'tool_result') {
          return prevBlocks !== null && prevBlocks.some((pb) => pb.type === 'tool_use' && pb.id === b.tool_use_id)
        }
        return true
      })
      if (blocks.length === 0) {
        result.splice(i, 1)
        i--
      } else {
        msg.content = blocks
      }
    }

    conversationValue.value = result
  }

  // F-301: rebuild conversation only when messages are added/removed.
  const messagesVersion = ref(0)
  watch(messagesVersion, () => buildConversation())
  buildConversation()

  const conversation = computed(() => conversationValue.value)

  const systemPrompt = computed(() => SYSTEM_RULES)

  // Reload AI config when settings change via sync
  unsubSettingsChanged?.()
  unsubSettingsChanged = EventsOn('store:settings:changed', () => {
    initConfig()
  })

  function dispose() {
    unsubSettingsChanged?.()
    unsubSettingsChanged = null
  }

  return {
    visible,
    toggle,
    messages,
    addMessage, addSkillCard, addCommandCard,
    clearMessages,
    mode,
    config,
    isRunning,
    status,
    saveConfig,
    initConfig,
    setConfig,
    conversation,
    systemPrompt,
    stopRequested,
    stop,
    resetStop,
    sessions,
    currentSessionId,
    createSession,
    switchSession,
    deleteSession,
    renameSession,
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
    doSave,
    dispose
  }
})
