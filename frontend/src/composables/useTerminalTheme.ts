import type { ITheme } from '@xterm/xterm'

/**
 * Resolve the real background color for an xterm theme.
 *
 * When the user enables a custom background image, uniterm previously set
 * `theme.background = 'rgba(0,0,0,0)'` so the canvas would show the image
 * through. xterm.js however parses that string and stores `rgb:0/0/0` as the
 * background — which means OSC 11 (background query) responses also report
 * pure black. TUI applications that probe the background to decide light/dark
 * (Claude Code, lazygit, etc.) then wrongly pick dark-mode colors.
 *
 * Fix: resolve `--bg-base` (the same CSS variable the application chrome
 * uses) to its actual computed color and hand that hex/rgb string to xterm.
 * With `allowTransparency: true` the canvas can still composite the
 * background image underneath.
 */
export function resolveXtermBackground(
  baseTheme: ITheme,
  backgroundEnabled: boolean,
  backgroundImage: string | null | undefined
): ITheme {
  if (!backgroundEnabled || !backgroundImage) return baseTheme

  const resolved = getComputedStyle(document.documentElement)
    .getPropertyValue('--bg-base')
    .trim()

  if (!resolved) return baseTheme

  return { ...baseTheme, background: resolved }
}