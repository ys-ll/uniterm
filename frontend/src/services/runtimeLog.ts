import { FrontendLog } from '../../wailsjs/go/main/App'

type Fields = Record<string, unknown>

function send(level: string, tag: string, msg: string, fields?: Fields) {
  try {
    void FrontendLog(level, tag, msg, fields ? JSON.stringify(fields) : '')
  } catch {
    /* diagnostics must never crash the app */
  }
}

export const runtimeLog = {
  debug(tag: string, msg: string, fields?: Fields) { send('DEBUG', tag, msg, fields) },
  info (tag: string, msg: string, fields?: Fields) { send('INFO',  tag, msg, fields) },
  warn (tag: string, msg: string, fields?: Fields) { send('WARN',  tag, msg, fields) },
  error(tag: string, msg: string, fields?: Fields) { send('ERROR', tag, msg, fields) },

  record(name: string, elapsedMs: number, err?: unknown) {
    const msg = err ? String((err as Error)?.message ?? err) : 'ok'
    const level = err ? 'WARN' : 'DEBUG'
    send(level, `bridge.${name}`, msg, { elapsed_ms: elapsedMs })
  },

  install() {
    window.addEventListener('error', (e) => {
      send('ERROR', 'frontend.unhandled', e.message, {
        filename: e.filename,
        lineno: e.lineno,
        colno: e.colno,
        stack: e.error instanceof Error ? e.error.stack : undefined,
      })
    })
    window.addEventListener('unhandledrejection', (e) => {
      const reason = e.reason
      send(
        'ERROR',
        'frontend.unhandledrejection',
        reason instanceof Error ? reason.message : String(reason),
        { stack: reason instanceof Error ? reason.stack : undefined },
      )
    })
  },
}