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
