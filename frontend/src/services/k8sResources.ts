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

function apisListPath(group: string, version: string, plural: string, ns: string): string {
  return ns
    ? `/apis/${group}/${version}/namespaces/${encodeURIComponent(ns)}/${plural}?limit=500`
    : `/apis/${group}/${version}/${plural}?limit=500`
}

function apisWatchPath(group: string, version: string, plural: string, ns: string, rv: string): string {
  const base = ns
    ? `/apis/${group}/${version}/namespaces/${encodeURIComponent(ns)}/${plural}`
    : `/apis/${group}/${version}/${plural}`
  return `${base}?watch=true&allowWatchBookmarks=true&resourceVersion=${encodeURIComponent(rv || '')}`
}

// ── 描述器 ────────────────────────────────────────────────────

export const RESOURCES: ResourceDescriptor[] = [
  // ── Workloads ───────────────────────────────────────────────
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
  {
    key: 'deployments', kind: 'Deployment', apiVersion: 'apps/v1',
    namespaced: true, group: 'workloads', icon: 'Layers', label: 'Deployments',
    listPath: ns => apisListPath('apps', 'v1', 'deployments', ns),
    watchPath: (ns, rv) => apisWatchPath('apps', 'v1', 'deployments', ns, rv),
    columns: [
      { header: 'Name', value: d => d.metadata?.name || '' },
      { header: 'Namespace', value: d => d.metadata?.namespace || '' },
      { header: 'Ready', value: d => `${d.status?.readyReplicas || 0}/${d.spec?.replicas ?? 0}` },
      { header: 'Up-to-date', value: d => d.status?.updatedReplicas || 0 },
      { header: 'Available', value: d => d.status?.availableReplicas || 0 },
      { header: 'Age', value: d => age(d.metadata?.creationTimestamp) },
    ],
  },
  {
    key: 'statefulsets', kind: 'StatefulSet', apiVersion: 'apps/v1',
    namespaced: true, group: 'workloads', icon: 'Boxes', label: 'StatefulSets',
    listPath: ns => apisListPath('apps', 'v1', 'statefulsets', ns),
    watchPath: (ns, rv) => apisWatchPath('apps', 'v1', 'statefulsets', ns, rv),
    columns: [
      { header: 'Name', value: s => s.metadata?.name || '' },
      { header: 'Namespace', value: s => s.metadata?.namespace || '' },
      { header: 'Ready', value: s => `${s.status?.readyReplicas || 0}/${s.spec?.replicas ?? 0}` },
      { header: 'Age', value: s => age(s.metadata?.creationTimestamp) },
    ],
  },
  {
    key: 'daemonsets', kind: 'DaemonSet', apiVersion: 'apps/v1',
    namespaced: true, group: 'workloads', icon: 'GitFork', label: 'DaemonSets',
    listPath: ns => apisListPath('apps', 'v1', 'daemonsets', ns),
    watchPath: (ns, rv) => apisWatchPath('apps', 'v1', 'daemonsets', ns, rv),
    columns: [
      { header: 'Name', value: d => d.metadata?.name || '' },
      { header: 'Namespace', value: d => d.metadata?.namespace || '' },
      { header: 'Desired', value: d => d.status?.desiredNumberScheduled || 0 },
      { header: 'Current', value: d => d.status?.currentNumberScheduled || 0 },
      { header: 'Ready', value: d => d.status?.numberReady || 0 },
      { header: 'Age', value: d => age(d.metadata?.creationTimestamp) },
    ],
  },
  {
    key: 'jobs', kind: 'Job', apiVersion: 'batch/v1',
    namespaced: true, group: 'workloads', icon: 'PlayCircle', label: 'Jobs',
    listPath: ns => apisListPath('batch', 'v1', 'jobs', ns),
    watchPath: (ns, rv) => apisWatchPath('batch', 'v1', 'jobs', ns, rv),
    columns: [
      { header: 'Name', value: j => j.metadata?.name || '' },
      { header: 'Namespace', value: j => j.metadata?.namespace || '' },
      { header: 'Completions', value: j => `${j.status?.succeeded || 0}/${j.spec?.completions ?? 1}` },
      { header: 'Age', value: j => age(j.metadata?.creationTimestamp) },
    ],
  },
  {
    key: 'cronjobs', kind: 'CronJob', apiVersion: 'batch/v1',
    namespaced: true, group: 'workloads', icon: 'Clock', label: 'CronJobs',
    listPath: ns => apisListPath('batch', 'v1', 'cronjobs', ns),
    watchPath: (ns, rv) => apisWatchPath('batch', 'v1', 'cronjobs', ns, rv),
    columns: [
      { header: 'Name', value: c => c.metadata?.name || '' },
      { header: 'Namespace', value: c => c.metadata?.namespace || '' },
      { header: 'Schedule', value: c => c.spec?.schedule || '' },
      { header: 'Suspend', value: c => c.spec?.suspend ? 'true' : 'false' },
      { header: 'Active', value: c => (c.status?.active || []).length },
      { header: 'Last Schedule', value: c => age(c.status?.lastScheduleTime) },
      { header: 'Age', value: c => age(c.metadata?.creationTimestamp) },
    ],
  },
  {
    key: 'replicasets', kind: 'ReplicaSet', apiVersion: 'apps/v1',
    namespaced: true, group: 'workloads', icon: 'Copy', label: 'ReplicaSets',
    listPath: ns => apisListPath('apps', 'v1', 'replicasets', ns),
    watchPath: (ns, rv) => apisWatchPath('apps', 'v1', 'replicasets', ns, rv),
    columns: [
      { header: 'Name', value: r => r.metadata?.name || '' },
      { header: 'Namespace', value: r => r.metadata?.namespace || '' },
      { header: 'Desired', value: r => r.spec?.replicas ?? 0 },
      { header: 'Ready', value: r => r.status?.readyReplicas || 0 },
      { header: 'Age', value: r => age(r.metadata?.creationTimestamp) },
    ],
  },
  // ── Network ─────────────────────────────────────────────────
  {
    key: 'services', kind: 'Service', apiVersion: 'v1',
    namespaced: true, group: 'network', icon: 'Network', label: 'Services',
    listPath: ns => coreListPath('services', ns),
    watchPath: (ns, rv) => coreWatchPath('services', ns, rv),
    columns: [
      { header: 'Name', value: s => s.metadata?.name || '' },
      { header: 'Namespace', value: s => s.metadata?.namespace || '' },
      { header: 'Type', value: s => s.spec?.type || '' },
      { header: 'Cluster-IP', value: s => s.spec?.clusterIP || '' },
      { header: 'Ports', value: s => (s.spec?.ports || []).map((p: any) => `${p.port}/${p.protocol || 'TCP'}`).join(',') },
      { header: 'Age', value: s => age(s.metadata?.creationTimestamp) },
    ],
  },
  {
    key: 'ingresses', kind: 'Ingress', apiVersion: 'networking.k8s.io/v1',
    namespaced: true, group: 'network', icon: 'Globe', label: 'Ingresses',
    listPath: ns => apisListPath('networking.k8s.io', 'v1', 'ingresses', ns),
    watchPath: (ns, rv) => apisWatchPath('networking.k8s.io', 'v1', 'ingresses', ns, rv),
    columns: [
      { header: 'Name', value: i => i.metadata?.name || '' },
      { header: 'Namespace', value: i => i.metadata?.namespace || '' },
      { header: 'Class', value: i => i.spec?.ingressClassName || '' },
      { header: 'Hosts', value: i => (i.spec?.rules || []).map((r: any) => r.host).filter(Boolean).join(',') || '*' },
      { header: 'Age', value: i => age(i.metadata?.creationTimestamp) },
    ],
  },
  // ── Config ──────────────────────────────────────────────────
  {
    key: 'configmaps', kind: 'ConfigMap', apiVersion: 'v1',
    namespaced: true, group: 'config', icon: 'FileText', label: 'ConfigMaps',
    listPath: ns => coreListPath('configmaps', ns),
    watchPath: (ns, rv) => coreWatchPath('configmaps', ns, rv),
    columns: [
      { header: 'Name', value: c => c.metadata?.name || '' },
      { header: 'Namespace', value: c => c.metadata?.namespace || '' },
      { header: 'Data', value: c => Object.keys(c.data || {}).length },
      { header: 'Age', value: c => age(c.metadata?.creationTimestamp) },
    ],
  },
  {
    key: 'secrets', kind: 'Secret', apiVersion: 'v1',
    namespaced: true, group: 'config', icon: 'Lock', label: 'Secrets',
    listPath: ns => coreListPath('secrets', ns),
    watchPath: (ns, rv) => coreWatchPath('secrets', ns, rv),
    columns: [
      { header: 'Name', value: s => s.metadata?.name || '' },
      { header: 'Namespace', value: s => s.metadata?.namespace || '' },
      { header: 'Type', value: s => s.type || '' },
      { header: 'Data', value: s => Object.keys(s.data || {}).length },
      { header: 'Age', value: s => age(s.metadata?.creationTimestamp) },
    ],
  },
  // ── Storage ─────────────────────────────────────────────────
  {
    key: 'persistentvolumeclaims', kind: 'PersistentVolumeClaim', apiVersion: 'v1',
    namespaced: true, group: 'storage', icon: 'HardDrive', label: 'PVCs',
    listPath: ns => coreListPath('persistentvolumeclaims', ns),
    watchPath: (ns, rv) => coreWatchPath('persistentvolumeclaims', ns, rv),
    columns: [
      { header: 'Name', value: p => p.metadata?.name || '' },
      { header: 'Namespace', value: p => p.metadata?.namespace || '' },
      { header: 'Status', value: p => p.status?.phase || '' },
      { header: 'Volume', value: p => p.spec?.volumeName || '' },
      { header: 'Capacity', value: p => p.status?.capacity?.storage || '' },
      { header: 'Storage Class', value: p => p.spec?.storageClassName || '' },
      { header: 'Age', value: p => age(p.metadata?.creationTimestamp) },
    ],
  },
  {
    key: 'persistentvolumes', kind: 'PersistentVolume', apiVersion: 'v1',
    namespaced: false, group: 'storage', icon: 'Database', label: 'PVs',
    listPath: () => coreListPath('persistentvolumes', ''),
    watchPath: (_ns, rv) => coreWatchPath('persistentvolumes', '', rv),
    columns: [
      { header: 'Name', value: p => p.metadata?.name || '' },
      { header: 'Capacity', value: p => p.spec?.capacity?.storage || '' },
      { header: 'Access', value: p => (p.spec?.accessModes || []).join(',') },
      { header: 'Reclaim', value: p => p.spec?.persistentVolumeReclaimPolicy || '' },
      { header: 'Status', value: p => p.status?.phase || '' },
      { header: 'Claim', value: p => p.spec?.claimRef ? `${p.spec.claimRef.namespace}/${p.spec.claimRef.name}` : '' },
      { header: 'Storage Class', value: p => p.spec?.storageClassName || '' },
      { header: 'Age', value: p => age(p.metadata?.creationTimestamp) },
    ],
  },
  // ── Cluster ─────────────────────────────────────────────────
  {
    key: 'nodes', kind: 'Node', apiVersion: 'v1',
    namespaced: false, group: 'cluster', icon: 'Server', label: 'Nodes',
    listPath: () => coreListPath('nodes', ''),
    watchPath: (_ns, rv) => coreWatchPath('nodes', '', rv),
    columns: [
      { header: 'Name', value: n => n.metadata?.name || '' },
      { header: 'Status', value: n => {
        const c = (n.status?.conditions || []).find((c: any) => c.type === 'Ready')
        return c?.status === 'True' ? 'Ready' : 'NotReady'
      }},
      { header: 'Roles', value: n => Object.keys(n.metadata?.labels || {})
          .filter(l => l.startsWith('node-role.kubernetes.io/'))
          .map(l => l.substring('node-role.kubernetes.io/'.length))
          .join(',') || '<none>' },
      { header: 'Version', value: n => n.status?.nodeInfo?.kubeletVersion || '' },
      { header: 'Internal-IP', value: n => (n.status?.addresses || []).find((a: any) => a.type === 'InternalIP')?.address || '' },
      { header: 'OS', value: n => n.status?.nodeInfo?.osImage || '' },
      { header: 'Age', value: n => age(n.metadata?.creationTimestamp) },
    ],
  },
  {
    key: 'namespaces', kind: 'Namespace', apiVersion: 'v1',
    namespaced: false, group: 'cluster', icon: 'Folder', label: 'Namespaces',
    listPath: () => coreListPath('namespaces', ''),
    watchPath: (_ns, rv) => coreWatchPath('namespaces', '', rv),
    columns: [
      { header: 'Name', value: n => n.metadata?.name || '' },
      { header: 'Status', value: n => n.status?.phase || '' },
      { header: 'Age', value: n => age(n.metadata?.creationTimestamp) },
    ],
  },
  {
    key: 'events', kind: 'Event', apiVersion: 'v1',
    namespaced: true, group: 'cluster', icon: 'Bell', label: 'Events',
    listPath: ns => coreListPath('events', ns),
    watchPath: (ns, rv) => coreWatchPath('events', ns, rv),
    columns: [
      { header: 'Type', value: e => e.type || '' },
      { header: 'Reason', value: e => e.reason || '' },
      { header: 'Object', value: e => `${e.involvedObject?.kind}/${e.involvedObject?.name || ''}` },
      { header: 'Message', value: e => e.message || '' },
      { header: 'Namespace', value: e => e.metadata?.namespace || '' },
      { header: 'Age', value: e => age(e.metadata?.creationTimestamp || e.lastTimestamp) },
    ],
  },
]

export function getResource(key: string): ResourceDescriptor | undefined {
  return RESOURCES.find(r => r.key === key)
}
