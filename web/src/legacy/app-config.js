// ChatDock legacy config：工作空间模型配置和 MCP 配置保存。
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
