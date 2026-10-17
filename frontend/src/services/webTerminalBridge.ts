import { Call } from '@wailsio/runtime'
import type { PanelStatus, PanelType } from '../types/workspace'

export interface WebSession {
  id: string
  type: PanelType
  title: string
  status: PanelStatus
  cols: number
  rows: number
  activeClient: string
  resizeCapability: 'pty' | 'display'
  connectionId?: string
  host?: string
  shellPath?: string
  remoteOS?: string
}

export interface WebTerminalSnapshot {
  sessionId: string
  sequence: number
  cols: number
  rows: number
  ansi: string
  state: Record<string, any>
}

export type TerminalPresentation =
  | { kind: 'write'; data: string }
  | { kind: 'clear' | 'reset' }
  | { kind: 'stamp'; timestamp: number }
  | { kind: 'configure'; cursorBlink: boolean; scrollback: number }
export interface SnapshotRequest {
  requestId: string
  sessionId: string
  sequence: number
  session: WebSession
}

// beta.27 bindings.go uses reflect.Type.PkgPath, which is "main" for App.
// Verified against generated GetPlatform ID 3709063056 = FNV("main.App.GetPlatform").
// No generated files or numeric IDs for methods owned by another worktree.
const call = <T>(method: string, ...args: unknown[]): Promise<T> => Call.ByName(`main.App.${method}`, ...args)
export const webTerminalBridge = {
  sessions: () => call<WebSession[]>('GetWebSessions'),
  snapshot: (id: string) => call<WebTerminalSnapshot>('GetWebTerminalSnapshot', id),
  provide: (requestId: string, snapshot: WebTerminalSnapshot) => call<void>('ProvideWebTerminalSnapshot', requestId, snapshot),
  acknowledge: (requestId: string) => call<void>('AcknowledgeWebTerminal', requestId),
  input: (id: string, client: string, data: string, cols: number, rows: number, binary: boolean) =>
    call<void>('WebTerminalInput', id, client, data, cols, rows, binary),
  resize: (id: string, client: string, cols: number, rows: number, active: boolean) =>
    call<void>('ResizeWebTerminal', id, client, cols, rows, active),
  present: (id: string, operation: TerminalPresentation) => call<void>('PublishWebTerminalPresentation', id, operation),
  fileRestored: (id: string) => call<void>('WebZmodemRestored', id),
}
