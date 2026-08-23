import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App.jsx';
import './app.css';

createRoot(document.getElementById('root')).render(<App />);

// Markdown 解析器是懒加载分包（见 components/base.jsx）。首屏渲染后立即在空闲时预取，
// 否则第一条带表格的历史消息会先按纯文本高度布局、分包到达后才塌成真实表格，
// 消息区高度突变又被贴底自动滚动放大成整屏闪动。
function prefetchMarkdownRenderer() {
  void import('./components/markdown.jsx');
}

if (typeof window.requestIdleCallback === 'function') {
  window.requestIdleCallback(prefetchMarkdownRenderer, { timeout: 2000 });
} else {
  window.setTimeout(prefetchMarkdownRenderer, 200);
}
