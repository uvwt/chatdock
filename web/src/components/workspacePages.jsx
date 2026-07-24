import React, { useRef, useState } from 'react';
import { ArrowLeft, MoreHorizontal, Plus, RefreshCw } from './icons.js';
import { fmtTime, runStatusClass, runStatusLabel, scheduleSummary, taskStatusClass, taskStatusLabel } from '../lib/appUtils.js';
import { fetchScheduledTaskRuns } from '../lib/settingsApi.js';
import { scheduledTaskSessionRows } from '../lib/sessionPresentation.js';
import '../styles/workspace-pages.css';
import '../styles/workspace-pages-mobile.css';

function PageHeader({ eyebrow, title, description, onClose, actions }) {
  return <header className="workspace-page-header">
    <button type="button" className="workspace-page-back icon-button" onClick={onClose} aria-label="返回聊天" title="返回聊天"><ArrowLeft size={18} aria-hidden="true" /></button>
    <div className="workspace-page-heading"><span>{eyebrow}</span><h1>{title}</h1><p>{description}</p></div>
    <div className="workspace-page-actions">{actions}</div>
  </header>;
}

function SummaryCard({ label, value, onClick }) {
  const content = <><span>{label}</span><b>{value}</b></>;
  return onClick ? <button type="button" className="workspace-summary-card actionable" onClick={onClick}>{content}</button> : <div className="workspace-summary-card">{content}</div>;
}

function closeDetailsAndRun(event, action) {
  event.currentTarget.closest('details')?.removeAttribute('open');
  action();
}

async function togglePinned(api, type, item, apply, showToast) {
  try {
    const data = await api('/api/' + type + '/' + encodeURIComponent(item.id) + '/pin', {method: 'POST', body: JSON.stringify({pinned: !item.pinned})});
    await apply(data);
  } catch (error) {
    showToast('置顶操作失败：' + error.message, 'error');
  }
}

export function WorkspacePage(props) {
  if (props.page === 'scheduled-tasks') return <ScheduledTasksPage {...props} />;
  return <ProjectsPage {...props} />;
}

function ProjectsPage({ api, closeWorkspacePage, deleteProject, editProject, loadProjects, openProjectSessions, projectPromptPreview, projects, projectSessionCounts, showProjectPromptPreview, showToast }) {
  const pinProject = project => togglePinned(api, 'projects', project, loadProjects, showToast);
  return <section className="workspace-page projects-page">
    <PageHeader
      eyebrow="Workspace / Projects"
      title="项目"
      description="集中管理上下文、提示词和会话归属。"
      onClose={closeWorkspacePage}
      actions={<button type="button" onClick={() => editProject()}><Plus size={16} aria-hidden="true" /><span>新增项目</span></button>}
    />
    <div className="workspace-page-body">
      <div className="workspace-summary-grid">
        <SummaryCard label="项目" value={projects.length} />
        <SummaryCard label="全部会话" value={projectSessionCounts?.all ?? 0} onClick={() => openProjectSessions('all')} />
        <SummaryCard label="普通会话" value={projectSessionCounts?.plain ?? 0} onClick={() => openProjectSessions('plain')} />
      </div>
      <div className="workspace-section-head"><div><span>项目列表</span><p>选择项目进入对应会话，或在这里维护项目提示词。</p></div></div>
      <div className="workspace-card-grid">
        {projects.length ? projects.map(project => <article key={project.id} className={'workspace-card project-workspace-card ' + (project.pinned ? 'pinned' : '')}>
          <header><div><span>Project</span><h2>{project.name}</h2></div><div className="project-card-actions"><em>{projectSessionCounts?.byProject?.[project.id] || 0} 会话</em><details className="workspace-more-menu"><summary aria-label="更多项目操作" title="更多操作"><MoreHorizontal size={17} aria-hidden="true" /></summary><div>
            <button type="button" onClick={event => closeDetailsAndRun(event, () => pinProject(project))}>{project.pinned ? '取消置顶' : '置顶项目'}</button>
            <button type="button" onClick={event => closeDetailsAndRun(event, () => editProject(project))}>编辑项目</button>
            <button type="button" onClick={event => closeDetailsAndRun(event, () => showProjectPromptPreview(project.id))}>预览 Prompt</button>
            <button type="button" className="danger" onClick={event => closeDetailsAndRun(event, () => deleteProject(project))}>删除项目</button>
          </div></details></div></header>
          <p>{project.prompt || '未设置项目提示词'}</p>
          <div className="workspace-card-meta">ID：{project.id}</div>
          <footer>
            <button type="button" onClick={() => openProjectSessions(project.id)}>查看会话</button>
          </footer>
        </article>) : <div className="workspace-empty"><b>还没有项目</b><span>普通会话不需要项目；需要固定上下文时再创建。</span></div>}
      </div>
      {projectPromptPreview ? <section className="workspace-preview"><div><span>Prompt Preview</span><b>最终 Prompt 预览</b></div><pre>{projectPromptPreview}</pre></section> : null}
    </div>
  </section>;
}

function scheduledTaskContextLabel(mode) {
  return ({stateless: '每次独立执行', last_result: '带上次结果', session: '连续会话'})[mode] || '每次独立执行';
}

function ScheduledTaskCard({ task, deleteScheduledTask, editScheduledTask, openScheduledTaskSession, openTaskSessions, pinScheduledTask, runScheduledTaskNow, toggleScheduledTask }) {
  const prompt = (task.prompt || '').trim().slice(0, 180) || '无提示内容';
  return <article className={'workspace-card scheduled-task-workspace-card ' + (task.pinned ? 'pinned' : '')}>
    <header>
      <div><span>Scheduled Task</span><h2>{task.title || '未命名任务'}</h2></div>
      <div className="scheduled-task-card-actions">
        <em className={'badge ' + taskStatusClass(task)}>{taskStatusLabel(task)}</em>
        <details className="workspace-more-menu"><summary aria-label="更多任务操作" title="更多操作"><MoreHorizontal size={17} aria-hidden="true" /></summary><div>
          <button type="button" onClick={event => closeDetailsAndRun(event, () => pinScheduledTask(task))}>{task.pinned ? '取消置顶' : '置顶任务'}</button>
          {task.session_id ? <button type="button" onClick={event => closeDetailsAndRun(event, () => openScheduledTaskSession(task.session_id))}>打开最近会话</button> : null}
          <button type="button" onClick={event => closeDetailsAndRun(event, () => editScheduledTask(task.id))}>编辑任务</button>
          <button type="button" className="danger" onClick={event => closeDetailsAndRun(event, () => deleteScheduledTask(task.id))}>删除任务</button>
        </div></details>
      </div>
    </header>
    <p>{prompt}</p>
    {task.running ? <div className="workspace-inline-note">任务正在运行，配置修改会从下次执行生效。</div> : null}
    {task.last_error ? <div className="workspace-inline-note error">上次错误：{task.last_error}</div> : null}
    <div className="workspace-card-facts">
      <div className="workspace-card-meta">{scheduleSummary(task)}</div>
      <div className="workspace-card-meta">上下文：{scheduledTaskContextLabel(task.context_mode || 'stateless')}</div>
    </div>
    <footer>
      <label className="workspace-toggle"><input type="checkbox" checked={!!task.enabled} onChange={event => toggleScheduledTask(task.id, event.target.checked)} /><span>启用</span></label>
      <button type="button" className="secondary" disabled={task.running} onClick={() => runScheduledTaskNow(task.id)}>立即运行</button>
      <button type="button" className="secondary" onClick={() => openTaskSessions(task)}>会话记录</button>
    </footer>
  </article>;
}

function ScheduledSessionList({ onBack, onOpen, rows, task }) {
  return <section className="scheduled-session-view">
    <div className="workspace-list-toolbar scheduled-session-toolbar">
      <button type="button" className="secondary" onClick={onBack}>返回任务</button>
      <div><span>{task.title} · 会话记录</span><p>{rows === null ? '正在读取会话' : rows === false ? '读取失败' : rows.length + ' 个运行会话'}</p></div>
    </div>
    {rows === null ? <div className="workspace-empty"><b>正在读取会话</b></div> : rows === false ? <div className="workspace-empty"><b>会话读取失败</b></div> : rows.length ? <div className="scheduled-session-list">
      {rows.map(row => <button type="button" key={row.session_id} className="scheduled-session-row" onClick={() => onOpen(row.session_id)}>
        <span className="scheduled-session-copy"><b>{row.session_title || row.task_title}</b><small>{fmtTime(row.started_at)} · {row.manual ? '手动运行' : '自动运行'}</small></span>
        <em className={'badge ' + runStatusClass(row.status)}>{runStatusLabel(row.status)}</em>
      </button>)}
    </div> : <div className="workspace-empty"><b>还没有运行会话</b></div>}
  </section>;
}

function ScheduledTasksPage({ api, closeWorkspacePage, deleteScheduledTask, editScheduledTask, loadScheduledTasks, openScheduledTaskSession, runScheduledTaskNow, scheduledTasks, setScheduledTasks, setTaskSearch, showToast, taskSearch, toggleScheduledTask }) {
  const [sessionTask, setSessionTask] = useState(null);
  const [sessionRuns, setSessionRuns] = useState([]);
  const requestRef = useRef(0);
  const pinScheduledTask = task => togglePinned(api, 'scheduled-tasks', task, data => setScheduledTasks(data.tasks || []), showToast);
  const query = taskSearch.trim().toLowerCase();
  const filteredTasks = query ? scheduledTasks.filter(task => [task.title, task.prompt, task.last_status, task.last_error].some(value => String(value || '').toLowerCase().includes(query))) : scheduledTasks;
  const loadTaskSessions = async task => {
    const request = ++requestRef.current;
    setSessionTask(task);
    setSessionRuns(null);
    try {
      const data = await fetchScheduledTaskRuns(api, task.id);
      if (request === requestRef.current) setSessionRuns(scheduledTaskSessionRows(data.runs || []));
    } catch {
      if (request === requestRef.current) setSessionRuns(false);
    }
  };
  const enabled = scheduledTasks.filter(task => task.enabled).length;
  const running = scheduledTasks.filter(task => task.running).length;
  const failed = scheduledTasks.filter(task => task.last_status === 'failed').length;
  return <section className="workspace-page scheduled-tasks-page">
    <PageHeader
      eyebrow="Workspace / Scheduled Tasks"
      title="定时任务"
      description="创建、运行和追踪自动执行任务。"
      onClose={closeWorkspacePage}
      actions={<><button type="button" className="secondary icon-button workspace-refresh" onClick={() => loadScheduledTasks()} aria-label="刷新任务" title="刷新任务"><RefreshCw size={17} aria-hidden="true" /></button><button type="button" onClick={() => editScheduledTask()}><Plus size={16} aria-hidden="true" /><span>新增任务</span></button></>}
    />
    <div className="workspace-page-body">
      <div className="workspace-summary-grid task-summary-grid">
        <SummaryCard label="全部任务" value={scheduledTasks.length} />
        <SummaryCard label="已启用" value={enabled} />
        <SummaryCard label="运行中" value={running} />
        <SummaryCard label="失败" value={failed} />
      </div>
      {sessionTask ? <ScheduledSessionList
        task={sessionTask} rows={sessionRuns} onBack={() => { requestRef.current++; setSessionTask(null); }}
        onOpen={openScheduledTaskSession}
      /> : <>
        <div className="workspace-list-toolbar"><div><span>任务列表</span><p>搜索标题、提示词或运行状态。</p></div><input className="workspace-search" placeholder="搜索定时任务" value={taskSearch} onChange={event => setTaskSearch(event.target.value)} /></div>
        <div className="workspace-card-grid scheduled-task-grid">
          {filteredTasks.length ? filteredTasks.map(task => <ScheduledTaskCard key={task.id} task={task} deleteScheduledTask={deleteScheduledTask} editScheduledTask={editScheduledTask} openScheduledTaskSession={openScheduledTaskSession} openTaskSessions={loadTaskSessions} pinScheduledTask={pinScheduledTask} runScheduledTaskNow={runScheduledTaskNow} toggleScheduledTask={toggleScheduledTask} />) : <div className="workspace-empty"><b>{taskSearch.trim() ? '没有匹配任务' : '还没有定时任务'}</b><span>{taskSearch.trim() ? '换个关键词再试。' : '创建任务后可按一次、间隔或日历计划自动运行。'}</span></div>}
        </div>
      </>}
    </div>
  </section>;
}
