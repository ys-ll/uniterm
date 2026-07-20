// ResourceDescriptor 表驱动 K8sTree + K8sResourceList。
// 每种资源在这里加一条，不写新组件。

export interface ColoredCell { text: string; tone?: 'ok' | 'warn' | 'err' }

export interface ColumnDef {
  header: string
  value: (obj: any) => string | number | ColoredCell
  width?: number
}

export type ResourceGroup = 'workloads' | 'network' | 'config' | 'storage' | 'cluster'

export interface ResourceDescriptor {
  key: string
  kind: string
  apiVersion: string
  namespaced: boolean
  group: ResourceGroup
  icon: string          // lucide 图标名
  label: string
  listPath: (ns: string) => string
  watchPath: (ns: string, rv: string) => string
  columns: ColumnDef[]
}

// ── 通用列生成器 ────────────────────────────────────────────────

export function age(ts: string | undefined): string {
  if (!ts) return '—'
  const diff = Date.now() - new Date(ts).getTime()
  const s = Math.floor(diff / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h`
  return `${Math.floor(h / 24)}d`
}

function podReady(p: any): string {
  const cs = p.status?.containerStatuses || []
  const ready = cs.filter((c: any) => c.ready).length
  return `${ready}/${cs.length}`
}

function podRestarts(p: any): number {
  const cs = p.status?.containerStatuses || []
  return cs.reduce((sum: number, c: any) => sum + (c.restartCount || 0), 0)
}

// ── 路径 helper ────────────────────────────────────────────────

function coreListPath(plural: string, ns: string): string {
  return ns
    ? `/api/v1/namespaces/${encodeURIComponent(ns)}/${plural}?limit=500`
    : `/api/v1/${plural}?limit=500`
}

function coreWatchPath(plural: string, ns: string, rv: string): string {
  const base = ns
    ? `/api/v1/namespaces/${encodeURIComponent(ns)}/${plural}`
    : `/api/v1/${plural}`
  return `${base}?watch=true&allowWatchBookmarks=true&resourceVersion=${encodeURIComponent(rv || '')}`
}

// ── 描述器 ────────────────────────────────────────────────────

export const RESOURCES: ResourceDescriptor[] = [
  {
    key: 'pods', kind: 'Pod', apiVersion: 'v1',
    namespaced: true, group: 'workloads', icon: 'Box', label: 'Pods',
    listPath: ns => coreListPath('pods', ns),
    watchPath: (ns, rv) => coreWatchPath('pods', ns, rv),
    columns: [
      { header: 'Name', value: p => p.metadata?.name || '' },
      { header: 'Namespace', value: p => p.metadata?.namespace || '' },
      { header: 'Ready', value: podReady },
      { header: 'Status', value: p => p.status?.phase || '' },
      { header: 'Restarts', value: podRestarts },
      { header: 'Age', value: p => age(p.metadata?.creationTimestamp) },
      { header: 'Node', value: p => p.spec?.nodeName || '' },
    ],
  },
]

export function getResource(key: string): ResourceDescriptor | undefined {
  return RESOURCES.find(r => r.key === key)
}
