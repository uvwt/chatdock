// ChatDock module runs：MCP 执行记录和 AgentDock 任务状态。
async function loadRuns() {
  const target = document.getElementById('runCards');
  if (!target) return;
  target.innerHTML = '<div class="hint">正在加载执行记录...</div>';
  try {
    const data = await api('/api/runs?limit=80');
    runItems = data.runs || [];
    renderRuns();
  } catch (e) {
    renderPanelError(target, e, 'runs-load');
  }
}

function renderRuns() {
  const target = document.getElementById('runCards');
  if (!target) return;
  if (!runItems.length) {
    target.innerHTML = '<div class="empty compact">还没有 MCP 执行记录。聊天触发工具调用后会出现在这里。</div>';
    return;
  }
  target.innerHTML = runItems.map(r => '<div class="task-card run-card">' +
    '<div class="task-head"><div><div class="task-name">' + escapeHtml(r.title || 'MCP 执行') + '</div>' +
    '<div class="task-desc">' + escapeHtml(r.summary || '暂无摘要') + '</div></div>' +
    '<div class="task-actions"><span class="badge ' + runStatusClass(r.status) + '">' + runStatusLabel(r.status) + '</span></div></div>' +
    '<div class="task-meta">工作空间：' + escapeHtml(r.workspace || '-') + ' · 事件 ' + (r.event_count || 0) + ' · 耗时 ' + fmtDuration(r.duration_ms) + ' · ' + fmtTime(r.updated_at) + '</div>' +
    (r.error ? '<div class="task-error">' + escapeHtml(r.error) + '</div>' : '') +
  '</div>').join('');
}

async function loadAgentTasks() {
  const target = document.getElementById('agentTaskCards');
  if (!target) return;
  target.innerHTML = '<div class="hint">正在加载 Agent 任务...</div>';
  try {
    const data = await api('/api/agent-tasks?limit=80');
    agentTaskItems = data.tasks || [];
    renderAgentTasks();
  } catch (e) {
    renderPanelError(target, e, 'agent-tasks-load');
  }
}

function renderAgentTasks() {
  const target = document.getElementById('agentTaskCards');
  if (!target) return;
  if (!agentTaskItems.length) {
    target.innerHTML = '<div class="empty compact">还没有 AgentDock 任务记录。task_manage 调用后会自动提取。</div>';
    return;
  }
  target.innerHTML = agentTaskItems.map(t => '<div class="task-card agent-task-card">' +
    '<div class="task-head"><div><div class="task-name">' + escapeHtml(t.title || 'AgentDock 任务') + '</div>' +
    '<div class="task-desc">' + escapeHtml(t.summary || '暂无摘要') + '</div></div>' +
    '<div class="task-actions"><span class="badge ' + runStatusClass(t.status) + '">' + agentStatusLabel(t.status) + '</span>' +
    '<button class="secondary small" data-action="agent-task-continue" data-id="' + dataAttr(t.id) + '">继续任务</button></div></div>' +
    '<div class="task-meta">' + escapeHtml(t.server || 'AgentDock') + ' · ' + escapeHtml(t.action || '-') + (t.phase ? ' · 阶段：' + escapeHtml(t.phase) : '') + ' · ' + fmtTime(t.updated_at) + '</div>' +
    (t.error ? '<div class="task-error">' + escapeHtml(t.error) + '</div>' : '') +
  '</div>').join('');
}

function continueAgentTask(id) {
  const task = agentTaskItems.find(t => t.id === id);
  if (!task) return showToast('任务不存在或尚未加载', 'error');
  const text = '继续任务：' + (task.title || 'AgentDock 任务') + '\n任务 ID：' + task.id + '\n来源 Run：' + (task.source_run_id || '');
  input.value = text;
  toggleSettingsPanel(false);
  input.focus();
  showToast('已填入继续任务指令，可直接发送。', 'success');
}

function runStatusLabel(status) {
  return ({running:'执行中', success:'成功', failed:'失败', completed:'已完成', blocked:'已阻塞', active:'进行中', matched:'已匹配'})[status] || (status || '未知');
}

function agentStatusLabel(status) { return runStatusLabel(status); }

function runStatusClass(status) {
  if (status === 'failed' || status === 'blocked') return 'error';
  if (status === 'running' || status === 'active') return 'warn';
  return 'ok';
}

function fmtDuration(ms) {
  const n = Number(ms || 0);
  if (n <= 0) return '-';
  if (n < 1000) return n + 'ms';
  return (n / 1000).toFixed(1) + 's';
}
