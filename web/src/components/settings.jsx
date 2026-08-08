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
import { unsavedSettingsPrompt, validateMCPConfigRaw } from '../lib/settingsDraft.js';
import { ProjectsPage, ScheduledTasksPage } from './managementPages.jsx';
import { ProviderEditor, SettingsEditorPage } from './settingsEditors.jsx';

const settingsModuleMeta = {
  model: {label: '模型', desc: '选择默认模型并管理模型供应商。', icon: Bot},
  tools: {label: '工具', desc: '管理 ChatDock 内置工具和 MCP Server。', icon: Wrench},
  projects: {label: '项目', desc: '管理项目名称、提示词和会话归属。', icon: Folder},
  automation: {label: '定时任务', desc: '创建、运行和暂停自动执行任务。', icon: ListTodo},
  security: {label: '系统', desc: '查看运行状态、数据库、备份、诊断信息和访问入口。', icon: ShieldCheck},
};

function restoreFocusTo(element) {
  if (!element || !document.contains(element)) return;
  window.requestAnimationFrame(() => element.focus?.());
}

function useMountedRef() {
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => { mountedRef.current = false; };
  }, []);
  return mountedRef;
}

function modalFocusableElements(modal) {
  if (!modal) return [];
  const selector = 'button, [href], input, select, textarea, summary, [tabindex]:not([tabindex="-1"])';
  return Array.from(modal.querySelectorAll(selector)).filter(element => {
    if (element.disabled || element.getAttribute('aria-disabled') === 'true') return false;
    if (element.tabIndex < 0) return false;
    return !!(element.offsetWidth || element.offsetHeight || element.getClientRects().length);
  });
}

function usePendingActions() {
  const mountedRef = useMountedRef();
  const pendingRef = useRef({});
  const [pending, setPending] = useState({});

  const setActionPending = useCallback((key, value) => {
    const next = {...pendingRef.current};
    if (value) next[key] = true;
    else delete next[key];
    pendingRef.current = next;
    if (mountedRef.current) setPending(next);
  }, [mountedRef]);

  const runPending = useCallback(async (key, action) => {
    if (pendingRef.current[key]) return {started: false, value: undefined};
    setActionPending(key, true);
    try {
      return {started: true, value: await action()};
    } finally {
      setActionPending(key, false);
    }
  }, [setActionPending]);

  const isPending = useCallback(key => !!pendingRef.current[key], []);
  return {pending, isPending, runPending};
}

function useSettingsModalInteraction(open, modalRef, onClose, closeDisabled = false) {
  useEffect(() => {
    if (!open) return undefined;
    const frame = window.requestAnimationFrame(() => {
      const modal = modalRef.current;
      const firstFocusable = modalFocusableElements(modal)[0];
      (firstFocusable || modal)?.focus?.();
    });
    function closeOnEscape(event) {
      if (event.key === 'Tab') {
        const modal = modalRef.current;
        const focusable = modalFocusableElements(modal);
        if (!focusable.length) {
          event.preventDefault();
          modal?.focus?.();
          return;
        }
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        const active = document.activeElement;
        if (event.shiftKey && (!modal?.contains(active) || active === first)) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && active === last) {
          event.preventDefault();
          first.focus();
        }
        return;
      }
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        event.stopImmediatePropagation?.();
        if (!closeDisabled) onClose();
      }
    }
    window.addEventListener('keydown', closeOnEscape, true);
    return () => {
      window.cancelAnimationFrame(frame);
      window.removeEventListener('keydown', closeOnEscape, true);
    };
  }, [closeDisabled, modalRef, onClose, open]);
}

export function SettingsPanel(props) {
  const {
    activeModule, api, closeSettings, config, configDirty, mcpConfigDirty, dataStatus, saveModelProvider, deleteModelProvider, testSavedModelProvider, fetchSavedProviderModels,
    loadDataStatus, loadMCPStatus, loadSystemStatus, logout, builtinTools, mcpConfig, mcpStatus, onCopy, providers, projectPromptPreview, refreshProductState, refreshVisibleSettings,
    saveConfig, saveMCPConfig, setConfig, setupStatus, showDialog, showProjectPromptPreview, switchSettingsModule, systemStatus,
    testMCP, fetchMCPServerTools, testModelProvider, addCandidateModelsToProvider, loadingModels,
    projects, projectSessionCounts, saveProject, deleteProject, openProjectSessions, loadProjects, onPinnedProjectChange, startProjectConversation, showToast,
    scheduledTasks, taskSearch, setTaskSearch, saveScheduledTask, deleteScheduledTask, setScheduledTasks, toggleScheduledTask, runScheduledTaskNow, openScheduledTaskSession, loadScheduledTasks, onPinnedTaskChange,
  } = props;
  const saveTimerRef = useRef(null);
  const saveInFlightRef = useRef(false);
  const mountedRef = useMountedRef();
  const settingsPending = usePendingActions();
  const moduleTabsRef = useRef(null);
  const [saveState, setSaveState] = useState({scope: '', status: 'idle', message: ''});
  const refreshing = !!settingsPending.pending.refresh;
  useEffect(() => () => window.clearTimeout(saveTimerRef.current), []);

  const saveModelConfig = useCallback(async () => {
    if (!configDirty || !saveConfig || saveInFlightRef.current) return;
    saveInFlightRef.current = true;
    window.clearTimeout(saveTimerRef.current);
    setSaveState({scope: 'config', status: 'saving', message: ''});
    try {
      await saveConfig({silent: true});
      if (mountedRef.current) {
        setSaveState({scope: 'config', status: 'saved', message: ''});
        saveTimerRef.current = window.setTimeout(() => setSaveState({scope: '', status: 'idle', message: ''}), 2200);
      }
    } catch (error) {
      if (mountedRef.current) setSaveState({scope: 'config', status: 'error', message: error?.message || '保存失败，请稍后重试。'});
    } finally {
      saveInFlightRef.current = false;
    }
  }, [configDirty, mountedRef, saveConfig]);

  useEffect(() => {
    function saveWithKeyboard(event) {
      if (activeModule !== 'model' || !(event.metaKey || event.ctrlKey) || event.altKey || event.key.toLowerCase() !== 's') return;
      event.preventDefault();
      if (configDirty) saveModelConfig();
    }
    window.addEventListener('keydown', saveWithKeyboard);
    return () => window.removeEventListener('keydown', saveWithKeyboard);
  }, [activeModule, configDirty, saveModelConfig]);

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      window.scrollTo({top: 0, left: 0, behavior: 'auto'});
      const tabs = moduleTabsRef.current;
      const activeTab = tabs?.querySelector('.module-tab.active');
      if (!tabs || !activeTab) return;
      const left = activeTab.offsetLeft - (tabs.clientWidth - activeTab.offsetWidth) / 2;
      tabs.scrollTo({left: Math.max(0, left), behavior: 'smooth'});
    });
    return () => window.cancelAnimationFrame(frame);
  }, [activeModule]);

  const configSaveState = saveState.scope === 'config' ? saveState : {scope: 'config', status: 'idle', message: ''};
  const moduleIsDirty = (name) => name === 'model' ? !!configDirty : name === 'tools' ? !!mcpConfigDirty : false;
  const handleModuleTabsKeyDown = (event) => {
    const direction = {ArrowLeft: -1, ArrowUp: -1, ArrowRight: 1, ArrowDown: 1}[event.key];
    if (direction == null && event.key !== 'Home' && event.key !== 'End') return;
    event.preventDefault();
    const current = Math.max(0, settingsModules.indexOf(event.target?.dataset?.module || activeModule));
    const nextIndex = event.key === 'Home' ? 0 : event.key === 'End' ? settingsModules.length - 1 : (current + direction + settingsModules.length) % settingsModules.length;
    const next = settingsModules[nextIndex];
    switchSettingsModule(next);
    document.getElementById('settings-tab-' + next)?.focus();
  };
  const refreshSettings = async () => {
    await settingsPending.runPending('refresh', async () => {
      const prompt = unsavedSettingsPrompt('refresh', configDirty, mcpConfigDirty);
      if (prompt && !window.confirm(prompt)) return;
      await (refreshVisibleSettings || refreshProductState)?.();
    });
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
      <div className="settings-header-actions"><button className={'secondary small settings-refresh-button' + (refreshing ? ' refreshing' : '')} onClick={refreshSettings} disabled={refreshing} aria-busy={refreshing} aria-label={refreshing ? '正在刷新配置' : '刷新配置'} title="刷新配置"><RefreshCw className="settings-header-icon settings-refresh-icon" size={16} aria-hidden="true" /><span className="settings-refresh-text">{refreshing ? '刷新中…' : '刷新'}</span></button></div>
    </header>
    <div className="settings-sidebar">
      <select className="settings-mobile-module-select" value={activeModule} onChange={e => switchSettingsModule(e.target.value)} aria-label="选择配置模块">
        {settingsModules.map(m => <option key={m} value={m}>{moduleLabel(m)}{moduleIsDirty(m) ? ' · 未保存' : ''}</option>)}
      </select>
      <nav ref={moduleTabsRef} className="module-tabs" aria-label="配置模块" role="tablist" onKeyDown={handleModuleTabsKeyDown}>{settingsModules.map(m => {
        const dirty = moduleIsDirty(m);
        const ModuleIcon = settingsModuleMeta[m]?.icon;
        return <button key={m} type="button" id={'settings-tab-' + m} data-module={m} role="tab" aria-selected={activeModule === m} aria-controls={'settings-panel-' + m} tabIndex={activeModule === m ? 0 : -1} className={'module-tab ' + (activeModule === m ? 'active ' : '') + (dirty ? 'dirty' : '')} onClick={() => switchSettingsModule(m)}>
          <span className="module-tab-main">{ModuleIcon ? <ModuleIcon className="module-tab-icon" size={17} aria-hidden="true" /> : null}<span className="module-tab-label">{moduleLabel(m)}</span></span>
          {dirty ? <span className="module-tab-dirty" aria-label="有未保存修改">未保存</span> : null}
        </button>;
      })}</nav>
    </div>
    <main className="settings-content">
      <ModuleView name="model" activeModule={activeModule} dirty={configDirty} saveState={configSaveState} onSave={saveModelConfig}>
        <ModelModule config={config} setConfig={setConfig} projectPromptPreview={projectPromptPreview} testModelProvider={testModelProvider} providers={providers} />
        <ProvidersModule providers={providers} saveModelProvider={saveModelProvider} deleteModelProvider={deleteModelProvider} testSavedModelProvider={testSavedModelProvider} fetchSavedProviderModels={fetchSavedProviderModels} addCandidateModelsToProvider={addCandidateModelsToProvider} loadingModels={loadingModels} />
      </ModuleView>
      <ModuleView name="tools" activeModule={activeModule}><ToolsModule builtinTools={builtinTools} mcpStatus={mcpStatus} mcpConfig={mcpConfig} saveMCPConfig={saveMCPConfig} loadMCPStatus={loadMCPStatus} testMCP={testMCP} fetchMCPServerTools={fetchMCPServerTools} showDialog={showDialog} /></ModuleView>
      <ModuleView name="projects" activeModule={activeModule}>
        <ProjectsPage
          api={api}
          embedded
          projects={projects}
          projectSessionCounts={projectSessionCounts}
          saveProject={saveProject}
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
      <ModuleView name="automation" activeModule={activeModule}>
        <ScheduledTasksPage
          api={api}
          embedded
          scheduledTasks={scheduledTasks}
          taskSearch={taskSearch}
          setTaskSearch={setTaskSearch}
          saveScheduledTask={saveScheduledTask}
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
  return <div id={'settings-panel-' + name} role="tabpanel" aria-labelledby={'settings-tab-' + name} aria-hidden={activeModule !== name} className={'module-view ' + (activeModule === name ? 'active ' : '') + (dirty ? 'dirty ' : '') + (bare ? 'bare' : '')} data-module-view={name}>
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
  const pendingActions = usePendingActions();
  const update = (key, value) => setConfig(current => ({...current, [key]: value}));
  const providerModels = provider => normalizeModelNames([...(provider?.models || []), provider?.default_model].filter(Boolean));
  const activeProvider = providers.find(provider => provider.id === config.provider_id) || providers[0] || null;
  const fallbackProvider = providers.find(provider => provider.id === config.fallback_provider_id) || null;
  const fallbackEnabled = !!fallbackProvider;
  const chooseProvider = id => setConfig(current => {
    const provider = providers.find(item => item.id === id) || providers[0] || null;
    const models = providerModels(provider);
    const model = models.includes(current.model) ? current.model : (provider?.default_model || models[0] || current.model || '');
    return {...current, provider_id: provider?.id || '', base_url: provider?.base_url || '', has_api_key: !!provider?.has_api_key, model, models};
  });
  const chooseModel = name => setConfig(current => ({...current, model: name, models: normalizeModelNames([...(current.models || []), name])}));
  const chooseFallbackProvider = id => setConfig(current => {
    const provider = providers.find(item => item.id === id) || null;
    if (!provider) return {...current, fallback_provider_id: '', fallback_model: ''};
    const models = providerModels(provider);
    return {...current, fallback_provider_id: provider.id, fallback_model: models.includes(current.fallback_model) ? current.fallback_model : (provider.default_model || models[0] || '')};
  });
  const selectedProviderModels = normalizeModelNames([...providerModels(activeProvider), config.model].filter(Boolean));
  const fallbackModels = normalizeModelNames([...providerModels(fallbackProvider), config.fallback_model].filter(Boolean));
  const contextMode = config.context_mode || 'auto';
  const testingModel = !!pendingActions.pending.testModel;
  const testDefaultModel = async () => {
    if (!testModelProvider) return;
    await pendingActions.runPending('testModel', testModelProvider);
  };
  return <>
    <section className="settings-section model-routing-section">
      <div className="settings-section-head"><div><b>模型路由</b><p>默认模型和备用模型属于同一条调用链。</p></div><button className="secondary small" onClick={testDefaultModel} disabled={testingModel || !activeProvider} aria-busy={testingModel}>{testingModel ? '测试中…' : '测试连接'}</button></div>
      <div className="settings-editor-grid model-route-grid">
        <label>默认供应商<select value={activeProvider?.id || ''} onChange={event => chooseProvider(event.target.value)}>{providers.length ? providers.map(provider => <option key={provider.id} value={provider.id}>{provider.name || provider.id}</option>) : <option value="">未配置供应商</option>}</select></label>
        <label>默认模型<select value={config.model || ''} onChange={event => chooseModel(event.target.value)} disabled={!activeProvider}><option value="">{activeProvider ? '选择模型' : '先选择供应商'}</option>{selectedProviderModels.map(name => <option key={name} value={name}>{name}</option>)}</select></label>
      </div>
      <label className="settings-editor-check model-fallback-toggle"><input type="checkbox" checked={fallbackEnabled} onChange={event => event.target.checked ? chooseFallbackProvider(providers.find(provider => provider.id !== activeProvider?.id)?.id || providers[0]?.id || '') : chooseFallbackProvider('')} /><span><b>启用备用模型</b><small>仅在主模型尚未输出内容或执行工具前失败时切换。</small></span></label>
      {fallbackEnabled ? <div className="settings-editor-grid model-route-grid fallback-model-grid">
        <label>备用供应商<select value={fallbackProvider?.id || ''} onChange={event => chooseFallbackProvider(event.target.value)}>{providers.map(provider => <option key={provider.id} value={provider.id}>{provider.name || provider.id}</option>)}</select></label>
        <label>备用模型<select value={config.fallback_model || ''} onChange={event => update('fallback_model', event.target.value)}><option value="">使用供应商默认模型</option>{fallbackModels.map(name => <option key={name} value={name}>{name}</option>)}</select></label>
      </div> : null}
      {!selectedProviderModels.length ? <div className="hint">当前供应商没有可用模型，请在下方编辑供应商。</div> : null}
    </section>
    <section className="settings-section model-response-section">
      <div className="settings-section-head"><div><b>回复设置</b><p>高频参数直接展示，不再嵌套展开。</p></div></div>
      <div className="settings-editor-grid">
        <label>上下文<select value={contextMode} onChange={event => update('context_mode', event.target.value)}><option value="auto">自动</option><option value="compact">精简</option><option value="expanded">更多历史</option><option value="custom">自定义</option></select></label>
        <label>Temperature<input type="number" step="0.1" min="0" max="2" value={config.temperature} onChange={event => update('temperature', event.target.value)} /></label>
      </div>
      {contextMode === 'custom' ? <label>最近消息数<input type="number" min="1" max="200" value={config.max_context_messages} onChange={event => update('max_context_messages', event.target.value)} /></label> : null}
      <label>全局系统提示词<textarea className="system-prompt-editor" rows="9" value={config.system_prompt} onChange={event => update('system_prompt', event.target.value)} /></label>
      <label className="settings-editor-check"><input type="checkbox" checked={!!config.hide_thinking} onChange={event => update('hide_thinking', event.target.checked)} /><span><b>隐藏模型思考内容</b><small>只影响界面显示，不改变模型推理能力。</small></span></label>
      {projectPromptPreview ? <pre className="settings-editor-preview">{projectPromptPreview}</pre> : null}
    </section>
    <section className="settings-section model-search-section">
      <div className="settings-section-head"><div><b>工具搜索</b><p>可选向量服务；留空时自动使用关键词搜索。</p></div></div>
      <div className="settings-editor-grid">
        <label>Base URL<input value={config.embedding_base_url || ''} onChange={event => update('embedding_base_url', event.target.value)} placeholder="http://127.0.0.1:8000/v1" /></label>
        <label>模型<input value={config.embedding_model || 'BAAI/bge-m3'} onChange={event => update('embedding_model', event.target.value)} placeholder="BAAI/bge-m3" /></label>
      </div>
      <label>API Key<input type="password" value={config.embedding_api_key || ''} onChange={event => update('embedding_api_key', event.target.value)} placeholder={config.has_embedding_api_key ? '已保存，留空不修改' : '可留空'} /></label>
    </section>
  </>;
}

function ProvidersModule({ providers, saveModelProvider, deleteModelProvider, testSavedModelProvider, fetchSavedProviderModels, addCandidateModelsToProvider, loadingModels }) {
  const [editingProvider, setEditingProvider] = useState(undefined);
  const [candidatePicker, setCandidatePicker] = useState(null);
  const pendingActions = usePendingActions();
  const candidateModalRef = useRef(null);
  const candidateReturnFocusRef = useRef(null);
  const candidateSaving = !!pendingActions.pending.candidateSave;
  const discoveringModels = !!pendingActions.pending.discoverModels;
  const providerTestPending = Object.keys(pendingActions.pending).some(key => key.startsWith('providerTest:'));

  const closeCandidatePicker = useCallback(() => {
    setCandidatePicker(null);
    restoreFocusTo(candidateReturnFocusRef.current);
  }, []);
  useSettingsModalInteraction(!!candidatePicker, candidateModalRef, closeCandidatePicker, candidateSaving);

  async function discoverModels(provider, trigger) {
    candidateReturnFocusRef.current = trigger || document.activeElement;
    const result = await pendingActions.runPending('discoverModels', () => fetchSavedProviderModels?.(provider));
    if (!result.started || !result.value) return;
    setCandidatePicker({provider, models: result.value, selected: [], query: ''});
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
    const result = await pendingActions.runPending('candidateSave', () => addCandidateModelsToProvider?.(candidatePicker.provider.id, candidatePicker.selected));
    if (result.started && result.value !== false) closeCandidatePicker();
  }
  async function testSavedProvider(provider) {
    if (!provider?.id || providerTestPending) return;
    await pendingActions.runPending('providerTest:' + provider.id, () => testSavedModelProvider?.(provider));
  }

  if (editingProvider !== undefined) return <ProviderEditor
    provider={editingProvider}
    onBack={() => setEditingProvider(undefined)}
    onSave={saveModelProvider}
    onDelete={async provider => { await deleteModelProvider(provider); setEditingProvider(undefined); }}
    onTest={testSavedProvider}
  />;

  const existingModels = normalizeModelNames([...(candidatePicker?.provider?.models || []), candidatePicker?.provider?.default_model].filter(Boolean));
  const candidateQuery = String(candidatePicker?.query || '').trim().toLowerCase();
  const visibleCandidateModels = candidatePicker?.models.filter(name => !candidateQuery || String(name).toLowerCase().includes(candidateQuery)) || [];
  return <section className="settings-section provider-management-section">
    <div className="settings-section-head"><div><b>模型供应商</b><p>独立资源进入二级编辑页；发现模型保留临时选择弹窗。</p></div><button className="secondary small" onClick={() => setEditingProvider(null)}>新增供应商</button></div>
    <div className="provider-grid provider-page-grid">{providers.length ? providers.map(provider => {
      const testingThisProvider = !!pendingActions.pending['providerTest:' + provider.id];
      return <TextCard key={provider.id} title={provider.name || provider.id} hint={provider.base_url || '-'} badge={provider.enabled ? (provider.type || 'openai') : '停用'}><div className="product-meta">默认：{provider.default_model || '-'} · 模型 {provider.models?.length || 0} · Key {provider.api_keys?.length || (provider.has_api_key ? 1 : 0)}</div><div className="product-actions"><button className="secondary small" onClick={() => setEditingProvider(provider)}>编辑</button><button className="secondary small" onClick={event => discoverModels(provider, event.currentTarget)} disabled={loadingModels || discoveringModels}>{loadingModels || discoveringModels ? '读取中…' : '发现模型'}</button><button className="secondary small" onClick={() => testSavedProvider(provider)} disabled={providerTestPending} aria-busy={testingThisProvider}>{testingThisProvider ? '测试中…' : '测试'}</button><button className="danger small" onClick={() => deleteModelProvider(provider)}>删除</button></div></TextCard>;
    }) : <div className="empty compact">还没有模型供应商。</div>}</div>
    {candidatePicker ? <div className="app-modal-backdrop show" onClick={event => { if (event.target === event.currentTarget && !candidateSaving) closeCandidatePicker(); }}>
      <div ref={candidateModalRef} className="app-modal-card candidate-model-modal" role="dialog" aria-modal="true" aria-labelledby="candidateModelTitle" tabIndex="-1">
        <div className="app-modal-form">
          <div id="candidateModelTitle" className="app-modal-title">发现模型 · {candidatePicker.provider.name || candidatePicker.provider.id}</div>
          <div className="candidate-model-modal-body">
            <p>选择要加入该供应商的模型，最后统一保存。</p>
            <div className="candidate-model-filter"><input aria-label="搜索候选模型" autoComplete="off" placeholder="搜索模型名称" value={candidatePicker.query || ''} onChange={event => setCandidatePicker(current => current ? {...current, query: event.target.value} : current)} /><span role="status" aria-live="polite">显示 {visibleCandidateModels.length} / {candidatePicker.models.length} 个{candidatePicker.selected.length ? ` · 已选 ${candidatePicker.selected.length} 个` : ''}</span></div>
            <div className="model-options candidate-model-options">{visibleCandidateModels.map(name => {
              const existing = existingModels.includes(name);
              const selected = candidatePicker.selected.includes(name);
              return <button key={name} type="button" className={'model-option candidate ' + (existing ? 'added ' : '') + (selected ? 'active' : '')} onClick={() => toggleCandidate(name)} disabled={existing}>{existing ? '已存在 · ' : selected ? '已选择 · ' : ''}{name}</button>;
            })}</div>
            {!visibleCandidateModels.length ? <div className="empty compact">没有匹配的模型。</div> : null}
          </div>
          <div className="app-modal-actions"><button type="button" className="secondary" onClick={closeCandidatePicker} disabled={candidateSaving}>取消</button><button type="button" onClick={saveCandidates} disabled={candidateSaving || !candidatePicker.selected.length} aria-busy={candidateSaving}>{candidateSaving ? '保存中…' : '保存所选模型'}{candidatePicker.selected.length ? `（${candidatePicker.selected.length}）` : ''}</button></div>
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
  return <div className="mcp-tool-picker builtin-tool-picker">
    <div className="builtin-tool-picker-summary"><span><b>单工具覆盖</b><small>只调整例外工具，其他工具继承默认加载方式。</small></span><em>{options.length} 个</em></div>
    <div className="mcp-tool-policy-list">{options.map(tool => <div className="mcp-tool-policy-row builtin-tool-exposure-row" key={tool.value}>
      <div className="mcp-tool-policy-name"><b>{tool.title || tool.name}</b><span>{tool.value}</span></div>
      <select className="mcp-tool-exposure-select" aria-label={(tool.title || tool.name) + ' 加载方式'} value={toolExposureForValue(tool.value, draft)} onChange={event => onChange(setToolExposureDraft(draft, tool.value, event.target.value))}><option value="inherit">跟随默认</option><option value="direct">直接加载</option><option value="on_demand">按需加载</option></select>
    </div>)}</div>
  </div>;
}


function ToolsModule({ builtinTools, mcpStatus, mcpConfig, saveMCPConfig, loadMCPStatus, testMCP, fetchMCPServerTools, showDialog }) {
  const [editor, setEditor] = useState(null);
  const [formError, setFormError] = useState('');
  const [serverTools, setServerTools] = useState({});
  const pendingActions = usePendingActions();
  const mountedRef = useMountedRef();
  const editorReturnFocusRef = useRef(null);
  const parsed = useMemo(() => parseMCPConfigDraft(mcpConfig), [mcpConfig]);
  const configEditable = !parsed.error;
  const serverNames = Object.keys(parsed.config.servers || {}).sort();
  const statusByName = useMemo(() => Object.fromEntries((mcpStatus || []).map(status => [status.name, status])), [mcpStatus]);
  const saving = !!pendingActions.pending.mcpSave || !!pendingActions.pending.mcpDelete;
  const checkingStatus = !!pendingActions.pending.mcpStatus;
  const testingServerName = Object.keys(pendingActions.pending).find(key => key.startsWith('mcpTest:'))?.slice('mcpTest:'.length) || '';

  const closeEditor = useCallback(() => {
    setEditor(null);
    setFormError('');
    restoreFocusTo(editorReturnFocusRef.current);
  }, []);
  function rememberEditorTrigger(trigger) { editorReturnFocusRef.current = trigger || document.activeElement; }
  function openBuiltinEditor(trigger) {
    if (!configEditable) { setFormError('当前 MCP 配置不是合法 JSON，请先修复配置后再编辑工具资源。'); return; }
    rememberEditorTrigger(trigger); setFormError('');
    setEditor({kind: 'builtin', name: 'chatdock', draft: builtinToolsToDraft(parsed.config.builtin_tools)});
  }
  function openServerEditor(name, trigger) {
    if (!configEditable) { setFormError('当前 MCP 配置不是合法 JSON，请先修复配置后再编辑工具资源。'); return; }
    rememberEditorTrigger(trigger); setFormError('');
    setEditor({kind: 'server', name, draft: {name, ...mcpServerToDraft(parsed.config.servers[name])}});
  }
  function openNewServerEditor(trigger) {
    if (!configEditable) { setFormError('当前 MCP 配置不是合法 JSON，请先修复配置后再新增 Server。'); return; }
    rememberEditorTrigger(trigger); setFormError('');
    setEditor({kind: 'new', name: '', draft: defaultMCPServerDraft()});
  }
  function openRawConfigEditor(trigger) {
    rememberEditorTrigger(trigger); setFormError('');
    setEditor({kind: 'raw', name: 'mcp-json', draft: {content: mcpConfig || ''}});
  }
  function updateEditor(patch) { setEditor(current => current ? {...current, draft: {...current.draft, ...patch}} : current); }
  async function checkMCPStatus() {
    if (!loadMCPStatus) return;
    await pendingActions.runPending('mcpStatus', async () => {
      setFormError('');
      try { await loadMCPStatus(); }
      catch (error) { if (mountedRef.current) setFormError('检查 MCP 连接失败：' + (error.message || '未知错误')); }
    });
  }
  async function testSavedMCPServer(name) {
    if (!name || testingServerName) return;
    await pendingActions.runPending('mcpTest:' + name, () => testMCP?.(name));
  }
  async function refreshServerTools(name) {
    if (!name || !fetchMCPServerTools) return;
    await pendingActions.runPending('mcpTools:' + name, async () => {
      try {
        const tools = await fetchMCPServerTools(name);
        if (mountedRef.current) {
          setServerTools(previous => ({...previous, [name]: tools || []}));
          setFormError('已读取 ' + name + ' 的 ' + ((tools || []).length) + ' 个工具。');
        }
      } catch (error) {
        if (mountedRef.current) setFormError('读取工具列表失败：' + error.message);
      }
    });
  }
  async function saveEditor() {
    if (!editor || !saveMCPConfig) return;
    if (editor.kind === 'raw') {
      const validation = validateMCPConfigRaw(editor.draft.content);
      if (!validation.ok) { setFormError(validation.error); return; }
      await pendingActions.runPending('mcpSave', async () => {
        setFormError('');
        try { await saveMCPConfig({content: validation.content, silent: true}); if (mountedRef.current) closeEditor(); }
        catch (error) { if (mountedRef.current) setFormError(error.message || '保存 MCP JSON 失败。'); }
      });
      return;
    }
    if (!configEditable) { setFormError('当前 MCP 配置不是合法 JSON，请先用原文修复后再保存。'); return; }
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
    await pendingActions.runPending('mcpSave', async () => {
      setFormError('');
      try { await saveMCPConfig({content: stringifyMCPConfigDraft(next), silent: true}); if (mountedRef.current) closeEditor(); }
      catch (error) { if (mountedRef.current) setFormError(error.message || '保存工具配置失败。'); }
    });
  }
  async function deleteCurrentServer() {
    if (editor?.kind !== 'server') return;
    const confirmed = await showDialog?.({title: '删除 MCP Server', message: '确定删除 MCP Server「' + editor.name + '」？', confirmText: '删除 Server', danger: true, type: 'confirm'});
    if (!confirmed) return;
    await pendingActions.runPending('mcpDelete', async () => {
      const next = {...parsed.config, servers: {...(parsed.config.servers || {})}};
      delete next.servers[editor.name];
      try { await saveMCPConfig({content: stringifyMCPConfigDraft(next), silent: true}); if (mountedRef.current) closeEditor(); }
      catch (error) { if (mountedRef.current) setFormError(error.message || '删除 MCP Server 失败。'); }
    });
  }

  if (editor) {
    const isBuiltin = editor.kind === 'builtin';
    const isNew = editor.kind === 'new';
    const isRaw = editor.kind === 'raw';
    const serverDraft = !isBuiltin && !isRaw ? editor.draft : null;
    const editorServerName = editor.kind === 'server' ? editor.name : '';
    const urlHasLocalhost = serverDraft ? /^http:\/\/(127\.0\.0\.1|localhost)(?=[:/]|$)/i.test(normalizeMCPURLDraft(serverDraft.url)) : false;
    const urlHasSpace = serverDraft ? /\s/.test(String(serverDraft.url || '')) : false;
    const tokenEnvLooksLikeHeader = serverDraft ? String(serverDraft.token_env || '').trim().toLowerCase() === 'authorization' : false;
    const hasSavedToken = serverDraft ? !!String(serverDraft.saved_token_ref || '').trim() : false;
    const tokenExpiry = serverDraft ? mcpTokenExpiryState(serverDraft.token) : null;
    const title = isRaw ? '修复 MCP JSON' : isBuiltin ? 'ChatDock 内置工具' : isNew ? '新增 MCP Server' : '编辑 ' + editor.name;
    const actions = <><div className="settings-editor-secondary-actions">{editor.kind === 'server' ? <><button type="button" className="danger" onClick={deleteCurrentServer} disabled={saving}>删除 Server</button><button type="button" className="secondary" onClick={() => testSavedMCPServer(editor.name)} disabled={saving || !!testingServerName} aria-busy={testingServerName === editor.name}>{testingServerName === editor.name ? '测试中…' : '测试已保存配置'}</button></> : null}</div><div className="settings-editor-primary-actions"><button type="button" className="secondary" onClick={closeEditor} disabled={saving}>取消</button><button type="button" onClick={saveEditor} disabled={saving}>{saving ? '保存中…' : isRaw ? '保存 JSON' : isBuiltin ? '保存工具设置' : isNew ? '保存 Server' : '保存修改'}</button></div></>;
    return <SettingsEditorPage eyebrow="工具资源" title={title} description={isBuiltin ? '设置内置工具默认加载方式和单工具例外。' : isRaw ? '仅用于修复损坏配置，保存前会校验 JSON。' : '连接、认证、加载策略和工具权限在同一页维护。'} onBack={closeEditor} actions={actions}>
      <div className="settings-editor-form mcp-settings-editor">
        {isRaw ? <section className="settings-editor-section"><label>MCP JSON 原文<textarea rows="22" value={editor.draft.content} onChange={event => updateEditor({content: event.target.value})} spellCheck="false" /></label><div className="hint">这里只保存你编辑后的原文，不会自动重置为空配置。</div></section> : null}
        {isBuiltin ? <section className="settings-editor-section builtin-tools-section"><div className="settings-editor-section-head"><div><b>加载策略</b><p>未单独设置的工具继承默认方式。</p></div></div><div className="builtin-default-exposure"><div><b>默认加载方式</b><span>直接加载适合高频工具，按需加载减少上下文体积。</span></div><select aria-label="内置工具默认加载方式" value={editor.draft.tool_exposure} onChange={event => updateEditor({tool_exposure: event.target.value})}><option value="direct">直接加载</option><option value="on_demand">按需加载</option></select></div><BuiltinToolExposurePicker draft={editor.draft} tools={builtinTools || []} onChange={updateEditor} /></section> : null}
        {serverDraft ? <>
          <section className="settings-editor-section"><div className="settings-editor-section-head"><div><b>基本信息</b><p>资源说明会进入模型看到的轻量索引。</p></div></div><div className="settings-editor-grid"><label>Server 名称<input value={serverDraft.name} onChange={event => updateEditor({name: event.target.value})} placeholder="例如 DockMini" /></label><label>状态<select value={serverDraft.disabled ? 'disabled' : 'enabled'} onChange={event => updateEditor({disabled: event.target.value === 'disabled'})}><option value="enabled">启用</option><option value="disabled">禁用</option></select></label></div><label>资源说明<input maxLength="240" value={serverDraft.description} onChange={event => updateEditor({description: event.target.value})} placeholder="例如 Mac mini 本机开发、文件、命令和 Git 能力" /></label><label>MCP HTTP 地址<input value={serverDraft.url} onChange={event => updateEditor({url: event.target.value})} placeholder="http://host.docker.internal:8765/mcp" /></label>{urlHasLocalhost || urlHasSpace ? <div className="mcp-inline-warning"><b>当前地址可能连不上</b><span>{urlHasSpace ? 'URL 里有空格；' : ''}{urlHasLocalhost ? 'Docker 内不能用 127.0.0.1 访问宿主机。' : ''}</span><button className="secondary small" onClick={() => updateEditor({url: dockerHostMCPURL(serverDraft.url)})}>改成 Docker 宿主机地址</button></div> : null}</section>
          <section className="settings-editor-section"><div className="settings-editor-section-head"><div><b>认证与连接</b><p>Token、超时和缓存属于连接设置，不再藏在嵌套展开中。</p></div></div><div className="settings-editor-grid"><label>连接类型<select value={serverDraft.type} onChange={event => updateEditor({type: event.target.value})}><option value="streamable-http">HTTP / Streamable HTTP</option></select></label><label>认证方式<select value={serverDraft.auth_type} onChange={event => updateEditor({auth_type: event.target.value})}><option value="none">无</option><option value="bearer">Bearer Token</option></select></label><label>Token 环境变量名（可选）<input value={serverDraft.token_env} onChange={event => updateEditor({token_env: event.target.value})} placeholder="例如 AGENTDOCK_MCP_TOKEN" /></label><label>本地路径备注<input value={serverDraft.path} onChange={event => updateEditor({path: event.target.value})} placeholder="可选备注" /></label></div>{tokenEnvLooksLikeHeader ? <div className="mcp-inline-warning"><b>这里不要填 Authorization</b><span>这里需要环境变量名，不是 HTTP Header 名。</span><button className="secondary small" onClick={() => updateEditor({token_env: ''})}>清空</button></div> : null}{hasSavedToken ? <div className="mcp-inline-warning ok mcp-token-state"><b>已保存 Token</b><span>留空不会修改；输入新值才会替换。</span><button type="button" className="secondary small" onClick={() => updateEditor({token: '', saved_token_ref: ''})}>清除 Token</button></div> : null}{serverDraft.auth_type !== 'none' ? <label>{hasSavedToken ? '替换 Token（可选）' : 'Token'}<input type="password" autoComplete="new-password" value={serverDraft.token} onChange={event => updateEditor({token: event.target.value})} placeholder={hasSavedToken ? '留空保持已保存 Token' : '粘贴 Bearer Token'} /></label> : null}{serverDraft.auth_type !== 'none' && tokenExpiry ? <div className={'mcp-inline-warning ' + (tokenExpiry.expired ? 'danger' : 'ok')}><b>{tokenExpiry.expired ? 'Token 已过期' : '新 Token 有效期'}</b><span>{tokenExpiry.text}</span></div> : null}<div className="settings-editor-grid"><label>超时 ms<input type="number" inputMode="numeric" value={serverDraft.timeout_ms} onChange={event => updateEditor({timeout_ms: event.target.value})} placeholder="30000" /></label><label>工具缓存 ms<input type="number" inputMode="numeric" value={serverDraft.cache_ttl_ms} onChange={event => updateEditor({cache_ttl_ms: event.target.value})} placeholder="可留空" /></label></div></section>
          <section className="settings-editor-section"><div className="settings-editor-section-head"><div><b>工具加载与权限</b><p>默认加载方式和单工具权限在同一处设置。</p></div></div><label>工具默认加载<select value={serverDraft.tool_exposure} onChange={event => updateEditor({tool_exposure: event.target.value})}><option value="on_demand">按需加载</option><option value="direct">直接加载</option></select></label>{!isNew ? <MCPToolPolicyPicker name={editorServerName} draft={serverDraft} tools={serverTools[editorServerName] || []} loading={!!pendingActions.pending['mcpTools:' + editorServerName]} onRefresh={() => refreshServerTools(editorServerName)} onChange={updateEditor} /> : <div className="hint">保存 Server 后可读取并配置工具权限。</div>}</section>
        </> : null}
        {formError ? <div className="backup-health warn" role="alert">{formError}</div> : null}
      </div>
    </SettingsEditorPage>;
  }

  return <>
    <section className="settings-section mcp-overview-section mcp-directory-section">
      <div className="settings-section-head"><div><b>工具资源</b><p>独立资源进入二级编辑页；连接检查在列表页完成。</p></div><div className="mcp-directory-actions"><button type="button" className="secondary small" onClick={checkMCPStatus} disabled={checkingStatus} aria-busy={checkingStatus}>{checkingStatus ? '检查中…' : '检查连接'}</button><button type="button" className="small" onClick={event => openNewServerEditor(event.currentTarget)} disabled={!configEditable}>新增 Server</button></div></div>
      <div className="mcp-config-directory"><button type="button" className="secondary mcp-config-directory-row" onClick={event => openBuiltinEditor(event.currentTarget)} disabled={!configEditable}><span><b>ChatDock 内置工具</b><small>定时任务、图片和供应商工具</small></span><em>{(builtinTools || []).length} 个 · 编辑</em></button>{serverNames.map(name => { const server = parsed.config.servers[name]; const status = statusByName[name]; return <button type="button" className="secondary mcp-config-directory-row" key={name} onClick={event => openServerEditor(name, event.currentTarget)} disabled={!configEditable}><span><b>{name}</b><small>{server.url || '未填写 HTTP 地址'}</small></span><em className={'badge ' + runStatusClass(status?.last_status || 'unknown')}>{status?.last_status ? runStatusLabel(status.last_status) : '未检测'}</em></button>; })}{!serverNames.length ? <div className="empty compact">暂无 MCP Server。</div> : null}</div>
    </section>
    {parsed.error ? <div className="backup-health warn">当前配置 JSON 损坏：{parsed.error}<button type="button" className="secondary small" onClick={event => openRawConfigEditor(event.currentTarget)}>修复 JSON 原文</button></div> : null}
    {formError ? <div className="backup-health warn">{formError}</div> : null}
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
    {(dataStatus.backup_checked_dirs || []).length ? <div className="backup-path-list"><div className="settings-block-head"><label>已检查备份目录</label></div>{dataStatus.backup_checked_dirs.map(dir => <div className="backup-path-row" key={dir}><code>{dir}</code><button className="secondary mini" onClick={() => onCopy?.(dir)}>复制</button></div>)}</div> : null}
    {backups.length ? <div className="backup-list"><div className="settings-block-head"><label>最近数据库备份</label></div>{backups.map(item => <div className="backup-item" key={item.path || item.name}><div className="backup-main"><div><b>{item.name || safePathName(item.path)}</b><div className="hint">{fmtTime(item.updated_at)} · {fmtRelativeAge(item.age_seconds)} · {fmtBytes(item.size_bytes)}</div></div>{item.path ? <button className="secondary mini" onClick={() => onCopy?.(item.path)}>复制路径</button> : null}</div>{item.path ? <code className="backup-path-value">{item.path}</code> : null}</div>)}</div> : null}
  </>;
}

function SecurityModule({ systemStatus, setupStatus, dataStatus, mcpStatus, providers, loadSystemStatus, loadDataStatus, logout, onCopy }) {
  const pendingActions = usePendingActions();
  const refreshingStatus = !!pendingActions.pending.securityRefresh;
  const text = diagnosticsText({setupStatus, systemStatus, dataStatus, mcpStatus, providers});
  const refreshStatus = async () => {
    await pendingActions.runPending('securityRefresh', async () => {
      await Promise.allSettled([loadSystemStatus?.(), loadDataStatus?.()]);
    });
  };
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
    <div className="settings-block-head"><label>系统状态</label><button className="secondary small" onClick={refreshStatus} disabled={refreshingStatus} aria-busy={refreshingStatus}>{refreshingStatus ? '刷新中…' : '刷新状态'}</button></div>
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
