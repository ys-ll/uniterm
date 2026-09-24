<template>
  <el-dialog append-to-body
    :model-value="visible"
    @update:model-value="(v: boolean) => !v && onDeny()"
    :title="t('mcp.approvalTitle')"
    width="32rem"
    :close-on-click-modal="false"
    :show-close="false"
  >
    <div class="mcp-approval">
      <p class="mcp-approval-client">
        {{ t('mcp.approvalClient') }}:
        <el-tag size="small" type="warning">{{ request?.client }}</el-tag>
      </p>
      <p v-if="request?.connection" class="mcp-approval-conn">
        {{ t('mcp.approvalConnection') }}:
        <el-tag size="small">{{ request.connection }}</el-tag>
      </p>
      <div v-if="request?.command" class="mcp-approval-command">
        <label class="mcp-approval-label">{{ t('mcp.approvalCommand') }}</label>
        <pre>{{ request.command }}</pre>
      </div>
      <p v-else class="mcp-approval-note">{{ t('mcp.approvalConnectNote') }}</p>
      <el-input
        v-model="reason"
        type="textarea"
        :rows="2"
        :placeholder="t('mcp.approvalReasonPlaceholder')"
      />
      <p class="mcp-approval-timeout">{{ t('mcp.approvalTimeoutHint', { seconds: 110 }) }}</p>
    </div>
    <template #footer>
      <el-button @click="onDeny">{{ t('mcp.deny') }}</el-button>
      <el-button type="danger" plain @click="onDenyWithReason">{{ t('mcp.denyWithReason') }}</el-button>
      <el-button type="primary" @click="onApprove">{{ t('mcp.approve') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from '../i18n'
import type { MCPApprovalRequest } from '../types/mcp'

const { t } = useI18n()

const props = defineProps<{
  visible: boolean
  request: MCPApprovalRequest | null
}>()

const emit = defineEmits<{
  (e: 'resolve', approved: boolean, reason: string): void
}>()

const reason = ref('')

watch(() => props.visible, v => { if (v) reason.value = '' })

function onApprove() {
  emit('resolve', true, '')
}

function onDeny() {
  emit('resolve', false, '')
}

function onDenyWithReason() {
  emit('resolve', false, reason.value || t('mcp.deniedDefaultReason'))
}
</script>

<style scoped>
.mcp-approval-client {
  margin: 0 0 0.5rem;
}
.mcp-approval-conn {
  margin: 0 0 0.5rem;
}
.mcp-approval-label {
  display: block;
  font-size: 0.75rem;
  opacity: 0.7;
  margin-bottom: 0.25rem;
}
.mcp-approval-command pre {
  margin: 0 0 0.75rem;
  padding: 0.5rem 0.75rem;
  background: var(--el-fill-color-dark);
  border-radius: 4px;
  font-family: var(--el-font-family-mono, monospace);
  font-size: 0.8rem;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 12rem;
  overflow: auto;
}
.mcp-approval-note {
  opacity: 0.7;
  font-size: 0.8rem;
  margin: 0 0 0.75rem;
}
.mcp-approval-timeout {
  font-size: 0.75rem;
  opacity: 0.6;
  margin: 0.5rem 0 0;
}
</style>
