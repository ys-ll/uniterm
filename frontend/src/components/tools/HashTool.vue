<template>
  <div class="pane">
    <span class="pane-label">{{ t('tools.input') }}</span>
    <el-input
      v-model="hashInput"
      type="textarea"
      class="mono-input"
      :rows="5"
      resize="vertical"
      :placeholder="t('tools.hashInputPlaceholder')"
    />

    <span class="pane-label">{{ t('tools.hashResults') }}</span>
    <div class="hash-list">
      <div v-for="row in hashRows" :key="row.key" class="hash-row">
        <span class="hash-name">{{ row.label }}</span>
        <span class="hash-value">{{ row.value }}</span>
        <el-button size="small" :disabled="!row.value" @click="copy(row.value)">{{ t('tools.copy') }}</el-button>
      </div>
    </div>

    <div class="encode-header">
      <span class="pane-label">{{ t('tools.encodeDecode') }}</span>
      <el-radio-group v-model="encodeMode" size="small">
        <el-radio-button value="encode">{{ t('tools.encode') }}</el-radio-button>
        <el-radio-button value="decode">{{ t('tools.decode') }}</el-radio-button>
      </el-radio-group>
      <el-select v-model="encodeKind" size="small" class="encode-kind">
        <el-option label="Base64" value="base64" />
        <el-option label="Hex" value="hex" />
      </el-select>
    </div>
    <div class="hash-row single">
      <span class="hash-value" :class="{ 'is-error': encodeError }">{{ encodeError || encodeResult || '—' }}</span>
      <el-button size="small" :disabled="!encodeResult" @click="copy(encodeResult)">{{ t('tools.copy') }}</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from '../../i18n'
import { HashString, EncodeString } from '../../../bindings/github.com/ys-ll/uniterm/app'
import { useCopyText } from '../../composables/useCopyText'

const { t } = useI18n()
const copy = useCopyText()

const hashInput = ref('')
const hashResults = ref<Record<string, string>>({})

const HASH_ALGOS: Array<{ key: string; label: string }> = [
  { key: 'md5', label: 'MD5' },
  { key: 'sha1', label: 'SHA-1' },
  { key: 'sha256', label: 'SHA-256' },
  { key: 'sha512', label: 'SHA-512' },
  { key: 'crc32', label: 'CRC32' },
]

const hashRows = computed(() => HASH_ALGOS.map(a => ({
  key: a.key,
  label: a.label,
  value: hashResults.value[a.key] ?? '',
})))

const encodeMode = ref<'encode' | 'decode'>('encode')
const encodeKind = ref<'base64' | 'hex'>('base64')
const encodeResult = ref('')
const encodeError = ref('')

let hashTimer: ReturnType<typeof setTimeout> | null = null
watch(hashInput, (value) => {
  if (hashTimer) clearTimeout(hashTimer)
  hashTimer = setTimeout(async () => {
    if (!value) {
      hashResults.value = {}
      return
    }
    try {
      hashResults.value = await HashString(value)
    } catch {
      hashResults.value = {}
    }
  }, 300)
})

watch([hashInput, encodeMode, encodeKind], ([value, mode, kind]) => {
  encodeResult.value = ''
  encodeError.value = ''
  if (!value) return
  EncodeString(value, kind, mode === 'decode')
    .then((out) => { encodeResult.value = out })
    .catch((err: any) => { encodeError.value = String(err?.message ?? err) })
})
</script>
