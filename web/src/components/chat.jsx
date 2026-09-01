// Chat workbench, message rendering, empty state, and attachment chips.
import React, { useEffect, useRef, useState } from 'react';
import {
  ChevronRight,
  CircleX,
  Copy,
  GitBranch,
  MoreHorizontal,
  Paperclip,
  Pencil,
  Trash2,
} from './icons.js';
import { fmtBytes } from '../lib/appUtils.js';
import { assistantMessageBlocks, executionBlockSummary, toolEventDisplayName, toolEventMetaText } from '../lib/messageExecution.js';
import { formatMessageTimeDivider, shouldShowMessageTimeDivider } from '../lib/messageTimeline.js';
import { Markdown } from './base.jsx';
import { MCPAppFrame } from './mcpApp.jsx';
import { mcpAppArgumentsFromEvent, mcpAppResultFromEvent } from '../lib/mcpApps.js';
import { Tooltip } from '../shared/ui/tooltip.jsx';

function MessageActions({ text, onCopy, onBranch, onEdit, user = false }) {
  const copyLabel = user ? '复制当前消息' : '复制当前回复';
  const editLabel = '编辑当前消息';
  const branchLabel = '在新聊天中创建分支对话';

  return <div className={'msg-actions ' + (user ? 'user-message-actions' : '')}>
    <Tooltip content={copyLabel}>
      <button type="button" className="secondary small msg-action-copy" onClick={() => onCopy(text)} aria-label={copyLabel}><Copy className="msg-action-icon" size={16} aria-hidden="true" /></button>
    </Tooltip>
    {onEdit ? <Tooltip content={editLabel}>
      <button type="button" className="secondary small msg-action-edit" onClick={onEdit} aria-label={editLabel}><Pencil className="msg-action-icon" size={16} aria-hidden="true" /></button>
    </Tooltip> : null}
    {onBranch ? <Tooltip content={branchLabel}>
      <button type="button" className="secondary small msg-action-branch" onClick={onBranch} aria-label={branchLabel}><GitBranch className="msg-action-icon" size={16} aria-hidden="true" /></button>
    </Tooltip> : null}
    {!user ? <button type="button" className="secondary small msg-action-more" aria-label="更多操作" title="更多操作"><MoreHorizontal className="msg-action-icon" size={16} aria-hidden="true" /></button> : null}
  </div>;
}

function attachmentStatusLabel(item) {
  if (item.uploading) return '上传中 ' + (item.progress || 0) + '%';
  if (item.error) return '失败：' + item.error;
  if (item.has_text) return '已提取文本';
  if (item.status === 'extracted') return '已提取文本';
  if (item.status === 'stored') return '已上传';
  return item.status || '已上传';
}

export function AttachmentList({ attachments, removable = false, onRemove, onDownload }) {
  if (!attachments?.length) return null;
  return <div className="attachment-list">
    {attachments.map(item => {
      const canDownload = !!onDownload && item.id && !item.uploading && !item.error && !String(item.id).startsWith('local_');
      const download = () => canDownload ? onDownload(item) : null;
      return <div key={item.id || item.name} className={'attachment-chip ' + (item.error ? 'error ' : '') + (canDownload ? 'downloadable' : '')} onClick={download} role={canDownload ? 'button' : undefined} tabIndex={canDownload ? 0 : undefined} onKeyDown={e => { if (canDownload && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); download(); } }} title={canDownload ? '点击下载附件' : undefined}>
        <span className="attachment-icon"><Paperclip size={16} aria-hidden="true" /></span>
        <span className="attachment-main"><b>{item.name || '附件'}</b><span>{fmtBytes(item.size)} · {attachmentStatusLabel(item)}</span></span>
        {removable ? <button className="attachment-remove icon-button" type="button" onClick={e => { e.stopPropagation(); onRemove?.(item.id); }} title="移除附件" aria-label="移除附件"><Trash2 size={14} aria-hidden="true" /></button> : null}
      </div>;
    })}
  </div>;
}

function toolEventStatus(event) {
  if (event.phase === 'running') return { icon: '\u25CB', text: '运行中' };
  if (event.phase === 'error') return { icon: '\u00D7', text: '失败' };
  return { icon: '\u2713', text: '完成' };
}

function ToolEventRow({ event, onInspectToolEvent }) {
  const status = toolEventStatus(event);
  const name = toolEventDisplayName(event);
  const meta = toolEventMetaText(event);
  return <button type="button" className={'tool-step-row ' + (event.phase ? 'phase-' + event.phase : '')} onClick={() => event.details ? onInspectToolEvent?.(event) : null} disabled={!event.details}>
    <span className="tool-step-icon">{status.icon}</span>
    <span className="tool-step-body">
      <span className="tool-step-name">{name}</span>
      {meta ? <span className="tool-step-meta">{meta}</span> : null}
    </span>
    <span className="tool-step-status">{status.text}</span>
  </button>;
}

function PendingConfirmations({ confirmations = [], onResolveConfirmation, onInspectToolEvent }) {
  return confirmations.map((event, i) => <div key={'confirm-' + i} className={'tool-event ' + (event.details ? 'has-details' : '')} onClick={() => event.details ? onInspectToolEvent?.(event) : null} role={event.details ? 'button' : undefined} tabIndex={event.details ? 0 : undefined} onKeyDown={e => { if (event.details && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onInspectToolEvent?.(event); } }}>
      <div className="tool-event-main">
        <div>{event.text}</div>
        {event.meta ? <div className="tool-event-meta">{event.meta}</div> : null}
      </div>
      <div className="tool-event-actions" onClick={e => e.stopPropagation()}>
        <button className="secondary small" type="button" onClick={() => onResolveConfirmation?.(event.confirmation.id, true)}>允许一次</button>
        <button className="danger small" type="button" onClick={() => onResolveConfirmation?.(event.confirmation.id, false)}>拒绝</button>
      </div>
    </div>);
}

function MCPAppEventFrame({ event, onMCPAppToolCall, onResolveToolEvent }) {
  const [resolvedEvent, setResolvedEvent] = useState(event);
  const [loadError, setLoadError] = useState('');
  const initialData = event?.details?.data || {};
  const descriptor = initialData.mcp_app;
  const needsHydration = !!descriptor && !descriptor.html && !!event?.details?.lazy;

  useEffect(() => {
    setResolvedEvent(event);
    setLoadError('');
    if (!needsHydration || !onResolveToolEvent) return undefined;
    let active = true;
    onResolveToolEvent(event).then(fullEvent => {
      if (active) setResolvedEvent(fullEvent || event);
    }).catch(error => {
      if (active) setLoadError(String(error?.message || error || '加载失败'));
    });
    return () => { active = false; };
  }, [event, needsHydration, onResolveToolEvent]);

  const data = resolvedEvent?.details?.data || {};
  if (loadError) return <div className="mcp-app-error" role="status">MCP App 详情加载失败：{loadError}</div>;
  if (!data.mcp_app) {
    if (data.mcp_app_error) return <div className="mcp-app-error" role="status">MCP App 无法加载：{data.mcp_app_error}</div>;
    if (needsHydration) return <div className="mcp-app-loading" role="status">正在加载 MCP App…</div>;
    return null;
  }
  if (!data.mcp_app.html) return <div className="mcp-app-loading" role="status">正在加载 MCP App…</div>;
  return <MCPAppFrame
    app={data.mcp_app}
    arguments={mcpAppArgumentsFromEvent(resolvedEvent)}
    result={mcpAppResultFromEvent(resolvedEvent)}
    sourceTool={resolvedEvent?.details?.tool}
    onToolCall={onMCPAppToolCall}
  />;
}

function ExecutionBlock({ block, streaming = false, onInspectToolEvent, onMCPAppToolCall, onResolveToolEvent }) {
  const [manuallyOpen, setManuallyOpen] = useState(false);
  const summary = executionBlockSummary(block, {streaming});
  const events = block.kind === 'tools' ? block.events : [];
  const reasoning = block.kind === 'reasoning' ? block.text : '';
  const appEvents = events.filter(event => event?.details?.data?.mcp_app || event?.details?.data?.mcp_app_error);
  const open = streaming || manuallyOpen;

  useEffect(() => {
    // 流式阶段结束后自动恢复折叠，历史消息仍可在之后手动展开。
    if (!streaming) setManuallyOpen(false);
  }, [streaming]);

  const icon = block.kind === 'reasoning'
    ? '\u2726'
    : (summary.tone === 'running' ? '\u25CB' : (summary.tone === 'error' ? '!' : '\u2713'));
  const summaryLabel = [summary.label, summary.meta].filter(Boolean).join('，');
  return <section className={`execution-summary kind-${block.kind} tone-${summary.tone}${open ? ' is-open' : ''}`}>
    <button
      type="button"
      className="execution-summary-trigger"
      onClick={() => { if (!streaming) setManuallyOpen(value => !value); }}
      aria-expanded={open}
      aria-label={streaming ? `${summaryLabel}，流式详情已展开` : `${summaryLabel}，点击${open ? '收起' : '展开'}详情`}
    >
      <span className="execution-summary-icon" aria-hidden="true">{icon}</span>
      <span className="execution-summary-copy">
        <b>{summary.label}</b>
        {summary.meta ? <small>{summary.meta}</small> : null}
      </span>
      <span className="execution-summary-chevron" aria-hidden="true"><ChevronRight className={open ? 'is-open' : ''} size={15} /></span>
    </button>
    {open ? <div className="execution-inline-detail">
      {events.length ? <div className="execution-inline-tools">{events.map((event, index) => <ToolEventRow key={event.callKey || event.id || index} event={event} onInspectToolEvent={onInspectToolEvent} />)}</div> : null}
      {reasoning ? <div className="execution-inline-reasoning"><Markdown className="markdown" value={reasoning} /></div> : null}
    </div> : null}
    {appEvents.length ? <div className="execution-mcp-apps">{appEvents.map((event, index) => {
      const data = event?.details?.data || {};
      return <MCPAppEventFrame key={(event.callKey || index) + ':' + (data.mcp_app?.resource_uri || 'error')} event={event} onMCPAppToolCall={onMCPAppToolCall} onResolveToolEvent={onResolveToolEvent} />;
    })}</div> : null}
  </section>;
}


function ErrorNotice({ error }) {
  const message = String(error?.message || error || '').trim();
  if (!message) return null;
  const raw = String(error?.raw || '').trim();
  const code = String(error?.code || '').trim();
  const requestID = String(error?.request_id || '').trim();
  return <div className="chat-error-card" role="alert">
    <CircleX className="chat-error-icon" size={18} aria-hidden="true" />
    <div className="chat-error-content">
      <b>响应中断</b>
      <p className="chat-error-message">{message}</p>
      {(requestID || raw) ? <div className="chat-error-meta">
        {requestID ? <small>请求 ID：{requestID}</small> : null}
        {raw ? <details className="chat-error-details">
          <summary>查看原始错误</summary>
          {code ? <small>错误码：{code}{error?.retryable ? ' · 可重试' : ''}</small> : null}
          <pre>{raw}</pre>
        </details> : null}
      </div> : null}
    </div>
  </div>;
}

function MessageUsage({ usage, missing = false }) {
  if (!usage && !missing) return null;
  if (!usage) return <details className="message-usage message-usage-missing">
    <summary title="供应商未返回用量">用量 未提供</summary>
    <div className="message-usage-detail" role="status">供应商未提供 usage，未使用估算值。</div>
  </details>;
  const hit = Number(usage.cache_hit_tokens || 0);
  const miss = Number(usage.cache_miss_tokens || 0);
  const cacheTotal = hit + miss;
  const cacheRate = cacheTotal ? Math.round(hit / cacheTotal * 100) : 0;
  const total = Number(usage.total_tokens || 0).toLocaleString('zh-CN');
  return <details className="message-usage">
    <summary title={`总 Token：${total}${cacheTotal ? ` · 缓存命中率 ${cacheRate}%` : ' · 缓存未提供'}`}>用量 {total}</summary>
    <div className="message-usage-detail" role="status"><span>输入 {Number(usage.input_tokens || 0).toLocaleString('zh-CN')} · 输出 {Number(usage.output_tokens || 0).toLocaleString('zh-CN')}</span><span>缓存命中 {hit.toLocaleString('zh-CN')} · 未命中 {miss.toLocaleString('zh-CN')}{cacheTotal ? ` · 命中率 ${cacheRate}%` : ''}</span>{usage.reasoning_tokens ? <span>推理 {Number(usage.reasoning_tokens).toLocaleString('zh-CN')}</span> : null}</div>
  </details>;
}

function AssistantContent({ message, streaming = false, hideThinking = false, onResolveConfirmation, onInspectToolEvent, onMCPAppToolCall, onResolveToolEvent }) {
  const { blocks } = assistantMessageBlocks(message, {streaming, hideThinking});
  const lastBlock = blocks[blocks.length - 1];
  const activeExecutionIndex = streaming && (lastBlock?.kind === 'reasoning' || lastBlock?.kind === 'tools')
    ? blocks.length - 1
    : -1;

  return <>
    {blocks.map((block, index) => {
      if (block.kind === 'text') {
        return <Markdown key={'text-' + index} className={streaming ? 'answer markdown' : undefined} value={block.text} />;
      }
      if (block.kind === 'confirmation') {
        return <PendingConfirmations key={'confirm-' + index} confirmations={[block.event]} onResolveConfirmation={onResolveConfirmation} onInspectToolEvent={onInspectToolEvent} />;
      }
      return <ExecutionBlock
        key={block.kind + '-' + index}
        block={block}
        streaming={index === activeExecutionIndex}
        onInspectToolEvent={onInspectToolEvent}
        onMCPAppToolCall={onMCPAppToolCall}
        onResolveToolEvent={onResolveToolEvent}
      />;
    })}
    {!blocks.length && streaming ? <div className="assistant-waiting" role="status" aria-label="模型正在生成">
      <span className="assistant-waiting-dot" aria-hidden="true" />
    </div> : null}
    <ErrorNotice error={message.error} />
    {!streaming ? <MessageUsage usage={message.usage} missing /> : null}
  </>;
}

function UserMessageView({ message, messageIndex, onCopy, onEditUserMessage, onDownloadAttachment }) {
  const text = message.content || '';
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(text);
  const [saving, setSaving] = useState(false);
  const editRef = useRef(null);

  useEffect(() => {
    if (!editing) setDraft(text);
  }, [editing, text]);

  useEffect(() => {
    if (editing) requestAnimationFrame(() => editRef.current?.focus());
  }, [editing]);

  async function saveEdit() {
    const value = draft.trim();
    if (!value || saving) return;
    setSaving(true);
    try {
      await onEditUserMessage?.({messageIndex, messageID: message.id || '', content: value});
      setEditing(false);
    } finally {
      setSaving(false);
    }
  }

  return <div className={'user-message-wrap ' + (editing ? 'editing' : '')}>
    <div className="msg user">
      {editing ? <div className="user-message-editor">
        <textarea rows={1} aria-label="编辑消息" ref={editRef} value={draft} disabled={saving} onChange={e => setDraft(e.target.value)} onKeyDown={e => { if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); saveEdit(); } if (e.key === 'Escape') { e.preventDefault(); setDraft(text); setEditing(false); } }} />
        <div className="user-message-editor-footer">
          <small>保存后会删除后续消息</small>
          <div className="user-message-editor-actions">
            <button type="button" className="secondary small" disabled={saving} onClick={() => { setDraft(text); setEditing(false); }}>取消</button>
            <button type="button" className="primary small" disabled={saving || !draft.trim()} onClick={saveEdit}>{saving ? '保存中' : '保存'}</button>
          </div>
        </div>
      </div> : <>{text ? <div>{text}</div> : null}<AttachmentList attachments={message.attachments || []} onDownload={onDownloadAttachment} /></>}
    </div>
    {!editing ? <MessageActions text={text} onCopy={onCopy} onEdit={() => setEditing(true)} user /> : null}
  </div>;
}

function ConversationLoadingState({ label = '正在加载会话' }) {
  return <div className="conversation-loading" role="status" aria-live="polite">
    <span className="conversation-loading-mark" aria-hidden="true"><i /><i /><i /></span>
    <span>{label}</span>
  </div>;
}

export function MessageView({ message, previousMessage, messageIndex = -1, onCopy, onBranch, onEditUserMessage, onDownloadAttachment, hideThinking = true, onResolveConfirmation, onInspectToolEvent, onMCPAppToolCall, onResolveToolEvent }) {
  if (message.role === 'loading') return <ConversationLoadingState label={message.content} />;
  if (message.role === 'empty') return <div className="empty">{message.content}</div>;

  const timeLabel = shouldShowMessageTimeDivider(previousMessage, message)
    ? formatMessageTimeDivider(message.created_at)
    : '';
  let content;

  if (message.role === 'assistant-stream') {
    content = <div className="msg assistant" data-model-message="true">
      <AssistantContent message={message} streaming hideThinking={hideThinking} onResolveConfirmation={onResolveConfirmation} onInspectToolEvent={onInspectToolEvent} onMCPAppToolCall={onMCPAppToolCall} onResolveToolEvent={onResolveToolEvent} />
    </div>;
  } else if (message.role === 'assistant') {
    const copyText = assistantMessageBlocks(message, {hideThinking: true}).textParts.join('\n\n');
    content = <div className="msg assistant markdown" data-model-message="true">
      <AssistantContent message={message} hideThinking={hideThinking} onResolveConfirmation={onResolveConfirmation} onInspectToolEvent={onInspectToolEvent} onMCPAppToolCall={onMCPAppToolCall} onResolveToolEvent={onResolveToolEvent} />
      <MessageActions text={copyText} onCopy={onCopy} onBranch={onBranch ? () => onBranch(messageIndex) : null} />
    </div>;
  } else {
    const role = message.role || 'user';
    if (role === 'user') {
      content = <UserMessageView message={message} messageIndex={messageIndex} onCopy={onCopy} onEditUserMessage={onEditUserMessage} onDownloadAttachment={onDownloadAttachment} />;
    } else {
      const text = message.content || '';
      content = <div className={'msg ' + role}>{text ? <div>{text}</div> : null}<AttachmentList attachments={message.attachments || []} onDownload={onDownloadAttachment} /></div>;
    }
  }

  return <>
    {timeLabel ? <div className="message-time-divider" role="separator" aria-label={'会话时间 ' + timeLabel}><time dateTime={message.created_at}>{timeLabel}</time></div> : null}
    {content}
  </>;
}

export const MemoizedMessageView = React.memo(MessageView);

export function EmptyState() {
  return <div className="conversation-start">
    <h1>有什么可以帮忙的？</h1>
  </div>;
}
