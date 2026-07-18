import { useEffect } from 'react';
import { isTextEntryTarget, normalizeViewportMetrics, shouldKeepMessagesAtBottom } from '../lib/viewportLayout.js';

const mobileViewportQuery = '(max-width: 720px)';

export function useVisualViewportLayout() {
  useEffect(() => {
    const root = document.documentElement;
    const mobile = window.matchMedia(mobileViewportQuery);
    const visualViewport = window.visualViewport;
    let layoutFrame = 0;
    let scrollFrame = 0;

    const clearViewportState = () => {
      root.style.removeProperty('--chatdock-viewport-height');
      root.style.removeProperty('--chatdock-viewport-offset-top');
      root.classList.remove('chatdock-keyboard-open');
    };

    const applyViewportState = () => {
      const messageBox = document.querySelector('#app.app .messages');
      const keepMessagesAtBottom = shouldKeepMessagesAtBottom(messageBox);

      window.cancelAnimationFrame(layoutFrame);
      layoutFrame = window.requestAnimationFrame(() => {
        if (!mobile.matches) {
          clearViewportState();
          return;
        }

        // iOS 聚焦输入框时会同时缩小并平移 visualViewport。
        // 应用固定到这个真实可见矩形，内部仍保持正常 flex 流，避免状态栏重叠和输入区覆盖消息。
        const metrics = normalizeViewportMetrics(visualViewport, window.innerHeight);
        const keyboardOpen = isTextEntryTarget(document.activeElement);
        root.style.setProperty('--chatdock-viewport-height', `${metrics.height}px`);
        root.style.setProperty('--chatdock-viewport-offset-top', `${metrics.offsetTop}px`);
        root.classList.toggle('chatdock-keyboard-open', keyboardOpen);

        if (!keyboardOpen || !keepMessagesAtBottom || !messageBox) return;
        window.cancelAnimationFrame(scrollFrame);
        scrollFrame = window.requestAnimationFrame(() => {
          messageBox.scrollTop = messageBox.scrollHeight;
        });
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
      window.cancelAnimationFrame(layoutFrame);
      window.cancelAnimationFrame(scrollFrame);
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
