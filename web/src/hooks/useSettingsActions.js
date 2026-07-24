import { useCallback, useState } from 'react';
import { cronScheduleFormValue, cronSchedulePayload, defaultRunAtValue, fmtTime } from '../lib/appUtils.js';
import { scheduledTaskRunsText } from '../lib/chatPresentation.js';
import {
  createModelProvider as createModelProviderRequest,
  createProject as createProjectRequest,
  deleteModelProvider as deleteModelProviderRequest,
  deleteProject as deleteProjectRequest,
  deleteScheduledTaskRecord,
  fetchProviderModels as fetchProviderModelsRequest,
  fetchProjectPromptPreview,
  fetchScheduledTaskRuns,
  initializeSetup,
  runScheduledTask,
  saveGlobalConfig,
  saveMCPConfigRequest,
  saveScheduledTaskRecord,
  testMCPServer,
  testModelProvider as testModelProviderRequest,
  updateModelProvider as updateModelProviderRequest,
  updateProject as updateProjectRequest,
} from '../lib/settingsApi.js';
import {
  providerKeyRows,
  providerPayloadForModelAppend,
  providerPayloadFromFormValues,
  uniqueModelNames,
} from '../lib/modelProviderForm.js';

export function useSettingsActions({
  api,
  busy,
  closeSidebarOnMobile,
  config,
  loadConfig,
  loadMCPConfig,
  loadMCPStatus,
  loadModelProviders,
  loadProjects,
  loadScheduledTasks,
  loadSessions,
  loadSetupStatus,
  loadSystemStatus,
  mcpConfig,
  openSession,
  providers,
  refreshProductState,
  scheduledTasks,
  selectedProject,
  setConfig,
  setProjectFilter,
  setProjectPromptPreview,
  setScheduledTasks,
  showDialog,
  showToast,
}) {
  const [availableModels, setAvailableModels] = useState([]);
  const [candidateProviderID, setCandidateProviderID] = useState('');
  const [loadingModels, setLoadingModels] = useState(false);

  const editProject = useCallback(async (project = null) => {
    if (busy) return;
    const values = await showDialog({
      title: project ? '编辑项目' : '新增项目', confirmText: project ? '保存项目' : '创建项目', fields: [
        { name: 'name', label: '项目名称', value: project?.name || '', required: true },
        { name: 'prompt', label: '项目提示词', type: 'textarea', rows: 5, value: project?.prompt || '' },
      ]
    });
    if (!values || !values.name.trim()) return;
    const payload = { name: values.name.trim(), prompt: values.prompt || '' };
    try {
      const savedProject = project?.id ? await updateProjectRequest(api, project.id, payload) : await createProjectRequest(api, payload);
      await Promise.allSettled([loadProjects(), loadSetupStatus()]);
      if (savedProject?.id) setProjectFilter(savedProject.id);
      closeSidebarOnMobile();
      showToast(project ? '项目已保存' : '项目已创建', 'success');
    } catch (error) {
      showToast((project ? '保存项目失败：' : '创建项目失败：') + error.message, 'error');
    }
  }, [api, busy, closeSidebarOnMobile, loadProjects, loadSetupStatus, setProjectFilter, showDialog, showToast]);

  const deleteProject = useCallback(async (project) => {
    if (!project?.id) return;
    const ok = await showDialog({ title: '删除项目', message: '确定删除项目「' + (project.name || project.id) + '」？会话会保留并转为普通会话。', confirmText: '删除项目', danger: true, type: 'confirm' });
    if (!ok) return;
    try {
      await deleteProjectRequest(api, project.id);
      setProjectFilter('plain');
      await Promise.allSettled([loadProjects(), loadSetupStatus()]);
      showToast('项目已删除，会话已转为普通会话', 'success');
    } catch (error) {
      showToast('删除项目失败：' + error.message, 'error');
    }
  }, [api, loadProjects, loadSetupStatus, setProjectFilter, showDialog, showToast]);

  const saveConfig = useCallback(async ({silent = false} = {}) => {
    await saveGlobalConfig(api, {
      provider_id: config.provider_id,
      model: config.model,
      fallback_provider_id: config.fallback_provider_id,
      fallback_model: config.fallback_model,
      system_prompt: config.system_prompt,
      context_mode: config.context_mode || 'auto',
      max_context_messages: Number(config.max_context_messages || 12),
      temperature: Number(config.temperature || 0.7),
      hide_thinking: !!config.hide_thinking,
      embedding_base_url: config.embedding_base_url,
      embedding_api_key: config.embedding_api_key,
      embedding_model: config.embedding_model || 'BAAI/bge-m3',
    });
    setConfig(c => ({ ...c, api_key: '', embedding_api_key: '' }));
    await loadConfig();
    await Promise.allSettled([loadSetupStatus(), loadProjects(), loadModelProviders(), loadSystemStatus()]);
    if (!silent) showToast('全局配置已保存', 'success');
  }, [api, config, loadConfig, loadModelProviders, loadSetupStatus, loadSystemStatus, loadProjects, showToast]);

  const showProjectPromptPreview = useCallback(async (projectID = null) => {
    const id = projectID == null ? (selectedProject?.id || '') : String(projectID || '').trim();
    if (!id) {
      setProjectPromptPreview(config.system_prompt || '(全局系统提示词为空)');
      return;
    }
    const data = await fetchProjectPromptPreview(api, id);
    setProjectPromptPreview(data.content || '(空)');
  }, [api, config.system_prompt, selectedProject?.id, setProjectPromptPreview]);

  const runSetupWizard = useCallback(async () => {
    const values = await showDialog({
      title: '首次配置', message: '配置默认模型后即可开始对话。', confirmText: '完成初始化', fields: [
        { name: 'base_url', label: '模型 Base URL', value: config.base_url || 'https://api.openai.com/v1', required: true },
        { name: 'model', label: '默认模型', value: config.model || 'gpt-4o-mini', required: true },
        { name: 'api_key', label: 'API Key（可留空）', type: 'password', value: '' },
        { name: 'system_prompt', label: '默认 System Prompt', type: 'textarea', rows: 4, value: config.system_prompt || '你是 ChatDock，本地优先 AI 工作台。默认用中文回答。' },
      ]
    });
    if (!values) return;
    await initializeSetup(api, values);
    await Promise.allSettled([refreshProductState(), loadConfig()]);
    showToast('初始化完成', 'success');
  }, [api, config, loadConfig, refreshProductState, showDialog, showToast]);

  const testModelProvider = useCallback(async () => {
    try {
      const data = await testModelProviderRequest(api, {
        provider_id: config.provider_id,
        model: config.model,
        system_prompt: config.system_prompt,
        context_mode: config.context_mode || 'auto',
        max_context_messages: Number(config.max_context_messages || 12),
        temperature: Number(config.temperature || 0.7),
        hide_thinking: !!config.hide_thinking,
      });
      showToast(data.ok ? '模型连接正常：' + (data.model || '') : '模型连接失败：' + (data.error || 'unknown'), data.ok ? 'success' : 'error');
    } catch (e) { showToast('模型连接失败：' + e.message, 'error'); }
  }, [api, config, showToast]);

  const fetchProviderModels = useCallback(async () => {
    setLoadingModels(true);
    try {
      const data = await fetchProviderModelsRequest(api, {
        provider_id: config.provider_id,
        model: config.model,
        system_prompt: config.system_prompt,
        context_mode: config.context_mode || 'auto',
        max_context_messages: Number(config.max_context_messages || 12),
        temperature: Number(config.temperature || 0.7),
        hide_thinking: !!config.hide_thinking,
      });
      const models = data.candidate_models || data.models || [];
      setCandidateProviderID(config.provider_id || data.provider_id || '');
      setAvailableModels(models);
      showToast(models.length ? '已获取 ' + models.length + ' 个候选模型，仅用于查看，需手动保存为可用模型' : '接口可用，但没有返回候选模型名称', models.length ? 'success' : 'warn');
    } catch (e) {
      setAvailableModels([]);
      showToast('获取候选模型失败：' + e.message, 'error');
    } finally {
      setLoadingModels(false);
    }
  }, [api, config, showToast]);

  const addCandidateModelToProvider = useCallback(async (modelName) => {
    const name = String(modelName || '').trim();
    if (!name) return;
    const providerID = candidateProviderID || config.provider_id;
    const provider = providers.find(p => p.id === providerID) || providers.find(p => p.id === config.provider_id);
    if (!provider?.id) {
      showToast('请先选择供应商，再添加候选模型', 'error');
      return;
    }
    const payload = providerPayloadForModelAppend(provider, name);
    await updateModelProviderRequest(api, provider.id, payload);
    setConfig(c => ({
      ...c,
      provider_id: provider.id,
      base_url: provider.base_url || c.base_url || '',
      has_api_key: !!provider.has_api_key,
      model: name,
      models: payload.models,
    }));
    await Promise.allSettled([loadModelProviders(), loadProjects()]);
    showToast((provider.models || []).includes(name) ? '候选模型已在可用列表：' + name : '已加入可用模型列表：' + name, 'success');
  }, [api, candidateProviderID, config.provider_id, loadModelProviders, loadProjects, providers, showToast]);

  const editModelProvider = useCallback(async (existing = null) => {
    const modelText = uniqueModelNames([...(existing?.models || []), existing?.default_model].filter(Boolean)).join('\n');
    const keyRows = providerKeyRows(existing);
    const selectedKeyID = existing?.selected_key_id || keyRows[0]?.id || 'main';
    const values = await showDialog({
      variant: 'provider-modal provider-modal-simple',
      title: existing ? '编辑模型供应商' : '新增模型供应商',
      message: '统一按 OpenAI 兼容接口配置：名称、Base URL、模型和 Key。Key ID、优先级自动处理。',
      confirmText: existing ? '保存供应商' : '新增供应商',
      fields: [
        { name: 'name', label: '名称', value: existing?.name || '', required: true, placeholder: '例如：火山 / OpenAI / Claude Proxy' },
        { name: 'base_url', label: 'Base URL', value: existing?.base_url || '', required: true, placeholder: 'https://example.com/v1' },
        { name: 'default_model', label: '默认模型', value: existing?.default_model || '', required: true, placeholder: '例如：gpt-4o-mini / deepseek-v4-pro' },
        { name: 'selected_key_id', label: '当前 Key', type: 'hidden', value: selectedKeyID },
        { name: 'key_strategy', label: 'Key 策略', type: 'hidden', value: existing?.key_strategy || 'auto' },
        { name: 'api_keys', label: 'Key 列表', type: 'provider_keys', value: keyRows, hint: '只需要填 Key 名称和 Key。ID 与优先级自动生成；当前 Key 用单选按钮切换；已保存 Key 只隐藏中间字段。' },
        { name: 'models', label: '可用模型（每行一个）', type: 'textarea', rows: 4, value: modelText || (existing?.default_model || ''), hint: '这里是真正会出现在聊天模型选择器里的模型。候选模型需要逐个加入。' },
        { name: 'enabled', label: '状态', type: 'select', value: existing && existing.enabled === false ? 'false' : 'true', options: [{ value: 'true', label: '启用' }, { value: 'false', label: '停用' }] },
      ]
    });
    if (!values) return;
    const payload = providerPayloadFromFormValues(values);
    if (existing) await updateModelProviderRequest(api, existing.id, payload);
    else await createModelProviderRequest(api, payload);
    await Promise.allSettled([loadModelProviders(), loadConfig(), loadSetupStatus(), loadProjects()]);
    showToast(existing ? '模型供应商已保存' : '模型供应商已新增', 'success');
  }, [api, loadConfig, loadModelProviders, loadSetupStatus, loadProjects, showDialog, showToast]);

  const deleteModelProvider = useCallback(async (provider) => {
    if (!provider?.id) return;
    const ok = await showDialog({ title: '删除模型供应商', message: '确定删除模型供应商「' + (provider.name || provider.id) + '」？正在被全局配置使用的供应商不会被删除。', confirmText: '删除', danger: true, type: 'confirm' });
    if (!ok) return;
    await deleteModelProviderRequest(api, provider.id);
    await Promise.allSettled([loadModelProviders(), loadConfig(), loadSetupStatus(), loadProjects()]);
    showToast('模型供应商已删除', 'success');
  }, [api, loadConfig, loadModelProviders, loadSetupStatus, loadProjects, showDialog, showToast]);

  const testSavedModelProvider = useCallback(async (provider) => {
    if (!provider?.id) return;
    try {
      const data = await testModelProviderRequest(api, { provider_id: provider.id, model: provider.default_model });
      showToast(data.ok ? '供应商连接正常：' + (data.model || provider.default_model || '') : '供应商连接失败：' + (data.error || 'unknown'), data.ok ? 'success' : 'error');
    } catch (e) { showToast('供应商连接失败：' + e.message, 'error'); }
  }, [api, showToast]);

  const fetchSavedProviderModels = useCallback(async (provider) => {
    if (!provider?.id) return;
    try {
      const data = await fetchProviderModelsRequest(api, { provider_id: provider.id, model: provider.default_model });
      const models = data.candidate_models || data.models || [];
      setCandidateProviderID(provider.id || data.provider_id || '');
      setAvailableModels(models);
      showToast(models.length ? '已获取 ' + models.length + ' 个候选模型，点击单个模型可加入可用模型列表' : '接口可用，但没有返回候选模型名称', models.length ? 'success' : 'warn');
    } catch (e) { showToast('获取候选模型失败：' + e.message, 'error'); }
  }, [api, showToast]);

  const saveMCPConfig = useCallback(async ({silent = false} = {}) => {
    try {
      JSON.parse(mcpConfig || '{}');
    } catch (e) {
      const error = new Error('MCP 配置不是合法 JSON：' + e.message);
      if (!silent) showToast(error.message, 'error');
      throw error;
    }
    await saveMCPConfigRequest(api, mcpConfig);
    // 保存后重新读取服务端规范化结果，同时更新未保存基线。
    await loadMCPConfig();
    await loadMCPStatus().catch(() => { });
    if (!silent) showToast('MCP 配置已保存', 'success');
  }, [api, loadMCPConfig, loadMCPStatus, mcpConfig, showToast]);

  const testMCP = useCallback(async (serverName = '') => {
    try {
      const data = await testMCPServer(api, serverName);
      const name = data.server || serverName || '默认 MCP';
      showToast(data.ok ? 'MCP 连接正常：' + name + '，工具数 ' + data.tool_count : 'MCP 连接失败：' + name + '，' + (data.error || 'unknown error'), data.ok ? 'success' : 'error');
    } catch (e) { showToast('MCP 测试失败：' + e.message, 'error'); }
  }, [api, showToast]);


  const fetchMCPServerTools = useCallback(async (serverName = '') => {
    const data = await testMCPServer(api, serverName);
    if (!data.ok) throw new Error(data.error || 'MCP 连接失败');
    return data.tools || [];
  }, [api]);

  const editScheduledTask = useCallback(async (id) => {
    if (busy) return;
    const existing = id ? scheduledTasks.find(t => t.id === id) : null;
    const values = await showDialog({
      title: existing ? '编辑自动化任务' : '新增自动化任务', message: existing ? '普通保存会保留下一次运行时间；需要从现在重新计算时勾选“保存后重新计时”。' : '选择调度类型后，只需要填写对应的时间字段。', confirmText: existing ? '保存任务' : '新增任务', fields: [
        { name: 'title', label: '任务标题', value: existing ? existing.title : '', required: true },
        { name: 'prompt', label: '任务提示词', type: 'textarea', rows: 6, value: existing ? (existing.prompt || '') : '', required: true },
        { name: 'schedule_type', label: '调度类型', type: 'select', value: existing ? existing.schedule_type : 'once', options: [{ value: 'once', label: '一次性' }, { value: 'interval', label: '按分钟间隔' }, { value: 'cron', label: '重复计划' }] },
        { name: 'run_at', label: '一次性运行时间', type: 'datetime-local', value: existing && existing.run_at ? existing.run_at.slice(0, 16) : defaultRunAtValue(), showWhen: { schedule_type: 'once' } },
        { name: 'interval_minutes', label: '间隔分钟数', type: 'number', min: 1, step: 1, value: existing && existing.interval_minutes ? String(existing.interval_minutes) : '60', showWhen: { schedule_type: 'interval' }, hint: '当前本地调度器最低按分钟执行；过短间隔会更频繁占用模型额度。' },
        { name: 'cron_schedule', label: '重复计划', type: 'schedule_builder', value: cronScheduleFormValue(existing || {}, Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai'), showWhen: { schedule_type: 'cron' }, hint: '可添加多个执行时间，保存时会自动生成底层调度规则。' },
        { name: 'context_mode', label: '上下文模式', type: 'select', value: existing ? (existing.context_mode || 'stateless') : 'stateless', options: [{ value: 'stateless', label: '每次独立执行，最省 token' }, { value: 'last_result', label: '带上次运行结果' }, { value: 'session', label: '连续会话，保留完整上下文' }], hint: '默认独立执行：只使用本次任务提示词；需要长期上下文时再选择连续会话。' },
        ...(existing ? [{ name: 'reschedule', label: '保存后重新计时', type: 'checkbox', value: false, hint: '关闭时仅保存内容；开启后会按当前时间重新计算间隔或重复计划的下一次运行。' }] : []),
      ]
    });
    if (!values) return;
    const titleValue = (values.title || '').trim();
    const promptValue = (values.prompt || '').trim();
    const typeValue = (values.schedule_type || '').trim().toLowerCase();
    if (!titleValue || !promptValue) { showToast('任务标题和提示词不能为空', 'error'); return; }
    if (!['once', 'interval', 'cron'].includes(typeValue)) { showToast('调度类型只能是 once、interval 或 cron', 'error'); return; }
    const contextMode = ['stateless', 'last_result', 'session'].includes(values.context_mode) ? values.context_mode : 'stateless';
    const payload = { title: titleValue, prompt: promptValue, enabled: existing ? !!existing.enabled : true, schedule_type: typeValue, context_mode: contextMode, reschedule: !!values.reschedule };
    if (typeValue === 'once') payload.run_at = values.run_at || '';
    if (typeValue === 'interval') payload.interval_minutes = Math.floor(Number(values.interval_minutes || 0));
    if (typeValue === 'cron') Object.assign(payload, cronSchedulePayload(values));
    const data = await saveScheduledTaskRecord(api, existing, payload);
    setScheduledTasks(data.tasks || []);
    const savedTask = (data.tasks || []).find(task => task.id === (existing?.id || '')) || (data.tasks || [])[0];
    const nextRunText = savedTask?.next_run_at ? '，下次运行：' + fmtTime(savedTask.next_run_at) : '';
    showToast((existing ? '任务已保存' : '任务已新增') + nextRunText, 'success');
  }, [api, busy, scheduledTasks, showDialog, showToast]);

  const toggleScheduledTask = useCallback(async (id, enabled) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    try {
      const payload = { title: existing.title, prompt: existing.prompt, enabled: !!enabled, schedule_type: existing.schedule_type, run_at: existing.run_at || '', cron_expressions: existing.cron_expressions || [], timezone: existing.timezone || '', interval_minutes: existing.interval_minutes || 0, context_mode: existing.context_mode || 'stateless' };
      const data = await saveScheduledTaskRecord(api, existing, payload);
      setScheduledTasks(data.tasks || []);
      showToast(enabled ? '自动化任务已启用' : '自动化任务已停用', 'success');
    } catch (error) {
      showToast('修改自动化任务状态失败：' + (error.message || '未知错误'), 'error');
    }
  }, [api, scheduledTasks, setScheduledTasks, showToast]);

  const deleteScheduledTask = useCallback(async (id) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    const ok = await showDialog({ title: '删除自动化任务', message: '确定删除定时任务「' + existing.title + '」？此操作不可恢复。', confirmText: '删除', danger: true, type: 'confirm' });
    if (!ok) return;
    const data = await deleteScheduledTaskRecord(api, id);
    setScheduledTasks(data.tasks || []);
    showToast('任务已删除', 'success');
  }, [api, scheduledTasks, showDialog, showToast]);

  const openScheduledTaskSession = useCallback(async (sessionID) => {
    const id = String(sessionID || '').trim();
    if (!id) return;
    try {
      await openSession(id);
    } catch (e) { showToast('打开运行会话失败：' + e.message, 'error'); }
  }, [openSession, showToast]);

  const viewScheduledTaskRuns = useCallback(async (id) => {
    const existing = scheduledTasks.find(task => task.id === id);
    if (!existing) return;
    try {
      const data = await fetchScheduledTaskRuns(api, id);
      const runs = data.runs || [];
      await showDialog({
        title: '运行记录 · ' + (existing.title || '定时任务'),
        confirmText: '关闭',
        hideCancel: true,
        fields: [{
          name: 'runs',
          label: runs.length ? runs.length + ' 条运行记录' : '暂无运行记录',
          type: 'textarea',
          rows: 16,
          value: scheduledTaskRunsText(existing, runs),
        }],
      });
    } catch (error) {
      showToast('读取运行记录失败：' + error.message, 'error');
    }
  }, [api, scheduledTasks, showDialog, showToast]);


  const runScheduledTaskNow = useCallback(async (id) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    const ok = await showDialog({ title: '立即运行任务', message: '立即运行定时任务「' + existing.title + '」？', confirmText: '立即运行', type: 'confirm' });
    if (!ok) return;
    try {
      const result = await runScheduledTask(api, id);
      await loadScheduledTasks();
      await loadSessions();
      await refreshProductState();
      if (result.session && result.session.id) {
        await openScheduledTaskSession(result.session.id);
      }
      showToast(result.session ? '定时任务已运行，已打开运行会话' : '定时任务已运行，结果已写入运行记录', 'success');
    } catch (e) { await loadScheduledTasks().catch(() => { }); showToast('运行失败：' + e.message, 'error'); }
  }, [api, loadScheduledTasks, loadSessions, openScheduledTaskSession, refreshProductState, scheduledTasks, showDialog, showToast]);

  return {
    addCandidateModelToProvider,
    availableModels,
    candidateProviderID,
    deleteModelProvider,
    deleteProject,
    deleteScheduledTask,
    editModelProvider,
    editProject,
    editScheduledTask,
    fetchMCPServerTools,
    fetchProviderModels,
    fetchSavedProviderModels,
    loadingModels,
    openScheduledTaskSession,
    runScheduledTaskNow,
    runSetupWizard,
    saveConfig,
    saveMCPConfig,
    showProjectPromptPreview,
    testMCP,
    testModelProvider,
    testSavedModelProvider,
    toggleScheduledTask,
    viewScheduledTaskRuns,
  };
}
