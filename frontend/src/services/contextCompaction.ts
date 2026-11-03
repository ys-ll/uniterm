import { SummarizeAIContext } from '../../bindings/github.com/ys-ll/uniterm/app'
import { useAIStore } from '../stores/aiStore'
import { useSettingsStore } from '../stores/settingsStore'
import type { AIContextSummary } from '../types/ai'
import type { AIModelConfig } from '../types/settings'
import { t } from '../i18n'
import { ChatCancelledError } from './llm'
import {
  activeHistory, compactionCut, estimateRequestTokens, estimateTokens,
  summaryTranscript, splitSummaryTranscript, CONTEXT_TRIGGER_TOKENS,
  CONTEXT_TARGET_TOKENS, SUMMARY_MAX_TOKENS,
} from './aiContext'

// Single tool_result blocks below this size are left alone: stubbing them
// cannot meaningfully shrink the payload and would only destroy context the
// model still benefits from.
const STUB_MIN_TOOL_RESULT_TOKENS = 1024

// Replace the largest tool_result blocks with explicit stubs until the
// payload fits the trigger budget. Only the request copy is modified — the
// messages built by buildConversation are fresh objects, so the stored
// history (and the UI) keep the full tool output. The stub text states the
// tool name and size and points at a narrower re-run, so the model never
// mistakes a dropped result for a failed or empty one and does not silently
// assume the omitted data.
//
// Oversized tool results are the one bloat source compaction cannot remove:
// the newest group is always retained, so a single enormous output stays in
// the payload no matter how much older history is cut. Stubbing is the last
// degradation step — it only runs when the request is already over budget.
function stubOversizedToolResults(
  messages: Array<Record<string, unknown>>,
  budgetTokens: number,
): Array<Record<string, unknown>> {
  const names = new Map<string, string>()
  for (const m of messages) {
    if (!Array.isArray(m.content)) continue
    for (const block of m.content as Array<Record<string, unknown>>) {
      if (block.type === 'tool_use' && typeof block.id === 'string') {
        names.set(block.id, typeof block.name === 'string' ? block.name : 'unknown')
      }
    }
  }
  // Shallow-copy every array-content message so mutation stays local.
  const out = messages.map(m => Array.isArray(m.content) ? { ...m, content: [...m.content as unknown[]] } : m)
  let total = estimateTokens(out)
  for (;;) {
    if (total < budgetTokens) break
    let target: { msg: Record<string, unknown>; index: number; size: number } | null = null
    for (const m of out) {
      if (!Array.isArray(m.content)) continue
      const content = m.content as Array<Record<string, unknown>>
      for (let index = 0; index < content.length; index++) {
        const block = content[index]
        if (block.type !== 'tool_result') continue
        const size = estimateTokens(block.content)
        if (size >= STUB_MIN_TOOL_RESULT_TOKENS && (!target || size > target.size)) {
          target = { msg: m, index, size }
        }
      }
    }
    if (!target) break
    const content = target.msg.content as Array<Record<string, unknown>>
    const block = content[target.index]
    const name = names.get(String(block.tool_use_id)) || 'unknown'
    content[target.index] = {
      ...block,
      content: `[Tool result omitted to fit the context budget: tool "${name}", about ${target.size} tokens. The tool ran; only its output was dropped here. Re-run it with a narrower query if this data is essential.]`,
    }
    total = estimateTokens(out)
  }
  return out
}

export async function prepareAgentContext(system: string, tools: unknown[], skipCompaction = false, isCurrent = () => true, modelSnapshot?: AIModelConfig): Promise<{
  messages: Array<Record<string, unknown>>
  failed: boolean
}> {
  const store = useAIStore()
  const settings = useSettingsStore()
  const preferences = settings.ai
  const sessionId = store.currentSessionId
  const revision = store.contextRevision
  const started = performance.now()
  if (preferences?.autoCompact === false) {
    checkCurrent()
    store.contextNotice = ''
    store.contextStats = null
    // Full history remains available even if this session has an earlier
    // checkpoint. Disabling compaction must not silently omit that history.
    return { messages: store.buildConversation(store.messages), failed: false }
  }
  const configuredBudget = preferences?.contextTokenBudget
  const triggerTokens = typeof configuredBudget === 'number' && Number.isFinite(configuredBudget)
    ? Math.max(16000, Math.min(2000000, Math.floor(configuredBudget))) : CONTEXT_TRIGGER_TOKENS
  const targetTokens = Math.floor(triggerTokens * CONTEXT_TARGET_TOKENS / CONTEXT_TRIGGER_TOKENS)

  // Last-step degradation before a request goes out: once the assembled
  // payload is over the trigger, stub the largest tool results (request copy
  // only). Whatever remains is sent even if still over budget — the provider
  // is the authority on its own context window, and rejecting locally turned
  // recoverable turns into permanently stuck sessions.
  function finalize(messages: Array<Record<string, unknown>>) {
    if (estimateRequestTokens(system, messages, tools) < triggerTokens) return messages
    return stubOversizedToolResults(messages, triggerTokens)
  }

  const prior = store.contextSummary
  const history = activeHistory(store.messages, prior)
  checkCurrent()
  const original = store.buildConversation(history, prior)
  const inputTokens = estimateRequestTokens(system, original, tools)
  if (inputTokens < triggerTokens) return { messages: original, failed: false }

  // Compaction can only remove older groups: the newest meaningful group is
  // always retained, and system prompt + tool definitions are fixed cost.
  // When there is nothing to cut, send the payload as-is — oversized user
  // input in particular is never silently dropped or shortened.
  const overhead = estimateRequestTokens(system, [], tools)
  const recentBudget = Math.max(1024, targetTokens - overhead - SUMMARY_MAX_TOKENS)
  const cut = compactionCut(history, recentBudget)
  if (cut === 0) {
    return { messages: finalize(original), failed: false }
  }

  function checkCurrent() {
    if (!isCurrent() || store.stopRequested || store.currentSessionId !== sessionId || store.contextRevision !== revision) {
      throw new ChatCancelledError()
    }
  }

  const recent = history.slice(cut)
  let summary: AIContextSummary | undefined
  let failed = skipCompaction
  if (!skipCompaction) {
    // A removed summary model falls back to the current request's model.
    const configuredModel = preferences?.models?.find(m => m.id === preferences.summaryModelId)
      || modelSnapshot || settings.activeModel
    const model = configuredModel ? { ...configuredModel } : undefined
    const requestId = `compact-${crypto.randomUUID()}`
    store.compactionRequestId = requestId
    store.status = 'compacting'
    try {
      if (!model?.apiKey) throw new Error('API key not configured')
      let content = prior?.content || ''
      for (const chunk of splitSummaryTranscript(summaryTranscript(history.slice(0, cut)))) {
        checkCurrent()
        content = await SummarizeAIContext(
          model.apiKey, model.baseURL, model.model, model.protocol,
          model.userAgent || '', model.proxyId || '', requestId, content, chunk,
        )
        checkCurrent()
        if (!content.trim() || estimateTokens(content) > SUMMARY_MAX_TOKENS) throw new Error('Invalid summary size')
      }
      if (!content.trim()) throw new Error('Empty history summary')
      summary = { version: 1, content, throughMessageId: history[cut - 1].id, createdAt: Date.now() }
      checkCurrent()
      // Commit the checkpoint even when the retained latest exchange alone
      // keeps the request over budget: the checkpoint permanently shrinks
      // what future turns have to carry, and an oversized latest group is
      // dealt with by stubbing, not by discarding the summary.
      if (!store.applyContextSummary(summary, revision)) throw new ChatCancelledError()
      store.contextNotice = ''
      await store.doSave()
      checkCurrent()
    } catch (error) {
      checkCurrent()
      if (error instanceof ChatCancelledError) throw error
      failed = true
      summary = undefined
    } finally {
      if (store.compactionRequestId === requestId) {
        store.compactionRequestId = ''
        if (store.status === 'compacting') store.status = 'thinking'
      }
    }
  }

  // Failed summaries are never committed. Keep complete recent groups for
  // this request, with a visible notice; do not retry on every tool step.
  const requestSummary = summary || (failed ? {
    version: 1 as const,
    content: `${prior?.content || ''}\n[Some additional older exchanges were omitted because summarization failed. Do not assume omitted operations succeeded or infer permissions. Ask the user if missing context is needed.]`,
    throughMessageId: history[cut - 1].id,
    createdAt: Date.now(),
  } : prior)
  const messages = finalize(store.buildConversation(recent, requestSummary))
  const outputTokens = estimateRequestTokens(system, messages, tools)
  if (failed) store.contextNotice = t('ai.compactionFailed')
  store.contextStats = { beforeTokens: inputTokens, afterTokens: outputTokens, durationMs: Math.round(performance.now() - started), fallback: failed }
  return { messages, failed }
}
