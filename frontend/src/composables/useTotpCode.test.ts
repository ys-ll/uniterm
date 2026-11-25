import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ GetTOTPCode: vi.fn() }))

import { GetTOTPCode } from '../../bindings/github.com/ys-ll/uniterm/app'
import { useTotpCode } from './useTotpCode'

type State = ReturnType<typeof useTotpCode>
const secret = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ'

let state: State

// Called outside a component: Vue warns about the missing instance for the
// unmount hook, which is irrelevant to the timer/debounce logic under test.
function setup(initialSecret: string, active = true) {
  const secretRef = ref(initialSecret)
  const activeRef = ref(active)
  state = useTotpCode(secretRef, activeRef)
  return { secretRef, activeRef }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.spyOn(console, 'warn').mockImplementation(() => {})
  vi.mocked(GetTOTPCode).mockReset()
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('useTotpCode', () => {
  it('shows the backend code and reloads it when the period expires', async () => {
    vi.mocked(GetTOTPCode)
      .mockResolvedValueOnce({ code: '123456', secondsRemaining: 2, period: 30 })
      .mockResolvedValueOnce({ code: '654321', secondsRemaining: 30, period: 30 })

    setup(secret)
    await vi.advanceTimersByTimeAsync(200) // input debounce
    expect(GetTOTPCode).toHaveBeenCalledTimes(1)
    expect(state.code.value).toBe('123456')
    expect(state.secondsLeft.value).toBe(2)

    await vi.advanceTimersByTimeAsync(2000)
    expect(GetTOTPCode).toHaveBeenCalledTimes(2)
    expect(state.code.value).toBe('654321')
    expect(state.secondsLeft.value).toBe(30)
  })

  it('debounces edits to the secret and stops polling while inactive', async () => {
    vi.mocked(GetTOTPCode).mockResolvedValue({ code: '111111', secondsRemaining: 30, period: 30 })
    const { secretRef, activeRef } = setup('')
    secretRef.value = 'JBSWY3DP'
    secretRef.value = secret
    await vi.advanceTimersByTimeAsync(200)
    expect(GetTOTPCode).toHaveBeenCalledTimes(1)
    expect(GetTOTPCode).toHaveBeenCalledWith(secret)

    activeRef.value = false
    await vi.advanceTimersByTimeAsync(30_000)
    expect(GetTOTPCode).toHaveBeenCalledTimes(1)
    expect(state.code.value).toBe('')
  })

  it('distinguishes a locked vault from an invalid secret', async () => {
    vi.mocked(GetTOTPCode)
      .mockRejectedValueOnce(new Error('unlock the credential vault to use TOTP'))
      .mockRejectedValueOnce(new Error('invalid TOTP secret'))
    setup('enc:v1:ciphertext')
    await vi.advanceTimersByTimeAsync(200)
    expect(state.errorKind.value).toBe('vault')
    expect(state.code.value).toBe('')

    setup('not-a-secret')
    await vi.advanceTimersByTimeAsync(200)
    expect(state.errorKind.value).toBe('invalid')
  })
})
