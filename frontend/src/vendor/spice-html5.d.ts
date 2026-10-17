export class SpiceMainConn {
  constructor(options: { uri: string; password?: string; screen_id?: string })
  state: string
  onerror?: (error: unknown) => void
  onsuccess?: (message: string) => void
  onclipboard?: (text: string) => void
  sendClipboard(text: string): boolean
  stop(): void
}
