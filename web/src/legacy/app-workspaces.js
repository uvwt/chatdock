// ChatDock legacy workspaces：工作空间选择、新建和登录状态切换入口。
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
