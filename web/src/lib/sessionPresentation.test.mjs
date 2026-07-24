import test from 'node:test';
import assert from 'node:assert/strict';
import { scheduledTaskSessionRows, visibleSessionRows } from './sessionPresentation.js';

test('scheduledTaskSessionRows exposes unique generated conversations', () => {
  const rows = scheduledTaskSessionRows([
    {session_id: 's1', session_title: '生成标题', started_at: 'new'},
    {session_id: 's1', session_title: '旧标题', started_at: 'old'},
    {session_id: 's2', task_title: '任务标题'},
    {session_id: ''},
  ]);
  assert.deepEqual(rows.map(row => [row.session_id, row.session_title, row.started_at]), [
    ['s1', '生成标题', 'new'],
    ['s2', undefined, undefined],
  ]);
});

test('visibleSessionRows prioritizes search results over the current session list', () => {
  assert.deepEqual(visibleSessionRows({sessionSearch: 'abc', sessionSearchResults: [{id: 'hit'}], sessions: [{id: 'normal'}]}), [{id: 'hit'}]);
  assert.deepEqual(visibleSessionRows({sessions: [{id: 'normal'}]}), [{id: 'normal'}]);
});
