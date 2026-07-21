<template>
  <div
    class="base-terminal"
    @dragover.prevent="onDragOver"
    @dragenter.prevent="onDragEnter"
    @dragleave="onDragLeave"
    @drop="onDragDrop"
  >
    <div ref="terminalRef" class="terminal-area" @contextmenu="menu.onContextMenu"></div>

    <div v-if="dragOver" class="drop-overlay">
      <span>{{ t('sftp.dropHere') }}</span>
    </div>

    <!-- Search bar -->
    <div v-show="searchVisible" class="terminal-search-bar">
      <input
        ref="searchInputRef"
        v-model="searchText"
        class="search-input"
        :placeholder="t('terminal.searchPlaceholder')"
        @input="onSearchInput"
        @keydown.enter.prevent="onSearchNext"
        @keydown.escape="closeSearch"
      />
      <span class="search-count" v-if="searchText">{{ searchResultIndex + 1 }}/{{ searchResultCount || 0 }}</span>
      <button class="search-btn" @click="onSearchPrev" :title="t('terminal.searchPrev')">
        <ChevronUp :size="14" />
      </button>
      <button class="search-btn" @click="onSearchNext" :title="t('terminal.searchNext')">
        <ChevronDown :size="14" />
      </button>
      <button class="search-btn" @click="closeSearch" :title="t('terminal.searchClose')">
        <X :size="14" />
      </button>
    </div>

    <!-- Terminal context menu -->
    <div
      v-show="menu.menuVisible.value"
      class="context-menu"
      :style="menu.menuStyle.value"
      @click.stop
    >
      <div class="menu-item" :class="{ disabled: !menu.hasSelection.value }" @click="menu.askAI">
        {{ t('terminal.askAI') }}
      </div>
      <div class="menu-item" :class="{ disabled: !menu.hasSelection.value }" @click="menu.copySelection">
        {{ t('terminal.copy') }}
      </div>
      <div class="menu-item" :class="{ disabled: !menu.hasSelection.value }" @click="menu.copyAndPaste">
        {{ t('terminal.copyAndPaste') }}
      </div>
      <div class="menu-item" @click="menu.pasteFromClipboard">{{ t('terminal.paste') }}</div>
      <div class="menu-item" @click="menu.closeMenu(); exportContent()">{{ t('terminal.export') }}</div>
    </div>

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
    <ZmodemTransfer :session-id="props.sessionId || ''" @cancel="onZmodemCancel" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, onUnmounted, onActivated, onDeactivated, watch, nextTick } from 'vue'
import type { Terminal } from '@xterm/xterm'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { SessionWrite, SessionResize, SessionEndZmodem } from '../../wailsjs/go/main/App'
import { WriteFileBase64, SaveFileDialog, FrontendLog, WriteTempFile } from '../../wailsjs/go/main/App'
import { EventsOn, BrowserOpenURL, ClipboardGetText } from '../../wailsjs/runtime'
import { useSettingsStore } from '../stores/settingsStore'
import { useLocalStateStore } from '../stores/localStateStore'
import { highlight } from '../composables/useHighlight'
import { onTerminalKey } from '../composables/useKeyboardShortcuts'
import { useSessionStore } from '../stores/sessionStore'
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { useTerminalMenu } from '../composables/useTerminalMenu'
import { useI18n } from '../i18n'
import {
  acquireTerminal,
  releaseTerminal,
  attachTerminal,
  detachTerminal,
  getManagedTerminal,
  transferTerminal,
  bumpOnDataGeneration,
} from '../services/terminalManager'
import { getXtermTheme } from '../composables/useTerminal'
import { stripCursorBlink } from '../utils/cursor'
import { useTerminalInput } from '../composables/useTerminalInput'
import { useSuggestions, quickCommandCache } from '../composables/useSuggestions'
import TerminalSuggestion from './TerminalSuggestion.vue'
import { startZmodemService } from '../services/zmodemService'
import { useZmodemStore } from '../stores/zmodemStore'
import ZmodemTransfer from './ZmodemTransfer.vue'
import { ChevronUp, ChevronDown, X } from '@lucide/vue'

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
const sessionStore = useSessionStore()
const tabStore = useTabStore()
const panelStore = usePanelStore()
const zmodemStore = useZmodemStore()
const localStateStore = useLocalStateStore()
const { t } = useI18n()

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
const searchInputRef = ref<HTMLInputElement>()
const searchVisible = ref(false)

const dragOver = ref(false)
let dragEnterCount = 0

const suggestions = useSuggestions()
let terminalInput: ReturnType<typeof useTerminalInput> | null = null
let terminal: Terminal | null = null
let onDataDispose: { dispose(): void } | null = null
let keyHandlerDispose: { dispose(): void } | null = null
let resizeObserver: ResizeObserver | null = null
let intersectionObserver: IntersectionObserver | null = null
// Track how many sessionStore chunks have been written to the terminal
// so we can replay only missed data on KeepAlive reactivation.
let writtenChunks = 0
// Viewport position (top line in the buffer) captured on KeepAlive
// deactivation so reactivation can restore the user's scroll position
// instead of jumping to the bottom. baseY at deactivation is also kept so
// we can tell whether the user was scrolled to the bottom before
// deactivation — if they were, new output during the inactive period
// (replayed on activation) should pull the viewport down to the new
// bottom; otherwise the viewport stays where the user left it.
let savedViewportY: number | null = null
let savedBaseY: number | null = null
let unsubscribe: (() => void) | null = null
let statusUnsubscribe: (() => void) | null = null
let onDocumentMouseDown: ((e: MouseEvent) => void) | null = null
let onTerminalAuxClick: ((e: MouseEvent) => void) | null = null
let onOpenSearch: ((e: Event) => void) | null = null
let onExport: ((e: Event) => void) | null = null
let onSendRz: ((e: Event) => void) | null = null

let resizeTimer: ReturnType<typeof setTimeout> | null = null
let isResizing = false
let splitResizing = false
let suppressResizeUntil = 0
let retryOnEnter = false
let zmodemService: ReturnType<typeof startZmodemService> | null = null
let isZmodemStarting = false
let zmodemStartTimer: ReturnType<typeof setTimeout> | null = null
let zmodemDirection: 'upload' | 'download' | undefined = undefined
let zmodemCancellingUntil = 0
let exporting = false

function initZmodemService(sessionId: string) {
  if (!sessionId || props.mode !== 'ssh') return
  // Don't create a duplicate zmodem service if a transfer is already
  // active for this session. The existing service (in a deactivated
  // BaseTerminal) continues to handle the transfer.
  if (zmodemStore.getActiveTransfer(sessionId)) return
  zmodemService = startZmodemService({
    // Register abort so any BaseTerminal component can cancel the transfer
    onRegister: (abort) => zmodemStore.registerAbort(sessionId, abort),
    sessionId,
    direction: zmodemDirection,
    onComplete: (files, hint) => {
      if (files.length > 0) {
        terminal?.write(`\r\n\x1b[32mZmodem: ${files.length} file(s) transferred\x1b[0m\r\n`)
      }
      if (hint) {
        terminal?.write(`\r\n\x1b[33m${hint}\x1b[0m\r\n`)
      }
      if (files.length === 0 && !hint) {
        // 取消或未选择文件：打印提示
        terminal?.write(`\r\n\x1b[33mZmodem transfer cancelled\x1b[0m\r\n`)
        // 等吞数据保护过期后再发送一次回车，确保 sz 已退出、bash 恢复前台后触发提示符
        const cancelUntil = Math.max(zmodemCancellingUntil, zmodemStore.getCancelUntil(sessionId))
        const remaining = Math.max(0, cancelUntil - Date.now())
        setTimeout(() => {
          SessionWrite(sessionId, '\n').catch(() => {})
        }, remaining + 100)
      }
      zmodemStore.clearTransfers(sessionId)
      zmodemDirection = undefined
      disposeZmodemService(sessionId)
      initZmodemService(sessionId)
    },
    onError: (err) => {
      terminal?.write(`\r\n\x1b[31mZmodem error: ${err}\x1b[0m\r\n`)
      zmodemStore.clearTransfers(sessionId)
      zmodemDirection = undefined
      disposeZmodemService(sessionId)
      initZmodemService(sessionId)
    },
  })
}

async function disposeZmodemService(sessionId: string, resetDirection = true, endSession = true) {
  zmodemService?.dispose()
  zmodemService = null
  isZmodemStarting = false
  if (resetDirection) {
    zmodemDirection = undefined
  }
  if (zmodemStartTimer) {
    clearTimeout(zmodemStartTimer)
    zmodemStartTimer = null
  }
  if (sessionId && endSession) {
    await SessionEndZmodem(sessionId).catch(() => {})
  }
}

// Native file drop handler (Wails provides real file paths via the OS,
// bypassing WebView2's File.path limitation).
let fileDropRegistered = false

function onDragOver(e: DragEvent) {
  if (!e.dataTransfer?.types.includes('Files')) return
  e.stopPropagation()
  e.dataTransfer.dropEffect = 'copy'
  dragOver.value = true
}

function onDragEnter(e: DragEvent) {
  if (!e.dataTransfer?.types.includes('Files')) return
  e.stopPropagation()
  dragEnterCount++
  dragOver.value = true
}

function onDragLeave() {
  dragEnterCount--
  if (dragEnterCount <= 0) {
    dragEnterCount = 0
    dragOver.value = false
  }
}

function onDragDrop(e: DragEvent) {
  dragOver.value = false
  dragEnterCount = 0
  const files = e.dataTransfer?.files
  if (!files || files.length === 0 || !props.sessionId) return

  // Reject internal panel/tab drags — let them bubble to workspace handlers
  if (e.dataTransfer?.types.includes('application/panel-id') ||
      e.dataTransfer?.types.includes('application/tab-id')) return

  e.preventDefault()
  handleDroppedFiles(props.sessionId, Array.from(files))
}

// Convert a Windows native path to the format expected by the active shell.
// - PowerShell / cmd: keep C:\foo\bar (native)
// - Git Bash / MSYS2 / Cygwin / MinGW: /c/foo/bar
// - WSL: /mnt/c/foo/bar
function toShellPath(nativePath: string, shellPath?: string): string {
  const isWinPath = /^[A-Za-z]:\\/.test(nativePath)
  if (!isWinPath) return nativePath.replace(/\\/g, '/')

  const lower = (shellPath || '').toLowerCase()
  let converted: string
  if (lower.includes('wsl')) {
    const drive = nativePath.charAt(0).toLowerCase()
    converted = '/mnt/' + drive + nativePath.slice(2).replace(/\\/g, '/')
  } else if (lower.includes('bash') || lower.includes('git') || lower.includes('msys') || lower.includes('cygwin') || lower.includes('mingw')) {
    const drive = nativePath.charAt(0).toLowerCase()
    converted = '/' + drive + nativePath.slice(2).replace(/\\/g, '/')
  } else {
    converted = nativePath
  }
  // Quote paths with spaces so the shell treats them as a single argument
  return converted.includes(' ') ? `"${converted}"` : converted
}

async function handleDroppedFiles(sessionId: string, files: File[]) {
  const paths: string[] = []
  for (const f of files) {
    const nativePath = (f as any).path as string | undefined
    if (nativePath) {
      paths.push(nativePath)
    } else {
      try {
        const base64 = await new Promise<string>((resolve, reject) => {
          const reader = new FileReader()
          reader.onload = () => resolve((reader.result as string).split(',')[1])
          reader.onerror = () => reject(reader.error)
          reader.readAsDataURL(f)
        })
        paths.push(await WriteTempFile(f.name, base64))
      } catch (err) {
        terminal?.write(`\r\n\x1b[33mFailed to read "${f.name}": ${err}\x1b[0m\r\n`)
      }
    }
  }
  if (paths.length === 0) return

  // Local terminal: paste file paths as input, adapting format to the shell
  if (props.mode === 'local') {
    const panel = panelStore.getPanel(props.panelId || '')
    const shellPath = panel?.config?.shellPath
    const text = paths.map(p => toShellPath(p, shellPath)).join(' ')
    SessionWrite(sessionId, text)
    return
  }

  // Remote terminal: trigger zmodem upload
  zmodemStore.setPendingUploadFiles(sessionId, paths)
  SessionWrite(sessionId, 'rz -be\n')
}

function onZmodemCancel() {
  const ts = Date.now() + 2000
  zmodemCancellingUntil = ts
  if (props.sessionId) {
    zmodemStore.setCancelUntil(props.sessionId, ts)
    zmodemStore.abortTransfer(props.sessionId)
  }
}

// Search state
const searchText = ref('')
const searchResultIndex = ref(0)
const searchResultCount = ref(0)

function sanitizeTerminalHistory(text: string): string {
  if (!text) return text
  let cleaned = text
  // ZModem HEX header fragments
  cleaned = cleaned.replace(/\*{2,}(?:\x18)?[ABC][0-9a-fA-F]{10,}/g, '')
  // ZModem ZDLE (0x18) and backspace (0x08) sequences
  cleaned = cleaned.replace(/\x18+/g, '')
  cleaned = cleaned.replace(/\x08+/g, '')
  // ASCII control chars except \n, \r, \t and ESC
  cleaned = cleaned.replace(/[\x00-\x08\x0b\x0c\x0e-\x1a\x1c-\x1f\x7f]/g, '')
  // Drop binary garbage decoded as random Unicode blocks. Keep ASCII plus CJK
  // so normal Chinese/Japanese/Korean terminal output is preserved.
  cleaned = cleaned.replace(/[^\x00-\x7f一-鿿぀-ゟ゠-ヿ가-힯]/g, '')
  // Collapse blank lines left by removed garbage
  cleaned = cleaned.replace(/\n{3,}/g, '\n\n')
  // Forward debug info to backend log so we can inspect the raw garbage.
  if (cleaned !== text) {
    FrontendLog('sanitizeTerminalHistory', `raw last 400: ${JSON.stringify(text.slice(-400))}`)
    FrontendLog('sanitizeTerminalHistory', `cleaned last 400: ${JSON.stringify(cleaned.slice(-400))}`)
  }
  return cleaned
}

// SFTP line buffer
let inputBuffer = ''

function getTerminalOptions() {
  const ts = settingsStore.settings.terminal
  return {
    fontSize: ts.fontSize || 13,
    fontFamily: ts.fontFamily || 'Consolas, "Courier New", monospace',
    themeName: ts.theme || 'dark',
    scrollback: ts.maxHistoryLines || 2500,
  }
}

function getFitAddon() {
  return props.sessionId ? getManagedTerminal(props.sessionId)?.fitAddon : undefined
}

function getSearchAddon() {
  return props.sessionId ? getManagedTerminal(props.sessionId)?.searchAddon : undefined
}

function getSelection(): string {
  return terminal?.getSelection() || ''
}

async function applySuggestion(item: ReturnType<typeof suggestions.getSelectedItem>) {
  if (!item || !terminal || !terminalInput) return

  if (item.type === 'ai-preview') {
    // Step 1: Generate AI suggestion
    await suggestions.generateAISuggestion(terminalInput.lineBuffer.value)
    return
  }

  const sid = props.sessionId

  if (item.type === 'ai-result' || item.type === 'history' || item.type === 'quick-command') {
    // Replace entire line with Ctrl+U. Using backspaces only works when the
    // replacement is exactly the currentToken; for multi-token input (e.g.
    // "git che" → "git checkout") backspaces leave the earlier text behind.
    if (props.broadcastActive && props.workspaceId) {
      const targets = tabStore.getBroadcastPanelIdsInWorkspace(props.workspaceId)
      for (const pid of targets) {
        const p = panelStore.getPanel(pid)
        if (p?.sessionId && (p.type === 'ssh' || p.type === 'local')) {
          SessionWrite(p.sessionId, '\x15')
          SessionWrite(p.sessionId, item.value)
        }
      }
    } else if (sid) {
      SessionWrite(sid, '\x15')
      SessionWrite(sid, item.value)
    }
    terminalInput.lineBuffer.value = item.value
    terminalInput.cursorIndex.value = item.value.length
    terminalInput.currentToken.value = ''
  }

  suggestions.close()
}

function resize() {
  if (props.mode === 'ssh' || props.mode === 'local') {
    const sid = props.sessionId
    if (!terminal || !sid) return
    const fitAddon = getFitAddon()
    if (!fitAddon) return
    const el = terminalRef.value
    if (!el) return

    const rect = el.getBoundingClientRect()
    // Skip resize when the component is hidden (e.g. during tab switching
    // with KeepAlive). A zero-size resize would corrupt xterm.js buffers.
    if (rect.width === 0 || rect.height === 0) return

    // Always fit first to update CSS dimensions on the .xterm element.
    // Without this, stale inline pixel dimensions from a previous fit()
    // prevent the terminal from filling the container after window resize
    // (e.g. after duplicating a session tab).
    fitAddon.fit()
    if (terminal.cols <= 0 || terminal.rows <= 0) return

    let cellWidth = 0
    let cellHeight = 0
    try {
      const core = (terminal as any)._core
      const dims = core?._renderService?.dimensions
      if (dims) {
        cellWidth = dims.css?.cell?.width || 0
        cellHeight = dims.css?.cell?.height || 0
      }
    } catch {
      cellWidth = 0
      cellHeight = 0
    }

    if (cellWidth === 0 || cellHeight === 0) {
      SessionResize(sid, terminal.cols, terminal.rows).catch(() => {})
      return
    }

    const TERMINAL_PADDING = 4
    const scrollbarWidth = (terminal as any)._core?.viewport?.scrollBarWidth || 0
    const cols = Math.floor((rect.width - TERMINAL_PADDING * 2 - scrollbarWidth) / cellWidth)
    const rows = Math.floor((rect.height - TERMINAL_PADDING * 2) / cellHeight)
    const newCols = Math.max(2, cols)
    const newRows = Math.max(1, rows)

    terminal.resize(newCols, newRows)
    SessionResize(sid, newCols, newRows).catch(() => {})
  } else {
    getFitAddon()?.fit()
  }
}

function write(data: string) {
  terminal?.write(data)
}

function focus() {
  // Don't steal focus while the user is typing in an input (e.g. renaming a
  // tab); a stray session:status event would otherwise blur the rename box.
  const el = document.activeElement
  if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || (el as HTMLElement).isContentEditable)) {
    return
  }
  terminal?.focus()
}

function toBase64(str: string): string {
  const encoder = new TextEncoder()
  const bytes = encoder.encode(str)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary)
}

async function exportContent() {
  if (!terminal || exporting) return
  exporting = true
  try {
    let content = ''
    try {
      const buffer = terminal.buffer.active
      const totalLines = buffer.length
      const lines = new Array(totalLines)
      for (let i = 0; i < totalLines; i++) {
        const line = buffer.getLine(i)
        lines[i] = line ? line.translateToString() : ''
      }
      content = lines.join('\n')
    } catch (e) {
      console.error('Buffer read failed:', e)
      return
    }
    try {
      const filePath = await SaveFileDialog('terminal.txt')
      if (!filePath) return
      await WriteFileBase64(filePath, toBase64(content))
    } catch (e) {
      console.error('Export failed:', e)
    }
  } finally {
    exporting = false
  }
}

function setRetryOnEnter(value: boolean) {
  retryOnEnter = value
}

function onWindowResize() {
  const el = terminalRef.value
  if (!el) return
  if (!isResizing) {
    isResizing = true
    el.classList.add('resizing')
  }
  if (resizeTimer) clearTimeout(resizeTimer)
  resizeTimer = setTimeout(() => {
    isResizing = false
    el.classList.remove('resizing')
    resize()
  }, 400)
}

function onSplitResizeStart() {
  splitResizing = true
}

function onSplitResizeEnd() {
  splitResizing = false
  if (resizeTimer) {
    clearTimeout(resizeTimer)
    resizeTimer = null
  }
  suppressResizeUntil = Date.now() + 200
  nextTick(() => {
    setTimeout(() => {
      void terminalRef.value?.offsetWidth
      resize()
    }, 0)
  })
}

// Strip OSC sequences that xterm.js generates internally (color queries etc.)
// and CSI *responses* it auto-generates when the remote app queries the
// terminal (CPR cursor-position, DSR status, DA device-attributes, cell/window
// size). These are xterm.js talking back to a query — echoing them to the
// remote as if they were user input corrupts the app: a stray `ESC[2;2R`
// arriving mid-render makes some remote vims exit, closing the channel (issue
// #242). This must happen in the alternate screen too — vim/less/tmux are
// exactly the apps that emit `ESC[6n` and friends. Focus in/out (I/O) is left
// intact in the alternate screen because full-screen apps legitimately want
// FocusGained/FocusLost.
function filterTerminalInput(input: string, inAlternateScreen: boolean): string {
  // OSC sequences: ESC ] ... BEL or ESC ] ... ESC \
  let filtered = input.replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, '')
  // Terminal query responses — strip in both normal and alternate screens.
  filtered = filtered.replace(/\x1b\[(?:[?>][\d;]*|[\d;]*)([Rntc])/g, '')
  if (inAlternateScreen) {
    return filtered
  }
  // Normal screen only: also strip focus in/out, which a shell doesn't want.
  filtered = filtered.replace(/\x1b\[(?:[?>][\d;]*|[\d;]*)([IO])/g, '')
  return filtered
}

function writeTerminalInput(data: string, inAlternateScreen: boolean) {
  const sid = props.sessionId
  const filtered = filterTerminalInput(data, inAlternateScreen)
  // Don't send empty input - avoids extra blank line when pressing Enter with no command
  if (!sid || !filtered) return

  if (props.broadcastActive && props.workspaceId) {
    const targets = tabStore.getBroadcastPanelIdsInWorkspace(props.workspaceId)
    if (targets.length > 0) {
      for (const pid of targets) {
        const p = panelStore.getPanel(pid)
        if (p?.sessionId && (p.type === 'ssh' || p.type === 'local')) {
          SessionWrite(p.sessionId, filtered)
        }
      }
      return
    }
  }

  SessionWrite(sid, filtered)
}

function handleTerminalKey(e: KeyboardEvent): boolean {
  // Check global shortcuts first (Ctrl+Shift+/Alt+ combos)
  if (e.type === 'keydown' && !onTerminalKey(e)) return false

  // Paste via Wails clipboard (xterm's DOM paste is unreliable in WKWebView).
  // Bind to the platform's paste shortcut only — Cmd+V on macOS, Ctrl+Shift+V
  // elsewhere. Plain Ctrl+V is never intercepted, so it passes through to the
  // terminal app (vim visual block, bash literal-next…) on every platform.
  const pasteCombo = isMac
    ? (e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey)
    : (e.ctrlKey && e.shiftKey && !e.metaKey && !e.altKey)
  if (pasteCombo && (e.key === 'v' || e.key === 'V') && e.type === 'keydown') {
    e.preventDefault()
    if (props.mode === 'ssh' || props.mode === 'local') {
      ClipboardGetText().then(text => {
        if (text) pasteToSession(text)
      }).catch(() => {})
    }
    return false
  }

  // Ctrl+Shift+C: copy terminal selection to clipboard
  if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'c' || e.key === 'C') && e.type === 'keydown') {
    const sel = terminal?.getSelection()
    if (sel) {
      navigator.clipboard.writeText(sel).catch(() => {})
    }
    return false
  }

  // macOS-style cursor word/line jumping via Option/Cmd + arrow keys
  if (e.type === 'keydown' && (e.altKey || e.metaKey)) {
    if (e.key === 'ArrowLeft') {
      e.preventDefault()
      if (e.metaKey) {
        // Cmd+Left → beginning of line
        SessionWrite(props.sessionId || '', '\x1b[H')
      } else if (e.altKey) {
        // Option+Left → backward word
        SessionWrite(props.sessionId || '', '\x1bb')
      }
      return false
    }
    if (e.key === 'ArrowRight') {
      e.preventDefault()
      if (e.metaKey) {
        // Cmd+Right → end of line
        SessionWrite(props.sessionId || '', '\x1b[F')
      } else if (e.altKey) {
        // Option+Right → forward word
        SessionWrite(props.sessionId || '', '\x1bf')
      }
      return false
    }
  }

  // Suggestion navigation (only on keydown, ignore keyup)
  if (suggestions.isVisible() && e.type === 'keydown') {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      suggestions.selectNext()
      return false
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      suggestions.selectPrev()
      return false
    }
    if (e.key === 'Tab') {
      const selected = suggestions.getSelectedItem()
      if (selected) {
        e.preventDefault()
        applySuggestion(selected)
        return false
      }
    }
    if (e.key === 'Enter') {
      // Only apply suggestion if user explicitly selected one with arrow keys
      const selected = suggestions.getSelectedItem()
      if (selected) {
        e.preventDefault()
        applySuggestion(selected)
        return false
      }
      // No selection: let xterm handle Enter normally (terminal command execution)
    }
    if (e.key === 'Escape') {
      suggestions.close()
      return false
    }
  }

  return true
}

let bindListeners: (() => void) | null = null

onMounted(() => {
  if (!terminalRef.value) return

  // Acquire shared terminal from manager (or create if first mount)
  const opts = getTerminalOptions()
  terminal = acquireTerminal(props.sessionId || '', terminalInstanceRef, opts, settingsStore.settings.customTerminalThemes)

  // Load WebLinksAddon per-component (has custom callbacks)
  let hoverEl: HTMLDivElement | null = null
  const webLinksAddon = new WebLinksAddon(
    (event, uri) => {
      if (event.ctrlKey || event.metaKey) {
        BrowserOpenURL(uri)
      }
    },
    {
      hover(event, _text, _location) {
        if (!hoverEl) {
          hoverEl = document.createElement('div')
          hoverEl.className = 'xterm-link-tooltip'
          terminal!.element!.appendChild(hoverEl)
        }
        const rect = terminal!.element!.getBoundingClientRect()
        hoverEl.textContent = 'Ctrl + Click to open'
        hoverEl.style.left = (event.clientX - rect.left + 12) + 'px'
        hoverEl.style.top = (event.clientY - rect.top - 28) + 'px'
        hoverEl.style.display = 'block'
      },
      leave() {
        if (hoverEl) {
          hoverEl.style.display = 'none'
        }
      }
    }
  )
  terminal.loadAddon(webLinksAddon)

  // Unicode 11 support
  try { terminal.unicode.activeVersion = '11' } catch (_) {}

  // Set up search results listener from shared SearchAddon
  const managed = getManagedTerminal(props.sessionId || '')
  if (managed) {
    managed.searchAddon.onDidChangeResults((e) => {
      searchResultIndex.value = e.resultIndex
      searchResultCount.value = e.resultCount
    })
  }

  // Attach terminal DOM to this component's container
  attachTerminal(props.sessionId || '', terminalRef.value)

  initZmodemService(props.sessionId || '')

  // Initialize terminal input handling for SSH
  if (props.mode === 'ssh') {
    const smartOn = settingsStore.settings.terminal.smartCompletion ?? true
    terminalInput = useTerminalInput(terminal, {
      mode: props.mode,
      sessionId: props.sessionId,
      enableHistory: true,  // was: smartOn
      onHistoryExtract: (command: string) => {
        suggestions.addHistoryCommand(command)
      },
      onResetSuppress: () => {
        suggestions.resetSuppress()
      },
    })
    suggestions.loadHistory()
  }

  if (props.mode === 'ssh' || props.mode === 'local') {
    // Restore terminal content from session buffer on first mount only.
    // Subsequent mounts reuse the shared terminal whose buffer already
    // contains the correct content.
    const sid = props.sessionId
    const isNewTerminal = managed?.isNew
    if (sid && isNewTerminal) {
      const raw = sessionStore.getData(sid)
      const history = sanitizeTerminalHistory(raw)
      if (history) {
        // Apply syntax highlighting when restoring history so it matches
        // newly arriving lines after a tab switch.
        const hlOn = (settingsStore.settings.terminal.highlightEnabled ?? true) && props.mode !== 'local'
        terminal.write(hlOn ? highlight(history) : history)
      }
    }
    // Always sync writtenChunks to prevent onActivated from replaying
    // all session data when the terminal was reused (isNewTerminal=false).
    if (sid) {
      writtenChunks = sessionStore.getChunkCount(sid)
    }
    // Force initial resize with retries — needed because cell dimensions
    // may not be available immediately, and for reused terminals the cols/rows
    // may hold stale dimensions from the previous container.
    ;[50, 150, 300, 600, 1000, 2000].forEach(d => setTimeout(() => {
      if (!terminal) return
      const el = terminalRef.value
      const inDOM = el ? document.contains(el) : false
      const hasXterm = el?.querySelector('.xterm') ? true : false
      const kids = el?.children.length ?? 0
      const rect = el?.getBoundingClientRect()
      getFitAddon()?.fit()
      const sessionId = props.sessionId
      if (sessionId && terminal.cols > 0 && terminal.rows > 0) {
        SessionResize(sessionId, terminal.cols, terminal.rows).catch(() => {})
      }
    }, d))
  }

  // Bind per-component listeners (onData, keyHandler).
  // Called from onMounted and onActivated; disposed in onDeactivated.
  bindListeners = () => {
    // Dispose previous listeners before re-registering
    onDataDispose?.dispose()
    onDataDispose = null
    keyHandlerDispose?.dispose()
    keyHandlerDispose = null

    // Bump the TERMINAL-SHARED generation counter so that ALL
    // components sharing this terminal can detect stale callbacks.
    // Per-component counter allowed KeepAlive-cached duplicate
    // components to both pass their independent guards.
    const sidNow = props.sessionId
    const gen = sidNow ? bumpOnDataGeneration(sidNow) : 0

    keyHandlerDispose = terminal.attachCustomKeyEventHandler(handleTerminalKey)

    // Input handling
    onDataDispose = terminal.onData((data) => {
      // ── Stale callback guard (terminal-shared) ──
      // Check against the MANAGED terminal's current generation.
      // If another component (e.g. KeepAlive-cached) has registered
      // a newer handler on the same shared terminal, bail out.
      const curGen = sidNow ? (getManagedTerminal(sidNow)?.onDataGeneration ?? gen) : gen
      if (gen !== curGen) {
        return
      }

      if (props.mode === 'ssh' || props.mode === 'local') {
      if (retryOnEnter && (data === '\r' || data === '\n')) {
        retryOnEnter = false
        if (props.onSessionStatus) {
          props.onSessionStatus('retry')
        }
        return
      }

      // Detect rz/sz command to hint zmodem transfer direction.
      // Must happen BEFORE terminalInput.handleInput because handleInput
      // clears the line buffer on Enter.
      if ((data === '\r' || data === '\n') && terminalInput && !terminalInput.isInAlternateScreen()) {
        const line = terminalInput.lineBuffer.value.trim()
        if (/^(?:sudo\s+)?rz\b/.test(line)) {
          zmodemDirection = 'upload'
          // Recreate the zmodem service so on_detect sees the new direction.
          if (props.sessionId) {
            disposeZmodemService(props.sessionId, false).then(() => {
              initZmodemService(props.sessionId)
            })
          }
        } else if (/^(?:sudo\s+)?sz\b/.test(line)) {
          zmodemDirection = 'download'
          if (props.sessionId) {
            disposeZmodemService(props.sessionId, false).then(() => {
              initZmodemService(props.sessionId)
            })
          }
        }
      }

      // Handle suggestions input (skip in alternate screen apps like vim/k9s)
      if (terminalInput && !terminalInput.isInAlternateScreen() && (props.mode === 'ssh' || props.mode === 'local')) {
        terminalInput.handleInput(data)

        // When suggestions are visible, intercept certain keys synchronously
        if (suggestions.isVisible()) {
          if (data === '\t') {
            const selected = suggestions.getSelectedItem()
            if (selected) {
              applySuggestion(selected)
              return
            }
          }
          if (data === '\r' || data === '\n') {
            const selected = suggestions.getSelectedItem()
            if (selected) {
              applySuggestion(selected)
              return
            }
          }
          if (data === '\x1b') {
            suggestions.close()
            return
          }
        }

        // Defer suggestion update/close so SessionWrite is not blocked
        const wasVisible = suggestions.isVisible()
        setTimeout(() => {
          if (!terminalInput) return
          const smartOn = settingsStore.settings.terminal.smartCompletion ?? true
          if (!smartOn) {
            suggestions.close()
            return
          }
          // Don't show suggestions if line buffer was already cleared (e.g. Enter pressed)
          if (!terminalInput.lineBuffer.value) {
            suggestions.close()
            return
          }
          // Only show suggestions if they were already visible or if the
          // input is a printable character (not arrow keys / navigation).
          const isPrintable = data.length === 1 && data >= ' '
          if (!wasVisible && !isPrintable) return
          if (terminalInput.isAtLineEnd() && terminalInput.currentToken.value && !terminalInput.isPasswordMode()) {
            suggestions.updateSuggestions(terminalInput.currentToken.value)
          } else {
            suggestions.close()
          }
        }, 0)
      } else if (terminalInput?.isInAlternateScreen()) {
        suggestions.close()
      }

      const sid = props.sessionId
      const inAlt = terminalInput?.isInAlternateScreen() ?? false
      if (sid) {
        writeTerminalInput(data, inAlt)
      }
    } else {
      // SFTP line buffering
      for (let i = 0; i < data.length; i++) {
        const char = data[i]
        const code = data.charCodeAt(i)
        if (char === '\r' || char === '\n') {
          if (inputBuffer) {
            const sid = props.sessionId
            if (sid) {
              for (let j = 0; j < inputBuffer.length; j++) {
                terminal!.write('\b \b')
              }
              SessionWrite(sid, inputBuffer)
            }
            inputBuffer = ''
          }
        } else if (code === 127 || char === '\b') {
          if (inputBuffer.length > 0) {
            inputBuffer = inputBuffer.slice(0, -1)
            terminal!.write('\b \b')
          }
        } else if (code >= 32 && code <= 126) {
          inputBuffer += char
          terminal!.write(char)
        }
      }
    }
  })

  } // end bindListeners

  // Selection action: copy to clipboard via xterm's native selection event.
  // Use setTimeout to let xterm finish processing the selection (especially
  // for double-click word selection) before reading getSelection().
  let lastSelectionText = ''
  terminal.onSelectionChange(() => {
    if (settingsStore.settings.terminal.selectionAction !== 'copy') return
    setTimeout(() => {
      const text = terminal?.getSelection()
      if (text && text !== lastSelectionText) {
        lastSelectionText = text
        navigator.clipboard.writeText(text).catch(() => {})
      }
    }, 0)
  })

  // Close suggestion popup when clicking outside
  onDocumentMouseDown = (event: MouseEvent) => {
    if (!suggestions.isVisible()) return
    const baseTerminalEl = terminalRef.value?.parentElement
    const popupEl = baseTerminalEl?.querySelector('.terminal-suggestion-popup')
    if (popupEl && !popupEl.contains(event.target as Node)) {
      suggestions.close()
    }
  }
  document.addEventListener('mousedown', onDocumentMouseDown)

  // Middle-click paste: read setting and paste clipboard if enabled
  onTerminalAuxClick = (event: MouseEvent) => {
    if (event.button !== 1) return
    if (!terminalRef.value?.contains(event.target as Node)) return
    const action = settingsStore.settings.terminal.middleClickAction
    if (action !== 'paste') return
    event.preventDefault()
    event.stopPropagation()
    ClipboardGetText().then(text => {
      if (text && props.sessionId) {
        pasteToSession(text)
      }
    }).catch(() => {})
  }
  document.addEventListener('auxclick', onTerminalAuxClick)

  // Session data
  unsubscribe = EventsOn('session:data', (payload: { id: string; data: string }) => {
    if (!isActive.value) {
      // Mark notification dot on the tab when inactive terminal receives output
      // Only process events for this instance's session (events are global)
      if (payload.id === props.sessionId && props.sessionId) {
        // Find the panel by sessionId (panels Map is keyed by panelId)
        let panelId: string | null = null
        for (const [id, p] of panelStore.panels) {
          if (p.sessionId === props.sessionId) {
            panelId = id
            break
          }
        }
        if (panelId) {
          const tab = tabStore.tabs.find(t =>
            (t.type === 'terminal' && t.panelId === panelId) ||
            (t.type === 'workspace' && t.panelIds.includes(panelId))
          )
          if (tab && tab.id !== tabStore.activeTabId) {
            tabStore.markTabNotification(tab.id)
          }
        }
      }
      return
    }
    if (payload.id !== props.sessionId || !terminal) return

    // 取消后 2 秒内吞掉所有数据，防止残余二进制乱码
    if (Date.now() < zmodemCancellingUntil) {
      return
    }

    // tab 切换后服务还没重建，但 store 里还有活跃传输（旧的 handleReceive 还在跑），先吞数据
    const hasStoreTransfer = zmodemStore.getActiveTransfer(props.sessionId || '')
    if (!zmodemService && hasStoreTransfer) {
      return
    }

    // Zmodem detection: scan for HEX header in normal terminal output.
    // Skip if the service was aborted (waiting for the next rz/sz command).
    const activeZmodem = zmodemService && zmodemStore.getActiveTransfer(props.sessionId || '')
    if (zmodemService && !activeZmodem && !zmodemService.isAborted?.()) {
      if (isZmodemStarting) {
        // Already detected header and waiting for SessionStartZmodem / on_detect.
        // Feed subsequent data to sentry so it can complete detection without
        // re-processing the header heuristic on every retry frame.
        zmodemService.consume(payload.data)
        return
      }
      // Real ZMODEM headers always contain the ZDLE control byte (0x18)
      // before the frame type: `**` (ZPAD ZPAD) `\x18` (ZDLE) `[ABC]` then
      // hex digits. Requiring the 0x18 avoids false positives on ordinary
      // terminal output — e.g. vim rendering a file whose content happens to
      // contain `**B<hex>` — which previously flipped the session into binary
      // ZMODEM mode and made the sentry write protocol bytes back to the
      // server, crashing the remote shell (issue #242).
      const ZMODEM_HEX_RE = /\*{2,}\x18[ABC][0-9a-fA-F]{10,}/
      if (ZMODEM_HEX_RE.test(payload.data)) {
        console.debug('[zmodem] header detected, entering transfer mode')
        isZmodemStarting = true
        if (zmodemStartTimer) clearTimeout(zmodemStartTimer)
        zmodemStartTimer = setTimeout(() => {
          isZmodemStarting = false
        }, 3000)
        const sid = props.sessionId
        if (sid) {
          // Consume immediately to avoid losing data during async handoff
          zmodemService.consume(payload.data)
          import('../../wailsjs/go/main/App').then(({ SessionStartZmodem }) => {
            SessionStartZmodem(sid).catch(() => {})
          })
        }
        // Hide zmodem data from terminal
        return
      }
    }

    // If zmodem is active, skip writing data to terminal (data comes via session:binary)
    if (activeZmodem) {
      isZmodemStarting = false
      return
    }

    // Filter ED3 (erase scrollback).
    let data = stripCursorBlink(payload.data, settingsStore.settings.terminal.cursorBlink ?? true).replace(/\x1b\[3J/g, '')
    // For ED2 (clear screen) in the main buffer, replace with scrolling
    // to preserve scrollback history. In alternate screen (vim, less,
    // k9s), pass through unchanged — the app manages its own screen.
    if (data.includes('\x1b[2J') && terminal.buffer.active.type !== 'alternate') {
      const rows = terminal.rows
      const scrollClear = '\n'.repeat(rows) + '\x1b[H'
      data = data.replace(/\x1b\[H\x1b\[2J/g, scrollClear)
      data = data.replace(/\x1b\[2J/g, scrollClear)
    }
    if (props.mode === 'sftp') {
      const cleaned = data.replace(/\x1b\]633;S[^\x07]*\x07/g, '')
      if (cleaned) {
        terminal.write(cleaned)
      }
      writtenChunks++
    } else {
      // Extract history commands from SSH output
      if (props.mode === 'ssh' && terminalInput) {
        terminalInput.handleSessionData(data)
        // Close suggestions if we entered an alternate screen app (vim, k9s, etc.)
        if (terminalInput.isInAlternateScreen()) {
          suggestions.close()
        }
      }
      const hlOn = (settingsStore.settings.terminal.highlightEnabled ?? true) && props.mode !== 'local'
      terminal.write(hlOn ? highlight(data) : data)
      writtenChunks++
      if (props.mode === 'ssh' && props.onSessionStatus) {
        // onSessionData is handled by the consumer via EventsOn if needed
      }
    }
  })

  // SSH/Local: session status events
  if (props.mode === 'ssh' || props.mode === 'local') {
    retryOnEnter = false
    statusUnsubscribe = EventsOn('session:status', (payload: { id: string; status: string }) => {
      if (!isActive.value) return
      if (payload.id !== props.sessionId) return
      if (payload.status === 'connected') {
        retryOnEnter = false
        if (props.onSessionStatus) {
          props.onSessionStatus(payload.status)
        }
        // Force send current terminal size to sync the backend PTY after reconnect.
        // The new session defaults to 80x24; without this, apps like vim/k9s use the wrong size.
        if (terminal && terminal.cols > 0 && terminal.rows > 0) {
          SessionResize(props.sessionId, terminal.cols, terminal.rows).catch(() => {})
        }
        resize()
      } else if (payload.status === 'error') {
        retryOnEnter = true
        if (props.onSessionStatus) {
          props.onSessionStatus(payload.status)
        }
        terminal?.write('\r\n\x1b[31mConnection failed. Press Enter to retry.\x1b[0m\r\n')
      } else if (payload.status === 'disconnected') {
        retryOnEnter = true
        if (props.onSessionStatus) {
          props.onSessionStatus(payload.status)
        }
      } else {
        // Focus terminal on connecting so user can type password immediately.
        focus()
        if (props.onSessionStatus) {
          props.onSessionStatus(payload.status)
        }
      }
    })
  }

  window.addEventListener('resize', onWindowResize)
  window.addEventListener('split:resize-start', onSplitResizeStart)
  window.addEventListener('split:resize-end', onSplitResizeEnd)
  onOpenSearch = (e: Event) => {
    if (!isActive.value) return
    const detail = (e as CustomEvent).detail
    if (!props.panelId || detail?.panelId !== props.panelId) return
    openSearch()
  }
  window.addEventListener('terminal:open-search', onOpenSearch)

  onExport = (e: Event) => {
    if (!isActive.value) return
    const detail = (e as CustomEvent).detail
    if (!props.panelId || detail?.panelId !== props.panelId) return
    exportContent()
  }
  window.addEventListener('terminal:export', onExport)

  onSendRz = (e: Event) => {
    const detail = (e as CustomEvent).detail
    if (detail?.panelId && detail.panelId !== props.panelId) return
    if (props.sessionId) {
      SessionWrite(props.sessionId, 'rz -be\n')
    }
  }
  window.addEventListener('terminal:send-rz', onSendRz)

  bindListeners()

  resizeObserver = new ResizeObserver(() => {
    if (isResizing || splitResizing || Date.now() < suppressResizeUntil) return
    const el = terminalRef.value
    if (!el) return
    if (resizeTimer) clearTimeout(resizeTimer)
    resizeTimer = setTimeout(() => resize(), 150)
  })
  resizeObserver.observe(terminalRef.value)

  if (props.mode === 'ssh') {
    intersectionObserver = new IntersectionObserver((entries) => {
      entries.forEach(entry => {
        if (entry.isIntersecting) {
          resize()
        }
      })
    })
    intersectionObserver.observe(terminalRef.value)
  }
})

onActivated(() => {
  // Component restored from KeepAlive cache.

  // Replay session data that arrived while deactivated BEFORE
  // setting isActive = true. The session:data handler gates on
  // isActive, so new data would race with the gap replay and
  // advance writtenChunks, making the gap undetectable.
  // Uses a monotonic sequence number (getChunkCount), not an array
  // index, so front-trimming of the sessionStore buffer during a
  // long background session doesn't invalidate the tracking position.
  if (props.sessionId) {
    const total = sessionStore.getChunkCount(props.sessionId)
    if (total > writtenChunks) {
      const tail = sessionStore.getDataFromChunk(props.sessionId, writtenChunks)
      const hlOn = (settingsStore.settings.terminal.highlightEnabled ?? true) && props.mode !== 'local'
      terminal?.write(hlOn ? highlight(tail) : tail)
      writtenChunks = total
    }
  }

  // Sync retryOnEnter from stored session status. The session:status
  // event is guarded by isActive (which was false during deactivation),
  // so if the session disconnected while we were cached, retryOnEnter
  // was never set and Enter would do nothing despite the reconnect
  // message being replayed above.
  if (props.sessionId) {
    const st = sessionStore.getStatus(props.sessionId)
    if (st === 'disconnected' || st === 'error') {
      retryOnEnter = true
    }
  }

  isActive.value = true
  // Re-attach terminal element — another component may have moved it while
  // we were cached (e.g. merge→drag-out→re-merge keeps panelId, KeepAlive
  // reuses the cached BaseTerminal, but the terminal is in holding).
  if (terminalRef.value && props.sessionId) {
    attachTerminal(props.sessionId, terminalRef.value)
    nextTick(() => getFitAddon()?.fit())
  }
  // Re-register onData/keyHandler listeners that were disposed in onDeactivated.
  bindListeners?.()
  // Restore the user's scroll position captured on deactivation BEFORE
  // resize. resize() triggers viewport.syncScrollArea → _innerRefresh
  // which reads ydisp and writes scrollTop. We must keep ydisp and the
  // DOM scrollTop in sync — wheel events compute the next ydisp from
  // scrollTop, so a mismatch (e.g. ydisp = 100 but scrollTop = 0 after
  // the element was moved out of a display:none holding container)
  // breaks scroll-up/scroll-down.
  //
  // We can't rely solely on scrollToLine() because it only updates ydisp;
  // the DOM scrollTop stays at 0 (or whatever the browser left it at)
  // until _innerRefresh runs with a valid viewport height. resize() bails
  // when the container has zero size, which is the case right after
  // KeepAlive activation before layout settles. So we do three things:
  //   1. scrollToLine(target) to restore ydisp immediately.
  //   2. If rowHeight is known, also set scrollTop directly so the
  //      viewport shows the right content from the first paint — no
  //      flash to position 0.
  //   3. retry resize() a few times; once layout settles and resize()
  //      succeeds, _innerRefresh writes scrollTop = ydisp * rowHeight,
  //      which matches what we set in step 2, so it is a no-op.
  // If no position was saved, leave the viewport alone — the natural
  // _innerRefresh path during resize lands on the bottom.
  if (savedViewportY != null && savedBaseY != null) {
    const probeBuf = terminal?.buffer?.active as { baseY?: number; length?: number } | undefined
    if (probeBuf && typeof probeBuf.baseY === 'number' && typeof probeBuf.length === 'number') {
      const wasAtBottom = savedViewportY >= savedBaseY - 1
      const target = wasAtBottom ? probeBuf.baseY : Math.min(savedViewportY, probeBuf.length - 1)
      terminal?.scrollToLine(target)
      const vp = terminal?.element?.querySelector('.xterm-viewport') as HTMLElement | null
      const core = (terminal as any)?._core
      const rowHeight: number = core?.viewport?._currentRowHeight ?? 0
      if (vp && rowHeight > 0) {
        vp.scrollTop = target * rowHeight
      }
    }
  }
  // Terminal dimensions may be stale after tab switch; recalculate.
  // resize() bails when rect is 0×0 — common right after KeepAlive
  // activation before layout settles — so retry a few times. Each retry
  // that succeeds will run _innerRefresh which writes scrollTop from the
  // restored ydisp; once scrollTop matches the desired value, the next
  // _innerRefresh is a no-op.
  resize()
  ;[0, 50, 150, 300, 600].forEach(d => setTimeout(resize, d))
  // Re-initialize zmodem service only if it was disposed in onDeactivated.
  // If a transfer was active, the service is still running — skip recreate.
  // safe: no focus() call here, avoids WebView2 crash race with native dialogs.
  if (props.sessionId && props.mode === 'ssh' && !zmodemService) {
    initZmodemService(props.sessionId)
  }
  // Note: focus() is intentionally skipped here. Calling focus() during
  // activation can race with native dialogs (OpenDirectoryDialog etc.)
  // and trigger a WebView2 crash (edge.Chromium.Focus parameter error).
})

onDeactivated(() => {
  // Component deactivated by KeepAlive (e.g. terminal tab moved into workspace).
  // Mark inactive so session event handlers become no-ops.
  isActive.value = false
  // Capture viewport position before listeners are torn down so reactivation
  // can restore the user's scroll position. Reading from the public IBuffer
  // API avoids depending on internal _core field shape.
  const buf = terminal?.buffer?.active as { viewportY?: number; baseY?: number } | undefined
  if (buf && typeof buf.viewportY === 'number') {
    savedViewportY = buf.viewportY
  }
  if (buf && typeof buf.baseY === 'number') {
    savedBaseY = buf.baseY
  }
  // Dispose per-component listeners to prevent duplicate input when another
  // BaseTerminal mounts with the same shared terminal instance.
  onDataDispose?.dispose()
  onDataDispose = null
  keyHandlerDispose?.dispose()
  keyHandlerDispose = null

  // If a transfer is still active, keep the service running so the background
  // transfer continues. Otherwise dispose and restore backend state.
  const hasStoreTransfer = zmodemStore.getActiveTransfer(props.sessionId || '')
  if (hasStoreTransfer) {
    return
  }
  disposeZmodemService(props.sessionId || '')
})

// Watch sessionId changes to rebind session data
watch(() => props.sessionId, (newId, oldId) => {
  if (oldId && oldId !== newId) {
    if (terminalRef.value) detachTerminal(oldId, terminalRef.value)
    disposeZmodemService(oldId)
    // Transfer the terminal to the new sessionId so scrollback is
    // preserved across reconnects. releaseTerminal is intentionally
    // skipped — we want to keep the same terminal instance alive.
    if (newId) transferTerminal(oldId, newId)
  }
  // Reset write tracking when session changes so onActivated replay
  // starts from the correct offset for the new session.
  writtenChunks = 0
  // Reset saved scroll position — the new sessionId has a different buffer.
  savedViewportY = null
  savedBaseY = null
  if (newId && (props.mode === 'ssh' || props.mode === 'local')) {
    initZmodemService(newId)
    terminal = getManagedTerminal(newId)?.terminal ?? null
    if (terminalRef.value) {
      attachTerminal(newId, terminalRef.value)
    }
    // Re-create terminalInput with the new terminal reference.
    // Otherwise it would still hold the old (disposed) terminal and
    // cursor position tracking returns {0,0}, pinning the suggestion
    // popup to the top-left corner.
    if (props.mode === 'ssh') {
      const smartOn = settingsStore.settings.terminal.smartCompletion ?? true
      terminalInput = useTerminalInput(terminal, {
        mode: props.mode,
        sessionId: newId,
        enableHistory: true,  // was: smartOn
        onHistoryExtract: (command: string) => {
          suggestions.addHistoryCommand(command)
        },
        onResetSuppress: () => {
          suggestions.resetSuppress()
        },
      })
      suggestions.loadHistory()
    }
    // Re-bind onData/keyHandler on the new terminal
    bindListeners?.()
    const delays = [200, 400, 600, 800, 1000, 1500, 2000]
    delays.forEach((delay) => {
      setTimeout(() => resize(), delay)
    })
  }
})

// ── Search ──
const searchDecoOptions = {
  matchBackground: '#515c6e',
  matchBorder: '#22d3ee',
  matchOverviewRuler: '#22d3ee',
  activeMatchBackground: '#22d3ee44',
  activeMatchBorder: '#22d3ee',
  activeMatchColorOverviewRuler: '#22d3ee',
}

function openSearch() {
  searchVisible.value = true
  nextTick(() => {
    searchInputRef.value?.focus()
    if (searchText.value) {
      searchInputRef.value?.select()
      getSearchAddon()?.findNext(searchText.value, { decorations: searchDecoOptions })
    }
  })
}

function closeSearch() {
  searchVisible.value = false
  searchText.value = ''
  searchResultIndex.value = 0
  searchResultCount.value = 0
  getSearchAddon()?.clearDecorations()
}

function onSearchInput() {
  if (!searchText.value) {
    searchResultIndex.value = 0
    searchResultCount.value = 0
    getSearchAddon()?.clearDecorations()
    return
  }
  getSearchAddon()?.findNext(searchText.value, { incremental: true, decorations: searchDecoOptions })
}

function onSearchNext() {
  if (!searchText.value) return
  getSearchAddon()?.findNext(searchText.value, { decorations: searchDecoOptions })
}

function onSearchPrev() {
  if (!searchText.value) return
  getSearchAddon()?.findPrevious(searchText.value, { decorations: searchDecoOptions })
}

function applyXtermTheme(themeName: string) {
  if (!terminal) return
  const theme = getXtermTheme(themeName, settingsStore.settings.customTerminalThemes)
  if (localStateStore.state.backgroundEnabled && localStateStore.state.backgroundImage) {
    theme.background = 'rgba(0,0,0,0)'
  }
  terminal.options.theme = theme
}

// Watch terminal settings changes
watch(() => settingsStore.settings.terminal, (ts) => {
  if (!terminal) return
  if (ts.fontSize) terminal.options.fontSize = ts.fontSize
  if (ts.fontFamily) terminal.options.fontFamily = ts.fontFamily
  if (ts.maxHistoryLines) terminal.options.scrollback = ts.maxHistoryLines
  if (ts.theme) applyXtermTheme(ts.theme)
  if (typeof ts.cursorBlink === 'boolean') {
    terminal.options.cursorBlink = ts.cursorBlink
    // xterm keeps an internal blink state set by DECSET 12; if the
    // terminal previously received \x1b[?12h from a remote shell, just
    // flipping the option may not stop the running blink animation.
    // Force-reset by feeding the cursor a DECRST 12 sequence.
    if (!ts.cursorBlink) terminal.write('\x1b[?12l')
  }
  resize()
}, { deep: true })

// Watch for background image toggling to update terminal transparency
watch(
  () => localStateStore.state.backgroundEnabled,
  () => applyXtermTheme(settingsStore.settings.terminal.theme || 'dark')
)

// Detach terminal before Vue nulls template refs.
// In Vue 3, onUnmounted fires AFTER template refs are set to null,
// so detachTerminal would see terminalRef.value === null and skip.
// onBeforeUnmount fires while refs are still valid.
onBeforeUnmount(() => {
  if (props.sessionId && terminalRef.value) {
    detachTerminal(props.sessionId, terminalRef.value)
  }
})

onUnmounted(() => {
  resizeObserver?.disconnect()
  intersectionObserver?.disconnect()

  // Dispose per-component listeners BEFORE releasing terminal.
  // The terminal instance may outlive this component if another
  // component still holds a reference.
  onDataDispose?.dispose()
  onDataDispose = null
  keyHandlerDispose?.dispose()
  keyHandlerDispose = null

  // Release reference (delayed dispose: terminal survives 500ms)
  if (props.sessionId) {
    releaseTerminal(props.sessionId, terminalInstanceRef)
  }

  unsubscribe?.()
  statusUnsubscribe?.()
  if (onDocumentMouseDown) {
    document.removeEventListener('mousedown', onDocumentMouseDown)
    onDocumentMouseDown = null
  }
  if (onTerminalAuxClick) {
    document.removeEventListener('auxclick', onTerminalAuxClick)
    onTerminalAuxClick = null
  }
  window.removeEventListener('resize', onWindowResize)
  window.removeEventListener('split:resize-start', onSplitResizeStart)
  window.removeEventListener('split:resize-end', onSplitResizeEnd)
  if (onOpenSearch) window.removeEventListener('terminal:open-search', onOpenSearch)
  if (onExport) window.removeEventListener('terminal:export', onExport)
  if (onSendRz) window.removeEventListener('terminal:send-rz', onSendRz)
  suggestions.close()
  if (!zmodemStore.getActiveTransfer(props.sessionId || '')) {
    disposeZmodemService(props.sessionId || '')
  }
})

// Paste handling
function pasteToTerminal(text: string) {
  if (props.mode === 'sftp' && terminal) {
    for (const char of text) {
      const code = char.charCodeAt(0)
      if (code >= 32 && code <= 126) {
        inputBuffer += char
        terminal.write(char)
      }
    }
  }
}

// Wrap in bracketed-paste markers when the app enabled the mode, so vim etc.
// don't re-indent each pasted line.
function bracketPaste(text: string): string {
  const sid = props.sessionId
  const managed = sid ? getManagedTerminal(sid) : undefined
  if (managed?.terminal.modes.bracketedPasteMode) {
    return '\x1b[200~' + text + '\x1b[201~'
  }
  return text
}

async function pasteToSession(text: string) {
  if (props.mode === 'ssh' || props.mode === 'local') {
    const sid = props.sessionId
    if (sid) {
      // Normalize line endings: \r\n -> \r to prevent extra newlines
      // when pasting into editors like vim (Windows clipboard often has \r\n)
      const normalized = text.replace(/\r\n/g, '\n').replace(/\r/g, '')
      SessionWrite(sid, bracketPaste(normalized))
    }
  }
}

const menu = useTerminalMenu({
  getSelection,
  onPaste: async (text) => {
    if (props.mode === 'ssh' || props.mode === 'local') {
      if (props.broadcastActive && props.workspaceId) {
        const targets = tabStore.getBroadcastPanelIdsInWorkspace(props.workspaceId)
        if (targets.length > 0) {
          const filtered = filterTerminalInput(text, false)
          for (const pid of targets) {
            const p = panelStore.getPanel(pid)
            if (p?.sessionId && (p.type === 'ssh' || p.type === 'local')) {
              // Bracket per target session — each has its own paste mode.
              const wrap = getManagedTerminal(p.sessionId)?.terminal.modes.bracketedPasteMode
              SessionWrite(p.sessionId, wrap ? '\x1b[200~' + filtered + '\x1b[201~' : filtered)
            }
          }
        } else {
          await pasteToSession(text)
        }
      } else {
        await pasteToSession(text)
      }
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

defineExpose({
  getSelection,
  resize,
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
  min-height: 0;
  overflow: hidden;
}

/* Search bar */
.terminal-search-bar {
  position: absolute;
  top: 8px;
  right: 8px;
  display: flex;
  align-items: center;
  gap: 4px;
  background: rgba(20, 23, 29, 0.88);
  backdrop-filter: blur(8px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-md);
  padding: 4px 6px;
  z-index: 50;
}
.search-input {
  width: 160px;
  background: transparent;
  border: none;
  outline: none;
  color: var(--text-primary);
  font-family: var(--font-ui);
  font-size: 12px;
  padding: 2px 4px;
}
.search-input::placeholder {
  color: var(--text-muted);
}
.search-count {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
  min-width: 32px;
  text-align: center;
}
.search-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s;
}
.search-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  color: var(--text-primary);
}
.terminal-area :deep(.xterm) {
  width: 100%;
  height: 100%;
  display: block;
  box-sizing: border-box;
  padding: 4px;
}
.terminal-area :deep(.xterm),
.terminal-area :deep(.xterm-viewport) {
  background: var(--bg-base);
}
.terminal-area :deep(.xterm-viewport) {
  overflow-y: scroll !important;
}
.terminal-area :deep(.xterm-viewport::-webkit-scrollbar) {
  width: 8px;
}
.terminal-area :deep(.xterm-viewport::-webkit-scrollbar-track) {
  background: var(--bg-elevated);
}
.terminal-area :deep(.xterm-viewport::-webkit-scrollbar-thumb) {
  background: var(--scrollbar-thumb);
  border-radius: 10px;
}
.terminal-area :deep(.xterm-viewport::-webkit-scrollbar-thumb:hover) {
  background: var(--scrollbar-thumb-hover);
}

.context-menu {
  position: fixed;
  z-index: 9999;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
  min-width: 120px;
  padding: 4px;
  backdrop-filter: blur(8px);
}

.menu-item {
  padding: 7px 14px;
  font-size: 12px;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  cursor: pointer;
  user-select: none;
  border-radius: var(--radius-sm);
  transition: all 0.1s ease;
}

.menu-item:hover:not(.disabled) {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.menu-item.disabled {
  color: var(--text-disabled);
  cursor: default;
}

.drop-overlay {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.45);
  pointer-events: none;
}
.drop-overlay span {
  font-size: 14px;
  color: #fff;
  padding: 12px 24px;
  border: 2px dashed rgba(255, 255, 255, 0.6);
  border-radius: var(--radius-md);
}
</style>
