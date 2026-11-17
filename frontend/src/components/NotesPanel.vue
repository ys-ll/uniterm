<template>
  <div class="notes-panel" :class="{ wide }">
    <!-- List column: shared by both layouts. In the narrow (left sidebar)
         layout the editor replaces it inline (no dialog); in the wide
         (bottom panel) layout it sits beside the editor, mirroring the
         layout the feature request (#1204) showed. -->
    <div v-show="wide || viewMode === 'list'" class="notes-list-col" :style="wide ? { width: listWidth + 'px' } : undefined">
      <div class="notes-toolbar">
        <el-input v-model="search" class="notes-search" :placeholder="t('notes.searchPlaceholder')" clearable />
        <button class="sb-icon-btn" :title="t('notes.newNote')" @click="onNew">
          <Plus :size="'0.9375rem'" />
        </button>
      </div>
      <div v-if="categories.length > 0" class="notes-filter">
        <div class="cat-chips" :class="{ collapsed: catRowCollapsed }">
          <button class="cat-chip" :class="{ active: categoryFilter === 'all' }" @click="categoryFilter = 'all'">
            {{ t('notes.allCategories') }}
          </button>
          <button
            v-for="c in categories"
            :key="c"
            class="cat-chip"
            :class="{ active: categoryFilter === c }"
            @click="categoryFilter = categoryFilter === c ? 'all' : c"
            @contextmenu.prevent.stop="onCatContextMenu($event, c)"
          >{{ c }}</button>
        </div>
        <button
          class="cat-toggle"
          :title="catRowCollapsed ? t('notes.expandCategories') : t('notes.collapseCategories')"
          @click="setCatRowCollapsed(!catRowCollapsed)"
        >
          <el-icon><component :is="catRowCollapsed ? ChevronRight : ChevronDown" :size="'0.75rem'" /></el-icon>
        </button>
        <!-- Category chip context menu: rename / delete (delete keeps the
             notes, they become uncategorized) -->
        <Menu ref="catMenuRef" v-model:visible="catMenuVisible">
          <MenuItem @click="startRenameCategory">{{ t('notes.renameCategory') }}</MenuItem>
          <MenuItem class="danger" @click="onDeleteCategory">{{ t('notes.deleteCategory') }}</MenuItem>
        </Menu>
      </div>
      <div class="notes-list">
        <div
          v-for="n in filteredNotes"
          :key="n.id"
          class="note-item"
          :class="{ active: n.id === selectedNoteId }"
          @click="onSelect(n)"
          @contextmenu.prevent.stop="onNoteContextMenu($event, n)"
        >
          <div class="note-item-main">
            <span class="note-title">{{ noteDisplayTitle(n) || t('notes.untitled') }}</span>
            <span v-if="n.category" class="note-cat">{{ n.category }}</span>
          </div>
          <span class="note-date">{{ formatDate(n.updatedAt) }}</span>
        </div>
        <div v-if="filteredNotes.length === 0" class="empty-state">
          {{ search.trim() || categoryFilter !== 'all' ? t('sidebar.noSearchResults') : t('notes.empty') }}
        </div>
      </div>
      <!-- Note context menu: move to category / delete. The category list
           leads with "new category" — confirming names a fresh tag and moves
           the right-clicked note into it. -->
      <Menu ref="noteMenuRef" v-model:visible="noteMenuVisible">
        <MenuSubmenu :label="t('notes.moveToCategory')">
          <MenuItem iconic :icon="Plus" @click="openNewCategoryForCtxNote">{{ t('notes.newCategory') }}</MenuItem>
          <MenuItem
            iconic
            :icon="(current?.category || '') === '' && current?.id === ctxNoteId ? Check : undefined"
            @click="moveCtxNote('')"
          >{{ t('notes.noCategory') }}</MenuItem>
          <MenuItem
            v-for="c in categories"
            :key="c"
            iconic
            :icon="current?.id === ctxNoteId && current?.category === c ? Check : undefined"
            @click="moveCtxNote(c)"
          >{{ c }}</MenuItem>
        </MenuSubmenu>
        <MenuItem class="danger" @click="deleteCtxNote">{{ t('notes.delete') }}</MenuItem>
      </Menu>
    </div>

    <!-- Wide (bottom panel) layout: draggable divider between list and editor -->
    <div
      v-if="wide"
      class="notes-divider"
      :class="{ dragging: isResizing }"
      @mousedown="onDividerStart"
    />

    <!-- Wide (bottom panel) layout: list + side-by-side editor -->
    <div v-if="wide" class="notes-editor-col">
      <template v-if="current">
        <div class="editor-header">
          <el-input v-model="editTitle" class="title-input" :placeholder="t('notes.titlePlaceholder')" />
          <div class="view-toggle">
            <button class="toggle-btn" :class="{ active: !previewing }" :title="t('notes.edit')" @click="setPreviewMode(false)">
              <Pencil :size="'0.75rem'" />
            </button>
            <button class="toggle-btn" :class="{ active: previewing }" :title="t('notes.preview')" @click="setPreviewMode(true)">
              <Eye :size="'0.75rem'" />
            </button>
          </div>
          <button class="sb-icon-btn danger" :title="t('notes.delete')" @click="onDelete(current)">
            <Trash2 :size="'0.875rem'" />
          </button>
        </div>
        <SyntaxEditor v-show="!previewing" v-model="editContent" class="notes-editor" lang="md" compact />
        <!-- Markdown preview (shared renderer, same as AI messages); selectable
             + right-click copy via the global .copyable-text menu -->
        <div v-show="previewing" class="notes-preview markdown-body copyable-text" v-html="previewHtml" @click="onPreviewClick" />
      </template>
      <div v-else class="empty-state editor-empty">{{ t('notes.noNoteSelected') }}</div>
    </div>

    <!-- Narrow (left sidebar) layout: the editor renders inline in place of
         the list, with a back button — no dialog. -->
    <div v-else-if="viewMode === 'editor' && current" class="notes-editor-col">
      <div class="editor-header narrow-header">
        <button class="sb-icon-btn" :title="t('sidebar.collapse')" @click="backToList">
          <ArrowLeft :size="'0.9375rem'" />
        </button>
        <el-input v-model="editTitle" class="title-input" :placeholder="t('notes.titlePlaceholder')" />
        <div class="view-toggle">
          <button class="toggle-btn" :class="{ active: !previewing }" :title="t('notes.edit')" @click="setPreviewMode(false)">
            <Pencil :size="'0.75rem'" />
          </button>
          <button class="toggle-btn" :class="{ active: previewing }" :title="t('notes.preview')" @click="setPreviewMode(true)">
            <Eye :size="'0.75rem'" />
          </button>
        </div>
        <button class="sb-icon-btn danger" :title="t('notes.delete')" @click="onDelete(current)">
          <Trash2 :size="'0.875rem'" />
        </button>
      </div>
      <SyntaxEditor v-show="!previewing" v-model="editContent" class="notes-editor" lang="md" compact />
      <div v-show="previewing" class="notes-preview markdown-body copyable-text" v-html="previewHtml" @click="onPreviewClick" />
    </div>

    <!-- New-category dialog (both layouts, opened from the "+" menu) -->
    <el-dialog v-model="categoryDialogVisible" :title="t('notes.newCategory')" width="22rem" append-to-body>
      <el-input v-model="categoryNameInput" :placeholder="t('notes.categoryPlaceholder')" maxlength="30" @keyup.enter="doCreateCategory" />
      <template #footer>
        <el-button @click="categoryDialogVisible = false">{{ t('conn.cancel') }}</el-button>
        <el-button type="primary" @click="doCreateCategory">{{ t('conn.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- Rename-category dialog (opened from the chip context menu) -->
    <el-dialog v-model="renameDialogVisible" :title="t('notes.renameCategory')" width="22rem" append-to-body>
      <el-input v-model="renameCategoryInput" :placeholder="t('notes.categoryPlaceholder')" maxlength="30" @keyup.enter="doRenameCategory" />
      <template #footer>
        <el-button @click="renameDialogVisible = false">{{ t('conn.cancel') }}</el-button>
        <el-button type="primary" @click="doRenameCategory">{{ t('conn.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessageBox } from 'element-plus'
import { ArrowLeft, Check, ChevronDown, ChevronRight, Eye, Pencil, Plus, Trash2 } from '@lucide/vue'
import { useI18n } from '../i18n'
import { useNotesStore, type Note } from '../stores/notesStore'
import { useUiLayoutStore } from '../stores/uiLayoutStore'
import { useLocalStateStore } from '../stores/localStateStore'
import { sanitizeRenderedHtml, renderMarkdownHtml, codeCopyTarget, COPY_ICON, CHECK_ICON } from '../utils/markdown'
import { writeClipboard } from '../composables/useClipboardWrite'
import SyntaxEditor from './SyntaxEditor.vue'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import MenuSubmenu from './MenuSubmenu.vue'

const { t } = useI18n()
const notesStore = useNotesStore()
const uiLayout = useUiLayoutStore()

// Wide = docked to the bottom panel (list + editor side by side). The left
// sidebar shows the list, with the editor rendered inline over it.
const wide = computed(() => uiLayout.effectiveLocation('notes') === 'bottom')

// ── List state ──
const search = ref('')
const categoryFilter = ref('all')
const selectedNoteId = ref<string | null>(null)
// Narrow layout only: which surface the panel shows.
const viewMode = ref<'list' | 'editor'>('list')

// Categories in use (notesStore computed), exposed for the template.
const categories = computed(() => notesStore.categories)

const filteredNotes = computed(() => {
  const q = search.value.trim().toLowerCase()
  return notesStore.sortedNotes.filter(n => {
    if (categoryFilter.value !== 'all' && (n.category || '') !== categoryFilter.value) return false
    if (!q) return true
    return (n.title || '').toLowerCase().includes(q) || n.content.toLowerCase().includes(q)
  })
})

const current = computed(() => (selectedNoteId.value ? notesStore.get(selectedNoteId.value) : undefined))

// ── List/editor divider (wide layout) ──
const NOTES_LIST_MIN_WIDTH = 180
const NOTES_LIST_MAX_WIDTH = 480
const listWidth = ref(240)
const isResizing = ref(false)

onMounted(async () => {
  void notesStore.load()
  // Restore persisted panel prefs (idempotent re-init — children mount
  // before App.vue awaits localState init, same pattern as Sidebar).
  try {
    const ls = useLocalStateStore()
    if (!ls.loaded) await ls.init()
    const w = ls.state.notesListWidth
    if (w > 0) listWidth.value = Math.min(Math.max(w, NOTES_LIST_MIN_WIDTH), NOTES_LIST_MAX_WIDTH)
    previewing.value = !!ls.state.notesPreviewMode
    catRowCollapsed.value = !!ls.state.notesCategoriesCollapsed
  } catch {
    // keep the defaults
  }
})

function onDividerStart(e: MouseEvent) {
  // Block the mousedown default action so a resize drag can never promote
  // into a text selection (same as the sidebar/AI-sidebar handles).
  e.preventDefault()
  isResizing.value = true
  const startX = e.clientX
  const startWidth = listWidth.value

  window.dispatchEvent(new CustomEvent('split:resize-start'))

  function onMouseMove(ev: MouseEvent) {
    if (!isResizing.value) return
    listWidth.value = Math.min(Math.max(startWidth + (ev.clientX - startX), NOTES_LIST_MIN_WIDTH), NOTES_LIST_MAX_WIDTH)
  }

  function onMouseUp() {
    isResizing.value = false
    useLocalStateStore().update({ notesListWidth: listWidth.value })
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    window.dispatchEvent(new CustomEvent('split:resize-end'))
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

// ── New-category dialog ──
// Opened only from the note context menu's "move to category" list: the
// confirmed name becomes a new tag and the right-clicked note moves into it.
const categoryDialogVisible = ref(false)
const categoryNameInput = ref('')

function openNewCategoryForCtxNote() {
  noteMenuVisible.value = false
  categoryNameInput.value = ''
  categoryDialogVisible.value = true
}

function doCreateCategory() {
  const name = categoryNameInput.value.trim()
  if (!name) return
  const id = ctxNoteId.value
  if (id) {
    notesStore.update(id, { category: name })
    if (id === selectedNoteId.value) syncEditorCategory(name)
  }
  categoryNameInput.value = ''
  categoryDialogVisible.value = false
}

// ── Category chip context menu (rename / delete) ──
const catMenuVisible = ref(false)
const catMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const ctxCategory = ref('')
const renameDialogVisible = ref(false)
const renameCategoryInput = ref('')

function onCatContextMenu(e: MouseEvent, c: string) {
  ctxCategory.value = c
  catMenuRef.value?.openAt(e.clientX, e.clientY)
}

function startRenameCategory() {
  catMenuVisible.value = false
  renameCategoryInput.value = ctxCategory.value
  renameDialogVisible.value = true
}

function doRenameCategory() {
  const newName = renameCategoryInput.value.trim()
  renameDialogVisible.value = false
  if (!newName || !ctxCategory.value) return
  notesStore.renameCategory(ctxCategory.value, newName)
  if (categoryFilter.value === ctxCategory.value) categoryFilter.value = newName
  // Keep the open editor's category mirror in sync so a pending flush can't
  // resurrect the old tag.
  if (current.value?.category === newName) syncEditorCategory(newName)
}

async function onDeleteCategory() {
  catMenuVisible.value = false
  const name = ctxCategory.value
  if (!name) return
  try {
    await ElMessageBox.confirm(t('notes.deleteCategoryConfirm'), t('notes.deleteCategory'), {
      type: 'warning',
    })
  } catch {
    return
  }
  notesStore.removeCategory(name)
  if (categoryFilter.value === name) categoryFilter.value = 'all'
  if (current.value && !current.value.category) syncEditorCategory(undefined)
}

// ── Editing fields ──
// Local mirrors of the current note's fields; watchers flush them back into
// the store debounced (every keystroke otherwise serializes the whole list).
const editTitle = ref('')
const editCategory = ref<string | undefined>(undefined)
const editContent = ref('')
// Edit vs markdown-preview mode. Sticky across note switches and restarts
// (persisted in LocalState.notesPreviewMode) — it is a viewing preference,
// not per-note state.
const previewing = ref(false)
// Category chip row collapsed to one line (persisted in LocalState).
const catRowCollapsed = ref(false)

function setPreviewMode(v: boolean) {
  previewing.value = v
  useLocalStateStore().update({ notesPreviewMode: v })
}

function setCatRowCollapsed(v: boolean) {
  catRowCollapsed.value = v
  useLocalStateStore().update({ notesCategoriesCollapsed: v })
}

const previewHtml = computed(() =>
  sanitizeRenderedHtml(renderMarkdownHtml(editContent.value, { copyButtonTitle: t('notes.copyCode') }))
)

// Copy button inside the preview's rendered code blocks (delegated — the
// buttons live in v-html). Same feedback swap as the AI sidebar.
async function onPreviewClick(event: MouseEvent) {
  const code = codeCopyTarget(event)
  if (!code) return
  const btn = (event.target as HTMLElement).closest('.code-copy-btn') as HTMLElement | null
  if (btn && await writeClipboard(code)) {
    btn.innerHTML = CHECK_ICON
    setTimeout(() => { btn.innerHTML = COPY_ICON }, 2000)
  }
}

let syncing = false
let dirty = false
let pendingTimer: ReturnType<typeof setTimeout> | null = null

function loadCurrent(n: Note | undefined) {
  syncing = true
  editTitle.value = n?.title || ''
  editCategory.value = n?.category || undefined
  editContent.value = n?.content || ''
  nextTick(() => { syncing = false })
}

function onFieldChange() {
  if (syncing) return
  dirty = true
  if (pendingTimer) clearTimeout(pendingTimer)
  pendingTimer = setTimeout(flushPending, 400)
}

function flushPending() {
  if (pendingTimer) {
    clearTimeout(pendingTimer)
    pendingTimer = null
  }
  if (!dirty || !selectedNoteId.value) return
  dirty = false
  notesStore.update(selectedNoteId.value, {
    title: editTitle.value,
    // Cleared select → '' so the store actually clears the category
    // (update() maps '' to undefined).
    category: editCategory.value ?? '',
    content: editContent.value,
  })
}

// Re-align the editor's category mirror without going through the dirty
// flag (used after category-level ops touched the current note).
function syncEditorCategory(v: string | undefined) {
  syncing = true
  editCategory.value = v
  nextTick(() => { syncing = false })
}

watch([editTitle, editCategory, editContent], onFieldChange)

watch(selectedNoteId, () => {
  flushPending()
  loadCurrent(current.value)
})

function onSelect(n: Note) {
  if (n.id === selectedNoteId.value) {
    if (!wide.value) viewMode.value = 'editor'
    return
  }
  selectedNoteId.value = n.id
  if (!wide.value) viewMode.value = 'editor'
}

// ── Note context menu: move to category / delete ──
const noteMenuVisible = ref(false)
const noteMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const ctxNoteId = ref<string | null>(null)

function onNoteContextMenu(e: MouseEvent, n: Note) {
  ctxNoteId.value = n.id
  noteMenuRef.value?.openAt(e.clientX, e.clientY)
}

function moveCtxNote(cat: string) {
  noteMenuVisible.value = false
  const id = ctxNoteId.value
  if (!id) return
  notesStore.update(id, { category: cat })
  if (id === selectedNoteId.value) syncEditorCategory(cat || undefined)
}

async function deleteCtxNote() {
  noteMenuVisible.value = false
  const n = ctxNoteId.value ? notesStore.get(ctxNoteId.value) : undefined
  if (n) await onDelete(n)
}

function backToList() {
  flushPending()
  viewMode.value = 'list'
}

function onNew() {
  // New notes inherit the category filter so they land in the group being
  // browsed ("all" leaves them uncategorized).
  const note = notesStore.add(categoryFilter.value !== 'all' ? categoryFilter.value : undefined)
  selectedNoteId.value = note.id
  loadCurrent(note)
  if (!wide.value) viewMode.value = 'editor'
}

async function onDelete(n: Note) {
  try {
    await ElMessageBox.confirm(t('notes.deleteConfirmDesc'), t('notes.deleteConfirmTitle'), {
      type: 'warning',
    })
  } catch {
    return
  }
  dirty = false
  if (pendingTimer) { clearTimeout(pendingTimer); pendingTimer = null }
  notesStore.remove(n.id)
  if (selectedNoteId.value === n.id) {
    selectedNoteId.value = null
    loadCurrent(undefined)
    if (!wide.value) viewMode.value = 'list'
  }
}

// List-title fallback: an untitled note shows its first non-empty content
// line (light markdown markers stripped). Nothing is written back — the
// stored title stays empty until the user names it.
function noteDisplayTitle(n: Note): string {
  const title = (n.title || '').trim()
  if (title) return title
  const line = (n.content || '')
    .split(/\r?\n/)
    .map(l => l.trim())
    .find(l => !!l) || ''
  return line.replace(/^(#{1,6}\s+|[-*]\s+|>\s*|\d+\.\s+|`+)/, '').trim()
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString()
}

onBeforeUnmount(flushPending)

defineExpose({ flushPending })
</script>

<style scoped>
.notes-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

.notes-list-col {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  flex: 1;
}

.notes-panel.wide .notes-list-col {
  flex: 0 0 auto;
  width: 15rem;
}

.notes-toolbar {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.625rem 0.625rem 0.375rem;
  flex-shrink: 0;
}

.notes-search {
  flex: 1;
  min-width: 0;
}

.sb-icon-btn {
  width: 1.625rem;
  height: 1.625rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  flex-shrink: 0;
  transition: all 0.12s ease;
}

.sb-icon-btn:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

/* Delete keeps the same ghost-button metrics; only the hover tint differs. */
.sb-icon-btn.danger:hover {
  color: var(--el-color-danger);
}

/* Draggable divider between the list and the editor (wide layout). Idle it
   draws the 1px separator; hover/drag widens it into the accent bar. */
.notes-divider {
  width: 0.375rem;
  flex-shrink: 0;
  cursor: col-resize;
  position: relative;
  user-select: none;
  z-index: 5;
}

.notes-divider::before {
  content: '';
  position: absolute;
  left: 0.1875rem;
  top: 0;
  bottom: 0;
  width: 1px;
  background: var(--border-subtle);
}

.notes-divider:hover::before,
.notes-divider.dragging::before {
  left: 0.125rem;
  width: 0.1875rem;
  background: var(--accent);
}

/* Category chips: free-form tags, not folders — wrap onto multiple lines so
   a long category list never gets cut off (the list below scrolls). The row
   is collapsible: collapsed clamps the chips to one line. */
.notes-filter {
  display: flex;
  align-items: flex-start;
  gap: 0.125rem;
  padding: 0 0.625rem 0.375rem;
  flex-shrink: 0;
}

.cat-toggle {
  width: 1.25rem;
  height: 1.25rem;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  margin-top: 0.0625rem;
  margin-left: auto;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  flex-shrink: 0;
  transition: all 0.12s ease;
}

.cat-toggle:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.cat-chips {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.25rem;
  min-width: 0;
  flex: 1;
}

.cat-chips.collapsed {
  flex-wrap: nowrap;
  overflow: hidden;
  max-height: 1.375rem;
}

.cat-chip {
  flex-shrink: 0;
  padding: 0.125rem 0.5rem;
  font-size: 0.6875rem;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  background: var(--bg-subtle);
  border: 1px solid var(--border-subtle);
  border-radius: 999px;
  cursor: pointer;
  transition: all 0.12s ease;
}

.cat-chip:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
  border-color: var(--accent);
}

.cat-chip.active {
  color: var(--accent);
  background: var(--accent-subtle);
  border-color: var(--accent);
}

.notes-list {
  flex: 1;
  overflow-y: auto;
  padding: 0 0.5rem 0.5rem;
  min-height: 0;
}

.note-item {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.4375rem 0.625rem;
  border-radius: var(--radius-sm);
  transition: all 0.12s ease;
  margin-bottom: 0.125rem;
  user-select: none;
  cursor: pointer;
}

.note-item:hover {
  background: var(--bg-hover);
}

.note-item.active {
  background: var(--accent-subtle);
  box-shadow: inset 0 0 0 1px var(--accent);
}

.note-item.active .note-title {
  color: var(--accent);
}

.note-item-main {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  flex: 1;
  min-width: 0;
}

.note-title {
  font-family: var(--font-ui);
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
  min-width: 0;
}

.note-cat {
  flex-shrink: 0;
  max-width: 6rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 0 0.375rem;
  font-size: 0.625rem;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  background: var(--bg-subtle);
  border-radius: 999px;
  line-height: 1.4;
}

.note-date {
  flex-shrink: 0;
  font-family: var(--font-ui);
  font-size: 0.625rem;
  color: var(--text-disabled);
}

/* ── Editor column ── */
.notes-editor-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.editor-header {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.625rem 0.625rem 0.375rem;
  flex-shrink: 0;
}

.title-input {
  flex: 1;
  min-width: 0;
}

.narrow-header {
  padding-top: 0.5rem;
}

.view-toggle {
  display: flex;
  flex-shrink: 0;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.toggle-btn {
  width: 1.625rem;
  height: 1.625rem;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--text-muted);
  transition: all 0.12s ease;
}

.toggle-btn:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.toggle-btn.active {
  color: var(--accent);
  background: var(--accent-subtle);
}

.notes-editor {
  flex: 1;
  min-height: 0;
  border: none;
  border-radius: 0;
}

.notes-preview {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0.75rem 1rem;
  font-size: 0.8125rem;
  line-height: 1.6;
  /* Body disables selection globally (style.css) — the preview is reading
     surface and must be selectable; right-click offers the shared
     .copyable-text copy menu (main.ts). */
  user-select: text;
  -webkit-user-select: text;
  cursor: text;
  --wails-draggable: no-drag;
}

/* ── Empty states ── */
.empty-state {
  padding: 2rem 1rem;
  text-align: center;
  font-size: 0.75rem;
  color: var(--text-disabled);
  font-family: var(--font-ui);
}

.editor-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* Markdown preview typography (used by the wide-layout preview pane).
   :deep() everywhere — the preview body is injected via v-html. */
.markdown-body h1,
.markdown-body h2,
.markdown-body h3,
.markdown-body h4,
.markdown-body h5,
.markdown-body h6 {
  margin: 0.75rem 0 0.375rem;
  line-height: 1.4;
}

.markdown-body p {
  margin: 0.375rem 0;
}

.markdown-body ul,
.markdown-body ol {
  margin: 0.375rem 0;
  padding-left: 1.25rem;
}

.markdown-body blockquote {
  margin: 0.375rem 0;
  padding: 0.125rem 0.75rem;
  border-left: 2px solid var(--border-subtle);
  color: var(--text-secondary);
}

.markdown-body table {
  border-collapse: collapse;
  margin: 0.5rem 0;
}

.markdown-body th,
.markdown-body td {
  border: 1px solid var(--border-subtle);
  padding: 0.25rem 0.5rem;
  font-size: 0.75rem;
}

.markdown-body code {
  font-family: var(--font-mono, monospace);
  font-size: 0.75rem;
  background: var(--bg-subtle);
  padding: 0.0625rem 0.25rem;
  border-radius: var(--radius-sm);
}

.markdown-body pre {
  background: var(--bg-subtle);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 0.5rem 0.75rem;
  overflow-x: auto;
}

.markdown-body pre code {
  background: transparent;
  padding: 0;
}

/* Fenced code blocks carry a shared header (lang tag + copy button) — same
   look as the AI sidebar's rendered output. */
.markdown-body :deep(.code-block-wrapper) {
  margin: 0.5rem 0;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.markdown-body :deep(.code-block-header) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.1875rem 0.5rem;
  background: var(--bg-overlay);
  border-bottom: 1px solid var(--border-subtle);
}

.markdown-body :deep(.code-lang-tag) {
  font-size: 0.625rem;
  font-family: var(--font-ui);
  color: var(--text-muted);
}

.markdown-body :deep(.code-block-wrapper pre) {
  margin: 0;
  border: none;
  border-radius: 0;
  background: var(--bg-base);
}

.markdown-body :deep(.code-block-wrapper code) {
  padding: 0;
  background: transparent;
  color: var(--text-primary);
}

.markdown-body :deep(.code-block-header .tool-copy-btn) {
  background: none;
  border: none;
  padding: 0 0.125rem;
  opacity: 0.4;
  transition: opacity 0.15s;
  color: var(--text-muted);
  display: inline-flex;
  align-items: center;
}

.markdown-body :deep(.code-block-header .tool-copy-btn:hover) {
  opacity: 1;
}

.markdown-body a {
  color: var(--accent);
}
</style>
