import { describe, it, expect } from 'vitest'
import { charToScancode, charToKeysym, CODE_KEYSYMS } from './remoteCharInput'

describe('charToScancode', () => {
  it('maps lowercase letters without shift', () => {
    expect(charToScancode('a')).toEqual({ scancode: 0x1e, shift: false })
    expect(charToScancode('z')).toEqual({ scancode: 0x2c, shift: false })
  })

  it('maps uppercase letters with shift', () => {
    expect(charToScancode('A')).toEqual({ scancode: 0x1e, shift: true })
    expect(charToScancode('Z')).toEqual({ scancode: 0x2c, shift: true })
  })

  it('maps digits without shift', () => {
    expect(charToScancode('0')).toEqual({ scancode: 0x0b, shift: false })
    expect(charToScancode('7')).toEqual({ scancode: 0x08, shift: false })
  })

  it('maps whitespace characters', () => {
    expect(charToScancode(' ')).toEqual({ scancode: 0x39, shift: false })
    expect(charToScancode('\n')).toEqual({ scancode: 0x1c, shift: false })
    expect(charToScancode('\t')).toEqual({ scancode: 0x0f, shift: false })
  })

  it('maps unshifted punctuation', () => {
    expect(charToScancode('-')).toEqual({ scancode: 0x0c, shift: false })
    expect(charToScancode(';')).toEqual({ scancode: 0x27, shift: false })
    expect(charToScancode('/')).toEqual({ scancode: 0x35, shift: false })
  })

  it('maps shifted punctuation with the shift flag', () => {
    expect(charToScancode('_')).toEqual({ scancode: 0x0c, shift: true })
    expect(charToScancode(':')).toEqual({ scancode: 0x27, shift: true })
    expect(charToScancode('?')).toEqual({ scancode: 0x35, shift: true })
    expect(charToScancode('~')).toEqual({ scancode: 0x29, shift: true })
  })

  it('returns null for characters with no scancode representation', () => {
    expect(charToScancode('中')).toBeNull()
    expect(charToScancode('é')).toBeNull()
    expect(charToScancode('')).toBeNull()
  })
})

describe('charToKeysym', () => {
  it('maps control keys to their keysyms', () => {
    expect(charToKeysym('\n')).toBe(0xff0d)
    expect(charToKeysym('\t')).toBe(0xff09)
    expect(charToKeysym(' ')).toBe(0x20)
  })

  it('maps Latin-1 characters to their code points', () => {
    expect(charToKeysym('a')).toBe(0x61)
    expect(charToKeysym('A')).toBe(0x41)
    expect(charToKeysym(';')).toBe(0x3b)
  })

  it('maps non-Latin characters into the Unicode keysym range', () => {
    const cp = '中'.codePointAt(0)!
    expect(charToKeysym('中')).toBe(0x1000000 + cp)
  })

  it('returns null for empty input', () => {
    expect(charToKeysym('')).toBeNull()
  })
})

describe('CODE_KEYSYMS', () => {
  it('covers the special keys a soft keyboard can report', () => {
    expect(CODE_KEYSYMS['Enter']).toBe(0xff0d)
    expect(CODE_KEYSYMS['Backspace']).toBe(0xff08)
    expect(CODE_KEYSYMS['Escape']).toBe(0xff1b)
    expect(CODE_KEYSYMS['ArrowUp']).toBe(0xff52)
    expect(CODE_KEYSYMS['Delete']).toBe(0xffff)
  })
})
