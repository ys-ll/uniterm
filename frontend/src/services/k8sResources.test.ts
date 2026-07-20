import { describe, it, expect } from 'vitest'
import { RESOURCES, getResource } from './k8sResources'

describe('k8sResources', () => {
  it('exposes at least Pods', () => {
    const pods = getResource('pods')
    expect(pods).toBeTruthy()
    expect(pods!.kind).toBe('Pod')
    expect(pods!.apiVersion).toBe('v1')
    expect(pods!.namespaced).toBe(true)
    expect(pods!.group).toBe('workloads')
  })

  it('pods listPath varies by namespace', () => {
    const pods = getResource('pods')!
    expect(pods.listPath('default')).toBe('/api/v1/namespaces/default/pods?limit=500')
    expect(pods.listPath('')).toBe('/api/v1/pods?limit=500')
  })

  it('pods watchPath includes resourceVersion', () => {
    const pods = getResource('pods')!
    const wp = pods.watchPath('kube-system', '12345')
    expect(wp).toContain('/api/v1/namespaces/kube-system/pods')
    expect(wp).toContain('watch=true')
    expect(wp).toContain('resourceVersion=12345')
    expect(wp).toContain('allowWatchBookmarks=true')
  })

  it('pods columns cover Name / Namespace / Ready / Status / Restarts / Age / Node', () => {
    const pods = getResource('pods')!
    const headers = pods.columns.map(c => c.header)
    expect(headers).toEqual(['Name', 'Namespace', 'Ready', 'Status', 'Restarts', 'Age', 'Node'])
  })

  it('every column value fn returns non-undefined for a minimal pod fixture', () => {
    const pods = getResource('pods')!
    const pod = {
      metadata: { name: 'p', namespace: 'default', uid: 'u1', creationTimestamp: new Date().toISOString() },
      status: { phase: 'Running', containerStatuses: [{ ready: true, restartCount: 0 }] },
      spec: { nodeName: 'n1' },
    }
    for (const col of pods.columns) {
      const v = col.value(pod)
      expect(v).not.toBeUndefined()
    }
  })

  it('RESOURCES array contains pods', () => {
    expect(RESOURCES.find(r => r.key === 'pods')).toBeTruthy()
  })
})
