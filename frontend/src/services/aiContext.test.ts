import { describe, expect, it } from 'vitest'
import type { AIMessage } from '../types/ai'
import {
  activeHistory, compactionCut, contextGroups, estimateRequestTokens,
  estimateTokens, splitSummaryGroups,
} from './aiContext'

const message = (id: string, role: AIMessage['role'], content = 'hello'): AIMessage => ({ id, role, content })

describe('AI context budgeting and compaction boundaries', () => {
  it('counts actual system, tools, headers and attachments without tokenizing base64', () => {
    const text = estimateRequestTokens('system', [{ role: 'user', content: 'hello' }], [])
    expect(estimateRequestTokens('system', [{ role: 'user', content: 'hello' }], [{ description: 'x'.repeat(10000) }])).toBeGreaterThan(text + 2000)
    expect(estimateTokens({ type: 'image', source: { data: 'A'.repeat(100_000) } })).toBe(4096)
  })

  it('never separates a parallel tool invocation from its results', () => {
    const call = message('call', 'assistant', '')
    call.tool_calls = ['a', 'b'].map(id => ({ id, type: 'function', function: { name: 'read', arguments: '{}' } }))
    const history = [message('old', 'user', 'x'.repeat(10000)), call,
      { ...message('a-result', 'tool', 'x'.repeat(10000)), tool_call_id: 'a' },
      { ...message('b-result', 'tool'), tool_call_id: 'b' }, message('reply', 'assistant'), message('new', 'user')]
    expect(contextGroups(history).map(g => g.map(m => m.id))).toEqual([
      ['old'], ['call', 'a-result', 'b-result'], ['reply'], ['new'],
    ])
    expect(compactionCut(history, 100)).toBe(4)
  })

  it('treats a dangling tool_use as compactable but still keeps the newest meaningful group', () => {
    // Legacy sessions can contain a tool_use whose result never arrived (e.g.
    // a dropped parallel call from before the skip-result fix). It must not
    // glue the entire remaining transcript into one unsplittable group — that
    // made compactionCut a no-op and every later turn died with
    // ai.contextTooLarge. buildConversation filters the dangling block from
    // the request, so compaction may cut through it.
    const pending = { ...message('call', 'assistant'), tool_calls: [{ id: 'pending', type: 'function' as const, function: { name: 'run', arguments: '{}' } }] }
    const history = [message('old', 'user', 'x'.repeat(10000)), pending, message('latest', 'user', 'x'.repeat(200000))]
    expect(contextGroups(history).map(g => g.map(m => m.id))).toEqual([
      ['old'], ['call'], ['latest'],
    ])
    // The dangling call can be trimmed under budget pressure, but the newest
    // meaningful group (the enormous latest input) is always retained intact.
    expect(compactionCut(history, 100)).toBe(2)
  })

  it('closes a resolved chain at its result and keeps the newest group intact', () => {
    // A resolved invocation groups itself with ALL of its results, then the
    // group closes — following messages form their own groups. The enormous
    // latest input is still never compacted away: compactionCut retains the
    // newest meaningful group.
    const call = { ...message('call', 'assistant'), tool_calls: [{ id: 'run1', type: 'function' as const, function: { name: 'run', arguments: '{}' } }] }
    const history = [message('old', 'user', 'x'.repeat(10000)), call,
      { ...message('run1-result', 'tool', 'x'.repeat(10000)), tool_call_id: 'run1' }, message('latest', 'user', 'x'.repeat(200000))]
    expect(contextGroups(history).map(g => g.map(m => m.id))).toEqual([
      ['old'], ['call', 'run1-result'], ['latest'],
    ])
    // Under tight budget pressure only the newest meaningful group survives.
    expect(compactionCut(history, 100)).toBe(3)
  })

  it('uses checkpoint IDs and ignores missing or unsupported checkpoints', () => {
    const history = [message('a', 'user'), message('b', 'assistant'), message('c', 'user')]
    const summary = { version: 1 as const, content: 'handoff', throughMessageId: 'b', createdAt: 1 }
    expect(activeHistory(history, summary).map(m => m.id)).toEqual(['c'])
    expect(activeHistory(history, { ...summary, throughMessageId: 'deleted' })).toEqual(history)
  })

  it('splits history into request-sized runs of complete tool groups', () => {
    const call = { ...message('call', 'assistant'), tool_calls: [{ id: 'run1', type: 'function' as const, function: { name: 'run', arguments: '{}' } }] }
    const history = [
      message('a', 'user', 'x'.repeat(14000)),
      call,
      { ...message('run1-result', 'tool', 'x'.repeat(14000)), tool_call_id: 'run1' },
      message('b', 'user', 'x'.repeat(14000)),
    ]
    const runs = splitSummaryGroups(history, 6000)
    expect(runs.length).toBe(3)
    // A tool invocation and its result never land in different requests.
    const ids = runs.map(run => run.map(m => m.id))
    expect(ids).toEqual([['a'], ['call', 'run1-result'], ['b']])
  })

  it('keeps a single oversized group whole instead of splitting it', () => {
    const huge = message('huge', 'user', 'x'.repeat(100000))
    const history = [message('a', 'user', 'x'.repeat(14000)), huge]
    const runs = splitSummaryGroups(history, 6000)
    expect(runs.map(run => run.map(m => m.id))).toEqual([['a'], ['huge']])
  })
})
