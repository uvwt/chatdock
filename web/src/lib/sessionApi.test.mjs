import assert from 'node:assert/strict';
import test from 'node:test';

import { callMCPAppTool, createSessionRecord, fetchPinned, fetchProviderSystemPrompt, fetchSessions, resolveSessionToolEvent, searchSessions } from './sessionApi.js';

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

test('fetchProviderSystemPrompt uses the provider prompt endpoint', async () => {
  const calls = [];
  await fetchProviderSystemPrompt(captureApi(calls), 'session/1');

  assert.equal(calls[0].path, '/api/sessions/session%2F1/provider-system-prompt');
});

test('MCP App calls are bound to the current session', async () => {
  const calls = [];
  await callMCPAppTool(captureApi(calls), 'session-1', {sourceTool: 'demo__source', name: 'safe', arguments: {value: 1}});
  assert.equal(calls[0].path, '/api/mcp/apps/call');
  assert.deepEqual(JSON.parse(calls[0].options.body), {session_id: 'session-1', source_tool: 'demo__source', name: 'safe', arguments: {value: 1}});
});

test('resolveSessionToolEvent hydrates lazy event once and reuses cache', async () => {
  const calls = [];
  const cache = new Map();
  const api = async path => { calls.push(path); return {event: {id: 'evt-1', details: {data: {mcp_app: {html: '<p>app</p>'}}}}}; };
  const lazy = {id: 'evt-1', details: {lazy: true, session_id: 'session-1', event_id: 'evt-1'}};
  const first = await resolveSessionToolEvent(api, '', lazy, cache);
  const second = await resolveSessionToolEvent(api, '', lazy, cache);
  assert.equal(first.details.data.mcp_app.html, '<p>app</p>');
  assert.equal(second, first);
  assert.equal(calls.length, 1);
});
