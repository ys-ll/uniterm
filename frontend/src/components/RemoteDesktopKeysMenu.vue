<template>
  <el-dropdown
    trigger="click"
    :teleported="teleported"
    :disabled="disabled"
    @command="onCommand"
    @visible-change="onVisibleChange"
  >
    <button
      v-if="compact"
      class="rdp-keys-trigger-icon"
      :aria-label="t('remoteDesktop.sendKeys')"
      :title="t('remoteDesktop.sendKeys')"
      :disabled="disabled"
    >
      <Keyboard :size="16" />
    </button>
    <button v-else class="rdp-keys-trigger-text" :disabled="disabled">
      {{ t('remoteDesktop.sendKeys') }}
    </button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item command="cad">Ctrl+Alt+Del</el-dropdown-item>
        <el-dropdown-item command="ctrlShiftEsc">Ctrl+Shift+Esc</el-dropdown-item>
        <el-dropdown-item command="win">Win</el-dropdown-item>
        <el-dropdown-item command="winL">Win+L</el-dropdown-item>
        <el-dropdown-item command="altTab">Alt+Tab</el-dropdown-item>
        <el-dropdown-item command="altF4">Alt+F4</el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup lang="ts">
import { Keyboard } from '@lucide/vue'
import { useI18n } from '../i18n'

const { t } = useI18n()

// Shortcut menu shared by the remote-desktop status bar and fullscreen
// toolbar. Carries only combos the local OS reserves before they can reach
// the webview (Ctrl+Alt+Del, Win+L) plus a few conveniences; ordinary keys
// pass through the keyboard capture already.
// `teleported` must stay false inside the fullscreen toolbar: a body portal
// is invisible there (same constraint as the toolbar selects).
withDefaults(defineProps<{
  // Icon-only trigger matching the fullscreen toolbar buttons; the default
  // text trigger matches the status bar's fullscreen button.
  compact?: boolean
  disabled?: boolean
  teleported?: boolean
}>(), { compact: false, disabled: false, teleported: true })

const emit = defineEmits<{
  send: [id: string]
  closed: []
}>()

function onCommand(id: string | number | object) {
  emit('send', String(id))
}

function onVisibleChange(visible: boolean) {
  if (!visible) emit('closed')
}
</script>

<style scoped>
/* Text trigger mirrors the status bar's fullscreen button. */
.rdp-keys-trigger-text {
  border: 1px solid var(--border-default);
  background: transparent;
  color: var(--text-muted);
  font-size: 0.75rem;
  padding: 1px 0.625rem;
  border-radius: var(--radius-sm);
}
.rdp-keys-trigger-text:hover:not(:disabled) {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.rdp-keys-trigger-text:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
/* Icon trigger mirrors the fullscreen toolbar buttons. */
.rdp-keys-trigger-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  width: 1.875rem;
  height: 1.875rem;
  padding: 0;
  border: 0;
  border-radius: 0.25rem;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  outline: none;
}
.rdp-keys-trigger-icon:hover:not(:disabled),
.rdp-keys-trigger-icon:focus-visible {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.rdp-keys-trigger-icon:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
</style>
