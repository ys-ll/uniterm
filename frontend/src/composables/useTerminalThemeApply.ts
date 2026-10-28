import type { Terminal } from '@xterm/xterm'
import { watch } from 'vue'
import { notifyOverlayHighlightThemeChanged } from './overlayHighlight'
import { getXtermTheme } from './useTerminal'
import { resolveXtermBackground, applyTerminalBgVar, resolveTerminalThemeName } from './useTerminalTheme'
import { formatFontFamily } from '../utils/formatFontFamily'
import { useSettingsStore } from '../stores/settingsStore'
import { useLocalStateStore } from '../stores/localStateStore'

export interface UseTerminalThemeApplyOptions {
  getTerminal: () => Terminal | null
  onResize: () => void
}

/**
 * Applies xterm theme/options live and watches settings + app-theme changes.
 *
 * Owns the three reactive watchers that keep the terminal's visual options
 * (font, theme, cursor, contrast, scrollback) in sync with the settings
 * store. The host calls applyXtermTheme() once after the terminal is
 * acquired so the initial paint matches the stored theme.
 */
export function useTerminalThemeApply(options: UseTerminalThemeApplyOptions) {
  const { getTerminal, onResize } = options
  const settingsStore = useSettingsStore()
  const localStateStore = useLocalStateStore()

  function applyXtermTheme(themeName: string) {
    const terminal = getTerminal()
    if (!terminal) return
    const theme = resolveXtermBackground(
      getXtermTheme(resolveTerminalThemeName(themeName, settingsStore.resolvedAppTheme), settingsStore.settings.customTerminalThemes),
      localStateStore.state.backgroundEnabled,
      localStateStore.state.backgroundImage
    )
    terminal.options.theme = theme
    applyTerminalBgVar(terminal, theme)
    // Highlight colors are resolved from the palette per refresh — force a
    // pass so recolored text switches to the new theme immediately.
    notifyOverlayHighlightThemeChanged(terminal)
  }

  // Watch terminal settings changes
  watch(() => settingsStore.settings.terminal, (ts) => {
    const terminal = getTerminal()
    if (!terminal) return
    if (ts.fontSize) terminal.options.fontSize = ts.fontSize
    if (ts.fontFamily) terminal.options.fontFamily = formatFontFamily(ts.fontFamily, ts.fallbackFont)
    if (ts.fontWeight) terminal.options.fontWeight = ts.fontWeight
    if (ts.maxHistoryLines) terminal.options.scrollback = ts.maxHistoryLines
    if (ts.theme) applyXtermTheme(ts.theme)
    if (typeof ts.cursorBlink === 'boolean') {
      terminal.options.cursorBlink = ts.cursorBlink
      // xterm keeps an internal blink state set by DECSET 12; if the
      // terminal previously received \x1b[?12h from a remote shell, just
      // flipping the option may not stop the running blink animation.
      // Force-reset by feeding the cursor a DECRST 12 sequence.
      if (!ts.cursorBlink) terminal.write('\x1b[?12l')
    }
    if (ts.wordSeparator) {
      // xterm reads wordSeparator on each selection via rawOptions; assigning
      // here is enough to change the next double-click selection without
      // recreating the terminal.
      terminal.options.wordSeparator = ts.wordSeparator
    }
    // Live minimum-contrast (F-039): xterm reads it on each render, so a plain
    // assignment re-evaluates the atlas without recreating the terminal.
    if (typeof ts.minimumContrast === 'number') {
      terminal.options.minimumContrastRatio = ts.minimumContrast
    }
    // Live cursorStyle is applied by xterm on the next cursor render.
    if (ts.cursorStyle) terminal.options.cursorStyle = ts.cursorStyle
    onResize()
  }, { deep: true })

  // Watch for background image toggling to update terminal transparency
  watch(
    () => localStateStore.state.backgroundEnabled,
    () => applyXtermTheme(settingsStore.settings.terminal.theme || 'dark')
  )

  // Re-apply when the app theme flips so FOLLOW_APP_THEME tracks it live.
  watch(
    () => settingsStore.resolvedAppTheme,
    () => applyXtermTheme(settingsStore.settings.terminal.theme || 'dark')
  )

  return { applyXtermTheme }
}