import assert from 'node:assert/strict';
import test from 'node:test';

import { createSessionRecord, fetchPinned, fetchSessions, searchSessions } from './sessionApi.js';

function captureApi(calls) {
  return async (path, options = {}) => {
    calls.push({path, options});
    return {};
  };
}

test('session API adds project filters as query parameters', async () => {
  const calls = [];
  const api = captureApi(calls);
  await fetchSessions(api, {limit: 10, cursor: 'c1', projectFilter: 'project-1'});
  await fetchSessions(api, {projectFilter: 'plain'});
  await fetchSessions(api, {projectFilter: 'plain', pinned: true});
  await fetchSessions(api, {projectFilter: 'plain', pinned: false});
  await searchSessions(api, 'hello', {projectFilter: 'project-2'});
  await fetchPinned(api);

  assert.equal(calls[0].path, '/api/sessions?limit=10&cursor=c1&project_id=project-1');
  assert.equal(calls[1].path, '/api/sessions?limit=30&project_id=');
  assert.equal(calls[2].path, '/api/sessions?limit=30&project_id=&pinned=1');
  assert.equal(calls[3].path, '/api/sessions?limit=30&project_id=&pinned=0');
  assert.equal(calls[4].path, '/api/sessions/search?q=hello&limit=30&project_id=project-2');
  assert.equal(calls[5].path, '/api/pinned');
});

test('createSessionRecord only sends project_id for project sessions', async () => {
  const calls = [];
  const api = captureApi(calls);
  await createSessionRecord(api);
  await createSessionRecord(api, {projectID: 'project-1'});

  assert.equal(calls[0].options.body, '{}');
  assert.equal(calls[1].options.body, JSON.stringify({project_id: 'project-1'}));
});
