import test from 'node:test';
import assert from 'node:assert/strict';
import { visibleSessionRows } from './sessionPresentation.js';

test('visibleSessionRows prioritizes search results over the current session list', () => {
  assert.deepEqual(visibleSessionRows({sessionSearch: 'abc', sessionSearchResults: [{id: 'hit'}], sessions: [{id: 'normal'}]}), [{id: 'hit'}]);
  assert.deepEqual(visibleSessionRows({sessions: [{id: 'normal'}]}), [{id: 'normal'}]);
});
