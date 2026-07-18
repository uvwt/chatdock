export function normalizeViewportMetrics(visualViewport, fallbackHeight) {
  const viewportHeight = Number(visualViewport?.height);
  const fallback = Number(fallbackHeight);
  const height = Number.isFinite(viewportHeight) && viewportHeight > 0
    ? viewportHeight
    : (Number.isFinite(fallback) && fallback > 0 ? fallback : 1);

  return {
    height: Math.max(1, Math.round(height)),
  };
}

export function isTextEntryTarget(target) {
  const tagName = String(target?.tagName || '').toLowerCase();
  return tagName === 'input'
    || tagName === 'textarea'
    || tagName === 'select'
    || target?.isContentEditable === true;
}
