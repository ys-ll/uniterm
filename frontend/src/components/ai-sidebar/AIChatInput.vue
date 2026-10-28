<template>
  <div class="ai-input">
    <!-- Panel tags area -->
    <div class="ai-panel-tags">
      <div class="panel-tags-list">
        <template v-if="lockedPanels.length === 0 && currentIsTerminal">
          <span class="panel-tag panel-tag-default">{{ currentTerminalLabel }}</span>
        </template>
        <template v-else-if="lockedPanels.length > 0">
          <span
            v-for="pid in lockedPanels"
            :key="pid"
            class="panel-tag"
          >
            {{ getPanelDisplayName(pid) }}
            <button class="panel-tag-close" @click="onRemovePanelTag(pid)">&times;</button>
          </span>
        </template>
        <button class="panel-tag-add-btn" :title="t('ai.addTerminal')" @click.stop="addTagMenuRef?.toggle($event.currentTarget as HTMLElement)">+</button>
        <Menu ref="addTagMenuRef" v-model:visible="addTagMenuVisible">
          <MenuItem
            v-for="p in availableTerminalPanels"
            :key="p.id"
            :class="{ active: lockedPanels.includes(p.id) }"
            @click="onAddPanelTagSelect(p.id)"
          >
            <span>{{ getPanelDisplayName(p.id) }}</span>
            <span class="panel-shell-hint">{{ getPanelShellHint(p.id) }}</span>
          </MenuItem>
        </Menu>
      </div>
    </div>

    <div
      :id="aiDropId"
      class="input-container"
      data-file-drop-target
      @dragenter.prevent="onDropDragEnter"
      @dragover.prevent="onDropDragOver"
      @dragleave="onDropDragLeave"
      @drop="onDropDragDrop"
    >
      <div v-if="aiDropHover" class="ai-drop-overlay">
        <span>{{ t('ai.attachmentDropHere') }}</span>
      </div>
      <!-- # reference dropdown -->
      <div
        v-if="hashDropdownVisible && hashMatchingPanels.length > 0"
        class="hash-dropdown"
      >
        <div
          v-for="(p, i) in hashMatchingPanels"
          :key="p.id"
          class="hash-dropdown-item"
          :class="{ highlighted: i === hashHighlightIndex }"
          @mousedown.prevent="onSelectHashPanel(p.title)"
        >
          <span class="hash-panel-name">#{{ p.title }}</span>
          <span v-if="lockedPanels.includes(p.id)" class="hash-associated-badge">{{ t('ai.hashAssociated') }}</span>
          <span class="hash-panel-hint">{{ getPanelShellHint(p.id) }}</span>
        </div>
      </div>

      <!-- / skill & command dropdown -->
      <div
        v-if="skillDropdownVisible && skillMatchingItems.length > 0"
        class="skill-dropdown"
      >
        <div
          v-for="(item, i) in skillMatchingItems"
          :key="item.kind + '/' + item.name"
          class="skill-dropdown-item"
          :class="{ highlighted: i === skillHighlightIndex }"
          @mousedown.prevent="onSelectItem(item)"
        >
          <component :is="item.kind === 'command' ? Terminal : BookOpen" :size="'0.8125rem'" class="skill-dropdown-kind-icon" />
          <span class="skill-dropdown-name">/{{ item.name }}</span>
          <span v-if="item.kind === 'command' && item.argumentHint" class="skill-dropdown-args">{{ item.argumentHint }}</span>
          <span class="skill-dropdown-desc">{{ item.description }}</span>
        </div>
      </div>

      <div v-if="aiStore.queuedMessages.length" class="queued-area">
        <div v-for="q in aiStore.queuedMessages" :key="q.id" class="queued-chip">
          <span class="queued-text">{{ q.content }}</span>
          <button class="queued-remove" :title="t('ai.queueRemove')" @click="aiStore.removeQueuedMessage(q.id)">
            <X :size="'0.75rem'" />
          </button>
        </div>
      </div>
      <div v-if="attachments.length" class="attachments-row">
        <div v-for="att in attachments" :key="att.id" class="attachment-chip" :title="`${att.name} · ${formatBytes(att.size)}`">
          <img v-if="att.kind === 'image'" class="attachment-thumb" :src="attachmentPreviewUrl(att)" :alt="att.name" />
          <FileText v-else :size="16" class="attachment-icon" />
          <div class="attachment-meta">
            <span class="attachment-name">{{ att.name }}</span>
            <span class="attachment-size">{{ formatBytes(att.size) }}</span>
          </div>
          <button class="attachment-remove" :title="t('ai.attachRemove')" @click="removeAttachment(att.id)">
            <X :size="12" />
          </button>
        </div>
      </div>
      <div class="textarea-wrap">
        <div
          ref="editableRef"
          class="ai-editable"
          :contenteditable="lockedPanels.length > 0 || currentIsTerminal ? 'true' : 'false'"
          :data-placeholder="lockedPanels.length === 0 && !currentIsTerminal ? t('ai.noTerminalHint') : t('ai.placeholder')"
          @input="onEditableInput"
          @keydown="onKeydown"
          @paste="onPaste"
        />
      </div>
      <input
        ref="fileInputRef"
        class="attachment-file-input"
        type="file"
        multiple
        :accept="ATTACHMENT_FILE_ACCEPT"
        @change="onFileInputChange"
      />
      <div class="input-actions">
        <div class="input-actions-left">
          <button
            v-if="!isMobile"
            class="ghost-btn hash-btn attach-btn"
            :title="t('ai.attachFile')"
            :disabled="lockedPanels.length === 0 && !currentIsTerminal"
            @click="onAttachClick"
          >
            <Paperclip :size="14" />
          </button>
          <button class="ghost-btn hash-btn" :title="t('ai.referenceTerminal')" :disabled="lockedPanels.length === 0 && !currentIsTerminal" @click="onHashButtonClick">
            <span class="hash-btn-icon">#</span>
          </button>
          <button class="ghost-btn hash-btn" :title="t('ai.skillOrCommand')" :disabled="lockedPanels.length === 0 && !currentIsTerminal" @click="onSlashButtonClick">
            <span class="hash-btn-icon">/</span>
          </button>
          <template v-if="settingsStore.settings.ai.models.length > 0">
            <button class="ghost-btn model-btn" :title="currentModelName" @click.stop="modelMenuRef?.toggle($event.currentTarget as HTMLElement)">{{ currentModelName }}</button>
            <Menu ref="modelMenuRef" v-model:visible="modelMenuVisible">
              <MenuItem
                v-for="m in settingsStore.settings.ai.models"
                :key="m.id"
                :class="{ active: m.id === settingsStore.settings.ai.activeModelId }"
                @click="onModelSelect(m.id)"
              >
                <span>{{ m.name }}</span>
              </MenuItem>
              <MenuDivider />
              <MenuItem iconic :icon="Plus" @click="onModelSelect('__add_model__')">
                {{ t('settings.addModel') }}
              </MenuItem>
            </Menu>
          </template>
          <button v-else class="ghost-btn model-btn add-model-btn" @click="onModelChange('__add_model__')">
            <Plus :size="'0.875rem'" />
            <span>{{ t('settings.addModel') }}</span>
          </button>
        </div>
        <div class="input-actions-right">
          <button class="ghost-btn mode-btn" :title="modeLabel" @click.stop="modeMenuRef?.toggle($event.currentTarget as HTMLElement)">{{ modeLabel }}</button>
          <Menu ref="modeMenuRef" v-model:visible="modeMenuVisible">
            <MenuItem @click="onModeSelect('confirm_all')">
              <span class="mode-option mode-confirm">{{ t('ai.confirmAll') }}</span>
            </MenuItem>
            <MenuItem @click="onModeSelect('confirm_write')">
              <span class="mode-option mode-write">{{ t('ai.confirmWrite') }}</span>
            </MenuItem>
            <MenuItem @click="onModeSelect('confirm_dangerous')">
              <span class="mode-option mode-warning">{{ t('ai.confirmDangerous') }}</span>
            </MenuItem>
            <MenuItem @click="onModeSelect('bypass')">
              <span class="mode-option mode-auto">{{ t('ai.bypass') }}</span>
            </MenuItem>
          </Menu>
          <button
            v-if="!(busy && !inputText.trim())"
            class="send-btn"
            :disabled="(!inputText.trim() && !hasSkillTag && !hasCommandTag && !attachments.length) || (lockedPanels.length === 0 && !currentIsTerminal)"
            :title="busy ? t('ai.queue') : t('ai.send')"
            @click="onSend"
          >
            <ArrowUp :size="'1.125rem'" />
          </button>
          <button v-else class="send-btn stop" :title="t('ai.stop')" @click="onStop">
            <Square :size="'0.9375rem'" :fill="'currentColor'" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick, computed, watch, onMounted, onUnmounted } from 'vue'
import { X, ArrowUp, Square, Plus, BookOpen, Terminal, Paperclip, FileText } from '@lucide/vue'
import { ElMessage } from 'element-plus'
import { useAIStore } from '../../stores/aiStore'
import { useSettingsStore } from '../../stores/settingsStore'
import { useSkillStore } from '../../stores/skillStore'
import { useCommandStore } from '../../stores/commandStore'
import { useTabStore } from '../../stores/tabStore'
import { usePanelStore } from '../../stores/panelStore'
import { useI18n } from '../../i18n'
import { ReadLocalAttachmentFile } from '../../../bindings/github.com/ys-ll/uniterm/app'
import type { AIAttachment } from '../../types/ai'
import type { ConnectionConfig } from '../../types/session'
import {
  ATTACHMENT_FILE_ACCEPT,
  ATTACHMENT_LIMITS,
  attachmentErrorMessageKey,
  attachmentFormatsLabel,
  attachmentPreviewUrl,
  bytesFromBase64,
  classifyAttachment,
  createImageAttachmentFromDataURL,
  formatBytes,
  readAttachmentFile,
  validateAttachment,
} from '../../services/attachments'
import { useNativeFileDrop } from '../../composables/useFilePanel'
import { isMobilePlatform } from '../../utils/platform'
import Menu from '../Menu.vue'
import MenuItem from '../MenuItem.vue'
import MenuDivider from '../MenuDivider.vue'
import { Clipboard } from '@wailsio/runtime'
import { useAIChat } from '../../composables/useAIChat'

const emit = defineEmits<{
  'open-settings': []
  'sent': []
}>()

const aiStore = useAIStore()
const settingsStore = useSettingsStore()
const tabStore = useTabStore()
const panelStore = usePanelStore()
const skillStore = useSkillStore()
const commandStore = useCommandStore()
const { t } = useI18n()
const { busy, modeLabel, currentModelName, setMode, send, stop } = useAIChat()

const editableRef = ref<HTMLDivElement | null>(null)

// Derive plain text from contenteditable div (hash-tag spans contribute #PanelName)
function getEditableText(): string {
  const el = editableRef.value
  if (!el) return ''
  let text = ''
  const walk = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      text += node.textContent || ''
    } else if (node instanceof HTMLElement) {
      if (node.classList.contains('skill-tag')) {
        // skill tag 不贡献文本(skill 正文单独注入,不发给终端)
        return
      }
      if (node.classList.contains('command-tag')) {
        return // command tag 不贡献文本（正文后台组装）
      }
      if (node.classList.contains('hash-tag')) {
        text += node.getAttribute('data-ref') || node.textContent || ''
      } else {
        node.childNodes.forEach(walk)
      }
    }
  }
  el.childNodes.forEach(walk)
  return text
}

// 从输入框里提取已插入的 skill 名(取第一个 skill-tag)
function extractSkillFromInput(): string | null {
  const el = editableRef.value
  if (!el) return null
  const tag = el.querySelector('.skill-tag')
  return tag?.getAttribute('data-skill') || null
}

// Create a skill inline tag span (/name), styled like a hash-tag, carries data-skill
function createSkillTagSpan(name: string): HTMLSpanElement {
  const span = document.createElement('span')
  span.className = 'hash-tag skill-tag'
  span.setAttribute('data-skill', name)
  span.contentEditable = 'false'
  span.textContent = '/' + name
  const el = editableRef.value
  if (el) {
    for (const attr of el.attributes) {
      if (attr.name.startsWith('data-v-')) {
        span.setAttribute(attr.name, '')
        break
      }
    }
  }
  return span
}

function createCommandTagSpan(name: string): HTMLSpanElement {
  const span = document.createElement('span')
  span.className = 'hash-tag command-tag'
  span.setAttribute('data-command', name)
  span.contentEditable = 'false'
  span.textContent = '/' + name
  const el = editableRef.value
  if (el) {
    for (const attr of el.attributes) {
      if (attr.name.startsWith('data-v-')) {
        span.setAttribute(attr.name, '')
        break
      }
    }
  }
  return span
}

function extractCommandFromInput(): { name: string; args: string } | null {
  const el = editableRef.value
  if (!el) return null
  const tag = el.querySelector('.command-tag')
  const name = tag?.getAttribute('data-command')
  if (!name) return null
  // 参数 = tag 之后的纯文本
  let args = ''
  let after = false
  const walk = (node: Node) => {
    if (node === tag) {
      after = true
      return
    }
    if (!after) return
    if (node.nodeType === Node.TEXT_NODE) {
      args += node.textContent || ''
      return
    }
    // skill/command/hash tag 的文本（/name、#panel）不是参数，跳过
    if (node instanceof HTMLElement && (node.classList.contains('skill-tag') || node.classList.contains('command-tag') || node.classList.contains('hash-tag'))) return
    node.childNodes.forEach(walk)
  }
  el.childNodes.forEach(walk)
  return { name, args: args.trim() }
}

// Create a hash-tag span with scoped CSS attribute so styles apply
function createHashTagSpan(panelTitle: string): HTMLSpanElement {
  const span = document.createElement('span')
  span.className = 'hash-tag'
  span.setAttribute('data-ref', '#' + panelTitle)
  span.contentEditable = 'false'
  span.textContent = '#' + panelTitle
  // Copy scoped style attribute from editable div so Vue scoped CSS matches
  const el = editableRef.value
  if (el) {
    for (const attr of el.attributes) {
      if (attr.name.startsWith('data-v-') || attr.name.startsWith('data-v')) {
        span.setAttribute(attr.name, '')
        break
      }
    }
  }
  return span
}

// Computed: input text (for watch)
const inputText = ref('')
const hasSkillTag = ref(false)
const hasCommandTag = ref(false)

// --- Attachments -----------------------------------------------------------
const isMobile = isMobilePlatform()
const attachments = ref<AIAttachment[]>([])
const fileInputRef = ref<HTMLInputElement | null>(null)

function showAttachmentError(key: string, params?: Record<string, string | number>) {
  ElMessage.error(t(key, params))
}

function removeAttachment(id: string) {
  attachments.value = attachments.value.filter((a) => a.id !== id)
}

function clearAttachments() {
  attachments.value = []
}

function onAttachClick() {
  fileInputRef.value?.click()
}

/** Add one file, reporting the reason on rejection instead of dropping it. */
async function addFile(file: File) {
  const verdict = validateAttachment({ name: file.name, size: file.size, type: file.type }, attachments.value)
  if (!verdict.ok) {
    showAttachmentError(attachmentErrorMessageKey(verdict.error), verdict.error.params)
    return
  }
  try {
    attachments.value = [...attachments.value, await readAttachmentFile(file, verdict.kind, verdict.mime)]
  } catch (e) {
    const key = e instanceof Error && e.message === 'binary file' ? 'ai.attachmentBinary'
      : e instanceof Error && e.message === 'image pixels too large' ? 'ai.attachmentTooLargePixels'
      : 'ai.attachmentReadFailed'
    showAttachmentError(key, { name: file.name, limit: ATTACHMENT_LIMITS.maxImageSide })
  }
}

async function onFileInputChange(e: Event) {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files || [])
  // Reset immediately so picking the same file twice still fires 'change'.
  input.value = ''
  for (const file of files) await addFile(file)
}

// --- Drag-drop attachments (native OS paths) --------------------------------
const aiDropId = 'ai-drop-' + (crypto.randomUUID?.() || Math.random().toString(36).slice(2))
const aiDropHover = ref(false)
let aiDropEnterCount = 0
const dropAllowed = computed(() => !isMobile && (lockedPanels.value.length > 0 || currentIsTerminal.value))

const nativeDrop = useNativeFileDrop({
  elementId: aiDropId,
  isActive: () => dropAllowed.value,
  upload: (paths) => onNativePathsDropped(paths),
})

function onDropDragEnter(e: DragEvent) {
  if (!dropAllowed.value || !e.dataTransfer?.types.includes('Files')) return
  aiDropEnterCount++
  aiDropHover.value = true
}

function onDropDragOver(e: DragEvent) {
  if (!dropAllowed.value || !e.dataTransfer?.types.includes('Files')) return
  e.dataTransfer.dropEffect = 'copy'
}

function onDropDragLeave() {
  if (--aiDropEnterCount <= 0) {
    aiDropEnterCount = 0
    aiDropHover.value = false
  }
}

function onDropDragDrop() {
  // The native layer consumes the drop; here we only retire the overlay.
  aiDropEnterCount = 0
  aiDropHover.value = false
}

/** Add files dropped as absolute OS paths. */
async function onNativePathsDropped(paths: string[]) {
  for (const p of paths) {
    const name = p.replace(/\\/g, '/').split('/').pop() || p
    if (!classifyAttachment(name, '')) {
      showAttachmentError('ai.attachmentUnsupportedType', { name, formats: attachmentFormatsLabel })
      continue
    }
    try {
      const f = await ReadLocalAttachmentFile(p)
      await addFile(new File([bytesFromBase64(f.dataB64)], f.name || name))
    } catch (e) {
      const detail = e instanceof Error ? e.toString() : String(e)
      if (detail.includes('AI_ATTACHMENT_IS_DIRECTORY')) {
        showAttachmentError('ai.attachmentIsDirectory', { name })
      } else if (detail.includes('AI_ATTACHMENT_TOO_LARGE')) {
        showAttachmentError('ai.attachmentTooLarge', { name, limit: formatBytes(ATTACHMENT_LIMITS.maxImageBytes) })
      } else if (detail.includes('AI_ATTACHMENT_EMPTY')) {
        showAttachmentError('ai.attachmentEmpty', { name })
      } else {
        showAttachmentError('ai.attachmentReadFailed', { name })
      }
    }
  }
}

/** Take an image off the clipboard. Returns true when an image was consumed. */
function attachClipboardImage(blob: Blob | null | undefined, fallbackMime = ''): boolean {
  if (!blob || !blob.size) return false
  const name = t('ai.attachPastedImage')
  const verdict = validateAttachment({ name, size: blob.size, type: blob.type || fallbackMime }, attachments.value)
  if (!verdict.ok) {
    showAttachmentError(attachmentErrorMessageKey(verdict.error), verdict.error.params)
    return true
  }
  const reader = new FileReader()
  reader.onerror = () => showAttachmentError('ai.attachmentReadFailed', { name })
  reader.onload = () => {
    createImageAttachmentFromDataURL(typeof reader.result === 'string' ? reader.result : '', name, blob.size, verdict.mime)
      .then((att) => {
        attachments.value = [...attachments.value, att]
      })
      .catch((e: unknown) => {
        const key = e instanceof Error && e.message === 'image pixels too large' ? 'ai.attachmentTooLargePixels' : 'ai.attachmentReadFailed'
        showAttachmentError(key, { name, limit: ATTACHMENT_LIMITS.maxImageSide })
      })
  }
  reader.readAsDataURL(blob)
  return true
}

/** Pull an image out of a DOM paste event's clipboard data, if any. */
function clipboardImageFromEvent(e: ClipboardEvent): Blob | null {
  const items = e.clipboardData?.items
  if (items) {
    for (const item of Array.from(items)) {
      if (item.kind === 'file' && item.type.startsWith('image/')) {
        const file = item.getAsFile()
        if (file) return file
      }
    }
  }
  const files = e.clipboardData?.files
  if (files) {
    for (const file of Array.from(files)) {
      if (file.type.startsWith('image/')) return file
    }
  }
  return null
}

async function readClipboardImage(): Promise<Blob | null> {
  const clipboard = navigator.clipboard
  if (!clipboard || typeof clipboard.read !== 'function') return null
  const items = await clipboard.read()
  for (const item of items) {
    const type = item.types.find((tp) => tp.startsWith('image/'))
    if (type) return await item.getType(type)
  }
  return null
}

function pasteFromClipboard() {
  let settled = false
  const pasteText = () => {
    if (settled) return
    settled = true
    Clipboard.Text().then((text) => {
      if (text) insertTextAtCursor(text)
    }).catch(() => {})
  }
  const timer = window.setTimeout(pasteText, 500)
  void readClipboardImage()
    .then((blob) => {
      if (settled) return
      window.clearTimeout(timer)
      settled = true
      if (!attachClipboardImage(blob)) pasteText()
    })
    .catch(() => {
      window.clearTimeout(timer)
      pasteText()
    })
}

function syncInputText() {
  inputText.value = getEditableText()
  hasSkillTag.value = extractSkillFromInput() !== null
  hasCommandTag.value = extractCommandFromInput() !== null
}

function onEditableInput() {
  syncInputText()
  refreshHashDropdown()
  refreshSkillDropdown()
}

// MutationObserver as backup — catches changes that don't fire 'input' event
let editableObserver: MutationObserver | null = null
onMounted(() => {
  if (editableRef.value) {
    editableObserver = new MutationObserver(() => {
      syncInputText()
      refreshHashDropdown()
      refreshSkillDropdown()
    })
    editableObserver.observe(editableRef.value, { childList: true, subtree: true, characterData: true })
  }
})

function focus() {
  nextTick(() => {
    editableRef.value?.focus()
  })
}

// --- Panel tags ---
const lockedPanels = computed(() => [...tabStore.aiLockedPanelIds])

const currentIsTerminal = computed(() => {
  const tab = tabStore.activeTab
  if (tab?.type === 'terminal') return true
  if (tab?.type === 'workspace') {
    const pid = tab.activePanelId
    return !!(pid && panelStore.getPanel(pid))
  }
  return false
})

const currentTerminalLabel = computed(() => {
  const tab = tabStore.activeTab
  if (!tab) return t('ai.currentTerminal')
  let panelId: string | undefined
  if (tab.type === 'terminal') {
    panelId = tab.panelId
  } else if (tab.type === 'workspace') {
    panelId = tab.activePanelId ?? undefined
  }
  if (!panelId) return t('ai.currentTerminal')
  const panel = panelStore.getPanel(panelId)
  return panel ? `${t('ai.currentTerminal')}: ${panel.title}` : t('ai.currentTerminal')
})

const availableTerminalPanels = computed(() => {
  const result: Array<{ id: string; title: string; type: string; shellPath?: string; config?: ConnectionConfig | null }> = []
  const seen = new Set<string>()
  for (const tab of tabStore.tabs) {
    if (tab.type === 'terminal' && tab.panelId) {
      const p = panelStore.getPanel(tab.panelId)
      if (p && (p.type === 'ssh' || p.type === 'local' || p.type === 'wsl') && !seen.has(p.id)) {
        seen.add(p.id)
        result.push({ id: p.id, title: p.title, type: p.type, shellPath: p.config?.shellPath, config: p.config })
      }
    }
    if (tab.type === 'workspace' && tab.panelIds) {
      for (const pid of tab.panelIds) {
        const p = panelStore.getPanel(pid)
        if (p && (p.type === 'ssh' || p.type === 'local' || p.type === 'wsl') && !seen.has(p.id)) {
          seen.add(p.id)
          result.push({ id: p.id, title: p.title, type: p.type, shellPath: p.config?.shellPath, config: p.config })
        }
      }
    }
  }
  return result
})

function getPanelDisplayName(panelId: string): string {
  const p = panelStore.getPanel(panelId)
  if (!p) return panelId
  const dup = availableTerminalPanels.value.filter((ap) => ap.title === p.title)
  return dup.length > 1 ? `${p.title} (id: ${p.id})` : p.title
}

function getPanelShellHint(panelId: string): string {
  const p = panelStore.getPanel(panelId)
  if (!p) return ''
  const shellPath = p.config?.shellPath
  if (shellPath) {
    const lower = shellPath.toLowerCase()
    if (lower.includes('bash') || lower.includes('sh')) return 'Bash'
    if (lower.includes('powershell') || lower.includes('pwsh')) return 'PowerShell'
    if (lower.includes('cmd')) return 'CMD'
    if (lower.includes('zsh')) return 'Zsh'
    return shellPath.split(/[\\/]/).pop() || 'Shell'
  }
  if (p.type === 'ssh') return 'SSH'
  return ''
}

function onRemovePanelTag(panelId: string) {
  tabStore.removeAILockedPanel(panelId)
}

function onAddPanelTag(panelId: string) {
  if (tabStore.isPanelAILocked(panelId)) {
    tabStore.removeAILockedPanel(panelId)
  } else {
    tabStore.addAILockedPanel(panelId)
  }
}

// --- # reference state ---
const hashQuery = ref('')
const hashDropdownVisible = ref(false)
const hashHighlightIndex = ref(0)

// --- / skill & command state ---
const skillQuery = ref('')
const skillDropdownVisible = ref(false)
const skillHighlightIndex = ref(0)

const hashMatchingPanels = computed(() => {
  const src = hashDropdownVisible.value && !hashQuery.value
    ? availableTerminalPanels.value
    : availableTerminalPanels.value
  let list = hashQuery.value
    ? src.filter((p) => p.title.toLowerCase().includes(hashQuery.value.toLowerCase()))
    : [...src]
  // Sort: associated panels first
  list = [...list].sort((a, b) => {
    const aLocked = lockedPanels.value.includes(a.id) ? 0 : 1
    const bLocked = lockedPanels.value.includes(b.id) ? 0 : 1
    return aLocked - bLocked
  })
  return list
})

const skillNameToken = computed(() => skillQuery.value.trimStart().split(/\s+/)[0] || '')

type SlashItem = { kind: 'skill' | 'command'; name: string; description: string; argumentHint?: string }

const skillMatchingItems = computed<SlashItem[]>(() => {
  const q = skillNameToken.value.toLowerCase()
  const commands: SlashItem[] = commandStore.enabledCommands.map((c) => ({ kind: 'command', name: c.name, description: c.description, argumentHint: c.argumentHint }))
  const skills: SlashItem[] = skillStore.enabledSkills.map((s) => ({ kind: 'skill', name: s.name, description: s.description }))
  let list = [...commands, ...skills]
  if (q) list = list.filter((i) => i.name.toLowerCase().includes(q) || i.description.toLowerCase().includes(q))
  return list.slice(0, 8)
})

function findLastSkillSlash(text: string): number {
  for (let i = text.length - 1; i >= 0; i--) {
    if (text[i] === '/') {
      if (i === 0 || /[\s,;:.(\{\[]/.test(text[i - 1])) {
        return i
      }
    }
  }
  return -1
}

function detectSlashQuery(): string | null {
  const sel = window.getSelection()
  const el = editableRef.value
  if (!sel || !sel.rangeCount || !el) return null
  const node = sel.anchorNode
  if (!node || !el.contains(node)) return null
  if (node.nodeType === Node.TEXT_NODE) {
    const text = node.textContent?.slice(0, sel.anchorOffset) || ''
    const idx = findLastSkillSlash(text)
    if (idx >= 0) {
      return text.slice(idx + 1)
    }
  }
  return null
}

function refreshSkillDropdown() {
  const query = detectSlashQuery()
  if (query !== null) {
    skillDropdownVisible.value = true
    skillQuery.value = query
    skillHighlightIndex.value = 0
  } else {
    skillDropdownVisible.value = false
    skillQuery.value = ''
  }
}

function onSelectSkill(name: string) {
  const el = editableRef.value
  // 删除输入框里正在输入的 /query 片段（无论从补全还是按钮触发），只留 chip
  if (el) {
    const sel = window.getSelection()
    let removed = false
    if (sel && sel.rangeCount > 0 && el.contains(sel.anchorNode)) {
      const node = sel.anchorNode
      if (node && node.nodeType === Node.TEXT_NODE) {
        const tn = node as Text
        const caretPos = sel.anchorOffset
        const c = tn.textContent || ''
        const hi = c.slice(0, caretPos).lastIndexOf('/')
        if (hi >= 0) {
          const delRange = document.createRange()
          delRange.setStart(tn, hi)
          delRange.setEnd(tn, caretPos)
          delRange.deleteContents()
          sel.removeAllRanges()
          sel.addRange(delRange)
          removed = true
        }
      }
    }
    // 兜底：若光标不在输入框（如按钮触发），按文本删掉末尾的 /query
    if (!removed) {
      const text = getEditableText()
      const idx = findLastSkillSlash(text)
      if (idx >= 0) {
        el.textContent = text.slice(0, idx) + text.slice(idx).replace(/^\/\S*/, '')
      }
    }
  }
  // 移除已有的 skill tag（只允许一个），再在光标处插入新的
  if (el) {
    el.querySelectorAll('.skill-tag').forEach((n) => n.remove())
    const tagSpan = createSkillTagSpan(name)
    const sel = window.getSelection()
    if (sel && sel.rangeCount > 0 && el.contains(sel.anchorNode)) {
      const range = sel.getRangeAt(0)
      range.collapse(false)
      range.insertNode(tagSpan)
      const trailing = document.createTextNode(' ')
      tagSpan.after(trailing)
      range.setStart(trailing, 1)
      range.collapse(true)
      sel.removeAllRanges()
      sel.addRange(range)
    } else {
      // 光标不在输入框（按钮触发）：插到开头
      el.insertBefore(tagSpan, el.firstChild)
      el.insertBefore(document.createTextNode(' '), tagSpan.nextSibling)
    }
  }
  skillDropdownVisible.value = false
  skillQuery.value = ''
  syncInputText()
}

function onSelectCommand(name: string) {
  const el = editableRef.value
  // 删除输入框里正在输入的 /query 片段（无论从补全还是按钮触发），只留 chip
  if (el) {
    const sel = window.getSelection()
    let removed = false
    if (sel && sel.rangeCount > 0 && el.contains(sel.anchorNode)) {
      const node = sel.anchorNode
      if (node && node.nodeType === Node.TEXT_NODE) {
        const tn = node as Text
        const caretPos = sel.anchorOffset
        const c = tn.textContent || ''
        const hi = c.slice(0, caretPos).lastIndexOf('/')
        if (hi >= 0) {
          const delRange = document.createRange()
          delRange.setStart(tn, hi)
          delRange.setEnd(tn, caretPos)
          delRange.deleteContents()
          sel.removeAllRanges()
          sel.addRange(delRange)
          removed = true
        }
      }
    }
    // 兜底：若光标不在输入框（如按钮触发），按文本删掉末尾的 /query
    if (!removed) {
      const text = getEditableText()
      const idx = findLastSkillSlash(text)
      if (idx >= 0) {
        el.textContent = text.slice(0, idx) + text.slice(idx).replace(/^\/\S*/, '')
      }
    }
  }
  // 移除已有的 command tag（只允许一个），再在光标处插入新的
  if (el) {
    el.querySelectorAll('.command-tag').forEach((n) => n.remove())
    const tagSpan = createCommandTagSpan(name)
    const sel = window.getSelection()
    if (sel && sel.rangeCount > 0 && el.contains(sel.anchorNode)) {
      const range = sel.getRangeAt(0)
      range.collapse(false)
      range.insertNode(tagSpan)
      const trailing = document.createTextNode(' ')
      tagSpan.after(trailing)
      range.setStart(trailing, 1)
      range.collapse(true)
      sel.removeAllRanges()
      sel.addRange(range)
    } else {
      // 光标不在输入框（按钮触发）：插到开头
      el.insertBefore(tagSpan, el.firstChild)
      el.insertBefore(document.createTextNode(' '), tagSpan.nextSibling)
    }
  }
  skillDropdownVisible.value = false
  skillQuery.value = ''
  syncInputText()
}

// `/` 下拉选中：skill/command 都走插 tag（正文后台组装，不在输入框展开）。
function onSelectItem(item: SlashItem) {
  if (item.kind === 'skill') {
    onSelectSkill(item.name)
    return
  }
  onSelectCommand(item.name)
}

function detectHashQuery(): string | null {
  const sel = window.getSelection()
  const el = editableRef.value
  if (!sel || !sel.rangeCount || !el) return null
  const node = sel.anchorNode
  if (!node || node.nodeType !== Node.TEXT_NODE || !el.contains(node)) return null
  const caret = sel.anchorOffset
  const before = (node.textContent || '').slice(0, caret)
  const hashIdx = before.lastIndexOf('#')
  if (hashIdx < 0) return null
  const query = before.slice(hashIdx + 1)
  if (/\s/.test(query)) return null
  if (hashIdx > 0 && !/[\s,;:.(\{\[]/.test(before[hashIdx - 1])) return null
  return query
}

function refreshHashDropdown() {
  const query = detectHashQuery()
  if (query !== null) {
    hashDropdownVisible.value = true
    hashQuery.value = query
    hashHighlightIndex.value = 0
  } else {
    hashDropdownVisible.value = false
    hashQuery.value = ''
  }
}

function onSelectHashPanel(panelTitle: string) {
  const el = editableRef.value
  if (!el) return

  const text = getEditableText()
  const lastHashIdx = -1

  if (lastHashIdx >= 0) {
    const before = text.slice(0, lastHashIdx)
    const after = text.slice(lastHashIdx + 1)
    const spaceIdx = after.indexOf(' ')
    const rest = spaceIdx >= 0 ? after.slice(spaceIdx) : ' '

    const tagSpan = createHashTagSpan(panelTitle)

    el.innerHTML = ''
    if (before) el.appendChild(document.createTextNode(before))
    el.appendChild(tagSpan)
    if (rest) el.appendChild(document.createTextNode(rest))
    nextTick(() => {
      const sel2 = window.getSelection()
      if (sel2) {
        const range = document.createRange()
        range.setStartAfter(tagSpan)
        range.collapse(true)
        sel2.removeAllRanges()
        sel2.addRange(range)
      }
    })
  } else {
    // Button-triggered: append tag span at cursor position
    const sel = window.getSelection()
    if (sel && sel.rangeCount > 0 && sel.anchorNode && sel.anchorNode.nodeType === Node.TEXT_NODE && el.contains(sel.anchorNode)) {
      const tn = sel.anchorNode as Text
      const caretPos = sel.anchorOffset
      const c = tn.textContent || ''
      const hi = c.slice(0, caretPos).lastIndexOf('#')
      if (hi >= 0) {
        const delRange = document.createRange()
        delRange.setStart(tn, hi)
        delRange.setEnd(tn, caretPos)
        delRange.deleteContents()
        sel.removeAllRanges()
        sel.addRange(delRange)
      }
    }
    if (sel && sel.rangeCount > 0 && el.contains(sel.anchorNode)) {
      const range = sel.getRangeAt(0)
      range.collapse(false)

      const tagSpan = createHashTagSpan(panelTitle)

      range.insertNode(tagSpan)
      const trailingBtn = document.createTextNode(' ')
      tagSpan.after(trailingBtn)
      range.setStart(trailingBtn, 0)
      range.collapse(true)
      sel.removeAllRanges()
      sel.addRange(range)
    } else {
      const tagSpan = createHashTagSpan(panelTitle)
      el.appendChild(tagSpan)
      el.appendChild(document.createTextNode(' '))
    }
  }

  syncInputText()
  refreshHashDropdown()
  hashDropdownVisible.value = false
  hashQuery.value = ''
  hashHighlightIndex.value = 0

  // Auto-add to locked panels
  const panel = availableTerminalPanels.value.find((p) => p.title === panelTitle)
  if (panel && !tabStore.isPanelAILocked(panel.id)) {
    tabStore.addAILockedPanel(panel.id)
  }
}

function onHashButtonClick() {
  const el = editableRef.value
  if (!el) return
  el.focus()

  const sel = window.getSelection()
  if (sel && sel.rangeCount > 0 && el.contains(sel.anchorNode)) {
    const range = sel.getRangeAt(0)
    range.deleteContents()
    const textNode = document.createTextNode('#')
    range.insertNode(textNode)
    range.setStart(textNode, 1)
    range.collapse(true)
    sel.removeAllRanges()
    sel.addRange(range)
  } else {
    const textNode = document.createTextNode('#')
    el.appendChild(textNode)
    const range = document.createRange()
    range.setStart(textNode, 1)
    range.collapse(true)
    sel?.removeAllRanges()
    sel?.addRange(range)
  }
  syncInputText()
  refreshHashDropdown()
}

function onSlashButtonClick() {
  const el = editableRef.value
  if (!el) return
  el.focus()

  const sel = window.getSelection()
  if (sel && sel.rangeCount > 0 && el.contains(sel.anchorNode)) {
    const range = sel.getRangeAt(0)
    range.deleteContents()
    const textNode = document.createTextNode('/')
    range.insertNode(textNode)
    range.setStart(textNode, 1)
    range.collapse(true)
    sel.removeAllRanges()
    sel.addRange(range)
  } else {
    const textNode = document.createTextNode('/')
    el.appendChild(textNode)
    const range = document.createRange()
    range.setStart(textNode, 1)
    range.collapse(true)
    sel?.removeAllRanges()
    sel?.addRange(range)
  }
  syncInputText()
  refreshSkillDropdown()
}

function onEscHashDropdown() {
  hashDropdownVisible.value = false
  hashQuery.value = ''
}

// --- Trigger menus ---
const addTagMenuVisible = ref(false)
const modelMenuVisible = ref(false)
const modeMenuVisible = ref(false)

const addTagMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const modelMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const modeMenuRef = ref<InstanceType<typeof Menu> | null>(null)

function closeMenus() {
  addTagMenuVisible.value = false
  modelMenuVisible.value = false
  modeMenuVisible.value = false
}

function onAddPanelTagSelect(id: string) {
  closeMenus()
  onAddPanelTag(id)
}

function onModelSelect(id: string) {
  closeMenus()
  onModelChange(id)
}

function onModeSelect(mode: string) {
  closeMenus()
  setMode(mode)
}

function onModelChange(modelId: string) {
  if (modelId === '__add_model__') {
    emit('open-settings')
    nextTick(() => {
      settingsStore.openCategory = 'ai'
    })
    return
  }
  settingsStore.setActiveModel(modelId)
}

function onKeydown(e: KeyboardEvent) {
  // Hash dropdown navigation
  if (hashDropdownVisible.value) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onEscHashDropdown()
      return
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      hashHighlightIndex.value = Math.min(hashHighlightIndex.value + 1, hashMatchingPanels.value.length - 1)
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      hashHighlightIndex.value = Math.max(hashHighlightIndex.value - 1, 0)
      return
    }
    if (e.key === 'Enter') {
      e.preventDefault()
      if (hashMatchingPanels.value.length > 0) {
        onSelectHashPanel(hashMatchingPanels.value[hashHighlightIndex.value].title)
      }
      return
    }
  }

  // Skill dropdown navigation
  if (skillDropdownVisible.value) {
    if (e.key === 'Escape') {
      e.preventDefault()
      skillDropdownVisible.value = false
      return
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      skillHighlightIndex.value = Math.min(skillHighlightIndex.value + 1, skillMatchingItems.value.length - 1)
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      skillHighlightIndex.value = Math.max(skillHighlightIndex.value - 1, 0)
      return
    }
    if (e.key === 'Enter') {
      e.preventDefault()
      if (skillMatchingItems.value.length > 0) {
        onSelectItem(skillMatchingItems.value[skillHighlightIndex.value])
      }
      return
    }
  }

  // Cmd/Ctrl+V is deliberately NOT preventDefault-ed: letting the browser run
  // its default action is the only way to receive a paste event carrying image
  // data, and onPaste() handles both the image and the text case.
  if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && (e.key === 'v' || e.key === 'V')) {
    if (isMobile) {
      e.preventDefault()
      Clipboard.Text().then((text) => {
        if (text) insertTextAtCursor(text)
      }).catch(() => {})
    }
    return
  }

  // Normal Enter to send
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    onSend()
  }
}

function onPaste(e: ClipboardEvent) {
  if (!isMobile) {
    const image = clipboardImageFromEvent(e)
    if (image) {
      e.preventDefault()
      attachClipboardImage(image)
      return
    }
  }

  const text = e.clipboardData?.getData('text/plain')
  if (text) {
    e.preventDefault()
    insertTextAtCursor(text)
    return
  }

  e.preventDefault()
  if (isMobile) {
    Clipboard.Text().then((t) => {
      if (t) insertTextAtCursor(t)
    }).catch(() => {})
    return
  }
  pasteFromClipboard()
}

function insertTextAtCursor(text: string) {
  const el = editableRef.value
  if (!el) return
  el.focus()
  const sel = window.getSelection()
  if (sel && sel.rangeCount > 0) {
    const range = sel.getRangeAt(0)
    range.deleteContents()
    range.insertNode(document.createTextNode(text))
    range.collapse(false)
    sel.removeAllRanges()
    sel.addRange(range)
  } else {
    el.textContent += text
  }
  syncInputText()
  refreshHashDropdown()
}

function clearInput() {
  const el = editableRef.value
  if (el) el.innerHTML = ''
  syncInputText()
  refreshHashDropdown()
}

/** Set the input text directly (used by ask-AI / external ai:ask events). */
function setText(text: string) {
  const el = editableRef.value
  if (el) el.textContent = text
  syncInputText()
  refreshHashDropdown()
}

async function onSend() {
  const text = getEditableText().trim()
  const pending = attachments.value
  const cmd = extractCommandFromInput()
  const skillName = extractSkillFromInput()
  if (!text && !skillName && !cmd && pending.length === 0) return
  const ok = await send(
    { text, skillName, cmd, attachments: pending },
    () => {
      clearInput()
      clearAttachments()
    },
  )
  if (ok) emit('sent')
}

function onStop() {
  stop()
}

watch(
  () => aiStore.visible,
  (visible) => {
    if (visible) {
      nextTick(() => editableRef.value?.focus())
    }
  },
)

onMounted(() => {
  skillStore.load()
  commandStore.load()
  nativeDrop.bind()
})

onUnmounted(() => {
  nativeDrop.unbind()
  editableObserver?.disconnect()
})

defineExpose({ setText, focus })
</script>

<style scoped>
.ai-input {
  padding: 0.625rem 1rem;
  flex-shrink: 0;
  position: relative;
}
.input-container {
  border: 1px solid var(--border-subtle);
  border-radius: 0 0 var(--radius-sm) var(--radius-sm);
  background: var(--bg-elevated);
  transition: border-color 0.15s ease;
  position: relative;
}
.input-container:focus-within {
  border-color: var(--accent);
  border-top-color: var(--accent);
}
.ai-drop-overlay {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--scrim);
  border-radius: inherit;
  pointer-events: none;
}
.ai-drop-overlay span {
  font-size: 0.875rem;
  color: var(--text-primary);
  padding: 0.75rem 1.5rem;
  border: 0.125rem dashed var(--border-hover);
  border-radius: var(--radius-sm);
}
.textarea-wrap {
  position: relative;
}
.attachments-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.375rem;
  padding: 0.5rem 0.75rem 0;
}
.attachment-chip {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  max-width: 100%;
  padding: 0.25rem 0.375rem;
  background: var(--bg-tertiary, rgba(127, 127, 127, 0.12));
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
}
.attachment-thumb {
  width: 1.75rem;
  height: 1.75rem;
  object-fit: cover;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}
.attachment-icon {
  flex-shrink: 0;
  color: var(--text-muted);
}
.attachment-meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
  line-height: 1.3;
}
.attachment-name {
  font-size: 0.6875rem;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 11rem;
}
.attachment-size {
  font-size: 0.625rem;
  color: var(--text-muted);
}
.attachment-remove {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0.125rem;
  border: none;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}
.attachment-remove:hover {
  color: var(--text-primary);
  background: var(--bg-hover, rgba(127, 127, 127, 0.18));
}
.attachment-file-input {
  display: none;
}
.ai-editable {
  padding: 0.75rem 1rem;
  font-size: 0.75rem;
  font-family: var(--font-ui);
  color: var(--text-primary);
  background: transparent;
  border: none;
  outline: none;
  min-height: 3.75rem;
  max-height: 13.75rem;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
  user-select: text;
  -webkit-user-select: text;
  cursor: text;
}
.ai-editable:empty::before {
  content: attr(data-placeholder);
  color: var(--text-muted);
  pointer-events: none;
}
.hash-tag {
  display: inline;
  background: var(--accent);
  color: var(--on-accent);
  border-radius: var(--radius-sm);
  padding: 1px 0.3125rem;
  font-size: 0.75rem;
  font-weight: 500;
  white-space: nowrap;
  user-select: none;
  margin: 0 0.125rem;
}
.input-actions {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  align-items: center;
  padding: 0 0.5rem 0.5rem 0.5rem;
}
.input-actions-left {
  display: flex;
  gap: 0.125rem;
  align-items: center;
}
.input-actions-right {
  display: flex;
  gap: 0.375rem;
  align-items: center;
}
.ghost-btn {
  display: inline-block;
  box-sizing: border-box;
  height: 1.5rem;
  line-height: 1.5rem;
  padding: 0 0.375rem;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  font-family: var(--font-ui);
  font-size: 0.6875rem;
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  transition: background 0.12s ease, color 0.12s ease;
}
.ghost-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.ghost-btn:disabled {
  cursor: not-allowed;
  opacity: 0.4;
}
.model-btn {
  max-width: 6rem;
}
.add-model-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  max-width: none;
}
.mode-btn {
  max-width: 6.75rem;
}
.send-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.875rem;
  height: 1.875rem;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm);
  background: var(--accent);
  color: var(--on-accent);
  transition: background 0.12s ease, opacity 0.12s ease;
}
.send-btn:hover:not(:disabled) {
  background: var(--accent);
}
.send-btn:disabled {
  background: var(--bg-active);
  color: var(--text-disabled);
  cursor: not-allowed;
}
.send-btn.stop {
  background: var(--error);
  color: var(--on-accent);
}
.send-btn.stop:hover {
  background: var(--error);
  opacity: 0.85;
}
.mode-option {
  font-size: 0.75rem;
  font-weight: 500;
  font-family: var(--font-ui);
}
.mode-auto {
  color: var(--error);
}
.mode-confirm {
  color: var(--success);
}
.mode-write {
  color: var(--accent);
}
.mode-warning {
  color: var(--warning);
}
.ai-panel-tags {
  padding: 0.25rem 0.75rem;
  background: var(--bg-overlay);
  border-radius: var(--radius-sm) var(--radius-sm) 0 0;
}
.panel-tags-list {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  flex-wrap: wrap;
  min-height: 1.375rem;
}
.panel-tag {
  display: inline-flex;
  align-items: center;
  gap: 0.1875rem;
  padding: 1px 0.375rem;
  font-size: 0.6875rem;
  background: var(--accent-subtle);
  color: var(--accent);
  border: 1px solid var(--accent-glow);
  border-radius: var(--radius-sm);
  line-height: 1.5;
}
.panel-tag-default {
  background: var(--bg-overlay);
  color: var(--text-muted);
  border-color: var(--border-subtle);
}
.panel-tag-close {
  background: none;
  border: none;
  padding: 0;
  font-size: 0.8125rem;
  line-height: 1;
  color: var(--text-muted);
  transition: color 0.15s;
}
.panel-tag-close:hover {
  color: var(--text-primary);
}
.panel-tag-add-btn {
  background: none;
  border: 1px dashed var(--border-hover);
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  width: 1.25rem;
  height: 1.25rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 0.8125rem;
  line-height: 1;
  padding: 0;
  transition: border-color 0.15s, color 0.15s;
}
.panel-tag-add-btn:hover {
  border-color: var(--accent);
  color: var(--accent);
}
.panel-shell-hint {
  margin-left: 0.5rem;
  font-size: 0.625rem;
  color: var(--text-muted);
}
.hash-dropdown {
  position: absolute;
  bottom: 100%;
  left: -1px;
  right: -1px;
  max-height: 11.25rem;
  overflow-y: auto;
  background: var(--bg-surface);
  border: 1px solid var(--accent-glow);
  border-radius: var(--radius-sm);
  box-shadow: 0 -0.25rem 0.75rem rgba(0, 0, 0, 0.4);
  z-index: 100;
  margin-bottom: 0.25rem;
}
.hash-dropdown-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.375rem 0.625rem;
  font-size: 0.75rem;
  transition: background 0.1s;
}
.hash-dropdown-item:hover,
.hash-dropdown-item.highlighted {
  background: var(--accent-subtle);
}
.hash-panel-name {
  color: var(--accent);
  font-weight: 500;
}
.hash-panel-hint {
  font-size: 0.625rem;
  color: var(--text-muted);
  margin-left: auto;
}
.hash-associated-badge {
  font-size: 0.5625rem;
  color: var(--accent);
  background: var(--accent-subtle);
  padding: 0 0.25rem;
  border-radius: var(--radius-sm);
  margin-left: 0.25rem;
  flex-shrink: 0;
}
.hash-btn-icon {
  font-family: var(--font-mono);
  font-size: 0.875rem;
  font-weight: 600;
}
.attach-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.skill-dropdown {
  position: absolute;
  bottom: 100%;
  left: -1px;
  right: -1px;
  max-height: 13.75rem;
  overflow-y: auto;
  background: var(--bg-surface);
  border: 1px solid var(--accent-glow);
  border-radius: var(--radius-sm);
  box-shadow: 0 -0.25rem 0.75rem rgba(0, 0, 0, 0.4);
  z-index: 100;
  margin-bottom: 0.25rem;
}
.skill-dropdown-item {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  padding: 0.375rem 0.625rem;
  font-size: 0.75rem;
  transition: background 0.1s;
}
.skill-dropdown-item:hover,
.skill-dropdown-item.highlighted {
  background: var(--accent-subtle);
}
.skill-dropdown-name {
  color: var(--accent);
  font-weight: 500;
  font-family: var(--font-mono);
  flex-shrink: 0;
}
.skill-dropdown-kind-icon {
  color: var(--text-muted);
  flex-shrink: 0;
  align-self: center;
}
.skill-dropdown-args {
  color: var(--text-muted);
  font-size: 0.6875rem;
  font-family: var(--font-mono);
  flex-shrink: 0;
}
.skill-dropdown-desc {
  flex: 1;
  min-width: 0;
  color: var(--text-muted);
  font-size: 0.6875rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.queued-area {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.5rem 0.5rem 0 0.5rem;
}
.queued-chip {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.25rem 0.5rem;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  font-size: 0.75rem;
  font-family: var(--font-ui);
  color: var(--text-secondary);
}
.queued-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.queued-remove {
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  color: var(--text-muted);
  padding: 0;
}
.queued-remove:hover {
  color: var(--text-primary);
}
</style>