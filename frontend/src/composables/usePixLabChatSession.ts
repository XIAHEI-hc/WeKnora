import { nextTick, onMounted, onUnmounted, reactive, ref, watch, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { listChatMessages, stopChatAnswer, streamChatAnswer, WorkbenchApiError, type WorkbenchMessage } from '@/pixlab-workbench/api'
import { useChatStreamHandler } from '@/composables/useChatStreamHandler'
import { useStickyBottomOnResize } from '@/composables/useStickyBottomOnResize'
import { embedToast } from '@/utils/embedToast'

type WorkbenchChatMessage = WorkbenchMessage & {
  knowledge_references?: unknown[]
  is_completed?: boolean
  request_id?: string
  isAgentMode?: boolean
  isRagMode?: boolean
}

function mapMessage(message: WorkbenchMessage): WorkbenchChatMessage {
  return {
    ...message,
    is_completed: message.completed,
    request_id: message.id,
    isAgentMode: false,
    isRagMode: true,
    knowledge_references: message.citations.map((citation) => ({
      id: citation.id,
      knowledge_id: citation.document_id,
      knowledge_title: citation.document_title,
      knowledge_filename: citation.document_file_name,
      content: citation.content,
      chunk_index: citation.chunk_index,
    })),
  }
}

export function usePixLabChatSession(options: {
  projectCode: string
  sessionId: Ref<string>
  onMessagesChange?: (has: boolean) => void
  onSessionTitle?: (title: string) => void
  onTurnComplete?: (message: Record<string, unknown>) => void
}) {
  const { t } = useI18n()
  const messagesList = reactive<Record<string, unknown>[]>([])
  const loading = ref(false)
  const isReplying = ref(false)
  const historyLoading = ref(true)
  const historyLoadingMore = ref(false)
  const hasMoreHistory = ref(false)
  const isFirstEnter = ref(true)
  const currentAssistantMessageId = ref('')
  const fullContent = ref('')
  const scrollContainer = ref<HTMLElement | null>(null)
  const userHasScrolledUp = ref(false)
  let streamController: AbortController | undefined

  watch(() => messagesList.length, (length) => options.onMessagesChange?.(length > 0), { immediate: true })

  const isNearBottom = () => {
    if (!scrollContainer.value) return true
    const { scrollTop, scrollHeight, clientHeight } = scrollContainer.value
    return scrollHeight - scrollTop - clientHeight < 80
  }

  const scrollToBottom = (force = false) => {
    if (!force && userHasScrolledUp.value) return
    void nextTick(() => {
      if (scrollContainer.value) scrollContainer.value.scrollTop = scrollContainer.value.scrollHeight
    })
  }

  const handleScroll = () => {
    if (scrollContainer.value) userHasScrolledUp.value = !isNearBottom()
  }

  useStickyBottomOnResize(scrollContainer, userHasScrolledUp)

  const {
    shouldRenderAssistantMessage,
    shouldShowGlobalTypingIndicator,
    handleMsgList,
    processStreamChunk,
    prepareForNewOutgoingMessage,
    markInFlightAssistantStopped,
  } = useChatStreamHandler({
    messagesList,
    loading,
    isReplying,
    currentAssistantMessageId,
    fullContent,
    isAgentStreamSession: () => false,
    scrollToBottom,
    onReplyComplete: () => undefined,
    onTurnComplete: options.onTurnComplete,
    onAfterMsgList: () => undefined,
    onError: embedToast,
    isFirstEnter,
    scrollContainer,
  })

  async function loadMessages() {
    if (!options.sessionId.value) {
      messagesList.splice(0)
      historyLoading.value = false
      return
    }
    historyLoading.value = true
    try {
      const result = await listChatMessages(options.projectCode, options.sessionId.value)
      messagesList.splice(0, messagesList.length, ...result.messages.map((message) => mapMessage(message) as unknown as Record<string, unknown>))
      hasMoreHistory.value = false
      scrollToBottom(true)
    } catch {
      embedToast(t('error.streamFailed'))
    } finally {
      historyLoading.value = false
    }
  }

  async function sendMsg(value: string) {
    const query = value.trim()
    if (!query || !options.sessionId.value || isReplying.value) return
    prepareForNewOutgoingMessage()
    const now = new Date().toISOString()
    messagesList.push({ role: 'user', content: query, created_at: now, id: `pixlab-user-${Date.now()}` })
    isReplying.value = true
    loading.value = true
    scrollToBottom(true)
    streamController?.abort()
    streamController = new AbortController()
    try {
      await streamChatAnswer(options.projectCode, options.sessionId.value, query, (event) => {
        if (event.assistant_message_id) currentAssistantMessageId.value = event.assistant_message_id
        // The workbench API deliberately has a smaller event envelope. Add the
        // stable message id expected by WeKnora's native stream renderer.
        processStreamChunk({
          ...event,
          id: event.id || event.assistant_message_id || currentAssistantMessageId.value || undefined,
          is_completed: event.response_type === 'complete' || (event.response_type === 'answer' && event.done === true),
        } as unknown as Record<string, unknown>)
        if (event.response_type === 'session_title' && event.content) options.onSessionTitle?.(event.content)
        if (event.response_type === 'error') throw new Error(event.content || t('error.streamFailed'))
      }, streamController.signal)
      await loadMessages()
    } catch (error) {
      if ((error as Error).name !== 'AbortError') {
        const message = error instanceof WorkbenchApiError ? error.message : t('error.streamFailed')
        const assistant = [...messagesList].reverse().find((item) => item.role === 'assistant')
        if (assistant) {
          assistant.content = message
          assistant.is_completed = true
        } else {
          messagesList.push({ role: 'assistant', content: message, is_completed: true, created_at: new Date().toISOString() })
        }
        embedToast(message)
      }
    } finally {
      isReplying.value = false
      loading.value = false
      streamController = undefined
      scrollToBottom(true)
    }
  }

  async function handleStopGeneration() {
    streamController?.abort()
    markInFlightAssistantStopped(currentAssistantMessageId.value)
    if (options.sessionId.value && currentAssistantMessageId.value) {
      await stopChatAnswer(options.projectCode, options.sessionId.value, currentAssistantMessageId.value).catch(() => undefined)
    }
    isReplying.value = false
    loading.value = false
  }

  watch(() => options.sessionId.value, () => void loadMessages(), { immediate: true })
  onMounted(() => { loading.value = false; isReplying.value = false })
  onUnmounted(() => streamController?.abort())

  return {
    messagesList,
    loading,
    isReplying,
    historyLoading,
    historyLoadingMore,
    hasMoreHistory,
    scrollContainer,
    userHasScrolledUp,
    shouldRenderAssistantMessage,
    shouldShowGlobalTypingIndicator,
    getUserQuery: (index: number) => String(messagesList[index - 1]?.content || ''),
    handleScroll,
    scrollToBottom,
    onClickScrollToBottom: () => { userHasScrolledUp.value = false; scrollToBottom(true) },
    sendMsg,
    handleStopGeneration,
    setSuggestionAttribution: () => undefined,
  }
}
