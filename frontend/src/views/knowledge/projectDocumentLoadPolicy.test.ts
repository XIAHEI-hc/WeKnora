import assert from 'node:assert/strict'
import test from 'node:test'

import { shouldReloadDocumentsForTagChange } from './projectDocumentLoadPolicy'

test('suppresses the tag watcher while a knowledge base initializes', () => {
  assert.equal(shouldReloadDocumentsForTagChange([], ['tag-a'], true), false)
})

test('ignores equivalent tag selections', () => {
  assert.equal(shouldReloadDocumentsForTagChange([], [], false), false)
  assert.equal(shouldReloadDocumentsForTagChange(['tag-a'], ['tag-a'], false), false)
})

test('reloads after a user changes the tag selection', () => {
  assert.equal(shouldReloadDocumentsForTagChange(['tag-b'], ['tag-a'], false), true)
  assert.equal(shouldReloadDocumentsForTagChange(['tag-a'], [], false), true)
})
