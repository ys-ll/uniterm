export type SessionStatus = 'connecting' | 'connected' | 'disconnected' | 'error'

export interface ConnectionGroup {
  id: string
  name: string
}

export interface PostLoginExpectStep {
  expect: string
  send: string
  enter: boolean
  timeoutSecond?: number
}

export interface ConnectionConfig {
  id: string
  name: string
  type: 'ssh' | 'telnet' | 'mosh' | 'rdp' | 'vnc' | 'spice' | 'database' | 'local' | 'sftp' | 'monitor' | 'ftp' | 'serial' | 'smb' | 'webdav' | 's3'
  host: string
  port: number
  user: string
  authType: 'password' | 'key' | 'agent'
  password?: string
  keyPath?: string
  groupId?: string
  // RDP-specific
  rdpFixedWidth?: number
  rdpFixedHeight?: number
  rdpSmartSizing?: boolean
  rdpEnableNLA?: boolean
  // Local terminal shell path
  shellPath?: string
  // Serial port
  serialPort?: string
  serialBaudRate?: number
  serialDataBits?: number
  serialStopBits?: number
  serialParity?: string
  dbType?: string   // database type key
  dbName?: string   // default database name
  dbParams?: string // extra DSN query parameters, e.g. "sslmode=require&connect_timeout=30"
  postLoginScript?: string
  postLoginExpectSteps?: PostLoginExpectStep[]
  // SSH tunnel: reference to an existing SSH connection used as a jump host
  tunnelSSHConnId?: string
  tunnelSSHUser?: string
  tunnelSSHPassword?: string
  // SFTP max concurrent transfers (0 = unlimited)
  sftpMaxConcurrency?: number
  // FTP-specific
  ftpEncryption?: string  // "none" | "auto" | "required"
  ftpPassive?: boolean
  ftpEncoding?: string    // "utf-8" | "gbk" | "shift-jis" | "latin-1"
  // SMB-specific
  smbDomain?: string
  smbShare?: string
  // WebDAV-specific
  webdavURL?: string
  webdavUseSSL?: boolean
  // S3-specific
  s3Region?: string
  s3Bucket?: string
  // Terminal encoding (SSH/Telnet)
  encoding?: string // "utf-8" | "gbk" | "gb2312" | "gb18030" | "big5" | "shift-jis" | "euc-jp" | "euc-kr"
}

export interface SessionInfo {
  id: string
  type: string
  title: string
  status: SessionStatus
}

export interface Tab {
  id: string
  sessionId: string
  title: string
  type: 'ssh' | 'settings'
  groupId?: string
  config?: ConnectionConfig
  aiLocked?: boolean
}

export interface SplitNode {
  id: string
  direction: 'horizontal' | 'vertical' | null
  children: SplitNode[]
  tabGroupId?: string
  ratio: number
}
