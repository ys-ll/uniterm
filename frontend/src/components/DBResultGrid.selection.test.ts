// @vitest-environment jsdom
// Temporary debug harness: verify cell selection state machine in real DOM.
import { describe, it, expect, beforeAll, afterEach } from 'vitest'
import { createApp, nextTick } from 'vue'
import ElementPlus from 'element-plus'
import DBResultGrid from './DBResultGrid.vue'

// jsdom has no ResizeObserver; el-table needs it.
class RO {
  observe() { /* noop */ }
  unobserve() { /* noop */ }
  disconnect() { /* noop */ }
}
beforeAll(() => {
  ;(globalThis as any).ResizeObserver = RO
})

const rows = [
  { id: 'a', name: 'x' },
  { id: 'b', name: 'y' },
]
const columns = [{ name: 'id' }, { name: 'name' }]

const mountedApps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  mountedApps.splice(0).forEach(a => a.unmount())
})

async function mountGrid(canEdit = false) {
  const root = document.createElement('div')
  document.body.appendChild(root)
  const app = createApp(DBResultGrid, { rows, columns, canEdit })
  mountedApps.push(app)
  app.use(ElementPlus)
  app.mount(root)
  await nextTick()
  await nextTick()
  return root
}

function stubPoint(el: Element) {
  ;(document as any).elementsFromPoint = () => [el]
}

function mousedown(el: Element) {
  el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0, clientX: 5, clientY: 5 }))
}

describe('DBResultGrid selection (debug)', () => {
  it('click cell → active cell class + expand button rendered', async () => {
    const root = await mountGrid()
    const box = root.querySelector('[data-db-c="0"]') as HTMLElement
    expect(box).toBeTruthy()
    stubPoint(box)
    mousedown(box)
    await nextTick()
    await nextTick()

    const td = box.closest('td') as HTMLElement
    console.log('TD CLASS:', td?.className)
    console.log('BUTTON COUNT:', root.querySelectorAll('.cell-expand-btn').length)
    console.log('BOX HTML:', box.outerHTML.slice(0, 400))
    expect(td?.className).toContain('db-cell-selected')
    expect(root.querySelectorAll('.cell-expand-btn').length).toBe(1)
  })

  it('rownum header click → select all cells', async () => {
    const root = await mountGrid()
    const head = root.querySelector('[data-db-headall]') as HTMLElement
    expect(head).toBeTruthy()
    stubPoint(head)
    mousedown(head)
    await nextTick()
    await nextTick()
    const inSel = root.querySelectorAll('.db-cell-in-sel').length
    console.log('IN-SEL COUNT:', inSel)
    expect(inSel).toBe(4)
  })

  it('context menu 内联编辑 → inline editor rendered', async () => {
    const root = await mountGrid(true)
    const box = root.querySelector('[data-db-c="0"]') as HTMLElement
    stubPoint(box)
    box.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 5, clientY: 5 }))
    await nextTick()
    await nextTick()
    const items = [...document.body.querySelectorAll('.menu-item')]
    const inlineItem = items.find(el => el.textContent?.includes('Inline Edit')) as HTMLElement
    console.log('MENU ITEMS:', items.length, 'inline found:', !!inlineItem)
    console.log('CONN MENU IN BODY:', document.body.querySelectorAll('.conn-context-menu').length)
    console.log('COPY-SELECTED IN BODY:', [...document.body.querySelectorAll('.menu-item')].filter(el => el.textContent?.includes('复制所选')).length)
    console.log('ITEM TEXTS:', items.slice(0, 8).map(el => el.textContent?.trim().slice(0, 12)).join('|'))
    expect(inlineItem).toBeTruthy()
    inlineItem.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    await nextTick()
    const editing = root.querySelectorAll('.cell-edit-wrap').length
    console.log('EDIT WRAP COUNT:', editing)
    expect(editing).toBe(1)
  })
})
