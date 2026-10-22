export type ExecutionMode = 'confirm_all' | 'confirm_write' | 'confirm_dangerous' | 'bypass'

export type AIAgentStatus = 'thinking' | 'outputting' | 'executing' | 'confirming' | 'compacting'

export type AIReasoningEffort = 'default' | 'low' | 'medium' | 'high' | 'xhigh' | 'max'

export interface AIReasoningPreference {
  reasoningEffort: AIReasoningEffort
}

export interface AILocalState {
  activeModelId: string
  modelPreferences: Record<string, AIReasoningPreference>
}

export interface ToolCall {
  id: string
  type: 'function'
  function: {
    name: string
    arguments: string
  }
}

export interface ToolResult {
  tool_call_id: string
  role: 'tool'
  content: string
}

export interface PendingTool {
  id: string
  name: string
  arguments: Record<string, unknown>
  dangerous: boolean
}

export type AIAttachmentKind = 'image' | 'text'

/**
 * An image or text-file the user attached to a user message.
 *
 * `data` holds raw base64 for images (no `data:` prefix — that is the shape
 * every upstream protocol wants) and the verbatim file contents for text
 * files. It is persisted with the session so attachments keep working across
 * turns and survive a restart. Never write it to a log.
 */
export interface AIAttachment {
  id: string
  kind: AIAttachmentKind
  name: string
  mime: string
  size: number
  data: string
}

export interface AIMessage {
  id: string
  role: 'user' | 'assistant' | 'tool'
  content: string
  thinking?: string   // reasoning/thinking text streamed by the model (collapsible in UI)
  createdAt?: number           // epoch ms when the message was created
  thinkingDurationMs?: number  // how long the model's thinking lasted (assistant)
  _rawApiMsg?: Record<string, unknown>  // exact message from API, passed back verbatim
  _contextHeader?: string  // dynamic context prepended in API requests but hidden in UI
  attachments?: AIAttachment[]  // user-message attachments forwarded to the model
  tool_calls?: ToolCall[]
  tool_call_id?: string
  pendingTools?: PendingTool[]
  needsContinue?: boolean  // UI-only: max turns reached, prompt user to continue
  skillName?: string       // 显式调用的 skill 名（对话卡片渲染用）
  skillSource?: string     // 'explicit' | 'auto'
  commandName?: string     // 显式调用的 command 名（对话卡片渲染用）
  commandArgs?: string
}

export interface AISession {
  id: string
  name: string
  createdAt: number
  updatedAt: number
  messages: AIMessage[]
  contextSummary?: AIContextSummary
  // Host binding key (issue #1036): the connection this session belongs to —
  // a saved connection id, or a host:user:port fallback for ad-hoc ones.
  // Sessions without it stay unbound (manual switching only).
  connectionId?: string
}

export interface AIContextSummary {
  version: 1
  content: string
  throughMessageId: string
  createdAt: number
}
