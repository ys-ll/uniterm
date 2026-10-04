export type ExecutionMode = 'confirm_all' | 'confirm_write' | 'confirm_dangerous' | 'bypass'

export type AIAgentStatus = 'thinking' | 'outputting' | 'executing' | 'confirming'

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

export interface AIMessage {
  id: string
  role: 'user' | 'assistant' | 'tool'
  content: string
  thinking?: string   // reasoning/thinking text streamed by the model (collapsible in UI)
  createdAt?: number           // epoch ms when the message was created
  thinkingDurationMs?: number  // how long the model's thinking lasted (assistant)
  _rawApiMsg?: Record<string, unknown>  // exact message from API, passed back verbatim
  _contextHeader?: string  // dynamic context prepended in API requests but hidden in UI
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
  /** Terminal tab this conversation belongs to. Tab ids are per app run;
   *  after a restart stale ids mean "detached history", not a live binding. */
  tabId?: string
  /** Display name of the owning tab, captured at creation for history rows. */
  tabName?: string
}
