import { useCallback, useEffect, useRef } from 'react';
import { createStreamTextQueue, streamRevealCount } from '../lib/streamTextQueue.js';

const revealIntervalMS = 20;

export function useStreamTextDisplay(appendParts) {
  const queueRef = useRef(createStreamTextQueue());
  const timerRef = useRef(null);
  const tickRef = useRef(null);
  const idleWaitersRef = useRef([]);

  const resolveIdleWaiters = useCallback(() => {
    if (queueRef.current.length || timerRef.current != null) return;
    const waiters = idleWaitersRef.current.splice(0);
    waiters.forEach(resolve => resolve());
  }, []);

  const schedule = useCallback(() => {
    if (!queueRef.current.length || timerRef.current != null) return;
    timerRef.current = window.setTimeout(() => tickRef.current?.(), revealIntervalMS);
  }, []);

  const tick = useCallback(() => {
    timerRef.current = null;
    const queue = queueRef.current;
    const parts = queue.take(streamRevealCount(queue.length));
    if (parts.length) appendParts(parts);
    if (queue.length) schedule();
    else resolveIdleWaiters();
  }, [appendParts, resolveIdleWaiters, schedule]);

  useEffect(() => {
    tickRef.current = tick;
  }, [tick]);

  const enqueue = useCallback((delta) => {
    queueRef.current.enqueue(delta);
    schedule();
  }, [schedule]);

  const flush = useCallback(() => {
    if (timerRef.current != null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    const parts = queueRef.current.drain();
    if (parts.length) appendParts(parts);
    resolveIdleWaiters();
  }, [appendParts, resolveIdleWaiters]);

  const reset = useCallback(() => {
    if (timerRef.current != null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    queueRef.current.clear();
    resolveIdleWaiters();
  }, [resolveIdleWaiters]);

  const waitUntilIdle = useCallback(() => {
    if (!queueRef.current.length && timerRef.current == null) return Promise.resolve();
    return new Promise(resolve => idleWaitersRef.current.push(resolve));
  }, []);

  useEffect(() => reset, [reset]);

  return { enqueue, flush, reset, waitUntilIdle };
}
