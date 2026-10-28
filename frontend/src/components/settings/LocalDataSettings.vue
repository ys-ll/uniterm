<template>
  <div class="settings-section" :class="{ narrow: isNarrow }">
    <h2 class="section-title">{{ t('settings.localData') }}</h2>
    <div class="settings-group">
      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('config.dataDir') }}</div>
          <div class="setting-desc">{{ t('config.dataDirDesc') }}</div>
        </div>
        <div class="setting-control">
          <div class="config-value">{{ dataDirTypeLabel }}</div>
          <el-button size="small" @click="openDataDirChange">{{ t('config.changeDir') }}</el-button>
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('config.encryption') }}</div>
          <div class="setting-desc">{{ t('config.encryptionDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="modeSelect" @change="onModeChange" popper-class="mode-select-popper" style="width: 100%">
            <el-option v-if="!credStore.status.mode" :label="t('config.notConfigured')" value="" disabled />
            <el-option :label="t('encrypt.keychain')" value="keychain">
              <div class="mode-option">
                <div class="mode-option-title">{{ t('encrypt.keychain') }}</div>
                <div class="mode-option-desc">{{ t('config.switchKeychainHint') }}</div>
              </div>
            </el-option>
            <el-option :label="t('encrypt.masterPassword')" value="master-password">
              <div class="mode-option">
                <div class="mode-option-title">{{ t('encrypt.masterPassword') }}</div>
                <div class="mode-option-desc">{{ t('config.switchMasterHint') }}</div>
              </div>
            </el-option>
          </el-select>
        </div>
      </div>

      <div v-if="credStore.status.mode === 'master-password'" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('config.masterPassword') }}</div>
          <div class="setting-desc">{{ t('config.changePasswordDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-button size="small" @click="openChangePassword">{{ t('config.changePassword') }}</el-button>
        </div>
      </div>
    </div>

    <!-- Config dialogs -->
    <DataDirDialog v-model:visible="dataDirVisible" :first-run="false" @done="onDataDirChange" />

    <el-dialog append-to-body v-model="showSetupMaster" :title="t('config.setupMasterTitle')" width="27.5rem">
      <el-form label-width="6.25rem" class="settings-form" @submit.prevent="doSetupMaster">
        <el-form-item :label="t('encrypt.password')">
          <el-input v-model="setupPw" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('encrypt.confirm')">
          <el-input v-model="setupPw2" type="password" show-password />
        </el-form-item>
      </el-form>
      <div v-if="setupError" class="form-error">{{ setupError }}</div>
      <template #footer>
        <el-button @click="showSetupMaster = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="setupSubmitting" @click="doSetupMaster">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog append-to-body v-model="showVerifyMaster" :title="t('config.verifyMasterTitle')" width="27.5rem">
      <el-form label-width="6.25rem" class="settings-form" @submit.prevent="doVerifyMaster">
        <el-form-item :label="t('config.verifyCurrentPassword')">
          <div class="verify-field">
            <el-input v-model="verifyPw" type="password" show-password />
            <div class="switch-mode-hint">{{ t('config.verifyCurrentHint') }}</div>
          </div>
        </el-form-item>
      </el-form>
      <div v-if="verifyError" class="form-error">{{ verifyError }}</div>
      <template #footer>
        <el-button @click="showVerifyMaster = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="verifySubmitting" @click="doVerifyMaster">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog append-to-body v-model="showChangePassword" :title="t('config.changePassword')" width="27.5rem">
      <el-form label-width="6.25rem" class="settings-form" @submit.prevent="doChangePassword">
        <el-form-item :label="t('config.oldPassword')">
          <el-input v-model="oldPw" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('config.newPassword')">
          <el-input v-model="newPw" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('encrypt.confirm')">
          <el-input v-model="newPw2" type="password" show-password />
        </el-form-item>
      </el-form>
      <div v-if="changePwError" class="form-error">{{ changePwError }}</div>
      <template #footer>
        <el-button @click="showChangePassword = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="changePwSubmitting" @click="doChangePassword">{{ t('common.confirm') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { msg } from '../../services/message'
import { RelaunchApp } from '../../../bindings/github.com/ys-ll/uniterm/app'
import { useCredentialStore } from '../../stores/credentialStore'
import { useI18n } from '../../i18n'
import { backendErrorText } from '../../utils/backendError'
import DataDirDialog from '../DataDirDialog.vue'

defineProps<{ isNarrow: boolean }>()

const credStore = useCredentialStore()
const { t } = useI18n()

const dataDirVisible = ref(false)
const showSetupMaster = ref(false)
const showVerifyMaster = ref(false)
const showChangePassword = ref(false)

const dataDirTypeLabel = computed(() => {
  if (!credStore.dataDirInfo.dataDir) return t('config.notSet')
  switch (credStore.dataDirInfo.type) {
    case 'default': return t('dataDir.default')
    case 'portable': return t('dataDir.portable')
    case 'custom': return t('dataDir.custom')
    default: return t('config.notSet')
  }
})

function openDataDirChange() {
  dataDirVisible.value = true   // reuse DataDirDialog (change mode: migrate checkbox visible)
}
function onDataDirChange(restart: boolean) {
  if (restart) {
    RelaunchApp()
  }
}
async function openChangePassword() { showChangePassword.value = true }

// Encryption-mode dropdown. The select mirrors the live mode; choosing a
// different option reverts immediately and opens the matching directional
// dialog instead. modeSelect only advances after a successful switch (via the
// watch below), so cancelling a dialog leaves the dropdown unchanged.
const modeSelect = ref<string>('')
watch(() => credStore.status.mode, (m) => { modeSelect.value = m || '' }, { immediate: true })

function onModeChange(val: string) {
  modeSelect.value = credStore.status.mode || ''  // revert until the switch actually lands
  if (!val || val === credStore.status.mode) return
  if (credStore.status.mode === '') {
    // No credentials configured yet: no existing password to verify.
    if (val === 'keychain') doSetupKeychain()
    else showSetupMaster.value = true
    return
  }
  if (val === 'master-password') showSetupMaster.value = true
  else showVerifyMaster.value = true
}

// keychain → master-password (or first-run master-password): configure a new password.
const setupPw = ref(''); const setupPw2 = ref('')
const setupError = ref(''); const setupSubmitting = ref(false)
watch(showSetupMaster, (v) => { if (v) { setupPw.value = ''; setupPw2.value = ''; setupError.value = '' } })

async function doSetupMaster() {
  if (setupSubmitting.value) return
  setupError.value = ''
  if (!setupPw.value) { setupError.value = t('encrypt.passwordRequired'); return }
  if (setupPw.value !== setupPw2.value) { setupError.value = t('encrypt.mismatch'); return }
  setupSubmitting.value = true
  try {
    if (credStore.status.mode === '') await credStore.setup('master-password', setupPw.value)
    else await credStore.switchMode('master-password', setupPw.value)
    showSetupMaster.value = false
    msg.success(t('config.switchDone'))
  } catch (e: any) { setupError.value = backendErrorText(e) }
  finally { setupSubmitting.value = false }
}

// master-password → keychain: verify the current master password once.
const verifyPw = ref('')
const verifyError = ref(''); const verifySubmitting = ref(false)
watch(showVerifyMaster, (v) => { if (v) { verifyPw.value = ''; verifyError.value = '' } })

async function doVerifyMaster() {
  if (verifySubmitting.value) return
  verifyError.value = ''
  if (!verifyPw.value) { verifyError.value = t('encrypt.passwordRequired'); return }
  verifySubmitting.value = true
  try {
    await credStore.switchMode('keychain', verifyPw.value)
    showVerifyMaster.value = false
    msg.success(t('config.switchDone'))
  } catch (e: any) { verifyError.value = backendErrorText(e) }
  finally { verifySubmitting.value = false }
}

// first-run / unconfigured → keychain: no password needed.
async function doSetupKeychain() {
  try {
    await credStore.setup('keychain', '')
    msg.success(t('config.switchDone'))
  } catch (e: any) { msg.error(backendErrorText(e)) }
}

const oldPw = ref(''); const newPw = ref(''); const newPw2 = ref('')
const changePwError = ref(''); const changePwSubmitting = ref(false)

// Clear fields each time the dialog opens so a previous password isn't shown.
watch(showChangePassword, (v) => {
  if (v) {
    oldPw.value = ''
    newPw.value = ''
    newPw2.value = ''
    changePwError.value = ''
  }
})

async function doChangePassword() {
  if (changePwSubmitting.value) return
  changePwError.value = ''
  if (!oldPw.value || !newPw.value) { changePwError.value = t('config.oldPassword') + ' / ' + t('config.newPassword'); return }
  if (newPw.value !== newPw2.value) { changePwError.value = t('encrypt.mismatch'); return }
  changePwSubmitting.value = true
  try {
    await credStore.changeMasterPassword(oldPw.value, newPw.value)
    showChangePassword.value = false
    msg.success(t('config.changeDone'))
  } catch (e: any) { changePwError.value = backendErrorText(e) }
  finally { changePwSubmitting.value = false }
}
</script>

<style scoped>
@import './_shared.css';
</style>