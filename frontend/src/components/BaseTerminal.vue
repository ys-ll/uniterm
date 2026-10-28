<template>
  <div
    class="base-terminal"
    :data-terminal-session-id="props.sessionId"
    :id="terminalDropId"
    data-file-drop-target
    @dragover.prevent="overlayRef?.onDragOver"
    @dragenter.prevent="overlayRef?.onDragEnter"
    @dragleave="overlayRef?.onDragLeave"
    @drop="overlayRef?.onDragDrop"
  >
    <div class="terminal-area" @contextmenu="onTerminalContextMenu">
      <TerminalGutter
        ref="gutterRef"
        :session-id="props.sessionId"
        :show-line-numbers="showLineNumbers"
        :show-timestamps="showTimestamps"
        :get-host="getTerminalHost"
      />
      <div ref="terminalRef" class="terminal-host"></div>
    </div>

    <TerminalOverlay ref="overlayRef" />
    <TerminalSearch ref="searchRef" :session-id="props.sessionId" />
    <TerminalToolbar
      ref="toolbarRef"
      :menu="menu"
      :menu-shortcut="menuShortcut"
      :show-line-numbers="showLineNumbers"
      :show-timestamps="showTimestamps"
      :is-ssh="isSsh"
      :supports-output-log="supportsOutputLog"
      :is-output-log-on="isOutputLogOn"
      :panel="panel"
      @toggle-line-numbers="toggleLineNumbers"
      @toggle-timestamps="toggleTimestamps"
      @trigger-search="triggerSearch"
      @export="recorder.exportContent"
      @reset-output="recorder.resetTerminalOutput"
      @clear-scrollback="recorder.clearScrollback"
      @toggle-output-log="recorder.toggleOutputLog"
      @open-log-dir="recorder.openLogDir"
      @open-sftp="openSftp"
      @upload-file-rz="uploadFileRz"
      @open-monitor="openMonitor"
    />

    <!-- Terminal suggestions popup -->
    <TerminalSuggestion
      :visible="suggestions.state.value.visible"
      :items="suggestions.state.value.items"
      :selected-index="suggestions.state.value.selectedIndex"
      :cursor-x="terminalInput?.cursorPixelPos.value.x ?? 0"
      :cursor-y="terminalInput?.cursorPixelPos.value.y ?? 0"
      @select="(idx: number) => applySuggestion(suggestions.state.value.items[idx])"
      @hover="(idx: number) => suggestions.state.value.selectedIndex = idx"
      @remove="(id: string) => suggestions.removeHistoryCommandById(id)"
    />

    <!-- Zmodem transfer panel -->
    <ZmodemTransfer :session-id="props.sessionId || ''" @cancel="zmodem.onZmodemCancel" />

    <!-- Screen preview on scrollbar hover (issue #729) -->
    <TerminalScreenPreview :session-id="props.sessionId || ''" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'

import { useNativeFileDrop } from '../composables/useFilePanel'
import { isMobilePlatform } from '../utils/platform'

import { useSettingsStore } from '../stores/settingsStore'
import { formatKeyBinding } from '../composables/useKeyboardShortcuts'
import type { ShortcutAction } from '../types/settings'
import { usePanelStore } from '../stores/panelStore'
import { useTerminalMenu } from '../composables/useTerminalMenu'

import { getManagedTerminal } from '../services/terminalManager'
import { getTerminalSelection } from '../services/terminalCommandWrap'

import type { useTerminalInput } from '../composables/useTerminalInput'
import { useSuggestions } from '../composables/useSuggestions'
import TerminalSuggestion from './TerminalSuggestion.vue'
import TerminalGutter from './TerminalGutter.vue'
import { useZmodemStore } from '../stores/zmodemStore'
import ZmodemTransfer from './ZmodemTransfer.vue'
import TerminalScreenPreview from './TerminalScreenPreview.vue'
// Extracted sub-components and composables
import TerminalSearch from './terminal/TerminalSearch.vue'
import TerminalOverlay from './terminal/TerminalOverlay.vue'
import TerminalToolbar from './terminal/TerminalToolbar.vue'
import { useTerminalResize } from '../composables/useTerminalResize'
import { useTerminalRecorder } from '../composables/useTerminalRecorder'
import { useTerminalKeyHandler } from '../composables/useTerminalKeyHandler'
import { useTerminalTouchPan } from '../composables/useTerminalTouchPan'
import { useTerminalZmodem } from '../composables/useTerminalZmodem'
import { useTerminalThemeApply } from '../composables/useTerminalThemeApply'
import { useTerminalSessionEvents } from '../composables/useTerminalSessionEvents'
import { useTerminalDataFlow } from '../composables/useTerminalDataFlow'
import { useTerminalRender } from '../composables/useTerminalRender'
import { useTerminalHelpers } from '../composables/useTerminalHelpers'
import { useTerminalLifecycle } from '../composables/useTerminalLifecycle'

const props = defineProps<{
  mode: 'ssh' | 'sftp' | 'local'
  sessionId: string | null | undefined
  onSessionStatus?: (status: string) => void
  broadcastActive?: boolean
  workspaceId?: string
  panelId?: string
}>()

const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent)

const settingsStore = useSettingsStore()

// Human-readable keybinding for a shortcut action ('' when unset), used to show
// the current shortcut hint in the terminal right-click context menu. Reactive
// via settingsStore, so hints update automatically when the user rebinds keys.
function menuShortcut(action: ShortcutAction): string {
  const b = settingsStore.settings.keyboard[action]
  if (!b) return ''
  return formatKeyBinding(b, isMac)
}
const panelStore = usePanelStore()
const zmodemStore = useZmodemStore()


// Prevent deactivated (KeepAlive-cached) components from processing
// terminal events. Only the active component should handle input/output.
const isActive = ref(true)

// Unique ref per BaseTerminal instance, so each instance independently
// contributes to the TerminalManager ref count. Without this, two instances
// rendering the same panel (e.g. KeepAlive'd tab + workspace panel) share
// the same panelId ref, and one release drops the count to zero.
const terminalInstanceRef = crypto.randomUUID?.() ||
  Math.random().toString(36).slice(2, 10) +
  Date.now().toString(36)

const terminalRef = ref<HTMLDivElement>()

// Whether the line-number gutter is enabled (from the persistent setting).
const showLineNumbers = computed(() => settingsStore.settings.terminal.showLineNumbers ?? false)
// Whether the timestamp column is enabled (from the persistent setting).
const showTimestamps = computed(() => settingsStore.settings.terminal.showTimestamps ?? false)

// Write terminal data and fold it into the logical-line registry for the
// gutter's number/time columns: lines that just received characters get the
// next sequential number and the arrival time; lines the cursor moved past

// Pass the xterm host element to the gutter so it can verify the terminal it
// renders is attached to this very component (KeepAlive/drag can move it).
function getTerminalHost(): HTMLElement | null {
  return terminalRef.value ?? null
}

function toggleLineNumbers() {
  menu.closeMenu()
  if (toolbarRef.value) toolbarRef.value.gutterMenuVisible = false
  settingsStore.updateTerminal({ showLineNumbers: !showLineNumbers.value })
}

function toggleTimestamps() {
  menu.closeMenu()
  if (toolbarRef.value) toolbarRef.value.gutterMenuVisible = false
  settingsStore.updateTerminal({ showTimestamps: !showTimestamps.value })
}

const suggestions = useSuggestions()
let terminalInput: ReturnType<typeof useTerminalInput> | null = null
let terminal: Terminal | null = null

// While the gap replay re-parses stale device queries, xterm's
// auto-generated replies (CPR/DSR/DA) must not reach the remote: the
// mirror terminal already answered them in real time while this panel
// was inactive, and a second reply arriving after the remote's read
// timed out leaks its tail into the shell (the "0R" artifact).
// Timestamp cap in case a parse callback never fires; cleared early
// once the last replayed chunk reports parsed.
const replySuppressUntil = { value: 0 }


const retryOnEnter = { value: false }

// ── Touch panning (mobile) ──
const isMobileTouch = isMobilePlatform()

// OS file drops (resource manager) are delivered by Wails via the native
// file-drop event (common:WindowFilesDropped) with real absolute paths,
// routed here by this component's data-file-drop-target id. The HTML5 drop
// handler below only clears overlay state — OS file drops never carry file
// content into the webview.
const terminalDropId = 'terminal-drop-' + (crypto.randomUUID?.() || Math.random().toString(36).slice(2))

const nativeDrop = useNativeFileDrop({
  elementId: terminalDropId,
  isActive: () => !!props.sessionId && !zmodemStore.getActiveTransfer(props.sessionId),
  upload: (paths) => onNativePathsDropped(paths),
})

// Convert a Windows native path to the format expected by the active shell.
// - PowerShell / cmd: keep C:\foo\bar (native)
// - Git Bash / MSYS2 / Cygwin / MinGW: /c/foo/bar
// - WSL: /mnt/c/foo/bar


// SFTP line buffer



function getFitAddon() {
  return props.sessionId ? getManagedTerminal(props.sessionId)?.fitAddon : undefined
}

function getSelection(): string {
  return terminal ? getTerminalSelection(terminal) : ''
}



// ── Extracted composables ──
// Terminal data rendering pipeline (strip/expand/sanitize/DCS/timestamp)
const render = useTerminalRender({
  getTerminal: () => terminal,
  getMode: () => props.mode,
  getSessionId: () => props.sessionId,
  getTerminalInput: () => terminalInput,
  suggestions,
})
const renderTerminalData = render.renderTerminalData


// Terminal helpers (suggestion apply, file-drop paths, history sanitize)
const helpers = useTerminalHelpers({
  getTerminal: () => terminal,
  getTerminalInput: () => terminalInput,
  suggestions,
  getSessionId: () => props.sessionId,
  getPanelId: () => props.panelId,
  getMode: () => props.mode,
  getBroadcastActive: () => props.broadcastActive,
})
const applySuggestion = helpers.applySuggestion
const onNativePathsDropped = helpers.onNativePathsDropped
const sanitizeTerminalHistory = helpers.sanitizeTerminalHistory

// Resize / observers
const resize = useTerminalResize({
  getTerminal: () => terminal,
  terminalRef,
  getMode: () => props.mode,
  getSessionId: () => props.sessionId,
  getFitAddon,
})

// Key handling / IME / paste
const keyHandler = useTerminalKeyHandler({
  getTerminal: () => terminal,
  isMac,
  getMode: () => props.mode,
  getSessionId: () => props.sessionId,
  getPanelId: () => props.panelId,
  getBroadcastActive: () => props.broadcastActive,
  getPanel: () => panel.value,
  getTerminalInput: () => terminalInput,
  suggestions,

  applySuggestion,
})

// Output log recording + export/reset/clear buffer operations
const recorder = useTerminalRecorder({
  getTerminal: () => terminal,
  getPanelId: () => props.panelId,
  getPanel: () => panel.value,
  getSessionId: () => props.sessionId || undefined,
  closeMenu: () => menu.closeMenu(),
  isZmodemActive: (sid) => !!zmodemStore.getActiveTransfer(sid),
})
const isOutputLogOn = recorder.isOutputLogOn

// Touch panning (mobile)
const touchPan = useTerminalTouchPan({
  getTerminal: () => terminal,
  terminalRef,
  getSessionId: () => props.sessionId || undefined,
})

// Zmodem file-transfer lifecycle + session:data interception
const zmodem = useTerminalZmodem({
  getTerminal: () => terminal,
  getMode: () => props.mode,
  getSessionId: () => props.sessionId,
  renderTerminalData,
})

// Keystroke pipeline: onData → IME reorder → processTerminalData
const dataFlow = useTerminalDataFlow({
  getTerminal: () => terminal,
  getMode: () => props.mode,
  getSessionId: () => props.sessionId,
  getOnSessionStatus: () => props.onSessionStatus,
  keyHandler,
  suggestions,
  getTerminalInput: () => terminalInput,
  zmodem,
  applySuggestion,
  isMobileTouch,
  replySuppressUntil,
  retryOnEnter,
})

// Theme/options live application + settings watchers
useTerminalThemeApply({
  getTerminal: () => terminal,
  onResize: () => resize.resize(),
})

function focus() {
  // Don't steal focus while the user is typing in an input (e.g. renaming a
  // tab); a stray session:status event would otherwise blur the rename box.
  const el = document.activeElement
  if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || (el as HTMLElement).isContentEditable)) {
    return
  }
  terminal?.focus()
}

function write(data: string) {
  terminal?.write(data)
}

function setRetryOnEnter(value: boolean) {
  retryOnEnter.value = value
}

// Paste handling (SFTP — shares the dataFlow inputBuffer)
function pasteToTerminal(text: string) {
  if (props.mode === 'sftp' && terminal) {
    for (const char of text) {
      const code = char.charCodeAt(0)
      if (code >= 32 && code <= 126) {
        dataFlow.inputBuffer.value += char
        terminal.write(char)
      }
    }
  }
}


// ── Terminal right-click menu context (mirrors the tab-level menu) ──
const panel = computed(() => (props.panelId ? panelStore.getPanel(props.panelId) : undefined))
const isSsh = computed(() => panel.value?.type === 'ssh')
const supportsOutputLog = computed(() =>
  !!panel.value && ['ssh', 'telnet', 'serial', 'mosh', 'local', 'wsl'].includes(panel.value.type),
)

function onTerminalContextMenu(e: MouseEvent) {
  // Right-click on the gutter (line-number/time columns) opens the gutter
  // toggle menu instead of the terminal menu. The gutter itself is
  // pointer-events: none, so the event surfaces on .terminal-area and is
  // routed here by hit-testing the click position against the gutter rect.
  if (showLineNumbers.value || showTimestamps.value) {
    const g = gutterRef.value?.$el as HTMLElement | null
    const r = g?.getBoundingClientRect()
    if (r && e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom) {
      e.preventDefault()
      toolbarRef.value?.gutterMenuRef?.openAt(e.clientX, e.clientY)
      return
    }
  }
  if (supportsOutputLog.value) {
    recorder.refreshOutputLogState()
  }
  menu.onContextMenu(e)
}

function triggerSearch() {
  menu.closeMenu()
  searchRef.value?.openSearch()
}

function openSftp() {
  menu.closeMenu()
  const p = panel.value
  if (p) {
    // sourceTabId anchors the new SFTP tab beside the owning terminal tab
    window.dispatchEvent(new CustomEvent('app:connect-sftp', { detail: { ...p, sourceTabId: p.tabId } }))
  }
}

function uploadFileRz() {
  window.dispatchEvent(new CustomEvent('terminal:send-rz', { detail: { panelId: props.panelId } }))
  menu.closeMenu()
}

function openMonitor() {
  menu.closeMenu()
  const p = panel.value
  if (p) {
    // sourceTabId anchors the new monitor tab beside the owning terminal tab
    window.dispatchEvent(new CustomEvent('app:connect-monitor', { detail: { ...p, sourceTabId: p.tabId } }))
  }
}

const gutterRef = ref<{ $el: HTMLElement } | null>(null)
const menu = useTerminalMenu({
  getSelection,
  onSelectAll: () => terminal?.selectAll(),
  openAt: (x, y) => toolbarRef.value?.terminalMenuRef?.openAt(x, y),
  onPaste: async (text) => {
    if (props.mode === 'ssh' || props.mode === 'local') {
      await keyHandler.pasteToSession(text)
    } else {
      pasteToTerminal(text)
    }
    // Restore focus after paste so the cursor stays active in the terminal
    focus()
  },
  onAskAI: (text) => {
    window.dispatchEvent(new CustomEvent('ai:ask', { detail: text }))
  },
})

// Refs to extracted sub-components
const overlayRef = ref<InstanceType<typeof TerminalOverlay> | null>(null)
const searchRef = ref<InstanceType<typeof TerminalSearch> | null>(null)
const toolbarRef = ref<InstanceType<typeof TerminalToolbar> | null>(null)

// Session/window event listeners (selection, mouse, session:data, session:status, terminal:* events)
const sessionEvents = useTerminalSessionEvents({
  getTerminal: () => terminal,
  isActive,
  getSessionId: () => props.sessionId,
  getPanelId: () => props.panelId,
  getMode: () => props.mode,
  getOnSessionStatus: () => props.onSessionStatus,
  terminalRef,
  searchRef,
  retryOnEnter,
  zmodem,
  renderTerminalData,
  suggestions,
  keyHandler,
  recorder,
  menu,
  onResize: () => resize.resize(),
  focus,
})

// Lifecycle orchestration (onMounted/onActivated/onDeactivated/watch/onUnmounted)
useTerminalLifecycle({
  getTerminal: () => terminal,
  setTerminal: (t) => { terminal = t },
  getTerminalInput: () => terminalInput,
  setTerminalInput: (ti) => { terminalInput = ti },
  terminalRef,
  terminalInstanceRef,
  isActive,
  replySuppressUntil,
  retryOnEnter,
  getMode: () => props.mode,
  getSessionId: () => props.sessionId,
  nativeDrop,
  resize,
  touchPan,
  zmodem,
  dataFlow,
  render,
  sessionEvents,
  keyHandler,
  suggestions,
  getFitAddon,
  sanitizeTerminalHistory,
})

defineExpose({
  getSelection,
  resize: resize.resize,
  focus,
  write,
  setRetryOnEnter,
})
</script>

<style scoped>
.base-terminal {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}
.terminal-area {
  flex: 1;
  display: flex;
  flex-direction: row;
  align-items: stretch;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
}
.terminal-host {
  flex: 1;
  min-width: 0;
  min-height: 0;
  /* `clip` (not `hidden`): a hidden box is still programmatically scrollable,
   * so a focus()-driven auto-scroll during a resize transition could leave a
   * stale scrollTop here and shove the whole .xterm up, leaving blank space
   * below (it is never scrolled deliberately — xterm scrolls its own layers).
   * `clip` makes the box a non-scroll-container, so the offset cannot exist. */
  overflow: clip;
  position: relative;
}
.terminal-area :deep(.xterm) {
  width: 100%;
  height: 100%;
  display: block;
  box-sizing: border-box;
  /* 左/上/下 0.5rem，右侧不留：那 0.875rem 的滚动条轨道本身已把文本挡开
     （文本右缘与轨道间还有 0.125rem），右 padding 只会把整条滚动条往左推、
     在轨道外侧留一条空白。 */
  padding: 0.5rem 0 0.5rem 0.5rem;
}
/* 0.5rem padding（右 0）那圈用终端背景色，而不是应用主题色（--bg-base）。
   v5 时 xterm 把终端色内联在 .xterm-viewport 上，而它 absolute inset:0
   盖满 padding box，边缘因此自带终端色；v6 改成内联到 .xterm-scrollable-element，
   该元素止于 padding 内侧，边缘便露出 .xterm 自身的应用主题色 ——
   深色应用 + 浅色终端时就是一圈深色边框。--terminal-bg 由
   applyTerminalBgVar() 写在终端根元素上，取不到时回退旧行为。 */
.terminal-area :deep(.xterm),
.terminal-area :deep(.xterm-viewport) {
  background: var(--terminal-bg, var(--bg-base));
}
/* v6 起 .xterm-viewport 只是 overview ruler 的定位锚点，不再是滚动容器；
   留着 overflow-y: scroll 会多出一条空的原生轨道。终端滚动条样式见
   style.css 里的 .xterm-scrollable-element > .scrollbar 规则。 */
.terminal-area :deep(.xterm-viewport) {
  overflow: hidden;
}
</style>
