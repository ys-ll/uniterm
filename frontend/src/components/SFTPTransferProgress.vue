<template>
  <div v-if="tasks.length > 0" class="transfer-progress-bar">
    <div v-for="task in tasks" :key="task.id" class="transfer-task">
      <span class="task-type">{{ task.type === 'upload' ? '↑' : '↓' }}</span>
      <span class="task-name">{{ task.name }}</span>
      <span class="task-eta" v-if="task.eta">{{ task.eta }}</span>
      <span class="task-speed" v-if="task.status === 'running' || task.status === 'paused'">{{ task.speed || '--' }}</span>
      <el-progress
        :percentage="task.percentage"
        :status="task.status === 'error' ? 'exception' : task.status === 'cancelled' ? 'warning' : undefined"
        :stroke-width="4"
        style="flex: 1"
      />
      <el-button
        v-if="task.status === 'running'"
       
        :icon="Pause"
        circle
        @click="emit('pause', task.id)"
        :title="t('sftp.pauseTransfer')"
      />
      <el-button
        v-else-if="task.status === 'paused'"
       
        type="success"
        :icon="Play"
        circle
        @click="emit('resume', task.id)"
        :title="t('sftp.resumeTransfer')"
      />
      <el-button
        v-if="task.status === 'running' || task.status === 'paused'"
       
        type="danger"
        :icon="X"
        circle
        @click="emit('cancel', task.id)"
        :title="t('sftp.cancelTransfer')"
      />
      <span v-else-if="task.status === 'cancelled'" class="status-text">{{ t('sftp.cancelled') }}</span>
      <span v-else-if="task.status === 'done'" class="status-text done">{{ t('sftp.done') }}</span>
      <span v-else-if="task.status === 'error'" class="status-text error">{{ t('sftp.error') }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { X, Pause, Play } from '@lucide/vue'
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
  padding: 2px 0;
}
.task-type {
  font-size: 11px;
  color: var(--accent);
  flex-shrink: 0;
}
.task-name {
  font-size: 11px;
  font-family: var(--font-mono);
  color: var(--text-secondary);
  min-width: 90px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.task-eta {
  font-size: 10px;
  font-family: var(--font-mono);
  color: var(--text-disabled);
  min-width: 48px;
  flex-shrink: 0;
}
.task-speed {
  font-size: 10px;
  font-family: var(--font-mono);
  color: var(--text-disabled);
  min-width: 56px;
  flex-shrink: 0;
}
.status-text {
  font-size: 10px;
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
