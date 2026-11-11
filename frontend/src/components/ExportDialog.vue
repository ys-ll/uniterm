<template>
  <el-dialog append-to-body
    :model-value="visible"
    @update:model-value="(v: boolean) => !v && onCancel()"
    :title="t('importExport.exportTitle')"
    width="27.5rem"
  >
    <el-form label-width="6.25rem">
      <el-form-item :label="t('importExport.configPassword')">
        <el-input v-model="password" type="password" show-password />
      </el-form-item>
      <el-form-item :label="t('importExport.confirmPassword')">
        <el-input v-model="confirmPassword" type="password" show-password />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="onCancel">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="exporting" @click="onExport">{{ t('importExport.export') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from '../i18n'
import { msg } from '../services/message'
import { ExportConnections, ExportConnectionsData, SaveFileDialogFiltered } from '../../bindings/github.com/ys-ll/uniterm/app'
import { androidSaveAvailable, finishAndroidSave, stageAndroidSave } from '../utils/androidSave'
import { isWebServerMode, saveBase64FileInBrowser } from '../services/webSftpBridge'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'done'): void }>()

const { t } = useI18n()
const password = ref('')
const confirmPassword = ref('')
const exporting = ref(false)

watch(() => props.visible, (v) => {
  if (v) { password.value = ''; confirmPassword.value = '' }
})

function onCancel() { emit('update:visible', false) }

async function onExport() {
  if (!password.value) { msg.error(t('importExport.configPassword')); return }
  if (password.value !== confirmPassword.value) { msg.error(t('importExport.passwordMismatch')); return }
  // Android has no save dialog — stage the export in the sandbox and hand it
  // to the system "save to" picker (see androidSave.ts).
  const viaAndroid = androidSaveAvailable()
  exporting.value = true
  try {
    if (!viaAndroid && await isWebServerMode()) {
      // Web-server mode: no native save dialog exists — render the .utm in
      // memory and hand it to the browser's download manager instead.
      const b64 = await ExportConnectionsData(password.value)
      saveBase64FileInBrowser('uniterm-connections.utm', 'application/octet-stream', b64)
      msg.success(t('importExport.exported'))
      emit('done')
      emit('update:visible', false)
      return
    }
    let dest: string
    if (viaAndroid) {
      dest = await stageAndroidSave('uniterm-connections.utm')
    } else {
      dest = await SaveFileDialogFiltered(t('importExport.exportTitle'), 'uniterm-connections.utm', 'uniTerm (*.utm)', '*.utm')
      if (!dest) return
    }
    await ExportConnections(dest, password.value)
    if (viaAndroid) {
      const outcome = await finishAndroidSave(dest, 'uniterm-connections.utm', 'application/octet-stream')
      if (outcome !== 'saved') return
    }
    msg.success(t('importExport.exported'))
    emit('done')
    emit('update:visible', false)
  } catch (e: any) {
    msg.error(e?.message || String(e))
  } finally {
    exporting.value = false
  }
}
</script>
