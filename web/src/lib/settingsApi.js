export function fetchPrompts(api) { return api('/api/prompts'); }
export function fetchConfig(api) { return api('/api/config'); }
export function fetchMCPConfig(api) { return api('/api/mcp-config'); }
export function fetchSetupStatus(api) { return api('/api/setup/status'); }
export function fetchWorkspaces(api) { return api('/api/workspaces'); }
export function fetchModelProviders(api) { return api('/api/model-providers'); }

export function createModelProvider(api, payload) {
  return api('/api/model-providers', {method:'POST', body: JSON.stringify(payload)});
}

export function updateModelProvider(api, id, payload) {
  return api('/api/model-providers/' + encodeURIComponent(id), {method:'PUT', body: JSON.stringify(payload)});
}

export function deleteModelProvider(api, id) {
  return api('/api/model-providers/' + encodeURIComponent(id), {method:'DELETE'});
}
export function fetchSkills(api) { return api('/api/skills'); }
export function fetchScheduledTasks(api) { return api('/api/scheduled-tasks'); }
export function fetchScheduledTaskRuns(api, id) { return api('/api/scheduled-tasks/' + encodeURIComponent(id) + '/runs?limit=30'); }
export function fetchDataStatus(api) { return api('/api/data/status'); }
export function fetchSystemStatus(api) { return api('/api/system/status'); }
export function fetchMCPStatus(api) { return api('/api/mcp/status'); }
export function fetchRuns(api) { return api('/api/runs?limit=80'); }
export function fetchAgentTasks(api) { return api('/api/agent-tasks?limit=80'); }

export function selectWorkspace(api, name) {
  return api('/api/workspaces/' + encodeURIComponent(name) + '/select', {method:'POST', body:'{}'});
}

export function createWorkspaceRecord(api, input) {
  return api('/api/workspaces', {method:'POST', body: JSON.stringify(input)});
}

export function deleteWorkspaceRecord(api, id) {
  return api('/api/workspaces/' + encodeURIComponent(id), {method:'DELETE'});
}

export function saveWorkspaceConfig(api, workspaceID, config) {
  return api('/api/workspaces/' + encodeURIComponent(workspaceID) + '/config', {method:'POST', body: JSON.stringify(config)});
}

export function fetchPromptPreview(api, workspaceID) {
  return api('/api/workspaces/' + encodeURIComponent(workspaceID) + '/prompt-preview');
}

export function initializeSetup(api, input) {
  return api('/api/setup/init', {method:'POST', body: JSON.stringify(input)});
}

export function testModelProvider(api, config) {
  return api('/api/model-providers/test', {method:'POST', body: JSON.stringify(config)});
}

export function fetchProviderModels(api, config) {
  return api('/api/model-providers/models', {method:'POST', body: JSON.stringify(config)});
}

export function saveMCPConfigRequest(api, content) {
  return api('/api/mcp-config', {method:'POST', body: JSON.stringify({content})});
}

export function testMCPServer(api, serverName = '') {
  const suffix = serverName ? '?server=' + encodeURIComponent(serverName) : '';
  return api('/api/mcp/test' + suffix);
}

export function saveSkillRecord(api, existing, payload) {
  const path = existing ? '/api/skills/' + encodeURIComponent(existing.id) : '/api/skills';
  return api(path, {method: existing ? 'PUT' : 'POST', body: JSON.stringify(payload)});
}

export function deleteSkillRecord(api, id) {
  return api('/api/skills/' + encodeURIComponent(id), {method:'DELETE'});
}

export function saveScheduledTaskRecord(api, existing, payload) {
  const path = existing ? '/api/scheduled-tasks/' + encodeURIComponent(existing.id) : '/api/scheduled-tasks';
  return api(path, {method: existing ? 'PUT' : 'POST', body: JSON.stringify(payload)});
}

export function deleteScheduledTaskRecord(api, id) {
  return api('/api/scheduled-tasks/' + encodeURIComponent(id), {method:'DELETE'});
}

export function runScheduledTask(api, id) {
  return api('/api/scheduled-tasks/' + encodeURIComponent(id) + '/run', {method:'POST', body:'{}'});
}
