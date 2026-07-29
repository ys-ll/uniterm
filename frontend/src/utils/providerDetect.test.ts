import { describe, expect, it } from 'vitest'
import { detectProtocol, SUPPORTED_PROTOCOLS, PROTOCOL_LABEL } from './providerDetect'

describe('detectProtocol — well-known providers', () => {
  it('Anthropic official', () => {
    expect(detectProtocol('https://api.anthropic.com')).toBe('anthropic')
    expect(detectProtocol('https://api.anthropic.com/v1')).toBe('anthropic')
  })

  it('OpenAI official', () => {
    expect(detectProtocol('https://api.openai.com/v1')).toBe('openai-compatible')
  })

  it('DeepSeek', () => {
    expect(detectProtocol('https://api.deepseek.com/v1')).toBe('openai-compatible')
  })

  it('Qwen DashScope (compatible-mode)', () => {
    expect(detectProtocol('https://dashscope.aliyuncs.com/compatible-mode/v1')).toBe('openai-compatible')
  })

  it('Moonshot Kimi', () => {
    expect(detectProtocol('https://api.moonshot.cn/v1')).toBe('openai-compatible')
  })

  it('OpenRouter', () => {
    expect(detectProtocol('https://openrouter.ai/api/v1')).toBe('openai-compatible')
  })

  it('Zhipu GLM', () => {
    expect(detectProtocol('https://open.bigmodel.cn/api/paas/v4')).toBe('glm-native')
  })
})

describe('detectProtocol — ambiguous hosts fall through to custom', () => {
  it('localhost', () => {
    expect(detectProtocol('http://localhost:8080/v1')).toBe('custom')
  })

  it('127.0.0.1', () => {
    expect(detectProtocol('http://127.0.0.1:11434/v1')).toBe('custom')
  })

  it('IPv6 loopback', () => {
    expect(detectProtocol('http://[::1]:11434/v1')).toBe('custom')
  })

  it('unknown host (likely a self-hosted proxy)', () => {
    expect(detectProtocol('https://oneapi.example.com/v1')).toBe('custom')
  })
})

describe('detectProtocol — input validation', () => {
  it('empty string', () => {
    expect(detectProtocol('')).toBe('custom')
  })

  it('garbage input (no scheme)', () => {
    expect(detectProtocol('not-a-url')).toBe('custom')
  })

  it('case-insensitive hostname match', () => {
    expect(detectProtocol('https://API.ANTHROPIC.COM')).toBe('anthropic')
    expect(detectProtocol('https://API.DeepSeek.COM/v1')).toBe('openai-compatible')
  })
})

describe('SUPPORTED_PROTOCOLS + PROTOCOL_LABEL are internally consistent', () => {
  it('every protocol in SUPPORTED_PROTOCOLS has a label', () => {
    for (const p of SUPPORTED_PROTOCOLS) {
      expect(PROTOCOL_LABEL[p]).toBeTruthy()
    }
  })

  it('every label key is a supported protocol', () => {
    for (const key of Object.keys(PROTOCOL_LABEL)) {
      expect(SUPPORTED_PROTOCOLS).toContain(key)
    }
  })
})