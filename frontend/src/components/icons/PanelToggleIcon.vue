<template>
  <svg class="panel-toggle-icon" viewBox="0 0 16 16" fill="none" aria-hidden="true">
    <path
      v-if="side === 'left'"
      d="M3.5 2.2h2.6v11.6H3.5a1.3 1.3 0 0 1-1.3-1.3v-9a1.3 1.3 0 0 1 1.3-1.3Z"
      fill="currentColor"
      :opacity="active ? 1 : 0"
    />
    <path
      v-else-if="side === 'bottom'"
      d="M2.2 9.9h11.6v2.6a1.3 1.3 0 0 1-1.3 1.3h-9a1.3 1.3 0 0 1-1.3-1.3V9.9Z"
      fill="currentColor"
      :opacity="active ? 1 : 0"
    />
    <path
      v-else
      d="M9.9 2.2h2.6a1.3 1.3 0 0 1 1.3 1.3v9a1.3 1.3 0 0 1-1.3 1.3H9.9V2.2Z"
      fill="currentColor"
      :opacity="active ? 1 : 0"
    />
    <rect x="1.5" y="1.5" width="13" height="13" rx="2" stroke="currentColor" stroke-width="1.3" />
    <path v-if="side === 'left'" d="M6.1 2v12" stroke="currentColor" stroke-width="1.3" />
    <path v-else-if="side === 'bottom'" d="M2 9.9h12" stroke="currentColor" stroke-width="1.3" />
    <path v-else d="M9.9 2v12" stroke="currentColor" stroke-width="1.3" />
  </svg>
</template>

<script setup lang="ts">
defineProps<{
  side: 'left' | 'bottom' | 'right'
  active: boolean
}>()
</script>

<style scoped>
.panel-toggle-icon {
  width: 0.875rem;
  height: 0.875rem;
  flex: none;
}
</style>
