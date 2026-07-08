<template>
  <el-dialog
    v-model="visible"
    :title="editingId ? t('tunnels.editTunnel') : t('tunnels.addTunnel')"
    width="500px"
    :close-on-click-modal="false"
    class="tunnel-dialog"
    @close="resetForm"
  >
    <el-form :model="form" label-width="88px" label-position="right">
      <el-form-item :label="t('tunnels.name')" required>
        <el-input v-model="form.name" :placeholder="t('tunnels.namePlaceholder')" maxlength="50" />
      </el-form-item>

      <el-form-item :label="t('tunnels.sshConn')" required>
        <el-select v-model="form.sshConnId" :placeholder="t('tunnels.sshConnPlaceholder')" filterable class="full">
          <el-option v-for="c in sshConnections" :key="c.id" :label="c.name" :value="c.id">
            <span>{{ c.name }}</span>
            <span class="opt-meta">{{ c.user }}@{{ c.host }}</span>
          </el-option>
        </el-select>
      </el-form-item>
      <div class="row-hint">{{ t('tunnels.sshConnHint') }}</div>

      <el-form-item :label="t('tunnels.mode')" required>
        <el-radio-group v-model="form.mode" class="mode-group">
          <el-radio-button value="local">{{ t('tunnels.mode.local') }}</el-radio-button>
          <el-radio-button value="remote">{{ t('tunnels.mode.remote') }}</el-radio-button>
          <el-radio-button value="dynamic">{{ t('tunnels.mode.dynamic') }}</el-radio-button>
        </el-radio-group>
      </el-form-item>

      <!-- Local -->
      <template v-if="form.mode === 'local'">
        <el-form-item :label="t('tunnels.localPort')" required>
          <el-input v-model.number="form.listenPort" type="number" placeholder="13306" />
        </el-form-item>
        <el-form-item :label="t('tunnels.bind')">
          <el-input v-model="form.listenHost" placeholder="127.0.0.1" />
        </el-form-item>
        <el-form-item :label="t('tunnels.destination')" required>
          <div class="hostport">
            <el-input v-model="form.targetHost" placeholder="10.0.1.20" />
            <span class="colon">:</span>
            <el-input v-model.number="form.targetPort" type="number" placeholder="3306" />
          </div>
        </el-form-item>
        <div class="row-hint">{{ t('tunnels.hint.local') }}</div>
      </template>

      <!-- Remote -->
      <template v-else-if="form.mode === 'remote'">
        <el-form-item :label="t('tunnels.remotePort')" required>
          <el-input v-model.number="form.listenPort" type="number" placeholder="8022" />
        </el-form-item>
        <el-form-item :label="t('tunnels.remoteBind')">
          <el-input v-model="form.listenHost" placeholder="0.0.0.0" />
        </el-form-item>
        <el-form-item :label="t('tunnels.toLocal')" required>
          <div class="hostport">
            <el-input v-model="form.targetHost" placeholder="127.0.0.1" />
            <span class="colon">:</span>
            <el-input v-model.number="form.targetPort" type="number" placeholder="22" />
          </div>
        </el-form-item>
        <div class="row-hint">{{ t('tunnels.hint.remote') }}</div>
      </template>

      <!-- Dynamic -->
      <template v-else>
        <el-form-item :label="t('tunnels.socksPort')" required>
          <el-input v-model.number="form.listenPort" type="number" placeholder="1080" />
        </el-form-item>
        <el-form-item :label="t('tunnels.bind')">
          <el-input v-model="form.listenHost" placeholder="127.0.0.1" />
        </el-form-item>
        <div class="row-hint">{{ t('tunnels.hint.dynamic') }}</div>
      </template>

      <el-divider />

      <el-form-item :label="t('tunnels.autoStartLabel')">
        <el-switch v-model="form.autoStart" />
        <span class="inline-hint">{{ t('tunnels.autoStart') }}</span>
      </el-form-item>
    </el-form>

    <div v-if="errorMsg" class="form-error">{{ errorMsg }}</div>

    <template #footer>
      <el-button disabled class="test-btn">
        {{ t('tunnels.test') }} <span class="soon">{{ t('tunnels.testSoon') }}</span>
      </el-button>
      <el-button @click="visible = false">{{ t('tunnels.cancel') }}</el-button>
      <el-button type="primary" @click="handleSave">{{ t('tunnels.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useTunnelStore, type TunnelMode } from '../stores/tunnelStore'
import { useConnectionStore } from '../stores/connectionStore'
import { useI18n } from '../i18n'

const { t } = useI18n()
const store = useTunnelStore()
const connectionStore = useConnectionStore()

const props = defineProps<{
  modelValue: boolean
  editingId?: string
  initialGroupId?: string
}>()

const emit = defineEmits<{ 'update:modelValue': [v: boolean] }>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const sshConnections = computed(() =>
  connectionStore.connections.filter(c => c.type === 'ssh')
)

function blankForm() {
  return {
    name: '',
    sshConnId: '',
    mode: 'local' as TunnelMode,
    listenHost: '127.0.0.1',
    listenPort: undefined as number | undefined,
    targetHost: '',
    targetPort: undefined as number | undefined,
    autoStart: false,
    groupId: undefined as string | undefined,
  }
}

const form = reactive(blankForm())
const errorMsg = ref('')

watch(visible, (v) => {
  if (!v) return
  errorMsg.value = ''
  if (props.editingId) {
    const t0 = store.tunnels.find(x => x.id === props.editingId)
    if (t0) {
      Object.assign(form, blankForm(), {
        name: t0.name,
        sshConnId: t0.sshConnId,
        mode: t0.mode,
        listenHost: t0.listenHost || '127.0.0.1',
        listenPort: t0.listenPort,
        targetHost: t0.targetHost || '',
        targetPort: t0.targetPort,
        autoStart: !!t0.autoStart,
        groupId: t0.groupId,
      })
    }
  } else {
    Object.assign(form, blankForm(), { groupId: props.initialGroupId })
  }
})

function handleSave() {
  if (!form.name.trim()) { errorMsg.value = t('tunnels.errName'); return }
  if (!form.sshConnId) { errorMsg.value = t('tunnels.errConn'); return }
  if (!form.listenPort) { errorMsg.value = t('tunnels.errListenPort'); return }
  if (form.mode !== 'dynamic' && (!form.targetHost.trim() || !form.targetPort)) {
    errorMsg.value = t('tunnels.errTarget'); return
  }
  const payload = {
    name: form.name.trim(),
    mode: form.mode,
    sshConnId: form.sshConnId,
    listenHost: form.listenHost.trim() || '127.0.0.1',
    listenPort: form.listenPort,
    targetHost: form.mode === 'dynamic' ? undefined : form.targetHost.trim(),
    targetPort: form.mode === 'dynamic' ? undefined : form.targetPort,
    autoStart: form.autoStart,
    groupId: form.groupId,
  }
  if (props.editingId) {
    store.updateTunnel(props.editingId, payload)
  } else {
    store.addTunnel(payload)
  }
  visible.value = false
}

function resetForm() {
  Object.assign(form, blankForm())
  errorMsg.value = ''
}
</script>

<style scoped>
.full { width: 100%; }
.tunnel-dialog :deep(.el-select) { width: 100%; }
.opt-meta { float: right; color: var(--text-muted); font-size: 11px; margin-left: 12px; }
.row-hint { font-size: 11px; color: var(--text-muted); line-height: 1.5; margin: -10px 0 14px 88px; }
.inline-hint { font-size: 12px; color: var(--text-secondary); margin-left: 10px; }
.hostport { display: grid; grid-template-columns: 1fr auto 100px; gap: 8px; align-items: center; width: 100%; }
.hostport .colon { color: var(--text-muted); text-align: center; }
.mode-group { display: flex; width: 100%; }
.mode-group :deep(.el-radio-button) { flex: 1; }
.mode-group :deep(.el-radio-button__inner) { width: 100%; }
.form-error { color: var(--error); font-size: 12px; margin-top: 2px; }
.test-btn { margin-right: auto; }
.test-btn .soon { font-size: 10px; color: var(--warning, #e0a54b); margin-left: 4px; }
</style>
