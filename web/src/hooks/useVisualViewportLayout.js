import { useEffect } from 'react';
import {
  messageBottomClearance,
  normalizeViewportMetrics,
  shouldKeepMessagesAtBottom,
  shouldUseComposerKeyboardLayout,
} from '../lib/viewportLayout.js';

const mobileViewportQuery = '(max-width: 720px)';

export function useVisualViewportLayout() {
  useEffect(() => {
    const root = document.documentElement;
    const mobile = window.matchMedia(mobileViewportQuery);
    const visualViewport = window.visualViewport;
    let layoutFrame = 0;
    let scrollFrame = 0;
    let clearanceFrame = 0;
    let clearanceScrollFrame = 0;
    let mainObserver = null;
    const observedOverlays = new Set();

    const overlayResizeObserver = typeof ResizeObserver === 'function'
      ? new ResizeObserver(() => applyViewportState())
      : null;

    const clearViewportState = () => {
      root.style.removeProperty('--chatdock-viewport-height');
      root.style.removeProperty('--chatdock-viewport-offset-top');
      root.style.removeProperty('--chatdock-message-bottom-clearance');
      root.classList.remove('chatdock-keyboard-open');
    };

    const syncOverlayObservers = () => {
      const overlays = [
        document.querySelector('#app.app .composer-shell'),
        document.querySelector('#app.app .current-session-task'),
      ].filter(Boolean);

      for (const element of observedOverlays) {
        if (overlays.includes(element)) continue;
        overlayResizeObserver?.unobserve(element);
        observedOverlays.delete(element);
      }
      for (const element of overlays) {
        if (observedOverlays.has(element)) continue;
        observedOverlays.add(element);
        overlayResizeObserver?.observe(element);
      }
    };

    const updateMessageBottomClearance = (messageBox, preserveBottom) => {
      window.cancelAnimationFrame(clearanceFrame);
      clearanceFrame = window.requestAnimationFrame(() => {
        if (!mobile.matches || !messageBox) {
          root.style.removeProperty('--chatdock-message-bottom-clearance');
          return;
        }

        const overlayRects = [
          document.querySelector('#app.app .composer-shell'),
          document.querySelector('#app.app .current-session-task'),
        ].filter(Boolean).map(element => element.getBoundingClientRect());
        const clearance = messageBottomClearance(messageBox.getBoundingClientRect(), overlayRects);
        root.style.setProperty('--chatdock-message-bottom-clearance', `${clearance}px`);
        if (!preserveBottom) return;
        window.cancelAnimationFrame(clearanceScrollFrame);
        clearanceScrollFrame = window.requestAnimationFrame(() => {
          messageBox.scrollTop = messageBox.scrollHeight;
        });
      });
    };

    const applyViewportState = () => {
      const messageBox = document.querySelector('#app.app .messages');
      const composerShell = document.querySelector('#app.app .composer-shell');
      const keepMessagesAtBottom = shouldKeepMessagesAtBottom(messageBox);
      const preserveBottomAfterClearance = shouldKeepMessagesAtBottom(messageBox, 24);
      syncOverlayObservers();

      window.cancelAnimationFrame(layoutFrame);
      layoutFrame = window.requestAnimationFrame(() => {
        if (!mobile.matches) {
          clearViewportState();
          return;
        }

        // iOS 聚焦输入框时会同时缩小并平移 visualViewport。
        // 应用固定到 Safari 返回的真实可见矩形，避免再人为叠加键盘附件栏高度。
        const metrics = normalizeViewportMetrics(visualViewport, window.innerHeight);
        const activeElement = document.activeElement;
        // 配置中心也包含 input/select；只有聊天输入区获得焦点时才启用键盘布局，避免误锁配置页滚动。
        const keyboardOpen = shouldUseComposerKeyboardLayout(activeElement, composerShell);
        root.style.setProperty('--chatdock-viewport-height', `${metrics.height}px`);
        root.style.setProperty('--chatdock-viewport-offset-top', `${metrics.offsetTop}px`);
        root.classList.toggle('chatdock-keyboard-open', keyboardOpen);
        updateMessageBottomClearance(messageBox, preserveBottomAfterClearance);

        if (!keyboardOpen || !keepMessagesAtBottom || !messageBox) return;
        window.cancelAnimationFrame(scrollFrame);
        scrollFrame = window.requestAnimationFrame(() => {
          messageBox.scrollTop = messageBox.scrollHeight;
        });
      });
    };

    const main = document.querySelector('#app.app main');
    if (main && typeof MutationObserver === 'function') {
      mainObserver = new MutationObserver(() => applyViewportState());
      mainObserver.observe(main, { childList: true });
    }

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
      window.cancelAnimationFrame(clearanceFrame);
      window.cancelAnimationFrame(clearanceScrollFrame);
      mainObserver?.disconnect();
      overlayResizeObserver?.disconnect();
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
