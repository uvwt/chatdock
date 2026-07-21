import React from 'react';
import { activeAgentTaskCount, agentTaskProgress, agentTaskStatusMeta, agentTaskStepMeta } from '../lib/agentTasks.js';

export function TaskPanelToggle({ available, open, tasks, onClick }) {
  const activeCount = activeAgentTaskCount(tasks);
  const label = available ? (open ? '关闭全部任务' : '打开全部任务') : 'AgentDock 任务接口尚未配置';
  return <button type="button" className={'secondary task-panel-toggle ' + (open ? 'active' : '')} onClick={onClick} aria-label={label} aria-expanded={open ? 'true' : 'false'} title={label}>
    <svg className="task-panel-toggle-icon" aria-hidden="true" viewBox="0 0 24 24"><path d="M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01" /></svg>
    <span className="task-panel-toggle-label">任务</span>
    {activeCount > 0 ? <span className="task-panel-toggle-count">{activeCount > 99 ? '99+' : activeCount}</span> : null}
  </button>;
}

export function CurrentSessionTask({ error, loading, onRefresh, task, taskID }) {
  const [expanded, setExpanded] = React.useState(false);
  React.useEffect(() => setExpanded(false), [taskID]);
  if (!taskID) return null;

  if (!task && loading) {
    return <section className="current-session-task loading" aria-label="当前会话任务加载中">
      <span className="current-session-task-kicker">当前会话任务</span>
      <span className="current-session-task-loading-bar" />
    </section>;
  }

  if (!task && error) {
    return <section className="current-session-task error" aria-label="当前会话任务读取失败">
      <div><span className="current-session-task-kicker">当前会话任务</span><strong>任务状态暂时不可用</strong></div>
      <button type="button" className="secondary small" onClick={() => onRefresh({ initial: true })}>重试</button>
    </section>;
  }

  if (!task) return null;
  const progress = agentTaskProgress(task);
  const status = agentTaskStatusMeta(task.status);
  const currentStep = task.current_step;
  return <section className={'current-session-task ' + status.tone + (expanded ? ' expanded' : '')} aria-label="当前会话任务">
    <button type="button" className="current-session-task-summary" onClick={() => setExpanded(value => !value)} aria-expanded={expanded ? 'true' : 'false'}>
      <span className="current-session-task-kicker">当前会话任务</span>
      <span className={'current-session-task-status ' + status.tone}><span aria-hidden="true" />{status.label}</span>
      <strong>{task.title}</strong>
      <span className="current-session-task-step">{task.blocker || currentStep?.title || task.summary || '等待下一步'}</span>
      <span className="current-session-task-progress"><span style={{ width: `${progress.percent}%` }} /></span>
      <span className="current-session-task-count">{progress.text}</span>
      <span className="current-session-task-chevron" aria-hidden="true">{expanded ? '⌃' : '⌄'}</span>
    </button>
    {expanded ? <TaskCardDetail task={task} loading={loading} error={error} /> : null}
  </section>;
}

export function TaskPanel({ available, deletingTaskID, detailError, detailLoading, error, expandedTaskID, lastUpdatedAt, loading, onClose, onDelete, onExpand, onRefresh, taskDetail, tasks }) {
  const activeTasks = tasks.filter(task => task.status !== 'completed');
  const completedTasks = tasks.filter(task => task.status === 'completed').slice(0, 6);
  return <section className="agent-task-panel" aria-label="AgentDock 任务面板">
    <header className="agent-task-panel-head">
      <div>
        <div className="agent-task-panel-title-row"><h2>全部任务</h2><span className="agent-task-live"><span aria-hidden="true" />{available ? '实时' : '未配置'}</span></div>
        <p>{available ? (activeTasks.length ? `${activeTasks.length} 个任务正在推进` : '当前没有进行中的任务') : '配置 AgentDock 后可实时查看任务进度'}</p>
      </div>
      <div className="agent-task-panel-actions">
        <button type="button" className="secondary agent-task-refresh" onClick={() => onRefresh({ initial: true })} disabled={!available || loading} aria-label="刷新任务" title="刷新任务">↻</button>
        <button type="button" className="secondary agent-task-close" onClick={onClose} aria-label="关闭任务面板" title="关闭任务面板">×</button>
      </div>
    </header>

    {available && error ? <div className="agent-task-error"><b>任务服务暂时不可用</b><span>{error}</span><button type="button" className="secondary small" onClick={() => onRefresh({ initial: true })}>重试</button></div> : null}

    <div className="agent-task-panel-body">
      {!available ? <div className="agent-task-empty"><span className="agent-task-empty-icon" aria-hidden="true">⚙</span><b>AgentDock 任务接口尚未配置</b><p>设置 CHATDOCK_AGENTDOCK_CONTEXT_URL 并重启服务后，这里会自动开始同步。</p></div> : null}
      {available && loading && !tasks.length ? <TaskPanelLoading /> : null}
      {available && !loading && !tasks.length && !error ? <div className="agent-task-empty"><span className="agent-task-empty-icon" aria-hidden="true">✓</span><b>任务列表为空</b><p>AgentDock 创建多步骤任务后，会在这里实时显示进度。</p></div> : null}
      {activeTasks.length ? <TaskSection title="进行中" count={activeTasks.length}>
        {activeTasks.map(task => <TaskCard key={task.id} task={task} deleting={deletingTaskID === task.id} expanded={expandedTaskID === task.id} detail={taskDetail} detailLoading={detailLoading} detailError={detailError} onDelete={onDelete} onExpand={onExpand} />)}
      </TaskSection> : null}
      {completedTasks.length ? <TaskSection title="最近完成" count={completedTasks.length} muted>
        {completedTasks.map(task => <TaskCard key={task.id} task={task} deleting={deletingTaskID === task.id} expanded={expandedTaskID === task.id} detail={taskDetail} detailLoading={detailLoading} detailError={detailError} onDelete={onDelete} onExpand={onExpand} />)}
      </TaskSection> : null}
    </div>

    <footer className="agent-task-panel-foot">
      <span>{available ? (lastUpdatedAt ? `最近变化 ${formatTaskTime(lastUpdatedAt)}` : '等待首次同步') : '等待 AgentDock 配置'}</span>
      <span>{available ? '面板打开时每 2 秒同步' : '当前不发起轮询'}</span>
    </footer>
  </section>;
}

function TaskSection({ children, count, muted = false, title }) {
  return <section className={'agent-task-section ' + (muted ? 'muted' : '')}>
    <div className="agent-task-section-head"><span>{title}</span><span>{count}</span></div>
    <div className="agent-task-list">{children}</div>
  </section>;
}

function TaskCard({ deleting, detail, detailError, detailLoading, expanded, onDelete, onExpand, task }) {
  const view = expanded && detail?.id === task.id ? detail : task;
  const progress = agentTaskProgress(view);
  const status = agentTaskStatusMeta(view.status);
  const currentStep = view.current_step;
  return <article className={'agent-task-card ' + status.tone + (expanded ? ' expanded' : '')}>
    <button type="button" className="agent-task-card-summary" onClick={() => onExpand(task.id)} aria-expanded={expanded ? 'true' : 'false'}>
      <div className="agent-task-card-top">
        <span className={'agent-task-status ' + status.tone}><span aria-hidden="true" />{status.label}</span>
        <span className="agent-task-progress-text">{progress.text}</span>
      </div>
      <h3>{view.title}</h3>
      {view.blocker ? <p className="agent-task-blocker">{view.blocker}</p> : currentStep ? <p className="agent-task-current"><span>当前</span>{currentStep.title}</p> : view.summary ? <p className="agent-task-summary">{view.summary}</p> : null}
      <div className="agent-task-progress" aria-label={`完成 ${progress.percent}%`}><span style={{ width: `${progress.percent}%` }} /></div>
    </button>
    <div className="agent-task-card-actions">
      <button type="button" className="agent-task-card-action expand" onClick={() => onExpand(task.id)} aria-expanded={expanded ? 'true' : 'false'}>{expanded ? '收起步骤' : '查看步骤'} <span aria-hidden="true">{expanded ? '⌃' : '⌄'}</span></button>
      <button type="button" className="agent-task-card-action delete" onClick={() => onDelete(task)} disabled={deleting} aria-label={`删除任务 ${view.title}`}>{deleting ? '删除中…' : '删除'}</button>
    </div>
    {expanded ? <TaskCardDetail task={view} loading={detailLoading} error={detailError} /> : null}
  </article>;
}

function TaskCardDetail({ error, loading, task }) {
  if (loading && !task.steps?.length) return <div className="agent-task-detail-state">正在读取步骤…</div>;
  if (error) return <div className="agent-task-detail-state error">{error}</div>;
  return <div className="agent-task-detail">
    {task.goal ? <p className="agent-task-goal">{task.goal}</p> : null}
    {task.steps?.length ? <ol className="agent-task-steps">{task.steps.map(step => {
      const meta = agentTaskStepMeta(step.status);
      return <li key={step.id} className={meta.tone}><span className="agent-task-step-symbol" aria-hidden="true">{meta.symbol}</span><span className="agent-task-step-title">{step.title}</span><span className="agent-task-step-status">{meta.label}</span></li>;
    })}</ol> : <div className="agent-task-detail-state">这个任务没有分解步骤。</div>}
    {task.summary ? <div className="agent-task-last-update"><span>最近进展</span><p>{task.summary}</p></div> : null}
  </div>;
}

function TaskPanelLoading() {
  return <div className="agent-task-loading" aria-label="任务加载中"><span /><span /><span /></div>;
}

function formatTaskTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '刚刚';
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(date);
}
