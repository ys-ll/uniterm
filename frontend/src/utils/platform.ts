import { ref } from 'vue'
import { isWebMode } from './webMode'

// Platform detection shared by components that adapt desktop-only UI
// (window controls and keyboard hints) for mobile.
export type UiPlatform = 'windows' | 'darwin' | 'linux' | 'android' | 'ios'

// Detect synchronously so the layout is correct on first render, even if the
// Wails System.Environment() call is slow or fails.
export function detectPlatformSync(): UiPlatform {
  const ua = navigator.userAgent
  if (/Android/.test(ua)) return 'android'
  if (/iPhone|iPad/.test(ua)) return 'ios'
  if (/Mac/.test(ua)) return 'darwin'
  if (/Linux/.test(ua)) return 'linux'
  return 'windows'
}

// Mobile platforms use touch layouts without desktop window controls.
export function isMobilePlatform(p: UiPlatform = detectPlatformSync()): boolean {
  return p === 'android' || p === 'ios'
}

export function supportsLocalTerminal(p: UiPlatform = detectPlatformSync()): boolean {
  return isWebMode() || p !== 'ios'
}

// Phone-shaped viewport: a touch platform whose window is phone-narrow.
// Gates *behavior* changes (long-press menus, key bar) so tablets keep
// desktop-like behavior. Pure layout narrowing is done in CSS via
// @media (max-width: 640px) and must NOT use this.
const NARROW_MQ = '(max-width: 640px)'
const narrowMq =
  typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia(NARROW_MQ)
    : null

export const isPhoneLayout = ref(isMobilePlatform() && !!narrowMq?.matches)

// Narrow viewport (<640px) on ANY platform: gates layout switches such as the
// tree drawer in DB/Redis/Mongo/ES/Container/K8s panels. Desktop windows that
// happen to be small get the same compact layout as phones.
export const isNarrowScreen = ref(!!narrowMq?.matches)

if (narrowMq) {
  narrowMq.addEventListener('change', () => {
    isPhoneLayout.value = isMobilePlatform() && narrowMq.matches
    isNarrowScreen.value = narrowMq.matches
  })
}
