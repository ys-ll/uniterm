import { useTunnelCredentials } from './useTunnelCredentials'

// 会话级远程抓包凭据缓存：同一主机的账密在本次应用运行期间只弹一次框。
// 仅保存在内存（不落盘、不写回连接存储），下次启动应用需重新输入。
// 不提供清除入口（符合“会话内记住、重启重输”的预期）。
const sessionCache = new Map<string, { user: string; password: string }>()

/**
 * useRemoteCaptureCredentials 提供带会话缓存的远程抓包凭据解析。
 * 必须在组件 setup 内调用（依赖注入的账密弹框）。命中缓存时直接返回；
 * 未命中则委托 useTunnelCredentials().resolveTunnelCredentials 弹框（连接
 * 未保存密码时），成功后写入缓存。用户取消弹框返回 null，调用方应中止。
 */
export function useRemoteCaptureCredentials() {
  const { resolveTunnelCredentials } = useTunnelCredentials()

  async function resolveRemoteCaptureCredentials(connId: string): Promise<{ user: string; password: string } | null> {
    const hit = sessionCache.get(connId)
    if (hit) return { user: hit.user, password: hit.password }

    const cred = await resolveTunnelCredentials(connId)
    if (cred === null) return null

    // 缓存的可能是 key 认证（user/password 为空）或 password 认证，一并记住
    // “已解析”结果，避免同一会话内反复弹框。
    sessionCache.set(connId, { user: cred.user, password: cred.password })
    return cred
  }

  // 认证失败（如密码输错、handshake failed）时清除该主机的会话缓存，让下次
  // 调用重新弹框输入正确凭据，而不是反复复用错误账密。
  function invalidRemoteCaptureCredentials(connId: string) {
    sessionCache.delete(connId)
  }

  return { resolveRemoteCaptureCredentials, invalidRemoteCaptureCredentials }
}
