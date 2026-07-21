<template>
  <el-dialog append-to-body
    v-model="visible"
    :title="editingId ? t('tunnels.editTunnel') : t('tunnels.addTunnel')"
    width="500px"
    class="tunnel-dialog"
    @close="resetForm"
  >
    <!-- Tunnel mode: same look as the connection form's type selection -->
    <div class="mode-section">
      <div class="mode-grid">
        <button
          v-for="m in modes"
          :key="m.value"
          type="button"
          class="mode-btn"
          :class="{ active: form.mode === m.value }"
          @click="form.mode = m.value"
        >
          <component :is="m.icon" :size="18" />
          <span>{{ m.label }}</span>
        </button>
      </div>
      <div class="mode-desc">{{ t(`tunnels.hint.${form.mode}`) }}</div>
    </div>

    <el-form :model="form" label-width="90px">
      <el-form-item :label="t('tunnels.name')" required>
        <el-input v-model="form.name" :placeholder="t('tunnels.namePlaceholder')" maxlength="50" />
      </el-form-item>

      <el-form-item :label="t('tunnels.sshConn')" required>
        <el-select v-model="form.sshConnId" :placeholder="t('tunnels.sshConnPlaceholder')" filterable class="full">
          <el-option
            v-for="c in sshConnections"
            :key="c.id"
            :label="`${c.name} (${c.user}@${c.host}:${c.port})`"
            :value="c.id"
          />
        </el-select>
      </el-form-item>

      <!-- Local -->
      <template v-if="form.mode === 'local'">
        <el-form-item :label="t('tunnels.listenLocal')" required>
          <div class="hostport">
            <el-input v-model="form.listenHost" placeholder="127.0.0.1" />
            <span class="colon">:</span>
            <el-input-number v-model="form.listenPort" :min="1" :max="65535" :controls="false" :placeholder="'13306'" />
          </div>
        </el-form-item>
        <el-form-item :label="t('tunnels.destination')" required>
          <div class="hostport">
            <el-input v-model="form.targetHost" placeholder="10.0.1.20" />
            <span class="colon">:</span>
            <el-input-number v-model="form.targetPort" :min="1" :max="65535" :controls="false" :placeholder="'3306'" />
          </div>
        </el-form-item>
      </template>

      <!-- Remote -->
      <template v-else-if="form.mode === 'remote'">
        <el-form-item :label="t('tunnels.listenRemote')" required>
          <div class="hostport">
            <el-input v-model="form.listenHost" placeholder="0.0.0.0" />
            <span class="colon">:</span>
            <el-input-number v-model="form.listenPort" :min="1" :max="65535" :controls="false" :placeholder="'8022'" />
          </div>
        </el-form-item>
        <el-form-item :label="t('tunnels.toLocal')" required>
          <div class="hostport">
            <el-input v-model="form.targetHost" placeholder="127.0.0.1" />
            <span class="colon">:</span>
            <el-input-number v-model="form.targetPort" :min="1" :max="65535" :controls="false" :placeholder="'22'" />
          </div>
        </el-form-item>
      </template>

      <!-- Dynamic -->
      <template v-else>
        <el-form-item :label="t('tunnels.listenLocal')" required>
          <div class="hostport">
            <el-input v-model="form.listenHost" placeholder="127.0.0.1" />
            <span class="colon">:</span>
            <el-input-number v-model="form.listenPort" :min="1" :max="65535" :controls="false" :placeholder="'1080'" />
          </div>
        </el-form-item>
      </template>

      <el-form-item :label="t('tunnels.autoStartLabel')">
        <el-switch v-model="form.autoStart" />
        <span class="inline-hint">{{ t('tunnels.autoStart') }}</span>
      </el-form-item>
    </el-form>

    <div v-if="errorMsg" class="form-error">{{ errorMsg }}</div>

    <template #footer>
      <el-button @click="visible = false">{{ t('tunnels.cancel') }}</el-button>
      <el-button type="primary" @click="handleSave">{{ t('tunnels.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { ArrowRightToLine, ArrowLeftToLine, Waypoints } from '@lucide/vue'
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
  connectionStore.connections
    .filter(c => c.type === 'ssh')
    .sort((a, b) => a.name.localeCompare(b.name))
)

const modes = computed(() => [
  { value: 'local' as TunnelMode, label: t('tunnels.mode.local'), icon: ArrowRightToLine },
  { value: 'remote' as TunnelMode, label: t('tunnels.mode.remote'), icon: ArrowLeftToLine },
  { value: 'dynamic' as TunnelMode, label: t('tunnels.mode.dynamic'), icon: Waypoints },
])

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
.inline-hint { font-size: 12px; color: var(--text-secondary); margin-left: 10px; }
.hostport { display: grid; grid-template-columns: 1fr auto 120px; gap: 8px; align-items: center; width: 100%; }
.hostport .colon { color: var(--text-muted); text-align: center; }
.hostport :deep(.el-input-number) { width: 100%; }
.form-error { color: var(--error); font-size: 12px; margin-top: 2px; }

/* Mode buttons: identical look to the connection form's sub-type selection */
.mode-section {
  padding-bottom: 14px;
  margin-bottom: 16px;
  border-bottom: 1px solid var(--border-subtle);
}
.mode-grid {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 6px;
}
.mode-btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  width: 80px;
  height: 56px;
  padding: 4px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  font-family: var(--font-ui);
  font-size: 11px;
  font-weight: 500;
  transition: all 0.15s ease;
}
.mode-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
  border-color: var(--border-default);
}
.mode-btn.active {
  background: linear-gradient(135deg, var(--accent), var(--accent));
  color: var(--on-accent);
  border-color: var(--accent-glow);
  box-shadow: 0 0 0 1px var(--accent-glow), 0 2px 8px var(--accent-glow);
}
.mode-btn span {
  text-align: center;
  line-height: 1.2;
}
.mode-desc {
  margin-top: 12px;
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.5;
  text-align: center;
}

/* ── Dialog overrides ── */
:deep(.el-dialog__body) {
  padding: 16px 20px;
}
</style>
