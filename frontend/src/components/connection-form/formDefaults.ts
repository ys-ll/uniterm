import type { ConnectionConfig } from '../../types/session'

/**
 * Initial form state for the connection dialog. Extracted from ConnectionForm
 * so the parent component stays a thin orchestrator. Returns a fresh object on
 * every call so reactive() binds to a unique instance.
 */
export function createInitialForm(): ConnectionConfig {
  return {
    id: '',
    name: '',
    remark: '',
    type: 'ssh',
    host: '',
    port: 22,
    user: '',
    authType: 'password',
    kerberosRealm: '',
    password: '',
    keyPath: '',
    keyContent: '',
    groupId: undefined,
    rdpFixedWidth: -2,
    rdpFixedHeight: -2,
    rdpSmartSizing: false,
    rdpEnableNLA: true,
    rdpDomain: '',
    rdpAdminSession: false,
    dbType: '',
    dbName: '',
    dbParams: '',
    esUseSsl: false,
    esPathPrefix: '',
    esSkipVerify: false,
    redisMode: 'standalone',
    redisMasterName: '',
    redisSentinels: '',
    redisKeySeparator: ':',
    sentinelUser: '',
    sentinelPassword: '',
    postLoginScript: '',
    postLoginExpectSteps: [],
    sftpMaxConcurrency: 5,
    fileTransferProto: 'sftp' as 'sftp' | 'scp',
    x11Forwarding: false,
    agentForwarding: false,
    ftpEncryption: 'none',
    ftpPassive: true,
    ftpSkipVerify: false,
    vncShared: true,
    vncRepeaterID: '',
    encoding: 'utf-8',
    termType: 'xterm-256color',
    backspaceKey: 'del',
    telnetNegotiationMode: 'active' as 'active' | 'passive',
    telnetSendMode: 'character' as 'character' | 'line',
    localEcho: false,
    newlineMode: 'cr' as 'cr' | 'crlf',
    shellPath: '',
    smbDomain: 'WORKGROUP',
    smbShare: '',
    s3Region: 'us-east-1',
    s3Bucket: '',
    s3UrlStyle: 'virtual',
    logOnConnect: false,
    k8sConfigPath: '~/.kube/config',
    k8sConfigInline: '',
    k8sContext: '',
    k8sNamespace: 'default',
    containerTransport: 'ssh',
    containerSSHConnId: undefined,
    containerRuntime: 'docker',
    proxyId: undefined,
    x11DesktopDesktopType: 'gnome',
    x11DesktopCustomCmd: '',
    sshAlgorithms: undefined,
  }
}

/**
 * Reset every form field back to the defaults. Mirrors createInitialForm but
 * mutates the existing reactive object in place (so watchers bound to `form`
 * keep firing on the same instance). The caller resets the non-form local
 * refs (serial/k8s/rdp/group UI state) separately.
 */
export function resetFormFields(form: ConnectionConfig): void {
  form.id = ''
  form.name = ''
  form.remark = ''
  form.type = 'ssh'
  form.host = ''
  form.port = 22
  form.user = ''
  form.authType = 'password'
  form.password = ''
  form.keyPath = ''
  form.keyContent = ''
  form.identityId = ''
  form.groupId = undefined
  form.rdpFixedWidth = -2
  form.rdpFixedHeight = -2
  form.rdpSmartSizing = false
  form.rdpEnableNLA = true
  form.rdpDomain = ''
  form.rdpAdminSession = false
  form.kerberosRealm = ''
  form.sshAlgorithms = undefined
  form.dbType = ''
  form.dbName = ''
  form.dbParams = ''
  form.esUseSsl = false
  form.esPathPrefix = ''
  form.esSkipVerify = false
  form.redisMode = 'standalone'
  form.redisMasterName = ''
  form.redisSentinels = ''
  form.redisKeySeparator = ':'
  form.sentinelUser = ''
  form.sentinelPassword = ''
  form.postLoginScript = ''
  form.postLoginExpectSteps = []
  form.sftpMaxConcurrency = 5
  form.fileTransferProto = 'sftp'
  form.x11Forwarding = false
  form.agentForwarding = false
  form.ftpEncryption = 'none'
  form.ftpPassive = true
  form.ftpSkipVerify = false
  form.vncShared = true
  form.vncRepeaterID = ''
  form.encoding = 'utf-8'
  form.termType = 'xterm-256color'
  form.backspaceKey = 'del'
  form.telnetNegotiationMode = 'active'
  form.telnetSendMode = 'character'
  form.localEcho = false
  form.newlineMode = 'cr'
  form.shellPath = ''
  form.serialPort = ''
  form.serialBaudRate = 115200
  form.serialDataBits = 8
  form.serialStopBits = 1
  form.serialParity = 'none'
  form.tunnelSSHConnId = undefined
  form.proxyId = undefined
  form.logOnConnect = false
  form.k8sConfigPath = '~/.kube/config'
  form.k8sConfigInline = ''
  form.k8sContext = ''
  form.k8sNamespace = 'default'
  form.containerTransport = 'ssh'
  form.containerSSHConnId = undefined
  form.containerRuntime = 'docker'
  form.x11DesktopDesktopType = 'gnome'
  form.x11DesktopCustomCmd = ''
}