import { loadWebBootstrap, startBrowserEvents, stopBrowserEvents } from './utils/browserRuntime'
import { setLocale, t } from './i18n'

setLocale('system')
const root = document.getElementById('app')!
root.textContent = t('web.connecting')

try {
  const bootstrap = await loadWebBootstrap()
  // Subscribe before App/terminal initialization can request any snapshot.
  await startBrowserEvents(bootstrap.eventsUrl)
  root.removeAttribute('aria-live')
  await import('./main')
  window.addEventListener('pagehide', stopBrowserEvents)
  window.addEventListener('pageshow', (event) => {
    if (event.persisted) window.location.reload()
  })
} catch (error) {
  root.replaceChildren()
  const message = document.createElement('p')
  message.setAttribute('role', 'alert')
  message.textContent = error instanceof Error ? error.message : t('web.eventsUnavailable')
  const retry = document.createElement('button')
  retry.textContent = t('web.retry')
  retry.onclick = () => window.location.reload()
  const login = document.createElement('a')
  login.href = '/web/login'
  login.textContent = t('web.signIn')
  root.append(message, retry, document.createTextNode(' '), login)
}
