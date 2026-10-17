import { afterEach, beforeEach, expect, it, vi } from 'vitest'

const mock = vi.hoisted(() => ({
  settings: vi.fn(), initTerminals: vi.fn(), mount: vi.fn(), unmount: vi.fn(), title: vi.fn(),
}))
vi.mock('vue', () => ({ createApp: () => ({ use() {}, mount: mock.mount, onUnmount: mock.unmount }) }))
vi.mock('pinia', () => ({ createPinia: () => ({}) }))
vi.mock('element-plus', () => ({ default: {}, ElDialog: { props: {} } }))
vi.mock('./App.vue', () => ({ default: {} }))
vi.mock('@wailsio/runtime', () => ({ Window: { SetTitle: mock.title } }))
vi.mock('./stores/settingsStore', () => ({ useSettingsStore: () => ({ init: mock.settings }) }))
vi.mock('./utils/webTerminalIntegration', () => ({ initBrowserTerminalIntegration: mock.initTerminals }))
vi.mock('./i18n', () => ({ setLocale() {} }))

beforeEach(() => {
  vi.resetModules()
  vi.clearAllMocks()
  vi.stubGlobal('window', {})
  vi.stubGlobal('document', { addEventListener() {}, title: '' })
})
afterEach(() => vi.unstubAllGlobals())

it.each([false, true])('installs terminal authority after settings and before mounted session creators (web=%s)', async web => {
  if (web) window.__UNITERM_WEB__ = { instanceId: 'host', clientId: 'visitor' }
  const order: string[] = []
  let finishSettings!: () => void
  mock.settings.mockImplementation(() => new Promise<void>(resolve => { finishSettings = () => { order.push('settings'); resolve() } }))
  const dispose = vi.fn()
  mock.initTerminals.mockImplementation(() => { order.push('terminals'); return dispose })
  mock.mount.mockImplementation(() => order.push('mount'))
  await import('./main')
  expect(mock.initTerminals).not.toHaveBeenCalled()
  expect(mock.mount).not.toHaveBeenCalled()
  finishSettings()
  await vi.waitFor(() => expect(mock.mount).toHaveBeenCalledWith('#app'))
  expect(order).toEqual(['settings', 'terminals', 'mount'])
  expect(mock.unmount).toHaveBeenCalledExactlyOnceWith(dispose)
  expect(mock.title).toHaveBeenCalledTimes(web ? 0 : 1)
})
