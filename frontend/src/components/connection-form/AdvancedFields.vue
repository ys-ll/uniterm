<template>
  <div v-if="showAdvancedToggle" class="advanced-toggle" @click="emit('update:showAdvanced', !showAdvanced)">
    <el-icon class="advanced-arrow" :class="{ expanded: showAdvanced }"><ChevronRight :size="'0.875rem'" /></el-icon>
    <span>{{ t('conn.advanced') }}</span>
  </div>
  <template v-if="showAdvanced">
    <template v-if="advGroupProtocol">
      <div class="adv-grid">
        <div class="adv-group-title">{{ t('conn.advGroupProtocol') }}</div>
        <el-form-item v-if="isSSHAlgoType" :label="t('conn.sshAlgoMode')">
          <el-radio-group v-model="sshAlgoMode">
            <el-radio-button value="compatible">{{ t('conn.sshAlgoModeCompatible') }}</el-radio-button>
            <el-radio-button value="secure">{{ t('conn.sshAlgoModeSecure') }}</el-radio-button>
            <el-radio-button value="custom">{{ t('conn.sshAlgoModeCustom') }}</el-radio-button>
          </el-radio-group>
          <el-button v-if="sshAlgoMode === 'custom'" size="small" style="margin-left: 0.75rem;" @click="emit('openSSHAlgoDialog')">
            {{ t('conn.sshAlgoConfigure') }}
          </el-button>
        </el-form-item>
        <el-form-item v-if="form.type === 'ssh'" :label="t('conn.agentForwarding')">
          <el-switch v-model="form.agentForwarding" />
          <span class="field-hint hint-after-switch">{{ t('conn.agentForwardingDesc') }}</span>
        </el-form-item>
        <el-form-item v-if="form.type === 'ssh' && !isMobilePlatform()" :label="t('conn.x11Forwarding')">
          <el-switch v-model="form.x11Forwarding" />
          <span v-if="x11HintKey" class="field-hint hint-after-switch">{{ t(x11HintKey) }}</span>
        </el-form-item>
        <div class="inline-pair-row">
          <el-form-item v-if="form.type === 'ssh'" :label="t('conn.fileTransferProto')">
            <el-radio-group v-model="form.fileTransferProto">
              <el-radio-button value="sftp">SFTP</el-radio-button>
              <el-radio-button value="scp">SCP</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="form.type === 'ssh' || form.type === 'sftp' || form.type === 'scp'" :label="t('conn.sftpMaxConcurrency')">
            <el-input-number v-model="form.sftpMaxConcurrency" :min="0" :max="20" controls-position="right" style="width: 8rem" />
          </el-form-item>
        </div>
        <el-form-item v-if="form.type === 'database' || form.type === 'mongodb'" :label="t('db.params')">
          <el-input v-model="form.dbParams" :placeholder="defaultParamsHint" style="width:100%" />
        </el-form-item>
        <el-form-item v-if="form.type === 'redis'" :label="t('conn.redisKeySeparator')">
          <el-input v-model="form.redisKeySeparator" style="width: 100%" />
        </el-form-item>
        <el-form-item v-if="isElasticsearch" :label="t('conn.esPathPrefix')">
          <el-input v-model="form.esPathPrefix" placeholder="/es" />
        </el-form-item>
        <template v-if="form.type === 'ftp'">
          <el-form-item :label="t('conn.ftpEncryption')">
            <el-select v-model="form.ftpEncryption">
              <el-option :label="t('conn.ftpEncryptionNone')" value="none" />
              <el-option :label="t('conn.ftpEncryptionAuto')" value="auto" />
              <el-option :label="t('conn.ftpEncryptionRequired')" value="required" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="form.ftpEncryption !== 'none'" :label="t('conn.ftpSkipVerify')">
            <el-switch v-model="form.ftpSkipVerify" />
            <span class="field-hint hint-after-switch">{{ t('conn.ftpSkipVerifyDesc') }}</span>
          </el-form-item>
          <el-form-item :label="t('conn.ftpPassive')">
            <el-switch v-model="form.ftpPassive" />
          </el-form-item>
        </template>
        <template v-if="form.type === 'vnc'">
          <el-form-item :label="t('conn.vncShared')">
            <el-switch v-model="form.vncShared" />
          </el-form-item>
          <el-form-item :label="t('conn.vncRepeaterID')">
            <el-input v-model="form.vncRepeaterID" :placeholder="t('conn.vncRepeaterIDPlaceholder')" />
          </el-form-item>
        </template>
        <template v-if="form.type === 'rdp' && isWindows">
          <el-form-item :label="t('conn.rdpAdminSession')">
            <el-select v-model="form.rdpAdminSession" style="width: 100%">
              <el-option :value="false" :label="t('conn.rdpAdminSessionOff')" />
              <el-option :value="true" :label="t('conn.rdpAdminSessionOn')" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('conn.rdpSmartSizing')">
            <el-switch v-model="form.rdpSmartSizing" />
          </el-form-item>
        </template>
        <template v-if="form.type === 'telnet'">
          <el-form-item :label="t('conn.telnetNegotiationMode')">
            <el-select v-model="form.telnetNegotiationMode">
              <el-option :label="t('conn.telnetNegotiationActive')" value="active" />
              <el-option :label="t('conn.telnetNegotiationPassive')" value="passive" />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('conn.telnetSendMode')">
            <el-select v-model="form.telnetSendMode">
              <el-option :label="t('conn.telnetSendModeChar')" value="character" />
              <el-option :label="t('conn.telnetSendModeLine')" value="line" />
            </el-select>
          </el-form-item>
        </template>
      </div>
    </template>
    <template v-if="advGroupTerminal">
      <div class="adv-grid">
        <div class="adv-group-title">{{ t('conn.advGroupTerminal') }}</div>
        <el-form-item
          v-if="form.type === 'ssh' || form.type === 'telnet' || form.type === 'serial' || form.type === 'mosh' || form.type === 'local' || form.type === 'wsl' || form.type === 'tcp' || form.type === 'ftp'"
          :label="t('conn.encoding')"
        >
          <el-select v-model="form.encoding" placeholder="Unicode (UTF-8)">
            <el-option label="Unicode (UTF-8)" value="utf-8" />
            <el-option label="Simplified Chinese (GBK)" value="gbk" />
            <el-option label="Simplified Chinese (GB2312)" value="gb2312" />
            <el-option label="Simplified Chinese (GB18030)" value="gb18030" />
            <el-option label="Traditional Chinese (Big5)" value="big5" />
            <el-option label="Japanese (Shift-JIS)" value="shift-jis" />
            <el-option label="Japanese (EUC-JP)" value="euc-jp" />
            <el-option label="Korean (EUC-KR)" value="euc-kr" />
            <el-option label="Latin-1" value="latin-1" />
          </el-select>
        </el-form-item>
        <el-form-item
          v-if="form.type === 'ssh' || form.type === 'telnet' || form.type === 'local'"
          :label="t('conn.termType')"
        >
          <el-select v-model="form.termType">
            <el-option label="xterm-256color" value="xterm-256color" />
            <el-option label="xterm" value="xterm" />
            <el-option label="vt100" value="vt100" />
            <el-option label="vt220" value="vt220" />
            <el-option label="vt320" value="vt320" />
            <el-option label="ansi" value="ansi" />
            <el-option label="linux" value="linux" />
          </el-select>
        </el-form-item>
        <el-form-item
          v-if="form.type === 'ssh' || form.type === 'telnet' || form.type === 'serial' || form.type === 'mosh' || form.type === 'tcp'"
          :label="t('conn.backspaceKey')"
        >
          <el-select v-model="form.backspaceKey">
            <el-option label="Backspace (Ctrl+H)" value="bs" />
            <el-option label="ASCII 127 (Ctrl+?)" value="del" />
            <el-option label="VT220 Del (Esc[3~)" value="vt220" />
          </el-select>
        </el-form-item>
        <template v-if="form.type === 'telnet' || form.type === 'serial' || form.type === 'tcp'">
          <div class="inline-pair-row">
            <el-form-item :label="t('conn.localEcho')">
              <el-switch v-model="form.localEcho" />
            </el-form-item>
            <el-form-item :label="t('conn.newlineMode')">
              <el-select v-model="form.newlineMode">
                <el-option label="CR" value="cr" />
                <el-option label="CR+LF" value="crlf" />
              </el-select>
            </el-form-item>
          </div>
        </template>
        <el-form-item v-if="form.type === 'ssh' || form.type === 'telnet' || form.type === 'mosh' || form.type === 'local' || form.type === 'wsl'" :label="t('conn.postLoginScript')">
          <div class="post-login-config">
            <el-radio-group v-if="form.type === 'ssh'" v-model="postLoginMode" size="small">
              <el-radio-button value="script">{{ t('conn.postLoginModeScript') }}</el-radio-button>
              <el-radio-button value="expect">{{ t('conn.postLoginModeExpect') }}</el-radio-button>
            </el-radio-group>
            <el-input
              v-if="form.type !== 'ssh' || postLoginMode === 'script'"
              v-model="form.postLoginScript"
              type="textarea"
              :rows="3"
              :placeholder="t('conn.postLoginScriptPlaceholder')"
            />
            <div v-else class="expect-steps">
              <div class="expect-table">
                <div class="expect-row expect-head">
                  <span></span>
                  <span>{{ t('conn.expectColExpect') }}</span>
                  <span>{{ t('conn.expectColSend') }}</span>
                  <span>{{ t('conn.expectColTimeout') }}</span>
                  <span>{{ t('conn.expectEnter') }}</span>
                  <span></span>
                </div>
                <div
                  v-for="(step, idx) in form.postLoginExpectSteps"
                  :key="idx"
                  class="expect-row"
                >
                  <span class="step-index">{{ idx + 1 }}</span>
                  <el-input
                    v-model="step.expect"
                    :placeholder="t('conn.expectPlaceholder')"
                    class="expect-input"
                  />
                  <el-input
                    v-model="step.send"
                    :placeholder="t('conn.sendPlaceholder')"
                    class="send-input"
                  />
                  <el-input-number
                    v-model="step.timeoutSecond"
                    :min="1"
                    :max="120"
                    :controls="false"
                    class="timeout-input"
                  />
                  <el-checkbox v-model="step.enter" class="enter-check" />
                  <el-button
                    link
                    type="danger"
                    class="remove-step-btn"
                    :title="t('conn.expectRemoveStep')"
                    @click="removeExpectStep(idx)"
                  >
                    <Trash2 :size="'0.875rem'" />
                  </el-button>
                </div>
              </div>
              <el-button class="add-step-btn" @click="addExpectStep">
                <Plus :size="'0.875rem'" />
                {{ t('conn.expectAddStep') }}
              </el-button>
              <div class="expect-help">{{ t('conn.expectVariableHint') }}</div>
            </div>
          </div>
        </el-form-item>
        <el-form-item
          v-if="['ssh','telnet','serial','mosh','local','wsl','tcp'].includes(form.type)"
          :label="t('conn.logOnConnect')"
        >
          <el-switch v-model="form.logOnConnect" />
          <span class="field-hint hint-after-switch">{{ t('conn.logOnConnectDesc') }}</span>
        </el-form-item>
      </div>
    </template>
    <template v-if="advGroupNetwork">
      <div class="adv-grid">
        <div class="adv-group-title">{{ t('conn.advGroupNetwork') }}</div>
        <el-form-item v-if="showTunnel" :label="t('conn.tunnel')">
          <el-select
            v-model="form.tunnelSSHConnId"
            :placeholder="t('conn.tunnelPlaceholder')"
            clearable
            filterable
          >
            <el-option
              v-for="c in sshConnections"
              :key="c.id"
              :label="`${c.name} (${c.user}@${c.host}:${c.port})`"
              :value="c.id"
            />
          </el-select>
        </el-form-item>
        <ProxyFields :form="form" @openNewProxyDialog="emit('openNewProxyDialog')" />
      </div>
    </template>
  </template>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ChevronRight, Plus, Trash2 } from '@lucide/vue'
import { useI18n } from '../../i18n'
import { isMobilePlatform } from '../../utils/platform'
import type { ConnectionConfig } from '../../types/session'
import ProxyFields from './ProxyFields.vue'

const { t } = useI18n()

const props = defineProps<{
  form: ConnectionConfig
  isWindows: boolean
  platform: string
  /** Whether the advanced section is expanded. */
  showAdvanced: boolean
  /** 'script' | 'expect' radio for post-login automation. */
  postLoginMode: 'script' | 'expect'
  /** SSH algorithm mode ('compatible' | 'secure' | 'custom'). */
  sshAlgoMode: 'compatible' | 'secure' | 'custom'
  /** SSH connections available as tunnel jump hosts. */
  sshConnections: { id: string; name: string; user: string; host: string; port: number }[]
}>()

const emit = defineEmits<{
  'update:showAdvanced': [value: boolean]
  'update:postLoginMode': [value: 'script' | 'expect']
  'update:sshAlgoMode': [value: 'compatible' | 'secure' | 'custom']
  openSSHAlgoDialog: []
  openNewProxyDialog: []
}>()

// Two-way bridges for v-model props.
const postLoginMode = computed({
  get: () => props.postLoginMode,
  set: (v) => emit('update:postLoginMode', v),
})
const sshAlgoMode = computed({
  get: () => props.sshAlgoMode,
  set: (v) => emit('update:sshAlgoMode', v),
})

// Derived flags that only depend on `form`.
const isSqliteDb = computed(() => props.form.type === 'database' && props.form.dbType === 'sqlite')
const isElasticsearch = computed(() => props.form.type === 'elasticsearch')
const isSSHAlgoType = computed(() => ['ssh', 'scp', 'sftp', 'mosh', 'x11-desktop'].includes(props.form.type))

const TUNNEL_UNSUPPORTED = ['spice', 'mosh', 'local', 'wsl', 'serial', 'container']
const showTunnel = computed(() => !TUNNEL_UNSUPPORTED.includes(props.form.type) && !isSqliteDb.value)
const showProxy = computed(() => ['ssh', 'sftp', 'scp', 'monitor'].includes(props.form.type))
const showAdvancedToggle = computed(() =>
  showTunnel.value || props.form.type === 'ssh' || props.form.type === 'sftp' || props.form.type === 'scp' || props.form.type === 'telnet' || props.form.type === 'mosh' || props.form.type === 'local' || props.form.type === 'wsl' || props.form.type === 'serial' || props.form.type === 'ftp' || props.form.type === 'database' || props.form.type === 'elasticsearch' || props.form.type === 'x11-desktop'
)

// Advanced pane visibility: a pane renders only when at least one of its
// fields applies to the selected type.
const advGroupTerminal = computed(() =>
  ['ssh', 'telnet', 'serial', 'mosh', 'local', 'wsl', 'tcp', 'ftp'].includes(props.form.type)
)
const advGroupNetwork = computed(() => showProxy.value || showTunnel.value)
const advGroupProtocol = computed(() =>
  ['database', 'mongodb', 'redis', 'elasticsearch', 'ftp', 'vnc', 'telnet'].includes(props.form.type) ||
  (props.form.type === 'rdp' && props.isWindows) ||
  isSSHAlgoType.value
)

const x11HintKey = computed(() => {
  if (props.platform === 'darwin') return 'conn.x11ForwardingDescMac'
  return ''
})

const defaultParamsHint = computed(() => {
  if (props.form.type === 'mongodb') return `${t('db.paramsDefault')}: authSource=admin&directConnection=true`
  switch (props.form.dbType) {
    case 'mysql': return `${t('db.paramsDefault')}: charset=utf8mb4`
    case 'postgres': return `${t('db.paramsDefault')}: sslmode=disable`
    case 'sqlserver': return `${t('db.paramsDefault')}: encrypt=disable`
    case 'sqlite': return `${t('db.paramsExample')}: journal_mode=WAL`
    default: return ''
  }
})

// Expect-step editors operate directly on the reactive form array.
function addExpectStep() {
  if (!props.form.postLoginExpectSteps) {
    props.form.postLoginExpectSteps = []
  }
  props.form.postLoginExpectSteps.push({
    expect: '',
    send: '',
    enter: true,
    timeoutSecond: 10
  })
}

function removeExpectStep(index: number) {
  props.form.postLoginExpectSteps?.splice(index, 1)
}
</script>

<style scoped>
/* ── Advanced toggle ── */
.advanced-toggle {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.625rem 0 0.5rem;
  margin-bottom: 0.25rem;
  user-select: none;
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 0.8125rem;
  font-weight: 500;
  transition: color 0.15s;
}
.advanced-toggle:hover {
  color: var(--accent);
}
.advanced-arrow {
  transition: transform 0.2s;
  display: inline-flex;
  align-items: center;
}
.advanced-arrow.expanded {
  transform: rotate(90deg);
}
/* ── Advanced groups ── */
.adv-group-title {
  margin: 0 0 0.375rem;
  color: var(--text-muted);
  font-size: 0.75rem;
  font-weight: 600;
  user-select: none;
}
.adv-grid {
  display: grid;
  grid-template-columns: 1fr;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 0.625rem 0.875rem 0;
  margin-bottom: 0.75rem;
}
/* Two short fields share one row */
.inline-pair-row {
  display: flex;
  gap: 1rem;
}
.inline-pair-row .el-form-item {
  flex: 1;
  min-width: 0;
}
/* ── Field hint text ── */
.field-hint {
  color: var(--text-muted);
  font-size: 0.75rem;
  line-height: 1.4;
}
.hint-after-switch {
  flex: 1;
  min-width: 0;
  margin-left: 0.75rem;
}
/* ── Post-login config ── */
.post-login-config {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  width: 100%;
}
.expect-steps {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.expect-table {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  overflow: hidden;
}
.expect-row {
  display: grid;
  grid-template-columns: 1.625rem minmax(5rem, 1fr) minmax(5.625rem, 1fr) 4rem 2.5rem 1.875rem;
  align-items: stretch;
}
.expect-row:not(:last-child) {
  border-bottom: 1px solid var(--border-subtle);
}
.expect-row > * {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
}
.expect-row > *:not(:last-child) {
  border-right: 1px solid var(--border-subtle);
}
.expect-head {
  background: var(--bg-elevated);
  font-size: 0.75rem;
  line-height: 1.2;
  color: var(--text-secondary);
}
.expect-head > span {
  padding: 0.1875rem 0.25rem;
}
.expect-row :deep(.el-input__wrapper),
.expect-row :deep(.el-input-number .el-input__wrapper) {
  box-shadow: none;
  border-radius: 0;
}
.expect-input,
.send-input,
.timeout-input {
  width: 100%;
}
.timeout-input :deep(.el-input__inner) {
  text-align: center;
}
.step-index {
  color: var(--text-muted);
  font-size: 0.75rem;
}
.remove-step-btn {
  min-width: 0;
}
.add-step-btn {
  align-self: flex-start;
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
}
.expect-help {
  color: var(--text-muted);
  font-size: 0.75rem;
  line-height: 1.4;
}
</style>