<template>
  <div v-if="phase !== 'ready'" class="pixlab-project-state" role="status">
    <span v-if="phase !== 'error'" class="pixlab-project-spinner" aria-hidden="true" />
    <strong>{{ phase === 'error' ? '无法打开项目知识服务' : '正在连接项目知识服务' }}</strong>
    <p v-if="error">{{ error }}</p>
  </div>
  <RouterView v-else />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, provide, ref } from 'vue'
import { useRoute } from 'vue-router'

import {
  clearWorkbenchSession,
  createWorkbenchSession,
  getContext,
  WorkbenchApiError,
  type WorkbenchContext,
} from './api'
import { createBootstrapBridge, notifyParent, type PixLabBootstrapMessage } from './bridge'
import { PIXLAB_PROJECT_CONTEXT, setActiveProjectContext } from './context'

const route = useRoute()
const projectCode = String(route.params.projectCode || '')
const phase = ref<'waiting' | 'loading' | 'ready' | 'error'>('waiting')
const error = ref('')
const context = ref<WorkbenchContext | null>(null)
let disposeBridge: (() => void) | undefined

provide(PIXLAB_PROJECT_CONTEXT, context)

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
  error.value = ''
  try {
    await createWorkbenchSession(message.ticket, projectCode)
    context.value = await getContext(projectCode)
    setActiveProjectContext(context.value)
    phase.value = 'ready'
    notifyParent('wk-pixlab.authenticated', message.nonce)
  } catch (cause) {
    phase.value = 'error'
    error.value = friendlyError(cause)
    notifyParent(
      'wk-pixlab.error',
      message.nonce,
      cause instanceof WorkbenchApiError ? cause.code : 'WORKBENCH_FAILED',
    )
  }
}

onMounted(() => {
  if (!projectCode) {
    phase.value = 'error'
    error.value = '项目地址无效。'
    return
  }
  disposeBridge = createBootstrapBridge(projectCode, (message) => void bootstrap(message))
})

onBeforeUnmount(() => {
  disposeBridge?.()
  setActiveProjectContext(null)
  clearWorkbenchSession()
})
</script>

<style scoped>
.pixlab-project-state {
  display: grid;
  place-items: center;
  align-content: center;
  gap: 10px;
  width: 100%;
  height: 100%;
  color: var(--td-text-color-secondary, #6b7280);
  background: var(--td-bg-color-container, #fff);
}

.pixlab-project-state p {
  max-width: 520px;
  margin: 0;
  font-size: 13px;
  text-align: center;
}

.pixlab-project-spinner {
  width: 18px;
  height: 18px;
  border: 2px solid var(--td-component-stroke, #d1d5db);
  border-top-color: var(--td-brand-color, #0052d9);
  border-radius: 50%;
  animation: pixlab-project-spin 0.8s linear infinite;
}

@keyframes pixlab-project-spin {
  to { transform: rotate(360deg); }
}
</style>
