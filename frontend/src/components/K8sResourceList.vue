<template>
  <div class="k8s-list-wrap">
    <div class="k8s-list-toolbar">
      <el-select
        v-if="desc?.namespaced"
        v-model="localNs"
        size="small"
        class="k8s-ns-select"
        placeholder="all namespaces"
        clearable
      >
        <el-option label="all namespaces" value="" />
        <el-option v-for="opt in namespaceOptions" :key="opt" :label="opt" :value="opt" />
      </el-select>
      <span v-else class="k8s-ns-placeholder">cluster-scoped</span>

      <el-input
        v-model="filter"
        size="small"
        placeholder="filter"
        clearable
        class="k8s-filter"
      />

      <el-button size="small" :icon="Refresh" @click="onRefresh" />

      <span class="k8s-list-title">
        {{ desc?.label || resourceKey }} ({{ items.length }})
      </span>
    </div>

    <div v-if="listError" class="k8s-list-err">{{ listError }}</div>

    <el-table
      :data="filtered"
      size="small"
      height="calc(100% - 40px)"
      class="k8s-list-table"
      @row-click="onRowClick"
    >
      <el-table-column
        v-for="col in desc?.columns || []"
        :key="col.header"
        :label="col.header"
        :width="col.width"
        sortable
      >
        <template #default="{ row }">{{ cellText(col.value(row)) }}</template>
      </el-table-column>
    </el-table>

    <div v-if="!items.length && !listError" class="k8s-list-empty">
      No {{ desc?.label || resourceKey }}<template v-if="desc?.namespaced"> in namespace <code>{{ localNs || '(all)' }}</code></template>.
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount } from 'vue'
import { ElTable, ElTableColumn, ElInput, ElButton, ElSelect, ElOption } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { useK8sStore } from '../stores/k8sStore'
import { getResource, type ColoredCell } from '../services/k8sResources'

const props = defineProps<{
  connId: string
  resourceKey: string
  initialNamespace: string
  namespaceOptions: string[]
}>()
const emit = defineEmits<{ (e: 'row-click', obj: any): void }>()

const store = useK8sStore()

const desc = computed(() => getResource(props.resourceKey))
const localNs = ref(props.initialNamespace || '')
const filter = ref('')

const items = computed(() => store.getItems(props.connId, props.resourceKey, localNs.value))
const listError = computed(() => store.getError(props.connId, props.resourceKey, localNs.value))

const filtered = computed(() => {
  const f = filter.value.trim().toLowerCase()
  if (!f) return items.value
  return items.value.filter(o => (o.metadata?.name || '').toLowerCase().includes(f))
})

function cellText(v: string | number | ColoredCell): string {
  if (v == null) return ''
  if (typeof v === 'object' && 'text' in v) return v.text
  return String(v)
}

function onRowClick(row: any) {
  emit('row-click', row)
}

async function subscribeCurrent() {
  if (!props.connId) return
  await store.subscribe(props.connId, props.resourceKey, localNs.value)
}

function unsubscribeCurrent(oldRes: string, oldNs: string) {
  if (!props.connId) return
  store.unsubscribe(props.connId, oldRes, oldNs)
}

// resourceKey 切换：卸旧订新
watch(() => props.resourceKey, async (newRes, oldRes) => {
  unsubscribeCurrent(oldRes, localNs.value)
  await store.subscribe(props.connId, newRes, localNs.value)
})

// namespace 切换：卸旧订新（同一 resource）
watch(localNs, async (newNs, oldNs) => {
  unsubscribeCurrent(props.resourceKey, oldNs)
  await store.subscribe(props.connId, props.resourceKey, newNs)
})

// connId 就绪：首次订阅
watch(() => props.connId, async (v) => {
  if (v) await subscribeCurrent()
}, { immediate: true })

async function onRefresh() {
  if (!props.connId) return
  store.unsubscribe(props.connId, props.resourceKey, localNs.value)
  await store.subscribe(props.connId, props.resourceKey, localNs.value)
}

onBeforeUnmount(() => {
  unsubscribeCurrent(props.resourceKey, localNs.value)
})
</script>

<style scoped>
.k8s-list-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  height: 100%;
}
.k8s-list-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-bottom: 1px solid var(--el-border-color-lighter, #333);
  flex-shrink: 0;
}
.k8s-ns-select {
  width: 180px;
}
.k8s-ns-placeholder {
  color: var(--text-secondary, #888);
  font-size: 12px;
  padding: 0 4px;
}
.k8s-filter {
  width: 220px;
}
.k8s-list-title {
  margin-left: auto;
  color: var(--text-secondary, #888);
  font-size: 12px;
}
.k8s-list-table {
  flex: 1;
}
.k8s-list-err {
  color: var(--el-color-danger, #f56);
  padding: 8px 12px;
  font-size: 12px;
  border-bottom: 1px solid var(--el-border-color-lighter, #333);
}
.k8s-list-empty {
  padding: 24px;
  text-align: center;
  opacity: 0.55;
  font-size: 13px;
}
.k8s-list-empty code {
  padding: 1px 6px;
  background: rgba(255,255,255,0.06);
  border-radius: 3px;
  font-family: var(--font-mono, monospace);
}
</style>
