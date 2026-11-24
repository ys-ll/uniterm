<template>
  <div class="tools-tab-content">
    <div class="tools-inner">
      <el-tabs v-model="activeSubTab" class="tools-tabs">
        <el-tab-pane :label="t('tools.jsonFormat')" name="json" lazy>
          <JsonTool />
        </el-tab-pane>
        <el-tab-pane :label="t('tools.hashEncode')" name="hash" lazy>
          <HashTool />
        </el-tab-pane>
        <el-tab-pane :label="t('tools.url')" name="url" lazy>
          <UrlTool />
        </el-tab-pane>
        <el-tab-pane :label="t('tools.jwt')" name="jwt" lazy>
          <JwtTool />
        </el-tab-pane>
        <el-tab-pane :label="t('tools.timestamp')" name="timestamp" lazy>
          <TimestampTool />
        </el-tab-pane>
        <el-tab-pane :label="t('tools.cron')" name="cron" lazy>
          <CronTool />
        </el-tab-pane>
        <el-tab-pane :label="t('tools.uuid')" name="uuid" lazy>
          <UuidTool />
        </el-tab-pane>
        <el-tab-pane :label="t('tools.regex')" name="regex" lazy>
          <RegexTool />
        </el-tab-pane>
        <el-tab-pane :label="t('tools.yaml')" name="yaml" lazy>
          <YamlTool />
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from '../i18n'
import JsonTool from './tools/JsonTool.vue'
import HashTool from './tools/HashTool.vue'
import UrlTool from './tools/UrlTool.vue'
import JwtTool from './tools/JwtTool.vue'
import TimestampTool from './tools/TimestampTool.vue'
import CronTool from './tools/CronTool.vue'
import UuidTool from './tools/UuidTool.vue'
import RegexTool from './tools/RegexTool.vue'
import YamlTool from './tools/YamlTool.vue'

const { t } = useI18n()
const activeSubTab = ref('json')
</script>

<style scoped>
.tools-tab-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: auto;
  background: var(--bg-base);
}

.tools-inner {
  flex: 1;
  min-width: 0;
  max-width: 960px;
  width: 100%;
  margin: 0 auto;
  padding: 1rem 1.25rem;
  display: flex;
  flex-direction: column;
}

.tools-tabs {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.tools-tabs :deep(.el-tabs__content) {
  flex: 1;
  overflow: auto;
}

/* Common layout + control styles for the per-tool subcomponents. Token
   spans and child-element markup come from runtime/v-html (no scope
   attribute), so everything reachable from child components uses :deep(). */
.tools-tabs :deep(.pane) {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.tools-tabs :deep(.pane-toolbar) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.tools-tabs :deep(.toolbar-actions) {
  display: flex;
  gap: 0.5rem;
}

.tools-tabs :deep(.pane-subtoolbar),
.tools-tabs :deep(.encode-header) {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.tools-tabs :deep(.pane-subtoolbar) {
  justify-content: space-between;
}

.tools-tabs :deep(.pane-label) {
  font-size: 0.8125rem;
  color: var(--text-secondary, #909399);
}

.tools-tabs :deep(.mono-input textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.8125rem;
}

.tools-tabs :deep(.error-line) {
  font-size: 0.8125rem;
  color: var(--el-color-danger);
  word-break: break-word;
}

.tools-tabs :deep(.hash-list) {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.tools-tabs :deep(.hash-row) {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  background: var(--bg-raised, var(--bg-elevated, rgba(144, 147, 153, 0.08)));
}

.tools-tabs :deep(.hash-row.single) {
  justify-content: space-between;
}

.tools-tabs :deep(.hash-name) {
  flex: 0 0 5.5rem;
  font-size: 0.8125rem;
  font-weight: 600;
}

.tools-tabs :deep(.hash-value) {
  flex: 1;
  min-width: 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.8125rem;
  word-break: break-all;
  white-space: pre-wrap;
}

.tools-tabs :deep(.hash-value.is-error) {
  color: var(--el-color-danger);
}

.tools-tabs :deep(.encode-kind) {
  width: 8.5rem;
}

.tools-tabs :deep(.tools-btn) {
  padding: 0.3125rem 0.875rem;
  border: 1px solid var(--border-color, rgba(144, 147, 153, 0.3));
  border-radius: 6px;
  background: transparent;
  color: inherit;
  font-size: 0.8125rem;
  cursor: pointer;
  white-space: nowrap;
}

.tools-tabs :deep(.tools-btn:hover:not(:disabled)) {
  background: rgba(144, 147, 153, 0.12);
}

.tools-tabs :deep(.tools-btn:disabled) {
  opacity: 0.45;
  cursor: not-allowed;
}

.tools-tabs :deep(.tools-btn.small) {
  padding: 0.25rem 0.625rem;
}

.tools-tabs :deep(.json-output) {
  margin: 0;
  padding: 0.625rem 0.75rem;
  border: 1px solid var(--border-color, rgba(144, 147, 153, 0.3));
  border-radius: 6px;
  min-height: 12rem;
  max-height: 32rem;
  overflow: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.8125rem;
  line-height: 1.55;
  white-space: pre;
  user-select: text;
  --json-key: #79b8ff;
  --json-string: #9ece6a;
  --json-number: #d19a66;
  --json-bool: #c678dd;
  --json-null: #56b6c2;
}

[data-theme="light"] .tools-tabs :deep(.json-output) {
  --json-key: #0550ae;
  --json-string: #116329;
  --json-number: #953800;
  --json-bool: #8250df;
  --json-null: #0f7d87;
}

.tools-tabs :deep(.json-output .tok-key) { color: var(--json-key); }
.tools-tabs :deep(.json-output .tok-string) { color: var(--json-string); }
.tools-tabs :deep(.json-output .tok-number) { color: var(--json-number); }
.tools-tabs :deep(.json-output .tok-bool) { color: var(--json-bool); }
.tools-tabs :deep(.json-output .tok-null) { color: var(--json-null); }
</style>
