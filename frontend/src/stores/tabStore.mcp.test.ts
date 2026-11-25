import { afterEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useTabStore } from './tabStore'
import { usePanelStore } from './panelStore'

describe('background MCP tabs', () => {
  afterEach(() => {
    const store = useTabStore()
    for (const tab of store.mcpTabs) store.removeMcpTab(tab.id)
    for (const tab of [...store.tabs]) store.closeTab(tab.id)
  })

  it('keeps multiple new sessions out of the normal tab bar and leaves focus alone', () => {
    setActivePinia(createPinia())
    const store = useTabStore()
    const home = store.createStartTab()
    const first = store.createMcpTab('Fixture A', 'mcp-panel-a')
    const second = store.createMcpTab('Fixture B', 'mcp-panel-b')

    expect(store.tabs).toEqual([home])
    expect(store.activeTab).toEqual(home)
    expect(store.mcpTabs).toEqual([first, second])

    // The ordinary tab close path can only see the normal tab list.
    expect(store.closeTab(first.id)).toEqual([])
    expect(store.mcpTabs).toEqual([first, second])
  })

  it('restores the previous tab and reopens the same panel and session', () => {
    setActivePinia(createPinia())
    const store = useTabStore()
    const panels = usePanelStore()
    const home = store.createStartTab()
    const panel = panels.createPanel(null)
    panel.sessionId = 'mcp-session-a'
    const first = store.createMcpTab('Fixture A', panel.id)
    const second = store.createMcpTab('Fixture B', 'mcp-panel-b')

    store.showMcpTab(first.id)
    expect(store.getActivePanelId()).toBe(panel.id)
    store.showMcpTab(second.id)
    expect(store.hideMcpTab(second.id)).toBe(true)
    expect(store.activeTab).toEqual(home)

    store.showMcpTab(first.id)
    expect(store.activeTab).toEqual(first)
    expect(panels.getPanel(store.getActivePanelId()!)?.sessionId).toBe('mcp-session-a')
    expect(store.removeMcpTab(first.id)).toEqual(first)
    expect(store.activeTab).toEqual(home)
    expect(store.mcpTabs).toEqual([second])
  })
})
