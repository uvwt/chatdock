import test from 'node:test';
import assert from 'node:assert/strict';
import { scheduledTaskSessionRows, sessionRowID, unpinnedSessionRows, visibleSessionRows } from './sessionPresentation.js';

test('sessionRowID uses the generated conversation id for scheduled runs', () => {
  assert.equal(sessionRowID({id: 'run-1', session_id: 'session-1'}), 'session-1');
  assert.equal(sessionRowID({id: 'session-2'}), 'session-2');
});

test('scheduledTaskSessionRows exposes unique generated conversations', () => {
  const rows = scheduledTaskSessionRows([
    {session_id: 's1', session_title: '生成标题', started_at: 'new'},
    {session_id: 's1', session_title: '旧标题', started_at: 'old'},
    {session_id: 's2', task_title: '任务标题'},
    {id: 'run-without-session', session_id: ''},
  ]);
  assert.deepEqual(rows.map(row => [row.session_id, row.session_title, row.started_at]), [
    ['s1', '生成标题', 'new'],
    ['s2', undefined, undefined],
  ]);
});

test('unpinnedSessionRows keeps pinned conversations only in the pinned area', () => {
  const rows = [
    {id: 'plain', title: '普通会话'},
    {id: 'server-pinned', pinned: true},
    {session_id: 'feed-pinned', session_title: '刚刚置顶'},
  ];

  assert.deepEqual(unpinnedSessionRows(rows, [{id: 'feed-pinned', pinned: true}]), [
    {id: 'plain', title: '普通会话'},
  ]);
});

test('visibleSessionRows prioritizes search results over the current session list', () => {
  assert.deepEqual(visibleSessionRows({sessionSearch: 'abc', sessionSearchResults: [{id: 'hit'}], sessions: [{id: 'normal'}]}), [{id: 'hit'}]);
  assert.deepEqual(visibleSessionRows({sessions: [{id: 'normal'}]}), [{id: 'normal'}]);
});
