// Markdown 解析器是懒加载分包。这里用单例 Promise 暴露加载入口，
// 让预热、Suspense 和"正文渲染前等待解析器"三处共用同一次 import，避免重复请求与竞态。
let markdownModule = null;
let resolvedModule = null;

export function loadMarkdownRenderer() {
  if (!markdownModule) {
    markdownModule = import('../components/markdown.jsx').then(mod => {
      resolvedModule = mod;
      return mod;
    });
  }
  return markdownModule;
}

// React.lazy 的内部状态只在首次渲染时初始化，因此即使分包已下载，
// 首帧仍会 suspend 一次。已就绪时直接同步取用模块可以跳过这一帧。
export function markdownRendererIfReady() {
  return resolvedModule;
}

