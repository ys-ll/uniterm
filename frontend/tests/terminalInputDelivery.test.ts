import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useTerminalInput } from '../src/composables/useTerminalInput'
import { pasteWithScroll } from '../src/utils/terminalPaste'
import { filterTerminalInput, stripDeviceReplies } from '../src/utils/terminalInputFilter'
import { applyMobileCtrl } from '../src/utils/mobileCtrlKey'

// Exercise the composable's actual onData callback without mounting Wails,
// WebGL, and every store used by BaseTerminal.
// After the BaseTerminal split, the onData listener lives in useTerminalDataFlow.
const source = readFileSync(new URL('../src/composables/useTerminalDataFlow.ts', import.meta.url), 'utf8')
const script = source // .ts file — no <script> tag to extract
const ast = ts.createSourceFile('useTerminalDataFlow.ts', script, ts.ScriptTarget.Latest, true)
let callback = ''
function visit(node: ts.Node) {
  if (ts.isBinaryExpression(node) && node.left.getText(ast) === 'onDataDispose' &&
      ts.isCallExpression(node.right) && node.right.expression.getText(ast) === 'terminal.onData') {
    callback = node.right.arguments[0].getText(ast)
  }
  ts.forEachChild(node, visit)
}
visit(ast)
if (!callback) throw new Error('useTerminalDataFlow onData callback not found')
const executable = ts.transpile(`const onData = ${callback}; onData`, {
  target: ts.ScriptTarget.ES2022,
})

function setup() {
  vi.stubGlobal('requestAnimationFrame', () => 0)
  vi.stubGlobal('cancelAnimationFrame', () => {})
  const input = useTerminalInput(null, { mode: 'ssh', sessionId: 'test' })
  const writes: string[] = []
  // The keystroke pipeline in useTerminalDataFlow is:
  //   onData → imeOrder.push → processTerminalData → writeTerminalInput
  // imeOrder (createKeystrokeReorder) reorders IME bursts; without IME input
  // it passes straight through to processTerminalData. processTerminalData
  // updates the local line buffer (handleInput), applies the Backspace
  // line-start guard (issue #814), then writes to the session. We wire the
  // same chain in the VM context so the extracted onData callback exercises
  // the real delivery behaviour.
  const writeTerminalInputFn = (data: string, inAlt: boolean) => {
    const filtered = filterTerminalInput(data, inAlt)
    if (filtered) writes.push(filtered)
  }
  const ctx: Record<string, unknown> = {
    sidNow: 'test', gen: 1,
    getManagedTerminal: () => ({ onDataGeneration: 1 }),
    replySuppressUntil: { value: 0 }, isMobileTouch: false, retryOnEnter: false,
    props: { mode: 'ssh', sessionId: 'test' },
    terminalInput: input,
    suggestions: { isVisible: () => false, close: () => {} },
    setTimeout: () => 0,
    Date,
    getMode: () => 'ssh',
    stripDeviceReplies,
    applyMobileCtrl,
    keyHandler: { writeTerminalInput: writeTerminalInputFn },
    writeTerminalInput: writeTerminalInputFn,
  }
  ctx.processTerminalData = (data: string) => {
    input.handleInput(data)
    const inAlt = input.isInAlternateScreen() ?? false
    // NOTE: the Backspace line-start guard (issue #814, shouldSuppressBackspace)
    // is a local-only addition that upstream removed ("Always deliver Backspace
    // and let the remote line editor enforce its bounds"). These tests verify
    // the upstream delivery behaviour through the imeOrder chain, so the guard
    // is intentionally omitted here. The guard itself is covered by
    // useSmartCopyPaste.test.ts.
    ;(ctx.writeTerminalInput as (d: string, a: boolean) => void)(data, inAlt)
  }
  ctx.imeOrder = {
    push: (data: string) => (ctx.processTerminalData as (d: string) => void)(data),
    dispose: () => {},
  }
  const onData = runInNewContext(executable, ctx) as (data: string) => void
  return { input, writes, onData }
}

afterEach(() => vi.unstubAllGlobals())

describe('BaseTerminal Backspace delivery', () => {
  it.each([
    'clusconf copy -b -w p[1-8]c[0-9]',
    'vim /opt/gridview/agent/etc/output_redis.ini  ',
  ])('forwards every Backspace after native paste: %s', (command) => {
    const { input, writes, onData } = setup()
    // Cmd+V/context-menu paste writes straight to the session, bypassing
    // onData and the suggestion buffer.
    pasteWithScroll({
      bracketedPasteMode: true,
      write: data => writes.push(data),
      scrollToBottom: () => {},
    }, command)
    expect(input.cursorIndex.value).toBe(0)
    for (const _ of command) onData('\x7f')
    expect(writes.slice(1)).toEqual(Array(command.length).fill('\x7f'))
  })

  it('continues forwarding when a partially tracked command reaches local cursor zero', () => {
    const { input, writes, onData } = setup()
    onData('vim ')
    // A pasted path or shell completion adds text the tracker never sees.
    writes.push('/opt/gridview/agent/etc/output_redis.ini')
    for (let i = 0; i < 42; i++) onData('\x7f')
    expect(input.cursorIndex.value).toBe(0)
    expect(writes.slice(2)).toEqual(Array(42).fill('\x7f'))
  })

  it('forwards Backspace after remote shell history recall', () => {
    const { writes, onData } = setup()
    onData('\x1b[A')
    onData('\x7f')
    expect(writes).toEqual(['\x1b[A', '\x7f'])
  })

  it('leaves alternate-screen Backspace delivery intact', () => {
    const { input, writes, onData } = setup()
    input.handleSessionData('\x1b[?1049h')
    onData('\x7f')
    expect(writes).toEqual(['\x7f'])
  })
})
