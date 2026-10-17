import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
vi.mock('../../bindings/github.com/ys-ll/uniterm/app', () => ({ SessionWrite: vi.fn(async () => {}) }))
vi.mock('./webTerminalBridge', () => ({ webTerminalBridge: { input: vi.fn(async () => {}), resize: vi.fn(async () => {}) } }))
vi.mock('../utils/webMode', () => ({ isWebMode: vi.fn(() => false), webClientId: () => 'browser-1' }))
import { SessionWrite } from '../../bindings/github.com/ys-ll/uniterm/app'
import { webTerminalBridge } from './webTerminalBridge'
import { isWebMode } from '../utils/webMode'
import { enqueueSessionOperation, queuedSessionWrite, queuedSessionReply, resizeSessionView, setSessionInputSizeProvider, writeSessionInput } from './sessionWriter'

describe('actual session input writer', () => {
  beforeEach(() => { vi.clearAllMocks(); vi.mocked(isWebMode).mockReturnValue(false) })
  afterEach(() => vi.restoreAllMocks())
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
    setSessionInputSizeProvider('b', () => ({ cols: 0, rows: 0, generation: 1 }))
    queuedSessionReply('b', 'query response')
    queuedSessionWrite('b', '\x1b[M\xff !', true)
    await enqueueSessionOperation('b', async () => {})
    expect(SessionWrite).not.toHaveBeenCalled()
    expect(webTerminalBridge.input).toHaveBeenCalledWith('b', 'browser-1', btoa('\x1b[M\xff !'), 0, 0, true)
    setSessionInputSizeProvider('b')
  })

  it('rejects input offered while syncing and queued input/resize from an earlier snapshot generation', async () => {
    vi.mocked(isWebMode).mockReturnValue(true)
    const errors = vi.spyOn(console, 'error').mockImplementation(() => {})
    let ready = true, generation = 1, cols = 80
    setSessionInputSizeProvider('gated', () => {
      if (!ready) throw new Error('Terminal is syncing')
      return { cols, rows: 24, generation }
    })
    let release!: () => void
    void enqueueSessionOperation('gated', () => new Promise<void>(resolve => { release = resolve }))
    queuedSessionWrite('gated', 'queued before disconnect')
    const resize = resizeSessionView('gated', 90, 25, true).catch(error => String(error))
    ready = false
    queuedSessionWrite('gated', 'typed while syncing')
    await expect(resizeSessionView('gated', 100, 30)).rejects.toThrow('syncing')
    await vi.waitFor(() => expect(release).toBeTypeOf('function'))
    ready = true
    generation++
    cols = 120
    release()
    expect(await resize).toContain('Terminal view changed')
    await enqueueSessionOperation('gated', async () => {})
    expect(webTerminalBridge.input).not.toHaveBeenCalled()
    expect(webTerminalBridge.resize).not.toHaveBeenCalled()
    expect(errors.mock.calls.map(args => String(args[2]))).toEqual(expect.arrayContaining([
      expect.stringContaining('syncing'), expect.stringContaining('input was not sent'),
    ]))
    queuedSessionWrite('gated', 'fresh input')
    await enqueueSessionOperation('gated', async () => {})
    expect(webTerminalBridge.input).toHaveBeenCalledWith('gated', 'browser-1', 'fresh input', 120, 24, false)
    setSessionInputSizeProvider('gated')
  })

  it('requires an attached browser view and rechecks errors before sending queued input', async () => {
    vi.mocked(isWebMode).mockReturnValue(true)
    const errors = vi.spyOn(console, 'error').mockImplementation(() => {})
    queuedSessionWrite('missing', 'unattached')
    await expect(resizeSessionView('missing', 80, 24)).rejects.toThrow('not attached')
    let failed = false
    setSessionInputSizeProvider('failed', () => {
      if (failed) throw new Error('Invalid snapshot')
      return { cols: 80, rows: 24, generation: 1 }
    })
    queuedSessionWrite('failed', 'pending')
    failed = true
    await enqueueSessionOperation('failed', async () => {})
    expect(webTerminalBridge.input).not.toHaveBeenCalled()
    expect(errors.mock.calls.map(args => String(args[2]))).toContainEqual(expect.stringContaining('Invalid snapshot'))
    setSessionInputSizeProvider('failed')
  })

  it('awaits staged-drop preparation before rz, preserves input ordering and measures the current grid', async () => {
    const calls: string[] = []
    let release!: () => void, cols = 80
    setSessionInputSizeProvider('drop', () => ({ cols, rows: 24 }))
    vi.mocked(webTerminalBridge.input).mockImplementation(async (_id, _client, data) => { calls.push(data) })
    const dropped = writeSessionInput('drop', 'rz -be\n', false, async () => {
      calls.push('upload')
      await new Promise<void>(resolve => { release = resolve })
      calls.push('target:stage')
    })
    queuedSessionWrite('drop', 'next key')
    queuedSessionReply('drop', 'protocol reply')
    await vi.waitFor(() => expect(calls).toEqual(['upload']))
    cols = 120
    release()
    await dropped
    await enqueueSessionOperation('drop', async () => {})
    expect(calls).toEqual(['upload', 'target:stage', 'rz -be\n', 'next key'])
    expect(webTerminalBridge.input).toHaveBeenCalledWith('drop', 'desktop', 'rz -be\n', 120, 24, false)
    expect(SessionWrite).toHaveBeenCalledWith('drop', 'protocol reply')
    setSessionInputSizeProvider('drop')
  })

  it('rejects rz if a browser reattaches while the upload is being prepared', async () => {
    vi.mocked(isWebMode).mockReturnValue(true)
    let generation = 1, release!: () => void
    setSessionInputSizeProvider('drop', () => ({ cols: 80, rows: 24, generation }))
    const dropping = writeSessionInput('drop', 'rz -be\n', false,
      () => new Promise<void>(resolve => { release = resolve })).catch(String)
    await vi.waitFor(() => expect(release).toBeTypeOf('function'))
    generation++
    release()
    expect(await dropping).toContain('changed during file preparation')
    expect(webTerminalBridge.input).not.toHaveBeenCalled()
    setSessionInputSizeProvider('drop')
  })
})
