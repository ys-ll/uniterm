<template>
  <el-dialog v-model="visible" :title="isEdit ? t('conn.editTitle') : t('conn.newTitle')" width="550px">
    <el-form id="conn-form" :model="form" label-width="100px" @submit.prevent="onSave">
      <el-form-item :label="t('conn.name')">
        <el-input v-model="form.name" :placeholder="t('conn.namePlaceholder')" />
      </el-form-item>
      <el-form-item :label="t('conn.group')">
        <el-select v-model="selectedGroupId" :placeholder="t('conn.noGroup')" clearable @change="onGroupSelect">
          <el-option
            v-for="g in connectionStore.groups"
            :key="g.id"
            :label="g.name"
            :value="g.id"
          />
          <el-option
            :label="t('conn.noGroup')"
            value="__none__"
          />
          <el-option
            :label="t('conn.newGroup')"
            value="__new__"
          />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('conn.type')">
        <el-radio-group :model-value="category" @change="onCategoryChange">
          <el-radio-button value="terminal">{{ t('conn.categoryTerminal') }}</el-radio-button>
          <el-radio-button value="filetransfer">{{ t('conn.categoryFileTransfer') }}</el-radio-button>
          <el-radio-button value="remote">{{ t('conn.categoryRemote') }}</el-radio-button>
          <el-radio-button value="database">{{ t('db.database') }}</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item v-if="category" label="">
        <template v-if="category === 'terminal'">
          <el-radio-group v-model="form.type">
            <el-radio-button label="ssh">SSH (SFTP)</el-radio-button>
            <el-radio-button label="telnet">Telnet</el-radio-button>
            <el-radio-button label="mosh">Mosh</el-radio-button>
          </el-radio-group>
        </template>
        <template v-if="category === 'filetransfer'">
          <el-radio-group v-model="form.type">
            <el-radio-button label="ftp">FTP</el-radio-button>
            <el-radio-button label="ssh">SSH (SFTP)</el-radio-button>
          </el-radio-group>
        </template>
        <template v-if="category === 'remote'">
          <el-radio-group v-model="form.type">
            <el-radio-button label="rdp" v-if="isWindows">RDP</el-radio-button>
            <el-radio-button label="vnc">VNC</el-radio-button>
            <el-radio-button label="spice">SPICE</el-radio-button>
          </el-radio-group>
        </template>
        <template v-if="category === 'database'">
          <el-radio-group v-model="form.dbType">
            <el-radio-button label="mysql">MySQL</el-radio-button>
            <el-radio-button label="postgres">PostgreSQL</el-radio-button>
            <el-radio-button label="oracle">Oracle</el-radio-button>
            <el-radio-button label="sqlserver">SQL Server</el-radio-button>
            <el-radio-button label="rqlite">rqlite</el-radio-button>
          </el-radio-group>
        </template>
      </el-form-item>
      <el-form-item :label="t('conn.host')" required>
        <el-input v-model="form.host" :placeholder="t('conn.hostPlaceholder')" />
      </el-form-item>
      <el-form-item :label="t('conn.port')">
        <el-input-number v-model="form.port" :min="0" :max="65535" />
      </el-form-item>
      <el-form-item v-if="form.type !== 'vnc' && form.type !== 'spice' && !(form.type === 'database' && form.dbType === 'rqlite')" :label="t('conn.user')">
        <el-input v-model="form.user" :placeholder="t('conn.userPlaceholder')" />
      </el-form-item>
      <el-form-item v-if="form.type === 'ssh' || form.type === 'mosh'" :label="t('conn.authType')">
        <el-radio-group v-model="form.authType">
          <el-radio-button label="password">{{ t('conn.password') }}</el-radio-button>
          <el-radio-button label="key">{{ t('conn.keyPath') }}</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item v-if="(form.authType === 'password' || form.type === 'rdp' || form.type === 'vnc' || form.type === 'spice' || form.type === 'database' || form.type === 'mosh' || form.type === 'telnet' || form.type === 'ftp') && !(form.type === 'database' && form.dbType === 'rqlite')" :label="t('conn.password')">
        <el-input v-model="form.password" type="password" show-password :key="passwordInputKey" />
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
      <el-form-item v-if="form.type === 'database' && form.dbType !== 'rqlite'" :label="t('db.databases')">
        <el-input v-model="form.dbName" :placeholder="t('db.databases')" />
      </el-form-item>
      <div class="advanced-toggle" @click="showAdvanced = !showAdvanced">
        <el-icon class="advanced-arrow" :class="{ expanded: showAdvanced }"><ChevronRight :size="14" /></el-icon>
        <span>{{ t('conn.advanced') }}</span>
      </div>
      <template v-if="showAdvanced">
      <template v-if="form.type === 'rdp'">
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
      <el-form-item v-if="form.type === 'ssh' || form.type === 'telnet' || form.type === 'mosh'" :label="t('conn.postLoginScript')">
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
      </template>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">{{ t('conn.cancel') }}</el-button>
      <el-button @click="onSave">{{ t('conn.saveOnly') }}</el-button>
      <el-button type="primary" @click="onConnect">{{ t('conn.saveConnect') }}</el-button>
    </template>
  </el-dialog>

  <!-- New group dialog -->
  <el-dialog v-model="showNewGroupDialog" :title="t('conn.newGroupTitle')" width="360px">
    <el-form @submit.prevent="confirmNewGroup">
      <el-form-item :label="t('conn.groupName')">
        <el-input
          v-model="newGroupName"
          :placeholder="t('conn.groupNamePlaceholder')"
          @keyup.enter="confirmNewGroup"
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
import { reactive, computed, watch, ref, onMounted } from 'vue'
import { useConnectionStore } from '../stores/connectionStore'
import { useI18n } from '../i18n'
import type { ConnectionConfig, PostLoginExpectStep } from '../types/session'
import { GetPlatform, OpenFileDialog } from '../../wailsjs/go/main/App'
import { Plus, Trash2, ChevronRight, FolderOpen } from '@lucide/vue'

const { t } = useI18n()
const connectionStore = useConnectionStore()

const isWindows = ref(true)
const passwordInputKey = ref(0)
const postLoginMode = ref<'script' | 'expect'>('script')
const showAdvanced = ref(false)

onMounted(async () => {
  try { isWindows.value = (await GetPlatform()) === 'windows' } catch (_) {}
})

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

const TERMINAL_TYPES = ['ssh', 'telnet', 'mosh']
const REMOTE_TYPES = ['rdp', 'vnc', 'spice']
const FILETRANSFER_TYPES = ['ftp', 'ssh']

const category = computed(() => {
  if (TERMINAL_TYPES.includes(form.type)) return 'terminal'
  if (FILETRANSFER_TYPES.includes(form.type)) return 'filetransfer'
  if (REMOTE_TYPES.includes(form.type)) return 'remote'
  if (form.type === 'database') return 'database'
  return 'terminal'
})

const sshConnections = computed(() =>
  connectionStore.connections.filter(c =>
    c.type === 'ssh' && c.id !== form.id
  )
)

const TUNNEL_UNSUPPORTED = ['spice', 'mosh', 'local']
const showTunnel = computed(() =>
  !TUNNEL_UNSUPPORTED.includes(form.type)
)

function onCategoryChange(cat: string) {
  if (cat === 'terminal') {
    form.type = 'ssh'
    if (!isEdit.value) form.port = 22
  } else if (cat === 'filetransfer') {
    form.type = 'ftp'
    if (!isEdit.value) form.port = 21
  } else if (cat === 'remote') {
    form.type = isWindows.value ? 'rdp' : 'vnc'
    if (!isEdit.value) form.port = isWindows.value ? 3389 : 5900
  } else if (cat === 'database') {
    form.type = 'database'
    form.dbType = form.dbType || 'mysql'
    if (!isEdit.value) form.port = 3306
  }
}

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
  dbType: '',
  dbName: '',
  postLoginScript: '',
  postLoginExpectSteps: [],
  sftpMaxConcurrency: 5,
  ftpEncryption: 'none',
  ftpPassive: true,
  ftpEncoding: 'utf-8',
  encoding: 'utf-8',
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

// New group dialog
const showNewGroupDialog = ref(false)
const newGroupName = ref('')

watch(() => props.editConfig, (config) => {
  if (config) {
    // If editing an existing connection (has id), merge its full config.
    // Otherwise (sparse config from quick-new), reset first to avoid stale
    // data from a previously edited connection leaking in.
    if (!config.id) {
      resetForm()
    }
    Object.assign(form, { ...config, postLoginExpectSteps: cloneExpectSteps(config.postLoginExpectSteps || []) })
    postLoginMode.value = (config.postLoginExpectSteps?.length || 0) > 0 ? 'expect' : 'script'
    selectedGroupId.value = config.groupId || undefined
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
  if (REMOTE_TYPES.includes(newType) || newType === 'database') {
    form.authType = 'password'
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
  form.dbType = ''
  form.dbName = ''
  form.postLoginScript = ''
  form.postLoginExpectSteps = []
  postLoginMode.value = 'script'
  form.sftpMaxConcurrency = 5
  form.ftpEncryption = 'none'
  form.ftpPassive = true
  form.ftpEncoding = 'utf-8'
  form.encoding = 'utf-8'
  form.tunnelSSHConnId = undefined
  rdpResolution.value = '1280 × 720 (HD)'
  selectedGroupId.value = undefined
}

function onGroupSelect(value: string | undefined) {
  if (value === '__new__') {
    showNewGroupDialog.value = true
    newGroupName.value = ''
    selectedGroupId.value = form.groupId || undefined
    return
  }
  if (value === '__none__') {
    form.groupId = undefined
    selectedGroupId.value = undefined
    return
  }
  form.groupId = value
  selectedGroupId.value = value
}

async function confirmNewGroup() {
  const name = newGroupName.value.trim()
  if (!name) {
    return
  }
  if (connectionStore.groups.some(g => g.name === name)) {
    return
  }
  const group = await connectionStore.addGroup(name)
  form.groupId = group.id
  selectedGroupId.value = group.id
  showNewGroupDialog.value = false
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
  const normalized = { ...form }
  normalized.postLoginExpectSteps = normalizeExpectSteps(form.postLoginExpectSteps || [])
  if (postLoginMode.value === 'script') {
    normalized.postLoginExpectSteps = []
  } else {
    normalized.postLoginScript = ''
  }
  if (!normalized.host.trim()) {
    throw new Error(t('conn.hostRequired'))
  }
  if (!normalized.name.trim()) {
    normalized.name = generateUniqueName(normalized.host.trim())
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
.advanced-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 0 8px;
  margin-bottom: 4px;
  cursor: pointer;
  user-select: none;
  color: var(--text-secondary, #909399);
  font-size: 13px;
  font-weight: 500;
  border-bottom: 1px solid var(--border-subtle, #e4e7ed);
  transition: color 0.15s;
}

.advanced-toggle:hover {
  color: var(--accent, #409eff);
}

.advanced-arrow {
  transition: transform 0.2s;
  display: inline-flex;
  align-items: center;
}

.advanced-arrow.expanded {
  transform: rotate(90deg);
}

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
</style>
