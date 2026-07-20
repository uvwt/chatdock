import test from 'node:test';
import assert from 'node:assert/strict';
import { agentTaskDataEnabled, fmtBytes, fmtDuration, logoutAndReload, normalizeSettingsModule, runStatusClass, runStatusLabel, setSettingsDocumentScroll, taskStatusClass, taskStatusLabel } from './appUtils.js';

test('format helpers keep compact Chinese UI labels', () => {
  assert.equal(fmtBytes(1536), '1.5 KB');
  assert.equal(fmtDuration(1500), '1.5s');
  assert.equal(runStatusLabel('failed'), '失败');
  assert.equal(runStatusClass('blocked'), 'error');
});

test('settings module and task status normalize defaults', () => {
  assert.equal(normalizeSettingsModule('missing'), 'workspace');
  assert.equal(normalizeSettingsModule('data'), 'security');
  assert.equal(taskStatusLabel({running: true}), '运行中');
  assert.equal(taskStatusClass({last_status: 'failed'}), 'error');
});

test('settings page switches the document back to native scrolling', () => {
  const rootCalls = [];
  const bodyCalls = [];
  const root = { classList: { toggle: (...args) => rootCalls.push(args) } };
  const body = { classList: { toggle: (...args) => bodyCalls.push(args) } };

  setSettingsDocumentScroll(true, root, body);
  setSettingsDocumentScroll(false, root, body);

  assert.deepEqual(rootCalls, [['settings-page-visible', true], ['settings-page-visible', false]]);
  assert.deepEqual(bodyCalls, rootCalls);
});

test('AgentDock task polling starts only after setup and runtime configuration are ready', () => {
  assert.equal(agentTaskDataEnabled(null, null), false);
  assert.equal(agentTaskDataEnabled({needs_setup: true}, {agentdock_tasks_configured: true}), false);
  assert.equal(agentTaskDataEnabled({needs_setup: false}, {agentdock_tasks_configured: false}), false);
  assert.equal(agentTaskDataEnabled({needs_setup: false}, {agentdock_tasks_configured: true}), true);
  assert.equal(agentTaskDataEnabled({needs_setup: false}, {agentdock_tasks_configured: true}, true), false);
});
