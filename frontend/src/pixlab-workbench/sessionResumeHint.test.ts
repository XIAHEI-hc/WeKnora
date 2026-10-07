import assert from 'node:assert/strict'
import test from 'node:test'

import {
  forgetWorkbenchSession,
  hasWorkbenchSessionHint,
  rememberWorkbenchSession,
} from './sessionResumeHint'

function memoryStorage() {
  const values = new Map<string, string>()
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => { values.set(key, value) },
    removeItem: (key: string) => { values.delete(key) },
  }
}

test('tracks resumable sessions independently for each project', () => {
  const storage = memoryStorage()
  assert.equal(hasWorkbenchSessionHint('PROJECT_A', storage), false)

  rememberWorkbenchSession('PROJECT_A', storage)
  assert.equal(hasWorkbenchSessionHint('PROJECT_A', storage), true)
  assert.equal(hasWorkbenchSessionHint('PROJECT_B', storage), false)

  forgetWorkbenchSession('PROJECT_A', storage)
  assert.equal(hasWorkbenchSessionHint('PROJECT_A', storage), false)
})

test('fails open to ticket bootstrap when storage is unavailable', () => {
  assert.equal(hasWorkbenchSessionHint('PROJECT_A', null), false)
  assert.doesNotThrow(() => rememberWorkbenchSession('PROJECT_A', null))
  assert.doesNotThrow(() => forgetWorkbenchSession('PROJECT_A', null))
})
