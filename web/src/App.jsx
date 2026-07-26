import React, { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowDown } from './components/icons.js';
import { EmptyState, MemoizedMessageView } from './components/chat.jsx';
import { ComposerBar, Sidebar, Topbar } from './components/appChrome.jsx';
import { CurrentSessionTask, TaskPanel } from './components/taskPanel.jsx';
import { DialogHost, LoginPage, Markdown, PageLoadingState, QuickPalette } from './components/base.jsx';
import { agentTaskDataEnabled, diagnosticsText, filenameFromResponse, logoutAndReload, normalizeSettingsModule, sessionIDFromPath, sessionPath, setSettingsDocumentScroll, settingsModuleFromPath } from './lib/appUtils.js';
import { attachmentLooksLikeImage, chatErrorDetails, contextPreviewText, finalAssistantMessageFromSession, readableChatError, streamStatusText } from './lib/chatPresentation.js';
import { buildToolEventDetail } from './lib/toolEventDetails.js';
import { deleteAgentTask as deleteAgentTaskRequest } from './lib/agentTaskApi.js';
import { createJsonApi } from './lib/http.js';
import { cancelChatJob, fetchChatJobs, guideChatJob, resolveMCPConfirmation, streamChat, streamChatJobEvents } from './lib/chatApi.js';
import { branchSession, cloneSession, createSessionRecord, deleteSession, editSessionMessage, fetchContextPreview, fetchSession, fetchSessionMarkdown, fetchSessionSystemPrompt, fetchSessionToolEvent, pinSession, renameSession, updateSessionModel } from './lib/sessionApi.js';
import { useAttachments } from './hooks/useAttachments.js';
import { useAgentTasks } from './hooks/useAgentTasks.js';
import { useCurrentSessionTask } from './hooks/useCurrentSessionTask.js';
import { useActiveAssistantStream } from './hooks/useActiveAssistantStream.js';
import { chatStreamAssistantAfterEvent, chatStreamStatsAfterEvent, messagesForRunningJobReplay, projectsChatStreamAssistant } from './lib/chatStreamEvents.js';
import { globalDefaultModelChoice, providerChoiceID, providerLabel, sessionModelChoice, uniqueModelNames } from './lib/modelProviderForm.js';
import { useSettingsData } from './hooks/useSettingsData.js';
import { useSettingsActions } from './hooks/useSettingsActions.js';
import { useSessionList } from './hooks/useSessionList.js';
import { useVisualViewportLayout } from './hooks/useVisualViewportLayout.js';
import { useMessageAutoFollow } from './hooks/useMessageAutoFollow.js';
import { buildQuickActions } from './lib/quickActions.js';
import { visibleSessionRows } from './lib/sessionPresentation.js';
const SettingsPanel = lazy(() => import('./components/settings.jsx').then(module => ({default: module.SettingsPanel})));
export default function App() {
  useVisualViewportLayout();

  const [authPage, setAuthPage] = useState(null);
  const [theme, setThemeState] = useState(() => localStorage.getItem('chatdock.theme.v2') === 'night' ? 'night' : 'day');
  const [sidebarCollapsed, setSidebarCollapsedState] = useState(() => {
    const saved = localStorage.getItem('chatdock.sidebarCollapsed');
    return saved == null ? window.matchMedia('(max-width: 720px)').matches : saved === '1';
  });
  const [settingsOpen, setSettingsOpen] = useState(() => !!settingsModuleFromPath());
  const [activeModule, setActiveModule] = useState(() => normalizeSettingsModule(settingsModuleFromPath() || localStorage.getItem('chatdock.settingsModule') || 'model'));
  const [toast, setToast] = useState(null);
  const toastTimerRef = useRef(null);
  const [dialog, setDialog] = useState(null);
  const [projectFilter, setProjectFilterState] = useState(() => localStorage.getItem('chatdock.projectFilter') || 'plain');
  const [quickPaletteOpen, setQuickPaletteOpen] = useState(false);
  const [modelPickerOpen, setModelPickerOpen] = useState(false);
  const [taskPanelOpen, setTaskPanelOpen] = useState(false);
  const [deletingAgentTaskID, setDeletingAgentTaskID] = useState('');
  const [chatModel, setChatModel] = useState({ provider_id: '', model: '' });
  const [showJumpToLatest, setShowJumpToLatest] = useState(false);

  const [sessionMenuID, setSessionMenuID] = useState('');
  const [current, setCurrent] = useState(null);
  const [currentTitle, setCurrentTitle] = useState('未选择会话');
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [busy, setBusy] = useState(false);
  const [streamPaused, setStreamPaused] = useState(false);
  const [streamStats, setStreamStats] = useState({ state: 'idle', started_at: 0, chars: 0, events: 0, tools: 0, error: '' });
  const [activeJobID, setActiveJobID] = useState('');
  const [taskSearch, setTaskSearch] = useState('');

  const {
    messagesRef,
    scrollToLatestModelMessage,
    resetMessageAutoFollow,
    handleMessagesScroll,
    handleMessagesWheel,
    handleMessagesTouchStart,
    handleMessagesTouchMove,
    handleMessagesTouchEnd,
  } = useMessageAutoFollow({ messages, setShowJumpToLatest });

  const abortRef = useRef(null);
  const activeJobIDRef = useRef('');
  const activeJobSessionRef = useRef('');
  const detachedControllersRef = useRef(new WeakSet());
  const currentRef = useRef(null);
  const pausedRef = useRef(false);
  const pendingDeltaRef = useRef('');
  const pendingReasoningRef = useRef('');
  const inputRef = useRef(null);
  const fileInputRef = useRef(null);
  const sessionOpenSeqRef = useRef(0);
  const toolEventDetailCacheRef = useRef(new Map());

  const {
    updateAssistant: appendToActiveAssistant, appendAnswer, appendReasoning,
    enqueue: enqueueStreamText, flush: flushStreamText, reset: resetStreamText, waitUntilIdle: waitForStreamText,
  } = useActiveAssistantStream(setMessages);

  useEffect(() => { pausedRef.current = streamPaused; }, [streamPaused]);
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
    resetStreamText();
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
    setActiveJobID('');
    setBusy(false);
    setStreamPaused(false);
    setStreamStats({ state: 'idle', started_at: 0, chars: 0, events: 0, tools: 0, error: '' });
  }, [resetStreamText]);

  useEffect(() => {
    if (busy && current && activeJobSessionRef.current && activeJobSessionRef.current !== current) {
      detachActiveStream();
    }
  }, [busy, current, detachActiveStream]);

  useEffect(() => {
    document.body.classList.toggle('theme-light', theme === 'day');
    document.body.classList.toggle('theme-night', theme !== 'day');
    localStorage.setItem('chatdock.theme.v2', theme);
  }, [theme]);

  useEffect(() => {
    document.body.classList.toggle('auth-page-visible', !!authPage);
    return () => document.body.classList.remove('auth-page-visible');
  }, [authPage]);

  useEffect(() => { setSettingsDocumentScroll(settingsOpen); return () => setSettingsDocumentScroll(false); }, [settingsOpen]);

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
    sessions,
    pinnedSessions,
    pinnedProjects,
    pinnedTasks,
    sessionSearch,
    setSessionSearch,
    sessionSearchResults,
    sessionSearchBusy,
    sessionsHasMore,
    sessionsLoadingMore,
    sessionSearchHasMore,
    sessionSearchLoadingMore,
    loadSessions,
    loadPinnedFeed,
    loadMoreSessions,
    loadMoreSearchSessions,
    upsertSession,
    upsertPinnedProject,
    upsertPinnedTask,
    removeSession,
  } = useSessionList(api, 'plain');

  const {
    setupStatus,
    projects,
    projectsLoaded,
    projectSessionCounts,
    providers,
    scheduledTasks,
    setScheduledTasks,
    dataStatus,
    systemStatus,
    mcpStatus,
    projectPromptPreview,
    setProjectPromptPreview,
    mcpConfig,
    setMcpConfig,
    builtinTools,
    config,
    setConfig,
    configDirty,
    mcpConfigDirty,
    loadConfig,
    loadMCPConfig,
    loadSetupStatus,
    loadProjects,
    loadModelProviders,
    loadScheduledTasks,
    loadDataStatus,
    loadSystemStatus,
    loadMCPStatus,
  } = useSettingsData(api);

  useEffect(() => {
    if (!configDirty && !mcpConfigDirty) return undefined;
    const warnBeforeUnload = (event) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', warnBeforeUnload);
    return () => window.removeEventListener('beforeunload', warnBeforeUnload);
  }, [configDirty, mcpConfigDirty]);

  const taskDataEnabled = agentTaskDataEnabled(setupStatus, systemStatus, !!authPage);
  const agentTasks = useAgentTasks(api, taskDataEnabled, taskPanelOpen);
  const currentSessionTask = useCurrentSessionTask(api, current, taskDataEnabled, busy);
  const closeTaskPanel = useCallback(() => {
    agentTasks.setExpandedTaskID('');
    setTaskPanelOpen(false);
  }, [agentTasks.setExpandedTaskID]);
  const toggleTaskPanel = useCallback(() => {
    if (taskPanelOpen) agentTasks.setExpandedTaskID('');
    setTaskPanelOpen(!taskPanelOpen);
  }, [agentTasks.setExpandedTaskID, taskPanelOpen]);

  const deleteAgentTaskFromPanel = useCallback(async (task) => {
    if (!task?.id || deletingAgentTaskID) return;
    const confirmed = await showDialog({
      title: '删除任务',
      message: `确定删除任务「${task.title || task.id}」？任务记录和步骤将被永久删除，此操作不可恢复。`,
      confirmText: '删除',
      danger: true,
      type: 'confirm',
    });
    if (!confirmed) return;

    setDeletingAgentTaskID(task.id);
    try {
      await deleteAgentTaskRequest(api, task.id);
      if (agentTasks.expandedTaskID === task.id) agentTasks.setExpandedTaskID('');
      await Promise.allSettled([
        agentTasks.refresh({ initial: true }),
        currentSessionTask.refresh({ initial: true }),
      ]);
      showToast(`任务「${task.title || task.id}」已删除`, 'success');
    } catch (err) {
      showToast('删除任务失败：' + (err.message || '未知错误'), 'error');
    } finally {
      setDeletingAgentTaskID('');
    }
  }, [agentTasks.expandedTaskID, agentTasks.refresh, agentTasks.setExpandedTaskID, api, currentSessionTask.refresh, deletingAgentTaskID, showDialog, showToast]);

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

  const refreshProductState = useCallback(async () => {
    await Promise.allSettled([loadSetupStatus(), loadProjects(), loadModelProviders(), loadDataStatus(), loadSystemStatus()]);
  }, [loadSetupStatus, loadProjects, loadModelProviders, loadDataStatus, loadSystemStatus]);

  const refreshVisibleSettings = useCallback(async () => {
    const jobs = [loadSetupStatus(), loadProjects(), loadModelProviders(), loadDataStatus(), loadSystemStatus()];
    if (activeModule === 'tools') jobs.push(loadMCPStatus());
    await Promise.allSettled(jobs);
    showToast('配置中心已刷新', 'success');
  }, [activeModule, loadDataStatus, loadMCPStatus, loadModelProviders, loadSetupStatus, loadSystemStatus, loadProjects, showToast]);

  const setProjectFilter = useCallback((value) => {
    const next = String(value || '').trim() || 'plain';
    setProjectFilterState(next);
    localStorage.setItem('chatdock.projectFilter', next);
    setSessionMenuID('');
  }, []);

  useEffect(() => {
    if (!projectsLoaded) return;
    if (projectFilter === 'all' || projectFilter === 'plain') return;
    if (projects.some(project => project.id === projectFilter)) return;
    setProjectFilter('plain');
  }, [projectFilter, projects, projectsLoaded, setProjectFilter]);

  const loadSessionFromRoute = useCallback(async (id) => {
    if (!id) return false;
    const seq = sessionOpenSeqRef.current + 1;
    sessionOpenSeqRef.current = seq;
    if (abortRef.current) detachActiveStream();
    setCurrent(null);
    setCurrentTitle('正在加载会话…');
    setMessages([{ role: 'loading', content: '正在加载会话' }]);
    clearAttachments();
    resetMessageAutoFollow();

    const s = await fetchSession(api, id);
    if (sessionOpenSeqRef.current !== seq) return false;
    setCurrent(s.id);
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    applySessionModel(s);
    upsertSession(s);
    setProjectFilter(s.project_id || 'plain');
    return true;
  }, [api, applySessionModel, clearAttachments, detachActiveStream, resetMessageAutoFollow, setProjectFilter, upsertSession]);

  const refreshAfterLogin = useCallback(async () => {
    await Promise.allSettled([refreshProductState(), loadConfig(), loadMCPConfig(), loadScheduledTasks(), loadSessions({reset: true})]);
    const routeSession = sessionIDFromPath();
    if (routeSession) await loadSessionFromRoute(routeSession).catch(e => showToast('会话路由加载失败：' + e.message, 'error'));
  }, [refreshProductState, loadConfig, loadMCPConfig, loadScheduledTasks, loadSessions, loadSessionFromRoute, showToast]);

  useEffect(() => {
    if (authPage || !setupStatus || setupStatus.needs_setup) return;
    loadSessions({reset: true}).catch(() => {});
  }, [authPage, loadSessions, projectFilter, setupStatus]);

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
    if (activeModule === 'projects') loadProjects().catch(e => showToast('项目加载失败：' + e.message, 'error'));
    if (activeModule === 'automation') loadScheduledTasks().catch(e => showToast('定时任务加载失败：' + e.message, 'error'));
    if (activeModule === 'security') {
      loadSystemStatus().catch(e => showToast('系统状态加载失败：' + e.message, 'error'));
      loadDataStatus().catch(e => showToast('数据状态加载失败：' + e.message, 'error'));
    }
  }, [settingsOpen, activeModule, loadMCPStatus, loadDataStatus, loadSystemStatus, loadProjects, loadScheduledTasks, showToast]);

  const setSidebarCollapsed = useCallback((value) => {
    setSidebarCollapsedState(current => {
      const next = typeof value === 'function' ? value(current) : value;
      localStorage.setItem('chatdock.sidebarCollapsed', next ? '1' : '0');
      return next;
    });
  }, []);

  const closeSidebarOnMobile = useCallback(() => {
    if (window.matchMedia('(max-width: 720px)').matches) setSidebarCollapsed(true);
  }, [setSidebarCollapsed]);

  const goHome = useCallback(() => {
    if (busy) detachActiveStream();
    sessionOpenSeqRef.current += 1;
    setSessionMenuID('');
    setSettingsOpen(false);
    setSessionSearch('');
    setCurrent(null);
    setCurrentTitle('未选择会话');
    setMessages([]);
    clearAttachments();
    setChatModel({ provider_id: '', model: '' });
    if (window.location.pathname !== '/') window.history.pushState({ chatdock: true }, '', '/');
    closeSidebarOnMobile();
  }, [busy, clearAttachments, closeSidebarOnMobile, detachActiveStream, setSessionSearch]);

  const openProjectSessions = useCallback((projectID) => {
    setProjectFilter(projectID);
    goHome();
  }, [goHome, setProjectFilter]);

  const openSettings = useCallback((moduleName = activeModule, syncRoute = true) => {
    const normalized = normalizeSettingsModule(moduleName);
    setActiveModule(normalized);
    setSettingsOpen(true);
    setTaskPanelOpen(false);
    setSessionMenuID('');
    localStorage.setItem('chatdock.settingsModule', normalized);
    if (syncRoute && window.location.pathname !== '/settings/' + normalized) window.history.pushState({ chatdock: true }, '', '/settings/' + normalized);
    closeSidebarOnMobile();
  }, [activeModule, closeSidebarOnMobile]);

  const openManagementPage = useCallback((page) => {
    openSettings(page === 'automation' ? 'automation' : 'projects');
  }, [openSettings]);

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
      else if (settingsOpen) closeSettings();
    }
    window.addEventListener('keydown', closeTopLayer);
    return () => window.removeEventListener('keydown', closeTopLayer);
  }, [closeDialog, closeSettings, dialog, quickPaletteOpen, settingsOpen]);

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

  const createPersistedSession = useCallback(async ({ refreshList = true } = {}) => {
    const s = await createSessionRecord(api, { projectID: projectFilter !== 'all' && projectFilter !== 'plain' ? projectFilter : '' });
    setCurrent(s.id);
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    clearAttachments();
    applySessionModel(s, { fallbackToDefault: false });
    if (refreshList) upsertSession(s);
    void loadProjects().catch(() => {});
    if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({ chatdock: true }, '', sessionPath(s.id));
    return s;
  }, [api, applySessionModel, clearAttachments, loadProjects, projectFilter, upsertSession]);

  const createSession = useCallback(() => {
    if (busy) detachActiveStream();
    // “新会话”只是进入一个本地草稿，不应该提前写入后端。
    // 真正的 session id 只有在发送首条消息、或上传附件需要绑定会话时才创建。
    setSessionMenuID('');
    setSettingsOpen(false);
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

  const startProjectConversation = useCallback((projectID) => {
    const normalizedProjectID = String(projectID || '').trim();
    if (!normalizedProjectID) return;

    // 项目对话仍沿用“发送首条消息时再落库”的会话语义，只提前固定项目上下文。
    setProjectFilter(normalizedProjectID);
    createSession();
  }, [createSession, setProjectFilter]);

  const openSession = useCallback(async (id, summary = null) => {
    if (!id) return;
    const seq = sessionOpenSeqRef.current + 1;
    sessionOpenSeqRef.current = seq;
    if (busy) detachActiveStream();
    setSessionMenuID('');
    setSettingsOpen(false);
    setCurrent(null);
    setCurrentTitle(summary?.title || '正在加载会话…');
    clearAttachments();
    resetMessageAutoFollow();
    setMessages([{ role: 'loading', content: '正在加载会话' }]);
    if (window.location.pathname !== sessionPath(id)) window.history.pushState({ chatdock: true }, '', sessionPath(id));
    closeSidebarOnMobile();
    try {
      const s = await fetchSession(api, id);
      if (sessionOpenSeqRef.current !== seq) return;
      setCurrent(s.id || id);
      setCurrentTitle(s.title || summary?.title || '新会话');
      setMessages(s.messages || []);
      applySessionModel(s);
      upsertSession(s);
      setProjectFilter(s.project_id || 'plain');
    } catch (e) {
      if (sessionOpenSeqRef.current !== seq) return;
      if (e?.status === 404) {
        goHome();
        showToast('会话已删除', 'error');
        return false;
      }
      setMessages([{ role: 'empty', content: e.message }]);
      showToast(e.message, 'error');
    }
  }, [api, applySessionModel, busy, clearAttachments, closeSidebarOnMobile, detachActiveStream, goHome, resetMessageAutoFollow, setProjectFilter, showToast, upsertSession]);

  const newSession = useCallback(async () => { await createSession(); }, [createSession]);

  const renameCurrent = useCallback(async () => {
    if (!current) return;
    const values = await showDialog({ title: '重命名会话', confirmText: '保存标题', fields: [{ name: 'title', label: '新的会话标题', value: currentTitle || '', required: true }] });
    if (!values || !values.title.trim()) return;
    const s = await renameSession(api, current, values.title.trim());
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    upsertSession(s);
    showToast('会话标题已保存', 'success');
  }, [api, current, currentTitle, showDialog, showToast, upsertSession]);

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
    upsertSession(s);
    showToast('会话标题已保存', 'success');
  }, [api, busy, current, showDialog, showToast, upsertSession]);

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
    removeSession(id);
    await Promise.all([loadSessions(), loadProjects()]);
    showToast('会话已删除', 'success');
  }, [api, busy, current, loadProjects, loadSessions, removeSession, showDialog, showToast]);

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
      upsertSession(s);
      await loadProjects();
      if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({ chatdock: true }, '', sessionPath(s.id));
      showToast('会话已复制', 'success');
    } catch (e) {
      showToast('复制会话失败：' + e.message, 'error');
    }
  }, [api, applySessionModel, busy, current, loadProjects, showToast, upsertSession]);


  const branchCurrent = useCallback(async (messageIndex = messages.length - 1) => {
    if (!current || busy) return;
    try {
      const s = await branchSession(api, current, messageIndex);
      setCurrent(s.id);
      setCurrentTitle(s.title || '分支对话');
      setMessages(s.messages || []);
      clearAttachments();
      applySessionModel(s);
      upsertSession(s);
      await loadProjects();
      if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({ chatdock: true }, '', sessionPath(s.id));
      closeSidebarOnMobile();
      showToast('已在新聊天中创建分支对话', 'success');
    } catch (e) {
      showToast('创建分支对话失败：' + e.message, 'error');
    }
  }, [api, applySessionModel, busy, closeSidebarOnMobile, current, loadProjects, messages.length, showToast, upsertSession]);

  const pinCurrent = useCallback(async () => {
    if (!current) return;
    const currentSummary = pinnedSessions.find(s => s.id === current) || sessions.find(s => s.id === current);
    const nextPinned = !currentSummary?.pinned;
    const s = await pinSession(api, current, nextPinned);
    setCurrentTitle(s.title || currentTitle || '新会话');
    setMessages(s.messages || []);
    upsertSession(s);
    showToast(nextPinned ? '会话已置顶' : '已取消置顶', 'success');
  }, [api, current, currentTitle, pinnedSessions, sessions, showToast, upsertSession]);

  const pinSessionByID = useCallback(async (id, pinned = false) => {
    if (!id || busy) return;
    setSessionMenuID('');
    const nextPinned = !pinned;
    const s = await pinSession(api, id, nextPinned);
    if (id === current) {
      setCurrentTitle(s.title || currentTitle || '新会话');
      setMessages(s.messages || []);
    }
    upsertSession(s);
    showToast(nextPinned ? '会话已置顶' : '已取消置顶', 'success');
  }, [api, busy, current, currentTitle, showToast, upsertSession]);

  const finishActiveAssistant = useCallback((finalSession) => {
    const finalAssistant = finalAssistantMessageFromSession(finalSession);
    setMessages(prev => prev.map((m, index) => {
      if (index !== prev.length - 1 || m.role !== 'assistant-stream') return m;
      const content = finalAssistant?.content || m.answer || '';
      return {
        ...m,
        role: 'assistant',
        content,
        answer: content,
        reasoning: finalAssistant?.reasoning || m.reasoning || '',
        parts: finalAssistant?.parts || m.parts,
        events: finalAssistant?.events || m.events,
        error: finalAssistant?.error || m.error,
        created_at: finalAssistant?.created_at || m.created_at,
      };
    }));
  }, []);

  const handleChatStreamEvent = useCallback((event, data, setFinalSession) => {
    if (event === 'job_started') {
      activeJobIDRef.current = data.id || '';
      setActiveJobID(data.id || '');
      return;
    }

    setStreamStats(prev => chatStreamStatsAfterEvent(prev, event, data, pausedRef.current));
    if (event === 'delta') {
      const reasoning = data.reasoning_content || '';
      const content = data.content || '';
      if (pausedRef.current) {
        pendingReasoningRef.current += reasoning;
        pendingDeltaRef.current += content;
      } else {
        enqueueStreamText({ reasoning, content });
      }
      return;
    }

    if (projectsChatStreamAssistant(event, data)) {
      // 工具、确认和错误事件必须排在此前文本之后，避免平滑显示改变真实时间线。
      flushStreamText();
      appendToActiveAssistant(message => chatStreamAssistantAfterEvent(message, event, data));
    }
    if (event === 'message_end' || event === 'done') {
      activeJobIDRef.current = '';
      setActiveJobID('');
      if (data.session) setFinalSession(data.session);
    }
  }, [appendToActiveAssistant, enqueueStreamText, flushStreamText]);

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
        resetStreamText();
        activeJobIDRef.current = job.id || '';
        activeJobSessionRef.current = current;
        setActiveJobID(job.id || '');
        setStreamPaused(false);
        pausedRef.current = false;
        pendingDeltaRef.current = '';
        pendingReasoningRef.current = '';
        abortRef.current = abort;
        setStreamStats({ state: 'streaming', started_at: Date.now(), chars: 0, events: 0, tools: 0, error: '' });
        setMessages(prev => messagesForRunningJobReplay(prev, job));
        let finalSession = null;
        await streamChatJobEvents({ jobID: job.id, authHeaders, signal: abort.signal, onEvent: (event, data) => handleChatStreamEvent(event, data, s => { finalSession = s; }) });
        if (finalSession && !stopped) {
          await waitForStreamText();
          if (stopped) return;
          pendingDeltaRef.current = '';
          pendingReasoningRef.current = '';
          finishActiveAssistant(finalSession);
          setCurrentTitle(finalSession.title || '新会话');
          upsertSession(finalSession);
        }
      } catch (e) {
        if (!abort.signal.aborted && !stopped) {
          flushStreamText();
          const message = readableChatError(e);
          setStreamStats(prev => ({ ...prev, state: 'error', error: message }));
          appendToActiveAssistant(m => ({ ...m, error: chatErrorDetails(e, message), answer: m.answer || '' }));
        }
      } finally {
        if (!stopped) {
          setBusy(false);
          abortRef.current = null;
          activeJobIDRef.current = '';
          activeJobSessionRef.current = '';
          setActiveJobID('');
          setStreamPaused(false);
        }
      }
    }
    resumeRunningJob();
    return () => { stopped = true; resetStreamText(); abort.abort(); };
  }, [current, api, authHeaders, handleChatStreamEvent, appendToActiveAssistant, finishActiveAssistant, flushStreamText, resetStreamText, upsertSession, waitForStreamText]);

  const selectedProject = useMemo(() => projects.find(project => project.id === projectFilter) || null, [projectFilter, projects]);
  const draftKey = useMemo(() => 'chatdock.draft.project-filter.' + encodeURIComponent(projectFilter || 'plain') + '.' + encodeURIComponent(current || 'new'), [projectFilter, current]);


  const providerChoices = useMemo(() => providers.map(provider => {
    const id = providerChoiceID(provider);
    const models = uniqueModelNames([...(provider.models || []), provider.default_model].filter(Boolean));
    return { ...provider, choice_id: id, models };
  }).filter(provider => provider.choice_id && (provider.enabled || provider.models.length)), [providers]);
  const selectedModelProvider = useMemo(() => {
    const activeID = config.provider_id || '';
    return providerChoices.find(provider => provider.choice_id === chatModel.provider_id)
      || providerChoices.find(provider => provider.choice_id === activeID)
      || providerChoices[0]
      || null;
  }, [chatModel.provider_id, config.provider_id, providerChoices]);
  const selectedChatModel = chatModel.model || globalDefaultModelChoice(config, selectedModelProvider).model;
  const selectedModelBaseURL = selectedModelProvider?.base_url || config.base_url || '';

  useEffect(() => {
    if (!providerChoices.length) return;
    const stillValid = providerChoices.some(provider => provider.choice_id === chatModel.provider_id && (!chatModel.model || provider.models.includes(chatModel.model)));
    if (stillValid) return;
    const provider = providerChoices.find(item => item.choice_id === (config.provider_id || '')) || providerChoices[0];
    setChatModel(globalDefaultModelChoice(config, provider));
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
          upsertSession(session);
          return session;
        })
        .catch(error => showToast('会话模型保存失败：' + error.message, 'error'));
    }
  }, [api, applySessionModel, current, showToast, upsertSession]);

  useEffect(() => {
    setInput(localStorage.getItem(draftKey) || '');
  }, [draftKey]);

  useEffect(() => {
    if (busy) return;
    const text = input.trim();
    if (text) localStorage.setItem(draftKey, input);
    else localStorage.removeItem(draftKey);
  }, [busy, draftKey, input]);

  const runChatCompletion = useCallback(async ({
    sessionID,
    message = '',
    attachmentIDs = [],
    regenerate = false,
    fallbackTitle = '新会话',
    hasImageAttachments = false,
  }) => {
    setBusy(true);
    resetStreamText();
    setStreamPaused(false);
    setStreamStats({ state: 'connecting', started_at: Date.now(), chars: 0, events: 0, tools: 0, error: '' });
    pausedRef.current = false;
    pendingDeltaRef.current = '';
    pendingReasoningRef.current = '';
    const abort = new AbortController();
    abortRef.current = abort;
    activeJobSessionRef.current = sessionID;
    resetMessageAutoFollow();
    try {
      setStreamStats(prev => ({ ...prev, state: 'streaming' }));
      let finalSession = null;
      await streamChat({
        authHeaders,
        signal: abort.signal,
        sessionID,
        message,
        attachmentIDs,
        providerID: selectedModelProvider?.choice_id || '',
        model: selectedChatModel,
        regenerate,
        onEvent: (event, data) => {
          if (currentRef.current === sessionID) handleChatStreamEvent(event, data, session => { finalSession = session; });
        },
      });
      if (finalSession) {
        await waitForStreamText();
        pendingDeltaRef.current = '';
        pendingReasoningRef.current = '';
        if (currentRef.current === sessionID) {
          finishActiveAssistant(finalSession);
          setCurrentTitle(finalSession.title || fallbackTitle);
        }
        upsertSession(finalSession);
      }
    } catch (error) {
      if (abort.signal.aborted) {
        if (!detachedControllersRef.current.has(abort)) {
          flushStreamText();
          appendReasoning(pendingReasoningRef.current);
          appendAnswer(pendingDeltaRef.current);
          appendAnswer('\n\n【已中断】');
        }
        await loadSessions().catch(() => { });
      } else {
        flushStreamText();
        const message = readableChatError(error, hasImageAttachments);
        setStreamStats(prev => ({ ...prev, state: 'error', error: message }));
        appendToActiveAssistant(currentMessage => ({ ...currentMessage, error: chatErrorDetails(error, message), answer: currentMessage.answer || '' }));
      }
    } finally {
      detachedControllersRef.current.delete(abort);
      if (abortRef.current === abort) {
        setBusy(false);
        abortRef.current = null;
        activeJobIDRef.current = '';
        activeJobSessionRef.current = '';
        setActiveJobID('');
        setStreamPaused(false);
      }
    }
  }, [appendAnswer, appendReasoning, appendToActiveAssistant, authHeaders, finishActiveAssistant, flushStreamText, handleChatStreamEvent, loadSessions, resetMessageAutoFollow, resetStreamText, selectedChatModel, selectedModelProvider, upsertSession, waitForStreamText]);

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
      const session = await createPersistedSession({ refreshList: false });
      if (!session) return;
      sessionID = session.id;
    }
    localStorage.removeItem(draftKey);
    setInput('');
    const sentAt = new Date().toISOString();
    setMessages(prev => [...prev,
      { role: 'user', content: text, attachments: attachmentsForMessage, created_at: sentAt },
      { role: 'assistant-stream', answer: '', reasoning: '', events: [], created_at: sentAt },
    ]);
    clearAttachments();
    await runChatCompletion({
      sessionID,
      message: text,
      attachmentIDs,
      fallbackTitle: currentTitle || '新会话',
      hasImageAttachments: attachmentsForMessage.some(attachmentLooksLikeImage),
    });
  }, [busy, clearAttachments, createPersistedSession, current, currentTitle, draftKey, input, openSettings, pendingAttachmentIDs, pendingAttachments, readyAttachments, runChatCompletion, selectedChatModel, selectedModelBaseURL, showToast, uploadingFiles]);


  const regenerateEditedReply = useCallback(async (sessionID, baseMessages, title) => {
    setMessages([...(baseMessages || []), { role: 'assistant-stream', answer: '', reasoning: '', events: [], created_at: new Date().toISOString() }]);
    await runChatCompletion({ sessionID, regenerate: true, fallbackTitle: title || '新会话' });
  }, [runChatCompletion]);

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
      upsertSession(next);
      showToast('已替换该消息，正在重新生成回复', 'success');
      void regenerateEditedReply(current, nextMessages, next.title || currentTitle || '新会话');
    } catch (error) {
      const message = error.message === 'busy' ? '正在生成' : error.message;
      showToast('编辑失败：' + message, 'error');
      throw error;
    }
  }, [api, busy, current, currentTitle, openSettings, regenerateEditedReply, selectedChatModel, selectedModelBaseURL, showToast, upsertSession]);

  const guideActiveJob = useCallback(async () => {
    if (!busy) return;
    const text = input.trim();
    if (!text) {
      showToast('先输入要追加的引导内容。', 'error');
      return;
    }
    const jobID = activeJobIDRef.current || activeJobID;
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
  }, [activeJobID, api, busy, input, showToast]);

  const stopStreaming = useCallback(async () => {
    if (!busy) return;
    setStreamStats(prev => ({ ...prev, state: 'stopping' }));
    const jobID = activeJobIDRef.current || activeJobID;
    if (jobID) {
      try { await cancelChatJob(api, jobID); } catch (e) { showToast('停止生成失败：' + e.message, 'error'); }
    }
    if (abortRef.current) abortRef.current.abort();
  }, [activeJobID, api, busy, showToast]);

  const resolveToolConfirmation = useCallback(async (id, approve) => {
    try {
      await resolveMCPConfirmation(api, id, approve);
      appendToActiveAssistant(m => ({ ...m, events: (m.events || []).map(item => item.confirmation?.id === id ? { ...item, status: 'resolved', text: (approve ? '已允许工具：' : '已拒绝工具：') + (item.confirmation?.tool || 'MCP 工具') } : item) }));
      showToast(approve ? '已允许工具执行' : '已拒绝工具执行', approve ? 'success' : 'info');
    } catch (e) {
      showToast('确认工具失败：' + e.message, 'error');
    }
  }, [api, appendToActiveAssistant, showToast]);

  const {
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
  } = useSettingsActions({
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
  });

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
  }, [api, current, showDialog, showToast]);

  const showContextPreview = useCallback(async () => {
    if (!current) return;
    try {
      const preview = await fetchContextPreview(api, current);
      await showDialog({ title: '上下文 / Token 预览', confirmText: '关闭', hideCancel: true, fields: [{ name: 'preview', label: '实际会发送给模型的上下文（token 为粗略估算）', type: 'textarea', rows: 16, value: contextPreviewText(preview) }] });
    } catch (e) {
      showToast('上下文预览失败：' + e.message, 'error');
    }
  }, [api, current, showDialog, showToast]);

  const showSystemPrompt = useCallback(async () => {
    if (!current) return;
    try {
      const data = await fetchSessionSystemPrompt(api, current);
      const prompt = String(data?.system_prompt || '').trim();
      await showDialog({ title: '当前会话系统提示词', confirmText: '关闭', hideCancel: true, fields: [{ name: 'prompt', label: prompt ? '全局提示词 + 项目提示词合并后的完整 System Prompt' : '当前未配置系统提示词', type: 'textarea', rows: 16, value: prompt || '(空)' }] });
    } catch (e) {
      showToast('系统提示词加载失败：' + e.message, 'error');
    }
  }, [api, current, showDialog, showToast]);

  const logout = useCallback(() => logoutAndReload(), []);

  const filteredSessions = useMemo(() => visibleSessionRows({
    sessionSearch, sessionSearchResults, sessions,
  }), [sessionSearch, sessionSearchResults, sessions]);
  const searchingSessions = !!sessionSearch.trim();
  const visibleSessionsHasMore = searchingSessions ? sessionSearchHasMore : sessionsHasMore;
  const visibleSessionsLoadingMore = searchingSessions ? sessionSearchLoadingMore : sessionsLoadingMore;
  const loadMoreVisibleSessions = searchingSessions ? loadMoreSearchSessions : loadMoreSessions;

  const currentSummary = useMemo(
    () => pinnedSessions.find(s => s.id === current) || sessions.find(s => s.id === current) || null,
    [current, pinnedSessions, sessions],
  );
  const currentPinned = !!currentSummary?.pinned;
  const appClass = 'app ' + (sidebarCollapsed ? 'sidebar-collapsed ' : '') + (!messages.length ? 'chat-empty ' : '') + (taskPanelOpen ? 'task-panel-open' : '');
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
    renameCurrent, sendMsg, showContextPreview, showSystemPrompt, setThemeState, theme,
  }), [branchCurrent, busy, cloneCurrent, copyCurrentMarkdown, copyText, createSession, current, currentPinned, deleteCurrent, exportCurrent, messages.length, openSettings, pinCurrent, renameCurrent, sendMsg, showContextPreview, showSystemPrompt, theme]);

  const settingsPanel = (
    <SettingsPanel
      activeModule={activeModule} api={api} closeSettings={closeSettings} config={config}
      configDirty={configDirty} mcpConfigDirty={mcpConfigDirty} dataStatus={dataStatus}
      editModelProvider={editModelProvider} deleteModelProvider={deleteModelProvider}
      testSavedModelProvider={testSavedModelProvider} fetchSavedProviderModels={fetchSavedProviderModels}
      loadDataStatus={loadDataStatus} loadMCPConfig={loadMCPConfig} loadMCPStatus={loadMCPStatus} loadSystemStatus={loadSystemStatus}
      builtinTools={builtinTools} mcpConfig={mcpConfig} mcpStatus={mcpStatus} onCopy={copyText} providers={providers}
      projectPromptPreview={projectPromptPreview} refreshProductState={refreshProductState} refreshVisibleSettings={refreshVisibleSettings}
      saveConfig={saveConfig} saveMCPConfig={saveMCPConfig} setConfig={setConfig} setMcpConfig={setMcpConfig}
      setupStatus={setupStatus} showProjectPromptPreview={showProjectPromptPreview} switchSettingsModule={switchSettingsModule}
      systemStatus={systemStatus} testMCP={testMCP} fetchMCPServerTools={fetchMCPServerTools}
      testModelProvider={testModelProvider} fetchProviderModels={fetchProviderModels} availableModels={availableModels}
      candidateProviderID={candidateProviderID} addCandidateModelToProvider={addCandidateModelToProvider} loadingModels={loadingModels}
      logout={logout}
      projects={projects} projectSessionCounts={projectSessionCounts} editProject={editProject} deleteProject={deleteProject}
      openProjectSessions={openProjectSessions} loadProjects={loadProjects} startProjectConversation={startProjectConversation} showToast={showToast}
      onPinnedProjectChange={upsertPinnedProject} onPinnedTaskChange={upsertPinnedTask}
      scheduledTasks={scheduledTasks} taskSearch={taskSearch} setTaskSearch={setTaskSearch}
      editScheduledTask={editScheduledTask} deleteScheduledTask={deleteScheduledTask} setScheduledTasks={setScheduledTasks}
      toggleScheduledTask={toggleScheduledTask} runScheduledTaskNow={runScheduledTaskNow} openScheduledTaskSession={openScheduledTaskSession}
      loadScheduledTasks={loadScheduledTasks}
    />
  );

  return <>
    <div id="sidebarMask" className={'sidebar-mask ' + (!settingsOpen && !sidebarCollapsed ? 'show' : '')} onClick={() => setSidebarCollapsed(true)} />
    {settingsOpen ? <div id="settingsPage" className="settings-page"><Suspense fallback={<PageLoadingState fullscreen title="正在打开配置中心" detail="正在准备模型、工具与系统设置。" />}>{settingsPanel}</Suspense></div> : <div id="app" className={appClass}>
      <Sidebar
        api={api} busy={busy} current={current} deleteSessionByID={deleteSessionByID} filteredSessions={filteredSessions} goHome={goHome}
        hasMoreSessions={visibleSessionsHasMore} loadingMoreSessions={visibleSessionsLoadingMore} newSession={newSession}
        onLoadMoreSessions={loadMoreVisibleSessions} openSession={openSession} openManagementPage={openManagementPage}
        pinSessionByID={pinSessionByID} pinnedSessions={pinnedSessions} pinnedProjects={pinnedProjects} pinnedTasks={pinnedTasks} projects={projects} projectFilter={projectFilter} renameSessionByID={renameSessionByID}
        startProjectConversation={startProjectConversation}
        scheduledTasks={scheduledTasks} setTaskSearch={setTaskSearch}
        sessionMenuID={sessionMenuID} sessionSearch={sessionSearch} sessionSearchBusy={sessionSearchBusy}
        setSessionMenuID={setSessionMenuID} setSessionSearch={setSessionSearch}
        setSidebarCollapsed={setSidebarCollapsed} sidebarCollapsed={sidebarCollapsed}
      />
      <main>
        <Topbar
          currentTitle={currentTitle} newSession={newSession} openSettings={openSettings} selectedProject={selectedProject}
          setQuickPaletteOpen={setQuickPaletteOpen} setSidebarCollapsed={setSidebarCollapsed} setThemeState={setThemeState}
          sidebarCollapsed={sidebarCollapsed} taskPanelAvailable={taskDataEnabled} taskPanelOpen={taskPanelOpen}
          taskPanelTasks={agentTasks.tasks} theme={theme} toggleTaskPanel={toggleTaskPanel}
        />
        <div className="messages" ref={messagesRef} onScroll={handleMessagesScroll} onWheel={handleMessagesWheel} onTouchStart={handleMessagesTouchStart} onTouchMove={handleMessagesTouchMove} onTouchEnd={handleMessagesTouchEnd} onTouchCancel={handleMessagesTouchEnd}>{messages.length ? messages.map((m, i) => <MemoizedMessageView key={i} message={m} previousMessage={messages[i - 1]} messageIndex={i} onCopy={copyText} onBranch={!busy && current ? branchCurrent : null} onEditUserMessage={editUserMessage} onDownloadAttachment={downloadAttachment} hideThinking={!!config.hide_thinking} onResolveConfirmation={resolveToolConfirmation} onInspectToolEvent={inspectToolEvent} />) : <EmptyState />}</div>
        {showJumpToLatest ? <button type="button" className="jump-latest" onClick={scrollToLatestModelMessage} aria-label="跳到最新模型消息" title="跳到最新模型消息"><ArrowDown className="jump-latest-icon" size={17} aria-hidden="true" /></button> : null}
        <CurrentSessionTask
          error={currentSessionTask.error} loading={currentSessionTask.loading} onRefresh={currentSessionTask.refresh}
          task={currentSessionTask.task} taskID={currentSessionTask.taskID}
        />
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
      {taskPanelOpen ? <>
        <div className="agent-task-panel-mask" onClick={closeTaskPanel} />
        <TaskPanel
          available={taskDataEnabled} deletingTaskID={deletingAgentTaskID} detailError={agentTasks.detailError} detailLoading={agentTasks.detailLoading} error={agentTasks.error}
          expandedTaskID={agentTasks.expandedTaskID} lastUpdatedAt={agentTasks.lastUpdatedAt} loading={agentTasks.loading}
          onClose={closeTaskPanel} onDelete={deleteAgentTaskFromPanel} onExpand={agentTasks.setExpandedTaskID} onRefresh={agentTasks.refresh}
          taskDetail={agentTasks.taskDetail} tasks={agentTasks.tasks}
        />
      </> : null}
    </div>}
    <QuickPalette open={quickPaletteOpen} actions={quickActions} onClose={() => setQuickPaletteOpen(false)} />
    {authPage ? <LoginPage api={api} error={authPage} refreshAfterLogin={refreshAfterLogin} setAuthPage={setAuthPage} /> : null}
    <DialogHost dialog={dialog} closeDialog={closeDialog} />
    {toast ? <div id="appToast" className={'app-toast show ' + toast.variant}>{toast.message}</div> : null}
  </>;
}
