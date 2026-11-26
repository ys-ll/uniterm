<template>
  <div class="pane">
    <div class="pane-toolbar">
      <el-input
        v-model="timeInput"
        class="mono-input"
        style="flex: 1"
        :placeholder="t('tools.timestampInputPlaceholder')"
        clearable
      />
      <el-button size="small" @click="fillNow">{{ t('tools.now') }}</el-button>
    </div>
    <div v-if="error" class="error-line">{{ error }}</div>
    <template v-else-if="rows">
      <div class="hash-list">
        <div v-for="row in rows" :key="row.name" class="hash-row">
          <span class="hash-name">{{ row.name }}</span>
          <span class="hash-value">{{ row.value }}</span>
          <el-button size="small" @click="copy(row.value)">{{ t('tools.copy') }}</el-button>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from '../../i18n'
import { useCopyText } from '../../composables/useCopyText'

const { t } = useI18n()
const copy = useCopyText()

const timeInput = ref('')

function fillNow() {
  timeInput.value = String(Math.floor(Date.now() / 1000))
}

const parsedDate = computed<Date | null>(() => {
  const s = timeInput.value.trim()
  if (!s) return null
  // Bare digits: auto-detect seconds (10) vs milliseconds (13).
  if (/^\d{10}$/.test(s) || /^\d{9}$/.test(s)) return new Date(Number(s) * 1000)
  if (/^\d{13}$/.test(s) || /^\d{12}$/.test(s)) return new Date(Number(s))
  if (/^\d{1,8}$/.test(s)) return new Date(Number(s) * 1000)
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? null : d
})

const error = computed(() => {
  if (!timeInput.value.trim()) return ''
  return parsedDate.value ? '' : t('tools.invalidDate')
})

const rows = computed(() => {
  const d = parsedDate.value
  if (!d) return null
  const pad = (n: number) => String(n).padStart(2, '0')
  const local = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  const utc = `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}`
  return [
    { name: t('tools.localTime'), value: local },
    { name: t('tools.utcTime'), value: utc },
    { name: 'ISO 8601', value: d.toISOString() },
    { name: t('tools.unixSeconds'), value: String(Math.floor(d.getTime() / 1000)) },
    { name: t('tools.unixMillis'), value: String(d.getTime()) },
  ]
})
</script>
