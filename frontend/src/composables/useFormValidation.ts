import type { Ref } from 'vue'
import type { ConnectionConfig, PostLoginExpectStep } from '../types/session'
import { msg } from '../services/message'
import { backendErrorText } from '../utils/backendError'

/**
 * Connection-form validation & normalization logic, extracted from
 * ConnectionForm.vue so the parent component only orchestrates layout.
 *
 * All functions are pure with respect to the form object they receive — they
 * mutate/normalize the passed reactive `form` (mirroring the original inline
 * behavior) but never reach into stores/i18n directly except through the
 * deps passed to the composable.
 */

/** Deep-clone expect steps so editing a hydrated config can't mutate the store copy. */
export function cloneExpectSteps(steps: PostLoginExpectStep[]): PostLoginExpectStep[] {
  return steps.map(step => ({ ...step }))
}

/** Trim/normalize expect steps and drop empty rows before save. */
export function normalizeExpectSteps(steps: PostLoginExpectStep[]): PostLoginExpectStep[] {
  return steps
    .map(step => ({
      expect: step.expect.trim(),
      send: step.send,
      enter: step.enter !== false,
      timeoutSecond: step.timeoutSecond || 10
    }))
    .filter(step => step.expect || step.send)
}

/** Generate a non-colliding "Name (n)" variant when the user leaves name blank. */
export function generateUniqueName(name: string, existing: { name: string }[]): string {
  if (!existing.some(c => c.name === name)) {
    return name
  }
  let idx = 1
  while (existing.some(c => c.name === `${name} (${idx})`)) {
    idx++
  }
  return `${name} (${idx})`
}

export interface UseFormValidationOptions {
  form: ConnectionConfig
  /** Existing connections, used for unique-name generation. */
  connections: { id: string; name: string }[]
  /** i18n translate function. */
  t: (key: string) => string
  /** True when editing a sqlite database (file-backed, no host/port). */
  isSqliteDb: Ref<boolean>
  /** True for ssh-like session types that carry ssh algorithm prefs. */
  isSSHAlgoType: Ref<boolean>
  /** 'script' | 'expect' radio for post-login automation. */
  postLoginMode: Ref<'script' | 'expect'>
  /** Serial refs kept outside `form` so allow-create won't produce strings. */
  serialBaudRateInput: Ref<string>
  serialDataBitsValue: Ref<number>
  serialStopBitsValue: Ref<number>
  serialParityValue: Ref<string>
  /** SSH connections available as container/tunnel jump hosts. */
  sshConnections: Ref<{ id: string; name: string; user: string; host: string; port: number }[]>
}

/**
 * Normalize the in-progress form into a saveable ConnectionConfig.
 *
 * Mirrors the original inline implementation: syncs serial refs into `form`,
 * strips type-irrelevant fields, validates required fields (throwing on
 * failure), and backfills a unique name when blank.
 */
export function normalizeForm(opts: UseFormValidationOptions): ConnectionConfig {
  const { form, connections, t, isSqliteDb, isSSHAlgoType, postLoginMode,
    serialBaudRateInput, serialDataBitsValue, serialStopBitsValue, serialParityValue } = opts

  // Sync serial refs into form before normalization
  if (form.type === 'serial') {
    form.serialBaudRate = parseInt(serialBaudRateInput.value, 10) || 115200
    serialBaudRateInput.value = String(form.serialBaudRate)
    form.serialDataBits = serialDataBitsValue.value
    form.serialStopBits = serialStopBitsValue.value
    form.serialParity = serialParityValue.value
  }
  const normalized = { ...form }
  // Terminal type only applies to ssh/telnet/local; strip it elsewhere so
  // other connection JSON stays clean.
  if (normalized.type !== 'ssh' && normalized.type !== 'telnet' && normalized.type !== 'local') {
    normalized.termType = undefined
  }
  // SSH algorithms: only SSH-like session types carry the preference.
  // Compatible mode stores nothing (backend default), secure mode stores
  // just the mode; custom mode requires all four lists filled.
  if (!isSSHAlgoType.value) {
    normalized.sshAlgorithms = undefined
  } else if (normalized.sshAlgorithms?.mode === 'compatible') {
    normalized.sshAlgorithms = undefined
  } else if (normalized.sshAlgorithms?.mode === 'secure') {
    normalized.sshAlgorithms = { mode: 'secure' }
  } else if (normalized.sshAlgorithms?.mode === 'custom') {
    const algo = normalized.sshAlgorithms
    if (!algo.keyExchanges?.length || !algo.ciphers?.length || !algo.macs?.length || !algo.hostKeys?.length) {
      throw new Error(t('conn.sshAlgoEmptyList'))
    }
  }
  // Identity (密钥库) 的凭据完全由所引用的 identity 提供，连接时
  // MaterializeIdentity 会以 identity 的 password 覆盖本字段。
  // 从别的认证方式切到 identity 时，旧字段若残留 enc:v1: 密文，会被
  // 云同步原样带进仓库（同步规范化只处理 authType=="password" 的连接），
  // 导致他机显示字面量 enc:v1:xxx（issue #711）。保存时清掉以绝后患。
  // User 不清：identity 认证下它是表单里显式填写的覆盖用户名
  // （issue #959），连接时非空则覆盖密钥库的用户名，为空则回退。
  if (normalized.authType === 'identity') {
    normalized.password = ''
  }
  // Drop the retired timing option when an older connection is edited.
  delete (normalized as unknown as Record<string, unknown>).cwdHookMode
  // key 相关字段只在对应认证方式下有效；切走后清掉，避免把残留的路径/密钥
  // 文本原样带进仓库（同 #711 语义）。
  if (normalized.authType !== 'key') normalized.keyPath = ''
  if (normalized.authType !== 'keyText') normalized.keyContent = ''
  normalized.kerberosRealm = normalized.authType === 'kerberos'
    ? normalized.kerberosRealm?.trim()
    : ''
  normalized.postLoginExpectSteps = normalizeExpectSteps(form.postLoginExpectSteps || [])
  if (postLoginMode.value === 'script') {
    normalized.postLoginExpectSteps = []
  } else {
    normalized.postLoginScript = ''
  }
  const redisSentinel = normalized.type === 'redis' && normalized.redisMode === 'sentinel'
  if (redisSentinel) {
    if (!normalized.redisSentinels?.trim()) throw new Error(t('conn.redisSentinelsRequired'))
    if (!normalized.redisMasterName?.trim()) throw new Error(t('conn.redisMasterNameRequired'))
  } else if (normalized.type !== 'local' && normalized.type !== 'wsl' && normalized.type !== 'serial' && normalized.type !== 'k8s' && normalized.type !== 'container' && !isSqliteDb.value && !normalized.host.trim()) {
    throw new Error(t('conn.hostRequired'))
  }
  if (isSqliteDb.value && !normalized.host.trim()) {
    throw new Error(t('db.sqlitePathRequired'))
  }
  if (normalized.type === 's3') {
    if (!normalized.user?.trim()) throw new Error('S3: Access Key is required')
    if (!normalized.password?.trim()) throw new Error('S3: Secret Key is required')
  }
  if (normalized.type === 'database' && normalized.dbType === 'postgres' && !normalized.dbName?.trim()) {
    throw new Error(t('db.pgDbNameRequired'))
  }
  if (normalized.type === 'elasticsearch' && normalized.authType === 'apikey' && !normalized.password?.trim()) {
    throw new Error(t('conn.esApiKeyRequired'))
  }
  if (!normalized.name.trim()) {
    normalized.name = generateUniqueName(
      normalized.type === 'serial' ? (normalized.serialPort || 'Serial')
        : normalized.type === 'k8s' ? (normalized.k8sContext || 'Kubernetes')
        : normalized.type === 'container' ? (normalized.containerRuntime || 'container')
        : redisSentinel ? (normalized.redisMasterName?.trim() || 'redis')
        : normalized.host.trim(),
      connections
    )
  }
  return normalized
}

/** Container transport validation: SSH transport needs a referenced SSH connection. */
export function validateContainer(
  form: ConnectionConfig,
  sshConnections: { id: string }[],
  t: (key: string) => string
): boolean {
  if (form.type !== 'container') return true
  if (form.containerTransport === 'ssh' && !form.containerSSHConnId) {
    msg.error(t('conn.containerSSHRefRequired'))
    return false
  }
  if (form.containerTransport === 'ssh' && !sshConnections.length) return false
  return true
}

/** X11 desktop validation: custom desktop type needs a non-empty command. */
export function validateX11Desktop(
  form: ConnectionConfig,
  t: (key: string) => string
): boolean {
  if (form.type !== 'x11-desktop') return true
  if (form.x11DesktopDesktopType === 'custom' && !form.x11DesktopCustomCmd?.trim()) {
    msg.error(t('conn.x11DesktopCustomCmdRequired'))
    return false
  }
  return true
}

/** Run a connection test against the backend, surfacing the result via msg. */
export async function runConnectionTest(
  config: ConnectionConfig,
  testStatus: Ref<'idle' | 'checking' | 'success' | 'error'>
): Promise<void> {
  // TestConnection is imported lazily to avoid pulling backend bindings into
  // every consumer of this composable (and to keep the original import shape).
  const { TestConnection } = await import('../../bindings/github.com/ys-ll/uniterm/app')
  testStatus.value = 'checking'
  try {
    const desc = await TestConnection(config)
    testStatus.value = 'success'
    msg.success(desc)
  } catch (e: any) {
    testStatus.value = 'error'
    msg.error(backendErrorText(e))
  }
}