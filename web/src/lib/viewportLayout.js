export function normalizeViewportMetrics(visualViewport, fallbackHeight) {
  const viewportHeight = Number(visualViewport?.height);
  const fallback = Number(fallbackHeight);
  const height = Number.isFinite(viewportHeight) && viewportHeight > 0
    ? viewportHeight
    : (Number.isFinite(fallback) && fallback > 0 ? fallback : 1);

  const viewportOffsetTop = Number(visualViewport?.offsetTop);
  const offsetTop = Number.isFinite(viewportOffsetTop) && viewportOffsetTop > 0
    ? viewportOffsetTop
    : 0;

  return {
    height: Math.max(1, Math.round(height)),
    offsetTop: Math.max(0, Math.round(offsetTop)),
  };
}

export function isTextEntryTarget(target) {
  const tagName = String(target?.tagName || '').toLowerCase();
  return tagName === 'input'
    || tagName === 'textarea'
    || tagName === 'select'
    || target?.isContentEditable === true;
}

export function shouldUseComposerKeyboardLayout(activeElement, composerShell) {
  return composerShell?.contains?.(activeElement) === true && isTextEntryTarget(activeElement);
}

export function shouldKeepMessagesAtBottom(messageBox, threshold = 120) {
  if (!messageBox) return false;
  const bottomGap = messageBox.scrollHeight - messageBox.scrollTop - messageBox.clientHeight;
  return bottomGap <= threshold;
}

export function nextMessageAutoFollowState(messageBox, previousScrollTop = 0, paused = false, threshold = 24) {
  if (!messageBox) {
    return { scrollTop: previousScrollTop, paused, stickToBottom: false };
  }

  const scrollTop = Number(messageBox.scrollTop) || 0;
  const bottomGap = Math.max(0, (Number(messageBox.scrollHeight) || 0) - scrollTop - (Number(messageBox.clientHeight) || 0));
  const nearBottom = bottomGap <= threshold;
  const movedUp = scrollTop < previousScrollTop - 1;

  // 用户上滑后持续暂停自动跟随；只有真正回到底部附近才恢复。
  const nextPaused = nearBottom ? false : (paused || movedUp);
  return {
    scrollTop,
    paused: nextPaused,
    stickToBottom: nearBottom && !nextPaused,
  };
}
