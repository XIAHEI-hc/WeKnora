import assert from 'node:assert/strict'
import { test } from 'node:test'

import { isBootstrapMessageForProject } from './bridge'

test('bootstrap message requires the matching nonce and project and a non-empty ticket', () => {
  const valid = {
    type: 'pixlab.bootstrap', version: 1, nonce: 'nonce-1',
    ticket: 'one-time-ticket', project_code: 'PROJECT_P',
  }
  assert.equal(isBootstrapMessageForProject(valid, 'PROJECT_P', 'nonce-1'), true)
  assert.equal(isBootstrapMessageForProject({ ...valid, nonce: 'nonce-2' }, 'PROJECT_P', 'nonce-1'), false)
  assert.equal(isBootstrapMessageForProject({ ...valid, project_code: 'PROJECT_Q' }, 'PROJECT_P', 'nonce-1'), false)
  assert.equal(isBootstrapMessageForProject({ ...valid, ticket: '' }, 'PROJECT_P', 'nonce-1'), false)
  assert.equal(isBootstrapMessageForProject({ ...valid, type: 'weknora.host.init' }, 'PROJECT_P', 'nonce-1'), false)
})
