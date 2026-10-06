import type { InjectionKey, Ref } from 'vue'

import type { WorkbenchContext } from './api'

export const PIXLAB_PROJECT_CONTEXT: InjectionKey<Ref<WorkbenchContext | null>> = Symbol(
  'pixlabProjectContext',
)

let activeContext: WorkbenchContext | null = null

export function setActiveProjectContext(context: WorkbenchContext | null) {
  activeContext = context
}

export function getActiveProjectContext() {
  return activeContext
}

export function isActiveProjectKnowledgeBase(knowledgeBaseId: string) {
  return Boolean(activeContext && activeContext.knowledge_base.id === knowledgeBaseId)
}
