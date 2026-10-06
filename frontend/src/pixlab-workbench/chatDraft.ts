const DRAFT_KEY = 'weknora:pixlab-project-chat-draft'

export function storeProjectChatDraft(sessionId: string, query: string) {
  sessionStorage.setItem(DRAFT_KEY, JSON.stringify({ sessionId, query }))
}

export function takeProjectChatDraft(sessionId: string) {
  try {
    const value = JSON.parse(sessionStorage.getItem(DRAFT_KEY) || 'null') as {
      sessionId?: string
      query?: string
    } | null
    if (!value || value.sessionId !== sessionId) return ''
    sessionStorage.removeItem(DRAFT_KEY)
    return String(value.query || '').trim()
  } catch {
    sessionStorage.removeItem(DRAFT_KEY)
    return ''
  }
}
