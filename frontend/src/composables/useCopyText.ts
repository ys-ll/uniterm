import { ElMessage } from 'element-plus'
import { useI18n } from '../i18n'
import { writeClipboard } from './useClipboardWrite'

// Copy text to the OS clipboard with the shared success/failure feedback.
// Used across the tools-tab subcomponents.
export function useCopyText() {
  const { t } = useI18n()
  return async (text: string) => {
    if (!text) return
    if (await writeClipboard(text)) {
      ElMessage.success(t('tools.copied'))
    } else {
      ElMessage.error(t('tools.copyFailed'))
    }
  }
}
