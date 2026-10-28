<template>
  <div class="settings-section sync-settings" :class="{ narrow: isNarrow }">
    <h2 class="section-title">{{ t('settings.sync') }}</h2>
    <p class="section-desc">{{ t('settings.syncDesc') }}</p>

    <!-- Empty state: no repo configured -->
    <div v-if="!isSyncConfigured" class="sync-card">
      <div class="sync-card-header">{{ t('settings.syncRepoCard') }}</div>
      <div class="sync-card-body empty-state">
        <p class="empty-text">{{ t('settings.syncEmptyDesc') }}</p>
        <el-button type="primary" @click="syncStore.showAddRepo = true">
          {{ t('settings.syncAddRepo') }}
        </el-button>
      </div>
    </div>

    <!-- Configured state -->
    <template v-else>
      <!-- Repo config card -->
      <div class="sync-card">
        <div class="sync-card-header">
          <span>{{ t('settings.syncRepoCard') }}</span>
          <el-button text @click="openEditRepo">{{ t('settings.syncEdit') }}</el-button>
        </div>
        <div class="sync-card-body">
          <div class="repo-info">
            <div class="repo-info-row">
              <span class="repo-label">{{ syncStore.config.backend !== 'git' ? t('addRepo.serverUrl') : t('settings.syncRepoUrl') }}</span>
              <span class="repo-value">{{ syncStore.config.backend !== 'git' ? syncStore.config.remote?.server : syncStore.config.repoUrl }}</span>
            </div>
            <div class="repo-info-row">
              <span class="repo-label">{{ t('settings.syncUsername') }}</span>
              <span class="repo-value">{{ syncStore.config.backend !== 'git' ? syncStore.config.remote?.user : syncStore.config.username }}</span>
            </div>
          </div>
          <div class="repo-actions">
            <el-button @click="syncStore.showChangePassword = true">{{ t('settings.syncChangePassword') }}</el-button>
            <el-button @click="syncStore.showDeleteRepo = true">{{ t('settings.syncDeleteRepo') }}</el-button>
          </div>
        </div>
      </div>

      <!-- Sync card -->
      <div class="sync-card">
        <div class="sync-card-header">{{ t('settings.syncSyncCard') }}</div>
        <div class="sync-card-body">
          <div class="sync-status">
            <div class="sync-status-row">
              <span class="sync-label">{{ t('settings.syncLastSync') }}</span>
              <span class="sync-value">{{ syncStore.formatSyncTime() }}</span>
              <span v-if="syncStore.config.lastSyncStatus === 'success'" class="sync-tag success">{{ t('settings.syncStatusSuccess') }}</span>
              <span v-else-if="syncStore.config.lastSyncStatus === 'failed'" class="sync-tag failed">{{ t('settings.syncStatusFailed') }}</span>
            </div>
            <div v-if="syncStore.config.lastSyncStatus === 'failed' && syncStore.config.lastSyncError" class="sync-status-row sync-error">
              <span class="sync-label">{{ t('settings.syncReason') }}</span>
              <span class="sync-value error-text">{{ syncStore.config.lastSyncError }}</span>
            </div>
          </div>
          <div class="sync-actions-row">
            <el-button
              type="primary"
              :loading="syncStore.syncing"
              @click="handleSyncNow"
            >
              {{ t('settings.syncNow') }}
            </el-button>
          </div>
          <div class="sync-auto-row">
            <span class="sync-auto-label">{{ t('settings.syncAuto') }}</span>
            <span class="sync-auto-desc">{{ t('settings.syncAutoDesc') }}</span>
            <el-switch v-model="syncStore.config.autoSync" @change="handleAutoSyncToggle" />
          </div>
          <div class="sync-scope-row">
            <div class="sync-scope-header">
              <span class="sync-auto-label">{{ t('settings.syncScope') }}</span>
              <span class="scope-hint">{{ t('settings.syncScopeHint') }}</span>
            </div>
            <el-checkbox-group
              v-model="syncScopeModel"
              :disabled="syncStore.syncing"
              @change="handleScopeChange"
            >
              <el-checkbox v-for="item in SYNC_FILE_ITEMS" :key="item.file" :value="item.file">
                {{ t(item.label) }}
              </el-checkbox>
            </el-checkbox-group>
          </div>
        </div>
      </div>
    </template>

    <!-- Sync dialogs -->
    <AddRepoDialog />
    <EditRepoDialog />
    <ChangePasswordDialog />
    <DeleteRepoDialog />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { msg } from '../../services/message'
import { useSyncStore, SYNC_FILE_ITEMS } from '../../stores/syncStore'
import { useI18n } from '../../i18n'
import AddRepoDialog from '../AddRepoDialog.vue'
import EditRepoDialog from '../EditRepoDialog.vue'
import ChangePasswordDialog from '../ChangePasswordDialog.vue'
import DeleteRepoDialog from '../DeleteRepoDialog.vue'

defineProps<{ isNarrow: boolean }>()

const syncStore = useSyncStore()
const { t } = useI18n()

function openEditRepo() {
  syncStore.showEditRepo = true
}

// WebDAV configure leaves repoUrl empty, so "configured" depends on backend.
const isSyncConfigured = computed(() =>
  syncStore.config.backend !== 'git'
    ? !!syncStore.config.remote?.server
    : !!syncStore.config.repoUrl
)

async function handleSyncNow() {
  const result = await syncStore.doSync()
  if (!result) {
    msg.error(syncStore.lastResult || t('settings.syncFailed'))
    return
  }
  if (result.direction === 3) {
    return  // conflict — handled by SyncConflictDialog
  }
  msg.success(result.message || t('settings.syncSuccess'))
}

async function handleAutoSyncToggle() {
  try {
    await syncStore.saveConfig()
  } catch (e) {
    console.error('Failed to save auto sync toggle:', e)
  }
}

// nil scope = legacy default: settings.json unchecked, everything else checked.
const LEGACY_SCOPE = SYNC_FILE_ITEMS.filter(i => i.file !== 'settings.json').map(i => i.file)

const syncScopeModel = computed<string[]>({
  get: () => syncStore.config.syncScope ?? LEGACY_SCOPE,
  set: (v) => { syncStore.config.syncScope = v },
})

// An empty selection would persist as [] which the backend resolves back to
// the legacy default — the UI would show nothing syncing while the device
// syncs all legacy files. Keep the last non-empty selection instead.
const lastValidScope = ref<string[]>([...syncScopeModel.value])
// Track the store reactively: loadConfig (and any future store reset) replaces
// config.value asynchronously after setup, so a one-time snapshot could go stale.
watch(() => syncStore.config.syncScope, (v) => {
  if (v && v.length) lastValidScope.value = [...v]
})

async function handleScopeChange(v: string[]) {
  if (v.length === 0) {
    syncStore.config.syncScope = [...lastValidScope.value]
    msg.warning(t('settings.syncScopeEmpty'))
    return
  }
  lastValidScope.value = [...v]
  try {
    await syncStore.saveConfig()
  } catch (e) {
    console.error('Failed to save sync scope:', e)
  }
}

onMounted(() => {
  syncStore.loadConfig()
})
</script>

<style scoped>
@import './_shared.css';

.sync-card {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  margin-bottom: 1rem;
  overflow: hidden;
}

.sync-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 0.8125rem;
  font-weight: 600;
  font-family: var(--font-ui);
  color: var(--text-primary);
  padding: 0.5rem 0.75rem 0.5rem 1.125rem;
  background: var(--bg-hover);
  border-bottom: 1px solid var(--border-subtle);
}

.sync-card-body {
  padding: 1rem 1.125rem;
}

.sync-card-body.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
  padding: 1.75rem 1.125rem;
}

.empty-text {
  font-size: 0.8125rem;
  color: var(--text-muted);
  margin: 0;
}

/* Repo config */
.repo-info {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  margin-bottom: 0.75rem;
}

.repo-info-row {
  display: flex;
  gap: 0.75rem;
  font-size: 0.8125rem;
}

.repo-label {
  color: var(--text-muted);
  min-width: 4.375rem;
  flex-shrink: 0;
}

.repo-value {
  color: var(--text-primary);
  font-family: var(--font-mono);
  word-break: break-all;
}

.repo-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

/* Sync status */
.sync-status {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  margin-bottom: 0.875rem;
}

.sync-status-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.8125rem;
}

.sync-label {
  color: var(--text-muted);
  min-width: 4.375rem;
  flex-shrink: 0;
}

.sync-value {
  color: var(--text-primary);
}

.sync-tag {
  font-size: 0.6875rem;
  padding: 0.125rem 0.5rem;
  border-radius: var(--radius-sm);
  font-weight: 500;
}

.sync-tag.success {
  background: var(--el-color-success-light-9);
  color: var(--el-color-success-dark-2);
}

.sync-tag.failed {
  background: var(--el-color-danger-light-9);
  color: var(--el-color-danger-dark-2);
}

.sync-error {
  align-items: flex-start;
}

.error-text {
  color: var(--el-color-danger);
}

.sync-actions-row {
  margin-bottom: 0.875rem;
}

.sync-auto-row {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  padding-top: 0.875rem;
  padding-bottom: 0.875rem;
  border-top: 1px solid var(--border-subtle);
}

.sync-auto-label {
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text-primary);
}

.sync-auto-desc {
  font-size: 0.75rem;
  color: var(--text-muted);
  flex: 1;
}

.sync-scope-row {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.5rem;
  padding-top: 0.875rem;
  border-top: 1px solid var(--border-subtle);
}

.sync-scope-row :deep(.el-checkbox-group) {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem 1rem;
}

.sync-scope-header {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
}

.scope-hint {
  font-size: 0.75rem;
  color: var(--text-muted);
  line-height: 1.4;
}
</style>