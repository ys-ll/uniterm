<template>
  <el-dialog
    append-to-body
    v-model="visible"
    :title="t('terminal.multilinePasteTitle')"
    width="32rem"
    :close-on-click-modal="false"
    @open="onOpen"
    @close="onCancel"
  >
    <div class="multiline-paste-warning">
      <el-icon class="warning-icon"><WarningFilled /></el-icon>
      <span>{{ t('terminal.multilinePasteWarning', { n: lineCount }) }}</span>
    </div>
    <div class="multiline-paste-editor">
      <SyntaxEditor ref="editorRef" v-model="state.text" compact :wrap="wrap" :font-size="12" />
    </div>
    <div class="multiline-paste-meta-row">
      <div class="multiline-paste-meta">{{ t('terminal.multilinePasteMeta', { lines: lineCount, chars: state.text.length }) }}</div>
      <el-checkbox v-model="wrap" class="multiline-paste-wrap">
        {{ t('terminal.multilinePasteWrap') }}
      </el-checkbox>
    </div>
    <template #footer>
      <div class="multiline-paste-footer">
        <el-checkbox v-model="state.dontAskAgain" class="multiline-paste-dontask">
          {{ t('terminal.multilinePasteDontAsk') }}
        </el-checkbox>
        <div class="multiline-paste-footer-buttons">
          <el-button @click="onCancel">{{ t('common.cancel') }}</el-button>
          <el-button type="primary" @click="onConfirm">{{ t('terminal.multilinePasteConfirm') }}</el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
// Multi-line paste confirmation (issue #1370): editable preview of clipboard
// content before it is written to a terminal session. The editor is the
// shared SyntaxEditor (CodeMirror) — line numbers stay aligned with each
// logical line even when soft wrap folds one, unlike a raw textarea.
// State lives in composables/multilinePasteDialog.ts; this component is the
// single mounted instance (App.vue).
import { computed, ref, nextTick } from 'vue'
import { WarningFilled } from '@element-plus/icons-vue'
import { useI18n } from '../i18n'
import { useMultilinePasteDialogState, finishMultilinePaste } from '../composables/multilinePasteDialog'
import SyntaxEditor from './SyntaxEditor.vue'

const { t } = useI18n()

const state = useMultilinePasteDialogState()
const editorRef = ref<InstanceType<typeof SyntaxEditor> | null>(null)

// Word wrap on/off (default on — full commands stay readable without
// hunting for a horizontal scrollbar). Toggling rebuilds the CodeMirror
// editor via the prop watcher.
const wrap = ref(true)

const visible = computed({
  get: () => state.visible,
  set: (v: boolean) => { if (!v) return } // close always goes through @close → onCancel
})

// Count content lines the way the paste path executes them: trailing
// newlines are stripped first (mirrors isMultilinePaste).
const lineCount = computed(() => state.text.replace(/\n+$/, '').split('\n').length)

function onOpen() {
  nextTick(() => editorRef.value?.focus())
}

function onConfirm() {
  finishMultilinePaste(state.text, state.dontAskAgain)
}

function onCancel() {
  finishMultilinePaste(null, false)
}
</script>

<style scoped>
.multiline-paste-warning {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.6rem;
  color: var(--el-color-warning);
}

.multiline-paste-warning .warning-icon {
  flex-shrink: 0;
}

.multiline-paste-warning span {
  color: var(--el-text-color-primary);
  line-height: 1.4;
}

.multiline-paste-editor {
  height: 14rem;
}

.multiline-paste-meta-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 0.3rem;
}

.multiline-paste-meta {
  font-size: 11px;
  color: var(--el-text-color-secondary);
}

.multiline-paste-wrap {
  height: auto;
}

.multiline-paste-wrap :deep(.el-checkbox__label) {
  font-size: 11px;
  color: var(--el-text-color-secondary);
}

.multiline-paste-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}

.multiline-paste-footer-buttons {
  display: flex;
  gap: 0.5rem;
}
</style>
