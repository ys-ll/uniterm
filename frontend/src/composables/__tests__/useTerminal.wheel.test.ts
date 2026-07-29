import { describe, it, expect } from 'vitest'
import { decideWheelAction, LINES_PER_NOTCH } from '../useTerminal.wheel'

describe('decideWheelAction', () => {
  it.each([
    [true,  true,  -100, { forward: false, scrollLines: -LINES_PER_NOTCH }],
    [true,  true,  +100, { forward: false, scrollLines:  LINES_PER_NOTCH }],
    [true,  true,     0, { forward: false, scrollLines:  0 }],
    [true,  false, -100, { forward: true,  scrollLines:  0 }],
    [false, true,  -100, { forward: true,  scrollLines:  0 }],
    [false, false, +100, { forward: true,  scrollLines:  0 }],
  ] as const)('swallow=%s inAlt=%s deltaY=%s → %s', (swallow, inAlt, deltaY, expected) => {
    expect(decideWheelAction(deltaY, swallow, inAlt)).toEqual(expected)
  })
})