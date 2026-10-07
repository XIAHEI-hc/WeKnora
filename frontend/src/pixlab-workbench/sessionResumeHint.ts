type SessionHintStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

const KEY_PREFIX = 'weknora:pixlab-workbench:session:'

function key(projectCode: string) {
  return `${KEY_PREFIX}${projectCode}`
}

function browserSessionStorage(): SessionHintStorage | null {
  if (typeof window === 'undefined') return null
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

export function hasWorkbenchSessionHint(
  projectCode: string,
  storage: SessionHintStorage | null = browserSessionStorage(),
) {
  if (!projectCode || !storage) return false
  try {
    return storage.getItem(key(projectCode)) === '1'
  } catch {
    return false
  }
}

export function rememberWorkbenchSession(
  projectCode: string,
  storage: SessionHintStorage | null = browserSessionStorage(),
) {
  if (!projectCode || !storage) return
  try {
    storage.setItem(key(projectCode), '1')
  } catch {
    // Storage can be unavailable in restricted browser contexts.
  }
}

export function forgetWorkbenchSession(
  projectCode: string,
  storage: SessionHintStorage | null = browserSessionStorage(),
) {
  if (!projectCode || !storage) return
  try {
    storage.removeItem(key(projectCode))
  } catch {
    // A stale hint is harmless: resume still fails closed and requests a ticket.
  }
}
