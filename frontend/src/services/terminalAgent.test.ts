import { describe, it, expect, vi, beforeEach } from 'vitest'

// Use vi.hoisted to allow factory references to variables defined at top level
const { mockSessionWrite } = vi.hoisted(() => {
  const mockSessionWrite = vi.fn().mockResolvedValue(undefined)
  return { mockSessionWrite }
})

// ---- mock wailsjs modules ----
vi.mock('../../wailsjs/runtime', () => ({
  EventsOn: vi.fn(() => () => {}),
}))

vi.mock('../../wailsjs/go/main/App', () => ({
  SessionWrite: mockSessionWrite,
}))

// ---- mock terminal manager (prompt-line capture) ----
const PROMPT = '[root@node140 ~]#'
const mockGetManagedTerminal = vi.fn(() => ({
  terminal: {
    buffer: {
      active: {
        baseY: 0,
        cursorY: 0,
        getLine: (_n: number) => ({
          translateToString: (_trim?: boolean) => PROMPT,
        }),
      },
    },
  },
}))
vi.mock('../services/terminalManager', () => ({
  getManagedTerminal: (...args: any[]) => mockGetManagedTerminal(...(args as [])),
}))

// ---- mock pinia stores ----
const mockPanel = {
  sessionId: 'test-session-id',
  config: { shellPath: '/bin/bash' },
}
const mockGetPanel = vi.fn().mockReturnValue(mockPanel)
const mockGetAILockedPanel = vi.fn().mockReturnValue(null)

const mockActiveTab: { type: string; panelId: string } = {
  type: 'terminal',
  panelId: 'panel-1',
}
const mockTabStore = {
  getAILockedPanel: mockGetAILockedPanel,
  activeTab: mockActiveTab,
}
const mockPanelStore = {
  getPanel: mockGetPanel,
}

vi.mock('../stores/tabStore', () => ({
  useTabStore: vi.fn(() => mockTabStore),
}))
vi.mock('../stores/panelStore', () => ({
  usePanelStore: vi.fn(() => mockPanelStore),
}))

// ---- import after mocks ----
import { watchOutput, executeCommand, truncateOutput } from './terminalAgent'
import type { ExecuteResult, WatchResult } from './terminalAgent'
import { EventsOn } from '../../wailsjs/runtime'

// ---- helpers ----
const MOCK_TIMESTAMP = 1700000000000

function fakeData(sessionId: string, data: string) {
  return { id: sessionId, data }
}

function withMockedTime() {
  const originalNow = Date.now
  const originalRandom = Math.random
  Date.now = vi.fn(() => MOCK_TIMESTAMP)
  Math.random = vi.fn(() => 0)
  return () => {
    Date.now = originalNow
    Math.random = originalRandom
  }
}

describe('truncateOutput', () => {
  it('returns full text when lines <= threshold', () => {
    const text = 'line1\nline2\nline3'
    const result = truncateOutput(text, 2, 2)
    expect(result).toBe(text)
  })

  it('truncates middle when lines > threshold', () => {
    const lines = Array.from({ length: 20 }, (_, i) => `line${i + 1}`)
    const text = lines.join('\n')
    const result = truncateOutput(text, 2, 3)

    expect(result).toContain('line1')
    expect(result).toContain('line2')
    expect(result).not.toContain('line3')
    expect(result).not.toContain('line17')
    expect(result).toContain('line18')
    expect(result).toContain('line19')
    expect(result).toContain('line20')
    expect(result).toContain('截断')
    expect(result).toContain('已省略')
  })

  it('handles edge case: headLines=0', () => {
    const lines = Array.from({ length: 10 }, (_, i) => `line${i + 1}`)
    const text = lines.join('\n')
    const result = truncateOutput(text, 0, 2)

    expect(result).toContain('省略')
    expect(result).toContain('line9')
    expect(result).toContain('line10')
  })

  it('handles edge case: tailLines=0', () => {
    const lines = Array.from({ length: 10 }, (_, i) => `line${i + 1}`)
    const text = lines.join('\n')
    const result = truncateOutput(text, 3, 0)

    expect(result).toContain('line1')
    expect(result).toContain('line3')
    expect(result).toContain('省略')
  })

  it('handles single line input', () => {
    const result = truncateOutput('single', 1, 1)
    expect(result).toBe('single')
  })

  it('handles empty string', () => {
    const result = truncateOutput('', 1, 1)
    expect(result).toBe('')
  })
})

describe('ExecuteResult interface', () => {
  it('has optional timedOut field', () => {
    const result: ExecuteResult = {
      output: 'test',
      exitCode: 0,
      timedOut: false,
    }
    expect(result.timedOut).toBe(false)

    const result2: ExecuteResult = {
      output: 'test',
      exitCode: -1,
      timedOut: true,
    }
    expect(result2.timedOut).toBe(true)

    const result3: ExecuteResult = {
      output: 'test',
      exitCode: 0,
    }
    expect(result3.timedOut).toBeUndefined()
  })
})

describe('watchOutput', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns promise and cleanup', () => {
    const result = watchOutput('session-1', PROMPT, 1000)
    expect(result.promise).toBeInstanceOf(Promise)
    expect(typeof result.cleanup).toBe('function')
  })

  it('resolves when the prompt line reappears after the command', async () => {
    let capturedCallback: ((payload: { id: string; data: string }) => void) | null = null
    vi.mocked(EventsOn).mockImplementation((_eventName, callback) => {
      capturedCallback = callback
      return () => { }
    })

    const { promise } = watchOutput('s1', PROMPT, 5000)

    // Echoed command line, then output, then the prompt returns.
    capturedCallback!(fakeData('s1', `${PROMPT} echo hi\nhi\n${PROMPT} `))

    const result: WatchResult = await promise
    expect(result.timedOut).toBe(false)
    expect(result.output).toContain('hi')
  })

  it('resolves via idle heuristic when prompt differs (e.g. dynamic prompt)', async () => {
    vi.useFakeTimers()
    let capturedCallback: ((payload: { id: string; data: string }) => void) | null = null
    vi.mocked(EventsOn).mockImplementation((_eventName, callback) => {
      capturedCallback = callback
      return () => { }
    })

    const { promise } = watchOutput('s1', PROMPT, 5000)

    capturedCallback!(fakeData('s1', `${PROMPT} echo hi\nhi\n[user@host ~]$ `))
    vi.advanceTimersByTime(800)

    const result: WatchResult = await promise
    expect(result.timedOut).toBe(false)
    expect(result.output).toContain('hi')
    vi.useRealTimers()
  })

  it('does not resolve on the initial prompt alone', async () => {
    vi.useFakeTimers()
    let capturedCallback: ((payload: { id: string; data: string }) => void) | null = null
    vi.mocked(EventsOn).mockImplementation((_eventName, callback) => {
      capturedCallback = callback
      return () => { }
    })

    const { promise } = watchOutput('s1', PROMPT, 1000)

    // Only the prompt so far (no echoed command line before it) → keep waiting.
    capturedCallback!(fakeData('s1', `${PROMPT} `))
    vi.advanceTimersByTime(1000)

    const result: WatchResult = await promise
    expect(result.timedOut).toBe(true)
    vi.useRealTimers()
  })

  it('never resolves early when promptLine is empty (timeout only)', async () => {
    vi.useFakeTimers()
    let capturedCallback: ((payload: { id: string; data: string }) => void) | null = null
    vi.mocked(EventsOn).mockImplementation((_eventName, callback) => {
      capturedCallback = callback
      return () => { }
    })

    const { promise } = watchOutput('s1', '', 1000)

    // Even output that looks like a prompt must not trigger detection.
    capturedCallback!(fakeData('s1', `${PROMPT} echo hi\nhi\n${PROMPT} `))
    vi.advanceTimersByTime(1000)

    const result: WatchResult = await promise
    expect(result.timedOut).toBe(true)
    expect(result.output).toContain('hi')
    vi.useRealTimers()
  })

  it('times out after timeoutMs', async () => {
    vi.useFakeTimers()
    let capturedCallback: ((payload: { id: string; data: string }) => void) | null = null
    vi.mocked(EventsOn).mockImplementation((_eventName, callback) => {
      capturedCallback = callback
      return () => { }
    })

    const { promise } = watchOutput('s1', PROMPT, 1000)

    capturedCallback!(fakeData('s1', 'partial output'))
    vi.advanceTimersByTime(1000)

    const result: WatchResult = await promise
    expect(result.timedOut).toBe(true)
    expect(result.output).toContain('partial output')
    vi.useRealTimers()
  })

  it('ignores events from different sessions', async () => {
    vi.useFakeTimers()
    let capturedCallback: ((payload: { id: string; data: string }) => void) | null = null
    vi.mocked(EventsOn).mockImplementation((_eventName, callback) => {
      capturedCallback = callback
      return () => { }
    })

    const { promise } = watchOutput('s1', PROMPT, 1000)

    capturedCallback!(fakeData('s2', 'wrong session data'))
    vi.advanceTimersByTime(1000)

    const result: WatchResult = await promise
    expect(result.output).toBe('')
    vi.useRealTimers()
  })

  it('cleanup prevents resolution', async () => {
    vi.useFakeTimers()
    vi.mocked(EventsOn).mockImplementation((_eventName, _callback) => {
      return () => { }
    })

    const { promise, cleanup } = watchOutput('s1', PROMPT, 1000)
    cleanup()

    // Should not resolve/resolve with undefined after cleanup
    let resolved = false
    promise.then(() => { resolved = true }).catch(() => { resolved = true })
    vi.advanceTimersByTime(2000)
    // After cleanup, the promise should not settle via the normal path
    // (it's prevented by the resolved flag)
    expect(resolved).toBe(false)
    vi.useRealTimers()
  })
})

describe('executeCommand', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(EventsOn).mockReturnValue(() => {})
    vi.mocked(mockSessionWrite).mockClear()
    vi.mocked(mockGetPanel).mockReturnValue(mockPanel)
    vi.mocked(mockGetAILockedPanel).mockReturnValue(null)
    mockActiveTab.type = 'terminal'
    mockActiveTab.panelId = 'panel-1'
  })

  it('throws when no active session', async () => {
    vi.mocked(mockGetPanel).mockReturnValue(null)
    mockActiveTab.type = 'settings' // not terminal, not workspace

    await expect(executeCommand('ls')).rejects.toThrow('No active terminal session')
  })

  it('sends the command with no injected marker', async () => {
    const restore = withMockedTime()

    let capturedCallback: ((payload: { id: string; data: string }) => void) | null = null
    vi.mocked(EventsOn).mockImplementation((_eventName, callback) => {
      capturedCallback = callback
      return () => {}
    })

    const cmdPromise = executeCommand('echo hello')

    // Should have written to session — command only, no marker echo appended.
    expect(mockSessionWrite).toHaveBeenCalledOnce()
    const writtenArg = mockSessionWrite.mock.calls[0][1]
    expect(writtenArg).toContain('echo hello')
    expect(writtenArg).not.toContain('__AI_DONE_')
    expect(writtenArg).not.toContain('echo "')

    // Wait for async EventsOn to fire (inside Promise constructor = microtask)
    await Promise.resolve()
    expect(capturedCallback).not.toBeNull()

    // Prompt reappears after the echoed command + output → completion.
    capturedCallback!(fakeData('test-session-id', `${PROMPT} echo hello\nhello\n${PROMPT} `))

    const result = await cmdPromise
    expect(result.exitCode).toBe(0)
    expect(result.timedOut).toBe(false)
    expect(typeof result.output).toBe('string')

    restore()
  }, 10000)

  it('returns timedOut=true on timeout', async () => {
    vi.useFakeTimers()
    let capturedCallback: ((payload: { id: string; data: string }) => void) | null = null
    vi.mocked(EventsOn).mockImplementation((_eventName, callback) => {
      capturedCallback = callback
      return () => { }
    })

    const cmdPromise = executeCommand('long-command', 1000, 2, 2)

    // Wait for async EventsOn to capture the callback
    await Promise.resolve()
    expect(capturedCallback).not.toBeNull()

    capturedCallback!(fakeData('test-session-id', 'some output line1\nline2\nline3\nline4\nline5'))
    vi.advanceTimersByTime(1000)

    const result: ExecuteResult = await cmdPromise
    expect(result.exitCode).toBe(-1)
    expect(result.timedOut).toBe(true)
    expect(result.output).toContain('截断')
    expect(result.output).toContain('line1')
    expect(result.output).toContain('line2')
    expect(result.output).toContain('line4')
    expect(result.output).toContain('line5')
    expect(result.output).not.toContain('line3') // truncated middle
    vi.useRealTimers()
  })

  it('truncates long output on success path', async () => {
    const restore = withMockedTime()

    let capturedCallback: ((payload: { id: string; data: string }) => void) | null = null
    vi.mocked(EventsOn).mockImplementation((_eventName, callback) => {
      capturedCallback = callback
      return () => { }
    })

    const lines = Array.from({ length: 10 }, (_, i) => `line${i + 1}`)
    const output = lines.join('\n')

    // headLines=3 so the head keeps the echoed command line + line1 + line2.
    const cmdPromise = executeCommand('some-cmd', 5000, 3, 3)

    // Wait for async EventsOn to capture the callback
    await Promise.resolve()
    expect(capturedCallback).not.toBeNull()

    // Echoed command + long output + returning prompt triggers completion.
    capturedCallback!(fakeData('test-session-id', `${PROMPT} some-cmd\n` + output + `\n${PROMPT} `))

    const result: ExecuteResult = await cmdPromise
    expect(result.exitCode).toBe(0)
    expect(result.timedOut).toBe(false)
    expect(result.output).toContain('截断')
    expect(result.output).toContain('line1')
    expect(result.output).toContain('line2')
    expect(result.output).toContain('line8')
    expect(result.output).toContain('line9')
    expect(result.output).toContain('line10')

    restore()
  })
})
