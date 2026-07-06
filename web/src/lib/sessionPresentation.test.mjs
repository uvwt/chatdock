import test from 'node:test';
import assert from 'node:assert/strict';
import { scheduledTaskSessionRows, visibleSessionRows } from './sessionPresentation.js';

test('scheduledTaskSessionRows merges run metadata into existing sessions', () => {
  const run = {session_id: 's1', task_title: '日报', status: 'done', started_at: '2026-07-06T12:00:00Z'};
  const rows = scheduledTaskSessionRows({selectedScheduledTaskID: 't1', selectedScheduledTaskRuns: [run], sessions: [{id: 's1', title: '旧标题', preview: 'x'}]});
  assert.equal(rows.length, 1);
  assert.equal(rows[0].title, '旧标题');
  assert.equal(rows[0].preview, '');
  assert.equal(rows[0].scheduled_run, run);
});

test('visibleSessionRows prioritizes search results then task sessions', () => {
  assert.deepEqual(visibleSessionRows({sessionSearch: 'abc', sessionSearchResults: [{id: 'hit'}], sessions: [{id: 'normal'}]}), [{id: 'hit'}]);
  assert.deepEqual(visibleSessionRows({selectedScheduledTaskID: 't1', selectedScheduledTaskSessions: [{id: 'task'}], sessions: [{id: 'normal'}]}), [{id: 'task'}]);
});
