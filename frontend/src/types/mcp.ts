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

// Mirrors Go store.MCPSettings — lives in local_state.json (per device),
// never in synced settings.
export interface MCPSettings {
  enabled: boolean
  allowRemote?: boolean
  port?: number
  policy?: string
  execMode?: string
  maxConcurrentCommands?: number
}

export const DEFAULT_MCP_SETTINGS: MCPSettings = {
  enabled: false,
  allowRemote: false,
  policy: 'confirm_write',
  execMode: 'background',
  maxConcurrentCommands: 8,
}
