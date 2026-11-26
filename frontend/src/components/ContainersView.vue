<template>
  <div class="view-wrap">
    <div class="container-toolbar">
      <slot name="toggle" />
      <el-select
        v-if="session?.runtime === 'nerdctl'"
        :model-value="session.namespace"
        size="small"
        style="width: 8.75rem"
        filterable
        allow-create
        @change="onNamespaceChange"
      >
        <el-option v-for="ns in namespaceOptions" :key="ns" :label="ns" :value="ns" />
      </el-select>
      <el-tag
        v-if="imageFilter"
        closable
        size="small"
        class="image-filter-tag"
        @close="$emit('clear-image-filter')"
      >{{ t('container.imageFilter', { name: imageFilter }) }}</el-tag>
      <el-tag
        v-if="stateFilter"
        closable
        size="small"
        class="image-filter-tag"
        @close="$emit('clear-state-filter')"
      >{{ t('container.colState') }}: {{ stateFilter }}</el-tag>
      <el-input
        v-model="filter"
        size="small"
        :placeholder="t('k8s.filter')"
        clearable
        style="width: 12.5rem"
      />
      <el-checkbox v-model="runningOnly" size="small" border>{{ t('container.running') }}</el-checkbox>
      <div class="toolbar-spacer" />
      <el-button size="small" @click="createOpen = true">{{ t('container.create') }}</el-button>
      <el-button size="small" :icon="RefreshCw" @click="store.refresh(tab.id)" />
    </div>

    <div class="container-table-wrap">
      <el-table
        v-loading="!session?.containers.length && !!session?.refreshing"
        :data="filteredContainers"
        size="small"
        height="calc(100% - 2.5rem)"
        class="k8s-list-table"
        border
        @row-click="openDetail"
      >
        <el-table-column :label="t('container.colName')" :min-width="uiPx(180)" sortable :sort-method="(a, b) => a.name.localeCompare(b.name)" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('container.colImage')" :min-width="uiPx(220)" sortable :sort-method="(a, b) => a.image.localeCompare(b.image)" show-overflow-tooltip>
          <template #default="{ row }">{{ row.image }}</template>
        </el-table-column>
        <el-table-column :label="t('container.colId')" :width="uiPx(120)">
          <template #default="{ row }">{{ shortId(row.id) }}</template>
        </el-table-column>
        <el-table-column
          :label="t('container.colState')"
          :width="uiPx(110)"
          sortable
          :sort-method="(a, b) => a.state.localeCompare(b.state)"
          :filters="stateFilters"
          :filter-method="(val, row) => row.state === val"
        >
          <template #default="{ row }">
            <span :data-state="row.state" class="container-state">{{ row.state }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('container.colPorts')" :min-width="uiPx(160)" sortable :sort-method="(a, b) => a.ports.localeCompare(b.ports)" show-overflow-tooltip>
          <template #default="{ row }">{{ row.ports }}</template>
        </el-table-column>
        <el-table-column :label="t('container.colCreated')" :min-width="uiPx(130)" sortable :sort-method="(a, b) => a.createdAt.localeCompare(b.createdAt)" show-overflow-tooltip>
          <template #default="{ row }">{{ row.createdAt }}</template>
        </el-table-column>
        <el-table-column :label="t('container.colActions')" :width="uiPx(150)" fixed="right" class-name="k8s-action-cell">
          <template #default="{ row }">
            <button class="btn btn-ghost btn-icon btn-sm" :title="t('container.exec')" :disabled="row.state !== 'running'" @click.stop="openExec(row)">
              <SquareTerminal :size="'0.875rem'" />
            </button>
            <button class="btn btn-ghost btn-icon btn-sm" :title="t('container.openFiles')" :disabled="row.state !== 'running'" @click.stop="openFiles(row)">
              <FolderOpen :size="'0.875rem'" />
            </button>
            <button class="btn btn-ghost btn-icon btn-sm" :title="t('container.logs')" @click.stop="openLogs(row)">
              <ScrollText :size="'0.875rem'" />
            </button>
            <button v-if="row.state !== 'running'" class="btn btn-ghost btn-icon btn-sm" :title="t('container.start')" @click.stop="runAction(row, 'start')">
              <Play :size="'0.875rem'" />
            </button>
            <button v-if="row.state === 'running'" class="btn btn-ghost btn-icon btn-sm" :title="t('container.stop')" @click.stop="runAction(row, 'stop')">
              <Square :size="'0.875rem'" />
            </button>
            <button class="btn btn-ghost btn-icon btn-sm" :title="t('common.more')" @click.stop="moreMenuRef?.toggle($event.currentTarget as HTMLElement, row)">
              <Ellipsis :size="'0.875rem'" />
            </button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <ContainerDetailDrawer
      :mode="drawerMode"
      :tab-id="tab.id"
      :target="drawerTarget"
      @close="drawerMode = null"
    />
    <ContainerCommitDialog v-model="commitOpen" :conn-id="session?.connId || ''" :target="commitTarget" @committed="store.loadImages(tab.id); store.refresh(tab.id)" />
    <ContainerCreateDialog v-model="createOpen" :tab-id="tab.id" @created="store.refresh(tab.id)" />

    <!-- 共享的行级"更多"菜单：单个实例，行数据经 slot current 传入 -->
    <Menu ref="moreMenuRef" v-model:visible="moreVisible">
      <template #default="{ current }">
        <MenuItem v-if="session?.runtime !== 'wslc'" :icon="Power" iconic @click="onMoreCommand('restart', current as ContainerInfo)">
          {{ t('container.restart') }}
        </MenuItem>
        <MenuItem v-if="session?.runtime !== 'wslc'" :icon="Pencil" iconic @click="onMoreCommand('rename', current as ContainerInfo)">
          {{ t('container.rename') }}
        </MenuItem>
        <MenuItem v-if="session?.runtime === 'docker'" :icon="Camera" iconic @click="onMoreCommand('commit', current as ContainerInfo)">{{ t('container.commitImage') }}</MenuItem>
        <MenuItem :icon="Trash2" iconic class="danger" @click="onMoreCommand('remove', current as ContainerInfo)">
          {{ t('container.remove') }}
        </MenuItem>
      </template>
    </Menu>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { RefreshCw, SquareTerminal, FolderOpen, ScrollText, Play, Square, Power, Pencil, Trash2, Ellipsis, Camera } from '@lucide/vue'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import { useContainerStore } from '../stores/containerStore'
import { useI18n } from '../i18n'
import ContainerDetailDrawer from './ContainerDetailDrawer.vue'
import ContainerCreateDialog from './ContainerCreateDialog.vue'
import ContainerCommitDialog from './ContainerCommitDialog.vue'
import type { ContainerSession } from '../stores/containerStore'
import type { ContainerInfo, ContainerTab } from '../types/container'
import { uiPx } from '../utils/uiScale'

const props = defineProps<{
  tab: ContainerTab
  session: ContainerSession | undefined
  imageFilter: string
  stateFilter: string
}>()
defineEmits<{ (e: 'clear-image-filter'): void; (e: 'clear-state-filter'): void }>()

const { t } = useI18n()
const store = useContainerStore()

const namespaceOptions = computed(() => {
  const s = props.session
  if (!s) return []
  const set = new Set(s.namespaces.length ? s.namespaces : [s.namespace])
  set.add('default')
  return [...set]
})

async function onNamespaceChange(ns: string) {
  try {
    await store.setNamespace(props.tab.id, ns)
  } catch (e: any) {
    ElMessage.error(String(e?.message || e))
  }
}

function shortId(id: string) {
  const raw = id.replace(/^sha256:/, '')
  return raw.length > 12 ? raw.slice(0, 12) : raw
}

// ── search / filter ────────────────────────────────────────────
const filter = ref('')
// 默认只看运行中的容器：k8s 节点上 -a 会列出大量已退出的 infra 容器
const runningOnly = ref(true)
const filteredContainers = computed(() => {
  const s = props.session
  const nameFilter = filter.value.trim().toLowerCase()
  const imgFilter = props.imageFilter
  const stFilter = props.stateFilter
  let list = s?.containers || []
  if (runningOnly.value) list = list.filter(c => c.state === 'running')
  if (imgFilter) list = list.filter(c => matchesImage(c, imgFilter))
  if (stFilter) list = list.filter(c => c.state === stFilter)
  if (nameFilter) list = list.filter(c =>
    c.name.toLowerCase().includes(nameFilter) ||
    c.image.toLowerCase().includes(nameFilter) ||
    c.id.toLowerCase().includes(nameFilter))
  return list
})
const stateFilters = computed(() => {
  const set = new Set<string>()
  for (const c of props.session?.containers || []) set.add(c.state)
  return [...set].filter(Boolean).sort().map(v => ({ text: v, value: v }))
})

// 关联容器过滤：ps 输出的 image 列是 "repo:tag"，镜像未打 tag/悬空时是
// image ID（可能带 sha256: 前缀）。名称与 ID 两种形态都匹配。
function matchesImage(c: ContainerInfo, ref: string) {
  if (c.image === ref) return true
  const id = ref.replace(/^sha256:/, '')
  if (id && (c.image === id || c.image.replace(/^sha256:/, '').startsWith(id))) return true
  return false
}

// ── drawer / create ────────────────────────────────────────────
const drawerMode = ref<'detail' | 'logs' | null>(null)
const drawerTarget = ref<ContainerInfo | null>(null)
const createOpen = ref(false)
const commitOpen = ref(false)
const commitTarget = ref<ContainerInfo | null>(null)

function openDetail(c: ContainerInfo) {
  drawerMode.value = 'detail'
  drawerTarget.value = c
}
function openLogs(c: ContainerInfo) {
  drawerTarget.value = c
  drawerMode.value = 'logs'
}
async function openExec(c: ContainerInfo) {
  try {
    await store.openContainerExec(props.tab, c)
  } catch (e: any) {
    ElMessage.error(String(e?.message || e))
  }
}

async function openFiles(c: ContainerInfo) {
  try {
    await store.openContainerFiles(props.tab, c)
  } catch (e: any) {
    ElMessage.error(String(e?.message || e))
  }
}

// ── 更多菜单（低频操作收纳；单实例共享，行数据经 slot current 传入）──
const moreMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const moreVisible = ref(false)

function onMoreCommand(cmd: string, c: ContainerInfo) {
  moreMenuRef.value?.close()
  switch (cmd) {
    case 'commit':
      commitTarget.value = c
      commitOpen.value = true
      break
    case 'restart':
      void runAction(c, 'restart')
      break
    case 'rename':
      void onRename(c)
      break
    case 'remove':
      void onRemove(c)
      break
  }
}

// ── actions ────────────────────────────────────────────────────
async function runAction(c: ContainerInfo, act: string) {
  try {
    if (act === 'start' || act === 'stop' || act === 'restart') {
      await ElMessageBox.confirm(
        t('container.actionConfirm', { action: t('container.' + act), name: c.name }),
        t('common.confirm'),
        { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
      )
    }
    await store.action(props.tab.id, c.id, act)
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(String(e?.message || e))
  }
}

async function onRename(c: ContainerInfo) {
  try {
    const { value } = await ElMessageBox.prompt(
      t('container.renameMessage', { name: c.name }),
      t('container.rename'),
      { inputValue: c.name, confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
    if (value && value !== c.name) await store.rename(props.tab.id, c.id, value)
  } catch (e: any) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(String(e?.message || e))
  }
}

async function onRemove(c: ContainerInfo) {
  try {
    await ElMessageBox.confirm(
      t('container.removeConfirm', { name: c.name }),
      t('common.confirm'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
    await store.action(props.tab.id, c.id, 'rm')
  } catch (e: any) {
    if (e !== 'cancel' && e !== 'close') ElMessage.error(String(e?.message || e))
  }
}
</script>

<style scoped>
.view-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.container-toolbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.625rem;
  border-bottom: 1px solid var(--el-border-color-lighter, #333);
  flex-shrink: 0;
}

.toolbar-spacer {
  flex: 1;
}

.image-filter-tag {
  max-width: 16rem;
}

.container-table-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
}

.k8s-list-table {
  flex: 1;
  /* Floats inside the island pane now: keep the top divider only, and let
     the bottom line back now that the pane no longer supplies one */
  border-radius: 0;
}
.k8s-list-table::before,
.k8s-list-table::after {
  display: none;
}
.k8s-list-table.el-table--border {
  border-left: none;
  border-right: none;
}

/* Action-column cell: tighter padding + 0.25rem gap between .btn-icon buttons
   (mirrors K8sResourceList's action-cell styling). */
.k8s-list-table :deep(.k8s-action-cell .cell) {
  padding: 0 0.25rem;
  white-space: nowrap;
  display: flex;
  align-items: center;
}
.k8s-list-table :deep(.k8s-action-cell .btn-icon + .btn-icon) {
  margin-left: 0.25rem;
}

.k8s-list-table :deep(.el-table__row) {
  cursor: pointer;
}

.container-state[data-state='running'] {
  color: var(--el-color-success);
}

.container-state[data-state='paused'] {
  color: var(--el-color-warning);
}
</style>
