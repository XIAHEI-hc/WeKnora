export const PIXLAB_SESSIONS_CHANGED_EVENT = 'weknora:pixlab-sessions-changed'

export function notifyPixLabSessionsChanged() {
  window.dispatchEvent(new CustomEvent(PIXLAB_SESSIONS_CHANGED_EVENT))
}
