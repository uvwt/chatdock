export function buildQuickActions(opts) {
  const { busy, current, exportCurrent, sendMsg, showContextPreview, showProviderSystemPrompt, setThemeState, theme } = opts;
  return [
    { id: 'continue', group: '会话', title: '发送“继续”', hint: '延续当前对话的上一轮内容', disabled: busy, run: () => sendMsg('继续') },
    { id: 'context-preview', group: '会话', title: '查看上下文 / Token 预览', hint: '检查实际消息构成与 Token 估算', disabled: !current, run: showContextPreview },
    { id: 'provider-system-prompt', group: '会话', title: '查看供应商 System Prompt', hint: '查看模型实际收到的完整 system 消息', disabled: !current, run: showProviderSystemPrompt },
    { id: 'export-session', group: '会话', title: '导出当前会话', hint: '下载 Markdown 文件', disabled: !current, run: exportCurrent },
    { id: 'theme', group: '界面', title: '切换明暗主题', hint: '当前：' + (theme === 'day' ? '白天' : '夜晚'), run: () => setThemeState(theme === 'day' ? 'night' : 'day') },
  ];
}
