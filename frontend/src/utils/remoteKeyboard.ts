/** App shortcuts yield to the focused remote desktop, including when
 * ordinary DOM events provide the keyboard fallback. */
export function remoteDesktopOwnsKey(event: KeyboardEvent): boolean {
  const target = event.target as Element | null
  return !!target?.closest?.('[data-remote-keyboard="true"]')
}

/** Menus opened without moving DOM focus still need local keyboard control. */
export function releaseRemoteKeyboardFocus(): void {
  const active = document.activeElement as HTMLElement | null
  if (active?.closest?.('[data-remote-keyboard="true"]')) active.blur()
}

// Panels share one hook. A delayed blur must not disable a newly focused panel.
let captureChange: Promise<unknown> = Promise.resolve()
export function queueRemoteKeyboardChange(change: () => Promise<unknown>): Promise<unknown> {
  const next = captureChange.then(change)
  captureChange = next.catch(() => {})
  return next
}

export type NativeRdpKey = {
  captureID: string
  scancode: number
  down: boolean
  repeat: boolean
  ctrl: boolean
  shift: boolean
  meta: boolean
  reset: boolean
  virtualKey?: number
}

export function isRdpPasteKey(scancode: number, ctrl: boolean, meta: boolean, shift: boolean): boolean {
  return ((ctrl || meta) && scancode === 0x2f) || (shift && scancode === 0xe052)
}

// KeyboardEvent.code → XT set-1 make code. Extended keys carry the 0xE0
// prefix in the high byte (Scancode::from_u16 treats 0xE000 as the extended
// flag).
export const REMOTE_KEY_SCANCODES: Record<string, number> = {
  Escape: 0x01, Digit1: 0x02, Digit2: 0x03, Digit3: 0x04, Digit4: 0x05, Digit5: 0x06,
  Digit6: 0x07, Digit7: 0x08, Digit8: 0x09, Digit9: 0x0a, Digit0: 0x0b,
  Minus: 0x0c, Equal: 0x0d, Backspace: 0x0e, Tab: 0x0f,
  KeyQ: 0x10, KeyW: 0x11, KeyE: 0x12, KeyR: 0x13, KeyT: 0x14, KeyY: 0x15,
  KeyU: 0x16, KeyI: 0x17, KeyO: 0x18, KeyP: 0x19,
  BracketLeft: 0x1a, BracketRight: 0x1b, Enter: 0x1c, ControlLeft: 0x1d,
  KeyA: 0x1e, KeyS: 0x1f, KeyD: 0x20, KeyF: 0x21, KeyG: 0x22, KeyH: 0x23,
  KeyJ: 0x24, KeyK: 0x25, KeyL: 0x26, Semicolon: 0x27, Quote: 0x28,
  Backquote: 0x29, ShiftLeft: 0x2a, Backslash: 0x2b,
  KeyZ: 0x2c, KeyX: 0x2d, KeyC: 0x2e, KeyV: 0x2f, KeyB: 0x30, KeyN: 0x31, KeyM: 0x32,
  Comma: 0x33, Period: 0x34, Slash: 0x35, ShiftRight: 0x36,
  NumpadMultiply: 0x37, AltLeft: 0x38, Space: 0x39, CapsLock: 0x3a,
  F1: 0x3b, F2: 0x3c, F3: 0x3d, F4: 0x3e, F5: 0x3f, F6: 0x40, F7: 0x41, F8: 0x42,
  F9: 0x43, F10: 0x44, NumLock: 0x45, ScrollLock: 0x46,
  Numpad7: 0x47, Numpad8: 0x48, Numpad9: 0x49, NumpadSubtract: 0x4a,
  Numpad4: 0x4b, Numpad5: 0x4c, Numpad6: 0x4d, NumpadAdd: 0x4e,
  Numpad1: 0x4f, Numpad2: 0x50, Numpad3: 0x51, Numpad0: 0x52, NumpadDecimal: 0x53,
  F11: 0x57, F12: 0x58, F13: 0x64, F14: 0x65, F15: 0x66,
  IntlBackslash: 0x56,
  // Extended (0xE0-prefixed) keys
  AltRight: 0xe038, ControlRight: 0xe01d,
  ArrowUp: 0xe048, ArrowLeft: 0xe04b, ArrowRight: 0xe04d, ArrowDown: 0xe050,
  NumpadEnter: 0xe01c, NumpadDivide: 0xe035,
  Home: 0xe047, End: 0xe04f, Delete: 0xe053, PageUp: 0xe049, PageDown: 0xe051,
  Insert: 0xe052, MetaLeft: 0xe05b, MetaRight: 0xe05c, ContextMenu: 0xe05d,
  PrintScreen: 0xe037,
}

// Combos the local OS reserves before they can reach the webview
// (Ctrl+Alt+Del, Win+L) plus a few conveniences, sent as one press-then-
// release sequence — the same scancodes mstsc sends. Scancode-based, shared
// by the RDP and SPICE send paths.
export const REMOTE_KEY_COMBOS: Record<string, number[]> = {
  cad: [REMOTE_KEY_SCANCODES.ControlLeft, REMOTE_KEY_SCANCODES.AltLeft, REMOTE_KEY_SCANCODES.Delete],
  ctrlShiftEsc: [REMOTE_KEY_SCANCODES.ControlLeft, REMOTE_KEY_SCANCODES.ShiftLeft, REMOTE_KEY_SCANCODES.Escape],
  win: [REMOTE_KEY_SCANCODES.MetaLeft],
  winL: [REMOTE_KEY_SCANCODES.MetaLeft, REMOTE_KEY_SCANCODES.KeyL],
  altTab: [REMOTE_KEY_SCANCODES.AltLeft, REMOTE_KEY_SCANCODES.Tab],
  altF4: [REMOTE_KEY_SCANCODES.AltLeft, REMOTE_KEY_SCANCODES.F4],
}
