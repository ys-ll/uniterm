import type { AIContextSummary, AIMessage } from '../types/ai'
import { DEFAULT_CONTEXT_TOKEN_BUDGET } from '../types/settings'

export const CONTEXT_TRIGGER_TOKENS = DEFAULT_CONTEXT_TOKEN_BUDGET
export const CONTEXT_TARGET_TOKENS = 60_000
export const SUMMARY_MAX_TOKENS = 4096
export const SUMMARY_CHUNK_TOKENS = 16_000

// Approximation, not a model tokenizer. Images have a conservative fixed
// allowance; base64 bytes must never be treated as natural-language tokens.
export function estimateTokens(value: unknown): number {
  if (typeof value === 'string') {
    let cost = 0
    for (let i = 0; i < value.length; i++) cost += value.charCodeAt(i) <= 0x7f ? 1 / 3.5 : 1 / 1.8
    return Math.ceil(cost)
  }
  if (Array.isArray(value)) return value.reduce((n, v) => n + estimateTokens(v), 0)
  if (value && typeof value === 'object') {
    const obj = value as Record<string, unknown>
    if (obj.type === 'image' || obj.type === 'image_url' || obj.type === 'input_image') return 4096
    return Object.entries(obj).reduce((n, [key, v]) => n + estimateTokens(key) + estimateTokens(v), 4)
  }
  return value == null ? 0 : 1
}

export function estimateRequestTokens(system: string, messages: Array<Record<string, unknown>>, tools: unknown): number {
  return estimateTokens(system) + estimateTokens(messages) + estimateTokens(tools)
}

export function activeHistory(messages: AIMessage[], summary?: AIContextSummary): AIMessage[] {
  if (!summary || summary.version !== 1 || !summary.content) return messages
  const through = messages.findIndex(m => m.id === summary.throughMessageId)
  return through < 0 ? messages : messages.slice(through + 1)
}

function toolIds(message: AIMessage): string[] {
  const raw = message._rawApiMsg?.content
  if (Array.isArray(raw)) return raw.filter(b => b.type === 'tool_use').map(b => b.id)
  return message.tool_calls?.map(tc => tc.id) || []
}

export function isContextMessage(m: AIMessage): boolean {
  if (m.id.startsWith('dbg-') || m.needsContinue) return false
  if (m.role === 'tool') return !!m.tool_call_id
  if (m.role === 'assistant') return !!(m.content || m._rawApiMsg || m.tool_calls?.length)
  return !!(m.content || m._contextHeader || m.attachments?.length)
}

// A tool invocation and ALL of its results form an indivisible group.
// An unresolved invocation protects the remaining suffix from compaction.
//
// "Unresolved" means the invocation has no matching tool_result anywhere in
// the transcript. A tool_use that never got a result (the model asked for a
// parallel call that was never executed) would otherwise keep `pending`
// non-empty forever and swallow the entire remaining transcript into one
// group — an unsplittable group that compactionCut() must retain, which then
// pushes the request past CONTEXT_TRIGGER_TOKENS (it still goes out; the
// oversized tool results in it get stubbed by prepareAgentContext).
export function contextGroups(messages: AIMessage[]): AIMessage[][] {
  const resolved = new Set<string>()
  for (const m of messages) {
    if (m.role === 'tool' && m.tool_call_id) resolved.add(m.tool_call_id)
  }
  const groups: AIMessage[][] = []
  for (let i = 0; i < messages.length; i++) {
    const m = messages[i]
    const group = [m]
    const pending = new Set(toolIds(m).filter(id => resolved.has(id)))
    if (pending.size) {
      while (i + 1 < messages.length && pending.size) {
        const next = messages[++i]
        group.push(next)
        if (next.role === 'tool' && next.tool_call_id) pending.delete(next.tool_call_id)
      }
    }
    groups.push(group)
  }
  return groups
}

function messageTokens(m: AIMessage): number {
  if (!isContextMessage(m)) return 0
  const content = m._rawApiMsg || { role: m.role, content: m.content, tool_calls: m.tool_calls }
  let tokens = estimateTokens(content) + estimateTokens(m._contextHeader)
  for (const a of m.attachments || []) tokens += a.kind === 'image' ? 4096 : estimateTokens(a.data)
  return tokens
}

export function compactionCut(messages: AIMessage[], recentBudget: number): number {
  const groups = contextGroups(messages)
  let kept = 0
  let cut = messages.length
  let keptGroups = 0
  for (let i = groups.length - 1; i >= 0; i--) {
    const group = groups[i]
    const tokens = group.reduce((n, m) => n + messageTokens(m), 0)
    // The newest meaningful group is always retained, even when it alone is
    // above the target. Older groups are kept only while the budget permits.
    if (keptGroups >= 1 && kept + tokens > recentBudget) break
    kept += tokens
    cut -= group.length
    if (group.some(isContextMessage)) keptGroups++
  }
  return cut
}

export function summaryTranscript(messages: AIMessage[]): string {
  return messages.filter(isContextMessage).map(m => {
    const record: Record<string, unknown> = { role: m.role }
    if (m._contextHeader) record.environmentAndInstructions = m._contextHeader
    record.content = m.content
    if (m._rawApiMsg && Array.isArray(m._rawApiMsg.content)) {
      if (!m.content) record.content = m._rawApiMsg.content.filter(b => b.type === 'text').map(b => b.text).join('\n')
      record.toolCalls = m._rawApiMsg.content.filter(b => b.type === 'tool_use')
    } else if (m.tool_calls?.length) record.toolCalls = m.tool_calls
    if (m.tool_call_id) record.toolCallId = m.tool_call_id
    if (m.attachments?.length) record.attachments = m.attachments.map(a => ({
      name: a.name,
      content: a.kind === 'text' ? a.data : '[Image unavailable in this text handoff; original attachment remains in chat history. Do not infer its contents.]',
    }))
    return JSON.stringify(record)
  }).join('\n')
}

// Bounded chunks also handle old sessions and individual multi-MB tool results.
// Only the summarizer's text transcript is split, never the live tool chain.
export function splitSummaryTranscript(transcript: string): string[] {
  const chunks: string[] = []
  let start = 0
  let cost = 0
  for (let i = 0; i < transcript.length; i++) {
    cost += transcript.charCodeAt(i) <= 0x7f ? 1 / 3.5 : 1 / 1.8
    if (cost >= SUMMARY_CHUNK_TOKENS && i + 1 < transcript.length) {
      // Do not split a UTF-16 surrogate pair.
      if (transcript.charCodeAt(i) >= 0xd800 && transcript.charCodeAt(i) <= 0xdbff) continue
      chunks.push(transcript.slice(start, i + 1))
      start = i + 1
      cost = 0
    }
  }
  if (start < transcript.length) chunks.push(transcript.slice(start))
  return chunks
}
