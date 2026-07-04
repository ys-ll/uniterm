<template>
  <div v-if="hasActiveTransfers" class="zmodem-transfer-panel">
    <div v-for="t in activeTransfers" :key="t.id" class="transfer-item">
      <div class="transfer-header">
        <span class="transfer-icon"><Download v-if="t.direction === 'download'" :size="14" /><Upload v-else :size="14" /></span>
        <span class="transfer-name">{{ t.filename }}</span>
        <span v-if="t.status === 'completed'" class="transfer-status success"><Check :size="14" /></span>
        <span v-else-if="t.status === 'error'" class="transfer-status error"><X :size="14" /></span>
        <span v-else-if="t.status === 'cancelled'" class="transfer-status cancelled"><CircleOff :size="14" /></span>
      </div>
      <div v-if="t.status === 'transferring' || t.status === 'pending'" class="transfer-progress">
        <div class="progress-bar">
          <div class="progress-fill" :style="{ width: progressPercent(t) + '%' }"></div>
        </div>
        <div class="progress-info">
          <span>{{ formatBytes(t.transferred) }} / {{ formatBytes(t.size) }}</span>
          <span v-if="t.speed > 0">{{ formatBytes(t.speed) }}/s</span>
        </div>
      </div>
      <div v-if="t.status === 'transferring'" class="transfer-actions">
        <button class="cancel-btn" @click="cancelTransfer(t)">{{ tt('common.cancel') }}</button>
      </div>
      <div v-if="t.status === 'completed'" class="transfer-complete">{{ tt('zmodem.transferComplete') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Download, Upload, Check, X, CircleOff } from '@lucide/vue'
import { useZmodemStore } from '../stores/zmodemStore'
import { useI18n } from '../i18n'

const props = defineProps<{
  sessionId: string
}>()

const emit = defineEmits<{
  cancel: []
}>()

const store = useZmodemStore()
const { t: tt } = useI18n()

const activeTransfers = computed(() => {
  return store.getTransfers(props.sessionId).filter(t =>
    t.status === 'transferring' || t.status === 'pending'
  )
})

const hasActiveTransfers = computed(() => activeTransfers.value.length > 0)

function progressPercent(t: ReturnType<typeof store.getTransfers>[number]) {
  if (t.size === 0) return 0
  return Math.min(100, Math.round((t.transferred / t.size) * 100))
}

function formatBytes(bytes: number) {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function cancelTransfer(t: ReturnType<typeof store.getTransfers>[number]) {
  store.updateTransfer(props.sessionId, t.id, { status: 'cancelled' })
  emit('cancel')
}
</script>

<style scoped>
.zmodem-transfer-panel {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 320px;
  max-width: 90%;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: 12px 16px;
  z-index: 20;
  backdrop-filter: blur(8px);
  box-shadow: var(--shadow-lg);
}
.transfer-item + .transfer-item {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--border-subtle);
}
.transfer-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-primary);
}
.transfer-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.transfer-status.success { color: var(--success); }
.transfer-status.error { color: var(--error); }
.transfer-status.cancelled { color: var(--text-muted); }
.transfer-progress {
  margin-top: 6px;
}
.progress-bar {
  height: 4px;
  background: var(--bg-elevated);
  border-radius: 2px;
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  background: var(--accent);
  border-radius: 2px;
  transition: width 0.3s ease;
}
.progress-info {
  display: flex;
  justify-content: space-between;
  margin-top: 4px;
  font-size: 11px;
  color: var(--text-muted);
  font-family: var(--font-mono);
}
.transfer-actions {
  margin-top: 6px;
  text-align: right;
}
.cancel-btn {
  padding: 3px 10px;
  font-size: 11px;
  background: transparent;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  cursor: pointer;
}
.cancel-btn:hover {
  border-color: var(--error);
  color: var(--error);
}
.transfer-complete {
  margin-top: 4px;
  font-size: 11px;
  color: var(--success);
}
</style>
