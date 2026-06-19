let current = null;
let busy = false;
let activeAbortController = null;
let activeAssistantEl = null;
let activeReasoningEl = null;
let activeAnswerEl = null;
let streamPaused = false;
let pendingDelta = '';
let pendingReasoning = '';
let activeAnswerBuffer = '';
let activeReasoningBuffer = '';
let skillItems = [];
let scheduledTaskItems = [];
let workspaceItems = [];
let providerItems = [];
let dataStatusCache = null;

let appDialogEl = null;
let toastTimer = null;

function closeAppDialog(value) {
  if (!appDialogEl) return;
  const resolve = appDialogEl.__resolve;
  document.removeEventListener('keydown', appDialogEl.__onKeydown);
  appDialogEl.remove();
  appDialogEl = null;
  if (resolve) resolve(value);
}

function showToast(message, variant='info') {
  let toast = document.getElementById('appToast');
  if (!toast) {
    toast = document.createElement('div');
    toast.id = 'appToast';
    toast.className = 'app-toast';
    document.body.appendChild(toast);
  }
  toast.className = 'app-toast show ' + (variant || 'info');
  toast.textContent = message || '';
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast.classList.remove('show'), 3200);
}

function showChoice(title, message, options={}) {
  return showAppDialog({
    title,
    message,
    variant: options.variant || 'info',
    confirmText: options.confirmText || '确定',
    cancelText: options.cancelText || '取消',
    danger: !!options.danger,
  });
}

function showFormDialog(options={}) {
  return showAppDialog({...options, type: 'form', confirmText: options.confirmText || '保存'});
}

function showAppDialog(config={}) {
  if (appDialogEl) closeAppDialog(null);
  return new Promise(resolve => {
    const backdrop = document.createElement('div');
    backdrop.className = 'app-modal-backdrop show';
    backdrop.__resolve = resolve;
    const fields = config.fields || [];
    const message = config.message ? '<div class="app-modal-message">' + escapeHtml(config.message) + '</div>' : '';
    const fieldHTML = fields.map(appDialogFieldHTML).join('');
    const cancelButton = config.hideCancel ? '' : '<button type="button" class="secondary app-modal-cancel">' + escapeHtml(config.cancelText || '取消') + '</button>';
    const confirmClass = config.danger ? 'danger' : '';
    backdrop.innerHTML = '<div class="app-modal-card ' + escapeHtml(config.variant || '') + '" role="dialog" aria-modal="true">' +
      '<form class="app-modal-form">' +
        '<div class="app-modal-title">' + escapeHtml(config.title || '确认') + '</div>' +
        message +
        '<div class="app-modal-fields">' + fieldHTML + '</div>' +
        '<div class="app-modal-actions">' + cancelButton + '<button type="submit" class="' + confirmClass + '">' + escapeHtml(config.confirmText || '确定') + '</button></div>' +
      '</form>' +
    '</div>';
    const form = backdrop.querySelector('form');
    const cancel = backdrop.querySelector('.app-modal-cancel');
    const finishCancel = () => closeAppDialog(null);
    const finishSubmit = event => {
      event.preventDefault();
      if (config.type !== 'form') {
        closeAppDialog(true);
        return;
      }
      const values = {};
      fields.forEach(field => {
        const el = form.querySelector('[data-field="' + field.name + '"]');
        values[field.name] = el ? el.value : '';
      });
      closeAppDialog(values);
    };
    backdrop.__onKeydown = event => {
      if (event.key === 'Escape') finishCancel();
    };
    form.addEventListener('submit', finishSubmit);
    if (cancel) cancel.addEventListener('click', finishCancel);
    backdrop.addEventListener('click', event => {
      if (event.target === backdrop) finishCancel();
    });
    document.addEventListener('keydown', backdrop.__onKeydown);
    document.body.appendChild(backdrop);
    appDialogEl = backdrop;
    const firstInput = form.querySelector('input, textarea, select, button');
    if (firstInput) firstInput.focus();
  });
}

function appDialogFieldHTML(field) {
  const name = escapeHtml(field.name || 'field');
  const label = escapeHtml(field.label || field.name || '');
  const value = escapeHtml(field.value ?? '');
  const placeholder = field.placeholder ? ' placeholder="' + escapeHtml(field.placeholder) + '"' : '';
  const hint = field.hint ? '<div class="app-modal-field-hint">' + escapeHtml(field.hint) + '</div>' : '';
  const required = field.required ? ' required' : '';
  let control;
  if (field.type === 'textarea') {
    control = '<textarea data-field="' + name + '" rows="' + (field.rows || 5) + '"' + placeholder + required + '>' + value + '</textarea>';
  } else if (field.type === 'select') {
    const options = (field.options || []).map(opt => {
      const optValue = typeof opt === 'string' ? opt : opt.value;
      const optLabel = typeof opt === 'string' ? opt : opt.label;
      return '<option value="' + escapeHtml(optValue) + '" ' + (String(optValue) === String(field.value || '') ? 'selected' : '') + '>' + escapeHtml(optLabel) + '</option>';
    }).join('');
    control = '<select data-field="' + name + '"' + required + '>' + options + '</select>';
  } else {
    const type = escapeHtml(field.type || 'text');
    const min = field.min != null ? ' min="' + escapeHtml(field.min) + '"' : '';
    const max = field.max != null ? ' max="' + escapeHtml(field.max) + '"' : '';
    const step = field.step != null ? ' step="' + escapeHtml(field.step) + '"' : '';
    control = '<input data-field="' + name + '" type="' + type + '" value="' + value + '"' + placeholder + required + min + max + step + ' />';
  }
  return '<label class="app-modal-field"><span>' + label + '</span>' + control + hint + '</label>';
}


function authHeaders(extra={}) {
  const token = localStorage.getItem('chatdock.authToken') || '';
  return token ? {'Authorization':'Bearer ' + token, ...extra} : extra;
}

function authURL(path) {
  const token = localStorage.getItem('chatdock.authToken') || '';
  if (!token) return path;
  const sep = path.includes('?') ? '&' : '?';
  return path + sep + 'token=' + encodeURIComponent(token);
}

async function api(path, opt={}) {
  const res = await fetch(path, {...opt, headers: authHeaders({'Content-Type':'application/json', ...(opt.headers || {})})});
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.error || res.statusText);
    err.status = res.status;
    err.path = path;
    if (res.status === 401 && path !== '/api/auth/login') showLoginPage(err);
    throw err;
  }
  return data;
}

function loginFormHTML(error) {
  const message = error ? (error.status === 401 ? '登录已过期，请重新登录。' : error.message) : '请输入 ChatDock 账号和密码。';
  return '<form class="login-card" onsubmit="submitLogin(event)">' +
    '<div class="login-brand">ChatDock</div>' +
    '<b>登录 ChatDock</b>' +
    '<div class="hint">' + escapeHtml(message) + '</div>' +
    '<label>账号</label><input id="login_username" autocomplete="username" placeholder="账号" />' +
    '<label>密码</label><input id="login_credential" type="password" autocomplete="current-password" placeholder="密码" />' +
    '<div id="loginError" class="task-error"></div>' +
    '<button type="submit" class="login-submit">登录</button>' +
  '</form>';
}

function panelErrorHTML(error, retryCall) {
  if (error && error.status === 401) return '<div class="empty compact">登录已过期，请重新登录。</div>';
  return '<div class="empty compact error-state"><b>加载失败</b><div class="hint">' + escapeHtml(error.message || '请稍后重试') + '</div><div class="settings-actions">' +
    '<button class="secondary small" onclick="' + retryCall + '">重试</button></div></div>';
}

function renderPanelError(target, error, retryCall) {
  if (target) target.innerHTML = panelErrorHTML(error, retryCall);
}

function showLoginForm() {
  showLoginPage();
}

function showLoginPage(error) {
  document.body.classList.add('auth-page-visible');
  let page = document.getElementById('authPage');
  if (!page) {
    page = document.createElement('div');
    page.id = 'authPage';
    page.className = 'auth-page';
    document.body.appendChild(page);
  }
  page.innerHTML = '<div class="auth-shell">' + loginFormHTML(error) + '</div>';
  const username = document.getElementById('login_username');
  if (username) username.focus();
}

function hideLoginPage() {
  document.body.classList.remove('auth-page-visible');
  const page = document.getElementById('authPage');
  if (page) page.remove();
}

async function submitLogin(event) {
  event.preventDefault();
  const username = ((document.getElementById('login_username') || {}).value || '').trim();
  const credential = ((document.getElementById('login_credential') || {}).value || '').trim();
  const errorBox = document.getElementById('loginError');
  if (errorBox) errorBox.textContent = '';
  try {
    const data = await api('/api/auth/login', {method:'POST', body: JSON.stringify({username, credential})});
    if (data.token) localStorage.setItem('chatdock.authToken', data.token);
    hideLoginPage();
    await refreshAfterLogin();
  } catch (e) {
    if (errorBox) errorBox.textContent = '登录失败：' + e.message;
  }
}

async function refreshAfterLogin() {
  await Promise.allSettled([refreshProductState(), loadConfig(), loadMCPConfig(), loadSkills(), loadScheduledTasks(), loadSessions()]);
}

async function ensureAuthenticated() {
  try {
    const status = await api('/api/auth/status');
    if (status.enabled && status.login_enabled && !localStorage.getItem('chatdock.authToken')) {
      showLoginPage();
      return false;
    }
    hideLoginPage();
    return true;
  } catch (e) {
    if (e.status === 401) {
      showLoginPage(e);
      return false;
    }
    throw e;
  }
}

async function startApp() {
  initTheme();
  initSidebar();
  try {
    const ok = await ensureAuthenticated();
    if (!ok) return;
    await refreshAfterLogin();
  } catch (e) {
    showLoginPage(e);
  }
}

function fmtTime(value) { try { return new Date(value).toLocaleString(); } catch { return ''; } }

function fmtBytes(value) {
  const n = Number(value || 0);
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1024 / 1024).toFixed(1) + ' MB';
}

function toggleSettingsPanel(force) {
  const appEl = document.getElementById('app');
  const mask = document.getElementById('settingsMask');
  if (!appEl) return;
  const next = typeof force === 'boolean' ? force : !appEl.classList.contains('settings-open');
  appEl.classList.toggle('settings-open', next);
  if (mask) mask.classList.toggle('show', next);
}

function closeSettingsPanel() { toggleSettingsPanel(false); }

function switchSettingsModule(name) {
  document.querySelectorAll('.module-tab').forEach(btn => btn.classList.toggle('active', btn.dataset.module === name));
  document.querySelectorAll('.module-view').forEach(view => view.classList.toggle('active', view.dataset.moduleView === name));
  if (name === 'tools') loadMCPStatus();
  if (name === 'data') loadDataStatus();
  if (name === 'security') loadSystemStatus();
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
      setupBanner.innerHTML = panelErrorHTML(e, 'refreshProductState()');
    }
    return;
  }
  if (!setupBanner) return;
  if (s.needs_setup) {
    setupBanner.classList.remove('ok');
    setupBanner.innerHTML = '<div><b>首次配置未完成</b><div class="hint">请配置模型供应商和默认工作空间，完成后即可开始对话。</div></div>' +
      '<button class="small" onclick="runSetupWizard()">开始引导</button>';
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
    renderPanelError(workspaceCards, e, 'loadWorkspaces()');
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
    (!ws.active ? '<button class="secondary small" onclick="selectWorkspace(\'' + escapeHtml(ws.id) + '\')">切换到此工作空间</button>' : '') +
  '</div>').join('');
}

async function loadModelProviders() {
  try {
    const data = await api('/api/model-providers');
    providerItems = data.providers || [];
    renderModelProviders();
  } catch (e) {
    providerItems = [];
    renderPanelError(providerCards, e, 'loadModelProviders()');
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
    renderPanelError(dataStatus, e, 'loadDataStatus()');
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
    renderPanelError(systemStatus, e, 'loadSystemStatus()');
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

async function loadConfig() {
  let c;
  try {
    c = await api('/api/config');
  } catch (e) {
    if (e.status === 401) api_key.placeholder = '访问 Token 已失效，请在安全页重新设置';
    return;
  }
  base_url.value = c.base_url || '';
  api_key.value = '';
  api_key.placeholder = c.has_api_key ? '已保存，留空不修改' : '未设置';
  model.value = c.model || '';
  system_prompt.value = c.system_prompt || '';
  max_context_messages.value = c.max_context_messages || 12;
  temperature.value = c.temperature ?? 0.7;
  enable_thinking.checked = !!c.enable_thinking;
  hide_thinking.checked = c.hide_thinking !== false;
}

async function loadMCPConfig() {
  let c;
  try {
    c = await api('/api/mcp-config');
  } catch (e) {
    if (mcp_config) mcp_config.value = e.status === 401 ? '访问 Token 已失效，请在安全页重新设置。' : ('加载失败：' + e.message);
    return;
  }
  const fallback = ['{', '  "servers": {}', '}'].join('\n') + '\n';
  mcp_config.value = c.content || fallback;
}

async function loadSkills() {
  try {
    const data = await api('/api/skills');
    skillItems = data.skills || [];
    renderSkills();
  } catch (e) {
    skillItems = [];
    renderPanelError(skills, e, 'loadSkills()');
  }
}

function renderSkills() {
  const q = ((document.getElementById('skillSearch') || {}).value || '').trim().toLowerCase();
  const list = q ? skillItems.filter(s => [s.name, s.description, s.content].some(v => String(v || '').toLowerCase().includes(q))) : skillItems;
  if (!list.length) {
    skills.innerHTML = '<div class="hint">暂无技能。技能会作为当前提示词空间的补充系统指令注入模型请求。</div>';
    return;
  }
  skills.innerHTML = list.map(s => '<div class="skill-card">' +
    '<div class="skill-head">' +
      '<div><div class="skill-name">' + escapeHtml(s.name || '未命名技能') + '</div>' +
      '<div class="skill-desc">' + escapeHtml(s.description || '无描述') + '</div></div>' +
      '<div class="skill-actions">' +
        '<button class="secondary small" onclick="editSkill(\'' + escapeHtml(s.id) + '\')">编辑</button>' +
        '<button class="danger small" onclick="deleteSkill(\'' + escapeHtml(s.id) + '\')">删除</button>' +
      '</div>' +
    '</div>' +
    '<label class="skill-toggle"><input type="checkbox" ' + (s.enabled ? 'checked' : '') + ' onchange="toggleSkill(\'' + escapeHtml(s.id) + '\', this.checked)" /> 启用</label>' +
  '</div>').join('');
}

function findSkill(id) {
  return skillItems.find(s => s.id === id) || null;
}

async function editSkill(id) {
  if (busy) return;
  const existing = id ? findSkill(id) : null;
  const values = await showFormDialog({
    title: existing ? '编辑技能' : '新增技能',
    confirmText: existing ? '保存技能' : '新增技能',
    fields: [
      {name: 'name', label: '技能名称', value: existing ? existing.name : '', required: true},
      {name: 'description', label: '技能描述（可选）', type: 'textarea', rows: 2, value: existing ? (existing.description || '') : ''},
      {name: 'content', label: '技能内容', type: 'textarea', rows: 8, value: existing ? (existing.content || '') : '', required: true},
    ],
  });
  if (!values) return;
  if (!values.name.trim() || !values.content.trim()) {
    showToast('技能名称和内容不能为空', 'error');
    return;
  }
  const payload = {name: values.name.trim(), description: values.description || '', content: values.content.trim(), enabled: existing ? !!existing.enabled : true};
  const data = await api(existing ? '/api/skills/' + encodeURIComponent(existing.id) : '/api/skills', {method: existing ? 'PUT' : 'POST', body: JSON.stringify(payload)});
  skillItems = data.skills || [];
  renderSkills();
  showToast(existing ? '技能已保存' : '技能已新增', 'success');
}

async function toggleSkill(id, enabled) {
  const existing = findSkill(id);
  if (!existing) return;
  const data = await api('/api/skills/' + encodeURIComponent(id), {method:'PUT', body: JSON.stringify({
    name: existing.name,
    description: existing.description || '',
    content: existing.content || '',
    enabled: !!enabled,
  })});
  skillItems = data.skills || [];
  renderSkills();
}

async function deleteSkill(id) {
  const existing = findSkill(id);
  if (!existing) return;
  const ok = await showChoice('删除技能', '确定删除技能「' + existing.name + '」？此操作不可恢复。', {confirmText: '删除', danger: true});
  if (!ok) return;
  const data = await api('/api/skills/' + encodeURIComponent(id), {method:'DELETE'});
  skillItems = data.skills || [];
  renderSkills();
  showToast('技能已删除', 'success');
}


async function loadScheduledTasks() {
  try {
    const data = await api('/api/scheduled-tasks');
    scheduledTaskItems = data.tasks || [];
    renderScheduledTasks();
  } catch (e) {
    scheduledTaskItems = [];
    renderPanelError(scheduledTasks, e, 'loadScheduledTasks()');
  }
}

function renderScheduledTasks() {
  const q = ((document.getElementById('taskSearch') || {}).value || '').trim().toLowerCase();
  const list = q ? scheduledTaskItems.filter(t => [t.title, t.prompt, t.schedule_type, t.last_status, t.last_error].some(v => String(v || '').toLowerCase().includes(q))) : scheduledTaskItems;
  if (!list.length) {
    scheduledTasks.innerHTML = '<div class="hint">暂无定时任务。任务会按当前提示词空间运行，并把结果写入专属会话。</div>';
    return;
  }
  scheduledTasks.innerHTML = list.map(t => '<div class="task-card">' +
    '<div class="task-head">' +
      '<div><div class="task-name">' + escapeHtml(t.title || '未命名任务') + (t.running ? ' · 运行中' : '') + '</div>' +
      '<div class="task-desc">' + escapeHtml((t.prompt || '').slice(0, 120) || '无提示内容') + '</div></div>' +
      '<div class="task-actions">' +
        '<span class="badge ' + taskStatusClass(t) + '">' + taskStatusLabel(t) + '</span>' +
        '<button class="secondary small" onclick="runScheduledTaskNow(\'' + escapeHtml(t.id) + '\')" ' + (t.running ? 'disabled' : '') + '>立即运行</button>' +
        '<button class="secondary small" onclick="editScheduledTask(\'' + escapeHtml(t.id) + '\')">编辑</button>' +
        '<button class="danger small" onclick="deleteScheduledTask(\'' + escapeHtml(t.id) + '\')">删除</button>' +
      '</div>' +
    '</div>' +
    '<label class="task-toggle"><input type="checkbox" ' + (t.enabled ? 'checked' : '') + ' onchange="toggleScheduledTask(\'' + escapeHtml(t.id) + '\', this.checked)" /> 启用</label>' +
    '<div class="task-meta">' + scheduleSummary(t) + '</div>' +
    (t.last_error ? '<div class="task-error">上次错误：' + escapeHtml(t.last_error) + '</div>' : '') +
  '</div>').join('');
}

function taskStatusLabel(t) {
  if (t.running) return '运行中';
  if (t.last_status === 'success') return '成功';
  if (t.last_status === 'failed') return '失败';
  return t.enabled ? '已启用' : '已暂停';
}

function taskStatusClass(t) {
  if (t.running) return 'warn';
  if (t.last_status === 'success') return 'ok';
  if (t.last_status === 'failed') return 'danger-badge';
  return t.enabled ? 'ok' : 'warn';
}

function scheduleSummary(t) {
  const next = t.next_run_at ? fmtTime(t.next_run_at) : '未计划';
  const last = t.last_run_at ? fmtTime(t.last_run_at) : '未运行';
  let plan = '一次性：' + (t.run_at ? fmtTime(t.run_at) : next);
  if (t.schedule_type === 'daily') plan = '每天 ' + (t.time_of_day || '--:--');
  if (t.schedule_type === 'interval') plan = '每 ' + (t.interval_minutes || 0) + ' 分钟';
  return escapeHtml(plan + ' · 下次：' + next + ' · 上次：' + last);
}

function findScheduledTask(id) {
  return scheduledTaskItems.find(t => t.id === id) || null;
}

function defaultRunAtValue() {
  const d = new Date(Date.now() + 60 * 60 * 1000);
  const pad = n => String(n).padStart(2, '0');
  return d.getFullYear() + '-' + pad(d.getMonth()+1) + '-' + pad(d.getDate()) + 'T' + pad(d.getHours()) + ':' + pad(d.getMinutes());
}

async function editScheduledTask(id) {
  if (busy) return;
  const existing = id ? findScheduledTask(id) : null;
  const values = await showFormDialog({
    title: existing ? '编辑自动化任务' : '新增自动化任务',
    message: '选择调度类型后，只需要填写对应的时间字段。',
    confirmText: existing ? '保存任务' : '新增任务',
    fields: [
      {name: 'title', label: '任务标题', value: existing ? existing.title : '', required: true},
      {name: 'prompt', label: '任务提示词', type: 'textarea', rows: 6, value: existing ? (existing.prompt || '') : '', required: true},
      {name: 'schedule_type', label: '调度类型', type: 'select', value: existing ? existing.schedule_type : 'once', options: [{value: 'once', label: '一次性'}, {value: 'daily', label: '每天固定时间'}, {value: 'interval', label: '按分钟间隔'}]},
      {name: 'run_at', label: '一次性运行时间', type: 'datetime-local', value: existing && existing.run_at ? existing.run_at.slice(0, 16) : defaultRunAtValue()},
      {name: 'time_of_day', label: '每天运行时间', type: 'time', value: existing ? (existing.time_of_day || '09:00') : '09:00'},
      {name: 'interval_minutes', label: '间隔分钟数', type: 'number', min: 1, step: 1, value: existing && existing.interval_minutes ? String(existing.interval_minutes) : '60'},
    ],
  });
  if (!values) return;
  const titleValue = (values.title || '').trim();
  const promptValue = (values.prompt || '').trim();
  const typeValue = (values.schedule_type || '').trim().toLowerCase();
  if (!titleValue || !promptValue) {
    showToast('任务标题和提示词不能为空', 'error');
    return;
  }
  if (!['once', 'daily', 'interval'].includes(typeValue)) {
    showToast('调度类型只能是 once、daily 或 interval', 'error');
    return;
  }
  const payload = {title: titleValue, prompt: promptValue, enabled: existing ? !!existing.enabled : true, schedule_type: typeValue};
  if (typeValue === 'once') {
    if (!values.run_at || !values.run_at.trim()) return showToast('请填写一次性运行时间', 'error');
    payload.run_at = values.run_at.trim();
  } else if (typeValue === 'daily') {
    if (!values.time_of_day || !values.time_of_day.trim()) return showToast('请填写每天运行时间', 'error');
    payload.time_of_day = values.time_of_day.trim();
  } else {
    const minutes = Number(values.interval_minutes || 0);
    if (!Number.isFinite(minutes) || minutes <= 0) {
      showToast('间隔分钟数必须大于 0', 'error');
      return;
    }
    payload.interval_minutes = Math.floor(minutes);
  }
  const data = await api(existing ? '/api/scheduled-tasks/' + encodeURIComponent(existing.id) : '/api/scheduled-tasks', {method: existing ? 'PUT' : 'POST', body: JSON.stringify(payload)});
  scheduledTaskItems = data.tasks || [];
  renderScheduledTasks();
  showToast(existing ? '任务已保存' : '任务已新增', 'success');
}

async function toggleScheduledTask(id, enabled) {
  const existing = findScheduledTask(id);
  if (!existing) return;
  const payload = {title: existing.title, prompt: existing.prompt, enabled: !!enabled, schedule_type: existing.schedule_type, run_at: existing.run_at || '', time_of_day: existing.time_of_day || '', interval_minutes: existing.interval_minutes || 0};
  const data = await api('/api/scheduled-tasks/' + encodeURIComponent(id), {method:'PUT', body: JSON.stringify(payload)});
  scheduledTaskItems = data.tasks || [];
  renderScheduledTasks();
}

async function deleteScheduledTask(id) {
  const existing = findScheduledTask(id);
  if (!existing) return;
  const ok = await showChoice('删除自动化任务', '确定删除定时任务「' + existing.title + '」？此操作不可恢复。', {confirmText: '删除', danger: true});
  if (!ok) return;
  const data = await api('/api/scheduled-tasks/' + encodeURIComponent(id), {method:'DELETE'});
  scheduledTaskItems = data.tasks || [];
  renderScheduledTasks();
  showToast('任务已删除', 'success');
}

async function runScheduledTaskNow(id) {
  const existing = findScheduledTask(id);
  if (!existing) return;
  const ok = await showChoice('立即运行任务', '立即运行定时任务「' + existing.title + '」？', {confirmText: '立即运行'});
  if (!ok) return;
  try {
    const result = await api('/api/scheduled-tasks/' + encodeURIComponent(id) + '/run', {method:'POST', body:'{}'});
    await loadScheduledTasks();
    await loadSessions();
    refreshProductState();
    if (result.session && result.session.id) {
      current = result.session.id;
      await renderSession(result.session);
      await loadSessions();
    }
    showToast('定时任务已运行', 'success');
  } catch (e) {
    await loadScheduledTasks().catch(() => {});
    showToast('运行失败：' + e.message, 'error');
  }
}

async function saveMCPConfig() {
  const content = mcp_config.value || '';
  try {
    JSON.parse(content || '{}');
  } catch (e) {
    showToast('MCP 配置不是合法 JSON：' + e.message, 'error');
    return;
  }
  const c = await api('/api/mcp-config', {method:'POST', body: JSON.stringify({content})});
  mcp_config.value = c.content || content;
  await loadMCPStatus().catch(() => {});
  showToast('MCP 配置已保存', 'success');
}

async function saveConfig() {
  const workspaceID = promptSelector.value || 'default';
  await api('/api/workspaces/' + encodeURIComponent(workspaceID) + '/config', {method:'POST', body: JSON.stringify({
    base_url: base_url.value,
    api_key: api_key.value,
    model: model.value,
    system_prompt: system_prompt.value,
    max_context_messages: Number(max_context_messages.value || 12),
    temperature: Number(temperature.value || 0.7),
    enable_thinking: enable_thinking.checked,
    hide_thinking: hide_thinking.checked,
  })});
  api_key.value = '';
  await loadConfig();
  await Promise.allSettled([loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadSystemStatus()]);
  showToast('已保存到工作空间：' + workspaceID, 'success');
}

async function loadSessions() {
  const list = await api('/api/sessions');
  const q = ((document.getElementById('sessionSearch') || {}).value || '').trim().toLowerCase();
  const filtered = q ? list.filter(s => String(s.title || '').toLowerCase().includes(q)) : list;
  sessions.innerHTML = filtered.map(s => '<div class="session ' + (current===s.id?'active':'') + '" onclick="openSession(\'' + s.id + '\')">' +
    '<div class="session-title">' + escapeHtml(s.title) + '</div>' +
    '<div class="session-meta">' + s.count + ' 条 · ' + fmtTime(s.updated_at) + '</div>' +
  '</div>').join('');
}

async function newSession() {
  const s = await api('/api/sessions', {method:'POST', body:'{}'});
  current = s.id;
  await loadSessions();
  await renderSession(s);
  closeSidebarOnMobile();
}

async function openSession(id) {
  current = id;
  const s = await api('/api/sessions/' + id);
  await loadSessions();
  await renderSession(s);
  closeSidebarOnMobile();
}

async function renameCurrent() {
  if (!current) return;
  const values = await showFormDialog({
    title: '重命名会话',
    confirmText: '保存标题',
    fields: [{name: 'title', label: '新的会话标题', value: title.textContent || '', required: true}],
  });
  if (!values || !values.title.trim()) return;
  const s = await api('/api/sessions/' + current + '/rename', {method:'POST', body: JSON.stringify({title: values.title.trim()})});
  await renderSession(s);
  await loadSessions();
  showToast('会话标题已保存', 'success');
}

function exportCurrent() {
  if (!current) return;
  window.open(authURL('/api/sessions/' + current + '/export?format=md'), '_blank');
}

async function deleteCurrent() {
  if (!current) return;
  const ok = await showChoice('删除当前会话', '确定删除当前会话？此操作不可恢复。', {confirmText: '删除', danger: true});
  if (!ok) return;
  await api('/api/sessions/' + current, {method:'DELETE'});
  current = null;
  title.textContent = '未选择会话';
  messages.innerHTML = '<div class="empty">创建一个会话，然后开始聊天。</div>';
  await loadSessions();
  showToast('会话已删除', 'success');
}

async function renderSession(s) {
  title.textContent = s.title || '新会话';
  if (!s.messages || s.messages.length === 0) {
    messages.innerHTML = '<div class="empty">这个会话还没有消息。</div>';
    return;
  }
  messages.innerHTML = s.messages.map(renderMessage).join('');
  messages.scrollTop = messages.scrollHeight;
}

function setStreamControls(active) {
  pauseStream.hidden = !active;
  stopStream.hidden = !active;
  pauseStream.disabled = !active;
  stopStream.disabled = !active;
  stopStream.textContent = '中断';
  send.disabled = active;
  continueBtn.disabled = active;
  if (!active) {
    activeAbortController = null;
    activeAssistantEl = null;
    activeReasoningEl = null;
    activeAnswerEl = null;
    streamPaused = false;
    pendingDelta = '';
    pendingReasoning = '';
    activeAnswerBuffer = '';
    activeReasoningBuffer = '';
    pauseStream.textContent = '暂停';
    pauseStream.title = '暂停显示输出';
    pauseStream.setAttribute('aria-label', pauseStream.title);
  }
}

function toggleStreamPause() {
  if (!busy) return;
  streamPaused = !streamPaused;
  pauseStream.textContent = streamPaused ? '继续' : '暂停';
  pauseStream.title = streamPaused ? '继续显示输出' : '暂停显示输出';
  pauseStream.setAttribute('aria-label', pauseStream.title);
  if (!streamPaused) {
    if (pendingReasoning && activeReasoningEl) {
      appendReasoning(pendingReasoning);
      pendingReasoning = '';
    }
    if (pendingDelta && activeAnswerEl) {
      appendAnswer(pendingDelta);
      pendingDelta = '';
    }
    messages.scrollTop = messages.scrollHeight;
  }
}

function stopStreaming() {
  if (!busy || !activeAbortController) return;
  stopStream.disabled = true;
  stopStream.textContent = '中断中';
  activeAbortController.abort();
}

function sendQuickMessage(text) {
  if (busy) return;
  input.value = text;
  sendMsg();
}

function renderMessage(m) {
  const role = m.role || '';
  const content = m.content || '';
  if (role === 'assistant') {
    return '<div class="msg assistant markdown">' + renderMarkdown(content) + '</div>';
  }
  return '<div class="msg ' + escapeHtml(role) + '">' + escapeHtml(content) + '</div>';
}

function appendReasoning(text) {
  if (!text || !activeReasoningEl) return;
  activeReasoningBuffer += text;
  activeReasoningEl.classList.add('show');
  const contentEl = activeReasoningEl.querySelector('.reasoning-content');
  if (contentEl) contentEl.innerHTML = renderMarkdown(activeReasoningBuffer);
}

function appendAnswer(text) {
  if (!text || !activeAnswerEl) return;
  activeAnswerBuffer += text;
  activeAnswerEl.innerHTML = renderMarkdown(activeAnswerBuffer);
}

async function sendMsg() {
  if (busy) return;
  if (!current) await newSession();
  const text = input.value.trim();
  if (!text) return;
  input.value = '';
  busy = true;
  activeAbortController = new AbortController();
  setStreamControls(true);

  const assistantId = 'assistant-' + Date.now();
  const reasoningId = assistantId + '-reasoning';
  const answerId = assistantId + '-answer';
  messages.innerHTML += '<div class="msg user">' + escapeHtml(text) + '</div>' +
    '<div class="msg assistant" id="' + assistantId + '">' +
      '<div class="reasoning" id="' + reasoningId + '"><div class="reasoning-title">思考</div><div class="reasoning-content markdown"></div></div>' +
      '<div class="answer markdown" id="' + answerId + '"></div>' +
    '</div>';
  const assistantEl = document.getElementById(assistantId);
  activeAssistantEl = assistantEl;
  activeReasoningEl = document.getElementById(reasoningId);
  activeAnswerEl = document.getElementById(answerId);
  activeReasoningBuffer = '';
  activeAnswerBuffer = '';
  messages.scrollTop = messages.scrollHeight;

  try {
    const res = await fetch('/api/chat/stream', {
      method: 'POST',
      headers: authHeaders({'Content-Type': 'application/json'}),
      body: JSON.stringify({session_id: current, message: text}),
      signal: activeAbortController.signal
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.error || res.statusText);
    }

    let finalSession = null;
    await readSSE(res, (event, data) => {
      if (event === 'delta') {
        const reasoning = data.reasoning_content || '';
        const content = data.content || '';
        if (streamPaused) {
          pendingReasoning += reasoning;
          pendingDelta += content;
        } else {
          if (reasoning) appendReasoning(reasoning);
          if (content) appendAnswer(content);
          messages.scrollTop = messages.scrollHeight;
        }
      } else if (event === 'tool_call_start') {
        if (activeAssistantEl) activeAssistantEl.insertAdjacentHTML('beforeend', renderToolEvent('start', data));
      } else if (event === 'tool_call_result') {
        if (activeAssistantEl) activeAssistantEl.insertAdjacentHTML('beforeend', renderToolEvent('result', data));
      } else if (event === 'done') {
        finalSession = data.session;
      } else if (event === 'error') {
        throw new Error(data.message || 'stream error');
      }
    });

    if (finalSession) {
      pendingDelta = '';
      pendingReasoning = '';
      await loadSessions();
    }
  } catch (e) {
    if (activeAbortController && activeAbortController.signal.aborted) {
      if (pendingReasoning) {
        appendReasoning(pendingReasoning);
        pendingReasoning = '';
      }
      if (pendingDelta) {
        appendAnswer(pendingDelta);
        pendingDelta = '';
      }
      appendAnswer((activeAnswerBuffer ? '\n\n' : '') + '【已中断】');
      messages.scrollTop = messages.scrollHeight;
      await loadSessions().catch(() => {});
    } else {
      assistantEl.textContent = '错误：' + e.message;
    }
  } finally {
    busy = false;
    setStreamControls(false);
  }
}

async function readSSE(res, onEvent) {
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const {value, done} = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, {stream: true});
    const parts = buffer.split('\n\n');
    buffer = parts.pop() || '';
    for (const part of parts) {
      const event = parseSSE(part);
      if (event) onEvent(event.event, event.data);
    }
  }
  if (buffer.trim()) {
    const event = parseSSE(buffer);
    if (event) onEvent(event.event, event.data);
  }
}

function parseSSE(block) {
  let event = 'message';
  const dataLines = [];
  for (const line of block.split('\n')) {
    if (line.startsWith('event:')) event = line.slice(6).trim();
    if (line.startsWith('data:')) dataLines.push(line.slice(5).trim());
  }
  if (!dataLines.length) return null;
  return {event, data: JSON.parse(dataLines.join('\n'))};
}

input.addEventListener('keydown', e => {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMsg(); }
});

async function loadPrompts() {
  const data = await api('/api/prompts');
  promptSelector.innerHTML = data.prompts.map(p => {
    const label = escapeHtml(p.name) + '（' + p.count + '）' + (p.active ? ' ✔' : '');
    return '<option value="' + escapeHtml(p.name) + '" ' + (p.active ? 'selected' : '') + '>' + label + '</option>';
  }).join('');
}

async function selectPrompt(name) {
  if (busy || !name) return;
  await selectWorkspace(name);
}

async function selectWorkspace(name) {
  if (busy || !name) return;
  await api('/api/workspaces/' + encodeURIComponent(name) + '/select', {method:'POST', body:'{}'});
  current = null;
  title.textContent = '未选择会话';
  messages.innerHTML = '<div class="empty">已切换工作空间。创建或选择一个会话。</div>';
  await Promise.allSettled([loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
  await loadPrompts();
  await loadConfig();
  await loadMCPConfig();
  await loadSkills();
  await loadScheduledTasks();
  await loadSessions();
  closeSidebarOnMobile();
}

async function createPromptSpace() {
  if (busy) return;
  const values = await showFormDialog({
    title: '新增工作空间',
    confirmText: '创建工作空间',
    fields: [
      {name: 'name', label: '工作空间名称', value: '', required: true},
      {name: 'system_prompt', label: '系统提示词内容', type: 'textarea', rows: 5, value: system_prompt.value || ''},
    ],
  });
  if (!values || !values.name.trim()) return;
  await api('/api/workspaces', {method:'POST', body: JSON.stringify({name: values.name.trim(), system_prompt: values.system_prompt || ''})});
  current = null;
  title.textContent = '未选择会话';
  messages.innerHTML = '<div class="empty">已创建并切换到新工作空间。</div>';
  await Promise.allSettled([loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
  await loadPrompts();
  await loadConfig();
  await loadMCPConfig();
  await loadSkills();
  await loadScheduledTasks();
  await loadSessions();
  closeSidebarOnMobile();
  showToast('工作空间已创建', 'success');
}


function setAuthToken() { showLoginForm(); }

function logout() {
  localStorage.removeItem('chatdock.authToken');
  showLoginForm();
}

startApp();
