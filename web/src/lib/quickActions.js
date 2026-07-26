export function buildQuickActions(opts) {
  const { busy, copyCurrentMarkdown, current, exportCurrent, sendMsg, showContextPreview, showSystemPrompt, setThemeState, theme } = opts;
  return [
    { id: 'continue', title: '发送"继续"', hint: '让当前会话继续上一轮内容', disabled: busy, run: () => sendMsg('继续') },
    { id: 'system-prompt', title: '查看系统提示词', hint: '当前会话生效的完整 System Prompt', disabled: !current, run: showSystemPrompt },
    { id: 'context-preview', title: '查看上下文 / Token 预览', hint: '查看实际发送给模型的消息构成', disabled: !current, run: showContextPreview },
    { id: 'copy-session', title: '复制当前会话全文', hint: '复制为 Markdown', disabled: !current, run: copyCurrentMarkdown },
    { id: 'export-session', title: '导出当前会话', hint: '下载 Markdown 文件', disabled: !current, run: exportCurrent },
    { id: 'theme', title: '切换明暗主题', hint: '当前：' + (theme === 'day' ? '白天' : '夜晚'), run: () => setThemeState(theme === 'day' ? 'night' : 'day') },
  ];
}
