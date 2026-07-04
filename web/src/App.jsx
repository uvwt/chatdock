import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AttachmentList, EmptyState, MessageView } from './components/chat.jsx';
import { DialogHost, LoginPage, Markdown, QuickPalette, WorkspacePicker } from './components/base.jsx';
import { SettingsPanel } from './components/settings.jsx';
import { defaultRunAtValue, diagnosticsText, filenameFromResponse, fmtDuration, fmtTime, normalizeSettingsModule, runStatusLabel, sessionIDFromPath, sessionPath, settingsModuleFromPath } from './lib/appUtils.js';
import { createJsonApi } from './lib/http.js';
import { cancelChatJob, fetchChatJobs, resolveMCPConfirmation, streamChat, streamChatJobEvents } from './lib/chatApi.js';
import { branchSession, cloneSession, createSessionRecord, deleteSession, fetchContextPreview, fetchSession, fetchSessionMarkdown, fetchSessions, pinSession, renameSession, searchSessions } from './lib/sessionApi.js';
import { createWorkspaceRecord, deleteScheduledTaskRecord, deleteSkillRecord, deleteWorkspaceRecord, fetchAgentTasks, fetchConfig, fetchDataStatus, fetchMCPConfig, fetchMCPStatus, fetchModelProviders, fetchPrompts, fetchProviderModels as fetchProviderModelsRequest, fetchPromptPreview, fetchRuns, fetchScheduledTasks, fetchSetupStatus, fetchSkills, fetchSystemStatus, fetchWorkspaces, initializeSetup, runScheduledTask, saveMCPConfigRequest, saveScheduledTaskRecord, saveSkillRecord, saveWorkspaceConfig, selectWorkspace as selectWorkspaceRequest, testMCPServer, testModelProvider as testModelProviderRequest } from './lib/settingsApi.js';
import { uploadFileRequest } from './lib/upload.js';

function streamStatusText(stats, elapsed) {
  const labels = {connecting:'连接模型中', streaming:'流式输出中', paused:'已暂停，后台继续接收', stopping:'正在中断', done:'已完成', error:'输出失败'};
  const parts = [labels[stats.state] || '待命'];
  if (elapsed) parts.push(elapsed + 's');
  if (stats.chars) parts.push(stats.chars + ' 字');
  if (stats.tools) parts.push(stats.tools + ' 个工具');
  if (stats.events) parts.push(stats.events + ' 个事件');
  if (stats.error) parts.push(stats.error);
  return parts.join(' · ');
}

function attachmentLooksLikeImage(item) {
  return String(item?.mime_type || item?.type || '').toLowerCase().startsWith('image/');
}

function uniqueModelNames(value) {
  const raw = Array.isArray(value) ? value.join('\n') : String(value || '');
  const seen = new Set();
  return raw.split(/[\n,，]+/).map(item => item.trim()).filter(Boolean).filter(item => {
    if (seen.has(item)) return false;
    seen.add(item);
    return true;
  });
}

function providerChoiceID(provider) {
  return provider?.workspace_id || provider?.id || '';
}

function providerLabel(provider) {
  return provider?.workspace_name || provider?.name || provider?.id || '供应商';
}

function compactModelName(name) {
  name = String(name || '').trim();
  return name.length > 22 ? name.slice(0, 21) + '…' : name;
}

function ComposerModelPicker({ busy, providers, selectedProvider, selectedModel, open, setOpen, selectModel, openSettings }) {
  return <div className="model-picker">
    <button type="button" className="secondary model-picker-trigger" disabled={busy || !providers.length} onClick={() => setOpen(value => !value)} title="选择供应商 / 模型"><span>{providerLabel(selectedProvider)}</span><b>{compactModelName(selectedModel) || '未选择模型'}</b></button>
    {open ? <div className="model-picker-popover">
      <div className="model-picker-head"><b>选择供应商模型</b><button type="button" className="secondary small" onClick={() => setOpen(false)}>关闭</button></div>
      <div className="model-provider-list">
        {providers.length ? providers.map(provider => <div className="model-provider-item" key={provider.choice_id}>
          <div className="model-provider-title"><b>{providerLabel(provider)}</b><small>{provider.base_url || '-'}</small></div>
          <div className="model-chip-list">
            {provider.models.length ? provider.models.map(name => <button type="button" key={provider.choice_id + name} className={'model-chip ' + (selectedProvider?.choice_id === provider.choice_id && selectedModel === name ? 'active' : '')} onClick={() => selectModel(provider, name)}>{name}</button>) : <button type="button" className="model-chip" onClick={() => openSettings('model')}>添加模型</button>}
          </div>
        </div>) : <div className="empty compact">还没有可选模型，请先到配置中心添加。</div>}
      </div>
    </div> : null}
  </div>;
}

function readableChatError(error, hasImageAttachment = false) {
  const raw = String(error?.message || error || '').trim();
  if (!raw) return '模型调用失败。';
  if (/only support text input|model only support text|不支持.*图片|只支持文本/i.test(raw)) {
    return hasImageAttachment
      ? '当前模型只支持文本输入，不能读取图片。请切换支持图片/视觉的模型，或移除图片附件后再发送。'
      : '当前模型只支持文本输入，不能处理这次请求里的非文本内容。';
  }
  const jsonStart = raw.indexOf('{');
  if (jsonStart >= 0) {
    try {
      const data = JSON.parse(raw.slice(jsonStart));
      const message = data?.error?.message || data?.message || '';
      if (message) return String(message);
    } catch {}
  }
  return raw.replace(/^model api failed:\s*/i, '');
}

function safeJSONStringify(value) {
  try { return JSON.stringify(value, null, 2); }
  catch { return String(value || ''); }
}

function hasDialogValue(value) {
  if (value == null || value === '') return false;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'object') return Object.keys(value).length > 0;
  return true;
}

function buildToolEventDetail(event) {
  const details = event?.details || {};
  const data = details.data && typeof details.data === 'object' ? details.data : {};
  const eventName = details.event || data.event || 'tool_event';
  const tool = details.tool || data.tool || '';
  const ok = typeof details.ok === 'boolean' ? details.ok : (typeof data.ok === 'boolean' ? data.ok : null);
  const failed = ok === false || /error|failed|cancelled/i.test(eventName) || details.error || data.error;
  const ready = /ready|resolved|finish|done/i.test(eventName) || ok === true;
  const status = failed ? '失败' : (ready ? '完成' : '事件');
  const duration = details.duration_ms || data.duration_ms;
  const metrics = [
    {label:'工具总数', value:data.tool_count ?? details.tool_count},
    {label:'内置工具', value:data.builtin_tool_count ?? details.builtin_tool_count},
    {label:'耗时', value:duration ? fmtDuration(duration) : ''},
  ].filter(item => hasDialogValue(item.value));
  const rows = [
    {label:'事件类型', value:eventName},
    {label:'工具名称', value:tool},
    {label:'服务', value:data.server || details.server},
    {label:'动作', value:data.action || details.action},
    {label:'状态', value:status},
  ].filter(item => hasDialogValue(item.value));
  const sections = [];
  if (hasDialogValue(details.arguments ?? data.arguments)) sections.push({title:'参数', value:details.arguments ?? data.arguments, emptyText:'无参数'});
  if (hasDialogValue(details.result ?? data.result)) sections.push({title:'响应', value:details.result ?? data.result, emptyText:'无响应'});
  if (hasDialogValue(details.error || data.error)) sections.push({title:'错误', value:details.error || data.error, tone:'danger'});
  if (hasDialogValue(data) && !sections.length) sections.push({title:'事件数据', value:data});
  sections.push({title:'原始事件', value:details});
  return {
    event:eventName,
    heading:event?.text || tool || '工具事件',
    status,
    statusTone:failed ? 'danger' : (ready ? 'success' : ''),
    metrics,
    rows,
    sections,
  };
}

function contextPreviewText(data) {
  const lines = [];
  lines.push('工作空间：' + (data.workspace || '-'));
  lines.push('上下文模式：' + (data.context_mode || '-') + ' · 最近消息窗口：' + (data.recent_messages || 0) + ' · 早期摘要：' + (data.summarize_old ? '开启' : '关闭'));
  lines.push('会话消息：' + (data.message_count || 0) + ' 条 · 实际发送片段：' + (data.context_count || 0) + ' 条 · 粗略 token：' + (data.estimated_tokens || 0));
  lines.push('');
  (data.items || []).forEach((item, index) => {
    lines.push((index + 1) + '. ' + (item.source || item.role || '上下文') + ' · ' + (item.role || '-') + ' · ' + (item.chars || 0) + ' 字 · ≈' + (item.estimated_tokens || 0) + ' tokens');
    lines.push(item.content_preview || '');
    lines.push('');
  });
  return lines.join('\n');
}

export default function App() {
  const [authPage, setAuthPage] = useState(null);
  const [theme, setThemeState] = useState(() => localStorage.getItem('chatdock.theme') === 'day' ? 'day' : 'night');
  const [sidebarCollapsed, setSidebarCollapsedState] = useState(() => {
    const saved = localStorage.getItem('chatdock.sidebarCollapsed');
    return saved == null ? window.matchMedia('(max-width: 720px)').matches : saved === '1';
  });
  const [settingsOpen, setSettingsOpen] = useState(() => !!settingsModuleFromPath());
  const [activeModule, setActiveModule] = useState(() => normalizeSettingsModule(settingsModuleFromPath() || localStorage.getItem('chatdock.settingsModule') || 'workspace'));
  const [toast, setToast] = useState(null);
  const toastTimerRef = useRef(null);
  const [dialog, setDialog] = useState(null);
  const [workspacePickerOpen, setWorkspacePickerOpen] = useState(false);
  const [sessionActionsOpen, setSessionActionsOpen] = useState(false);
  const [quickPaletteOpen, setQuickPaletteOpen] = useState(false);
  const [modelPickerOpen, setModelPickerOpen] = useState(false);
  const [chatModel, setChatModel] = useState({provider_id:'', model:''});
  const [showJumpToLatest, setShowJumpToLatest] = useState(false);

  const [setupStatus, setSetupStatus] = useState(null);
  const [workspaces, setWorkspaces] = useState([]);
  const [providers, setProviders] = useState([]);
  const [availableModels, setAvailableModels] = useState([]);
  const [loadingModels, setLoadingModels] = useState(false);
  const [prompts, setPrompts] = useState([]);
  const [sessions, setSessions] = useState([]);
  const [sessionSearch, setSessionSearch] = useState('');
  const [sessionSearchResults, setSessionSearchResults] = useState([]);
  const [sessionSearchBusy, setSessionSearchBusy] = useState(false);
  const [current, setCurrent] = useState(null);
  const [currentTitle, setCurrentTitle] = useState('未选择会话');
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [pendingAttachments, setPendingAttachments] = useState([]);
  const [uploadingFiles, setUploadingFiles] = useState(false);
  const [busy, setBusy] = useState(false);
  const [streamPaused, setStreamPaused] = useState(false);
  const [streamStats, setStreamStats] = useState({state:'idle', started_at:0, chars:0, events:0, tools:0, error:''});
  const [activeJobID, setActiveJobID] = useState('');
  const [skills, setSkills] = useState([]);
  const [skillSearch, setSkillSearch] = useState('');
  const [scheduledTasks, setScheduledTasks] = useState([]);
  const [taskSearch, setTaskSearch] = useState('');
  const [runs, setRuns] = useState([]);
  const [agentTasks, setAgentTasks] = useState([]);
  const [dataStatus, setDataStatus] = useState(null);
  const [systemStatus, setSystemStatus] = useState(null);
  const [mcpStatus, setMcpStatus] = useState([]);
  const [promptPreview, setPromptPreview] = useState('');
  const [mcpConfig, setMcpConfig] = useState('');
  const [config, setConfig] = useState({base_url:'', api_key:'', model:'', models:[], system_prompt:'', context_mode:'auto', max_context_messages:12, temperature:0.7, enable_thinking:false, hide_thinking:false, has_api_key:false});

  const abortRef = useRef(null);
  const activeJobIDRef = useRef('');
  const pausedRef = useRef(false);
  const pendingDeltaRef = useRef('');
  const pendingReasoningRef = useRef('');
  const messagesRef = useRef(null);
  const stickToBottomRef = useRef(true);
  const forceScrollRef = useRef(false);
  const inputRef = useRef(null);
  const fileInputRef = useRef(null);

  useEffect(() => { pausedRef.current = streamPaused; }, [streamPaused]);
  useEffect(() => {
    document.body.classList.toggle('theme-light', theme === 'day');
    document.body.classList.toggle('theme-night', theme !== 'day');
    localStorage.setItem('chatdock.theme', theme);
  }, [theme]);

  useEffect(() => {
    document.body.classList.toggle('auth-page-visible', !!authPage);
    return () => document.body.classList.remove('auth-page-visible');
  }, [authPage]);

  const showToast = useCallback((message, variant='info') => {
    setToast({message, variant});
    clearTimeout(toastTimerRef.current);
    toastTimerRef.current = setTimeout(() => setToast(null), 3200);
  }, []);

  const showDialog = useCallback((config) => new Promise(resolve => {
    setDialog({...config, resolve});
  }), []);

  const copyText = useCallback(async (text) => {
    const value = String(text || '').trim();
    if (!value) {
      showToast('没有可复制的内容', 'error');
      return;
    }
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(value);
      } else {
        const area = document.createElement('textarea');
        area.value = value;
        area.setAttribute('readonly', '');
        area.style.position = 'fixed';
        area.style.opacity = '0';
        document.body.appendChild(area);
        area.select();
        document.execCommand('copy');
        area.remove();
      }
      showToast('已复制到剪贴板', 'success');
    } catch (e) {
      showToast('复制失败：' + e.message, 'error');
    }
  }, [showToast]);

  const closeDialog = useCallback((value) => {
    setDialog(currentDialog => {
      if (currentDialog?.resolve) currentDialog.resolve(value);
      return null;
    });
  }, []);

  const downloadBlob = useCallback((blob, filename) => {
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename || 'chatdock-session.md';
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }, []);

  const authHeaders = useCallback((extra={}) => {
    const token = localStorage.getItem('chatdock.authToken') || '';
    return token ? {'Authorization':'Bearer ' + token, ...extra} : extra;
  }, []);

  const api = useMemo(() => createJsonApi({authHeaders, onUnauthorized: setAuthPage}), [authHeaders]);

  const loadPrompts = useCallback(async () => {
    const data = await fetchPrompts(api);
    setPrompts(data.prompts || []);
  }, [api]);

  const loadSessions = useCallback(async () => {
    const list = await fetchSessions(api);
    setSessions(list || []);
  }, [api]);

  useEffect(() => {
    const q = sessionSearch.trim();
    if (!q) {
      setSessionSearchResults([]);
      setSessionSearchBusy(false);
      return;
    }
    let stopped = false;
    setSessionSearchBusy(true);
    const timer = window.setTimeout(async () => {
      try {
        const data = await searchSessions(api, q);
        if (!stopped) setSessionSearchResults(data.sessions || []);
      } catch {
        if (!stopped) setSessionSearchResults([]);
      } finally {
        if (!stopped) setSessionSearchBusy(false);
      }
    }, 260);
    return () => { stopped = true; window.clearTimeout(timer); };
  }, [api, sessionSearch]);

  const loadConfig = useCallback(async () => {
    const c = await fetchConfig(api);
    setConfig({
      base_url: c.base_url || '',
      api_key: '',
      model: c.model || '',
      models: Array.isArray(c.models) ? c.models : [],
      system_prompt: c.system_prompt || '',
      context_mode: c.context_mode || 'auto',
      max_context_messages: c.max_context_messages || 12,
      temperature: c.temperature ?? 0.7,
      enable_thinking: !!c.enable_thinking,
      hide_thinking: !!c.hide_thinking,
      has_api_key: !!c.has_api_key,
    });
  }, [api]);

  const loadMCPConfig = useCallback(async () => {
    const c = await fetchMCPConfig(api);
    setMcpConfig(c.content || '{\n  "servers": {}\n}\n');
  }, [api]);

  const loadSetupStatus = useCallback(async () => {
    const data = await fetchSetupStatus(api);
    setSetupStatus(data);
  }, [api]);

  const loadWorkspaces = useCallback(async () => {
    const data = await fetchWorkspaces(api);
    setWorkspaces(data.workspaces || []);
  }, [api]);

  const loadModelProviders = useCallback(async () => {
    const data = await fetchModelProviders(api);
    setProviders(data.providers || []);
  }, [api]);

  const loadSkills = useCallback(async () => {
    const data = await fetchSkills(api);
    setSkills(data.skills || []);
  }, [api]);

  const loadScheduledTasks = useCallback(async () => {
    const data = await fetchScheduledTasks(api);
    setScheduledTasks(data.tasks || []);
  }, [api]);

  const loadDataStatus = useCallback(async () => {
    const data = await fetchDataStatus(api);
    setDataStatus(data);
  }, [api]);

  const loadSystemStatus = useCallback(async () => {
    const data = await fetchSystemStatus(api);
    setSystemStatus(data);
  }, [api]);

  const loadMCPStatus = useCallback(async () => {
    const data = await fetchMCPStatus(api);
    setMcpStatus(data.servers || []);
  }, [api]);

  const loadRuns = useCallback(async () => {
    const data = await fetchRuns(api);
    setRuns(data.runs || []);
  }, [api]);

  const loadAgentTasks = useCallback(async () => {
    const data = await fetchAgentTasks(api);
    setAgentTasks(data.tasks || []);
  }, [api]);

  const refreshProductState = useCallback(async () => {
    await Promise.allSettled([loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
  }, [loadSetupStatus, loadWorkspaces, loadModelProviders, loadDataStatus, loadSystemStatus]);

  const refreshVisibleSettings = useCallback(async () => {
    const jobs = [loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadDataStatus(), loadSystemStatus()];
    if (activeModule === 'skills') jobs.push(loadSkills());
    if (activeModule === 'tools') jobs.push(loadMCPStatus());
    if (activeModule === 'runs') jobs.push(loadRuns());
    if (activeModule === 'agent') jobs.push(loadAgentTasks());
    if (activeModule === 'automation') jobs.push(loadScheduledTasks());
    await Promise.allSettled(jobs);
    showToast('配置中心已刷新', 'success');
  }, [activeModule, loadAgentTasks, loadDataStatus, loadMCPStatus, loadModelProviders, loadRuns, loadScheduledTasks, loadSetupStatus, loadSkills, loadSystemStatus, loadWorkspaces, showToast]);

  const loadSessionFromRoute = useCallback(async (id) => {
    if (!id) return false;
    const s = await fetchSession(api, id);
    setCurrent(s.id);
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    setPendingAttachments([]);
    await loadSessions();
    return true;
  }, [api, loadSessions]);

  const refreshAfterLogin = useCallback(async () => {
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig(), loadMCPConfig(), loadSkills(), loadScheduledTasks(), loadSessions()]);
    const routeSession = sessionIDFromPath();
    if (routeSession) await loadSessionFromRoute(routeSession).catch(e => showToast('会话路由加载失败：' + e.message, 'error'));
  }, [refreshProductState, loadPrompts, loadConfig, loadMCPConfig, loadSkills, loadScheduledTasks, loadSessions, loadSessionFromRoute, showToast]);

  useEffect(() => {
    let mounted = true;
    async function start() {
      try {
        const status = await api('/api/auth/status');
        if (!mounted) return;
        if (status.enabled && status.login_enabled && !localStorage.getItem('chatdock.authToken')) {
          setAuthPage({message: '请输入 ChatDock 账号和密码。'});
          return;
        }
        setAuthPage(null);
        await refreshAfterLogin();
      } catch (e) {
        if (mounted) setAuthPage(e);
      }
    }
    start();
    return () => { mounted = false; };
  }, [api, refreshAfterLogin]);

  useEffect(() => {
    function onPopState() {
      const routeModule = settingsModuleFromPath();
      if (routeModule) {
        setActiveModule(routeModule);
        setSettingsOpen(true);
        return;
      }
      setSettingsOpen(false);
      const routeSession = sessionIDFromPath();
      if (routeSession) loadSessionFromRoute(routeSession).catch(e => showToast('会话路由加载失败：' + e.message, 'error'));
      else {
        setCurrent(null);
        setCurrentTitle('未选择会话');
        setMessages([]);
      }
    }
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, [loadSessionFromRoute, showToast]);

  useEffect(() => {
    if (!settingsOpen) return;
    if (activeModule === 'tools') loadMCPStatus().catch(e => showToast('MCP 状态加载失败：' + e.message, 'error'));
    if (activeModule === 'runs') loadRuns().catch(e => showToast('执行记录加载失败：' + e.message, 'error'));
    if (activeModule === 'agent') loadAgentTasks().catch(e => showToast('Agent 任务加载失败：' + e.message, 'error'));
    if (activeModule === 'data') loadDataStatus().catch(e => showToast('数据状态加载失败：' + e.message, 'error'));
    if (activeModule === 'security') loadSystemStatus().catch(e => showToast('系统状态加载失败：' + e.message, 'error'));
  }, [settingsOpen, activeModule, loadMCPStatus, loadRuns, loadAgentTasks, loadDataStatus, loadSystemStatus, showToast]);

  const latestModelMessageElement = useCallback(() => {
    const box = messagesRef.current;
    if (!box) return null;
    const nodes = box.querySelectorAll('[data-model-message="true"]');
    return nodes[nodes.length - 1] || null;
  }, []);

  const updateJumpToLatestVisibility = useCallback(() => {
    const box = messagesRef.current;
    if (!box) return;
    const latest = latestModelMessageElement();
    const bottomGap = box.scrollHeight - box.scrollTop - box.clientHeight;
    if (!latest) {
      setShowJumpToLatest(messages.length > 0 && bottomGap > 160);
      return;
    }
    const latestTop = latest.offsetTop - box.scrollTop;
    const latestBottom = latestTop + latest.offsetHeight;
    // 只在用户确实停在“最新模型消息”上方时出现；如果用户已经在最新回复内部阅读，不打扰。
    const shouldShow = bottomGap > 160 && latestTop > box.clientHeight - 96 && latestBottom > box.clientHeight;
    setShowJumpToLatest(prev => prev === shouldShow ? prev : shouldShow);
  }, [latestModelMessageElement, messages.length]);

  const scrollToLatestModelMessage = useCallback(() => {
    const box = messagesRef.current;
    if (!box) return;
    const latest = latestModelMessageElement();
    if (latest) {
      box.scrollTo({top: Math.max(0, latest.offsetTop - 14), behavior: 'smooth'});
    } else {
      box.scrollTo({top: box.scrollHeight, behavior: 'smooth'});
    }
    window.setTimeout(updateJumpToLatestVisibility, 360);
  }, [latestModelMessageElement, updateJumpToLatestVisibility]);

  useEffect(() => {
    const box = messagesRef.current;
    if (!box) return;
    if (!messages.length) {
      // 空状态本身可能高于手机视口；此时不要沿用聊天流的“贴底”策略。
      box.scrollTop = 0;
      forceScrollRef.current = false;
      stickToBottomRef.current = true;
      setShowJumpToLatest(false);
      return;
    }
    if (forceScrollRef.current || stickToBottomRef.current) {
      box.scrollTop = box.scrollHeight;
      forceScrollRef.current = false;
    }
    window.requestAnimationFrame(updateJumpToLatestVisibility);
  }, [messages, updateJumpToLatestVisibility]);

  const handleMessagesScroll = useCallback(() => {
    const box = messagesRef.current;
    if (!box) return;
    // 用户手动上滑时停止跟随流式输出；只有接近底部时继续自动贴底。
    stickToBottomRef.current = box.scrollHeight - box.scrollTop - box.clientHeight < 120;
    updateJumpToLatestVisibility();
  }, [updateJumpToLatestVisibility]);

  const setSidebarCollapsed = useCallback((value) => {
    setSidebarCollapsedState(value);
    localStorage.setItem('chatdock.sidebarCollapsed', value ? '1' : '0');
  }, []);

  const closeSidebarOnMobile = useCallback(() => {
    if (window.matchMedia('(max-width: 720px)').matches) setSidebarCollapsed(true);
  }, [setSidebarCollapsed]);

  const openSettings = useCallback((moduleName = activeModule, syncRoute = true) => {
    const normalized = normalizeSettingsModule(moduleName);
    setActiveModule(normalized);
    setSettingsOpen(true);
    localStorage.setItem('chatdock.settingsModule', normalized);
    if (syncRoute && window.location.pathname !== '/settings/' + normalized) window.history.pushState({chatdock:true}, '', '/settings/' + normalized);
  }, [activeModule]);

  const closeSettings = useCallback((syncRoute = true) => {
    setSettingsOpen(false);
    const target = current ? sessionPath(current) : '/';
    if (syncRoute && window.location.pathname !== target) window.history.pushState({chatdock:true}, '', target);
  }, [current]);

  const switchSettingsModule = useCallback((moduleName) => openSettings(moduleName), [openSettings]);

  useEffect(() => {
    function closeTopLayer(event) {
      if (event.key !== 'Escape') return;
      if (dialog) closeDialog(null);
      else if (quickPaletteOpen) setQuickPaletteOpen(false);
      else if (sessionActionsOpen) setSessionActionsOpen(false);
      else if (workspacePickerOpen) setWorkspacePickerOpen(false);
      else if (settingsOpen) closeSettings();
    }
    window.addEventListener('keydown', closeTopLayer);
    return () => window.removeEventListener('keydown', closeTopLayer);
  }, [closeDialog, closeSettings, dialog, quickPaletteOpen, sessionActionsOpen, settingsOpen, workspacePickerOpen]);

  useEffect(() => {
    function onGlobalShortcut(event) {
      const target = event.target;
      const tag = String(target?.tagName || '').toLowerCase();
      const editing = tag === 'input' || tag === 'textarea' || tag === 'select' || target?.isContentEditable;
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setQuickPaletteOpen(true);
        return;
      }
      if (!editing && event.key === '/') {
        event.preventDefault();
        inputRef.current?.focus();
      }
    }
    window.addEventListener('keydown', onGlobalShortcut);
    return () => window.removeEventListener('keydown', onGlobalShortcut);
  }, []);

  const selectWorkspace = useCallback(async (name) => {
    if (busy) { showToast('当前回复还在进行中，请先暂停或中断后再切换工作空间。', 'error'); return; }
    if (!name) return;
    setWorkspacePickerOpen(false);
    await selectWorkspaceRequest(api, name);
    setCurrent(null);
    setCurrentTitle('未选择会话');
    setMessages([{role:'empty', content:'已切换工作空间。创建或选择一个会话。'}]);
    setPendingAttachments([]);
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig(), loadMCPConfig(), loadSkills(), loadScheduledTasks(), loadSessions()]);
    if (window.location.pathname !== '/') window.history.pushState({chatdock:true}, '', '/');
    closeSidebarOnMobile();
  }, [api, busy, refreshProductState, loadPrompts, loadConfig, loadMCPConfig, loadSkills, loadScheduledTasks, loadSessions, closeSidebarOnMobile, showToast]);

  const createPersistedSession = useCallback(async ({refreshList = true} = {}) => {
    const s = await createSessionRecord(api);
    setCurrent(s.id);
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    setPendingAttachments([]);
    if (refreshList) await loadSessions();
    if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({chatdock:true}, '', sessionPath(s.id));
    return s;
  }, [api, loadSessions]);

  const createSession = useCallback(() => {
    if (busy) {
      showToast('当前回复还在进行中，请先暂停或中断后再新建会话。', 'error');
      return null;
    }
    // “新会话”只是进入一个本地草稿，不应该提前写入后端。
    // 真正的 session id 只有在发送首条消息、或上传附件需要绑定会话时才创建。
    setCurrent(null);
    setCurrentTitle('新会话');
    setMessages([]);
    setPendingAttachments([]);
    if (window.location.pathname !== '/') window.history.pushState({chatdock:true}, '', '/');
    closeSidebarOnMobile();
    window.setTimeout(() => inputRef.current?.focus(), 0);
    return {id:'', title:'新会话', messages:[], draft:true};
  }, [busy, closeSidebarOnMobile, showToast]);

  const openSession = useCallback(async (id) => {
    if (busy) {
      showToast('当前回复还在进行中，请先暂停或中断后再切换会话。', 'error');
      return;
    }
    setCurrent(id);
    const s = await fetchSession(api, id);
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    setPendingAttachments([]);
    await loadSessions();
    if (window.location.pathname !== sessionPath(id)) window.history.pushState({chatdock:true}, '', sessionPath(id));
    closeSidebarOnMobile();
  }, [api, busy, loadSessions, closeSidebarOnMobile, showToast]);

  const newSession = useCallback(async () => { await createSession(); }, [createSession]);

  const renameCurrent = useCallback(async () => {
    if (!current) return;
    const values = await showDialog({title:'重命名会话', confirmText:'保存标题', fields:[{name:'title', label:'新的会话标题', value:currentTitle || '', required:true}]});
    if (!values || !values.title.trim()) return;
    const s = await renameSession(api, current, values.title.trim());
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    await loadSessions();
    showToast('会话标题已保存', 'success');
  }, [api, current, currentTitle, loadSessions, showDialog, showToast]);

  const deleteSessionByID = useCallback(async (id, title = '当前会话') => {
    if (!id || busy) return;
    const ok = await showDialog({
      title:'删除会话',
      message:'确定删除「' + (title || '未命名会话') + '」？此操作不可恢复。',
      confirmText:'删除',
      danger:true,
      type:'confirm'
    });
    if (!ok) return;
    await deleteSession(api, id);
    // 如果删的是当前打开的会话，需要同步清空主界面和路由，避免 URL 指向已删除会话。
    if (id === current) {
      setCurrent(null);
      setCurrentTitle('未选择会话');
      setMessages([]);
      setPendingAttachments([]);
      if (window.location.pathname !== '/') window.history.pushState({chatdock:true}, '', '/');
    }
    await loadSessions();
    showToast('会话已删除', 'success');
  }, [api, busy, current, loadSessions, showDialog, showToast]);

  const deleteCurrent = useCallback(async () => {
    if (!current) return;
    await deleteSessionByID(current, currentTitle || '当前会话');
  }, [current, currentTitle, deleteSessionByID]);

  const exportCurrent = useCallback(async () => {
    if (!current) return;
    try {
      const res = await fetchSessionMarkdown(current, authHeaders);
      downloadBlob(await res.blob(), filenameFromResponse(res, 'chatdock-session.md'));
      showToast('已导出 Markdown', 'success');
    } catch (e) {
      showToast('导出失败：' + e.message, 'error');
    }
  }, [authHeaders, current, downloadBlob, showToast]);

  const copyCurrentMarkdown = useCallback(async () => {
    if (!current) return;
    try {
      const res = await fetchSessionMarkdown(current, authHeaders);
      await copyText(await res.text());
    } catch (e) {
      showToast('复制全文失败：' + e.message, 'error');
    }
  }, [authHeaders, copyText, current, showToast]);

  const cloneCurrent = useCallback(async () => {
    if (!current || busy) return;
    try {
      const s = await cloneSession(api, current);
      setCurrent(s.id);
      setCurrentTitle(s.title || '会话副本');
      setMessages(s.messages || []);
      await loadSessions();
      if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({chatdock:true}, '', sessionPath(s.id));
      showToast('会话已复制', 'success');
    } catch (e) {
      showToast('复制会话失败：' + e.message, 'error');
    }
  }, [api, busy, current, loadSessions, showToast]);


  const branchCurrent = useCallback(async (messageIndex = messages.length - 1) => {
    if (!current || busy) return;
    try {
      const s = await branchSession(api, current, messageIndex);
      setCurrent(s.id);
      setCurrentTitle(s.title || '分支对话');
      setMessages(s.messages || []);
      setPendingAttachments([]);
      await loadSessions();
      if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({chatdock:true}, '', sessionPath(s.id));
      closeSidebarOnMobile();
      showToast('已在新聊天中创建分支对话', 'success');
    } catch (e) {
      showToast('创建分支对话失败：' + e.message, 'error');
    }
  }, [api, busy, closeSidebarOnMobile, current, loadSessions, messages.length, showToast]);

  const pinCurrent = useCallback(async () => {
    if (!current) return;
    const currentSummary = sessions.find(s => s.id === current);
    const nextPinned = !currentSummary?.pinned;
    const s = await pinSession(api, current, nextPinned);
    setCurrentTitle(s.title || currentTitle || '新会话');
    setMessages(s.messages || []);
    await loadSessions();
    showToast(nextPinned ? '会话已置顶' : '已取消置顶', 'success');
  }, [api, current, currentTitle, loadSessions, sessions, showToast]);

  const appendToActiveAssistant = useCallback((patcher) => {
    setMessages(prev => prev.map((m, index) => index === prev.length - 1 && m.role === 'assistant-stream' ? patcher(m) : m));
  }, []);

  const appendAnswer = useCallback((text) => {
    if (!text) return;
    appendToActiveAssistant(m => ({...m, answer: (m.answer || '') + text}));
  }, [appendToActiveAssistant]);

  const appendReasoning = useCallback((text) => {
    if (!text) return;
    appendToActiveAssistant(m => ({...m, reasoning: (m.reasoning || '') + text}));
  }, [appendToActiveAssistant]);

  const handleChatStreamEvent = useCallback((event, data, setFinalSession) => {
    if (event === 'job_started') {
      activeJobIDRef.current = data.id || '';
      setActiveJobID(data.id || '');
    } else if (event === 'delta') {
      const reasoning = data.reasoning_content || '';
      const content = data.content || '';
      setStreamStats(prev => ({...prev, state: pausedRef.current ? 'paused' : 'streaming', chars: prev.chars + content.length + reasoning.length}));
      if (pausedRef.current) {
        pendingReasoningRef.current += reasoning;
        pendingDeltaRef.current += content;
      } else {
        appendReasoning(reasoning);
        appendAnswer(content);
      }
    } else if (event === 'tool_setup_ready') {
      setStreamStats(prev => ({...prev, events: prev.events + 1}));
      appendToActiveAssistant(m => ({...m, events:[...(m.events || []), {kind:'tool', text:'🧰 MCP 已接入：' + (data.tool_count || 0) + ' 个工具', details:{event, data}}]}));
    } else if (event === 'tool_setup_error') {
      setStreamStats(prev => ({...prev, events: prev.events + 1, error: data.message || 'MCP 工具未接入'}));
      appendToActiveAssistant(m => ({...m, events:[...(m.events || []), {kind:'tool', text:'⚠️ MCP 未接入：' + (data.message || '工具初始化失败'), details:{event, data}}]}));
    } else if (event === 'tool_call_start') {
      setStreamStats(prev => ({...prev, events: prev.events + 1, tools: prev.tools + 1}));
      appendToActiveAssistant(m => ({...m, events:[...(m.events || []), {kind:'tool', text:'🔧 开始调用：' + (data.tool || 'tool'), details:{event, tool:data.tool || '', arguments:data.arguments || {}, data}}]}));
    } else if (event === 'tool_call_result') {
      setStreamStats(prev => ({...prev, events: prev.events + 1}));
      appendToActiveAssistant(m => ({...m, events:[...(m.events || []), {kind:'tool', text:'🔧 ' + (data.ok ? '调用完成：' : '调用失败：') + (data.tool || 'tool'), details:{event, tool:data.tool || '', ok:!!data.ok, result:data.result, error:data.error || '', data}}]}));
    } else if (event === 'tool_confirmation_required') {
      setStreamStats(prev => ({...prev, events: prev.events + 1, state:'paused'}));
      appendToActiveAssistant(m => ({...m, events:[...(m.events || []), {kind:'confirm', text:'⏳ 等待确认工具：' + (data.tool || 'MCP 工具'), meta:'确认后模型会继续执行；拒绝则把拒绝结果返回给模型。', confirmation:data, status:'pending', details:{event, tool:data.tool || '', arguments:data.arguments || {}, data}}]}));
    } else if (event === 'tool_confirmation_resolved') {
      setStreamStats(prev => ({...prev, events: prev.events + 1, state:'streaming'}));
      appendToActiveAssistant(m => ({...m, events:(m.events || []).map(item => item.confirmation?.id === data.id ? {...item, status:'resolved', text:(data.approved ? '✅ 已允许工具：' : '⛔ 已拒绝工具：') + (data.tool || item.confirmation?.tool || 'MCP 工具')} : item)}));
    } else if (event === 'job_cancelled') {
      setStreamStats(prev => ({...prev, events: prev.events + 1, state:'stopping'}));
      appendToActiveAssistant(m => ({...m, events:[...(m.events || []), {kind:'tool', text:'⏹️ 已请求停止生成', details:{event, data}}]}));
    } else if (event === 'run_event') {
      const meta = [runStatusLabel(data.status || ''), data.server, data.action, fmtDuration(data.duration_ms)].filter(Boolean).join(' · ');
      setStreamStats(prev => ({...prev, events: prev.events + 1}));
      appendToActiveAssistant(m => ({...m, events:[...(m.events || []), {kind:'run', text:'🧭 ' + (data.summary || data.tool || 'MCP 工具事件'), meta, details:{event, tool:data.tool || '', arguments:data.arguments, result:data.result, error:data.error || '', duration_ms:data.duration_ms, data}}]}));
    } else if (event === 'run_finish') {
      loadRuns().catch(() => {});
      loadAgentTasks().catch(() => {});
    } else if (event === 'done') {
      setFinalSession(data.session);
      activeJobIDRef.current = '';
      setActiveJobID('');
      setStreamStats(prev => ({...prev, state:'done'}));
    } else if (event === 'error') {
      throw new Error(data.message || 'stream error');
    }
  }, [appendAnswer, appendReasoning, appendToActiveAssistant, loadRuns, loadAgentTasks]);

  useEffect(() => {
    if (!current || busy) return;
    let stopped = false;
    const abort = new AbortController();
    async function resumeRunningJob() {
      try {
        const list = await fetchChatJobs(api, current);
        if (stopped) return;
        const job = (list.jobs || []).find(j => j.status === 'running');
        if (!job) return;
        setBusy(true);
        activeJobIDRef.current = job.id || '';
        setActiveJobID(job.id || '');
        setStreamPaused(false);
        pausedRef.current = false;
        pendingDeltaRef.current = '';
        pendingReasoningRef.current = '';
        abortRef.current = abort;
        setStreamStats({state:'streaming', started_at:Date.now(), chars:0, events:0, tools:0, error:''});
        setMessages(prev => prev.some(m => m.role === 'assistant-stream') ? prev : [...prev, {role:'assistant-stream', answer:'', reasoning:'', events:[{kind:'tool', text:'↩️ 已恢复后台生成', details:{event:'resume_running_job', job}}]}]);
        let finalSession = null;
        await streamChatJobEvents({jobID: job.id, authHeaders, signal: abort.signal, onEvent: (event, data) => handleChatStreamEvent(event, data, s => { finalSession = s; })});
        if (finalSession && !stopped) {
          pendingDeltaRef.current = '';
          pendingReasoningRef.current = '';
          setMessages(finalSession.messages || []);
          setCurrentTitle(finalSession.title || currentTitle || '新会话');
          await loadSessions();
          await Promise.allSettled([loadRuns(), loadAgentTasks()]);
        }
      } catch (e) {
        if (!abort.signal.aborted && !stopped) {
          const message = readableChatError(e);
          setStreamStats(prev => ({...prev, state:'error', error:message}));
          appendToActiveAssistant(m => ({...m, answer:'错误：' + message}));
        }
      } finally {
        if (!stopped) {
          setBusy(false);
          abortRef.current = null;
          activeJobIDRef.current = '';
          setActiveJobID('');
          setStreamPaused(false);
        }
      }
    }
    resumeRunningJob();
    return () => { stopped = true; abort.abort(); };
  }, [current, api, authHeaders, handleChatStreamEvent, appendToActiveAssistant, currentTitle, loadSessions, loadRuns, loadAgentTasks]);

  const activePrompt = useMemo(() => prompts.find(p => p.active) || prompts[0] || null, [prompts]);
  const draftKey = useMemo(() => 'chatdock.draft.' + encodeURIComponent(activePrompt?.name || 'default') + '.' + encodeURIComponent(current || 'new'), [activePrompt?.name, current]);


  const providerChoices = useMemo(() => providers.map(provider => {
    const id = providerChoiceID(provider);
    const models = uniqueModelNames([...(provider.models || []), provider.default_model].filter(Boolean));
    return {...provider, choice_id: id, models};
  }).filter(provider => provider.choice_id && (provider.enabled || provider.models.length)), [providers]);
  const selectedModelProvider = useMemo(() => {
    const activeID = activePrompt?.name || '';
    return providerChoices.find(provider => provider.choice_id === chatModel.provider_id)
      || providerChoices.find(provider => provider.choice_id === activeID)
      || providerChoices[0]
      || null;
  }, [activePrompt?.name, chatModel.provider_id, providerChoices]);
  const selectedChatModel = chatModel.model || selectedModelProvider?.default_model || selectedModelProvider?.models?.[0] || config.model || '';
  const selectedModelBaseURL = selectedModelProvider?.base_url || config.base_url || '';

  useEffect(() => {
    if (!providerChoices.length) return;
    const stillValid = providerChoices.some(provider => provider.choice_id === chatModel.provider_id && (!chatModel.model || provider.models.includes(chatModel.model)));
    if (stillValid) return;
    const activeID = activePrompt?.name || '';
    const provider = providerChoices.find(item => item.choice_id === activeID) || providerChoices[0];
    setChatModel({provider_id: provider.choice_id, model: provider.default_model || provider.models[0] || config.model || ''});
  }, [activePrompt?.name, chatModel.model, chatModel.provider_id, config.model, providerChoices]);

  const selectChatModel = useCallback((provider, modelName) => {
    setChatModel({provider_id: provider.choice_id, model: modelName});
    setModelPickerOpen(false);
    showToast('已切换模型：' + providerLabel(provider) + ' · ' + modelName, 'success');
  }, [showToast]);

  useEffect(() => {
    setInput(localStorage.getItem(draftKey) || '');
  }, [draftKey]);

  useEffect(() => {
    if (busy) return;
    const text = input.trim();
    if (text) localStorage.setItem(draftKey, input);
    else localStorage.removeItem(draftKey);
  }, [busy, draftKey, input]);

  const readyAttachments = useMemo(() => pendingAttachments.filter(item => item.id && !item.uploading && !item.error && !String(item.id).startsWith('local_')), [pendingAttachments]);
  const pendingAttachmentIDs = useMemo(() => readyAttachments.map(item => item.id), [readyAttachments]);

  const removePendingAttachment = useCallback((id) => {
    setPendingAttachments(prev => prev.filter(item => item.id !== id));
  }, []);

  const downloadAttachment = useCallback(async (item) => {
    if (!item?.id || String(item.id).startsWith('local_')) return;
    try {
      const response = await fetch('/api/files/' + encodeURIComponent(item.id), {headers: authHeaders()});
      if (!response.ok) throw new Error('HTTP ' + response.status);
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = item.name || 'attachment';
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      showToast('附件下载失败：' + e.message, 'error');
    }
  }, [authHeaders, showToast]);

  const handleFileSelect = useCallback(async (event) => {
    const files = Array.from(event.target.files || []);
    event.target.value = '';
    if (!files.length) return;
    if (busy) {
      showToast('当前回复还在进行中，请稍后再上传。', 'error');
      return;
    }
    let sessionID = current;
    if (!sessionID) {
      const s = await createPersistedSession();
      if (!s) return;
      sessionID = s.id;
    }
    setUploadingFiles(true);
    try {
      for (const file of files) {
        const localID = 'local_' + Date.now() + '_' + Math.random().toString(16).slice(2);
        setPendingAttachments(prev => [...prev, {id: localID, name: file.name || 'upload', size: file.size || 0, mime_type: file.type || 'application/octet-stream', status: 'uploading', uploading: true, progress: 0}]);
        try {
          const data = await uploadFileRequest(file, sessionID, authHeaders, progress => {
            setPendingAttachments(prev => prev.map(item => item.id === localID ? {...item, progress} : item));
          });
          setPendingAttachments(prev => prev.map(item => item.id === localID ? {...data.attachment, progress: 100} : item));
        } catch (e) {
          if (e.status === 401) setAuthPage(e);
          setPendingAttachments(prev => prev.map(item => item.id === localID ? {...item, uploading: false, error: e.message || '上传失败', status: 'failed'} : item));
          showToast('上传失败：' + (e.message || '未知错误'), 'error');
        }
      }
    } finally {
      setUploadingFiles(false);
    }
  }, [authHeaders, busy, createPersistedSession, current, setAuthPage, showToast]);

  const sendMsg = useCallback(async (overrideText) => {
    if (busy) return;
    const text = (overrideText ?? input).trim();
    const attachmentIDs = pendingAttachmentIDs;
    const attachmentsForMessage = readyAttachments;
    if (!text && !attachmentIDs.length) {
      inputRef.current?.focus();
      return;
    }
    if (uploadingFiles || pendingAttachments.some(item => item.uploading)) {
      showToast('文件还在上传，请上传完成后再发送。', 'error');
      return;
    }
    if (!String(selectedModelBaseURL || '').trim() || !String(selectedChatModel || '').trim()) {
      showToast('请先配置模型供应商和模型，再发送消息。', 'error');
      openSettings('model');
      return;
    }
    let sessionID = current;
    if (!sessionID) {
      const s = await createPersistedSession({refreshList:false});
      if (!s) return;
      sessionID = s.id;
    }
    localStorage.removeItem(draftKey);
    setInput('');
    setBusy(true);
    setStreamPaused(false);
    setStreamStats({state:'connecting', started_at:Date.now(), chars:0, events:0, tools:0, error:''});
    pausedRef.current = false;
    pendingDeltaRef.current = '';
    pendingReasoningRef.current = '';
    const abort = new AbortController();
    abortRef.current = abort;
    forceScrollRef.current = true;
    stickToBottomRef.current = true;
    setMessages(prev => [...prev, {role:'user', content:text, attachments:attachmentsForMessage}, {role:'assistant-stream', answer:'', reasoning:'', events:[]}]);
    try {
      setPendingAttachments([]);
      setStreamStats(prev => ({...prev, state:'streaming'}));
      let finalSession = null;
      await streamChat({authHeaders, signal: abort.signal, sessionID, message:text, attachmentIDs, providerID: selectedModelProvider?.choice_id || '', model: selectedChatModel, onEvent: (event, data) => handleChatStreamEvent(event, data, s => { finalSession = s; })});
      if (finalSession) {
        pendingDeltaRef.current = '';
        pendingReasoningRef.current = '';
        setMessages(finalSession.messages || []);
        setCurrentTitle(finalSession.title || currentTitle || '新会话');
        await loadSessions();
        await Promise.allSettled([loadRuns(), loadAgentTasks()]);
      }
    } catch (e) {
      if (abort.signal.aborted) {
        appendReasoning(pendingReasoningRef.current);
        appendAnswer(pendingDeltaRef.current);
        appendAnswer('\n\n【已中断】');
        await loadSessions().catch(() => {});
      } else {
        const message = readableChatError(e, attachmentsForMessage.some(attachmentLooksLikeImage));
        setStreamStats(prev => ({...prev, state:'error', error:message}));
        appendToActiveAssistant(m => ({...m, answer:'错误：' + message}));
      }
    } finally {
      setBusy(false);
      abortRef.current = null;
      activeJobIDRef.current = '';
      setActiveJobID('');
      setStreamPaused(false);
    }
  }, [authHeaders, busy, selectedModelBaseURL, selectedChatModel, selectedModelProvider, current, currentTitle, draftKey, input, pendingAttachmentIDs, pendingAttachments, readyAttachments, uploadingFiles, createPersistedSession, loadSessions, appendAnswer, appendReasoning, appendToActiveAssistant, handleChatStreamEvent, loadRuns, loadAgentTasks, openSettings, showToast]);

  const toggleStreamPause = useCallback(() => {
    if (!busy) return;
    setStreamPaused(prev => {
      const next = !prev;
      pausedRef.current = next;
      setStreamStats(current => ({...current, state: next ? 'paused' : 'streaming'}));
      if (!next) {
        appendReasoning(pendingReasoningRef.current);
        appendAnswer(pendingDeltaRef.current);
        pendingReasoningRef.current = '';
        pendingDeltaRef.current = '';
      }
      return next;
    });
  }, [appendAnswer, appendReasoning, busy]);

  const stopStreaming = useCallback(async () => {
    if (!busy) return;
    setStreamStats(prev => ({...prev, state:'stopping'}));
    const jobID = activeJobIDRef.current || activeJobID;
    if (jobID) {
      try { await cancelChatJob(api, jobID); } catch (e) { showToast('停止生成失败：' + e.message, 'error'); }
    }
    if (abortRef.current) abortRef.current.abort();
  }, [activeJobID, api, busy, showToast]);

  const resolveToolConfirmation = useCallback(async (id, approve) => {
    try {
      await resolveMCPConfirmation(api, id, approve);
      appendToActiveAssistant(m => ({...m, events:(m.events || []).map(item => item.confirmation?.id === id ? {...item, status:'resolved', text:(approve ? '✅ 已允许工具：' : '⛔ 已拒绝工具：') + (item.confirmation?.tool || 'MCP 工具')} : item)}));
      showToast(approve ? '已允许工具执行' : '已拒绝工具执行', approve ? 'success' : 'info');
    } catch (e) {
      showToast('确认工具失败：' + e.message, 'error');
    }
  }, [api, appendToActiveAssistant, showToast]);

  const createWorkspace = useCallback(async () => {
    if (busy) return;
    const values = await showDialog({title:'新增工作空间', confirmText:'创建工作空间', fields:[
      {name:'name', label:'工作空间名称', value:'', required:true},
      {name:'system_prompt', label:'系统提示词内容', type:'textarea', rows:5, value:config.system_prompt || ''},
    ]});
    if (!values || !values.name.trim()) return;
    await createWorkspaceRecord(api, {name: values.name.trim(), system_prompt: values.system_prompt || ''});
    setCurrent(null);
    setCurrentTitle('未选择会话');
    setMessages([{role:'empty', content:'已创建并切换到新工作空间。'}]);
    setPendingAttachments([]);
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig(), loadMCPConfig(), loadSkills(), loadScheduledTasks(), loadSessions()]);
    closeSidebarOnMobile();
    showToast('工作空间已创建', 'success');
  }, [api, busy, closeSidebarOnMobile, config.system_prompt, loadConfig, loadMCPConfig, loadPrompts, loadScheduledTasks, loadSessions, loadSkills, refreshProductState, showDialog, showToast]);

  const deleteWorkspace = useCallback(async (id, name) => {
    const ok = await showDialog({title:'删除工作空间', message:'确定删除工作空间「' + (name || id) + '」？这会删除该工作空间下的配置、技能、任务和会话。若删除当前工作空间，会自动切换到默认工作空间。', confirmText:'删除', danger:true, type:'confirm'});
    if (!ok) return;
    const data = await deleteWorkspaceRecord(api, id);
    setWorkspaces(data.workspaces || []);
    setCurrent(null);
    setCurrentTitle('未选择会话');
    setMessages([{role:'empty', content:'工作空间已删除。当前工作空间：' + (data.active || 'default')}]);
    setPendingAttachments([]);
    await Promise.allSettled([loadPrompts(), loadConfig(), loadMCPConfig(), loadSkills(), loadScheduledTasks(), loadSessions(), loadSetupStatus(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
    showToast('工作空间已删除', 'success');
  }, [api, loadConfig, loadDataStatus, loadMCPConfig, loadModelProviders, loadPrompts, loadScheduledTasks, loadSessions, loadSetupStatus, loadSkills, loadSystemStatus, showDialog, showToast]);

  const saveConfig = useCallback(async () => {
    const workspaceID = (prompts.find(p => p.active) || {}).name || 'default';
    await saveWorkspaceConfig(api, workspaceID, {
      base_url: config.base_url,
      api_key: config.api_key,
      model: config.model,
      models: uniqueModelNames(config.models?.length ? config.models : config.model),
      system_prompt: config.system_prompt,
      context_mode: config.context_mode || 'auto',
      max_context_messages: Number(config.max_context_messages || 12),
      temperature: Number(config.temperature || 0.7),
      enable_thinking: !!config.enable_thinking,
      hide_thinking: !!config.hide_thinking,
    });
    setConfig(c => ({...c, api_key:''}));
    await loadConfig();
    await Promise.allSettled([loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadSystemStatus()]);
    showToast('已保存到工作空间：' + workspaceID, 'success');
  }, [api, config, loadConfig, loadModelProviders, loadSetupStatus, loadSystemStatus, loadWorkspaces, prompts, showToast]);

  const showPromptPreview = useCallback(async () => {
    const workspaceID = (prompts.find(p => p.active) || {}).name || 'default';
    const data = await fetchPromptPreview(api, workspaceID);
    setPromptPreview(data.content || '(空)');
  }, [api, prompts]);

  const runSetupWizard = useCallback(async () => {
    const values = await showDialog({title:'首次配置', message:'配置默认工作空间和模型后即可开始对话。', confirmText:'完成初始化', fields:[
      {name:'workspace_name', label:'默认工作空间名称', value:'default', required:true},
      {name:'base_url', label:'模型 Base URL', value:config.base_url || 'https://api.openai.com/v1', required:true},
      {name:'model', label:'默认模型', value:config.model || 'gpt-4o-mini', required:true},
      {name:'api_key', label:'API Key（可留空）', type:'password', value:''},
      {name:'system_prompt', label:'默认 System Prompt', type:'textarea', rows:4, value:config.system_prompt || '你是 ChatDock，本地优先 AI 工作台。默认用中文回答。'},
    ]});
    if (!values) return;
    await initializeSetup(api, values);
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig()]);
    showToast('初始化完成', 'success');
  }, [api, config, loadConfig, loadPrompts, refreshProductState, showDialog, showToast]);

  const testModelProvider = useCallback(async () => {
    try {
      const data = await testModelProviderRequest(api, {
        base_url: config.base_url,
        api_key: config.api_key,
        model: config.model,
        system_prompt: config.system_prompt,
        context_mode: config.context_mode || 'auto',
        max_context_messages: Number(config.max_context_messages || 12),
        temperature: Number(config.temperature || 0.7),
        enable_thinking: !!config.enable_thinking,
        hide_thinking: !!config.hide_thinking,
      });
      showToast(data.ok ? '模型连接正常：' + (data.model || '') : '模型连接失败：' + (data.error || 'unknown'), data.ok ? 'success' : 'error');
    } catch (e) { showToast('模型连接失败：' + e.message, 'error'); }
  }, [api, config, showToast]);

  const fetchProviderModels = useCallback(async () => {
    setLoadingModels(true);
    try {
      const data = await fetchProviderModelsRequest(api, {
        base_url: config.base_url,
        api_key: config.api_key,
        model: config.model,
        system_prompt: config.system_prompt,
        context_mode: config.context_mode || 'auto',
        max_context_messages: Number(config.max_context_messages || 12),
        temperature: Number(config.temperature || 0.7),
        enable_thinking: !!config.enable_thinking,
        hide_thinking: !!config.hide_thinking,
      });
      const models = data.models || [];
      setAvailableModels(models);
      showToast(models.length ? '已获取 ' + models.length + ' 个模型' : '接口可用，但没有返回模型名称', models.length ? 'success' : 'warn');
    } catch (e) {
      setAvailableModels([]);
      showToast('获取模型列表失败：' + e.message, 'error');
    } finally {
      setLoadingModels(false);
    }
  }, [api, config, showToast]);

  const saveMCPConfig = useCallback(async () => {
    try { JSON.parse(mcpConfig || '{}'); } catch (e) { showToast('MCP 配置不是合法 JSON：' + e.message, 'error'); return; }
    const c = await saveMCPConfigRequest(api, mcpConfig);
    setMcpConfig(c.content || mcpConfig);
    await loadMCPStatus().catch(() => {});
    showToast('MCP 配置已保存', 'success');
  }, [api, loadMCPStatus, mcpConfig, showToast]);

  const testMCP = useCallback(async (serverName = '') => {
    try {
      const data = await testMCPServer(api, serverName);
      const name = data.server || serverName || '默认 MCP';
      showToast(data.ok ? 'MCP 连接正常：' + name + '，工具数 ' + data.tool_count : 'MCP 连接失败：' + name + '，' + (data.error || 'unknown error'), data.ok ? 'success' : 'error');
    } catch (e) { showToast('MCP 测试失败：' + e.message, 'error'); }
  }, [api, showToast]);

  const editSkill = useCallback(async (id) => {
    if (busy) return;
    const existing = id ? skills.find(s => s.id === id) : null;
    const values = await showDialog({title: existing ? '编辑技能' : '新增技能', confirmText: existing ? '保存技能' : '新增技能', fields:[
      {name:'name', label:'技能名称', value: existing ? existing.name : '', required:true},
      {name:'description', label:'技能描述（可选）', type:'textarea', rows:2, value: existing ? (existing.description || '') : ''},
      {name:'content', label:'技能内容', type:'textarea', rows:8, value: existing ? (existing.content || '') : '', required:true},
    ]});
    if (!values) return;
    if (!values.name.trim() || !values.content.trim()) { showToast('技能名称和内容不能为空', 'error'); return; }
    const payload = {name: values.name.trim(), description: values.description || '', content: values.content.trim(), enabled: existing ? !!existing.enabled : true};
    const data = await saveSkillRecord(api, existing, payload);
    setSkills(data.skills || []);
    showToast(existing ? '技能已保存' : '技能已新增', 'success');
  }, [api, busy, skills, showDialog, showToast]);

  const toggleSkill = useCallback(async (id, enabled) => {
    const existing = skills.find(s => s.id === id);
    if (!existing) return;
    const data = await saveSkillRecord(api, existing, {name: existing.name, description: existing.description || '', content: existing.content || '', enabled: !!enabled});
    setSkills(data.skills || []);
  }, [api, skills]);

  const deleteSkill = useCallback(async (id) => {
    const existing = skills.find(s => s.id === id);
    if (!existing) return;
    const ok = await showDialog({title:'删除技能', message:'确定删除技能「' + existing.name + '」？此操作不可恢复。', confirmText:'删除', danger:true, type:'confirm'});
    if (!ok) return;
    const data = await deleteSkillRecord(api, id);
    setSkills(data.skills || []);
    showToast('技能已删除', 'success');
  }, [api, showDialog, showToast, skills]);

  const editScheduledTask = useCallback(async (id) => {
    if (busy) return;
    const existing = id ? scheduledTasks.find(t => t.id === id) : null;
    const values = await showDialog({title: existing ? '编辑自动化任务' : '新增自动化任务', message:'选择调度类型后，只需要填写对应的时间字段。', confirmText: existing ? '保存任务' : '新增任务', fields:[
      {name:'title', label:'任务标题', value: existing ? existing.title : '', required:true},
      {name:'prompt', label:'任务提示词', type:'textarea', rows:6, value: existing ? (existing.prompt || '') : '', required:true},
      {name:'schedule_type', label:'调度类型', type:'select', value: existing ? existing.schedule_type : 'once', options:[{value:'once', label:'一次性'}, {value:'daily', label:'每天固定时间'}, {value:'interval', label:'按分钟间隔'}]},
      {name:'run_at', label:'一次性运行时间', type:'datetime-local', value: existing && existing.run_at ? existing.run_at.slice(0,16) : defaultRunAtValue(), showWhen:{schedule_type:'once'}},
      {name:'time_of_day', label:'每天运行时间', type:'time', value: existing ? (existing.time_of_day || '09:00') : '09:00', showWhen:{schedule_type:'daily'}},
      {name:'interval_minutes', label:'间隔分钟数', type:'number', min:1, step:1, value: existing && existing.interval_minutes ? String(existing.interval_minutes) : '60', showWhen:{schedule_type:'interval'}, hint:'当前本地调度器最低按分钟执行；过短间隔会更频繁占用模型额度。'},
    ]});
    if (!values) return;
    const titleValue = (values.title || '').trim();
    const promptValue = (values.prompt || '').trim();
    const typeValue = (values.schedule_type || '').trim().toLowerCase();
    if (!titleValue || !promptValue) { showToast('任务标题和提示词不能为空', 'error'); return; }
    if (!['once','daily','interval'].includes(typeValue)) { showToast('调度类型只能是 once、daily 或 interval', 'error'); return; }
    const payload = {title:titleValue, prompt:promptValue, enabled: existing ? !!existing.enabled : true, schedule_type:typeValue};
    if (typeValue === 'once') payload.run_at = values.run_at || '';
    if (typeValue === 'daily') payload.time_of_day = values.time_of_day || '';
    if (typeValue === 'interval') payload.interval_minutes = Math.floor(Number(values.interval_minutes || 0));
    const data = await saveScheduledTaskRecord(api, existing, payload);
    setScheduledTasks(data.tasks || []);
    showToast(existing ? '任务已保存' : '任务已新增', 'success');
  }, [api, busy, scheduledTasks, showDialog, showToast]);

  const toggleScheduledTask = useCallback(async (id, enabled) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    const payload = {title: existing.title, prompt: existing.prompt, enabled: !!enabled, schedule_type: existing.schedule_type, run_at: existing.run_at || '', time_of_day: existing.time_of_day || '', interval_minutes: existing.interval_minutes || 0};
    const data = await saveScheduledTaskRecord(api, existing, payload);
    setScheduledTasks(data.tasks || []);
  }, [api, scheduledTasks]);

  const deleteScheduledTask = useCallback(async (id) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    const ok = await showDialog({title:'删除自动化任务', message:'确定删除定时任务「' + existing.title + '」？此操作不可恢复。', confirmText:'删除', danger:true, type:'confirm'});
    if (!ok) return;
    const data = await deleteScheduledTaskRecord(api, id);
    setScheduledTasks(data.tasks || []);
    showToast('任务已删除', 'success');
  }, [api, scheduledTasks, showDialog, showToast]);

  const runScheduledTaskNow = useCallback(async (id) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    const ok = await showDialog({title:'立即运行任务', message:'立即运行定时任务「' + existing.title + '」？', confirmText:'立即运行', type:'confirm'});
    if (!ok) return;
    try {
      const result = await runScheduledTask(api, id);
      await loadScheduledTasks();
      await loadSessions();
      await refreshProductState();
      if (result.session && result.session.id) {
        setCurrent(result.session.id);
        setCurrentTitle(result.session.title || '定时任务');
        setMessages(result.session.messages || []);
        if (window.location.pathname !== sessionPath(result.session.id)) window.history.pushState({chatdock:true}, '', sessionPath(result.session.id));
        closeSettings();
      }
      showToast('定时任务已运行', 'success');
    } catch (e) { await loadScheduledTasks().catch(() => {}); showToast('运行失败：' + e.message, 'error'); }
  }, [api, closeSettings, loadScheduledTasks, loadSessions, refreshProductState, scheduledTasks, showDialog, showToast]);

  const inspectToolEvent = useCallback(async (event) => {
    if (!event?.details) return;
    await showDialog({title:'工具事件详情', confirmText:'关闭', hideCancel:true, variant:'tool-event-modal', toolEventDetail:buildToolEventDetail(event)});
  }, [showDialog]);

  const showContextPreview = useCallback(async () => {
    if (!current) return;
    try {
      const preview = await fetchContextPreview(api, current);
      await showDialog({title:'上下文 / Token 预览', confirmText:'关闭', hideCancel:true, fields:[{name:'preview', label:'实际会发送给模型的上下文（token 为粗略估算）', type:'textarea', rows:16, value:contextPreviewText(preview)}]});
    } catch (e) {
      showToast('上下文预览失败：' + e.message, 'error');
    }
  }, [api, current, showDialog, showToast]);

  const continueAgentTask = useCallback((task) => {
    setInput('继续任务：' + (task.title || 'AgentDock 任务') + '\n任务 ID：' + task.id + '\n来源 Run：' + (task.source_run_id || ''));
    closeSettings();
    showToast('已填入继续任务指令，可直接发送。', 'success');
  }, [closeSettings, showToast]);

  const logout = useCallback(() => {
    localStorage.removeItem('chatdock.authToken');
    setAuthPage({message:'请输入 ChatDock 账号和密码。'});
  }, []);

  const filteredSessions = useMemo(() => {
    const q = sessionSearch.trim();
    if (q) return sessionSearchResults;
    return sessions;
  }, [sessionSearch, sessionSearchResults, sessions]);

  const currentSummary = useMemo(() => sessions.find(s => s.id === current) || null, [current, sessions]);
  const currentPinned = !!currentSummary?.pinned;
  const appClass = 'app ' + (sidebarCollapsed ? 'sidebar-collapsed ' : '') + (settingsOpen ? 'settings-open' : '');
  const productReady = setupStatus && !setupStatus.needs_setup;
  const modelReady = !!String(selectedModelBaseURL || '').trim() && !!String(selectedChatModel || '').trim();
  const productStatusText = setupStatus == null ? '加载中' : (productReady ? '就绪' : '待配置');
  const productStatusClass = setupStatus == null ? 'warn' : (productReady ? 'ok' : 'warn');
  const streamElapsed = streamStats.started_at ? Math.max(0, Math.round((Date.now() - streamStats.started_at) / 1000)) : 0;
  const streamStatsText = busy ? streamStatusText(streamStats, streamElapsed) : '';
  const inputStats = busy ? streamStatsText : (pendingAttachments.length ? pendingAttachments.length + ' 个附件 · ' + (input.trim() ? input.trim().length + ' 字' : '可直接发送') : (input.trim() ? input.trim().length + ' 字 · 草稿自动保存' : (modelReady ? 'Enter 发送 · Shift+Enter 换行 · 点击 + 上传文件' : '请先在配置中心完成模型 Base URL 和 Model')));
  const productDiagnostics = diagnosticsText({setupStatus, systemStatus, dataStatus, mcpStatus, providers});
  const hasVisibleChatMessages = messages.some(m => m.role !== 'empty');

  const quickActions = useMemo(() => [
    {id:'focus-input', title:'聚焦输入框', hint:'按 / 也可以快速输入', run:() => inputRef.current?.focus()},
    {id:'new-session', title:'新建会话', hint:'在当前工作空间开始新对话', disabled:busy, run:createSession},
    {id:'continue', title:'发送“继续”', hint:'让当前会话继续上一轮内容', disabled:busy, run:() => sendMsg('继续')},
    {id:'workspace-picker', title:'切换工作空间', hint:'加载不同模型、技能和会话', disabled:busy || !prompts.length, run:() => setWorkspacePickerOpen(true)},
    {id:'settings', title:'打开配置中心', hint:'工作空间、模型、工具和数据统一管理', run:() => openSettings()},
    {id:'settings-model', title:'模型设置', hint:'Base URL、API Key、模型和最终 Prompt', run:() => openSettings('model')},
    {id:'settings-tools', title:'工具中心', hint:'MCP 配置、状态检测和连接测试', run:() => openSettings('tools')},
    {id:'settings-automation', title:'自动化任务', hint:'创建、运行和暂停定时任务', run:() => openSettings('automation')},
    {id:'settings-data', title:'数据状态', hint:'数据库、工作空间和会话数量', run:() => openSettings('data')},
    {id:'copy-diagnostics', title:'复制诊断信息', hint:'复制脱敏后的系统、数据库、备份和 MCP 状态', run:() => copyText(productDiagnostics)},
    {id:'copy-session', title:'复制当前会话全文', hint:'复制为 Markdown', disabled:!current, run:copyCurrentMarkdown},
    {id:'export-session', title:'导出当前会话', hint:'下载 Markdown 文件', disabled:!current, run:exportCurrent},
    {id:'context-preview', title:'查看上下文 / Token 预览', hint:'查看实际发送给模型的消息构成', disabled:!current, run:showContextPreview},
    {id:'delete-session', title:'删除当前会话', hint:'删除后不可恢复', disabled:!current || busy, run:deleteCurrent},
    {id:'rename-session', title:'重命名当前会话', hint:'整理侧栏会话列表', disabled:!current || busy, run:renameCurrent},
    {id:'clone-session', title:'复制当前会话', hint:'保留上下文开一个副本', disabled:!current || busy, run:cloneCurrent},
    {id:'branch-session', title:'创建分支对话', hint:'在新聊天中从当前上下文继续', disabled:!current || busy || !messages.length, run:() => branchCurrent()},
    {id:'pin-session', title: currentPinned ? '取消置顶当前会话' : '置顶当前会话', hint:'让重要会话固定在列表顶部', disabled:!current, run:pinCurrent},
    {id:'theme', title:'切换明暗主题', hint:'当前：' + (theme === 'day' ? '白天' : '夜晚'), run:() => setThemeState(theme === 'day' ? 'night' : 'day')},
  ], [branchCurrent, busy, cloneCurrent, copyCurrentMarkdown, copyText, createSession, current, currentPinned, deleteCurrent, exportCurrent, messages.length, openSettings, pinCurrent, productDiagnostics, prompts.length, renameCurrent, sendMsg, showContextPreview, theme]);

  const settingsPanel = (
    <SettingsPanel
      activeModule={activeModule} busy={busy} closeSettings={closeSettings} config={config} continueAgentTask={continueAgentTask}
      createWorkspace={createWorkspace} dataStatus={dataStatus} deleteScheduledTask={deleteScheduledTask} deleteSkill={deleteSkill} deleteWorkspace={deleteWorkspace}
      editScheduledTask={editScheduledTask} editSkill={editSkill} loadAgentTasks={loadAgentTasks} loadDataStatus={loadDataStatus} loadMCPConfig={loadMCPConfig}
      loadMCPStatus={loadMCPStatus} loadRuns={loadRuns} loadScheduledTasks={loadScheduledTasks} loadSkills={loadSkills} loadSystemStatus={loadSystemStatus}
      mcpConfig={mcpConfig} mcpStatus={mcpStatus} onCopy={copyText} providers={providers} promptPreview={promptPreview} refreshProductState={refreshProductState} refreshVisibleSettings={refreshVisibleSettings}
      runScheduledTaskNow={runScheduledTaskNow} runSetupWizard={runSetupWizard} runs={runs} saveConfig={saveConfig} saveMCPConfig={saveMCPConfig}
      scheduledTasks={scheduledTasks} selectWorkspace={selectWorkspace} setConfig={setConfig} setMcpConfig={setMcpConfig} setTaskSearch={setTaskSearch}
      setupStatus={setupStatus} showPromptPreview={showPromptPreview} skillSearch={skillSearch} skills={skills} switchSettingsModule={switchSettingsModule}
      systemStatus={systemStatus} taskSearch={taskSearch} testMCP={testMCP} testModelProvider={testModelProvider} fetchProviderModels={fetchProviderModels} availableModels={availableModels} loadingModels={loadingModels} toggleScheduledTask={toggleScheduledTask}
      toggleSkill={toggleSkill} workspaces={workspaces} agentTasks={agentTasks} setSkillSearch={setSkillSearch} logout={logout}
    />
  );

  return <>
    <div id="sidebarMask" className={'sidebar-mask ' + (!settingsOpen && !sidebarCollapsed ? 'show' : '')} onClick={() => setSidebarCollapsed(true)} />
    <div className={'session-actions-backdrop ' + (sessionActionsOpen ? 'show' : '')} onClick={() => setSessionActionsOpen(false)}>
      <div className="session-actions-sheet" onClick={e => e.stopPropagation()}>
        <div className="session-actions-head"><b>会话操作</b><button className="secondary small" onClick={() => setSessionActionsOpen(false)}>关闭</button></div>
        <button className="secondary" disabled={!current} onClick={() => { setSessionActionsOpen(false); pinCurrent(); }}>{currentPinned ? '取消置顶' : '置顶会话'}</button>
        <button className="secondary" disabled={!current || busy} onClick={() => { setSessionActionsOpen(false); renameCurrent(); }}>重命名</button>
        <button className="secondary" disabled={!current} onClick={() => { setSessionActionsOpen(false); copyCurrentMarkdown(); }}>复制全文</button>
        <button className="secondary" disabled={!current || busy} onClick={() => { setSessionActionsOpen(false); cloneCurrent(); }}>复制会话</button>
        <button className="secondary" disabled={!current || busy || !messages.length} onClick={() => { setSessionActionsOpen(false); branchCurrent(); }}>创建分支对话</button>
        <button className="secondary" disabled={!current} onClick={() => { setSessionActionsOpen(false); exportCurrent(); }}>导出 Markdown</button>
        <button className="secondary" disabled={!current} onClick={() => { setSessionActionsOpen(false); showContextPreview(); }}>上下文 / Token</button>
        <button className="danger" disabled={!current || busy} onClick={() => { setSessionActionsOpen(false); deleteCurrent(); }}>删除会话</button>
      </div>
    </div>
    {settingsOpen ? <div id="settingsPage" className="settings-page">{settingsPanel}</div> : <div id="app" className={appClass}>
      <aside>
        <div className="sidebar-head">
          <div className="brand"><span className="brand-logo">✦</span><div className="brand-copy"><span className="brand-text">ChatDock</span><div className="sub">本地优先的 AI 工作台</div></div></div>
          <button id="sidebarToggle" className="sidebar-toggle" onClick={() => setSidebarCollapsed(!sidebarCollapsed)} title={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}>{sidebarCollapsed ? '›' : '‹'}</button>
        </div>
        <div className="prompt-box">
          <button className="workspace-picker-trigger" type="button" disabled={busy || !prompts.length} onClick={() => setWorkspacePickerOpen(true)}>
            <span className="workspace-picker-icon">▣</span>
            <span className="workspace-picker-name">{activePrompt ? (activePrompt.name === 'default' ? '默认工作区' : activePrompt.name) : '未选择'}</span>
            <span className="workspace-picker-meta">{activePrompt ? activePrompt.count : '暂无'}</span>
            <span className="workspace-picker-arrow">⌄</span>
          </button>
        </div>
        <div className="session-search-row">
          <label className="session-search-box"><span className="session-search-icon">⌕</span><input className="session-search" placeholder="搜索聊天记录" value={sessionSearch} onChange={e => setSessionSearch(e.target.value)} /></label>
          <button className="new" disabled={busy} onClick={newSession}><span className="new-symbol">+</span><span className="new-label">新会话</span></button>
        </div>
        {sessionSearch.trim() ? <div className="session-search-meta">{sessionSearchBusy ? '搜索中…' : '全文搜索 ' + filteredSessions.length + ' 条'}</div> : null}
        <div id="sessions">{filteredSessions.length ? filteredSessions.map(s => <div key={s.id} className={'session ' + (current === s.id ? 'active ' : '') + (s.pinned ? 'pinned' : '')} onClick={() => openSession(s.id)}><div className="session-main"><div className="session-title">{s.pinned ? <span className="pin-mark">置顶</span> : null}{s.title}</div>{s.match_snippet ? <div className="session-preview search-hit">{s.match_field ? s.match_field + '：' : ''}{s.match_snippet}</div> : (s.preview ? <div className="session-preview">{s.preview}</div> : null)}<div className="session-meta">{s.count} 条 · {fmtTime(s.updated_at)}</div></div><button type="button" className="session-delete" disabled={busy} onClick={e => { e.stopPropagation(); deleteSessionByID(s.id, s.title); }} aria-label={'删除会话 ' + (s.title || '')} title="删除会话">×</button></div>) : <div className="empty compact">{sessionSearch.trim() ? '没有匹配会话' : '暂无会话，开始新会话'}</div>}</div>
      </aside>
      <main>
        <div className="topbar">
          <div className="top-left"><button className="mobile-menu" onClick={() => setSidebarCollapsed(!sidebarCollapsed)}>☰</button><b id="title">{currentTitle}</b><span className={'status-pill ' + productStatusClass}>{productStatusText}</span></div>
          <div className="top-actions">
            <button className="secondary quick-palette-toggle" onClick={() => setQuickPaletteOpen(true)} title="快捷指令（⌘/Ctrl K）">快捷</button>
            <button className="secondary config-toggle" onClick={() => openSettings()} title="配置中心">配置</button>
            <button className="secondary session-actions-toggle mobile-new-toggle" onClick={newSession} disabled={busy} title="新会话">新会话</button>
            <button className="theme-toggle" onClick={() => setThemeState(theme === 'day' ? 'night' : 'day')}>{theme === 'day' ? '白天' : '夜晚'}</button>
            <button className="secondary" onClick={renameCurrent} disabled={!current || busy}>重命名</button>
            <button className="secondary" onClick={copyCurrentMarkdown} disabled={!current}>复制全文</button>
            <button className="secondary" onClick={cloneCurrent} disabled={!current || busy}>复制会话</button>
            <button className="secondary" onClick={pinCurrent} disabled={!current}>{currentPinned ? '取消置顶' : '置顶'}</button>
            <button className="secondary" onClick={exportCurrent} disabled={!current}>导出</button>
            <button className="secondary" onClick={showContextPreview} disabled={!current}>上下文</button>
            <button className="danger" onClick={deleteCurrent} disabled={!current || busy}>删除</button>
          </div>
        </div>
        <div className="messages" ref={messagesRef} onScroll={handleMessagesScroll}>{messages.length ? messages.map((m, i) => <MessageView key={i} message={m} messageIndex={i} onCopy={copyText} onBranch={!busy && current ? branchCurrent : null} onDownloadAttachment={downloadAttachment} hideThinking={!!config.hide_thinking} onResolveConfirmation={resolveToolConfirmation} onInspectToolEvent={inspectToolEvent} />) : <EmptyState createSession={createSession} openSettings={openSettings} openWorkspacePicker={() => setWorkspacePickerOpen(true)} busy={busy} hasWorkspaces={!!prompts.length} setInput={setInput} modelReady={modelReady} />}</div>
        {showJumpToLatest ? <button type="button" className="jump-latest" onClick={scrollToLatestModelMessage} aria-label="跳到最新模型消息" title="跳到最新模型消息">↓</button> : null}
        <div className="composer-shell">
        {pendingAttachments.length ? <AttachmentList attachments={pendingAttachments} removable={!busy} onRemove={removePendingAttachment} onDownload={downloadAttachment} /> : null}
        <div className="composer">
          <input ref={fileInputRef} type="file" multiple className="file-input" onChange={handleFileSelect} />
          <button className="secondary attach-control" disabled={busy || uploadingFiles} onClick={() => fileInputRef.current?.click()} title="上传文件">+</button>
          <ComposerModelPicker busy={busy} providers={providerChoices} selectedProvider={selectedModelProvider} selectedModel={selectedChatModel} open={modelPickerOpen} setOpen={setModelPickerOpen} selectModel={selectChatModel} openSettings={openSettings} />
          <button className="secondary quick-control" disabled={busy} onClick={() => sendMsg('继续')}>继续</button>
          {busy ? <button className="secondary stream-control" onClick={toggleStreamPause}>{streamPaused ? '继续' : '暂停'}</button> : null}
          {busy ? <button className="danger stream-control" onClick={stopStreaming}>中断</button> : null}
          <textarea ref={inputRef} id="input" value={input} onChange={e => setInput(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); sendMsg(); } }} placeholder="输入消息" />
          <button id="send" disabled={busy || uploadingFiles || (!input.trim() && !pendingAttachmentIDs.length) || !modelReady} onClick={() => sendMsg()} title={!modelReady ? '请先配置模型' : '发送'}>发送</button>
        </div>
        <div className="composer-meta">{inputStats}</div>
        </div>
      </main>

    </div>}
    <WorkspacePicker open={workspacePickerOpen} prompts={prompts} busy={busy} activeName={activePrompt?.name || ''} onClose={() => setWorkspacePickerOpen(false)} onSelect={selectWorkspace} />
    <QuickPalette open={quickPaletteOpen} actions={quickActions} onClose={() => setQuickPaletteOpen(false)} />
    {authPage ? <LoginPage api={api} error={authPage} refreshAfterLogin={refreshAfterLogin} setAuthPage={setAuthPage} /> : null}
    <DialogHost dialog={dialog} closeDialog={closeDialog} />
    {toast ? <div id="appToast" className={'app-toast show ' + toast.variant}>{toast.message}</div> : null}
  </>;
}
