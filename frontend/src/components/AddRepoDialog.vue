<template>
  <el-dialog append-to-body
    v-model="visible"
    :title="t('addRepo.title')"
    width="32.5rem"
    :close-on-click-modal="false"
    @close="handleClose"
  >
    <el-form label-width="6.25rem" class="add-repo-form">
      <el-form-item :label="t('addRepo.backend')">
        <el-radio-group v-model="backendType">
          <el-radio-button value="git">{{ t('addRepo.backendGit') }}</el-radio-button>
          <el-radio-button value="webdav">{{ t('addRepo.backendWebDAV') }}</el-radio-button>
          <el-radio-button value="sftp">SFTP</el-radio-button>
          <el-radio-button value="s3">S3</el-radio-button>
          <el-radio-button value="smb">SMB</el-radio-button>
        </el-radio-group>
      </el-form-item>

      <template v-if="backendType === 'git'">
        <el-form-item :label="t('addRepo.url')">
          <el-input
            v-model="repoUrl"
            :placeholder="t('addRepo.urlPlaceholder')"
          />
          <div class="form-hint warning">{{ t('addRepo.urlHint') }}</div>
        </el-form-item>

        <el-form-item :label="t('addRepo.username')">
          <el-input
            v-model="username"
            :placeholder="t('addRepo.usernamePlaceholder')"
          />
        </el-form-item>

        <el-form-item :label="t('addRepo.token')">
          <el-input
            v-model="token"
            type="password"
            show-password
            :placeholder="t('addRepo.tokenPlaceholder')"
          />
        </el-form-item>
      </template>

      <template v-else>
        <el-form-item :label="t('addRepo.serverUrl')">
          <el-input
            v-model="remoteServer"
            :placeholder="serverPlaceholder"
          />
          <div class="form-hint">{{ serverHint }}</div>
        </el-form-item>

        <!-- Uniform field order for every snapshot backend: server,
             credentials (AK/SK for S3), backend-specific fields
             (share/domain or bucket/region/style), then the directory. -->
        <el-form-item :label="backendType === 's3' ? 'Access Key' : t('addRepo.username')">
          <el-input
            v-model="remoteUser"
          />
        </el-form-item>

        <el-form-item :label="backendType === 's3' ? 'Secret Key' : t('conn.password')">
          <el-input
            v-model="remoteSecret"
            type="password"
            show-password
          />
        </el-form-item>

        <template v-if="backendType === 's3'">
          <el-form-item :label="t('conn.s3Bucket')">
            <el-input
              v-model="s3Bucket"
              :placeholder="t('conn.s3BucketPlaceholder')"
            />
          </el-form-item>

          <el-form-item :label="t('conn.s3Region')">
            <el-input
              v-model="s3Region"
              :placeholder="t('conn.s3RegionPlaceholder')"
            />
          </el-form-item>

          <el-form-item :label="t('conn.s3UrlStyle')">
            <el-select v-model="s3UrlStyle">
              <el-option label="Virtual-hosted (https://bucket.endpoint/key)" value="virtual" />
              <el-option label="Path (https://endpoint/bucket/key)" value="path" />
            </el-select>
          </el-form-item>
        </template>

        <template v-if="backendType === 'smb'">
          <el-form-item :label="t('conn.smbShare')">
            <el-input
              v-model="smbShare"
              placeholder="Share name"
            />
          </el-form-item>

          <el-form-item :label="t('conn.smbDomain')">
            <el-input
              v-model="smbDomain"
              placeholder="e.g. WORKGROUP"
            />
          </el-form-item>
        </template>

        <el-form-item :label="t('addRepo.basePath')">
          <!-- Prefilled with the sync directory name (issue #1589): a
               placeholder read like a default, so users left the field
               empty expecting uniTerm-sync to be used and WebDAV servers
               rejecting root writes failed with e.g. "MkdirAll /: 403".
               The backend treats an empty path as the server root. -->
          <el-input
            v-model="remotePath"
          />
        </el-form-item>
      </template>


      <el-form-item :label="t('addRepo.masterPassword')">
        <el-input
          v-model="masterPassword"
          type="password"
          show-password
          :placeholder="t('addRepo.masterPasswordPlaceholder')"
        />
        <div class="form-hint">{{ t('addRepo.masterPasswordHint') }}</div>
      </el-form-item>

      <el-form-item :label="t('importExport.confirmPassword')">
        <el-input
          v-model="confirmPassword"
          type="password"
          show-password
        />
      </el-form-item>
    </el-form>

    <div v-if="errorMsg" class="form-error">{{ errorMsg }}</div>

    <template #footer>
      <el-button @click="handleClose">{{ t('common.cancel') }}</el-button>
      <el-button
        v-if="backendType !== 'git'"
        :loading="testingRemote"
        @click="handleTestRemote"
      >
        {{ t('conn.testConnection') }}
      </el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">
        {{ t('addRepo.saveAndConnect') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from '../i18n'
import { useSyncStore, type RemoteConfig } from '../stores/syncStore'
import { backendErrorText } from '../utils/backendError'
import { msg } from '../services/message'

const { t } = useI18n()
const syncStore = useSyncStore()

const visible = computed({
  get: () => syncStore.showAddRepo,
  set: (v) => { if (!v) syncStore.showAddRepo = false },
})

const backendType = ref<'git' | 'webdav' | 'sftp' | 's3' | 'smb'>('git')
const repoUrl = ref('')
const username = ref('')
const token = ref('')
const remoteServer = ref('')
// Real prefilled value, not a placeholder: an empty path means the server
// root on every snapshot backend, which WebDAV services like jianguoyun
// reject on write (issue #1589).
const remotePath = ref('uniTerm-sync')
const remoteUser = ref('')
const remoteSecret = ref('')
const s3Bucket = ref('')
const s3Region = ref('')
const s3UrlStyle = ref('virtual')
const smbShare = ref('')
const smbDomain = ref('')
const masterPassword = ref('')
const confirmPassword = ref('')
const submitting = ref(false)
const testingRemote = ref(false)
const errorMsg = ref('')

// Per-backend server placeholder/hint. WebDAV takes a full URL; sftp/smb
// take host[:port]; s3 takes the endpoint.
const serverHint = computed(() => {
  switch (backendType.value) {
    case 'webdav': return t('addRepo.serverUrlHint')
    case 's3': return t('addRepo.s3EndpointHint')
    case 'sftp': return t('addRepo.sftpServerHint')
    default: return t('addRepo.smbServerHint')
  }
})

const serverPlaceholder = computed(() => {
  switch (backendType.value) {
    case 'webdav': return 'https://dav.example.com'
    case 's3': return 'https://s3.example.com'
    default: return 'host[:port]'
  }
})

function handleClose() {
  syncStore.showAddRepo = false
  resetForm()
}

function resetForm() {
  backendType.value = 'git'
  repoUrl.value = ''
  username.value = ''
  token.value = ''
  remoteServer.value = ''
  remotePath.value = 'uniTerm-sync'
  remoteUser.value = ''
  remoteSecret.value = ''
  s3Bucket.value = ''
  s3Region.value = ''
  s3UrlStyle.value = 'virtual'
  smbShare.value = ''
  smbDomain.value = ''
  masterPassword.value = ''
  confirmPassword.value = ''
  errorMsg.value = ''
}

function buildRemote(): RemoteConfig {
  return {
    server: remoteServer.value.trim(),
    path: remotePath.value.trim(),
    user: remoteUser.value.trim(),
    bucket: backendType.value === 's3' ? s3Bucket.value.trim() : '',
    region: backendType.value === 's3' ? s3Region.value.trim() : '',
    urlStyle: backendType.value === 's3' ? s3UrlStyle.value : '',
    domain: backendType.value === 'smb' ? smbDomain.value.trim() : '',
    share: backendType.value === 'smb' ? smbShare.value.trim() : '',
  }
}

async function handleTestRemote() {
  errorMsg.value = ''
  if (!remoteServer.value.trim()) {
    errorMsg.value = t('addRepo.remoteServerRequired')
    return
  }
  if (backendType.value === 's3' && !s3Bucket.value.trim()) {
    errorMsg.value = t('addRepo.s3BucketRequired')
    return
  }
  if (backendType.value === 'smb' && !smbShare.value.trim()) {
    errorMsg.value = t('addRepo.smbShareRequired')
    return
  }
  testingRemote.value = true
  try {
    const err = await syncStore.testRemote(backendType.value, buildRemote(), remoteSecret.value)
    if (err) {
      errorMsg.value = err
    } else {
      msg.success(t('addRepo.testConnectionOk'))
    }
  } finally {
    testingRemote.value = false
  }
}

async function handleSubmit() {
  errorMsg.value = ''

  if (backendType.value === 'git') {
    if (!repoUrl.value.trim()) {
      errorMsg.value = t('addRepo.urlRequired')
      return
    }
    if (!username.value.trim()) {
      errorMsg.value = t('addRepo.usernameRequired')
      return
    }
    if (!token.value.trim()) {
      errorMsg.value = t('addRepo.tokenRequired')
      return
    }
  } else {
    if (!remoteServer.value.trim()) {
      errorMsg.value = t('addRepo.remoteServerRequired')
      return
    }
    if (backendType.value === 's3' && !s3Bucket.value.trim()) {
      errorMsg.value = t('addRepo.s3BucketRequired')
      return
    }
    if (backendType.value === 'smb' && !smbShare.value.trim()) {
      errorMsg.value = t('addRepo.smbShareRequired')
      return
    }
    if (!remoteUser.value.trim()) {
      errorMsg.value = t('addRepo.usernameRequired')
      return
    }
    if (!remoteSecret.value) {
      errorMsg.value = t('addRepo.webdavPasswordRequired')
      return
    }
  }
  if (!masterPassword.value) {
    errorMsg.value = t('addRepo.masterPasswordRequired')
    return
  }
  if (confirmPassword.value !== masterPassword.value) {
    errorMsg.value = t('importExport.passwordMismatch')
    return
  }

  submitting.value = true
  try {
    let result: Awaited<ReturnType<typeof syncStore.configureRepo>>
    if (backendType.value === 'git') {
      result = await syncStore.configureRepo(
        repoUrl.value.trim(),
        username.value.trim(),
        token.value,
        masterPassword.value
      )
    } else {
      result = await syncStore.configureRemote(
        backendType.value,
        buildRemote(),
        remoteSecret.value,
        masterPassword.value
      )
    }
    if (result && result.direction === 3) {
      // Conflict: repo connected but data differs
      syncStore.showAddRepo = false
      resetForm()
      return
    }
    msg.success(t('addRepo.success'))
    syncStore.showAddRepo = false
    resetForm()
  } catch (e: any) {
    errorMsg.value = backendErrorText(e)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.add-repo-form {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.add-repo-form :deep(.el-form-item) {
  /* The flex gap above already spaces the rows; Element Plus's
     default 18px form-item margin stacked on top of it made the
     rows feel double-spaced. */
  margin-bottom: 0;
}

.form-hint {
  font-size: 0.75rem;
  color: var(--text-muted);
  margin-top: 0.25rem;
  line-height: 1.4;
}

.form-hint.warning {
  color: var(--el-color-warning-dark-2);
}

.form-error {
  color: var(--el-color-danger);
  font-size: 0.8125rem;
  margin-top: 0.5rem;
}
</style>
