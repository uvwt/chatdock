// Chat workbench, message rendering, empty state, and attachment chips.
import React, { useEffect, useRef, useState } from 'react';
import { fmtBytes } from '../lib/appUtils.js';
import { Markdown } from './base.jsx';

function MessageActions({ text, onCopy, onBranch, onEdit, user = false }) {
  return <div className={'msg-actions ' + (user ? 'user-message-actions' : '')}>
    <button type="button" className="secondary small msg-action-copy" onClick={() => onCopy(text)} aria-label={user ? '复制当前消息' : '复制当前回复'} title={user ? '复制当前消息' : '复制当前回复'}><svg className="msg-action-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M9 8.5h8.5v8.5H9z" /><path d="M6.5 15.5h-1a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v1" /></svg></button>
    {onEdit ? <button type="button" className="secondary small msg-action-edit" onClick={onEdit} aria-label="编辑当前消息" title="编辑当前消息"><svg className="msg-action-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 20h4.5L19 9.5 14.5 5 4 15.5V20z" /><path d="M13.5 6l4.5 4.5" /></svg></button> : null}
    {onBranch ? <button type="button" className="secondary small msg-action-branch" onClick={onBranch} aria-label="在新聊天中创建分支对话" title="在新聊天中创建分支对话"><svg className="msg-action-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 4v7a4 4 0 0 0 4 4h5" /><path d="M13 11l4 4-4 4" /></svg></button> : null}
    {!user ? <button type="button" className="secondary small msg-action-more" aria-label="更多操作" title="更多操作"><svg className="msg-action-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12h.01M12 12h.01M19 12h.01" /></svg></button> : null}
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

function ReasoningBlock({ value, streaming = false, hidden = false }) {
  // 流式输出时思考内容要实时可见；完成后变成普通 assistant 消息，再默认折叠。
  const [open, setOpen] = useState(!!streaming);
  if (hidden || !value) return null;
  const title = '思考过程';
  return <section className={'reasoning ' + (open ? 'show' : 'collapsed')}>
    <button type="button" className="reasoning-toggle" onClick={() => setOpen(v => !v)} aria-expanded={open}>
      <span><b>{title}</b></span>
      <span className="reasoning-chevron">{open ? '⌃' : '⌄'}</span>
    </button>
    {open ? <Markdown className="reasoning-content markdown" value={value} /> : null}
  </section>;
}


function toolEventMetaText(event) {
  const meta = String(event?.meta || '').replace(/^关键词：/, '').trim();
  if (meta) return meta;
  const details = event?.details || {};
  const query = details.arguments?.query || details.data?.arguments?.query || details.data?.result?.query;
  return query ? String(query).trim() : '';
}

function uniqueToolEventMetas(events) {
  const seen = new Set();
  const metas = [];
  for (const event of events) {
    const meta = toolEventMetaText(event);
    if (!meta || seen.has(meta)) continue;
    seen.add(meta);
    metas.push(meta);
  }
  return metas;
}

function ToolEvents({ events = [], onResolveConfirmation, onInspectToolEvent }) {
  const [open, setOpen] = useState(false);
  if (!events.length) return null;

  const confirmations = events.filter(event => event.kind === 'confirm' && event.status !== 'resolved');
  const normalEvents = events.filter(event => !(event.kind === 'confirm' && event.status !== 'resolved'));
  const running = normalEvents.some(event => event.phase === 'running');
  const failed = normalEvents.some(event => event.phase === 'error');
  const metas = uniqueToolEventMetas(normalEvents);
  const title = failed ? '工具调用有异常' : (running ? '正在调用工具' : '已完成工具调用');
  const metaText = metas.length ? metas.slice(0, 6).join('、') + (metas.length > 6 ? ' 等' : '') : '';

  return <>
    {normalEvents.length ? <section className={'tool-events-compact ' + (open ? 'open' : '')}>
      <button type="button" className="tool-events-toggle" onClick={() => setOpen(value => !value)} aria-expanded={open}>
        <span className="tool-events-dot" />
        <span className="tool-events-title">{title} · {normalEvents.length} 次</span>
        {metaText ? <span className="tool-events-meta">{metaText}</span> : null}
        <span className="tool-events-chevron">{open ? '⌃' : '⌄'}</span>
      </button>
      {open ? <div className="tool-events-list">
        {normalEvents.map((event, i) => <div key={i} className={'tool-event ' + (event.kind === 'run' ? 'run-event-inline ' : '') + (event.phase ? 'phase-' + event.phase + ' ' : '') + (event.details ? 'has-details' : '')} onClick={() => event.details ? onInspectToolEvent?.(event) : null} role={event.details ? 'button' : undefined} tabIndex={event.details ? 0 : undefined} onKeyDown={e => { if (event.details && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onInspectToolEvent?.(event); } }}>
          <div className="tool-event-main">
            <div>{event.text}</div>
            {event.meta ? <div className="tool-event-meta">{event.meta}</div> : null}
          </div>
        </div>)}
      </div> : null}
    </section> : null}
    {confirmations.map((event, i) => <div key={'confirm-' + i} className={'tool-event ' + (event.details ? 'has-details' : '')} onClick={() => event.details ? onInspectToolEvent?.(event) : null} role={event.details ? 'button' : undefined} tabIndex={event.details ? 0 : undefined} onKeyDown={e => { if (event.details && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onInspectToolEvent?.(event); } }}>
      <div className="tool-event-main">
        <div>{event.text}</div>
        {event.meta ? <div className="tool-event-meta">{event.meta}</div> : null}
      </div>
      <div className="tool-event-actions" onClick={e => e.stopPropagation()}>
        <button className="secondary small" type="button" onClick={() => onResolveConfirmation?.(event.confirmation.id, true)}>允许一次</button>
        <button className="danger small" type="button" onClick={() => onResolveConfirmation?.(event.confirmation.id, false)}>拒绝</button>
      </div>
    </div>)}
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
        <textarea ref={editRef} value={draft} disabled={saving} onChange={e => setDraft(e.target.value)} onKeyDown={e => { if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); saveEdit(); } if (e.key === 'Escape') { e.preventDefault(); setDraft(text); setEditing(false); } }} />
        <div className="user-message-editor-actions">
          <button type="button" className="secondary small" disabled={saving} onClick={() => { setDraft(text); setEditing(false); }}>取消</button>
          <button type="button" className="primary small" disabled={saving || !draft.trim()} onClick={saveEdit}>{saving ? '保存中' : '保存'}</button>
        </div>
        <small>保存后会替换这条消息，并删除它下面的所有消息。</small>
      </div> : <>{text ? <div>{text}</div> : null}<AttachmentList attachments={message.attachments || []} onDownload={onDownloadAttachment} /></>}
    </div>
    {!editing ? <MessageActions text={text} onCopy={onCopy} onEdit={() => setEditing(true)} user /> : null}
  </div>;
}

export function MessageView({ message, messageIndex = -1, onCopy, onBranch, onEditUserMessage, onDownloadAttachment, hideThinking = true, onResolveConfirmation, onInspectToolEvent }) {
  if (message.role === 'empty') return <div className="empty">{message.content}</div>;
  if (message.role === 'assistant-stream') {
    const reasoning = hideThinking ? '' : message.reasoning;
    return <div className="msg assistant" data-model-message="true">
      <ReasoningBlock value={message.reasoning} streaming hidden={hideThinking} />
      <Markdown className="answer markdown" value={message.answer} />
      <ToolEvents events={message.events || []} onResolveConfirmation={onResolveConfirmation} onInspectToolEvent={onInspectToolEvent} />
      <MessageActions text={[reasoning, message.answer].filter(Boolean).join('\n\n')} onCopy={onCopy} onBranch={onBranch ? () => onBranch(messageIndex) : null} />
    </div>;
  }
  if (message.role === 'assistant') {
    const reasoning = hideThinking ? '' : message.reasoning;
    return <div className="msg assistant markdown" data-model-message="true">
      <ReasoningBlock value={message.reasoning} hidden={hideThinking} />
      <Markdown value={message.content} />
      <MessageActions text={[reasoning, message.content].filter(Boolean).join('\n\n')} onCopy={onCopy} onBranch={onBranch ? () => onBranch(messageIndex) : null} />
    </div>;
  }
  const role = message.role || 'user';
  if (role === 'user') {
    return <UserMessageView message={message} messageIndex={messageIndex} onCopy={onCopy} onEditUserMessage={onEditUserMessage} onDownloadAttachment={onDownloadAttachment} />;
  }
  const text = message.content || '';
  return <div className={'msg ' + role}>{text ? <div>{text}</div> : null}<AttachmentList attachments={message.attachments || []} onDownload={onDownloadAttachment} /></div>;
}


export function EmptyState({ createSession, openSettings, openWorkspacePicker, busy, hasWorkspaces, setInput, modelReady }) {
  const flowSteps = [
    {title:'开始', text:'先开一个干净会话，保留后续上下文。', key:'↵'},
    {title:'调用', text:'模型、MCP、Skill 和附件统一进同一入口。', key:'⌘K'},
    {title:'追踪', text:'任务、工具事件和运行记录都能复查。', key:'✓'},
  ];
  return <div className="empty-state product-empty-state">
    <section className="product-hero">
      <div className="hero-copy">
        <div className="empty-state-kicker"><span className="kicker-dot" /> ChatDock 工作台</div>
        <h1>今天想完成什么？</h1>
        <p>从一个会话开始，把模型配置、工具调用、任务记录和数据状态收在同一个工作流里。</p>
        <div className="empty-state-actions hero-actions">
          <button disabled={busy || !modelReady} onClick={createSession}>{modelReady ? '开始新会话' : '先配置模型'}</button>
          <button className="secondary" onClick={() => openSettings('model')}>{modelReady ? '检查模型' : '配置模型'}</button>
          <button className="secondary" disabled={!hasWorkspaces || busy} onClick={openWorkspacePicker}>切换工作空间</button>
        </div>
        <div className="hero-trust-row">
          <span>本地数据优先</span><span>工作空间隔离</span><span>快捷指令 ⌘K</span>
        </div>
      </div>
      <div className="hero-panel" aria-label="ChatDock 工作台能力概览">
        <div className="hero-panel-top"><span>当前流程</span><b>Ready</b></div>
        {flowSteps.map((step, index) => <div key={step.title} className="hero-metric-row">
          <div><small>{String(index + 1).padStart(2, '0')}</small><b>{step.title}</b><span>{step.text}</span></div><strong>{step.key}</strong>
        </div>)}
      </div>
    </section>
  </div>;
}
