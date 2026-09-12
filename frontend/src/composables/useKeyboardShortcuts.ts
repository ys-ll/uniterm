import type { KeyboardSettings, KeyBinding, ShortcutAction } from '../types/settings'

type ActionHandlers = Record<ShortcutAction, () => void>

/**
 * Render a KeyBinding as a human-readable combo, e.g. Ctrl+Shift+C.
 * Shared by the settings UI and the terminal context-menu shortcut hints so
 * both show the same format (Cmd on macOS, Meta elsewhere).
 */
export function formatKeyBinding(b: KeyBinding, isMac: boolean): string {
  if (!b) return ''
  const parts: string[] = []
  if (b.ctrl) parts.push('Ctrl')
  if (b.meta) parts.push(isMac ? 'Cmd' : 'Meta')
  if (b.shift) parts.push('Shift')
  if (b.alt) parts.push('Alt')
  parts.push(b.key)
  return parts.join('+')
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
  parts.push(e.key.toLowerCase())
  return parts.join('+')
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
) {
  shortcutMap.clear()
  terminalShortcutMap.clear()
  actionKeyMap.clear()
  for (const [action, b] of Object.entries(bindings) as [ShortcutAction, KeyBinding][]) {
    const key = bindingKey(b)
    if (!key) continue
    const handler = handlers[action]
    if (handler) {
      const target = TERMINAL_SCOPED_ACTIONS.includes(action) ? terminalShortcutMap : shortcutMap
      target.set(key, handler)
      if (!b.meta && b.ctrl) {
        target.set(key.replace(/^ctrl\+/, 'meta+'), handler)
      }
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

// ── Configurable digit tab switching (tabSwitchModifier) ──
//
// Digit shortcuts live in onPlatformSystemShortcut (App.vue): by default
// Ctrl/Cmd+1…9 switches tabs and Alt/Option+1…9 switches workspace panels.
// The optional keyboard.tabSwitchModifier setting moves the tab-switch combo
// to any modifier the user prefers (e.g. Alt). The helpers below are the
// pure decision logic, shared by the runtime handler and the tab/panel
// shortcut badges so the UI can never show a combo that doesn't work.

function flagsMatch(e: KeyboardEvent, mod: KeyBinding): boolean {
  return e.ctrlKey === !!mod.ctrl && e.metaKey === !!mod.meta
    && e.shiftKey === !!mod.shift && e.altKey === !!mod.alt
}

function hasAnyFlag(mod: KeyBinding | undefined): boolean {
  return !!mod && !!(mod.ctrl || mod.meta || mod.shift || mod.alt)
}

// True when the custom modifier claims the alt-only combo, i.e. the
// workspace-panel digit shortcuts would collide with tab switching and must
// be suppressed (runtime handler skips them, badges hide the hint).
export function panelDigitShortcutsSuppressed(mod?: KeyBinding): boolean {
  return !!mod && !!mod.alt && !mod.ctrl && !mod.meta && !mod.shift
}

/**
 * Resolve which target a digit keydown addresses, or null when it belongs to
 * nobody. `mod` is the configured tabSwitchModifier: undefined falls back to
 * the fixed platform bindings (Ctrl/Cmd tabs, Alt/Option panels); a binding
 * with no modifier set disables digit tab switching entirely; a configured
 * combo moves tab switching to it and steals the digits from the panels only
 * when it is the plain Alt/Option combo.
 */
export function matchDigitShortcut(
  e: KeyboardEvent,
  isMac: boolean,
  mod?: KeyBinding,
): 'tab' | 'panel' | null {
  const altOnly = e.altKey && !e.metaKey && !e.ctrlKey && !e.shiftKey
  if (altOnly && !panelDigitShortcutsSuppressed(mod)) return 'panel'
  if (mod === undefined) {
    if (isMac) return e.metaKey && !e.ctrlKey && !e.shiftKey ? 'tab' : null
    return e.ctrlKey && !e.metaKey && !e.shiftKey ? 'tab' : null
  }
  if (!hasAnyFlag(mod)) return null
  return flagsMatch(e, mod) ? 'tab' : null
}

/**
 * Badge prefix for the tab digit shortcut ('1'…'9' is appended by callers):
 * platform symbols for the default binding, plain modifier text for a
 * configured one. '' when digit tab switching is disabled.
 */
export function tabDigitShortcutPrefix(isMac: boolean, mod?: KeyBinding): string {
  if (mod === undefined) return isMac ? '⌘' : 'Ctrl'
  if (!hasAnyFlag(mod)) return ''
  if (isMac) {
    let s = ''
    if (mod.ctrl) s += '⌃'
    if (mod.meta) s += '⌘'
    if (mod.shift) s += '⇧'
    if (mod.alt) s += '⌥'
    return s
  }
  const parts: string[] = []
  if (mod.ctrl) parts.push('Ctrl')
  if (mod.meta) parts.push('Meta')
  if (mod.shift) parts.push('Shift')
  if (mod.alt) parts.push('Alt')
  return parts.join('+')
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
