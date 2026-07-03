import type { ConnectionConfig } from '../types/session'

const QUICK_PROTOCOLS: Record<string, { type: string; dbType?: string; defaultPort?: number }> = {
  ssh: { type: 'ssh', defaultPort: 22 },
  telnet: { type: 'telnet', defaultPort: 23 },
  mosh: { type: 'mosh', defaultPort: 22 },
  rdp: { type: 'rdp', defaultPort: 3389 },
  vnc: { type: 'vnc', defaultPort: 5900 },
  spice: { type: 'spice' },
  ftp: { type: 'ftp', defaultPort: 21 },
  sftp: { type: 'sftp', defaultPort: 22 },
  smb: { type: 'smb', defaultPort: 445 },
  s3: { type: 's3' },
  webdav: { type: 'webdav' },
  http: { type: 'webdav' },
  https: { type: 'webdav' },
  mysql: { type: 'database', dbType: 'mysql', defaultPort: 3306 },
  postgres: { type: 'database', dbType: 'postgres', defaultPort: 5432 },
  postgresql: { type: 'database', dbType: 'postgres', defaultPort: 5432 },
  redis: { type: 'database', dbType: 'redis', defaultPort: 6379 },
  oracle: { type: 'database', dbType: 'oracle', defaultPort: 1521 },
  sqlserver: { type: 'database', dbType: 'sqlserver', defaultPort: 1433 },
  rqlite: { type: 'database', dbType: 'rqlite', defaultPort: 4001 },
}

// Helper: parse [user[:password]@]host[:port]
function parseHost(s: string) {
  const m = s.match(/^(?:([^@]+)@)?([^:]+)(?::(\d+))?$/)
  if (!m) return { host: s }
  let user = ''
  let pass = ''
  if (m[1]) {
    const colonIdx = m[1].indexOf(':')
    if (colonIdx >= 0) {
      user = m[1].slice(0, colonIdx)
      pass = m[1].slice(colonIdx + 1)
    } else {
      user = m[1]
    }
  }
  return {
    user,
    password: pass || undefined,
    host: m[2],
    port: m[3] ? parseInt(m[3]) : undefined,
  }
}

export function parseQuickConnect(raw: string): Partial<ConnectionConfig> | null {
  const input = raw.trim()
  if (!input) return null

  const parts = input.split(/\s+/)
  const first = parts[0].toLowerCase()

  // Pattern: [type] [user[:password]@]host[:port]
  if (parts.length >= 2 && QUICK_PROTOCOLS[first]) {
    const cfg = QUICK_PROTOCOLS[first]
    const rest = parts.slice(1).join(' ')
    const h = parseHost(rest)
    const result: any = { type: cfg.type, host: h.host }
    if (cfg.dbType) result.dbType = cfg.dbType
    if (h.user) result.user = h.user
    if (h.password) result.password = h.password
    result.port = h.port || cfg.defaultPort
    return result
  }

  // Patterns: [user[:password]@]host[:port]  or  host[:port]  (default ssh)
  const h = parseHost(input)
  const result: any = { type: 'ssh', host: h.host }
  if (h.user) result.user = h.user
  if (h.password) result.password = h.password
  result.port = h.port || 22
  return result
}

// Look up default port from QUICK_PROTOCOLS by type or dbType
function getDefaultPort(type: string, dbType?: string): number | undefined {
  return QUICK_PROTOCOLS[type]?.defaultPort ?? (dbType ? QUICK_PROTOCOLS[dbType]?.defaultPort : undefined)
}

export function formatConnSubtitle(config: ConnectionConfig, getShellLabel?: (path: string) => string): string {
  const typeLabel = config.type === 'database' ? (config.dbType || config.type) : config.type
  let detail: string
  if (config.type === 's3') {
    detail = config.host
  } else if (config.type === 'local') {
    detail = getShellLabel ? getShellLabel(config.shellPath || '') : 'Local'
  } else {
    const defaultPort = getDefaultPort(config.type, config.dbType)
    const showPort = defaultPort !== config.port && defaultPort !== undefined
    const portStr = showPort ? `:${config.port}` : ''
    detail = config.user ? `${config.user}@${config.host}${portStr}` : `${config.host}${portStr}`
  }
  return `${typeLabel} ${detail}`
}
