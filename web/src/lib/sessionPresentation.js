import { fmtTime } from './appUtils.js';

export function scheduledTaskSessionRows({ selectedScheduledTaskID, selectedScheduledTaskRuns = [], selectedScheduledTask = null, sessions = [] }) {
  if (!selectedScheduledTaskID) return [];
  const byID = new Map(sessions.map(item => [item.id, item]));
  const seen = new Set();
  return (selectedScheduledTaskRuns || []).filter(run => run.session_id && !seen.has(run.session_id)).map(run => {
    seen.add(run.session_id);
    const session = byID.get(run.session_id);
    const runTitle = session?.title || run.session_title || run.task_title || selectedScheduledTask?.title || '定时任务';
    if (session) return { ...session, title: runTitle, preview: '', scheduled_run: run };
    return { id: run.session_id, title: runTitle + ' · ' + fmtTime(run.started_at), preview: '', last_role: run.status === 'failed' ? 'error' : 'assistant', count: 1, updated_at: run.finished_at || run.started_at, scheduled_run: run };
  });
}

export function visibleSessionRows({ sessionSearch = '', sessionSearchResults = [], selectedScheduledTaskID = '', selectedScheduledTaskSessions = [], sessions = [] }) {
  if (String(sessionSearch || '').trim()) return sessionSearchResults;
  if (selectedScheduledTaskID) return selectedScheduledTaskSessions;
  return sessions;
}
