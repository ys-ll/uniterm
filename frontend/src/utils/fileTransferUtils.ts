// Shared utilities for file-transfer features (companion panels, standalone
// SFTP/SCP connections, transfer menus). Protocol-related helpers that are
// used across components/stores live here.

export type FileTransferProto = 'sftp' | 'scp' | 'none'

// Effective file-transfer protocol of a connection. Only SSH connections carry
// the choice; anything else (and legacy configs without the field) is SFTP.
// 'none' disables the companion file panel entirely (hosts without any file
// transfer service, e.g. switches — issue #1624).
export function fileTransferProto(config?: { type?: string; fileTransferProto?: FileTransferProto } | null): FileTransferProto {
  if (config?.type !== 'ssh') return 'sftp'
  return config.fileTransferProto === 'scp' || config.fileTransferProto === 'none' ? config.fileTransferProto : 'sftp'
}

// True when the connection's companion file sidebar is explicitly disabled
// ('none'): no companion session, no directory probing/cwd-hook injection, and
// the file-transfer menu entries are hidden.
export function fileTransferDisabled(config?: { type?: string; fileTransferProto?: FileTransferProto } | null): boolean {
  return fileTransferProto(config) === 'none'
}

// i18n key for the "connect SFTP / connect SCP" context-menu item of an SSH
// connection, matching its configured file-transfer protocol. Empty when the
// file transfer is disabled ('none') and the menu entry must not render.
export function connectFileMenuKey(config?: { type?: string; fileTransferProto?: FileTransferProto } | null): string {
  const proto = fileTransferProto(config)
  if (proto === 'none') return ''
  return proto === 'scp' ? 'sidebar.connectScp' : 'sidebar.connectSftp'
}

// True when the connection's remote filesystem can create symbolic links
// (SSH = SFTP/SCP companion or standalone, and WSL — 'wsl' for the companion
// sidebar's terminal config, 'wsl-file' for the dual-pane file window). FTP,
// SMB, WebDAV and S3 have no link semantics, so their file panels hide the
// "new link" entry.
export function supportsRemoteSymlink(config?: { type?: string } | null): boolean {
  if (!config) return false
  return config.type === 'ssh' || config.type === 'sftp' || config.type === 'scp' || config.type === 'wsl' || config.type === 'wsl-file' || config.type === 'container-file'
}

// True when a file-browser tab may offer "open terminal": the backing config
// is SSH-backed — the ssh companion (sftp/scp per fileTransferProto) or a
// standalone sftp/scp connection, whose config already carries the SSH
// host/port/auth. FTP, SMB, WebDAV and S3 have no SSH session to open.
export function canOpenSshTerminal(config?: { type?: string } | null): boolean {
  return !!config && ['ssh', 'sftp', 'scp'].includes(config.type ?? '')
}

// Config for launching an SSH terminal from a file-browser tab. Standalone
// sftp/scp configs must be rewritten to type 'ssh' — launchConnection would
// otherwise follow their spec and open another file browser instead of a
// terminal. An ssh config passes through unchanged.
export function asSshTerminalConfig<T extends { type?: string }>(config: T): T {
  return config.type === 'ssh' ? config : { ...config, type: 'ssh' }
}

// True when an operation failed because the transport died rather than because
// the remote side rejected the request. Covers pkg/sftp's "connection lost"
// (SFTP/SCP), crypto/ssh and net package wording (SMB's Stat path wraps the
// raw net error), and the HTTP transport errors surfaced by WebDAV/S3. A
// matching error means retrying on a fresh connection may succeed — anything
// else (e.g. "no such directory") would fail again, so callers can skip the
// reconnect.
export function isConnectionLostError(err?: string | null): boolean {
  if (!err) return false
  return /connection lost|not connected|use of closed network connection|broken pipe|connection reset|connection refused|unexpected eof|\beof\b|already closed|ssh: disconnected|i\/o timeout|connection timed out/i.test(err)
}
