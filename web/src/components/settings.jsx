// Configuration-center modules: workspace, model/provider, MCP tools, automation, and system diagnostics.
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import '../styles/settings-entry.css';
import { TextCard } from './base.jsx';
import { settingsModules, diagnosticsText, fmtBytes, fmtRelativeAge, fmtTime, runStatusClass, runStatusLabel, safePathName, scheduleSummary, taskStatusClass, taskStatusLabel } from '../lib/appUtils.js';

const settingsModuleMeta = {
  workspace: {label: '工作区', desc: '工作空间、会话数量和自动化任务的整体入口。'},
  model: {label: '模型', desc: '默认供应商和默认模型。'},
  providers: {label: '供应商', desc: '新增、编辑、测试模型供应商和候选模型。'},
  tools: {label: '工具', desc: '添加、检测和维护 MCP Server。'},
  automation: {label: '自动化', desc: '管理当前工作空间下的定时任务和运行状态。'},
  security: {label: '系统', desc: '查看运行状态、数据库、备份、诊断信息和访问入口。'},
};

export function SettingsPanel(props) {
  const {
    activeModule, busy, closeSettings, config, configDirty, mcpConfigDirty, createWorkspace, dataStatus, deleteScheduledTask, deleteWorkspace, editModelProvider, deleteModelProvider, testSavedModelProvider, fetchSavedProviderModels,
    editScheduledTask, loadDataStatus, loadMCPConfig, loadMCPStatus, loadScheduledTasks,
    loadSystemStatus, logout, builtinTools, mcpConfig, mcpStatus, onCopy, providers, workspacePromptPreview, refreshProductState, refreshVisibleSettings, runScheduledTaskNow, viewScheduledTaskRuns, openScheduledTaskSession, runSetupWizard,
    saveConfig, saveMCPConfig, scheduledTasks, selectWorkspace, setConfig, setMcpConfig, setTaskSearch, setupStatus,
    showWorkspacePromptPreview, switchSettingsModule, systemStatus, taskSearch, testMCP, fetchMCPServerTools, testModelProvider, fetchProviderModels, availableModels, candidateProviderID, addCandidateModelToProvider, loadingModels, toggleScheduledTask,
    workspaces,
  } = props;
  const filteredTasks = useMemo(() => {
    const q = taskSearch.trim().toLowerCase();
    return q ? scheduledTasks.filter(t => [t.title, t.prompt, t.schedule_type, t.last_status, t.last_error].some(v => String(v || '').toLowerCase().includes(q))) : scheduledTasks;
  }, [scheduledTasks, taskSearch]);
  const saveTimerRef = useRef(null);
  const [saveState, setSaveState] = useState({scope: '', status: 'idle', message: ''});
  const unsavedCount = Number(!!configDirty) + Number(!!mcpConfigDirty);
  const moduleIsDirty = useCallback((name) => {
    if (name === 'model' || name === 'providers') return !!configDirty;
    if (name === 'tools') return !!mcpConfigDirty;
    return false;
  }, [configDirty, mcpConfigDirty]);

  useEffect(() => () => window.clearTimeout(saveTimerRef.current), []);

  const saveScope = useCallback(async (scope) => {
    const save = scope === 'mcp' ? saveMCPConfig : saveConfig;
    if (!save) return;
    window.clearTimeout(saveTimerRef.current);
    setSaveState({scope, status: 'saving', message: ''});
    try {
      await save({silent: true});
      setSaveState({scope, status: 'saved', message: ''});
      saveTimerRef.current = window.setTimeout(() => setSaveState(current => current.scope === scope ? {scope: '', status: 'idle', message: ''} : current), 2800);
    } catch (error) {
      setSaveState({scope, status: 'error', message: error?.message || '保存失败，请稍后重试。'});
    }
  }, [saveConfig, saveMCPConfig]);

  const configSaveState = saveState.scope === 'config' ? saveState : {scope: 'config', status: 'idle', message: ''};
  const mcpSaveState = saveState.scope === 'mcp' ? saveState : {scope: 'mcp', status: 'idle', message: ''};
  const activeWorkspaceName = setupStatus?.active_workspace || workspaces.find(ws => ws.active)?.name || '默认工作空间';
  const refreshSettings = () => {
    if (unsavedCount && !window.confirm('刷新会丢弃尚未保存的配置修改，确定继续吗？')) return;
    (refreshVisibleSettings || refreshProductState)?.();
  };
  return <section className="settings">
    <header className="settings-header">
      <div className="settings-header-main">
        <button className="secondary small settings-back-button" onClick={() => closeSettings()} aria-label="返回聊天" title="返回聊天"><svg className="settings-header-icon" aria-hidden="true" viewBox="0 0 24 24"><path d="m15 18-6-6 6-6" /></svg></button>
        <div>
          <div className="settings-title-row"><h2>配置中心</h2>{unsavedCount ? <span className="settings-global-save-state dirty"><span aria-hidden="true" />{unsavedCount} 处未保存</span> : saveState.status === 'saved' ? <span className="settings-global-save-state saved">✓ 已保存</span> : null}</div>
          <p>统一管理工作区、模型、工具与自动化。</p>
        </div>
      </div>
      <div className="settings-header-actions"><button className="secondary small settings-refresh-button" onClick={refreshSettings} aria-label="刷新配置" title="刷新配置"><svg className="settings-header-icon settings-refresh-icon" aria-hidden="true" viewBox="0 0 24 24"><path d="M20 11a8 8 0 1 0-2.34 5.66" /><path d="M20 4v7h-7" /></svg><span className="settings-refresh-text">刷新</span></button></div>
    </header>
    <div className="settings-sidebar">
      <select className="settings-mobile-module-select" value={activeModule} onChange={e => switchSettingsModule(e.target.value)} aria-label="选择配置模块">
        {settingsModules.map(m => <option key={m} value={m}>{moduleLabel(m)}{moduleIsDirty(m) ? ' · 未保存' : ''}</option>)}
      </select>
      <nav className="module-tabs" aria-label="配置模块">{settingsModules.map(m => { const dirty = moduleIsDirty(m); return <button key={m} className={'module-tab ' + (activeModule === m ? 'active ' : '') + (dirty ? 'dirty' : '')} onClick={() => switchSettingsModule(m)}><span className="module-tab-label">{moduleLabel(m)}</span>{dirty ? <span className="module-tab-dirty" aria-label="有未保存修改">未保存</span> : null}</button>; })}</nav>
      <div className="settings-sidebar-footer"><span>当前工作空间</span><b title={activeWorkspaceName}>{activeWorkspaceName}</b></div>
    </div>
    <main className="settings-content">
      <ModuleView name="workspace" activeModule={activeModule}><WorkspaceModule setupStatus={setupStatus} workspaces={workspaces} createWorkspace={createWorkspace} selectWorkspace={selectWorkspace} deleteWorkspace={deleteWorkspace} runSetupWizard={runSetupWizard} /></ModuleView>
      <ModuleView name="model" activeModule={activeModule} dirty={configDirty} saveState={configSaveState} onSave={() => saveScope('config')} saveHint="保存后将用于新的对话和自动化任务。"><ModelModule config={config} configDirty={configDirty} saveState={configSaveState} setConfig={setConfig} saveConfig={() => saveScope('config')} showWorkspacePromptPreview={showWorkspacePromptPreview} workspacePromptPreview={workspacePromptPreview} testModelProvider={testModelProvider} providers={providers} /></ModuleView>
      <ModuleView name="providers" activeModule={activeModule} dirty={configDirty} saveState={configSaveState} onSave={() => saveScope('config')} saveHint="当前默认供应商需要保存后才会生效。"><ProvidersModule config={config} setConfig={setConfig} providers={providers} editModelProvider={editModelProvider} deleteModelProvider={deleteModelProvider} testSavedModelProvider={testSavedModelProvider} fetchSavedProviderModels={fetchSavedProviderModels} availableModels={availableModels} candidateProviderID={candidateProviderID} addCandidateModelToProvider={addCandidateModelToProvider} loadingModels={loadingModels} /></ModuleView>
      <ModuleView name="tools" activeModule={activeModule} dirty={mcpConfigDirty} saveState={mcpSaveState} onSave={() => saveScope('mcp')} saveHint="保存后工具加载方式和权限才会生效。"><ToolsModule builtinTools={builtinTools} mcpStatus={mcpStatus} mcpConfig={mcpConfig} mcpConfigDirty={mcpConfigDirty} saveState={mcpSaveState} setMcpConfig={setMcpConfig} saveMCPConfig={() => saveScope('mcp')} loadMCPConfig={loadMCPConfig} loadMCPStatus={loadMCPStatus} testMCP={testMCP} fetchMCPServerTools={fetchMCPServerTools} /></ModuleView>
      <ModuleView name="automation" activeModule={activeModule}><div className="settings-block-head"><label>自动化任务（当前工作空间）</label><button className="secondary small" onClick={() => editScheduledTask()}>新增任务</button></div><input className="session-search" placeholder="搜索任务" value={taskSearch} onChange={e => setTaskSearch(e.target.value)} /><div className="tasks-list">{filteredTasks.length ? filteredTasks.map(t => <TaskCard key={t.id} task={t} editScheduledTask={editScheduledTask} deleteScheduledTask={deleteScheduledTask} toggleScheduledTask={toggleScheduledTask} runScheduledTaskNow={runScheduledTaskNow} viewScheduledTaskRuns={viewScheduledTaskRuns} openScheduledTaskSession={openScheduledTaskSession} />) : <div className="hint">暂无定时任务。默认每次独立执行，运行结果写入任务记录；需要连续上下文时可在编辑中开启。</div>}</div><div className="settings-actions"><button className="secondary" onClick={() => loadScheduledTasks?.()}>刷新任务</button></div></ModuleView>
      <ModuleView name="security" activeModule={activeModule}><SecurityModule systemStatus={systemStatus} setupStatus={setupStatus} dataStatus={dataStatus} mcpStatus={mcpStatus} providers={providers} loadSystemStatus={loadSystemStatus} loadDataStatus={loadDataStatus} logout={logout} onCopy={onCopy} /></ModuleView>
    </main>
  </section>;
}

function moduleLabel(m) {
  return settingsModuleMeta[m]?.label || m;
}
function moduleDescription(m) {
  return settingsModuleMeta[m]?.desc || '';
}
function ModuleView({ name, activeModule, children, dirty = false, saveState, onSave, saveHint = '' }) {
  return <div className={'module-view ' + (activeModule === name ? 'active ' : '') + (dirty ? 'dirty' : '')} data-module-view={name}>
    <div className="module-view-title"><div><span>{moduleLabel(name)}</span><p>{moduleDescription(name)}</p></div></div>
    <SettingsSaveState dirty={dirty} state={saveState} onSave={onSave} hint={saveHint} />
    {children}
  </div>;
}

function SettingsSaveState({ dirty, state = {}, onSave, hint }) {
  const status = state.status === 'saving' ? 'saving' : state.status === 'error' ? 'error' : dirty ? 'dirty' : state.status === 'saved' ? 'saved' : 'idle';
  if (status === 'idle') return null;
  const content = {
    dirty: ['修改尚未保存', hint || '保存后配置才会生效。'],
    saving: ['正在保存', '完成前请保持当前页面。'],
    saved: ['保存成功', '配置已写入当前工作空间。'],
    error: ['保存失败', state.message || '请检查配置后重试。'],
  }[status];
  return <div className={'settings-save-state ' + status} role={status === 'error' ? 'alert' : 'status'}>
    <div className="settings-save-state-copy"><span className="settings-save-state-icon" aria-hidden="true">{status === 'saved' ? '✓' : status === 'error' ? '!' : ''}</span><div><b>{content[0]}</b><span>{content[1]}</span></div></div>
    {dirty ? <button type="button" onClick={onSave} disabled={status === 'saving'}>{status === 'saving' ? '保存中…' : '保存更改'}</button> : null}
  </div>;
}

function normalizeModelNames(value) {
  const raw = Array.isArray(value) ? value.join('\n') : String(value || '');
  const seen = new Set();
  return raw.split(/[\n,，]+/).map(item => item.trim()).filter(Boolean).filter(item => {
    if (seen.has(item)) return false;
    seen.add(item);
    return true;
  });
}

function modelListText(config) {
  const models = Array.isArray(config.models) && config.models.length ? config.models : [config.model].filter(Boolean);
  return normalizeModelNames(models).join('\n');
}

function WorkspaceModule({ setupStatus, workspaces, createWorkspace, selectWorkspace, deleteWorkspace, runSetupWizard }) {
  const activeWorkspace = workspaces.find(ws => ws.active) || workspaces[0] || {};
  const totals = workspaces.reduce((acc, ws) => ({
    sessions: acc.sessions + Number(ws.session_count || 0),
    tasks: acc.tasks + Number(ws.task_count || 0),
  }), {sessions: 0, tasks: 0});
  const summaryItems = [
    ['当前工作空间', setupStatus?.active_workspace || activeWorkspace.name || '-'],
    ['工作空间', String(workspaces.length || 0)],
    ['会话总数', String(totals.sessions)],
    ['自动化任务', String(totals.tasks)],
  ];
  return <>
    <div className="workspace-summary-grid">{summaryItems.map(([label, value]) => <div className="workspace-summary-card" key={label}><span>{label}</span><b>{value}</b></div>)}</div>
    <div className={'setup-banner show ' + (setupStatus && !setupStatus.needs_setup ? 'ok' : '')}>{setupStatus?.needs_setup ? <><div><b>首次配置未完成</b><div className="hint">请配置模型供应商和默认工作空间，完成后即可开始对话。</div></div><button className="small" onClick={runSetupWizard}>开始引导</button></> : <div><b>系统已就绪</b><div className="hint">当前工作空间：{setupStatus?.active_workspace || '-'} · 数据目录：{setupStatus?.data_dir || '-'}</div></div>}</div>
    <div className="settings-block-head"><label>工作空间概览</label><button className="secondary small" onClick={createWorkspace}>新增工作空间</button></div>
    <div id="workspaceCards">{workspaces.length ? workspaces.map(ws => <TextCard key={ws.id || ws.name} title={ws.name} hint={ws.description || ''} badge={ws.active ? '当前' : '可切换'} active={ws.active}><div className="product-meta">模型：{ws.model || '-'} · 会话 {ws.session_count || 0} · 任务 {ws.task_count || 0}</div><div className="product-actions">{!ws.active ? <button className="secondary small" onClick={() => selectWorkspace(ws.id || ws.name)}>切换到此工作空间</button> : null}{(ws.id || ws.name) !== 'default' && workspaces.length > 1 ? <button className="danger small" onClick={() => deleteWorkspace(ws.id || ws.name, ws.name || ws.id)}>{ws.active ? '删除当前工作空间' : '删除'}</button> : null}</div></TextCard>) : <div className="empty compact">还没有工作空间，请创建第一个工作空间。</div>}</div>
  </>;
}

function ModelModule({ config, configDirty, saveState, setConfig, saveConfig, showWorkspacePromptPreview, workspacePromptPreview, testModelProvider, providers }) {
  const update = (key, value) => setConfig(c => ({...c, [key]: value}));
  const providerModels = (provider) => normalizeModelNames([...(provider?.models || []), provider?.default_model].filter(Boolean));
  const activeProvider = providers.find(p => p.id === config.provider_id) || providers[0] || null;
  const fallbackProvider = providers.find(p => p.id === config.fallback_provider_id) || null;
  const chooseProvider = (id) => setConfig(c => {
    const provider = providers.find(p => p.id === id) || providers[0] || null;
    const models = providerModels(provider);
    const model = models.includes(c.model) ? c.model : (provider?.default_model || models[0] || c.model || '');
    return {...c, provider_id: provider?.id || '', base_url: provider?.base_url || '', has_api_key: !!provider?.has_api_key, model, models};
  });
  const chooseModel = (name) => setConfig(c => {
    const models = normalizeModelNames([...(c.models || []), name]);
    return {...c, model: name, models};
  });
  const chooseFallbackProvider = (id) => setConfig(c => {
    const provider = providers.find(p => p.id === id) || null;
    if (!provider) return {...c, fallback_provider_id: '', fallback_model: ''};
    const models = providerModels(provider);
    const model = models.includes(c.fallback_model) ? c.fallback_model : (provider.default_model || models[0] || '');
    return {...c, fallback_provider_id: provider.id, fallback_model: model};
  });
  const chooseFallbackModel = (name) => setConfig(c => ({...c, fallback_model: name}));
  const selectedProviderModels = providerModels(activeProvider);
  const fallbackModels = normalizeModelNames([...providerModels(fallbackProvider), config.fallback_model].filter(Boolean));
  const contextMode = config.context_mode || 'auto';
  const saving = saveState?.status === 'saving';
  return <>
    <section className="model-quick-panel model-page-single">
      <div className="model-quick-head"><div><b>默认模型</b><span>{activeProvider?.name || '未选择供应商'} · {config.model || activeProvider?.default_model || '未选择模型'}</span></div><div className="model-quick-actions"><button className={configDirty ? 'settings-inline-save-button dirty' : 'settings-inline-save-button'} onClick={() => saveConfig?.()} disabled={!configDirty || saving}>{saving ? '保存中…' : configDirty ? '保存更改' : '已保存'}</button><button className="secondary" onClick={() => testModelProvider?.()}>测试</button><button className="secondary" onClick={() => showWorkspacePromptPreview?.()}>Prompt</button></div></div>
      <div className="model-quick-grid">
        <label>供应商<select value={activeProvider?.id || ''} onChange={e => chooseProvider(e.target.value)}>{providers.length ? providers.map(p => <option key={p.id} value={p.id}>{p.name || p.id}</option>) : <option value="">未配置供应商</option>}</select></label>
        <label>模型<input value={config.model || ''} onChange={e => chooseModel(e.target.value)} placeholder={activeProvider?.default_model || 'gpt-4o-mini'} /></label>
        <label>备用供应商<select value={fallbackProvider?.id || ''} onChange={e => chooseFallbackProvider(e.target.value)}><option value="">不使用备用模型</option>{providers.map(p => <option key={p.id} value={p.id}>{p.name || p.id}</option>)}</select></label>
        <label>备用模型<select value={config.fallback_model || ''} onChange={e => chooseFallbackModel(e.target.value)} disabled={!fallbackProvider}><option value="">{fallbackProvider ? '使用供应商默认模型' : '先选择备用供应商'}</option>{fallbackModels.map(name => <option key={name} value={name}>{name}</option>)}</select></label>
      </div>
      {selectedProviderModels.length ? <div className="model-options compact-model-options">{selectedProviderModels.map(name => <button key={name} type="button" className={'model-option ' + (name === config.model ? 'active' : '')} onClick={() => chooseModel(name)}>{name}</button>)}</div> : <div className="hint">没有可用模型。去“供应商”页添加模型或候选模型。</div>}
      <div className="hint model-fallback-hint">主模型在尚未输出内容或执行工具前失败时自动切换；备用模型不会永久覆盖当前会话选择。</div>
    </section>
    <div className="model-inline-grid">
      <section className="settings-section model-inline-card reply-inline-card">
        <div className="settings-section-head"><div><b>回复</b></div></div>
        <div className="settings-form-grid compact"><label>上下文<select value={contextMode} onChange={e => update('context_mode', e.target.value)}><option value="auto">自动</option><option value="compact">精简</option><option value="expanded">更多历史</option><option value="custom">自定义</option></select></label><label>Temperature<input type="number" step="0.1" min="0" max="2" value={config.temperature} onChange={e => update('temperature', e.target.value)} /></label></div>
        {contextMode === 'custom' ? <label>最近消息数<input type="number" min="1" max="200" value={config.max_context_messages} onChange={e => update('max_context_messages', e.target.value)} /></label> : null}
        <details className="model-mini-details"><summary>System Prompt</summary><textarea className="system-prompt-editor compact" value={config.system_prompt} onChange={e => update('system_prompt', e.target.value)} /></details>
        <div className="thinking-options compact"><label className="check-row"><input type="checkbox" checked={!!config.hide_thinking} onChange={e => update('hide_thinking', e.target.checked)} /> 隐藏模型思考内容</label></div>
        {workspacePromptPreview ? <pre className="code-preview compact">{workspacePromptPreview}</pre> : null}
      </section>
      <section className="settings-section model-inline-card embedding-inline-card">
        <div className="settings-section-head"><div><b>向量</b></div></div>
        <label>Base URL<input value={config.embedding_base_url || ''} onChange={e => update('embedding_base_url', e.target.value)} placeholder="http://127.0.0.1:8000/v1" /></label>
        <label>模型<input value={config.embedding_model || 'BAAI/bge-m3'} onChange={e => update('embedding_model', e.target.value)} placeholder="BAAI/bge-m3" /></label>
        <details className="model-mini-details"><summary>API Key</summary><input type="password" value={config.embedding_api_key || ''} onChange={e => update('embedding_api_key', e.target.value)} placeholder={config.has_embedding_api_key ? '已保存，留空不修改' : '可留空'} /></details>
        <div className="hint">用于工具搜索；不配置则使用关键词搜索。</div>
      </section>
    </div>
  </>;
}

function ProvidersModule({ config, setConfig, providers, editModelProvider, deleteModelProvider, testSavedModelProvider, fetchSavedProviderModels, availableModels, candidateProviderID, addCandidateModelToProvider, loadingModels }) {
  const providerModels = (provider) => normalizeModelNames([...(provider?.models || []), provider?.default_model].filter(Boolean));
  const activeProvider = providers.find(p => p.id === config.provider_id) || providers[0] || null;
  const candidateProvider = providers.find(p => p.id === candidateProviderID) || activeProvider;
  const candidateProviderModels = providerModels(candidateProvider);
  const chooseProvider = (id) => setConfig(c => {
    const provider = providers.find(p => p.id === id) || providers[0] || null;
    const models = providerModels(provider);
    const model = models.includes(c.model) ? c.model : (provider?.default_model || models[0] || c.model || '');
    return {...c, provider_id: provider?.id || '', base_url: provider?.base_url || '', has_api_key: !!provider?.has_api_key, model, models};
  });
  return <>
    <section className="settings-section provider-section provider-primary-section">
      <div className="settings-section-head"><div><b>供应商</b></div><button className="secondary small" onClick={() => editModelProvider(null)}>新增供应商</button></div>
      <div className="settings-form-grid compact"><label>当前默认供应商<select value={activeProvider?.id || ''} onChange={e => chooseProvider(e.target.value)}>{providers.length ? providers.map(p => <option key={p.id} value={p.id}>{p.name || p.id}</option>) : <option value="">未配置供应商</option>}</select></label><div className="provider-actions-inline"><button className="secondary small" onClick={() => activeProvider && testSavedModelProvider(activeProvider)} disabled={!activeProvider}>测试当前</button><button className="secondary small" onClick={() => activeProvider && fetchSavedProviderModels(activeProvider)} disabled={!activeProvider || loadingModels}>{loadingModels ? '获取中…' : '候选模型'}</button></div></div>
    </section>
    {availableModels.length ? <section className="settings-section provider-section"><div className="settings-section-head"><div><b>候选模型</b></div><span className="hint">{candidateProvider?.name || '当前供应商'} · {availableModels.length} 个</span></div><div className="model-options candidate-model-options">{availableModels.map(name => { const alreadyAdded = candidateProviderModels.includes(name); return <button key={'candidate-' + name} type="button" className={'model-option candidate ' + (alreadyAdded ? 'added ' : '') + (name === config.model ? 'active' : '')} onClick={() => addCandidateModelToProvider?.(name)}>{alreadyAdded ? '已加入 · ' : '+ 加入 · '}{name}</button>; })}</div></section> : null}
    <div className="provider-grid provider-page-grid">{providers.length ? providers.map(p => <TextCard key={p.id} title={p.name || p.id} hint={p.base_url || '-'} badge={p.enabled ? (p.type || 'openai') : '停用'} active={p.id === config.provider_id}><div className="product-meta">默认：{p.default_model || '-'} · 模型 {p.models?.length || 0} · Key {p.api_keys?.length || (p.has_api_key ? 1 : 0)}</div><div className="product-actions"><button className="secondary small" onClick={() => editModelProvider(p)}>编辑</button><button className="secondary small" onClick={() => testSavedModelProvider(p)}>测试</button><button className="secondary small" onClick={() => fetchSavedProviderModels(p)}>候选</button><button className="danger small" onClick={() => deleteModelProvider(p)}>删除</button></div></TextCard>) : <div className="empty compact">还没有模型供应商。</div>}</div>
  </>;
}


function parseMCPConfigDraft(content) {
  try {
    const config = JSON.parse(String(content || '{}')) || {};
    if (!config.servers || typeof config.servers !== 'object' || Array.isArray(config.servers)) config.servers = {};
    return {config, error: ''};
  } catch (e) {
    return {config: {servers: {}}, error: e.message};
  }
}

function stringifyMCPConfigDraft(config) {
  return JSON.stringify({...config, servers: config.servers || {}}, null, 2) + '\n';
}

function joinMCPToolList(value) {
  return (Array.isArray(value) ? value : []).join('\n');
}

function splitMCPToolList(value) {
  return String(value || '').split(/[\n,]+/).map(s => s.trim()).filter(Boolean);
}

function normalizeMCPToolOverrides(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  return Object.fromEntries(Object.entries(value).filter(([name, exposure]) => name && ['direct', 'on_demand', 'inherit'].includes(exposure)));
}

function cleanMCPServerName(value) {
  return String(value || '').trim();
}

function normalizeMCPURLDraft(value) {
  return String(value || '').trim().replace(/\s+/g, '');
}

function dockerHostMCPURL(value) {
  const cleaned = normalizeMCPURLDraft(value);
  return cleaned.replace(/^http:\/\/(127\.0\.0\.1|localhost)(?=[:/]|$)/i, 'http://host.docker.internal');
}

function normalizeBearerTokenDraft(value) {
  return String(value || '').trim().replace(/^Bearer\s+/i, '');
}

function mcpTokenExpiryState(token) {
  const parts = String(token || '').split('.');
  if (parts.length < 2) return null;
  try {
    const payload = JSON.parse(atob(parts[1].replace(/-/g, '+').replace(/_/g, '/')));
    const exp = Number(payload.exp || 0);
    if (!exp) return null;
    const expiresAt = new Date(exp * 1000);
    if (Date.now() >= exp * 1000) return {expired: true, text: 'Token 已过期：' + expiresAt.toLocaleString()};
    return {expired: false, text: 'Token 有效期至：' + expiresAt.toLocaleString()};
  } catch {
    return null;
  }
}

function mcpServerToDraft(server = {}) {
  const auth = server.auth || {};
  return {
    type: server.type || 'streamable-http',
    url: server.url || '',
    path: server.path || '',
    disabled: !!server.disabled,
    auth_type: auth.type || (auth.token || auth.token_env ? 'bearer' : 'none'),
    token: auth.token || '',
    token_env: auth.token_env || '',
    allow_tools: joinMCPToolList(server.allow_tools),
    deny_tools: joinMCPToolList(server.deny_tools),
    confirm_tools: joinMCPToolList(server.confirm_tools),
    tool_exposure: ['direct', 'on_demand'].includes(server.tool_exposure) ? server.tool_exposure : 'on_demand',
    tool_overrides: normalizeMCPToolOverrides(server.tool_overrides),
    timeout_ms: server.timeout_ms ? String(server.timeout_ms) : '',
    cache_ttl_ms: server.cache_ttl_ms ? String(server.cache_ttl_ms) : '',
  };
}

function cleanMCPServerDraft(draft) {
  const next = {};
  const type = String(draft.type || '').trim();
  const url = normalizeMCPURLDraft(draft.url);
  const path = String(draft.path || '').trim();
  const authType = String(draft.auth_type || '').trim();
  const token = normalizeBearerTokenDraft(draft.token);
  const tokenEnv = String(draft.token_env || '').trim();
  if (type && type !== 'streamable-http') next.type = type;
  if (url) next.url = url;
  if (path) next.path = path;
  if (draft.disabled) next.disabled = true;
  if (authType && authType !== 'none') {
    next.auth = {type: authType};
    if (token) next.auth.token = token;
    if (tokenEnv) next.auth.token_env = tokenEnv;
  }
  const allow = splitMCPToolList(draft.allow_tools);
  const deny = splitMCPToolList(draft.deny_tools);
  const confirm = splitMCPToolList(draft.confirm_tools);
  if (allow.length) next.allow_tools = allow;
  if (deny.length) next.deny_tools = deny;
  if (confirm.length) next.confirm_tools = confirm;
  next.tool_exposure = draft.tool_exposure === 'direct' ? 'direct' : 'on_demand';
  const toolOverrides = Object.fromEntries(Object.entries(normalizeMCPToolOverrides(draft.tool_overrides)).filter(([, exposure]) => exposure !== 'inherit'));
  if (Object.keys(toolOverrides).length) next.tool_overrides = toolOverrides;
  const timeout = Number(draft.timeout_ms || 0);
  const cacheTTL = Number(draft.cache_ttl_ms || 0);
  if (Number.isFinite(timeout) && timeout > 0) next.timeout_ms = Math.round(timeout);
  if (Number.isFinite(cacheTTL) && cacheTTL > 0) next.cache_ttl_ms = Math.round(cacheTTL);
  return next;
}

function defaultMCPServerDraft() {
  return {name: '', type: 'streamable-http', url: '', path: '', disabled: false, auth_type: 'none', token: '', token_env: '', allow_tools: '', deny_tools: '', confirm_tools: '', tool_exposure: 'on_demand', tool_overrides: {}, timeout_ms: '30000', cache_ttl_ms: ''};
}

function builtinToolsToDraft(config = {}) {
  return {
    tool_exposure: ['direct', 'on_demand'].includes(config.tool_exposure) ? config.tool_exposure : 'direct',
    tool_overrides: normalizeMCPToolOverrides(config.tool_overrides),
  };
}

function cleanBuiltinToolsDraft(draft) {
  const next = {tool_exposure: draft.tool_exposure === 'on_demand' ? 'on_demand' : 'direct'};
  const overrides = Object.fromEntries(Object.entries(normalizeMCPToolOverrides(draft.tool_overrides)).filter(([, exposure]) => exposure !== 'inherit'));
  if (Object.keys(overrides).length) next.tool_overrides = overrides;
  return next;
}

function normalizeMCPToolOptions(name, tools, draft) {
  const selected = [...splitMCPToolList(draft.allow_tools), ...splitMCPToolList(draft.deny_tools), ...splitMCPToolList(draft.confirm_tools), ...Object.keys(normalizeMCPToolOverrides(draft.tool_overrides))].filter(item => item !== '*');
  const byName = new Map();
  (tools || []).forEach(tool => {
    const value = String(tool.full_name || tool.fullName || tool.name || '').trim();
    if (!value) return;
    byName.set(value, {value, name: tool.name || value, title: tool.title || tool.name || value, description: tool.description || '', server: tool.server || name});
  });
  selected.forEach(value => {
    if (!byName.has(value)) byName.set(value, {value, name: value, title: value, description: '已在配置中，但当前工具列表未返回', server: name, configuredOnly: true});
  });
  return [...byName.values()].sort((a, b) => a.value.localeCompare(b.value));
}

function toolPolicyForValue(value, draft) {
  const allow = splitMCPToolList(draft.allow_tools);
  const deny = splitMCPToolList(draft.deny_tools);
  const confirm = splitMCPToolList(draft.confirm_tools);
  if (deny.includes(value)) return 'deny';
  if (confirm.includes(value)) return 'confirm';
  if (allow.includes(value)) return 'allow';
  return 'default';
}

function setToolPolicyDraft(draft, value, policy) {
  const unique = (items) => Array.from(new Set(items.filter(Boolean)));
  const allowAll = splitMCPToolList(draft.allow_tools).includes('*');
  let allow = splitMCPToolList(draft.allow_tools).filter(item => item !== value && item !== '*');
  let deny = splitMCPToolList(draft.deny_tools).filter(item => item !== value);
  let confirm = splitMCPToolList(draft.confirm_tools).filter(item => item !== value);
  if (policy === 'allow') allow.push(value);
  if (policy === 'deny') deny.push(value);
  if (policy === 'confirm') confirm.push(value);
  if (allowAll) allow.unshift('*');
  return {allow_tools: joinMCPToolList(unique(allow)), deny_tools: joinMCPToolList(unique(deny)), confirm_tools: joinMCPToolList(unique(confirm))};
}

function setAllowAllDraft(draft, enabled) {
  const allow = splitMCPToolList(draft.allow_tools).filter(item => item !== '*');
  if (enabled) allow.unshift('*');
  return {allow_tools: joinMCPToolList(Array.from(new Set(allow)))};
}

function toolExposureForValue(value, draft) {
  return normalizeMCPToolOverrides(draft.tool_overrides)[value] || 'inherit';
}

function setToolExposureDraft(draft, value, exposure) {
  const overrides = {...normalizeMCPToolOverrides(draft.tool_overrides)};
  if (exposure === 'inherit') delete overrides[value];
  else overrides[value] = exposure;
  return {tool_overrides: overrides};
}

function MCPToolPolicyPicker({name, draft, tools, loading, onRefresh, onChange}) {
  const options = normalizeMCPToolOptions(name, tools, draft);
  const allowAll = splitMCPToolList(draft.allow_tools).includes('*');
  return <div className="mcp-tool-picker">
    <div className="mcp-tool-picker-head"><div><b>工具权限</b><span>{options.length ? options.length + ' 个工具' : '先读取工具列表'}</span></div><button type="button" className="secondary small" onClick={onRefresh} disabled={loading || !name}>{loading ? '读取中…' : '读取工具列表'}</button></div>
    <label className="mcp-tool-allow-all"><input type="checkbox" checked={allowAll} onChange={e => onChange(setAllowAllDraft(draft, e.target.checked))} /> 允许全部工具</label>
    {options.length ? <div className="mcp-tool-policy-list">{options.map(tool => {
      const policy = toolPolicyForValue(tool.value, draft);
      const exposure = toolExposureForValue(tool.value, draft);
      return <div className="mcp-tool-policy-row" key={tool.value}>
        <div className="mcp-tool-policy-name"><b>{tool.title || tool.name}</b><span>{tool.value}{tool.configuredOnly ? ' · 配置项' : ''}</span></div>
        <label className="mcp-tool-exposure"><span>加载方式</span><select aria-label={(tool.title || tool.name) + ' 加载方式'} value={exposure} onChange={e => onChange(setToolExposureDraft(draft, tool.value, e.target.value))}><option value="inherit">跟随默认</option><option value="direct">直接加载</option><option value="on_demand">按需加载</option></select></label>
        <div className="mcp-tool-policy-actions" role="group" aria-label={tool.value + ' 权限'}>
          {[
            ['default', allowAll ? '默认允许' : '默认'],
            ['allow', '允许'],
            ['confirm', '确认'],
            ['deny', '禁止'],
          ].map(([value, label]) => <button key={value} type="button" className={(policy === value ? 'active ' : '') + value} onClick={() => onChange(setToolPolicyDraft(draft, tool.value, value))}>{label}</button>)}
        </div>
      </div>;
    })}</div> : <div className="hint">点击“读取工具列表”后勾选；未读取时仍会保留已保存的工具规则。</div>}
  </div>;
}

function BuiltinToolExposurePicker({draft, tools, onChange}) {
  const options = normalizeMCPToolOptions('chatdock', tools, draft);
  return <details className="mcp-tool-picker builtin-tool-picker">
    <summary className="builtin-tool-picker-summary"><span><b>单工具覆盖</b><small>仅调整例外工具</small></span><em>{options.length} 个</em></summary>
    <div className="mcp-tool-policy-list">{options.map(tool => <div className="mcp-tool-policy-row builtin-tool-exposure-row" key={tool.value}>
      <div className="mcp-tool-policy-name"><b>{tool.title || tool.name}</b><span>{tool.value}</span></div>
      <select className="mcp-tool-exposure-select" aria-label={(tool.title || tool.name) + ' 加载方式'} value={toolExposureForValue(tool.value, draft)} onChange={e => onChange(setToolExposureDraft(draft, tool.value, e.target.value))}><option value="inherit">跟随默认</option><option value="direct">直接加载</option><option value="on_demand">按需加载</option></select>
    </div>)}</div>
  </details>;
}

function ToolsModule({ builtinTools, mcpStatus, mcpConfig, mcpConfigDirty, saveState, setMcpConfig, saveMCPConfig, loadMCPConfig, loadMCPStatus, testMCP, fetchMCPServerTools }) {
  const [newServer, setNewServer] = useState(defaultMCPServerDraft);
  const [renameDrafts, setRenameDrafts] = useState({});
  const [formError, setFormError] = useState('');
  const [serverTools, setServerTools] = useState({});
  const [loadingTools, setLoadingTools] = useState({});
  const parsed = useMemo(() => parseMCPConfigDraft(mcpConfig), [mcpConfig]);
  const builtinDraft = builtinToolsToDraft(parsed.config.builtin_tools);
  const serverNames = Object.keys(parsed.config.servers || {}).sort();
  const saving = saveState?.status === 'saving';
  const statusByName = useMemo(() => Object.fromEntries((mcpStatus || []).map(s => [s.name, s])), [mcpStatus]);

  async function refreshServerTools(name) {
    if (!name || !fetchMCPServerTools) return;
    setLoadingTools(prev => ({...prev, [name]: true}));
    try {
      const tools = await fetchMCPServerTools(name);
      setServerTools(prev => ({...prev, [name]: tools || []}));
      setFormError('已读取 ' + name + ' 的 ' + ((tools || []).length) + ' 个工具。');
    } catch (e) {
      setFormError('读取工具列表失败：' + e.message);
    } finally {
      setLoadingTools(prev => ({...prev, [name]: false}));
    }
  }

  function replaceConfig(mutator) {
    setMcpConfig(prev => {
      const parsedPrev = parseMCPConfigDraft(prev);
      if (parsedPrev.error) return prev;
      const next = {...parsedPrev.config, servers: {...(parsedPrev.config.servers || {})}};
      mutator(next);
      return stringifyMCPConfigDraft(next);
    });
  }

  function patchServer(name, patch) {
    replaceConfig(next => {
      const draft = {...mcpServerToDraft(next.servers[name]), ...patch};
      next.servers[name] = cleanMCPServerDraft(draft);
    });
  }

  function patchBuiltinTools(patch) {
    replaceConfig(next => {
      const draft = {...builtinToolsToDraft(next.builtin_tools), ...patch};
      next.builtin_tools = cleanBuiltinToolsDraft(draft);
    });
  }

  function removeServer(name) {
    replaceConfig(next => { delete next.servers[name]; });
  }

  function renameServer(oldName) {
    const nextName = cleanMCPServerName(renameDrafts[oldName] ?? oldName);
    if (!nextName) { setFormError('Server 名称不能为空。'); return; }
    if (nextName === oldName) { setRenameDrafts(drafts => { const next = {...drafts}; delete next[oldName]; return next; }); setFormError(''); return; }
    if ((parsed.config.servers || {})[nextName]) { setFormError('Server 名称已存在，请换一个名称。'); return; }
    replaceConfig(next => {
      const servers = next.servers || {};
      const renamed = {};
      // MCP 配置的 key 就是模型看到的 server 名称；改名时只迁移 key，保留 URL、Token、权限和确认规则。
      Object.entries(servers).forEach(([name, server]) => { renamed[name === oldName ? nextName : name] = server; });
      next.servers = renamed;
    });
    setRenameDrafts(drafts => { const next = {...drafts}; delete next[oldName]; return next; });
    setFormError('已改名为 ' + nextName + '，记得保存 MCP 配置。');
  }

  function addServer() {
    const name = cleanMCPServerName(newServer.name);
    if (!name) { setFormError('请先填写 Server 名称。'); return; }
    if ((parsed.config.servers || {})[name]) { setFormError('Server 名称已存在，请换一个名称。'); return; }
    if (!String(newServer.url || '').trim() && !String(newServer.path || '').trim()) { setFormError('请填写 MCP HTTP 地址；Docker 部署访问本机服务通常用 http://host.docker.internal:18766/mcp。'); return; }
    replaceConfig(next => { next.servers[name] = cleanMCPServerDraft(newServer); });
    setNewServer(defaultMCPServerDraft());
    setFormError('');
  }

  function serverStatusSummary(status) {
    if (!status) return '未检测';
    if (status.last_error) return status.last_error;
    if (status.disabled) return '已禁用，不会参与模型工具调用。';
    return 'allow ' + status.allow_count + ' · deny ' + status.deny_count + ' · confirm ' + status.confirm_count + ' · token ' + (status.has_token ? '已配置' : '无');
  }

  const renderServerForm = (name, server, isNew = false) => {
    const draft = isNew ? newServer : mcpServerToDraft(server);
    const status = !isNew ? statusByName[name] : null;
    const update = patch => isNew ? setNewServer(s => ({...s, ...patch})) : patchServer(name, patch);
    const serverNameDraft = !isNew ? (renameDrafts[name] ?? name) : '';
    const urlHasLocalhost = /^http:\/\/(127\.0\.0\.1|localhost)(?=[:/]|$)/i.test(normalizeMCPURLDraft(draft.url));
    const urlHasSpace = /\s/.test(String(draft.url || ''));
    const tokenEnvLooksLikeHeader = String(draft.token_env || '').trim().toLowerCase() === 'authorization';
    const tokenExpiry = mcpTokenExpiryState(draft.token);
    return <div className={'mcp-form-card ' + (isNew ? 'new-server' : '')} key={isNew ? 'new' : name}>
      <div className="mcp-form-head">
        <div><b>{isNew ? '新增 MCP Server' : name}</b><div className="hint">{isNew ? '填地址即可，不需要手写 JSON。' : serverStatusSummary(status)}</div></div>
        {!isNew ? <div className="mcp-form-head-actions"><button className="secondary small" onClick={() => patchServer(name, {disabled: !draft.disabled})}>{draft.disabled ? '启用' : '禁用'}</button><button className="danger small" onClick={() => removeServer(name)}>删除</button></div> : null}
      </div>
      {isNew ? <label>Server 名称<input value={draft.name} onChange={e => setNewServer(s => ({...s, name: e.target.value}))} placeholder="例如 agentdock" /></label> : <label>Server 名称<div className="mcp-rename-row"><input value={serverNameDraft} onChange={e => setRenameDrafts(drafts => ({...drafts, [name]: e.target.value}))} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); renameServer(name); } }} placeholder="例如 agentdock" /><button type="button" className="secondary small" onClick={() => renameServer(name)} disabled={cleanMCPServerName(serverNameDraft) === name}>改名</button></div></label>}
      <div className="mcp-form-grid">
        <label>连接类型<select value={draft.type} onChange={e => update({type: e.target.value})}><option value="streamable-http">HTTP / Streamable HTTP</option></select></label>
        <label>状态<select value={draft.disabled ? 'disabled' : 'enabled'} onChange={e => update({disabled: e.target.value === 'disabled'})}><option value="enabled">启用</option><option value="disabled">禁用</option></select></label>
        <label>工具默认加载<select value={draft.tool_exposure} onChange={e => update({tool_exposure: e.target.value})}><option value="on_demand">按需加载</option><option value="direct">直接加载</option></select></label>
      </div>
      <div className="hint">按需加载只在搜索命中后向模型加入真实工具；单个工具可以在下方工具列表覆盖默认方式。</div>
      <label>MCP HTTP 地址<input value={draft.url} onChange={e => update({url: e.target.value})} placeholder="http://host.docker.internal:18766/mcp" /></label>
      <div className="hint">ChatDock 生产环境跑在 Docker 里，127.0.0.1 会连到容器自己；访问电脑上的 AgentDock 应填 http://host.docker.internal:18766/mcp，且 URL 里不能有空格。</div>
      {urlHasLocalhost || urlHasSpace ? <div className="mcp-inline-warning"><b>当前地址可能连不上</b><span>{urlHasSpace ? 'URL 里有空格；' : ''}{urlHasLocalhost ? 'Docker 内不能用 127.0.0.1 访问宿主机。' : ''}</span><button className="secondary small" onClick={() => update({url: dockerHostMCPURL(draft.url)})}>改成 Docker 宿主机地址</button></div> : null}
      <details className="mcp-advanced-fields">
        <summary>高级权限、Token 和缓存</summary>
        <div className="mcp-form-grid">
          <label>认证方式<select value={draft.auth_type} onChange={e => update({auth_type: e.target.value})}><option value="none">无</option><option value="bearer">Bearer Token</option></select></label>
          <label>Token 环境变量名（可选）<input value={draft.token_env} onChange={e => update({token_env: e.target.value})} placeholder="例如 AGENTDOCK_MCP_TOKEN，不是 Authorization" /></label>
        </div>
        {tokenEnvLooksLikeHeader ? <div className="mcp-inline-warning"><b>这里不要填 Authorization</b><span>这是环境变量名，不是 HTTP Header 名。已经在下方 Token 粘贴值时可以留空。</span><button className="secondary small" onClick={() => update({token_env: ''})}>清空此项</button></div> : null}
        {draft.auth_type !== 'none' ? <label>Token<input type="password" value={draft.token} onChange={e => update({token: e.target.value})} placeholder="粘贴 AgentDock Bearer Token；没有就需要重新授权生成" /></label> : null}
        {draft.auth_type !== 'none' && tokenExpiry ? <div className={'mcp-inline-warning ' + (tokenExpiry.expired ? 'danger' : 'ok')}><b>{tokenExpiry.expired ? 'Token 已过期，需要重新生成' : 'Token 有效期'}</b><span>{tokenExpiry.text}</span></div> : null}
        <div className="mcp-form-grid">
          <label>超时 ms<input type="number" inputMode="numeric" value={draft.timeout_ms} onChange={e => update({timeout_ms: e.target.value})} placeholder="30000" /></label>
          <label>工具缓存 ms<input type="number" inputMode="numeric" value={draft.cache_ttl_ms} onChange={e => update({cache_ttl_ms: e.target.value})} placeholder="可留空" /></label>
        </div>
        {!isNew ? <MCPToolPolicyPicker name={name} draft={draft} tools={serverTools[name] || []} loading={!!loadingTools[name]} onRefresh={() => refreshServerTools(name)} onChange={patch => update(patch)} /> : <div className="hint">添加 Server 后再选择允许、禁止和调用前确认的工具。</div>}
        <label>本地路径备注<input value={draft.path} onChange={e => update({path: e.target.value})} placeholder="仅作为备注保留；当前 MCP 调用使用 HTTP 地址" /></label>
      </details>
      <div className="mcp-form-actions">
        {isNew ? <button onClick={addServer}>添加 Server</button> : <button className="secondary" onClick={() => testMCP(name)} disabled={draft.disabled || !draft.url}>测试此 Server</button>}
        {!isNew && status?.last_error ? <button className="secondary" onClick={() => patchServer(name, {disabled: true})}>禁用异常 Server</button> : null}
      </div>
    </div>;
  };

  return <>
    <section className="settings-section mcp-overview-section builtin-tools-section">
      <div className="settings-section-head"><div><b>ChatDock 内置工具</b><div className="hint">定时任务、图片和模型供应商工具</div></div></div>
      <div className="builtin-default-exposure">
        <div><b>默认加载方式</b><span>未单独设置的工具继承此方式</span></div>
        <select aria-label="内置工具默认加载方式" value={builtinDraft.tool_exposure} onChange={e => patchBuiltinTools({tool_exposure: e.target.value})}><option value="direct">直接加载</option><option value="on_demand">按需加载</option></select>
      </div>
      <BuiltinToolExposurePicker draft={builtinDraft} tools={builtinTools || []} onChange={patchBuiltinTools} />
    </section>
    <section className="settings-section mcp-overview-section mcp-status-section">
      <div className="settings-section-head"><div><b>MCP Server</b><div className="hint">连接状态和工具权限</div></div><button className="secondary small mcp-detect-button" onClick={() => loadMCPStatus?.()}>检测</button></div>
      <div id="mcpStatusCards" className="mcp-server-list">{mcpStatus.length ? mcpStatus.map(s => <div key={s.name} className="mcp-server-row"><div className="mcp-server-main"><b>{s.name}</b><span>{s.url || '未填写 HTTP 地址'}</span>{s.last_error ? <em>{s.last_error}</em> : null}</div><div className="mcp-server-meta"><span className={'badge ' + runStatusClass(s.last_status || 'unknown')}>{runStatusLabel(s.last_status || 'unknown')}</span><small>allow {s.allow_count} · deny {s.deny_count} · confirm {s.confirm_count}</small><button className="secondary small" onClick={() => testMCP(s.name)} disabled={s.disabled || !s.url}>测试</button></div></div>) : <div className="empty compact">尚未配置 MCP Server。</div>}</div>
    </section>
    {parsed.error ? <div className="backup-health warn">当前配置 JSON 损坏，表单无法解析：{parsed.error}。可以在下方高级区修复原始内容。</div> : null}
    {!parsed.error ? <div className="mcp-form-list redesigned">{serverNames.length ? serverNames.map(name => renderServerForm(name, parsed.config.servers[name])) : <div className="empty compact">暂无 Server，先添加一个 HTTP MCP 地址。</div>}{renderServerForm('', {}, true)}</div> : null}
    {formError ? <div className="backup-health warn">{formError}</div> : null}
    <div className="settings-actions mcp-primary-actions"><button className={mcpConfigDirty ? 'settings-inline-save-button dirty' : 'settings-inline-save-button'} onClick={() => saveMCPConfig?.()} disabled={!mcpConfigDirty || saving}>{saving ? '保存中…' : mcpConfigDirty ? '保存 MCP 更改' : 'MCP 已保存'}</button><button className="secondary" onClick={() => { if (!mcpConfigDirty || window.confirm('重新加载会丢弃尚未保存的 MCP 修改，确定继续吗？')) loadMCPConfig?.(); }}>重新加载</button><button className="secondary" onClick={() => testMCP()}>测试默认 MCP</button></div>
    <details className="mcp-raw-json"><summary>高级：查看 / 编辑原始 JSON</summary><textarea className="mcp-editor" value={mcpConfig} onChange={e => setMcpConfig(e.target.value)} /></details>
  </>;
}

function scheduledTaskContextLabel(mode) {
  return ({stateless: '每次独立执行', last_result: '带上次结果', session: '连续会话'})[mode] || '每次独立执行';
}

function TaskCard({ task, editScheduledTask, deleteScheduledTask, toggleScheduledTask, runScheduledTaskNow, viewScheduledTaskRuns, openScheduledTaskSession }) {
  const prompt = (task.prompt || '').trim().slice(0, 160) || '无提示内容';
  const closeMenuAndRun = (event, action) => {
    event.currentTarget.closest('details')?.removeAttribute('open');
    action();
  };

  return <article className="task-card automation-task-card">
    <header className="task-head automation-task-head">
      <div className="automation-task-title-wrap">
        <div className="task-name">{task.title || '未命名任务'}</div>
        {task.running ? <span className="automation-task-running">运行中</span> : null}
      </div>
      <div className="automation-task-head-actions">
        <span className={'badge ' + taskStatusClass(task)}>{taskStatusLabel(task)}</span>
        <details className="automation-task-more">
          <summary aria-label="更多任务操作" title="更多操作">•••</summary>
          <div className="automation-task-menu">
            {task.session_id ? <button type="button" className="secondary small" onClick={event => closeMenuAndRun(event, () => openScheduledTaskSession(task.session_id))}>打开最近会话</button> : null}
            <button type="button" className="secondary small" onClick={event => closeMenuAndRun(event, () => editScheduledTask(task.id))}>编辑任务</button>
            <button type="button" className="danger small" onClick={event => closeMenuAndRun(event, () => deleteScheduledTask(task.id))}>删除任务</button>
          </div>
        </details>
      </div>
    </header>

    <div className="task-desc automation-task-desc" title={prompt}>{prompt}</div>

    {task.running ? <div className="hint automation-task-notice">任务运行中：编辑和启用状态会从下次运行生效；删除不会中断已发出的模型请求。</div> : null}
    {task.last_error ? <div className="task-error automation-task-error">上次错误：{task.last_error}</div> : null}

    <div className="automation-task-meta-list">
      <div className="task-meta">{scheduleSummary(task)}</div>
      <div className="task-meta">上下文：{scheduledTaskContextLabel(task.context_mode || 'stateless')}{task.context_mode === 'session' && task.session_id ? ' · 会话 ' + task.session_id : ''}</div>
    </div>

    <footer className="automation-task-footer">
      <label className="task-toggle automation-task-toggle"><input type="checkbox" checked={!!task.enabled} onChange={e => toggleScheduledTask(task.id, e.target.checked)} /><span>启用</span></label>
      <div className="task-actions automation-task-primary-actions">
        <button type="button" className="secondary small" disabled={task.running} onClick={() => runScheduledTaskNow(task.id)}>立即运行</button>
        <button type="button" className="secondary small" onClick={() => viewScheduledTaskRuns(task.id)}>查看记录</button>
      </div>
    </footer>
  </article>;
}

function DataStatus({ dataStatus, onCopy }) {
  if (!dataStatus) return <div className="hint">尚未加载数据状态。</div>;
  const databaseItems = [
    ['数据目录', safePathName(dataStatus.data_dir)],
    ['数据库健康', dataStatus.database_healthy ? '正常' : (dataStatus.database_warning || '需要检查')],
    ['WAL', dataStatus.wal_enabled ? '启用' : '未检测到'],
    ['工作空间 / 会话', `${dataStatus.workspace_count || 0} / ${dataStatus.session_count || 0}`],
  ];
  const backupItems = [
    ['备份目录', safePathName(dataStatus.backup_dir || '未检测到')],
    ['备份数量', String(dataStatus.backup_count || 0)],
    ['已检查目录', String((dataStatus.backup_checked_dirs || []).length)],
    ['最近备份', dataStatus.latest_backup_at ? fmtTime(dataStatus.latest_backup_at) + ' · ' + fmtBytes(dataStatus.latest_backup_size_bytes) + ' · ' + fmtRelativeAge(dataStatus.latest_backup_age_seconds) : '暂无数据库备份'],
  ];
  const backups = dataStatus.backups || [];
  return <>
    {dataStatus.backup_warning ? <div className="backup-health warn">{dataStatus.backup_warning}</div> : <div className="backup-health ok">数据库备份状态正常。</div>}
    <div className="settings-block-head"><label>数据库</label></div>
    <div id="dataStatus" className="data-info-table">{databaseItems.map(item => <div className="data-info-row" key={item[0]}><span>{item[0]}</span><b>{item[1]}</b></div>)}</div>
    <div className="settings-block-head"><label>备份</label></div>
    <div className="data-info-table backup-info-table">{backupItems.map(item => <div className="data-info-row" key={item[0]}><span>{item[0]}</span><b>{item[1]}</b></div>)}</div>
    {(dataStatus.backup_checked_dirs || []).length ? <details className="backup-path checked-dirs"><summary>查看已检查备份目录</summary>{dataStatus.backup_checked_dirs.map(dir => <code key={dir}>{dir}</code>)}</details> : null}
    {backups.length ? <div className="backup-list"><div className="settings-block-head"><label>最近数据库备份</label></div>{backups.map(item => <div className="backup-item" key={item.path || item.name}><div className="backup-main"><div><b>{item.name || safePathName(item.path)}</b><div className="hint">{fmtTime(item.updated_at)} · {fmtRelativeAge(item.age_seconds)} · {fmtBytes(item.size_bytes)}</div></div>{item.path ? <button className="secondary mini" onClick={() => onCopy?.(item.path)}>复制路径</button> : null}</div>{item.path ? <details className="backup-path"><summary>查看完整路径</summary><code>{item.path}</code></details> : null}</div>)}</div> : null}
  </>;
}

function SecurityModule({ systemStatus, setupStatus, dataStatus, mcpStatus, providers, loadSystemStatus, loadDataStatus, logout, onCopy }) {
  const text = diagnosticsText({setupStatus, systemStatus, dataStatus, mcpStatus, providers});
  const refreshStatus = () => Promise.allSettled([loadSystemStatus?.(), loadDataStatus?.()]);
  const setup = setupStatus || systemStatus?.setup || {};
  const data = dataStatus || systemStatus?.data || {};
  const healthy = Boolean(systemStatus?.ok);
  const activeWorkspace = setup.active_workspace || data.active_workspace || '-';
  const overview = [
    ['运行状态', healthy ? '正常' : '待检查'],
    ['当前工作空间', activeWorkspace],
    ['数据库大小', fmtBytes(data.database_size_bytes)],
    ['最近备份', data.latest_backup_at ? fmtRelativeAge(data.latest_backup_age_seconds) : '暂无'],
  ];
  const runtime = [
    ['访问地址', systemStatus?.addr || '-'],
    ['Web 资源', systemStatus?.web_dir ? safePathName(systemStatus.web_dir) : '内嵌'],
    ['数据库文件', safePathName(data.database_path || systemStatus?.database)],
    ['会话 / 工作空间', `${data.session_count ?? '-'} / ${data.workspace_count ?? setup.workspace_count ?? '-'}`],
    ['模型供应商', String((providers || []).length)],
    ['MCP Server', String((mcpStatus || []).length)],
  ];

  return <>
    <div className="settings-block-head"><label>系统状态</label><button className="secondary small" onClick={refreshStatus}>刷新状态</button></div>
    <div className={'setup-banner show ' + (healthy ? 'ok' : '')}>
      <div><b>{healthy ? 'ChatDock 运行正常' : 'ChatDock 状态待确认'}</b><div className="hint">{healthy ? '核心服务可用，配置和数据状态已加载。' : '刷新后仍异常时，可展开下方诊断信息排查。'}</div></div>
      <span className={'badge ' + (healthy ? 'ok' : 'warn')}>{healthy ? 'healthy' : 'unknown'}</span>
    </div>
    <div className="workspace-summary-grid">{overview.map(([label, value]) => <div className="workspace-summary-card" key={label}><span>{label}</span><b className="stat-value" title={value}>{value}</b></div>)}</div>
    <section className="settings-section data-table-section">
      <div className="settings-section-head"><div><b>运行环境</b><div className="hint">服务入口、数据与扩展连接概览</div></div></div>
      <div className="data-info-table">{runtime.map(([label, value]) => <div className="data-info-row" key={label}><span>{label}</span><b title={value}>{value}</b></div>)}</div>
    </section>
    <details className="settings-section settings-disclosure">
      <summary><div><b>数据与备份</b><p>数据库健康、备份目录和最近备份</p></div><span>展开</span></summary>
      <DataStatus dataStatus={dataStatus} onCopy={onCopy} />
    </details>
    <details className="settings-section settings-disclosure">
      <summary><div><b>完整诊断信息</b><p>排障或反馈问题时再展开查看</p></div><span>展开</span></summary>
      <pre className="diagnostics-preview">{text}</pre>
      <div className="settings-actions"><button className="secondary" onClick={() => onCopy?.(text)}>复制诊断信息</button></div>
    </details>
    <div className="settings-actions"><button className="secondary" onClick={logout}>登录 / 切换账号</button></div>
  </>;
}
