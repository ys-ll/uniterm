export type SessionStatus = 'connecting' | 'connected' | 'disconnected' | 'error'

export interface ConnectionGroup {
  id: string
  name: string
  parentId?: string
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
  type: 'ssh' | 'telnet' | 'mosh' | 'rdp' | 'vnc' | 'spice' | 'database' | 'local' | 'sftp' | 'monitor' | 'ftp' | 'serial' | 'smb' | 'webdav' | 's3' | 'k8s'
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
  // Working directory for local terminal (defaults to user home)
  cwd?: string
  // Serial port
  serialPort?: string
  serialBaudRate?: number
  serialDataBits?: number
  serialStopBits?: number
  serialParity?: string
  dbType?: string   // database type key
  dbName?: string   // default database name
  dbParams?: string // extra DSN query parameters, e.g. "sslmode=require&connect_timeout=30"
  // Redis Sentinel fields (only used when redisMode === 'sentinel')
  redisMode?: string        // ''/'standalone'(default) | 'sentinel'
  redisMasterName?: string  // Sentinel primary group name, e.g. "mymaster"
  redisSentinels?: string   // comma-separated sentinel host:port list
  sentinelUser?: string     // Sentinel ACL user (optional)
  sentinelPassword?: string // Sentinel requirepass (optional)
  postLoginScript?: string
  postLoginExpectSteps?: PostLoginExpectStep[]
  // SSH tunnel: reference to an existing SSH connection used as a jump host
  tunnelSSHConnId?: string
  // Initial terminal size reported by the frontend BEFORE the SSH/local PTY
  // is created. Without this the backend starts the remote shell with the
  // default 80x24 and Claude Code (or any TUI app) draws tables at that
  // width; by the time the frontend's fitAddon measures the actual xterm
  // cols and sends SessionResize, several lines of output are already
  // wrapped at 80 cols and the rest at the real cols — the table borders
  // drift apart. Frontend fills these after acquireTerminal + fitAddon.fit()
  // and passes them in CreateSession.
  initialCols?: number
  initialRows?: number
  // When true, CreateSession returns immediately without connecting.
  // The frontend calls SessionStart after it has mounted the xterm
  // terminal and written initialCols/Rows via SetPendingSize — so the
  // PTY starts at the real xterm size, not the 80x24 default that
  // would otherwise wrap Claude Code tables at the wrong column count.
  deferConnect?: boolean
  tunnelSSHUser?: string
  tunnelSSHPassword?: string
  // SFTP max concurrent transfers (0 = unlimited)
  sftpMaxConcurrency?: number
  // FTP-specific
  ftpEncryption?: string  // "none" | "auto" | "required"
  ftpPassive?: boolean
  ftpEncoding?: string    // "utf-8" | "gbk" | "shift-jis" | "latin-1"
  // Opt in to FTPS InsecureSkipVerify. Defaults to false (verify enabled).
  // Off by default preserves backwards compatibility for users today who
  // rely on it for self-signed certs — but the toggle now exists so the
  // choice is explicit, and a one-shot session-log warning fires on connect.
  ftpSkipVerify?: boolean
  // SMB-specific
  smbDomain?: string
  smbShare?: string
  // S3-specific
  s3Region?: string
  s3Bucket?: string
  // Terminal encoding (SSH/Telnet)
  encoding?: string // "utf-8" | "gbk" | "gb2312" | "gb18030" | "big5" | "shift-jis" | "euc-jp" | "euc-kr"
  // Enable session output log automatically on first connect. Applies
  // to terminal-stream types (ssh/telnet/serial/mosh/local).
  logOnConnect?: boolean
  // Kubernetes-specific
  k8sConfigPath?: string
  k8sConfigInline?: string
  k8sContext?: string
  k8sNamespace?: string
  k8sInsecureTls?: boolean
  // K8s exec terminal (k8s-exec panel) — params needed to reconnect the exec stream.
  k8sExecConnId?: string
  k8sExecPod?: string
  k8sExecContainer?: string
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
