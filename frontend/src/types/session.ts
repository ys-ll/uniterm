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

// Per-connection SSH algorithm preferences. mode omitted/'compatible' uses the
// built-in compatible set; 'custom' relies on the four preference lists (order
// = negotiation priority, each list non-empty).
export interface SSHAlgoConfig {
  mode?: 'compatible' | 'secure' | 'custom'
  keyExchanges?: string[]
  ciphers?: string[]
  macs?: string[]
  hostKeys?: string[]
}

export interface ConnectionConfig {
  id: string
  name: string
  remark?: string
  type: 'ssh' | 'telnet' | 'mosh' | 'rdp' | 'vnc' | 'spice' | 'database' | 'redis' | 'mongodb' | 'elasticsearch' | 'local' | 'wsl' | 'wsl-file' | 'sftp' | 'scp' | 'monitor' | 'ftp' | 'serial' | 'smb' | 'webdav' | 's3' | 'tcp' | 'k8s' | 'container' | 'container-file' | 'x11-desktop' | 'workspace'
  host: string
  port: number
  user: string
  authType: 'password' | 'key' | 'keyText' | 'agent' | 'identity' | 'kerberos' | 'apikey'
  kerberosRealm?: string // realm used for IP targets, e.g. EXAMPLE.COM
  sshAlgorithms?: SSHAlgoConfig // undefined = compatible default
  totpEnabled?: boolean
  totpSecret?: string
  password?: string
  keyPath?: string
  keyContent?: string // inline private-key text (authType === 'keyText')
  identityId?: string // reference to a vault identity (authType === 'identity')
  proxyId?: string // reference to a saved outbound proxy (SOCKS5/HTTP)
  // File-transfer protocol for the SSH companion file panel / "connect SFTP/SCP"
  // actions. Undefined/'sftp' = SFTP (default); 'scp' for hosts without an
  // SFTP subsystem.
  fileTransferProto?: 'sftp' | 'scp'
  groupId?: string
  // RDP-specific
  // Desktop size, or a sentinel: -1 = follow the primary monitor
  // ("full screen" option), -2 = adaptive (desktop follows the window size;
  // the backend reconnects with the new size after it stays stable).
  rdpFixedWidth?: number
  rdpFixedHeight?: number
  rdpEnableNLA?: boolean
  // RDP security-protocol preference. 'nla-prefer' offers TLS + CredSSP and
  // falls back to TLS when the server's CredSSP handshake breaks (legacy
  // default); 'nla' and 'tls' pin one protocol with no fallback; and
  // 'unencrypted' forces Standard RDP Security with ENCRYPTION_LEVEL_NONE
  // (plaintext traffic — never for confidential targets). Older connections
  // carry only rdpEnableNLA and resolve to nla-prefer/tls from it.
  rdpSecurityMode?: 'nla-prefer' | 'nla' | 'tls' | 'unencrypted'
  // Windows domain (or computer name for local accounts). When empty, a
  // "DOMAIN\user" prefix in user is used as a fallback.
  rdpDomain?: string
  rdpAdminSession?: boolean
  // Graphics/network profile tier: 'high' (LAN experience), 'balanced'
  // (default, broadband: wallpaper/theming off), 'low' (modem profile for
  // low-bandwidth links). Maps to the RDP connection_type + performance flags.
  rdpQuality?: 'high' | 'balanced' | 'low'
  // Local terminal shell path (for WSL, `wsl://<distro>` carries the distro)
  shellPath?: string
  // Extra command-line arguments appended to the local shell's launch
  // command line (WindTerm-style). For cmd.exe they are consumed by /k as
  // the command to run; also passed through to wsl:// distros and Unix shells.
  shellArgs?: string
  // Working directory for local terminal (defaults to user home)
  cwd?: string
  // Serial port
  serialPort?: string
  serialBaudRate?: number
  serialDataBits?: number
  serialStopBits?: number
  serialParity?: string
  dbType?: string   // database type key — SQL family only (mysql/postgres/oracle/sqlserver/rqlite/sqlite, sqlite keeps the file path in host); Redis/MongoDB/Elasticsearch are standalone types
  dbName?: string   // default database name
  dbParams?: string // extra DSN query parameters, e.g. "sslmode=require&connect_timeout=30"
  // Elasticsearch / OpenSearch. The auth type reuses the shared `authType`
  // field: 'basic'(default) | 'apikey'. In 'apikey' mode the API key is stored
  // in `password` (reusing the credential field, same convention as S3).
  esUseSsl?: boolean
  esPathPrefix?: string
  esSkipVerify?: boolean
  // Redis Sentinel fields (only used when redisMode === 'sentinel')
  redisMode?: string        // ''/'standalone'(default) | 'sentinel'
  redisMasterName?: string  // Sentinel primary group name, e.g. "mymaster"
  redisSentinels?: string   // comma-separated sentinel host:port list
  sentinelUser?: string     // Sentinel ACL user (optional)
  sentinelPassword?: string // Sentinel requirepass (optional)
  // Separator splitting keys into a folder tree in the Redis browser.
  // Defaults to ':' when empty; set to a character that never appears in
  // keys to disable grouping (flat list).
  redisKeySeparator?: string
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
  tunnelSSHUser?: string
  tunnelSSHPassword?: string
  // SFTP max concurrent transfers (0 = unlimited)
  sftpMaxConcurrency?: number
  // FTP-specific
  ftpEncryption?: string  // "none" | "auto" | "required"
  // Opt in to FTPS InsecureSkipVerify. Defaults to false (verify enabled).
  // Off by default preserves backwards compatibility for users today who
  // rely on it for self-signed certs — but the toggle now exists so the
  // choice is explicit, and a one-shot session-log warning fires on connect.
  ftpSkipVerify?: boolean
  // vncShared is forwarded to noVNC's RFB constructor as `shared`.
  // true (default) — the new client may connect alongside other clients.
  // false — the server will typically disconnect other clients on connect.
  vncShared?: boolean
  // Status bar preferences, remembered per remote desktop connection.
  desktopPreferences?: {
    scaleViewport?: boolean
    viewOnly?: boolean
    showDotCursor?: boolean
    // RDP/SPICE display zoom: 'adaptive' (fit pane), 'stretch' (fill pane,
    // ignore aspect ratio) or a factor ("0.75"); absent falls back to the
    // legacy scaleViewport boolean (true → 'adaptive').
    rdpZoom?: string
  }
  desktopView?: import('../utils/vncDisplays').DesktopView
  // vncRepeaterID is forwarded to noVNC's RFB constructor as `repeaterID`.
  // Empty (default) — direct connection to the VNC server.
  // Non-empty — connect via an UltraVNC-compatible repeater using the given ID.
  vncRepeaterID?: string
  vncClipboardEncoding?: 'auto' | 'latin1' | 'utf-8' | 'gbk' | 'gb2312' | 'gb18030' | 'big5' | 'shift-jis' | 'euc-jp' | 'euc-kr'
  // SMB-specific
  smbDomain?: string
  smbShare?: string
  // S3-specific
  s3Region?: string
  s3Bucket?: string
  // S3 URL addressing style. "virtual" (default) uses virtual-hosted style
  // URLs (https://bucket.endpoint/key) — required by Alibaba Cloud OSS /
  // Tencent COS / Huawei OBS. "path" uses path-style
  // (https://endpoint/bucket/key) for AWS S3 and MinIO.
  s3UrlStyle?: 'virtual' | 'path'
  // Terminal encoding (SSH/Telnet)
  encoding?: string // "utf-8" | "gbk" | "gb2312" | "gb18030" | "big5" | "shift-jis" | "euc-jp" | "euc-kr"
  // Terminal type reported to the remote host (SSH pty-req / telnet TTYPE /
  // local TERM env). Values outside the backend session.EffectiveTermType
  // whitelist fall back to xterm-256color.
  termType?: string
  // Backspace key byte sequence for terminal-stream types (ssh/telnet/serial).
  // 'del'  = ASCII DEL (0x7F) — xterm.js default
  // 'bs'   = ASCII BS  (0x08) — Huawei/H3C/Cisco network gear, Windows convention
  // 'vt220'= VT220 Delete (ESC[3~) — H3C / late Cisco IOS
  backspaceKey?: 'del' | 'bs' | 'vt220'
  // Telnet-specific options
  telnetNegotiationMode?: 'active' | 'passive'
  telnetSendMode?: 'character' | 'line'
  // Shared terminal options
  localEcho?: boolean
  newlineMode?: 'cr' | 'crlf'
  // Enable SSH X11 forwarding (ssh -X semantics). Bridges remote X11
  // clients to the local X server at $DISPLAY. Requires a local X server
  // (XQuartz on macOS, VcXsrv on Windows). Silent degradation on
  // missing $DISPLAY / xauth — see backend x11_forward.go.
  x11Forwarding?: boolean
  agentForwarding?: boolean
  // Enable session output log automatically on first connect. Applies
  // to terminal-stream types (ssh/telnet/serial/mosh/local).
  logOnConnect?: boolean
  // Kubernetes-specific
  k8sConfigPath?: string
  k8sConfigInline?: string
  k8sContext?: string
  k8sNamespace?: string
  k8sInsecureTls?: boolean
  // Exec terminal (container-exec panel) — params needed to reconnect the exec
  // stream. containerExecNamespace/containerExecContainerName are k8s-only
  // (pod namespace / target container in the pod); empty for docker runtimes.
  containerExecNamespace?: string
  containerExecContainerName?: string
  // Container connection (type: 'container')
  containerTransport?: 'ssh' | 'local'
  containerSSHConnId?: string
  containerRuntime?: 'docker' | 'podman' | 'nerdctl' | 'wslc'
  // Container exec terminal (container-exec panel) — 重连参数
  containerExecConnId?: string
  containerExecContainerId?: string
  containerExecShell?: string
  // Container file browser (type: 'container-file') — config.ID 即容器/k8s 连接 ID，
  // containerId 为目标容器（k8s 为 Pod 名），containerNamespace 仅 k8s 使用
  containerId?: string
  containerNamespace?: string
  // X11 Desktop (type: 'x11-desktop') — carries its own SSH credentials
  // (host, port, user, authType, password, keyPath) for direct connection.
  // X11 forwarding is forced on automatically. The actual desktop is
  // rendered by the local X server (VcXsrv/XQuartz/Xorg), not inside uniTerm.
  x11DesktopDesktopType?: 'gnome' | 'kde' | 'xfce' | 'mate' | 'cinnamon' | 'openbox' | 'custom'
  // Custom desktop command (only used when desktopType === 'custom').
  // Passed to sshd verbatim, so it goes through /bin/sh -c on the remote.
  x11DesktopCustomCmd?: string
  // Saved-workspace fields (type === 'workspace'): a named workspace snapshot
  // living in the connection tree. Members hold only connectionId references —
  // host credentials re-resolve at connect time, never stored here.
  workspaceMembers?: SavedWorkspaceMember[]
  workspaceLayout?: SavedWorkspaceLayoutNode | null
}

// One terminal slot inside a saved workspace. Layout leaves reference `id`
// (stable across re-saves of the same live workspace — the panel id is reused).
export interface SavedWorkspaceMember {
  id: string
  connectionId?: string // "" for self-contained members (local/wsl)
  type: string
  title: string
  shellPath?: string
}

// Mirrors the runtime LayoutNode with leaf panelId replaced by member id.
export interface SavedWorkspaceLayoutNode {
  type: 'leaf' | 'split'
  memberId?: string
  direction?: 'horizontal' | 'vertical'
  children?: SavedWorkspaceLayoutNode[]
  sizes?: number[]
}

// Result of the generic terminal connect path, consumed by workspace
// orchestration (create-from-selection / open-saved) to remap layouts and
// distinguish credential-cancel from connect failure.
export type MemberConnectStatus = 'ok' | 'cancelled' | 'failed'
export interface MemberConnectResult {
  status: MemberConnectStatus
  panelId?: string
}

export interface SessionInfo {
  id: string
  type: string
  title: string
  status: SessionStatus
  // Local WebSocket URL for VNC/SPICE sessions bridged through the
  // WebSocket↔TCP proxy; returned by CreateSession.
  proxyAddr?: string
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
