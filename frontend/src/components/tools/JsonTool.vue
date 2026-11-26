<template>
  <div class="pane">
    <div class="pane-toolbar">
      <el-radio-group v-model="jsonIndent" size="small">
        <el-radio-button value="2">2 {{ t('tools.spaces') }}</el-radio-button>
        <el-radio-button value="4">4 {{ t('tools.spaces') }}</el-radio-button>
        <el-radio-button value="tab">Tab</el-radio-button>
        <el-radio-button value="min">{{ t('tools.minify') }}</el-radio-button>
      </el-radio-group>
      <div class="toolbar-actions">
        <el-button size="small" :disabled="!jsonInput.trim()" @click="onFormatJSON">{{ t('tools.format') }}</el-button>
        <el-button size="small" :disabled="!jsonInput.trim()" @click="onPasteAndFormat">{{ t('tools.pasteAndFormat') }}</el-button>
      </div>
    </div>
    <el-input
      v-model="jsonInput"
      type="textarea"
      class="mono-input"
      :rows="10"
      resize="vertical"
      :placeholder="t('tools.jsonInputPlaceholder')"
    />
    <div v-if="jsonError" class="error-line">{{ jsonError }}</div>
    <template v-else-if="jsonOutput">
      <div class="pane-subtoolbar">
        <span>{{ t('tools.output') }}</span>
        <el-button size="small" @click="copy(jsonOutput)">{{ t('tools.copy') }}</el-button>
      </div>
      <pre class="json-output" v-html="highlightedJson"></pre>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from '../../i18n'
import { FormatJSON } from '../../../bindings/github.com/ys-ll/uniterm/app'
import { useCopyText } from '../../composables/useCopyText'
import { highlightJSON } from '../../utils/jsonHighlight'

const { t } = useI18n()
const copy = useCopyText()

const jsonInput = ref('')
const jsonIndent = ref<'2' | '4' | 'tab' | 'min'>('2')
const jsonOutput = ref('')
const jsonError = ref('')

const highlightedJson = computed(() => highlightJSON(jsonOutput.value))

async function onFormatJSON() {
  const input = jsonInput.value
  if (!input.trim()) return
  try {
    jsonOutput.value = await FormatJSON(input, jsonIndent.value)
    jsonError.value = ''
  } catch (err: any) {
    jsonOutput.value = ''
    jsonError.value = String(err?.message ?? err)
  }
}

async function onPasteAndFormat() {
  const { readClipboardText } = await import('../../composables/useClipboardWrite')
  const text = await readClipboardText()
  if (!text.trim()) return
  jsonInput.value = text
  await onFormatJSON()
}
</script>
