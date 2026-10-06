import assert from 'node:assert/strict'
import { afterEach, test } from 'node:test'

import {
  clearWorkbenchSession,
  createWorkbenchSession,
  deleteChatSession,
  listChatMessages,
  streamChatAnswer,
  uploadDocument,
  WorkbenchApiError,
  type WorkbenchStreamEvent,
} from './api'

const originalFetch = globalThis.fetch

afterEach(() => {
  globalThis.fetch = originalFetch
})

function sseResponse(chunks: string[]) {
  const encoder = new TextEncoder()
  return new Response(new ReadableStream({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
      controller.close()
    },
  }), {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  })
}

test('workbench SSE preserves split frames and completes only on a terminal event', async () => {
  globalThis.fetch = async () => sseResponse([
    'data: {"response_type":"answer","content":"project ","done":false}\n\nda',
    'ta: {"response_type":"answer","content":"answer","done":false}\n\n',
    'data: {"response_type":"complete","content":"","done":true}\n\n',
  ])
  const events: WorkbenchStreamEvent[] = []

  await streamChatAnswer('PROJECT_P', 'session-1', 'question', (event) => events.push(event))

  assert.deepEqual(events.map((event) => event.response_type), ['answer', 'answer', 'complete'])
  assert.equal(events.map((event) => event.content || '').join(''), 'project answer')
})

test('workbench SSE does not swallow errors thrown by the event consumer', async () => {
  globalThis.fetch = async () => sseResponse([
    'data: {"response_type":"error","content":"provider failed","done":true}\n\n',
  ])

  await assert.rejects(
    streamChatAnswer('PROJECT_P', 'session-1', 'question', (event) => {
      if (event.response_type === 'error') throw new Error(event.content)
    }),
    /provider failed/,
  )
})

test('workbench SSE reports a connection that closes before completion', async () => {
  globalThis.fetch = async () => sseResponse([
    'data: {"response_type":"answer","content":"partial","done":false}\n\n',
  ])

  await assert.rejects(
    streamChatAnswer('PROJECT_P', 'session-1', 'question', () => undefined),
    (error: unknown) => error instanceof WorkbenchApiError && error.code === 'STREAM_INTERRUPTED',
  )
})

test('workbench transports stay on the restricted API and attach Cookie and CSRF', async () => {
  const calls: Array<{ url: string; init: RequestInit }> = []
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input)
    calls.push({ url, init })
    if (url === '/api/v1/pixlab-workbench/session') {
      return Response.json({ data: { csrf_token: 'csrf-1', expires_in: 3600 } })
    }
    return Response.json({ data: url.includes('/messages?') ? { messages: [] } : { deleted: true } })
  }

  await createWorkbenchSession('ticket-1', 'PROJECT P')
  await listChatMessages('PROJECT P', 'session/1', 25, '2026-10-06T12:00:00Z')
  await uploadDocument('PROJECT P', new File(['content'], 'notes.txt'), 'folder/notes.txt')
  await deleteChatSession('PROJECT P', 'session/1')

  assert.equal(calls.length, 4)
  for (const call of calls) {
    assert.equal(call.url.startsWith('/api/v1/pixlab-workbench/'), true, call.url)
    assert.equal(call.init.credentials, 'same-origin')
    assert.equal(new Headers(call.init.headers).has('Authorization'), false)
  }
  assert.match(calls[1].url, /before_time=2026-10-06T12%3A00%3A00Z/)
  assert.match(calls[1].url, /projects\/PROJECT%20P\/sessions\/session%2F1\/messages/)
  assert.equal(new Headers(calls[2].init.headers).get('X-CSRF-Token'), 'csrf-1')
  assert.equal(new Headers(calls[3].init.headers).get('X-CSRF-Token'), 'csrf-1')
})

test('clearing the workbench session removes the CSRF credential', async () => {
  const headers: Headers[] = []
  globalThis.fetch = async (input, init = {}) => {
    if (String(input).endsWith('/session')) {
      return Response.json({ data: { csrf_token: 'csrf-2', expires_in: 3600 } })
    }
    headers.push(new Headers(init.headers))
    return Response.json({ data: { deleted: true } })
  }

  await createWorkbenchSession('ticket-2', 'PROJECT_P')
  clearWorkbenchSession()
  await deleteChatSession('PROJECT_P', 'session-1')

  assert.equal(headers[0].has('X-CSRF-Token'), false)
})
