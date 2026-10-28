import type { Ref } from 'vue'
import type { Terminal } from '@xterm/xterm'
import { isMobilePlatform } from '../utils/platform'
import { queuedSessionWriteObserved as queuedSessionWrite } from '../services/terminalCommandWrite'

export interface UseTerminalTouchPanOptions {
  getTerminal: () => Terminal | null
  terminalRef: Ref<HTMLDivElement | undefined>
  getSessionId: () => string | undefined
}

/**
 * Touch panning for mobile devices.
 *
 * xterm v6 scrolls via a VS Code-style custom scrollable element that only
 * handles mouse wheel and scrollbar drags — touch drags do nothing, so on
 * touch devices finger-panning can't scroll the buffer. This composable
 * translates single-finger drags into scrollLines; in the alternate screen it
 * sends arrow keys instead, matching desktop wheel behaviour.
 *
 * The host calls bindTouchEvents() in onMounted and unbindTouchEvents() in
 * onBeforeUnmount; touchPanStop() is called in onUnmounted to cancel inertia.
 */
export function useTerminalTouchPan(options: UseTerminalTouchPanOptions) {
  const { getTerminal, terminalRef, getSessionId } = options

  const isMobileTouch = isMobilePlatform()
  const touchPan = {
    active: false,
    moved: false,
    lastY: 0,
    lastT: 0,
    accum: 0,     // fractional line remainder so slow drags still accumulate
    velocity: 0,  // px/ms for release inertia
    raf: 0,
  }
  let panCellHeight = 16

  function touchPanStop() {
    touchPan.active = false
    if (touchPan.raf) {
      cancelAnimationFrame(touchPan.raf)
      touchPan.raf = 0
    }
  }

  function touchPanBy(lines: number) {
    const terminal = getTerminal()
    if (!terminal || lines === 0) return
    if (terminal.buffer.active.type === 'alternate') {
      const sid = getSessionId()
      if (!sid) return
      const seq = lines > 0 ? '\x1b[B' : '\x1b[A'
      for (let i = 0; i < Math.min(Math.abs(lines), 8); i++) queuedSessionWrite(sid, seq)
    } else {
      terminal.scrollLines(lines)
    }
  }

  function onTerminalTouchStart(e: TouchEvent) {
    if (e.touches.length !== 1) {
      touchPanStop()
      touchPan.moved = false
      return
    }
    touchPanStop()
    touchPan.active = true
    touchPan.moved = false
    touchPan.lastY = e.touches[0].clientY
    touchPan.lastT = performance.now()
    touchPan.velocity = 0
    touchPan.accum = 0
  }

  function onTerminalTouchMove(e: TouchEvent) {
    if (!touchPan.active || e.touches.length !== 1) return
    const y = e.touches[0].clientY
    const now = performance.now()
    const dy = touchPan.lastY - y // finger up => positive => scroll down
    if (!touchPan.moved && Math.abs(dy) < 2) return
    touchPan.moved = true
    e.preventDefault()
    const dt = Math.max(1, now - touchPan.lastT)
    touchPan.velocity = 0.8 * touchPan.velocity + 0.2 * (dy / dt)
    touchPan.lastY = y
    touchPan.lastT = now
    const host = terminalRef.value
    const rows = getTerminal()?.rows || 1
    if (host && rows > 0 && host.clientHeight > 0) panCellHeight = host.clientHeight / rows
    touchPan.accum += dy / panCellHeight
    const lines = Math.trunc(touchPan.accum)
    if (lines !== 0) {
      touchPan.accum -= lines
      touchPanBy(lines)
    }
  }

  function onTerminalTouchEnd() {
    if (!touchPan.active) return
    touchPan.active = false
    if (!touchPan.moved) return
    // Release inertia: keep scrolling with exponential decay.
    let v = touchPan.velocity * 16 // px per frame
    const step = () => {
      if (Math.abs(v) < 1 || !getTerminal()) {
        touchPan.raf = 0
        return
      }
      touchPan.accum += v / panCellHeight
      const lines = Math.trunc(touchPan.accum)
      if (lines !== 0) {
        touchPan.accum -= lines
        touchPanBy(lines)
      }
      v *= 0.94
      touchPan.raf = requestAnimationFrame(step)
    }
    touchPan.raf = requestAnimationFrame(step)
  }

  function onTerminalTouchCancel() {
    touchPanStop()
    touchPan.moved = false
  }

  function bindTouchEvents() {
    if (!isMobileTouch || !terminalRef.value) return
    const el = terminalRef.value
    el.addEventListener('touchstart', onTerminalTouchStart, { passive: true })
    el.addEventListener('touchmove', onTerminalTouchMove, { passive: false })
    el.addEventListener('touchend', onTerminalTouchEnd, { passive: true })
    el.addEventListener('touchcancel', onTerminalTouchCancel, { passive: true })
  }

  function unbindTouchEvents() {
    if (!isMobileTouch || !terminalRef.value) return
    const el = terminalRef.value
    el.removeEventListener('touchstart', onTerminalTouchStart)
    el.removeEventListener('touchmove', onTerminalTouchMove)
    el.removeEventListener('touchend', onTerminalTouchEnd)
    el.removeEventListener('touchcancel', onTerminalTouchCancel)
  }

  return {
    bindTouchEvents,
    unbindTouchEvents,
    touchPanStop,
  }
}