<template>
  <div class="command-send-panel">
    <div class="cs-toolbar">
      <el-select v-model="scope" class="cs-select" :title="t('commandSend.scopeHint')">
        <el-option :label="t('commandSend.scopeCurrent')" value="current" />
        <el-option :label="t('commandSend.scopeWorkspace')" value="workspace" />
        <el-option :label="t('commandSend.scopeAllTabs')" value="allTabs" />
      </el-select>
      <el-select v-model="mode" class="cs-select" :title="t('commandSend.modeHint')">
        <el-option :label="t('commandSend.modeLines')" value="lines" />
        <el-option :label="t('commandSend.modeBracketed')" value="bracketed" />
      </el-select>
      <div class="cs-toolbar-spacer" />
      <button class="btn btn-ghost btn-icon btn-sm" :title="t('commandSend.clear')" :disabled="!draft" @click="clearDraft">
        <Eraser :size="14" />
      </button>
      <button
        class="btn btn-primary btn-sm cs-send"
        :title="sendTitle"
        :disabled="!canSend"
        @click="send"
      >
        <Send :size="14" />
        <span>{{ t('commandSend.send') }}</span>
      </button>
    </div>

    <div class="cs-editor">
      <SyntaxEditor
        ref="editorRef"
        v-model="draft"
        compact
        :wrap="wrap"
        :font-size="12"
        @execute="send"
      />
    </div>

    <div class="cs-meta">
      <span>{{ t('commandSend.meta', { lines: lineCount, chars: draft.length }) }}</span>
      <span v-if="draft && targets.length === 0" class="cs-no-target">{{ t('commandSend.noTarget') }}</span>
      <el-checkbox v-model="wrap" class="cs-wrap">{{ t('commandSend.wrap') }}</el-checkbox>
    </div>
  </div>
</template>

<script setup lang="ts">
// Command-send panel (issue #1341): compose commands in an editor and push
// them to terminals — current panel, the whole active workspace, or every
// tab. Sending to the active panel follows typed input: when it
// participates in broadcast the write fans out to every broadcast panel.
// No props/events — dock views pull their own stores (see HistoryPanel).
import { ref, computed, onMounted, watch } from 'vue'
import { Eraser, Send } from '@lucide/vue'
import { useI18n } from '../i18n'
import { useSettingsStore } from '../stores/settingsStore'
import { useLocalStateStore } from '../stores/localStateStore'
import { queuedSessionWrite } from '../services/sessionWriter'
import { focusActivePanelTerminal } from '../composables/useFocusTerminal'
import { msg } from '../services/message'
import SyntaxEditor from './SyntaxEditor.vue'
import {
  resolveSendTargets,
  buildSendPayload,
  targetBracketedPasteMode,
  type CommandSendScope,
} from '../utils/commandSend'

const { t } = useI18n()
const settingsStore = useSettingsStore()
const localState = useLocalStateStore()

// Draft persists across restarts (device-local). Saved debounced per edit
// via update(); scope/mode live in the synced settings where sensible.
const draft = ref('')

const scope = ref<CommandSendScope>('current')
// Delivery semantics — a synced terminal setting (Settings → Terminal).
const mode = computed<'lines' | 'bracketed'>({
  get: () => settingsStore.settings.terminal.commandSendMode ?? 'lines',
  set: (v) => {
    settingsStore.settings.terminal.commandSendMode = v
    settingsStore.save()
  },
})

const targets = computed(() => resolveSendTargets(scope.value))
const lineCount = computed(() => draft.value.replace(/\n+$/, '').split('\n').length)

// Word wrap of the editor — device-local preference, survives restarts.
const wrap = computed({
  get: () => localState.state.commandSendWrap ?? true,
  set: (v: boolean) => { void localState.update({ commandSendWrap: v }) },
})

const canSend = computed(() => draft.value.trim().length > 0 && targets.value.length > 0)
const sendTitle = computed(() => t('commandSend.sendHint', { n: targets.value.length }))

onMounted(async () => {
  await localState.init()
  draft.value = localState.state.commandSendDraft || ''
})

// Debounced draft persistence: keystrokes stay cheap, restart keeps the text.
watch(draft, persistDraft)

let draftSaveTimer: ReturnType<typeof setTimeout> | null = null
function persistDraft() {
  if (draftSaveTimer !== null) clearTimeout(draftSaveTimer)
  draftSaveTimer = setTimeout(() => {
    draftSaveTimer = null
    void localState.update({ commandSendDraft: draft.value })
  }, 400)
}

function clearDraft() {
  draft.value = ''
  void localState.update({ commandSendDraft: '' })
}

function send() {
  if (!canSend.value) return
  const modeValue = mode.value
  const text = draft.value
  for (const sid of targets.value) {
    queuedSessionWrite(sid, buildSendPayload(text, modeValue, targetBracketedPasteMode(sid)))
  }
  msg.success(t('commandSend.sent', { n: targets.value.length }))
  clearDraft()
  // Return focus to the terminal like the history/quick-command runners do.
  focusActivePanelTerminal()
}
</script>

<style scoped>
.command-send-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.cs-toolbar {
  display: flex;
  /* Wrap so the two selects stack in a narrow dock (left sidebar) instead
     of overflowing; in the wide bottom panel everything fits one row. */
  flex-wrap: wrap;
  align-items: center;
  gap: 0.375rem;
  padding: 0.5rem 0.625rem;
  flex-shrink: 0;
}

.cs-select {
  /* Compact fixed width: no stretching in a wide dock, shrink + wrap when
     the dock is narrow (left sidebar). */
  flex: 0 1 8.5rem;
  min-width: 0;
}

.cs-toolbar-spacer {
  flex: 1;
}

.cs-send {
  display: flex;
  align-items: center;
  gap: 0.3rem;
}

.cs-editor {
  flex: 1;
  min-height: 0;
  padding: 0 0.625rem;
}

.cs-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.3rem 0.625rem 0.5rem;
  font-size: 0.6875rem;
  color: var(--text-muted);
  flex-shrink: 0;
}

.cs-wrap {
  height: auto;
  flex-shrink: 0;
}

.cs-wrap :deep(.el-checkbox__label) {
  font-size: 0.6875rem;
  color: var(--text-muted);
}

.cs-no-target {
  color: var(--el-color-warning);
}
</style>
