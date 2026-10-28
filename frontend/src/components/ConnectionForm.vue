<template>
  <el-dialog append-to-body v-model="visible" :title="isEdit ? t('conn.editTitle') : t('conn.newTitle')" width="45rem" class="conn-dialog" @opened="onDialogOpened" @closed="onDialogClosed">
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
          <component :is="cat.icon" :size="'1.25rem'" />
          <span>{{ cat.label }}</span>
        </div>
      </div>

      <!-- Right content: sub-type grid + form -->
      <div class="conn-main">
        <!-- Sub-type icon grid -->
        <div class="subtype-grid">
          <button
            v-for="st in currentSubTypes"
            :key="st.type + (st.dbType || '') + (st.containerRuntime || '')"
            class="subtype-btn"
            :class="{ active: isSubTypeActive(st) }"
            @click="selectType(st)"
          >
            <component :is="st.icon" :size="'1.125rem'" />
            <span>{{ st.label }}</span>
          </button>
        </div>

        <!-- Form fields (split into field-group subcomponents) -->
        <div class="conn-fields">
          <el-form :model="form" label-width="6.5rem" @submit.prevent="onSave">
            <BasicFields
              ref="basicFieldsRef"
              :form="form"
              v-model:selectedGroupId="selectedGroupId"
              :group-tree-data="groupTreeData"
              :is-windows="isWindows"
              :shell-options-for-type="shellOptionsForType"
              :serial-ports="serialPorts"
              :serial-scanning="serialScanning"
              v-model:serialBaudRateInput="serialBaudRateInput"
              v-model:serialDataBitsValue="serialDataBitsValue"
              v-model:serialStopBitsValue="serialStopBitsValue"
              v-model:serialParityValue="serialParityValue"
              :port-placeholder="portPlaceholder"
              :ssh-connections="sshConnections"
              v-model:k8sSourceMode="k8sSourceMode"
              :k8s-contexts="k8sContexts"
              :k8s-contexts-loading="k8sContextsLoading"
              :k8s-contexts-error="k8sContextsError"
              :k8s-namespace-options="k8sNamespaceOptions"
              v-model:k8sConfigRevealed="k8sConfigRevealed"
              v-model:rdpResolution="rdpResolution"
              :rdp-resolution-options="rdpResolutionOptions"
              v-model:rdpCustomWidth="rdpCustomWidth"
              v-model:rdpCustomHeight="rdpCustomHeight"
              :rdp-custom-resolution-const="RDP_CUSTOM_RESOLUTION"
              @group-select="onGroupSelect"
              @select-sqlite-file="selectSqliteFile"
              @scan-serial-ports="scanSerialPorts"
              @pick-kubeconfig-file="pickKubeconfigFile"
              @import-kubeconfig-text="importKubeconfigText"
              @reload-k8s-contexts="reloadK8sContexts"
              @k8s-namespace-input="onK8sNamespaceInput"
            />
            <AuthFields
              :form="form"
              :is-windows="isWindows"
              :is-mac="isMac"
              :password-input-key="passwordInputKey"
              v-model:keyContentRevealed="keyContentRevealed"
              @open-new-identity-dialog="openNewIdentityDialog"
              @select-key-file="selectKeyFile"
              @use-default-key-path="useDefaultKeyPath"
              @import-key-text="importKeyText"
            />
            <AdvancedFields
              :form="form"
              :is-windows="isWindows"
              :platform="platform"
              v-model:showAdvanced="showAdvanced"
              v-model:postLoginMode="postLoginMode"
              v-model:sshAlgoMode="sshAlgoMode"
              :ssh-connections="sshConnections"
              @open-ssh-algo-dialog="sshAlgoDialogVisible = true"
              @open-new-proxy-dialog="openNewProxyDialog"
            />
          </el-form>
        </div>
      </div>
    </div>
    <template #footer>
      <el-button @click="visible = false">{{ t('conn.cancel') }}</el-button>
      <el-button
        v-if="canTest"
        :loading="testStatus === 'checking'"
        :icon="testStatus === 'success' ? CircleCheck : testStatus === 'error' ? CircleX : undefined"
        :class="{ 'test-success': testStatus === 'success', 'test-error': testStatus === 'error' }"
        class="test-status-btn"
        @click="onTest"
      >
        {{ t('conn.testConnection') }}
      </el-button>
      <el-button v-if="!isEdit" @click="onConnectOnly">{{ t('conn.connectOnly') }}</el-button>
      <el-button @click="onSave">{{ t('conn.saveOnly') }}</el-button>
      <el-button type="primary" @click="onConnect">{{ t('conn.saveConnect') }}</el-button>
    </template>
  </el-dialog>

  <!-- New group dialog -->
  <el-dialog append-to-body v-model="showNewGroupDialog" :title="t('conn.newGroupTitle')" width="25rem">
    <el-form label-width="5rem" @submit.prevent="confirmNewGroup">
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

  <!-- New identity / keychain dialog (reuses the settings one) -->
  <IdentityEditDialog v-model:visible="identityDialogVisible" :identity="null" @saved="onIdentitySaved" />
  <!-- New proxy dialog (reuses the settings one) -->
  <ProxyEditDialog v-model:visible="proxyDialogVisible" :proxy="null" @saved="onProxySaved" />
  <!-- SSH algorithm editor for custom mode -->
  <SSHAlgorithmsDialog v-model:visible="sshAlgoDialogVisible" :config="form.sshAlgorithms" @save="onSSHAlgoSaved" />
</template>

<script setup lang="ts">
import { reactive, computed, watch, ref, nextTick, onMounted } from 'vue'
import { useConnectionStore } from '../stores/connectionStore'

import { useIdentityStore } from '../stores/identityStore'
import { useProxyStore } from '../stores/proxyStore'
import { useI18n } from '../i18n'
import type { ConnectionConfig, SSHAlgoConfig } from '../types/session'
import { GetPlatform, ListSerialPorts } from '../../bindings/github.com/ys-ll/uniterm/app'
import { msg } from '../services/message'
import { CircleCheck, CircleX } from '@lucide/vue'

import IdentityEditDialog from './IdentityEditDialog.vue'
import ProxyEditDialog from './ProxyEditDialog.vue'
import SSHAlgorithmsDialog from './SSHAlgorithmsDialog.vue'
import { defaultPortFor } from '../utils/connectionTypes'

import type { Identity } from '../types/identity'
import type { Proxy } from '../types/proxy'
// Extracted field-group subcomponents & composables
import BasicFields from './connection-form/BasicFields.vue'
import AuthFields from './connection-form/AuthFields.vue'
import AdvancedFields from './connection-form/AdvancedFields.vue'
import { createInitialForm, resetFormFields } from './connection-form/formDefaults'
import { useRdpResolution, RDP_CUSTOM_RESOLUTION } from './connection-form/useRdpResolution'
import { useConnectionCategories } from './connection-form/useConnectionCategories'
import { useConnectionFilePickers } from './connection-form/useConnectionFilePickers'
import { useK8sContexts } from './connection-form/useK8sContexts'
import { useShellOptions } from './connection-form/useShellOptions'
import { useGroupDialog } from './connection-form/useGroupDialog'
import {
  normalizeForm,
  validateContainer,
  validateX11Desktop,
  runConnectionTest,
  cloneExpectSteps,
} from '../composables/useFormValidation'

const { t } = useI18n()
const connectionStore = useConnectionStore()

const identityStore = useIdentityStore()
const proxyStore = useProxyStore()

// Gates the keyText paste area: the private key stays hidden behind a single
// "show" button until the user reveals it. Declared early (top of setup) so
// resetForm — fired by an immediate watch during setup — can read it without
// hitting the temporal dead zone (a late declaration there caused a ReferenceError
// that blanked the whole form in dev).
const keyContentRevealed = ref(false)
// Gates the inline kubeconfig paste area, mirroring keyContentRevealed: hidden
// behind a single "show" button so a stored kubeconfig isn't dumped on screen
// the moment the form opens.
const k8sConfigRevealed = ref(false)

onMounted(() => {
  identityStore.load()
  proxyStore.load()
})

// ── Platform detection (before allSubTypes so it's available in computed closures) ──
const isWindows = ref(/windows/i.test(navigator.userAgent) || /win32/i.test(navigator.platform))
const isMac = ref(/mac/i.test(navigator.platform))
const platform = ref<string>('')
GetPlatform().then((p: string) => {
  platform.value = p
  isWindows.value = p === 'windows'
  isMac.value = p === 'darwin'
})

// ── Categories & sub-types (extracted to useConnectionCategories) ──
const form = reactive(createInitialForm())
const {
  categories,
  currentSubTypes,
  category,
  isSubTypeActive,
  selectType,
  onCategorySelect,
} = useConnectionCategories(form, isWindows, t)

// ── Shell options (extracted to useShellOptions) ──
const { localShellOptions, wslShellOptions, shellOptionsForType } = useShellOptions(form)

const passwordInputKey = ref(0)

const postLoginMode = ref<'script' | 'expect'>('script')
const showAdvanced = ref(false)

// Connection types that support the "test connection" button (issue #377).
// local/serial/mosh/vnc/rdp/spice/x11-desktop don't have a non-interactive probe.
const TESTABLE_TYPES = ['ssh', 'telnet', 'ftp', 'sftp', 'scp', 's3', 'webdav', 'smb', 'database', 'mongodb', 'redis', 'elasticsearch', 'k8s', 'container', 'tcp']
// Test-connection result state: 'checking' shows a spinner; 'success'/'error'
// keep a colored status icon on the button until the next test or a reopen.
const testStatus = ref<'idle' | 'checking' | 'success' | 'error'>('idle')
const canTest = computed(() => TESTABLE_TYPES.includes(form.type))

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

const props = defineProps<{
  modelValue: boolean
  editConfig?: ConnectionConfig
  defaultGroupId?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  save: [config: ConnectionConfig]
  connect: [config: ConnectionConfig]
  connectOnly: [config: ConnectionConfig]
  cancel: []
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

// Dismissal bookkeeping: close routed through save/connect/connectOnly is
// "handled" (the host already cleared its state); any other close — cancel
// button, X, Esc, overlay click — reports `cancel` so the host can reset the
// edited config. Reset on every open.
let handled = false
// Form reset is deferred to the dialog's @closed event: resetting while the
// close transition is still fading out would flash default values (port 22,
// no group) for a frame. Reopening the dialog cancels the pending reset.
let pendingReset = false
watch(visible, (v) => {
  if (v) {
    handled = false
    pendingReset = false
  } else if (!handled) {
    emit('cancel')
  }
})

// Fires after the close transition finishes, so the reset below can't flash
// default values (port 22, no group) while the dialog is still fading out.
function onDialogClosed() {
  if (!pendingReset) return
  pendingReset = false
  resetForm()
  if (props.defaultGroupId) {
    selectedGroupId.value = props.defaultGroupId
    form.groupId = props.defaultGroupId
  }
}

// Ref to the BasicFields subcomponent so we can focus its host input on open.
const basicFieldsRef = ref<InstanceType<typeof BasicFields> | null>(null)

watch(visible, (val) => {
  if (val) {
    testStatus.value = 'idle'
    passwordInputKey.value++
    // Apply group on open: the defaultGroupId watch won't fire when the value
    // is unchanged, so restore it here to avoid losing the group on the second
    // consecutive create within the same group.
    if (!props.editConfig) {
      selectedGroupId.value = props.defaultGroupId
      form.groupId = props.defaultGroupId
    }
  }
})

function onDialogOpened() {
  // New connections: focus the host field by default. The dialog's @opened
  // event fires after the open transition, so the host input is mounted.
  if (!isEdit.value) {
    nextTick(() => basicFieldsRef.value?.hostInputRef?.focus())
  }
}

const isEdit = computed(() => !!props.editConfig?.id)

const sshConnections = computed(() =>
  connectionStore.connections
    .filter(c => c.type === 'ssh' && c.id !== form.id)
    .sort((a, b) => a.name.localeCompare(b.name))
)

// Derived flags retained here because the validation composable consumes them.
const isSqliteDb = computed(() =>
  form.type === 'database' && form.dbType === 'sqlite'
)
const isSSHAlgoType = computed(() => ['ssh', 'scp', 'sftp', 'mosh', 'x11-desktop'].includes(form.type))


// SSH algorithm mode: a connection-level preference stored in
// sshAlgorithms. undefined = compatible (the built-in full set); custom
// mode opens the algorithm editor to fill the four preference lists.
const sshAlgoDialogVisible = ref(false)
const sshAlgoMode = computed<'compatible' | 'secure' | 'custom'>({
  get: () => form.sshAlgorithms?.mode ?? 'compatible',
  set: (mode) => {
    if (mode === 'compatible') {
      form.sshAlgorithms = undefined
      return
    }
    if (mode === 'secure') {
      form.sshAlgorithms = { mode: 'secure' }
      return
    }
    form.sshAlgorithms = { mode: 'custom', keyExchanges: [], ciphers: [], macs: [], hostKeys: [] }
    sshAlgoDialogVisible.value = true
  },
})

function onSSHAlgoSaved(config: SSHAlgoConfig) {
  form.sshAlgorithms = config
}

// ── RDP resolution picker (extracted to useRdpResolution) ──
const {
  rdpResolution,
  rdpCustomWidth,
  rdpCustomHeight,
  rdpResolutionOptions,
  syncFromConfig: syncRdpFromConfig,
} = useRdpResolution(form, t)

const selectedGroupId = ref<string | undefined>(undefined)

// ── Group tree + new-group dialog (extracted to useGroupDialog) ──
const {
  groupTreeData,
  showNewGroupDialog,
  newGroupName,
  newGroupParentId,
  onGroupSelect,
  confirmNewGroup,
} = useGroupDialog(form, selectedGroupId, t)

// New identity / proxy dialogs (reuse the settings ones, in "create" mode)
const identityDialogVisible = ref(false)
const proxyDialogVisible = ref(false)

function openNewIdentityDialog() {
  identityDialogVisible.value = true
}

function openNewProxyDialog() {
  proxyDialogVisible.value = true
}

// After creating an item from the "+" button, select it immediately so the
// user keeps typing. The stores are already up to date (the dialogs add via
// the same stores), so no explicit reload is needed.
function onIdentitySaved(id: Identity) {
  form.identityId = id.id
}

function onProxySaved(p: Proxy) {
  form.proxyId = p.id
}

// ── K8s state (extracted to useK8sContexts) ──
// hydrating is owned here because it also gates the edit-config hydration.
const hydrating = ref(false)
const {
  k8sSourceMode,
  k8sContexts,
  k8sContextsLoading,
  k8sContextsError,
  k8sNamespaceOptions,
  reloadK8sContexts,
  onK8sNamespaceInput,
  resetK8s,
} = useK8sContexts(form, hydrating)

watch(() => props.editConfig, (config) => {
  if (config) {
    // Always reset first so fields absent from the config (e.g. an empty
    // password) don't leak values from a previously edited connection. Then
    // merge the config over the clean defaults.
    resetForm()
    hydrating.value = true
    Object.assign(form, { ...config, postLoginExpectSteps: cloneExpectSteps(config.postLoginExpectSteps || []) })
    if (!form.termType) form.termType = 'xterm-256color'
    // Backfill legacy ES auth: older versions stored the auth type in a dedicated
    // `esAuthType` ('basic'|'apikey') and the key in `esApiKey`. Both now live in
    // the shared `authType` ('password'|'apikey') and `password` fields.
    if (form.type === 'elasticsearch') {
      const legacy = config as any
      const legacyType = legacy?.esAuthType
      if (legacyType) {
        form.authType = legacyType === 'apikey' ? 'apikey' : 'password'
        if (form.authType === 'apikey' && !form.password && legacy?.esApiKey) {
          form.password = legacy.esApiKey
        }
      } else if (form.authType !== 'apikey' && form.authType !== 'password') {
        form.authType = 'password'
      }
    }
    // Existing connections without the field default to NLA off (old behavior).
    form.rdpEnableNLA = config.rdpEnableNLA ?? false
    form.rdpAdminSession = config.rdpAdminSession ?? false
    form.x11Forwarding = config.x11Forwarding ?? false
    form.agentForwarding = config.agentForwarding ?? false
    // Existing SSH connections without the field default to SFTP (old behavior).
    form.fileTransferProto = config.fileTransferProto ?? 'sftp'
    // Redis key separator defaults to ":" (empty from old connections = ":").
    form.redisKeySeparator = config.redisKeySeparator ?? ':'
    postLoginMode.value = (config.postLoginExpectSteps?.length || 0) > 0 ? 'expect' : 'script'
    selectedGroupId.value = config.groupId || undefined
    // Sync serial refs from config
    if (config.serialBaudRate) serialBaudRateInput.value = String(config.serialBaudRate)
    if (config.serialDataBits) serialDataBitsValue.value = config.serialDataBits
    if (config.serialStopBits) serialStopBitsValue.value = config.serialStopBits
    if (config.serialParity) serialParityValue.value = config.serialParity
    if (config.type === 'serial') scanSerialPorts()
    // Sync k8s source-mode radio from restored config
    if (config.type === 'k8s') {
      k8sSourceMode.value = config.k8sConfigInline ? 'inline' : 'file'
    }
    // Defaults for container connections saved before these fields existed
    if (config.type === 'container') {
      form.containerTransport = config.containerTransport ?? 'ssh'
      form.containerRuntime = config.containerRuntime ?? 'docker'
    }
    if (config.type === 'x11-desktop') {
      form.x11DesktopDesktopType = config.x11DesktopDesktopType ?? 'gnome'
      form.x11DesktopCustomCmd = config.x11DesktopCustomCmd ?? ''
    }
    // Sync resolution picker to the config's fixed size. Sizes outside the
    // preset list fall back to the custom inputs so they stay visible and
    // editable.
    syncRdpFromConfig(config)
    nextTick(() => { hydrating.value = false })
  } else {
    // Defer the reset to the dialog's @closed event — clearing editConfig
    // happens right after save while the close transition is still running,
    // and an immediate reset would flash defaults (port 22, no group).
    pendingReset = true
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
  // A result icon for the previous type is stale; clear it.
  testStatus.value = 'idle'
  if (newType !== 'ssh' && postLoginMode.value === 'expect') {
    postLoginMode.value = 'script'
  }
  if (isEdit.value) return
  const port = defaultPortFor(newType)
  if (port !== undefined) form.port = port
  if (newType === 'elasticsearch') {
    // ES shares the common `authType` field; default to 'password' unless an API key was already chosen.
    form.authType = form.authType === 'apikey' ? 'apikey' : 'password'
  }
  if (['rdp', 'vnc', 'spice', 'x11-desktop'].includes(newType) || newType === 'database') {
    form.authType = 'password'
  }
  // The shell ("terminal type") is specific to local/wsl; always reset to the
  // new type's default instead of inheriting the previous type's shell path.
  if (newType === 'local' && localShellOptions.value.length > 0) {
    form.shellPath = localShellOptions.value[0].value
  }
  if (newType === 'wsl' && wslShellOptions.value.length > 0) {
    form.shellPath = wslShellOptions.value[0].value
  }
  if (newType === 'serial') {
    scanSerialPorts()
  }
})

// Auto-switch default port when changing SQL database subtype
watch(() => form.dbType, (newType) => {
  if (isEdit.value) return
  const port = defaultPortFor('database', newType)
  if (port !== undefined) form.port = port
  // SQLite is file-backed and carries no port or tunnel.
  if (newType === 'sqlite') {
    form.port = 0
    form.tunnelSSHConnId = undefined
  }
})

// Clear the identity reference when switching away from the identity auth
// type, and don't carry credentials (username, password/passphrase, key path,
// key content, kerberos realm) across an auth type switch — each type owns its
// own fields. Skip while hydrating the form from an existing config, or
// opening the edit dialog would wipe the stored credentials.
watch(() => form.authType, (val) => {
  if (hydrating.value) return
  if (val !== 'identity') form.identityId = ''
  if (val !== 'kerberos') form.kerberosRealm = ''
  form.user = ''
  form.password = ''
  form.keyPath = ''
  form.keyContent = ''
})

function resetForm() {
  // Always re-hide the keyText paste area when the dialog (re)opens, whether
  // editing an existing connection or creating a new one, so a prior reveal
  // can't leak the previous connection's PEM into a fresh edit.
  keyContentRevealed.value = false
  // Same for the inline kubeconfig area — a stored kubeconfig is sensitive
  // material just like a private key.
  k8sConfigRevealed.value = false
  resetFormFields(form)
  postLoginMode.value = 'script'
  serialBaudRateInput.value = ''
  resetK8s()
  rdpResolution.value = 'fullscreen'
  selectedGroupId.value = undefined
}

// Sync tree-select value to form
watch(selectedGroupId, (val) => {
  form.groupId = val === '__none__' ? undefined : (val || undefined)
})


// File-picker dialogs (extracted to useConnectionFilePickers)
const {
  selectKeyFile,
  selectSqliteFile,
  useDefaultKeyPath,
  importKeyText,
  importKubeconfigText,
  pickKubeconfigFile,
} = useConnectionFilePickers(form, keyContentRevealed, k8sConfigRevealed, t)


// Shared validation-options bundle for the extracted composable.
const validationOptions = computed(() => ({
  form,
  connections: connectionStore.connections,
  t,
  isSqliteDb,
  isSSHAlgoType,
  postLoginMode,
  serialBaudRateInput,
  serialDataBitsValue,
  serialStopBitsValue,
  serialParityValue,
  sshConnections,
}))

/** Validate + normalize the form; returns undefined (with a toast) on failure. */
function validateAndNormalize(): ConnectionConfig | undefined {
  if (!validateContainer(form, sshConnections.value, t)) return
  if (!validateX11Desktop(form, t)) return
  try {
    return normalizeForm(validationOptions.value)
  } catch (e: any) {
    msg.error(e?.message || String(e))
    return
  }
}

async function onTest() {
  const config = validateAndNormalize()
  if (config) await runConnectionTest(config, testStatus)
}

/** Shared save/connect/connectOnly flow: validate, emit, then close the dialog. */
function emitAndClose(event: 'save' | 'connect' | 'connectOnly') {
  const config = validateAndNormalize()
  if (!config) return
  handled = true
  if (event === 'save') emit('save', config)
  else if (event === 'connect') emit('connect', config)
  else emit('connectOnly', config)
  visible.value = false
  if (!props.editConfig) pendingReset = true
}

function onSave() { emitAndClose('save') }
function onConnectOnly() { emitAndClose('connectOnly') }
function onConnect() { emitAndClose('connect') }
</script>

<style scoped>
/* Color the connection-test status icon (rendered via el-button's `icon` prop,
   so spacing matches the native loading spinner) by result. */
.test-status-btn.test-success :deep(.el-icon) {
  color: var(--success);
}
.test-status-btn.test-error :deep(.el-icon) {
  color: var(--error);
}

/* ── Layout ── */
/* Fills the dialog body (a padding-less flex container — see style.css):
   the left category rail stays fixed while only the right column scrolls. */
.conn-layout {
  display: flex;
  gap: 0;
  flex: 1;
  min-height: 0;
}

/* ── Left sidebar ── */
.conn-categories {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  width: 6.875rem;
  flex-shrink: 0;
  padding: 0.5rem 0.625rem;
  border-right: 1px solid var(--border-subtle);
  /* Scroll the category list inside the rail when the dialog is too short;
     the rail (and its divider) stays put. min-height: 0 lets the rail shrink
     below its content height as a flex item of .conn-layout. */
  min-height: 0;
  overflow-y: auto;
}

.cat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  min-width: 3.75rem;
  padding: 0.75rem 0.25rem;
  border-radius: var(--radius-sm);
  user-select: none;
  color: var(--text-muted);
  border-left: 0.125rem solid transparent;
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
  font-size: 0.6875rem;
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
  /* Vertical + right spacing moved here from the dialog body (which is
     padding-less so the rail divider spans header-to-footer) */
  padding: 1rem 1.25rem 1rem 1rem;
  /* The scrolling happens here, not in the dialog body, so the left
     category rail never moves */
  overflow-y: auto;
}

/* ── Sub-type icon grid ── */
.subtype-grid {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.25rem;
  padding-bottom: 0.875rem;
  margin-bottom: 0.75rem;
  border-bottom: 1px solid var(--border-subtle);
}

.subtype-btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.1875rem;
  min-width: 4rem;
  height: 3.25rem;
  padding: 0.25rem 0.5rem;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-family: var(--font-ui);
  font-size: 0.6875rem;
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
  box-shadow: 0 0 0 1px var(--accent-glow), 0 0.125rem 0.5rem var(--accent-glow);
}

.subtype-btn span {
  text-align: center;
  line-height: 1.2;
  font-size: 0.6875rem;
  white-space: nowrap;
}

/* ── Form fields ── */
.conn-fields {
  padding-right: 0.25rem;
}

/* ── Dialog overrides ── */
/* The body's padding/display override lives in style.css (.conn-dialog):
   scoped :deep() can't reach the teleport'd append-to-body dialog. */
</style>
