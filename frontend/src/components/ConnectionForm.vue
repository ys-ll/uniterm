<template>
  <el-dialog append-to-body v-model="visible" :title="isEdit ? t('conn.editTitle') : t('conn.newTitle')" width="680px" class="conn-dialog">
    <div class="conn-layout">
      <!-- Left sidebar: category icons -->
      <div class="conn-categories">
        <div
          v-for="cat in categories"
          :key="cat.key"
          class="cat-item"
          :class="{ active: category === cat.key }"
          @click="onCategorySelect(cat.key)"
        >
          <component :is="cat.icon" :size="20" />
          <span>{{ cat.label }}</span>
        </div>
      </div>

      <!-- Right content: sub-type grid + form -->
      <div class="conn-main">
        <!-- Sub-type icon grid -->
        <div class="subtype-grid">
          <button
            v-for="st in currentSubTypes"
            :key="st.type + (st.dbType || '')"
            class="subtype-btn"
            :class="{ active: isSubTypeActive(st) }"
            @click="selectType(st)"
          >
            <component :is="st.icon" :size="18" />
            <span>{{ st.label }}</span>
          </button>
        </div>

        <!-- Form fields -->
        <div class="conn-fields">
          <el-form :model="form" label-width="90px" @submit.prevent="onSave">
            <el-form-item :label="t('conn.name')">
              <el-input v-model="form.name" :placeholder="t('conn.namePlaceholder')" />
            </el-form-item>
            <el-form-item :label="t('conn.group')">
              <div style="display:flex;gap:6px;width:100%">
                <el-tree-select
                  v-model="selectedGroupId"
                  :data="groupTreeData"
                  :render-after-expand="false"
                  check-strictly
                  clearable
                  :placeholder="t('conn.noGroup')"
                  style="flex:1;min-width:0"
                />
                <el-button style="flex-shrink:0;width:32px;height:32px;padding:0" @click="onGroupSelect('__new__')" :title="t('conn.newGroup')">
                  <Plus :size="14" />
                </el-button>
              </div>
            </el-form-item>
            <el-form-item :label="form.type === 's3' ? 'Endpoint' : form.type === 'webdav' ? 'URL' : t('conn.host')" required v-if="form.type !== 'local' && form.type !== 'serial'">
              <div class="host-port-row">
                <el-input v-model="form.host" class="host-input" :placeholder="form.type === 's3' ? 'e.g. s3.amazonaws.com' : form.type === 'webdav' ? 'https://dav.example.com/dav/' : t('conn.hostPlaceholder')" />
                <template v-if="form.type !== 's3' && form.type !== 'webdav'">
                  <span class="host-port-sep">:</span>
                  <el-input-number v-model="form.port" :min="0" :max="65535" class="port-input" />
                </template>
              </div>
            </el-form-item>
            <el-form-item v-if="form.type !== 'vnc' && form.type !== 'spice' && !(form.type === 'database' && form.dbType === 'rqlite') && form.type !== 'local' && form.type !== 'serial'" :label="form.type === 's3' ? 'Access Key' : t('conn.user')">
              <el-input v-model="form.user" :placeholder="form.type === 's3' ? 'Access Key ID' : t('conn.userPlaceholder')" />
            </el-form-item>
            <el-form-item v-if="form.type === 'ssh' || form.type === 'mosh'" :label="t('conn.authType')">
              <el-radio-group v-model="form.authType">
                <el-radio-button label="password">{{ t('conn.password') }}</el-radio-button>
                <el-radio-button label="key">{{ t('conn.keyPath') }}</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <template v-if="form.type === 'rdp' && isWindows">
              <el-form-item :label="t('conn.rdpEnableNLA')">
                <div class="nla-row">
                  <el-switch v-model="form.rdpEnableNLA" />
                  <span class="field-hint">{{ form.rdpEnableNLA ? t('conn.rdpEnableNLAOnHint') : t('conn.rdpEnableNLAOffHint') }}</span>
                </div>
              </el-form-item>
            </template>
            <el-form-item v-if="form.type !== 'local' && form.type !== 'serial' && ((form.authType === 'password' && form.type !== 'rdp') || (form.type === 'rdp' && !form.rdpEnableNLA) || form.type === 'vnc' || form.type === 'spice' || form.type === 'database' || form.type === 'telnet' || form.type === 'ftp' || form.type === 'smb' || form.type === 'webdav' || form.type === 's3') && !(form.type === 'database' && form.dbType === 'rqlite')" :label="form.type === 's3' ? 'Secret Key' : t('conn.password')">
              <el-input v-model="form.password" type="password" show-password :key="passwordInputKey" :placeholder="form.type === 's3' ? 'Secret Access Key' : ''" />
            </el-form-item>
            <el-form-item v-if="form.authType === 'key' && (form.type === 'ssh' || form.type === 'mosh')" :label="t('conn.keyPath')">
              <el-input v-model="form.keyPath" :placeholder="t('conn.keyPathPlaceholder')">
                <template #append>
                  <el-tooltip :content="t('conn.selectKeyFile')" placement="top">
                    <el-button :aria-label="t('conn.selectKeyFile')" @click="selectKeyFile">
                      <el-icon><FolderOpen :size="16" /></el-icon>
                    </el-button>
                  </el-tooltip>
                </template>
              </el-input>
            </el-form-item>
            <el-form-item v-if="form.authType === 'key' && (form.type === 'ssh' || form.type === 'mosh')" :label="t('conn.keyPassphrase')">
              <el-input v-model="form.password" type="password" show-password :key="passwordInputKey" :placeholder="t('conn.keyPassphrasePlaceholder')" />
            </el-form-item>
            <el-form-item v-if="form.type === 'database' && form.dbType !== 'rqlite' && form.dbType !== 'redis'" :label="t('db.databases')">
              <el-input v-model="form.dbName" :placeholder="t('db.databases')" />
            </el-form-item>
            <el-form-item v-if="form.type === 'database'" :label="t('db.params')">
              <el-input v-model="form.dbParams" :placeholder="defaultParamsHint" style="width:100%" />
            </el-form-item>
            <el-form-item v-if="form.type === 'local'" :label="t('conn.shell')">
              <el-select v-model="form.shellPath" filterable>
                <el-option
                  v-for="sh in shellOptions"
                  :key="sh.value"
                  :label="sh.label"
                  :value="sh.value"
                />
              </el-select>
            </el-form-item>
            <template v-if="form.type === 'serial'">
              <el-form-item :label="t('serial.portLabel')" required>
                <div style="display:flex;gap:8px;width:100%">
                  <el-select v-model="form.serialPort" :placeholder="portPlaceholder" :disabled="serialPorts.length === 0 || serialScanning" :loading="serialScanning" style="flex:1">
                    <el-option v-for="p in serialPorts" :key="p" :label="p" :value="p" />
                  </el-select>
                  <el-button :icon="RefreshCw" :loading="serialScanning" @click="scanSerialPorts">
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
              <el-form-item label="Domain" required>
                <el-input v-model="form.smbDomain" placeholder="e.g. WORKGROUP" />
              </el-form-item>
              <el-form-item label="Share">
                <el-input v-model="form.smbShare" placeholder="Share name (leave empty to browse all)" />
              </el-form-item>
            </template>
            <template v-if="form.type === 's3'">
              <el-form-item label="Region" required>
                <el-input v-model="form.s3Region" placeholder="us-east-1" />
              </el-form-item>
              <el-form-item label="Bucket">
                <el-input v-model="form.s3Bucket" placeholder="my-bucket (leave empty to list all buckets)" />
              </el-form-item>
            </template>
            <template v-if="form.type === 'rdp' && isWindows">
              <el-form-item :label="t('rdp.resolution')">
                <el-select v-model="rdpResolution" placeholder="1280×720">
                  <el-option
                    v-for="r in rdpResolutions"
                    :key="r.label"
                    :label="r.label"
                    :value="r.label"
                  />
                </el-select>
              </el-form-item>
              <el-form-item :label="t('conn.rdpSmartSizing')">
                <el-switch v-model="form.rdpSmartSizing" />
              </el-form-item>
            </template>
            <div v-if="showAdvancedToggle" class="advanced-toggle" @click="showAdvanced = !showAdvanced">
              <el-icon class="advanced-arrow" :class="{ expanded: showAdvanced }"><ChevronRight :size="14" /></el-icon>
              <span>{{ t('conn.advanced') }}</span>
            </div>
            <template v-if="showAdvanced">
            <el-form-item v-if="form.type === 'ssh' || form.type === 'telnet' || form.type === 'mosh' || form.type === 'local'" :label="t('conn.postLoginScript')">
              <div class="post-login-config">
                <el-radio-group v-model="postLoginMode" size="small">
                  <el-radio-button label="script">{{ t('conn.postLoginModeScript') }}</el-radio-button>
                  <el-radio-button label="expect" :disabled="form.type !== 'ssh'">{{ t('conn.postLoginModeExpect') }}</el-radio-button>
                </el-radio-group>
                <el-input
                  v-if="postLoginMode === 'script'"
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
                        <Trash2 :size="14" />
                      </el-button>
                    </div>
                  </div>
                  <el-button class="add-step-btn" @click="addExpectStep">
                    <Plus :size="14" />
                    {{ t('conn.expectAddStep') }}
                  </el-button>
                  <div class="expect-help">{{ t('conn.expectVariableHint') }}</div>
                </div>
              </div>
            </el-form-item>
            <el-form-item
              v-if="form.type === 'ssh'"
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
              </el-select>
            </el-form-item>
            <el-form-item v-if="form.type === 'ssh'" :label="t('conn.sftpMaxConcurrency')">
              <el-input-number v-model="form.sftpMaxConcurrency" :min="0" :max="20" />
            </el-form-item>
            <template v-if="form.type === 'ftp'">
              <el-form-item :label="t('conn.ftpEncryption')">
                <el-select v-model="form.ftpEncryption">
                  <el-option :label="t('conn.ftpEncryptionNone')" value="none" />
                  <el-option :label="t('conn.ftpEncryptionAuto')" value="auto" />
                  <el-option :label="t('conn.ftpEncryptionRequired')" value="required" />
                </el-select>
              </el-form-item>
              <el-form-item :label="t('conn.ftpPassive')">
                <el-switch v-model="form.ftpPassive" />
              </el-form-item>
              <el-form-item :label="t('conn.ftpEncoding')">
                <el-select v-model="form.ftpEncoding" placeholder="UTF-8">
                  <el-option label="UTF-8" value="utf-8" />
                  <el-option label="GBK" value="gbk" />
                  <el-option label="Shift-JIS" value="shift-jis" />
                  <el-option label="Latin-1" value="latin-1" />
                </el-select>
              </el-form-item>
            </template>
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
            <el-form-item
              v-if="['ssh','telnet','serial','mosh','local'].includes(form.type)"
              :label="t('conn.logOnConnect')"
            >
              <el-switch v-model="form.logOnConnect" />
              <div class="field-hint">{{ t('conn.logOnConnectDesc') }}</div>
            </el-form-item>
            </template>
          </el-form>
        </div>
      </div>
    </div>
    <template #footer>
      <el-button @click="visible = false">{{ t('conn.cancel') }}</el-button>
      <el-button @click="onSave">{{ t('conn.saveOnly') }}</el-button>
      <el-button type="primary" @click="onConnect">{{ t('conn.saveConnect') }}</el-button>
    </template>
  </el-dialog>

  <!-- New group dialog -->
  <el-dialog append-to-body v-model="showNewGroupDialog" :title="t('conn.newGroupTitle')" width="400px">
    <el-form label-width="80px" @submit.prevent="confirmNewGroup">
      <el-form-item :label="t('conn.groupName')">
        <el-input
          v-model="newGroupName"
          :placeholder="t('conn.groupNamePlaceholder')"
          @keyup.enter="confirmNewGroup"
        />
      </el-form-item>
      <el-form-item :label="t('conn.parentGroup')">
        <el-tree-select
          v-model="newGroupParentId"
          :data="groupTreeData"
          :render-after-expand="false"
          check-strictly
          clearable
          :placeholder="t('conn.noGroup')"
          style="width:100%"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showNewGroupDialog = false">{{ t('conn.cancel') }}</el-button>
      <el-button type="primary" @click="confirmNewGroup">{{ t('conn.save') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, computed, watch, ref } from 'vue'
import { useConnectionStore } from '../stores/connectionStore'
import { useSettingsStore } from '../stores/settingsStore'
import { useI18n } from '../i18n'
import type { ConnectionConfig, PostLoginExpectStep } from '../types/session'
import { OpenFileDialog } from '../../wailsjs/go/main/App'
import { Plus, Trash2, ChevronDown, ChevronRight, FolderOpen, RefreshCw, Terminal, Monitor, Database, DatabaseZap, Layers, SquareTerminal, Zap, Laptop, Cable, FolderUp, HardDrive, Cloud, Globe, MonitorCloud, MonitorSmartphone } from '@lucide/vue'
import { ListSerialPorts } from '../../wailsjs/go/main/App'

const { t } = useI18n()
const connectionStore = useConnectionStore()
const settingsStore = useSettingsStore()

// ── Categories & sub-types ──
interface SubTypeInfo {
  type: string
  dbType?: string
  label: string
  icon: any
}

const categories = computed(() => [
  { key: 'terminal', label: t('conn.categoryTerminal'), icon: SquareTerminal },
  { key: 'filetransfer', label: t('conn.categoryFileTransfer'), icon: FolderUp },
  { key: 'remote', label: t('conn.categoryRemote'), icon: Monitor },
  { key: 'database', label: t('db.database'), icon: Database },
])

const allSubTypes = computed(() => ({
  terminal: [
    { type: 'ssh', label: 'SSH (SFTP)', icon: SquareTerminal },
    { type: 'telnet', label: 'Telnet', icon: Terminal },
    { type: 'mosh', label: 'Mosh', icon: Zap },
    { type: 'local', label: t('conn.localTerminal'), icon: Laptop },
    { type: 'serial', label: t('serial.title'), icon: Cable },
  ],
  filetransfer: [
    { type: 'ftp', label: 'FTP', icon: FolderUp },
    { type: 'smb', label: 'SMB', icon: HardDrive },
    { type: 's3', label: 'S3', icon: Cloud },
    { type: 'webdav', label: 'WebDAV', icon: Globe },
  ],
  remote: [
    ...(isWindows.value ? [{ type: 'rdp', label: 'RDP', icon: Monitor }] : []),
    { type: 'vnc', label: 'VNC', icon: MonitorSmartphone },
    { type: 'spice', label: 'SPICE', icon: MonitorCloud },
  ],
  database: [
    { type: 'database', dbType: 'mysql', label: 'MySQL', icon: Database },
    { type: 'database', dbType: 'postgres', label: 'PostgreSQL', icon: Database },
    { type: 'database', dbType: 'oracle', label: 'Oracle', icon: Database },
    { type: 'database', dbType: 'sqlserver', label: 'SQL Server', icon: Database },
    { type: 'database', dbType: 'rqlite', label: 'rqlite', icon: Database },
    { type: 'database', dbType: 'redis', label: 'Redis', icon: DatabaseZap },
    { type: 'database', dbType: 'mongodb', label: 'MongoDB', icon: Layers },
  ],
}))

const currentSubTypes = computed(() => allSubTypes.value[category.value] || allSubTypes.value.terminal)

function isSubTypeActive(st: SubTypeInfo): boolean {
  if (st.dbType) {
    return form.type === 'database' && form.dbType === st.dbType
  }
  return form.type === st.type
}

function selectType(st: SubTypeInfo) {
  if (st.dbType) {
    form.type = 'database'
    form.dbType = st.dbType
  } else {
    form.type = st.type
  }
}

function onCategorySelect(catKey: string) {
  if (category.value === catKey) return
  const subs = allSubTypes.value[catKey]
  if (subs && subs.length > 0) {
    selectType(subs[0])
  }
}

function getShellLabel(path: string): string {
  if (!path) return ''
  const lower = path.toLowerCase()
  if (lower.startsWith('wsl://')) {
    const distro = path.slice(6)
    return distro ? `WSL - ${distro}` : 'WSL'
  }
  if (lower.includes('pwsh')) return 'PowerShell'
  if (lower.includes('powershell')) return 'Windows PowerShell'
  if (lower.includes('bash')) return 'Git Bash'
  if (lower.includes('cmd')) return 'Command Prompt'
  return path.split(/[\\/]/).pop() || path
}

const shellOptions = computed(() =>
  settingsStore.availableShells.map(sh => ({ label: getShellLabel(sh), value: sh }))
)

const isWindows = ref(/windows/i.test(navigator.userAgent))
const passwordInputKey = ref(0)
const postLoginMode = ref<'script' | 'expect'>('script')
const showAdvanced = ref(false)

// Serial port config (separate refs so allow-create doesn't produce strings)
const serialPorts = ref<string[]>([])
const serialScanning = ref(false)
const serialBaudRateInput = ref('')
const serialDataBitsValue = ref(8)
const serialStopBitsValue = ref(1)
const serialParityValue = ref('none')

const portPlaceholder = computed(() => {
  if (serialScanning.value) return t('serial.scanning')
  if (serialPorts.value.length === 0) return t('serial.noPorts')
  return t('serial.portLabel')
})

async function scanSerialPorts() {
  serialScanning.value = true
  try {
    serialPorts.value = await ListSerialPorts()
  } catch {
    serialPorts.value = []
  } finally {
    serialScanning.value = false
  }
}

const baudRatePresets = [300, 1200, 2400, 4800, 9600, 14400, 19200, 38400, 57600, 115200, 230400, 460800, 921600]

function queryBaudRateSuggestions(queryString: string, cb: (results: { value: string }[]) => void) {
  const suggestions = baudRatePresets
    .filter(r => String(r).includes(queryString))
    .map(r => ({ value: String(r) }))
  cb(suggestions)
}

const props = defineProps<{
  modelValue: boolean
  editConfig?: ConnectionConfig
  defaultGroupId?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  save: [config: ConnectionConfig]
  connect: [config: ConnectionConfig]
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

watch(visible, (val) => {
  if (val) {
    passwordInputKey.value++
  }
})

const isEdit = computed(() => !!props.editConfig?.id)

const TERMINAL_TYPES = ['ssh', 'telnet', 'mosh', 'local', 'serial']
const REMOTE_TYPES = ['rdp', 'vnc', 'spice']
const FILETRANSFER_TYPES = ['ftp', 'ssh', 'smb', 'webdav', 's3']

const category = computed(() => {
  if (TERMINAL_TYPES.includes(form.type)) return 'terminal'
  if (FILETRANSFER_TYPES.includes(form.type)) return 'filetransfer'
  if (REMOTE_TYPES.includes(form.type)) return 'remote'
  if (form.type === 'database') return 'database'
  return 'terminal'
})

const sshConnections = computed(() =>
  connectionStore.connections
    .filter(c => c.type === 'ssh' && c.id !== form.id)
    .sort((a, b) => a.name.localeCompare(b.name))
)

const TUNNEL_UNSUPPORTED = ['spice', 'mosh', 'local', 'serial']
const showTunnel = computed(() =>
  !TUNNEL_UNSUPPORTED.includes(form.type)
)
const showAdvancedToggle = computed(() =>
  showTunnel.value || form.type === 'ssh' || form.type === 'telnet' || form.type === 'mosh' || form.type === 'local' || form.type === 'serial' || form.type === 'ftp'
)

const defaultParamsHint = computed(() => {
  switch (form.dbType) {
    case 'mysql': return '默认: charset=utf8mb4'
    case 'postgres': return '默认: sslmode=disable'
    case 'sqlserver': return '默认: encrypt=disable'
    default: return ''
  }
})

const form = reactive<ConnectionConfig>({
  id: '',
  name: '',
  type: 'ssh',
  host: '',
  port: 22,
  user: '',
  authType: 'password',
  password: '',
  keyPath: '',
  groupId: undefined,
  rdpFixedWidth: undefined,
  rdpFixedHeight: undefined,
  rdpSmartSizing: true,
  rdpEnableNLA: true,
  dbType: '',
  dbName: '',
  dbParams: '',
  postLoginScript: '',
  postLoginExpectSteps: [],
  sftpMaxConcurrency: 5,
  ftpEncryption: 'none',
  ftpPassive: true,
  ftpEncoding: 'utf-8',
  encoding: 'utf-8',
  shellPath: '',
  smbDomain: 'WORKGROUP',
  smbShare: '',
  s3Region: 'us-east-1',
  s3Bucket: '',
  logOnConnect: false,
})

const rdpResolutions = [
  { label: '800 × 600 (SVGA)', w: 800, h: 600 },
  { label: '1024 × 768 (XGA)', w: 1024, h: 768 },
  { label: '1280 × 720 (HD)', w: 1280, h: 720 },
  { label: '1680 × 1050 (WSXGA+)', w: 1680, h: 1050 },
  { label: '1600 × 1200 (UXGA)', w: 1600, h: 1200 },
  { label: '1920 × 1080 (Full HD)', w: 1920, h: 1080 },
  { label: '2560 × 1440 (QHD)', w: 2560, h: 1440 },
]

const rdpResolution = ref('1280 × 720 (HD)')

const selectedGroupId = ref<string | undefined>(undefined)

// Tree data for el-tree-select
interface TreeOption {
  value: string
  label: string
  children?: TreeOption[]
}

const groupTreeData = computed<TreeOption[]>(() => {
  function buildTree(nodes: any[]): TreeOption[] {
    return nodes.map(node => ({
      value: node.group.id,
      label: node.group.name,
      children: node.children.length > 0 ? buildTree(node.children) : undefined,
    }))
  }
  return [
    { value: '__none__', label: t('conn.noGroup') },
    ...buildTree(connectionStore.groupedConnections.roots),
  ]
})

const selectedGroupName = computed(() => {
  if (!form.groupId) return ''
  const g = connectionStore.groups.find(g => g.id === form.groupId)
  return g?.name || form.groupId
})

// New group dialog
const showNewGroupDialog = ref(false)
const newGroupName = ref('')
const newGroupParentId = ref<string | undefined>(undefined)

watch(() => props.editConfig, (config) => {
  if (config) {
    // If editing an existing connection (has id), merge its full config.
    // Otherwise (sparse config from quick-new), reset first to avoid stale
    // data from a previously edited connection leaking in.
    if (!config.id) {
      resetForm()
    }
    Object.assign(form, { ...config, postLoginExpectSteps: cloneExpectSteps(config.postLoginExpectSteps || []) })
    // Existing connections without the field default to NLA off (old behavior).
    form.rdpEnableNLA = config.rdpEnableNLA ?? false
    postLoginMode.value = (config.postLoginExpectSteps?.length || 0) > 0 ? 'expect' : 'script'
    selectedGroupId.value = config.groupId || undefined
    // Sync serial refs from config
    if (config.serialBaudRate) serialBaudRateInput.value = String(config.serialBaudRate)
    if (config.serialDataBits) serialDataBitsValue.value = config.serialDataBits
    if (config.serialStopBits) serialStopBitsValue.value = config.serialStopBits
    if (config.serialParity) serialParityValue.value = config.serialParity
    if (config.type === 'serial') scanSerialPorts()
    // Sync resolution dropdown to the config's fixed size
    const match = rdpResolutions.find(r => r.w === config.rdpFixedWidth && r.h === config.rdpFixedHeight)
    if (match) rdpResolution.value = match.label
  } else {
    resetForm()
    if (props.defaultGroupId) {
      selectedGroupId.value = props.defaultGroupId
      form.groupId = props.defaultGroupId
    }
  }
}, { immediate: true })

watch(() => props.defaultGroupId, (gid) => {
  if (!props.editConfig && gid) {
    selectedGroupId.value = gid
    form.groupId = gid
  }
})

// Auto-switch default port when changing type
watch(() => form.type, (newType) => {
  if (newType !== 'ssh' && postLoginMode.value === 'expect') {
    postLoginMode.value = 'script'
  }
  if (isEdit.value) return
  if (newType === 'ssh') form.port = 22
  else if (newType === 'telnet') form.port = 23
  else if (newType === 'mosh') form.port = 22
  else if (newType === 'rdp') form.port = 3389
  else if (newType === 'vnc') form.port = 5900
  else if (newType === 'spice') form.port = 5900
  else if (newType === 'database') form.port = 3306
  else if (newType === 'ftp') form.port = 21
  else if (newType === 'smb') form.port = 445
  if (REMOTE_TYPES.includes(newType) || newType === 'database') {
    form.authType = 'password'
  }
  if (newType === 'local' && !form.shellPath && settingsStore.availableShells.length > 0) {
    form.shellPath = settingsStore.availableShells[0]
  }
  if (newType === 'serial') {
    scanSerialPorts()
  }
})

watch(postLoginMode, (mode) => {
  if (mode === 'expect' && (!form.postLoginExpectSteps || form.postLoginExpectSteps.length === 0)) {
    addExpectStep()
  }
})

// Auto-switch default port when changing database type
watch(() => form.dbType, (newType) => {
  if (isEdit.value) return
  if (newType === 'mysql') form.port = 3306
  else if (newType === 'postgres') form.port = 5432
  else if (newType === 'rqlite') form.port = 4001
  else if (newType === 'oracle') form.port = 1521
  else if (newType === 'sqlserver') form.port = 1433
  else if (newType === 'redis') form.port = 6379
  else if (newType === 'mongodb') form.port = 27017
})

// Sync resolution picker to form fields
watch(rdpResolution, (val) => {
  const found = rdpResolutions.find(r => r.label === val)
  if (found) {
    form.rdpFixedWidth = found.w
    form.rdpFixedHeight = found.h
  }
})

function resetForm() {
  form.id = ''
  form.name = ''
  form.type = 'ssh'
  form.host = ''
  form.port = 22
  form.user = ''
  form.authType = 'password'
  form.password = ''
  form.keyPath = ''
  form.groupId = undefined
  form.rdpFixedWidth = undefined
  form.rdpFixedHeight = undefined
  form.rdpSmartSizing = true
  form.rdpEnableNLA = true
  form.dbType = ''
  form.dbName = ''
  form.dbParams = ''
  form.postLoginScript = ''
  form.postLoginExpectSteps = []
  postLoginMode.value = 'script'
  form.sftpMaxConcurrency = 5
  form.ftpEncryption = 'none'
  form.ftpPassive = true
  form.ftpEncoding = 'utf-8'
  form.encoding = 'utf-8'
  form.shellPath = ''
  form.serialPort = ''
  form.serialBaudRate = 115200
  form.serialDataBits = 8
  form.serialStopBits = 1
  form.serialParity = 'none'
  serialBaudRateInput.value = ''
  form.tunnelSSHConnId = undefined
  form.logOnConnect = false
  rdpResolution.value = '1280 × 720 (HD)'
  selectedGroupId.value = undefined
}

// Sync tree-select value to form
watch(selectedGroupId, (val) => {
  form.groupId = val === '__none__' ? undefined : (val || undefined)
})

function onNodeClick(data: any) {
  // el-tree-select auto-closes and syncs via v-model
}

function onGroupSelect(value: string | undefined) {
  if (value === '__new__') {
    showNewGroupDialog.value = true
    newGroupName.value = ''
    newGroupParentId.value = undefined
    return
  }
}

async function confirmNewGroup() {
  const name = newGroupName.value.trim()
  if (!name) {
    return
  }
  showNewGroupDialog.value = false
  const group = await connectionStore.addGroup(name, newGroupParentId.value)
  newGroupParentId.value = undefined
  newGroupName.value = ''
  form.groupId = group.id
  selectedGroupId.value = group.id
}

async function selectKeyFile() {
  try {
    const selected = await OpenFileDialog()
    if (selected) form.keyPath = selected
  } catch (e) {
    console.error('select key file:', e)
  }
}

function generateUniqueName(name: string): string {
  if (!connectionStore.connections.some(c => c.name === name)) {
    return name
  }
  let idx = 1
  while (connectionStore.connections.some(c => c.name === `${name} (${idx})`)) {
    idx++
  }
  return `${name} (${idx})`
}

function normalizeForm(): ConnectionConfig {
  // Sync serial refs into form before normalization
  if (form.type === 'serial') {
    form.serialBaudRate = parseInt(serialBaudRateInput.value, 10) || 115200
    serialBaudRateInput.value = String(form.serialBaudRate)
    form.serialDataBits = serialDataBitsValue.value
    form.serialStopBits = serialStopBitsValue.value
    form.serialParity = serialParityValue.value
  }
  const normalized = { ...form }
  normalized.postLoginExpectSteps = normalizeExpectSteps(form.postLoginExpectSteps || [])
  if (postLoginMode.value === 'script') {
    normalized.postLoginExpectSteps = []
  } else {
    normalized.postLoginScript = ''
  }
  if (normalized.type !== 'local' && normalized.type !== 'serial' && !normalized.host.trim()) {
    throw new Error(t('conn.hostRequired'))
  }
  if (normalized.type === 's3') {
    if (!normalized.user?.trim()) throw new Error('S3: Access Key is required')
    if (!normalized.password?.trim()) throw new Error('S3: Secret Key is required')
  }
  if (!normalized.name.trim()) {
    normalized.name = generateUniqueName(
      normalized.type === 'serial' ? (normalized.serialPort || 'Serial') : normalized.host.trim()
    )
  }
  return normalized
}

function cloneExpectSteps(steps: PostLoginExpectStep[]): PostLoginExpectStep[] {
  return steps.map(step => ({ ...step }))
}

function normalizeExpectSteps(steps: PostLoginExpectStep[]): PostLoginExpectStep[] {
  return steps
    .map(step => ({
      expect: step.expect.trim(),
      send: step.send,
      enter: step.enter !== false,
      timeoutSecond: step.timeoutSecond || 10
    }))
    .filter(step => step.expect || step.send)
}

function addExpectStep() {
  if (!form.postLoginExpectSteps) {
    form.postLoginExpectSteps = []
  }
  form.postLoginExpectSteps.push({
    expect: '',
    send: '',
    enter: true,
    timeoutSecond: 10
  })
}

function removeExpectStep(index: number) {
  form.postLoginExpectSteps?.splice(index, 1)
}

function onSave() {
  try {
    const config = normalizeForm()
    emit('save', config)
    visible.value = false
    if (!props.editConfig) {
      resetForm()
    }
  } catch (e: any) {
    // Host empty, silently return
  }
}

function onConnect() {
  try {
    const config = normalizeForm()
    emit('connect', config)
    visible.value = false
    if (!props.editConfig) {
      resetForm()
    }
  } catch (e: any) {
    // Host empty
  }
}
</script>

<style scoped>
/* ── Layout ── */
.conn-layout {
  display: flex;
  gap: 0;
  min-height: 360px;
}

/* ── Left sidebar ── */
.conn-categories {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 90px;
  flex-shrink: 0;
  padding: 8px 8px 8px 0;
  border-right: 1px solid var(--border-subtle);
}

.cat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 12px 4px;
  border-radius: var(--radius-md);
  cursor: pointer;
  user-select: none;
  color: var(--text-muted);
  border-left: 2px solid transparent;
  transition: all 0.15s ease;
}

.cat-item:hover {
  background: var(--bg-hover);
  color: var(--text-secondary);
}

.cat-item.active {
  color: var(--accent);
  background: var(--accent-subtle);
  border-left-color: var(--accent);
}

.cat-item span {
  font-size: 11px;
  font-weight: 500;
  font-family: var(--font-ui);
  text-align: center;
  line-height: 1.2;
}

/* ── Right main content ── */
.conn-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  padding: 0 0 0 16px;
}

/* ── Sub-type icon grid ── */
.subtype-grid {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 4px;
  padding-bottom: 14px;
  margin-bottom: 12px;
  border-bottom: 1px solid var(--border-subtle);
}

.subtype-btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  width: 64px;
  height: 52px;
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

.subtype-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
  border-color: var(--border-default);
}

.subtype-btn.active {
  background: linear-gradient(135deg, var(--accent), var(--accent));
  color: var(--on-accent);
  border-color: var(--accent-glow);
  box-shadow: 0 0 0 1px var(--accent-glow), 0 2px 8px var(--accent-glow);
}

.subtype-btn span {
  text-align: center;
  line-height: 1.2;
  font-size: 11px;
  white-space: nowrap;
}

/* ── Form fields ── */
.conn-fields {
  padding-right: 4px;
}

/* ── Host + port row ── */
.host-port-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
}

.host-input {
  width: calc(100% - 150px) !important;
}

.host-port-sep {
  color: var(--text-muted);
  font-weight: 500;
}

.port-input {
  width: 130px !important;
  flex-shrink: 0;
}

/* ── NLA toggle row ── */
.nla-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

/* ── Field hint text ── */
.field-hint {
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.4;
}

/* ── Advanced toggle ── */
.advanced-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 0 8px;
  margin-bottom: 4px;
  cursor: pointer;
  user-select: none;
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 500;
  border-bottom: 1px solid var(--border-subtle);
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

/* ── Post-login config ── */
.post-login-config {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.expect-steps {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.expect-table {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  overflow: hidden;
}

.expect-row {
  display: grid;
  grid-template-columns: 26px minmax(80px, 1fr) minmax(90px, 1fr) 64px 40px 30px;
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
  font-size: 12px;
  line-height: 1.2;
  color: var(--text-secondary);
}

.expect-head > span {
  padding: 3px 4px;
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
  font-size: 12px;
}

.remove-step-btn {
  min-width: 0;
}

.add-step-btn {
  align-self: flex-start;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.expect-help {
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.4;
}

/* ── Group selector row ── */
.group-select-row {
  display: flex;
  gap: 6px;
  align-items: center;
}
.add-group-btn {
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  padding: 0;
}

/* ── Dialog overrides ── */
:deep(.el-dialog__body) {
  padding: 16px 20px;
}
</style>
