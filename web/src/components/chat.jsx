// Chat workbench, message rendering, empty state, and attachment chips.
import React, { useEffect, useRef, useState } from 'react';
import { fmtBytes } from '../lib/appUtils.js';
import { assistantMessageBlocks, executionBlockSummary, toolEventDisplayName, toolEventMetaText } from '../lib/messageExecution.js';
import { Markdown } from './base.jsx';

function MessageActions({ text, onCopy, onBranch, onEdit, user = false }) {
  return <div className={'msg-actions ' + (user ? 'user-message-actions' : '')}>
    <button type="button" className="secondary small msg-action-copy" onClick={() => onCopy(text)} aria-label={user ? '复制当前消息' : '复制当前回复'} title={user ? '复制当前消息' : '复制当前回复'}><svg className="msg-action-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2" /></svg></button>
    {onEdit ? <button type="button" className="secondary small msg-action-edit" onClick={onEdit} aria-label="编辑当前消息" title="编辑当前消息"><svg className="msg-action-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 20h9" /><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L8 18l-4 1 1-4Z" /></svg></button> : null}
    {onBranch ? <button type="button" className="secondary small msg-action-branch" onClick={onBranch} aria-label="在新聊天中创建分支对话" title="在新聊天中创建分支对话"><svg className="msg-action-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 4v8a4 4 0 0 0 4 4h9" /><path d="m16 13 3 3-3 3" /></svg></button> : null}
    {!user ? <button type="button" className="secondary small msg-action-more" aria-label="更多操作" title="更多操作"><svg className="msg-action-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="5" cy="12" r="1.4" /><circle cx="12" cy="12" r="1.4" /><circle cx="19" cy="12" r="1.4" /></svg></button> : null}
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
        <span className="attachment-icon">📎</span>
        <span className="attachment-main"><b>{item.name || '附件'}</b><span>{fmtBytes(item.size)} · {attachmentStatusLabel(item)}</span></span>
        {removable ? <button className="attachment-remove" type="button" onClick={e => { e.stopPropagation(); onRemove?.(item.id); }} title="移除附件">×</button> : null}
      </div>;
    })}
  </div>;
}

function toolEventStatus(event) {
  if (event.phase === 'running') return {icon: '○', text: '运行中'};
  if (event.phase === 'error') return {icon: '×', text: '失败'};
  return {icon: '✓', text: '完成'};
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
    ? '✦'
    : (summary.tone === 'running' ? '○' : (summary.tone === 'error' ? '!' : '✓'));
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
      <span className="execution-summary-chevron" aria-hidden="true">›</span>
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

export function MessageView({ message, messageIndex = -1, onCopy, onBranch, onEditUserMessage, onDownloadAttachment, hideThinking = true, onResolveConfirmation, onInspectToolEvent }) {
  if (message.role === 'empty') return <div className="empty">{message.content}</div>;
  if (message.role === 'assistant-stream') {
    return <div className="msg assistant" data-model-message="true">
      <AssistantContent message={message} streaming hideThinking={hideThinking} onResolveConfirmation={onResolveConfirmation} onInspectToolEvent={onInspectToolEvent} />
    </div>;
  }
  if (message.role === 'assistant') {
    const copyText = assistantMessageBlocks(message, {hideThinking: true}).textParts.join('\n\n');
    return <div className="msg assistant markdown" data-model-message="true">
      <AssistantContent message={message} hideThinking={hideThinking} onResolveConfirmation={onResolveConfirmation} onInspectToolEvent={onInspectToolEvent} />
      <MessageActions text={copyText} onCopy={onCopy} onBranch={onBranch ? () => onBranch(messageIndex) : null} />
    </div>;
  }
  const role = message.role || 'user';
  if (role === 'user') {
    return <UserMessageView message={message} messageIndex={messageIndex} onCopy={onCopy} onEditUserMessage={onEditUserMessage} onDownloadAttachment={onDownloadAttachment} />;
  }
  const text = message.content || '';
  return <div className={'msg ' + role}>{text ? <div>{text}</div> : null}<AttachmentList attachments={message.attachments || []} onDownload={onDownloadAttachment} /></div>;
}

export const MemoizedMessageView = React.memo(MessageView);

export function EmptyState({ createSession, openSettings, openWorkspacePicker, busy, hasWorkspaces, setInput, modelReady }) {
  const starters = [
    { number: '01', title: '梳理一个需求', text: '把模糊想法拆成清晰步骤和风险。', prompt: '帮我把这个需求拆成可执行步骤，并指出主要风险：' },
    { number: '02', title: '检查当前项目', text: '快速找出最值得优先处理的问题。', prompt: '检查当前项目状态，找出最需要优先处理的问题。' },
    { number: '03', title: '设计自动化', text: '规划触发条件、执行步骤和失败处理。', prompt: '帮我设计一个自动化任务，包括触发方式、执行步骤和失败处理。' },
  ];
  return <div className="empty-state product-empty-state">
    <section className="product-hero">
      <div className="hero-copy">
        <div className="empty-state-kicker"><span className="kicker-dot" /> ChatDock</div>
        <h1>今天想完成什么？</h1>
        <p>从聊天开始，模型、工具和任务会自然汇入同一条工作流。</p>
        <div className="empty-state-actions hero-actions">
          <button disabled={busy || !modelReady} onClick={createSession}>{modelReady ? '新建会话' : '先配置模型'}</button>
          <button className="secondary" onClick={() => openSettings('model')}>{modelReady ? '模型配置' : '配置模型'}</button>
          <button className="secondary" disabled={!hasWorkspaces || busy} onClick={openWorkspacePicker}>切换工作区</button>
        </div>
      </div>
      <div className="starter-grid" aria-label="常用起始任务">
        {starters.map(item => <button key={item.title} type="button" className="starter-card" onClick={() => setInput(item.prompt)}>
          <span className="starter-icon">{item.number}</span>
          <b>{item.title}</b>
          <span>{item.text}</span>
          <span className="starter-card-arrow" aria-hidden="true">↗</span>
        </button>)}
      </div>
      <div className="hero-trust-row" aria-label="工作台特性">
        <span>本地数据优先</span><span>工作空间隔离</span><span>快捷指令 ⌘K</span>
      </div>
    </section>
  </div>;
}
