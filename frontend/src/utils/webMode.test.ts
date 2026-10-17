import { afterEach, expect, it, vi } from 'vitest'
import { isWebMode, setWebClientId, webClientId } from './webMode'

afterEach(() => vi.unstubAllGlobals())

it('derives the visitor identity from the typed bootstrap object', () => {
  vi.stubGlobal('window', { __UNITERM_WEB__: { instanceId: 'host', clientId: '' } })
  expect(isWebMode()).toBe(true)
  expect(webClientId()).toBe('')
  setWebClientId('bootstrap-client')
  expect(webClientId()).toBe('bootstrap-client')
  expect(window.__UNITERM_WEB__).toEqual({ instanceId: 'host', clientId: 'bootstrap-client' })
  window.__UNITERM_WEB__ = undefined
  expect(isWebMode()).toBe(false)
  expect(webClientId()).toBe('desktop')
})
