// ChatDock legacy tasks：自动化任务列表、编辑、开关、删除和立即运行。
async function loadScheduledTasks() {
  try {
    const data = await api('/api/scheduled-tasks');
    scheduledTaskItems = data.tasks || [];
    renderScheduledTasks();
  } catch (e) {
    scheduledTaskItems = [];
    renderPanelError(scheduledTasks, e, 'tasks-load');
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
        '<button class="secondary small" data-action="task-run" data-id="' + dataAttr(t.id) + '" ' + (t.running ? 'disabled' : '') + '>立即运行</button>' +
        '<button class="secondary small" data-action="task-edit" data-id="' + dataAttr(t.id) + '">编辑</button>' +
        '<button class="danger small" data-action="task-delete" data-id="' + dataAttr(t.id) + '">删除</button>' +
      '</div>' +
    '</div>' +
    '<label class="task-toggle"><input type="checkbox" data-action="task-toggle" data-id="' + dataAttr(t.id) + '" ' + (t.enabled ? 'checked' : '') + ' /> 启用</label>' +
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
