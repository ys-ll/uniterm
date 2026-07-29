export const LINES_PER_NOTCH = 3

export function decideWheelAction(
  deltaY: number,
  swallow: boolean,
  inAltScreen: boolean
): { forward: boolean; scrollLines: number } {
  if (!swallow) return { forward: true, scrollLines: 0 }
  if (!inAltScreen) return { forward: true, scrollLines: 0 }
  return { forward: false, scrollLines: Math.sign(deltaY) * LINES_PER_NOTCH }
}