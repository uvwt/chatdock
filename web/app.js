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

async function api(path, opt={}) {
  const res = await fetch(path, {headers: {'Content-Type':'application/json'}, ...opt});
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function fmtTime(value) { try { return new Date(value).toLocaleString(); } catch { return ''; } }

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
  if (!skillItems.length) {
    skills.innerHTML = '<div class="hint">暂无技能。技能会作为当前提示词空间的补充系统指令注入模型请求。</div>';
    return;
  }
  skills.innerHTML = skillItems.map(s => '<div class="skill-card">' +
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
  alert('MCP 配置已保存');
}

async function saveConfig() {
  await api('/api/config', {method:'POST', body: JSON.stringify({
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
  alert('已保存');
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
  window.open('/api/sessions/' + current + '/export?format=md', '_blank');
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
      headers: {'Content-Type': 'application/json'},
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
  await api('/api/prompts/select', {method:'POST', body: JSON.stringify({name})});
  current = null;
  title.textContent = '未选择会话';
  messages.innerHTML = '<div class="empty">已切换提示词空间。创建或选择一个会话。</div>';
  await loadPrompts();
  await loadConfig();
  await loadMCPConfig();
  await loadSkills();
  await loadSessions();
  closeSidebarOnMobile();
}

async function createPromptSpace() {
  if (busy) return;
  const name = prompt('新提示词空间名称：');
  if (!name || !name.trim()) return;
  const systemPrompt = prompt('系统提示词内容：', system_prompt.value || '');
  await api('/api/prompts', {method:'POST', body: JSON.stringify({name: name.trim(), system_prompt: systemPrompt || ''})});
  current = null;
  title.textContent = '未选择会话';
  messages.innerHTML = '<div class="empty">已创建并切换到新提示词空间。</div>';
  await loadPrompts();
  await loadConfig();
  await loadMCPConfig();
  await loadSkills();
  await loadSessions();
  closeSidebarOnMobile();
}

initTheme();
initSidebar();
loadPrompts();
loadConfig();
loadMCPConfig();
loadSkills();
loadSessions();
