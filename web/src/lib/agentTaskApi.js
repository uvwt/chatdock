export async function fetchAgentTasks(api, limit = 30) {
  return api('/api/agent-tasks?limit=' + encodeURIComponent(limit));
}

export async function fetchAgentTask(api, taskID) {
  return api('/api/agent-tasks/' + encodeURIComponent(taskID));
}

export async function blockAgentTask(api, taskID, summary) {
  return api('/api/agent-tasks/' + encodeURIComponent(taskID) + '/block', {
    method: 'POST',
    body: JSON.stringify({ summary }),
  });
}

export async function fetchCurrentSessionAgentTask(api, sessionID) {
  return api('/api/sessions/' + encodeURIComponent(sessionID) + '/agent-task');
}
