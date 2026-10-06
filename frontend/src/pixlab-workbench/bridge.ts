export interface PixLabBootstrapMessage {
  type: 'pixlab.bootstrap'
  version: 1
  nonce: string
  ticket: string
  project_code: string
}

export function createBootstrapBridge(
  projectCode: string,
  onBootstrap: (message: PixLabBootstrapMessage) => void,
) {
  const nonce = crypto.randomUUID()
  const targetOrigin = window.location.origin

  const receive = (event: MessageEvent) => {
    if (event.origin !== targetOrigin || event.source !== window.parent) return
    const message = event.data as Partial<PixLabBootstrapMessage> | null
    if (!message || message.type !== 'pixlab.bootstrap' || message.version !== 1) return
    if (message.nonce !== nonce || message.project_code !== projectCode || !message.ticket) return
    onBootstrap(message as PixLabBootstrapMessage)
  }

  window.addEventListener('message', receive)
  window.parent.postMessage({ type: 'wk-pixlab.ready', version: 1, nonce }, targetOrigin)

  return () => window.removeEventListener('message', receive)
}

export function notifyParent(type: 'wk-pixlab.authenticated' | 'wk-pixlab.error', nonce: string, code?: string) {
  window.parent.postMessage({ type, version: 1, nonce, ...(code ? { code } : {}) }, window.location.origin)
}
