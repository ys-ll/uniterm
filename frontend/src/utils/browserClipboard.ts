import { t } from '../i18n'

let active = false

// HTTP LAN origins cannot use navigator.clipboard. Give the user a real text
// field for the browser's own Copy/Paste command; never fall back to the host.
export function clipboardDialog(mode: 'copy' | 'paste', text = ''): Promise<string | boolean> {
  if (active) return Promise.resolve(mode === 'copy' ? false : '')
  active = true
  return new Promise(resolve => {
    const previous = document.activeElement as HTMLElement | null
    const dialog = document.createElement('dialog')
    dialog.style.cssText = 'width:min(32rem,85vw);padding:1.25rem;border:1px solid var(--border-subtle);border-radius:0.5rem;background:var(--bg-elevated);color:var(--text-primary)'
    dialog.setAttribute('aria-label', t(`web.clipboard${mode === 'copy' ? 'Copy' : 'Paste'}`))
    const hint = document.createElement('p')
    hint.id = 'web-clipboard-hint'
    hint.textContent = t(`web.clipboard${mode === 'copy' ? 'Copy' : 'Paste'}Hint`)
    const input = document.createElement('textarea')
    input.value = text
    input.readOnly = mode === 'copy'
    input.rows = 8
    input.setAttribute('aria-label', t(`web.clipboard${mode === 'copy' ? 'Copy' : 'Paste'}`))
    input.setAttribute('aria-describedby', hint.id)
    input.style.cssText = 'display:block;box-sizing:border-box;width:100%;margin-bottom:1rem;resize:vertical'
    const cancel = document.createElement('button')
    cancel.textContent = t('common.cancel')
    cancel.className = 'btn btn-ghost'
    cancel.onclick = () => dialog.close()
    const confirm = document.createElement('button')
    confirm.textContent = t(mode === 'copy' ? 'web.clipboardCopied' : 'input.paste')
    confirm.className = 'btn btn-primary'
    confirm.onclick = () => dialog.close('confirm')
    dialog.addEventListener('close', () => {
      const confirmed = dialog.returnValue === 'confirm'
      const value = mode === 'copy' ? confirmed : confirmed ? input.value : ''
      dialog.remove()
      active = false
      previous?.focus()
      resolve(value)
    }, { once: true })
    dialog.append(hint, input, cancel, confirm)
    document.body.append(dialog)
    dialog.showModal()
    input.focus()
    if (mode === 'copy') input.select()
  })
}
