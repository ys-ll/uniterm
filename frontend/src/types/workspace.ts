export type PanelType = 'ssh' | 'telnet' | 'mosh' | 'sftp' | 'settings' | 'rdp' | 'vnc' | 'spice' | 'local' | 'wsl' | 'database' | 'monitor' | 'serial' | 'tcp' | 'k8s' | 'container' | 'container-exec' | 'x11-desktop' | 'netscan' | 'tools' | 'other'
export type PanelStatus = 'connecting' | 'connected' | 'disconnected' | 'error'

import type { ConnectionConfig } from './session'
export type { ConnectionConfig }

import type { K8sTab } from './k8s'
export type { K8sTab }

import type { ContainerTab } from './container'
export type { ContainerTab }

export interface Panel {
  id: string
  tabId: string
  type: PanelType
  sessionId: string | null
  title: string
  status: PanelStatus
  config: ConnectionConfig | null
  outputLog?: { enabled: boolean; path: string }
  // Initial remote directory the file tab should open at (terminal cwd via
  // the "open SFTP" context menu, or the sidebar's current directory via
  // "open standalone tab"). Consumed once by the tab content on first load.
  initialPath?: string
}

export interface PanelLayout {
  root: LayoutNode
}

export type LayoutNode =
  | { type: 'leaf'; panelId: string }
  | { type: 'split'; direction: 'horizontal' | 'vertical'; children: LayoutNode[]; sizes: number[] }

// ── Tab types ──

export type Tab = TerminalTab | SettingsTab | WorkspaceTab | SFTPTab | RDPTab | VNCTab | SPICETab | DBTab | MonitorTab | StartTab | K8sTab | ContainerTab | X11DesktopTab | NetscanTab | ToolsTab

export interface ToolsTab {
  type: 'tools'
  id: string
  panelId: string
  name: string
  locked?: boolean
}

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
  maximizedPanelId?: string | null
  locked?: boolean
  // Connection id of the saved-workspace record this tab was saved to / opened
  // from. Set after the first save so the next save updates in place.
  savedWorkspaceId?: string
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
  type: 'database' | 'mongodb' | 'redis' | 'elasticsearch' | 'opensearch'
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

export interface X11DesktopTab {
  type: 'x11-desktop'
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

export interface NetscanTab {
  type: 'netscan'
  id: string
  panelId: string
  name: string
  locked?: boolean
}
