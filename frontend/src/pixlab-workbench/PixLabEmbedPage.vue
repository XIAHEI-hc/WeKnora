<template>
  <main class="pixlab-embed-page">
    <div v-if="phase !== 'ready'" class="pixlab-embed-state" role="status">
      <span v-if="phase === 'loading'" class="pixlab-embed-spinner" aria-hidden="true" />
      <strong>{{ phase === 'error' ? '无法打开 AI 问答' : '正在连接 AI 问答' }}</strong>
      <p v-if="error">{{ error }}</p>
    </div>
    <template v-else>
      <header class="pixlab-embed-header">
        <strong>{{ context?.project_name || projectCode }}</strong>
        <span>WeKnora AI 问答</span>
      </header>
      <EmbedChatCore
        :session-id="sessionId"
        session-sig=""
        visitor-id=""
        channel-id=""
        token=""
        :agent-id="context?.agent_id || 'builtin-quick-answer'"
        :kb-ids="context ? [context.knowledge_base.id] : []"
        :show-suggested-questions="false"
        :allow-web-search="false"
        :allow-file-upload="false"
        :workbench-project-code="projectCode"
      />
    </template>
  </main>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import EmbedChatCore from '@/views/embed/EmbedChatCore.vue'
import { createChatSession, createWorkbenchSession, getContext, listChatSessions, WorkbenchApiError, type WorkbenchContext } from './api'
import { createBootstrapBridge, notifyParent, type PixLabBootstrapMessage } from './bridge'

function projectCodeFromPath() {
  const encoded = window.location.pathname.match(/\/weknora-workbench\/projects\/([^/]+)/)?.[1]
  if (!encoded) return ''
  try { return decodeURIComponent(encoded) } catch { return '' }
}

const projectCode = projectCodeFromPath()
const phase = ref<'waiting' | 'loading' | 'ready' | 'error'>('waiting')
const error = ref('')
const context = ref<WorkbenchContext | null>(null)
const sessionId = ref('')
let bridge: ReturnType<typeof createBootstrapBridge> | undefined

function friendlyError(cause: unknown) {
  if (cause instanceof WorkbenchApiError) {
    if (cause.code === 'UNAUTHENTICATED') return '登录状态已失效，请返回 PixLab 重新进入。'
    if (cause.code === 'PROJECT_FORBIDDEN') return '你已没有该项目的访问权限。'
    if (cause.code === 'BINDING_NOT_READY' || cause.code === 'BINDING_CHANGED') return '项目知识库尚未准备完成。'
    return cause.message
  }
  return cause instanceof Error ? cause.message : '项目知识服务暂时不可用。'
}

async function bootstrap(message: PixLabBootstrapMessage) {
  if (!projectCode || phase.value === 'loading' || phase.value === 'ready') return
  phase.value = 'loading'
  try {
    await createWorkbenchSession(message.ticket, projectCode)
    context.value = await getContext(projectCode)
    const sessions = await listChatSessions(projectCode)
    const existing = sessions.sessions[0]
    sessionId.value = existing?.id || (await createChatSession(projectCode)).id
    phase.value = 'ready'
    notifyParent('wk-pixlab.authenticated', message.nonce)
  } catch (cause) {
    phase.value = 'error'
    error.value = friendlyError(cause)
    notifyParent('wk-pixlab.error', message.nonce, cause instanceof WorkbenchApiError ? cause.code : 'WORKBENCH_FAILED')
  }
}

onMounted(() => {
  if (!projectCode) { phase.value = 'error'; error.value = '项目地址无效。'; return }
  bridge = createBootstrapBridge(projectCode, (message) => void bootstrap(message))
  bridge.announceReady()
})
onBeforeUnmount(() => bridge?.dispose())
</script>

<style scoped>
.pixlab-embed-page { height: 100vh; display: flex; flex-direction: column; overflow: hidden; background: var(--td-bg-color-container, #fff); color: var(--td-text-color-primary, #1f2937); }
.pixlab-embed-header { display: flex; align-items: baseline; gap: 10px; flex: 0 0 auto; padding: 12px 20px; border-bottom: 1px solid var(--td-component-stroke, #e5e7eb); }
.pixlab-embed-header span { color: var(--td-text-color-secondary, #6b7280); font-size: 12px; }
.pixlab-embed-page :deep(.embed-chat) { min-height: 0; }
.pixlab-embed-state { display: grid; place-items: center; align-content: center; gap: 10px; height: 100%; color: var(--td-text-color-secondary, #6b7280); }
.pixlab-embed-state p { margin: 0; max-width: 520px; text-align: center; font-size: 13px; }
.pixlab-embed-spinner { width: 16px; height: 16px; border: 2px solid #d1d5db; border-top-color: #2563eb; border-radius: 50%; animation: pixlab-spin .8s linear infinite; }
@keyframes pixlab-spin { to { transform: rotate(360deg); } }
</style>
