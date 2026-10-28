import { type Ref } from 'vue'
import type { ConnectionConfig } from '../../types/session'
import {
  OpenFileDialog,
  OpenFileDialogFiltered,
  OpenPrivateKeyFile,
  OpenKubeconfigFile,
  GetDefaultSSHKeyPaths,
} from '../../../bindings/github.com/ys-ll/uniterm/app'
import { msg } from '../../services/message'
import { backendErrorText } from '../../utils/backendError'

export interface UseConnectionFilePickersReturn {
  selectKeyFile: () => Promise<void>
  selectSqliteFile: () => Promise<void>
  useDefaultKeyPath: () => Promise<void>
  importKeyText: () => Promise<void>
  importKubeconfigText: () => Promise<void>
  pickKubeconfigFile: () => Promise<void>
}

/**
 * Native file-picker dialogs used by the connection form (key file, sqlite
 * database, kubeconfig, default-key cycling). Extracted from ConnectionForm
 * so the parent only wires the buttons to these handlers.
 *
 * `keyContentRevealed` / `k8sConfigRevealed` are flipped to true after a
 * successful import so the paste area expands to show the loaded content.
 */
export function useConnectionFilePickers(
  form: ConnectionConfig,
  keyContentRevealed: Ref<boolean>,
  k8sConfigRevealed: Ref<boolean>,
  t: (key: string) => string
): UseConnectionFilePickersReturn {
  async function selectKeyFile() {
    try {
      const selected = await OpenFileDialog('~/.ssh')
      if (selected) form.keyPath = selected
    } catch (e) {
      console.error('select key file:', e)
    }
  }

  // Pick the local SQLite database file for a sqlite connection; the path is
  // stored in the Host field.
  async function selectSqliteFile() {
    try {
      const selected = await OpenFileDialogFiltered(t('db.sqlitePickTitle'), 'SQLite Database', '*.sqlite;*.sqlite3;*.db;*.db3')
      if (selected) form.host = selected
    } catch (e) {
      console.error('select sqlite file:', e)
    }
  }

  // Fill the key path with a local default private key (~/.ssh/id_ed25519,
  // ~/.ssh/id_rsa, etc.). The backend resolves the home directory and returns
  // only the standard keys that exist, in preference order. Repeated clicks
  // cycle through them (wrapping around); if the field holds a non-default path
  // the first click resets it to the preferred default.
  async function useDefaultKeyPath() {
    try {
      const paths = await GetDefaultSSHKeyPaths()
      if (!paths?.length) return
      const current = (form.keyPath ?? '').trim()
      const idx = paths.indexOf(current)
      form.keyPath = idx >= 0 ? paths[(idx + 1) % paths.length] : paths[0]
    } catch (e) {
      console.error('default key paths:', e)
    }
  }

  async function importKeyText() {
    try {
      const content = await OpenPrivateKeyFile()
      if (content) {
        form.keyContent = content
        keyContentRevealed.value = true
      }
    } catch (e: any) {
      msg.error(backendErrorText(e))
    }
  }

  async function importKubeconfigText() {
    try {
      const content = await OpenKubeconfigFile()
      if (content) {
        form.k8sConfigInline = content
        k8sConfigRevealed.value = true
      }
    } catch (e: any) {
      msg.error(backendErrorText(e))
    }
  }

  async function pickKubeconfigFile() {
    try {
      const selected = await OpenFileDialog('~/.kube')
      if (selected) form.k8sConfigPath = selected
    } catch (e) {
      console.error('pick kubeconfig:', e)
    }
  }

  return { selectKeyFile, selectSqliteFile, useDefaultKeyPath, importKeyText, importKubeconfigText, pickKubeconfigFile }
}