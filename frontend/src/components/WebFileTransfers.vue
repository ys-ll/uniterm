<template>
  <div v-if="isWebMode()" class="web-file-transfers" aria-live="polite">
    <div v-if="uploading">
      Device → host: uploading…
      <button @click="$emit('cancelUpload')">Cancel upload</button>
    </div>
    <div v-for="item in downloads" :key="item.id">
      <span>{{ item.name }} — {{ item.state === 'ready' ? 'Ready for browser download' : item.error || 'Remote → host: preparing…' }}</span>
      <a v-if="item.state === 'ready'" :href="fileDownloadURL(item.id)" download>Download</a>
      <button v-if="item.state === 'error'" @click="watchDownload(item.id)">Check status</button>
      <button @click="dismiss(item.id)">{{ item.state === 'transferring' ? 'Cancel' : 'Dismiss' }}</button>
    </div>
  </div>
</template>
<script setup lang="ts">
import { webDownloads, fileDownloadURL, removeWebDownload, watchDownload } from '../services/webFiles'
import { isWebMode } from '../utils/webMode'
import { msg } from '../services/message'
import { computed } from 'vue'
const props = defineProps<{ uploading?: boolean; terminalSessionId?: string }>()
const downloads = computed(() => webDownloads.value.filter(item => props.terminalSessionId !== undefined
  ? item.terminal && item.sessionId === props.terminalSessionId
  : !item.terminal))
defineEmits<{ cancelUpload: [] }>()
async function dismiss(id: string) {
  try { await removeWebDownload(id) } catch (e) { msg.error(String(e)) }
}
</script>
<style scoped>
.web-file-transfers { font-size: 12px; padding: 0 8px; max-height: 100px; overflow: auto; }
.web-file-transfers div { display: flex; gap: 8px; align-items: center; padding: 4px 0; }
.web-file-transfers span { flex: 1; overflow-wrap: anywhere; }
</style>
