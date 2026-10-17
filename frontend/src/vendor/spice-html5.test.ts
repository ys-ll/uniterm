import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

class Socket extends EventTarget {
  static all: Socket[] = []
  binaryType = 'arraybuffer'
  readyState = 1
  close = vi.fn(() => { this.readyState = 3 })
  send = vi.fn()
  constructor(public url: string, public protocol: string) { super(); Socket.all.push(this) }
}
let SpiceMainConn: any
beforeEach(async () => {
  vi.stubGlobal('WebSocket', Socket)
  vi.stubGlobal('window', { setTimeout, clearTimeout, URL: { revokeObjectURL: vi.fn() } })
  vi.stubGlobal('navigator', { appName: 'Netscape', appVersion: '5' })
  Socket.all = []
  SpiceMainConn = (await import('./spice-html5.js')).SpiceMainConn
})
afterEach(() => { vi.unstubAllGlobals() })

function agentMessage(type: number, payload: Uint8Array) {
  const bytes = new Uint8Array(20 + payload.length)
  const view = new DataView(bytes.buffer)
  view.setUint32(0, 1, true); view.setUint32(4, type, true); view.setUint32(16, payload.length, true)
  bytes.set(payload, 20)
  return bytes
}
function words(...values: number[]) {
  const bytes = new Uint8Array(4 * values.length)
  values.forEach((value, i) => new DataView(bytes.buffer).setUint32(i * 4, value, true))
  return bytes
}

describe('SPICE client integration patches', () => {
  it('opens distinct sibling channels, retains cursor and playback, and closes every socket/media', () => {
    const client = new SpiceMainConn({ uri: 'ws://host/web/remote/spice/session/main', screen_id: 'screen' })
    const channels = new Uint8Array([5, 0, 0, 0, 2, 0, 3, 0, 4, 0, 5, 0, 10, 0])
    client.process_channel_message({ type: 104, data: channels.buffer })
    expect(Socket.all).toHaveLength(6)
    expect(Socket.all.slice(1).every(ws => ws.url.endsWith('/channel') && ws.protocol === 'binary')).toBe(true)
    expect(client.cursor).not.toBe(client.playback)
    const audio = { src: 'blob:audio', pause: vi.fn(), load: vi.fn(), removeAttribute: vi.fn(), remove: vi.fn() }
    client.playback.audio = audio
    client.stop(); client.stop()
    expect(Socket.all.every(ws => ws.close.mock.calls.length === 1)).toBe(true)
    expect(audio.pause).toHaveBeenCalledOnce()
    expect(window.URL.revokeObjectURL).toHaveBeenCalledWith('blob:audio')
  })
  it('negotiates text clipboard, honors agent tokens, and reassembles fragmented UTF-8 data', () => {
    const client = new SpiceMainConn({ uri: 'ws://gateway/spice' })
    try {
      expect(client.sendClipboard('hello')).toBe(false)
      client.agent_connected = true
      client.agent_tokens = 0
      client.handle_agent_data(agentMessage(6, words(0, 1 << 5)).buffer)
      expect(client.sendClipboard('hello 世界')).toBe(true)
      expect(client.agent_msg_queue).toHaveLength(1)
      client.handle_agent_data(agentMessage(8, words(1)).buffer)
      expect(client.agent_msg_queue).toHaveLength(2)
      const offered = new DataView(client.agent_msg_queue[1].data)
      expect(offered.getUint32(4, true)).toBe(4)
      expect(new TextDecoder().decode(client.agent_msg_queue[1].data.slice(24))).toBe('hello 世界')
      client.onclipboard = vi.fn()
      const text = new TextEncoder().encode('guest → browser 🌍'.repeat(300))
      const payload = new Uint8Array(4 + text.length)
      payload.set(words(1)); payload.set(text, 4)
      const message = agentMessage(4, payload)
      for (let offset = 0; offset < message.length; offset += 127) {
        client.process_channel_message({ type: 109, data: message.slice(offset, offset + 127).buffer })
      }
      expect(client.onclipboard).toHaveBeenCalledExactlyOnceWith(new TextDecoder().decode(text))
      client.process_channel_message({ type: 108 })
      expect(client.agent_msg_queue).toHaveLength(0)
      expect(client.sendClipboard('stale')).toBe(false)
    } finally { client.stop() }
  })
  it('rejects oversized agent payloads and propagates sibling errors to the owner', () => {
    const client = new SpiceMainConn({ uri: 'ws://gateway/spice' })
    try {
      const header = agentMessage(4, new Uint8Array())
      new DataView(header.buffer).setUint32(16, 2 << 20, true)
      expect(() => client.handle_agent_data(header.buffer)).toThrow('1 MiB')
      client.process_channel_message({ type: 104, data: new Uint8Array([1, 0, 0, 0, 3, 0]).buffer })
      client.onerror = vi.fn()
      client.inputs.report_error(new Error('input channel closed'))
      expect(client.onerror).toHaveBeenCalledWith(expect.objectContaining({ message: 'input channel closed' }))
    } finally { client.stop() }
  })
})
