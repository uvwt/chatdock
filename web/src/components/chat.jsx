// Chat workbench, message rendering, empty state, and attachment chips.
import React, { useEffect, useRef, useState } from 'react';
import {
  ChevronDown,
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

function MessageActions({ text, onCopy, onBranch, onEdit, user = false }) {
  return <div className={'msg-actions ' + (user ? 'user-message-actions' : '')}>
    <button type="button" className="secondary small msg-action-copy" onClick={() => onCopy(text)} aria-label={user ? '复制当前消息' : '复制当前回复'} title={user ? '复制当前消息' : '复制当前回复'}><Copy className="msg-action-icon" size={16} aria-hidden="true" /></button>
    {onEdit ? <button type="button" className="secondary small msg-action-edit" onClick={onEdit} aria-label="编辑当前消息" title="编辑当前消息"><Pencil className="msg-action-icon" size={16} aria-hidden="true" /></button> : null}
    {onBranch ? <button type="button" className="secondary small msg-action-branch" onClick={onBranch} aria-label="在新聊天中创建分支对话" title="在新聊天中创建分支对话"><GitBranch className="msg-action-icon" size={16} aria-hidden="true" /></button> : null}
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

function ExecutionBlock({ block, streaming = false, onInspectToolEvent }) {
  const [manuallyOpen, setManuallyOpen] = useState(false);
  const summary = executionBlockSummary(block, {streaming});
  const events = block.kind === 'tools' ? block.events : [];
  const reasoning = block.kind === 'reasoning' ? block.text : '';
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
      <span className="execution-summary-chevron" aria-hidden="true"><ChevronDown className={open ? 'is-open' : ''} size={15} /></span>
    </button>
    {open ? <div className="execution-inline-detail">
      {events.length ? <div className="execution-inline-tools">{events.map((event, index) => <ToolEventRow key={event.callKey || event.id || index} event={event} onInspectToolEvent={onInspectToolEvent} />)}</div> : null}
      {reasoning ? <div className="execution-inline-reasoning"><Markdown className="markdown" value={reasoning} /></div> : null}
    </div> : null}
  </section>;
}


function ErrorNotice({ error }) {
  const message = String(error?.message || error || '').trim();
  if (!message) return null;
  const raw = String(error?.raw || '').trim();
  const code = String(error?.code || '').trim();
  const requestID = String(error?.request_id || '').trim();
  return <div className="chat-error-card" role="alert">
    <b>响应中断</b>
    <span>{message}</span>
    {requestID ? <small>请求 ID：{requestID}</small> : null}
    {raw ? <details className="chat-error-details">
      <summary>查看原始错误</summary>
      {code ? <small>错误码：{code}{error?.retryable ? ' · 可重试' : ''}</small> : null}
      <pre>{raw}</pre>
    </details> : null}
  </div>;
}

function AssistantContent({ message, streaming = false, hideThinking = false, onResolveConfirmation, onInspectToolEvent }) {
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
      />;
    })}
    {!blocks.length && streaming ? <div className="assistant-waiting" role="status" aria-label="模型正在生成">
      <span className="assistant-waiting-dot" aria-hidden="true" />
    </div> : null}
    <ErrorNotice error={message.error} />
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

export function MessageView({ message, previousMessage, messageIndex = -1, onCopy, onBranch, onEditUserMessage, onDownloadAttachment, hideThinking = true, onResolveConfirmation, onInspectToolEvent }) {
  if (message.role === 'loading') return <ConversationLoadingState label={message.content} />;
  if (message.role === 'empty') return <div className="empty">{message.content}</div>;

  const timeLabel = shouldShowMessageTimeDivider(previousMessage, message)
    ? formatMessageTimeDivider(message.created_at)
    : '';
  let content;

  if (message.role === 'assistant-stream') {
    content = <div className="msg assistant" data-model-message="true">
      <AssistantContent message={message} streaming hideThinking={hideThinking} onResolveConfirmation={onResolveConfirmation} onInspectToolEvent={onInspectToolEvent} />
    </div>;
  } else if (message.role === 'assistant') {
    const copyText = assistantMessageBlocks(message, {hideThinking: true}).textParts.join('\n\n');
    content = <div className="msg assistant markdown" data-model-message="true">
      <AssistantContent message={message} hideThinking={hideThinking} onResolveConfirmation={onResolveConfirmation} onInspectToolEvent={onInspectToolEvent} />
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

export function EmptyState({ createSession, openSettings, busy, modelReady }) {
  return <div className="empty-state product-empty-state">
    <section className="product-hero">
      <div className="hero-ambient" aria-hidden="true">
        <span className="hero-orbit hero-orbit-a" />
        <span className="hero-orbit hero-orbit-b" />
        <span className="hero-core"><i /></span>
      </div>
      <div className="hero-copy">
        <div className="empty-state-kicker">ChatDock · AI Console</div>
        <h1>把想法，<span>推进到完成。</span></h1>
        <p>在一个会话里串起模型、工具与任务。过程清晰，结果可追踪。</p>
        <div className="empty-state-actions hero-actions">
          <button disabled={busy || !modelReady} onClick={createSession}>{modelReady ? '开始对话' : '先配置模型'}</button>
          <button className="secondary" onClick={() => openSettings('model')}>{modelReady ? '配置模型' : '打开配置'}</button>
        </div>
      </div>
    </section>
  </div>;
}
