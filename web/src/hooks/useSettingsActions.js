import { useCallback, useState } from 'react';
import { cronSchedulePayload, fmtTime } from '../lib/appUtils.js';
import {
  createModelProvider as createModelProviderRequest,
  createProject as createProjectRequest,
  deleteModelProvider as deleteModelProviderRequest,
  deleteProject as deleteProjectRequest,
  deleteScheduledTaskRecord,
  fetchProviderModels as fetchProviderModelsRequest,
  fetchProjectPromptPreview,
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
  providerPayloadForModelAppend,
  providerPayloadFromFormValues,
  uniqueModelNames,
} from '../lib/modelProviderForm.js';

export function useSettingsActions({
  api,
  busy,
  closeSidebarOnMobile,
  config,
  configDirty,
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
  const [loadingModels, setLoadingModels] = useState(false);

  const refreshAfterProviderMutation = useCallback(async () => {
    const refreshes = [loadModelProviders(), loadSetupStatus(), loadProjects()];
    // 供应商接口是即时保存；存在全局配置草稿时不能重新加载配置，否则会静默覆盖用户尚未保存的模型设置。
    if (!configDirty) refreshes.push(loadConfig());
    await Promise.allSettled(refreshes);
  }, [configDirty, loadConfig, loadModelProviders, loadProjects, loadSetupStatus]);

  const saveProject = useCallback(async (project = null, values = {}) => {
    if (busy) return false;
    const name = String(values.name || '').trim();
    if (!name) throw new Error('项目名称不能为空');
    const payload = {name, prompt: values.prompt || ''};
    try {
      const savedProject = project?.id ? await updateProjectRequest(api, project.id, payload) : await createProjectRequest(api, payload);
      await Promise.allSettled([loadProjects(), loadSetupStatus()]);
      if (savedProject?.id) setProjectFilter(savedProject.id);
      closeSidebarOnMobile();
      showToast(project?.id ? '项目已保存' : '项目已创建', 'success');
      return savedProject || true;
    } catch (error) {
      showToast((project?.id ? '保存项目失败：' : '创建项目失败：') + error.message, 'error');
      throw error;
    }
  }, [api, busy, closeSidebarOnMobile, loadProjects, loadSetupStatus, setProjectFilter, showToast]);

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

  const addCandidateModelsToProvider = useCallback(async (providerID, modelNames) => {
    const names = uniqueModelNames(modelNames || []);
    if (!names.length) return false;
    const provider = providers.find(item => item.id === providerID);
    if (!provider?.id) {
      showToast('找不到要更新的模型供应商', 'error');
      return false;
    }
    try {
      const payload = providerPayloadForModelAppend(provider, names[0]);
      payload.models = uniqueModelNames([...(payload.models || []), ...names]);
      await updateModelProviderRequest(api, provider.id, payload);
      await Promise.allSettled([loadModelProviders(), loadProjects()]);
      showToast('已保存 ' + names.length + ' 个模型', 'success');
      return true;
    } catch (error) {
      showToast('保存候选模型失败：' + (error.message || '未知错误'), 'error');
      return false;
    }
  }, [api, loadModelProviders, loadProjects, providers, showToast]);

  const saveModelProvider = useCallback(async (existing = null, values = {}) => {
    const payload = providerPayloadFromFormValues(values);
    if (!payload.name || !payload.base_url || !payload.default_model) throw new Error('名称、Base URL 和默认模型不能为空');
    try {
      if (existing?.id) await updateModelProviderRequest(api, existing.id, payload);
      else await createModelProviderRequest(api, payload);
      await refreshAfterProviderMutation();
      showToast(existing?.id ? '模型供应商已保存' : '模型供应商已新增', 'success');
      return true;
    } catch (error) {
      showToast((existing?.id ? '保存供应商失败：' : '新增供应商失败：') + (error.message || '未知错误'), 'error');
      throw error;
    }
  }, [api, refreshAfterProviderMutation, showToast]);

  const deleteModelProvider = useCallback(async (provider) => {
    if (!provider?.id) return;
    const ok = await showDialog({ title: '删除模型供应商', message: '确定删除模型供应商「' + (provider.name || provider.id) + '」？正在被全局配置使用的供应商不会被删除。', confirmText: '删除', danger: true, type: 'confirm' });
    if (!ok) return;
    await deleteModelProviderRequest(api, provider.id);
    await refreshAfterProviderMutation();
    showToast('模型供应商已删除', 'success');
  }, [api, refreshAfterProviderMutation, showDialog, showToast]);

  const testSavedModelProvider = useCallback(async (provider) => {
    if (!provider?.id) return;
    try {
      const data = await testModelProviderRequest(api, { provider_id: provider.id, model: provider.default_model });
      showToast(data.ok ? '供应商连接正常：' + (data.model || provider.default_model || '') : '供应商连接失败：' + (data.error || 'unknown'), data.ok ? 'success' : 'error');
    } catch (e) { showToast('供应商连接失败：' + e.message, 'error'); }
  }, [api, showToast]);

  const fetchSavedProviderModels = useCallback(async (provider) => {
    if (!provider?.id) return;
    setLoadingModels(true);
    try {
      const data = await fetchProviderModelsRequest(api, { provider_id: provider.id, model: provider.default_model });
      const models = data.candidate_models || data.models || [];
      showToast(models.length ? '已读取 ' + models.length + ' 个模型' : '接口可用，但没有返回模型名称', models.length ? 'success' : 'warn');
      return models;
    } catch (e) {
      showToast('获取候选模型失败：' + e.message, 'error');
      return null;
    } finally {
      setLoadingModels(false);
    }
  }, [api, showToast]);

  const saveMCPConfig = useCallback(async ({silent = false, content = mcpConfig} = {}) => {
    try {
      JSON.parse(content || '{}');
    } catch (e) {
      const error = new Error('MCP 配置不是合法 JSON：' + e.message);
      if (!silent) showToast(error.message, 'error');
      throw error;
    }
    await saveMCPConfigRequest(api, content);
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

  const saveScheduledTask = useCallback(async (existing = null, values = {}) => {
    if (busy) return false;
    const titleValue = String(values.title || '').trim();
    const promptValue = String(values.prompt || '').trim();
    const typeValue = String(values.schedule_type || '').trim().toLowerCase();
    if (!titleValue || !promptValue) throw new Error('任务标题和提示词不能为空');
    if (!['once', 'interval', 'cron'].includes(typeValue)) throw new Error('调度类型无效');
    const contextMode = ['stateless', 'last_result', 'session'].includes(values.context_mode) ? values.context_mode : 'stateless';
    const payload = {title: titleValue, prompt: promptValue, enabled: existing ? !!existing.enabled : true, schedule_type: typeValue, context_mode: contextMode, reschedule: !!values.reschedule};
    if (typeValue === 'once') payload.run_at = values.run_at || '';
    if (typeValue === 'interval') payload.interval_minutes = Math.floor(Number(values.interval_minutes || 0));
    if (typeValue === 'cron') Object.assign(payload, cronSchedulePayload(values));
    try {
      const data = await saveScheduledTaskRecord(api, existing, payload);
      setScheduledTasks(data.tasks || []);
      const savedTask = existing?.id ? (data.tasks || []).find(task => task.id === existing.id) : (data.tasks || [])[0];
      const nextRunText = savedTask?.next_run_at ? '，下次运行：' + fmtTime(savedTask.next_run_at) : '';
      showToast((existing?.id ? '任务已保存' : '任务已新增') + nextRunText, 'success');
      return savedTask || true;
    } catch (error) {
      showToast((existing?.id ? '保存任务失败：' : '新增任务失败：') + (error.message || '未知错误'), 'error');
      throw error;
    }
  }, [api, busy, setScheduledTasks, showToast]);

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

  const openScheduledTaskSession = openSession;

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
    addCandidateModelsToProvider,
    deleteModelProvider,
    deleteProject,
    deleteScheduledTask,
    saveModelProvider,
    saveProject,
    saveScheduledTask,
    fetchMCPServerTools,
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
  };
}
