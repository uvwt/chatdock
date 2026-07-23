import React from 'react';
import { createPortal } from 'react-dom';
import { AttachmentList } from './chat.jsx';
import { ComposerModelPicker } from './modelPicker.jsx';
import { TaskPanelToggle } from './taskPanel.jsx';
import { fmtTime } from '../lib/appUtils.js';

export function Topbar({ busy, cloneCurrent, copyCurrentMarkdown, current, currentPinned, currentTitle, deleteCurrent, exportCurrent, newSession, openSettings, pinCurrent, renameCurrent, setQuickPaletteOpen, setSidebarCollapsed, setThemeState, showContextPreview, sidebarCollapsed, taskPanelAvailable, taskPanelOpen, taskPanelTasks, theme, toggleTaskPanel }) {
  return <div className="topbar">
    <div className="top-left"><button className="mobile-menu" onClick={() => setSidebarCollapsed(!sidebarCollapsed)} aria-label="打开会话列表" title="会话列表"><svg className="topbar-icon" aria-hidden="true" viewBox="0 0 24 24"><path d="M5 7h14M5 12h14M5 17h14" /></svg></button><b id="title">{currentTitle}</b></div>
    <div className="top-actions">
      <button className="secondary quick-palette-toggle" onClick={() => setQuickPaletteOpen(true)} title="快捷指令（⌘/Ctrl K）"><svg className="topbar-icon" aria-hidden="true" viewBox="0 0 24 24"><path d="m13 2-1.8 6.2L5 10l6.2 1.8L13 18l1.8-6.2L21 10l-6.2-1.8Z" /><path d="m5 3-.7 2.3L2 6l2.3.7L5 9l.7-2.3L8 6l-2.3-.7Z" /></svg><span className="action-label">快捷</span></button>
      <button className="secondary config-toggle" onClick={() => openSettings()} title="配置中心"><svg className="topbar-icon" aria-hidden="true" viewBox="0 0 24 24"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.83 2.83-.06-.06A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 .6 1.7 1.7 0 0 0-.4 1.1V21h-4v-.09A1.7 1.7 0 0 0 8.6 19.4a1.7 1.7 0 0 0-1.88.34l-.06.06-2.83-2.83.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-.6-1 1.7 1.7 0 0 0-1.1-.4H3v-4h.09A1.7 1.7 0 0 0 4.6 8.6a1.7 1.7 0 0 0-.34-1.88l-.06-.06 2.83-2.83.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-.6 1.7 1.7 0 0 0 .4-1.1V3h4v.09A1.7 1.7 0 0 0 15.4 4.6a1.7 1.7 0 0 0 1.88-.34l.06-.06 2.83 2.83-.06.06A1.7 1.7 0 0 0 19.4 9c.13.37.35.7.64.96.3.27.69.42 1.1.44H21v4h-.09A1.7 1.7 0 0 0 19.4 15Z" /></svg><span className="action-label">配置</span></button>
      <TaskPanelToggle available={taskPanelAvailable} open={taskPanelOpen} tasks={taskPanelTasks} onClick={toggleTaskPanel} />
      <button className="secondary session-actions-toggle mobile-new-toggle" onClick={newSession} aria-label="新会话" title="新会话"><svg className="mobile-new-icon" aria-hidden="true" viewBox="0 0 24 24"><path d="M12 5v14M5 12h14" /></svg></button>
      <button className="theme-toggle" onClick={() => setThemeState(theme === 'day' ? 'night' : 'day')}><span className="action-icon" aria-hidden="true">{theme === 'day' ? '☀' : '☾'}</span><span className="action-label">{theme === 'day' ? '白天' : '夜晚'}</span></button>
      <button className="secondary" onClick={renameCurrent} disabled={!current || busy}>重命名</button>
      <button className="secondary" onClick={copyCurrentMarkdown} disabled={!current}>复制全文</button>
      <button className="secondary" onClick={cloneCurrent} disabled={!current || busy}>复制会话</button>
      <button className="secondary" onClick={pinCurrent} disabled={!current}>{currentPinned ? '取消置顶' : '置顶'}</button>
      <button className="secondary" onClick={exportCurrent} disabled={!current}>导出</button>
      <button className="secondary" onClick={showContextPreview} disabled={!current}>上下文</button>
      <button className="danger" onClick={deleteCurrent} disabled={!current || busy}>删除</button>
    </div>
  </div>;
}

export function ComposerBar({ busy, createPersistedSession, current, downloadAttachment, fileInputRef, guideActiveJob, handleFileSelect, input, inputRef, inputStats, modelPickerOpen, modelReady, openSettings, pendingAttachmentIDs, pendingAttachments, providerChoices, removePendingAttachment, selectChatModel, selectedChatModel, selectedModelProvider, sendMsg, setInput, setModelPickerOpen, stopStreaming, uploadingFiles }) {
  return <div className="composer-shell">
    {pendingAttachments.length ? <AttachmentList attachments={pendingAttachments} removable={!busy} onRemove={removePendingAttachment} onDownload={downloadAttachment} /> : null}
    <div className={'composer' + (busy ? ' composer-streaming' : '')}>
      <input ref={fileInputRef} type="file" multiple className="file-input" onChange={event => handleFileSelect(event, { current, createPersistedSession })} />
      <button className="secondary attach-control" disabled={busy || uploadingFiles} onClick={() => fileInputRef.current?.click()} aria-label="上传文件" title="上传文件"><svg className="attach-control-icon" aria-hidden="true" viewBox="0 0 24 24"><path d="M12 5v14M5 12h14" /></svg></button>
      <ComposerModelPicker busy={busy} providers={providerChoices} selectedProvider={selectedModelProvider} selectedModel={selectedChatModel} open={modelPickerOpen} setOpen={setModelPickerOpen} selectModel={selectChatModel} openSettings={openSettings} />
      {busy ? <button className="secondary stream-control guide-control" onClick={guideActiveJob} disabled={!input.trim()} aria-label="追加引导" title="追加引导">引导</button> : null}
      {busy ? <button className="danger stream-control stop-control" onClick={stopStreaming} aria-label="中断生成" title="中断生成">中断</button> : null}
      <textarea ref={inputRef} id="input" value={input} onChange={e => setInput(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); busy ? guideActiveJob() : sendMsg(); } }} placeholder={busy ? '输入引导内容' : '输入消息'} />
      <button id="send" disabled={busy || uploadingFiles || (!input.trim() && !pendingAttachmentIDs.length) || !modelReady} onClick={() => sendMsg()} title={!modelReady ? '请先配置模型' : '发送'}>发送</button>
    </div>
    {inputStats ? <div className="composer-meta">{inputStats}</div> : null}
  </div>;
}

export function Sidebar({ activeScheduledTasks, busy, clearScheduledTaskRunList, current, deleteSessionByID, editProject, filteredSessions, goHome, hasMoreSessions, loadingMoreSessions, newSession, onLoadMoreSessions, openScheduledTaskRunList, openSession, openSettings, pinSessionByID, projects, projectFilter, projectSessionCounts, renameSessionByID, selectedScheduledTask, selectedScheduledTaskID, selectedScheduledTaskSessions, sessionMenuID, sessionSearch, sessionSearchBusy, setProjectFilter, setSessionMenuID, setSessionSearch, sessions, setSidebarCollapsed, sidebarCollapsed }) {
  const [menuPosition, setMenuPosition] = React.useState(null);
  const sessionsRef = React.useRef(null);
  const loadMoreRef = React.useRef(null);
  const menuSession = filteredSessions.find(session => session.id === sessionMenuID) || null;

  React.useEffect(() => {
    if (!sessionMenuID) return undefined;
    const closeMenu = event => {
      if (event?.target?.closest?.('.session-menu-trigger, .session-row-menu-portal')) return;
      setSessionMenuID('');
    };
    window.addEventListener('resize', closeMenu);
    document.addEventListener('scroll', closeMenu, true);
    document.addEventListener('pointerdown', closeMenu, true);
    return () => {
      window.removeEventListener('resize', closeMenu);
      document.removeEventListener('scroll', closeMenu, true);
      document.removeEventListener('pointerdown', closeMenu, true);
    };
  }, [sessionMenuID, setSessionMenuID]);

  const toggleSessionMenu = (event, session) => {
    event.stopPropagation();
    if (sessionMenuID === session.id) {
      setSessionMenuID('');
      return;
    }
    const rect = event.currentTarget.getBoundingClientRect();
    const menuWidth = 146;
    const menuHeight = 146;
    const gap = 6;
    const left = Math.min(window.innerWidth - menuWidth - 8, Math.max(8, rect.right - menuWidth));
    const below = rect.bottom + gap;
    const top = below + menuHeight <= window.innerHeight - 8 ? below : Math.max(8, rect.top - menuHeight - gap);
    setMenuPosition({ left, top });
    setSessionMenuID(session.id);
  };

  React.useEffect(() => {
    if (sessionsRef.current) sessionsRef.current.scrollTop = 0;
  }, [projectFilter, selectedScheduledTaskID, sessionSearch]);

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

  const menu = menuSession && menuPosition ? createPortal(
    <div className="session-row-menu-portal" role="menu" style={menuPosition} onClick={event => event.stopPropagation()}>
      <button type="button" role="menuitem" onClick={() => { setSessionMenuID(''); pinSessionByID(menuSession.id, !!menuSession.pinned); }}>{menuSession.pinned ? '取消置顶' : '置顶'}</button>
      <button type="button" role="menuitem" className="danger" onClick={() => { setSessionMenuID(''); deleteSessionByID(menuSession.id, menuSession.title); }} disabled={busy}>删除</button>
      <button type="button" role="menuitem" onClick={() => { setSessionMenuID(''); renameSessionByID(menuSession.id, menuSession.title); }}>重命名标题</button>
    </div>,
    document.body,
  ) : null;

  return <>
    <aside>
      <div className="sidebar-head">
        <a className="brand" href="/" aria-label="返回 ChatDock 首页" style={{ color: 'inherit', textDecoration: 'none' }} onClick={event => { event.preventDefault(); goHome(); }}><span className="brand-mark" aria-hidden="true"><span /></span><div className="brand-copy"><span className="brand-text">ChatDock</span><div className="sub">AI workspace · local first</div></div></a>
        <button id="sidebarToggle" className="sidebar-toggle" onClick={() => setSidebarCollapsed(!sidebarCollapsed)} title={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}>{sidebarCollapsed ? '›' : '‹'}</button>
      </div>
      <div className="session-search-row"><label className="session-search-box"><input className="session-search" placeholder="搜索聊天记录" value={sessionSearch} onChange={event => setSessionSearch(event.target.value)} /></label><button className="new" onClick={newSession} aria-label="新会话" title="新会话"><span className="new-icon" aria-hidden="true">＋</span></button></div>
      <div className="project-nav">
        <div className="sidebar-section-head compact"><div className="sidebar-section-title">项目</div><button type="button" className="secondary small project-add-button" onClick={() => editProject?.()} title="新增项目" aria-label="新增项目">＋</button></div>
        <div className="project-nav-list">
          <button type="button" className={'project-nav-item ' + (projectFilter === 'all' ? 'active' : '')} onClick={() => setProjectFilter('all')}><span>全部会话</span><em>{projectSessionCounts?.all ?? sessions.length}</em></button>
          <button type="button" className={'project-nav-item ' + (projectFilter === 'plain' ? 'active' : '')} onClick={() => setProjectFilter('plain')}><span>普通会话</span><em>{projectSessionCounts?.plain ?? 0}</em></button>
          {(projects || []).map(project => <div key={project.id} className={'project-nav-row ' + (projectFilter === project.id ? 'active' : '')}>
            <button type="button" className="project-nav-item" onClick={() => setProjectFilter(project.id)}><span title={project.name}>{project.name}</span><em>{projectSessionCounts?.byProject?.[project.id] || 0}</em></button>
            <button type="button" className="project-edit-button" onClick={() => editProject?.(project)} title={'编辑项目：' + project.name} aria-label={'编辑项目：' + project.name}>•••</button>
          </div>)}
        </div>
      </div>
      {activeScheduledTasks.length ? <div className="sidebar-tasks"><div className="sidebar-section-head compact"><div className="sidebar-section-title">定时任务</div><span className="sidebar-section-count">{activeScheduledTasks.length}</span></div><div className="sidebar-task-list session-list-like">{activeScheduledTasks.slice(0, 3).map(task => <button key={task.id} type="button" className={'sidebar-task-item session ' + (selectedScheduledTaskID === task.id ? 'active ' : '') + (task.running ? 'running ' : '')} onClick={() => openScheduledTaskRunList(task.id)}><div className="session-main"><div className="sidebar-task-name session-title">{task.title || '未命名任务'}</div></div></button>)}</div>{activeScheduledTasks.length > 3 ? <button type="button" className="sidebar-task-more" onClick={() => openSettings('automation')}>查看全部 {activeScheduledTasks.length} 个任务</button> : null}</div> : null}
      <div className="sidebar-section-head"><div className="sidebar-section-title">{selectedScheduledTask ? '任务会话' : projectFilter === 'all' ? '全部会话' : projectFilter === 'plain' ? '普通会话' : ((projects || []).find(project => project.id === projectFilter)?.name || '项目会话')}</div>{selectedScheduledTask ? <button type="button" className="secondary small sidebar-clear-task" onClick={clearScheduledTaskRunList}>全部</button> : null}</div>
      {selectedScheduledTask ? <div className="session-search-meta">{selectedScheduledTask.title || '定时任务'} · {selectedScheduledTaskSessions.length} 次会话</div> : (sessionSearch.trim() ? <div className="session-search-meta">{sessionSearchBusy ? '搜索中…' : (hasMoreSessions ? '全文搜索 · 已加载 ' : '全文搜索 ') + filteredSessions.length + ' 条'}</div> : null)}
      <div id="sessions" ref={sessionsRef} onScroll={handleSessionScroll}>{filteredSessions.length ? filteredSessions.map(session => {
        const isActive = current === session.id;
        const menuOpen = sessionMenuID === session.id;
        return <div key={session.id} data-session-id={session.id} className={'session ' + (session.scheduled_run ? 'scheduled-run ' : '') + (isActive ? 'active ' : '') + (session.pinned ? 'pinned ' : '') + (menuOpen ? 'menu-open' : '')} onClick={() => openSession(session.id, session)}>
          <div className="session-main"><div className="session-title">{session.pinned ? <span className="pin-mark" aria-label="置顶" title="置顶" /> : null}{session.title}</div>{session.scheduled_run ? null : (session.match_snippet ? <div className="session-preview search-hit">{session.match_field ? session.match_field + '：' : ''}{session.match_snippet}</div> : (session.preview ? <div className="session-preview">{session.preview}</div> : null))}{session.scheduled_run ? null : <div className="session-meta">{session.count} 条 · {fmtTime(session.updated_at)}</div>}</div>
          <button type="button" className="session-menu-trigger" disabled={busy} onClick={event => toggleSessionMenu(event, session)} aria-label={(session.title || '会话') + ' 操作'} aria-expanded={menuOpen ? 'true' : 'false'} title="会话操作">⋯</button>
        </div>;
      }) : <div className="empty compact">{selectedScheduledTask ? '这个任务还没有可打开的运行会话' : (sessionSearch.trim() ? '没有匹配会话' : '暂无会话，开始新会话')}</div>}<div ref={loadMoreRef} style={{ minHeight: 1 }} aria-hidden="true" />{loadingMoreSessions ? <div className="session-search-meta" role="status">正在加载更多…</div> : null}</div>
    </aside>
    {menu}
  </>;
}
