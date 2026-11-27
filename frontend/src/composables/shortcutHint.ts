import { useSettingsStore } from '../stores/settingsStore'
import { isMobilePlatform } from '../utils/platform'
import { formatKeyBinding, type ShortcutAction } from './useKeyboardShortcuts'

// " (Ctrl+Shift+K)" suffix for a shortcut action's tooltip, '' when the
// action is unbound or on mobile (no hardware keyboard — the hint would be
// meaningless). Reads the live settingsStore, so tooltips follow the user's
// rebinds.
export function shortcutSuffix(action: ShortcutAction): string {
  if (isMobilePlatform()) return ''
  const b = useSettingsStore().settings.keyboard[action]
  if (!b) return ''
  const key = formatKeyBinding(b, /Mac|iPhone|iPad/.test(navigator.userAgent))
  return key ? ` (${key})` : ''
}

// Dockable panel view → the shortcut action that toggles it. Tabs show the
// bound combo in their tooltip; views without an entry (none today) would
// show the plain name only.
export const PANEL_VIEW_ACTIONS: Record<string, ShortcutAction> = {
  connections: 'toggleSidebar',
  files: 'toggleFilePanel',
  monitor: 'toggleMonitor',
  tunnels: 'toggleTunnels',
  quickCommands: 'openQuickCommands',
  history: 'toggleHistory',
  personalization: 'togglePersonalization',
  commandSend: 'toggleCommandSend',
  notes: 'toggleNotes',
  ai: 'focusAI',
}
