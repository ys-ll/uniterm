<template>
  <!-- Lets a user read the current code for a saved connection without
       reopening the edit form: bastion web consoles require it while binding
       an authenticator (issue #1583). -->
  <el-dialog
    append-to-body
    :model-value="visible"
    :title="t('conn.totpViewCode')"
    width="22rem"
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <div v-if="hostName" class="totp-dialog-host">{{ hostName }}</div>
    <TotpCodePanel :secret="secret" :active="visible" />
    <div class="totp-dialog-hint">{{ t('conn.totpCodeHint') }}</div>
    <template #footer>
      <el-button type="primary" @click="emit('update:visible', false)">{{ t('common.done') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { useI18n } from '../i18n'
import TotpCodePanel from './TotpCodePanel.vue'

defineProps<{
  visible: boolean
  secret: string
  hostName?: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
}>()

const { t } = useI18n()
</script>

<style scoped>
.totp-dialog-host {
  font-weight: 600;
  margin-bottom: 0.5rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.totp-dialog-hint {
  margin-top: 0.75rem;
  color: var(--text-muted);
  font-size: 0.8125rem;
  line-height: 1.5;
}
</style>
