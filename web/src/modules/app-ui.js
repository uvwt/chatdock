// ChatDock module UI：弹层、Toast、事件委托和通用 HTML 属性处理。
let appDialogEl = null;
let toastTimer = null;

function closeAppDialog(value) {
  if (!appDialogEl) return;
  const resolve = appDialogEl.__resolve;
  document.removeEventListener('keydown', appDialogEl.__onKeydown);
  appDialogEl.remove();
  appDialogEl = null;
  if (resolve) resolve(value);
}

function showToast(message, variant='info') {
  let toast = document.getElementById('appToast');
  if (!toast) {
    toast = document.createElement('div');
    toast.id = 'appToast';
    toast.className = 'app-toast';
    document.body.appendChild(toast);
  }
  toast.className = 'app-toast show ' + (variant || 'info');
  toast.textContent = message || '';
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast.classList.remove('show'), 3200);
}

function showChoice(title, message, options={}) {
  return showAppDialog({
    title,
    message,
    variant: options.variant || 'info',
    confirmText: options.confirmText || '确定',
    cancelText: options.cancelText || '取消',
    danger: !!options.danger,
  });
}

function showFormDialog(options={}) {
  return showAppDialog({...options, type: 'form', confirmText: options.confirmText || '保存'});
}

function showAppDialog(config={}) {
  if (appDialogEl) closeAppDialog(null);
  return new Promise(resolve => {
    const backdrop = document.createElement('div');
    backdrop.className = 'app-modal-backdrop show';
    backdrop.__resolve = resolve;
    const fields = config.fields || [];
    const message = config.message ? '<div class="app-modal-message">' + escapeHtml(config.message) + '</div>' : '';
    const fieldHTML = fields.map(appDialogFieldHTML).join('');
    const cancelButton = config.hideCancel ? '' : '<button type="button" class="secondary app-modal-cancel">' + escapeHtml(config.cancelText || '取消') + '</button>';
    const confirmClass = config.danger ? 'danger' : '';
    backdrop.innerHTML = '<div class="app-modal-card ' + escapeHtml(config.variant || '') + '" role="dialog" aria-modal="true">' +
      '<form class="app-modal-form">' +
        '<div class="app-modal-title">' + escapeHtml(config.title || '确认') + '</div>' +
        message +
        '<div class="app-modal-fields">' + fieldHTML + '</div>' +
        '<div class="app-modal-actions">' + cancelButton + '<button type="submit" class="' + confirmClass + '">' + escapeHtml(config.confirmText || '确定') + '</button></div>' +
      '</form>' +
    '</div>';
    const form = backdrop.querySelector('form');
    const cancel = backdrop.querySelector('.app-modal-cancel');
    const finishCancel = () => closeAppDialog(null);
    const finishSubmit = event => {
      event.preventDefault();
      if (config.type !== 'form') {
        closeAppDialog(true);
        return;
      }
      const values = {};
      fields.forEach(field => {
        const el = form.querySelector('[data-field="' + field.name + '"]');
        values[field.name] = el ? el.value : '';
      });
      closeAppDialog(values);
    };
    backdrop.__onKeydown = event => {
      if (event.key === 'Escape') finishCancel();
    };
    form.addEventListener('submit', finishSubmit);
    if (cancel) cancel.addEventListener('click', finishCancel);
    backdrop.addEventListener('click', event => {
      if (event.target === backdrop) finishCancel();
    });
    document.addEventListener('keydown', backdrop.__onKeydown);
    document.body.appendChild(backdrop);
    appDialogEl = backdrop;
    const firstInput = form.querySelector('input, textarea, select, button');
    if (firstInput) firstInput.focus();
  });
}

function dataAttr(value) {
  return escapeHtml(String(value ?? ''));
}

function initDelegatedActions() {
  if (delegatedActionsReady) return;
  delegatedActionsReady = true;

  // 动态列表统一走事件委托，避免把用户输入的 id/name 拼进事件字符串。
  document.addEventListener('click', event => {
    const target = event.target.closest('[data-action]');
    if (!target || target.disabled) return;
    handleDelegatedClick(target).catch(err => showToast(err.message || String(err), 'error'));
  });
  document.addEventListener('change', event => {
    const target = event.target.closest('[data-action]');
    if (!target) return;
    handleDelegatedChange(target).catch(err => showToast(err.message || String(err), 'error'));
  });
  document.addEventListener('input', event => {
    const target = event.target.closest('[data-action]');
    if (!target) return;
    handleDelegatedInput(target).catch(err => showToast(err.message || String(err), 'error'));
  });
  document.addEventListener('submit', event => {
    const target = event.target.closest('[data-action]');
    if (!target) return;
    handleDelegatedSubmit(event, target).catch(err => showToast(err.message || String(err), 'error'));
  });
}

const clickActionHandlers = {
  // 页面框架动作
  'sidebar-close-mobile': () => closeSidebarOnMobile(),
  'settings-close': () => closeSettingsPanel(),
  'sidebar-toggle': () => toggleSidebar(),
  'settings-toggle': () => toggleSettingsPanel(),
  'theme-toggle': () => toggleTheme(),

  // 会话和消息动作
  'session-new': () => newSession(),
  'session-rename': () => renameCurrent(),
  'session-export': () => exportCurrent(),
  'session-delete': () => deleteCurrent(),
  'session-open': target => openSession(target.dataset.id || ''),
  'quick-message': target => sendQuickMessage(target.dataset.message || ''),
  'stream-pause': () => toggleStreamPause(),
  'stream-stop': () => stopStreaming(),
  'message-send': () => sendMsg(),

  // 配置中心通用动作
  'chat-return': () => returnToChat(),
  'product-refresh': () => refreshProductState(),
  'settings-module': target => switchSettingsModule(target.dataset.module),
  'config-save': () => saveConfig(),
  'prompt-preview': () => showPromptPreview(),
  'model-test': () => testModelProvider(),
  'model-providers-load': () => loadModelProviders(),
  'auth-switch': () => setAuthToken(),
  'setup-wizard': () => runSetupWizard(),

  // 工作空间动作
  'prompt-create': () => createPromptSpace(),
  'workspace-select': target => selectWorkspace(target.dataset.id || ''),
  'workspace-delete': target => deleteWorkspace(target.dataset.id || '', target.dataset.name || target.dataset.id || ''),
  'workspaces-load': () => loadWorkspaces(),

  // 技能动作
  'skill-create': () => editSkill(),
  'skill-edit': target => editSkill(target.dataset.id || ''),
  'skill-delete': target => deleteSkill(target.dataset.id || ''),
  'skills-load': () => loadSkills(),

  // MCP 动作
  'mcp-status': () => loadMCPStatus(),
  'mcp-save': () => saveMCPConfig(),
  'mcp-reload': () => loadMCPConfig(),
  'mcp-test': () => testMCP(),

  // MCP 执行与 Agent 任务动作
  'runs-load': () => loadRuns(),
  'agent-tasks-load': () => loadAgentTasks(),
  'agent-task-continue': target => continueAgentTask(target.dataset.id || ''),

  // 自动化任务动作
  'task-create': () => editScheduledTask(),
  'task-run': target => runScheduledTaskNow(target.dataset.id || ''),
  'task-edit': target => editScheduledTask(target.dataset.id || ''),
  'task-delete': target => deleteScheduledTask(target.dataset.id || ''),
  'tasks-load': () => loadScheduledTasks(),

  // 状态面板动作
  'data-status': () => loadDataStatus(),
  'system-status': () => loadSystemStatus(),
};

async function handleDelegatedClick(target) {
  const handler = clickActionHandlers[target.dataset.action];
  if (!handler) return;
  await handler(target);
}

async function handleDelegatedChange(target) {
  const id = target.dataset.id || '';
  if (target.dataset.action === 'prompt-select') {
    await selectPrompt(target.value);
    return;
  }
  if (target.dataset.action === 'skill-toggle') {
    await toggleSkill(id, target.checked);
    return;
  }
  if (target.dataset.action === 'task-toggle') {
    await toggleScheduledTask(id, target.checked);
  }
}

async function handleDelegatedInput(target) {
  if (target.dataset.action === 'session-search') {
    await loadSessions();
    return;
  }
  if (target.dataset.action === 'skill-search') {
    renderSkills();
    return;
  }
  if (target.dataset.action === 'task-search') {
    renderScheduledTasks();
  }
}

async function handleDelegatedSubmit(event, target) {
  if (target.dataset.action === 'login-submit') {
    await submitLogin(event);
  }
}

function appDialogFieldHTML(field) {
  const name = escapeHtml(field.name || 'field');
  const label = escapeHtml(field.label || field.name || '');
  const value = escapeHtml(field.value ?? '');
  const placeholder = field.placeholder ? ' placeholder="' + escapeHtml(field.placeholder) + '"' : '';
  const hint = field.hint ? '<div class="app-modal-field-hint">' + escapeHtml(field.hint) + '</div>' : '';
  const required = field.required ? ' required' : '';
  let control;
  if (field.type === 'textarea') {
    control = '<textarea data-field="' + name + '" rows="' + (field.rows || 5) + '"' + placeholder + required + '>' + value + '</textarea>';
  } else if (field.type === 'select') {
    const options = (field.options || []).map(opt => {
      const optValue = typeof opt === 'string' ? opt : opt.value;
      const optLabel = typeof opt === 'string' ? opt : opt.label;
      return '<option value="' + escapeHtml(optValue) + '" ' + (String(optValue) === String(field.value || '') ? 'selected' : '') + '>' + escapeHtml(optLabel) + '</option>';
    }).join('');
    control = '<select data-field="' + name + '"' + required + '>' + options + '</select>';
  } else {
    const type = escapeHtml(field.type || 'text');
    const min = field.min != null ? ' min="' + escapeHtml(field.min) + '"' : '';
    const max = field.max != null ? ' max="' + escapeHtml(field.max) + '"' : '';
    const step = field.step != null ? ' step="' + escapeHtml(field.step) + '"' : '';
    control = '<input data-field="' + name + '" type="' + type + '" value="' + value + '"' + placeholder + required + min + max + step + ' />';
  }
  return '<label class="app-modal-field"><span>' + label + '</span>' + control + hint + '</label>';
}
