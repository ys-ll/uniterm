<template>
  <div
    class="tn-item"
    :class="{ running: status === 'running', errored: status === 'error' }"
    draggable="true"
    @dragstart="$emit('dragstart', $event, tunnel)"
    @dblclick="toggleRun"
    @contextmenu.prevent="$emit('context', $event, tunnel)"
    @mouseenter="hovered = true"
    @mouseleave="hovered = false"
  >
    <span class="tn-status" :title="statusTitle"></span>
    <div class="tn-item-content">
      <div class="tn-item-name">{{ tunnel.name }}</div>
      <div class="tn-item-meta">
        <span class="tn-badge" :class="tunnel.mode">{{ modeLabel }}</span>
        <span class="tn-port">:{{ effectivePort }}</span>
      </div>
    </div>
    <button class="tn-run-btn" :title="status === 'running' ? t('tunnels.stop') : t('tunnels.start')" @click.stop="toggleRun">
      <Square v-if="status === 'running'" :size="13" />
      <Play v-else :size="13" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Play, Square } from '@lucide/vue'
import { useTunnelStore, type Tunnel } from '../stores/tunnelStore'
import { useI18n } from '../i18n'
import { msg } from '../services/message'

const { t } = useI18n()
const store = useTunnelStore()

const props = defineProps<{ tunnel: Tunnel }>()
defineEmits<{
  edit: [id: string]
  context: [e: MouseEvent, tn: Tunnel]
  dragstart: [e: DragEvent, tn: Tunnel]
}>()

const hovered = ref(false)

const status = computed(() => store.statusOf(props.tunnel.id))
const modeLabel = computed(() => props.tunnel.mode.charAt(0).toUpperCase() + props.tunnel.mode.slice(1))
const effectivePort = computed(() => store.states[props.tunnel.id]?.localPort || props.tunnel.listenPort)
const statusTitle = computed(() => {
  if (status.value === 'error') return store.states[props.tunnel.id]?.error || t('tunnels.statusError')
  return status.value === 'running' ? t('tunnels.statusRunning') : t('tunnels.statusStopped')
})

async function toggleRun() {
  if (status.value === 'running') {
    await store.stop(props.tunnel.id)
  } else {
    const st = await store.start(props.tunnel.id)
    if (st.status === 'error') msg.error(st.error || t('tunnels.startFailed'))
  }
}
</script>

<style scoped>
.tn-item { display: flex; align-items: center; gap: 10px; padding: 7px 10px; min-height: 40px; border-radius: var(--radius-sm); cursor: pointer; margin: 1px 0; user-select: none; }
.tn-item:hover { background: var(--bg-hover); }
.tn-status { width: 8px; height: 8px; border-radius: 50%; background: var(--text-disabled); flex-shrink: 0; }
.tn-item.running .tn-status { background: var(--success, #24c08a); box-shadow: 0 0 0 3px color-mix(in srgb, var(--success, #24c08a) 20%, transparent); }
.tn-item.errored .tn-status { background: var(--error, #e5604d); }
.tn-item-content { flex: 1; min-width: 0; }
.tn-item-name { font-size: 12px; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.tn-item-meta { display: flex; align-items: center; gap: 8px; margin-top: 2px; }
.tn-badge { font-size: 10px; font-weight: 600; padding: 0 5px; border-radius: 4px; background: var(--bg-active, rgba(255,255,255,.06)); color: var(--text-muted); }
.tn-badge.local { color: #8fb6ff; }
.tn-badge.remote { color: #e0a54b; }
.tn-badge.dynamic { color: #24c08a; }
.tn-port { font-size: 11px; color: var(--text-muted); font-family: var(--font-mono, monospace); }
.tn-run-btn { width: 24px; height: 24px; display: inline-flex; align-items: center; justify-content: center; border: none; border-radius: 5px; background: transparent; color: var(--text-muted); cursor: pointer; flex-shrink: 0; }
.tn-item.running .tn-run-btn { color: var(--error, #e5604d); }
.tn-item:not(.running) .tn-run-btn { color: var(--success, #24c08a); }
.tn-run-btn:hover { background: var(--bg-active, rgba(255,255,255,.08)); }
</style>
