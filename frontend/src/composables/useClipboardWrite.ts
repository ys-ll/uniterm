import { Clipboard } from '@wailsio/runtime'
import { isWebMode } from '../utils/webMode'
import { clipboardDialog } from '../utils/browserClipboard'

// Write/read the OS clipboard with layered fallbacks.
//
// Write order is platform-dependent. On macOS, Wails must go first:
// navigator.clipboard silently fails in WKWebView when the webview isn't
// first responder (#827) — exactly when selection-event-driven copies fire.
// On Windows/Linux the browser API runs entirely inside the renderer and is
// the fast, dependable path for the high-frequency selection copy; the Wails
// route adds an IPC hop that serializes on the Go main thread and can stall
// under heavy terminal output. The untried side always serves as fallback, so
// a primary-path failure (browser write rejected on an unfocused document, or
// a Wails error) still lands the text.
//
// Every call is wrapped in a timeout: a hung call must degrade to the other
// path instead of leaving a copy silently dropped (the selection copy has no
// visible progress, and paste would then read a stale clipboard).
type ClipboardWriter = (text: string) => Promise<boolean>

const CALL_TIMEOUT_MS = 500

// Resolves with fallback() if p neither resolves nor rejects within the budget.
function withTimeout<T>(p: Promise<T>, fallback: () => T): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => resolve(fallback()), CALL_TIMEOUT_MS)
    p.then(
      value => { clearTimeout(timer); resolve(value) },
      error => { clearTimeout(timer); reject(error) },
    )
  })
}

const browserWriter: ClipboardWriter = async (text) => {
  try {
    return await withTimeout(
      navigator.clipboard.writeText(text).then(() => true),
      () => false,
    )
  } catch {
    return false
  }
}

const wailsWriter: ClipboardWriter = async (text) => {
  let ok = false
  try {
    ok = await withTimeout(Clipboard.SetText(text).then(v => Boolean(v)), () => false)
  } catch {
    ok = false
  }
  return ok || browserWriter(text)
}

const isApplePlatform = () => /Mac|iPhone|iPad|iPod/.test(navigator.userAgent)
const isAndroidPlatform = () => /Android/.test(navigator.userAgent)

export const writeClipboard: ClipboardWriter = async (text) => {
  if (isWebMode()) return (await browserWriter(text)) || Boolean(await clipboardDialog('copy', text))
  if (typeof Clipboard.SetText !== 'function') return browserWriter(text)
  // Wails must go first on macOS (#827, see above) and on Android: the
  // WebView's navigator.clipboard.writeText resolves successfully there but
  // only writes the renderer-internal clipboard — the system ClipboardManager
  // (what Wails reads, and what every other app sees) stays untouched, so a
  // browser-first copy lands nowhere. Verified in-device on an emulator.
  if (isApplePlatform() || isAndroidPlatform()) return wailsWriter(text)
  return (await browserWriter(text)) || wailsWriter(text)
}

// Read with one delayed retry. An empty result can mean the clipboard was
// momentarily locked by another process (clipboard managers, IME history) and
// OpenClipboard timed out; the retry rides out the contention window instead
// of silently killing a paste.
const READ_RETRY_DELAY_MS = 120

export async function readClipboardText(): Promise<string> {
  if (isWebMode()) {
    try {
      // Do not time out the browser's permission prompt and open a second
      // dialog beneath it. An empty successful read is a valid clipboard.
      return await navigator.clipboard.readText()
    } catch {
      return String(await clipboardDialog('paste'))
    }
  }
  for (let attempt = 0; attempt < 2; attempt++) {
    if (attempt > 0) await new Promise(resolve => setTimeout(resolve, READ_RETRY_DELAY_MS))
    let text = ''
    try {
      text = await withTimeout(Clipboard.Text().then(t => t ?? ''), () => '')
    } catch {
      text = ''
    }
    if (text) return text
  }
  return ''
}
