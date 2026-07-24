export function buildQuickActions(opts) {
  const { branchCurrent, busy, cloneCurrent, copyCurrentMarkdown, copyText, createSession, current, currentPinned, deleteCurrent, exportCurrent, inputRef, messagesLength, openSettings, openWorkspacePage, pinCurrent, productDiagnostics, renameCurrent, sendMsg, setProjectFilter, setThemeState, showContextPreview, theme } = opts;
  return [
    { id: 'focus-input', title: '聚焦输入框', hint: '按 / 也可以快速输入', run: () => inputRef.current?.focus() },
    { id: 'new-session', title: '新建会话', hint: '按当前会话筛选创建新对话', run: createSession },
    { id: 'continue', title: '发送“继续”', hint: '让当前会话继续上一轮内容', disabled: busy, run: () => sendMsg('继续') },
    { id: 'all-sessions', title: '全部会话', hint: '显示所有普通会话和项目会话', run: () => setProjectFilter?.('all') },
    { id: 'plain-sessions', title: '普通会话', hint: '只显示未归属项目的会话', run: () => setProjectFilter?.('plain') },
    { id: 'settings-projects', title: '项目管理', hint: '管理项目名称、提示词和会话归属', run: () => openWorkspacePage('projects') },
    { id: 'settings', title: '打开配置中心', hint: '模型、工具和系统设置', run: () => openSettings() },
    { id: 'settings-model', title: '模型设置', hint: 'Base URL、API Key、模型和最终 Prompt', run: () => openSettings('model') },
    { id: 'settings-tools', title: '工具中心', hint: 'MCP 配置、状态检测和连接测试', run: () => openSettings('tools') },
    { id: 'settings-automation', title: '定时任务', hint: '创建、运行和暂停自动执行任务', run: () => openWorkspacePage('scheduled-tasks') },
    { id: 'settings-system', title: '系统与数据', hint: '运行状态、数据库、备份和诊断信息', run: () => openSettings('security') },
    { id: 'copy-diagnostics', title: '复制诊断信息', hint: '复制脱敏后的系统、数据库、备份和 MCP 状态', run: () => copyText(productDiagnostics) },
    { id: 'copy-session', title: '复制当前会话全文', hint: '复制为 Markdown', disabled: !current, run: copyCurrentMarkdown },
    { id: 'export-session', title: '导出当前会话', hint: '下载 Markdown 文件', disabled: !current, run: exportCurrent },
    { id: 'context-preview', title: '查看上下文 / Token 预览', hint: '查看实际发送给模型的消息构成', disabled: !current, run: showContextPreview },
    { id: 'delete-session', title: '删除当前会话', hint: '删除后不可恢复', disabled: !current || busy, run: deleteCurrent },
    { id: 'rename-session', title: '重命名当前会话', hint: '整理侧栏会话列表', disabled: !current || busy, run: renameCurrent },
    { id: 'clone-session', title: '复制当前会话', hint: '保留上下文开一个副本', disabled: !current || busy, run: cloneCurrent },
    { id: 'branch-session', title: '创建分支对话', hint: '在新聊天中从当前上下文继续', disabled: !current || busy || !messagesLength, run: () => branchCurrent() },
    { id: 'pin-session', title: currentPinned ? '取消置顶当前会话' : '置顶当前会话', hint: '让重要会话固定在列表顶部', disabled: !current, run: pinCurrent },
    { id: 'theme', title: '切换明暗主题', hint: '当前：' + (theme === 'day' ? '白天' : '夜晚'), run: () => setThemeState(theme === 'day' ? 'night' : 'day') },
  ];
}
