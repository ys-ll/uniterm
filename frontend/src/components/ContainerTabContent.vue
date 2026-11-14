<template>
  <div class="container-tab">
    <div v-if="session?.error" class="container-error">{{ session.error }}</div>
    <div v-else-if="!session || session.loading" class="container-loading">{{ t('container.loading') }}</div>

    <div v-else class="db-main" :class="{ 'm-tree-host': isNarrowScreen }">
      <div v-if="isNarrowScreen && treeOpen" class="m-tree-overlay" @click="treeOpen = false" />
      <div
        v-show="isNarrowScreen || treeOpen"
        class="db-left"
        :class="{ 'm-tree': isNarrowScreen, 'm-tree-open': isNarrowScreen && treeOpen }"
        :style="isNarrowScreen ? {} : { width: leftWidth + 'px' }"
      >
        <ContainerTree v-model="view" />
      </div>
      <div v-if="!isNarrowScreen && treeOpen" class="db-resizer" @mousedown="onResizeStart" />
      <div class="db-right">
        <OverviewView
          v-if="view === 'overview'"
          :tab="tab"
          :session="session"
          @show-state="onShowState"
          @show-images="view = 'images'"
        >
          <template #toggle>
            <button
              class="btn btn-ghost btn-icon btn-sm m-tree-toggle"
              :title="treeOpen ? t('common.collapse') : t('common.expand')"
              @click="treeOpen = !treeOpen"
            >
              <PanelLeft :size="'0.875rem'" />
            </button>
          </template>
        </OverviewView>
        <!-- v-show 常驻：切换视图不销毁组件，进行中的拉取/推送任务面板跨视图存活 -->
        <ContainersView
          v-show="view === 'containers'"
          :tab="tab"
          :session="session"
          :image-filter="imageFilter"
          :state-filter="stateFilter"
          @clear-image-filter="imageFilter = ''"
          @clear-state-filter="stateFilter = ''"
        >
          <template #toggle>
            <button
              class="btn btn-ghost btn-icon btn-sm m-tree-toggle"
              :title="treeOpen ? t('common.collapse') : t('common.expand')"
              @click="treeOpen = !treeOpen"
            >
              <PanelLeft :size="'0.875rem'" />
            </button>
          </template>
        </ContainersView>
        <ImagesView
          v-show="view === 'images'"
          :tab="tab"
          :session="session"
          @view-related="onViewRelated"
        >
          <template #toggle>
            <button
              class="btn btn-ghost btn-icon btn-sm m-tree-toggle"
              :title="treeOpen ? t('common.collapse') : t('common.expand')"
              @click="treeOpen = !treeOpen"
            >
              <PanelLeft :size="'0.875rem'" />
            </button>
          </template>
        </ImagesView>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { PanelLeft } from '@lucide/vue'
import { isNarrowScreen } from '../utils/platform'
import { useContainerStore } from '../stores/containerStore'
import { useConnectionStore } from '../stores/connectionStore'
import { useTunnelCredentials } from '../composables/useTunnelCredentials'
import { useI18n } from '../i18n'
import ContainerTree from './ContainerTree.vue'
import OverviewView from './OverviewView.vue'
import ContainersView from './ContainersView.vue'
import ImagesView from './ImagesView.vue'
import type { ContainerTab } from '../types/container'

const props = defineProps<{ tab: ContainerTab }>()

const { t } = useI18n()
const store = useContainerStore()
const connectionStore = useConnectionStore()
const { ensureConnectionCredentials } = useTunnelCredentials()
const session = computed(() => store.sessions[props.tab.id])

// SSH 传输的容器连接先走统一的凭据补全（含跳板机链），被引用的 SSH 主机
// 未保存密码时弹窗补全而不是直接报认证错误；取消则中止连接。无需补全时
// ensureConnectionCredentials 原样返回，不影响正常连接。
async function openWithCredentials() {
  const conn = connectionStore.connections.find(c => c.id === props.tab.connectionId)
  if (conn && conn.containerTransport === 'ssh' && conn.containerSSHConnId) {
    const sshCfg = connectionStore.connections.find(c => c.id === conn.containerSSHConnId)
    if (sshCfg) {
      const resolved = await ensureConnectionCredentials(sshCfg)
      if (!resolved) {
        await store.open(props.tab, { error: t('credential.cancelled') })
        return
      }
      await store.open(props.tab, {
        sshUser: resolved.user,
        sshPassword: resolved.password,
        tunnelUser: resolved.tunnelSSHUser,
        tunnelPassword: resolved.tunnelSSHPassword,
      })
      return
    }
  }
  await store.open(props.tab)
}

// ── view switch（左树：总览分组 + 概览/容器/镜像）──────────────
const view = ref<'overview' | 'containers' | 'images'>('overview')
const imageFilter = ref('')
const stateFilter = ref('')

// Narrow screens turn the tree into an overlay drawer (closed by default);
// wide screens keep it inline and the toggle collapses it.
const treeOpen = ref(!isNarrowScreen.value)
watch(view, () => {
  if (isNarrowScreen.value) treeOpen.value = false
})

// 镜像列表「关联容器」点击：切到容器页并按镜像过滤。
function onViewRelated(ref: string) {
  imageFilter.value = ref
  stateFilter.value = ''
  view.value = 'containers'
}

// 概览页状态统计点击：切到容器页并按状态过滤。
function onShowState(state: string) {
  stateFilter.value = state
  imageFilter.value = ''
  view.value = 'containers'
}

// ── 左侧宽度 + resizer（抄 K8sTabContent / DBTabContent）───────
const leftWidth = ref(180)
let resizeStartX = 0
let resizeStartWidth = 0
let resizing = false
function onResizeStart(e: MouseEvent) {
  // Block the mousedown default action so the drag cannot promote into a text
  // selection / native drag (see AISidebar.onResizeStart for the full story).
  e.preventDefault()
  resizeStartX = e.clientX
  resizeStartWidth = leftWidth.value
  resizing = true
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
}
function onResizeMove(e: MouseEvent) {
  const dx = e.clientX - resizeStartX
  leftWidth.value = Math.max(120, Math.min(360, resizeStartWidth + dx))
}
function onResizeEnd() {
  resizing = false
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
}

function onReconnectEvent(e: Event) {
  const panelId = (e as CustomEvent)?.detail?.panelId
  if (panelId && panelId === props.tab.panelId) reconnect()
}

// Force-reconnect: close the current container connection, then re-open it
// (open() re-initializes loading/error state and reconnects the client).
async function reconnect() {
  try { store.close(props.tab.id) } catch (_) {}
  await openWithCredentials()
}

onMounted(() => {
  openWithCredentials()
  // Tab right-click 「重连」(Reconnect) menu → forced reconnect of this tab.
  window.addEventListener('panel:reconnect', onReconnectEvent)
})
onBeforeUnmount(() => {
  window.removeEventListener('panel:reconnect', onReconnectEvent)
  if (resizing) {
    document.removeEventListener('mousemove', onResizeMove)
    document.removeEventListener('mouseup', onResizeEnd)
  }
  store.close(props.tab.id)
})
</script>

<style scoped>
/* 直接抄 K8sTabContent 的骨架 CSS，class 同名 */
.container-tab {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}

.db-main {
  flex: 1;
  display: flex;
  overflow: hidden;
}
.db-left {
  flex-shrink: 0;
  border-right: 1px solid var(--border-subtle, #333);
  overflow: hidden;
}
.db-resizer {
  /* Hover area floats over the tree edge (negative margin) so it takes
     no layout width when idle and never blanks the content side. */
  width: 0.25rem;
  margin-left: -0.25rem;
  position: relative;
  z-index: 1;
  cursor: col-resize;
  background: transparent;
  flex-shrink: 0;
  transition: background 0.15s ease;
}
.db-resizer:hover::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: 0.1875rem;
  background: var(--accent);
  box-shadow: 0 0 0.375rem var(--accent-glow);
}
.db-right {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.container-error {
  color: var(--el-color-danger, #f56);
  padding: 0.75rem;
  white-space: pre-wrap;
}

.container-loading {
  padding: 0.75rem;
  opacity: 0.7;
}
</style>
