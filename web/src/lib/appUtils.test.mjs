import test from 'node:test';
import assert from 'node:assert/strict';
import { agentTaskDataEnabled, fmtBytes, fmtDuration, normalizeSettingsModule, runStatusClass, runStatusLabel, taskStatusClass, taskStatusLabel } from './appUtils.js';

test('format helpers keep compact Chinese UI labels', () => {
  assert.equal(fmtBytes(1536), '1.5 KB');
  assert.equal(fmtDuration(1500), '1.5s');
  assert.equal(runStatusLabel('failed'), '失败');
  assert.equal(runStatusClass('blocked'), 'error');
});

test('settings module and task status normalize defaults', () => {
  assert.equal(normalizeSettingsModule('missing'), 'workspace');
  assert.equal(taskStatusLabel({running: true}), '运行中');
  assert.equal(taskStatusClass({last_status: 'failed'}), 'error');
});

test('AgentDock task polling starts only after setup and runtime configuration are ready', () => {
  assert.equal(agentTaskDataEnabled(null, null), false);
  assert.equal(agentTaskDataEnabled({needs_setup: true}, {agentdock_tasks_configured: true}), false);
  assert.equal(agentTaskDataEnabled({needs_setup: false}, {agentdock_tasks_configured: false}), false);
  assert.equal(agentTaskDataEnabled({needs_setup: false}, {agentdock_tasks_configured: true}), true);
});
