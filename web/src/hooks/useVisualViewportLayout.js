import { useEffect } from 'react';
import { isTextEntryTarget, normalizeViewportMetrics } from '../lib/viewportLayout.js';

const mobileViewportQuery = '(max-width: 720px)';

export function useVisualViewportLayout() {
  useEffect(() => {
    const root = document.documentElement;
    const mobile = window.matchMedia(mobileViewportQuery);
    const visualViewport = window.visualViewport;
    let animationFrame = 0;

    const clearViewportState = () => {
      root.style.removeProperty('--chatdock-viewport-height');
      root.classList.remove('chatdock-keyboard-open');
    };

    const applyViewportState = () => {
      window.cancelAnimationFrame(animationFrame);
      animationFrame = window.requestAnimationFrame(() => {
        if (!mobile.matches) {
          clearViewportState();
          return;
        }

        // iOS 键盘会缩小 visualViewport，但部分 WebView 不会同步更新 100dvh。
        // 只同步可见高度，让主布局在正常 flex 流内收缩，避免输入区覆盖消息内容。
        const metrics = normalizeViewportMetrics(window.visualViewport, window.innerHeight);
        root.style.setProperty('--chatdock-viewport-height', `${metrics.height}px`);
        root.classList.toggle('chatdock-keyboard-open', isTextEntryTarget(document.activeElement));
      });
    };

    applyViewportState();
    visualViewport?.addEventListener('resize', applyViewportState);
    visualViewport?.addEventListener('scroll', applyViewportState);
    window.addEventListener('resize', applyViewportState);
    window.addEventListener('orientationchange', applyViewportState);
    document.addEventListener('focusin', applyViewportState);
    document.addEventListener('focusout', applyViewportState);
    mobile.addEventListener('change', applyViewportState);

    return () => {
      window.cancelAnimationFrame(animationFrame);
      visualViewport?.removeEventListener('resize', applyViewportState);
      visualViewport?.removeEventListener('scroll', applyViewportState);
      window.removeEventListener('resize', applyViewportState);
      window.removeEventListener('orientationchange', applyViewportState);
      document.removeEventListener('focusin', applyViewportState);
      document.removeEventListener('focusout', applyViewportState);
      mobile.removeEventListener('change', applyViewportState);
      clearViewportState();
    };
  }, []);
}
