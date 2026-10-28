import type { Terminal } from '@xterm/xterm'
import { queuedSessionWriteObserved as queuedSessionWrite } from '../services/terminalCommandWrite'
import { sanitizeTerminalOutput } from '../utils/terminalSanitize'
import { FrontendLog } from '../../bindings/github.com/ys-ll/uniterm/app'
import { usePanelStore } from '../stores/panelStore'
import { useTabStore } from '../stores/tabStore'
import { useZmodemStore } from '../stores/zmodemStore'
import type { useTerminalInput } from './useTerminalInput'
import type { useSuggestions } from './useSuggestions'

export interface UseTerminalHelpersOptions {
  getTerminal: () => Terminal | null
  getTerminalInput: () => ReturnType<typeof useTerminalInput> | null
  suggestions: ReturnType<typeof useSuggestions>
  getSessionId: () => string | null | undefined
  getPanelId: () => string | undefined
  getMode: () => 'ssh' | 'sftp' | 'local'
  getBroadcastActive: () => boolean
}

/**
 * Terminal helper functions: suggestion application, native file-drop path
 * conversion, and terminal history sanitization.
 */
export function useTerminalHelpers(options: UseTerminalHelpersOptions) {
  const {
    getTerminal, getTerminalInput, suggestions,
    getSessionId, getPanelId, getMode, getBroadcastActive,
  } = options

  const panelStore = usePanelStore()
  const tabStore = useTabStore()
  const zmodemStore = useZmodemStore()

  function toShellPath(nativePath: string, shellPath?: string): string {
    const isWinPath = /^[A-Za-z]:\\/.test(nativePath)
    if (!isWinPath) return nativePath.replace(/\\/g, '/')

    const lower = (shellPath || '').toLowerCase()
    let converted: string
    if (lower.includes('wsl')) {
      const drive = nativePath.charAt(0).toLowerCase()
      converted = '/mnt/' + drive + nativePath.slice(2).replace(/\\/g, '/')
    } else if (lower.includes('bash') || lower.includes('git') || lower.includes('msys') || lower.includes('cygwin') || lower.includes('mingw')) {
      const drive = nativePath.charAt(0).toLowerCase()
      converted = '/' + drive + nativePath.slice(2).replace(/\\/g, '/')
    } else {
      converted = nativePath
    }
    // Quote paths with spaces so the shell treats them as a single argument
    return converted.includes(' ') ? `"${converted}"` : converted
  }

  // Called with real absolute paths from the native OS file-drop event.
  function onNativePathsDropped(paths: string[]) {
    const sid = getSessionId()
    if (!sid || paths.length === 0) return

    // Local terminal: paste file paths as input, adapting format to the shell
    if (getMode() === 'local') {
      const panel = panelStore.getPanel(getPanelId() || '')
      const shellPath = panel?.config?.shellPath
      const text = paths.map(p => toShellPath(p, shellPath)).join(' ')
      queuedSessionWrite(sid, text)
      return
    }

    // Remote terminal: trigger zmodem upload
    zmodemStore.setPendingUploadFiles(sid, paths)
    queuedSessionWrite(sid, 'rz -be\n')
  }

  function sanitizeTerminalHistory(text: string): string {
    const cleaned = sanitizeTerminalOutput(text)
    // Forward debug info to backend log so we can inspect the raw garbage.
    if (cleaned !== text) {
      FrontendLog('sanitizeTerminalHistory', `raw last 400: ${JSON.stringify(text.slice(-400))}`)
      FrontendLog('sanitizeTerminalHistory', `cleaned last 400: ${JSON.stringify(cleaned.slice(-400))}`)
    }
    return cleaned
  }

  async function applySuggestion(item: ReturnType<typeof suggestions.getSelectedItem>) {
    const terminal = getTerminal()
    const terminalInput = getTerminalInput()
    if (!item || !terminal || !terminalInput) return

    if (item.type === 'ai-preview') {
      // Step 1: Generate AI suggestion
      await suggestions.generateAISuggestion(terminalInput.lineBuffer.value)
      return
    }

    const sid = getSessionId()

    if (item.type === 'ai-result' || item.type === 'history' || item.type === 'quick-command') {
      // Replace entire line with Ctrl+U. Using backspaces only works when the
      // replacement is exactly the currentToken; for multi-token input (e.g.
      // "git che" → "git checkout") backspaces leave the earlier text behind.
      if (getBroadcastActive()) {
        const targets = tabStore.getAllBroadcastPanelIds()
        for (const pid of targets) {
          const p = panelStore.getPanel(pid)
          if (p?.sessionId && (p.type === 'ssh' || p.type === 'local' || p.type === 'wsl')) {
            queuedSessionWrite(p.sessionId, '\x15')
            queuedSessionWrite(p.sessionId, item.value)
          }
        }
      } else if (sid) {
        queuedSessionWrite(sid, '\x15')
        queuedSessionWrite(sid, item.value)
      }
      terminalInput.lineBuffer.value = item.value
      terminalInput.cursorIndex.value = item.value.length
      terminalInput.currentToken.value = ''
    }

    suggestions.close()
  }

  return { toShellPath, onNativePathsDropped, sanitizeTerminalHistory, applySuggestion }
}