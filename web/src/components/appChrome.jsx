import React from 'react';
import { AttachmentList } from './chat.jsx';
import { ComposerModelPicker } from './modelPicker.jsx';
import { fmtTime } from '../lib/appUtils.js';

export function Topbar({ busy, cloneCurrent, copyCurrentMarkdown, current, currentPinned, currentTitle, deleteCurrent, exportCurrent, newSession, openSettings, pinCurrent, renameCurrent, setQuickPaletteOpen, setSidebarCollapsed, setThemeState, showContextPreview, sidebarCollapsed, theme }) {
  return <div className="topbar">
    <div className="top-left"><button className="mobile-menu" onClick={() => setSidebarCollapsed(!sidebarCollapsed)}>☰</button><b id="title">{currentTitle}</b></div>
    <div className="top-actions">
      <button className="secondary quick-palette-toggle" onClick={() => setQuickPaletteOpen(true)} title="快捷指令（⌘/Ctrl K）"><span className="action-icon" aria-hidden="true">✦</span><span className="action-label">快捷</span></button>
      <button className="secondary config-toggle" onClick={() => openSettings()} title="配置中心"><span className="action-icon" aria-hidden="true">⚙</span><span className="action-label">配置</span></button>
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
      <button className="secondary attach-control" disabled={busy || uploadingFiles} onClick={() => fileInputRef.current?.click()} title="上传文件">+</button>
      <ComposerModelPicker busy={busy} providers={providerChoices} selectedProvider={selectedModelProvider} selectedModel={selectedChatModel} open={modelPickerOpen} setOpen={setModelPickerOpen} selectModel={selectChatModel} openSettings={openSettings} />
      {busy ? <button className="secondary stream-control guide-control" onClick={guideActiveJob} disabled={!input.trim()} aria-label="追加引导" title="追加引导">引导</button> : null}
      {busy ? <button className="danger stream-control stop-control" onClick={stopStreaming} aria-label="中断生成" title="中断生成">中断</button> : null}
      <textarea ref={inputRef} id="input" value={input} onChange={e => setInput(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); busy ? guideActiveJob() : sendMsg(); } }} placeholder={busy ? '生成中可输入引导内容，不会中断当前回答' : '输入消息'} />
      <button id="send" disabled={busy || uploadingFiles || (!input.trim() && !pendingAttachmentIDs.length) || !modelReady} onClick={() => sendMsg()} title={!modelReady ? '请先配置模型' : '发送'}>发送</button>
    </div>
    {inputStats ? <div className="composer-meta">{inputStats}</div> : null}
  </div>;
}

export function Sidebar({ activeWorkspace, activeScheduledTasks, busy, clearScheduledTaskRunList, current, deleteSessionByID, filteredSessions, newSession, openScheduledTaskRunList, openSession, openSettings, pinSessionByID, workspaceSummaries, renameSessionByID, selectedScheduledTask, selectedScheduledTaskID, selectedScheduledTaskSessions, sessionMenuID, sessionSearch, sessionSearchBusy, setSessionMenuID, setSessionSearch, setWorkspacePickerOpen, sessions, setSidebarCollapsed, sidebarCollapsed }) {
  return <aside>
    <div className="sidebar-head">
      <div className="brand"><div className="brand-copy"><span className="brand-text">ChatDock</span><div className="sub">会话、工具、任务，一站协同</div></div></div>
      <button id="sidebarToggle" className="sidebar-toggle" onClick={() => setSidebarCollapsed(!sidebarCollapsed)} title={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}>{sidebarCollapsed ? '›' : '‹'}</button>
    </div>
    <div className="prompt-box"><button className="workspace-picker-trigger" type="button" disabled={busy || !workspaceSummaries.length} onClick={() => setWorkspacePickerOpen(true)}><span className="workspace-picker-name">{activeWorkspace ? (activeWorkspace.name === 'default' ? '默认工作区' : activeWorkspace.name) : '未选择'}</span><span className="workspace-picker-meta">{activeWorkspace ? activeWorkspace.count : sessions.length}</span></button></div>
    <div className="session-search-row"><label className="session-search-box"><input className="session-search" placeholder="搜索聊天记录" value={sessionSearch} onChange={e => setSessionSearch(e.target.value)} /></label><button className="new" onClick={newSession} aria-label="新会话" title="新会话"><span className="new-icon" aria-hidden="true">＋</span></button></div>
    {activeScheduledTasks.length ? <div className="sidebar-tasks"><div className="sidebar-section-head compact"><div className="sidebar-section-title">定时任务</div><span className="sidebar-section-count">{activeScheduledTasks.length}</span></div><div className="sidebar-task-list session-list-like">{activeScheduledTasks.slice(0, 3).map(task => <button key={task.id} type="button" className={'sidebar-task-item session ' + (selectedScheduledTaskID === task.id ? 'active ' : '') + (task.running ? 'running ' : '')} onClick={() => openScheduledTaskRunList(task.id)}><div className="session-main"><div className="sidebar-task-name session-title">{task.title || '未命名任务'}</div></div></button>)}</div>{activeScheduledTasks.length > 3 ? <button type="button" className="sidebar-task-more" onClick={() => openSettings('automation')}>查看全部 {activeScheduledTasks.length} 个任务</button> : null}</div> : null}
    <div className="sidebar-section-head"><div className="sidebar-section-title">{selectedScheduledTask ? '任务会话' : '最近会话'}</div>{selectedScheduledTask ? <button type="button" className="secondary small sidebar-clear-task" onClick={clearScheduledTaskRunList}>全部</button> : null}</div>
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
  </aside>;
}
