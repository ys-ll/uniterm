<template>
  <div class="window-controls">
      <button class="wc-btn win minimise" @click="$emit('minimise')" :aria-label="t('window.minimize')">
        <svg viewBox="0 0 12 12" width="14" height="14"><path d="M1 5.5h10v1H1z"/></svg>
      </button>
      <button class="wc-btn win maximise" @click="$emit('maximise')" :aria-label="t('window.maximize')">
        <svg v-if="isMaximised" viewBox="0 0 12 12" width="14" height="14">
          <defs>
            <mask :id="restoreMaskId">
              <rect width="12" height="12" fill="white"/>
              <rect x="1" y="3.5" width="6.5" height="6.5" fill="black"/>
            </mask>
          </defs>
          <!-- 后方大矩形（右上），被前方遮挡重叠区域 -->
          <rect x="3.5" y="1" width="6.5" height="6.5" fill="none" stroke="currentColor" stroke-width="1" :mask="`url(#${restoreMaskId})`"/>
          <!-- 前方小矩形（左下），完整显示 -->
          <rect x="1" y="3.5" width="6.5" height="6.5" fill="none" stroke="currentColor" stroke-width="1"/>
        </svg>
        <svg v-else viewBox="0 0 12 12" width="14" height="14"><rect x="1.5" y="1.5" width="9" height="9" fill="none" stroke="currentColor" stroke-width="1"/></svg>
      </button>
      <button class="wc-btn win close" @click="$emit('close')" :aria-label="t('window.close')">
        <svg viewBox="0 0 12 12" width="14" height="14"><path d="M2 2l8 8M10 2L2 10" stroke="currentColor" stroke-width="1.2"/></svg>
      </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from '../i18n'

const { t } = useI18n()

defineProps<{
  isMaximised: boolean
}>()

defineEmits(['minimise', 'maximise', 'close'])

const restoreMaskId = `rm-${Math.random().toString(36).slice(2, 9)}`
</script>

<style scoped>
.window-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  --wails-draggable: no-drag;
}

.window-controls {
  gap: 2px;
}


/* Windows/Linux buttons — match header-btn style */
.wc-btn.win {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 5px 8px;
  height: 28px;
  border: none;
  border-radius: var(--radius-sm);
  cursor: pointer;
  background: transparent;
  color: var(--text-secondary);
  transition: all 0.15s ease;
}

.wc-btn.win:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.wc-btn.win.close:hover {
  background: #e81123;
  color: var(--on-accent);
}

.wc-btn.win.close:active {
  background: #f1707a;
}

.wc-btn.win svg {
  fill: currentColor;
}
</style>
