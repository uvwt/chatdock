// Configuration-center modules: model/provider, MCP tools, and system diagnostics.
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ArrowLeft,
  Bot,
  Check,
  CircleX,
  Folder,
  ListTodo,
  RefreshCw,
  ShieldCheck,
  Wrench,
} from './icons.js';
import '../styles/settings-entry.css';
import { TextCard } from './base.jsx';
import { settingsModules, diagnosticsText, fmtBytes, fmtRelativeAge, fmtTime, runStatusClass, runStatusLabel, safePathName } from '../lib/appUtils.js';
import { mcpAuthDraft, mcpAuthPayload } from '../lib/mcpAuthDraft.js';
import { ProjectsPage, ScheduledTasksPage } from './managementPages.jsx';

const settingsModuleMeta = {
  model: {label: '模型', desc: '选择默认模型并管理模型供应商。', icon: Bot},
  tools: {label: '工具', desc: '管理 ChatDock 内置工具和 MCP Server。', icon: Wrench},
  projects: {label: '项目', desc: '管理项目名称、提示词和会话归属。', icon: Folder},
  automation: {label: '定时任务', desc: '创建、运行和暂停自动执行任务。', icon: ListTodo},
  security: {label: '系统', desc: '查看运行状态、数据库、备份、诊断信息和访问入口。', icon: ShieldCheck},
};

export function SettingsPanel(props) {
  const {
    activeModule, api, closeSettings, config, configDirty, mcpConfigDirty, dataStatus, editModelProvider, deleteModelProvider, testSavedModelProvider, fetchSavedProviderModels,
    loadDataStatus, loadMCPStatus, loadSystemStatus, logout, builtinTools, mcpConfig, mcpStatus, onCopy, providers, projectPromptPreview, refreshProductState, refreshVisibleSettings,
    saveConfig, saveMCPConfig, setConfig, setupStatus, switchSettingsModule, systemStatus,
    testMCP, fetchMCPServerTools, testModelProvider, addCandidateModelsToProvider, loadingModels,
    projects, projectSessionCounts, editProject, deleteProject, openProjectSessions, loadProjects, onPinnedProjectChange, startProjectConversation, showToast,
    scheduledTasks, taskSearch, setTaskSearch, editScheduledTask, deleteScheduledTask, setScheduledTasks, toggleScheduledTask, runScheduledTaskNow, openScheduledTaskSession, loadScheduledTasks, onPinnedTaskChange,
  } = props;
  const saveTimerRef = useRef(null);
  const [saveState, setSaveState] = useState({scope: '', status: 'idle', message: ''});
  useEffect(() => () => window.clearTimeout(saveTimerRef.current), []);

  const saveModelConfig = useCallback(async () => {
    if (!configDirty || !saveConfig) return;
    window.clearTimeout(saveTimerRef.current);
    setSaveState({scope: 'config', status: 'saving', message: ''});
    try {
      await saveConfig({silent: true});
      setSaveState({scope: 'config', status: 'saved', message: ''});
      saveTimerRef.current = window.setTimeout(() => setSaveState({scope: '', status: 'idle', message: ''}), 2200);
    } catch (error) {
      setSaveState({scope: 'config', status: 'error', message: error?.message || '保存失败，请稍后重试。'});
    }
  }, [configDirty, saveConfig]);

  const configSaveState = saveState.scope === 'config' ? saveState : {scope: 'config', status: 'idle', message: ''};
  const refreshSettings = () => {
    if ((configDirty || mcpConfigDirty) && !window.confirm('刷新会丢弃尚未保存的配置修改，确定继续吗？')) return;
    (refreshVisibleSettings || refreshProductState)?.();
  };
  return <section className="settings">
    <header className="settings-header">
      <div className="settings-header-main">
        <button className="secondary small settings-back-button icon-button" onClick={() => closeSettings()} aria-label="返回聊天" title="返回聊天"><ArrowLeft className="settings-header-icon settings-back-icon" size={17} aria-hidden="true" /></button>
        <div>
          <div className="settings-title-row"><h2>配置中心</h2></div>
          <p>统一管理模型、供应商、工具、项目、定时任务与系统。</p>
        </div>
      </div>
      <div className="settings-header-actions"><button className="secondary small settings-refresh-button" onClick={refreshSettings} aria-label="刷新配置" title="刷新配置"><RefreshCw className="settings-header-icon settings-refresh-icon" size={16} aria-hidden="true" /><span className="settings-refresh-text">刷新</span></button></div>
    </header>
    <div className="settings-sidebar">
      <select className="settings-mobile-module-select" value={activeModule} onChange={e => switchSettingsModule(e.target.value)} aria-label="选择配置模块">
        {settingsModules.map(m => <option key={m} value={m}>{moduleLabel(m)}</option>)}
      </select>
      <nav className="module-tabs" aria-label="配置模块">{settingsModules.map(m => {
        const ModuleIcon = settingsModuleMeta[m]?.icon;
        return <button key={m} className={'module-tab ' + (activeModule === m ? 'active' : '')} onClick={() => switchSettingsModule(m)}>
          <span className="module-tab-main">{ModuleIcon ? <ModuleIcon className="module-tab-icon" size={17} aria-hidden="true" /> : null}<span className="module-tab-label">{moduleLabel(m)}</span></span>
        </button>;
      })}</nav>
    </div>
    <main className="settings-content">
      <ModuleView name="model" activeModule={activeModule} dirty={configDirty} saveState={configSaveState} onSave={saveModelConfig}>
        <ModelModule config={config} setConfig={setConfig} projectPromptPreview={projectPromptPreview} testModelProvider={testModelProvider} providers={providers} />
        <ProvidersModule providers={providers} editModelProvider={editModelProvider} deleteModelProvider={deleteModelProvider} testSavedModelProvider={testSavedModelProvider} fetchSavedProviderModels={fetchSavedProviderModels} addCandidateModelsToProvider={addCandidateModelsToProvider} loadingModels={loadingModels} />
      </ModuleView>
      <ModuleView name="tools" activeModule={activeModule}><ToolsModule builtinTools={builtinTools} mcpStatus={mcpStatus} mcpConfig={mcpConfig} saveMCPConfig={saveMCPConfig} loadMCPStatus={loadMCPStatus} testMCP={testMCP} fetchMCPServerTools={fetchMCPServerTools} /></ModuleView>
      <ModuleView name="projects" activeModule={activeModule} bare>
        <ProjectsPage
          api={api}
          embedded
          projects={projects}
          projectSessionCounts={projectSessionCounts}
          editProject={editProject}
          deleteProject={deleteProject}
          showProjectPromptPreview={showProjectPromptPreview}
          projectPromptPreview={projectPromptPreview}
          openProjectSessions={openProjectSessions}
          loadProjects={loadProjects}
          onPinnedProjectChange={onPinnedProjectChange}
          startProjectConversation={startProjectConversation}
          showToast={showToast}
        />
      </ModuleView>
      <ModuleView name="automation" activeModule={activeModule} bare>
        <ScheduledTasksPage
          api={api}
          embedded
          scheduledTasks={scheduledTasks}
          taskSearch={taskSearch}
          setTaskSearch={setTaskSearch}
          editScheduledTask={editScheduledTask}
          deleteScheduledTask={deleteScheduledTask}
          setScheduledTasks={setScheduledTasks}
          showToast={showToast}
          toggleScheduledTask={toggleScheduledTask}
          runScheduledTaskNow={runScheduledTaskNow}
          openScheduledTaskSession={openScheduledTaskSession}
          loadScheduledTasks={loadScheduledTasks}
          onPinnedTaskChange={onPinnedTaskChange}
        />
      </ModuleView>
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
function ModuleView({ name, activeModule, children, dirty = false, saveState, onSave, bare = false }) {
  return <div className={'module-view ' + (activeModule === name ? 'active ' : '') + (dirty ? 'dirty ' : '') + (bare ? 'bare' : '')} data-module-view={name}>
    {bare ? null : <div className="module-view-title"><div><span>{moduleLabel(name)}</span><p>{moduleDescription(name)}</p></div></div>}
    {bare ? null : <SettingsSaveState dirty={dirty} state={saveState} onSave={onSave} />}
    {children}
  </div>;
}

function SettingsSaveState({ dirty, state = {}, onSave }) {
  const status = state.status === 'saving' ? 'saving' : state.status === 'error' ? 'error' : dirty ? 'dirty' : state.status === 'saved' ? 'saved' : 'idle';
  if (status === 'idle') return null;
  const content = {
    dirty: ['有未保存修改', '保存后用于新对话和定时任务。'],
    saving: ['正在保存', '请稍候。'],
    saved: ['已保存', '模型设置已经生效。'],
    error: ['保存失败', state.message || '请检查设置后重试。'],
  }[status];
  return <div className={'settings-save-state ' + status} role={status === 'error' ? 'alert' : 'status'}>
    <div className="settings-save-state-copy"><span className="settings-save-state-icon" aria-hidden="true">{status === 'saved' ? <Check size={15} /> : status === 'error' ? <CircleX size={15} /> : null}</span><div><b>{content[0]}</b><span>{content[1]}</span></div></div>
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

function ModelModule({ config, setConfig, projectPromptPreview, testModelProvider, providers }) {
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
  const selectedProviderModels = normalizeModelNames([...providerModels(activeProvider), config.model].filter(Boolean));
  const fallbackModels = normalizeModelNames([...providerModels(fallbackProvider), config.fallback_model].filter(Boolean));
  const contextMode = config.context_mode || 'auto';
  return <>
    <section className="model-quick-panel model-page-single">
      <div className="model-quick-head"><div><b>默认模型</b><span>{activeProvider?.name || '未选择供应商'} · {config.model || activeProvider?.default_model || '未选择模型'}</span></div><div className="model-quick-actions"><button className="secondary" onClick={() => testModelProvider?.()}>测试连接</button></div></div>
      <div className="model-quick-grid">
        <label>供应商<select value={activeProvider?.id || ''} onChange={e => chooseProvider(e.target.value)}>{providers.length ? providers.map(p => <option key={p.id} value={p.id}>{p.name || p.id}</option>) : <option value="">未配置供应商</option>}</select></label>
        <label>模型<select value={config.model || ''} onChange={e => chooseModel(e.target.value)} disabled={!activeProvider}><option value="">{activeProvider ? '选择模型' : '先选择供应商'}</option>{selectedProviderModels.map(name => <option key={name} value={name}>{name}</option>)}</select></label>
      </div>
      {!selectedProviderModels.length ? <div className="hint">当前供应商没有可用模型，请在下方编辑供应商。</div> : null}
      <details className="model-option-group">
        <summary><span><b>备用模型</b><small>{fallbackProvider ? (fallbackProvider.name || fallbackProvider.id) + ' · ' + (config.fallback_model || fallbackProvider.default_model || '默认模型') : '未启用'}</small></span><em>配置</em></summary>
        <div className="model-quick-grid fallback-model-grid">
          <label>备用供应商<select value={fallbackProvider?.id || ''} onChange={e => chooseFallbackProvider(e.target.value)}><option value="">不使用备用模型</option>{providers.map(p => <option key={p.id} value={p.id}>{p.name || p.id}</option>)}</select></label>
          <label>备用模型<select value={config.fallback_model || ''} onChange={e => chooseFallbackModel(e.target.value)} disabled={!fallbackProvider}><option value="">{fallbackProvider ? '使用供应商默认模型' : '先选择备用供应商'}</option>{fallbackModels.map(name => <option key={name} value={name}>{name}</option>)}</select></label>
        </div>
        <div className="hint model-fallback-hint">仅在主模型尚未输出内容或执行工具前失败时切换。</div>
      </details>
    </section>
    <div className="model-inline-grid model-advanced-grid">
      <details className="settings-section model-inline-card reply-inline-card" open>
        <summary><div><b>回复设置</b><p>上下文、随机性和系统提示词</p></div><span>展开</span></summary>
        <div className="settings-form-grid compact"><label>上下文<select value={contextMode} onChange={e => update('context_mode', e.target.value)}><option value="auto">自动</option><option value="compact">精简</option><option value="expanded">更多历史</option><option value="custom">自定义</option></select></label><label>Temperature<input type="number" step="0.1" min="0" max="2" value={config.temperature} onChange={e => update('temperature', e.target.value)} /></label></div>
        {contextMode === 'custom' ? <label>最近消息数<input type="number" min="1" max="200" value={config.max_context_messages} onChange={e => update('max_context_messages', e.target.value)} /></label> : null}
        <details className="model-mini-details"><summary>全局系统提示词</summary><textarea className="system-prompt-editor compact" value={config.system_prompt} onChange={e => update('system_prompt', e.target.value)} /></details>
        <div className="thinking-options compact"><label className="check-row"><input type="checkbox" checked={!!config.hide_thinking} onChange={e => update('hide_thinking', e.target.checked)} /> 隐藏模型思考内容</label></div>
        {projectPromptPreview ? <pre className="code-preview compact">{projectPromptPreview}</pre> : null}
      </details>
      <details className="settings-section model-inline-card embedding-inline-card">
        <summary><div><b>工具搜索</b><p>向量服务，可选</p></div><span>展开</span></summary>
        <label>Base URL<input value={config.embedding_base_url || ''} onChange={e => update('embedding_base_url', e.target.value)} placeholder="http://127.0.0.1:8000/v1" /></label>
        <label>模型<input value={config.embedding_model || 'BAAI/bge-m3'} onChange={e => update('embedding_model', e.target.value)} placeholder="BAAI/bge-m3" /></label>
        <details className="model-mini-details"><summary>API Key</summary><input type="password" value={config.embedding_api_key || ''} onChange={e => update('embedding_api_key', e.target.value)} placeholder={config.has_embedding_api_key ? '已保存，留空不修改' : '可留空'} /></details>
        <div className="hint">用于工具搜索；不配置则使用关键词搜索。</div>
      </details>
    </div>
  </>;
}

function ProvidersModule({ providers, editModelProvider, deleteModelProvider, testSavedModelProvider, fetchSavedProviderModels, addCandidateModelsToProvider, loadingModels }) {
  const [candidatePicker, setCandidatePicker] = useState(null);

  async function discoverModels(provider) {
    const models = await fetchSavedProviderModels?.(provider);
    if (!models) return;
    setCandidatePicker({provider, models, selected: []});
  }

  function toggleCandidate(name) {
    setCandidatePicker(current => {
      if (!current) return current;
      const selected = current.selected.includes(name) ? current.selected.filter(item => item !== name) : [...current.selected, name];
      return {...current, selected};
    });
  }

  async function saveCandidates() {
    if (!candidatePicker?.selected.length) return;
    await addCandidateModelsToProvider?.(candidatePicker.provider.id, candidatePicker.selected);
    setCandidatePicker(null);
  }

  const existingModels = normalizeModelNames([...(candidatePicker?.provider?.models || []), candidatePicker?.provider?.default_model].filter(Boolean));
  return <section className="settings-section provider-management-section">
    <div className="settings-section-head"><div><b>模型供应商</b><p>供应商在弹窗中保存；默认模型在上方选择。</p></div><button className="secondary small" onClick={() => editModelProvider(null)}>新增供应商</button></div>
    <div className="provider-grid provider-page-grid">{providers.length ? providers.map(provider => <TextCard key={provider.id} title={provider.name || provider.id} hint={provider.base_url || '-'} badge={provider.enabled ? (provider.type || 'openai') : '停用'}><div className="product-meta">默认：{provider.default_model || '-'} · 模型 {provider.models?.length || 0} · Key {provider.api_keys?.length || (provider.has_api_key ? 1 : 0)}</div><div className="product-actions"><button className="secondary small" onClick={() => editModelProvider(provider)}>编辑</button><button className="secondary small" onClick={() => discoverModels(provider)} disabled={loadingModels}>{loadingModels ? '读取中…' : '发现模型'}</button><button className="secondary small" onClick={() => testSavedModelProvider(provider)}>测试</button><button className="danger small" onClick={() => deleteModelProvider(provider)}>删除</button></div></TextCard>) : <div className="empty compact">还没有模型供应商。</div>}</div>
    {candidatePicker ? <div className="app-modal-backdrop show" onClick={event => { if (event.target === event.currentTarget) setCandidatePicker(null); }}>
      <div className="app-modal-card candidate-model-modal" role="dialog" aria-modal="true" aria-labelledby="candidateModelTitle">
        <div className="app-modal-form">
          <div id="candidateModelTitle" className="app-modal-title">发现模型 · {candidatePicker.provider.name || candidatePicker.provider.id}</div>
          <div className="candidate-model-modal-body">
            <p>选择要加入该供应商的模型，最后统一保存。</p>
            <div className="model-options candidate-model-options">{candidatePicker.models.map(name => {
              const existing = existingModels.includes(name);
              const selected = candidatePicker.selected.includes(name);
              return <button key={name} type="button" className={'model-option candidate ' + (existing ? 'added ' : '') + (selected ? 'active' : '')} onClick={() => toggleCandidate(name)} disabled={existing}>{existing ? '已存在 · ' : selected ? '已选择 · ' : ''}{name}</button>;
            })}</div>
          </div>
          <div className="app-modal-actions"><button type="button" className="secondary" onClick={() => setCandidatePicker(null)}>取消</button><button type="button" onClick={saveCandidates} disabled={!candidatePicker.selected.length}>保存所选模型{candidatePicker.selected.length ? `（${candidatePicker.selected.length}）` : ''}</button></div>
        </div>
      </div>
    </div> : null}
  </section>;
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
  return {
    type: server.type || 'streamable-http',
    url: server.url || '',
    description: server.description || '',
    path: server.path || '',
    disabled: !!server.disabled,
    ...mcpAuthDraft(server.auth),
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
  const description = String(draft.description || '').trim();
  const path = String(draft.path || '').trim();
  if (type && type !== 'streamable-http') next.type = type;
  if (url) next.url = url;
  if (description) next.description = description;
  if (path) next.path = path;
  if (draft.disabled) next.disabled = true;
  const auth = mcpAuthPayload(draft);
  if (auth) next.auth = auth;
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
  return {name: '', type: 'streamable-http', url: '', description: '', path: '', disabled: false, auth_type: 'none', token: '', token_env: '', saved_token_ref: '', allow_tools: '', deny_tools: '', confirm_tools: '', tool_exposure: 'on_demand', tool_overrides: {}, timeout_ms: '30000', cache_ttl_ms: ''};
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

function ToolsModule({ builtinTools, mcpStatus, mcpConfig, saveMCPConfig, loadMCPStatus, testMCP, fetchMCPServerTools }) {
  const [editor, setEditor] = useState(null);
  const [formError, setFormError] = useState('');
  const [serverTools, setServerTools] = useState({});
  const [loadingTools, setLoadingTools] = useState({});
  const [saving, setSaving] = useState(false);
  const parsed = useMemo(() => parseMCPConfigDraft(mcpConfig), [mcpConfig]);
  const serverNames = Object.keys(parsed.config.servers || {}).sort();
  const statusByName = useMemo(() => Object.fromEntries((mcpStatus || []).map(status => [status.name, status])), [mcpStatus]);

  function openBuiltinEditor() {
    setFormError('');
    setEditor({kind: 'builtin', name: 'chatdock', draft: builtinToolsToDraft(parsed.config.builtin_tools)});
  }

  function openServerEditor(name) {
    setFormError('');
    setEditor({kind: 'server', name, draft: {name, ...mcpServerToDraft(parsed.config.servers[name])}});
  }

  function openNewServerEditor() {
    setFormError('');
    setEditor({kind: 'new', name: '', draft: defaultMCPServerDraft()});
  }

  function updateEditor(patch) {
    setEditor(current => current ? {...current, draft: {...current.draft, ...patch}} : current);
  }

  async function refreshServerTools(name) {
    if (!name || !fetchMCPServerTools) return;
    setLoadingTools(previous => ({...previous, [name]: true}));
    try {
      const tools = await fetchMCPServerTools(name);
      setServerTools(previous => ({...previous, [name]: tools || []}));
      setFormError('已读取 ' + name + ' 的 ' + ((tools || []).length) + ' 个工具。');
    } catch (error) {
      setFormError('读取工具列表失败：' + error.message);
    } finally {
      setLoadingTools(previous => ({...previous, [name]: false}));
    }
  }

  async function saveEditor() {
    if (!editor || !saveMCPConfig) return;
    const next = {...parsed.config, servers: {...(parsed.config.servers || {})}};

    if (editor.kind === 'builtin') {
      next.builtin_tools = cleanBuiltinToolsDraft(editor.draft);
    } else {
      const nextName = cleanMCPServerName(editor.draft.name);
      if (!nextName) { setFormError('Server 名称不能为空。'); return; }
      if (nextName.toLowerCase() === 'chatdock') { setFormError('ChatDock 是内置资源名，请换一个 Server 名称。'); return; }
      if (!String(editor.draft.url || '').trim() && !String(editor.draft.path || '').trim()) { setFormError('请填写 MCP HTTP 地址。'); return; }
      if (nextName !== editor.name && next.servers[nextName]) { setFormError('Server 名称已存在，请换一个名称。'); return; }
      if (editor.kind === 'server' && editor.name !== nextName) delete next.servers[editor.name];
      next.servers[nextName] = cleanMCPServerDraft(editor.draft);
    }

    setSaving(true);
    setFormError('');
    try {
      await saveMCPConfig({content: stringifyMCPConfigDraft(next), silent: true});
      setEditor(null);
    } catch (error) {
      setFormError(error.message || '保存工具配置失败。');
    } finally {
      setSaving(false);
    }
  }

  async function deleteCurrentServer() {
    if (editor?.kind !== 'server' || !window.confirm('确定删除 MCP Server「' + editor.name + '」？')) return;
    const next = {...parsed.config, servers: {...(parsed.config.servers || {})}};
    delete next.servers[editor.name];
    setSaving(true);
    try {
      await saveMCPConfig({content: stringifyMCPConfigDraft(next), silent: true});
      setEditor(null);
    } catch (error) {
      setFormError(error.message || '删除 MCP Server 失败。');
    } finally {
      setSaving(false);
    }
  }

  const isBuiltin = editor?.kind === 'builtin';
  const isNew = editor?.kind === 'new';
  const serverDraft = !isBuiltin ? editor?.draft : null;
  const editorServerName = editor?.kind === 'server' ? editor.name : '';
  const urlHasLocalhost = serverDraft ? /^http:\/\/(127\.0\.0\.1|localhost)(?=[:/]|$)/i.test(normalizeMCPURLDraft(serverDraft.url)) : false;
  const urlHasSpace = serverDraft ? /\s/.test(String(serverDraft.url || '')) : false;
  const tokenEnvLooksLikeHeader = serverDraft ? String(serverDraft.token_env || '').trim().toLowerCase() === 'authorization' : false;
  const hasSavedToken = serverDraft ? !!String(serverDraft.saved_token_ref || '').trim() : false;
  const tokenExpiry = serverDraft ? mcpTokenExpiryState(serverDraft.token) : null;

  return <>
    <section className="settings-section mcp-overview-section mcp-directory-section">
      <div className="settings-section-head">
        <div><b>工具资源</b><p>点击资源进入编辑，修改在弹窗中直接保存。</p></div>
        <div className="mcp-directory-actions">
          <button className="secondary small" onClick={() => loadMCPStatus?.()}>检查连接</button>
          <button className="small" onClick={openNewServerEditor}>新增 Server</button>
        </div>
      </div>
      <div className="mcp-config-directory">
        <button type="button" className="secondary mcp-config-directory-row" onClick={openBuiltinEditor}>
          <span><b>ChatDock 内置工具</b><small>定时任务、图片和供应商工具</small></span>
          <em>{(builtinTools || []).length} 个 · 编辑</em>
        </button>
        {serverNames.map(name => {
          const server = parsed.config.servers[name];
          const status = statusByName[name];
          return <button type="button" className="secondary mcp-config-directory-row" key={name} onClick={() => openServerEditor(name)}>
            <span><b>{name}</b><small>{server.url || '未填写 HTTP 地址'}</small></span>
            <em className={'badge ' + runStatusClass(status?.last_status || 'unknown')}>{status?.last_status ? runStatusLabel(status.last_status) : '未检测'}</em>
          </button>;
        })}
        {!serverNames.length ? <div className="empty compact">暂无 MCP Server。</div> : null}
      </div>
    </section>
    {parsed.error ? <div className="backup-health warn">当前配置 JSON 损坏：{parsed.error}</div> : null}
    {editor ? <div className="app-modal-backdrop show" onClick={event => { if (event.target === event.currentTarget && !saving) setEditor(null); }}>
      <div className="app-modal-card provider-modal provider-modal-simple mcp-config-modal" role="dialog" aria-modal="true" aria-labelledby="mcpConfigModalTitle">
        <div className="app-modal-form">
          <div id="mcpConfigModalTitle" className="app-modal-title">{isBuiltin ? 'ChatDock 内置工具' : isNew ? '新增 MCP Server' : '编辑 ' + editor.name}</div>
          <div className="mcp-config-modal-body">
            {isBuiltin ? <section className="settings-section mcp-detail-section builtin-tools-section">
              <div className="builtin-default-exposure"><div><b>默认加载方式</b><span>未单独设置的工具继承此方式</span></div><select aria-label="内置工具默认加载方式" value={editor.draft.tool_exposure} onChange={event => updateEditor({tool_exposure: event.target.value})}><option value="direct">直接加载</option><option value="on_demand">按需加载</option></select></div>
              <BuiltinToolExposurePicker draft={editor.draft} tools={builtinTools || []} onChange={updateEditor} />
            </section> : <div className="mcp-form-card">
              <label>Server 名称<input value={serverDraft.name} onChange={event => updateEditor({name: event.target.value})} placeholder="例如 DockMini" /></label>
              <label>资源说明<input maxLength="240" value={serverDraft.description} onChange={event => updateEditor({description: event.target.value})} placeholder="例如 Mac mini 本机开发、文件、命令和 Git 能力" /></label>
              <div className="hint">说明会进入模型看到的轻量资源索引。</div>
              <div className="mcp-form-grid">
                <label>连接类型<select value={serverDraft.type} onChange={event => updateEditor({type: event.target.value})}><option value="streamable-http">HTTP / Streamable HTTP</option></select></label>
                <label>状态<select value={serverDraft.disabled ? 'disabled' : 'enabled'} onChange={event => updateEditor({disabled: event.target.value === 'disabled'})}><option value="enabled">启用</option><option value="disabled">禁用</option></select></label>
                <label>工具默认加载<select value={serverDraft.tool_exposure} onChange={event => updateEditor({tool_exposure: event.target.value})}><option value="on_demand">按需加载</option><option value="direct">直接加载</option></select></label>
              </div>
              <label>MCP HTTP 地址<input value={serverDraft.url} onChange={event => updateEditor({url: event.target.value})} placeholder="http://host.docker.internal:18766/mcp" /></label>
              {urlHasLocalhost || urlHasSpace ? <div className="mcp-inline-warning"><b>当前地址可能连不上</b><span>{urlHasSpace ? 'URL 里有空格；' : ''}{urlHasLocalhost ? 'Docker 内不能用 127.0.0.1 访问宿主机。' : ''}</span><button className="secondary small" onClick={() => updateEditor({url: dockerHostMCPURL(serverDraft.url)})}>改成 Docker 宿主机地址</button></div> : null}
              <details className="mcp-advanced-fields">
                <summary>高级权限、Token 和缓存</summary>
                <div className="mcp-form-grid">
                  <label>认证方式<select value={serverDraft.auth_type} onChange={event => updateEditor({auth_type: event.target.value})}><option value="none">无</option><option value="bearer">Bearer Token</option></select></label>
                  <label>Token 环境变量名（可选）<input value={serverDraft.token_env} onChange={event => updateEditor({token_env: event.target.value})} placeholder="例如 AGENTDOCK_MCP_TOKEN" /></label>
                </div>
                {tokenEnvLooksLikeHeader ? <div className="mcp-inline-warning"><b>这里不要填 Authorization</b><span>这里需要环境变量名，不是 HTTP Header 名。</span><button className="secondary small" onClick={() => updateEditor({token_env: ''})}>清空</button></div> : null}
                {hasSavedToken ? <div className="mcp-inline-warning ok mcp-token-state"><b>已保存 Token</b><span>留空不会修改；输入新值才会替换。</span><button type="button" className="secondary small" onClick={() => updateEditor({token: '', saved_token_ref: ''})}>清除 Token</button></div> : null}
                {serverDraft.auth_type !== 'none' ? <label>{hasSavedToken ? '替换 Token（可选）' : 'Token'}<input type="password" autoComplete="new-password" value={serverDraft.token} onChange={event => updateEditor({token: event.target.value})} placeholder={hasSavedToken ? '留空保持已保存 Token' : '粘贴 Bearer Token'} /></label> : null}
                {serverDraft.auth_type !== 'none' && tokenExpiry ? <div className={'mcp-inline-warning ' + (tokenExpiry.expired ? 'danger' : 'ok')}><b>{tokenExpiry.expired ? 'Token 已过期' : '新 Token 有效期'}</b><span>{tokenExpiry.text}</span></div> : null}
                <div className="mcp-form-grid">
                  <label>超时 ms<input type="number" inputMode="numeric" value={serverDraft.timeout_ms} onChange={event => updateEditor({timeout_ms: event.target.value})} placeholder="30000" /></label>
                  <label>工具缓存 ms<input type="number" inputMode="numeric" value={serverDraft.cache_ttl_ms} onChange={event => updateEditor({cache_ttl_ms: event.target.value})} placeholder="可留空" /></label>
                </div>
                {!isNew ? <MCPToolPolicyPicker name={editorServerName} draft={serverDraft} tools={serverTools[editorServerName] || []} loading={!!loadingTools[editorServerName]} onRefresh={() => refreshServerTools(editorServerName)} onChange={updateEditor} /> : <div className="hint">保存 Server 后可读取并配置工具权限。</div>}
                <label>本地路径备注<input value={serverDraft.path} onChange={event => updateEditor({path: event.target.value})} placeholder="可选备注" /></label>
              </details>
            </div>}
            {formError ? <div className="backup-health warn">{formError}</div> : null}
          </div>
          <div className="app-modal-actions mcp-config-modal-actions">
            <div className="mcp-modal-secondary-actions">{editor.kind === 'server' ? <><button type="button" className="danger" onClick={deleteCurrentServer} disabled={saving}>删除</button><button type="button" className="secondary" onClick={() => testMCP(editor.name)} disabled={saving}>测试已保存配置</button></> : null}</div>
            <div className="mcp-modal-primary-actions"><button type="button" className="secondary" onClick={() => setEditor(null)} disabled={saving}>取消</button><button type="button" onClick={saveEditor} disabled={saving}>{saving ? '保存中…' : isBuiltin ? '保存工具设置' : isNew ? '保存 Server' : '保存修改'}</button></div>
          </div>
        </div>
      </div>
    </div> : null}
  </>;
}

function DataStatus({ dataStatus, onCopy }) {
  if (!dataStatus) return <div className="hint">尚未加载数据状态。</div>;
  const databaseItems = [
    ['数据目录', safePathName(dataStatus.data_dir)],
    ['数据库健康', dataStatus.database_healthy ? '正常' : (dataStatus.database_warning || '需要检查')],
    ['WAL', dataStatus.wal_enabled ? '启用' : '未检测到'],
    ['项目 / 会话', `${dataStatus.project_count || 0} / ${dataStatus.session_count || 0}`],
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
  const overview = [
    ['运行状态', healthy ? '正常' : '待检查'],
    ['项目数量', String(data.project_count ?? setup.project_count ?? '-')],
    ['数据库大小', fmtBytes(data.database_size_bytes)],
    ['最近备份', data.latest_backup_at ? fmtRelativeAge(data.latest_backup_age_seconds) : '暂无'],
  ];
  const runtime = [
    ['访问地址', systemStatus?.addr || '-'],
    ['Web 资源', systemStatus?.web_dir ? safePathName(systemStatus.web_dir) : '内嵌'],
    ['数据库文件', safePathName(data.database_path || systemStatus?.database)],
    ['会话 / 项目', `${data.session_count ?? '-'} / ${data.project_count ?? setup.project_count ?? '-'}`],
    ['模型供应商', String((providers || []).length)],
    ['MCP Server', String((mcpStatus || []).length)],
  ];

  return <>
    <div className="settings-block-head"><label>系统状态</label><button className="secondary small" onClick={refreshStatus}>刷新状态</button></div>
    <div className={'setup-banner show ' + (healthy ? 'ok' : '')}>
      <div><b>{healthy ? 'ChatDock 运行正常' : 'ChatDock 状态待确认'}</b><div className="hint">{healthy ? '核心服务可用，配置和数据状态已加载。' : '刷新后仍异常时，可展开下方诊断信息排查。'}</div></div>
      <span className={'badge ' + (healthy ? 'ok' : 'warn')}>{healthy ? 'healthy' : 'unknown'}</span>
    </div>
    <div className="project-summary-grid">{overview.map(([label, value]) => <div className="project-summary-card" key={label}><span>{label}</span><b className="stat-value" title={value}>{value}</b></div>)}</div>
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
