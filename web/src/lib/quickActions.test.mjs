import test from 'node:test';
import assert from 'node:assert/strict';
import { buildQuickActions } from './quickActions.js';

const noop = () => {};
function actionsFor(overrides = {}) {
  return buildQuickActions({
    busy: false, current: 's1', exportCurrent: noop,
    openSettings: noop, sendMsg: noop, setThemeState: noop,
    showProviderSystemPrompt: noop, taskPanelAvailable: true,
    theme: 'day', toggleTaskPanel: noop, ...overrides,
  });
}

test('buildQuickActions exposes conversation and compact navigation actions', () => {
  const actions = actionsFor();
  assert.deepEqual(actions.map(item => item.id), [
    'continue',
    'provider-system-prompt',
    'export-session',
    'settings',
    'tasks',
    'theme',
  ]);
  assert.deepEqual(actions.map(item => item.group), ['会话', '会话', '会话', '界面', '界面', '界面']);
});

test('buildQuickActions disables unavailable retained actions', () => {
  const actions = actionsFor({ current: '', busy: true });
  assert.equal(actions.find(item => item.id === 'continue').disabled, true);
  assert.equal(actions.find(item => item.id === 'provider-system-prompt').disabled, true);
  assert.equal(actions.find(item => item.id === 'export-session').disabled, true);
  assert.equal(actions.find(item => item.id === 'tasks').disabled, undefined);
  assert.equal(actions.find(item => item.id === 'theme').disabled, undefined);
});

test('buildQuickActions disables task navigation when AgentDock is unavailable', () => {
  const actions = actionsFor({ taskPanelAvailable: false });
  assert.equal(actions.find(item => item.id === 'tasks').disabled, true);
});
