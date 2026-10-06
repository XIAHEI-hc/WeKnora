import assert from 'node:assert/strict'
import { afterEach, test } from 'node:test'

import { streamChatAnswer, WorkbenchApiError, type WorkbenchStreamEvent } from './api'

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
