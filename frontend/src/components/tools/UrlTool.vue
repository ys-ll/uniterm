<template>
  <div class="pane">
    <span class="pane-label">{{ t('tools.input') }}</span>
    <el-input
      v-model="urlInput"
      type="textarea"
      class="mono-input"
      :rows="4"
      resize="vertical"
      :placeholder="t('tools.urlInputPlaceholder')"
    />

    <div class="encode-header">
      <el-radio-group v-model="urlMode" size="small">
        <el-radio-button value="encode">{{ t('tools.urlEncode') }}</el-radio-button>
        <el-radio-button value="decode">{{ t('tools.urlDecode') }}</el-radio-button>
      </el-radio-group>
      <el-radio-group v-model="urlComponent" size="small">
        <el-radio-button value="uri">{{ t('tools.urlFull') }}</el-radio-button>
        <el-radio-button value="component">{{ t('tools.urlComponent') }}</el-radio-button>
      </el-radio-group>
    </div>
    <div class="hash-row single">
      <span class="hash-value">{{ urlResult || '—' }}</span>
      <el-button size="small" :disabled="!urlResult" @click="copy(urlResult)">{{ t('tools.copy') }}</el-button>
    </div>

    <template v-if="urlParts">
      <span class="pane-label">{{ t('tools.urlParse') }}</span>
      <div class="hash-list">
        <div class="hash-row">
          <span class="hash-name">URL</span>
          <span class="hash-value">{{ urlInput }}</span>
        </div>
        <div v-for="part in urlParts" :key="part.name" class="hash-row">
          <span class="hash-name">{{ part.name }}</span>
          <span class="hash-value">{{ part.value }}</span>
        </div>
      </div>
    </template>
    <div v-else-if="parseError" class="error-line">{{ parseError }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from '../../i18n'
import { useCopyText } from '../../composables/useCopyText'

const { t } = useI18n()
const copy = useCopyText()

const urlInput = ref('')
const urlMode = ref<'encode' | 'decode'>('encode')
const urlComponent = ref<'uri' | 'component'>('uri')

const urlResult = computed(() => {
  const value = urlInput.value
  if (!value) return ''
  try {
    if (urlMode.value === 'encode') {
      return urlComponent.value === 'uri' ? encodeURI(value) : encodeURIComponent(value)
    }
    return urlComponent.value === 'uri' ? decodeURI(value) : decodeURIComponent(value)
  } catch {
    return value
  }
})

const PARSE_KEYS: Array<{ name: string; get: (u: URL) => string }> = [
  { name: 'Protocol', get: u => u.protocol },
  { name: 'Host', get: u => u.host },
  { name: 'Port', get: u => u.port || '—' },
  { name: 'Path', get: u => u.pathname },
  { name: 'Hash', get: u => u.hash || '—' },
]

const urlParts = computed(() => {
  const value = urlInput.value.trim()
  if (!value || !/^[a-z][a-z0-9+.-]*:\/\//i.test(value)) return null
  try {
    const u = new URL(value)
    const parts = PARSE_KEYS.map(p => ({ name: p.name, value: p.get(u) }))
    if (u.search) {
      parts.push({ name: `Params (${u.searchParams.size})`, value: u.search })
    }
    return parts
  } catch {
    return null
  }
})

const parseError = computed(() => '')
</script>
