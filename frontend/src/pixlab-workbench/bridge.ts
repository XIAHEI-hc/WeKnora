export interface PixLabBootstrapMessage {
  type: 'pixlab.bootstrap'
  version: 1
  nonce: string
  ticket: string
  project_code: string
}

export function isBootstrapMessageForProject(
  value: unknown,
  projectCode: string,
  nonce: string,
): value is PixLabBootstrapMessage {
  if (!value || typeof value !== 'object') return false
  const message = value as Partial<PixLabBootstrapMessage>
  return message.type === 'pixlab.bootstrap' &&
    message.version === 1 &&
    message.nonce === nonce &&
    message.project_code === projectCode &&
    typeof message.ticket === 'string' &&
    message.ticket.length > 0
}

export function createBootstrapBridge(
  projectCode: string,
  onBootstrap: (message: PixLabBootstrapMessage) => void,
) {
  const nonce = crypto.randomUUID()
  const targetOrigin = window.location.origin

  const receive = (event: MessageEvent) => {
    if (event.origin !== targetOrigin || event.source !== window.parent) return
    if (!isBootstrapMessageForProject(event.data, projectCode, nonce)) return
    onBootstrap(event.data)
  }

  window.addEventListener('message', receive)
  return {
    nonce,
    announceReady: () => {
      window.parent.postMessage({ type: 'wk-pixlab.ready', version: 1, nonce }, targetOrigin)
    },
    dispose: () => window.removeEventListener('message', receive),
  }
}

export function notifyParent(type: 'wk-pixlab.authenticated' | 'wk-pixlab.error', nonce: string, code?: string) {
  window.parent.postMessage({ type, version: 1, nonce, ...(code ? { code } : {}) }, window.location.origin)
}
