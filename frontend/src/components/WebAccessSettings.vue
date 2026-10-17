<template>
  <section class="web-access">
    <h2>{{ t('web.settingsTitle') }}</h2>
    <p class="description">{{ t('web.settingsDescription') }}</p>
    <el-alert v-if="error || status?.error" :title="error || status?.error" type="error" :closable="false" role="alert" />
    <el-form label-position="top" @submit.prevent="save">
      <el-form-item :label="t('web.enabled')">
        <el-switch v-model="config.enabled" :disabled="busy || !status" :aria-label="t('web.enabled')" />
      </el-form-item>
      <el-form-item :label="t('web.host')">
        <el-select v-model="config.host" :disabled="busy || !status" :aria-label="t('web.host')">
          <el-option :label="t('web.loopback')" value="127.0.0.1" />
          <el-option :label="t('web.lan')" value="0.0.0.0" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('web.startPort')">
        <el-input-number v-model="config.startPort" :min="1" :max="65535" :step="1" :precision="0" :disabled="busy || !status" :aria-label="t('web.startPort')" />
        <p class="description">{{ t('web.portHint') }}</p>
      </el-form-item>
      <el-form-item :label="t('web.token')">
        <el-input v-model="token" autocomplete="off" :disabled="busy || !status" :placeholder="t(status?.tokenSet ? 'web.tokenKeep' : 'web.tokenEnter')" :aria-label="t('web.token')" />
        <p class="description">{{ t('web.tokenHint') }} {{ t(status?.tokenSet ? 'web.tokenSaved' : 'web.tokenNotSet') }}</p>
        <div class="actions">
          <el-button :disabled="busy || !status" @click="generate">{{ t('web.generateToken') }}</el-button>
          <el-button :disabled="!token" @click="writeClipboard(token)">{{ t('input.copy') }}</el-button>
        </div>
      </el-form-item>
      <div class="actions">
        <el-button type="primary" native-type="submit" :loading="busy" :disabled="!status">{{ t('common.save') }}</el-button>
        <el-button :disabled="busy" @click="refresh">{{ t('web.refreshStatus') }}</el-button>
      </div>
    </el-form>
    <div v-if="status" class="status" role="status">
      <strong>{{ t(status.running ? 'web.running' : 'web.stopped') }}</strong>
      <span v-if="status.running"> · {{ t('web.actualPort') }}: {{ status.actualPort }}</span>
      <div v-for="url in status.running ? status.urls : []" :key="url" class="address">
        <a :href="url" target="_blank" rel="noopener noreferrer" @click.prevent="openExternalLink(url)">{{ url }}</a>
        <el-button size="small" @click="writeClipboard(url)">{{ t('input.copy') }}</el-button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from '../i18n'
import { backendErrorText } from '../utils/backendError'
import { openExternalLink } from '../utils/nativeCapabilities'
import { writeClipboard } from '../composables/useClipboardWrite'
import { getWebAccessStatus, configureWebAccess, generateWebAccessToken } from '../services/webAccess'
import type { WebAccessConfig, WebAccessStatus } from '../services/webAccess'

const { t } = useI18n()
const config = reactive<WebAccessConfig>({ enabled: false, host: '127.0.0.1', startPort: 34116 })
const status = ref<WebAccessStatus>()
const token = ref('')
const error = ref('')
const busy = ref(false)

async function refresh() {
  busy.value = true
  error.value = ''
  try {
    status.value = await getWebAccessStatus()
    Object.assign(config, status.value.config)
  } catch (e) { error.value = backendErrorText(e) }
  finally { busy.value = false }
}

async function generate() {
  busy.value = true
  error.value = ''
  try { token.value = await generateWebAccessToken() }
  catch (e) { error.value = backendErrorText(e) }
  finally { busy.value = false }
}

async function save() {
  if (busy.value) return
  error.value = ''
  if (!Number.isInteger(config.startPort) || config.startPort < 1 || config.startPort > 65535) {
    error.value = t('web.invalidPort')
    return
  }
  if (config.enabled && config.host === '0.0.0.0' && !status.value?.tokenSet && !token.value.trim()) {
    error.value = t('web.tokenRequired')
    return
  }
  busy.value = true
  try {
    status.value = await configureWebAccess({ ...config }, token.value)
    Object.assign(config, status.value.config)
    token.value = ''
  } catch (e) {
    error.value = backendErrorText(e)
    // A bind error can still change actual running state. Wails rejects
    // (status,error), so retrieve the authoritative status separately.
    try { status.value = await getWebAccessStatus() } catch { /* retain the configure error */ }
  } finally { busy.value = false }
}

onMounted(refresh)
</script>

<style scoped>
.web-access { max-width: 42rem; }
h2 { font-size: 1.125rem; margin: 0 0 0.75rem; }
.description { color: var(--text-secondary); font-size: 0.8125rem; width: 100%; }
.el-form { margin-top: 1rem; }
.actions, .address { display: flex; flex-wrap: wrap; align-items: center; gap: 0.5rem; }
.status { margin-top: 1.5rem; padding: 1rem; background: var(--bg-elevated); border: 1px solid var(--border-subtle); border-radius: var(--radius-md); }
.address { margin-top: 0.75rem; }
a { color: var(--accent); overflow-wrap: anywhere; }
</style>
