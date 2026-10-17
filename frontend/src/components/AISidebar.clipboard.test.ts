import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import sidebarSource from './AISidebar.vue?raw'
import { readClipboardText } from '../composables/useClipboardWrite'

const { hostRead, dialog } = vi.hoisted(() => ({ hostRead: vi.fn(), dialog: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Clipboard: { Text: hostRead } }))
vi.mock('../utils/browserClipboard', () => ({ clipboardDialog: dialog }))

// Exercise the actual paste controller without mounting the sidebar's unrelated
// stores, file pickers and terminal views. No copy of its implementation lives here.
const pasteSource = sidebarSource.match(/^function pasteFromClipboard\(\) \{[\s\S]*?^\}/m)?.[0]
if (!pasteSource) throw new Error('AISidebar paste controller not found')
const createPaste = new Function(
  'readClipboardImage', 'attachClipboardImage', 'readClipboardText', 'insertTextAtCursor', 'Clipboard',
  `${pasteSource}\nreturn pasteFromClipboard`,
)

describe('AI sidebar clipboard fallback', () => {
  const browserRead = vi.fn()
  const attach = vi.fn()
  const insert = vi.fn()

  function paste(image: Promise<Blob | null> = Promise.resolve(null)) {
    createPaste(() => image, attach, readClipboardText, insert, { Text: hostRead })()
  }

  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('window', {
      __UNITERM_WEB__: { instanceId: 'host', clientId: 'visitor' },
      setTimeout, clearTimeout,
    })
    vi.stubGlobal('navigator', { clipboard: { readText: browserRead } })
    browserRead.mockReset().mockResolvedValue('visitor text')
    hostRead.mockReset().mockResolvedValue('host clipboard')
    dialog.mockReset()
    attach.mockReset().mockReturnValue(false)
    insert.mockReset()
  })

  afterEach(() => {
    expect(hostRead).not.toHaveBeenCalled()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('pastes visitor text when no usable clipboard image is found', async () => {
    paste()
    await vi.runAllTimersAsync()
    expect(insert).toHaveBeenCalledExactlyOnceWith('visitor text')
    expect(dialog).not.toHaveBeenCalled()
  })

  it.each(['visitor manual paste', ''])('uses only explicit visitor input on HTTP, including cancellation (%j)', async text => {
    vi.stubGlobal('navigator', {})
    dialog.mockResolvedValue(text)
    paste()
    await vi.runAllTimersAsync()
    expect(dialog).toHaveBeenCalledExactlyOnceWith('paste')
    if (text) expect(insert).toHaveBeenCalledExactlyOnceWith(text)
    else expect(insert).not.toHaveBeenCalled()
  })

  it('offers visitor input when image and text clipboard permissions are denied', async () => {
    browserRead.mockRejectedValue(new Error('denied'))
    dialog.mockResolvedValue('visitor manual paste')
    paste(Promise.reject(new Error('denied')))
    await vi.runAllTimersAsync()
    expect(dialog).toHaveBeenCalledExactlyOnceWith('paste')
    expect(insert).toHaveBeenCalledExactlyOnceWith('visitor manual paste')
  })

  it('ignores a late image after the text fallback has already run', async () => {
    let resolveImage!: (image: Blob) => void
    paste(new Promise(resolve => { resolveImage = resolve }))
    await vi.advanceTimersByTimeAsync(500)
    expect(insert).toHaveBeenCalledExactlyOnceWith('visitor text')
    resolveImage(new Blob(['image'], { type: 'image/png' }))
    await vi.runAllTimersAsync()
    expect(attach).not.toHaveBeenCalled()
    expect(insert).toHaveBeenCalledTimes(1)
  })

  it('does not also paste text when an image was consumed', async () => {
    const image = new Blob(['image'], { type: 'image/png' })
    attach.mockReturnValue(true)
    paste(Promise.resolve(image))
    await vi.runAllTimersAsync()
    expect(attach).toHaveBeenCalledExactlyOnceWith(image)
    expect(browserRead).not.toHaveBeenCalled()
    expect(dialog).not.toHaveBeenCalled()
    expect(insert).not.toHaveBeenCalled()
  })
})
