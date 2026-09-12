<template>
  <el-dialog
    :model-value="updateCheck.updateDialogVisible"
    :title="t('settings.updateDialogTitle')"
    width="540px"
    :close-on-click-modal="false"
    :close-on-press-escape="!locked"
    :show-close="!locked"
    @update:model-value="(v: boolean) => { if (!v) updateCheck.closeUpdateDialog() }"
  >
    <div class="update-dialog">
      <div class="update-dialog-version">
        {{ t('settings.version') }} {{ updateCheck.updateInfo?.current || '...' }} → {{ updateCheck.updateInfo?.latest || '...' }}
      </div>

      <div v-if="updateCheck.channel === 'package'" class="update-dialog-hint">
        {{ t('settings.updatePackageManager') }}
      </div>

      <!-- Release notes are markdown; render as sanitized HTML -->
      <div v-if="changelogHtml" class="update-dialog-changelog" v-html="changelogHtml"></div>

      <div v-if="updateCheck.updatePhase === 'error'" class="update-dialog-error">
        {{ t('settings.updateFailed') }}: {{ updateCheck.updateError }}
      </div>

      <div v-if="updateCheck.updatePhase === 'downloading' || updateCheck.updatePhase === 'verifying'" class="update-dialog-progress">
        <el-progress :percentage="updateCheck.downloadProgress.percent" :stroke-width="10" :indeterminate="updateCheck.downloadProgress.total <= 0" />
        <div class="update-dialog-progress-label">
          {{ updateCheck.updatePhase === 'verifying' ? t('settings.updateVerifying') : t('settings.updateDownloading') }}
          <span v-if="updateCheck.updatePhase === 'downloading' && updateCheck.downloadProgress.received > 0" class="update-dialog-progress-size">
            ({{ fmtSize(updateCheck.downloadProgress.received) }}<template v-if="updateCheck.downloadProgress.total > 0"> / {{ fmtSize(updateCheck.downloadProgress.total) }}</template>)
          </span>
        </div>
      </div>

      <div v-if="updateCheck.updatePhase === 'applying'" class="update-dialog-progress-label">
        {{ t('settings.updateApplying') }}
      </div>
      <div v-if="updateCheck.updatePhase === 'restarting'" class="update-dialog-progress-label">
        {{ t('settings.updateRestarting') }}
      </div>
    </div>
    <template #footer>
      <el-button
        v-if="updateCheck.updatePhase === 'idle' || updateCheck.updatePhase === 'error'"
        :disabled="!releaseUrl"
        @click="openRelease"
      >
        {{ t('settings.openRelease') }}
      </el-button>
      <el-button v-if="updateCheck.updatePhase === 'idle' || updateCheck.updatePhase === 'error'" @click="updateCheck.closeUpdateDialog()">
        {{ t('settings.updateLater') }}
      </el-button>
      <el-button
        v-if="(updateCheck.updatePhase === 'idle' || updateCheck.updatePhase === 'error') && updateCheck.channel !== 'package'"
        type="primary"
        @click="updateCheck.startUpdate()"
      >
        {{ t('settings.updateInstall') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Browser } from '@wailsio/runtime'
import { useUpdateCheck } from '../composables/useUpdateCheck'
import { useI18n, locale } from '../i18n'
import { renderMarkdownHtml, sanitizeRenderedHtml } from '../utils/markdown'

const { t } = useI18n()
const updateCheck = useUpdateCheck()

// Cannot dismiss the dialog while the binary is being replaced.
const locked = computed(() =>
  updateCheck.updatePhase === 'applying' || updateCheck.updatePhase === 'restarting'
)

// Release notes have a fixed layout: English section first, then the Chinese
// one under the "### 更新内容" heading. zh locales show the Chinese section,
// other locales the English part; if the marker is missing (unexpected
// format), fall back to the full body.
const changelogHtml = computed(() => {
  const body = updateCheck.updateInfo?.changelog || ''
  if (!body) return ''
  const zhIdx = body.search(/^#{2,3}\s+更新内容\s*$/m)
  if (zhIdx < 0) {
    return sanitizeRenderedHtml(renderMarkdownHtml(body))
  }
  const isZh = locale.value === 'zh-CN' || locale.value === 'zh-TW'
  if (!isZh) {
    return sanitizeRenderedHtml(renderMarkdownHtml(body.slice(0, zhIdx)))
  }
  // Keep the leading version heading (e.g. "## v1.9.2") above the Chinese section.
  const header = body.match(/^#{1,6}\s+[^\n]*\n?/)
  return sanitizeRenderedHtml(renderMarkdownHtml((header ? header[0] : '') + body.slice(zhIdx)))
})

const releaseUrl = computed(() => updateCheck.updateInfo?.releaseUrl || '')

function openRelease() {
  if (releaseUrl.value) Browser.OpenURL(releaseUrl.value)
}

function fmtSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB']
  let v = bytes
  let i = -1
  do {
    v /= 1024
    i++
  } while (v >= 1024 && i < units.length - 1)
  return `${v.toFixed(1)} ${units[i]}`
}
</script>

<style scoped>
.update-dialog-version {
  font-size: 14px;
  font-weight: 600;
  font-family: var(--font-ui);
  margin-bottom: 10px;
}
.update-dialog-hint {
  font-size: 13px;
  color: var(--text-muted, #8a8a8a);
  margin-bottom: 10px;
}
.update-dialog-changelog {
  max-height: 320px;
  overflow-y: auto;
  background: var(--bg-overlay);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 10px 12px;
  margin-bottom: 12px;
  font-size: 12.5px;
  line-height: 1.6;
  font-family: var(--font-ui);
  color: var(--text-primary);
}
.update-dialog-changelog :deep(h2) {
  font-size: 14px;
  margin: 0 0 8px;
}
.update-dialog-changelog :deep(h3) {
  font-size: 13px;
  margin: 12px 0 6px;
}
.update-dialog-changelog :deep(p) {
  margin: 6px 0;
}
.update-dialog-changelog :deep(ul) {
  margin: 6px 0;
  padding-left: 18px;
}
.update-dialog-changelog :deep(li) {
  margin: 3px 0;
}
.update-dialog-changelog :deep(a) {
  color: var(--accent);
  text-decoration: none;
}
.update-dialog-changelog :deep(a:hover) {
  text-decoration: underline;
}
.update-dialog-changelog :deep(code) {
  background: var(--bg-overlay);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 0 4px;
  font-family: var(--font-mono);
  font-size: 11.5px;
}
.update-dialog-changelog :deep(hr) {
  border: none;
  border-top: 1px solid var(--border-subtle);
  margin: 10px 0;
}
.update-dialog-error {
  color: #f56c6c;
  font-size: 13px;
  margin-bottom: 10px;
  word-break: break-word;
}
.update-dialog-progress-label {
  font-size: 13px;
  margin-top: 8px;
  color: var(--text-muted, #8a8a8a);
}
.update-dialog-progress-size {
  font-family: var(--font-mono);
}
</style>
