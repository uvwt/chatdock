import React from 'react';
import {
  ArrowDown,
  ArrowUp,
  Folder,
  ListTodo,
  Menu,
  MessageSquare,
  MessageSquarePlus,
  Moon,
  MoreHorizontal,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Search,
  Settings2,
  Square,
  Sun,
} from './icons.js';
import { AttachmentList } from './chat.jsx';
import { ComposerModelPicker } from './modelPicker.jsx';
import { TaskPanelToggle } from './taskPanel.jsx';
import { Menu as UiMenu, MenuItem, MenuPopup, MenuPortal, MenuPositioner, MenuTrigger } from '../shared/ui/menu.jsx';
import { Tooltip } from '../shared/ui/tooltip.jsx';
import { fetchSessions } from '../lib/sessionApi.js';
import { fetchScheduledTaskRuns } from '../lib/settingsApi.js';
import { scheduledTaskSessionRows, sessionRowID, unpinnedSessionRows } from '../lib/sessionPresentation.js';

const iconProps = { size: 17, 'aria-hidden': true };

export function Topbar({ currentTitle, newSession, openSettings, selectedProject, setQuickPaletteOpen, setSidebarCollapsed, setThemeState, sidebarCollapsed, taskPanelAvailable, taskPanelOpen, taskPanelTasks, theme, toggleTaskPanel }) {
  const darkMode = theme !== 'day';
  const ThemeIcon = darkMode ? Sun : Moon;
  return <div className={'topbar' + (selectedProject ? ' project-context-active' : '')}>
    <div className="top-left">
      <button className="mobile-menu icon-button" onClick={() => setSidebarCollapsed(current => !current)} aria-label="打开会话列表"><Menu {...iconProps} /></button>
      <div className="topbar-title-wrap">
        {selectedProject ? <span className="topbar-project-context"><Folder size={12} aria-hidden="true" /><span>{selectedProject.name}</span></span> : null}
        <b id="title">{currentTitle}</b>
      </div>
    </div>
    <div className="top-actions">
      <button className="secondary quick-palette-toggle" onClick={() => setQuickPaletteOpen(true)}><MoreHorizontal {...iconProps} /><span className="action-label">快捷</span></button>
      <button className="secondary config-toggle" onClick={() => openSettings()}><Settings2 {...iconProps} /><span className="action-label">配置</span></button>
      <TaskPanelToggle available={taskPanelAvailable} open={taskPanelOpen} tasks={taskPanelTasks} onClick={toggleTaskPanel} />
      <Tooltip content="新会话">
        <button className="secondary session-actions-toggle mobile-new-toggle icon-button" onClick={newSession} aria-label="新会话"><MessageSquarePlus {...iconProps} /></button>
      </Tooltip>
      <button className="theme-toggle" onClick={() => setThemeState(darkMode ? 'day' : 'night')} aria-label={darkMode ? '切换到浅色模式' : '切换到深色模式'}>
        <ThemeIcon {...iconProps} />
        <span className="action-label">{darkMode ? '浅色' : '深色'}</span>
      </button>
    </div>
  </div>;
}

export function ComposerBar({ busy, createPersistedSession, current, downloadAttachment, fileInputRef, guideActiveJob, handleFileSelect, input, inputRef, inputStats, modelPickerOpen, modelReady, openSettings, pendingAttachmentIDs, pendingAttachments, providerChoices, removePendingAttachment, selectChatModel, selectedChatModel, selectedModelProvider, sendMsg, setInput, setModelPickerOpen, showSelectedChatModel, stopStreaming, uploadingFiles }) {
  const modelPicker = <ComposerModelPicker busy={busy} providers={providerChoices} selectedProvider={selectedModelProvider} selectedModel={selectedChatModel} showSelection={showSelectedChatModel} open={modelPickerOpen} setOpen={setModelPickerOpen} selectModel={selectChatModel} openSettings={openSettings} />;
  const messageInput = <textarea ref={inputRef} id="input" value={input} onChange={e => setInput(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); busy ? guideActiveJob() : sendMsg(); } }} placeholder={busy ? '输入引导内容' : '输入消息'} />;

  return <div className="composer-shell">
    {pendingAttachments.length ? <AttachmentList attachments={pendingAttachments} removable={!busy} onRemove={removePendingAttachment} onDownload={downloadAttachment} /> : null}
    <div className={'composer' + (busy ? ' composer-streaming' : '') + (modelPickerOpen ? ' composer-model-picker-open' : '')}>
      <input ref={fileInputRef} type="file" multiple className="file-input" onChange={event => handleFileSelect(event, { current, createPersistedSession })} />
      <button className="secondary attach-control icon-button" disabled={busy || uploadingFiles} onClick={() => fileInputRef.current?.click()} aria-label="添加附件"><Plus size={20} aria-hidden="true" /></button>
      {messageInput}
      {busy ? <div className="composer-stream-actions">
        {modelPicker}
        <button className="secondary stream-control guide-control" onClick={guideActiveJob} disabled={!input.trim()} aria-label="追加引导"><span>引导</span></button>
        <button className="danger stream-control stop-control" onClick={stopStreaming} aria-label="中止生成"><Square className="stop-icon" size={12} fill="currentColor" stroke="none" aria-hidden="true" /><span>中止</span></button>
      </div> : <>
        {modelPicker}
        <button id="send" className="icon-button" disabled={uploadingFiles || (!input.trim() && !pendingAttachmentIDs.length) || !modelReady} onClick={() => sendMsg()} aria-label={!modelReady ? '请先配置模型' : '发送'}><ArrowUp size={19} aria-hidden="true" /></button>
      </>}
    </div>
    {inputStats ? <div className="composer-meta">{inputStats}</div> : null}
  </div>;
}

function SessionActionMenu({ activeMenuKey, busy, current, deleteSessionByID, iconSize = 16, menuKey, onDelete, pinSessionByID, renameSessionByID, session, sessionMenuID, setActiveMenuKey, setSessionMenuID, triggerClassName, triggerLabel }) {
  const open = sessionMenuID === session.id && activeMenuKey === menuKey;
  const deletingCurrentStream = !!(busy && session.id === current);

  const closeMenu = () => {
    setActiveMenuKey('');
    setSessionMenuID('');
  };
  const runAction = action => {
    closeMenu();
    action();
  };

  return <UiMenu modal={false} open={open} onOpenChange={nextOpen => {
    if (nextOpen) {
      setActiveMenuKey(menuKey);
      setSessionMenuID(session.id);
      return;
    }
    if (open) closeMenu();
  }}>
    <MenuTrigger className={triggerClassName} onClick={event => event.stopPropagation()} aria-label={triggerLabel || (session.title || '会话') + ' 操作'}>
      <MoreHorizontal size={iconSize} aria-hidden="true" />
    </MenuTrigger>
    <MenuPortal>
      <MenuPositioner className="z-[3000]" side="bottom" align="end" sideOffset={6} collisionPadding={8} positionMethod="fixed">
        <MenuPopup className="session-row-menu-portal">
          <MenuItem onClick={() => runAction(() => pinSessionByID(session.id, !!session.pinned))}>{session.pinned ? '取消置顶' : '置顶'}</MenuItem>
          <MenuItem danger onClick={() => runAction(() => deleteSessionByID(session.id, session.title).then(() => onDelete?.()))} disabled={deletingCurrentStream} title={deletingCurrentStream ? '生成结束或中断后才能删除当前会话' : undefined}>删除</MenuItem>
          <MenuItem onClick={() => runAction(() => renameSessionByID(session.id, session.title))}>重命名标题</MenuItem>
        </MenuPopup>
      </MenuPositioner>
    </MenuPortal>
  </UiMenu>;
}

function SidebarTreeNode({ api, current, item, kind, openSession, pinnedSessions, renderSessionMenu, startProjectConversation }) {
  const [rows, setRows] = React.useState();
  const Icon = kind === 'project' ? Folder : ListTodo;
  const visibleRows = Array.isArray(rows) ? unpinnedSessionRows(rows, pinnedSessions) : rows;
  const removeRow = id => setRows(currentRows => Array.isArray(currentRows) ? currentRows.filter(row => sessionRowID(row) !== id) : currentRows);
  const renderRow = row => {
    const id = sessionRowID(row);
    const title = row.title || row.session_title || row.task_title || '新会话';
    const summary = { id, title, pinned: !!row.pinned };
    return <div key={id} className="sidebar-tree-session-row">
      <button className={'sidebar-tree-session ' + (current === id ? 'active' : '')} onClick={async () => { if (await openSession(id) === false) removeRow(id); }}>{title}</button>
      {renderSessionMenu(summary, {
        iconSize: 15,
        menuKey: `tree:${kind}:${item.id}:${id}`,
        onDelete: () => removeRow(id),
        triggerClassName: 'sidebar-tree-session-menu icon-button',
        triggerLabel: '操作',
      })}
    </div>;
  };
  const loadRows = async event => {
    if (!event.currentTarget.open || rows === null || Array.isArray(rows)) return;
    setRows(null);
    try {
      const data = kind === 'project' ? await fetchSessions(api, {projectFilter: item.id, limit: 30, pinned: false}) : await fetchScheduledTaskRuns(api, item.id);
      setRows(kind === 'project' ? data.sessions || [] : scheduledTaskSessionRows(data.runs || []));
    } catch {
      setRows(false);
    }
  };
  return <details name="sidebar-tree" className="sidebar-tree-node" onToggle={loadRows}>
    <summary><Icon size={15} aria-hidden="true" /><span>{item.name || item.title}</span><ArrowDown size={14} aria-hidden="true" /></summary>
    <div className="sidebar-tree-children">
      {kind === 'project' ? <button type="button" className="sidebar-tree-new-chat" onClick={() => startProjectConversation(item.id)}><MessageSquarePlus size={14} aria-hidden="true" /><span>新建对话</span></button> : null}
      {visibleRows == null ? <span className="sidebar-tree-state">正在加载…</span> : visibleRows === false ? <span className="sidebar-tree-state">加载失败</span> : visibleRows.length ? <>
        {visibleRows.slice(0, 5).map(renderRow)}
        {visibleRows.length > 5 ? <details className="sidebar-tree-more"><summary>显示更多</summary>{visibleRows.slice(5).map(renderRow)}</details> : null}
      </> : <span className="sidebar-tree-state">暂无会话</span>}
    </div>
  </details>;
}

// 侧栏分段首屏占位。三段的真实条目数只有请求回来才知道，纯靠猜行数一定对不上，
// 所以把上一次的行数缓存到 localStorage，首屏按它预留等高空间，数据到达时高度不变。
const SECTION_ROWS_KEY = 'chatdock.sidebar.sectionRows.v1';

function readSectionRows() {
  try {
    const raw = JSON.parse(localStorage.getItem(SECTION_ROWS_KEY) || '{}');
    return raw && typeof raw === 'object' ? raw : {};
  } catch {
    return {};
  }
}

function writeSectionRows(next) {
  try {
    localStorage.setItem(SECTION_ROWS_KEY, JSON.stringify(next));
  } catch {
    // localStorage 不可用时只是回退到默认行数，不影响功能。
  }
}

// 占位高度必须和真实布局算法一致：每行内容高 40px（--ui-control，.session 与 tree summary 共用），
// 行与行之间是 .sidebar-*-list 的 2px grid gap。按 rows*42 算会每段多出 2px，实测三段累计 6px 位移。
const SECTION_ROW_HEIGHT = 40;
const SECTION_ROW_GAP = 2;

function SectionPlaceholder({ rows, label }) {
  const safeRows = Math.max(0, Math.min(12, Number(rows) || 0));
  if (!safeRows) return null;
  const height = safeRows * SECTION_ROW_HEIGHT + (safeRows - 1) * SECTION_ROW_GAP;
  return <div
    className="sidebar-section-placeholder"
    style={{ height: `${height}px` }}
    role="status"
    aria-label={label}
    aria-busy="true"
  />;
}

export function Sidebar({ api, busy, current, deleteSessionByID, filteredSessions, goHome, hasMoreSessions, loadingMoreSessions, sessionsLoaded = true, newSession, onLoadMoreSessions, openSession, openManagementPage, pinSessionByID, pinnedSessions = [], pinnedProjects = [], pinnedTasks = [], pinnedLoaded = true, projects, projectsLoaded = true, projectFilter, renameSessionByID, scheduledTasks, scheduledTasksLoaded = true, sessionMenuID, sessionSearch, sessionSearchBusy, setSessionMenuID, setSessionSearch, setSidebarCollapsed, setTaskSearch, sidebarCollapsed, startProjectConversation }) {
  const [activeMenuKey, setActiveMenuKey] = React.useState('');
  const sessionsRef = React.useRef(null);
  const loadMoreRef = React.useRef(null);
  // 首屏用上一次记录的行数预留高度；本次数据到齐后再写回，供下次首屏使用。
  const cachedRowsRef = React.useRef(null);
  if (cachedRowsRef.current === null) cachedRowsRef.current = readSectionRows();
  const cachedRows = cachedRowsRef.current;

  // sessionMenuID 仍由 App 持有，activeMenuKey 只区分同一会话在树节点和全部会话中的不同触发器。
  // 外部路由或筛选切换清空 sessionMenuID 时，同步丢弃本地触发器标识，避免残留不可见状态。
  React.useEffect(() => {
    if (!sessionMenuID && activeMenuKey) setActiveMenuKey('');
  }, [activeMenuKey, sessionMenuID]);

  React.useEffect(() => {
    if (sessionsRef.current) sessionsRef.current.scrollTop = 0;
  }, [projectFilter, sessionSearch]);

  React.useEffect(() => {
    if (!hasMoreSessions || loadingMoreSessions || !onLoadMoreSessions || !window.IntersectionObserver) return undefined;
    const target = loadMoreRef.current;
    const root = sessionsRef.current;
    if (!target || !root) return undefined;
    const observer = new IntersectionObserver(entries => {
      if (entries.some(entry => entry.isIntersecting)) onLoadMoreSessions();
    }, { root, rootMargin: '120px 0px' });
    observer.observe(target);
    return () => observer.disconnect();
  }, [filteredSessions.length, hasMoreSessions, loadingMoreSessions, onLoadMoreSessions]);

  const handleSessionScroll = event => {
    if (!hasMoreSessions || loadingMoreSessions || !onLoadMoreSessions) return;
    const list = event.currentTarget;
    if (list.scrollHeight - list.scrollTop - list.clientHeight <= 120) onLoadMoreSessions();
  };

  const renderSessionMenu = (session, { iconSize = 16, menuKey, onDelete, triggerClassName = 'session-menu-trigger icon-button', triggerLabel } = {}) => <SessionActionMenu
    activeMenuKey={activeMenuKey}
    busy={busy}
    current={current}
    deleteSessionByID={deleteSessionByID}
    iconSize={iconSize}
    menuKey={menuKey || `session:${session.id}`}
    onDelete={onDelete}
    pinSessionByID={pinSessionByID}
    renameSessionByID={renameSessionByID}
    session={session}
    sessionMenuID={sessionMenuID}
    setActiveMenuKey={setActiveMenuKey}
    setSessionMenuID={setSessionMenuID}
    triggerClassName={triggerClassName}
    triggerLabel={triggerLabel}
  />;

  const searchingSessions = !!sessionSearch.trim();
  const pinnedSessionRows = searchingSessions ? [] : pinnedSessions;
  const pinnedProjectRows = searchingSessions ? [] : pinnedProjects;
  const pinnedTaskRows = searchingSessions ? [] : pinnedTasks;
  // 置顶区在完全没有置顶项时整段不渲染，但必须先确认 pinned feed 已到达，
  // 否则整段会从“不渲染”跳到“渲染”，同样把下方内容推走。
  const hasPinnedRows = pinnedSessionRows.length > 0 || pinnedProjectRows.length > 0 || pinnedTaskRows.length > 0;
  const pinnedPlaceholderRows = pinnedLoaded ? 0 : (cachedRows.pinned || 0);
  const showPinnedSection = !searchingSessions && (pinnedLoaded ? hasPinnedRows : pinnedPlaceholderRows > 0);
  // 列表为空时要先分清是还在取数据还是真的没有会话：搜索态看 sessionSearchBusy，
  // 普通列表看 sessionsLoaded，两者都未就绪时展示骨架而不是“暂无会话”。
  const pendingSessionRows = searchingSessions ? sessionSearchBusy : !sessionsLoaded;
  const pinnedProjectIDs = new Set(pinnedProjects.map(item => item.id));
  const pinnedTaskIDs = new Set(pinnedTasks.map(item => item.id));
  const renderTreeNode = (kind, item) => <SidebarTreeNode key={kind + item.id} api={api} current={current} item={item} kind={kind} openSession={openSession} pinnedSessions={pinnedSessions} renderSessionMenu={renderSessionMenu} startProjectConversation={startProjectConversation} />;
  const managementProjects = (projects || []).filter(item => !item.pinned && !pinnedProjectIDs.has(item.id));
  const managementTasks = (scheduledTasks || []).filter(item => !item.pinned && !pinnedTaskIDs.has(item.id));
  const sessionRows = filteredSessions;
  const pinnedRowCount = pinnedSessions.length + pinnedProjects.length + pinnedTasks.length;
  const projectRowCount = managementProjects.length;
  const taskRowCount = managementTasks.length;
  // 三段都到齐后记录真实行数，供下次首屏预留同样高度。搜索态的结果数不代表常规列表长度，不记录。
  React.useEffect(() => {
    if (!pinnedLoaded || !projectsLoaded || !scheduledTasksLoaded || !sessionsLoaded) return;
    if (sessionSearch.trim()) return;
    writeSectionRows({
      pinned: pinnedRowCount,
      projects: projectRowCount,
      tasks: taskRowCount,
      sessions: Math.min(12, filteredSessions.length),
    });
  }, [pinnedLoaded, projectsLoaded, scheduledTasksLoaded, sessionsLoaded, sessionSearch, pinnedRowCount, projectRowCount, taskRowCount, filteredSessions.length]);
  const renderSession = session => {
    const isActive = current === session.id;
    const menuKey = `session:${session.id}`;
    const menuOpen = sessionMenuID === session.id && activeMenuKey === menuKey;
    return <div key={session.id} data-session-id={session.id} className={'session ' + (isActive ? 'active ' : '') + (session.pinned ? 'pinned ' : '') + (menuOpen ? 'menu-open' : '')} onClick={() => openSession(session.id, session)}>
      <div className="session-main">
        {session.pinned ? <MessageSquare className="session-kind-icon" size={15} aria-hidden="true" /> : null}
        <div className="session-title">{session.title}</div>
      </div>
      {renderSessionMenu(session, { menuKey })}
    </div>;
  };

  return <>
    <aside>
      <div className="sidebar-head">
        <a className="brand" href="/" aria-label="返回 ChatDock 首页" onClick={event => { event.preventDefault(); goHome(); }}>
          <div className="brand-copy"><span className="brand-text">ChatDock</span><div className="sub">Local AI</div></div>
        </a>
        <Tooltip content={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}>
          <button id="sidebarToggle" className="sidebar-toggle icon-button" onClick={() => setSidebarCollapsed(current => !current)} aria-label={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}>{sidebarCollapsed ? <PanelLeftOpen {...iconProps} /> : <PanelLeftClose {...iconProps} />}</button>
        </Tooltip>
      </div>
      <div className="session-search-row">
        <label className="session-search-box"><Search size={15} aria-hidden="true" /><input className="session-search" placeholder="搜索聊天记录" value={sessionSearch} onChange={event => setSessionSearch(event.target.value)} /></label>
        <Tooltip content="新会话">
          <button className="new icon-button" onClick={newSession} aria-label="新会话"><MessageSquarePlus {...iconProps} /></button>
        </Tooltip>
      </div>
      <div id="sessions" ref={sessionsRef} onScroll={handleSessionScroll}>
        {showPinnedSection ? <>
          <div className="sidebar-section-head"><div className="sidebar-section-title sidebar-section-title-emphasis">置顶</div></div>
          <div className="sidebar-pinned-list">{pinnedLoaded
            ? <>{pinnedSessionRows.map(renderSession)}{pinnedProjectRows.map(item => renderTreeNode('project', item))}{pinnedTaskRows.map(item => renderTreeNode('task', item))}</>
            : <SectionPlaceholder rows={pinnedPlaceholderRows} label="正在加载置顶" />}</div>
        </> : null}
        {!searchingSessions ? <>
          <div className="sidebar-section-head"><button className="sidebar-section-title sidebar-section-title-emphasis" onClick={() => openManagementPage('projects')}>项目</button></div>
          <div className="sidebar-manage-list">{projectsLoaded
            ? managementProjects.map(item => renderTreeNode('project', item))
            : <SectionPlaceholder rows={cachedRows.projects || 0} label="正在加载项目" />}</div>
          <div className="sidebar-section-head"><button className="sidebar-section-title" onClick={() => { setTaskSearch(''); openManagementPage('automation'); }}>定时任务</button></div>
          <div className="sidebar-manage-list">{scheduledTasksLoaded
            ? managementTasks.map(item => renderTreeNode('task', item))
            : <SectionPlaceholder rows={cachedRows.tasks || 0} label="正在加载定时任务" />}</div>
        </> : null}
        <div className="sidebar-section-head"><div className="sidebar-section-title sidebar-section-title-emphasis">全部会话</div></div>
        {searchingSessions ? <div className="session-search-meta">{sessionSearchBusy ? '搜索中…' : (hasMoreSessions ? '全文搜索 · 已加载 ' : '全文搜索 ') + filteredSessions.length + ' 条'}</div> : null}
        {sessionRows.length ? sessionRows.map(renderSession) : (pendingSessionRows
          ? <SectionPlaceholder rows={cachedRows.sessions || 6} label="正在加载会话列表" />
          : <div className="empty compact">{searchingSessions ? '没有匹配会话' : '暂无会话，开始新会话'}</div>)}
        <div ref={loadMoreRef} style={{ minHeight: 1 }} aria-hidden="true" />
        {loadingMoreSessions ? <div className="session-search-meta" role="status">正在加载更多…</div> : null}
      </div>
    </aside>
  </>;
}
