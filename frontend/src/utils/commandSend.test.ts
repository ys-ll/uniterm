// Tests for the command-send panel helpers (issue #1341): payload building
// and target resolution. Store access is mocked so the targeting rules can
// be exercised without wiring a real panel/tab graph.
import { describe, it, expect, vi, beforeEach } from 'vitest'

const state = vi.hoisted(() => ({
  activeTab: null as any,
  tabs: [] as any[],
  panels: new Map<string, any>(),
  broadcastingPanelIds: new Set<string>(),
}))

vi.mock('../stores/tabStore', () => ({
  useTabStore: () => ({
    get activeTab() { return state.activeTab },
    get tabs() { return state.tabs },
    isPanelBroadcasting: (pid: string) => state.broadcastingPanelIds.has(pid),
    getAllBroadcastPanelIds: () => [...state.broadcastingPanelIds],
  }),
}))

vi.mock('../stores/panelStore', () => ({
  usePanelStore: () => ({
    getPanel: (pid: string) => state.panels.get(pid),
  }),
}))

vi.mock('../services/terminalManager', () => ({
  getManagedTerminal: (sid: string) =>
    sid === 'sid-bracket'
      ? { terminal: { modes: { bracketedPasteMode: true } } }
      : undefined,
}))

import { resolveSendTargets, buildSendPayload, targetBracketedPasteMode } from './commandSend'

function panel(pid: string, type: string, sessionId: string | null) {
  state.panels.set(pid, { id: pid, type, sessionId })
}

beforeEach(() => {
  state.activeTab = null
  state.tabs = []
  state.panels.clear()
  state.broadcastingPanelIds.clear()
})

describe('resolveSendTargets', () => {
  it('resolves the focused panel of a workspace tab', () => {
    panel('p1', 'ssh', 'sid1')
    panel('p2', 'ssh', 'sid2')
    state.activeTab = { type: 'workspace', panelIds: ['p1', 'p2'], activePanelId: 'p2' }
    expect(resolveSendTargets('current')).toEqual(['sid2'])
    expect(resolveSendTargets('workspace')).toEqual(['sid1', 'sid2'])
  })

  it('resolves the single panel of a terminal tab', () => {
    panel('p1', 'local', 'sid1')
    state.activeTab = { type: 'terminal', panelId: 'p1' }
    expect(resolveSendTargets('current')).toEqual(['sid1'])
    expect(resolveSendTargets('workspace')).toEqual(['sid1'])
  })

  it('fans out to every broadcast panel when the active panel broadcasts', () => {
    panel('p1', 'ssh', 'sid1')
    panel('p2', 'ssh', 'sid2')
    panel('p3', 'rdp', 'sid3')
    state.activeTab = { type: 'workspace', panelIds: ['p1', 'p2'], activePanelId: 'p1' }
    state.broadcastingPanelIds.add('p1')
    state.broadcastingPanelIds.add('p2')
    // Non-PTY panels (rdp) never receive writes.
    expect(resolveSendTargets('current')).toEqual(['sid1', 'sid2'])
  })

  it('collects deduplicated targets across all tabs', () => {
    panel('p1', 'ssh', 'sid1')
    panel('p2', 'telnet', 'sid2')
    panel('p3', 'ssh', 'sid1')
    panel('p4', 'ssh', null)
    state.tabs = [
      { type: 'terminal', panelId: 'p1' },
      { type: 'terminal', panelId: 'p2' },
      { type: 'workspace', panelIds: ['p3', 'p4'] },
    ]
    // telnet panels are not PTY-backed by the terminal writer, unbound
    // panels are skipped, duplicate sessions collapse.
    expect(resolveSendTargets('allTabs')).toEqual(['sid1'])
    expect(resolveSendTargets('workspace')).toEqual([])
  })

  it('returns nothing without an active tab', () => {
    state.activeTab = null
    expect(resolveSendTargets('current')).toEqual([])
    expect(resolveSendTargets('allTabs')).toEqual([])
  })
})

describe('buildSendPayload', () => {
  it('converts every newline to Enter in lines mode', () => {
    expect(buildSendPayload('a\r\nb\nc', 'lines', false)).toBe('a\rb\rc')
    // The final line executes too.
    expect(buildSendPayload('ls\n', 'lines', false)).toBe('ls\r')
  })

  it('wraps the whole block when the target enabled bracketed paste', () => {
    expect(buildSendPayload('a\r\nb', 'bracketed', true)).toBe('\x1b[200~a\nb\x1b[201~')
  })

  it('falls back to lines mode when the target has no bracketed paste', () => {
    expect(buildSendPayload('a\r\nb', 'bracketed', false)).toBe('a\rb')
  })
})

describe('targetBracketedPasteMode', () => {
  it('reads the mode from the managed terminal, false when unknown', () => {
    expect(targetBracketedPasteMode('sid-bracket')).toBe(true)
    expect(targetBracketedPasteMode('unknown')).toBe(false)
  })
})
