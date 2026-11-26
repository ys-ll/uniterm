<template>
  <div class="pane">
    <div class="pane-toolbar">
      <span class="pane-label">{{ t('tools.regexPattern') }}</span>
      <div class="toolbar-actions">
        <el-checkbox v-for="f in FLAG_DEFS" :key="f" v-model="flags[f]" size="small">{{ f }}</el-checkbox>
      </div>
    </div>
    <el-input v-model="pattern" class="mono-input" :placeholder="t('tools.regexPatternPlaceholder')" clearable />
    <div v-if="error" class="error-line">{{ error }}</div>

    <span class="pane-label">{{ t('tools.regexTestText') }}</span>
    <el-input
      v-model="testText"
      type="textarea"
      class="mono-input"
      :rows="5"
      resize="vertical"
      :placeholder="t('tools.regexTestPlaceholder')"
    />

    <template v-if="pattern">
      <span class="pane-label">{{ t('tools.regexPreview') }}</span>
      <pre class="json-output regex-preview" v-html="highlightedText"></pre>

      <span class="pane-label">{{ t('tools.regexMatches') }}</span>
      <div v-if="matches.length === 0" class="hash-row single">
        <span class="hash-value">{{ t('tools.regexNoMatch') }}</span>
      </div>
      <div v-else class="hash-list">
        <div v-for="(m, i) in matches" :key="i" class="hash-row">
          <span class="hash-name">#{{ i + 1 }} @{{ m.index }}</span>
          <span class="hash-value">{{ m.text }}<template v-if="m.groups.length"> · {{ t('tools.regexGroup') }}: {{ m.groups.join(', ') }}</template></span>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from '../../i18n'
import { escapeHtml } from '../../utils/jsonHighlight'

const { t } = useI18n()

const FLAG_DEFS = ['g', 'i', 'm', 's', 'u'] as const
const flags = ref<Record<(typeof FLAG_DEFS)[number], boolean>>({
  g: true, i: false, m: false, s: false, u: false,
})

const pattern = ref('')
const testText = ref('')

const regex = computed<RegExp | null>(() => {
  const src = pattern.value
  if (!src) return null
  try {
    let flagStr = ''
    for (const f of FLAG_DEFS) if (flags.value[f]) flagStr += f
    return new RegExp(src, flagStr)
  } catch {
    return null
  }
})

const error = computed(() => {
  if (!pattern.value) return ''
  return regex.value ? '' : t('tools.regexInvalid')
})

const MAX_MATCHES = 500

interface MatchInfo {
  text: string
  index: number
  groups: string[]
}

const matches = computed<MatchInfo[]>(() => {
  const re = regex.value
  const text = testText.value
  if (!re || !text) return []
  const out: MatchInfo[] = []
  let m: RegExpExecArray | null
  const globalRe = re.global ? re : new RegExp(re.source, re.flags + 'g')
  while ((m = globalRe.exec(text)) !== null) {
    if (m[0] === '') { globalRe.lastIndex++; continue }
    const groups: string[] = []
    for (let g = 1; g < m.length; g++) groups.push(m[g] ?? '—')
    out.push({ text: m[0], index: m.index, groups })
    if (out.length >= MAX_MATCHES) break
  }
  return out
})

const highlightedText = computed(() => {
  const re = regex.value
  const text = testText.value
  if (!re || !text) return escapeHtml(text)
  const globalRe = re.global ? re : new RegExp(re.source, re.flags + 'g')
  let out = ''
  let last = 0
  let count = 0
  let m: RegExpExecArray | null
  while ((m = globalRe.exec(text)) !== null && count < MAX_MATCHES) {
    if (m[0] === '') { globalRe.lastIndex++; continue }
    out += escapeHtml(text.slice(last, m.index))
    out += `<mark class="rx-match">${escapeHtml(m[0])}</mark>`
    last = m.index + m[0].length
    count++
  }
  out += escapeHtml(text.slice(last))
  return out
})
</script>

<style scoped>
.regex-preview :deep(.rx-match) {
  background: rgba(64, 158, 255, 0.35);
  color: inherit;
  border-radius: 3px;
  padding: 0 1px;
}
</style>
