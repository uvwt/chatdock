import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App.jsx';
import './app.css';
import { loadMarkdownRenderer } from './lib/markdownLoader.js';

createRoot(document.getElementById('root')).render(<App />);

// Markdown 解析器是懒加载分包（见 lib/markdownLoader.js）。这里立即触发同一个单例 import，
// 让打开会话时的等待多半已经完成；会话渲染路径本身也会 await 它，因此不依赖预热的时机。
loadMarkdownRenderer().catch(() => {});
