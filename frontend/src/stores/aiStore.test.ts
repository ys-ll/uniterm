// Per-tab AI conversation binding: each terminal tab owns one conversation;
// runs are serialized app-wide and keep writing to their owning session even
// while the sidebar shows another tab.

import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const cancelChatStream = vi.fn().mockResolvedValue(undefined)
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({
  SaveAISessions: vi.fn().mockResolvedValue(undefined),
  LoadAISessions: vi.fn().mockResolvedValue({ sessions: [], currentSessionId: '' }),
  CancelChatStream: (...args: any[]) => cancelChatStream(...(args as [])),
}))

const panels: Record<string, { id: string; title: string; sessionId: string | null; config?: any }> = {
  'panel-a': { id: 'panel-a', title: 'A', sessionId: 'sess-a', config: { shellPath: '/bin/bash' } },
  'panel-b': { id: 'panel-b', title: 'B', sessionId: 'sess-b', config: { shellPath: '/bin/zsh' } },
}
vi.mock('./panelStore', () => ({
  usePanelStore: vi.fn(() => ({
    getPanel: (id: string) => panels[id],
  })),
}))

import { useAIStore } from './aiStore'

function userMsg(content: string) {
  return { id: `msg-${Math.random().toString(36).slice(2)}`, role: 'user' as const, content }
}

beforeEach(() => {
  setActivePinia(createPinia())
  cancelChatStream.mockClear()
})

describe('per-tab conversation binding', () => {
  it('attachTab creates one conversation per tab and switches views', () => {
    const ai = useAIStore()
    ai.attachTab('tab-1', 'First')
    const sid1 = ai.currentSessionId
    expect(sid1).toBeTruthy()
    expect(ai.sessions[0].tabId).toBe('tab-1')

    ai.addMessage(userMsg('hello tab 1'))

    ai.attachTab('tab-2', 'Second')
    expect(ai.currentSessionId).toBeTruthy()
    expect(ai.currentSessionId).not.toBe(sid1)
    expect(ai.messages).toHaveLength(0)

    // Back to tab 1: its conversation is restored
    ai.attachTab('tab-1', 'First')
    expect(ai.currentSessionId).toBe(sid1)
    expect(ai.messages).toHaveLength(1)
    expect(ai.messages[0].content).toBe('hello tab 1')
  })

  it('beginRun pins the run to its tab; a second run is refused', () => {
    const ai = useAIStore()
    ai.attachTab('tab-1', 'First')
    expect(ai.beginRun('tab-1', 'First', 'panel-a')).toBeTruthy()
    ai.isRunning = true // beginRun alone does not flip this; runAgent does
    expect(ai.runTabId).toBe('tab-1')

    // Another tab tries to run while tab-1 owns the stream
    ai.attachTab('tab-2', 'Second')
    expect(ai.backgroundRun).toBe(true)
    expect(ai.beginRun('tab-2', 'Second', 'panel-b')).toBeNull()
  })

  it('a background run writes to its own session, not the visible one', () => {
    const ai = useAIStore()
    ai.attachTab('tab-1', 'First')
    ai.addMessage(userMsg('run here'))
    ai.beginRun('tab-1', 'First', 'panel-a')

    ai.attachTab('tab-2', 'Second')
    ai.addMessage({ id: 'x-1', role: 'assistant', content: 'background reply' })

    // The visible (tab-2) view is untouched by tab-1's run output
    expect(ai.messages).toHaveLength(0)
    const s1 = ai.sessions.find(s => s.tabId === 'tab-1')!
    expect(s1.messages.map(m => m.content)).toEqual(['run here', 'background reply'])

    // conversation() builds from the run snapshot, not the visible array
    ai.beginRun('tab-1', 'First', 'panel-a') // refused: different current session
    expect(ai.runTabId).toBe('tab-1')
  })

  it('runPanelTitle suffix-qualifies the run panel after tab switches', () => {
    const ai = useAIStore()
    ai.attachTab('tab-1', 'First')
    ai.beginRun('tab-1', 'First', 'panel-a')
    ai.attachTab('tab-2', 'Second')
    expect(ai.runPanelTitle()).toBe('A (id: panel-a)')
  })

  it('endRun clears the background state', () => {
    const ai = useAIStore()
    ai.attachTab('tab-1', 'First')
    ai.beginRun('tab-1', 'First', 'panel-a')
    ai.isRunning = true
    ai.attachTab('tab-2', 'Second')
    expect(ai.backgroundRun).toBe(true)
    ai.endRun()
    expect(ai.backgroundRun).toBe(false)
    expect(ai.runTabId).toBeNull()
  })

  it('onTabClosed unbinds the tab and cancels a run it owns', () => {
    const ai = useAIStore()
    ai.attachTab('tab-1', 'First')
    ai.addMessage(userMsg('keep me as history'))
    ai.beginRun('tab-1', 'First', 'panel-a')
    ai.attachTab('tab-2', 'Second')

    ai.onTabClosed('tab-1')

    expect(cancelChatStream).toHaveBeenCalledTimes(1)
    expect(ai.tabSessionMap['tab-1']).toBeUndefined()
    expect(ai.runTabId).toBeNull()
    // The conversation survives as detached history
    const s1 = ai.sessions.find(s => s.tabId === 'tab-1')!
    expect(s1.messages).toHaveLength(1)
    // tab-2 view is intact and unbothered
    expect(ai.viewTabId).toBe('tab-2')
  })

  it('onTabClosed clears the view when the viewed tab closes', () => {
    const ai = useAIStore()
    ai.attachTab('tab-1', 'First')
    ai.onTabClosed('tab-1')
    expect(ai.viewTabId).toBeNull()
    expect(ai.currentSessionId).toBeNull()
  })

  it('switchSession rebinds the conversation to the tab in view', () => {
    const ai = useAIStore()
    ai.attachTab('tab-1', 'First')
    const s1 = ai.currentSessionId!
    ai.attachTab('tab-2', 'Second')
    const s2 = ai.currentSessionId!

    // User picks tab-1's history while looking at tab-2: it moves to tab-2
    ai.switchSession(s1)
    expect(ai.currentSessionId).toBe(s1)
    expect(ai.tabSessionMap['tab-2']).toBe(s1)
    expect(ai.tabSessionMap['tab-1']).toBeUndefined()
    expect(ai.sessions.find(s => s.id === s1)!.tabId).toBe('tab-2')
    // s2 is no longer bound to any tab
    expect(ai.tabSessionMap['tab-2'] === s2).toBe(false)
  })

  it('deleteSession of the running conversation cancels the run and rebinds the tab', () => {
    const ai = useAIStore()
    ai.attachTab('tab-1', 'First')
    ai.addMessage(userMsg('busy work'))
    ai.beginRun('tab-1', 'First', 'panel-a')
    ai.attachTab('tab-2', 'Second')

    ai.deleteSession('session-never') // no-op sanity
    ai.deleteSession(ai.tabSessionMap['tab-1'])

    expect(cancelChatStream).toHaveBeenCalledTimes(1)
    expect(ai.runTabId).toBeNull()
    // tab-2 (the tab in view) got a fresh bound session
    const fresh = ai.sessions.find(s => s.tabId === 'tab-2')!
    expect(fresh.messages).toHaveLength(0)
    expect(ai.currentSessionId).toBe(fresh.id)
  })
})
