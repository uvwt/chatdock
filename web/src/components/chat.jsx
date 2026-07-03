// Chat workbench, message rendering, empty state, and attachment chips.
import React, { useState } from 'react';
import { fmtBytes } from '../lib/appUtils.js';
import { Markdown } from './base.jsx';

function MessageActions({ text, onCopy }) {
  return <div className="msg-actions">
    <button type="button" className="secondary small msg-action-copy" onClick={() => onCopy(text)} aria-label="复制当前回复" title="复制当前回复">复制</button>
    <button type="button" className="secondary small msg-action-more" aria-label="更多操作" title="更多操作">更多</button>
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

export function AttachmentList({ attachments, removable = false, onRemove }) {
  if (!attachments?.length) return null;
  return <div className="attachment-list">
    {attachments.map(item => <div key={item.id || item.name} className={'attachment-chip ' + (item.error ? 'error' : '')}>
      <span className="attachment-icon">📎</span>
      <span className="attachment-main"><b>{item.name || '附件'}</b><span>{fmtBytes(item.size)} · {attachmentStatusLabel(item)}</span></span>
      {removable ? <button className="attachment-remove" type="button" onClick={() => onRemove?.(item.id)} title="移除附件">×</button> : null}
    </div>)}
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

export function MessageView({ message, onCopy, hideThinking = true, onResolveConfirmation, onInspectToolEvent }) {
  if (message.role === 'empty') return <div className="empty">{message.content}</div>;
  if (message.role === 'assistant-stream') {
    const reasoning = hideThinking ? '' : message.reasoning;
    return <div className="msg assistant">
      <ReasoningBlock value={message.reasoning} streaming hidden={hideThinking} />
      <Markdown className="answer markdown" value={message.answer} />
      {(message.events || []).map((event, i) => <div key={i} className={'tool-event ' + (event.kind === 'run' ? 'run-event-inline' : '') + (event.details ? ' has-details' : '')}>
        <div className="tool-event-main" onClick={() => event.details ? onInspectToolEvent?.(event) : null} role={event.details ? 'button' : undefined} tabIndex={event.details ? 0 : undefined} onKeyDown={e => { if (event.details && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onInspectToolEvent?.(event); } }}>
          <div>{event.text}</div>
          {event.meta ? <div className="tool-event-meta">{event.meta}</div> : null}
        </div>
        <div className="tool-event-actions">
          {event.details ? <button className="secondary small" type="button" onClick={() => onInspectToolEvent?.(event)}>详情</button> : null}
          {event.confirmation && event.status !== 'resolved' ? <><button className="secondary small" type="button" onClick={() => onResolveConfirmation?.(event.confirmation.id, true)}>允许一次</button><button className="danger small" type="button" onClick={() => onResolveConfirmation?.(event.confirmation.id, false)}>拒绝</button></> : null}
        </div>
      </div>)}
      <MessageActions text={[reasoning, message.answer].filter(Boolean).join('\n\n')} onCopy={onCopy} />
    </div>;
  }
  if (message.role === 'assistant') {
    const reasoning = hideThinking ? '' : message.reasoning;
    return <div className="msg assistant markdown">
      <ReasoningBlock value={message.reasoning} hidden={hideThinking} />
      <Markdown value={message.content} />
      <MessageActions text={[reasoning, message.content].filter(Boolean).join('\n\n')} onCopy={onCopy} />
    </div>;
  }
  return <div className={'msg ' + (message.role || 'user')}>{message.content ? <div>{message.content}</div> : null}<AttachmentList attachments={message.attachments || []} /></div>;
}


export function EmptyState({ createSession, openSettings, openWorkspacePicker, busy, hasWorkspaces, setInput, modelReady }) {
  const starterCards = [
    {title:'规划一个任务', text:'把目标拆成可执行步骤，并保留上下文。', prompt:'帮我把这个任务拆成可执行计划：'},
    {title:'整理一段资料', text:'提取结论、风险点和下一步动作。', prompt:'请帮我整理这段资料，并输出重点：'},
    {title:'排查一个问题', text:'按现象、证据、假设和验证步骤推进。', prompt:'我遇到一个问题，请按排障流程分析：'},
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
        <div className="hero-metric-row"><div><b>1. 开始</b><span>新建会话，保留上下文</span></div><strong>↵</strong></div>
        <div className="hero-metric-row"><div><b>2. 调用</b><span>模型、MCP、Skill 统一入口</span></div><strong>⌘K</strong></div>
        <div className="hero-metric-row"><div><b>3. 追踪</b><span>任务、数据和运行记录可复查</span></div><strong>✓</strong></div>
      </div>
    </section>
    <section className="starter-grid">
      {starterCards.map(card => <button key={card.title} type="button" className="starter-card" disabled={busy} onClick={() => { setInput(card.prompt); createSession(); }}>
        <span className="starter-icon">✦</span>
        <b>{card.title}</b>
        <span>{card.text}</span>
      </button>)}
    </section>
    <section className="empty-capability-strip">
      <div><b>配置中心</b><span>模型、Prompt、工具状态统一维护</span></div>
      <div><b>数据状态</b><span>数据库、备份和会话健康可见</span></div>
      <div><b>自动化</b><span>定时任务与运行记录可追踪</span></div>
    </section>
  </div>;
}
