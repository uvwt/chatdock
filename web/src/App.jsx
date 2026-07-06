import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AttachmentList, EmptyState, MessageView } from './components/chat.jsx';
import { DialogHost, LoginPage, Markdown, QuickPalette, WorkspacePicker } from './components/base.jsx';
import { SettingsPanel } from './components/settings.jsx';
import { defaultRunAtValue, diagnosticsText, filenameFromResponse, fmtDuration, fmtTime, normalizeSettingsModule, runStatusLabel, sessionIDFromPath, sessionPath, settingsModuleFromPath } from './lib/appUtils.js';
import { createJsonApi } from './lib/http.js';
import { cancelChatJob, fetchChatJobs, guideChatJob, resolveMCPConfirmation, streamChat, streamChatJobEvents } from './lib/chatApi.js';
import { branchSession, cloneSession, createSessionRecord, deleteSession, editSessionMessage, fetchContextPreview, fetchSession, fetchSessionMarkdown, fetchSessionToolEvent, fetchSessions, pinSession, renameSession, searchSessions, updateSessionModel } from './lib/sessionApi.js';
import { createModelProvider as createModelProviderRequest, createWorkspaceRecord, deleteModelProvider as deleteModelProviderRequest, deleteScheduledTaskRecord, deleteWorkspaceRecord, fetchConfig, fetchDataStatus, fetchMCPConfig, fetchMCPStatus, fetchModelProviders, fetchPrompts, fetchProviderModels as fetchProviderModelsRequest, fetchPromptPreview, fetchScheduledTaskRuns, fetchScheduledTasks, fetchSetupStatus, fetchSystemStatus, fetchWorkspaces, initializeSetup, runScheduledTask, saveMCPConfigRequest, saveScheduledTaskRecord, saveWorkspaceConfig, selectWorkspace as selectWorkspaceRequest, testMCPServer, testModelProvider as testModelProviderRequest, updateModelProvider as updateModelProviderRequest } from './lib/settingsApi.js';
import { uploadFileRequest } from './lib/upload.js';

function streamStatusText(stats, elapsed) {
  const labels = { connecting: '连接模型中', streaming: '流式输出中', paused: '已暂停，后台继续接收', stopping: '正在中断', done: '已完成', error: '输出失败' };
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



function providerKeyRows(provider = {}) {
  const keys = Array.isArray(provider.api_keys) ? provider.api_keys : (Array.isArray(provider.apiKeys) ? provider.apiKeys : []);
  if (!keys.length) return [{id: 'main', name: '主 key', api_key: '', enabled: true, priority: 1}];
  return keys.map((key, index) => ({
    id: String(key.id || ('key-' + (index + 1))).trim(),
    name: String(key.name || key.id || ('Key ' + (index + 1))).trim(),
    api_key: key.api_key || key.apiKey || key.api_key_masked || key.apiKeyMasked || (key.has_api_key ? '********' : ''),
    enabled: key.enabled === false ? false : true,
    priority: Number(key.priority || index + 1) || index + 1,
    saved: !!(key.api_key || key.apiKey || key.api_key_masked || key.apiKeyMasked || key.has_api_key),
  }));
}

function providerKeyInputsFromRows(rows, fallbackSecret = '') {
  const values = Array.isArray(rows) ? rows : [];
  const used = new Set();
  const clean = [];
  values.forEach((item, index) => {
    const secret = String(item?.api_key || item?.apiKey || '').trim();
    const saved = item?.saved === true || secret.includes('*');
    if (!secret && !saved && !fallbackSecret) return;
    let id = String(item?.id || '').trim();
    if (!id) id = index === 0 ? 'main' : 'key-' + (index + 1);
    while (used.has(id)) id = id + '-' + (index + 1);
    used.add(id);
    const name = String(item?.name || (index === 0 ? '主 key' : '备用 key ' + index)).trim();
    clean.push({ id, name, api_key: secret || fallbackSecret || '********', enabled: item?.enabled === false ? false : true, priority: clean.length + 1 });
  });
  return clean.length ? clean : null;
}

function providerChoiceID(provider) {
  return provider?.id || '';
}

function providerLabel(provider) {
  return provider?.name || provider?.id || '供应商';
}

function compactModelName(name) {
  name = String(name || '').trim();
  return name.length > 22 ? name.slice(0, 21) + '…' : name;
}

function sessionModelChoice(session) {
  return {
    provider_id: String(session?.provider_id || '').trim(),
    model: String(session?.model || '').trim(),
  };
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
        </div>) : <div className="empty compact">还没有可用模型，请先到配置中心手动添加。</div>}
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
    } catch { }
  }
  return appendRequestID(raw.replace(/^model api failed:\s*/i, ''), error?.request_id);
}

function appendRequestID(message, requestID) {
  requestID = String(requestID || '').trim();
  return requestID ? message + '\n请求 ID：' + requestID : message;
}

function streamErrorMessage(data = {}) {
  const message = String(data.message || '模型响应中断。').trim();
  return appendRequestID(message, data.request_id);
}

function safeJSONStringify(value) {
  try { return JSON.stringify(value, null, 2); }
  catch { return String(value || ''); }
}


function targetToolName(data) {
  if (data?.arguments?._parse_error) return '参数 JSON 解析失败';
  return data?.arguments?.name || data?.result?.tool || data?.tool || '工具';
}

function toolEventText(phase, data = {}) {
  const tool = data.tool || '';
  if (tool === 'chatdock_tools_search') return phase === 'start' ? '正在查找可用工具' : (data.ok === false ? '查找可用工具失败' : '已查找可用工具');
  if (tool === 'chatdock_tools_describe') return phase === 'start' ? '正在查看工具详情' : (data.ok === false ? '查看工具详情失败' : '已查看工具详情');
  if (tool === 'chatdock_tool_execute') {
    const target = targetToolName(data);
    if (phase === 'start') return '正在调用工具：' + target;
    return data.ok === false ? '调用工具失败：' + target : '已调用工具：' + target;
  }
  if (phase === 'start') return '正在调用：' + (tool || '工具');
  return (data.ok ? '调用完成：' : '调用失败：') + (tool || '工具');
}

function toolCallKey(data = {}) {
  const args = data.arguments || {};
  return [data.tool || '', safeJSONStringify(args)].join('::');
}

function toolEventMeta(data = {}) {
  const query = data.arguments?.query || data.result?.query;
  if (data.tool === 'chatdock_tools_search' && query) return '关键词：' + query;
  const target = data.arguments?.name || data.result?.tool;
  if ((data.tool === 'chatdock_tools_describe' || data.tool === 'chatdock_tool_execute') && target) return target;
  return '';
}

function appendInlineTextPart(message, text) {
  const parts = [...(message.parts || [])];
  const last = parts[parts.length - 1];
  if (last?.kind === 'text') parts[parts.length - 1] = {...last, text: (last.text || '') + text};
  else parts.push({kind: 'text', text});
  return {...message, answer: (message.answer || '') + text, parts};
}

function appendInlineReasoningPart(message, text) {
  const parts = [...(message.parts || [])];
  const last = parts[parts.length - 1];
  if (last?.kind === 'reasoning') parts[parts.length - 1] = {...last, text: (last.text || '') + text};
  else parts.push({kind: 'reasoning', text});
  return {...message, reasoning: (message.reasoning || '') + text, parts};
}

function eventHasDisplayName(item = {}) {
  return !!(item.details?.arguments?.name || item.details?.data?.arguments?.name || item.details?.data?.result?.tool || item.details?.tool || item.details?.data?.tool);
}

function appendEventPart(message, item) {
  const events = [...(message.events || []), item];
  const parts = eventHasDisplayName(item) ? [...(message.parts || []), {kind: 'tool', callKey: item.callKey || '', event: item}] : (message.parts || []);
  return {...message, events, parts};
}

function appendToolStartEvent(message, event, data) {
  return appendEventPart(message, {
    kind: 'tool',
    phase: 'running',
    callKey: toolCallKey(data),
    text: toolEventText('start', data),
    meta: toolEventMeta(data),
    details: {event, tool: data.tool || '', arguments: data.arguments || {}, data},
  });
}

function mergeToolResultEvent(message, event, data) {
  const key = toolCallKey(data);
  const events = [...(message.events || [])];
  const parts = [...(message.parts || [])];
  const hasArguments = Object.keys(data.arguments || {}).length > 0;
  const sameRunningEvent = item => {
    if (item.kind !== 'tool' || item.phase !== 'running') return false;
    if (item.callKey === key) return true;
    return !hasArguments && item.details?.tool === data.tool;
  };
  const buildResultEvent = previousEvent => {
    const previousDetails = previousEvent?.details || {};
    const previousData = previousDetails.data && typeof previousDetails.data === 'object' ? previousDetails.data : {};
    const mergedArguments = hasArguments ? data.arguments : (previousDetails.arguments || previousData.arguments || data.arguments || {});
    const mergedData = { ...previousData, ...data };
    if (hasDialogValue(mergedArguments)) mergedData.arguments = mergedArguments;
    return {
      kind: 'tool',
      phase: data.ok ? 'done' : 'error',
      callKey: previousEvent?.callKey || key,
      text: toolEventText('result', data),
      meta: toolEventMeta(mergedData),
      details: {
        ...previousDetails,
        event,
        tool: data.tool || previousDetails.tool || '',
        ok: !!data.ok,
        arguments: mergedArguments,
        result: data.result,
        error: data.error || '',
        data: mergedData,
      },
    };
  };
  const index = events.findLastIndex(sameRunningEvent);
  const nextEvent = buildResultEvent(index >= 0 ? events[index] : null);
  if (index >= 0) events[index] = {...events[index], ...nextEvent};
  else events.push(nextEvent);
  const partIndex = parts.findLastIndex(part => part.kind === 'tool' && sameRunningEvent(part.event || {}));
  if (partIndex >= 0) {
    const mergedPartEvent = buildResultEvent(parts[partIndex].event || null);
    parts[partIndex] = {...parts[partIndex], event: {...parts[partIndex].event, ...mergedPartEvent}};
  } else if (eventHasDisplayName(nextEvent)) parts.push({kind: 'tool', callKey: nextEvent.callKey, event: nextEvent});
  return {...message, events, parts};
}

function finalAssistantMessageFromSession(session) {
  const messages = session?.messages || [];
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    if (messages[i]?.role === 'assistant') return messages[i];
  }
  return null;
}

function hasDialogValue(value) {
  if (value == null || value === '') return false;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'object') return Object.keys(value).length > 0;
  return true;
}

function arrayValue(value) {
  return Array.isArray(value) ? value : [];
}

function actualToolCall(details = {}, data = {}) {
  const proxyTool = details.tool || data.tool || '';
  const args = details.arguments ?? data.arguments ?? {};
  const result = details.result ?? data.result;
  if (proxyTool === 'chatdock_tool_execute') {
    const resultObject = result && typeof result === 'object' && !Array.isArray(result) ? result : {};
    const parseError = args?._parse_error || data?._parse_error || '';
    if (parseError) {
      return {
        proxyTool,
        actualTool: '工具参数解析失败',
        actualArguments: args,
        actualResult: resultObject.result ?? result,
        parseError,
        mode: 'execute_parse_error',
      };
    }
    return {
      proxyTool,
      actualTool: args.name || resultObject.tool || '',
      actualArguments: args.arguments ?? {},
      actualResult: resultObject.result ?? result,
      mode: 'execute',
    };
  }
  if (proxyTool === 'chatdock_tools_describe') {
    const names = arrayValue(args.names || result?.tools?.map?.(item => item?.name)).filter(Boolean);
    return {
      proxyTool,
      actualTool: names.length === 1 ? names[0] : (names.length ? names.length + ' 个工具说明' : ''),
      actualArguments: { names },
      actualResult: result,
      names,
      mode: 'describe',
    };
  }
  if (proxyTool === 'chatdock_tools_search') {
    const candidates = arrayValue(result?.tools).map(item => item?.name || item?.full_name || item).filter(Boolean);
    const count = Number(data.count ?? result?.count ?? candidates.length) || candidates.length;
    return {
      proxyTool,
      actualTool: count ? count + ' 个候选工具' : '工具搜索',
      actualArguments: args,
      actualResult: result,
      candidates,
      candidateCount: count,
      query: args.query || result?.query || data.query || '',
      mode: 'search',
    };
  }
  return {
    proxyTool: '',
    actualTool: proxyTool,
    actualArguments: args,
    actualResult: result,
    mode: 'direct',
  };
}

function buildToolEventDetail(event) {
  const details = event?.details || {};
  const data = details.data && typeof details.data === 'object' ? details.data : {};
  const eventName = details.event || data.event || 'tool_event';
  const tool = details.tool || data.tool || '';
  const actual = actualToolCall(details, data);
  const ok = typeof details.ok === 'boolean' ? details.ok : (typeof data.ok === 'boolean' ? data.ok : null);
  const failed = ok === false || /error|failed|cancelled/i.test(eventName) || details.error || data.error;
  const ready = /ready|resolved|finish|done/i.test(eventName) || ok === true;
  const status = failed ? '失败' : (ready ? '完成' : '事件');
  const duration = details.duration_ms || data.duration_ms;
  const heading = actual.mode === 'execute_parse_error'
    ? '工具参数 JSON 解析失败'
    : (actual.mode === 'search'
      ? (actual.candidateCount ? '找到 ' + actual.candidateCount + ' 个候选工具' : '工具搜索')
      : (actual.mode === 'describe'
        ? (actual.names?.length ? '查看 ' + actual.names.length + ' 个工具说明' : '工具说明')
        : (actual.actualTool || event?.text || tool || '工具事件')));
  const subheading = [
    actual.query ? '关键词：' + actual.query : '',
    actual.proxyTool && actual.proxyTool !== actual.actualTool ? '代理：' + actual.proxyTool : '',
  ].filter(Boolean).join(' · ');
  const metrics = [
    actual.mode === 'search' ? { label: '候选', value: actual.candidateCount || actual.candidates?.length || '' } : null,
    { label: '工具总数', value: data.tool_count ?? details.tool_count },
    { label: '内置工具', value: data.builtin_tool_count ?? details.builtin_tool_count },
    { label: '耗时', value: duration ? fmtDuration(duration) : '' },
  ].filter(item => item && hasDialogValue(item.value));
  const rows = [
    { label: '事件类型', value: eventName },
    { label: '状态', value: status },
    actual.proxyTool ? { label: '代理工具', value: actual.proxyTool } : null,
    actual.query ? { label: '搜索关键词', value: actual.query } : null,
    actual.parseError ? { label: '解析错误', value: actual.parseError } : null,
    { label: '服务', value: data.server || details.server },
    { label: '动作', value: data.action || details.action },
  ].filter(item => item && hasDialogValue(item.value));
  const sections = [];
  if (actual.mode === 'search' && actual.candidates?.length) {
    sections.push({ title: '候选工具', value: actual.candidates, display: 'tools', emptyText: '没有候选工具' });
  }
  if (hasDialogValue(details.error || data.error)) sections.push({ title: '错误', value: details.error || data.error, tone: 'danger' });
  const hasArgumentsSection = hasDialogValue(actual.actualArguments);
  const hasResultSection = hasDialogValue(actual.actualResult);
  if (hasArgumentsSection) sections.push({ title: actual.mode === 'execute_parse_error' ? '模型原始参数' : (actual.mode === 'execute' ? '参数' : '请求参数'), value: actual.actualArguments, emptyText: '无参数' });
  if (hasResultSection) sections.push({ title: actual.mode === 'execute' ? '响应' : '完整响应', value: actual.actualResult, emptyText: '无响应', collapsed: hasArgumentsSection });
  if (hasDialogValue(data) && !sections.length) sections.push({ title: '事件数据', value: data });
  sections.push({ title: '原始事件', value: details, collapsed: true, muted: true });
  return {
    event: eventName,
    heading,
    subheading,
    status,
    statusTone: failed ? 'danger' : (ready ? 'success' : ''),
    primary: actual.actualTool ? {
      label: actual.mode === 'execute_parse_error' ? '失败原因' : (actual.mode === 'execute' ? '实际调用' : (actual.mode === 'search' ? '搜索结果' : '说明对象')),
      name: actual.actualTool,
      hint: subheading || (actual.proxyTool ? '通过 ' + actual.proxyTool : ''),
    } : null,
    metrics,
    rows,
    sections,
  };
}

function scheduledTaskContextLabel(mode) {
  return ({stateless: '每次独立执行', last_result: '带上次运行结果', session: '连续会话'})[mode] || mode || '每次独立执行';
}

function scheduledTaskRunsText(task, runs = []) {
  const lines = [];
  lines.push('任务：' + (task?.title || '-'));
  lines.push('上下文模式：' + scheduledTaskContextLabel(task?.context_mode || 'stateless'));
  lines.push('');
  if (!runs.length) {
    lines.push('暂无运行记录。');
    return lines.join('\n');
  }
  runs.forEach((run, index) => {
    lines.push((index + 1) + '. ' + runStatusLabel(run.status) + ' · ' + fmtTime(run.started_at) + ' · ' + fmtDuration(run.duration_ms) + (run.manual ? ' · 手动' : ' · 自动'));
    if (run.session_id) lines.push('会话：' + run.session_id);
    if (run.error) lines.push('错误：' + run.error);
    if (run.output) {
      lines.push('输出：');
      lines.push(String(run.output).slice(0, 1800));
    }
    lines.push('');
  });
  return lines.join('\n');
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
  const [quickPaletteOpen, setQuickPaletteOpen] = useState(false);
  const [modelPickerOpen, setModelPickerOpen] = useState(false);
  const [chatModel, setChatModel] = useState({ provider_id: '', model: '' });
  const [showJumpToLatest, setShowJumpToLatest] = useState(false);

  const [setupStatus, setSetupStatus] = useState(null);
  const [workspaces, setWorkspaces] = useState([]);
  const [providers, setProviders] = useState([]);
  const [availableModels, setAvailableModels] = useState([]);
  const [candidateProviderID, setCandidateProviderID] = useState('');
  const [loadingModels, setLoadingModels] = useState(false);
  const [prompts, setPrompts] = useState([]);
  const [sessions, setSessions] = useState([]);
  const [sessionSearch, setSessionSearch] = useState('');
  const [sessionSearchResults, setSessionSearchResults] = useState([]);
  const [sessionSearchBusy, setSessionSearchBusy] = useState(false);
  const [sessionMenuID, setSessionMenuID] = useState('');
  const [current, setCurrent] = useState(null);
  const [currentTitle, setCurrentTitle] = useState('未选择会话');
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [pendingAttachments, setPendingAttachments] = useState([]);
  const [uploadingFiles, setUploadingFiles] = useState(false);
  const [busy, setBusy] = useState(false);
  const [streamPaused, setStreamPaused] = useState(false);
  const [streamStats, setStreamStats] = useState({ state: 'idle', started_at: 0, chars: 0, events: 0, tools: 0, error: '' });
  const [activeJobID, setActiveJobID] = useState('');
  const [scheduledTasks, setScheduledTasks] = useState([]);
  const [selectedScheduledTaskID, setSelectedScheduledTaskID] = useState('');
  const [selectedScheduledTaskRuns, setSelectedScheduledTaskRuns] = useState([]);
  const [taskSearch, setTaskSearch] = useState('');
  const [dataStatus, setDataStatus] = useState(null);
  const [systemStatus, setSystemStatus] = useState(null);
  const [mcpStatus, setMcpStatus] = useState([]);
  const [promptPreview, setPromptPreview] = useState('');
  const [mcpConfig, setMcpConfig] = useState('');
  const [config, setConfig] = useState({ base_url: '', api_key: '', model: '', models: [], system_prompt: '', context_mode: 'auto', max_context_messages: 12, temperature: 0.7, enable_thinking: false, hide_thinking: false, has_api_key: false, embedding_base_url: '', embedding_api_key: '', embedding_model: 'BAAI/bge-m3', has_embedding_api_key: false });

  const abortRef = useRef(null);
  const activeJobIDRef = useRef('');
  const activeJobSessionRef = useRef('');
  const detachedControllersRef = useRef(new WeakSet());
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
  const toolEventDetailCacheRef = useRef(new Map());

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
  }, []);

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

  const applySessionModel = useCallback((session, { fallbackToDefault = true } = {}) => {
    const next = sessionModelChoice(session);
    if (fallbackToDefault || next.provider_id || next.model) setChatModel(next);
  }, []);

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
      provider_id: c.provider_id || '',
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
      embedding_base_url: c.embedding_base_url || '',
      embedding_api_key: '',
      embedding_model: c.embedding_model || 'BAAI/bge-m3',
      has_embedding_api_key: !!c.has_embedding_api_key,
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
    setPendingAttachments([]);
    applySessionModel(s);
    await loadSessions();
    return true;
  }, [api, applySessionModel, loadSessions]);

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
    setPendingAttachments([]);
    setChatModel({ provider_id: '', model: '' });
    await Promise.allSettled([refreshProductState(), loadPrompts(), loadConfig(), loadMCPConfig(), loadScheduledTasks(), loadSessions()]);
    if (window.location.pathname !== '/') window.history.pushState({ chatdock: true }, '', '/');
    closeSidebarOnMobile();
  }, [api, busy, refreshProductState, loadPrompts, loadConfig, loadMCPConfig, loadScheduledTasks, loadSessions, closeSidebarOnMobile, showToast]);

  const createPersistedSession = useCallback(async ({ refreshList = true } = {}) => {
    const s = await createSessionRecord(api);
    setCurrent(s.id);
    setCurrentTitle(s.title || '新会话');
    setMessages(s.messages || []);
    setPendingAttachments([]);
    applySessionModel(s, { fallbackToDefault: false });
    if (refreshList) await loadSessions();
    if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({ chatdock: true }, '', sessionPath(s.id));
    return s;
  }, [api, applySessionModel, loadSessions]);

  const createSession = useCallback(() => {
    if (busy) detachActiveStream();
    // “新会话”只是进入一个本地草稿，不应该提前写入后端。
    // 真正的 session id 只有在发送首条消息、或上传附件需要绑定会话时才创建。
    setSessionMenuID('');
    setCurrent(null);
    setCurrentTitle('新会话');
    setMessages([]);
    setPendingAttachments([]);
    setChatModel({ provider_id: '', model: '' });
    if (window.location.pathname !== '/') window.history.pushState({ chatdock: true }, '', '/');
    closeSidebarOnMobile();
    window.setTimeout(() => inputRef.current?.focus(), 0);
    return { id: '', title: '新会话', messages: [], draft: true };
  }, [busy, closeSidebarOnMobile, detachActiveStream]);

  const openSession = useCallback(async (id, summary = null) => {
    if (!id) return;
    const seq = sessionOpenSeqRef.current + 1;
    sessionOpenSeqRef.current = seq;
    if (busy) detachActiveStream();
    setSessionMenuID('');
    setCurrent(id);
    setCurrentTitle(summary?.title || '正在加载会话…');
    setPendingAttachments([]);
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
  }, [api, applySessionModel, busy, closeSidebarOnMobile, detachActiveStream, loadSessions, messages.length, showToast]);

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
  }, [api, busy, current, loadSessions, showDialog, showToast]);

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
      setPendingAttachments([]);
      setChatModel({ provider_id: '', model: '' });
      if (window.location.pathname !== '/') window.history.pushState({ chatdock: true }, '', '/');
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
      setPendingAttachments([]);
      applySessionModel(s);
      await loadSessions();
      if (window.location.pathname !== sessionPath(s.id)) window.history.pushState({ chatdock: true }, '', sessionPath(s.id));
      closeSidebarOnMobile();
      showToast('已在新聊天中创建分支对话', 'success');
    } catch (e) {
      showToast('创建分支对话失败：' + e.message, 'error');
    }
  }, [api, applySessionModel, busy, closeSidebarOnMobile, current, loadSessions, messages.length, showToast]);

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
      setActiveJobID(data.id || '');
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
      setActiveJobID('');
      if (data.status === 'failed') {
        setStreamStats(prev => ({ ...prev, state: 'error' }));
      } else if (data.status === 'interrupted') {
        setStreamStats(prev => ({ ...prev, state: 'done' }));
      }
    } else if (event === 'done') {
      setFinalSession(data.session);
      activeJobIDRef.current = '';
      setActiveJobID('');
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
        setActiveJobID(job.id || '');
        setStreamPaused(false);
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
          setActiveJobID('');
          setStreamPaused(false);
        }
      }
    }
    resumeRunningJob();
    return () => { stopped = true; abort.abort(); };
  }, [current, api, authHeaders, handleChatStreamEvent, appendToActiveAssistant, currentTitle, loadSessions, finishActiveAssistant]);

  const activePrompt = useMemo(() => prompts.find(p => p.active) || prompts[0] || null, [prompts]);
  const draftKey = useMemo(() => 'chatdock.draft.' + encodeURIComponent(activePrompt?.name || 'default') + '.' + encodeURIComponent(current || 'new'), [activePrompt?.name, current]);


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

  const readyAttachments = useMemo(() => pendingAttachments.filter(item => item.id && !item.uploading && !item.error && !String(item.id).startsWith('local_')), [pendingAttachments]);
  const pendingAttachmentIDs = useMemo(() => readyAttachments.map(item => item.id), [readyAttachments]);

  const removePendingAttachment = useCallback((id) => {
    setPendingAttachments(prev => prev.filter(item => item.id !== id));
  }, []);

  const downloadAttachment = useCallback(async (item) => {
    if (!item?.id || String(item.id).startsWith('local_')) return;
    try {
      const response = await fetch('/api/files/' + encodeURIComponent(item.id), { headers: authHeaders() });
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
        setPendingAttachments(prev => [...prev, { id: localID, name: file.name || 'upload', size: file.size || 0, mime_type: file.type || 'application/octet-stream', status: 'uploading', uploading: true, progress: 0 }]);
        try {
          const data = await uploadFileRequest(file, sessionID, authHeaders, progress => {
            setPendingAttachments(prev => prev.map(item => item.id === localID ? { ...item, progress } : item));
          });
          setPendingAttachments(prev => prev.map(item => item.id === localID ? { ...data.attachment, progress: 100 } : item));
        } catch (e) {
          if (e.status === 401) setAuthPage(e);
          setPendingAttachments(prev => prev.map(item => item.id === localID ? { ...item, uploading: false, error: e.message || '上传失败', status: 'failed' } : item));
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
      const s = await createPersistedSession({ refreshList: false });
      if (!s) return;
      sessionID = s.id;
    }
    localStorage.removeItem(draftKey);
    setInput('');
    setBusy(true);
    setStreamPaused(false);
    setStreamStats({ state: 'connecting', started_at: Date.now(), chars: 0, events: 0, tools: 0, error: '' });
    pausedRef.current = false;
    pendingDeltaRef.current = '';
    pendingReasoningRef.current = '';
    const abort = new AbortController();
    abortRef.current = abort;
    activeJobSessionRef.current = sessionID;
    forceScrollRef.current = true;
    stickToBottomRef.current = true;
    setMessages(prev => [...prev, { role: 'user', content: text, attachments: attachmentsForMessage }, { role: 'assistant-stream', answer: '', reasoning: '', events: [] }]);
    try {
      setPendingAttachments([]);
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
        setActiveJobID('');
        setStreamPaused(false);
      }
    }
  }, [authHeaders, busy, selectedModelBaseURL, selectedChatModel, selectedModelProvider, current, currentTitle, draftKey, input, pendingAttachmentIDs, pendingAttachments, readyAttachments, uploadingFiles, createPersistedSession, loadSessions, appendAnswer, appendReasoning, appendToActiveAssistant, handleChatStreamEvent, finishActiveAssistant, openSettings, showToast]);


  const regenerateEditedReply = useCallback(async (sessionID, baseMessages, title) => {
    setBusy(true);
    setStreamPaused(false);
    setStreamStats({ state: 'connecting', started_at: Date.now(), chars: 0, events: 0, tools: 0, error: '' });
    pausedRef.current = false;
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
        setActiveJobID('');
        setStreamPaused(false);
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
    setPendingAttachments([]);
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
    setPendingAttachments([]);
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
      await loadScheduledTasks();
      await loadSessions();
      if (selectedScheduledTaskID === id) {
        const runsData = await fetchScheduledTaskRuns(api, id).catch(() => null);
        if (runsData) setSelectedScheduledTaskRuns(runsData.runs || []);
      }
      await refreshProductState();
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

  const selectedScheduledTaskSessions = useMemo(() => {
    if (!selectedScheduledTaskID) return [];
    const byID = new Map(sessions.map(item => [item.id, item]));
    const seen = new Set();
    return (selectedScheduledTaskRuns || []).filter(run => run.session_id && !seen.has(run.session_id)).map(run => {
      seen.add(run.session_id);
      const session = byID.get(run.session_id);
      const runTitle = session?.title || run.task_title || selectedScheduledTask?.title || '定时任务';
      if (session) {
        return {
          ...session,
          title: runTitle,
          preview: '',
          scheduled_run: run,
        };
      }
      return {
        id: run.session_id,
        title: runTitle + ' · ' + fmtTime(run.started_at),
        preview: '',
        last_role: run.status === 'failed' ? 'error' : 'assistant',
        count: 1,
        updated_at: run.finished_at || run.started_at,
        scheduled_run: run,
      };
    });
  }, [selectedScheduledTask?.title, selectedScheduledTaskID, selectedScheduledTaskRuns, sessions]);

  const filteredSessions = useMemo(() => {
    const q = sessionSearch.trim();
    if (q) return sessionSearchResults;
    if (selectedScheduledTaskID) return selectedScheduledTaskSessions;
    return sessions;
  }, [selectedScheduledTaskID, selectedScheduledTaskSessions, sessionSearch, sessionSearchResults, sessions]);

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

  const quickActions = useMemo(() => [
    { id: 'focus-input', title: '聚焦输入框', hint: '按 / 也可以快速输入', run: () => inputRef.current?.focus() },
    { id: 'new-session', title: '新建会话', hint: '在当前工作空间开始新对话', run: createSession },
    { id: 'continue', title: '发送“继续”', hint: '让当前会话继续上一轮内容', disabled: busy, run: () => sendMsg('继续') },
    { id: 'workspace-picker', title: '切换工作空间', hint: '加载不同模型和会话', disabled: busy || !prompts.length, run: () => setWorkspacePickerOpen(true) },
    { id: 'settings', title: '打开配置中心', hint: '工作空间、模型、工具和数据统一管理', run: () => openSettings() },
    { id: 'settings-model', title: '模型设置', hint: 'Base URL、API Key、模型和最终 Prompt', run: () => openSettings('model') },
    { id: 'settings-tools', title: '工具中心', hint: 'MCP 配置、状态检测和连接测试', run: () => openSettings('tools') },
    { id: 'settings-automation', title: '自动化任务', hint: '创建、运行和暂停定时任务', run: () => openSettings('automation') },
    { id: 'settings-data', title: '数据状态', hint: '数据库、工作空间和会话数量', run: () => openSettings('data') },
    { id: 'copy-diagnostics', title: '复制诊断信息', hint: '复制脱敏后的系统、数据库、备份和 MCP 状态', run: () => copyText(productDiagnostics) },
    { id: 'copy-session', title: '复制当前会话全文', hint: '复制为 Markdown', disabled: !current, run: copyCurrentMarkdown },
    { id: 'export-session', title: '导出当前会话', hint: '下载 Markdown 文件', disabled: !current, run: exportCurrent },
    { id: 'context-preview', title: '查看上下文 / Token 预览', hint: '查看实际发送给模型的消息构成', disabled: !current, run: showContextPreview },
    { id: 'delete-session', title: '删除当前会话', hint: '删除后不可恢复', disabled: !current || busy, run: deleteCurrent },
    { id: 'rename-session', title: '重命名当前会话', hint: '整理侧栏会话列表', disabled: !current || busy, run: renameCurrent },
    { id: 'clone-session', title: '复制当前会话', hint: '保留上下文开一个副本', disabled: !current || busy, run: cloneCurrent },
    { id: 'branch-session', title: '创建分支对话', hint: '在新聊天中从当前上下文继续', disabled: !current || busy || !messages.length, run: () => branchCurrent() },
    { id: 'pin-session', title: currentPinned ? '取消置顶当前会话' : '置顶当前会话', hint: '让重要会话固定在列表顶部', disabled: !current, run: pinCurrent },
    { id: 'theme', title: '切换明暗主题', hint: '当前：' + (theme === 'day' ? '白天' : '夜晚'), run: () => setThemeState(theme === 'day' ? 'night' : 'day') },
  ], [branchCurrent, busy, cloneCurrent, copyCurrentMarkdown, copyText, createSession, current, currentPinned, deleteCurrent, exportCurrent, messages.length, openSettings, pinCurrent, productDiagnostics, prompts.length, renameCurrent, sendMsg, showContextPreview, theme]);

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
    <div id="sidebarMask" className={'sidebar-mask ' + (!settingsOpen && !sidebarCollapsed ? 'show' : '')} onClick={() => setSidebarCollapsed(true)} />
    {settingsOpen ? <div id="settingsPage" className="settings-page">{settingsPanel}</div> : <div id="app" className={appClass}>
      <aside>
        <div className="sidebar-head">
          <div className="brand"><div className="brand-copy"><span className="brand-text">ChatDock</span><div className="sub">会话、工具、任务，一站协同</div></div></div>
          <button id="sidebarToggle" className="sidebar-toggle" onClick={() => setSidebarCollapsed(!sidebarCollapsed)} title={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}>{sidebarCollapsed ? '›' : '‹'}</button>
        </div>
        <div className="prompt-box">
          <button className="workspace-picker-trigger" type="button" disabled={busy || !prompts.length} onClick={() => setWorkspacePickerOpen(true)}>
            <span className="workspace-picker-name">{activePrompt ? (activePrompt.name === 'default' ? '默认工作区' : activePrompt.name) : '未选择'}</span>
            <span className="workspace-picker-meta">{activePrompt ? activePrompt.count : sessions.length}</span>
          </button>
        </div>
        <div className="session-search-row">
          <label className="session-search-box"><input className="session-search" placeholder="搜索聊天记录" value={sessionSearch} onChange={e => setSessionSearch(e.target.value)} /></label>
          <button className="new" onClick={newSession} aria-label="新会话" title="新会话"><span className="new-icon" aria-hidden="true">＋</span></button>
        </div>
        {activeScheduledTasks.length ? <div className="sidebar-tasks">
          <div className="sidebar-section-head compact"><div className="sidebar-section-title">定时任务</div><span className="sidebar-section-count">{activeScheduledTasks.length}</span></div>
          <div className="sidebar-task-list session-list-like">{activeScheduledTasks.slice(0, 3).map(task => <button key={task.id} type="button" className={'sidebar-task-item session ' + (selectedScheduledTaskID === task.id ? 'active ' : '') + (task.running ? 'running ' : '')} onClick={() => openScheduledTaskRunList(task.id)}>
            <div className="session-main"><div className="sidebar-task-name session-title">{task.title || '未命名任务'}</div></div>
          </button>)}</div>
          {activeScheduledTasks.length > 3 ? <button type="button" className="sidebar-task-more" onClick={() => openSettings('automation')}>查看全部 {activeScheduledTasks.length} 个任务</button> : null}
        </div> : null}
        <div className="sidebar-section-head"><div className="sidebar-section-title">{selectedScheduledTask ? '任务会话' : '最近会话'}</div>{selectedScheduledTask ? <button type="button" className="secondary small sidebar-clear-task" onClick={clearScheduledTaskRunList}>全部</button> : null}</div>
        {selectedScheduledTask ? <div className="session-search-meta">{selectedScheduledTask.title || '定时任务'} · {selectedScheduledTaskSessions.length} 次会话</div> : (sessionSearch.trim() ? <div className="session-search-meta">{sessionSearchBusy ? '搜索中…' : '全文搜索 ' + filteredSessions.length + ' 条'}</div> : null)}
        <div id="sessions">{filteredSessions.length ? filteredSessions.map(s => {
          const isActive = current === s.id;
          const menuOpen = sessionMenuID === s.id;
          return <div key={s.id} className={'session ' + (s.scheduled_run ? 'scheduled-run ' : '') + (isActive ? 'active ' : '') + (s.pinned ? 'pinned ' : '') + (menuOpen ? 'menu-open' : '')} onClick={() => openSession(s.id, s)}>
            <div className="session-main"><div className="session-title">{s.pinned ? <span className="pin-mark" aria-label="置顶" title="置顶" /> : null}{s.title}</div>{s.scheduled_run ? null : (s.match_snippet ? <div className="session-preview search-hit">{s.match_field ? s.match_field + '：' : ''}{s.match_snippet}</div> : (s.preview ? <div className="session-preview">{s.preview}</div> : null))}{s.scheduled_run ? null : <div className="session-meta">{s.count} 条 · {fmtTime(s.updated_at)}</div>}</div>
            <button type="button" className="session-menu-trigger" disabled={busy} onClick={e => { e.stopPropagation(); setSessionMenuID(menuOpen ? '' : s.id); }} aria-label={(s.title || '会话') + ' 操作'} aria-expanded={menuOpen ? 'true' : 'false'} title="会话操作">⋯</button>
            {menuOpen ? <div className="session-row-menu" onClick={e => e.stopPropagation()}>
              <button type="button" onClick={() => pinSessionByID(s.id, !!s.pinned)}>{s.pinned ? '取消置顶' : '置顶'}</button>
              <button type="button" className="danger" onClick={() => { setSessionMenuID(''); deleteSessionByID(s.id, s.title); }} disabled={busy}>删除</button>
              <button type="button" onClick={() => renameSessionByID(s.id, s.title)}>重命名标题</button>
            </div> : null}
          </div>;
        }) : <div className="empty compact">{selectedScheduledTask ? '这个任务还没有可打开的运行会话' : (sessionSearch.trim() ? '没有匹配会话' : '暂无会话，开始新会话')}</div>}</div>
      </aside>
      <main>
        <div className="topbar">
          <div className="top-left"><button className="mobile-menu" onClick={() => setSidebarCollapsed(!sidebarCollapsed)}>☰</button><b id="title">{currentTitle}</b></div>
          <div className="top-actions">
            <button className="secondary quick-palette-toggle" onClick={() => setQuickPaletteOpen(true)} title="快捷指令（⌘/Ctrl K）"><span className="action-icon" aria-hidden="true">✦</span><span className="action-label">快捷</span></button>
            <button className="secondary config-toggle" onClick={() => openSettings()} title="配置中心"><span className="action-icon" aria-hidden="true">⚙</span><span className="action-label">配置</span></button>
            <button className="secondary session-actions-toggle mobile-new-toggle" onClick={newSession} aria-label="新会话" title="新会话"><span className="new-icon" aria-hidden="true">＋</span></button>
            <button className="theme-toggle" onClick={() => setThemeState(theme === 'day' ? 'night' : 'day')}><span className="action-icon" aria-hidden="true">{theme === 'day' ? '☀' : '☾'}</span><span className="action-label">{theme === 'day' ? '白天' : '夜晚'}</span></button>
            <button className="secondary" onClick={renameCurrent} disabled={!current || busy}>重命名</button>
            <button className="secondary" onClick={copyCurrentMarkdown} disabled={!current}>复制全文</button>
            <button className="secondary" onClick={cloneCurrent} disabled={!current || busy}>复制会话</button>
            <button className="secondary" onClick={pinCurrent} disabled={!current}>{currentPinned ? '取消置顶' : '置顶'}</button>
            <button className="secondary" onClick={exportCurrent} disabled={!current}>导出</button>
            <button className="secondary" onClick={showContextPreview} disabled={!current}>上下文</button>
            <button className="danger" onClick={deleteCurrent} disabled={!current || busy}>删除</button>
          </div>
        </div>
        <div className="messages" ref={messagesRef} onScroll={handleMessagesScroll}>{messages.length ? messages.map((m, i) => <MessageView key={i} message={m} messageIndex={i} onCopy={copyText} onBranch={!busy && current ? branchCurrent : null} onEditUserMessage={editUserMessage} onDownloadAttachment={downloadAttachment} hideThinking={!!config.hide_thinking} onResolveConfirmation={resolveToolConfirmation} onInspectToolEvent={inspectToolEvent} />) : <EmptyState createSession={createSession} openSettings={openSettings} openWorkspacePicker={() => setWorkspacePickerOpen(true)} busy={busy} hasWorkspaces={!!prompts.length} setInput={setInput} modelReady={modelReady} />}</div>
        {showJumpToLatest ? <button type="button" className="jump-latest" onClick={scrollToLatestModelMessage} aria-label="跳到最新模型消息" title="跳到最新模型消息">↓</button> : null}
        <div className="composer-shell">
          {pendingAttachments.length ? <AttachmentList attachments={pendingAttachments} removable={!busy} onRemove={removePendingAttachment} onDownload={downloadAttachment} /> : null}
          <div className="composer">
            <input ref={fileInputRef} type="file" multiple className="file-input" onChange={handleFileSelect} />
            <button className="secondary attach-control" disabled={busy || uploadingFiles} onClick={() => fileInputRef.current?.click()} title="上传文件">+</button>
            <ComposerModelPicker busy={busy} providers={providerChoices} selectedProvider={selectedModelProvider} selectedModel={selectedChatModel} open={modelPickerOpen} setOpen={setModelPickerOpen} selectModel={selectChatModel} openSettings={openSettings} />
            {busy ? <button className="secondary stream-control" onClick={guideActiveJob} disabled={!input.trim()}>引导</button> : null}
            {busy ? <button className="danger stream-control" onClick={stopStreaming}>中断</button> : null}
            <textarea ref={inputRef} id="input" value={input} onChange={e => setInput(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); busy ? guideActiveJob() : sendMsg(); } }} placeholder={busy ? '生成中可输入引导内容，不会中断当前回答' : '输入消息'} />
            <button id="send" disabled={busy || uploadingFiles || (!input.trim() && !pendingAttachmentIDs.length) || !modelReady} onClick={() => sendMsg()} title={!modelReady ? '请先配置模型' : '发送'}>发送</button>
          </div>
          {inputStats ? <div className="composer-meta">{inputStats}</div> : null}
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
