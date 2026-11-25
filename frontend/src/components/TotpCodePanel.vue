<template>
  <!-- Shared live TOTP readout: used inline in the connection form and inside
       TotpCodeDialog (opened from the connection context menu). -->
  <div class="totp-panel">
    <div class="totp-code-line">
      <span class="totp-code mono">{{ code ? formattedCode : '------' }}</span>
      <el-button
        size="small"
        text
        :disabled="!code"
        :title="t('common.copy')"
        :aria-label="t('common.copy')"
        @click="copyCode"
      >
        <el-icon><Copy :size="15" /></el-icon>
      </el-button>
      <el-button size="small" text :title="t('common.refresh')" :aria-label="t('common.refresh')" @click="refresh">
        <el-icon><RefreshCw :size="15" /></el-icon>
      </el-button>
    </div>
    <div class="totp-progress" role="progressbar" :aria-valuenow="secondsLeft" aria-valuemin="0" :aria-valuemax="period">
      <div class="totp-progress-fill" :style="{ width: progressPercent + '%' }" />
    </div>
    <div v-if="errorKind" class="totp-error field-hint">
      {{ t(errorKind === 'vault' ? 'conn.totpVaultLocked' : 'conn.totpCodeUnavailable') }}
    </div>
    <div v-else class="field-hint">{{ t('conn.totpSecondsLeft', { seconds: secondsLeft }) }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import { Copy, RefreshCw } from '@lucide/vue'
import { useI18n } from '../i18n'
import { useTotpCode } from '../composables/useTotpCode'
import { writeClipboard } from '../composables/useClipboardWrite'
import { msg } from '../services/message'

const props = defineProps<{
  secret: string
  // Inline form instances are only polled while their parent dialog is open.
  active?: boolean
}>()

const { t } = useI18n()
const { code, secondsLeft, period, errorKind, refresh } = useTotpCode(
  toRef(props, 'secret'),
  computed(() => props.active ?? true),
)

// Split the six digits for readability, but always copy the exact code the
// backend would submit to SSH.
const formattedCode = computed(() => {
  const raw = code.value
  return raw.length === 6 ? `${raw.slice(0, 3)} ${raw.slice(3)}` : raw
})
const progressPercent = computed(() => {
  if (!period.value || !code.value) return 0
  return Math.max(0, Math.min(100, (secondsLeft.value / period.value) * 100))
})

async function copyCode() {
  if (!code.value) return
  // writeClipboard already tries every platform fallback; match the rest of
  // the app and stay silent when all of them fail.
  if (await writeClipboard(code.value)) {
    msg.success(t('common.copied'))
  }
}
</script>

<style scoped>
.totp-panel {
  width: 100%;
}

.totp-code-line {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.totp-code {
  font-size: 1.5rem;
  font-weight: 600;
  letter-spacing: 0.125rem;
  line-height: 1.4;
  color: var(--text-primary);
  font-variant-numeric: tabular-nums;
}

.totp-progress {
  width: 100%;
  height: 0.25rem;
  margin: 0.375rem 0;
  border-radius: 0.25rem;
  background: var(--border-subtle);
  overflow: hidden;
}

.totp-progress-fill {
  height: 100%;
  border-radius: inherit;
  background: var(--accent);
  transition: width 0.25s linear;
}

.totp-error {
  color: var(--el-color-danger);
}

.field-hint {
  color: var(--text-muted);
  font-size: 0.75rem;
  line-height: 1.4;
}
</style>
