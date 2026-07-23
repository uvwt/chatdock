import test from 'node:test';
import assert from 'node:assert/strict';
import { buildQuickActions } from './quickActions.js';

const noop = () => {};
function actionsFor(overrides = {}) {
  return buildQuickActions({
    branchCurrent: noop, busy: false, cloneCurrent: noop, copyCurrentMarkdown: noop, copyText: noop,
    createSession: noop, current: 's1', currentPinned: false, deleteCurrent: noop, exportCurrent: noop,
    inputRef: { current: { focus: noop } }, messagesLength: 2, openSettings: noop, pinCurrent: noop,
    productDiagnostics: 'diag', projectCount: 1, renameCurrent: noop, sendMsg: noop, setProjectFilter: noop, setThemeState: noop,
    showContextPreview: noop, theme: 'day', ...overrides,
  });
}

test('buildQuickActions exposes stable primary actions', () => {
  const actions = actionsFor();
  assert.deepEqual(actions.slice(0, 5).map(item => item.id), ['focus-input', 'new-session', 'continue', 'all-sessions', 'plain-sessions']);
  assert.ok(actions.some(item => item.id === 'settings-automation'));
  assert.ok(actions.some(item => item.id === 'copy-diagnostics'));
});

test('buildQuickActions marks unavailable actions disabled', () => {
  const actions = actionsFor({ current: '', busy: true, projectCount: 0, messagesLength: 0 });
  assert.equal(actions.find(item => item.id === 'continue').disabled, true);
  assert.equal(actions.find(item => item.id === 'settings-projects').disabled, true);
  assert.equal(actions.find(item => item.id === 'delete-session').disabled, true);
  assert.equal(actions.find(item => item.id === 'branch-session').disabled, true);
});
