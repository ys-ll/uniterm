import { describe, it, expect } from 'vitest'
import { fileTransferProto, fileTransferDisabled, connectFileMenuKey, supportsRemoteSymlink, canOpenSshTerminal, asSshTerminalConfig } from './fileTransferUtils'

// "New link" must appear wherever the backend can create symbolic links. The
// WSL dual-pane file window carries config type 'wsl-file' (the companion
// sidebar uses 'wsl'), and both are served by the WSL file session.
describe('supportsRemoteSymlink', () => {
  it('accepts every backend with link semantics', () => {
    for (const type of ['ssh', 'sftp', 'scp', 'wsl', 'wsl-file']) {
      expect(supportsRemoteSymlink({ type })).toBe(true)
    }
  })

  it('rejects protocols without link semantics', () => {
    for (const type of ['ftp', 'smb', 'webdav', 's3', 'local', 'serial']) {
      expect(supportsRemoteSymlink({ type })).toBe(false)
    }
    expect(supportsRemoteSymlink(null)).toBe(false)
    expect(supportsRemoteSymlink(undefined)).toBe(false)
  })
})

// "Open terminal" may appear on a file-browser tab only when the backing
// config is SSH-backed: the ssh companion (sftp/scp per fileTransferProto)
// and standalone sftp/scp connections, whose config already carries the SSH
// host/port/auth. FTP, SMB, WebDAV and S3 have no SSH session to open.
describe('canOpenSshTerminal', () => {
  it('accepts ssh-backed configs', () => {
    for (const type of ['ssh', 'sftp', 'scp']) {
      expect(canOpenSshTerminal({ type })).toBe(true)
    }
  })

  it('rejects non-ssh file protocols', () => {
    for (const type of ['ftp', 'smb', 'webdav', 's3', 'wsl', 'wsl-file']) {
      expect(canOpenSshTerminal({ type })).toBe(false)
    }
    expect(canOpenSshTerminal(null)).toBe(false)
    expect(canOpenSshTerminal(undefined)).toBe(false)
  })
})

// Launching a terminal from a standalone sftp/scp tab must rewrite the config
// type to 'ssh' (launchConnection would otherwise open another file browser),
// while an ssh config passes through unchanged.
describe('asSshTerminalConfig', () => {
  it('rewrites sftp/scp configs to ssh, keeping every other field', () => {
    for (const type of ['sftp', 'scp'] as const) {
      const cfg = { type, host: 'h', port: 22, user: 'u', name: 'n' }
      expect(asSshTerminalConfig(cfg)).toEqual({ ...cfg, type: 'ssh' })
    }
  })

  it('returns ssh configs unchanged (same reference)', () => {
    const cfg = { type: 'ssh', host: 'h' }
    expect(asSshTerminalConfig(cfg)).toBe(cfg)
  })
})

// 'none' (issue #1624) disables the companion file sidebar entirely: hosts
// without any file-transfer service (switches) must not spawn companion
// sessions or error chatter. Only SSH configs carry the field; every other
// type keeps the SFTP default.
describe('fileTransferProto', () => {
  it('defaults to sftp for ssh configs without the field', () => {
    expect(fileTransferProto({ type: 'ssh' })).toBe('sftp')
    expect(fileTransferProto(null)).toBe('sftp')
    expect(fileTransferProto({ type: 'sftp' })).toBe('sftp')
  })

  it('honors scp and none on ssh configs', () => {
    expect(fileTransferProto({ type: 'ssh', fileTransferProto: 'scp' })).toBe('scp')
    expect(fileTransferProto({ type: 'ssh', fileTransferProto: 'none' })).toBe('none')
    // non-ssh configs never inherit the field
    expect(fileTransferProto({ type: 'scp', fileTransferProto: 'none' })).toBe('sftp')
  })
})

describe('fileTransferDisabled', () => {
  it('is true only for ssh configs set to none', () => {
    expect(fileTransferDisabled({ type: 'ssh', fileTransferProto: 'none' })).toBe(true)
    expect(fileTransferDisabled({ type: 'ssh', fileTransferProto: 'sftp' })).toBe(false)
    expect(fileTransferDisabled({ type: 'ssh' })).toBe(false)
    expect(fileTransferDisabled({ type: 'local' })).toBe(false)
  })
})

describe('connectFileMenuKey', () => {
  it('maps the protocol to its menu label key', () => {
    expect(connectFileMenuKey({ type: 'ssh', fileTransferProto: 'scp' })).toBe('sidebar.connectScp')
    expect(connectFileMenuKey({ type: 'ssh' })).toBe('sidebar.connectSftp')
  })

  it('returns empty for disabled transfer so menu entries hide', () => {
    expect(connectFileMenuKey({ type: 'ssh', fileTransferProto: 'none' })).toBe('')
  })
})
