import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { EmptyState, MessageView } from './components/chat.jsx';
import { ComposerBar, Sidebar, Topbar } from './components/appChrome.jsx';
import { DialogHost, LoginPage, Markdown, QuickPalette, WorkspacePicker } from './components/base.jsx';
import { SettingsPanel } from './components/settings.jsx';
import { defaultRunAtValue, diagnosticsText, filenameFromResponse, fmtDuration, fmtTime, normalizeSettingsModule, runStatusLabel, sessionIDFromPath, sessionPath, settingsModuleFromPath } from './lib/appUtils.js';
import { attachmentLooksLikeImage, contextPreviewText, finalAssistantMessageFromSession, readableChatError, scheduledTaskContextLabel, scheduledTaskRunsText, streamErrorMessage, streamStatusText } from './lib/chatPresentation.js';
import { buildToolEventDetail } from './lib/toolEventDetails.js';
import { createJsonApi } from './lib/http.js';
import { cancelChatJob, fetchChatJobs, guideChatJob, resolveMCPConfirmation, streamChat, streamChatJobEvents } from './lib/chatApi.js';
import { branchSession, cloneSession, createSessionRecord, deleteSession, editSessionMessage, fetchContextPreview, fetchSession, fetchSessionMarkdown, fetchSessionToolEvent, fetchSessions, pinSession, renameSession, searchSessions, updateSessionModel } from './lib/sessionApi.js';
import { createModelProvider as createModelProviderRequest, createWorkspaceRecord, deleteModelProvider as deleteModelProviderRequest, deleteScheduledTaskRecord, deleteWorkspaceRecord, fetchProviderModels as fetchProviderModelsRequest, fetchPromptPreview, fetchScheduledTaskRuns, initializeSetup, runScheduledTask, saveMCPConfigRequest, saveScheduledTaskRecord, saveWorkspaceConfig, selectWorkspace as selectWorkspaceRequest, testMCPServer, testModelProvider as testModelProviderRequest, updateModelProvider as updateModelProviderRequest } from './lib/settingsApi.js';
import { useAttachments } from './hooks/useAttachments.js';
import { appendInlineReasoningPart, appendInlineTextPart, appendToolStartEvent, mergeToolResultEvent } from './lib/toolEvents.js';
import { providerChoiceID, providerKeyInputsFromRows, providerKeyRows, providerLabel, sessionModelChoice, uniqueModelNames } from './lib/modelProviderForm.js';
import { useSettingsData } from './hooks/useSettingsData.js';
import { buildQuickActions } from './lib/quickActions.js';
import { scheduledTaskSessionRows, visibleSessionRows } from './lib/sessionPresentation.js';

function useLazyRef(createValue) {
  const ref = useRef(null);
  if (ref.current === null) ref.current = createValue();
  return ref;
}

function messageKey(message, index) {
  return message?.id || message?.message_id || message?.created_at || [message?.role || 'message', String(message?.content || message?.answer || '').slice(0, 80), index].join(':');
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
  const [quickPaletteOpen, setQuickPaletteOpen] = useState(false);
  const [modelPickerOpen, setModelPickerOpen] = useState(false);
  const [chatModel, setChatModel] = useState({ provider_id: '', model: '' });
  const [showJumpToLatest, setShowJumpToLatest] = useState(false);

  const [availableModels, setAvailableModels] = useState([]);
  const [candidateProviderID, setCandidateProviderID] = useState('');
  const [loadingModels, setLoadingModels] = useState(false);
  const [sessions, setSessions] = useState([]);
  const [sessionSearch, setSessionSearch] = useState('');
  const [sessionSearchResults, setSessionSearchResults] = useState([]);
  const [sessionSearchBusy, setSessionSearchBusy] = useState(false);
  const [sessionMenuID, setSessionMenuID] = useState('');
  const [current, setCurrent] = useState(null);
  const [currentTitle, setCurrentTitle] = useState('未选择会话');
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [busy, setBusy] = useState(false);

  const [streamStats, setStreamStats] = useState({ state: 'idle', started_at: 0, chars: 0, events: 0, tools: 0, error: '' });
  const [selectedScheduledTaskID, setSelectedScheduledTaskID] = useState('');
  const [selectedScheduledTaskRuns, setSelectedScheduledTaskRuns] = useState([]);
  const [taskSearch, setTaskSearch] = useState('');

  const abortRef = useRef(null);
  const activeJobIDRef = useRef('');
  const activeJobSessionRef = useRef('');
  const detachedControllersRef = useLazyRef(() => new WeakSet());
  const currentRef = useRef(null);
  const pausedRef = useRef(false);
  const pendingDeltaRef = useRef('');
  const pendingReasoningRef = useRef('');
  const messagesRef = useRef(null);
  const stickToBottomRef = useRef(true);
  const forceScrollRef = useRef(false);
  const inputRef = useRef(null);
  const fileInputRef = useRef(null);
  const sessionOpenSeqRef = useRef(0);
  const toolEventDetailCacheRef = useLazyRef(() => new Map());

  useEffect(() => { currentRef.current = current; }, [current]);
  useEffect(() => {
    if (!sessionMenuID) return;
    const close = () => setSessionMenuID('');
    const onKey = (event) => { if (event.key === 'Escape') close(); };
    window.addEventListener('click', close);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('click', close);
      window.removeEventListener('keydown', onKey);
    };
  }, [sessionMenuID]);

  const detachActiveStream = useCallback(() => {
    // 切换会话只断开当前页面的 SSE 监听，不取消后端 ChatJob；
    // 这样原会话继续后台生成，用户可以马上去另一个会话发送消息。
    const abort = abortRef.current;
    if (abort) {
      detachedControllersRef.current.add(abort);
      abort.abort();
    }
    abortRef.current = null;
    activeJobIDRef.current = '';
    activeJobSessionRef.current = '';
    setBusy(false);
    pausedRef.current = false;
    setStreamStats({ state: 'idle', started_at: 0, chars: 0, events: 0, tools: 0, error: '' });
  }, [detachedControllersRef]);

  useEffect(() => {
    if (busy && current && activeJobSessionRef.current && activeJobSessionRef.current !== current) {
      detachActiveStream();
    }
  }, [busy, current, detachActiveStream]);

  useEffect(() => {
    document.body.classList.toggle('theme-light', theme === 'day');
    document.body.classList.toggle('theme-night', theme !== 'day');
    localStorage.setItem('chatdock.theme', theme);
  }, [theme]);

  useEffect(() => {
    document.body.classList.toggle('auth-page-visible', !!authPage);
    return () => document.body.classList.remove('auth-page-visible');
  }, [authPage]);

  const showToast = useCallback((message, variant = 'info') => {
    setToast({ message, variant });
    clearTimeout(toastTimerRef.current);
    toastTimerRef.current = setTimeout(() => setToast(null), 3200);
  }, []);

  const showDialog = useCallback((config) => new Promise(resolve => {
    setDialog({ ...config, resolve });
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

  const authHeaders = useCallback((extra = {}) => {
    const token = localStorage.getItem('chatdock.authToken') || '';
    return token ? { 'Authorization': 'Bearer ' + token, ...extra } : extra;
  }, []);

  const api = useMemo(() => createJsonApi({ authHeaders, onUnauthorized: setAuthPage }), [authHeaders]);

  const {
    setupStatus,
    workspaces,
    setWorkspaces,
    providers,
    prompts,
    scheduledTasks,
    dataStatus,
    systemStatus,
    mcpStatus,
    promptPreview,
    setPromptPreview,
    mcpConfig,
    setMcpConfig,
    config,
    setConfig,
    loadPrompts,
    loadConfig,
    loadMCPConfig,
    loadSetupStatus,
    loadWorkspaces,
    loadModelProviders,
    loadScheduledTasks,
    loadDataStatus,
    loadSystemStatus,
    loadMCPStatus,
  } = useSettingsData(api);

  const {
    pendingAttachments,
    pendingAttachmentIDs,
    readyAttachments,
    uploadingFiles,
    clearAttachments,
    downloadAttachment,
    handleFileSelect,
    removePendingAttachment,
  } = useAttachments({ authHeaders, busy, setAuthPage, showToast });

  const applySessionModel = useCallback((session, { fallbackToDefault = true } = {}) => {
    const next = sessionModelChoice(session);
    if (fallbackToDefault || next.provider_id || next.model) setChatModel(next);
  }, []);

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


  const refreshProductState = useCallback(async () => {
    await Promise.allSettled([loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
  }, [loadSetupStatus, loadWorkspaces, loadModelProviders, loadDataStatus, loadSystemStatus]);

  const refreshVisibleSettings = useCallback(async () => {
    const jobs = [loadSetupStatus(), loadWorkspaces(), loadModelProviders(), loadDataStatus(), loadSystemStatus()];
    if (activeModule === 'tools') jobs.push(loadMCPStatus());
    if (activeModule === 'automation') jobs.push(loadScheduledTasks());
    await Promise.allSettled(jobs);
    showToast('配置中心已刷新', 'success');
  }, [activeModule, loadDataStatus, loadMCPStatus, loadModelProviders, loadScheduledTasks, loadSetupStatus, loadSystemStatus, loadWorkspaces, showToast]);

  const loadSessionFromRoute = useCallback(async (id) => {
    if (!id) return false;
    const s = await fetchSession(api, id);
    setCurrent(s.id);
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    clearAttachments();
    applySessionModel(s);
    await loadSessions();
    return true;
  }, [api, applySessionModel, clearAttachments, loadSessions]);

  const refreshAfterLogin = useCallback(async () => {
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig(), loadMCPConfig(), loadScheduledTasks(), loadSessions()]);
    const routeSession = sessionIDFromPath();
    if (routeSession) await loadSessionFromRoute(routeSession).catch(e => showToast('会话路由加载失败：' + e.message, 'error'));
  }, [refreshProductState, loadPrompts, loadConfig, loadMCPConfig, loadScheduledTasks, loadSessions, loadSessionFromRoute, showToast]);

  useEffect(() => {
    let mounted = true;
    async function start() {
      try {
        const status = await api('/api/auth/status');
        if (!mounted) return;
        if (status.enabled && status.login_enabled && !localStorage.getItem('chatdock.authToken')) {
          setAuthPage({ message: '请输入 ChatDock 账号和密码。' });
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
        setChatModel({ provider_id: '', model: '' });
      }
    }
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, [loadSessionFromRoute, showToast]);

  useEffect(() => {
    if (!settingsOpen) return;
    if (activeModule === 'tools') loadMCPStatus().catch(e => showToast('MCP 状态加载失败：' + e.message, 'error'));
    if (activeModule === 'data') loadDataStatus().catch(e => showToast('数据状态加载失败：' + e.message, 'error'));
    if (activeModule === 'security') loadSystemStatus().catch(e => showToast('系统状态加载失败：' + e.message, 'error'));
  }, [settingsOpen, activeModule, loadMCPStatus, loadDataStatus, loadSystemStatus, showToast]);

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
      box.scrollTo({ top: Math.max(0, latest.offsetTop - 14), behavior: 'smooth' });
    } else {
      box.scrollTo({ top: box.scrollHeight, behavior: 'smooth' });
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
    if (syncRoute && window.location.pathname !== '/settings/' + normalized) window.history.pushState({ chatdock: true }, '', '/settings/' + normalized);
  }, [activeModule]);

  const closeSettings = useCallback((syncRoute = true) => {
    setSettingsOpen(false);
    const target = current ? sessionPath(current) : '/';
    if (syncRoute && window.location.pathname !== target) window.history.pushState({ chatdock: true }, '', target);
  }, [current]);

  const switchSettingsModule = useCallback((moduleName) => openSettings(moduleName), [openSettings]);

  useEffect(() => {
    function closeTopLayer(event) {
      if (event.key !== 'Escape') return;
      if (dialog) closeDialog(null);
      else if (quickPaletteOpen) setQuickPaletteOpen(false);
      else if (workspacePickerOpen) setWorkspacePickerOpen(false);
      else if (settingsOpen) closeSettings();
    }
    window.addEventListener('keydown', closeTopLayer);
    return () => window.removeEventListener('keydown', closeTopLayer);
  }, [closeDialog, closeSettings, dialog, quickPaletteOpen, settingsOpen, workspacePickerOpen]);

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
    setMessages([{ role: 'empty', content: '已切换工作空间。创建或选择一个会话。' }]);
    clearAttachments();
    setChatModel({ provider_id: '', model: '' });
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig(), loadMCPConfig(), loadScheduledTasks(), loadSessions()]);
    if (window.location.pathname !== '/') window.history.pushState({ chatdock: true }, '', '/');
    closeSidebarOnMobile();
  }, [api, busy, clearAttachments, refreshProductState, loadPrompts, loadConfig, loadMCPConfig, loadScheduledTasks, loadSessions, closeSidebarOnMobile, showToast]);

  const createPersistedSession = useCallback(async ({ refreshList = true } = {}) => {
    const s = await createSessionRecord(api);
    setCurrent(s.id);
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    clearAttachments();
    applySessionModel(s, { fallbackToDefault: false });
    if (refreshList) await loadSessions();
    if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({ chatdock: true }, '', sessionPath(s.id));
    return s;
  }, [api, applySessionModel, clearAttachments, loadSessions]);

  const createSession = useCallback(() => {
    if (busy) detachActiveStream();
    // “新会话”只是进入一个本地草稿，不应该提前写入后端。
    // 真正的 session id 只有在发送首条消息、或上传附件需要绑定会话时才创建。
    setSessionMenuID('');
    setCurrent(null);
    setCurrentTitle('新会话');
    setMessages([]);
    clearAttachments();
    setChatModel({ provider_id: '', model: '' });
    if (window.location.pathname !== '/') window.history.pushState({ chatdock: true }, '', '/');
    closeSidebarOnMobile();
    window.setTimeout(() => inputRef.current?.focus(), 0);
    return { id: '', title: '新会话', messages: [], draft: true };
  }, [busy, clearAttachments, closeSidebarOnMobile, detachActiveStream]);

  const openSession = useCallback(async (id, summary = null) => {
    if (!id) return;
    const seq = sessionOpenSeqRef.current + 1;
    sessionOpenSeqRef.current = seq;
    if (busy) detachActiveStream();
    setSessionMenuID('');
    setCurrent(id);
    setCurrentTitle(summary?.title || '正在加载会话…');
    clearAttachments();
    stickToBottomRef.current = true;
    forceScrollRef.current = true;
    if (!messages.length) setMessages([{ role: 'empty', content: '正在加载会话…' }]);
    if (window.location.pathname !== sessionPath(id)) window.history.pushState({ chatdock: true }, '', sessionPath(id));
    closeSidebarOnMobile();
    try {
      const s = await fetchSession(api, id);
      if (sessionOpenSeqRef.current !== seq) return;
      setCurrent(s.id || id);
      setCurrentTitle(s.title || summary?.title || '新会话');
      setMessages(s.messages || []);
      applySessionModel(s);
      await loadSessions();
    } catch (e) {
      if (sessionOpenSeqRef.current !== seq) return;
      setMessages([{ role: 'empty', content: '会话加载失败：' + e.message }]);
      showToast('会话加载失败：' + e.message, 'error');
    }
  }, [api, applySessionModel, busy, clearAttachments, closeSidebarOnMobile, detachActiveStream, loadSessions, messages.length, showToast]);

  const newSession = useCallback(async () => { await createSession(); }, [createSession]);

  const renameCurrent = useCallback(async () => {
    if (!current) return;
    const values = await showDialog({ title: '重命名会话', confirmText: '保存标题', fields: [{ name: 'title', label: '新的会话标题', value: currentTitle || '', required: true }] });
    if (!values || !values.title.trim()) return;
    const s = await renameSession(api, current, values.title.trim());
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    await loadSessions();
    showToast('会话标题已保存', 'success');
  }, [api, current, currentTitle, loadSessions, showDialog, showToast]);

  const renameSessionByID = useCallback(async (id, title = '') => {
    if (!id || busy) return;
    setSessionMenuID('');
    const values = await showDialog({ title: '重命名会话', confirmText: '保存标题', fields: [{ name: 'title', label: '新的会话标题', value: title || '', required: true }] });
    if (!values || !values.title.trim()) return;
    const s = await renameSession(api, id, values.title.trim());
    if (id === current) {
      setCurrentTitle(s.title || '新会话');
      setMessages(s.messages || []);
    }
    await loadSessions();
    showToast('会话标题已保存', 'success');
  }, [api, busy, clearAttachments, current, loadSessions, showDialog, showToast]);

  const deleteSessionByID = useCallback(async (id, title = '当前会话') => {
    if (!id || busy) return;
    const ok = await showDialog({
      title: '删除会话',
      message: '确定删除「' + (title || '未命名会话') + '」？此操作不可恢复。',
      confirmText: '删除',
      danger: true,
      type: 'confirm'
    });
    if (!ok) return;
    await deleteSession(api, id);
    // 如果删的是当前打开的会话，需要同步清空主界面和路由，避免 URL 指向已删除会话。
    if (id === current) {
      setCurrent(null);
      setCurrentTitle('未选择会话');
      setMessages([]);
      clearAttachments();
      setChatModel({ provider_id: '', model: '' });
      if (window.location.pathname !== '/') window.history.pushState({ chatdock: true }, '', '/');
    }
    await loadSessions();
    showToast('会话已删除', 'success');
  }, [api, busy, clearAttachments, current, loadSessions, showDialog, showToast]);

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
      applySessionModel(s);
      await loadSessions();
      if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({ chatdock: true }, '', sessionPath(s.id));
      showToast('会话已复制', 'success');
    } catch (e) {
      showToast('复制会话失败：' + e.message, 'error');
    }
  }, [api, applySessionModel, busy, current, loadSessions, showToast]);


  const branchCurrent = useCallback(async (messageIndex = messages.length - 1) => {
    if (!current || busy) return;
    try {
      const s = await branchSession(api, current, messageIndex);
      setCurrent(s.id);
      setCurrentTitle(s.title || '分支对话');
      setMessages(s.messages || []);
      clearAttachments();
      applySessionModel(s);
      await loadSessions();
      if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({ chatdock: true }, '', sessionPath(s.id));
      closeSidebarOnMobile();
      showToast('已在新聊天中创建分支对话', 'success');
    } catch (e) {
      showToast('创建分支对话失败：' + e.message, 'error');
    }
  }, [api, applySessionModel, busy, clearAttachments, closeSidebarOnMobile, current, loadSessions, messages.length, showToast]);

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

  const pinSessionByID = useCallback(async (id, pinned = false) => {
    if (!id || busy) return;
    setSessionMenuID('');
    const nextPinned = !pinned;
    const s = await pinSession(api, id, nextPinned);
    if (id === current) {
      setCurrentTitle(s.title || currentTitle || '新会话');
      setMessages(s.messages || []);
    }
    await loadSessions();
    showToast(nextPinned ? '会话已置顶' : '已取消置顶', 'success');
  }, [api, busy, current, currentTitle, loadSessions, showToast]);

  const appendToActiveAssistant = useCallback((patcher) => {
    setMessages(prev => prev.map((m, index) => index === prev.length - 1 && m.role === 'assistant-stream' ? patcher(m) : m));
  }, []);

  const appendAnswer = useCallback((text) => {
    if (!text) return;
    appendToActiveAssistant(m => appendInlineTextPart(m, text));
  }, [appendToActiveAssistant]);

  const appendReasoning = useCallback((text) => {
    if (!text) return;
    appendToActiveAssistant(m => appendInlineReasoningPart(m, text));
  }, [appendToActiveAssistant]);


  const finishActiveAssistant = useCallback((finalSession) => {
    const finalAssistant = finalAssistantMessageFromSession(finalSession);
    setMessages(prev => prev.map((m, index) => {
      if (index !== prev.length - 1 || m.role !== 'assistant-stream') return m;
      const content = finalAssistant?.content || m.answer || '';
      return {...m, role: 'assistant', content, answer: content, reasoning: finalAssistant?.reasoning || m.reasoning || '', created_at: finalAssistant?.created_at || m.created_at};
    }));
  }, []);

  const handleChatStreamEvent = useCallback((event, data, setFinalSession) => {
    if (event === 'job_started') {
      activeJobIDRef.current = data.id || '';
    } else if (event === 'delta') {
      const reasoning = data.reasoning_content || '';
      const content = data.content || '';
      setStreamStats(prev => ({ ...prev, state: pausedRef.current ? 'paused' : 'streaming', chars: prev.chars + content.length + reasoning.length }));
      if (pausedRef.current) {
        pendingReasoningRef.current += reasoning;
        pendingDeltaRef.current += content;
      } else {
        appendReasoning(reasoning);
        appendAnswer(content);
      }
    } else if (event === 'tool_setup_ready') {
      setStreamStats(prev => ({ ...prev, events: prev.events + 1 }));
      appendToActiveAssistant(m => ({ ...m, events: [...(m.events || []), { kind: 'tool', text: data.mode === 'discovery' ? ('已准备可用工具索引：' + (data.tool_count || 0) + ' 个工具') : ('MCP 已接入：' + (data.tool_count || 0) + ' 个工具'), details: { event, data } }] }));
    } else if (event === 'tool_setup_error') {
      setStreamStats(prev => ({ ...prev, events: prev.events + 1, error: data.message || 'MCP 工具未接入' }));
      appendToActiveAssistant(m => ({ ...m, events: [...(m.events || []), { kind: 'tool', text: '⚠️ MCP 未接入：' + (data.message || '工具初始化失败'), details: { event, data } }] }));
    } else if (event === 'tool_call_start') {
      setStreamStats(prev => ({ ...prev, events: prev.events + 1, tools: prev.tools + 1 }));
      appendToActiveAssistant(m => appendToolStartEvent(m, event, data));
    } else if (event === 'tool_call_result') {
      setStreamStats(prev => ({ ...prev, events: prev.events + 1 }));
      appendToActiveAssistant(m => mergeToolResultEvent(m, event, data));
    } else if (event === 'tool_confirmation_required') {
      setStreamStats(prev => ({ ...prev, events: prev.events + 1, state: 'paused' }));
      appendToActiveAssistant(m => ({ ...m, events: [...(m.events || []), { kind: 'confirm', text: '⏳ 等待确认工具：' + (data.tool || 'MCP 工具'), meta: '确认后模型会继续执行；拒绝则把拒绝结果返回给模型。', confirmation: data, status: 'pending', details: { event, tool: data.tool || '', arguments: data.arguments || {}, data } }] }));
    } else if (event === 'tool_confirmation_resolved') {
      setStreamStats(prev => ({ ...prev, events: prev.events + 1, state: 'streaming' }));
      appendToActiveAssistant(m => ({ ...m, events: (m.events || []).map(item => item.confirmation?.id === data.id ? { ...item, status: 'resolved', text: (data.approved ? '✅ 已允许工具：' : '⛔ 已拒绝工具：') + (data.tool || item.confirmation?.tool || 'MCP 工具') } : item) }));
    } else if (event === 'job_cancelled') {
      setStreamStats(prev => ({ ...prev, events: prev.events + 1, state: 'stopping' }));
      appendToActiveAssistant(m => ({ ...m, events: [...(m.events || []), { kind: 'tool', text: '⏹️ 已请求停止生成', details: { event, data } }] }));
    } else if (event === 'guidance_queued') {
      setStreamStats(prev => ({ ...prev, events: prev.events + 1 }));
      appendToActiveAssistant(m => ({ ...m, events: [...(m.events || []), { kind: 'guide', phase: 'running', text: '🧭 已收到引导，等待下一轮模型调用', meta: data.message || '', details: { event, data } }] }));
    } else if (event === 'guidance_injected') {
      setStreamStats(prev => ({ ...prev, events: prev.events + 1 }));
      appendToActiveAssistant(m => ({ ...m, events: [...(m.events || []), { kind: 'guide', phase: 'done', text: '🧭 已将引导加入下一轮模型上下文', meta: data.message || '', details: { event, data } }] }));
    } else if (event === 'run_event') {
      setStreamStats(prev => ({ ...prev, events: prev.events + 1 }));
      if (data.kind !== 'tool_call' && data.kind !== 'tool_result') {
        const meta = [runStatusLabel(data.status || ''), data.server, data.action, fmtDuration(data.duration_ms)].filter(Boolean).join(' · ');
        appendToActiveAssistant(m => ({ ...m, events: [...(m.events || []), { kind: 'run', text: '🧭 ' + (data.summary || data.tool || 'MCP 工具事件'), meta, details: { event, tool: data.tool || '', arguments: data.arguments, result: data.result, error: data.error || '', duration_ms: data.duration_ms, data } }] }));
      }
    } else if (event === 'run_finish') {
    } else if (event === 'message_end') {
      activeJobIDRef.current = '';
      if (data.status === 'failed') {
        setStreamStats(prev => ({ ...prev, state: 'error' }));
      } else if (data.status === 'interrupted') {
        setStreamStats(prev => ({ ...prev, state: 'done' }));
      }
    } else if (event === 'done') {
      setFinalSession(data.session);
      activeJobIDRef.current = '';
      setStreamStats(prev => ({ ...prev, state: 'done' }));
    } else if (event === 'error') {
      const message = streamErrorMessage(data);
      setStreamStats(prev => ({ ...prev, state: 'error', error: message }));
      appendToActiveAssistant(m => ({ ...m, error: data, answer: m.answer || '', events: [...(m.events || []), { kind: 'error', phase: 'error', text: '⚠️ ' + message, details: { event, error: message, data } }] }));
    }
  }, [appendAnswer, appendReasoning, appendToActiveAssistant]);

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
        activeJobSessionRef.current = current;
        pausedRef.current = false;
        pendingDeltaRef.current = '';
        pendingReasoningRef.current = '';
        abortRef.current = abort;
        setStreamStats({ state: 'streaming', started_at: Date.now(), chars: 0, events: 0, tools: 0, error: '' });
        setMessages(prev => prev.some(m => m.role === 'assistant-stream') ? prev : [...prev, { role: 'assistant-stream', answer: '', reasoning: '', events: [{ kind: 'tool', text: '↩️ 已恢复后台生成', details: { event: 'resume_running_job', job } }] }]);
        let finalSession = null;
        await streamChatJobEvents({ jobID: job.id, authHeaders, signal: abort.signal, onEvent: (event, data) => handleChatStreamEvent(event, data, s => { finalSession = s; }) });
        if (finalSession && !stopped) {
          pendingDeltaRef.current = '';
          pendingReasoningRef.current = '';
          finishActiveAssistant(finalSession);
          setCurrentTitle(finalSession.title || currentTitle || '新会话');
          loadSessions().catch(() => { });
        }
      } catch (e) {
        if (!abort.signal.aborted && !stopped) {
          const message = readableChatError(e);
          setStreamStats(prev => ({ ...prev, state: 'error', error: message }));
          appendToActiveAssistant(m => ({ ...m, error: { message }, answer: m.answer || '' }));
        }
      } finally {
        if (!stopped) {
          setBusy(false);
          abortRef.current = null;
          activeJobIDRef.current = '';
          activeJobSessionRef.current = '';
          pausedRef.current = false;
        }
      }
    }
    resumeRunningJob();
    return () => { stopped = true; abort.abort(); };
  }, [current, api, authHeaders, handleChatStreamEvent, appendToActiveAssistant, currentTitle, loadSessions, finishActiveAssistant]);

  const activePrompt = useMemo(() => prompts.find(p => p.active) || prompts[0] || null, [prompts]);
  const draftKey = useMemo(() => 'chatdock.draft.' + encodeURIComponent(activePrompt?.name || 'default') + '.' + encodeURIComponent(current || 'new'), [activePrompt?.name, current]);


  const providerChoices = useMemo(() => {
    const choices = [];
    for (const provider of providers) {
      const id = providerChoiceID(provider);
      const models = uniqueModelNames([...(provider.models || []), provider.default_model].filter(Boolean));
      if (id && (provider.enabled || models.length)) choices.push({ ...provider, choice_id: id, models });
    }
    return choices;
  }, [providers]);
  const selectedModelProvider = useMemo(() => {
    const activeID = config.provider_id || '';
    return providerChoices.find(provider => provider.choice_id === chatModel.provider_id)
      || providerChoices.find(provider => provider.choice_id === activeID)
      || providerChoices[0]
      || null;
  }, [chatModel.provider_id, config.provider_id, providerChoices]);
  const selectedChatModel = chatModel.model || selectedModelProvider?.default_model || selectedModelProvider?.models?.[0] || config.model || '';
  const selectedModelBaseURL = selectedModelProvider?.base_url || config.base_url || '';

  useEffect(() => {
    if (!providerChoices.length) return;
    const stillValid = providerChoices.some(provider => provider.choice_id === chatModel.provider_id && (!chatModel.model || provider.models.includes(chatModel.model)));
    if (stillValid) return;
    const provider = providerChoices.find(item => item.choice_id === (config.provider_id || '')) || providerChoices[0];
    setChatModel({ provider_id: provider.choice_id, model: provider.default_model || provider.models[0] || config.model || '' });
  }, [chatModel.model, chatModel.provider_id, config.model, config.provider_id, providerChoices]);

  const selectChatModel = useCallback((provider, modelName) => {
    const next = { provider_id: provider.choice_id, model: modelName };
    setChatModel(next);
    setModelPickerOpen(false);
    showToast('已切换模型：' + providerLabel(provider) + ' · ' + modelName, 'success');
    if (current) {
      updateSessionModel(api, current, { providerID: next.provider_id, model: next.model })
        .then(session => {
          applySessionModel(session, { fallbackToDefault: false });
          return loadSessions();
        })
        .catch(error => showToast('会话模型保存失败：' + error.message, 'error'));
    }
  }, [api, applySessionModel, current, loadSessions, showToast]);

  useEffect(() => {
    setInput(localStorage.getItem(draftKey) || '');
  }, [draftKey]);

  useEffect(() => {
    if (busy) return;
    const text = input.trim();
    if (text) localStorage.setItem(draftKey, input);
    else localStorage.removeItem(draftKey);
  }, [busy, draftKey, input]);

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
      const s = await createPersistedSession({ refreshList: false });
      if (!s) return;
      sessionID = s.id;
    }
    localStorage.removeItem(draftKey);
    setInput('');
    setBusy(true);
    pausedRef.current = false;
    setStreamStats({ state: 'connecting', started_at: Date.now(), chars: 0, events: 0, tools: 0, error: '' });
    pendingDeltaRef.current = '';
    pendingReasoningRef.current = '';
    const abort = new AbortController();
    abortRef.current = abort;
    activeJobSessionRef.current = sessionID;
    forceScrollRef.current = true;
    stickToBottomRef.current = true;
    setMessages(prev => [...prev, { role: 'user', content: text, attachments: attachmentsForMessage }, { role: 'assistant-stream', answer: '', reasoning: '', events: [] }]);
    try {
      clearAttachments();
      setStreamStats(prev => ({ ...prev, state: 'streaming' }));
      let finalSession = null;
      await streamChat({
        authHeaders, signal: abort.signal, sessionID, message: text, attachmentIDs, providerID: selectedModelProvider?.choice_id || '', model: selectedChatModel, onEvent: (event, data) => {
          if (currentRef.current === sessionID) handleChatStreamEvent(event, data, s => { finalSession = s; });
        }
      });
      if (finalSession) {
        pendingDeltaRef.current = '';
        pendingReasoningRef.current = '';
        if (currentRef.current === sessionID) {
          finishActiveAssistant(finalSession);
          setCurrentTitle(finalSession.title || currentTitle || '新会话');
        }
        loadSessions().catch(() => { });
      }
    } catch (e) {
      if (abort.signal.aborted) {
        if (!detachedControllersRef.current.has(abort)) {
          appendReasoning(pendingReasoningRef.current);
          appendAnswer(pendingDeltaRef.current);
          appendAnswer('\n\n【已中断】');
        }
        await loadSessions().catch(() => { });
      } else {
        const message = readableChatError(e, attachmentsForMessage.some(attachmentLooksLikeImage));
        setStreamStats(prev => ({ ...prev, state: 'error', error: message }));
        appendToActiveAssistant(m => ({ ...m, error: { message }, answer: m.answer || '' }));
      }
    } finally {
      if (abortRef.current === abort || activeJobSessionRef.current === sessionID) {
        setBusy(false);
        if (abortRef.current === abort) abortRef.current = null;
        activeJobIDRef.current = '';
        activeJobSessionRef.current = '';
        pausedRef.current = false;
      }
    }
  }, [authHeaders, busy, detachedControllersRef, selectedModelBaseURL, selectedChatModel, selectedModelProvider, current, currentTitle, draftKey, input, pendingAttachmentIDs, pendingAttachments, readyAttachments, uploadingFiles, clearAttachments, createPersistedSession, loadSessions, appendAnswer, appendReasoning, appendToActiveAssistant, handleChatStreamEvent, finishActiveAssistant, openSettings, showToast]);


  const regenerateEditedReply = useCallback(async (sessionID, baseMessages, title) => {
    setBusy(true);
    pausedRef.current = false;
    setStreamStats({ state: 'connecting', started_at: Date.now(), chars: 0, events: 0, tools: 0, error: '' });
    pendingDeltaRef.current = '';
    pendingReasoningRef.current = '';
    const abort = new AbortController();
    abortRef.current = abort;
    activeJobSessionRef.current = sessionID;
    forceScrollRef.current = true;
    stickToBottomRef.current = true;
    setMessages([...(baseMessages || []), { role: 'assistant-stream', answer: '', reasoning: '', events: [] }]);
    try {
      setStreamStats(prev => ({ ...prev, state: 'streaming' }));
      let finalSession = null;
      await streamChat({
        authHeaders,
        signal: abort.signal,
        sessionID,
        message: '',
        attachmentIDs: [],
        providerID: selectedModelProvider?.choice_id || '',
        model: selectedChatModel,
        regenerate: true,
        onEvent: (event, data) => {
          if (currentRef.current === sessionID) handleChatStreamEvent(event, data, s => { finalSession = s; });
        }
      });
      if (finalSession) {
        pendingDeltaRef.current = '';
        pendingReasoningRef.current = '';
        if (currentRef.current === sessionID) {
          finishActiveAssistant(finalSession);
          setCurrentTitle(finalSession.title || title || '新会话');
        }
        loadSessions().catch(() => { });
      }
    } catch (e) {
      if (abort.signal.aborted) {
        appendReasoning(pendingReasoningRef.current);
        appendAnswer(pendingDeltaRef.current);
        appendAnswer('\n\n【已中断】');
        await loadSessions().catch(() => { });
      } else {
        const message = readableChatError(e);
        setStreamStats(prev => ({ ...prev, state: 'error', error: message }));
        appendToActiveAssistant(m => ({ ...m, error: { message }, answer: m.answer || '' }));
      }
    } finally {
      if (abortRef.current === abort || activeJobSessionRef.current === sessionID) {
        setBusy(false);
        if (abortRef.current === abort) abortRef.current = null;
        activeJobIDRef.current = '';
        activeJobSessionRef.current = '';
        pausedRef.current = false;
      }
    }
  }, [authHeaders, selectedModelProvider, selectedChatModel, handleChatStreamEvent, finishActiveAssistant, loadSessions, appendReasoning, appendAnswer, appendToActiveAssistant]);

  const editUserMessage = useCallback(async ({ messageIndex, messageID, content }) => {
    if (busy) {
      showToast('正在生成，先停止当前输出再编辑。', 'error');
      throw new Error('busy');
    }
    if (!current) return;
    if (!String(selectedModelBaseURL || '').trim() || !String(selectedChatModel || '').trim()) {
      showToast('请先配置模型供应商和模型，再保存编辑。', 'error');
      openSettings('model');
      throw new Error('model is not configured');
    }
    try {
      const next = await editSessionMessage(api, current, { messageIndex, messageID, content });
      const nextMessages = next.messages || [];
      setMessages(nextMessages);
      await loadSessions();
      showToast('已替换该消息，正在重新生成回复', 'success');
      void regenerateEditedReply(current, nextMessages, next.title || currentTitle || '新会话');
    } catch (error) {
      const message = error.message === 'busy' ? '正在生成' : error.message;
      showToast('编辑失败：' + message, 'error');
      throw error;
    }
  }, [api, busy, current, currentTitle, loadSessions, openSettings, regenerateEditedReply, selectedChatModel, selectedModelBaseURL, showToast]);

  const guideActiveJob = useCallback(async () => {
    if (!busy) return;
    const text = input.trim();
    if (!text) {
      showToast('先输入要追加的引导内容。', 'error');
      return;
    }
    const jobID = activeJobIDRef.current;
    if (!jobID) {
      showToast('当前生成还没有可引导的任务 ID。', 'error');
      return;
    }
    try {
      await guideChatJob(api, jobID, text);
      setInput('');
      showToast('引导已加入队列，会在下一轮模型调用前生效。', 'success');
    } catch (e) {
      showToast('引导失败：' + e.message, 'error');
    }
  }, [api, busy, input, showToast]);

  const stopStreaming = useCallback(async () => {
    if (!busy) return;
    setStreamStats(prev => ({ ...prev, state: 'stopping' }));
    const jobID = activeJobIDRef.current;
    if (jobID) {
      try { await cancelChatJob(api, jobID); } catch (e) { showToast('停止生成失败：' + e.message, 'error'); }
    }
    if (abortRef.current) abortRef.current.abort();
  }, [api, busy, showToast]);

  const resolveToolConfirmation = useCallback(async (id, approve) => {
    try {
      await resolveMCPConfirmation(api, id, approve);
      appendToActiveAssistant(m => ({ ...m, events: (m.events || []).map(item => item.confirmation?.id === id ? { ...item, status: 'resolved', text: (approve ? '✅ 已允许工具：' : '⛔ 已拒绝工具：') + (item.confirmation?.tool || 'MCP 工具') } : item) }));
      showToast(approve ? '已允许工具执行' : '已拒绝工具执行', approve ? 'success' : 'info');
    } catch (e) {
      showToast('确认工具失败：' + e.message, 'error');
    }
  }, [api, appendToActiveAssistant, showToast]);

  const createWorkspace = useCallback(async () => {
    if (busy) return;
    const values = await showDialog({
      title: '新增工作空间', confirmText: '创建工作空间', fields: [
        { name: 'name', label: '工作空间名称', value: '', required: true },
        { name: 'system_prompt', label: '系统提示词内容', type: 'textarea', rows: 5, value: config.system_prompt || '' },
      ]
    });
    if (!values || !values.name.trim()) return;
    await createWorkspaceRecord(api, { name: values.name.trim(), system_prompt: values.system_prompt || '' });
    setCurrent(null);
    setCurrentTitle('未选择会话');
    setMessages([{ role: 'empty', content: '已创建并切换到新工作空间。' }]);
    clearAttachments();
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig(), loadMCPConfig(), loadScheduledTasks(), loadSessions()]);
    closeSidebarOnMobile();
    showToast('工作空间已创建', 'success');
  }, [api, busy, closeSidebarOnMobile, config.system_prompt, loadConfig, loadMCPConfig, loadPrompts, loadScheduledTasks, loadSessions, refreshProductState, showDialog, showToast]);

  const deleteWorkspace = useCallback(async (id, name) => {
    const ok = await showDialog({ title: '删除工作空间', message: '确定删除工作空间「' + (name || id) + '」？这会删除该工作空间下的配置、任务和会话。若删除当前工作空间，会自动切换到默认工作空间。', confirmText: '删除', danger: true, type: 'confirm' });
    if (!ok) return;
    const data = await deleteWorkspaceRecord(api, id);
    setWorkspaces(data.workspaces || []);
    setCurrent(null);
    setCurrentTitle('未选择会话');
    setMessages([{ role: 'empty', content: '工作空间已删除。当前工作空间：' + (data.active || 'default') }]);
    clearAttachments();
    await Promise.allSettled([loadPrompts(), loadConfig(), loadMCPConfig(), loadScheduledTasks(), loadSessions(), loadSetupStatus(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
    showToast('工作空间已删除', 'success');
  }, [api, loadConfig, loadDataStatus, loadMCPConfig, loadModelProviders, loadPrompts, loadScheduledTasks, loadSessions, loadSetupStatus, loadSystemStatus, showDialog, showToast]);

  const saveConfig = useCallback(async () => {
    const workspaceID = (prompts.find(p => p.active) || {}).name || 'default';
    await saveWorkspaceConfig(api, workspaceID, {
      provider_id: config.provider_id,
      model: config.model,
      system_prompt: config.system_prompt,
      context_mode: config.context_mode || 'auto',
      max_context_messages: Number(config.max_context_messages || 12),
      temperature: Number(config.temperature || 0.7),
      enable_thinking: !!config.enable_thinking,
      hide_thinking: !!config.hide_thinking,
      embedding_base_url: config.embedding_base_url,
      embedding_api_key: config.embedding_api_key,
      embedding_model: config.embedding_model || 'BAAI/bge-m3',
    });
    setConfig(c => ({ ...c, api_key: '', embedding_api_key: '' }));
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
    const values = await showDialog({
      title: '首次配置', message: '配置默认工作空间和模型后即可开始对话。', confirmText: '完成初始化', fields: [
        { name: 'workspace_name', label: '默认工作空间名称', value: 'default', required: true },
        { name: 'base_url', label: '模型 Base URL', value: config.base_url || 'https://api.openai.com/v1', required: true },
        { name: 'model', label: '默认模型', value: config.model || 'gpt-4o-mini', required: true },
        { name: 'api_key', label: 'API Key（可留空）', type: 'password', value: '' },
        { name: 'system_prompt', label: '默认 System Prompt', type: 'textarea', rows: 4, value: config.system_prompt || '你是 ChatDock，本地优先 AI 工作台。默认用中文回答。' },
      ]
    });
    if (!values) return;
    await initializeSetup(api, values);
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig()]);
    showToast('初始化完成', 'success');
  }, [api, config, loadConfig, loadPrompts, refreshProductState, showDialog, showToast]);

  const testModelProvider = useCallback(async () => {
    try {
      const data = await testModelProviderRequest(api, {
        provider_id: config.provider_id,
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
        provider_id: config.provider_id,
        model: config.model,
        system_prompt: config.system_prompt,
        context_mode: config.context_mode || 'auto',
        max_context_messages: Number(config.max_context_messages || 12),
        temperature: Number(config.temperature || 0.7),
        enable_thinking: !!config.enable_thinking,
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
    const models = uniqueModelNames([...(provider.models || []), name]);
    await updateModelProviderRequest(api, provider.id, {
      name: provider.name || provider.id,
      base_url: provider.base_url || '',
      api_key: '********',
      default_model: provider.default_model || name,
      models,
      enabled: provider.enabled !== false,
      key_strategy: provider.key_strategy || provider.keyStrategy,
      selected_key_id: provider.selected_key_id || provider.selectedKeyID || '',
      api_keys: (provider.api_keys || provider.apiKeys || []).map(key => ({
        ...key,
        api_key: key.api_key || '********',
      })),
    });
    setConfig(c => ({
      ...c,
      provider_id: provider.id,
      base_url: provider.base_url || c.base_url || '',
      has_api_key: !!provider.has_api_key,
      model: name,
      models,
    }));
    await Promise.allSettled([loadModelProviders(), loadWorkspaces()]);
    showToast((provider.models || []).includes(name) ? '候选模型已在可用列表：' + name : '已加入可用模型列表：' + name, 'success');
  }, [api, candidateProviderID, config.provider_id, loadConfig, loadModelProviders, loadWorkspaces, providers, showToast]);

  const editModelProvider = useCallback(async (existing = null) => {
    const modelText = uniqueModelNames([...(existing?.models || []), existing?.default_model].filter(Boolean)).join('\n');
    const keyRows = providerKeyRows(existing);
    const selectedKeyID = existing?.selected_key_id || existing?.selectedKeyID || keyRows[0]?.id || 'main';
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
        { name: 'key_strategy', label: 'Key 策略', type: 'hidden', value: existing?.key_strategy || existing?.keyStrategy || 'auto' },
        { name: 'api_keys', label: 'Key 列表', type: 'provider_keys', value: keyRows, hint: '只需要填 Key 名称和 Key。ID 与优先级自动生成；当前 Key 用单选按钮切换；已保存 Key 只隐藏中间字段。' },
        { name: 'models', label: '可用模型（每行一个）', type: 'textarea', rows: 4, value: modelText || (existing?.default_model || ''), hint: '这里是真正会出现在聊天模型选择器里的模型。候选模型需要逐个加入。' },
        { name: 'enabled', label: '状态', type: 'select', value: existing && existing.enabled === false ? 'false' : 'true', options: [{ value: 'true', label: '启用' }, { value: 'false', label: '停用' }] },
      ]
    });
    if (!values) return;
    const apiKeys = providerKeyInputsFromRows(values.api_keys, '');
    const selectedFromDraft = String(values.selected_key_id || '').trim();
    const defaultModel = String(values.default_model || '').trim();
    const payload = {
      name: String(values.name || '').trim(),
      base_url: String(values.base_url || '').trim(),
      api_key: '',
      default_model: defaultModel,
      models: uniqueModelNames(values.models || defaultModel),
      enabled: values.enabled !== 'false',
      key_strategy: values.key_strategy || 'auto',
      selected_key_id: selectedFromDraft || (apiKeys?.[0]?.id || ''),
      api_keys: apiKeys || undefined,
    };
    if (existing) await updateModelProviderRequest(api, existing.id, payload);
    else await createModelProviderRequest(api, payload);
    await Promise.allSettled([loadModelProviders(), loadConfig(), loadSetupStatus(), loadWorkspaces()]);
    showToast(existing ? '模型供应商已保存' : '模型供应商已新增', 'success');
  }, [api, loadConfig, loadModelProviders, loadSetupStatus, loadWorkspaces, showDialog, showToast]);

  const deleteModelProvider = useCallback(async (provider) => {
    if (!provider?.id) return;
    const ok = await showDialog({ title: '删除模型供应商', message: '确定删除模型供应商「' + (provider.name || provider.id) + '」？正在被工作空间使用的供应商不会被删除。', confirmText: '删除', danger: true, type: 'confirm' });
    if (!ok) return;
    await deleteModelProviderRequest(api, provider.id);
    await Promise.allSettled([loadModelProviders(), loadConfig(), loadSetupStatus(), loadWorkspaces()]);
    showToast('模型供应商已删除', 'success');
  }, [api, loadConfig, loadModelProviders, loadSetupStatus, loadWorkspaces, showDialog, showToast]);

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

  const saveMCPConfig = useCallback(async () => {
    try { JSON.parse(mcpConfig || '{}'); } catch (e) { showToast('MCP 配置不是合法 JSON：' + e.message, 'error'); return; }
    const c = await saveMCPConfigRequest(api, mcpConfig);
    setMcpConfig(c.content || mcpConfig);
    await loadMCPStatus().catch(() => { });
    showToast('MCP 配置已保存', 'success');
  }, [api, loadMCPStatus, mcpConfig, showToast]);

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
      title: existing ? '编辑自动化任务' : '新增自动化任务', message: '选择调度类型后，只需要填写对应的时间字段。', confirmText: existing ? '保存任务' : '新增任务', fields: [
        { name: 'title', label: '任务标题', value: existing ? existing.title : '', required: true },
        { name: 'prompt', label: '任务提示词', type: 'textarea', rows: 6, value: existing ? (existing.prompt || '') : '', required: true },
        { name: 'schedule_type', label: '调度类型', type: 'select', value: existing ? existing.schedule_type : 'once', options: [{ value: 'once', label: '一次性' }, { value: 'daily', label: '每天固定时间' }, { value: 'interval', label: '按分钟间隔' }] },
        { name: 'run_at', label: '一次性运行时间', type: 'datetime-local', value: existing && existing.run_at ? existing.run_at.slice(0, 16) : defaultRunAtValue(), showWhen: { schedule_type: 'once' } },
        { name: 'time_of_day', label: '每天运行时间', type: 'time', value: existing ? (existing.time_of_day || '09:00') : '09:00', showWhen: { schedule_type: 'daily' } },
        { name: 'interval_minutes', label: '间隔分钟数', type: 'number', min: 1, step: 1, value: existing && existing.interval_minutes ? String(existing.interval_minutes) : '60', showWhen: { schedule_type: 'interval' }, hint: '当前本地调度器最低按分钟执行；过短间隔会更频繁占用模型额度。' },
        { name: 'context_mode', label: '上下文模式', type: 'select', value: existing ? (existing.context_mode || 'stateless') : 'stateless', options: [{ value: 'stateless', label: '每次独立执行，最省 token' }, { value: 'last_result', label: '带上次运行结果' }, { value: 'session', label: '连续会话，保留完整上下文' }], hint: '默认独立执行：只使用本次任务提示词；需要长期上下文时再选择连续会话。' },
      ]
    });
    if (!values) return;
    const titleValue = (values.title || '').trim();
    const promptValue = (values.prompt || '').trim();
    const typeValue = (values.schedule_type || '').trim().toLowerCase();
    if (!titleValue || !promptValue) { showToast('任务标题和提示词不能为空', 'error'); return; }
    if (!['once', 'daily', 'interval'].includes(typeValue)) { showToast('调度类型只能是 once、daily 或 interval', 'error'); return; }
    const contextMode = ['stateless', 'last_result', 'session'].includes(values.context_mode) ? values.context_mode : 'stateless';
    const payload = { title: titleValue, prompt: promptValue, enabled: existing ? !!existing.enabled : true, schedule_type: typeValue, context_mode: contextMode };
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
    const payload = { title: existing.title, prompt: existing.prompt, enabled: !!enabled, schedule_type: existing.schedule_type, run_at: existing.run_at || '', time_of_day: existing.time_of_day || '', interval_minutes: existing.interval_minutes || 0, context_mode: existing.context_mode || 'stateless' };
    const data = await saveScheduledTaskRecord(api, existing, payload);
    setScheduledTasks(data.tasks || []);
  }, [api, scheduledTasks]);

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
      closeSettings();
    } catch (e) { showToast('打开运行会话失败：' + e.message, 'error'); }
  }, [closeSettings, openSession, showToast]);

  const openScheduledTaskRunList = useCallback(async (taskID) => {
    const id = String(taskID || '').trim();
    if (!id) return;
    try {
      const data = await fetchScheduledTaskRuns(api, id);
      setSelectedScheduledTaskID(id);
      setSelectedScheduledTaskRuns(data.runs || []);
      setSessionSearch('');
      setSessionSearchResults([]);
    } catch (e) { showToast('读取任务会话失败：' + e.message, 'error'); }
  }, [api, showToast]);

  const clearScheduledTaskRunList = useCallback(() => {
    setSelectedScheduledTaskID('');
    setSelectedScheduledTaskRuns([]);
  }, []);

  const viewScheduledTaskRuns = useCallback(async (id) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    try {
      const data = await fetchScheduledTaskRuns(api, id);
      const runs = data.runs || [];
      setSelectedScheduledTaskID(id);
      setSelectedScheduledTaskRuns(runs);
      setSessionSearch('');
      setSessionSearchResults([]);
      closeSettings();
      showToast(runs.length ? ('已在侧边栏展示 ' + runs.length + ' 条运行会话') : '这个任务还没有运行记录', runs.length ? 'success' : 'info');
    } catch (e) { showToast('读取运行记录失败：' + e.message, 'error'); }
  }, [api, closeSettings, scheduledTasks, showToast]);


  const runScheduledTaskNow = useCallback(async (id) => {
    const existing = scheduledTasks.find(t => t.id === id);
    if (!existing) return;
    const ok = await showDialog({ title: '立即运行任务', message: '立即运行定时任务「' + existing.title + '」？', confirmText: '立即运行', type: 'confirm' });
    if (!ok) return;
    try {
      const result = await runScheduledTask(api, id);
      const runsPromise = selectedScheduledTaskID === id ? fetchScheduledTaskRuns(api, id).catch(() => null) : Promise.resolve(null);
      const [runsData] = await Promise.all([
        runsPromise,
        loadScheduledTasks(),
        loadSessions(),
        refreshProductState(),
      ]);
      if (runsData) setSelectedScheduledTaskRuns(runsData.runs || []);
      if (result.session && result.session.id) {
        await openScheduledTaskSession(result.session.id);
      }
      showToast(result.session ? '定时任务已运行，已打开运行会话' : '定时任务已运行，结果已写入运行记录', 'success');
    } catch (e) { await loadScheduledTasks().catch(() => { }); showToast('运行失败：' + e.message, 'error'); }
  }, [api, loadScheduledTasks, loadSessions, openScheduledTaskSession, refreshProductState, scheduledTasks, selectedScheduledTaskID, showDialog, showToast]);

  const inspectToolEvent = useCallback(async (event) => {
    if (!event?.details) return;
    let detailEvent = event;
    const ref = event.details || {};
    if (ref.lazy) {
      const sessionID = ref.session_id || current;
      const messageIndex = ref.message_index;
      const eventID = ref.event_id || event.id || '';
      const hasEventIndex = ref.event_index !== undefined && ref.event_index !== null;
      const hasPartIndex = ref.part_index !== undefined && ref.part_index !== null;
      if (!sessionID || (!eventID && (messageIndex === undefined || (!hasEventIndex && !hasPartIndex)))) return;
      const cacheKey = eventID ? [sessionID, eventID].join(':') : [sessionID, messageIndex, hasEventIndex ? ref.event_index : '', hasPartIndex ? ref.part_index : ''].join(':');
      try {
        if (!toolEventDetailCacheRef.current.has(cacheKey)) {
          const data = await fetchSessionToolEvent(api, sessionID, ref);
          toolEventDetailCacheRef.current.set(cacheKey, data.event || event);
        }
        detailEvent = toolEventDetailCacheRef.current.get(cacheKey) || event;
      } catch (e) {
        showToast('工具事件详情加载失败：' + e.message, 'error');
        return;
      }
    }
    await showDialog({ title: '工具事件详情', confirmText: '关闭', hideCancel: true, variant: 'tool-event-modal', toolEventDetail: buildToolEventDetail(detailEvent) });
  }, [api, current, showDialog, showToast, toolEventDetailCacheRef]);

  const showContextPreview = useCallback(async () => {
    if (!current) return;
    try {
      const preview = await fetchContextPreview(api, current);
      await showDialog({ title: '上下文 / Token 预览', confirmText: '关闭', hideCancel: true, fields: [{ name: 'preview', label: '实际会发送给模型的上下文（token 为粗略估算）', type: 'textarea', rows: 16, value: contextPreviewText(preview) }] });
    } catch (e) {
      showToast('上下文预览失败：' + e.message, 'error');
    }
  }, [api, current, showDialog, showToast, toolEventDetailCacheRef]);

  const logout = useCallback(() => {
    localStorage.removeItem('chatdock.authToken');
    setAuthPage({ message: '请输入 ChatDock 账号和密码。' });
  }, []);

  const activeScheduledTasks = useMemo(() => scheduledTasks.filter(task => task.enabled || task.running), [scheduledTasks]);
  const selectedScheduledTask = useMemo(() => scheduledTasks.find(task => task.id === selectedScheduledTaskID) || null, [scheduledTasks, selectedScheduledTaskID]);

  useEffect(() => {
    if (selectedScheduledTaskID && !scheduledTasks.some(task => task.id === selectedScheduledTaskID)) {
      setSelectedScheduledTaskID('');
      setSelectedScheduledTaskRuns([]);
    }
  }, [scheduledTasks, selectedScheduledTaskID]);

  const selectedScheduledTaskSessions = useMemo(() => scheduledTaskSessionRows({
    selectedScheduledTaskID, selectedScheduledTaskRuns, selectedScheduledTask, sessions,
  }), [selectedScheduledTask, selectedScheduledTaskID, selectedScheduledTaskRuns, sessions]);

  const filteredSessions = useMemo(() => visibleSessionRows({
    sessionSearch, sessionSearchResults, selectedScheduledTaskID, selectedScheduledTaskSessions, sessions,
  }), [selectedScheduledTaskID, selectedScheduledTaskSessions, sessionSearch, sessionSearchResults, sessions]);

  const currentSummary = useMemo(() => sessions.find(s => s.id === current) || null, [current, sessions]);
  const currentPinned = !!currentSummary?.pinned;
  const appClass = 'app ' + (sidebarCollapsed ? 'sidebar-collapsed ' : '') + (settingsOpen ? 'settings-open' : '');
  const productReady = setupStatus && !setupStatus.needs_setup;
  const modelReady = !!String(selectedModelBaseURL || '').trim() && !!String(selectedChatModel || '').trim();
  const productStatusText = setupStatus == null ? '加载中' : (productReady ? '就绪' : '待配置');
  const productStatusClass = setupStatus == null ? 'warn' : (productReady ? 'ok' : 'warn');
  const streamElapsed = streamStats.started_at ? Math.max(0, Math.round((Date.now() - streamStats.started_at) / 1000)) : 0;
  const streamStatsText = busy ? streamStatusText(streamStats, streamElapsed) : '';
  const inputStats = busy ? streamStatsText : (pendingAttachments.length ? pendingAttachments.length + ' 个附件' + (input.trim() ? ' · ' + input.trim().length + ' 字' : '') : (input.trim() ? input.trim().length + ' 字' : (!modelReady ? '请先在配置中心完成模型 Base URL 和 Model' : '')));
  const productDiagnostics = diagnosticsText({ setupStatus, systemStatus, dataStatus, mcpStatus, providers });
  const hasVisibleChatMessages = messages.some(m => m.role !== 'empty');

  const quickActions = useMemo(() => buildQuickActions({
    branchCurrent, busy, cloneCurrent, copyCurrentMarkdown, copyText, createSession, current, currentPinned,
    deleteCurrent, exportCurrent, inputRef, messagesLength: messages.length, openSettings, pinCurrent,
    productDiagnostics, promptsLength: prompts.length, renameCurrent, sendMsg, setThemeState, setWorkspacePickerOpen,
    showContextPreview, theme,
  }), [branchCurrent, busy, cloneCurrent, copyCurrentMarkdown, copyText, createSession, current, currentPinned, deleteCurrent, exportCurrent, messages.length, openSettings, pinCurrent, productDiagnostics, prompts.length, renameCurrent, sendMsg, showContextPreview, theme]);

  const settingsPanel = (
    <SettingsPanel
      activeModule={activeModule} busy={busy} closeSettings={closeSettings} config={config}
      createWorkspace={createWorkspace} dataStatus={dataStatus} deleteScheduledTask={deleteScheduledTask} deleteWorkspace={deleteWorkspace}
      editModelProvider={editModelProvider} deleteModelProvider={deleteModelProvider} testSavedModelProvider={testSavedModelProvider} fetchSavedProviderModels={fetchSavedProviderModels}
      editScheduledTask={editScheduledTask} loadDataStatus={loadDataStatus} loadMCPConfig={loadMCPConfig}
      loadMCPStatus={loadMCPStatus} loadScheduledTasks={loadScheduledTasks} loadSystemStatus={loadSystemStatus}
      mcpConfig={mcpConfig} mcpStatus={mcpStatus} onCopy={copyText} providers={providers} promptPreview={promptPreview} refreshProductState={refreshProductState} refreshVisibleSettings={refreshVisibleSettings}
      runScheduledTaskNow={runScheduledTaskNow} viewScheduledTaskRuns={viewScheduledTaskRuns} openScheduledTaskSession={openScheduledTaskSession} runSetupWizard={runSetupWizard} saveConfig={saveConfig} saveMCPConfig={saveMCPConfig}
      scheduledTasks={scheduledTasks} selectWorkspace={selectWorkspace} setConfig={setConfig} setMcpConfig={setMcpConfig} setTaskSearch={setTaskSearch}
      setupStatus={setupStatus} showPromptPreview={showPromptPreview} switchSettingsModule={switchSettingsModule}
      systemStatus={systemStatus} taskSearch={taskSearch} testMCP={testMCP} fetchMCPServerTools={fetchMCPServerTools} testModelProvider={testModelProvider} fetchProviderModels={fetchProviderModels} availableModels={availableModels} candidateProviderID={candidateProviderID} addCandidateModelToProvider={addCandidateModelToProvider} loadingModels={loadingModels} toggleScheduledTask={toggleScheduledTask}
      workspaces={workspaces} logout={logout}
    />
  );

  return <>
    <div id="sidebarMask" className={'sidebar-mask ' + (!settingsOpen && !sidebarCollapsed ? 'show' : '')} role="presentation" onClick={() => setSidebarCollapsed(true)} />
    {settingsOpen ? <div id="settingsPage" className="settings-page">{settingsPanel}</div> : <div id="app" className={appClass}>
      <Sidebar
        activePrompt={activePrompt} activeScheduledTasks={activeScheduledTasks} busy={busy} clearScheduledTaskRunList={clearScheduledTaskRunList}
        current={current} deleteSessionByID={deleteSessionByID} filteredSessions={filteredSessions} newSession={newSession}
        openScheduledTaskRunList={openScheduledTaskRunList} openSession={openSession} openSettings={openSettings}
        pinSessionByID={pinSessionByID} prompts={prompts} renameSessionByID={renameSessionByID}
        selectedScheduledTask={selectedScheduledTask} selectedScheduledTaskID={selectedScheduledTaskID} selectedScheduledTaskSessions={selectedScheduledTaskSessions}
        sessionMenuID={sessionMenuID} sessionSearch={sessionSearch} sessionSearchBusy={sessionSearchBusy}
        setSessionMenuID={setSessionMenuID} setSessionSearch={setSessionSearch} setWorkspacePickerOpen={setWorkspacePickerOpen}
        sessions={sessions} setSidebarCollapsed={setSidebarCollapsed} sidebarCollapsed={sidebarCollapsed}
      />
      <main>
        <Topbar
          busy={busy} cloneCurrent={cloneCurrent} copyCurrentMarkdown={copyCurrentMarkdown} current={current}
          currentPinned={currentPinned} currentTitle={currentTitle} deleteCurrent={deleteCurrent} exportCurrent={exportCurrent}
          newSession={newSession} openSettings={openSettings} pinCurrent={pinCurrent} renameCurrent={renameCurrent}
          setQuickPaletteOpen={setQuickPaletteOpen} setSidebarCollapsed={setSidebarCollapsed} setThemeState={setThemeState}
          showContextPreview={showContextPreview} sidebarCollapsed={sidebarCollapsed} theme={theme}
        />
        <div className="messages" ref={messagesRef} onScroll={handleMessagesScroll}>{messages.length ? messages.map((m, i) => <MessageView key={messageKey(m, i)} message={m} messageIndex={i} onCopy={copyText} onBranch={!busy && current ? branchCurrent : null} onEditUserMessage={editUserMessage} onDownloadAttachment={downloadAttachment} hideThinking={!!config.hide_thinking} onResolveConfirmation={resolveToolConfirmation} onInspectToolEvent={inspectToolEvent} />) : <EmptyState createSession={createSession} openSettings={openSettings} openWorkspacePicker={() => setWorkspacePickerOpen(true)} busy={busy} hasWorkspaces={!!prompts.length} setInput={setInput} modelReady={modelReady} />}</div>
        {showJumpToLatest ? <button type="button" className="jump-latest" onClick={scrollToLatestModelMessage} aria-label="跳到最新模型消息" title="跳到最新模型消息">↓</button> : null}
        <ComposerBar
          busy={busy} createPersistedSession={createPersistedSession} current={current} downloadAttachment={downloadAttachment}
          fileInputRef={fileInputRef} guideActiveJob={guideActiveJob} handleFileSelect={handleFileSelect}
          input={input} inputRef={inputRef} inputStats={inputStats} modelPickerOpen={modelPickerOpen} modelReady={modelReady}
          openSettings={openSettings} pendingAttachmentIDs={pendingAttachmentIDs} pendingAttachments={pendingAttachments}
          providerChoices={providerChoices} removePendingAttachment={removePendingAttachment} selectChatModel={selectChatModel}
          selectedChatModel={selectedChatModel} selectedModelProvider={selectedModelProvider} sendMsg={sendMsg}
          setInput={setInput} setModelPickerOpen={setModelPickerOpen} stopStreaming={stopStreaming} uploadingFiles={uploadingFiles}
        />
      </main>

    </div>}
    <WorkspacePicker open={workspacePickerOpen} prompts={prompts} busy={busy} activeName={activePrompt?.name || ''} onClose={() => setWorkspacePickerOpen(false)} onSelect={selectWorkspace} />
    <QuickPalette open={quickPaletteOpen} actions={quickActions} onClose={() => setQuickPaletteOpen(false)} />
    {authPage ? <LoginPage api={api} error={authPage} refreshAfterLogin={refreshAfterLogin} setAuthPage={setAuthPage} /> : null}
    <DialogHost dialog={dialog} closeDialog={closeDialog} />
    {toast ? <div id="appToast" className={'app-toast show ' + toast.variant}>{toast.message}</div> : null}
  </>;
}
