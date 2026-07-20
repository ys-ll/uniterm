import type { CredentialResult } from '../components/CredentialPrompt.vue'
import type { ConnectionConfig } from '../types/session'
import { useConnectionStore } from '../stores/connectionStore'
import { useI18n } from '../i18n'
import { inject } from 'vue'

type ShowCredentialDialog = (
  title: string,
  subtitle: string,
  fields: ('user' | 'password')[],
  initialUser?: string,
  initialPassword?: string
) => Promise<CredentialResult | null>

function needsCred(cfg: ConnectionConfig): boolean {
  if (cfg.type !== 'ssh' && cfg.type !== 'mosh' && cfg.type !== 'sftp' && cfg.type !== 'ftp') return false
  if ((cfg.type === 'ssh' || cfg.type === 'mosh') && cfg.authType === 'key') return false
  return !cfg.user || !cfg.password
}

// 解析 tunnel 连接的 user/password：需要时弹 credential 对话框，用户取消返回 null。
// 逻辑复刻自 App.vue 中 ensureCredentials 的 tunnel 分支，供无 App.vue 上下文的 tab
// 组件（例如 K8sTabContent）在不复用整个 ensureCredentials 的情况下同样得到凭据。
export function useTunnelCredentials() {
  const connectionStore = useConnectionStore()
  const { t } = useI18n()
  const showCredentialDialog = inject<ShowCredentialDialog>(
    'showCredentialDialog',
    () => Promise.resolve(null)
  )

  async function resolve(tunnelSSHConnId: string): Promise<{ user: string; password: string } | null> {
    if (!tunnelSSHConnId) return { user: '', password: '' }
    const tunnelConn = connectionStore.connections.find(c => c.id === tunnelSSHConnId)
    if (!tunnelConn) return { user: '', password: '' }
    if (!needsCred(tunnelConn)) {
      return { user: tunnelConn.user || '', password: tunnelConn.password || '' }
    }
    const result = await showCredentialDialog(
      t('credential.tunnelTitle'),
      t('credential.tunnelSubtitle', { name: tunnelConn.name }),
      ['user', 'password'],
      tunnelConn.user,
      tunnelConn.password
    )
    if (!result) return null
    const user = result.user || tunnelConn.user || ''
    const password = result.password || tunnelConn.password || ''
    if (result.action === 'save_and_connect') {
      await connectionStore.update(tunnelConn.id, { user, password })
    }
    return { user, password }
  }

  return { resolveTunnelCredentials: resolve }
}
