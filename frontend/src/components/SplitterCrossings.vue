<template>
  <div ref="hostEl" class="splitter-crossings">
    <div
      v-for="(c, i) in crossings"
      :key="i"
      class="corner-grip"
      :style="{ left: c.x + 'px', top: c.y + 'px' }"
      @mousedown="onGripMouseDown($event, c)"
      @mouseenter="setHint(c, true)"
      @mouseleave="setHint(c, false)"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

interface Bar {
  el: HTMLElement
  r: DOMRect
}

interface Crossing {
  x: number
  y: number
  vBar: HTMLElement
  hBar: HTMLElement
}

const hostEl = ref<HTMLElement | null>(null)
const crossings = ref<Crossing[]>([])

let observer: MutationObserver | null = null
let resizeObserver: ResizeObserver | null = null
let raf = 0

/* Drag bars that participate in ＋ junctions. Vertical bars move on the x
   axis, horizontal bars on the y axis; every perpendicular intersecting
   pair gets a four-way grip. Includes the shell panels' resize handles and
   every in-tab splitter (db/es/mongo/redis/drawer/monitor/notes/editor), so
   e.g. a database splitter's bottom T-junction drags both borders. */
const BAR_SELECTOR = [
  '.panel-splitter',
  '.sidebar .resize-handle',
  '.right-panel .resize-handle',
  '.bottom-panel .resize-handle',
  '.db-resizer',
  '.es-resizer',
  '.mongo-resizer',
  '.redis-resizer',
  '.drawer-resizer',
  '.svc-resizer',
  '.notes-divider',
  '.editor-resizer',
].join(', ')

function collectBars(root: HTMLElement): { vBars: Bar[]; hBars: Bar[] } {
  const vBars: Bar[] = []
  const hBars: Bar[] = []
  const containerRect = root.getBoundingClientRect()
  root.querySelectorAll(BAR_SELECTOR).forEach((node) => {
    const el = node as HTMLElement
    const r = el.getBoundingClientRect()
    /* Zero-size = collapsed/hidden (display:none or off-flow): skip */
    if (r.width < 1 && r.height < 1) return
    const rect = {
      left: r.left - containerRect.left,
      right: r.right - containerRect.left,
      top: r.top - containerRect.top,
      bottom: r.bottom - containerRect.top,
    }
    if (r.width <= r.height) vBars.push({ el, r: rect as DOMRect })
    else hBars.push({ el, r: rect as DOMRect })
  })
  return { vBars, hBars }
}

/* Last known pointer position (viewport coords) — lets recompute() re-light
   the hint of a grip that is still under the pointer. */
let lastPointerX = -1
let lastPointerY = -1

function onWindowPointerMove(e: MouseEvent) {
  lastPointerX = e.clientX
  lastPointerY = e.clientY
}

function recompute() {
  const root = hostEl.value?.parentElement
  if (!root) return
  /* A layout shift can destroy or move a grip the pointer rests on — its
     mouseleave then never fires and the accent hint on the two bars it
     joined stays lit forever (reads as a stray colored line across the
     gutter). Drop every hint on each recompute, then re-light the grip the
     pointer is still on: during a drag the layout shifts every frame and no
     mouseenter refires while the pointer stays inside the re-rendered grip
     — without this the two-bar highlight would go dark mid-drag. */
  for (const el of hintedBars) el.classList.remove('hint')
  hintedBars.clear()
  const containerRect = root.getBoundingClientRect()
  const px = lastPointerX - containerRect.left
  const py = lastPointerY - containerRect.top
  const { vBars, hBars } = collectBars(root)
  const found: Crossing[] = []
  /* T-junction bars often only TOUCH edge-to-edge (a sidebar's gutter ends
     exactly where the tab area's splitter begins), so grow each rect by a
     half pixel before the intersection test */
  const EPS = 0.5
  for (const v of vBars) {
    for (const h of hBars) {
      const left = Math.max(v.r.left, h.r.left) - EPS
      const right = Math.min(v.r.right, h.r.right) + EPS
      const top = Math.max(v.r.top, h.r.top) - EPS
      const bottom = Math.min(v.r.bottom, h.r.bottom) + EPS
      if (left < right && top < bottom) {
        found.push({
          x: (left + right) / 2,
          y: (top + bottom) / 2,
          vBar: v.el,
          hBar: h.el,
        })
      }
    }
  }
  crossings.value = found
  /* Grip box: 0.75rem (12px) centered on the crossing, so ±7px covers it. */
  for (const c of crossings.value) {
    if (Math.abs(px - c.x) <= 7 && Math.abs(py - c.y) <= 7) setHint(c, true)
  }
}

function scheduleRecompute() {
  cancelAnimationFrame(raf)
  raf = requestAnimationFrame(recompute)
}

/* Bars currently carrying the hint accent, so recompute() can clear them. */
const hintedBars = new Set<HTMLElement>()

function setHint(c: Crossing, on: boolean) {
  c.vBar.classList.toggle('hint', on)
  c.hBar.classList.toggle('hint', on)
  if (on) {
    hintedBars.add(c.vBar)
    hintedBars.add(c.hBar)
  } else {
    hintedBars.delete(c.vBar)
    hintedBars.delete(c.hBar)
  }
}

/* Hand the same gesture to both bars as synthetic mousedowns carrying the
   grip's real coordinates: every resizer already has a native
   @mousedown="onResizeStart" handler, so a single mechanism works for all
   of them and each adds its own document listeners for its own axis — one
   gesture drags both borders. */
function onGripMouseDown(e: MouseEvent, c: Crossing) {
  e.preventDefault()
  window.dispatchEvent(new CustomEvent('split:resize-start'))
  const init: MouseEventInit = {
    clientX: e.clientX,
    clientY: e.clientY,
    button: 0,
    bubbles: true,
  }
  c.vBar.dispatchEvent(new MouseEvent('mousedown', init))
  c.hBar.dispatchEvent(new MouseEvent('mousedown', init))
}

onMounted(() => {
  recompute()
  observer = new MutationObserver(scheduleRecompute)
  if (hostEl.value?.parentElement) {
    observer.observe(hostEl.value.parentElement, { childList: true, subtree: true, attributes: true, attributeFilter: ['style', 'class'] })
    resizeObserver = new ResizeObserver(scheduleRecompute)
    resizeObserver.observe(hostEl.value.parentElement)
  }
  window.addEventListener('resize', scheduleRecompute)
  window.addEventListener('mousemove', onWindowPointerMove, { passive: true })
})

onUnmounted(() => {
  observer?.disconnect()
  resizeObserver?.disconnect()
  cancelAnimationFrame(raf)
  window.removeEventListener('resize', scheduleRecompute)
  window.removeEventListener('mousemove', onWindowPointerMove)
})
</script>

<style scoped>
/* Overlay covering the whole app; invisible except the grips themselves */
.splitter-crossings {
  position: absolute;
  inset: 0;
  pointer-events: none;
  z-index: 20;
}
.corner-grip {
  position: absolute;
  width: 0.75rem;
  height: 0.75rem;
  margin-left: -0.375rem;
  margin-top: -0.375rem;
  pointer-events: auto;
  /* Four-direction arrow: the grip moves both crossing borders */
  cursor: move;
}
.corner-grip::after {
  content: '';
  position: absolute;
  inset: 0.25rem;
  opacity: 0;
  transition: opacity 0.15s;
  background: var(--accent);
  box-shadow: 0 0 0.375rem var(--accent-glow);
}
.corner-grip:hover::after {
  opacity: 1;
}
</style>
