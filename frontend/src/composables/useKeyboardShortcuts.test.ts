import { describe, it, expect, vi } from 'vitest'
import { loadKeybindings, onGlobalKeydown, matchDigitShortcut, panelDigitShortcutsSuppressed, tabDigitShortcutPrefix } from './useKeyboardShortcuts'
import type { KeyboardSettings } from '../types/settings'

function fakeKey(partial: Partial<{ ctrlKey: boolean; metaKey: boolean; shiftKey: boolean; altKey: boolean; key: string; isComposing: boolean; keyCode: number }>): KeyboardEvent {
  return {
    ctrlKey: false, metaKey: false, shiftKey: false, altKey: false, key: '',
    isComposing: false, keyCode: 0,
    preventDefault() {}, stopPropagation() {},
    ...partial,
  } as any
}

describe('useKeyboardShortcuts — font zoom bindings', () => {
  it('fires the handler on Ctrl+= (zoom in)', () => {
    const zoomFontIn = vi.fn()
    loadKeybindings(
      { zoomFontIn: { ctrl: true, shift: false, alt: false, key: '=' } } as Particle<KeyboardSettings>,
      { zoomFontIn } as any,
    )
    onGlobalKeydown(fakeKey({ ctrlKey: true, key: '=' }))
    expect(zoomFontIn).toHaveBeenCalledTimes(1)
  })

  it('mirrors Ctrl+= to Meta+= (issue #614 ⌘ zoom)', () => {
    const zoomFontIn = vi.fn()
    loadKeybindings(
      { zoomFontIn: { ctrl: true, shift: false, alt: false, key: '=' } } as Particle<KeyboardSettings>,
      { zoomFontIn } as any,
    )
    onGlobalKeydown(fakeKey({ metaKey: true, key: '=' }))
    expect(zoomFontIn).toHaveBeenCalledTimes(1)
  })

  it('fires the handler on Ctrl+- (zoom out) but not on the raw "=" key alone', () => {
    const zoomFontOut = vi.fn()
    loadKeybindings(
      { zoomFontOut: { ctrl: true, shift: false, alt: false, key: '-' } } as Particle<KeyboardSettings>,
      { zoomFontOut } as any,
    )
    onGlobalKeydown(fakeKey({ ctrlKey: true, key: '-' }))
    expect(zoomFontOut).toHaveBeenCalledTimes(1)
    onGlobalKeydown(fakeKey({ key: '=' }))
    expect(zoomFontOut).toHaveBeenCalledTimes(1) // no modifier → unharmed
  })

  it('does not fire when the active modifiers do not match the binding', () => {
    const zoomFontIn = vi.fn()
    loadKeybindings(
      { zoomFontIn: { ctrl: true, shift: false, alt: false, key: '=' } } as Particle<KeyboardSettings>,
      { zoomFontIn } as any,
    )
    onGlobalKeydown(fakeKey({ ctrlKey: true, shiftKey: true, key: '=' }))
    expect(zoomFontIn).not.toHaveBeenCalled()
  })
})

type Particle<T> = Partial<{ [K in keyof T]?: T[K] }>

describe('useKeyboardShortcuts — quick commands', () => {
  it('fires the quick-command handler on Meta+K only', () => {
    const openQuickCommands = vi.fn()
    loadKeybindings(
      { openQuickCommands: { ctrl: false, meta: true, shift: false, alt: false, key: 'k' } },
      { openQuickCommands } as any,
    )

    onGlobalKeydown(fakeKey({ metaKey: true, key: 'k' }))
    onGlobalKeydown(fakeKey({ ctrlKey: true, key: 'k' }))

    expect(openQuickCommands).toHaveBeenCalledTimes(1)
  })
})

describe('matchDigitShortcut — configurable tab-switch modifier', () => {
  const alt = { ctrl: false, meta: false, shift: false, alt: true, key: '' }
  const ctrlAlt = { ctrl: true, meta: false, shift: false, alt: true, key: '' }
  const none = { ctrl: false, meta: false, shift: false, alt: false, key: '' }

  it('keeps the fixed platform bindings when unset', () => {
    expect(matchDigitShortcut(fakeKey({ ctrlKey: true, key: '1' }), false, undefined)).toBe('tab')
    expect(matchDigitShortcut(fakeKey({ metaKey: true, key: '1' }), true, undefined)).toBe('tab')
    expect(matchDigitShortcut(fakeKey({ altKey: true, key: '2' }), false, undefined)).toBe('panel')
    expect(matchDigitShortcut(fakeKey({ altKey: true, key: '2' }), true, undefined)).toBe('panel')
  })

  it('moves tab switching to the configured modifier (Alt)', () => {
    expect(matchDigitShortcut(fakeKey({ altKey: true, key: '3' }), false, alt)).toBe('tab')
    expect(matchDigitShortcut(fakeKey({ ctrlKey: true, key: '3' }), false, alt)).toBe(null)
  })

  it('suppresses workspace panels only for the plain Alt combo', () => {
    expect(panelDigitShortcutsSuppressed(alt)).toBe(true)
    expect(panelDigitShortcutsSuppressed(undefined)).toBe(false)
    expect(panelDigitShortcutsSuppressed(none)).toBe(false)
    // Ctrl+Alt+digits for tabs leaves plain Alt+digits to the panels.
    expect(panelDigitShortcutsSuppressed(ctrlAlt)).toBe(false)
    expect(matchDigitShortcut(fakeKey({ altKey: true, key: '2' }), false, ctrlAlt)).toBe('panel')
    expect(matchDigitShortcut(fakeKey({ ctrlKey: true, altKey: true, key: '2' }), false, ctrlAlt)).toBe('tab')
  })

  it('a cleared modifier disables digit tab switching entirely', () => {
    expect(matchDigitShortcut(fakeKey({ ctrlKey: true, key: '1' }), false, none)).toBe(null)
    expect(matchDigitShortcut(fakeKey({ altKey: true, key: '1' }), false, none)).toBe('panel')
  })

  it('requires the modifiers to match exactly', () => {
    expect(matchDigitShortcut(fakeKey({ altKey: true, shiftKey: true, key: '1' }), false, alt)).toBe(null)
    expect(matchDigitShortcut(fakeKey({ altKey: true, ctrlKey: true, key: '1' }), false, alt)).toBe(null)
  })

  it('badges follow the configured modifier and hide suppressed panels', () => {
    expect(tabDigitShortcutPrefix(false, undefined)).toBe('Ctrl')
    expect(tabDigitShortcutPrefix(true, undefined)).toBe('⌘')
    expect(tabDigitShortcutPrefix(false, alt)).toBe('Alt')
    expect(tabDigitShortcutPrefix(true, alt)).toBe('⌥')
    expect(tabDigitShortcutPrefix(false, ctrlAlt)).toBe('Ctrl+Alt')
    expect(tabDigitShortcutPrefix(false, none)).toBe('')
  })
})
