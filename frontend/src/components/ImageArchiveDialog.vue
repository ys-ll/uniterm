<template>
  <el-dialog append-to-body v-model="visible" :title="t('container.loadArchive')" width="34rem" :close-on-click-modal="false">
    <p class="operation-hint">{{ t('container.archiveHint') }}</p>
    <el-form label-width="7rem" @submit.prevent="submit">
      <el-form-item :label="t('container.archiveFile')" required>
        <div class="archive-path">
          <el-input v-model="archivePath" :disabled="busy" :placeholder="t('container.archivePathHint')" />
          <el-button :disabled="busy" @click="browse">{{ t('container.chooseArchive') }}</el-button>
        </div>
      </el-form-item>
    </el-form>
    <p v-if="busy" role="status">{{ t('container.loadingArchive') }}</p>
    <p v-if="error" class="operation-error" role="alert">{{ error }}</p>
    <p v-if="busy" class="operation-hint">{{ t('container.imageCancelHint') }}</p>
    <template #footer>
      <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="busy" :disabled="busy || !archivePath.trim()" @click="submit">{{ t('container.loadArchive') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { OpenFileDialogFiltered } from '../../bindings/github.com/ys-ll/uniterm/app'
import { isWebServerMode, pickAndStageLocalFileInBrowser } from '../services/webSftpBridge'
import { useI18n } from '../i18n'
import { loadImage } from '../services/containerClient'
import { backendErrorText } from '../utils/backendError'
import { useContainerImageOperation } from '../composables/useContainerImageOperation'

const props = defineProps<{ modelValue: boolean; connId: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void; (e: 'loaded'): void }>()
const { t } = useI18n()
const { busy, error, run, cancel } = useContainerImageOperation()
const archivePath = ref('')
const visible = computed({ get: () => props.modelValue, set: value => { if (!value) cancel(); emit('update:modelValue', value) } })
watch(() => props.modelValue, open => {
  if (open) { archivePath.value = ''; error.value = '' }
  else cancel()
})
watch(() => props.connId, () => { cancel(); visible.value = false })
onBeforeUnmount(cancel)

async function browse() {
  try {
    // Web mode: stage the archive into the worker; loadImage consumes the
    // container path immediately.
    if (await isWebServerMode()) {
      const staged = await pickAndStageLocalFileInBrowser('.tar,.gz,.tgz,.bz2,.tbz2,.xz,.txz,.zst,.tzst')
      if (staged) archivePath.value = staged.stagedPath
      return
    }
    const path = await OpenFileDialogFiltered(t('container.chooseArchive'), 'Image archive', '*.tar;*.tar.gz;*.tgz;*.tar.bz2;*.tbz2;*.tar.xz;*.txz;*.tar.zst;*.tzst')
    if (path) archivePath.value = path
  } catch (e) { error.value = backendErrorText(e) }
}

async function submit() {
  const path = archivePath.value.trim()
  if (!path || !props.connId || busy.value) return
  if (await run(props.connId, id => loadImage(id, path))) {
    ElMessage.success(t('container.archiveLoaded'))
    emit('loaded')
    visible.value = false
  }
}
</script>

<style scoped>
.archive-path { display: flex; gap: 8px; width: 100%; }
.operation-hint { color: var(--text-muted); font-size: 12px; line-height: 1.5; }
.operation-error { color: var(--el-color-danger); overflow-wrap: anywhere; }
</style>
