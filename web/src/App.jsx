import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { renderMarkdown } from './lib/markdown.js';

const settingsModules = ['workspace', 'model', 'skills', 'tools', 'runs', 'agent', 'automation', 'data', 'security'];

function normalizeSettingsModule(name) {
  return settingsModules.includes(name) ? name : 'workspace';
}

function fmtTime(value) {
  if (!value) return '';
  try { return new Date(value).toLocaleString(); } catch { return ''; }
}

function fmtBytes(value) {
  const n = Number(value || 0);
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / 1024 / 1024).toFixed(1) + ' MB';
}

function fmtDuration(ms) {
  const n = Number(ms || 0);
  if (n <= 0) return '-';
  if (n < 1000) return n + 'ms';
  return (n / 1000).toFixed(1) + 's';
}

function runStatusLabel(status) {
  return ({running:'执行中', success:'成功', failed:'失败', completed:'已完成', blocked:'已阻塞', active:'进行中', matched:'已匹配'})[status] || (status || '未知');
}

function runStatusClass(status) {
  if (status === 'failed' || status === 'blocked') return 'error';
  if (status === 'running' || status === 'active') return 'warn';
  return 'ok';
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
  if (t.last_status === 'failed') return 'error';
  return t.enabled ? 'ok' : 'warn';
}

function defaultRunAtValue() {
  const d = new Date(Date.now() + 60 * 60 * 1000);
  const pad = n => String(n).padStart(2, '0');
  return d.getFullYear() + '-' + pad(d.getMonth()+1) + '-' + pad(d.getDate()) + 'T' + pad(d.getHours()) + ':' + pad(d.getMinutes());
}

function scheduleSummary(t) {
  const next = t.next_run_at ? fmtTime(t.next_run_at) : '未计划';
  const last = t.last_run_at ? fmtTime(t.last_run_at) : '未运行';
  let plan = '一次性：' + (t.run_at ? fmtTime(t.run_at) : next);
  if (t.schedule_type === 'daily') plan = '每天 ' + (t.time_of_day || '--:--');
  if (t.schedule_type === 'interval') plan = '每 ' + (t.interval_minutes || 0) + ' 分钟';
  return plan + ' · 下次：' + next + ' · 上次：' + last;
}

function settingsModuleFromPath() {
  const parts = window.location.pathname.split('/').filter(Boolean);
  if (parts[0] !== 'settings') return '';
  return normalizeSettingsModule(parts[1] || localStorage.getItem('chatdock.settingsModule') || 'workspace');
}

function sessionIDFromPath() {
  const parts = window.location.pathname.split('/').filter(Boolean);
  return parts[0] === 'sessions' && parts[1] ? parts[1] : '';
}

function sessionPath(id) {
  return '/sessions/' + encodeURIComponent(id);
}

function Markdown({ value, className = '' }) {
  return <div className={className} dangerouslySetInnerHTML={{__html: renderMarkdown(value || '')}} />;
}

function TextCard({ title, hint, badge, active, children }) {
  return <div className={'product-card ' + (active ? 'active' : '')}>
    <div className="product-card-head"><div><b>{title}</b>{hint ? <div className="hint">{hint}</div> : null}</div>{badge ? <span className="badge">{badge}</span> : null}</div>
    {children}
  </div>;
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

  const [setupStatus, setSetupStatus] = useState(null);
  const [workspaces, setWorkspaces] = useState([]);
  const [providers, setProviders] = useState([]);
  const [prompts, setPrompts] = useState([]);
  const [sessions, setSessions] = useState([]);
  const [sessionSearch, setSessionSearch] = useState('');
  const [current, setCurrent] = useState(null);
  const [currentTitle, setCurrentTitle] = useState('未选择会话');
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [busy, setBusy] = useState(false);
  const [streamPaused, setStreamPaused] = useState(false);
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
  const [config, setConfig] = useState({base_url:'', api_key:'', model:'', system_prompt:'', max_context_messages:12, temperature:0.7, enable_thinking:false, hide_thinking:true, has_api_key:false});

  const abortRef = useRef(null);
  const pausedRef = useRef(false);
  const pendingDeltaRef = useRef('');
  const pendingReasoningRef = useRef('');
  const messagesRef = useRef(null);

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

  const authHeaders = useCallback((extra={}) => {
    const token = localStorage.getItem('chatdock.authToken') || '';
    return token ? {'Authorization':'Bearer ' + token, ...extra} : extra;
  }, []);

  const authURL = useCallback((path) => {
    const token = localStorage.getItem('chatdock.authToken') || '';
    if (!token) return path;
    const sep = path.includes('?') ? '&' : '?';
    return path + sep + 'token=' + encodeURIComponent(token);
  }, []);

  const api = useCallback(async (path, opt={}) => {
    const res = await fetch(path, {...opt, headers: authHeaders({'Content-Type':'application/json', ...(opt.headers || {})})});
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      const err = new Error(data.error || res.statusText);
      err.status = res.status;
      err.path = path;
      if (res.status === 401 && path !== '/api/auth/login') setAuthPage(err);
      throw err;
    }
    return data;
  }, [authHeaders]);

  const loadPrompts = useCallback(async () => {
    const data = await api('/api/prompts');
    setPrompts(data.prompts || []);
  }, [api]);

  const loadSessions = useCallback(async () => {
    const list = await api('/api/sessions');
    setSessions(list || []);
  }, [api]);

  const loadConfig = useCallback(async () => {
    const c = await api('/api/config');
    setConfig({
      base_url: c.base_url || '',
      api_key: '',
      model: c.model || '',
      system_prompt: c.system_prompt || '',
      max_context_messages: c.max_context_messages || 12,
      temperature: c.temperature ?? 0.7,
      enable_thinking: !!c.enable_thinking,
      hide_thinking: c.hide_thinking !== false,
      has_api_key: !!c.has_api_key,
    });
  }, [api]);

  const loadMCPConfig = useCallback(async () => {
    const c = await api('/api/mcp-config');
    setMcpConfig(c.content || '{\n  "servers": {}\n}\n');
  }, [api]);

  const loadSetupStatus = useCallback(async () => {
    const data = await api('/api/setup/status');
    setSetupStatus(data);
  }, [api]);

  const loadWorkspaces = useCallback(async () => {
    const data = await api('/api/workspaces');
    setWorkspaces(data.workspaces || []);
  }, [api]);

  const loadModelProviders = useCallback(async () => {
    const data = await api('/api/model-providers');
    setProviders(data.providers || []);
  }, [api]);

  const loadSkills = useCallback(async () => {
    const data = await api('/api/skills');
    setSkills(data.skills || []);
  }, [api]);

  const loadScheduledTasks = useCallback(async () => {
    const data = await api('/api/scheduled-tasks');
    setScheduledTasks(data.tasks || []);
  }, [api]);

  const loadDataStatus = useCallback(async () => {
    const data = await api('/api/data/status');
    setDataStatus(data);
  }, [api]);

  const loadSystemStatus = useCallback(async () => {
    const data = await api('/api/system/status');
    setSystemStatus(data);
  }, [api]);

  const loadMCPStatus = useCallback(async () => {
    const data = await api('/api/mcp/status');
    setMcpStatus(data.servers || []);
  }, [api]);

  const loadRuns = useCallback(async () => {
    const data = await api('/api/runs?limit=80');
    setRuns(data.runs || []);
  }, [api]);

  const loadAgentTasks = useCallback(async () => {
    const data = await api('/api/agent-tasks?limit=80');
    setAgentTasks(data.tasks || []);
  }, [api]);

  const refreshProductState = useCallback(async () => {
    await Promise.allSettled([loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
  }, [loadSetupStatus, loadWorkspaces, loadModelProviders, loadDataStatus, loadSystemStatus]);

  const loadSessionFromRoute = useCallback(async (id) => {
    if (!id) return false;
    const s = await api('/api/sessions/' + encodeURIComponent(id));
    setCurrent(s.id);
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
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

  useEffect(() => {
    if (messagesRef.current) messagesRef.current.scrollTop = messagesRef.current.scrollHeight;
  }, [messages]);

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
      else if (sessionActionsOpen) setSessionActionsOpen(false);
      else if (workspacePickerOpen) setWorkspacePickerOpen(false);
      else if (settingsOpen) closeSettings();
    }
    window.addEventListener('keydown', closeTopLayer);
    return () => window.removeEventListener('keydown', closeTopLayer);
  }, [closeDialog, closeSettings, dialog, sessionActionsOpen, settingsOpen, workspacePickerOpen]);

  const selectWorkspace = useCallback(async (name) => {
    if (busy || !name) return;
    setWorkspacePickerOpen(false);
    await api('/api/workspaces/' + encodeURIComponent(name) + '/select', {method:'POST', body:'{}'});
    setCurrent(null);
    setCurrentTitle('未选择会话');
    setMessages([{role:'empty', content:'已切换工作空间。创建或选择一个会话。'}]);
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig(), loadMCPConfig(), loadSkills(), loadScheduledTasks(), loadSessions()]);
    if (window.location.pathname !== '/') window.history.pushState({chatdock:true}, '', '/');
    closeSidebarOnMobile();
  }, [api, busy, refreshProductState, loadPrompts, loadConfig, loadMCPConfig, loadSkills, loadScheduledTasks, loadSessions, closeSidebarOnMobile]);

  const createSession = useCallback(async () => {
    const s = await api('/api/sessions', {method:'POST', body:'{}'});
    setCurrent(s.id);
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    await loadSessions();
    if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({chatdock:true}, '', sessionPath(s.id));
    closeSidebarOnMobile();
    return s;
  }, [api, loadSessions, closeSidebarOnMobile]);

  const openSession = useCallback(async (id) => {
    setCurrent(id);
    const s = await api('/api/sessions/' + encodeURIComponent(id));
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    await loadSessions();
    if (window.location.pathname !== sessionPath(id)) window.history.pushState({chatdock:true}, '', sessionPath(id));
    closeSidebarOnMobile();
  }, [api, loadSessions, closeSidebarOnMobile]);

  const newSession = useCallback(async () => { await createSession(); }, [createSession]);

  const renameCurrent = useCallback(async () => {
    if (!current) return;
    const values = await showDialog({title:'重命名会话', confirmText:'保存标题', fields:[{name:'title', label:'新的会话标题', value:currentTitle || '', required:true}]});
    if (!values || !values.title.trim()) return;
    const s = await api('/api/sessions/' + current + '/rename', {method:'POST', body: JSON.stringify({title: values.title.trim()})});
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    await loadSessions();
    showToast('会话标题已保存', 'success');
  }, [api, current, currentTitle, loadSessions, showDialog, showToast]);

  const deleteCurrent = useCallback(async () => {
    if (!current) return;
    const ok = await showDialog({title:'删除当前会话', message:'确定删除当前会话？此操作不可恢复。', confirmText:'删除', danger:true, type:'confirm'});
    if (!ok) return;
    await api('/api/sessions/' + current, {method:'DELETE'});
    setCurrent(null);
    setCurrentTitle('未选择会话');
    setMessages([]);
    if (window.location.pathname !== '/') window.history.pushState({chatdock:true}, '', '/');
    await loadSessions();
    showToast('会话已删除', 'success');
  }, [api, current, loadSessions, showDialog, showToast]);

  const exportCurrent = useCallback(() => {
    if (!current) return;
    window.open(authURL('/api/sessions/' + current + '/export?format=md'), '_blank');
  }, [authURL, current]);

  const copyCurrentMarkdown = useCallback(async () => {
    if (!current) return;
    try {
      const res = await fetch('/api/sessions/' + encodeURIComponent(current) + '/export?format=md', {headers: authHeaders()});
      if (!res.ok) throw new Error(await res.text() || res.statusText);
      await copyText(await res.text());
    } catch (e) {
      showToast('复制全文失败：' + e.message, 'error');
    }
  }, [authHeaders, copyText, current, showToast]);

  const cloneCurrent = useCallback(async () => {
    if (!current || busy) return;
    try {
      const s = await api('/api/sessions/' + encodeURIComponent(current) + '/clone', {method:'POST', body:'{}'});
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

  const sendMsg = useCallback(async (overrideText) => {
    if (busy) return;
    const text = (overrideText ?? input).trim();
    if (!text) return;
    let sessionID = current;
    if (!sessionID) {
      const s = await createSession();
      sessionID = s.id;
    }
    setInput('');
    setBusy(true);
    setStreamPaused(false);
    pausedRef.current = false;
    pendingDeltaRef.current = '';
    pendingReasoningRef.current = '';
    const abort = new AbortController();
    abortRef.current = abort;
    setMessages(prev => [...prev, {role:'user', content:text}, {role:'assistant-stream', answer:'', reasoning:'', events:[]}]);
    try {
      const res = await fetch('/api/chat/stream', {
        method:'POST',
        headers: authHeaders({'Content-Type':'application/json'}),
        body: JSON.stringify({session_id: sessionID, message:text}),
        signal: abort.signal,
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || res.statusText);
      }
      let finalSession = null;
      await readSSE(res, (event, data) => {
        if (event === 'delta') {
          const reasoning = data.reasoning_content || '';
          const content = data.content || '';
          if (pausedRef.current) {
            pendingReasoningRef.current += reasoning;
            pendingDeltaRef.current += content;
          } else {
            appendReasoning(reasoning);
            appendAnswer(content);
          }
        } else if (event === 'tool_call_start') {
          appendToActiveAssistant(m => ({...m, events:[...(m.events || []), {kind:'tool', text:'🔧 开始调用：' + (data.tool || 'tool')}]}));
        } else if (event === 'tool_call_result') {
          appendToActiveAssistant(m => ({...m, events:[...(m.events || []), {kind:'tool', text:'🔧 ' + (data.ok ? '调用完成：' : '调用失败：') + (data.tool || 'tool')}]}));
        } else if (event === 'run_event') {
          const meta = [runStatusLabel(data.status || ''), data.server, data.action, fmtDuration(data.duration_ms)].filter(Boolean).join(' · ');
          appendToActiveAssistant(m => ({...m, events:[...(m.events || []), {kind:'run', text:'🧭 ' + (data.summary || data.tool || 'MCP 工具事件'), meta}]}));
        } else if (event === 'run_finish') {
          loadRuns().catch(() => {});
          loadAgentTasks().catch(() => {});
        } else if (event === 'done') {
          finalSession = data.session;
        } else if (event === 'error') {
          throw new Error(data.message || 'stream error');
        }
      });
      if (finalSession) {
        pendingDeltaRef.current = '';
        pendingReasoningRef.current = '';
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
        appendToActiveAssistant(m => ({...m, answer:'错误：' + e.message}));
      }
    } finally {
      setBusy(false);
      abortRef.current = null;
      setStreamPaused(false);
    }
  }, [api, authHeaders, busy, current, input, createSession, loadSessions, appendAnswer, appendReasoning, appendToActiveAssistant, loadRuns, loadAgentTasks]);

  const toggleStreamPause = useCallback(() => {
    if (!busy) return;
    setStreamPaused(prev => {
      const next = !prev;
      pausedRef.current = next;
      if (!next) {
        appendReasoning(pendingReasoningRef.current);
        appendAnswer(pendingDeltaRef.current);
        pendingReasoningRef.current = '';
        pendingDeltaRef.current = '';
      }
      return next;
    });
  }, [appendAnswer, appendReasoning, busy]);

  const stopStreaming = useCallback(() => {
    if (busy && abortRef.current) abortRef.current.abort();
  }, [busy]);

  const createWorkspace = useCallback(async () => {
    if (busy) return;
    const values = await showDialog({title:'新增工作空间', confirmText:'创建工作空间', fields:[
      {name:'name', label:'工作空间名称', value:'', required:true},
      {name:'system_prompt', label:'系统提示词内容', type:'textarea', rows:5, value:config.system_prompt || ''},
    ]});
    if (!values || !values.name.trim()) return;
    await api('/api/workspaces', {method:'POST', body: JSON.stringify({name: values.name.trim(), system_prompt: values.system_prompt || ''})});
    setCurrent(null);
    setCurrentTitle('未选择会话');
    setMessages([{role:'empty', content:'已创建并切换到新工作空间。'}]);
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig(), loadMCPConfig(), loadSkills(), loadScheduledTasks(), loadSessions()]);
    closeSidebarOnMobile();
    showToast('工作空间已创建', 'success');
  }, [api, busy, closeSidebarOnMobile, config.system_prompt, loadConfig, loadMCPConfig, loadPrompts, loadScheduledTasks, loadSessions, loadSkills, refreshProductState, showDialog, showToast]);

  const deleteWorkspace = useCallback(async (id, name) => {
    const ok = await showDialog({title:'删除工作空间', message:'确定删除工作空间「' + (name || id) + '」？这会删除该工作空间下的配置、技能、任务和会话。若删除当前工作空间，会自动切换到默认工作空间。', confirmText:'删除', danger:true, type:'confirm'});
    if (!ok) return;
    const data = await api('/api/workspaces/' + encodeURIComponent(id), {method:'DELETE'});
    setWorkspaces(data.workspaces || []);
    setCurrent(null);
    setCurrentTitle('未选择会话');
    setMessages([{role:'empty', content:'工作空间已删除。当前工作空间：' + (data.active || 'default')}]);
    await Promise.allSettled([loadPrompts(), loadConfig(), loadMCPConfig(), loadSkills(), loadScheduledTasks(), loadSessions(), loadSetupStatus(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
    showToast('工作空间已删除', 'success');
  }, [api, loadConfig, loadDataStatus, loadMCPConfig, loadModelProviders, loadPrompts, loadScheduledTasks, loadSessions, loadSetupStatus, loadSkills, loadSystemStatus, showDialog, showToast]);

  const saveConfig = useCallback(async () => {
    const workspaceID = (prompts.find(p => p.active) || {}).name || 'default';
    await api('/api/workspaces/' + encodeURIComponent(workspaceID) + '/config', {method:'POST', body: JSON.stringify({
      base_url: config.base_url,
      api_key: config.api_key,
      model: config.model,
      system_prompt: config.system_prompt,
      max_context_messages: Number(config.max_context_messages || 12),
      temperature: Number(config.temperature || 0.7),
      enable_thinking: !!config.enable_thinking,
      hide_thinking: !!config.hide_thinking,
    })});
    setConfig(c => ({...c, api_key:''}));
    await loadConfig();
    await Promise.allSettled([loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadSystemStatus()]);
    showToast('已保存到工作空间：' + workspaceID, 'success');
  }, [api, config, loadConfig, loadModelProviders, loadSetupStatus, loadSystemStatus, loadWorkspaces, prompts, showToast]);

  const showPromptPreview = useCallback(async () => {
    const workspaceID = (prompts.find(p => p.active) || {}).name || 'default';
    const data = await api('/api/workspaces/' + encodeURIComponent(workspaceID) + '/prompt-preview');
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
    await api('/api/setup/init', {method:'POST', body: JSON.stringify(values)});
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig()]);
    showToast('初始化完成', 'success');
  }, [api, config, loadConfig, loadPrompts, refreshProductState, showDialog, showToast]);

  const testModelProvider = useCallback(async () => {
    try {
      const data = await api('/api/model-providers/test', {method:'POST', body: JSON.stringify({
        base_url: config.base_url,
        api_key: config.api_key,
        model: config.model,
        system_prompt: config.system_prompt,
        max_context_messages: Number(config.max_context_messages || 12),
        temperature: Number(config.temperature || 0.7),
        enable_thinking: !!config.enable_thinking,
        hide_thinking: !!config.hide_thinking,
      })});
      showToast(data.ok ? '模型连接正常：' + (data.model || '') : '模型连接失败：' + (data.error || 'unknown'), data.ok ? 'success' : 'error');
    } catch (e) { showToast('模型连接失败：' + e.message, 'error'); }
  }, [api, config, showToast]);

  const saveMCPConfig = useCallback(async () => {
    try { JSON.parse(mcpConfig || '{}'); } catch (e) { showToast('MCP 配置不是合法 JSON：' + e.message, 'error'); return; }
    const c = await api('/api/mcp-config', {method:'POST', body: JSON.stringify({content:mcpConfig})});
    setMcpConfig(c.content || mcpConfig);
    await loadMCPStatus().catch(() => {});
    showToast('MCP 配置已保存', 'success');
  }, [api, loadMCPStatus, mcpConfig, showToast]);

  const testMCP = useCallback(async (serverName = '') => {
    try {
      const suffix = serverName ? '?server=' + encodeURIComponent(serverName) : '';
      const data = await api('/api/mcp/test' + suffix);
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
    const data = await api(existing ? '/api/skills/' + encodeURIComponent(existing.id) : '/api/skills', {method: existing ? 'PUT' : 'POST', body: JSON.stringify(payload)});
    setSkills(data.skills || []);
    showToast(existing ? '技能已保存' : '技能已新增', 'success');
  }, [api, busy, skills, showDialog, showToast]);

  const toggleSkill = useCallback(async (id, enabled) => {
    const existing = skills.find(s => s.id === id);
    if (!existing) return;
    const data = await api('/api/skills/' + encodeURIComponent(id), {method:'PUT', body: JSON.stringify({name: existing.name, description: existing.description || '', content: existing.content || '', enabled: !!enabled})});
    setSkills(data.skills || []);
  }, [api, skills]);

  const deleteSkill = useCallback(async (id) => {
    const existing = skills.find(s => s.id === id);
    if (!existing) return;
    const ok = await showDialog({title:'删除技能', message:'确定删除技能「' + existing.name + '」？此操作不可恢复。', confirmText:'删除', danger:true, type:'confirm'});
    if (!ok) return;
    const data = await api('/api/skills/' + encodeURIComponent(id), {method:'DELETE'});
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
    const data = await api(existing ? '/api/scheduled-tasks/' + encodeURIComponent(existing.id) : '/api/scheduled-tasks', {method: existing ? 'PUT' : 'POST', body: JSON.stringify(payload)});
    setScheduledTasks(data.tasks || []);
    showToast(existing ? '任务已保存' : '任务已新增', 'success');
  }, [api, busy, scheduledTasks, showDialog, showToast]);

  const toggleScheduledTask = useCallback(async (id, enabled) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    const payload = {title: existing.title, prompt: existing.prompt, enabled: !!enabled, schedule_type: existing.schedule_type, run_at: existing.run_at || '', time_of_day: existing.time_of_day || '', interval_minutes: existing.interval_minutes || 0};
    const data = await api('/api/scheduled-tasks/' + encodeURIComponent(id), {method:'PUT', body: JSON.stringify(payload)});
    setScheduledTasks(data.tasks || []);
  }, [api, scheduledTasks]);

  const deleteScheduledTask = useCallback(async (id) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    const ok = await showDialog({title:'删除自动化任务', message:'确定删除定时任务「' + existing.title + '」？此操作不可恢复。', confirmText:'删除', danger:true, type:'confirm'});
    if (!ok) return;
    const data = await api('/api/scheduled-tasks/' + encodeURIComponent(id), {method:'DELETE'});
    setScheduledTasks(data.tasks || []);
    showToast('任务已删除', 'success');
  }, [api, scheduledTasks, showDialog, showToast]);

  const runScheduledTaskNow = useCallback(async (id) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    const ok = await showDialog({title:'立即运行任务', message:'立即运行定时任务「' + existing.title + '」？', confirmText:'立即运行', type:'confirm'});
    if (!ok) return;
    try {
      const result = await api('/api/scheduled-tasks/' + encodeURIComponent(id) + '/run', {method:'POST', body:'{}'});
      await loadScheduledTasks();
      await loadSessions();
      refreshProductState();
      if (result.session && result.session.id) {
        setCurrent(result.session.id);
        setCurrentTitle(result.session.title || '定时任务');
        setMessages(result.session.messages || []);
      }
      showToast('定时任务已运行', 'success');
    } catch (e) { await loadScheduledTasks().catch(() => {}); showToast('运行失败：' + e.message, 'error'); }
  }, [api, loadScheduledTasks, loadSessions, refreshProductState, scheduledTasks, showDialog, showToast]);

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
    const q = sessionSearch.trim().toLowerCase();
    return q ? sessions.filter(s => [s.title, s.preview, s.last_role].some(v => String(v || '').toLowerCase().includes(q))) : sessions;
  }, [sessionSearch, sessions]);

  const activePrompt = useMemo(() => prompts.find(p => p.active) || prompts[0] || null, [prompts]);

  const appClass = 'app ' + (sidebarCollapsed ? 'sidebar-collapsed ' : '') + (settingsOpen ? 'settings-open' : '');

  return <>
    <div id="sidebarMask" className={'sidebar-mask ' + (!sidebarCollapsed ? 'show' : '')} onClick={() => setSidebarCollapsed(true)} />
    <div id="settingsMask" className={'settings-mask ' + (settingsOpen ? 'show' : '')} onClick={() => closeSettings()} />
    <div className={'session-actions-backdrop ' + (sessionActionsOpen ? 'show' : '')} onClick={() => setSessionActionsOpen(false)}>
      <div className="session-actions-sheet" onClick={e => e.stopPropagation()}>
        <div className="session-actions-head"><b>会话操作</b><button className="secondary small" onClick={() => setSessionActionsOpen(false)}>关闭</button></div>
        <button className="secondary" disabled={!current} onClick={() => { setSessionActionsOpen(false); renameCurrent(); }}>重命名</button>
        <button className="secondary" disabled={!current} onClick={() => { setSessionActionsOpen(false); copyCurrentMarkdown(); }}>复制全文</button>
        <button className="secondary" disabled={!current || busy} onClick={() => { setSessionActionsOpen(false); cloneCurrent(); }}>复制会话</button>
        <button className="secondary" disabled={!current} onClick={() => { setSessionActionsOpen(false); exportCurrent(); }}>导出 Markdown</button>
        <button className="danger" disabled={!current} onClick={() => { setSessionActionsOpen(false); deleteCurrent(); }}>删除会话</button>
      </div>
    </div>
    <div id="app" className={appClass}>
      <aside>
        <div className="sidebar-head">
          <div className="brand"><span className="brand-text">ChatDock</span><div className="sub">本地优先 AI 工作台</div></div>
          <button id="sidebarToggle" className="sidebar-toggle" onClick={() => setSidebarCollapsed(!sidebarCollapsed)} title={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}>{sidebarCollapsed ? '›' : '‹'}</button>
        </div>
        <div className="prompt-box">
          <label>工作空间</label>
          <div className="prompt-row">
            <button className="workspace-picker-trigger" type="button" disabled={busy || !prompts.length} onClick={() => setWorkspacePickerOpen(true)}>
              <span className="workspace-picker-name">{activePrompt ? activePrompt.name : '未选择'}</span>
              <span className="workspace-picker-meta">{activePrompt ? activePrompt.count + ' 条' : '暂无工作空间'}</span>
              <span className="workspace-picker-arrow">⌄</span>
            </button>
            <button className="prompt-add" onClick={createWorkspace}>+</button>
          </div>
        </div>
        <input className="session-search" placeholder="搜索会话" value={sessionSearch} onChange={e => setSessionSearch(e.target.value)} />
        <button className="new" onClick={newSession}>+ <span className="new-label">新会话</span></button>
        <div id="sessions">{filteredSessions.length ? filteredSessions.map(s => <div key={s.id} className={'session ' + (current === s.id ? 'active' : '')} onClick={() => openSession(s.id)}><div className="session-title">{s.title}</div>{s.preview ? <div className="session-preview">{s.preview}</div> : null}<div className="session-meta">{s.count} 条 · {fmtTime(s.updated_at)}</div></div>) : <div className="empty compact">没有匹配会话</div>}</div>
      </aside>
      <main>
        <div className="topbar">
          <div className="top-left"><button className="mobile-menu" onClick={() => setSidebarCollapsed(!sidebarCollapsed)}>☰</button><b id="title">{currentTitle}</b></div>
          <div className="top-actions">
            <button className="secondary config-toggle" onClick={() => openSettings()} title="配置中心">配置</button>
            <button className="secondary session-actions-toggle" onClick={() => setSessionActionsOpen(true)} disabled={!current} title="会话操作">会话</button>
            <button className="theme-toggle" onClick={() => setThemeState(theme === 'day' ? 'night' : 'day')}>{theme === 'day' ? '白天' : '夜晚'}</button>
            <button className="secondary" onClick={renameCurrent} disabled={!current}>重命名</button>
            <button className="secondary" onClick={copyCurrentMarkdown} disabled={!current}>复制全文</button>
            <button className="secondary" onClick={cloneCurrent} disabled={!current || busy}>复制会话</button>
            <button className="secondary" onClick={exportCurrent} disabled={!current}>导出</button>
            <button className="danger" onClick={deleteCurrent} disabled={!current}>删除</button>
          </div>
        </div>
        <div className="messages" ref={messagesRef}>{messages.length ? messages.map((m, i) => <MessageView key={i} message={m} onCopy={copyText} />) : <div className="empty">创建一个会话，然后开始聊天。</div>}</div>
        <div className="composer">
          <button className="secondary quick-control" disabled={busy} onClick={() => sendMsg('继续')}>继续</button>
          {busy ? <button className="secondary stream-control" onClick={toggleStreamPause}>{streamPaused ? '继续' : '暂停'}</button> : null}
          {busy ? <button className="danger stream-control" onClick={stopStreaming}>中断</button> : null}
          <textarea id="input" value={input} onChange={e => setInput(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMsg(); } }} placeholder="输入消息，Enter 发送，Shift+Enter 换行" />
          <button id="send" disabled={busy} onClick={() => sendMsg()}>发送</button>
        </div>
      </main>
      <SettingsPanel
        activeModule={activeModule} busy={busy} closeSettings={closeSettings} config={config} continueAgentTask={continueAgentTask}
        createWorkspace={createWorkspace} dataStatus={dataStatus} deleteScheduledTask={deleteScheduledTask} deleteSkill={deleteSkill} deleteWorkspace={deleteWorkspace}
        editScheduledTask={editScheduledTask} editSkill={editSkill} loadAgentTasks={loadAgentTasks} loadDataStatus={loadDataStatus} loadMCPConfig={loadMCPConfig}
        loadMCPStatus={loadMCPStatus} loadRuns={loadRuns} loadScheduledTasks={loadScheduledTasks} loadSkills={loadSkills} loadSystemStatus={loadSystemStatus}
        mcpConfig={mcpConfig} mcpStatus={mcpStatus} providers={providers} promptPreview={promptPreview} refreshProductState={refreshProductState}
        runScheduledTaskNow={runScheduledTaskNow} runSetupWizard={runSetupWizard} runs={runs} saveConfig={saveConfig} saveMCPConfig={saveMCPConfig}
        scheduledTasks={scheduledTasks} selectWorkspace={selectWorkspace} setConfig={setConfig} setMcpConfig={setMcpConfig} setTaskSearch={setTaskSearch}
        setupStatus={setupStatus} showPromptPreview={showPromptPreview} skillSearch={skillSearch} skills={skills} switchSettingsModule={switchSettingsModule}
        systemStatus={systemStatus} taskSearch={taskSearch} testMCP={testMCP} testModelProvider={testModelProvider} toggleScheduledTask={toggleScheduledTask}
        toggleSkill={toggleSkill} workspaces={workspaces} agentTasks={agentTasks} setSkillSearch={setSkillSearch} logout={logout}
      />
    </div>
    <WorkspacePicker open={workspacePickerOpen} prompts={prompts} busy={busy} activeName={activePrompt?.name || ''} onClose={() => setWorkspacePickerOpen(false)} onSelect={selectWorkspace} />
    {authPage ? <LoginPage api={api} error={authPage} refreshAfterLogin={refreshAfterLogin} setAuthPage={setAuthPage} /> : null}
    <DialogHost dialog={dialog} closeDialog={closeDialog} />
    {toast ? <div id="appToast" className={'app-toast show ' + toast.variant}>{toast.message}</div> : null}
  </>;
}

function MessageActions({ text, onCopy }) {
  return <div className="msg-actions"><button type="button" className="secondary small" onClick={() => onCopy(text)}>复制</button></div>;
}

function MessageView({ message, onCopy }) {
  if (message.role === 'empty') return <div className="empty">{message.content}</div>;
  if (message.role === 'assistant-stream') return <div className="msg assistant">
    <MessageActions text={message.answer || message.reasoning || ''} onCopy={onCopy} />
    {message.reasoning ? <div className="reasoning show"><div className="reasoning-title">思考</div><Markdown className="reasoning-content markdown" value={message.reasoning} /></div> : null}
    <Markdown className="answer markdown" value={message.answer} />
    {(message.events || []).map((event, i) => <div key={i} className={'tool-event ' + (event.kind === 'run' ? 'run-event-inline' : '')}>{event.text}{event.meta ? <div className="tool-event-meta">{event.meta}</div> : null}</div>)}
  </div>;
  if (message.role === 'assistant') return <div className="msg assistant markdown"><MessageActions text={message.content} onCopy={onCopy} /><Markdown value={message.content} /></div>;
  return <div className={'msg ' + (message.role || 'user')}><MessageActions text={message.content} onCopy={onCopy} />{message.content}</div>;
}

function WorkspacePicker({ open, prompts, busy, activeName, onClose, onSelect }) {
  if (!open) return null;
  return <div className="workspace-picker-backdrop show" onClick={onClose}>
    <div className="workspace-picker-sheet" role="dialog" aria-modal="true" aria-label="选择工作空间" onClick={e => e.stopPropagation()}>
      <div className="workspace-picker-head">
        <div><b>选择工作空间</b><div className="hint">切换后会加载对应会话、模型和技能。</div></div>
        <button className="secondary small" type="button" onClick={onClose}>关闭</button>
      </div>
      <div className="workspace-picker-list">
        {prompts.length ? prompts.map(item => <button
          key={item.name}
          type="button"
          disabled={busy}
          className={'workspace-picker-item ' + (item.name === activeName ? 'active' : '')}
          onClick={() => onSelect(item.name)}>
          <span className="workspace-picker-item-main"><b>{item.name}</b><span>{item.count} 条会话</span></span>
          <span className="workspace-picker-check">{item.name === activeName ? '✓' : ''}</span>
        </button>) : <div className="empty compact">暂无工作空间。</div>}
      </div>
    </div>
  </div>;
}

function LoginPage({ api, error, refreshAfterLogin, setAuthPage }) {
  const [username, setUsername] = useState('');
  const [credential, setCredential] = useState('');
  const [loginError, setLoginError] = useState('');
  const message = error ? (error.status === 401 ? '登录已过期，请重新登录。' : error.message) : '请输入 ChatDock 账号和密码。';
  async function submit(event) {
    event.preventDefault();
    setLoginError('');
    try {
      const data = await api('/api/auth/login', {method:'POST', body: JSON.stringify({username: username.trim(), credential})});
      if (data.token) localStorage.setItem('chatdock.authToken', data.token);
      setAuthPage(null);
      await refreshAfterLogin();
    } catch (e) { setLoginError('登录失败：' + e.message); }
  }
  return <div id="authPage" className="auth-page"><div className="auth-shell"><form className="login-card" onSubmit={submit}>
    <div className="login-brand">ChatDock</div><b>登录 ChatDock</b><div className="hint">{message}</div>
    <label>账号</label><input autoComplete="username" placeholder="账号" value={username} onChange={e => setUsername(e.target.value)} autoFocus />
    <label>密码</label><input type="password" autoComplete="current-password" placeholder="密码" value={credential} onChange={e => setCredential(e.target.value)} />
    <div className="task-error">{loginError}</div><button type="submit" className="login-submit">登录</button>
  </form></div></div>;
}

function DialogHost({ dialog, closeDialog }) {
  const [values, setValues] = useState({});
  useEffect(() => {
    const next = {};
    (dialog?.fields || []).forEach(f => { next[f.name] = f.value ?? ''; });
    setValues(next);
  }, [dialog]);
  if (!dialog) return null;
  const visibleFields = (dialog.fields || []).filter(field => {
    if (!field.showWhen) return true;
    return Object.entries(field.showWhen).every(([key, expected]) => {
      const current = values[key];
      return Array.isArray(expected) ? expected.includes(current) : current === expected;
    });
  });
  function submit(event) {
    event.preventDefault();
    if (dialog.type === 'confirm') closeDialog(true);
    else closeDialog(values);
  }
  return <div className="app-modal-backdrop show" onClick={e => { if (e.target === e.currentTarget) closeDialog(null); }}>
    <div className={'app-modal-card ' + (dialog.variant || '')} role="dialog" aria-modal="true"><form className="app-modal-form" onSubmit={submit}>
      <div className="app-modal-title">{dialog.title || '确认'}</div>
      {dialog.message ? <div className="app-modal-message">{dialog.message}</div> : null}
      <div className="app-modal-fields">{visibleFields.map(field => <label key={field.name} className="app-modal-field"><span>{field.label || field.name}</span>{renderDialogField(field, values[field.name] ?? '', value => setValues(v => ({...v, [field.name]: value})))}{field.hint ? <div className="app-modal-field-hint">{field.hint}</div> : null}</label>)}</div>
      <div className="app-modal-actions">{dialog.hideCancel ? null : <button type="button" className="secondary app-modal-cancel" onClick={() => closeDialog(null)}>{dialog.cancelText || '取消'}</button>}<button type="submit" className={dialog.danger ? 'danger' : ''}>{dialog.confirmText || '确定'}</button></div>
    </form></div>
  </div>;
}

function renderDialogField(field, value, setValue) {
  if (field.type === 'textarea') return <textarea rows={field.rows || 5} value={value} placeholder={field.placeholder || ''} required={!!field.required} onChange={e => setValue(e.target.value)} />;
  if (field.type === 'select') return <select value={value} required={!!field.required} onChange={e => setValue(e.target.value)}>{(field.options || []).map(opt => typeof opt === 'string' ? <option key={opt} value={opt}>{opt}</option> : <option key={opt.value} value={opt.value}>{opt.label}</option>)}</select>;
  return <input type={field.type || 'text'} min={field.min} max={field.max} step={field.step} value={value} placeholder={field.placeholder || ''} required={!!field.required} onChange={e => setValue(e.target.value)} />;
}

function SettingsPanel(props) {
  const {
    activeModule, busy, closeSettings, config, continueAgentTask, createWorkspace, dataStatus, deleteScheduledTask, deleteSkill, deleteWorkspace,
    editScheduledTask, editSkill, loadAgentTasks, loadDataStatus, loadMCPConfig, loadMCPStatus, loadRuns, loadScheduledTasks, loadSkills,
    loadSystemStatus, logout, mcpConfig, mcpStatus, providers, promptPreview, refreshProductState, runScheduledTaskNow, runSetupWizard,
    runs, saveConfig, saveMCPConfig, scheduledTasks, selectWorkspace, setConfig, setMcpConfig, setSkillSearch, setTaskSearch, setupStatus,
    showPromptPreview, skillSearch, skills, switchSettingsModule, systemStatus, taskSearch, testMCP, testModelProvider, toggleScheduledTask,
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
    <div className="settings-header"><div><h2>配置中心</h2><p>工作空间、模型、技能、工具和数据状态统一管理。</p></div><div className="settings-header-actions"><button className="secondary small" onClick={() => closeSettings()}>返回对话</button><button className="secondary small" onClick={refreshProductState}>刷新</button></div></div>
    <div className="module-tabs">{settingsModules.map(m => <button key={m} className={'module-tab ' + (activeModule === m ? 'active' : '')} onClick={() => switchSettingsModule(m)}>{moduleLabel(m)}</button>)}</div>
    <ModuleView name="workspace" activeModule={activeModule}><WorkspaceModule setupStatus={setupStatus} workspaces={workspaces} createWorkspace={createWorkspace} selectWorkspace={selectWorkspace} deleteWorkspace={deleteWorkspace} runSetupWizard={runSetupWizard} /></ModuleView>
    <ModuleView name="model" activeModule={activeModule}><ModelModule config={config} setConfig={setConfig} saveConfig={saveConfig} showPromptPreview={showPromptPreview} promptPreview={promptPreview} testModelProvider={testModelProvider} providers={providers} /></ModuleView>
    <ModuleView name="skills" activeModule={activeModule}><div className="settings-block-head"><label>技能库（当前工作空间）</label><button className="secondary small" onClick={() => editSkill()}>新增技能</button></div><input className="session-search" placeholder="搜索技能" value={skillSearch} onChange={e => setSkillSearch(e.target.value)} /><div className="skills-list">{filteredSkills.length ? filteredSkills.map(s => <SkillCard key={s.id} skill={s} editSkill={editSkill} deleteSkill={deleteSkill} toggleSkill={toggleSkill} />) : <div className="hint">暂无技能。技能会作为当前工作空间的补充系统指令注入模型请求。</div>}</div><div className="settings-actions"><button className="secondary" onClick={loadSkills}>刷新技能</button></div></ModuleView>
    <ModuleView name="tools" activeModule={activeModule}><ToolsModule mcpStatus={mcpStatus} mcpConfig={mcpConfig} setMcpConfig={setMcpConfig} saveMCPConfig={saveMCPConfig} loadMCPConfig={loadMCPConfig} loadMCPStatus={loadMCPStatus} testMCP={testMCP} /></ModuleView>
    <ModuleView name="runs" activeModule={activeModule}><div className="settings-block-head"><label>MCP 执行记录</label><button className="secondary small" onClick={loadRuns}>刷新</button></div>{runs.length ? runs.map(r => <RunCard key={r.id} run={r} />) : <div className="empty compact">还没有 MCP 执行记录。</div>}</ModuleView>
    <ModuleView name="agent" activeModule={activeModule}><div className="settings-block-head"><label>AgentDock 任务</label><button className="secondary small" onClick={loadAgentTasks}>刷新</button></div>{agentTasks.length ? agentTasks.map(t => <AgentTaskCard key={t.id} task={t} continueAgentTask={continueAgentTask} />) : <div className="empty compact">还没有 AgentDock 任务记录。</div>}</ModuleView>
    <ModuleView name="automation" activeModule={activeModule}><div className="settings-block-head"><label>自动化任务（当前工作空间）</label><button className="secondary small" onClick={() => editScheduledTask()}>新增任务</button></div><input className="session-search" placeholder="搜索任务" value={taskSearch} onChange={e => setTaskSearch(e.target.value)} /><div className="tasks-list">{filteredTasks.length ? filteredTasks.map(t => <TaskCard key={t.id} task={t} editScheduledTask={editScheduledTask} deleteScheduledTask={deleteScheduledTask} toggleScheduledTask={toggleScheduledTask} runScheduledTaskNow={runScheduledTaskNow} />) : <div className="hint">暂无定时任务。任务会按当前工作空间运行，并把结果写入专属会话。</div>}</div><div className="settings-actions"><button className="secondary" onClick={loadScheduledTasks}>刷新任务</button></div></ModuleView>
    <ModuleView name="data" activeModule={activeModule}><div className="settings-block-head"><label>数据状态</label><button className="secondary small" onClick={loadDataStatus}>刷新数据状态</button></div><DataStatus dataStatus={dataStatus} /></ModuleView>
    <ModuleView name="security" activeModule={activeModule}><SecurityModule systemStatus={systemStatus} loadSystemStatus={loadSystemStatus} logout={logout} /></ModuleView>
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

function ModelModule({ config, setConfig, saveConfig, showPromptPreview, promptPreview, testModelProvider, providers }) {
  const update = (key, value) => setConfig(c => ({...c, [key]: value}));
  return <>
    <div className="settings-block-head"><label>当前工作空间模型</label></div>
    <label>Base URL</label><input value={config.base_url} onChange={e => update('base_url', e.target.value)} placeholder="https://api.openai.com/v1" />
    <label>API Key</label><input type="password" value={config.api_key} onChange={e => update('api_key', e.target.value)} placeholder={config.has_api_key ? '已保存，留空不修改' : '未设置'} />
    <label>Model</label><input value={config.model} onChange={e => update('model', e.target.value)} placeholder="gpt-4o-mini" />
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

function ToolsModule({ mcpStatus, mcpConfig, setMcpConfig, saveMCPConfig, loadMCPConfig, loadMCPStatus, testMCP }) {
  return <>
    <div className="settings-block-head"><label>MCP 工具中心</label><button className="secondary small" onClick={loadMCPStatus}>检测状态</button></div>
    <div id="mcpStatusCards">{mcpStatus.length ? mcpStatus.map(s => <TextCard key={s.name} title={s.name} hint={s.url || '-'} badge={s.last_status || 'unknown'}><div className="product-meta">allow {s.allow_count} · deny {s.deny_count} · confirm {s.confirm_count} · token {s.has_token ? '已配置' : '无'}</div>{s.last_error ? <div className="task-error">{s.last_error}</div> : null}<div className="product-actions"><button className="secondary small" onClick={() => testMCP(s.name)}>测试此 Server</button></div></TextCard>) : <div className="empty compact">尚未配置 MCP Server。添加后可在这里查看状态、权限和确认规则。</div>}</div>
    <label>MCP 配置 JSON</label><textarea className="mcp-editor" value={mcpConfig} onChange={e => setMcpConfig(e.target.value)} />
    <div className="settings-actions"><button className="secondary" onClick={saveMCPConfig}>保存 MCP 配置</button><button className="secondary" onClick={loadMCPConfig}>重新加载 MCP</button><button className="secondary" onClick={() => testMCP()}>测试默认 MCP</button></div>
  </>;
}

function RunCard({ run }) {
  return <div className="task-card run-card"><div className="task-head"><div><div className="task-name">{run.title || 'MCP 执行'}</div><div className="task-desc">{run.summary || '暂无摘要'}</div></div><div className="task-actions"><span className={'badge ' + runStatusClass(run.status)}>{runStatusLabel(run.status)}</span></div></div><div className="task-meta">工作空间：{run.workspace || '-'} · 事件 {run.event_count || 0} · 耗时 {fmtDuration(run.duration_ms)} · {fmtTime(run.updated_at)}</div>{run.error ? <div className="task-error">{run.error}</div> : null}</div>;
}

function AgentTaskCard({ task, continueAgentTask }) {
  return <div className="task-card agent-task-card"><div className="task-head"><div><div className="task-name">{task.title || 'AgentDock 任务'}</div><div className="task-desc">{task.summary || '暂无摘要'}</div></div><div className="task-actions"><span className={'badge ' + runStatusClass(task.status)}>{runStatusLabel(task.status)}</span><button className="secondary small" onClick={() => continueAgentTask(task)}>继续任务</button></div></div><div className="task-meta">{task.server || 'AgentDock'} · {task.action || '-'}{task.phase ? ' · 阶段：' + task.phase : ''} · {fmtTime(task.updated_at)}</div>{task.error ? <div className="task-error">{task.error}</div> : null}</div>;
}

function TaskCard({ task, editScheduledTask, deleteScheduledTask, toggleScheduledTask, runScheduledTaskNow }) {
  return <div className="task-card"><div className="task-head"><div><div className="task-name">{task.title || '未命名任务'}{task.running ? ' · 运行中' : ''}</div><div className="task-desc">{(task.prompt || '').slice(0, 120) || '无提示内容'}</div></div><div className="task-actions"><span className={'badge ' + taskStatusClass(task)}>{taskStatusLabel(task)}</span><button className="secondary small" disabled={task.running} onClick={() => runScheduledTaskNow(task.id)}>立即运行</button><button className="secondary small" onClick={() => editScheduledTask(task.id)}>编辑</button><button className="danger small" onClick={() => deleteScheduledTask(task.id)}>删除</button></div></div><label className="task-toggle"><input type="checkbox" checked={!!task.enabled} onChange={e => toggleScheduledTask(task.id, e.target.checked)} /> 启用</label><div className="task-meta">{scheduleSummary(task)}</div>{task.last_error ? <div className="task-error">上次错误：{task.last_error}</div> : null}</div>;
}

function DataStatus({ dataStatus }) {
  if (!dataStatus) return <div className="hint">尚未加载数据状态。</div>;
  const items = [['数据目录', dataStatus.data_dir || '-'], ['数据库', dataStatus.database_path || '-'], ['数据库大小', fmtBytes(dataStatus.database_size_bytes)], ['工作空间', String(dataStatus.workspace_count || 0)], ['会话', String(dataStatus.session_count || 0)], ['WAL', dataStatus.wal_enabled ? '启用' : '未检测到']];
  return <div id="dataStatus">{items.map(item => <div className="stat-card" key={item[0]}><div className="stat-label">{item[0]}</div><div className="stat-value">{item[1]}</div></div>)}</div>;
}

function SecurityModule({ systemStatus, loadSystemStatus, logout }) {
  return <><div className="settings-block-head"><label>系统状态</label><button className="secondary small" onClick={loadSystemStatus}>刷新系统状态</button></div>{systemStatus ? <TextCard title="ChatDock" hint={systemStatus.addr || ''} badge={systemStatus.ok ? 'healthy' : 'unknown'}><div className="product-meta">Web：{systemStatus.web_dir || '-'} · DB：{systemStatus.database || '-'} · 当前工作空间：{(systemStatus.setup || {}).active_workspace || '-'}</div></TextCard> : <div className="hint">尚未加载系统状态。</div>}<div className="settings-actions"><button className="secondary" onClick={logout}>登录 / 切换账号</button></div></>;
}

async function readSSE(res, onEvent) {
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const {value, done} = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, {stream:true});
    const parts = buffer.split('\n\n');
    buffer = parts.pop() || '';
    for (const part of parts) {
      const event = parseSSE(part);
      if (event) onEvent(event.event, event.data);
    }
  }
  if (buffer.trim()) {
    const event = parseSSE(buffer);
    if (event) onEvent(event.event, event.data);
  }
}

function parseSSE(block) {
  let event = 'message';
  const dataLines = [];
  for (const line of block.split('\n')) {
    if (line.startsWith('event:')) event = line.slice(6).trim();
    if (line.startsWith('data:')) dataLines.push(line.slice(5).trim());
  }
  if (!dataLines.length) return null;
  return {event, data: JSON.parse(dataLines.join('\n'))};
}
