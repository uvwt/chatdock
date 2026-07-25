// Shared UI formatting, routing, and product-status helpers.
export const settingsModules = ['model', 'providers', 'tools', 'projects', 'automation', 'security'];

export function normalizeSettingsModule(name) {
  if (name === 'data') return 'security';
  return settingsModules.includes(name) ? name : 'model';
}

export function agentTaskDataEnabled(setupStatus, systemStatus, authPageVisible = false) {
  return !authPageVisible && !!setupStatus && !setupStatus.needs_setup && systemStatus?.agentdock_tasks_configured === true;
}

export function logoutAndReload(storage = localStorage, location = window.location) {
  // 硬刷新确保轮询、流连接和内存中的受保护数据与登录态一起释放。
  storage.removeItem('chatdock.authToken');
  location.reload();
}

export function setSettingsDocumentScroll(enabled, root = document.documentElement, body = document.body) {
  // 聊天页会锁住文档滚动；配置页内容较长，必须切回浏览器原生文档滚动，避免 iOS Safari 嵌套滚动失效。
  root?.classList?.toggle('settings-page-visible', enabled);
  body?.classList?.toggle('settings-page-visible', enabled);
}

export function fmtTime(value) {
  if (!value) return '';
  try { return new Date(value).toLocaleString(); } catch { return ''; }
}

export function fmtBytes(value) {
  const n = Number(value || 0);
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB';
  return (n / 1024 / 1024 / 1024).toFixed(1) + ' GB';
}

export function fmtDuration(ms) {
  const n = Number(ms || 0);
  if (n <= 0) return '-';
  if (n < 1000) return n + 'ms';
  return (n / 1000).toFixed(1) + 's';
}

export function fmtRelativeAge(seconds) {
  const n = Number(seconds || 0);
  if (n <= 0) return '刚刚';
  if (n < 3600) return Math.floor(n / 60) + ' 分钟前';
  if (n < 86400) return Math.floor(n / 3600) + ' 小时前';
  return Math.floor(n / 86400) + ' 天前';
}

export function safePathName(value) {
  const text = String(value || '').trim();
  if (!text) return '-';
  return text.split(/[\/]/).filter(Boolean).pop() || text;
}

export function diagnosticsText({ setupStatus, systemStatus, dataStatus, mcpStatus, providers }) {
  const setup = setupStatus || systemStatus?.setup || {};
  const data = dataStatus || systemStatus?.data || {};
  const lines = [
    '# ChatDock 诊断信息',
    '- 时间：' + new Date().toLocaleString(),
    '- 状态：' + (systemStatus?.ok ? 'healthy' : 'unknown'),
    '- 地址：' + (systemStatus?.addr || '-'),
    '- 数据目录：' + safePathName(data.data_dir || setup.data_dir),
    '- 项目数量：' + (data.project_count ?? setup.project_count ?? '-'),
    '- 会话数量：' + (data.session_count ?? '-'),
    '- 数据库：' + (data.database_exists ? safePathName(data.database_path || systemStatus?.database) : '未创建'),
    '- 数据库大小：' + fmtBytes(data.database_size_bytes),
    '- 数据库健康：' + (data.database_healthy ? '正常' : (data.database_warning || '未知')),
    '- WAL：' + (data.wal_enabled ? '启用' : '未检测到'),
    '- 备份目录：' + (data.backup_dir ? safePathName(data.backup_dir) : '未检测到'),
    '- 已检查备份目录：' + ((data.backup_checked_dirs || []).map(safePathName).join(', ') || '无'),
    '- 备份数量：' + (data.backup_count || 0),
    '- 最近备份：' + (data.latest_backup_at ? fmtTime(data.latest_backup_at) + '（' + fmtRelativeAge(data.latest_backup_age_seconds) + '）' : '暂无'),
    '- 备份健康：' + (data.backup_healthy ? '正常' : (data.backup_warning || '异常或未知')),
    '- 模型供应商数量：' + (providers || []).length,
    '- MCP Server 数量：' + (mcpStatus || []).length,
  ];
  return lines.join('\n');
}

export function runStatusLabel(status) {
  return ({running:'执行中', success:'成功', failed:'失败', completed:'已完成', blocked:'已阻塞', active:'进行中', matched:'已匹配'})[status] || (status || '未知');
}

export function runStatusClass(status) {
  if (status === 'failed' || status === 'blocked') return 'error';
  if (status === 'running' || status === 'active') return 'warn';
  return 'ok';
}

export function taskStatusLabel(t) {
  if (t.running) return '运行中';
  if (t.last_status === 'success') return '成功';
  if (t.last_status === 'failed') return '失败';
  return t.enabled ? '已启用' : '已暂停';
}

export function taskStatusClass(t) {
  if (t.running) return 'warn';
  if (t.last_status === 'success') return 'ok';
  if (t.last_status === 'failed') return 'error';
  return t.enabled ? 'ok' : 'warn';
}

export function defaultRunAtValue() {
  const d = new Date(Date.now() + 60 * 60 * 1000);
  const pad = n => String(n).padStart(2, '0');
  return d.getFullYear() + '-' + pad(d.getMonth()+1) + '-' + pad(d.getDate()) + 'T' + pad(d.getHours()) + ':' + pad(d.getMinutes());
}

const cronExpressionPattern = /^(\d{1,2})\s+(\d{1,2})\s+(\*|[1-9]|[12]\d|3[01])\s+\*\s+(\*|[0-6]|1-5)$/;
const cronWeekdayLabels = {'0': '星期日', '1': '星期一', '2': '星期二', '3': '星期三', '4': '星期四', '5': '星期五', '6': '星期六'};

function scheduleTimezone(value) {
  return String(value || '').trim() || 'Asia/Shanghai';
}

function parsedCronExpression(expression) {
  const match = String(expression || '').trim().match(cronExpressionPattern);
  if (!match) return null;
  const minute = Number(match[1]);
  const hour = Number(match[2]);
  if (minute < 0 || minute > 59 || hour < 0 || hour > 23) return null;
  return {
    month_day: match[3],
    weekday: match[4],
    time: String(hour).padStart(2, '0') + ':' + String(minute).padStart(2, '0'),
  };
}

function customCronSchedule(expressions, timezone) {
  return {frequency: 'custom', weekday: '1', month_day: '1', times: [], timezone, original_expressions: expressions};
}

export function cronScheduleFormValue(task = {}, fallbackTimezone = 'Asia/Shanghai') {
  const timezone = scheduleTimezone(task?.timezone || fallbackTimezone);
  const expressions = Array.isArray(task?.cron_expressions) ? task.cron_expressions.map(value => String(value || '').trim()).filter(Boolean) : [];
  if (!expressions.length) return {frequency: 'daily', weekday: '1', month_day: '1', times: ['09:00'], timezone, original_expressions: []};

  const parsed = expressions.map(parsedCronExpression);
  if (parsed.some(value => !value)) return customCronSchedule(expressions, timezone);
  const {month_day: monthDay, weekday} = parsed[0];
  if (parsed.some(value => value.month_day !== monthDay || value.weekday !== weekday)) return customCronSchedule(expressions, timezone);

  let frequency = '';
  if (monthDay === '*' && weekday === '*') frequency = 'daily';
  if (monthDay === '*' && weekday === '1-5') frequency = 'weekdays';
  if (monthDay === '*' && cronWeekdayLabels[weekday]) frequency = 'weekly';
  if (monthDay !== '*' && weekday === '*') frequency = 'monthly';
  if (!frequency) return customCronSchedule(expressions, timezone);

  const times = [...new Set(parsed.map(value => value.time))].sort();
  return {frequency, weekday: frequency === 'weekly' ? weekday : '1', month_day: frequency === 'monthly' ? monthDay : '1', times, timezone, original_expressions: expressions};
}

export function cronSchedulePayload(values = {}) {
  const schedule = values.cron_schedule && typeof values.cron_schedule === 'object' ? values.cron_schedule : {};
  const timezone = scheduleTimezone(schedule.timezone);
  if (schedule.frequency === 'custom') return {cron_expressions: [...(schedule.original_expressions || [])], timezone};

  const times = [...new Set((Array.isArray(schedule.times) ? schedule.times : []).map(value => String(value || '').trim()).filter(value => /^([01]\d|2[0-3]):[0-5]\d$/.test(value)))].sort();
  let suffix = '* * *';
  if (schedule.frequency === 'weekdays') suffix = '* * 1-5';
  if (schedule.frequency === 'weekly') suffix = '* * ' + String(schedule.weekday || '1');
  if (schedule.frequency === 'monthly') suffix = String(schedule.month_day || '1') + ' * *';
  return {
    cron_expressions: times.map(value => {
      const [hour, minute] = value.split(':');
      return Number(minute) + ' ' + Number(hour) + ' ' + suffix;
    }),
    timezone,
  };
}

function cronScheduleText(task) {
  const schedule = cronScheduleFormValue(task, task?.timezone);
  if (schedule.frequency === 'custom') return '自定义重复计划';
  const times = schedule.times.join('、') || '未设置时间';
  if (schedule.frequency === 'weekdays') return '工作日 ' + times;
  if (schedule.frequency === 'weekly') return '每周' + (cronWeekdayLabels[schedule.weekday] || '') + ' ' + times;
  if (schedule.frequency === 'monthly') return '每月 ' + schedule.month_day + ' 日 ' + times;
  return '每天 ' + times;
}

export function scheduleSummary(t) {
  const next = t.next_run_at ? fmtTime(t.next_run_at) : '未计划';
  const last = t.last_run_at ? fmtTime(t.last_run_at) : '未运行';
  let plan = '一次性：' + (t.run_at ? fmtTime(t.run_at) : next);
  if (t.schedule_type === 'interval') plan = '每 ' + (t.interval_minutes || 0) + ' 分钟';
  if (t.schedule_type === 'cron') plan = cronScheduleText(t) + ' · ' + scheduleTimezone(t.timezone);
  return plan + ' · 下次：' + next + ' · 上次：' + last;
}

export function settingsModuleFromPath() {
  const parts = window.location.pathname.split('/').filter(Boolean);
  if (parts[0] !== 'settings') return '';
  return normalizeSettingsModule(parts[1] || localStorage.getItem('chatdock.settingsModule') || 'model');
}

export function sessionIDFromPath() {
  const parts = window.location.pathname.split('/').filter(Boolean);
  return parts[0] === 'sessions' && parts[1] ? parts[1] : '';
}

export function sessionPath(id) {
  return '/sessions/' + encodeURIComponent(id);
}

export function filenameFromResponse(res, fallback) {
  const value = res.headers.get('Content-Disposition') || '';
  const match = value.match(/filename="?([^";]+)"?/i);
  return match ? match[1] : fallback;
}
