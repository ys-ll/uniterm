<template>
  <el-drawer
    v-model="visible"
    :title="title"
    direction="rtl"
    size="60%"
    :with-header="true"
    :append-to-body="true"
  >
    <template #header>
      <div class="k8s-yaml-drawer-head">
        <span class="k8s-yaml-drawer-title">{{ title }}</span>
        <el-button size="small" @click="copy" :icon="CopyDocument">Copy</el-button>
      </div>
    </template>
    <pre class="k8s-yaml-drawer-body">{{ yamlText }}</pre>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElDrawer, ElButton, ElMessage } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'
import { dump } from 'js-yaml'

const props = defineProps<{ obj: any | null }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const visible = ref(false)

// 打开：外部把 obj 从 null 变成对象；关闭：obj 变回 null。
watch(() => props.obj, (v) => { visible.value = !!v })
watch(visible, (v) => { if (!v) emit('close') })

const title = computed(() => {
  const o = props.obj
  if (!o) return ''
  const ns = o.metadata?.namespace || 'cluster'
  const kind = o.kind || '?'
  const name = o.metadata?.name || ''
  return `${kind} / ${ns} / ${name}`
})

const yamlText = computed(() => {
  if (!props.obj) return ''
  try {
    return dump(props.obj, { sortKeys: false, lineWidth: 120 })
  } catch (e: any) {
    return `# yaml.dump failed: ${e?.message || e}\n\n${JSON.stringify(props.obj, null, 2)}`
  }
})

async function copy() {
  try {
    await navigator.clipboard.writeText(yamlText.value)
    ElMessage.success('Copied')
  } catch (e: any) {
    ElMessage.error(`Copy failed: ${e?.message || e}`)
  }
}
</script>

<style scoped>
.k8s-yaml-drawer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  gap: 8px;
}
.k8s-yaml-drawer-title {
  font-weight: 600;
  font-family: var(--font-mono, monospace);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.k8s-yaml-drawer-body {
  margin: 0;
  padding: 12px;
  overflow: auto;
  font-family: var(--font-mono, monospace);
  font-size: 12px;
  white-space: pre-wrap;
  height: 100%;
  box-sizing: border-box;
}
</style>
