<template>
  <!-- Windows/Linux: match header-btn style. macOS uses the native AppKit
       traffic lights instead (re-enabled by the backend; see AppHeader.vue). -->
  <div class="window-controls">
      <button class="wc-btn win minimise" @click="$emit('minimise')" :aria-label="t('window.minimize')">
        <WinMinimise />
      </button>
      <button class="wc-btn win maximise" @click="$emit('maximise')" :aria-label="t('window.maximize')">
        <WinRestore v-if="isMaximised" />
        <WinMaximise v-else />
      </button>
      <button class="wc-btn win close" @click="$emit('close')" :aria-label="t('window.close')">
        <!-- Same glyph as the dialog close keys (WinClose / --win-close-x) -->
        <WinClose />
      </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from '../i18n'
import WinClose from './icons/WinClose.vue'
import WinMinimise from './icons/WinMinimise.vue'
import WinMaximise from './icons/WinMaximise.vue'
import WinRestore from './icons/WinRestore.vue'

const { t } = useI18n()

defineProps<{
  isMaximised: boolean
}>()

defineEmits(['minimise', 'maximise', 'close'])
</script>

<style scoped>
.window-controls {
  display: flex;
  align-items: center;
  gap: 0.125rem;
  --wails-draggable: no-drag;
}

/* ── Buttons — match header-btn style ── */
.wc-btn.win {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0.3125rem 0.5rem;
  height: 1.75rem;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-secondary);
  transition: all 0.15s ease;
}

.wc-btn.win:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.wc-btn.win svg {
  width: 0.875rem;
  height: 0.875rem;
}

.wc-btn.win.close:hover {
  background: #e81123;
  color: var(--on-accent);
}

.wc-btn.win.close:active {
  background: #f1707a;
}
</style>
