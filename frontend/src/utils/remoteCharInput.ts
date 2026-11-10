import { REMOTE_KEY_SCANCODES } from './remoteKeyboard'

// Character → (scancode, shift) mapping for soft-keyboard input on mobile
// remote desktop sessions (RDP / SPICE forward set-1 scancodes; VNC takes
// keysyms). US layout; characters outside it (dead keys, CJK) return null —
// scancode protocols have no way to type them.

type CharKey = { scancode: number; shift: boolean }

// US-layout punctuation: char → [KeyboardEvent.code, shifted]
const PUNCTUATION: Record<string, [string, boolean]> = {
  '-': ['Minus', false], '_': ['Minus', true],
  '=': ['Equal', false], '+': ['Equal', true],
  '[': ['BracketLeft', false], '{': ['BracketLeft', true],
  ']': ['BracketRight', false], '}': ['BracketRight', true],
  '\\': ['Backslash', false], '|': ['Backslash', true],
  ';': ['Semicolon', false], ':': ['Semicolon', true],
  "'": ['Quote', false], '"': ['Quote', true],
  '`': ['Backquote', false], '~': ['Backquote', true],
  ',': ['Comma', false], '<': ['Comma', true],
  '.': ['Period', false], '>': ['Period', true],
  '/': ['Slash', false], '?': ['Slash', true],
}

const WHITESPACE: Record<string, [string, boolean]> = {
  ' ': ['Space', false],
  '\n': ['Enter', false],
  '\r': ['Enter', false],
  '\t': ['Tab', false],
}

export function charToScancode(char: string): CharKey | null {
  if (char.length !== 1) return null
  const lower = char.toLowerCase()
  if (lower !== char.toUpperCase() && /^[a-z]$/.test(lower)) {
    const scancode = REMOTE_KEY_SCANCODES[`Key${lower.toUpperCase()}`]
    return scancode !== undefined ? { scancode, shift: char !== lower } : null
  }
  if (/^[0-9]$/.test(char)) {
    const scancode = REMOTE_KEY_SCANCODES[`Digit${char}`]
    return scancode !== undefined ? { scancode, shift: false } : null
  }
  const mapped = WHITESPACE[char] ?? PUNCTUATION[char]
  if (!mapped) return null
  const scancode = REMOTE_KEY_SCANCODES[mapped[0]]
  return scancode !== undefined ? { scancode, shift: mapped[1] } : null
}

// keysym constants used by the VNC path
const KEYSYM_SPACE = 0x20
const KEYSYM_ENTER = 0xff0d
const KEYSYM_TAB = 0xff09
const KEYSYM_BACKSPACE = 0xff08
const UNICODE_KEYSYM_BASE = 0x1000000

export function charToKeysym(char: string): number | null {
  if (char.length !== 1) return null
  switch (char) {
    case ' ': return KEYSYM_SPACE
    case '\n': case '\r': return KEYSYM_ENTER
    case '\t': return KEYSYM_TAB
    case '\b': return KEYSYM_BACKSPACE
  }
  const cp = char.codePointAt(0)!
  // Latin-1 has direct keysyms; anything above goes through the Unicode
  // keysym range (0x01000000 + codepoint), which X11 servers commonly accept.
  return cp <= 0xff ? cp : UNICODE_KEYSYM_BASE + cp
}

// KeyboardEvent.code → keysym for the special (non-printable) keys a soft or
// hardware keyboard reports through keydown, for the VNC path. Printable
// characters go through charToKeysym instead.
export const CODE_KEYSYMS: Record<string, number> = {
  Enter: KEYSYM_ENTER, Backspace: KEYSYM_BACKSPACE, Tab: KEYSYM_TAB, Escape: 0xff1b,
  ArrowUp: 0xff52, ArrowDown: 0xff54, ArrowLeft: 0xff51, ArrowRight: 0xff53,
  Home: 0xff50, End: 0xff57, Insert: 0xff63, Delete: 0xffff,
  PageUp: 0xff55, PageDown: 0xff56,
  ControlLeft: 0xffe3, ControlRight: 0xffe4, ShiftLeft: 0xffe1, ShiftRight: 0xffe2,
  AltLeft: 0xffe9, AltRight: 0xffea, MetaLeft: 0xffeb, MetaRight: 0xffec,
  CapsLock: 0xffe5, NumLock: 0xff7f,
  ContextMenu: 0xff67, PrintScreen: 0xff61, ScrollLock: 0xff14, Pause: 0xff13,
}
