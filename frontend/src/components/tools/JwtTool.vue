<template>
  <div class="pane">
    <el-input
      v-model="token"
      type="textarea"
      class="mono-input"
      :rows="4"
      resize="vertical"
      :placeholder="t('tools.jwtInputPlaceholder')"
    />
    <div v-if="result.error" class="error-line">{{ result.error }}</div>
    <template v-else-if="result.decoded">
      <div class="hash-list">
        <div class="hash-row">
          <span class="hash-name">{{ t('tools.jwtHeader') }}</span>
          <span class="hash-value" v-html="highlightJSON(result.decoded.header)"></span>
        </div>
        <div class="hash-row">
          <span class="hash-name">{{ t('tools.jwtPayload') }}</span>
          <span class="hash-value" v-html="highlightJSON(result.decoded.payload)"></span>
        </div>
        <div class="hash-row">
          <span class="hash-name">{{ t('tools.jwtSignature') }}</span>
          <span class="hash-value">{{ result.decoded.signature }}</span>
        </div>
        <div class="hash-row single">
          <span class="hash-name">{{ result.decoded.expired ? t('tools.jwtExpired') : t('tools.jwtExpiresAt') }}</span>
          <span class="hash-value" :class="{ 'is-error': result.decoded.expired }">{{ result.decoded.expiresText }}</span>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from '../../i18n'
import { highlightJSON } from '../../utils/jsonHighlight'

const { t } = useI18n()

const token = ref('')

interface DecodedJwt {
  header: string
  payload: string
  signature: string
  expired: boolean
  expiresText: string
}

function b64urlDecode(part: string): string {
  let s = part.replace(/-/g, '+').replace(/_/g, '/')
  while (s.length % 4) s += '='
  const bytes = Uint8Array.from(atob(s), c => c.charCodeAt(0))
  return new TextDecoder().decode(bytes)
}

const result = computed(() => {
  const value = token.value.trim()
  if (!value) return { error: '', decoded: null as DecodedJwt | null }
  const parts = value.split('.')
  try {
    const header = JSON.parse(b64urlDecode(parts[0]))
    const payload = JSON.parse(b64urlDecode(parts[1]))
    const expRaw = (payload as any).exp
    let expired = false
    let expiresText = '—'
    if (typeof expRaw === 'number') {
      const expDate = new Date(expRaw * 1000)
      if (!Number.isNaN(expDate.getTime())) {
        expired = expDate.getTime() < Date.now()
        expiresText = expDate.toLocaleString()
      }
    }
    return {
      error: '',
      decoded: {
        header: JSON.stringify(header, null, 2),
        payload: JSON.stringify(payload, null, 2),
        signature: parts[2] || '—',
        expired,
        expiresText,
      },
    }
  } catch {
    return { error: t('tools.jwtInvalid'), decoded: null as DecodedJwt | null }
  }
})
</script>
