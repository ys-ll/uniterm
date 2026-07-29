// Provider / protocol inference from base URL hostname. Pure function —
// trivially testable, no DOM access.
//
// The mapping is intentionally narrow: well-known hostnames with stable
// conventions. Unknown hosts (including localhost / 127.0.0.1 / self-hosted
// proxies that don't carry a recognizable hostname) return "custom" so
// the user must pick a protocol explicitly.

export type Protocol =
  | 'anthropic'
  | 'openai'
  | 'openai-compatible'
  | 'responses'
  | 'glm-native'
  | 'custom'

export function detectProtocol(baseURL: string): Protocol {
  if (!baseURL) return 'custom'
  let host: string
  try {
    host = new URL(baseURL).hostname.toLowerCase()
  } catch {
    return 'custom'
  }
  if (!host) return 'custom'

  switch (host) {
    // Anthropic and Anthropic-compatible proxies
    case 'api.anthropic.com':
      return 'anthropic'

    // OpenAI official + Responses API
    case 'api.openai.com':
      // api.openai.com supports both Chat Completions and Responses.
      // The user picks explicitly when both are configured; default to
      // openai-compatible (Chat Completions) for backwards compatibility.
      return 'openai-compatible'

    // Chinese / Asian LLM providers — all OpenAI-compatible Chat Completions
    case 'api.deepseek.com':
    case 'api.moonshot.cn':
    case 'openrouter.ai':
    case 'dashscope.aliyuncs.com':
      return 'openai-compatible'

    // Zhipu GLM uses a non-standard path — user must supply the full
    // endpoint (e.g. /api/paas/v4/chat/completions).
    case 'open.bigmodel.cn':
      return 'glm-native'

    // Localhost / self-hosted proxies / unknown → user picks
    case 'localhost':
    case '127.0.0.1':
    case '::1':
      return 'custom'
  }

  // Anything else (custom proxy domains) — let the user pick.
  return 'custom'
}

// All protocols the AI settings UI exposes. Order matches the dropdown
// ordering in AISettings.vue so the user-facing labels stay stable.
export const SUPPORTED_PROTOCOLS: Protocol[] = [
  'anthropic',
  'openai',
  'openai-compatible',
  'responses',
  'glm-native',
  'custom',
]

// Human-readable label per protocol. The UI uses this when surfacing the
// auto-suggestion in AISettings.vue.
export const PROTOCOL_LABEL: Record<Protocol, string> = {
  'anthropic': 'Anthropic',
  'openai': 'OpenAI (Chat Completions)',
  'openai-compatible': 'OpenAI-compatible (DeepSeek / Qwen / Kimi / OneAPI / Ollama / …)',
  'responses': 'OpenAI Responses API',
  'glm-native': 'Zhipu GLM (native)',
  'custom': 'Custom (full URL)',
}