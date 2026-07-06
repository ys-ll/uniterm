<template>
  <div class="sftp-tab-content">
    <div class="panes-area">
      <div
        class="local-pane"
        @dragover.prevent="onDragOver"
        @dragenter.prevent="onDragEnter('local')"
        @dragleave="onDragLeave('local')"
        @drop.capture="onDropLocal"
      >
        <div v-if="dragOverLocal" class="drop-overlay">
          <span>{{ t('sftp.dropHere') }}</span>
        </div>
        <SFTPPathBreadcrumb :path="localCwd" :drives="localDrives" bookmark-mode="local" :saved-paths="settingsStore.sftpBookmarks.localPaths" @navigate="onLocalNavigate" @save-bookmark="onSaveBookmark('local', $event)" @remove-bookmark="onRemoveBookmark('local', $event)" />
        <SFTPFileList
          mode="local"
          :files="localFiles"
          :loading="loadingLocal"
          :paste-loading="pasteLoadingLocal"
          :cut-item-names="localCutItemNames"
          :clipboard-count="localClipboardCount"
          :clipboard-mode="localClipboard?.mode"
          @navigate="onLocalNavigate"
          @send-to-other="onSendToRemote"
          @rename="(item: FileItem) => { dialogMode = 'local'; onRename(item) }"
          @delete="(items: FileItem[]) => { dialogMode = 'local'; onDelete(items) }"
          @refresh="onRefreshLocal"
          @mkdir="() => { dialogMode = 'local'; onMkdir() }"
          @edit="onLocalEditFile"
          @new-file="onLocalNewFile"
          @copy-to-clipboard="onLocalCopyToClipboard"
          @cut-to-clipboard="onLocalCutToClipboard"
          @paste="onLocalPaste"
          @cancel-paste="onLocalCancelPaste"
          @clear-clipboard="onLocalClearClipboard"
          @open="onLocalEditFile"
          @cancel-load="onCancelLoadLocal"
        />
      </div>
      <div
        class="remote-pane"
        @dragover.prevent="onDragOver"
        @dragenter.prevent="onDragEnter('remote')"
        @dragleave="onDragLeave('remote')"
        @drop.capture="onDropRemote"
      >
        <div v-if="dragOverRemote" class="drop-overlay">
          <span>{{ t('sftp.dropHere') }}</span>
        </div>
        <SFTPPathBreadcrumb :path="cwd" bookmark-mode="remote" :saved-paths="settingsStore.sftpBookmarks.remotePaths" @navigate="onRemoteNavigate" @save-bookmark="onSaveBookmark('remote', $event)" @remove-bookmark="onRemoveBookmark('remote', $event)" />
        <SFTPFileList
          mode="remote"
          :files="remoteFiles"
          :loading="loadingRemote"
          :paste-loading="pasteLoadingRemote"
          :cut-item-names="cutItemNames"
          :clipboard-count="clipboardCount"
          :clipboard-mode="clipboard?.mode"
          @navigate="onRemoteNavigate"
          @send-to-other="onSendToLocal"
          @rename="(item: FileItem) => { dialogMode = 'remote'; onRename(item) }"
          @delete="(items: FileItem[]) => { dialogMode = 'remote'; onDelete(items) }"
          @refresh="onRefreshRemote"
          @mkdir="() => { dialogMode = 'remote'; onMkdir() }"
          @chmod="(item: FileItem) => { dialogMode = 'remote'; onChmod(item) }"
          @upload="onUpload"
          @download-to="onDownloadTo"
          @edit="onEditFile"
          @new-file="onNewFile"
          @copy-to-clipboard="onCopyToClipboard"
          @cut-to-clipboard="onCutToClipboard"
          @paste="onPaste"
          @clear-clipboard="onClearClipboard"
          @cancel-paste="onCancelPaste"
          @open="onEditFile"
          @cancel-load="onCancelLoadRemote"
        />
      </div>
    </div>
    <SFTPTransferProgress :tasks="transferTasks" @cancel="onCancelTransfer" @pause="onPauseTransfer" @resume="onResumeTransfer" />

    <!-- Custom Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="400px"
      :close-on-click-modal="false"
      @closed="onDialogClosed"
    >
      <template v-if="dialogType === 'delete'">
        <p>{{ dialogMessage }}</p>
      </template>
      <template v-else-if="dialogType === 'chmod'">
        <div class="chmod-file-info">
          <span class="chmod-filename">{{ dialogItem?.name }}</span>
          <span v-if="dialogItem?.owner || dialogItem?.group" class="chmod-ownergroup">{{ dialogItem?.owner || '-' }}:{{ dialogItem?.group || '-' }}</span>
        </div>
        <el-form class="chmod-form" label-width="80px">
          <el-form-item label="Owner">
            <el-checkbox v-model="chmodOwnerR">Read</el-checkbox>
            <el-checkbox v-model="chmodOwnerW">Write</el-checkbox>
            <el-checkbox v-model="chmodOwnerX">Execute</el-checkbox>
          </el-form-item>
          <el-form-item label="Group">
            <el-checkbox v-model="chmodGroupR">Read</el-checkbox>
            <el-checkbox v-model="chmodGroupW">Write</el-checkbox>
            <el-checkbox v-model="chmodGroupX">Execute</el-checkbox>
          </el-form-item>
          <el-form-item label="Other">
            <el-checkbox v-model="chmodOtherR">Read</el-checkbox>
            <el-checkbox v-model="chmodOtherW">Write</el-checkbox>
            <el-checkbox v-model="chmodOtherX">Execute</el-checkbox>
          </el-form-item>
          <el-form-item :label="t('sftp.dialog.chmodOctal')">
            <el-input
              v-model="chmodOctalInput"
              class="chmod-octal-input"
              maxlength="3"
              :placeholder="'644'"
              @input="onOctalInput"
            />
          </el-form-item>
        </el-form>
      </template>
      <template v-else>
        <el-input v-model="dialogInput" :placeholder="dialogPlaceholder" @keyup.enter="onDialogConfirm" />
      </template>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('sftp.dialog.cancel') }}</el-button>
        <el-button type="primary" @click="onDialogConfirm">{{ t('sftp.dialog.confirm') }}</el-button>
      </template>
    </el-dialog>

    <!-- Editor Dialog -->
    <el-dialog
      v-model="editorVisible"
      :title="editorTitle"
      width="80%"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <div class="editor-container">
        <div ref="editorLineNumbers" class="editor-line-numbers">{{ editorLineNumbersText }}</div>
        <textarea
          ref="editorTextarea"
          v-model="editorContent"
          class="editor-textarea"
          :class="{ 'editor-textarea-wrap': editorWrapEnabled }"
          spellcheck="false"
          :wrap="editorWrapEnabled ? 'soft' : 'off'"
          @scroll="onEditorScroll"
          @input="onEditorInput"
        ></textarea>
      </div>
      <div ref="editorMirror" class="editor-mirror" aria-hidden="true"></div>
      <template #footer>
        <div class="editor-footer">
          <div class="editor-footer-left">
            <el-checkbox v-model="editorWrapEnabled">{{ t('sftp.edit.wrap') }}</el-checkbox>
            <el-select v-model="editorEncoding" style="width: 100px">
              <el-option label="UTF-8" value="utf-8" />
              <el-option label="UTF-16 LE" value="utf-16le" />
              <el-option label="UTF-16 BE" value="utf-16be" />
              <el-option label="GBK" value="gbk" />
            </el-select>
            <el-select v-model="editorLineEnding" style="width: 140px">
              <el-option label="LF (Linux/macOS)" value="lf" />
              <el-option label="CRLF (Windows)" value="crlf" />
              <el-option label="CR (old Mac)" value="cr" />
            </el-select>
          </div>
          <div class="editor-footer-buttons">
            <el-button @click="editorVisible = false">{{ t('sftp.dialog.cancel') }}</el-button>
            <el-button type="primary" :loading="editorSaving" @click="onEditorSave">{{ t('sftp.edit.save') }}</el-button>
          </div>
        </div>
      </template>
    </el-dialog>

    <!-- Conflict Dialog -->
    <el-dialog
      v-model="conflictVisible"
      :title="t('sftp.dialog.conflictTitle')"
      width="450px"
      :close-on-click-modal="false"
    >
      <p>{{ t('sftp.dialog.conflictPrompt') }}</p>
      <ul class="conflict-list">
        <li v-for="f in conflictFiles" :key="f">{{ f }}</li>
      </ul>
      <template #footer>
        <el-button @click="onConflictResolve('cancel')">{{ t('sftp.dialog.cancel') }}</el-button>
        <el-button @click="onConflictResolve('overwrite')">{{ t('sftp.dialog.conflictOverwrite') }}</el-button>
        <el-button type="primary" @click="onConflictResolve('rename')">{{ t('sftp.dialog.conflictRename') }}</el-button>
      </template>
    </el-dialog>

    <!-- New File Dialog -->
    <el-dialog
      v-model="newFileVisible"
      :title="t('sftp.dialog.newFileTitle')"
      width="400px"
      :close-on-click-modal="false"
      destroy-on-close
      @closed="newFileError = ''"
    >
      <el-input
        v-model="newFileName"
        :placeholder="t('sftp.dialog.newFilePrompt')"
        @keyup.enter="onNewFileCreate"
      />
      <p v-if="newFileError" style="color: var(--el-color-danger); margin-top: 8px;">{{ newFileError }}</p>
      <template #footer>
        <el-button @click="newFileVisible = false">{{ t('sftp.dialog.cancel') }}</el-button>
        <el-button type="primary" :loading="newFileCreating" @click="onNewFileCreate">{{ t('sftp.dialog.confirm') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, onActivated, onDeactivated, watch, nextTick } from 'vue'

import { msg } from '../services/message'
import { usePanelStore } from '../stores/panelStore'
import { useSettingsStore } from '../stores/settingsStore'
import { useI18n } from '../i18n'
import {
  SftpListRemote, SftpListLocal, SftpListLocalDrives,
  SftpChangeRemoteDir, SftpChangeLocalDir,
  SftpMakeDir, SftpRemove, SftpRename, SftpChmod,
  SftpLocalRemove, SftpLocalRename, SftpLocalMkdir,
  SftpLocalGetContent, SftpLocalPutContent, SftpLocalCopy, SftpLocalMove,
  SftpGet, SftpPut, SftpPutContent, SftpGetContent, SftpCopy, SftpMove,
  WriteTempFile, SftpCancelTransfer, SftpPauseTransfer, SftpResumeTransfer, ListSessions, GetDesktopPath,
  OpenMultipleFilesDialog, OpenDirectoryDialog,
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime'
import SFTPPathBreadcrumb from './SFTPPathBreadcrumb.vue'
import SFTPFileList from './SFTPFileList.vue'
import SFTPTransferProgress from './SFTPTransferProgress.vue'
import type { FileItem } from './SFTPFileList.vue'
import type { TransferTaskUI } from '../stores/panelStore'

const props = defineProps<{
  panelId: string
}>()

const panelStore = usePanelStore()
const settingsStore = useSettingsStore()
const transferTasks = panelStore.getTransferTasks(props.panelId)
const { t } = useI18n()
const panel = computed(() => panelStore.getPanel(props.panelId))

const localCwd = ref('/')
const cwd = ref('/')
const localFiles = ref<FileItem[]>([])
const remoteFiles = ref<FileItem[]>([])
const localDrives = ref<string[]>([])
const loadingLocal = ref(false)
const loadingRemote = ref(false)
let loadVersionLocal = 0
let loadVersionRemote = 0
const pasteLoadingLocal = ref(false)
const pasteLoadingRemote = ref(false)
const dragOverLocal = ref(false)
const dragOverRemote = ref(false)
const dragSource = ref<'local' | 'remote' | null>(null)
let dragEnterLocalCount = 0
let dragEnterRemoteCount = 0
let dragDroppedInternally = false
let draggedRemoteItems: FileItem[] = []
const dialogMode = ref<'local' | 'remote'>('remote')

function joinPath(base: string, name: string): string {
  if (base.endsWith('/') || base.endsWith('\\')) return base + name
  return base + '/' + name
}

function fromBase64(b64: string): Uint8Array {
  const binary = atob(b64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}

type Encoding = 'utf-8' | 'utf-16le' | 'utf-16be' | 'gbk'
type LineEnding = 'lf' | 'crlf' | 'cr'

function detectEncoding(bytes: Uint8Array): { encoding: Encoding, hasBom: boolean } {
  if (bytes.length >= 2) {
    if (bytes[0] === 0xFF && bytes[1] === 0xFE) return { encoding: 'utf-16le', hasBom: true }
    if (bytes[0] === 0xFE && bytes[1] === 0xFF) return { encoding: 'utf-16be', hasBom: true }
  }
  if (bytes.length >= 3 && bytes[0] === 0xEF && bytes[1] === 0xBB && bytes[2] === 0xBF) {
    return { encoding: 'utf-8', hasBom: true }
  }
  let nullCount = 0
  const checkLen = Math.min(bytes.length, 1024)
  for (let i = 0; i < checkLen; i++) { if (bytes[i] === 0) nullCount++ }
  if (nullCount > checkLen * 0.3) return { encoding: 'utf-16le', hasBom: false }

  // Try UTF-8 first (most common). Use non-fatal mode so valid UTF-8
  // sequences are accepted even if some bytes might be interpreted as
  // legacy encodings. WebView2's TextDecoder(fatal:true) may be overly
  // strict for certain byte sequences.
  try {
    const sample = bytes.slice(0, 8192)
    // Quick check: valid UTF-8 lead/trail byte pattern
    let i = 0
    let valid = true
    while (i < sample.length) {
      const b = sample[i]
      if (b < 0x80) { i++; continue }
      let trailing: number
      if ((b & 0xE0) === 0xC0) trailing = 1
      else if ((b & 0xF0) === 0xE0) trailing = 2
      else if ((b & 0xF8) === 0xF0) trailing = 3
      else { valid = false; break }
      if (i + trailing >= sample.length) { valid = false; break }
      for (let j = 1; j <= trailing; j++) {
        if ((sample[i + j] & 0xC0) !== 0x80) { valid = false; break }
      }
      if (!valid) break
      i += trailing + 1
    }
    if (valid) return { encoding: 'utf-8', hasBom: false }
  } catch { /* fall through */ }

  // Try GBK/GB18030 — common for Chinese text on Windows
  return { encoding: 'gbk', hasBom: false }
}

function detectLineEnding(text: string): LineEnding {
  let crlf = 0, lf = 0, cr = 0
  for (let i = 0; i < text.length; i++) {
    if (text[i] === '\r' && text[i + 1] === '\n') { crlf++; i++ }
    else if (text[i] === '\n') lf++
    else if (text[i] === '\r') cr++
  }
  if (crlf > lf && crlf > cr) return 'crlf'
  if (cr > lf && cr > crlf) return 'cr'
  return 'lf'
}

function decodeContent(bytes: Uint8Array, enc: Encoding): string {
  if (enc === 'gbk') {
    try { return new TextDecoder('gbk').decode(bytes) }
    catch { return new TextDecoder('gb18030').decode(bytes) }
  }
  return new TextDecoder(enc === 'utf-16le' ? 'utf-16le' : enc === 'utf-16be' ? 'utf-16be' : 'utf-8').decode(bytes)
}

function encodeContent(text: string, enc: Encoding, lineEnding: LineEnding): string {
  let normalized = text
  if (lineEnding === 'crlf') normalized = text.replace(/\r\n/g, '\n').replace(/\n/g, '\r\n')
  else if (lineEnding === 'cr') normalized = text.replace(/\r\n/g, '\n').replace(/\n/g, '\r')
  else normalized = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n')

  if (enc === 'utf-8' || enc === 'gbk') {
    // Always encode as UTF-8 here. The backend will re-encode to GBK
    // when needed (TextEncoder only supports UTF-8 in browsers).
    return toBase64(normalized)
  }
  const buf = new Uint8Array(normalized.length * 2 + 2)
  let pos = 0
  buf[pos++] = enc === 'utf-16le' ? 0xFF : 0xFE
  buf[pos++] = enc === 'utf-16le' ? 0xFE : 0xFF
  for (let i = 0; i < normalized.length; i++) {
    const code = normalized.charCodeAt(i)
    buf[pos++] = enc === 'utf-16le' ? (code & 0xFF) : ((code >> 8) & 0xFF)
    buf[pos++] = enc === 'utf-16le' ? ((code >> 8) & 0xFF) : (code & 0xFF)
  }
  let binary = ''
  for (let i = 0; i < pos; i++) binary += String.fromCharCode(buf[i])
  return btoa(binary)
}

function toBase64(str: string): string {
  const bytes = new TextEncoder().encode(str)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary)
}

// Dialog state
const dialogVisible = ref(false)
const dialogType = ref<'rename' | 'mkdir' | 'chmod' | 'delete'>('rename')
const dialogTitle = ref('')
const dialogMessage = ref('')
const dialogInput = ref('')
const dialogPlaceholder = ref('')
const dialogItem = ref<FileItem | null>(null)
const dialogItems = ref<FileItem[]>([])

// Clipboard state
interface Clipboard {
  items: string[]
  sourceDir: string
  mode: 'copy' | 'cut'
}
const clipboard = ref<Clipboard | null>(null)
const localClipboard = ref<Clipboard | null>(null)
const cutItemNames = computed(() =>
  clipboard.value?.mode === 'cut' ? clipboard.value.items : []
)
const localCutItemNames = computed(() =>
  localClipboard.value?.mode === 'cut' ? localClipboard.value.items : []
)
const clipboardCount = computed(() => clipboard.value?.items.length ?? 0)
const localClipboardCount = computed(() => localClipboard.value?.items.length ?? 0)

// Editor dialog state
const editorVisible = ref(false)
const editorTitle = ref('')
const editorPath = ref('')
const editorMode = ref<'local' | 'remote'>('remote')
const editorContent = ref('')
const editorRawBytes = ref<Uint8Array | null>(null)
const editorSaving = ref(false)
const editorLineNumbers = ref<HTMLElement | null>(null)
const editorTextarea = ref<HTMLTextAreaElement | null>(null)
const editorMirror = ref<HTMLDivElement | null>(null)
const editorWrapEnabled = ref(true)
const editorEncoding = ref<Encoding>('utf-8')
const editorLineEnding = ref<LineEnding>('lf')
const editorVisualLines = ref<number[]>([])
const LINE_HEIGHT = 24

const editorLineCount = computed(() => {
  if (!editorContent.value) return 1
  return (editorContent.value.match(/\n/g) || []).length + 1
})

const editorLineNumbersText = computed(() => {
  if (editorWrapEnabled.value && editorVisualLines.value.length > 0) {
    let lastNum = -1
    const lines = editorVisualLines.value.map(n => {
      const isFirst = n !== lastNum
      lastNum = n
      return isFirst ? String(n) : ''
    })
    return lines.join('\n')
  }
  const lines: string[] = []
  const count = editorLineCount.value
  for (let i = 1; i <= count; i++) {
    lines.push(String(i))
  }
  return lines.join('\n')
})

const editorMinHeight = computed(() => '')

// Re-decode content when user switches encoding
watch(editorEncoding, (newEnc) => {
  if (editorRawBytes.value) {
    editorContent.value = decodeContent(editorRawBytes.value, newEnc)
  }
})

// Sync editorMode from the source that set it (remote edit sets 'remote', local sets 'local')

function computeVisualLines() {
  if (!editorWrapEnabled.value || !editorMirror.value || !editorTextarea.value) return
  const textareaWidth = editorTextarea.value.clientWidth
  if (textareaWidth <= 0) return

  const lines = editorContent.value.split('\n')
  const result: number[] = []
  const mirror = editorMirror.value
  mirror.style.width = textareaWidth + 'px'
  const cs = getComputedStyle(editorTextarea.value!)
  mirror.style.fontFamily = cs.fontFamily
  mirror.style.fontSize = cs.fontSize
  mirror.style.lineHeight = cs.lineHeight

  for (let i = 0; i < lines.length; i++) {
    const testEl = document.createElement('div')
    testEl.textContent = lines[i] || ' '
    mirror.appendChild(testEl)
    const h = testEl.offsetHeight
    mirror.removeChild(testEl)
    const visualLines = Math.max(1, Math.round(h / LINE_HEIGHT))
    for (let j = 0; j < visualLines; j++) {
      result.push(i + 1)
    }
  }
  editorVisualLines.value = result
}

function onEditorInput() {
  if (editorWrapEnabled.value) {
    requestAnimationFrame(computeVisualLines)
  }
}

function onEditorScroll() {
  if (editorLineNumbers.value && editorTextarea.value) {
    editorLineNumbers.value.scrollTop = editorTextarea.value.scrollTop
  }
}

watch(editorWrapEnabled, (enabled) => {
  if (enabled) {
    nextTick(() => {
      requestAnimationFrame(computeVisualLines)
    })
  } else {
    editorVisualLines.value = []
  }
})

watch(editorVisible, (visible) => {
  if (!visible) {
    editorWrapEnabled.value = true
    editorVisualLines.value = []
  }
})

// New File dialog state
const newFileVisible = ref(false)
const newFileName = ref('newfile.txt')
const newFileMode = ref<'local' | 'remote'>('remote')
const newFileError = ref('')
const newFileCreating = ref(false)

// Conflict dialog state
const conflictVisible = ref(false)
const conflictFiles = ref<string[]>([])
const conflictResolve = ref<((action: 'overwrite' | 'rename' | 'cancel') => void) | null>(null)

// Chmod checkbox state
const chmodOwnerR = ref(false)
const chmodOwnerW = ref(false)
const chmodOwnerX = ref(false)
const chmodGroupR = ref(false)
const chmodGroupW = ref(false)
const chmodGroupX = ref(false)
const chmodOtherR = ref(false)
const chmodOtherW = ref(false)
const chmodOtherX = ref(false)

const chmodOctal = computed(() => {
  const o = (chmodOwnerR.value ? 4 : 0) + (chmodOwnerW.value ? 2 : 0) + (chmodOwnerX.value ? 1 : 0)
  const g = (chmodGroupR.value ? 4 : 0) + (chmodGroupW.value ? 2 : 0) + (chmodGroupX.value ? 1 : 0)
  const t = (chmodOtherR.value ? 4 : 0) + (chmodOtherW.value ? 2 : 0) + (chmodOtherX.value ? 1 : 0)
  return String(o) + String(g) + String(t)
})

// Editable octal input (e.g. "644"); kept in sync with the checkbox grid both ways.
const chmodOctalInput = ref('000')

function applyOctal(digits: string) {
  const nums = digits.split('').map(Number)
  chmodOwnerR.value = !!(nums[0] & 4); chmodOwnerW.value = !!(nums[0] & 2); chmodOwnerX.value = !!(nums[0] & 1)
  chmodGroupR.value = !!(nums[1] & 4); chmodGroupW.value = !!(nums[1] & 2); chmodGroupX.value = !!(nums[1] & 1)
  chmodOtherR.value = !!(nums[2] & 4); chmodOtherW.value = !!(nums[2] & 2); chmodOtherX.value = !!(nums[2] & 1)
}

// User typing in the octal field: strip invalid chars, apply once 3 digits are present.
function onOctalInput() {
  chmodOctalInput.value = chmodOctalInput.value.replace(/[^0-7]/g, '').slice(0, 3)
  if (chmodOctalInput.value.length === 3) applyOctal(chmodOctalInput.value)
}

// Checkbox grid -> octal field. No feedback loop: when applyOctal sets the grid,
// the value written back equals what the user typed, so the field is unchanged.
watch(chmodOctal, (v) => { chmodOctalInput.value = v })

function parseMode(mode: string) {
  // mode example: "drwxr-xr-x" or "-rw-r--r--" — strip leading file type char
  const m = mode.length >= 10 ? mode.slice(1) : mode
  chmodOwnerR.value = m[0] === 'r'
  chmodOwnerW.value = m[1] === 'w'
  chmodOwnerX.value = m[2] === 'x' || m[2] === 's'
  chmodGroupR.value = m[3] === 'r'
  chmodGroupW.value = m[4] === 'w'
  chmodGroupX.value = m[5] === 'x' || m[5] === 's'
  chmodOtherR.value = m[6] === 'r'
  chmodOtherW.value = m[7] === 'w'
  chmodOtherX.value = m[8] === 'x' || m[8] === 't'
  chmodOctalInput.value = chmodOctal.value
}

let unsubscribe: (() => void) | null = null
let unsubscribeStatus: (() => void) | null = null
let initialNavDone = false

onMounted(async () => {
  unsubscribeStatus = EventsOn('session:status', (payload: { id: string; status: string }) => {
    if (payload.id === panel.value?.sessionId) {
      if (payload.status === 'connected') {
        onRefreshLocal()
        onRefreshRemote().then(() => doInitialAutoNav())
      } else if (payload.status === 'error') {
        msg.error(t('sftp.connectError'))
      }
    }
  })

  unsubscribe = EventsOn('session:data', (payload: { id: string; data: string }) => {
    if (payload.id !== panel.value?.sessionId) return
    // Connection failed messages from backend (SFTP/FTP async connect errors)
    const connMatch = payload.data.match(/\[Connection failed: ([^\]]+)\]/)
    if (connMatch) {
      msg.error(connMatch[1])
      return
    }
    const match = payload.data.match(/\x1b\]633;S([^\x07]*)\x07/)
    if (!match) return
    try {
      const msg = JSON.parse(match[1])
      if (msg.type === 'sftp:transfer') {
        if (msg.event === 'start') {
          const existing = transferTasks.find(t => t.id === msg.taskId)
          if (existing) {
            existing.status = 'running'
            existing.speed = ''
            existing.eta = ''
            existing.lastBytes = 0
            existing.lastTime = Date.now()
          } else {
            transferTasks.push({
              id: msg.taskId,
              type: msg.tfType,
              name: msg.name,
              percentage: 0,
              speed: '',
              eta: '',
              status: 'running',
              lastBytes: 0,
              lastTime: Date.now(),
              total: msg.total || 0
            })
          }
        } else if (msg.event === 'progress') {
          const existing = transferTasks.find(t => t.id === msg.taskId)
          if (existing) {
            existing.total = msg.total || existing.total
            existing.percentage = existing.total > 0 ? Math.round((msg.progress / existing.total) * 100) : 0
            const now = Date.now()
            const elapsed = (now - existing.lastTime) / 1000
            if (elapsed >= 0.5) {
              const bytesSince = msg.progress - existing.lastBytes
              const bytesPerSec = bytesSince / elapsed
              existing.speed = formatSpeed(bytesPerSec)
              if (bytesPerSec > 0 && existing.total > 0) {
                const remaining = (existing.total - msg.progress) / bytesPerSec
                existing.eta = formatETA(remaining)
              }
              existing.lastBytes = msg.progress
              existing.lastTime = now
            }
          }
        } else if (msg.event === 'complete') {
          const existing = transferTasks.find(t => t.id === msg.taskId)
          if (existing) {
            const st = msg.status as string
            existing.status = st === 'done' ? 'done' : st === 'cancelled' ? 'cancelled' : st === 'paused' ? 'paused' : 'error'
            existing.percentage = existing.status === 'done' ? 100 : existing.percentage
            if (existing.status === 'done') {
              if (existing.type === 'download') {
                onRefreshLocal()
              } else {
                onRefreshRemote()
              }
            }
            if (existing.status !== 'running' && existing.status !== 'paused') {
              setTimeout(() => {
                const idx = transferTasks.findIndex(t => t.id === msg.taskId)
                if (idx >= 0) transferTasks.splice(idx, 1)
              }, 5000)
            }
          }
        }
      }
    } catch {}
  })

  // Proactively check if session is already connected (race: event may have fired before listener registered)
  const sid = panel.value?.sessionId
  if (sid) {
    fetchLocalDrives()
    try {
      const sessions = await ListSessions()
      const sess = sessions.find(s => s.id === sid)
      if (sess && sess.status === 'connected') {
        onRefreshLocal()
        onRefreshRemote().then(() => doInitialAutoNav())
      }
    } catch {}
  }
})

watch(() => panel.value?.sessionId, (newId, oldId) => {
  if (newId && !oldId) {
    fetchLocalDrives()
  }
})

onUnmounted(() => {
  unsubscribe?.()
  unsubscribeStatus?.()
})

// With KeepAlive, only the active instance should listen for global document
// drag/drop events to avoid cached instances picking up drops from other tabs.
onActivated(() => {
  document.addEventListener('dragstart', onDragStart)
  document.addEventListener('dragend', clearDragState)
  document.addEventListener('drop', onDocumentDrop)
})

onDeactivated(() => {
  document.removeEventListener('dragstart', onDragStart)
  document.removeEventListener('dragend', clearDragState)
  document.removeEventListener('drop', onDocumentDrop)
})

async function fetchLocalDrives() {
  const sid = panel.value?.sessionId
  if (!sid) return
  try {
    const drives = await SftpListLocalDrives(sid)
    localDrives.value = drives.map(d => d.name)
  } catch {}
}

function onCancelLoadLocal() {
  loadVersionLocal++
  loadingLocal.value = false
}

function onCancelLoadRemote() {
  loadVersionRemote++
  loadingRemote.value = false
}

async function onRefreshLocal() {
  const sid = panel.value?.sessionId
  if (!sid) return
  const version = ++loadVersionLocal
  loadingLocal.value = true
  try {
    const result = await SftpListLocal(sid, '')
    if (version !== loadVersionLocal) return
    localFiles.value = result.files
    localCwd.value = result.dir
    if (/^[A-Za-z]:\\$/.test(result.dir)) {
      try {
        const drives = await SftpListLocalDrives(sid)
        if (version !== loadVersionLocal) return
        localDrives.value = drives.map(d => d.name)
      } catch {}
    }
  } catch (e: any) {
    if (version !== loadVersionLocal) return
    msg.error(e?.toString() || 'Failed to list local files')
  } finally {
    if (version === loadVersionLocal) loadingLocal.value = false
  }
}

async function onRefreshRemote() {
  const sid = panel.value?.sessionId
  if (!sid) return
  const version = ++loadVersionRemote
  loadingRemote.value = true
  try {
    const result = await SftpListRemote(sid, '')
    if (version !== loadVersionRemote) return
    remoteFiles.value = result.files
    cwd.value = result.dir
  } catch (e: any) {
    if (version !== loadVersionRemote) return
    msg.error(e?.toString() || 'Failed to list remote files')
  } finally {
    if (version === loadVersionRemote) loadingRemote.value = false
  }
}

// Auto-navigate into the configured share/bucket on initial load.
async function doInitialAutoNav() {
  if (initialNavDone) return
  initialNavDone = true
  const cfg = panel.value?.config
  if (!cfg) return
  let target = ''
  if (cfg.type === 'smb' && cfg.smbShare) {
    target = '/' + cfg.smbShare
  } else if (cfg.type === 's3' && cfg.s3Bucket) {
    target = '/' + cfg.s3Bucket
  }
  if (target) {
    await onRemoteNavigate(target)
  }
}

async function onLocalNavigate(path: string) {
  const sid = panel.value?.sessionId
  if (!sid) return
  let fullPath: string
  if (path === '..') {
    const parts = localCwd.value.replace(/\\/g, '/').split('/').filter(Boolean)
    parts.pop()
    if (parts.length === 0) {
      fullPath = localCwd.value
    } else if (/^[A-Za-z]:$/.test(parts[0])) {
      fullPath = parts[0] + '\\' + parts.slice(1).join('\\')
    } else {
      fullPath = '/' + parts.join('/')
    }
  } else if (!path.startsWith('/') && !/^[A-Za-z]:/.test(path)) {
    fullPath = joinPath(localCwd.value, path)
  } else {
    fullPath = path
  }
  const version = ++loadVersionLocal
  loadingLocal.value = true
  try {
    const result = await SftpChangeLocalDir(sid, fullPath)
    if (version !== loadVersionLocal) return
    localFiles.value = result.files
    localCwd.value = result.dir
    if (/^[A-Za-z]:\\$/.test(result.dir)) {
      try {
        const drives = await SftpListLocalDrives(sid)
        if (version !== loadVersionLocal) return
        localDrives.value = drives.map(d => d.name)
      } catch {}
    }
  } catch (e: any) {
    if (version !== loadVersionLocal) return
    msg.error(e?.toString() || 'Failed to navigate')
  } finally {
    if (version === loadVersionLocal) loadingLocal.value = false
  }
}

async function onRemoteNavigate(path: string) {
  const sid = panel.value?.sessionId
  if (!sid) return
  let fullPath: string
  if (path === '..') {
    fullPath = cwd.value.split('/').filter(Boolean).slice(0, -1).join('/')
    fullPath = '/' + fullPath
  } else if (!path.startsWith('/')) {
    fullPath = joinPath(cwd.value, path)
  } else {
    fullPath = path
  }
  const version = ++loadVersionRemote
  loadingRemote.value = true
  try {
    const result = await SftpChangeRemoteDir(sid, fullPath)
    if (version !== loadVersionRemote) return
    remoteFiles.value = result.files
    cwd.value = result.dir
  } catch (e: any) {
    if (version !== loadVersionRemote) return
    msg.error(e?.toString() || 'Failed to navigate')
  } finally {
    if (version === loadVersionRemote) loadingRemote.value = false
  }
}

function onSaveBookmark(mode: 'local' | 'remote', path: string) {
  settingsStore.addSftpBookmark(mode, path)
}

function onRemoveBookmark(mode: 'local' | 'remote', path: string) {
  settingsStore.removeSftpBookmark(mode, path)
}

function formatSpeed(bytesPerSec: number): string {
  if (bytesPerSec < 1024) return Math.round(bytesPerSec) + ' B/s'
  if (bytesPerSec < 1024 * 1024) return (bytesPerSec / 1024).toFixed(1) + ' KB/s'
  if (bytesPerSec < 1024 * 1024 * 1024) return (bytesPerSec / (1024 * 1024)).toFixed(1) + ' MB/s'
  return (bytesPerSec / (1024 * 1024 * 1024)).toFixed(1) + ' GB/s'
}

function formatETA(seconds: number): string {
  if (seconds < 1) return ''
  if (seconds < 60) return Math.round(seconds) + 's'
  if (seconds < 3600) return Math.floor(seconds / 60) + 'm ' + Math.round(seconds % 60) + 's'
  return Math.floor(seconds / 3600) + 'h ' + Math.floor((seconds % 3600) / 60) + 'm'
}

async function onCancelTransfer(taskId: string) {
  const sid = panel.value?.sessionId
  if (!sid) return
  try {
    await SftpCancelTransfer(sid, taskId)
  } catch (e) {
    console.error('cancel transfer:', e)
  }
}

async function onPauseTransfer(taskId: string) {
  const sid = panel.value?.sessionId
  if (!sid) return
  try {
    await SftpPauseTransfer(sid, taskId)
  } catch (e) {
    console.error('pause transfer:', e)
  }
}

async function onResumeTransfer(taskId: string) {
  const sid = panel.value?.sessionId
  if (!sid) return
  try {
    await SftpResumeTransfer(sid, taskId)
  } catch (e) {
    console.error('resume transfer:', e)
  }
}

async function onSendToRemote(items: FileItem[]) {
  const sid = panel.value?.sessionId
  if (!sid) return

  const fileNames = items.filter(i => i.name !== '..').map(i => i.name)
  const action = await checkRemoteConflicts(fileNames)
  if (action === 'cancel') return

  const existingNames = remoteFiles.value.map(f => f.name)
  for (const item of items) {
    if (item.name === '..') continue
    let resolvedName = item.name
    if (action === 'rename' && existingNames.includes(item.name)) {
      resolvedName = autoRename(item.name, existingNames)
    }
    existingNames.push(resolvedName)
    const localPath = joinPath(localCwd.value, item.name)
    const remotePath = cwd.value + '/' + resolvedName
    SftpPut(sid, localPath, remotePath, item.isDir)
  }
}

function onSendToLocal(items: FileItem[]) {
  const sid = panel.value?.sessionId
  if (!sid) return
  for (const item of items) {
    if (item.name === '..') continue
    const remotePath = joinPath(cwd.value, item.name)
    const localPath = joinPath(localCwd.value, item.name).replace(/\\/g, '/')
    SftpGet(sid, remotePath, localPath, item.isDir)
  }
}

async function onUpload() {
  const sid = panel.value?.sessionId
  if (!sid) return
  try {
    const files = await OpenMultipleFilesDialog()
    if (!files || files.length === 0) return

    const fileNames = files.map(fp => fp.replace(/\\/g, '/').split('/').pop() || 'upload')
    const action = await checkRemoteConflicts(fileNames)
    if (action === 'cancel') return

    const existingNames = remoteFiles.value.map(f => f.name)
    for (const fp of files) {
      let name = fp.replace(/\\/g, '/').split('/').pop() || 'upload'
      if (action === 'rename' && existingNames.includes(name)) {
        name = autoRename(name, existingNames)
      }
      existingNames.push(name)
      SftpPut(sid, fp, cwd.value + '/' + name, false)
    }
  } catch (e) {
    console.error('upload:', e)
  }
}

async function onDownloadTo(items: FileItem[]) {
  const sid = panel.value?.sessionId
  if (!sid) return
  try {
    const dir = await OpenDirectoryDialog()
    if (!dir) return
    for (const item of items) {
      if (item.name === '..') continue
      const remotePath = joinPath(cwd.value, item.name)
      const localPath = (dir + '/' + item.name).replace(/\\/g, '/')
      SftpGet(sid, remotePath, localPath, item.isDir)
    }
  } catch (e) {
    console.error('downloadTo:', e)
  }
}

// --- Clipboard handlers ---

function onCopyToClipboard(items: FileItem[]) {
  clipboard.value = {
    items: items.map(i => i.name),
    sourceDir: cwd.value,
    mode: 'copy'
  }
  msg.success(t('sftp.copy'))
}

function onCutToClipboard(items: FileItem[]) {
  clipboard.value = {
    items: items.map(i => i.name),
    sourceDir: cwd.value,
    mode: 'cut'
  }
  msg.success(t('sftp.cut'))
}

function onClearClipboard() {
  clipboard.value = null
}

function onLocalCopyToClipboard(items: FileItem[]) {
  localClipboard.value = {
    items: items.map(i => i.name),
    sourceDir: localCwd.value,
    mode: 'copy'
  }
  msg.success(t('sftp.copy'))
}

function onLocalCutToClipboard(items: FileItem[]) {
  localClipboard.value = {
    items: items.map(i => i.name),
    sourceDir: localCwd.value,
    mode: 'cut'
  }
  msg.success(t('sftp.cut'))
}

function onLocalClearClipboard() {
  localClipboard.value = null
}

function autoRename(targetName: string, existingNames: string[]): string {
  if (!existingNames.includes(targetName)) return targetName
  const dotIdx = targetName.lastIndexOf('.')
  const base = dotIdx > 0 ? targetName.slice(0, dotIdx) : targetName
  const ext = dotIdx > 0 ? targetName.slice(dotIdx) : ''
  let n = 1
  let candidate: string
  do {
    candidate = `${base} (${n})${ext}`
    n++
  } while (existingNames.includes(candidate))
  return candidate
}

function isPathInside(child: string, parent: string): boolean {
  const c = child.endsWith('/') ? child : child + '/'
  const p = parent.endsWith('/') ? parent : parent + '/'
  return c.startsWith(p)
}

function showConflictDialog(conflicts: string[]): Promise<'overwrite' | 'rename' | 'cancel'> {
  return new Promise((resolve) => {
    conflictFiles.value = conflicts
    conflictResolve.value = resolve
    conflictVisible.value = true
  })
}

async function checkRemoteConflicts(fileNames: string[]): Promise<'overwrite' | 'rename' | 'cancel'> {
  const existingNames = remoteFiles.value.map(f => f.name)
  const conflicts = fileNames.filter(n => existingNames.includes(n))
  if (conflicts.length === 0) return 'overwrite'
  return showConflictDialog(conflicts)
}

function onConflictResolve(action: 'overwrite' | 'rename' | 'cancel') {
  conflictVisible.value = false
  if (conflictResolve.value) {
    conflictResolve.value(action)
    conflictResolve.value = null
  }
}

let pasteCancelled = false

function onCancelPaste() {
  pasteCancelled = true
  pasteLoadingRemote.value = false
}

async function onPaste() {
  const sid = panel.value?.sessionId
  if (!sid || !clipboard.value) return
  const { items, sourceDir, mode } = clipboard.value
  pasteCancelled = false
  pasteLoadingRemote.value = true
  const targetDir = cwd.value

  // Cut to same directory: error immediately, no conflict dialog
  if (mode === 'cut' && sourceDir === targetDir) {
    msg.warning(t('sftp.paste.cutSameDir'))
    pasteLoadingRemote.value = false
    return
  }

  const existingNames = remoteFiles.value.map(f => f.name)

  // Check for name conflicts
  const conflicts = items.filter(name => existingNames.includes(name))
  let resolveAction: 'overwrite' | 'rename' | 'cancel' = 'rename'
  if (conflicts.length > 0) {
    resolveAction = await showConflictDialog(conflicts)
    if (resolveAction === 'cancel') { pasteLoadingRemote.value = false; return }
  }

  let success = 0
  const failed: string[] = []

  for (const name of items) {
    if (pasteCancelled) break
    const source = joinPath(sourceDir, name)
    const target = joinPath(targetDir, name)
    // Same path: copy mode force auto-rename below
    if (source === target) {
      // copy mode (cut already blocked at top level)
    } else if (isPathInside(target, source)) {
    // Circular check (only when source !== target)
      msg.warning(t('sftp.paste.circularWarning'))
      continue
    }
    const forceRename = source === target && mode === 'copy'
    let resolvedName = name
    if (forceRename || (resolveAction === 'rename' && existingNames.includes(name))) {
      resolvedName = autoRename(name, existingNames)
    }
    const resolvedTarget = joinPath(targetDir, resolvedName)
    existingNames.push(resolvedName)
    try {
      if (mode === 'copy') {
        await SftpCopy(sid, source, resolvedTarget)
      } else {
        await SftpMove(sid, source, resolvedTarget)
      }
      success++
    } catch (e: any) {
      failed.push(name + ': ' + (e?.toString() || 'unknown'))
    }
  }

  pasteLoadingRemote.value = false

  if (pasteCancelled) {
    // keep clipboard so user can retry
  } else if (failed.length > 0) {
    msg.error(`Copied/Moved ${success}/${items.length}, ${failed.length} failed`)
  } else {
    msg.success(t('sftp.paste'))
  }

  if (!pasteCancelled) clipboard.value = null
  onRefreshRemote()
}

// --- Editor handlers ---

function isBinaryContent(bytes: Uint8Array): boolean {
  const sample = bytes.slice(0, 8192)
  if (!sample.length) return false
  let nullCount = 0
  let nonPrintable = 0
  for (let i = 0; i < sample.length; i++) {
    const c = sample[i]
    if (c < 0x09 || (c > 0x0D && c < 0x20)) nonPrintable++
  }
  if (nonPrintable > sample.length * 0.3) return true
  return false
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

async function onEditFile(item: FileItem) {
  if (item.isDir) return
  const sid = panel.value?.sessionId
  if (!sid) return

  if (item.size > 5 * 1024 * 1024) {
    msg.warning(t('sftp.edit.fileTooLarge'))
    return
  }
  if (item.size > 500 * 1024) {
    const ok = window.confirm(t('sftp.edit.fileLargeWarning', { size: formatFileSize(item.size) }))
    if (!ok) return
  }

  editorPath.value = joinPath(cwd.value, item.name)
  editorTitle.value = t('sftp.dialog.editTitle', { path: editorPath.value })
  editorContent.value = ''
  editorVisible.value = true

  try {
    const rawB64 = await SftpGetContent(sid, editorPath.value)
    const bytes = fromBase64(rawB64)
    if (isBinaryContent(bytes)) {
      editorVisible.value = false
      msg.warning(t('sftp.edit.binaryFile'))
      return
    }
    const detected = detectEncoding(bytes)
    editorEncoding.value = detected.encoding
    editorRawBytes.value = bytes
    editorMode.value = 'remote'
    const text = decodeContent(bytes, detected.encoding)
    editorLineEnding.value = detectLineEnding(text)
    editorContent.value = text
  } catch (e: any) {
    editorVisible.value = false
    msg.error(e?.toString() || 'Failed to read file')
  }
}

async function onEditorSave() {
  const sid = panel.value?.sessionId
  if (!sid) return
  editorSaving.value = true
  try {
    if (editorMode.value === 'local') {
      await SftpLocalPutContent(sid, editorPath.value, encodeContent(editorContent.value, editorEncoding.value, editorLineEnding.value), editorEncoding.value)
      onRefreshLocal()
    } else {
      await SftpPutContent(sid, editorPath.value, encodeContent(editorContent.value, editorEncoding.value, editorLineEnding.value), editorEncoding.value)
      onRefreshRemote()
    }
    msg.success(t('sftp.edit.saveSuccess'))
    editorVisible.value = false
  } catch (e: any) {
    msg.error(e?.toString() || 'Failed to save file')
  } finally {
    editorSaving.value = false
  }
}

// --- Local file handlers ---

async function onLocalEditFile(item: FileItem) {
  if (item.isDir) return
  const sid = panel.value?.sessionId
  if (!sid) return
  if (item.size > 5 * 1024 * 1024) {
    msg.warning(t('sftp.edit.fileTooLarge'))
    return
  }
  editorPath.value = joinPath(localCwd.value, item.name)
  editorTitle.value = t('sftp.dialog.editTitle', { path: editorPath.value })
  editorMode.value = 'local'
  editorContent.value = ''
  editorVisible.value = true
  try {
    const rawB64 = await SftpLocalGetContent(sid, editorPath.value)
    const bytes = fromBase64(rawB64)
    if (isBinaryContent(bytes)) { editorVisible.value = false; msg.warning(t('sftp.edit.binaryFile')); return }
    const detected = detectEncoding(bytes)
    editorEncoding.value = detected.encoding
    editorRawBytes.value = bytes
    const text = decodeContent(bytes, detected.encoding)
    editorLineEnding.value = detectLineEnding(text)
    editorContent.value = text
  } catch (e: any) {
    editorVisible.value = false
    msg.error(e?.toString() || 'Failed to read file')
  }
}

function onLocalNewFile() {
  newFileName.value = 'newfile.txt'
  newFileMode.value = 'local'
  newFileError.value = ''
  newFileVisible.value = true
}

let localPasteCancelled = false

function onLocalCancelPaste() {
  localPasteCancelled = true
  pasteLoadingLocal.value = false
}

async function onLocalPaste() {
  const sid = panel.value?.sessionId
  if (!sid || !localClipboard.value) return
  const { items, sourceDir, mode } = localClipboard.value
  localPasteCancelled = false
  pasteLoadingLocal.value = true
  const targetDir = localCwd.value
  if (mode === 'cut' && sourceDir === targetDir) {
    msg.warning(t('sftp.paste.cutSameDir'))
    pasteLoadingLocal.value = false
    return
  }
  const existingNames = localFiles.value.map(f => f.name)
  const conflicts = items.filter(n => existingNames.includes(n))
  let resolveAction: 'overwrite' | 'rename' | 'cancel' = 'rename'
  if (conflicts.length > 0) {
    resolveAction = await showConflictDialog(conflicts)
    if (resolveAction === 'cancel') { pasteLoadingLocal.value = false; return }
  }
  let success = 0
  const failed: string[] = []
  for (const name of items) {
    if (localPasteCancelled) break
    const source = joinPath(sourceDir, name)
    const target = joinPath(targetDir, name)
    if (source === target) { /* copy: auto-rename below */ }
    const forceRename = source === target && mode === 'copy'
    let resolvedName = name
    if (forceRename || (resolveAction === 'rename' && existingNames.includes(name))) {
      resolvedName = autoRename(name, existingNames)
    }
    const resolvedTarget = joinPath(targetDir, resolvedName)
    existingNames.push(resolvedName)
    try {
      if (mode === 'copy') await SftpLocalCopy(sid, source, resolvedTarget)
      else await SftpLocalMove(sid, source, resolvedTarget)
      success++
    } catch (e: any) { failed.push(name + ': ' + (e?.toString() || 'unknown')) }
  }
  pasteLoadingLocal.value = false
  if (!localPasteCancelled) {
    if (failed.length > 0) msg.error(`Copied/Moved ${success}/${items.length}, ${failed.length} failed`)
    else msg.success(t('sftp.paste'))
  }
  if (!localPasteCancelled) localClipboard.value = null
  onRefreshLocal()
}

// --- New File handlers ---

function onNewFile() {
  newFileName.value = 'newfile.txt'
  newFileMode.value = 'remote'
  newFileError.value = ''
  newFileVisible.value = true
}

async function onNewFileCreate() {
  const name = newFileName.value.trim()
  if (!name) { newFileError.value = t('sftp.dialog.newFileEmpty'); return }
  if (name.includes('/') || name.includes('\\')) { newFileError.value = t('sftp.dialog.newFileInvalid'); return }
  const sid = panel.value?.sessionId
  if (!sid) return
  const isLocal = newFileMode.value === 'local'
  const existingNames = (isLocal ? localFiles.value : remoteFiles.value).map(f => f.name)
  const finalName = autoRename(name, existingNames)
  const targetPath = joinPath(isLocal ? localCwd.value : cwd.value, finalName)
  newFileCreating.value = true
  try {
    if (isLocal) {
      await SftpLocalPutContent(sid, targetPath, '')
      onRefreshLocal()
    } else {
      await SftpPutContent(sid, targetPath, '')
      onRefreshRemote()
    }
    msg.success(t('sftp.dialog.confirm'))
    newFileVisible.value = false
  } catch (e: any) {
    msg.error(e?.toString() || 'Failed to create file')
  } finally {
    newFileCreating.value = false
  }
}

// Dialog helpers
function openDialog(
  type: 'rename' | 'mkdir' | 'chmod' | 'delete',
  title: string,
  inputValue: string = '',
  placeholder: string = '',
  message: string = ''
) {
  dialogType.value = type
  dialogTitle.value = title
  dialogInput.value = inputValue
  dialogPlaceholder.value = placeholder
  dialogMessage.value = message
  dialogVisible.value = true
}

function onDialogClosed() {
  dialogInput.value = ''
  dialogPlaceholder.value = ''
  dialogMessage.value = ''
  dialogItem.value = null
  dialogItems.value = []
}

async function onDialogConfirm() {
  dialogVisible.value = false
  const sid = panel.value?.sessionId
  if (!sid) return
  const isLocal = dialogMode.value === 'local'
  const baseDir = isLocal ? localCwd.value : cwd.value
  switch (dialogType.value) {
    case 'rename':
      if (dialogInput.value && dialogInput.value !== dialogItem.value?.name) {
        const oldPath = joinPath(baseDir, dialogItem.value!.name)
        const newPath = joinPath(baseDir, dialogInput.value)
        try {
          if (isLocal) {
            await SftpLocalRename(sid, oldPath, newPath)
            onRefreshLocal()
          } else {
            await SftpRename(sid, oldPath, newPath)
            onRefreshRemote()
          }
        } catch (e) { console.error('rename:', e) }
      }
      break
    case 'mkdir':
      if (dialogInput.value) {
        try {
          if (isLocal) {
            await SftpLocalMkdir(sid, joinPath(baseDir, dialogInput.value))
            onRefreshLocal()
          } else {
            await SftpMakeDir(sid, joinPath(baseDir, dialogInput.value))
            onRefreshRemote()
          }
        } catch (e) { console.error('mkdir:', e) }
      }
      break
    case 'chmod':
      try {
        await SftpChmod(sid, joinPath(baseDir, dialogItem.value!.name), chmodOctal.value)
        onRefreshRemote()
      } catch (e) { console.error('chmod:', e) }
      break
    case 'delete':
      for (const item of dialogItems.value) {
        const itemPath = joinPath(baseDir, item.name)
        try {
          if (isLocal) {
            await SftpLocalRemove(sid, itemPath, item.isDir)
          } else {
            await SftpRemove(sid, itemPath, item.isDir)
          }
        } catch (e) { console.error('delete item:', item.name, e) }
      }
      if (isLocal) {
        onRefreshLocal()
      } else {
        onRefreshRemote()
      }
      break
  }
}

function onRename(item: FileItem) {
  dialogItem.value = item
  openDialog(
    'rename',
    t('sftp.dialog.renameTitle'),
    item.name,
    t('sftp.dialog.renamePrompt', { name: item.name })
  )
}
function onDelete(items: FileItem[]) {
  dialogItems.value = items
  const hasDir = items.some(i => i.isDir)
  const hasFile = items.some(i => !i.isDir)
  let msg: string
  if (hasDir && hasFile) {
    msg = t('sftp.dialog.deleteConfirmMixed', { count: items.length })
  } else if (hasDir) {
    msg = t('sftp.dialog.deleteConfirmDir', { count: items.length })
  } else {
    msg = t('sftp.dialog.deleteConfirmFile', { count: items.length })
  }
  openDialog('delete', t('sftp.dialog.deleteTitle'), '', '', msg)
}
function onMkdir() {
  openDialog('mkdir', t('sftp.dialog.mkdirTitle'), '', t('sftp.dialog.mkdirPrompt'))
}
function onChmod(item: FileItem) {
  dialogItem.value = item
  parseMode(item.mode)
  openDialog(
    'chmod',
    t('sftp.dialog.chmodTitle'),
    '',
    t('sftp.dialog.chmodPrompt', { name: item.name })
  )
}

async function onDocumentDrop(e: DragEvent) {
  if (!dragDroppedInternally) {
    // If drop didn't fire on a pane but files are available, handle as external upload
    const files = e.dataTransfer?.files
    if (files && files.length > 0 && dragSource.value === null) {
      e.preventDefault()
      const sid = panel.value?.sessionId
      if (sid) {
        const fileNames: string[] = []
        for (let i = 0; i < files.length; i++) {
          fileNames.push(files[i].name)
        }
        const action = await checkRemoteConflicts(fileNames)
        if (action === 'cancel') return

        const existingNames = remoteFiles.value.map(f => f.name)
        for (let i = 0; i < files.length; i++) {
          const f = files[i]
          let resolvedName = f.name
          if (action === 'rename' && existingNames.includes(f.name)) {
            resolvedName = autoRename(f.name, existingNames)
          }
          existingNames.push(resolvedName)
          const remotePath = cwd.value + '/' + resolvedName
          const nativePath = (f as any).path
          if (nativePath) {
            SftpPut(sid, nativePath, remotePath, false)
          } else {
            readAndUpload(f, remotePath)
          }
        }
      }
    }
  }
}

async function readAndUpload(file: File, remotePath: string) {
  const sid = panel.value?.sessionId
  if (!sid) return
  const reader = new FileReader()
  reader.onload = async () => {
    const base64 = (reader.result as string).split(',')[1]
    try {
      const tmpPath = await WriteTempFile(file.name, base64)
      SftpPut(sid, tmpPath, remotePath, false)
    } catch (e) {
      msg.error('Failed to prepare file for upload')
    }
  }
  reader.readAsDataURL(file)
}

function onDragOver(e: DragEvent) {
  if (dragSource.value === null) {
    e.dataTransfer!.dropEffect = 'copy'
  } else {
    e.dataTransfer!.dropEffect = 'move'
  }
}

function onDragStart(e: DragEvent) {
  const target = e.target as HTMLElement | null
  dragDroppedInternally = false
  draggedRemoteItems = []
  if (target?.closest('.local-pane')) {
    dragSource.value = 'local'
  } else if (target?.closest('.remote-pane')) {
    dragSource.value = 'remote'
    // Capture the dragged file items for potential drag-out download
    try {
      const raw = e.dataTransfer?.getData('application/sftp-file')
      if (raw) {
        draggedRemoteItems = [JSON.parse(raw)]
      }
    } catch {}
  }
}

function onDragEnter(mode: 'local' | 'remote') {
  // Internal drag: skip overlay on source pane
  if (dragSource.value !== null && dragSource.value === mode) return
  // External drag (from desktop): only show overlay on remote pane
  if (dragSource.value === null && mode === 'local') return
  if (mode === 'local') {
    dragEnterLocalCount++
    dragOverLocal.value = true
  } else {
    dragEnterRemoteCount++
    dragOverRemote.value = true
  }
}

function onDragLeave(mode: 'local' | 'remote') {
  if (mode === 'local') {
    dragEnterLocalCount--
    if (dragEnterLocalCount <= 0) {
      dragEnterLocalCount = 0
      dragOverLocal.value = false
    }
  } else {
    dragEnterRemoteCount--
    if (dragEnterRemoteCount <= 0) {
      dragEnterRemoteCount = 0
      dragOverRemote.value = false
    }
  }
}

async function clearDragState() {
  dragOverLocal.value = false
  dragOverRemote.value = false
  dragEnterLocalCount = 0
  dragEnterRemoteCount = 0
  // Drag-out: remote file dragged outside app (e.g. to desktop)
  if (dragSource.value === 'remote' && !dragDroppedInternally && draggedRemoteItems.length > 0) {
    const sid = panel.value?.sessionId
    if (sid) {
      let desktopPath = ''
      try { desktopPath = await GetDesktopPath() } catch {}
      for (const item of draggedRemoteItems) {
        const remotePath = joinPath(cwd.value, item.name)
        const localPath = (desktopPath || localCwd.value) + '/' + item.name
        SftpGet(sid, remotePath, localPath, item.isDir)
      }
    }
  }
  dragSource.value = null
  draggedRemoteItems = []
}

function onDropLocal(e: DragEvent) {
  e.preventDefault()
  dragDroppedInternally = true
  clearDragState()
  const data = e.dataTransfer?.getData('application/sftp-file')
  if (!data) return
  try {
    const item = JSON.parse(data)
    if (item.mode === 'remote') {
      const remotePath = joinPath(cwd.value, item.name)
      const localPath = joinPath(localCwd.value, item.name).replace(/\\/g, '/')
      SftpGet(panel.value?.sessionId!, remotePath, localPath, item.isDir)
    }
  } catch (e) { console.error('onDropLocal:', e) }
}

async function onDropRemote(e: DragEvent) {
  e.preventDefault()
  dragDroppedInternally = true
  clearDragState()
  const sid = panel.value?.sessionId
  if (!sid) return

  // External files from desktop / file explorer
  const files = e.dataTransfer?.files
  if (files && files.length > 0) {
    const fileNames: string[] = []
    for (let i = 0; i < files.length; i++) {
      fileNames.push(files[i].name)
    }
    const action = await checkRemoteConflicts(fileNames)
    if (action === 'cancel') return

    const existingNames = remoteFiles.value.map(f => f.name)
    for (let i = 0; i < files.length; i++) {
      const f = files[i]
      let resolvedName = f.name
      if (action === 'rename' && existingNames.includes(f.name)) {
        resolvedName = autoRename(f.name, existingNames)
      }
      existingNames.push(resolvedName)
      const remotePath = cwd.value + '/' + resolvedName
      // Try native path first (WebView2 may expose it), fall back to reading content
      const nativePath = (f as any).path
      if (nativePath) {
        SftpPut(sid, nativePath, remotePath, false)
      } else {
        readAndUpload(f, remotePath)
      }
    }
    return
  }

  // Internal SFTP file drag
  const data = e.dataTransfer?.getData('application/sftp-file')
  if (!data) return
  try {
    const item = JSON.parse(data)
    if (item.mode === 'local') {
      const action = await checkRemoteConflicts([item.name])
      if (action === 'cancel') return

      let resolvedName = item.name
      if (action === 'rename') {
        const existingNames = remoteFiles.value.map(f => f.name)
        resolvedName = autoRename(item.name, existingNames)
      }
      const localPath = joinPath(localCwd.value, item.name)
      const remotePath = cwd.value + '/' + resolvedName
      SftpPut(panel.value?.sessionId!, localPath, remotePath, item.isDir)
    }
  } catch (e) { console.error('onDropRemote:', e) }
}
</script>

<style scoped>
.sftp-tab-content {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}
.panes-area {
  flex: 1;
  display: flex;
  overflow: hidden;
}
.local-pane, .remote-pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-right: 1px solid var(--border-subtle);
  position: relative;
}
.remote-pane {
  border-right: none;
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
  color: var(--on-accent);
  padding: 12px 24px;
  border: 2px dashed rgba(255, 255, 255, 0.6);
  border-radius: var(--radius-md);
}

.editor-container {
  display: flex;
  height: 55vh;
  border: 1px solid var(--el-border-color);
  border-radius: 4px;
  overflow: hidden;
}
.editor-container:focus-within {
  border-color: var(--el-color-primary);
}
.editor-line-numbers {
  flex-shrink: 0;
  min-width: 36px;
  padding: 12px 8px 12px 12px;
  font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', 'Consolas', 'Courier New', monospace;
  font-size: 14px;
  line-height: 24px;
  color: var(--text-disabled);
  background: var(--el-fill-color-light);
  text-align: right;
  overflow: hidden;
  user-select: none;
  white-space: pre;
}
.editor-textarea {
  flex: 1;
  font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', 'Consolas', 'Courier New', monospace;
  font-size: 14px;
  line-height: 24px;
  background: var(--el-fill-color-blank);
  color: var(--el-text-color-primary);
  border: none;
  padding: 12px;
  white-space: pre;
  overflow-x: auto;
  resize: none;
  outline: none;
  overflow-y: auto;
  tab-size: 4;
}

.conflict-list {
  max-height: 200px;
  overflow-y: auto;
  margin: 8px 0;
  padding-left: 20px;
}
.conflict-list li {
  font-size: 13px;
  padding: 2px 0;
  color: var(--text-secondary);
  font-family: monospace;
}

.editor-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}
.editor-footer-left {
  display: flex;
  align-items: center;
  gap: 8px;
}
.editor-footer-buttons {
  display: flex;
  gap: 8px;
}

.editor-textarea-wrap {
  white-space: pre-wrap;
  overflow-wrap: break-word;
  overflow-x: hidden;
}

.editor-mirror {
  position: fixed;
  top: -9999px;
  left: -9999px;
  visibility: hidden;
  white-space: pre-wrap;
  overflow-wrap: break-word;
  padding: 0;
}
</style>

<style>
.chmod-file-info {
  text-align: center;
  margin-bottom: 16px;
}
.chmod-filename {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-mono, monospace);
}
.chmod-ownergroup {
  display: block;
  font-size: 11px;
  color: var(--text-disabled);
  margin-top: 2px;
}
.chmod-form {
  margin-top: 4px;
}
.chmod-form .el-form-item {
  margin-bottom: 12px;
}
.chmod-octal-input {
  width: 120px;
}
.chmod-octal-input :deep(.el-input__inner) {
  font-family: var(--font-mono, monospace);
  font-weight: 700;
  letter-spacing: 2px;
}
</style>
