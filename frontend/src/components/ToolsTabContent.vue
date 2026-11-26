<template>
  <div ref="toolsTabRef" class="tools-tab-content">
    <div class="tools-sidebar" :class="{ collapsed: sidebarCollapsed, 'no-anim': !sidebarAnimReady }">
      <button
        class="sidebar-collapse-btn"
        :title="t('settings.toggleSidebar')"
        @click="sidebarCollapsed = !sidebarCollapsed"
      >
        <el-icon><component :is="sidebarCollapsed ? ChevronRight : ChevronLeft" /></el-icon>
      </button>
      <div
        v-for="tool in TOOLS"
        :key="tool.key"
        class="tools-category"
        :class="{ active: activeSubTab === tool.key }"
        @click="activeSubTab = tool.key"
      >
        <el-icon class="category-icon"><component :is="tool.icon" /></el-icon>
        <span v-if="!sidebarCollapsed" class="category-label">{{ t(tool.labelKey) }}</span>
      </div>
    </div>

    <div class="tools-panel">
      <div class="tools-inner">
        <KeepAlive>
          <component :is="activeTool.component" />
        </KeepAlive>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onActivated, onMounted, onUnmounted, ref, type Component } from 'vue'
import { useI18n } from '../i18n'
import {
  CalendarClock, ChevronLeft, ChevronRight, Clock, Dices, FileBraces, FileCode,
  Hash, KeySquare, Link, Regex,
} from '@lucide/vue'
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

interface ToolEntry {
  key: string
  labelKey: string
  icon: Component
  component: Component
}

const TOOLS: ToolEntry[] = [
  { key: 'json', labelKey: 'tools.jsonFormat', icon: FileBraces, component: JsonTool },
  { key: 'hash', labelKey: 'tools.hashEncode', icon: Hash, component: HashTool },
  { key: 'url', labelKey: 'tools.url', icon: Link, component: UrlTool },
  { key: 'jwt', labelKey: 'tools.jwt', icon: KeySquare, component: JwtTool },
  { key: 'timestamp', labelKey: 'tools.timestamp', icon: Clock, component: TimestampTool },
  { key: 'cron', labelKey: 'tools.cron', icon: CalendarClock, component: CronTool },
  { key: 'uuid', labelKey: 'tools.uuid', icon: Dices, component: UuidTool },
  { key: 'regex', labelKey: 'tools.regex', icon: Regex, component: RegexTool },
  { key: 'yaml', labelKey: 'tools.yaml', icon: FileCode, component: YamlTool },
]

const activeSubTab = ref('json')
const activeTool = computed(() => TOOLS.find(tl => tl.key === activeSubTab.value) ?? TOOLS[0])

// Auto-collapse mirrors the settings page: below the width where the expanded
// sidebar (~13rem) plus a usable tool pane (~31rem) fit, collapse to icons.
const toolsTabRef = ref<HTMLElement>()
const sidebarCollapsed = ref(false)
const sidebarAnimReady = ref(false)

function updateSidebarCollapse() {
  const el = toolsTabRef.value
  if (!el) return
  const rem = parseFloat(getComputedStyle(document.documentElement).fontSize)
  sidebarCollapsed.value = el.clientWidth < 44 * rem
}

let sidebarResizeObserver: ResizeObserver | null = null

onMounted(() => {
  updateSidebarCollapse()
  requestAnimationFrame(() => {
    requestAnimationFrame(() => sidebarAnimReady.value = true)
  })
  if (toolsTabRef.value) {
    sidebarResizeObserver = new ResizeObserver(updateSidebarCollapse)
    sidebarResizeObserver.observe(toolsTabRef.value)
  }
})

onUnmounted(() => {
  sidebarResizeObserver?.disconnect()
  sidebarResizeObserver = null
})

// Re-activation re-attaches the DOM and can flip the collapsed decision while
// the width transition is enabled; suppress the transition for one frame so
// the correction lands instantly.
onActivated(() => {
  sidebarAnimReady.value = false
  updateSidebarCollapse()
  requestAnimationFrame(() => {
    requestAnimationFrame(() => sidebarAnimReady.value = true)
  })
})
</script>

<style scoped>
.tools-tab-content {
  display: flex;
  width: 100%;
  height: 100%;
  background: var(--bg-base);
  color: var(--text-primary);
}

.tools-sidebar {
  width: 11.25rem;
  flex-shrink: 0;
  margin-left: 1.25rem;
  margin-right: 0.625rem;
  padding: 1rem 0;
  border-right: 1px solid var(--border-hover);
  transition: width 0.15s ease;
  /* Scroll independently when the window is too short for every tool */
  overflow-y: auto;
  overflow-x: hidden;
}

.sidebar-collapse-btn {
  position: sticky;
  top: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.75rem;
  height: 1.75rem;
  margin: 0 0.5rem 0.5rem;
  margin-left: auto;
  padding: 0;
  background: var(--bg-base);
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  transition: all 0.12s ease;
}

.sidebar-collapse-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.tools-sidebar.no-anim,
.tools-sidebar.no-anim .tools-category,
.tools-sidebar.no-anim .sidebar-collapse-btn {
  transition: none;
}

.tools-sidebar.collapsed {
  width: 3.5rem;
  margin-left: 0.625rem;
  margin-right: 0.375rem;
}

.tools-sidebar.collapsed .sidebar-collapse-btn {
  margin-left: auto;
}

.tools-sidebar.collapsed .tools-category {
  justify-content: center;
  padding: 0.625rem 0.5rem;
  margin: 0 0.375rem;
  gap: 0;
}

.tools-category {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  padding: 0.625rem 1rem;
  margin: 0 0.5rem;
  font-size: 0.8125rem;
  font-family: var(--font-ui);
  user-select: none;
  white-space: nowrap;
  color: var(--text-secondary);
  border-radius: var(--radius-sm);
  transition: all 0.12s ease;
  border-left: 0.1875rem solid transparent;
  cursor: pointer;
}

.tools-category:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.tools-category.active {
  background: var(--accent-subtle);
  color: var(--accent);
  border-left-color: var(--accent);
  backdrop-filter: blur(0.5rem);
}

.category-icon {
  font-size: 1rem;
}

.category-label {
  line-height: 1;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tools-panel {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
}

.tools-inner {
  max-width: 960px;
  width: 100%;
  margin: 0 auto;
  padding: 1.25rem 1.5rem 1.5rem;
  display: flex;
  flex-direction: column;
}

/* Common layout + control styles for the per-tool subcomponents. Token
   spans and child-element markup come from runtime/v-html (no scope
   attribute), so everything reachable from child components uses :deep(). */
.tools-inner :deep(.pane) {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.tools-inner :deep(.pane-toolbar) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.tools-inner :deep(.toolbar-actions) {
  display: flex;
  gap: 0.5rem;
}

.tools-inner :deep(.pane-subtoolbar),
.tools-inner :deep(.encode-header) {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.tools-inner :deep(.pane-subtoolbar) {
  justify-content: space-between;
}

.tools-inner :deep(.pane-label) {
  font-size: 0.8125rem;
  color: var(--text-secondary, #909399);
}

.tools-inner :deep(.mono-input textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.8125rem;
}

.tools-inner :deep(.error-line) {
  font-size: 0.8125rem;
  color: var(--el-color-danger);
  word-break: break-word;
}

.tools-inner :deep(.hash-list) {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.tools-inner :deep(.hash-row) {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0.75rem;
  border-radius: 6px;
  background: var(--bg-raised, var(--bg-elevated, rgba(144, 147, 153, 0.08)));
}

.tools-inner :deep(.hash-row.single) {
  justify-content: space-between;
}

.tools-inner :deep(.hash-name) {
  flex: 0 0 5.5rem;
  font-size: 0.8125rem;
  font-weight: 600;
}

.tools-inner :deep(.hash-value) {
  flex: 1;
  min-width: 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.8125rem;
  word-break: break-all;
  white-space: pre-wrap;
}

.tools-inner :deep(.hash-value.is-error) {
  color: var(--el-color-danger);
}

.tools-inner :deep(.encode-kind) {
  width: 8.5rem;
}

.tools-inner :deep(.json-output) {
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

[data-theme="light"] .tools-inner :deep(.json-output) {
  --json-key: #0550ae;
  --json-string: #116329;
  --json-number: #953800;
  --json-bool: #8250df;
  --json-null: #0f7d87;
}

.tools-inner :deep(.json-output .tok-key) { color: var(--json-key); }
.tools-inner :deep(.json-output .tok-string) { color: var(--json-string); }
.tools-inner :deep(.json-output .tok-number) { color: var(--json-number); }
.tools-inner :deep(.json-output .tok-bool) { color: var(--json-bool); }
.tools-inner :deep(.json-output .tok-null) { color: var(--json-null); }
</style>
