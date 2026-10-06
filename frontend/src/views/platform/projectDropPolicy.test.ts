import assert from 'node:assert/strict'
import { test } from 'node:test'

import { canDropOnProjectKnowledge, isChatFileDropRoute } from './projectDropPolicy'

test('project chat routes never enter the ordinary attachment drop path', () => {
  assert.equal(isChatFileDropRoute('chat'), true)
  assert.equal(isChatFileDropRoute('pixlabProjectChat'), false)
  assert.equal(isChatFileDropRoute('pixlabProjectNewChat'), false)
})

test('project knowledge drop requires the upload capability', () => {
  assert.equal(canDropOnProjectKnowledge('pixlabProjectKnowledge', true, ['read', 'upload']), true)
  assert.equal(canDropOnProjectKnowledge('pixlabProjectKnowledge', true, ['read']), false)
  assert.equal(canDropOnProjectKnowledge('pixlabProjectChat', true, ['upload']), false)
})
