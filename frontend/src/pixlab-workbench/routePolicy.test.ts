import assert from 'node:assert/strict'
import { test } from 'node:test'

import { isAllowedProjectRoute, projectRouteBase } from './routePolicy'

test('project route policy allows only the native project knowledge and chat routes', () => {
  const code = 'PROJECT P/研发'
  const base = projectRouteBase(code)

  assert.equal(base, '/projects/PROJECT%20P%2F%E7%A0%94%E5%8F%91')
  assert.equal(isAllowedProjectRoute(base, code), true)
  assert.equal(isAllowedProjectRoute(`${base}/knowledge`, code), true)
  assert.equal(isAllowedProjectRoute(`${base}/chat/new`, code), true)
  assert.equal(isAllowedProjectRoute(`${base}/chat/session-1`, code), true)

  assert.equal(isAllowedProjectRoute(`${base}-other/knowledge`, code), false)
  assert.equal(isAllowedProjectRoute(`${base}/settings`, code), false)
  assert.equal(isAllowedProjectRoute(`${base}/chat/session-1/export`, code), false)
  assert.equal(isAllowedProjectRoute('/platform/knowledge-bases/secret', code), false)
})
