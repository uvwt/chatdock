import assert from 'node:assert/strict';
import test from 'node:test';

import { createProject, deleteProject, fetchProjectPromptPreview, fetchProjects, normalizeProjectListResponse, saveGlobalConfig, updateProject } from './settingsApi.js';

function captureApi(calls) {
  return async (path, options = {}) => {
    calls.push({path, options});
    return {};
  };
}

test('project settings API uses the project endpoints', async () => {
  const calls = [];
  const api = captureApi(calls);
  await fetchProjects(api);
  await createProject(api, {name: 'Alpha', prompt: 'P'});
  await updateProject(api, 'p1', {name: 'Beta', prompt: 'Q'});
  await fetchProjectPromptPreview(api, 'p1');
  await deleteProject(api, 'p1');

  assert.equal(calls[0].path, '/api/projects');
  assert.deepEqual(calls[1], {path: '/api/projects', options: {method: 'POST', body: JSON.stringify({name: 'Alpha', prompt: 'P'})}});
  assert.deepEqual(calls[2], {path: '/api/projects/p1', options: {method: 'PUT', body: JSON.stringify({name: 'Beta', prompt: 'Q'})}});
  assert.equal(calls[3].path, '/api/projects/p1/prompt-preview');
  assert.deepEqual(calls[4], {path: '/api/projects/p1', options: {method: 'DELETE'}});
});

test('saveGlobalConfig posts to the global config endpoint', async () => {
  const calls = [];
  await saveGlobalConfig(captureApi(calls), {model: 'gpt-test'});
  assert.deepEqual(calls[0], {path: '/api/config', options: {method: 'POST', body: JSON.stringify({model: 'gpt-test'})}});
});

test('normalizeProjectListResponse requires exact project counts', () => {
  assert.deepEqual(normalizeProjectListResponse({
    projects: [
      {id: 'p1', name: 'Alpha', prompt: '', session_count: 2},
      {id: 'p2', name: 'Beta', prompt: '', session_count: 1},
    ],
    session_count: 4,
    plain_session_count: 1,
  }), {
    projects: [
      {id: 'p1', name: 'Alpha', prompt: '', session_count: 2},
      {id: 'p2', name: 'Beta', prompt: '', session_count: 1},
    ],
    sessionCounts: {all: 4, plain: 1, byProject: {p1: 2, p2: 1}},
  });
  assert.throws(() => normalizeProjectListResponse({projects: []}), /invalid project list response/);
  assert.throws(() => normalizeProjectListResponse({projects: [{id: 'p1'}], session_count: 0, plain_session_count: 0}), /invalid project summary response/);
});
