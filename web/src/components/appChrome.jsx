import React from 'react';
import { createPortal } from 'react-dom';
import {
  ArrowUp,
  Menu,
  MessageSquarePlus,
  Moon,
  MoreHorizontal,
  Orbit,
  Paperclip,
  Pin,
  Search,
  Settings2,
  Square,
  Sun,
} from './icons.js';
import { AttachmentList } from './chat.jsx';
import { ComposerModelPicker } from './modelPicker.jsx';
import { TaskPanelToggle } from './taskPanel.jsx';
import { fmtTime } from '../lib/appUtils.js';

const iconProps = { size: 17, 'aria-hidden': true };

export function Topbar({ busy, cloneCurrent, copyCurrentMarkdown, current, currentPinned, currentTitle, deleteCurrent, exportCurrent, newSession, openSettings, pinCurrent, renameCurrent, setQuickPaletteOpen, setSidebarCollapsed, setThemeState, showContextPreview, sidebarCollapsed, taskPanelAvailable, taskPanelOpen, taskPanelTasks, theme, toggleTaskPanel }) {
  const darkMode = theme !== 'day';
  const ThemeIcon = darkMode ? Sun : Moon;
  return <div className="topbar">
    <div className="top-left">
      <button className="mobile-menu icon-button" onClick={() => setSidebarCollapsed(!sidebarCollapsed)} aria-label="打开会话列表" title="会话列表"><Menu {...iconProps} /></button>
      <div className="topbar-title-wrap"><b id="title">{currentTitle}</b></div>
    </div>
    <div className="top-actions">
      <button className="secondary quick-palette-toggle" onClick={() => setQuickPaletteOpen(true)} title="快捷指令（⌘/Ctrl K）"><Search {...iconProps} /><span className="action-label">快捷</span></button>
      <button className="secondary config-toggle" onClick={() => openSettings()} title="配置中心"><Settings2 {...iconProps} /><span className="action-label">配置</span></button>
      <TaskPanelToggle available={taskPanelAvailable} open={taskPanelOpen} tasks={taskPanelTasks} onClick={toggleTaskPanel} />
      <button className="secondary session-actions-toggle mobile-new-toggle icon-button" onClick={newSession} aria-label="新会话" title="新会话"><MessageSquarePlus {...iconProps} /></button>
      <button className="theme-toggle" onClick={() => setThemeState(darkMode ? 'day' : 'night')} aria-label={darkMode ? '切换到浅色模式' : '切换到深色模式'} title={darkMode ? '切换到浅色模式' : '切换到深色模式'}>
        <ThemeIcon {...iconProps} />
        <span className="action-label">{darkMode ? '浅色' : '深色'}</span>
      </button>
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
      <button className="secondary attach-control icon-button" disabled={busy || uploadingFiles} onClick={() => fileInputRef.current?.click()} aria-label="上传文件" title="上传文件"><Paperclip {...iconProps} /></button>
      <ComposerModelPicker busy={busy} providers={providerChoices} selectedProvider={selectedModelProvider} selectedModel={selectedChatModel} open={modelPickerOpen} setOpen={setModelPickerOpen} selectModel={selectChatModel} openSettings={openSettings} />
      {busy ? <button className="secondary stream-control guide-control" onClick={guideActiveJob} disabled={!input.trim()} aria-label="追加引导" title="追加引导"><span>引导</span></button> : null}
      {busy ? <button className="danger stream-control stop-control" onClick={stopStreaming} aria-label="中断生成" title="中断生成"><Square size={13} aria-hidden="true" /><span>中断</span></button> : null}
      <textarea ref={inputRef} id="input" value={input} onChange={e => setInput(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); busy ? guideActiveJob() : sendMsg(); } }} placeholder={busy ? '输入引导内容' : '输入消息'} />
      <button id="send" className="icon-button" disabled={busy || uploadingFiles || (!input.trim() && !pendingAttachmentIDs.length) || !modelReady} onClick={() => sendMsg()} aria-label={!modelReady ? '请先配置模型' : '发送'} title={!modelReady ? '请先配置模型' : '发送'}><ArrowUp size={19} aria-hidden="true" /></button>
    </div>
    {inputStats ? <div className="composer-meta">{inputStats}</div> : null}
  </div>;
}


export function Sidebar({ busy, current, deleteSessionByID, filteredSessions, goHome, hasMoreSessions, loadingMoreSessions, newSession, onLoadMoreSessions, openSession, openWorkspacePage, pinSessionByID, projects, projectFilter, renameSessionByID, sessionMenuID, sessionSearch, sessionSearchBusy, setSessionMenuID, setSessionSearch, setSidebarCollapsed, sidebarCollapsed, workspacePage }) {
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

  const menu = menuSession && menuPosition ? createPortal(
    <div className="session-row-menu-portal" role="menu" style={menuPosition} onClick={event => event.stopPropagation()}>
      <button type="button" role="menuitem" onClick={() => { setSessionMenuID(''); pinSessionByID(menuSession.id, !!menuSession.pinned); }}>{menuSession.pinned ? '取消置顶' : '置顶'}</button>
      <button type="button" role="menuitem" className="danger" onClick={() => { setSessionMenuID(''); deleteSessionByID(menuSession.id, menuSession.title); }} disabled={busy}>删除</button>
      <button type="button" role="menuitem" onClick={() => { setSessionMenuID(''); renameSessionByID(menuSession.id, menuSession.title); }}>重命名标题</button>
    </div>,
    document.body,
  ) : null;

  const sessionSectionTitle = projectFilter === 'all'
    ? '全部会话'
    : projectFilter === 'plain'
      ? '普通会话'
      : ((projects || []).find(project => project.id === projectFilter)?.name || '项目会话');

  return <>
    <aside>
      <div className="sidebar-head">
        <a className="brand" href="/" aria-label="返回 ChatDock 首页" onClick={event => { event.preventDefault(); goHome(); }}>
          <span className="brand-mark" aria-hidden="true"><Orbit size={20} /></span>
          <div className="brand-copy"><span className="brand-text">ChatDock</span><div className="sub">Local AI</div></div>
        </a>
        <button id="sidebarToggle" className="sidebar-toggle icon-button" onClick={() => setSidebarCollapsed(!sidebarCollapsed)} title={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'} aria-label={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}><Menu {...iconProps} /></button>
      </div>
      <div className="session-search-row">
        <label className="session-search-box"><Search size={15} aria-hidden="true" /><input className="session-search" placeholder="搜索聊天记录" value={sessionSearch} onChange={event => setSessionSearch(event.target.value)} /></label>
        <button className="new icon-button" onClick={newSession} aria-label="新会话" title="新会话"><MessageSquarePlus {...iconProps} /></button>
      </div>
      <nav className="sidebar-hubs" aria-label="工作区">
        <button type="button" className={workspacePage === 'projects' ? 'active' : ''} onClick={() => openWorkspacePage('projects')}><span>项目</span><small>管理上下文</small></button>
        <button type="button" className={workspacePage === 'scheduled-tasks' ? 'active' : ''} onClick={() => openWorkspacePage('scheduled-tasks')}><span>定时任务</span><small>管理自动执行</small></button>
      </nav>
      <div className="sidebar-section-head"><div className="sidebar-section-title">{sessionSectionTitle}</div></div>
      {sessionSearch.trim() ? <div className="session-search-meta">{sessionSearchBusy ? '搜索中…' : (hasMoreSessions ? '全文搜索 · 已加载 ' : '全文搜索 ') + filteredSessions.length + ' 条'}</div> : null}
      <div id="sessions" ref={sessionsRef} onScroll={handleSessionScroll}>{filteredSessions.length ? filteredSessions.map(session => {
        const isActive = current === session.id;
        const menuOpen = sessionMenuID === session.id;
        return <div key={session.id} data-session-id={session.id} className={'session ' + (session.scheduled_run ? 'scheduled-run ' : '') + (isActive ? 'active ' : '') + (session.pinned ? 'pinned ' : '') + (menuOpen ? 'menu-open' : '')} onClick={() => openSession(session.id, session)}>
          <div className="session-main"><div className="session-title">{session.pinned ? <Pin className="pin-mark" size={13} aria-label="置顶" /> : null}{session.title}</div>{session.scheduled_run ? null : (session.match_snippet ? <div className="session-preview search-hit">{session.match_field ? session.match_field + '：' : ''}{session.match_snippet}</div> : (session.preview ? <div className="session-preview">{session.preview}</div> : null))}{session.scheduled_run ? null : <div className="session-meta">{session.count} 条 · {fmtTime(session.updated_at)}</div>}</div>
          <button type="button" className="session-menu-trigger icon-button" disabled={busy} onClick={event => toggleSessionMenu(event, session)} aria-label={(session.title || '会话') + ' 操作'} aria-expanded={menuOpen ? 'true' : 'false'} title="会话操作"><MoreHorizontal size={16} aria-hidden="true" /></button>
        </div>;
      }) : <div className="empty compact">{sessionSearch.trim() ? '没有匹配会话' : '暂无会话，开始新会话'}</div>}<div ref={loadMoreRef} style={{ minHeight: 1 }} aria-hidden="true" />{loadingMoreSessions ? <div className="session-search-meta" role="status">正在加载更多…</div> : null}</div>
    </aside>
    {menu}
  </>;
}
