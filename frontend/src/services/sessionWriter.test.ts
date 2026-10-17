import { beforeEach, describe, expect, it, vi } from 'vitest'
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ SessionWrite: vi.fn(async () => {}) }))
vi.mock('./webTerminalBridge', () => ({ webTerminalBridge: { input: vi.fn(async () => {}), resize: vi.fn(async () => {}) } }))
vi.mock('../utils/webMode', () => ({ isWebMode: vi.fn(() => false), webClientId: () => 'browser-1' }))
import { SessionWrite } from '../../bindings/github.com/ys-ll/uniterm/app'
import { webTerminalBridge } from './webTerminalBridge'
import { isWebMode } from '../utils/webMode'
import { enqueueSessionOperation, queuedSessionWrite, queuedSessionReply, resizeSessionView, setSessionInputSizeProvider } from './sessionWriter'

describe('actual session input writer', () => {
  beforeEach(() => vi.clearAllMocks())
  it('orders human input, source replies and passive resize without reply ownership changes', async () => {
    const calls: string[] = []
    let release!: () => void
    vi.mocked(webTerminalBridge.input).mockImplementationOnce(async () => {
      calls.push('input:start')
      await new Promise<void>(resolve => { release = resolve })
      calls.push('input:end')
    })
    vi.mocked(SessionWrite).mockImplementationOnce(async () => { calls.push('reply') })
    vi.mocked(webTerminalBridge.resize).mockImplementationOnce(async () => { calls.push('resize') })
    setSessionInputSizeProvider('s', () => ({ cols: 100, rows: 31 }))
    queuedSessionWrite('s', 'a')
    queuedSessionReply('s', '\x1b[1;1R')
    const resized = resizeSessionView('s', 80, 24)
    await vi.waitFor(() => expect(calls).toEqual(['input:start']))
    release()
    await resized
    expect(calls).toEqual(['input:start', 'input:end', 'reply', 'resize'])
    expect(webTerminalBridge.input).toHaveBeenCalledWith('s', 'desktop', 'a', 100, 31, false)
    expect(SessionWrite).toHaveBeenCalledWith('s', '\x1b[1;1R')
    expect(webTerminalBridge.resize).toHaveBeenCalledWith('s', 'desktop', 80, 24, false)
  })

  it('uses browser identity, base64 binary input and never replies from a browser', async () => {
    vi.mocked(isWebMode).mockReturnValue(true)
    queuedSessionReply('b', 'query response')
    queuedSessionWrite('b', '\x1b[M\xff !', true)
    await enqueueSessionOperation('b', async () => {})
    expect(SessionWrite).not.toHaveBeenCalled()
    expect(webTerminalBridge.input).toHaveBeenCalledWith('b', 'browser-1', btoa('\x1b[M\xff !'), 0, 0, true)
    vi.mocked(isWebMode).mockReturnValue(false)
  })
})
