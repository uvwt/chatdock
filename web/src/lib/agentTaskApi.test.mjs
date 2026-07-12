import assert from 'node:assert/strict';
import test from 'node:test';

import { blockAgentTask } from './agentTaskApi.js';

test('blockAgentTask posts a block summary to the task action route', async () => {
  let request;
  const api = async (path, options) => {
    request = { path, options };
    return { ok: true };
  };

  await blockAgentTask(api, 'tsk/a b', '等待用户确认');

  assert.equal(request.path, '/api/agent-tasks/tsk%2Fa%20b/block');
  assert.equal(request.options.method, 'POST');
  assert.deepEqual(JSON.parse(request.options.body), { summary: '等待用户确认' });
});
