import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { K8sWatchEvent } from '../types/k8s'
import * as client from '../services/k8sClient'

interface PodListState {
  namespace: string
  pods: Map<string, any>          // uid → object
  resourceVersion: string
  watch: client.WatchHandle | null
  refCount: number
  error: string
}

export const useK8sStore = defineStore('k8s', () => {
  // key = `${connID}::${namespace}` （'' 表示 all）
  const pods = ref<Map<string, PodListState>>(new Map())

  function key(connID: string, ns: string) { return `${connID}::${ns}` }

  async function subscribePods(connID: string, ns: string) {
    const k = key(connID, ns)
    let st = pods.value.get(k)
    if (st) {
      st.refCount++
      return
    }
    st = { namespace: ns, pods: new Map(), resourceVersion: '', watch: null, refCount: 1, error: '' }
    pods.value.set(k, st)

    // 初始 list
    const path = ns
      ? `/api/v1/namespaces/${encodeURIComponent(ns)}/pods?limit=500`
      : `/api/v1/pods?limit=500`
    const { status, data, raw } = await client.requestJSON<any>(connID, 'GET', path)
    if (status !== 200 || !data) {
      st.error = `list pods HTTP ${status}: ${raw?.slice(0, 400) || ''}`
      return
    }
    for (const item of data.items || []) {
      st.pods.set(item.metadata?.uid || item.metadata?.name, item)
    }
    st.resourceVersion = data.metadata?.resourceVersion || ''
    // 与 handleEvent 一致：显式换掉 slot 触发外层 ref 的响应式更新，
    // 否则初次 list 结果（往内嵌 Map 里塞）不会推给 Vue。
    pods.value.set(k, { ...st, pods: new Map(st.pods) })
    st = pods.value.get(k)!

    // 建 watch
    const watchPath = ns
      ? `/api/v1/namespaces/${encodeURIComponent(ns)}/pods?watch=true&allowWatchBookmarks=true&resourceVersion=${st.resourceVersion}`
      : `/api/v1/pods?watch=true&allowWatchBookmarks=true&resourceVersion=${st.resourceVersion}`
    st.watch = await client.startWatch(connID, watchPath, (ev) => handleEvent(k, ev))
  }

  function handleEvent(k: string, ev: K8sWatchEvent) {
    const st = pods.value.get(k)
    if (!st || !ev.object) return
    const uid = ev.object.metadata?.uid || ev.object.metadata?.name
    if (!uid) return
    const rv = ev.object.metadata?.resourceVersion
    switch (ev.type) {
      case 'ADDED':
      case 'MODIFIED':
        st.pods.set(uid, ev.object)
        if (rv) st.resourceVersion = rv
        break
      case 'DELETED':
        st.pods.delete(uid)
        if (rv) st.resourceVersion = rv
        break
      case 'BOOKMARK':
        if (rv) st.resourceVersion = rv
        break
    }
    // 触发响应式（Map 需要 shallow-refresh）
    pods.value.set(k, { ...st, pods: new Map(st.pods) })
  }

  function unsubscribePods(connID: string, ns: string) {
    const k = key(connID, ns)
    const st = pods.value.get(k)
    if (!st) return
    st.refCount--
    if (st.refCount <= 0) {
      st.watch?.stop()
      pods.value.delete(k)
    }
  }

  function getPods(connID: string, ns: string): any[] {
    const st = pods.value.get(key(connID, ns))
    if (!st) return []
    return Array.from(st.pods.values())
  }

  function getError(connID: string, ns: string): string {
    return pods.value.get(key(connID, ns))?.error || ''
  }

  return { subscribePods, unsubscribePods, getPods, getError }
})
