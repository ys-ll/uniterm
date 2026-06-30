<template>
  <div
    ref="breadcrumbRef"
    class="sftp-breadcrumb"
    @click.self="startEdit"
  >
    <template v-if="editing">
      <span v-if="label" class="breadcrumb-label">{{ label }}</span>
      <el-input
        ref="inputRef"
        v-model="pathInput"
       
        class="path-input"
        @keyup.enter="commitEdit"
        @blur="commitEdit"
        @keyup.escape="cancelEdit"
      />
    </template>
    <template v-else>
      <span v-if="label" class="breadcrumb-label">{{ label }}</span>
      <template v-for="(item, idx) in visibleParts" :key="idx">
        <span
          v-if="item === '...'"
          class="breadcrumb-part breadcrumb-ellipsis"
          @click.stop="onEllipsisClick"
        >...</span>
        <span
          v-else-if="isWindowsPath && item === pathParts[0]"
          class="breadcrumb-part breadcrumb-drive"
          @click.stop="toggleDriveMenu"
        >
          {{ item }}
          <span class="drive-arrow">&#9660;</span>
        </span>
        <span
          v-else
          class="breadcrumb-part"
          @click="onBreadcrumbClick(item)"
        >
          {{ item }}
        </span>
        <span v-if="idx < visibleParts.length - 1" class="separator" @click.stop>&gt;</span>
      </template>
      <button
        v-if="bookmarkMode"
        ref="bookmarkBtnRef"
        class="bookmark-btn"
        :title="t('sftp.bookmark.title')"
        @click.stop="toggleBookmarkMenu"
      >
        <Bookmark :size="14" :class="{ 'bookmark-active': hasCurrentPathBookmarked }" />
      </button>
    </template>

    <!-- Drive dropdown -->
    <Teleport to="body">
      <div
        v-show="driveMenuVisible"
        class="drive-dropdown"
        :style="driveMenuStyle"
        @click.stop
        @mousedown.stop
      >
        <div
          v-for="drive in drives"
          :key="drive"
          class="drive-item"
          :class="{ active: drive === currentDrive }"
          @click="onDriveSelect(drive)"
        >
          {{ drive }}
        </div>
      </div>
    </Teleport>

    <!-- Bookmark dropdown -->
    <Teleport to="body">
      <div
        v-show="bookmarkMenuVisible"
        class="bookmark-dropdown"
        :style="bookmarkMenuStyle"
        @click.stop
        @mousedown.stop
      >
        <div
          v-if="!hasCurrentPathBookmarked"
          class="bookmark-item bookmark-save"
          @click="onSaveBookmark"
        >
          <BookmarkPlus :size="14" />
          <span>{{ t('sftp.bookmark.saveCurrent') }}</span>
        </div>
        <div
          v-else
          class="bookmark-item bookmark-saved-hint"
        >
          <BookmarkCheck :size="14" />
          <span>{{ t('sftp.bookmark.saved') }}</span>
        </div>
        <div v-if="savedPaths.length > 0" class="bookmark-divider"></div>
        <div
          v-for="(savedPath, idx) in savedPaths"
          :key="idx"
          class="bookmark-item bookmark-path-item"
          :class="{ active: savedPath === currentPath }"
          @click="onBookmarkClick(savedPath)"
          @mouseenter="hoveredBookmarkIdx = idx"
          @mouseleave="hoveredBookmarkIdx = -1"
        >
          <span class="bookmark-path-text" :title="savedPath">{{ savedPath }}</span>
          <button
            v-show="hoveredBookmarkIdx === idx"
            class="bookmark-remove-btn"
            @click.stop="onRemoveBookmark(savedPath)"
            :title="t('sftp.bookmark.remove')"
          >
            <Trash2 :size="12" />
          </button>
        </div>
        <div
          v-if="savedPaths.length === 0"
          class="bookmark-item bookmark-empty"
        >
          {{ t('sftp.bookmark.empty') }}
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, nextTick, watch, onMounted, onUnmounted } from 'vue'
import { Bookmark, BookmarkPlus, BookmarkCheck, Trash2 } from '@lucide/vue'
import { useI18n } from '../i18n'

const { t } = useI18n()

const props = defineProps<{
  path: string
  label?: string
  drives?: string[]
  savedPaths?: string[]
  bookmarkMode?: 'local' | 'remote'
}>()

const emit = defineEmits<{
  navigate: [path: string]
  saveBookmark: [path: string]
  removeBookmark: [path: string]
}>()

const isWindowsPath = computed(() => {
  return /^[A-Za-z]:[\\\/]/.test(props.path)
})

const currentDrive = computed(() => {
  if (!isWindowsPath.value) return ''
  const match = props.path.match(/^([A-Za-z]:)/)
  return match ? match[1] + '\\' : ''
})

const pathParts = computed(() => {
  if (isWindowsPath.value) {
    const clean = props.path.replace(/\\/g, '/')
    const parts = clean.split('/').filter(Boolean)
    return parts
  }

  const clean = props.path.replace(/\\/g, '/')
  if (!clean || clean === '/') return ['/']
  const parts = clean.split('/').filter(Boolean)
  return ['/', ...parts]
})

// Overflow collapse
const containerWidth = ref(0)
const collapsedCount = ref(0)
const breadcrumbRef = ref<HTMLElement>()

const visibleParts = computed(() => {
  const parts = pathParts.value
  if (collapsedCount.value <= 0 || parts.length <= 2) return [...parts]
  const hidden = Math.min(collapsedCount.value, parts.length - 2)
  return [parts[0], '...', ...parts.slice(1 + hidden)]
})

let resizeObserver: ResizeObserver | null = null

function recalcOverflow() {
  nextTick(() => {
    const el = breadcrumbRef.value
    if (!el) return
    const maxCollapse = Math.max(0, pathParts.value.length - 2)
    // Only ever collapse more within a layout pass. Expanding is handled by
    // resetting collapsedCount to 0 on path/size change. Mixing increment and
    // decrement here causes an infinite flip-flop when a path sits exactly at
    // the overflow boundary (e.g. a single very long segment), freezing the UI.
    if (el.scrollWidth > el.clientWidth && collapsedCount.value < maxCollapse) {
      collapsedCount.value++
    }
  })
}

watch(() => props.path, () => {
  collapsedCount.value = 0
  recalcOverflow()
})

watch(containerWidth, () => {
  // Recompute from scratch so a wider container can re-expand collapsed parts.
  collapsedCount.value = 0
  recalcOverflow()
})

watch(collapsedCount, () => {
  recalcOverflow()
})

onMounted(() => {
  if (breadcrumbRef.value) {
    resizeObserver = new ResizeObserver((entries) => {
      for (const entry of entries) {
        containerWidth.value = entry.contentRect.width
      }
    })
    resizeObserver.observe(breadcrumbRef.value)
  }
})

onUnmounted(() => {
  resizeObserver?.disconnect()
})

// Path edit mode
const editing = ref(false)
const pathInput = ref('')
const inputRef = ref()

function startEdit() {
  // Build current path string from parts
  if (isWindowsPath.value) {
    pathInput.value = pathParts.value.join('\\')
    if (pathParts.value.length === 1 && /^[A-Za-z]:$/.test(pathParts.value[0])) {
      pathInput.value += '\\'
    }
  } else {
    pathInput.value = '/' + pathParts.value.slice(1).join('/')
    if (pathInput.value === '') pathInput.value = '/'
  }
  editing.value = true
  nextTick(() => {
    inputRef.value?.focus()
  })
}

function commitEdit() {
  if (!editing.value) return
  editing.value = false
  const val = pathInput.value.trim()
  if (val && val !== props.path) {
    emit('navigate', val)
  }
}

function cancelEdit() {
  editing.value = false
}

// Drive menu
const driveMenuVisible = ref(false)
const driveMenuStyle = ref({ left: '0px', top: '0px' })

function toggleDriveMenu(event?: MouseEvent) {
  if (driveMenuVisible.value) {
    driveMenuVisible.value = false
    return
  }
  if (event) {
    const rect = (event.target as HTMLElement).getBoundingClientRect()
    driveMenuStyle.value = {
      left: rect.left + 'px',
      top: (rect.bottom + 4) + 'px'
    }
  }
  closeDriveMenu()
  driveMenuVisible.value = true
  nextTick(() => {
    document.addEventListener('mousedown', closeDriveMenu, { once: true })
  })
}

function closeDriveMenu() {
  driveMenuVisible.value = false
}

function onGlobalContextMenu(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest('.sftp-breadcrumb')) {
    closeDriveMenu()
  }
}

onMounted(() => {
  document.addEventListener('contextmenu', onGlobalContextMenu)
  document.addEventListener('contextmenu', onGlobalBookmarkContextMenu)
})

onUnmounted(() => {
  document.removeEventListener('contextmenu', onGlobalContextMenu)
  document.removeEventListener('contextmenu', onGlobalBookmarkContextMenu)
  document.removeEventListener('mousedown', closeDriveMenu)
  document.removeEventListener('mousedown', closeBookmarkMenu)
})

function onDriveSelect(drive: string) {
  closeDriveMenu()
  emit('navigate', drive)
}

function onEllipsisClick() {
  const parts = pathParts.value
  const lastHidden = collapsedCount.value
  const selected = parts.slice(0, lastHidden + 1)
  if (isWindowsPath.value) {
    let target = selected.join('\\')
    emit('navigate', target)
  } else {
    let target = selected.join('/').replace(/\/+/g, '/')
    if (!target.startsWith('/')) target = '/' + target
    emit('navigate', target)
  }
}

function onBreadcrumbClick(part: string) {
  if (part === '...') return
  const parts = pathParts.value
  const index = parts.indexOf(part)
  if (index < 0) return
  if (isWindowsPath.value && index === 0) return // handled by dropdown

  const selected = parts.slice(0, index + 1)

  if (isWindowsPath.value) {
    let target = selected.join('\\')
    if (selected.length === 1 && /^[A-Za-z]:$/.test(selected[0])) {
      target += '\\'
    }
    emit('navigate', target)
    return
  }

  let target = selected.join('/').replace(/\/+/g, '/')
  if (!target.startsWith('/')) target = '/' + target
  emit('navigate', target)
}

// Bookmark menu
const bookmarkMenuVisible = ref(false)
const bookmarkMenuStyle = ref({ left: '0px', top: '0px' })
const bookmarkBtnRef = ref<HTMLElement>()
const hoveredBookmarkIdx = ref(-1)

const currentPath = computed(() => props.path)
const hasCurrentPathBookmarked = computed(() => {
  return (props.savedPaths || []).includes(props.path)
})

function toggleBookmarkMenu(event?: MouseEvent) {
  if (bookmarkMenuVisible.value) {
    bookmarkMenuVisible.value = false
    return
  }
  if (event) {
    const rect = (event.target as HTMLElement).closest('.bookmark-btn')?.getBoundingClientRect()
    if (rect) {
      bookmarkMenuStyle.value = {
        left: (rect.right - 200) + 'px',
        top: (rect.bottom + 4) + 'px'
      }
    }
  }
  closeBookmarkMenu()
  bookmarkMenuVisible.value = true
  nextTick(() => {
    document.addEventListener('mousedown', closeBookmarkMenu, { once: true })
  })
}

function closeBookmarkMenu() {
  bookmarkMenuVisible.value = false
  hoveredBookmarkIdx.value = -1
}

function onGlobalBookmarkContextMenu(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest('.sftp-breadcrumb')) {
    closeBookmarkMenu()
  }
}

function onSaveBookmark() {
  emit('saveBookmark', props.path)
  bookmarkMenuVisible.value = false
}

function onRemoveBookmark(path: string) {
  emit('removeBookmark', path)
}

function onBookmarkClick(path: string) {
  closeBookmarkMenu()
  if (path !== props.path) {
    emit('navigate', path)
  }
}
</script>

<style scoped>
.sftp-breadcrumb {
  display: flex;
  align-items: center;
  padding: 4px 12px;
  font-size: 12px;
  font-family: var(--font-mono);
  color: var(--text-primary);
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--border-subtle);
  overflow: hidden;
  white-space: nowrap;
}
.path-input {
  flex: 1;
}
.breadcrumb-part {
  cursor: pointer;
  padding: 2px 4px;
  border-radius: var(--radius-sm);
  transition: all 0.1s ease;
  flex-shrink: 0;
}
.breadcrumb-part:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.breadcrumb-drive {
  cursor: pointer;
  user-select: none;
}
.breadcrumb-ellipsis {
  color: var(--text-disabled);
  cursor: pointer;
  padding: 2px 6px;
}
.drive-arrow {
  font-size: 8px;
  margin-left: 4px;
  color: var(--text-disabled);
}
.breadcrumb-label {
  color: var(--accent);
  font-weight: 600;
  margin-right: 8px;
  flex-shrink: 0;
}
.separator {
  color: var(--text-disabled);
  margin: 0 2px;
  flex-shrink: 0;
}
.bookmark-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  margin-left: auto;
  flex-shrink: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  transition: all 0.1s ease;
}
.bookmark-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.bookmark-btn .bookmark-active {
  color: var(--accent);
}
</style>

<style>
.drive-dropdown {
  position: fixed;
  z-index: 99999;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
  min-width: 80px;
  padding: 4px;
}
.drive-item {
  padding: 5px 10px;
  font-size: 12px;
  font-family: var(--font-mono);
  cursor: pointer;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
}
.drive-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.drive-item.active {
  color: var(--accent);
}

.bookmark-dropdown {
  position: fixed;
  z-index: 99999;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
  min-width: 200px;
  max-width: 320px;
  padding: 4px;
}
.bookmark-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 10px;
  font-size: 12px;
  font-family: var(--font-mono);
  cursor: pointer;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
}
.bookmark-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.bookmark-item.active {
  color: var(--accent);
}
.bookmark-save,
.bookmark-saved-hint {
  color: var(--accent);
  font-family: var(--font-ui);
}
.bookmark-path-item {
  justify-content: space-between;
}
.bookmark-path-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}
.bookmark-remove-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  flex-shrink: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  padding: 0;
}
.bookmark-remove-btn:hover {
  background: var(--bg-hover);
  color: var(--error);
}
.bookmark-divider {
  height: 1px;
  background: var(--border-subtle);
  margin: 4px 6px;
}
.bookmark-empty {
  font-family: var(--font-ui);
  color: var(--text-disabled);
  cursor: default;
}
</style>
