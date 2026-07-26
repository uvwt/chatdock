import test from 'node:test';
import assert from 'node:assert/strict';
import { agentTaskDataEnabled, cronScheduleFormValue, cronSchedulePayload, fmtBytes, fmtDuration, logoutAndReload, normalizeSettingsModule, runStatusClass, runStatusLabel, scheduleSummary, setSettingsDocumentScroll, settingsModuleFromPath, taskStatusClass, taskStatusLabel } from './appUtils.js';

test('format helpers keep compact Chinese UI labels', () => {
  assert.equal(fmtBytes(1536), '1.5 KB');
  assert.equal(fmtDuration(1500), '1.5s');
  assert.equal(runStatusLabel('failed'), '失败');
  assert.equal(runStatusClass('blocked'), 'error');
});

test('settings module and task status normalize defaults', () => {
  assert.equal(normalizeSettingsModule('missing'), 'model');
  assert.equal(normalizeSettingsModule('data'), 'security');
  assert.equal(normalizeSettingsModule('providers'), 'model');
  assert.equal(normalizeSettingsModule('projects'), 'projects');
  assert.equal(normalizeSettingsModule('automation'), 'automation');
  assert.equal(taskStatusLabel({running: true}), '运行中');
  assert.equal(taskStatusClass({last_status: 'failed'}), 'error');
});

test('settings modules resolve only from settings routes', () => {
  const originalWindow = globalThis.window;
  try {
    globalThis.window = { location: { pathname: '/projects' } };
    assert.equal(settingsModuleFromPath(), '');
    globalThis.window.location.pathname = '/scheduled-tasks';
    assert.equal(settingsModuleFromPath(), '');
    globalThis.window.location.pathname = '/settings/projects';
    assert.equal(settingsModuleFromPath(), 'projects');
    globalThis.window.location.pathname = '/settings/automation';
    assert.equal(settingsModuleFromPath(), 'automation');
    globalThis.window.location.pathname = '/';
    assert.equal(settingsModuleFromPath(), '');
  } finally {
    globalThis.window = originalWindow;
  }
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


test('common Cron expressions become a structured daily plan', () => {
  assert.deepEqual(cronScheduleFormValue({
    cron_expressions: ['30 9 * * *', '0 18 * * *'],
    timezone: 'Asia/Shanghai',
  }), {
    frequency: 'daily',
    weekday: '1',
    month_day: '1',
    times: ['09:30', '18:00'],
    timezone: 'Asia/Shanghai',
    original_expressions: ['30 9 * * *', '0 18 * * *'],
  });
});

test('structured weekday, weekly and monthly plans generate Cron automatically', () => {
  assert.deepEqual(cronSchedulePayload({cron_schedule: {
    frequency: 'weekdays', weekday: '1', month_day: '1', times: ['08:15'], timezone: 'Asia/Shanghai',
  }}).cron_expressions, ['15 8 * * 1-5']);
  assert.deepEqual(cronSchedulePayload({cron_schedule: {
    frequency: 'weekly', weekday: '5', month_day: '1', times: ['20:30', '09:00'], timezone: 'Asia/Shanghai',
  }}), {
    cron_expressions: ['0 9 * * 5', '30 20 * * 5'],
    timezone: 'Asia/Shanghai',
  });
  assert.deepEqual(cronSchedulePayload({cron_schedule: {
    frequency: 'monthly', weekday: '1', month_day: '15', times: ['08:05'], timezone: 'Asia/Shanghai',
  }}).cron_expressions, ['5 8 15 * *']);
});

test('unsupported advanced plans stay intact without exposing an editor', () => {
  const form = cronScheduleFormValue({cron_expressions: ['*/15 * * * *'], timezone: 'UTC'});
  assert.equal(form.frequency, 'custom');
  assert.deepEqual(cronSchedulePayload({cron_schedule: form}), {
    cron_expressions: ['*/15 * * * *'],
    timezone: 'UTC',
  });
});

test('schedule summary uses readable plan text instead of Cron syntax', () => {
  const summary = scheduleSummary({
    schedule_type: 'cron',
    cron_expressions: ['30 9 * * *', '0 18 * * *'],
    timezone: 'Asia/Shanghai',
  });
  assert.match(summary, /每天 09:30、18:00/);
  assert.match(summary, /Asia\/Shanghai/);
  assert.doesNotMatch(summary, /30 9 \* \* \*/);
});
