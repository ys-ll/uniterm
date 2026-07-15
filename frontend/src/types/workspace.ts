export type PanelType = 'ssh' | 'telnet' | 'mosh' | 'sftp' | 'settings' | 'rdp' | 'vnc' | 'spice' | 'local' | 'database' | 'monitor' | 'serial' | 'other'
export type PanelStatus = 'connecting' | 'connected' | 'disconnected' | 'error'

import type { ConnectionConfig } from './session'
export type { ConnectionConfig }

export interface Panel {
  id: string
  tabId: string
  type: PanelType
  sessionId: string | null
  title: string
  status: PanelStatus
  config: ConnectionConfig | null
}

export interface PanelLayout {
  root: LayoutNode
}

export type LayoutNode =
  | { type: 'leaf'; panelId: string }
  | { type: 'split'; direction: 'horizontal' | 'vertical'; children: LayoutNode[]; sizes: number[] }

// ── Tab types ──

export type Tab = TerminalTab | SettingsTab | WorkspaceTab | SFTPTab | RDPTab | VNCTab | SPICETab | DBTab | MongoDBTab | MonitorTab | StartTab

export interface TerminalTab {
  type: 'terminal'
  id: string
  panelId: string
  name: string
  locked?: boolean
}

export interface SettingsTab {
  type: 'settings'
  id: string
  panelId: string
  name: string
  locked?: boolean
}

export interface WorkspaceTab {
  type: 'workspace'
  id: string
  name: string
  panelIds: string[]
  layout: PanelLayout
  activePanelId: string | null
  locked?: boolean
}

export interface SFTPTab {
  type: 'sftp'
  id: string
  panelId: string
  name: string
  locked?: boolean
}

export interface RDPTab {
  type: 'rdp'
  id: string
  panelId: string
  name: string
  locked?: boolean
}

export interface VNCTab {
  type: 'vnc'
  id: string
  panelId: string
  name: string
  locked?: boolean
}

export interface DBTab {
  type: 'database'
  id: string
  panelId: string
  name: string
  locked?: boolean
}

export interface SPICETab {
  type: 'spice'
  id: string
  panelId: string
  name: string
  locked?: boolean
}

export interface MongoDBTab {
  type: 'mongodb'
  id: string
  panelId: string
  name: string
  locked?: boolean
}

export interface MonitorTab {
  type: 'monitor'
  id: string
  panelId: string
  name: string
  locked?: boolean
}

export interface StartTab {
  type: 'start'
  id: string
  name: string
  viewMode: 'home' | 'group'
  groupId?: string
  locked?: boolean
}
