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
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
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
  const s = await api('/api/setup/status');
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
  const workspaceName = prompt('默认工作空间名称：', 'default');
  if (workspaceName === null) return;
  const baseURL = prompt('模型 Base URL：', base_url.value || 'https://api.openai.com/v1');
  if (baseURL === null) return;
  const modelName = prompt('默认模型：', model.value || 'gpt-4o-mini');
  if (modelName === null) return;
  const apiKeyValue = prompt('API Key（可留空）：', '');
  if (apiKeyValue === null) return;
  const systemPromptValue = prompt('默认 System Prompt：', system_prompt.value || '你是 ChatDock，本地优先 AI 工作台。默认用中文回答。');
  await api('/api/setup/init', {method:'POST', body: JSON.stringify({workspace_name: workspaceName || 'default', base_url: baseURL || '', model: modelName || '', api_key: apiKeyValue || '', system_prompt: systemPromptValue || ''})});
  await loadPrompts();
  await loadConfig();
  await refreshProductState();
  alert('初始化完成');
}

async function loadWorkspaces() {
  const data = await api('/api/workspaces');
  workspaceItems = data.workspaces || [];
  renderWorkspaces();
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
  const data = await api('/api/model-providers');
  providerItems = data.providers || [];
  renderModelProviders();
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
    alert(data.ok ? '模型连接正常：' + (data.model || '') : '模型连接失败：' + (data.error || 'unknown'));
  } catch (e) {
    alert('模型连接失败：' + e.message);
  }
}

async function showPromptPreview() {
  const workspaceID = promptSelector.value || 'default';
  const data = await api('/api/workspaces/' + encodeURIComponent(workspaceID) + '/prompt-preview');
  promptPreview.hidden = false;
  promptPreview.textContent = data.content || '(空)';
}

async function loadDataStatus() {
  const data = await api('/api/data/status');
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
  const data = await api('/api/system/status');
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
  const c = await api('/api/config');
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
  const c = await api('/api/mcp-config');
  const fallback = ['{', '  "servers": {}', '}'].join('\n') + '\n';
  mcp_config.value = c.content || fallback;
}

async function loadSkills() {
  const data = await api('/api/skills');
  skillItems = data.skills || [];
  renderSkills();
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
  const name = prompt(existing ? '技能名称：' : '新技能名称：', existing ? existing.name : '');
  if (!name || !name.trim()) return;
  const description = prompt('技能描述（可选）：', existing ? (existing.description || '') : '');
  const content = prompt('技能内容：', existing ? (existing.content || '') : '');
  if (!content || !content.trim()) return;
  const payload = {name: name.trim(), description: description || '', content: content.trim(), enabled: existing ? !!existing.enabled : true};
  const data = await api(existing ? '/api/skills/' + encodeURIComponent(existing.id) : '/api/skills', {method: existing ? 'PUT' : 'POST', body: JSON.stringify(payload)});
  skillItems = data.skills || [];
  renderSkills();
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
  if (!existing || !confirm('确定删除技能「' + existing.name + '」？')) return;
  const data = await api('/api/skills/' + encodeURIComponent(id), {method:'DELETE'});
  skillItems = data.skills || [];
  renderSkills();
}


async function loadScheduledTasks() {
  const data = await api('/api/scheduled-tasks');
  scheduledTaskItems = data.tasks || [];
  renderScheduledTasks();
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
  const titleValue = prompt(existing ? '任务标题：' : '新任务标题：', existing ? existing.title : '');
  if (!titleValue || !titleValue.trim()) return;
  const promptValue = prompt('任务提示词：', existing ? (existing.prompt || '') : '');
  if (!promptValue || !promptValue.trim()) return;
  const typeValue = (prompt('调度类型：once / daily / interval', existing ? existing.schedule_type : 'once') || '').trim().toLowerCase();
  if (!['once', 'daily', 'interval'].includes(typeValue)) {
    alert('调度类型只能是 once、daily 或 interval');
    return;
  }
  const payload = {title: titleValue.trim(), prompt: promptValue.trim(), enabled: existing ? !!existing.enabled : true, schedule_type: typeValue};
  if (typeValue === 'once') {
    const value = prompt('运行时间（本地时间，格式 2026-06-02T09:30）：', existing && existing.run_at ? existing.run_at.slice(0, 16) : defaultRunAtValue());
    if (!value || !value.trim()) return;
    payload.run_at = value.trim();
  } else if (typeValue === 'daily') {
    const value = prompt('每天运行时间（HH:MM）：', existing ? (existing.time_of_day || '09:00') : '09:00');
    if (!value || !value.trim()) return;
    payload.time_of_day = value.trim();
  } else {
    const value = prompt('间隔分钟数：', existing && existing.interval_minutes ? String(existing.interval_minutes) : '60');
    const minutes = Number(value || 0);
    if (!Number.isFinite(minutes) || minutes <= 0) {
      alert('间隔分钟数必须大于 0');
      return;
    }
    payload.interval_minutes = Math.floor(minutes);
  }
  const data = await api(existing ? '/api/scheduled-tasks/' + encodeURIComponent(existing.id) : '/api/scheduled-tasks', {method: existing ? 'PUT' : 'POST', body: JSON.stringify(payload)});
  scheduledTaskItems = data.tasks || [];
  renderScheduledTasks();
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
  if (!existing || !confirm('确定删除定时任务「' + existing.title + '」？')) return;
  const data = await api('/api/scheduled-tasks/' + encodeURIComponent(id), {method:'DELETE'});
  scheduledTaskItems = data.tasks || [];
  renderScheduledTasks();
}

async function runScheduledTaskNow(id) {
  const existing = findScheduledTask(id);
  if (!existing || !confirm('立即运行定时任务「' + existing.title + '」？')) return;
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
    alert('定时任务已运行');
  } catch (e) {
    await loadScheduledTasks().catch(() => {});
    alert('运行失败：' + e.message);
  }
}

async function saveMCPConfig() {
  const content = mcp_config.value || '';
  try {
    JSON.parse(content || '{}');
  } catch (e) {
    alert('MCP 配置不是合法 JSON：' + e.message);
    return;
  }
  const c = await api('/api/mcp-config', {method:'POST', body: JSON.stringify({content})});
  mcp_config.value = c.content || content;
  await loadMCPStatus().catch(() => {});
  alert('MCP 配置已保存');
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
  alert('已保存到工作空间：' + workspaceID);
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
  const next = prompt('新的会话标题：', title.textContent || '');
  if (!next || !next.trim()) return;
  const s = await api('/api/sessions/' + current + '/rename', {method:'POST', body: JSON.stringify({title: next.trim()})});
  await renderSession(s);
  await loadSessions();
}

function exportCurrent() {
  if (!current) return;
  window.open(authURL('/api/sessions/' + current + '/export?format=md'), '_blank');
}

async function deleteCurrent() {
  if (!current || !confirm('确定删除当前会话？')) return;
  await api('/api/sessions/' + current, {method:'DELETE'});
  current = null;
  title.textContent = '未选择会话';
  messages.innerHTML = '<div class="empty">创建一个会话，然后开始聊天。</div>';
  await loadSessions();
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
  const name = prompt('新工作空间名称：');
  if (!name || !name.trim()) return;
  const systemPrompt = prompt('系统提示词内容：', system_prompt.value || '');
  await api('/api/workspaces', {method:'POST', body: JSON.stringify({name: name.trim(), system_prompt: systemPrompt || ''})});
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
}


function setAuthToken() {
  const currentToken = localStorage.getItem('chatdock.authToken') || '';
  const next = prompt('ChatDock 访问 Token（留空表示清除）：', currentToken);
  if (next === null) return;
  if (next.trim()) localStorage.setItem('chatdock.authToken', next.trim());
  else localStorage.removeItem('chatdock.authToken');
  alert(next.trim() ? 'Token 已保存到浏览器本地' : 'Token 已清除');
}

initTheme();
initSidebar();
loadPrompts();
loadConfig();
loadMCPConfig();
loadSkills();
loadScheduledTasks();
loadSessions();
refreshProductState();
