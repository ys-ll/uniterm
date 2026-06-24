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
    </template>

    <!-- Drive dropdown -->
    <Teleport to="body">
      <div
        v-show="driveMenuVisible"
        class="drive-dropdown"
        :style="driveMenuStyle"
        @click.stop
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
  </div>
</template>

<script setup lang="ts">
import { computed, ref, nextTick, watch, onMounted, onUnmounted } from 'vue'

const props = defineProps<{
  path: string
  label?: string
  drives?: string[]
}>()

const emit = defineEmits<{
  navigate: [path: string]
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
    if (el.scrollWidth > el.clientWidth) {
      if (collapsedCount.value < maxCollapse) {
        collapsedCount.value++
      }
    } else if (collapsedCount.value > 0) {
      collapsedCount.value--
    }
  })
}

watch(() => props.path, () => {
  collapsedCount.value = 0
  recalcOverflow()
})

watch(containerWidth, () => {
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
})

onUnmounted(() => {
  document.removeEventListener('contextmenu', onGlobalContextMenu)
  document.removeEventListener('mousedown', closeDriveMenu)
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
</style>
