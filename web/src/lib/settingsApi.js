export function fetchConfig(api) { return api('/api/config'); }
export function fetchMCPConfig(api) { return api('/api/mcp-config'); }
export function fetchSetupStatus(api) { return api('/api/setup/status'); }
export function fetchProjects(api) { return api('/api/projects'); }
export function fetchModelProviders(api) { return api('/api/model-providers'); }

export function normalizeProjectListResponse(data) {
  if (!data || !Array.isArray(data.projects) || !Number.isInteger(data.session_count) || !Number.isInteger(data.plain_session_count)) {
    throw new TypeError('invalid project list response');
  }
  if (data.projects.some(project => !project?.id || !Number.isInteger(project.session_count))) {
    throw new TypeError('invalid project summary response');
  }
  return {
    projects: data.projects,
    sessionCounts: {
      all: data.session_count,
      plain: data.plain_session_count,
      byProject: Object.fromEntries(data.projects.map(project => [project.id, project.session_count])),
    },
  };
}

export function createModelProvider(api, payload) {
  return api('/api/model-providers', {method:'POST', body: JSON.stringify(payload)});
}

export function updateModelProvider(api, id, payload) {
  return api('/api/model-providers/' + encodeURIComponent(id), {method:'PUT', body: JSON.stringify(payload)});
}

export function deleteModelProvider(api, id) {
  return api('/api/model-providers/' + encodeURIComponent(id), {method:'DELETE'});
}
export function fetchScheduledTasks(api) { return api('/api/scheduled-tasks'); }
export function fetchScheduledTaskRuns(api, id) { return api('/api/scheduled-tasks/' + encodeURIComponent(id) + '/runs?limit=30'); }
export function fetchDataStatus(api) { return api('/api/data/status'); }
export function fetchSystemStatus(api) { return api('/api/system/status'); }
export function fetchMCPStatus(api) { return api('/api/mcp/status'); }

export function createProject(api, input) {
  return api('/api/projects', {method:'POST', body: JSON.stringify(input)});
}

export function updateProject(api, id, input) {
  return api('/api/projects/' + encodeURIComponent(id), {method:'PUT', body: JSON.stringify(input)});
}

export function deleteProject(api, id) {
  return api('/api/projects/' + encodeURIComponent(id), {method:'DELETE'});
}

export function saveGlobalConfig(api, config) {
  return api('/api/config', {method:'POST', body: JSON.stringify(config)});
}

export function fetchProjectPromptPreview(api, projectID) {
  return api('/api/projects/' + encodeURIComponent(projectID) + '/prompt-preview');
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
