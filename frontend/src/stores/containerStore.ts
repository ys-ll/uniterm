import { defineStore } from 'pinia'
import * as client from '../services/containerClient'
import type { ContainerConnectCreds } from '../services/containerClient'
import { usePanelStore } from './panelStore'
import { useTabStore } from './tabStore'
import { useSessionStore } from './sessionStore'
import type { ContainerTab, ContainerInfo, ContainerImage, InspectResult, ImageLayer, RuntimeInfo } from '../types/container'

export interface ContainerSession {
  connId: string
  runtime: ContainerTab['runtime']
  containers: ContainerInfo[]
  images: ContainerImage[]
  namespaces: string[]
  namespace: string
  loading: boolean
  refreshing: boolean
  imagesLoading: boolean
  error: string
  info: RuntimeInfo | null
}

export const useContainerStore = defineStore('container', {
  state: () => ({ sessions: {} as Record<string, ContainerSession> }),
  actions: {
    async open(tab: ContainerTab, creds?: ContainerConnectCreds) {
      this.close(tab.id)
      this.sessions[tab.id] = {
        connId: '', runtime: tab.runtime,
        containers: [], images: [], namespaces: [], namespace: 'default',
        loading: true, refreshing: false, imagesLoading: false, error: '',
        info: null,
      }
      // 读回响应式代理再操作：直接改闭包里的原始对象不会触发视图更新
      const s = this.sessions[tab.id]
      // 用户在凭据补全弹窗里点了取消：不发起连接，展示取消状态
      if (creds?.error) {
        s.error = creds.error
        s.loading = false
        return
      }
      try {
        const handle = await client.connect(tab.connectionId, creds)
        if (this.sessions[tab.id] !== s) {
          // 连接期间 tab 已关闭：回收后端连接
          await client.disconnect(handle)
          return
        }
        s.connId = handle
        // 连接成功即结束"连接中"状态，列表各自异步加载：
        // 大节点上 docker ps -a 可能要几十秒，不能让整页卡在加载屏。
        s.loading = false
        if (tab.runtime === 'nerdctl') {
          this.loadNamespaces(tab.id)
        }
        await this.refresh(tab.id)
      } catch (e: any) {
        s.error = e?.message || String(e)
      } finally {
        s.loading = false
      }
    },
    async refresh(tabId: string) {
      const s = this.sessions[tabId]
      if (!s || s.refreshing) return // 防止手动连点叠加并发 SSH 会话
      s.refreshing = true
      try {
        s.containers = await client.list(s.connId)
        s.error = ''
      } catch (e: any) {
        s.error = e?.message || String(e)
      } finally {
        s.refreshing = false
      }
    },
    async action(tabId: string, cid: string, act: string) {
      const s = this.sessions[tabId]
      if (!s) return
      await client.action(s.connId, cid, act)
      await this.refresh(tabId)
    },
    async rename(tabId: string, cid: string, name: string) {
      const s = this.sessions[tabId]
      if (!s) return
      await client.rename(s.connId, cid, name)
      await this.refresh(tabId)
    },
    async loadDetail(tabId: string, cid: string): Promise<InspectResult | null> {
      const s = this.sessions[tabId]
      if (!s) return null
      return await client.inspect(s.connId, cid)
    },
    async loadImages(tabId: string) {
      const s = this.sessions[tabId]
      if (!s) return
      s.imagesLoading = true
      try {
        s.images = await client.images(s.connId)
      } finally {
        s.imagesLoading = false
      }
    },
    async loadInfo(tabId: string) {
      const s = this.sessions[tabId]
      if (!s) return
      try {
        s.info = await client.runtimeInfo(s.connId)
      } catch {
        s.info = null
      }
    },
    async loadImageInspect(tabId: string, imageId: string): Promise<string | null> {
      const s = this.sessions[tabId]
      if (!s) return null
      return await client.imageInspect(s.connId, imageId)
    },
    async loadImageHistory(tabId: string, imageId: string): Promise<ImageLayer[]> {
      const s = this.sessions[tabId]
      if (!s) return []
      return await client.imageHistory(s.connId, imageId)
    },
    async loadNamespaces(tabId: string) {
      const s = this.sessions[tabId]
      if (!s) return
      try {
        s.namespaces = await client.namespaces(s.connId)
      } catch {
        s.namespaces = []
      }
    },
    async setNamespace(tabId: string, ns: string) {
      const s = this.sessions[tabId]
      if (!s) return
      s.namespace = ns
      await client.setNamespace(s.connId, ns)
      s.refreshing = false
      await this.refresh(tabId)
    },
    async createContainer(tabId: string, opts: Parameters<typeof client.create>[1]) {
      const s = this.sessions[tabId]
      if (!s) return
      await client.create(s.connId, opts)
      await this.refresh(tabId)
    },
    // Mirrors K8sTabContent.openTerminal: exec params go on the panel config so
    // Panel.vue / TabItem.vue can redial the stream on reconnect / duplicate.
    async openContainerExec(tab: ContainerTab, c: ContainerInfo, shell = 'sh') {
      const connId = this.sessions[tab.id]?.connId
      if (!connId) return
      const info = await client.execSession(connId, c.id, shell)
      const cfg = {
        id: '', name: c.name, type: 'container-exec' as any, host: '', port: 0, user: '', authType: 'password' as any,
        containerExecConnId: connId, containerExecContainerId: c.id, containerExecShell: shell,
      }
      const panelStore = usePanelStore()
      const tabStore = useTabStore()
      const sessionStore = useSessionStore()
      const panel = panelStore.createPanel(cfg as any, 'container-exec')
      panelStore.updateTitle(panel.id, c.name)
      panelStore.bindSession(panel.id, info.id)
      sessionStore.initSession(info.id)
      sessionStore.updateStatus(info.id, 'connected')
      const termTab = tabStore.createTerminalTab(panel.title, panel.id)
      panelStore.movePanelToTab(panel.id, termTab.id)
    },
    // 打开容器文件浏览器：container-file 会话走 CreateSession（App 层拦截），
    // 面板/标签复用协议无关的 'sftp' 双栏文件浏览器（同 wsl-file 模式）。
    async openContainerFiles(tab: ContainerTab, c: ContainerInfo) {
      const connId = this.sessions[tab.id]?.connId
      if (!connId) return
      const { launchContainerFileBrowser } = await import('../composables/connectionLauncher')
      const config = {
        id: connId, name: c.name, type: 'container-file' as any, host: '', port: 0, user: '', authType: 'password' as any,
        containerId: c.id,
      }
      await launchContainerFileBrowser(config as any, { persist: false })
    },
    close(tabId: string) {
      const s = this.sessions[tabId]
      if (!s) return
      if (s.connId) void client.disconnect(s.connId)
      delete this.sessions[tabId]
    },
  },
})
