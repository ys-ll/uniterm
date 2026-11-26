<template>
  <div class="pane">
    <div class="pane-toolbar">
      <el-input
        v-model="expr"
        class="mono-input"
        style="flex: 1"
        :placeholder="t('tools.cronInputPlaceholder')"
        clearable
      />
    </div>
    <div v-if="error" class="error-line">{{ error }}</div>
    <template v-else-if="expr">
      <span class="pane-label">{{ t('tools.cronDesc') }}</span>
      <div class="hash-row single">
        <span class="hash-value">{{ description }}</span>
      </div>
      <span class="pane-label">{{ t('tools.cronNextRuns') }}</span>
      <div class="hash-list">
        <div v-for="(run, i) in nextRuns" :key="i" class="hash-row">
          <span class="hash-name">#{{ i + 1 }}</span>
          <span class="hash-value">{{ run }}</span>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from '../../i18n'
import { CronNextRuns } from '../../../bindings/github.com/ys-ll/uniterm/app'
import cronstrueEn from 'cronstrue'
import cronstrueZh from 'cronstrue/locales/zh_CN'

const { t, locale } = useI18n()

const expr = ref('')
const nextRuns = ref<string[]>([])

const description = computed(() => {
  const value = expr.value.trim()
  if (!value) return ''
  try {
    const instance = locale.value.toLowerCase().startsWith('zh') ? cronstrueZh : cronstrueEn
    return instance.toString(value)
  } catch {
    return '—'
  }
})

const error = computed(() => {
  if (!expr.value.trim()) return ''
  if (!description.value || description.value === '—') return t('tools.cronInvalid')
  return ''
})

let timer: ReturnType<typeof setTimeout> | null = null
watch(expr, (value) => {
  if (timer) clearTimeout(timer)
  timer = setTimeout(async () => {
    const trimmed = value.trim()
    if (!trimmed || error.value) {
      nextRuns.value = []
      return
    }
    try {
      nextRuns.value = await CronNextRuns(trimmed, 5)
    } catch {
      nextRuns.value = []
    }
  }, 300)
})
</script>
