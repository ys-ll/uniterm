<template>
  <el-dialog
    append-to-body
    :model-value="visible"
    :title="cellTitle"
    width="80%"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    destroy-on-close
    @update:model-value="(v: boolean) => emit('update:visible', v)"
    @closed="onClosed"
  >
    <div class="editor-toolbar">
      <!-- Clipboard ops: cut/paste need an editable buffer; copy always works. -->
      <button v-if="!readOnly" class="toolbar-icon-btn" :title="t('db.cellEditor.cut')" @click="editorRef?.cut()">
        <el-icon><Scissors :size="14" /></el-icon>
      </button>
      <button class="toolbar-icon-btn" :title="t('db.cellEditor.copy')" @click="editorRef?.copy()">
        <el-icon><Copy :size="14" /></el-icon>
      </button>
      <button v-if="!readOnly" class="toolbar-icon-btn" :title="t('db.cellEditor.paste')" @click="editorRef?.paste()">
        <el-icon><ClipboardPaste :size="14" /></el-icon>
      </button>
      <template v-if="!readOnly">
        <span class="toolbar-divider" />
        <!-- Edit ops: undo/redo wired to CodeMirror history. -->
        <button class="toolbar-icon-btn" @click="editorRef?.undo()" :title="t('db.cellEditor.undo')">
          <el-icon><Undo2 :size="14" /></el-icon>
        </button>
        <button class="toolbar-icon-btn" @click="editorRef?.redo()" :title="t('db.cellEditor.redo')">
          <el-icon><Redo2 :size="14" /></el-icon>
        </button>
      </template>
      <span class="toolbar-divider" />
      <!-- View: font size. -->
      <button class="toolbar-icon-btn" @click="fontSizeDown" :title="t('db.cellEditor.fontSize')">
        <el-icon><ZoomOut :size="14" /></el-icon>
      </button>
      <span class="font-size-label">{{ fontSize }}px</span>
      <button class="toolbar-icon-btn" @click="fontSizeUp" :title="t('db.cellEditor.fontSize')">
        <el-icon><ZoomIn :size="14" /></el-icon>
      </button>
      <span class="toolbar-divider" />
      <!-- Search toggle (the editor owns the search bar; Ctrl+F also opens it). -->
      <button
        class="toolbar-icon-btn"
        :class="{ active: searchOpen }"
        :title="t('db.cellEditor.search')"
        @click="searchOpen = editorRef?.toggleSearch() ?? false"
      >
        <el-icon><Search :size="14" /></el-icon>
      </button>
      <!-- View: wrap toggle — the whole point for long/multi-line cell values. -->
      <button
        class="toolbar-icon-btn"
        :class="{ active: wrapEnabled }"
        :title="t('db.cellEditor.wrap')"
        @click="wrapEnabled = !wrapEnabled"
      >
        <el-icon><WrapText :size="14" /></el-icon>
      </button>
    </div>
    <div class="editor-host">
      <SyntaxEditor
        ref="editorRef"
        v-model="content"
        :lang="syntaxLang"
        :wrap="wrapEnabled"
        :font-size="fontSize"
        :compact="true"
        :readonly="readOnly || isNull"
        @search-open="(v: boolean) => (searchOpen = v)"
      />
    </div>
    <template #footer>
      <div class="editor-footer">
        <div class="editor-opts">
          <el-select v-model="syntaxLang" style="width: 6.875rem" filterable>
            <el-option v-for="l in LANG_OPTIONS" :key="l.value" :label="l.label" :value="l.value" />
          </el-select>
          <!-- NULL toggle, same semantics as the grid's inline "N" button. -->
          <button
            v-if="nullable && !readOnly"
            type="button"
            class="null-btn"
            :class="{ active: isNull }"
            :title="t('db.null')"
            @click="isNull = !isNull"
          >
            N
          </button>
          <span v-if="isNull" class="null-hint">{{ t('db.null') }}</span>
        </div>
        <div class="editor-buttons">
          <el-button @click="close">{{ t('db.cellEditor.cancel') }}</el-button>
          <el-button v-if="!readOnly" type="primary" @click="save">{{ t('db.cellEditor.save') }}</el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useI18n } from '../i18n'
import SyntaxEditor from './SyntaxEditor.vue'
import { Undo2, Redo2, ZoomOut, ZoomIn, WrapText, Search, Scissors, Copy, ClipboardPaste } from '@lucide/vue'

const { t } = useI18n()

defineProps<{
  visible?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  /** Save confirmed; payload is the new value (null when NULL is toggled on). */
  (e: 'save', value: string | null): void
}>()

const editorRef = ref<{ focus: () => void; undo: () => void; redo: () => void; toggleSearch: () => boolean; cut: () => Promise<boolean>; copy: () => Promise<boolean>; paste: () => Promise<boolean>; flush: () => void } | null>(null)
// Mirrors the editor's search-bar visibility (kept in sync via the
// search-open event, so Ctrl+F inside the editor updates the button too).
const searchOpen = ref(false)
const cellTitle = ref('')
const content = ref('')
const readOnly = ref(false)
const nullable = ref(false)
const isNull = ref(false)
const wrapEnabled = ref(true)
// Editor font size, 10–24px. Session-scoped: applies via the host element's
// font-size (SyntaxEditor inherits it), so changes never rebuild the editor.
const fontSize = ref(13)
const FONT_MIN = 10
const FONT_MAX = 24
function fontSizeDown() { fontSize.value = Math.max(FONT_MIN, fontSize.value - 1) }
function fontSizeUp() { fontSize.value = Math.min(FONT_MAX, fontSize.value + 1) }

// Highlight language; sniffed from the content on open (JSON → json, else
// Plain Text), user-overridable from the dropdown.
const syntaxLang = ref('text')

// Independent copy of the highlight-language list (kept local so this dialog
// stays self-contained). DB cells only need the DBeaver-style basics — the
// full language list lives in the SFTP file editor.
const LANG_OPTIONS = [
  { value: 'html', label: 'HTML' },
  { value: 'json', label: 'JSON' },
  { value: 'md', label: 'Markdown' },
  { value: 'conf', label: 'Properties/INI' },
  { value: 'text', label: 'Text' },
  { value: 'xml', label: 'XML' },
  { value: 'yaml', label: 'YAML' },
]

/**
 * Guess a highlighting language from the cell content itself (no file path to
 * go by): JSON objects/arrays are the only cheap, reliable sniff; everything
 * else stays Plain Text.
 */
function detectLangFromContent(value: string): string {
  const s = value.trim()
  if ((s.startsWith('{') && s.endsWith('}')) || (s.startsWith('[') && s.endsWith(']'))) {
    try { JSON.parse(s); return 'json' } catch { /* not JSON */ }
  }
  return 'text'
}

interface OpenOptions {
  value: unknown
  field: string
  nullable?: boolean
  readonly?: boolean
}

async function open(opts: OpenOptions) {
  cellTitle.value = opts.field
  readOnly.value = opts.readonly ?? false
  nullable.value = opts.nullable ?? false
  isNull.value = opts.value === null
  content.value = opts.value === null || opts.value === undefined ? '' : String(opts.value)
  syntaxLang.value = detectLangFromContent(content.value)
  wrapEnabled.value = true
  emit('update:visible', true)
  // destroy-on-close rebuilds the editor per open; wait for it to mount
  // before focusing, or the first open loses focus.
  await nextTick()
  editorRef.value?.focus()
}

// Large docs emit v-model updates on a debounce (SyntaxEditor), so drain any
// pending emit before reading content.
function flushEditorContent() {
  editorRef.value?.flush()
}

function save() {
  if (readOnly.value) return
  flushEditorContent()
  emit('save', isNull.value ? null : content.value)
  emit('update:visible', false)
}

function close() {
  emit('update:visible', false)
}

function onClosed() {
  content.value = ''
  isNull.value = false
  searchOpen.value = false
}

defineExpose({ open })
</script>

<style scoped>
/* Toolbar/host/footer mirror the SFTP file editor (FileEditorDialog) so both
   editors read as one component family. */
.editor-toolbar {
  display: flex;
  align-items: center;
  gap: 0.125rem;
  padding: 0 0.625rem 0.375rem;
}
.toolbar-divider {
  width: 1px;
  height: 1rem;
  margin: 0 0.1875rem;
  flex-shrink: 0;
  background: var(--border-subtle);
}
.toolbar-icon-btn {
  width: 1.625rem;
  height: 1.625rem;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  flex-shrink: 0;
  transition: all 0.12s ease;
}
.toolbar-icon-btn:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}
.toolbar-icon-btn.active {
  color: var(--accent);
  background: var(--accent-subtle);
}
.font-size-label {
  font-size: 0.75rem;
  color: var(--text-secondary);
  min-width: 2.25rem;
  text-align: center;
  flex-shrink: 0;
}
.editor-host {
  height: 60vh;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  overflow: hidden;
  background: #282c34;
}
.editor-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 0.5rem 0.75rem;
  width: 100%;
}
.editor-buttons {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-left: auto;
}
.editor-buttons .el-button + .el-button {
  margin-left: 0;
}
.editor-opts {
  display: flex;
  align-items: center;
  flex: 1 1 auto;
  min-width: 0;
  gap: 0.5rem 0.75rem;
}
.null-btn {
  flex-shrink: 0;
  width: 1.75rem;
  height: 2rem;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-size: 0.75rem;
  font-weight: 700;
  font-style: italic;
  transition: all 0.12s ease;
}
.null-btn:hover {
  color: var(--text-primary);
  border-color: var(--accent);
}
.null-btn.active {
  color: var(--accent);
  border-color: var(--accent);
  background: var(--accent-subtle);
}
.null-hint {
  font-size: 0.75rem;
  color: var(--text-muted);
  font-style: italic;
}
</style>
