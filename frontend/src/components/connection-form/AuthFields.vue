<template>
  <el-form-item v-if="form.type === 'ssh' || form.type === 'scp' || form.type === 'sftp' || form.type === 'mosh' || form.type === 'x11-desktop' || isElasticsearch" :label="t('conn.authType')">
    <el-radio-group v-model="form.authType">
      <el-radio-button v-if="form.type === 'ssh' || form.type === 'scp' || form.type === 'sftp' || form.type === 'mosh' || form.type === 'x11-desktop'" value="identity">{{ t('conn.identity') }}</el-radio-button>
      <el-radio-button value="password">{{ t('conn.password') }}</el-radio-button>
      <el-radio-button v-if="form.type === 'ssh' || form.type === 'scp' || form.type === 'sftp' || form.type === 'mosh' || form.type === 'x11-desktop'" value="key">{{ t('conn.keyPath') }}</el-radio-button>
      <el-radio-button v-if="form.type === 'ssh' || form.type === 'scp' || form.type === 'sftp' || form.type === 'mosh' || form.type === 'x11-desktop'" value="keyText">{{ t('conn.keyText') }}</el-radio-button>
      <el-radio-button v-if="(form.type === 'ssh' || form.type === 'scp' || form.type === 'sftp' || form.type === 'mosh' || form.type === 'x11-desktop') && !isMobilePlatform()" value="kerberos">{{ t('conn.kerberos') }}</el-radio-button>
      <el-radio-button v-if="(isWindows || isMac) && (form.type === 'ssh' || form.type === 'scp' || form.type === 'sftp' || form.type === 'mosh' || form.type === 'x11-desktop')" value="agent">{{ t('conn.sshAgent') }}</el-radio-button>
      <el-radio-button v-if="isElasticsearch" value="apikey">{{ t('conn.esAuthApiKey') }}</el-radio-button>
    </el-radio-group>
  </el-form-item>
  <el-form-item
    v-if="form.authType === 'identity' && isSSHAlgoType"
    :label="t('conn.identity')"
  >
    <div class="inline-add-row">
      <el-select v-model="form.identityId" filterable style="flex: 1; min-width: 0">
        <el-option v-for="id in identityStore.identities" :key="id.id" :label="`${id.name} (${id.username})`" :value="id.id" />
      </el-select>
      <el-button class="inline-add-btn" :title="t('conn.newIdentity')" @click="emit('openNewIdentityDialog')">
        <Plus :size="'0.875rem'" />
      </el-button>
    </div>
  </el-form-item>
  <el-form-item v-if="(form.authType !== 'identity' || isSSHAlgoType) && form.type !== 'vnc' && form.type !== 'spice' && !(form.type === 'database' && form.dbType === 'rqlite') && !isSqliteDb && form.type !== 'local' && form.type !== 'wsl' && form.type !== 'serial' && form.type !== 'tcp' && form.type !== 'k8s' && form.type !== 'container' && !isEsApiKey" :label="form.authType === 'identity' ? t('conn.identityUserOverride') : (form.type === 's3' ? 'Access Key' : t('conn.user'))">
    <el-input v-model="form.user" :placeholder="form.authType === 'identity' ? t('conn.identityUserOverrideHint') : (form.type === 's3' ? 'Access Key ID' : t('conn.userPlaceholder'))" />
  </el-form-item>
  <el-form-item v-if="form.type === 'rdp' && isWindows && form.authType !== 'identity'" :label="t('conn.rdpDomain')">
    <el-input v-model="form.rdpDomain" placeholder="e.g. WORKGROUP or WIN-ABC123" />
  </el-form-item>
  <template v-if="form.type === 'rdp' && isWindows">
    <el-form-item :label="t('conn.rdpEnableNLA')">
      <el-select v-model="form.rdpEnableNLA" style="width: 100%">
        <el-option :value="true" :label="t('conn.rdpEnableNLAOn')" />
        <el-option :value="false" :label="t('conn.rdpEnableNLAOff')" />
      </el-select>
    </el-form-item>
  </template>
  <el-form-item v-if="form.type !== 'local' && form.type !== 'wsl' && form.type !== 'serial' && form.type !== 'tcp' && form.type !== 'k8s' && form.type !== 'container' && form.authType !== 'identity' && ((form.authType === 'password' && form.type !== 'rdp') || (form.type === 'rdp' && !form.rdpEnableNLA) || form.type === 'vnc' || form.type === 'spice' || form.type === 'database' || form.type === 'telnet' || form.type === 'ftp' || form.type === 'smb' || form.type === 'webdav' || form.type === 's3' || form.type === 'elasticsearch') && !(form.type === 'database' && form.dbType === 'rqlite') && !isSqliteDb" :label="form.type === 's3' ? 'Secret Key' : (isEsApiKey ? t('conn.esApiKey') : t('conn.password'))">
    <el-input v-model="form.password" type="password" show-password :key="passwordInputKey" :placeholder="form.type === 's3' ? 'Secret Access Key' : (isEsApiKey ? t('conn.esApiKeyPlaceholder') : '')" />
  </el-form-item>
  <el-form-item v-if="form.authType === 'kerberos' && !isMobilePlatform()" :label="t('conn.kerberos')">
    <div class="field-hint">{{ t('conn.kerberosHint') }}</div>
  </el-form-item>
  <el-form-item v-if="form.authType === 'kerberos' && !isMobilePlatform()" :label="t('conn.kerberosRealm')">
    <el-input v-model="form.kerberosRealm" :placeholder="t('conn.kerberosRealmPlaceholder')" />
    <div class="field-hint">{{ t('conn.kerberosRealmHint') }}</div>
  </el-form-item>
  <template v-if="isElasticsearch">
    <el-form-item :label="t('conn.esUseSsl')">
      <el-switch v-model="form.esUseSsl" />
    </el-form-item>
    <el-form-item v-if="form.esUseSsl" :label="t('conn.esSkipVerify')">
      <div class="nla-row">
        <el-switch v-model="form.esSkipVerify" />
        <span class="field-hint hint-after-switch">{{ t('conn.esSkipVerifyHint') }}</span>
      </div>
    </el-form-item>
  </template>
  <template v-if="isRedisSentinel">
    <el-form-item :label="t('conn.sentinelUser')">
      <el-input v-model="form.sentinelUser" :placeholder="t('conn.sentinelAuthHint')" />
    </el-form-item>
    <el-form-item :label="t('conn.sentinelPassword')">
      <el-input v-model="form.sentinelPassword" type="password" show-password :key="passwordInputKey" :placeholder="t('conn.sentinelAuthHint')" />
    </el-form-item>
  </template>
  <el-form-item v-if="form.authType === 'key' && (form.type === 'ssh' || form.type === 'scp' || form.type === 'sftp' || form.type === 'mosh' || form.type === 'x11-desktop')" :label="t('conn.keyPath')">
    <el-input v-model="form.keyPath" :placeholder="t('conn.keyPathPlaceholder')">
      <template #append>
        <div class="key-path-actions">
          <el-tooltip :content="t('conn.selectKeyFile')" placement="top">
            <el-button :aria-label="t('conn.selectKeyFile')" @click="emit('selectKeyFile')">
              <el-icon><FolderOpen :size="'1rem'" /></el-icon>
            </el-button>
          </el-tooltip>
          <span class="key-path-divider" aria-hidden="true"></span>
          <el-tooltip :content="t('conn.useDefaultKey')" placement="top">
            <el-button :aria-label="t('conn.useDefaultKey')" @click="emit('useDefaultKeyPath')">
              <el-icon><KeyRound :size="'1rem'" /></el-icon>
            </el-button>
          </el-tooltip>
        </div>
      </template>
    </el-input>
  </el-form-item>
  <el-form-item v-if="form.authType === 'keyText' && (form.type === 'ssh' || form.type === 'scp' || form.type === 'sftp' || form.type === 'mosh' || form.type === 'x11-desktop')" :label="t('conn.keyContent')">
    <template v-if="!keyContentRevealed">
      <el-button size="small" @click="emit('update:keyContentRevealed', true)">
        <el-icon><Eye :size="'0.875rem'" /></el-icon>
        <span style="margin-left: 0.25rem">{{ t('conn.keyTextReveal') }}</span>
      </el-button>
    </template>
    <template v-else>
      <el-input
        v-model="form.keyContent"
        type="textarea"
        :rows="7"
        class="key-text-area mono"
        :placeholder="t('conn.keyContentPlaceholder')"
        spellcheck="false"
      />
      <div class="key-content-actions">
        <el-button size="small" @click="emit('update:keyContentRevealed', false)">
          <el-icon><EyeOff :size="'0.875rem'" /></el-icon>
          <span style="margin-left: 0.25rem">{{ t('conn.keyTextHide') }}</span>
        </el-button>
        <el-button size="small" @click="emit('importKeyText')">
          <el-icon><FolderOpen :size="'0.875rem'" /></el-icon>
          <span style="margin-left: 0.25rem">{{ t('conn.importFromFile') }}</span>
        </el-button>
      </div>
    </template>
  </el-form-item>
  <el-form-item v-if="(form.authType === 'key' || form.authType === 'keyText') && (form.type === 'ssh' || form.type === 'scp' || form.type === 'sftp' || form.type === 'mosh' || form.type === 'x11-desktop')" :label="t('conn.keyPassphrase')">
    <el-input v-model="form.password" type="password" show-password :key="passwordInputKey" :placeholder="t('conn.keyPassphrasePlaceholder')" />
  </el-form-item>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Plus, FolderOpen, Eye, EyeOff, KeyRound } from '@lucide/vue'
import { useI18n } from '../../i18n'
import { useIdentityStore } from '../../stores/identityStore'
import { isMobilePlatform } from '../../utils/platform'
import type { ConnectionConfig } from '../../types/session'

const { t } = useI18n()
const identityStore = useIdentityStore()

const props = defineProps<{
  form: ConnectionConfig
  isWindows: boolean
  isMac: boolean
  /** Increments on every dialog open to force password inputs to remount clean. */
  passwordInputKey: number
  /** Whether the inline private-key paste area is revealed. */
  keyContentRevealed: boolean
}>()

const emit = defineEmits<{
  'update:keyContentRevealed': [value: boolean]
  openNewIdentityDialog: []
  selectKeyFile: []
  useDefaultKeyPath: []
  importKeyText: []
}>()

// Derived flags that only depend on `form`.
const isSqliteDb = computed(() => props.form.type === 'database' && props.form.dbType === 'sqlite')
const isRedisSentinel = computed(() => props.form.type === 'redis' && props.form.redisMode === 'sentinel')
const isElasticsearch = computed(() => props.form.type === 'elasticsearch')
const isEsApiKey = computed(() => isElasticsearch.value && props.form.authType === 'apikey')
const isSSHAlgoType = computed(() => ['ssh', 'scp', 'sftp', 'mosh', 'x11-desktop'].includes(props.form.type))
</script>

<style scoped>
/* Key-path field: the folder-picker and use-default buttons share one append
   slot. Element Plus styles a single append button with `margin: 0 -20px` to
   cancel the slot's padding; with two buttons that negative margin makes them
   overlap each other, so neutralise it and let the pair flow side by side. */
.key-path-actions {
  display: flex;
  align-items: center;
  height: 100%;
  margin: 0 -20px;
}
.key-path-actions :deep(.el-button) {
  margin: 0;
}
/* Thin vertical divider between the two append buttons. */
.key-path-divider {
  width: 1px;
  align-self: stretch;
  margin: 6px 0;
  background: var(--el-input-border-color);
}
/* KeyText paste area renders in a monospace font so PEM blocks stay aligned. */
.key-text-area :deep(textarea) {
  font-family: var(--font-mono, ui-monospace, "JetBrains Mono", monospace);
}
.key-content-actions {
  margin-top: 0.375rem;
}
.key-content-actions .el-button + .el-button {
  margin-left: 0.5rem;
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
/* ── Inline select + "+" rows (identity) ── */
.inline-add-row {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  width: 100%;
}
.inline-add-btn {
  flex-shrink: 0;
  width: 2rem;
  height: 2rem;
  padding: 0;
}
</style>