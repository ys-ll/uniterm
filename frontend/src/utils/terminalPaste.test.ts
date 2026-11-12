import { describe, it, expect, vi } from 'vitest'
import { normalizePastedText, bracketPaste, pasteWithScroll, isMultilinePaste } from './terminalPaste'
import type { PasteTarget } from './terminalPaste'

function mkTarget(overrides: Partial<PasteTarget> = {}): PasteTarget & { scrollToBottom: ReturnType<typeof vi.fn> } {
  return {
    bracketedPasteMode: false,
    write: vi.fn(),
    scrollToBottom: vi.fn(),
    ...overrides,
  }
}

describe('normalizePastedText', () => {
  it('converts CRLF to LF and strips stray CR', () => {
    expect(normalizePastedText('a\r\nb\r\nc')).toBe('a\nb\nc')
    expect(normalizePastedText('a\rb')).toBe('ab')
  })

  it('leaves plain text untouched', () => {
    expect(normalizePastedText('ls -la')).toBe('ls -la')
  })
})

describe('isMultilinePaste', () => {
  it('detects text with multiple lines', () => {
    expect(isMultilinePaste('a\nb')).toBe(true)
    expect(isMultilinePaste('a\nb\nc')).toBe(true)
  })

  // Copying a single command out of an editor usually carries one trailing
  // newline — that must not trigger the multi-line confirmation.
  it('ignores trailing newlines', () => {
    expect(isMultilinePaste('ls -la')).toBe(false)
    expect(isMultilinePaste('ls -la\n')).toBe(false)
    expect(isMultilinePaste('ls -la\n\n')).toBe(false)
  })
})

describe('bracketPaste', () => {
  it('wraps when bracketed-paste mode is on', () => {
    expect(bracketPaste('vim', true)).toBe('\x1b[200~vim\x1b[201~')
  })

  it('passes through when bracketed-paste mode is off', () => {
    expect(bracketPaste('vim', false)).toBe('vim')
  })
})

describe('pasteWithScroll', () => {
  it('writes the normalized, wrapped payload to the session', () => {
    const t = mkTarget({ bracketedPasteMode: true })
    pasteWithScroll(t, 'a\r\nb')
    expect(t.write).toHaveBeenCalledWith('\x1b[200~a\nb\x1b[201~')
  })

  // When the target has no bracketed-paste protocol the payload must mimic
  // keyboard input: ConPTY only maps \r to the Enter key, so pasted newlines
  // must become \r or cmd/PowerShell merge every line into one (issue 1125).
  it('sends \r for newlines when bracketed-paste mode is off', () => {
    const t = mkTarget({ bracketedPasteMode: false })
    pasteWithScroll(t, 'a\r\nb\nc')
    expect(t.write).toHaveBeenCalledWith('a\rb\rc')
  })

  // The regression under test (issue 629): pasting while scrolled up must
  // yank the viewport back to the bottom, mirroring how Enter/arrow keys
  // auto-scroll via xterm's scrollOnUserInput.
  it('scrolls the viewport to the bottom after writing pasted text', () => {
    const t = mkTarget()
    pasteWithScroll(t, 'printf "big log\\n"')
    expect(t.scrollToBottom).toHaveBeenCalled()
  })
})