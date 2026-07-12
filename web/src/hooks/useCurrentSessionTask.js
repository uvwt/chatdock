import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchCurrentSessionAgentTask } from '../lib/agentTaskApi.js';
import { normalizeAgentTaskDetail } from '../lib/agentTasks.js';

const taskPollIntervalMS = 2000;

export function useCurrentSessionTask(api, sessionID, enabled) {
  const [task, setTask] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [lastUpdatedAt, setLastUpdatedAt] = useState(0);
  const requestSequenceRef = useRef(0);

  const refresh = useCallback(async ({ initial = false } = {}) => {
    if (!enabled || !sessionID || document.visibilityState === 'hidden') return;
    const sequence = ++requestSequenceRef.current;
    if (initial) setLoading(true);
    try {
      const payload = await fetchCurrentSessionAgentTask(api, sessionID);
      if (sequence !== requestSequenceRef.current) return;
      setTask(payload.task ? normalizeAgentTaskDetail(payload) : null);
      setError('');
      setLastUpdatedAt(Date.now());
    } catch (err) {
      if (sequence !== requestSequenceRef.current) return;
      // 短暂断线时保留上一份任务，避免输入框上方的任务卡闪烁消失。
      setError(err.message || '读取当前会话任务失败');
    } finally {
      if (sequence === requestSequenceRef.current) setLoading(false);
    }
  }, [api, enabled, sessionID]);

  useEffect(() => {
    requestSequenceRef.current += 1;
    setTask(null);
    setError('');
    setLastUpdatedAt(0);
    setLoading(false);
  }, [sessionID]);

  useEffect(() => {
    if (!enabled || !sessionID) return undefined;
    void refresh({ initial: true });
    const timer = window.setInterval(() => void refresh(), taskPollIntervalMS);
    const handleVisibility = () => {
      if (document.visibilityState === 'visible') void refresh();
    };
    document.addEventListener('visibilitychange', handleVisibility);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener('visibilitychange', handleVisibility);
      requestSequenceRef.current += 1;
    };
  }, [enabled, refresh, sessionID]);

  return {
    task,
    taskID: task?.id || '',
    loading,
    error,
    lastUpdatedAt,
    refresh,
  };
}
