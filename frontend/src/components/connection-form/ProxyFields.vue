<template>
  <el-form-item v-if="showProxy" :label="t('conn.proxy')">
    <div class="inline-add-row">
      <el-select v-model="form.proxyId" :placeholder="t('conn.proxyPlaceholder')" clearable filterable style="flex: 1; min-width: 0">
        <el-option :label="t('conn.proxySystemOption')" value="system" />
        <el-option
          v-for="p in proxyStore.proxies"
          :key="p.id"
          :label="proxyOptionLabel(p)"
          :value="p.id"
          :disabled="p.enabled === false"
        />
      </el-select>
      <el-button class="inline-add-btn" :title="t('conn.newProxy')" @click="emit('openNewProxyDialog')">
        <Plus :size="'0.875rem'" />
      </el-button>
    </div>
  </el-form-item>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Plus } from '@lucide/vue'
import { useI18n } from '../../i18n'
import { useProxyStore } from '../../stores/proxyStore'
import type { ConnectionConfig } from '../../types/session'
import type { Proxy } from '../../types/proxy'

const { t } = useI18n()
const proxyStore = useProxyStore()

const props = defineProps<{
  form: ConnectionConfig
}>()

const emit = defineEmits<{
  openNewProxyDialog: []
}>()

// Proxy only applies to a handful of session types.
const showProxy = computed(() => ['ssh', 'sftp', 'scp', 'monitor'].includes(props.form.type))

// Enable toggle (issue #749): disabled proxies are dimmed and unselectable for
// NEW connections; existing connections referencing one dial directly (backend).
function proxyOptionLabel(p: Proxy): string {
  const base = `${p.name} (${p.kind} ${p.host}:${p.port})`
  return p.enabled === false ? `${base} — ${t('conn.proxyDisabled')}` : base
}
</script>

<style scoped>
.inline-add-row {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  width: 100%;
}
.inline-add-btn {
  flex-shrink: 0;
  width: 2rem;
  height: 2rem;
  padding: 0;
}
</style>