<template>
  <el-dialog append-to-body
    :model-value="visible"
    @update:model-value="() => emit('close')"
    :title="t('mcp.setupTitle')"
    width="46rem"
    top="6vh"
    :close-on-click-modal="false"
  >
    <div class="mcp-setup">
      <el-alert type="warning" show-icon :closable="false" class="mcp-setup-once">
        {{ t('mcp.tokenOnceWarning') }}
      </el-alert>

      <div class="mcp-setup-token">
        <span class="mcp-setup-label">{{ t('mcp.bearerToken') }}</span>
        <div class="mcp-setup-token-row">
          <code>{{ token }}</code>
          <el-tooltip :content="copied === 'token' ? t('common.copied') : t('common.copy')" placement="top">
            <el-button link :type="copied === 'token' ? ('success' as const) : ('default' as const)" @click="copy('token', token)">
              <el-icon><Check v-if="copied === 'token'" /><Copy v-else :size="'0.875rem'" /></el-icon>
            </el-button>
          </el-tooltip>
        </div>
      </div>

      <el-tabs v-model="activeClient">
        <el-tab-pane v-for="c in clients" :key="c.id" :name="c.id" :label="c.label">
          <p v-if="c.hint" class="mcp-setup-hint">{{ c.hint }}</p>
          <div class="mcp-setup-cmd">
            <pre>{{ c.config }}</pre>
            <el-tooltip :content="copied === c.id ? t('common.copied') : t('common.copy')" placement="top">
              <el-button link class="mcp-setup-copy" :type="copied === c.id ? ('success' as const) : ('default' as const)" @click="copy(c.id, c.config)">
                <el-icon><Check v-if="copied === c.id" /><Copy v-else :size="'0.875rem'" /></el-icon>
              </el-button>
            </el-tooltip>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>

    <template #footer>
      <el-button type="primary" @click="emit('close')">{{ t('common.done') }}</el-button>
    </template>
  </el-dialog>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from '../i18n'
import { writeClipboard } from '../composables/useClipboardWrite'
import { Check, Copy } from '@lucide/vue'

const { t } = useI18n()

const props = defineProps<{
  visible: boolean
  token: string
  port: number
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const activeClient = ref('claude')
const copied = ref('')

watch(() => props.visible, v => {
  if (v) {
    activeClient.value = 'claude'
    copied.value = ''
  }
})

const url = computed(() => `http://127.0.0.1:${props.port}/mcp`)

const clients = computed(() => [
  {
    id: 'claude',
    label: 'Claude Code',
    hint: t('mcp.claudeHint'),
    config: `claude mcp add --transport http uniterm ${url.value} \\\n  --header "Authorization: Bearer ${props.token}"`,
  },
  {
    id: 'codex',
    label: 'Codex',
    hint: t('mcp.codexHint'),
    config: `[mcp_servers.uniterm]\nurl = "${url.value}"\nhttp_headers = { "Authorization" = "Bearer ${props.token}" }`,
  },
  {
    id: 'zcode',
    label: 'ZCode',
    hint: t('mcp.zcodeHint'),
    config: JSON.stringify({
      mcp: {
        servers: {
          uniterm: {
            type: 'http',
            url: url.value,
            headers: { Authorization: `Bearer ${props.token}` },
          },
        },
      },
    }, null, 2),
  },
  {
    id: 'trae',
    label: 'Trae',
    hint: t('mcp.traeHint'),
    config: JSON.stringify({
      mcpServers: {
        uniterm: {
          url: url.value,
          headers: { Authorization: `Bearer ${props.token}` },
        },
      },
    }, null, 2),
  },
  {
    id: 'workbuddy',
    label: 'WorkBuddy',
    hint: t('mcp.workbuddyHint'),
    config: JSON.stringify({
      mcpServers: {
        uniterm: {
          type: 'streamableHttp',
          url: url.value,
          headers: { Authorization: `Bearer ${props.token}` },
        },
      },
    }, null, 2),
  },
  {
    id: 'kimi',
    label: 'Kimi',
    hint: t('mcp.kimiHint'),
    config: `kimi mcp add --transport http uniterm ${url.value} \\\n  --header "Authorization: Bearer ${props.token}"`,
  },
  {
    id: 'gemini',
    label: 'Gemini',
    hint: t('mcp.geminiHint'),
    config: JSON.stringify({
      mcpServers: {
        uniterm: {
          type: 'http',
          url: url.value,
          headers: { Authorization: `Bearer ${props.token}` },
        },
      },
    }, null, 2),
  },
])

async function copy(key: string, text: string) {
  await writeClipboard(text)
  copied.value = key
  setTimeout(() => { if (copied.value === key) copied.value = '' }, 2000)
}
</script>

<style scoped>
.mcp-setup {
  display: flex;
  flex-direction: column;
  gap: 0.875rem;
}
/* Seven client tabs: let the tab strip wrap to two rows instead of
   scrolling/overflow, and drop the bottom border so the wrapped second
   row doesn't look detached. */
.mcp-setup :deep(.el-tabs__nav-wrap) {
  overflow: visible;
}
.mcp-setup :deep(.el-tabs__nav-wrap::after) {
  display: none;
}
.mcp-setup :deep(.el-tabs__nav-scroll) {
  overflow: visible;
}
.mcp-setup :deep(.el-tabs__nav) {
  flex-wrap: wrap;
  gap: 0.25rem 0.75rem;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.mcp-setup :deep(.el-tabs__item) {
  padding: 0 0.5rem;
}
.mcp-setup-once {
  border-radius: 6px;
}
.mcp-setup-token {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.mcp-setup-label {
  font-size: 0.75rem;
  color: var(--text-muted);
}
.mcp-setup-token-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.mcp-setup-token-row code {
  flex: 1;
  font-family: var(--font-mono);
  font-size: 0.75rem;
  padding: 0.4rem 0.6rem;
  background: var(--bg-surface);
  color: var(--text-primary);
  border: 1px solid var(--border-color, var(--el-border-color));
  border-radius: 4px;
  white-space: nowrap;
  overflow-x: auto;
  user-select: all;
}
.mcp-setup-hint {
  font-size: 0.75rem;
  color: var(--text-muted);
  margin: 0 0 0.5rem;
}
.mcp-setup-cmd {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: 0.375rem;
}
.mcp-setup-cmd pre {
  flex: 1;
  margin: 0;
  padding: 0.625rem 0.75rem;
  font-family: var(--font-mono);
  font-size: 0.72rem;
  line-height: 1.5;
  background: var(--bg-surface);
  color: var(--text-primary);
  border: 1px solid var(--border-color, var(--el-border-color));
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  user-select: all;
}
.mcp-setup-copy {
  flex-shrink: 0;
}
</style>
