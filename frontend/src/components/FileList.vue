<template>
  <div
    ref="listRootRef"
    class="sftp-file-list"
    tabindex="0"
    @keydown="onListKeydown"
    @click="onListClick"
  >
    <div class="filter-bar">
      <el-input
        v-model="filterText"
        :placeholder="t('sftp.filterByName')"
       
        clearable
      />
      <!-- History navigation: toolbar buttons in the flat (dual-pane) layout,
           menu entries in the compact (sidebar) layout. -->
      <button v-if="flatToolbar" class="filter-icon-btn" :disabled="!canBack" @click="emit('back')" :title="t('sftp.back')">
        <el-icon><ChevronLeft :size="14" /></el-icon>
      </button>
      <button v-if="flatToolbar" class="filter-icon-btn" :disabled="!canForward" @click="emit('forward')" :title="t('sftp.forward')">
        <el-icon><ChevronRight :size="14" /></el-icon>
      </button>
      <button v-if="flatToolbar" class="filter-icon-btn" @click="emit('up')" :title="t('sftp.goUp')">
        <el-icon><CornerLeftUp :size="14" /></el-icon>
      </button>
      <!-- View group: refresh + hidden-files visibility. -->
      <span v-if="flatToolbar" class="toolbar-divider" />
      <button class="filter-icon-btn" @click="emit('refresh')" :title="t('sftp.refresh')">
        <el-icon><RefreshCw :size="14" /></el-icon>
      </button>
      <button
        v-if="flatToolbar"
        class="filter-icon-btn"
        :class="{ active: showHidden }"
        @click="toggleShowHidden"
        :title="showHidden ? t('sftp.hideHidden') : t('sftp.showHidden')"
      >
        <el-icon><Eye :size="14" /></el-icon>
      </button>
      <!-- Transfer group: upload. -->
      <span v-if="flatToolbar && mode === 'remote'" class="toolbar-divider" />
      <button v-if="mode === 'remote'" class="filter-icon-btn" @click="emit('upload')" :title="t('sftp.upload')">
        <el-icon><Upload :size="14" /></el-icon>
      </button>
      <!-- Create group: new file / directory / link. Flat keeps every action
           on the bar, so there is no more-menu in this layout. -->
      <span v-if="flatToolbar" class="toolbar-divider" />
      <button v-if="flatToolbar" class="filter-icon-btn" @click="doNewFile" :title="t('sftp.newFile')">
        <el-icon><FilePlus2 :size="14" /></el-icon>
      </button>
      <button v-if="flatToolbar" class="filter-icon-btn" @click="doMkdir" :title="t('sftp.newDirectory')">
        <el-icon><FolderPlus :size="14" /></el-icon>
      </button>
      <button v-if="flatToolbar && supportsSymlink" class="filter-icon-btn" @click="doSymlink" :title="t('sftp.newLink')">
        <el-icon><Link :size="14" /></el-icon>
      </button>
      <button v-if="!flatToolbar" class="filter-icon-btn" @click.stop="moreMenuRef?.toggle($event.currentTarget as HTMLElement)" :title="t('sftp.more')">
        <el-icon><MoreHorizontal :size="14" /></el-icon>
      </button>
    </div>
    <PathBreadcrumb
      v-if="breadcrumbMode"
      :path="breadcrumbPath || ''"
      :drives="breadcrumbDrives"
      :bookmark-mode="breadcrumbMode"
      :saved-paths="breadcrumbSavedPaths"
      @navigate="(p: string) => emit('navigate', p)"
      @save-bookmark="(p: string) => emit('saveBookmark', p)"
      @remove-bookmark="(p: string) => emit('removeBookmark', p)"
    />
    <div v-if="clipboardCount" class="clipboard-bar">
      <span class="clipboard-info">{{ clipboardMode === 'cut' ? t('sftp.cut') : t('sftp.copy') }} ({{ clipboardCount }})</span>
      <el-button type="primary" @click="emit('paste')">{{ t('sftp.paste') }}</el-button>
      <el-button @click="emit('clearClipboard')">{{ t('sftp.dialog.cancel') }}</el-button>
    </div>
    <div class="table-wrapper" @contextmenu.prevent="onEmptyAreaContextMenu" @mousedown="onTableMouseDown">
      <div v-if="loading || pasteLoading" class="loading-overlay">
        <div class="loading-content">
          <div class="loading-spinner"></div>
          <span class="loading-text">{{ pasteLoading ? t('sftp.pasting') : t('sftp.loading') }}</span>
          <el-button @click="pasteLoading ? emit('cancelPaste') : emit('cancelLoad')">{{ t('sftp.cancel') }}</el-button>
        </div>
      </div>
      <el-table
        ref="tableRef"
        :key="locale"
        :data="visibleFiles"
        size="small"
        border
        :row-class-name="getRowClassName"
        @row-click="onRowClick"
        @row-dblclick="onRowDblClick"
        @row-contextmenu="onRowContextMenu"
      >
      <el-table-column :label="t('sftp.name')" min-width="160" sortable :sort-method="sortByName" show-overflow-tooltip>
        <template #default="{ row }">
          <div class="name-cell" :draggable="true" @dragstart="onDragStart($event, row)">
            <el-icon v-if="isSymlink(row)"><Link :size="14" /></el-icon>
            <el-icon v-else-if="row.isDir"><Folder :size="14" /></el-icon>
            <el-icon v-else><File :size="14" /></el-icon>
            <div class="name-info">
              <span class="file-name" :class="{ selected: isSelected(row) }">{{ row.name }}</span>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('sftp.type')" width="90" sortable :sort-method="sortByType" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="cell-secondary">{{ fileTypeLabel(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('sftp.modified')" width="150" sortable :sort-method="sortByTime" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="cell-secondary">{{ formatDate(row.modTime) }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('sftp.size')" width="70" align="right" sortable :sort-method="sortBySize" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="cell-secondary">{{ row.isDir ? '-' : formatSize(row.size) }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('sftp.permission')" width="110" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="cell-secondary">{{ row.mode || '-' }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('sftp.owner')" width="100" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="cell-secondary">{{ row.owner || '-' }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('sftp.group')" width="100" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="cell-secondary">{{ row.group || '-' }}</span>
        </template>
      </el-table-column>
    </el-table>
    <div
      v-if="bandRect"
      class="band-rect"
      :style="{ left: bandRect.x + 'px', top: bandRect.y + 'px', width: bandRect.w + 'px', height: bandRect.h + 'px' }"
    />
    </div>
    <div v-if="selectionStats.count > 0" class="selection-bar">
      <span class="selection-info">{{ t('sftp.selectionStats', { count: selectionStats.count }) }}</span>
      <span v-if="selectionStats.size > 0">{{ formatSize(selectionStats.size) }}</span>
    </div>

    <Menu ref="ctxMenuRef" v-model:visible="ctxMenuVisible" v-slot="{ current }">
        <template v-if="menuType === 'file'">
          <MenuItem @click="doEdit">{{ t('sftp.edit') }}</MenuItem>
          <MenuItem @click="doEditExternal">{{ t('sftp.editExternal') }}</MenuItem>
          <MenuItem @click="doNewFile">{{ t('sftp.newFile') }}</MenuItem>
          <MenuItem @click="doMkdir">{{ t('sftp.newDirectory') }}</MenuItem>
          <MenuItem v-if="supportsSymlink" @click="doSymlink">{{ t('sftp.newLink') }}</MenuItem>
          <MenuDivider />
          <MenuItem @click="doCopyToClipboard">{{ t('sftp.copy') }}</MenuItem>
          <MenuItem @click="doCutToClipboard">{{ t('sftp.cut') }}</MenuItem>
          <MenuItem :class="{ disabled: !clipboardCount }" @click="clipboardCount && doPaste()">{{ t('sftp.paste') }}</MenuItem>
          <MenuDivider />
          <MenuItem v-if="props.showSendToOther !== false" @click="doSendToOther">{{ t(sendToKey) }}</MenuItem>
          <MenuItem @click="doCopyPath">{{ t('sftp.copyPath') }}</MenuItem>
          <MenuItem v-if="showCopyPathToTerminal" @click="doCopyPathToTerminal">{{ t('sftp.copyPathToTerminal') }}</MenuItem>
          <MenuItem v-if="mode === 'remote'" @click="doDownloadTo">{{ t('sftp.downloadTo') }}</MenuItem>
          <MenuDivider />
          <MenuItem @click="doRename">{{ t('sftp.rename') }}</MenuItem>
          <MenuItem @click="doDelete">{{ t('sftp.delete') }}</MenuItem>
          <MenuItem v-if="mode === 'remote'" @click="doChmod">{{ t('sftp.changePermission') }}</MenuItem>
        </template>
        <template v-else-if="menuType === 'dir'">
          <MenuItem @click="doNewFile">{{ t('sftp.newFile') }}</MenuItem>
          <MenuItem @click="doMkdir">{{ t('sftp.newDirectory') }}</MenuItem>
          <MenuItem v-if="supportsSymlink" @click="doSymlink">{{ t('sftp.newLink') }}</MenuItem>
          <MenuDivider />
          <MenuItem @click="doCopyToClipboard">{{ t('sftp.copy') }}</MenuItem>
          <MenuItem @click="doCutToClipboard">{{ t('sftp.cut') }}</MenuItem>
          <MenuItem :class="{ disabled: !clipboardCount }" @click="clipboardCount && doPaste()">{{ t('sftp.paste') }}</MenuItem>
          <MenuDivider />
          <MenuItem v-if="props.showSendToOther !== false" @click="doSendToOther">{{ t(sendToKey) }}</MenuItem>
          <MenuItem @click="doCopyPath">{{ t('sftp.copyPath') }}</MenuItem>
          <MenuItem v-if="showCopyPathToTerminal" @click="doCopyPathToTerminal">{{ t('sftp.copyPathToTerminal') }}</MenuItem>
          <MenuItem v-if="mode === 'remote'" @click="doDownloadTo">{{ t('sftp.downloadTo') }}</MenuItem>
          <MenuDivider />
          <MenuItem @click="doRename">{{ t('sftp.rename') }}</MenuItem>
          <MenuItem @click="doDelete">{{ t('sftp.delete') }}</MenuItem>
          <MenuItem v-if="mode === 'remote'" @click="doChmod">{{ t('sftp.changePermission') }}</MenuItem>
        </template>
        <template v-else-if="menuType === 'batch'">
          <MenuItem @click="doCopyToClipboard">{{ t('sftp.copy') }}</MenuItem>
          <MenuItem @click="doCutToClipboard">{{ t('sftp.cut') }}</MenuItem>
          <MenuItem :class="{ disabled: !clipboardCount }" @click="clipboardCount && doPaste()">{{ t('sftp.paste') }}</MenuItem>
          <MenuDivider />
          <MenuItem v-if="props.showSendToOther !== false" @click="doSendToOther">{{ t(sendToKey) }}</MenuItem>
          <MenuItem @click="doCopyPath">{{ t('sftp.copyPath') }}</MenuItem>
          <MenuItem v-if="showCopyPathToTerminal" @click="doCopyPathToTerminal">{{ t('sftp.copyPathToTerminal') }}</MenuItem>
          <MenuItem v-if="mode === 'remote'" @click="doDownloadTo">{{ t('sftp.downloadTo') }}</MenuItem>
          <MenuDivider />
          <MenuItem v-if="mode === 'remote'" class="disabled">{{ t('sftp.renameDisabled') }}</MenuItem>
          <MenuItem v-if="mode === 'local'" @click="doRename">{{ t('sftp.rename') }}</MenuItem>
          <MenuItem @click="doDelete">{{ t('sftp.delete') }}</MenuItem>
          <MenuItem v-if="mode === 'remote'" class="disabled">{{ t('sftp.chmodDisabled') }}</MenuItem>
        </template>
        <template v-else-if="menuType === 'empty'">
          <MenuItem @click="doNewFile">{{ t('sftp.newFile') }}</MenuItem>
          <MenuItem @click="doMkdir">{{ t('sftp.newDirectory') }}</MenuItem>
          <MenuItem v-if="supportsSymlink" @click="doSymlink">{{ t('sftp.newLink') }}</MenuItem>
          <MenuDivider />
          <MenuItem :class="{ disabled: !clipboardCount }" @click="clipboardCount && doPaste()">{{ t('sftp.paste') }}</MenuItem>
        </template>
    </Menu>

    <!-- The more-menu only exists in the compact (sidebar) layout; flat keeps
         every action on the toolbar. -->
    <Menu v-if="!flatToolbar" ref="moreMenuRef" v-model:visible="moreMenuVisible">
      <!-- Compact (sidebar) layout: history navigation lives here instead of
           the narrow toolbar. Flat keeps it as toolbar buttons — toolbar
           actions are never duplicated into this menu. -->
      <template v-if="!flatToolbar">
        <MenuItem :class="{ disabled: !canBack }" @click="canBack && emit('back')">{{ t('sftp.back') }}</MenuItem>
        <MenuItem :class="{ disabled: !canForward }" @click="canForward && emit('forward')">{{ t('sftp.forward') }}</MenuItem>
        <MenuItem @click="emit('up')">{{ t('sftp.goUp') }}</MenuItem>
        <MenuDivider />
      </template>
      <MenuItem v-if="!flatToolbar" @click="doNewFile">{{ t('sftp.newFile') }}</MenuItem>
      <MenuItem v-if="!flatToolbar" @click="doMkdir">{{ t('sftp.newDirectory') }}</MenuItem>
      <MenuItem v-if="supportsSymlink" @click="doSymlink">{{ t('sftp.newLink') }}</MenuItem>
      <MenuDivider />
      <MenuItem class="iconic" :class="{ active: showHidden }" @click="toggleShowHidden">
        <el-icon><Eye :size="14" /></el-icon>
        {{ showHidden ? t('sftp.hideHidden') : t('sftp.showHidden') }}
      </MenuItem>
    </Menu>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Folder, File, Link, RefreshCw, Eye, Upload, FilePlus2, FolderPlus, MoreHorizontal, ChevronLeft, ChevronRight, CornerLeftUp } from '@lucide/vue'
import { useI18n } from '../i18n'
import { msg } from '../services/message'
import { joinPath } from '../composables/useFilePanel'
import PathBreadcrumb from './PathBreadcrumb.vue'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import MenuDivider from './MenuDivider.vue'

export interface FileItem {
  name: string
  size: number
  modTime: string
  mode: string
  isDir: boolean
  isHidden: boolean
  owner: string
  group: string
}

const props = defineProps<{
  files: FileItem[]
  mode: 'local' | 'remote'
  loading?: boolean
  pasteLoading?: boolean
  cutItemNames?: string[]
  clipboardCount?: number
  clipboardMode?: 'copy' | 'cut'
  /** Hide the "send to other pane" entry — for hosts with a single pane. */
  showSendToOther?: boolean
  /** Show the "new link" (symbolic link) entry — only for backends with link semantics. */
  supportsSymlink?: boolean
  /** Show the "copy path to terminal" context-menu entry — sidebar hosts only:
   *  the dual-pane tab has no terminal beside it to receive the path. */
  showCopyPathToTerminal?: boolean
  breadcrumbMode?: 'local' | 'remote'
  breadcrumbPath?: string
  breadcrumbSavedPaths?: string[]
  breadcrumbDrives?: string[]
  /** Whether history navigation has a previous / next directory. The back and
   *  forward buttons are disabled when omitted (hosts without history). */
  canBack?: boolean
  canForward?: boolean
  /** Toolbar form: 'flat' also shows the create actions (and a selection-driven
   *  download) as icon buttons in the filter bar; 'compact' (default) keeps them
   *  in the more-menu only. */
  toolbarLayout?: 'flat' | 'compact'
}>()

const emit = defineEmits<{
  open: [item: FileItem]
  navigate: [path: string]
  sendToOther: [items: FileItem[]]
  rename: [item: FileItem]
  delete: [items: FileItem[]]
  refresh: []
  mkdir: []
  symlink: []
  chmod: [item: FileItem]
  upload: []
  downloadTo: [items: FileItem[]]
  cancelLoad: []
  cancelPaste: []
  edit: [item: FileItem]
  editExternal: [item: FileItem]
  newFile: []
  copyToClipboard: [items: FileItem[]]
  cutToClipboard: [items: FileItem[]]
  paste: []
  clearClipboard: []
  saveBookmark: [path: string]
  removeBookmark: [path: string]
  back: []
  forward: []
  up: []
  copyPathToTerminal: [text: string]
}>()

const { t, locale } = useI18n()

const filterText = ref('')
const showHidden = ref(false)
const selectedItems = ref<FileItem[]>([])
// Keep the selection pointing at live row objects: after a refresh the list is
// rebuilt with new objects, and stale ones would carry outdated fields (e.g.
// the old permission mode when re-opening the chmod dialog).
watch(() => props.files, (fresh) => {
  if (!selectedItems.value.length) return
  selectedItems.value = selectedItems.value
    .map(sel => fresh.find(f => f.name === sel.name))
    .filter((f): f is FileItem => !!f)
})
const lastClickedIndex = ref(-1)
const ctxMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const ctxMenuVisible = ref(false)
const menuType = ref<'file' | 'dir' | 'batch' | 'empty'>('file')
const moreMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const moreMenuVisible = ref(false)
const tableRef = ref<any>(null)

const targetSide = computed(() => props.mode === 'local' ? t('sftp.remote') : t('sftp.local'))
const sendToKey = computed(() => props.mode === 'local' ? 'sftp.sendToRemote' : 'sftp.sendToLocal')
const flatToolbar = computed(() => props.toolbarLayout === 'flat')

// Footer stats for the current multi-selection. The '..' parent row is not a
// real entry, so it never counts toward the item total or the size sum.
const selectionStats = computed(() => {
  const items = selectedItems.value.filter(i => i.name !== '..')
  const totalSize = items.reduce((sum, i) => sum + (i.isDir ? 0 : i.size), 0)
  return { count: items.length, size: totalSize }
})

const filteredFiles = computed(() => {
  let list = [...props.files]
  if (!list.find(f => f.name === '..')) {
    list.unshift({ name: '..', size: 0, modTime: '', mode: '', isDir: true, isHidden: false, owner: '', group: '' })
  }
  list.sort((a, b) => {
    if (a.name === '..') return -1
    if (b.name === '..') return 1
    if (a.isDir && !b.isDir) return -1
    if (!a.isDir && b.isDir) return 1
    return a.name.localeCompare(b.name)
  })
  if (!showHidden.value) {
    list = list.filter(f => f.name === '..' || (!f.name.startsWith('.') && !f.isHidden))
  }
  const q = filterText.value.trim().toLowerCase()
  if (!q) return list
  return list.filter(f => f.name.toLowerCase().includes(q))
})

// --- Progressive loading ---------------------------------------------------
// A directory can contain tens of thousands of entries. Rendering the whole
// list into el-table at once creates an unbounded number of DOM rows, which
// freezes / crashes the renderer (issue 478). So we only hand el-table a slice
// of the filtered list and grow it as the user scrolls near the bottom, keeping
// the rendered DOM bounded regardless of the total entry count.
const PAGE_SIZE = 200
const INITIAL_PAGES = 1
const NEAR_BOTTOM_PX = 300

const visibleCount = ref(INITIAL_PAGES)
const visibleFiles = computed(() => filteredFiles.value.slice(0, visibleCount.value * PAGE_SIZE))

let scrollWrapEl: HTMLElement | null = null

// el-table scrolls through an internal .el-scrollbar once it has a height; that
// element is where we must listen for scroll (native scroll does not bubble).
function bindTableScroll() {
  const el = (tableRef.value?.$el ?? null) as HTMLElement | null
  scrollWrapEl = el?.querySelector('.el-scrollbar__wrap') ?? null
  if (scrollWrapEl && !scrollWrapEl.dataset.lazyBinded) {
    scrollWrapEl.dataset.lazyBinded = '1'
    scrollWrapEl.addEventListener('scroll', onTableScroll, { passive: true })
  }
}

function onTableScroll() {
  // Defer to the next frame so the threshold is computed against the scroll
  // height AFTER any just-triggered batch has been added to the DOM.
  requestAnimationFrame(loadMoreIfNeeded)
}

function loadMoreIfNeeded() {
  if (!scrollWrapEl) return
  const { scrollTop, clientHeight, scrollHeight } = scrollWrapEl
  if (scrollHeight - scrollTop - clientHeight < NEAR_BOTTOM_PX) {
    visibleCount.value += 1
  }
}

// Reset to a fresh first batch whenever the underlying dataset or the filter
// changes, so the table always starts at the top again.
watch(filteredFiles, () => {
  visibleCount.value = INITIAL_PAGES
  // The scroll wrap survives data swaps, but the Keyed locale swap below
  // remounts el-table and its scrollbar, so re-resolve the listener target.
  nextTick(bindTableScroll)
})

// --- Quick locate (issue #700) ----------------------------------------------
// Pressing a letter key while the list has focus jumps to the first entry whose
// name starts with that letter (case-insensitive, dirs listed first), like
// Windows Explorer. Pressing the same letter again cycles to the next match.
const listRootRef = ref<HTMLElement | null>(null)
const quickTargetName = ref<string | null>(null)
let quickKey = ''
let quickIndex = -1
let quickTimer: ReturnType<typeof setTimeout> | null = null
const QUICK_RESET_MS = 1200

function onListClick(e: MouseEvent) {
  // Focus the list on click so letter keys locate within it, but only when the
  // click lands on the table itself (not the filter/breadcrumb/clipboard bars
  // or other controls), so we never steal focus while the user is editing.
  const t = e.target as HTMLElement
  if (t.closest && !t.closest('.table-wrapper')) return
  listRootRef.value?.focus({ preventScroll: true })
}

function onListKeydown(e: KeyboardEvent) {
  const t = e.target as HTMLElement | null
  // Never hijack typing that is going somewhere else (filter box, editors).
  if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return
  if (e.ctrlKey || e.metaKey || e.altKey) return
  const k = e.key
  if (k.length !== 1 || !/[\x00-\x7F]/.test(k)) return // printable single char only
  e.preventDefault()
  locateByKey(k.toLowerCase())
}

function locateByKey(key: string) {
  // Candidate indices in display order (skip the '..' row).
  const matches: number[] = []
  filteredFiles.value.forEach((f, i) => {
    if (f.name !== '..' && f.name.toLowerCase().startsWith(key)) matches.push(i)
  })
  if (!matches.length) return

  if (quickKey !== key) {
    // New letter: start from the first match.
    quickKey = key
    quickIndex = matches[0]
  } else {
    // Same letter again: cycle to the next match.
    const pos = matches.indexOf(quickIndex)
    quickIndex = pos >= 0 ? matches[(pos + 1) % matches.length] : matches[0]
  }

  // Reset the "repeated same key" state after a short pause.
  if (quickTimer) clearTimeout(quickTimer)
  quickTimer = setTimeout(() => {
    quickKey = ''
    quickIndex = -1
    quickTargetName.value = null
  }, QUICK_RESET_MS)

  scrollToIndex(quickIndex)
}

function scrollToIndex(index: number) {
  const target = filteredFiles.value[index]
  if (!target) return
  // Make sure the row is actually rendered (progressive loading).
  const pages = Math.ceil((index + 1) / PAGE_SIZE)
  if (visibleCount.value < pages) visibleCount.value = pages
  // Select it (reuses the existing selection highlight) and flag it briefly.
  selectedItems.value = [target]
  quickTargetName.value = target.name
  nextTick(() => {
    const el = (tableRef.value?.$el ?? null) as HTMLElement | null
    const tr = el?.querySelectorAll('.el-table__body tr')[index] as HTMLElement | undefined
    tr?.scrollIntoView({ block: 'nearest' })
  })
}

// <el-table :key="locale"> remounts the table on locale switch, creating a new
// scrollbar element that needs the scroll listener re-attached.
watch(locale, () => {
  visibleCount.value = INITIAL_PAGES
  nextTick(bindTableScroll)
})

onMounted(() => {
  nextTick(bindTableScroll)
})

function isSelected(row: FileItem): boolean {
  return selectedItems.value.some(s => s.name === row.name)
}

function isSymlink(row: FileItem): boolean {
  return row.mode.startsWith('L') || row.mode.startsWith('l')
}

function formatDate(ts: string): string {
  if (!ts) return '-'
  const d = new Date(ts)
  return d.toLocaleString()
}

function sortByName(a: FileItem, b: FileItem): number {
  if (a.name === '..') return -1
  if (b.name === '..') return 1
  return a.name.localeCompare(b.name)
}

function sortByTime(a: FileItem, b: FileItem): number {
  if (a.name === '..') return -1
  if (b.name === '..') return 1
  const ta = a.modTime ? new Date(a.modTime).getTime() : 0
  const tb = b.modTime ? new Date(b.modTime).getTime() : 0
  return ta - tb
}

function sortBySize(a: FileItem, b: FileItem): number {
  if (a.name === '..') return -1
  if (b.name === '..') return 1
  if (a.isDir && !b.isDir) return -1
  if (!a.isDir && b.isDir) return 1
  return a.size - b.size
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB'
}

// File type column: based purely on the filename suffix (no MIME detection).
// Directories and symlinks have no suffix, so label them explicitly. The '..'
// row has no type.
function fileTypeLabel(row: FileItem): string {
  if (row.name === '..') return ''
  if (isSymlink(row)) return t('sftp.link')
  if (row.isDir) return t('sftp.folder')
  const dot = row.name.lastIndexOf('.')
  if (dot <= 0 || dot === row.name.length - 1) return '-'
  return row.name.slice(dot + 1).toLowerCase()
}

function sortByType(a: FileItem, b: FileItem): number {
  return fileTypeLabel(a).toLowerCase().localeCompare(fileTypeLabel(b).toLowerCase())
}

function onRowClick(row: FileItem, _column: any, event: MouseEvent) {
  // A rubber band that starts and ends on the same row still produces a click
  // event after its mouseup; never let that click clobber the band's result.
  if (bandJustEnded) {
    bandJustEnded = false
    return
  }
  const index = filteredFiles.value.findIndex(f => f.name === row.name)
  if (event.ctrlKey || event.metaKey) {
    const idx = selectedItems.value.findIndex(s => s.name === row.name)
    if (idx >= 0) {
      selectedItems.value.splice(idx, 1)
    } else {
      selectedItems.value.push(row)
    }
  } else if (event.shiftKey && lastClickedIndex.value >= 0) {
    const start = Math.min(lastClickedIndex.value, index)
    const end = Math.max(lastClickedIndex.value, index)
    selectedItems.value = filteredFiles.value.slice(start, end + 1)
  } else {
    selectedItems.value = [row]
    lastClickedIndex.value = index
  }
}

function onRowDblClick(row: FileItem) {
  if (row.name === '..') {
    emit('navigate', '..')
    return
  }
  if (row.isDir) {
    emit('navigate', row.name)
  } else {
    emit('open', row)
  }
}

function onRowContextMenu(row: FileItem, _column: any, event: MouseEvent) {
  if (row.name === '..') {
    // Show empty area menu for parent directory
    event.preventDefault()
    event.stopPropagation()
    selectedItems.value = []
    menuType.value = 'empty'
    ctxMenuRef.value?.openAt(event.clientX, event.clientY)
    return
  }
  event.preventDefault()
  event.stopPropagation()
  if (!selectedItems.value.some(s => s.name === row.name)) {
    selectedItems.value = [row]
  }
  if (selectedItems.value.length > 1) {
    menuType.value = 'batch'
  } else if (selectedItems.value[0]?.isDir) {
    menuType.value = 'dir'
  } else {
    menuType.value = 'file'
  }
  ctxMenuRef.value?.openAt(event.clientX, event.clientY)
}

function onEmptyAreaContextMenu(event: MouseEvent, force = false) {
  const target = event.target as HTMLElement
  // Only show empty menu if not clicking on a row (unless forced)
  if (!force && target.closest('tr')) return
  event.stopPropagation()
  selectedItems.value = []
  menuType.value = 'empty'
  ctxMenuRef.value?.openAt(event.clientX, event.clientY)
}

function doSendToOther() { emit('sendToOther', [...selectedItems.value]); ctxMenuVisible.value = false }
function doDownloadTo() { emit('downloadTo', [...selectedItems.value]); ctxMenuVisible.value = false }

// "Copy path" actions: one full path per selected entry ('..' excluded),
// joined by newlines so a multi-selection pastes as a path list.
function buildSelectedPathsText(): string {
  const items = selectedItems.value.filter(i => i.name !== '..')
  if (!items.length) return ''
  const base = props.breadcrumbPath || ''
  return items.map(i => base ? joinPath(base, i.name) : i.name).join('\n')
}

async function doCopyPath() {
  ctxMenuVisible.value = false
  const text = buildSelectedPathsText()
  if (!text) return
  await navigator.clipboard.writeText(text).catch(() => {})
  msg.success(t('sftp.pathCopied'))
}

async function doCopyPathToTerminal() {
  ctxMenuVisible.value = false
  const text = buildSelectedPathsText()
  if (!text) return
  await navigator.clipboard.writeText(text).catch(() => {})
  // The host types the text at its terminal's prompt (no trailing newline);
  // silently no-ops when no terminal panel exists for this session.
  emit('copyPathToTerminal', text)
}

function doRename() { emit('rename', selectedItems.value[0]); ctxMenuVisible.value = false }
function doDelete() { emit('delete', [...selectedItems.value]); ctxMenuVisible.value = false }
function doChmod() { emit('chmod', selectedItems.value[0]); ctxMenuVisible.value = false }
function doEdit() { emit('edit', selectedItems.value[0]); ctxMenuVisible.value = false }
function doEditExternal() { emit('editExternal', selectedItems.value[0]); ctxMenuVisible.value = false }
function doNewFile() { emit('newFile'); ctxMenuVisible.value = false; moreMenuVisible.value = false }
function doMkdir() { emit('mkdir'); ctxMenuVisible.value = false; moreMenuVisible.value = false }
function doSymlink() { emit('symlink'); ctxMenuVisible.value = false; moreMenuVisible.value = false }
function toggleShowHidden() { showHidden.value = !showHidden.value }
function doCopyToClipboard() { emit('copyToClipboard', [...selectedItems.value]); ctxMenuVisible.value = false }
function doCutToClipboard() { emit('cutToClipboard', [...selectedItems.value]); ctxMenuVisible.value = false }
function doPaste() { emit('paste'); ctxMenuVisible.value = false }

function getRowClassName({ row }: { row: FileItem }): string {
  const cls: string[] = []
  if (props.cutItemNames && props.cutItemNames.includes(row.name)) cls.push('cut-item-row')
  if (row.name === quickTargetName.value) cls.push('quick-target-row')
  if (isSelected(row)) cls.push('row-selected')
  return cls.join(' ')
}

function onDragStart(event: DragEvent, row: FileItem) {
  if (event.dataTransfer) {
    // Carry the whole selection when the dragged row is part of it, so
    // ctrl/shift multi-selection drags up/ down more than one item.
    const inSelection = selectedItems.value.some(s => s.name === row.name)
    const dragged = (inSelection ? selectedItems.value : [row]).filter(i => i.name !== '..')
    event.dataTransfer.setData('application/sftp-file', JSON.stringify({
      mode: props.mode,
      items: dragged.map(i => ({ name: i.name, isDir: i.isDir }))
    }))
  }
}

// --- Rubber-band selection (drag to select) ---------------------------------
// Press the left button anywhere in the table body (a non-name cell or empty
// space) and drag: a translucent band follows the cursor and every rendered
// row it intersects is added to the selection, unioned with what was already
// selected before the drag. A press below the 4px drag threshold is a plain
// click and keeps the normal row-click / empty-area behavior.

const bandRect = ref<{ x: number; y: number; w: number; h: number } | null>(null)
const BAND_THRESHOLD = 4
let bandStart: { x: number; y: number } | null = null
let bandWrapper: HTMLElement | null = null
let bandBaseSelection: FileItem[] = []
// Row elements are cached once per gesture; their rects are still re-measured
// on every move so a wheel scroll mid-drag stays correct.
let bandRows: HTMLElement[] = []
let bandCleanup: (() => void) | null = null
let bandDownOnRow = false
let bandAdditive = false
let bandJustEnded = false
let bandPrevUserSelect: string | null = null

onBeforeUnmount(() => {
  // Never leak the document-level listeners if the component unmounts
  // mid-drag (e.g. the host switches tabs while the button is held).
  bandCleanup?.()
  bandCleanup = null
})

function onTableMouseDown(e: MouseEvent) {
  bandJustEnded = false
  if (bandCleanup) return
  // Ctrl/Cmd starts an ADDITIVE band (existing selection kept, swept rows are
  // added — Windows Explorer semantics); a plain band REPLACES the selection.
  // Shift stays reserved for the row-click range toggle.
  bandAdditive = e.ctrlKey || e.metaKey
  if (e.button !== 0 || e.shiftKey) return
  if (props.loading || props.pasteLoading) return
  const t = e.target as HTMLElement
  if (!t.closest) return
  // Keep interactive controls and the custom scrollbar gestures intact.
  if (t.closest('input, textarea, button, .el-checkbox, .el-scrollbar__bar')) return
  // Keep header sort / column-resize gestures intact.
  if (t.closest('th')) return
  // The name cell owns the native HTML5 row drag (the application/sftp-file
  // payload dragged between panes). A band starting there would race the
  // native drag, so band-dragging starts on any other cell or empty space.
  if (t.closest('.name-cell')) return
  const wrap = scrollWrapEl
  if (!wrap) return

  const wrapper = e.currentTarget as HTMLElement
  bandStart = { x: e.clientX, y: e.clientY }
  bandWrapper = wrapper
  bandBaseSelection = selectedItems.value
  bandDownOnRow = !!t.closest('tr')
  bandRows = Array.from(wrapper.querySelectorAll<HTMLElement>('.el-table__body tr'))

  const onMove = (ev: MouseEvent) => {
    if (!bandStart || !bandWrapper) return
    const dx = ev.clientX - bandStart.x
    const dy = ev.clientY - bandStart.y
    if (!bandRect.value && Math.abs(dx) < BAND_THRESHOLD && Math.abs(dy) < BAND_THRESHOLD) return
    if (bandPrevUserSelect === null) {
      // Suppress native text selection while the band sweeps the rows.
      bandPrevUserSelect = document.body.style.userSelect
      document.body.style.userSelect = 'none'
    }
    // The overlay is anchored to .table-wrapper (its positioning context) and
    // nothing scrolls under the band during a drag, so viewport-relative
    // coordinates measured against the wrapper are used for both the overlay
    // and the row intersection test.
    const baseRect = bandWrapper.getBoundingClientRect()
    const x0 = Math.min(bandStart.x, ev.clientX)
    const y0 = Math.min(bandStart.y, ev.clientY)
    bandRect.value = {
      x: x0 - baseRect.left,
      y: y0 - baseRect.top,
      w: Math.abs(dx),
      h: Math.abs(dy),
    }
    applyBandSelection()
  }

  const onUp = () => {
    bandCleanup?.()
    bandCleanup = null
    const wasBand = !!bandRect.value
    bandRect.value = null
    bandStart = null
    bandWrapper = null
    bandRows = []
    bandJustEnded = wasBand
    if (!wasBand && !bandDownOnRow && !additive) {
      // Plain click on empty space clears the selection; on a row the normal
      // row-click handler takes over. Ctrl/Cmd clicks on empty space keep it.
      selectedItems.value = []
    }
  }

  bandCleanup = () => {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    if (bandPrevUserSelect !== null) {
      document.body.style.userSelect = bandPrevUserSelect
      bandPrevUserSelect = null
    }
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

function applyBandSelection() {
  const r = bandRect.value
  if (!bandWrapper || !r) return
  const baseRect = bandWrapper.getBoundingClientRect()
  const sel: FileItem[] = []
  const selNames = new Set<string>()
  bandRows.forEach(rowEl => {
    const rr = rowEl.getBoundingClientRect()
    const ry = rr.top - baseRect.top
    if (ry >= r.y + r.h || ry + rr.height <= r.y) return
    // Resolve the row through its file name rather than its DOM index, so the
    // mapping stays correct even if el-table re-orders rows after a header sort.
    const name = rowEl.querySelector('.file-name')?.textContent?.trim()
    if (!name || name === '..') return // '..' is navigation, never selectable
    if (selNames.has(name)) return
    const item = visibleFiles.value.find(f => f.name === name)
    if (item) {
      selNames.add(name)
      sel.push(item)
    }
  })
  // Windows Explorer semantics: a plain band REPLACES the selection; a
  // Ctrl/Cmd band adds the swept rows to whatever was already selected.
  selectedItems.value = bandAdditive
    ? [...bandBaseSelection.filter(b => !selNames.has(b.name)), ...sel]
    : sel
  if (sel.length) {
    const lastName = sel[sel.length - 1].name
    lastClickedIndex.value = filteredFiles.value.findIndex(f => f.name === lastName)
  }
}
</script>

<style scoped>
.sftp-file-list {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}
/* The list root is focusable so letter keys can quick-locate; hide the ring. */
.sftp-file-list:focus {
  outline: none;
}
/* Brief highlight shown while a quick-located row is in view (issue #700). */
:deep(.quick-target-row) td {
  background-color: rgba(var(--color-primary, 64, 158, 255), 0.14);
}
/* Full-row background for every row in the current selection. */
:deep(.row-selected) td {
  background-color: var(--accent-subtle) !important;
}
/* Non-name columns read dimmer than the file name (issue #702). */
.cell-secondary {
  color: var(--el-text-color-secondary, #909399);
}
/* Keep the `border` prop on el-table (column drag-resize needs it) but hide the
   visible vertical lines only on the data rows, keeping the header's. */
:deep(.el-table--border .el-table__body .el-table__cell) {
  border-right: none;
}
.filter-bar {
  display: flex;
  align-items: center;
  gap: 2px;
  padding-top: 0;
  padding-left: 10px;
  padding-right: 10px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--border-subtle);
}
.filter-bar .el-input {
  flex: 1;
}
/* Vertical separator between flat-toolbar button groups (nav / view /
   transfer / create). Compact (sidebar) layout has no groups and no dividers. */
.toolbar-divider {
  width: 1px;
  height: 16px;
  margin: 0 3px;
  flex-shrink: 0;
  background: var(--border-subtle);
}
/* Match the sidebar's tab / close icon-button style (transparent, 26px, muted) */
.filter-icon-btn {
  width: 26px;
  height: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.12s ease;
}
.filter-icon-btn:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}
.filter-icon-btn:disabled {
  opacity: 0.4;
  cursor: default;
  background: transparent;
  color: var(--text-muted);
}
.filter-icon-btn.active {
  color: var(--accent);
  background: var(--accent-subtle);
}
.clipboard-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--border-subtle);
  font-size: 12px;
}
.clipboard-info {
  flex: 1;
  color: var(--text-secondary);
}
.selection-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-top: 1px solid var(--border-subtle);
  font-size: 12px;
  color: var(--text-secondary);
}
.selection-info {
  flex: 1;
}
.band-rect {
  position: absolute;
  z-index: 20;
  border: 1px solid var(--accent);
  background: color-mix(in srgb, var(--accent) 15%, transparent);
  pointer-events: none;
}
.name-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}
.name-info {
  display: flex;
  flex-direction: column;
}
.file-name {
  color: var(--text-primary);
}
.file-name.selected {
  color: var(--accent);
}
.file-mode {
  font-size: 11px;
  color: var(--text-disabled);
}

</style>

<style>
/* Custom loading overlay */
.table-wrapper {
  flex: 1;
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.loading-overlay {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--scrim);
}
.loading-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.loading-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid rgba(255, 255, 255, 0.15);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
.loading-text {
  font-size: 12px;
  color: var(--text-primary);
}

/* Remove horizontal borders between data rows (keep header border) */
.sftp-file-list .el-table__inner-wrapper::before {
  height: 0 !important;
}
.sftp-file-list .el-table td.el-table__cell {
  border-bottom: none !important;
}

/* Make table fill entire pane with consistent background */
.sftp-file-list .el-table__body-wrapper {
  background: transparent;
}
.sftp-file-list .el-table__empty-block,
.sftp-file-list .el-table__empty-text {
  background: transparent;
}

/* Override ElMessage popup to match dark theme */
.el-message {
  background: var(--bg-surface) !important;
  border: 1px solid var(--border-subtle) !important;
  box-shadow: var(--shadow-md) !important;
}
.el-message .el-message__content {
  color: var(--text-primary) !important;
}
.el-message--error {
  background: var(--bg-surface) !important;
}
.el-message--error .el-message__content {
  color: var(--error) !important;
}
.el-table tr.cut-item-row {
  opacity: 0.4;
}

/* Make table fill pane and keep scrollbar at bottom */
.sftp-file-list .table-wrapper .el-table {
  height: 100%;
}
</style>
