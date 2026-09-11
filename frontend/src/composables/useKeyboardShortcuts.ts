import type { KeyboardSettings, KeyBinding, ShortcutAction } from '../types/settings'

type ActionHandlers = Record<ShortcutAction, () => void>

/**
 * Render a KeyBinding as a human-readable combo, e.g. Ctrl+Shift+C.
 * Shared by the settings UI and the terminal context-menu shortcut hints so
 * both show the same format. Explicit primary bindings resolve to Command on
 * macOS and Ctrl elsewhere; manually configured Ctrl/Meta modifiers stay exact.
 */
export function formatKeyBinding(b: KeyBinding, isMac: boolean): string {
  if (!b) return ''
  const resolved = resolveKeyBinding(b, isMac)
  const parts: string[] = []
  if (resolved.ctrl) parts.push('Ctrl')
  if (resolved.meta) parts.push(isMac ? 'Cmd' : 'Meta')
  if (resolved.shift) parts.push('Shift')
  if (resolved.alt) parts.push('Alt')
  parts.push(resolved.key)
  return parts.join('+')
}

export function resolveKeyBinding(b: KeyBinding, isMac: boolean): KeyBinding {
  if (!b.primary) return b
  return {
    ...b,
    primary: false,
    ctrl: !isMac,
    meta: isMac,
  }
}

export function migrateLegacyQuickCommandsBinding(
  binding: KeyBinding | undefined,
  isMac: boolean,
): KeyBinding | undefined {
  if (isMac || !binding?.meta || binding.primary !== undefined || binding.ctrl || binding.shift || binding.alt
    || binding.key.toLowerCase() !== 'k') {
    return binding
  }
  return { ctrl: false, meta: false, primary: true, shift: false, alt: false, key: 'k' }
}

export function migrateLegacyPrimaryBinding(
  binding: KeyBinding | undefined,
  defaultBinding: KeyBinding | undefined,
): KeyBinding | undefined {
  if (!binding || !defaultBinding?.primary || binding.primary !== undefined || !binding.ctrl || binding.meta
    || binding.shift !== defaultBinding.shift || binding.alt !== defaultBinding.alt
    || binding.key.toLowerCase() !== defaultBinding.key.toLowerCase()) {
    return binding
  }
  return { ...defaultBinding }
}

function bindingKey(b: KeyBinding): string {
  if (!b.key) return ''
  let k = ''
  if (b.ctrl) k += 'ctrl+'
  if (b.meta) k += 'meta+'
  if (b.shift) k += 'shift+'
  if (b.alt) k += 'alt+'
  k += b.key.toLowerCase()
  return k
}

function normalize(e: KeyboardEvent): string {
  const parts: string[] = []
  if (e.ctrlKey) parts.push('ctrl')
  if (e.metaKey) parts.push('meta')
  if (e.shiftKey) parts.push('shift')
  if (e.altKey) parts.push('alt')
  parts.push(bindingKeyFromEvent(e))
  return parts.join('+')
}

export function bindingKeyFromEvent(
  e: Pick<KeyboardEvent, 'key' | 'code'>,
): string {
  const digit = e.code.match(/^Digit([0-9])$/)
  return digit ? digit[1] : e.key.toLowerCase()
}

export type PlatformDigitShortcut =
  | { action: 'tab'; index: number }
  | { action: 'workspace'; index: number }

// Resolve only exact platform digit combinations. In particular, Alt+N and
// Ctrl+N are mutually exclusive on Windows and must never select the same UI.
export function resolvePlatformDigitShortcut(
  e: Pick<KeyboardEvent, 'code' | 'ctrlKey' | 'metaKey' | 'shiftKey' | 'altKey'>,
  isMac: boolean,
): PlatformDigitShortcut | null {
  const match = e.code.match(/^Digit([0-9])$/)
  if (!match || e.shiftKey) return null
  // Match the conventional terminal shortcut layout: 1…9 select the first
  // nine entries and 0 selects the tenth.
  const digit = Number(match[1])
  const index = digit === 0 ? 9 : digit - 1
  if (e.altKey && !e.metaKey && !e.ctrlKey) {
    return { action: 'workspace', index }
  }
  const tabModifier = !e.altKey && (
    (isMac && e.metaKey && !e.ctrlKey) ||
    (!isMac && e.ctrlKey && !e.metaKey)
  )
  return tabModifier ? { action: 'tab', index } : null
}

// Digit navigation is fixed and takes precedence over configurable actions.
// Modified variants such as Ctrl+Shift+1 remain available to users.
export function isReservedDigitBinding(b: KeyBinding, isMac: boolean): boolean {
  const resolved = resolveKeyBinding(b, isMac)
  if (!/^[0-9]$/.test(resolved.key) || resolved.shift) return false
  if (resolved.alt && !resolved.ctrl && !resolved.meta) return true
  return isMac
    ? !!resolved.meta && !resolved.ctrl && !resolved.alt
    : !!resolved.ctrl && !resolved.meta && !resolved.alt
}

export function effectiveBindingKey(b: KeyBinding, isMac: boolean): string {
  return bindingKey(resolveKeyBinding(b, isMac))
}

// Module-level state: key combo → action handler
const shortcutMap = new Map<string, () => void>()
// Terminal-scoped shortcuts: only fire while a terminal session is focused
// (handled by onTerminalKey), never from the global capture listener, so they
// don't hijack copy/paste while the user is typing in another input.
const terminalShortcutMap = new Map<string, () => void>()
// Reverse lookup: action → key combo (for display / dedup)
const actionKeyMap = new Map<ShortcutAction, string>()

// Actions that should only take effect while a terminal session is focused.
const TERMINAL_SCOPED_ACTIONS: ShortcutAction[] = ['copy', 'paste']

export function loadKeybindings(
  bindings: KeyboardSettings,
  handlers: ActionHandlers,
  isMac = false,
) {
  shortcutMap.clear()
  terminalShortcutMap.clear()
  actionKeyMap.clear()
  for (const [action, b] of Object.entries(bindings) as [ShortcutAction, KeyBinding][]) {
    if (isReservedDigitBinding(b, isMac)) continue
    const key = effectiveBindingKey(b, isMac)
    if (!key) continue
    const handler = handlers[action]
    if (handler) {
      const target = TERMINAL_SCOPED_ACTIONS.includes(action) ? terminalShortcutMap : shortcutMap
      target.set(key, handler)
      actionKeyMap.set(action, key)
    }
  }
}

export function getActionKey(action: ShortcutAction): string {
  return actionKeyMap.get(action) || ''
}

function fire(e: KeyboardEvent, normalized: string, map: Map<string, () => void>): boolean {
  const handler = map.get(normalized)
  if (!handler) return false
  e.preventDefault()
  e.stopPropagation()
  handler()
  return true
}

export function onGlobalKeydown(e: KeyboardEvent) {
  fire(e, normalize(e), shortcutMap)
}

export function onTerminalKey(e: KeyboardEvent): boolean {
  const normalized = normalize(e)
  if (fire(e, normalized, shortcutMap)) return false
  if (fire(e, normalized, terminalShortcutMap)) return false
  return true
}

let registered = false

export function installGlobalListener() {
  if (registered) return
  registered = true
  window.addEventListener('keydown', onGlobalKeydown, true)
}

export function uninstallGlobalListener() {
  registered = false
  window.removeEventListener('keydown', onGlobalKeydown, true)
}
