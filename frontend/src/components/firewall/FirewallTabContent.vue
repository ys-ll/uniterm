<template>
  <el-dialog
    append-to-body
    :model-value="visible"
    :title="t('firewall.title')"
    width="72%"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    destroy-on-close
    class="firewall-dialog"
    style="display:flex; flex-direction:column; height:88vh; border-radius:var(--radius-sm);"
    @update:model-value="(v: boolean) => {
      if (!v) handleDialogClose()
      emit('update:visible', v)
    }"
  >
    <div class="fw-content">
      <!-- ── Connection selector + status switches ── -->
      <div class="fw-hostbar">
        <div class="fw-host-left">
          <span class="ctl-label">{{ t('firewall.host') }}</span>
          <el-select
            v-model="selectedRemoteConn"
            class="fw-host-select"
            :placeholder="t('firewall.hostPlaceholder')"
            filterable
            @change="onRemoteConnChange"
          >
            <el-option
              v-for="conn in remoteConnections"
              :key="conn.id"
              :value="conn.id"
              :label="`${conn.name}  (${conn.user}@${conn.host})`"
            />
          </el-select>
          <el-button class="flat-btn" :disabled="!selectedRemoteConn || loading || busy" @click="refreshConfig">
            {{ t('firewall.refresh') }}
          </el-button>
        </div>
        <div class="fw-host-switches">
          <div class="fw-switch-cell">
            <span class="fw-switch-label">{{ t('firewall.status') }}</span>
            <el-switch
              v-if="status"
              size="small"
              :model-value="status.active"
              :loading="busy"
              :disabled="!selectedRemoteConn || !canModify"
              @change="(v: boolean | string | number) => toggleEnabled(!!v)"
            />
            <span
              v-if="status && status.backend"
              class="fw-backend cap"
              :class="status.active ? 'fw-backend-on' : ''"
            >{{ displayBackend(status.backend) }}</span>
          </div>
          <div class="fw-switch-cell">
            <span class="fw-switch-label">{{ t('firewall.autoStart') }}</span>
            <el-switch
              v-if="status"
              size="small"
              :model-value="status.autoStart"
              :loading="autostartBusy"
              :disabled="!selectedRemoteConn || !canModify"
              @change="(v: boolean | string | number) => toggleAutoStart(!!v)"
            />
          </div>
        </div>
      </div>

      <template v-if="selectedRemoteConn">
        <el-alert
          v-if="notInstalled"
          :title="t('firewall.notInstalled')"
          type="warning"
          :closable="false"
          show-icon
          class="fw-alert"
        />
        <el-alert
          v-else-if="noPermission"
          :title="t('firewall.noPermission')"
          type="error"
          :closable="false"
          show-icon
          class="fw-alert"
        />

        <template v-if="canManage">
          <!-- ── Allow port form ── -->
          <div class="fw-form srf">
            <div class="fw-form-head t">{{ t('firewall.allowPortTitle') }}</div>
            <div class="fw-form-row">
              <el-select v-model="formPort.action" class="fw-act">
                <el-option :value="'allow'" :label="t('firewall.allow')" />
                <el-option :value="'deny'" :label="t('firewall.deny')" />
              </el-select>
              <el-select v-model="formPort.protocol" class="fw-proto">
                <el-option value="tcp" label="TCP" />
                <el-option value="udp" label="UDP" />
              </el-select>
              <el-input v-model="formPort.port" class="fw-port" :placeholder="t('firewall.portPlaceholder')" clearable />
              <el-input v-model="formPort.source" class="fw-source" :placeholder="t('firewall.sourcePlaceholder')" clearable />
              <el-button type="primary" class="fw-add" :disabled="!canAdd || busy" @click="addRule">
                {{ formPort.action === 'allow' ? t('firewall.allow') : t('firewall.deny') }}
              </el-button>
            </div>
            <div class="cap form-hint">{{ t('firewall.portHint') }} · {{ t('firewall.editHint') }}</div>
          </div>

          <!-- ── Rules list ── -->
          <div class="fw-rules srf">
            <div class="fw-rules-head">
              <span class="t">{{ t('firewall.rulesTitle') }}</span>
              <span v-if="hasUnsavedChanges" class="pill p-pending">{{ t('firewall.unsaved') }}</span>
              <span v-else-if="needsApply" class="pill p-pending">{{ t('firewall.savedNotApplied') }}</span>
            </div>
            <el-table :data="rules" v-loading="loading" size="small" empty-text="" class="fw-table" height="100%">
              <template #empty>
                <span class="dim-txt">{{ notInstalled ? t('firewall.notInstalled') : t('firewall.rulesEmpty') }}</span>
              </template>
              <el-table-column :label="t('firewall.colPort')" prop="port" min-width="110" />
              <el-table-column :label="t('firewall.colProto')" prop="protocol" width="80" />
              <el-table-column :label="t('firewall.colSource')" prop="source" min-width="120">
                <template #default="{ row }">{{ row.source || t('firewall.any') }}</template>
              </el-table-column>
              <el-table-column :label="t('firewall.colPersist')" width="100">
                <template #default="{ row }">{{ row.persist === 'temporary' ? t('firewall.temp') : t('firewall.perm') }}</template>
              </el-table-column>
              <el-table-column :label="t('firewall.colActions')" width="80" align="center">
                <template #default="{ row }">
                  <el-button link type="danger" @click="removeRule(row)">{{ t('firewall.delete') }}</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>

          <div class="fw-actions srf">
            <div class="cap ssh-hint">
              <template v-if="isFirewalld">{{ t('firewall.sshCheckHint', { port: selectedSshPort }) }}</template>
              <template v-else>{{ t('firewall.ufwApplyHint', { port: selectedSshPort }) }}</template>
            </div>
            <div class="fw-action-buttons">
              <el-button class="flat-btn" :loading="saving" :disabled="!hasUnsavedChanges || busy" @click="saveConfig">
                {{ t('firewall.saveConfig') }}
              </el-button>
              <el-button v-if="isFirewalld" type="primary" :loading="applying" :disabled="busy" @click="applyConfig">
                {{ t('firewall.applyConfig') }}
              </el-button>
            </div>
          </div>
        </template>
      </template>
      <template v-else>
        <div class="fw-empty dim-txt">{{ t('firewall.selectHostFirst') }}</div>
      </template>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'
import {
  FirewallProbe,
  FirewallSetEnabled,
  FirewallSetAutoStart,
  FirewallSaveConfig,
  FirewallApplyConfig,
  FirewallListRules
} from '../../../bindings/github.com/ys-ll/uniterm/app'
import { ElMessageBox } from 'element-plus'
import { msg } from '../../services/message'
import { useI18n } from '../../i18n'
import { useConnectionStore } from '../../stores/connectionStore'
import { useRemoteCaptureCredentials } from '../../composables/useRemoteCaptureCredentials'

defineOptions({ name: 'FirewallTabContent' })

defineProps<{ visible?: boolean }>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
}>()

const { t } = useI18n()
const connectionStore = useConnectionStore()
const { resolveRemoteCaptureCredentials, invalidRemoteCaptureCredentials } = useRemoteCaptureCredentials()

// ── State ──
const remoteConnections = computed(() => connectionStore.connections.filter(c => c.type === 'ssh'))
const selectedRemoteConn = ref('')
const status = ref<any>(null)
const rules = ref<any[]>([])
const savedRules = ref<any[]>([])
const needsApply = ref(false)
const loading = ref(false)
const busy = ref(false)
const autostartBusy = ref(false)
const saving = ref(false)
const applying = ref(false)

// 权限/安装状态：由后端返回的错误推导。
const notInstalled = ref(false)
const noPermission = ref(false)

const formPort = ref({
  action: 'allow' as 'allow' | 'deny',
  protocol: 'tcp' as 'tcp' | 'udp',
  port: '',
  source: ''
})

const canManage = computed(() => !!selectedRemoteConn.value && !!status.value && !notInstalled.value && !noPermission.value)
const canModify = computed(() => !!selectedRemoteConn.value && !notInstalled.value && !noPermission.value)
const canAdd = computed(() => !!formPort.value.port.trim() && canManage.value)
const isFirewalld = computed(() => status.value?.backend === 'firewalld')
const selectedSshPort = computed(() => {
  const conn = remoteConnections.value.find(c => c.id === selectedRemoteConn.value)
  return Number(conn?.port) || 22
})

function ruleKey(rule: any): string {
  return [rule.direction, rule.protocol, rule.port, rule.source || ''].join('|')
}

function comparableRules(items: any[]): string {
  return items.map(ruleKey).sort().join('\n')
}

const hasUnsavedChanges = computed(() => comparableRules(rules.value) !== comparableRules(savedRules.value))

function displayBackend(backend: string): string {
  if (backend === 'firewalld') return 'firewalld'
  if (backend === 'ufw') return 'ufw'
  if (backend === 'none') return t('firewall.unsupported')
  return backend || t('firewall.unknown')
}

function handleDialogClose() {
  status.value = null
  rules.value = []
  savedRules.value = []
  needsApply.value = false
  selectedRemoteConn.value = ''
  notInstalled.value = false
  noPermission.value = false
}

// 认证失败判断、凭据重试，与远端抓包一致。
function isSshAuthError(e: any): boolean {
  const m = String(e?.message || e || '')
  return /unable to authenticate|handshake failed|authentication failed|no supported methods|permission denied by server/i.test(m)
}

async function withSshAuthRetry<T>(
  op: (cred: { user: string; password: string }) => Promise<T>
): Promise<T> {
  const connId = selectedRemoteConn.value
  for (let attempt = 0; attempt < 3; attempt++) {
    const cred = await resolveRemoteCaptureCredentials(connId)
    if (cred === null) throw new Error('cancel')
    try {
      return await op(cred)
    } catch (e: any) {
      if (isSshAuthError(e)) {
        invalidRemoteCaptureCredentials(connId)
        continue
      }
      throw e
    }
  }
  throw new Error('auth-retry-exhausted')
}

function classifyError(e: any) {
  const m = String(e?.message || e || '')
  notInstalled.value = /未安装支持的防火墙|no supported firewall/i.test(m)
  noPermission.value = /permission denied|operation not permitted|access denied|not a sudoers|must be root/i.test(m)
}

async function loadStatus() {
  if (!selectedRemoteConn.value) return
  const connId = selectedRemoteConn.value
  loading.value = true
  notInstalled.value = false
  noPermission.value = false
  status.value = null
  try {
    status.value = await withSshAuthRetry((cred) => FirewallProbe(connId, cred.user, cred.password))
    if (status.value && status.value.backend === 'none') {
      notInstalled.value = true
    }
  } catch (e: any) {
    if (e?.message === 'cancel') return
    classifyError(e)
    if (notInstalled.value) return
    if (!noPermission.value) {
      msg.error(e?.message || t('firewall.loadFailed'))
    }
  } finally {
    loading.value = false
  }
  if (status.value) await loadRules()
}

async function loadRules() {
  if (!selectedRemoteConn.value || !status.value || notInstalled.value) return
  const connId = selectedRemoteConn.value
  loading.value = true
  try {
    const persisted = (await withSshAuthRetry<any[]>((cred) => FirewallListRules(connId, cred.user, cred.password))) || []
    savedRules.value = persisted.map(rule => ({ ...rule }))
    rules.value = persisted.map(rule => ({ ...rule }))
  } catch (e: any) {
    if (e?.message !== 'cancel') classifyError(e)
  } finally {
    loading.value = false
  }
}

async function loadAll() {
  await loadStatus()
}

async function refreshConfig() {
  if (hasUnsavedChanges.value) {
    try {
      await ElMessageBox.confirm(t('firewall.discardConfirm'), t('firewall.unsaved'), {
        type: 'warning',
        confirmButtonText: t('common.confirm'),
        cancelButtonText: t('common.cancel')
      })
    } catch {
      return
    }
  }
  await loadAll()
}

async function onRemoteConnChange() {
  status.value = null
  rules.value = []
  savedRules.value = []
  needsApply.value = false
  notInstalled.value = false
  noPermission.value = false
  await loadStatus()
}

async function toggleEnabled(enable: boolean) {
  if (!selectedRemoteConn.value) return
  const connId = selectedRemoteConn.value
  busy.value = true
  try {
    await withSshAuthRetry((cred) => FirewallSetEnabled(connId, cred.user, cred.password, enable))
    status.value = await withSshAuthRetry((cred) => FirewallProbe(connId, cred.user, cred.password))
    msg.success(enable ? t('firewall.enabledOk') : t('firewall.disabledOk'))
  } catch (e: any) {
    if (e?.message !== 'cancel') {
      classifyError(e)
      if (noPermission.value) msg.error(t('firewall.noPermission'))
      else msg.error(e?.message || t('firewall.opFailed'))
    }
  } finally {
    busy.value = false
  }
}

async function toggleAutoStart(enable: boolean) {
  if (!selectedRemoteConn.value) return
  const connId = selectedRemoteConn.value
  if (!enable) {
    // 关闭开机自启不影响当前已运行的防火墙，仅取消重启后的自动加载，需向用户说明。
    try {
      await ElMessageBox.confirm(t('firewall.autoStartOffConfirm'), t('firewall.autoStart'), {
        type: 'warning',
        confirmButtonText: t('common.confirm'),
        cancelButtonText: t('common.cancel')
      })
    } catch {
      return
    }
  }
  autostartBusy.value = true
  try {
    await withSshAuthRetry((cred) => FirewallSetAutoStart(connId, cred.user, cred.password, enable))
    status.value = await withSshAuthRetry((cred) => FirewallProbe(connId, cred.user, cred.password))
    msg.success(enable ? t('firewall.autoStartOnOk') : t('firewall.autoStartOffOk'))
  } catch (e: any) {
    if (e?.message !== 'cancel') {
      classifyError(e)
      if (noPermission.value) msg.error(t('firewall.noPermission'))
      else msg.error(e?.message || t('firewall.opFailed'))
    }
  } finally {
    autostartBusy.value = false
  }
}

function addRule() {
  if (!canAdd.value || busy.value) return
  const rule = {
    direction: 'in',
    protocol: formPort.value.protocol,
    port: formPort.value.port.trim(),
    source: formPort.value.source.trim(),
    persist: 'permanent'
  }
  if (formPort.value.action === 'allow') {
    if (!rules.value.some(item => ruleKey(item) === ruleKey(rule))) {
      rules.value.push(rule)
    }
  } else {
    rules.value = rules.value.filter(item => !(
      item.direction === rule.direction && item.protocol === rule.protocol && item.port === rule.port && (item.source || '') === (rule.source || '')
    ))
  }
  formPort.value.port = ''
  formPort.value.source = ''
}

function removeRule(row: any) {
  if (busy.value) return
  const key = ruleKey(row)
  rules.value = rules.value.filter(item => ruleKey(item) !== key)
}

async function saveConfig() {
  if (!selectedRemoteConn.value || !hasUnsavedChanges.value || busy.value) return
  const connId = selectedRemoteConn.value
  busy.value = true
  saving.value = true
  try {
    const desired = rules.value.map(rule => ({ ...rule, persist: 'permanent' }))
    await withSshAuthRetry((cred) => FirewallSaveConfig(connId, cred.user, cred.password, desired))
    savedRules.value = desired.map(rule => ({ ...rule }))
    needsApply.value = status.value?.backend === 'firewalld'
    msg.success(t(needsApply.value ? 'firewall.saveOk' : 'firewall.saveOkUfw'))
  } catch (e: any) {
    if (e?.message !== 'cancel') {
      classifyError(e)
      if (noPermission.value) msg.error(t('firewall.noPermission'))
      else msg.error(e?.message || t('firewall.opFailed'))
    }
  } finally {
    saving.value = false
    busy.value = false
  }
}

async function applyConfig() {
  if (!selectedRemoteConn.value || busy.value) return
  if (hasUnsavedChanges.value) {
    try {
      await ElMessageBox.confirm(t('firewall.applyUnsavedConfirm'), t('firewall.applyConfig'), {
        type: 'warning',
        confirmButtonText: t('common.confirm'),
        cancelButtonText: t('common.cancel')
      })
    } catch {
      return
    }
  }
  const connId = selectedRemoteConn.value
  busy.value = true
  applying.value = true
  try {
    await withSshAuthRetry((cred) => FirewallApplyConfig(connId, cred.user, cred.password))
    needsApply.value = false
    msg.success(t('firewall.applyOk'))
  } catch (e: any) {
    if (e?.message !== 'cancel') {
      classifyError(e)
      if (noPermission.value) msg.error(t('firewall.noPermission'))
      else msg.error(e?.message || t('firewall.opFailed'))
    }
  } finally {
    applying.value = false
    busy.value = false
  }
}

let unsub: (() => void) | undefined
onMounted(() => {
  unsub = Events.On('firewall:error', () => { /* handled via call returns */ })
})
onUnmounted(() => {
  unsub?.()
})
</script>

<style scoped>
.firewall-dialog {
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  color: var(--text-primary);
}
.firewall-dialog :deep(.el-dialog__header) {
  margin: 0;
  padding: 12px 16px;
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--border-subtle);
}
.firewall-dialog :deep(.el-dialog__title) { color: var(--text-primary); font-size: 14px; }
.firewall-dialog :deep(.el-dialog__body) {
  flex: 1;
  min-height: 0;
  padding: 12px 16px 16px;
  overflow: hidden;
  background: var(--bg-base);
}

.fw-content { display: flex; flex-direction: column; gap: 12px; height: 100%; min-height: 0; color: var(--text-primary); }
.fw-hostbar, .fw-form, .fw-actions, .fw-alert { flex-shrink: 0; }
.fw-hostbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
}
.fw-host-left { display: flex; align-items: center; gap: 8px; min-width: 0; flex: 1; }
.fw-host-select { flex: 1; }
.fw-host-switches { display: flex; align-items: center; gap: 20px; flex-shrink: 0; }
.ctl-label { color: var(--text-secondary); font-size: 12px; white-space: nowrap; }
.cap { font-size: 12px; color: var(--text-muted); }
.t { color: var(--text-primary); font-weight: 600; font-size: 14px; }
.dim-txt { color: var(--text-muted); font-size: 13px; }

.srf {
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 12px;
  background: var(--bg-elevated);
}

.fw-content :deep(.el-input),
.fw-content :deep(.el-select) {
  --el-input-bg-color: var(--bg-base);
  --el-input-border-color: var(--border-subtle);
  --el-input-hover-border-color: var(--accent);
  --el-input-focus-border-color: var(--accent);
  --el-input-text-color: var(--text-primary);
  --el-input-placeholder-color: var(--text-muted);
  --el-select-input-color: var(--text-primary);
}
.fw-content :deep(.el-input__wrapper),
.fw-content :deep(.el-select__wrapper) {
  background: var(--bg-base);
  box-shadow: 0 0 0 1px var(--border-subtle) inset;
}
.fw-content :deep(.el-input__inner),
.fw-content :deep(.el-select__selected-item) { color: var(--text-primary); }

.flat-btn.el-button {
  --el-button-bg-color: transparent;
  --el-button-border-color: var(--border-subtle);
  --el-button-text-color: var(--text-secondary);
  --el-button-hover-bg-color: var(--bg-hover);
  --el-button-hover-border-color: var(--border-subtle);
  --el-button-hover-text-color: var(--text-primary);
}
.fw-content :deep(.el-button--primary) {
  --el-button-bg-color: var(--accent);
  --el-button-border-color: var(--accent);
  --el-button-hover-bg-color: var(--accent);
  --el-button-hover-border-color: var(--accent);
}

.fw-form .fw-form-head { margin-bottom: 8px; }
.fw-switch-cell { display: flex; align-items: center; gap: 6px; }
.fw-switch-label { font-size: 12px; color: var(--text-secondary); white-space: nowrap; }
.fw-backend { margin-left: 2px; color: var(--text-secondary); }
.fw-backend-on { color: var(--success); }
.pill { padding: 1px 8px; border-radius: 999px; font-size: 12px; }
.p-on { background: var(--success-subtle); color: var(--success); }
.p-off { background: var(--danger-subtle); color: var(--danger); }
.p-pending { background: var(--warning-subtle); color: var(--warning); }

.fw-alert { margin: 0; --el-alert-bg-color: var(--bg-base); }
.fw-form-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.fw-act { width: 90px; }
.fw-proto { width: 90px; }
.fw-port { width: 140px; }
.fw-source { width: 180px; }
.form-hint { margin-top: 8px; line-height: 1.5; }

.fw-rules { display: flex; flex: 1; flex-direction: column; min-height: 120px; overflow: hidden; }
.fw-rules .fw-rules-head { display: flex; flex-shrink: 0; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.fw-table {
  flex: 1;
  width: 100%;
  min-height: 0;
  --el-table-text-color: var(--text-primary);
  --el-table-header-text-color: var(--text-secondary);
  --el-table-header-bg-color: var(--bg-base);
  --el-table-tr-bg-color: var(--bg-base);
  --el-table-bg-color: var(--bg-base);
  --el-table-border-color: var(--border-subtle);
  --el-table-row-hover-bg-color: var(--bg-hover);
}
.fw-table :deep(.el-table__empty-block) { background: var(--bg-base); }
.fw-table :deep(.el-table__cell) { border-bottom-color: var(--border-subtle); }

.fw-actions { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.ssh-hint { line-height: 1.5; }
.fw-action-buttons { display: flex; gap: 8px; flex-shrink: 0; }
.fw-empty { padding: 40px 0; text-align: center; }

@media (max-width: 760px) {
  .fw-actions { align-items: flex-start; flex-direction: column; }
  .fw-action-buttons { align-self: flex-end; }
}
</style>
