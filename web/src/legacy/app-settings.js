// ChatDock legacy settings：配置中心路由、概览卡片、主题和侧栏。
function fmtTime(value) { try { return new Date(value).toLocaleString(); } catch { return ''; } }

function fmtBytes(value) {
  const n = Number(value || 0);
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1024 / 1024).toFixed(1) + ' MB';
}

const settingsModules = ['workspace', 'model', 'skills', 'tools', 'automation', 'data', 'security'];

function normalizeSettingsModule(name) {
  return settingsModules.includes(name) ? name : 'workspace';
}

function activeSettingsModule() {
  const active = document.querySelector('.module-tab.active');
  return normalizeSettingsModule((active || {}).dataset ? active.dataset.module : localStorage.getItem('chatdock.settingsModule'));
}

function settingsModuleFromPath() {
  const parts = window.location.pathname.split('/').filter(Boolean);
  if (parts[0] !== 'settings') return '';
  return normalizeSettingsModule(parts[1] || localStorage.getItem('chatdock.settingsModule') || 'workspace');
}

function replaceRoute(path) {
  if (window.history && window.history.replaceState && window.location.pathname !== path) {
    window.history.replaceState({chatdock: true}, '', path);
  }
}

function pushRoute(path) {
  if (window.history && window.history.pushState && window.location.pathname !== path) {
    window.history.pushState({chatdock: true}, '', path);
  }
}

function initSettingsRoute() {
  if (!window.history || !window.history.pushState) return;
  const routeModule = settingsModuleFromPath();
  if (routeModule) {
    switchSettingsModule(routeModule, {syncRoute: false, lazyLoad: false});
    toggleSettingsPanel(true, {syncRoute: false});
    replaceRoute('/settings/' + routeModule);
  } else {
    const savedModule = normalizeSettingsModule(localStorage.getItem('chatdock.settingsModule') || 'workspace');
    switchSettingsModule(savedModule, {syncRoute: false, lazyLoad: false});
  }
  window.addEventListener('popstate', syncSettingsFromRoute);
}

function syncSettingsFromRoute() {
  const routeModule = settingsModuleFromPath();
  if (routeModule) {
    switchSettingsModule(routeModule, {syncRoute: false});
    toggleSettingsPanel(true, {syncRoute: false});
  } else {
    toggleSettingsPanel(false, {syncRoute: false});
  }
}

function returnToChat() {
  toggleSettingsPanel(false);
}

function toggleSettingsPanel(force, options={}) {
  const appEl = document.getElementById('app');
  const mask = document.getElementById('settingsMask');
  if (!appEl) return;
  const next = typeof force === 'boolean' ? force : !appEl.classList.contains('settings-open');
  appEl.classList.toggle('settings-open', next);
  if (mask) mask.classList.toggle('show', next);
  if (next) {
    closeSidebarOnMobile();
    if (options.syncRoute !== false) pushRoute('/settings/' + activeSettingsModule());
  } else if (options.syncRoute !== false) {
    pushRoute('/');
  }
}

function closeSettingsPanel() { toggleSettingsPanel(false); }

function switchSettingsModule(name, options={}) {
  const moduleName = normalizeSettingsModule(name);
  localStorage.setItem('chatdock.settingsModule', moduleName);
  document.querySelectorAll('.module-tab').forEach(btn => btn.classList.toggle('active', btn.dataset.module === moduleName));
  document.querySelectorAll('.module-view').forEach(view => view.classList.toggle('active', view.dataset.moduleView === moduleName));
  if (options.syncRoute !== false) pushRoute('/settings/' + moduleName);
  if (options.lazyLoad === false) return;
  window.requestAnimationFrame(() => {
    if (moduleName === 'tools') loadMCPStatus();
    if (moduleName === 'data') loadDataStatus();
    if (moduleName === 'security') loadSystemStatus();
  });
}

async function refreshProductState() {
  await Promise.allSettled([loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
}

async function loadSetupStatus() {
  let s;
  try {
    s = await api('/api/setup/status');
  } catch (e) {
    if (setupBanner) {
      setupBanner.classList.remove('ok');
      setupBanner.classList.add('show');
      setupBanner.innerHTML = panelErrorHTML(e, 'product-refresh');
    }
    return;
  }
  if (!setupBanner) return;
  if (s.needs_setup) {
    setupBanner.classList.remove('ok');
    setupBanner.innerHTML = '<div><b>首次配置未完成</b><div class="hint">请配置模型供应商和默认工作空间，完成后即可开始对话。</div></div>' +
      '<button class="small" data-action="setup-wizard">开始引导</button>';
    setupBanner.classList.add('show');
  } else {
    setupBanner.innerHTML = '<div><b>系统已就绪</b><div class="hint">当前工作空间：' + escapeHtml(s.active_workspace || '-') + ' · 数据目录：' + escapeHtml(s.data_dir || '-') + '</div></div>';
    setupBanner.classList.add('show', 'ok');
  }
}

async function runSetupWizard() {
  const values = await showFormDialog({
    title: '首次配置',
    message: '配置默认工作空间和模型后即可开始对话。',
    confirmText: '完成初始化',
    fields: [
      {name: 'workspace_name', label: '默认工作空间名称', value: 'default', required: true},
      {name: 'base_url', label: '模型 Base URL', value: base_url.value || 'https://api.openai.com/v1', required: true},
      {name: 'model', label: '默认模型', value: model.value || 'gpt-4o-mini', required: true},
      {name: 'api_key', label: 'API Key（可留空）', type: 'password', value: ''},
      {name: 'system_prompt', label: '默认 System Prompt', type: 'textarea', rows: 4, value: system_prompt.value || '你是 ChatDock，本地优先 AI 工作台。默认用中文回答。'},
    ],
  });
  if (!values) return;
  await api('/api/setup/init', {method:'POST', body: JSON.stringify({
    workspace_name: values.workspace_name.trim() || 'default',
    base_url: values.base_url || '',
    model: values.model || '',
    api_key: values.api_key || '',
    system_prompt: values.system_prompt || '',
  })});
  await loadPrompts();
  await loadConfig();
  await refreshProductState();
  showToast('初始化完成', 'success');
}

async function loadWorkspaces() {
  try {
    const data = await api('/api/workspaces');
    workspaceItems = data.workspaces || [];
    renderWorkspaces();
  } catch (e) {
    workspaceItems = [];
    renderPanelError(workspaceCards, e, 'workspaces-load');
  }
}

function renderWorkspaces() {
  if (!workspaceCards) return;
  if (!workspaceItems.length) {
    workspaceCards.innerHTML = '<div class="empty compact">还没有工作空间，请创建第一个工作空间。</div>';
    return;
  }
  workspaceCards.innerHTML = workspaceItems.map(ws => '<div class="product-card ' + (ws.active ? 'active' : '') + '">' +
    '<div class="product-card-head"><div><b>' + escapeHtml(ws.name) + '</b><div class="hint">' + escapeHtml(ws.description || '') + '</div></div><span class="badge">' + (ws.active ? '当前' : '可切换') + '</span></div>' +
    '<div class="product-meta">模型：' + escapeHtml(ws.model || '-') + ' · 会话 ' + (ws.session_count || 0) + ' · 技能 ' + (ws.enabled_skill_count || 0) + '/' + (ws.skill_count || 0) + ' · 任务 ' + (ws.task_count || 0) + '</div>' +
    workspaceActionsHTML(ws) +
  '</div>').join('');
}

function workspaceActionsHTML(ws) {
  if (!ws || ws.active) return '';
  const id = ws.id || ws.name || '';
  const name = ws.name || ws.id || '';
  const canDelete = id !== 'default' && workspaceItems.length > 1;
  return '<div class="product-actions">' +
    '<button class="secondary small" data-action="workspace-select" data-id="' + dataAttr(id) + '">切换到此工作空间</button>' +
    (canDelete ? '<button class="danger small" data-action="workspace-delete" data-id="' + dataAttr(id) + '" data-name="' + dataAttr(name) + '">删除</button>' : '') +
  '</div>';
}

async function deleteWorkspace(id, name) {
  if (!id) return;
  const ok = await showChoice('删除工作空间', '确定删除工作空间「' + (name || id) + '」？这会删除该工作空间下的配置、技能、任务和会话。', {confirmText: '删除', danger: true});
  if (!ok) return;
  const data = await api('/api/workspaces/' + encodeURIComponent(id), {method:'DELETE'});
  workspaceItems = data.workspaces || [];
  renderWorkspaces();
  await loadPrompts();
  await Promise.allSettled([loadSetupStatus(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
  showToast('工作空间已删除', 'success');
}

async function loadModelProviders() {
  try {
    const data = await api('/api/model-providers');
    providerItems = data.providers || [];
    renderModelProviders();
  } catch (e) {
    providerItems = [];
    renderPanelError(providerCards, e, 'model-providers-load');
  }
}

function renderModelProviders() {
  if (!providerCards) return;
  if (!providerItems.length) {
    providerCards.innerHTML = '<div class="empty compact">还没有模型供应商配置。</div>';
    return;
  }
  providerCards.innerHTML = providerItems.map(p => '<div class="product-card">' +
    '<div class="product-card-head"><div><b>' + escapeHtml(p.name || p.id) + '</b><div class="hint">' + escapeHtml(p.base_url || '-') + '</div></div><span class="badge">' + escapeHtml(p.type || 'openai') + '</span></div>' +
    '<div class="product-meta">默认模型：' + escapeHtml(p.default_model || '-') + ' · Key：' + (p.has_api_key ? escapeHtml(p.api_key_masked || '******') : '未设置') + ' · 工作空间：' + escapeHtml(p.workspace_name || '-') + '</div>' +
  '</div>').join('');
}

async function testModelProvider() {
  try {
    const data = await api('/api/model-providers/test', {method:'POST', body:'{}'});
    showToast(data.ok ? '模型连接正常：' + (data.model || '') : '模型连接失败：' + (data.error || 'unknown'), data.ok ? 'success' : 'error');
  } catch (e) {
    showToast('模型连接失败：' + e.message, 'error');
  }
}

async function showPromptPreview() {
  const workspaceID = promptSelector.value || 'default';
  const data = await api('/api/workspaces/' + encodeURIComponent(workspaceID) + '/prompt-preview');
  promptPreview.hidden = false;
  promptPreview.textContent = data.content || '(空)';
}

async function loadDataStatus() {
  let data;
  try {
    data = await api('/api/data/status');
  } catch (e) {
    dataStatusCache = null;
    renderPanelError(dataStatus, e, 'data-status');
    return;
  }
  dataStatusCache = data;
  if (!dataStatus) return;
  dataStatus.innerHTML = [
    ['数据目录', data.data_dir || '-'],
    ['数据库', data.database_path || '-'],
    ['数据库大小', fmtBytes(data.database_size_bytes)],
    ['工作空间', String(data.workspace_count || 0)],
    ['会话', String(data.session_count || 0)],
    ['WAL', data.wal_enabled ? '启用' : '未检测到'],
  ].map(item => '<div class="stat-card"><div class="stat-label">' + escapeHtml(item[0]) + '</div><div class="stat-value">' + escapeHtml(item[1]) + '</div></div>').join('');
}

async function loadSystemStatus() {
  let data;
  try {
    data = await api('/api/system/status');
  } catch (e) {
    renderPanelError(systemStatus, e, 'system-status');
    return;
  }
  if (!systemStatus) return;
  systemStatus.innerHTML = '<div class="product-card"><div class="product-card-head"><div><b>ChatDock</b><div class="hint">' + escapeHtml(data.addr || '') + '</div></div><span class="badge">' + (data.ok ? 'healthy' : 'unknown') + '</span></div>' +
    '<div class="product-meta">Web：' + escapeHtml(data.web_dir || '-') + ' · DB：' + escapeHtml(data.database || '-') + ' · 当前工作空间：' + escapeHtml((data.setup || {}).active_workspace || '-') + '</div></div>';
}

async function loadMCPStatus() {
  if (!mcpStatusCards) return;
  mcpStatusCards.innerHTML = '<div class="hint">正在检测 MCP Server...</div>';
  try {
    const data = await api('/api/mcp/status');
    const servers = data.servers || [];
    if (!servers.length) {
      mcpStatusCards.innerHTML = '<div class="empty compact">尚未配置 MCP Server。添加后可在这里查看状态、权限和确认规则。</div>';
      return;
    }
    mcpStatusCards.innerHTML = servers.map(s => '<div class="product-card">' +
      '<div class="product-card-head"><div><b>' + escapeHtml(s.name) + '</b><div class="hint">' + escapeHtml(s.url || '-') + '</div></div><span class="badge ' + (String(s.last_status).startsWith('ok') ? 'ok' : 'warn') + '">' + escapeHtml(s.last_status || 'unknown') + '</span></div>' +
      '<div class="product-meta">allow ' + s.allow_count + ' · deny ' + s.deny_count + ' · confirm ' + s.confirm_count + ' · token ' + (s.has_token ? '已配置' : '无') + '</div>' +
      (s.last_error ? '<div class="task-error">' + escapeHtml(s.last_error) + '</div>' : '') +
    '</div>').join('');
  } catch (e) {
    mcpStatusCards.innerHTML = '<div class="task-error">MCP 状态检测失败：' + escapeHtml(e.message) + '</div>';
  }
}

function initTheme() {
  const saved = localStorage.getItem('chatdock.theme') || 'night';
  setTheme(saved === 'day' ? 'day' : 'night');
}

function toggleTheme() {
  const next = document.body.classList.contains('theme-light') ? 'night' : 'day';
  setTheme(next);
}

function setTheme(theme) {
  const isDay = theme === 'day';
  document.body.classList.toggle('theme-light', isDay);
  document.body.classList.toggle('theme-night', !isDay);
  localStorage.setItem('chatdock.theme', isDay ? 'day' : 'night');
  if (typeof themeToggle !== 'undefined' && themeToggle) {
    themeToggle.textContent = isDay ? '白天' : '夜晚';
    themeToggle.title = isDay ? '当前：白天，点击切换夜晚' : '当前：夜晚，点击切换白天';
    themeToggle.setAttribute('aria-label', themeToggle.title);
  }
}

function initSidebar() {
  const saved = localStorage.getItem('chatdock.sidebarCollapsed');
  const collapsed = saved == null ? isMobileViewport() : saved === '1';
  setSidebarCollapsed(collapsed);
  window.addEventListener('resize', syncSidebarMask);
}

function toggleSidebar() {
  setSidebarCollapsed(!app.classList.contains('sidebar-collapsed'));
}

function setSidebarCollapsed(collapsed) {
  app.classList.toggle('sidebar-collapsed', collapsed);
  sidebarToggle.textContent = collapsed ? '›' : '‹';
  sidebarToggle.title = collapsed ? '展开侧栏' : '折叠侧栏';
  sidebarToggle.setAttribute('aria-label', sidebarToggle.title);
  localStorage.setItem('chatdock.sidebarCollapsed', collapsed ? '1' : '0');
  syncSidebarMask();
}

function isMobileViewport() {
  return window.matchMedia('(max-width: 720px)').matches;
}

function syncSidebarMask() {
  sidebarMask.classList.toggle('show', isMobileViewport() && !app.classList.contains('sidebar-collapsed'));
}

function closeSidebarOnMobile() {
  if (isMobileViewport()) setSidebarCollapsed(true);
}
