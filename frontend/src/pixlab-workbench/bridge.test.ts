import assert from 'node:assert/strict'
import { test } from 'node:test'

import { isBootstrapMessageForProject } from './bridge'
import { generateUUID } from '../utils/uuid'

test('bootstrap message requires the matching nonce and project and a non-empty ticket', () => {
  const valid = {
    type: 'pixlab.bootstrap', version: 1, nonce: 'nonce-1',
    ticket: 'one-time-ticket', project_code: 'PROJECT_P',
  }
  assert.equal(isBootstrapMessageForProject(valid, 'PROJECT_P', 'nonce-1'), true)
  assert.equal(isBootstrapMessageForProject({ ...valid, type: 'workbench.bootstrap' }, 'PROJECT_P', 'nonce-1'), true)
  assert.equal(isBootstrapMessageForProject({ ...valid, nonce: 'nonce-2' }, 'PROJECT_P', 'nonce-1'), false)
  assert.equal(isBootstrapMessageForProject({ ...valid, project_code: 'PROJECT_Q' }, 'PROJECT_P', 'nonce-1'), false)
  assert.equal(isBootstrapMessageForProject({ ...valid, ticket: '' }, 'PROJECT_P', 'nonce-1'), false)
  assert.equal(isBootstrapMessageForProject({ ...valid, type: 'weknora.host.init' }, 'PROJECT_P', 'nonce-1'), false)
})

test('UUID fallback produces a v4-shaped nonce for HTTP-compatible bridges', () => {
  const value = generateUUID()
  assert.match(value, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
})
