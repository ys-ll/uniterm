<template>
  <el-dialog append-to-body
    v-model="visible"
    :title="t('addRepo.title')"
    width="560px"
    :close-on-click-modal="false"
    @close="handleClose"
  >
    <el-form label-width="120px" class="add-repo-form">
      <el-form-item :label="t('addRepo.mode')">
        <el-radio-group v-model="mode">
          <el-radio-button value="remote">{{ t('addRepo.modeRemote') }}</el-radio-button>
          <el-radio-button value="local">{{ t('addRepo.modeLocal') }}</el-radio-button>
        </el-radio-group>
      </el-form-item>

      <template v-if="mode === 'remote'">
        <el-form-item :label="t('addRepo.url')">
          <el-input v-model="repoUrl" :placeholder="t('addRepo.urlPlaceholder')" />
          <div class="form-hint warning">{{ t('addRepo.urlHint') }}</div>
        </el-form-item>

        <el-form-item :label="t('addRepo.username')">
          <el-input v-model="username" :placeholder="t('addRepo.usernamePlaceholder')" />
        </el-form-item>

        <el-form-item :label="t('addRepo.token')">
          <el-input v-model="token" type="password" show-password :placeholder="t('addRepo.tokenPlaceholder')" />
          <div class="form-hint">{{ t('addRepo.tokenHint') }}</div>
        </el-form-item>
      </template>

      <template v-else>
        <el-form-item :label="t('addRepo.localPath')">
          <div class="path-row">
            <el-input v-model="localPath" :placeholder="t('addRepo.localPathPlaceholder')" />
            <el-button @click="pickLocalPath">{{ t('settings.browse') }}</el-button>
          </div>
          <div class="form-hint">{{ t('addRepo.localPathHint') }}</div>
        </el-form-item>
      </template>

      <el-form-item :label="t('addRepo.masterPassword')">
        <el-input v-model="masterPassword" type="password" show-password
          :placeholder="t('addRepo.masterPasswordPlaceholder')" />
        <div class="form-hint">{{ t('addRepo.masterPasswordHint') }}</div>
      </el-form-item>
    </el-form>

    <div v-if="errorMsg" class="form-error">{{ errorMsg }}</div>

    <template #footer>
      <el-button @click="handleClose">{{ t('common.cancel') }}</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">
        {{ t('addRepo.saveAndConnect') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from '../i18n'
import { useSyncStore } from '../stores/syncStore'
import { OpenDirectoryDialog } from '../../wailsjs/go/main/App'
import { msg } from '../services/message'

const { t } = useI18n()
const syncStore = useSyncStore()

const visible = computed({
  get: () => syncStore.showAddRepo,
  set: (v) => { if (!v) syncStore.showAddRepo = false },
})

type Mode = 'remote' | 'local'
const mode = ref<Mode>('remote')

const repoUrl = ref('')
const username = ref('')
const token = ref('')
const localPath = ref('')
const masterPassword = ref('')
const submitting = ref(false)
const errorMsg = ref('')

watch(visible, (v) => {
  if (v) {
    mode.value = 'remote'
    resetForm()
  }
})

function handleClose() {
  syncStore.showAddRepo = false
  resetForm()
}

function resetForm() {
  repoUrl.value = ''
  username.value = ''
  token.value = ''
  localPath.value = ''
  masterPassword.value = ''
  errorMsg.value = ''
}

async function pickLocalPath() {
  try {
    const chosen = await OpenDirectoryDialog()
    if (chosen) localPath.value = chosen
  } catch (e: any) {
    errorMsg.value = e?.message || String(e)
  }
}

async function handleSubmit() {
  errorMsg.value = ''
  if (!masterPassword.value) {
    errorMsg.value = t('addRepo.masterPasswordRequired')
    return
  }

  submitting.value = true
  try {
    if (mode.value === 'local') {
      if (!localPath.value.trim()) {
        errorMsg.value = t('addRepo.localPathRequired')
        submitting.value = false
        return
      }
      const result = await syncStore.configureLocalRepo(localPath.value.trim(), masterPassword.value)
      if (result && result.direction === 3) {
        syncStore.showAddRepo = false
        resetForm()
        return
      }
    } else {
      if (!repoUrl.value.trim()) {
        errorMsg.value = t('addRepo.urlRequired')
        submitting.value = false
        return
      }
      if (!username.value.trim()) {
        errorMsg.value = t('addRepo.usernameRequired')
        submitting.value = false
        return
      }
      if (!token.value.trim()) {
        errorMsg.value = t('addRepo.tokenRequired')
        submitting.value = false
        return
      }
      const result = await syncStore.configureRepo(
        repoUrl.value.trim(),
        username.value.trim(),
        token.value,
        masterPassword.value
      )
      if (result && result.direction === 3) {
        syncStore.showAddRepo = false
        resetForm()
        return
      }
    }
    msg.success(t('addRepo.success'))
    syncStore.showAddRepo = false
    resetForm()
  } catch (e: any) {
    errorMsg.value = e?.message || String(e)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.add-repo-form {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.path-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.path-row .el-input {
  flex: 1;
  min-width: 0;
}

.form-hint {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 4px;
  line-height: 1.4;
}

.form-hint.warning {
  color: var(--el-color-warning-dark-2);
}

.form-error {
  color: var(--el-color-danger);
  font-size: 13px;
  margin-top: 8px;
}
</style>
