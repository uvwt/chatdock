import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchCurrentSessionAgentTask } from '../lib/agentTaskApi.js';
import { agentTaskPollInterval, normalizeAgentTaskDetail } from '../lib/agentTasks.js';

export function useCurrentSessionTask(api, sessionID, enabled, busy) {
  const [task, setTask] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [lastUpdatedAt, setLastUpdatedAt] = useState(0);
  const taskSnapshotRef = useRef('');
  const requestSequenceRef = useRef(0);
  const refreshInFlightRef = useRef(false);
  const pollIntervalMS = agentTaskPollInterval(task, busy);

  const refresh = useCallback(async ({ initial = false } = {}) => {
    if (!enabled || !sessionID || document.visibilityState === 'hidden' || refreshInFlightRef.current) return;
    refreshInFlightRef.current = true;
    const sequence = ++requestSequenceRef.current;
    if (initial) setLoading(true);
    try {
      const payload = await fetchCurrentSessionAgentTask(api, sessionID);
      if (sequence !== requestSequenceRef.current) return;
      const nextTask = payload.task ? normalizeAgentTaskDetail(payload) : null;
      const nextSnapshot = JSON.stringify(nextTask);
      const changed = nextSnapshot !== taskSnapshotRef.current;
      if (changed) {
        taskSnapshotRef.current = nextSnapshot;
        setTask(nextTask);
      }
      setError('');
      // 后台检查无变化时不触发 App 重渲染，聊天消息中的 Markdown 不再周期性重复计算。
      if (changed || initial) setLastUpdatedAt(Date.now());
    } catch (err) {
      if (sequence !== requestSequenceRef.current) return;
      // 短暂断线时保留上一份任务，避免输入框上方的任务卡闪烁消失。
      setError(err.message || '读取当前会话任务失败');
    } finally {
      refreshInFlightRef.current = false;
      if (sequence === requestSequenceRef.current) setLoading(false);
    }
  }, [api, enabled, sessionID]);

  useEffect(() => {
    requestSequenceRef.current += 1;
    taskSnapshotRef.current = '';
    setTask(null);
    setError('');
    setLastUpdatedAt(0);
    setLoading(false);
  }, [sessionID]);

  useEffect(() => {
    if (!enabled || !sessionID) return undefined;
    void refresh({ initial: true });
    const timer = window.setInterval(() => void refresh(), pollIntervalMS);
    const handleVisibility = () => {
      if (document.visibilityState === 'visible') void refresh();
    };
    document.addEventListener('visibilitychange', handleVisibility);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener('visibilitychange', handleVisibility);
      requestSequenceRef.current += 1;
    };
  }, [enabled, pollIntervalMS, refresh, sessionID]);

  return {
    task,
    taskID: task?.id || '',
    loading,
    error,
    lastUpdatedAt,
    refresh,
  };
}
