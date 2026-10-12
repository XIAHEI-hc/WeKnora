/**
 * Create a UUID-shaped random identifier in browsers with or without a
 * secure context. The getRandomValues path is important for HTTP development
 * hosts, where crypto.randomUUID() is unavailable.
 */
export function generateUUID(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }

  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    const buf = new Uint8Array(16)
    crypto.getRandomValues(buf)
    buf[6] = (buf[6] & 0x0f) | 0x40
    buf[8] = (buf[8] & 0x3f) | 0x80
    const hex = Array.from(buf, (b) => b.toString(16).padStart(2, '0'))
    return [
      hex[0], hex[1], hex[2], hex[3], '-',
      hex[4], hex[5], '-',
      hex[6], hex[7], '-',
      hex[8], hex[9], '-',
      hex[10], hex[11], hex[12], hex[13], hex[14], hex[15],
    ].join('')
  }

  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}
