const CHAT_DROP_ROUTE_NAMES = new Set(['chat', 'globalCreatChat', 'kbCreatChat'])

export function isChatFileDropRoute(routeName: unknown) {
  return CHAT_DROP_ROUTE_NAMES.has(String(routeName || ''))
}

export function canDropOnProjectKnowledge(
  routeName: unknown,
  pixlabProject: boolean,
  capabilities: readonly string[] = [],
) {
  return pixlabProject && routeName === 'pixlabProjectKnowledge' && capabilities.includes('upload')
}
