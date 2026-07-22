import { useCallback, useEffect, useRef } from 'react';
import { nextMessageAutoFollowState } from '../lib/viewportLayout.js';

export function useMessageAutoFollow({ messages, setShowJumpToLatest }) {
  const messagesRef = useRef(null);
  const stickToBottomRef = useRef(true);
  const autoFollowPausedRef = useRef(false);
  const lastScrollTopRef = useRef(0);
  const touchStartYRef = useRef(null);
  const forceScrollRef = useRef(false);

  const latestModelMessageElement = useCallback(() => {
    const box = messagesRef.current;
    if (!box) return null;
    const nodes = box.querySelectorAll('[data-model-message="true"]');
    return nodes[nodes.length - 1] || null;
  }, []);

  const updateJumpToLatestVisibility = useCallback(() => {
    const box = messagesRef.current;
    if (!box) return;
    const latest = latestModelMessageElement();
    const bottomGap = box.scrollHeight - box.scrollTop - box.clientHeight;
    if (!latest) {
      setShowJumpToLatest(messages.length > 0 && bottomGap > 160);
      return;
    }
    const latestTop = latest.offsetTop - box.scrollTop;
    const latestBottom = latestTop + latest.offsetHeight;
    // 用户已经在最新回复内部阅读时不显示跳转按钮，避免遮挡正文。
    const shouldShow = bottomGap > 160 && latestTop > box.clientHeight - 96 && latestBottom > box.clientHeight;
    setShowJumpToLatest(previous => previous === shouldShow ? previous : shouldShow);
  }, [latestModelMessageElement, messages.length, setShowJumpToLatest]);

  const scrollToLatestModelMessage = useCallback(() => {
    const box = messagesRef.current;
    if (!box) return;
    const latest = latestModelMessageElement();
    const top = latest ? Math.max(0, latest.offsetTop - 14) : box.scrollHeight;
    box.scrollTo({ top, behavior: 'smooth' });
    window.setTimeout(updateJumpToLatestVisibility, 360);
  }, [latestModelMessageElement, updateJumpToLatestVisibility]);

  const resetMessageAutoFollow = useCallback(() => {
    forceScrollRef.current = true;
    stickToBottomRef.current = true;
    autoFollowPausedRef.current = false;
  }, []);

  useEffect(() => {
    const box = messagesRef.current;
    if (!box) return;
    if (!messages.length) {
      // 空状态本身可能高于手机视口；此时不要沿用聊天流的贴底策略。
      box.scrollTop = 0;
      forceScrollRef.current = false;
      stickToBottomRef.current = true;
      autoFollowPausedRef.current = false;
      lastScrollTopRef.current = 0;
      setShowJumpToLatest(false);
      return;
    }
    if (forceScrollRef.current || (stickToBottomRef.current && !autoFollowPausedRef.current)) {
      box.scrollTop = box.scrollHeight;
      lastScrollTopRef.current = box.scrollTop;
      autoFollowPausedRef.current = false;
      stickToBottomRef.current = true;
      forceScrollRef.current = false;
    }
    window.requestAnimationFrame(updateJumpToLatestVisibility);
  }, [messages, setShowJumpToLatest, updateJumpToLatestVisibility]);

  const handleMessagesScroll = useCallback(() => {
    const box = messagesRef.current;
    if (!box) return;
    const state = nextMessageAutoFollowState(box, lastScrollTopRef.current, autoFollowPausedRef.current);
    lastScrollTopRef.current = state.scrollTop;
    autoFollowPausedRef.current = state.paused;
    stickToBottomRef.current = state.stickToBottom;
    updateJumpToLatestVisibility();
  }, [updateJumpToLatestVisibility]);

  const pauseAutoFollow = useCallback(() => {
    autoFollowPausedRef.current = true;
    stickToBottomRef.current = false;
  }, []);

  const handleMessagesWheel = useCallback((event) => {
    if (event.deltaY < 0) pauseAutoFollow();
  }, [pauseAutoFollow]);

  const handleMessagesTouchStart = useCallback((event) => {
    touchStartYRef.current = event.touches?.[0]?.clientY ?? null;
  }, []);

  const handleMessagesTouchMove = useCallback((event) => {
    const currentY = event.touches?.[0]?.clientY;
    const startY = touchStartYRef.current;
    if (!Number.isFinite(currentY) || !Number.isFinite(startY)) return;
    // 手指向下拖动会查看更早的消息，必须先于下一帧流式更新暂停贴底。
    if (currentY > startY + 4) pauseAutoFollow();
    touchStartYRef.current = currentY;
  }, [pauseAutoFollow]);

  const handleMessagesTouchEnd = useCallback(() => {
    touchStartYRef.current = null;
  }, []);

  return {
    messagesRef,
    scrollToLatestModelMessage,
    resetMessageAutoFollow,
    handleMessagesScroll,
    handleMessagesWheel,
    handleMessagesTouchStart,
    handleMessagesTouchMove,
    handleMessagesTouchEnd,
  };
}
