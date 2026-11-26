<template>
  <div class="pane">
    <div class="pane-toolbar">
      <span class="pane-label">{{ t('tools.uuidV4') }}</span>
      <div class="toolbar-actions">
        <el-input-number v-model="uuidCount" :min="1" :max="50" size="small" style="width: 6.5rem" />
        <el-button size="small" @click="generateUuids">{{ t('tools.generate') }}</el-button>
      </div>
    </div>
    <div class="hash-list">
      <div v-for="(id, i) in uuids" :key="`${id}-${i}`" class="hash-row">
        <span class="hash-value">{{ id }}</span>
        <el-button size="small" @click="copy(id)">{{ t('tools.copy') }}</el-button>
      </div>
    </div>
    <div v-if="uuids.length > 1" class="toolbar-actions">
      <el-button size="small" @click="copy(uuids.join('\n'))">{{ t('tools.copyAll') }}</el-button>
    </div>

    <div class="pane-toolbar">
      <span class="pane-label">{{ t('tools.randomString') }}</span>
      <div class="toolbar-actions">
        <el-input-number v-model="randLen" :min="4" :max="256" size="small" style="width: 6.5rem" />
        <el-input-number v-model="randCount" :min="1" :max="50" size="small" style="width: 6.5rem" />
        <el-button size="small" @click="generateRandom">{{ t('tools.generate') }}</el-button>
      </div>
    </div>
    <div class="toolbar-actions">
      <el-checkbox v-model="csUpper" size="small">A-Z</el-checkbox>
      <el-checkbox v-model="csLower" size="small">a-z</el-checkbox>
      <el-checkbox v-model="csDigits" size="small">0-9</el-checkbox>
      <el-checkbox v-model="csSymbols" size="small">{{ t('tools.charsetSymbols') }}</el-checkbox>
    </div>
    <div class="hash-list">
      <div v-for="(s, i) in randomStrings" :key="`${s}-${i}`" class="hash-row">
        <span class="hash-value">{{ s }}</span>
        <el-button size="small" @click="copy(s)">{{ t('tools.copy') }}</el-button>
      </div>
    </div>
    <div v-if="randomStrings.length > 1" class="toolbar-actions">
      <el-button size="small" @click="copy(randomStrings.join('\n'))">{{ t('tools.copyAll') }}</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from '../../i18n'
import { useCopyText } from '../../composables/useCopyText'

const { t } = useI18n()
const copy = useCopyText()

const uuidCount = ref(5)
const uuids = ref<string[]>([])

function generateUuids() {
  const out: string[] = []
  for (let i = 0; i < uuidCount.value; i++) {
    out.push(crypto.randomUUID())
  }
  uuids.value = out
}
generateUuids()

const randLen = ref(16)
const randCount = ref(5)
const csUpper = ref(true)
const csLower = ref(true)
const csDigits = ref(true)
const csSymbols = ref(false)
const randomStrings = ref<string[]>([])

function randomChars(count: number, length: number, charset: string): string[] {
  const out: string[] = []
  for (let i = 0; i < count; i++) {
    const bytes = new Uint32Array(length)
    crypto.getRandomValues(bytes)
    let s = ''
    for (let j = 0; j < length; j++) {
      s += charset[bytes[j] % charset.length]
    }
    out.push(s)
  }
  return out
}

function generateRandom() {
  let charset = ''
  if (csUpper.value) charset += 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'
  if (csLower.value) charset += 'abcdefghijklmnopqrstuvwxyz'
  if (csDigits.value) charset += '0123456789'
  if (csSymbols.value) charset += '!@#$%^&*()-_=+[]{};:,.<>?'
  if (!charset) return
  randomStrings.value = randomChars(randCount.value, randLen.value, charset)
}
generateRandom()
</script>
