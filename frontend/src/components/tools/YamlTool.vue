<template>
  <div class="pane">
    <div class="pane-toolbar">
      <el-radio-group v-model="direction" size="small">
        <el-radio-button value="yaml2json">{{ t('tools.yamlToJson') }}</el-radio-button>
        <el-radio-button value="json2yaml">{{ t('tools.jsonToYaml') }}</el-radio-button>
      </el-radio-group>
    </div>
    <el-input
      v-model="input"
      type="textarea"
      class="mono-input"
      :rows="10"
      resize="vertical"
      :placeholder="direction === 'yaml2json' ? t('tools.yamlInputPlaceholder') : t('tools.jsonInputPlaceholder')"
    />
    <div v-if="error" class="error-line">{{ error }}</div>
    <template v-else-if="output">
      <div class="pane-subtoolbar">
        <span>{{ t('tools.output') }}</span>
        <el-button size="small" @click="copy(output)">{{ t('tools.copy') }}</el-button>
      </div>
      <pre class="json-output" v-html="highlightedOutput"></pre>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from '../../i18n'
import { JsonToYaml, YamlToJson } from '../../../bindings/github.com/ys-ll/uniterm/app'
import { useCopyText } from '../../composables/useCopyText'
import { highlightJSON } from '../../utils/jsonHighlight'

const { t } = useI18n()
const copy = useCopyText()

const direction = ref<'yaml2json' | 'json2yaml'>('yaml2json')
const input = ref('')
const output = ref('')
const error = ref('')

const highlightedOutput = computed(() => highlightJSON(direction.value === 'yaml2json' ? output.value : ''))

let timer: ReturnType<typeof setTimeout> | null = null
watch([input, direction], ([value, dir]) => {
  if (timer) clearTimeout(timer)
  timer = setTimeout(async () => {
    if (!value.trim()) {
      output.value = ''
      error.value = ''
      return
    }
    try {
      output.value = dir === 'yaml2json' ? await YamlToJson(value) : await JsonToYaml(value)
      error.value = ''
    } catch (err: any) {
      output.value = ''
      error.value = String(err?.message ?? err)
    }
  }, 300)
})
</script>
