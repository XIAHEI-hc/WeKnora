import assert from 'node:assert/strict'
import { test } from 'node:test'

import {
  historyPageMayHaveOlderRows,
  oldestHistoryCursor,
  prependDistinctHistory,
} from './pixlabChatHistory'

test('project chat history uses the oldest row as the next cursor', () => {
  assert.equal(oldestHistoryCursor([
    { id: 'older', created_at: '2026-10-06T10:00:00Z' },
    { id: 'newer', created_at: '2026-10-06T11:00:00Z' },
  ]), '2026-10-06T10:00:00Z')
})

test('project chat history prepends only messages not already rendered', () => {
  const current = [{ id: 'm2' }, { id: 'm3' }]
  const older = [{ id: 'm1' }, { id: 'm2' }]
  assert.deepEqual(prependDistinctHistory(current, older), [{ id: 'm1' }])
})

test('a short project chat page is terminal', () => {
  assert.equal(historyPageMayHaveOlderRows(50, 50), true)
  assert.equal(historyPageMayHaveOlderRows(49, 50), false)
})
