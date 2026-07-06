import { ElMessage } from 'element-plus'

const CLOSABLE = { showClose: true, duration: 5000, offset: 50 }

export const msg = {
  success(m: string) { ElMessage.success({ message: m, ...CLOSABLE }) },
  error(m: string)   { ElMessage.error({ message: m, ...CLOSABLE }) },
  warning(m: string) { ElMessage.warning({ message: m, ...CLOSABLE }) },
  info(m: string)    { ElMessage.info({ message: m, ...CLOSABLE }) },
}
