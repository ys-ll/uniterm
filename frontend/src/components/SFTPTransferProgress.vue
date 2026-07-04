<template>
  <div v-if="tasks.length > 0" class="transfer-progress-bar">
    <div v-for="task in tasks" :key="task.id" class="transfer-task">
      <span class="task-type"><ArrowUp v-if="task.type === 'upload'" :size="12" /><ArrowDown v-else :size="12" /></span>
      <span class="task-name">{{ task.name }}</span>
      <span class="task-eta" v-if="task.eta">{{ task.eta }}</span>
      <span class="task-speed" v-if="task.status === 'running' || task.status === 'paused'">{{ task.speed || '--' }}</span>
      <el-progress
        :percentage="task.percentage"
        :status="task.status === 'error' ? 'exception' : task.status === 'cancelled' ? 'warning' : undefined"
        :stroke-width="4"
        style="flex: 1"
      />
      <div class="task-actions">
        <button
          v-if="task.status === 'running'"
          class="btn btn-ghost btn-icon btn-sm"
          :title="t('sftp.pauseTransfer')"
          @click="emit('pause', task.id)"
        ><Pause :size="14" /></button>
        <button
          v-else-if="task.status === 'paused'"
          class="btn btn-ghost btn-icon btn-sm"
          :title="t('sftp.resumeTransfer')"
          @click="emit('resume', task.id)"
        ><Play :size="14" /></button>
        <button
          v-if="task.status === 'running' || task.status === 'paused'"
          class="btn btn-ghost btn-icon btn-sm danger"
          :title="t('sftp.cancelTransfer')"
          @click="emit('cancel', task.id)"
        ><X :size="14" /></button>
        <span v-else-if="task.status === 'cancelled'" class="status-text">{{ t('sftp.cancelled') }}</span>
        <span v-else-if="task.status === 'done'" class="status-text done" :title="t('sftp.done')"><Check :size="14" /></span>
        <span v-else-if="task.status === 'error'" class="status-text error">{{ t('sftp.error') }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { X, Pause, Play, ArrowUp, ArrowDown, Check } from '@lucide/vue'
import { useI18n } from '../i18n'

interface TransferTaskUI {
  id: string
  type: 'upload' | 'download'
  name: string
  percentage: number
  speed: string
  eta: string
  status: 'running' | 'paused' | 'done' | 'error' | 'cancelled'
}

defineProps<{
  tasks: TransferTaskUI[]
}>()

const emit = defineEmits<{
  cancel: [taskId: string]
  pause: [taskId: string]
  resume: [taskId: string]
}>()

const { t } = useI18n()
</script>

<style scoped>
.transfer-progress-bar {
  padding: 4px 12px;
  background: var(--bg-elevated);
  border-top: 1px solid var(--border-subtle);
  max-height: 200px;
  overflow-y: auto;
}
.transfer-task {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0;
  height: 26px;
}
.task-type {
  display: inline-flex;
  align-items: center;
  color: var(--accent);
  flex-shrink: 0;
}
.task-name {
  font-size: 11px;
  line-height: 1;
  font-family: var(--font-mono);
  color: var(--text-secondary);
  min-width: 90px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.task-eta {
  font-size: 10px;
  line-height: 1;
  font-family: var(--font-mono);
  color: var(--text-disabled);
  min-width: 48px;
  flex-shrink: 0;
}
.task-speed {
  font-size: 10px;
  line-height: 1;
  font-family: var(--font-mono);
  color: var(--text-disabled);
  min-width: 56px;
  flex-shrink: 0;
}
.task-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  min-width: 52px;
  height: 24px;
}
.status-text {
  font-size: 10px;
  line-height: 1;
  color: var(--text-disabled);
  flex-shrink: 0;
}
.status-text.done {
  color: var(--accent);
}
.status-text.error {
  color: var(--error);
}
</style>

<style>
/* Progress percentage text — not scoped so it penetrates el-progress */
.transfer-progress-bar .el-progress__text {
  font-size: 11px !important;
  font-family: var(--font-mono);
}
</style>
