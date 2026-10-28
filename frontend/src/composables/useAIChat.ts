import { computed } from 'vue'
import { ElMessage } from 'element-plus'
import { useAIStore } from '../stores/aiStore'
import { useSettingsStore } from '../stores/settingsStore'
import { useSkillStore } from '../stores/skillStore'
import { useCommandStore } from '../stores/commandStore'
import { useI18n } from '../i18n'
import {
  runAgent,
  approveTool,
  rejectTool,
  continueAgent,
  answerQuestion,
  dismissQuestion,
} from '../services/agent'
import { CancelChatStream } from '../../bindings/github.com/ys-ll/uniterm/app'
import type { AIAttachment, ExecutionMode } from '../types/ai'

/**
 * 占位符替换：先 $1..$9 按空格分词逐个替换，再处理 $ARGUMENTS；
 * 无占位符时把参数追加到末尾。
 */
export function applyArguments(body: string, args: string): string {
  let out = body
  const words = args.trim() === '' ? [] : args.trim().split(/\s+/)
  for (let i = 1; i <= 9; i++) {
    out = out.split(`$${i}`).join(words[i - 1] ?? '')
  }
  if (out.includes('$ARGUMENTS')) {
    out = out.split('$ARGUMENTS').join(args)
  } else if (args.trim() !== '') {
    out = out.trimEnd() + '\n\n' + args
  }
  return out
}

export interface SendPayload {
  text: string
  skillName: string | null
  cmd: { name: string; args: string } | null
  attachments: AIAttachment[]
}

/**
 * AI 对话状态管理与核心动作逻辑（不涉及 DOM）。
 *
 * 消息列表过滤、状态文案、模式/模型标签等派生状态，以及发送、停止、
 * 审批、回答、继续等动作都集中在此。所有 UI/DOM 相关逻辑（输入框、
 * 附件、滚动）留在调用组件中，通过返回值/事件衔接。
 */
export function useAIChat() {
  const aiStore = useAIStore()
  const settingsStore = useSettingsStore()
  const skillStore = useSkillStore()
  const commandStore = useCommandStore()
  const { t } = useI18n()

  /** 过滤后展示的消息：隐藏有 tool_call_id 的 tool 消息，隐藏空 assistant 消息。 */
  const visibleMessages = computed(() => {
    return aiStore.messages.filter((m) => {
      if (m.role === 'tool' && m.tool_call_id) return false
      if (m.role === 'tool' && !m.tool_call_id) return true
      if (m.role !== 'assistant') return true
      const hasPending = aiStore.pendingCommand?.messageId === m.id
      return m.content || m.tool_calls?.length || hasPending || m.needsContinue
    })
  })

  /** 当前状态文案（确认中/等待回答/思考中…）。 */
  const statusText = computed(() => {
    if (aiStore.pendingCommand) return t('ai.confirming')
    if (aiStore.pendingQuestion) return t('ai.awaitingAnswer')
    const key = `ai.${aiStore.status}`
    return t(key) || t('ai.thinking')
  })

  /** 是否忙碌（运行中/待确认/待回答）。 */
  const busy = computed(
    () => aiStore.isRunning || !!aiStore.pendingCommand || !!aiStore.pendingQuestion,
  )

  /** 执行模式标签。 */
  const modeLabel = computed(() => {
    switch (aiStore.mode) {
      case 'bypass':
        return t('ai.bypass')
      case 'confirm_dangerous':
        return t('ai.confirmDangerous')
      case 'confirm_write':
        return t('ai.confirmWrite')
      case 'confirm_all':
        return t('ai.confirmAll')
      default:
        return t('ai.confirmDangerous')
    }
  })

  /** 当前模型名。 */
  const currentModelName = computed(() => {
    const m = settingsStore.settings.ai.models.find(
      (m) => m.id === settingsStore.settings.ai.activeModelId,
    )
    return m?.name || 'Model'
  })

  /** 相对时间格式化（刚刚/N 分钟前…）。 */
  function formatRelativeTime(timestamp: number): string {
    const diff = Date.now() - timestamp
    const seconds = Math.floor(diff / 1000)
    if (seconds < 60) return t('ai.justNow')
    const minutes = Math.floor(seconds / 60)
    if (minutes < 60) return t('ai.minutesAgo', { n: minutes })
    const hours = Math.floor(minutes / 60)
    if (hours < 24) return t('ai.hoursAgo', { n: hours })
    const days = Math.floor(hours / 24)
    if (days < 30) return t('ai.daysAgo', { n: days })
    const months = Math.floor(days / 30)
    if (months < 12) return t('ai.monthsAgo', { n: months })
    const years = Math.floor(months / 12)
    return t('ai.yearsAgo', { n: years })
  }

  /** 切换执行模式。 */
  function setMode(mode: string) {
    aiStore.mode = mode as ExecutionMode
  }

  /**
   * 发送一条用户消息。负责 skill/command 正文组装、入队或直接 runAgent。
   *
   * `onReady` 在校验通过后、enqueue/runAgent 之前调用，供调用方清空输入框
   * 和附件（与原内联实现的时序一致：先清空再发起请求）。调用方另需在
   * Promise resolve 后滚动到底部。
   *
   * 返回 true 表示已处理，false 表示因加载失败而中止（此时不会调用 onReady）。
   */
  async function send(payload: SendPayload, onReady?: () => void): Promise<boolean> {
    const { text, skillName, cmd, attachments } = payload
    if (!text && !skillName && !cmd && attachments.length === 0) return false

    let skillBody = ''
    if (skillName) {
      try {
        skillBody = await skillStore.getBody(skillName)
      } catch (e) {
        console.error('Failed to get skill body:', e)
      }
    }

    let final = text
    if (cmd) {
      let body = ''
      try {
        body = await commandStore.getBody(cmd.name)
      } catch (e) {
        console.error('Failed to get command body:', e)
      }
      final = body ? applyArguments(body, cmd.args) : cmd.args
    }

    // 正文拉取失败时不要静默跑空 user turn，直接提示并终止
    if (cmd && !final.trim()) {
      ElMessage.error(t('ai.commandLoadFailed', { name: cmd.name }))
      return false
    }
    if (!cmd && skillName && !text && !skillBody) {
      ElMessage.error(t('ai.skillLoadFailed', { name: skillName }))
      return false
    }

    if (busy.value) {
      if (cmd) aiStore.addCommandCard(cmd.name, cmd.args)
      aiStore.enqueueMessage(
        cmd ? '' : final,
        skillName || undefined,
        skillBody || undefined,
        cmd ? final : undefined,
        attachments,
      )
      onReady?.()
      return true
    }
    onReady?.()
    if (cmd) aiStore.addCommandCard(cmd.name, cmd.args)
    await runAgent(
      cmd ? '' : final,
      skillName || undefined,
      skillBody || undefined,
      cmd ? final : undefined,
      attachments,
    )
    return true
  }

  /** 停止当前对话/取消待确认命令/取消待回答问题。 */
  function stop() {
    if (aiStore.pendingCommand) {
      const cmd = aiStore.pendingCommand
      aiStore.clearPendingCommand()
      aiStore.addMessage({
        id: `msg-${Date.now()}`,
        role: 'tool',
        content: 'User cancelled this command.',
        tool_call_id: cmd.toolId,
      })
      aiStore.clearQueue()
      return
    }
    if (aiStore.pendingQuestion) {
      dismissQuestion()
      aiStore.clearQueue()
      return
    }
    CancelChatStream().catch(() => {
      /* ignore */
    })
    aiStore.stop()
  }

  /** 审批工具调用。 */
  async function approve(messageId: string) {
    await approveTool(messageId)
  }

  /** 拒绝工具调用。 */
  function reject(messageId: string) {
    rejectTool(messageId)
  }

  /** 回答问题。 */
  function answer(selectedLabels: string[], customText?: string) {
    answerQuestion(selectedLabels, customText)
  }

  /** 忽略问题。 */
  function dismiss() {
    dismissQuestion()
  }

  /** 继续对话（max turns reached）。 */
  async function continueChat() {
    await continueAgent()
  }

  return {
    visibleMessages,
    statusText,
    busy,
    modeLabel,
    currentModelName,
    formatRelativeTime,
    setMode,
    send,
    stop,
    approve,
    reject,
    answer,
    dismiss,
    continueChat,
  }
}