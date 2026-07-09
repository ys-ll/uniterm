import { chat, AVAILABLE_TOOLS, ChatCancelledError } from './llm'
import { executeCommand, startCommand, captureTerminal, collectOutput, sendTerminalKey } from './terminalAgent'
import { useAIStore } from '../stores/aiStore'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { EventsOn } from '../../wailsjs/runtime'
import type { AIMessage } from '../types/ai'

// Global token listener management: only one runAgent instance should receive
// ai:token events at a time. Registering a new listener automatically cancels
// the previous one, preventing duplicate streaming into multiple assistant
// messages when a stop/continue sequence races.
let activeTokenUnsubscribe: (() => void) | null = null
let activeAssistantMsg: AIMessage | null = null

function registerTokenListener(callback: (data: any) => void): () => void {
  activeTokenUnsubscribe?.()
  activeTokenUnsubscribe = EventsOn('ai:token', callback)
  return () => {
    activeTokenUnsubscribe?.()
    activeTokenUnsubscribe = null
    activeAssistantMsg = null
  }
}

function setActiveAssistantMsg(msg: AIMessage | null) {
  activeAssistantMsg = msg
}

function getActivePanel() {
  const tabStore = useTabStore()
  const panelStore = usePanelStore()

  // Check for AI-locked panel first
  const lockedPanelId = tabStore.getAILockedPanel()
  if (lockedPanelId) {
    return panelStore.getPanel(lockedPanelId)
  }

  // Fall back to active panel based on tab type
  const activeTab = tabStore.activeTab
  if (!activeTab) return undefined

  if (activeTab.type === 'terminal' || activeTab.type === 'settings') {
    return panelStore.getPanel(activeTab.panelId)
  }

  if (activeTab.type === 'workspace' && activeTab.activePanelId) {
    return panelStore.getPanel(activeTab.activePanelId)
  }

  return undefined
}

function hasActiveSession(): boolean {
  const panel = getActivePanel()
  return !!panel?.sessionId
}

type RiskLevel = 'read' | 'write' | 'dangerous'

function getRisk(tu: { name: string; input: Record<string, unknown> }): RiskLevel {
  if (tu.name !== 'execute_command') return 'write'
  const risk = tu.input.risk as string | undefined
  if (risk === 'read' || risk === 'write' || risk === 'dangerous') return risk
  return 'write' // conservative fallback
}

function shouldConfirm(risk: RiskLevel): boolean {
  const store = useAIStore()
  switch (store.mode) {
    case 'confirm_all': return true
    case 'confirm_write': return risk !== 'read'
    case 'confirm_dangerous': return risk === 'dangerous'
    case 'bypass': return false
    default: return risk !== 'read'
  }
}

function getShellName(path?: string): string {
  if (!path) return 'Unknown'
  const lower = path.toLowerCase()
  if (lower.includes('pwsh')) return 'PowerShell'
  if (lower.includes('powershell')) return 'Windows PowerShell'
  if (lower.includes('bash')) return 'Bash'
  if (lower.includes('zsh')) return 'Zsh'
  if (lower.includes('fish')) return 'Fish'
  if (lower.includes('cmd')) return 'CMD'
  if (lower.includes('sh')) return 'Sh'
  return path.split(/[\\/]/).pop() || 'Unknown'
}

/**
 * Shell-specific suffix: appended to system rules dynamically (NOT cached).
 * Lightweight enough that it doesn't significantly impact cache efficiency.
 */
function getShellGuidance(shellPath?: string, isWindowsShell?: boolean): string {
  if (isWindowsShell) {
    const isCmd = shellPath?.toLowerCase().includes('cmd')
    return isCmd
      ? '\n\nThe active terminal is Windows CMD. Use CMD syntax (dir, cd, type, wmic, etc.).'
      : '\n\nThe active terminal is Windows PowerShell. Use cmdlets like Get-ChildItem, Set-Location, Get-Content, etc.'
  }
  return '\n\nThe active terminal is a Unix-like shell. Use standard Unix syntax (ls, cat, grep, find, etc.).'
}

/**
 * Build the dynamic context header injected into the latest user message.
 * Carries current shell/terminal state WITHOUT polluting the system prompt.
 */
function buildDynamicContext(): string {
  const store = useAIStore()
  const activePanel = getActivePanel()

  if (!activePanel) return ''

  const parts: string[] = []
  const shellPath = activePanel.config?.shellPath

  const shellName = shellPath
    ? getShellName(shellPath)
    : (activePanel.type === 'ssh' ? 'SSH (Unix-like)' : 'Unknown')

  const isWindowsShell = !!shellPath && (
    shellPath.toLowerCase().includes('powershell') ||
    shellPath.toLowerCase().includes('pwsh') ||
    shellPath.toLowerCase().includes('cmd')
  )

  const syntaxStyle = isWindowsShell
    ? (shellPath!.toLowerCase().includes('cmd')
      ? 'Windows CMD (dir, cd, type, wmic)'
      : 'Windows PowerShell (cmdlets like Get-ChildItem, Set-Location)')
    : 'Unix (ls, cat, grep, df, find)'

  parts.push(`========================================`)
  parts.push(`CURRENT SHELL: ${shellName}`)
  parts.push(`SYNTAX: ${syntaxStyle}`)
  parts.push(`PANEL: ${activePanel.title} (id: ${activePanel.id})`)
  parts.push(`========================================`)

  if (activePanel.type === 'ssh' && activePanel.config) {
    parts.push(`Connected to: ${activePanel.config.user}@${activePanel.config.host}:${activePanel.config.port}`)
  }

  // Detect terminal switch and inject explicit notice
  const lastCtx = store.lastPanelContext
  if (lastCtx && lastCtx.panelId !== activePanel.id) {
    const prev = lastCtx.shellPath ? getShellName(lastCtx.shellPath) : 'another terminal'
    const curr = shellPath ? getShellName(shellPath) : (activePanel.type === 'ssh' ? 'SSH' : 'local terminal')
    parts.push(`\n【TERMINAL SWITCHED】The user has switched from "${prev}" to "${curr}". This is a DIFFERENT terminal environment. Reassess the environment from scratch.`)
  }

  return parts.join('\n')
}

/**
 * Build the system prompt: static rules from store + lightweight shell guidance.
 * Dynamic context (shell banner, switch notices) goes into the user message.
 */
function buildSystemPrompt(): string {
  const store = useAIStore()
  const activePanel = getActivePanel()
  const shellPath = activePanel?.config?.shellPath
  const isWindowsShell = !!shellPath && (
    shellPath.toLowerCase().includes('powershell') ||
    shellPath.toLowerCase().includes('pwsh') ||
    shellPath.toLowerCase().includes('cmd')
  )
  return store.systemPrompt + getShellGuidance(shellPath, isWindowsShell)
}

export async function runAgent(userInput: string) {
  const store = useAIStore()

  if (!hasActiveSession()) {
    if (userInput) {
      store.addMessage({
        id: `msg-${Date.now()}`,
        role: 'user',
        content: userInput
      })
    }
    store.addMessage({
      id: `msg-${Date.now()}`,
      role: 'tool',
      content: '请先在主窗口中打开一个终端会话，这样我才能执行命令。'
    })
    return
  }

  // Auto-reject any pending command from previous turn
  if (userInput && store.pendingCommand) {
    store.addMessage({
      id: `msg-${Date.now()}`,
      role: 'tool',
      content: 'User started a new conversation. Previous command was cancelled.',
      tool_call_id: store.pendingCommand.toolId
    })
    store.clearPendingCommand()
  }

  store.resetStop()
  store.isRunning = true

  // Record current panel context so buildDynamicContext can detect terminal switches
  const activePanel = getActivePanel()
  if (activePanel) {
    store.setLastPanelContext(activePanel.id, activePanel.config?.shellPath || '')
  }

  if (userInput) {
    const dynamicCtx = buildDynamicContext()
    store.addMessage({
      id: `msg-${Date.now()}`,
      role: 'user',
      content: userInput,
      _contextHeader: dynamicCtx || undefined
    })
  }

  // Track active stream listeners for cleanup
  // Track whether streaming already delivered text, to skip onChunk duplication
  let streamedText = ''

  // Register stream event listener (fires from Go backend SSE events)
  const cleanupTokenListener = registerTokenListener((data: any) => {
    if (store.stopRequested) return
    if (activeAssistantMsg && data.text) {
      activeAssistantMsg.content += data.text
      streamedText += data.text
    }
  })

  function cleanupStreamListeners() {
    cleanupTokenListener()
    setActiveAssistantMsg(null)
  }

  let turnCount = 0
  const maxTurns = 20

  while (turnCount < maxTurns) {
    turnCount++

    if (store.stopRequested) {
      store.addMessage({
        id: `msg-${Date.now()}`,
        role: 'tool',
        content: '[INTERRUPTED]'
      })
      store.isRunning = false
      cleanupStreamListeners()
      return
    }

    const assistantMsg = store.addMessage({
      id: `msg-${Date.now()}`,
      role: 'assistant',
      content: ''
    })
    setActiveAssistantMsg(assistantMsg)

    const toolUses: Array<{ id: string; name: string; input: Record<string, unknown> }> = []

    const chatOptions: any = {
      system: buildSystemPrompt(),
      messages: store.conversation,
      tools: AVAILABLE_TOOLS,
      onChunk: (chunk: string) => {
        if (store.stopRequested) return
        if (streamedText) return // already handled by ai:token events
        assistantMsg.content += chunk
      },
      onToolUse: (tu: { id: string; name: string; input: Record<string, unknown> }) => {
        if (store.stopRequested) return
        toolUses.push(tu)
      }
    }
    try {
      await chat(chatOptions)
      // Preserve raw API message blocks for conversation history
      if (chatOptions._rawApiMsg) {
        assistantMsg._rawApiMsg = chatOptions._rawApiMsg
      }
      // Save immediately so the complete assistant content and raw API message are persisted
      store.doSave()
    } catch (e: any) {
      const errMsg = e.message ?? String(e)
      // Convert the failed assistant placeholder to a display-only tool message.
      // This keeps the error visible in the UI without polluting the API conversation.
      assistantMsg.role = 'tool'
      if (e instanceof ChatCancelledError || store.stopRequested) {
        assistantMsg.content = '[INTERRUPTED]'
      } else {
        assistantMsg.content = `[Error: ${errMsg}]`
      }
      delete assistantMsg._rawApiMsg
      delete assistantMsg.tool_calls
      store.setDebugInfo(store.conversation, errMsg)
      store.isRunning = false
      cleanupStreamListeners()
      return
    }

    if (store.stopRequested) {
      // Cancel any tool calls that were received but not executed
      const cancelledIds = new Set<string>()
      const rawContent = assistantMsg._rawApiMsg?.content
      if (Array.isArray(rawContent)) {
        for (const block of rawContent) {
          if (block.type === 'tool_use' && !cancelledIds.has(block.id)) {
            store.addMessage({
              id: `msg-${Date.now()}`,
              role: 'tool',
              content: 'Command was cancelled by user (Stop).',
              tool_call_id: block.id
            })
            cancelledIds.add(block.id)
          }
        }
      }
      // If no tool calls were pending, the user stopped during text generation.
      // Replace the partial assistant message with a clean stop notice.
      if (cancelledIds.size === 0) {
        assistantMsg.role = 'tool'
        assistantMsg.content = '[INTERRUPTED]'
        delete assistantMsg._rawApiMsg
        delete assistantMsg.tool_calls
      }
      store.isRunning = false
      cleanupStreamListeners()
      return
    }

    // Enforce single tool call
    if (toolUses.length > 1) {
      toolUses.splice(1)
    }

    // Store tool calls in the message for UI confirmation
    if (toolUses.length > 0) {
      assistantMsg.tool_calls = toolUses.map(tu => ({
        id: tu.id,
        type: 'function' as const,
        function: {
          name: tu.name,
          arguments: JSON.stringify(tu.input)
        }
      }))
    }

    if (!assistantMsg.content && toolUses.length === 0) {
      assistantMsg.content = '[No response received from the model. Check your API settings and network connection.]'
      store.isRunning = false
      cleanupStreamListeners()
      return
    }

    if (toolUses.length === 0) {
      store.isRunning = false
      cleanupStreamListeners()
      return
    }

    // Process exactly one tool call
    const tu = toolUses[0]
    if (tu.name === 'execute_command') {
      const command = tu.input.command as string
      const timeoutSec = (tu.input.timeout as number) || 60
      const timeoutMs = Math.max(5000, Math.min(timeoutSec * 1000, 300000))
      const headLines = (tu.input.head_lines as number) ?? 50
      const tailLines = (tu.input.tail_lines as number) ?? 300
      const risk = getRisk(tu)

      if (shouldConfirm(risk)) {
        store.setPendingCommand({
          messageId: assistantMsg.id,
          toolId: tu.id,
          command,
          risk,
          dangerous: risk === 'dangerous'
        })
        assistantMsg.tool_calls = [{
          id: tu.id,
          type: 'function' as const,
          function: {
            name: tu.name,
            arguments: JSON.stringify(tu.input)
          }
        }]
        store.isRunning = false
        cleanupStreamListeners()
        return
      }

      try {
        const result = await executeCommand(command, timeoutMs, headLines, tailLines, () => store.stopRequested)
        if (result.cancelled || store.stopRequested) {
          store.addMessage({
            id: `msg-${Date.now()}`,
            role: 'tool',
            content: '[INTERRUPTED]'
          })
          store.isRunning = false
          cleanupStreamListeners()
          return
        }
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: result.output,
          tool_call_id: tu.id
        })
      } catch (e: any) {
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: `[Error executing command: ${e.message ?? e}]`,
          tool_call_id: tu.id
        })
      }
    } else if (tu.name === 'start_command') {
      const command = tu.input.command as string
      try {
        const result = await startCommand(command)
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: result.output || '(command started)',
          tool_call_id: tu.id
        })
      } catch (e: any) {
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: `[Error starting command: ${e.message ?? e}]`,
          tool_call_id: tu.id
        })
      }
    } else if (tu.name === 'capture_terminal') {
      const tailLines = (tu.input.tail_lines as number) ?? 200
      try {
        const result = captureTerminal(tailLines)
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: result.output || '(terminal is empty)',
          tool_call_id: tu.id
        })
      } catch (e: any) {
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: `[Error capturing terminal: ${e.message ?? e}]`,
          tool_call_id: tu.id
        })
      }
    } else if (tu.name === 'collect_output') {
      const timeoutSec = (tu.input.timeout as number) || 30
      const timeoutMs = Math.max(5000, Math.min(timeoutSec * 1000, 120000))
      const headLines = (tu.input.head_lines as number) ?? 100
      const tailLines = (tu.input.tail_lines as number) ?? 300
      try {
        const result = await collectOutput(timeoutMs, headLines, tailLines)
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: result.output,
          tool_call_id: tu.id
        })
      } catch (e: any) {
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: `[Error collecting output: ${e.message ?? e}]`,
          tool_call_id: tu.id
        })
      }
    } else if (tu.name === 'send_terminal_key') {
      const input = tu.input.input as string | undefined
      const control = tu.input.control as string | undefined
      const sendEnter = (tu.input.send_enter as boolean) ?? true
      try {
        const result = await sendTerminalKey(
          input,
          control as 'ctrl_c' | 'ctrl_d' | 'enter' | undefined,
          sendEnter
        )
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: result.output || '(input sent)',
          tool_call_id: tu.id
        })
      } catch (e: any) {
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: `[Error sending terminal input: ${e.message ?? e}]`,
          tool_call_id: tu.id
        })
      }
    } else if (tu.name === 'interrupt_command') {
      try {
        const result = await sendTerminalKey(undefined, 'ctrl_c')
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: result.output || 'Sent Ctrl+C to interrupt the running command.',
          tool_call_id: tu.id
        })
      } catch (e: any) {
        store.addMessage({
          id: `msg-${Date.now()}`,
          role: 'tool',
          content: `[Error sending Ctrl+C: ${e.message ?? e}]`,
          tool_call_id: tu.id
        })
      }
    }
  }

  // Max turns reached - prompt user to continue
  if (turnCount >= maxTurns) {
    store.addMessage({
      id: `msg-${Date.now()}`,
      role: 'assistant',
      content: `已达到最大对话轮次限制（${maxTurns}轮）。点击"继续"或发送任意消息以继续。`,
      needsContinue: true
    })
  }

  cleanupStreamListeners()
  store.isRunning = false
  store.doSave()
}

export async function continueAgent() {
  const store = useAIStore()
  const lastMsg = store.messages[store.messages.length - 1]
  if (lastMsg?.needsContinue) {
    store.messages.pop()
  }
  await runAgent('')
}

export async function approveTool(_messageId: string) {
  const store = useAIStore()
  const cmd = store.pendingCommand
  if (!cmd) return

  if (!hasActiveSession()) {
    store.clearPendingCommand()
    store.addMessage({
      id: `msg-${Date.now()}`,
      role: 'tool',
      content: '请先打开一个终端会话，再执行此命令。'
    })
    return
  }

  store.clearPendingCommand()
  store.isRunning = true


  try {
    const result = await executeCommand(cmd.command)
    store.addMessage({
      id: `msg-${Date.now()}`,
      role: 'tool',
      content: result.output,
      tool_call_id: cmd.toolId
    })
  } catch (e: any) {
    store.addMessage({
      id: `msg-${Date.now()}`,
      role: 'tool',
      content: `[Error executing command: ${e.message ?? e}]`,
      tool_call_id: cmd.toolId
    })
  }
  await runAgent('')
}

export function rejectTool(_messageId: string) {
  const store = useAIStore()
  const cmd = store.pendingCommand
  if (!cmd) return

  store.clearPendingCommand()

  store.addMessage({
    id: `msg-${Date.now()}`,
    role: 'tool',
    content: 'User rejected this command.',
    tool_call_id: cmd.toolId
  })

  setTimeout(() => runAgent(''), 0)
}
