import assert from 'node:assert/strict';
import test from 'node:test';

import { activeAgentTaskCount, agentTaskProgress, agentTaskStatusMeta, normalizeAgentTaskDetail, sortAgentTasks } from './agentTasks.js';

test('normalizes task detail and derives current step', () => {
  const task = normalizeAgentTaskDetail({
    task: {
      id: 'tsk_1',
      title: 'Deploy',
      status: 'active',
      steps: [
        { id: 'check', title: 'Check', status: 'completed' },
        { id: 'deploy', title: 'Deploy', status: 'in_progress' },
      ],
    },
  });
  assert.equal(task.current_step.id, 'deploy');
  assert.equal(task.completed_step_count, 1);
  assert.equal(task.step_count, 2);
  assert.deepEqual(agentTaskProgress(task), { completed: 1, total: 2, percent: 50, text: '1/2' });
});

test('sorts blocked and active tasks before completed tasks', () => {
  const tasks = sortAgentTasks([
    { id: 'done', status: 'completed', updated_at: '2026-07-12T12:00:00Z' },
    { id: 'active', status: 'active', updated_at: '2026-07-12T13:00:00Z' },
    { id: 'blocked', status: 'blocked', updated_at: '2026-07-12T11:00:00Z' },
  ]);
  assert.deepEqual(tasks.map(task => task.id), ['blocked', 'active', 'done']);
  assert.equal(activeAgentTaskCount(tasks), 2);
  assert.equal(agentTaskStatusMeta('blocked').label, '已阻塞');
});

test('uses completion state for step-less completed tasks', () => {
  assert.deepEqual(agentTaskProgress({ status: 'completed' }), { completed: 0, total: 0, percent: 100, text: '已完成' });
});
