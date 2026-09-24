// MCP (external AI agents) frontend types.

export interface MCPApprovalRequest {
  id: string
  client: string
  connection?: string
  command?: string
  createdAt?: number
}

export interface MCPStatus {
  running: boolean
  port: number
}

export interface MCPTools {
  exec: boolean
  terminal: boolean
  files: boolean
}

export interface MCPSettings {
  enabled: boolean
  port?: number
  policy?: string
  tools: MCPTools
}

export const DEFAULT_MCP_SETTINGS: MCPSettings = {
  enabled: false,
  policy: 'confirm_all',
  tools: { exec: true, terminal: false, files: false },
}
