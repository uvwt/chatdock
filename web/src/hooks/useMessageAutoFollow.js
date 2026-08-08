import { useCallback, useEffect, useRef } from 'react';
import { isLoadingMessageList, nextMessageAutoFollowState } from '../lib/viewportLayout.js';

export function useMessageAutoFollow({ messages, setShowJumpToLatest }) {
  const messagesRef = useRef(null);
  const stickToBottomRef = useRef(true);
  const autoFollowPausedRef = useRef(false);
  const lastScrollTopRef = useRef(0);
  const touchStartYRef = useRef(null);
  const pointerScrollStartRef = useRef(null);
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
    const loadingConversation = isLoadingMessageList(messages);
    if (loadingConversation) {
      // 路由加载占位不能消耗 resetMessageAutoFollow 设置的强制贴底状态；
      // 否则真实会话替换占位后会停在旧 scrollTop 对应的中间位置。
      box.scrollTop = 0;
      stickToBottomRef.current = true;
      autoFollowPausedRef.current = false;
      lastScrollTopRef.current = 0;
      setShowJumpToLatest(false);
      return;
    }
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

  useEffect(() => {
    const box = messagesRef.current;
    if (!box || !messages.length || typeof ResizeObserver !== 'function') return undefined;

    let followFrame = 0;
    const observer = new ResizeObserver(() => {
      // Markdown 懒加载、图片加载或工具块展开都会改变消息高度。
      // 布局变化期间 scroll 事件可能暂时把 stickToBottom 置为 false；
      // 是否继续跟随只由用户主动暂停状态决定，避免把内容重排误判成上滑阅读。
      if (autoFollowPausedRef.current) return;
      window.cancelAnimationFrame(followFrame);
      followFrame = window.requestAnimationFrame(() => {
        if (autoFollowPausedRef.current) return;
        box.scrollTop = box.scrollHeight;
        lastScrollTopRef.current = box.scrollTop;
        stickToBottomRef.current = true;
        updateJumpToLatestVisibility();
      });
    });

    for (const child of box.children) observer.observe(child);
    return () => {
      window.cancelAnimationFrame(followFrame);
      observer.disconnect();
    };
  }, [messages, updateJumpToLatestVisibility]);

  const handleMessagesScroll = useCallback(() => {
    const box = messagesRef.current;
    if (!box) return;
    const pointerStart = pointerScrollStartRef.current;
    const pointerMovedUp = Number.isFinite(pointerStart) && box.scrollTop < pointerStart - 1;
    // 没有真实向上手势时，scrollTop 变小通常来自 Markdown/图片重排或滚动锚定，
    // 不能把这种布局位移误判为用户暂停自动跟随。
    const previousScrollTop = pointerMovedUp
      ? lastScrollTopRef.current
      : Math.min(lastScrollTopRef.current, box.scrollTop);
    const state = nextMessageAutoFollowState(box, previousScrollTop, autoFollowPausedRef.current);
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

  const handleMessagesPointerDown = useCallback(() => {
    pointerScrollStartRef.current = messagesRef.current?.scrollTop ?? null;
  }, []);

  const handleMessagesPointerEnd = useCallback(() => {
    pointerScrollStartRef.current = null;
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
    handleMessagesPointerDown,
    handleMessagesPointerEnd,
  };
}
