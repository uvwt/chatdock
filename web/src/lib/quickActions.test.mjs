import test from 'node:test';
import assert from 'node:assert/strict';
import { buildQuickActions } from './quickActions.js';

const noop = () => {};
function actionsFor(overrides = {}) {
  return buildQuickActions({
    busy: false, current: 's1', exportCurrent: noop,
    sendMsg: noop, setThemeState: noop,
    showProviderSystemPrompt: noop, theme: 'day', ...overrides,
  });
}

test('buildQuickActions only exposes the retained compact action set', () => {
  const actions = actionsFor();
  assert.deepEqual(actions.map(item => item.id), [
    'continue',
    'provider-system-prompt',
    'export-session',
    'theme',
  ]);
  assert.deepEqual(actions.map(item => item.group), ['会话', '会话', '会话', '界面']);
});

test('buildQuickActions disables unavailable retained actions', () => {
  const actions = actionsFor({ current: '', busy: true });
  assert.equal(actions.find(item => item.id === 'continue').disabled, true);
  assert.equal(actions.find(item => item.id === 'provider-system-prompt').disabled, true);
  assert.equal(actions.find(item => item.id === 'export-session').disabled, true);
  assert.equal(actions.find(item => item.id === 'theme').disabled, undefined);
});
