export async function fetchAgentTasks(api, limit = 30) {
  return api('/api/agent-tasks?limit=' + encodeURIComponent(limit));
}

export async function fetchAgentTask(api, taskID) {
  return api('/api/agent-tasks/' + encodeURIComponent(taskID));
}
