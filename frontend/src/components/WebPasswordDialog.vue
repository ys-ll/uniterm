<template>
  <el-dialog
    append-to-body
    :model-value="visible"
    @update:model-value="(v: boolean) => !v && close()"
    :title="t('webPassword.title')"
    width="30rem"
    :close-on-click-modal="false"
  >
    <el-form label-width="7.5rem" @submit.prevent="submit">
      <el-form-item :label="t('webPassword.current')">
        <el-input v-model="current" type="password" show-password autocomplete="current-password" />
      </el-form-item>
      <el-form-item :label="t('webPassword.new')">
        <el-input v-model="next" type="password" show-password autocomplete="new-password" />
      </el-form-item>
      <el-form-item :label="t('webPassword.confirm')">
        <el-input v-model="confirm" type="password" show-password autocomplete="new-password" />
      </el-form-item>
    </el-form>
    <div v-if="error" class="form-error">{{ error }}</div>
    <template #footer>
      <el-button @click="close">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">{{ t('common.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from '../i18n'
import { msg } from '../services/message'
import { changeWebPassword } from '../services/webSftpBridge'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

const { t } = useI18n()
const current = ref('')
const next = ref('')
const confirm = ref('')
const error = ref('')
const submitting = ref(false)

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    current.value = ''
    next.value = ''
    confirm.value = ''
    error.value = ''
  }
)

function close() {
  emit('update:visible', false)
}

async function submit() {
  error.value = ''
  if (next.value.length < 6) {
    error.value = t('webPassword.tooShort')
    return
  }
  if (next.value !== confirm.value) {
    error.value = t('webPassword.mismatch')
    return
  }
  submitting.value = true
  try {
    await changeWebPassword(current.value, next.value)
    msg.success(t('webPassword.success'))
    close()
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.form-error {
  color: var(--el-color-danger);
  font-size: 0.8125rem;
  margin-top: 0.5rem;
}
</style>