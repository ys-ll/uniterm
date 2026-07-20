import { ElMessage } from 'element-plus'

const CLOSABLE = { showClose: true, duration: 5000, offset: 50 }

export const msg = {
  success(m: string) { ElMessage.success({ message: m, ...CLOSABLE }) },
  error(m: string)   { ElMessage.error({ message: m, ...CLOSABLE }) },
  warning(m: string) { ElMessage.warning({ message: m, ...CLOSABLE }) },
  info(m: string)    { ElMessage.info({ message: m, ...CLOSABLE }) },
  // Stays until closed so a long path can be read and copied. The message is
  // wrapped in a span with inline no-drag so the WKWebView hands mouse events
  // back to the DOM instead of initiating a window drag on macOS frameless
  // windows. CSS customClass alone isn't enough — the mousedown lands on a
  // text node inside .el-message__content and Wails walks up to find no-drag;
  // an inline style on the immediate parent is the most reliable target.
  copyable(m: string, type: 'success' | 'info' | 'warning' | 'error' = 'success') {
    const safe = m.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    ElMessage({
      dangerouslyUseHTMLString: true,
      message: `<span style="--wails-draggable:no-drag;user-select:text;-webkit-user-select:text;cursor:text">${safe}</span>`,
      type,
      showClose: true,
      duration: 0,
      offset: 56,
    })
  },
}
