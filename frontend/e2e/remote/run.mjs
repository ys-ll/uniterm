import assert from 'node:assert/strict'
import { readFile, writeFile, mkdir } from 'node:fs/promises'
import path from 'node:path'
import net from 'node:net'
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright-core')
const base = (await readFile(process.env.REMOTE_E2E_URL_FILE || '.scratch/remote-e2e/url', 'utf8')).trim()
const artifacts = path.resolve(process.env.REMOTE_E2E_ARTIFACTS || '.scratch/remote-e2e')
await mkdir(artifacts, { recursive: true })
const logs = []
const log = (message) => { logs.push(message); console.log(message) }
function qmp(execute, args = {}) {
  return new Promise((resolve, reject) => {
    const socket = net.connect(Number(process.env.QMP_PORT || 33926), '127.0.0.1')
    let buffer = '', ready = false
    socket.setTimeout(5000, () => socket.destroy(new Error('QMP timeout')))
    socket.on('error', reject)
    socket.on('data', chunk => {
      buffer += chunk.toString()
      let end
      while ((end = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, end); buffer = buffer.slice(end + 1)
        const message = JSON.parse(line)
        if (message.QMP) socket.write(JSON.stringify({ execute: 'qmp_capabilities' }) + '\n')
        else if (message.error) { reject(new Error(JSON.stringify(message.error))); socket.end() }
        else if ('return' in message && !ready) {
          ready = true; socket.write(JSON.stringify({ execute, arguments: args }) + '\n')
        } else if ('return' in message) { socket.end(); resolve(message.return) }
      }
    })
  })
}
async function guestText() {
  const memory = await qmp('human-monitor-command', { 'command-line': 'xp /4000bx 0xb8000' })
  const bytes = [...memory.matchAll(/0x([a-f0-9]{2})\b/gi)].map(match => parseInt(match[1], 16))
  return bytes.filter((_, i) => i % 2 === 0).map(byte => String.fromCharCode(byte)).join('')
}
async function eventually(check, description) {
  const end = Date.now() + 15000
  while (Date.now() < end) {
    if (await check()) return
    await new Promise(resolve => setTimeout(resolve, 100))
  }
  throw new Error(description)
}
const browser = await chromium.launch({ executablePath: process.env.CHROME_BINARY || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const context = await browser.newContext({ viewport: { width: 1100, height: 800 } })
await context.addCookies([{ name: 'remote-e2e', value: 'test-only', url: base }])
await context.addInitScript(() => Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true }))
const state = async () => (await context.request.get(base + '/e2e/state')).json()
try {
  const baselineVNCClients = (await qmp('query-vnc')).clients.length
  const anonymous = await browser.newContext()
  assert.equal((await anonymous.request.post(base + '/web/remote/vnc', { data: { host: '127.0.0.1', port: 5909 } })).status(), 401)
  await anonymous.close()
  for (const type of ['vnc', 'spice']) {
    const page = await context.newPage()
    page.on('console', message => logs.push(`${type} console ${message.type()}: ${message.text()}`))
    page.on('pageerror', error => logs.push(`${type} PAGE ERROR: ${error.stack}`))
    const sockets = []
    page.on('websocket', ws => sockets.push(ws.url()))
    await page.goto(`${base}/e2e/remote/index.html?type=${type}`)
    const canvas = page.locator('canvas').first()
    await canvas.waitFor({ state: 'visible', timeout: 20000 })
    await page.waitForFunction(() => {
      const canvas = document.querySelector('canvas')
      return canvas && canvas.width > 0 && canvas.height > 0 && canvas.getContext('2d').getImageData(0, 0, canvas.width, canvas.height).data.some((v, i) => i % 4 !== 3 && v > 0)
    })
    await page.screenshot({ path: `${artifacts}/${type}-first-frame.png` })
    log(`${type}: first visible nonblank framebuffer`)
    const originalSession = (await state())[0].id
    await page.evaluate(() => window.dispatchEvent(new Event('e2e:toggle-tab')))
    await page.getByText('Another tab', { exact: true }).waitFor()
    await page.evaluate(() => window.dispatchEvent(new Event('e2e:toggle-tab')))
    await canvas.waitFor({ state: 'visible' })
    assert.equal((await state())[0].id, originalSession)
    log(`${type}: tab cache restored the same protocol connection`)
    await canvas.click({ position: { x: 100, y: 40 } })
    const marker = `${type}15${Date.now().toString().slice(-6)}`
    await page.keyboard.type(marker, { delay: 60 })
    await eventually(async () => (await guestText()).includes(marker), `${type}: keyboard text absent from real guest VGA memory`)
    log(`${type}: keyboard marker ${marker} verified in QEMU VGA memory`)
    await page.screenshot({ path: `${artifacts}/${type}-keyboard.png` })
    await qmp('screendump', { filename: `${artifacts}/${type}-guest.ppm` })
    const protocol = await qmp(type === 'spice' ? 'query-spice' : 'query-vnc')
    await writeFile(`${artifacts}/${type}-protocol.json`, JSON.stringify(protocol, null, 2))
    if (type === 'spice') {
      const channels = protocol.channels.map(channel => channel['channel-type'])
      for (const channel of [1, 2, 3, 4]) assert(channels.includes(channel), `missing SPICE channel ${channel}`)
      log(`spice: real channel types ${channels.join(',')}`)
    }
    // Vite's dev-client control socket is not a graphical protocol connection.
    const graphicalSockets = sockets.filter(url => !new URL(url).searchParams.has('token'))
    assert(graphicalSockets.length > 0 && graphicalSockets.every(url => url.startsWith(base.replace(/^http/, 'ws') + '/web/remote/')), sockets.join('\n'))
    await page.locator('summary').click()
    await page.getByRole('button', { name: 'Paste from browser', exact: true }).click()
    const area = page.locator('textarea')
    await area.fill('manual clipboard fallback')
    await page.getByRole('button', { name: 'Copy text', exact: true }).click()
    assert.equal(await area.evaluate(node => node.selectionEnd - node.selectionStart), 'manual clipboard fallback'.length)
    if (type === 'spice') {
      await page.getByRole('button', { name: 'Send to remote', exact: true }).click()
      await page.getByText('The SPICE guest clipboard agent is not available.', { exact: true }).waitFor()
    }
    log(`${type}: Clipboard API absent — manual copy/paste field works`)
    const first = (await state())[0].id
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('panel:reconnect', { detail: { panelId: 'protocol-e2e' } })))
    await eventually(async () => { const sessions = await state(); return sessions.length === 1 && sessions[0].id !== first }, `${type}: reconnect did not replace session`)
    await canvas.waitFor({ state: 'visible' })
    const beforeRefresh = (await state())[0].id
    await page.reload()
    await canvas.waitFor({ state: 'visible' })
    await eventually(async () => { const sessions = await state(); return sessions.length === 1 && sessions[0].id !== beforeRefresh }, `${type}: refresh leaked session`)
    log(`${type}: reconnect and browser refresh replaced/reclaimed their sessions`)
    assert.equal(await page.evaluate(() => !!window.__remoteClosedDesktop), false)
    await page.close()
    await eventually(async () => (await state()).length === 0, `${type}: closed browser leaked session`)
    await eventually(async () => {
      const protocol = await qmp(type === 'spice' ? 'query-spice' : 'query-vnc')
      return (type === 'spice' ? protocol.channels?.length === 0 : protocol.clients?.length === baselineVNCClients)
    }, `${type}: upstream connections survived browser close`)
    log(`${type}: browser close released its QEMU connections; native VNC client remains connected`)
  }
  log('PASS: real Vue → same-origin Go handler → QEMU VNC/SPICE; no guest audio/clipboard-agent claim')
} catch (error) {
  const page = context.pages().at(-1)
  if (page) {
    await page.screenshot({ path: `${artifacts}/failure.png` }).catch(() => {})
    await writeFile(`${artifacts}/failure.html`, await page.content())
  }
  throw error
} finally {
  await writeFile(`${artifacts}/browser.log`, logs.join('\n'))
  await context.request.post(base + '/e2e/finish').catch(() => {})
  await browser.close()
}
