import { Clipboard } from '@wailsio/runtime'
import { isWebServerMode } from '../services/webSftpBridge'

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
// In web-server mode (Docker/browser deployment) the Wails clipboard is a
// compile-time no-op stub that reports success without doing anything, so
// every call must stay in the browser: navigator.clipboard on secure
// origins, a textarea+execCommand fallback for plain-HTTP deployments where
// navigator.clipboard does not exist.
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

// Fallback for insecure origins (plain-HTTP server deployments), where
// navigator.clipboard is undefined. execCommand('copy') is deprecated but
// still honored inside the user-gesture handlers every caller runs in.
const legacyWriter: ClipboardWriter = async (text) => {
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}

const wailsWriter: ClipboardWriter = async (text) => {
  let ok = false
  try {
    // Wails v3's runtime SetText resolves with no value on success, even
    // though the underlying Go API returns bool. Do not mistake that void
    // result for failure and write again through WKWebView's browser API.
    // Keep explicit false as failure for runtimes that expose a status.
    ok = await withTimeout(Clipboard.SetText(text).then((value: unknown) => value !== false), () => false)
  } catch {
    ok = false
  }
  return ok || browserWriter(text)
}

const isApplePlatform = () => /Mac|iPhone|iPad|iPod/.test(navigator.userAgent)
const isAndroidPlatform = () => /Android/.test(navigator.userAgent)

export const writeClipboard: ClipboardWriter = async (text) => {
  if (await isWebServerMode()) {
    return (await browserWriter(text)) || legacyWriter(text)
  }
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

const browserReader = async (): Promise<string> => {
  for (let attempt = 0; attempt < 2; attempt++) {
    if (attempt > 0) await new Promise(resolve => setTimeout(resolve, READ_RETRY_DELAY_MS))
    try {
      const text = await withTimeout(
        navigator.clipboard.readText().then(t => t ?? ''),
        () => '',
      )
      if (text) return text
    } catch {
      // Read refused (no permission, insecure origin) — fall through to the retry.
    }
  }
  return ''
}

export async function readClipboardText(): Promise<string> {
  // Web-server mode: read through the browser (secure origins only). On
  // plain-HTTP deployments the browser refuses programmatic reads and there
  // is no way to reach the clipboard outside a paste event — paste stays
  // empty there; plain Ctrl+V (xterm's native paste event) still works.
  if (await isWebServerMode()) {
    return await browserReader()
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
