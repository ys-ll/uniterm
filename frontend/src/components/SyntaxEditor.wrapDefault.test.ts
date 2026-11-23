// @vitest-environment jsdom
// Regression: SyntaxEditor must enable CodeMirror line wrapping by default.
// Vue casts an absent Boolean prop (no declared default) to false, so callers
// that omit `wrap` used to silently lose EditorView.lineWrapping.
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import SyntaxEditor from './SyntaxEditor.vue'

vi.mock('@wailsio/runtime', () => {
  const stub: any = new Proxy(function () {}, {
    get: (_t, k) => (k === Symbol.toPrimitive ? undefined : stub),
    apply: () => stub,
    construct: () => stub,
  })
  return { Clipboard: { Text: vi.fn() }, Create: stub, ...stub }
})
vi.mock('../composables/useClipboardWrite', () => ({ writeClipboard: vi.fn() }))
vi.mock('../i18n', () => ({ useI18n: () => ({ t: (k: string) => k }) }))
vi.mock('../stores/settingsStore', () => ({
  useSettingsStore: () => ({ resolvedAppTheme: 'light' }),
}))

const mounted: ReturnType<typeof createApp>[] = []

function mountEditor(props: any) {
  if (!window.matchMedia) {
    window.matchMedia = (() => ({
      matches: false,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
    })) as any
  }
  const root = document.createElement('div')
  document.body.appendChild(root)
  const app = createApp({ render: () => h(SyntaxEditor, props) })
  app.use(createPinia())
  mounted.push(app)
  app.mount(root)
  return root
}

afterEach(() => {
  mounted.splice(0).forEach(a => a.unmount())
  document.body.innerHTML = ''
})

describe('SyntaxEditor wrap default', () => {
  it('wraps by default when the wrap prop is omitted (notes editor usage)', () => {
    const root = mountEditor({ modelValue: 'hello\nworld', lang: 'md', compact: true })
    const content = root.querySelector('.cm-content') as HTMLElement
    expect(content.className).toContain('cm-lineWrapping')
    expect(getComputedStyle(content).whiteSpace).not.toBe('pre')
  })

  it('still allows explicit opt-out via :wrap="false"', () => {
    const root = mountEditor({ modelValue: 'a\nb', wrap: false })
    const content = root.querySelector('.cm-content') as HTMLElement
    expect(content.className).not.toContain('cm-lineWrapping')
    expect(getComputedStyle(content).whiteSpace).toBe('pre')
  })
})
