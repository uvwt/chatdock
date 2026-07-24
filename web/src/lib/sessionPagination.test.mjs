import test from 'node:test';
import assert from 'node:assert/strict';
import { mergeSessionPages, normalizeSessionPage, sessionSummaryFromSession, upsertSessionSummary } from './sessionPagination.js';

test('normalizes the current paginated session response', () => {
  assert.throws(() => normalizeSessionPage([{ id: 'legacy' }]), /invalid session page response/);
  assert.deepEqual(normalizeSessionPage({ sessions: [{ id: 'page' }], next_cursor: 'next', has_more: true }), {
    sessions: [{ id: 'page' }],
    nextCursor: 'next',
    hasMore: true,
  });
});

test('merges session pages without duplicate rows', () => {
  assert.deepEqual(mergeSessionPages(
    [{ id: 'a', title: 'old' }, { id: 'b' }],
    [{ id: 'a', title: 'new' }, { id: 'c' }],
  ), [
    { id: 'a', title: 'new' },
    { id: 'b' },
    { id: 'c' },
  ]);
});

test('projects a full session into a compact list summary', () => {
  const summary = sessionSummaryFromSession({
    id: 'session-1',
    title: '分页会话',
    pinned: true,
    messages: [{ role: 'assistant', content: '  最新\n回复  ' }],
  });
  assert.deepEqual(summary, {
    id: 'session-1',
    title: '分页会话',
    pinned: true,
    project_id: '',
    updated_at: undefined,
  });
});

test('upserts refreshed sessions and keeps pinned/newer rows first', () => {
  const items = upsertSessionSummary([
    { id: 'older', title: '旧会话', pinned: false, updated_at: '2026-07-20T10:00:00Z' },
    { id: 'target', title: '旧标题', pinned: false, updated_at: '2026-07-20T11:00:00Z' },
  ], {
    id: 'target',
    title: '新标题',
    pinned: true,
    updated_at: '2026-07-21T10:00:00Z',
    messages: [],
  });
  assert.equal(items.length, 2);
  assert.equal(items[0].id, 'target');
  assert.equal(items[0].title, '新标题');
  assert.equal(items[0].pinned, true);
});
