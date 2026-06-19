import React, { useEffect } from 'react';
import { createRoot } from 'react-dom/client';
import { pageHtml } from './pageHtml.js';
import './legacy/app.css';
import markdownSource from './legacy/markdown.js?raw';
import mcpSource from './legacy/mcp.js?raw';
import appSource from './legacy/app.js?raw';

function runClassicScript(source) {
  const script = document.createElement('script');
  script.textContent = source;
  document.body.appendChild(script);
}

function ChatDockShell() {
  useEffect(() => {
    if (window.__chatdockLegacyLoaded) return;
    window.__chatdockLegacyLoaded = true;
    // 先把现有前端逻辑作为 classic script 挂到全局，兼容旧的内联事件和 DOM id 访问。
    runClassicScript(markdownSource);
    runClassicScript(mcpSource);
    runClassicScript(appSource);
  }, []);

  return <div dangerouslySetInnerHTML={{ __html: pageHtml }} />;
}

createRoot(document.getElementById('root')).render(<ChatDockShell />);
