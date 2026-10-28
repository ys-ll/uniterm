<template>
  <!-- Name + group -->
  <el-form-item :label="t('conn.name')">
    <div class="name-group-row">
      <el-input v-model="form.name" :placeholder="t('conn.namePlaceholder')" class="name-input" />
      <el-tree-select
        v-model="localGroupId"
        :data="groupTreeData"
        :render-after-expand="false"
        check-strictly
        clearable
        :placeholder="t('conn.noGroup')"
        class="group-select"
      />
      <el-button class="new-group-btn" @click="emit('groupSelect', '__new__')" :title="t('conn.newGroup')">
        <Plus :size="'0.875rem'" />
      </el-button>
    </div>
  </el-form-item>
  <el-form-item v-if="form.type === 'redis'" :label="t('conn.redisMode')">
    <el-radio-group v-model="form.redisMode">
      <el-radio-button value="standalone">{{ t('conn.redisModeStandalone') }}</el-radio-button>
      <el-radio-button value="sentinel">{{ t('conn.redisModeSentinel') }}</el-radio-button>
    </el-radio-group>
  </el-form-item>
  <template v-if="isRedisSentinel">
    <el-form-item :label="t('conn.redisSentinels')" required>
      <el-input v-model="form.redisSentinels" :placeholder="t('conn.redisSentinelsPlaceholder')" />
    </el-form-item>
    <el-form-item :label="t('conn.redisMasterName')" required>
      <el-input v-model="form.redisMasterName" placeholder="mymaster" />
    </el-form-item>
  </template>
  <el-form-item :label="form.type === 's3' ? 'Endpoint' : form.type === 'webdav' ? 'URL' : t('conn.host')" required v-if="!isSqliteDb && form.type !== 'local' && form.type !== 'wsl' && form.type !== 'serial' && form.type !== 'k8s' && form.type !== 'container' && !isRedisSentinel">
    <div class="host-port-row">
      <el-input ref="hostInputRef" v-model="form.host" class="host-input" :placeholder="form.type === 's3' ? 'e.g. https://s3.amazonaws.com' : form.type === 'webdav' ? 'https://dav.example.com/dav/' : t('conn.hostPlaceholder')" />
      <template v-if="form.type !== 's3' && form.type !== 'webdav'">
        <span class="host-port-sep">:</span>
        <el-input-number v-model="form.port" :min="0" :max="65535" class="port-input" controls-position="right" />
      </template>
    </div>
  </el-form-item>
  <el-form-item v-if="isSqliteDb" :label="t('db.sqlitePath')" required>
    <div class="host-port-row">
      <el-input v-model="form.host" style="flex: 1; min-width: 0" :placeholder="t('db.sqlitePathPlaceholder')" />
      <el-button @click="emit('selectSqliteFile')">{{ t('db.sqliteBrowse') }}</el-button>
    </div>
  </el-form-item>
  <el-form-item v-if="(form.type === 'database' && form.dbType !== 'rqlite' && form.dbType !== 'sqlite' && form.dbType !== 'redis' && form.dbType !== 'elasticsearch') || form.type === 'mongodb'" :label="t('db.databases')" :required="form.dbType === 'postgres'">
    <el-input v-model="form.dbName" :placeholder="t('db.databases')" />
  </el-form-item>
  <el-form-item v-if="form.type === 'local' || form.type === 'wsl'" :label="t('conn.shell')">
    <el-select v-model="form.shellPath" filterable allow-create clearable>
      <el-option
        v-for="sh in shellOptionsForType"
        :key="sh.value"
        :label="sh.label"
        :value="sh.value"
      />
    </el-select>
  </el-form-item>
  <template v-if="form.type === 'serial'">
    <el-form-item :label="t('serial.portLabel')" required>
      <div style="display:flex;gap:0.5rem;width:100%">
        <el-select v-model="form.serialPort" :placeholder="portPlaceholder" :disabled="serialPorts.length === 0 || serialScanning" :loading="serialScanning" style="flex:1">
          <el-option v-for="p in serialPorts" :key="p" :label="p" :value="p" />
        </el-select>
        <el-button :icon="RefreshCw" :loading="serialScanning" @click="emit('scanSerialPorts')">
          {{ t('serial.scan') }}
        </el-button>
      </div>
    </el-form-item>
    <el-form-item :label="t('serial.baudRate')">
      <el-autocomplete
        v-model="serialBaudRateInput"
        :fetch-suggestions="queryBaudRateSuggestions"
        :placeholder="t('serial.baudRate')"
        clearable
        style="width:100%"
      />
    </el-form-item>
    <div class="inline-pair-row">
      <el-form-item :label="t('serial.dataBits')">
        <el-select v-model="serialDataBitsValue">
          <el-option v-for="b in [5,6,7,8]" :key="b" :label="String(b)" :value="b" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('serial.stopBits')">
        <el-select v-model="serialStopBitsValue">
          <el-option v-for="b in [1,1.5,2]" :key="b" :label="String(b)" :value="b" />
        </el-select>
      </el-form-item>
    </div>
    <el-form-item :label="t('serial.parity')">
      <el-select v-model="serialParityValue">
        <el-option :label="t('serial.parityNone')" value="none" />
        <el-option :label="t('serial.parityOdd')" value="odd" />
        <el-option :label="t('serial.parityEven')" value="even" />
        <el-option :label="t('serial.parityMark')" value="mark" />
        <el-option :label="t('serial.paritySpace')" value="space" />
      </el-select>
    </el-form-item>
  </template>
  <template v-if="form.type === 'smb'">
    <el-form-item :label="t('conn.smbDomain')" required>
      <el-input v-model="form.smbDomain" placeholder="e.g. WORKGROUP" />
    </el-form-item>
    <el-form-item :label="t('conn.smbShare')">
      <el-input v-model="form.smbShare" placeholder="Share name (leave empty to browse all)" />
    </el-form-item>
  </template>
  <template v-if="form.type === 's3'">
    <el-form-item :label="t('conn.s3Region')" required>
      <el-input v-model="form.s3Region" placeholder="us-east-1" />
    </el-form-item>
    <el-form-item :label="t('conn.s3Bucket')">
      <el-input v-model="form.s3Bucket" placeholder="my-bucket (leave empty to list all buckets)" />
    </el-form-item>
    <el-form-item :label="t('conn.s3UrlStyle')">
      <el-select v-model="form.s3UrlStyle">
        <el-option label="Virtual-hosted (https://bucket.endpoint/key)" value="virtual" />
        <el-option label="Path (https://endpoint/bucket/key)" value="path" />
      </el-select>
    </el-form-item>
  </template>
  <!-- ── K8s 字段 ── -->
  <template v-if="form.type === 'k8s'">
    <el-form-item :label="t('conn.k8sConfigSource')">
      <el-radio-group v-model="k8sSourceMode">
        <el-radio-button value="inline">{{ t('conn.k8sConfigSourceInline') }}</el-radio-button>
        <el-radio-button value="file">{{ t('conn.k8sConfigSourceFile') }}</el-radio-button>
      </el-radio-group>
    </el-form-item>

    <el-form-item v-if="k8sSourceMode === 'file'" :label="t('conn.k8sConfigPath')">
      <el-input v-model="form.k8sConfigPath" placeholder="~/.kube/config">
        <template #append>
          <el-button @click="emit('pickKubeconfigFile')">
            <el-icon><FolderOpen :size="'1rem'" /></el-icon>
          </el-button>
        </template>
      </el-input>
    </el-form-item>

    <el-form-item v-else :label="t('conn.k8sConfigInline')">
      <template v-if="!k8sConfigRevealed">
        <el-button size="small" @click="emit('update:k8sConfigRevealed', true)">
          <el-icon><Eye :size="'0.875rem'" /></el-icon>
          <span style="margin-left: 0.25rem">{{ t('conn.k8sConfigReveal') }}</span>
        </el-button>
      </template>
      <template v-else>
        <SyntaxEditor
          :model-value="form.k8sConfigInline ?? ''"
          lang="yaml"
          class="kubeconfig-editor"
          @update:model-value="form.k8sConfigInline = $event"
        />
        <div class="key-content-actions">
          <el-button size="small" @click="emit('update:k8sConfigRevealed', false)">
            <el-icon><EyeOff :size="'0.875rem'" /></el-icon>
            <span style="margin-left: 0.25rem">{{ t('conn.k8sConfigHide') }}</span>
          </el-button>
          <el-button size="small" @click="emit('importKubeconfigText')">
            <el-icon><FolderOpen :size="'0.875rem'" /></el-icon>
            <span style="margin-left: 0.25rem">{{ t('conn.importFromFile') }}</span>
          </el-button>
        </div>
      </template>
    </el-form-item>

    <el-form-item :label="t('conn.k8sContext')">
      <div style="display: flex; align-items: center; gap: 0.5rem; width: 100%">
        <el-select v-model="form.k8sContext" filterable :placeholder="k8sContextsError || ''" :loading="k8sContextsLoading" style="flex: 1">
          <el-option v-for="c in k8sContexts" :key="c.name" :value="c.name" :label="c.current ? c.name + ' (current)' : c.name" />
        </el-select>
        <el-button @click="emit('reloadK8sContexts')" :loading="k8sContextsLoading" :title="t('conn.k8sReloadContexts')">
          <el-icon><RefreshCw :size="'1rem'" /></el-icon>
        </el-button>
      </div>
    </el-form-item>

    <el-form-item :label="t('conn.k8sNamespace')">
      <el-select
        v-model="form.k8sNamespace"
        filterable
        allow-create
        default-first-option
        clearable
        placeholder="Namespace"
        style="width: 100%"
        @change="emit('k8sNamespaceInput')"
      >
        <el-option v-for="ns in k8sNamespaceOptions" :key="ns" :value="ns" :label="ns" />
      </el-select>
    </el-form-item>
  </template>
  <!-- ── Container 字段 ── -->
  <template v-if="form.type === 'container'">
    <el-form-item :label="t('conn.containerTransport')">
      <el-radio-group v-model="form.containerTransport">
        <el-radio-button value="ssh">{{ t('conn.transportSSH') }}</el-radio-button>
        <el-radio-button value="local">{{ t('conn.transportLocal') }}</el-radio-button>
      </el-radio-group>
    </el-form-item>
    <el-form-item v-if="form.containerTransport === 'ssh'" :label="t('conn.containerSSHRef')" required>
      <el-select
        v-model="form.containerSSHConnId"
        :placeholder="t('conn.containerSSHRefPlaceholder')"
        filterable
      >
        <el-option
          v-for="c in sshConnections"
          :key="c.id"
          :label="`${c.name} (${c.user}@${c.host}:${c.port})`"
          :value="c.id"
        />
      </el-select>
      <div class="field-hint">
        {{ sshConnections.length ? t('conn.containerSSHRefHint') : t('conn.containerNoSSH') }}
      </div>
    </el-form-item>
    <el-form-item v-if="form.containerTransport === 'local'">
      <div class="field-hint">{{ t('conn.containerLocalHint') }}</div>
    </el-form-item>
  </template>
  <template v-if="form.type === 'rdp' && isWindows">
    <el-form-item :label="t('rdp.resolution')">
      <el-select v-model="rdpResolution" placeholder="1280×720">
        <el-option
          v-for="r in rdpResolutionOptions"
          :key="r.value"
          :label="r.label"
          :value="r.value"
        />
      </el-select>
    </el-form-item>
    <el-form-item v-if="rdpResolution === rdpCustomResolutionConst" :label="t('rdp.customResolution')">
      <div class="rdp-custom-resolution">
        <el-input-number v-model="rdpCustomWidth" :min="320" :max="7680" :step="8" controls-position="right" />
        <span class="rdp-custom-resolution-x">×</span>
        <el-input-number v-model="rdpCustomHeight" :min="240" :max="4320" :step="8" controls-position="right" />
      </div>
    </el-form-item>
  </template>
  <template v-if="form.type === 'x11-desktop'">
    <el-form-item :label="t('conn.x11DesktopDE')" required>
      <el-select v-model="form.x11DesktopDesktopType">
        <el-option label="GNOME" value="gnome" />
        <el-option label="KDE" value="kde" />
        <el-option label="XFCE" value="xfce" />
        <el-option label="MATE" value="mate" />
        <el-option label="Cinnamon" value="cinnamon" />
        <el-option label="Openbox" value="openbox" />
        <el-option :label="t('conn.x11DesktopDECustom')" value="custom" />
      </el-select>
    </el-form-item>
    <el-form-item v-if="form.x11DesktopDesktopType === 'custom'"
                  :label="t('conn.x11DesktopCustomCmd')" required>
      <el-input v-model="form.x11DesktopCustomCmd"
                :placeholder="t('conn.x11DesktopCustomCmdPlaceholder')" />
      <div class="field-hint">{{ t('conn.x11DesktopCustomCmdHint') }}</div>
    </el-form-item>
  </template>
  <el-form-item :label="t('conn.remark')">
    <el-input v-model="form.remark" type="textarea" :rows="3" />
  </el-form-item>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElInput } from 'element-plus'
import { Plus, FolderOpen, Eye, EyeOff, RefreshCw } from '@lucide/vue'
import { useI18n } from '../../i18n'
import type { ConnectionConfig } from '../../types/session'
import type { K8sContextInfo } from '../../types/k8s'
import SyntaxEditor from '../SyntaxEditor.vue'

const { t } = useI18n()

interface TreeOption {
  value: string
  label: string
  children?: TreeOption[]
}

const props = defineProps<{
  form: ConnectionConfig
  selectedGroupId: string | undefined
  groupTreeData: TreeOption[]
  isWindows: boolean
  shellOptionsForType: { label: string; value: string }[]
  serialPorts: string[]
  serialScanning: boolean
  serialBaudRateInput: string
  serialDataBitsValue: number
  serialStopBitsValue: number
  serialParityValue: string
  portPlaceholder: string
  sshConnections: { id: string; name: string; user: string; host: string; port: number }[]
  k8sSourceMode: 'file' | 'inline'
  k8sContexts: K8sContextInfo[]
  k8sContextsLoading: boolean
  k8sContextsError: string
  k8sNamespaceOptions: string[]
  k8sConfigRevealed: boolean
  rdpResolution: string
  rdpResolutionOptions: { value: string; label: string }[]
  rdpCustomWidth: number
  rdpCustomHeight: number
  rdpCustomResolutionConst: string
}>()

const emit = defineEmits<{
  'update:selectedGroupId': [value: string | undefined]
  'update:serialBaudRateInput': [value: string]
  'update:serialDataBitsValue': [value: number]
  'update:serialStopBitsValue': [value: number]
  'update:serialParityValue': [value: string]
  'update:k8sSourceMode': [value: 'file' | 'inline']
  'update:k8sConfigRevealed': [value: boolean]
  'update:rdpResolution': [value: string]
  'update:rdpCustomWidth': [value: number]
  'update:rdpCustomHeight': [value: number]
  groupSelect: [value: string]
  selectSqliteFile: []
  scanSerialPorts: []
  pickKubeconfigFile: []
  importKubeconfigText: []
  reloadK8sContexts: []
  k8sNamespaceInput: []
}>()

// Local two-way bound mirrors of the v-model props (the parent owns the
// canonical refs; we just bridge el-* v-model writes back via emits).
const localGroupId = computed({
  get: () => props.selectedGroupId,
  set: (v) => emit('update:selectedGroupId', v),
})
const serialBaudRateInput = computed({
  get: () => props.serialBaudRateInput,
  set: (v) => emit('update:serialBaudRateInput', v),
})
const serialDataBitsValue = computed({
  get: () => props.serialDataBitsValue,
  set: (v) => emit('update:serialDataBitsValue', v),
})
const serialStopBitsValue = computed({
  get: () => props.serialStopBitsValue,
  set: (v) => emit('update:serialStopBitsValue', v),
})
const serialParityValue = computed({
  get: () => props.serialParityValue,
  set: (v) => emit('update:serialParityValue', v),
})
const k8sSourceMode = computed({
  get: () => props.k8sSourceMode,
  set: (v) => emit('update:k8sSourceMode', v),
})
const rdpResolution = computed({
  get: () => props.rdpResolution,
  set: (v) => emit('update:rdpResolution', v),
})
const rdpCustomWidth = computed({
  get: () => props.rdpCustomWidth,
  set: (v) => emit('update:rdpCustomWidth', v),
})
const rdpCustomHeight = computed({
  get: () => props.rdpCustomHeight,
  set: (v) => emit('update:rdpCustomHeight', v),
})

// Derived flags that only depend on `form` — recomputed locally to avoid
// threading them through props.
const isSqliteDb = computed(() => props.form.type === 'database' && props.form.dbType === 'sqlite')
const isRedisSentinel = computed(() => props.form.type === 'redis' && props.form.redisMode === 'sentinel')

// Baud-rate autocomplete suggestions (presets list mirrors the original).
const baudRatePresets = [300, 1200, 2400, 4800, 9600, 14400, 19200, 38400, 57600, 115200, 230400, 460800, 921600]
function queryBaudRateSuggestions(queryString: string, cb: (results: { value: string }[]) => void) {
  const suggestions = baudRatePresets
    .filter(r => String(r).includes(queryString))
    .map(r => ({ value: String(r) }))
  cb(suggestions)
}

// Host input ref is owned locally and exposed so the parent can focus it on
// dialog open (the input lives inside this child).
const hostInputRef = ref<InstanceType<typeof ElInput> | null>(null)
defineExpose({ hostInputRef })
</script>

<style scoped>
/* ── Name + group row ── */
.name-group-row {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  width: 100%;
}
.name-input {
  flex: 1;
  min-width: 0;
}
.group-select {
  width: 10rem;
  flex-shrink: 0;
}
.new-group-btn {
  flex-shrink: 0;
  width: 2rem;
  height: 2rem;
  padding: 0;
}
/* ── Host + port row ── */
.host-port-row {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  width: 100%;
}
.host-input {
  width: calc(100% - 9.375rem) !important;
}
.host-port-sep {
  color: var(--text-muted);
  font-weight: 500;
}
.port-input {
  width: 8.125rem !important;
  flex-shrink: 0;
}
/* ── Field hint text ── */
.field-hint {
  color: var(--text-muted);
  font-size: 0.75rem;
  line-height: 1.4;
}
/* ── RDP custom resolution inputs ── */
.rdp-custom-resolution {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
}
.rdp-custom-resolution .el-input-number {
  flex: 1;
}
.rdp-custom-resolution-x {
  color: var(--text-muted);
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
/* Inline kubeconfig YAML editor */
.kubeconfig-editor {
  height: 8.75rem;
  width: 100%;
}
.key-content-actions {
  margin-top: 0.375rem;
}
.key-content-actions .el-button + .el-button {
  margin-left: 0.5rem;
}
</style>