import React from 'react';
import { AttachmentList } from './chat.jsx';
import { ComposerModelPicker } from './modelPicker.jsx';
import { TaskPanelToggle } from './taskPanel.jsx';
import { fmtTime } from '../lib/appUtils.js';

function ChromeIcon({ name }) {
  const paths = {
    menu: <path d="M4 7h16M4 12h16M4 17h16" />,
    panel: <path d="M4 5h16v14H4zM9 5v14" />,
    compose: <path d="M12 20h8M16.5 3.5a2.12 2.12 0 0 1 3 3L8 18l-4 1 1-4Z" />,
    search: <><circle cx="11" cy="11" r="6" /><path d="m16 16 4 4" /></>,
    sparkles: <path d="m12 3 1.45 3.55L17 8l-3.55 1.45L12 13l-1.45-3.55L7 8l3.55-1.45ZM5 14l.8 2.2L8 17l-2.2.8L5 20l-.8-2.2L2 17l2.2-.8Z" />,
    settings: <><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.83 2.83-.06-.06A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 .6 1.7 1.7 0 0 0-.4 1.1V21h-4v-.09A1.7 1.7 0 0 0 8.6 19.4a1.7 1.7 0 0 0-1.88.34l-.06.06-2.83-2.83.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-.6-1 1.7 1.7 0 0 0-1.1-.4H3v-4h.09A1.7 1.7 0 0 0 4.6 8.6a1.7 1.7 0 0 0-.34-1.88l-.06-.06 2.83-2.83.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-.6 1.7 1.7 0 0 0 .4-1.1V3h4v.09A1.7 1.7 0 0 0 15.4 4.6a1.7 1.7 0 0 0 1.88-.34l.06-.06 2.83 2.83-.06.06A1.7 1.7 0 0 0 19.4 9c.2.36.31.76.31 1.17V10h.09v4h-.09c0 .35-.11.69-.31 1Z" /></>,
    sun: <><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.93 4.93l1.42 1.42M17.65 17.65l1.42 1.42M2 12h2M20 12h2M4.93 19.07l1.42-1.42M17.65 6.35l1.42-1.42" /></>,
    moon: <path d="M20 15.3A8.5 8.5 0 0 1 8.7 4 8.5 8.5 0 1 0 20 15.3Z" />,
  };
  return <svg className="chrome-icon" aria-hidden="true" viewBox="0 0 24 24">{paths[name]}</svg>;
}

export function Topbar({ busy, cloneCurrent, copyCurrentMarkdown, current, currentPinned, currentTitle, deleteCurrent, exportCurrent, newSession, openSettings, pinCurrent, renameCurrent, setQuickPaletteOpen, setSidebarCollapsed, setThemeState, showContextPreview, sidebarCollapsed, taskPanelOpen, taskPanelTasks, theme, toggleTaskPanel }) {
  return <div className="topbar">
    <div className="top-left">
      <button className="mobile-menu" onClick={() => setSidebarCollapsed(!sidebarCollapsed)} aria-label="打开会话栏" title="打开会话栏"><ChromeIcon name="menu" /></button>
      <b id="title">{currentTitle}</b>
    </div>
    <div className="top-actions">
      <button className="secondary quick-palette-toggle" onClick={() => setQuickPaletteOpen(true)} aria-label="快捷指令" title="快捷指令（⌘/Ctrl K）"><ChromeIcon name="sparkles" /><span className="action-label">快捷</span></button>
      <button className="secondary config-toggle" onClick={() => openSettings()} aria-label="配置中心" title="配置中心"><ChromeIcon name="settings" /><span className="action-label">配置</span></button>
      <TaskPanelToggle open={taskPanelOpen} tasks={taskPanelTasks} onClick={toggleTaskPanel} />
      <button className="secondary session-actions-toggle mobile-new-toggle" onClick={newSession} aria-label="新会话" title="新会话"><ChromeIcon name="compose" /></button>
      <button className="theme-toggle" onClick={() => setThemeState(theme === 'day' ? 'night' : 'day')} aria-label={theme === 'day' ? '切换到深色模式' : '切换到浅色模式'} title={theme === 'day' ? '深色模式' : '浅色模式'}><ChromeIcon name={theme === 'day' ? 'moon' : 'sun'} /><span className="action-label">{theme === 'day' ? '深色' : '浅色'}</span></button>
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
      <textarea ref={inputRef} id="input" value={input} onChange={e => setInput(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); busy ? guideActiveJob() : sendMsg(); } }} placeholder={busy ? '输入引导内容' : '向 ChatDock 发送消息'} />
      <button id="send" disabled={busy || uploadingFiles || (!input.trim() && !pendingAttachmentIDs.length) || !modelReady} onClick={() => sendMsg()} title={!modelReady ? '请先配置模型' : '发送'}>发送</button>
    </div>
    {inputStats ? <div className="composer-meta">{inputStats}</div> : null}
  </div>;
}

export function Sidebar({ activeWorkspace, activeScheduledTasks, busy, clearScheduledTaskRunList, current, deleteSessionByID, filteredSessions, newSession, openScheduledTaskRunList, openSession, openSettings, pinSessionByID, workspaceSummaries, renameSessionByID, selectedScheduledTask, selectedScheduledTaskID, selectedScheduledTaskSessions, sessionMenuID, sessionSearch, sessionSearchBusy, setSessionMenuID, setSessionSearch, setWorkspacePickerOpen, sessions, setSidebarCollapsed, sidebarCollapsed }) {
  const workspaceName = activeWorkspace ? (activeWorkspace.name === 'default' ? '默认工作区' : activeWorkspace.name) : '未选择';
  return <aside>
    <div className="sidebar-head">
      <button className="sidebar-brand-button" type="button" onClick={newSession} aria-label="ChatDock 新会话" title="ChatDock 新会话">
        <span className="brand-logo" aria-hidden="true">C</span>
        <span className="brand-text">ChatDock</span>
      </button>
      <button id="sidebarToggle" className="sidebar-toggle" onClick={() => setSidebarCollapsed(!sidebarCollapsed)} aria-label={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'} title={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}><ChromeIcon name="panel" /></button>
    </div>

    <button className="new sidebar-new-chat" onClick={newSession}><ChromeIcon name="compose" /><span className="new-label">新聊天</span></button>
    <label className="session-search-box"><ChromeIcon name="search" /><input className="session-search" placeholder="搜索聊天记录" value={sessionSearch} onChange={e => setSessionSearch(e.target.value)} /></label>
    <div className="prompt-box"><button className="workspace-picker-trigger" type="button" disabled={busy || !workspaceSummaries.length} onClick={() => setWorkspacePickerOpen(true)}><span className="workspace-picker-name">{workspaceName}</span><span className="workspace-picker-meta">{activeWorkspace ? activeWorkspace.count : sessions.length} 个会话</span><span className="workspace-picker-arrow" aria-hidden="true">⌄</span></button></div>

    {activeScheduledTasks.length ? <div className="sidebar-tasks"><div className="sidebar-section-head compact"><div className="sidebar-section-title">定时任务</div><span className="sidebar-section-count">{activeScheduledTasks.length}</span></div><div className="sidebar-task-list session-list-like">{activeScheduledTasks.slice(0, 3).map(task => <button key={task.id} type="button" className={'sidebar-task-item session ' + (selectedScheduledTaskID === task.id ? 'active ' : '') + (task.running ? 'running ' : '')} onClick={() => openScheduledTaskRunList(task.id)}><div className="session-main"><div className="sidebar-task-name session-title">{task.title || '未命名任务'}</div></div></button>)}</div>{activeScheduledTasks.length > 3 ? <button type="button" className="sidebar-task-more" onClick={() => openSettings('automation')}>查看全部 {activeScheduledTasks.length} 个任务</button> : null}</div> : null}

    <div className="sidebar-section-head"><div className="sidebar-section-title">{selectedScheduledTask ? '任务会话' : '聊天记录'}</div>{selectedScheduledTask ? <button type="button" className="secondary small sidebar-clear-task" onClick={clearScheduledTaskRunList}>全部</button> : null}</div>
    {selectedScheduledTask ? <div className="session-search-meta">{selectedScheduledTask.title || '定时任务'} · {selectedScheduledTaskSessions.length} 次会话</div> : (sessionSearch.trim() ? <div className="session-search-meta">{sessionSearchBusy ? '搜索中…' : '全文搜索 ' + filteredSessions.length + ' 条'}</div> : null)}
    <div id="sessions">{filteredSessions.length ? filteredSessions.map(s => {
      const isActive = current === s.id;
      const menuOpen = sessionMenuID === s.id;
      return <div key={s.id} className={'session ' + (s.scheduled_run ? 'scheduled-run ' : '') + (isActive ? 'active ' : '') + (s.pinned ? 'pinned ' : '') + (menuOpen ? 'menu-open' : '')} onClick={() => openSession(s.id, s)}>
        <div className="session-main"><div className="session-title">{s.pinned ? <span className="pin-mark" aria-label="置顶" title="置顶" /> : null}{s.title}</div>{s.scheduled_run ? null : (s.match_snippet ? <div className="session-preview search-hit">{s.match_field ? s.match_field + '：' : ''}{s.match_snippet}</div> : (s.preview ? <div className="session-preview">{s.preview}</div> : null))}{s.scheduled_run ? null : <div className="session-meta">{s.count} 条 · {fmtTime(s.updated_at)}</div>}</div>
        <button type="button" className="session-menu-trigger" disabled={busy} onClick={e => { e.stopPropagation(); setSessionMenuID(menuOpen ? '' : s.id); }} aria-label={(s.title || '会话') + ' 操作'} aria-expanded={menuOpen ? 'true' : 'false'} title="会话操作">⋯</button>
        {menuOpen ? <div className="session-row-menu" onClick={e => e.stopPropagation()}><button type="button" onClick={() => pinSessionByID(s.id, !!s.pinned)}>{s.pinned ? '取消置顶' : '置顶'}</button><button type="button" className="danger" onClick={() => { setSessionMenuID(''); deleteSessionByID(s.id, s.title); }} disabled={busy}>删除</button><button type="button" onClick={() => renameSessionByID(s.id, s.title)}>重命名标题</button></div> : null}
      </div>;
    }) : <div className="empty compact">{selectedScheduledTask ? '这个任务还没有可打开的运行会话' : (sessionSearch.trim() ? '没有匹配会话' : '暂无会话，开始新会话')}</div>}</div>

    <button className="sidebar-settings-entry" type="button" onClick={() => openSettings()}><ChromeIcon name="settings" /><span><b>设置</b><small>{workspaceName}</small></span></button>
  </aside>;
}
