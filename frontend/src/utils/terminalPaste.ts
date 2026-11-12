// Paste payload preparation + write/scroll for an SSH/local session.
//
// Extracted from BaseTerminal.pasteToSession so the paste behavior is
// unit-testable (the component itself is not mountable in vitest).

export interface PasteTarget {
  /** Already-known bracketed-paste mode of the target session's terminal. */
  bracketedPasteMode: boolean
  /** Write the final payload to the session. */
  write: (payload: string) => void
  /** Yank the viewport back to the bottom after writing. */
  scrollToBottom: () => void
}

/** True when the text would execute as more than one command line. A single
 * trailing newline is ignored — copying one command out of an editor usually
 * carries one, and prompting for it would be noise (issue 1370). */
export function isMultilinePaste(text: string): boolean {
  return text.replace(/\n+$/, '').includes('\n')
}

/** Normalize line endings: \r\n -> \n, then drop leftover \r (Windows
 * clipboard). Keeps multi-line pastes from doubling newlines in vim. */
export function normalizePastedText(text: string): string {
  return text.replace(/\r\n/g, '\n').replace(/\r/g, '')
}

/** Wrap in bracketed-paste markers when the app enabled the mode, so vim etc.
 * don't re-indent each pasted line. */
export function bracketPaste(text: string, enabled: boolean): string {
  return enabled ? `\x1b[200~${text}\x1b[201~` : text
}

// Paste a block of text into one session: normalize + bracket-wrap, write to
// the session, then yank the viewport back to the bottom. Pasted input must
// behave like keyboard input — Enter/arrows already auto-scroll via xterm's
// scrollOnUserInput, but paste bypasses the keydown path (issue 629).
export function pasteWithScroll(target: PasteTarget, text: string): void {
  const normalized = normalizePastedText(text)
  // With the bracketed-paste protocol the shell asked for literal data, so
  // \n passes through untouched. Without it the payload must mimic keyboard
  // input: ConPTY only maps \r to Enter, so a bare \n makes cmd/PowerShell
  // merge every pasted line into one (issue 1125) — convert like the native
  // Windows console does.
  const payload = target.bracketedPasteMode
    ? bracketPaste(normalized, true)
    : normalized.replace(/\n/g, '\r')
  target.write(payload)
  target.scrollToBottom()
}