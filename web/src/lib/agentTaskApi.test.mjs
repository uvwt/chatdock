import assert from 'node:assert/strict';
import test from 'node:test';

import { deleteAgentTask } from './agentTaskApi.js';

test('deleteAgentTask sends DELETE to the task route', async () => {
  let request;
  const api = async (path, options) => {
    request = { path, options };
    return { ok: true };
  };

  await deleteAgentTask(api, 'tsk/a b');

  assert.equal(request.path, '/api/agent-tasks/tsk%2Fa%20b');
  assert.equal(request.options.method, 'DELETE');
});
