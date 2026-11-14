import { describe, it, expect, vi, beforeEach } from 'vitest'

// vitest runs in node by default and the project has no jsdom dep;
// stub the surface the composable touches (window, Event) the same way
// useTerminalMenu.test.ts does.
if (typeof globalThis.window === 'undefined') {
  const dispatched: unknown[] = []
  const g = globalThis as any
  g.__dispatchedEvents = dispatched
  g.window = {
    dispatchEvent: vi.fn((e: unknown) => {
      dispatched.push(e)
      return true
    }),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }
  g.Event = class {
    type: string
    constructor(type: string) {
      this.type = type
    }
  }
}

const { mockSettingsStore } = vi.hoisted(() => ({
  mockSettingsStore: {
    settings: { terminal: { multilinePaste: 'confirm' } },
    save: vi.fn(),
  },
}))

vi.mock('../stores/settingsStore', () => ({
  useSettingsStore: () => mockSettingsStore,
}))

import { confirmMultilinePaste, finishMultilinePaste } from './multilinePasteDialog'

// Issue #1481: the multi-line paste dialog and context menus must be mutually
// exclusive — when the dialog opens, any open menu closes via the shared
// global:close-context-menus broadcast (Menu.vue listens for it), whatever
// paste path (menu item, right-click paste, middle-click, shortcut) opened it.
describe('confirmMultilinePaste closes context menus on open', () => {
  beforeEach(() => {
    mockSettingsStore.settings.terminal.multilinePaste = 'confirm'
    ;(globalThis as any).__dispatchedEvents.length = 0
  })

  it('broadcasts global:close-context-menus when the dialog opens', async () => {
    const pending = confirmMultilinePaste('line1\nline2')
    expect((globalThis as any).__dispatchedEvents.some((e: any) => e.type === 'global:close-context-menus')).toBe(true)
    finishMultilinePaste(null, false)
    await expect(pending).resolves.toBeNull()
  })

  it('does not broadcast for single-line pastes (dialog never opens)', async () => {
    const resolved = confirmMultilinePaste('single-line')
    expect((globalThis as any).__dispatchedEvents.some((e: any) => e.type === 'global:close-context-menus')).toBe(false)
    await expect(resolved).resolves.toBe('single-line')
  })
})
