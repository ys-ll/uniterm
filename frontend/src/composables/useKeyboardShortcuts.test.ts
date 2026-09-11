import { describe, it, expect, vi } from 'vitest'
import { bindingKeyFromEvent, effectiveBindingKey, formatKeyBinding, isReservedDigitBinding, loadKeybindings, migrateLegacyPrimaryBinding, migrateLegacyQuickCommandsBinding, onGlobalKeydown, resolvePlatformDigitShortcut } from './useKeyboardShortcuts'
import { DEFAULT_KEYBOARD, type KeyboardSettings } from '../types/settings'

function fakeKey(partial: Partial<{ ctrlKey: boolean; metaKey: boolean; shiftKey: boolean; altKey: boolean; key: string; code: string; isComposing: boolean; keyCode: number }>): KeyboardEvent {
  return {
    ctrlKey: false, metaKey: false, shiftKey: false, altKey: false, key: '', code: '',
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

  it('resolves the primary modifier to Command on macOS', () => {
    const zoomFontIn = vi.fn()
    loadKeybindings(
      { zoomFontIn: DEFAULT_KEYBOARD.zoomFontIn! } as Particle<KeyboardSettings>,
      { zoomFontIn } as any,
      true,
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
  it('uses Ctrl+K on Windows and mirrors it to Command+K on macOS', () => {
    const openQuickCommands = vi.fn()
    loadKeybindings(
      { openQuickCommands: DEFAULT_KEYBOARD.openQuickCommands! },
      { openQuickCommands } as any,
    )

    onGlobalKeydown(fakeKey({ ctrlKey: true, key: 'k' }))
    onGlobalKeydown(fakeKey({ metaKey: true, key: 'k' }))

    expect(openQuickCommands).toHaveBeenCalledTimes(1)
    expect(formatKeyBinding(DEFAULT_KEYBOARD.openQuickCommands!, false)).toBe('Ctrl+k')
    expect(formatKeyBinding(DEFAULT_KEYBOARD.openQuickCommands!, true)).toBe('Cmd+k')
  })

  it('uses Command+K instead of Ctrl+K when loaded for macOS', () => {
    const openQuickCommands = vi.fn()
    loadKeybindings(
      { openQuickCommands: DEFAULT_KEYBOARD.openQuickCommands! },
      { openQuickCommands } as any,
      true,
    )

    onGlobalKeydown(fakeKey({ ctrlKey: true, key: 'k' }))
    onGlobalKeydown(fakeKey({ metaKey: true, key: 'k' }))
    expect(openQuickCommands).toHaveBeenCalledTimes(1)
  })

  it('migrates the legacy Meta+K default to Ctrl+K on Windows', () => {
    const legacy = { ctrl: false, meta: true, shift: false, alt: false, key: 'k' }
    expect(migrateLegacyQuickCommandsBinding(legacy, false))
      .toEqual({ ctrl: false, meta: false, primary: true, shift: false, alt: false, key: 'k' })
    expect(migrateLegacyQuickCommandsBinding(legacy, true)).toBe(legacy)
  })

  it('preserves an explicitly configured Meta+K binding on Windows', () => {
    const explicit = { ctrl: false, meta: true, primary: false, shift: false, alt: false, key: 'k' }
    expect(migrateLegacyQuickCommandsBinding(explicit, false)).toBe(explicit)
  })

  it('preserves an explicit macOS Control binding', () => {
    const openQuickCommands = vi.fn()
    const explicitControl = { ctrl: true, meta: false, primary: false, shift: false, alt: false, key: 'm' }
    loadKeybindings({ openQuickCommands: explicitControl }, { openQuickCommands } as any, true)

    onGlobalKeydown(fakeKey({ metaKey: true, key: 'm' }))
    onGlobalKeydown(fakeKey({ ctrlKey: true, key: 'm' }))

    expect(openQuickCommands).toHaveBeenCalledTimes(1)
    expect(formatKeyBinding(explicitControl, true)).toBe('Ctrl+m')
  })

  it('migrates only bindings that exactly match an old primary default', () => {
    const oldDefault = { ctrl: true, meta: false, shift: true, alt: false, key: 'n' }
    expect(migrateLegacyPrimaryBinding(oldDefault, DEFAULT_KEYBOARD.newConnection!))
      .toEqual(DEFAULT_KEYBOARD.newConnection)
    const customized = { ...oldDefault, key: 'm' }
    expect(migrateLegacyPrimaryBinding(customized, DEFAULT_KEYBOARD.newConnection!)).toBe(customized)
    const explicitControl = { ...oldDefault, primary: false }
    expect(migrateLegacyPrimaryBinding(explicitControl, DEFAULT_KEYBOARD.newConnection!)).toBe(explicitControl)
  })

  it('uses the same effective key for primary and explicit Command bindings', () => {
    expect(effectiveBindingKey(DEFAULT_KEYBOARD.openQuickCommands!, true)).toBe('meta+k')
    expect(effectiveBindingKey(
      { ctrl: false, meta: true, primary: false, shift: false, alt: false, key: 'k' },
      true,
    )).toBe('meta+k')
  })
})

describe('useKeyboardShortcuts — workspace maximize', () => {
  it('uses Ctrl+Shift+Enter on Windows', () => {
    const toggleWorkspaceMaximize = vi.fn()
    loadKeybindings(
      { toggleWorkspaceMaximize: DEFAULT_KEYBOARD.toggleWorkspaceMaximize! },
      { toggleWorkspaceMaximize } as any,
    )

    onGlobalKeydown(fakeKey({ ctrlKey: true, shiftKey: true, key: 'Enter' }))
    onGlobalKeydown(fakeKey({ metaKey: true, shiftKey: true, key: 'Enter' }))

    expect(toggleWorkspaceMaximize).toHaveBeenCalledTimes(1)
    expect(formatKeyBinding(DEFAULT_KEYBOARD.toggleWorkspaceMaximize!, false))
      .toBe('Ctrl+Shift+enter')
  })

  it('uses Command+Shift+Enter on macOS', () => {
    const toggleWorkspaceMaximize = vi.fn()
    loadKeybindings(
      { toggleWorkspaceMaximize: DEFAULT_KEYBOARD.toggleWorkspaceMaximize! },
      { toggleWorkspaceMaximize } as any,
      true,
    )

    onGlobalKeydown(fakeKey({ ctrlKey: true, shiftKey: true, key: 'Enter' }))
    onGlobalKeydown(fakeKey({ metaKey: true, shiftKey: true, key: 'Enter' }))

    expect(toggleWorkspaceMaximize).toHaveBeenCalledTimes(1)
    expect(formatKeyBinding(DEFAULT_KEYBOARD.toggleWorkspaceMaximize!, true))
      .toBe('Cmd+Shift+enter')
  })

  it('honours a user-defined maximize binding', () => {
    const toggleWorkspaceMaximize = vi.fn()
    loadKeybindings(
      { toggleWorkspaceMaximize: { ctrl: false, meta: false, shift: true, alt: true, key: 'm' } },
      { toggleWorkspaceMaximize } as any,
    )

    onGlobalKeydown(fakeKey({ ctrlKey: true, shiftKey: true, key: 'Enter' }))
    onGlobalKeydown(fakeKey({ altKey: true, shiftKey: true, key: 'm' }))

    expect(toggleWorkspaceMaximize).toHaveBeenCalledTimes(1)
  })
})

describe('useKeyboardShortcuts — platform digit shortcuts', () => {
  const digit = (modifiers: Partial<KeyboardEvent>) => ({
    code: 'Digit3', ctrlKey: false, metaKey: false, shiftKey: false, altKey: false,
    ...modifiers,
  } as KeyboardEvent)

  it('keeps Windows Alt+N and Ctrl+N on separate actions', () => {
    expect(resolvePlatformDigitShortcut(digit({ altKey: true }), false))
      .toEqual({ action: 'workspace', index: 2 })
    expect(resolvePlatformDigitShortcut(digit({ ctrlKey: true }), false))
      .toEqual({ action: 'tab', index: 2 })
  })

  it('rejects mixed Alt+Ctrl and maps macOS Command+N to tabs', () => {
    expect(resolvePlatformDigitShortcut(digit({ altKey: true, ctrlKey: true }), false)).toBeNull()
    expect(resolvePlatformDigitShortcut(digit({ metaKey: true }), true))
      .toEqual({ action: 'tab', index: 2 })
  })

  it('maps 0 to the tenth tab and workspace panel', () => {
    const zero = (modifiers: Partial<KeyboardEvent>) => ({
      code: 'Digit0', ctrlKey: false, metaKey: false, shiftKey: false, altKey: false,
      ...modifiers,
    } as KeyboardEvent)

    expect(resolvePlatformDigitShortcut(zero({ ctrlKey: true }), false))
      .toEqual({ action: 'tab', index: 9 })
    expect(resolvePlatformDigitShortcut(zero({ altKey: true }), false))
      .toEqual({ action: 'workspace', index: 9 })
    expect(resolvePlatformDigitShortcut(zero({ metaKey: true }), true))
      .toEqual({ action: 'tab', index: 9 })
    expect(resolvePlatformDigitShortcut(zero({ altKey: true }), true))
      .toEqual({ action: 'workspace', index: 9 })
  })

  it('recognizes fixed digit combinations as reserved', () => {
    const binding = (overrides: Partial<NonNullable<KeyboardSettings['openQuickCommands']>>) => ({
      ctrl: false, meta: false, shift: false, alt: false, key: '1', ...overrides,
    })

    expect(isReservedDigitBinding(binding({ ctrl: true }), false)).toBe(true)
    expect(isReservedDigitBinding(binding({ alt: true }), false)).toBe(true)
    expect(isReservedDigitBinding(binding({ meta: true }), true)).toBe(true)
    expect(isReservedDigitBinding(binding({ alt: true }), true)).toBe(true)
    expect(isReservedDigitBinding(binding({ primary: true }), true)).toBe(true)
    expect(isReservedDigitBinding(binding({ primary: true }), false)).toBe(true)
    expect(isReservedDigitBinding(binding({ ctrl: true }), true)).toBe(false)
    expect(isReservedDigitBinding(binding({ ctrl: true, shift: true }), false)).toBe(false)
  })

  it('uses the physical digit for Option-modified keyboard layouts', () => {
    expect(bindingKeyFromEvent({ key: '¡', code: 'Digit1' } as KeyboardEvent)).toBe('1')
    expect(bindingKeyFromEvent({ key: 'K', code: 'KeyK' } as KeyboardEvent)).toBe('k')
  })

  it('does not register configurable actions that claim reserved digits', () => {
    const openQuickCommands = vi.fn()
    loadKeybindings(
      { openQuickCommands: { ctrl: true, meta: false, shift: false, alt: false, key: '1' } },
      { openQuickCommands } as any,
    )

    onGlobalKeydown(fakeKey({ ctrlKey: true, key: '1' }))
    expect(openQuickCommands).not.toHaveBeenCalled()
  })

  it('matches allowed Shift+digit bindings by physical digit code', () => {
    const openQuickCommands = vi.fn()
    loadKeybindings(
      { openQuickCommands: { ctrl: true, meta: false, shift: true, alt: false, key: '1' } },
      { openQuickCommands } as any,
    )

    onGlobalKeydown(fakeKey({ ctrlKey: true, shiftKey: true, key: '!', code: 'Digit1' }))
    expect(openQuickCommands).toHaveBeenCalledTimes(1)
  })
})
