import { onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { GetTOTPCode } from '../../bindings/github.com/ys-ll/uniterm/app'
import { backendErrorText } from '../utils/backendError'

// The backend owns the RFC 6238 implementation (backend/session/totp.go), so
// the code rendered here is the exact code SSH keyboard-interactive would
// submit. Only the countdown is kept locally: the deadline is relative to the
// moment the response arrives, so a clock skew between Go and the webview can
// never leave an expired code on screen.
const DEBOUNCE_MS = 200
const TICK_MS = 250
const DEFAULT_PERIOD = 30

export type TotpCodeErrorKind = '' | 'vault' | 'invalid'

export function useTotpCode(secret: Ref<string>, active: Ref<boolean> = ref(true)) {
  const code = ref('')
  const secondsLeft = ref(0)
  const period = ref(DEFAULT_PERIOD)
  const errorKind = ref<TotpCodeErrorKind>('')
  const loading = ref(false)

  let deadline = 0
  let timer: ReturnType<typeof setInterval> | null = null
  let debounce: ReturnType<typeof setTimeout> | null = null
  // Guards against out-of-order responses while the user is typing a secret.
  let requestSeq = 0

  function stopTimer() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  function clearDebounce() {
    if (debounce) {
      clearTimeout(debounce)
      debounce = null
    }
  }

  function reset() {
    code.value = ''
    secondsLeft.value = 0
    period.value = DEFAULT_PERIOD
    errorKind.value = ''
    loading.value = false
    deadline = 0
  }

  function startTimer() {
    stopTimer()
    timer = setInterval(() => {
      const left = Math.max(0, Math.ceil((deadline - Date.now()) / 1000))
      secondsLeft.value = left
      if (left <= 0) void refresh()
    }, TICK_MS)
  }

  async function refresh() {
    const value = secret.value
    if (!active.value || !value.trim()) {
      requestSeq++
      stopTimer()
      reset()
      return
    }
    const seq = ++requestSeq
    loading.value = true
    errorKind.value = ''
    try {
      const result = await GetTOTPCode(value)
      if (seq !== requestSeq || !active.value) return
      code.value = result.code || ''
      period.value = result.period > 0 ? result.period : DEFAULT_PERIOD
      const remaining = result.secondsRemaining > 0 ? result.secondsRemaining : period.value
      deadline = Date.now() + remaining * 1000
      secondsLeft.value = Math.max(0, Math.ceil((deadline - Date.now()) / 1000))
      startTimer()
    } catch (e) {
      if (seq !== requestSeq || !active.value) return
      code.value = ''
      secondsLeft.value = 0
      stopTimer()
      errorKind.value = /vault|unlock/i.test(backendErrorText(e)) ? 'vault' : 'invalid'
    } finally {
      if (seq === requestSeq) loading.value = false
    }
  }

  watch([secret, active], () => {
    clearDebounce()
    stopTimer()
    if (!active.value) {
      requestSeq++
      reset()
      return
    }
    // The connection form passes the secret as the user types it; debounce so
    // every keystroke does not become an IPC round-trip.
    debounce = setTimeout(() => void refresh(), DEBOUNCE_MS)
  }, { immediate: true })

  onBeforeUnmount(() => {
    clearDebounce()
    stopTimer()
    requestSeq++
  })

  return { code, secondsLeft, period, errorKind, loading, refresh }
}
