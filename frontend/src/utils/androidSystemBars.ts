// JS bridge to WailsJSBridge's system-bar background method (Android only —
// the `wails` object is injected by MainActivity via addJavascriptInterface
// and only exposes these on Android builds).
//
// The Android shell pads the web content out of the status bar / camera
// cutout, and the padding area shows whatever color the native side paints
// there. Push the active theme's background so that area matches instead of
// flashing the hard-coded default (issue #1599).
export function setAndroidSystemBarBackground(color: string): void {
  const w = window as unknown as { wails?: { setSystemBarBackground?: (json: string) => void } }
  if (typeof w.wails?.setSystemBarBackground === 'function') {
    w.wails.setSystemBarBackground(JSON.stringify({ color }))
  }
}
