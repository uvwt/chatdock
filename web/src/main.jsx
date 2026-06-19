import React, { useEffect } from 'react';
import { createRoot } from 'react-dom/client';
import { pageHtml } from './pageHtml.js';
import './legacy/app.css';
import markdownSource from './legacy/markdown.js?raw';
import mcpSource from './legacy/mcp.js?raw';
import appStateSource from './legacy/app-state.js?raw';
import appUiSource from './legacy/app-ui.js?raw';
import appAuthSource from './legacy/app-auth.js?raw';
import appSettingsSource from './legacy/app-settings.js?raw';
import appConfigSource from './legacy/app-config.js?raw';
import appSkillsSource from './legacy/app-skills.js?raw';
import appTasksSource from './legacy/app-tasks.js?raw';
import appRunsSource from './legacy/app-runs.js?raw';
import appChatSource from './legacy/app-chat.js?raw';
import appWorkspacesSource from './legacy/app-workspaces.js?raw';
import appBootSource from './legacy/app-boot.js?raw';

const legacySources = [
  markdownSource,
  mcpSource,
  appStateSource,
  appUiSource,
  appAuthSource,
  appSettingsSource,
  appConfigSource,
  appSkillsSource,
  appTasksSource,
  appRunsSource,
  appChatSource,
  appWorkspacesSource,
  appBootSource,
];

function runClassicScript(source) {
  const script = document.createElement('script');
  script.textContent = source;
  document.body.appendChild(script);
}

function ChatDockShell() {
  useEffect(() => {
    if (window.__chatdockLegacyLoaded) return;
    window.__chatdockLegacyLoaded = true;
    // 先把 legacy 前端分片作为 classic script 按顺序挂到全局，保持旧 DOM id 全局访问兼容。
    legacySources.forEach(runClassicScript);
  }, []);

  return <div dangerouslySetInnerHTML={{ __html: pageHtml }} />;
}

createRoot(document.getElementById('root')).render(<ChatDockShell />);
