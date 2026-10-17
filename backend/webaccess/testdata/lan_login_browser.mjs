import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'

const { chromium } = createRequire(import.meta.url)('playwright-core')
const { url, token, instanceId } = JSON.parse(readFileSync(0, 'utf8'))
const browser = await chromium.launch({
  executablePath: process.env.CHROME_PATH,
  headless: true,
  timeout: 15000,
  args: ['--no-proxy-server'],
})
try {
  const page = await browser.newPage()
  page.setDefaultTimeout(10000)
  const login = await page.goto(url, { waitUntil: 'domcontentloaded' })
  assert.equal(login.status(), 200)
  assert.equal(page.url(), `${url}/web/login`, 'login redirect must stay on the advertised origin')
  assert.equal(await page.evaluate(() => window.isSecureContext), false, 'the regression requires insecure LAN HTTP')
  await page.getByLabel('Access token').fill(token)
  const posted = page.waitForRequest(request => request.method() === 'POST' && request.url() === `${url}/web/login`)
  const replied = page.waitForResponse(response => response.request().method() === 'POST' && response.url() === `${url}/web/login`)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  const request = await posted
  // Only record these three headers. Never record the form body, cookies, or a HAR.
  const headers = Object.fromEntries(await Promise.all(
    ['Origin', 'Sec-Fetch-Site', 'Referer'].map(async name => [name, await request.headerValue(name)]),
  ))
  console.log(JSON.stringify(headers))
  const response = await replied
  assert.equal(response.status(), 303, 'native Chrome form POST must log in')
  await page.waitForURL(`${url}/`)
  const bootstrap = await page.evaluate(async () => {
    const response = await fetch('/web/bootstrap')
    return { status: response.status, body: await response.json() }
  })
  assert.equal(bootstrap.status, 200)
  assert.equal(bootstrap.body.instanceId, instanceId, 'the form must authenticate to this instance')
  assert.equal(headers.Origin, url, 'Chrome must preserve the same-origin signal')
  console.log('PASS insecure LAN Chrome form login and authenticated bootstrap')
} catch (error) {
  console.error(String(error).replaceAll(token, '<REDACTED>'))
  process.exitCode = 1
} finally {
  await browser.close()
}
