import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// vitest runs in node by default and the project has no jsdom dep; stub the
// navigator surface the module touches (clipboard, userAgent).
function stubNavigatorMember(name: string, value: unknown) {
  Object.defineProperty(globalThis.navigator, name, { configurable: true, value })
}

const WIN_UA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36'
const MAC_UA =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36'

async function flushAsync() {
  for (let i = 0; i < 5; i++) await Promise.resolve()
}

const { mockClipboardSetText, mockClipboardText } = vi.hoisted(() => ({
  mockClipboardSetText: vi.fn(),
  mockClipboardText: vi.fn(),
}))

const { dialog } = vi.hoisted(() => ({ dialog: vi.fn() }))
vi.mock('../utils/browserClipboard', () => ({ clipboardDialog: dialog }))

vi.mock('@wailsio/runtime', () => ({
  Clipboard: {
    SetText: (...args: unknown[]) => mockClipboardSetText(...args),
    Text: (...args: unknown[]) => mockClipboardText(...args),
  },
}))

import { writeClipboard, readClipboardText } from './useClipboardWrite'

describe('writeClipboard', () => {
  let writeText: ReturnType<typeof vi.fn>

  beforeEach(() => {
    mockClipboardSetText.mockReset()
    mockClipboardText.mockReset()
    writeText = vi.fn().mockResolvedValue(undefined)
    stubNavigatorMember('userAgent', WIN_UA)
    stubNavigatorMember('clipboard', { writeText })
    vi.useRealTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // Windows/Linux: the browser API runs in the renderer — it must be the
  // primary path so the hot selection copy never queues behind the Go main
  // thread via the Wails IPC hop.
  it('writes via the browser API first on Windows and does not touch Wails on success', async () => {
    mockClipboardSetText.mockResolvedValue(true)
    const ok = await writeClipboard('hello')
    expect(ok).toBe(true)
    expect(writeText).toHaveBeenCalledWith('hello')
    expect(mockClipboardSetText).not.toHaveBeenCalled()
  })

  it('falls back to Wails when the browser write fails on Windows', async () => {
    writeText.mockRejectedValue(new Error('document not focused'))
    mockClipboardSetText.mockResolvedValue(true)
    const ok = await writeClipboard('hello')
    expect(ok).toBe(true)
    expect(writeText).toHaveBeenCalledWith('hello')
    expect(mockClipboardSetText).toHaveBeenCalledWith('hello')
  })

  // macOS: WKWebView's navigator.clipboard silently fails when the webview
  // isn't first responder — Wails must be the primary path there.
  it('writes via Wails first on macOS and does not touch the browser API on success', async () => {
    stubNavigatorMember('userAgent', MAC_UA)
    mockClipboardSetText.mockResolvedValue(true)
    const ok = await writeClipboard('hello')
    expect(ok).toBe(true)
    expect(mockClipboardSetText).toHaveBeenCalledWith('hello')
    expect(writeText).not.toHaveBeenCalled()
  })

  it('falls back to the browser API when Wails resolves false on macOS', async () => {
    stubNavigatorMember('userAgent', MAC_UA)
    mockClipboardSetText.mockResolvedValue(false)
    const ok = await writeClipboard('hello')
    expect(ok).toBe(true)
    expect(mockClipboardSetText).toHaveBeenCalledWith('hello')
    expect(writeText).toHaveBeenCalledWith('hello')
  })

  // A stalled Wails call (busy Go main thread) must degrade to the browser
  // path instead of leaving the copy silently dropped.
  it('falls back to the browser API when the Wails write hangs past the timeout', async () => {
    stubNavigatorMember('userAgent', MAC_UA)
    vi.useFakeTimers()
    mockClipboardSetText.mockReturnValue(new Promise(() => {}))
    const pending = writeClipboard('hello')
    await vi.advanceTimersByTimeAsync(500)
    const ok = await pending
    expect(ok).toBe(true)
    expect(mockClipboardSetText).toHaveBeenCalledWith('hello')
    expect(writeText).toHaveBeenCalledWith('hello')
  })

  it('falls back to Wails when the browser write hangs past the timeout on Windows', async () => {
    vi.useFakeTimers()
    stubNavigatorMember('clipboard', { writeText: () => new Promise(() => {}) })
    mockClipboardSetText.mockResolvedValue(true)
    const pending = writeClipboard('hello')
    await vi.advanceTimersByTimeAsync(500)
    const ok = await pending
    expect(ok).toBe(true)
    expect(mockClipboardSetText).toHaveBeenCalledWith('hello')
  })
})

describe('readClipboardText', () => {
  beforeEach(() => {
    mockClipboardText.mockReset()
    vi.useRealTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns non-empty text immediately without retrying', async () => {
    mockClipboardText.mockResolvedValue('clipboard-content')
    const text = await readClipboardText()
    expect(text).toBe('clipboard-content')
    expect(mockClipboardText).toHaveBeenCalledTimes(1)
  })

  // An empty read can mean the clipboard was momentarily locked by another
  // process; one quiet retry rides out the contention window.
  it('retries once after a delay when the first read comes back empty', async () => {
    vi.useFakeTimers()
    mockClipboardText
      .mockResolvedValueOnce('')
      .mockResolvedValueOnce('late-content')
    const pending = readClipboardText()
    await vi.advanceTimersByTimeAsync(120)
    const text = await pending
    expect(text).toBe('late-content')
    expect(mockClipboardText).toHaveBeenCalledTimes(2)
  })

  it('retries once when the first read throws', async () => {
    vi.useFakeTimers()
    mockClipboardText
      .mockRejectedValueOnce(new Error('clipboard locked'))
      .mockResolvedValueOnce('recovered')
    const pending = readClipboardText()
    await vi.advanceTimersByTimeAsync(120)
    const text = await pending
    expect(text).toBe('recovered')
    expect(mockClipboardText).toHaveBeenCalledTimes(2)
  })

  it('gives up after one retry when both reads come back empty', async () => {
    vi.useFakeTimers()
    mockClipboardText.mockResolvedValue('')
    const pending = readClipboardText()
    await vi.advanceTimersByTimeAsync(200)
    const text = await pending
    expect(text).toBe('')
    expect(mockClipboardText).toHaveBeenCalledTimes(2)
  })
})

describe('browser clipboard isolation', () => {
  beforeEach(() => {
    vi.stubGlobal('window', { __UNITERM_WEB__: { instanceId: 'host', clientId: 'visitor' } })
    mockClipboardSetText.mockReset()
    mockClipboardText.mockReset()
    dialog.mockReset()
    stubNavigatorMember('userAgent', MAC_UA)
  })
  afterEach(() => vi.unstubAllGlobals())

  it('uses the visiting browser clipboard even on Apple platforms', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    const readText = vi.fn().mockResolvedValue('visitor text')
    stubNavigatorMember('clipboard', { writeText, readText })
    expect(await writeClipboard('copy')).toBe(true)
    expect(await readClipboardText()).toBe('visitor text')
    expect(writeText).toHaveBeenCalledWith('copy')
    expect(mockClipboardSetText).not.toHaveBeenCalled()
    expect(mockClipboardText).not.toHaveBeenCalled()
  })

  it('offers explicit copy/paste on insecure LAN HTTP without accessing the host clipboard', async () => {
    stubNavigatorMember('clipboard', undefined)
    dialog.mockResolvedValueOnce(false).mockResolvedValueOnce('pasted by visitor')
    expect(await writeClipboard('sensitive text')).toBe(false)
    expect(dialog).toHaveBeenCalledWith('copy', 'sensitive text')
    expect(await readClipboardText()).toBe('pasted by visitor')
    expect(dialog).toHaveBeenCalledWith('paste')
    expect(mockClipboardSetText).not.toHaveBeenCalled()
    expect(mockClipboardText).not.toHaveBeenCalled()
  })
})
