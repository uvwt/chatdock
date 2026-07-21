import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchAgentTask, fetchAgentTasks } from '../lib/agentTaskApi.js';
import { normalizeAgentTaskDetail, sortAgentTasks } from '../lib/agentTasks.js';

const liveTaskPollIntervalMS = 2000;
const idleTaskPollIntervalMS = 15000;

export function useAgentTasks(api, enabled, live) {
  const [tasks, setTasks] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [lastUpdatedAt, setLastUpdatedAt] = useState(0);
  const [expandedTaskID, setExpandedTaskIDState] = useState('');
  const [taskDetail, setTaskDetail] = useState(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');
  const expandedTaskIDRef = useRef('');
  const tasksSnapshotRef = useRef('');
  const taskDetailSnapshotRef = useRef('');
  const requestSequenceRef = useRef(0);
  const refreshInFlightRef = useRef(false);
  const pollIntervalMS = live ? liveTaskPollIntervalMS : idleTaskPollIntervalMS;

  const loadDetail = useCallback(async (taskID, sequence, showLoading = false) => {
    if (!taskID) {
      taskDetailSnapshotRef.current = '';
      setTaskDetail(null);
      setDetailError('');
      return;
    }
    if (showLoading) setDetailLoading(true);
    try {
      const payload = await fetchAgentTask(api, taskID);
      if (sequence !== requestSequenceRef.current || expandedTaskIDRef.current !== taskID) return;
      const nextDetail = normalizeAgentTaskDetail(payload);
      const nextSnapshot = JSON.stringify(nextDetail);
      if (nextSnapshot !== taskDetailSnapshotRef.current) {
        taskDetailSnapshotRef.current = nextSnapshot;
        setTaskDetail(nextDetail);
      }
      setDetailError('');
    } catch (err) {
      if (sequence !== requestSequenceRef.current || expandedTaskIDRef.current !== taskID) return;
      setDetailError(err.message || '读取任务详情失败');
    } finally {
      if (sequence === requestSequenceRef.current && expandedTaskIDRef.current === taskID) setDetailLoading(false);
    }
  }, [api]);

  const refresh = useCallback(async ({ initial = false } = {}) => {
    if (!enabled || document.visibilityState === 'hidden' || refreshInFlightRef.current) return;
    refreshInFlightRef.current = true;
    const sequence = ++requestSequenceRef.current;
    if (initial) setLoading(true);
    try {
      const payload = await fetchAgentTasks(api, 30);
      if (sequence !== requestSequenceRef.current) return;
      const nextTasks = sortAgentTasks(payload.tasks || []);
      const nextSnapshot = JSON.stringify(nextTasks);
      const changed = nextSnapshot !== tasksSnapshotRef.current;
      if (changed) {
        tasksSnapshotRef.current = nextSnapshot;
        setTasks(nextTasks);
      }
      setError('');
      // 周期检查没有数据变化时不更新 React 状态，避免整棵聊天界面每两秒重渲染。
      if (changed || initial) setLastUpdatedAt(Date.now());
      await loadDetail(expandedTaskIDRef.current, sequence);
    } catch (err) {
      if (sequence !== requestSequenceRef.current) return;
      // 短暂断线时保留上一次任务数据，面板展示错误而不是闪成空列表。
      setError(err.message || '读取任务失败');
    } finally {
      refreshInFlightRef.current = false;
      if (sequence === requestSequenceRef.current) setLoading(false);
    }
  }, [api, enabled, loadDetail]);

  const setExpandedTaskID = useCallback((taskID) => {
    const next = taskID === expandedTaskIDRef.current ? '' : taskID;
    expandedTaskIDRef.current = next;
    taskDetailSnapshotRef.current = '';
    setExpandedTaskIDState(next);
    setTaskDetail(null);
    setDetailError('');
    if (next && enabled) {
      const sequence = ++requestSequenceRef.current;
      void loadDetail(next, sequence, true);
    }
  }, [enabled, loadDetail]);

  useEffect(() => {
    if (!enabled) return undefined;
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
  }, [enabled, pollIntervalMS, refresh]);

  useEffect(() => {
    if (enabled) return;
    expandedTaskIDRef.current = '';
    tasksSnapshotRef.current = '';
    taskDetailSnapshotRef.current = '';
    setTasks([]);
    setLastUpdatedAt(0);
    setExpandedTaskIDState('');
    setTaskDetail(null);
    setDetailError('');
  }, [enabled]);

  return {
    tasks,
    loading,
    error,
    lastUpdatedAt,
    expandedTaskID,
    taskDetail,
    detailLoading,
    detailError,
    refresh,
    setExpandedTaskID,
  };
}
