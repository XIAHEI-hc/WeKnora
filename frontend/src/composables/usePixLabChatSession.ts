import { nextTick, onMounted, onUnmounted, reactive, ref, watch, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { listChatMessages, stopChatAnswer, streamChatAnswer, WorkbenchApiError, type WorkbenchMessage } from '@/pixlab-workbench/api'
import { useChatStreamHandler } from '@/composables/useChatStreamHandler'
import { useStickyBottomOnResize } from '@/composables/useStickyBottomOnResize'
import { embedToast } from '@/utils/embedToast'
import {
  historyPageMayHaveOlderRows,
  oldestHistoryCursor,
  PIXLAB_CHAT_HISTORY_PAGE_SIZE,
  prependDistinctHistory,
} from './pixlabChatHistory'

type WorkbenchChatMessage = WorkbenchMessage & {
  knowledge_references?: unknown[]
  is_completed?: boolean
  request_id?: string
  isAgentMode?: boolean
  isRagMode?: boolean
}

function mapMessage(message: WorkbenchMessage, knowledgeBaseId: string): WorkbenchChatMessage {
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
      knowledge_base_id: knowledgeBaseId,
      content: citation.content,
      chunk_index: citation.chunk_index,
      source_locators: citation.source_locators,
    })),
  }
}

export function usePixLabChatSession(options: {
  projectCode: string
  sessionId: Ref<string>
  knowledgeBaseId: string
  enabled?: () => boolean
  scrollContainer?: Ref<HTMLElement | null>
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
  const oldestCursor = ref('')
  const scrollContainer = options.scrollContainer ?? ref<HTMLElement | null>(null)
  const userHasScrolledUp = ref(false)
  let streamController: AbortController | undefined
  let loadRevision = 0

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
    if (!scrollContainer.value) return
    userHasScrolledUp.value = !isNearBottom()
    if (scrollContainer.value.scrollTop <= 4) void loadOlderMessages()
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
    if (options.enabled && !options.enabled()) return
    const revision = ++loadRevision
    const sessionId = options.sessionId.value
    if (!options.sessionId.value) {
      messagesList.splice(0)
      oldestCursor.value = ''
      hasMoreHistory.value = false
      historyLoading.value = false
      return
    }
    historyLoading.value = true
    historyLoadingMore.value = false
    try {
      const result = await listChatMessages(
        options.projectCode,
        sessionId,
        PIXLAB_CHAT_HISTORY_PAGE_SIZE,
      )
      if (revision !== loadRevision || sessionId !== options.sessionId.value) return
      const mapped = result.messages.map((message) => mapMessage(message, options.knowledgeBaseId))
      messagesList.splice(0, messagesList.length, ...mapped as unknown as Record<string, unknown>[])
      oldestCursor.value = oldestHistoryCursor(result.messages)
      hasMoreHistory.value = historyPageMayHaveOlderRows(result.messages.length)
      scrollToBottom(true)
    } catch {
      if (revision === loadRevision) embedToast(t('error.streamFailed'))
    } finally {
      if (revision === loadRevision) historyLoading.value = false
    }
  }

  async function loadOlderMessages() {
    if (options.enabled && !options.enabled()) return
    if (historyLoading.value || historyLoadingMore.value || !hasMoreHistory.value) return
    const sessionId = options.sessionId.value
    const revision = loadRevision
    const cursor = oldestCursor.value
    const container = scrollContainer.value
    if (!sessionId || !cursor || !container) {
      hasMoreHistory.value = false
      return
    }
    historyLoadingMore.value = true
    const previousHeight = container.scrollHeight
    try {
      const result = await listChatMessages(
        options.projectCode,
        sessionId,
        PIXLAB_CHAT_HISTORY_PAGE_SIZE,
        cursor,
      )
      if (revision !== loadRevision || sessionId !== options.sessionId.value) return
      const nextCursor = oldestHistoryCursor(result.messages)
      const mapped = result.messages.map((message) => mapMessage(message, options.knowledgeBaseId))
      const distinct = prependDistinctHistory(
        messagesList as unknown as WorkbenchChatMessage[],
        mapped,
      )
      if (!result.messages.length || !nextCursor || nextCursor === cursor || !distinct.length) {
        hasMoreHistory.value = false
        return
      }
      messagesList.unshift(...distinct as unknown as Record<string, unknown>[])
      oldestCursor.value = nextCursor
      hasMoreHistory.value = historyPageMayHaveOlderRows(result.messages.length)
      await nextTick()
      if (scrollContainer.value) {
        scrollContainer.value.scrollTop += scrollContainer.value.scrollHeight - previousHeight
      }
    } catch {
      if (revision === loadRevision) embedToast(t('error.streamFailed'))
    } finally {
      if (revision === loadRevision) historyLoadingMore.value = false
    }
  }

  async function sendMsg(value: string) {
    if (options.enabled && !options.enabled()) return
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
    if (options.enabled && !options.enabled()) return
    streamController?.abort()
    markInFlightAssistantStopped(currentAssistantMessageId.value)
    if (options.sessionId.value && currentAssistantMessageId.value) {
      await stopChatAnswer(options.projectCode, options.sessionId.value, currentAssistantMessageId.value).catch(() => undefined)
    }
    isReplying.value = false
    loading.value = false
  }

  watch(() => options.sessionId.value, () => void loadMessages(), { immediate: true })
  onMounted(() => {
    if (options.enabled && !options.enabled()) return
    loading.value = false
    isReplying.value = false
  })
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
    currentAssistantMessageId,
    shouldRenderAssistantMessage,
    shouldShowGlobalTypingIndicator,
    getUserQuery: (index: number) => String(messagesList[index - 1]?.content || ''),
    handleScroll,
    loadOlderMessages,
    scrollToBottom,
    onClickScrollToBottom: () => { userHasScrolledUp.value = false; scrollToBottom(true) },
    sendMsg,
    handleStopGeneration,
    setSuggestionAttribution: () => undefined,
  }
}
