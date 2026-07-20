import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { K8sWatchEvent } from '../types/k8s'
import * as client from '../services/k8sClient'
import { getResource } from '../services/k8sResources'

interface ResourceListState {
  items: Map<string, any>          // uid → object
  resourceVersion: string
  watch: client.WatchHandle | null
  refCount: number
  error: string
}

export const useK8sStore = defineStore('k8s', () => {
  // key = `${connID}::${resourceKey}::${ns}` （ns='' 表示 all / 集群级）
  const states = ref<Map<string, ResourceListState>>(new Map())

  function k(connID: string, resourceKey: string, ns: string) {
    return `${connID}::${resourceKey}::${ns}`
  }

  // 显式换 slot 触发外层 ref 响应式；Map 突变本身不会。
  function bump(key: string, st: ResourceListState) {
    states.value.set(key, { ...st, items: new Map(st.items) })
  }

  async function subscribe(connID: string, resourceKey: string, ns: string) {
    const desc = getResource(resourceKey)
    const effectiveNs = desc && !desc.namespaced ? '' : ns
    const key = k(connID, resourceKey, effectiveNs)
    if (!desc) {
      // 未知资源：写错误，不发请求
      states.value.set(key, {
        items: new Map(), resourceVersion: '', watch: null, refCount: 1,
        error: `unknown resource ${resourceKey}`,
      })
      return
    }

    let st = states.value.get(key)
    if (st) {
      st.refCount++
      return
    }
    st = { items: new Map(), resourceVersion: '', watch: null, refCount: 1, error: '' }
    states.value.set(key, st)

    // 初始 list
    const listPath = desc.listPath(effectiveNs)
    const { status, data, raw } = await client.requestJSON<any>(connID, 'GET', listPath)
    if (status !== 200 || !data) {
      st.error = `list ${desc.kind} HTTP ${status}: ${raw?.slice(0, 400) || ''}`
      bump(key, st)
      return
    }
    for (const item of data.items || []) {
      st.items.set(item.metadata?.uid || item.metadata?.name, item)
    }
    st.resourceVersion = data.metadata?.resourceVersion || ''
    bump(key, st)
    st = states.value.get(key)!

    // watch
    const watchPath = desc.watchPath(effectiveNs, st.resourceVersion)
    st.watch = await client.startWatch(connID, watchPath, (ev) => handleEvent(key, ev))
  }

  function handleEvent(key: string, ev: K8sWatchEvent) {
    const st = states.value.get(key)
    if (!st || !ev.object) return
    const uid = ev.object.metadata?.uid || ev.object.metadata?.name
    if (!uid) return
    const rv = ev.object.metadata?.resourceVersion
    switch (ev.type) {
      case 'ADDED':
      case 'MODIFIED':
        st.items.set(uid, ev.object)
        if (rv) st.resourceVersion = rv
        break
      case 'DELETED':
        st.items.delete(uid)
        if (rv) st.resourceVersion = rv
        break
      case 'BOOKMARK':
        if (rv) st.resourceVersion = rv
        break
    }
    bump(key, st)
  }

  function unsubscribe(connID: string, resourceKey: string, ns: string) {
    const desc = getResource(resourceKey)
    const effectiveNs = desc && !desc.namespaced ? '' : ns
    const key = k(connID, resourceKey, effectiveNs)
    const st = states.value.get(key)
    if (!st) return
    st.refCount--
    if (st.refCount <= 0) {
      st.watch?.stop()
      states.value.delete(key)
    }
  }

  function getItems(connID: string, resourceKey: string, ns: string): any[] {
    const desc = getResource(resourceKey)
    const effectiveNs = desc && !desc.namespaced ? '' : ns
    const st = states.value.get(k(connID, resourceKey, effectiveNs))
    if (!st) return []
    return Array.from(st.items.values())
  }

  function getError(connID: string, resourceKey: string, ns: string): string {
    const desc = getResource(resourceKey)
    const effectiveNs = desc && !desc.namespaced ? '' : ns
    return states.value.get(k(connID, resourceKey, effectiveNs))?.error || ''
  }

  return { subscribe, unsubscribe, getItems, getError }
})
