import test from 'node:test';
import assert from 'node:assert/strict';
import { buildQuickActions } from './quickActions.js';

const noop = () => {};
function actionsFor(overrides = {}) {
  return buildQuickActions({
    branchCurrent: noop, busy: false, cloneCurrent: noop, copyCurrentMarkdown: noop, copyText: noop,
    createSession: noop, current: 's1', currentPinned: false, deleteCurrent: noop, exportCurrent: noop,
    inputRef: { current: { focus: noop } }, messagesLength: 2, openSettings: noop, pinCurrent: noop,
    renameCurrent: noop, sendMsg: noop, setThemeState: noop,
    showContextPreview: noop, showSystemPrompt: noop, theme: 'day', ...overrides,
  });
}

test('buildQuickActions exposes stable primary actions', () => {
  const actions = actionsFor();
  assert.deepEqual(actions.slice(0, 3).map(item => item.id), ['focus-input', 'new-session', 'continue']);
  assert.ok(actions.some(item => item.id === 'settings-projects'));
  assert.ok(actions.some(item => item.id === 'settings-automation'));
  assert.ok(actions.some(item => item.id === 'system-prompt'));
});

test('buildQuickActions keeps creation pages available while disabling session-only actions', () => {
  const actions = actionsFor({ current: '', busy: true, messagesLength: 0 });
  assert.equal(actions.find(item => item.id === 'continue').disabled, true);
  assert.equal(actions.find(item => item.id === 'system-prompt').disabled, true);
  assert.equal(actions.find(item => item.id === 'delete-session').disabled, true);
  assert.equal(actions.find(item => item.id === 'branch-session').disabled, true);
});

test('buildQuickActions no longer exposes removed actions', () => {
  const actions = actionsFor();
  assert.equal(actions.find(item => item.id === 'all-sessions'), undefined);
  assert.equal(actions.find(item => item.id === 'plain-sessions'), undefined);
  assert.equal(actions.find(item => item.id === 'copy-diagnostics'), undefined);
});
