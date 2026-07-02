// Chat workbench, message rendering, empty state, and attachment chips.
import React from 'react';
import { fmtBytes } from '../lib/appUtils.js';
import { Markdown } from './base.jsx';

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

export function WorkbenchBrief({ setupStatus, config, activePrompt, sessions, skills, scheduledTasks, mcpStatus, dataStatus, productReady, busy, streamStats, openSettings }) {
  const modelReady = !!String(config.base_url || '').trim() && !!String(config.model || '').trim();
  const keyReady = !!config.has_api_key || !!String(config.api_key || '').trim();
  const mcpReady = (mcpStatus || []).some(s => !s.disabled && s.last_status === 'ok');
  const dbHealthy = dataStatus == null ? true : dataStatus.database_healthy !== false;
  const items = [
    {label:'工作空间', value:activePrompt?.name || setupStatus?.active_workspace || 'default', ok:!!activePrompt || !!setupStatus?.has_workspace, action:'workspace'},
    {label:'模型', value:config.model || '未配置', ok:modelReady, action:'model'},
    {label:'API Key', value:keyReady ? '已保存' : '可选 / 未设置', ok:keyReady || modelReady, action:'model'},
    {label:'MCP', value:mcpReady ? '可用' : '未启用', ok:true, action:'tools'},
    {label:'数据', value:dbHealthy ? fmtBytes(dataStatus?.database_size_bytes || 0) : '异常', ok:dbHealthy, action:'data'},
  ];
  const summary = [
    (sessions || []).length + ' 个会话',
    (skills || []).filter(s => s.enabled).length + '/' + (skills || []).length + ' 个技能启用',
    (scheduledTasks || []).length + ' 个自动化任务',
  ].join(' · ');
  return <section className={'workbench-brief ' + (productReady ? 'ready' : 'needs-setup') + (busy ? ' busy' : '')}>
    <div className="brief-main">
      <div className="brief-title"><span className="brief-dot" />{busy ? '正在生成回复' : (productReady ? '工作台已就绪' : '完成配置后即可开始')}</div>
      <div className="brief-subtitle">{busy ? streamStatusText(streamStats, streamStats.started_at ? Math.max(0, Math.round((Date.now() - streamStats.started_at) / 1000)) : 0) : summary}</div>
    </div>
    <div className="brief-checks">{items.map(item => <button key={item.label} type="button" className={'brief-check ' + (item.ok ? 'ok' : 'warn')} onClick={() => openSettings(item.action)}><span>{item.label}</span><b>{item.value}</b></button>)}</div>
  </section>;
}


function MessageActions({ text, onCopy }) {
  return <div className="msg-actions"><button type="button" className="secondary small" onClick={() => onCopy(text)}>复制</button></div>;
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

export function MessageView({ message, onCopy }) {
  if (message.role === 'empty') return <div className="empty">{message.content}</div>;
  if (message.role === 'assistant-stream') return <div className="msg assistant">
    <MessageActions text={[message.reasoning, message.answer].filter(Boolean).join('\n\n')} onCopy={onCopy} />
    {message.reasoning ? <div className="reasoning show"><div className="reasoning-title">思考中</div><Markdown className="reasoning-content markdown" value={message.reasoning} /></div> : null}
    <Markdown className="answer markdown" value={message.answer} />
    {(message.events || []).map((event, i) => <div key={i} className={'tool-event ' + (event.kind === 'run' ? 'run-event-inline' : '')}>{event.text}{event.meta ? <div className="tool-event-meta">{event.meta}</div> : null}</div>)}
  </div>;
  if (message.role === 'assistant') return <div className="msg assistant markdown"><MessageActions text={[message.reasoning, message.content].filter(Boolean).join('\n\n')} onCopy={onCopy} />{message.reasoning ? <details className="reasoning reasoning-collapsed"><summary>思考</summary><Markdown className="reasoning-content markdown" value={message.reasoning} /></details> : null}<Markdown value={message.content} /></div>;
  return <div className={'msg ' + (message.role || 'user')}><MessageActions text={message.content} onCopy={onCopy} />{message.content ? <div>{message.content}</div> : null}<AttachmentList attachments={message.attachments || []} /></div>;
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
        <div className="empty-state-kicker"><span className="kicker-dot" /> ChatDock · Local-first AI Workspace</div>
        <h1>把会话、模型、工具和自动化放进一个工作台</h1>
        <p>为本地优先的 AI 工作流设计：会话不只是聊天窗口，模型配置、MCP 工具、技能、任务记录和数据状态都能在同一个界面里闭环。</p>
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
        <div className="hero-panel-top"><span>今日工作台</span><b>Ready</b></div>
        <div className="hero-metric-row"><div><b>会话</b><span>多工作空间管理</span></div><strong>∞</strong></div>
        <div className="hero-metric-row"><div><b>模型</b><span>OpenAI 兼容配置</span></div><strong>API</strong></div>
        <div className="hero-metric-row"><div><b>工具</b><span>MCP / Skill / 自动化</span></div><strong>Live</strong></div>
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
