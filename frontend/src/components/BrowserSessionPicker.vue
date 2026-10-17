<template>
  <el-dialog :model-value="visible" :title="t('web.existingTerminals')" width="min(36rem, 94vw)" @update:model-value="emit('update:visible', $event)">
    <p>{{ t('web.attachHint') }}</p>
    <el-alert v-if="error" :title="error" type="error" :closable="false" role="alert" />
    <div class="catalog" :aria-busy="loading">
      <p v-if="!loading && !sessions.length">{{ t('web.noTerminals') }}</p>
      <button v-for="session in sessions" :key="session.id" class="session" :disabled="attaching !== '' || !browserConnection.connected" @click="attach(session)">
        <span>{{ session.title || session.type }}</span>
        <small>{{ session.type }} · {{ session.status }} · {{ session.id }}</small>
      </button>
    </div>
    <template #footer>
      <el-button :loading="loading" @click="refresh">{{ t('web.refreshStatus') }}</el-button>
      <el-button @click="emit('update:visible', false)">{{ t('common.cancel') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'
import { useI18n } from '../i18n'
import { backendErrorText } from '../utils/backendError'
import { attachBrowserTerminal } from '../utils/webTerminalIntegration'
import { browserConnection, waitForBrowserEvents } from '../utils/browserRuntime'
import { webTerminalBridge, type WebSession } from '../services/webTerminals'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [value: boolean] }>()
const { t } = useI18n()
const sessions = ref<WebSession[]>([])
const loading = ref(false)
const attaching = ref('')
const error = ref('')

async function refresh() {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    await waitForBrowserEvents()
    sessions.value = (await webTerminalBridge.sessions()) || []
  }
  catch (e) { error.value = backendErrorText(e) }
  finally { loading.value = false }
}

async function attach(session: WebSession) {
  attaching.value = session.id
  error.value = ''
  try {
    await attachBrowserTerminal(session)
    emit('update:visible', false)
  } catch (e) { error.value = backendErrorText(e) }
  finally { attaching.value = '' }
}

watch(() => props.visible, visible => { if (visible) void refresh() }, { immediate: true })
const offs = ['web:session-created', 'session:closed', 'web:resync'].map(name =>
  Events.On(name, () => { if (props.visible) void refresh() }))
onUnmounted(() => offs.forEach(off => off()))
</script>

<style scoped>
.catalog { display: flex; flex-direction: column; gap: 0.5rem; max-height: 55vh; overflow: auto; margin-top: 1rem; }
.session { text-align: left; padding: 0.75rem; border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); color: var(--text-primary); background: var(--bg-elevated); cursor: pointer; }
.session:hover, .session:focus-visible { border-color: var(--accent); }
.session:disabled { opacity: 0.5; cursor: default; }
small { display: block; color: var(--text-secondary); margin-top: 0.25rem; overflow-wrap: anywhere; }
</style>
