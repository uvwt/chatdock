import test from 'node:test';
import assert from 'node:assert/strict';
import { buildQuickActions } from './quickActions.js';

const noop = () => {};
function actionsFor(overrides = {}) {
  return buildQuickActions({
    busy: false, copyCurrentMarkdown: noop, current: 's1', exportCurrent: noop,
    sendMsg: noop, setThemeState: noop, showContextPreview: noop,
    showSystemPrompt: noop, theme: 'day', ...overrides,
  });
}

test('buildQuickActions only exposes the retained compact action set', () => {
  const actions = actionsFor();
  assert.deepEqual(actions.map(item => item.id), [
    'continue',
    'system-prompt',
    'context-preview',
    'copy-session',
    'export-session',
    'theme',
  ]);
});

test('buildQuickActions disables unavailable retained actions', () => {
  const actions = actionsFor({ current: '', busy: true });
  assert.equal(actions.find(item => item.id === 'continue').disabled, true);
  assert.equal(actions.find(item => item.id === 'system-prompt').disabled, true);
  assert.equal(actions.find(item => item.id === 'context-preview').disabled, true);
  assert.equal(actions.find(item => item.id === 'copy-session').disabled, true);
  assert.equal(actions.find(item => item.id === 'export-session').disabled, true);
  assert.equal(actions.find(item => item.id === 'theme').disabled, undefined);
});
