// Command-send panel support (issue #1341): resolve the set of sessions a
// send targets and build the payload each target receives. Kept out of the
// component so the targeting rules are unit-testable.
//
// Targets are always PTY-backed panels (ssh/local/wsl) with a bound session.
// Sending to the active panel follows the keyboard: when that panel
// participates in broadcast the write fans out to every broadcast panel,
// exactly like typed input.
import { useTabStore } from '../stores/tabStore'
import { usePanelStore } from '../stores/panelStore'
import { getManagedTerminal } from '../services/terminalManager'
import { normalizePastedText } from './terminalPaste'

export type CommandSendScope = 'current' | 'workspace' | 'allTabs'

function isPtyPanelType(type: string): boolean {
  return type === 'ssh' || type === 'local' || type === 'wsl'
}

/** Session ids of every PTY panel in one tab (workspace: all its panels). */
function tabPanelIds(tab: { type: string; panelId?: string; panelIds?: string[] }): string[] {
  if (tab.type === 'workspace') return tab.panelIds ?? []
  return tab.panelId ? [tab.panelId] : []
}

/** Every sendable session id of the given panels, in panel order. */
export function sendableSessionIds(panelIds: string[]): string[] {
  const panelStore = usePanelStore()
  const out: string[] = []
  for (const pid of panelIds) {
    const p = panelStore.getPanel(pid)
    if (p?.sessionId && isPtyPanelType(p.type)) out.push(p.sessionId)
  }
  return out
}

/**
 * Resolve target session ids for a scope.
 *
 * 'current': the active tab's focused (or only) panel; if it takes part in
 * broadcast, expand to every broadcast panel — mirroring what typed input
 * would reach (BaseTerminal's writeToSessions, cross-tab like the input
 * router). 'workspace': all panels of the active tab. 'allTabs': every
 * panel of every tab. Returns session ids, deduplicated, in tab order.
 */
export function resolveSendTargets(scope: CommandSendScope): string[] {
  const tabStore = useTabStore()
  const panelStore = usePanelStore()

  // 'allTabs' works without any active tab; the other scopes need one.
  if (scope === 'allTabs') {
    const out: string[] = []
    for (const tab of tabStore.tabs) {
      out.push(...sendableSessionIds(tabPanelIds(tab)))
    }
    return [...new Set(out)]
  }

  const activeTab = tabStore.activeTab
  if (!activeTab) return []

  if (scope === 'current') {
    const activePanelId = activeTab.type === 'workspace'
      ? activeTab.activePanelId
      : (activeTab.type === 'terminal' ? activeTab.panelId : null)
    if (!activePanelId) return []
    const panel = panelStore.getPanel(activePanelId)
    if (!panel?.sessionId || !isPtyPanelType(panel.type)) return []
    // Broadcast on: fan out like typed input (BaseTerminal.writeToSessions).
    if (tabStore.isPanelBroadcasting(activePanelId)) {
      const sids = sendableSessionIds(tabStore.getAllBroadcastPanelIds())
      return [...new Set([panel.sessionId, ...sids])]
    }
    return [panel.sessionId]
  }

  return sendableSessionIds(tabPanelIds(activeTab))
}

/**
 * Build the payload for one target. Mode 'lines': every newline becomes
 * Enter (works on cmd/PowerShell where a bare \n is not Enter; final line
 * executes too). Mode 'bracketed': wrap the block in bracketed-paste
 * markers so the shell receives it literally without executing — only safe
 * when that target's terminal actually enabled bracketed paste, so targets
 * without it fall back to 'lines' for that write.
 */
export function buildSendPayload(text: string, mode: 'lines' | 'bracketed', bracketedPasteMode: boolean): string {
  const normalized = normalizePastedText(text)
  if (mode === 'bracketed' && bracketedPasteMode) {
    return `\x1b[200~${normalized}\x1b[201~`
  }
  return normalized.replace(/\n/g, '\r')
}

/** The bracketed-paste mode of a target session (false when unknown). */
export function targetBracketedPasteMode(sessionId: string): boolean {
  return getManagedTerminal(sessionId)?.terminal.modes.bracketedPasteMode ?? false
}
