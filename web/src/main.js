import { pageHtml } from './pageHtml.js';
import './modules/app.css';
import markdownSource from './modules/markdown.js?raw';
import mcpSource from './modules/mcp.js?raw';
import appStateSource from './modules/app-state.js?raw';
import appUiSource from './modules/app-ui.js?raw';
import appAuthSource from './modules/app-auth.js?raw';
import appSettingsSource from './modules/app-settings.js?raw';
import appConfigSource from './modules/app-config.js?raw';
import appSkillsSource from './modules/app-skills.js?raw';
import appTasksSource from './modules/app-tasks.js?raw';
import appRunsSource from './modules/app-runs.js?raw';
import appChatSource from './modules/app-chat.js?raw';
import appWorkspacesSource from './modules/app-workspaces.js?raw';
import appBootSource from './modules/app-boot.js?raw';

const appSources = [
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

function mountHTML() {
  const root = document.getElementById('root');
  if (!root) throw new Error('ChatDock root element not found');
  root.innerHTML = pageHtml;
}

function runClassicModule(source, index) {
  const script = document.createElement('script');
  script.dataset.chatdockModule = String(index + 1);
  script.textContent = source;
  document.body.appendChild(script);
}

function bootChatDock() {
  if (window.__chatdockAppLoaded) return;
  window.__chatdockAppLoaded = true;
  mountHTML();
  // 这些文件仍按 classic script 执行，避免一次性重写全局 DOM/函数依赖导致功能回退。
  appSources.forEach(runClassicModule);
}

bootChatDock();
