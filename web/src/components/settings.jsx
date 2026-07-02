// Configuration-center modules: workspace, model, skills, MCP tools, runs, automation, data, and security.
import React, { useMemo, useState } from 'react';
import { TextCard } from './base.jsx';
import { settingsModules, diagnosticsText, fmtBytes, fmtDuration, fmtRelativeAge, fmtTime, runStatusClass, runStatusLabel, safePathName, scheduleSummary, taskStatusClass, taskStatusLabel } from '../lib/appUtils.js';

export function SettingsPanel(props) {
  const {
    activeModule, busy, closeSettings, config, continueAgentTask, createWorkspace, dataStatus, deleteScheduledTask, deleteSkill, deleteWorkspace,
    editScheduledTask, editSkill, loadAgentTasks, loadDataStatus, loadMCPConfig, loadMCPStatus, loadRuns, loadScheduledTasks, loadSkills,
    loadSystemStatus, logout, mcpConfig, mcpStatus, onCopy, providers, promptPreview, refreshProductState, refreshVisibleSettings, runScheduledTaskNow, runSetupWizard,
    runs, saveConfig, saveMCPConfig, scheduledTasks, selectWorkspace, setConfig, setMcpConfig, setSkillSearch, setTaskSearch, setupStatus,
    showPromptPreview, skillSearch, skills, switchSettingsModule, systemStatus, taskSearch, testMCP, testModelProvider, fetchProviderModels, availableModels, loadingModels, toggleScheduledTask,
    toggleSkill, workspaces, agentTasks,
  } = props;
  const filteredSkills = useMemo(() => {
    const q = skillSearch.trim().toLowerCase();
    return q ? skills.filter(s => [s.name, s.description, s.content].some(v => String(v || '').toLowerCase().includes(q))) : skills;
  }, [skillSearch, skills]);
  const filteredTasks = useMemo(() => {
    const q = taskSearch.trim().toLowerCase();
    return q ? scheduledTasks.filter(t => [t.title, t.prompt, t.schedule_type, t.last_status, t.last_error].some(v => String(v || '').toLowerCase().includes(q))) : scheduledTasks;
  }, [scheduledTasks, taskSearch]);
  return <section className="settings">
    <div className="settings-header"><div><h2>配置中心</h2><p>工作空间、模型、技能、工具和数据状态统一管理。</p></div><div className="settings-header-actions"><button className="secondary small" onClick={() => closeSettings()}>返回对话</button><button className="secondary small" onClick={refreshVisibleSettings || refreshProductState}>刷新</button></div></div>
    <div className="module-tabs">{settingsModules.map(m => <button key={m} className={'module-tab ' + (activeModule === m ? 'active' : '')} onClick={() => switchSettingsModule(m)}>{moduleLabel(m)}</button>)}</div>
    <ModuleView name="workspace" activeModule={activeModule}><WorkspaceModule setupStatus={setupStatus} workspaces={workspaces} createWorkspace={createWorkspace} selectWorkspace={selectWorkspace} deleteWorkspace={deleteWorkspace} runSetupWizard={runSetupWizard} /></ModuleView>
    <ModuleView name="model" activeModule={activeModule}><ModelModule config={config} setConfig={setConfig} saveConfig={saveConfig} showPromptPreview={showPromptPreview} promptPreview={promptPreview} testModelProvider={testModelProvider} fetchProviderModels={fetchProviderModels} availableModels={availableModels} loadingModels={loadingModels} providers={providers} /></ModuleView>
    <ModuleView name="skills" activeModule={activeModule}><div className="settings-block-head"><label>技能库（当前工作空间）</label><button className="secondary small" onClick={() => editSkill()}>新增技能</button></div><input className="session-search" placeholder="搜索技能" value={skillSearch} onChange={e => setSkillSearch(e.target.value)} /><div className="skills-list">{filteredSkills.length ? filteredSkills.map(s => <SkillCard key={s.id} skill={s} editSkill={editSkill} deleteSkill={deleteSkill} toggleSkill={toggleSkill} />) : <div className="hint">暂无技能。技能会作为当前工作空间的补充系统指令注入模型请求。</div>}</div><div className="settings-actions"><button className="secondary" onClick={loadSkills}>刷新技能</button></div></ModuleView>
    <ModuleView name="tools" activeModule={activeModule}><ToolsModule mcpStatus={mcpStatus} mcpConfig={mcpConfig} setMcpConfig={setMcpConfig} saveMCPConfig={saveMCPConfig} loadMCPConfig={loadMCPConfig} loadMCPStatus={loadMCPStatus} testMCP={testMCP} /></ModuleView>
    <ModuleView name="runs" activeModule={activeModule}><div className="settings-block-head"><label>MCP 执行记录</label><button className="secondary small" onClick={loadRuns}>刷新</button></div>{runs.length ? runs.map(r => <RunCard key={r.id} run={r} />) : <div className="empty compact">还没有 MCP 执行记录。</div>}</ModuleView>
    <ModuleView name="agent" activeModule={activeModule}><div className="settings-block-head"><label>AgentDock 任务</label><button className="secondary small" onClick={loadAgentTasks}>刷新</button></div>{agentTasks.length ? agentTasks.map(t => <AgentTaskCard key={t.id} task={t} continueAgentTask={continueAgentTask} />) : <div className="empty compact">还没有 AgentDock 任务记录。</div>}</ModuleView>
    <ModuleView name="automation" activeModule={activeModule}><div className="settings-block-head"><label>自动化任务（当前工作空间）</label><button className="secondary small" onClick={() => editScheduledTask()}>新增任务</button></div><input className="session-search" placeholder="搜索任务" value={taskSearch} onChange={e => setTaskSearch(e.target.value)} /><div className="tasks-list">{filteredTasks.length ? filteredTasks.map(t => <TaskCard key={t.id} task={t} editScheduledTask={editScheduledTask} deleteScheduledTask={deleteScheduledTask} toggleScheduledTask={toggleScheduledTask} runScheduledTaskNow={runScheduledTaskNow} />) : <div className="hint">暂无定时任务。任务会按当前工作空间运行，并把结果写入专属会话。</div>}</div><div className="settings-actions"><button className="secondary" onClick={loadScheduledTasks}>刷新任务</button></div></ModuleView>
    <ModuleView name="data" activeModule={activeModule}><div className="settings-block-head"><label>数据状态</label><button className="secondary small" onClick={loadDataStatus}>刷新数据状态</button></div><DataStatus dataStatus={dataStatus} onCopy={onCopy} /></ModuleView>
    <ModuleView name="security" activeModule={activeModule}><SecurityModule systemStatus={systemStatus} setupStatus={setupStatus} dataStatus={dataStatus} mcpStatus={mcpStatus} providers={providers} loadSystemStatus={loadSystemStatus} logout={logout} onCopy={onCopy} /></ModuleView>
  </section>;
}

function moduleLabel(m) {
  return ({workspace:'工作空间', model:'模型', skills:'技能库', tools:'工具中心', runs:'执行记录', agent:'Agent 任务', automation:'自动化', data:'数据', security:'安全'})[m] || m;
}
function ModuleView({ name, activeModule, children }) { return <div className={'module-view ' + (activeModule === name ? 'active' : '')} data-module-view={name}>{children}</div>; }

function WorkspaceModule({ setupStatus, workspaces, createWorkspace, selectWorkspace, deleteWorkspace, runSetupWizard }) {
  return <>
    <div className={'setup-banner show ' + (setupStatus && !setupStatus.needs_setup ? 'ok' : '')}>{setupStatus?.needs_setup ? <><div><b>首次配置未完成</b><div className="hint">请配置模型供应商和默认工作空间，完成后即可开始对话。</div></div><button className="small" onClick={runSetupWizard}>开始引导</button></> : <div><b>系统已就绪</b><div className="hint">当前工作空间：{setupStatus?.active_workspace || '-'} · 数据目录：{setupStatus?.data_dir || '-'}</div></div>}</div>
    <div className="settings-block-head"><label>工作空间概览</label><button className="secondary small" onClick={createWorkspace}>新增工作空间</button></div>
    <div id="workspaceCards">{workspaces.length ? workspaces.map(ws => <TextCard key={ws.id || ws.name} title={ws.name} hint={ws.description || ''} badge={ws.active ? '当前' : '可切换'} active={ws.active}><div className="product-meta">模型：{ws.model || '-'} · 会话 {ws.session_count || 0} · 技能 {ws.enabled_skill_count || 0}/{ws.skill_count || 0} · 任务 {ws.task_count || 0}</div><div className="product-actions">{!ws.active ? <button className="secondary small" onClick={() => selectWorkspace(ws.id || ws.name)}>切换到此工作空间</button> : null}{(ws.id || ws.name) !== 'default' && workspaces.length > 1 ? <button className="danger small" onClick={() => deleteWorkspace(ws.id || ws.name, ws.name || ws.id)}>{ws.active ? '删除当前工作空间' : '删除'}</button> : null}</div></TextCard>) : <div className="empty compact">还没有工作空间，请创建第一个工作空间。</div>}</div>
  </>;
}

function ModelModule({ config, setConfig, saveConfig, showPromptPreview, promptPreview, testModelProvider, fetchProviderModels, availableModels, loadingModels, providers }) {
  const update = (key, value) => setConfig(c => ({...c, [key]: value}));
  return <>
    <div className="settings-block-head"><label>当前工作空间模型</label></div>
    <label>Base URL</label><input value={config.base_url} onChange={e => update('base_url', e.target.value)} placeholder="https://api.openai.com/v1" />
    <label>API Key</label><input type="password" value={config.api_key} onChange={e => update('api_key', e.target.value)} placeholder={config.has_api_key ? '已保存，留空不修改' : '未设置'} />
    <div className="settings-block-head inline-head"><label>Model</label><button className="secondary small" onClick={fetchProviderModels} disabled={loadingModels}>{loadingModels ? '获取中…' : '获取模型列表'}</button></div>
    <input value={config.model} onChange={e => update('model', e.target.value)} placeholder="gpt-4o-mini" />
    {availableModels.length ? <div className="model-options">{availableModels.map(name => <button key={name} type="button" className={'model-option ' + (name === config.model ? 'active' : '')} onClick={() => update('model', name)}>{name}</button>)}</div> : <div className="hint">填写 Base URL 和 API Key 后，可从接口获取可用模型名称。</div>}
    <label>System Prompt</label><textarea value={config.system_prompt} onChange={e => update('system_prompt', e.target.value)} />
    <div className="row"><div><label>上下文消息数</label><input type="number" value={config.max_context_messages} onChange={e => update('max_context_messages', e.target.value)} /></div><div><label>Temperature</label><input type="number" step="0.1" min="0" max="2" value={config.temperature} onChange={e => update('temperature', e.target.value)} /></div></div>
    <label className="check-row"><input type="checkbox" checked={!!config.enable_thinking} onChange={e => update('enable_thinking', e.target.checked)} /> 启用模型思考</label>
    <label className="check-row"><input type="checkbox" checked={!!config.hide_thinking} onChange={e => update('hide_thinking', e.target.checked)} /> 隐藏思考内容</label>
    <div className="settings-actions"><button onClick={saveConfig}>保存模型设置</button><button className="secondary" onClick={showPromptPreview}>查看最终 Prompt</button><button className="secondary" onClick={testModelProvider}>测试连接</button></div>
    {promptPreview ? <pre className="code-preview">{promptPreview}</pre> : null}
    <div className="settings-block-head"><label>模型供应商</label></div>
    {providers.length ? providers.map(p => <TextCard key={(p.workspace_id || '') + p.id} title={p.name || p.id} hint={p.base_url || '-'} badge={p.type || 'openai'}><div className="product-meta">默认模型：{p.default_model || '-'} · Key：{p.has_api_key ? (p.api_key_masked || '******') : '未设置'} · 工作空间：{p.workspace_name || '-'}</div></TextCard>) : <div className="empty compact">还没有模型供应商配置。</div>}
  </>;
}

function SkillCard({ skill, editSkill, deleteSkill, toggleSkill }) {
  return <div className="skill-card"><div className="skill-head"><div><div className="skill-name">{skill.name || '未命名技能'}</div><div className="skill-desc">{skill.description || '无描述'}</div></div><div className="skill-actions"><button className="secondary small" onClick={() => editSkill(skill.id)}>编辑</button><button className="danger small" onClick={() => deleteSkill(skill.id)}>删除</button></div></div><label className="skill-toggle"><input type="checkbox" checked={!!skill.enabled} onChange={e => toggleSkill(skill.id, e.target.checked)} /> 启用</label></div>;
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
    const payload = JSON.parse(atob(parts[0].replace(/-/g, '+').replace(/_/g, '/')));
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
  const timeout = Number(draft.timeout_ms || 0);
  const cacheTTL = Number(draft.cache_ttl_ms || 0);
  if (Number.isFinite(timeout) && timeout > 0) next.timeout_ms = Math.round(timeout);
  if (Number.isFinite(cacheTTL) && cacheTTL > 0) next.cache_ttl_ms = Math.round(cacheTTL);
  return next;
}

function defaultMCPServerDraft() {
  return {name: '', type: 'streamable-http', url: '', path: '', disabled: false, auth_type: 'none', token: '', token_env: '', allow_tools: '', deny_tools: '', confirm_tools: '', timeout_ms: '30000', cache_ttl_ms: ''};
}

function ToolsModule({ mcpStatus, mcpConfig, setMcpConfig, saveMCPConfig, loadMCPConfig, loadMCPStatus, testMCP }) {
  const [newServer, setNewServer] = useState(defaultMCPServerDraft);
  const [formError, setFormError] = useState('');
  const parsed = useMemo(() => parseMCPConfigDraft(mcpConfig), [mcpConfig]);
  const serverNames = Object.keys(parsed.config.servers || {}).sort();
  const statusByName = useMemo(() => Object.fromEntries((mcpStatus || []).map(s => [s.name, s])), [mcpStatus]);

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

  function removeServer(name) {
    replaceConfig(next => { delete next.servers[name]; });
  }

  function addServer() {
    const name = String(newServer.name || '').trim();
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
    const urlHasLocalhost = /^http:\/\/(127\.0\.0\.1|localhost)(?=[:/]|$)/i.test(normalizeMCPURLDraft(draft.url));
    const urlHasSpace = /\s/.test(String(draft.url || ''));
    const tokenEnvLooksLikeHeader = String(draft.token_env || '').trim().toLowerCase() === 'authorization';
    const tokenExpiry = mcpTokenExpiryState(draft.token);
    return <div className={'mcp-form-card ' + (isNew ? 'new-server' : '')} key={isNew ? 'new' : name}>
      <div className="mcp-form-head">
        <div><b>{isNew ? '新增 MCP Server' : name}</b><div className="hint">{isNew ? '填地址即可，不需要手写 JSON。' : serverStatusSummary(status)}</div></div>
        {!isNew ? <div className="mcp-form-head-actions"><button className="secondary small" onClick={() => patchServer(name, {disabled: !draft.disabled})}>{draft.disabled ? '启用' : '禁用'}</button><button className="danger small" onClick={() => removeServer(name)}>删除</button></div> : null}
      </div>
      {isNew ? <label>Server 名称<input value={draft.name} onChange={e => setNewServer(s => ({...s, name: e.target.value}))} placeholder="例如 agentdock" /></label> : null}
      <div className="mcp-form-grid">
        <label>连接类型<select value={draft.type} onChange={e => update({type: e.target.value})}><option value="streamable-http">HTTP / Streamable HTTP</option></select></label>
        <label>状态<select value={draft.disabled ? 'disabled' : 'enabled'} onChange={e => update({disabled: e.target.value === 'disabled'})}><option value="enabled">启用</option><option value="disabled">禁用</option></select></label>
      </div>
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
        <label>允许工具（每行一个，可留空）<textarea value={draft.allow_tools} onChange={e => update({allow_tools: e.target.value})} placeholder={'tool_a\nserver__tool_b'} /></label>
        <label>禁止工具（每行一个，可留空）<textarea value={draft.deny_tools} onChange={e => update({deny_tools: e.target.value})} /></label>
        <label>调用前确认的工具（每行一个，可留空）<textarea value={draft.confirm_tools} onChange={e => update({confirm_tools: e.target.value})} /></label>
        <label>本地路径备注<input value={draft.path} onChange={e => update({path: e.target.value})} placeholder="仅作为备注保留；当前 MCP 调用使用 HTTP 地址" /></label>
      </details>
      <div className="mcp-form-actions">
        {isNew ? <button onClick={addServer}>添加 Server</button> : <button className="secondary" onClick={() => testMCP(name)} disabled={draft.disabled || !draft.url}>测试此 Server</button>}
        {!isNew && status?.last_error ? <button className="secondary" onClick={() => patchServer(name, {disabled: true})}>禁用异常 Server</button> : null}
      </div>
    </div>;
  };

  return <>
    <div className="settings-block-head"><label>MCP 工具中心</label><button className="secondary small" onClick={loadMCPStatus}>检测状态</button></div>
    <div id="mcpStatusCards" className="mcp-status-grid">{mcpStatus.length ? mcpStatus.map(s => <TextCard key={s.name} title={s.name} hint={s.url || '未填写 HTTP 地址'} badge={runStatusLabel(s.last_status || 'unknown')} badgeClass={runStatusClass(s.last_status || 'unknown')}><div className="product-meta">allow {s.allow_count} · deny {s.deny_count} · confirm {s.confirm_count} · token {s.has_token ? '已配置' : '无'}</div>{s.last_error ? <div className="task-error">{s.last_error}</div> : null}<div className="product-actions"><button className="secondary small" onClick={() => testMCP(s.name)} disabled={s.disabled || !s.url}>测试</button></div></TextCard>) : <div className="empty compact">尚未配置 MCP Server。添加后可在这里查看状态、权限和确认规则。</div>}</div>
    <div className="settings-block-head"><label>MCP Server 配置</label></div>
    {parsed.error ? <div className="backup-health warn">当前配置 JSON 损坏，表单无法解析：{parsed.error}。可以在下方高级区修复原始内容。</div> : null}
    {!parsed.error ? <div className="mcp-form-list">{serverNames.length ? serverNames.map(name => renderServerForm(name, parsed.config.servers[name])) : <div className="empty compact">暂无 Server，先添加一个 HTTP MCP 地址。</div>}{renderServerForm('', {}, true)}</div> : null}
    {formError ? <div className="backup-health warn">{formError}</div> : null}
    <div className="settings-actions mcp-primary-actions"><button onClick={saveMCPConfig}>保存 MCP 配置</button><button className="secondary" onClick={loadMCPConfig}>重新加载</button><button className="secondary" onClick={() => testMCP()}>测试默认 MCP</button></div>
    <details className="mcp-raw-json"><summary>高级：查看 / 编辑原始 JSON</summary><textarea className="mcp-editor" value={mcpConfig} onChange={e => setMcpConfig(e.target.value)} /></details>
  </>;
}

function RunCard({ run }) {
  return <div className="task-card run-card"><div className="task-head"><div><div className="task-name">{run.title || 'MCP 执行'}</div><div className="task-desc">{run.summary || '暂无摘要'}</div></div><div className="task-actions"><span className={'badge ' + runStatusClass(run.status)}>{runStatusLabel(run.status)}</span></div></div><div className="task-meta">工作空间：{run.workspace || '-'} · 事件 {run.event_count || 0} · 耗时 {fmtDuration(run.duration_ms)} · {fmtTime(run.updated_at)}</div>{run.error ? <div className="task-error">{run.error}</div> : null}</div>;
}

function AgentTaskCard({ task, continueAgentTask }) {
  return <div className="task-card agent-task-card"><div className="task-head"><div><div className="task-name">{task.title || 'AgentDock 任务'}</div><div className="task-desc">{task.summary || '暂无摘要'}</div></div><div className="task-actions"><span className={'badge ' + runStatusClass(task.status)}>{runStatusLabel(task.status)}</span><button className="secondary small" onClick={() => continueAgentTask(task)}>继续任务</button></div></div><div className="task-meta">{task.server || 'AgentDock'} · {task.action || '-'}{task.phase ? ' · 阶段：' + task.phase : ''} · {fmtTime(task.updated_at)}</div>{task.error ? <div className="task-error">{task.error}</div> : null}</div>;
}

function TaskCard({ task, editScheduledTask, deleteScheduledTask, toggleScheduledTask, runScheduledTaskNow }) {
  return <div className="task-card"><div className="task-head"><div><div className="task-name">{task.title || '未命名任务'}{task.running ? ' · 运行中' : ''}</div><div className="task-desc">{(task.prompt || '').slice(0, 120) || '无提示内容'}</div></div><div className="task-actions"><span className={'badge ' + taskStatusClass(task)}>{taskStatusLabel(task)}</span><button className="secondary small" disabled={task.running} onClick={() => runScheduledTaskNow(task.id)}>立即运行</button><button className="secondary small" disabled={task.running} onClick={() => editScheduledTask(task.id)}>编辑</button><button className="danger small" disabled={task.running} onClick={() => deleteScheduledTask(task.id)}>删除</button></div></div><label className="task-toggle"><input type="checkbox" disabled={task.running} checked={!!task.enabled} onChange={e => toggleScheduledTask(task.id, e.target.checked)} /> 启用</label><div className="task-meta">{scheduleSummary(task)}</div>{task.running ? <div className="hint">任务运行中，暂时不能编辑、删除或切换启用状态。</div> : null}{task.last_error ? <div className="task-error">上次错误：{task.last_error}</div> : null}</div>;
}

function DataStatus({ dataStatus, onCopy }) {
  if (!dataStatus) return <div className="hint">尚未加载数据状态。</div>;
  const items = [
    ['当前工作空间', dataStatus.active_workspace || '-'],
    ['数据目录', dataStatus.data_dir || '-'],
    ['数据库', dataStatus.database_exists ? (dataStatus.database_path || '-') : '未创建'],
    ['数据库大小', fmtBytes(dataStatus.database_size_bytes)],
    ['数据库健康', dataStatus.database_healthy ? '正常' : (dataStatus.database_warning || '需要检查')],
    ['工作空间', String(dataStatus.workspace_count || 0)],
    ['会话', String(dataStatus.session_count || 0)],
    ['WAL', dataStatus.wal_enabled ? '启用' : '未检测到'],
    ['备份目录', dataStatus.backup_dir || '未检测到'],
    ['数据库备份数量', String(dataStatus.backup_count || 0)],
    ['已检查备份目录', String((dataStatus.backup_checked_dirs || []).length)],
    ['最近数据库备份', dataStatus.latest_backup_at ? fmtTime(dataStatus.latest_backup_at) + ' · ' + fmtBytes(dataStatus.latest_backup_size_bytes) + ' · ' + fmtRelativeAge(dataStatus.latest_backup_age_seconds) : '暂无数据库备份'],
    ['备份健康', dataStatus.backup_healthy ? '正常' : (dataStatus.backup_warning || '需要检查')],
  ];
  const backups = dataStatus.backups || [];
  return <>
    {dataStatus.backup_warning ? <div className="backup-health warn">{dataStatus.backup_warning}</div> : <div className="backup-health ok">数据库备份状态正常。</div>}
    <div id="dataStatus">{items.map(item => <div className="stat-card" key={item[0]}><div className="stat-label">{item[0]}</div><div className="stat-value">{item[1]}</div></div>)}</div>
    {(dataStatus.backup_checked_dirs || []).length ? <details className="backup-path checked-dirs"><summary>查看已检查备份目录</summary>{dataStatus.backup_checked_dirs.map(dir => <code key={dir}>{dir}</code>)}</details> : null}
    {backups.length ? <div className="backup-list"><div className="settings-block-head"><label>最近数据库备份</label></div>{backups.map(item => <div className="backup-item" key={item.path || item.name}><div className="backup-main"><div><b>{item.name || safePathName(item.path)}</b><div className="hint">{fmtTime(item.updated_at)} · {fmtRelativeAge(item.age_seconds)} · {fmtBytes(item.size_bytes)}</div></div>{item.path ? <button className="secondary mini" onClick={() => onCopy?.(item.path)}>复制路径</button> : null}</div>{item.path ? <details className="backup-path"><summary>查看完整路径</summary><code>{item.path}</code></details> : null}</div>)}</div> : null}
  </>;
}

function SecurityModule({ systemStatus, setupStatus, dataStatus, mcpStatus, providers, loadSystemStatus, logout, onCopy }) {
  const text = diagnosticsText({setupStatus, systemStatus, dataStatus, mcpStatus, providers});
  return <>
    <div className="settings-block-head"><label>系统状态</label><button className="secondary small" onClick={loadSystemStatus}>刷新系统状态</button></div>
    {systemStatus ? <TextCard title="ChatDock" hint={systemStatus.addr || ''} badge={systemStatus.ok ? 'healthy' : 'unknown'} badgeClass={systemStatus.ok ? 'ok' : 'warn'}><div className="product-meta">Web：{systemStatus.web_dir || '内嵌'} · DB：{systemStatus.database || '-'} · 当前工作空间：{(systemStatus.setup || {}).active_workspace || '-'}</div></TextCard> : <div className="hint">尚未加载系统状态。</div>}
    <div className="settings-block-head"><label>诊断信息</label></div>
    <pre className="diagnostics-preview">{text}</pre>
    <div className="settings-actions"><button className="secondary" onClick={() => onCopy?.(text)}>复制诊断信息</button><button className="secondary" onClick={logout}>登录 / 切换账号</button></div>
  </>;
}
